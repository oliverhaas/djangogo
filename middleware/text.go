package middleware

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// htmlContentType is the content type of the pages this package writes, which
// is Django's default content type.
const htmlContentType = "text/html; charset=utf-8"

// errorPageFormat is ERROR_PAGE_TEMPLATE of django/views/defaults.py, the page
// Django answers with when a project has no template for an error. The verbs
// take the title, the title again, and the details.
const errorPageFormat = `
<!doctype html>
<html lang="en">
<head>
  <title>%s</title>
</head>
<body>
  <h1>%s</h1><p>%s</p>
</body>
</html>
`

// htmlEscaper escapes the five characters Python's html.escape escapes, with
// the same entities.
var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#x27;",
)

// escapeHTML escapes s for use in HTML text and in quoted attribute values. It
// writes the entities of Python's html.escape rather than those of the Go
// html package, so that fragments of Django's pages can be compared verbatim.
// Bytes that are not valid UTF-8 become the replacement character.
func escapeHTML(s string) string {
	return htmlEscaper.Replace(strings.ToValidUTF8(s, "�"))
}

// writeErrorPage answers with Django's plain error page for the status. title
// is the page title, for example "Server Error (500)". details is HTML and goes
// into the page as it is, so escape any text in it first.
func writeErrorPage(w http.ResponseWriter, status int, title, details string) {
	w.Header().Set("Content-Type", htmlContentType)
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, errorPageFormat, title, title, details)
}

// writeEscapedRune writes r as the escape sequence Python uses for a character
// it does not print: \xhh, \uhhhh, or \Uhhhhhhhh, depending on the code point.
func writeEscapedRune(w io.Writer, r rune) {
	switch {
	case r <= 0xff:
		_, _ = fmt.Fprintf(w, `\x%02x`, r)
	case r <= 0xffff:
		_, _ = fmt.Fprintf(w, `\u%04x`, r)
	default:
		_, _ = fmt.Fprintf(w, `\U%08x`, r)
	}
}

// unicodeEscape is Python's s.encode("unicode_escape").decode("ascii"), which
// Django applies to the arguments of its request log messages: printable ASCII
// is kept, a backslash is doubled, and everything else becomes an escape
// sequence. A request cannot forge log lines or send terminal control
// sequences through a string escaped this way. A byte that is not valid UTF-8
// is written as \xhh.
func unicodeEscape(s string) string {
	clean := true
	for i := range len(s) {
		if c := s[i]; c < ' ' || c >= 0x7f || c == '\\' {
			clean = false
			break
		}
	}
	if clean {
		return s
	}

	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			_, _ = fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r >= ' ' && r < 0x7f:
			b.WriteRune(r)
		default:
			writeEscapedRune(&b, r)
		}
		i += size
	}
	return b.String()
}

// pyRepr formats s the way Python's repr formats a str. Django shows request
// data on the debug page with repr and quotes the host in the DisallowedHost
// message with it, and both are reproduced verbatim. The string is put in
// single quotes, or in double quotes when it holds a single quote and no
// double quote. Characters Python does not consider printable are escaped.
func pyRepr(s string) string {
	quote := '\''
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}

	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteRune(quote)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			_, _ = fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == quote || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsPrint(r):
			b.WriteRune(r)
		default:
			writeEscapedRune(&b, r)
		}
		i += size
	}
	b.WriteRune(quote)
	return b.String()
}
