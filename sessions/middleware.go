package sessions

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// DefaultCookieName is Django's SESSION_COOKIE_NAME.
const DefaultCookieName = "sessionid"

// interruptedMessage is the text of Django's SessionInterrupted error.
const interruptedMessage = "The request's session was deleted before the request completed. " +
	"The user may have logged out in a concurrent request, for example."

// Options are the session settings of the middleware. The zero value is
// Django's default for every setting.
type Options struct {
	// CookieName is the name of the session cookie (SESSION_COOKIE_NAME).
	// Empty means "sessionid".
	CookieName string
	// CookieAge is the lifetime of the session and of its cookie
	// (SESSION_COOKIE_AGE). Zero means two weeks.
	CookieAge time.Duration
	// CookieDomain is the Domain attribute (SESSION_COOKIE_DOMAIN). Empty
	// leaves it out, which binds the cookie to the host that set it.
	CookieDomain string
	// CookiePath is the Path attribute (SESSION_COOKIE_PATH). Empty means "/".
	CookiePath string
	// CookieSecure sets the Secure attribute (SESSION_COOKIE_SECURE). Turn it
	// on for every site served over HTTPS.
	CookieSecure bool
	// DisableHTTPOnly leaves out the HttpOnly attribute, which lets scripts
	// read the session cookie (SESSION_COOKIE_HTTPONLY = False).
	DisableHTTPOnly bool
	// CookieSameSite is the SameSite attribute (SESSION_COOKIE_SAMESITE). The
	// zero value means Lax. http.SameSiteDefaultMode leaves the attribute out,
	// which is Django's False.
	CookieSameSite http.SameSite
	// SaveEveryRequest saves the session and renews its cookie on every
	// request, not only when the session changed
	// (SESSION_SAVE_EVERY_REQUEST).
	SaveEveryRequest bool
	// ExpireAtBrowserClose issues the cookie without an expiry, so that the
	// browser drops it when it closes (SESSION_EXPIRE_AT_BROWSER_CLOSE).
	ExpireAtBrowserClose bool
}

// withDefaults returns o with Django's defaults in place of zero values.
func (o Options) withDefaults() Options {
	if o.CookieName == "" {
		o.CookieName = DefaultCookieName
	}
	if o.CookieAge <= 0 {
		o.CookieAge = DefaultCookieAge
	}
	if o.CookiePath == "" {
		o.CookiePath = "/"
	}
	if o.CookieSameSite == 0 {
		o.CookieSameSite = http.SameSiteLaxMode
	}
	return o
}

// Middleware is MiddlewareWithOptions with Django's defaults and the given
// cookie name.
func Middleware(store Store, cookieName string) func(http.Handler) http.Handler {
	return MiddlewareWithOptions(store, Options{CookieName: cookieName})
}

// MiddlewareWithOptions returns the session middleware (Django's
// SessionMiddleware). It puts the session named by the request cookie into the
// request context for FromContext, reads it from the store when the handler
// first uses it, and finishes it just before the response headers go out:
//
//   - An empty session whose cookie came with the request has that cookie
//     deleted.
//   - A session that was modified, or every session with SaveEveryRequest, is
//     saved and its cookie renewed. Responses with a 5xx status do not save.
//   - A response that depends on the session gets "Vary: Cookie".
//
// A session that a concurrent request deleted is not saved again: the
// response becomes a 400 with Django's SessionInterrupted text. When the store
// fails to read the session, the handler sees an empty session and the
// response becomes a 500.
func MiddlewareWithOptions(store Store, opts Options) func(http.Handler) http.Handler {
	opts = opts.withDefaults()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			sess := store.New()
			hadCookie := false
			if c, err := req.Cookie(opts.CookieName); err == nil {
				hadCookie = true
				sess.loader = func() (*Session, error) { return store.Decode(req.Context(), c.Value) }
			}
			sess.cookieAge = opts.CookieAge
			sess.browserClose = opts.ExpireAtBrowserClose

			ctx = NewContext(ctx, sess)
			sw := &sessionWriter{
				ResponseWriter: w,
				store:          store,
				opts:           opts,
				ctx:            ctx,
				sess:           sess,
				hadCookie:      hadCookie,
			}
			// A handler that panics after Flush or CycleKey must not leave the
			// retired session usable.
			defer sw.dropStale()

			next.ServeHTTP(sw, req.WithContext(ctx))
			sw.finish(http.StatusOK)
		})
	}
}

// sessionWriter finishes the session before the response headers are written.
type sessionWriter struct {
	http.ResponseWriter
	// store, opts, ctx and sess are the state of the request.
	store Store
	opts  Options
	ctx   context.Context
	sess  *Session
	// hadCookie reports that the request carried the session cookie.
	hadCookie bool
	// finished guards against finishing the session twice.
	finished bool
	// replaced reports that the middleware wrote an error response of its own
	// and discards what the handler writes.
	replaced bool
}

// Unwrap returns the wrapped writer for http.ResponseController.
func (w *sessionWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader finishes the session and writes the status.
func (w *sessionWriter) WriteHeader(status int) {
	w.finish(status)
	if !w.replaced {
		w.ResponseWriter.WriteHeader(status)
	}
}

// Write finishes the session before the first bytes of the body.
func (w *sessionWriter) Write(b []byte) (int, error) {
	w.finish(http.StatusOK)
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

// Flush finishes the session and flushes the wrapped writer.
func (w *sessionWriter) Flush() {
	w.finish(http.StatusOK)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// finish applies Django's SessionMiddleware.process_response for a response
// with the given status. Only the first call has an effect.
func (w *sessionWriter) finish(status int) {
	if w.finished {
		return
	}
	w.finished = true
	header := w.Header()
	sess := w.sess

	// A save needs the values, also of a session that the handler only
	// marked modified.
	if (sess.Modified() || w.opts.SaveEveryRequest) && status < http.StatusInternalServerError {
		_ = sess.load()
	}
	if sess.loadErr != nil {
		w.replaced = true
		http.Error(w.ResponseWriter, "session error", http.StatusInternalServerError)
		return
	}
	if w.hadCookie && sess.IsEmpty() {
		w.dropStale()
		w.deleteCookie()
		patchVary(header, "Cookie")
		return
	}
	if sess.Accessed() {
		patchVary(header, "Cookie")
	}
	if !sess.Modified() && !w.opts.SaveEveryRequest || sess.IsEmpty() {
		return
	}
	if status >= http.StatusInternalServerError {
		return
	}
	value, err := w.store.Encode(w.ctx, sess)
	if err != nil {
		w.replaced = true
		if errors.Is(err, ErrUpdate) {
			http.Error(w.ResponseWriter, interruptedMessage, http.StatusBadRequest)
			return
		}
		http.Error(w.ResponseWriter, "session error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w.ResponseWriter, w.cookie(value))
	patchVary(header, "Cookie")
}

// cookie builds the session cookie for a saved session.
func (w *sessionWriter) cookie(value string) *http.Cookie {
	c := &http.Cookie{
		Name:     w.opts.CookieName,
		Value:    value,
		Domain:   w.opts.CookieDomain,
		Path:     w.opts.CookiePath,
		Secure:   w.opts.CookieSecure,
		HttpOnly: !w.opts.DisableHTTPOnly,
		SameSite: w.opts.CookieSameSite,
	}
	if w.sess.GetExpireAtBrowserClose() {
		return c
	}
	now := time.Now()
	age := w.sess.expiryAge(now)
	c.Expires = now.Add(age)
	c.MaxAge = int(age / time.Second)
	if c.MaxAge <= 0 {
		// net/http writes "Max-Age=0" for a negative value: the browser drops
		// the cookie at once.
		c.MaxAge = -1
	}
	return c
}

// deleteCookie tells the browser to drop the session cookie. A browser only
// accepts that for a "__Secure-" or "__Host-" cookie, and for SameSite=None,
// when the Secure attribute is present.
func (w *sessionWriter) deleteCookie() {
	name := w.opts.CookieName
	secure := strings.HasPrefix(name, "__Secure-") || strings.HasPrefix(name, "__Host-") ||
		w.opts.CookieSameSite == http.SameSiteNoneMode
	http.SetCookie(w.ResponseWriter, &http.Cookie{
		Name:     name,
		Value:    "",
		Domain:   w.opts.CookieDomain,
		Path:     w.opts.CookiePath,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		Secure:   secure,
		SameSite: w.opts.CookieSameSite,
	})
}

// dropStale removes from the store the keys the session gave up in Flush and
// CycleKey. It runs even when the client has gone away, because a logout must
// not depend on the client waiting for the response. An error is not reported:
// the response is already decided and the stored sessions expire on their
// own.
func (w *sessionWriter) dropStale() {
	if len(w.sess.stale) == 0 {
		return
	}
	_ = w.store.Delete(context.WithoutCancel(w.ctx), &Session{stale: w.sess.stale})
	w.sess.stale = nil
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
