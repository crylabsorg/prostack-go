package ps_bootstrap //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/crylabsorg/prostack-go/ps_grpc"
	"github.com/crylabsorg/prostack-go/ps_healthcheck"
	"github.com/crylabsorg/prostack-go/ps_http"
	"github.com/crylabsorg/prostack-go/ps_metrics"
	"github.com/crylabsorg/prostack-go/ps_otel"
)

type Config interface {
	GetLogPreset() logger.Preset
	GetServiceName() string
	GetHTTPListenAddr() string
	GetGRPCListenAddr() string
}

type App struct {
	mux               *http.ServeMux
	healthOptions     *ps_healthcheck.Options
	grpcRegistrations []func(*grpc.Server)
	grpcHealth        *health.Server
	metricsRegistry   ps_metrics.Registry
	cleanups          []func() error
	workers           []<-chan struct{}
}

// Run owns the common lifecycle. setup registers handlers and probes before
// either listener starts. setup receives a work context, canceled after handlers
// finish or their grace period expires, and a draining context, canceled as soon
// as shutdown starts to stop background workers from taking new work.
func Run[C Config](getConfig func() (C, error), setup func(context.Context, context.Context, C, *App) error) {
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()

	cfg := loadConfig(workCtx, getConfig)
	defer logger.Init(cfg.GetLogPreset())()

	drainingCtx, startDraining := signal.NotifyContext(workCtx, os.Interrupt, syscall.SIGTERM)
	defer startDraining()

	app := newApp(drainingCtx, cfg.GetGRPCListenAddr())
	defer ps_otel.InitTraces(workCtx, cfg.GetServiceName())()
	defer ps_otel.InitMetrics(workCtx, app.mux)()
	if err := setup(workCtx, drainingCtx, cfg, app); err != nil {
		logger.GetLogger(workCtx).Fatal("setup failed", zap.Error(err))
	}

	// Servers keep accepting requests during the load-balancer delay.
	stopAccepting := make(chan struct{})
	var servers sync.WaitGroup
	servers.Go(func() {
		if err := ps_http.Run(
			workCtx,
			stopAccepting,
			cfg.GetHTTPListenAddr(),
			app.mux,
			cfg.GetServiceName(),
		); err != nil {
			logger.GetLogger(workCtx).Error("http server failed", zap.Error(err))
			startDraining()
		}
	})
	if app.grpcHealth != nil {
		servers.Go(func() {
			if err := ps_grpc.Run(
				workCtx,
				stopAccepting,
				cfg.GetGRPCListenAddr(),
				app.grpcHealth,
				app.registerGRPCServices,
			); err != nil {
				logger.GetLogger(workCtx).Error("grpc server failed", zap.Error(err))
				startDraining()
			}
		})
	}

	// Draining disables readiness and tells workers to stop taking new work.
	<-drainingCtx.Done()
	logger.GetLogger(workCtx).Info("draining")
	if app.grpcHealth != nil {
		app.grpcHealth.Shutdown()
	}
	const loadBalancerDelay = 10 * time.Second
	time.Sleep(loadBalancerDelay)
	close(stopAccepting)

	// The timer cancels active work even while the main goroutine is waiting.
	// Servers and workers run concurrently and share this one grace period.
	const gracePeriod = 30 * time.Second
	graceTimer := time.AfterFunc(gracePeriod, func() {
		logger.GetLogger(workCtx).Warn("shutdown grace period expired; canceling active handlers")
		cancelWork()
	})
	servers.Wait() // Close server connections before cleaning up dependencies.
	app.waitForWorkers(workCtx)
	graceTimer.Stop()
	cancelWork()

	for _, cleanup := range slices.Backward(app.cleanups) {
		if err := cleanup(); err != nil {
			logger.GetLogger(workCtx).Error("cleanup failed", zap.Error(err))
		}
	}
}

func loadConfig[C Config](ctx context.Context, getConfig func() (C, error)) C {
	defer logger.Init(logger.PresetProduction)()
	cfg, err := getConfig()
	if err != nil {
		logger.GetLogger(ctx).Fatal("config loading error", zap.Error(err))
	}
	return cfg
}

func newApp(drainingCtx context.Context, grpcAddress string) *App {
	app := &App{
		mux: http.NewServeMux(),
		healthOptions: &ps_healthcheck.Options{
			Probes:      make(map[string]ps_healthcheck.Probe),
			DrainingCtx: drainingCtx,
		},
		metricsRegistry: ps_metrics.New(),
	}
	if grpcAddress != "" {
		app.grpcHealth = health.NewServer()
	}
	app.metricsRegistry.Register(app.mux)
	ps_healthcheck.Register(app.mux, app.healthOptions)
	return app
}

// waitForWorkers waits for graceful completion, or releases the wait when the
// work context is canceled so a stuck worker cannot block dependency cleanup.
func (a *App) waitForWorkers(ctx context.Context) {
	for _, done := range a.workers {
		select {
		case <-done:
		case <-ctx.Done():
			return
		}
	}
}

// RegisterWorker tracks a background worker separately from dependency cleanup.
// done must close after the worker and its active handlers have finished.
func (a *App) RegisterWorker(done <-chan struct{}) {
	a.workers = append(a.workers, done)
}

// RegisterCleanup registers dependency cleanup in reverse registration order,
// after graceful completion or cancellation of active work.
func (a *App) RegisterCleanup(cleanup func() error) {
	a.cleanups = append(a.cleanups, cleanup)
}

func (a *App) RegisterHealthProbe(ctx context.Context, name string, probe ps_healthcheck.Probe) {
	if _, ok := a.healthOptions.Probes[name]; ok {
		logger.GetLogger(ctx).Fatal("probe already exists", zap.String("name", name))
	}
	a.healthOptions.Probes[name] = probe
}

func (a *App) GetMetricsRegistry() *prometheus.Registry {
	return a.metricsRegistry.Registry
}

func (a *App) GetHTTPServeMux() *http.ServeMux {
	return a.mux
}

// RegisterGRPC registers application services before the gRPC listener starts.
func (a *App) RegisterGRPC(register func(*grpc.Server)) {
	a.grpcRegistrations = append(a.grpcRegistrations, register)
}

func (a *App) registerGRPCServices(server *grpc.Server) {
	for _, register := range a.grpcRegistrations {
		register(server)
	}
}
