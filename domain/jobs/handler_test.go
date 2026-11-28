package jobs_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standards-lab/go-messaging/memory"
	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
)

func serve(t *testing.T, s *jobs.Service) *httptest.Server {
	t.Helper()
	api := web.NewGroup("/api")
	api.Mount(jobs.Routes(s, slog.New(slog.DiscardHandler)))
	router := web.NewRouter()
	router.Mount(web.NewModule(api))
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, method, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
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

func getJob(t *testing.T, srv *httptest.Server, id string) client.Job {
	t.Helper()
	resp := call(t, http.MethodGet, srv.URL+"/api/jobs/"+id)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GetJob status = %d, want 200; body %s", resp.StatusCode, b)
	}
	var j client.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		t.Fatalf("decode the job: %v", err)
	}
	return j
}

func TestRoutes_GetJobThenCancelJob(t *testing.T) {
	s := newService(memory.New())
	srv := serve(t, s)
	id := jobID()
	mustAppend(t, s, id, queued(t))

	if j := getJob(t, srv, id); j.ID != id || j.State != client.StateQueued || j.Sequence != 1 {
		t.Errorf("GetJob = %+v, want the job queued at sequence 1", j)
	}

	resp := call(t, http.MethodPost, srv.URL+"/api/jobs/"+strings.ToUpper(id)+"/cancel")
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("CancelJob status = %d, want 202; body %s", resp.StatusCode, b)
	}
	if loc := resp.Header.Get("Location"); loc != "/api/jobs/"+id {
		t.Errorf("Location = %q, want /api/jobs/%s", loc, id)
	}
	if b, _ := io.ReadAll(resp.Body); len(b) != 0 {
		t.Errorf("CancelJob body = %q, want none", b)
	}
	if j := getJob(t, srv, id); j.State != client.StateCancelled || j.Sequence != 3 {
		t.Errorf("GetJob after CancelJob = %+v, want cancelled at sequence 3", j)
	}

	problem(t, call(t, http.MethodPost, srv.URL+"/api/jobs/"+id+"/cancel"), http.StatusConflict, client.ProblemJobEnded)
}

func TestRoutes_RefusalsAreBunreiProblems(t *testing.T) {
	srv := serve(t, newService(memory.New()))
	unknown := jobID()
	for _, tc := range []struct {
		name, method, path string
		status             int
		typ                string
	}{
		{"GetJob of an unknown job", http.MethodGet, "/api/jobs/" + unknown, http.StatusNotFound, client.ProblemJobNotFound},
		{"GetJob of an invalid ID", http.MethodGet, "/api/jobs/not-a-uuid", http.StatusBadRequest, client.ProblemInvalidRequest},
		{"CancelJob of an unknown job", http.MethodPost, "/api/jobs/" + unknown + "/cancel", http.StatusNotFound, client.ProblemJobNotFound},
		{"CancelJob of an invalid ID", http.MethodPost, "/api/jobs/not-a-uuid/cancel", http.StatusBadRequest, client.ProblemInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problem(t, call(t, tc.method, srv.URL+tc.path), tc.status, tc.typ)
		})
	}
}
