package admin

import (
	"cmp"
	"errors"
	"maps"
	"net/http"
	"strings"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// msgInvalidAdminLogin is Django's AdminAuthenticationForm.error_messages["invalid_login"].
const msgInvalidAdminLogin = "Please enter the correct username and password for a staff account. Note that both fields may be case-sensitive."

// newAdminLoginForm returns the admin login form (username + password).
func newAdminLoginForm() *forms.Form {
	return forms.New(
		forms.F("username", forms.NewCharField(
			forms.Label("Username"),
			forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"autofocus": true}}),
		)),
		forms.F("password", forms.NewCharField(
			forms.Label("Password"),
			forms.Strip(false),
			forms.UseWidget(forms.PasswordInput{}),
		)),
	)
}

// login renders and processes the admin login form (Django's AdminSite.login).
// GET redirects already-authorized staff users to a safe next target (or the
// admin index). POST validates credentials, checks staff/active access, logs
// the user into the session, and redirects.
func (s *AdminSite) login(w http.ResponseWriter, r *http.Request) {
	neverCache(w)
	if r.Method == http.MethodPost {
		s.loginPOST(w, r)
		return
	}
	next := s.safeRedirectTarget(r, s.indexURL())
	if s.HasPermission(r) {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}
	s.renderLogin(w, r, newAdminLoginForm(), next)
}

// loginPOST validates submitted credentials and, on success, establishes the
// session and redirects; otherwise it re-renders the login form with errors.
func (s *AdminSite) loginPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	next := s.safeRedirectTarget(r, s.indexURL())
	form := newAdminLoginForm().Bind(r.PostForm)
	if !form.IsValid() {
		s.renderLogin(w, r, form, next)
		return
	}

	username := auth.NormalizeUsername(strings.TrimSpace(r.PostForm.Get("username")))
	password := r.PostForm.Get("password")
	user, err := auth.Authenticate(r.Context(), s.db, username, password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		form.AddError("", msgInvalidAdminLogin)
		s.renderLogin(w, r, form, next)
		return
	}
	if err != nil {
		http.Error(w, "authentication error", http.StatusInternalServerError)
		return
	}

	if msg, ok := s.confirmLoginAllowed(r, user); !ok {
		form.AddError("", msg)
		s.renderLogin(w, r, form, next)
		return
	}

	if err := auth.LoginRequest(r, s.db, user); err != nil {
		http.Error(w, "no session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// confirmLoginAllowed checks whether an authenticated user may enter the admin
// site, mirroring AdminAuthenticationForm.confirm_login_allowed. When denied,
// it returns the user-facing message and false.
func (s *AdminSite) confirmLoginAllowed(r *http.Request, u *auth.User) (string, bool) {
	if s.LoginConfirmAllowed != nil {
		if err := s.LoginConfirmAllowed(r, u); err != nil {
			return err.Error(), false
		}
		return "", true
	}
	if !u.IsActive || !u.IsStaff {
		return msgInvalidAdminLogin, false
	}
	return "", true
}

// renderLogin renders admin/login.html with the form, redirect target, and
// site context.
func (s *AdminSite) renderLogin(w http.ResponseWriter, r *http.Request, form *forms.Form, next string) {
	ctx := s.EachContext(r)
	u, _ := auth.GetUser(r)
	ctx["title"] = "Log in"
	ctx["subtitle"] = ""
	ctx["app_path"] = r.URL.RequestURI()
	ctx["username"] = u.GetUsername()
	ctx["next"] = next
	ctx["redirect_field_name"] = auth.DefaultRedirectFieldName
	ctx["form"] = form
	ctx["action"] = r.URL.RequestURI()
	maps.Copy(ctx, s.extraContext(r, "login"))
	s.render(w, r, cmp.Or(s.LoginTemplate, "admin/login.html"), ctx)
}

// logout handles POST /admin/logout/ (Django's AdminSite.logout). GET returns
// 405 Method Not Allowed, matching Django 5+.
func (s *AdminSite) logout(w http.ResponseWriter, r *http.Request) {
	ctx := s.EachContext(r)
	// Override has_permission because the user is being logged out now.
	ctx["has_permission"] = false
	maps.Copy(ctx, s.extraContext(r, "logout"))
	v := &auth.LogoutView{
		Templates:    s.engine,
		TemplateName: cmp.Or(s.LogoutTemplate, auth.LoggedOutTemplate),
		ExtraContext: ctx,
	}
	v.ServeHTTP(w, r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName())))
}

// passwordChange handles /admin/password_change/ via auth.PasswordChangeView
// using the admin's templates and site context.
func (s *AdminSite) passwordChange(w http.ResponseWriter, r *http.Request) {
	ctx := s.EachContext(r)
	maps.Copy(ctx, s.extraContext(r, "password_change"))
	v := &auth.PasswordChangeView{
		DB:           s.db,
		Templates:    s.engine,
		TemplateName: cmp.Or(s.PasswordChangeTemplate, auth.PasswordChangeTemplate),
		SuccessURL:   s.passwordChangeDoneURL(),
		ExtraContext: ctx,
	}
	v.ServeHTTP(w, r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName())))
}

// passwordChangeDone handles /admin/password_change/done/ via
// auth.PasswordChangeDoneView using the admin's templates and site context.
func (s *AdminSite) passwordChangeDone(w http.ResponseWriter, r *http.Request) {
	ctx := s.EachContext(r)
	maps.Copy(ctx, s.extraContext(r, "password_change_done"))
	v := &auth.PasswordChangeDoneView{
		Templates:    s.engine,
		TemplateName: cmp.Or(s.PasswordChangeDoneTemplate, auth.PasswordChangeDoneTemplate),
		ExtraContext: ctx,
	}
	v.ServeHTTP(w, r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName())))
}

// safeRedirectTarget returns the safe "next" target from POST or query
// parameters, falling back to fallback when absent or unsafe.
func (s *AdminSite) safeRedirectTarget(r *http.Request, fallback string) string {
	next := ""
	if r.PostForm != nil {
		if vs := r.PostForm[auth.DefaultRedirectFieldName]; len(vs) > 0 {
			next = vs[len(vs)-1]
		}
	}
	if next == "" {
		if vs := r.URL.Query()[auth.DefaultRedirectFieldName]; len(vs) > 0 {
			next = vs[len(vs)-1]
		}
	}
	if next != "" && auth.URLHasAllowedHostAndScheme(next, []string{r.Host}, r.TLS != nil) {
		return next
	}
	return fallback
}

// passwordResetRoutes returns the routes of AdminSite.PasswordReset: the
// password reset views of the auth package, rendered with the admin's
// templates and site context.
func (s *AdminSite) passwordResetRoutes() []urls.Route {
	return urls.Include(s.prefix(), "",
		urls.Path("password_reset/", auth.LoginNotRequired(http.HandlerFunc(s.passwordReset)), "admin_password_reset"),
		urls.Path("password_reset/done/", auth.LoginNotRequired(http.HandlerFunc(s.passwordResetDone)), "password_reset_done"),
		urls.Path("reset/<uidb64>/<token>/", auth.LoginNotRequired(http.HandlerFunc(s.passwordResetConfirm)), "password_reset_confirm"),
		urls.Path("reset/done/", auth.LoginNotRequired(http.HandlerFunc(s.passwordResetComplete)), "password_reset_complete"),
	)
}

// passwordResetContext returns the template context of the password reset
// page view: the context of every admin page, the login URL of the admin and
// the ExtraContext values of view.
func (s *AdminSite) passwordResetContext(r *http.Request, view string) map[string]any {
	ctx := s.EachContext(r)
	ctx["login_url"] = s.loginURL()
	maps.Copy(ctx, s.extraContext(r, view))
	return ctx
}

// passwordReset serves the page that asks for the email address.
func (s *AdminSite) passwordReset(w http.ResponseWriter, r *http.Request) {
	v := &auth.PasswordResetView{
		DB:           s.db,
		Templates:    s.engine,
		SuccessURL:   s.prefix() + "/password_reset/done/",
		ExtraContext: s.passwordResetContext(r, "password_reset"),
	}
	v.ServeHTTP(w, r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName())))
}

// passwordResetDone serves the page after the email is sent.
func (s *AdminSite) passwordResetDone(w http.ResponseWriter, r *http.Request) {
	v := &auth.PasswordResetDoneView{
		Templates:    s.engine,
		ExtraContext: s.passwordResetContext(r, "password_reset_done"),
	}
	v.ServeHTTP(w, r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName())))
}

// passwordResetConfirm serves the link of the email and the new password
// form.
func (s *AdminSite) passwordResetConfirm(w http.ResponseWriter, r *http.Request) {
	v := &auth.PasswordResetConfirmView{
		DB:           s.db,
		Templates:    s.engine,
		SuccessURL:   s.prefix() + "/reset/done/",
		ExtraContext: s.passwordResetContext(r, "password_reset_confirm"),
	}
	v.ServeHTTP(w, r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName())))
}

// passwordResetComplete serves the page after the new password is set.
func (s *AdminSite) passwordResetComplete(w http.ResponseWriter, r *http.Request) {
	v := &auth.PasswordResetCompleteView{
		Templates:    s.engine,
		ExtraContext: s.passwordResetContext(r, "password_reset_complete"),
	}
	v.ServeHTTP(w, r.WithContext(templates.WithCurrentApp(r.Context(), s.siteName())))
}
