package auth_test

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// viewsURLs is the port of tests/auth_tests/urls.py for the view tests: the
// views of django.contrib.auth.urls at their Django paths, plus the variants
// the tests use. Named URLs of Django ("password_reset") are written as paths.
func viewsURLs() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/login/", &auth.LoginView{})
	mux.Handle("/logout/", &auth.LogoutView{})
	mux.Handle("/password_change/", &auth.PasswordChangeView{})
	mux.Handle("/password_change/done/", &auth.PasswordChangeDoneView{})
	mux.Handle("/password_change/custom/", &auth.PasswordChangeView{SuccessURL: "/custom/"})
	mux.Handle("/password_change/custom/named/", &auth.PasswordChangeView{SuccessURL: "/password_reset/"})
	mux.Handle("/logout/custom_query/", &auth.LogoutView{RedirectFieldName: "follow"})
	mux.Handle("/logout/next_page/", &auth.LogoutView{NextPage: "/somewhere/"})
	mux.Handle("/logout/next_page/named/", &auth.LogoutView{NextPage: "/password_reset/"})
	mux.Handle("/logout/allowed_hosts/", &auth.LogoutView{SuccessURLAllowedHosts: []string{"otherserver"}})
	mux.Handle("/login_required/", auth.LoginRequired("")(emptyResponseView))
	mux.Handle("/login_required_login_url/", auth.LoginRequired("/somewhere/")(emptyResponseView))
	mux.Handle("/login/redirect_authenticated_user_default/", &auth.LoginView{})
	mux.Handle("/login/redirect_authenticated_user/", &auth.LoginView{RedirectAuthenticatedUser: true})
	mux.Handle("/login/allowed_hosts/", &auth.LoginView{SuccessURLAllowedHosts: []string{"otherserver"}})
	mux.Handle("/login/get_default_redirect_url/", &auth.LoginView{NextPage: "/custom/"})
	mux.Handle("/login/next_page/", &auth.LoginView{NextPage: "/somewhere/"})
	mux.Handle("/login/next_page/named/", &auth.LoginView{NextPage: "/password_reset/"})
	mux.Handle("/logout_then_login/", auth.LogoutThenLogin(""))
	mux.Handle("/logout_then_login/custom/", auth.LogoutThenLogin("/custom/"))
	mux.Handle("/permission_required_redirect/", auth.PermissionRequired("unknown.permission")(emptyResponseView))
	mux.Handle("/permission_required_exception/",
		auth.PermissionRequired("unknown.permission", auth.WithRaiseException())(emptyResponseView))
	mux.Handle("/login_and_permission_required_exception/",
		auth.LoginRequired("")(auth.PermissionRequired("unknown.permission", auth.WithRaiseException())(emptyResponseView)))
	return mux
}

// viewsSite is the setup of Django's AuthViewsTestCase: the users testclient
// and staff with the password "password", and a client for viewsURLs.
type viewsSite struct {
	*site
	// u1 and u3 are the users of setUpTestData.
	u1, u3 *auth.User
}

// newViewsSite builds the site of the view tests on db. The password
// validators are off, as in Django's test settings.
func newViewsSite(t *testing.T, db *orm.DB) *viewsSite {
	t.Helper()
	return newViewsSiteServing(t, db, viewsURLs())
}

// newViewsSiteServing is newViewsSite for a site that serves handler, such
// as viewsURLs behind more middleware.
func newViewsSiteServing(t *testing.T, db *orm.DB, handler http.Handler) *viewsSite {
	t.Helper()
	noValidators(t)
	s := &viewsSite{site: newSite(t, db, handler)}
	s.u1 = createUserWithEmail(t, db, "testclient", "password", "testclient@example.com")
	s.u3 = createUserWithEmail(t, db, "staff", "password", "staffmember@example.com")
	return s
}

// noValidators turns the password validators off for the test, as Django's
// test settings leave AUTH_PASSWORD_VALIDATORS empty.
func noValidators(t *testing.T) {
	t.Helper()
	cfg := auth.CurrentConfig()
	cfg.PasswordValidators = []auth.PasswordValidator{}
	t.Cleanup(auth.Configure(cfg))
}

// configure applies change to the auth settings for the duration of the test
// (Django's override_settings).
func configure(t *testing.T, change func(*auth.Config)) {
	t.Helper()
	cfg := auth.CurrentConfig()
	change(&cfg)
	t.Cleanup(auth.Configure(cfg))
}

// createUserWithEmail inserts an active user (Django's create_user).
func createUserWithEmail(t *testing.T, db *orm.DB, username, password, email string) *auth.User {
	t.Helper()
	u, err := auth.CreateUser(t.Context(), db, username, email, password, false, false, true)
	must(t, err)
	return u
}

// login posts the credentials to loginURL and checks that the session holds
// a user afterwards (AuthViewsTestCase.login).
func (s *viewsSite) login(t *testing.T, username, password, loginURL string) *djangotest.Response {
	t.Helper()
	resp := s.Post(loginURL, url.Values{"username": {username}, "password": {password}})
	if _, ok := s.Session().Get(auth.SessionKey); !ok {
		t.Fatalf("POST %s as %q: the session holds no user (status %d)", loginURL, username, resp.StatusCode)
	}
	return resp
}

// assertLoggedOut fails when the session still holds a user
// (confirm_logged_out).
func (s *viewsSite) assertLoggedOut(t *testing.T) {
	t.Helper()
	if _, ok := s.Session().Get(auth.SessionKey); ok {
		t.Fatal("the session still holds a user")
	}
}

// assertFormError fails unless msg is among the form errors in the context of
// the response (AuthViewsTestCase.assertFormError).
func assertFormError(t *testing.T, resp *djangotest.Response, msg string) {
	t.Helper()
	raw, ok := resp.Context("errors")
	if !ok {
		t.Fatalf("the response has no errors in its context")
	}
	for _, msgs := range raw.(map[string][]string) {
		if slices.Contains(msgs, msg) {
			return
		}
	}
	t.Fatalf("%q is not among the form errors %v", msg, raw)
}

// changePassword posts the password change form to path.
func (s *viewsSite) changePassword(path, old, new1, new2 string) *djangotest.Response {
	return s.Post(path, url.Values{"old_password": {old}, "new_password1": {new1}, "new_password2": {new2}})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_fails_with_invalid_old_password
func TestDjangoPasswordChangeFailsWithInvalidOldPassword(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.changePassword("/password_change/", "donuts", "password1", "password1")
		assertFormError(t, resp, "Your old password was entered incorrectly. Please enter it again.")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_fails_with_mismatched_passwords
func TestDjangoPasswordChangeFailsWithMismatchedPasswords(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.changePassword("/password_change/", "password", "password1", "donuts")
		assertFormError(t, resp, "The two password fields didn’t match.")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_succeeds
func TestDjangoPasswordChangeSucceeds(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		s.changePassword("/password_change/", "password", "password1", "password1")
		// fail_login: the old password no longer works.
		resp := s.Post("/login/", url.Values{"username": {"testclient"}, "password": {"password"}})
		assertFormError(t, resp,
			"Please enter a correct username and password. Note that both fields may be case-sensitive.")
		s.login(t, "testclient", "password1", "/login/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_done_succeeds
func TestDjangoPasswordChangeDoneSucceeds(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.changePassword("/password_change/", "password", "password1", "password1")
		djangotest.AssertRedirects(t, resp, "/password_change/done/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_done_fails
func TestDjangoPasswordChangeDoneFails(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginURL = "/login/" })
		s := newViewsSite(t, db)
		resp := s.Get("/password_change/done/")
		djangotest.AssertRedirects(t, resp, "/login/?next=/password_change/done/",
			djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::SessionAuthenticationTests.test_user_password_change_updates_session
func TestDjangoUserPasswordChangeUpdatesSession(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		originalKey := s.Session().Key()
		resp := s.changePassword("/password_change/", "password", "password1", "password1")
		// If the hash is not updated, the done page redirects to the login.
		djangotest.AssertRedirects(t, resp, "/password_change/done/")
		djangotest.AssertNotEqual(t, s.Session().Key(), originalKey)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_security_check
func TestDjangoLoginSecurityCheck(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		badURLs := []string{
			"http://example.com",
			"http:///example.com",
			"https://example.com",
			"ftp://example.com",
			"///example.com",
			"//example.com",
			`javascript:alert("XSS")`,
		}
		for _, bad := range badURLs {
			resp := s.Post("/login/?next="+pythonQuote(bad),
				url.Values{"username": {"testclient"}, "password": {"password"}})
			djangotest.AssertStatus(t, resp, http.StatusFound)
			if loc := resp.Header.Get("Location"); strings.Contains(loc, bad) {
				t.Errorf("%s should be blocked, Location = %q", bad, loc)
			}
		}
		goodURLs := []string{
			"/view/?param=http://example.com",
			"/view/?param=https://example.com",
			"/view?param=ftp://example.com",
			"view/?param=//example.com",
			"https://testserver/",
			"HTTPS://testserver/",
			"//testserver/",
			"/url%20with%20spaces/",
		}
		for _, good := range goodURLs {
			resp := s.Post("/login/?next="+pythonQuote(good),
				url.Values{"username": {"testclient"}, "password": {"password"}})
			djangotest.AssertStatus(t, resp, http.StatusFound)
			if loc := resp.Header.Get("Location"); !strings.Contains(loc, good) {
				t.Errorf("%s should be allowed, Location = %q", good, loc)
			}
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_default
func TestDjangoLogoutDefault(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/", nil)
		djangotest.AssertContains(t, resp, "Logged out")
		s.assertLoggedOut(t)
	})
}

// passwordChangeData is the form of a valid password change.
var passwordChangeData = url.Values{
	"old_password": {"password"}, "new_password1": {"password1"}, "new_password2": {"password1"},
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_redirect_default
func TestDjangoPasswordChangeRedirectDefault(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/password_change/", passwordChangeData)
		djangotest.AssertRedirects(t, resp, "/password_change/done/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_redirect_custom
func TestDjangoPasswordChangeRedirectCustom(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/password_change/custom/", passwordChangeData)
		djangotest.AssertRedirects(t, resp, "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// The Django name "password_reset" is written as its path "/password_reset/".
//
// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_password_change_redirect_custom_named
func TestDjangoPasswordChangeRedirectCustomNamed(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/password_change/custom/named/", passwordChangeData)
		djangotest.AssertRedirects(t, resp, "/password_reset/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::ChangePasswordTest.test_access_under_login_required_middleware
func TestDjangoPasswordChangeAccessUnderLoginRequiredMiddleware(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSiteServing(t, db, withLoginRequired(viewsURLs()))
		resp := s.Post("/password_change/", passwordChangeData)
		djangotest.AssertRedirects(t, resp, auth.DefaultLoginURL+"?next=/password_change/",
			djangotest.FetchRedirectResponse(false))

		s.login(t, "testclient", "password", "/login/")
		resp = s.Post("/password_change/", passwordChangeData)
		djangotest.AssertRedirects(t, resp, "/password_change/done/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_current_site_in_context_after_login
//
// Go has no sites framework and no form object: the context holds the request
// host as "site_name" and the form errors as "errors".
func TestDjangoLoginCurrentSiteInContextAfterLogin(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.Get("/login/")
		djangotest.AssertStatus(t, resp, http.StatusOK)
		name, _ := resp.Context("site_name")
		djangotest.AssertEqual(t, name, any("testserver"))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_security_check_https
func TestDjangoLoginSecurityCheckHTTPS(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		nonHTTPSNext := "http://testserver/path"
		notSecured := "/login/?next=" + pythonQuote(nonHTTPSNext)
		resp := s.Post(notSecured, url.Values{"username": {"testclient"}, "password": {"password"}},
			djangotest.Secure())
		djangotest.AssertStatus(t, resp, http.StatusFound)
		loc := resp.Header.Get("Location")
		djangotest.AssertNotEqual(t, loc, nonHTTPSNext)
		djangotest.AssertEqual(t, loc, auth.DefaultLoginRedirectURL)
	})
}

// Django drives the middleware by hand with bare requests; here the login view
// runs behind csrf.Middleware and the client reads the cookie.
//
// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_csrf_rotate
func TestDjangoLoginCSRFRotate(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		noValidators(t)
		s := newSite(t, db, csrf.Middleware(viewsURLs()))
		createUserWithEmail(t, db, "testclient", "password", "testclient@example.com")

		// Do a GET to establish a CSRF token.
		s.Get("/login/")
		cookie, ok := s.Cookie(csrf.DefaultCookieName)
		if !ok {
			t.Fatal("the login page set no CSRF cookie")
		}
		token1 := cookie.Value

		resp := s.Post("/login/", url.Values{
			"username": {"testclient"}, "password": {"password"}, "csrfmiddlewaretoken": {token1},
		})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		cookie, ok = s.Cookie(csrf.DefaultCookieName)
		if !ok {
			t.Fatal("the login response set no CSRF cookie")
		}
		djangotest.AssertNotEqual(t, cookie.Value, token1)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_session_key_flushed_on_login
func TestDjangoSessionKeyFlushedOnLogin(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		original := s.Session().Key()

		s.login(t, "staff", "password", "/login/")
		djangotest.AssertNotEqual(t, s.Session().Key(), original)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_session_key_flushed_on_login_after_password_change
func TestDjangoSessionKeyFlushedOnLoginAfterPasswordChange(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		original := s.Session().Key()

		// If no password change, session key should not be flushed.
		s.login(t, "testclient", "password", "/login/")
		djangotest.AssertEqual(t, s.Session().Key(), original)

		setPassword(t, db, s.u1, "foobar")

		s.login(t, "testclient", "foobar", "/login/")
		djangotest.AssertNotEqual(t, s.Session().Key(), original)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_session_without_hash_session_key
func TestDjangoLoginSessionWithoutHashSessionKey(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		sess := s.Session()
		sess.Set(auth.SessionKey, strconv.FormatInt(s.u1.ID, 10))
		s.SaveSession(sess)
		original := sess.Key()

		s.login(t, "testclient", "password", "/login/")
		djangotest.AssertNotEqual(t, s.Session().Key(), original)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_get_default_redirect_url
func TestDjangoLoginGetDefaultRedirectURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.login(t, "testclient", "password", "/login/get_default_redirect_url/")
		djangotest.AssertRedirects(t, resp, "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_next_page
func TestDjangoLoginNextPage(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.login(t, "testclient", "password", "/login/next_page/")
		djangotest.AssertRedirects(t, resp, "/somewhere/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_named_next_page_named
func TestDjangoLoginNamedNextPageNamed(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.login(t, "testclient", "password", "/login/next_page/named/")
		djangotest.AssertRedirects(t, resp, "/password_reset/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_next_page_overrides_login_redirect_url_setting
func TestDjangoLoginNextPageOverridesLoginRedirectURLSetting(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginRedirectURL = "/custom/" })
		s := newViewsSite(t, db)
		resp := s.login(t, "testclient", "password", "/login/next_page/")
		djangotest.AssertRedirects(t, resp, "/somewhere/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_redirect_url_overrides_next_page
func TestDjangoLoginRedirectURLOverridesNextPage(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.login(t, "testclient", "password", "/login/next_page/?next=/test/")
		djangotest.AssertRedirects(t, resp, "/test/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_login_redirect_url_overrides_get_default_redirect_url
func TestDjangoLoginRedirectURLOverridesGetDefaultRedirectURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.login(t, "testclient", "password", "/login/get_default_redirect_url/?next=/test/")
		djangotest.AssertRedirects(t, resp, "/test/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginTest.test_access_under_login_required_middleware
func TestDjangoLoginAccessUnderLoginRequiredMiddleware(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSiteServing(t, db, withLoginRequired(viewsURLs()))
		djangotest.AssertStatus(t, s.Get("/login/"), http.StatusOK)
	})
}

// assertLoginURLEquals is LoginURLSettings.assertLoginURLEquals. The named
// URL "login" of Django is written as "/login/".
func assertLoginURLEquals(t *testing.T, db *orm.DB, loginURL, want string) {
	t.Helper()
	configure(t, func(c *auth.Config) { c.LoginURL = loginURL })
	s := newViewsSite(t, db)
	djangotest.AssertRedirects(t, s.Get("/login_required/"), want, djangotest.FetchRedirectResponse(false))
}

// Ported from Django: tests/auth_tests/test_views.py::LoginURLSettings.test_standard_login_url
func TestDjangoLoginURLSettingsStandardLoginURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		assertLoginURLEquals(t, db, "/login/", "/login/?next=/login_required/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginURLSettings.test_named_login_url
//
// Go has no reverse(): the setting holds the path.
func TestDjangoLoginURLSettingsNamedLoginURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		assertLoginURLEquals(t, db, "/login/", "/login/?next=/login_required/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginURLSettings.test_remote_login_url
func TestDjangoLoginURLSettingsRemoteLoginURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		quotedNext := pythonQuote("http://testserver/login_required/")
		assertLoginURLEquals(t, db, "http://remote.example.com/login",
			"http://remote.example.com/login?next="+quotedNext)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginURLSettings.test_https_login_url
func TestDjangoLoginURLSettingsHTTPSLoginURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		quotedNext := pythonQuote("http://testserver/login_required/")
		assertLoginURLEquals(t, db, "https:///login/", "https:///login/?next="+quotedNext)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginURLSettings.test_login_url_with_querystring
func TestDjangoLoginURLSettingsLoginURLWithQuerystring(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		assertLoginURLEquals(t, db, "/login/?pretty=1", "/login/?pretty=1&next=/login_required/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginURLSettings.test_remote_login_url_with_next_querystring
func TestDjangoLoginURLSettingsRemoteLoginURLWithNextQuerystring(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		quotedNext := pythonQuote("http://testserver/login_required/")
		assertLoginURLEquals(t, db, "http://remote.example.com/login/?next=/default/",
			"http://remote.example.com/login/?next="+quotedNext)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginURLSettings.test_lazy_login_url
//
// Go has no lazy reverse(): the setting holds the path.
func TestDjangoLoginURLSettingsLazyLoginURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		assertLoginURLEquals(t, db, "/login/", "/login/?next=/login_required/")
	})
}

// assertLoginRedirectURLEqual is LoginRedirectUrlTest.assertLoginRedirectURLEqual.
func assertLoginRedirectURLEqual(t *testing.T, db *orm.DB, want string) {
	t.Helper()
	s := newViewsSite(t, db)
	resp := s.login(t, "testclient", "password", "/login/")
	djangotest.AssertRedirects(t, resp, want, djangotest.FetchRedirectResponse(false))
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectUrlTest.test_default
func TestDjangoLoginRedirectURLDefault(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		assertLoginRedirectURLEqual(t, db, "/accounts/profile/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectUrlTest.test_custom
func TestDjangoLoginRedirectURLCustom(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginRedirectURL = "/custom/" })
		assertLoginRedirectURLEqual(t, db, "/custom/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectUrlTest.test_named
//
// Go has no reverse(): the setting holds the path.
func TestDjangoLoginRedirectURLNamed(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginRedirectURL = "/password_reset/" })
		assertLoginRedirectURLEqual(t, db, "/password_reset/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectUrlTest.test_remote
func TestDjangoLoginRedirectURLRemote(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginRedirectURL = "http://remote.example.com/welcome/" })
		assertLoginRedirectURLEqual(t, db, "http://remote.example.com/welcome/")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::RedirectToLoginTests.test_redirect_to_login_with_lazy
//
// Go has no lazy reverse(): the setting holds the path.
func TestDjangoRedirectToLoginWithLazy(t *testing.T) {
	configure(t, func(c *auth.Config) { c.LoginURL = "/login/" })
	djangotest.AssertEqual(t, auth.RedirectToLoginURL("/else/where/", "", "next"), "/login/?next=/else/where/")
}

// Ported from Django: tests/auth_tests/test_views.py::RedirectToLoginTests.test_redirect_to_login_with_lazy_and_unicode
func TestDjangoRedirectToLoginWithLazyAndUnicode(t *testing.T) {
	configure(t, func(c *auth.Config) { c.LoginURL = "/login/" })
	djangotest.AssertEqual(t, auth.RedirectToLoginURL("/else/where/झ/", "", "next"),
		"/login/?next=/else/where/%E0%A4%9D/")
}

// Django calls logout_then_login with a bare request; here the handler is
// mounted and driven through the client.
//
// Ported from Django: tests/auth_tests/test_views.py::LogoutThenLoginTests.test_default_logout_then_login
func TestDjangoDefaultLogoutThenLogin(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginURL = "/login/" })
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout_then_login/", nil)
		s.assertLoggedOut(t)
		djangotest.AssertRedirects(t, resp, "/login/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutThenLoginTests.test_logout_then_login_with_custom_login
func TestDjangoLogoutThenLoginWithCustomLogin(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout_then_login/custom/", nil)
		s.assertLoggedOut(t)
		djangotest.AssertRedirects(t, resp, "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutThenLoginTests.test_default_logout_then_login_get
func TestDjangoDefaultLogoutThenLoginGet(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginURL = "/login/" })
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		djangotest.AssertStatus(t, s.Get("/logout_then_login/"), http.StatusMethodNotAllowed)
	})
}

const (
	dontRedirectURL = "/login/redirect_authenticated_user_default/"
	doRedirectURL   = "/login/redirect_authenticated_user/"
)

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_default
func TestDjangoLoginRedirectAuthenticatedUserDefault(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Get(dontRedirectURL)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		next, _ := resp.Context("next")
		djangotest.AssertEqual(t, next, any(""))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_guest
func TestDjangoLoginRedirectAuthenticatedUserGuest(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		djangotest.AssertStatus(t, s.Get(doRedirectURL), http.StatusOK)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_redirect
func TestDjangoLoginRedirectAuthenticatedUserRedirect(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		djangotest.AssertRedirects(t, s.Get(doRedirectURL), "/accounts/profile/",
			djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_redirect_url
func TestDjangoLoginRedirectAuthenticatedUserRedirectURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginRedirectURL = "/custom/" })
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		djangotest.AssertRedirects(t, s.Get(doRedirectURL), "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_redirect_param
func TestDjangoLoginRedirectAuthenticatedUserRedirectParam(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		djangotest.AssertRedirects(t, s.Get(doRedirectURL+"?next=/custom_next/"), "/custom_next/",
			djangotest.FetchRedirectResponse(false))
	})
}

// Django raises ValueError; a Go handler answers 500 with the message as the
// body.
//
// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_redirect_loop
func TestDjangoLoginRedirectAuthenticatedUserRedirectLoop(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		msg := "Redirection loop for authenticated user detected. Check that " +
			"your LOGIN_REDIRECT_URL doesn't point to a login page."
		configure(t, func(c *auth.Config) { c.LoginRedirectURL = doRedirectURL })
		for _, u := range []string{doRedirectURL, doRedirectURL + "?bla=2"} {
			resp := s.Get(u)
			djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
			if !strings.Contains(string(resp.Body), msg) {
				t.Errorf("GET %s: body = %q, want it to contain %q", u, resp.Body, msg)
			}
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_permission_required_not_logged_in
func TestDjangoLoginRedirectAuthenticatedUserPermissionRequiredNotLoggedIn(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LoginURL = doRedirectURL })
		s := newViewsSite(t, db)
		// Redirected to login.
		djangotest.AssertStatus(t, s.Get("/permission_required_redirect/", djangotest.Follow()), http.StatusOK)
		// Exception raised.
		djangotest.AssertStatus(t, s.Get("/permission_required_exception/", djangotest.Follow()),
			http.StatusForbidden)
		// Redirected to login.
		djangotest.AssertStatus(t, s.Get("/login_and_permission_required_exception/", djangotest.Follow()),
			http.StatusOK)
	})
}

// Django's permission_required without raise_exception redirects a logged-in
// user to the login page, which redirects back: the client raises
// RedirectCycleError. auth.PermissionRequired answers a logged-in user with 403
// instead, because a second login cannot change the outcome, so the Go test
// asserts 403 for all three views.
//
// Ported from Django: tests/auth_tests/test_views.py::LoginRedirectAuthenticatedUser.test_permission_required_logged_in
func TestDjangoLoginRedirectAuthenticatedUserPermissionRequiredLoggedIn(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		configure(t, func(c *auth.Config) { c.LoginURL = doRedirectURL })
		for _, p := range []string{
			"/permission_required_redirect/",
			"/permission_required_exception/",
			"/login_and_permission_required_exception/",
		} {
			djangotest.AssertStatus(t, s.Get(p, djangotest.Follow()), http.StatusForbidden)
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginSuccessURLAllowedHostsTest.test_success_url_allowed_hosts_same_host
func TestDjangoLoginSuccessURLAllowedHostsSameHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.Post("/login/allowed_hosts/", url.Values{
			"username": {"testclient"}, "password": {"password"}, "next": {"https://testserver/home"},
		})
		if _, ok := s.Session().Get(auth.SessionKey); !ok {
			t.Error("the session holds no user")
		}
		djangotest.AssertRedirects(t, resp, "https://testserver/home", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginSuccessURLAllowedHostsTest.test_success_url_allowed_hosts_safe_host
func TestDjangoLoginSuccessURLAllowedHostsSafeHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.Post("/login/allowed_hosts/", url.Values{
			"username": {"testclient"}, "password": {"password"}, "next": {"https://otherserver/home"},
		})
		if _, ok := s.Session().Get(auth.SessionKey); !ok {
			t.Error("the session holds no user")
		}
		djangotest.AssertRedirects(t, resp, "https://otherserver/home", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LoginSuccessURLAllowedHostsTest.test_success_url_allowed_hosts_unsafe_host
func TestDjangoLoginSuccessURLAllowedHostsUnsafeHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.Post("/login/allowed_hosts/", url.Values{
			"username": {"testclient"}, "password": {"password"}, "next": {"https://evil/home"},
		})
		if _, ok := s.Session().Get(auth.SessionKey); !ok {
			t.Error("the session holds no user")
		}
		djangotest.AssertRedirects(t, resp, "/accounts/profile/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_with_post
func TestDjangoLogoutWithPost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/", nil)
		djangotest.AssertContains(t, resp, "Logged out")
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_14377
//
// Go has no sites framework: the context holds "site_name" in place of "site".
func TestDjango14377(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/", nil)
		if _, ok := resp.Context("site_name"); !ok {
			t.Error("site_name is not in the context")
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_doesnt_cache
func TestDjangoLogoutDoesntCache(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.Post("/logout/", nil)
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("Cache-Control = %q, want no-store", cc)
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_with_overridden_redirect_url
func TestDjangoLogoutWithOverriddenRedirectURL(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/next_page/", nil)
		djangotest.AssertRedirects(t, resp, "/somewhere/", djangotest.FetchRedirectResponse(false))

		resp = s.Post("/logout/next_page/?next=/login/", nil)
		djangotest.AssertRedirects(t, resp, "/login/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_with_next_page_specified
func TestDjangoLogoutWithNextPageSpecified(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/next_page/", nil)
		djangotest.AssertRedirects(t, resp, "/somewhere/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_with_redirect_argument
func TestDjangoLogoutWithRedirectArgument(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/?next=/login/", nil)
		djangotest.AssertRedirects(t, resp, "/login/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_with_custom_redirect_argument
func TestDjangoLogoutWithCustomRedirectArgument(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/custom_query/?follow=/somewhere/", nil)
		djangotest.AssertRedirects(t, resp, "/somewhere/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_with_named_redirect
//
// Go has no reverse(): the view holds the path.
func TestDjangoLogoutWithNamedRedirect(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/next_page/named/", nil)
		djangotest.AssertRedirects(t, resp, "/password_reset/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_success_url_allowed_hosts_same_host
func TestDjangoLogoutSuccessURLAllowedHostsSameHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/allowed_hosts/?next=https://testserver/", nil)
		djangotest.AssertRedirects(t, resp, "https://testserver/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_success_url_allowed_hosts_safe_host
func TestDjangoLogoutSuccessURLAllowedHostsSafeHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/allowed_hosts/?next=https://otherserver/", nil)
		djangotest.AssertRedirects(t, resp, "https://otherserver/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_success_url_allowed_hosts_unsafe_host
func TestDjangoLogoutSuccessURLAllowedHostsUnsafeHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/allowed_hosts/?next=https://evil/", nil)
		djangotest.AssertRedirects(t, resp, "/logout/allowed_hosts/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_security_check
func TestDjangoLogoutSecurityCheck(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		badURLs := []string{
			"http://example.com",
			"http:///example.com",
			"https://example.com",
			"ftp://example.com",
			"///example.com",
			"//example.com",
			`javascript:alert("XSS")`,
		}
		for _, bad := range badURLs {
			s.login(t, "testclient", "password", "/login/")
			resp := s.Post("/logout/?next="+pythonQuote(bad), nil)
			djangotest.AssertStatus(t, resp, http.StatusFound)
			if loc := resp.Header.Get("Location"); strings.Contains(loc, bad) {
				t.Errorf("%s should be blocked, Location = %q", bad, loc)
			}
			s.assertLoggedOut(t)
		}
		goodURLs := []string{
			"/view/?param=http://example.com",
			"/view/?param=https://example.com",
			"/view?param=ftp://example.com",
			"view/?param=//example.com",
			"https://testserver/",
			"HTTPS://testserver/",
			"//testserver/",
			"/url%20with%20spaces/",
		}
		for _, good := range goodURLs {
			s.login(t, "testclient", "password", "/login/")
			resp := s.Post("/logout/?next="+pythonQuote(good), nil)
			djangotest.AssertStatus(t, resp, http.StatusFound)
			if loc := resp.Header.Get("Location"); !strings.Contains(loc, good) {
				t.Errorf("%s should be allowed, Location = %q", good, loc)
			}
			s.assertLoggedOut(t)
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_security_check_https
func TestDjangoLogoutSecurityCheckHTTPS(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/?next="+pythonQuote("http://testserver/"), nil, djangotest.Secure())
		djangotest.AssertRedirects(t, resp, "/logout/", djangotest.FetchRedirectResponse(false))
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_redirect_url_setting
func TestDjangoLogoutRedirectURLSetting(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LogoutRedirectURL = "/custom/" })
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/", nil)
		djangotest.AssertRedirects(t, resp, "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_redirect_url_setting_allowed_hosts_unsafe_host
func TestDjangoLogoutRedirectURLSettingAllowedHostsUnsafeHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LogoutRedirectURL = "/custom/" })
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/allowed_hosts/?next=https://evil/", nil)
		djangotest.AssertRedirects(t, resp, "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_logout_redirect_url_named_setting
//
// Go has no reverse(): the setting holds the path of the logout view.
func TestDjangoLogoutRedirectURLNamedSetting(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		configure(t, func(c *auth.Config) { c.LogoutRedirectURL = "/logout/" })
		s := newViewsSite(t, db)
		s.login(t, "testclient", "password", "/login/")
		resp := s.Post("/logout/", nil)
		djangotest.AssertContains(t, resp, "Logged out")
		s.assertLoggedOut(t)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::LogoutTest.test_access_under_login_required_middleware
func TestDjangoLogoutAccessUnderLoginRequiredMiddleware(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSiteServing(t, db, withLoginRequired(viewsURLs()))
		resp := s.Post("/logout/", nil)
		djangotest.AssertRedirects(t, resp, auth.DefaultLoginURL+"?next=/logout/",
			djangotest.FetchRedirectResponse(false))

		s.login(t, "testclient", "password", "/login/")
		djangotest.AssertStatus(t, s.Post("/logout/", nil), http.StatusOK)
	})
}

// The logout view takes POST only since Django 5; a GET answers 405.
func TestLogoutGetNotAllowed(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.Get("/logout/")
		djangotest.AssertStatus(t, resp, http.StatusMethodNotAllowed)
		djangotest.AssertEqual(t, resp.Header.Get("Allow"), "POST, OPTIONS")
	})
}

// An empty submission shows the required message for both fields.
func TestLoginEmptyFieldsRequired(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		resp := s.Post("/login/", url.Values{"username": {""}, "password": {""}})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		raw, _ := resp.Context("field_errors")
		fields, _ := raw.(map[string][]string)
		want := []string{"This field is required."}
		djangotest.AssertSequenceEqual(t, fields["username"], want)
		djangotest.AssertSequenceEqual(t, fields["password"], want)
		djangotest.AssertContains(t, resp, "This field is required.", djangotest.Count(2))
	})
}

// A user with is_active false logs in only under AllowAllUsersModelBackend,
// and the form then refuses with the inactive message.
func TestLoginInactiveUserShowsInactiveMessage(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		useBackends(t, auth.AllowAllUsersModelBackend{})
		s := newViewsSite(t, db)
		_, err := auth.CreateUser(t.Context(), db, "inactive", "inactive@example.com", "password", false, false, false)
		must(t, err)
		resp := s.Post("/login/", url.Values{"username": {"inactive"}, "password": {"password"}})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		assertFormError(t, resp, "This account is inactive.")
		s.assertLoggedOut(t)
	})
}

// The login page and the login redirect must not be cached.
func TestLoginResponsesAreNotCached(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		s := newViewsSite(t, db)
		for _, resp := range []*djangotest.Response{
			s.Get("/login/"),
			s.Post("/login/", url.Values{"username": {"testclient"}, "password": {"password"}}),
		} {
			if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
				t.Errorf("status %d: Cache-Control = %q, want no-store", resp.StatusCode, cc)
			}
		}
	})
}

// pythonQuote is Python's urllib.parse.quote with its default safe "/".
func pythonQuote(s string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.IndexByte("_.-~/", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0f])
	}
	return b.String()
}
