package config

import (
	"github.com/crylabsorg/prostack/pkg/config"
	"github.com/crylabsorg/prostack/pkg/logger"
)

type Config struct {
	LogPreset logger.Preset `mapstructure:"LOG_PRESET"       env:"LOG_PRESET"       validate:"required,oneof=production development"`
}

func New() (*Config, error) {
	cfg := &Config{}
	err := config.New(cfg)
	return cfg, err
}
