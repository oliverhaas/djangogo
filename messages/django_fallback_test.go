package messages_test

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/sessions"
)

// The tests in this file are the tests of Django's
// messages_tests/test_fallback.py FallbackTests. The tests of BaseTests that
// FallbackTests runs too are in django_session_test.go, as subtests for each
// storage.

// fallbackStorage returns a fallback storage of a request whose messages
// cookie is cookie, none when it is empty, and whose session holds session,
// with the session (FallbackTests.get_request, set_cookie_data and
// set_session_data).
func fallbackStorage(cookie string, session []messages.Message) (*messages.Storage, *sessions.Session) {
	sess := sessions.NewSignedCookieStore([]byte("k")).New()
	if session != nil {
		sess.Set(messages.SessionKey, messages.EncodeMessages(session))
	}
	return messages.NewFallbackStorage(cookieRequest(cookie), sess, testCookieOptions), sess
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_get
//
// Django replaces the _get method of the session storage to prove that it is
// not used. The Go test checks that the session was not read.
func TestFallbackTestsGet(t *testing.T) {
	// Set initial cookie data.
	example := textMessages("0", "1", "2", "3", "4")
	storage, sess := fallbackStorage(messages.EncodeCookie(testCookieOptions, example, false, false), nil)
	if got := storage.Messages(); !slices.Equal(got, example) {
		t.Errorf("messages = %v, want %v", got, example)
	}
	if sess.Accessed() {
		t.Error("the session was read")
	}
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_get_empty
func TestFallbackTestsGetEmpty(t *testing.T) {
	storage, sess := fallbackStorage("", nil)
	if got := storage.Messages(); len(got) != 0 {
		t.Errorf("messages = %v, want none", got)
	}
	if sess.Accessed() {
		t.Error("the session was read")
	}
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_get_fallback
func TestFallbackTestsGetFallback(t *testing.T) {
	// Set initial cookie and session data.
	example := textMessages("0", "1", "2", "3", "4")
	storage, _ := fallbackStorage(messages.EncodeCookie(testCookieOptions, example[:4], true, false), example[4:])
	if got := storage.Messages(); !slices.Equal(got, example) {
		t.Errorf("messages = %v, want %v", got, example)
	}
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_get_fallback_only
func TestFallbackTestsGetFallbackOnly(t *testing.T) {
	// Set initial cookie and session data.
	example := textMessages("0", "1", "2", "3", "4")
	storage, _ := fallbackStorage(messages.EncodeCookie(testCookieOptions, nil, true, true), example)
	if got := storage.Messages(); !slices.Equal(got, example) {
		t.Errorf("messages = %v, want %v", got, example)
	}
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_flush_used_backends
func TestFallbackTestsFlushUsedBackends(t *testing.T) {
	// Set initial cookie and session data.
	storage, sess := fallbackStorage(messages.EncodeCookie(testCookieOptions, textMessages("cookie"), true, false), textMessages("session"))
	// When updating, previously used but no longer needed backends are
	// flushed.
	h := http.Header{}
	storage.Messages()
	storage.Update(h)
	if got := storedSessionCount(sess); got != 0 {
		t.Errorf("stored session messages = %d, want 0", got)
	}
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_no_fallback
//
// (1) A short number of messages whose data size doesn't exceed what is
// allowed in a cookie will all be stored in the cookie.
//
// (2) If the cookie can store all messages, the session won't be written to
// at all. Django replaces the _store method of the session storage to prove
// it; the Go test checks that the session was not changed.
func TestFallbackTestsNoFallback(t *testing.T) {
	storage, sess := fallbackStorage("", nil)
	storage.SetLoaded(nil)
	h := http.Header{}
	for i := range 5 {
		storage.Add(messages.LevelInfo, strings.Repeat(strconv.Itoa(i), 100), "")
	}
	storage.Update(h)
	if got := storedCookieCount(h); got != 5 {
		t.Errorf("stored cookie messages = %d, want 5", got)
	}
	if got := storedSessionCount(sess); got != 0 {
		t.Errorf("stored session messages = %d, want 0", got)
	}
	if sess.Modified() {
		t.Error("the session was changed")
	}
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_session_fallback
//
// If the data exceeds what is allowed in a cookie, messages which did not fit
// are stored in the session.
func TestFallbackTestsSessionFallback(t *testing.T) {
	storage, sess := fallbackStorage("", nil)
	storage.SetLoaded(nil)
	h := http.Header{}
	// See the comment in TestCookieTestsMaxCookieLength.
	msgSize := (messages.MaxCookieSize-54)*2/9 - 37 // int((max_cookie_size - 54) / 4.5 - 37)
	for _, text := range randomTexts(5, msgSize) {
		storage.Add(messages.LevelInfo, text, "")
	}
	storage.Update(h)
	if got := storedCookieCount(h); got != 4 {
		t.Errorf("stored cookie messages = %d, want 4", got)
	}
	if got := storedSessionCount(sess); got != 1 {
		t.Errorf("stored session messages = %d, want 1", got)
	}
}

// Ported from Django: tests/messages_tests/test_fallback.py::FallbackTests.test_session_fallback_only
//
// Large messages, none of which fit in a cookie, are stored in the session
// (and nothing is stored in the cookie).
func TestFallbackTestsSessionFallbackOnly(t *testing.T) {
	storage, sess := fallbackStorage("", nil)
	storage.SetLoaded(nil)
	h := http.Header{}
	storage.Add(messages.LevelInfo, randomTexts(1, 5000)[0], "")
	storage.Update(h)
	if got := storedCookieCount(h); got != 0 {
		t.Errorf("stored cookie messages = %d, want 0", got)
	}
	if got := storedSessionCount(sess); got != 1 {
		t.Errorf("stored session messages = %d, want 1", got)
	}
}
