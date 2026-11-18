package middleware_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The test in this file is a port from Django's tests/handlers/tests.py. Most
// tests there are about WSGI, ASGI, transactions, or signals and have no
// counterpart in a net/http middleware package.

// fullChain wraps view in every middleware of the package, in the order the
// package documentation recommends, with all logs discarded.
func fullChain(t *testing.T, view http.Handler) http.Handler {
	t.Helper()
	security, err := middleware.Security(middleware.SecurityOptions{
		ContentTypeNosniff:      true,
		ReferrerPolicy:          "same-origin",
		CrossOriginOpenerPolicy: "same-origin",
	})
	if err != nil {
		t.Fatal(err)
	}
	return middleware.Chain(
		middleware.RequestLog(middleware.RequestLogOptions{Out: io.Discard}),
		middleware.Recover(middleware.RecoverOptions{Log: io.Discard}),
		middleware.AllowedHosts(middleware.AllowedHostsOptions{AllowedHosts: []string{"testserver"}}),
		security,
		middleware.GZip(),
		middleware.XFrameOptions("DENY"),
	)(view)
}

// TestStreaming checks that a streaming response passes the whole middleware
// chain: the view is the port of the streaming view, with a flush after every
// chunk.
//
// Ported from Django: tests/handlers/tests.py::HandlerRequestTests.test_streaming
func TestStreaming(t *testing.T) {
	streaming := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for _, chunk := range []string{"streaming", " ", "content"} {
			_, _ = io.WriteString(w, chunk)
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("flush: %v", err)
			}
		}
	})

	resp := djangotest.NewClient(t, fullChain(t, streaming)).Get("/streaming/")
	djangotest.AssertStatus(t, resp, http.StatusOK)
	djangotest.AssertEqual(t, resp.Content(), "streaming content")
}
