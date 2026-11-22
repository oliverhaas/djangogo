package templates

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/flosch/pongo2/v6"
)

// filterPprint is Django's pprint, a filter for debugging: the value as
// Python's pprint.pformat writes it. Django registers it with is_safe=True.
func filterPprint(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return keep(in, pformat(in))
}

// pformatWidth is the line width of Python's pprint.pformat.
const pformatWidth = 80

// pformat returns what Python's pprint.pformat gives for v: the repr when it
// fits in 80 characters, otherwise a list or map with one element per line
// and a string split into several literals, as pprint.PrettyPrinter
// formats them with its defaults.
func pformat(v any) string {
	p := prettyPrinter{}
	p.format(v, 0, 0, 0)
	return p.b.String()
}

// prettyPrinter is the state of pformat, a port of the formatting methods
// of Python's pprint.PrettyPrinter with indent=1, width=80 and
// compact=False.
type prettyPrinter struct {
	b strings.Builder
}

// format writes v, PrettyPrinter._format.
func (p *prettyPrinter) format(v any, indent, allowance, level int) {
	v = unwrapValue(v)
	rep := pyRepr(v)
	if utf8.RuneCountInString(rep) <= pformatWidth-indent-allowance {
		p.b.WriteString(rep)
		return
	}
	if s, ok := v.(string); ok {
		p.str(s, indent, allowance, level+1)
		return
	}
	_, special := v.(modelView)
	switch rv := reflect.ValueOf(v); {
	case special:
	case rv.Kind() == reflect.Map:
		p.dict(rv, indent, allowance, level+1)
		return
	case rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array:
		p.b.WriteByte('[')
		items := make([]any, rv.Len())
		for i := range items {
			items[i] = rv.Index(i).Interface()
		}
		p.items(items, indent, allowance+1, level+1)
		p.b.WriteByte(']')
		return
	}
	p.b.WriteString(rep)
}

// dict writes a map with one key per line, PrettyPrinter._pprint_dict and
// _format_dict_items.
func (p *prettyPrinter) dict(rv reflect.Value, indent, allowance, level int) {
	p.b.WriteByte('{')
	keys := sortedKeys(rv)
	indent++
	allowance++
	for i, k := range keys {
		last := i == len(keys)-1
		rep := pyRepr(k.Interface())
		p.b.WriteString(rep)
		p.b.WriteString(": ")
		p.format(rv.MapIndex(k).Interface(), indent+utf8.RuneCountInString(rep)+2, lastAllowance(last, allowance), level)
		if !last {
			p.b.WriteString(",\n" + strings.Repeat(" ", indent))
		}
	}
	p.b.WriteByte('}')
}

// items writes the elements of a list one per line,
// PrettyPrinter._format_items.
func (p *prettyPrinter) items(items []any, indent, allowance, level int) {
	indent++
	for i, item := range items {
		if i > 0 {
			p.b.WriteString(",\n" + strings.Repeat(" ", indent))
		}
		p.format(item, indent, lastAllowance(i == len(items)-1, allowance), level)
	}
}

// lastAllowance is the allowance of an element: the room the closing
// brackets need after the last one, and the comma after the others.
func lastAllowance(last bool, allowance int) int {
	if last {
		return allowance
	}
	return 1
}

// str writes a long string as several literals, split at line ends and
// then at white space, PrettyPrinter._pprint_str. At the top level the
// literals are put in parentheses.
func (p *prettyPrinter) str(s string, indent, allowance, level int) {
	if s == "" {
		p.b.WriteString(pyStrRepr(s))
		return
	}
	var chunks []string
	lines := splitLinesKeepEnds(s)
	if level == 1 {
		indent++
		allowance++
	}
	maxWidth1 := pformatWidth - indent
	maxWidth := maxWidth1
	reprLen := func(s string) int { return utf8.RuneCountInString(pyStrRepr(s)) }
	var rep string
	for i, line := range lines {
		rep = pyStrRepr(line)
		lastLine := i == len(lines)-1
		if lastLine {
			maxWidth1 -= allowance
		}
		if utf8.RuneCountInString(rep) <= maxWidth1 {
			chunks = append(chunks, rep)
			continue
		}
		parts := splitWords(line)
		maxWidth2 := maxWidth
		current := ""
		for j, part := range parts {
			candidate := current + part
			if j == len(parts)-1 && lastLine {
				maxWidth2 -= allowance
			}
			if reprLen(candidate) > maxWidth2 {
				if current != "" {
					chunks = append(chunks, pyStrRepr(current))
				}
				current = part
			} else {
				current = candidate
			}
		}
		if current != "" {
			chunks = append(chunks, pyStrRepr(current))
		}
	}
	if len(chunks) == 1 {
		p.b.WriteString(rep)
		return
	}
	if level == 1 {
		p.b.WriteByte('(')
	}
	for i, chunk := range chunks {
		if i > 0 {
			p.b.WriteString("\n" + strings.Repeat(" ", indent))
		}
		p.b.WriteString(chunk)
	}
	if level == 1 {
		p.b.WriteByte(')')
	}
}

// splitLinesKeepEnds splits s after each line boundary Python's
// str.splitlines knows, keeping the boundaries.
func splitLinesKeepEnds(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '\r':
			if strings.HasPrefix(s[i+size:], "\n") {
				size++
			}
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		default:
			i += size
			continue
		}
		i += size
		lines = append(lines, s[start:i])
		start = i
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// splitWords splits s into runs of non-space characters, each with the
// white space after it, what re.findall(r'\S*\s*', s) gives without its
// final empty match.
func splitWords(s string) []string {
	var parts []string
	for i := 0; i < len(s); {
		start := i
		for i < len(s) {
			r, size := utf8.DecodeRuneInString(s[i:])
			if unicode.IsSpace(r) {
				break
			}
			i += size
		}
		for i < len(s) {
			r, size := utf8.DecodeRuneInString(s[i:])
			if !unicode.IsSpace(r) {
				break
			}
			i += size
		}
		parts = append(parts, s[start:i])
	}
	return parts
}
