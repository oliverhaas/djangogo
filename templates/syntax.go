package templates

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
)

// tagRe finds the tags of a template source like the lexer of Django
// (tag_re in django/template/base.py): a tag ends on the line it starts on,
// so a "{{" without "}}" on the same line is text.
var tagRe = regexp.MustCompile(`\{%.*?%\}|\{\{.*?\}\}|\{#.*?#\}`)

// blockSuperRe finds Django's spelling of the parent block's content.
var blockSuperRe = regexp.MustCompile(`\bblock\.super\b`)

// dictViewRe finds a {% for %} tag that iterates over the items, keys or
// values of a variable, see dictViewFilter.
var dictViewRe = regexp.MustCompile(`^(.*\sin\s+)([A-Za-z_][\w.]*)\.(items|keys|values)(\s.*|-?%\})$`)

// translatedRe finds a string literal marked for translation, _("text").
// There is no translation catalog, so the literal stands for itself.
var translatedRe = regexp.MustCompile(`\b_\(\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')\s*\)`)

// outFilter is the filter djangoSyntax appends to every {{ }} tag, see
// filterOut.
const outFilter = "_out"

// djangoSyntax rewrites a Django template source into one pongo2 renders the
// way Django does. It covers what pongo2 cannot be taught through its tag
// and filter registry:
//
//   - text that Django's lexer leaves alone but pongo2 would read as a tag
//     (a "{{" without "}}" on its line, the content of {% verbatim %}), see
//     literalText;
//   - {# #} comments and {% comment %} blocks, which are removed with
//     everything in them;
//   - {{ block.super }}, which pongo2 only knows as {{ block.Super }};
//   - the conditions of {% if %} and {% elif %}, see rewriteCondition;
//   - a loop over mapping.items, .keys or .values, see dictViewFilter;
//   - the key of {% regroup %}, see regroupSyntax;
//   - the partial names of {% partialdef %} and {% partial %}, see
//     partialSyntax;
//   - {% extends %}, see extendsSyntax, and the names of {% block %}, which
//     Django checks;
//   - a backslash in a string literal, see keepBackslashes;
//   - string literals, which Django never escapes, see trustLiterals and
//     safeLiteral;
//   - the filters that escape their input themselves, which take the
//     autoescape setting of the {% autoescape %} block they are in, see
//     noEscapeFilters;
//   - the final filter of every {{ }} tag, see filterOut.
//
// name is the template name, "" for a template given as a string. A rewrite
// keeps the line of every tag but {% extends %}, so the lines in error
// messages stay right; a column can shift by a few characters.
func djangoSyntax(src []byte, name string) []byte { return rewrite(src, name, nil) }

// rewrite is djangoSyntax with extendsTo, which decides the parent of the
// {% extends %} tag. It gets the parent the tag names, with literal set for
// a string literal, and returns the parent to extend instead with ok set.
// An empty parent removes the tag.
func rewrite(src []byte, name string, extendsTo func(parent string, literal bool) (to string, ok bool)) []byte {
	if bytes.IndexByte(src, '{') < 0 {
		return src
	}
	r := rewriter{
		autoescape: []bool{true},
		out:        make([]byte, 0, len(src)+len(src)/8),
		name:       name,
		extendsTo:  extendsTo,
	}
	last := 0
	for _, loc := range tagRe.FindAllIndex(src, -1) {
		r.addText(src[last:loc[0]])
		r.token(src[loc[0]:loc[1]])
		last = loc[1]
	}
	r.addText(src[last:])
	r.finish()
	return r.out
}

// rewriter holds the state of djangoSyntax while it walks a source.
type rewriter struct {
	out []byte
	// text is the text that has not been written yet, see literalText.
	text []byte
	// autoescape is the stack of {% autoescape %} settings, innermost last.
	autoescape []bool
	// verbatim is, inside a {% verbatim %} block, the content of the tag
	// that ends it ("endverbatim" or "endverbatim name").
	verbatim string
	// commentTag is, inside a {% comment %} block, the tag that opened it.
	commentTag []byte
	// name and extendsTo are the arguments of rewrite.
	name      string
	extendsTo func(parent string, literal bool) (string, bool)
	// sawTag is set after a tag other than {# #}. {% extends %} must come
	// before such a tag.
	sawTag bool
	// extended is set after the {% extends %} tag.
	extended bool
	// extendsTag is the {% extends %} tag with a string literal. finish puts
	// it at the end: pongo2 loads the parent when it parses the tag, and a
	// syntax error in the rest must come first, as in Django.
	extendsTag []byte
	// extendsVar is set when a {% extends %} with a variable wraps the rest
	// of the template, see extendsVarTag.
	extendsVar bool
	// blocks holds the names of the {% block %} tags.
	blocks map[string]bool
}

// addText adds text of the template. Inside a comment only its line breaks
// are kept.
func (r *rewriter) addText(text []byte) {
	if r.commentTag != nil {
		r.text = append(r.text, bytes.Repeat([]byte{'\n'}, bytes.Count(text, []byte{'\n'}))...)
		return
	}
	r.text = append(r.text, text...)
}

// emit writes the pending text and then b.
func (r *rewriter) emit(b []byte) {
	r.out = append(r.out, literalText(r.text)...)
	r.text = r.text[:0]
	r.out = append(r.out, b...)
}

// finish writes what is left. An unclosed {% comment %} or {% verbatim %}
// is put back, so that pongo2 reports it as unclosed.
func (r *rewriter) finish() {
	switch {
	case r.commentTag != nil:
		r.emit(r.commentTag)
	case r.verbatim != "":
		r.emit([]byte("{% verbatim %}"))
	default:
		r.emit(nil)
	}
	if r.extendsVar {
		r.out = append(r.out, "{% end"+extendsVarTag+" %}"...)
	}
	r.out = append(r.out, r.extendsTag...)
}

// token handles one tag, as Django's lexer and parser see it.
func (r *rewriter) token(tag []byte) {
	kind := tag[1]
	content := strings.TrimSpace(string(tag[2 : len(tag)-2]))
	if r.verbatim != "" {
		if kind == '%' && content == r.verbatim {
			r.verbatim = ""
		} else {
			r.addText(tag)
		}
		return
	}
	if kind == '%' && (content == "verbatim" || strings.HasPrefix(content, "verbatim ")) {
		r.verbatim = "end" + content
		r.sawTag = true
		return
	}
	if r.commentTag != nil {
		if kind == '%' && content == "endcomment" {
			r.commentTag = nil
			if len(r.text) > 0 {
				// The line breaks of the comment go into a pongo2
				// comment, which keeps the line numbers of the tags
				// after it without printing them.
				r.out = append(r.out, "{% comment %}"...)
				r.out = append(r.out, r.text...)
				r.out = append(r.out, "{% endcomment %}"...)
				r.text = r.text[:0]
			}
		}
		return
	}
	switch kind {
	case '#':
	case '{':
		r.emit(r.variable(tag))
		r.sawTag = true
	default:
		r.emit(r.block(tag))
	}
}

// variable rewrites a {{ }} tag.
func (r *rewriter) variable(tag []byte) []byte {
	inner := bytes.TrimRight(tag[2:len(tag)-2], " \t")
	if len(bytes.TrimSpace(inner)) == 0 {
		return tag
	}
	inner = safeLiteral(r.expression(inner))
	out := make([]byte, 0, len(inner)+len(outFilter)+8)
	out = append(out, "{{"...)
	out = append(out, inner...)
	out = append(out, "|"+outFilter+" }}"...)
	return out
}

// block rewrites a {% %} tag.
func (r *rewriter) block(tag []byte) []byte {
	_, name, args, _ := tagParts(tag)
	if name == "extends" {
		return r.extendsSyntax(tag)
	}
	r.sawTag = true
	switch name {
	case "comment":
		r.commentTag = tag
		return nil
	case "partialdef", "endpartialdef", "partial":
		return partialSyntax(tag)
	case "block":
		if bits := splitBits(args); len(bits) == 1 {
			if r.blocks[bits[0]] {
				return syntaxError("'block' tag with name '" + bits[0] + "' appears more than once")
			}
			if r.blocks == nil {
				r.blocks = map[string]bool{}
			}
			r.blocks[bits[0]] = true
		}
	case "autoescape":
		switch strings.TrimSpace(args) {
		case "on":
			r.autoescape = append(r.autoescape, true)
		case "off":
			r.autoescape = append(r.autoescape, false)
		}
		return tag
	case "endautoescape":
		if len(r.autoescape) > 1 {
			r.autoescape = r.autoescape[:len(r.autoescape)-1]
		}
		return tag
	case "filter":
		// The first filter of the tag has no "|" in front.
		for _, f := range filterNameRe.FindAllStringSubmatch("|"+args, -1) {
			if f[2] == "escape" || f[2] == "safe" {
				return syntaxError(`"filter ` + f[2] + `" is not permitted.  Use the "autoescape" tag instead.`)
			}
		}
		if !r.autoescape[len(r.autoescape)-1] {
			head, _, _, tail := tagParts(tag)
			tag = []byte(head + string(noEscapeFilters([]byte("|" + args))[1:]) + tail)
		}
	}
	repl := r.expression(tag)
	switch name {
	case "if", "elif":
		head, _, args, tail := tagParts(repl)
		if cond, ok := rewriteCondition(splitBits(args)); ok {
			repl = []byte(head + " " + cond + " " + tail)
		}
	case "for":
		repl = dictViewRe.ReplaceAll(repl, []byte("$1$2.$3|_dict_$3:$2$4"))
	case "with", "include":
		repl = safeAssignments(repl)
	case "regroup":
		repl = regroupSyntax(repl)
	}
	return repl
}

// extendsSyntax checks a {% extends %} tag as Django does: one argument,
// once in a template and before any other tag. A tag with a string literal
// goes to the end of the source, see rewriter.extendsTag, and one with a
// variable wraps the rest of the template, see extendsVarTag.
func (r *rewriter) extendsSyntax(tag []byte) []byte {
	head, _, args, tail := tagParts(tag)
	bits := splitBits(args)
	switch {
	case len(bits) != 1:
		return syntaxError("'extends' takes one argument")
	case r.sawTag:
		origin := "the template"
		if r.name != "" {
			origin = pyRepr(r.name)
		}
		contents := strings.TrimSpace(string(tag[2 : len(tag)-2]))
		return syntaxError("{% " + contents + " %} must be the first tag in " + origin + ".")
	case r.extended:
		return syntaxError("'extends' cannot appear more than once in the same template")
	}
	r.extended = true
	parent, literal := bits[0], isLiteral(bits[0])
	if literal {
		parent = parent[1 : len(parent)-1]
	}
	if r.extendsTo != nil {
		if to, ok := r.extendsTo(parent, literal); ok {
			if to != "" {
				r.extendsTag = []byte(head + " " + quoteName(to) + " " + tail)
			}
			return nil
		}
	}
	if literal {
		r.extendsTag = r.expression(tag)
		return nil
	}
	r.extendsVar = true
	return []byte("{% " + extendsVarTag + " " + quoteName(bits[0]) + " " + string(r.expression([]byte(bits[0]))) + " %}")
}

// partialSyntax checks the arguments of {% partialdef %}, {% endpartialdef %}
// and {% partial %} as Django does, and quotes the partial name for pongo2.
// A name is any word, such as "card-1", "{curly}" or "'two words'" with its
// quotes.
func partialSyntax(tag []byte) []byte {
	head, name, args, tail := tagParts(tag)
	bits := splitBits(args)
	switch name {
	case "partialdef":
		switch {
		case len(bits) == 0:
			return syntaxError("'partialdef' tag requires a name")
		case len(bits) > 2:
			return syntaxError("'partialdef' tag takes at most 2 arguments")
		case len(bits) == 2 && bits[1] != "inline":
			return syntaxError("The 'inline' argument does not have any parameters; " +
				"either use 'inline' or remove it completely.")
		}
		bits[0] = quoteName(bits[0])
	case "partial":
		if len(bits) != 1 {
			return syntaxError("'partial' tag requires a single argument")
		}
		bits[0] = quoteName(bits[0])
	default:
		// Django compares everything after "endpartialdef" with the name.
		if len(bits) > 0 {
			bits = []string{quoteName(strings.Join(bits, " "))}
		}
	}
	return []byte(head + " " + strings.Join(bits, " ") + " " + tail)
}

// regroupSyntax checks the arguments of a {% regroup %} tag as Django does
// and prefixes the key with the variable name: Django evaluates
// "name.key" with each element of the list bound to name.
func regroupSyntax(tag []byte) []byte {
	head, _, args, tail := tagParts(tag)
	bits := splitBits(args)
	switch {
	case len(bits) != 5:
		return syntaxError("'regroup' tag takes five arguments")
	case bits[1] != "by":
		return syntaxError("second argument to 'regroup' tag must be 'by'")
	case bits[3] != "as":
		return syntaxError("next-to-last argument to 'regroup' tag must be 'as'")
	}
	bits[2] = bits[4] + "." + bits[2]
	return []byte(head + " " + strings.Join(bits, " ") + " " + tail)
}

// assignedLiteralRe matches a {% with %} or {% include ... with %} argument
// that assigns a string literal.
var assignedLiteralRe = regexp.MustCompile(`^\w+=("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')$`)

// safeAssignments marks the string literals a {% with %} or {% include %}
// tag assigns as safe, so that {% with x="<b>" %}{{ x }} prints them as
// written, like Django.
func safeAssignments(tag []byte) []byte {
	head, name, args, tail := tagParts(tag)
	bits := splitBits(args)
	changed := false
	for i, bit := range bits {
		// {% with "text" as name %} is the older form.
		legacy := name == "with" && i == 0 && len(bits) == 3 && bits[1] == "as" && isLiteral(bit)
		if legacy || assignedLiteralRe.MatchString(bit) {
			bits[i] = bit + "|" + safeLiteralFilter
			changed = true
		}
	}
	if !changed {
		return tag
	}
	return []byte(head + " " + strings.Join(bits, " ") + " " + tail)
}

// expression applies the rewrites that hold for every expression of a tag.
func (r *rewriter) expression(b []byte) []byte {
	if bytes.IndexByte(b, '\\') >= 0 {
		b = quotedRe.ReplaceAllFunc(b, keepBackslashes)
	}
	if bytes.Contains(b, []byte("_(")) {
		b = translatedRe.ReplaceAll(b, []byte("$1"))
	}
	if bytes.Contains(b, []byte("default")) || bytes.Contains(b, []byte("join")) {
		b = trustLiterals(b)
	}
	if bytes.Contains(b, []byte("block.super")) {
		b = blockSuperRe.ReplaceAll(b, []byte("block.Super"))
	}
	if !r.autoescape[len(r.autoescape)-1] {
		b = noEscapeFilters(b)
	}
	return b
}

// syntaxError returns a tag that fails to compile with msg, for a template
// Django rejects but pongo2 would accept.
func syntaxError(msg string) []byte {
	return []byte("{% " + syntaxErrorTag + " " + strconv.Quote(msg) + " %}")
}

// literalText returns text that pongo2 prints as it is. Text with "{{",
// "{%" or "{#" in it, or that ends with "{" in front of a tag, is wrapped in
// pongo2's {% verbatim %}, which ends at the first "{% endverbatim %}"; an
// "{% endverbatim %}" in the text is put together from {% templatetag %}.
func literalText(text []byte) []byte {
	if !bytes.Contains(text, []byte("{{")) && !bytes.Contains(text, []byte("{%")) &&
		!bytes.Contains(text, []byte("{#")) && !bytes.HasSuffix(text, []byte("{")) {
		return text
	}
	const end = "{% endverbatim %}"
	var out []byte
	for i, part := range bytes.Split(text, []byte(end)) {
		if i > 0 {
			out = append(out, "{% templatetag openblock %} endverbatim {% templatetag closeblock %}"...)
		}
		if len(part) > 0 {
			out = append(out, "{% verbatim %}"...)
			out = append(out, part...)
			out = append(out, end...)
		}
	}
	return out
}

// safeLiteral marks a string literal that starts a {{ }} expression as safe:
// Django prints {{ "<b>" }} as written.
func safeLiteral(inner []byte) []byte {
	trimmed := bytes.TrimLeft(inner, " \t")
	loc := quotedRe.FindIndex(trimmed)
	if loc == nil || loc[0] != 0 {
		return inner
	}
	rest := bytes.TrimLeft(trimmed[loc[1]:], " \t")
	if len(rest) > 0 && rest[0] != '|' {
		return inner
	}
	at := len(inner) - len(trimmed) + loc[1]
	out := make([]byte, 0, len(inner)+len(safeLiteralFilter)+1)
	out = append(out, inner[:at]...)
	out = append(out, "|"+safeLiteralFilter...)
	return append(out, inner[at:]...)
}

// filterNameRe finds the filters of an expression: a "|", then the name.
var filterNameRe = regexp.MustCompile(`\|(\s*)(\w+)`)

// noEscapeFilters renames, outside string literals, the filters that escape
// their input themselves (Django's needs_autoescape filters) to the variant
// for {% autoescape off %}, see autoescapeFilter.
func noEscapeFilters(b []byte) []byte {
	var out []byte
	last := 0
	rename := func(seg []byte) []byte {
		return filterNameRe.ReplaceAllFunc(seg, func(m []byte) []byte {
			sub := filterNameRe.FindSubmatch(m)
			name := strings.TrimPrefix(string(sub[2]), literalPrefix)
			if !needsAutoescape[name] {
				return m
			}
			return []byte("|" + string(sub[1]) + noEscapePrefix + name)
		})
	}
	for _, loc := range quotedRe.FindAllIndex(b, -1) {
		out = append(out, rename(b[last:loc[0]])...)
		out = append(out, b[loc[0]:loc[1]]...)
		last = loc[1]
	}
	return append(out, rename(b[last:])...)
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

// isLiteral reports whether s is one string literal.
func isLiteral(s string) bool {
	loc := quotedRe.FindStringIndex(s)
	return loc != nil && loc[0] == 0 && loc[1] == len(s)
}

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
