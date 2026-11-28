package config

import (
	"fmt"
	"strconv"
	"time"

	libconfig "github.com/standards-lab/go-core/config"
)

const (
	defaultUploadsLifetime  = 24 * time.Hour
	defaultUploadsChunkSize = 4 << 20
)

// UploadsConfig is the service-owned policy of the resumable uploads:
// Lifetime is how long an upload stays open from its creation, after
// which it is missing to every request and the sweeper reclaims its rows,
// and how long a completed upload's record stays from its completion,
// after which CompleteUpload no longer finds it and the sweeper reclaims
// it. ChunkSize is the most bytes one creation or append carries, the
// max-append-size the upload handler announces and enforces. The sweeper
// reclaims the objects under the objects block's upload prefix at twice
// the lifetime, so no object an open upload names is reclaimed. Pointer
// fields distinguish unset from zero.
type UploadsConfig struct {
	Lifetime  *libconfig.Duration `json:"lifetime"`
	ChunkSize *int64              `json:"chunk_size"`
	// Env records the environment-variable names Finalize composed and read.
	Env UploadsEnv `json:"-"`
}

// UploadsEnv holds the environment-variable names of the uploads block.
type UploadsEnv struct {
	Lifetime  string
	ChunkSize string
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
	if src.ChunkSize != nil {
		v := *src.ChunkSize
		c.ChunkSize = &v
	}
}

// Finalize applies the defaults, reads the block's environment overrides,
// and validates: the lifetime and the chunk size are positive. An empty
// prefix composes empty names, which disable the overrides.
func (c *UploadsConfig) Finalize(envPrefix string) error {
	if c.Lifetime == nil {
		c.Lifetime = new(libconfig.Duration(defaultUploadsLifetime))
	}
	if c.ChunkSize == nil {
		c.ChunkSize = new(int64(defaultUploadsChunkSize))
	}
	c.Env.Lifetime = libconfig.EnvName(envPrefix, "uploads_lifetime")
	c.Env.ChunkSize = libconfig.EnvName(envPrefix, "uploads_chunk_size")
	if err := libconfig.SetDurationFromEnv(&c.Lifetime, c.Env.Lifetime); err != nil {
		return err
	}
	parse := func(v string) (int64, error) { return strconv.ParseInt(v, 10, 64) }
	if err := libconfig.SetFromEnv(&c.ChunkSize, c.Env.ChunkSize, parse); err != nil {
		return err
	}
	if *c.Lifetime <= 0 {
		return fmt.Errorf("lifetime must be positive, got %s", c.Lifetime)
	}
	if *c.ChunkSize <= 0 {
		return fmt.Errorf("chunk_size must be positive, got %d", *c.ChunkSize)
	}
	return nil
}

// ObjectAge is the age past which the sweeper reclaims an object under the
// upload prefix: twice the lifetime.
func (c UploadsConfig) ObjectAge() time.Duration {
	return 2 * c.Lifetime.Duration()
}
