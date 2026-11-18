package templates_test

import (
	"testing"
)

// TestStaticTag ports Django's StaticTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_static.py::StaticTagTests.test_static_prefixtag_without_as
// Ported from Django: tests/template_tests/syntax_tests/test_static.py::StaticTagTests.test_static_statictag_without_path
func TestStaticTag(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_static_prefixtag_without_as", src: "{% load static %}{% get_media_prefix ad media_prefix %}{{ media_prefix }}", wantErr: true},
		{name: "test_static_statictag_without_path", src: "{% load static %}{% static %}", wantErr: true},
	})
}
