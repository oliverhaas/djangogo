package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/oliverhaas/djangogo/orm"
)

// guard is the configuration of an access guard (Django's AccessMixin).
type guard struct {
	// loginURL is where an anonymous user is sent. Empty means
	// Config.LoginURL.
	loginURL string
	// redirectField is the query parameter that carries the page to return
	// to. Empty means that the redirect has no such parameter.
	redirectField string
	// raise answers 403 to anonymous users too.
	raise bool
	// forbidden writes the 403 response.
	forbidden http.Handler
	// message tells the forbidden handler why the request was denied.
	message string
	// db is the database of the permission check. Nil means the database
	// of Middleware.
	db *orm.DB
}

// GuardOption changes how a guard answers a request it does not let through.
type GuardOption func(*guard)

// WithLoginURL sends anonymous users to loginURL (Django's login_url). The
// default is Config.LoginURL.
func WithLoginURL(loginURL string) GuardOption {
	return func(g *guard) { g.loginURL = loginURL }
}

// WithRedirectFieldName names the query parameter that tells the login page
// where to return to (Django's redirect_field_name). The default is "next";
// with an empty name the login URL gets no such parameter.
func WithRedirectFieldName(name string) GuardOption {
	return func(g *guard) { g.redirectField = name }
}

// WithRaiseException answers 403 to anonymous users as well (Django's
// raise_exception). The default sends them to the login page.
func WithRaiseException() GuardOption {
	return func(g *guard) { g.raise = true }
}

// WithForbiddenHandler makes h write the response for a user that is denied.
// h is responsible for the status code. The default is a bare 403 page.
func WithForbiddenHandler(h http.Handler) GuardOption {
	return func(g *guard) { g.forbidden = h }
}

// WithPermissionDeniedMessage attaches msg to a request the guard answers with
// 403 (Django's permission_denied_message). The default 403 page does not
// show it; a handler of WithForbiddenHandler reads it with
// PermissionDeniedMessage.
func WithPermissionDeniedMessage(msg string) GuardOption {
	return func(g *guard) { g.message = msg }
}

// permissionDeniedKey is the context key of the message of a denied request.
type permissionDeniedKey struct{}

// PermissionDeniedMessage returns the message of the guard that denied the
// request, or "" when the guard has none. It is meant for the handler that
// writes the 403 page.
func PermissionDeniedMessage(ctx context.Context) string {
	msg, _ := ctx.Value(permissionDeniedKey{}).(string)
	return msg
}

// WithGuardDB makes a permission guard check permissions in db. Without it the
// guard uses the database of Middleware.
func WithGuardDB(db *orm.DB) GuardOption {
	return func(g *guard) { g.db = db }
}

// newGuard applies opts to the defaults.
func newGuard(opts []GuardOption) *guard {
	g := &guard{redirectField: DefaultRedirectFieldName, forbidden: http.HandlerFunc(forbidden)}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// forbidden writes the 403 page Django shows without a 403.html template.
func forbidden(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("<h1>403 Forbidden</h1>"))
}

// deny answers a request that did not pass (Django's handle_no_permission): a
// logged-in user gets 403, because a second login would not change the
// outcome; an anonymous user is sent to the login page.
func (g *guard) deny(w http.ResponseWriter, r *http.Request, u *User) {
	if g.raise || u.IsAuthenticated() {
		if g.message != "" {
			r = r.WithContext(context.WithValue(r.Context(), permissionDeniedKey{}, g.message))
		}
		g.forbidden.ServeHTTP(w, r)
		return
	}
	loginURL := g.loginURL
	if loginURL == "" {
		loginURL = configuredLoginURL()
	}
	redirect(w, RedirectToLoginURL(nextFor(r, loginURL), loginURL, g.redirectField))
}

// nextFor returns the address of the request for the login page to return to:
// the path and query when the login page is on the same scheme and host, and
// the absolute URL otherwise.
func nextFor(r *http.Request, loginURL string) string {
	scheme := "http"
	if requestIsSecure(r) {
		scheme = "https"
	}
	full := r.URL.RequestURI()
	loginScheme, loginHost, err := splitURL(loginURL)
	if err == nil && (loginScheme == "" || loginScheme == scheme) && (loginHost == "" || loginHost == r.Host) {
		return full
	}
	return scheme + "://" + r.Host + full
}

// wrap returns middleware that lets a request through when test passes for
// its user.
func (g *guard) wrap(test func(r *http.Request, u *User) (bool, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return &guardHandler{guard: g, test: test, next: next}
	}
}

// guardHandler is the handler a guard wraps a view in.
type guardHandler struct {
	guard *guard
	test  func(r *http.Request, u *User) (bool, error)
	next  http.Handler
}

// ServeHTTP serves the view when the test passes for the user of the request.
// An error of the test is a 500: the guard cannot tell whether the user may
// pass.
func (h *guardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, err := GetUser(r)
	if err == nil {
		var ok bool
		if ok, err = h.test(r, u); err == nil && ok {
			h.next.ServeHTTP(w, r)
			return
		}
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.guard.deny(w, r, u)
}

// Unwrap returns the view.
func (h *guardHandler) Unwrap() http.Handler { return h.next }

// LoginRedirect returns the login URL and the redirect field name of the
// guard for LoginRequiredMiddleware, as Django's user_passes_test sets them
// on the view.
func (h *guardHandler) LoginRedirect() []GuardOption {
	opts := []GuardOption{WithRedirectFieldName(h.guard.redirectField)}
	if h.guard.loginURL != "" {
		opts = append(opts, WithLoginURL(h.guard.loginURL))
	}
	return opts
}

// LoginRequired returns middleware that lets logged-in users through and
// sends everyone else to the login page, with the address of the request in
// the next parameter (Django's login_required and LoginRequiredMixin). An
// empty loginURL means Config.LoginURL. Middleware must run first.
func LoginRequired(loginURL string, opts ...GuardOption) func(http.Handler) http.Handler {
	g := newGuard(append([]GuardOption{WithLoginURL(loginURL)}, opts...))
	return g.wrap(func(_ *http.Request, u *User) (bool, error) {
		return u.IsAuthenticated(), nil
	})
}

// UserPassesTest returns middleware that lets a request through when test
// returns true for its user (Django's user_passes_test and
// UserPassesTestMixin). For an anonymous request test gets the zero User. A
// logged-in user that fails gets 403; an anonymous one is sent to the login
// page, or gets 403 too under WithRaiseException.
//
// Django's decorator sends a logged-in user that fails to the login page as
// well. The guards follow the mixins, which answer 403: logging in again does
// not help that user.
func UserPassesTest(test func(u *User) bool, opts ...GuardOption) func(http.Handler) http.Handler {
	return newGuard(opts).wrap(func(_ *http.Request, u *User) (bool, error) {
		return test(u), nil
	})
}

// StaffRequired returns middleware that lets active staff users through
// (Django's staff_member_required).
func StaffRequired(opts ...GuardOption) func(http.Handler) http.Handler {
	return UserPassesTest(func(u *User) bool { return u.IsActive && u.IsStaff }, opts...)
}

// PermissionRequired returns middleware that lets a user through that holds
// the permission perm, written "<app_label>.<codename>" (Django's
// permission_required and PermissionRequiredMixin). It answers like
// UserPassesTest. The permission is checked in the database of Middleware, or
// in the one of WithGuardDB.
func PermissionRequired(perm string, opts ...GuardOption) func(http.Handler) http.Handler {
	return PermissionsRequired([]string{perm}, opts...)
}

// PermissionsRequired is PermissionRequired for a user that has to hold every
// permission of perms.
func PermissionsRequired(perms []string, opts ...GuardOption) func(http.Handler) http.Handler {
	g := newGuard(opts)
	return g.wrap(func(r *http.Request, u *User) (bool, error) {
		db := g.db
		if db == nil {
			db = requestDB(r.Context())
		}
		if db == nil {
			return false, errNoDatabase
		}
		return HasPerms(r.Context(), db, u, perms...)
	})
}

// errNoDatabase reports a permission guard that has no database to ask:
// Middleware did not run and WithGuardDB was not given.
var errNoDatabase = errors.New("auth: permission guard has no database; run Middleware first or pass WithGuardDB")

// requestDB returns the database Middleware was built with, or nil.
func requestDB(ctx context.Context) *orm.DB {
	if st := stateFrom(ctx); st != nil {
		return st.db
	}
	return nil
}
