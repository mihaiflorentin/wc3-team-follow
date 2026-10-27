# Getting started

This guide covers local development, configuration and builds for **wc3-team-follow**. Clone or fork https://github.com/mihaiflorentin/wc3-team-follow before making changes.

## Prerequisites

- Go 1.27
- Make
- Docker (only for `make docker-build` and `make docker-image`)

## Build and run

```bash
go mod tidy
make build
./bin/wc3-team-follow server
```

`make build` writes `bin/wc3-team-follow`. The commands:

```bash
./bin/wc3-team-follow --help
./bin/wc3-team-follow --version
./bin/wc3-team-follow server --help
./bin/wc3-team-follow worker
./bin/wc3-team-follow example-job --name nightly
```

`server` flags override `[http]` only when set explicitly: `--host`, `--port`, `--read-timeout`, `--write-timeout`, `--idle-timeout`. It also accepts `--pool` (sets `GOMAXPROCS`; the Go runtime default applies otherwise), `--cert-file`/`--key-file` (TLS), `--profile` (pprof on `localhost:6060`) and `--shutdown-max-requests` (graceful shutdown after N requests, with `--shutdown-max-requests-jitter` adding a random 0..J). `SIGINT` and `SIGTERM` shut the server down gracefully and close every adapter.

## Worker and one-shot job

`worker` is a long-lived process for background work. It calls `ExampleService.Ping` every `--interval` (default `30s`) as a placeholder loop; replace `runPingLoop` in `cmd/cli/worker.go` with your own work. `SIGINT` or `SIGTERM` stops it cleanly with exit code 0.

`example-job` runs `ExampleService.RunJob` once with `dto.ExampleJob{Name: --name}` (default `example`) and exits: 0 on success, non-zero on failure (an empty `--name` fails). Schedulers such as Kubernetes CronJobs run it.

Both resolve their dependencies from the container before doing any work, like `server`.

With the default config the server listens on port 8080:

| Route | Response |
| --- | --- |
| `GET /health` | `{"status":"ok"}`: liveness, checks no dependency |
| `GET /health/ready` | `{"status":"ok"}`, or 503 `{"status":"unavailable","failed":["mysql"]}` naming the dependencies whose readiness check failed or timed out |
| `GET /example` | `{"message":"pong","examples":0}`: the example service's `Ping` status mapped to `response.Example` |
| `GET /docs/` | Swagger UI for `cmd/http/resource/swagger/swagger.json` |

## Configuration

`config/config.toml` holds the defaults and is embedded into the binary, so edits take effect after `make build`. Its sections:

- `[app]` — `name`, `env`.
- `[logging]` — `default` (process log format), `server_default` (access-log format), `level`. See `docs/logging-and-middleware.md`.
- `[http]` — `host`, `port`, `read_timeout`, `write_timeout`, `idle_timeout`, `readiness_timeout` (bounds `GET /health/ready`).

Override any key at runtime with an environment variable: the key path upper-cased, with dots replaced by underscores.

```bash
HTTP_PORT=9090 ./bin/wc3-team-follow server
LOGGING_DEFAULT=json LOGGING_LEVEL=debug ./bin/wc3-team-follow server
./bin/wc3-team-follow server --port 9090
```

Provide secrets through environment variables rather than committing them to `config.toml`.

## Project layout

```text
├── AGENTS.md        rules for every change (start here)
├── main.go          composition root: config → logger → container → CLI
├── cmd/
│   ├── cli/         Cobra commands; resolve dependencies from the container
│   └── http/        NewHandler, Pipeline, StartServer, handlers, middleware, swagger
├── config/          config.toml (embedded) and NewConfig
├── container/       lazy (T, error) accessors and Close
├── docs/            documentation; docs/adr/ holds decisions
├── domain/          business logic (domain/example is the placeholder)
├── infrastructure/  adapters implementing port/contract
├── logging/         logging.New and the slog handlers
├── mock/            test doubles for every contract
└── port/
    ├── contract/    interfaces only
    └── dto/         DTOs passed between layers (package dto)
        ├── request/     inbound HTTP DTOs
        └── response/    outbound HTTP DTOs
```

## Testing

```bash
go vet ./...
go test ./...
```

Domain services are tested with the mocks in `mock/`; the HTTP layer is tested with `httptest` against `httpserver.NewHandler` (see `cmd/http/handler_test.go`). No test needs a running backend.

## Docker

```bash
make docker-build          # Linux binary built inside golang:1.27, written to dist/
make docker-image          # runtime image from Dockerfile, tagged wc3-team-follow:latest
docker run --rm -p 8080:8080 wc3-team-follow:latest
```

`make docker-image IMAGE_REPOSITORY=... IMAGE_TAG=v1.2.3` overrides the repository and tag. The image sets `TZ=Europe/Bucharest`; the binary embeds Go's timezone database (`time/tzdata`, imported by `main.go`), so the timezone resolves although the distroless image has no zoneinfo files. `timezone_test.go` checks it loads; to check a binary without zoneinfo, build the test for Linux (`GOOS=linux go test -c -o tz.test .`) and run it with `TZ` set in an empty (`scratch`) image.

## Next steps

1. Replace `domain/example` with your own bounded contexts.
2. Declare the ports they need in `port/contract`, implement them as adapters, and wire them in `container/` (`docs/container.md`).
3. Record architectural decisions in `docs/adr/` and keep `docs/` and `AGENTS.md` in step with the code.
