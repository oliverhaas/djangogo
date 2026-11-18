package apps

import (
	"strings"
	"unicode"
)

// defaultLabel returns the last dot- or slash-separated component of an app
// name. Django splits at the last dot of a Python path; the slash covers a Go
// import path.
func defaultLabel(name string) string {
	return name[strings.LastIndexAny(name, "./")+1:]
}

// isIdentifier reports whether s is a valid Python identifier, the test
// Django applies to an app label (str.isidentifier): a letter or an
// underscore, followed by letters, digits, underscores and combining marks.
// The label ends up in table names and migration names, so the rule is kept.
// Python applies the XID variants of the Unicode properties used here; the two
// differ in about twenty compatibility characters, which this check accepts.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if isIdentifierStart(r) {
			continue
		}
		if i == 0 || !isIdentifierContinue(r) {
			return false
		}
	}
	return true
}

// isIdentifierStart reports whether r may start an identifier (the Unicode
// property ID_Start, plus the underscore).
func isIdentifierStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.In(r, unicode.Nl, unicode.Other_ID_Start)
}

// isIdentifierContinue reports whether r may follow the first character of an
// identifier, in addition to the characters that may start one (the Unicode
// property ID_Continue).
func isIdentifierContinue(r rune) bool {
	return unicode.In(r, unicode.Nd, unicode.Mn, unicode.Mc, unicode.Pc, unicode.Other_ID_Continue)
}

// title returns s in title case as Python's str.title does: the first letter
// of every run of cased letters is upper case, the rest of the run is lower
// case, and a character without case (a digit, an underscore) ends the run.
func title(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inWord := false
	for _, r := range s {
		switch {
		case !isCased(r):
			inWord = false
		case inWord:
			r = unicode.ToLower(r)
		default:
			r = unicode.ToTitle(r)
			inWord = true
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isCased reports whether r is a letter with a case.
func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r)
}
