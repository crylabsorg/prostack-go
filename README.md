# prostack-go — Reusable Go backend infrastructure

**Build your business logic on a foundation you do not have to rebuild.**

`prostack-go` is a **Go infrastructure library for backend services**: application bootstrap, HTTP and gRPC, PostgreSQL, MongoDB, Redis, NATS JetStream, Kafka, configuration, structured logging, health checks, and OpenTelemetry integration. Use individual packages in an existing application or assemble a service with the shared lifecycle.

prostack means **production stack**. It grew out of repeating the same infrastructure setup across projects and companies before writing business code. This repository contains the reusable foundation; [prostack-go-showcase](https://github.com/crylabsorg/prostack-go-showcase) demonstrates it in a runnable application.

[Build a small service](#quick-start) · [Explore the packages](#packages) · [Integration guide](#integration-guide) · [Run the complete example](https://github.com/crylabsorg/prostack-go-showcase#quick-start)

## Why prostack-go?

Database and broker clients solve individual problems. A backend also needs consistent startup, configuration, telemetry, error transport, health probes, shutdown, and tests. `prostack-go` connects these recurring concerns while keeping your domain and business rules in your own Go packages.

- **Adopt what you need.** Import a PostgreSQL adapter or the event publisher without adopting the showcase's three-service architecture.
- **Keep familiar clients.** Database adapters expose pgx, MongoDB driver, and go-redis types, so your repositories use their existing APIs.
- **Share lifecycle conventions.** Register handlers, probes, background workers, and cleanup in one setup function.
- **Follow real examples.** The showcase demonstrates an HTTP → gRPC → PostgreSQL transactional outbox → NATS and Kafka → email workflow, with integration tests.

This is for Go developers and teams building backend services or standardizing infrastructure across applications. Your application owns its domain model, repositories, schemas, API contracts, and deployment.

## Quick start

Build a small HTTP service with JSON responses, health endpoints, metrics, and graceful shutdown. This example needs **Go 1.27.1** and curl; it runs without Docker, databases, brokers, or a telemetry collector.

### 1. Create a Go module

```sh
mkdir hello-prostack
cd hello-prostack
go mod init example.com/hello-prostack
export GOEXPERIMENT=jsonv2
go get github.com/crylabsorg/prostack-go@22ac19f4178c91fb35440356a134988d3e7776c5
```

The example pins a full source revision. Keep dependencies pinned to the complete release version or source commit you adopt. `GOEXPERIMENT=jsonv2` enables the `encoding/json/v2` package used by the HTTP response helpers.

### 2. Create `main.go`

```go
package main

import (
	"context"
	"net/http"

	"github.com/crylabsorg/prostack-go/logger"
	"github.com/crylabsorg/prostack-go/ps_bootstrap"
	"github.com/crylabsorg/prostack-go/ps_controller"
)

type config struct{}

func (config) GetLogPreset() logger.Preset { return logger.PresetDevelopment }
func (config) GetServiceName() string      { return "hello" }
func (config) GetHTTPListenAddr() string   { return "127.0.0.1:33003" }
func (config) GetGRPCListenAddr() string   { return "" }

func main() {
	ps_bootstrap.Run(
		func() (config, error) { return config{}, nil },
		func(_ context.Context, _ context.Context, _ config, app *ps_bootstrap.App) error {
			controller := ps_controller.NewController()
			app.GetHTTPServeMux().HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
				controller.WriteJSON(r.Context(), w, http.StatusOK, map[string]string{
					"message": "hello from prostack",
				})
			})
			return nil
		},
	)
}
```

The four getters implement the bootstrap configuration interface. An empty gRPC address keeps this example HTTP-only. Configuration is inline here; applications can load a typed configuration with `ps_config` as described below.

### 3. Run and inspect

Resolve the dependencies for the imports above, then start the service:

```sh
GOEXPERIMENT=jsonv2 go mod tidy

GOEXPERIMENT=jsonv2 \
OTEL_SERVICE_NAME=hello \
OTEL_TRACES_EXPORTER=none \
OTEL_METRICS_EXPORTER=ps_prometheus \
go run .
```

The explicit exporter settings keep the example self-contained: traces are disabled and OpenTelemetry metrics are exposed over HTTP. In a second terminal:

```sh
curl --fail-with-body -i http://127.0.0.1:33003/hello
curl --fail -i http://127.0.0.1:33003/readyz
curl --fail http://127.0.0.1:33003/metrics
curl --fail http://127.0.0.1:33003/otel-metrics
```

Expect **HTTP 200** and `{"message":"hello from prostack"}` from `/hello`. `/metrics` exposes the application, Go, and process registry; `/otel-metrics` exposes OpenTelemetry metrics when `ps_prometheus` is selected.

Press Ctrl+C and wait for shutdown. Bootstrap marks the service unready, waits **10 seconds** for load balancers to react, then drains handlers and registered workers with a shared **30-second** grace period before dependency cleanup.

For databases, brokers, migrations, tracing in Jaeger, and a complete business workflow, continue with the [showcase quick start](https://github.com/crylabsorg/prostack-go-showcase#quick-start).

## Packages

Each link opens the implementation in this repository. Import the packages relevant to your application; connections are created when you call their setup functions.

| Package | What you get |
| --- | --- |
| [`ps_bootstrap`](ps_bootstrap/bootstrap.go) | Application setup, HTTP and optional gRPC listeners, probes, worker draining, and cleanup. |
| [`ps_config`](ps_config/config.go) | Typed configuration from YAML, local overrides, declared environment variables, and validation. |
| [`logger`](logger/logger.go) | Zap development/production presets, context-aware logging, and an OpenTelemetry log bridge. |
| [`ps_http`](ps_http/http.go), [`ps_controller`](ps_controller/controller.go) | Instrumented HTTP serving, JSON responses, and conversion of gRPC error details into HTTP errors. |
| [`ps_grpc`](ps_grpc/grpc.go), [`ps_error`](ps_error/error.go) | Instrumented gRPC servers/clients, unary recovery/error interceptors, and typed application errors. |
| [`ps_pg`](ps_pg/pg.go) | Instrumented pgx connection pools and connection health verification. |
| [`ps_mongo`](ps_mongo/mongo.go) | Instrumented MongoDB connections and database selection from the URI. |
| [`ps_redis`](ps_redis/redis.go) | go-redis clients with tracing, metrics, and an initial connection check. |
| [`ps_nats`](ps_nats/nats.go) | JetStream stream setup, concurrent consumers, publisher support, and trace propagation. |
| [`ps_kafka`](ps_kafka/kafka.go) | Kafka publishing, concurrent consumers, ordered partition commits, and trace propagation. |
| [`ps_event`](ps_event/outbox.go) | Outbox/publisher interfaces, a background publishing worker, and fanout to multiple publishers. |
| [`ps_otel`](ps_otel/traces.go), [`ps_metrics`](ps_metrics/metrics.go) | OpenTelemetry initialization and an application Prometheus registry. |
| [`ps_healthcheck`](ps_healthcheck/healthcheck.go) | `/healthz`, `/readyz`, and `/startupz` handlers. |
| [`ps_swagger`](ps_swagger/swagger.go), [`ps_asyncapi`](ps_asyncapi/asyncapi.go) | Embedded documentation UIs for your supplied OpenAPI and AsyncAPI documents. |
| [`ps_test`](ps_test/postgres.go) | Testcontainers helpers for PostgreSQL, MongoDB, NATS, Kafka, plus gRPC test utilities. |

## Integration guide

### Add a package to an existing service

You can keep your existing application lifecycle. For example, `ps_pg.Connect` returns an ordinary `*pgxpool.Pool`, a cleanup function, and an error:

```go
databasePool, disconnectDatabase, err := ps_pg.Connect(ctx, databaseURL)
if err != nil {
	return err
}
defer disconnectDatabase()
```

Your repository can use `databasePool` directly. If you use bootstrap, register cleanup with `app.RegisterCleanup` instead of deferring it inside `setup`, and register a PostgreSQL readiness probe with `databasePool.Ping`. See the [showcase user bootstrap](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/user/main.go) for complete wiring.

Choose constructors deliberately: `ps_pg.Connect` returns connection errors; `ps_pg.New`, `ps_mongo.New`, and `ps_redis.New` log fatal startup failures. Database adapters do not create your domain schema or apply migrations. MongoDB transactions require a replica set.

### Configuration and telemetry

`ps_config.New` fills your configuration struct using `mapstructure`, `env`, and `validate` tags. It reads `config.yaml` and optional `config.local.yaml` from the **process working directory**, then applies declared environment overrides. Unknown non-OpenTelemetry file keys are rejected. See a complete [configuration type](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/user/config/config.go) and its [YAML file](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/user/config.yaml).

Bootstrap initializes logging, traces, and metrics. Set `OTEL_SERVICE_NAME` for the telemetry resource name; the bootstrap service-name getter does not replace it. For collector-based tracing, configure the OpenTelemetry exporter and endpoint explicitly, as the showcase does. OTLP log exporting requires a separate call to `ps_otel.InitLogs` and its shutdown function; the logger bridge alone does not initialize that exporter.

OpenTelemetry setup installs process-wide providers and propagation and instruments `http.DefaultTransport`. Account for this when integrating with an application that already owns those globals.

### Lifecycle and health

Bootstrap runs `setup` before opening listeners. Its two contexts have different jobs: the work context remains alive while active work drains; the draining context is canceled as soon as shutdown begins. Workers should stop taking new work on draining and close the channel registered with `app.RegisterWorker` when their active work finishes. Cleanup callbacks run in reverse registration order after servers and workers stop or their grace period expires.

`/healthz` indicates that the HTTP process is serving. `/readyz` runs only the probes you register and returns unavailable during draining; it does not discover dependencies or verify an entire event pipeline. `/startupz` is ready by default unless a startup condition is supplied through `ps_healthcheck` directly.

### Errors across gRPC and HTTP

Define service errors as `ps_error.PSError` with an application error code, HTTP status, and gRPC status. Return them from gRPC handlers; `ps_grpc.ErrorInterceptor` adds the transport details. An HTTP gateway then passes the received gRPC error to `ps_controller.Controller.WriteError`, which reads those details instead of maintaining another status map. See [example errors](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/user/user/errors.go) and the [HTTP controller](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/gateway/user/http.go).

### Events and the transactional outbox

`ps_event` supplies publishing interfaces and a worker. Your application implements persistence: write the business change and outbox row in one transaction, reserve pending rows, and mark only acknowledged events complete. The library does not supply an outbox database schema or consumer deduplication. The showcase's [repository](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/user/user/repository.go) and [adapter](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/user/user/outbox.go) demonstrate that boundary.

Publishing is **at least once**. Fanout completes only when every publisher succeeds; partial failures retry all publishers. Concurrent processing does not guarantee handler execution order. Your consumers own idempotency and side-effect recovery.

With NATS, your handler owns acknowledgement, retry, or termination of each message. Kafka commits after a successful handler and returns on processing failure; your application owns its restart/backoff loop. Dead-letter handling is application-specific. See the showcase's [NATS](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/email/email/nats.go) and [Kafka](https://github.com/crylabsorg/prostack-go-showcase/blob/main/apps/email/email/kafka.go) controllers.

### Deployment context

Review defaults against your environment. The supplied HTTP server serves plaintext; gRPC defaults to plaintext and enables reflection. Add your application's authentication, transport security, ingress limits, secrets, and deployment configuration. The library provides infrastructure building blocks, not a deployment platform or an availability guarantee.

## Development

From a checkout of **this repository**:

```sh
export GOEXPERIMENT=jsonv2
make mod-download
make help
make test
```

The test suite uses real containerized dependencies and requires a reachable Docker daemon, tested with Docker **29.8.2**. Library Testcontainers tests do not require the showcase Compose stack.

| Command | Purpose |
| --- | --- |
| `make test` | Run all foundation packages, including integration tests. |
| `make lint` | Install the pinned linter if needed and lint the library. |
| `make proto-gen` | Regenerate the shared error protobuf; requires Buf **1.74.0**. Go generator versions are pinned by Make. |
| `make mod-tidy` | Reconcile module dependencies after code changes. |

Reusable [Make fragments](make/) can also be included by a consuming repository. The showcase [Makefile](https://github.com/crylabsorg/prostack-go-showcase/blob/main/Makefile) demonstrates app discovery, build/run/test targets, migrations, and local Compose commands. Those application targets belong to the consuming repository; the foundation's Makefile focuses on library development.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| JSON packages are excluded by build constraints | Use Go 1.27.1 and enable `GOEXPERIMENT=jsonv2` in the terminal running Go commands. |
| Exporters try to reach a local collector | Use the explicit quick-start exporter settings or configure your own OTLP endpoint. |
| No OpenTelemetry metrics at `/metrics` | Scrape `/otel-metrics` with `OTEL_METRICS_EXPORTER=ps_prometheus`; `/metrics` is a separate registry. |
| Configuration files are not found | Check the process working directory and your declared configuration tags. |
| Ctrl+C does not exit immediately | Allow the fixed 10-second load-balancer delay and handler/worker draining to finish. |
| `/readyz` succeeds while a dependency is unavailable | Register a probe for that dependency; readiness only runs registered checks. |
| Private or unpublished source cannot be downloaded | Ensure the pinned commit is reachable and configure Git access for the repository, or use a local `replace` while developing. |

## Contributing

[Open an issue](https://github.com/crylabsorg/prostack-go/issues) with reproduction steps, tool versions, and relevant logs with secrets removed. Focused fixes, tests, and documentation improvements are welcome; follow [project conventions](AGENTS.md) and run the relevant checks.

Infrastructure changes belong here. Application workflows and runnable examples belong in [prostack-go-showcase](https://github.com/crylabsorg/prostack-go-showcase).

## License

[MIT](LICENSE).
