// application_test.go
package djangogo

import (
	"testing"

	"github.com/oliverhaas/djangogo/conf"
)

func TestNewWiresRegistries(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if app.Settings.Host != "127.0.0.1" {
		t.Errorf("defaults not applied; Host = %q", app.Settings.Host)
	}
	if app.Handler == nil {
		t.Error("Handler should be set")
	}
	// built-in commands are registered
	names := app.Commands.Names()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["runserver"] || !found["version"] {
		t.Errorf("built-in commands missing: %v", names)
	}
}

func TestNewRejectsBadSettings(t *testing.T) {
	if _, err := New(conf.Settings{}); err == nil {
		t.Error("New should reject settings without a SecretKey")
	}
}

func TestExecuteRunsCommand(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := app.Execute([]string{"version"}); err != nil {
		t.Errorf("Execute(version): %v", err)
	}
}
