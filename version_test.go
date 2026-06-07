// version_test.go
package djangogo

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var buf bytes.Buffer
	cmd := versionCommand{out: &buf}

	if cmd.Name() != "version" {
		t.Errorf("Name() = %q", cmd.Name())
	}
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(buf.String(), Version) {
		t.Errorf("output %q does not contain version %q", buf.String(), Version)
	}
}
