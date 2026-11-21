package admin

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/oliverhaas/djangogo/orm"
)

// modelVerboseNames returns the singular and plural verbose names of m: its
// Meta.VerboseName and Meta.VerboseNamePlural, or Django's defaults derived
// from the type name.
func modelVerboseNames(m *orm.Model) (singular, plural string) {
	return m.VerboseName(), m.VerboseNamePlural()
}

// capFirst uppercases the first rune of s without altering the rest, matching
// django.utils.text.capfirst.
func capFirst(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// titleCase converts s the way Python's str.title() does: the first cased rune
// in each letter run is uppercased and subsequent cased runes are lowercased
// ("admin_views" -> "Admin_Views", "auth" -> "Auth").
func titleCase(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inWord := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			if !inWord {
				b.WriteRune(unicode.ToTitle(r))
				inWord = true
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
		} else {
			b.WriteRune(r)
			inWord = false
		}
	}
	return b.String()
}

// fieldLabel returns the display label of a model field, preferring its
// VerboseName and otherwise deriving it from Attr as Django does.
func fieldLabel(f *orm.Field) string {
	if f.VerboseName != "" {
		return capFirst(f.VerboseName)
	}
	return capFirst(strings.ReplaceAll(f.Attr, "_", " "))
}
