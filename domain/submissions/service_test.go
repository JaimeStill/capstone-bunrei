package submissions_test

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/standards-lab/docinfer"
	docstorage "github.com/standards-lab/docproc/storage"
	"github.com/standards-lab/go-core/event"
	"github.com/standards-lab/go-messaging"
	"github.com/standards-lab/go-messaging/memory"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/go-web-sdk/upload"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
	"github.com/JaimeStill/capstone-bunrei/domain/submissions"
)

const limit = 64

type recordingQueue struct {
	*memory.Queue
	mu     sync.Mutex
	events []event.Event
}

func (q *recordingQueue) Enqueue(ctx context.Context, e event.Event) error {
	q.mu.Lock()
	q.events = append(q.events, e)
	q.mu.Unlock()
	return q.Queue.Enqueue(ctx, e)
}

func (q *recordingQueue) enqueued() []event.Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.events)
}

func (q *recordingQueue) stored(t *testing.T) []event.Event {
	t.Helper()
	src, err := q.Subscribe(messaging.Worker{Name: "test", MaxDeliver: 1, DeadLetter: "bunrei.work.dead"}, nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	var mu sync.Mutex
	var got []event.Event
	if err := src.Receive(ctx, func(_ context.Context, e event.Event) error {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	return got
}

type journal struct {
	*memory.Broker
	before func(n int) error
	calls  int
}

func (j *journal) Append(ctx context.Context, subject string, expect uint64, e event.Event) (uint64, error) {
	j.calls++
	if j.before != nil {
		if err := j.before(j.calls); err != nil {
			return 0, err
		}
	}
	return j.Broker.Append(ctx, subject, expect, e)
}

type fixture struct {
	service *submissions.Service
	rec     *sqltest.Recorder
	objects *storagetest.Fake
	journal *journal
	root    *jobs.Service
	queue   *recordingQueue
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool, rec := sqltest.Open(t)
	objects := storagetest.NewFake(storagetest.Options{})
	input, err := docstorage.New(docstorage.Config{Client: objects, Prefix: "input/"})
	if err != nil {
		t.Fatalf("input store: %v", err)
	}
	j := &journal{Broker: memory.New()}
	root := jobs.New(j, jobs.Options{})
	q := &recordingQueue{Queue: memory.NewQueue()}
	return &fixture{
		service: submissions.New(sqlate.Wrap(pool, sqltest.Dialect{}), input, root, q, limit, time.Hour, time.Hour),
		rec:     rec,
		objects: objects,
		journal: j,
		root:    root,
		queue:   q,
	}
}

func (f *fixture) claims() {
	f.rec.Queue(sqltest.Response{Affected: 0}, sqltest.Response{Affected: 1})
}

func (f *fixture) held(job, digest string) {
	f.rec.Queue(
		sqltest.Response{Affected: 0},
		sqltest.Response{Err: fmt.Errorf("duplicate key: %w", sqlate.ErrUniqueViolation)},
		sqltest.Response{Columns: []string{"job_id", "digest"}, Rows: [][]driver.Value{{job, digest}}},
	)
}

func (f *fixture) record(t *testing.T) (job, digest string) {
	t.Helper()
	for _, c := range f.rec.Calls() {
		if strings.Contains(c.SQL, "INSERT INTO submission") {
			return c.Args[1].(string), c.Args[2].(string)
		}
	}
	t.Fatal("no submission record was inserted")
	return "", ""
}

func (f *fixture) events(t *testing.T, job string) []event.Event {
	t.Helper()
	var got []event.Event
	for entry, err := range f.journal.Read(t.Context(), "bunrei.job."+job, 0) {
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		got = append(got, entry.Event)
	}
	return got
}

func extract(prompt string) *docinfer.Extract {
	return &docinfer.Extract{Name: "invoice", Prompt: prompt, Schema: []byte(`{"type":"object"}`)}
}

func request() client.Submit {
	return client.Submit{
		ID:        "retry-" + uuid.NewV7().String(),
		Profile:   "standard@1",
		Name:      "invoice.json",
		MediaType: "application/json",
		Content:   []byte(`{"total":12.5}`),
		Extract:   extract("per source"),
		Document:  extract("whole document"),
		Timeline:  extract("timeline"),
	}
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestSubmit_QueuesTheJob(t *testing.T) {
	f := newFixture(t)
	f.claims()
	req := request()

	j, err := f.service.Submit(t.Context(), req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	id, _ := f.record(t)
	if u, err := uuid.Parse(j.ID); err != nil || u[6]>>4 != 7 || j.ID != id {
		t.Errorf("job ID %q (%v), want the UUIDv7 the record names, %s", j.ID, err, id)
	}
	sum := sha(req.Content)
	want := client.Job{ID: j.ID, Profile: req.Profile, Name: req.Name, Size: int64(len(req.Content)), SHA256: sum, State: client.StateQueued, Sequence: 1}
	if j != want {
		t.Errorf("job = %+v, want %+v", j, want)
	}

	r, err := f.objects.Get(t.Context(), "input/"+sum[:2]+"/"+sum, storage.GetOptions{})
	if err != nil {
		t.Fatalf("the staged input: %v", err)
	}
	staged, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if string(staged) != string(req.Content) {
		t.Errorf("staged %q, want the content", staged)
	}

	events := f.events(t, j.ID)
	if len(events) != 1 || events[0].Type != jobs.TypeQueued {
		t.Fatalf("events = %+v, want one queued", events)
	}
	var q client.Queued
	if err := json.Unmarshal(events[0].Data, &q); err != nil {
		t.Fatalf("queued data: %v", err)
	}
	wantQ := client.Queued{
		Profile: req.Profile, Name: req.Name, MediaType: req.MediaType, Size: int64(len(req.Content)), SHA256: sum,
		Extract: req.Extract, Document: req.Document, Timeline: req.Timeline,
	}
	if got, _ := json.Marshal(q); string(got) != string(must(json.Marshal(wantQ))) {
		t.Errorf("queued = %s, want %s", got, must(json.Marshal(wantQ)))
	}
	if _, ok := events[0].Extensions[jobs.ExtCausation]; ok {
		t.Errorf("queued carries a causationid, want none: a Command causes it")
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestSubmit_EnqueuesUnderAnIDDerivedFromTheJob(t *testing.T) {
	f := newFixture(t)
	f.claims()

	j, err := f.service.Submit(t.Context(), request())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	enqueued := f.queue.enqueued()
	if len(enqueued) != 1 {
		t.Fatalf("enqueued %d events, want 1", len(enqueued))
	}
	e := enqueued[0]
	if e.ID != j.ID || e.Source != jobs.Source || e.Type != jobs.TypeWork || e.Subject != j.ID {
		t.Errorf("work event id %q source %q type %q subject %q, want id and subject the job's %s, source %s, type %s",
			e.ID, e.Source, e.Type, e.Subject, j.ID, jobs.Source, jobs.TypeWork)
	}
	if string(e.Data) != `{"job":"`+j.ID+`"}` {
		t.Errorf("work data = %s, want {\"job\":%q}", e.Data, j.ID)
	}
}

func TestSubmit_Refusals(t *testing.T) {
	big := strings.Repeat("x", submissions.ExtractionLimit)
	for _, tc := range []struct {
		name string
		edit func(*client.Submit)
		want error
	}{
		{"an empty profile", func(r *client.Submit) { r.Profile = "" }, submissions.ErrInvalid},
		{"an empty ID", func(r *client.Submit) { r.ID = "" }, submissions.ErrInvalid},
		{"an ID past 255 bytes", func(r *client.Submit) { r.ID = strings.Repeat("i", 256) }, submissions.ErrInvalid},
		{"no media type", func(r *client.Submit) { r.MediaType = "" }, submissions.ErrInvalid},
		{"a media type with no subtype", func(r *client.Submit) { r.MediaType = "pdf" }, submissions.ErrInvalid},
		{"content past the limit", func(r *client.Submit) { r.Content = make([]byte, limit+1) }, submissions.ErrContentTooLarge},
		{"one extract past the cap", func(r *client.Submit) { r.Document = extract(big) }, submissions.ErrExtractionTooLarge},
		{"three extracts past the cap together", func(r *client.Submit) {
			third := strings.Repeat("x", submissions.ExtractionLimit/3)
			r.Extract, r.Document, r.Timeline = extract(third), extract(third), extract(third)
		}, submissions.ErrExtractionTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			req := request()
			tc.edit(&req)
			if _, err := f.service.Submit(t.Context(), req); !errors.Is(err, tc.want) {
				t.Fatalf("Submit = %v, want %v", err, tc.want)
			}
			if calls := f.rec.Calls(); len(calls) != 0 {
				t.Errorf("database calls = %v, want none for a refusal", calls)
			}
			if f.journal.calls != 0 || len(f.queue.enqueued()) != 0 {
				t.Errorf("appends %d, enqueues %d; want none for a refusal", f.journal.calls, len(f.queue.enqueued()))
			}
		})
	}
}

func TestSubmit_AcceptsContentAtTheLimitAndARecord(t *testing.T) {
	f := newFixture(t)
	f.claims()
	req := request()
	req.Content = []byte(`{"pad":"` + strings.Repeat("x", limit-10) + `"}`)
	if len(req.Content) != limit {
		t.Fatalf("content is %d bytes, want the limit, %d", len(req.Content), limit)
	}
	if _, err := f.service.Submit(t.Context(), req); err != nil {
		t.Fatalf("Submit of application/json content at the limit: %v", err)
	}
}

func TestSubmit_IdenticalRetryReturnsTheFirstJob(t *testing.T) {
	f := newFixture(t)
	f.claims()
	req := request()
	first, err := f.service.Submit(t.Context(), req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	f.held(f.record(t))

	again, err := f.service.Submit(t.Context(), req)
	if err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if again != first {
		t.Errorf("the retry returned %+v, want the first job %+v", again, first)
	}
	if events := f.events(t, first.ID); len(events) != 1 {
		t.Errorf("job %s holds %d events, want its one queued", first.ID, len(events))
	}
	if n := len(f.queue.enqueued()); n != 2 {
		t.Errorf("enqueued %d times, want the first and the retry's re-enqueue", n)
	}
	if stored := f.queue.stored(t); len(stored) != 1 || stored[0].ID != first.ID {
		t.Errorf("the queue holds %v, want the job's one work event", stored)
	}
}

func TestSubmit_DifferentRequestConflicts(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*client.Submit)
	}{
		{"profile", func(r *client.Submit) { r.Profile = "standard@2" }},
		{"name", func(r *client.Submit) { r.Name = "other.json" }},
		{"media type", func(r *client.Submit) { r.MediaType = "text/plain" }},
		{"content", func(r *client.Submit) { r.Content = []byte(`{"total":13}`) }},
		{"per-source extract", func(r *client.Submit) { r.Extract = nil }},
		{"document extract", func(r *client.Submit) { r.Document = extract("another") }},
		{"timeline extract", func(r *client.Submit) { r.Timeline = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.claims()
			req := request()
			first, err := f.service.Submit(t.Context(), req)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			f.held(f.record(t))
			tc.edit(&req)

			if _, err := f.service.Submit(t.Context(), req); !errors.Is(err, submissions.ErrConflict) {
				t.Fatalf("Submit with a different %s = %v, want ErrConflict", tc.name, err)
			}
			if events := f.events(t, first.ID); len(events) != 1 {
				t.Errorf("job %s holds %d events, want its one queued", first.ID, len(events))
			}
			if n := len(f.queue.enqueued()); n != 1 {
				t.Errorf("enqueued %d times, want the first Submit's alone", n)
			}
		})
	}
}

func TestSubmit_RetryCompletesTheStepsACrashLeftUndone(t *testing.T) {
	f := newFixture(t)
	f.claims()
	req := request()
	lost := errors.New("journal unavailable")
	f.journal.before = func(int) error { return event.Permanent(lost) }
	if _, err := f.service.Submit(t.Context(), req); !errors.Is(err, lost) {
		t.Fatalf("Submit = %v, want the journal's error", err)
	}
	job, digest := f.record(t)
	if n := len(f.queue.enqueued()); n != 0 {
		t.Fatalf("enqueued %d times before queued was appended, want none", n)
	}

	f.journal.before = nil
	f.held(job, digest)
	j, err := f.service.Submit(t.Context(), req)
	if err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if j.ID != job || j.State != client.StateQueued {
		t.Errorf("the retry returned job %s %s, want the recorded job %s queued", j.ID, j.State, job)
	}
	if events := f.events(t, job); len(events) != 1 || events[0].Type != jobs.TypeQueued {
		t.Errorf("events = %+v, want one queued", events)
	}
	if stored := f.queue.stored(t); len(stored) != 1 || stored[0].ID != job {
		t.Errorf("the queue holds %v, want the job's work event", stored)
	}
}

func TestSubmit_RetryOfAStartedJobEnqueuesNothing(t *testing.T) {
	f := newFixture(t)
	f.claims()
	req := request()
	first, err := f.service.Submit(t.Context(), req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := f.root.Append(t.Context(), first.ID, func(jobs.Job) (event.Event, error) {
		return event.Event{Type: jobs.TypeStarted, Data: []byte(`{"attempt":1,"replica":"bunrei-0","profile":{}}`)}, nil
	}); err != nil {
		t.Fatalf("Append started: %v", err)
	}
	f.held(f.record(t))

	j, err := f.service.Submit(t.Context(), req)
	if err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if j.ID != first.ID || j.State != client.StateRunning || j.Sequence != 2 {
		t.Errorf("the retry returned %+v, want job %s running at sequence 2", j, first.ID)
	}
	if n := len(f.queue.enqueued()); n != 1 {
		t.Errorf("enqueued %d times, want the first Submit's alone", n)
	}
}

func TestSubmit_ConcurrentSubmitsOfOneIDQueueOneJob(t *testing.T) {
	f := newFixture(t)
	f.claims()
	req := request()

	var other client.Job
	var otherErr error
	f.journal.before = func(n int) error {
		if n == 1 {
			f.held(f.record(t))
			other, otherErr = f.service.Submit(t.Context(), req)
		}
		return nil
	}
	j, err := f.service.Submit(t.Context(), req)
	if err != nil || otherErr != nil {
		t.Fatalf("Submits = %v, %v; want both to succeed", err, otherErr)
	}
	if j.ID != other.ID || j != other {
		t.Errorf("the Submits returned %+v and %+v, want one job", j, other)
	}
	if events := f.events(t, j.ID); len(events) != 1 || events[0].Type != jobs.TypeQueued {
		t.Errorf("events = %+v, want one queued", events)
	}
	if stored := f.queue.stored(t); len(stored) != 1 || stored[0].ID != j.ID {
		t.Errorf("the queue holds %v, want the job's one work event", stored)
	}
}

func TestReclaimer_PassDeletesAtMostTheBudgetEarliestExpiryFirst(t *testing.T) {
	for _, c := range []struct {
		name     string
		affected int64
		more     bool
	}{
		{"a full batch", 25, true},
		{"a short batch", 7, false},
		{"nothing expired", 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.rec.Queue(sqltest.Response{Affected: c.affected})
			more, err := f.service.Reclaimer().Pass(t.Context(), 25)
			if err != nil {
				t.Fatal(err)
			}
			if more != c.more {
				t.Errorf("more = %t after deleting %d of 25, want %t", more, c.affected, c.more)
			}
			calls := f.rec.Calls()
			if len(calls) != 1 || calls[0].Op != sqltest.OpExec {
				t.Fatalf("calls = %v, want one delete", calls)
			}
			for _, want := range []string{"DELETE FROM submission", "expires_at <= CURRENT_TIMESTAMP", "ORDER BY expires_at", "FETCH FIRST"} {
				if !strings.Contains(calls[0].SQL, want) {
					t.Errorf("delete = %q, want it to contain %q", calls[0].SQL, want)
				}
			}
			if !slices.Equal(calls[0].Args, []any{25}) {
				t.Errorf("delete args = %v, want the budget", calls[0].Args)
			}
		})
	}
}

func TestReclaimer_PassRefusesABudgetBelowOne(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.Reclaimer().Pass(t.Context(), 0); err == nil {
		t.Error("Pass with budget 0 = nil error, want refused")
	}
	if ops := f.rec.Ops(); len(ops) != 0 {
		t.Errorf("ops = %v, want none", ops)
	}
}

var completedColumns = []string{"sha256", "name", "media_type", "size", "job_id", "digest"}

func file(content string) upload.File {
	return upload.File{ID: uuid.NewV4().String(), MediaType: "application/pdf", Digest: sha([]byte(content)), Name: "scan.pdf", Length: int64(len(content))}
}

func completeRequest(f upload.File) client.CompleteUpload {
	return client.CompleteUpload{Upload: f.ID, Profile: "standard@1", Extract: extract("per source"), Timeline: extract("timeline")}
}

func queuedOf(f upload.File, req client.CompleteUpload) client.Queued {
	return client.Queued{
		Profile: req.Profile, Name: f.Name, MediaType: f.MediaType, Size: f.Length, SHA256: f.Digest,
		Extract: req.Extract, Document: req.Document, Timeline: req.Timeline,
	}
}

func digestOf(q client.Queued) string {
	return sha(must(json.Marshal(q)))
}

func (f *fixture) completed(up upload.File, job, digest any) {
	f.rec.Queue(sqltest.Response{Columns: completedColumns, Rows: [][]driver.Value{{up.Digest, up.Name, up.MediaType, up.Length, job, digest}}})
}

func (f *fixture) assigned(up upload.File, changed int64, job, digest string) {
	f.rec.Queue(sqltest.Response{Affected: changed})
	f.completed(up, job, digest)
}

func (f *fixture) call(t *testing.T, contains string) sqltest.Call {
	t.Helper()
	for _, c := range f.rec.Calls() {
		if strings.Contains(c.SQL, contains) {
			return c
		}
	}
	t.Fatalf("no call contains %q", contains)
	return sqltest.Call{}
}

func TestComplete_StagesTheFileAndRecordsTheUpload(t *testing.T) {
	f := newFixture(t)
	f.rec.Queue(sqltest.Response{Affected: 1})
	const content = "a large scanned document"
	up := file(content)

	if err := f.service.Complete(t.Context(), up, strings.NewReader(content)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	r, err := f.objects.Get(t.Context(), "input/"+up.Digest[:2]+"/"+up.Digest, storage.GetOptions{})
	if err != nil {
		t.Fatalf("the staged input: %v", err)
	}
	staged, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if string(staged) != content {
		t.Errorf("staged %q, want the file", staged)
	}
	insert := f.call(t, "INSERT INTO completed_upload")
	want := []any{up.ID, up.Digest, up.Name, up.MediaType, up.Length}
	if len(insert.Args) != 6 || !slices.Equal(insert.Args[:5], want) {
		t.Errorf("record args = %v, want %v and an expiry", insert.Args, want)
	}
	if expires, ok := insert.Args[5].(time.Time); !ok || time.Until(expires) <= 59*time.Minute || time.Until(expires) > time.Hour {
		t.Errorf("expiry = %v, want a lifetime from now", insert.Args[5])
	}
}

func TestComplete_IsIdempotentOnTheUpload(t *testing.T) {
	f := newFixture(t)
	const content = "a large scanned document"
	up := file(content)
	f.rec.Queue(sqltest.Response{Affected: 1}, sqltest.Response{Err: fmt.Errorf("duplicate key: %w", sqlate.ErrUniqueViolation)})

	for i := range 2 {
		if err := f.service.Complete(t.Context(), up, strings.NewReader(content)); err != nil {
			t.Fatalf("Complete %d: %v", i+1, err)
		}
	}
	if got := keysUnder(t, f, "input/"); len(got) != 1 {
		t.Errorf("staged objects = %v, want the file's one", got)
	}
}

func keysUnder(t *testing.T, f *fixture, prefix string) []string {
	t.Helper()
	page, err := f.objects.List(t.Context(), storage.ListOptions{Prefix: prefix})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var keys []string
	for _, o := range page.Objects {
		keys = append(keys, o.Key)
	}
	return keys
}

func TestCompleteUpload_QueuesTheJobFromTheRecord(t *testing.T) {
	f := newFixture(t)
	up := file("a large scanned document")
	req := completeRequest(up)
	q := queuedOf(up, req)
	job := uuid.NewV7().String()
	f.completed(up, nil, nil)
	f.assigned(up, 1, job, digestOf(q))

	j, err := f.service.CompleteUpload(t.Context(), req)
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	want := client.Job{ID: job, Profile: req.Profile, Name: up.Name, Size: up.Length, SHA256: up.Digest, State: client.StateQueued, Sequence: 1}
	if j != want {
		t.Errorf("job = %+v, want %+v", j, want)
	}
	update := f.call(t, "UPDATE completed_upload")
	if !slices.Contains(update.Args, any(up.ID)) || !slices.Contains(update.Args, any(digestOf(q))) {
		t.Errorf("job record args = %v, want the upload and the request's digest", update.Args)
	}
	for _, a := range update.Args {
		if s, ok := a.(string); ok && s != up.ID && s != digestOf(q) {
			if u, err := uuid.Parse(s); err != nil || u[6]>>4 != 7 {
				t.Errorf("job record names job %q, want a new UUIDv7", s)
			}
		}
	}

	events := f.events(t, job)
	if len(events) != 1 || events[0].Type != jobs.TypeQueued {
		t.Fatalf("events = %+v, want one queued", events)
	}
	if string(events[0].Data) != string(must(json.Marshal(q))) {
		t.Errorf("queued = %s, want Submit's shape over the staged file, %s", events[0].Data, must(json.Marshal(q)))
	}
	if stored := f.queue.stored(t); len(stored) != 1 || stored[0].ID != job {
		t.Errorf("the queue holds %v, want the job's work event", stored)
	}
}

func TestCompleteUpload_RepeatReturnsTheFirstJob(t *testing.T) {
	f := newFixture(t)
	up := file("a large scanned document")
	req := completeRequest(up)
	job, digest := uuid.NewV7().String(), digestOf(queuedOf(up, req))
	f.completed(up, nil, nil)
	f.assigned(up, 1, job, digest)
	first, err := f.service.CompleteUpload(t.Context(), req)
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	f.completed(up, job, digest)

	again, err := f.service.CompleteUpload(t.Context(), req)
	if err != nil {
		t.Fatalf("the repeat: %v", err)
	}
	if again != first {
		t.Errorf("the repeat returned %+v, want the first job %+v", again, first)
	}
	if n := strings.Count(fmt.Sprint(f.rec.Calls()), "UPDATE completed_upload"); n != 1 {
		t.Errorf("recorded a job %d times, want the first CompleteUpload's alone", n)
	}
	if events := f.events(t, job); len(events) != 1 {
		t.Errorf("job %s holds %d events, want its one queued", job, len(events))
	}
	if stored := f.queue.stored(t); len(stored) != 1 || stored[0].ID != job {
		t.Errorf("the queue holds %v, want the job's one work event", stored)
	}
}

func TestCompleteUpload_ARacingRepeatReadsTheRecordedJob(t *testing.T) {
	f := newFixture(t)
	up := file("a large scanned document")
	req := completeRequest(up)
	job := uuid.NewV7().String()
	f.completed(up, nil, nil)
	f.assigned(up, 0, job, digestOf(queuedOf(up, req)))

	j, err := f.service.CompleteUpload(t.Context(), req)
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if j.ID != job || j.State != client.StateQueued {
		t.Errorf("job = %+v, want the job the other CompleteUpload recorded, %s, queued", j, job)
	}
}

func TestCompleteUpload_DifferentRequestConflicts(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*client.CompleteUpload)
	}{
		{"profile", func(r *client.CompleteUpload) { r.Profile = "standard@2" }},
		{"per-source extract", func(r *client.CompleteUpload) { r.Extract = nil }},
		{"document extract", func(r *client.CompleteUpload) { r.Document = extract("another") }},
		{"timeline extract", func(r *client.CompleteUpload) { r.Timeline = extract("another") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			up := file("a large scanned document")
			req := completeRequest(up)
			f.completed(up, uuid.NewV7().String(), digestOf(queuedOf(up, req)))
			tc.edit(&req)

			if _, err := f.service.CompleteUpload(t.Context(), req); !errors.Is(err, submissions.ErrConflict) {
				t.Fatalf("CompleteUpload with a different %s = %v, want ErrConflict", tc.name, err)
			}
			if f.journal.calls != 0 || len(f.queue.enqueued()) != 0 {
				t.Errorf("appends %d, enqueues %d; want none for a conflict", f.journal.calls, len(f.queue.enqueued()))
			}
		})
	}
}

func TestCompleteUpload_AnUnknownOrExpiredUploadIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.rec.Queue(sqltest.Response{Columns: completedColumns})

	if _, err := f.service.CompleteUpload(t.Context(), completeRequest(file("never completed"))); !errors.Is(err, submissions.ErrUploadNotFound) {
		t.Fatalf("CompleteUpload = %v, want ErrUploadNotFound", err)
	}
	if read := f.call(t, "FROM completed_upload"); !strings.Contains(read.SQL, "expires_at > CURRENT_TIMESTAMP") {
		t.Errorf("read = %q, want it to skip an expired record", read.SQL)
	}
	if f.journal.calls != 0 || len(f.queue.enqueued()) != 0 {
		t.Errorf("appends %d, enqueues %d; want none", f.journal.calls, len(f.queue.enqueued()))
	}
}

func TestCompleteUpload_Refusals(t *testing.T) {
	big := strings.Repeat("x", submissions.ExtractionLimit)
	for _, tc := range []struct {
		name string
		edit func(*client.CompleteUpload)
		want error
	}{
		{"an empty profile", func(r *client.CompleteUpload) { r.Profile = "" }, submissions.ErrInvalid},
		{"no upload", func(r *client.CompleteUpload) { r.Upload = "" }, submissions.ErrInvalid},
		{"an upload that is not a UUID", func(r *client.CompleteUpload) { r.Upload = "upload-1" }, submissions.ErrInvalid},
		{"extracts past the cap", func(r *client.CompleteUpload) { r.Document = extract(big) }, submissions.ErrExtractionTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			req := completeRequest(file("a document"))
			tc.edit(&req)
			if _, err := f.service.CompleteUpload(t.Context(), req); !errors.Is(err, tc.want) {
				t.Fatalf("CompleteUpload = %v, want %v", err, tc.want)
			}
			if calls := f.rec.Calls(); len(calls) != 0 {
				t.Errorf("database calls = %v, want none for a refusal", calls)
			}
		})
	}
}

func TestUploadReclaimer_PassDeletesExpiredCompletedUploads(t *testing.T) {
	for _, c := range []struct {
		name     string
		affected int64
		more     bool
	}{
		{"a full batch", 25, true},
		{"a short batch", 7, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.rec.Queue(sqltest.Response{Affected: c.affected})
			more, err := f.service.UploadReclaimer().Pass(t.Context(), 25)
			if err != nil {
				t.Fatal(err)
			}
			if more != c.more {
				t.Errorf("more = %t after deleting %d of 25, want %t", more, c.affected, c.more)
			}
			calls := f.rec.Calls()
			if len(calls) != 1 || calls[0].Op != sqltest.OpExec {
				t.Fatalf("calls = %v, want one delete", calls)
			}
			for _, want := range []string{"DELETE FROM completed_upload", "expires_at <= CURRENT_TIMESTAMP", "ORDER BY expires_at", "FETCH FIRST"} {
				if !strings.Contains(calls[0].SQL, want) {
					t.Errorf("delete = %q, want it to contain %q", calls[0].SQL, want)
				}
			}
		})
	}
	f := newFixture(t)
	if _, err := f.service.UploadReclaimer().Pass(t.Context(), 0); err == nil || len(f.rec.Ops()) != 0 {
		t.Errorf("Pass with budget 0 = %v after %v, want refused before any call", err, f.rec.Ops())
	}
}
