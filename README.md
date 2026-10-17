# prostack-go — Reusable Go backend infrastructure

**Build your business logic on a foundation you do not have to rebuild.**

`prostack-go` is a **Go infrastructure library for backend services**: application bootstrap, HTTP and gRPC, PostgreSQL, MongoDB, Redis, NATS JetStream, Kafka, configuration, structured logging, health checks, and OpenTelemetry integration. Use individual packages in an existing application or assemble a service with the shared lifecycle.

prostack means **production stack**. It grew out of repeating the same infrastructure setup across projects and companies before writing business code. This repository contains the reusable foundation; [prostack-go-showcase](https://github.com/crylabsorg/prostack-go-showcase) demonstrates it in a runnable application.

I also saw organizations with many services and teams, each assembling its own mix of technologies and infrastructure conventions. I wanted a shared baseline for recurring concerns—lifecycle, errors, telemetry, and tests—to standardize engineering practices across teams and make it easier for developers to move between projects. Teams can choose the components their applications need.

[Quick start](#quick-start) · [Explore the packages](#packages) · [Integration guide](#integration-guide)

## Why prostack-go?

Database and broker clients solve individual problems. A backend also needs consistent startup, configuration, telemetry, error transport, health probes, shutdown, and tests. `prostack-go` connects these recurring concerns while keeping your domain and business rules in your own Go packages.

- **Adopt what you need.** Import a PostgreSQL adapter or the event publisher without adopting the showcase's three-service architecture.
- **Keep familiar clients.** Database adapters expose pgx, MongoDB driver, and go-redis types, so your repositories use their existing APIs.
- **Share lifecycle conventions.** Register handlers, probes, background workers, and cleanup in one setup function.
- **Follow real examples.** The showcase demonstrates an HTTP → gRPC → PostgreSQL transactional outbox → NATS and Kafka → email workflow, with integration tests.

This is for Go developers and teams building backend services or standardizing infrastructure across applications. Your application owns its domain model, repositories, schemas, API contracts, and deployment.

## Quick start

Start with [prostack-go-showcase](https://github.com/crylabsorg/prostack-go-showcase#quick-start), a runnable application with configuration, migrations, API contracts, and local infrastructure already connected.

Create a user and follow the complete workflow: **HTTP → gRPC → PostgreSQL + transactional outbox → NATS JetStream and Kafka → email consumer**. The walkthrough shows how to:

- Create a user with an HTTP request, receive **201 Created**, and fetch the stored user.
- Follow `user.created` from the database transaction through both brokers to a logged fake welcome email and a delivery record in MongoDB.
- Inspect request traces and asynchronous span links in Jaeger, plus health and metrics endpoints.

`prostack-go` supplies the shared lifecycle, instrumented clients and transports, error handling, and event publishing worker. The showcase demonstrates how to connect those pieces to application business logic and persistence.

**[Run the showcase](https://github.com/crylabsorg/prostack-go-showcase#quick-start)** for the required tools, dependency setup, migrations, application startup, and verification steps.

## Packages

Each link opens the implementation in this repository. Import the packages relevant to your application; connections are created when you call their setup functions.

| Package                                                                         | What you get                                                                                        |
| ------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| [`ps_bootstrap`](ps_bootstrap/bootstrap.go)                                     | Application setup, HTTP and optional gRPC listeners, probes, worker draining, and cleanup.          |
| [`ps_config`](ps_config/config.go)                                              | Typed configuration from YAML, local overrides, declared environment variables, and validation.     |
| [`logger`](logger/logger.go)                                                    | Zap development/production presets, context-aware logging, and an OpenTelemetry log bridge.         |
| [`ps_http`](ps_http/http.go), [`ps_controller`](ps_controller/controller.go)    | Instrumented HTTP serving, JSON responses, and conversion of gRPC error details into HTTP errors.   |
| [`ps_grpc`](ps_grpc/grpc.go), [`ps_error`](ps_error/error.go)                   | Instrumented gRPC servers/clients, unary recovery/error interceptors, and typed application errors. |
| [`ps_pg`](ps_pg/pg.go)                                                          | Instrumented pgx connection pools and connection health verification.                               |
| [`ps_mongo`](ps_mongo/mongo.go)                                                 | Instrumented MongoDB connections and database selection from the URI.                               |
| [`ps_redis`](ps_redis/redis.go)                                                 | go-redis clients with tracing, metrics, and an initial connection check.                            |
| [`ps_nats`](ps_nats/nats.go)                                                    | JetStream stream setup, concurrent consumers, publisher support, and trace propagation.             |
| [`ps_kafka`](ps_kafka/kafka.go)                                                 | Kafka publishing, concurrent consumers, ordered partition commits, and trace propagation.           |
| [`ps_event`](ps_event/outbox.go)                                                | Outbox/publisher interfaces, a background publishing worker, and fanout to multiple publishers.     |
| [`ps_otel`](ps_otel/traces.go), [`ps_metrics`](ps_metrics/metrics.go)           | OpenTelemetry initialization and an application Prometheus registry.                                |
| [`ps_healthcheck`](ps_healthcheck/healthcheck.go)                               | `/healthz`, `/readyz`, and `/startupz` handlers.                                                    |
| [`ps_swagger`](ps_swagger/swagger.go), [`ps_asyncapi`](ps_asyncapi/asyncapi.go) | Embedded documentation UIs for your supplied OpenAPI and AsyncAPI documents.                        |
| [`ps_test`](ps_test/postgres.go)                                                | Testcontainers helpers for PostgreSQL, MongoDB, NATS, Kafka, plus gRPC test utilities.              |

## Development

From a checkout of **this repository**:

```sh
make mod-download
make help
make test
```

The test suite uses real containerized dependencies and requires a reachable Docker daemon, tested with Docker **29.8.2**. Library Testcontainers tests do not require the showcase Compose stack.

| Command          | Purpose                                                                                                  |
| ---------------- | -------------------------------------------------------------------------------------------------------- |
| `make test`      | Run all foundation packages, including integration tests.                                                |
| `make lint`      | Install the pinned linter if needed and lint the library.                                                |
| `make proto-gen` | Regenerate the shared error protobuf; requires Buf **1.74.0**. Go generator versions are pinned by Make. |
| `make mod-tidy`  | Reconcile module dependencies after code changes.                                                        |

Reusable [Make fragments](make/) can also be included by a consuming repository. The showcase [Makefile](https://github.com/crylabsorg/prostack-go-showcase/blob/main/Makefile) demonstrates app discovery, build/run/test targets, migrations, and local Compose commands. Those application targets belong to the consuming repository; the foundation's Makefile focuses on library development.

## Troubleshooting

| Symptom                                              | What to check                                                                                                                   |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Exporters try to reach a local collector             | Use the exporter settings from the [minimal bootstrap example](#minimal-bootstrap-example) or configure your own OTLP endpoint. |
| No OpenTelemetry metrics at `/metrics`               | Scrape `/otel-metrics` with `OTEL_METRICS_EXPORTER=ps_prometheus`; `/metrics` is a separate registry.                           |
| Configuration files are not found                    | Check the process working directory and your declared configuration tags.                                                       |
| Ctrl+C does not exit immediately                     | Allow the fixed 10-second load-balancer delay and handler/worker draining to finish.                                            |
| `/readyz` succeeds while a dependency is unavailable | Register a probe for that dependency; readiness only runs registered checks.                                                    |
| Private or unpublished source cannot be downloaded   | Ensure the pinned commit is reachable and configure Git access for the repository, or use a local `replace` while developing.   |

## Contributing

[Open an issue](https://github.com/crylabsorg/prostack-go/issues) with reproduction steps, tool versions, and relevant logs with secrets removed. Focused fixes, tests, and documentation improvements are welcome; follow [project conventions](AGENTS.md) and run the relevant checks.

Infrastructure changes belong here. Application workflows and runnable examples belong in [prostack-go-showcase](https://github.com/crylabsorg/prostack-go-showcase).

## License

[MIT](LICENSE).
