package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// Template names of the password reset views, as in Django.
const (
	PasswordResetTemplate         = "registration/password_reset_form.html"
	PasswordResetDoneTemplate     = "registration/password_reset_done.html"
	PasswordResetConfirmTemplate  = "registration/password_reset_confirm.html"
	PasswordResetCompleteTemplate = "registration/password_reset_complete.html"
)

// InternalResetSessionToken is the session key under which
// PasswordResetConfirmView keeps the token of a reset link while the user
// enters the new password (Django's INTERNAL_RESET_SESSION_TOKEN).
const InternalResetSessionToken = "_password_reset_token"

// DefaultResetURLToken is the text that takes the place of the token in the
// path of the password form (Django's PasswordResetConfirmView.reset_url_token).
const DefaultResetURLToken = "set-password"

// PasswordResetView asks for an email address and mails a password reset
// link to the users with that address (Django's PasswordResetView). GET shows
// the form; a valid POST sends the emails with PasswordResetForm.Save and
// redirects to SuccessURL, also when no user has the address. The link is
// https when the request is secure. The domain in the email is the host of
// the request: run the view behind middleware.AllowedHosts, as an
// Application does, so that a forged Host header cannot reach the email.
//
// The context has "form" (a PasswordResetForm), "title" and "csrf_token",
// plus ExtraContext. The email templates are rendered with Templates when it
// is an EmailRenderer, as *templates.Engine is, and with the embedded
// defaults otherwise. The default body names the confirmation page with
// {% url 'password_reset_confirm' uidb64=uid token=token %}, which needs the
// routes of URLs and a URL resolver, as an Application sets.
type PasswordResetView struct {
	// DB holds the users. Nil means the database of Middleware.
	DB *orm.DB
	// Templates renders the page and the email. Nil means the embedded
	// default templates.
	Templates Renderer
	// TemplateName is the page template. Empty means PasswordResetTemplate.
	TemplateName string
	// SuccessURL is where the user goes after the form. Empty means "done/"
	// below the path of this page, the place of Django's
	// password_reset_done URL.
	SuccessURL string
	// SubjectTemplateName renders the subject of the email. Empty means
	// PasswordResetSubjectTemplate.
	SubjectTemplateName string
	// EmailTemplateName renders the body of the email. Empty means
	// PasswordResetEmailTemplate.
	EmailTemplateName string
	// HTMLEmailTemplateName, when set, renders an HTML alternative of the
	// body.
	HTMLEmailTemplateName string
	// ExtraEmailContext is added to the context of the email templates.
	ExtraEmailContext map[string]any
	// FromEmail is the sender of the email. Empty means the default sender
	// of the mail package.
	FromEmail string
	// TokenGenerator makes the token of the link. Nil means
	// DefaultTokenGenerator.
	TokenGenerator *PasswordResetTokenGenerator
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// LoginRequired reports false: the page is for users who cannot log in, and
// LoginRequiredMiddleware lets everyone reach it.
func (*PasswordResetView) LoginRequired() bool { return false }

// ServeHTTP implements http.Handler.
func (v *PasswordResetView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if allowMethods(w, r, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodHead) {
		return
	}
	form := NewPasswordResetForm()
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if err := r.ParseForm(); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		form = form.Bind(r.PostForm)
		if form.IsValid() {
			db := viewDB(r, v.DB)
			if db == nil {
				serverError(w)
				return
			}
			emails, _ := v.Templates.(EmailRenderer)
			err := form.Save(r.Context(), db, PasswordResetOptions{
				Request:               r,
				UseHTTPS:              requestIsSecure(r),
				SubjectTemplateName:   v.SubjectTemplateName,
				EmailTemplateName:     v.EmailTemplateName,
				HTMLEmailTemplateName: v.HTMLEmailTemplateName,
				ExtraEmailContext:     v.ExtraEmailContext,
				FromEmail:             v.FromEmail,
				TokenGenerator:        v.TokenGenerator,
				Templates:             emails,
			})
			if err != nil {
				logServerError(r, err)
				serverError(w)
				return
			}
			redirect(w, v.successURL(r))
			return
		}
	}
	data := map[string]any{"form": form, "title": "Password reset", "subtitle": nil}
	render(w, r, v.Templates, templateOr(v.TemplateName, PasswordResetTemplate), merge(data, v.ExtraContext))
}

// successURL returns SuccessURL, or "done/" below the path of the request.
func (v *PasswordResetView) successURL(r *http.Request) string {
	if v.SuccessURL != "" {
		return v.SuccessURL
	}
	return strings.TrimSuffix(r.URL.Path, "/") + "/done/"
}

// PasswordResetDoneView tells the user that the reset email is on its way
// (Django's PasswordResetDoneView). The context has "title", plus
// ExtraContext.
type PasswordResetDoneView struct {
	// Templates renders the page. Nil means the embedded default template.
	Templates Renderer
	// TemplateName is the page template. Empty means
	// PasswordResetDoneTemplate.
	TemplateName string
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// LoginRequired reports false, as for PasswordResetView.
func (*PasswordResetDoneView) LoginRequired() bool { return false }

// ServeHTTP implements http.Handler.
func (v *PasswordResetDoneView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if allowMethods(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	data := map[string]any{"title": "Password reset sent", "subtitle": nil}
	render(w, r, v.Templates, templateOr(v.TemplateName, PasswordResetDoneTemplate), merge(data, v.ExtraContext))
}

// PasswordResetConfirmView checks the link of a reset email and lets the
// user set a new password (Django's PasswordResetConfirmView). Its route
// captures the encoded user id as "uidb64" and the token as "token", for
// example urls.Path("reset/<uidb64>/<token>/", ...) or the ServeMux pattern
// "/reset/{uidb64}/{token}/"; a route without them is a configuration error
// and is answered with 500.
//
// A valid link is redirected to the same path with ResetURLToken in place of
// the token, which moves to the session, so that the token does not leak in
// the Referer header of the next request. There the view shows a
// SetPasswordForm; a valid POST stores the password, removes the token from
// the session, logs the user in when PostResetLogin is set and redirects to
// SuccessURL. Responses are marked as not cacheable.
//
// The context has "validlink", "form" (a SetPasswordForm, or nil for an
// invalid link), "title" and "csrf_token", plus ExtraContext.
type PasswordResetConfirmView struct {
	// DB holds the users. Nil means the database of Middleware.
	DB *orm.DB
	// Templates renders the page. Nil means the embedded default template.
	Templates Renderer
	// TemplateName is the page template. Empty means
	// PasswordResetConfirmTemplate.
	TemplateName string
	// SuccessURL is where the user goes after the reset. Empty means the
	// path with "done/" in place of the user id and the token, the place of
	// Django's password_reset_complete URL.
	SuccessURL string
	// PostResetLogin logs the user in after the reset.
	PostResetLogin bool
	// PostResetLoginBackend names the backend of that login. Empty means
	// the single configured backend.
	PostResetLoginBackend string
	// ResetURLToken replaces the token in the path of the password form.
	// Empty means DefaultResetURLToken.
	ResetURLToken string
	// TokenGenerator checks the token. Nil means DefaultTokenGenerator.
	TokenGenerator *PasswordResetTokenGenerator
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// LoginRequired reports false, as for PasswordResetView.
func (*PasswordResetConfirmView) LoginRequired() bool { return false }

// ServeHTTP implements http.Handler.
func (v *PasswordResetConfirmView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	neverCache(w)
	uidb64, token := r.PathValue("uidb64"), r.PathValue("token")
	if uidb64 == "" || token == "" {
		slog.ErrorContext(r.Context(), "The URL path must contain 'uidb64' and 'token' parameters.")
		serverError(w)
		return
	}
	sess, ok := sessions.FromContext(r.Context())
	if !ok {
		serverError(w)
		return
	}
	db := viewDB(r, v.DB)
	if db == nil {
		serverError(w)
		return
	}
	user, err := resetUser(r, db, uidb64)
	if err != nil {
		logServerError(r, err)
		serverError(w)
		return
	}
	generator := v.TokenGenerator
	if generator == nil {
		generator = DefaultTokenGenerator
	}
	resetToken := v.ResetURLToken
	if resetToken == "" {
		resetToken = DefaultResetURLToken
	}
	if user != nil {
		if token == resetToken {
			sessionToken, _ := sess.Get(InternalResetSessionToken)
			if s, _ := sessionToken.(string); generator.CheckToken(user, s) {
				v.serveForm(w, r, db, sess, user)
				return
			}
		} else if generator.CheckToken(user, token) {
			sess.Set(InternalResetSessionToken, token)
			redirect(w, strings.ReplaceAll(r.URL.Path, token, resetToken))
			return
		}
	}
	data := map[string]any{"validlink": false, "form": nil, "title": "Password reset unsuccessful", "subtitle": nil}
	render(w, r, v.Templates, templateOr(v.TemplateName, PasswordResetConfirmTemplate), merge(data, v.ExtraContext))
}

// serveForm serves the password form of a valid link for user.
func (v *PasswordResetConfirmView) serveForm(w http.ResponseWriter, r *http.Request, db *orm.DB, sess *sessions.Session, user *User) {
	if allowMethods(w, r, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodHead) {
		return
	}
	form := NewSetPasswordForm(user)
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if err := r.ParseForm(); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		form = form.Bind(r.PostForm)
		if form.IsValid() {
			if _, err := form.Save(r.Context(), db); err != nil {
				logServerError(r, err)
				serverError(w)
				return
			}
			sess.Delete(InternalResetSessionToken)
			if v.PostResetLogin {
				if v.PostResetLoginBackend != "" {
					user.SetBackend(v.PostResetLoginBackend)
				}
				if err := LoginRequest(r, db, user); err != nil {
					logServerError(r, err)
					serverError(w)
					return
				}
			}
			redirect(w, v.successURL(r))
			return
		}
	}
	data := map[string]any{"validlink": true, "form": form, "title": "Enter new password", "subtitle": nil}
	render(w, r, v.Templates, templateOr(v.TemplateName, PasswordResetConfirmTemplate), merge(data, v.ExtraContext))
}

// successURL returns SuccessURL, or the path with "done/" in place of its
// last two segments.
func (v *PasswordResetConfirmView) successURL(r *http.Request) string {
	if v.SuccessURL != "" {
		return v.SuccessURL
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	for range 2 {
		if i := strings.LastIndex(path, "/"); i >= 0 {
			path = path[:i]
		}
	}
	return path + "/done/"
}

// resetUser returns the user whose id uidb64 encodes, or nil when it encodes
// no id or no user has it (Django's PasswordResetConfirmView.get_user).
func resetUser(r *http.Request, db *orm.DB, uidb64 string) (*User, error) {
	id, ok := decodeUID(uidb64)
	if !ok {
		return nil, nil
	}
	u, err := orm.Query[User](db).Get(r.Context(), "id", id)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// decodeUID returns the user id that uidb64 encodes, and false when it
// encodes no integer.
func decodeUID(uidb64 string) (int64, bool) {
	raw, err := URLSafeBase64Decode(uidb64)
	if err != nil {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	return id, err == nil
}

// PasswordResetCompleteView tells the user that the password is set (Django's
// PasswordResetCompleteView). The context has "title" and "login_url", which
// is Config.LoginURL, plus ExtraContext.
type PasswordResetCompleteView struct {
	// Templates renders the page. Nil means the embedded default template.
	Templates Renderer
	// TemplateName is the page template. Empty means
	// PasswordResetCompleteTemplate.
	TemplateName string
	// ExtraContext is added to the template context.
	ExtraContext map[string]any
}

// LoginRequired reports false, as for PasswordResetView.
func (*PasswordResetCompleteView) LoginRequired() bool { return false }

// ServeHTTP implements http.Handler.
func (v *PasswordResetCompleteView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if allowMethods(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	data := map[string]any{"title": "Password reset complete", "subtitle": nil, "login_url": configuredLoginURL()}
	render(w, r, v.Templates, templateOr(v.TemplateName, PasswordResetCompleteTemplate), merge(data, v.ExtraContext))
}

// templateOr returns name, or fallback when name is empty.
func templateOr(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

// logServerError logs err, the cause of a 500 response, as Django's
// django.request logger does for an exception in a view.
func logServerError(r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "Internal Server Error: "+r.URL.Path,
		"status", http.StatusInternalServerError, "method", r.Method, "error", err)
}
