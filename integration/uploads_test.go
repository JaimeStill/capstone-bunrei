//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/standards-lab/docinfer"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-web-sdk/upload"
	"github.com/standards-lab/go-web-sdk/upload/uploadtest"
	"github.com/standards-lab/go-web-sdk/webtest"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/uploads"
	"github.com/JaimeStill/capstone-bunrei/integration"
)

const uploadDigest = "0000000000000000000000000000000000000000000000000000000000000000"

type replicas struct {
	stores  [2]*uploads.Service
	objects *storage.Store
	prefix  string
	next    atomic.Uint64
}

func (r *replicas) pick() *uploads.Service { return r.stores[r.next.Add(1)%2] }

type namespaced struct {
	*replicas
	ns string
}

func (s namespaced) Create(ctx context.Context, id, mediaType, digest, name string, length int64, expires time.Time) error {
	return s.pick().Create(ctx, s.ns+id, mediaType, digest, name, length, expires)
}

func (s namespaced) Meta(ctx context.Context, id string) (string, string, string, error) {
	return s.pick().Meta(ctx, s.ns+id)
}

func (s namespaced) Progress(ctx context.Context, id string) (int64, int64, []byte, time.Time, error) {
	return s.pick().Progress(ctx, s.ns+id)
}

func (s namespaced) SetLength(ctx context.Context, id string, length int64) (int64, error) {
	return s.pick().SetLength(ctx, s.ns+id, length)
}

func (s namespaced) Append(ctx context.Context, id string, offset int64, chunk, state []byte) (int64, bool, error) {
	return s.pick().Append(ctx, s.ns+id, offset, chunk, state)
}

func (s namespaced) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	return s.pick().Open(ctx, s.ns+id)
}

func (s namespaced) Delete(ctx context.Context, id string) error {
	return s.pick().Delete(ctx, s.ns+id)
}

func run() string { return fmt.Sprintf("uploads-%d", time.Now().UnixNano()) }

func uploadReplicas(t *testing.T) *replicas {
	t.Helper()
	prefix := run() + "/upload/"
	t.Setenv("BUNREI_OBJECTS_UPLOAD_PREFIX", prefix)
	a := integration.Compose(t)
	n := a.Nodes()
	r := &replicas{prefix: prefix}
	for i := range r.stores {
		sys, err := a.Graph().Build(n.Schema, n.Uploads)
		if err != nil {
			t.Fatalf("Build replica %d: %v", i, err)
		}
		start(t, "database", sys.Get(n.Database))
		start(t, "storage", sys.Get(n.Storage))
		start(t, "schema", sys.Get(n.Schema))
		r.stores[i] = sys.Get(n.Uploads)
		r.objects = sys.Get(n.Storage)
	}
	return r
}

func objectsUnder(t *testing.T, objects *storage.Store, prefix string) []string {
	t.Helper()
	var keys []string
	opts := storage.ListOptions{Prefix: prefix}
	for {
		page, err := objects.List(context.Background(), opts)
		if err != nil {
			t.Fatalf("List %s: %v", prefix, err)
		}
		for _, obj := range page.Objects {
			keys = append(keys, obj.Key)
		}
		if page.Next == "" {
			return keys
		}
		opts.Token = page.Next
	}
}

func uploadRows(t *testing.T, db *sql.DB, table, column, id string) int {
	t.Helper()
	var n int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", table, column)
	if err := db.QueryRowContext(context.Background(), query, id).Scan(&n); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	return n
}

func TestUploads_StoreConformsAcrossReplicas(t *testing.T) {
	r := uploadReplicas(t)
	ns := run()
	var calls atomic.Uint64
	uploadtest.Run(t, func() upload.Store {
		return namespaced{replicas: r, ns: fmt.Sprintf("%s-%d-", ns, calls.Add(1))}
	})
}

func TestUploads_RacingAppendLoserLeavesNoObject(t *testing.T) {
	r := uploadReplicas(t)
	db := integration.Database(t)
	ctx := context.Background()
	for i := range 10 {
		id := fmt.Sprintf("%s-race-%d", run(), i)
		if err := r.stores[0].Create(ctx, id, "application/pdf", uploadDigest, "f.pdf", -1, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var committed atomic.Int32
		start := make(chan struct{})
		for j := range r.stores {
			s := r.stores[j]
			wg.Go(func() {
				<-start
				_, ok, err := s.Append(ctx, id, 0, []byte{byte(j + 1), byte(j + 1)}, []byte{byte(j)})
				if err != nil {
					t.Errorf("Append from replica %d = %v", j, err)
				}
				if ok {
					committed.Add(1)
				}
			})
		}
		close(start)
		wg.Wait()
		if got := committed.Load(); got != 1 {
			t.Fatalf("racing appends committed %d, want 1", got)
		}
		if got := uploadRows(t, db, "upload_chunk", "upload_id", id); got != 1 {
			t.Errorf("chunk rows after the race = %d, want the winner's alone", got)
		}
		if got := objectsUnder(t, r.objects, r.prefix+id+"/"); len(got) != 1 {
			t.Errorf("objects after the race = %v, want the winner's alone", got)
		}
	}
}

func TestUploads_ChunksAreObjectsTheRowsName(t *testing.T) {
	r := uploadReplicas(t)
	db := integration.Database(t)
	ctx := context.Background()
	id := run() + "-a"
	if err := r.stores[0].Create(ctx, id, "application/pdf", uploadDigest, "f.pdf", 6, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, at := range []int64{0, 3} {
		if _, ok, err := r.stores[1].Append(ctx, id, at, []byte("abc"), nil); err != nil || !ok {
			t.Fatalf("Append at %d = %v, %v", at, ok, err)
		}
	}

	rows, err := db.QueryContext(ctx, "SELECT chunk_offset, object_key FROM upload_chunk WHERE upload_id = $1 ORDER BY chunk_offset", id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var recorded []string
	for rows.Next() {
		var offset int64
		var key string
		if err := rows.Scan(&offset, &key); err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("%s%s/%d/", r.prefix, id, offset); !strings.HasPrefix(key, want) || key == want {
			t.Errorf("chunk at %d names %s, want a key under %s", offset, key, want)
		}
		recorded = append(recorded, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if got := objectsUnder(t, r.objects, r.prefix+id+"/"); len(got) != 2 || len(recorded) != 2 {
		t.Fatalf("objects = %v, chunk rows = %v; want two of each", got, recorded)
	}

	if err := r.stores[0].Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if got := uploadRows(t, db, "upload_chunk", "upload_id", id); got != 0 {
		t.Errorf("chunk rows after the delete = %d, want none", got)
	}
	if got := objectsUnder(t, r.objects, r.prefix+id+"/"); len(got) != 0 {
		t.Errorf("objects after the delete = %v, want none", got)
	}
}

type cutOff struct {
	base    http.RoundTripper
	patches atomic.Int32
	at      int32
}

func (c *cutOff) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPatch && c.patches.Add(1) == c.at {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body[:len(body)/2]), failing{}))
	}
	return c.base.RoundTrip(req)
}

type failing struct{}

func (failing) Read([]byte) (int, error) { return 0, errors.New("the connection dropped") }

func composedStorage(t *testing.T) *storage.Store {
	t.Helper()
	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Storage)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	store := sys.Get(n.Storage)
	start(t, "storage", store)
	return store
}

func uploadOffset(t *testing.T, location string) (int, int64) {
	t.Helper()
	req, err := http.NewRequest(http.MethodHead, location, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HEAD %s: %v", location, err)
	}
	_ = res.Body.Close()
	offset, _ := upload.ParseInteger(res.Header.Get("Upload-Offset"))
	return res.StatusCode, offset
}

func TestUploads_AFileSentInChunksAndResumedQueuesAJob(t *testing.T) {
	objects := composedStorage(t)
	s := integration.Start(t, integration.Options{})
	db := integration.Database(t)
	ctx := context.Background()

	const chunk = 16 << 10
	file := bytes.Repeat([]byte("a scanned page of a large document. "), 3000)
	raw := sha256.Sum256(file)
	sum := hex.EncodeToString(raw[:])
	cut := &cutOff{base: http.DefaultTransport, at: 3}
	c := upload.Client{HTTP: &http.Client{Transport: cut}, Attempts: 1, ChunkSize: chunk}

	res, err := c.Create(ctx, s.URL()+"/api/uploads", "application/pdf", "scan.pdf", bytes.NewReader(file))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := res.Send(ctx); err == nil {
		t.Fatal("the first Send completed, want it cut off at its third append")
	}
	status, offset := uploadOffset(t, res.Location())
	if status != http.StatusNoContent || offset != 2*chunk {
		t.Fatalf("HEAD after the cut = %d at %d, want 204 at the two appends committed, %d", status, offset, 2*chunk)
	}

	final, err := res.Send(ctx)
	if err != nil {
		t.Fatalf("the resumed Send: %v", err)
	}
	defer func() { _ = final.Body.Close() }()
	var completed client.Upload
	if final.StatusCode != http.StatusOK || json.NewDecoder(final.Body).Decode(&completed) != nil {
		t.Fatalf("the completing response = %s, want 200 with the upload", final.Status)
	}
	id := res.Location()[strings.LastIndex(res.Location(), "/")+1:]
	if completed.ID != id {
		t.Errorf("the completing response names upload %q, want %q", completed.ID, id)
	}

	if status, _ := uploadOffset(t, res.Location()); status != http.StatusNotFound {
		t.Errorf("HEAD of the completed upload = %d, want 404", status)
	}
	if rows, chunks := uploadRows(t, db, "upload", "id", id), uploadRows(t, db, "upload_chunk", "upload_id", id); rows != 0 || chunks != 0 {
		t.Errorf("upload rows %d, chunk rows %d after completion; want none", rows, chunks)
	}
	if got := objectsUnder(t, objects, "upload/"+id+"/"); len(got) != 0 {
		t.Errorf("chunk objects after completion = %v, want none", got)
	}
	if _, err := objects.Stat(ctx, "input/"+sum[:2]+"/"+sum); err != nil {
		t.Errorf("the staged input: %v", err)
	}
	if got := uploadRows(t, db, "completed_upload", "id", id); got != 1 {
		t.Errorf("completed upload rows = %d, want 1", got)
	}

	req := client.CompleteUpload{
		Profile: "standard@1",
		Extract: &docinfer.Extract{Name: "invoice", Prompt: "The total.", Schema: []byte(`{"type":"object"}`)},
	}
	api := s.Client()
	path := "/api/uploads/" + id + "/complete"
	resp := api.Post(t, path, req)
	j := webtest.Decode[client.Job](t, resp, http.StatusAccepted)
	if j.State != client.StateQueued || j.SHA256 != sum || j.Size != int64(len(file)) || j.Name != "scan.pdf" {
		t.Errorf("job = %+v, want it queued over the %d-byte file staged under %s", j, len(file), sum)
	}
	if loc := resp.Header.Get("Location"); loc != "/api/jobs/"+j.ID {
		t.Errorf("Location = %q, want /api/jobs/%s", loc, j.ID)
	}
	if got := webtest.Decode[client.Job](t, api.Get(t, "/api/jobs/"+j.ID), http.StatusOK); got != j {
		t.Errorf("GetJob = %+v, want %+v", got, j)
	}
	if again := webtest.Decode[client.Job](t, api.Post(t, path, req), http.StatusAccepted); again != j {
		t.Errorf("the repeat returned %+v, want the first job %+v", again, j)
	}
	req.Profile = "standard@2"
	if p := api.Post(t, path, req).Problem(t, http.StatusConflict); p.Type != client.ProblemSubmitConflict {
		t.Errorf("a repeat with another profile = %q, want %s", p.Type, client.ProblemSubmitConflict)
	}
	if p := api.Post(t, "/api/uploads/"+uuid.NewV4().String()+"/complete", req).Problem(t, http.StatusNotFound); p.Type != client.ProblemUploadNotFound {
		t.Errorf("an unknown upload = %q, want %s", p.Type, client.ProblemUploadNotFound)
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}
