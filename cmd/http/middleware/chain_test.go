package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/cmd/http/middleware"
)

func TestChain_FirstRegisteredRunsOutermost(t *testing.T) {
	var order []string
	trace := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+":in")
				next.ServeHTTP(w, r)
				order = append(order, name+":out")
			})
		}
	}

	chain := middleware.NewChain()
	chain.Register(trace("first"))
	chain.Register(trace("second"))
	handler := chain.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"first:in", "second:in", "handler", "second:out", "first:out"}
	if !slices.Equal(order, want) {
		t.Errorf("execution order = %v, want %v", order, want)
	}
}
