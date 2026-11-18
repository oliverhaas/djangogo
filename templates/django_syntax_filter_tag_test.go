package templates_test

import (
	"testing"
)

// TestFilterTag ports Django's FilterTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter01
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter02
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter03
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter04
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter05
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter05bis
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter06
// Ported from Django: tests/template_tests/syntax_tests/test_filter_tag.py::FilterTagTests.test_filter06bis
func TestFilterTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_filter01", src: "{% filter upper %}{% endfilter %}", want: ""},
		{name: "test_filter02", src: "{% filter upper %}django{% endfilter %}", want: "DJANGO"},
		{name: "test_filter03", src: "{% filter upper|lower %}django{% endfilter %}", want: "django"},
		{name: "test_filter04", src: "{% filter cut:remove %}djangospam{% endfilter %}", ctx: map[string]any{"remove": "spam"}, want: "django"},
		{name: "test_filter05", src: "{% filter safe %}fail{% endfilter %}", wantErr: true},
		{name: "test_filter05bis", src: "{% filter upper|safe %}fail{% endfilter %}", wantErr: true},
		{name: "test_filter06", src: "{% filter escape %}fail{% endfilter %}", wantErr: true},
		{name: "test_filter06bis", src: "{% filter upper|escape %}fail{% endfilter %}", wantErr: true},
	})
}
