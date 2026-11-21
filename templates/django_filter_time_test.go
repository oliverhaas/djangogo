package templates_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// TestTime ports Django's TimeTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_time.py::TimeTests.test_time00
// Ported from Django: tests/template_tests/filter_tests/test_time.py::TimeTests.test_time01
// Ported from Django: tests/template_tests/filter_tests/test_time.py::TimeTests.test_time04
// Ported from Django: tests/template_tests/filter_tests/test_time.py::TimeTests.test_time05
// Ported from Django: tests/template_tests/filter_tests/test_time.py::TimeTests.test_time06
func TestTime(t *testing.T) {
	tzPlus0315 := time.FixedZone("+0315", 3*3600+15*60)
	runRenderCases(t, []renderCase{
		{name: "test_time00", src: "{{ dt|time }}", ctx: map[string]any{"dt": orm.NewTime(16, 25, 0, 0)}, want: "4:25 p.m."},
		{name: "test_time01", src: `{{ dt|time:"e:O:T:Z" }}`, ctx: map[string]any{"dt": time.Date(2024, 5, 1, 16, 25, 0, 0, tzPlus0315)}, want: "+0315:+0315:+0315:11700"},
		{name: "test_time04", src: `{{ t|time:"P:e:O:T:Z" }}`, ctx: map[string]any{"t": orm.NewTime(4, 0, 0, 0)}, want: "4 a.m.::::"},
		{name: "test_time05", src: `{{ d|time:"P:e:O:T:Z" }}`, ctx: map[string]any{"d": orm.NewDate(2024, 5, 1)}, want: ""},
		{name: "test_time06", src: `{{ obj|time:"P:e:O:T:Z" }}`, ctx: map[string]any{"obj": "non-datetime-value"}, want: ""},
	})
}

// TestTimeFunction ports Django's FunctionTests.
//
// Ported from Django: tests/template_tests/filter_tests/test_time.py::FunctionTests.test_no_args
// Ported from Django: tests/template_tests/filter_tests/test_time.py::FunctionTests.test_inputs
func TestTimeFunction(t *testing.T) {
	runRenderCases(t, []renderCase{
		{name: "test_no_args_1", src: "{{ a|time }}", ctx: map[string]any{"a": ""}, want: ""},
		{name: "test_no_args_2", src: "{{ a|time }}", ctx: map[string]any{"a": nil}, want: ""},
		{name: "test_inputs_13", src: `{{ a|time:"h" }}`, ctx: map[string]any{"a": orm.NewTime(13, 0, 0, 0)}, want: "01"},
		{name: "test_inputs_0", src: `{{ a|time:"h" }}`, ctx: map[string]any{"a": orm.NewTime(0, 0, 0, 0)}, want: "12"},
	})
}
