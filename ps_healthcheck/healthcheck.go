// Package healthcheck exposes the standard k8s probes (/healthz, /readyz,
// /startupz) on an HTTP mux. /healthz is liveness — always 200 while the
// process is up. /readyz runs caller-supplied probes (Mongo / Redis pings)
// and returns 503 if any probe fails; this is what k8s should be wired to.
package ps_healthcheck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/crylabsorg/prostack-go/logger"
)

// Probe is a readiness check. It must respect ctx and return quickly (typically
// well under the readyzTimeout below).
type Probe func(ctx context.Context) error

// Options configures Register. Probes is a name→check map; the readyz handler
// fans out and reports the first failure. StartupReady toggles /startupz from
// 503 to 200 when set; if nil, /startupz mirrors /healthz. Draining, when set
// and true, forces /readyz to 503 without running probes — this is the
// lame-duck signal flipped on SIGTERM so k8s pulls the pod out of the load
// balancer before the servers shut down.
type Options struct {
	Probes       map[string]Probe
	StartupReady *atomic.Bool
	Draining     *atomic.Bool
}

// readyzTimeout caps how long a single readyz fan-out blocks. Long-running
// pings would otherwise stack up under k8s's default 10s probe interval.
const readyzTimeout = 3 * time.Second

type handler struct {
	probes       map[string]Probe
	startupReady *atomic.Bool
	draining     *atomic.Bool
}

// Register installs the standard probes on mux. For deployments without
// dependency probes, pass an empty Options{} — readyz then trivially returns
// 200.
func Register(mux *http.ServeMux, opt Options) {
	h := &handler{
		probes:       opt.Probes,
		startupReady: opt.StartupReady,
		draining:     opt.Draining,
	}
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /startupz", h.startupz)
}

func (h *handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *handler) startupz(w http.ResponseWriter, _ *http.Request) {
	if h.startupReady != nil && !h.startupReady.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) readyz(w http.ResponseWriter, r *http.Request) {
	if h.draining != nil && h.draining.Load() {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte("draining"))
		logger.GetLogger(r.Context()).Warn("draining write failed", zap.Error(err))
		return
	}
	if len(h.probes) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
	defer cancel()

	failures := make(chan error, len(h.probes))
	var wg sync.WaitGroup

	for name, probe := range h.probes {
		wg.Go(func() {
			if err := probe(ctx); err != nil {
				logger.GetLogger(ctx).Warn(
					"readyz probe failed",
					zap.String("probe", name),
					zap.Error(err),
				)
				failures <- fmt.Errorf("%s: %w", name, err)
			}
		})
	}

	wg.Wait()
	close(failures)

	var errs []error
	for err := range failures {
		errs = append(errs, err)
	}

	if len(failures) > 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte(errors.Join(errs...).Error()))
		logger.GetLogger(r.Context()).Warn("failures write failed", zap.Error(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}
