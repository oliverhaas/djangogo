package templates_test

import (
	"testing"
)

// TestIfChangedTag ports Django's IfChangedTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged01
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged02
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged03
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged04
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged05
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged06
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged07
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged08
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_param01
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_param02
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_param03
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_param04
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_param05
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_else01
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_else02
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_else03
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_else04
// Ported from Django: tests/template_tests/syntax_tests/test_if_changed.py::IfChangedTagTests.test_ifchanged_filter_ws
func TestIfChangedTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_ifchanged01", src: "{% for n in num %}{% ifchanged %}{{ n }}{% endifchanged %}{% endfor %}", ctx: map[string]any{"num": []any{1, 2, 3}}, want: "123"},
		{name: "test_ifchanged02", src: "{% for n in num %}{% ifchanged %}{{ n }}{% endifchanged %}{% endfor %}", ctx: map[string]any{"num": []any{1, 1, 3}}, want: "13"},
		{name: "test_ifchanged03", src: "{% for n in num %}{% ifchanged %}{{ n }}{% endifchanged %}{% endfor %}", ctx: map[string]any{"num": []any{1, 1, 1}}, want: "1"},
		{name: "test_ifchanged04", src: "{% for n in num %}{% ifchanged %}{{ n }}{% endifchanged %}{% for x in numx %}{% ifchanged %}{{ x }}{% endifchanged %}{% endfor %}{% endfor %}", ctx: map[string]any{"num": []any{1, 2, 3}, "numx": []any{2, 2, 2}}, want: "122232"},
		{name: "test_ifchanged05", src: "{% for n in num %}{% ifchanged %}{{ n }}{% endifchanged %}{% for x in numx %}{% ifchanged %}{{ x }}{% endifchanged %}{% endfor %}{% endfor %}", ctx: map[string]any{"num": []any{1, 1, 1}, "numx": []any{1, 2, 3}}, want: "1123123123"},
		{name: "test_ifchanged06", src: "{% for n in num %}{% ifchanged %}{{ n }}{% endifchanged %}{% for x in numx %}{% ifchanged %}{{ x }}{% endifchanged %}{% endfor %}{% endfor %}", ctx: map[string]any{"num": []any{1, 1, 1}, "numx": []any{2, 2, 2}}, want: "1222"},
		{name: "test_ifchanged07", src: "{% for n in num %}{% ifchanged %}{{ n }}{% endifchanged %}{% for x in numx %}{% ifchanged %}{{ x }}{% endifchanged %}{% for y in numy %}{% ifchanged %}{{ y }}{% endifchanged %}{% endfor %}{% endfor %}{% endfor %}", ctx: map[string]any{"num": []any{1, 1, 1}, "numx": []any{2, 2, 2}, "numy": []any{3, 3, 3}}, want: "1233323332333"},
		{name: "test_ifchanged08", src: "{% for data in datalist %}{% for c,d in data %}{% if c %}{% ifchanged %}{{ d }}{% endifchanged %}{% endif %}{% endfor %}{% endfor %}", ctx: map[string]any{"datalist": []any{[]any{[]any{1, "a"}, []any{1, "a"}, []any{0, "b"}, []any{1, "c"}}, []any{[]any{0, "a"}, []any{1, "c"}, []any{1, "d"}, []any{1, "d"}, []any{0, "e"}}}}, want: "accd"},
		{name: "test_ifchanged_param01", src: "{% for n in num %}{% ifchanged n %}..{% endifchanged %}{{ n }}{% endfor %}", ctx: map[string]any{"num": []any{1, 2, 3}}, want: "..1..2..3"},
		{name: "test_ifchanged_param02", src: "{% for n in num %}{% for x in numx %}{% ifchanged n %}..{% endifchanged %}{{ x }}{% endfor %}{% endfor %}", ctx: map[string]any{"num": []any{1, 2, 3}, "numx": []any{5, 6, 7}}, want: "..567..567..567"},
		{name: "test_ifchanged_param03", src: "{% for n in num %}{{ n }}{% for x in numx %}{% ifchanged x n %}{{ x }}{% endifchanged %}{% endfor %}{% endfor %}", ctx: map[string]any{"num": []any{1, 1, 2}, "numx": []any{5, 6, 6}}, want: "156156256"},
		{name: "test_ifchanged_param04", src: "{% for d in days %}{% ifchanged %}{{ d.day }}{% endifchanged %}{% for h in d.hours %}{% ifchanged d h %}{{ h }}{% endifchanged %}{% endfor %}{% endfor %}", ctx: map[string]any{"days": []any{map[string]any{"hours": []any{1, 2, 3}, "day": 1}, map[string]any{"hours": []any{3}, "day": 2}}}, want: "112323"},
		{name: "test_ifchanged_param05", src: "{% for d in days %}{% ifchanged d.day %}{{ d.day }}{% endifchanged %}{% for h in d.hours %}{% ifchanged d.day h %}{{ h }}{% endifchanged %}{% endfor %}{% endfor %}", ctx: map[string]any{"days": []any{map[string]any{"hours": []any{1, 2, 3}, "day": 1}, map[string]any{"hours": []any{3}, "day": 2}}}, want: "112323"},
		{name: "test_ifchanged_else01", src: "{% for id in ids %}{{ id }}{% ifchanged id %}-first{% else %}-other{% endifchanged %},{% endfor %}", ctx: map[string]any{"ids": []any{1, 1, 2, 2, 2, 3}}, want: "1-first,1-other,2-first,2-other,2-other,3-first,"},
		{name: "test_ifchanged_else02", src: `{% for id in ids %}{{ id }}-{% ifchanged id %}{% cycle "red" "blue" %}{% else %}gray{% endifchanged %},{% endfor %}`, ctx: map[string]any{"ids": []any{1, 1, 2, 2, 2, 3}}, want: "1-red,1-gray,2-blue,2-gray,2-gray,3-red,"},
		{name: "test_ifchanged_else03", src: `{% for id in ids %}{{ id }}{% ifchanged id %}-{% cycle "red" "blue" %}{% else %}{% endifchanged %},{% endfor %}`, ctx: map[string]any{"ids": []any{1, 1, 2, 2, 2, 3}}, want: "1-red,1,2-blue,2,2,3-red,"},
		{name: "test_ifchanged_else04", src: "{% for id in ids %}{% ifchanged %}***{{ id }}*{% else %}...{% endifchanged %}{{ forloop.counter }}{% endfor %}", ctx: map[string]any{"ids": []any{1, 1, 2, 2, 2, 3, 4}}, want: "***1*1...2***2*3...4...5***3*6***4*7"},
		{name: "test_ifchanged_filter_ws", src: `{% load custom %}{% for n in num %}{% ifchanged n|noop:"x y" %}..{% endifchanged %}{{ n }}{% endfor %}`, ctx: map[string]any{"num": []any{1, 2, 3}}, want: "..1..2..3"},
	})
}
