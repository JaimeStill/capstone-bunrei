//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-core/process/processtest"
	"github.com/standards-lab/go-storage"

	"github.com/JaimeStill/capstone-bunrei/integration"
)

func start(t *testing.T, name string, p lifecycle.Starter) {
	t.Helper()
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("start the %s: %v", name, err)
	}
	if s, ok := p.(lifecycle.Stopper); ok {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.Shutdown(ctx)
		})
	}
}

func TestSweep_ExpiresAnAgedObjectAndKeepsAFreshOne(t *testing.T) {
	run := fmt.Sprintf("sweep-%d/", time.Now().UnixNano())
	t.Setenv("BUNREI_OBJECTS_CONTENT_PREFIX", run+"content/")
	t.Setenv("BUNREI_OBJECTS_INPUT_PREFIX", run+"input/")
	t.Setenv("BUNREI_OBJECTS_RENEW_AGE", "1s")
	t.Setenv("BUNREI_SWEEP_CONTENT_AGE", "3s")
	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Schema, n.Sweeper, n.Content)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	start(t, "database", sys.Get(n.Database))
	start(t, "storage", sys.Get(n.Storage))
	start(t, "schema", sys.Get(n.Schema))

	ctx := context.Background()
	content := sys.Get(n.Content)
	key := func(body string) string {
		raw := sha256.Sum256([]byte(body))
		sum := hex.EncodeToString(raw[:])
		if err := content.Put(ctx, sum, strings.NewReader(body)); err != nil {
			t.Fatalf("Put: %v", err)
		}
		return run + "content/" + sum[:2] + "/" + sum
	}
	aged := key("a document no job has put since")
	time.Sleep(4 * time.Second)
	fresh := key("a document a job has just put")

	sweeper := sys.Get(n.Sweeper)
	start(t, "sweeper", sweeper)
	objects := sys.Get(n.Storage)
	processtest.WaitFor(t, "the aged object's expiry", func() bool {
		_, err := objects.Stat(ctx, aged)
		return errors.Is(err, storage.ErrNotFound)
	})
	shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sweeper.Shutdown(shutdown); err != nil {
		t.Fatalf("Shutdown the sweeper: %v", err)
	}

	if _, err := objects.Stat(ctx, fresh); err != nil {
		t.Errorf("Stat %s = %v, want the fresh object kept", fresh, err)
	}
}
