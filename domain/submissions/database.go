package submissions

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
)

//go:embed statements/*.sql
var files embed.FS

const claims = 3

type record struct {
	Job    string `json:"job_id"`
	Digest string `json:"digest"`
}

type store struct {
	db      *sqlate.DB
	reclaim query.Statement
	create  query.Statement
	find    query.Rows[record]
	expired query.Statement
}

func newStore(db *sqlate.DB) *store {
	stmts := query.MustCatalog(query.Patterns()).MustCompile(files, "statements", db.Dialect())
	return &store{
		db:      db,
		reclaim: stmts.Statement("reclaim"),
		create:  stmts.Statement("create"),
		find:    stmts.Statement("find").Scan(query.Scanner[record]()),
		expired: stmts.Statement("reclaim_expired"),
	}
}

func (s *store) claim(ctx context.Context, id string, r record, expires time.Time) (record, error) {
	args := query.Args{"id": id, "job_id": r.Job, "digest": r.Digest, "expires_at": expires}
	for attempt := 1; ; attempt++ {
		_, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (int64, error) {
			if _, err := s.reclaim.Exec(ctx, tx, query.Args{"id": id}); err != nil {
				return 0, err
			}
			return s.create.Exec(ctx, tx, args)
		}, nil)
		if err == nil {
			return r, nil
		}
		if !errors.Is(err, sqlate.ErrUniqueViolation) {
			return record{}, fmt.Errorf("record submission %q: %w", id, err)
		}
		held, err := s.find.One(ctx, s.db, query.Args{"id": id})
		if err == nil {
			return held, nil
		}
		if !errors.Is(err, sql.ErrNoRows) || attempt == claims {
			return record{}, fmt.Errorf("read submission %q: %w", id, err)
		}
	}
}

func (s *store) reclaimExpired(ctx context.Context, budget int) (int64, error) {
	if budget < 1 {
		return 0, fmt.Errorf("submissions: budget must be at least 1, got %d", budget)
	}
	return s.expired.Exec(ctx, s.db, query.Args{"budget": budget})
}
