package sessions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// SignedCookieStore stores all session data in an HMAC-signed cookie, keeping no
// server-side state. The cookie value is "payload.mac" where payload is the
// URL-safe base64 of the JSON-encoded data and mac is the URL-safe base64 of
// HMAC-SHA256(secret, payload).
type SignedCookieStore struct {
	secret []byte
}

// NewSignedCookieStore returns a SignedCookieStore that signs cookies with secret.
func NewSignedCookieStore(secret []byte) *SignedCookieStore {
	return &SignedCookieStore{secret: secret}
}

// New returns a fresh, empty session.
func (s *SignedCookieStore) New() *Session {
	return &Session{data: make(map[string]any)}
}

// Encode serializes s into a signed cookie value of the form "payload.mac".
func (s *SignedCookieStore) Encode(_ context.Context, sess *Session) (string, error) {
	raw, err := json.Marshal(sess.Data())
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := s.sign(payload)
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

// Decode verifies and parses a signed cookie value. A tampered, malformed, or
// wrongly-signed value yields a fresh empty session and a nil error so the request
// proceeds as if logged out.
func (s *SignedCookieStore) Decode(_ context.Context, cookieValue string) (*Session, error) {
	data, ok := s.verify(cookieValue)
	if !ok {
		return s.New(), nil
	}
	return &Session{data: data}, nil
}

// verify validates the MAC over the payload and returns the decoded data. The boolean
// is false for any tampered, malformed, or wrongly-signed value.
func (s *SignedCookieStore) verify(cookieValue string) (map[string]any, bool) {
	idx := strings.LastIndex(cookieValue, ".")
	if idx < 0 {
		return nil, false
	}
	payload := cookieValue[:idx]
	gotMAC, err := base64.RawURLEncoding.DecodeString(cookieValue[idx+1:])
	if err != nil {
		return nil, false
	}
	if !hmac.Equal(gotMAC, s.sign(payload)) {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, false
	}
	data := make(map[string]any)
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, false
	}
	return data, true
}

// Delete is a no-op: the signed cookie store keeps no server-side state.
func (s *SignedCookieStore) Delete(_ context.Context, _ *Session) error { return nil }

// sign returns HMAC-SHA256(secret, payload).
func (s *SignedCookieStore) sign(payload string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
