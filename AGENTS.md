# AGENTS.md

wc3-team-follow is a Go 1.27 service built as a hexagonal (ports and adapters) application with dependency inversion: business logic depends on interfaces in `port/contract`, adapters implement them, and one composition root wires everything together. Every change keeps the rules below. The docs in `docs/` explain each area in depth; read the matching doc before changing that area, and update it in the same change.

## Layout

| Path | Holds |
| --- | --- |
| `main.go` | Composition root: builds config, logger and container once, runs the CLI, closes the container. |
| `cmd/cli/` | Cobra commands. Wiring layer: resolves dependencies from the container and passes them on. |
| `cmd/http/` | Driving HTTP adapter: `NewHandler(Deps, HandlerOptions)`, `Pipeline`, `StartServer`. Handlers live in `cmd/http/app/<group>/handler/`, middleware in `cmd/http/middleware/`. |
| `container/` | `ServiceContainer`: lazy `(T, error)` accessors and `Close`. Used only by the wiring layer. |
| `config/` | `config.toml` (embedded into the binary) and `NewConfig`. |
| `logging/` | `New(cfg, w) *slog.Logger` and the log handlers. Cross-cutting: every layer may receive a `*slog.Logger`. |
| `domain/` | Business logic, one package per bounded context (`domain/example` is the placeholder). |
| `port/contract/` | Interfaces (ports) that domain and adapters meet at. Interfaces only, no structs. |
| `port/dto/` | Plain data shapes. In-app DTOs (`package dto`, e.g. `port/dto/example.go`); HTTP DTOs in `port/dto/request/` and `port/dto/response/`. |
| `infrastructure/` | Driven adapters, one package per backend, each implementing a `port/contract` interface. |
| `mock/` | Hand-written test doubles, one package per contract. |
| `docs/` | Living documentation; architectural decisions go in `docs/adr/`. |

## Project settings

Chosen when the project was generated; change them in the files named here.

- `[http] port` is `8080` (`HTTP_PORT` or `server --port` override it); the `Dockerfile` exposes the same port.
- `[logging] default` and `server_default` are `color`, `level` is `debug` (`config/config.toml`).
- The `Dockerfile` sets `TZ=Europe/Bucharest`. `main.go` imports `time/tzdata`, so the binary carries the timezone database and `TZ` resolves in the distroless image; `timezone_test.go` checks the timezone loads. Keep that import.
- `make docker-image` tags `wc3-team-follow:$(IMAGE_TAG)` (`IMAGE_REPOSITORY` and `IMAGE_TAG`, default `latest`, in the `Makefile`).
- `LICENSE`: MIT, copyright 2026 Mihai Mihaila.

## Dependency rules

- Dependencies point inward. `domain/` imports only `port/` and the standard library. `infrastructure/` imports `port/` and `config/`, never `domain/` or `cmd/`. `cmd/http/` imports `port/`, `config/` and `logging/`, never the container or `infrastructure/`.
- Only `main.go`, `cmd/cli/` and `container/` import `container`. There is no package-level container variable; nothing reaches into the container to look up a dependency.
- Consumers depend on `port/contract` interfaces, never on concrete types: handlers take `contract.ExampleService`, not `*example.Service`. Adapter constructors return the contract (`func New(...) (contract.X, error)`).

## Composition root and dependency injection

- `main.go` calls `config.NewConfig()`, `logging.New(cfg.Logging, os.Stdout)`, `container.New(cfg, logger)` and `cli.Execute(c)`, then `c.Close()`. Nothing else builds a container.
- Each command receives the container through its constructor (`newServerCmd(c)`) and resolves what it needs before doing any work. The `server` command resolves everything in `httpDeps` and passes `httpserver.Deps` (dependencies) and `httpserver.HandlerOptions` (settings) to `httpserver.NewHandler`, so an unreachable backend fails before the server listens. `worker` and `example-job` follow the same pattern: `resolveWorkerDeps` and `resolveJobDeps` resolve their dependencies, and `workerCmd`/`exampleJobCmd` take a resolve function so tests run them with mocks.
- Every collaborator arrives as a constructor parameter and is stored in a field: `example.NewService(logger)`, `handler.New(exampleSvc, logger)`. No globals, no `init()`, no config lookups inside domain code or handlers.
- Container accessors are lazy singletons returning `(T, error)`. A missing config section or unreachable backend is an error, and a failed build is retried on the next call. Adapters that hold connections register a hook with `s.onClose`; `Close` runs the hooks in reverse order. Next to it they register their `Ping` as a readiness check with `s.onReady(name, client.Ping)`; `GET /health/ready` runs every registered check (`GET /health` is liveness only).

To add a dependency (an outbound adapter or a domain service):

1. Declare the interface in `port/contract/<name>.go`.
2. Implement it in its own package with a `New` constructor that returns the contract.
3. Add a mock in `mock/<name>/` that starts with `var _ contract.<Name> = (*Client)(nil)`.
4. If it needs settings, add a `[<name>]` section to `config/config.toml` and a pointer field to `config.Config`.
5. Add a lazy accessor to `container/infrastructure.go` (adapters) or `container/domain.go` (domain services) that returns `(contract.<Name>, error)` and reports a missing config section as an error. If the adapter holds a connection, register `s.onClose(name, client.Close)` and `s.onReady(name, client.Ping)` there.
6. Add the dependency as a constructor parameter of its consumer, and resolve it in `container/` or `cmd/cli/`.
7. Document it in `docs/` and in this file.

`docs/container.md` shows the accessor pattern in full.

## Logging

- The process logger is the `*slog.Logger` built by `logging.New` in `main.go` (also installed with `slog.SetDefault`); the `server` command adds the access logger from `logging.NewAccessLogger`. Components receive a logger through their constructor and keep it in a field.
- Log a dotted event name as the message and the details as typed attributes: `logger.ErrorContext(ctx, "example.ping", slog.Any("error", err))`. Use the `...Context` variants when a context is available.
- Return errors upward; log them once, at the edge that handles them (handler, command or middleware).
- `[logging] default` picks the process format (`simple`, `color`, `json`, `pretty-json`), `level` the minimum level, and `server_default` the access-log format. See `docs/logging-and-middleware.md`.

## DTOs

- Interfaces go in `port/contract`; every DTO goes in `port/dto`. Never declare a struct in `port/contract`.
- Data passed between the project's own layers (domain results such as `dto.ExampleStatus`, repository records, job payloads) lives directly in `port/dto` as `package dto` (see `port/dto/example.go`). Do not create a `port/dto` subfolder for it.
- HTTP DTOs live in `port/dto/request` (inbound bodies) and `port/dto/response` (outbound payloads).
- Handlers decode into a request DTO, call the domain through a contract, and map the result to a response DTO. Domain types never cross the transport boundary. DTOs are plain structs without behaviour. See `docs/data-contracts.md`.

## Coding style

- `gofmt` everything (`make fmt`) and follow the Go style guide. See `docs/coding-style.md`.
- Constructors are named `New`/`NewX` and take every dependency as a parameter.
- Wrap errors with context: `fmt.Errorf("list examples: %w", err)`. I/O methods take `ctx context.Context` first.
- Register routes with method and path patterns: `mux.HandleFunc("GET "+RouteHealth, ...)`.

## Testing

- Tests sit next to the code they cover; run `go test ./...`. Tests use the mocks in `mock/` and need no running backend.
- Domain services: construct them with mocks and assert on results (`domain/example/service_test.go`).
- HTTP: build the handler with `httpserver.NewHandler(httpserver.Deps{...}, httpserver.HandlerOptions{...})`, inject mocks, and drive it with `httptest` (`cmd/http/handler_test.go`). Test middleware through `httpserver.Pipeline` (`cmd/http/pipeline_test.go`).
- Commands: build them with `workerCmd(...)`/`exampleJobCmd(...)` and a resolve function returning mocks, then `Execute` (`cmd/cli/example_job_test.go`, `cmd/cli/worker_signal_test.go`).
- Container: the lazy build and the `Close` order are tested in `container/container_test.go`.
- Every new contract gets a mock with a compile-time assertion; every new route gets an `httptest` case.

## Commands

| Command | Purpose |
| --- | --- |
| `make build` | Build `bin/wc3-team-follow` with version ldflags. |
| `make test` / `go test ./...` | Run every unit test. |
| `go vet ./...` | Static checks. |
| `make fmt` | `gofmt` every Go file. |
| `make lint` | `golangci-lint run ./...`. |
| `make tidy` | `go mod tidy` after changing imports. |
| `make build-all` | Cross-compile into `dist/`. |
| `make docker-image` | Build the runtime image `wc3-team-follow:$(IMAGE_TAG)` from `Dockerfile`. |
| `./bin/wc3-team-follow server` | Serve HTTP using `[http]` from `config/config.toml`. |
| `./bin/wc3-team-follow worker [--interval 30s]` | Run the example loop until `SIGTERM`. |
| `./bin/wc3-team-follow example-job [--name example]` | Run one example job and exit (non-zero on failure). |

Before finishing a change, run `make fmt`, `go vet ./...` and `go test ./...`.

## CI
- GitHub Actions: `.github/workflows/release.yml` runs on pushed Release Tags (`X.Y.Z`, optional `v`, classified by `scripts/release-tag.sh`): vet and test, `make build-all`, then a release. Branch pushes never deploy.
- The pipelines call the Makefile targets, so keep build changes in the `Makefile`. See `docs/ci.md`.

## Optional features in this project
- Filesystem and environment: `contract.Sys`. See `docs/sys.md`.

## Decisions

Architectural decisions are recorded as ADRs in `docs/adr/`, one numbered file per decision (context, decision, consequences). Read them before changing layering, dependency injection, logging or configuration; a change that departs from an ADR adds a new ADR that supersedes it. `docs/adr/0001-composition-root-and-constructor-injection.md` records the rules above.
