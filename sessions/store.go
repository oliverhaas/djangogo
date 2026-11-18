package sessions

import (
	"context"
	"crypto/rand"
	"errors"
)

// ErrUpdate reports that a session to be updated is no longer in the store
// (Django's UpdateError). It happens when a concurrent request deleted the
// session, for example by logging out. The session is not created again,
// which would undo the logout.
var ErrUpdate = errors.New("sessions: session was deleted before it could be saved")

// Store loads and saves sessions. The value it exchanges with the middleware
// is the content of the session cookie.
type Store interface {
	// New returns an empty session.
	New() *Session
	// Decode returns the session for a cookie value. An unknown, expired or
	// forged value yields an empty session and a nil error: the request goes
	// on without a session instead of failing.
	Decode(ctx context.Context, cookieValue string) (*Session, error)
	// Encode saves s and returns the cookie value. It returns ErrUpdate when
	// the session was deleted in the meantime.
	Encode(ctx context.Context, s *Session) (string, error)
	// Delete removes what the store holds for s, including the keys the
	// session had before CycleKey and Flush.
	Delete(ctx context.Context, s *Session) error
}

// ExpiredClearer is implemented by a store that can remove expired sessions
// (Django's clear_expired). The clearsessions command needs it.
type ExpiredClearer interface {
	// ClearExpired removes the sessions whose expiry date has passed.
	ClearExpired(ctx context.Context) error
}

// Rotate gives s a new key and keeps its values. It is CycleKey under the
// name earlier releases used.
func Rotate(s *Session) {
	s.CycleKey()
}

// Alphabet and length of a session key, as in Django.
const (
	keyAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	keyLength   = 32
)

// newKey returns a random session key: 32 characters of [a-z0-9], about 165
// bits from crypto/rand.
func newKey() string {
	return randomString(keyLength, keyAlphabet)
}

// randomString returns n characters drawn uniformly from alphabet with
// crypto/rand. Bytes that would favor the first characters of the alphabet are
// thrown away.
func randomString(n int, alphabet string) string {
	limit := 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n+n/4)
	for len(out) < n {
		// rand.Read cannot fail: it aborts the program instead of returning
		// a predictable value.
		_, _ = rand.Read(buf)
		for _, b := range buf {
			if int(b) < limit && len(out) < n {
				out = append(out, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	return string(out)
}
