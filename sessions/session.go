// Package sessions provides per-request session storage with pluggable backends:
// an HMAC-signed cookie store that keeps all data client-side and a database-backed
// store that keeps data server-side keyed by an opaque session id. A middleware loads
// the session from the request cookie and persists it (Set-Cookie) when modified.
package sessions

import "context"

// Session is a per-request bag of values identified by a key.
type Session struct {
	key      string
	data     map[string]any
	modified bool
}

// Key returns the session's key (its server-side identifier or cookie id). It may be
// empty for a brand-new, never-persisted session.
func (s *Session) Key() string { return s.key }

// Get returns the value stored under k and whether it was present.
func (s *Session) Get(k string) (any, bool) {
	v, ok := s.data[k]
	return v, ok
}

// Set stores v under k and marks the session modified.
func (s *Session) Set(k string, v any) {
	if s.data == nil {
		s.data = make(map[string]any)
	}
	s.data[k] = v
	s.modified = true
}

// Delete removes k from the session and marks it modified.
func (s *Session) Delete(k string) {
	if _, ok := s.data[k]; !ok {
		return
	}
	delete(s.data, k)
	s.modified = true
}

// Pop returns the value stored under k, removing it from the session. The boolean
// reports whether k was present; when present the session is marked modified.
func (s *Session) Pop(k string) (any, bool) {
	v, ok := s.data[k]
	if !ok {
		return nil, false
	}
	delete(s.data, k)
	s.modified = true
	return v, true
}

// Modified reports whether the session has been changed since it was loaded.
func (s *Session) Modified() bool { return s.modified }

// Data returns a shallow copy of the session's values.
func (s *Session) Data() map[string]any {
	cp := make(map[string]any, len(s.data))
	for k, v := range s.data {
		cp[k] = v
	}
	return cp
}

// Clear removes all values from the session and marks it modified.
func (s *Session) Clear() {
	s.data = make(map[string]any)
	s.modified = true
}

// contextKey is an unexported type for the session context key to avoid collisions.
type contextKey struct{}

// NewContext returns a copy of ctx carrying the session s.
func NewContext(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// FromContext returns the session stored in ctx, if any.
func FromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(contextKey{}).(*Session)
	return s, ok
}
