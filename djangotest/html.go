package djangotest

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// The HTML comparison below is a port of django/test/html.py: two documents
// are parsed into normalized element trees and the trees are compared, so the
// result does not depend on whitespace between tags, attribute order, class
// order, the spelling of boolean attributes, or how a character is escaped.

// asciiWhitespace matches a run of ASCII whitespace, which the comparison
// treats as a single space.
var asciiWhitespace = regexp.MustCompile(`[\t\n\f\r ]+`)

// voidElements are the elements that never have content or an end tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
	// Deprecated tags.
	"frame": true, "spacer": true,
}

// booleanAttributes are the attributes whose presence alone carries the value,
// so <input checked> equals <input checked="checked">.
var booleanAttributes = map[string]bool{
	"allowfullscreen": true, "async": true, "autofocus": true, "autoplay": true,
	"checked": true, "controls": true, "default": true, "defer": true,
	"disabled": true, "formnovalidate": true, "hidden": true, "ismap": true,
	"itemscope": true, "loop": true, "multiple": true, "muted": true,
	"nomodule": true, "novalidate": true, "open": true, "playsinline": true,
	"readonly": true, "required": true, "reversed": true, "selected": true,
	// Attributes for deprecated tags.
	"truespeed": true,
}

// escapeText escapes text the way Python's html.escape does, so the rendering
// of a tree in a failure message reads as it does in Django.
var escapeText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;")

// htmlAttr is one normalized attribute. bare marks a boolean attribute, which
// has no value.
type htmlAttr struct {
	name  string
	value string
	bare  bool
}

// htmlNode is a node of the normalized tree: a text node (isText) or an
// element. The root node is the container of a document's top-level nodes.
type htmlNode struct {
	isText   bool
	text     string
	name     string
	attrs    []htmlAttr
	children []*htmlNode
	root     bool
}

// normalizeWhitespace collapses every run of ASCII whitespace to one space.
func normalizeWhitespace(s string) string {
	return asciiWhitespace.ReplaceAllString(s, " ")
}

// normalizeAttributes sorts the classes of a class attribute, drops the value
// of boolean attributes, and sorts the attributes by name.
func normalizeAttributes(attrs []html.Attribute) []htmlAttr {
	out := make([]htmlAttr, 0, len(attrs))
	for _, a := range attrs {
		na := htmlAttr{name: a.Key, value: a.Val}
		if na.name == "class" && na.value != "" {
			classes := strings.Fields(asciiWhitespace.ReplaceAllString(na.value, " "))
			sort.Strings(classes)
			na.value = strings.Join(classes, " ")
		}
		if booleanAttributes[na.name] && (na.value == "" || na.value == na.name) {
			na.value, na.bare = "", true
		}
		out = append(out, na)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].value < out[j].value
	})
	return out
}

// appendText adds text to n, merging it into a preceding text node.
func (n *htmlNode) appendText(text string) {
	text = normalizeWhitespace(text)
	if last := n.lastChild(); last != nil && last.isText {
		last.text = normalizeWhitespace(last.text + text)
		return
	}
	if text != "" {
		n.children = append(n.children, &htmlNode{isText: true, text: text})
	}
}

// appendElement adds a child element to n, first dropping a preceding text node
// that holds only whitespace.
func (n *htmlNode) appendElement(child *htmlNode) {
	if last := n.lastChild(); last != nil && last.isText && strings.TrimSpace(last.text) == "" {
		n.children = n.children[:len(n.children)-1]
	}
	n.children = append(n.children, child)
}

// lastChild returns the last child of n, or nil.
func (n *htmlNode) lastChild() *htmlNode {
	if len(n.children) == 0 {
		return nil
	}
	return n.children[len(n.children)-1]
}

// finalize strips the whitespace around text nodes and drops trailing text
// nodes that are empty, throughout the subtree.
func (n *htmlNode) finalize() {
	for {
		last := n.lastChild()
		if last == nil || !last.isText {
			break
		}
		last.text = strings.TrimRightFunc(last.text, unicode.IsSpace)
		if last.text != "" {
			break
		}
		n.children = n.children[:len(n.children)-1]
	}
	for _, child := range n.children {
		if child.isText {
			child.text = strings.TrimSpace(child.text)
			continue
		}
		child.finalize()
	}
}

// equal reports whether two nodes are the same text or the same element with
// equal attributes and equal children.
func (n *htmlNode) equal(other *htmlNode) bool {
	if n.isText || other.isText {
		return n.isText && other.isText && n.text == other.text
	}
	if n.name != other.name || len(n.attrs) != len(other.attrs) {
		return false
	}
	for i := range n.attrs {
		if n.attrs[i] != other.attrs[i] {
			return false
		}
	}
	return childrenEqual(n.children, other.children)
}

// childrenEqual reports whether two child lists are element-wise equal.
func childrenEqual(a, b []*htmlNode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].equal(b[i]) {
			return false
		}
	}
	return true
}

// count returns how many times needle occurs within n. A text needle is
// counted as a substring of text nodes; an element needle is counted where an
// equal element occurs; a root needle (several top-level nodes) is counted
// where its children occur as a run of siblings.
func (n *htmlNode) count(needle *htmlNode) int {
	if n.isText {
		return 0
	}
	if !needle.isText && !needle.root && n.equal(needle) {
		return 1
	}
	if needle.root && childrenEqual(n.children, needle.children) {
		return 1
	}
	total, matched := 0, 0
	for _, child := range n.children {
		if child.isText {
			if needle.isText {
				total += strings.Count(child.text, needle.text)
			}
			continue
		}
		total += child.count(needle)
		if !needle.root || len(needle.children) == 0 {
			continue
		}
		// Also look for the needle's children as a run among n's children, so
		// that "<a/><b/>" is found in "<a/><b/><c/>".
		if needle.children[matched].equal(child) {
			matched++
			if matched == len(needle.children) {
				total++
				matched = 0
			}
		} else {
			matched = 0
		}
	}
	return total
}

// String renders the normalized tree the way Django's Element.__str__ does, for
// failure messages: an element with children puts them on their own line, and
// sibling nodes follow each other without a separator.
func (n *htmlNode) String() string {
	if n.isText {
		return escapeText.Replace(n.text)
	}
	var inner strings.Builder
	for _, child := range n.children {
		inner.WriteString(child.String())
	}
	if n.root {
		return inner.String()
	}
	var b strings.Builder
	b.WriteString("<" + n.name)
	for _, a := range n.attrs {
		if a.bare {
			b.WriteString(" " + a.name)
			continue
		}
		b.WriteString(" " + a.name + `="` + a.value + `"`)
	}
	b.WriteString(">")
	if len(n.children) > 0 {
		b.WriteString("\n" + inner.String() + "\n</" + n.name + ">")
	}
	return b.String()
}

// parseHTML parses src into a normalized tree. Comments and the doctype are
// ignored and tags left open at the end are closed implicitly. An end tag with
// no matching start tag is an error, which includes the end tag of a void
// element such as </br>. When the document has exactly one top-level element,
// that element is returned instead of the root.
func parseHTML(src string) (*htmlNode, error) {
	root := &htmlNode{root: true}
	open := []*htmlNode{}
	current := func() *htmlNode {
		if len(open) > 0 {
			return open[len(open)-1]
		}
		return root
	}
	startTag := func(tok html.Token) {
		el := &htmlNode{name: tok.Data, attrs: normalizeAttributes(tok.Attr)}
		current().appendElement(el)
		if !voidElements[tok.Data] {
			open = append(open, el)
		}
	}
	// start is the offset in src of the token being handled.
	start := 0
	endTag := func(name string) error {
		for len(open) > 0 {
			el := open[len(open)-1]
			open = open[:len(open)-1]
			if el.name == name {
				return nil
			}
		}
		line, column := position(src, start)
		return fmt.Errorf("unexpected end tag `%s` (line %d, column %d)", name, line, column)
	}

	z := html.NewTokenizer(strings.NewReader(src))
	next := 0
	for {
		tokenType := z.Next()
		start, next = next, next+len(z.Raw())
		switch tokenType {
		case html.ErrorToken:
			if err := z.Err(); !errors.Is(err, io.EOF) {
				return nil, err
			}
			root.finalize()
			if len(root.children) == 1 && !root.children[0].isText {
				return root.children[0], nil
			}
			return root, nil
		case html.TextToken:
			current().appendText(z.Token().Data)
		case html.StartTagToken:
			startTag(z.Token())
		case html.SelfClosingTagToken:
			tok := z.Token()
			startTag(tok)
			if !voidElements[tok.Data] {
				if err := endTag(tok.Data); err != nil {
					return nil, err
				}
			}
		case html.EndTagToken:
			if err := endTag(z.Token().Data); err != nil {
				return nil, err
			}
		case html.CommentToken, html.DoctypeToken:
			// Ignored, as Django's parser ignores them.
		}
	}
}

// position returns the line (counted from 1) and the column (counted from 0,
// in characters) of the byte offset in src, the position Python's HTMLParser
// reports.
func position(src string, offset int) (line, column int) {
	before := src[:min(offset, len(src))]
	lineStart := strings.LastIndexByte(before, '\n') + 1
	return strings.Count(before, "\n") + 1, utf8.RuneCountInString(before[lineStart:])
}

// lineDiff compares a and b line by line in the style of Python's
// difflib.ndiff, which Django appends to the failure of assertHTMLEqual: "- "
// marks a line that is only in a, "+ " a line that is only in b, and two
// spaces a line they share.
func lineDiff(a, b string) string {
	x, y := splitLines(a), splitLines(b)
	// common[i][j] is the length of the longest common subsequence of x[i:]
	// and y[j:].
	common := make([][]int, len(x)+1)
	for i := range common {
		common[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				common[i][j] = common[i+1][j+1] + 1
			} else {
				common[i][j] = max(common[i+1][j], common[i][j+1])
			}
		}
	}
	out := make([]string, 0, len(x)+len(y))
	i, j := 0, 0
	for i < len(x) && j < len(y) {
		switch {
		case x[i] == y[j]:
			out = append(out, "  "+x[i])
			i, j = i+1, j+1
		case common[i+1][j] >= common[i][j+1]:
			out = append(out, "- "+x[i])
			i++
		default:
			out = append(out, "+ "+y[j])
			j++
		}
	}
	for ; i < len(x); i++ {
		out = append(out, "- "+x[i])
	}
	for ; j < len(y); j++ {
		out = append(out, "+ "+y[j])
	}
	return strings.Join(out, "\n")
}

// splitLines splits s into lines; an empty string has none.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// AssertHTMLEqual asserts that two HTML fragments are equivalent (Django's
// assertHTMLEqual). The comparison is semantic: whitespace before and after
// tags is ignored and all whitespace is equivalent, attribute order and class
// order do not matter, <br> equals <br />, a boolean attribute equals its
// name-valued form (checked and checked="checked"), and character references
// equal the characters they refer to.
func AssertHTMLEqual(t TB, got, want string) {
	t.Helper()
	gotTree, wantTree, ok := parseHTMLPair(t, got, want)
	if !ok {
		return
	}
	if !gotTree.equal(wantTree) {
		first, second := gotTree.String(), wantTree.String()
		t.Errorf("%s != %s\n%s", first, second, lineDiff(first, second))
	}
}

// AssertHTMLNotEqual asserts that two HTML fragments are not equivalent under
// the rules of AssertHTMLEqual (Django's assertHTMLNotEqual).
func AssertHTMLNotEqual(t TB, got, want string) {
	t.Helper()
	gotTree, wantTree, ok := parseHTMLPair(t, got, want)
	if !ok {
		return
	}
	if gotTree.equal(wantTree) {
		t.Errorf("%s == %s", gotTree, wantTree)
	}
}

// AssertInHTML asserts that the HTML fragment needle occurs in haystack
// (Django's assertInHTML), comparing as AssertHTMLEqual does. needle must occur
// at least once, or exactly Count times when that option is given. MsgPrefix
// is the only other option that applies.
func AssertInHTML(t TB, needle, haystack string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(0, opts)
	needleTree, haystackTree, ok := parseHTMLPair(t, needle, haystack)
	if !ok {
		return
	}
	n := haystackTree.count(needleTree)
	switch {
	case c.hasCount && n != c.count && c.count == 0:
		t.Errorf("%s'%s' unexpectedly found in the following response\n'%s'", c.msgPrefix, needle, haystack)
	case c.hasCount && n != c.count:
		t.Errorf("%sFound %d instances of '%s' (expected %d) in the following response\n'%s'", c.msgPrefix, n, needle, c.count, haystack)
	case !c.hasCount && n == 0:
		t.Errorf("%sCouldn't find '%s' in the following response\n'%s'", c.msgPrefix, needle, haystack)
	}
}

// AssertNotInHTML asserts that the HTML fragment needle does not occur in
// haystack (Django's assertNotInHTML). MsgPrefix is the only option that
// applies.
func AssertNotInHTML(t TB, needle, haystack string, opts ...AssertOption) {
	t.Helper()
	AssertInHTML(t, needle, haystack, append(opts, Count(0))...)
}

// parseHTMLPair parses both fragments, reporting a parse failure on t.
func parseHTMLPair(t TB, first, second string) (a, b *htmlNode, ok bool) {
	t.Helper()
	a, err := parseHTML(first)
	if err != nil {
		t.Errorf("First argument is not valid HTML:\n%v", err)
		return nil, nil, false
	}
	b, err = parseHTML(second)
	if err != nil {
		t.Errorf("Second argument is not valid HTML:\n%v", err)
		return nil, nil, false
	}
	return a, b, true
}
