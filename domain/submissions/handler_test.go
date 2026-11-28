package submissions_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/submissions"
)

func serve(t *testing.T, f *fixture) *httptest.Server {
	t.Helper()
	transfer := func(limit int64) web.Transfer { return web.NewTransfer(limit, 1<<20, time.Second) }
	api := web.NewGroup("/api")
	api.Mount(submissions.Routes(f.service, transfer, slog.New(slog.DiscardHandler)))
	router := web.NewRouter()
	router.Mount(web.NewModule(api))
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, body []byte) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func problem(t *testing.T, resp *http.Response, status int, typ string) {
	t.Helper()
	if resp.StatusCode != status {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, status, b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != web.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %s", ct, web.ProblemMediaType)
	}
	var p web.Problem
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode the problem: %v", err)
	}
	if p.Type != typ || p.Status != status || p.Detail == "" {
		t.Errorf("problem = %+v, want type %s, status %d, and a detail", p, typ, status)
	}
}

func TestRoutes_SubmitAnswersTheQueuedJob(t *testing.T) {
	f := newFixture(t)
	f.claims()
	srv := serve(t, f)

	resp := post(t, srv, must(json.Marshal(request())))
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 202; body %s", resp.StatusCode, b)
	}
	var j client.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		t.Fatalf("decode the job: %v", err)
	}
	if j.State != client.StateQueued || j.ID == "" {
		t.Errorf("job = %+v, want it queued", j)
	}
	if loc := resp.Header.Get("Location"); loc != "/api/jobs/"+j.ID {
		t.Errorf("Location = %q, want /api/jobs/%s", loc, j.ID)
	}
}

func TestRoutes_RefusalsAreBunreiProblems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   func() []byte
		status int
		typ    string
	}{
		{"a body that is not JSON", func() []byte { return []byte("{") }, http.StatusBadRequest, client.ProblemInvalidRequest},
		{"an unknown member", func() []byte { return []byte(`{"id":"a","extraction":{}}`) }, http.StatusBadRequest, client.ProblemInvalidRequest},
		{"content that is not base64", func() []byte { return []byte(`{"id":"a","content":"!"}`) }, http.StatusBadRequest, client.ProblemInvalidRequest},
		{"an empty profile", func() []byte {
			r := request()
			r.Profile = ""
			return must(json.Marshal(r))
		}, http.StatusBadRequest, client.ProblemInvalidRequest},
		{"content that decodes past the limit", func() []byte {
			r := request()
			r.Content = make([]byte, limit+1)
			return must(json.Marshal(r))
		}, http.StatusRequestEntityTooLarge, client.ProblemContentTooLarge},
		{"a body past its encoded bound", func() []byte {
			r := request()
			r.Name = strings.Repeat("n", submissions.ExtractionLimit+128<<10)
			return must(json.Marshal(r))
		}, http.StatusRequestEntityTooLarge, client.ProblemContentTooLarge},
		{"extracts past the cap", func() []byte {
			r := request()
			r.Document = extract(strings.Repeat("x", submissions.ExtractionLimit))
			return must(json.Marshal(r))
		}, http.StatusRequestEntityTooLarge, client.ProblemExtractionTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			problem(t, post(t, serve(t, f), tc.body()), tc.status, tc.typ)
		})
	}
}

func TestRoutes_ADifferentRequestUnderAnIDIsAConflictProblem(t *testing.T) {
	f := newFixture(t)
	f.claims()
	srv := serve(t, f)
	req := request()
	if resp := post(t, srv, must(json.Marshal(req))); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("the first Submit = %d, want 202", resp.StatusCode)
	}
	f.held(f.record(t))
	req.Name = "other.json"

	problem(t, post(t, srv, must(json.Marshal(req))), http.StatusConflict, client.ProblemSubmitConflict)
}
