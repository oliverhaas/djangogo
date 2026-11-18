package templates

import (
	"strings"

	"github.com/flosch/pongo2/v6"
)

// htmlEscaper escapes the five characters Django's escape() does, with the
// same entities (the apostrophe as &#x27;, where pongo2 writes &#39;).
var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#x27;",
)

// escapeHTML escapes s for HTML text and attribute values like Django.
func escapeHTML(s string) string { return htmlEscaper.Replace(s) }

// text returns the text of a template value for a filter that works on
// strings. A model instance gives its label unescaped, where String would
// give the escaped form meant for direct output.
func text(v *pongo2.Value) string {
	if mv, ok := v.Interface().(modelView); ok {
		return mv.label()
	}
	return v.String()
}

// filterEscape implements Django's escape filter. pongo2 also runs it on every
// string printed under autoescape, so this is where the entities of the whole
// engine are decided.
func filterEscape(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(escapeHTML(text(in))), nil
}

// filterForceEscape implements Django's force_escape: escape now, and let
// autoescape escape the result again if it is printed unmarked.
func filterForceEscape(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(escapeHTML(text(in))), nil
}
