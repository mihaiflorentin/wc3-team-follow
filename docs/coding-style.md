# Coding style

These conventions match the generated code; keep new code consistent with them. Layering and injection rules are in `docs/architecture.md` and `docs/container.md`.

## Layers

- `cmd/cli/` is the only place that resolves dependencies from the container. `cmd/http/` receives everything it needs in `httpserver.Deps`; handlers hold `port/contract` interfaces, never concrete domain or adapter types.
- `domain/` imports only `port/` and the standard library. A domain package exposes a type with behaviour (`example.Service`) built by a constructor that takes its collaborators as contracts.
- Data crosses layer boundaries as contracts and DTOs (`docs/data-contracts.md`), never as another layer's internal structs.

## Constructors and dependencies

- Every type with dependencies has a `New`/`NewX` constructor taking each dependency as a parameter and storing it in an unexported field. Optional knobs go in an options struct (`httpserver.Options`).
- Adapter constructors return the contract they implement, plus an error when construction can fail.
- No package-level mutable state and no `init()` functions. Constants and embedded files are fine.
- Inject a `*slog.Logger` into anything that logs; do not reach for `slog.Default()` in library code.

## Errors and context

- Return errors; wrap them with what was being attempted: `fmt.Errorf("list examples: %w", err)`.
- Log an error once, where it is handled (handler, command, middleware), then respond or exit. Do not log and return the same error.
- Functions that do I/O take `ctx context.Context` as their first parameter and honour cancellation.

## HTTP

- Register routes with method and path patterns (`"GET "+RouteHealth`) and keep route constants exported so tests can use them.
- Handlers are small: decode input, call one contract method, map the result, write the response.
- Middleware has the shape `func(http.Handler) http.Handler` and is registered in `Pipeline` (`docs/logging-and-middleware.md`).

## Formatting and style

- Run `make fmt` (`gofmt`) and `go vet ./...` before committing; `make lint` runs `golangci-lint`.
- Follow the Go style guide: https://google.github.io/styleguide/go/.
- Prefer small functions and one aggregate argument over long parameter lists; accept slices only when the behaviour is about a collection.
- Doc comments describe behaviour and contracts, not history.
