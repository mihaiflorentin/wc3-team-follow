package http

import (
	"log/slog"
	"net/http"
	"time"

	examplehandler "github.com/mihaiflorentin/wc3-team-follow/cmd/http/app/example/handler"
	roothandler "github.com/mihaiflorentin/wc3-team-follow/cmd/http/app/root/handler"
	"github.com/mihaiflorentin/wc3-team-follow/cmd/http/middleware"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

// RouteHealth is the liveness probe: a plain 200 while the process runs.
const RouteHealth = "/health"

// RouteReady is the readiness probe: 200 when every dependency check passes,
// 503 naming the failing checks otherwise.
const RouteReady = "/health/ready"
const RouteExample = "/example"

// Deps are the resolved dependencies of the HTTP layer. The CLI server
// command resolves them from the container; tests fill them with mocks.
type Deps struct {
	Logger *slog.Logger
	// AccessLogger writes the access log, in the [logging] server_default
	// format (see logging.NewAccessLogger). Nil falls back to Logger.
	AccessLogger   *slog.Logger
	ExampleService contract.ExampleService
	// Readiness lists the dependency checks GET /health/ready runs (the
	// container implements it). Nil means always ready.
	Readiness contract.ReadinessChecks
	// Shutdown requests a graceful shutdown, typically by cancelling the
	// context passed to StartServer. It is called at most once.
	Shutdown func()
}

// HandlerOptions are the settings of the HTTP layer. The zero value is valid.
type HandlerOptions struct {
	// MaxRequests calls Deps.Shutdown once after that many requests (0 disables it).
	MaxRequests uint64
	// MaxRequestsJitter adds a random 0..MaxRequestsJitter to MaxRequests, so
	// replicas started together do not restart in lockstep (0 disables it).
	MaxRequestsJitter uint64
	// ReadinessTimeout bounds GET /health/ready; checks still running when it
	// expires count as failed (0 uses handler.DefaultReadinessTimeout).
	ReadinessTimeout time.Duration
}

// NewHandler builds every route and wraps the mux in the middleware
// pipeline. It opens no socket, so tests can drive it with httptest.
//
// Routes use the standard library method+path patterns ("GET /health"); the matched
// pattern is available to middleware as r.Pattern after the mux has run.
func NewHandler(deps Deps, opts HandlerOptions) http.Handler {
	logger := loggerOrDiscard(deps.Logger)

	mux := http.NewServeMux()
	RegisterSwagger(mux)
	mux.Handle("GET "+RouteHealth, roothandler.New())
	mux.Handle("GET "+RouteReady, roothandler.NewReady(deps.Readiness, opts.ReadinessTimeout, logger))
	mux.Handle("GET "+RouteExample, examplehandler.New(deps.ExampleService, logger))

	return Pipeline(deps, opts, mux)
}

// Pipeline wraps next in the middleware chain. Middleware runs in
// registration order (first registered = outermost):
//
//	recover → access log → (auth) → (statsd metrics) → (prometheus metrics) → max-requests → next
func Pipeline(deps Deps, opts HandlerOptions, next http.Handler) http.Handler {
	logger := loggerOrDiscard(deps.Logger)
	accessLogger := logger
	if deps.AccessLogger != nil {
		accessLogger = deps.AccessLogger
	}

	chain := middleware.NewChain()
	chain.Register(middleware.Recover(logger))
	chain.Register(middleware.AccessLog(accessLogger))
	chain.Register(middleware.MaxRequests(opts.MaxRequests, opts.MaxRequestsJitter, deps.Shutdown, logger))
	return chain.Wrap(next)
}

func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return logger
}
