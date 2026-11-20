package templates_test

import (
	"fmt"
	"testing"
	"time"
)

// TestNowTag ports Django's NowTagTests.
//
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now01
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now03
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now05
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now06
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now07
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now_args
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now_variable
// Ported from Django: tests/template_tests/syntax_tests/test_now.py::NowTagTests.test_now_variable_as
func TestNowTag(t *testing.T) {
	// Django builds the expected value from the current date, as here.
	now := time.Now()
	day, month, year := now.Day(), int(now.Month()), now.Year()
	today := fmt.Sprintf("%d %d %d", day, month, year)
	runRenderCases(t, []renderCase{
		{name: "test_now01", src: `{% now "j n Y" %}`, want: today},
		{name: "test_now03", src: "{% now 'j n Y' %}", want: today},
		{name: "test_now05", src: `{% now 'j "n" Y'%}`, want: fmt.Sprintf(`%d "%d" %d`, day, month, year)},
		{name: "test_now06", src: `{% now "j 'n' Y"%}`, want: fmt.Sprintf("%d '%d' %d", day, month, year)},
		{name: "test_now07", src: `{% now "j n Y" as N %}-{{N}}-`, want: "-" + today + "-"},
		{name: "test_now_args", src: "{% now %}", wantErr: true},
		{name: "test_now_variable", src: "{% now my_format %}", ctx: map[string]any{"my_format": "j n Y"}, want: today},
		{name: "test_now_variable_as", src: "{% now my_format as N %}-{{ N }}-", ctx: map[string]any{"my_format": "j n Y"}, want: "-" + today + "-"},
	})
}
