package templates_test

import (
	"testing"
)

// TestCommentSyntax ports Django's CommentSyntaxTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax01
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax02
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax03
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax04
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax05
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax06
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax07
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax08
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax09
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax10
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax11
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_syntax12
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_tag01
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_tag02
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_tag03
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_tag04
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_comment_tag05
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_closing_tag_after_literal_opening_brace
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_template_syntax_in_raw_blocks
// Ported from Django: tests/template_tests/syntax_tests/test_comment.py::CommentSyntaxTests.test_literal_multiline_block_opener_in_raw_blocks
func TestCommentSyntax(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_comment_syntax01", src: "{# this is hidden #}hello", want: "hello"},
		{name: "test_comment_syntax02", src: "{# this is hidden #}hello{# foo #}", want: "hello"},
		{name: "test_comment_syntax03", src: "foo{#  {% if %}  #}", want: "foo"},
		{name: "test_comment_syntax04", src: "foo{#  {% endblock %}  #}", want: "foo"},
		{name: "test_comment_syntax05", src: "foo{#  {% somerandomtag %}  #}", want: "foo"},
		{name: "test_comment_syntax06", src: "foo{# {% #}", want: "foo"},
		{name: "test_comment_syntax07", src: "foo{# %} #}", want: "foo"},
		{name: "test_comment_syntax08", src: "foo{# %} #}bar", want: "foobar"},
		{name: "test_comment_syntax09", src: "foo{# {{ #}", want: "foo"},
		{name: "test_comment_syntax10", src: "foo{# }} #}", want: "foo"},
		{name: "test_comment_syntax11", src: "foo{# { #}", want: "foo"},
		{name: "test_comment_syntax12", src: "foo{# } #}", want: "foo"},
		{name: "test_comment_tag01", src: "{% comment %}this is hidden{% endcomment %}hello", want: "hello"},
		{name: "test_comment_tag02", src: "{% comment %}this is hidden{% endcomment %}hello{% comment %}foo{% endcomment %}", want: "hello"},
		{name: "test_comment_tag03", src: "foo{% comment %} {% if %} {% endcomment %}", want: "foo"},
		{name: "test_comment_tag04", src: "foo{% comment %} {% endblock %} {% endcomment %}", want: "foo"},
		{name: "test_comment_tag05", src: "foo{% comment %} {% somerandomtag %} {% endcomment %}", want: "foo"},
		{name: "test_closing_tag_after_literal_opening_brace", src: "{% comment %}{{% endcomment %}visible", want: "visible"},
		{name: "test_template_syntax_in_raw_blocks", src: "{% comment %}\n{% if user %}Hello, {{ user }}{% endif %}\n{% endcomment %}", want: ""},
		{name: "test_literal_multiline_block_opener_in_raw_blocks", src: "{% comment %}\n{%\n{% endcomment %}", want: ""},
	})
}
