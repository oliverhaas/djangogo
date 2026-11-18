package middleware

import (
	"bufio"
	"net"
	"net/http"
)

// responseWriter wraps an http.ResponseWriter and records what the handler
// sent: the status code, the number of body bytes, and whether the response has
// been started. It can also run a hook right before the response header is
// sent, which is the last moment a header can still be added.
//
// It implements http.Flusher and http.Hijacker by delegation, and Unwrap gives
// http.ResponseController access to everything else the wrapped writer can do.
type responseWriter struct {
	http.ResponseWriter
	// beforeHeader runs once, right before the header is sent. It may be nil.
	beforeHeader func()
	// okStatus, when it is not zero, is the status sent in place of a 200,
	// whether the handler asked for the 200 or got it by writing.
	okStatus int
	// hookRan guards beforeHeader against a second run.
	hookRan bool
	// started is set once the header is sent or the connection is hijacked.
	// From then on the status can no longer be changed.
	started bool
	// status is the status code sent, or 0 while nothing is sent.
	status int
	// size is the number of body bytes written.
	size int64
}

// newResponseWriter wraps w. beforeHeader may be nil.
func newResponseWriter(w http.ResponseWriter, beforeHeader func()) *responseWriter {
	return &responseWriter{ResponseWriter: w, beforeHeader: beforeHeader}
}

// statusCode returns the status code of the response. A handler that has not
// written anything gets the 200 that net/http sends for it.
func (w *responseWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// runBeforeHeader runs the hook unless it ran already. Call it as well after
// the handler returned, for the handlers that never write: net/http sends
// their header later, with whatever the header map holds by then.
func (w *responseWriter) runBeforeHeader() {
	if w.hookRan {
		return
	}
	w.hookRan = true
	if w.beforeHeader != nil {
		w.beforeHeader()
	}
}

// start marks the response as started with the given status.
func (w *responseWriter) start(status int) {
	w.started = true
	w.status = status
}

// WriteHeader records the status code and sends the header. An informational
// status (1xx other than 101) is passed on without being recorded, because the
// final header is still to come.
func (w *responseWriter) WriteHeader(code int) {
	informational := code >= 100 && code < 200 && code != http.StatusSwitchingProtocols
	if !w.started && !informational {
		if code == http.StatusOK && w.okStatus != 0 {
			code = w.okStatus
		}
		w.runBeforeHeader()
		w.start(code)
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write sends body bytes and counts them. The first write sends the header
// with status 200 when WriteHeader was not called.
func (w *responseWriter) Write(p []byte) (int, error) {
	switch {
	case w.started:
	case w.okStatus != 0:
		// The write would send a 200. Send the replacement first.
		w.WriteHeader(http.StatusOK)
	default:
		// Leave the header to the wrapped writer. It sends it with this write
		// and can guess a missing Content-Type from p.
		w.runBeforeHeader()
		w.start(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.size += int64(n)
	return n, err
}

// Flush implements http.Flusher. It does nothing when the wrapped writer
// cannot flush.
func (w *responseWriter) Flush() {
	_ = w.FlushError()
}

// FlushError flushes the wrapped writer and reports whether that worked. It is
// the method http.ResponseController calls. A flush sends the header, so the
// hook runs first.
func (w *responseWriter) FlushError() error {
	w.runBeforeHeader()
	if !w.started && w.okStatus != 0 {
		// The flush would send a 200. Send the replacement first.
		w.WriteHeader(http.StatusOK)
	}
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err == nil && !w.started {
		w.start(http.StatusOK)
	}
	return err
}

// Hijack implements http.Hijacker by delegation. A hijacked connection counts
// as a started response, since nothing can be sent through the writer any
// more. When no status was sent it is recorded as 101 Switching Protocols,
// which is what a handler takes over a connection for.
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && !w.started {
		w.start(http.StatusSwitchingProtocols)
	}
	return conn, rw, err
}

// Unwrap returns the wrapped writer, so that http.ResponseController reaches
// the features this type does not delegate itself.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
