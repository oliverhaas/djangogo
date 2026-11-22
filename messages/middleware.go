package messages

import (
	"context"
	"net/http"
	"sync"

	"github.com/oliverhaas/djangogo/sessions"
)

// StorageKind selects where the middleware keeps the messages between
// requests (Django's MESSAGE_STORAGE).
type StorageKind int

const (
	// StorageSession keeps the messages in the session (Django's
	// SessionStorage).
	StorageSession StorageKind = iota
	// StorageCookie keeps the messages in a signed cookie and drops the
	// oldest ones that do not fit into MaxCookieSize (Django's
	// CookieStorage).
	StorageCookie
	// StorageFallback keeps the messages in a signed cookie and the ones
	// that do not fit there in the session (Django's FallbackStorage, its
	// default).
	StorageFallback
)

// Options configures MiddlewareWithOptions.
type Options struct {
	// Storage selects the storage. The zero value is StorageSession.
	Storage StorageKind
	// Cookie configures the cookie of StorageCookie and StorageFallback,
	// which need its Secret.
	Cookie CookieOptions
}

// Middleware attaches a session storage to every request and stores the
// unread messages before the response headers go out (Django's
// MessageMiddleware). A message added after that point is lost. It is
// MiddlewareWithOptions with the zero Options.
//
// It must run inside the session middleware, which loads the session first;
// for a request without a session it panics with Django's
// ImproperlyConfigured text.
func Middleware(next http.Handler) http.Handler {
	return MiddlewareWithOptions(Options{})(next)
}

// MiddlewareWithOptions returns the middleware of Middleware with the storage
// of opts. The session and fallback storages need the session middleware
// outside. It panics when the storage needs a cookie and opts.Cookie.Secret
// is empty.
func MiddlewareWithOptions(opts Options) func(http.Handler) http.Handler {
	if opts.Storage != StorageSession && len(opts.Cookie.Secret) == 0 {
		panic(configError(noSecretMessage))
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := opts.storage(r)
			if rec, ok := RecorderFromContext(r.Context()); ok {
				rec.set(s)
			}
			mw := &responseWriter{ResponseWriter: w, storage: s}
			next.ServeHTTP(mw, r.WithContext(NewContext(r.Context(), s)))
			mw.finish()
		})
	}
}

// storage returns the storage of opts for r.
func (opts Options) storage(r *http.Request) *Storage {
	sess, _ := sessions.FromContext(r.Context())
	switch opts.Storage {
	case StorageCookie:
		return NewCookieStorage(r, opts.Cookie)
	case StorageFallback:
		return NewFallbackStorage(r, sess, opts.Cookie)
	}
	return NewSessionStorage(sess)
}

// responseWriter updates the storage before the response headers go out,
// while the response can still take a cookie and the session middleware
// outside can still save the session.
type responseWriter struct {
	http.ResponseWriter
	storage *Storage
	// finished guards against updating the storage twice.
	finished bool
}

// Unwrap returns the wrapped writer for http.ResponseController.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader updates the storage and writes the status.
func (w *responseWriter) WriteHeader(status int) {
	w.finish()
	w.ResponseWriter.WriteHeader(status)
}

// Write updates the storage before the first bytes of the body.
func (w *responseWriter) Write(b []byte) (int, error) {
	w.finish()
	return w.ResponseWriter.Write(b)
}

// Flush updates the storage and flushes the wrapped writer.
func (w *responseWriter) Flush() {
	w.finish()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// finish updates the storage on the first call (Django's process_response).
func (w *responseWriter) finish() {
	if w.finished {
		return
	}
	w.finished = true
	w.storage.Update(w.Header())
}

// Recorder captures the message storage of a request, for tests that check
// the messages of a response as djangotest.AssertMessages does. Put it into
// the request context with WithRecorder; Middleware hands it the storage of
// the request.
type Recorder struct {
	mu      sync.Mutex
	storage *Storage
}

// set records the storage of the request.
func (rec *Recorder) set(s *Storage) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.storage = s
}

// Storage returns the storage of the recorded request, or nil when the
// request did not pass through Middleware.
func (rec *Recorder) Storage() *Storage {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.storage
}

// recorderKey is the request context key of the recorder.
type recorderKey struct{}

// WithRecorder returns a copy of ctx that carries rec.
func WithRecorder(ctx context.Context, rec *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, rec)
}

// RecorderFromContext returns the recorder in ctx and whether there is one.
func RecorderFromContext(ctx context.Context) (*Recorder, bool) {
	rec, ok := ctx.Value(recorderKey{}).(*Recorder)
	return rec, ok
}
