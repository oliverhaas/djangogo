package forms_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/forms"
)

// myMultiWidget is the MyMultiWidget of Django's multiwidget tests: it splits
// a value at "__".
func myMultiWidget(attrs forms.Attrs, widgets ...forms.Widget) forms.MultiWidget {
	return forms.MultiWidget{
		Widgets: widgets,
		Attrs:   attrs,
		Decompress: func(value any) []any {
			if s, ok := value.(string); ok && s != "" {
				parts := strings.Split(s, "__")
				out := make([]any, len(parts))
				for i, part := range parts {
					out[i] = part
				}
				return out
			}
			return []any{"", ""}
		},
	}
}

// complexMultiWidget is the ComplexMultiWidget of Django's multiwidget and
// multivaluefield tests: a text input, a multiple select of the beatles and a
// split date and time.
func complexMultiWidget() forms.MultiWidget {
	return forms.MultiWidget{
		Widgets: []forms.Widget{
			forms.TextInput{},
			forms.SelectMultiple{Choices: widgetBeatles},
			forms.SplitDateTimeWidget{},
		},
		Decompress: func(value any) []any {
			s, _ := value.(string)
			if s == "" {
				return []any{nil, nil, nil}
			}
			data := strings.Split(s, ",")
			when, _ := time.Parse("2006-01-02 15:04:05", data[2])
			return []any{data[0], strings.Split(data[1], ""), when}
		},
	}
}

// Django's dictionary of widgets is the Go Names and Widgets in the same
// order.
//
// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_subwidgets_name
func TestMultiWidgetSubwidgetsName(t *testing.T) {
	w := forms.MultiWidget{
		Names: []string{"", "big", "small"},
		Widgets: []forms.Widget{
			forms.TextInput{},
			forms.TextInput{Attrs: forms.Attrs{"class": "big"}},
			forms.TextInput{Attrs: forms.Attrs{"class": "small"}},
		},
	}
	widgetCheckHTML(t, w, "name", []any{"John", "George", "Paul"}, nil,
		`<input type="text" name="name" value="John">`+
			`<input type="text" name="name_big" value="George" class="big">`+
			`<input type="text" name="name_small" value="Paul" class="small">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_text_inputs
func TestMultiWidgetTextInputs(t *testing.T) {
	w := myMultiWidget(nil, forms.TextInput{Attrs: forms.Attrs{"class": "big"}}, forms.TextInput{Attrs: forms.Attrs{"class": "small"}})
	want := `<input type="text" class="big" value="john" name="name_0">` +
		`<input type="text" class="small" value="lennon" name="name_1">`
	widgetCheckHTML(t, w, "name", []any{"john", "lennon"}, nil, want)
	widgetCheckHTML(t, w, "name", []string{"john", "lennon"}, nil, want)
	widgetCheckHTML(t, w, "name", "john__lennon", nil, want)
	widgetCheckHTML(t, w, "name", "john__lennon", forms.Attrs{"id": "foo"},
		`<input id="foo_0" type="text" class="big" value="john" name="name_0">`+
			`<input id="foo_1" type="text" class="small" value="lennon" name="name_1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_constructor_attrs
func TestMultiWidgetConstructorAttrs(t *testing.T) {
	w := myMultiWidget(forms.Attrs{"id": "bar"},
		forms.TextInput{Attrs: forms.Attrs{"class": "big"}}, forms.TextInput{Attrs: forms.Attrs{"class": "small"}})
	widgetCheckHTML(t, w, "name", []any{"john", "lennon"}, nil,
		`<input id="bar_0" type="text" class="big" value="john" name="name_0">`+
			`<input id="bar_1" type="text" class="small" value="lennon" name="name_1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_constructor_attrs_with_type
func TestMultiWidgetConstructorAttrsWithType(t *testing.T) {
	attrs := forms.Attrs{"type": "number"}
	w := myMultiWidget(attrs, forms.TextInput{}, forms.TextInput{})
	widgetCheckHTML(t, w, "code", []any{"1", "2"}, nil,
		`<input type="number" value="1" name="code_0"><input type="number" value="2" name="code_1">`)
	w = myMultiWidget(forms.Attrs{"class": "bar"}, forms.TextInput{Attrs: attrs}, forms.TextInput{Attrs: attrs})
	widgetCheckHTML(t, w, "code", []any{"1", "2"}, nil,
		`<input type="number" value="1" name="code_0" class="bar">`+
			`<input type="number" value="2" name="code_1" class="bar">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_value_omitted_from_data
func TestMultiWidgetValueOmittedFromData(t *testing.T) {
	w := myMultiWidget(nil, forms.TextInput{}, forms.TextInput{})
	equal(t, w.ValueOmittedFromData(url.Values{}, "field"), true)
	equal(t, w.ValueOmittedFromData(vals("field_0", "x"), "field"), false)
	equal(t, w.ValueOmittedFromData(vals("field_1", "y"), "field"), false)
	equal(t, w.ValueOmittedFromData(vals("field_0", "x", "field_1", "y"), "field"), false)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_value_from_datadict_subwidgets_name
func TestMultiWidgetValueFromDatadictSubwidgetsName(t *testing.T) {
	w := forms.MultiWidget{Names: []string{"x", ""}, Widgets: []forms.Widget{forms.TextInput{}, forms.TextInput{}}}
	for _, tc := range []struct {
		data url.Values
		want []any
	}{
		{url.Values{}, []any{nil, nil}},
		{vals("field", "x"), []any{nil, "x"}},
		{vals("field_x", "y"), []any{"y", nil}},
		{vals("field", "x", "field_x", "y"), []any{"y", "x"}},
	} {
		equal(t, w.ValueFromData(tc.data, "field"), any(tc.want))
	}
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_value_omitted_from_data_subwidgets_name
func TestMultiWidgetValueOmittedFromDataSubwidgetsName(t *testing.T) {
	w := forms.MultiWidget{Names: []string{"x", ""}, Widgets: []forms.Widget{forms.TextInput{}, forms.TextInput{}}}
	for _, tc := range []struct {
		data url.Values
		want bool
	}{
		{url.Values{}, true},
		{vals("field", "x"), false},
		{vals("field_x", "y"), false},
		{vals("field", "x", "field_x", "y"), false},
	} {
		equal(t, w.ValueOmittedFromData(tc.data, "field"), tc.want)
	}
}

// FIXME-PORT: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_needs_multipart_true
// and test_needs_multipart_false need FileInput and needs_multipart_form,
// which the Go forms package does not have.

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_nested_multiwidget
func TestMultiWidgetNestedMultiwidget(t *testing.T) {
	// MultiWidgets can be composed of other MultiWidgets.
	widgetCheckHTML(t, complexMultiWidget(), "name", "some text,JP,2007-04-25 06:24:00", nil, `
		<input type="text" name="name_0" value="some text">
		<select multiple name="name_1">
			<option value="J" selected>John</option>
			<option value="P" selected>Paul</option>
			<option value="G">George</option>
			<option value="R">Ringo</option>
		</select>
		<input type="text" name="name_2_0" value="2007-04-25">
		<input type="text" name="name_2_1" value="06:24:00">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_no_whitespace_between_widgets
func TestMultiWidgetNoWhitespaceBetweenWidgets(t *testing.T) {
	w := myMultiWidget(nil, forms.TextInput{}, forms.TextInput{})
	equal(t, w.Render("code", nil, nil), `<input type="text" name="code_0"><input type="text" name="code_1">`)
}

// tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_deepcopy
// has no Go counterpart: a Go widget is a value that a form never copies or
// changes.

// Django renders the form with the template forms_tests/use_fieldset.html;
// the default div layout gives the same markup for the field.
//
// Ported from Django: tests/forms_tests/widget_tests/test_multiwidget.py::MultiWidgetTest.test_fieldset
func TestMultiWidgetFieldset(t *testing.T) {
	form := forms.New(forms.F("field", newComplexField(forms.UseWidget(complexMultiWidget()))))
	equal(t, form.Field("field").UseFieldset(), true)
	assertHTMLEqual(t, form.AsDiv(), `<div><fieldset><legend>Field:</legend>`+
		`<input type="text" name="field_0" required id="id_field_0">`+
		`<select name="field_1" required id="id_field_1" multiple>`+
		`<option value="J">John</option><option value="P">Paul</option>`+
		`<option value="G">George</option><option value="R">Ringo</option></select>`+
		`<input type="text" name="field_2_0" required id="id_field_2_0">`+
		`<input type="text" name="field_2_1" required id="id_field_2_1">`+
		`</fieldset></div>`)
}

// splitDateTime is the value of Django's split date and time widget tests.
var splitDateTime = time.Date(2006, time.January, 10, 7, 30, 0, 0, time.UTC)

// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_render_empty
func TestSplitDateTimeWidgetRenderEmpty(t *testing.T) {
	widgetCheckHTML(t, forms.SplitDateTimeWidget{}, "date", "", nil,
		`<input type="text" name="date_0"><input type="text" name="date_1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_render_none
func TestSplitDateTimeWidgetRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.SplitDateTimeWidget{}, "date", nil, nil,
		`<input type="text" name="date_0"><input type="text" name="date_1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_render_datetime
func TestSplitDateTimeWidgetRenderDatetime(t *testing.T) {
	widgetCheckHTML(t, forms.SplitDateTimeWidget{}, "date", splitDateTime, nil,
		`<input type="text" name="date_0" value="2006-01-10"><input type="text" name="date_1" value="07:30:00">`)
}

// A Go date is a time.Time at midnight and a time of day one on January 1 of
// year 0.
//
// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_render_date_and_time
func TestSplitDateTimeWidgetRenderDateAndTime(t *testing.T) {
	widgetCheckHTML(t, forms.SplitDateTimeWidget{}, "date",
		[]any{date(2006, time.January, 10), time.Date(0, time.January, 1, 7, 30, 0, 0, time.UTC)}, nil,
		`<input type="text" name="date_0" value="2006-01-10"><input type="text" name="date_1" value="07:30:00">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_constructor_attrs
func TestSplitDateTimeWidgetConstructorAttrs(t *testing.T) {
	w := forms.SplitDateTimeWidget{Attrs: forms.Attrs{"class": "pretty"}}
	widgetCheckHTML(t, w, "date", splitDateTime, nil,
		`<input type="text" class="pretty" value="2006-01-10" name="date_0">`+
			`<input type="text" class="pretty" value="07:30:00" name="date_1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_constructor_different_attrs
func TestSplitDateTimeWidgetConstructorDifferentAttrs(t *testing.T) {
	want := `<input type="text" class="foo" value="2006-01-10" name="date_0">` +
		`<input type="text" class="bar" value="07:30:00" name="date_1">`
	widgetCheckHTML(t, forms.SplitDateTimeWidget{DateAttrs: forms.Attrs{"class": "foo"}, TimeAttrs: forms.Attrs{"class": "bar"}},
		"date", splitDateTime, nil, want)
	widgetCheckHTML(t, forms.SplitDateTimeWidget{DateAttrs: forms.Attrs{"class": "foo"}, Attrs: forms.Attrs{"class": "bar"}},
		"date", splitDateTime, nil, want)
	widgetCheckHTML(t, forms.SplitDateTimeWidget{TimeAttrs: forms.Attrs{"class": "bar"}, Attrs: forms.Attrs{"class": "foo"}},
		"date", splitDateTime, nil, want)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_formatting
func TestSplitDateTimeWidgetFormatting(t *testing.T) {
	// DateFormat and TimeFormat change the way a value is displayed.
	w := forms.SplitDateTimeWidget{DateFormat: "%d/%m/%Y", TimeFormat: "%H:%M"}
	want := `<input type="text" name="date_0" value="10/01/2006"><input type="text" name="date_1" value="07:30">`
	widgetCheckHTML(t, w, "date", splitDateTime, nil, want)
	widgetCheckHTML(t, w, "date", splitDateTime, nil, want)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splitdatetimewidget.py::SplitDateTimeWidgetTest.test_fieldset
func TestSplitDateTimeWidgetFieldset(t *testing.T) {
	w := forms.SplitDateTimeWidget{}
	form := forms.New(forms.F("field", forms.NewSplitDateTimeField(forms.UseWidget(w))))
	equal(t, w.UseFieldset(), true)
	assertHTMLEqual(t, form.AsDiv(), `<div><fieldset><legend>Field:</legend><input type="text" `+
		`name="field_0" required id="id_field_0"><input type="text" `+
		`name="field_1" required id="id_field_1"></fieldset></div>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splithiddendatetimewidget.py::SplitHiddenDateTimeWidgetTest.test_render_empty
func TestSplitHiddenDateTimeWidgetRenderEmpty(t *testing.T) {
	widgetCheckHTML(t, forms.SplitHiddenDateTimeWidget{}, "date", "", nil,
		`<input type="hidden" name="date_0"><input type="hidden" name="date_1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splithiddendatetimewidget.py::SplitHiddenDateTimeWidgetTest.test_render_value
func TestSplitHiddenDateTimeWidgetRenderValue(t *testing.T) {
	w := forms.SplitHiddenDateTimeWidget{}
	d := time.Date(2007, time.September, 17, 12, 51, 34, 482548000, time.UTC)
	widgetCheckHTML(t, w, "date", d, nil,
		`<input type="hidden" name="date_0" value="2007-09-17"><input type="hidden" name="date_1" value="12:51:34">`)
	widgetCheckHTML(t, w, "date", time.Date(2007, time.September, 17, 12, 51, 34, 0, time.UTC), nil,
		`<input type="hidden" name="date_0" value="2007-09-17"><input type="hidden" name="date_1" value="12:51:34">`)
	widgetCheckHTML(t, w, "date", time.Date(2007, time.September, 17, 12, 51, 0, 0, time.UTC), nil,
		`<input type="hidden" name="date_0" value="2007-09-17"><input type="hidden" name="date_1" value="12:51:00">`)
}

// FIXME-PORT: tests/forms_tests/widget_tests/test_splithiddendatetimewidget.py::SplitHiddenDateTimeWidgetTest.test_l10n
// needs localized formats, which the Go forms package does not have.

// Ported from Django: tests/forms_tests/widget_tests/test_splithiddendatetimewidget.py::SplitHiddenDateTimeWidgetTest.test_constructor_different_attrs
func TestSplitHiddenDateTimeWidgetConstructorDifferentAttrs(t *testing.T) {
	want := `<input type="hidden" class="foo" value="2006-01-10" name="date_0">` +
		`<input type="hidden" class="bar" value="07:30:00" name="date_1">`
	widgetCheckHTML(t, forms.SplitHiddenDateTimeWidget{DateAttrs: forms.Attrs{"class": "foo"}, TimeAttrs: forms.Attrs{"class": "bar"}},
		"date", splitDateTime, nil, want)
	widgetCheckHTML(t, forms.SplitHiddenDateTimeWidget{DateAttrs: forms.Attrs{"class": "foo"}, Attrs: forms.Attrs{"class": "bar"}},
		"date", splitDateTime, nil, want)
	widgetCheckHTML(t, forms.SplitHiddenDateTimeWidget{TimeAttrs: forms.Attrs{"class": "bar"}, Attrs: forms.Attrs{"class": "foo"}},
		"date", splitDateTime, nil, want)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splithiddendatetimewidget.py::SplitHiddenDateTimeWidgetTest.test_fieldset
func TestSplitHiddenDateTimeWidgetFieldset(t *testing.T) {
	w := forms.SplitHiddenDateTimeWidget{}
	form := forms.New(forms.F("field", forms.NewSplitDateTimeField(forms.UseWidget(w))))
	equal(t, w.UseFieldset(), true)
	assertHTMLEqual(t, form.AsDiv(), `<input type="hidden" name="field_0" id="id_field_0">`+
		`<input type="hidden" name="field_1" id="id_field_1">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_splithiddendatetimewidget.py::SplitHiddenDateTimeWidgetTest.test_fieldset_with_unhidden_field
func TestSplitHiddenDateTimeWidgetFieldsetWithUnhiddenField(t *testing.T) {
	w := forms.SplitHiddenDateTimeWidget{}
	form := forms.New(
		forms.F("hidden_field", forms.NewSplitDateTimeField(forms.UseWidget(w))),
		forms.F("unhidden_field", forms.NewSplitDateTimeField()),
	)
	equal(t, w.UseFieldset(), true)
	assertHTMLEqual(t, form.AsDiv(), `<div><fieldset><legend>Unhidden field:</legend>`+
		`<input type="text" name="unhidden_field_0" required `+
		`id="id_unhidden_field_0"><input type="text" `+
		`name="unhidden_field_1" required id="id_unhidden_field_1">`+
		`</fieldset><input type="hidden" name="hidden_field_0" `+
		`id="id_hidden_field_0"><input type="hidden" `+
		`name="hidden_field_1" id="id_hidden_field_1"></div>`)
}
