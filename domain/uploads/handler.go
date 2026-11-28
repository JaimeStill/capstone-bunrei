package uploads

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/go-web-sdk/upload"

	"github.com/JaimeStill/capstone-bunrei/client"
)

// Routes builds the upload route group, /uploads: go-web-sdk's upload.Handler over service,
// whose POST /uploads creates an upload and whose HEAD, PATCH, and DELETE /uploads/{id} read its
// offset, append to it, and cancel it. transfer sets each request's connection deadlines, its
// limit the chunk size, the most content one creation or append carries; an upload expires
// lifetime after its creation. complete is called once with each completed upload's file, as
// the handler's Complete; on its success the completing request is answered 200 with the
// upload's ID as [client.Upload], and the handler deletes the upload's rows and chunks. logger
// records the cause of every 5xx the group sends.
func Routes(service *Service, complete func(ctx context.Context, f upload.File, content io.Reader) error, transfer web.Transfer, lifetime time.Duration, logger *slog.Logger) *web.Group {
	return upload.Handler{
		Store:    service,
		Transfer: transfer,
		Lifetime: lifetime,
		Complete: func(w http.ResponseWriter, r *http.Request, f upload.File, content io.Reader) error {
			if err := complete(r.Context(), f, content); err != nil {
				return err
			}
			if err := transfer.WidenDownload(w, 0); err != nil {
				return err
			}
			return web.WriteJSON(w, http.StatusOK, client.Upload{ID: f.ID})
		},
		Logger: logger,
	}.Routes("/uploads")
}
