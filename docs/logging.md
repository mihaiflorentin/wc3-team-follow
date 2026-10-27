# Logging

`main.go` builds the process logger once with `logging.Setup(cfg.Logging, os.Stdout, dataDir)`, installs it with `slog.SetDefault` and passes it to `container.New`. From there it reaches every component through constructors: `c.Logger()` in `cmd/cli/`, `gui.Deps.Logger` in the window, and a constructor parameter everywhere else. `logging.New(cfg, w)` builds just the console logger, which tests use.

## Where logs go

| Output | Format | Level |
| --- | --- | --- |
| Console (`os.Stdout`) | `[logging] default`: `simple`, `color`, `json` or `pretty-json` | `[logging] level` |
| `<data folder>/logs/team-follow.log` | JSON lines, each with `time`, `level`, `source` (function, file, line) and `msg` | `[logging] file_level` (`debug`) |
| `<data folder>/logs/crash.log` | Go runtime crash reports: fatal errors and unrecovered panics with every goroutine's stack (`debug.SetCrashOutput`) | always |

The data folder is `config.DataDir`: `%LOCALAPPDATA%\WarcraftTeamFollow`, `~/Library/Application Support/WarcraftTeamFollow` or `~/.config/WarcraftTeamFollow`. The log file rotates at `file_max_mb` (5 MB), keeping `file_keep` (3) old files as `team-follow.log.1` … `.3`; each run appends. `[logging] file = false` turns the files off. When the file can't be opened, the app still runs and logs to the console.

The Windows release is a GUI program with no console, so for it the file is the log. A command started from a terminal (for example `discover`) attaches to that terminal and prints there too.

## Getting logs from a player

The window's **Report a problem** dialog shows the diagnostic report, the log folder's path and an **Open log folder** button. Ask for the report plus `team-follow.log` (and `crash.log` if it isn't empty). Every run starts with an `app.start` line naming the version, commit, OS, architecture, Go version and folders.

## What is logged

| Event | Level | Where |
| --- | --- | --- |
| `app.start`, `app.exit`, `app.panic` (with stack) | info / error | `main.go` |
| `discovery.process`, `.listeners`, `.probe` (per port), `.session` (digit count and time taken), `.found` | debug | `infrastructure/discovery` |
| `companion.run`, `.connected` (version, pid, port, verified), `.waiting` (each new reason once), `.disconnected`, `.stop` | info | `domain/companion` |
| `companion.send` (command name), `companion.event` (event type) | debug | `domain/companion` |
| `follow.status`: every status line the engine sets | info | `domain/companion` |
| `follow.configure` (what changed, or why it was refused), `follow.password` (sent or refused) | info | `domain/companion` |
| `companion.panic` (with stack) in background work | error | `domain/companion` |

`GameListUpdate` and `GameListRemove` stream several times a second in Custom Games, so they are counted in diagnostics instead of logged.

## Privacy

Logs never contain the game's menu session, lobby passwords, account ids or event payloads. Only command names and event types are logged; `domain/companion/service_test.go` checks the session and a password stay out. Logs do contain BattleTags (status lines such as "Asked to join Arthas#1123's lobby", chosen friends in `follow.configure`) and lobby names, which is what makes them useful for debugging. The diagnostic report has neither.

## Conventions

```go
logger.InfoContext(ctx, "companion.connected", slog.String("version", conn.Version))
logger.WarnContext(ctx, "companion.send", slog.String("command", message), slog.Any("error", err))
```

- The message is a dotted event name; details go in typed attributes. Durations are logged as text (`"10s"`).
- Return errors to the caller and log them once, where they are handled.
- Background goroutines start with a panic guard that logs the stack and re-panics, so a crash is explained in `team-follow.log` as well as `crash.log`.
