//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
	"uuid"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/standards-lab/docinfer"
	"github.com/standards-lab/go-core/event"
	"github.com/standards-lab/go-core/process/processtest"
	"github.com/standards-lab/go-web-sdk/webtest"

	"github.com/JaimeStill/capstone-bunrei/client"
	"github.com/JaimeStill/capstone-bunrei/domain/jobs"
	"github.com/JaimeStill/capstone-bunrei/integration"
)

func jetStream(t *testing.T) jetstream.JetStream {
	t.Helper()
	nc, err := natsgo.Connect(integration.NATSURL())
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	return js
}

func jobEvents(t *testing.T, js jetstream.JetStream, job string) []event.Event {
	t.Helper()
	ctx := context.Background()
	stream, err := js.Stream(ctx, "BUNREI_JOBS")
	if err != nil {
		t.Fatalf("stream BUNREI_JOBS: %v", err)
	}
	subject := "bunrei.job." + job
	info, err := stream.Info(ctx, jetstream.WithSubjectFilter(subject))
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	var events []event.Event
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		if err != nil {
			t.Fatalf("message %d: %v", seq, err)
		}
		if msg.Subject != subject {
			continue
		}
		e, err := event.Decode(event.Header(msg.Header), msg.Data)
		if err != nil {
			t.Fatalf("decode message %d: %v", seq, err)
		}
		events = append(events, e)
	}
	return events
}

func workMessages(t *testing.T, js jetstream.JetStream, job string) []event.Event {
	t.Helper()
	ctx := context.Background()
	stream, err := js.Stream(ctx, "BUNREI_WORK")
	if err != nil {
		t.Fatalf("stream BUNREI_WORK: %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	var found []event.Event
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq && info.State.Msgs > 0; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		if err != nil {
			t.Fatalf("message %d: %v", seq, err)
		}
		e, err := event.Decode(event.Header(msg.Header), msg.Data)
		if err != nil {
			t.Fatalf("decode message %d: %v", seq, err)
		}
		if msg.Subject == "bunrei.work" && e.ID == job {
			found = append(found, e)
		}
	}
	return found
}

func submitRequest() client.Submit {
	return client.Submit{
		ID:        "submit-" + uuid.NewV7().String(),
		Profile:   "standard@1",
		Name:      "invoice.json",
		MediaType: "application/json",
		Content:   []byte(`{"invoice":"INV-7","total":12.5}`),
		Extract:   &docinfer.Extract{Name: "invoice", Prompt: "The total.", Schema: []byte(`{"type":"object"}`)},
		Timeline:  &docinfer.Extract{Name: "scenes", Prompt: "The scenes.", Schema: []byte(`{"type":"object"}`)},
	}
}

func TestSubmissions_SubmitQueuesTheJobOnBothStreams(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := s.Client()
	js := jetStream(t)
	req := submitRequest()

	resp := c.Post(t, "/api/jobs", req)
	j := webtest.Decode[client.Job](t, resp, http.StatusAccepted)
	raw := sha256.Sum256(req.Content)
	sum := hex.EncodeToString(raw[:])
	if j.State != client.StateQueued || j.SHA256 != sum || j.Size != int64(len(req.Content)) || j.Sequence != 1 {
		t.Errorf("job = %+v, want it queued at sequence 1 with the content's size and SHA-256", j)
	}
	if loc := resp.Header.Get("Location"); loc != "/api/jobs/"+j.ID {
		t.Errorf("Location = %q, want /api/jobs/%s", loc, j.ID)
	}

	events := jobEvents(t, js, j.ID)
	if len(events) != 1 || events[0].Type != jobs.TypeQueued {
		t.Fatalf("BUNREI_JOBS holds %+v for the job, want its one queued", events)
	}
	var q client.Queued
	if err := json.Unmarshal(events[0].Data, &q); err != nil {
		t.Fatalf("queued data: %v", err)
	}
	if q.Profile != req.Profile || q.Name != req.Name || q.MediaType != req.MediaType || q.SHA256 != sum ||
		q.Extract == nil || q.Document != nil || q.Timeline == nil || q.Timeline.Name != "scenes" {
		t.Errorf("queued = %+v, want the request's input and its two extracts", q)
	}

	work := workMessages(t, js, j.ID)
	if len(work) != 1 || work[0].Type != jobs.TypeWork || string(work[0].Data) != `{"job":"`+j.ID+`"}` {
		t.Fatalf("BUNREI_WORK holds %+v for the job, want one work event naming it", work)
	}

	again := webtest.Decode[client.Job](t, c.Post(t, "/api/jobs", req), http.StatusAccepted)
	if again != j {
		t.Errorf("the retry returned %+v, want the first job %+v", again, j)
	}
	if n := len(workMessages(t, js, j.ID)); n != 1 {
		t.Errorf("BUNREI_WORK holds %d work events for the job after the retry, want 1", n)
	}
	if n := len(jobEvents(t, js, j.ID)); n != 1 {
		t.Errorf("BUNREI_JOBS holds %d events for the job after the retry, want 1", n)
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}

func TestSubmissions_ConcurrentSubmitsOfOneIDYieldOneJob(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	js := jetStream(t)
	body, err := json.Marshal(submitRequest())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	results := make([]*webtest.Response, n)
	for i := range n {
		wg.Go(func() { results[i] = s.Client().Post(t, "/api/jobs", body) })
	}
	wg.Wait()

	ids := map[string]bool{}
	for i, r := range results {
		var j client.Job
		r.Expect(t, http.StatusAccepted).JSON(t, &j)
		if j.State != client.StateQueued {
			t.Errorf("Submit %d returned %+v, want the job queued", i, j)
		}
		ids[j.ID] = true
	}
	if len(ids) != 1 {
		t.Fatalf("the Submits returned jobs %v, want one", ids)
	}
	for id := range ids {
		if events := jobEvents(t, js, id); len(events) != 1 || events[0].Type != jobs.TypeQueued {
			t.Errorf("BUNREI_JOBS holds %+v for the job, want its one queued", events)
		}
		if n := len(workMessages(t, js, id)); n != 1 {
			t.Errorf("BUNREI_WORK holds %d work events for the job, want 1", n)
		}
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}

func TestSweep_ReclaimsAnExpiredSubmission(t *testing.T) {
	t.Setenv("BUNREI_SUBMISSIONS_RETENTION", "2s")
	t.Setenv("BUNREI_SWEEP_INTERVAL", "500ms")
	a := integration.Compose(t)
	n := a.Nodes()
	sys, err := a.Graph().Build(n.Schema, n.Sweeper)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	start(t, "database", sys.Get(n.Database))
	start(t, "storage", sys.Get(n.Storage))
	start(t, "job journal", sys.Get(n.JobJournal))
	start(t, "work queue", sys.Get(n.WorkQueue))
	start(t, "schema", sys.Get(n.Schema))

	ctx := context.Background()
	submit := sys.Get(n.Submissions)
	db := integration.Database(t)
	rows := func(id string) int {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM submission WHERE id = $1", id).Scan(&count); err != nil {
			t.Fatalf("count the submission rows of %s: %v", id, err)
		}
		return count
	}
	aged := submitRequest()
	if _, err := submit.Submit(ctx, aged); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got := rows(aged.ID); got != 1 {
		t.Fatalf("the submission's rows = %d, want its record", got)
	}
	time.Sleep(2500 * time.Millisecond)

	sweeper := sys.Get(n.Sweeper)
	start(t, "sweeper", sweeper)
	processtest.WaitFor(t, fmt.Sprintf("the expired submission %s's reclaim", aged.ID), func() bool {
		return rows(aged.ID) == 0
	})
	shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sweeper.Shutdown(shutdown); err != nil {
		t.Fatalf("Shutdown the sweeper: %v", err)
	}
}
