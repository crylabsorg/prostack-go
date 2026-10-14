package ps_http //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

// Run serves until stopAccepting closes, then waits for active requests.
// Canceling ctx cancels request contexts and closes remaining connections.
// Run returns after serving and shutdown have finished.
func Run(
	ctx context.Context,
	stopAccepting <-chan struct{},
	addr string,
	handler http.Handler,
	serverName string,
) error {
	server := newServer(ctx, addr, handler, serverName)
	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(ctx, "tcp", server.Addr)
	if err != nil {
		return err
	}
	logger.GetLogger(ctx).Info("running http server", zap.String("address", listener.Addr().String()))
	return serve(ctx, stopAccepting, server, listener)
}

func serve(ctx context.Context, stopAccepting <-chan struct{}, server *http.Server, listener net.Listener) error {
	serveResult := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveResult <- err
	}()

	select {
	case err := <-serveResult:
		return errors.Join(err, server.Close())
	case <-stopAccepting:
	case <-ctx.Done():
	}

	shutdownErr := server.Shutdown(ctx)
	if shutdownErr != nil {
		closeErr := server.Close()
		if errors.Is(shutdownErr, ctx.Err()) {
			shutdownErr = nil // Context cancellation is a normal forced shutdown.
		}
		shutdownErr = errors.Join(shutdownErr, closeErr)
	}
	return errors.Join(shutdownErr, <-serveResult)
}

func newServer(ctx context.Context, addr string, handler http.Handler, serverName string) *http.Server {
	const readTimeout = 30 * time.Second
	const writeTimeout = 30 * time.Second
	const idleTimeout = 5 * time.Second
	return &http.Server{
		Addr:         addr,
		Handler:      otelhttp.NewHandler(handler, serverName),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
		BaseContext:  func(net.Listener) context.Context { return ctx },
	}
}
