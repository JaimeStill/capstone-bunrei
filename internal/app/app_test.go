package app_test

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaimeStill/capstone-bunrei/internal/app"
	"github.com/JaimeStill/capstone-bunrei/internal/config/configtest"
)

const failsafe = 2 * time.Second

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRun_FailsStartupWithoutItsConnections(t *testing.T) {
	log, stderr := &syncBuffer{}, &syncBuffer{}
	a := app.New(configtest.Config(t), log)

	done := make(chan int, 1)
	go func() { done <- a.Run(context.Background(), stderr) }()

	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("Run = %d, want 1 on a failed startup", code)
		}
	case <-time.After(failsafe):
		t.Fatalf("timed out waiting for Run to fail startup; log: %q", log.String())
	}

	out := log.String()
	failure := regexp.MustCompile(`msg="service failed" error="[^"]*startup: (database|storage|job journal|work queue)`)
	if !failure.MatchString(out) {
		t.Errorf("log = %q, want a service failure naming a connection's startup", out)
	}
	if strings.Contains(out, "server ready") {
		t.Error("log carries a ready record despite the failed startup")
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want nothing: a startup failure is logged", got)
	}
}
