//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/integration"
)

func TestOutputs_GetOutputStreamsContentAndRefusesAnInvalidSHA256(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := s.Client()

	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Content)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ctx := context.Background()
	objects := sys.Get(n.Storage)
	if err := objects.Start(ctx); err != nil {
		t.Fatalf("start the storage: %v", err)
	}
	t.Cleanup(func() { _ = objects.Shutdown(context.Background()) })

	body := "an output put under content/ at " + time.Now().Format(time.RFC3339Nano)
	raw := sha256.Sum256([]byte(body))
	sum := hex.EncodeToString(raw[:])

	if err := sys.Get(n.Content).Put(ctx, sum, strings.NewReader(body)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if obj, err := objects.Stat(ctx, "content/"+sum[:2]+"/"+sum); err != nil || obj.Size != int64(len(body)) {
		t.Fatalf("Stat = %+v, %v; want the object under content/", obj, err)
	}
	resp := c.Get(t, "/api/outputs/"+sum).Expect(t, http.StatusOK)
	if string(resp.Body) != body || resp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("GetOutput = %q as %q, want %q as application/octet-stream", resp.Body, resp.Header.Get("Content-Type"), body)
	}

	p := c.Get(t, "/api/outputs/"+strings.ToUpper(sum)).Problem(t, http.StatusBadRequest)
	if p.Type != client.ProblemInvalidRequest {
		t.Errorf("problem type = %q, want %s", p.Type, client.ProblemInvalidRequest)
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}
