package app

import (
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
	"github.com/JaimeStill/capstone-bunrei/domain/uploads"
)

func defineDomain(g *graph.Graph, n *Nodes) {
	n.Jobs = g.Define("jobs", n.newJobs, graph.After(n.WorkQueue, n.Schema))
	n.Uploads = g.Define("uploads", n.newUploads, graph.After(n.Schema))
}

func (n *Nodes) newJobs(s *graph.Scope) (*jobs.Service, error) {
	return jobs.New(s.Use(n.JobJournal), jobs.Options{
		Prefix: s.Use(n.Config).NATS.Prefix,
	}), nil
}

func (n *Nodes) newUploads(s *graph.Scope) (*uploads.Service, error) {
	return uploads.New(s.Use(n.SQL), s.Use(n.Storage), *s.Use(n.Config).Objects.UploadPrefix), nil
}

func mountAPI(s *graph.Scope, n *Nodes) *web.Group {
	return web.NewGroup("/api")
}
