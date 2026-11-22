package auth

import (
	"strconv"
	"strings"
	"time"
)

// DefaultPasswordResetTimeout is how long a password reset link stays valid
// when Config.PasswordResetTimeout is zero: three days, Django's default
// PASSWORD_RESET_TIMEOUT.
const DefaultPasswordResetTimeout = 3 * 24 * time.Hour

// tokenKeySalt is the key_salt of Django's PasswordResetTokenGenerator. Tokens
// made by Django with the same secret key check out here, and the other way
// round.
const tokenKeySalt = "django.contrib.auth.tokens.PasswordResetTokenGenerator"

// tokenEpoch is the Unix time of 2001-01-01 00:00:00, from which Django counts
// the seconds in a token.
const tokenEpoch = 978307200

// PasswordResetTokenGenerator makes and checks the tokens of password reset
// links (Django's PasswordResetTokenGenerator). A token holds its time of
// creation and a hash of the user's id, password hash, last login, email
// address and that time, signed with the secret key. It stops working when
// one of these changes, so a reset, which sets a new password, uses it up.
// It also stops working after Config.PasswordResetTimeout.
//
// The zero value uses the secret key of Config. DefaultTokenGenerator is the
// generator the views use when they name none.
type PasswordResetTokenGenerator struct {
	// Secret signs the tokens. Empty means Config.SecretKey.
	Secret string
	// SecretFallbacks are former secrets: a token signed with one of them
	// still checks out. Nil means Config.SecretKeyFallbacks; an empty,
	// non-nil list accepts none.
	SecretFallbacks []string
	// Now returns the current time. Nil means time.Now. Tests set it to
	// move the clock.
	Now func() time.Time
}

// DefaultTokenGenerator is the token generator of the password reset views
// (Django's default_token_generator).
var DefaultTokenGenerator = &PasswordResetTokenGenerator{}

// MakeToken returns a token that resets the password of u once.
func (g *PasswordResetTokenGenerator) MakeToken(u *User) string {
	return g.tokenWithTimestamp(u, tokenSeconds(g.now()), g.secret())
}

// CheckToken reports whether token is a valid reset token for u: made by
// MakeToken with the secret or one of the fallbacks, for the current state of
// u, and not older than the timeout. A nil user or an empty token is never
// valid.
func (g *PasswordResetTokenGenerator) CheckToken(u *User, token string) bool {
	if u == nil || token == "" {
		return false
	}
	parts := strings.Split(token, "-")
	if len(parts) != 2 {
		return false
	}
	ts, err := Base36ToInt(parts[0])
	if err != nil {
		return false
	}
	valid := false
	for _, secret := range append([][]byte{g.secret()}, g.fallbacks()...) {
		if sameString(g.tokenWithTimestamp(u, ts, secret), token) {
			valid = true
			break
		}
	}
	if !valid {
		return false
	}
	return tokenSeconds(g.now())-ts <= int64(passwordResetTimeout()/time.Second)
}

// tokenWithTimestamp returns the token of u made at ts with secret: ts in
// base 36, a dash and every second character of the hex digest.
func (g *PasswordResetTokenGenerator) tokenWithTimestamp(u *User, ts int64, secret []byte) string {
	digest := saltedHMAC(tokenKeySalt, tokenHashValue(u, ts), secret)
	short := make([]byte, 0, len(digest)/2)
	for i := 0; i < len(digest); i += 2 {
		short = append(short, digest[i])
	}
	return IntToBase36(uint64(ts)) + "-" + string(short)
}

// tokenHashValue is the text the token signs, as in Django: the id, the
// password hash, the last login to the second in UTC, the timestamp and the
// email address.
func tokenHashValue(u *User, ts int64) string {
	login := ""
	if u.LastLogin != nil {
		login = u.LastLogin.UTC().Format("2006-01-02 15:04:05")
	}
	return strconv.FormatInt(u.ID, 10) + u.Password + login + strconv.FormatInt(ts, 10) + u.Email
}

// now returns the current time from Now or the clock.
func (g *PasswordResetTokenGenerator) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// secret returns Secret or the secret key of Config.
func (g *PasswordResetTokenGenerator) secret() []byte {
	if g.Secret != "" {
		return []byte(g.Secret)
	}
	return secretKey()
}

// fallbacks returns SecretFallbacks or the fallbacks of Config.
func (g *PasswordResetTokenGenerator) fallbacks() [][]byte {
	list := g.SecretFallbacks
	if list == nil {
		list = settings().SecretKeyFallbacks
	}
	out := make([][]byte, len(list))
	for i, s := range list {
		out[i] = []byte(s)
	}
	return out
}

// tokenSeconds returns the seconds from 2001-01-01 to t, both read on the
// wall clock of Config.Location, as Django counts them from the naive local
// time.
func tokenSeconds(t time.Time) int64 {
	t = t.In(tokenLocation())
	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	return time.Date(y, mo, d, h, mi, s, 0, time.UTC).Unix() - tokenEpoch
}

// tokenLocation returns Config.Location or the local time zone.
func tokenLocation() *time.Location {
	if loc := settings().Location; loc != nil {
		return loc
	}
	return time.Local
}

// passwordResetTimeout returns Config.PasswordResetTimeout or its default.
func passwordResetTimeout() time.Duration {
	if d := settings().PasswordResetTimeout; d > 0 {
		return d
	}
	return DefaultPasswordResetTimeout
}
