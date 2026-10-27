# Logging

`main.go` builds the process logger once with `logging.Setup(cfg.Logging, os.Stdout, dataDir)`, installs it with `slog.SetDefault` and passes it to `container.New`. From there it reaches every component through constructors: `c.Logger()` in `cmd/cli/`, `gui.Deps.Logger` in the window, and a constructor parameter everywhere else. `logging.New(cfg, w)` builds just the console logger, which tests use.

## Where logs go

| Output | Format | Level |
| --- | --- | --- |
| Console (`os.Stdout`) | `[logging] default`: `simple`, `color`, `json` or `pretty-json` | `[logging] level` |
| `<data folder>/logs/team-follow.log` | JSON lines, each with `time`, `level`, `source` (function, file, line) and `msg` | `[logging] file_level` (`debug`) |
| `<data folder>/logs/crash.log` | Go runtime crash reports: fatal errors and unrecovered panics with every goroutine's stack (`debug.SetCrashOutput`), and one line for every crash dialog the supervisor showed | always |

The data folder is `config.DataDir`: `%LOCALAPPDATA%\WarcraftTeamFollow`, `~/Library/Application Support/WarcraftTeamFollow` or `~/.config/WarcraftTeamFollow`. The log file rotates at `file_max_mb` (5 MB), keeping `file_keep` (3) old files as `team-follow.log.1` … `.3`; each run appends. `[logging] file = false` turns the files off. When the file can't be opened, the app still runs and logs to the console.

The Windows release is a GUI program with no console, so for it the file is the log. A command started from a terminal (for example `discover`) attaches to that terminal and prints there too.

## Crash dialog

When the window stops because of an error, the player sees a dialog with the reason and the log folder (`crashreport`, see `docs/adr/0004-crash-dialog-and-supervisor.md`). On Windows and Linux a supervisor process runs the window as a child and reads the reason from its output: a `panic:`, `fatal error:` or `SIG…:` line, `[WebView2 Error]`, the app's `error:` line, or a `log.Fatal` line. It stays quiet for a stop the player asked for and for a second launch that hands over to the open window (code 1 before `gui.startup`, or after `gui.already_open`). If the window hasn't started within a minute, it tells the player the usual cause (WebView2 on Windows, WebKitGTK on Linux) and closes the stuck window process. On macOS the window process shows the dialog itself for an error returned by `run` or a panic on the main goroutine.

## Getting logs from a player

The window's **Report a problem** dialog shows the diagnostic report, the log folder's path and an **Open log folder** button. Ask for the report plus `team-follow.log` (and `crash.log` if it isn't empty). Every run starts with an `app.start` line naming the version, commit, OS, architecture, Go version and folders.

## What is logged

| Event | Level | Where |
| --- | --- | --- |
| `app.start`, `app.exit`, `app.panic` (with stack) | info / error | `main.go` |
| `gui.startup`: the window has started (the crash reporter's marker) | info | `cmd/gui` |
| `gui.already_open`: another window holds the one-instance lock, so this launch hands over to it (Linux; the crash reporter's marker) | info | `cmd/gui` |
| `gui.wails` (`message`): Wails' own messages, such as web view start-up and process failures | debug to error | `cmd/gui` |
| `discovery.process`, `.listeners`, `.probe` (per port), `.session` (digit count and time taken), `.found` | debug | `infrastructure/discovery` |
| `discovery.close` (pids asked to close, pids forced, failures), `discovery.close_skipped` (count of other users' games left alone) | info | `infrastructure/discovery` |
| `companion.close_games` (count closed; the error at warn) | info / warn | `domain/companion` |
| `companion.run`, `.connected` (version, pid, port, verified), `.waiting` (each new reason once), `.disconnected` (error, or `reason` logged out), `.stop` | info | `domain/companion` |
| `companion.send` (command name), `companion.event` (event type) | debug | `domain/companion` |
| `companion.screen`: the game screen the menu switched to (`SetGlueScreen`) | info | `domain/companion` |
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
