package configtest

import (
	"fmt"
	"net"
	"testing"

	"github.com/standards-lab/go-core/logging"

	"github.com/JaimeStill/capstone-bunrei/internal/config"
)

// Minimal returns an unfinalized Config with every block's required fields
// set and nothing else: the base for tests that exercise Finalize
// themselves, under their own prefix.
func Minimal() *config.Config {
	cfg := &config.Config{}
	cfg.Database.Name = "app"
	cfg.Storage.Container = "bunrei"
	cfg.NATS.Stream = "BUNREI_JOBS"
	cfg.NATS.Prefix = "bunrei.job"
	cfg.NATSQueue.Stream = "BUNREI_WORK"
	cfg.NATSQueue.Subject = "bunrei.work"
	cfg.NATSQueue.DeadLetter = "bunrei.work.dead"
	return cfg
}

// Config returns a finalized Config whose composition performs no I/O: the
// server on a loopback ephemeral port, debug logging so requests leave
// records, and the database, the object store, and the NATS broker and
// work queue each aimed at a closed loopback port, so each connection is
// refused immediately instead of timing out, and the store tries once
// rather than retrying. The empty prefix disables environment overrides.
func Config(t *testing.T) *config.Config {
	t.Helper()
	cfg := Minimal()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = new(int)
	cfg.Log.Level = logging.LevelDebug
	cfg.Database.Host = "127.0.0.1"
	cfg.Database.User = "app"
	port := ClosedPort(t)
	cfg.Database.Port = &port
	cfg.Storage.Endpoint = fmt.Sprintf("http://127.0.0.1:%d", ClosedPort(t))
	cfg.Storage.Account = "admin"
	cfg.Storage.Key = "secret"
	cfg.Storage.Options = map[string]string{"max_retries": "0"}
	nats := fmt.Sprintf("nats://127.0.0.1:%d", ClosedPort(t))
	cfg.NATS.URL = nats
	cfg.NATSQueue.URL = nats
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("finalize hermetic config: %v", err)
	}
	return cfg
}

// ClosedPort reserves an ephemeral loopback port and releases it, so a
// connection attempt against it is refused immediately.
func ClosedPort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	if err := lis.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	return port
}
