package outputs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	docstorage "github.com/standards-lab/docproc/storage"
)

// Service is GetOutput: it opens the content docproc's content store keeps under an output's
// SHA-256.
type Service struct {
	content *docstorage.Content
}

// New constructs the service over content, docproc's content store. Construction performs no
// I/O. A nil content is a wiring defect and panics.
func New(content *docstorage.Content) *Service {
	if content == nil {
		panic("outputs: nil content")
	}
	return &Service{content: content}
}

// Open returns the content stored under sha256, for the caller to read and close. A sha256 that
// is not 64 lowercase hex digits is ErrInvalid, and one the store holds nothing under is
// ErrNotFound.
func (s *Service) Open(ctx context.Context, sha256 string) (io.ReadCloser, error) {
	if !isSHA256(sha256) {
		return nil, fmt.Errorf("%w: %q", ErrInvalid, sha256)
	}
	body, err := s.content.Open(ctx, sha256)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("output %s: %w", sha256, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("output %s: %w", sha256, err)
	}
	return body, nil
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range []byte(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
