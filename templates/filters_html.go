package templates

import (
	"html"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/flosch/pongo2/v6"
)

// The filters of this file build HTML. Under autoescape they escape their
// input, unless it is marked safe, and they return a value marked safe, so
// the markup they add is not escaped again. Inside {% autoescape off %} the
// input is used as it is (see autoescapeFilters).

// safe wraps a filter result that is finished HTML.
func safe(s string) (*pongo2.Value, *pongo2.Error) { return pongo2.AsSafeValue(s), nil }

// escaperFor returns the escaping a filter applies to the text of in: none
// when the value is marked safe or autoescape is off.
func escaperFor(in *pongo2.Value, autoescape bool) func(string) string {
	if !autoescape || isSafe(in) {
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

// linebreaksFilter returns Django's linebreaks: paragraphs separated by a
// blank line become <p> elements, single line breaks become <br>.
func linebreaksFilter(autoescape bool) pongo2.FilterFunction {
	return func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		paras := paragraphBreak.Split(escaperFor(in, autoescape)(newlines(text(in))), -1)
		for i, p := range paras {
			paras[i] = "<p>" + strings.ReplaceAll(p, "\n", "<br>") + "</p>"
		}
		return safe(strings.Join(paras, "\n\n"))
	}
}

// linebreaksbrFilter returns Django's linebreaksbr: every line break
// becomes <br>.
func linebreaksbrFilter(autoescape bool) pongo2.FilterFunction {
	return func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		return safe(strings.ReplaceAll(escaperFor(in, autoescape)(newlines(text(in))), "\n", "<br>"))
	}
}

// urlWordSplit separates the words urlize looks at, as Django does.
var urlWordSplit = regexp.MustCompile(`[\s<>"']+`)

// Patterns of the words urlize turns into links. urlBareDomain is Django's
// hostname and domain pattern without its look-arounds, which Go's regexp
// lacks: each label has 1 to 63 characters and neither starts nor ends with
// a hyphen.
var (
	urlWithScheme = regexp.MustCompile(`(?i)^https?://\[?\w`)
	urlBareDomain = regexp.MustCompile(`(?i)^` + urlLabel + `(?:\.` + urlLabel + `)*\.(com|edu|gov|int|mil|net|org)($|/.*)$`)
)

// urlLabel is one label of a host name.
const urlLabel = `[a-z\x{a1}-\x{ffff}0-9](?:[a-z\x{a1}-\x{ffff}0-9-]{0,61}[a-z\x{a1}-\x{ffff}0-9])?`

// trimPunctuation splits a word into leading punctuation, the part that may
// be a URL, and trailing punctuation, so that "(see example.com)." links
// only the address. It follows Django's Urlizer.trim_punctuation, which
// keeps a trailing ";" that ends an HTML entity.
func trimPunctuation(word string) (lead, middle, trail string) {
	middle = strings.TrimLeft(word, "([")
	lead = word[:len(word)-len(middle)]
	counts := map[string]int{
		"(": strings.Count(middle, "("), ")": strings.Count(middle, ")"),
		"[": strings.Count(middle, "["), "]": strings.Count(middle, "]"),
	}
	for changed := true; changed && middle != ""; {
		changed = false
		for _, pair := range [...][2]string{{"(", ")"}, {"[", "]"}} {
			opening, closing := pair[0], pair[1]
			if counts[opening] < counts[closing] && strings.TrimRight(middle, closing) != middle {
				strip := min(counts[closing]-counts[opening], len(middle))
				trail = middle[len(middle)-strip:] + trail
				middle = middle[:len(middle)-strip]
				changed = true
				counts[closing] -= strip
			}
		}
		amp := strings.LastIndex(middle, "&")
		chars := ".,:;!"
		if amp >= 0 {
			chars = ".,:!"
		}
		if stripped := strings.TrimRight(middle, chars); stripped != middle {
			trail, middle, changed = middle[len(stripped):]+trail, stripped, true
		}
		if !strings.HasSuffix(middle, ";") {
			continue
		}
		entity := middle[len(middle)-1:]
		if amp >= 0 {
			entity = middle[amp:]
		}
		if unescaped := html.UnescapeString(entity); unescaped != entity && !strings.HasSuffix(unescaped, ";") {
			// The ";" ends an entity.
			continue
		}
		stripped := strings.TrimRight(middle, ".,:;!")
		cut := len(stripped)
		semicolons := len(middle) - len(strings.TrimRight(middle, ";"))
		if amp >= 0 && semicolons > 1 {
			// The first ";" may end an entity.
			cut += strings.Index(middle[cut:], ";") + 1
			stripped = middle[:cut]
		}
		trail, middle, changed = middle[cut:]+trail, stripped, true
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
// escaping everything else under autoescape unless in is marked safe. limit,
// unless negative, shortens the link text.
func urlize(in *pongo2.Value, limit int, autoescape bool) string {
	s, esc := text(in), escaperFor(in, autoescape)
	var b strings.Builder
	last := 0
	word := func(w string) {
		if !strings.ContainsAny(w, ".@:") {
			b.WriteString(esc(w))
			return
		}
		lead, middle, trail := trimPunctuation(w)
		// The address of a link is taken from the text the entities stand
		// for.
		addr := html.UnescapeString(middle)
		const urlSafe = "!*'();:@&=+$,/?#[]~%"
		href, attrs := "", ` rel="nofollow"`
		switch {
		case urlWithScheme.MatchString(middle):
			href = percentEncode(addr, urlSafe)
		case strings.HasPrefix(strings.ToLower(middle), "www.") ||
			(!strings.HasPrefix(strings.ToLower(middle), "http") && urlBareDomain.MatchString(middle)):
			href = percentEncode("http://"+addr, urlSafe)
		case !strings.Contains(middle, ":") && isSimpleEmail(middle):
			local, domain, _ := strings.Cut(middle, "@")
			href, attrs = "mailto:"+percentEncode(local, "")+"@"+percentEncode(domain, ""), ""
		default:
			b.WriteString(esc(w))
			return
		}
		shown := middle
		if runes := []rune(shown); limit >= 0 && len(runes) > limit {
			shown = string(runes[:max(limit-1, 0)]) + ellipsis
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

// urlizeFilter returns Django's urlize, or with trunc its urlizetrunc:
// urlize with the link text cut to the given number of characters.
func urlizeFilter(trunc, autoescape bool) pongo2.FilterFunction {
	return func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		limit := -1
		if trunc {
			n, err := intArg("urlizetrunc", param)
			if err != nil {
				return nil, err
			}
			limit = n
		}
		return safe(urlize(in, limit, autoescape))
	}
}

// joinFilter returns Django's join: the elements of a list, or the
// characters of a string, joined by the argument. Under autoescape every
// element is escaped unless it is marked safe; the result is marked safe.
// trusted says that the separator is a string literal of the template,
// which is printed as written; a separator that comes from a variable is
// escaped like an element. Without autoescape, a list of anything but
// strings is given back, as Python's str.join fails on it.
func joinFilter(trusted, autoescape bool) pongo2.FilterFunction {
	return func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		sep := param.String()
		if !trusted && autoescape {
			sep = conditionalEscape(param)
		}
		var parts []string
		rv := reflect.ValueOf(in.Interface())
		switch {
		case isList(rv):
			parts = make([]string, rv.Len())
			for i := range parts {
				item := itemValue(elem(rv, i))
				switch {
				case autoescape:
					parts[i] = conditionalEscape(item)
				case !item.IsString():
					return in, nil
				default:
					parts[i] = item.String()
				}
			}
		case in.IsString():
			esc := escaperFor(in, autoescape)
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
func unorderedList(b *strings.Builder, list reflect.Value, tabs int, esc func(*pongo2.Value) string) {
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
		b.WriteString(indent + "<li>" + esc(itemValue(item)))
		if children.IsValid() && children.Len() > 0 {
			b.WriteString("\n" + indent + "<ul>\n")
			unorderedList(b, children, tabs+1, esc)
			b.WriteString("\n" + indent + "</ul>\n" + indent)
		}
		b.WriteString("</li>")
	}
}

// unorderedListFilter returns Django's unordered_list: a nested list as the
// <li> elements of an HTML list, without the outer <ul>.
func unorderedListFilter(autoescape bool) pongo2.FilterFunction {
	esc := conditionalEscape
	if !autoescape {
		esc = text
	}
	return func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		rv := reflect.ValueOf(in.Interface())
		if !isList(rv) {
			return safe("")
		}
		var b strings.Builder
		unorderedList(&b, rv, 1, esc)
		return safe(b.String())
	}
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
