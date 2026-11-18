package templates_test

import (
	"testing"
	"time"
)

// TestDate ports Django's DateTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_date.py::DateTests.test_date01
// Ported from Django: tests/template_tests/filter_tests/test_date.py::DateTests.test_date02
// Ported from Django: tests/template_tests/filter_tests/test_date.py::DateTests.test_date03
// Ported from Django: tests/template_tests/filter_tests/test_date.py::DateTests.test_date04
// Ported from Django: tests/template_tests/filter_tests/test_date.py::DateTests.test_date05
// Ported from Django: tests/template_tests/filter_tests/test_date.py::DateTests.test_date07
func TestDate(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_date01", src: `{{ d|date:"m" }}`, ctx: map[string]any{"d": time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)}, want: "01"},
		{name: "test_date02", src: "{{ d|date }}", ctx: map[string]any{"d": time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)}, want: "Jan. 1, 2008"},
		{name: "test_date03", src: `{{ d|date:"m" }}`, ctx: map[string]any{"d": "fail_string"}, want: ""},
		{name: "test_date04", src: `{{ d|date:"o" }}`, ctx: map[string]any{"d": time.Date(2008, 12, 29, 0, 0, 0, 0, time.UTC)}, want: "2009"},
		{name: "test_date05", src: `{{ d|date:"o" }}`, ctx: map[string]any{"d": time.Date(2010, 1, 3, 0, 0, 0, 0, time.UTC)}, want: "2009"},
		{name: "test_date07", src: `{{ d|date:"e" }}`, ctx: map[string]any{"d": time.Date(2009, 3, 12, 0, 0, 0, 0, time.UTC)}, want: "", skip: "a time.Time always has a zone; Django's naive datetime has none, see fidelity/divergences.md"},
	})
}

// TestDateFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_date.py::FunctionTests.test_date
// Ported from Django: tests/template_tests/filter_tests/test_date.py::FunctionTests.test_no_args
// Ported from Django: tests/template_tests/filter_tests/test_date.py::FunctionTests.test_escape_characters
func TestDateFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_date", src: "{{ a|date:b }}", ctx: map[string]any{"a": time.Date(2005, 12, 29, 0, 0, 0, 0, time.UTC), "b": "d F Y"}, want: "29 December 2005"},
		{name: "test_no_args_1", src: "{{ a|date }}", ctx: map[string]any{"a": ""}, want: ""},
		{name: "test_no_args_2", src: "{{ a|date }}", ctx: map[string]any{"a": nil}, want: ""},
		{name: "test_escape_characters", src: "{{ a|date:b }}", ctx: map[string]any{"a": time.Date(2005, 12, 29, 0, 0, 0, 0, time.UTC), "b": `jS \o\f F`}, want: "29th of December"},
	})
}
