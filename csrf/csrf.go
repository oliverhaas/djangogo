// Package csrf is the port of Django's CsrfViewMiddleware: protection against
// cross-site request forgery for requests that change state.
//
// The middleware keeps a random secret per browser, in a cookie or in the
// session. A form carries the secret in masked form (Token); a request with an
// unsafe method is served only when the token it submits matches the secret
// and its Origin or Referer header names a trusted origin. Settings come in
// through Options; the zero Options is Django's default for every setting.
package csrf

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// SessionKey is the session value that holds the secret when Options.UseSessions
// is on (Django's CSRF_SESSION_KEY).
const SessionKey = "_csrftoken"

// FormField is the name of the form field that carries the token.
const FormField = "csrfmiddlewaretoken"

// HeaderName is the default name of the request header that carries the token
// (Django's CSRF_HEADER_NAME, "HTTP_X_CSRFTOKEN").
const HeaderName = "X-CSRFToken"

// DefaultCookieName is Django's CSRF_COOKIE_NAME.
const DefaultCookieName = "csrftoken"

// DefaultCookieAge is Django's CSRF_COOKIE_AGE: 52 weeks.
const DefaultCookieAge = 52 * 7 * 24 * time.Hour

// Reasons for a rejected request, with Django's wording. The failure handler
// receives one of them, or one of the texts that name the offending origin,
// referer or token source.
const (
	ReasonNoReferer        = "Referer checking failed - no Referer."
	ReasonMalformedReferer = "Referer checking failed - Referer is malformed."
	ReasonInsecureReferer  = "Referer checking failed - Referer is insecure while host is secure."
	ReasonNoCSRFCookie     = "CSRF cookie not set."
	ReasonCSRFTokenMissing = "CSRF token missing."
)

// Formats of the reasons that quote a value of the request.
const (
	reasonBadOrigin  = "Origin checking failed - %s does not match any trusted origins."
	reasonBadReferer = "Referer checking failed - %s does not match any trusted origins."
)

// Options are the CSRF settings of the middleware. The zero value is Django's
// default for every setting.
type Options struct {
	// UseSessions keeps the secret in the session instead of a cookie
	// (CSRF_USE_SESSIONS). The sessions middleware must run first.
	UseSessions bool
	// CookieName is the name of the cookie that holds the secret
	// (CSRF_COOKIE_NAME). Empty means "csrftoken".
	CookieName string
	// CookieAge is the lifetime of the cookie (CSRF_COOKIE_AGE). Zero means 52
	// weeks. A negative value issues a cookie that the browser drops when it
	// closes, which is Django's None.
	CookieAge time.Duration
	// CookieDomain is the Domain attribute (CSRF_COOKIE_DOMAIN). It is also
	// the domain a Referer must belong to; empty means the host of the
	// request. With UseSessions set it to the domain of the session cookie.
	CookieDomain string
	// CookiePath is the Path attribute (CSRF_COOKIE_PATH). Empty means "/".
	CookiePath string
	// CookieSecure sets the Secure attribute (CSRF_COOKIE_SECURE).
	CookieSecure bool
	// CookieHTTPOnly sets the HttpOnly attribute (CSRF_COOKIE_HTTPONLY).
	// Scripts that send the token in a header must then read it from the page
	// instead of the cookie.
	CookieHTTPOnly bool
	// CookieSameSite is the SameSite attribute (CSRF_COOKIE_SAMESITE). The
	// zero value means Lax. http.SameSiteDefaultMode leaves the attribute out.
	CookieSameSite http.SameSite
	// HeaderName is the request header that carries the token
	// (CSRF_HEADER_NAME). Empty means "X-CSRFToken".
	HeaderName string
	// TrustedOrigins lists origins that may send unsafe requests besides the
	// origin of the site itself (CSRF_TRUSTED_ORIGINS). An entry has a scheme
	// and a host, as in "https://api.example.com". A host that starts with
	// "*." matches every subdomain, as in "https://*.example.com".
	TrustedOrigins []string
	// FailureHandler answers a rejected request (CSRF_FAILURE_VIEW). The
	// default writes a 403 without the reason. The reason quotes headers of
	// the request, so a handler that shows it must escape it.
	FailureHandler func(w http.ResponseWriter, r *http.Request, reason string)
	// IsSecure reports whether the browser reached the site over HTTPS
	// (Django's request.is_secure). The default looks at the TLS state of the
	// connection. Behind a proxy that terminates TLS, supply a function that
	// reads the header the proxy sets, or every request with an Origin header
	// is rejected.
	IsSecure func(r *http.Request) bool
	// Resolve returns the handler that will serve r. The middleware asks it
	// whether the handler was marked with Exempt. Without it Exempt has no
	// effect and every unsafe request is checked.
	Resolve func(r *http.Request) http.Handler
}

// stateKey is the context key of the per-request state.
type stateKey struct{}

// state is what the middleware knows about one request (Django keeps it in
// request.META).
type state struct {
	// secret is the unmasked secret in use, "CSRF_COOKIE" in Django. It is
	// empty until the request brings one or a token is asked for.
	secret string
	// needsUpdate reports that the secret has to be sent to the browser or
	// stored in the session ("CSRF_COOKIE_NEEDS_UPDATE").
	needsUpdate bool
}

// stateFrom returns the state the middleware put into ctx, or nil.
func stateFrom(ctx context.Context) *state {
	st, _ := ctx.Value(stateKey{}).(*state)
	return st
}

// Token returns the CSRF token for a form of the request ctx belongs to
// (Django's get_token). The token is the secret under a fresh random mask, so
// it differs on every call; that keeps compression attacks such as BREACH
// from recovering it.
//
// The first call makes the middleware send the secret to the browser with the
// response, together with "Vary: Cookie". Call it before the response headers
// are written. Without the middleware the result is empty.
func Token(ctx context.Context) string {
	st := stateFrom(ctx)
	if st == nil {
		return ""
	}
	if st.secret == "" {
		st.secret = newSecret()
	}
	st.needsUpdate = true
	return maskSecret(st.secret)
}

// GetToken is Token for a request.
func GetToken(r *http.Request) string {
	return Token(r.Context())
}

// RotateToken replaces the secret of the request (Django's rotate_token).
// Login calls it, so that a secret set or read by someone else before the
// login is not accepted afterwards.
func RotateToken(r *http.Request) {
	st := stateFrom(r.Context())
	if st == nil {
		return
	}
	st.secret = newSecret()
	st.needsUpdate = true
}

// lazyToken is the template value of the token. It asks for the token when a
// template prints it, so a page that does not show the token sets no cookie.
type lazyToken struct {
	// r is the request the token belongs to.
	r *http.Request
	// once guards token, so one page shows one token however often it prints it.
	once sync.Once
	// token is the token, made on first use.
	token string
}

// String returns the token, making it on the first call.
func (t *lazyToken) String() string {
	t.once.Do(func() { t.token = GetToken(t.r) })
	return t.token
}

// ContextProcessor adds the token to the template context as "csrf_token"
// (Django's context_processors.csrf). The value is a fmt.Stringer that makes
// the token when the template prints it, with {{ csrf_token }} or the
// csrf_token tag; a page that prints neither sets no CSRF cookie.
func ContextProcessor(r *http.Request) map[string]any {
	return map[string]any{"csrf_token": &lazyToken{r: r}}
}

// skipKey is the context key that marks a request as not to be checked.
type skipKey struct{}

// WithoutEnforcement returns a copy of ctx that makes the middleware skip the
// origin and token checks for the request that carries it. Tokens still work.
// It is the counterpart of the flag Django's test client sets unless it was
// built with enforce_csrf_checks=True. Only code on the server can set a
// context value, so a client cannot reach it.
func WithoutEnforcement(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipKey{}, true)
}

// enforcementSkipped reports whether ctx was marked by WithoutEnforcement.
func enforcementSkipped(ctx context.Context) bool {
	skip, _ := ctx.Value(skipKey{}).(bool)
	return skip
}

// exemptHandler is a handler marked by Exempt.
type exemptHandler struct {
	http.Handler
}

// Unwrap returns the wrapped handler.
func (h exemptHandler) Unwrap() http.Handler { return h.Handler }

// Exempt marks a handler as not to be checked (Django's csrf_exempt). Use it
// for endpoints that authenticate every request by other means, such as a
// signed webhook.
//
// The middleware runs before the router picks a handler, so it only sees the
// mark when Options.Resolve tells it which handler will serve the request.
// The mark can sit under other wrappers that have an Unwrap() http.Handler
// method, such as auth.LoginNotRequired.
func Exempt(h http.Handler) http.Handler {
	return exemptHandler{h}
}

// IsExempt reports whether h, or a handler that h wraps, was marked with
// Exempt. It follows the Unwrap() http.Handler methods of the wrappers.
func IsExempt(h http.Handler) bool {
	for h != nil {
		if _, ok := h.(exemptHandler); ok {
			return true
		}
		u, ok := h.(interface{ Unwrap() http.Handler })
		if !ok {
			return false
		}
		h = u.Unwrap()
	}
	return false
}
