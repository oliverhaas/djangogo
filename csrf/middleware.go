package csrf

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/sessions"
)

// failureMessage is the body of the default answer to a rejected request, the
// headline of Django's 403 page.
const failureMessage = "CSRF verification failed. Request aborted."

// config is Options with the defaults applied and the trusted origins parsed.
type config struct {
	Options
	// trustedExact holds the trusted origins without a wildcard.
	trustedExact map[string]bool
	// trustedSubdomains maps a scheme to the host suffixes, each with its
	// leading dot, whose subdomains are trusted for that scheme.
	trustedSubdomains map[string][]string
	// trustedHosts holds the host of every trusted origin, wildcards as a
	// suffix with a leading dot. The Referer check uses it.
	trustedHosts []string
}

// newConfig applies Django's defaults to opts and parses the trusted origins.
func newConfig(opts Options) *config {
	if opts.CookieName == "" {
		opts.CookieName = DefaultCookieName
	}
	if opts.CookieAge == 0 {
		opts.CookieAge = DefaultCookieAge
	}
	if opts.CookiePath == "" {
		opts.CookiePath = "/"
	}
	if opts.CookieSameSite == 0 {
		opts.CookieSameSite = http.SameSiteLaxMode
	}
	if opts.HeaderName == "" {
		opts.HeaderName = HeaderName
	}
	if opts.FailureHandler == nil {
		opts.FailureHandler = defaultFailureHandler
	}
	if opts.IsSecure == nil {
		opts.IsSecure = func(r *http.Request) bool { return r.TLS != nil }
	}
	cfg := &config{
		Options:           opts,
		trustedExact:      make(map[string]bool),
		trustedSubdomains: make(map[string][]string),
	}
	for _, origin := range opts.TrustedOrigins {
		scheme, rest, ok := strings.Cut(origin, "://")
		if !ok {
			continue
		}
		host, _, _ := strings.Cut(rest, "/")
		host = strings.ToLower(strings.TrimLeft(host, "*"))
		cfg.trustedHosts = append(cfg.trustedHosts, host)
		if strings.Contains(origin, "*") {
			cfg.trustedSubdomains[scheme] = append(cfg.trustedSubdomains[scheme], host)
		} else {
			cfg.trustedExact[origin] = true
		}
	}
	return cfg
}

// defaultFailureHandler answers 403 with a fixed text. The reason stays out of
// the response, as on Django's page when DEBUG is off.
func defaultFailureHandler(w http.ResponseWriter, _ *http.Request, _ string) {
	http.Error(w, failureMessage, http.StatusForbidden)
}

// Middleware is MiddlewareWithOptions with Django's defaults: the secret lives
// in the "csrftoken" cookie.
func Middleware(next http.Handler) http.Handler {
	return MiddlewareWithOptions(Options{})(next)
}

// MiddlewareWithOptions returns the CSRF middleware (Django's
// CsrfViewMiddleware).
//
// A request with the method GET, HEAD, OPTIONS or TRACE passes. Any other
// request must come from a trusted origin: its Origin header, or over HTTPS
// its Referer header when there is no Origin, has to name the site itself or
// an entry of Options.TrustedOrigins. It must also carry a token that matches
// the secret of the browser, in the form field "csrfmiddlewaretoken" of a POST
// or in the header Options.HeaderName. A request that fails goes to
// Options.FailureHandler with the reason.
//
// The secret reaches the browser only when a handler asks for a token, so
// pages without a form stay cacheable.
func MiddlewareWithOptions(opts Options) func(http.Handler) http.Handler {
	cfg := newConfig(opts)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var sess *sessions.Session
			if cfg.UseSessions {
				s, ok := sessions.FromContext(r.Context())
				if !ok {
					http.Error(w, "csrf: UseSessions is on but the request has no session; "+
						"the sessions middleware must run first", http.StatusInternalServerError)
					return
				}
				sess = s
			}

			st := &state{}
			secret, formatReason := cfg.requestSecret(r, sess)
			switch {
			case formatReason != "":
				// A cookie of the wrong form is replaced with the response.
				st.secret = newSecret()
				st.needsUpdate = true
			case secret != "":
				st.secret = secret
			}
			r = r.WithContext(context.WithValue(r.Context(), stateKey{}, st))

			cw := &responseWriter{ResponseWriter: w, cfg: cfg, st: st, sess: sess}
			if reason := cfg.check(r, secret, formatReason); reason != "" {
				cfg.FailureHandler(cw, r, reason)
			} else {
				next.ServeHTTP(cw, r)
			}
			cw.finish()
		})
	}
}

// requestSecret returns the secret the request brought, unmasked, or "" when
// it brought none (Django's _get_secret). formatReason is set when the cookie
// has the wrong length or characters.
func (c *config) requestSecret(r *http.Request, sess *sessions.Session) (secret, formatReason string) {
	if c.UseSessions {
		v, _ := sess.Get(SessionKey)
		secret, _ = v.(string)
		if checkFormat(secret) != "" {
			return "", ""
		}
	} else {
		cookie, err := r.Cookie(c.CookieName)
		if err != nil {
			return "", ""
		}
		if reason := checkFormat(cookie.Value); reason != "" {
			return "", reason
		}
		secret = cookie.Value
	}
	if len(secret) == tokenLength {
		secret = unmaskToken(secret)
	}
	return secret, ""
}

// safeMethods are the methods that must not change state and are not checked.
var safeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

// check decides whether the request may pass (Django's process_view). It
// returns the reason for a rejection, or "".
func (c *config) check(r *http.Request, secret, formatReason string) string {
	if c.Resolve != nil && IsExempt(c.Resolve(r)) {
		return ""
	}
	if safeMethods[r.Method] || enforcementSkipped(r.Context()) {
		return ""
	}
	if _, ok := r.Header["Origin"]; ok {
		if reason := c.checkOrigin(r); reason != "" {
			return reason
		}
	} else if c.IsSecure(r) {
		// Without an Origin header the token alone would let a network
		// attacker who controls a plain-HTTP subdomain set the cookie and
		// forge a request to the HTTPS site. The strict Referer check closes
		// that hole.
		if reason := c.checkReferer(r); reason != "" {
			return reason
		}
	}
	return c.checkToken(r, secret, formatReason)
}

// checkToken compares the token of the request with the secret (Django's
// _check_token).
func (c *config) checkToken(r *http.Request, secret, formatReason string) string {
	if formatReason != "" {
		return "CSRF cookie " + formatReason + "."
	}
	if secret == "" {
		return ReasonNoCSRFCookie
	}

	token, source := "", "POST"
	if r.Method == http.MethodPost {
		token = r.PostFormValue(FormField)
	}
	if token == "" {
		name := http.CanonicalHeaderKey(c.HeaderName)
		values, ok := r.Header[name]
		if !ok || len(values) == 0 {
			return ReasonCSRFTokenMissing
		}
		token, source = values[0], "the '"+name+"' HTTP header"
	}

	if reason := checkFormat(token); reason != "" {
		return "CSRF token from " + source + " " + reason + "."
	}
	if !tokenMatches(token, secret) {
		return "CSRF token from " + source + " incorrect."
	}
	return ""
}

// responseWriter sends the secret to the browser before the response headers
// are written.
type responseWriter struct {
	http.ResponseWriter
	// cfg, st and sess are the state of the request.
	cfg  *config
	st   *state
	sess *sessions.Session
	// finished guards against sending the secret twice.
	finished bool
}

// Unwrap returns the wrapped writer for http.ResponseController.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader sends the secret and writes the status.
func (w *responseWriter) WriteHeader(status int) {
	w.finish()
	w.ResponseWriter.WriteHeader(status)
}

// Write sends the secret before the first bytes of the body.
func (w *responseWriter) Write(b []byte) (int, error) {
	w.finish()
	return w.ResponseWriter.Write(b)
}

// Flush sends the secret and flushes the wrapped writer.
func (w *responseWriter) Flush() {
	w.finish()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// finish stores the secret when a token was handed out or the secret changed
// (Django's process_response). Only the first call has an effect.
func (w *responseWriter) finish() {
	if w.finished {
		return
	}
	w.finished = true
	if !w.st.needsUpdate {
		return
	}
	w.st.needsUpdate = false

	if w.cfg.UseSessions {
		if cur, _ := w.sess.Get(SessionKey); cur != w.st.secret {
			w.sess.Set(SessionKey, w.st.secret)
		}
		return
	}
	cookie := &http.Cookie{
		Name:     w.cfg.CookieName,
		Value:    w.st.secret,
		Domain:   w.cfg.CookieDomain,
		Path:     w.cfg.CookiePath,
		Secure:   w.cfg.CookieSecure,
		HttpOnly: w.cfg.CookieHTTPOnly,
		SameSite: w.cfg.CookieSameSite,
	}
	if w.cfg.CookieAge > 0 {
		cookie.MaxAge = int(w.cfg.CookieAge / time.Second)
		cookie.Expires = time.Now().Add(w.cfg.CookieAge)
	}
	http.SetCookie(w.ResponseWriter, cookie)
	patchVary(w.Header(), "Cookie")
}

// patchVary adds name to the Vary header unless it is listed already (Django's
// patch_vary_headers).
func patchVary(h http.Header, name string) {
	for _, line := range h.Values("Vary") {
		for part := range strings.SplitSeq(line, ",") {
			part = strings.TrimSpace(part)
			if part == "*" || strings.EqualFold(part, name) {
				return
			}
		}
	}
	h.Add("Vary", name)
}
