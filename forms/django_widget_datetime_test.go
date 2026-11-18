package forms_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/widget_tests/test_dateinput.py::DateInputTest.test_render_none
func TestDateInputRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.DateInput{}, "date", nil, nil, `<input type="text" name="date">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_dateinput.py::DateInputTest.test_render_value
func TestDateInputRenderValue(t *testing.T) {
	d := date(2007, time.September, 17)
	widgetCheckHTML(t, forms.DateInput{}, "date", d, nil, `<input type="text" name="date" value="2007-09-17">`)
	widgetCheckHTML(t, forms.DateInput{}, "date", date(2007, time.September, 17), nil,
		`<input type="text" name="date" value="2007-09-17">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_dateinput.py::DateInputTest.test_string
func TestDateInputString(t *testing.T) {
	// A string value is shown as it is.
	widgetCheckHTML(t, forms.DateInput{}, "date", "2007-09-17", nil, `<input type="text" name="date" value="2007-09-17">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_dateinput.py::DateInputTest.test_format
func TestDateInputFormat(t *testing.T) {
	// Format changes the way a value is displayed.
	w := forms.DateInput{Format: "%d/%m/%Y", Attrs: forms.Attrs{"type": "date"}}
	widgetCheckHTML(t, w, "date", date(2007, time.September, 17), nil, `<input type="date" name="date" value="17/09/2007">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_dateinput.py::DateInputTest.test_fieldset
func TestDateInputFieldset(t *testing.T) {
	w := forms.DateInput{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<input id="id_field" name="field" required type="text">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_datetimeinput.py::DateTimeInputTest.test_render_none
func TestDateTimeInputRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.DateTimeInput{}, "date", nil, nil, `<input type="text" name="date">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_datetimeinput.py::DateTimeInputTest.test_render_value
func TestDateTimeInputRenderValue(t *testing.T) {
	// The microseconds are trimmed on display, by default.
	w := forms.DateTimeInput{}
	d := time.Date(2007, time.September, 17, 12, 51, 34, 482548000, time.UTC)
	widgetCheckHTML(t, w, "date", d, nil, `<input type="text" name="date" value="2007-09-17 12:51:34">`)
	widgetCheckHTML(t, w, "date", time.Date(2007, time.September, 17, 12, 51, 34, 0, time.UTC), nil,
		`<input type="text" name="date" value="2007-09-17 12:51:34">`)
	widgetCheckHTML(t, w, "date", time.Date(2007, time.September, 17, 12, 51, 0, 0, time.UTC), nil,
		`<input type="text" name="date" value="2007-09-17 12:51:00">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_datetimeinput.py::DateTimeInputTest.test_render_formatted
func TestDateTimeInputRenderFormatted(t *testing.T) {
	// Format changes the way a value is displayed.
	w := forms.DateTimeInput{Format: "%d/%m/%Y %H:%M", Attrs: forms.Attrs{"type": "datetime"}}
	d := time.Date(2007, time.September, 17, 12, 51, 34, 482548000, time.UTC)
	widgetCheckHTML(t, w, "date", d, nil, `<input type="datetime" name="date" value="17/09/2007 12:51">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_datetimeinput.py::DateTimeInputTest.test_fieldset
func TestDateTimeInputFieldset(t *testing.T) {
	w := forms.DateTimeInput{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<input id="id_field" name="field" required type="text">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_timeinput.py::TimeInputTest.test_render_none
func TestTimeInputRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.TimeInput{}, "time", nil, nil, `<input type="text" name="time">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_timeinput.py::TimeInputTest.test_render_value
func TestTimeInputRenderValue(t *testing.T) {
	// The microseconds are trimmed on display, by default.
	w := forms.TimeInput{}
	widgetCheckHTML(t, w, "time", time.Date(1, 1, 1, 12, 51, 34, 482548000, time.UTC), nil,
		`<input type="text" name="time" value="12:51:34">`)
	widgetCheckHTML(t, w, "time", time.Date(1, 1, 1, 12, 51, 34, 0, time.UTC), nil,
		`<input type="text" name="time" value="12:51:34">`)
	widgetCheckHTML(t, w, "time", time.Date(1, 1, 1, 12, 51, 0, 0, time.UTC), nil,
		`<input type="text" name="time" value="12:51:00">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_timeinput.py::TimeInputTest.test_string
func TestTimeInputString(t *testing.T) {
	widgetCheckHTML(t, forms.TimeInput{}, "time", "13:12:11", nil, `<input type="text" name="time" value="13:12:11">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_timeinput.py::TimeInputTest.test_format
func TestTimeInputFormat(t *testing.T) {
	// Format changes the way a value is displayed.
	w := forms.TimeInput{Format: "%H:%M", Attrs: forms.Attrs{"type": "time"}}
	widgetCheckHTML(t, w, "time", time.Date(1, 1, 1, 12, 51, 34, 482548000, time.UTC), nil,
		`<input type="time" name="time" value="12:51">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_timeinput.py::TimeInputTest.test_fieldset
func TestTimeInputFieldset(t *testing.T) {
	w := forms.TimeInput{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<input id="id_field" name="field" required type="text">`)
}
