package submissions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"
	"uuid"

	docstorage "github.com/standards-lab/docproc/storage"
	"github.com/standards-lab/go-core/event"
	"github.com/standards-lab/go-messaging"
	"github.com/standards-lab/sqlate"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
)

// ExtractionLimit is the most bytes a Submit's three extracts may marshal to together, so the
// queued event that carries them stays within NATS's 1 MB max_payload.
const ExtractionLimit = 256 << 10

const maxID = 255

var errQueued = errors.New("the job is already queued")

// Service is Submit: it stages a whole document under its SHA-256, appends the job's queued
// event, and puts the job on the work queue, idempotently per the caller's ID.
type Service struct {
	store     *store
	input     *docstorage.Content
	jobs      *jobs.Service
	queue     messaging.Queue
	limit     int64
	retention time.Duration
}

// New constructs the service over db, the session the submission table is migrated in; input,
// the store a job's input is staged in by its SHA-256; root, the job root the queued event is
// appended through; and queue, the work queue. limit is the small-document limit in bytes, and
// retention how long a submission record holds its ID. Construction compiles the statements and
// performs no I/O. A nil dependency, or a limit or retention that is not positive, is a wiring
// defect and panics.
func New(db *sqlate.DB, input *docstorage.Content, root *jobs.Service, queue messaging.Queue, limit int64, retention time.Duration) *Service {
	switch {
	case db == nil:
		panic("submissions: nil db")
	case input == nil:
		panic("submissions: nil input")
	case root == nil:
		panic("submissions: nil job root")
	case queue == nil:
		panic("submissions: nil queue")
	case limit <= 0:
		panic(fmt.Sprintf("submissions: limit must be positive, got %d", limit))
	case retention <= 0:
		panic(fmt.Sprintf("submissions: retention must be positive, got %s", retention))
	}
	return &Service{store: newStore(db), input: input, jobs: root, queue: queue, limit: limit, retention: retention}
}

// Limit returns the small-document limit, the most bytes a submitted document may decode to.
func (s *Service) Limit() int64 { return s.limit }

// Submit queues req's document as a job and returns the job. It refuses a request missing its ID
// or profile, with an ID past 255 bytes, or whose media type is not type/subtype with
// ErrInvalid; a document past the small-document limit with ErrContentTooLarge; and extracts
// that marshal past [ExtractionLimit] together with ErrExtractionTooLarge.
//
// The first Submit under an ID records the ID with a new UUIDv7 job and a digest of the request:
// its profile, name, media type, content's SHA-256, and extracts. It then stages the content
// under its SHA-256, appends the job's queued event, and enqueues the job's work event, whose
// id is the job's ID. A Submit repeating the ID with the same request returns that job, doing
// again each step: the staging and the append find their work done, and the enqueue, made only
// while the job is still queued, is stored once within the queue's deduplication window. The
// same ID with a different request is ErrConflict. Of concurrent Submits under one ID, one
// records it and the others read its record, and the job root's append, guarded by the job's
// sequence, appends one queued event. A record expires after the retention, and a Submit under
// an expired record's ID records a new job.
func (s *Service) Submit(ctx context.Context, req client.Submit) (client.Job, error) {
	queued, err := s.check(req)
	if err != nil {
		return client.Job{}, err
	}
	data, err := json.Marshal(queued)
	if err != nil {
		return client.Job{}, fmt.Errorf("submit %q: %w", req.ID, err)
	}
	sum := sha256.Sum256(data)
	want := record{Job: uuid.NewV7().String(), Digest: hex.EncodeToString(sum[:])}
	held, err := s.store.claim(ctx, req.ID, want, time.Now().Add(s.retention))
	if err != nil {
		return client.Job{}, err
	}
	if held.Digest != want.Digest {
		return client.Job{}, fmt.Errorf("%w: %q", ErrConflict, req.ID)
	}
	if err := s.input.Put(ctx, queued.SHA256, bytes.NewReader(req.Content)); err != nil {
		return client.Job{}, fmt.Errorf("stage job %s: %w", held.Job, err)
	}
	j, err := s.append(ctx, held.Job, data)
	if err != nil {
		return client.Job{}, err
	}
	if j.State == client.StateQueued {
		e, err := work(held.Job, time.Now())
		if err == nil {
			err = s.queue.Enqueue(ctx, e)
		}
		if err != nil {
			return client.Job{}, fmt.Errorf("enqueue job %s: %w", held.Job, err)
		}
	}
	return j.Job, nil
}

func (s *Service) check(req client.Submit) (client.Queued, error) {
	switch {
	case req.ID == "":
		return client.Queued{}, fmt.Errorf("%w: the ID is required", ErrInvalid)
	case len(req.ID) > maxID:
		return client.Queued{}, fmt.Errorf("%w: the ID is %d bytes, past %d", ErrInvalid, len(req.ID), maxID)
	case req.Profile == "":
		return client.Queued{}, fmt.Errorf("%w: the profile is required", ErrInvalid)
	}
	if mediaType, _, err := mime.ParseMediaType(req.MediaType); err != nil || !strings.Contains(mediaType, "/") {
		return client.Queued{}, fmt.Errorf("%w: media type %q is not type/subtype", ErrInvalid, req.MediaType)
	}
	if n := int64(len(req.Content)); n > s.limit {
		return client.Queued{}, fmt.Errorf("%w: %d bytes, past %d", ErrContentTooLarge, n, s.limit)
	}
	x, err := json.Marshal(extracts{req.Extract, req.Document, req.Timeline})
	if err != nil {
		return client.Queued{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if len(x) > ExtractionLimit {
		return client.Queued{}, fmt.Errorf("%w: %d bytes, past %d", ErrExtractionTooLarge, len(x), ExtractionLimit)
	}
	sum := sha256.Sum256(req.Content)
	return client.Queued{
		Profile:   req.Profile,
		Name:      req.Name,
		MediaType: req.MediaType,
		Size:      int64(len(req.Content)),
		SHA256:    hex.EncodeToString(sum[:]),
		Extract:   req.Extract,
		Document:  req.Document,
		Timeline:  req.Timeline,
	}, nil
}

func (s *Service) append(ctx context.Context, id string, data []byte) (jobs.Job, error) {
	a, err := s.jobs.Append(ctx, id, func(j jobs.Job) (event.Event, error) {
		if j.Sequence > 0 {
			return event.Event{}, errQueued
		}
		return event.Event{Type: jobs.TypeQueued, Data: data}, nil
	})
	if errors.Is(err, errQueued) || errors.Is(err, jobs.ErrEnded) {
		j, _, err := s.jobs.Load(ctx, id)
		return j, err
	}
	return a.Job, err
}

// Reclaimer returns the expired submission records' reclaimer over the service's store, which
// go-database's sweeper runs.
func (s *Service) Reclaimer() *Reclaimer {
	return &Reclaimer{store: s.store}
}

// Reclaimer is the expired submission records' reclaimer, a go-database sweep.Reclaimer, which
// it satisfies structurally: each pass deletes records past their expiry, by the database's
// clock.
type Reclaimer struct {
	store *store
}

// Pass deletes at most budget expired submission records, earliest expiry first, and reports
// more when it deleted budget of them, so a further pass may find more. A budget below 1 is an
// error.
func (r *Reclaimer) Pass(ctx context.Context, budget int) (more bool, err error) {
	n, err := r.store.reclaimExpired(ctx, budget)
	return err == nil && n >= int64(budget), err
}
