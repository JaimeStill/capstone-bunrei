package jobs

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"time"
	"uuid"

	"github.com/standards-lab/go-core/event"
	"github.com/standards-lab/go-messaging"

	"github.com/JaimeStill/capstone-bunrei/client"
)

const retries = 3

var (
	errRequested = errors.New("the job's cancellation is already requested")
	errNotQueued = errors.New("the job is not queued")
)

// Service is the job root: it loads a job by folding its events from the journal, and appends
// the event a caller decides from the folded job, guarded by the subject's last stream sequence.
type Service struct {
	journal messaging.Journal
	prefix  string
	ids     func() string
	now     func() time.Time
}

// Options configures a [Service]. The zero value keeps jobs under bunrei.job, mints version 7
// UUIDs, and stamps the time from time.Now.
type Options struct {
	// Prefix is the subject prefix a job's ID follows, joined by a dot; empty uses bunrei.job.
	Prefix string
	// IDs mints an event id; nil mints a version 7 UUID.
	IDs func() string
	// Clock returns the time stamped on an event that has none; nil uses time.Now.
	Clock func() time.Time
}

// New constructs the job root over journal, the journal of the stream that captures the subject
// prefix. Each field set in opts replaces its default. Construction performs no I/O.
func New(journal messaging.Journal, opts Options) *Service {
	s := &Service{journal: journal, prefix: opts.Prefix, ids: opts.IDs, now: opts.Clock}
	if s.prefix == "" {
		s.prefix = "bunrei.job"
	}
	if s.ids == nil {
		s.ids = func() string { return uuid.NewV7().String() }
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// Load reads the job's subject in order and returns the job its events fold to, and the stream
// sequence of the subject's last event. A job with no events is ErrNotFound; an event of a type
// that is none of the job's 13 fails the load with ErrUnknownType, naming the type.
func (s *Service) Load(ctx context.Context, id string) (Job, uint64, error) {
	if err := checkID(id); err != nil {
		return Job{}, 0, err
	}
	j, last, _, err := s.read(ctx, Job{ID: id}, 0, "")
	if err != nil {
		return Job{}, 0, err
	}
	if j.Sequence == 0 {
		return Job{}, 0, fmt.Errorf("job %s: %w", id, ErrNotFound)
	}
	return j, last, nil
}

// Append loads the job, asks decide for its next event, stamps it, and appends it with the
// job's last stream sequence as the expected one. A job with no events folds to its zero state,
// its ID set, so decide can start it. An ended job is ErrEnded, and decide is not asked; an error
// from decide is returned as it is, and nothing is appended, as with an event the job's fold
// cannot apply.
//
// The stamp sets the id when the event has none, the time when it has none, source to [Source],
// subject and correlationid to the job's ID, jobsequence to the folded count plus one, and layer
// to the type's; on an event with data, datacontenttype to application/json and dataschema to its
// layer's URN. A type that is none of the job's 13 is ErrUnknownType.
//
// When another writer appended first, Append reads on from the expected sequence, folds what it
// finds, and asks decide again. An append whose outcome is unknown is retried, up to three times,
// with the same event and expected sequence, which returns the stored event's sequence while it is
// the subject's last; when an event has followed it, Append finds it as it reads on and returns
// it.
func (s *Service) Append(ctx context.Context, id string, decide Decide) (Appended, error) {
	if err := checkID(id); err != nil {
		return Appended{}, err
	}
	j, last, _, err := s.read(ctx, Job{ID: id}, 0, "")
	if err != nil {
		return Appended{}, err
	}
	for {
		if j.Ended() {
			return Appended{}, fmt.Errorf("job %s, %s: %w", id, j.State, ErrEnded)
		}
		e, err := decide(j)
		if err != nil {
			return Appended{}, err
		}
		if e, err = s.stamp(j, e); err != nil {
			return Appended{}, err
		}
		folded, err := j.apply(e)
		if err != nil {
			return Appended{}, fmt.Errorf("append to job %s: %w", id, err)
		}
		seq, unknown, err := s.append(ctx, last, e)
		if err == nil {
			return Appended{Event: e, Sequence: seq, Job: folded}, nil
		}
		if !errors.Is(err, messaging.ErrConflict) {
			return Appended{}, fmt.Errorf("append to job %s: %w", id, err)
		}
		pending := ""
		if unknown {
			pending = e.ID
		}
		var found *messaging.Entry
		if j, last, found, err = s.read(ctx, j, last, pending); err != nil {
			return Appended{}, err
		}
		if found != nil {
			return Appended{Event: found.Event, Sequence: found.Sequence, Job: j}, nil
		}
	}
}

func (s *Service) append(ctx context.Context, last uint64, e event.Event) (uint64, bool, error) {
	unknown := false
	for attempt := 0; ; attempt++ {
		seq, err := s.journal.Append(ctx, s.subject(e.Subject), last, e)
		if err == nil || errors.Is(err, messaging.ErrConflict) || event.IsPermanent(err) || ctx.Err() != nil || attempt == retries {
			return seq, unknown, err
		}
		unknown = true
	}
}

func (s *Service) read(ctx context.Context, j Job, after uint64, pending string) (Job, uint64, *messaging.Entry, error) {
	for entry, err := range s.journal.Read(ctx, s.subject(j.ID), after) {
		if err != nil {
			return Job{}, 0, nil, fmt.Errorf("read job %s: %w", j.ID, err)
		}
		if j, err = j.apply(entry.Event); err != nil {
			return Job{}, 0, nil, fmt.Errorf("job %s at stream sequence %d: %w", j.ID, entry.Sequence, err)
		}
		after = entry.Sequence
		if pending != "" && entry.Event.Source == Source && entry.Event.ID == pending {
			return j, after, &entry, nil
		}
	}
	return j, after, nil, nil
}

func (s *Service) stamp(j Job, e event.Event) (event.Event, error) {
	layer, ok := layers[e.Type]
	if !ok {
		return event.Event{}, fmt.Errorf("append to job %s: %w: %q", j.ID, ErrUnknownType, e.Type)
	}
	if e.ID == "" {
		e.ID = s.ids()
	}
	if e.Time.IsZero() {
		e.Time = s.now()
	}
	e.Source, e.Subject = Source, j.ID
	ext := maps.Clone(e.Extensions)
	if ext == nil {
		ext = map[string]string{}
	}
	ext[ExtCorrelation] = j.ID
	ext[ExtSequence] = strconv.FormatUint(j.Sequence+1, 10)
	ext[ExtLayer] = layer
	e.Extensions = ext
	e.DataContentType, e.DataSchema = "", ""
	if len(e.Data) > 0 {
		e.DataContentType, e.DataSchema = "application/json", schemas[layer]
	}
	return e, nil
}

func checkID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return nil
}

// Get returns the job id's events fold to, as GetJob answers it. id is a UUID, read in its
// canonical form; any other is ErrInvalidID, and a job with no events is ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (client.Job, error) {
	id, err := canonical(id)
	if err != nil {
		return client.Job{}, err
	}
	j, _, err := s.Load(ctx, id)
	return j.Job, err
}

// Cancel is CancelJob: it requests the job's cancellation, and ends the job while it is queued.
// id is read as [Service.Get] reads it; a job with no events is ErrNotFound, and an ended one
// ErrEnded.
//
// Cancel first appends cancel_requested, unless the job already holds one. Then, while the job
// is still queued, it appends cancelled, its causationid the cancel_requested event's id. Each
// append is guarded by the job's last stream sequence, so a started event that lands first
// wins: the job stays running with its cancellation requested, and Cancel returns nil, as it
// does when the job ends some other way between the two appends.
func (s *Service) Cancel(ctx context.Context, id string) error {
	id, err := canonical(id)
	if err != nil {
		return err
	}
	_, err = s.Append(ctx, id, func(j Job) (event.Event, error) {
		switch {
		case j.Sequence == 0:
			return event.Event{}, fmt.Errorf("job %s: %w", id, ErrNotFound)
		case j.CancelRequested:
			return event.Event{}, errRequested
		}
		return event.Event{Type: TypeCancelRequested}, nil
	})
	if err != nil && !errors.Is(err, errRequested) {
		return err
	}
	_, err = s.Append(ctx, id, func(j Job) (event.Event, error) {
		if j.State != client.StateQueued {
			return event.Event{}, errNotQueued
		}
		return event.Event{Type: TypeCancelled, Extensions: map[string]string{ExtCausation: j.cancelRequest}}, nil
	})
	if errors.Is(err, errNotQueued) || errors.Is(err, ErrEnded) {
		return nil
	}
	return err
}

func canonical(id string) (string, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return u.String(), nil
}
