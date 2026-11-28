package app

import (
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
)

func defineDomain(g *graph.Graph, n *Nodes) {
	n.Jobs = g.Define("jobs", n.newJobs, graph.After(n.WorkQueue))
}

func (n *Nodes) newJobs(s *graph.Scope) (*jobs.Service, error) {
	return jobs.New(s.Use(n.JobJournal), jobs.Options{}), nil
}

func mountAPI(s *graph.Scope, n *Nodes) *web.Group {
	return web.NewGroup("/api")
}
