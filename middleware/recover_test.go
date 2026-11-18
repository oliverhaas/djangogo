package middleware_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// serverErrorPage is Django's plain 500 page (ERROR_PAGE_TEMPLATE rendered by
// django.views.defaults.server_error), byte for byte.
const serverErrorPage = `
<!doctype html>
<html lang="en">
<head>
  <title>Server Error (500)</title>
</head>
<body>
  <h1>Server Error (500)</h1><p></p>
</body>
</html>
`

// servePanic serves a GET through handler and returns the value the handler
// panicked with, or nil when it returned.
func servePanic(handler http.Handler) (value any) {
	defer func() { value = recover() }()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	return nil
}

// TestRecoverPlainPage checks the answer to a panic without Debug and without
// Handler500: Django's plain 500 page, and none of the headers the handler set
// for the response it did not send.
func TestRecoverPlainPage(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/csv")
		h.Set("Content-Length", "5")
		h.Set("Content-Disposition", `attachment; filename="report.csv"`)
		h.Set("Cache-Control", "public, max-age=3600")
		h.Set("ETag", `"report-1"`)
		h.Set("X-Request-Id", "42")
		panic("boom")
	})
	handler := middleware.Recover(middleware.RecoverOptions{Log: io.Discard})(view)

	resp := djangotest.NewClient(t, handler).Get("/report/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, resp.Content(), serverErrorPage)
	djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), "text/html; charset=utf-8")
	for _, name := range []string{"Content-Length", "Content-Disposition", "Cache-Control", "ETag"} {
		assertNoHeader(t, resp, name)
	}
	djangotest.AssertEqual(t, resp.Header.Get("X-Request-Id"), "42")
}

// TestRecoverLogRecord checks the layout of a log record: Django's message
// with the path, the type and the value of the panic, and the stack trace,
// which starts at the panic and not at the code that recovered it.
func TestRecoverLogRecord(t *testing.T) {
	var log bytes.Buffer
	handler := middleware.Recover(middleware.RecoverOptions{Log: &log})(raises500(errors.New("kaboom")))
	djangotest.NewClient(t, handler).Get("/fail/")

	record := regexp.MustCompile(`^Internal Server Error: /fail/\n` +
		`panic: \*errors\.errorString: kaboom\n\n` +
		`goroutine \d+ \[running\]:\npanic\(`)
	if !record.MatchString(log.String()) {
		t.Errorf("the log record does not match %s:\n%s", record, log.String())
	}
	if !strings.Contains(log.String(), "raises500") {
		t.Errorf("the stack trace lacks the function that panicked:\n%s", log.String())
	}
}

// TestRecoverStartedResponse checks that a panic after the response was started
// is only logged: the status is on its way, and an error page would end up in
// the middle of the body.
func TestRecoverStartedResponse(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "partial")
		panic("boom")
	})
	for _, debug := range []bool{false, true} {
		var log bytes.Buffer
		handler := middleware.Recover(middleware.RecoverOptions{Debug: debug, Log: &log})(view)

		resp := djangotest.NewClient(t, handler).Get("/late/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, resp.Content(), "partial")
		djangotest.AssertEqual(t, firstLine(&log), "Internal Server Error: /late/")
	}
}

// TestRecoverAbortHandler checks that http.ErrAbortHandler passes Recover: the
// server needs it to abort the response, and it is no error worth a record.
func TestRecoverAbortHandler(t *testing.T) {
	for _, abort := range []error{
		http.ErrAbortHandler,
		fmt.Errorf("copy the upstream response: %w", http.ErrAbortHandler),
	} {
		var log bytes.Buffer
		handler := middleware.Recover(middleware.RecoverOptions{Log: &log})(raises500(abort))

		value := servePanic(handler)
		if err, ok := value.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("panic(%v): the handler panicked with %v, want the same error", abort, value)
		}
		djangotest.AssertEqual(t, log.String(), "")
	}
}

// TestRecoverHandler500 checks that Handler500 answers a panic. It gets the
// request that failed, and its page is sent as a 500 although it chose no
// status. The type of the response that was not sent does not stick to it.
func TestRecoverHandler500(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		panic("boom")
	})
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "custom page for "+r.URL.Path)
	})
	var log bytes.Buffer
	handler := middleware.Recover(middleware.RecoverOptions{Log: &log, Handler500: page})(view)

	resp := djangotest.NewClient(t, handler).Get("/broken/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, resp.Content(), "custom page for /broken/")
	if got := resp.Header.Get("Content-Type"); got == "text/csv" {
		t.Error("the 500 page kept the Content-Type of the response that was not sent")
	}
	djangotest.AssertEqual(t, strings.Count(log.String(), "Internal Server Error: "), 1)
}

// TestRecoverHandler500Status checks the status of the response of Handler500:
// a 200, however it comes about, is sent as 500, and any other status the
// handler chooses is kept.
func TestRecoverHandler500Status(t *testing.T) {
	for _, tc := range []struct {
		name       string
		page       http.HandlerFunc
		wantStatus int
		wantBody   string
	}{
		{"nothing written", func(http.ResponseWriter, *http.Request) {}, http.StatusInternalServerError, ""},
		{"explicit 200", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "page")
		}, http.StatusInternalServerError, "page"},
		{"flush first", func(w http.ResponseWriter, _ *http.Request) {
			_ = http.NewResponseController(w).Flush()
			_, _ = io.WriteString(w, "page")
		}, http.StatusInternalServerError, "page"},
		{"explicit 503", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "maintenance")
		}, http.StatusServiceUnavailable, "maintenance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := middleware.Recover(middleware.RecoverOptions{Log: io.Discard, Handler500: tc.page})(raises500("boom"))
			resp := djangotest.NewClient(t, handler).Get("/")
			djangotest.AssertStatus(t, resp, tc.wantStatus)
			djangotest.AssertEqual(t, resp.Content(), tc.wantBody)
		})
	}
}

// TestRecoverHandler500Panics checks that a panic of Handler500 is logged as
// well and answered with the plain page, without the headers the handler set.
// When the handler had started its response, nothing is added to it.
func TestRecoverHandler500Panics(t *testing.T) {
	t.Run("before the response", func(t *testing.T) {
		page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			panic("the template is broken")
		})
		var log bytes.Buffer
		handler := middleware.Recover(middleware.RecoverOptions{Log: &log, Handler500: page})(raises500("boom"))

		resp := djangotest.NewClient(t, handler).Get("/broken/")
		djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
		djangotest.AssertEqual(t, resp.Content(), serverErrorPage)
		djangotest.AssertEqual(t, resp.Header.Get("Content-Type"), "text/html; charset=utf-8")
		djangotest.AssertEqual(t, strings.Count(log.String(), "Internal Server Error: /broken/\n"), 2)
		for _, want := range []string{"\npanic: string: boom\n", "\npanic in Handler500: string: the template is broken\n"} {
			if !strings.Contains(log.String(), want) {
				t.Errorf("the log lacks %q:\n%s", want, log.String())
			}
		}
	})

	t.Run("after the response started", func(t *testing.T) {
		page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "half a page")
			panic("the template is broken")
		})
		var log bytes.Buffer
		handler := middleware.Recover(middleware.RecoverOptions{Log: &log, Handler500: page})(raises500("boom"))

		resp := djangotest.NewClient(t, handler).Get("/broken/")
		djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
		djangotest.AssertEqual(t, resp.Content(), "half a page")
		djangotest.AssertEqual(t, strings.Count(log.String(), "panic in Handler500: "), 1)
	})
}

// TestRecoverDebugIgnoresHandler500 checks that with Debug the technical page
// is the answer, as Django's DEBUG leaves handler500 out.
func TestRecoverDebugIgnoresHandler500(t *testing.T) {
	page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "custom page")
	})
	handler := middleware.Recover(middleware.RecoverOptions{Debug: true, Log: io.Discard, Handler500: page})(raises500("boom"))

	resp := djangotest.NewClient(t, handler).Get("/")
	djangotest.AssertContains(t, resp, `<header id="summary">`, djangotest.StatusCode(500))
	djangotest.AssertNotContains(t, resp, "custom page", djangotest.StatusCode(500))
}

// TestDebugPageEscapesRequestData checks that markup a client sends does not
// reach the technical 500 page as markup, wherever the page shows it: in the
// path, in the names and the values of the query, in a cookie, and in a header.
func TestDebugPageEscapesRequestData(t *testing.T) {
	c := djangotest.NewClient(t, technical500(raises500("boom")))
	c.SetCookie(&http.Cookie{Name: "theme", Value: "<u>"})
	resp := c.Get("/%3Cb%3Epath%3C%2Fb%3E/?%3Cem%3E=%3Ci%3E",
		djangotest.Header("X-Evil", "<script>alert(2)</script>"))

	for _, markup := range []string{"<b>", "<em>", "<i>", "<u>", "<script>alert(2)</script>"} {
		djangotest.AssertNotContains(t, resp, markup, djangotest.StatusCode(500))
	}
	djangotest.AssertContains(t, resp, "<h1>string at /&lt;b&gt;path&lt;/b&gt;/</h1>", djangotest.StatusCode(500))
	html := resp.Content()
	djangotest.AssertInHTML(t, debugRow("&lt;em&gt;", "&lt;i&gt;"), html)
	djangotest.AssertInHTML(t, debugRow("theme", "&lt;u&gt;"), html)
	djangotest.AssertInHTML(t, debugRow("X-Evil", "&lt;script&gt;alert(2)&lt;/script&gt;"), html)
}

// TestDebugPageMasksSecretHeaders checks that the page shows no credential a
// client sent in a header: the header is listed, and its value is not.
func TestDebugPageMasksSecretHeaders(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500("boom"))).Get("/",
		djangotest.Header("Authorization", "Bearer hunter2"),
		djangotest.Header("X-Api-Key", "hunter3"),
		djangotest.Header("Accept-Language", "de"))

	djangotest.AssertNotContains(t, resp, "hunter", djangotest.StatusCode(500))
	html := resp.Content()
	djangotest.AssertInHTML(t, debugRow("Authorization", cleansedSubstitute), html)
	djangotest.AssertInHTML(t, debugRow("X-Api-Key", cleansedSubstitute), html)
	djangotest.AssertInHTML(t, debugRow("Accept-Language", "de"), html)
}

// TestDebugPagePostData checks that the POST section shows the form whatever
// the handler did with the body before it panicked, and that a body that is no
// form is not shown at all.
func TestDebugPagePostData(t *testing.T) {
	form := url.Values{"name": {"value"}, "other": {"thing"}}

	for _, tc := range []struct {
		name string
		read func(r *http.Request) error
	}{
		{"form parsed", func(r *http.Request) error { return r.ParseForm() }},
		{"body partly read", func(r *http.Request) error {
			_, err := io.ReadFull(r.Body, make([]byte, 7))
			return err
		}},
		{"body read and closed", func(r *http.Request) error {
			if _, err := io.ReadAll(r.Body); err != nil {
				return err
			}
			return r.Body.Close()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if err := tc.read(r); err != nil {
					t.Errorf("read the body: %v", err)
				}
				panic("boom")
			})
			resp := djangotest.NewClient(t, technical500(view)).Post("/submit/", form)
			djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
			djangotest.AssertInHTML(t, debugRow("name", "value"), resp.Content())
			djangotest.AssertInHTML(t, debugRow("other", "thing"), resp.Content())
		})
	}

	t.Run("not a form", func(t *testing.T) {
		resp := djangotest.NewClient(t, technical500(raises500("boom"))).Post("/api/", `{"name": "hunter2"}`,
			djangotest.ContentType("application/json"))
		djangotest.AssertContains(t, resp, "<p>No POST data</p>", djangotest.StatusCode(500))
		djangotest.AssertNotContains(t, resp, "hunter2", djangotest.StatusCode(500))
	})
}

// TestDebugPageValueRepr checks how the page shows values: as Python shows a
// string, with the quotes that need no escaping. A name with several values
// shows all of them as a list, where Django shows only the last one.
func TestDebugPageValueRepr(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500("boom"))).Get("/?tag=a&tag=b&q=it%27s&empty=")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	html := resp.Content()
	djangotest.AssertInHTML(t, `<td>tag</td><td class="code"><pre>[&#x27;a&#x27;, &#x27;b&#x27;]</pre></td>`, html)
	djangotest.AssertInHTML(t, `<td>q</td><td class="code"><pre>&quot;it&#x27;s&quot;</pre></td>`, html)
	djangotest.AssertInHTML(t, debugRow("empty", ""), html)
}

// TestDebugPageEmptyPanicValue checks the page for a panic whose value prints
// as nothing: Django's text for an exception without a message stands in, and
// the summary has no row for the value.
func TestDebugPageEmptyPanicValue(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500(""))).Get("/")
	djangotest.AssertContains(t, resp, `<pre class="exception_value">No exception message supplied</pre>`,
		djangotest.StatusCode(500))
	djangotest.AssertNotContains(t, resp, "Exception Value:", djangotest.StatusCode(500))
}

// TestDebugPageLocation checks that the page names the place of the panic. For
// a runtime error that is the code that caused it, not the function of the
// runtime that raised it.
func TestDebugPageLocation(t *testing.T) {
	view := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, 100/len(r.URL.RawQuery))
	})

	resp := djangotest.NewClient(t, technical500(view)).Get("/divide/")
	djangotest.AssertContains(t, resp, "<pre>runtime error: integer divide by zero</pre>", djangotest.StatusCode(500))
	location := regexp.MustCompile(`Exception Location:</th><td><span class="fname">[^<]*/recover_test\.go</span>, ` +
		`line \d+, in [\w./]+\.TestDebugPageLocation\.func1</td>`)
	if !location.MatchString(resp.Content()) {
		t.Errorf("the page does not name the place of the panic (%s):\n%s", location, resp.Content())
	}
}
