package templates_test

import (
	"testing"
)

// TestTemplateTag ports Django's TemplateTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag01
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag02
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag03
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag04
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag05
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag06
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag07
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag08
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag09
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag10
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag11
// Ported from Django: tests/template_tests/syntax_tests/test_template_tag.py::TemplateTagTests.test_templatetag12
func TestTemplateTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_templatetag01", src: "{% templatetag openblock %}", want: "{%"},
		{name: "test_templatetag02", src: "{% templatetag closeblock %}", want: "%}"},
		{name: "test_templatetag03", src: "{% templatetag openvariable %}", want: "{{"},
		{name: "test_templatetag04", src: "{% templatetag closevariable %}", want: "}}"},
		{name: "test_templatetag05", src: "{% templatetag %}", wantErr: true},
		{name: "test_templatetag06", src: "{% templatetag foo %}", wantErr: true},
		{name: "test_templatetag07", src: "{% templatetag openbrace %}", want: "{"},
		{name: "test_templatetag08", src: "{% templatetag closebrace %}", want: "}"},
		{name: "test_templatetag09", src: "{% templatetag openbrace %}{% templatetag openbrace %}", want: "{{"},
		{name: "test_templatetag10", src: "{% templatetag closebrace %}{% templatetag closebrace %}", want: "}}"},
		{name: "test_templatetag11", src: "{% templatetag opencomment %}", want: "{#"},
		{name: "test_templatetag12", src: "{% templatetag closecomment %}", want: "#}"},
	})
}
