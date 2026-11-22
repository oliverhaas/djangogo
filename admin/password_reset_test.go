package admin_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/mail"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/urls"
)

// passwordResetClient serves the routes of an admin site with PasswordReset
// set to reset behind the sessions, CSRF and auth middleware, and returns a
// client for it.
func passwordResetClient(t *testing.T, db *orm.DB, reset bool) *djangotest.Client {
	t.Helper()
	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	site.PasswordReset = reset
	store := sessions.NewSignedCookieStore([]byte("password-reset-test-secret"))
	var h http.Handler = urls.NewRouter(site.Routes()...)
	h = sessions.Middleware(store, "sessionid")(csrf.Middleware(auth.Middleware(db)(h)))
	return djangotest.NewClient(t, h, djangotest.WithSessions(store, "sessionid"))
}

// No Django test exists: the login page links to the password reset page only
// when the site serves it, as Django's login template shows the link only
// when the URL name admin_password_reset resolves.
func TestAdminLoginPasswordResetLink(t *testing.T) {
	db := newUserDB(t)
	link := `<a href="/admin/password_reset/">Forgotten your login credentials?</a>`
	djangotest.AssertNotContains(t, passwordResetClient(t, db, false).Get("/admin/login/"), "Forgotten your")
	djangotest.AssertContains(t, passwordResetClient(t, db, true).Get("/admin/login/"), link, djangotest.HTML())
}

// No Django test exists: with PasswordReset set, a user resets the password
// through the admin pages, from the email to the login link.
func TestAdminPasswordResetFlow(t *testing.T) {
	db := newUserDB(t)
	djangotest.LocmemMail(t)
	user, err := auth.CreateUser(t.Context(), db, "admin", "admin@example.com", "old-password", true, true, true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	c := passwordResetClient(t, db, true)

	djangotest.AssertContains(t, c.Get("/admin/password_reset/"), "Forgotten your password?")
	resp := c.Post("/admin/password_reset/", url.Values{"email": {"admin@example.com"}})
	djangotest.AssertRedirects(t, resp, "/admin/password_reset/done/")
	djangotest.AssertContains(t, c.Get("/admin/password_reset/done/"), "We’ve emailed you instructions")

	outbox := mail.Outbox()
	if len(outbox) != 1 {
		t.Fatalf("the outbox holds %d messages, want 1", len(outbox))
	}
	m := regexp.MustCompile(`http://testserver(/admin/reset/[^/]+/[^/]+/)`).FindStringSubmatch(outbox[0].Body)
	if m == nil {
		t.Fatalf("no reset link in the email %q", outbox[0].Body)
	}
	uidb64 := strings.Split(m[1], "/")[3]
	setPassword := "/admin/reset/" + uidb64 + "/set-password/"
	djangotest.AssertRedirects(t, c.Get(m[1]), setPassword)
	djangotest.AssertContains(t, c.Get(setPassword), "Please enter your new password twice")

	resp = c.Post(setPassword, url.Values{"new_password1": {"new-password"}, "new_password2": {"new-password"}})
	djangotest.AssertRedirects(t, resp, "/admin/reset/done/", djangotest.FetchRedirectResponse(false))
	djangotest.AssertContains(t, c.Get("/admin/reset/done/"), `<a href="/admin/login/">Log in</a>`, djangotest.HTML())
	stored, err := orm.Query[auth.User](db).Get(t.Context(), "id", user.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !stored.CheckPassword("new-password") {
		t.Error("the password did not change")
	}
	djangotest.AssertContains(t, c.Get(m[1]), "The password reset link was invalid")
}
