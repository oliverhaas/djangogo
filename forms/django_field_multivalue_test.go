package forms_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
)

// newComplexField is the ComplexField of Django's multiwidget and
// multivaluefield tests: a text, a multiple choice of the beatles and a split
// date and time, compressed into one comma-separated string.
func newComplexField(opts ...forms.FieldOption) *forms.MultiValueField {
	return forms.NewMultiValueField([]forms.Field{
		forms.NewCharField(),
		forms.NewMultipleChoiceField(forms.ChoiceList(widgetBeatles)),
		forms.NewSplitDateTimeField(),
	}, func(values []any) (any, error) {
		if len(values) == 0 {
			return nil, nil
		}
		when, _ := values[2].(time.Time)
		return fmt.Sprintf("%s,%s,%s", values[0], strings.Join(values[1].([]string), ""),
			when.Format("2006-01-02 15:04:05")), nil
	}, opts...)
}

// complexFieldForm is the ComplexFieldForm of Django's multivaluefield tests.
func complexFieldForm() *forms.Form {
	return forms.New(forms.F("field1", newComplexField(forms.UseWidget(complexMultiWidget()))))
}

// complexData is the submitted value of a ComplexField in Django's tests.
var complexData = []any{"some text", []string{"J", "P"}, []any{"2007-04-25", "6:24:00"}}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_clean
func TestMultiValueFieldClean(t *testing.T) {
	cleans(t, newComplexField(forms.UseWidget(complexMultiWidget())), complexData, "some text,JP,2007-04-25 06:24:00")
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_clean_disabled_multivalue
func TestMultiValueFieldCleanDisabledMultivalue(t *testing.T) {
	for _, data := range []any{"some text,JP,2007-04-25 06:24:00", complexData} {
		form := forms.New(forms.F("f", newComplexField(forms.Disabled(), forms.UseWidget(complexMultiWidget()))))
		form.Initial = map[string]any{"f": data}
		form = form.Bind(nil)
		equal(t, form.IsValid(), true)
		equal(t, form.Errors().Len(), 0)
		equal(t, form.CleanedData(), map[string]any{"f": "some text,JP,2007-04-25 06:24:00"})
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_bad_choice
func TestMultiValueFieldBadChoice(t *testing.T) {
	rejects(t, newComplexField(forms.UseWidget(complexMultiWidget())),
		[]any{"some text", []string{"X"}, []any{"2007-04-25", "6:24:00"}},
		"Select a valid choice. X is not one of the available choices.")
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_no_value
func TestMultiValueFieldNoValue(t *testing.T) {
	// If insufficient data is provided, nil is substituted.
	rejects(t, newComplexField(forms.UseWidget(complexMultiWidget())),
		[]any{"some text", []string{"JP"}}, "This field is required.")
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_has_changed_no_initial
// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_has_changed_same
// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_has_changed_first_widget
// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_has_changed_last_widget
func TestMultiValueFieldHasChanged(t *testing.T) {
	field := newComplexField(forms.UseWidget(complexMultiWidget()))
	initial := "some text,JP,2007-04-25 06:24:00"
	equal(t, field.HasChanged(nil, complexData), true)
	equal(t, field.HasChanged(initial, complexData), false)
	// The data of the first widget changed.
	equal(t, field.HasChanged(initial, []any{"other text", []string{"J", "P"}, []any{"2007-04-25", "6:24:00"}}), true)
	// The data of the last widget changed: the check does not stop early.
	equal(t, field.HasChanged(initial, []any{"some text", []string{"J", "P"}, []any{"2009-04-25", "11:44:00"}}), true)
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_disabled_has_changed
func TestMultiValueFieldDisabledHasChanged(t *testing.T) {
	f := forms.NewMultiValueField([]forms.Field{forms.NewCharField(), forms.NewCharField()}, nil, forms.Disabled())
	equal(t, f.HasChanged([]any{"x", "x"}, []any{"y", "y"}), false)
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_form_as_table
func TestMultiValueFieldFormAsTable(t *testing.T) {
	assertHTMLEqual(t, complexFieldForm().AsTable(), `
		<tr><th><label>Field1:</label></th>
		<td><input type="text" name="field1_0" id="id_field1_0" required>
		<select multiple name="field1_1" id="id_field1_1" required>
		<option value="J">John</option>
		<option value="P">Paul</option>
		<option value="G">George</option>
		<option value="R">Ringo</option>
		</select>
		<input type="text" name="field1_2_0" id="id_field1_2_0" required>
		<input type="text" name="field1_2_1" id="id_field1_2_1" required></td></tr>`)
}

// complexFormData is the submitted data of Django's ComplexFieldForm tests.
func complexFormData() *forms.Form {
	return complexFieldForm().Bind(vals("field1_0", "some text", "field1_1", "J", "field1_1", "P",
		"field1_2_0", "2007-04-25", "field1_2_1", "06:24:00"))
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_form_as_table_data
func TestMultiValueFieldFormAsTableData(t *testing.T) {
	assertHTMLEqual(t, complexFormData().AsTable(), `
		<tr><th><label>Field1:</label></th>
		<td><input type="text" name="field1_0" value="some text" id="id_field1_0" required>
		<select multiple name="field1_1" id="id_field1_1" required>
		<option value="J" selected>John</option>
		<option value="P" selected>Paul</option>
		<option value="G">George</option>
		<option value="R">Ringo</option>
		</select>
		<input type="text" name="field1_2_0" value="2007-04-25" id="id_field1_2_0" required>
		<input type="text" name="field1_2_1" value="06:24:00" id="id_field1_2_1" required></td></tr>`)
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_form_cleaned_data
func TestMultiValueFieldFormCleanedData(t *testing.T) {
	form := complexFormData()
	form.IsValid()
	equal(t, form.CleanedData()["field1"], any("some text,JP,2007-04-25 06:24:00"))
}

// joinCompress joins the cleaned strings with commas, the compress of
// Django's PartiallyRequiredField.
func joinCompress(values []any) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i], _ = v.(string)
	}
	return strings.Join(parts, ","), nil
}

// Ported from Django: tests/forms_tests/field_tests/test_multivaluefield.py::MultiValueFieldTest.test_render_required_attributes
func TestMultiValueFieldRenderRequiredAttributes(t *testing.T) {
	partiallyRequiredForm := func(data ...string) *forms.Form {
		return forms.New(forms.F("f", forms.NewMultiValueField(
			[]forms.Field{forms.NewCharField(forms.Required(true)), forms.NewCharField(forms.Required(false))},
			joinCompress,
			forms.Required(true),
			forms.RequireAllFields(false),
			forms.UseWidget(forms.MultiWidget{Widgets: []forms.Widget{forms.TextInput{}, forms.TextInput{}}}),
		))).Bind(vals(data...))
	}
	form := partiallyRequiredForm("f_0", "Hello", "f_1", "")
	equal(t, form.IsValid(), true)
	djangotest.AssertInHTML(t, `<input type="text" name="f_0" value="Hello" required id="id_f_0">`, form.AsP())
	djangotest.AssertInHTML(t, `<input type="text" name="f_1" id="id_f_1">`, form.AsP())
	form = partiallyRequiredForm("f_0", "", "f_1", "")
	equal(t, form.IsValid(), false)
}

// Ported from Django: tests/forms_tests/field_tests/test_splitdatetimefield.py::SplitDateTimeFieldTest.test_splitdatetimefield_1
func TestSplitDateTimeField1(t *testing.T) {
	f := forms.NewSplitDateTimeField()
	if _, ok := f.Widget.(forms.SplitDateTimeWidget); !ok {
		t.Errorf("widget is %T, want forms.SplitDateTimeWidget", f.Widget)
	}
	cleans(t, f, []any{date(2006, time.January, 10), time.Date(0, time.January, 1, 7, 30, 0, 0, time.UTC)},
		time.Date(2006, time.January, 10, 7, 30, 0, 0, time.UTC))
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, "", "This field is required.")
	rejects(t, f, "hello", "Enter a list of values.")
	rejects(t, f, []any{"hello", "there"}, "Enter a valid date.", "Enter a valid time.")
	rejects(t, f, []any{"2006-01-10", "there"}, "Enter a valid time.")
	rejects(t, f, []any{"hello", "07:30"}, "Enter a valid date.")
}

// Ported from Django: tests/forms_tests/field_tests/test_splitdatetimefield.py::SplitDateTimeFieldTest.test_splitdatetimefield_2
func TestSplitDateTimeField2(t *testing.T) {
	f := forms.NewSplitDateTimeField(forms.Required(false))
	want := time.Date(2006, time.January, 10, 7, 30, 0, 0, time.UTC)
	cleans(t, f, []any{date(2006, time.January, 10), time.Date(0, time.January, 1, 7, 30, 0, 0, time.UTC)}, want)
	cleans(t, f, []any{"2006-01-10", "07:30"}, want)
	cleans(t, f, nil, nil)
	cleans(t, f, "", nil)
	cleans(t, f, []any{""}, nil)
	cleans(t, f, []any{"", ""}, nil)
	rejects(t, f, "hello", "Enter a list of values.")
	rejects(t, f, []any{"hello", "there"}, "Enter a valid date.", "Enter a valid time.")
	rejects(t, f, []any{"2006-01-10", "there"}, "Enter a valid time.")
	rejects(t, f, []any{"hello", "07:30"}, "Enter a valid date.")
	rejects(t, f, []any{"2006-01-10", ""}, "Enter a valid time.")
	rejects(t, f, []any{"2006-01-10"}, "Enter a valid time.")
	rejects(t, f, []any{"", "07:30"}, "Enter a valid date.")
}

// Ported from Django: tests/forms_tests/field_tests/test_splitdatetimefield.py::SplitDateTimeFieldTest.test_splitdatetimefield_changed
func TestSplitDateTimeFieldChanged(t *testing.T) {
	f := forms.NewSplitDateTimeField(forms.InputDateFormats("%d/%m/%Y"))
	initial := time.Date(2008, time.May, 6, 12, 40, 0, 0, time.UTC)
	equal(t, f.HasChanged([]any{"11/01/2012", "09:18:15"}, []any{"11/01/2012", "09:18:15"}), false)
	equal(t, f.HasChanged(initial, []any{"2008-05-06", "12:40:00"}), true)
	equal(t, f.HasChanged(initial, []any{"06/05/2008", "12:40"}), false)
	equal(t, f.HasChanged(initial, []any{"06/05/2008", "12:41"}), true)
}

// Ported from Django: tests/forms_tests/field_tests/test_splitdatetimefield.py::SplitDateTimeFieldTest.test_form_as_table
func TestSplitDateTimeFieldFormAsTable(t *testing.T) {
	f := forms.New(forms.F("datetime", forms.NewSplitDateTimeField()))
	assertHTMLEqual(t, f.AsTable(), `<tr><th><label>Datetime:</label></th><td>`+
		`<input type="text" name="datetime_0" required id="id_datetime_0">`+
		`<input type="text" name="datetime_1" required id="id_datetime_1">`+
		`</td></tr>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_label_split_datetime_not_displayed
func TestLabelSplitDatetimeNotDisplayed(t *testing.T) {
	form := forms.New(forms.F("happened_at", forms.NewSplitDateTimeField(forms.UseWidget(forms.SplitHiddenDateTimeWidget{}))))
	assertHTMLEqual(t, form.AsUL(), `<input type="hidden" name="happened_at_0" id="id_happened_at_0">`+
		`<input type="hidden" name="happened_at_1" id="id_happened_at_1">`)
}

// nameField is the NameField of Django's test_multivalue_field_validation: a
// first and a last name joined by a space.
func nameField(opts ...forms.FieldOption) *forms.MultiValueField {
	return forms.NewMultiValueField([]forms.Field{
		forms.NewCharField(forms.Label("First name"), forms.MaxLength(10)),
		forms.NewCharField(forms.Label("Last name"), forms.MaxLength(10)),
	}, func(values []any) (any, error) {
		parts := make([]string, len(values))
		for i, v := range values {
			parts[i], _ = v.(string)
		}
		return strings.Join(parts, " "), nil
	}, opts...)
}

// Django's form gets a list under the field's own name, which its default
// TextInput passes on; the Go form reads the values of two text inputs.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multivalue_field_validation
func TestMultivalueFieldValidation(t *testing.T) {
	badNames := func(value any) error {
		if value == "bad value" {
			return forms.NewValidationError("bad value not allowed", "")
		}
		return nil
	}
	nameForm := func(first, last string) *forms.Form {
		widget := forms.MultiWidget{Widgets: []forms.Widget{forms.TextInput{}, forms.TextInput{}}}
		return forms.New(forms.F("name", nameField(forms.Validators(badNames), forms.UseWidget(widget)))).
			Bind(vals("name_0", first, "name_1", last))
	}
	form := nameForm("bad", "value")
	equal(t, form.IsValid(), false)
	equal(t, errorsOf(form, "name"), []string{"bad value not allowed"})
	form = nameForm("should be overly", "long for the field names")
	equal(t, form.IsValid(), false)
	equal(t, errorsOf(form, "name"), []string{
		"Ensure this value has at most 10 characters (it has 16).",
		"Ensure this value has at most 10 characters (it has 24).",
	})
	form = nameForm("fname", "lname")
	equal(t, form.IsValid(), true)
	equal(t, form.CleanedData(), map[string]any{"name": "fname lname"})
}

// tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multivalue_deep_copy
// has no Go counterpart: a form shares its field definitions and never copies
// them.

// Django's form gets a list under the field's own name; the Go form reads the
// values of the subwidgets.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multivalue_initial_data
func TestMultivalueInitialData(t *testing.T) {
	// Invalid initial data does not break ChangedData.
	form := forms.New(forms.F("date_age", forms.NewMultiValueField(
		[]forms.Field{forms.NewDateField(forms.Label("Date")), forms.NewIntegerField(forms.Label("Age"))}, nil,
		forms.UseWidget(forms.MultiWidget{Widgets: []forms.Widget{forms.DateInput{}, forms.NumberInput{}}}))))
	form.Initial = map[string]any{"date_age": []any{"200-10-10", 14}}
	form = form.Bind(vals("date_age_0", "1998-12-06", "date_age_1", "16"))
	equal(t, form.HasChanged(), true)
}

// phoneField is the PhoneField of Django's test_multivalue_optional_subfields.
func phoneField(opts ...forms.FieldOption) *forms.MultiValueField {
	return forms.NewMultiValueField([]forms.Field{
		forms.NewCharField(forms.Label("Country Code"), forms.Validators(
			forms.RegexValidator(regexp.MustCompile(`^\+[0-9]{1,2}$`), "Enter a valid country code.", ""))),
		forms.NewCharField(forms.Label("Phone Number")),
		forms.NewCharField(forms.Label("Extension"), forms.ErrorMessages(map[string]string{"incomplete": "Enter an extension."})),
		forms.NewCharField(forms.Label("Label"), forms.Required(false), forms.HelpText("E.g. home, work.")),
	}, func(values []any) (any, error) {
		if len(values) == 0 {
			return nil, nil
		}
		return fmt.Sprintf("%s.%s ext. %s (label: %s)", values...), nil
	}, opts...)
}

// Django checks that the message holds the given text; the Go test lists all
// the messages, so the cases with ["+61"] also have the incomplete error of
// the extension.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multivalue_optional_subfields
func TestMultivalueOptionalSubfields(t *testing.T) {
	// An empty value for any field raises a required error on a required
	// MultiValueField.
	f := phoneField()
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, []any{}, "This field is required.")
	rejects(t, f, []any{"+61"}, "This field is required.")
	rejects(t, f, []any{"+61", "287654321", "123"}, "This field is required.")
	cleans(t, f, []any{"+61", "287654321", "123", "Home"}, "+61.287654321 ext. 123 (label: Home)")
	rejects(t, f, []any{"61", "287654321", "123", "Home"}, "Enter a valid country code.")

	// Empty values for fields do not raise a required error on an optional
	// MultiValueField.
	f = phoneField(forms.Required(false))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
	cleans(t, f, []any{}, nil)
	cleans(t, f, []any{"+61"}, "+61. ext.  (label: )")
	cleans(t, f, []any{"+61", "287654321", "123"}, "+61.287654321 ext. 123 (label: )")
	cleans(t, f, []any{"+61", "287654321", "123", "Home"}, "+61.287654321 ext. 123 (label: Home)")
	rejects(t, f, []any{"61", "287654321", "123", "Home"}, "Enter a valid country code.")

	// For a required MultiValueField with RequireAllFields(false), a required
	// error is only raised if all fields are empty. Fields can individually
	// be required or optional. An empty value for any required field raises
	// an incomplete error.
	f = phoneField(forms.RequireAllFields(false))
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, []any{}, "This field is required.")
	rejects(t, f, []any{"+61"}, "Enter a complete value.", "Enter an extension.")
	cleans(t, f, []any{"+61", "287654321", "123"}, "+61.287654321 ext. 123 (label: )")
	rejects(t, f, []any{"", "", "", "Home"}, "Enter a complete value.", "Enter an extension.")
	rejects(t, f, []any{"61", "287654321", "123", "Home"}, "Enter a valid country code.")

	// For an optional MultiValueField with RequireAllFields(false), there is
	// no required error but there are incomplete errors.
	f = phoneField(forms.Required(false), forms.RequireAllFields(false))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
	cleans(t, f, []any{}, nil)
	rejects(t, f, []any{"+61"}, "Enter a complete value.", "Enter an extension.")
	cleans(t, f, []any{"+61", "287654321", "123"}, "+61.287654321 ext. 123 (label: )")
	rejects(t, f, []any{"", "", "", "Home"}, "Enter a complete value.", "Enter an extension.")
	rejects(t, f, []any{"61", "287654321", "123", "Home"}, "Enter a valid country code.")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multivalue_optional_subfields_rendering
func TestMultivalueOptionalSubfieldsRendering(t *testing.T) {
	phone := func(opts ...forms.FieldOption) *forms.MultiValueField {
		widget := forms.MultiWidget{
			Widgets:    []forms.Widget{forms.TextInput{}, forms.TextInput{}},
			Decompress: func(any) []any { return []any{nil, nil} },
		}
		return forms.NewMultiValueField([]forms.Field{forms.NewCharField(), forms.NewCharField(forms.Required(false))},
			nil, append([]forms.FieldOption{forms.UseWidget(widget)}, opts...)...)
	}
	form := withoutAutoID(forms.New(
		forms.F("phone1", phone()),
		forms.F("phone2", phone(forms.Required(false))),
		forms.F("phone3", phone(forms.RequireAllFields(false))),
		forms.F("phone4", phone(forms.Required(false), forms.RequireAllFields(false))),
	))
	assertHTMLEqual(t, form.AsP(), `
		<p>Phone1:<input type="text" name="phone1_0" required>
		<input type="text" name="phone1_1" required></p>
		<p>Phone2:<input type="text" name="phone2_0">
		<input type="text" name="phone2_1"></p>
		<p>Phone3:<input type="text" name="phone3_0" required>
		<input type="text" name="phone3_1"></p>
		<p>Phone4:<input type="text" name="phone4_0">
		<input type="text" name="phone4_1"></p>`)
}
