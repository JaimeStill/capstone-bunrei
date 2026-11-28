package integration

import (
	"context"
	"database/sql"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process/processtest"
	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-database/postgres"
	"github.com/standards-lab/go-web-sdk/webtest"

	"github.com/JaimeStill/capstone-bunrei/internal/app"
	"github.com/JaimeStill/capstone-bunrei/internal/config"
)

const (
	defaultDatabaseHost     = "127.0.0.1"
	defaultDatabasePort     = "5443"
	defaultDatabasePassword = "app"
	defaultStorageEndpoint  = "http://127.0.0.1:8338"
	defaultStorageAccount   = "admin"
	defaultStorageKey       = "secret"
	defaultNATSURL          = "nats://127.0.0.1:4227"
)

func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func databaseHostPort() (host, port string) {
	return getenv("BUNREI_DATABASE_HOST", defaultDatabaseHost), getenv("BUNREI_DATABASE_PORT", defaultDatabasePort)
}

func databasePassword() string {
	return getenv("BUNREI_DATABASE_PASSWORD", defaultDatabasePassword)
}

// NATSURL is the compose NATS server the harness points the service's job
// journal and work queue at, so a test can inspect the streams the service
// provisions there.
func NATSURL() string {
	return getenv("BUNREI_NATS_URL", defaultNATSURL)
}

func stack() []string {
	dbHost, dbPort := databaseHostPort()
	return []string{
		"BUNREI_DATABASE_HOST=" + dbHost,
		"BUNREI_DATABASE_PORT=" + dbPort,
		"BUNREI_DATABASE_PASSWORD=" + databasePassword(),
		"BUNREI_STORAGE_ENDPOINT=" + getenv("BUNREI_STORAGE_ENDPOINT", defaultStorageEndpoint),
		"BUNREI_STORAGE_ACCOUNT=" + getenv("BUNREI_STORAGE_ACCOUNT", defaultStorageAccount),
		"BUNREI_STORAGE_KEY=" + getenv("BUNREI_STORAGE_KEY", defaultStorageKey),
		"BUNREI_NATS_URL=" + NATSURL(),
		"BUNREI_NATS_QUEUE_URL=" + getenv("BUNREI_NATS_QUEUE_URL", defaultNATSURL),
	}
}

// Main is the suite's TestMain: it builds cmd/server once, with the race
// detector so the service runs under it too, runs the tests, and removes
// the build.
func Main(m *testing.M) {
	processtest.Main(m, "./cmd/server")
}

// Options shapes one service process. The zero value runs the service on
// the base configuration file and the harness's own variables alone,
// against the compose database.
type Options struct {
	// Env appends further KEY=VALUE overrides, applied last.
	Env []string
}

// Service is one running service process: its address, its captured
// output, and its exit.
type Service struct {
	*processtest.Process
	addr   string
	client *webtest.Client
}

// Start runs the service with opts and returns once it is live: Launch
// then Ready.
func Start(t testing.TB, opts Options) *Service {
	t.Helper()
	return Launch(t, opts).Ready(t)
}

// Launch runs the service with opts on a reserved loopback port and returns
// without waiting, so a test can start several processes at once; Ready
// waits for one.
func Launch(t testing.TB, opts Options) *Service {
	t.Helper()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(processtest.FreePort(t)))
	s := &Service{addr: addr, client: webtest.NewClient("http://" + addr)}
	s.Process = processtest.Launch(t, environment(opts, addr)...)
	return s
}

// Ready waits until the service's liveness probe answers, failing the test
// with the captured output if the process exits or the failsafe elapses
// first. The server is alone in the top layer of the service's graph, so a
// live probe means every node beneath it started.
func (s *Service) Ready(t testing.TB) *Service {
	t.Helper()
	s.Await(t, "liveness", func() bool { return webtest.Live(s.URL()) })
	return s
}

func environment(opts Options, addr string) []string {
	host, port, _ := net.SplitHostPort(addr)
	env := []string{
		"BUNREI_ENV=",
		"BUNREI_LOG_LEVEL=debug",
		"BUNREI_LOG_FORMAT=text",
		"BUNREI_SERVER_HOST=" + host,
		"BUNREI_SERVER_PORT=" + port,
	}
	env = append(env, stack()...)
	return append(env, opts.Env...)
}

// Addr is the service's address, host:port.
func (s *Service) Addr() string { return s.addr }

// URL is the service's base URL.
func (s *Service) URL() string { return "http://" + s.addr }

// Client returns the client bound to the service, one per process so its
// connection is reused across calls.
func (s *Service) Client() *webtest.Client { return s.client }

// Database opens a pool on the compose database the harness points the
// service at, so a test can read beneath the API what no route reaches,
// such as the tables the startup migrations create. It is the harness's
// pool, not the service's, and it is shut down at cleanup.
func Database(t testing.TB) *sql.DB {
	t.Helper()
	host, port := databaseHostPort()
	p, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("database port %q: %v", port, err)
	}
	cfg := database.Config{
		Host:     host,
		Port:     &p,
		Name:     "app",
		User:     "app",
		Password: databasePassword(),
		Options:  map[string]string{"sslmode": "disable"},
	}
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("database config: %v", err)
	}
	db, err := postgres.New(cfg)
	if err != nil {
		t.Fatalf("database pool: %v", err)
	}
	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("start the database pool: %v", err)
	}
	t.Cleanup(func() { _ = db.Shutdown(context.Background()) })
	return db.Conn()
}

// Compose loads the service's configuration as the binary does, from the
// module root's base file with the harness's variables pointing it at the
// compose stack, and returns the App described over it, unbuilt, so a test
// can Build and drive a node no route reaches, such as the content store or
// the job root.
// It sets the variables and the working directory for the test's duration,
// so a test that calls it cannot run in parallel.
func Compose(t *testing.T) *app.App {
	t.Helper()
	t.Chdir("..")
	t.Setenv("BUNREI_ENV", "")
	for _, kv := range stack() {
		name, value, _ := strings.Cut(kv, "=")
		t.Setenv(name, value)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load the configuration: %v", err)
	}
	return app.New(cfg, io.Discard)
}
