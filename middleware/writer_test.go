package middleware_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are about the response writers the middleware hands
// to the wrapped handler: whatever the writer of the server can do must still
// work through them.

// deadlineWriter is a response writer with a feature the middleware of this
// package does not know about: a write deadline, as the writer of net/http has
// one.
type deadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

// SetWriteDeadline records the deadline. http.ResponseController reaches the
// method through the Unwrap methods of the writers in between.
func (w *deadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

// serverRequest builds a GET for a test server that the chain of fullChain
// accepts: the server listens on a local address, and the chain allows the
// host of the test client only.
func serverRequest(ctx context.Context, t *testing.T, srv *httptest.Server) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "testserver"
	return req
}

// TestWritersKeepServerFeatures checks, for every middleware and for all of
// them in a chain, that the writer the handler gets is an http.Flusher and
// lets http.ResponseController through to the writer of the server.
func TestWritersKeepServerFeatures(t *testing.T) {
	deadline := time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Errorf("%T is not an http.Flusher", w)
		}
		if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil {
			t.Errorf("SetWriteDeadline through %T: %v", w, err)
		}
	})

	for name, handler := range map[string]http.Handler{
		"RequestLog":    middleware.RequestLog(middleware.RequestLogOptions{Out: io.Discard})(view),
		"Recover":       middleware.Recover(middleware.RecoverOptions{Log: io.Discard})(view),
		"Recover debug": technical500(view),
		"AllowedHosts":  middleware.AllowedHosts(middleware.AllowedHostsOptions{AllowedHosts: []string{"*"}})(view),
		"Security":      mustSecurity(t, middleware.SecurityOptions{ContentTypeNosniff: true})(view),
		"GZip":          middleware.GZip()(view),
		"XFrameOptions": middleware.XFrameOptions("DENY")(view),
		"all of them":   fullChain(t, view),
	} {
		t.Run(name, func(t *testing.T) {
			w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://testserver/", nil))
			djangotest.AssertEqual(t, w.Code, http.StatusOK)
			djangotest.AssertEqual(t, w.deadline, deadline)
		})
	}
}

// TestFlushReachesClient checks on a real connection that a flush delivers
// what was written so far through the whole chain, the compression included:
// the client reads the first part of the body while the handler still waits.
func TestFlushReachesClient(t *testing.T) {
	release := make(chan struct{})
	view := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		// Wait for the client to confirm the first part. A client that gave up
		// ends the wait as well.
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, "second")
	})
	srv := httptest.NewServer(fullChain(t, view))
	defer srv.Close()

	// A flush that does not arrive blocks the client. The deadline turns that
	// into a failure.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	req := serverRequest(ctx, t, srv)
	// An Accept-Encoding of its own keeps the client from decompressing behind
	// the back of the test.
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	djangotest.AssertEqual(t, resp.Header.Get("Content-Encoding"), "gzip")
	djangotest.AssertEqual(t, resp.Header.Get("X-Frame-Options"), "DENY")

	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("the response is not a gzip stream: %v", err)
	}
	first := make([]byte, len("first"))
	if _, err := io.ReadFull(zr, first); err != nil {
		t.Fatalf("read the flushed part: %v", err)
	}
	djangotest.AssertEqual(t, string(first), "first")

	close(release)
	rest, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read the rest: %v", err)
	}
	djangotest.AssertEqual(t, string(rest), "second")
}

// TestHijackThroughChain checks on a real connection that a handler can take
// over the connection through the whole chain, as a WebSocket upgrade does. The
// request log records the takeover as 101 Switching Protocols.
func TestHijackThroughChain(t *testing.T) {
	const raw = "HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\nhijacked"
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		_, _ = buf.WriteString(raw)
		_ = buf.Flush()
	})

	var log bytes.Buffer
	logged := make(chan struct{})
	chain := middleware.Chain(
		middleware.RequestLog(middleware.RequestLogOptions{Out: &log}),
		middleware.Recover(middleware.RecoverOptions{Log: io.Discard}),
		mustSecurity(t, middleware.SecurityOptions{ContentTypeNosniff: true}),
		middleware.GZip(),
		middleware.XFrameOptions("DENY"),
	)(view)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server does not wait for the handler of a hijacked connection,
		// so the test does.
		defer close(logged)
		chain.ServeHTTP(w, r)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	resp, err := srv.Client().Do(serverRequest(ctx, t, srv))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the response: %v", err)
	}
	djangotest.AssertEqual(t, string(body), "hijacked")

	<-logged
	if want := `"GET / HTTP/1.1" 101 0` + "\n"; !strings.HasSuffix(log.String(), want) {
		t.Errorf("log line %q does not end in %q", log.String(), want)
	}
}
