package outputs

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/client"
)

type handler struct {
	service *Service
}

// Routes builds the layer's route group, /outputs, whose GET /outputs/{sha256} is GetOutput: the
// content stored under the SHA-256, streamed as application/octet-stream with the SHA-256 as its
// ETag, or 304 for a matching If-None-Match. logger records the cause of every 5xx the group's
// error writer sends. A refusal is a problem whose type is one of client's Problem constants:
// ErrInvalid, [client.ProblemInvalidRequest]; and ErrNotFound, [client.ProblemOutputNotFound].
func Routes(service *Service, logger *slog.Logger) *web.Group {
	h := &handler{service: service}
	ew := web.NewErrorWriter(logger, status)
	ew.Detail(http.StatusNotFound)
	g := web.NewGroup("/outputs")
	g.SetErrorWriter(ew)
	g.HandleErr("GET", "/{sha256}", h.get)
	return g
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) error {
	sha256 := r.PathValue("sha256")
	body, err := h.service.Open(r.Context(), sha256)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	o := web.Object{ContentType: "application/octet-stream", Size: -1, ETag: `"` + sha256 + `"`}
	return web.WriteObject(w, r, o, func() (io.ReadCloser, error) { return io.NopCloser(body), nil })
}

func status(err error) (web.Problem, bool) {
	switch {
	case errors.Is(err, ErrInvalid):
		return web.Problem{Type: client.ProblemInvalidRequest, Status: http.StatusBadRequest}, true
	case errors.Is(err, ErrNotFound):
		return web.Problem{Type: client.ProblemOutputNotFound, Status: http.StatusNotFound}, true
	}
	return web.Problem{}, false
}
