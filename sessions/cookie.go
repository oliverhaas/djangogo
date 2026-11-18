package sessions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// cookieSalt separates the signatures of session cookies from other uses of
// the same secret.
const cookieSalt = "djangogo.sessions.SignedCookieStore"

// SignedCookieStore keeps the whole session in a signed cookie and nothing on
// the server (Django's signed_cookies backend).
//
// The cookie value is "payload.timestamp.mac": the URL-safe base64 of the
// values as JSON, the time of signing in seconds since the epoch, and an
// HMAC-SHA256 over both. The values are signed, not encrypted: the browser can
// read them. A cookie that was copied stays valid until it expires, even after
// logout, because there is no server state to delete.
type SignedCookieStore struct {
	// MaxAge is the time for which a cookie value is accepted after it was
	// signed. Zero means DefaultCookieAge. Set it to the cookie age of the
	// middleware.
	MaxAge time.Duration
	// Fallbacks are earlier secrets. Values signed with one of them are still
	// read; new values are signed with the current secret only.
	Fallbacks [][]byte

	// secret is the current signing secret.
	secret []byte
}

// NewSignedCookieStore returns a SignedCookieStore that signs with secret.
func NewSignedCookieStore(secret []byte) *SignedCookieStore {
	return &SignedCookieStore{secret: secret}
}

// New returns an empty session.
func (s *SignedCookieStore) New() *Session {
	return &Session{data: make(map[string]any)}
}

// Encode returns the signed cookie value for the values of sess.
func (s *SignedCookieStore) Encode(_ context.Context, sess *Session) (string, error) {
	raw, err := json.Marshal(sess.data)
	if err != nil {
		return "", err
	}
	sess.stale = nil
	signed := base64.RawURLEncoding.EncodeToString(raw) + "." + strconv.FormatInt(time.Now().Unix(), 10)
	mac := sign(s.secret, signed)
	return signed + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

// Decode checks the signature and the age of a cookie value and returns its
// session. A forged, malformed or expired value yields an empty session.
func (s *SignedCookieStore) Decode(_ context.Context, cookieValue string) (*Session, error) {
	data, signedAt, ok := s.verify(cookieValue)
	if !ok {
		return s.New(), nil
	}
	sess := &Session{data: data}
	seconds, _, kind := sess.expiry()
	custom := kind == kindDate || (kind == kindSeconds && seconds != 0)
	if custom && !sess.expiryDate(signedAt).After(time.Now()) {
		return s.New(), nil
	}
	return sess, nil
}

// Delete does nothing: there is no server state.
func (s *SignedCookieStore) Delete(_ context.Context, sess *Session) error {
	sess.stale = nil
	return nil
}

// ClearExpired does nothing: an expired cookie is rejected when it is read.
func (s *SignedCookieStore) ClearExpired(_ context.Context) error { return nil }

// verify returns the values and the signing time of a cookie value. The
// boolean is false for a value that is malformed, carries a wrong signature or
// is older than the maximum age.
func (s *SignedCookieStore) verify(cookieValue string) (data map[string]any, signedAt time.Time, ok bool) {
	idx := strings.LastIndex(cookieValue, ".")
	if idx < 0 {
		return nil, time.Time{}, false
	}
	signed := cookieValue[:idx]
	gotMAC, err := base64.RawURLEncoding.DecodeString(cookieValue[idx+1:])
	if err != nil || !s.validMAC(signed, gotMAC) {
		return nil, time.Time{}, false
	}
	payload, stamp, found := strings.Cut(signed, ".")
	if !found {
		return nil, time.Time{}, false
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return nil, time.Time{}, false
	}
	signedAt = time.Unix(seconds, 0)
	maxAge := s.MaxAge
	if maxAge <= 0 {
		maxAge = DefaultCookieAge
	}
	if time.Since(signedAt) > maxAge {
		return nil, time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, time.Time{}, false
	}
	data = make(map[string]any)
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, time.Time{}, false
	}
	return data, signedAt, true
}

// validMAC reports whether mac signs the value under the current secret or one
// of the fallbacks. Every comparison takes constant time.
func (s *SignedCookieStore) validMAC(signed string, mac []byte) bool {
	valid := hmac.Equal(mac, sign(s.secret, signed))
	for _, secret := range s.Fallbacks {
		if hmac.Equal(mac, sign(secret, signed)) {
			valid = true
		}
	}
	return valid
}

// sign returns the HMAC-SHA256 of value under a key derived from secret and
// cookieSalt.
func sign(secret []byte, value string) []byte {
	derive := hmac.New(sha256.New, secret)
	derive.Write([]byte(cookieSalt))
	mac := hmac.New(sha256.New, derive.Sum(nil))
	mac.Write([]byte(value))
	return mac.Sum(nil)
}
