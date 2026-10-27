# 1. Composition root and constructor injection

Status: accepted (2026-10-27T16:36:16+02:00, generated with the project)

## Context

wc3-team-follow is organised as a hexagonal application: domain code depends on interfaces in `port/contract`, and adapters implement them. That only holds if nothing in the domain or the transports reaches out for its dependencies. A global service locator, a package-level logger or config read at run time would each let a layer depend on concrete wiring again and make it hard to test in isolation.

## Decision

- `main.go` is the composition root. It builds the configuration (`config.NewConfig`), the logger (`logging.New`) and the `container.ServiceContainer` (`container.New`) exactly once, hands the container to the CLI (`cli.Execute`) and closes it on exit.
- The container is a wiring helper, not a locator. Only `main.go`, `cmd/cli/` and `container/` import it, and there is no package-level container variable. Its accessors are lazy singletons that return `(T, error)`, so a missing config section or an unreachable backend is reported where the dependency is resolved.
- Every other component receives its collaborators, `port/contract` interfaces and a `*slog.Logger`, through its constructor.
- Logging is a top-level, cross-cutting `logging` package beside `config`, not an infrastructure adapter. It builds the `*slog.Logger` that is injected everywhere.
- Configuration stays TOML (`config/config.toml`, embedded into the binary), overridable per key through environment variables.
- Agent and contributor rules live in `AGENTS.md`; decisions live in `docs/adr/`.

## Consequences

- Each layer can be tested with the mocks in `mock/`, and the window's bridge with `mock/companion`, without the container.
- Adding a dependency means adding a contract, an implementation, a mock, a container accessor and a constructor parameter. That is more steps than a global lookup, and it is the intended price.
- Commands fail before doing work when a dependency cannot be built, instead of failing on first use.
