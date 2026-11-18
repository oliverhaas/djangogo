package middleware

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// RecoverOptions configures the Recover middleware.
type RecoverOptions struct {
	// Debug answers a panic with the technical 500 page, which shows the panic,
	// the stack trace, and the data of the request (Django's DEBUG setting).
	// Leave it off in production: the page tells a visitor how the site works
	// inside.
	Debug bool
	// Log receives one record for every panic. Nil means os.Stderr.
	Log io.Writer
	// Handler500 answers a panic when Debug is off, the place of a project's
	// 500.html (Django's handler500). It gets the request that failed. Its
	// status 200 is sent as 500, so a handler that only renders a page is
	// enough. Nil means Django's plain "Server Error (500)" page.
	Handler500 http.Handler
	// SessionCookieName is the name of the session cookie, whose value the
	// technical 500 page never shows (Django's SESSION_COOKIE_NAME). Empty
	// means "sessionid".
	SessionCookieName string
}

// staleHeaders are the response headers that describe the body the handler
// meant to send, or tell a client how to store and present it. They are removed
// before an error page is sent in its place: the page is not a download, and a
// cache must not keep it as the answer.
var staleHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Content-Encoding",
	"Content-Range",
	"Content-Disposition",
	"Accept-Ranges",
	"ETag",
	"Last-Modified",
	"Cache-Control",
	"Expires",
}

// recoverer is the state of one Recover middleware.
type recoverer struct {
	opts RecoverOptions
	// out receives the log records.
	out io.Writer
	// mu keeps the records of concurrent requests apart.
	mu sync.Mutex
}

// panicReport describes a recovered panic.
type panicReport struct {
	// typ is the Go type of the panic value, the counterpart of the exception
	// class.
	typ string
	// value is the panic value, formatted with %v.
	value string
	// stack is the stack trace of the goroutine, from the panic down.
	stack []byte
	// file, line, and function locate the code that panicked. function is
	// empty when the place is not known.
	file     string
	line     int
	function string
}

// Recover is the exception handling of Django's request handler: it turns a
// panic of the wrapped handler into a 500 response and logs it.
//
// The log record starts with the line "Internal Server Error: " and the path of
// the request, escaped as Django escapes it. The type and the value of the
// panic and the stack trace follow.
//
// With opts.Debug the response is the technical 500 page: the type and the
// value of the panic, the method and the URL of the request, the stack trace,
// the GET and POST data, the cookies, and the request headers. Everything is
// HTML-escaped. Values whose name hints at a secret (it contains API, AUTH,
// TOKEN, KEY, SECRET, PASS, or SIGNATURE in any case) are replaced by
// asterisks, and so are the Cookie header and the session cookie. To show the
// POST data of a form the page reads the body of the request, up to 1 MiB.
//
// Without opts.Debug the response comes from opts.Handler500, and it is
// Django's plain "Server Error (500)" page when that is nil or panics as well.
//
// The error response keeps the headers that are set by then, except those that
// belong to the body the handler meant to send: its type, length, encoding,
// range, and validators, and the headers that make it a download or let a cache
// store it.
//
// A panic that happens after the response was started is only logged: the
// status is sent and cannot be taken back. A panic with http.ErrAbortHandler
// is passed on without a log record, because it is the way a handler aborts
// its response on purpose.
//
// Put Recover outside every middleware and handler whose panics it should
// catch, and inside RequestLog.
func Recover(opts RecoverOptions) func(http.Handler) http.Handler {
	rc := &recoverer{opts: opts, out: opts.Log}
	if rc.out == nil {
		rc.out = os.Stderr
	}
	if rc.opts.SessionCookieName == "" {
		rc.opts.SessionCookieName = "sessionid"
	}
	return rc.wrap
}

// wrap is the middleware function Recover returns.
func (rc *recoverer) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := newResponseWriter(w, nil)
		inner := r
		var body *bodyRecorder
		if rc.opts.Debug {
			inner, body = recordBody(r)
		}

		defer func() {
			value := recover()
			if value == nil {
				return
			}
			if isAbort(value) {
				panic(value)
			}
			report := newPanicReport(value)
			rc.log(r, "panic", report)
			if rw.started {
				return
			}
			rc.respond(rw, r, &debugReport{
				request:           inner,
				body:              body,
				panic:             report,
				sessionCookieName: rc.opts.SessionCookieName,
				now:               time.Now(),
			})
		}()

		next.ServeHTTP(rw, inner)
	})
}

// isAbort reports whether a panic value is http.ErrAbortHandler, with which a
// handler aborts its response on purpose.
func isAbort(value any) bool {
	err, ok := value.(error)
	return ok && errors.Is(err, http.ErrAbortHandler)
}

// newPanicReport describes the panic that is being recovered. Call it in the
// deferred function that recovered, or in a function called by it: only there
// does the stack still hold the frames of the panic.
func newPanicReport(value any) *panicReport {
	report := &panicReport{
		typ:   fmt.Sprintf("%T", value),
		value: fmt.Sprint(value),
		stack: trimStack(debug.Stack()),
	}

	// Walk up to the panic. The first frame below it that is not part of the
	// runtime is the code that panicked.
	pcs := make([]uintptr, 64)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	inPanic := false
	for {
		frame, more := frames.Next()
		switch {
		case frame.Function == "runtime.gopanic":
			inPanic = true
		case inPanic && !strings.HasPrefix(frame.Function, "runtime."):
			report.file, report.line, report.function = frame.File, frame.Line, frame.Function
			return report
		}
		if !more {
			return report
		}
	}
}

// trimStack removes the frames of the recovery itself from a stack trace of
// debug.Stack, so that the trace starts at the panic.
func trimStack(stack []byte) []byte {
	header, frames, ok := bytes.Cut(stack, []byte("\n"))
	if !ok {
		return stack
	}
	i := bytes.Index(frames, []byte("\npanic("))
	if i < 0 {
		return stack
	}
	trimmed := make([]byte, 0, len(stack))
	trimmed = append(trimmed, header...)
	return append(trimmed, frames[i:]...)
}

// log writes the record of a panic in one piece. what names the panic on the
// second line of the record.
func (rc *recoverer) log(r *http.Request, what string, report *panicReport) {
	path := ""
	if r.URL != nil {
		path = r.URL.Path
	}

	var b bytes.Buffer
	b.WriteString("Internal Server Error: ")
	// The path is under the control of the client. Escaped, it cannot forge
	// log lines or send control sequences to a terminal.
	b.WriteString(unicodeEscape(path))
	b.WriteString("\n" + what + ": " + report.typ + ": " + report.value + "\n\n")
	b.Write(report.stack)
	if !bytes.HasSuffix(report.stack, []byte("\n")) {
		b.WriteByte('\n')
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()
	// A log that cannot be written must not fail the request a second time.
	_, _ = rc.out.Write(b.Bytes())
}

// respond answers a request whose handler panicked before it started the
// response.
func (rc *recoverer) respond(w http.ResponseWriter, r *http.Request, report *debugReport) {
	clearStaleHeaders(w.Header())

	if rc.opts.Debug {
		page := renderDebugPage(report)
		w.Header().Set("Content-Type", htmlContentType)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, page)
		return
	}

	if rc.opts.Handler500 != nil {
		if rc.serveHandler500(w, r) {
			return
		}
		clearStaleHeaders(w.Header())
	}
	writeErrorPage(w, http.StatusInternalServerError, "Server Error (500)", "")
}

// clearStaleHeaders removes the headers of the response that was not sent.
func clearStaleHeaders(h http.Header) {
	for _, name := range staleHeaders {
		h.Del(name)
	}
}

// serveHandler500 lets Handler500 answer. It reports whether the response was
// started. That is not the case when the handler panicked before it wrote
// anything, and then the plain page can still be sent.
func (rc *recoverer) serveHandler500(w http.ResponseWriter, r *http.Request) (started bool) {
	hw := newResponseWriter(w, nil)
	hw.okStatus = http.StatusInternalServerError

	defer func() {
		value := recover()
		if value == nil {
			return
		}
		if isAbort(value) {
			panic(value)
		}
		rc.log(r, "panic in Handler500", newPanicReport(value))
		started = hw.started
	}()

	rc.opts.Handler500.ServeHTTP(hw, r)
	if !hw.started {
		// A handler that wrote nothing still answers a failed request.
		hw.WriteHeader(http.StatusInternalServerError)
	}
	return true
}
