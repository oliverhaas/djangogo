package auth

import (
	"net/http"
	"net/url"

	"github.com/oliverhaas/djangogo/orm"
)

// LoginRequired returns middleware that redirects anonymous requests to loginURL
// with a 302 and a ?next=<request path> query parameter, and otherwise calls the
// wrapped handler. A user is considered authenticated when CurrentUser finds one
// in the request context (so auth.Middleware must run first).
func LoginRequired(loginURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := CurrentUser(r.Context()); !ok {
				redirectToLogin(w, r, loginURL)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// redirectToLogin issues a 302 to loginURL carrying the current request path in a
// next query parameter so the login flow can return the user afterwards.
func redirectToLogin(w http.ResponseWriter, r *http.Request, loginURL string) {
	target := loginURL + "?next=" + url.QueryEscape(r.URL.Path)
	http.Redirect(w, r, target, http.StatusFound)
}

// PermissionRequired returns middleware that guards the wrapped handler by the
// permission perm ("<app_label>.<codename>"): anonymous requests are redirected
// to loginURL, an authenticated user lacking the permission gets a 403, and a
// user holding it proceeds.
func PermissionRequired(db *orm.DB, perm, loginURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			u, ok := CurrentUser(ctx)
			if !ok {
				redirectToLogin(w, r, loginURL)
				return
			}
			allowed, err := HasPerm(ctx, db, u, perm)
			if err != nil {
				http.Error(w, "permission check failed", http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
