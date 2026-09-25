package ps_bootstrap

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/crylabsorg/prostack-go/ps_grpc"
	"github.com/crylabsorg/prostack-go/ps_healthcheck"
	"github.com/crylabsorg/prostack-go/ps_http"
	"github.com/crylabsorg/prostack-go/ps_metrics"
	"github.com/crylabsorg/prostack-go/ps_otel"
	"github.com/crylabsorg/prostack-go/ps_shutdown"
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
	Server       *grpc.Server
	HealthServer *health.Server
}

type App struct {
	Mux             *http.ServeMux
	Health          *ps_healthcheck.Options
	GRPC            *GRPCSettings
	MetricsRegistry ps_metrics.Registry
}

// Run owns the common lifecycle. setup registers handlers and probes before
// either listener starts.
func Run[C Config](getConfig func() (C, error), setup func(context.Context, C, *App) error) {
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
	defer ps_otel.InitOTELTraces(ctx, cfg.GetServiceName())()
	defer ps_otel.InitOTELMetrics(ctx, mux)()

	app := &App{}
	app.Mux = mux
	app.Health = &ps_healthcheck.Options{
		Probes:       make(map[string]ps_healthcheck.Probe),
		StartupReady: &atomic.Bool{},
		Draining:     ps_shutdown.New(cancel),
	}
	if cfg.GetGRPCListenAddr() != "" {
		hs := health.NewServer()
		app.GRPC = &GRPCSettings{}
		app.GRPC.HealthServer = hs
		app.GRPC.Server = ps_grpc.NewServer(hs)
	}
	app.MetricsRegistry = ps_metrics.New()
	app.MetricsRegistry.Register(app.Mux)
	ps_healthcheck.Register(app.Mux, app.Health)

	if err := setup(ctx, cfg, app); err != nil {
		logger.GetLogger(ctx).Fatal("setup failed", zap.Error(err))
	}

	wg := sync.WaitGroup{}
	wg.Go(func() {
		if err1 := ps_http.Run(ctx, cfg.GetHTTPListenAddr(), app.Mux, cfg.GetServiceName()); err1 != nil {
			logger.GetLogger(ctx).Error("http run errored", zap.Error(err1))
		}
	})
	if app.GRPC != nil {
		wg.Go(func() {
			ps_grpc.Start(ctx, cfg.GetGRPCListenAddr(), app.GRPC.Server)
		})
	}
	wg.Wait()
}
