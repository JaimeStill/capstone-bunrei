package client

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// ProblemPrefix begins the type of every problem Bunrei answers a refusal with; the problem's
// name follows it.
const ProblemPrefix = "urn:standards-lab:bunrei:problem:"

// The types of the problems Bunrei refuses a request with, each [ProblemPrefix] followed by the
// problem's name.
const (
	// ProblemInvalidRequest is a 400: a body that is not the Command's JSON; a Command missing
	// its ID or profile, or with a media type that is not type/subtype; a job ID or an upload
	// that is not a UUID; or an output's SHA-256 that is not 64 lowercase hex digits.
	ProblemInvalidRequest = ProblemPrefix + "invalid-request"
	// ProblemContentTooLarge is a 413: a document past the small-document limit, which goes
	// through the upload instead, or a body past the limit's encoded size.
	ProblemContentTooLarge = ProblemPrefix + "content-too-large"
	// ProblemExtractionTooLarge is a 413: extracts that marshal past 256 KiB together.
	ProblemExtractionTooLarge = ProblemPrefix + "extraction-too-large"
	// ProblemSubmitConflict is a 409: a Submit repeating an ID with a different request, or a
	// CompleteUpload repeating an upload with a different profile or extracts.
	ProblemSubmitConflict = ProblemPrefix + "submit-conflict"
	// ProblemJobNotFound is a 404: a job with no events, never submitted or past the job
	// stream's retention.
	ProblemJobNotFound = ProblemPrefix + "job-not-found"
	// ProblemJobEnded is a 409: a CancelJob of a job that has ended, completed, failed, or
	// cancelled.
	ProblemJobEnded = ProblemPrefix + "job-ended"
	// ProblemOutputNotFound is a 404: a GetOutput of a SHA-256 the content store holds nothing
	// under.
	ProblemOutputNotFound = ProblemPrefix + "output-not-found"
	// ProblemUploadNotFound is a 404: a CompleteUpload of an upload with no completed record,
	// never completed or completed longer than the upload lifetime ago.
	ProblemUploadNotFound = ProblemPrefix + "upload-not-found"
)

// Problem is a refusal as the [Client] returns it: the RFC 9457 problem Bunrei answered with.
// Every method of a [Client] returns a refusal as a *Problem, which errors.As matches. A response
// whose body is not a problem, such as one a proxy wrote, is a *Problem too, typed about:blank
// with the response's status, the status's text as its title, and the body's start as its
// detail. errors.Is matches a *Problem against any other *Problem of the same type, so each of
// the Err sentinels matches the refusals of its type.
type Problem struct {
	Type     string `json:"type"`     // [ProblemPrefix] followed by the problem's name, or about:blank
	Title    string `json:"title"`    // a short summary of the type
	Status   int    `json:"status"`   // the HTTP status the refusal was answered with
	Detail   string `json:"detail"`   // what about this request was refused
	Instance string `json:"instance"` // the occurrence, when the server names one
}

// Error describes the refusal by its status, its type, and its detail or title.
func (p *Problem) Error() string {
	msg := "bunrei: " + strconv.Itoa(p.Status) + " " + p.Type
	switch {
	case p.Detail != "":
		msg += ": " + p.Detail
	case p.Title != "":
		msg += ": " + p.Title
	}
	return msg
}

// Is reports whether target is a *Problem of the same type, so errors.Is(err, ErrJobNotFound)
// holds for every job-not-found refusal whatever its detail.
func (p *Problem) Is(target error) bool {
	t, ok := target.(*Problem)
	return ok && t.Type == p.Type
}

// The sentinels of the problem types, one per Problem constant, each a *Problem with only its
// type and status, for errors.Is.
var (
	ErrInvalidRequest     = &Problem{Type: ProblemInvalidRequest, Status: http.StatusBadRequest}
	ErrContentTooLarge    = &Problem{Type: ProblemContentTooLarge, Status: http.StatusRequestEntityTooLarge}
	ErrExtractionTooLarge = &Problem{Type: ProblemExtractionTooLarge, Status: http.StatusRequestEntityTooLarge}
	ErrSubmitConflict     = &Problem{Type: ProblemSubmitConflict, Status: http.StatusConflict}
	ErrJobNotFound        = &Problem{Type: ProblemJobNotFound, Status: http.StatusNotFound}
	ErrJobEnded           = &Problem{Type: ProblemJobEnded, Status: http.StatusConflict}
	ErrOutputNotFound     = &Problem{Type: ProblemOutputNotFound, Status: http.StatusNotFound}
	ErrUploadNotFound     = &Problem{Type: ProblemUploadNotFound, Status: http.StatusNotFound}
)

const (
	problemBlank   = "about:blank"
	problemMedia   = "application/problem+json"
	problemBodyMax = 64 << 10
	detailMax      = 512
)

func refusal(res *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(res.Body, problemBodyMax))
	if err != nil {
		return fmt.Errorf("bunrei: read the %d response: %w", res.StatusCode, err)
	}
	if media, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type")); media == problemMedia {
		var p Problem
		if json.Unmarshal(body, &p) == nil {
			if p.Type == "" {
				p.Type = problemBlank
			}
			if p.Status == 0 {
				p.Status = res.StatusCode
			}
			return &p
		}
	}
	detail := strings.TrimSpace(strings.ToValidUTF8(string(body), ""))
	if len(detail) > detailMax {
		detail = strings.ToValidUTF8(detail[:detailMax], "")
	}
	return &Problem{Type: problemBlank, Title: http.StatusText(res.StatusCode), Status: res.StatusCode, Detail: detail}
}
