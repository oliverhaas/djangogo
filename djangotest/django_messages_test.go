package djangotest_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/sessions"
)

// fakeResponse returns a response whose request carries a message storage
// (Django's FakeResponse with request._messages = DummyStorage()).
func fakeResponse() *djangotest.Response {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	storage := messages.NewSessionStorage(sessions.NewSignedCookieStore([]byte("k")).New())
	return &djangotest.Response{Request: req.WithContext(messages.NewContext(req.Context(), storage))}
}

// addMessage adds a message to the request of resp.
func addMessage(t *testing.T, resp *djangotest.Response, level int, msg string, extraTags ...string) {
	t.Helper()
	if err := messages.Add(resp.Request, level, msg, extraTags...); err != nil {
		t.Fatal(err)
	}
}

// Ported from Django: tests/messages_tests/tests.py::AssertMessagesTest.test_assertion
//
// Django's DummyStorage records every level; the real storage drops a message
// below Config.Level, so the test lowers the level to DEBUG.
func TestAssertMessagesTestAssertion(t *testing.T) {
	t.Cleanup(messages.Configure(messages.Config{Level: messages.LevelDebug}))
	resp := fakeResponse()
	addMessage(t, resp, messages.LevelDebug, "DEBUG message.")
	addMessage(t, resp, messages.LevelInfo, "INFO message.")
	addMessage(t, resp, messages.LevelSuccess, "SUCCESS message.")
	addMessage(t, resp, messages.LevelWarning, "WARNING message.")
	addMessage(t, resp, messages.LevelError, "ERROR message.")
	djangotest.AssertMessages(t, resp, []messages.Message{
		{Level: messages.LevelDebug, Message: "DEBUG message."},
		{Level: messages.LevelInfo, Message: "INFO message."},
		{Level: messages.LevelSuccess, Message: "SUCCESS message."},
		{Level: messages.LevelWarning, Message: "WARNING message."},
		{Level: messages.LevelError, Message: "ERROR message."},
	})
}

// Ported from Django: tests/messages_tests/tests.py::AssertMessagesTest.test_with_tags
func TestAssertMessagesTestWithTags(t *testing.T) {
	resp := fakeResponse()
	addMessage(t, resp, messages.LevelInfo, "INFO message.", "extra-info")
	addMessage(t, resp, messages.LevelSuccess, "SUCCESS message.", "extra-success")
	addMessage(t, resp, messages.LevelWarning, "WARNING message.", "extra-warning")
	addMessage(t, resp, messages.LevelError, "ERROR message.", "extra-error")
	djangotest.AssertMessages(t, resp, []messages.Message{
		{Level: messages.LevelInfo, Message: "INFO message.", ExtraTags: "extra-info"},
		{Level: messages.LevelSuccess, Message: "SUCCESS message.", ExtraTags: "extra-success"},
		{Level: messages.LevelWarning, Message: "WARNING message.", ExtraTags: "extra-warning"},
		{Level: messages.LevelError, Message: "ERROR message.", ExtraTags: "extra-error"},
	})
}

// Ported from Django: tests/messages_tests/tests.py::AssertMessagesTest.test_custom_levelname
func TestAssertMessagesTestCustomLevelname(t *testing.T) {
	t.Cleanup(messages.Configure(messages.Config{Tags: map[int]string{42: "CUSTOM"}}))
	resp := fakeResponse()
	addMessage(t, resp, 42, "CUSTOM message.")
	djangotest.AssertMessages(t, resp, []messages.Message{{Level: 42, Message: "CUSTOM message."}})
}

// Ported from Django: tests/messages_tests/tests.py::AssertMessagesTest.test_ordered
func TestAssertMessagesTestOrdered(t *testing.T) {
	resp := fakeResponse()
	addMessage(t, resp, messages.LevelInfo, "First message.")
	addMessage(t, resp, messages.LevelWarning, "Second message.")
	expected := []messages.Message{
		{Level: messages.LevelWarning, Message: "Second message."},
		{Level: messages.LevelInfo, Message: "First message."},
	}
	djangotest.AssertMessages(t, resp, expected, djangotest.UnorderedMessages())
	mustFail(t, "Lists differ: ", func(tb djangotest.TB) {
		djangotest.AssertMessages(tb, resp, expected)
	})
}

// Ported from Django: tests/messages_tests/tests.py::AssertMessagesTest.test_mismatching_length
func TestAssertMessagesTestMismatchingLength(t *testing.T) {
	resp := fakeResponse()
	addMessage(t, resp, messages.LevelInfo, "INFO message.")
	want := "Lists differ: [Message(level=20, message='INFO message.')] != []\n\n" +
		"First list contains 1 additional elements.\n" +
		"First extra element 0:\n" +
		"Message(level=20, message='INFO message.')\n\n" +
		"- [Message(level=20, message='INFO message.')]\n" +
		"+ []"
	got := failures(func(tb djangotest.TB) { djangotest.AssertMessages(tb, resp, nil) })
	if len(got) != 1 || !strings.Contains(got[0], want) {
		t.Errorf("failures = %q, want one containing %q", got, want)
	}
}

// Ported from Django: tests/messages_tests/tests.py::AssertMessagesTest.test_method_frames_ignored_by_unittest
func TestAssertMessagesTestMethodFramesIgnoredByUnittest(t *testing.T) {
	t.Skip("the test inspects the Python traceback that unittest reports")
}

// The tests below are not ports: Django tests only the ordered and the
// length failure, so the other failure messages are compared with the output
// of Python's unittest for lists of Django messages.

// TestAssertMessagesUnorderedFailureMessage checks the failure message of
// the unordered assertion against assertCountEqual.
func TestAssertMessagesUnorderedFailureMessage(t *testing.T) {
	x := messages.Message{Level: messages.LevelInfo, Message: "x"}
	y := messages.Message{Level: messages.LevelWarning, Message: "y"}
	z := messages.Message{Level: messages.LevelWarning, Message: "z"}
	tests := []struct {
		name          string
		got, expected []messages.Message
		want          string
	}{
		{
			"extra message",
			[]messages.Message{x},
			nil,
			"Element counts were not equal:\nFirst has 1, Second has 0:  Message(level=20, message='x')",
		},
		{
			"missing message", nil,
			[]messages.Message{x},
			"Element counts were not equal:\nFirst has 0, Second has 1:  Message(level=20, message='x')",
		},
		{
			"different messages",
			[]messages.Message{x, y},
			[]messages.Message{x, z},
			"Element counts were not equal:\n" +
				"First has 1, Second has 0:  Message(level=30, message='y')\n" +
				"First has 0, Second has 1:  Message(level=30, message='z')",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := fakeResponse()
			for _, m := range tc.got {
				addMessage(t, resp, m.Level, m.Message)
			}
			msgs := failures(func(tb djangotest.TB) {
				djangotest.AssertMessages(tb, resp, tc.expected, djangotest.UnorderedMessages())
			})
			if len(msgs) != 1 || msgs[0] != tc.want {
				t.Errorf("failures = %q, want [%q]", msgs, tc.want)
			}
		})
	}
}

// TestAssertMessagesFirstDifferingElement checks the failure message of the
// ordered assertion for two lists of the same length against assertEqual. The
// line diff of Python has "?" hint lines that the Go message leaves out, so
// the test compares the part before the diff.
func TestAssertMessagesFirstDifferingElement(t *testing.T) {
	resp := fakeResponse()
	addMessage(t, resp, messages.LevelInfo, "x")
	addMessage(t, resp, messages.LevelWarning, "y")
	want := "Lists differ: [Message(level=20, message='x'), Message(level=30, message='y')] != " +
		"[Message(level=20, message='x'), Message(level=30, message='z')]\n\n" +
		"First differing element 1:\n" +
		"Message(level=30, message='y')\n" +
		"Message(level=30, message='z')\n\n"
	msgs := failures(func(tb djangotest.TB) {
		djangotest.AssertMessages(tb, resp, []messages.Message{
			{Level: messages.LevelInfo, Message: "x"},
			{Level: messages.LevelWarning, Message: "z"},
		})
	})
	if len(msgs) != 1 || !strings.HasPrefix(msgs[0], want) {
		t.Errorf("failures = %q, want one starting with %q", msgs, want)
	}
}

// TestAssertMessagesNeedsRecording checks that AssertMessages on a response
// whose request carries no storage, from a client without WithMessages, fails
// with a hint to WithMessages.
func TestAssertMessagesNeedsRecording(t *testing.T) {
	resp := &djangotest.Response{Request: httptest.NewRequest(http.MethodGet, "/", nil)}
	mustFail(t, "WithMessages", func(tb djangotest.TB) {
		djangotest.AssertMessages(tb, resp, nil)
	})
}
