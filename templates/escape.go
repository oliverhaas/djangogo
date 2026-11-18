package templates

import (
	"reflect"
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

// isSafe reports whether v is marked as HTML that needs no escaping: by the
// safe filter, by MarkSafe or by a filter of this package that builds HTML.
//
// pongo2 keeps the mark in an unexported field and has no accessor for it, so
// it is read through reflection. Should the field be renamed, every value
// counts as not safe and is escaped, which TestIsSafe reports.
func isSafe(v *pongo2.Value) bool {
	if v == nil {
		return false
	}
	mark := reflect.ValueOf(v).Elem().FieldByName("safe")
	return mark.IsValid() && mark.Kind() == reflect.Bool && mark.Bool()
}

// conditionalEscape returns the text of v escaped for HTML, or as it is when
// v is marked safe, Django's conditional_escape.
func conditionalEscape(v *pongo2.Value) string {
	if isSafe(v) {
		return text(v)
	}
	return escapeHTML(text(v))
}

// filterEscape implements Django's escape filter. pongo2 also runs it on
// every string printed under autoescape, so this is where the entities of
// the whole engine are decided. The result is marked safe, so that
// {{ value|escape }} is escaped once and not again by autoescape. Input that
// is already marked safe is left alone.
func filterEscape(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsSafeValue(conditionalEscape(in)), nil
}

// filterForceEscape implements Django's force_escape: it escapes its input
// even when that is marked safe.
func filterForceEscape(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsSafeValue(escapeHTML(text(in))), nil
}

// filterSafe implements Django's safe filter. pongo2's own filter only notes
// that it was applied, which is enough to print the value unescaped; this one
// also marks a string, so that a filter or tag that receives the value next
// ({{ text|safe|linebreaks }}) leaves it unescaped too.
func filterSafe(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if _, isModel := in.Interface().(modelView); isModel || in.IsString() {
		return pongo2.AsSafeValue(text(in)), nil
	}
	return in, nil
}

// MarkSafe marks s as HTML that is safe to output as is, Django's mark_safe.
// A view passes markup it built itself (a rendered form, say) into a context
// this way, and the template prints it without the safe filter. Never mark a
// string safe that contains unescaped user input.
func MarkSafe(s string) any { return pongo2.AsSafeValue(s) }
