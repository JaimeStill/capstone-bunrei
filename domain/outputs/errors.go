package outputs

import "errors"

var (
	// ErrInvalid reports a SHA-256 that is not 64 lowercase hex digits: a 400.
	ErrInvalid = errors.New("an output's SHA-256 is 64 lowercase hex digits")

	// ErrNotFound reports a SHA-256 the content store holds nothing under: a 404.
	ErrNotFound = errors.New("no output is stored under the SHA-256")
)
