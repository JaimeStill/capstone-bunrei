package client_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/go-web-sdk/upload"

	"github.com/JaimeStill/capstone-bunrei/client"
)

type received struct {
	mu    sync.Mutex
	files map[string][]byte
	names map[string]string
}

func uploads(t *testing.T, refuse bool) (*httptest.Server, *received) {
	t.Helper()
	got := &received{files: map[string][]byte{}, names: map[string]string{}}
	h := upload.Handler{
		Store:    upload.NewMemoryStore(),
		Transfer: web.NewTransfer(1<<20, 1<<20, time.Second),
		Lifetime: time.Hour,
		Complete: func(w http.ResponseWriter, _ *http.Request, f upload.File, content io.Reader) error {
			if refuse {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, err := io.WriteString(w, `{"type":"urn:example:unreadable","status":422}`)
				return err
			}
			b, err := io.ReadAll(content)
			if err != nil {
				return err
			}
			got.mu.Lock()
			got.files[f.ID], got.names[f.ID] = b, f.Name
			got.mu.Unlock()
			return web.WriteJSON(w, http.StatusOK, client.Upload{ID: f.ID})
		},
		Logger: slog.New(slog.DiscardHandler),
	}
	api := web.NewGroup("/api")
	api.Mount(h.Routes("/uploads"))
	srv := httptest.NewServer(web.NewModule(api))
	t.Cleanup(srv.Close)
	return srv, got
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

func uploadClient(t *testing.T, srv *httptest.Server, cutAt int32) *client.Client {
	t.Helper()
	c, err := client.New(srv.URL+"/api", &http.Client{Transport: &cutOff{base: srv.Client().Transport, at: cutAt}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestClient_UploadSendsTheFileAndReturnsItsID(t *testing.T) {
	srv, got := uploads(t, false)
	file := bytes.Repeat([]byte("a page of a scanned document. "), 500)

	id, err := uploadClient(t, srv, 0).Upload(t.Context(), "scan.pdf", "application/pdf", bytes.NewReader(file), client.UploadOptions{ChunkSize: 1 << 10})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !bytes.Equal(got.files[id], file) || got.names[id] != "scan.pdf" {
		t.Errorf("the server completed upload %q as %q with %d bytes, want scan.pdf with the %d-byte file", id, got.names[id], len(got.files[id]), len(file))
	}
}

func TestClient_UploadResumesACutOffSendWithinItsAttempts(t *testing.T) {
	srv, got := uploads(t, false)
	file := bytes.Repeat([]byte("a page of a scanned document. "), 500)
	opts := client.UploadOptions{Attempts: 2, ChunkSize: 1 << 10}

	id, err := uploadClient(t, srv, 3).Upload(t.Context(), "scan.pdf", "application/pdf", bytes.NewReader(file), opts)
	if err != nil {
		t.Fatalf("Upload with its third append cut off: %v", err)
	}
	if !bytes.Equal(got.files[id], file) {
		t.Errorf("the server completed upload %q with %d bytes, want the %d-byte file", id, len(got.files[id]), len(file))
	}

	opts.Attempts = 1
	if _, err := uploadClient(t, srv, 3).Upload(t.Context(), "scan.pdf", "application/pdf", bytes.NewReader(file), opts); err == nil {
		t.Error("Upload with one attempt and its third append cut off succeeded, want it to give up")
	}
}

func TestClient_UploadReturnsARefusedCompletionsProblem(t *testing.T) {
	srv, _ := uploads(t, true)
	_, err := uploadClient(t, srv, 0).Upload(t.Context(), "scan.pdf", "application/pdf", bytes.NewReader([]byte("x")), client.UploadOptions{})
	p, ok := errors.AsType[*client.Problem](err)
	if !ok || p.Type != "urn:example:unreadable" || p.Status != http.StatusUnprocessableEntity {
		t.Errorf("err = %v, want the 422 problem the completion answered", err)
	}
}
