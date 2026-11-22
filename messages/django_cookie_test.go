package messages_test

import (
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/messages"
)

// The tests in this file are the tests of Django's messages_tests/test_cookie.py
// CookieTests and BisectTests. The tests of BaseTests that CookieTests runs
// too are in django_session_test.go, as subtests for each storage.

// cookieTestsOptions are the cookie settings of CookieTests: its
// override_settings of SESSION_COOKIE_DOMAIN, SESSION_COOKIE_SECURE and
// SESSION_COOKIE_HTTPONLY.
var cookieTestsOptions = messages.CookieOptions{Secret: testSecret, Domain: ".example.com", Secure: true}

// cookieRequest returns a request whose messages cookie has value, or no
// messages cookie when value is empty.
func cookieRequest(value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if value != "" {
		r.AddCookie(&http.Cookie{Name: messages.CookieName, Value: value})
	}
	return r
}

// textMessages returns a message of LevelInfo for each text.
func textMessages(texts ...string) []messages.Message {
	out := make([]messages.Message, len(texts))
	for i, text := range texts {
		out[i] = messages.Message{Level: messages.LevelInfo, Message: text}
	}
	return out
}

// randomTexts returns n texts of size random letters and digits, the same on
// every run, as random.seed(42) and get_random_string in Django's tests: zlib
// compresses them little.
func randomTexts(n, size int) []string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rng := rand.New(rand.NewPCG(42, 42))
	out := make([]string, n)
	for i := range out {
		b := make([]byte, size)
		for k := range b {
			b[k] = chars[rng.IntN(len(chars))]
		}
		out[i] = string(b)
	}
	return out
}

// lastCookie returns the last messages cookie that h sets.
func lastCookie(t *testing.T, h http.Header) *http.Cookie {
	t.Helper()
	var found *http.Cookie
	for _, c := range (&http.Response{Header: h}).Cookies() {
		if c.Name == messages.CookieName {
			found = c
		}
	}
	if found == nil {
		t.Fatalf("no %s cookie in %q", messages.CookieName, h.Values("Set-Cookie"))
	}
	return found
}

// encodeDecode stores one message of LevelDebug in a cookie and reads it back
// (CookieTests.encode_decode).
func encodeDecode(t *testing.T, m messages.Message) messages.Message {
	t.Helper()
	m.Level = messages.LevelDebug
	ms, _, ok := messages.DecodeCookie(cookieTestsOptions, messages.EncodeCookie(cookieTestsOptions, []messages.Message{m}, false, false))
	if !ok || len(ms) != 1 {
		t.Fatalf("decoded %v, %v; want one message", ms, ok)
	}
	return ms[0]
}

// Ported from Django: tests/messages_tests/test_cookie.py::CookieTests.test_get
//
// Django stores the plain strings "test" and "me". The Go storage reads only
// stored messages, so the test stores messages.
func TestCookieTestsGet(t *testing.T) {
	example := textMessages("test", "me")
	storage := messages.NewCookieStorage(cookieRequest(messages.EncodeCookie(cookieTestsOptions, example, false, false)), cookieTestsOptions)
	if got := storage.Messages(); !slices.Equal(got, example) {
		t.Errorf("messages = %v, want %v", got, example)
	}
}

// Ported from Django: tests/messages_tests/test_cookie.py::CookieTests.test_cookie_settings
//
// CookieStorage honors SESSION_COOKIE_DOMAIN, SESSION_COOKIE_SECURE, and
// SESSION_COOKIE_HTTPONLY (#15618, #20972). net/http writes the domain
// without its leading dot, which browsers ignore.
func TestCookieTestsCookieSettings(t *testing.T) {
	opts := cookieTestsOptions
	opts.SameSite = http.SameSiteStrictMode

	// Test before the messages have been consumed
	storage := messages.NewCookieStorage(cookieRequest(""), opts)
	h := http.Header{}
	storage.Add(messages.LevelInfo, "test", "")
	storage.Update(h)
	c := lastCookie(t, h)
	ms, _, _ := messages.DecodeCookie(opts, c.Value)
	if len(ms) != 1 || ms[0].Message != "test" {
		t.Errorf("stored messages = %v, want one with the text test", ms)
	}
	if c.Domain != "example.com" || c.RawExpires != "" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie = %q, want Domain=example.com, no Expires, Secure, HttpOnly and SameSite=Strict", c.String())
	}

	// Deletion of the cookie (storing with an empty value) after the messages
	// have been consumed.
	storage = messages.NewCookieStorage(cookieRequest(""), opts)
	h = http.Header{}
	storage.Add(messages.LevelInfo, "test", "")
	storage.Messages() // Simulates consumption of the messages.
	storage.Update(h)
	c = lastCookie(t, h)
	if c.Value != "" || c.Domain != "example.com" || c.RawExpires != "Thu, 01 Jan 1970 00:00:00 GMT" || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie = %q, want an empty value, Domain=example.com, the expiry of 1970 and SameSite=Strict", h.Get("Set-Cookie"))
	}
}

// Ported from Django: tests/messages_tests/test_cookie.py::CookieTests.test_get_bad_cookie
func TestCookieTestsGetBadCookie(t *testing.T) {
	// Set initial (invalid) data.
	value := messages.EncodeCookie(cookieTestsOptions, textMessages("test", "me"), false, false)
	storage := messages.NewCookieStorage(cookieRequest(value[1:]), cookieTestsOptions)
	// The message actually contains what we expect.
	if got := storage.Messages(); len(got) != 0 {
		t.Errorf("messages = %v, want none", got)
	}
}

// Ported from Django: tests/messages_tests/test_cookie.py::CookieTests.test_max_cookie_length
//
// If the data exceeds what is allowed in a cookie, older messages are removed
// before saving (and returned by Update).
func TestCookieTestsMaxCookieLength(t *testing.T) {
	storage := messages.NewCookieStorage(cookieRequest(""), cookieTestsOptions)
	storage.SetLoaded(nil)
	h := http.Header{}
	// When storing as a cookie, the cookie has constant overhead of approx 54
	// chars, and each message has a constant overhead of about 37 chars and a
	// variable overhead of zero in the best case. We aim for a message size
	// which will fit 4 messages into the cookie, but not 5. See also
	// TestFallbackTestsSessionFallback.
	msgSize := (messages.MaxCookieSize-54)*2/9 - 37 // int((max_cookie_size - 54) / 4.5 - 37)
	texts := randomTexts(5, msgSize)
	for _, text := range texts {
		storage.Add(messages.LevelInfo, text, "")
	}
	unstored := storage.Update(h)
	if got := storedCookieCount(h); got != 4 {
		t.Errorf("stored messages = %d, want 4", got)
	}
	if len(unstored) != 1 || unstored[0].Message != texts[0] {
		t.Errorf("unstored = %v, want the first message", unstored)
	}
}

// Ported from Django: tests/messages_tests/test_cookie.py::CookieTests.test_message_rfc6265
func TestCookieTestsMessageRFC6265(t *testing.T) {
	encoded := messages.EncodeCookie(cookieTestsOptions, textMessages("\\te,st", ";m\"e", "\u2019", "123\"NOTRECEIVED\""), false, false)
	if i := strings.IndexAny(encoded, "\\,;\""); i >= 0 {
		t.Errorf("encoded %q holds %q", encoded, encoded[i])
	}
}

// No Go counterpart: tests/messages_tests/test_cookie.py::CookieTests.test_json_encoder_decoder
// encodes Python values that nest messages in lists and dicts. The Go cookie
// holds a list of messages only.

// Ported from Django: tests/messages_tests/test_cookie.py::CookieTests.test_safedata
//
// A message containing SafeData is keeping its safe status when retrieved
// from the message storage.
func TestCookieTestsSafedata(t *testing.T) {
	if !encodeDecode(t, messages.Message{Message: "<b>Hello Django!</b>", Safe: true}).Safe {
		t.Error("a safe message is not safe after the cookie")
	}
	if encodeDecode(t, messages.Message{Message: "<b>Hello Django!</b>"}).Safe {
		t.Error("a message is safe after the cookie")
	}
}

// Ported from Django: tests/messages_tests/test_cookie.py::CookieTests.test_extra_tags
//
// A message's extra_tags attribute is correctly preserved when retrieved from
// the message storage. Django also checks extra_tags None, which a Go string
// cannot hold.
func TestCookieTestsExtraTags(t *testing.T) {
	for _, extraTags := range []string{"", "some tags"} {
		t.Run(extraTags, func(t *testing.T) {
			if got := encodeDecode(t, messages.Message{Message: "message", ExtraTags: extraTags}).ExtraTags; got != extraTags {
				t.Errorf("extra tags = %q, want %q", got, extraTags)
			}
		})
	}
}

// sum adds up a.
func sum(a []int) int {
	total := 0
	for _, x := range a {
		total += x
	}
	return total
}

// Ported from Django: tests/messages_tests/test_cookie.py::BisectTests.test_bisect_keep_left
func TestBisectTestsBisectKeepLeft(t *testing.T) {
	tests := []struct {
		a    []int
		fn   func([]int) bool
		want int
	}{
		{[]int{1, 1, 1}, func(a []int) bool { return sum(a) != 2 }, 2},
		{[]int{1, 1, 1}, func(a []int) bool { return sum(a) != 0 }, 0},
		{[]int{}, func(a []int) bool { return sum(a) != 0 }, 0},
	}
	for _, tt := range tests {
		if got := messages.BisectKeepLeft(tt.a, tt.fn); got != tt.want {
			t.Errorf("BisectKeepLeft(%v) = %d, want %d", tt.a, got, tt.want)
		}
	}
}

// Ported from Django: tests/messages_tests/test_cookie.py::BisectTests.test_bisect_keep_right
func TestBisectTestsBisectKeepRight(t *testing.T) {
	tests := []struct {
		a    []int
		fn   func([]int) bool
		want int
	}{
		{[]int{1, 1, 1}, func(a []int) bool { return sum(a) != 2 }, 1},
		{[]int{1, 1, 1, 1}, func(a []int) bool { return sum(a) != 2 }, 2},
		{[]int{1, 1, 1, 1, 1}, func(a []int) bool { return sum(a) != 1 }, 4},
		{[]int{}, func(a []int) bool { return sum(a) != 0 }, 0},
	}
	for _, tt := range tests {
		if got := messages.BisectKeepRight(tt.a, tt.fn); got != tt.want {
			t.Errorf("BisectKeepRight(%v) = %d, want %d", tt.a, got, tt.want)
		}
	}
}
