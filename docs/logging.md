# Logging

`logging.New(cfg *config.LoggingConfig, w io.Writer) *slog.Logger` builds the process logger. `main.go` calls it once with `cfg.Logging` and `os.Stdout`, installs the result with `slog.SetDefault`, and passes it to `container.New`. From there it reaches every component through constructors: `c.Logger()` in `cmd/cli/`, `gui.Deps.Logger` in the window, and a constructor parameter everywhere else.

The formats are the typed constants `logging.LoggerTypeSimple`, `LoggerTypeColor`, `LoggerTypeJSON` and `LoggerTypePrettyJSON`; one switch in `logging/logger.go` maps them to handlers.

`[logging]` in `config/config.toml`:

| Key | Values | Effect |
| --- | --- | --- |
| `default` | `simple` (fallback), `color`, `json`, `pretty-json` | Format of every log record. |
| `level` | `debug`, `info` (fallback), `warn`, `error` | Minimum level. |

All handlers in `logging/` keep attributes and groups added with `logger.With(...)` and `logger.WithGroup(...)`.

## Conventions

```go
logger.InfoContext(ctx, "companion.connected", slog.String("version", conn.Version))
logger.WarnContext(ctx, "companion.send", slog.String("command", message), slog.Any("error", err))
```

- The message is a dotted event name; details go in typed attributes.
- Return errors to the caller and log them once, where they are handled.
- Never log the menu session, passwords, account ids or the session URL. Discovery logs the process id, port and version only, and the websocket dialer keeps the URL out of its errors.

The Windows release is a GUI program, so it has no console. Its logs are only visible when it is started from a terminal with a command (for example `discover`), which attaches to that terminal.
