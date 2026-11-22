package auth

import (
	"net/http"
	"slices"
)

// LoginOptOut is implemented by a handler that anonymous users can reach
// under LoginRequiredMiddleware, the counterpart of the login_required
// attribute that Django's login_not_required sets on a view. LoginView
// implements it, and LoginNotRequired adds it to any handler.
type LoginOptOut interface {
	// LoginRequired reports whether LoginRequiredMiddleware sends anonymous
	// requests for the handler to the login page.
	LoginRequired() bool
}

// LoginRedirector is implemented by a handler that sets where
// LoginRequiredMiddleware sends anonymous requests for it, the counterpart of
// the login_url and redirect_field_name attributes that Django's
// login_required sets on a view. The handlers of the guards implement it with
// their login URL and redirect field name.
type LoginRedirector interface {
	// LoginRedirect returns the options that apply over the options of the
	// middleware.
	LoginRedirect() []GuardOption
}

// loginNotRequired is the handler LoginNotRequired returns.
type loginNotRequired struct{ http.Handler }

// LoginRequired lets anonymous requests through.
func (loginNotRequired) LoginRequired() bool { return false }

// Unwrap returns the wrapped handler.
func (h loginNotRequired) Unwrap() http.Handler { return h.Handler }

// LoginNotRequired lets anonymous users reach h under LoginRequiredMiddleware,
// Django's login_not_required. Use it for pages such as a sign-up or a
// password reset page.
func LoginNotRequired(h http.Handler) http.Handler { return loginNotRequired{h} }

// LoginRequiredMiddleware returns middleware that sends anonymous users to
// the login page, unless the view of the request opts out with
// LoginNotRequired (Django's LoginRequiredMiddleware). resolve returns the
// view that serves a request, or nil when no route matches; a request without
// a view passes, so that the router answers it with 404. Router.HandlerFor of
// package urls is such a function.
//
// The redirect follows the guard options in opts, and a view that implements
// LoginRedirector overrides them: a view behind LoginRequired("/custom/")
// sends anonymous users to /custom/. The middleware finds LoginOptOut and
// LoginRedirector also on a handler that the view wraps, through the Unwrap
// methods of the wrappers. Middleware must run first. LoginRequiredMiddleware
// panics on a nil resolve.
func LoginRequiredMiddleware(resolve func(r *http.Request) http.Handler, opts ...GuardOption) func(http.Handler) http.Handler {
	if resolve == nil {
		panic("auth: LoginRequiredMiddleware needs a resolve function")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			view := resolve(r)
			if view == nil {
				next.ServeHTTP(w, r)
				return
			}
			if optOut, ok := wrapped[LoginOptOut](view); ok && !optOut.LoginRequired() {
				next.ServeHTTP(w, r)
				return
			}
			u, err := GetUser(r)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if u.IsAuthenticated() {
				next.ServeHTTP(w, r)
				return
			}
			viewOpts := slices.Clone(opts)
			if redirector, ok := wrapped[LoginRedirector](view); ok {
				viewOpts = append(viewOpts, redirector.LoginRedirect()...)
			}
			newGuard(viewOpts).deny(w, r, u)
		})
	}
}

// wrapped returns the first handler of the chain that starts at h and follows
// Unwrap methods that implements T.
func wrapped[T any](h http.Handler) (T, bool) {
	for h != nil {
		if t, ok := h.(T); ok {
			return t, true
		}
		u, ok := h.(interface{ Unwrap() http.Handler })
		if !ok {
			break
		}
		h = u.Unwrap()
	}
	var zero T
	return zero, false
}
