package logger

import (
	"time"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Preset string

const (
	PresetProduction  Preset = "production"
	PresetDevelopment Preset = "development"
)

func Init(preset Preset) func() {
	isBadPreset := false

	var cfg zap.Config
	switch preset {
	case PresetProduction:
		cfg = getProductionConfig()
	case PresetDevelopment:
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	default:
		// Use the production preset by default so logs are emitted in the expected format.
		cfg = getProductionConfig()
		isBadPreset = true
	}

	log, err := cfg.Build(
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(
				traceCore{core},
				otelzap.NewCore("logger"),
			)
		}),
		zap.IncreaseLevel(cfg.Level),
	)
	if err != nil {
		panic(err)
	}

	if isBadPreset {
		log.Fatal("invalid preset", zap.String("preset", string(preset)))
	}

	zap.ReplaceGlobals(log)

	return func() { _ = log.Sync() }
}

func getProductionConfig() zap.Config {
	cfg := zap.NewProductionConfig()
	// Sampling is handled by the log collector.
	cfg.Sampling = nil
	// Use consistent field names for log collection across services with different loggers.
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = func(t time.Time, pae zapcore.PrimitiveArrayEncoder) {
		pae.AppendString(t.UTC().Format(time.RFC3339Nano))
	}
	return cfg
}
