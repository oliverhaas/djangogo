package views_test

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// defaultsHandler serves the routes of Django's tests/view_tests/urls.py that
// the tests of the default error views request. Unknown URLs fall through to
// the 404 page, as they do in Django's URL resolver.
func defaultsHandler(eng *templates.Engine) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/nonexistent_url/{$}", views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.NotFoundPage(w, r, eng, nil, "")
	}))
	mux.Handle("/server_error/{$}", views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.ServerErrorPage(w, r, eng, "")
	}))
	mux.Handle("/raises400/{$}", views.Handler(eng, func(http.ResponseWriter, *http.Request) error {
		return views.SuspiciousOperation("")
	}))
	mux.Handle("/raises403/{$}", views.Handler(eng, func(http.ResponseWriter, *http.Request) error {
		return views.PermissionDenied("Insufficient Permissions")
	}))
	mux.Handle("/technical404/{$}", views.Handler(eng, func(http.ResponseWriter, *http.Request) error {
		return views.NotFound("Testing technical 404.")
	}))
	mux.Handle("/", views.NotFoundHandler(eng))
	return mux
}

// nonexistentURLs are the two kinds of missing page of the Django tests: one
// routed to the 404 view and one that matches no route.
var nonexistentURLs = []string{"/nonexistent_url/", "/other_nonexistent_url/"}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_page_not_found
func TestPageNotFound(t *testing.T) {
	c := djangotest.NewClient(t, defaultsHandler(memEngine(t, nil)))
	for _, u := range nonexistentURLs {
		resp := c.Get(u)
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		djangotest.AssertContains(t, resp, "<h1>Not Found</h1>", djangotest.StatusCode(http.StatusNotFound))
		djangotest.AssertContains(t, resp, "<p>The requested resource was not found on this server.</p>",
			djangotest.StatusCode(http.StatusNotFound))
	}
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_csrf_token_in_404
//
// Django's csrf context processor is built in. Here a project registers a
// context processor that reads csrf.Token, and the 404 page must run it.
func TestCSRFTokenIn404(t *testing.T) {
	eng := memEngine(t, map[string]string{"404.html": "{{ csrf_token }}"})
	eng.AddContextProcessor(func(r *http.Request) map[string]any {
		return map[string]any{"csrf_token": csrf.Token(r.Context())}
	})
	store := sessions.NewSignedCookieStore([]byte("views-test-secret"))
	handler := sessions.Middleware(store, "sessionid")(csrf.Middleware(defaultsHandler(eng)))
	c := djangotest.NewClient(t, handler)
	for _, u := range nonexistentURLs {
		resp := c.Get(u)
		djangotest.AssertStatus(t, resp, http.StatusNotFound)
		djangotest.AssertNotEqual(t, resp.Content(), "NOTPROVIDED")
		djangotest.AssertNotEqual(t, resp.Content(), "")
	}
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_server_error
func TestServerError(t *testing.T) {
	resp := djangotest.NewClient(t, defaultsHandler(memEngine(t, nil))).Get("/server_error/")
	djangotest.AssertContains(t, resp, "<h1>Server Error (500)</h1>",
		djangotest.StatusCode(http.StatusInternalServerError))
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_bad_request
func TestBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	err := views.BadRequestPage(rec, httptest.NewRequest(http.MethodGet, "/", nil), nil, errors.New(""), "")
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, rec.Code, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "<h1>Bad Request (400)</h1>") {
		t.Errorf("body = %q, want the default 400 page", rec.Body.String())
	}
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_custom_bad_request_template
//
// Django asserts that the template context carries the request. The Go
// counterpart is that context processors run for the 400 template.
func TestCustomBadRequestTemplate(t *testing.T) {
	eng := memEngine(t, map[string]string{
		"400.html": "This is a test template for a 400 error (path: {{ request.path }})",
	})
	eng.AddContextProcessor(requestProcessor)
	resp := djangotest.NewClient(t, defaultsHandler(eng)).Get("/raises400/")
	djangotest.AssertContains(t, resp, "test template for a 400 error (path: /raises400/)",
		djangotest.StatusCode(http.StatusBadRequest))
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_custom_templates
func TestCustomTemplates(t *testing.T) {
	eng := memEngine(t, map[string]string{
		"404.html": "This is a test template for a 404 error " +
			"(path: {{ request_path }}, exception: {{ exception }}).",
		"500.html": "This is a test template for a 500 error.",
	})
	c := djangotest.NewClient(t, defaultsHandler(eng))
	resp := c.Get("/server_error/")
	djangotest.AssertContains(t, resp, "test template for a 500 error",
		djangotest.StatusCode(http.StatusInternalServerError))
	resp = c.Get("/no_such_url/")
	djangotest.AssertContains(t, resp, "path: /no_such_url/", djangotest.StatusCode(http.StatusNotFound))
	djangotest.AssertContains(t, resp, "exception: Resolver404", djangotest.StatusCode(http.StatusNotFound))
	resp = c.Get("/technical404/")
	djangotest.AssertContains(t, resp, "exception: Testing technical 404.",
		djangotest.StatusCode(http.StatusNotFound))
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_custom_templates_wrong
func TestCustomTemplatesWrong(t *testing.T) {
	eng := memEngine(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	pages := map[string]func(w http.ResponseWriter) error{
		"bad_request": func(w http.ResponseWriter) error {
			return views.BadRequestPage(w, req, eng, errors.New(""), "nonexistent")
		},
		"permission_denied": func(w http.ResponseWriter) error {
			return views.PermissionDeniedPage(w, req, eng, errors.New(""), "nonexistent")
		},
		"page_not_found": func(w http.ResponseWriter) error {
			return views.NotFoundPage(w, req, eng, views.ErrNotFound, "nonexistent")
		},
		"server_error": func(w http.ResponseWriter) error {
			return views.ServerErrorPage(w, req, eng, "nonexistent")
		},
	}
	for name, page := range pages {
		rec := httptest.NewRecorder()
		err := page(rec)
		if !errors.Is(err, views.ErrTemplateDoesNotExist) {
			t.Errorf("%s: error = %v, want ErrTemplateDoesNotExist", name, err)
		}
		djangotest.AssertEqual(t, rec.Body.Len(), 0)
	}
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_error_pages
func TestErrorPages(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	pages := []struct {
		title string
		page  func(w http.ResponseWriter) error
	}{
		{"Bad Request (400)", func(w http.ResponseWriter) error {
			return views.BadRequestPage(w, req, nil, errors.New(""), "")
		}},
		{"403 Forbidden", func(w http.ResponseWriter) error {
			return views.PermissionDeniedPage(w, req, nil, errors.New(""), "")
		}},
		{"Not Found", func(w http.ResponseWriter) error {
			return views.NotFoundPage(w, req, nil, views.ErrNotFound, "")
		}},
		{"Server Error (500)", func(w http.ResponseWriter) error {
			return views.ServerErrorPage(w, req, nil, "")
		}},
	}
	for _, tc := range pages {
		rec := httptest.NewRecorder()
		djangotest.AssertEqual(t, tc.page(rec), nil)
		body := rec.Body.String()
		for _, want := range []string{
			"<!doctype html>", `<html lang="en">`, "<head>", "<title>" + tc.title + "</title>", "<body>",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s page lacks %q:\n%s", tc.title, want, body)
			}
		}
	}
}

// TestServeErrorMapping asserts the central mapping from errors to statuses,
// including wrapped errors and the ORM's "does not exist".
func TestServeErrorMapping(t *testing.T) {
	eng := memEngine(t, map[string]string{
		"403.html": "forbidden: {{ exception }}",
		"404.html": "missing: {{ exception }}",
	})
	tests := []struct {
		name string
		err  error
		code int
		body string
	}{
		{"not found", views.NotFound("No Poll matches the given query."), 404, "missing: No Poll matches the given query."},
		{"not found sentinel", views.ErrNotFound, 404, "missing: Http404"},
		{"orm does not exist", fmt.Errorf("orm: get poll: %w", orm.ErrDoesNotExist), 404, "missing: Http404"},
		{"markup in the message", views.NotFound("<script>x</script>"), 404, "missing: &lt;script&gt;x&lt;/script&gt;"},
		{"permission denied", views.PermissionDenied("Insufficient Permissions"), 403, "forbidden: Insufficient Permissions"},
		{"wrapped permission denied", fmt.Errorf("check: %w", views.ErrPermissionDenied), 403, "forbidden: "},
		{"bad request", views.BadRequest("Malformed request syntax"), 400, "<h1>Bad Request (400)</h1>"},
		{"suspicious operation", views.SuspiciousOperation("dubious"), 400, "<h1>Bad Request (400)</h1>"},
		{"disallowed redirect", views.ErrDisallowedRedirect, 400, "<h1>Bad Request (400)</h1>"},
		{"improperly configured", views.ErrImproperlyConfigured, 500, "<h1>Server Error (500)</h1>"},
		{"other", errors.New("database is on fire"), 500, "<h1>Server Error (500)</h1>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			djangotest.AssertEqual(t, views.StatusCode(tc.err), tc.code)
			handler := views.Handler(eng, func(http.ResponseWriter, *http.Request) error { return tc.err })
			resp := djangotest.NewClient(t, handler).Get("/some/path/")
			djangotest.AssertContains(t, resp, tc.body, djangotest.StatusCode(tc.code))
			djangotest.AssertNotContains(t, resp, "on fire", djangotest.StatusCode(tc.code))
			var wantLogs []string
			if tc.code == http.StatusInternalServerError {
				wantLogs = []string{"Internal Server Error: /some/path/"}
			}
			djangotest.AssertSequenceEqual(t, logs.messages(slog.LevelError), wantLogs)
		})
	}
}

// TestHandlerRenderFailure asserts that a view returning the error of a failed
// Render gets one 500 response, not two.
func TestHandlerRenderFailure(t *testing.T) {
	logs := captureLogs(t)
	eng := memEngine(t, map[string]string{"500.html": "custom 500"})
	handler := views.Handler(eng, func(w http.ResponseWriter, r *http.Request) error {
		return views.Render(w, r, eng, "no_such_template.html", nil)
	})
	resp := djangotest.NewClient(t, handler).Get("/broken/")
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	djangotest.AssertEqual(t, resp.Content(), "custom 500")
	djangotest.AssertSequenceEqual(t, logs.messages(slog.LevelError), []string{"Internal Server Error: /broken/"})
}

// TestBrokenErrorTemplate asserts that an error template that cannot be
// rendered still gives a 500 response with the default body.
func TestBrokenErrorTemplate(t *testing.T) {
	captureLogs(t)
	eng := memEngine(t, map[string]string{"404.html": `{% include "no_such_partial.html" %}`})
	handler := views.Handler(eng, func(http.ResponseWriter, *http.Request) error { return views.ErrNotFound })
	resp := djangotest.NewClient(t, handler).Get("/missing/")
	djangotest.AssertContains(t, resp, "<h1>Server Error (500)</h1>",
		djangotest.StatusCode(http.StatusInternalServerError))
}

// TestNotFoundPageQuotesPath asserts that request_path is percent-quoted, as
// Django does to keep request text out of the page.
func TestNotFoundPageQuotesPath(t *testing.T) {
	eng := memEngine(t, map[string]string{"404.html": "path: {{ request_path }}"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = `/<b>bold</b>/ "x"/`
	djangotest.AssertEqual(t, views.NotFoundPage(rec, req, eng, views.ErrNotFound, ""), nil)
	djangotest.AssertEqual(t, rec.Body.String(), "path: /%3Cb%3Ebold%3C/b%3E/%20%22x%22/")
}
