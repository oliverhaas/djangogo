package auth

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// Template names of the views, as in Django.
const (
	LoginTemplate              = "registration/login.html"
	LoggedOutTemplate          = "registration/logged_out.html"
	PasswordChangeTemplate     = "registration/password_change_form.html"
	PasswordChangeDoneTemplate = "registration/password_change_done.html"
)

// Messages of the login and password forms, in Django's wording.
const (
	msgRequired          = "This field is required."
	msgInvalidLogin      = "Please enter a correct username and password. Note that both fields may be case-sensitive."
	msgInactive          = "This account is inactive."
	msgPasswordIncorrect = "Your old password was entered incorrectly. Please enter it again."
	msgPasswordMismatch  = "The two password fields didn’t match."
	msgRedirectLoop      = "Redirection loop for authenticated user detected. Check that your " +
		"LOGIN_REDIRECT_URL doesn't point to a login page."
)

// Renderer renders a named template for a request. *templates.Engine
// implements it; its context processors run as for any other page.
type Renderer interface {
	RenderRequest(r *http.Request, name string, data map[string]any) (string, error)
}

// embeddedTemplates holds the default templates of the views.
//
//go:embed templates
var embeddedTemplates embed.FS

// Templates returns the default templates of the views, under "registration/".
// They are plain pages without a base template. The engine of an Application
// that installs App finds them through App.Templates. A project that renders
// the views with an engine of its own adds this file system as its last
// template source, so that its own templates of the same names take
// precedence.
func Templates() fs.FS {
	sub, err := fs.Sub(embeddedTemplates, "templates")
	if err != nil {
		// fs.Sub fails only for an invalid directory name, and the name is a
		// constant.
		return embeddedTemplates
	}
	return sub
}

// defaultEngine renders the embedded templates for views without a Renderer.
var defaultEngine = sync.OnceValues(func() (*templates.Engine, error) {
	return templates.NewEngineFS(Templates())
})

// render writes the page name for r with status 200. The CSRF token is added
// to data, so that the form carries it also when the renderer has no CSRF
// context processor.
func render(w http.ResponseWriter, r *http.Request, renderer Renderer, name string, data map[string]any) {
	if renderer == nil {
		engine, err := defaultEngine()
		if err != nil {
			serverError(w)
			return
		}
		renderer = engine
	}
	if _, ok := data["csrf_token"]; !ok {
		data["csrf_token"] = csrf.ContextProcessor(r)["csrf_token"]
	}
	out, err := renderer.RenderRequest(r, name, data)
	if err != nil {
		serverError(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, out)
}

// serverError answers 500 without details: the cause may hold data that a
// visitor must not see.
func serverError(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// neverCache marks the response as not to be stored by any cache (Django's
// never_cache). A login or logout page must not be served from a cache.
func neverCache(w http.ResponseWriter) {
	h := w.Header()
	if h.Get("Expires") == "" {
		h.Set("Expires", time.Now().UTC().Format(http.TimeFormat))
	}
	h.Set("Cache-Control", "max-age=0, no-cache, no-store, must-revalidate, private")
}

// allowMethods answers a request whose method the view does not serve: 200
// with the Allow header for OPTIONS, 405 with the Allow header otherwise
// (Django's View.options and http_method_not_allowed). It reports whether it
// answered.
func allowMethods(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if r.Method == m {
			return false
		}
	}
	w.Header().Set("Allow", strings.Join(append(methods, http.MethodOptions), ", "))
	if r.Method == http.MethodOptions {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
		return true
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
	return true
}

// requestValue returns the value of a form field, from the POST body when it
// has the field and from the query string otherwise (Django's
// request.POST.get(name, request.GET.get(name))). Of a repeated field the
// last value counts, as in Django. ParseForm must have run.
func requestValue(r *http.Request, name string) string {
	if vs := r.PostForm[name]; len(vs) > 0 {
		return vs[len(vs)-1]
	}
	if vs := r.URL.Query()[name]; len(vs) > 0 {
		return vs[len(vs)-1]
	}
	return ""
}

// redirectTarget returns the page the request asks to go to next, in the
// field named field, when it is safe: on the host of the request or one of
// allowedHosts, and on https when the request came over https. Otherwise it
// returns "" (Django's RedirectURLMixin.get_redirect_url).
func redirectTarget(r *http.Request, field string, allowedHosts []string) string {
	target := requestValue(r, field)
	hosts := append([]string{r.Host}, allowedHosts...)
	if URLHasAllowedHostAndScheme(target, hosts, requestIsSecure(r)) {
		return target
	}
	return ""
}

// redirectField returns name or the default redirect field name.
func redirectField(name string) string {
	if name != "" {
		return name
	}
	return DefaultRedirectFieldName
}

// isIdentifier reports whether s can name a template variable: an ASCII
// letter or underscore, then letters, digits and underscores.
func isIdentifier(s string) bool {
	for i := range len(s) {
		c := s[i]
		if c != '_' && !isASCIILetter(c) && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return s != ""
}

// viewDB returns db, or the database of Middleware when db is nil.
func viewDB(r *http.Request, db *orm.DB) *orm.DB {
	if db != nil {
		return db
	}
	return requestDB(r.Context())
}

// formErrors collects the errors of a submitted form, per field and for the
// form as a whole.
type formErrors struct {
	// fields maps a field name to its messages.
	fields map[string][]string
	// nonField holds the messages that concern the whole form.
	nonField []string
}

// add records msg for field; an empty field means the whole form.
func (e *formErrors) add(field, msg string) {
	if field == "" {
		e.nonField = append(e.nonField, msg)
		return
	}
	if e.fields == nil {
		e.fields = make(map[string][]string)
	}
	e.fields[field] = append(e.fields[field], msg)
}

// empty reports whether no error was recorded.
func (e *formErrors) empty() bool { return len(e.fields) == 0 && len(e.nonField) == 0 }

// context adds the errors to a template context: "errors" in the shape of
// Django's form.errors, with the errors of the whole form under "__all__",
// "field_errors" with the fields only, and "non_field_errors".
func (e *formErrors) context(data map[string]any) {
	all := make(map[string][]string, len(e.fields)+1)
	fields := make(map[string][]string, len(e.fields))
	for name, msgs := range e.fields {
		all[name], fields[name] = msgs, msgs
	}
	if len(e.nonField) > 0 {
		all["__all__"] = e.nonField
	}
	data["errors"] = all
	data["field_errors"] = fields
	data["non_field_errors"] = e.nonField
}

// merge copies extra into data; keys of extra win.
func merge(data, extra map[string]any) map[string]any {
	for k, v := range extra {
		data[k] = v
	}
	return data
}

// LoginView shows the login form and logs the user in (Django's LoginView).
// GET shows the form; POST checks the credentials with Authenticate, logs the
// user in with LoginRequest and redirects to the page named in the next field
// of the form or the query string, when that page is safe, and to NextPage or
// Config.LoginRedirectURL otherwise. Responses are marked as not cacheable.
//
// The page is rendered from TemplateName with this context, in place of
// Django's form object: "username" (the submitted value), "next" (the safe
// page to return to, also under the name of the redirect field),
// "redirect_field_name", "errors" (field name to messages, "__all__" for the
// form as a whole), "field_errors", "non_field_errors", "title", "site_name"
// (the host of the request) and "csrf_token", plus ExtraContext.
//
// The sessions middleware and Middleware must run first. The zero value is
// ready to use.
type LoginView struct {
	// DB holds the users. Nil means the database of Middleware.
	DB *orm.DB
	// Templates renders the page. Nil means the embedded default template.
	Templates Renderer
	// TemplateName is the page template. Empty means LoginTemplate.
	TemplateName string
	// NextPage is where a user goes after login when the request names no
	// safe page (Django's next_page). Empty means Config.LoginRedirectURL.
	NextPage string
	// RedirectFieldName is the field that names the page to return to.
	// Empty means "next".
	RedirectFieldName string
	// SuccessURLAllowedHosts are hosts besides the host of the request that
	// the next page may be on (Django's success_url_allowed_hosts).
	SuccessURLAllowedHosts []string
	// RedirectAuthenticatedUser sends a user that is logged in already on
	// to the success URL instead of showing the form.
	RedirectAuthenticatedUser bool
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// successURL returns where the user goes after login (Django's
// get_success_url).
func (v *LoginView) successURL(r *http.Request) string {
	if target := redirectTarget(r, redirectField(v.RedirectFieldName), v.SuccessURLAllowedHosts); target != "" {
		return target
	}
	if v.NextPage != "" {
		return v.NextPage
	}
	return configuredLoginRedirectURL()
}

// ServeHTTP implements http.Handler.
func (v *LoginView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	neverCache(w)
	if allowMethods(w, r, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodHead) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if v.RedirectAuthenticatedUser {
		u, err := GetUser(r)
		if err != nil {
			serverError(w)
			return
		}
		if u.IsAuthenticated() {
			target := v.successURL(r)
			if target == r.URL.Path {
				// Django raises ValueError here: the configuration sends
				// the user back to this page forever.
				http.Error(w, msgRedirectLoop, http.StatusInternalServerError)
				return
			}
			redirect(w, target)
			return
		}
	}
	var errs formErrors
	username := ""
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		username = r.PostForm.Get("username")
		u, err := v.authenticate(r, username, r.PostForm.Get("password"), &errs)
		if err != nil {
			serverError(w)
			return
		}
		if u != nil {
			if err := LoginRequest(r, viewDB(r, v.DB), u); err != nil {
				serverError(w)
				return
			}
			redirect(w, v.successURL(r))
			return
		}
	}
	field := redirectField(v.RedirectFieldName)
	next := redirectTarget(r, field, v.SuccessURLAllowedHosts)
	data := map[string]any{
		"title":               "Log in",
		"username":            username,
		"next":                next,
		"redirect_field_name": field,
		"site_name":           r.Host,
	}
	if isIdentifier(field) {
		// Django names the context value after the field; a template
		// variable must be an identifier.
		data[field] = next
	}
	errs.context(data)
	name := v.TemplateName
	if name == "" {
		name = LoginTemplate
	}
	render(w, r, v.Templates, name, merge(data, v.ExtraContext))
}

// authenticate validates the submitted credentials like Django's
// AuthenticationForm and returns the user they belong to, or nil with the
// reasons in errs. The error is a failure of the database.
func (v *LoginView) authenticate(r *http.Request, username, password string, errs *formErrors) (*User, error) {
	username = NormalizeUsername(strings.TrimFunc(username, isPythonSpace))
	if username == "" {
		errs.add("username", msgRequired)
	}
	// The password is not stripped: spaces may be part of it.
	if password == "" {
		errs.add("password", msgRequired)
	}
	if !errs.empty() {
		return nil, nil
	}
	db := viewDB(r, v.DB)
	if db == nil {
		return nil, errNoDatabase
	}
	u, err := Authenticate(r.Context(), db, username, password)
	if errors.Is(err, ErrInvalidCredentials) {
		errs.add("", msgInvalidLogin)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		errs.add("", msgInactive)
		return nil, nil
	}
	return u, nil
}

// LogoutView logs the user out on POST and shows the logged-out page or
// redirects (Django's LogoutView). Since Django 5 logout is a POST, so that a
// link or an image on another site cannot end the session; GET answers 405.
//
// After the logout the user goes to the page named in the next field when it
// is safe, otherwise to NextPage, Config.LogoutRedirectURL, or this page,
// which then renders TemplateName. The context has "title", "site_name",
// "login_url" and "csrf_token", plus ExtraContext. Responses are marked as
// not cacheable.
type LogoutView struct {
	// Templates renders the page. Nil means the embedded default template.
	Templates Renderer
	// TemplateName is the page template. Empty means LoggedOutTemplate.
	TemplateName string
	// NextPage is where the user goes after logout when the request names no
	// safe page (Django's next_page). Empty means Config.LogoutRedirectURL.
	NextPage string
	// RedirectFieldName is the field that names the page to go to. Empty
	// means "next".
	RedirectFieldName string
	// SuccessURLAllowedHosts are hosts besides the host of the request that
	// the next page may be on.
	SuccessURLAllowedHosts []string
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// ServeHTTP implements http.Handler.
func (v *LogoutView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	neverCache(w)
	if allowMethods(w, r, http.MethodPost) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	LogoutRequest(r)
	target := redirectTarget(r, redirectField(v.RedirectFieldName), v.SuccessURLAllowedHosts)
	if target == "" {
		target = v.NextPage
	}
	if target == "" {
		target = settings().LogoutRedirectURL
	}
	if target == "" {
		target = r.URL.Path
	}
	if target != r.URL.RequestURI() {
		redirect(w, target)
		return
	}
	data := map[string]any{
		"title":     "Logged out",
		"site_name": r.Host,
		"login_url": configuredLoginURL(),
	}
	name := v.TemplateName
	if name == "" {
		name = LoggedOutTemplate
	}
	render(w, r, v.Templates, name, merge(data, v.ExtraContext))
}

// LogoutThenLogin returns a handler that logs the user out on POST and
// redirects to the login page (Django's logout_then_login). An empty loginURL
// means Config.LoginURL.
func LogoutThenLogin(loginURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := loginURL
		if next == "" {
			next = configuredLoginURL()
		}
		(&LogoutView{NextPage: next}).ServeHTTP(w, r)
	})
}

// PasswordChangeView lets a logged-in user change the password (Django's
// PasswordChangeView). It asks for the old password and the new one twice,
// checks the new one with the password validators, stores it, keeps the
// session of the request logged in with UpdateSessionAuthHash and redirects
// to SuccessURL. Other sessions of the user end. An anonymous user is sent to
// the login page.
//
// The form fields are "old_password", "new_password1" and "new_password2".
// The context has "title", "errors", "field_errors", "non_field_errors",
// "password_help_texts" and "csrf_token", plus ExtraContext.
type PasswordChangeView struct {
	// DB holds the users. Nil means the database of Middleware.
	DB *orm.DB
	// Templates renders the page. Nil means the embedded default template.
	Templates Renderer
	// TemplateName is the page template. Empty means PasswordChangeTemplate.
	TemplateName string
	// SuccessURL is where the user goes after the change. Empty means
	// "done/" below the path of this page, the place of Django's
	// password_change_done URL.
	SuccessURL string
	// Validators check the new password. Nil means Config.PasswordValidators.
	Validators []PasswordValidator
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// ServeHTTP implements http.Handler.
func (v *PasswordChangeView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := requireLogin(w, r)
	if !ok {
		return
	}
	if allowMethods(w, r, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodHead) {
		return
	}
	var errs formErrors
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if err := r.ParseForm(); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		password := v.validate(r, u, &errs)
		if errs.empty() {
			db := viewDB(r, v.DB)
			if db == nil {
				serverError(w)
				return
			}
			if err := ChangePassword(r.Context(), db, u, password); err != nil {
				serverError(w)
				return
			}
			if err := UpdateSessionAuthHash(r, u); err != nil {
				serverError(w)
				return
			}
			redirect(w, v.successURL(r))
			return
		}
	}
	data := map[string]any{
		"title":               "Password change",
		"password_help_texts": PasswordValidatorsHelpTexts(v.Validators),
	}
	errs.context(data)
	name := v.TemplateName
	if name == "" {
		name = PasswordChangeTemplate
	}
	render(w, r, v.Templates, name, merge(data, v.ExtraContext))
}

// validate checks the submitted fields like Django's PasswordChangeForm and
// returns the new password. The reasons for a refusal go to errs.
func (v *PasswordChangeView) validate(r *http.Request, u *User, errs *formErrors) string {
	old := r.PostForm.Get("old_password")
	password1, password2 := r.PostForm.Get("new_password1"), r.PostForm.Get("new_password2")
	switch {
	case old == "":
		errs.add("old_password", msgRequired)
	case !u.CheckPassword(old):
		errs.add("old_password", msgPasswordIncorrect)
	}
	if password1 == "" {
		errs.add("new_password1", msgRequired)
	}
	if password2 == "" {
		errs.add("new_password2", msgRequired)
	}
	switch {
	case password1 != "" && password2 != "" && password1 != password2:
		// Django drops the field from cleaned_data with the first error, so
		// the validators do not see a password that was not confirmed.
		errs.add("new_password2", msgPasswordMismatch)
	case password2 != "":
		var refused PasswordErrors
		if err := ValidatePassword(password2, u, v.Validators); errors.As(err, &refused) {
			for _, msg := range refused.Messages() {
				errs.add("new_password2", msg)
			}
		}
	}
	return password1
}

// successURL returns SuccessURL, or "done/" below the path of the request.
func (v *PasswordChangeView) successURL(r *http.Request) string {
	if v.SuccessURL != "" {
		return v.SuccessURL
	}
	return strings.TrimSuffix(r.URL.Path, "/") + "/done/"
}

// PasswordChangeDoneView tells a logged-in user that the password was changed
// (Django's PasswordChangeDoneView). An anonymous user is sent to the login
// page. The context has "title", plus ExtraContext.
type PasswordChangeDoneView struct {
	// Templates renders the page. Nil means the embedded default template.
	Templates Renderer
	// TemplateName is the page template. Empty means
	// PasswordChangeDoneTemplate.
	TemplateName string
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// ServeHTTP implements http.Handler.
func (v *PasswordChangeDoneView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireLogin(w, r); !ok {
		return
	}
	if allowMethods(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	name := v.TemplateName
	if name == "" {
		name = PasswordChangeDoneTemplate
	}
	render(w, r, v.Templates, name, merge(map[string]any{"title": "Password change successful"}, v.ExtraContext))
}

// requireLogin returns the user of the request when it is logged in, and
// otherwise redirects to the login page as LoginRequired does and returns
// false.
func requireLogin(w http.ResponseWriter, r *http.Request) (*User, bool) {
	u, err := GetUser(r)
	if err != nil {
		serverError(w)
		return nil, false
	}
	if !u.IsAuthenticated() {
		newGuard(nil).deny(w, r, u)
		return nil, false
	}
	return u, true
}
