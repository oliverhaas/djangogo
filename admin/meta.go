package admin

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/oliverhaas/djangogo/orm"
)

// modelVerboseNames returns the lowercase singular and plural verbose names of
// m, derived from its type name the way Django derives defaults before model
// Meta options exist ("Article" -> "article", "articles"; "ReadOnlyPizza" ->
// "read only pizza", "read only pizzas"). Once orm.Model exposes Meta verbose
// names, update this function to read them first.
func modelVerboseNames(m *orm.Model) (singular, plural string) {
	singular = camelCaseToSpaces(m.Name())
	plural = singular + "s"
	return singular, plural
}

// camelCaseToSpaces splits a Go type name at uppercase boundaries and
// lowercases it, matching django.utils.text.camel_case_to_spaces.
func camelCaseToSpaces(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			afterLower := unicode.IsLower(runes[i-1])
			beforeNonUpper := i+1 < len(runes) && !unicode.IsUpper(runes[i+1])
			if afterLower || beforeNonUpper {
				b.WriteByte(' ')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.TrimSpace(b.String())
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
