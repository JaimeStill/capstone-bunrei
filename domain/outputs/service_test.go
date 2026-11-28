package outputs_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	docstorage "github.com/standards-lab/docproc/storage"
	"github.com/standards-lab/go-storage/storagetest"

	"github.com/JaimeStill/capstone-bunrei/domain/outputs"
)

type fixture struct {
	service *outputs.Service
	content *docstorage.Content
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	content, err := docstorage.New(docstorage.Config{Client: storagetest.NewFake(storagetest.Options{}), Prefix: "content/"})
	if err != nil {
		t.Fatalf("content store: %v", err)
	}
	return &fixture{service: outputs.New(content), content: content}
}

func sum(body string) string {
	raw := sha256.Sum256([]byte(body))
	return hex.EncodeToString(raw[:])
}

func (f *fixture) put(t *testing.T, body string) string {
	t.Helper()
	s := sum(body)
	if err := f.content.Put(context.Background(), s, strings.NewReader(body)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return s
}

func TestOpen_ReadsTheStoredContent(t *testing.T) {
	f := newFixture(t)
	s := f.put(t, "an output's bytes")

	r, err := f.service.Open(context.Background(), s)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = r.Close() }()
	if got, err := io.ReadAll(r); err != nil || string(got) != "an output's bytes" {
		t.Errorf("read %q, %v; want the stored bytes", got, err)
	}
}

func TestOpen_AnAbsentOutputIsNotFound(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.Open(context.Background(), sum("never stored")); !errors.Is(err, outputs.ErrNotFound) {
		t.Errorf("Open = %v, want ErrNotFound", err)
	}
}

func TestOpen_AnInvalidSHA256IsRefused(t *testing.T) {
	f := newFixture(t)
	for _, s := range []string{"", "ab12", strings.ToUpper(sum("x")), sum("x")[:63] + "g", sum("x") + "0"} {
		if _, err := f.service.Open(context.Background(), s); !errors.Is(err, outputs.ErrInvalid) {
			t.Errorf("Open(%q) = %v, want ErrInvalid", s, err)
		}
	}
}

func TestNew_NilContentPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New(nil) did not panic")
		}
	}()
	outputs.New(nil)
}
