package ps_bootstrap

import (
	"context"
	"net/http"
	"sync"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/crylabsorg/prostack-go/ps_grpc"
	"github.com/crylabsorg/prostack-go/ps_healthcheck"
	"github.com/crylabsorg/prostack-go/ps_http"
	"github.com/crylabsorg/prostack-go/ps_metrics"
	"github.com/crylabsorg/prostack-go/ps_shutdown"
	"go.uber.org/zap"
)

type Config interface {
	GetLogPreset() logger.Preset
	GetServiceName() string
	GetHTTPListenAddr() string
	GetGRPCListenAddr() string
}

func Bootstrap(cfg Config, mux *http.ServeMux, grpcServer ps_grpc.GRPCServer) {
	logPreset := logger.PresetProduction
	if cfg != nil {
		logPreset = cfg.GetLogPreset()
	}
	defer logger.Init(logPreset)()
	serviceName := cfg.GetServiceName()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger.GetLogger(ctx).Debug("config loaded")
	logger.GetLogger(ctx).Debug("logger inialized")
	draining := ps_shutdown.New(cancel)
	logger.GetLogger(ctx).Debug("shutdown logic prepared")

	logger.GetLogger(ctx).Debug("mux initialized")
	ps_metrics.Register(mux)
	logger.GetLogger(ctx).Debug("metrics registered")
	ps_healthcheck.Register(mux, ps_healthcheck.Options{
		Draining: draining,
	})
	logger.GetLogger(ctx).Debug("healthchecks registered")

	wg := sync.WaitGroup{}
	wg.Go(func() {
		if err1 := ps_http.Run(ctx, cfg.GetHTTPListenAddr(), mux, serviceName); err1 != nil {
			logger.GetLogger(ctx).Fatal("http server error", zap.Error(err1))
		}
	})
	wg.Go(func() {
		ps_grpc.Start(ctx, cfg.GetGRPCListenAddr(), grpcServer)
	})
	wg.Wait()
	logger.GetLogger(ctx).Info("service gracefully stopped")
}
