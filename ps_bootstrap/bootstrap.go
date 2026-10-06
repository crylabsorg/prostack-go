package ps_bootstrap

import (
	"context"
	"net/http"
	"slices"
	"sync"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/crylabsorg/prostack-go/ps_grpc"
	"github.com/crylabsorg/prostack-go/ps_healthcheck"
	"github.com/crylabsorg/prostack-go/ps_http"
	"github.com/crylabsorg/prostack-go/ps_metrics"
	"github.com/crylabsorg/prostack-go/ps_otel"
	"github.com/crylabsorg/prostack-go/ps_shutdown"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
)

type Config interface {
	GetLogPreset() logger.Preset
	GetServiceName() string
	GetHTTPListenAddr() string
	GetGRPCListenAddr() string
}

type GRPCSettings struct {
	Server *grpc.Server
}

type App struct {
	mux             *http.ServeMux
	healthOptions   *ps_healthcheck.Options
	grpcSettings    *GRPCSettings
	metricsRegistry ps_metrics.Registry
	drainingCtx     context.Context
	cleanups        []func() error
}

// Run owns the common lifecycle. setup registers handlers and probes before
// either listener starts.
func Run[C Config](getConfig func() (C, error), setup func(context.Context, context.Context, C, *App) error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := func() C {
		defer logger.Init(logger.PresetProduction)()
		cfg, err := getConfig()
		if err != nil {
			logger.GetLogger(ctx).Fatal("config loading error", zap.Error(err))
		}
		return cfg
	}()

	defer logger.Init(cfg.GetLogPreset())()

	mux := http.NewServeMux()
	defer ps_otel.InitTraces(ctx, cfg.GetServiceName())()
	defer ps_otel.InitMetrics(ctx, mux)()

	drainingCtx := ps_shutdown.New(ctx, cancel)

	wg := sync.WaitGroup{}
	defer wg.Wait()
	app := &App{}
	wg.Go(func() {
		<-drainingCtx.Done()
		logger.GetLogger(ctx).Info("draining")
		for _, cleanup := range slices.Backward(app.cleanups) {
			if err := cleanup(); err != nil {
				logger.GetLogger(ctx).Error("cleanup failed", zap.Error(err))
			}
		}
	})
	app.mux = mux
	app.healthOptions = &ps_healthcheck.Options{
		Probes:       make(map[string]ps_healthcheck.Probe),
		StartupReady: nil,
		DrainingCtx:  drainingCtx,
	}
	if cfg.GetGRPCListenAddr() != "" {
		hs := health.NewServer()
		app.RegisterCleanup(func() error {
			hs.Shutdown()
			return nil
		})
		app.grpcSettings = &GRPCSettings{}
		app.grpcSettings.Server = ps_grpc.NewServer(hs)
	}
	app.metricsRegistry = ps_metrics.New()
	app.metricsRegistry.Register(app.mux)
	ps_healthcheck.Register(app.mux, app.healthOptions)

	if err := setup(ctx, drainingCtx, cfg, app); err != nil {
		logger.GetLogger(ctx).Fatal("setup failed", zap.Error(err))
	}

	wg.Go(func() {
		if err1 := ps_http.Run(ctx, cfg.GetHTTPListenAddr(), app.mux, cfg.GetServiceName()); err1 != nil {
			logger.GetLogger(ctx).Error("http run errored", zap.Error(err1))
		}
	})
	if app.grpcSettings != nil {
		wg.Go(func() {
			ps_grpc.Start(ctx, cfg.GetGRPCListenAddr(), app.grpcSettings.Server)
		})
	}
}

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

func (a *App) GetGRPCServer() *grpc.Server {
	return a.grpcSettings.Server
}
