package auth

import (
	"context"
	"net/http"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// sessionUserKey is the session key under which the logged-in user's id is stored.
const sessionUserKey = "_auth_user_id"

// userContextKey is an unexported type for the current-user context key to avoid
// collisions with keys from other packages.
type userContextKey struct{}

// Login associates u with the session. It rotates the session key first as a
// session-fixation defense, then records u's id under sessionUserKey.
func Login(sess *sessions.Session, u *User) {
	sessions.Rotate(sess)
	sess.Set(sessionUserKey, u.ID)
}

// Logout clears the session's data (dropping the auth state) and rotates the
// session key.
func Logout(sess *sessions.Session) {
	sess.Clear()
	sessions.Rotate(sess)
}

// UserFromSession loads the logged-in user referenced by the session, returning
// (nil, false) when the session has no user id or the user no longer exists. The
// stored id may be a JSON-decoded float64 or a native int64, both of which are
// handled.
func UserFromSession(ctx context.Context, db *orm.DB, sess *sessions.Session) (*User, bool) {
	raw, ok := sess.Get(sessionUserKey)
	if !ok {
		return nil, false
	}
	id, ok := asUserID(raw)
	if !ok {
		return nil, false
	}
	u, err := orm.Query[User](db).Get(ctx, "id", id)
	if err != nil {
		return nil, false
	}
	// A deactivated account is treated as anonymous, mirroring Django's
	// ModelBackend.get_user, which returns None when user_can_authenticate
	// (is_active) is false. Without this a user disabled mid-session would stay
	// authenticated until the session expired.
	if !u.IsActive {
		return nil, false
	}
	return &u, true
}

// asUserID coerces a session-stored user id (int64 from in-process sessions or
// float64 from a JSON round-trip) into an int64.
func asUserID(raw any) (int64, bool) {
	switch v := raw.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// WithUser returns a copy of ctx carrying the authenticated user u.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userContextKey{}, u)
}

// CurrentUser returns the authenticated user stored in ctx, if any.
func CurrentUser(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userContextKey{}).(*User)
	return u, ok
}

// Middleware loads the logged-in user from the request's session (placed in the
// context by sessions.Middleware, which must run first) and attaches it to the
// request context. Anonymous requests, or requests without a session, pass
// through unchanged.
func Middleware(db *orm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sess, ok := sessions.FromContext(ctx)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			u, ok := UserFromSession(ctx, db, sess)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(ctx, u)))
		})
	}
}
