package jobs

import "errors"

var (
	// ErrNotFound reports a load of a job with no events.
	ErrNotFound = errors.New("the job does not exist")

	// ErrInvalidID reports a job id that is not a UUID.
	ErrInvalidID = errors.New("a job id is a UUID")

	// ErrUnknownType reports an event whose type is none of the job's 13. Wrapped with the type,
	// it fails the load or the append that meets it.
	ErrUnknownType = errors.New("not a job event type")

	// ErrEnded reports an append to a job that has ended: completed, failed, or cancelled.
	ErrEnded = errors.New("the job has ended; nothing is appended to it")
)
