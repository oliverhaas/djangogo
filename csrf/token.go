package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"strings"
)

// Sizes and alphabet of secrets and tokens, as in Django.
const (
	secretLength = 32
	tokenLength  = 2 * secretLength
	allowedChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// Reasons for a secret or token of the wrong form. They are phrases without a
// subject because they follow either "CSRF cookie" or "CSRF token from ...".
const (
	reasonIncorrectLength   = "has incorrect length"
	reasonInvalidCharacters = "has invalid characters"
)

// newSecret returns a random secret: 32 characters of allowedChars from
// crypto/rand.
func newSecret() string {
	limit := 256 - 256%len(allowedChars)
	out := make([]byte, 0, secretLength)
	buf := make([]byte, 2*secretLength)
	for len(out) < secretLength {
		// rand.Read cannot fail: it aborts the program instead of returning
		// a predictable value.
		_, _ = rand.Read(buf)
		for _, b := range buf {
			if int(b) < limit && len(out) < secretLength {
				out = append(out, allowedChars[int(b)%len(allowedChars)])
			}
		}
	}
	return string(out)
}

// maskSecret returns a token for secret: a random mask followed by the secret
// shifted by the mask, character by character (Django's _mask_cipher_secret).
// The secret must have passed checkFormat.
func maskSecret(secret string) string {
	mask := newSecret()
	n := len(allowedChars)
	cipher := make([]byte, len(secret))
	for i := range len(secret) {
		x := strings.IndexByte(allowedChars, secret[i])
		y := strings.IndexByte(allowedChars, mask[i])
		cipher[i] = allowedChars[(x+y)%n]
	}
	return mask + string(cipher)
}

// unmaskToken returns the secret of a masked token (Django's
// _unmask_cipher_token). The token must have passed checkFormat and have
// tokenLength characters.
func unmaskToken(token string) string {
	mask, cipher := token[:secretLength], token[secretLength:]
	n := len(allowedChars)
	secret := make([]byte, secretLength)
	for i := range secretLength {
		x := strings.IndexByte(allowedChars, cipher[i])
		y := strings.IndexByte(allowedChars, mask[i])
		secret[i] = allowedChars[(x-y+n)%n]
	}
	return string(secret)
}

// checkFormat returns the reason a secret or token has the wrong form, or ""
// (Django's _check_token_format). The value may be masked or not.
func checkFormat(token string) string {
	if len(token) != tokenLength && len(token) != secretLength {
		return reasonIncorrectLength
	}
	for i := range len(token) {
		if strings.IndexByte(allowedChars, token[i]) < 0 {
			return reasonInvalidCharacters
		}
	}
	return ""
}

// tokenMatches reports whether a token of the request matches the secret,
// after unmasking it if needed (Django's _does_token_match). The token must
// have passed checkFormat. The comparison takes constant time.
func tokenMatches(token, secret string) bool {
	if len(token) == tokenLength {
		token = unmaskToken(token)
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}
