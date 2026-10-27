package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto/response"
)

// DefaultReadinessTimeout bounds a readiness request when no timeout is set.
const DefaultReadinessTimeout = 2 * time.Second

// Ready serves the readiness probe: it runs every registered check in
// parallel and answers 200, or 503 naming the checks that failed or did not
// finish within the timeout. Liveness (GET /health) never runs checks.
type Ready struct {
	checks  contract.ReadinessChecks
	timeout time.Duration
	logger  *slog.Logger
}

// NewReady returns the readiness handler. A nil checks list means no
// dependencies (always ready); a timeout <= 0 uses DefaultReadinessTimeout.
func NewReady(checks contract.ReadinessChecks, timeout time.Duration, logger *slog.Logger) Ready {
	if timeout <= 0 {
		timeout = DefaultReadinessTimeout
	}
	return Ready{checks: checks, timeout: timeout, logger: logger}
}

func (h Ready) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	failed := h.failedChecks(r.Context())

	body := response.Readiness{Status: "ok"}
	code := http.StatusOK
	if len(failed) > 0 {
		body = response.Readiness{Status: "unavailable", Failed: failed}
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// failedChecks returns the sorted names of the checks that failed or were
// still running when the timeout expired.
func (h Ready) failedChecks(ctx context.Context) []string {
	if h.checks == nil {
		return nil
	}
	checks := h.checks.ReadinessChecks()
	if len(checks) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(checks)) // buffered: late checks never block
	for _, check := range checks {
		go func() {
			results <- result{name: check.Name(), err: check.Check(ctx)}
		}()
	}

	pending := make(map[string]bool, len(checks))
	for _, check := range checks {
		pending[check.Name()] = true
	}
	var failed []string
collect:
	for range checks {
		select {
		case res := <-results:
			delete(pending, res.name)
			if res.err != nil {
				h.logger.WarnContext(ctx, "readiness.check_failed", slog.String("check", res.name), slog.Any("error", res.err))
				failed = append(failed, res.name)
			}
		case <-ctx.Done():
			for name := range pending {
				h.logger.WarnContext(ctx, "readiness.check_timeout", slog.String("check", name), slog.Duration("timeout", h.timeout))
				failed = append(failed, name)
			}
			break collect
		}
	}
	slices.Sort(failed)
	return failed
}
