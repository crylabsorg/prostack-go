package ps_swagger //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

//go:embed swaggerui
var swaggerFS embed.FS

func SetupSwagger(ctx context.Context, mux *http.ServeMux, swaggerSpec []byte) {
	mux.HandleFunc("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err1 := w.Write(swaggerSpec); err1 != nil {
			logger.GetLogger(r.Context()).Error("failed to write swagger spec", zap.Error(err1))
		}
	})

	swaggerSubFS, err := fs.Sub(swaggerFS, "internal/swaggerui")
	if err != nil {
		logger.GetLogger(ctx).Fatal("failed to create swagger sub filesystem", zap.Error(err))
	}
	mux.Handle("/swagger/", http.StripPrefix("/swagger/", http.FileServer(http.FS(swaggerSubFS))))
}
