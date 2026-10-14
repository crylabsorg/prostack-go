package ps_http //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunPreservesActiveRequestUntilCompletionOrCancellation(t *testing.T) {
	t.Parallel()
	for _, force := range []bool{false, true} {
		name := "graceful"
		if force {
			name = "forced"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			started, finish := make(chan struct{}), make(chan struct{})
			handlerResult := make(chan error, 1)
			listenConfig := net.ListenConfig{}
			listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			server := newServer(ctx, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-finish:
				case <-r.Context().Done():
				}
				handlerResult <- r.Context().Err()
				w.WriteHeader(http.StatusNoContent)
			}), "shutdown-test")
			shutdownStarted := make(chan struct{})
			server.RegisterOnShutdown(func() { close(shutdownStarted) })
			stopAccepting := make(chan struct{})
			runResult := make(chan error, 1)
			go func() { runResult <- serve(ctx, stopAccepting, server, listener) }()
			clientResult := make(chan error, 1)
			go func() {
				clientResult <- get(ctx, http.DefaultClient, "http://"+listener.Addr().String())
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("request did not start")
			}
			close(stopAccepting)
			select {
			case <-shutdownStarted:
			case <-ctx.Done():
				t.Fatal("shutdown did not start")
			}
			require.NoError(t, ctx.Err(), "graceful shutdown must preserve the work context")
			select {
			case <-runResult:
				t.Fatal("Run returned while a request was still active")
			default:
			}
			if force {
				cancel()
				require.ErrorIs(t, <-handlerResult, context.Canceled)
				require.Error(t, <-clientResult)
			} else {
				close(finish)
				require.NoError(t, <-handlerResult)
				require.NoError(t, <-clientResult)
			}
			require.NoError(t, <-runResult)
		})
	}
}

func get(ctx context.Context, client *http.Client, url string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	return response.Body.Close()
}
