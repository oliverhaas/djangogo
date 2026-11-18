package templates_test

import (
	"testing"
)

// TestYesNo ports Django's YesNoTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::YesNoTests.test_true
func TestYesNo(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_true", src: `{{ var|yesno:"yup,nup,mup" }} {{ var|yesno }}`, ctx: map[string]any{"var": true}, want: "yup yes"},
	})
}

// TestYesnoFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::FunctionTests.test_true
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::FunctionTests.test_false
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::FunctionTests.test_none
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::FunctionTests.test_true_arguments
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::FunctionTests.test_false_arguments
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::FunctionTests.test_none_two_arguments
// Ported from Django: tests/template_tests/filter_tests/test_yesno.py::FunctionTests.test_none_three_arguments
func TestYesnoFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_true", src: "{{ a|yesno }}", ctx: map[string]any{"a": true}, want: "yes"},
		{name: "test_false", src: "{{ a|yesno }}", ctx: map[string]any{"a": false}, want: "no"},
		{name: "test_none", src: "{{ a|yesno }}", ctx: map[string]any{"a": nil}, want: "maybe"},
		{name: "test_true_arguments", src: "{{ a|yesno:b }}", ctx: map[string]any{"a": true, "b": "certainly,get out of town,perhaps"}, want: "certainly"},
		{name: "test_false_arguments", src: "{{ a|yesno:b }}", ctx: map[string]any{"a": false, "b": "certainly,get out of town,perhaps"}, want: "get out of town"},
		{name: "test_none_two_arguments", src: "{{ a|yesno:b }}", ctx: map[string]any{"a": nil, "b": "certainly,get out of town"}, want: "get out of town"},
		{name: "test_none_three_arguments", src: "{{ a|yesno:b }}", ctx: map[string]any{"a": nil, "b": "certainly,get out of town,perhaps"}, want: "perhaps"},
	})
}
