package ps_config //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWithPathsReturnsErrorForDirectoryConfig(t *testing.T) {
	t.Parallel()
	configPath := t.TempDir()

	var cfg struct {
		Host string `mapstructure:"host" env:"host" validate:"required"`
		Port int    `mapstructure:"port" env:"port" validate:"required,min=1"`
	}
	require.Error(t, newWithPaths(&cfg, []string{configPath}))
}
