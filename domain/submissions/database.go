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

	"github.com/JaimeStill/capstone-bunrei/client"
)

//go:embed statements/*.sql
var files embed.FS

const claims = 3

type record struct {
	Job    string `json:"job_id"`
	Digest string `json:"digest"`
}

type completedRow struct {
	SHA256    string  `json:"sha256"`
	Name      string  `json:"name"`
	MediaType string  `json:"media_type"`
	Size      int64   `json:"size"`
	Job       *string `json:"job_id"`
	Digest    *string `json:"digest"`
}

type store struct {
	db               *sqlate.DB
	reclaim          query.Statement
	create           query.Statement
	find             query.Rows[record]
	expired          query.Statement
	complete         query.Statement
	completed        query.Rows[completedRow]
	assign           query.Returning[completedRow]
	expiredCompleted query.Statement
}

func newStore(db *sqlate.DB) *store {
	stmts := query.MustCatalog(query.Patterns()).MustCompile(files, "statements", db.Dialect())
	scan := query.Scanner[completedRow]()
	return &store{
		db:               db,
		reclaim:          stmts.Statement("reclaim"),
		create:           stmts.Statement("create"),
		find:             stmts.Statement("find").Scan(query.Scanner[record]()),
		expired:          stmts.Statement("reclaim_expired"),
		complete:         stmts.Statement("complete"),
		completed:        stmts.Statement("completed").Scan(scan),
		assign:           stmts.Statement("assign").Returning(scan),
		expiredCompleted: stmts.Statement("reclaim_completed"),
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

func (s *store) recordCompleted(ctx context.Context, id string, q client.Queued, expires time.Time) error {
	args := query.Args{
		"id":         id,
		"sha256":     q.SHA256,
		"name":       q.Name,
		"media_type": q.MediaType,
		"size":       q.Size,
		"expires_at": expires,
	}
	_, err := s.complete.Exec(ctx, s.db, args)
	if err != nil && !errors.Is(err, sqlate.ErrUniqueViolation) {
		return fmt.Errorf("record completed upload %q: %w", id, err)
	}
	return nil
}

func (s *store) completedUpload(ctx context.Context, id string) (completedRow, error) {
	c, err := s.completed.One(ctx, s.db, query.Args{"id": id})
	return c, uploadMissing(id, err)
}

func (s *store) assignJob(ctx context.Context, id string, r record) (record, error) {
	c, _, err := s.assign.One(ctx, s.db, query.Args{"id": id, "job_id": r.Job, "digest": r.Digest})
	if err != nil {
		return record{}, uploadMissing(id, err)
	}
	if c.Job == nil || c.Digest == nil {
		return record{}, fmt.Errorf("completed upload %q: no job recorded", id)
	}
	return record{Job: *c.Job, Digest: *c.Digest}, nil
}

func uploadMissing(id string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: %q", ErrUploadNotFound, id)
	}
	return fmt.Errorf("read completed upload %q: %w", id, err)
}

func (s *store) reclaimExpired(ctx context.Context, budget int) (int64, error) {
	return pass(ctx, s.db, s.expired, budget)
}

func (s *store) reclaimCompleted(ctx context.Context, budget int) (int64, error) {
	return pass(ctx, s.db, s.expiredCompleted, budget)
}

func pass(ctx context.Context, db *sqlate.DB, stmt query.Statement, budget int) (int64, error) {
	if budget < 1 {
		return 0, fmt.Errorf("submissions: budget must be at least 1, got %d", budget)
	}
	return stmt.Exec(ctx, db, query.Args{"budget": budget})
}
