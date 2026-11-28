package client

import (
	"github.com/standards-lab/docinfer"
	"github.com/standards-lab/docinfer/job"
)

// State is where a job stands.
type State string

// The states: a job is queued until an attempt starts it, running while the attempt runs, and
// ends completed, failed, or cancelled.
const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Job is one input's processing, the unit of retry, as the fold of its events reads it.
type Job struct {
	ID      string `json:"id"`      // a UUIDv7, which Bunrei mints
	Profile string `json:"profile"` // the versioned processing profile
	Name    string `json:"name"`    // the document's file name
	Size    int64  `json:"size"`    // the input's size in bytes
	SHA256  string `json:"sha256"`  // the input content's SHA-256, lowercase hex
	State   State  `json:"state"`
	// Attempt is the work queue's delivery, from 1; 0 before the first attempt starts.
	Attempt  int       `json:"attempt"`
	Sequence uint64    `json:"sequence"`           // the jobsequence of the job's last event
	Manifest *Manifest `json:"manifest,omitempty"` // set once the job completes
	Error    string    `json:"error,omitempty"`    // the error the job failed with
}

// Manifest is what a completed job's results add up to over every jobsequence from 1 through
// its completed event's, every attempt's batches included.
type Manifest struct {
	Batches int `json:"batches"` // the batch events
	Chunks  int `json:"chunks"`  // their batchchunks, summed
	// Digest is the lowercase hex SHA-256 over those batches' batchdigest values in sequence
	// order, each prefixed by its length as an 8-byte big-endian integer.
	Digest   string    `json:"digest"`
	Attempts []Attempt `json:"attempts"` // one per started event, in order
}

// Attempt is one attempt's entry in a [Manifest].
type Attempt struct {
	Done *job.Done `json:"done,omitempty"` // the attempt's job.Done; nil when it ended without one
}

// Queued is the data of bunrei.job.queued: the job's input, and the caller's extraction.
type Queued struct {
	Profile    string            `json:"profile"`
	Name       string            `json:"name"`
	MediaType  string            `json:"media_type"`
	Size       int64             `json:"size"`
	SHA256     string            `json:"sha256"`
	Extraction *docinfer.Extract `json:"extraction,omitempty"` // nil when the caller asked none
}

// Started is the data of bunrei.job.started: the attempt, the replica running it, and the
// docinfer Profile its profile resolved to when it started, prompt overrides applied.
type Started struct {
	Attempt int              `json:"attempt"`
	Replica string           `json:"replica"`
	Profile docinfer.Profile `json:"profile"`
}

// AttemptFailed is the data of bunrei.job.attempt.failed: an attempt that failed with
// deliveries left, and its error.
type AttemptFailed struct {
	Attempt int    `json:"attempt"`
	Error   string `json:"error"`
}

// Failed is the data of bunrei.job.failed: the attempt the job failed on, and its error.
type Failed struct {
	Attempt int    `json:"attempt"`
	Error   string `json:"error"`
}
