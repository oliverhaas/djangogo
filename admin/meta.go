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

// modelNgettext returns m's singular verbose name when n == 1 and its plural
// verbose name otherwise, matching django.contrib.admin.utils.model_ngettext.
func modelNgettext(m *orm.Model, n int) string {
	singular, plural := modelVerboseNames(m)
	if n == 1 {
		return singular
	}
	return plural
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
// VerboseName, using "ID" for an auto/primary-key "id" field (matching Django's
// AutoField verbose_name), and otherwise deriving it from Attr.
func fieldLabel(f *orm.Field) string {
	if f.VerboseName != "" {
		return capFirst(f.VerboseName)
	}
	if f.PrimaryKey && f.Attr == "id" {
		return "ID"
	}
	return capFirst(strings.ReplaceAll(f.Attr, "_", " "))
}

// fieldVerboseName returns the uncapitalized verbose name of f for sidebar
// filter titles ("By <title>"), matching Django's Field.verbose_name.
func fieldVerboseName(f *orm.Field) string {
	if f.VerboseName != "" {
		return f.VerboseName
	}
	if f.PrimaryKey && f.Attr == "id" {
		return "ID"
	}
	return strings.ReplaceAll(f.Attr, "_", " ")
}

// prettyName turns a snake_case or "__"-separated name into a human-readable
// label, matching django.forms.utils.pretty_name.
func prettyName(name string) string {
	if name == "" {
		return ""
	}
	return capFirst(strings.ReplaceAll(name, "_", " "))
}
