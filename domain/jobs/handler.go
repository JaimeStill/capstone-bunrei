package jobs

import (
	"errors"
	"log/slog"
	"net/http"
	"path"

	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/client"
)

type handler struct {
	service *Service
}

// Routes builds the job root's route group, /jobs: GET /jobs/{id} is GetJob, answered 200 with
// the job; POST /jobs/{id}/cancel is CancelJob, answered 202 with no body and the job's
// Location, where GetJob reads the outcome. logger records the cause of every 5xx the group's
// error writer sends. A refusal is a problem whose type is one of client's Problem constants:
// ErrInvalidID, [client.ProblemInvalidRequest]; ErrNotFound, [client.ProblemJobNotFound]; and
// ErrEnded, [client.ProblemJobEnded].
func Routes(service *Service, logger *slog.Logger) *web.Group {
	h := &handler{service: service}
	ew := web.NewErrorWriter(logger, status)
	ew.Detail(http.StatusNotFound, http.StatusConflict)
	g := web.NewGroup("/jobs")
	g.SetErrorWriter(ew)
	g.HandleErr("GET", "/{id}", h.get)
	g.HandleErr("POST", "/{id}/cancel", h.cancel)
	return g
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) error {
	j, err := h.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return web.WriteJSON(w, http.StatusOK, j)
}

func (h *handler) cancel(w http.ResponseWriter, r *http.Request) error {
	id, err := canonical(r.PathValue("id"))
	if err != nil {
		return err
	}
	if err := h.service.Cancel(r.Context(), id); err != nil {
		return err
	}
	w.Header().Set("Location", path.Join(path.Dir(path.Dir(r.URL.Path)), id))
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func status(err error) (web.Problem, bool) {
	for _, p := range []struct {
		err    error
		typ    string
		status int
	}{
		{ErrInvalidID, client.ProblemInvalidRequest, http.StatusBadRequest},
		{ErrNotFound, client.ProblemJobNotFound, http.StatusNotFound},
		{ErrEnded, client.ProblemJobEnded, http.StatusConflict},
	} {
		if errors.Is(err, p.err) {
			return web.Problem{Type: p.typ, Status: p.status}, true
		}
	}
	return web.Problem{}, false
}
