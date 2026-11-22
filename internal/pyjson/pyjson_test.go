package pyjson_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/internal/pyjson"
)

// No Django test exists: the expected values are the output of Python's
// json.dumps with its default ensure_ascii.
func TestWriteString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", `""`},
		{"plain text", `"plain text"`},
		{"quote \" and backslash \\", `"quote \" and backslash \\"`},
		{"\n\r\t\b\f", `"\n\r\t\b\f"`},
		{"\x00\x1f\x7f", `"\u0000\u001f\u007f"`},
		{"caf\u00e9 \u2019", `"caf\u00e9 \u2019"`},
		{"\U0001F600", `"\ud83d\ude00"`},
		{"</script>", `"</script>"`},
	}
	for _, tt := range tests {
		var b strings.Builder
		pyjson.WriteString(&b, tt.in)
		if got := b.String(); got != tt.want {
			t.Errorf("WriteString(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
