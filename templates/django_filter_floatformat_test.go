package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// TestFloatformat ports Django's FloatformatTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_floatformat.py::FloatformatTests.test_floatformat01
// Ported from Django: tests/template_tests/filter_tests/test_floatformat.py::FloatformatTests.test_floatformat02
func TestFloatformat(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_floatformat01", src: "{% autoescape off %}{{ a|floatformat }} {{ b|floatformat }}{% endautoescape %}", ctx: map[string]any{"a": "1.42", "b": templates.MarkSafe("1.42")}, want: "1.4 1.4"},
		{name: "test_floatformat02", src: "{{ a|floatformat }} {{ b|floatformat }}", ctx: map[string]any{"a": "1.42", "b": templates.MarkSafe("1.42")}, want: "1.4 1.4"},
	})
}

// TestFloatformatFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_floatformat.py::FunctionTests.test_inputs
// Ported from Django: tests/template_tests/filter_tests/test_floatformat.py::FunctionTests.test_zero_values
// Ported from Django: tests/template_tests/filter_tests/test_floatformat.py::FunctionTests.test_infinity
func TestFloatformatFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_inputs_1", src: "{{ a|floatformat }}", ctx: map[string]any{"a": 7.7}, want: "7.7"},
		{name: "test_inputs_2", src: "{{ a|floatformat }}", ctx: map[string]any{"a": 7.0}, want: "7"},
		{name: "test_inputs_3", src: "{{ a|floatformat }}", ctx: map[string]any{"a": 0.7}, want: "0.7"},
		{name: "test_inputs_4", src: "{{ a|floatformat }}", ctx: map[string]any{"a": -0.7}, want: "-0.7"},
		{name: "test_inputs_5", src: "{{ a|floatformat }}", ctx: map[string]any{"a": 0.07}, want: "0.1"},
		{name: "test_inputs_6", src: "{{ a|floatformat }}", ctx: map[string]any{"a": -0.07}, want: "-0.1"},
		{name: "test_inputs_7", src: "{{ a|floatformat }}", ctx: map[string]any{"a": 0.007}, want: "0.0"},
		{name: "test_inputs_8", src: "{{ a|floatformat }}", ctx: map[string]any{"a": 0.0}, want: "0"},
		{name: "test_inputs_9", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 7.7, "b": 0}, want: "8"},
		{name: "test_inputs_10", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 7.7, "b": 3}, want: "7.700"},
		{name: "test_inputs_11", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 6.0, "b": 3}, want: "6.000"},
		{name: "test_inputs_12", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 6.2, "b": 3}, want: "6.200"},
		{name: "test_inputs_13", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 6.2, "b": -3}, want: "6.200"},
		{name: "test_inputs_14", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 13.1031, "b": -3}, want: "13.103"},
		{name: "test_inputs_15", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 11.1197, "b": -2}, want: "11.12"},
		{name: "test_inputs_16", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 11.0, "b": -2}, want: "11"},
		{name: "test_inputs_17", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 11.000001, "b": -2}, want: "11.00"},
		{name: "test_inputs_18", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 8.2798, "b": 3}, want: "8.280"},
		{name: "test_inputs_19", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 5555.555, "b": 2}, want: "5555.56"},
		{name: "test_inputs_20", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 1.3, "b": 2}, want: "1.30"},
		{name: "test_inputs_21", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 0.12345, "b": 2}, want: "0.12"},
		{name: "test_inputs_22", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 13.1031, "b": "bar"}, want: "13.1031"},
		{name: "test_inputs_23", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 18.125, "b": 2}, want: "18.13"},
		{name: "test_inputs_24", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": -1.323297138040798e+35, "b": 2}, want: "-132329713804079800000000000000000000.00"},
		{name: "test_inputs_25", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": -1.323297138040798e+35, "b": -2}, want: "-132329713804079800000000000000000000"},
		{name: "test_inputs_26", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 1.5e-15, "b": 20}, want: "0.00000000000000150000"},
		{name: "test_inputs_27", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 1.5e-15, "b": -20}, want: "0.00000000000000150000"},
		{name: "test_inputs_28", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 1.0000000000000002, "b": 16}, want: "1.0000000000000002"},
		{name: "test_inputs_29", src: "{{ a|floatformat }}", ctx: map[string]any{"a": "1e199"}, want: "10000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"},
		{name: "test_zero_values_1", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 0, "b": 6}, want: "0.000000"},
		{name: "test_zero_values_2", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 0, "b": 7}, want: "0.0000000"},
		{name: "test_zero_values_3", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 0, "b": 10}, want: "0.0000000000"},
		{name: "test_zero_values_4", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": 1.5e-20, "b": 20}, want: "0.00000000000000000002"},
		{name: "test_zero_values_5", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": "0.00", "b": 0}, want: "0"},
		{name: "test_zero_values_6", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": "0.0000", "b": 2}, want: "0.00"},
		{name: "test_zero_values_7", src: "{{ a|floatformat:b }}", ctx: map[string]any{"a": "0.000000", "b": 4}, want: "0.0000"},
		{name: "test_infinity_1", src: "{{ a|floatformat }}", ctx: map[string]any{"a": "inf"}, want: "inf"},
		{name: "test_infinity_2", src: "{{ a|floatformat }}", ctx: map[string]any{"a": "NaN"}, want: "NaN"},
	})
}
