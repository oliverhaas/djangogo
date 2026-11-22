package auth_test

// Ports of PasswordResetTest and AuthViewNamedURLTests of
// tests/auth_tests/test_views.py. The views render the test templates of
// tests/auth_tests/templates, as Django's AUTH_TEMPLATES do.

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/mail"
	"github.com/oliverhaas/djangogo/middleware"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/urls"
)

// resetViewsURLs is viewsURLs plus the password reset views of
// tests/auth_tests/urls.py. Where Django reverses a name, the views get the
// path as SuccessURL.
func resetViewsURLs(t *testing.T) *http.ServeMux {
	t.Helper()
	engine := resetTestTemplates(t)
	mux := viewsURLs()
	mux.Handle("/password_reset/", &auth.PasswordResetView{Templates: engine})
	mux.Handle("/password_reset/done/", &auth.PasswordResetDoneView{Templates: engine})
	mux.Handle("/reset/{uidb64}/{token}/{$}", &auth.PasswordResetConfirmView{Templates: engine})
	mux.Handle("/reset/done/{$}", &auth.PasswordResetCompleteView{Templates: engine})
	mux.Handle("/password_reset_from_email/",
		&auth.PasswordResetView{Templates: engine, FromEmail: "staffmember@example.com"})
	mux.Handle("/password_reset_extra_email_context/", &auth.PasswordResetView{
		Templates:         engine,
		ExtraEmailContext: map[string]any{"greeting": "Hello!", "domain": "custom.example.com"},
	})
	mux.Handle("/password_reset/custom_redirect/", &auth.PasswordResetView{Templates: engine, SuccessURL: "/custom/"})
	mux.Handle("/password_reset/custom_redirect/named/",
		&auth.PasswordResetView{Templates: engine, SuccessURL: "/password_reset/"})
	mux.Handle("/password_reset/html_email_template/", &auth.PasswordResetView{
		Templates:             engine,
		HTMLEmailTemplateName: "registration/html_password_reset_email.html",
	})
	mux.Handle("/reset/custom/{uidb64}/{token}/{$}",
		&auth.PasswordResetConfirmView{Templates: engine, SuccessURL: "/custom/"})
	mux.Handle("/reset/custom/named/{uidb64}/{token}/{$}",
		&auth.PasswordResetConfirmView{Templates: engine, SuccessURL: "/password_reset/"})
	mux.Handle("/reset/custom/token/{uidb64}/{token}/{$}", &auth.PasswordResetConfirmView{
		Templates:     engine,
		ResetURLToken: "set-passwordcustom",
		SuccessURL:    "/reset/done/",
	})
	mux.Handle("/reset/post_reset_login/{uidb64}/{token}/{$}",
		&auth.PasswordResetConfirmView{Templates: engine, PostResetLogin: true, SuccessURL: "/reset/done/"})
	mux.Handle("/reset/post_reset_login_custom_backend/{uidb64}/{token}/{$}", &auth.PasswordResetConfirmView{
		Templates:             engine,
		PostResetLogin:        true,
		PostResetLoginBackend: auth.AllowAllUsersModelBackend{}.Name(),
		SuccessURL:            "/reset/done/",
	})
	mux.Handle("/reset/missing_parameters/{$}", &auth.PasswordResetConfirmView{Templates: engine})
	return mux
}

// resetClient is Django's PasswordResetConfirmClient: before a request to a
// reset link it puts the token of the link into the session and replaces it
// with resetURLToken, as the redirect of the confirmation view does.
type resetClient struct {
	*viewsSite
	// resetURLToken replaces the token in the path.
	resetURLToken string
}

// resetTokenRE is the pattern of extract_token_from_url.
var resetTokenRE = regexp.MustCompile(`/reset/.*/(.+?)/`)

// confirmPath stores the token of path in the session and returns the path
// with resetURLToken in its place. A path without a token stays as it is.
func (c *resetClient) confirmPath(path string) string {
	m := resetTokenRE.FindStringSubmatch(path)
	if m == nil {
		return path
	}
	sess := c.Session()
	sess.Set(auth.InternalResetSessionToken, m[1])
	c.SaveSession(sess)
	return strings.ReplaceAll(path, m[1], c.resetURLToken)
}

// Get sends a GET request to path after confirmPath.
func (c *resetClient) Get(path string, opts ...djangotest.RequestOption) *djangotest.Response {
	return c.viewsSite.Get(c.confirmPath(path), opts...)
}

// Post sends a POST request to path after confirmPath.
func (c *resetClient) Post(path string, data any, opts ...djangotest.RequestOption) *djangotest.Response {
	return c.viewsSite.Post(c.confirmPath(path), data, opts...)
}

// newResetClient is the setup of PasswordResetTest: the users of
// AuthViewsTestCase, a PasswordResetConfirmClient for handler and the mail
// outbox.
func newResetClient(t *testing.T, db *orm.DB, handler http.Handler) *resetClient {
	t.Helper()
	djangotest.LocmemMail(t)
	return &resetClient{viewsSite: newViewsSiteServing(t, db, handler), resetURLToken: auth.DefaultResetURLToken}
}

// resetViewsTest runs fn on each database with a resetClient for
// resetViewsURLs.
func resetViewsTest(t *testing.T, fn func(t *testing.T, c *resetClient)) {
	t.Helper()
	eachDB(t, func(t *testing.T, db *orm.DB) {
		fn(t, newResetClient(t, db, resetViewsURLs(t)))
	})
}

// newPasswordData is the POST data of the tests that set "anewpassword".
var newPasswordData = url.Values{"new_password1": {"anewpassword"}, "new_password2": {"anewpassword"}}

// signupURLRE is the pattern of _read_signup_email.
var signupURLRE = regexp.MustCompile(`https?://[^/]*(/.*reset/\S*)`)

// confirmStart asks for a reset email for staffmember@example.com and returns
// the link in it and its path (_test_confirm_start).
func (c *resetClient) confirmStart(t *testing.T) (link, path string) {
	t.Helper()
	c.Post("/password_reset/", url.Values{"email": {"staffmember@example.com"}})
	outbox := mail.Outbox()
	if len(outbox) != 1 {
		t.Fatalf("the outbox holds %d messages, want 1", len(outbox))
	}
	return readSignupEmail(t, outbox[0])
}

// readSignupEmail returns the reset link in msg and its path
// (_read_signup_email).
func readSignupEmail(t *testing.T, msg *mail.Message) (link, path string) {
	t.Helper()
	m := signupURLRE.FindStringSubmatch(msg.Body)
	if m == nil {
		t.Fatalf("No URL found in sent email: %q", msg.Body)
	}
	return m[0], m[1]
}

// staffUser returns the user staff as the database holds it.
func staffUser(t *testing.T, c *resetClient) *auth.User {
	t.Helper()
	u, err := orm.Query[auth.User](c.db).Get(t.Context(), "email", "staffmember@example.com")
	must(t, err)
	return &u
}

// Ported from Django: tests/auth_tests/test_views.py::AuthViewNamedURLTests.test_named_urls
//
// Named URLs should be reversible.
func TestDjangoAuthViewNamedURLs(t *testing.T) {
	router := urls.NewRouter(auth.URLs()...)
	cases := []struct {
		name   string
		kwargs urls.Kwargs
	}{
		{"login", nil},
		{"logout", nil},
		{"password_change", nil},
		{"password_change_done", nil},
		{"password_reset", nil},
		{"password_reset_done", nil},
		{"password_reset_confirm", urls.Kwargs{"uidb64": "aaaaaaa", "token": "1111-aaaaa"}},
		{"password_reset_complete", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var args []any
			if tc.kwargs != nil {
				args = append(args, tc.kwargs)
			}
			if _, err := router.Reverse(tc.name, args...); err != nil {
				t.Errorf("Reversal of url named '%s' failed with NoReverseMatch: %v", tc.name, err)
			}
		})
	}
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_email_not_found
//
// If the provided email is not registered, don't raise any error but also
// don't send any email.
func TestDjangoPasswordResetEmailNotFound(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		djangotest.AssertStatus(t, c.Get("/password_reset/"), http.StatusOK)
		resp := c.Post("/password_reset/", url.Values{"email": {"not_a_real_email@email.com"}})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertEqual(t, len(mail.Outbox()), 0)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_email_found
//
// Email is sent if a valid email address is provided for password reset.
func TestDjangoPasswordResetEmailFound(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		resp := c.Post("/password_reset/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		outbox := mail.Outbox()
		if len(outbox) != 1 {
			t.Fatalf("the outbox holds %d messages, want 1", len(outbox))
		}
		if !strings.Contains(outbox[0].Body, "http://") {
			t.Errorf("the body %q holds no http:// link", outbox[0].Body)
		}
		djangotest.AssertEqual(t, outbox[0].FromEmail(), mail.CurrentConfig().DefaultFromEmail)
		// The optional multipart text/html email is not added by default.
		djangotest.AssertEqual(t, parseMessage(t, outbox[0]).mediaType, "text/plain")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_extra_email_context
//
// extra_email_context should be available in the email template context.
func TestDjangoPasswordResetExtraEmailContext(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		resp := c.Post("/password_reset_extra_email_context/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		outbox := mail.Outbox()
		if len(outbox) != 1 {
			t.Fatalf("the outbox holds %d messages, want 1", len(outbox))
		}
		for _, want := range []string{`Email email context: "Hello!"`, "http://custom.example.com/reset/"} {
			if !strings.Contains(outbox[0].Body, want) {
				t.Errorf("the body %q does not hold %q", outbox[0].Body, want)
			}
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_html_mail_template
//
// A multipart email with text/plain and text/html is sent if the
// html_email_template parameter is passed to the view.
func TestDjangoPasswordResetHTMLMailTemplate(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		resp := c.Post("/password_reset/html_email_template/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		outbox := mail.Outbox()
		if len(outbox) != 1 {
			t.Fatalf("the outbox holds %d messages, want 1", len(outbox))
		}
		message := parseMessage(t, outbox[0])
		djangotest.AssertEqual(t, message.mediaType, "multipart/alternative")
		types, contents := message.parts(t)
		djangotest.AssertSequenceEqual(t, types, []string{"text/plain", "text/html"})
		if strings.Contains(contents[0], "<html>") {
			t.Errorf("the text part %q holds <html>", contents[0])
		}
		if !strings.Contains(contents[1], "<html>") {
			t.Errorf("the HTML part %q holds no <html>", contents[1])
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_email_found_custom_from
//
// Email is sent if a valid email address is provided for password reset when
// a custom from_email is provided.
func TestDjangoPasswordResetEmailFoundCustomFrom(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		resp := c.Post("/password_reset_from_email/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		outbox := mail.Outbox()
		if len(outbox) != 1 {
			t.Fatalf("the outbox holds %d messages, want 1", len(outbox))
		}
		djangotest.AssertEqual(t, outbox[0].FromEmail(), "staffmember@example.com")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_poisoned_http_host
//
// Poisoned HTTP_HOST headers can't be used for reset emails. The site runs
// behind middleware.AllowedHosts, which validates the host as Django's
// get_host does.
func TestDjangoPasswordResetPoisonedHTTPHost(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		hosts := middleware.AllowedHosts(middleware.AllowedHostsOptions{AllowedHosts: []string{"testserver"}})
		c := newResetClient(t, db, hosts(resetViewsURLs(t)))
		logs := captureLogs(t)
		resp := c.Post("/password_reset/", url.Values{"email": {"staffmember@example.com"}},
			djangotest.Header("Host", "www.example:dr.frankenstein@evil.tld"))
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)
		djangotest.AssertEqual(t, len(mail.Outbox()), 0)
		if !strings.Contains(logs.String(), "level=ERROR") ||
			!strings.Contains(logs.String(), "Invalid HTTP_HOST header") {
			t.Errorf("no DisallowedHost error in the log %q", logs.String())
		}
	})
}

// No Go counterpart: tests/auth_tests/test_views.py::PasswordResetTest.test_poisoned_http_host_admin_site
// The path /admin_password_reset/ has no view in Django's test URLconf; the
// test shows that the host check comes first, which
// TestDjangoPasswordResetPoisonedHTTPHost covers. The admin reset pages are
// tested in the admin package.

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_valid
func TestDjangoPasswordResetConfirmValid(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		djangotest.AssertContains(t, c.Get(path), "Please enter your new password")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_invalid
func TestDjangoPasswordResetConfirmInvalid(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		// Munge the token in the path, but keep the same length.
		path = path[:len(path)-5] + "0000" + path[len(path)-1:]
		djangotest.AssertContains(t, c.Get(path), "The password reset link was invalid")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_invalid_user
//
// A nonexistent user returns a 200 response, not a 404.
func TestDjangoPasswordResetConfirmInvalidUser(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		djangotest.AssertContains(t, c.Get("/reset/123456/1-1/"), "The password reset link was invalid")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_overflow_user
//
// A base36 user id that overflows int returns a 200 response.
func TestDjangoPasswordResetConfirmOverflowUser(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		djangotest.AssertContains(t, c.Get("/reset/zzzzzzzzzzzzz/1-1/"), "The password reset link was invalid")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_invalid_post
//
// Same as test_confirm_invalid, but trying to do a POST instead.
func TestDjangoPasswordResetConfirmInvalidPost(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		path = path[:len(path)-5] + "0000" + path[len(path)-1:]
		c.Post(path, url.Values{"new_password1": {"anewpassword"}, "new_password2": {" anewpassword"}})
		if staffUser(t, c).CheckPassword("anewpassword") {
			t.Error("the password changed")
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_invalid_hash
//
// A POST with an invalid token is rejected.
func TestDjangoPasswordResetConfirmInvalidHash(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		original := staffUser(t, c).Password
		_, path := c.confirmStart(t)
		parts := strings.Split(path, "-")
		parts[len(parts)-1] = strings.Repeat("0", 20) + "/"
		path = strings.Join(parts, "-")
		resp := c.Post(path, newPasswordData)
		validlink, _ := resp.Context("validlink")
		djangotest.AssertEqual(t, validlink, any(false))
		djangotest.AssertEqual(t, staffUser(t, c).Password, original)
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_complete
func TestDjangoPasswordResetConfirmComplete(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		c.Post(path, newPasswordData)
		if !staffUser(t, c).CheckPassword("anewpassword") {
			t.Error("the password did not change")
		}
		// The reset token is deleted from the session.
		if _, ok := c.Session().Get(auth.InternalResetSessionToken); ok {
			t.Error("the session still holds the reset token")
		}
		// The link cannot be used again.
		djangotest.AssertContains(t, c.Get(path), "The password reset link was invalid")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_different_passwords
func TestDjangoPasswordResetConfirmDifferentPasswords(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		resp := c.Post(path, url.Values{"new_password1": {"anewpassword"}, "new_password2": {"x"}})
		form, _ := resp.Context("form")
		djangotest.AssertFormError(t, form, "new_password2", []string{"The two password fields didn’t match."})
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_reset_redirect_default
func TestDjangoPasswordResetRedirectDefault(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		resp := c.Post("/password_reset/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertRedirects(t, resp, "/password_reset/done/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_reset_custom_redirect
func TestDjangoPasswordResetCustomRedirect(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		resp := c.Post("/password_reset/custom_redirect/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertRedirects(t, resp, "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_reset_custom_redirect_named
func TestDjangoPasswordResetCustomRedirectNamed(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		resp := c.Post("/password_reset/custom_redirect/named/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertRedirects(t, resp, "/password_reset/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_redirect_default
func TestDjangoPasswordResetConfirmRedirectDefault(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		resp := c.Post(path, newPasswordData)
		djangotest.AssertRedirects(t, resp, "/reset/done/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_redirect_custom
func TestDjangoPasswordResetConfirmRedirectCustom(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		path = strings.Replace(path, "/reset/", "/reset/custom/", 1)
		resp := c.Post(path, newPasswordData)
		djangotest.AssertRedirects(t, resp, "/custom/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_redirect_custom_named
func TestDjangoPasswordResetConfirmRedirectCustomNamed(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		path = strings.Replace(path, "/reset/", "/reset/custom/named/", 1)
		resp := c.Post(path, newPasswordData)
		djangotest.AssertRedirects(t, resp, "/password_reset/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_custom_reset_url_token
func TestDjangoPasswordResetConfirmCustomResetURLToken(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		path = strings.Replace(path, "/reset/", "/reset/custom/token/", 1)
		c.resetURLToken = "set-passwordcustom"
		resp := c.Post(path, newPasswordData)
		djangotest.AssertRedirects(t, resp, "/reset/done/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_login_post_reset
func TestDjangoPasswordResetConfirmLoginPostReset(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		path = strings.Replace(path, "/reset/", "/reset/post_reset_login/", 1)
		resp := c.Post(path, newPasswordData)
		djangotest.AssertRedirects(t, resp, "/reset/done/", djangotest.FetchRedirectResponse(false))
		if _, ok := c.Session().Get(auth.SessionKey); !ok {
			t.Error("the session holds no user")
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_login_post_reset_custom_backend
func TestDjangoPasswordResetConfirmLoginPostResetCustomBackend(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		useBackends(t, auth.ModelBackend{}, auth.AllowAllUsersModelBackend{})
		c := newResetClient(t, db, resetViewsURLs(t))
		// This backend is specified in the URL pattern.
		backend := auth.AllowAllUsersModelBackend{}.Name()
		_, path := c.confirmStart(t)
		path = strings.Replace(path, "/reset/", "/reset/post_reset_login_custom_backend/", 1)
		resp := c.Post(path, newPasswordData)
		djangotest.AssertRedirects(t, resp, "/reset/done/", djangotest.FetchRedirectResponse(false))
		sess := c.Session()
		if _, ok := sess.Get(auth.SessionKey); !ok {
			t.Error("the session holds no user")
		}
		got, _ := sess.Get(auth.BackendSessionKey)
		djangotest.AssertEqual(t, got, any(backend))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_login_post_reset_already_logged_in
func TestDjangoPasswordResetConfirmLoginPostResetAlreadyLoggedIn(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		path = strings.Replace(path, "/reset/", "/reset/post_reset_login/", 1)
		c.login(t, "testclient", "password", "/login/")
		resp := c.Post(path, newPasswordData)
		djangotest.AssertRedirects(t, resp, "/reset/done/", djangotest.FetchRedirectResponse(false))
		if _, ok := c.Session().Get(auth.SessionKey); !ok {
			t.Error("the session holds no user")
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_display_user_from_form
//
// The view passes the user to the SetPasswordForm, even on GET requests
// (#16919), but not on a form of an invalid link.
func TestDjangoPasswordResetConfirmDisplayUserFromForm(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		resp := c.Get(path)
		djangotest.AssertContains(t, resp, "Hello, "+staffUser(t, c).Username+".")
		djangotest.AssertContains(t, c.Get("/reset/zzzzzzzzzzzzz/1-1/"), "Hello, .")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_link_redirects_to_set_password_page
func TestDjangoPasswordResetConfirmLinkRedirectsToSetPasswordPage(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		// The plain client of viewsSite does not rewrite the link.
		resp := c.viewsSite.Get(path)
		parts := strings.Split(strings.Trim(path, "/"), "/")
		uidb64, token := parts[1], parts[2]
		djangotest.AssertRedirects(t, resp, "/reset/"+uidb64+"/set-password/")
		got, _ := c.Session().Get(auth.InternalResetSessionToken)
		djangotest.AssertEqual(t, got, any(token))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_confirm_custom_reset_url_token_link_redirects_to_set_password_page
func TestDjangoPasswordResetConfirmCustomResetURLTokenLinkRedirectsToSetPasswordPage(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		path = strings.Replace(path, "/reset/", "/reset/custom/token/", 1)
		resp := c.viewsSite.Get(path)
		parts := strings.Split(strings.Trim(path, "/"), "/")
		uidb64, token := parts[3], parts[4]
		djangotest.AssertRedirects(t, resp, "/reset/custom/token/"+uidb64+"/set-passwordcustom/")
		got, _ := c.Session().Get(auth.InternalResetSessionToken)
		djangotest.AssertEqual(t, got, any(token))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_invalid_link_if_going_directly_to_the_final_reset_password_url
func TestDjangoPasswordResetInvalidLinkIfGoingDirectlyToTheFinalResetPasswordURL(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		_, path := c.confirmStart(t)
		uidb64 := strings.Split(strings.Trim(path, "/"), "/")[1]
		resp := c.viewsSite.Get("/reset/" + uidb64 + "/set-password/")
		djangotest.AssertContains(t, resp, "The password reset link was invalid")
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_missing_kwargs
//
// Django raises ImproperlyConfigured; the Go view answers with 500 and logs
// the message.
func TestDjangoPasswordResetMissingKwargs(t *testing.T) {
	resetViewsTest(t, func(t *testing.T, c *resetClient) {
		logs := captureLogs(t)
		resp := c.Get("/reset/missing_parameters/")
		djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
		msg := "The URL path must contain 'uidb64' and 'token' parameters."
		if !strings.Contains(logs.String(), msg) {
			t.Errorf("the log %q does not hold %q", logs.String(), msg)
		}
	})
}

// Ported from Django: tests/auth_tests/test_views.py::PasswordResetTest.test_access_under_login_required_middleware
func TestDjangoPasswordResetAccessUnderLoginRequiredMiddleware(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		c := newResetClient(t, db, withLoginRequired(resetViewsURLs(t)))
		for _, path := range []string{"/password_reset/", "/password_reset/done/", "/reset/abc/def/", "/reset/done/"} {
			t.Run(path, func(t *testing.T) {
				djangotest.AssertStatus(t, c.Get(path), http.StatusOK)
			})
		}
		resp := c.Post("/password_reset/", url.Values{"email": {"staffmember@example.com"}})
		djangotest.AssertRedirects(t, resp, "/password_reset/done/", djangotest.FetchRedirectResponse(false))
	})
}

// Ported from Django: tests/auth_tests/test_views.py::CustomUserPasswordResetTest.test_confirm_valid_custom_user
func TestDjangoCustomUserPasswordResetConfirmValidCustomUser(t *testing.T) {
	t.Skip("needs a custom user model, which djangogo does not have")
}

// Ported from Django: tests/auth_tests/test_views.py::CustomUserCompositePrimaryKeyPasswordResetTest.test_confirm_valid_custom_user
func TestDjangoCustomUserCompositePrimaryKeyPasswordResetConfirmValidCustomUser(t *testing.T) {
	t.Skip("needs a custom user model with a composite primary key, which djangogo does not have")
}

// Ported from Django: tests/auth_tests/test_views.py::UUIDUserPasswordResetTest.test_confirm_valid_custom_user
func TestDjangoUUIDUserPasswordResetConfirmValidCustomUser(t *testing.T) {
	t.Skip("needs a custom user model with a UUID primary key, which djangogo does not have")
}

// Ported from Django: tests/auth_tests/test_views.py::UUIDUserPasswordResetTest.test_confirm_invalid_uuid
func TestDjangoUUIDUserPasswordResetConfirmInvalidUUID(t *testing.T) {
	t.Skip("needs a custom user model with a UUID primary key, which djangogo does not have")
}
