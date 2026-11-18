package middleware_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// tracer returns a middleware that records in calls when it is entered and
// when it is left.
func tracer(calls *[]string, name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*calls = append(*calls, name+" in")
			next.ServeHTTP(w, r)
			*calls = append(*calls, name+" out")
		})
	}
}

// TestChainOrder checks that the first argument of Chain is the outermost
// middleware and that a nil entry is skipped.
func TestChainOrder(t *testing.T) {
	var calls []string
	view := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls = append(calls, "view")
	})

	handler := middleware.Chain(tracer(&calls, "a"), nil, tracer(&calls, "b"))(view)
	djangotest.NewClient(t, handler).Get("/")
	djangotest.AssertEqual(t, calls, []string{"a in", "b in", "view", "b out", "a out"})
}

// TestChainKeepsItsList checks that a change to the slice that was spread into
// Chain does not reach the chain.
func TestChainKeepsItsList(t *testing.T) {
	var calls []string
	mws := []func(http.Handler) http.Handler{tracer(&calls, "a")}
	chain := middleware.Chain(mws...)
	mws[0] = tracer(&calls, "b")

	djangotest.NewClient(t, chain(emptyView)).Get("/")
	djangotest.AssertEqual(t, calls, []string{"a in", "a out"})
}

// TestChainEmpty checks that a chain without middleware serves the handler
// itself.
func TestChainEmpty(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	resp := djangotest.NewClient(t, middleware.Chain()(view)).Get("/")
	djangotest.AssertStatus(t, resp, http.StatusTeapot)
}
