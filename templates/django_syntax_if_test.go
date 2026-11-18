package templates_test

import (
	"testing"
)

// TestIfTag ports Django's IfTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag03
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag04
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag05
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag06
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag07
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag08
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag09
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag10
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag11
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag12
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag13
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_filter01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_filter02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_eq01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_eq02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_eq03
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_eq04
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_eq05
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_noteq01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_noteq02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_noteq03
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_noteq04
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_noteq05
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_gt_01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_gt_02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_gte_01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_gte_02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_lt_01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_lt_02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_lte_01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_lte_02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_in_01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_in_02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not_in_01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not_in_02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and03
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and04
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and05
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and06
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and07
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_and08
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or03
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or04
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or05
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or06
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or07
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or08
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_or09
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not06
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not07
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not08
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not09
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not10
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not11
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not12
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not13
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not14
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not15
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not16
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not17
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not18
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not19
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not20
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not21
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not22
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not23
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not24
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not25
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not26
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not27
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not28
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not29
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not30
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not31
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not32
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not33
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not34
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_not35
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error03
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error04
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error05
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error06
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error07
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error08
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error09
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error10
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error11
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_error12
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_else_if_tag_error01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_badarg01
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_badarg02
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_badarg03
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_badarg04
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_tag_single_eq
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_match
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_no_match
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_variable_missing
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_both_variables_missing
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_not_match
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_not_no_match
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_not_variable_missing
// Ported from Django: tests/template_tests/syntax_tests/test_if.py::IfTagTests.test_if_is_not_both_variables_missing
func TestIfTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_if_tag01", src: "{% if foo %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, want: "yes"},
		{name: "test_if_tag02", src: "{% if foo %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false}, want: "no"},
		{name: "test_if_tag03", src: "{% if foo %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag04", src: "{% if foo %}foo{% elif bar %}bar{% endif %}", ctx: map[string]any{"foo": true}, want: "foo"},
		{name: "test_if_tag05", src: "{% if foo %}foo{% elif bar %}bar{% endif %}", ctx: map[string]any{"bar": true}, want: "bar"},
		{name: "test_if_tag06", src: "{% if foo %}foo{% elif bar %}bar{% endif %}", want: ""},
		{name: "test_if_tag07", src: "{% if foo %}foo{% elif bar %}bar{% else %}nothing{% endif %}", ctx: map[string]any{"foo": true}, want: "foo"},
		{name: "test_if_tag08", src: "{% if foo %}foo{% elif bar %}bar{% else %}nothing{% endif %}", ctx: map[string]any{"bar": true}, want: "bar"},
		{name: "test_if_tag09", src: "{% if foo %}foo{% elif bar %}bar{% else %}nothing{% endif %}", want: "nothing"},
		{name: "test_if_tag10", src: "{% if foo %}foo{% elif bar %}bar{% elif baz %}baz{% else %}nothing{% endif %}", ctx: map[string]any{"foo": true}, want: "foo"},
		{name: "test_if_tag11", src: "{% if foo %}foo{% elif bar %}bar{% elif baz %}baz{% else %}nothing{% endif %}", ctx: map[string]any{"bar": true}, want: "bar"},
		{name: "test_if_tag12", src: "{% if foo %}foo{% elif bar %}bar{% elif baz %}baz{% else %}nothing{% endif %}", ctx: map[string]any{"baz": true}, want: "baz"},
		{name: "test_if_tag13", src: "{% if foo %}foo{% elif bar %}bar{% elif baz %}baz{% else %}nothing{% endif %}", want: "nothing"},
		{name: "test_if_tag_filter01", src: "{% if foo|length == 5 %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": "abcde"}, want: "yes"},
		{name: "test_if_tag_filter02", src: "{% if foo|upper == 'ABC' %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_eq01", src: "{% if foo == bar %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_eq02", src: "{% if foo == bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1}, want: "no"},
		{name: "test_if_tag_eq03", src: "{% if foo == bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1, "bar": 1}, want: "yes"},
		{name: "test_if_tag_eq04", src: "{% if foo == bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1, "bar": 2}, want: "no"},
		{name: "test_if_tag_eq05", src: "{% if foo == '' %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_noteq01", src: "{% if foo != bar %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_noteq02", src: "{% if foo != bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1}, want: "yes"},
		{name: "test_if_tag_noteq03", src: "{% if foo != bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1, "bar": 1}, want: "no"},
		{name: "test_if_tag_noteq04", src: "{% if foo != bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1, "bar": 2}, want: "yes"},
		{name: "test_if_tag_noteq05", src: `{% if foo != "" %}yes{% else %}no{% endif %}`, want: "yes"},
		{name: "test_if_tag_gt_01", src: "{% if 2 > 1 %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_gt_02", src: "{% if 1 > 1 %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_gte_01", src: "{% if 1 >= 1 %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_gte_02", src: "{% if 1 >= 2 %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_lt_01", src: "{% if 1 < 2 %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_lt_02", src: "{% if 1 < 1 %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_lte_01", src: "{% if 1 <= 1 %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_lte_02", src: "{% if 2 <= 1 %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_in_01", src: "{% if 1 in x %}yes{% else %}no{% endif %}", ctx: map[string]any{"x": []any{1}}, want: "yes"},
		{name: "test_if_tag_in_02", src: "{% if 2 in x %}yes{% else %}no{% endif %}", ctx: map[string]any{"x": []any{1}}, want: "no"},
		{name: "test_if_tag_not_in_01", src: "{% if 1 not in x %}yes{% else %}no{% endif %}", ctx: map[string]any{"x": []any{1}}, want: "no"},
		{name: "test_if_tag_not_in_02", src: "{% if 2 not in x %}yes{% else %}no{% endif %}", ctx: map[string]any{"x": []any{1}}, want: "yes"},
		{name: "test_if_tag_and01", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "yes"},
		{name: "test_if_tag_and02", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "no"},
		{name: "test_if_tag_and03", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "no"},
		{name: "test_if_tag_and04", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "no"},
		{name: "test_if_tag_and05", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false}, want: "no"},
		{name: "test_if_tag_and06", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"bar": false}, want: "no"},
		{name: "test_if_tag_and07", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, want: "no"},
		{name: "test_if_tag_and08", src: "{% if foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"bar": true}, want: "no"},
		{name: "test_if_tag_or01", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "yes"},
		{name: "test_if_tag_or02", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "yes"},
		{name: "test_if_tag_or03", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "yes"},
		{name: "test_if_tag_or04", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "no"},
		{name: "test_if_tag_or05", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false}, want: "no"},
		{name: "test_if_tag_or06", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"bar": false}, want: "no"},
		{name: "test_if_tag_or07", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, want: "yes"},
		{name: "test_if_tag_or08", src: "{% if foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"bar": true}, want: "yes"},
		{name: "test_if_tag_or09", src: "{% if foo or bar or baz %}yes{% else %}no{% endif %}", ctx: map[string]any{"baz": true}, want: "yes"},
		{name: "test_if_tag_not01", src: "{% if not foo %}no{% else %}yes{% endif %}", ctx: map[string]any{"foo": true}, want: "yes"},
		{name: "test_if_tag_not02", src: "{% if not not foo %}no{% else %}yes{% endif %}", ctx: map[string]any{"foo": true}, want: "no"},
		{name: "test_if_tag_not06", src: "{% if foo and not bar %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_not07", src: "{% if foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "no"},
		{name: "test_if_tag_not08", src: "{% if foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "yes"},
		{name: "test_if_tag_not09", src: "{% if foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "no"},
		{name: "test_if_tag_not10", src: "{% if foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "no"},
		{name: "test_if_tag_not11", src: "{% if not foo and bar %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_not12", src: "{% if not foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "no"},
		{name: "test_if_tag_not13", src: "{% if not foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "no"},
		{name: "test_if_tag_not14", src: "{% if not foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "yes"},
		{name: "test_if_tag_not15", src: "{% if not foo and bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "no"},
		{name: "test_if_tag_not16", src: "{% if foo or not bar %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_not17", src: "{% if foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "yes"},
		{name: "test_if_tag_not18", src: "{% if foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "yes"},
		{name: "test_if_tag_not19", src: "{% if foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "no"},
		{name: "test_if_tag_not20", src: "{% if foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "yes"},
		{name: "test_if_tag_not21", src: "{% if not foo or bar %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_not22", src: "{% if not foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "yes"},
		{name: "test_if_tag_not23", src: "{% if not foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "no"},
		{name: "test_if_tag_not24", src: "{% if not foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "yes"},
		{name: "test_if_tag_not25", src: "{% if not foo or bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "yes"},
		{name: "test_if_tag_not26", src: "{% if not foo and not bar %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_not27", src: "{% if not foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "no"},
		{name: "test_if_tag_not28", src: "{% if not foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "no"},
		{name: "test_if_tag_not29", src: "{% if not foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "no"},
		{name: "test_if_tag_not30", src: "{% if not foo and not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "yes"},
		{name: "test_if_tag_not31", src: "{% if not foo or not bar %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_tag_not32", src: "{% if not foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": true}, want: "no"},
		{name: "test_if_tag_not33", src: "{% if not foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true, "bar": false}, want: "yes"},
		{name: "test_if_tag_not34", src: "{% if not foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": true}, want: "yes"},
		{name: "test_if_tag_not35", src: "{% if not foo or not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false, "bar": false}, want: "yes"},
		{name: "test_if_tag_error01", src: "{% if %}yes{% endif %}", wantErr: true},
		{name: "test_if_tag_error02", src: "{% if foo and %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, wantErr: true},
		{name: "test_if_tag_error03", src: "{% if foo or %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, wantErr: true},
		{name: "test_if_tag_error04", src: "{% if not foo and %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, wantErr: true},
		{name: "test_if_tag_error05", src: "{% if not foo or %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, wantErr: true},
		{name: "test_if_tag_error06", src: "{% if abc def %}yes{% endif %}", wantErr: true},
		{name: "test_if_tag_error07", src: "{% if not %}yes{% endif %}", wantErr: true},
		{name: "test_if_tag_error08", src: "{% if and %}yes{% endif %}", wantErr: true},
		{name: "test_if_tag_error09", src: "{% if or %}yes{% endif %}", wantErr: true},
		{name: "test_if_tag_error10", src: "{% if == %}yes{% endif %}", wantErr: true},
		{name: "test_if_tag_error11", src: "{% if 1 == %}yes{% endif %}", wantErr: true},
		{name: "test_if_tag_error12", src: "{% if a not b %}yes{% endif %}", wantErr: true},
		{name: "test_else_if_tag_error01", src: "{% if foo is bar %} yes {% else if foo is not bar %} no {% endif %}", wantErr: true},
		{name: "test_if_tag_badarg01", src: "{% if x|default_if_none:y %}yes{% endif %}", want: ""},
		{name: "test_if_tag_badarg02", src: "{% if x|default_if_none:y %}yes{% endif %}", ctx: map[string]any{"y": 0}, want: ""},
		{name: "test_if_tag_badarg03", src: "{% if x|default_if_none:y %}yes{% endif %}", ctx: map[string]any{"y": 1}, want: "yes"},
		{name: "test_if_tag_badarg04", src: "{% if x|default_if_none:y %}yes{% else %}no{% endif %}", want: "no"},
		{name: "test_if_tag_single_eq", src: "{% if foo = bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1}, wantErr: true},
		{name: "test_if_is_match", src: "{% if foo is True %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": true}, want: "yes"},
		{name: "test_if_is_no_match", src: "{% if foo is True %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1}, want: "no"},
		{name: "test_if_is_variable_missing", src: "{% if foo is bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": 1}, want: "no"},
		{name: "test_if_is_both_variables_missing", src: "{% if foo is bar %}yes{% else %}no{% endif %}", want: "yes"},
		{name: "test_if_is_not_match", src: "{% if foo is not None %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false}, want: "yes"},
		{name: "test_if_is_not_no_match", src: "{% if foo is not None %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": nil}, want: "no"},
		{name: "test_if_is_not_variable_missing", src: "{% if foo is not bar %}yes{% else %}no{% endif %}", ctx: map[string]any{"foo": false}, want: "yes"},
		{name: "test_if_is_not_both_variables_missing", src: "{% if foo is not bar %}yes{% else %}no{% endif %}", want: "no"},
	})
}
