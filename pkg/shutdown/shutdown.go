// Package shutdown implements the k8s lame-duck termination sequence. On
// SIGTERM a service must keep serving briefly: k8s removes the pod from Service
// endpoints asynchronously, and until that propagates the load balancer keeps
// routing to it. Closing the listeners at once turns that traffic into
// connection-refused 5xx. New flips the draining flag (so /readyz returns 503
// and the pod is pulled from the load balancer), waits drainDelay for the
// deregistration to propagate, then calls cancel so the servers stop gracefully.
package shutdown

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/crylabsorg/prostack/pkg/logger"
)

const drainDelay = 10 * time.Second

// New installs the SIGTERM/Interrupt handler and returns the draining flag to
// feed into healthcheck.Options.Draining. On the first signal it sets the flag
// (so /readyz returns 503 immediately), waits the drain window, then calls
// cancel — pass the cancel of the context that drives su_http.Run / su_grpc.Start
// so listeners stay up until the pod has been deregistered.
func New(cancel context.CancelFunc) *atomic.Bool {
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	draining := &atomic.Bool{}
	go runDrain(signalCtx, stop, cancel, draining, drainDelay)
	return draining
}

func runDrain(
	signalCtx context.Context,
	stop context.CancelFunc,
	cancel context.CancelFunc,
	draining *atomic.Bool,
	delay time.Duration,
) {
	<-signalCtx.Done()
	stop()
	log := logger.GetLogger(signalCtx)
	log.Info("start shutdown, draining readiness")
	draining.Store(true)
	time.Sleep(delay)
	log.Info("drain window elapsed, stopping servers")
	cancel()
}
