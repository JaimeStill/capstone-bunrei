//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"uuid"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/standards-lab/go-core/event"
	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-messaging"
	"github.com/standards-lab/go-web-sdk/webtest"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
	"github.com/JaimeStill/capstone-bunrei/integration"
)

func composedJobs(t *testing.T, wrap func(messaging.Journal) messaging.Journal) *jobs.Service {
	t.Helper()
	a := integration.Compose(t)
	n := a.Nodes()
	if wrap != nil {
		a.Graph().Replace(n.Jobs, func(s *graph.Scope) (*jobs.Service, error) {
			return jobs.New(wrap(s.Use(n.JobJournal)), jobs.Options{Prefix: s.Use(n.Config).NATS.Prefix}), nil
		})
	}
	sys, err := a.Graph().Build(n.Jobs)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	journal := sys.Get(n.JobJournal)
	if err := journal.Start(context.Background()); err != nil {
		t.Fatalf("start the job journal: %v", err)
	}
	t.Cleanup(func() { _ = journal.Shutdown(context.Background()) })
	return sys.Get(n.Jobs)
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}

func queuedEvent(t *testing.T) event.Event {
	return event.Event{Type: jobs.TypeQueued, Data: encode(t, client.Queued{
		Profile: "standard@1", Name: "report.pdf", MediaType: "application/pdf", Size: 4096, SHA256: "ab12",
	})}
}

func startedEvent(t *testing.T, attempt int) event.Event {
	return event.Event{Type: jobs.TypeStarted, Data: encode(t, client.Started{Attempt: attempt, Replica: "bunrei-0"})}
}

func appendEvent(t *testing.T, s *jobs.Service, id string, e event.Event) jobs.Appended {
	t.Helper()
	a, err := s.Append(context.Background(), id, func(jobs.Job) (event.Event, error) { return e, nil })
	if err != nil {
		t.Fatalf("Append %s: %v", e.Type, err)
	}
	return a
}

func TestJobs_AppendLoadRoundTrip(t *testing.T) {
	s := composedJobs(t, nil)
	id := uuid.NewV7().String()

	q := appendEvent(t, s, id, queuedEvent(t))
	st := appendEvent(t, s, id, startedEvent(t, 1))
	if q.Sequence == 0 || st.Sequence <= q.Sequence {
		t.Errorf("stream sequences %d, %d; want positive and increasing", q.Sequence, st.Sequence)
	}
	if q.Event.Extensions[jobs.ExtSequence] != "1" || st.Event.Extensions[jobs.ExtSequence] != "2" {
		t.Errorf("jobsequence %s, %s; want 1, 2", q.Event.Extensions[jobs.ExtSequence], st.Event.Extensions[jobs.ExtSequence])
	}

	j, last, err := s.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if last != st.Sequence {
		t.Errorf("last = %d, want the started event's stream sequence, %d", last, st.Sequence)
	}
	if j.ID != id || j.Sequence != 2 || j.State != client.StateRunning || j.Name != "report.pdf" || len(j.Attempts) != 1 {
		t.Errorf("job = %+v, want job %s running its first attempt after 2 events", j, id)
	}
}

func TestJobs_ConflictBetweenTwoWritersRefolds(t *testing.T) {
	s := composedJobs(t, nil)
	id := uuid.NewV7().String()
	appendEvent(t, s, id, queuedEvent(t))

	var seen []jobs.Job
	var other jobs.Appended
	a, err := s.Append(context.Background(), id, func(j jobs.Job) (event.Event, error) {
		seen = append(seen, j)
		if len(seen) == 1 {
			other = appendEvent(t, s, id, startedEvent(t, 1))
		}
		return event.Event{Type: jobs.TypeCheckpoint, Data: encode(t, map[string]uint64{"after": uint64(j.Sequence)})}, nil
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
	if a.Sequence <= other.Sequence || a.Event.Extensions[jobs.ExtSequence] != "3" || a.Job.Checkpoint != 2 {
		t.Errorf("appended at %d after the other writer's %d, jobsequence %s, Checkpoint %d; want after it, 3, 2",
			a.Sequence, other.Sequence, a.Event.Extensions[jobs.ExtSequence], a.Job.Checkpoint)
	}
}

func TestJobs_ConfiguredPrefix(t *testing.T) {
	t.Setenv("BUNREI_NATS_PREFIX", "zzalt.job")
	t.Setenv("BUNREI_NATS_STREAM", "ZZALT_JOBS")
	s := composedJobs(t, nil)
	id := uuid.NewV7().String()

	appendEvent(t, s, id, queuedEvent(t))
	st := appendEvent(t, s, id, startedEvent(t, 1))
	j, last, err := s.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if j.Sequence != 2 || last != st.Sequence {
		t.Errorf("Load = sequence %d, last %d; want 2, %d", j.Sequence, last, st.Sequence)
	}

	nc, err := natsgo.Connect(integration.NATSURL())
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	stream, err := js.Stream(context.Background(), "ZZALT_JOBS")
	if err != nil {
		t.Fatalf("stream ZZALT_JOBS: %v", err)
	}
	msg, err := stream.GetLastMsgForSubject(context.Background(), "zzalt.job."+id)
	if err != nil {
		t.Fatalf("last message on zzalt.job.%s: %v", id, err)
	}
	if msg.Sequence != st.Sequence {
		t.Errorf("last message on zzalt.job.%s at %d, want the started event's %d", id, msg.Sequence, st.Sequence)
	}
}

var errLostAck = errors.New("ack lost")

type lossyJournal struct {
	messaging.Journal
	lose  bool
	calls int
}

func (j *lossyJournal) Append(ctx context.Context, subject string, expect uint64, e event.Event) (uint64, error) {
	j.calls++
	seq, err := j.Journal.Append(ctx, subject, expect, e)
	if err == nil && j.lose {
		j.lose = false
		return 0, errLostAck
	}
	return seq, err
}

func TestJobs_StaleRetryReturnsTheOriginalSequence(t *testing.T) {
	var lossy *lossyJournal
	s := composedJobs(t, func(j messaging.Journal) messaging.Journal {
		lossy = &lossyJournal{Journal: j}
		return lossy
	})
	id := uuid.NewV7().String()
	appendEvent(t, s, id, queuedEvent(t))
	lossy.lose, lossy.calls = true, 0

	a := appendEvent(t, s, id, startedEvent(t, 1))
	if lossy.calls != 2 || a.Job.Sequence != 2 {
		t.Errorf("calls %d, Sequence %d; want 2, 2", lossy.calls, a.Job.Sequence)
	}
	j, last, err := s.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if j.Sequence != 2 || last != a.Sequence {
		t.Errorf("Load = sequence %d, last %d; want 2, the retry's %d: stored once", j.Sequence, last, a.Sequence)
	}
}

func TestJobs_OtherJobsLeaveTheJobSequence(t *testing.T) {
	s := composedJobs(t, nil)
	id, other := uuid.NewV7().String(), uuid.NewV7().String()

	q := appendEvent(t, s, id, queuedEvent(t))
	appendEvent(t, s, other, queuedEvent(t))
	appendEvent(t, s, other, startedEvent(t, 1))
	st := appendEvent(t, s, id, startedEvent(t, 1))

	if st.Sequence-q.Sequence < 3 || st.Event.Extensions[jobs.ExtSequence] != "2" || st.Job.Sequence != 2 {
		t.Errorf("stream sequences %d, %d, jobsequence %s, Sequence %d; want 3 or more apart, 2, 2",
			q.Sequence, st.Sequence, st.Event.Extensions[jobs.ExtSequence], st.Job.Sequence)
	}
	j, last, err := s.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if j.Sequence != 2 || last != st.Sequence {
		t.Errorf("Load = sequence %d, last %d; want 2, %d", j.Sequence, last, st.Sequence)
	}
}

func TestJobs_GetJobAndCancelJobOverHTTP(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := s.Client()
	js := jetStream(t)

	j := webtest.Decode[client.Job](t, c.Post(t, "/api/jobs", submitRequest()), http.StatusAccepted)
	got := webtest.Decode[client.Job](t, c.Get(t, "/api/jobs/"+j.ID), http.StatusOK)
	if got != j || got.State != client.StateQueued {
		t.Errorf("GetJob = %+v, want the submitted job %+v, queued", got, j)
	}

	resp := c.Post(t, "/api/jobs/"+j.ID+"/cancel", nil).Expect(t, http.StatusAccepted)
	if loc := resp.Header.Get("Location"); loc != "/api/jobs/"+j.ID {
		t.Errorf("Location = %q, want /api/jobs/%s", loc, j.ID)
	}
	got = webtest.Decode[client.Job](t, c.Get(t, "/api/jobs/"+j.ID), http.StatusOK)
	if got.State != client.StateCancelled || got.Sequence != 3 {
		t.Errorf("GetJob after CancelJob = %+v, want cancelled at sequence 3", got)
	}
	events := jobEvents(t, js, j.ID)
	if len(events) != 3 || events[1].Type != jobs.TypeCancelRequested || events[2].Type != jobs.TypeCancelled ||
		events[2].Extensions[jobs.ExtCausation] != events[1].ID {
		t.Errorf("BUNREI_JOBS holds %+v, want queued, cancel_requested, and cancelled caused by it", events)
	}

	p := c.Post(t, "/api/jobs/"+j.ID+"/cancel", nil).Problem(t, http.StatusConflict)
	if p.Type != client.ProblemJobEnded {
		t.Errorf("problem type = %q, want %s", p.Type, client.ProblemJobEnded)
	}
	p = c.Get(t, "/api/jobs/"+uuid.NewV7().String()).Problem(t, http.StatusNotFound)
	if p.Type != client.ProblemJobNotFound {
		t.Errorf("problem type = %q, want %s", p.Type, client.ProblemJobNotFound)
	}
	p = c.Get(t, "/api/jobs/not-a-uuid").Problem(t, http.StatusBadRequest)
	if p.Type != client.ProblemInvalidRequest {
		t.Errorf("problem type = %q, want %s", p.Type, client.ProblemInvalidRequest)
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}
