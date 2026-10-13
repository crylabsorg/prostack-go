package ps_http //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

func newServer(addr string, handler http.Handler, serverName string) *http.Server {
	const readTimeout = 30 * time.Second
	const writeTimeout = 30 * time.Second
	const idleTimeout = 5 * time.Second
	return &http.Server{
		Addr:         addr,
		Handler:      otelhttp.NewHandler(handler, serverName),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
}

func Run(ctx context.Context, addr string, handler http.Handler, serverName string) error {
	s := newServer(addr, handler, serverName)

	errChan := make(chan error, 1)

	// Start the server in a goroutine.
	go func() {
		logger.GetLogger(ctx).Info("Server is starting", zap.String("addr", s.Addr))
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case err := <-errChan:
		// Listener failed to start (e.g. the port is already in use) before a
		// shutdown was requested. Surface it immediately so main can fail fast
		// instead of running on without its server until SIGTERM.
		return err
	case <-ctx.Done():
	}

	logger.GetLogger(ctx).Info("Start shutdown server")

	// Give outstanding requests a deadline for completion.
	const timeout = 5 * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	// Asking listener to shut down and shed load.
	err := s.Shutdown(shutdownCtx)
	if err != nil {
		return err
	}

	err = <-errChan
	logger.GetLogger(ctx).Info("Server shutdown complete")
	return err
}
