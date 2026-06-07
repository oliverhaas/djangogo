// Package csrf provides a CSRF enforcement middleware for HTTP handlers.
//
// It must be installed after the sessions middleware (it reads/writes the
// session via sessions.FromContext). If no session is present in the context
// the middleware skips token handling for safe methods and rejects unsafe
// methods with 403 (there is no session token to validate against).
package csrf

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/oliverhaas/djangogo/sessions"
)

// SessionKey is the session entry holding the CSRF token.
const SessionKey = "_csrf_token"

// FormField is the POST field carrying the token.
const FormField = "csrfmiddlewaretoken"

// HeaderName is the request header carrying the token.
const HeaderName = "X-CSRFToken"

// tokenContextKey is the unexported context key used to store the CSRF token.
type tokenContextKey struct{}

// Token returns the CSRF token bound to ctx by Middleware. It returns an empty
// string when no token has been seeded (i.e. Middleware was not in the chain).
func Token(ctx context.Context) string {
	t, _ := ctx.Value(tokenContextKey{}).(string)
	return t
}

// safeMethods lists the HTTP methods that do not mutate server state and
// therefore do not require CSRF validation.
var safeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

// Middleware ensures a per-session CSRF token exists and is exposed via
// Token(ctx), and rejects unsafe-method requests whose submitted token does
// not match the session token.
//
// It must run after the sessions middleware: it reads and writes the session
// via sessions.FromContext. When no session is present in the context the
// middleware skips token bootstrapping; unsafe methods are still rejected with
// 403 because there is no session token to validate against.
//
// For unsafe methods the submitted token is read from the FormField form value
// (r.ParseForm is called first) or, if that is empty, from the HeaderName
// request header. A constant-time comparison guards against timing attacks.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, hasSession := sessions.FromContext(r.Context())

		var sessionToken string
		if hasSession {
			if v, ok := sess.Get(SessionKey); ok {
				sessionToken, _ = v.(string)
			}
			if sessionToken == "" {
				sessionToken = newToken()
				sess.Set(SessionKey, sessionToken)
			}
			r = r.WithContext(context.WithValue(r.Context(), tokenContextKey{}, sessionToken))
		}

		if safeMethods[r.Method] {
			next.ServeHTTP(w, r)
			return
		}

		// Unsafe method: validate the submitted token.
		if !validateToken(r, sessionToken) {
			http.Error(w, "CSRF verification failed", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validateToken extracts the submitted CSRF token from the request and
// performs a constant-time comparison against the session token. It returns
// false when either value is empty or they do not match.
func validateToken(r *http.Request, sessionToken string) bool {
	if sessionToken == "" {
		return false
	}

	// ParseForm is idempotent and needed to populate r.Form/r.PostForm.
	_ = r.ParseForm()

	submitted := r.PostFormValue(FormField)
	if submitted == "" {
		submitted = r.Header.Get(HeaderName)
	}

	if submitted == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(submitted), []byte(sessionToken)) == 1
}

// newToken generates a cryptographically random 32-byte token encoded as
// URL-safe base64 without padding.
func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never errors on supported platforms; panic rather
		// than silently issuing a predictable token.
		panic("csrf: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
