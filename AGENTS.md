# AGENTS.md

Warcraft Team Follow is a Go 1.27 desktop app for Warcraft III: Reforged that follows friends into custom lobbies on Windows (amd64), macOS (arm64) and Linux (amd64, game under Wine/Proton). It is a hexagonal (ports and adapters) application with dependency inversion: business logic depends on interfaces in `port/contract`, adapters implement them, and one composition root wires everything together. Every change keeps the rules below. The docs in `docs/` explain each area in depth; read the matching doc before changing that area, and update it in the same change.

## Layout

| Path | Holds |
| --- | --- |
| `main.go` | Composition root: builds config, logger and container once, runs the CLI, closes the container. `console_windows.go` attaches the terminal for commands. |
| `cmd/cli/` | Cobra commands. Wiring layer: resolves dependencies from the container and passes them on. Running with no command opens the window; `discover` checks game discovery. |
| `cmd/gui/` | Driving desktop adapter: `Run(Deps, Options)` opens the Wails window; `Bridge` is the page's API. `frontend/` is plain HTML/CSS/JS, embedded. |
| `container/` | `ServiceContainer`: lazy `(T, error)` accessors and `Close`. Used only by the wiring layer. |
| `config/` | `config.toml` (embedded into the binary) and `NewConfig`. |
| `logging/` | `New(cfg, w) *slog.Logger` and the log handlers. Cross-cutting: every layer may receive a `*slog.Logger`. |
| `domain/follow/` | The follow state machine. Pure; port of 0.4's `team-follow.cjs`. |
| `domain/companion/` | `contract.Companion`: discovery loop, game session, saving friends, publishing state. |
| `port/contract/` | Interfaces (ports) that domain and adapters meet at. Interfaces only, no structs. |
| `port/dto/` | Plain data shapes, `package dto`, no subfolders. |
| `infrastructure/discovery/` | Finds the game, menu port, session and version. `platform_<os>.go` per OS; portable parsing beside it. |
| `infrastructure/gamesocket/` | Loopback websocket to the game with the command allowlist. |
| `infrastructure/settings/` | `friends.json` in the per-user data folder. |
| `mock/` | Hand-written test doubles, one package per contract group. |
| `packaging/darwin/` | `Info.plist` and entitlements for the macOS app bundle. |
| `docs/` | Living documentation; architectural decisions go in `docs/adr/`. |

## Project settings

- Release targets: `windows/amd64` (cross-compiled, no cgo, `-H windowsgui`), `darwin/arm64` (cgo, `.app` bundle, ad-hoc signed with `com.apple.security.cs.debugger`, macOS 13+), `linux/amd64` (cgo, WebKitGTK 4.1, tag `webkit2_41`). Builds need `-tags desktop,production`; use the `Makefile`.
- `[follow] verified_builds` gates live joining (`3.0.1.24371`). Never add a build without a live test.
- `[logging] default` is `color`, `level` is `debug`.
- Per-user data: `%LOCALAPPDATA%\WarcraftTeamFollow` (Windows), `~/Library/Application Support/WarcraftTeamFollow` (macOS), `$XDG_CONFIG_HOME/WarcraftTeamFollow` (Linux). `WF_DATA_DIR` overrides it.
- `LICENSE`: MIT, copyright 2026 Mihai Mihaila.

## Safety rules for game access

- Discovery is read-only and inspects only the Warcraft III process. Never write game memory, inject code or search for credentials.
- The menu session, passwords and account ids are never logged, printed, saved or put in `FollowState` or `Diagnostics`.
- The game socket sends only the allowlisted commands in `infrastructure/gamesocket`. `SendGameLobbySetup` and `GetGameInfo` crashed the game and must never be added.
- Follow always starts off; nothing re-enables it automatically after a disconnect.

## Dependency rules

- Dependencies point inward. `domain/` imports only `port/` and the standard library. `infrastructure/` imports `port/` and `config/`, never `domain/` or `cmd/`. `cmd/gui/` imports `port/`, `config/` and `logging/`, never the container or `infrastructure/`.
- Only `main.go`, `cmd/cli/` and `container/` import `container`. There is no package-level container variable; nothing reaches into the container to look up a dependency.
- Consumers depend on `port/contract` interfaces, never on concrete types: the bridge takes `contract.Companion`, not `*companion.Service`. Adapter constructors return the contract (`func New(...) contract.X`).

## Composition root and dependency injection

- `main.go` calls `config.NewConfig()`, `logging.New(cfg.Logging, os.Stdout)`, `container.New(cfg, logger)` and `cli.Execute(c)`, then `c.Close()`. Nothing else builds a container.
- Each command receives the container through its constructor and resolves what it needs before doing any work. The root command resolves `gui.Deps` in `guiDeps` and builds `gui.Options` from `[window]`. `discover` takes a resolve function, so tests run it with mocks.
- Every collaborator arrives as a constructor parameter and is stored in a field: `companion.New(discovery, dialer, prefs, logger, opts)`. No globals, no `init()`, no config lookups inside domain code or the window.
- Container accessors are lazy singletons returning `(T, error)`. A missing config section is an error, and a failed build is retried on the next call. Adapters that hold connections register `s.onClose`.

To add a dependency (an outbound adapter or a domain service):

1. Declare the interface in `port/contract/<name>.go`.
2. Implement it in its own package with a `New` constructor that returns the contract.
3. Add a mock in `mock/<name>/` that starts with `var _ contract.<Name> = (*X)(nil)`.
4. If it needs settings, add a `[<name>]` section to `config/config.toml` and a pointer field to `config.Config`.
5. Add a lazy accessor to `container/infrastructure.go` (adapters) or `container/domain.go` (domain services) that returns `(contract.<Name>, error)` and reports a missing config section as an error.
6. Add the dependency as a constructor parameter of its consumer, and resolve it in `container/` or `cmd/cli/`.
7. Document it in `docs/` and in this file.

`docs/container.md` shows the accessor pattern in full.

## Logging

- The process logger is the `*slog.Logger` built by `logging.New` in `main.go` (also installed with `slog.SetDefault`). Components receive it through their constructor and keep it in a field.
- Log a dotted event name as the message and the details as typed attributes: `logger.InfoContext(ctx, "companion.connected", slog.String("version", v))`.
- Return errors upward; log them once, at the edge that handles them. Errors meant for players are full sentences starting with a capital letter. See `docs/logging.md`.

## DTOs

- Interfaces go in `port/contract`; every DTO goes in `port/dto` (`package dto`). Never declare a struct in `port/contract`.
- `FollowState` and `FollowConfig` are the window's contract: rename a JSON field in Go and `cmd/gui/frontend/app.js` together. See `docs/data-contracts.md`.

## Coding style

- `gofmt` everything (`make fmt`) and follow the Go style guide. See `docs/coding-style.md`.
- Constructors are named `New`/`NewX` and take every dependency as a parameter.
- Wrap errors with context: `fmt.Errorf("list game listeners: %w", err)`. I/O methods take `ctx context.Context` first.
- OS-specific code goes in `platform_<os>.go` with a build tag; keep parsing in untagged files so it is tested on every OS.
- Interface text is sentence case and says what happens; behaviour rules live in `domain/follow`, not in `app.js`.

## Testing

- Tests sit next to the code they cover; run `go test ./...`. They need no game, no window and no network beyond loopback.
- Follow engine: drive `follow.Engine` with a fake clock and sender (`domain/follow/engine_test.go`).
- Companion: construct it with `mock/game` and `mock/preferences` and drive `Run` (`domain/companion/service_test.go`).
- Discovery: portable parsing against a fake `/proc`, fake memory and `httptest` (`infrastructure/discovery/discovery_test.go`).
- Window: test `gui.Bridge` with `mock/companion` (`cmd/gui/bridge_test.go`). Check the page in a browser with its demo bridge (`?scene=...`).
- Commands: build them with a resolve function returning mocks, then `Execute` (`cmd/cli/discover_test.go`).
- Container: lazy build, `Close` order and the companion accessor (`container/container_test.go`).
- Every new contract gets a mock with a compile-time assertion.

## Commands

| Command | Purpose |
| --- | --- |
| `make build` | Build `bin/wc3-team-follow` for this computer. |
| `make run` | Build and open the window. |
| `make test` / `go test ./...` | Run every unit test. |
| `go vet ./...` | Static checks. |
| `make fmt` | `gofmt` every Go file. |
| `make lint` | `golangci-lint run ./...`. |
| `make tidy` | `go mod tidy` after changing imports. |
| `make package-windows-amd64` | Windows zip into `dist/` (any OS). |
| `make package-darwin-arm64` | macOS `.app` and zip into `dist/` (macOS only). |
| `make package-linux-amd64` | Linux tarball into `dist/` (Linux only, needs `libwebkit2gtk-4.1-dev`). |
| `./bin/wc3-team-follow discover` | Find the running game; prints pid, version and port, never the session. |

Before finishing a change, run `make fmt`, `go vet ./...` and `go test ./...`.

## CI

- GitHub Actions: `.github/workflows/release.yml` tests on Windows, macOS and Linux, builds the three release files on suitable runners, and publishes a GitHub Release. A Release Tag (`X.Y.Z`, optional `v`, classified by `scripts/release-tag.sh`) publishes a versioned release; a push to `master` replaces the rolling `edge` pre-release.
- The pipeline calls the Makefile targets, so keep build changes in the `Makefile`. See `docs/ci.md`.

## Decisions

Architectural decisions are recorded as ADRs in `docs/adr/`, one numbered file per decision (context, decision, consequences). Read them before changing layering, dependency injection, the window, game discovery, logging or configuration; a change that departs from an ADR adds a new ADR that supersedes it.

- `0001`: composition root and constructor injection.
- `0002`: desktop window with Wails.
- `0003`: game discovery per operating system.
