package staticfiles_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of Django's
// tests/staticfiles_tests/test_views.py. Django runs the TestDefaults mixin
// against two URLconfs: one that routes STATIC_URL to the serve view
// (urls/default.py), which is Handler mounted on a mux here, and one built by
// the staticfiles_urlpatterns helper (urls/helper.py), for a project that
// routes nothing itself, which is Middleware here.
//
// The file also holds the ports of the tests of Django's StaticFilesHandler,
// the handler that runserver wraps around a project: TestRunserver in
// test_management.py and test_handlers.py. Not ported from those:
// TestRunserver.test_middleware_loaded_only_once (Django's middleware loading)
// and test_non_http_requests_passed_to_the_wrapped_application (an ASGI scope
// that is not HTTP; every request of net/http is).

// defaultURLConf is staticfiles_tests/urls/default.py: the serve view routed
// under STATIC_URL.
func defaultURLConf(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/static/", staticfiles.Handler("/static/", newSite(t).finder()))
	return mux
}

// helperURLConf is staticfiles_tests/urls/helper.py: a project without a
// route of its own for static files.
func helperURLConf(t *testing.T) http.Handler {
	t.Helper()
	return staticfiles.Middleware("static/", newSite(t).finder())(http.NotFoundHandler())
}

// TestRunserver404Response answers 404 for a static file that is missing,
// where the request could also have been passed on to the project. Django
// tests the handler that runserver builds, which is Middleware here, with
// DEBUG on and off; the handler has no such switch.
//
// Ported from Django: tests/staticfiles_tests/test_management.py::TestRunserver.test_404_response
func TestRunserver404Response(t *testing.T) {
	handler := staticfiles.Middleware("/static/", newSite(t).finder())(passedOn)

	resp := djangotest.NewClient(t, handler).Get("/static/unknown.css")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
}

// TestASGIStaticFilesHandlerGetAsyncResponse serves a static file through the
// handler that wraps the project. Django has one such handler for WSGI and one
// for ASGI; Middleware stands for both.
//
// Ported from Django: tests/staticfiles_tests/test_handlers.py::TestASGIStaticFilesHandler.test_get_async_response
func TestASGIStaticFilesHandlerGetAsyncResponse(t *testing.T) {
	handler := staticfiles.Middleware("/static/", newSite(t).finder())(passedOn)

	resp := djangotest.NewClient(t, handler).Get("/static/test/file.txt")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
}

// TestASGIStaticFilesHandlerGetAsyncResponseNotFound answers 404 for a missing
// file. Django also asserts on which thread its error handler runs, which has
// no counterpart here.
//
// Ported from Django: tests/staticfiles_tests/test_handlers.py::TestASGIStaticFilesHandler.test_get_async_response_not_found
func TestASGIStaticFilesHandlerGetAsyncResponseNotFound(t *testing.T) {
	handler := staticfiles.Middleware("/static/", newSite(t).finder())(passedOn)

	resp := djangotest.NewClient(t, handler).Get("/static/test/not-found.txt")

	djangotest.AssertEqual(t, resp.StatusCode, http.StatusNotFound)
}

// TestServeDisabledDisabledServing is not run: Django's serve view answers 404
// unless DEBUG is on, and here that decision belongs to the caller, which
// mounts Handler only in debug mode. The handler itself has no switch.
//
// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeDisabled.test_disabled_serving
func TestServeDisabledDisabledServing(t *testing.T) {
	t.Skip("the DEBUG gate is the caller's: it mounts staticfiles.Handler only when debug is on")
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithDefaultURL.test_staticfiles_dirs
func TestServeStaticWithDefaultURLStaticfilesDirs(t *testing.T) {
	checkStaticfilesDirs(t, served(t, defaultURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithDefaultURL.test_staticfiles_dirs_subdir
func TestServeStaticWithDefaultURLStaticfilesDirsSubdir(t *testing.T) {
	checkStaticfilesDirsSubdir(t, served(t, defaultURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithDefaultURL.test_staticfiles_dirs_priority
func TestServeStaticWithDefaultURLStaticfilesDirsPriority(t *testing.T) {
	checkStaticfilesDirsPriority(t, served(t, defaultURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithDefaultURL.test_app_files
func TestServeStaticWithDefaultURLAppFiles(t *testing.T) {
	checkAppFiles(t, served(t, defaultURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithDefaultURL.test_nonascii_filenames
func TestServeStaticWithDefaultURLNonasciiFilenames(t *testing.T) {
	checkNonasciiFilenames(t, served(t, defaultURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithDefaultURL.test_camelcase_filenames
func TestServeStaticWithDefaultURLCamelcaseFilenames(t *testing.T) {
	checkCamelcaseFilenames(t, served(t, defaultURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithDefaultURL.test_filename_with_percent_sign
func TestServeStaticWithDefaultURLFilenameWithPercentSign(t *testing.T) {
	checkFilenameWithPercentSign(t, served(t, defaultURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithURLHelper.test_staticfiles_dirs
func TestServeStaticWithURLHelperStaticfilesDirs(t *testing.T) {
	checkStaticfilesDirs(t, served(t, helperURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithURLHelper.test_staticfiles_dirs_subdir
func TestServeStaticWithURLHelperStaticfilesDirsSubdir(t *testing.T) {
	checkStaticfilesDirsSubdir(t, served(t, helperURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithURLHelper.test_staticfiles_dirs_priority
func TestServeStaticWithURLHelperStaticfilesDirsPriority(t *testing.T) {
	checkStaticfilesDirsPriority(t, served(t, helperURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithURLHelper.test_app_files
func TestServeStaticWithURLHelperAppFiles(t *testing.T) {
	checkAppFiles(t, served(t, helperURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithURLHelper.test_nonascii_filenames
func TestServeStaticWithURLHelperNonasciiFilenames(t *testing.T) {
	checkNonasciiFilenames(t, served(t, helperURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithURLHelper.test_camelcase_filenames
func TestServeStaticWithURLHelperCamelcaseFilenames(t *testing.T) {
	checkCamelcaseFilenames(t, served(t, helperURLConf(t)))
}

// Ported from Django: tests/staticfiles_tests/test_views.py::TestServeStaticWithURLHelper.test_filename_with_percent_sign
func TestServeStaticWithURLHelperFilenameWithPercentSign(t *testing.T) {
	checkFilenameWithPercentSign(t, served(t, helperURLConf(t)))
}
