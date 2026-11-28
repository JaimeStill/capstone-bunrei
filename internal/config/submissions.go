package config

import (
	"fmt"
	"strconv"
	"time"

	libconfig "github.com/standards-lab/go-core/config"
)

const (
	defaultSubmissionsDocumentLimit = 4 << 20
	defaultSubmissionsRetention     = 30 * 24 * time.Hour
)

// SubmissionsConfig is the service-owned policy of Submit: DocumentLimit is
// the small-document limit, the most bytes a submitted document may decode
// to, past which it goes through the upload instead; Retention is how long
// a Submit's idempotency record is kept, after which its ID is free again
// and the sweeper reclaims it. Pointer fields distinguish unset from zero.
type SubmissionsConfig struct {
	DocumentLimit *int64              `json:"document_limit"`
	Retention     *libconfig.Duration `json:"retention"`
	// Env records the environment-variable names Finalize composed and read.
	Env SubmissionsEnv `json:"-"`
}

// SubmissionsEnv holds the environment-variable names of the submissions
// block.
type SubmissionsEnv struct {
	DocumentLimit string
	Retention     string
}

// Merge overlays src's set fields onto the receiver.
func (c *SubmissionsConfig) Merge(src *SubmissionsConfig) {
	if src == nil {
		return
	}
	if src.DocumentLimit != nil {
		v := *src.DocumentLimit
		c.DocumentLimit = &v
	}
	if src.Retention != nil {
		v := *src.Retention
		c.Retention = &v
	}
}

// Finalize applies the defaults, reads the block's environment overrides,
// and validates: the document limit and the retention are positive. An
// empty prefix composes empty names, which disable the overrides.
func (c *SubmissionsConfig) Finalize(envPrefix string) error {
	if c.DocumentLimit == nil {
		c.DocumentLimit = new(int64(defaultSubmissionsDocumentLimit))
	}
	if c.Retention == nil {
		c.Retention = new(libconfig.Duration(defaultSubmissionsRetention))
	}
	c.Env.DocumentLimit = libconfig.EnvName(envPrefix, "submissions_document_limit")
	c.Env.Retention = libconfig.EnvName(envPrefix, "submissions_retention")
	parse := func(v string) (int64, error) { return strconv.ParseInt(v, 10, 64) }
	if err := libconfig.SetFromEnv(&c.DocumentLimit, c.Env.DocumentLimit, parse); err != nil {
		return err
	}
	if err := libconfig.SetDurationFromEnv(&c.Retention, c.Env.Retention); err != nil {
		return err
	}
	if *c.DocumentLimit <= 0 {
		return fmt.Errorf("document_limit must be positive, got %d", *c.DocumentLimit)
	}
	if *c.Retention <= 0 {
		return fmt.Errorf("retention must be positive, got %s", c.Retention)
	}
	return nil
}
