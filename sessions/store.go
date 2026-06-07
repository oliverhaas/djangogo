package sessions

import (
	"context"
	"crypto/rand"
	"encoding/base64"
)

// Store serializes sessions to/from an opaque cookie value.
type Store interface {
	// New returns a fresh, empty session.
	New() *Session
	// Decode turns a cookie value into a Session. An invalid value yields a fresh
	// empty session with a nil error, so a tampered or stale cookie simply logs the
	// user out rather than failing the request.
	Decode(ctx context.Context, cookieValue string) (*Session, error)
	// Encode persists/serializes s and returns the cookie value to store.
	Encode(ctx context.Context, s *Session) (string, error)
	// Delete removes server-side state for s (a no-op for the cookie store).
	Delete(ctx context.Context, s *Session) error
}

// Rotate gives s a brand-new key as a session-fixation defense; call it on login.
// The session data is preserved and the session is marked modified.
func Rotate(s *Session) {
	s.key = newKey()
	s.modified = true
}

// newKey returns a cryptographically random session key as 32 random bytes encoded
// with URL-safe base64 (no padding).
func newKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never returns an error on supported platforms; panicking
		// here is preferable to silently issuing a predictable session key.
		panic("sessions: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
