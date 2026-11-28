package outputs_test

import (
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
	"github.com/JaimeStill/capstone-bunrei/domain/outputs"
)

const (
	rate  = 1 << 20
	grace = time.Second
)

func router(f *fixture) *web.Router {
	transfer := func(limit int64) web.Transfer { return web.NewTransfer(limit, rate, grace) }
	api := web.NewGroup("/api")
	api.Mount(outputs.Routes(f.service, transfer, slog.New(slog.DiscardHandler)))
	r := web.NewRouter()
	r.Mount(web.NewModule(api))
	return r
}

func serve(t *testing.T, f *fixture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(router(f))
	t.Cleanup(srv.Close)
	return srv
}

type deadlines struct {
	*httptest.ResponseRecorder
	set []time.Time
	at  []int
}

func (d *deadlines) SetWriteDeadline(t time.Time) error {
	d.set = append(d.set, t)
	d.at = append(d.at, d.Body.Len())
	return nil
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

func TestRoutes_GetOutputExtendsTheWriteDeadlineAsTheBodyFlows(t *testing.T) {
	f := newFixture(t)
	body := strings.Repeat("x", 200<<10)
	s := f.put(t, body)
	w := &deadlines{ResponseRecorder: httptest.NewRecorder()}

	before := time.Now()
	router(f).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/outputs/"+s, nil))
	after := time.Now()

	if w.Code != http.StatusOK || w.Body.String() != body {
		t.Fatalf("status %d with %d bytes, want 200 with the %d stored", w.Code, w.Body.Len(), len(body))
	}
	if len(w.set) < 4 {
		t.Fatalf("set the write deadline %d times for %d bytes, want once per 64 KiB or less", len(w.set), len(body))
	}
	for i, at := range w.at {
		if i > 0 && at <= w.at[i-1] {
			t.Errorf("deadline %d set after %d bytes, deadline %d after %d; want each after more of the body", i, at, i-1, w.at[i-1])
		}
		next := len(body)
		if i+1 < len(w.at) {
			next = w.at[i+1]
		}
		piece := next - at
		if piece > 64<<10 {
			t.Errorf("deadline %d covers %d bytes, want at most 64 KiB", i, piece)
		}
		allowed := grace + time.Duration(piece)*time.Second/rate
		if d := w.set[i]; d.Before(before.Add(allowed)) || d.After(after.Add(allowed)) {
			t.Errorf("deadline %d = %s past the request, want the grace plus %d bytes at the rate, %s", i, d.Sub(before), piece, allowed)
		}
	}
}
