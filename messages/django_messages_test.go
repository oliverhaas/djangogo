package messages_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/messages"
)

// Ported from Django: tests/messages_tests/tests.py::MessageTests.test_eq
//
// Django also compares with mock.ANY, which has no Go counterpart.
func TestMessageTestsEq(t *testing.T) {
	msg1 := messages.Message{Level: messages.LevelInfo, Message: "Test message 1"}
	msg2 := messages.Message{Level: messages.LevelInfo, Message: "Test message 2"}
	msg3 := messages.Message{Level: messages.LevelWarning, Message: "Test message 1"}
	if !msg1.Equal(msg1) {
		t.Error("msg1 != msg1")
	}
	if msg1.Equal(msg2) {
		t.Error("msg1 == msg2")
	}
	if msg1.Equal(msg3) {
		t.Error("msg1 == msg3")
	}
	if msg2.Equal(msg3) {
		t.Error("msg2 == msg3")
	}
}

// Ported from Django: tests/messages_tests/tests.py::MessageTests.test_repr
func TestMessageTestsRepr(t *testing.T) {
	withConfig(t, messages.Config{Tags: map[int]string{
		messages.LevelWarning: "caution",
		messages.LevelError:   "",
		12:                    "custom",
	}})
	tests := []struct {
		level     int
		message   string
		extraTags string
		want      string
	}{
		{messages.LevelInfo, "thing", "", "Message(level=20, message='thing')"},
		{messages.LevelWarning, "careful", "tag1 tag2", "Message(level=30, message='careful', extra_tags='tag1 tag2')"},
		{messages.LevelError, "oops", "tag", "Message(level=40, message='oops', extra_tags='tag')"},
		{12, "custom", "", "Message(level=12, message='custom')"},
	}
	for _, tc := range tests {
		t.Run(tc.message, func(t *testing.T) {
			msg := messages.Message{Level: tc.level, Message: tc.message, ExtraTags: tc.extraTags}
			if got := msg.Repr(); got != tc.want {
				t.Errorf("Repr() = %s, want %s", got, tc.want)
			}
		})
	}
}

// levelTags is TestLevelTags.message_tags.
var levelTags = map[int]string{
	messages.LevelInfo:    "info",
	messages.LevelDebug:   "",
	messages.LevelWarning: "",
	messages.LevelError:   "bad",
	messages.LevelSuccess: "",
	12:                    "custom",
}

// Ported from Django: tests/messages_tests/tests.py::TestLevelTags.test_override_settings_level_tags
//
// Django compares base.LEVEL_TAGS with the map; the Go test compares the
// LevelTag of every level in it, as the map overrides all default levels.
func TestTestLevelTagsOverrideSettingsLevelTags(t *testing.T) {
	withConfig(t, messages.Config{Tags: levelTags})
	for level, want := range levelTags {
		if got := (messages.Message{Level: level}).LevelTag(); got != want {
			t.Errorf("LevelTag of level %d = %q, want %q", level, got, want)
		}
	}
}

// Ported from Django: tests/messages_tests/tests.py::TestLevelTags.test_lazy
//
// Django checks that the tags are read from the settings at first use. The
// Go tags are read at every use; the test checks that a configuration made
// after the Message was built is the one that applies.
func TestTestLevelTagsLazy(t *testing.T) {
	msg := messages.Message{Level: messages.LevelError}
	withConfig(t, messages.Config{Tags: map[int]string{messages.LevelError: "bad"}})
	if got := msg.LevelTag(); got != "bad" {
		t.Errorf("LevelTag() = %q, want bad", got)
	}
}

// Ported from Django: tests/messages_tests/tests.py::TestLevelTags.test_override_settings_lazy
//
// Django changes the setting inside override_settings and expects the new
// tags; the Go test configures again inside the first configuration.
func TestTestLevelTagsOverrideSettingsLazy(t *testing.T) {
	withConfig(t, messages.Config{Tags: levelTags})
	withConfig(t, messages.Config{Tags: map[int]string{messages.LevelError: "very-bad"}})
	if got := (messages.Message{Level: messages.LevelError}).LevelTag(); got != "very-bad" {
		t.Errorf("LevelTag() = %q, want very-bad", got)
	}
}
