package templates

import (
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/flosch/pongo2/v6"
)

// The filters of this file replace pongo2 filters that drop the safe mark of
// their input. Django marks the result of such a filter (is_safe=True) safe
// when the input was, so {{ html|safe|lower }} stays unescaped.

// keep wraps the result of a filter Django registers with is_safe=True: it
// is safe when the input was.
func keep(in *pongo2.Value, s string) (*pongo2.Value, *pongo2.Error) {
	if isSafe(in) {
		return pongo2.AsSafeValue(s), nil
	}
	return pongo2.AsValue(s), nil
}

// filterError wraps the error of a filter for pongo2.
func filterError(name string, err error) *pongo2.Error {
	return &pongo2.Error{Sender: "filter:" + name, OrigError: err}
}

// intArg converts the argument of a filter like Python's int() does, with
// the error Django's filter would raise.
func intArg(name string, param *pongo2.Value) (int, *pongo2.Error) {
	n, ok := asInt(param)
	if !ok {
		return 0, filterError(name, fmt.Errorf("invalid literal for int(): %q", param.String()))
	}
	return n, nil
}

// noEscapePrefix marks the variant of a filter that escapes its input
// itself for use inside {% autoescape off %}, see noEscapeFilters.
const noEscapePrefix = "_noescape_"

// autoescapeFilters are Django's needs_autoescape filters: they escape their
// input themselves when autoescape is on. A pongo2 filter cannot see the
// setting, so each is registered twice, under its name for autoescape on
// and with noEscapePrefix for off, and djangoSyntax picks the variant.
var autoescapeFilters = map[string]func(autoescape bool) pongo2.FilterFunction{
	"join":           func(on bool) pongo2.FilterFunction { return joinFilter(false, on) },
	"linebreaks":     linebreaksFilter,
	"linebreaksbr":   linebreaksbrFilter,
	"linenumbers":    linenumbersFilter,
	"unordered_list": unorderedListFilter,
	"urlize":         func(on bool) pongo2.FilterFunction { return urlizeFilter(false, on) },
	"urlizetrunc":    func(on bool) pongo2.FilterFunction { return urlizeFilter(true, on) },
}

// needsAutoescape reports whether name is one of autoescapeFilters.
var needsAutoescape = func() map[string]bool {
	m := make(map[string]bool, len(autoescapeFilters))
	for name := range autoescapeFilters {
		m[name] = true
	}
	return m
}()

// safeLiteralFilter is the filter djangoSyntax puts behind a string literal
// at the start of a {{ }} tag.
const safeLiteralFilter = "_safe_literal"

// filterSafeLiteral marks a string literal of the template safe, as Django
// does with every literal.
func filterSafeLiteral(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsSafeValue(in.String()), nil
}

// filterOut is the last filter of every {{ }} tag (see djangoSyntax). pongo2
// escapes only values that are strings; a value it prints through its String
// method, such as a model instance or any other fmt.Stringer, would reach the
// page unescaped. filterOut turns such a value into its text, which
// autoescape then escapes like any other string. A value marked safe stays
// as it is.
func filterOut(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	for {
		inner, ok := in.Interface().(*pongo2.Value)
		if !ok || inner == nil {
			break
		}
		in = inner
	}
	if in.IsString() || isSafe(in) {
		return in, nil
	}
	if in.IsFloat() {
		return pongo2.AsValue(pyFloatText(in.Float())), nil
	}
	switch v := in.Interface().(type) {
	case modelView:
		return pongo2.AsValue(v.label()), nil
	case fmt.Stringer:
		return pongo2.AsValue(v.String()), nil
	}
	return in, nil
}

// pyFloatText formats f as Django prints a float: Python's str(), the
// shortest form that reads back as f with ".0" on a whole number, but
// written out without an exponent, as Django's number formatting does.
// pongo2 prints six decimal places.
func pyFloatText(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if f != 0 && (math.Abs(f) < 1e-4 || math.Abs(f) >= 1e16) {
		// str() would use an exponent, which Django expands.
		return s
	}
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// filterLower implements Django's lower.
func filterLower(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return keep(in, strings.ToLower(text(in)))
}

// filterCapfirst implements Django's capfirst: the first character in upper
// case.
func filterCapfirst(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := text(in)
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return keep(in, s)
	}
	return keep(in, string(unicode.ToUpper(r))+s[size:])
}

// addslashesReplacer escapes backslashes and quotes like Django's addslashes.
var addslashesReplacer = strings.NewReplacer(`\`, `\\`, `"`, `\"`, `'`, `\'`)

// filterAddslashes implements Django's addslashes.
func filterAddslashes(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return keep(in, addslashesReplacer.Replace(text(in)))
}

// justify returns Django's ljust or rjust: the text padded with spaces to
// the given number of characters.
func justify(name string, left bool) pongo2.FilterFunction {
	return func(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		width, err := intArg(name, param)
		if err != nil {
			return nil, err
		}
		s := text(in)
		pad := width - utf8.RuneCountInString(s)
		if pad <= 0 {
			return keep(in, s)
		}
		if left {
			return keep(in, s+strings.Repeat(" ", pad))
		}
		return keep(in, strings.Repeat(" ", pad)+s)
	}
}

// filterCut implements Django's cut: every occurrence of the argument
// removed. Removing ";" can break an entity, so that result is not safe.
func filterCut(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	arg := param.String()
	out := strings.ReplaceAll(text(in), arg, "")
	if arg == ";" {
		return pongo2.AsValue(out), nil
	}
	return keep(in, out)
}

// phoneDigits maps the letters of a phone keypad to their digit.
var phoneDigits = strings.NewReplacer(
	"a", "2", "b", "2", "c", "2", "d", "3", "e", "3", "f", "3", "g", "4", "h", "4", "i", "4",
	"j", "5", "k", "5", "l", "5", "m", "6", "n", "6", "o", "6", "p", "7", "q", "7", "r", "7",
	"s", "7", "t", "8", "u", "8", "v", "8", "w", "9", "x", "9", "y", "9", "z", "9",
)

// filterPhone2numeric implements Django's phone2numeric: the letters of a
// phone number as keypad digits. Like Django, it lower-cases the rest.
func filterPhone2numeric(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return keep(in, phoneDigits.Replace(strings.ToLower(text(in))))
}

// sliceIndices returns the indices a Python slice [start:stop:step] selects
// from a sequence of n elements. A nil bound is an omitted one. ok is false
// for a step of zero, which Python rejects.
func sliceIndices(n int, start, stop, step *int) (out []int, ok bool) {
	st := 1
	if step != nil {
		st = *step
	}
	if st == 0 {
		return nil, false
	}
	bound := func(p *int, def, lo, hi int) int {
		if p == nil {
			return def
		}
		i := *p
		if i < 0 {
			i += n
		}
		return min(max(i, lo), hi)
	}
	if st > 0 {
		for i, end := bound(start, 0, 0, n), bound(stop, n, 0, n); i < end; i += st {
			out = append(out, i)
		}
		return out, true
	}
	for i, end := bound(start, n-1, -1, n-1), bound(stop, -1, -1, n-1); i > end; i += st {
		out = append(out, i)
	}
	return out, true
}

// sliceBounds reads the argument of slice, Python's slice syntax ("2",
// ":2", "1:-1", "::2"), into start, stop and step.
func sliceBounds(arg string) (bounds [3]*int, ok bool) {
	parts := strings.Split(arg, ":")
	if len(parts) > 3 {
		return bounds, false
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return bounds, false
		}
		bounds[i] = &n
	}
	if len(parts) == 1 {
		// slice(n) is slice(None, n).
		bounds[0], bounds[1] = nil, bounds[0]
	}
	return bounds, true
}

// filterSlice implements Django's slice on a string or a list. An argument
// that is not a slice gives the value back.
func filterSlice(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	bounds, ok := sliceBounds(param.String())
	if !ok {
		return in, nil
	}
	if in.IsString() {
		runes := []rune(in.String())
		idx, ok := sliceIndices(len(runes), bounds[0], bounds[1], bounds[2])
		if !ok {
			return in, nil
		}
		out := make([]rune, len(idx))
		for i, j := range idx {
			out[i] = runes[j]
		}
		return keep(in, string(out))
	}
	rv := reflect.ValueOf(in.Interface())
	if !isList(rv) {
		return in, nil
	}
	idx, ok := sliceIndices(rv.Len(), bounds[0], bounds[1], bounds[2])
	if !ok {
		return in, nil
	}
	out := reflect.MakeSlice(reflect.SliceOf(rv.Type().Elem()), len(idx), len(idx))
	for i, j := range idx {
		out.Index(i).Set(rv.Index(j))
	}
	return pongo2.AsValue(out.Interface()), nil
}

// element returns the i-th element of a string or a list, Python's
// value[i] with a negative i counting from the end; ok is false when there
// is none.
func element(in *pongo2.Value, i int) (*pongo2.Value, bool) {
	if in.IsString() {
		runes := []rune(in.String())
		if i < 0 {
			i += len(runes)
		}
		if i < 0 || i >= len(runes) {
			return nil, false
		}
		out, _ := keep(in, string(runes[i]))
		return out, true
	}
	rv := reflect.ValueOf(in.Interface())
	if !isList(rv) {
		return nil, false
	}
	if i < 0 {
		i += rv.Len()
	}
	if i < 0 || i >= rv.Len() {
		return nil, false
	}
	return itemValue(elem(rv, i)), true
}

// filterLast implements Django's last: the last element, or an empty
// string.
func filterLast(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if v, ok := element(in, -1); ok {
		return v, nil
	}
	return pongo2.AsValue(""), nil
}

// filterRandom implements Django's random: a random element, or an empty
// string for an empty sequence.
func filterRandom(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	n := in.Len()
	if n == 0 {
		return pongo2.AsValue(""), nil
	}
	if v, ok := element(in, rand.IntN(n)); ok {
		return v, nil
	}
	return pongo2.AsValue(""), nil
}

// linenumbersFilter returns Django's linenumbers: every line prefixed with
// its number, zero-padded to the width of the last number.
func linenumbersFilter(autoescape bool) pongo2.FilterFunction {
	return func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		esc := escaperFor(in, autoescape)
		lines := strings.Split(text(in), "\n")
		width := len(strconv.Itoa(len(lines)))
		for i, line := range lines {
			lines[i] = fmt.Sprintf("%0*d. %s", width, i+1, esc(line))
		}
		return safe(strings.Join(lines, "\n"))
	}
}

// filterEscapejs implements Django's escapejs: the characters that could end
// a JavaScript string or an HTML element, and the control characters, as
// \uXXXX escapes. pongo2's filter escapes every other punctuation too.
func filterEscapejs(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	var b strings.Builder
	for _, r := range text(in) {
		switch {
		case strings.ContainsRune("\\'\"><&=-;`\u2028\u2029", r),
			r < 0x20, r >= 0x7f && r < 0xa0:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	return pongo2.AsSafeValue(b.String()), nil
}
