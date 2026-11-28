//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/standards-lab/docinfer"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/integration"
)

func protocol(t *testing.T, s *integration.Service, transport http.RoundTripper) *client.Client {
	t.Helper()
	c, err := client.New(s.URL()+"/api", &http.Client{Transport: transport})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func wantProblem(t *testing.T, what string, err error, sentinel *client.Problem) {
	t.Helper()
	p, ok := errors.AsType[*client.Problem](err)
	if !ok || !errors.Is(err, sentinel) || p.Status != sentinel.Status {
		t.Errorf("%s = %v, want the %d problem %s", what, err, sentinel.Status, sentinel.Type)
	}
}

func TestClient_SubmitGetJobAndCancelJob(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := protocol(t, s, http.DefaultTransport)
	ctx := context.Background()
	req := submitRequest()

	j, err := c.Submit(ctx, req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	raw := sha256.Sum256(req.Content)
	if j.State != client.StateQueued || j.SHA256 != hex.EncodeToString(raw[:]) || j.Size != int64(len(req.Content)) || j.Name != req.Name {
		t.Errorf("Submit = %+v, want the job queued over the request's content", j)
	}
	if again, err := c.Submit(ctx, req); err != nil || again != j {
		t.Errorf("the identical retry = %+v, %v; want the first job %+v", again, err, j)
	}
	if got, err := c.GetJob(ctx, j.ID); err != nil || got != j {
		t.Errorf("GetJob = %+v, %v; want %+v", got, err, j)
	}

	conflict := req
	conflict.Name = "renamed.json"
	_, err = c.Submit(ctx, conflict)
	wantProblem(t, "a retry with another name", err, client.ErrSubmitConflict)

	big := submitRequest()
	big.Content = make([]byte, 4<<20+1)
	_, err = c.Submit(ctx, big)
	wantProblem(t, "a document past the limit", err, client.ErrContentTooLarge)

	extracting := submitRequest()
	extracting.Document = &docinfer.Extract{Name: "all", Prompt: strings.Repeat("p", 300<<10), Schema: []byte(`{"type":"object"}`)}
	_, err = c.Submit(ctx, extracting)
	wantProblem(t, "extracts past 256 KiB", err, client.ErrExtractionTooLarge)

	missing := submitRequest()
	missing.ID = ""
	_, err = c.Submit(ctx, missing)
	wantProblem(t, "a Submit without its ID", err, client.ErrInvalidRequest)

	unprofiled := submitRequest()
	unprofiled.Profile = ""
	_, err = c.Submit(ctx, unprofiled)
	wantProblem(t, "a Submit without its profile", err, client.ErrInvalidRequest)

	_, err = c.GetJob(ctx, uuid.NewV7().String())
	wantProblem(t, "GetJob of an unknown job", err, client.ErrJobNotFound)
	_, err = c.GetJob(ctx, "not-a-uuid")
	wantProblem(t, "GetJob of a malformed ID", err, client.ErrInvalidRequest)

	if err := c.CancelJob(ctx, j.ID); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	if got, err := c.GetJob(ctx, j.ID); err != nil || got.State != client.StateCancelled {
		t.Errorf("GetJob after CancelJob = %+v, %v; want it cancelled", got, err)
	}
	wantProblem(t, "a second CancelJob", c.CancelJob(ctx, j.ID), client.ErrJobEnded)

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}

func TestClient_GetOutputStreamsStoredContent(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := protocol(t, s, http.DefaultTransport)

	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Content)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ctx := context.Background()
	start(t, "storage", sys.Get(n.Storage))

	body := "an output the client reads back, put at " + time.Now().Format(time.RFC3339Nano)
	raw := sha256.Sum256([]byte(body))
	sum := hex.EncodeToString(raw[:])

	_, err = c.GetOutput(ctx, sum)
	wantProblem(t, "GetOutput of an absent output", err, client.ErrOutputNotFound)

	if err := sys.Get(n.Content).Put(ctx, sum, strings.NewReader(body)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	out, err := c.GetOutput(ctx, sum)
	if err != nil {
		t.Fatalf("GetOutput: %v", err)
	}
	got, err := io.ReadAll(out)
	_ = out.Close()
	if err != nil || string(got) != body {
		t.Errorf("GetOutput streamed %q, %v; want %q", got, err, body)
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}

func TestClient_UploadResumedThenCompleteUpload(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	ctx := context.Background()

	const chunk = 16 << 10
	file := bytes.Repeat([]byte("a scanned page the client uploads. "), 3000)
	cut := &cutOff{base: http.DefaultTransport, at: 3}
	c := protocol(t, s, cut)

	id, err := c.Upload(ctx, "scan.pdf", "application/pdf", bytes.NewReader(file), client.UploadOptions{Attempts: 2, ChunkSize: chunk})
	if err != nil {
		t.Fatalf("Upload with its third append cut off: %v", err)
	}
	chunks := (len(file) + chunk - 1) / chunk
	if got := int(cut.patches.Load()); got != chunks+1 {
		t.Errorf("Upload sent %d appends, want the %d chunks and the cut-off one resent", got, chunks)
	}

	req := client.CompleteUpload{
		Upload:  id,
		Profile: "standard@1",
		Extract: &docinfer.Extract{Name: "invoice", Prompt: "The total.", Schema: []byte(`{"type":"object"}`)},
	}
	j, err := c.CompleteUpload(ctx, req)
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	raw := sha256.Sum256(file)
	if j.State != client.StateQueued || j.SHA256 != hex.EncodeToString(raw[:]) || j.Size != int64(len(file)) || j.Name != "scan.pdf" {
		t.Errorf("CompleteUpload = %+v, want the job queued over the uploaded file", j)
	}
	if again, err := c.CompleteUpload(ctx, req); err != nil || again != j {
		t.Errorf("the repeat = %+v, %v; want the first job %+v", again, err, j)
	}
	if got, err := c.GetJob(ctx, j.ID); err != nil || got != j {
		t.Errorf("GetJob = %+v, %v; want %+v", got, err, j)
	}

	unprofiled := req
	unprofiled.Profile = ""
	_, err = c.CompleteUpload(ctx, unprofiled)
	wantProblem(t, "a CompleteUpload without its profile", err, client.ErrInvalidRequest)

	extracting := req
	prompt := strings.Repeat("p", 90<<10)
	extracting.Extract = &docinfer.Extract{Name: "invoice", Prompt: prompt, Schema: []byte(`{"type":"object"}`)}
	extracting.Document = &docinfer.Extract{Name: "all", Prompt: prompt, Schema: []byte(`{"type":"object"}`)}
	extracting.Timeline = &docinfer.Extract{Name: "scenes", Prompt: prompt, Schema: []byte(`{"type":"object"}`)}
	_, err = c.CompleteUpload(ctx, extracting)
	wantProblem(t, "a CompleteUpload whose three extracts pass 256 KiB together", err, client.ErrExtractionTooLarge)

	req.Profile = "standard@2"
	_, err = c.CompleteUpload(ctx, req)
	wantProblem(t, "a repeat with another profile", err, client.ErrSubmitConflict)
	req.Upload = uuid.NewV4().String()
	_, err = c.CompleteUpload(ctx, req)
	wantProblem(t, "CompleteUpload of an unknown upload", err, client.ErrUploadNotFound)

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}

func TestClient_ConcurrentSubmitsOfOneIDYieldOneJob(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := protocol(t, s, http.DefaultTransport)
	ctx := context.Background()
	req := submitRequest()

	const n = 8
	var wg sync.WaitGroup
	results := make([]client.Job, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() { results[i], errs[i] = c.Submit(ctx, req) })
	}
	wg.Wait()

	ids := map[string]bool{}
	for i := range n {
		if errs[i] != nil || results[i].State != client.StateQueued {
			t.Errorf("Submit %d = %+v, %v; want the job queued", i, results[i], errs[i])
		}
		ids[results[i].ID] = true
	}
	if len(ids) != 1 {
		t.Fatalf("the Submits returned jobs %v, want one", ids)
	}
	if got, err := c.GetJob(ctx, results[0].ID); err != nil || got != results[0] {
		t.Errorf("GetJob = %+v, %v; want %+v", got, err, results[0])
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}

func TestClient_ListProfilesIsNotServedYet(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := protocol(t, s, http.DefaultTransport)

	_, err := c.ListProfiles(context.Background())
	p, ok := errors.AsType[*client.Problem](err)
	if !ok || p.Status != http.StatusNotFound {
		t.Errorf("ListProfiles = %v, want a 404 problem until jobs serves GET /api/profiles", err)
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}
