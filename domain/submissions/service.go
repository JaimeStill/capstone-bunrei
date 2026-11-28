package submissions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"
	"uuid"

	docstorage "github.com/standards-lab/docproc/storage"
	"github.com/standards-lab/go-core/event"
	"github.com/standards-lab/go-messaging"
	"github.com/standards-lab/go-web-sdk/upload"
	"github.com/standards-lab/sqlate"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
)

// ExtractionLimit is the most bytes a Submit's or a CompleteUpload's three extracts may marshal
// to together, so the queued event that carries them stays within NATS's 1 MB max_payload.
const ExtractionLimit = 256 << 10

const maxID = 255

var errQueued = errors.New("the job is already queued")

// Service is Submit and the upload's two steps, Complete and CompleteUpload: it stages a
// document under its SHA-256, appends the job's queued event, and puts the job on the work
// queue, idempotently per the caller's ID or per the upload.
type Service struct {
	store     *store
	input     *docstorage.Content
	jobs      *jobs.Service
	queue     messaging.Queue
	limit     int64
	retention time.Duration
	lifetime  time.Duration
}

// New constructs the service over db, the session the submission and completed_upload tables
// are migrated in; input, the store a job's input is staged in by its SHA-256; root, the job
// root the queued event is appended through; and queue, the work queue. limit is the
// small-document limit in bytes, retention how long a submission record holds its ID, and
// lifetime how long a completed upload's record stays from its completion. Construction
// compiles the statements and performs no I/O. A nil dependency, or a limit, retention, or
// lifetime that is not positive, is a wiring defect and panics.
func New(db *sqlate.DB, input *docstorage.Content, root *jobs.Service, queue messaging.Queue, limit int64, retention, lifetime time.Duration) *Service {
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
	case lifetime <= 0:
		panic(fmt.Sprintf("submissions: lifetime must be positive, got %s", lifetime))
	}
	return &Service{store: newStore(db), input: input, jobs: root, queue: queue, limit: limit, retention: retention, lifetime: lifetime}
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
	data, want, err := minted(queued)
	if err != nil {
		return client.Job{}, fmt.Errorf("submit %q: %w", req.ID, err)
	}
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
	return s.enqueue(ctx, held.Job, data)
}

// Complete is the upload handler's completion of f: it stages content, the whole file, under
// f's SHA-256 in the input store, then records the completed upload under f's ID with the
// file's SHA-256, name, media type, and size, expiring a lifetime from now. It is idempotent on
// f.ID: of two completions of one upload, each stages the same bytes under the same key, and the
// second's record finds the first's in place and leaves it.
func (s *Service) Complete(ctx context.Context, f upload.File, content io.Reader) error {
	if err := s.input.Put(ctx, f.Digest, content); err != nil {
		return fmt.Errorf("stage upload %s: %w", f.ID, err)
	}
	q := client.Queued{Name: f.Name, MediaType: f.MediaType, Size: f.Length, SHA256: f.Digest}
	return s.store.recordCompleted(ctx, f.ID, q, time.Now().Add(s.lifetime))
}

// CompleteUpload queues the completed upload req names as a job, with req's profile and
// extracts, and returns the job. It refuses a request missing its profile, or whose upload is
// not a UUID, with ErrInvalid; extracts that marshal past [ExtractionLimit] together with
// ErrExtractionTooLarge; and an upload with no unexpired completed record with
// ErrUploadNotFound.
//
// The job's queued event has Submit's shape, its input the completed upload's staged file. The
// first CompleteUpload of an upload records a new UUIDv7 job and a digest of the request on the
// upload's record, then appends the job's queued event and enqueues its work event as Submit
// does. A CompleteUpload repeating the upload with the same profile and extracts returns that
// job, doing again the append and the enqueue, which find their work done; with a different
// profile or extracts it is ErrConflict. Of concurrent CompleteUploads of one upload, one
// records its job and the others read it.
func (s *Service) CompleteUpload(ctx context.Context, req client.CompleteUpload) (client.Job, error) {
	id, err := uuid.Parse(req.Upload)
	if err != nil {
		return client.Job{}, fmt.Errorf("%w: upload %q is not a UUID", ErrInvalid, req.Upload)
	}
	x := extracts{req.Extract, req.Document, req.Timeline}
	if err := options(req.Profile, x); err != nil {
		return client.Job{}, err
	}
	key := id.String()
	c, err := s.store.completedUpload(ctx, key)
	if err != nil {
		return client.Job{}, err
	}
	queued := client.Queued{
		Profile:   req.Profile,
		Name:      c.Name,
		MediaType: c.MediaType,
		Size:      c.Size,
		SHA256:    c.SHA256,
		Extract:   req.Extract,
		Document:  req.Document,
		Timeline:  req.Timeline,
	}
	data, want, err := minted(queued)
	if err != nil {
		return client.Job{}, fmt.Errorf("complete upload %s: %w", key, err)
	}
	held := want
	if c.Job != nil && c.Digest != nil {
		held = record{Job: *c.Job, Digest: *c.Digest}
	} else if held, err = s.store.assignJob(ctx, key, want); err != nil {
		return client.Job{}, err
	}
	if held.Digest != want.Digest {
		return client.Job{}, fmt.Errorf("%w: upload %s", ErrConflict, key)
	}
	return s.enqueue(ctx, held.Job, data)
}

func (s *Service) check(req client.Submit) (client.Queued, error) {
	switch {
	case req.ID == "":
		return client.Queued{}, fmt.Errorf("%w: the ID is required", ErrInvalid)
	case len(req.ID) > maxID:
		return client.Queued{}, fmt.Errorf("%w: the ID is %d bytes, past %d", ErrInvalid, len(req.ID), maxID)
	}
	if mediaType, _, err := mime.ParseMediaType(req.MediaType); err != nil || !strings.Contains(mediaType, "/") {
		return client.Queued{}, fmt.Errorf("%w: media type %q is not type/subtype", ErrInvalid, req.MediaType)
	}
	if n := int64(len(req.Content)); n > s.limit {
		return client.Queued{}, fmt.Errorf("%w: %d bytes, past %d", ErrContentTooLarge, n, s.limit)
	}
	if err := options(req.Profile, extracts{req.Extract, req.Document, req.Timeline}); err != nil {
		return client.Queued{}, err
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

func options(profile string, x extracts) error {
	if profile == "" {
		return fmt.Errorf("%w: the profile is required", ErrInvalid)
	}
	b, err := json.Marshal(x)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if len(b) > ExtractionLimit {
		return fmt.Errorf("%w: %d bytes, past %d", ErrExtractionTooLarge, len(b), ExtractionLimit)
	}
	return nil
}

func minted(q client.Queued) ([]byte, record, error) {
	data, err := json.Marshal(q)
	if err != nil {
		return nil, record{}, err
	}
	sum := sha256.Sum256(data)
	return data, record{Job: uuid.NewV7().String(), Digest: hex.EncodeToString(sum[:])}, nil
}

func (s *Service) enqueue(ctx context.Context, job string, data []byte) (client.Job, error) {
	j, err := s.append(ctx, job, data)
	if err != nil {
		return client.Job{}, err
	}
	if j.State == client.StateQueued {
		e, err := work(job, time.Now())
		if err == nil {
			err = s.queue.Enqueue(ctx, e)
		}
		if err != nil {
			return client.Job{}, fmt.Errorf("enqueue job %s: %w", job, err)
		}
	}
	return j.Job, nil
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
	return &Reclaimer{pass: s.store.reclaimExpired}
}

// UploadReclaimer returns the expired completed upload records' reclaimer over the service's
// store, which go-database's sweeper runs.
func (s *Service) UploadReclaimer() *Reclaimer {
	return &Reclaimer{pass: s.store.reclaimCompleted}
}

// Reclaimer is a reclaimer of expired records, a go-database sweep.Reclaimer, which it
// satisfies structurally: each pass deletes records of its table past their expiry, by the
// database's clock.
type Reclaimer struct {
	pass func(ctx context.Context, budget int) (int64, error)
}

// Pass deletes at most budget expired records, earliest expiry first, and reports more when it
// deleted budget of them, so a further pass may find more. A budget below 1 is an error.
func (r *Reclaimer) Pass(ctx context.Context, budget int) (more bool, err error) {
	n, err := r.pass(ctx, budget)
	return err == nil && n >= int64(budget), err
}
