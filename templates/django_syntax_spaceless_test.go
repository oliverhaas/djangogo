package templates_test

import (
	"testing"
)

// TestSpacelessTag ports Django's SpacelessTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_spaceless.py::SpacelessTagTests.test_spaceless01
// Ported from Django: tests/template_tests/syntax_tests/test_spaceless.py::SpacelessTagTests.test_spaceless02
// Ported from Django: tests/template_tests/syntax_tests/test_spaceless.py::SpacelessTagTests.test_spaceless03
// Ported from Django: tests/template_tests/syntax_tests/test_spaceless.py::SpacelessTagTests.test_spaceless04
// Ported from Django: tests/template_tests/syntax_tests/test_spaceless.py::SpacelessTagTests.test_spaceless05
// Ported from Django: tests/template_tests/syntax_tests/test_spaceless.py::SpacelessTagTests.test_spaceless06
func TestSpacelessTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_spaceless01", src: "{% spaceless %} <b>    <i> text </i>    </b> {% endspaceless %}", want: "<b><i> text </i></b>"},
		{name: "test_spaceless02", src: "{% spaceless %} <b> \n <i> text </i> \n </b> {% endspaceless %}", want: "<b><i> text </i></b>"},
		{name: "test_spaceless03", src: "{% spaceless %}<b><i>text</i></b>{% endspaceless %}", want: "<b><i>text</i></b>"},
		{name: "test_spaceless04", src: "{% spaceless %}<b>   <i>{{ text }}</i>  </b>{% endspaceless %}", ctx: map[string]any{"text": "This & that"}, want: "<b><i>This &amp; that</i></b>"},
		{name: "test_spaceless05", src: "{% autoescape off %}{% spaceless %}<b>   <i>{{ text }}</i>  </b>{% endspaceless %}{% endautoescape %}", ctx: map[string]any{"text": "This & that"}, want: "<b><i>This & that</i></b>"},
		{name: "test_spaceless06", src: "{% spaceless %}<b>   <i>{{ text|safe }}</i>  </b>{% endspaceless %}", ctx: map[string]any{"text": "This & that"}, want: "<b><i>This & that</i></b>"},
	})
}
