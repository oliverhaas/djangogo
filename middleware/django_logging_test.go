package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports from Django's tests/logging_tests/tests.py.
// Django logs to the loggers django.request and django.server; here Recover
// writes to RecoverOptions.Log and RequestLog writes to RequestLogOptions.Out.

// uncaughtException is the port of views.UncaughtException.
type uncaughtException struct{ msg string }

// Error makes uncaughtException an error.
func (e uncaughtException) Error() string { return e.msg }

// firstLine returns the first line of what was logged, which is the message of
// the record in Django's terms.
func firstLine(log *bytes.Buffer) string {
	line, _, _ := strings.Cut(log.String(), "\n")
	return line
}

// TestUncaughtException checks the log record of a panic: its message, the
// status of the response, and the type of the panic value in the place of the
// exception class.
//
// Ported from Django: tests/logging_tests/tests.py::HandlerLoggingTests.test_uncaught_exception
func TestUncaughtException(t *testing.T) {
	var log bytes.Buffer
	view := raises500(uncaughtException{"Uncaught exception"})
	handler := middleware.Recover(middleware.RecoverOptions{Log: &log})(view)

	resp := djangotest.NewClient(t, handler).Get("/uncaught_exception/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, firstLine(&log), "Internal Server Error: /uncaught_exception/")
	djangotest.AssertEqual(t, strings.Count(log.String(), "Internal Server Error: "), 1)
	for _, want := range []string{"middleware_test.uncaughtException", "Uncaught exception", "goroutine "} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("the log lacks %q:\n%s", want, log.String())
		}
	}
}

// TestControlCharsEscaped checks that control characters of the path are
// escaped in the log message. Django's test requests a path that does not
// exist and reads the "Not Found" message. The 404 is not answered by this
// package, so the path panics and the message is that of the 500.
//
// Ported from Django: tests/logging_tests/tests.py::HandlerLoggingTests.test_control_chars_escaped
func TestControlCharsEscaped(t *testing.T) {
	var log bytes.Buffer
	handler := middleware.Recover(middleware.RecoverOptions{Log: &log})(raises500("boom"))

	resp := djangotest.NewClient(t, handler).Get("/%1B[1;31mNOW IN RED!!!1B[0m/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, firstLine(&log), `Internal Server Error: /\x1b[1;31mNOW IN RED!!!1B[0m/`)
}

// TestUnicodeEscapeEscaping checks how the argument of a log message is
// escaped. Django passes the cases to log_response as the argument of a test
// message; here each case is the path of a request that panics, which is the
// one argument of the message Recover logs. The cases that are not strings have
// no counterpart.
//
// Ported from Django: tests/logging_tests/tests.py::LogResponseRealLoggerTests.test_unicode_escape_escaping
func TestUnicodeEscapeEscaping(t *testing.T) {
	testCases := []struct{ input, expected string }{
		// Control characters.
		{"line\nbreak", `line\nbreak`},
		{"carriage\rreturn", `carriage\rreturn`},
		{"tab\tseparated", `tab\tseparated`},
		{"formfeed\f", `formfeed\x0c`},
		{"bell\a", `bell\x07`},
		{"multi\nline\ntext", `multi\nline\ntext`},
		// Slashes.
		{`slash\test`, `slash\\test`},
		{`back\slash`, `back\\slash`},
		// Quotes.
		{`quote"test"`, `quote"test"`},
		{"quote'test'", "quote'test'"},
		// Accented, composed characters, emojis and symbols.
		{"café", `caf\xe9`},
		{"e\u0301", `e\u0301`}, // e + combining acute
		{"smile🙂", `smile\U0001f642`},
		{"weird ☃️", `weird \u2603\ufe0f`},
		// Non-Latin alphabets.
		{"Привет", `\u041f\u0440\u0438\u0432\u0435\u0442`},
		{"你好", `\u4f60\u597d`},
		// ANSI escape sequences.
		{"escape\x1b[31mred\x1b[0m", `escape\x1b[31mred\x1b[0m`},
		{"/\x1b[1;31mCAUTION!!YOU ARE PWNED\x1b[0m/", `/\x1b[1;31mCAUTION!!YOU ARE PWNED\x1b[0m/`},
		{
			"/\r\n\r\n1984-04-22 INFO    Listening on 0.0.0.0:8080\r\n\r\n",
			`/\r\n\r\n1984-04-22 INFO    Listening on 0.0.0.0:8080\r\n\r\n`,
		},
		// Plain safe input.
		{"normal-path", "normal-path"},
		{"slash/colon:", "slash/colon:"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			var log bytes.Buffer
			handler := middleware.Recover(middleware.RecoverOptions{Log: &log})(raises500("boom"))
			req := &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{Path: tc.input},
				Host:   "testserver",
				Header: make(http.Header),
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			djangotest.AssertEqual(t, rec.Code, http.StatusInternalServerError)
			djangotest.AssertEqual(t, firstLine(&log), "Internal Server Error: "+tc.expected)
		})
	}
}

// TestServerFormatterDefaultFormat checks the frame of a line of the request
// log: the server time in brackets, the message, and a newline. With a fixed
// clock the whole line is compared, and with the real clock the time is matched
// by the pattern of the Django test.
//
// Ported from Django: tests/logging_tests/tests.py::LogFormattersTests.test_server_formatter_default_format
func TestServerFormatterDefaultFormat(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	const logMsg = `"GET / HTTP/1.1" 200 2`

	var out bytes.Buffer
	serverTime := time.Date(2016, time.September, 25, 10, 20, 30, 0, time.UTC)
	handler := middleware.RequestLog(middleware.RequestLogOptions{
		Out: &out,
		Now: func() time.Time { return serverTime },
	})(view)
	djangotest.NewClient(t, handler).Get("/")
	djangotest.AssertEqual(t, out.String(), "[25/Sep/2016 10:20:30] "+logMsg+"\n")

	out.Reset()
	handler = middleware.RequestLog(middleware.RequestLogOptions{Out: &out})(view)
	djangotest.NewClient(t, handler).Get("/")
	pattern := regexp.MustCompile(`^\[[/:,\w\s\d]+\] ` + regexp.QuoteMeta(logMsg) + `\n$`)
	if !pattern.MatchString(out.String()) {
		t.Errorf("log line %q does not match %s", out.String(), pattern)
	}
}
