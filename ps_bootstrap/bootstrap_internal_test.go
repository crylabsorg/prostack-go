package ps_bootstrap //nolint:staticcheck // Preserve the public package name for import compatibility.

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/crylabsorg/prostack-go/logger"
)

// Each subprocess owns its signals and global telemetry providers. Exercise Run
// itself so moving its shutdown sequence cannot silently invalidate the tests.
func TestRunLifecycle(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"idle", "active", "timeout", "stuck", "http_failure", "grpc_failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			executable, err := os.Executable()
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestRunProcess$")
			command.Env = append(os.Environ(),
				"PROSTACK_BOOTSTRAP_TEST_SCENARIO="+scenario,
				"OTEL_TRACES_EXPORTER=none",
				"OTEL_METRICS_EXPORTER=none",
			)
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
		})
	}
}

func TestRunProcess(t *testing.T) {
	t.Parallel()
	scenario := os.Getenv("PROSTACK_BOOTSTRAP_TEST_SCENARIO")
	if scenario == "" {
		t.Skip("only runs in the lifecycle test subprocess")
	}

	var drainStarted time.Time
	var cleanupOrder []string
	workerResult := make(chan error, 1)
	config := lifecycleConfig{}
	if scenario == "http_failure" {
		config.httpAddress = "invalid-address"
	}
	if scenario == "grpc_failure" {
		config.grpcAddress = "invalid-address"
	}
	Run(func() (lifecycleConfig, error) { return config, nil },
		func(ctx, drainingCtx context.Context, _ lifecycleConfig, app *App) error {
			workerDone := make(chan struct{})
			app.RegisterWorker(workerDone)
			app.RegisterCleanup(func() error {
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				require.ErrorIs(t, drainingCtx.Err(), context.Canceled)
				cleanupOrder = append(cleanupOrder, "database")
				return nil
			})
			app.RegisterCleanup(func() error {
				cleanupOrder = append(cleanupOrder, "broker")
				return nil
			})
			go func() {
				if scenario == "stuck" {
					return // Never close workerDone: cleanup must still run after the grace period.
				}
				defer close(workerDone)
				workerResult <- runLifecycleWorker(ctx, drainingCtx, scenario)
			}()

			drainStarted = time.Now()
			if scenario == "http_failure" || scenario == "grpc_failure" {
				return nil // A listener error must initiate draining without an OS signal.
			}
			process, err := os.FindProcess(os.Getpid())
			if err != nil {
				return err
			}
			return process.Signal(syscall.SIGTERM)
		},
	)

	elapsed := time.Since(drainStarted)
	require.Equal(t, []string{"broker", "database"}, cleanupOrder)
	const loadBalancerDelay = 10 * time.Second
	require.GreaterOrEqual(t, elapsed, loadBalancerDelay)
	switch scenario {
	case "idle", "active", "http_failure", "grpc_failure":
		require.NoError(t, <-workerResult, "work must finish before cancellation")
		const earlyCompletionLimit = 20 * time.Second
		require.Less(t, elapsed, earlyCompletionLimit, "Run waited out the grace period")
	case "timeout", "stuck":
		if scenario == "timeout" {
			require.ErrorIs(t, <-workerResult, context.Canceled)
		}
		const totalGrace = 40 * time.Second
		const schedulingMargin = 5 * time.Second
		require.GreaterOrEqual(t, elapsed, totalGrace)
		require.Less(t, elapsed, totalGrace+schedulingMargin)
	}
}

func runLifecycleWorker(ctx, drainingCtx context.Context, scenario string) error {
	switch scenario {
	case "idle":
	case "http_failure", "grpc_failure":
		<-drainingCtx.Done()
	case "active":
		<-drainingCtx.Done()
		// Remain active beyond the mandatory load-balancer delay.
		const handlerDuration = 12 * time.Second
		timer := time.NewTimer(handlerDuration)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	case "timeout":
		<-ctx.Done()
	}
	return ctx.Err()
}

type lifecycleConfig struct {
	httpAddress string
	grpcAddress string
}

func (lifecycleConfig) GetLogPreset() logger.Preset { return logger.PresetProduction }
func (lifecycleConfig) GetServiceName() string      { return "bootstrap-test" }

func (c lifecycleConfig) GetHTTPListenAddr() string {
	if c.httpAddress != "" {
		return c.httpAddress
	}
	return "127.0.0.1:0"
}

func (c lifecycleConfig) GetGRPCListenAddr() string {
	if c.grpcAddress != "" {
		return c.grpcAddress
	}
	return "127.0.0.1:0"
}
