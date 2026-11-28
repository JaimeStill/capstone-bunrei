package client_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/standards-lab/docinfer"
	"github.com/standards-lab/docinfer/job"
	"github.com/standards-lab/docproc/dispatch"

	"github.com/JaimeStill/capstone-bunrei/client"
)

func done() *job.Done {
	return &job.Done{
		Parts:   map[dispatch.Outcome]int{dispatch.Prepared: 3, dispatch.Seen: 1},
		Outputs: 5,
		Counts:  docinfer.Counts{Texts: 2, Embedded: 7, Requests: 1},
	}
}

func extract() *docinfer.Extract {
	return &docinfer.Extract{
		Name:   "invoice",
		Prompt: "Extract the invoice's total.",
		Schema: []byte(`{"type":"object","properties":{"total":{"type":"number"}}}`),
	}
}

func profile() docinfer.Profile {
	return docinfer.Profile{
		Vision: &docinfer.Identity{
			Capability: docinfer.CapabilityVision,
			Model:      "vision-model",
			Settings:   `{"request":"1"}`,
		},
		Embeddings: docinfer.Identity{
			Capability: docinfer.CapabilityEmbeddings,
			Model:      "embed-model",
		},
		Dimensions: 768,
		Chunker:    `{"version":"1"}`,
	}
}

func TestEntities_RoundTripWithWireMembers(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		members []string
	}{
		{
			name: "Job",
			value: &client.Job{
				ID: "0192f0c4-8a3b-7c1d-9e2f-0123456789ab", Profile: "standard@1", Name: "report.pdf",
				Size: 4096, SHA256: "ab12", State: client.StateCompleted, Attempt: 2, Sequence: 41,
				Manifest: &client.Manifest{
					Batches: 3, Chunks: 12, Digest: "cd34",
					Attempts: []client.Attempt{{}, {Done: done()}},
				},
				Error: "boom",
			},
			members: []string{
				"attempt", "error", "id", "manifest", "name", "profile", "sequence", "sha256", "size", "state",
			},
		},
		{
			name:    "Job queued",
			value:   &client.Job{ID: "j", Profile: "standard@1", Name: "a.txt", State: client.StateQueued},
			members: []string{"attempt", "id", "name", "profile", "sequence", "sha256", "size", "state"},
		},
		{
			name: "Manifest",
			value: &client.Manifest{
				Batches: 1, Chunks: 4, Digest: "ef56", Attempts: []client.Attempt{{Done: done()}},
			},
			members: []string{"attempts", "batches", "chunks", "digest"},
		},
		{
			name:    "Attempt",
			value:   &client.Attempt{Done: done()},
			members: []string{"done"},
		},
		{
			name:    "Attempt without Done",
			value:   &client.Attempt{},
			members: []string{},
		},
		{
			name: "Queued",
			value: &client.Queued{
				Profile: "standard@1", Name: "invoice.pdf", MediaType: "application/pdf", Size: 2048,
				SHA256: "0a1b", Extraction: extract(),
			},
			members: []string{"extraction", "media_type", "name", "profile", "sha256", "size"},
		},
		{
			name: "Queued without extraction",
			value: &client.Queued{
				Profile: "standard@1", Name: "notes.txt", MediaType: "text/plain", Size: 12, SHA256: "2c3d",
			},
			members: []string{"media_type", "name", "profile", "sha256", "size"},
		},
		{
			name:    "Started",
			value:   &client.Started{Attempt: 1, Replica: "bunrei-0", Profile: profile()},
			members: []string{"attempt", "profile", "replica"},
		},
		{
			name:    "AttemptFailed",
			value:   &client.AttemptFailed{Attempt: 1, Error: "embeddings: timeout"},
			members: []string{"attempt", "error"},
		},
		{
			name:    "Failed",
			value:   &client.Failed{Attempt: 3, Error: "embeddings: timeout"},
			members: []string{"attempt", "error"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var wire map[string]json.RawMessage
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("Unmarshal to members: %v", err)
			}
			got := make([]string, 0, len(wire))
			for member := range wire {
				got = append(got, member)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.members) {
				t.Errorf("members = %v, want %v; JSON %s", got, tc.members, data)
			}

			back := reflect.New(reflect.TypeOf(tc.value).Elem()).Interface()
			if err := json.Unmarshal(data, back); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(back, tc.value) {
				t.Errorf("round trip = %+v, want %+v; JSON %s", back, tc.value, data)
			}
		})
	}
}

func TestState_WireValues(t *testing.T) {
	want := map[client.State]string{
		client.StateQueued:    `"queued"`,
		client.StateRunning:   `"running"`,
		client.StateCompleted: `"completed"`,
		client.StateFailed:    `"failed"`,
		client.StateCancelled: `"cancelled"`,
	}
	for state, wire := range want {
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("Marshal %q: %v", state, err)
		}
		if string(data) != wire {
			t.Errorf("Marshal(%q) = %s, want %s", state, data, wire)
		}
		var back client.State
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("Unmarshal %s: %v", data, err)
		}
		if back != state {
			t.Errorf("Unmarshal(%s) = %q, want %q", data, back, state)
		}
	}
}

func TestQueued_ExtractionSchemaIsJSON(t *testing.T) {
	data, err := json.Marshal(client.Queued{Extraction: extract()})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire struct {
		Extraction struct {
			Schema map[string]any `json:"schema"`
		} `json:"extraction"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v; JSON %s", err, data)
	}
	if wire.Extraction.Schema["type"] != "object" {
		t.Errorf("extraction.schema = %v, want the schema object; JSON %s", wire.Extraction.Schema, data)
	}
}
