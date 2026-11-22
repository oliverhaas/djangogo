package auth

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/oliverhaas/djangogo/validators"
)

// IntToBase36 returns i in base 36 with lower-case letters (Django's
// int_to_base36).
func IntToBase36(i uint64) string {
	return strconv.FormatUint(i, 36)
}

// Base36ToInt returns the number that s writes in base 36 (Django's
// base36_to_int). Like Python's int(s, 36) it accepts upper-case letters, a
// sign, underscores between digits and surrounding whitespace. Input longer
// than 13 characters is refused before it is parsed, as in Django, and so is
// a number outside the range of int64, which Python would return.
func Base36ToInt(s string) (int64, error) {
	if utf8.RuneCountInString(s) > 13 {
		return 0, errors.New("Base36 input too large") //nolint:revive,staticcheck // Django's message
	}
	invalid := errors.New("invalid literal for int() with base 36: " + validators.PyRepr(s))
	t := strings.TrimSpace(s)
	sign := ""
	if t != "" && (t[0] == '+' || t[0] == '-') {
		sign, t = t[:1], t[1:]
	}
	if t == "" || t[0] == '_' || t[len(t)-1] == '_' || strings.Contains(t, "__") {
		return 0, invalid
	}
	n, err := strconv.ParseInt(sign+strings.ReplaceAll(t, "_", ""), 36, 64)
	if errors.Is(err, strconv.ErrRange) {
		return 0, errors.New("Base36 input too large") //nolint:revive,staticcheck // Django's message
	}
	if err != nil {
		return 0, invalid
	}
	return n, nil
}

// URLSafeBase64Encode returns b in URL-safe base64 without the trailing
// equal signs (Django's urlsafe_base64_encode). The password reset links
// carry the user id in this form.
func URLSafeBase64Encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// URLSafeBase64Decode decodes s, written by URLSafeBase64Encode, with or
// without its trailing equal signs (Django's urlsafe_base64_decode). Unlike
// Python it refuses characters outside the URL-safe alphabet instead of
// skipping them.
func URLSafeBase64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}
