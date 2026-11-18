package auth_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
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
	noValidators(t)
	s := &viewsSite{site: newSite(t, db, viewsURLs())}
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
