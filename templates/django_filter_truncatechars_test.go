package templates_test

import (
	"testing"
)

// TestTruncatechars ports Django's TruncatecharsTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_truncatechars.py::TruncatecharsTests.test_truncatechars01
// Ported from Django: tests/template_tests/filter_tests/test_truncatechars.py::TruncatecharsTests.test_truncatechars02
// Ported from Django: tests/template_tests/filter_tests/test_truncatechars.py::TruncatecharsTests.test_fail_silently_incorrect_arg
// Ported from Django: tests/template_tests/filter_tests/test_truncatechars.py::TruncatecharsTests.test_truncatechars04
func TestTruncatechars(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_truncatechars01", src: "{{ a|truncatechars:3 }}", ctx: map[string]any{"a": "Testing, testing"}, want: "Te…"},
		{name: "test_truncatechars02", src: "{{ a|truncatechars:7 }}", ctx: map[string]any{"a": "Testing"}, want: "Testing"},
		{name: "test_fail_silently_incorrect_arg", src: "{{ a|truncatechars:'e' }}", ctx: map[string]any{"a": "Testing, testing"}, want: "Testing, testing"},
		{name: "test_truncatechars04", src: "{{ a|truncatechars:3 }}", ctx: map[string]any{"a": "abc"}, want: "abc"},
	})
}
