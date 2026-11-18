package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/middleware"
)

// The test in this file is a port from Django's tests/servers/test_basehttp.py,
// which tests the request handler of runserver. RequestLog writes the same log
// line. The other tests there are about WSGI and sockets.

// TestLogMessageEscapesControlSequences checks that control sequences in the
// request line are escaped, so a request cannot color or rewrite the terminal
// of the developer. Django calls log_message with a malicious path. Here the
// request is built by hand, because a client refuses to send such a target.
//
// Ported from Django: tests/servers/test_basehttp.py::WSGIRequestHandlerTestCase.test_log_message_escapes_control_sequences
func TestLogMessageEscapesControlSequences(t *testing.T) {
	const maliciousPath = "\x1b[31mALERT\x1b[0m"

	var out bytes.Buffer
	handler := middleware.RequestLog(middleware.RequestLogOptions{Out: &out})(http.NotFoundHandler())
	req := &http.Request{
		Method:     http.MethodGet,
		URL:        &url.URL{Path: maliciousPath},
		RequestURI: maliciousPath,
		Proto:      "HTTP/1.1",
		Host:       "testserver",
		Header:     make(http.Header),
	}
	handler.ServeHTTP(httptest.NewRecorder(), req)

	log := out.String()
	if strings.Contains(log, "\x1b[31m") {
		t.Errorf("the log holds a raw control sequence: %q", log)
	}
	if !strings.Contains(log, `\x1b[31mALERT\x1b[0m`) {
		t.Errorf("the log lacks the escaped path: %q", log)
	}
	if !strings.Contains(log, " 404 ") {
		t.Errorf("the log lacks the status 404: %q", log)
	}
}
