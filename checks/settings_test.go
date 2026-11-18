package checks_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// messageIDs returns the id of every message, in order.
func messageIDs(msgs []checks.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.ID)
	}
	return out
}

// TestInvalidSettingIsReported reports a setting Django has no check for as
// settings.E001, with the name of the setting in front of the problem.
func TestInvalidSettingIsReported(t *testing.T) {
	s := conf.Defaults()
	s.SessionCookieAge = -time.Second

	msgs := taggedMessages(s, checks.TagSettings)

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1: %v", len(msgs), msgs)
	}
	djangotest.AssertEqual(t, msgs[0].ID, "settings.E001")
	djangotest.AssertEqual(t, msgs[0].Level, checks.LevelError)
	if !strings.HasPrefix(msgs[0].Msg, "SESSION_COOKIE_AGE: ") {
		t.Errorf("the message %q does not start with the name of the setting", msgs[0].Msg)
	}
}

// TestSettingsTagShowsEveryInvalidSetting reports the settings with a Django
// check under Django's id and under the tag settings too, each one once.
func TestSettingsTagShowsEveryInvalidSetting(t *testing.T) {
	s := conf.Defaults()
	s.StaticURL = "noslash"
	s.CSRFTrustedOrigins = []string{"example.com"}
	s.SessionCookieAge = -time.Second

	msgs := taggedMessages(s, checks.TagSettings)

	djangotest.AssertEqual(t, messageIDs(msgs), []string{"urls.E006", "4_0.E001", "settings.E001"})
}

// limits is the value of the settings namespace of TestNamespaceErrorIsReported.
type limits struct {
	Max int
}

// Validate refuses a negative limit.
func (l limits) Validate() error {
	if l.Max < 0 {
		return errors.New("the limit must not be negative")
	}
	return nil
}

// TestNamespaceErrorIsReported reports the error of a settings namespace as
// settings.E002.
func TestNamespaceErrorIsReported(t *testing.T) {
	const namespace = "checks_test_limits"
	if err := conf.Register(namespace, limits{Max: 10}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s := conf.Defaults()
	if err := s.SetNamespace(namespace, limits{Max: -1}); err != nil {
		t.Fatalf("SetNamespace: %v", err)
	}

	msgs := taggedMessages(s, checks.TagSettings)

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1: %v", len(msgs), msgs)
	}
	djangotest.AssertEqual(t, msgs[0].ID, "settings.E002")
	djangotest.AssertEqual(t, msgs[0].Level, checks.LevelError)
	if !strings.Contains(msgs[0].Msg, "the limit must not be negative") {
		t.Errorf("the message %q does not hold the error of the namespace", msgs[0].Msg)
	}
}

// TestStaticRootWrittenDifferently finds the static root among the static
// directories when the two settings spell the directory differently.
func TestStaticRootWrittenDifferently(t *testing.T) {
	root := t.TempDir()
	s := conf.Defaults()
	s.StaticRoot = root + string(filepath.Separator)
	s.StaticFilesDirs = []string{root + string(filepath.Separator) + "."}

	msgs := taggedMessages(s, checks.TagStaticfiles)

	djangotest.AssertEqual(t, messageIDs(msgs), []string{"staticfiles.E002"})
}

// TestStaticDirIsAFile warns about a static directory that is a file.
func TestStaticDirIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "static")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := conf.Defaults()
	s.StaticFilesDirs = []string{file}

	assertMessages(t, taggedMessages(s, checks.TagStaticfiles), checks.Warning(
		"The directory '"+file+"' in the STATICFILES_DIRS setting does not exist.",
		checks.ID("staticfiles.W004"),
	))
}
