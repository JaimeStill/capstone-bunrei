package config_test

import (
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
	if *o.ContentPrefix != "content/" || *o.InputPrefix != "input/" {
		t.Errorf("Objects prefixes = %q, %q, want content/ and input/", *o.ContentPrefix, *o.InputPrefix)
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
	t.Setenv("BUNREI_OBJECTS_RENEW_AGE", "24h")

	cfg := configtest.Minimal()
	if err := cfg.Finalize("bunrei"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	o := cfg.Objects
	if *o.ContentPrefix != "c/" || *o.InputPrefix != "i/" || o.RenewAge.Duration() != 24*time.Hour {
		t.Errorf("Objects = %q, %q, %s, want c/, i/, 24h", *o.ContentPrefix, *o.InputPrefix, o.RenewAge)
	}
	want := config.ObjectsEnv{
		ContentPrefix: "BUNREI_OBJECTS_CONTENT_PREFIX",
		InputPrefix:   "BUNREI_OBJECTS_INPUT_PREFIX",
		RenewAge:      "BUNREI_OBJECTS_RENEW_AGE",
	}
	if o.Env != want {
		t.Errorf("Objects.Env = %+v, want %+v", o.Env, want)
	}
}

func TestObjects_FinalizeRejectsInvalidLayout(t *testing.T) {
	cases := map[string]struct {
		content, input, renew string
		want                  string
	}{
		"content without a slash": {"content", "input/", "1h", "content_prefix"},
		"input a bare slash":      {"content/", "/", "1h", "input_prefix"},
		"same prefix":             {"blobs/", "blobs/", "1h", "overlap"},
		"input under content":     {"blobs/", "blobs/input/", "1h", "overlap"},
		"negative renew age":      {"content/", "input/", "-1h", "renew_age"},
		"renew age not a period":  {"content/", "input/", "weekly", "BUNREI_OBJECTS_RENEW_AGE"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BUNREI_OBJECTS_CONTENT_PREFIX", tc.content)
			t.Setenv("BUNREI_OBJECTS_INPUT_PREFIX", tc.input)
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
