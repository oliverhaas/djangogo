// Package naming converts Go identifiers to the names that djangogo derives
// from them.
package naming

import (
	"strings"
	"unicode"
)

// SnakeCase converts a Go identifier in PascalCase or camelCase, with or
// without acronym runs, to snake_case: CreatedAt becomes created_at and
// GetAbsoluteURL becomes get_absolute_url. The orm names attributes and
// columns this way, and templates and the admin name model methods this way.
func SnakeCase(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if !unicode.IsUpper(r) {
			b.WriteRune(r)
			continue
		}
		if i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
			(i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
