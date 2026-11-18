package templates_test

import (
	"testing"
)

// TestForTag ports Django's ForTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag01
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag02
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_vars01
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_vars02
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_vars03
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_vars04
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_vars05
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_vars06
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack01
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack03
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack04
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack05
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack06
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack07
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack08
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_unpack_double_quote
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_unpack_single_quote
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_unpack_vertical_bar
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack09
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack13
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_empty01
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_empty02
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_empty03
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_filter_ws
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_unpack_strs
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_for_tag_context
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_invalid_arg
// Ported from Django: tests/template_tests/syntax_tests/test_for.py::ForTagTests.test_invalid_in_keyword
func TestForTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_for_tag01", src: "{% for val in values %}{{ val }}{% endfor %}", ctx: map[string]any{"values": []any{1, 2, 3}}, want: "123"},
		{name: "test_for_tag02", src: "{% for val in values reversed %}{{ val }}{% endfor %}", ctx: map[string]any{"values": []any{1, 2, 3}}, want: "321"},
		{name: "test_for_tag_vars01", src: "{% for val in values %}{{ forloop.counter }}{% endfor %}", ctx: map[string]any{"values": []any{6, 6, 6}}, want: "123"},
		{name: "test_for_tag_vars02", src: "{% for val in values %}{{ forloop.counter0 }}{% endfor %}", ctx: map[string]any{"values": []any{6, 6, 6}}, want: "012"},
		{name: "test_for_tag_vars03", src: "{% for val in values %}{{ forloop.revcounter }}{% endfor %}", ctx: map[string]any{"values": []any{6, 6, 6}}, want: "321"},
		{name: "test_for_tag_vars04", src: "{% for val in values %}{{ forloop.revcounter0 }}{% endfor %}", ctx: map[string]any{"values": []any{6, 6, 6}}, want: "210"},
		{name: "test_for_tag_vars05", src: "{% for val in values %}{% if forloop.first %}f{% else %}x{% endif %}{% endfor %}", ctx: map[string]any{"values": []any{6, 6, 6}}, want: "fxx"},
		{name: "test_for_tag_vars06", src: "{% for val in values %}{% if forloop.last %}l{% else %}x{% endif %}{% endfor %}", ctx: map[string]any{"values": []any{6, 6, 6}}, want: "xxl"},
		{name: "test_for_tag_unpack01", src: "{% for key,value in items %}{{ key }}:{{ value }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, want: "one:1/two:2/"},
		{name: "test_for_tag_unpack03", src: "{% for key, value in items %}{{ key }}:{{ value }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, want: "one:1/two:2/"},
		{name: "test_for_tag_unpack04", src: "{% for key , value in items %}{{ key }}:{{ value }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, want: "one:1/two:2/"},
		{name: "test_for_tag_unpack05", src: "{% for key ,value in items %}{{ key }}:{{ value }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, want: "one:1/two:2/"},
		{name: "test_for_tag_unpack06", src: "{% for key value in items %}{{ key }}:{{ value }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, wantErr: true},
		{name: "test_for_tag_unpack07", src: "{% for key,,value in items %}{{ key }}:{{ value }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, wantErr: true},
		{name: "test_for_tag_unpack08", src: "{% for key,value, in items %}{{ key }}:{{ value }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, wantErr: true},
		{name: "test_unpack_double_quote", src: `{% for "k" in items %}{{ "k" }}/{% endfor %}`, ctx: map[string]any{"items": []any{1, 2}}, wantErr: true},
		{name: "test_unpack_single_quote", src: "{% for 'k' in items %}{{ k }}/{% endfor %}", ctx: map[string]any{"items": []any{1, 2}}, wantErr: true},
		{name: "test_unpack_vertical_bar", src: "{% for k|upper in items %}{{ k|upper }}/{% endfor %}", ctx: map[string]any{"items": []any{1, 2}}, wantErr: true},
		{name: "test_for_tag_unpack09", src: "{% for val in items %}{{ val.0 }}:{{ val.1 }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1}, []any{"two", 2}}}, want: "one:1/two:2/"},
		{name: "test_for_tag_unpack13", src: "{% for x,y,z in items %}{{ x }}:{{ y }},{{ z }}/{% endfor %}", ctx: map[string]any{"items": []any{[]any{"one", 1, "carrot"}, []any{"two", 2, "cheese"}}}, want: "one:1,carrot/two:2,cheese/"},
		{name: "test_for_tag_empty01", src: "{% for val in values %}{{ val }}{% empty %}empty text{% endfor %}", ctx: map[string]any{"values": []any{1, 2, 3}}, want: "123"},
		{name: "test_for_tag_empty02", src: "{% for val in values %}{{ val }}{% empty %}values array empty{% endfor %}", ctx: map[string]any{"values": []any{}}, want: "values array empty"},
		{name: "test_for_tag_empty03", src: "{% for val in values %}{{ val }}{% empty %}values array not found{% endfor %}", want: "values array not found"},
		{name: "test_for_tag_filter_ws", src: "{% load custom %}{% for x in s|noop:'x y' %}{{ x }}{% endfor %}", ctx: map[string]any{"s": "abc"}, want: "abc"},
		{name: "test_for_tag_unpack_strs", src: "{% for x,y in items %}{{ x }}:{{ y }}/{% endfor %}", ctx: map[string]any{"items": []any{"ab", "ac"}}, want: "a:b/a:c/"},
		{name: "test_for_tag_context", src: `{% with alpha=alpha.values %}{% include "base" %}{% endwith %}_{% with alpha=alpha.extra %}{% include "base" %}{% endwith %}`, templates: map[string]string{"base": "{% for x, y in alpha %}{{ x }}:{{ y }},{% endfor %}"}, ctx: map[string]any{"alpha": map[string]any{"values": []any{[]any{"two", 2}, []any{"four", 4}}, "extra": []any{[]any{"six", 6}, []any{"eight", 8}}}}, want: "two:2,four:4,_six:6,eight:8,"},
		{name: "test_invalid_arg", src: "{% for x items %}{{ x }}{% endfor %}", ctx: map[string]any{"items": []any{1, 2}}, wantErr: true},
		{name: "test_invalid_in_keyword", src: "{% for x from items %}{{ x }}{% endfor %}", ctx: map[string]any{"items": []any{1, 2}}, wantErr: true},
	})
}
