package sessions

import (
	"context"
	"net/http"
)

// Middleware loads the session from the request cookie into the request context and
// saves it (via a Set-Cookie header) just before the response headers are written,
// whenever the session was modified. The cookie is named cookieName and is issued
// HttpOnly, Path "/", SameSite Lax.
//
// Because headers cannot be added after WriteHeader, the save happens on the first
// Write/WriteHeader. If the handler never writes, the save runs in a deferred step.
func Middleware(store Store, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			var sess *Session
			if c, err := req.Cookie(cookieName); err == nil {
				s, derr := store.Decode(ctx, c.Value)
				if derr != nil {
					http.Error(w, "session error", http.StatusInternalServerError)
					return
				}
				sess = s
			} else {
				sess = store.New()
			}

			ctx = NewContext(ctx, sess)
			req = req.WithContext(ctx)

			sw := &sessionWriter{
				ResponseWriter: w,
				store:          store,
				cookieName:     cookieName,
				ctx:            ctx,
				sess:           sess,
			}

			next.ServeHTTP(sw, req)

			// Handlers that never call Write/WriteHeader (e.g. a bare 200 with no body)
			// still need their session persisted; net/http flushes the headers itself.
			sw.save()
		})
	}
}

// sessionWriter wraps an http.ResponseWriter so that the session is persisted and the
// Set-Cookie header is set before the response headers are committed.
type sessionWriter struct {
	http.ResponseWriter
	store       Store
	cookieName  string
	ctx         context.Context
	sess        *Session
	wroteHeader bool
}

// save persists the session (once) and writes the Set-Cookie header. It must run
// before WriteHeader commits the response. On encode failure it skips the cookie so
// the response still proceeds rather than failing the request.
func (w *sessionWriter) save() {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if w.sess == nil || !w.sess.Modified() {
		return
	}
	value, err := w.store.Encode(w.ctx, w.sess)
	if err != nil {
		return
	}
	// Secure is intentionally not set here so the cookie works over plain HTTP in
	// development. A real deployment behind TLS should set Secure: true.
	http.SetCookie(w.ResponseWriter, &http.Cookie{
		Name:     w.cookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// WriteHeader saves the session before committing the status code, guarding against a
// double WriteHeader.
func (w *sessionWriter) WriteHeader(status int) {
	w.save()
	w.ResponseWriter.WriteHeader(status)
}

// Write saves the session before the first body write (which implicitly commits a 200
// status) and delegates to the wrapped writer.
func (w *sessionWriter) Write(b []byte) (int, error) {
	w.save()
	return w.ResponseWriter.Write(b)
}
