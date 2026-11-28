package app

import (
	"fmt"

	docprocpg "github.com/standards-lab/docproc/postgres"
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-database/sweep"
	msgpg "github.com/standards-lab/go-messaging/postgres"

	"github.com/JaimeStill/capstone-bunrei/expiry"
)

func defineReactors(g *graph.Graph, n *Nodes) {
	n.Reactors = g.Group("reactors")
	n.Sweeper = g.Define("sweeper", n.newSweeper, graph.In(n.Reactors), graph.After(n.Schema))
}

func (n *Nodes) newSweeper(s *graph.Scope) (*sweep.Sweeper, error) {
	cfg := s.Use(n.Config)
	db := s.Use(n.SQL)
	store := s.Use(n.Storage)
	ages := cfg.Sweep

	index, err := docprocpg.NewRetention(db, ages.IndexAge.Duration())
	if err != nil {
		return nil, fmt.Errorf("index retention: %w", err)
	}
	outbox, err := msgpg.OutboxRetention(db, ages.OutboxAge.Duration())
	if err != nil {
		return nil, fmt.Errorf("outbox retention: %w", err)
	}
	inbox, err := msgpg.InboxRetention(db, ages.InboxAge.Duration())
	if err != nil {
		return nil, fmt.Errorf("inbox retention: %w", err)
	}
	content, err := expiry.New(store, *cfg.Objects.ContentPrefix, ages.ContentAge.Duration(), expiry.Options{})
	if err != nil {
		return nil, err
	}
	input, err := expiry.New(store, *cfg.Objects.InputPrefix, ages.InputAge.Duration(), expiry.Options{})
	if err != nil {
		return nil, err
	}
	uploadObjects, err := expiry.New(store, *cfg.Objects.UploadPrefix, cfg.Uploads.ObjectAge(), expiry.Options{})
	if err != nil {
		return nil, err
	}

	policy := sweep.Policy{Interval: ages.Interval.Duration(), Budget: *ages.Batch}
	regs := []sweep.Registration{
		{Name: "index", Reclaimer: index, Policy: policy},
		{Name: "outbox", Reclaimer: outbox, Policy: policy},
		{Name: "inbox", Reclaimer: inbox, Policy: policy},
		{Name: "content", Reclaimer: content, Policy: policy},
		{Name: "input", Reclaimer: input, Policy: policy},
		{Name: "uploads", Reclaimer: s.Use(n.Uploads).Reclaimer(), Policy: policy},
		{Name: "upload objects", Reclaimer: uploadObjects, Policy: policy},
	}
	return sweep.New(db, regs, cfg.ShutdownTimeout.Duration(), s.Use(n.Logger), sweep.Options{})
}
