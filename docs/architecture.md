# Architecture

**wc3-team-follow** follows a hexagonal (ports and adapters) architecture. The application core (`domain/`) talks to the outside world only through interfaces in `port/contract`. Adapters on the driving side (`cmd/`) call into the core; adapters on the driven side (`infrastructure/`) implement interfaces the core needs. All of them meet in one composition root.

## Ports and adapters

- **Ports** are the interfaces in `port/contract`. An inbound port such as `contract.ExampleService` is what the transports call; an outbound port is what the domain needs from the outside world. Each port has a production adapter and a test double in `mock/`.
- **Driving adapters** start work: the HTTP server in `cmd/http/` and the Cobra commands in `cmd/cli/`.
- **Driven adapters** fulfil outbound ports: the packages under `infrastructure/`. Infrastructure constructors return the contract, not their concrete type.
- **Dependency inversion**: high-level code depends on the interfaces; the concrete adapters are chosen in one place, `container/`, and passed in through constructors.

## Layers

| Package | Role | May import |
| --- | --- | --- |
| `main.go` | Composition root | `config`, `logging`, `container`, `cmd/cli` |
| `cmd/cli/` | Command tree; resolves dependencies from the container | `container`, `cmd/http`, `config`, `port` |
| `cmd/http/` | HTTP handler, middleware pipeline, server | `port`, `config`, `logging` |
| `container/` | Builds adapters and services lazily, closes them | everything it wires |
| `domain/` | Business logic | `port`, standard library |
| `port/contract/` | Interfaces only | `port/dto` |
| `port/dto/` | DTOs: in-app shapes in `package dto`, HTTP shapes in `port/dto/request/` and `port/dto/response/` | standard library |
| `infrastructure/` | Adapters to external systems | `port`, `config` |
| `logging/` | Builds the `*slog.Logger` | `config` |
| `config/` | Loads `config.toml` | standard library, Viper |
| `mock/` | Test doubles for every contract | `port` |

Only `main.go`, `cmd/cli/` and `container/` import `container`. `docs/container.md` explains why and how.

## Startup wiring

```text
main.go
  config.NewConfig()                  embedded config/config.toml + environment overrides
  logging.New(cfg.Logging, os.Stdout) the *slog.Logger, also slog.SetDefault
  container.New(cfg, logger)          lazy (T, error) accessors
  cli.Execute(c)                      Cobra command tree built with the container
    server command
      httpDeps(c, cmd)                resolves ExampleService, ... from the container
      httpserver.NewHandler(deps)     routes + middleware pipeline
      httpserver.StartServer(ctx, handler, opts)
    worker command
      resolveWorkerDeps(c)            resolves ExampleService
      runPingLoop(ctx, ...)           runs Ping every --interval until SIGTERM
    example-job command
      resolveJobDeps(c)               resolves ExampleService
      ExampleService.RunJob(ctx, job) one run, then exit
  c.Close()                           closes every adapter in reverse creation order
```

## Request flow

```text
client
  → middleware pipeline (cmd/http/middleware, order in docs/logging-and-middleware.md)
  → http.ServeMux route ("GET /example")
  → handler (cmd/http/app/example/handler) holding contract.ExampleService
  → domain/example.Service
```

Routes registered by `NewHandler`:

| Route | Served by |
| --- | --- |
| `GET /health` | `cmd/http/app/root/handler` (`response.Health`), liveness |
| `GET /health/ready` | `cmd/http/app/root/handler` (`response.Readiness`), readiness: runs `Deps.Readiness` checks (see `docs/container.md`) |
| `GET /example` | `cmd/http/app/example/handler` (`response.Example`) |
| `GET /docs/`, `GET /docs/swagger.json` | Swagger UI and `cmd/http/resource/swagger/swagger.json` |

Any other path returns 404.

## Configuration

`config/config.toml` is embedded into the binary at build time, so a changed file takes effect after the next build. At runtime any key can be overridden with an environment variable named after the key, upper-cased, with dots replaced by underscores:

```bash
HTTP_PORT=9090 LOGGING_LEVEL=debug ./bin/wc3-team-follow server
```

`server` flags (`--port`, `--host`, timeouts) override `[http]` when set explicitly. Keep secrets out of the TOML file; provide them through the environment.

## Decisions

Architectural decisions are recorded as ADRs in `docs/adr/`. `docs/adr/0001-composition-root-and-constructor-injection.md` records the layering, injection and logging rules above. Add a new ADR when a decision changes.
