package main

import (
	"context"

	"github.com/crylabsorg/prostack/app1/config"
	"github.com/crylabsorg/prostack/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.New()
	defer logger.Init(cfg.LogPreset)()
	ctx := context.Background()
	if err != nil {
		logger.GetLogger(ctx).Fatal("config is not loaded", zap.Error(err))
	}

	logger.GetLogger(ctx).Info("Hello, World!")
}
