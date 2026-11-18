package middleware_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/middleware"
)

// The tests in this file are ports from Django's
// tests/view_tests/tests/test_defaults.py. Django calls the default error
// views directly. This package has no such views: the plain 500 page is what
// Recover answers with when Debug is off, and the plain 400 page is what
// AllowedHosts answers with for a host that is not allowed.

// serverErrorHandler is a view that panics inside Recover with Debug off.
func serverErrorHandler() http.Handler {
	return middleware.Recover(middleware.RecoverOptions{Log: io.Discard})(raises500(errors.New("kaboom")))
}

// badRequestHandler is a view behind AllowedHosts that does not allow the host
// of the test client.
func badRequestHandler() http.Handler {
	return middleware.AllowedHosts(middleware.AllowedHostsOptions{AllowedHosts: []string{"example.com"}})(emptyView)
}

// TestServerError checks that a panic gives the plain 500 page. The last two
// assertions are additions: with Debug off the page shows neither the panic
// value nor a traceback.
//
// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_server_error
func TestServerError(t *testing.T) {
	resp := djangotest.NewClient(t, serverErrorHandler()).Get("/server_error/")
	djangotest.AssertContains(t, resp, "<h1>Server Error (500)</h1>", djangotest.StatusCode(500))
	djangotest.AssertNotContains(t, resp, "kaboom", djangotest.StatusCode(500))
	djangotest.AssertNotContains(t, resp, "goroutine", djangotest.StatusCode(500))
}

// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_bad_request
func TestBadRequest(t *testing.T) {
	resp := djangotest.NewClient(t, badRequestHandler()).Get("/")
	djangotest.AssertContains(t, resp, "<h1>Bad Request (400)</h1>", djangotest.StatusCode(400))
}

// TestErrorPages checks the frame of the plain error pages. The 403 and 404
// pages of the Django test belong to other packages.
//
// Ported from Django: tests/view_tests/tests/test_defaults.py::DefaultsTests.test_error_pages
func TestErrorPages(t *testing.T) {
	for _, tc := range []struct {
		handler http.Handler
		title   string
	}{
		{badRequestHandler(), "Bad Request (400)"},
		{serverErrorHandler(), "Server Error (500)"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			content := djangotest.NewClient(t, tc.handler).Get("/").Content()
			for _, want := range []string{
				"<!doctype html>",
				`<html lang="en">`,
				"<head>",
				"<title>" + tc.title + "</title>",
				"<body>",
			} {
				if !strings.Contains(content, want) {
					t.Errorf("the page lacks %q:\n%s", want, content)
				}
			}
		})
	}
}
