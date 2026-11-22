package auth_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/urls"
)

// loginRequiredURLs is the part of tests/auth_tests/urls.py that the
// LoginRequiredMiddleware tests use. A class-based view and a view function
// are the same kind of handler in Go, so both paths of each pair serve one
// handler.
func loginRequiredURLs() *http.ServeMux {
	mux := http.NewServeMux()
	public := auth.LoginNotRequired(emptyResponseView)
	mux.Handle("/public_view/", public)
	mux.Handle("/public_function_view/", public)
	mux.Handle("/protected_view/", emptyResponseView)
	mux.Handle("/protected_function_view/", emptyResponseView)
	custom := auth.LoginRequired("/custom_login/", auth.WithRedirectFieldName("step"))(emptyResponseView)
	mux.Handle("/login_required_cbv_view/", custom)
	mux.Handle("/login_required_decorator_view/", custom)
	return mux
}

// muxResolver returns the view mux serves a request with, or nil when no
// pattern of mux matches the request.
func muxResolver(mux *http.ServeMux) func(*http.Request) http.Handler {
	return func(r *http.Request) http.Handler {
		h, pattern := mux.Handler(r)
		if pattern == "" {
			return nil
		}
		return h
	}
}

// withLoginRequired serves mux behind LoginRequiredMiddleware, as Django's
// modify_settings appends the middleware to MIDDLEWARE.
func withLoginRequired(mux *http.ServeMux, opts ...auth.GuardOption) http.Handler {
	return auth.LoginRequiredMiddleware(muxResolver(mux), opts...)(mux)
}

// newLoginRequiredSite is the setup of TestLoginRequiredMiddleware: the user
// test_user and the views of loginRequiredURLs behind the middleware.
func newLoginRequiredSite(t *testing.T, db *orm.DB, opts ...auth.GuardOption) (*site, *auth.User) {
	t.Helper()
	user := createUser(t, db, "test_user", "test_password")
	return newSite(t, db, withLoginRequired(loginRequiredURLs(), opts...)), user
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_public_paths
func TestLoginRequiredMiddlewarePublicPaths(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, _ := newLoginRequiredSite(t, db)
		for _, path := range []string{"public_view", "public_function_view"} {
			djangotest.AssertEqual(t, s.Get("/"+path+"/").StatusCode, http.StatusOK)
		}
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_protected_paths
func TestLoginRequiredMiddlewareProtectedPaths(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, _ := newLoginRequiredSite(t, db)
		for _, path := range []string{"protected_view", "protected_function_view"} {
			resp := s.Get("/" + path + "/")
			djangotest.AssertRedirects(t, resp, auth.DefaultLoginURL+"?next=/"+path+"/",
				djangotest.FetchRedirectResponse(false))
		}
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_login_required_paths
func TestLoginRequiredMiddlewareLoginRequiredPaths(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, _ := newLoginRequiredSite(t, db)
		for _, path := range []string{"login_required_cbv_view", "login_required_decorator_view"} {
			resp := s.Get("/" + path + "/")
			djangotest.AssertRedirects(t, resp, "/custom_login/?step=/"+path+"/",
				djangotest.FetchRedirectResponse(false))
		}
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_non_existent_path
func TestLoginRequiredMiddlewareNonExistentPath(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, _ := newLoginRequiredSite(t, db)
		djangotest.AssertEqual(t, s.Get("/non_existent/").StatusCode, http.StatusNotFound)
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_paths_with_logged_in_user
func TestLoginRequiredMiddlewarePathsWithLoggedInUser(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, user := newLoginRequiredSite(t, db)
		s.ForceLogin(user)
		for _, path := range []string{
			"public_view", "public_function_view", "protected_view",
			"protected_function_view", "login_required_cbv_view", "login_required_decorator_view",
		} {
			djangotest.AssertEqual(t, s.Get("/"+path+"/").StatusCode, http.StatusOK)
		}
	})
}

// Django reads the login URL of the view function. The Go view sets it with
// the guard's login URL.
//
// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_get_login_url_from_view_func
func TestLoginRequiredMiddlewareGetLoginURLFromViewFunc(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		mux := http.NewServeMux()
		mux.Handle("/view/", auth.LoginRequired("/custom_login/")(emptyResponseView))
		s := newSite(t, db, withLoginRequired(mux))
		djangotest.AssertRedirects(t, s.Get("/view/"), "/custom_login/?next=/view/",
			djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_get_login_url_from_settings
func TestLoginRequiredMiddlewareGetLoginURLFromSettings(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginURL = "/settings_login/" })
		s, _ := newLoginRequiredSite(t, db)
		djangotest.AssertRedirects(t, s.Get("/protected_view/"), "/settings_login/?next=/protected_view/",
			djangotest.FetchRedirectResponse(false))
	})
}

// No Go counterpart: TestLoginRequiredMiddleware.test_get_login_url_no_login_url.
// An empty Config.LoginURL means DefaultLoginURL, so there is always a login
// URL to redirect to.

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_get_redirect_field_name_from_view_func
func TestLoginRequiredMiddlewareGetRedirectFieldNameFromViewFunc(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		mux := http.NewServeMux()
		mux.Handle("/view/", auth.LoginRequired("", auth.WithRedirectFieldName("next_page"))(emptyResponseView))
		s := newSite(t, db, withLoginRequired(mux))
		djangotest.AssertRedirects(t, s.Get("/view/"), auth.DefaultLoginURL+"?next_page=/view/",
			djangotest.FetchRedirectResponse(false))
	})
}

// Django's LoginRequiredMiddlewareSubclass sets redirect_field_name to
// "redirect_to". The Go middleware takes it as an option.
//
// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_login_url_resolve_logic
func TestLoginRequiredMiddlewareLoginURLResolveLogic(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginURL = "/settings_login/" })
		s, _ := newLoginRequiredSite(t, db, auth.WithRedirectFieldName("redirect_to"))
		for _, path := range []string{"login_required_cbv_view", "login_required_decorator_view"} {
			djangotest.AssertRedirects(t, s.Get("/"+path+"/"), "/custom_login/?step=/"+path+"/",
				djangotest.FetchRedirectResponse(false))
		}
		for _, path := range []string{"protected_view", "protected_function_view"} {
			djangotest.AssertRedirects(t, s.Get("/"+path+"/"), "/settings_login/?redirect_to=/"+path+"/",
				djangotest.FetchRedirectResponse(false))
		}
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_get_redirect_field_name_default
func TestLoginRequiredMiddlewareGetRedirectFieldNameDefault(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, _ := newLoginRequiredSite(t, db)
		djangotest.AssertRedirects(t, s.Get("/protected_view/"), auth.DefaultLoginURL+"?"+auth.DefaultRedirectFieldName+"=/protected_view/",
			djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_public_view_logged_in_performance
func TestLoginRequiredMiddlewarePublicViewLoggedInPerformance(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, user := newLoginRequiredSite(t, db)
		s.ForceLogin(user)
		var resp *djangotest.Response
		djangotest.AssertNumQueries(t, db, 0, func() { resp = s.Get("/public_view/") })
		djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_middleware.py::TestLoginRequiredMiddleware.test_protected_view_logged_in_performance
func TestLoginRequiredMiddlewareProtectedViewLoggedInPerformance(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s, user := newLoginRequiredSite(t, db)
		s.ForceLogin(user)
		var resp *djangotest.Response
		// The session and the user.
		djangotest.AssertNumQueries(t, db, 2, func() { resp = s.Get("/protected_view/") })
		djangotest.AssertEqual(t, resp.StatusCode, http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_decorators.py::LoginNotRequiredTestCase.test_callable
func TestLoginNotRequiredCallable(_ *testing.T) {
	auth.LoginNotRequired(&auth.LogoutView{})
}

// Ported from Django: tests/auth_tests/test_decorators.py::LoginNotRequiredTestCase.test_view
func TestLoginNotRequiredView(_ *testing.T) {
	auth.LoginNotRequired(emptyResponseView)
}

// Ported from Django: tests/auth_tests/test_decorators.py::LoginNotRequiredTestCase.test_decorator_marks_view_as_login_not_required
func TestLoginNotRequiredMarksView(t *testing.T) {
	view, ok := auth.LoginNotRequired(emptyResponseView).(auth.LoginOptOut)
	if !ok {
		t.Fatal("LoginNotRequired returned a handler without LoginRequired")
	}
	djangotest.AssertEqual(t, view.LoginRequired(), false)
}

// No Django test exists: Django copies the attributes of a view to its
// decorators. The Go middleware finds the opt-out through the Unwrap method of
// a wrapper such as csrf.Exempt, in either order.
func TestLoginRequiredMiddlewareSeesThroughWrappers(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		mux := http.NewServeMux()
		mux.Handle("/hook/", csrf.Exempt(auth.LoginNotRequired(emptyResponseView)))
		mux.Handle("/other_hook/", auth.LoginNotRequired(csrf.Exempt(emptyResponseView)))
		mux.Handle("/guarded/", csrf.Exempt(auth.LoginRequired("/custom_login/")(emptyResponseView)))
		s := newSite(t, db, withLoginRequired(mux))
		djangotest.AssertStatus(t, s.Get("/hook/"), http.StatusOK)
		djangotest.AssertStatus(t, s.Get("/other_hook/"), http.StatusOK)
		djangotest.AssertRedirects(t, s.Get("/guarded/"), "/custom_login/?next=/guarded/",
			djangotest.FetchRedirectResponse(false))
		if !csrf.IsExempt(auth.LoginNotRequired(csrf.Exempt(emptyResponseView))) {
			t.Error("csrf.IsExempt does not see the mark under LoginNotRequired")
		}
	})
}

// No Django test exists: the routes of auth.URLs carry the names of
// django.contrib.auth.urls, and their login page is public.
func TestURLs(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		router := urls.NewRouter(urls.Include("accounts/", "", auth.URLs()...)...)
		for name, want := range map[string]string{
			"login":                "/accounts/login/",
			"logout":               "/accounts/logout/",
			"password_change":      "/accounts/password_change/",
			"password_change_done": "/accounts/password_change/done/",
		} {
			got, err := router.Reverse(name)
			must(t, err)
			djangotest.AssertEqual(t, got, want)
		}
		s := newSite(t, db, auth.LoginRequiredMiddleware(router.HandlerFor)(router))
		djangotest.AssertStatus(t, s.Get(auth.DefaultLoginURL), http.StatusOK)
		djangotest.AssertRedirects(t, s.Get("/accounts/password_change/"),
			auth.DefaultLoginURL+"?next=/accounts/password_change/", djangotest.FetchRedirectResponse(false))
	})
}
