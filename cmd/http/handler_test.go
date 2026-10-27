package http_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	httpserver "github.com/mihaiflorentin/wc3-team-follow/cmd/http"
	mockexample "github.com/mihaiflorentin/wc3-team-follow/mock/example"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto/response"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newTestHandler(svc *mockexample.Service) http.Handler {
	return httpserver.NewHandler(httpserver.Deps{
		Logger:         discardLogger(),
		ExampleService: svc,
	}, httpserver.HandlerOptions{})
}

func TestNewHandler_ExampleMapsServiceStatusToResponseDTO(t *testing.T) {
	svc := &mockexample.Service{PingStatus: dto.ExampleStatus{Examples: 7}}

	rec := httptest.NewRecorder()
	newTestHandler(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpserver.RouteExample, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", httpserver.RouteExample, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body response.Example
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	if want := (response.Example{Message: "pong", Examples: 7}); body != want {
		t.Errorf("body = %+v, want %+v", body, want)
	}
	if svc.PingCalls != 1 {
		t.Errorf("service called %d times, want 1", svc.PingCalls)
	}
}

func TestNewHandler_ExampleServiceErrorIs500(t *testing.T) {
	svc := &mockexample.Service{PingErr: errors.New("boom")}

	rec := httptest.NewRecorder()
	newTestHandler(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpserver.RouteExample, nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET %s = %d, want 500", httpserver.RouteExample, rec.Code)
	}
}

func TestNewHandler_HealthReturnsJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(&mockexample.Service{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpserver.RouteHealth, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", httpserver.RouteHealth, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body response.Health
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestNewHandler_HealthRejectsOtherMethods(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(&mockexample.Service{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, httpserver.RouteHealth, nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST %s = %d, want 405", httpserver.RouteHealth, rec.Code)
	}
}

func TestNewHandler_ServesSwaggerJSONAtDocsRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(&mockexample.Service{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpserver.SwaggerJSONRoute, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", httpserver.SwaggerJSONRoute, rec.Code)
	}
	var spec struct {
		Swagger string                     `json:"swagger"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("swagger spec is not JSON: %v\n%s", err, rec.Body.String())
	}
	if spec.Swagger != "2.0" {
		t.Errorf("swagger version = %q, want 2.0", spec.Swagger)
	}
	for _, route := range []string{httpserver.RouteHealth, httpserver.RouteReady, httpserver.RouteExample} {
		if _, ok := spec.Paths[route]; !ok {
			t.Errorf("swagger spec does not document %s", route)
		}
	}
}

func TestNewHandler_DocsUIIsServed(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(&mockexample.Service{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs/index.html", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /docs/index.html = %d, want 200", rec.Code)
	}
}

// newMaxRequestsHandler returns a handler limited by opts and a counter of
// the shutdown requests it made.
func newMaxRequestsHandler(opts httpserver.HandlerOptions) (http.Handler, *atomic.Int32) {
	var shutdowns atomic.Int32
	handler := httpserver.NewHandler(httpserver.Deps{
		Logger:         discardLogger(),
		ExampleService: &mockexample.Service{},
		Shutdown:       func() { shutdowns.Add(1) },
	}, opts)
	return handler, &shutdowns
}

func serveHealth(handler http.Handler) int {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, httpserver.RouteHealth, nil))
	return rec.Code
}

// requestsUntilShutdown serves requests one at a time and returns the number
// of the request that triggered the shutdown (0 if none did within limit).
func requestsUntilShutdown(handler http.Handler, shutdowns *atomic.Int32, limit int) int {
	for i := 1; i <= limit; i++ {
		serveHealth(handler)
		if shutdowns.Load() > 0 {
			return i
		}
	}
	return 0
}

func TestNewHandler_MaxRequestsWithoutJitterShutsDownOnTheNthRequest(t *testing.T) {
	handler, shutdowns := newMaxRequestsHandler(httpserver.HandlerOptions{MaxRequests: 3})

	if got := requestsUntilShutdown(handler, shutdowns, 10); got != 3 {
		t.Fatalf("shutdown triggered by request %d, want 3", got)
	}
}

func TestNewHandler_MaxRequestsTriggersShutdownOnce(t *testing.T) {
	handler, shutdowns := newMaxRequestsHandler(httpserver.HandlerOptions{MaxRequests: 3})
	for i := 1; i <= 3; i++ {
		serveHealth(handler)
	}

	// Requests past the limit are still served (graceful shutdown), but
	// shutdown is requested only once, even under concurrency.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code := serveHealth(handler); code != http.StatusOK {
				t.Errorf("request past the limit = %d, want 200", code)
			}
		}()
	}
	wg.Wait()
	if got := shutdowns.Load(); got != 1 {
		t.Fatalf("shutdown triggered %d times, want exactly 1", got)
	}
}

func TestNewHandler_MaxRequestsJitterKeepsTheLimitInRange(t *testing.T) {
	const limit, jitter = 5, 3
	seen := map[int]bool{}
	for run := 0; run < 200; run++ {
		handler, shutdowns := newMaxRequestsHandler(httpserver.HandlerOptions{MaxRequests: limit, MaxRequestsJitter: jitter})

		got := requestsUntilShutdown(handler, shutdowns, 20)
		if got < limit || got > limit+jitter {
			t.Fatalf("shutdown triggered by request %d, want between %d and %d", got, limit, limit+jitter)
		}
		seen[got] = true
	}
	if len(seen) < 2 {
		t.Errorf("200 handlers all shut down on the same request (%v); the jitter is not applied", seen)
	}
}

func TestNewHandler_ZeroMaxRequestsNeverShutsDown(t *testing.T) {
	handler, shutdowns := newMaxRequestsHandler(httpserver.HandlerOptions{MaxRequestsJitter: 5})

	if got := requestsUntilShutdown(handler, shutdowns, 10); got != 0 {
		t.Fatalf("shutdown triggered by request %d with the limit disabled", got)
	}
}

// This test ships in every generated project, so with the UI pack enabled it
// also proves the UI root redirect does not swallow unknown paths.
func TestNewHandler_UnknownPathIs404(t *testing.T) {
	for _, path := range []string{"/nope", "/health/extra", "/ui/nope"} {
		rec := httptest.NewRecorder()
		newTestHandler(&mockexample.Service{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}
