package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	docstorage "github.com/standards-lab/docproc/storage"
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"

	"github.com/JaimeStill/capstone-bunrei/internal/app"
	"github.com/JaimeStill/capstone-bunrei/internal/config/configtest"
)

func overFake(t *testing.T) (*app.App, *storagetest.Fake) {
	t.Helper()
	cfg := configtest.Config(t)
	a := app.New(cfg, io.Discard)
	fake := storagetest.NewFake(storagetest.Options{NoContainer: true})
	a.Graph().Replace(a.Nodes().Storage, func(*graph.Scope) (*storage.Store, error) {
		return storage.New(fake, cfg.Storage), nil
	})
	return a, fake
}

func startStorage(t *testing.T, sys *graph.System, n app.Nodes) {
	t.Helper()
	store := sys.Get(n.Storage)
	if err := store.Start(context.Background()); err != nil {
		t.Fatalf("start the storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Shutdown(context.Background()) })
}

func digest(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func TestStorage_StartEnsuresTheContainer(t *testing.T) {
	a, fake := overFake(t)
	sys := buildAsRun(t, a)
	startStorage(t, sys, a.Nodes())

	if !fake.HasContainer() {
		t.Error("the storage started without ensuring its container")
	}
}

func TestStorage_StoresKeyUnderTheirPrefixes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		node   func(app.Nodes) *graph.Node[*docstorage.Content]
		prefix string
	}{
		{"content", func(n app.Nodes) *graph.Node[*docstorage.Content] { return n.Content }, "content/"},
		{"input", func(n app.Nodes) *graph.Node[*docstorage.Content] { return n.Input }, "input/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, fake := overFake(t)
			n := a.Nodes()
			sys := buildAsRun(t, a)
			startStorage(t, sys, n)
			store := sys.Get(tc.node(n))
			ctx := context.Background()

			const body = "a staged document"
			sum := digest(body)
			if err := store.Put(ctx, sum, strings.NewReader(body)); err != nil {
				t.Fatalf("Put: %v", err)
			}
			key := tc.prefix + sum[:2] + "/" + sum
			if _, err := fake.Stat(ctx, key); err != nil {
				t.Errorf("Stat %s: %v, want the object at its prefix's key", key, err)
			}

			r, err := store.Open(ctx, sum)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer func() { _ = r.Close() }()
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if string(got) != body {
				t.Errorf("Open read %q, want %q", got, body)
			}

			if _, err := store.Open(ctx, digest("never put")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Open of an absent sha256 = %v, want fs.ErrNotExist", err)
			}
		})
	}
}
