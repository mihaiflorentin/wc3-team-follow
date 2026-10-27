# Coding style

These conventions match the generated code; keep new code consistent with them. Layering and injection rules are in `docs/architecture.md` and `docs/container.md`.

## Layers

- `cmd/cli/` is the only place that resolves dependencies from the container. `cmd/gui/` receives everything it needs in `gui.Deps`; the `Bridge` holds `contract.Companion`, never the concrete service.
- `domain/` imports only `port/` and the standard library. A domain package exposes a type with behaviour (`companion.Service`, `follow.Engine`) built by a constructor that takes its collaborators as contracts.
- Data crosses layer boundaries as contracts and DTOs (`docs/data-contracts.md`), never as another layer's internal structs.

## Constructors and dependencies

- Every type with dependencies has a `New`/`NewX` constructor taking each dependency as a parameter and storing it in an unexported field. Optional knobs go in an options struct (`companion.Options`, `gui.Options`).
- Adapter constructors return the contract they implement, plus an error when construction can fail.
- No package-level mutable state and no `init()` functions. Constants and embedded files are fine.
- Inject a `*slog.Logger` into anything that logs; do not reach for `slog.Default()` in library code.

## Errors and context

- Return errors; wrap them with what was being attempted: `fmt.Errorf("list game listeners: %w", err)`.
- Log an error once, where it is handled (command, companion loop, window), then show it or exit. Do not log and return the same error.
- Errors that reach players are full sentences starting with a capital letter (discovery's `ErrNoGame`, the engine's refusals). Internal errors stay lower-case and wrapped; the companion shows a generic hint for them.
- Functions that do I/O take `ctx context.Context` as their first parameter and honour cancellation.

## Platform code

- OS-specific code lives in files named `platform_<os>.go` with a matching build tag. Keep parsing and logic in untagged files (`procfs.go`, `lsof.go`, `scan.go`) so their tests run on every OS.
- Every adapter for the game is read-only. Never write game memory, inject code or send commands outside the allowlist in `infrastructure/gamesocket`.

## Window

- `cmd/gui/frontend/` is plain HTML, CSS and JavaScript with no build step. Colours, type and spacing are CSS custom properties at the top of `style.css`; reuse them.
- Interface text is sentence case and names actions by what they do ("Start preview", "Stop"). Errors say what to do next.
- The page derives everything it shows from `FollowState`; behaviour rules belong in `domain/follow`, not in `app.js`.

## Formatting and style

- Run `make fmt` (`gofmt`) and `go vet ./...` before committing; `make lint` runs `golangci-lint`.
- Follow the Go style guide: https://google.github.io/styleguide/go/.
- Prefer small functions and one aggregate argument over long parameter lists; accept slices only when the behaviour is about a collection.
- Doc comments describe behaviour and contracts, not history.
