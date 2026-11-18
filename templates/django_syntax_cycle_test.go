package templates_test

import (
	"testing"
)

// TestCycleTag ports Django's CycleTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle01
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle05
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle07
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle10
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle11
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle12
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle13
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle14
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle15
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle16
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle17
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle18
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle19
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle20
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle21
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle22
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle23
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle24
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle25
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle26
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle27
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle28
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle29
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle30
// Ported from Django: tests/template_tests/syntax_tests/test_cycle.py::CycleTagTests.test_cycle_undefined
func TestCycleTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_cycle01", src: "{% cycle a %}", wantErr: true},
		{name: "test_cycle05", src: "{% cycle %}", wantErr: true},
		{name: "test_cycle07", src: "{% cycle a,b,c as foo %}{% cycle bar %}", wantErr: true},
		{name: "test_cycle10", src: "{% cycle 'a' 'b' 'c' as abc %}{% cycle abc %}", want: "ab"},
		{name: "test_cycle11", src: "{% cycle 'a' 'b' 'c' as abc %}{% cycle abc %}{% cycle abc %}", want: "abc"},
		{name: "test_cycle12", src: "{% cycle 'a' 'b' 'c' as abc %}{% cycle abc %}{% cycle abc %}{% cycle abc %}", want: "abca"},
		{name: "test_cycle13", src: "{% for i in test %}{% cycle 'a' 'b' %}{{ i }},{% endfor %}", ctx: map[string]any{"test": []any{0, 1, 2, 3, 4}}, want: "a0,b1,a2,b3,a4,"},
		{name: "test_cycle14", src: "{% cycle one two as foo %}{% cycle foo %}", ctx: map[string]any{"one": "1", "two": "2"}, want: "12"},
		{name: "test_cycle15", src: "{% for i in test %}{% cycle aye bee %}{{ i }},{% endfor %}", ctx: map[string]any{"test": []any{0, 1, 2, 3, 4}, "aye": "a", "bee": "b"}, want: "a0,b1,a2,b3,a4,"},
		{name: "test_cycle16", src: "{% cycle one|lower two as foo %}{% cycle foo %}", ctx: map[string]any{"one": "A", "two": "2"}, want: "a2"},
		{name: "test_cycle17", src: "{% cycle 'a' 'b' 'c' as abc silent %}{% cycle abc %}{% cycle abc %}{% cycle abc %}{% cycle abc %}", want: ""},
		{name: "test_cycle18", src: "{% cycle 'a' 'b' 'c' as foo invalid_flag %}", wantErr: true},
		{name: "test_cycle19", src: "{% cycle 'a' 'b' as silent %}{% cycle silent %}", want: "ab"},
		{name: "test_cycle20", src: "{% cycle one two as foo %} &amp; {% cycle foo %}", ctx: map[string]any{"two": "C & D", "one": "A & B"}, want: "A &amp; B &amp; C &amp; D"},
		{name: "test_cycle21", src: "{% filter force_escape %}{% cycle one two as foo %} & {% cycle foo %}{% endfilter %}", ctx: map[string]any{"two": "C & D", "one": "A & B"}, want: "A &amp;amp; B &amp; C &amp;amp; D"},
		{name: "test_cycle22", src: "{% for x in values %}{% cycle 'a' 'b' 'c' as abc silent %}{{ x }}{% endfor %}", ctx: map[string]any{"values": []any{1, 2, 3, 4}}, want: "1234"},
		{name: "test_cycle23", src: "{% for x in values %}{% cycle 'a' 'b' 'c' as abc silent %}{{ abc }}{{ x }}{% endfor %}", ctx: map[string]any{"values": []any{1, 2, 3, 4}}, want: "a1b2c3a4"},
		{name: "test_cycle24", src: "{% for x in values %}{% cycle 'a' 'b' 'c' as abc silent %}{% include 'included-cycle' %}{% endfor %}", templates: map[string]string{"included-cycle": "{{ abc }}"}, ctx: map[string]any{"values": []any{1, 2, 3, 4}}, want: "abca"},
		{name: "test_cycle25", src: "{% cycle a as abc %}", ctx: map[string]any{"a": "<"}, want: "&lt;"},
		{name: "test_cycle26", src: "{% cycle a b as ab %}{% cycle ab %}", ctx: map[string]any{"a": "<", "b": ">"}, want: "&lt;&gt;"},
		{name: "test_cycle27", src: "{% autoescape off %}{% cycle a b as ab %}{% cycle ab %}{% endautoescape %}", ctx: map[string]any{"a": "<", "b": ">"}, want: "<>"},
		{name: "test_cycle28", src: "{% cycle a|safe b as ab %}{% cycle ab %}", ctx: map[string]any{"a": "<", "b": ">"}, want: "<&gt;"},
		{name: "test_cycle29", src: "{% cycle 'a' 'b' 'c' as cycler silent %}{% for x in values %}{% ifchanged x %}{% cycle cycler %}{{ cycler }}{% else %}{{ cycler }}{% endifchanged %}{% endfor %}", ctx: map[string]any{"values": []any{1, 2, 3, 4, 5, 6, 7, 8, 8, 8, 9, 9}}, want: "bcabcabcccaa"},
		{name: "test_cycle30", src: "{% cycle 'a' 'b' 'c' as cycler silent %}{% for x in values %}{% with doesnothing=irrelevant %}{% ifchanged x %}{% cycle cycler %}{{ cycler }}{% else %}{{ cycler }}{% endifchanged %}{% endwith %}{% endfor %}", ctx: map[string]any{"irrelevant": 1, "values": []any{1, 2, 3, 4, 5, 6, 7, 8, 8, 8, 9, 9}}, want: "bcabcabcccaa", skip: "{% cycle name %} inside {% with %} sets the variable for that block only, see fidelity/divergences.md"},
		{name: "test_cycle_undefined", src: "{% cycle 'a' 'b' 'c' as cycler silent %}{% for x in values %}{% cycle undefined %}{{ cycler }}{% endfor %}", wantErr: true},
	})
}
