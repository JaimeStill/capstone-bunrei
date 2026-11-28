package outputs

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/client"
)

const window = 64 << 10

type handler struct {
	service  *Service
	transfer web.Transfer
}

// Routes builds the layer's route group, /outputs, whose GET /outputs/{sha256} is GetOutput: the
// content stored under the SHA-256, streamed as application/octet-stream with the SHA-256 as its
// ETag, or 304 for a matching If-None-Match. The content store reports no size, so the body's
// write deadline is extended as it flows: before each 64 KiB or less of it is written, transfer,
// built for that window, sets the deadline to its grace plus that piece's time at its rate, so a
// download that keeps up the rate is never cut off and one that stalls is. logger records the
// cause of every 5xx the group's error writer sends. A refusal is a problem whose type is one of client's Problem constants:
// ErrInvalid, [client.ProblemInvalidRequest]; and ErrNotFound, [client.ProblemOutputNotFound].
func Routes(service *Service, transfer func(limit int64) web.Transfer, logger *slog.Logger) *web.Group {
	h := &handler{service: service, transfer: transfer(window)}
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
	paced := &pacedWriter{ResponseWriter: w, transfer: h.transfer}
	return web.WriteObject(paced, r, o, func() (io.ReadCloser, error) { return io.NopCloser(body), nil })
}

type pacedWriter struct {
	http.ResponseWriter
	transfer web.Transfer
}

func (p *pacedWriter) Write(b []byte) (int, error) {
	var n int
	for len(b) > 0 {
		piece := b[:min(int64(len(b)), p.transfer.Limit())]
		if err := p.transfer.WidenDownload(p.ResponseWriter, int64(len(piece))); err != nil {
			return n, err
		}
		m, err := p.ResponseWriter.Write(piece)
		n += m
		if err != nil {
			return n, err
		}
		b = b[len(piece):]
	}
	return n, nil
}

func (p *pacedWriter) Unwrap() http.ResponseWriter { return p.ResponseWriter }

func status(err error) (web.Problem, bool) {
	switch {
	case errors.Is(err, ErrInvalid):
		return web.Problem{Type: client.ProblemInvalidRequest, Status: http.StatusBadRequest}, true
	case errors.Is(err, ErrNotFound):
		return web.Problem{Type: client.ProblemOutputNotFound, Status: http.StatusNotFound}, true
	}
	return web.Problem{}, false
}
