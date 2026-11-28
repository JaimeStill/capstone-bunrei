package uploads

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
)

//go:embed statements/*.sql
var files embed.FS

type metaRow struct {
	MediaType string `json:"media_type"`
	Digest    string `json:"digest"`
	Name      string `json:"name"`
}

type progressRow struct {
	Offset    int64     `json:"upload_offset"`
	Length    *int64    `json:"upload_length"`
	State     []byte    `json:"hash_state"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (p progressRow) length() int64 {
	if p.Length == nil {
		return -1
	}
	return *p.Length
}

type store struct {
	db           *sqlate.DB
	create       query.Statement
	reclaim      query.Statement
	metaRows     query.Rows[metaRow]
	progressRows query.Rows[progressRow]
	setLen       query.Returning[progressRow]
	advance      query.Returning[progressRow]
	recordChunk  query.Statement
	listChunks   query.Rows[string]
	delete       query.Statement
	expired      query.Statement
}

func newStore(db *sqlate.DB) *store {
	stmts := query.MustCatalog(query.Patterns()).MustCompile(files, "statements", db.Dialect())
	scan := query.Scanner[progressRow]()
	return &store{
		db:           db,
		create:       stmts.Statement("create"),
		reclaim:      stmts.Statement("reclaim"),
		metaRows:     stmts.Statement("meta").Scan(query.Scanner[metaRow]()),
		progressRows: stmts.Statement("progress").Scan(scan),
		setLen:       stmts.Statement("set_length").Returning(scan),
		advance:      stmts.Statement("advance").Returning(scan),
		recordChunk:  stmts.Statement("record_chunk"),
		listChunks:   stmts.Statement("list_chunks").Scan(query.Scalar[string]),
		delete:       stmts.Statement("delete"),
		expired:      stmts.Statement("reclaim_expired"),
	}
}

func missing(op, id string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &fs.PathError{Op: op, Path: id, Err: fs.ErrNotExist}
	}
	return err
}

func (s *store) insert(ctx context.Context, id, mediaType, digest, name string, length int64, expires time.Time) error {
	args := query.Args{
		"id":            id,
		"media_type":    mediaType,
		"digest":        digest,
		"name":          name,
		"upload_length": nil,
		"expires_at":    expires,
	}
	if length >= 0 {
		args["upload_length"] = length
	}
	_, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (int64, error) {
		if _, err := s.reclaim.Exec(ctx, tx, query.Args{"id": id}); err != nil {
			return 0, err
		}
		return s.create.Exec(ctx, tx, args)
	}, nil)
	if errors.Is(err, sqlate.ErrUniqueViolation) {
		return &fs.PathError{Op: "create", Path: id, Err: fs.ErrExist}
	}
	return err
}

func (s *store) meta(ctx context.Context, id string) (metaRow, error) {
	m, err := s.metaRows.One(ctx, s.db, query.Args{"id": id})
	return m, missing("meta", id, err)
}

func (s *store) progress(ctx context.Context, id string) (progressRow, error) {
	p, err := s.progressRows.One(ctx, s.db, query.Args{"id": id})
	return p, missing("progress", id, err)
}

func (s *store) setLength(ctx context.Context, id string, length int64) (int64, error) {
	p, _, err := s.setLen.One(ctx, s.db, query.Args{"id": id, "upload_length": length})
	return p.length(), missing("setlength", id, err)
}

type committed struct {
	offset int64
	ok     bool
}

func (s *store) commit(ctx context.Context, id string, offset, size int64, state []byte, key string) (int64, bool, error) {
	c, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (committed, error) {
		p, changed, err := s.advance.One(ctx, tx, query.Args{
			"id":         id,
			"expected":   offset,
			"next":       offset + size,
			"hash_state": state,
		})
		if err != nil || !changed {
			return committed{offset: p.Offset}, err
		}
		if key != "" {
			args := query.Args{"upload_id": id, "chunk_offset": offset, "object_key": key}
			if _, err := s.recordChunk.Exec(ctx, tx, args); err != nil {
				return committed{}, err
			}
		}
		return committed{offset: p.Offset, ok: true}, nil
	}, nil)
	return c.offset, c.ok, missing("append", id, err)
}

func (s *store) chunks(ctx context.Context, id string) ([]string, error) {
	if _, err := s.progressRows.One(ctx, s.db, query.Args{"id": id}); err != nil {
		return nil, missing("open", id, err)
	}
	return s.listChunks.All(ctx, s.db, query.Args{"upload_id": id})
}

func (s *store) remove(ctx context.Context, id string) ([]string, error) {
	keys, err := s.db.Transact(ctx, func(tx *sqlate.Tx) ([]string, error) {
		keys, err := s.listChunks.All(ctx, tx, query.Args{"upload_id": id})
		if err != nil {
			return nil, err
		}
		n, err := s.delete.Exec(ctx, tx, query.Args{"id": id})
		if err == nil && n == 0 {
			err = sql.ErrNoRows
		}
		return keys, err
	}, nil)
	return keys, missing("delete", id, err)
}

func (s *store) reclaimExpired(ctx context.Context, budget int) (int64, error) {
	if budget < 1 {
		return 0, fmt.Errorf("uploads: budget must be at least 1, got %d", budget)
	}
	return s.expired.Exec(ctx, s.db, query.Args{"budget": budget})
}
