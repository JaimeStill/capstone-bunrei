package jobs_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/standards-lab/docinfer/job"
	"github.com/standards-lab/go-core/event"
	"github.com/standards-lab/go-messaging"
	"github.com/standards-lab/go-messaging/memory"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
)

var stamped = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

func newService(j messaging.Journal) *jobs.Service {
	n := 0
	return jobs.New(j, jobs.Options{
		IDs:   func() string { n++; return fmt.Sprintf("event-%d", n) },
		Clock: func() time.Time { return stamped },
	})
}

func jobID() string { return uuid.NewV7().String() }

func data(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}

func emit(e event.Event) jobs.Decide {
	return func(jobs.Job) (event.Event, error) { return e, nil }
}

func mustAppend(t *testing.T, s *jobs.Service, id string, e event.Event) jobs.Appended {
	t.Helper()
	a, err := s.Append(context.Background(), id, emit(e))
	if err != nil {
		t.Fatalf("Append %s: %v", e.Type, err)
	}
	return a
}

func queued(t *testing.T) event.Event {
	return event.Event{Type: jobs.TypeQueued, Data: data(t, client.Queued{
		Profile: "standard@1", Name: "report.pdf", MediaType: "application/pdf", Size: 4096, SHA256: "ab12",
	})}
}

func started(t *testing.T, attempt int) event.Event {
	return event.Event{Type: jobs.TypeStarted, Data: data(t, client.Started{Attempt: attempt, Replica: "bunrei-0"})}
}

func batch(digest string, chunks int) event.Event {
	return event.Event{
		Type: jobs.TypeBatch,
		Data: []byte(`"never decoded"`),
		Extensions: map[string]string{
			jobs.ExtBatchDigest: digest,
			jobs.ExtBatchChunks: fmt.Sprint(chunks),
		},
	}
}

func raw(typ string) event.Event {
	return event.Event{Type: typ, Data: []byte(`{}`)}
}

func TestLoad_NotFound(t *testing.T) {
	_, _, err := newService(memory.New()).Load(context.Background(), jobID())
	if !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("Load = %v, want ErrNotFound", err)
	}
}

func TestLoad_InvalidID(t *testing.T) {
	s := newService(memory.New())
	if _, _, err := s.Load(context.Background(), "a.b"); !errors.Is(err, jobs.ErrInvalidID) {
		t.Errorf("Load = %v, want ErrInvalidID", err)
	}
	if _, err := s.Append(context.Background(), "a.b", emit(raw(jobs.TypeQueued))); !errors.Is(err, jobs.ErrInvalidID) {
		t.Errorf("Append = %v, want ErrInvalidID", err)
	}
}

func TestLoad_UnknownType(t *testing.T) {
	b := memory.New()
	id := jobID()
	e := event.Event{ID: "x", Source: jobs.Source, Type: "bunrei.job.paused", Subject: id}
	if _, err := b.Append(context.Background(), "bunrei.job."+id, 0, e); err != nil {
		t.Fatalf("Append: %v", err)
	}
	_, _, err := newService(b).Load(context.Background(), id)
	if !errors.Is(err, jobs.ErrUnknownType) || !strings.Contains(err.Error(), "bunrei.job.paused") {
		t.Fatalf("Load = %v, want ErrUnknownType naming bunrei.job.paused", err)
	}
}

func TestAppend_UnknownTypeAppendsNothing(t *testing.T) {
	s := newService(memory.New())
	id := jobID()
	mustAppend(t, s, id, queued(t))
	_, err := s.Append(context.Background(), id, emit(raw("bunrei.job.paused")))
	if !errors.Is(err, jobs.ErrUnknownType) || !strings.Contains(err.Error(), "bunrei.job.paused") {
		t.Fatalf("Append = %v, want ErrUnknownType naming bunrei.job.paused", err)
	}
	if j, _, _ := s.Load(context.Background(), id); j.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", j.Sequence)
	}
}

func TestFold_EachType(t *testing.T) {
	s := newService(memory.New())
	id := jobID()
	ctx := context.Background()

	steps := []struct {
		event event.Event
		check func(jobs.Job) error
		only  bool
	}{
		{event: queued(t), check: func(j jobs.Job) error {
			want := client.Job{ID: id, Profile: "standard@1", Name: "report.pdf", Size: 4096, SHA256: "ab12", State: client.StateQueued, Sequence: 1}
			if !reflect.DeepEqual(j.Job, want) {
				return fmt.Errorf("job = %+v, want %+v", j.Job, want)
			}
			return nil
		}},
		{event: started(t, 1), check: func(j jobs.Job) error {
			if j.State != client.StateRunning || j.Attempt != 1 || len(j.Attempts) != 1 {
				return fmt.Errorf("state %s attempt %d entries %d, want running, 1, 1", j.State, j.Attempt, len(j.Attempts))
			}
			return nil
		}},
		{event: raw(jobs.TypePart), only: true},
		{event: raw(jobs.TypeOutput), only: true},
		{event: raw(jobs.TypeWarning), only: true},
		{event: raw(jobs.TypeInference), only: true},
		{event: event.Event{Type: jobs.TypeCheckpoint, Data: data(t, job.Checkpoint{After: 7})}, check: func(j jobs.Job) error {
			if j.Checkpoint != 7 {
				return fmt.Errorf("checkpoint = %d, want 7", j.Checkpoint)
			}
			return nil
		}},
		{event: event.Event{Type: jobs.TypeCheckpoint, Data: data(t, job.Checkpoint{After: 9})}, check: func(j jobs.Job) error {
			if j.Checkpoint != 9 {
				return fmt.Errorf("checkpoint = %d, want the last, 9", j.Checkpoint)
			}
			return nil
		}},
		{event: event.Event{Type: jobs.TypeAttemptFailed, Data: data(t, client.AttemptFailed{Attempt: 1, Error: "timeout"})}, check: func(j jobs.Job) error {
			if j.State != client.StateQueued || j.Error != "timeout" {
				return fmt.Errorf("state %s error %q, want queued, timeout", j.State, j.Error)
			}
			return nil
		}},
		{event: started(t, 2), check: func(j jobs.Job) error {
			if j.State != client.StateRunning || j.Attempt != 2 || len(j.Attempts) != 2 {
				return fmt.Errorf("state %s attempt %d entries %d, want running, 2, 2", j.State, j.Attempt, len(j.Attempts))
			}
			return nil
		}},
		{event: event.Event{Type: jobs.TypeCancelRequested}, check: func(j jobs.Job) error {
			if !j.CancelRequested || j.State != client.StateRunning {
				return fmt.Errorf("cancel requested %t state %s, want true, running", j.CancelRequested, j.State)
			}
			return nil
		}},
	}
	for i, step := range steps {
		before, _, _ := s.Load(ctx, id)
		a := mustAppend(t, s, id, step.event)
		loaded, _, err := s.Load(ctx, id)
		if err != nil {
			t.Fatalf("Load after %s: %v", step.event.Type, err)
		}
		if !reflect.DeepEqual(loaded, a.Job) {
			t.Errorf("%s: Load = %+v, Appended.Job = %+v", step.event.Type, loaded, a.Job)
		}
		if loaded.Sequence != uint64(i+1) {
			t.Errorf("%s: Sequence = %d, want %d", step.event.Type, loaded.Sequence, i+1)
		}
		if step.only {
			before.Sequence++
			if !reflect.DeepEqual(loaded, before) {
				t.Errorf("%s: job = %+v, want only the sequence advanced from %+v", step.event.Type, loaded, before)
			}
			continue
		}
		if err := step.check(loaded); err != nil {
			t.Errorf("%s: %v", step.event.Type, err)
		}
	}
}

func TestFold_Terminal(t *testing.T) {
	manifest := client.Manifest{Batches: 0, Chunks: 0, Digest: "e3b0", Attempts: []client.Attempt{{}}}
	cases := []struct {
		event event.Event
		state client.State
		check func(jobs.Job) error
	}{
		{event.Event{Type: jobs.TypeCompleted, Data: data(t, manifest)}, client.StateCompleted, func(j jobs.Job) error {
			if j.Manifest == nil || !reflect.DeepEqual(*j.Manifest, manifest) {
				return fmt.Errorf("manifest = %+v, want %+v", j.Manifest, manifest)
			}
			return nil
		}},
		{event.Event{Type: jobs.TypeFailed, Data: data(t, client.Failed{Attempt: 1, Error: "boom"})}, client.StateFailed, func(j jobs.Job) error {
			if j.Error != "boom" {
				return fmt.Errorf("error = %q, want boom", j.Error)
			}
			return nil
		}},
		{event.Event{Type: jobs.TypeCancelled}, client.StateCancelled, func(jobs.Job) error { return nil }},
	}
	for _, tc := range cases {
		t.Run(tc.event.Type, func(t *testing.T) {
			s := newService(memory.New())
			id := jobID()
			mustAppend(t, s, id, queued(t))
			mustAppend(t, s, id, started(t, 1))
			mustAppend(t, s, id, tc.event)
			j, _, err := s.Load(context.Background(), id)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if j.State != tc.state || !j.Ended() || j.Sequence != 3 {
				t.Errorf("state %s ended %t sequence %d, want %s, true, 3", j.State, j.Ended(), j.Sequence, tc.state)
			}
			if err := tc.check(j); err != nil {
				t.Error(err)
			}
		})
	}
}

func digestOf(digests ...string) string {
	h := sha256.New()
	for _, d := range digests {
		h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(d))))
		h.Write([]byte(d))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestFold_BatchFromHeaders(t *testing.T) {
	s := newService(memory.New())
	id := jobID()
	mustAppend(t, s, id, queued(t))
	if got, want := mustAppend(t, s, id, started(t, 1)).Job.Results().Digest, digestOf(); got != want {
		t.Errorf("digest before any batch = %s, want %s", got, want)
	}
	mustAppend(t, s, id, batch("aa11", 3))
	mustAppend(t, s, id, batch("bb22", 4))
	mustAppend(t, s, id, event.Event{Type: jobs.TypeAttemptFailed, Data: data(t, client.AttemptFailed{Attempt: 1, Error: "x"})})
	mustAppend(t, s, id, started(t, 2))
	mustAppend(t, s, id, batch("cc33", 5))

	j, _, err := s.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := client.Manifest{Batches: 3, Chunks: 12, Digest: digestOf("aa11", "bb22", "cc33"), Attempts: []client.Attempt{{}, {}}}
	if got := j.Results(); !reflect.DeepEqual(got, want) {
		t.Errorf("Results = %+v, want %+v", got, want)
	}
	if j.Batches != 3 || j.Chunks != 12 {
		t.Errorf("Batches %d Chunks %d, want 3, 12", j.Batches, j.Chunks)
	}
}

func TestFold_BatchWithoutHeadersFails(t *testing.T) {
	s := newService(memory.New())
	id := jobID()
	mustAppend(t, s, id, queued(t))
	for _, e := range []event.Event{
		{Type: jobs.TypeBatch, Extensions: map[string]string{jobs.ExtBatchChunks: "1"}},
		{Type: jobs.TypeBatch, Extensions: map[string]string{jobs.ExtBatchDigest: "aa", jobs.ExtBatchChunks: "many"}},
	} {
		if _, err := s.Append(context.Background(), id, emit(e)); err == nil {
			t.Errorf("Append %v = nil, want an error", e.Extensions)
		}
	}
}

func TestFold_SequenceCountsTheJobsEventsOnly(t *testing.T) {
	b := memory.New()
	s := newService(b)
	id, other := jobID(), jobID()
	mustAppend(t, s, id, queued(t))
	mustAppend(t, s, other, queued(t))
	mustAppend(t, s, other, started(t, 1))
	a := mustAppend(t, s, id, started(t, 1))

	if a.Sequence != 4 || a.Event.Extensions[jobs.ExtSequence] != "2" || a.Job.Sequence != 2 {
		t.Errorf("stream sequence %d jobsequence %s Sequence %d, want 4, 2, 2", a.Sequence, a.Event.Extensions[jobs.ExtSequence], a.Job.Sequence)
	}
	j, last, err := s.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if j.Sequence != 2 || last != 4 {
		t.Errorf("Sequence %d last %d, want 2, 4", j.Sequence, last)
	}
}

func TestAppend_Stamps(t *testing.T) {
	b := memory.New()
	s := newService(b)
	id := jobID()
	q := mustAppend(t, s, id, queued(t))
	st := started(t, 1)
	st.Extensions = map[string]string{jobs.ExtCausation: q.Event.ID, jobs.ExtSequence: "99"}
	callerExt := st.Extensions
	mustAppend(t, s, id, st)
	mustAppend(t, s, id, event.Event{ID: "mine", Type: jobs.TypeCancelRequested})
	mustAppend(t, s, id, raw(jobs.TypePart))
	mustAppend(t, s, id, raw(jobs.TypeInference))

	if len(callerExt) != 2 || callerExt[jobs.ExtSequence] != "99" {
		t.Errorf("caller's extensions changed: %v", callerExt)
	}

	type want struct {
		id, layer, schema, contentType string
		ext                            map[string]string
	}
	wants := []want{
		{"event-1", "bunrei", "urn:standards-lab:bunrei:client:1", "application/json", nil},
		{"event-2", "bunrei", "urn:standards-lab:bunrei:client:1", "application/json", map[string]string{jobs.ExtCausation: "event-1"}},
		{"mine", "bunrei", "", "", nil},
		{"event-3", "docproc", "urn:standards-lab:docproc:dispatch:1", "application/json", nil},
		{"event-4", "docinfer", "urn:standards-lab:docinfer:1", "application/json", nil},
	}
	var got []event.Event
	for entry, err := range b.Read(context.Background(), "bunrei.job."+id, 0) {
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		got = append(got, entry.Event)
	}
	if len(got) != len(wants) {
		t.Fatalf("read %d events, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		e := got[i]
		ext := map[string]string{"correlationid": id, "jobsequence": fmt.Sprint(i + 1), "layer": w.layer}
		maps.Copy(ext, w.ext)
		if e.ID != w.id || e.Source != "bunrei" || e.Subject != id || !e.Time.Equal(stamped) ||
			e.DataSchema != w.schema || e.DataContentType != w.contentType || !reflect.DeepEqual(e.Extensions, ext) {
			t.Errorf("event %d (%s) = %+v; want id %s, source bunrei, subject %s, time %v, dataschema %q, datacontenttype %q, extensions %v",
				i+1, e.Type, e, w.id, id, stamped, w.schema, w.contentType, ext)
		}
	}
}

func TestAppend_DefaultIDIsUUIDv7(t *testing.T) {
	s := jobs.New(memory.New(), jobs.Options{})
	a, err := s.Append(context.Background(), jobID(), emit(queued(t)))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	u, err := uuid.Parse(a.Event.ID)
	if err != nil || u[6]>>4 != 7 {
		t.Errorf("id %q, want a UUIDv7 (%v)", a.Event.ID, err)
	}
	if a.Event.Time.IsZero() {
		t.Error("time is zero, want stamped")
	}
}

func TestService_Prefix(t *testing.T) {
	b := memory.New()
	s := jobs.New(b, jobs.Options{Prefix: "zzalt.job"})
	id := jobID()
	mustAppend(t, s, id, queued(t))
	mustAppend(t, s, id, started(t, 1))

	n := 0
	for _, err := range b.Read(context.Background(), "zzalt.job."+id, 0) {
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		n++
	}
	if n != 2 {
		t.Errorf("zzalt.job.%s holds %d events, want 2", id, n)
	}
	for _, err := range b.Read(context.Background(), "bunrei.job."+id, 0) {
		t.Fatalf("bunrei.job.%s holds an event (%v), want none", id, err)
	}
	j, last, err := s.Load(context.Background(), id)
	if err != nil || j.Sequence != 2 || last != 2 {
		t.Errorf("Load = sequence %d, last %d, %v; want 2, 2, nil", j.Sequence, last, err)
	}
}

type interceptor struct {
	*memory.Broker
	before func(n int) error
	after  func(n int) error
	calls  int
}

func (i *interceptor) Append(ctx context.Context, subject string, expect uint64, e event.Event) (uint64, error) {
	i.calls++
	if i.before != nil {
		if err := i.before(i.calls); err != nil {
			return 0, err
		}
	}
	seq, err := i.Broker.Append(ctx, subject, expect, e)
	if err == nil && i.after != nil {
		if err := i.after(i.calls); err != nil {
			return 0, err
		}
	}
	return seq, err
}

func otherWriter(t *testing.T, b *memory.Broker, id string, e event.Event) {
	t.Helper()
	ctx := context.Background()
	var last uint64
	for entry, err := range b.Read(ctx, "bunrei.job."+id, 0) {
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		last = entry.Sequence
	}
	e.ID, e.Source, e.Subject = "other-"+e.Type, jobs.Source, id
	if _, err := b.Append(ctx, "bunrei.job."+id, last, e); err != nil {
		t.Fatalf("other writer: %v", err)
	}
}

func TestAppend_ConflictRefoldsAndDecidesAgain(t *testing.T) {
	j := &interceptor{Broker: memory.New()}
	s := newService(j)
	id := jobID()
	mustAppend(t, s, id, queued(t))

	j.calls = 0
	j.before = func(n int) error {
		if n == 1 {
			otherWriter(t, j.Broker, id, started(t, 1))
		}
		return nil
	}
	var seen []jobs.Job
	a, err := s.Append(context.Background(), id, func(job jobs.Job) (event.Event, error) {
		seen = append(seen, job)
		return event.Event{Type: jobs.TypeCheckpoint, Data: data(t, map[string]uint64{"after": uint64(job.Sequence)})}, nil
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("decided %d times, want 2", len(seen))
	}
	if seen[0].Sequence != 1 || seen[1].Sequence != 2 || seen[1].State != client.StateRunning {
		t.Errorf("decided on sequences %d, %d (state %s), want 1, then 2 running", seen[0].Sequence, seen[1].Sequence, seen[1].State)
	}
	if a.Sequence != 3 || a.Event.Extensions[jobs.ExtSequence] != "3" || a.Job.Sequence != 3 || a.Job.Checkpoint != 2 {
		t.Errorf("appended at %d, jobsequence %s, Sequence %d, Checkpoint %d; want 3, 3, 3, 2",
			a.Sequence, a.Event.Extensions[jobs.ExtSequence], a.Job.Sequence, a.Job.Checkpoint)
	}
}

func TestAppend_ConflictWithAnEndingRefuses(t *testing.T) {
	j := &interceptor{Broker: memory.New()}
	s := newService(j)
	id := jobID()
	mustAppend(t, s, id, queued(t))
	j.calls = 0
	j.before = func(n int) error {
		if n == 1 {
			otherWriter(t, j.Broker, id, event.Event{Type: jobs.TypeCancelled})
		}
		return nil
	}
	decided := 0
	_, err := s.Append(context.Background(), id, func(jobs.Job) (event.Event, error) {
		decided++
		return started(t, 1), nil
	})
	if !errors.Is(err, jobs.ErrEnded) || decided != 1 {
		t.Errorf("Append = %v after %d decisions, want ErrEnded after 1", err, decided)
	}
}

func TestAppend_RefusalAppendsNothing(t *testing.T) {
	b := memory.New()
	s := newService(b)
	id := jobID()
	mustAppend(t, s, id, queued(t))
	refused := errors.New("refused")
	if _, err := s.Append(context.Background(), id, func(jobs.Job) (event.Event, error) { return event.Event{}, refused }); !errors.Is(err, refused) {
		t.Fatalf("Append = %v, want the decision's error", err)
	}
	j, last, err := s.Load(context.Background(), id)
	if err != nil || j.Sequence != 1 || last != 1 {
		t.Errorf("Load = sequence %d, last %d, %v; want 1, 1, nil", j.Sequence, last, err)
	}
}

func TestAppend_EndedJobRefused(t *testing.T) {
	for _, end := range []event.Event{
		{Type: jobs.TypeCompleted, Data: []byte(`{"batches":0,"chunks":0,"digest":"","attempts":[]}`)},
		{Type: jobs.TypeFailed, Data: []byte(`{"attempt":1,"error":"boom"}`)},
		{Type: jobs.TypeCancelled},
	} {
		t.Run(end.Type, func(t *testing.T) {
			s := newService(memory.New())
			id := jobID()
			mustAppend(t, s, id, queued(t))
			mustAppend(t, s, id, end)
			decided := false
			_, err := s.Append(context.Background(), id, func(jobs.Job) (event.Event, error) {
				decided = true
				return event.Event{Type: jobs.TypeCancelRequested}, nil
			})
			if !errors.Is(err, jobs.ErrEnded) || decided {
				t.Errorf("Append = %v, decided %t; want ErrEnded, not decided", err, decided)
			}
			if j, _, _ := s.Load(context.Background(), id); j.Sequence != 2 {
				t.Errorf("Sequence = %d, want 2", j.Sequence)
			}
		})
	}
}

var errLostAck = errors.New("ack lost")

func TestAppend_UnknownOutcomeRetryReturnsTheOriginalSequence(t *testing.T) {
	j := &interceptor{Broker: memory.New()}
	s := newService(j)
	id := jobID()
	mustAppend(t, s, id, queued(t))
	j.calls = 0
	j.after = func(n int) error {
		if n == 1 {
			return errLostAck
		}
		return nil
	}
	a := mustAppend(t, s, id, started(t, 1))
	if j.calls != 2 || a.Sequence != 2 || a.Job.Sequence != 2 {
		t.Errorf("calls %d, sequence %d, Sequence %d; want 2, 2, 2", j.calls, a.Sequence, a.Job.Sequence)
	}
	loaded, last, err := s.Load(context.Background(), id)
	if err != nil || loaded.Sequence != 2 || last != 2 {
		t.Errorf("Load = sequence %d, last %d, %v; want 2, 2, nil: stored once", loaded.Sequence, last, err)
	}
}

func TestAppend_UnknownOutcomeFollowedFindsItsEvent(t *testing.T) {
	j := &interceptor{Broker: memory.New()}
	s := newService(j)
	id := jobID()
	mustAppend(t, s, id, queued(t))
	j.calls = 0
	j.after = func(n int) error {
		if n == 1 {
			otherWriter(t, j.Broker, id, event.Event{Type: jobs.TypeCancelRequested})
			return errLostAck
		}
		return nil
	}
	decided := 0
	a, err := s.Append(context.Background(), id, func(jobs.Job) (event.Event, error) {
		decided++
		return started(t, 1), nil
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if decided != 1 || a.Sequence != 2 || a.Job.Sequence != 2 || a.Event.Type != jobs.TypeStarted {
		t.Errorf("decided %d, sequence %d, Sequence %d, type %s; want 1, 2, 2, started", decided, a.Sequence, a.Job.Sequence, a.Event.Type)
	}
	loaded, last, err := s.Load(context.Background(), id)
	if err != nil || loaded.Sequence != 3 || last != 3 {
		t.Errorf("Load = sequence %d, last %d, %v; want 3, 3, nil", loaded.Sequence, last, err)
	}
}

func TestAppend_GivesUpOnAPersistentError(t *testing.T) {
	j := &interceptor{Broker: memory.New()}
	s := newService(j)
	id := jobID()
	j.before = func(int) error { return errLostAck }
	if _, err := s.Append(context.Background(), id, emit(queued(t))); !errors.Is(err, errLostAck) {
		t.Fatalf("Append = %v, want the journal's error", err)
	}
	if j.calls != 4 {
		t.Errorf("calls = %d, want 4: the append and 3 retries", j.calls)
	}
}

func events(t *testing.T, b *memory.Broker, id string) []event.Event {
	t.Helper()
	var got []event.Event
	for entry, err := range b.Read(context.Background(), "bunrei.job."+id, 0) {
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		got = append(got, entry.Event)
	}
	return got
}

func types(es []event.Event) []string {
	var ts []string
	for _, e := range es {
		ts = append(ts, e.Type)
	}
	return ts
}

func TestGet_ReturnsTheFoldedJob(t *testing.T) {
	s := newService(memory.New())
	id := jobID()
	mustAppend(t, s, id, queued(t))
	a := mustAppend(t, s, id, started(t, 1))

	j, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(j, a.Job.Job) || j.State != client.StateRunning || j.Sequence != 2 {
		t.Errorf("Get = %+v, want %+v, running at sequence 2", j, a.Job.Job)
	}
	upper, err := s.Get(context.Background(), strings.ToUpper(id))
	if err != nil || upper.ID != id {
		t.Errorf("Get of the uppercase ID = %+v, %v; want the job under its canonical ID", upper, err)
	}
}

func TestGet_UnknownAndInvalidIDs(t *testing.T) {
	s := newService(memory.New())
	if _, err := s.Get(context.Background(), jobID()); !errors.Is(err, jobs.ErrNotFound) {
		t.Errorf("Get of an unknown job = %v, want ErrNotFound", err)
	}
	for _, id := range []string{"", "a.b", "not-a-uuid", "bunrei.job.>"} {
		if _, err := s.Get(context.Background(), id); !errors.Is(err, jobs.ErrInvalidID) {
			t.Errorf("Get(%q) = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestCancel_AQueuedJobEndsCancelledCausedByTheRequest(t *testing.T) {
	b := memory.New()
	s := newService(b)
	id := jobID()
	mustAppend(t, s, id, queued(t))

	if err := s.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got := events(t, b, id)
	want := []string{jobs.TypeQueued, jobs.TypeCancelRequested, jobs.TypeCancelled}
	if !reflect.DeepEqual(types(got), want) {
		t.Fatalf("events = %v, want %v", types(got), want)
	}
	if _, ok := got[1].Extensions[jobs.ExtCausation]; ok {
		t.Errorf("cancel_requested carries causationid %q, want none", got[1].Extensions[jobs.ExtCausation])
	}
	if c := got[2].Extensions[jobs.ExtCausation]; c != got[1].ID || c == "" {
		t.Errorf("cancelled causationid = %q, want cancel_requested's id %q", c, got[1].ID)
	}
	j, err := s.Get(context.Background(), id)
	if err != nil || j.State != client.StateCancelled || j.Sequence != 3 {
		t.Errorf("Get = %+v, %v; want cancelled at sequence 3", j, err)
	}
}

func TestCancel_AnEndedJobIsRefused(t *testing.T) {
	for _, end := range []event.Event{
		{Type: jobs.TypeCompleted, Data: []byte(`{"batches":0,"chunks":0,"digest":"","attempts":[]}`)},
		{Type: jobs.TypeFailed, Data: []byte(`{"attempt":1,"error":"boom"}`)},
		{Type: jobs.TypeCancelled},
	} {
		t.Run(end.Type, func(t *testing.T) {
			b := memory.New()
			s := newService(b)
			id := jobID()
			mustAppend(t, s, id, queued(t))
			mustAppend(t, s, id, end)
			if err := s.Cancel(context.Background(), id); !errors.Is(err, jobs.ErrEnded) {
				t.Errorf("Cancel = %v, want ErrEnded", err)
			}
			if n := len(events(t, b, id)); n != 2 {
				t.Errorf("the job holds %d events, want 2: nothing appended", n)
			}
		})
	}
}

func TestCancel_ASecondCancelIsRefused(t *testing.T) {
	s := newService(memory.New())
	id := jobID()
	mustAppend(t, s, id, queued(t))
	if err := s.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := s.Cancel(context.Background(), id); !errors.Is(err, jobs.ErrEnded) {
		t.Errorf("the second Cancel = %v, want ErrEnded", err)
	}
}

func TestCancel_UnknownAndInvalidIDs(t *testing.T) {
	b := memory.New()
	s := newService(b)
	id := jobID()
	if err := s.Cancel(context.Background(), id); !errors.Is(err, jobs.ErrNotFound) {
		t.Errorf("Cancel of an unknown job = %v, want ErrNotFound", err)
	}
	if n := len(events(t, b, id)); n != 0 {
		t.Errorf("the unknown job holds %d events, want none", n)
	}
	if err := s.Cancel(context.Background(), "a.b"); !errors.Is(err, jobs.ErrInvalidID) {
		t.Errorf("Cancel(%q) = %v, want ErrInvalidID", "a.b", err)
	}
}

func TestCancel_ARunningJobOnlyRecordsTheRequest(t *testing.T) {
	b := memory.New()
	s := newService(b)
	id := jobID()
	mustAppend(t, s, id, queued(t))
	mustAppend(t, s, id, started(t, 1))

	for range 2 {
		if err := s.Cancel(context.Background(), id); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
	}
	want := []string{jobs.TypeQueued, jobs.TypeStarted, jobs.TypeCancelRequested}
	if got := types(events(t, b, id)); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v: one request, the job left running", got, want)
	}
}

func TestCancel_ARequestedJobBackInTheQueueEndsCancelledByTheFirstRequest(t *testing.T) {
	b := memory.New()
	s := newService(b)
	id := jobID()
	mustAppend(t, s, id, queued(t))
	mustAppend(t, s, id, started(t, 1))
	if err := s.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	mustAppend(t, s, id, event.Event{Type: jobs.TypeAttemptFailed, Data: data(t, client.AttemptFailed{Attempt: 1, Error: "x"})})

	if err := s.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got := events(t, b, id)
	want := []string{jobs.TypeQueued, jobs.TypeStarted, jobs.TypeCancelRequested, jobs.TypeAttemptFailed, jobs.TypeCancelled}
	if !reflect.DeepEqual(types(got), want) {
		t.Fatalf("events = %v, want %v", types(got), want)
	}
	if c := got[4].Extensions[jobs.ExtCausation]; c != got[2].ID {
		t.Errorf("cancelled causationid = %q, want the request's id %q", c, got[2].ID)
	}
}

func TestCancel_StartedLandingFirstLeavesTheJobRunning(t *testing.T) {
	j := &interceptor{Broker: memory.New()}
	s := newService(j)
	id := jobID()
	mustAppend(t, s, id, queued(t))
	j.calls = 0
	j.before = func(n int) error {
		if n == 2 {
			otherWriter(t, j.Broker, id, started(t, 1))
		}
		return nil
	}

	if err := s.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	want := []string{jobs.TypeQueued, jobs.TypeCancelRequested, jobs.TypeStarted}
	if got := types(events(t, j.Broker, id)); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v: the started append wins and no cancelled follows", got, want)
	}
	loaded, _, err := s.Load(context.Background(), id)
	if err != nil || loaded.State != client.StateRunning || !loaded.CancelRequested {
		t.Errorf("Load = state %s, cancel requested %t, %v; want running with the request recorded", loaded.State, loaded.CancelRequested, err)
	}
}
