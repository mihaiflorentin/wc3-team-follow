package middleware

import (
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
)

// MaxRequests requests a graceful shutdown once the server has received
// limit requests, plus a random 0..jitter extra so that replicas started
// together do not all restart at the same moment. shutdown is called exactly
// once and typically cancels the server context passed to StartServer;
// requests keep being served while the server drains. limit == 0 or a nil
// shutdown disables the middleware.
func MaxRequests(limit, jitter uint64, shutdown func(), logger *slog.Logger) func(http.Handler) http.Handler {
	if limit == 0 || shutdown == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	if jitter > 0 {
		limit += rand.Uint64N(jitter + 1)
	}
	return func(next http.Handler) http.Handler {
		var counter atomic.Uint64
		var triggered atomic.Bool
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if counter.Add(1) >= limit && triggered.CompareAndSwap(false, true) {
				logger.InfoContext(r.Context(), "http.max_requests", slog.Uint64("limit", limit))
				shutdown()
			}
			next.ServeHTTP(w, r)
		})
	}
}
