package config

import (
	"fmt"

	libconfig "github.com/standards-lab/go-core/config"
	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-core/logging"
	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-messaging/nats"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-web-sdk"
)

const envPrefix = "bunrei"

// Config is the service's root configuration: the library capability blocks
// (the log, the server, the database connection, the storage connection, the
// BUNREI_JOBS stream's NATS broker, and the BUNREI_WORK stream's NATS work
// queue) plus the service-owned read policy, object layout, upload policy,
// submission policy, and sweep policy. It embeds the lifecycle
// Coordinator's configuration by value and untagged, so its
// shutdown_timeout is a top-level key and ShutdownTimeout a promoted field;
// Config.Config is that lifecycle block, the one the composition root hands
// the Coordinator.
type Config struct {
	lifecycle.Config

	Log         logging.Config    `json:"log"`
	Server      web.Config        `json:"server"`
	Database    database.Config   `json:"database"`
	Storage     storage.Config    `json:"storage"`
	NATS        nats.Config       `json:"nats"`
	NATSQueue   nats.QueueConfig  `json:"nats_queue"`
	Objects     ObjectsConfig     `json:"objects"`
	Reads       ReadsConfig       `json:"reads"`
	Uploads     UploadsConfig     `json:"uploads"`
	Submissions SubmissionsConfig `json:"submissions"`
	Sweep       SweepConfig       `json:"sweep"`
}

// Merge overlays src's set fields onto the receiver, delegating each block
// to its own Merge.
func (c *Config) Merge(src *Config) {
	if src == nil {
		return
	}
	c.Config.Merge(&src.Config)
	c.Log.Merge(&src.Log)
	c.Server.Merge(&src.Server)
	c.Database.Merge(&src.Database)
	c.Storage.Merge(&src.Storage)
	c.NATS.Merge(&src.NATS)
	c.NATSQueue.Merge(&src.NATSQueue)
	c.Objects.Merge(&src.Objects)
	c.Reads.Merge(&src.Reads)
	c.Uploads.Merge(&src.Uploads)
	c.Submissions.Merge(&src.Submissions)
	c.Sweep.Merge(&src.Sweep)
}

// Finalize finalizes each block under the same prefix, the lifecycle block
// first, and validates. It satisfies the config package's Load contract.
// The lifecycle block's error returns as it is, unlabelled, since it names
// its own key or variable: it applies the 10s default shutdown timeout,
// reads <PREFIX>_SHUTDOWN_TIMEOUT, and rejects a non-positive timeout, on
// which the lifecycle coordinator panics. The nats and nats_queue blocks'
// errors return unlabelled too, since go-messaging's provider labels its
// own ("nats: config:" and "nats: queue config:"); their variables are
// <PREFIX>_NATS_* and <PREFIX>_NATS_QUEUE_*. An empty prefix composes empty
// variable names, which read as no override, so it disables every
// environment override; tests use this hermetic form. Once every block is
// final, it checks the sweep's content_age and input_age each exceed the
// objects block's renew_age.
func (c *Config) Finalize(envPrefix string) error {
	if err := c.Config.Finalize(envPrefix); err != nil {
		return err
	}
	if err := c.Log.Finalize(envPrefix); err != nil {
		return fmt.Errorf("log: %w", err)
	}
	if err := c.Server.Finalize(envPrefix); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if err := c.Database.Finalize(envPrefix); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := c.Storage.Finalize(envPrefix); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	if err := c.NATS.Finalize(envPrefix); err != nil {
		return err
	}
	if err := c.NATSQueue.Finalize(envPrefix); err != nil {
		return err
	}
	if err := c.Objects.Finalize(envPrefix); err != nil {
		return fmt.Errorf("objects: %w", err)
	}
	if err := c.Reads.Finalize(envPrefix); err != nil {
		return fmt.Errorf("reads: %w", err)
	}
	if err := c.Uploads.Finalize(envPrefix); err != nil {
		return fmt.Errorf("uploads: %w", err)
	}
	if err := c.Submissions.Finalize(envPrefix); err != nil {
		return fmt.Errorf("submissions: %w", err)
	}
	if err := c.Sweep.Finalize(envPrefix); err != nil {
		return fmt.Errorf("sweep: %w", err)
	}
	for _, age := range []struct {
		key   string
		value libconfig.Duration
	}{
		{"content_age", *c.Sweep.ContentAge},
		{"input_age", *c.Sweep.InputAge},
	} {
		if age.value <= *c.Objects.RenewAge {
			return fmt.Errorf("sweep: %s %s must exceed objects renew_age %s", age.key, age.value, c.Objects.RenewAge)
		}
	}
	return nil
}

// Load reads the layered configuration files and finalizes the result under
// the service's env prefix.
func Load() (*Config, error) {
	return libconfig.Load[Config](libconfig.Options{
		EnvPrefix: envPrefix,
	})
}
