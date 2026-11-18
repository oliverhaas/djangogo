package templates

import (
	"html"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/flosch/pongo2/v6"
)

// The filters of this file build HTML. They escape their input, unless it is
// marked safe, and return a value marked safe, so the markup they add is not
// escaped again. pongo2 does not show a filter whether autoescape is on, so
// the input is escaped inside {% autoescape off %} as well, where Django
// leaves it alone.

// safe wraps a filter result that is finished HTML.
func safe(s string) (*pongo2.Value, *pongo2.Error) { return pongo2.AsSafeValue(s), nil }

// escaperFor returns the escaping a filter applies to the text of in: none
// when the value is marked safe.
func escaperFor(in *pongo2.Value) func(string) string {
	if isSafe(in) {
		return func(s string) string { return s }
	}
	return escapeHTML
}

// newlines normalizes the line endings of s to "\n".
func newlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// paragraphBreak separates the paragraphs of linebreaks.
var paragraphBreak = regexp.MustCompile(`\n{2,}`)

// filterLinebreaks implements Django's linebreaks: paragraphs separated by a
// blank line become <p> elements, single line breaks become <br>.
func filterLinebreaks(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	paras := paragraphBreak.Split(escaperFor(in)(newlines(text(in))), -1)
	for i, p := range paras {
		paras[i] = "<p>" + strings.ReplaceAll(p, "\n", "<br>") + "</p>"
	}
	return safe(strings.Join(paras, "\n\n"))
}

// filterLinebreaksbr implements Django's linebreaksbr: every line break
// becomes <br>.
func filterLinebreaksbr(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return safe(strings.ReplaceAll(escaperFor(in)(newlines(text(in))), "\n", "<br>"))
}

// urlWordSplit separates the words urlize looks at, as Django does.
var urlWordSplit = regexp.MustCompile(`[\s<>"']+`)

// Patterns of the words urlize turns into links.
var (
	urlWithScheme = regexp.MustCompile(`(?i)^https?://\[?\w`)
	urlBareDomain = regexp.MustCompile(`(?i)^[a-z0-9_][^@]+\.(com|edu|gov|int|mil|net|org)($|/.*)$`)
)

// trimPunctuation splits a word into leading punctuation, the part that may
// be a URL, and trailing punctuation, so that "(see example.com)." links
// only the address.
func trimPunctuation(word string) (lead, middle, trail string) {
	middle = word
	for changed := true; changed; {
		changed = false
		for _, pair := range [...][2]string{{"(", ")"}, {"[", "]"}} {
			if strings.HasPrefix(middle, pair[0]) {
				middle, lead, changed = middle[1:], lead+pair[0], true
			}
			if strings.HasSuffix(middle, pair[1]) &&
				strings.Count(middle, pair[1]) == strings.Count(middle, pair[0])+1 {
				middle, trail, changed = middle[:len(middle)-1], pair[1]+trail, true
			}
		}
		if stripped := strings.TrimRight(middle, ".,:;!"); stripped != middle {
			trail, middle, changed = middle[len(stripped):]+trail, stripped, true
		}
	}
	return lead, middle, trail
}

// isSimpleEmail reports whether s looks like an e-mail address.
func isSimpleEmail(s string) bool {
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") {
		return false
	}
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".")
}

// urlize turns the URLs and e-mail addresses in the text of in into links,
// escaping everything else unless in is marked safe. limit, when positive,
// shortens the link text.
func urlize(in *pongo2.Value, limit int) string {
	s, esc, markedSafe := text(in), escaperFor(in), isSafe(in)
	var b strings.Builder
	last := 0
	word := func(w string) {
		if !strings.ContainsAny(w, ".@:") {
			b.WriteString(esc(w))
			return
		}
		lead, middle, trail := trimPunctuation(w)
		shown := middle
		if markedSafe {
			// The address of a link is taken from the text the entities
			// stand for.
			middle = html.UnescapeString(middle)
		}
		const urlSafe = "!*'();:@&=+$,/?#[]~%"
		href, attrs := "", ` rel="nofollow"`
		switch {
		case urlWithScheme.MatchString(middle):
			href = percentEncode(middle, urlSafe)
		case strings.HasPrefix(strings.ToLower(middle), "www.") ||
			(!strings.HasPrefix(strings.ToLower(middle), "http") && urlBareDomain.MatchString(middle)):
			href = percentEncode("http://"+middle, urlSafe)
		case !strings.Contains(middle, ":") && isSimpleEmail(middle):
			local, domain, _ := strings.Cut(middle, "@")
			href, attrs = "mailto:"+percentEncode(local, "")+"@"+percentEncode(domain, ""), ""
		default:
			b.WriteString(esc(w))
			return
		}
		if runes := []rune(shown); limit > 0 && len(runes) > limit {
			shown = string(runes[:limit-1]) + ellipsis
		}
		b.WriteString(esc(lead))
		b.WriteString(`<a href="` + escapeHTML(href) + `"` + attrs + `>` + esc(shown) + `</a>`)
		b.WriteString(esc(trail))
	}
	for _, loc := range urlWordSplit.FindAllStringIndex(s, -1) {
		word(s[last:loc[0]])
		b.WriteString(esc(s[loc[0]:loc[1]]))
		last = loc[1]
	}
	word(s[last:])
	return b.String()
}

// filterUrlize implements Django's urlize.
func filterUrlize(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return safe(urlize(in, 0))
}

// filterUrlizetrunc implements Django's urlizetrunc: urlize with the link
// text cut to the given number of characters.
func filterUrlizetrunc(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return safe(urlize(in, param.Integer()))
}

// joinFilter returns Django's join: the elements of a list, or the
// characters of a string, joined by the argument. Every element is escaped
// unless it is marked safe, and the result is marked safe. trusted says that
// the separator is a string literal of the template, which is printed as
// written; a separator that comes from a variable is escaped like an element.
func joinFilter(trusted bool) pongo2.FilterFunction {
	return func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		sep := param.String()
		if !trusted {
			sep = conditionalEscape(param)
		}
		var parts []string
		rv := reflect.ValueOf(in.Interface())
		switch {
		case isList(rv):
			parts = make([]string, rv.Len())
			for i := range parts {
				parts[i] = conditionalEscape(itemValue(elem(rv, i)))
			}
		case in.IsString():
			esc := escaperFor(in)
			parts = make([]string, 0, len(in.String()))
			for _, r := range in.String() {
				parts = append(parts, esc(string(r)))
			}
		default:
			// Django gives back a value that cannot be joined.
			return in, nil
		}
		return safe(strings.Join(parts, sep))
	}
}

// filterDefaultLiteral implements default for an argument that is a string
// literal of the template: the literal is printed as written.
func filterDefaultLiteral(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in.IsTrue() {
		return in, nil
	}
	return safe(param.String())
}

// filterDefaultIfNoneLiteral implements default_if_none for an argument that
// is a string literal of the template.
func filterDefaultIfNoneLiteral(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if !in.IsNil() {
		return in, nil
	}
	return safe(param.String())
}

// isList reports whether rv is a list for unordered_list and dictsort.
func isList(rv reflect.Value) bool {
	return rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array)
}

// elem returns the i-th element of a list with interfaces unpacked.
func elem(rv reflect.Value, i int) reflect.Value {
	e := rv.Index(i)
	for e.Kind() == reflect.Interface && !e.IsNil() {
		e = e.Elem()
	}
	return e
}

// itemValue returns the template value of a list element, which keeps the
// safe mark of an element that has one.
func itemValue(item reflect.Value) *pongo2.Value {
	if !item.IsValid() || !item.CanInterface() {
		return pongo2.AsValue(nil)
	}
	if v, ok := item.Interface().(*pongo2.Value); ok {
		return v
	}
	return pongo2.AsValue(item.Interface())
}

// unorderedList writes the <li> elements of a nested list. A list that
// follows an item holds the children of that item.
func unorderedList(b *strings.Builder, list reflect.Value, tabs int) {
	indent := strings.Repeat("\t", tabs)
	first := true
	for i := 0; i < list.Len(); i++ {
		item := elem(list, i)
		var children reflect.Value
		if i+1 < list.Len() {
			if next := elem(list, i+1); isList(next) {
				children = next
				i++
			}
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		b.WriteString(indent + "<li>" + conditionalEscape(itemValue(item)))
		if children.IsValid() && children.Len() > 0 {
			b.WriteString("\n" + indent + "<ul>\n")
			unorderedList(b, children, tabs+1)
			b.WriteString("\n" + indent + "</ul>\n" + indent)
		}
		b.WriteString("</li>")
	}
}

// filterUnorderedList implements Django's unordered_list: a nested list as
// the <li> elements of an HTML list, without the outer <ul>.
func filterUnorderedList(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	rv := reflect.ValueOf(in.Interface())
	if !isList(rv) {
		return safe("")
	}
	var b strings.Builder
	unorderedList(&b, rv, 1)
	return safe(b.String())
}

// htmlTokenRe finds the tags and comments of an HTML fragment.
var htmlTokenRe = regexp.MustCompile(`(?s)<!--.*?-->|</?[a-zA-Z][^>]*>`)

// voidElements are the HTML elements without an end tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "param": true,
	"source": true, "track": true, "wbr": true,
}

// htmlWords splits text at the white space between words. White space at the
// start or the end stays with the first or the last word, as in Django.
func htmlWords(data string) []string {
	fields := strings.Fields(data)
	if len(fields) == 0 {
		return []string{data}
	}
	rest := strings.TrimLeftFunc(data, unicode.IsSpace)
	fields[0] = data[:len(data)-len(rest)] + fields[0]
	fields[len(fields)-1] += rest[len(strings.TrimRightFunc(rest, unicode.IsSpace)):]
	return fields
}

// truncateHTML shortens the text of an HTML fragment to limit characters or
// words. Tags do not count and stay; the elements still open at the cut are
// closed. Text is written escaped, so the result holds no markup that the
// input did not hold as markup. Comments are dropped.
func truncateHTML(s string, limit int, words bool) string {
	if limit <= 0 {
		return ""
	}
	remaining, mark := limit-1, ellipsis
	if words {
		remaining, mark = limit, " "+ellipsis
	}
	var (
		out  strings.Builder
		open []string
		seen int
	)
	// data handles the text between two tags and reports whether the limit
	// is reached.
	data := func(chunk string, atEnd bool) bool {
		chunk = html.UnescapeString(chunk)
		var units []string
		if words {
			units = htmlWords(chunk)
		} else {
			units = strings.Split(chunk, "")
			seen += len(units)
			if seen == limit && atEnd {
				out.WriteString(escapeHTML(chunk))
				return true
			}
		}
		sep := ""
		if words {
			sep = " "
		}
		if remaining < len(units) {
			out.WriteString(escapeHTML(strings.Join(units[:remaining], sep)) + mark)
			return true
		}
		remaining -= len(units)
		out.WriteString(escapeHTML(strings.Join(units, sep)))
		return false
	}
	finish := func() string {
		for i := len(open) - 1; i >= 0; i-- {
			out.WriteString("</" + open[i] + ">")
		}
		return out.String()
	}
	last := 0
	for _, loc := range htmlTokenRe.FindAllStringIndex(s, -1) {
		if loc[0] > last && data(s[last:loc[0]], false) {
			return finish()
		}
		last = loc[1]
		tag := s[loc[0]:loc[1]]
		if strings.HasPrefix(tag, "<!--") {
			continue
		}
		closing := strings.HasPrefix(tag, "</")
		name := strings.TrimLeft(tag, "</")
		if end := strings.IndexAny(name, " \t\r\n/>"); end >= 0 {
			name = name[:end]
		}
		name = strings.ToLower(name)
		if !closing {
			out.WriteString(tag)
			if !voidElements[name] {
				open = append(open, name)
			}
		}
		if (closing || strings.HasSuffix(tag, "/>")) && !voidElements[name] {
			out.WriteString("</" + name + ">")
			if len(open) > 0 && open[len(open)-1] == name {
				open = open[:len(open)-1]
			}
		}
	}
	if last < len(s) && data(s[last:], true) {
		return finish()
	}
	return out.String()
}

// truncateHTMLFilter returns Django's truncatechars_html or
// truncatewords_html. The result is marked safe only when the input is, so
// markup in text that is not trusted is escaped on output. pongo2's own
// filters print it as it is.
func truncateHTMLFilter(words bool) pongo2.FilterFunction {
	return func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		limit, ok := asInt(param)
		if !ok {
			return in, nil
		}
		out := truncateHTML(text(in), limit, words)
		if isSafe(in) {
			return safe(out)
		}
		return str(out)
	}
}

// seqFilter returns Django's safeseq or escapeseq: the elements of a list
// as text that is marked safe, escaped first or not. The filters exist for
// the elements of a list that is printed through join.
func seqFilter(escape bool) pongo2.FilterFunction {
	return func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		rv := reflect.ValueOf(in.Interface())
		if !isList(rv) {
			return in, nil
		}
		out := make([]any, rv.Len())
		for i := range out {
			item := itemValue(elem(rv, i))
			if escape {
				out[i] = pongo2.AsSafeValue(conditionalEscape(item))
			} else {
				out[i] = pongo2.AsSafeValue(text(item))
			}
		}
		return pongo2.AsValue(out), nil
	}
}
