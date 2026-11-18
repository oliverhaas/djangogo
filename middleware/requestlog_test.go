package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// logTime is the fixed server time of the log lines in these tests.
var logTime = time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)

// requestLog returns the RequestLog middleware that writes to out with logTime
// as its clock.
func requestLog(out io.Writer) func(http.Handler) http.Handler {
	return middleware.RequestLog(middleware.RequestLogOptions{
		Out: out,
		Now: func() time.Time { return logTime },
	})
}

// TestRequestLogStatusAndSize checks that the line carries the status the
// handler chose and the size of the whole body, written in several pieces.
func TestRequestLogStatusAndSize(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "not ")
		_, _ = io.WriteString(w, "found")
	})

	var out bytes.Buffer
	resp := djangotest.NewClient(t, requestLog(&out)(view)).Get("/missing/?page=2")
	djangotest.AssertStatus(t, resp, http.StatusNotFound)
	djangotest.AssertEqual(t, out.String(), `[18/Nov/2026 12:00:00] "GET /missing/?page=2 HTTP/1.1" 404 9`+"\n")
}

// TestRequestLogSilentHandler checks the line of a handler that never touches
// the response writer: net/http answers 200 without a body for it. The request
// line keeps the method and the query as they were sent.
func TestRequestLogSilentHandler(t *testing.T) {
	silent := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	var out bytes.Buffer
	djangotest.NewClient(t, requestLog(&out)(silent)).Post("/form/?next=%2Fhome", url.Values{"a": {"b"}})
	djangotest.AssertEqual(t, out.String(), `[18/Nov/2026 12:00:00] "POST /form/?next=%2Fhome HTTP/1.1" 200 0`+"\n")
}

// TestRequestLogFlushedResponse checks that a response the handler starts with
// a flush is logged with the status the flush sent.
func TestRequestLogFlushedResponse(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		_, _ = io.WriteString(w, "late")
	})

	var out bytes.Buffer
	rec := httptest.NewRecorder()
	requestLog(&out)(view).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	djangotest.AssertEqual(t, rec.Flushed, true)
	djangotest.AssertEqual(t, out.String(), `[18/Nov/2026 12:00:00] "GET / HTTP/1.1" 200 4`+"\n")
}

// TestRequestLogPanic checks the two sides of the ordering rule. A panic that
// passes RequestLog leaves no line, so with RequestLog inside Recover a failed
// request is missing from the log. With Recover inside, the line shows the 500
// and the size of the error page.
func TestRequestLogPanic(t *testing.T) {
	var out bytes.Buffer
	recovered := middleware.Recover(middleware.RecoverOptions{Log: io.Discard})

	inside := middleware.Chain(recovered, requestLog(&out))(raises500("boom"))
	resp := djangotest.NewClient(t, inside).Get("/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, out.String(), "")

	outside := middleware.Chain(requestLog(&out), recovered)(raises500("boom"))
	resp = djangotest.NewClient(t, outside).Get("/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	size := strconv.Itoa(len(resp.Body))
	djangotest.AssertEqual(t, out.String(), `[18/Nov/2026 12:00:00] "GET / HTTP/1.1" 500 `+size+"\n")
	if len(resp.Body) == 0 {
		t.Error("the error page is empty, so the size in the log line proves nothing")
	}
}

// TestRequestLogConcurrentRequests checks that requests served at the same
// time each get a whole line, into a writer that allows no concurrent writes.
func TestRequestLogConcurrentRequests(t *testing.T) {
	const workers, perWorker = 8, 25
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	var out bytes.Buffer
	handler := requestLog(&out)(view)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range perWorker {
				handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/some/path/", nil))
			}
		})
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	djangotest.AssertEqual(t, len(lines), workers*perWorker)
	line := regexp.MustCompile(`^\[18/Nov/2026 12:00:00\] "GET /some/path/ HTTP/1\.1" 200 2$`)
	for _, got := range lines {
		if !line.MatchString(got) {
			t.Fatalf("log line %q is not a whole line", got)
		}
	}
}
