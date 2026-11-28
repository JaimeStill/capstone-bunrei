package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client calls Bunrei's API: one method per Command and Query, and [Client.Upload] for the
// resumable upload a large document goes through. Every method takes a context, which bounds
// its requests, and returns a refusal as a [*Problem]. A Client holds no state of its own beyond
// its base URL and the caller's http.Client, so it is safe for concurrent use as far as the
// http.Client is.
type Client struct {
	base *url.URL
	http *http.Client
}

// New returns a Client of the API served under base, the absolute http or https URL its routes
// hang from, such as http://bunrei:8080/api, sending every request through httpClient, which the
// caller owns and configures: its transport, its timeout, and its redirects. It returns an error
// when base is not such a URL or httpClient is nil.
func New(base string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("bunrei: base URL %q: %w", base, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("bunrei: base URL %q is not an absolute http or https URL", base)
	}
	if httpClient == nil {
		return nil, errors.New("bunrei: New requires an http.Client")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	u.RawQuery, u.Fragment = "", ""
	return &Client{base: u, http: httpClient}, nil
}

// Submit sends the Submit Command, POST /jobs, and returns the job it queued, or the first job
// when cmd repeats an earlier Submit's ID and request. It refuses a document past the
// small-document limit with [ErrContentTooLarge], extracts past 256 KiB together with
// [ErrExtractionTooLarge], an ID repeated with a different request with [ErrSubmitConflict], and
// a malformed Command with [ErrInvalidRequest].
func (c *Client) Submit(ctx context.Context, cmd Submit) (Job, error) {
	var j Job
	err := c.do(ctx, http.MethodPost, c.url("jobs"), cmd, http.StatusAccepted, &j)
	return j, err
}

// CompleteUpload sends the CompleteUpload Command, POST /uploads/{upload}/complete, for the
// upload cmd names, and returns the job it queued, or the first job when cmd repeats an earlier
// CompleteUpload of the upload with the same profile and extracts. It refuses an upload with no
// completed record with [ErrUploadNotFound], a repeat with a different profile or extracts with
// [ErrSubmitConflict], extracts past 256 KiB together with [ErrExtractionTooLarge], and a
// malformed Command with [ErrInvalidRequest].
func (c *Client) CompleteUpload(ctx context.Context, cmd CompleteUpload) (Job, error) {
	var j Job
	err := c.do(ctx, http.MethodPost, c.url("uploads", cmd.Upload, "complete"), cmd, http.StatusAccepted, &j)
	return j, err
}

// CancelJob sends the CancelJob Command, POST /jobs/{id}/cancel, and returns once Bunrei has
// accepted it; [Client.GetJob] reads the outcome. It refuses a job that has ended with
// [ErrJobEnded], an unknown job with [ErrJobNotFound], and an ID that is not a UUID with
// [ErrInvalidRequest].
func (c *Client) CancelJob(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, c.url("jobs", id, "cancel"), nil, http.StatusAccepted, nil)
}

// GetJob sends the GetJob Query, GET /jobs/{id}, and returns the job as the fold of its events
// reads it. It refuses an unknown job with [ErrJobNotFound] and an ID that is not a UUID with
// [ErrInvalidRequest].
func (c *Client) GetJob(ctx context.Context, id string) (Job, error) {
	var j Job
	err := c.do(ctx, http.MethodGet, c.url("jobs", id), nil, http.StatusOK, &j)
	return j, err
}

// GetOutput sends the GetOutput Query, GET /outputs/{sha256}, and returns the content stored
// under the SHA-256 as it streams, which the caller reads and closes. It refuses a SHA-256 the
// content store holds nothing under with [ErrOutputNotFound] and one that is not 64 lowercase
// hex digits with [ErrInvalidRequest].
func (c *Client) GetOutput(ctx context.Context, sha256 string) (io.ReadCloser, error) {
	res, err := c.send(ctx, http.MethodGet, c.url("outputs", sha256), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer func() { _ = res.Body.Close() }()
		return nil, refusal(res)
	}
	return res.Body, nil
}

// ListProfiles sends the ListProfiles Query, GET /profiles, and returns each processing profile
// Bunrei serves with the docinfer Profile it resolves to.
func (c *Client) ListProfiles(ctx context.Context) ([]Profile, error) {
	var ps []Profile
	err := c.do(ctx, http.MethodGet, c.url("profiles"), nil, http.StatusOK, &ps)
	return ps, err
}

func (c *Client) url(segments ...string) string {
	u := *c.base
	for _, s := range segments {
		u = *u.JoinPath(url.PathEscape(s))
	}
	return u.String()
}

func (c *Client) do(ctx context.Context, method, target string, in any, want int, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("bunrei: encode the request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	res, err := c.send(ctx, method, target, body)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != want {
		return refusal(res)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("bunrei: decode the %s %s response: %w", method, target, err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, method, target string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("bunrei: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json, "+problemMedia)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bunrei: %w", err)
	}
	return res, nil
}
