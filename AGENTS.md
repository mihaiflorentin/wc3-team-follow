# AGENTS.md

Warcraft Team Follow is a Go 1.27 desktop app for Warcraft III: Reforged that follows friends into custom lobbies on Windows (amd64), macOS (arm64) and Linux (amd64, game under Wine/Proton). It is a hexagonal (ports and adapters) application with dependency inversion: business logic depends on interfaces in `port/contract`, adapters implement them, and one composition root wires everything together. Every change keeps the rules below. The docs in `docs/` explain each area in depth; read the matching doc before changing that area, and update it in the same change.

## Layout

| Path | Holds |
| --- | --- |
| `changelog.go` | Embeds `CHANGELOG.md` for What's new; `main.go` hands it on as `cfg.App.Changelog`. |
| `main.go` | Composition root: builds config, logger and container once, runs the CLI, closes the container. Opening the window on Windows and Linux first starts the crash supervisor. `console_windows.go` attaches the terminal for commands. |
| `crashreport/` | Error dialog when the window stops because of an error: a supervisor process on Windows and Linux, in-process on macOS. Used only by `main.go`. |
| `cmd/cli/` | Cobra commands. Wiring layer: resolves dependencies from the container and passes them on. Running with no command opens the window; `discover` checks game discovery; `say` types one chat line to check the game chat; `type` types one line in the match chat with keystrokes. |
| `cmd/gui/` | Driving desktop adapter: `Run(Deps, Options)` opens the Wails window; `Bridge` is the page's API. `frontend/` is plain HTML/CSS/JS, embedded. |
| `container/` | `ServiceContainer`: lazy `(T, error)` accessors and `Close`. Used only by the wiring layer. |
| `config/` | `config.toml` (embedded into the binary) and `NewConfig`. |
| `logging/` | `Setup` (console + rotated log file + crash file), `New` (console only) and the log handlers. Cross-cutting: every layer may receive a `*slog.Logger`. |
| `domain/follow/` | The follow state machine (port of 0.4's `team-follow.cjs`), Auto-host (`host.go`), the recent players recorder (`recent.go`) and the blacklist with auto-ban (`blacklist.go`, `ban.go`). Pure. |
| `domain/companion/` | `contract.Companion`: discovery loop, game session, saving friends and recent players, running update checks, publishing state. |
| `domain/update/` | The update check's decisions (ADR 0009): when to check, version comparison, Later and Skip, the restart warning, and the changelog parser (`changelog.go`). Pure. |
| `port/contract/` | Interfaces (ports) that domain and adapters meet at. Interfaces only, no structs. |
| `port/dto/` | Plain data shapes, `package dto`, no subfolders. |
| `infrastructure/discovery/` | Finds the game, menu port, session and version, and the Maps folders (`MapsLocator`, with Wine path translation). `Processes` closes the game's own processes when the player asks (`contract.GameProcesses`). `platform_<os>.go` per OS; portable parsing beside it. |
| `infrastructure/maps/` | Lists and searches maps in a Maps folder (`MapLibrary`). |
| `infrastructure/gamesocket/` | Loopback websocket to the game with the command allowlist; a reader goroutine queues events so `TryReceive` can return the ones already waiting. |
| `infrastructure/keyboard/` | Types one line in the match chat with keystrokes (`contract.GameKeyboard`, ADR 0007): the sequence and its checks in `keyboard.go`, `SendInput` in `platform_windows.go`, `CGEventPost` in `platform_darwin.go`, not supported elsewhere yet. |
| `infrastructure/updater/` | Finds the newest stable release and its CHANGELOG.md through GitHub's REST API (`contract.Updater`, ADR 0009); HTTPS to GitHub's hosts only. |
| `infrastructure/settings/` | `friends.json` (friends, Auto-host setup, blacklist, update settings), `recent-players.json` (`RecentPlayers`) and `notes.json` (`PlayerNotes`) in the per-user data folder. |
| `mock/` | Hand-written test doubles, one package per contract group. |
| `scripts/` | `release-tag.sh` classifies release tags; `releasenotes/` prints a release's notes from `CHANGELOG.md` for the release workflow. |
| `packaging/` | App icon sources and renders (`icon/`), the Windows resource description (`windows/winres.json`), the macOS `Info.plist` and entitlements (`darwin/`), and the Linux `.desktop` entry (`linux/`). |
| `docs/` | Living documentation; architectural decisions go in `docs/adr/`. |

## Project settings

- Release targets: `windows/amd64` (cross-compiled, no cgo, `-H windowsgui`), `darwin/arm64` (cgo, `.app` bundle, ad-hoc signed with `com.apple.security.cs.debugger`, macOS 13+), `linux/amd64` (cgo, WebKitGTK 4.1, tag `webkit2_41`). Builds need `-tags desktop,production`; use the `Makefile`.
- `[follow] verified_builds` gates live joining (`3.0.1.24371`). Never add a build without a live test.
- `[logging] default` is `color`, `level` is `debug`; the log file (`file = true`) is JSON at `debug`, 5 MB × 3 rotations.
- `[updates]` checks `mihaiflorentin/wc3-team-follow` on `https://api.github.com` every `6h` (retry `30m`, timeout `20s`); `main.go` sets `cfg.App.Version` from the build info.
- Per-user data: `%LOCALAPPDATA%\WarcraftTeamFollow` (Windows), `~/Library/Application Support/WarcraftTeamFollow` (macOS), `$XDG_CONFIG_HOME/WarcraftTeamFollow` (Linux). `WF_DATA_DIR` overrides it. On Windows the WebView2 profile is its `webview` subfolder.
- `LICENSE`: MIT, copyright 2026 Mihai Mihaila.

## Safety rules for game access

- Discovery is read-only and inspects only the Warcraft III process. Never write game memory, inject code or search for credentials.
- The only action that changes a process is closing the game: `contract.GameProcesses` (`infrastructure/discovery/processes.go`) runs only when the player clicks Close all Warcraft III and confirms in the window. It closes only processes discovery identifies as the game executable (never Battle.net or anything else), only the current user's (the process uid on Linux and macOS, the token's user SID on Windows; a process whose owner can't be read is left alone), re-checks each pid is still the game before signalling it, and asks first (SIGTERM, then SIGKILL after 3 s; `TerminateProcess` on Windows). Nothing closes the game automatically.
- The menu session, passwords and account ids are never logged, printed, saved or put in `FollowState` or `Diagnostics`.
- The game socket sends only the allowlisted commands in `infrastructure/gamesocket`, each one the game's own menu sends for the same purpose: watching and Auto-join (`TeamGetInfo`, `FriendsGetFriends`, `RequestCancelTeamInvitations`, `TeamLeave`, `JoinFriendGameLobby`, `JoinGameByGameName`, `SendGameListing`, `GetGameList`) and Auto-host (`CreateLobby`, `LobbyStart`, `LobbyCancel`, `SetTeam`, `SendInviteToGame`) and Auto-host's chat line (`SendGameChatMessage`) and auto-ban (`BanPlayerFromGameLobby`). `SendGameLobbySetup` and `GetGameInfo` crashed the game and must never be added; `KickPlayerFromGameLobby` isn't allowed.
- Auto-ban (ADR 0008) sends `BanPlayerFromGameLobby {slot}` only through `Engine.SendPendingBan`: only when the player hosts the lobby (`isHost`) and Auto-ban is on; only for an occupied human slot whose non-empty `name` matches a blacklisted BattleTag (case-insensitive); never for the player's own slot, computers or open slots; at most once per player per lobby, again if they rejoin; never once the countdown is locked or the match has started. It is chosen only from the newest `GameLobbySetup`, never from a timer; the companion first handles every event already waiting (`GameSession.TryReceive`) and the engine re-checks the player's slot; one ban is in flight at a time; and the game's "Host banned player [X]" line is checked, with a named warning when someone else was hit. There is no unban.
- Typing in the game chat (`SendGameChatMessage`, ADR 0006) goes only through `Engine.sendChat`, only for Auto-host's own lobby or match as host, and only one trimmed, non-empty line of at most 100 characters; anything else is rejected. Same team's `!swap <position> <position>` (issue 5) is the one typed line; Auto-join, watching and Auto-host's other steps type nothing. Auto-join's Join the host's team sends `SetTeam` for the player's own slot only. The `say` command is the one manual exception, with the same limits (`follow.ChatLine`).
- Keystrokes (ADR 0007) go only through `contract.GameKeyboard` (`infrastructure/keyboard`), only to the discovered game process and only while it is the window in front, checked before every key; the moment it isn't, or the context ends, typing stops and the error says the chat box may hold part of the line. Never bring the game forward, activate a window or steal focus. Only Enter and printable ASCII characters (no hotkeys, orders or Esc), one line of at most 100 characters, refused whole rather than cut short. Only Auto-host's own Game mode command, as host, after the player entered one and switched it on (off by default); the `type` command is the one manual exception. Keystrokes write no memory and inject no code. Linux isn't supported yet; never add uinput or portal input, which can't check the target.
- Nothing starts a match unless the host switched on Start when full; Auto-host otherwise only creates the lobby.
- Follow always starts off; nothing re-enables it automatically after a disconnect.
- Update checks (ADR 0009) go only through `contract.Updater` (`infrastructure/updater`): plain GETs to `api.github.com` and the release hosts it links (`github.com`, `objects.githubusercontent.com`, `release-assets.githubusercontent.com`), over HTTPS only, redirects included, carrying no token, cookie or anything about the player. Only clean `vX.Y.Z` builds check; never offer a pre-release such as `edge`. Changelog and release notes reach the page only as plain-text spans set with `textContent`, never as HTML. Nothing is downloaded or changed without the player's click on Update; the bridge opens only `https://github.com/` pages from the companion's state.
- Recent players (`follow.Recent`) only read the lobby events the game already sends; recording sends nothing. The list (BattleTags, map, last seen) stays in `recent-players.json` on this computer, never in `Diagnostics` or the log (only `recent.saved` with a count).
- Player notes are written only by `Companion.SetNote` into `notes.json` on this computer. They are never sent to the game, logged (only `notes.saved` with a count) or put in `Diagnostics`, and the window shows them as plain text, never HTML.

## Dependency rules

- Dependencies point inward. `domain/` imports only `port/` and the standard library. `infrastructure/` imports `port/` and `config/`, never `domain/` or `cmd/`. `cmd/gui/` imports `port/`, `config/` and `logging/`, never the container or `infrastructure/`.
- Only `main.go`, `cmd/cli/` and `container/` import `container`. There is no package-level container variable; nothing reaches into the container to look up a dependency.
- Consumers depend on `port/contract` interfaces, never on concrete types: the bridge takes `contract.Companion`, not `*companion.Service`. Adapter constructors return the contract (`func New(...) contract.X`).

## Composition root and dependency injection

- `main.go` calls `config.NewConfig()`, `logging.New(cfg.Logging, os.Stdout)`, `container.New(cfg, logger)` and `cli.Execute(c)`, then `c.Close()`. Nothing else builds a container.
- Each command receives the container through its constructor and resolves what it needs before doing any work. The root command resolves `gui.Deps` in `guiDeps` and builds `gui.Options` from `[window]`. `discover` takes a resolve function, so tests run it with mocks.
- Every collaborator arrives as a constructor parameter and is stored in a field: `companion.New(companion.Ports{Discovery, Dialer, Prefs, Maps, Library, Processes, Keyboard, Recent, Notes}, logger, opts)`. No globals, no `init()`, no config lookups inside domain code or the window.
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

- The process logger is the `*slog.Logger` built by `logging.Setup` in `main.go` (also installed with `slog.SetDefault`): the console plus a rotated JSON-lines debug file and a crash file in `<data folder>/logs`. Components receive it through their constructor and keep it in a field.
- Never log the menu session, passwords, account ids or event payloads; log command names and event types. New background goroutines start with a panic guard that logs the stack.
- A failure that ends the window must reach the player as a dialog (`crashreport`): keep the `error:` line in `main.go` and the `gui.startup` and `gui.already_open` log lines in `cmd/gui`, which the supervisor reads.
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
| `./bin/wc3-team-follow type [--delay 3s] [--ask-permission] -- "<text>"` | Type one line in the match chat with keystrokes after the delay, only while the game is in front; never brings it forward. For live checks. |
| `./bin/wc3-team-follow say "<text>" [--wait 10s]` | Type one line in the game chat and print the game's events (names, chat lines, channel names only). For live checks. |

Before finishing a change, run `make fmt`, `go vet ./...` and `go test ./...`.

## Changelog

`CHANGELOG.md` is for players: Team Follow shows its entries in the update prompt and again on the first start after an update. Keep it current with every change.

- Every change a player can notice adds a line under `## [Unreleased]` in the same commit, in the right group: `Added`, `Changed`, `Fixed`, `Removed` or `Security`. Internal work (refactors, tests, CI, docs for developers) gets no entry.
- Write for players: what they can now do or what changed for them, in plain words and full sentences. No commit hashes, issue numbers, package names or protocol commands. Bold the feature name when an entry introduces a feature.
- Never put a real BattleTag in it; use samples such as Player#1234.
- Before tagging a release, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` (the tag without the `v`, today's date), add a new empty `## [Unreleased]` above it, and update the compare links at the bottom. The release tag's version must have a matching section, or the release workflow fails; check with `go run ./scripts/releasenotes -tag vX.Y.Z`. The section becomes the GitHub Release's notes, and the app shows it as What's new.
- Never rewrite a released version's entries, except to fix a mistake.

## CI

- GitHub Actions: `.github/workflows/release.yml` tests on Windows, macOS and Linux, builds the three release files on suitable runners, and publishes a GitHub Release. A Release Tag (`X.Y.Z`, optional `v`, classified by `scripts/release-tag.sh`) publishes a versioned release; a push to `master` replaces the rolling `edge` pre-release.
- The pipeline calls the Makefile targets, so keep build changes in the `Makefile`. See `docs/ci.md`.

## Decisions

Architectural decisions are recorded as ADRs in `docs/adr/`, one numbered file per decision (context, decision, consequences). Read them before changing layering, dependency injection, the window, game discovery, logging or configuration; a change that departs from an ADR adds a new ADR that supersedes it.

- `0001`: composition root and constructor injection.
- `0002`: desktop window with Wails.
- `0003`: game discovery per operating system.
- `0005`: closing the game's processes on request.
- `0004`: crash dialog and window supervisor.
- `0006`: typing in the game chat.
- `0007`: typing in the match chat with keystrokes.
- `0008`: banning blacklisted players from lobbies the player hosts.
- `0009`: self-update: checking GitHub Releases and asking the player.

## Agent skills

### Issue tracker

GitHub Issues in `mihaiflorentin/wc3-team-follow`, through the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` at the root (created when needed) and `docs/adr/`. See `docs/agents/domain.md`.
