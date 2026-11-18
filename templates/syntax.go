package templates

import (
	"bytes"
	"regexp"
	"strings"
)

// tagRe finds the variable and block tags of a template source.
var tagRe = regexp.MustCompile(`\{\{.*?\}\}|\{%.*?%\}`)

// blockSuperRe finds Django's spelling of the parent block's content.
var blockSuperRe = regexp.MustCompile(`\bblock\.super\b`)

// dictViewRe finds a {% for %} tag that iterates over the items, keys or
// values of a variable, see dictViewFilter.
var dictViewRe = regexp.MustCompile(`^(.*\sin\s+)([A-Za-z_][\w.]*)\.(items|keys|values)(\s.*|-?%\})$`)

// djangoSyntax rewrites the Django spellings pongo2 cannot be taught through
// its tag and filter registry:
//
//   - {{ block.super }}, which pongo2 only knows as {{ block.Super }};
//   - the conditions of {% if %} and {% elif %}, see rewriteCondition;
//   - a loop over mapping.items, .keys or .values, see dictViewFilter;
//   - a backslash in a string literal, see keepBackslashes;
//   - a string literal given to default, default_if_none or join, see
//     trustLiterals.
//
// Text outside tags and everything between {% verbatim %} and
// {% endverbatim %} is left alone. A rewrite keeps the line of every tag, so
// the lines in error messages stay right; a column after a rewritten condition
// may shift by a few characters.
func djangoSyntax(src []byte) []byte {
	if bytes.IndexByte(src, '{') < 0 {
		return src
	}
	var out []byte
	last, changed, verbatim := 0, false, false
	for _, loc := range tagRe.FindAllIndex(src, -1) {
		tag := src[loc[0]:loc[1]]
		_, name, _, _ := tagParts(tag)
		if verbatim {
			verbatim = name != "endverbatim"
			continue
		}
		if name == "verbatim" {
			verbatim = true
			continue
		}
		repl := tag
		if bytes.IndexByte(repl, '\\') >= 0 {
			repl = quotedRe.ReplaceAllFunc(repl, keepBackslashes)
		}
		if bytes.Contains(repl, []byte("default")) || bytes.Contains(repl, []byte("join")) {
			repl = trustLiterals(repl)
		}
		switch {
		case name == "if" || name == "elif":
			head, _, args, tail := tagParts(repl)
			if cond, ok := rewriteCondition(splitBits(args)); ok {
				repl = []byte(head + " " + cond + " " + tail)
			}
		case name == "for":
			repl = dictViewRe.ReplaceAll(repl, []byte("$1$2.$3|_dict_$3:$2$4"))
		case bytes.Contains(repl, []byte("block.super")):
			repl = blockSuperRe.ReplaceAll(repl, []byte("block.Super"))
		}
		if bytes.Equal(repl, tag) {
			continue
		}
		out = append(out, src[last:loc[0]]...)
		out = append(out, repl...)
		last, changed = loc[1], true
	}
	if !changed {
		return src
	}
	return append(out, src[last:]...)
}

// tagParts splits a {% %} tag into the text up to the end of its name, the
// name, the arguments and the closing delimiter. A {{ }} tag has no name.
func tagParts(tag []byte) (head, name, args, tail string) {
	s := string(tag)
	if !strings.HasPrefix(s, "{%") {
		return s, "", "", ""
	}
	start, end := 2, len(s)-2
	// "{%-" and "-%}" trim the white space around a tag.
	if start < end && s[start] == '-' {
		start++
	}
	if end > start && s[end-1] == '-' {
		end--
	}
	body := strings.TrimLeft(s[start:end], " \t")
	nameLen := strings.IndexAny(body, " \t")
	if nameLen < 0 {
		nameLen = len(body)
	}
	headLen := end - len(body) + nameLen
	return s[:headLen], body[:nameLen], s[headLen:end], s[end:]
}

// splitBits splits the arguments of a tag at white space and keeps quoted
// strings whole, as Django's Token.split_contents does.
func splitBits(s string) []string {
	var bits []string
	start := -1
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == ' ' || c == '\t':
			if start >= 0 {
				bits = append(bits, s[start:i])
				start = -1
			}
		default:
			if c == '"' || c == '\'' {
				quote = c
			}
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		bits = append(bits, s[start:])
	}
	return bits
}

// quotedRe finds the string literals of a tag.
var quotedRe = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)

// literalArgRe finds, at the end of the text in front of a string literal, a
// filter that prints its argument.
var literalArgRe = regexp.MustCompile(`\|\s*(default|default_if_none|join)\s*:\s*$`)

// literalPrefix marks the variant of a filter whose argument is a string
// literal of the template.
const literalPrefix = "_literal_"

// trustLiterals renames the filters default, default_if_none and join where
// their argument is a string literal: {{ items|join:"<br>" }} becomes
// {{ items|_literal_join:"<br>" }}. Django prints a string the template
// author wrote without escaping it and escapes one that comes from a
// variable. A pongo2 filter cannot tell the two apart, so the source decides
// which variant runs.
func trustLiterals(tag []byte) []byte {
	var out []byte
	last := 0
	for _, loc := range quotedRe.FindAllIndex(tag, -1) {
		before := tag[last:loc[0]]
		if m := literalArgRe.FindSubmatchIndex(before); m != nil {
			out = append(out, before[:m[2]]...)
			out = append(out, literalPrefix...)
			before = before[m[2]:]
		}
		out = append(out, before...)
		out = append(out, tag[loc[0]:loc[1]]...)
		last = loc[1]
	}
	if last == 0 {
		return tag
	}
	return append(out, tag[last:]...)
}

// keepBackslashes doubles the backslashes of a string literal that stand in
// front of an ordinary character. In Django such a backslash is part of the
// string, which is how a date format quotes a letter ("jS \o\f F"); pongo2
// rejects the literal. A backslash before a quote or another backslash is an
// escape in both and stays.
func keepBackslashes(lit []byte) []byte {
	out := make([]byte, 0, len(lit)+4)
	for i := 0; i < len(lit); i++ {
		c := lit[i]
		out = append(out, c)
		if c != '\\' || i+1 == len(lit) {
			continue
		}
		switch next := lit[i+1]; next {
		case '"', '\'', '\\':
			out = append(out, next)
			i++
		default:
			out = append(out, '\\')
		}
	}
	return out
}

// conditionWords are the words of a condition that are not operands.
var conditionWords = map[string]bool{
	"and": true, "or": true, "not": true, "in": true, "is": true,
	"==": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
}

// rewriteCondition gives the condition of an {% if %} tag, written in
// Django's grammar, the meaning it has in Django. pongo2 reads the same words
// differently: "not" binds tighter than a comparison ("not a == b" is
// "(not a) == b"), "and" and "or" have one precedence ("a and b or c" is
// "a and (b or c)"), and "not in", "is" and "is not" are unknown. The result
// spells the grouping out with parentheses, which pongo2 understands.
//
// ok is false when nothing has to change or when the condition is not in
// Django's grammar (it has parentheses, "&&", "||" or "!"), which leaves a
// condition written for pongo2 as it is.
func rewriteCondition(bits []string) (cond string, ok bool) {
	for _, bit := range bits {
		bare := quotedRe.ReplaceAllString(bit, "")
		if strings.ContainsAny(bare, "()") || bare == "&&" || bare == "||" ||
			(strings.HasPrefix(bare, "!") && bare != "!=") {
			return "", false
		}
	}
	groups := [][]string{nil}
	for i := 0; ; i++ {
		clause, next, parsed := rewriteClause(bits, i)
		if !parsed {
			return "", false
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], clause)
		i = next
		if i == len(bits) {
			break
		}
		switch bits[i] {
		case "and":
		case "or":
			groups = append(groups, nil)
		default:
			return "", false
		}
	}
	ors := make([]string, len(groups))
	for i, group := range groups {
		ors[i] = strings.Join(group, " and ")
		if len(group) > 1 && len(groups) > 1 {
			ors[i] = "(" + ors[i] + ")"
		}
	}
	cond = strings.Join(ors, " or ")
	return cond, cond != strings.Join(bits, " ")
}

// rewriteClause rewrites the clause starting at bits[i]: an operand, or a
// comparison of two operands, with any number of "not" in front. next is the
// index after the clause.
func rewriteClause(bits []string, i int) (clause string, next int, ok bool) {
	negate := false
	for i < len(bits) && bits[i] == "not" {
		negate = !negate
		i++
	}
	if i == len(bits) || conditionWords[bits[i]] {
		return "", 0, false
	}
	left := bits[i]
	i++
	op := ""
	if i < len(bits) {
		following := ""
		if i+1 < len(bits) {
			following = bits[i+1]
		}
		switch bits[i] {
		case "not":
			if following != "in" {
				return "", 0, false
			}
			negate, op = !negate, "in"
			i += 2
		case "is":
			op = "=="
			i++
			if following == "not" {
				op = "!="
				i++
			}
		case "==", "!=", "<", ">", "<=", ">=", "in":
			op = bits[i]
			i++
		}
	}
	if op == "" {
		if negate {
			return "not " + left, i, true
		}
		return left, i, true
	}
	if i == len(bits) || conditionWords[bits[i]] {
		return "", 0, false
	}
	clause = left + " " + op + " " + bits[i]
	if negate {
		clause = "not (" + clause + ")"
	}
	return clause, i + 1, true
}
