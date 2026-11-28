package config

import (
	"fmt"
	"strconv"
	"time"

	libconfig "github.com/standards-lab/go-core/config"
)

const (
	defaultSweepInterval   = 30 * time.Second
	defaultSweepBatch      = 100
	defaultSweepOutboxAge  = 7 * 24 * time.Hour
	defaultSweepInboxAge   = 7 * 24 * time.Hour
	defaultSweepIndexAge   = 30 * 24 * time.Hour
	defaultSweepContentAge = 67 * 24 * time.Hour
	defaultSweepInputAge   = 30 * 24 * time.Hour
)

// SweepConfig is the service-owned policy of the sweeper's reclaimers.
// Interval is how often each reclaimer runs, and Batch the budget each of
// its passes is given. OutboxAge is the age past which a published outbox
// row is deleted, and InboxAge the age past which a handled inbox row is;
// InboxAge must outlast every redelivery the broker can make. IndexAge is
// the age past which docproc's index drops an entry, whose content is
// prepared again on its next dispatch. ContentAge and InputAge are the
// ages past which an object under the content and input prefixes, unput
// for that long, is deleted; each must exceed the objects block's renew
// age, since an object in use is rewritten only once it is older than
// that, which [Config.Finalize] checks. Pointer fields distinguish unset
// from zero.
type SweepConfig struct {
	Interval   *libconfig.Duration `json:"interval"`
	Batch      *int                `json:"batch"`
	OutboxAge  *libconfig.Duration `json:"outbox_age"`
	InboxAge   *libconfig.Duration `json:"inbox_age"`
	IndexAge   *libconfig.Duration `json:"index_age"`
	ContentAge *libconfig.Duration `json:"content_age"`
	InputAge   *libconfig.Duration `json:"input_age"`
	// Env records the environment-variable names Finalize composed and read.
	Env SweepEnv `json:"-"`
}

// SweepEnv holds the environment-variable names of the sweep block.
type SweepEnv struct {
	Interval   string
	Batch      string
	OutboxAge  string
	InboxAge   string
	IndexAge   string
	ContentAge string
	InputAge   string
}

// Merge overlays src's set fields onto the receiver.
func (c *SweepConfig) Merge(src *SweepConfig) {
	if src == nil {
		return
	}
	if src.Batch != nil {
		v := *src.Batch
		c.Batch = &v
	}
	for _, d := range c.durations(src) {
		if *d.src != nil {
			v := **d.src
			*d.dst = &v
		}
	}
}

type sweepDuration struct {
	key      string
	dst, src **libconfig.Duration
	def      time.Duration
	env      *string
}

func (c *SweepConfig) durations(src *SweepConfig) []sweepDuration {
	if src == nil {
		src = &SweepConfig{}
	}
	return []sweepDuration{
		{"interval", &c.Interval, &src.Interval, defaultSweepInterval, &c.Env.Interval},
		{"outbox_age", &c.OutboxAge, &src.OutboxAge, defaultSweepOutboxAge, &c.Env.OutboxAge},
		{"inbox_age", &c.InboxAge, &src.InboxAge, defaultSweepInboxAge, &c.Env.InboxAge},
		{"index_age", &c.IndexAge, &src.IndexAge, defaultSweepIndexAge, &c.Env.IndexAge},
		{"content_age", &c.ContentAge, &src.ContentAge, defaultSweepContentAge, &c.Env.ContentAge},
		{"input_age", &c.InputAge, &src.InputAge, defaultSweepInputAge, &c.Env.InputAge},
	}
}

// Finalize applies the defaults, reads the block's environment overrides
// (<PREFIX>_SWEEP_ and the key, upper-cased), and validates: every
// duration positive and the batch at least 1, the bounds the sweeper and
// each reclaimer refuse outside. An empty prefix composes empty names,
// which disable the overrides.
func (c *SweepConfig) Finalize(envPrefix string) error {
	if c.Batch == nil {
		c.Batch = new(defaultSweepBatch)
	}
	c.Env.Batch = libconfig.EnvName(envPrefix, "sweep_batch")
	if err := libconfig.SetFromEnv(&c.Batch, c.Env.Batch, strconv.Atoi); err != nil {
		return err
	}
	for _, d := range c.durations(nil) {
		if *d.dst == nil {
			*d.dst = new(libconfig.Duration(d.def))
		}
		*d.env = libconfig.EnvName(envPrefix, "sweep_"+d.key)
		if err := libconfig.SetDurationFromEnv(d.dst, *d.env); err != nil {
			return err
		}
		if **d.dst <= 0 {
			return fmt.Errorf("%s must be positive, got %s", d.key, *d.dst)
		}
	}
	if *c.Batch < 1 {
		return fmt.Errorf("batch must be at least 1, got %d", *c.Batch)
	}
	return nil
}
