package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/crylabsorg/prostack/app1/config"
	"github.com/crylabsorg/prostack/pkg/healthcheck"
	"github.com/crylabsorg/prostack/pkg/logger"
	"github.com/crylabsorg/prostack/pkg/ps_http"
	"github.com/crylabsorg/prostack/pkg/ps_metrics"
	"github.com/crylabsorg/prostack/pkg/shutdown"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.New()
	defer logger.Init(cfg.LogPreset)()
	serviceName := cfg.ServiceName
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger.GetLogger(ctx).Debug("config loaded")
	logger.GetLogger(ctx).Debug("logger inialized")
	draining := shutdown.New(cancel)
	if err != nil {
		logger.GetLogger(ctx).Fatal("config is not loaded", zap.Error(err))
	}
	logger.GetLogger(ctx).Debug("shutdown logic prepared")

	mux := http.NewServeMux()
	logger.GetLogger(ctx).Debug("mux initialized")
	ps_metrics.Register(mux)
	logger.GetLogger(ctx).Debug("metrics registered")
	healthcheck.Register(mux, healthcheck.Options{
		Draining: draining,
	})
	logger.GetLogger(ctx).Debug("healthchecks registered")

	wg := sync.WaitGroup{}
	wg.Go(func() {
		if err1 := ps_http.Run(ctx, cfg.HTTPListenAddr, mux, serviceName); err1 != nil {
			logger.GetLogger(ctx).Fatal("http server error", zap.Error(err1))
		}
	})
	wg.Wait()
	logger.GetLogger(ctx).Info("service gracefully stopped")
}
