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
	"uuid"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-core/process/processtest"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-web-sdk/upload"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/submissions"
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

func TestSweep_ReclaimsAnExpiredUploadAndItsAgedObjects(t *testing.T) {
	run := fmt.Sprintf("sweep-%d", time.Now().UnixNano())
	prefix := run + "/upload/"
	expired, fresh := run+"-expired", run+"-open"
	t.Setenv("BUNREI_OBJECTS_UPLOAD_PREFIX", prefix)
	t.Setenv("BUNREI_UPLOADS_LIFETIME", "2s")
	t.Setenv("BUNREI_SWEEP_INTERVAL", "500ms")
	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Schema, n.Sweeper)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	start(t, "database", sys.Get(n.Database))
	start(t, "storage", sys.Get(n.Storage))
	start(t, "schema", sys.Get(n.Schema))

	ctx := context.Background()
	store := sys.Get(n.Uploads)
	objects := sys.Get(n.Storage)
	db := integration.Database(t)
	open := func(id string, lifetime time.Duration) {
		if err := store.Create(ctx, id, "application/pdf", uploadDigest, "f.pdf", -1, time.Now().Add(lifetime)); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
		if _, ok, err := store.Append(ctx, id, 0, []byte("abc"), nil); err != nil || !ok {
			t.Fatalf("Append to %s = %v, %v", id, ok, err)
		}
	}
	open(expired, time.Second)
	time.Sleep(4500 * time.Millisecond)
	open(fresh, time.Hour)

	sweeper := sys.Get(n.Sweeper)
	start(t, "sweeper", sweeper)
	processtest.WaitFor(t, "the expired upload's reclaim", func() bool {
		return uploadRows(t, db, "upload", "id", expired) == 0 &&
			uploadRows(t, db, "upload_chunk", "upload_id", expired) == 0 &&
			len(objectsUnder(t, objects, prefix+expired+"/")) == 0
	})
	shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sweeper.Shutdown(shutdown); err != nil {
		t.Fatalf("Shutdown the sweeper: %v", err)
	}

	if got := uploadRows(t, db, "upload_chunk", "upload_id", fresh); got != 1 {
		t.Errorf("the open upload's chunk rows = %d, want its one kept", got)
	}
	if got := objectsUnder(t, objects, prefix+fresh+"/"); len(got) != 1 {
		t.Errorf("the fresh upload's objects = %v, want its one kept, younger than twice the lifetime", got)
	}
}

func TestSweep_ReclaimsAnAgedCompletedUpload(t *testing.T) {
	t.Setenv("BUNREI_UPLOADS_LIFETIME", "2s")
	t.Setenv("BUNREI_SWEEP_INTERVAL", "500ms")
	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Schema, n.Sweeper)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	start(t, "database", sys.Get(n.Database))
	start(t, "storage", sys.Get(n.Storage))
	start(t, "schema", sys.Get(n.Schema))

	ctx := context.Background()
	submit := sys.Get(n.Submissions)
	db := integration.Database(t)
	complete := func(body string) string {
		raw := sha256.Sum256([]byte(body))
		f := upload.File{ID: uuid.NewV4().String(), MediaType: "application/pdf", Digest: hex.EncodeToString(raw[:]), Name: "f.pdf", Length: int64(len(body))}
		if err := submit.Complete(ctx, f, strings.NewReader(body)); err != nil {
			t.Fatalf("Complete %s: %v", f.ID, err)
		}
		return f.ID
	}
	aged := complete("a document completed a while ago")
	time.Sleep(2500 * time.Millisecond)
	fresh := complete("a document just completed")
	if _, err := submit.CompleteUpload(ctx, client.CompleteUpload{Upload: aged, Profile: "standard@1"}); !errors.Is(err, submissions.ErrUploadNotFound) {
		t.Errorf("CompleteUpload of the aged upload before its reclaim = %v, want ErrUploadNotFound", err)
	}

	sweeper := sys.Get(n.Sweeper)
	start(t, "sweeper", sweeper)
	processtest.WaitFor(t, "the aged completed upload's reclaim", func() bool {
		return uploadRows(t, db, "completed_upload", "id", aged) == 0
	})
	shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sweeper.Shutdown(shutdown); err != nil {
		t.Fatalf("Shutdown the sweeper: %v", err)
	}
	if got := uploadRows(t, db, "completed_upload", "id", fresh); got != 1 {
		t.Errorf("the fresh completed upload's rows = %d, want its one kept", got)
	}
}
