package messages

import (
	"bytes"
	"compress/zlib"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/internal/pyjson"
)

// CookieName is the name of the cookie of the cookie storage (Django's
// CookieStorage.cookie_name).
const CookieName = "messages"

// MaxCookieSize is the longest cookie value that the cookie storage writes
// (Django's CookieStorage.max_cookie_size): half of the 4 kB that servers such
// as uwsgi allow for all headers.
const MaxCookieSize = 2048

// notFinished ends the stored list when some messages did not fit into the
// cookie (Django's CookieStorage.not_finished).
const notFinished = "__messagesnotfinished__"

// notFinishedJSON is notFinished as a JSON string.
const notFinishedJSON = `"` + notFinished + `"`

// cookieSalt is the salt of the signature (Django's CookieStorage.key_salt).
const cookieSalt = "django.contrib.messages"

// noSecretMessage is Django's text for an empty SECRET_KEY.
const noSecretMessage = "The SECRET_KEY setting must not be empty."

// CookieOptions configures the cookie of the cookie and fallback storages.
// Django takes the values from SECRET_KEY and the SESSION_COOKIE_* settings.
type CookieOptions struct {
	// Secret signs the cookie (SECRET_KEY). It must not be empty.
	Secret []byte
	// Fallbacks are earlier secrets whose signatures are still accepted
	// (SECRET_KEY_FALLBACKS).
	Fallbacks [][]byte
	// Domain is the Domain attribute (SESSION_COOKIE_DOMAIN). Empty leaves it
	// out.
	Domain string
	// Secure sets the Secure attribute (SESSION_COOKIE_SECURE).
	Secure bool
	// DisableHTTPOnly leaves out the HttpOnly attribute
	// (SESSION_COOKIE_HTTPONLY = False).
	DisableHTTPOnly bool
	// SameSite is the SameSite attribute (SESSION_COOKIE_SAMESITE). The zero
	// value means Lax. http.SameSiteDefaultMode leaves the attribute out,
	// which is Django's False.
	SameSite http.SameSite
}

// cookieBackend keeps the messages in a signed cookie (Django's
// CookieStorage).
type cookieBackend struct {
	r    *http.Request
	opts CookieOptions
	// now returns the time of the signature.
	now func() time.Time
}

// newCookieBackend returns the cookie backend of r. It panics on an empty
// secret, as Django's CookieStorage raises ImproperlyConfigured.
func newCookieBackend(r *http.Request, opts CookieOptions) *cookieBackend {
	if len(opts.Secret) == 0 {
		panic(configError(noSecretMessage))
	}
	if opts.SameSite == 0 {
		opts.SameSite = http.SameSiteLaxMode
	}
	return &cookieBackend{r: r, opts: opts, now: time.Now}
}

// load reads the messages of the request cookie. A list that ends with the
// notFinished marker holds only part of the messages.
func (b *cookieBackend) load() (ms []Message, all, bad bool) {
	c, err := b.r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil, true, false
	}
	ms, all, ok := decodeCookie(b.opts, c.Value)
	if !ok {
		return nil, true, true
	}
	return ms, all, false
}

// store writes ms into the cookie and returns the messages that do not fit
// into MaxCookieSize. removeOldest leaves out the oldest messages; otherwise
// the newest ones are left out. The list of a cookie that does not hold every
// message ends with the notFinished marker. The cookie is deleted when ms is
// empty.
func (b *cookieBackend) store(ms []Message, h http.Header, removeOldest bool) []Message {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = encodeMessage(m)
	}
	value := b.encodeParts(parts, false)
	var unstored []Message
	if len(value) > MaxCookieSize {
		withMarker := func(parts []string) string {
			return b.encodeParts(append(slices.Clip(parts), notFinishedJSON), true)
		}
		tooLarge := func(parts []string) bool { return len(withMarker(parts)) > MaxCookieSize }
		if removeOldest {
			i := bisectKeepRight(parts, tooLarge)
			unstored, value = ms[:i], withMarker(parts[i:])
		} else {
			i := bisectKeepLeft(parts, tooLarge)
			unstored, value = ms[i:], withMarker(parts[:i])
		}
	}
	b.setCookie(h, value)
	return unstored
}

// setCookie adds the Set-Cookie header of value to h, or the one that deletes
// the cookie when value is empty (Django's _update_cookie).
func (b *cookieBackend) setCookie(h http.Header, value string) {
	c := &http.Cookie{Name: CookieName, Value: value, Path: "/", Domain: b.opts.Domain, SameSite: b.opts.SameSite}
	if value != "" {
		c.Secure = b.opts.Secure
		c.HttpOnly = !b.opts.DisableHTTPOnly
	} else {
		// A browser drops a SameSite=None cookie without Secure.
		c.Secure = c.SameSite == http.SameSiteNoneMode
		c.Expires = time.Unix(0, 0).UTC()
		c.MaxAge = -1
	}
	if v := c.String(); v != "" {
		h.Add("Set-Cookie", v)
	}
}

// encodeParts returns the signed cookie value of the encoded messages parts,
// or "" for no parts unless encodeEmpty is set (Django's _encode_parts).
func (b *cookieBackend) encodeParts(parts []string, encodeEmpty bool) string {
	if len(parts) == 0 && !encodeEmpty {
		return ""
	}
	return signValue(b.opts.Secret, "["+strings.Join(parts, ",")+"]", b.now())
}

// encodeMessage returns m as Django's MessageEncoder writes it into the
// cookie: ["__json_message", is_safe, level, message, extra_tags] without
// spaces and in ASCII.
func encodeMessage(m Message) string {
	var b strings.Builder
	b.WriteString(`["` + messageMarker + `",`)
	if m.Safe {
		b.WriteString("1,")
	} else {
		b.WriteString("0,")
	}
	b.WriteString(strconv.Itoa(m.Level))
	b.WriteByte(',')
	pyjson.WriteString(&b, m.Message)
	b.WriteByte(',')
	pyjson.WriteString(&b, m.ExtraTags)
	b.WriteByte(']')
	return b.String()
}

// signValue returns data as the sign_object of Django's default
// TimestampSigner writes it with compression at t: the URL-safe base64 of
// data, or of its zlib compression with a leading "." when that is shorter,
// then ":" and the base62 Unix time, then ":" and the signature of both.
func signValue(secret []byte, data string, t time.Time) string {
	payload := []byte(data)
	prefix := ""
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	// Writes to a bytes.Buffer do not fail.
	_, _ = zw.Write(payload)
	_ = zw.Close()
	if buf.Len() < len(payload)-1 {
		payload, prefix = buf.Bytes(), "."
	}
	value := prefix + base64.RawURLEncoding.EncodeToString(payload) + ":" + base62(t.Unix())
	return value + ":" + signature(secret, value)
}

// decodeCookie returns the messages of the cookie value and whether the list
// holds them all, that is, does not end with the notFinished marker. ok is
// false for a value with a wrong signature or that cannot be read.
func decodeCookie(opts CookieOptions, value string) (ms []Message, all, ok bool) {
	i := strings.LastIndexByte(value, ':')
	if i < 0 {
		return nil, false, false
	}
	signed, sig := value[:i], value[i+1:]
	if !validSignature(opts, signed, sig) {
		return nil, false, false
	}
	// The signed part ends with the time of the signature, which the cookie
	// storage does not check.
	i = strings.LastIndexByte(signed, ':')
	if i < 0 {
		return nil, false, false
	}
	payload := signed[:i]
	compressed := strings.HasPrefix(payload, ".")
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(payload, "."))
	if err != nil {
		return nil, false, false
	}
	if compressed {
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, false, false
		}
		// The signature rules out a payload that expands without bound.
		if data, err = io.ReadAll(zr); err != nil {
			return nil, false, false
		}
	}
	var rows []any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, false, false
	}
	all = true
	if n := len(rows); n > 0 && rows[n-1] == notFinished {
		rows, all = rows[:n-1], false
	}
	ms = make([]Message, 0, len(rows))
	for _, row := range rows {
		if m, ok := decodeMessage(row); ok {
			ms = append(ms, m)
		}
	}
	return ms, all, true
}

// validSignature reports whether sig signs payload with the secret or one of
// the fallbacks.
func validSignature(opts CookieOptions, payload, sig string) bool {
	for _, secret := range append([][]byte{opts.Secret}, opts.Fallbacks...) {
		if hmac.Equal([]byte(sig), []byte(signature(secret, payload))) {
			return true
		}
	}
	return false
}

// signature returns Django's signature of value for the messages cookie: the
// URL-safe base64 of an HMAC-SHA256 whose key is Django's salted_hmac key for
// the salt of the signer and the cookie signer key of secret.
func signature(secret []byte, value string) string {
	key := sha256.New()
	key.Write([]byte(cookieSalt + "signer" + "django.http.cookies"))
	key.Write(secret)
	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// base62Digits are the digits of Django's b62_encode.
const base62Digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// base62 returns n in Django's base62 (signing.b62_encode).
func base62(n int64) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	var digits []byte
	for n > 0 {
		digits = append(digits, base62Digits[n%62])
		n /= 62
	}
	slices.Reverse(digits)
	return sign + string(digits)
}

// bisectKeepLeft returns the index of the first element of a whose prefix,
// up to and including the element, meets tooLarge; len(a) when none does.
// tooLarge must hold for every longer prefix too (Django's
// bisect_keep_left).
func bisectKeepLeft[T any](a []T, tooLarge func([]T) bool) int {
	lo, hi := 0, len(a)
	for lo < hi {
		mid := (lo + hi) / 2
		if tooLarge(a[:mid+1]) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// bisectKeepRight returns the index of the first element of a whose suffix,
// from the element on, does not meet tooLarge; len(a) when none. tooLarge
// must hold for every longer suffix too (Django's bisect_keep_right).
func bisectKeepRight[T any](a []T, tooLarge func([]T) bool) int {
	lo, hi := 0, len(a)
	for lo < hi {
		mid := (lo + hi) / 2
		if tooLarge(a[mid:]) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
