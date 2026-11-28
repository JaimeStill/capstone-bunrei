//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-web-sdk/upload"
	"github.com/standards-lab/go-web-sdk/upload/uploadtest"

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
