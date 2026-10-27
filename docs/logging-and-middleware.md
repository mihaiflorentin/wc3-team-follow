# Logging and middleware

## The logger

`logging.New(cfg *config.LoggingConfig, w io.Writer) *slog.Logger` builds the process logger. `main.go` calls it once with `cfg.Logging` and `os.Stdout`, installs the result with `slog.SetDefault`, and passes it to `container.New`. From there it reaches every component through constructors (`c.Logger()` in `cmd/cli/`, `Deps.Logger` in `cmd/http/`, a constructor parameter everywhere else).

`logging.NewAccessLogger(cfg, w)` builds the HTTP access logger the same way, in the `server_default` format. The `server` command passes it as `Deps.AccessLogger`.

The formats are the typed constants `logging.LoggerTypeSimple`, `LoggerTypeColor`, `LoggerTypeJSON` and `LoggerTypePrettyJSON`; one switch in `logging/logger.go` maps them to handlers for both loggers.

`[logging]` in `config/config.toml`:

| Key | Values | Effect |
| --- | --- | --- |
| `default` | `simple` (fallback), `color`, `json`, `pretty-json` | Format of every log record. |
| `level` | `debug`, `info` (fallback), `warn`, `error` | Minimum level. |
| `server_default` | the same formats as `default`; empty means `default` | Format of the access log. |

All handlers in `logging/` keep attributes and groups added with `logger.With(...)` and `logger.WithGroup(...)`.

Log conventions:

```go
logger.InfoContext(ctx, "http.server", slog.String("addr", addr))
logger.ErrorContext(r.Context(), "example.ping", slog.Any("error", err))
```

The message is a dotted event name; details go in typed attributes. Return errors to the caller and log them once where they are handled.

## Middleware pipeline

`httpserver.NewHandler(deps, opts)` registers the routes on an `http.ServeMux` and wraps it with `httpserver.Pipeline(deps, opts, mux)`. `httpserver.Deps` holds the dependencies (logger, contracts, the shutdown callback) and `httpserver.HandlerOptions` the settings. `Pipeline` registers middleware on a `middleware.Chain`; execution order is registration order, so the first registered runs outermost. In this order:

- **Recover** (`middleware.Recover`) — turns a panic anywhere below it into a logged `http.panic` record and a 500.
- **Access log** (`middleware.AccessLog`) — one `http.access` record per request, through `Deps.AccessLogger`.
- **Max requests** (`middleware.MaxRequests`) — with `server --shutdown-max-requests N`, requests a graceful shutdown exactly once, on the N-th request. Disabled when N is 0. Add `--shutdown-max-requests-jitter J` to raise the limit by a random 0..J (chosen once at startup), so replicas started together do not restart in lockstep; the default 0 keeps the limit at exactly N.

Then the mux routes the request to its handler.

Each middleware is a constructor returning `func(http.Handler) http.Handler`: `middleware.Recover(logger)`, `middleware.AccessLog(logger)` and `middleware.MaxRequests(limit, jitter, shutdown, logger)`.

## Access log

Every request produces one record whose message is the event name `http.access` and whose details are typed attributes, in every format:

| Attribute | Type | Value |
| --- | --- | --- |
| `method` | string | Request method. |
| `url` | string | Request URL (path and query). |
| `status` | int | Response status code. |
| `status_class` | string | `1xx` … `5xx`. |
| `duration` | duration | Time spent in the handler chain below the access log. |
| `remote_address` | string | Client address. |
| `user_agent` | string | `User-Agent` header. |
| `request_body` | string | Request body. |
| `response_body` | string | Response body of non-2xx responses, empty otherwise. |

The format (`simple`, `color`, `json`, `pretty-json`) only changes how the record is rendered. The access log captures at most `middleware.MaxLogBody` (4 KiB) of the request body and of non-2xx response bodies, and the handler still receives the full request body.

## Adding middleware

Write it as `func(http.Handler) http.Handler`, take its dependencies (logger, contracts) as constructor parameters, and register it in `Pipeline` at the position it needs. Middleware that inspects the response wraps the writer with `middleware.NewRecorder(w)`, which records `StatusCode` and keeps `Flush` and `Unwrap` working. Test it through `httpserver.Pipeline` with `httptest`, as `cmd/http/pipeline_test.go` does.
