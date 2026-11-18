package middleware_test

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports from Django's
// tests/view_tests/tests/test_debug.py. Django builds an ExceptionReporter for
// a request and an exception, or lets the view raises500 return the technical
// 500 response. Here a view panics inside Recover with Debug on, and the tests
// read the page it answers with. A Python exception class becomes the Go type
// of the panic value.

// cleansedSubstitute is what the page shows in place of a sensitive value
// (SafeExceptionReporterFilter.cleansed_substitute).
const cleansedSubstitute = "********************"

// raises500 is the port of the raises500 view: it panics with value.
func raises500(value any) http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(value)
	})
}

// technical500 wraps view in Recover with Debug on and the log discarded.
func technical500(view http.Handler) http.Handler {
	return middleware.Recover(middleware.RecoverOptions{Debug: true, Log: io.Discard})(view)
}

// debugRow is the table row of the debug page for a variable and the Python
// repr of its value, as Django's template renders it.
func debugRow(name, value string) string {
	return "<td>" + name + `</td><td class="code"><pre>&#x27;` + value + "&#x27;</pre></td>"
}

// TestTechnical500 checks the three landmarks of the technical 500 page. The
// row "Raised during" names the view the URL resolver found, and the second
// half of the Django test asks for the plain text page. Recover knows no
// resolver and renders HTML only, so both are left out.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::DebugViewTests.test_technical_500
func TestTechnical500(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500(errors.New("boom")))).Get("/raises500/")
	djangotest.AssertContains(t, resp, `<header id="summary">`, djangotest.StatusCode(500))
	djangotest.AssertContains(t, resp, `<main id="info">`, djangotest.StatusCode(500))
	djangotest.AssertContains(t, resp, `<footer id="explanation">`, djangotest.StatusCode(500))
}

// TestSafestringInException checks that markup in the panic value is escaped
// everywhere it is shown: below the heading, in the summary table, and at the
// top of the traceback.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::DebugViewTests.test_safestring_in_exception
func TestSafestringInException(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500("<script>alert(1);</script>"))).Get("/safestring_exception/")
	djangotest.AssertNotContains(t, resp, "<script>alert(1);</script>", djangotest.StatusCode(500))
	djangotest.AssertContains(t, resp, "&lt;script&gt;alert(1);&lt;/script&gt;",
		djangotest.Count(3), djangotest.StatusCode(500))
}

// TestRequestAndException checks that a simple exception report can be
// generated. The Go type of an errors.New value stands for ValueError. The USER
// section has no counterpart: this package does not know about users.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterTests.test_request_and_exception
func TestRequestAndException(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500(errors.New("Can't find my keys")))).Get("/test_view/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	html := resp.Content()
	djangotest.AssertInHTML(t, "<h1>*errors.errorString at /test_view/</h1>", html)
	for _, want := range []string{
		`<pre class="exception_value">Can&#x27;t find my keys</pre>`,
		`<th scope="row">Request Method:</th>`,
		`<th scope="row">Request URL:</th>`,
		`<th scope="row">Exception Type:</th>`,
		`<th scope="row">Exception Value:</th>`,
		"<h2>Traceback ",
		"<h2>Request information</h2>",
		"<p>No POST data</p>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the debug page lacks %q", want)
		}
	}
	if strings.Contains(html, "<p>Request data not supplied</p>") {
		t.Error("the debug page says the request data was not supplied")
	}
}

// TestDisallowedHost checks that an exception report can be generated even for
// a disallowed host: the page shows the host as the client sent it.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterTests.test_disallowed_host
func TestDisallowedHost(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500("boom"))).Get("/", djangotest.Header("Host", "evil.com"))
	djangotest.AssertContains(t, resp, "http://evil.com/", djangotest.StatusCode(500))
}

// TestRequestWithItemsKey checks that an exception report can be generated for
// requests with "items" in the GET, POST, or COOKIES data. The FILES case is
// left out: the page has no section for uploaded files.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterTests.test_request_with_items_key
func TestRequestWithItemsKey(t *testing.T) {
	value := debugRow("items", "Oops")
	handler := technical500(raises500("boom"))

	// GET
	resp := djangotest.NewClient(t, handler).Get("/test_view/?items=Oops")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertInHTML(t, value, resp.Content())

	// POST
	resp = djangotest.NewClient(t, handler).Post("/test_view/", url.Values{"items": {"Oops"}})
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertInHTML(t, value, resp.Content())

	// COOKIES
	c := djangotest.NewClient(t, handler)
	c.SetCookie(&http.Cookie{Name: "items", Value: "Oops"})
	resp = c.Get("/test_view/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertInHTML(t, value, resp.Content())
}

// TestGetRawInsecureURI checks the request URL the page shows: the scheme, the
// host as the client sent it, and the request target, none of them validated.
// Django reads it from the private method _get_raw_insecure_uri; here it is the
// "Request URL" row. Django's RequestFactory takes two of the four slashes of
// the first URL for an empty authority, while a Go request keeps the target as
// it was sent, so the first expected value has all four.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterTests.test_get_raw_insecure_uri
func TestGetRawInsecureURI(t *testing.T) {
	handler := technical500(raises500("boom"))
	for _, tc := range []struct{ url, expected string }{
		{"////absolute-uri", "http://evil.com////absolute-uri"},
		{"/?foo=bar", "http://evil.com/?foo=bar"},
		{"/path/with:colons", "http://evil.com/path/with:colons"},
	} {
		t.Run(tc.url, func(t *testing.T) {
			resp := djangotest.NewClient(t, handler, djangotest.WithHost("evil.com")).Get(tc.url)
			djangotest.AssertContains(t, resp,
				`<th scope="row">Request URL:</th><td>`+tc.expected+"</td>",
				djangotest.StatusCode(500), djangotest.HTML())
		})
	}
}

// TestCleanseSettingBasic checks through the page what Django checks on
// SafeExceptionReporterFilter.cleanse_setting: an ordinary name keeps its
// value, and a sensitive name shows the substitute.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterFilterTests.test_cleanse_setting_basic
func TestCleanseSettingBasic(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500("boom"))).Get("/",
		djangotest.Query(url.Values{"TEST": {"TEST"}, "PASSWORD": {"super_secret"}}))
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	html := resp.Content()
	djangotest.AssertInHTML(t, debugRow("TEST", "TEST"), html)
	djangotest.AssertInHTML(t, debugRow("PASSWORD", cleansedSubstitute), html)
	djangotest.AssertNotContains(t, resp, "super_secret", djangotest.StatusCode(500))
}

// TestCleanseSettingIgnoreCase checks that a sensitive name is recognized in
// any case.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterFilterTests.test_cleanse_setting_ignore_case
func TestCleanseSettingIgnoreCase(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500("boom"))).Get("/",
		djangotest.Query(url.Values{"password": {"super_secret"}}))
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertInHTML(t, debugRow("password", cleansedSubstitute), resp.Content())
	djangotest.AssertNotContains(t, resp, "super_secret", djangotest.StatusCode(500))
}

// TestRequestMetaFiltering checks that the values of request headers with a
// sensitive name are cleansed, and the Cookie header with them. Django reads
// the cleansed META from get_safe_request_meta; here the rows of the request
// headers table are read. A Go header is shown under its canonical name, so
// HTTP_API_URL is the header Api-Url.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterFilterTests.test_request_meta_filtering
func TestRequestMetaFiltering(t *testing.T) {
	headers := []string{
		"Api-Url",
		"A-Signature-Value",
		"My-Key",
		"Password",
		"Secret-Value",
		"Some-Token",
		"The-Auth",
	}
	opts := make([]djangotest.RequestOption, 0, len(headers))
	for _, header := range headers {
		opts = append(opts, djangotest.Header(header, "super secret"))
	}
	c := djangotest.NewClient(t, technical500(raises500("boom")))
	c.SetCookie(&http.Cookie{Name: "flavor", Value: "oatmeal"})
	resp := c.Get("/", opts...)
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)

	html := resp.Content()
	for _, header := range headers {
		t.Run(header, func(t *testing.T) {
			djangotest.AssertInHTML(t, debugRow(header, cleansedSubstitute), html)
		})
	}
	djangotest.AssertInHTML(t, debugRow("Cookie", cleansedSubstitute), html)
	djangotest.AssertNotContains(t, resp, "super secret", djangotest.StatusCode(500))
}

// TestExceptionReportUsesMetaFiltering checks that the page applies the
// filtering of request headers. The second half of the Django test asks for a
// JSON report, which Recover does not render.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterFilterTests.test_exception_report_uses_meta_filtering
func TestExceptionReportUsesMetaFiltering(t *testing.T) {
	resp := djangotest.NewClient(t, technical500(raises500("boom"))).Get("/raises500/",
		djangotest.Header("Secret-Header", "super_secret"))
	djangotest.AssertNotContains(t, resp, "super_secret", djangotest.StatusCode(500))
}

// TestCleanseSessionCookieValue checks that the value of the session cookie is
// never shown. The setting SESSION_COOKIE_NAME is the option SessionCookieName.
//
// Ported from Django: tests/view_tests/tests/test_debug.py::ExceptionReporterFilterTests.test_cleanse_session_cookie_value
func TestCleanseSessionCookieValue(t *testing.T) {
	handler := middleware.Recover(middleware.RecoverOptions{
		Debug:             true,
		Log:               io.Discard,
		SessionCookieName: "djangosession",
	})(raises500("boom"))
	c := djangotest.NewClient(t, handler)
	c.SetCookie(&http.Cookie{Name: "djangosession", Value: "should not be displayed"})
	resp := c.Get("/raises500/")
	djangotest.AssertNotContains(t, resp, "should not be displayed", djangotest.StatusCode(500))
}
