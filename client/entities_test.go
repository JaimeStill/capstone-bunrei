package client_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
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
				SHA256: "0a1b", Extract: extract(), Document: extract(), Timeline: extract(),
			},
			members: []string{"document", "extract", "media_type", "name", "profile", "sha256", "size", "timeline"},
		},
		{
			name: "Queued without extracts",
			value: &client.Queued{
				Profile: "standard@1", Name: "notes.txt", MediaType: "text/plain", Size: 12, SHA256: "2c3d",
			},
			members: []string{"media_type", "name", "profile", "sha256", "size"},
		},
		{
			name: "Submit",
			value: &client.Submit{
				ID: "retry-1", Profile: "standard@1", Name: "invoice.pdf", MediaType: "application/pdf",
				Content: []byte("%PDF-1.7"), Extract: extract(), Document: extract(), Timeline: extract(),
			},
			members: []string{"content", "document", "extract", "id", "media_type", "name", "profile", "timeline"},
		},
		{
			name: "Submit without extracts",
			value: &client.Submit{
				ID: "retry-1", Profile: "standard@1", Name: "notes.txt", MediaType: "text/plain", Content: []byte("notes"),
			},
			members: []string{"content", "id", "media_type", "name", "profile"},
		},
		{
			name: "CompleteUpload",
			value: &client.CompleteUpload{
				Upload: "u1", Profile: "standard@1", Extract: extract(), Document: extract(), Timeline: extract(),
			},
			members: []string{"document", "extract", "profile", "timeline", "upload"},
		},
		{
			name:    "CompleteUpload without extracts",
			value:   &client.CompleteUpload{Upload: "u1", Profile: "standard@1"},
			members: []string{"profile", "upload"},
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

func TestQueued_ExtractSchemasAreJSON(t *testing.T) {
	data, err := json.Marshal(client.Queued{Extract: extract(), Document: extract(), Timeline: extract()})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v; JSON %s", err, data)
	}
	for _, member := range []string{"extract", "document", "timeline"} {
		var x struct {
			Schema map[string]any `json:"schema"`
		}
		if err := json.Unmarshal(wire[member], &x); err != nil || x.Schema["type"] != "object" {
			t.Errorf("%s.schema = %v (%v), want the schema object; JSON %s", member, x.Schema, err, data)
		}
	}
}

func TestSubmit_ContentIsBase64(t *testing.T) {
	data, err := json.Marshal(client.Submit{Content: []byte{0xff, 0x00, 0x7f}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v; JSON %s", err, data)
	}
	if wire.Content != "/wB/" {
		t.Errorf("content = %q, want the bytes in standard base64, %q", wire.Content, "/wB/")
	}
}

func TestProblems_TypesArePrefixedNames(t *testing.T) {
	for _, typ := range []string{
		client.ProblemInvalidRequest, client.ProblemContentTooLarge,
		client.ProblemExtractionTooLarge, client.ProblemSubmitConflict,
		client.ProblemJobNotFound, client.ProblemJobEnded, client.ProblemOutputNotFound,
	} {
		name, ok := strings.CutPrefix(typ, "urn:standards-lab:bunrei:problem:")
		if !ok || name == "" || strings.ContainsAny(name, ": ") {
			t.Errorf("problem type %q, want urn:standards-lab:bunrei:problem:<name>", typ)
		}
	}
}
