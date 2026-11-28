package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/standards-lab/go-web-sdk/upload"
)

// UploadOptions shapes how [Client.Upload] sends a file. The zero value sends it in one attempt,
// in the chunks the server announces.
type UploadOptions struct {
	// Attempts is how many requests in a row may fail to advance the upload before Upload gives
	// up: a request cut off or answered 5xx, 429, or 409 counts, and an append the server
	// commits starts the count again. After each failure Upload reads the server's offset and
	// resumes from it. Below 1 it is 1.
	Attempts int
	// Delay is how long Upload waits before each retry. Zero retries at once.
	Delay time.Duration
	// ChunkSize, when positive, is the most content one append carries, below the chunk size
	// the server announces. Zero leaves the chunk at the server's.
	ChunkSize int64
}

// Upload sends content as a resumable upload, POST /uploads and its appends, named name with
// the media type mediaType, and returns the completed upload's ID, which [CompleteUpload]
// names. It reads content once whole for the file's length and SHA-256, then seeks it to each
// offset it sends from, so content must hold the same bytes until Upload returns. A send cut off
// partway resumes from the offset the server holds, within opts.Attempts; when they run out,
// Upload returns an error and the server keeps the partial upload until its lifetime ends. A
// completing request the server refuses returns its [*Problem].
func (c *Client) Upload(ctx context.Context, name, mediaType string, content io.ReadSeeker, opts UploadOptions) (string, error) {
	uc := upload.Client{HTTP: c.http, Attempts: max(opts.Attempts, 1), Delay: opts.Delay, ChunkSize: opts.ChunkSize}
	r, err := uc.Create(ctx, c.url("uploads"), mediaType, name, content)
	if err != nil {
		return "", fmt.Errorf("bunrei: create the upload: %w", err)
	}
	res, err := r.Send(ctx)
	if err != nil {
		return "", fmt.Errorf("bunrei: send the upload: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", refusal(res)
	}
	var u Upload
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return "", fmt.Errorf("bunrei: decode the completed upload: %w", err)
	}
	return u.ID, nil
}
