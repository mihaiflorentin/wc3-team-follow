package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpserver "github.com/mihaiflorentin/wc3-team-follow/cmd/http"
	mockexample "github.com/mihaiflorentin/wc3-team-follow/mock/example"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto/response"
)

// fakeCheck is a readiness check that returns err, or blocks until ctx is
// done when hang is set.
type fakeCheck struct {
	name string
	err  error
	hang bool
}

func (f fakeCheck) Name() string { return f.name }

func (f fakeCheck) Check(ctx context.Context) error {
	if f.hang {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

// fakeChecks lists the checks it holds, like the container does.
type fakeChecks []contract.ReadinessCheck

func (f fakeChecks) ReadinessChecks() []contract.ReadinessCheck { return f }

func serveReady(t *testing.T, checks fakeChecks, opts httpserver.HandlerOptions) (int, response.Readiness) {
	t.Helper()
	handler := httpserver.NewHandler(httpserver.Deps{
		Logger:         discardLogger(),
		ExampleService: &mockexample.Service{},
		Readiness:      checks,
	}, opts)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpserver.RouteReady, nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body response.Readiness
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

func TestNewHandler_ReadyIs200WhenEveryCheckPasses(t *testing.T) {
	code, body := serveReady(t, fakeChecks{fakeCheck{name: "mysql"}, fakeCheck{name: "redis"}}, httpserver.HandlerOptions{})

	if code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", httpserver.RouteReady, code)
	}
	if body.Status != "ok" || len(body.Failed) != 0 {
		t.Errorf("body = %+v, want status ok and no failed checks", body)
	}
}

func TestNewHandler_ReadyIs200WithoutChecks(t *testing.T) {
	code, _ := serveReady(t, nil, httpserver.HandlerOptions{})

	if code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", httpserver.RouteReady, code)
	}
}

func TestNewHandler_ReadyIs503NamingTheFailingCheck(t *testing.T) {
	checks := fakeChecks{
		fakeCheck{name: "mysql"},
		fakeCheck{name: "redis", err: errors.New("connection refused")},
	}
	code, body := serveReady(t, checks, httpserver.HandlerOptions{})

	if code != http.StatusServiceUnavailable {
		t.Fatalf("GET %s = %d, want 503", httpserver.RouteReady, code)
	}
	if body.Status != "unavailable" {
		t.Errorf("status = %q, want unavailable", body.Status)
	}
	if len(body.Failed) != 1 || body.Failed[0] != "redis" {
		t.Errorf("failed = %v, want [redis]", body.Failed)
	}
}

func TestNewHandler_ReadyTimesOutAHangingCheck(t *testing.T) {
	checks := fakeChecks{fakeCheck{name: "mysql"}, fakeCheck{name: "broker", hang: true}}

	start := time.Now()
	code, body := serveReady(t, checks, httpserver.HandlerOptions{ReadinessTimeout: 50 * time.Millisecond})
	elapsed := time.Since(start)

	if code != http.StatusServiceUnavailable {
		t.Fatalf("GET %s = %d, want 503", httpserver.RouteReady, code)
	}
	if len(body.Failed) != 1 || body.Failed[0] != "broker" {
		t.Errorf("failed = %v, want [broker]", body.Failed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("readiness took %s, want it bounded by the 50ms timeout", elapsed)
	}
}

func TestNewHandler_HealthStaysLiveWhileNotReady(t *testing.T) {
	handler := httpserver.NewHandler(httpserver.Deps{
		Logger:         discardLogger(),
		ExampleService: &mockexample.Service{},
		Readiness:      fakeChecks{fakeCheck{name: "mysql", err: errors.New("down")}},
	}, httpserver.HandlerOptions{})

	if code := serveHealth(handler); code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200 while a readiness check fails", httpserver.RouteHealth, code)
	}
}
