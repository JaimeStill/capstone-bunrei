package config_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	libconfig "github.com/standards-lab/go-core/config"
	"github.com/standards-lab/go-core/logging"
	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/internal/config"
	"github.com/JaimeStill/capstone-bunrei/internal/config/configtest"
)

func TestConfig_MergeOverlaysSetFields(t *testing.T) {
	base := &config.Config{ShutdownTimeout: libconfig.Duration(10 * time.Second)}
	base.Log.Level = logging.LevelInfo
	base.Server.Host = "0.0.0.0"

	overlay := &config.Config{}
	overlay.Log.Level = logging.LevelDebug
	overlay.Server.Host = "127.0.0.1"

	base.Merge(overlay)

	if base.Log.Level != logging.LevelDebug {
		t.Errorf("Log.Level = %s, want debug", base.Log.Level)
	}
	if base.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %s, want 127.0.0.1", base.Server.Host)
	}
	if got := base.ShutdownTimeout.Duration(); got != 10*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 10s", got)
	}
}

func TestConfig_FinalizeDefaults(t *testing.T) {
	cfg := configtest.Minimal()
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if got := cfg.ShutdownTimeout.Duration(); got != 10*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 10s", got)
	}
	if cfg.Log.Level != logging.LevelInfo {
		t.Errorf("Log.Level = %s, want info", cfg.Log.Level)
	}
	if got := cfg.Server.Addr(); got != "0.0.0.0:8080" {
		t.Errorf("Server.Addr() = %s, want 0.0.0.0:8080", got)
	}
}

func TestConfig_FinalizeReadsEnvUnderPrefix(t *testing.T) {
	t.Setenv("SVC_LOG_LEVEL", "debug")
	t.Setenv("SVC_SERVER_PORT", "9090")
	t.Setenv("BUNREI_LOG_LEVEL", "error")
	t.Setenv("BUNREI_SERVER_PORT", "7070")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("svc"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if cfg.Log.Level != logging.LevelDebug {
		t.Errorf("Log.Level = %s, want debug from SVC_LOG_LEVEL", cfg.Log.Level)
	}
	if cfg.Server.Port == nil || *cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %v, want 9090 from SVC_SERVER_PORT", cfg.Server.Port)
	}
}

func TestConfig_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_SHUTDOWN_TIMEOUT", "30s")
	t.Setenv("BUNREI_LOG_LEVEL", "debug")
	t.Setenv("BUNREI_SERVER_PORT", "9090")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if got := cfg.ShutdownTimeout.Duration(); got != 30*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 30s", got)
	}
	if cfg.Log.Level != logging.LevelDebug {
		t.Errorf("Log.Level = %s, want debug", cfg.Log.Level)
	}
	if cfg.Server.Port == nil || *cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %v, want 9090", cfg.Server.Port)
	}
}

func TestConfig_FinalizeRejectsNonPositiveShutdownTimeout(t *testing.T) {
	for _, v := range []string{"-5s", "0s"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("BUNREI_SHUTDOWN_TIMEOUT", v)

			cfg := configtest.Minimal()
			err := cfg.Finalize("bunrei")
			if err == nil {
				t.Fatalf("Finalize accepted shutdown_timeout %s", v)
			}
			if !strings.Contains(err.Error(), "shutdown_timeout must be positive") {
				t.Errorf("error = %v, want \"shutdown_timeout must be positive\"", err)
			}
		})
	}
}

func TestConfig_FinalizeRejectsNonPositiveFileShutdownTimeout(t *testing.T) {
	cfg := &config.Config{ShutdownTimeout: libconfig.Duration(-time.Second)}
	err := cfg.Finalize("")
	if err == nil || !strings.Contains(err.Error(), "shutdown_timeout must be positive") {
		t.Errorf("Finalize = %v, want \"shutdown_timeout must be positive\"", err)
	}
}

func TestConfig_FinalizeRejectsUnparsableShutdownTimeout(t *testing.T) {
	t.Setenv("BUNREI_SHUTDOWN_TIMEOUT", "soon")

	cfg := configtest.Minimal()
	err := cfg.Finalize("bunrei")
	if err == nil {
		t.Fatal("Finalize accepted an unparsable BUNREI_SHUTDOWN_TIMEOUT")
	}
	if !strings.Contains(err.Error(), "BUNREI_SHUTDOWN_TIMEOUT") {
		t.Errorf("error = %v, want it to name BUNREI_SHUTDOWN_TIMEOUT", err)
	}
}

func TestConfig_FinalizeEmptyPrefixReadsNoShutdownOverride(t *testing.T) {
	t.Setenv("BUNREI_SHUTDOWN_TIMEOUT", "30s")
	t.Setenv("SHUTDOWN_TIMEOUT", "30s")

	cfg := configtest.Minimal()
	cfg.ShutdownTimeout = libconfig.Duration(3 * time.Second)
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if got := cfg.ShutdownTimeout.Duration(); got != 3*time.Second {
		t.Errorf("ShutdownTimeout = %s, want the configured 3s", got)
	}
}

func TestConfig_MergeOverlayShutdownTimeoutReplacesBase(t *testing.T) {
	base := &config.Config{ShutdownTimeout: libconfig.Duration(10 * time.Second)}
	base.Merge(&config.Config{ShutdownTimeout: libconfig.Duration(3 * time.Second)})

	if got := base.ShutdownTimeout.Duration(); got != 3*time.Second {
		t.Errorf("ShutdownTimeout = %s, want the overlay's 3s", got)
	}
}

func TestConfig_FinalizeWrapsChildErrors(t *testing.T) {
	t.Setenv("BUNREI_LOG_LEVEL", "verbose")

	cfg := configtest.Minimal()
	err := cfg.Finalize("bunrei")
	if err == nil {
		t.Fatal("Finalize accepted an invalid log level")
	}
	if !strings.Contains(err.Error(), "log:") {
		t.Errorf("error = %v, want the log block wrap", err)
	}
}

func TestReads_FinalizeDefaults(t *testing.T) {
	cfg := configtest.Minimal()
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if got := cfg.Reads.Limits(); got != (web.Limits{DefaultSize: 20, MaxSize: 100}) {
		t.Errorf("Reads.Limits() = %+v, want {20 100}", got)
	}
	if cfg.Reads.Env != (config.ReadsEnv{}) {
		t.Errorf("Reads.Env = %+v, want empty under the empty prefix", cfg.Reads.Env)
	}
}

func TestReads_MergeOverlaysSetFields(t *testing.T) {
	base := &config.Config{}
	base.Reads.DefaultSize = new(10)
	base.Reads.MaxSize = new(50)

	overlay := &config.Config{}
	overlay.Reads.MaxSize = new(200)

	base.Merge(overlay)

	if got := *base.Reads.MaxSize; got != 200 {
		t.Errorf("Reads.MaxSize = %d, want 200", got)
	}
	if got := *base.Reads.DefaultSize; got != 10 {
		t.Errorf("Reads.DefaultSize = %d, want 10", got)
	}
}

func TestReads_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_READS_DEFAULT_SIZE", "25")
	t.Setenv("BUNREI_READS_MAX_SIZE", "250")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if got := cfg.Reads.Limits(); got != (web.Limits{DefaultSize: 25, MaxSize: 250}) {
		t.Errorf("Reads.Limits() = %+v, want {25 250}", got)
	}
	want := config.ReadsEnv{DefaultSize: "BUNREI_READS_DEFAULT_SIZE", MaxSize: "BUNREI_READS_MAX_SIZE"}
	if cfg.Reads.Env != want {
		t.Errorf("Reads.Env = %+v, want %+v", cfg.Reads.Env, want)
	}
}

func TestReads_FinalizeRejectsInvalidPolicy(t *testing.T) {
	cases := map[string]struct {
		defaultSize, maxSize string
		want                 string
	}{
		"default below one":    {"0", "100", "default_size"},
		"max below default":    {"20", "10", "max_size"},
		"default not a number": {"twenty", "100", "BUNREI_READS_DEFAULT_SIZE"},
		"max not a number":     {"20", "many", "BUNREI_READS_MAX_SIZE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BUNREI_READS_DEFAULT_SIZE", tc.defaultSize)
			t.Setenv("BUNREI_READS_MAX_SIZE", tc.maxSize)

			cfg := configtest.Minimal()
			err := cfg.Finalize("bunrei")
			if err == nil {
				t.Fatal("Finalize accepted an invalid reads policy")
			}
			if !strings.Contains(err.Error(), "reads:") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want the reads block wrap naming %s", err, tc.want)
			}
		})
	}
}

func TestDatabase_MergeOverlaysSetFields(t *testing.T) {
	base := configtest.Minimal()
	base.Database.User = "app"
	base.Database.Options = map[string]string{"sslmode": "disable"}

	overlay := &config.Config{}
	overlay.Database.Host = "127.0.0.1"
	overlay.Database.Port = new(5443)

	base.Merge(overlay)

	if base.Database.Host != "127.0.0.1" || base.Database.Port == nil || *base.Database.Port != 5443 {
		t.Errorf("Database address = %s:%v, want the overlay's 127.0.0.1:5443", base.Database.Host, base.Database.Port)
	}
	if base.Database.Name != "app" || base.Database.User != "app" || base.Database.Options["sslmode"] != "disable" {
		t.Errorf("Database = %+v, want the base's name, user, and options kept", base.Database)
	}
}

func TestDatabase_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_DATABASE_HOST", "db.internal")
	t.Setenv("BUNREI_DATABASE_PORT", "5443")
	t.Setenv("BUNREI_DATABASE_NAME", "bunrei")
	t.Setenv("BUNREI_DATABASE_USER", "svc")
	t.Setenv("BUNREI_DATABASE_PASSWORD", "secret")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	db := cfg.Database
	if db.Host != "db.internal" || db.Port == nil || *db.Port != 5443 {
		t.Errorf("Database address = %s:%v, want db.internal:5443", db.Host, db.Port)
	}
	if db.Name != "bunrei" || db.User != "svc" || db.Password != "secret" {
		t.Errorf("Database identity = %s/%s/%s, want bunrei/svc/secret", db.Name, db.User, db.Password)
	}
	if db.Env.Port != "BUNREI_DATABASE_PORT" {
		t.Errorf("Database.Env.Port = %q, want BUNREI_DATABASE_PORT", db.Env.Port)
	}
}

func TestDatabase_FinalizeRequiresName(t *testing.T) {
	cfg := &config.Config{}
	err := cfg.Finalize("")
	if err == nil {
		t.Fatal("Finalize accepted a database block without a name")
	}
	if !strings.Contains(err.Error(), "database:") {
		t.Errorf("error = %v, want the database block wrap", err)
	}
}

func TestStorage_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_STORAGE_ENDPOINT", "http://127.0.0.1:8338")
	t.Setenv("BUNREI_STORAGE_CONTAINER", "jobs")
	t.Setenv("BUNREI_STORAGE_ACCOUNT", "admin")
	t.Setenv("BUNREI_STORAGE_KEY", "secret")
	t.Setenv("BUNREI_STORAGE_OPTIONS_REGION", "eu-west-1")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	st := cfg.Storage
	if st.Endpoint != "http://127.0.0.1:8338" || st.Container != "jobs" {
		t.Errorf("Storage = %s/%s, want http://127.0.0.1:8338/jobs", st.Endpoint, st.Container)
	}
	if st.Account != "admin" || st.Key != "secret" {
		t.Errorf("Storage credential = %s/%s, want admin/secret", st.Account, st.Key)
	}
	if st.Options["region"] != "eu-west-1" {
		t.Errorf("Storage.Options = %v, want region eu-west-1", st.Options)
	}
	if st.Env.Key != "BUNREI_STORAGE_KEY" {
		t.Errorf("Storage.Env.Key = %q, want BUNREI_STORAGE_KEY", st.Env.Key)
	}
}

func TestStorage_FinalizeRequiresContainer(t *testing.T) {
	cfg := configtest.Minimal()
	cfg.Storage.Container = ""
	err := cfg.Finalize("")
	if err == nil {
		t.Fatal("Finalize accepted a storage block without a container")
	}
	if !strings.Contains(err.Error(), "storage:") || !strings.Contains(err.Error(), "container") {
		t.Errorf("error = %v, want the storage block wrap naming the container", err)
	}
}

func TestObjects_FinalizeDefaults(t *testing.T) {
	cfg := configtest.Minimal()
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	o := cfg.Objects
	if *o.ContentPrefix != "content/" || *o.InputPrefix != "input/" || *o.UploadPrefix != "upload/" {
		t.Errorf("Objects prefixes = %q, %q, %q, want content/, input/, and upload/", *o.ContentPrefix, *o.InputPrefix, *o.UploadPrefix)
	}
	if got := o.RenewAge.Duration(); got != 7*24*time.Hour {
		t.Errorf("Objects.RenewAge = %s, want 168h", got)
	}
	if o.Env != (config.ObjectsEnv{}) {
		t.Errorf("Objects.Env = %+v, want empty under the empty prefix", o.Env)
	}
}

func TestObjects_MergeOverlaysSetFields(t *testing.T) {
	base := &config.Config{}
	base.Objects.ContentPrefix = new("content/")
	base.Objects.RenewAge = new(libconfig.Duration(time.Hour))

	overlay := &config.Config{}
	overlay.Objects.RenewAge = new(libconfig.Duration(2 * time.Hour))

	base.Merge(overlay)

	if got := base.Objects.RenewAge.Duration(); got != 2*time.Hour {
		t.Errorf("Objects.RenewAge = %s, want the overlay's 2h", got)
	}
	if got := *base.Objects.ContentPrefix; got != "content/" {
		t.Errorf("Objects.ContentPrefix = %q, want the base's content/", got)
	}
}

func TestObjects_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_OBJECTS_CONTENT_PREFIX", "c/")
	t.Setenv("BUNREI_OBJECTS_INPUT_PREFIX", "i/")
	t.Setenv("BUNREI_OBJECTS_UPLOAD_PREFIX", "u/")
	t.Setenv("BUNREI_OBJECTS_RENEW_AGE", "24h")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	o := cfg.Objects
	if *o.ContentPrefix != "c/" || *o.InputPrefix != "i/" || *o.UploadPrefix != "u/" || o.RenewAge.Duration() != 24*time.Hour {
		t.Errorf("Objects = %q, %q, %q, %s, want c/, i/, u/, 24h", *o.ContentPrefix, *o.InputPrefix, *o.UploadPrefix, o.RenewAge)
	}
	want := config.ObjectsEnv{
		ContentPrefix: "BUNREI_OBJECTS_CONTENT_PREFIX",
		InputPrefix:   "BUNREI_OBJECTS_INPUT_PREFIX",
		UploadPrefix:  "BUNREI_OBJECTS_UPLOAD_PREFIX",
		RenewAge:      "BUNREI_OBJECTS_RENEW_AGE",
	}
	if o.Env != want {
		t.Errorf("Objects.Env = %+v, want %+v", o.Env, want)
	}
}

func TestObjects_FinalizeRejectsInvalidLayout(t *testing.T) {
	cases := map[string]struct {
		content, input, upload, renew string
		want                          string
	}{
		"content without a slash": {"content", "input/", "upload/", "1h", "content_prefix"},
		"input a bare slash":      {"content/", "/", "upload/", "1h", "input_prefix"},
		"upload without a slash":  {"content/", "input/", "upload", "1h", "upload_prefix"},
		"same prefix":             {"blobs/", "blobs/", "upload/", "1h", "overlap"},
		"input under content":     {"blobs/", "blobs/input/", "upload/", "1h", "overlap"},
		"upload under input":      {"content/", "input/", "input/upload/", "1h", "overlap"},
		"content under upload":    {"upload/content/", "input/", "upload/", "1h", "overlap"},
		"negative renew age":      {"content/", "input/", "upload/", "-1h", "renew_age"},
		"renew age not a period":  {"content/", "input/", "upload/", "weekly", "BUNREI_OBJECTS_RENEW_AGE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BUNREI_OBJECTS_CONTENT_PREFIX", tc.content)
			t.Setenv("BUNREI_OBJECTS_INPUT_PREFIX", tc.input)
			t.Setenv("BUNREI_OBJECTS_UPLOAD_PREFIX", tc.upload)
			t.Setenv("BUNREI_OBJECTS_RENEW_AGE", tc.renew)

			cfg := configtest.Minimal()
			err := cfg.Finalize("bunrei")
			if err == nil {
				t.Fatal("Finalize accepted an invalid object layout")
			}
			if !strings.Contains(err.Error(), "objects:") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want the objects block wrap naming %s", err, tc.want)
			}
		})
	}
}

func TestNATS_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_NATS_URL", "nats://nats.internal:4222")
	t.Setenv("BUNREI_NATS_STREAM", "JOBS")
	t.Setenv("BUNREI_NATS_PREFIX", "jobs")
	t.Setenv("BUNREI_NATS_OWNER", "true")
	t.Setenv("BUNREI_NATS_MAX_AGE", "168h")
	t.Setenv("BUNREI_NATS_QUEUE_URL", "nats://queue.internal:4222")
	t.Setenv("BUNREI_NATS_QUEUE_STREAM", "WORK")
	t.Setenv("BUNREI_NATS_QUEUE_SUBJECT", "work")
	t.Setenv("BUNREI_NATS_QUEUE_DEAD_LETTER", "work.dead")
	t.Setenv("BUNREI_NATS_QUEUE_OWNER", "true")
	t.Setenv("BUNREI_NATS_QUEUE_MAX_AGE", "24h")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	n := cfg.NATS
	if n.URL != "nats://nats.internal:4222" || n.Stream != "JOBS" || n.Prefix != "jobs" ||
		n.Owner == nil || !*n.Owner || n.MaxAge.Duration() != 168*time.Hour {
		t.Errorf("nats = %+v, want every field from BUNREI_NATS_*", n)
	}
	q := cfg.NATSQueue
	if q.URL != "nats://queue.internal:4222" || q.Stream != "WORK" || q.Subject != "work" || q.DeadLetter != "work.dead" ||
		q.Owner == nil || !*q.Owner || q.MaxAge.Duration() != 24*time.Hour {
		t.Errorf("nats_queue = %+v, want every field from BUNREI_NATS_QUEUE_*", q)
	}
}

func TestNATS_FinalizeNamesInvalidBlocks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spoil func(*config.Config)
		block string
	}{
		{"no stream", func(c *config.Config) { c.NATS.Stream = "" }, "nats: config:"},
		{"a non-owner's max_age", func(c *config.Config) { c.NATS.MaxAge = libconfig.Duration(time.Hour) }, "nats: config:"},
		{"no queue subject", func(c *config.Config) { c.NATSQueue.Subject = "" }, "nats: queue config:"},
		{"a dead letter over the subject", func(c *config.Config) { c.NATSQueue.DeadLetter = "bunrei" }, "nats: queue config:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configtest.Minimal()
			tc.spoil(cfg)
			err := cfg.Finalize("")
			if err == nil {
				t.Fatal("Finalize accepted the block")
			}
			if !strings.HasPrefix(err.Error(), tc.block) {
				t.Errorf("error = %v, want it to name the block, %s", err, tc.block)
			}
		})
	}
}

func TestConfig_BaseFileLoadsWithTheExampleSecrets(t *testing.T) {
	cfg, err := libconfig.Load[config.Config](libconfig.Options{Dir: "../..", SecretsName: "secrets.example.json"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.Password != "app" || cfg.Storage.Key != "secret" {
		t.Errorf("secrets = database %q, storage %q; want the example file's app and secret", cfg.Database.Password, cfg.Storage.Key)
	}
}

func TestSweep_FinalizeDefaults(t *testing.T) {
	cfg := configtest.Minimal()
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	s := cfg.Sweep
	if s.Interval.Duration() != 30*time.Second || *s.Batch != 100 {
		t.Errorf("Sweep = {%s %d}, want {30s 100}", s.Interval, *s.Batch)
	}
	day := 24 * time.Hour
	got := []time.Duration{s.OutboxAge.Duration(), s.InboxAge.Duration(), s.IndexAge.Duration(), s.ContentAge.Duration(), s.InputAge.Duration()}
	want := []time.Duration{7 * day, 7 * day, 30 * day, 67 * day, 30 * day}
	if !slices.Equal(got, want) {
		t.Errorf("Sweep ages {outbox inbox index content input} = %v, want %v", got, want)
	}
	if s.Env != (config.SweepEnv{}) {
		t.Errorf("Sweep.Env = %+v, want empty under the empty prefix", s.Env)
	}
}

func TestSweep_MergeOverlaysSetFields(t *testing.T) {
	base := &config.Config{}
	base.Sweep.Batch = new(10)
	base.Sweep.IndexAge = new(libconfig.Duration(time.Hour))

	overlay := &config.Config{}
	overlay.Sweep.ContentAge = new(libconfig.Duration(2 * time.Hour))

	base.Merge(overlay)

	if *base.Sweep.Batch != 10 || base.Sweep.IndexAge.Duration() != time.Hour {
		t.Errorf("Sweep = {batch %d, index_age %s}, want the base's {10 1h}", *base.Sweep.Batch, base.Sweep.IndexAge)
	}
	if base.Sweep.ContentAge == nil || base.Sweep.ContentAge.Duration() != 2*time.Hour {
		t.Errorf("Sweep.ContentAge = %v, want the overlay's 2h", base.Sweep.ContentAge)
	}
}

func TestSweep_FinalizeEnvOverrides(t *testing.T) {
	vars := map[string]string{
		"BUNREI_SWEEP_INTERVAL":    "5s",
		"BUNREI_SWEEP_BATCH":       "7",
		"BUNREI_SWEEP_OUTBOX_AGE":  "48h",
		"BUNREI_SWEEP_INBOX_AGE":   "72h",
		"BUNREI_SWEEP_INDEX_AGE":   "96h",
		"BUNREI_SWEEP_CONTENT_AGE": "400h",
		"BUNREI_SWEEP_INPUT_AGE":   "200h",
	}
	for name, v := range vars {
		t.Setenv(name, v)
	}

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	s := cfg.Sweep
	if s.Interval.Duration() != 5*time.Second || *s.Batch != 7 {
		t.Errorf("Sweep = {%s %d}, want {5s 7}", s.Interval, *s.Batch)
	}
	got := []time.Duration{s.OutboxAge.Duration(), s.InboxAge.Duration(), s.IndexAge.Duration(), s.ContentAge.Duration(), s.InputAge.Duration()}
	want := []time.Duration{48 * time.Hour, 72 * time.Hour, 96 * time.Hour, 400 * time.Hour, 200 * time.Hour}
	if !slices.Equal(got, want) {
		t.Errorf("Sweep ages {outbox inbox index content input} = %v, want %v", got, want)
	}
	wantEnv := config.SweepEnv{
		Interval:   "BUNREI_SWEEP_INTERVAL",
		Batch:      "BUNREI_SWEEP_BATCH",
		OutboxAge:  "BUNREI_SWEEP_OUTBOX_AGE",
		InboxAge:   "BUNREI_SWEEP_INBOX_AGE",
		IndexAge:   "BUNREI_SWEEP_INDEX_AGE",
		ContentAge: "BUNREI_SWEEP_CONTENT_AGE",
		InputAge:   "BUNREI_SWEEP_INPUT_AGE",
	}
	if s.Env != wantEnv {
		t.Errorf("Sweep.Env = %+v, want %+v", s.Env, wantEnv)
	}
}

func TestSweep_FinalizeRejectsInvalidPolicy(t *testing.T) {
	cases := map[string]struct {
		name, value string
		want        string
	}{
		"zero interval":        {"BUNREI_SWEEP_INTERVAL", "0s", "interval"},
		"zero batch":           {"BUNREI_SWEEP_BATCH", "0", "batch"},
		"negative outbox age":  {"BUNREI_SWEEP_OUTBOX_AGE", "-1h", "outbox_age"},
		"zero inbox age":       {"BUNREI_SWEEP_INBOX_AGE", "0s", "inbox_age"},
		"zero index age":       {"BUNREI_SWEEP_INDEX_AGE", "0s", "index_age"},
		"index age no period":  {"BUNREI_SWEEP_INDEX_AGE", "monthly", "BUNREI_SWEEP_INDEX_AGE"},
		"content within renew": {"BUNREI_SWEEP_CONTENT_AGE", "168h", "content_age"},
		"input within renew":   {"BUNREI_SWEEP_INPUT_AGE", "24h", "input_age"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)

			err := configtest.Minimal().Finalize("bunrei")
			if err == nil {
				t.Fatal("Finalize accepted an invalid sweep policy")
			}
			if !strings.Contains(err.Error(), "sweep:") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want the sweep block wrap naming %s", err, tc.want)
			}
		})
	}
}

func TestUploads_FinalizeDefaults(t *testing.T) {
	cfg := configtest.Minimal()
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	u := cfg.Uploads
	if got := u.Lifetime.Duration(); got != 24*time.Hour {
		t.Errorf("Uploads.Lifetime = %s, want 24h", got)
	}
	if got := u.ObjectAge(); got != 48*time.Hour {
		t.Errorf("Uploads.ObjectAge() = %s, want 48h, twice the lifetime", got)
	}
	if u.Env != (config.UploadsEnv{}) {
		t.Errorf("Uploads.Env = %+v, want empty under the empty prefix", u.Env)
	}
}

func TestUploads_MergeOverlaysSetFields(t *testing.T) {
	base := &config.Config{}
	base.Uploads.Lifetime = new(libconfig.Duration(time.Hour))

	base.Merge(&config.Config{})
	if got := base.Uploads.Lifetime.Duration(); got != time.Hour {
		t.Errorf("Uploads.Lifetime = %s, want the base's 1h under an empty overlay", got)
	}

	overlay := &config.Config{}
	overlay.Uploads.Lifetime = new(libconfig.Duration(2 * time.Hour))
	base.Merge(overlay)
	if got := base.Uploads.Lifetime.Duration(); got != 2*time.Hour {
		t.Errorf("Uploads.Lifetime = %s, want the overlay's 2h", got)
	}
}

func TestUploads_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_UPLOADS_LIFETIME", "6h")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if got := cfg.Uploads.Lifetime.Duration(); got != 6*time.Hour {
		t.Errorf("Uploads.Lifetime = %s, want 6h from BUNREI_UPLOADS_LIFETIME", got)
	}
	if got := cfg.Uploads.Env.Lifetime; got != "BUNREI_UPLOADS_LIFETIME" {
		t.Errorf("Uploads.Env.Lifetime = %q, want BUNREI_UPLOADS_LIFETIME", got)
	}
}

func TestUploads_FinalizeRejectsInvalidLifetime(t *testing.T) {
	for value, want := range map[string]string{
		"0s":    "lifetime",
		"-1h":   "lifetime",
		"daily": "BUNREI_UPLOADS_LIFETIME",
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("BUNREI_UPLOADS_LIFETIME", value)

			err := configtest.Minimal().Finalize("bunrei")
			if err == nil {
				t.Fatal("Finalize accepted an invalid upload lifetime")
			}
			if !strings.Contains(err.Error(), "uploads:") || !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want the uploads block wrap naming %s", err, want)
			}
		})
	}
}

func TestSubmissions_FinalizeDefaults(t *testing.T) {
	cfg := configtest.Minimal()
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	s := cfg.Submissions
	if got := *s.DocumentLimit; got != 4<<20 {
		t.Errorf("Submissions.DocumentLimit = %d, want 4 MiB", got)
	}
	if got := s.Retention.Duration(); got != 30*24*time.Hour {
		t.Errorf("Submissions.Retention = %s, want 720h", got)
	}
	if s.Env != (config.SubmissionsEnv{}) {
		t.Errorf("Submissions.Env = %+v, want empty under the empty prefix", s.Env)
	}
}

func TestSubmissions_MergeOverlaysSetFields(t *testing.T) {
	base := &config.Config{}
	base.Submissions.DocumentLimit = new(int64(1024))
	base.Submissions.Retention = new(libconfig.Duration(time.Hour))

	base.Merge(&config.Config{})
	if got := *base.Submissions.DocumentLimit; got != 1024 {
		t.Errorf("Submissions.DocumentLimit = %d, want the base's 1024 under an empty overlay", got)
	}

	overlay := &config.Config{}
	overlay.Submissions.DocumentLimit = new(int64(2048))
	overlay.Submissions.Retention = new(libconfig.Duration(2 * time.Hour))
	base.Merge(overlay)
	if got := *base.Submissions.DocumentLimit; got != 2048 {
		t.Errorf("Submissions.DocumentLimit = %d, want the overlay's 2048", got)
	}
	if got := base.Submissions.Retention.Duration(); got != 2*time.Hour {
		t.Errorf("Submissions.Retention = %s, want the overlay's 2h", got)
	}
}

func TestSubmissions_FinalizeEnvOverrides(t *testing.T) {
	t.Setenv("BUNREI_SUBMISSIONS_DOCUMENT_LIMIT", "1048576")
	t.Setenv("BUNREI_SUBMISSIONS_RETENTION", "48h")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	s := cfg.Submissions
	if got := *s.DocumentLimit; got != 1<<20 {
		t.Errorf("Submissions.DocumentLimit = %d, want 1 MiB from BUNREI_SUBMISSIONS_DOCUMENT_LIMIT", got)
	}
	if got := s.Retention.Duration(); got != 48*time.Hour {
		t.Errorf("Submissions.Retention = %s, want 48h from BUNREI_SUBMISSIONS_RETENTION", got)
	}
	want := config.SubmissionsEnv{DocumentLimit: "BUNREI_SUBMISSIONS_DOCUMENT_LIMIT", Retention: "BUNREI_SUBMISSIONS_RETENTION"}
	if s.Env != want {
		t.Errorf("Submissions.Env = %+v, want %+v", s.Env, want)
	}
}

func TestSubmissions_FinalizeRejectsInvalidPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
	}{
		{"BUNREI_SUBMISSIONS_DOCUMENT_LIMIT", "0", "document_limit"},
		{"BUNREI_SUBMISSIONS_DOCUMENT_LIMIT", "-1", "document_limit"},
		{"BUNREI_SUBMISSIONS_DOCUMENT_LIMIT", "4MiB", "BUNREI_SUBMISSIONS_DOCUMENT_LIMIT"},
		{"BUNREI_SUBMISSIONS_RETENTION", "0s", "retention"},
		{"BUNREI_SUBMISSIONS_RETENTION", "monthly", "BUNREI_SUBMISSIONS_RETENTION"},
	} {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)

			err := configtest.Minimal().Finalize("bunrei")
			if err == nil {
				t.Fatal("Finalize accepted an invalid submissions policy")
			}
			if !strings.Contains(err.Error(), "submissions:") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want the submissions block wrap naming %s", err, tc.want)
			}
		})
	}
}
