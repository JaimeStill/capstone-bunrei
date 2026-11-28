package config

import (
	"fmt"
	"time"

	libconfig "github.com/standards-lab/go-core/config"
)

const defaultUploadsLifetime = 24 * time.Hour

// UploadsConfig is the service-owned policy of the resumable uploads:
// Lifetime is how long an upload stays open from its creation, after
// which it is missing to every request and the sweeper reclaims its rows.
// The sweeper reclaims the objects under the objects block's upload
// prefix at twice the lifetime, so no object an open upload names is
// reclaimed. Pointer fields distinguish unset from zero.
type UploadsConfig struct {
	Lifetime *libconfig.Duration `json:"lifetime"`
	// Env records the environment-variable names Finalize composed and read.
	Env UploadsEnv `json:"-"`
}

// UploadsEnv holds the environment-variable names of the uploads block.
type UploadsEnv struct {
	Lifetime string
}

// Merge overlays src's set fields onto the receiver.
func (c *UploadsConfig) Merge(src *UploadsConfig) {
	if src == nil {
		return
	}
	if src.Lifetime != nil {
		v := *src.Lifetime
		c.Lifetime = &v
	}
}

// Finalize applies the default, reads the block's environment override,
// and validates: the lifetime is positive. An empty prefix composes an
// empty name, which disables the override.
func (c *UploadsConfig) Finalize(envPrefix string) error {
	if c.Lifetime == nil {
		c.Lifetime = new(libconfig.Duration(defaultUploadsLifetime))
	}
	c.Env.Lifetime = libconfig.EnvName(envPrefix, "uploads_lifetime")
	if err := libconfig.SetDurationFromEnv(&c.Lifetime, c.Env.Lifetime); err != nil {
		return err
	}
	if *c.Lifetime <= 0 {
		return fmt.Errorf("lifetime must be positive, got %s", c.Lifetime)
	}
	return nil
}

// ObjectAge is the age past which the sweeper reclaims an object under the
// upload prefix: twice the lifetime.
func (c UploadsConfig) ObjectAge() time.Duration {
	return 2 * c.Lifetime.Duration()
}
