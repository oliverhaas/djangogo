package templates

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/flosch/pongo2/v6"
)

// ellipsis is the character Django appends to truncated text.
const ellipsis = "…"

// str wraps a filter result that autoescape should still escape.
func str(s string) (*pongo2.Value, *pongo2.Error) { return pongo2.AsValue(s), nil }

// filterTitle implements Django's title: the first letter of every word in
// upper case, the rest in lower case. A letter after an apostrophe or a digit
// stays lower ("it's", "2nd"), which pongo2's filter capitalizes.
func filterTitle(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	runes := []rune(text(in))
	inWord := false
	for i, r := range runes {
		switch {
		case !unicode.IsLetter(r):
			inWord = false
		case inWord:
			runes[i] = unicode.ToLower(r)
		default:
			inWord = true
			afterApostrophe := i >= 2 && runes[i-1] == '\'' && unicode.IsLower(runes[i-2])
			afterDigit := i >= 1 && unicode.IsDigit(runes[i-1])
			if afterApostrophe || afterDigit {
				runes[i] = unicode.ToLower(r)
			} else {
				runes[i] = unicode.ToTitle(r)
			}
		}
	}
	return keep(in, string(runes))
}

// asciiFold maps the accented Latin letters to their base letter, standing
// in for the Unicode decomposition Django's slugify applies.
var asciiFold = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a", "ā", "a", "ă", "a", "ą", "a",
	"ç", "c", "ć", "c", "č", "c", "ď", "d",
	"è", "e", "é", "e", "ê", "e", "ë", "e", "ē", "e", "ė", "e", "ę", "e", "ě", "e",
	"ğ", "g", "ì", "i", "í", "i", "î", "i", "ï", "i", "ī", "i", "į", "i",
	"ķ", "k", "ļ", "l", "ľ", "l", "ñ", "n", "ń", "n", "ň", "n", "ņ", "n",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ō", "o", "ő", "o",
	"ŕ", "r", "ř", "r", "ś", "s", "š", "s", "ş", "s", "ţ", "t", "ť", "t",
	"ù", "u", "ú", "u", "û", "u", "ü", "u", "ū", "u", "ů", "u", "ű", "u", "ų", "u",
	"ý", "y", "ÿ", "y", "ź", "z", "ż", "z", "ž", "z",
)

// slugify converts s to a URL slug like Django: lower case ASCII letters,
// digits, underscores and hyphens, with runs of spaces and hyphens collapsed
// to one hyphen.
func slugify(s string) string {
	s = asciiFold.Replace(strings.ToLower(s))
	var b strings.Builder
	pendingDash := false
	for _, r := range s {
		switch {
		case r == '-' || unicode.IsSpace(r):
			pendingDash = true
		case r < unicode.MaxASCII && (r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)):
			if pendingDash {
				b.WriteByte('-')
				pendingDash = false
			}
			b.WriteRune(r)
		}
	}
	if pendingDash {
		b.WriteByte('-')
	}
	return strings.Trim(b.String(), "-_")
}

// filterSlugify implements Django's slugify.
func filterSlugify(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return keep(in, slugify(text(in)))
}

// filterTruncatechars implements Django's truncatechars: at most n characters,
// the last of them an ellipsis when text was cut. An argument that is not a
// number gives the value back.
func filterTruncatechars(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	n, ok := asInt(param)
	if !ok {
		return in, nil
	}
	runes := []rune(text(in))
	switch {
	case n <= 0:
		return keep(in, "")
	case len(runes) <= n:
		return keep(in, string(runes))
	}
	return keep(in, string(runes[:n-1])+ellipsis)
}

// filterTruncatewords implements Django's truncatewords: at most n words,
// followed by an ellipsis when words were cut. An argument that is not a
// number gives the value back.
func filterTruncatewords(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	n, ok := asInt(param)
	if !ok {
		return in, nil
	}
	words := strings.Fields(text(in))
	switch {
	case n <= 0:
		return keep(in, "")
	case len(words) > n:
		return keep(in, strings.Join(words[:n], " ")+" "+ellipsis)
	}
	return keep(in, strings.Join(words, " "))
}

// pyLineBreak reports whether r ends a line for Python's str.splitlines.
func pyLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', '\u2028', '\u2029':
		return true
	}
	return false
}

// splitLines splits s like Python's str.splitlines: at every line break,
// "\r\n" counting as one, without a last empty line.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if i < start || !pyLineBreak(r) {
			continue
		}
		lines = append(lines, s[start:i])
		start = i + utf8.RuneLen(r)
		if r == '\r' && strings.HasPrefix(s[start:], "\n") {
			start++
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// wrapSpace reports whether r is white space for Python's textwrap.
func wrapSpace(r rune) bool { return strings.ContainsRune("\t\n\v\f\r ", r) }

// expandTabs replaces the tabs of a line with spaces up to the next multiple
// of eight columns, like Python's str.expandtabs.
func expandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// wrapLine wraps one line like Python's textwrap.TextWrapper with the
// options of Django's wrap: words are never broken, white space is kept
// except where a line is broken, and white space at the start of the line is
// kept.
func wrapLine(line string, width int) []string {
	var chunks []string
	for rest := expandTabs(line); rest != ""; {
		space := wrapSpace([]rune(rest)[0])
		end := strings.IndexFunc(rest, func(r rune) bool { return wrapSpace(r) != space })
		if end < 0 {
			end = len(rest)
		}
		chunks = append(chunks, rest[:end])
		rest = rest[end:]
	}
	isSpace := func(c string) bool { return strings.TrimFunc(c, wrapSpace) == "" }
	var lines []string
	for i := 0; i < len(chunks); {
		if len(lines) > 0 && isSpace(chunks[i]) {
			i++
		}
		var cur []string
		curLen := 0
		for i < len(chunks) {
			n := utf8.RuneCountInString(chunks[i])
			if curLen+n > width {
				break
			}
			cur = append(cur, chunks[i])
			curLen += n
			i++
		}
		if i < len(chunks) && len(cur) == 0 && utf8.RuneCountInString(chunks[i]) > width {
			cur = append(cur, chunks[i])
			i++
		}
		if len(cur) > 0 && isSpace(cur[len(cur)-1]) {
			cur = cur[:len(cur)-1]
		}
		if len(cur) > 0 {
			lines = append(lines, strings.Join(cur, ""))
		}
	}
	return lines
}

// filterWordwrap implements Django's wordwrap: lines of at most the given
// number of characters, broken at white space only; existing line breaks
// and other white space are kept.
func filterWordwrap(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	width, err := intArg("wordwrap", param)
	if err != nil {
		return nil, err
	}
	s := text(in)
	lines := splitLines(s)
	if width <= 0 && len(lines) > 0 {
		return nil, filterError("wordwrap", fmt.Errorf("invalid width %d (must be > 0)", width))
	}
	var out []string
	for _, line := range lines {
		wrapped := wrapLine(line, width)
		if len(wrapped) == 0 {
			// A line of white space only is kept as it is.
			out = append(out, line)
			continue
		}
		out = append(out, wrapped...)
	}
	if strings.HasSuffix(s, "\n") {
		out = append(out, "")
	}
	return keep(in, strings.Join(out, "\n"))
}

// filterStringformat implements Django's stringformat: the value formatted
// with a printf verb given without its percent sign ("03d", ".2f", "s"). A
// verb that does not fit the value gives an empty string. "s" formats
// Python's str() of the value and "r" its repr().
func filterStringformat(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	spec := param.String()
	if spec == "" {
		return keep(in, "")
	}
	var arg any
	flags, verb := spec[:len(spec)-1], spec[len(spec)-1]
	switch verb {
	case 's':
		arg = pyStr(in)
	case 'r':
		arg, verb = pyRepr(in), 's'
	case 'i', 'u':
		verb = 'd'
	}
	switch verb {
	case 'd', 'x', 'X', 'o', 'c':
		if !in.IsNumber() {
			return keep(in, "")
		}
		arg = in.Integer()
	case 'e', 'E', 'f', 'F', 'g', 'G':
		if !in.IsNumber() {
			return keep(in, "")
		}
		arg = in.Float()
	case 's':
	default:
		return keep(in, "")
	}
	out := fmt.Sprintf("%"+flags+string(verb), arg)
	if strings.Contains(out, "%!") {
		return keep(in, "")
	}
	return keep(in, out)
}

// filterMakeList implements Django's make_list: the characters of the
// value's text, so a number gives its digits and a list the characters of
// its repr. pongo2's filter takes the Go formatting of the value.
func filterMakeList(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := text(in)
	out := make([]any, 0, len(s))
	for _, r := range s {
		out = append(out, string(r))
	}
	return pongo2.AsValue(out), nil
}

// filterPluralize implements Django's pluralize: the plural suffix ("s", or
// the argument: "es", "y,ies") unless the value is 1, or a list of one
// element.
func filterPluralize(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	singular, plural := "", "s"
	if !param.IsNil() {
		parts := strings.Split(param.String(), ",")
		switch len(parts) {
		case 1:
			plural = parts[0]
		case 2:
			singular, plural = parts[0], parts[1]
		default:
			return str("")
		}
	}
	one := false
	switch {
	case in.IsNumber():
		one = in.Float() == 1
	case in.IsString():
		f, err := strconv.ParseFloat(strings.TrimSpace(in.String()), 64)
		if err != nil {
			return str("")
		}
		one = f == 1
	case in.IsNil():
		return str("")
	default:
		rv := reflect.ValueOf(in.Interface())
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			one = rv.Len() == 1
		case reflect.Bool:
			one = rv.Bool()
		default:
			return str("")
		}
	}
	if one {
		return str(singular)
	}
	return str(plural)
}

// percentEncode encodes s like Python's urllib.parse.quote: every byte except
// ASCII letters, digits, "_.-~" and the characters of safe becomes %XX.
func percentEncode(s, safe string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '_', c == '.', c == '-', c == '~', strings.IndexByte(safe, c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0f])
		}
	}
	return b.String()
}

// filterURLEncode implements Django's urlencode: percent-encoding that keeps
// "/" unless an argument names the characters to keep ("" for none). pongo2's
// filter encodes a space as "+".
func filterURLEncode(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	safe := "/"
	if !param.IsNil() {
		safe = param.String()
	}
	return str(percentEncode(text(in), safe))
}

// filterIRIEncode implements Django's iriencode: non-ASCII characters and
// spaces of an IRI percent-encoded, the URL syntax characters kept.
func filterIRIEncode(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return keep(in, percentEncode(text(in), "/#%[]=:;$&()+,!?*@'~"))
}

// filterFilesizeformat implements Django's filesizeformat: a byte count as
// "13 KB", "4.1 MB", "102 bytes", with a no-break space before the unit.
func filterFilesizeformat(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	bytes := in.Float()
	if in.IsString() {
		f, err := strconv.ParseFloat(strings.TrimSpace(in.String()), 64)
		if err != nil {
			return keep(in, "0"+nbsp+"bytes")
		}
		bytes = f
	}
	sign := ""
	if bytes < 0 {
		sign, bytes = "-", -bytes
	}
	const kb = 1 << 10
	units := [...]string{"KB", "MB", "GB", "TB", "PB"}
	if bytes < kb {
		unit := "bytes"
		if int64(bytes) == 1 {
			unit = "byte"
		}
		return keep(in, sign+strconv.FormatInt(int64(bytes), 10)+nbsp+unit)
	}
	scaled, unit := bytes/kb, units[0]
	for _, next := range units[1:] {
		if scaled < kb {
			break
		}
		scaled, unit = scaled/kb, next
	}
	return keep(in, sign+strconv.FormatFloat(math.RoundToEven(scaled*10)/10, 'f', 1, 64)+nbsp+unit)
}

// filterIntcomma implements humanize's intcomma: the digits before the
// decimal point grouped in threes ("4,500,000").
func filterIntcomma(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := in.String()
	if in.IsFloat() {
		s = strconv.FormatFloat(in.Float(), 'f', -1, 64)
	}
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || strings.TrimLeft(whole, "0123456789") != "" {
		return str(in.String())
	}
	out := sign + groupThousands(whole)
	if frac != "" {
		out += "." + frac
	}
	return str(out)
}

// filterCenter implements Django's center: the text in the middle of a field
// of the given width, the odd space on the right.
func filterCenter(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := text(in)
	pad := param.Integer() - utf8.RuneCountInString(s)
	if pad <= 0 {
		return keep(in, s)
	}
	return keep(in, strings.Repeat(" ", pad/2)+s+strings.Repeat(" ", pad-pad/2))
}

// stripTagsOnce removes the tags and comments of s. A "<" that does not
// start a tag ("1 < 2") is text and stays, as does a tag that is never
// closed.
func stripTagsOnce(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 || i+1 >= len(s) {
			break
		}
		b.WriteString(s[:i])
		rest := s[i:]
		end := -1
		switch c := rest[1]; {
		case strings.HasPrefix(rest, "<!--"):
			if j := strings.Index(rest, "-->"); j >= 0 {
				end = j + len("-->")
			}
		case c == '/' || c == '!' || c == '?' || (c|0x20 >= 'a' && c|0x20 <= 'z'):
			if j := strings.IndexByte(rest, '>'); j >= 0 {
				end = j + 1
			}
		}
		if end < 0 {
			b.WriteByte('<')
			s = rest[1:]
			continue
		}
		s = rest[end:]
	}
	b.WriteString(s)
	return b.String()
}

// filterStriptags implements Django's striptags. The tags are removed again
// until none is left, so that a tag put together from pieces
// ("<<b>script>") does not survive. The result is plain text: autoescape
// still applies to it, unless the input was marked safe.
func filterStriptags(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := text(in)
	for range 50 {
		stripped := stripTagsOnce(s)
		if stripped == s {
			break
		}
		s = stripped
	}
	return keep(in, s)
}
