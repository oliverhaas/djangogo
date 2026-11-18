package middleware

import (
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// requestLogTimeLayout is the server time of a log line, Django's
// "%d/%b/%Y %H:%M:%S".
const requestLogTimeLayout = "02/Jan/2006 15:04:05"

// RequestLogOptions configures the RequestLog middleware.
type RequestLogOptions struct {
	// Out receives the log lines. Nil means os.Stderr, where runserver logs.
	Out io.Writer
	// Now returns the time of a log line. Nil means time.Now. Tests set it to
	// get a fixed time.
	Now func() time.Time
}

// RequestLog writes one line for every request to opts.Out, in the format of
// Django's development server:
//
//	[18/Nov/2026 12:00:00] "GET / HTTP/1.1" 200 123
//
// The fields are the time the response was finished, the request line, the
// status code, and the number of body bytes sent. The request line is escaped
// the way Django escapes it, so a request cannot forge log lines or send
// control sequences to the terminal.
//
// The line is written after the wrapped handler returned. A request whose
// handler panics is not logged, so put RequestLog outside Recover, as the
// outermost middleware. The size is the one RequestLog sees, which is the
// compressed size when GZip is inside it.
func RequestLog(opts RequestLogOptions) func(http.Handler) http.Handler {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	// One lock for all requests of this middleware: a writer such as a
	// bytes.Buffer does not allow concurrent writes, and lines must not mix.
	var mu sync.Mutex

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := newResponseWriter(w, nil)
			next.ServeHTTP(rw, r)

			line := requestLogLine(now(), r, rw.statusCode(), rw.size)
			mu.Lock()
			defer mu.Unlock()
			// A log that cannot be written must not fail the request.
			_, _ = io.WriteString(out, line)
		})
	}
}

// requestLogLine formats the log line of a request, with its newline.
func requestLogLine(now time.Time, r *http.Request, status int, size int64) string {
	target := r.RequestURI
	if target == "" && r.URL != nil {
		// A request that was not read from a connection has no RequestURI.
		target = r.URL.RequestURI()
	}

	b := make([]byte, 0, 64+len(target))
	b = append(b, '[')
	b = now.AppendFormat(b, requestLogTimeLayout)
	b = append(b, `] "`...)
	b = append(b, unicodeEscape(r.Method)...)
	b = append(b, ' ')
	b = append(b, unicodeEscape(target)...)
	b = append(b, ' ')
	b = append(b, unicodeEscape(r.Proto)...)
	b = append(b, `" `...)
	b = strconv.AppendInt(b, int64(status), 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, size, 10)
	b = append(b, '\n')
	return string(b)
}
