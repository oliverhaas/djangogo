package djangotest

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The XML comparison below is a port of compare_xml in django/test/utils.py:
// both documents are parsed into element trees, and the trees are compared
// without regard to attribute order, comments, processing instructions or
// the document type.

// xmlWhitespaceRun matches a run of two or more spaces, tabs or newlines,
// which compare_xml folds into one space.
var xmlWhitespaceRun = regexp.MustCompile(`[ \t\n][ \t\n]+`)

// xmlElement is an element of a parsed XML document.
type xmlElement struct {
	// tag is the qualified tag name, with its namespace prefix.
	tag string
	// attrs maps each qualified attribute name to its value.
	attrs map[string]string
	// text is the concatenated text of the direct text children.
	text string
	// children are the child elements in document order.
	children []*xmlElement
}

// equal reports whether e and o match as compare_xml's check_element does.
func (e *xmlElement) equal(o *xmlElement) bool {
	if e.tag != o.tag ||
		xmlWhitespaceRun.ReplaceAllString(e.text, " ") != xmlWhitespaceRun.ReplaceAllString(o.text, " ") ||
		!maps.Equal(e.attrs, o.attrs) ||
		len(e.children) != len(o.children) {
		return false
	}
	for i, child := range e.children {
		if !child.equal(o.children[i]) {
			return false
		}
	}
	return true
}

// AssertXMLEqual asserts that the XML snippets xml1 and xml2 are semantically
// the same (Django's assertXMLEqual). Attribute order, comments, processing
// instructions, the document type and whitespace around the snippets do not
// matter, and a run of whitespace in text compares equal to one space. A
// snippet without an XML declaration can hold several elements. Both
// arguments must be valid XML. The failure message is Django's, except that
// the line diff has no "?" hint lines. CDATA sections count as text, where
// Django's DOM keeps them apart.
func AssertXMLEqual(t TB, xml1, xml2 string) {
	t.Helper()
	equal, err := compareXML(xml1, xml2)
	if err != nil {
		t.Errorf("First or second argument is not valid XML\n%v", err)
		return
	}
	if !equal {
		t.Errorf("%s != %s\n%s", safeRepr(xml1), safeRepr(xml2), lineDiff(xml1, xml2))
	}
}

// AssertXMLNotEqual asserts that the XML snippets xml1 and xml2 are not
// semantically the same under the rules of AssertXMLEqual (Django's
// assertXMLNotEqual).
func AssertXMLNotEqual(t TB, xml1, xml2 string) {
	t.Helper()
	equal, err := compareXML(xml1, xml2)
	if err != nil {
		t.Errorf("First or second argument is not valid XML\n%v", err)
		return
	}
	if equal {
		t.Errorf("%s == %s", safeRepr(xml1), safeRepr(xml2))
	}
}

// safeRepr returns the Python repr of s, cut after 80 characters as
// unittest's safe_repr(s, short=True) does.
func safeRepr(s string) string {
	const maxLength = 80
	r := pyRepr(s)
	if utf8.RuneCountInString(r) < maxLength {
		return r
	}
	return string([]rune(r)[:maxLength]) + " [truncated]..."
}

// compareXML reports whether the XML snippets want and got are semantically
// the same, porting Django's compare_xml. It returns an error when either
// snippet is not valid XML.
func compareXML(want, got string) (bool, error) {
	want = strings.ReplaceAll(strings.TrimSpace(want), `\n`, "\n")
	got = strings.ReplaceAll(strings.TrimSpace(got), `\n`, "\n")
	if !strings.HasPrefix(want, "<?xml") {
		want = "<root>" + want + "</root>"
		got = "<root>" + got + "</root>"
	}
	wantRoot, err := parseXMLDocument(want)
	if err != nil {
		return false, err
	}
	gotRoot, err := parseXMLDocument(got)
	if err != nil {
		return false, err
	}
	return wantRoot.equal(gotRoot), nil
}

// parseXMLDocument parses the XML document s and returns its document
// element.
func parseXMLDocument(s string) (*xmlElement, error) {
	dec := xml.NewDecoder(strings.NewReader(s))
	var root *xmlElement
	var open []*xmlElement
	for {
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if root != nil && len(open) == 0 {
				return nil, errors.New("junk after document element")
			}
			el := &xmlElement{tag: xmlQualifiedName(tok.Name), attrs: make(map[string]string, len(tok.Attr))}
			for _, a := range tok.Attr {
				el.attrs[xmlQualifiedName(a.Name)] = a.Value
			}
			if root == nil {
				root = el
			} else {
				parent := open[len(open)-1]
				parent.children = append(parent.children, el)
			}
			open = append(open, el)
		case xml.EndElement:
			name := xmlQualifiedName(tok.Name)
			if len(open) == 0 || open[len(open)-1].tag != name {
				return nil, fmt.Errorf("mismatched tag </%s>", name)
			}
			open = open[:len(open)-1]
		case xml.CharData:
			if len(open) > 0 {
				open[len(open)-1].text += string(tok)
			} else if strings.TrimSpace(string(tok)) != "" {
				return nil, errors.New("text outside the document element")
			}
		}
	}
	if root == nil || len(open) > 0 {
		return nil, errors.New("no element found")
	}
	return root, nil
}

// xmlQualifiedName returns name with its namespace prefix, as the DOM's
// tagName and attribute names hold it.
func xmlQualifiedName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}
