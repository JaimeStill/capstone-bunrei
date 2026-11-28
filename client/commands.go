package client

import "github.com/standards-lab/docinfer"

// Submit is the Submit Command: a whole document, at most the small-document limit, queued as
// one job under the caller's idempotency ID. Its JSON carries Content as base64. A Submit
// repeating an ID with the same profile, name, media type, content, and extracts returns the
// first job; the same ID with any of them different is refused with [ProblemSubmitConflict].
type Submit struct {
	ID        string            `json:"id"`         // the caller's idempotency ID
	Profile   string            `json:"profile"`    // a versioned processing profile
	Name      string            `json:"name"`       // the document's file name
	MediaType string            `json:"media_type"` // the document's media type
	Content   []byte            `json:"content"`    // the document's bytes
	Extract   *docinfer.Extract `json:"extract,omitempty"`
	Document  *docinfer.Extract `json:"document,omitempty"`
	Timeline  *docinfer.Extract `json:"timeline,omitempty"`
}

// CompleteUpload is the CompleteUpload Command: a completed upload queued as one job, with a
// profile and the same three optional extracts as [Submit].
type CompleteUpload struct {
	Upload   string            `json:"upload"`  // the completed upload's ID
	Profile  string            `json:"profile"` // a versioned processing profile
	Extract  *docinfer.Extract `json:"extract,omitempty"`
	Document *docinfer.Extract `json:"document,omitempty"`
	Timeline *docinfer.Extract `json:"timeline,omitempty"`
}
