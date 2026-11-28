package app

import (
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
	"github.com/JaimeStill/capstone-bunrei/domain/outputs"
	"github.com/JaimeStill/capstone-bunrei/domain/submissions"
	"github.com/JaimeStill/capstone-bunrei/domain/uploads"
)

func defineDomain(g *graph.Graph, n *Nodes) {
	n.Jobs = g.Define("jobs", n.newJobs, graph.After(n.WorkQueue, n.Schema))
	n.Uploads = g.Define("uploads", n.newUploads, graph.After(n.Schema))
	n.Submissions = g.Define("submissions", n.newSubmissions, graph.After(n.Schema))
	n.Outputs = g.Define("outputs", n.newOutputs, graph.After(n.Schema))
}

func (n *Nodes) newJobs(s *graph.Scope) (*jobs.Service, error) {
	return jobs.New(s.Use(n.JobJournal), jobs.Options{
		Prefix: s.Use(n.Config).NATS.Prefix,
	}), nil
}

func (n *Nodes) newUploads(s *graph.Scope) (*uploads.Service, error) {
	return uploads.New(s.Use(n.SQL), s.Use(n.Storage), *s.Use(n.Config).Objects.UploadPrefix), nil
}

func (n *Nodes) newSubmissions(s *graph.Scope) (*submissions.Service, error) {
	cfg := s.Use(n.Config)
	policy := cfg.Submissions
	return submissions.New(s.Use(n.SQL), s.Use(n.Input), s.Use(n.Jobs), s.Use(n.WorkQueue),
		*policy.DocumentLimit, policy.Retention.Duration(), cfg.Uploads.Lifetime.Duration()), nil
}

func (n *Nodes) newOutputs(s *graph.Scope) (*outputs.Service, error) {
	return outputs.New(s.Use(n.Content)), nil
}

func mountAPI(s *graph.Scope, n *Nodes) *web.Group {
	cfg, logger := s.Use(n.Config), s.Use(n.Logger)
	api := web.NewGroup("/api")
	submit := s.Use(n.Submissions)
	api.Mount(submissions.Routes(submit, cfg.Server.Transfer, logger))
	api.Mount(jobs.Routes(s.Use(n.Jobs), logger))
	api.Mount(outputs.Routes(s.Use(n.Outputs), cfg.Server.Transfer, logger))
	api.Mount(uploads.Routes(s.Use(n.Uploads), submit.Complete, cfg.Server.Transfer(*cfg.Uploads.ChunkSize),
		cfg.Uploads.Lifetime.Duration(), logger))
	api.Mount(submissions.UploadRoutes(submit, cfg.Server.Transfer, logger))
	return api
}
