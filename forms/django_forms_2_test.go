package forms_test

import (
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// userRegForm is the UserRegistration form of Django's forms tests: a
// username of at most 10 characters (with the given extra options) and a
// password rendered as a password input.
func userRegForm(usernameOpts ...forms.FieldOption) *forms.Form {
	return forms.New(
		forms.F("username", forms.NewCharField(append([]forms.FieldOption{forms.MaxLength(10)}, usernameOpts...)...)),
		forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
	)
}

// fieldDef returns the definition of a field of f and panics when the
// form has no such field.
func fieldDef(f *forms.Form, name string) forms.Field {
	field, ok := f.FieldDef(name)
	if !ok {
		panic("no field " + name)
	}
	return field
}

// withoutAutoID returns f with ids and label tags switched off, Django's
// auto_id=False.
func withoutAutoID(f *forms.Form) *forms.Form {
	f.AutoID = ""
	return f
}

// textFieldsForm returns a form of required CharFields with the given names.
func textFieldsForm(names ...string) *forms.Form {
	f := forms.New()
	for _, name := range names {
		f.AddField(name, forms.NewCharField())
	}
	return f
}

// optionsChoices are the choices of the options field of the callable initial
// tests.
func optionsChoices() forms.FieldOption {
	return forms.Choices("f", "foo", "b", "bar", "w", "whiz")
}

// pedanticField is the PedanticField of Django's test_changed_data: a field
// whose conversion always fails.
type pedanticField struct{ forms.BaseField }

// newPedanticField returns a pedanticField configured with opts.
func newPedanticField(opts ...forms.FieldOption) *pedanticField {
	f := &pedanticField{}
	f.Init(f, forms.TextInput{}, nil, opts...)
	return f
}

// ToGo always fails with "Whatever".
func (*pedanticField) ToGo(any) (any, error) {
	return nil, forms.NewValidationError("Whatever", "")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_empty_querydict_args
//
// The files argument is left out: the package has no file uploads. Bind keeps
// the map it is given, so the data is the same map.
func TestEmptyQuerydictArgs(t *testing.T) {
	data := url.Values{}
	p := personForm().Bind(data)
	if reflect.ValueOf(p.Data()).Pointer() != reflect.ValueOf(data).Pointer() {
		t.Error("Data() is not the map passed to Bind")
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_unicode_values
//
// Django's expected as_table and as_div output lacks a space between the
// value and id attributes of last_name; HTML parsing makes it irrelevant and
// the space is added here.
func TestUnicodeValues(t *testing.T) {
	p := personForm().Bind(vals(
		"first_name", "John",
		"last_name", "ŠĐĆŽćžšđ",
		"birthday", "1940-10-9"))
	assertHTMLEqual(t, p.AsTable(),
		`<tr><th><label for="id_first_name">First name:</label></th><td>`+
			`<input type="text" name="first_name" value="John" id="id_first_name" required></td></tr>`+"\n"+
			`<tr><th><label for="id_last_name">Last name:</label></th><td><input type="text" name="last_name" `+
			`value="ŠĐĆŽćžšđ" id="id_last_name" required></td></tr>`+"\n"+
			`<tr><th><label for="id_birthday">Birthday:</label></th><td>`+
			`<input type="text" name="birthday" value="1940-10-9" id="id_birthday" required></td></tr>`)
	assertHTMLEqual(t, p.AsUL(),
		`<li><label for="id_first_name">First name:</label> `+
			`<input type="text" name="first_name" value="John" id="id_first_name" required></li>`+"\n"+
			`<li><label for="id_last_name">Last name:</label> <input type="text" name="last_name" `+
			`value="ŠĐĆŽćžšđ" id="id_last_name" required></li>`+"\n"+
			`<li><label for="id_birthday">Birthday:</label> `+
			`<input type="text" name="birthday" value="1940-10-9" id="id_birthday" required></li>`)
	assertHTMLEqual(t, p.AsP(),
		`<p><label for="id_first_name">First name:</label> `+
			`<input type="text" name="first_name" value="John" id="id_first_name" required></p>`+"\n"+
			`<p><label for="id_last_name">Last name:</label> <input type="text" name="last_name" `+
			`value="ŠĐĆŽćžšđ" id="id_last_name" required></p>`+"\n"+
			`<p><label for="id_birthday">Birthday:</label> `+
			`<input type="text" name="birthday" value="1940-10-9" id="id_birthday" required></p>`)
	assertHTMLEqual(t, p.AsDiv(),
		`<div><label for="id_first_name">First name:</label>`+
			`<input type="text" name="first_name" value="John" id="id_first_name" `+
			`required></div><div><label for="id_last_name">Last name:</label>`+
			`<input type="text" name="last_name" `+
			`value="ŠĐĆŽćžšđ" `+
			`id="id_last_name" required></div><div><label for="id_birthday">`+
			`Birthday:</label><input type="text" name="birthday" value="1940-10-9" `+
			`id="id_birthday" required></div>`)

	p = personForm().Bind(vals("last_name", "Lennon"))
	equal(t, errorsOf(p, "first_name"), []string{"This field is required."})
	equal(t, errorsOf(p, "birthday"), []string{"This field is required."})
	equal(t, p.IsValid(), false)
	equal(t, p.Errors().Messages(), map[string][]string{
		"birthday":   {"This field is required."},
		"first_name": {"This field is required."},
	})
	equal(t, p.CleanedData(), map[string]any{"last_name": "Lennon"})
	equal(t, p.Field("first_name").Errors().Messages(), []string{"This field is required."})
	assertHTMLEqual(t, p.Field("first_name").Errors().AsUL(),
		`<ul class="errorlist" id="id_first_name_error"><li>This field is required.</li></ul>`)
	equal(t, p.Field("first_name").Errors().AsText(), "* This field is required.")

	p = personForm()
	assertHTMLEqual(t, p.Field("first_name").String(), `<input type="text" name="first_name" id="id_first_name" required>`)
	assertHTMLEqual(t, p.Field("last_name").String(), `<input type="text" name="last_name" id="id_last_name" required>`)
	assertHTMLEqual(t, p.Field("birthday").String(), `<input type="text" name="birthday" id="id_birthday" required>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_blank_choice_dash
//
// get_blank_choice_label() is forms.EmptyChoiceLabel; the deprecation
// warning asserted at the end is Python-specific and left out.
func TestBlankChoiceDash(t *testing.T) {
	f := forms.New(forms.F("somechoices", forms.NewChoiceField(
		forms.Choices("0", forms.EmptyChoiceLabel, "1", "Test 1", "2", "Test 2"))))
	assertHTMLEqual(t, f.AsP(), `
            <p>
                <label for="id_somechoices">Somechoices:</label>
                <select name="somechoices" id="id_somechoices">
                <option value="0">---------</option>
                <option value="1">Test 1</option>
                <option value="2">Test 2</option></select>
            </p>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multiple_choice_checkbox
func TestMultipleChoiceCheckbox(t *testing.T) {
	newForm := func() *forms.Form {
		return withoutAutoID(songForm(forms.NewMultipleChoiceField(composerChoices(),
			forms.UseWidget(forms.CheckboxSelectMultiple{}))))
	}
	assertHTMLEqual(t, newForm().Field("composers").String(), `
            <div>
            <div><label><input type="checkbox" name="composers" value="J">
            John Lennon</label></div>
            <div><label><input type="checkbox" name="composers" value="P">
            Paul McCartney</label></div>
            </div>`)
	assertHTMLEqual(t, newForm().Bind(vals("composers", "J")).Field("composers").String(), `
            <div>
            <div><label><input checked type="checkbox" name="composers" value="J">
            John Lennon</label></div>
            <div><label><input type="checkbox" name="composers" value="P">
            Paul McCartney</label></div>
            </div>`)
	assertHTMLEqual(t, newForm().Bind(vals("composers", "J", "composers", "P")).Field("composers").String(), `
            <div>
            <div><label><input checked type="checkbox" name="composers" value="J">
            John Lennon</label></div>
            <div><label><input checked type="checkbox" name="composers" value="P">
            Paul McCartney</label></div>
            </div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_checkbox_auto_id
func TestCheckboxAutoID(t *testing.T) {
	f := songForm(forms.NewMultipleChoiceField(composerChoices(), forms.UseWidget(forms.CheckboxSelectMultiple{})))
	f.AutoID = "%s_id"
	assertHTMLEqual(t, f.Field("composers").String(), `
            <div id="composers_id">
            <div><label for="composers_id_0">
            <input type="checkbox" name="composers" value="J" id="composers_id_0">
            John Lennon</label></div>
            <div><label for="composers_id_1">
            <input type="checkbox" name="composers" value="P" id="composers_id_1">
            Paul McCartney</label></div>
            </div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multiple_choice_list_data
//
// A Go form takes url.Values, which holds a list for every key; it covers the
// dict, QueryDict and MultiValueDict inputs. The MultiValueDictLike duck
// typing check has no Go counterpart and is left out, but its single-value
// case is run on url.Values.
func TestMultipleChoiceListData(t *testing.T) {
	newForm := func() *forms.Form {
		return songForm(forms.NewMultipleChoiceField(composerChoices(), forms.UseWidget(forms.CheckboxSelectMultiple{})))
	}
	f := newForm().Bind(vals("name", "Yesterday", "composers", "J", "composers", "P"))
	equal(t, f.Errors().Len(), 0)
	f = newForm().Bind(url.Values{"name": {"Yesterday"}, "composers": {"J", "P"}})
	equal(t, f.Errors().Len(), 0)
	f = newForm().Bind(vals("name", "Yesterday", "composers", "J"))
	equal(t, f.Errors().Len(), 0)
	equal(t, f.CleanedData()["composers"], any([]string{"J"}))
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multiple_hidden
//
// The MultiValueDictLike duck typing checks have no Go counterpart; their
// single-value case is run on url.Values.
func TestMultipleHidden(t *testing.T) {
	checkboxForm := func() *forms.Form {
		return withoutAutoID(songForm(forms.NewMultipleChoiceField(composerChoices(),
			forms.UseWidget(forms.CheckboxSelectMultiple{}))))
	}
	hiddenForm := withoutAutoID(songForm(forms.NewMultipleChoiceField(composerChoices(),
		forms.UseWidget(forms.MultipleHiddenInput{}))))

	f := hiddenForm.Bind(url.Values{"name": {"Yesterday"}, "composers": {"J", "P"}})
	assertHTMLEqual(t, f.AsUL(), `<li>Name: <input type="text" name="name" value="Yesterday" required>
<input type="hidden" name="composers" value="J">
<input type="hidden" name="composers" value="P"></li>`)

	f = checkboxForm().Bind(vals("name", "Yesterday"))
	equal(t, errorsOf(f, "composers"), []string{"This field is required."})
	f = checkboxForm().Bind(vals("name", "Yesterday", "composers", "J"))
	equal(t, f.Errors().Len(), 0)
	equal(t, f.CleanedData()["composers"], any([]string{"J"}))
	equal(t, f.CleanedData()["name"], any("Yesterday"))
	f = checkboxForm().Bind(vals("name", "Yesterday", "composers", "J", "composers", "P"))
	equal(t, f.Errors().Len(), 0)
	equal(t, f.CleanedData()["composers"], any([]string{"J", "P"}))
	equal(t, f.CleanedData()["name"], any("Yesterday"))
	f = checkboxForm().Bind(vals("name", "Yesterday", "composers", "J"))
	equal(t, f.Errors().Len(), 0)
	equal(t, f.CleanedData()["composers"], any([]string{"J"}))
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_update_error_dict
//
// ValidationError.update_error_dict and the private _errors attribute have no
// Go counterpart. The same errors are added with Form.AddError, which keeps
// every error of a field and the order of addition.
func TestUpdateErrorDict(t *testing.T) {
	f := forms.New(forms.F("code", forms.NewCharField(forms.MaxLength(10))))
	f.CleanWith(func(form *forms.Form) error {
		form.AddError("code", forms.NewValidationError("Code error 1.", ""))
		form.AddError("code", forms.NewValidationError("Code error 2.", ""))
		form.AddError("code", forms.NewErrorList("Code error 3.").AsData()[0])
		form.AddError("", "Non-field error 1.")
		form.AddError("", forms.ValidationErrors(forms.NewValidationError("Non-field error 2.", "")))
		return nil
	})
	form := f.Bind(vals("code", "hello"))
	equal(t, form.IsValid(), false)
	equal(t, form.Errors().Messages(), map[string][]string{
		"code":               {"Code error 1.", "Code error 2.", "Code error 3."},
		forms.NonFieldErrors: {"Non-field error 1.", "Non-field error 2."},
	})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_dynamic_construction
//
// A Go form is built per use, so no state is shared between two forms. The
// subclassing and __init__ overriding of the Django test become constructor
// functions that call AddField and change the field definitions.
func TestDynamicConstruction(t *testing.T) {
	p := withoutAutoID(textFieldsForm("first_name", "last_name"))
	p.AddField("birthday", forms.NewDateField())
	assertHTMLEqual(t, p.AsTable(), `
            <tr><th>First name:</th><td>
            <input type="text" name="first_name" required></td></tr>
            <tr><th>Last name:</th><td>
            <input type="text" name="last_name" required></td></tr>
            <tr><th>Birthday:</th><td>
            <input type="text" name="birthday" required></td></tr>`)

	myForm := func(defaults []string, fieldList ...string) *forms.Form {
		f := withoutAutoID(textFieldsForm(defaults...))
		for _, name := range fieldList {
			f.AddField(name, forms.NewCharField())
		}
		return f
	}
	assertHTMLEqual(t, myForm(nil, "field1", "field2").AsTable(), `
            <tr><th>Field1:</th><td><input type="text" name="field1" required></td></tr>
            <tr><th>Field2:</th><td><input type="text" name="field2" required></td></tr>`)
	assertHTMLEqual(t, myForm(nil, "field3", "field4").AsTable(), `
            <tr><th>Field3:</th><td><input type="text" name="field3" required></td></tr>
            <tr><th>Field4:</th><td><input type="text" name="field4" required></td></tr>`)
	defaults := []string{"default_field_1", "default_field_2"}
	assertHTMLEqual(t, myForm(defaults, "field1", "field2").AsTable(), `
            <tr><th>Default field 1:</th><td>
            <input type="text" name="default_field_1" required></td></tr>
            <tr><th>Default field 2:</th><td>
            <input type="text" name="default_field_2" required></td></tr>
            <tr><th>Field1:</th><td><input type="text" name="field1" required></td></tr>
            <tr><th>Field2:</th><td><input type="text" name="field2" required></td></tr>`)
	assertHTMLEqual(t, myForm(defaults, "field3", "field4").AsTable(), `
            <tr><th>Default field 1:</th><td>
            <input type="text" name="default_field_1" required></td></tr>
            <tr><th>Default field 2:</th><td>
            <input type="text" name="default_field_2" required></td></tr>
            <tr><th>Field3:</th><td><input type="text" name="field3" required></td></tr>
            <tr><th>Field4:</th><td><input type="text" name="field4" required></td></tr>`)

	// Changes to field attributes do not persist from one form to the next.
	namesForm := func(namesRequired bool) *forms.Form {
		f := forms.New(
			forms.F("first_name", forms.NewCharField(forms.Required(false))),
			forms.F("last_name", forms.NewCharField(forms.Required(false))),
		)
		if namesRequired {
			for _, name := range []string{"first_name", "last_name"} {
				base := fieldDef(f, name).Base()
				base.Required = true
				base.Widget = forms.TextInput{Attrs: forms.Attrs{"class": "required"}}
			}
		}
		return f
	}
	for _, required := range []bool{false, true, false} {
		f := namesForm(required)
		first, last := fieldDef(f, "first_name").Base(), fieldDef(f, "last_name").Base()
		equal(t, first.Required, required)
		equal(t, last.Required, required)
		wantAttrs := forms.Attrs(nil)
		if required {
			wantAttrs = forms.Attrs{"class": "required"}
		}
		equal(t, first.Widget.Attributes(), wantAttrs)
		equal(t, last.Widget.Attributes(), wantAttrs)
	}

	lengthForm := func(nameMaxLength int) *forms.Form {
		f := forms.New(
			forms.F("first_name", forms.NewCharField(forms.MaxLength(30))),
			forms.F("last_name", forms.NewCharField(forms.MaxLength(30))),
		)
		if nameMaxLength > 0 {
			fieldDef(f, "first_name").(*forms.CharField).MaxLength = nameMaxLength
			fieldDef(f, "last_name").(*forms.CharField).MaxLength = nameMaxLength
		}
		return f
	}
	for _, c := range []struct{ arg, want int }{{0, 30}, {20, 20}, {0, 30}} {
		f := lengthForm(c.arg)
		equal(t, fieldDef(f, "first_name").(*forms.CharField).MaxLength, c.want)
		equal(t, fieldDef(f, "last_name").(*forms.CharField).MaxLength, c.want)
	}

	// Choices do not persist from one form to the next (Django ticket 15127).
	genderForm := func(allowUnspecified bool) *forms.Form {
		f := forms.New(
			forms.F("first_name", forms.NewCharField(forms.Required(false))),
			forms.F("last_name", forms.NewCharField(forms.Required(false))),
			forms.F("gender", forms.NewChoiceField(forms.Choices("f", "Female", "m", "Male"))),
		)
		if allowUnspecified {
			gender := fieldDef(f, "gender").(*forms.ChoiceField)
			gender.Choices = append(gender.Choices, forms.Choice{Value: "u", Label: "Unspecified"})
		}
		return f
	}
	female, male := forms.Choice{Value: "f", Label: "Female"}, forms.Choice{Value: "m", Label: "Male"}
	equal(t, fieldDef(genderForm(false), "gender").(*forms.ChoiceField).Choices, []forms.Choice{female, male})
	equal(t, fieldDef(genderForm(true), "gender").(*forms.ChoiceField).Choices,
		[]forms.Choice{female, male, {Value: "u", Label: "Unspecified"}})
	equal(t, fieldDef(genderForm(false), "gender").(*forms.ChoiceField).Choices, []forms.Choice{female, male})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_validators_independence
//
// The validators of a field are held per field definition, and a form
// constructor builds new definitions, so replacing a validator of one form
// leaves the other untouched. Validators are functions and compare by their
// code pointers.
func TestValidatorsIndependence(t *testing.T) {
	newForm := func() *forms.Form {
		return forms.New(forms.F("myfield", forms.NewCharField(forms.MaxLength(25))))
	}
	f1, f2 := newForm(), newForm()
	fieldDef(f1, "myfield").Base().Validators[0] = forms.MaxValueValidator(12)
	v1 := reflect.ValueOf(fieldDef(f1, "myfield").Base().Validators[0]).Pointer()
	v2 := reflect.ValueOf(fieldDef(f2, "myfield").Base().Validators[0]).Pointer()
	if v1 == v2 {
		t.Error("the validator of f2 changed with the validator of f1")
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_hidden_widget
func TestHiddenWidget(t *testing.T) {
	newForm := func() *forms.Form {
		return forms.New(
			forms.F("first_name", forms.NewCharField()),
			forms.F("last_name", forms.NewCharField()),
			forms.F("hidden_text", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}))),
			forms.F("birthday", forms.NewDateField()),
		)
	}
	p := withoutAutoID(newForm())
	assertHTMLEqual(t, p.AsTable(), `
            <tr><th>First name:</th><td><input type="text" name="first_name" required>
            </td></tr>
            <tr><th>Last name:</th><td><input type="text" name="last_name" required>
            </td></tr>
            <tr><th>Birthday:</th>
            <td><input type="text" name="birthday" required>
            <input type="hidden" name="hidden_text"></td></tr>`)
	assertHTMLEqual(t, p.AsUL(), `
            <li>First name: <input type="text" name="first_name" required></li>
            <li>Last name: <input type="text" name="last_name" required></li>
            <li>Birthday: <input type="text" name="birthday" required>
            <input type="hidden" name="hidden_text"></li>`)
	assertHTMLEqual(t, p.AsP(), `
            <p>First name: <input type="text" name="first_name" required></p>
            <p>Last name: <input type="text" name="last_name" required></p>
            <p>Birthday: <input type="text" name="birthday" required>
            <input type="hidden" name="hidden_text"></p>`)
	assertHTMLEqual(t, p.AsDiv(),
		`<div>First name: <input type="text" name="first_name" required></div>`+
			`<div>Last name: <input type="text" name="last_name" required></div><div>`+
			`Birthday: <input type="text" name="birthday" required><input `+
			`type="hidden" name="hidden_text"></div>`)

	// With an auto id, a hidden input still gets an id but no label.
	p = newForm()
	assertHTMLEqual(t, p.AsTable(), `<tr><th><label for="id_first_name">First name:</label></th><td>
<input type="text" name="first_name" id="id_first_name" required></td></tr>
<tr><th><label for="id_last_name">Last name:</label></th><td>
<input type="text" name="last_name" id="id_last_name" required></td></tr>
<tr><th><label for="id_birthday">Birthday:</label></th><td>
<input type="text" name="birthday" id="id_birthday" required>
<input type="hidden" name="hidden_text" id="id_hidden_text"></td></tr>`)
	assertHTMLEqual(t, p.AsUL(), `<li><label for="id_first_name">First name:</label>
<input type="text" name="first_name" id="id_first_name" required></li>
<li><label for="id_last_name">Last name:</label>
<input type="text" name="last_name" id="id_last_name" required></li>
<li><label for="id_birthday">Birthday:</label>
<input type="text" name="birthday" id="id_birthday" required>
<input type="hidden" name="hidden_text" id="id_hidden_text"></li>`)
	assertHTMLEqual(t, p.AsP(), `<p><label for="id_first_name">First name:</label>
<input type="text" name="first_name" id="id_first_name" required></p>
<p><label for="id_last_name">Last name:</label>
<input type="text" name="last_name" id="id_last_name" required></p>
<p><label for="id_birthday">Birthday:</label>
<input type="text" name="birthday" id="id_birthday" required>
<input type="hidden" name="hidden_text" id="id_hidden_text"></p>`)
	assertHTMLEqual(t, p.AsDiv(),
		`<div><label for="id_first_name">First name:</label><input type="text" `+
			`name="first_name" id="id_first_name" required></div><div><label `+
			`for="id_last_name">Last name:</label><input type="text" name="last_name" `+
			`id="id_last_name" required></div><div><label for="id_birthday">Birthday:`+
			`</label><input type="text" name="birthday" id="id_birthday" required>`+
			`<input type="hidden" name="hidden_text" id="id_hidden_text"></div>`)

	// The errors of a hidden field are shown at the top with a prefix.
	p = withoutAutoID(newForm()).Bind(vals("first_name", "John", "last_name", "Lennon", "birthday", "1940-10-9"))
	assertHTMLEqual(t, p.AsTable(), `
            <tr><td colspan="2">
            <ul class="errorlist nonfield"><li>
            (Hidden field hidden_text) This field is required.</li></ul></td></tr>
            <tr><th>First name:</th><td>
            <input type="text" name="first_name" value="John" required></td></tr>
            <tr><th>Last name:</th><td>
            <input type="text" name="last_name" value="Lennon" required></td></tr>
            <tr><th>Birthday:</th><td>
            <input type="text" name="birthday" value="1940-10-9" required>
            <input type="hidden" name="hidden_text"></td></tr>`)
	assertHTMLEqual(t, p.AsUL(), `
            <li><ul class="errorlist nonfield"><li>
            (Hidden field hidden_text) This field is required.</li></ul></li>
            <li>First name: <input type="text" name="first_name" value="John" required>
            </li>
            <li>Last name: <input type="text" name="last_name" value="Lennon" required>
            </li>
            <li>Birthday: <input type="text" name="birthday" value="1940-10-9" required>
            <input type="hidden" name="hidden_text"></li>`)
	assertHTMLEqual(t, p.AsP(), `
            <ul class="errorlist nonfield"><li>
            (Hidden field hidden_text) This field is required.</li></ul>
            <p>First name: <input type="text" name="first_name" value="John" required>
            </p>
            <p>Last name: <input type="text" name="last_name" value="Lennon" required>
            </p>
            <p>Birthday: <input type="text" name="birthday" value="1940-10-9" required>
            <input type="hidden" name="hidden_text"></p>`)
	assertHTMLEqual(t, p.AsDiv(),
		`<ul class="errorlist nonfield"><li>(Hidden field hidden_text) This field `+
			`is required.</li></ul><div>First name: <input type="text" `+
			`name="first_name" value="John" required></div><div>Last name: <input `+
			`type="text" name="last_name" value="Lennon" required></div><div>`+
			`Birthday: <input type="text" name="birthday" value="1940-10-9" required>`+
			`<input type="hidden" name="hidden_text"></div>`)

	// A form can have only hidden inputs.
	only := withoutAutoID(forms.New(
		forms.F("foo", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}))),
		forms.F("bar", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}))),
	))
	const hidden = `<input type="hidden" name="foo"><input type="hidden" name="bar">`
	assertHTMLEqual(t, only.AsTable(), hidden)
	assertHTMLEqual(t, only.AsUL(), hidden)
	assertHTMLEqual(t, only.AsP(), hidden)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_hidden_widget_does_not_have_aria_describedby
func TestHiddenWidgetDoesNotHaveAriaDescribedby(t *testing.T) {
	f := forms.New(forms.F("hidden_text", forms.NewCharField(
		forms.UseWidget(forms.HiddenInput{}), forms.HelpText("Help Text"))))
	equal(t, f.String(), `<input type="hidden" name="hidden_text" id="id_hidden_text">`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_field_order
func TestFieldOrder(t *testing.T) {
	var names []string
	want := ""
	for i := 1; i <= 14; i++ {
		names = append(names, fmt.Sprintf("field%d", i))
		want += fmt.Sprintf(`<tr><th>Field%d:</th><td><input type="text" name="field%d" required></td></tr>`, i, i)
	}
	assertHTMLEqual(t, withoutAutoID(textFieldsForm(names...)).AsTable(), want)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_explicit_field_order
//
// The class attribute field_order and its inheritance have no Go
// counterpart; the order is applied with Form.OrderFields, and a removed
// field with RemoveField. The unknown-name case checks that OrderFields
// ignores names that are not fields.
func TestExplicitFieldOrder(t *testing.T) {
	parentOrder := []string{"field6", "field5", "field4", "field2", "field1"}
	formOrder := []string{"field2", "field4", "field3", "field5", "field6"}
	missingOrder := []string{"field2", "field4", "field3", "field5", "field6", "field1"}

	parent := func() *forms.Form { return textFieldsForm("field1", "field2", "field4", "field5", "field6") }
	child := func() *forms.Form {
		f := parent()
		f.AddField("field3", forms.NewCharField())
		return f
	}

	p := parent()
	p.OrderFields(parentOrder...)
	equal(t, p.FieldNames(), parentOrder)

	p = child()
	p.RemoveField("field1")
	p.OrderFields(formOrder...)
	equal(t, p.FieldNames(), formOrder)

	p = child()
	p.RemoveField("field1")
	p.OrderFields(missingOrder...)
	equal(t, p.FieldNames(), formOrder)

	p = child()
	p.OrderFields(missingOrder...)
	equal(t, p.FieldNames(), missingOrder)

	// TestFormInit: the parent plus field3, ordered in the constructor.
	p = child()
	p.OrderFields(formOrder...)
	equal(t, p.FieldNames(), append(slices.Clone(formOrder), "field1"))

	p = child()
	p.OrderFields("unknown")
	equal(t, p.FieldNames(), []string{"field1", "field2", "field4", "field5", "field6", "field3"})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_form_html_attributes
func TestFormHTMLAttributes(t *testing.T) {
	p := withoutAutoID(forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(10))),
		forms.F("password", forms.NewCharField(forms.MaxLength(10), forms.UseWidget(forms.PasswordInput{}))),
		forms.F("realname", forms.NewCharField(forms.MaxLength(10), forms.UseWidget(forms.TextInput{}))),
		forms.F("address", forms.NewCharField()),
	))
	assertHTMLEqual(t, p.AsUL(), `
            <li>Username: <input type="text" name="username" maxlength="10" required>
            </li>
            <li>Password: <input type="password" name="password" maxlength="10"
                required></li>
            <li>Realname: <input type="text" name="realname" maxlength="10" required>
            </li>
            <li>Address: <input type="text" name="address" required></li>`)

	// The max_length of the field overrides a maxlength in the widget attrs.
	p = withoutAutoID(forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(10),
			forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"maxlength": 20}}))),
		forms.F("password", forms.NewCharField(forms.MaxLength(10), forms.UseWidget(forms.PasswordInput{}))),
	))
	assertHTMLEqual(t, p.AsUL(),
		`<li>Username: <input type="text" name="username" maxlength="10" required>`+
			`</li>`+
			`<li>Password: <input type="password" name="password" maxlength="10" `+
			`required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_specifying_labels
func TestSpecifyingLabels(t *testing.T) {
	p := withoutAutoID(forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(10), forms.Label("Your username"))),
		forms.F("password1", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
		forms.F("password2", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}), forms.Label("Contraseña (de nuevo)"))),
	))
	assertHTMLEqual(t, p.AsUL(), `
            <li>Your username:
            <input type="text" name="username" maxlength="10" required></li>
            <li>Password1: <input type="password" name="password1" required></li>
            <li>Contraseña (de nuevo):
            <input type="password" name="password2" required></li>`)

	// A label ends in a colon only when it does not end in other punctuation.
	questions := func() *forms.Form {
		return forms.New(
			forms.F("q1", forms.NewCharField(forms.Label("The first question"))),
			forms.F("q2", forms.NewCharField(forms.Label("What is your name?"))),
			forms.F("q3", forms.NewCharField(forms.Label("The answer to life is:"))),
			forms.F("q4", forms.NewCharField(forms.Label("Answer this question!"))),
			forms.F("q5", forms.NewCharField(forms.Label("The last question. Period."))),
		)
	}
	assertHTMLEqual(t, withoutAutoID(questions()).AsP(), `<p>The first question: <input type="text" name="q1" required></p>
<p>What is your name? <input type="text" name="q2" required></p>
<p>The answer to life is: <input type="text" name="q3" required></p>
<p>Answer this question! <input type="text" name="q4" required></p>
<p>The last question. Period. <input type="text" name="q5" required></p>`)
	assertHTMLEqual(t, questions().AsP(), `
            <p><label for="id_q1">The first question:</label>
            <input type="text" name="q1" id="id_q1" required></p>
            <p><label for="id_q2">What is your name?</label>
            <input type="text" name="q2" id="id_q2" required></p>
            <p><label for="id_q3">The answer to life is:</label>
            <input type="text" name="q3" id="id_q3" required></p>
            <p><label for="id_q4">Answer this question!</label>
            <input type="text" name="q4" id="id_q4" required></p>
            <p><label for="id_q5">The last question. Period.</label>
            <input type="text" name="q5" id="id_q5" required></p>`)

	// An empty label leaves the label out.
	p = withoutAutoID(userRegForm(forms.Label("")))
	assertHTMLEqual(t, p.AsUL(), `<li> <input type="text" name="username" maxlength="10" required></li>
<li>Password: <input type="password" name="password" required></li>`)
	p = userRegForm(forms.Label(""))
	assertHTMLEqual(t, p.AsUL(), `
            <li>
            <input id="id_username" type="text" name="username" maxlength="10" required>
            </li>
            <li><label for="id_password">Password:</label>
            <input type="password" name="password" id="id_password" required></li>`)

	// Without a label option the label comes from the field name (Django's
	// label=None).
	p = withoutAutoID(userRegForm())
	assertHTMLEqual(t, p.AsUL(),
		`<li>Username: <input type="text" name="username" maxlength="10" required>`+
			`</li>`+
			`<li>Password: <input type="password" name="password" required></li>`)
	p = userRegForm()
	assertHTMLEqual(t, p.AsUL(), `<li><label for="id_username">Username:</label>
<input id="id_username" type="text" name="username" maxlength="10" required></li>
<li><label for="id_password">Password:</label>
<input type="password" name="password" id="id_password" required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_label_suffix
func TestLabelSuffix(t *testing.T) {
	newForm := func(suffix *string) *forms.Form {
		f := withoutAutoID(forms.New(
			forms.F("color", forms.NewCharField(forms.Label("Favorite color?"))),
			forms.F("animal", forms.NewCharField(forms.Label("Favorite animal"))),
			forms.F("answer", forms.NewCharField(forms.Label("Secret answer"), forms.LabelSuffix(" ="))),
		))
		if suffix != nil {
			f.LabelSuffix = *suffix
		}
		return f
	}
	str := func(s string) *string { return &s }
	assertHTMLEqual(t, newForm(nil).AsUL(), `<li>Favorite color? <input type="text" name="color" required></li>
<li>Favorite animal: <input type="text" name="animal" required></li>
<li>Secret answer = <input type="text" name="answer" required></li>`)
	assertHTMLEqual(t, newForm(str("?")).AsUL(), `<li>Favorite color? <input type="text" name="color" required></li>
<li>Favorite animal? <input type="text" name="animal" required></li>
<li>Secret answer = <input type="text" name="answer" required></li>`)
	assertHTMLEqual(t, newForm(str("")).AsUL(), `<li>Favorite color? <input type="text" name="color" required></li>
<li>Favorite animal <input type="text" name="animal" required></li>
<li>Secret answer = <input type="text" name="answer" required></li>`)
	assertHTMLEqual(t, newForm(str("→")).AsUL(),
		`<li>Favorite color? <input type="text" name="color" required></li>`+"\n"+
			"<li>Favorite animal→ "+
			`<input type="text" name="animal" required></li>`+"\n"+
			`<li>Secret answer = <input type="text" name="answer" required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_initial_data
func TestInitialData(t *testing.T) {
	newForm := func() *forms.Form { return withoutAutoID(userRegForm(forms.Initial("django"))) }
	p := newForm()
	assertHTMLEqual(t, p.AsUL(), `
            <li>Username: <input type="text" name="username" value="django"
                maxlength="10" required></li>
            <li>Password: <input type="password" name="password" required></li>`)

	// Submitted data hides the initial value.
	const empty = `<li><ul class="errorlist"><li>This field is required.</li></ul>
Username: <input type="text" name="username" maxlength="10" aria-invalid="true"
required></li><li><ul class="errorlist"><li>This field is required.</li></ul>
Password: <input type="password" name="password" aria-invalid="true" required></li>`
	assertHTMLEqual(t, newForm().Bind(vals()).AsUL(), empty)
	assertHTMLEqual(t, newForm().Bind(vals("username", "")).AsUL(), empty)
	assertHTMLEqual(t, newForm().Bind(vals("username", "foo")).AsUL(), `
            <li>Username: <input type="text" name="username" value="foo" maxlength="10"
                required></li>
            <li><ul class="errorlist"><li>This field is required.</li></ul>
            Password: <input type="password" name="password" aria-invalid="true"
            required></li>`)

	// The initial value is not a fallback for missing data.
	p = userRegForm(forms.Initial("django")).Bind(vals("password", "secret"))
	equal(t, errorsOf(p, "username"), []string{"This field is required."})
	equal(t, p.IsValid(), false)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_dynamic_initial_data
func TestDynamicInitialData(t *testing.T) {
	newForm := func(initial string) *forms.Form {
		f := withoutAutoID(userRegForm())
		if initial != "" {
			f.Initial = map[string]any{"username": initial}
		}
		return f
	}
	assertHTMLEqual(t, newForm("django").AsUL(), `
            <li>Username: <input type="text" name="username" value="django"
                maxlength="10" required></li>
            <li>Password: <input type="password" name="password" required></li>`)
	assertHTMLEqual(t, newForm("stephane").AsUL(), `
            <li>Username: <input type="text" name="username" value="stephane"
                maxlength="10" required></li>
            <li>Password: <input type="password" name="password" required></li>`)

	// The initial argument is meaningless when data is passed.
	const empty = `<li><ul class="errorlist"><li>This field is required.</li></ul>
Username: <input type="text" name="username" maxlength="10" aria-invalid="true"
required></li><li><ul class="errorlist"><li>This field is required.</li></ul>
Password: <input type="password" name="password" aria-invalid="true" required></li>`
	assertHTMLEqual(t, newForm("django").Bind(vals()).AsUL(), empty)
	assertHTMLEqual(t, newForm("django").Bind(vals("username", "")).AsUL(), empty)
	assertHTMLEqual(t, newForm("django").Bind(vals("username", "foo")).AsUL(), `
            <li>Username: <input type="text" name="username" value="foo" maxlength="10"
                required></li>
            <li><ul class="errorlist"><li>This field is required.</li></ul>
            Password: <input type="password" name="password" aria-invalid="true"
            required></li>`)

	p := newForm("django").Bind(vals("password", "secret"))
	equal(t, errorsOf(p, "username"), []string{"This field is required."})
	equal(t, p.IsValid(), false)

	// The initial of the form takes precedence over the initial of the field.
	f := withoutAutoID(userRegForm(forms.Initial("django")))
	f.Initial = map[string]any{"username": "babik"}
	assertHTMLEqual(t, f.AsUL(), `
            <li>Username: <input type="text" name="username" value="babik"
                maxlength="10" required></li>
            <li>Password: <input type="password" name="password" required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_callable_initial_data
func TestCallableInitialData(t *testing.T) {
	initialDjango := func() any { return "django" }
	initialStephane := func() any { return "stephane" }
	initialOptions := func() any { return []string{"f", "b"} }
	initialOtherOptions := func() any { return []string{"b", "w"} }

	newForm := func(usernameInitial, optionsInitial any, initial map[string]any) *forms.Form {
		usernameOpts, optionsOpts := []forms.FieldOption{forms.MaxLength(10)}, []forms.FieldOption{optionsChoices()}
		if usernameInitial != nil {
			usernameOpts = append(usernameOpts, forms.Initial(usernameInitial))
		}
		if optionsInitial != nil {
			optionsOpts = append(optionsOpts, forms.Initial(optionsInitial))
		}
		f := withoutAutoID(forms.New(
			forms.F("username", forms.NewCharField(usernameOpts...)),
			forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
			forms.F("options", forms.NewMultipleChoiceField(optionsOpts...)),
		))
		f.Initial = initial
		return f
	}
	formInitial := map[string]any{"username": initialDjango, "options": initialOptions}

	assertHTMLEqual(t, newForm(nil, nil, formInitial).AsUL(), `
            <li>Username: <input type="text" name="username" value="django"
                maxlength="10" required></li>
            <li>Password: <input type="password" name="password" required></li>
            <li>Options: <select multiple name="options" required>
            <option value="f" selected>foo</option>
            <option value="b" selected>bar</option>
            <option value="w">whiz</option>
            </select></li>`)

	// The initial argument is meaningless when data is passed.
	assertHTMLEqual(t, newForm(nil, nil, formInitial).Bind(vals()).AsUL(), `<li><ul class="errorlist"><li>This field is required.</li></ul>
Username: <input type="text" name="username" maxlength="10" aria-invalid="true"
required></li><li><ul class="errorlist"><li>This field is required.</li></ul>
Password: <input type="password" name="password" aria-invalid="true"
required></li><li><ul class="errorlist"><li>This field is required.</li></ul>
Options: <select multiple name="options" aria-invalid="true" required>
<option value="f">foo</option>
<option value="b">bar</option>
<option value="w">whiz</option>
</select></li>`)
	assertHTMLEqual(t, newForm(nil, nil, map[string]any{"username": initialDjango}).Bind(vals("username", "")).AsUL(),
		`<li><ul class="errorlist"><li>This field is required.</li></ul>
Username: <input type="text" name="username" maxlength="10" aria-invalid="true"
required></li><li><ul class="errorlist"><li>This field is required.</li></ul>
Password: <input type="password" name="password" aria-invalid="true" required></li>
<li><ul class="errorlist"><li>This field is required.</li></ul>
Options: <select multiple name="options" aria-invalid="true" required>
<option value="f">foo</option>
<option value="b">bar</option>
<option value="w">whiz</option>
</select></li>`)
	assertHTMLEqual(t, newForm(nil, nil, map[string]any{"username": initialDjango}).
		Bind(vals("username", "foo", "options", "f", "options", "b")).AsUL(), `
            <li>Username: <input type="text" name="username" value="foo" maxlength="10"
                required></li>
            <li><ul class="errorlist"><li>This field is required.</li></ul>
            Password: <input type="password" name="password" aria-invalid="true"
            required></li><li>Options: <select multiple name="options" required>
            <option value="f" selected>foo</option>
            <option value="b" selected>bar</option>
            <option value="w">whiz</option>
            </select></li>`)

	// The initial is not a fallback for missing data.
	p := newForm(nil, nil, formInitial).Bind(vals("password", "secret"))
	equal(t, errorsOf(p, "username"), []string{"This field is required."})
	equal(t, p.IsValid(), false)

	// The initial of the form takes precedence over the initial of the field.
	assertHTMLEqual(t, newForm(initialDjango, initialOtherOptions, nil).AsUL(), `
            <li>Username: <input type="text" name="username" value="django"
                maxlength="10" required></li>
            <li>Password: <input type="password" name="password" required></li>
            <li>Options: <select multiple name="options" required>
            <option value="f">foo</option>
            <option value="b" selected>bar</option>
            <option value="w" selected>whiz</option>
            </select></li>`)
	assertHTMLEqual(t, newForm(initialDjango, initialOtherOptions,
		map[string]any{"username": initialStephane, "options": initialOptions}).AsUL(), `
            <li>Username: <input type="text" name="username" value="stephane"
                maxlength="10" required></li>
            <li>Password: <input type="password" name="password" required></li>
            <li>Options: <select multiple name="options" required>
            <option value="f" selected>foo</option>
            <option value="b" selected>bar</option>
            <option value="w">whiz</option>
            </select></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_get_initial_for_field
//
// Form.get_initial_for_field is BoundField.Initial.
func TestGetInitialForField(t *testing.T) {
	now := datetime(2006, 10, 25, 14, 30, 45, 123456000)
	form := forms.New(
		forms.F("first_name", forms.NewCharField(forms.Initial("John"))),
		forms.F("last_name", forms.NewCharField(forms.Initial("Doe"))),
		forms.F("age", forms.NewIntegerField()),
		forms.F("occupation", forms.NewCharField(forms.Initial(func() any { return "Unknown" }))),
		forms.F("dt_fixed", forms.NewDateTimeField(forms.Initial(now))),
		forms.F("dt_callable", forms.NewDateTimeField(forms.Initial(func() any { return now }))),
	)
	form.Initial = map[string]any{"first_name": "Jane"}
	cases := []struct {
		name string
		want any
	}{
		{"age", nil},
		{"last_name", "Doe"},
		// The initial of the form overrides the initial of the field.
		{"first_name", "Jane"},
		// Callables are evaluated.
		{"occupation", "Unknown"},
		// Microseconds are removed from datetimes.
		{"dt_fixed", datetime(2006, 10, 25, 14, 30, 45, 0)},
		{"dt_callable", datetime(2006, 10, 25, 14, 30, 45, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			equal(t, form.Field(c.name).Initial(), c.want)
		})
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_changed_data
//
// show_hidden_initial does not exist in the package, so the pedantic field
// is compared with its own initial value instead of the hidden one; its
// conversion fails either way.
func TestChangedData(t *testing.T) {
	newForm := func(extra ...forms.NamedField) *forms.Form {
		return forms.New(append([]forms.NamedField{
			forms.F("first_name", forms.NewCharField(forms.Initial("Hans"))),
			forms.F("last_name", forms.NewCharField(forms.Initial("Greatel"))),
			forms.F("birthday", forms.NewDateField(forms.Initial(date(1974, 8, 16)))),
		}, extra...)...)
	}
	p := newForm().Bind(vals("first_name", "Hans", "last_name", "Scrmbl", "birthday", "1974-08-16"))
	equal(t, p.IsValid(), true)
	changed := p.ChangedData()
	equal(t, slices.Contains(changed, "first_name"), false)
	equal(t, slices.Contains(changed, "last_name"), true)
	equal(t, slices.Contains(changed, "birthday"), false)

	// A field raising a validation error is always in changed data.
	p = newForm(forms.F("pedantic", newPedanticField(forms.Initial("whatever")))).Bind(vals(
		"first_name", "Hans", "last_name", "Scrmbl", "birthday", "1974-08-16", "pedantic", "whatever"))
	equal(t, p.IsValid(), false)
	equal(t, slices.Contains(p.ChangedData(), "pedantic"), true)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_values
func TestBoundfieldValues(t *testing.T) {
	newForm := func() *forms.Form {
		return forms.New(
			forms.F("username", forms.NewCharField(forms.MaxLength(10), forms.Initial("djangonaut"))),
			forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
		)
	}
	unbound := newForm()
	bound := newForm().Bind(vals("password", "foo"))
	equal(t, bound.Field("username").Value(), nil)
	equal(t, unbound.Field("username").Value(), any("djangonaut"))
	equal(t, bound.Field("password").Value(), any("foo"))
	equal(t, unbound.Field("password").Value(), nil)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_initial_called_once
func TestBoundfieldInitialCalledOnce(t *testing.T) {
	calls := 0
	form := forms.New(forms.F("name", forms.NewCharField(forms.MaxLength(10), forms.Initial(func() any {
		calls++
		return fmt.Sprintf("uuid-%d", calls)
	}))))
	name := form.Field("name")
	equal(t, name.Value(), name.Value())
	// The bound field is also cached.
	if form.Field("name") != name {
		t.Error("Form.Field returned a different BoundField")
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_value_disabled_callable_initial
func TestBoundfieldValueDisabledCallableInitial(t *testing.T) {
	newForm := func() *forms.Form {
		return forms.New(forms.F("name", forms.NewCharField(
			forms.Initial(func() any { return "John Doe" }), forms.Disabled())))
	}
	// Without form data.
	equal(t, newForm().Field("name").Value(), any("John Doe"))
	// With form data. The field is disabled, so the data does not affect the
	// value.
	equal(t, newForm().Bind(vals()).Field("name").Value(), any("John Doe"))
}

// hiddenInputWithoutMicrosec is a HiddenInput with supports_microseconds =
// False.
type hiddenInputWithoutMicrosec struct{ forms.HiddenInput }

// SupportsMicroseconds reports false.
func (hiddenInputWithoutMicrosec) SupportsMicroseconds() bool { return false }

// textInputWithoutMicrosec is a TextInput with supports_microseconds = False.
type textInputWithoutMicrosec struct{ forms.TextInput }

// SupportsMicroseconds reports false.
func (textInputWithoutMicrosec) SupportsMicroseconds() bool { return false }

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_initial_datetime_values
//
// get_initial_for_field is BoundField.Initial. A fixed time stands in for
// datetime.now().
func TestInitialDatetimeValues(t *testing.T) {
	now := datetime(2006, 10, 25, 14, 30, 45, 123456000)
	nowNoMS := datetime(2006, 10, 25, 14, 30, 45, 0)
	delayedNow := func() any { return now }
	delayedNowTime := func() any { return clock(14, 30, 45, 123456000) }
	unbound := forms.New(
		forms.F("fixed", forms.NewDateTimeField(forms.Initial(now))),
		forms.F("auto_timestamp", forms.NewDateTimeField(forms.Initial(delayedNow))),
		forms.F("auto_time_only", forms.NewTimeField(forms.Initial(delayedNowTime))),
		forms.F("supports_microseconds", forms.NewDateTimeField(forms.Initial(delayedNow), forms.UseWidget(forms.TextInput{}))),
		forms.F("hi_default_microsec", forms.NewDateTimeField(forms.Initial(delayedNow), forms.UseWidget(forms.HiddenInput{}))),
		forms.F("hi_without_microsec", forms.NewDateTimeField(forms.Initial(delayedNow), forms.UseWidget(hiddenInputWithoutMicrosec{}))),
		forms.F("ti_without_microsec", forms.NewDateTimeField(forms.Initial(delayedNow), forms.UseWidget(textInputWithoutMicrosec{}))),
	)
	cases := []struct {
		name string
		want any
	}{
		{"fixed", nowNoMS},
		{"auto_timestamp", nowNoMS},
		{"auto_time_only", clock(14, 30, 45, 0)},
		{"supports_microseconds", now},
		{"hi_default_microsec", now},
		{"hi_without_microsec", nowNoMS},
		{"ti_without_microsec", nowNoMS},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			equal(t, unbound.Field(c.name).Value(), c.want)
			equal(t, unbound.Field(c.name).Initial(), c.want)
		})
	}
}

// datetimeFormWithCallableInitial is get_datetime_form_with_callable_initial
// of Django's forms tests: a bound form whose dt field has a callable initial
// that returns a time one second later on every call.
func datetimeFormWithCallableInitial(disabled bool, nsec int) *forms.Form {
	elapsed := 0
	initial := func() any {
		elapsed++
		return datetime(2006, 10, 25, 14, 30, 45+elapsed, nsec)
	}
	opts := []forms.FieldOption{forms.Initial(initial)}
	if disabled {
		opts = append(opts, forms.Disabled())
	}
	return forms.New(forms.F("dt", forms.NewDateTimeField(opts...))).Bind(vals())
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_datetime_clean_disabled_callable_initial_microseconds
func TestDatetimeCleanDisabledCallableInitialMicroseconds(t *testing.T) {
	form := datetimeFormWithCallableInitial(true, 123456000)
	equal(t, form.Errors().Len(), 0)
	equal(t, form.CleanedData(), map[string]any{"dt": datetime(2006, 10, 25, 14, 30, 46, 0)})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_datetime_clean_disabled_callable_initial_bound_field
func TestDatetimeCleanDisabledCallableInitialBoundField(t *testing.T) {
	form := datetimeFormWithCallableInitial(true, 0)
	equal(t, form.Errors().Len(), 0)
	cleaned := form.CleanedData()["dt"]
	equal(t, cleaned, any(datetime(2006, 10, 25, 14, 30, 46, 0)))
	bf := form.Field("dt")
	equal(t, cleaned, bf.Initial())
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_datetime_changed_data_callable_with_microseconds
func TestDatetimeChangedDataCallableWithMicroseconds(t *testing.T) {
	form := forms.New(forms.F("dt", forms.NewDateTimeField(
		forms.Initial(func() any { return datetime(2006, 10, 25, 14, 30, 45, 123456000) }),
		forms.Disabled(),
	))).Bind(vals("dt", "2006-10-25 14:30:45"))
	equal(t, form.ChangedData(), []string{})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_help_text
func TestHelpText(t *testing.T) {
	newForm := func() *forms.Form {
		return withoutAutoID(forms.New(
			forms.F("username", forms.NewCharField(forms.MaxLength(10), forms.HelpText("e.g., user@example.com"))),
			forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}), forms.HelpText("Wählen Sie mit Bedacht."))),
		))
	}
	p := newForm()
	assertHTMLEqual(t, p.AsUL(), `<li>Username: <input type="text" name="username" maxlength="10" required>
<span class="helptext">e.g., user@example.com</span></li>
<li>Password: <input type="password" name="password" required>
<span class="helptext">Wählen Sie mit Bedacht.</span></li>`)
	assertHTMLEqual(t, p.AsP(), `<p>Username: <input type="text" name="username" maxlength="10" required>
<span class="helptext">e.g., user@example.com</span></p>
<p>Password: <input type="password" name="password" required>
<span class="helptext">Wählen Sie mit Bedacht.</span></p>`)
	assertHTMLEqual(t, p.AsTable(), `
            <tr><th>Username:</th><td>
            <input type="text" name="username" maxlength="10" required><br>
            <span class="helptext">e.g., user@example.com</span></td></tr>
            <tr><th>Password:</th><td><input type="password" name="password" required>
            <br>
            <span class="helptext">Wählen Sie mit Bedacht.</span></td></tr>`)
	assertHTMLEqual(t, p.AsDiv(),
		`<div>Username: <div class="helptext">e.g., user@example.com</div>`+
			`<input type="text" name="username" maxlength="10" required></div>`+
			`<div>Password: <div class="helptext">Wählen Sie mit Bedacht.</div>`+
			`<input type="password" name="password" required></div>`)

	// The help text is displayed whether or not data is provided.
	p = newForm().Bind(vals("username", "foo"))
	assertHTMLEqual(t, p.AsUL(),
		`<li>Username: <input type="text" name="username" value="foo" `+
			`maxlength="10" required>`+
			`<span class="helptext">e.g., user@example.com</span></li>`+
			`<li><ul class="errorlist"><li>This field is required.</li></ul>`+
			`Password: <input type="password" name="password" aria-invalid="true" `+
			`required><span class="helptext">Wählen Sie mit Bedacht.</span></li>`)

	// The help text of a hidden field is not displayed.
	p = withoutAutoID(forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(10), forms.HelpText("e.g., user@example.com"))),
		forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
		forms.F("next", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}), forms.Initial("/"),
			forms.HelpText("Redirect destination"))),
	))
	assertHTMLEqual(t, p.AsUL(), `<li>Username: <input type="text" name="username" maxlength="10" required>
<span class="helptext">e.g., user@example.com</span></li>
<li>Password: <input type="password" name="password" required>
<input type="hidden" name="next" value="/"></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_help_text_html_safe
func TestHelpTextHTMLSafe(t *testing.T) {
	p := withoutAutoID(forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(10), forms.HelpText("e.g., user@example.com"))),
		forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}),
			forms.HelpText("Help text is <strong>escaped</strong>."))),
	))
	assertHTMLEqual(t, p.AsUL(),
		`<li>Username: <input type="text" name="username" maxlength="10" required>`+
			`<span class="helptext">e.g., user@example.com</span></li>`+
			`<li>Password: <input type="password" name="password" required>`+
			`<span class="helptext">Help text is <strong>escaped</strong>.</span></li>`)
	assertHTMLEqual(t, p.AsP(),
		`<p>Username: <input type="text" name="username" maxlength="10" required>`+
			`<span class="helptext">e.g., user@example.com</span></p>`+
			`<p>Password: <input type="password" name="password" required>`+
			`<span class="helptext">Help text is <strong>escaped</strong>.</span></p>`)
	assertHTMLEqual(t, p.AsTable(),
		`<tr><th>Username:</th><td>`+
			`<input type="text" name="username" maxlength="10" required><br>`+
			`<span class="helptext">e.g., user@example.com</span></td></tr>`+
			`<tr><th>Password:</th><td>`+
			`<input type="password" name="password" required><br>`+
			`<span class="helptext">Help text is <strong>escaped</strong>.</span>`+
			`</td></tr>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_widget_attrs_custom_aria_describedby
func TestWidgetAttrsCustomAriaDescribedby(t *testing.T) {
	p := forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(255), forms.HelpText("e.g., user@example.com"),
			forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"aria-describedby": "custom-description"}}))),
		forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}),
			forms.HelpText("Wählen Sie mit Bedacht."))),
	)
	assertHTMLEqual(t, p.AsDiv(),
		`<div><label for="id_username">Username:</label>`+
			`<div class="helptext" id="id_username_helptext">e.g., user@example.com`+
			`</div><input type="text" name="username" maxlength="255" required `+
			`aria-describedby="custom-description" id="id_username">`+
			`</div><div>`+
			`<label for="id_password">Password:</label>`+
			`<div class="helptext" id="id_password_helptext">Wählen Sie mit Bedacht.`+
			`</div><input type="password" name="password" required `+
			`aria-describedby="id_password_helptext" id="id_password"></div>`)
	assertHTMLEqual(t, p.AsUL(),
		`<li><label for="id_username">Username:</label><input type="text" `+
			`name="username" maxlength="255" required `+
			`aria-describedby="custom-description" id="id_username">`+
			`<span class="helptext" id="id_username_helptext">e.g., user@example.com`+
			`</span></li><li>`+
			`<label for="id_password">Password:</label>`+
			`<input type="password" name="password" required `+
			`aria-describedby="id_password_helptext" id="id_password">`+
			`<span class="helptext" id="id_password_helptext">Wählen Sie mit Bedacht.`+
			`</span></li>`)
	assertHTMLEqual(t, p.AsP(),
		`<p><label for="id_username">Username:</label><input type="text" `+
			`name="username" maxlength="255" required `+
			`aria-describedby="custom-description" id="id_username">`+
			`<span class="helptext" id="id_username_helptext">e.g., user@example.com`+
			`</span></p><p>`+
			`<label for="id_password">Password:</label>`+
			`<input type="password" name="password" required `+
			`aria-describedby="id_password_helptext" id="id_password">`+
			`<span class="helptext" id="id_password_helptext">Wählen Sie mit Bedacht.`+
			`</span></p>`)
	assertHTMLEqual(t, p.AsTable(),
		`<tr><th><label for="id_username">Username:</label></th><td>`+
			`<input type="text" name="username" maxlength="255" required `+
			`aria-describedby="custom-description" id="id_username"><br>`+
			`<span class="helptext" id="id_username_helptext">e.g., user@example.com`+
			`</span></td></tr><tr><th>`+
			`<label for="id_password">Password:</label></th><td>`+
			`<input type="password" name="password" required `+
			`aria-describedby="id_password_helptext" id="id_password"><br>`+
			`<span class="helptext" id="id_password_helptext">Wählen Sie mit Bedacht.`+
			`</span></td></tr>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_aria_describedby_custom_widget_id
func TestAriaDescribedbyCustomWidgetID(t *testing.T) {
	f := forms.New(forms.F("username", forms.NewCharField(forms.MaxLength(255),
		forms.HelpText("e.g., user@example.com"),
		forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"id": "custom-id"}}))))
	assertHTMLEqual(t, f.String(),
		`<div><label for="custom-id">Username:</label>`+
			`<div class="helptext" id="id_username_helptext">e.g., user@example.com`+
			`</div><input type="text" name="username" id="custom-id" maxlength="255" `+
			`required aria-describedby="id_username_helptext"></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_select_aria_describedby
//
// Django binds the plain dict {"color": "Blue"}, whose string value fails with
// "Enter a list of values.". Submitted data is url.Values here, always a list,
// so "Blue" is checked as a choice and fails with the invalid_choice message.
func TestSelectAriaDescribedby(t *testing.T) {
	f := forms.New(forms.F("color", forms.NewMultipleChoiceField(
		forms.Choices("red", "Red", "green", "Green"), forms.HelpText("Select Color")))).Bind(vals("color", "Blue"))
	assertHTMLEqual(t, f.String(),
		`<div><label for="id_color">Color:</label><div class="helptext" `+
			`id="id_color_helptext">Select Color</div>`+
			`<ul class="errorlist" id="id_color_error"><li>Select a valid choice. `+
			`Blue is not one of the available choices.</li></ul><select name="color" required aria-invalid="true" `+
			`aria-describedby="id_color_helptext id_color_error" id="id_color" `+
			`multiple><option value="red">Red</option>`+
			`<option value="green">Green</option></select></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_textarea_aria_describedby
func TestTextareaAriaDescribedby(t *testing.T) {
	f := forms.New(forms.F("color", forms.NewCharField(
		forms.UseWidget(forms.Textarea{}), forms.MaxLength(5), forms.HelpText("Enter Color")))).Bind(vals("color", "Purple"))
	assertHTMLEqual(t, f.String(),
		`<div><label for="id_color">Color:</label>`+
			`<div class="helptext" id="id_color_helptext">Enter Color</div>`+
			`<ul class="errorlist" id="id_color_error">`+
			`<li>Ensure this value has at most 5 characters (it has 6).</li></ul>`+
			`<textarea name="color" cols="40" rows="10" maxlength="5" required `+
			`aria-invalid="true" aria-describedby="id_color_helptext id_color_error" `+
			`id="id_color">Purple</textarea></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_aria_describedby_called_multiple_times
func TestAriaDescribedbyCalledMultipleTimes(t *testing.T) {
	f := forms.New(forms.F("color", forms.NewCharField(
		forms.UseWidget(forms.Textarea{}), forms.HelpText("Enter Color")))).Bind(vals("color", "Purple"))
	equal(t, f.Field("color").AriaDescribedBy(), "id_color_helptext")
	f.AddError("color", "An error about Purple.")
	equal(t, f.Field("color").AriaDescribedBy(), "id_color_helptext id_color_error")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_fieldset_aria_describedby
func TestFieldsetAriaDescribedby(t *testing.T) {
	newForm := func() *forms.Form {
		return forms.New(
			forms.F("checkbox", forms.NewMultipleChoiceField(forms.Choices("a", "A", "b", "B"),
				forms.UseWidget(forms.CheckboxSelectMultiple{}), forms.HelpText("Checkbox help text"))),
			forms.F("radio", forms.NewMultipleChoiceField(forms.Choices("a", "A", "b", "B"),
				forms.UseWidget(forms.RadioSelect{}), forms.HelpText("Radio help text"))),
			forms.F("datetime", forms.NewSplitDateTimeField(forms.HelpText("Enter Date and Time"))),
		)
	}
	assertHTMLEqual(t, newForm().String(),
		`<div><fieldset aria-describedby="id_checkbox_helptext">`+
			`<legend>Checkbox:</legend>`+
			`<div class="helptext" id="id_checkbox_helptext">Checkbox help text</div>`+
			`<div id="id_checkbox"><div>`+
			`<label for="id_checkbox_0"><input type="checkbox" name="checkbox" `+
			`value="a" id="id_checkbox_0" /> A</label>`+
			`</div><div>`+
			`<label for="id_checkbox_1"><input type="checkbox" name="checkbox" `+
			`value="b" id="id_checkbox_1" /> B</label>`+
			`</div></div></fieldset></div>`+
			`<div><fieldset aria-describedby="id_radio_helptext">`+
			`<legend>Radio:</legend>`+
			`<div class="helptext" id="id_radio_helptext">Radio help text</div>`+
			`<div id="id_radio"><div>`+
			`<label for="id_radio_0"><input type="radio" name="radio" value="a" `+
			`required id="id_radio_0" />A</label>`+
			`</div><div>`+
			`<label for="id_radio_1"><input type="radio" name="radio" value="b" `+
			`required id="id_radio_1" /> B</label>`+
			`</div></div></fieldset></div>`+
			`<div><fieldset aria-describedby="id_datetime_helptext">`+
			`<legend>Datetime:</legend>`+
			`<div class="helptext" id="id_datetime_helptext">Enter Date and Time</div>`+
			`<input type="text" name="datetime_0" required id="id_datetime_0" />`+
			`<input type="text" name="datetime_1" required id="id_datetime_1" />`+
			`</fieldset></div>`)
	assertHTMLEqual(t,
		`<div><fieldset aria-describedby="id_checkbox_helptext `+
			`id_checkbox_error"> <legend>Checkbox:</legend> <div class="helptext" `+
			`id="id_checkbox_helptext">Checkbox help text</div> <ul class="errorlist" `+
			`id="id_checkbox_error"> <li>This field is required.</li> </ul> `+
			`<div id="id_checkbox"> <div> <label for="id_checkbox_0"><input `+
			`type="checkbox" name="checkbox" value="a" aria-invalid="true" `+
			`id="id_checkbox_0" /> A</label> </div> <div> <label for="id_checkbox_1">`+
			`<input type="checkbox" name="checkbox" value="b" aria-invalid="true" `+
			`id="id_checkbox_1" /> B</label> </div> </div> </fieldset> </div> <div> `+
			`<fieldset aria-describedby="id_radio_helptext id_radio_error"> `+
			`<legend>Radio:</legend> <div class="helptext" id="id_radio_helptext">`+
			`Radio help text</div> <ul class="errorlist" id="id_radio_error"><li>`+
			`This field is required.</li> </ul> <div id="id_radio"><div><label `+
			`for="id_radio_0"><input type="radio" name="radio" value="a" required `+
			`aria-invalid="true" id="id_radio_0" />A</label></div><div><label `+
			`for="id_radio_1"><input type="radio" name="radio" value="b" required `+
			`aria-invalid="true" id="id_radio_1" />B</label></div></div></fieldset>`+
			`</div><div><fieldset aria-describedby="id_datetime_helptext `+
			`id_datetime_error"><legend>Datetime:</legend><div class="helptext" `+
			`id="id_datetime_helptext">Enter Date and Time</div><ul class="errorlist" `+
			`id="id_datetime_error"><li>This field is required.</li></ul><input `+
			`type="text" name="datetime_0" required aria-invalid="true" `+
			`id="id_datetime_0" /><input type="text" name="datetime_1" required `+
			`aria-invalid="true" id="id_datetime_1" /></fieldset></div>`,
		newForm().Bind(vals()).String())

	// aria-describedby is not included without an auto id.
	f := withoutAutoID(newForm())
	requireSubstring(t, f.String(), "<fieldset>")
	requireSubstring(t, f.String(), `<div class="helptext">`)

	// aria-describedby uses a custom auto id.
	f = newForm()
	f.AutoID = "custom_%s"
	requireSubstring(t, f.String(), `fieldset aria-describedby="custom_checkbox_helptext"`)
	requireSubstring(t, f.String(), `<div class="helptext" id="custom_checkbox_helptext">`)
}

// requireSubstring fails the test when s does not contain substr.
func requireSubstring(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("%q not found in %q", substr, s)
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_fieldset_custom_aria_describedby
func TestFieldsetCustomAriaDescribedby(t *testing.T) {
	// aria-describedby set on the widget goes to the inputs, not to the
	// fieldset.
	f := forms.New(forms.F("checkbox", forms.NewMultipleChoiceField(forms.Choices("a", "A", "b", "B"),
		forms.UseWidget(forms.CheckboxSelectMultiple{Attrs: forms.Attrs{"aria-describedby": "custom-id"}}),
		forms.HelpText("Checkbox help text"))))
	assertHTMLEqual(t, f.String(),
		`<div><fieldset><legend>Checkbox:</legend>`+
			`<div class="helptext" id="id_checkbox_helptext">Checkbox help text</div>`+
			`<div id="id_checkbox"><div>`+
			`<label for="id_checkbox_0"><input type="checkbox" name="checkbox" `+
			`value="a" aria-describedby="custom-id" id="id_checkbox_0" />A</label>`+
			`</div><div>`+
			`<label for="id_checkbox_1"><input type="checkbox" name="checkbox" `+
			`value="b" aria-describedby="custom-id" id="id_checkbox_1" />B</label>`+
			`</div></div></fieldset></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_as_widget_custom_aria_describedby
func TestAsWidgetCustomAriaDescribedby(t *testing.T) {
	form := forms.New(forms.F("intl_name", forms.NewCharField(
		forms.HelpText("The food's international name."))))
	form = form.Bind(vals("intl_name", "Rendang"))
	assertHTMLEqual(t, form.Field("intl_name").AsWidget(nil, forms.Attrs{"aria-describedby": "some_custom_id"}),
		`<input type="text" name="intl_name" value="Rendang" `+
			`aria-describedby="some_custom_id" required id="id_intl_name">`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_subclassing_forms
//
// Class inheritance and the order in which Django merges the fields of
// several base classes have no Go counterpart; the forms are composed with
// AddField, and the Beatle form lists the fields in the order Django's
// metaclass produces.
func TestSubclassingForms(t *testing.T) {
	person := func() *forms.Form {
		return withoutAutoID(forms.New(
			forms.F("first_name", forms.NewCharField()),
			forms.F("last_name", forms.NewCharField()),
			forms.F("birthday", forms.NewDateField()),
		))
	}
	assertHTMLEqual(t, person().AsUL(), `<li>First name: <input type="text" name="first_name" required></li>
<li>Last name: <input type="text" name="last_name" required></li>
<li>Birthday: <input type="text" name="birthday" required></li>`)

	m := person()
	m.AddField("instrument", forms.NewCharField())
	assertHTMLEqual(t, m.AsUL(), `<li>First name: <input type="text" name="first_name" required></li>
<li>Last name: <input type="text" name="last_name" required></li>
<li>Birthday: <input type="text" name="birthday" required></li>
<li>Instrument: <input type="text" name="instrument" required></li>`)

	b := withoutAutoID(forms.New(
		forms.F("instrument", forms.NewCharField()),
		forms.F("first_name", forms.NewCharField()),
		forms.F("last_name", forms.NewCharField()),
		forms.F("birthday", forms.NewDateField()),
		forms.F("haircut_type", forms.NewCharField()),
	))
	assertHTMLEqual(t, b.AsUL(), `<li>Instrument: <input type="text" name="instrument" required></li>
<li>First name: <input type="text" name="first_name" required></li>
<li>Last name: <input type="text" name="last_name" required></li>
<li>Birthday: <input type="text" name="birthday" required></li>
<li>Haircut type: <input type="text" name="haircut_type" required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_forms_with_prefixes
//
// A form cannot override add_prefix in Go, so the last part of the Django
// test, a Person subclass with its own add_prefix, is left out. Form.Field
// returns nil for an unknown name where Django raises KeyError.
func TestFormsWithPrefixes(t *testing.T) {
	newForm := func(prefix string) *forms.Form {
		f := personForm()
		f.Prefix = prefix
		return f
	}
	data := vals(
		"person1-first_name", "John",
		"person1-last_name", "Lennon",
		"person1-birthday", "1940-10-9")
	p := newForm("person1").Bind(data)
	assertHTMLEqual(t, p.AsUL(), `
            <li><label for="id_person1-first_name">First name:</label>
            <input type="text" name="person1-first_name" value="John"
                id="id_person1-first_name" required></li>
            <li><label for="id_person1-last_name">Last name:</label>
            <input type="text" name="person1-last_name" value="Lennon"
                id="id_person1-last_name" required></li>
            <li><label for="id_person1-birthday">Birthday:</label>
            <input type="text" name="person1-birthday" value="1940-10-9"
                id="id_person1-birthday" required></li>`)
	assertHTMLEqual(t, p.Field("first_name").String(),
		`<input type="text" name="person1-first_name" value="John" `+
			`id="id_person1-first_name" required>`)
	assertHTMLEqual(t, p.Field("last_name").String(),
		`<input type="text" name="person1-last_name" value="Lennon" `+
			`id="id_person1-last_name" required>`)
	assertHTMLEqual(t, p.Field("birthday").String(),
		`<input type="text" name="person1-birthday" value="1940-10-9" `+
			`id="id_person1-birthday" required>`)
	equal(t, p.Errors().Len(), 0)
	equal(t, p.IsValid(), true)
	equal(t, p.CleanedData()["first_name"], any("John"))
	equal(t, p.CleanedData()["last_name"], any("Lennon"))
	equal(t, p.CleanedData()["birthday"], any(date(1940, 10, 9)))

	// Bad data: errors are reported by field name, without the prefix.
	p = newForm("person1").Bind(vals("person1-first_name", "", "person1-last_name", "", "person1-birthday", ""))
	equal(t, errorsOf(p, "first_name"), []string{"This field is required."})
	equal(t, errorsOf(p, "last_name"), []string{"This field is required."})
	equal(t, errorsOf(p, "birthday"), []string{"This field is required."})
	equal(t, p.Field("first_name").Errors().Messages(), []string{"This field is required."})
	// Accessing a nonexistent field.
	if p.Field("person1-first_name") != nil {
		t.Error("Field of a prefixed name is not nil")
	}

	// Data without the prefix is not seen by a form that has one.
	p = newForm("person1").Bind(vals("first_name", "John", "last_name", "Lennon", "birthday", "1940-10-9"))
	equal(t, errorsOf(p, "first_name"), []string{"This field is required."})
	equal(t, errorsOf(p, "last_name"), []string{"This field is required."})
	equal(t, errorsOf(p, "birthday"), []string{"This field is required."})

	// One data set holds the data of several instances of a form.
	both := vals(
		"person1-first_name", "John",
		"person1-last_name", "Lennon",
		"person1-birthday", "1940-10-9",
		"person2-first_name", "Jim",
		"person2-last_name", "Morrison",
		"person2-birthday", "1943-12-8")
	p1 := newForm("person1").Bind(both)
	equal(t, p1.IsValid(), true)
	equal(t, p1.CleanedData()["first_name"], any("John"))
	equal(t, p1.CleanedData()["last_name"], any("Lennon"))
	equal(t, p1.CleanedData()["birthday"], any(date(1940, 10, 9)))
	p2 := newForm("person2").Bind(both)
	equal(t, p2.IsValid(), true)
	equal(t, p2.CleanedData()["first_name"], any("Jim"))
	equal(t, p2.CleanedData()["last_name"], any("Morrison"))
	equal(t, p2.CleanedData()["birthday"], any(date(1943, 12, 8)))
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_class_prefix
//
// A class attribute has no Go counterpart; a form constructor sets Prefix, and
// the caller can set another one.
func TestClassPrefix(t *testing.T) {
	newForm := func() *forms.Form {
		f := forms.New(forms.F("first_name", forms.NewCharField()))
		f.Prefix = "foo"
		return f
	}
	p := newForm()
	equal(t, p.Prefix, "foo")
	p = newForm()
	p.Prefix = "bar"
	equal(t, p.Prefix, "bar")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_forms_with_null_boolean
//
// Submitted data is a string in Go, so Django's Python values True and False
// are given as "True" and "False".
func TestFormsWithNullBoolean(t *testing.T) {
	const unknown = `<select name="is_cool">
<option value="unknown" selected>Unknown</option>
<option value="true">Yes</option>
<option value="false">No</option>
</select>`
	const yes = `<select name="is_cool">
<option value="unknown">Unknown</option>
<option value="true" selected>Yes</option>
<option value="false">No</option>
</select>`
	const no = `<select name="is_cool">
<option value="unknown">Unknown</option>
<option value="true">Yes</option>
<option value="false" selected>No</option>
</select>`
	newForm := func() *forms.Form {
		return withoutAutoID(forms.New(
			forms.F("name", forms.NewCharField()),
			forms.F("is_cool", forms.NewNullBooleanField()),
		))
	}
	assertHTMLEqual(t, newForm().Bind(vals("name", "Joe")).Field("is_cool").String(), unknown)
	cases := []struct{ value, want string }{
		{"1", unknown},
		{"2", yes},
		{"3", no},
		{"True", yes},
		{"False", no},
		{"unknown", unknown},
		{"true", yes},
		{"false", no},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			assertHTMLEqual(t, newForm().Bind(vals("name", "Joe", "is_cool", c.value)).Field("is_cool").String(), c.want)
		})
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_empty_permitted
//
// A missing value (Django's None) is a key absent from the data.
func TestEmptyPermitted(t *testing.T) {
	songForm := func(emptyPermitted bool) *forms.Form {
		f := textFieldsForm("artist", "name")
		f.EmptyPermitted = emptyPermitted
		f.UseRequiredAttribute = !emptyPermitted
		return f
	}
	data := vals("artist", "", "song", "")
	form := songForm(false).Bind(data)
	equal(t, form.IsValid(), false)
	equal(t, form.Errors().Messages(), map[string][]string{
		"name":   {"This field is required."},
		"artist": {"This field is required."},
	})
	equal(t, form.CleanedData(), map[string]any{})

	// An empty form passes when empty_permitted is set.
	form = songForm(true).Bind(data)
	equal(t, form.IsValid(), true)
	equal(t, form.Errors().Len(), 0)
	equal(t, form.CleanedData(), map[string]any{})

	// With data in one field the whole form must pass validation.
	form = songForm(false).Bind(vals("artist", "The Doors", "song", ""))
	equal(t, form.IsValid(), false)
	equal(t, form.Errors().Messages(), map[string][]string{"name": {"This field is required."}})
	equal(t, form.CleanedData(), map[string]any{"artist": "The Doors"})

	// A field missing from the data counts as empty.
	form = songForm(true).Bind(vals("song", ""))
	equal(t, form.IsValid(), true)

	// An initial value that is falsy is compared literally.
	price := forms.New(
		forms.F("amount", forms.NewFloatField()),
		forms.F("qty", forms.NewIntegerField()),
	)
	price.Initial = map[string]any{"amount": 0.0}
	price.EmptyPermitted = true
	price.UseRequiredAttribute = false
	equal(t, price.Bind(vals("amount", "0.0", "qty", "")).IsValid(), true)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_extracting_hidden_and_visible
func TestExtractingHiddenAndVisible(t *testing.T) {
	form := forms.New(
		forms.F("token", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}))),
		forms.F("artist", forms.NewCharField()),
		forms.F("name", forms.NewCharField()),
	)
	names := func(fields []*forms.BoundField) []string {
		out := make([]string, 0, len(fields))
		for _, bf := range fields {
			out = append(out, bf.Name)
		}
		return out
	}
	equal(t, names(form.HiddenFields()), []string{"token"})
	equal(t, names(form.VisibleFields()), []string{"artist", "name"})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_html_required_html_classes
func TestErrorHTMLRequiredHTMLClasses(t *testing.T) {
	f := forms.New(
		forms.F("name", forms.NewCharField()),
		forms.F("is_cool", forms.NewNullBooleanField()),
		forms.F("email", forms.NewEmailField(forms.Required(false))),
		forms.F("age", forms.NewIntegerField()),
	)
	p := f.Bind(vals())
	p.ErrorCSSClass = "error"
	p.RequiredCSSClass = "required"
	assertHTMLEqual(t, p.AsUL(), `
            <li class="required error"><ul class="errorlist" id="id_name_error">
            <li>This field is required.</li></ul>
            <label class="required" for="id_name">Name:</label>
            <input type="text" name="name" id="id_name" aria-invalid="true" required
             aria-describedby="id_name_error">
            </li><li class="required">
            <label class="required" for="id_is_cool">Is cool:</label>
            <select name="is_cool" id="id_is_cool">
            <option value="unknown" selected>Unknown</option>
            <option value="true">Yes</option>
            <option value="false">No</option>
            </select></li>
            <li><label for="id_email">Email:</label>
            <input type="email" name="email" id="id_email" maxlength="320"></li>
            <li class="required error"><ul class="errorlist" id="id_age_error">
            <li>This field is required.</li></ul>
            <label class="required" for="id_age">Age:</label>
            <input type="number" name="age" id="id_age" aria-invalid="true" required
            aria-describedby="id_age_error">
            </li>`)
	assertHTMLEqual(t, p.AsP(), `
            <ul class="errorlist" id="id_name_error"><li>This field is required.</li>
            </ul><p class="required error">
            <label class="required" for="id_name">Name:</label>
            <input type="text" name="name" id="id_name" aria-invalid="true" required
            aria-describedby="id_name_error">
            </p><p class="required">
            <label class="required" for="id_is_cool">Is cool:</label>
            <select name="is_cool" id="id_is_cool">
            <option value="unknown" selected>Unknown</option>
            <option value="true">Yes</option>
            <option value="false">No</option>
            </select></p>
            <p><label for="id_email">Email:</label>
            <input type="email" name="email" id="id_email" maxlength="320"></p>
            <ul class="errorlist" id="id_age_error"><li>This field is required.</li>
            </ul><p class="required error"><label class="required" for="id_age">
            Age:</label><input type="number" name="age" id="id_age" aria-invalid="true"
            required aria-describedby="id_age_error"></p>`)
	assertHTMLEqual(t, p.AsTable(), `<tr class="required error">
<th><label class="required" for="id_name">Name:</label></th>
<td><ul class="errorlist" id="id_name_error"><li>This field is required.</li></ul>
<input type="text" name="name" id="id_name" aria-invalid="true" required
aria-describedby="id_name_error"></td></tr>
<tr class="required"><th><label class="required" for="id_is_cool">Is cool:</label></th>
<td><select name="is_cool" id="id_is_cool">
<option value="unknown" selected>Unknown</option>
<option value="true">Yes</option>
<option value="false">No</option>
</select></td></tr>
<tr><th><label for="id_email">Email:</label></th><td>
<input type="email" name="email" id="id_email" maxlength="320"></td></tr>
<tr class="required error"><th><label class="required" for="id_age">Age:</label></th>
<td><ul class="errorlist" id="id_age_error"><li>This field is required.</li></ul>
<input type="number" name="age" id="id_age" aria-invalid="true" required
aria-describedby="id_age_error"></td></tr>`)
	assertHTMLEqual(t, p.AsDiv(),
		`<div class="required error"><label for="id_name" class="required">Name:`+
			`</label><ul class="errorlist" id="id_name_error"><li>This field is `+
			`required.</li></ul><input type="text" name="name" required id="id_name" `+
			`aria-invalid="true" aria-describedby="id_name_error" /></div>`+
			`<div class="required"><label for="id_is_cool" class="required">Is cool:`+
			`</label><select name="is_cool" id="id_is_cool">`+
			`<option value="unknown" selected>Unknown</option>`+
			`<option value="true">Yes</option><option value="false">No</option>`+
			`</select></div><div><label for="id_email">Email:</label>`+
			`<input type="email" name="email" id="id_email" maxlength="320"/></div>`+
			`<div class="required error"><label for="id_age" class="required">Age:`+
			`</label><ul class="errorlist" id="id_age_error"><li>This field is `+
			`required.</li></ul><input type="number" name="age" required id="id_age" `+
			`aria-invalid="true" aria-describedby="id_age_error" /></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_label_has_required_css_class
func TestLabelHasRequiredCSSClass(t *testing.T) {
	f := forms.New(
		forms.F("field", forms.NewCharField(forms.MaxLength(10))),
		forms.F("field2", forms.NewIntegerField(forms.Required(false))),
	)
	f.RequiredCSSClass = "required"
	f = f.Bind(vals("field", "test"))
	foo := forms.Attrs{"class": "foo"}
	assertHTMLEqual(t, f.Field("field").LabelTag(), `<label for="id_field" class="required">Field:</label>`)
	assertHTMLEqual(t, f.Field("field").LegendTag(), `<legend class="required">Field:</legend>`)
	assertHTMLEqual(t, f.Field("field").LabelTagWith("", foo, nil, "label"),
		`<label for="id_field" class="foo required">Field:</label>`)
	assertHTMLEqual(t, f.Field("field").LabelTagWith("", foo, nil, "legend"),
		`<legend class="foo required">Field:</legend>`)
	assertHTMLEqual(t, f.Field("field2").LabelTag(), `<label for="id_field2">Field2:</label>`)
	assertHTMLEqual(t, f.Field("field2").LegendTag(), `<legend>Field2:</legend>`)
}
