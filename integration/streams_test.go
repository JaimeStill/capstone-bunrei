//go:build integration

package integration_test

import (
	"context"
	"slices"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/JaimeStill/capstone-bunrei/integration"
)

func TestStreams_ProvisionedAsConfigured(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	assertReady(t, s.Client())

	nc, err := natsgo.Connect(integration.NATSURL())
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}

	for _, want := range []jetstream.StreamConfig{
		{
			Name:      "BUNREI_JOBS",
			Subjects:  []string{"bunrei.job.>"},
			Storage:   jetstream.FileStorage,
			Retention: jetstream.LimitsPolicy,
			MaxAge:    30 * 24 * time.Hour,
		},
		{
			Name:      "BUNREI_WORK",
			Subjects:  []string{"bunrei.work", "bunrei.work.dead", "bunrei.work.dead.>"},
			Storage:   jetstream.FileStorage,
			Retention: jetstream.WorkQueuePolicy,
			MaxAge:    0,
		},
	} {
		t.Run(want.Name, func(t *testing.T) {
			stream, err := js.Stream(context.Background(), want.Name)
			if err != nil {
				t.Fatalf("stream %s: %v", want.Name, err)
			}
			got := stream.CachedInfo().Config
			if !slices.Equal(got.Subjects, want.Subjects) {
				t.Errorf("subjects = %q, want %q", got.Subjects, want.Subjects)
			}
			if got.Storage != want.Storage {
				t.Errorf("storage = %s, want %s", got.Storage, want.Storage)
			}
			if got.Retention != want.Retention {
				t.Errorf("retention = %s, want %s", got.Retention, want.Retention)
			}
			if got.MaxAge != want.MaxAge {
				t.Errorf("max_age = %s, want %s", got.MaxAge, want.MaxAge)
			}
		})
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}
