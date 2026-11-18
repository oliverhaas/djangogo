package djangogo

import (
	"bytes"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
)

// TestVersionCommand checks that the version command and --version print the
// same line: the bare version, as Django's do.
func TestVersionCommand(t *testing.T) {
	app, err := New(conf.Settings{SecretKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, args := range [][]string{{"version"}, {"--version"}} {
		var buf bytes.Buffer
		app.Out = &buf
		if code := app.Run(args); code != 0 {
			t.Fatalf("Run(%v) = %d, want 0", args, code)
		}
		if got, want := buf.String(), Version+"\n"; got != want {
			t.Errorf("Run(%v) printed %q, want %q", args, got, want)
		}
	}
}
