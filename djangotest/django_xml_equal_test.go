package djangotest_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
)

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_equal
// Check that two identical elements are equal.
func TestXMLEqualSimpleEqual(t *testing.T) {
	djangotest.AssertXMLEqual(t, "<elem attr1='a' attr2='b' />", "<elem attr1='a' attr2='b' />")
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_equal_unordered
// Check that the order of attributes does not matter.
func TestXMLEqualSimpleEqualUnordered(t *testing.T) {
	djangotest.AssertXMLEqual(t, "<elem attr1='a' attr2='b' />", "<elem attr2='b' attr1='a' />")
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_equal_raise
// Check that an extra attribute makes AssertXMLEqual fail.
func TestXMLEqualSimpleEqualRaise(t *testing.T) {
	mustFail(t, "", func(ft djangotest.TB) {
		djangotest.AssertXMLEqual(ft, "<elem attr1='a' />", "<elem attr2='b' attr1='a' />")
	})
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_equal_raises_message
// Check the failure message of AssertXMLEqual. Python's line diff adds a "?"
// hint line that the Go message leaves out, so the test compares the part
// before it.
func TestXMLEqualSimpleEqualRaisesMessage(t *testing.T) {
	xml1 := "<elem attr1='a' />"
	xml2 := "<elem attr2='b' attr1='a' />"
	msg := `"<elem attr1='a' />" != "<elem attr2='b' attr1='a' />"` + "\n" +
		"- <elem attr1='a' />\n" +
		"+ <elem attr2='b' attr1='a' />"
	mustFail(t, msg, func(ft djangotest.TB) { djangotest.AssertXMLEqual(ft, xml1, xml2) })
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_not_equal
// Check that a different attribute value makes the elements not equal.
func TestXMLEqualSimpleNotEqual(t *testing.T) {
	djangotest.AssertXMLNotEqual(t, "<elem attr1='a' attr2='c' />", "<elem attr1='a' attr2='b' />")
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_not_equal_raise
// Check that AssertXMLNotEqual fails for elements that differ only in attribute order.
func TestXMLEqualSimpleNotEqualRaise(t *testing.T) {
	mustFail(t, "", func(ft djangotest.TB) {
		djangotest.AssertXMLNotEqual(ft, "<elem attr1='a' attr2='b' />", "<elem attr2='b' attr1='a' />")
	})
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_parsing_errors
// Check that invalid XML makes the assertion fail.
func TestXMLEqualParsingErrors(t *testing.T) {
	mustFail(t, "First or second argument is not valid XML", func(ft djangotest.TB) {
		djangotest.AssertXMLNotEqual(ft, "<elem attr1='a attr2='b' />", "<elem attr2='b' attr1='a' />")
	})
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_comment_root
// Check that comments before the document element are ignored.
func TestXMLEqualCommentRoot(t *testing.T) {
	djangotest.AssertXMLEqual(t,
		"<?xml version='1.0'?><!-- comment1 --><elem attr1='a' attr2='b' />",
		"<?xml version='1.0'?><!-- comment2 --><elem attr2='b' attr1='a' />",
	)
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_equal_with_leading_or_trailing_whitespace
// Check that whitespace around the snippets is ignored.
func TestXMLEqualSimpleEqualWithLeadingOrTrailingWhitespace(t *testing.T) {
	djangotest.AssertXMLEqual(t, "<elem>foo</elem> \t\n", " \t\n<elem>foo</elem>")
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_simple_not_equal_with_whitespace_in_the_middle
// Check that whitespace between elements of a fragment counts as text.
func TestXMLEqualSimpleNotEqualWithWhitespaceInTheMiddle(t *testing.T) {
	djangotest.AssertXMLNotEqual(t, "<elem>foo</elem><elem>bar</elem>", "<elem>foo</elem> <elem>bar</elem>")
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_doctype_root
// Check that the document type declaration is ignored.
func TestXMLEqualDoctypeRoot(t *testing.T) {
	djangotest.AssertXMLEqual(t,
		`<?xml version="1.0"?><!DOCTYPE root SYSTEM "example1.dtd"><root />`,
		`<?xml version="1.0"?><!DOCTYPE root SYSTEM "example2.dtd"><root />`,
	)
}

// Ported from Django: tests/test_utils/tests.py::XMLEqualTests.test_processing_instruction
// Check that processing instructions are ignored.
func TestXMLEqualProcessingInstruction(t *testing.T) {
	djangotest.AssertXMLEqual(t,
		`<?xml version="1.0"?><?xml-model href="http://www.example1.com"?><root />`,
		`<?xml version="1.0"?><?xml-model href="http://www.example2.com"?><root />`,
	)
	djangotest.AssertXMLEqual(t,
		`<?xml-stylesheet href="style1.xslt" type="text/xsl"?><root />`,
		`<?xml-stylesheet href="style2.xslt" type="text/xsl"?><root />`,
	)
}
