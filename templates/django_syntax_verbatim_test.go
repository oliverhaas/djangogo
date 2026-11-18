package templates_test

import (
	"testing"
)

// TestVerbatimTag ports Django's VerbatimTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_verbatim_tag01
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_verbatim_tag02
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_verbatim_tag03
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_verbatim_tag04
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_verbatim_tag05
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_verbatim_tag06
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_literal_multiline_opener_in_named_verbatim
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_template_syntax_in_raw_blocks
// Ported from Django: tests/template_tests/syntax_tests/test_verbatim.py::VerbatimTagTests.test_literal_multiline_block_opener_in_raw_blocks
func TestVerbatimTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_verbatim_tag01", src: "{% verbatim %}{{bare   }}{% endverbatim %}", want: "{{bare   }}"},
		{name: "test_verbatim_tag02", src: "{% verbatim %}{% endif %}{% endverbatim %}", want: "{% endif %}"},
		{name: "test_verbatim_tag03", src: "{% verbatim %}It's the {% verbatim %} tag{% endverbatim %}", want: "It's the {% verbatim %} tag"},
		{name: "test_verbatim_tag04", src: "{% verbatim %}{% verbatim %}{% endverbatim %}{% endverbatim %}", wantErr: true},
		{name: "test_verbatim_tag05", src: "{% verbatim %}{% endverbatim %}{% verbatim %}{% endverbatim %}", want: ""},
		{name: "test_verbatim_tag06", src: "{% verbatim special %}Don't {% endverbatim %} just yet{% endverbatim special %}", want: "Don't {% endverbatim %} just yet"},
		{name: "test_literal_multiline_opener_in_named_verbatim", src: "{% verbatim example %}\n{%\n{% endverbatim %}\n{% endverbatim example %}", want: "\n{%\n{% endverbatim %}\n"},
		{name: "test_template_syntax_in_raw_blocks", src: "{% verbatim %}\n{% if user %}Hello, {{ user }}{% endif %}\n{% endverbatim %}", want: "\n{% if user %}Hello, {{ user }}{% endif %}\n"},
		{name: "test_literal_multiline_block_opener_in_raw_blocks", src: "{% verbatim %}\n{%\n{% endverbatim %}", want: "\n{%\n"},
	})
}
