package templates_test

import (
	"testing"
)

// TestWithTag ports Django's WithTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_with.py::WithTagTests.test_with01
// Ported from Django: tests/template_tests/syntax_tests/test_with.py::WithTagTests.test_legacywith01
// Ported from Django: tests/template_tests/syntax_tests/test_with.py::WithTagTests.test_with02
// Ported from Django: tests/template_tests/syntax_tests/test_with.py::WithTagTests.test_legacywith02
// Ported from Django: tests/template_tests/syntax_tests/test_with.py::WithTagTests.test_with03
// Ported from Django: tests/template_tests/syntax_tests/test_with.py::WithTagTests.test_with_error01
// Ported from Django: tests/template_tests/syntax_tests/test_with.py::WithTagTests.test_with_error02
func TestWithTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_with01", src: "{% with key=dict.key %}{{ key }}{% endwith %}", ctx: map[string]any{"dict": map[string]any{"key": 50}}, want: "50"},
		{name: "test_legacywith01", src: "{% with dict.key as key %}{{ key }}{% endwith %}", ctx: map[string]any{"dict": map[string]any{"key": 50}}, want: "50"},
		{name: "test_with02", src: "{{ key }}{% with key=dict.key %}{{ key }}-{{ dict.key }}-{{ key }}{% endwith %}{{ key }}", ctx: map[string]any{"dict": map[string]any{"key": 50}}, want: "50-50-50"},
		{name: "test_legacywith02", src: "{{ key }}{% with dict.key as key %}{{ key }}-{{ dict.key }}-{{ key }}{% endwith %}{{ key }}", ctx: map[string]any{"dict": map[string]any{"key": 50}}, want: "50-50-50"},
		{name: "test_with03", src: "{% with a=alpha b=beta %}{{ a }}{{ b }}{% endwith %}", ctx: map[string]any{"alpha": "A", "beta": "B"}, want: "AB"},
		{name: "test_with_error01", src: "{% with dict.key xx key %}{{ key }}{% endwith %}", ctx: map[string]any{"dict": map[string]any{"key": 50}}, wantErr: true},
		{name: "test_with_error02", src: "{% with dict.key as %}{{ key }}{% endwith %}", ctx: map[string]any{"dict": map[string]any{"key": 50}}, wantErr: true},
	})
}
