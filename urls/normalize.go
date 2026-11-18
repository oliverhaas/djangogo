package urls

import (
	"strconv"
	"strings"
)

// This file ports django.utils.regex_helper.normalize, the routine that turns
// the regular expression of a route into the text forms Reverse fills in. It
// is deliberately the same linear scan, quirks included, so that every
// expression reverses to what it reverses to in Django.

// revPart is one piece of a reverse form: literal text, or a reference to the
// argument named ref.
type revPart struct {
	lit string
	ref string
}

// revForm is one way to write the URL of a route: the parts in order and the
// names of the arguments they need. An optional group yields two forms, one
// with and one without it.
type revForm struct {
	parts  []revPart
	params []string
}

// appendLit adds literal text, merging it into a preceding literal.
func (f *revForm) appendLit(s string) {
	if s == "" {
		return
	}
	if n := len(f.parts); n > 0 && f.parts[n-1].ref == "" {
		f.parts[n-1].lit += s
		return
	}
	f.parts = append(f.parts, revPart{lit: s})
}

// concat returns f followed by g, sharing no storage with either.
func (f revForm) concat(g revForm) revForm {
	out := revForm{
		parts:  append([]revPart(nil), f.parts...),
		params: append(append([]string(nil), f.params...), g.params...),
	}
	for _, p := range g.parts {
		if p.ref == "" {
			out.appendLit(p.lit)
			continue
		}
		out.parts = append(out.parts, p)
	}
	return out
}

// normItem is an element of the intermediate result of normalize: a literal
// (string), a capturing group (normGroup), a non-capturing group
// (normNonCapture), or an optional element (normChoice).
type normItem any

// normGroup is a capturing group. ref is the name its value is looked up
// under; param is false for a named backreference, which repeats a value
// without adding an argument.
type normGroup struct {
	ref   string
	param bool
}

// normNonCapture is a non-capturing group, kept together so that a quantifier
// can apply to all of it.
type normNonCapture []normItem

// normChoice lists alternatives; nil stands for "nothing".
type normChoice []normItem

// escapeMappings maps an escaped character to a representative of its class;
// the empty string means the escape is dropped. It is Django's
// ESCAPE_MAPPINGS.
var escapeMappings = map[rune]string{
	'A': "", 'b': "", 'B': "", 'Z': "",
	'd': "0", 'D': "x", 's': " ", 'S': "x", 'w': "x", 'W': "!",
}

// normScanner yields the characters of an expression with escapes resolved,
// Django's next_char generator.
type normScanner struct {
	src []rune
	pos int
}

// next returns the next character, whether it was escaped, and false at the
// end of the expression.
func (s *normScanner) next() (ch string, escaped, ok bool) {
	for s.pos < len(s.src) {
		r := s.src[s.pos]
		s.pos++
		if r != '\\' {
			return string(r), false, true
		}
		if s.pos >= len(s.src) {
			return "", false, false
		}
		r = s.src[s.pos]
		s.pos++
		rep, mapped := escapeMappings[r]
		if !mapped {
			return string(r), true, true
		}
		if rep == "" {
			continue
		}
		return rep, true, true
	}
	return "", false, false
}

// walkToEnd skips to the close of the capturing group the scanner is in,
// Django's walk_to_end.
func (s *normScanner) walkToEnd(ch string) {
	nesting := 0
	if ch == "(" {
		nesting = 1
	}
	for {
		c, escaped, ok := s.next()
		if !ok {
			return
		}
		if escaped {
			continue
		}
		switch c {
		case "(":
			nesting++
		case ")":
			if nesting == 0 {
				return
			}
			nesting--
		}
	}
}

// quantifier parses a quantifier that starts with ch and returns the minimum
// repeat count, and the character after the quantifier when one had to be
// read to find its end. ok is false when the expression ends inside a {m,n}
// quantifier. It is Django's get_quantifier.
func (s *normScanner) quantifier(ch string) (count int, next string, hasNext, ok bool) {
	if ch == "*" || ch == "?" || ch == "+" {
		c, _, more := s.next()
		hasNext = more && c != "?"
		if !hasNext {
			c = ""
		}
		if ch == "+" {
			return 1, c, hasNext, true
		}
		return 0, c, hasNext, true
	}
	var quant []string
	for ch != "}" {
		c, _, more := s.next()
		if !more {
			return 0, "", false, false
		}
		ch = c
		quant = append(quant, c)
	}
	quant = quant[:len(quant)-1]
	first, _, _ := strings.Cut(strings.Join(quant, ""), ",")
	count, _ = strconv.Atoi(first)
	c, _, more := s.next()
	hasNext = more && c != "?"
	if !hasNext {
		c = ""
	}
	return count, c, hasNext, true
}

// normalize returns the reverse forms of a regular expression. See Django's
// normalize for the rules: repeats keep their minimum count, optional groups
// with arguments yield a form with and a form without them, a character class
// is replaced by its first character, and alternation makes the expression
// irreversible (a single empty form).
func normalize(pattern string) []revForm {
	s := &normScanner{src: []rune(pattern)}
	var result []normItem
	var nonCapturing []int
	numArgs := 0

	ch, escaped, ok := s.next()
	if !ok {
		return []revForm{{}}
	}
	for ok {
		consumeNext := true
		switch {
		case escaped:
			result = append(result, ch)
		case ch == ".":
			result = append(result, ".")
		case ch == "|":
			return []revForm{{}}
		case ch == "^":
		case ch == "$":
			return flattenTop(result)
		case ch == ")":
			if len(nonCapturing) == 0 {
				break
			}
			start := nonCapturing[len(nonCapturing)-1]
			nonCapturing = nonCapturing[:len(nonCapturing)-1]
			inner := normNonCapture(append([]normItem(nil), result[start:]...))
			result = append(result[:start:start], inner)
		case ch == "[":
			if ch, _, ok = s.next(); !ok {
				return flattenTop(result)
			}
			result = append(result, ch)
			if ch, escaped, ok = s.next(); !ok {
				return flattenTop(result)
			}
			for escaped || ch != "]" {
				if ch, escaped, ok = s.next(); !ok {
					return flattenTop(result)
				}
			}
		case ch == "(":
			var done bool
			if result, numArgs, nonCapturing, done = s.group(result, numArgs, nonCapturing); done {
				return flattenTop(result)
			}
		case ch == "*" || ch == "?" || ch == "+" || ch == "{":
			count, next, hasNext, complete := s.quantifier(ch)
			if !complete {
				return flattenTop(result)
			}
			if hasNext {
				ch, escaped = next, false
				consumeNext = false
			}
			result = applyQuantifier(result, count)
		default:
			result = append(result, ch)
		}
		if consumeNext {
			ch, escaped, ok = s.next()
		}
	}
	return flattenTop(result)
}

// group handles an opening parenthesis: a positional group, a named group, a
// non-capturing group, or an assertion that is skipped. done reports that the
// expression ended inside the group.
func (s *normScanner) group(result []normItem, numArgs int, nonCapturing []int) ([]normItem, int, []int, bool) {
	ch, escaped, ok := s.next()
	if !ok {
		return result, numArgs, nonCapturing, true
	}
	if ch != "?" || escaped {
		name := "_" + strconv.Itoa(numArgs)
		numArgs++
		result = append(result, normGroup{ref: name, param: true})
		s.walkToEnd(ch)
		return result, numArgs, nonCapturing, false
	}
	if ch, _, ok = s.next(); !ok {
		return result, numArgs, nonCapturing, true
	}
	switch ch {
	case "!", "=":
		s.walkToEnd(ch)
	case ":":
		nonCapturing = append(nonCapturing, len(result))
	case "P", "<":
		// (?P<name>...) is Python's named group and (?P=name) its named
		// backreference. (?<name>...) is the spelling Go also accepts; Python
		// reads (?< as a lookbehind, which Go cannot compile anyway.
		terminal := ">"
		if ch == "P" {
			if ch, _, ok = s.next(); !ok {
				return result, numArgs, nonCapturing, true
			}
			if ch == "=" {
				terminal = ")"
			}
		}
		var name strings.Builder
		for {
			if ch, _, ok = s.next(); !ok {
				return result, numArgs, nonCapturing, true
			}
			if ch == terminal {
				break
			}
			name.WriteString(ch)
		}
		if terminal == ")" {
			result = append(result, normGroup{ref: name.String()})
		} else {
			result = append(result, normGroup{ref: name.String(), param: true})
			s.walkToEnd(ch)
		}
	default:
		// An inline flag group such as (?i) or (?i:...). Django raises
		// ValueError for these; reading past the flags costs nothing, because
		// Reverse checks every candidate against the expression anyway.
		for ch != ")" && ch != ":" {
			if ch, _, ok = s.next(); !ok {
				return result, numArgs, nonCapturing, true
			}
		}
		if ch == ":" {
			nonCapturing = append(nonCapturing, len(result))
		}
	}
	return result, numArgs, nonCapturing, false
}

// applyQuantifier applies a minimum repeat count to the last item of result.
func applyQuantifier(result []normItem, count int) []normItem {
	if len(result) == 0 {
		return result
	}
	last := result[len(result)-1]
	switch {
	case count == 0:
		if containsGroup(last) {
			result[len(result)-1] = normChoice{nil, last}
		} else {
			result = result[:len(result)-1]
		}
	case count > 1:
		for i := 1; i < count; i++ {
			result = append(result, last)
		}
	}
	return result
}

// containsGroup reports whether item is a capturing group or a non-capturing
// group that holds one.
func containsGroup(item normItem) bool {
	switch x := item.(type) {
	case normGroup:
		return true
	case normNonCapture:
		for _, e := range x {
			if containsGroup(e) {
				return true
			}
		}
	}
	return false
}

// flattenTop flattens the top-level result list.
func flattenTop(result []normItem) []revForm {
	return flattenResult(normNonCapture(result))
}

// flattenResult expands the nested result into the list of reverse forms,
// Django's flatten_result.
func flattenResult(source normItem) []revForm {
	switch x := source.(type) {
	case nil:
		return []revForm{{}}
	case normGroup:
		f := revForm{parts: []revPart{{ref: x.ref}}}
		if x.param {
			f.params = []string{x.ref}
		}
		return []revForm{f}
	case normNonCapture:
		return flattenList(x)
	}
	return []revForm{{}}
}

// flattenList is the list branch of flattenResult.
func flattenList(source []normItem) []revForm {
	result := []revForm{{}}
	var piece strings.Builder
	flush := func() {
		if piece.Len() == 0 {
			return
		}
		for i := range result {
			result[i].appendLit(piece.String())
		}
		piece.Reset()
	}
	for _, elt := range source {
		switch x := elt.(type) {
		case string:
			piece.WriteString(x)
		case normGroup:
			flush()
			for i := range result {
				result[i].parts = append(result[i].parts, revPart{ref: x.ref})
				if x.param {
					result[i].params = append(result[i].params, x.ref)
				}
			}
		case normChoice, normNonCapture:
			flush()
			var inner []revForm
			if choice, isChoice := x.(normChoice); isChoice {
				for _, item := range choice {
					inner = append(inner, flattenResult(item)...)
				}
			} else {
				inner = flattenResult(x)
			}
			crossed := make([]revForm, 0, len(result)*len(inner))
			for _, r := range result {
				for _, in := range inner {
					crossed = append(crossed, r.concat(in))
				}
			}
			result = crossed
		}
	}
	flush()
	return result
}
