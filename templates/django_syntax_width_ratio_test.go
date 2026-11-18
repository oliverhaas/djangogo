package templates_test

import (
	"testing"
)

// TestWidthRatioTag ports Django's WidthRatioTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio01
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio02
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio03
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio04
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio05
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio06
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio07
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio08
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio09
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio10
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio11
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio12a
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio12b
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio13a
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio13b
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio14a
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio14b
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio15
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio16
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio17
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio18
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_widthratio19
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_zerodivisionerror_as_var
// Ported from Django: tests/template_tests/syntax_tests/test_width_ratio.py::WidthRatioTagTests.test_typeerror_as_var
func TestWidthRatioTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_widthratio01", src: "{% widthratio a b 0 %}", ctx: map[string]any{"a": 50, "b": 100}, want: "0"},
		{name: "test_widthratio02", src: "{% widthratio a b 100 %}", ctx: map[string]any{"a": 0, "b": 0}, want: "0"},
		{name: "test_widthratio03", src: "{% widthratio a b 100 %}", ctx: map[string]any{"a": 0, "b": 100}, want: "0"},
		{name: "test_widthratio04", src: "{% widthratio a b 100 %}", ctx: map[string]any{"a": 50, "b": 100}, want: "50"},
		{name: "test_widthratio05", src: "{% widthratio a b 100 %}", ctx: map[string]any{"a": 100, "b": 100}, want: "100"},
		{name: "test_widthratio06", src: "{% widthratio a b 100 %}", ctx: map[string]any{"a": 50, "b": 80}, want: "62"},
		{name: "test_widthratio07", src: "{% widthratio a b 100 %}", ctx: map[string]any{"a": 50, "b": 70}, want: "71"},
		{name: "test_widthratio08", src: "{% widthratio %}", wantErr: true},
		{name: "test_widthratio09", src: "{% widthratio a b %}", ctx: map[string]any{"a": 50, "b": 100}, wantErr: true},
		{name: "test_widthratio10", src: "{% widthratio a b 100.0 %}", ctx: map[string]any{"a": 50, "b": 100}, want: "50"},
		{name: "test_widthratio11", src: "{% widthratio a b c %}", ctx: map[string]any{"a": 50, "c": 100, "b": 100}, want: "50"},
		{name: "test_widthratio12a", src: "{% widthratio a b c %}", ctx: map[string]any{"a": "a", "c": 100, "b": 100}, want: ""},
		{name: "test_widthratio12b", src: "{% widthratio a b c %}", ctx: map[string]any{"a": nil, "c": 100, "b": 100}, want: ""},
		{name: "test_widthratio13a", src: "{% widthratio a b c %}", ctx: map[string]any{"a": 0, "c": 100, "b": "b"}, want: ""},
		{name: "test_widthratio13b", src: "{% widthratio a b c %}", ctx: map[string]any{"a": 0, "c": 100, "b": nil}, want: ""},
		{name: "test_widthratio14a", src: "{% widthratio a b c %}", ctx: map[string]any{"a": 0, "c": "c", "b": 100}, wantErr: true},
		{name: "test_widthratio14b", src: "{% widthratio a b c %}", ctx: map[string]any{"a": 0, "c": nil, "b": 100}, wantErr: true},
		{name: "test_widthratio15", src: `{% load custom %}{% widthratio a|noop:"x y" b 0 %}`, ctx: map[string]any{"a": 50, "b": 100}, want: "0"},
		{name: "test_widthratio16", src: "{% widthratio a b 100 as variable %}-{{ variable }}-", ctx: map[string]any{"a": 50, "b": 100}, want: "-50-"},
		{name: "test_widthratio17", src: "{% widthratio a b 100 as variable %}-{{ variable }}-", ctx: map[string]any{"a": 100, "b": 100}, want: "-100-"},
		{name: "test_widthratio18", src: "{% widthratio a b 100 as %}", wantErr: true},
		{name: "test_widthratio19", src: "{% widthratio a b 100 not_as variable %}", wantErr: true},
		{name: "test_zerodivisionerror_as_var", src: "{% widthratio a b 100 as variable %}-{{ variable }}-", ctx: map[string]any{"a": 0, "b": 0}, want: "-0-"},
		{name: "test_typeerror_as_var", src: "{% widthratio a b c as variable %}-{{ variable }}-", ctx: map[string]any{"a": "a", "c": 100, "b": 100}, want: "--"},
	})
}
