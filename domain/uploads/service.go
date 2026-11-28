package uploads

import (
	"context"
	"io"
	"time"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/sqlate"
)

// Service is the uploads store, go-web-sdk's upload.Store, its rows in
// the database and its chunks in blob storage. A missing upload, one
// deleted or past its expiry, is an *fs.PathError wrapping fs.ErrNotExist.
type Service struct {
	store *store
	blobs *blobs
}

// New constructs the service over db, the session the upload tables are
// migrated in, and objects, the blob storage its chunks are kept in under
// prefix. Construction compiles the statements and performs no I/O. A nil
// db or objects, or an empty prefix, is a wiring defect and panics.
func New(db *sqlate.DB, objects storage.Client, prefix string) *Service {
	if db == nil {
		panic("uploads: nil db")
	}
	if objects == nil {
		panic("uploads: nil objects")
	}
	if prefix == "" {
		panic("uploads: empty prefix")
	}
	return &Service{store: newStore(db), blobs: &blobs{client: objects, prefix: prefix}}
}

// Create records a new upload under id at offset 0 with no hash state,
// its length -1 while unknown. It first reclaims an expired upload under
// id; an unexpired one is an *fs.PathError wrapping fs.ErrExist.
func (s *Service) Create(ctx context.Context, id, mediaType, digest, name string, length int64, expires time.Time) error {
	return s.store.insert(ctx, id, mediaType, digest, name, length, expires)
}

// Meta returns the upload's media type, digest, and file name.
func (s *Service) Meta(ctx context.Context, id string) (mediaType, digest, name string, err error) {
	m, err := s.store.meta(ctx, id)
	return m.MediaType, m.Digest, m.Name, err
}

// Progress returns the upload's offset, its length or -1 while unknown,
// the hash state its last committed append stored (empty at offset 0),
// and its expiry.
func (s *Service) Progress(ctx context.Context, id string) (offset, length int64, state []byte, expires time.Time, err error) {
	p, err := s.store.progress(ctx, id)
	return p.Offset, p.length(), p.State, p.ExpiresAt, err
}

// SetLength records the upload's length when none is recorded and
// returns the length recorded, which is the earlier one when there is.
func (s *Service) SetLength(ctx context.Context, id string, length int64) (int64, error) {
	return s.store.setLength(ctx, id, length)
}

// Append commits chunk at offset only when the upload's offset is offset,
// and returns the upload's offset after the call and whether it
// committed. An append at an offset the upload has left is refused before
// it writes anything. Otherwise it writes chunk as an object under a key
// of its own, then stores offset+len(chunk), state, and the chunk row
// naming the object in one transaction, conditional on the offset. A
// refused append removes its object, best effort; a refusal is not an
// error.
func (s *Service) Append(ctx context.Context, id string, offset int64, chunk, state []byte) (current int64, committed bool, err error) {
	p, err := s.store.progress(ctx, id)
	if err != nil {
		return 0, false, err
	}
	if p.Offset != offset {
		return p.Offset, false, nil
	}
	var key string
	if len(chunk) > 0 {
		if key, err = s.blobs.put(ctx, id, offset, chunk); err != nil {
			return 0, false, err
		}
	}
	current, committed, err = s.store.commit(ctx, id, offset, int64(len(chunk)), state, key)
	if !committed && key != "" {
		s.blobs.remove(ctx, key)
	}
	return current, committed, err
}

// Open returns the chunks committed when it is called, in offset order,
// as one reader that opens one chunk's object at a time. A delete of the
// upload before the reader finishes may surface as fs.ErrNotExist from
// Read.
func (s *Service) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	keys, err := s.store.chunks(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.blobs.open(ctx, id, keys), nil
}

// Delete removes the upload and its chunk rows, then its chunks' objects,
// best effort: an object a failed removal leaves is reclaimed by the
// upload prefix's expiry.
func (s *Service) Delete(ctx context.Context, id string) error {
	keys, err := s.store.remove(ctx, id)
	if err != nil {
		return err
	}
	for _, key := range keys {
		s.blobs.remove(ctx, key)
	}
	return nil
}

// Reclaimer returns the expired uploads' reclaimer over the service's
// store, which go-database's sweeper runs.
func (s *Service) Reclaimer() *Reclaimer {
	return &Reclaimer{store: s.store}
}

// Reclaimer is the expired uploads' reclaimer, a go-database
// sweep.Reclaimer, which it satisfies structurally: each pass deletes
// expired uploads, by the database's clock, and by cascade their chunk
// rows. Their objects are left to the upload prefix's expiry.
type Reclaimer struct {
	store *store
}

// Pass deletes at most budget expired uploads, earliest expiry first, and
// their chunk rows, and reports more when it deleted budget of them, so a
// further pass may find more. A budget below 1 is an error.
func (r *Reclaimer) Pass(ctx context.Context, budget int) (more bool, err error) {
	n, err := r.store.reclaimExpired(ctx, budget)
	return err == nil && n >= int64(budget), err
}
