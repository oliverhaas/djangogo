package messages_test

import (
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/messages"
)

// The tests in this file are the tests of Django's
// messages_tests/test_session.py SessionTests. The tests of BaseTests that
// SessionTests runs too are in django_base_test.go, as subtests for each
// storage.

// Ported from Django: tests/messages_tests/test_session.py::SessionTests.test_no_session
func TestSessionTestsNoSession(t *testing.T) {
	msg := "The session-based temporary message storage requires session " +
		"middleware to be installed, and come before the message " +
		"middleware in the MIDDLEWARE list."
	v := mustPanic(t, func() { messages.NewSessionStorage(nil) })
	if err, ok := v.(error); !ok || err.Error() != msg {
		t.Errorf("panic = %v, want an error with the text %q", v, msg)
	}
}

// Ported from Django: tests/messages_tests/test_session.py::SessionTests.test_get
//
// Django stores the plain strings "test" and "me" in the session. The Go
// storage reads only the JSON of stored messages, so the test stores
// messages.
func TestSessionTestsGet(t *testing.T) {
	storage := newStorage(messages.StorageSession)
	example := []messages.Message{{Level: messages.LevelInfo, Message: "test"}, {Level: messages.LevelInfo, Message: "me"}}
	storage.sess.Set(messages.SessionKey, messages.EncodeMessages(example))
	storage.ClearLoaded()
	if got := storage.Messages(); !slices.Equal(got, example) {
		t.Errorf("messages = %v, want %v", got, example)
	}
}

// Ported from Django: tests/messages_tests/test_session.py::SessionTests.test_safedata
func TestSessionTestsSafedata(t *testing.T) {
	// A message containing SafeData keeps its safe status when retrieved from
	// the message storage.
	storage := newStorage(messages.StorageSession)
	message := messages.Message{Level: messages.LevelDebug, Message: "<b>Hello Django!</b>", Safe: true}
	storage.sess.Set(messages.SessionKey, messages.EncodeMessages([]messages.Message{message}))
	storage.ClearLoaded()
	if got := storage.Messages(); len(got) != 1 || !got[0].Safe {
		t.Errorf("messages = %#v, want one safe message", got)
	}
}
