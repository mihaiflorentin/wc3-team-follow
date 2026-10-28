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
- `c.Close()` runs every registered close hook in reverse creation order and joins their errors. It is safe to call more than once.

## Files

- `container/main.go`: `ServiceContainer`, `New`, `Close`, the `onClose` hook registry and the generic `lazy[T]` singleton.
- `container/infrastructure.go`: one accessor per outbound adapter.
- `container/domain.go`: one accessor per domain service.

## Accessors

Every accessor is a method returning `(T, error)`, where `T` is a `port/contract` interface. It is built once on first successful use; concurrent first calls build it once, and a failed build is not cached, so a later call retries.

| Accessor | Builds |
| --- | --- |
| `GameDiscovery()` | `discovery.New(logger)`, the implementation for this OS |
| `GameProcesses()` | `discovery.NewProcesses(logger)`, the game process closer for this OS |
| `GameKeyboard()` | `keyboard.New()`, the match chat keystroke input for this OS (ADR 0007); no config |
| `GameDialer()` | `gamesocket.NewDialer()` |
| `PreferenceStore()` | `settings.New(dir)` in `settings.DataDir(cfg.Storage)`; a missing `[storage]` is an error |
| `RecentPlayers()` | `settings.NewRecentPlayers(dir)` in the same data folder; a missing `[storage]` is an error |
| `Companion()` | `companion.New(Ports{discovery, dialer, prefs, maps locator, map library, processes, keyboard, recent players}, logger, options from [follow])`; a missing `[follow]` or a bad duration is an error |

The domain accessor shows the pattern: read and validate config, resolve collaborators, wrap their errors, pass them to the constructor.

```go
// container/domain.go
discovery, err := s.GameDiscovery()
if err != nil {
	return nil, fmt.Errorf("resolve game discovery: %w", err)
}
```

An adapter that holds a connection registers `s.onClose(name, client.Close)`. The game session is owned by the companion, which opens and closes it as the game comes and goes, so no adapter registers one today.

## Resolving dependencies

Resolve from the container only in `cmd/cli/`, once per command, before doing any work. The window command resolves into `gui.Deps` in `guiDeps`; `discover` takes a resolve function, so its test passes a mock:

```go
// cmd/cli/window.go
if deps.Companion, err = c.Companion(); err != nil {
	return deps, fmt.Errorf("resolve companion: %w", err)
}
```

## Adding a service

1. Declare its interface in `port/contract`.
2. Write the implementation with a `New...` constructor taking every collaborator (contracts and `*slog.Logger`) as parameters.
3. Add a `lazy[contract.X]` field to `DomainContainer` (or `InfrastructureContainer` for an adapter) and an accessor like the ones above.
4. Resolve it in the command that needs it and pass it on through constructors (for the window, add a field to `gui.Deps`).
5. Add a mock in `mock/` and tests that use it.
