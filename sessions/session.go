// Package sessions is the port of django.contrib.sessions: a per-request
// session, a database store on Django's django_session table, a file store, a
// signed-cookie store, and the middleware that loads the session from its
// cookie and saves it when it changed.
package sessions

import (
	"context"
	"maps"
	"time"
)

// Session is the data of one browser session (Django's SessionBase). The zero
// value is an empty session without a key.
//
// A Session belongs to one request and is not safe for concurrent use. Values
// travel through JSON: a number read back from the store is a float64.
type Session struct {
	// key identifies the session in the store. It is empty until the first
	// save and for a session of the signed-cookie store.
	key string
	// data holds the values.
	data map[string]any
	// modified and accessed are Django's flags of the same name.
	modified bool
	accessed bool
	// persisted reports that the store holds the session under key.
	persisted bool
	// stale holds the keys the session had before CycleKey and Flush. The
	// store removes what it holds under them at the next save or delete.
	stale []string
	// cookieAge and browserClose are the expiry policy of the middleware that
	// loaded the session. Zero values mean Django's defaults.
	cookieAge    time.Duration
	browserClose bool
}

// Key returns the key that identifies the session in the store (Django's
// session_key). It is empty for a session that was never saved, after Flush,
// and for every session of the signed-cookie store.
func (s *Session) Key() string { return s.key }

// Get returns the value stored under k and whether it was present.
func (s *Session) Get(k string) (any, bool) {
	s.accessed = true
	v, ok := s.data[k]
	return v, ok
}

// Has reports whether a value is stored under k.
func (s *Session) Has(k string) bool {
	s.accessed = true
	_, ok := s.data[k]
	return ok
}

// Set stores v under k and marks the session modified.
func (s *Session) Set(k string, v any) {
	s.values()[k] = v
	s.accessed = true
	s.modified = true
}

// SetDefault returns the value stored under k. When there is none it stores v
// first, which marks the session modified (Django's setdefault).
func (s *Session) SetDefault(k string, v any) any {
	s.accessed = true
	if cur, ok := s.data[k]; ok {
		return cur
	}
	s.Set(k, v)
	return v
}

// Update stores every value of values and marks the session modified.
func (s *Session) Update(values map[string]any) {
	maps.Copy(s.values(), values)
	s.accessed = true
	s.modified = true
}

// Delete removes k from the session. It marks the session modified when k was
// present.
func (s *Session) Delete(k string) {
	s.Pop(k)
}

// Pop returns the value stored under k and removes it. The boolean reports
// whether k was present; the session is marked modified in that case only.
func (s *Session) Pop(k string) (any, bool) {
	s.accessed = true
	v, ok := s.data[k]
	if !ok {
		return nil, false
	}
	delete(s.data, k)
	s.modified = true
	return v, true
}

// Data returns a shallow copy of the values of the session.
func (s *Session) Data() map[string]any {
	s.accessed = true
	cp := make(map[string]any, len(s.data))
	maps.Copy(cp, s.data)
	return cp
}

// Clear removes all values and marks the session modified. The key stays; use
// Flush to end the session.
func (s *Session) Clear() {
	s.data = make(map[string]any)
	s.accessed = true
	s.modified = true
}

// Modified reports whether the session was changed since it was loaded. The
// middleware saves a modified session.
func (s *Session) Modified() bool { return s.modified }

// SetModified sets the modified flag. Set it after changing a value in place,
// for example a map stored in the session, which the session cannot notice.
func (s *Session) SetModified(modified bool) { s.modified = modified }

// Accessed reports whether the session was read or written during the
// request. The middleware adds "Vary: Cookie" to the response in that case.
func (s *Session) Accessed() bool { return s.accessed }

// IsEmpty reports whether the session has neither a key nor values (Django's
// is_empty). The middleware does not save an empty session and deletes its
// cookie.
func (s *Session) IsEmpty() bool {
	return s.key == "" && len(s.data) == 0
}

// CycleKey gives the session a new key and keeps its values (Django's
// cycle_key). Call it when the privileges of the browser change, as login
// does, so that a key known to an attacker beforehand is worthless. The store
// removes what it holds under the old key when the session is saved.
func (s *Session) CycleKey() {
	s.retire()
	s.key = newKey()
	s.accessed = true
	s.modified = true
}

// Flush removes all values and the key (Django's flush). The store removes
// what it holds under the old key, and the cookie is deleted, at the end of
// the request. Logout
// calls it so that the next user of the browser cannot read the old data.
func (s *Session) Flush() {
	s.retire()
	s.key = ""
	s.data = make(map[string]any)
	s.accessed = true
	s.modified = true
}

// retire queues the current key for removal from the store.
func (s *Session) retire() {
	if s.key != "" && s.persisted {
		s.stale = append(s.stale, s.key)
	}
	s.persisted = false
}

// values returns the data map, allocating it for a zero Session.
func (s *Session) values() map[string]any {
	if s.data == nil {
		s.data = make(map[string]any)
	}
	return s.data
}

// Names of the test cookie that tells whether the browser accepts cookies.
const (
	testCookieName  = "testcookie"
	testCookieValue = "worked"
)

// SetTestCookie stores a marker value. A later request that finds it with
// TestCookieWorked knows that the browser sends the session cookie back.
func (s *Session) SetTestCookie() { s.Set(testCookieName, testCookieValue) }

// TestCookieWorked reports whether the marker of SetTestCookie came back.
func (s *Session) TestCookieWorked() bool {
	v, _ := s.Get(testCookieName)
	return v == testCookieValue
}

// DeleteTestCookie removes the marker of SetTestCookie.
func (s *Session) DeleteTestCookie() { s.Delete(testCookieName) }

// contextKey is the type of the context key of the session.
type contextKey struct{}

// NewContext returns a copy of ctx carrying the session s.
func NewContext(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// FromContext returns the session stored in ctx by the middleware, if any.
func FromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(contextKey{}).(*Session)
	return s, ok
}
