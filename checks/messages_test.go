package checks_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// TestLevelNames names every level and reads the name back in any letter case.
func TestLevelNames(t *testing.T) {
	tests := []struct {
		level checks.Level
		name  string
	}{
		{checks.LevelDebug, "DEBUG"},
		{checks.LevelInfo, "INFO"},
		{checks.LevelWarning, "WARNING"},
		{checks.LevelError, "ERROR"},
		{checks.LevelCritical, "CRITICAL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			djangotest.AssertEqual(t, tt.level.String(), tt.name)
			for _, name := range []string{tt.name, strings.ToLower(tt.name)} {
				got, err := checks.ParseLevel(name)
				if err != nil {
					t.Fatalf("ParseLevel(%q): %v", name, err)
				}
				djangotest.AssertEqual(t, got, tt.level)
			}
		})
	}
}

// TestParseLevelUnknownName refuses a name that is no level with the text of
// Django's argument parser for check --fail-level.
func TestParseLevelUnknownName(t *testing.T) {
	_, err := checks.ParseLevel("x")

	if !errors.Is(err, checks.ErrInvalidLevel) {
		t.Fatalf("ParseLevel(\"x\") = %v, want an error that wraps ErrInvalidLevel", err)
	}
	djangotest.AssertEqual(t, err.Error(),
		"invalid choice: 'x' (choose from 'CRITICAL', 'ERROR', 'WARNING', 'INFO', 'DEBUG')")
}

// TestLevelStringOfUnnamedLevel prints a level between the named ones by its
// number.
func TestLevelStringOfUnnamedLevel(t *testing.T) {
	djangotest.AssertEqual(t, checks.Level(25).String(), "Level(25)")
}

// TestPrintingNilPointerObject prints "?" for an object that is a nil pointer,
// as for no object.
func TestPrintingNilPointerObject(t *testing.T) {
	var model *orm.Model
	e := checks.Error("Message", checks.Obj(model))

	djangotest.AssertEqual(t, e.String(), "?: Message")
}

// TestIsSerious compares the level of a message with the fail level.
func TestIsSerious(t *testing.T) {
	warning := checks.Warning("Message")

	djangotest.AssertEqual(t, warning.IsSerious(checks.LevelWarning), true)
	djangotest.AssertEqual(t, warning.IsSerious(checks.LevelError), false)
}

// TestIsSilenced silences a message by its id.
func TestIsSilenced(t *testing.T) {
	warning := checks.Warning("Message", checks.ID("app.W001"))

	djangotest.AssertEqual(t, warning.IsSilenced([]string{"app.E001", "app.W001"}), true)
	djangotest.AssertEqual(t, warning.IsSilenced([]string{"app.E001"}), false)
	djangotest.AssertEqual(t, warning.IsSilenced(nil), false)
}
