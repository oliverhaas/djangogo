package templates_test

import (
	"testing"
)

// TestBuiltins ports Django's BuiltinsTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_builtins.py::BuiltinsTests.test_builtins01
// Ported from Django: tests/template_tests/syntax_tests/test_builtins.py::BuiltinsTests.test_builtins02
// Ported from Django: tests/template_tests/syntax_tests/test_builtins.py::BuiltinsTests.test_builtins03
func TestBuiltins(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_builtins01", src: "{{ True }}", want: "True"},
		{name: "test_builtins02", src: "{{ False }}", want: "False"},
		{name: "test_builtins03", src: "{{ None }}", want: "None", skip: "None prints as an empty string, see fidelity/divergences.md"},
	})
}
