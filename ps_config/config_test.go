package ps_config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/crylabsorg/prostack-go/ps_config"
)

const testFilePermissions os.FileMode = 0o600

//nolint:paralleltest // t.Chdir changes the process-wide working directory.
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
	require.NoError(t, ps_config.New(&cfg))

	require.Equal(t, "example.com", cfg.Host)
	require.Equal(t, 9090, cfg.Port)
}

func TestNewUsesEnvironmentVariables(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("host", "from-environment")
	t.Setenv("port", "7070")

	var cfg testConfig
	require.NoError(t, ps_config.New(&cfg))

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
	require.NoError(t, ps_config.New(&cfg))

	require.Equal(t, "from-environment", cfg.Host)
	require.Equal(t, 7070, cfg.Port)
}

//nolint:paralleltest // t.Chdir changes the process-wide working directory.
func TestNewAllowsMissingConfigFiles(t *testing.T) {
	t.Chdir(t.TempDir())

	var cfg optionalConfig
	require.NoError(t, ps_config.New(&cfg))
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
			require.NoError(t, ps_config.New(&cfg))
			require.Empty(t, cfg.Optional)
			_, exists := os.LookupEnv("PS_CONFIG_OPTIONAL")
			require.False(t, exists)
		})
	}
}

func TestNewExportsOTELEntries(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "config.yaml", "OTEL_SERVICE_NAME: test-service\n")
	t.Setenv("OTEL_SERVICE_NAME", "")
	require.NoError(t, os.Unsetenv("OTEL_SERVICE_NAME"))

	var cfg struct {
		ServiceName string `mapstructure:"OTEL_SERVICE_NAME"`
	}
	require.NoError(t, ps_config.New(&cfg))
	require.Equal(t, "test-service", cfg.ServiceName)
	require.Equal(t, "test-service", os.Getenv("OTEL_SERVICE_NAME"))
}

func TestNewExportsOTELEntriesWithoutStructFields(t *testing.T) {
	for _, tc := range []struct {
		name        string
		localConfig string
		environment map[string]string
		serviceName string
	}{
		{name: "config", serviceName: "gateway"},
		{name: "local override", localConfig: "OTEL_SERVICE_NAME: local-gateway\n", serviceName: "local-gateway"},
		{name: "environment override", environment: map[string]string{"OTEL_SERVICE_NAME": "env-gateway"}, serviceName: "env-gateway"},
		{name: "empty environment", environment: map[string]string{"OTEL_SERVICE_NAME": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeFile(t, "config.yaml", `
OTEL_SERVICE_NAME: gateway
OTEL_METRICS_EXPORTER: ps_prometheus
OTEL_TRACES_EXPORTER: otlp
OTEL_EXPORTER_OTLP_TRACES_PROTOCOL: grpc
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT: http://localhost:4317
OTEL_BSP_MAX_QUEUE_SIZE: 2048
OTEL_SDK_DISABLED: false
`)
			if tc.localConfig != "" {
				writeFile(t, "config.local.yaml", tc.localConfig)
			}
			expected := map[string]string{
				"OTEL_SERVICE_NAME":                  tc.serviceName,
				"OTEL_METRICS_EXPORTER":              "ps_prometheus",
				"OTEL_TRACES_EXPORTER":               "otlp",
				"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": "grpc",
				"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://localhost:4317",
				"OTEL_BSP_MAX_QUEUE_SIZE":            "2048",
				"OTEL_SDK_DISABLED":                  "false",
			}
			for name := range expected {
				t.Setenv(name, "")
				require.NoError(t, os.Unsetenv(name))
			}
			for name, value := range tc.environment {
				t.Setenv(name, value)
			}

			var cfg optionalConfig
			require.NoError(t, ps_config.New(&cfg))
			for name, value := range expected {
				require.Equal(t, value, os.Getenv(name), name)
			}
		})
	}
}

//nolint:paralleltest // t.Chdir changes the process-wide working directory.
func TestNewReturnsErrorForUnknownField(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "config.yaml", "host: example.com\nunknown: value\n")

	var cfg testConfig
	err := ps_config.New(&cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "unknown")
}

//nolint:paralleltest // t.Chdir changes the process-wide working directory.
func TestNewReturnsErrorForInvalidYAML(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "config.yaml", "host: [unterminated\n")

	var cfg testConfig
	require.Error(t, ps_config.New(&cfg))
}

//nolint:paralleltest // t.Chdir changes the process-wide working directory.
func TestNewReturnsValidationError(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "config.yaml", "host: example.com\nport: 0\n")

	var cfg testConfig
	err := ps_config.New(&cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "Port")
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
