package djangotest

import (
	"sort"
	"testing"
)

// The tests in this file are ports of the tests in Django's
// tests/test_utils/tests.py that look at the tree parse_html returns, which is
// not exported here. The assertion-level tests of the same class are in
// django_test_utils_test.go.

// mustParse is parse_html(src).
func mustParse(t *testing.T, src string) *htmlNode {
	t.Helper()
	node, err := parseHTML(src)
	if err != nil {
		t.Fatalf("parseHTML(%q): %v", src, err)
	}
	return node
}

// textNeedle is the plain string a Django test passes where it passes an
// element elsewhere, as in dom.count("foo").
func textNeedle(text string) *htmlNode {
	return &htmlNode{isText: true, text: text}
}

// wantText checks that the node is the text node holding want.
func wantText(t *testing.T, node *htmlNode, want string) {
	t.Helper()
	if !node.isText || node.text != want {
		t.Errorf("node is %q (text: %v), want the text %q", node.String(), node.isText, want)
	}
}

// wantChildren checks the number of children of the node.
func wantChildren(t *testing.T, node *htmlNode, want int) {
	t.Helper()
	if len(node.children) != want {
		t.Fatalf("%q has %d children, want %d", node.String(), len(node.children), want)
	}
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_html_parser
func TestHTMLParser(t *testing.T) {
	element := mustParse(t, "<div><p>Hello</p></div>")
	wantChildren(t, element, 1)
	if element.children[0].name != "p" {
		t.Errorf("child is <%s>, want <p>", element.children[0].name)
	}
	wantText(t, element.children[0].children[0], "Hello")

	mustParse(t, "<p>")
	mustParse(t, "<p attr>")
	dom := mustParse(t, "<p>foo")
	wantChildren(t, dom, 1)
	if dom.name != "p" {
		t.Errorf("element is <%s>, want <p>", dom.name)
	}
	wantText(t, dom.children[0], "foo")
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_parse_html_in_script
func TestParseHTMLInScript(t *testing.T) {
	mustParse(t, `<script>var a = "<p" + ">";</script>`)
	mustParse(t, `
            <script>
            var js_sha_link='<p>***</p>';
            </script>
        `)

	// Script content will be parsed to text.
	dom := mustParse(t, `
            <script><p>foo</p> '</scr'+'ipt>' <span>bar</span></script>
        `)
	wantChildren(t, dom, 1)
	wantText(t, dom.children[0], "<p>foo</p> '</scr'+'ipt>' <span>bar</span>")
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_void_elements
func TestVoidElements(t *testing.T) {
	tags := make([]string, 0, len(voidElements))
	for tag := range voidElements {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		t.Run(tag, func(t *testing.T) {
			for _, src := range []string{
				"<p>Hello <" + tag + "> world</p>",
				"<p>Hello <" + tag + " /> world</p>",
			} {
				dom := mustParse(t, src)
				wantChildren(t, dom, 3)
				wantText(t, dom.children[0], "Hello")
				if dom.children[1].name != tag {
					t.Errorf("%s: second child is <%s>, want <%s>", src, dom.children[1].name, tag)
				}
				wantText(t, dom.children[2], "world")
			}
		})
	}
}

// contains is the port of Element.__contains__, the "in" of the Django tests.
func contains(haystack, needle *htmlNode) bool {
	return haystack.count(needle) > 0
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_html_contain
func TestHTMLContain(t *testing.T) {
	// Equal html contains each other.
	dom1 := mustParse(t, "<p>foo")
	dom2 := mustParse(t, "<p>foo</p>")
	if !contains(dom2, dom1) || !contains(dom1, dom2) {
		t.Error("equal HTML does not contain each other")
	}

	dom2 = mustParse(t, "<div><p>foo</p></div>")
	if !contains(dom2, dom1) {
		t.Error("<p>foo is not in <div><p>foo</p></div>")
	}
	if contains(dom1, dom2) {
		t.Error("<div><p>foo</p></div> is in <p>foo")
	}

	if contains(dom2, textNeedle("<p>foo</p>")) {
		t.Error(`the text "<p>foo</p>" is in <div><p>foo</p></div>`)
	}
	if !contains(dom2, textNeedle("foo")) {
		t.Error(`the text "foo" is not in <div><p>foo</p></div>`)
	}

	// When a root element is used ...
	dom1 = mustParse(t, "<p>foo</p><p>bar</p>")
	dom2 = mustParse(t, "<p>foo</p><p>bar</p>")
	if !contains(dom2, dom1) {
		t.Error("<p>foo</p><p>bar</p> is not in itself")
	}
	dom1 = mustParse(t, "<p>foo</p>")
	if !contains(dom2, dom1) {
		t.Error("<p>foo</p> is not in <p>foo</p><p>bar</p>")
	}
	dom1 = mustParse(t, "<p>bar</p>")
	if !contains(dom2, dom1) {
		t.Error("<p>bar</p> is not in <p>foo</p><p>bar</p>")
	}
	dom1 = mustParse(t, "<div><p>foo</p><p>bar</p></div>")
	if !contains(dom1, dom2) {
		t.Error("<p>foo</p><p>bar</p> is not in <div><p>foo</p><p>bar</p></div>")
	}
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_count
func TestCount(t *testing.T) {
	check := func(haystack string, needle *htmlNode, want int) {
		t.Helper()
		if got := mustParse(t, haystack).count(needle); got != want {
			t.Errorf("%q holds %q %d time(s), want %d", haystack, needle.String(), got, want)
		}
	}

	// Equal html contains each other one time.
	dom1 := mustParse(t, "<p>foo")
	check("<p>foo", mustParse(t, "<p>foo</p>"), 1)
	check("<p>foo</p>", dom1, 1)

	check("<p>foo</p><p>bar</p>", dom1, 1)

	check("<p>foo foo</p><p>foo</p>", textNeedle("foo"), 3)

	check(`<p class="bar">foo</p>`, textNeedle("bar"), 0)
	check(`<p class="bar">foo</p>`, textNeedle("class"), 0)
	check(`<p class="bar">foo</p>`, textNeedle("p"), 0)
	check(`<p class="bar">foo</p>`, textNeedle("o"), 2)

	check("<p>foo</p><p>foo</p>", dom1, 2)

	check(`<div><p>foo<input type=""></p><p>foo</p></div>`, dom1, 1)

	check("<div><div><p>foo</p></div></div>", dom1, 1)

	check("<p>foo<p>foo</p></p>", dom1, 1)

	check("<p>foo<p>bar</p></p>", dom1, 0)

	// HTML with a root element contains the same HTML with no root element.
	dom1 = mustParse(t, "<p>foo</p><p>bar</p>")
	check("<div><p>foo</p><p>bar</p></div>", dom1, 1)

	// Target of search is a sequence of child elements and appears more than
	// once.
	check("<div><p>foo</p><p>bar</p><p>foo</p><p>bar</p></div>", dom1, 2)

	// Searched HTML has additional children.
	check("<a/><b/><c/>", mustParse(t, "<a/><b/>"), 1)

	// No match found in children.
	check("<a/><b/><c/>", mustParse(t, "<b/><a/>"), 0)

	// Target of search found among children and grandchildren.
	check("<a><b/><b/></a><b/><b/>", mustParse(t, "<b/><b/>"), 2)
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_root_element_escaped_html
func TestRootElementEscapedHTML(t *testing.T) {
	html := "&lt;br&gt;"
	if got := mustParse(t, html).String(); got != html {
		t.Errorf("rendered %q, want %q", got, html)
	}
}

// TestBooleanAttributeString holds the str(parse_html(...)) checks of the two
// Django tests; their assertion checks are in django_test_utils_test.go.
//
// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_boolean_attribute
// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_non_boolean_attibutes
func TestBooleanAttributeString(t *testing.T) {
	cases := []struct{ src, want string }{
		{"<input checked>", "<input checked>"},
		{`<input checked="">`, "<input checked>"},
		{`<input checked="checked">`, "<input checked>"},
		{"<input value>", `<input value="">`},
		{`<input value="">`, `<input value="">`},
	}
	for _, tc := range cases {
		if got := mustParse(t, tc.src).String(); got != tc.want {
			t.Errorf("%s rendered as %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestParseUnexpectedEndTag is the parse_html("</p>") check of the Django
// test, which expects HTMLParseError.
//
// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_parsing_errors
func TestParseUnexpectedEndTag(t *testing.T) {
	_, err := parseHTML("</p>")
	if err == nil {
		t.Fatal("parsing </p> succeeded, want an error")
	}
	if want := "unexpected end tag `p` (line 1, column 0)"; err.Error() != want {
		t.Errorf("error is %q, want %q", err, want)
	}
}

// TestParsePosition is not a Django test: it checks the line and column of a
// parse error past the first line, counted in characters.
func TestParsePosition(t *testing.T) {
	_, err := parseHTML("<p>ŠĐ</p>\n<b>é</b></i>")
	if err == nil {
		t.Fatal("parsing succeeded, want an error")
	}
	if want := "unexpected end tag `i` (line 2, column 8)"; err.Error() != want {
		t.Errorf("error is %q, want %q", err, want)
	}
}

// TestLineDiff is not a Django test: it checks the comparison AssertHTMLEqual
// appends to its failure message.
func TestLineDiff(t *testing.T) {
	got := lineDiff("<p>\na\nb\n</p>", "<p>\nb\nc\n</p>")
	want := "  <p>\n- a\n  b\n+ c\n  </p>"
	if got != want {
		t.Errorf("lineDiff returned\n%s\nwant\n%s", got, want)
	}
	if got := lineDiff("", "x"); got != "+ x" {
		t.Errorf(`lineDiff("", "x") returned %q, want "+ x"`, got)
	}
}
