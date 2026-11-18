package messages

import (
	"context"
	"net/http"
	"sync"

	"github.com/oliverhaas/djangogo/sessions"
)

// Middleware attaches the message storage to every request and writes the
// unread messages to the session before the response headers go out
// (Django's MessageMiddleware). A message added after that point is lost.
//
// It must run inside the session middleware, which loads the session first;
// for a request without a session it panics with Django's
// ImproperlyConfigured text.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, _ := sessions.FromContext(r.Context())
		s := NewSessionStorage(sess)
		if rec, ok := RecorderFromContext(r.Context()); ok {
			rec.set(s)
		}
		mw := &responseWriter{ResponseWriter: w, storage: s}
		next.ServeHTTP(mw, r.WithContext(NewContext(r.Context(), s)))
		mw.finish()
	})
}

// responseWriter updates the storage before the response headers go out,
// while the session middleware outside can still save the session.
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
	w.storage.Update()
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
