package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestPhone2numeric ports Django's Phone2numericTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_phone2numeric.py::Phone2numericTests.test_phone2numeric01
// Ported from Django: tests/template_tests/filter_tests/test_phone2numeric.py::Phone2numericTests.test_phone2numeric02
// Ported from Django: tests/template_tests/filter_tests/test_phone2numeric.py::Phone2numericTests.test_phone2numeric03
func TestPhone2numeric(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_phone2numeric01", src: "{{ a|phone2numeric }} {{ b|phone2numeric }}", ctx: map[string]any{"a": "<1-800-call-me>", "b": templates.MarkSafe("<1-800-call-me>")}, want: "&lt;1-800-2255-63&gt; <1-800-2255-63>"},
		{name: "test_phone2numeric02", src: "{% autoescape off %}{{ a|phone2numeric }} {{ b|phone2numeric }}{% endautoescape %}", ctx: map[string]any{"a": "<1-800-call-me>", "b": templates.MarkSafe("<1-800-call-me>")}, want: "<1-800-2255-63> <1-800-2255-63>"},
		{name: "test_phone2numeric03", src: "{{ a|phone2numeric }}", ctx: map[string]any{"a": "How razorback-jumping frogs can level six piqued gymnasts!"}, want: "469 729672225-5867464 37647 226 53835 749 747833 49662787!"},
	})
}

// TestPhone2numericFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_phone2numeric.py::FunctionTests.test_phone2numeric
func TestPhone2numericFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_phone2numeric", src: "{{ a|phone2numeric }}", ctx: map[string]any{"a": "0800 flowers"}, want: "0800 3569377"},
	})
}
