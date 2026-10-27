# Container and composition root

`container.ServiceContainer` builds the application's adapters and services. It is used only at the composition root: `main.go` creates it, `cmd/cli/` resolves dependencies from it, and everything else receives those dependencies through constructors. Only `main.go`, `cmd/cli/` and `container/` import the package, and there is no package-level container variable.

## Lifecycle

```go
// main.go
cfg, err := config.NewConfig()
if err != nil {
	return fmt.Errorf("load config: %w", err)
}

logger := logging.New(cfg.Logging, os.Stdout)
slog.SetDefault(logger)

c := container.New(cfg, logger)
return errors.Join(cli.Execute(c), c.Close())
```

- `container.New(cfg, logger)` builds nothing yet; every accessor is lazy.
- `c.Config()` and `c.Logger()` return what the container was built with.
- `c.ReadinessChecks()` lists the readiness checks registered so far (it implements `contract.ReadinessChecks`). The `server` command passes the container as `httpserver.Deps.Readiness`, and `GET /health/ready` runs every check in parallel within `[http] readiness_timeout` (default `2s`), answering 503 with the names of the checks that failed or timed out. `GET /health` stays a plain liveness 200.
- `c.Close()` runs every registered close hook in reverse creation order and joins their errors. It is safe to call more than once; the `server` command calls it after a graceful shutdown and `main.go` calls it on exit.

## Files

- `container/main.go` — `ServiceContainer`, `New`, `Close`, the `onClose` hook registry, the `onReady` readiness check registry (`ReadinessChecks`) and the generic `lazy[T]` singleton.
- `container/infrastructure.go` — `InfrastructureContainer` and one accessor per outbound adapter.
- `container/domain.go` — `DomainContainer` and one accessor per domain service.

## Accessors

Every accessor is a method returning `(T, error)`, where `T` is a `port/contract` interface. It is built once on first successful use; concurrent first calls build it once, and a failed build is not cached, so a later call retries.

The domain service accessor shows the pattern: resolve collaborators, wrap their errors, pass them to the constructor.

```go
// container/domain.go
func (s *ServiceContainer) ExampleService() (contract.ExampleService, error) {
	return s.domain.exampleService.get(func() (contract.ExampleService, error) {
		return example.NewService(s.logger), nil
	})
}
```

Accessors wrap every error with the step that failed (`create redis client: ...`, `resolve mysql: ...`), so a failure reads like a path from the command down to the backend. Adapter accessors additionally:

- read their config section and return an error when it is missing (`missingSectionError("<section>")`), so `config.toml` without a section fails clearly instead of panicking;
- register the adapter's `Close(ctx context.Context) error` method as a close hook with `s.onClose(name, client.Close)` when the adapter holds connections; every closable contract has that signature;
- register the adapter's `Ping(ctx context.Context) error` method as a readiness check with `s.onReady(name, client.Ping)` next to the close hook, so `GET /health/ready` covers it. `Ping` must return once `ctx` is done. Adapters without a connection to check (the StatsD client, which sends UDP, `Sys`) register none.

Available accessors in this project:

- `Sys() (contract.Sys, error)` — see `docs/sys.md`.

## Resolving dependencies

Resolve from the container only in `cmd/cli/`, once per command, before doing any work. The `server` command shows the pattern in `httpDeps` (`worker` and `example-job` do the same in `resolveWorkerDeps` and `resolveJobDeps`):

```go
// cmd/cli/server.go
var err error
if deps.ExampleService, err = c.ExampleService(); err != nil {
	return deps, fmt.Errorf("resolve example service: %w", err)
}
```

The resolved `httpserver.Deps` then go to `httpserver.NewHandler`, together with the settings in `httpserver.HandlerOptions` (built by `httpHandlerOptions` from config and flags), and each dependency reaches the handler that needs it. Handlers, middleware, domain services and adapters never see the container.

## Adding a service

1. Declare its interface in `port/contract`.
2. Write the implementation with a `New...` constructor taking every collaborator (contracts and `*slog.Logger`) as parameters.
3. Add a `lazy[contract.X]` field to `DomainContainer` (or `InfrastructureContainer` for an adapter) and an accessor like the one above.
4. Resolve it in the command that needs it and pass it on through constructors (for HTTP, add a field to `httpserver.Deps`).
5. Add a mock in `mock/` and tests that use it.
