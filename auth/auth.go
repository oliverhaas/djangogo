package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// Session values of a logged-in user, with Django's names.
const (
	// SessionKey holds the primary key of the user as a decimal string
	// (Django's SESSION_KEY).
	SessionKey = "_auth_user_id"
	// BackendSessionKey holds the name of the backend that authenticated the
	// user (Django's BACKEND_SESSION_KEY).
	BackendSessionKey = "_auth_user_backend"
	// HashSessionKey holds User.SessionAuthHash at login (Django's
	// HASH_SESSION_KEY). A session whose value no longer matches is dropped,
	// which logs the user out everywhere after a password change.
	HashSessionKey = "_auth_user_hash"
)

// ErrNoSession is returned by the functions that log a request in or out when
// the request has no session: the sessions middleware must run first.
var ErrNoSession = errors.New("auth: the request has no session; the sessions middleware must run first")

// ErrMultipleBackends is returned by LoginRequest for a user that did not come
// from Authenticate while several backends are configured: the backend that
// loads the user on later requests is ambiguous. Set it with User.SetBackend.
var ErrMultipleBackends = errors.New(
	"auth: multiple authentication backends are configured; set the backend of the user with SetBackend")

// sessionAuthSalt is the key salt of Django's get_session_auth_hash, so that a
// hash made here equals the one Django makes for the same key and password.
const sessionAuthSalt = "django.contrib.auth.models.AbstractBaseUser.get_session_auth_hash"

// saltedHMAC is Django's salted_hmac with SHA-256, as a hex string: the HMAC
// of value under the key SHA-256(salt + secret).
func saltedHMAC(salt, value string, secret []byte) string {
	key := sha256.Sum256(append([]byte(salt), secret...))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// SessionAuthHash returns an HMAC of the password field of u under
// Config.SecretKey (Django's get_session_auth_hash). Login stores it in the
// session; it changes with the password, which invalidates the sessions that
// hold the old value.
func (u *User) SessionAuthHash() string {
	return saltedHMAC(sessionAuthSalt, u.Password, secretKey())
}

// sameString compares two strings in constant time.
func sameString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// sessionString returns the string stored in sess under key, or "".
func sessionString(sess *sessions.Session, key string) string {
	v, _ := sess.Get(key)
	s, _ := v.(string)
	return s
}

// Login writes u into the session (the session part of Django's login). A
// session that belongs to another user, or to u under an older password, is
// emptied first; any other session gets a new key, so that a key an attacker
// planted before the login does not identify the logged-in session.
//
// The backend stored is the one that authenticated u; for a user that did not
// come from Authenticate it is the first configured backend. Login does not
// touch the database or the CSRF secret: a view calls LoginRequest.
func Login(sess *sessions.Session, u *User) {
	hash := u.SessionAuthHash()
	if raw, ok := sess.Get(SessionKey); ok {
		id, _ := asUserID(raw)
		if id != u.ID || !sameString(sessionString(sess, HashSessionKey), hash) {
			sess.Flush()
		}
	} else {
		sess.CycleKey()
	}
	backend := u.backend
	if backend == "" {
		if all := backends(); len(all) > 0 {
			backend = all[0].Name()
		}
	}
	sess.Set(SessionKey, strconv.FormatInt(u.ID, 10))
	sess.Set(BackendSessionKey, backend)
	sess.Set(HashSessionKey, hash)
}

// LoginRequest logs u in for the request and the ones that follow (Django's
// login): it writes u into the session as Login does, makes u the user of the
// request, replaces the CSRF secret and stores the time in last_login.
//
// u normally comes from Authenticate. For any other user the single
// configured backend is used, and with several backends the caller chooses
// with User.SetBackend; otherwise the result is ErrMultipleBackends.
func LoginRequest(r *http.Request, db *orm.DB, u *User) error {
	sess, ok := sessions.FromContext(r.Context())
	if !ok {
		return ErrNoSession
	}
	if u.backend == "" {
		all := backends()
		if len(all) != 1 {
			return ErrMultipleBackends
		}
		u.backend = all[0].Name()
	}
	Login(sess, u)
	if st := stateFrom(r.Context()); st != nil {
		st.set(u)
	}
	csrf.RotateToken(r)
	return UpdateLastLogin(r.Context(), db, u)
}

// UpdateLastLogin stores the current time as the last login of u (Django's
// update_last_login).
func UpdateLastLogin(ctx context.Context, db *orm.DB, u *User) error {
	now := time.Now().UTC()
	if _, err := orm.Query[User](db).Filter("id", u.ID).Update(ctx, "last_login", now); err != nil {
		return fmt.Errorf("auth: update last login: %w", err)
	}
	u.LastLogin = now
	return nil
}

// Logout empties the session and retires its key (the session part of
// Django's logout).
func Logout(sess *sessions.Session) {
	sess.Flush()
}

// LogoutRequest logs the user of the request out (Django's logout): it empties
// the session and makes the request anonymous. A request without a session is
// left alone.
func LogoutRequest(r *http.Request) {
	if sess, ok := sessions.FromContext(r.Context()); ok {
		Logout(sess)
	}
	if st := stateFrom(r.Context()); st != nil {
		st.set(nil)
	}
}

// UpdateSessionAuthHash keeps the session of the request valid after the
// password of u changed (Django's update_session_auth_hash). Without it the
// user who changed the password is logged out with every other session. The
// session gets a new key as well. A session that belongs to another user is
// not touched beyond that.
func UpdateSessionAuthHash(r *http.Request, u *User) error {
	sess, ok := sessions.FromContext(r.Context())
	if !ok {
		return ErrNoSession
	}
	sess.CycleKey()
	raw, ok := sess.Get(SessionKey)
	if !ok {
		return nil
	}
	if id, ok := asUserID(raw); !ok || id != u.ID {
		return nil
	}
	sess.Set(HashSessionKey, u.SessionAuthHash())
	if st := stateFrom(r.Context()); st != nil {
		u.backend = sessionString(sess, BackendSessionKey)
		st.set(u)
	}
	return nil
}

// loadUser returns the user the session belongs to, or nil for an anonymous
// session (Django's get_user). The user is loaded through the backend that
// authenticated it. A session whose auth hash does not match the password of
// the user is emptied; one that matches under a fallback secret is signed
// again with the current secret and gets a new key.
func loadUser(ctx context.Context, db *orm.DB, sess *sessions.Session) (*User, error) {
	raw, ok := sess.Get(SessionKey)
	if !ok {
		return nil, nil
	}
	id, ok := asUserID(raw)
	if !ok {
		return nil, nil
	}
	backend := backendByName(sessionString(sess, BackendSessionKey))
	if backend == nil {
		return nil, nil
	}
	u, err := backend.GetUser(ctx, db, id)
	if err != nil || u == nil {
		return nil, err
	}
	u.backend = backend.Name()

	stored, want := sessionString(sess, HashSessionKey), u.SessionAuthHash()
	if stored != "" && sameString(stored, want) {
		return u, nil
	}
	if stored != "" && matchesFallback(u, stored) {
		sess.CycleKey()
		sess.Set(HashSessionKey, want)
		return u, nil
	}
	sess.Flush()
	return nil, nil
}

// matchesFallback reports whether hash is the session auth hash of u under one
// of Config.SecretKeyFallbacks.
func matchesFallback(u *User, hash string) bool {
	matched := false
	for _, key := range settings().SecretKeyFallbacks {
		if sameString(hash, saltedHMAC(sessionAuthSalt, u.Password, []byte(key))) {
			matched = true
		}
	}
	return matched
}

// UserFromSession returns the logged-in user of the session, or (nil, false)
// for an anonymous session, an unknown or inactive user, or a session whose
// auth hash no longer matches. A database error also counts as anonymous; use
// GetUser to see it.
func UserFromSession(ctx context.Context, db *orm.DB, sess *sessions.Session) (*User, bool) {
	u, err := loadUser(ctx, db, sess)
	if err != nil || u == nil {
		return nil, false
	}
	return u, true
}

// asUserID converts the user id of a session to an int64. Login writes a
// decimal string, as Django does; numbers are accepted for sessions that were
// filled by other code.
func asUserID(raw any) (int64, bool) {
	switch v := raw.(type) {
	case string:
		id, err := strconv.ParseInt(v, 10, 64)
		return id, err == nil
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

// userContextKey is the context key of a user set with WithUser.
type userContextKey struct{}

// stateKey is the context key of the state Middleware keeps for a request.
type stateKey struct{}

// requestState is what Middleware knows about one request. It loads the user
// when first asked, so a request that never looks at the user costs no query
// (Django's lazy request.user).
type requestState struct {
	// db and sess are where the user comes from. sess is nil for a request
	// without a session.
	db   *orm.DB
	sess *sessions.Session

	// mu guards the fields below; handlers may share the request between
	// goroutines.
	mu sync.Mutex
	// loaded reports that user and err are set.
	loaded bool
	// user is the user of the request, nil when it is anonymous.
	user *User
	// err is the error of the load. The request is anonymous then.
	err error
}

// stateFrom returns the state Middleware put into ctx, or nil.
func stateFrom(ctx context.Context) *requestState {
	st, _ := ctx.Value(stateKey{}).(*requestState)
	return st
}

// get returns the user of the request, loading it on the first call.
func (st *requestState) get(ctx context.Context) (*User, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.loaded {
		st.loaded = true
		if st.sess != nil {
			st.user, st.err = loadUser(ctx, st.db, st.sess)
		}
	}
	return st.user, st.err
}

// set makes u the user of the request; nil makes the request anonymous.
func (st *requestState) set(u *User) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.loaded, st.user, st.err = true, u, nil
}

// WithUser returns a copy of ctx in which u is the user, whatever the session
// says. A nil u makes the context anonymous. Tests and code that
// authenticates without a session use it.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userContextKey{}, u)
}

// CurrentUser returns the logged-in user of the request ctx belongs to, or
// (nil, false) for an anonymous request. Under Middleware the first call loads
// the user from the database. A database error counts as anonymous; use
// GetUser to see it.
func CurrentUser(ctx context.Context) (*User, bool) {
	if u, ok := ctx.Value(userContextKey{}).(*User); ok {
		return u, u != nil
	}
	st := stateFrom(ctx)
	if st == nil {
		return nil, false
	}
	u, _ := st.get(ctx)
	return u, u != nil
}

// GetUser returns the user of the request (Django's request.user). The result
// is never nil: for an anonymous request it is the zero User, whose
// IsAuthenticated method returns false. The error is a failure to load the
// user from the database.
func GetUser(r *http.Request) (*User, error) {
	ctx := r.Context()
	if u, ok := ctx.Value(userContextKey{}).(*User); ok {
		if u == nil {
			return &User{}, nil
		}
		return u, nil
	}
	st := stateFrom(ctx)
	if st == nil {
		return &User{}, nil
	}
	u, err := st.get(ctx)
	if u == nil {
		return &User{}, err
	}
	return u, nil
}

// Middleware makes the logged-in user of the session available to CurrentUser
// and GetUser (Django's AuthenticationMiddleware). The sessions middleware must
// run first; a request without a session is anonymous.
//
// The user is loaded when a handler first asks for it. The load checks the
// auth hash of the session against the password of the user and empties a
// session that no longer matches.
func Middleware(db *orm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			st := &requestState{db: db}
			if sess, ok := sessions.FromContext(r.Context()); ok {
				st.sess = sess
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), stateKey{}, st)))
		})
	}
}
