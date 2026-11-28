//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	docstorage "github.com/standards-lab/docproc/storage"
	"github.com/standards-lab/go-core/graph"

	"github.com/JaimeStill/capstone-bunrei/integration"
)

func TestStorage_StoresRoundTripUnderTheirPrefixes(t *testing.T) {
	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Content, n.Input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ctx := context.Background()
	objects := sys.Get(n.Storage)
	if err := objects.Start(ctx); err != nil {
		t.Fatalf("start the storage: %v", err)
	}
	t.Cleanup(func() { _ = objects.Shutdown(context.Background()) })

	for _, tc := range []struct {
		name   string
		node   *graph.Node[*docstorage.Content]
		prefix string
	}{
		{"content", n.Content, "content/"},
		{"input", n.Input, "input/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "a document staged as " + tc.name
			raw := sha256.Sum256([]byte(body))
			sum := hex.EncodeToString(raw[:])
			store := sys.Get(tc.node)

			if err := store.Put(ctx, sum, strings.NewReader(body)); err != nil {
				t.Fatalf("Put: %v", err)
			}
			key := tc.prefix + sum[:2] + "/" + sum
			if obj, err := objects.Stat(ctx, key); err != nil || obj.Size != int64(len(body)) {
				t.Errorf("Stat %s = %+v, %v; want the object, %d bytes, at its prefix's key", key, obj, err, len(body))
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
		})
	}
}
