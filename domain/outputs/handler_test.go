package outputs_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/outputs"
)

func serve(t *testing.T, f *fixture) *httptest.Server {
	t.Helper()
	api := web.NewGroup("/api")
	api.Mount(outputs.Routes(f.service, slog.New(slog.DiscardHandler)))
	router := web.NewRouter()
	router.Mount(web.NewModule(api))
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, method, url string, header ...string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
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

func TestRoutes_GetOutputStreamsTheContent(t *testing.T) {
	f := newFixture(t)
	s := f.put(t, "an output's bytes")
	srv := serve(t, f)

	resp := call(t, http.MethodGet, srv.URL+"/api/outputs/"+s)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body %s", resp.StatusCode, b)
	}
	if got, _ := io.ReadAll(resp.Body); string(got) != "an output's bytes" {
		t.Errorf("body = %q, want the stored bytes", got)
	}
	if ct, etag := resp.Header.Get("Content-Type"), resp.Header.Get("ETag"); ct != "application/octet-stream" || etag != `"`+s+`"` {
		t.Errorf("Content-Type %q, ETag %q; want application/octet-stream and the quoted SHA-256", ct, etag)
	}

	if resp := call(t, http.MethodGet, srv.URL+"/api/outputs/"+s, "If-None-Match", `"`+s+`"`); resp.StatusCode != http.StatusNotModified {
		t.Errorf("a matching If-None-Match = %d, want 304", resp.StatusCode)
	}
}

func TestRoutes_RefusalsAreBunreiProblems(t *testing.T) {
	f := newFixture(t)
	srv := serve(t, f)
	problem(t, call(t, http.MethodGet, srv.URL+"/api/outputs/"+sum("never stored")), http.StatusNotFound, client.ProblemOutputNotFound)
	problem(t, call(t, http.MethodGet, srv.URL+"/api/outputs/ab12"), http.StatusBadRequest, client.ProblemInvalidRequest)
	if resp := call(t, http.MethodHead, srv.URL+"/api/outputs/"+sum("never stored")); resp.StatusCode != http.StatusNotFound {
		t.Errorf("HEAD of an absent output = %d, want 404", resp.StatusCode)
	}
}
