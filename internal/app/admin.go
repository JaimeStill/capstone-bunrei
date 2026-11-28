package app

import (
	"fmt"

	docinferpg "github.com/standards-lab/docinfer/postgres"
	docprocpg "github.com/standards-lab/docproc/postgres"
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-database/admin"
	msgpg "github.com/standards-lab/go-messaging/postgres"
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	pgdialect "github.com/standards-lab/sqlate/postgres"
	"github.com/standards-lab/sqlate/query"
)

func defineAdmin(g *graph.Graph, n *Nodes) {
	n.Schema = g.Define("schema", n.newSchema)
}

func (n *Nodes) newSchema(s *graph.Scope) (*admin.Service, error) {
	pool := s.Use(n.Database)
	logger := s.Use(n.Logger)

	sets := make([]migrate.Set, 0, 3)
	for _, source := range []struct {
		name string
		set  func() (migrate.Set, error)
	}{
		{"docproc", docprocpg.Migrations},
		{"docinfer", docinferpg.Migrations},
		{"messaging", msgpg.Migrations},
	} {
		set, err := source.set()
		if err != nil {
			return nil, fmt.Errorf("%s migrations: %w", source.name, err)
		}
		sets = append(sets, set)
	}

	session := sqlate.Wrap(pool.Conn(), pgdialect.Dialect{})
	migrator, err := migrate.New(session, sets, migrate.Options{Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	catalog := query.MustCatalog(query.Patterns())
	return admin.New(pool, session, migrator, catalog, admin.Options{Logger: logger}), nil
}

func mountAdmin(s *graph.Scope, n *Nodes) *web.Group {
	return web.NewGroup("/admin")
}
