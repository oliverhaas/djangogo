package naming

import "testing"

// No Django test exists: Go identifiers have no Python counterpart.
func TestSnakeCase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"CreatedAt", "created_at"},
		{"ID", "id"},
		{"APIKey", "api_key"},
		{"URLSlug", "url_slug"},
		{"HTTPServer", "http_server"},
		{"Name", "name"},
		{"GetAbsoluteURL", "get_absolute_url"},
		{"Line2Total", "line2_total"},
	}
	for _, c := range cases {
		if got := SnakeCase(c.input); got != c.want {
			t.Errorf("SnakeCase(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
