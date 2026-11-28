package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/JaimeStill/capstone-bunrei/client"
)

type recorded struct {
	Method      string
	Path        string
	ContentType string
	Body        string
}

type fake struct {
	status      int
	contentType string
	body        string
	seen        []recorded
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.seen = append(f.seen, recorded{Method: r.Method, Path: r.URL.EscapedPath(), ContentType: r.Header.Get("Content-Type"), Body: string(b)})
	if f.contentType != "" {
		w.Header().Set("Content-Type", f.contentType)
	}
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.body)
}

func serve(t *testing.T, f *fake) *client.Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL+"/api/", srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func queuedJob() client.Job {
	return client.Job{
		ID: "0192f0c4-8a3b-7c1d-9e2f-0123456789ab", Profile: "standard@1", Name: "invoice.pdf",
		Size: 12, SHA256: "ab12", State: client.StateQueued, Sequence: 1,
	}
}

func wireOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(b)
}

func TestNew_RefusesABaseOrClientItCannotUse(t *testing.T) {
	for _, tc := range []struct {
		base string
		hc   *http.Client
	}{
		{"bunrei:8080/api", http.DefaultClient},
		{"/api", http.DefaultClient},
		{"ftp://bunrei/api", http.DefaultClient},
		{"http://bunrei/%zz", http.DefaultClient},
		{"http://bunrei/api", nil},
	} {
		if _, err := client.New(tc.base, tc.hc); err == nil {
			t.Errorf("New(%q, %v) succeeded, want an error", tc.base, tc.hc)
		}
	}
}

func TestClient_MethodsSendTheirRequestAndDecodeTheirResponse(t *testing.T) {
	submit := client.Submit{
		ID: "retry-1", Profile: "standard@1", Name: "invoice.pdf", MediaType: "application/pdf",
		Content: []byte("%PDF"), Extract: extract(), Document: extract(), Timeline: extract(),
	}
	complete := client.CompleteUpload{Upload: "u/1", Profile: "standard@1", Document: extract()}
	profiles := []client.Profile{{Name: "standard@1", Resolved: profile()}}
	cases := []struct {
		name   string
		status int
		body   string
		call   func(context.Context, *client.Client) (any, error)
		want   recorded
		result any
	}{
		{
			name: "Submit", status: http.StatusAccepted, body: wireOf(t, queuedJob()),
			call:   func(ctx context.Context, c *client.Client) (any, error) { return c.Submit(ctx, submit) },
			want:   recorded{Method: "POST", Path: "/api/jobs", ContentType: "application/json", Body: wireOf(t, submit)},
			result: queuedJob(),
		},
		{
			name: "CompleteUpload", status: http.StatusAccepted, body: wireOf(t, queuedJob()),
			call:   func(ctx context.Context, c *client.Client) (any, error) { return c.CompleteUpload(ctx, complete) },
			want:   recorded{Method: "POST", Path: "/api/uploads/u%2F1/complete", ContentType: "application/json", Body: wireOf(t, complete)},
			result: queuedJob(),
		},
		{
			name: "CancelJob", status: http.StatusAccepted,
			call: func(ctx context.Context, c *client.Client) (any, error) { return nil, c.CancelJob(ctx, "j1") },
			want: recorded{Method: "POST", Path: "/api/jobs/j1/cancel"},
		},
		{
			name: "GetJob", status: http.StatusOK, body: wireOf(t, queuedJob()),
			call: func(ctx context.Context, c *client.Client) (any, error) { return c.GetJob(ctx, "j1") },
			want: recorded{Method: "GET", Path: "/api/jobs/j1"}, result: queuedJob(),
		},
		{
			name: "ListProfiles", status: http.StatusOK, body: wireOf(t, profiles),
			call: func(ctx context.Context, c *client.Client) (any, error) { return c.ListProfiles(ctx) },
			want: recorded{Method: "GET", Path: "/api/profiles"}, result: profiles,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake{status: tc.status, contentType: "application/json", body: tc.body}
			got, err := tc.call(t.Context(), serve(t, f))
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(f.seen) != 1 || f.seen[0] != tc.want {
				t.Errorf("requests = %+v, want one, %+v", f.seen, tc.want)
			}
			if !reflect.DeepEqual(got, tc.result) {
				t.Errorf("result = %+v, want %+v", got, tc.result)
			}
		})
	}
}

func TestClient_GetOutputStreamsTheContent(t *testing.T) {
	f := &fake{status: http.StatusOK, contentType: "application/octet-stream", body: "the output's bytes"}
	body, err := serve(t, f).GetOutput(t.Context(), "ab12")
	if err != nil {
		t.Fatalf("GetOutput: %v", err)
	}
	defer func() { _ = body.Close() }()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != f.body {
		t.Errorf("content = %q, %v; want %q", got, err, f.body)
	}
	if want := (recorded{Method: "GET", Path: "/api/outputs/ab12"}); len(f.seen) != 1 || f.seen[0] != want {
		t.Errorf("requests = %+v, want one, %+v", f.seen, want)
	}
}

func TestClient_ARefusalIsItsProblem(t *testing.T) {
	problem := `{"type":"` + client.ProblemJobNotFound + `","title":"Not Found","status":404,"detail":"job j1 has no events","instance":"/api/jobs/j1"}`
	calls := map[string]func(context.Context, *client.Client) error{
		"Submit": func(ctx context.Context, c *client.Client) error {
			_, err := c.Submit(ctx, client.Submit{})
			return err
		},
		"CompleteUpload": func(ctx context.Context, c *client.Client) error {
			_, err := c.CompleteUpload(ctx, client.CompleteUpload{Upload: "u1"})
			return err
		},
		"CancelJob": func(ctx context.Context, c *client.Client) error { return c.CancelJob(ctx, "j1") },
		"GetJob": func(ctx context.Context, c *client.Client) error {
			_, err := c.GetJob(ctx, "j1")
			return err
		},
		"GetOutput": func(ctx context.Context, c *client.Client) error {
			_, err := c.GetOutput(ctx, "ab12")
			return err
		},
		"ListProfiles": func(ctx context.Context, c *client.Client) error {
			_, err := c.ListProfiles(ctx)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			f := &fake{status: http.StatusNotFound, contentType: "application/problem+json", body: problem}
			err := call(t.Context(), serve(t, f))
			p, ok := errors.AsType[*client.Problem](err)
			if !ok {
				t.Fatalf("err = %v, want a *client.Problem", err)
			}
			want := client.Problem{
				Type: client.ProblemJobNotFound, Title: "Not Found", Status: http.StatusNotFound,
				Detail: "job j1 has no events", Instance: "/api/jobs/j1",
			}
			if *p != want {
				t.Errorf("problem = %+v, want %+v", *p, want)
			}
			if !errors.Is(err, client.ErrJobNotFound) || errors.Is(err, client.ErrOutputNotFound) {
				t.Errorf("errors.Is matches ErrJobNotFound %t and ErrOutputNotFound %t, want only the first",
					errors.Is(err, client.ErrJobNotFound), errors.Is(err, client.ErrOutputNotFound))
			}
			if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), want.Detail) {
				t.Errorf("Error() = %q, want the status and the detail", err.Error())
			}
		})
	}
}

func TestClient_AProblemWithoutTypeOrStatusTakesTheDefaults(t *testing.T) {
	f := &fake{status: http.StatusTooManyRequests, contentType: "application/problem+json; charset=utf-8", body: `{"title":"slow down"}`}
	_, err := serve(t, f).GetJob(t.Context(), "j1")
	p, ok := errors.AsType[*client.Problem](err)
	if !ok || p.Type != "about:blank" || p.Status != http.StatusTooManyRequests || p.Title != "slow down" {
		t.Errorf("err = %#v, want an about:blank problem, 429, titled slow down", err)
	}
}

func TestClient_ANonProblemBodyIsABlankProblem(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		detail      string
	}{
		{"text", "text/html", "<html>upstream down</html>\n", "<html>upstream down</html>"},
		{"malformed problem", "application/problem+json", "{not json", "{not json"},
		{"long", "text/plain", strings.Repeat("x", 2000), strings.Repeat("x", 512)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake{status: http.StatusBadGateway, contentType: tc.contentType, body: tc.body}
			_, err := serve(t, f).Submit(t.Context(), client.Submit{})
			p, ok := errors.AsType[*client.Problem](err)
			if !ok {
				t.Fatalf("err = %v, want a *client.Problem", err)
			}
			want := client.Problem{Type: "about:blank", Title: "Bad Gateway", Status: http.StatusBadGateway, Detail: tc.detail}
			if *p != want {
				t.Errorf("problem = %+v, want %+v", *p, want)
			}
		})
	}
}

func TestClient_ASuccessBodyThatIsNotJSONIsAnError(t *testing.T) {
	f := &fake{status: http.StatusOK, contentType: "application/json", body: "{"}
	_, err := serve(t, f).GetJob(t.Context(), "j1")
	if _, ok := errors.AsType[*client.Problem](err); err == nil || ok {
		t.Errorf("err = %v, want a decoding error, not a problem", err)
	}
}

func TestProblem_SentinelsMatchByType(t *testing.T) {
	sentinels := map[string]*client.Problem{
		client.ProblemInvalidRequest:     client.ErrInvalidRequest,
		client.ProblemContentTooLarge:    client.ErrContentTooLarge,
		client.ProblemExtractionTooLarge: client.ErrExtractionTooLarge,
		client.ProblemSubmitConflict:     client.ErrSubmitConflict,
		client.ProblemJobNotFound:        client.ErrJobNotFound,
		client.ProblemJobEnded:           client.ErrJobEnded,
		client.ProblemOutputNotFound:     client.ErrOutputNotFound,
		client.ProblemUploadNotFound:     client.ErrUploadNotFound,
	}
	for typ, sentinel := range sentinels {
		err := error(&client.Problem{Type: typ, Status: 499, Detail: "d"})
		for other, s := range sentinels {
			if got := errors.Is(err, s); got != (other == typ) {
				t.Errorf("errors.Is(%s, the %s sentinel) = %t", typ, other, got)
			}
		}
		if sentinel.Type != typ || sentinel.Status == 0 {
			t.Errorf("sentinel = %+v, want type %s with its status", sentinel, typ)
		}
	}
}
