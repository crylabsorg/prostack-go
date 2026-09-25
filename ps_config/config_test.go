package ps_config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const testFilePermissions os.FileMode = 0o600

func TestNewLoadsConfigAndLocalOverride(t *testing.T) {
	t.Chdir(t.TempDir())

	writeFile(t, "config.yaml", `
host: example.com
port: 8080
`)
	writeFile(t, "config.local.yaml", `
port: 9090
`)

	var cfg testConfig
	require.NoError(t, New(&cfg))

	require.Equal(t, "example.com", cfg.Host)
	require.Equal(t, 9090, cfg.Port)
}

func TestNewUsesEnvironmentVariables(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("host", "from-environment")
	t.Setenv("port", "7070")

	var cfg testConfig
	require.NoError(t, New(&cfg))

	require.Equal(t, "from-environment", cfg.Host)
	require.Equal(t, 7070, cfg.Port)
}

func TestNewEnvironmentVariablesOverrideConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	writeFile(t, "config.yaml", `
host: from-config
port: 8080
`)
	t.Setenv("host", "from-environment")
	t.Setenv("port", "7070")

	var cfg testConfig
	require.NoError(t, New(&cfg))

	require.Equal(t, "from-environment", cfg.Host)
	require.Equal(t, 7070, cfg.Port)
}

func TestNewAllowsMissingConfigFiles(t *testing.T) {
	t.Chdir(t.TempDir())

	var cfg optionalConfig
	require.NoError(t, New(&cfg))
}

func TestNewDoesNotExportEmptyValue(t *testing.T) {
	for _, tc := range []struct{ name, contents string }{
		{"missing", ""},
		{"empty", "optional: ''\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("PS_CONFIG_OPTIONAL", "")
			require.NoError(t, os.Unsetenv("PS_CONFIG_OPTIONAL"))
			if tc.contents != "" {
				writeFile(t, "config.yaml", tc.contents)
			}

			var cfg struct {
				Optional string `mapstructure:"optional" env:"PS_CONFIG_OPTIONAL"`
			}
			require.NoError(t, New(&cfg))
			require.Empty(t, cfg.Optional)
			_, exists := os.LookupEnv("PS_CONFIG_OPTIONAL")
			require.False(t, exists)
		})
	}
}

func TestNewReturnsErrorForUnknownField(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "config.yaml", "host: example.com\nunknown: value\n")

	var cfg testConfig
	err := New(&cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "unknown")
}

func TestNewReturnsErrorForInvalidYAML(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "config.yaml", "host: [unterminated\n")

	var cfg testConfig
	require.Error(t, New(&cfg))
}

func TestNewReturnsValidationError(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "config.yaml", "host: example.com\nport: 0\n")

	var cfg testConfig
	err := New(&cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "Port")
}

func TestNewWithPathsReturnsErrorForDirectoryConfig(t *testing.T) {
	dir := t.TempDir()
	configPath, err := os.MkdirTemp(dir, "config.yaml")
	require.NoError(t, err)

	var cfg testConfig
	require.Error(t, newWithPaths(&cfg, []string{configPath}))
}

type testConfig struct {
	Host string `mapstructure:"host" env:"host" validate:"required"`
	Port int    `mapstructure:"port" env:"port" validate:"required,min=1"`
}

type optionalConfig struct {
	Name string `mapstructure:"name"`
}

func writeFile(t *testing.T, name, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(name, []byte(contents), testFilePermissions))
}
