package templates_test

import (
	"testing"
)

// TestResetCycleTag ports Django's ResetCycleTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle01
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle02
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle03
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle04
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle05
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle06
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle07
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle08
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle09
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle10
// Ported from Django: tests/template_tests/syntax_tests/test_resetcycle.py::ResetCycleTagTests.test_resetcycle11
func TestResetCycleTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_resetcycle01", src: "{% resetcycle %}", wantErr: true},
		{name: "test_resetcycle02", src: "{% resetcycle undefinedcycle %}", wantErr: true},
		{name: "test_resetcycle03", src: "{% cycle 'a' 'b' %}{% resetcycle undefinedcycle %}", wantErr: true},
		{name: "test_resetcycle04", src: "{% cycle 'a' 'b' as ab %}{% resetcycle undefinedcycle %}", wantErr: true},
		{name: "test_resetcycle05", src: "{% for i in test %}{% cycle 'a' 'b' %}{% resetcycle %}{% endfor %}", ctx: map[string]any{"test": []any{0, 1, 2, 3, 4}}, want: "aaaaa"},
		{name: "test_resetcycle06", src: "{% cycle 'a' 'b' 'c' as abc %}{% for i in test %}{% cycle abc %}{% cycle '-' '+' %}{% resetcycle %}{% endfor %}", ctx: map[string]any{"test": []any{0, 1, 2, 3, 4}}, want: "ab-c-a-b-c-"},
		{name: "test_resetcycle07", src: "{% cycle 'a' 'b' 'c' as abc %}{% for i in test %}{% resetcycle abc %}{% cycle abc %}{% cycle '-' '+' %}{% endfor %}", ctx: map[string]any{"test": []any{0, 1, 2, 3, 4}}, want: "aa-a+a-a+a-"},
		{name: "test_resetcycle08", src: "{% for i in outer %}{% for j in inner %}{% cycle 'a' 'b' %}{% endfor %}{% resetcycle %}{% endfor %}", ctx: map[string]any{"outer": []any{0, 1}, "inner": []any{0, 1, 2}}, want: "abaaba"},
		{name: "test_resetcycle09", src: "{% for i in outer %}{% cycle 'a' 'b' %}{% for j in inner %}{% cycle 'X' 'Y' %}{% endfor %}{% resetcycle %}{% endfor %}", ctx: map[string]any{"outer": []any{0, 1}, "inner": []any{0, 1, 2}}, want: "aXYXbXYX"},
		{name: "test_resetcycle10", src: "{% for i in test %}{% cycle 'X' 'Y' 'Z' as XYZ %}{% cycle 'a' 'b' 'c' as abc %}{% if i == 1 %}{% resetcycle abc %}{% endif %}{% endfor %}", ctx: map[string]any{"test": []any{0, 1, 2, 3, 4}}, want: "XaYbZaXbYc"},
		{name: "test_resetcycle11", src: "{% for i in test %}{% cycle 'X' 'Y' 'Z' as XYZ %}{% cycle 'a' 'b' 'c' as abc %}{% if i == 1 %}{% resetcycle XYZ %}{% endif %}{% endfor %}", ctx: map[string]any{"test": []any{0, 1, 2, 3, 4}}, want: "XaYbXcYaZb"},
	})
}
