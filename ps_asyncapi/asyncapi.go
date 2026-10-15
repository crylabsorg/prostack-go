package ps_asyncapi //nolint:staticcheck // Match the framework package naming convention.

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

//go:embed ui
var documentation embed.FS

// SetupAsyncAPI serves the supplied specification and the embedded AsyncAPI UI.
func SetupAsyncAPI(ctx context.Context, mux *http.ServeMux, asyncapiSpec []byte) {
	mux.HandleFunc("GET /asyncapi", serveUI)
	mux.HandleFunc("GET /asyncapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			return
		}
		if _, err := w.Write(asyncapiSpec); err != nil {
			logger.GetLogger(ctx).Error("failed to write asyncapi spec", zap.Error(err))
		}
	})

	asyncapiSubFS, err := fs.Sub(documentation, "ui")
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to create asyncapi sub filesystem", zap.Error(err))
	}
	mux.Handle("GET /asyncapi/", http.StripPrefix("/asyncapi/", http.FileServerFS(asyncapiSubFS)))
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, documentation, "ui/index.html")
}
