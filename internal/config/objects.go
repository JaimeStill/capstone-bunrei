package config

import (
	"fmt"
	"strings"
	"time"

	libconfig "github.com/standards-lab/go-core/config"
)

const (
	defaultObjectsContentPrefix = "content/"
	defaultObjectsInputPrefix   = "input/"
	defaultObjectsRenewAge      = 7 * 24 * time.Hour
)

// ObjectsConfig is the service-owned layout of its objects in the storage
// container: the prefix docproc's content store keeps each content under,
// the prefix a job's staged input is kept under, and the renew age both
// stores share, past which a put of an object already present writes it
// again. Pointer fields distinguish unset from zero.
type ObjectsConfig struct {
	ContentPrefix *string             `json:"content_prefix"`
	InputPrefix   *string             `json:"input_prefix"`
	RenewAge      *libconfig.Duration `json:"renew_age"`
	// Env records the environment-variable names Finalize composed and read.
	Env ObjectsEnv `json:"-"`
}

// ObjectsEnv holds the environment-variable names of the objects block.
type ObjectsEnv struct {
	ContentPrefix string
	InputPrefix   string
	RenewAge      string
}

// Merge overlays src's set fields onto the receiver.
func (c *ObjectsConfig) Merge(src *ObjectsConfig) {
	if src == nil {
		return
	}
	if src.ContentPrefix != nil {
		v := *src.ContentPrefix
		c.ContentPrefix = &v
	}
	if src.InputPrefix != nil {
		v := *src.InputPrefix
		c.InputPrefix = &v
	}
	if src.RenewAge != nil {
		v := *src.RenewAge
		c.RenewAge = &v
	}
}

// Finalize applies the defaults, reads the block's environment overrides,
// and validates: each prefix ends in a slash, so its objects sit under a
// directory of their own, and neither prefix begins the other, so the two
// stores never share a key; the renew age is not negative, and zero never
// rewrites. An empty prefix composes empty names, which disable the
// overrides.
func (c *ObjectsConfig) Finalize(envPrefix string) error {
	if c.ContentPrefix == nil {
		c.ContentPrefix = new(defaultObjectsContentPrefix)
	}
	if c.InputPrefix == nil {
		c.InputPrefix = new(defaultObjectsInputPrefix)
	}
	if c.RenewAge == nil {
		c.RenewAge = new(libconfig.Duration(defaultObjectsRenewAge))
	}
	c.Env.ContentPrefix = libconfig.EnvName(envPrefix, "objects_content_prefix")
	c.Env.InputPrefix = libconfig.EnvName(envPrefix, "objects_input_prefix")
	c.Env.RenewAge = libconfig.EnvName(envPrefix, "objects_renew_age")
	identity := func(v string) (string, error) { return v, nil }
	if err := libconfig.SetFromEnv(&c.ContentPrefix, c.Env.ContentPrefix, identity); err != nil {
		return err
	}
	if err := libconfig.SetFromEnv(&c.InputPrefix, c.Env.InputPrefix, identity); err != nil {
		return err
	}
	if err := libconfig.SetDurationFromEnv(&c.RenewAge, c.Env.RenewAge); err != nil {
		return err
	}
	for _, p := range []struct{ key, value string }{
		{"content_prefix", *c.ContentPrefix},
		{"input_prefix", *c.InputPrefix},
	} {
		if p.value == "/" || !strings.HasSuffix(p.value, "/") {
			return fmt.Errorf("%s must be a name ending in a slash, got %q", p.key, p.value)
		}
	}
	if strings.HasPrefix(*c.ContentPrefix, *c.InputPrefix) || strings.HasPrefix(*c.InputPrefix, *c.ContentPrefix) {
		return fmt.Errorf("content_prefix %q and input_prefix %q overlap", *c.ContentPrefix, *c.InputPrefix)
	}
	if *c.RenewAge < 0 {
		return fmt.Errorf("renew_age must not be negative, got %s", c.RenewAge)
	}
	return nil
}
