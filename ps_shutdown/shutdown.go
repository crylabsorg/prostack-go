// Package shutdown implements the k8s lame-duck termination sequence. On
// SIGTERM a service must keep serving briefly: k8s removes the pod from Service
// endpoints asynchronously, and until that propagates the load balancer keeps
// routing to it. Closing the listeners at once turns that traffic into
// connection-refused 5xx. New flips the draining flag (so /readyz returns 503
// and the pod is pulled from the load balancer), waits drainDelay for the
// deregistration to propagate, then calls cancel so the servers stop gracefully.
package ps_shutdown

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// New installs the SIGTERM/Interrupt handler and returns the draining flag to
// feed into healthcheck.Options.Draining. On the first signal it sets the flag
// (so /readyz returns 503 immediately), waits the drain window, then calls
// cancel — pass the cancel of the context that drives ps_http.Run / ps_grpc.Start
// so listeners stay up until the pod has been deregistered.
func New(ctx context.Context, cancel context.CancelFunc) context.Context {
	signalCtx, signalCtxStop := signal.NotifyContext(context.WithoutCancel(ctx), os.Interrupt, syscall.SIGTERM)
	go runDrain(ctx, cancel, signalCtx, signalCtxStop)
	return signalCtx
}

func runDrain(
	ctx context.Context,
	cancel context.CancelFunc,
	signalCtx context.Context,
	signalCtxStop context.CancelFunc,
) {
	select {
	case <-signalCtx.Done():
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		const drainDelay = 10 * time.Second
		time.Sleep(drainDelay)
		cancel()
	case <-ctx.Done():
		if errors.Is(signalCtx.Err(), context.Canceled) {
			return
		}
		signalCtxStop()
	}
}
