package submissions

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"

	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/client"
)

const bodySlack = 64 << 10

type handler struct {
	service  *Service
	transfer web.Transfer
}

// Routes builds the layer's route group, /jobs, whose POST is Submit: a body of
// [client.Submit]'s JSON, its content base64, answered 202 with the job, queued, and its
// Location. The body is bounded at the small-document limit's base64 length plus
// [ExtractionLimit] and room for the other members, and transfer sizes its read's deadline from
// that bound. logger records the cause of every 5xx the group's error writer sends. A refusal is
// a problem whose type is one of client's Problem constants: a body that is not the JSON,
// [ErrInvalid], [client.ProblemInvalidRequest]; a body past its bound or ErrContentTooLarge,
// [client.ProblemContentTooLarge]; ErrExtractionTooLarge, [client.ProblemExtractionTooLarge];
// and ErrConflict, [client.ProblemSubmitConflict].
func Routes(service *Service, transfer func(limit int64) web.Transfer, logger *slog.Logger) *web.Group {
	bound := int64(base64.StdEncoding.EncodedLen(int(service.Limit()))) + ExtractionLimit + bodySlack
	h := &handler{service: service, transfer: transfer(bound)}
	ew := web.NewErrorWriter(logger, status)
	ew.Detail(http.StatusConflict)
	g := web.NewGroup("/jobs")
	g.SetErrorWriter(ew)
	g.HandleErr("POST", "", h.submit)
	return g
}

// UploadRoutes builds the CompleteUpload route group, /uploads, whose POST /uploads/{id}/complete
// is CompleteUpload of the completed upload {id}: a body of [client.CompleteUpload]'s JSON, its
// upload member {id} or empty, answered 202 with the job, queued, and its Location under /jobs.
// The body is bounded at [ExtractionLimit] and room for the other members, and transfer sizes
// its read's deadline from that bound. logger records the cause of every 5xx the group's error
// writer sends. A refusal is a problem whose type is one of client's Problem constants: a body
// that is not the JSON, an upload member other than {id}, or [ErrInvalid],
// [client.ProblemInvalidRequest]; a body past its bound, [client.ProblemContentTooLarge];
// ErrExtractionTooLarge, [client.ProblemExtractionTooLarge]; ErrUploadNotFound,
// [client.ProblemUploadNotFound]; and ErrConflict, [client.ProblemSubmitConflict].
func UploadRoutes(service *Service, transfer func(limit int64) web.Transfer, logger *slog.Logger) *web.Group {
	h := &handler{service: service, transfer: transfer(ExtractionLimit + bodySlack)}
	ew := web.NewErrorWriter(logger, status)
	ew.Detail(http.StatusNotFound, http.StatusConflict)
	g := web.NewGroup("/uploads")
	g.SetErrorWriter(ew)
	g.HandleErr("POST", "/{id}/complete", h.completeUpload)
	return g
}

func (h *handler) submit(w http.ResponseWriter, r *http.Request) error {
	if err := h.transfer.WidenUpload(w, r); err != nil {
		return err
	}
	req, err := decode[client.Submit](w, r, h.transfer.Limit())
	if err != nil {
		return err
	}
	j, err := h.service.Submit(r.Context(), req)
	if err != nil {
		return err
	}
	w.Header().Set("Location", r.URL.Path+"/"+j.ID)
	return web.WriteJSON(w, http.StatusAccepted, j)
}

func (h *handler) completeUpload(w http.ResponseWriter, r *http.Request) error {
	if err := h.transfer.WidenUpload(w, r); err != nil {
		return err
	}
	req, err := decode[client.CompleteUpload](w, r, h.transfer.Limit())
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if req.Upload != "" && req.Upload != id {
		return fmt.Errorf("%w: the body names upload %q, the path %q", ErrInvalid, req.Upload, id)
	}
	req.Upload = id
	j, err := h.service.CompleteUpload(r.Context(), req)
	if err != nil {
		return err
	}
	w.Header().Set("Location", path.Join(path.Dir(path.Dir(path.Dir(r.URL.Path))), "jobs", j.ID))
	return web.WriteJSON(w, http.StatusAccepted, j)
}

func decode[T any](w http.ResponseWriter, r *http.Request, limit int64) (T, error) {
	req, err := web.DecodeJSON[T](w, r, limit)
	if be, ok := errors.AsType[*web.BodyError](err); ok {
		if be.TooLarge {
			return req, fmt.Errorf("%w: the body %s", ErrContentTooLarge, be.Reason)
		}
		return req, fmt.Errorf("%w: %s", ErrInvalid, be.Error())
	}
	return req, err
}

func status(err error) (web.Problem, bool) {
	for _, p := range []struct {
		err    error
		typ    string
		status int
	}{
		{ErrInvalid, client.ProblemInvalidRequest, http.StatusBadRequest},
		{ErrContentTooLarge, client.ProblemContentTooLarge, http.StatusRequestEntityTooLarge},
		{ErrExtractionTooLarge, client.ProblemExtractionTooLarge, http.StatusRequestEntityTooLarge},
		{ErrConflict, client.ProblemSubmitConflict, http.StatusConflict},
		{ErrUploadNotFound, client.ProblemUploadNotFound, http.StatusNotFound},
	} {
		if errors.Is(err, p.err) {
			return web.Problem{Type: p.typ, Status: p.status}, true
		}
	}
	return web.Problem{}, false
}
