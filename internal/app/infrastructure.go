package app

import (
	"log/slog"

	docstorage "github.com/standards-lab/docproc/storage"
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-core/logging"
	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-database/postgres"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/s3"
)

func defineInfrastructure(g *graph.Graph, n *Nodes) {
	n.LifecycleConfig = g.Define("lifecycle config", n.newLifecycleConfig)
	n.Logger = g.Define("logger", n.newLogger)
	n.Database = g.Define("database", n.newDatabase)
	n.Storage = g.Define("storage", n.newStorage)
	n.Content = g.Define("content", n.newContent)
	n.Input = g.Define("input", n.newInput)
}

func (n *Nodes) newLifecycleConfig(s *graph.Scope) (lifecycle.Config, error) {
	return s.Use(n.Config).Config, nil
}

func (n *Nodes) newLogger(s *graph.Scope) (*slog.Logger, error) {
	return logging.New(s.Use(n.Writer), s.Use(n.Config).Log), nil
}

func (n *Nodes) newDatabase(s *graph.Scope) (*database.DB, error) {
	return postgres.New(s.Use(n.Config).Database)
}

func (n *Nodes) newStorage(s *graph.Scope) (*storage.Store, error) {
	cfg := s.Use(n.Config).Storage
	client, err := s3.New(cfg)
	if err != nil {
		return nil, err
	}
	return storage.New(client, cfg), nil
}

func (n *Nodes) newContent(s *graph.Scope) (*docstorage.Content, error) {
	objects := s.Use(n.Config).Objects
	return docstorage.New(docstorage.Config{
		Client: s.Use(n.Storage),
		Prefix: *objects.ContentPrefix,
		Renew:  objects.RenewAge.Duration(),
	})
}

func (n *Nodes) newInput(s *graph.Scope) (*docstorage.Content, error) {
	objects := s.Use(n.Config).Objects
	return docstorage.New(docstorage.Config{
		Client: s.Use(n.Storage),
		Prefix: *objects.InputPrefix,
		Renew:  objects.RenewAge.Duration(),
	})
}
