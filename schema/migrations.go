package schema

import (
	"embed"
	"fmt"

	"github.com/standards-lab/sqlate/migrate"
)

// Source is the name Bunrei's migration set is recorded under.
const Source = "bunrei"

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations returns Bunrei's migration set, recorded in migrate's
// default history table. A layout the migrate package refuses returns its
// error.
func Migrations() (migrate.Set, error) {
	files, err := migrate.Files(migrationFiles, "migrations")
	if err != nil {
		return migrate.Set{}, fmt.Errorf("schema: %w", err)
	}
	return migrate.Set{Name: Source, Migrations: files}, nil
}
