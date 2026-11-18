package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_form
//
// Form.Field returns nil for an unknown name where Django raises KeyError, and
// a cleaned date is a time.Time at midnight UTC.
func TestForm(t *testing.T) {
	p := personForm().Bind(vals("first_name", "John", "last_name", "Lennon", "birthday", "1940-10-9"))

	equal(t, p.IsBound(), true)
	equal(t, p.Errors().Len(), 0)
	equal(t, p.IsValid(), true)
	djangotest.AssertHTMLEqual(t, p.Errors().AsUL(), "")
	equal(t, p.Errors().AsText(), "")
	equal(t, p.CleanedData()["first_name"], any("John"))
	equal(t, p.CleanedData()["last_name"], any("Lennon"))
	equal(t, p.CleanedData()["birthday"], any(date(1940, 10, 9)))
	djangotest.AssertHTMLEqual(t, p.Field("first_name").String(),
		`<input type="text" name="first_name" value="John" id="id_first_name" required>`)
	djangotest.AssertHTMLEqual(t, p.Field("last_name").String(),
		`<input type="text" name="last_name" value="Lennon" id="id_last_name" required>`)
	djangotest.AssertHTMLEqual(t, p.Field("birthday").String(),
		`<input type="text" name="birthday" value="1940-10-9" id="id_birthday" required>`)

	if p.Field("nonexistentfield") != nil {
		t.Error("Field of an unknown name is not nil")
	}

	var output string
	labelsAndData := make([][2]any, 0, len(p.Fields()))
	for _, bf := range p.Fields() {
		output += bf.String() + "\n"
		labelsAndData = append(labelsAndData, [2]any{bf.Label(), bf.Data()})
	}
	djangotest.AssertHTMLEqual(t, output,
		`<input type="text" name="first_name" value="John" id="id_first_name" required>`+
			`<input type="text" name="last_name" value="Lennon" id="id_last_name" required>`+
			`<input type="text" name="birthday" value="1940-10-9" id="id_birthday" required>`)
	equal(t, labelsAndData, [][2]any{
		{"First name", "John"},
		{"Last name", "Lennon"},
		{"Birthday", "1940-10-9"},
	})

	want := `<div><label for="id_first_name">First name:</label><input type="text" ` +
		`name="first_name" value="John" required id="id_first_name"></div><div>` +
		`<label for="id_last_name">Last name:</label><input type="text" ` +
		`name="last_name" value="Lennon" required id="id_last_name"></div><div>` +
		`<label for="id_birthday">Birthday:</label><input type="text" ` +
		`name="birthday" value="1940-10-9" required id="id_birthday"></div>`
	djangotest.AssertHTMLEqual(t, p.String(), want)
	djangotest.AssertHTMLEqual(t, p.AsDiv(), want)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_empty_dict
func TestEmptyDict(t *testing.T) {
	p := personForm().Bind(vals())

	equal(t, p.IsBound(), true)
	equal(t, errorsOf(p, "first_name"), []string{"This field is required."})
	equal(t, errorsOf(p, "last_name"), []string{"This field is required."})
	equal(t, errorsOf(p, "birthday"), []string{"This field is required."})
	equal(t, p.IsValid(), false)
	equal(t, len(p.CleanedData()), 0)

	asDiv := `<div><label for="id_first_name">First name:</label>` +
		`<ul class="errorlist" id="id_first_name_error"><li>This field is required.` +
		`</li></ul><input type="text" name="first_name" aria-invalid="true" ` +
		`required id="id_first_name" aria-describedby="id_first_name_error"></div>` +
		`<div><label for="id_last_name">Last name:</label>` +
		`<ul class="errorlist" id="id_last_name_error"><li>This field is required.` +
		`</li></ul><input type="text" name="last_name" aria-invalid="true" ` +
		`required id="id_last_name" aria-describedby="id_last_name_error"></div>` +
		`<div><label for="id_birthday">Birthday:</label>` +
		`<ul class="errorlist" id="id_birthday_error"><li>This field is required.` +
		`</li></ul><input type="text" name="birthday" aria-invalid="true" required ` +
		`id="id_birthday" aria-describedby="id_birthday_error"></div>`
	djangotest.AssertHTMLEqual(t, p.String(), asDiv)
	djangotest.AssertHTMLEqual(t, p.AsDiv(), asDiv)
	djangotest.AssertHTMLEqual(t, p.AsTable(), `<tr><th><label for="id_first_name">First name:</label></th><td>
<ul class="errorlist" id="id_first_name_error"><li>This field is required.</li></ul>
<input type="text" name="first_name" id="id_first_name" aria-invalid="true" required
aria-describedby="id_first_name_error">
</td></tr><tr><th><label for="id_last_name">Last name:</label></th>
<td><ul class="errorlist" id="id_last_name_error"><li>This field is required.</li></ul>
<input type="text" name="last_name" id="id_last_name" aria-invalid="true" required
aria-describedby="id_last_name_error">
</td></tr><tr><th><label for="id_birthday">Birthday:</label></th>
<td><ul class="errorlist" id="id_birthday_error"><li>This field is required.</li></ul>
<input type="text" name="birthday" id="id_birthday" aria-invalid="true" required
aria-describedby="id_birthday_error">
</td></tr>`)
	djangotest.AssertHTMLEqual(t, p.AsUL(), `<li><ul class="errorlist" id="id_first_name_error">
<li>This field is required.</li></ul>
<label for="id_first_name">First name:</label>
<input type="text" name="first_name" id="id_first_name" aria-invalid="true" required
aria-describedby="id_first_name_error">
</li><li><ul class="errorlist" id="id_last_name_error"><li>This field is required.</li>
</ul><label for="id_last_name">Last name:</label>
<input type="text" name="last_name" id="id_last_name" aria-invalid="true" required
aria-describedby="id_last_name_error">
</li><li><ul class="errorlist" id="id_birthday_error"><li>This field is required.</li>
</ul><label for="id_birthday">Birthday:</label>
<input type="text" name="birthday" id="id_birthday" aria-invalid="true" required
aria-describedby="id_birthday_error">
</li>`)
	djangotest.AssertHTMLEqual(t, p.AsP(), `<ul class="errorlist" id="id_first_name_error"><li>
This field is required.</li></ul>
<p><label for="id_first_name">First name:</label>
<input type="text" name="first_name" id="id_first_name" aria-invalid="true" required
aria-describedby="id_first_name_error">
</p><ul class="errorlist" id="id_last_name_error"><li>This field is required.</li></ul>
<p><label for="id_last_name">Last name:</label>
<input type="text" name="last_name" id="id_last_name" aria-invalid="true" required
aria-describedby="id_last_name_error">
</p><ul class="errorlist" id="id_birthday_error"><li>This field is required.</li></ul>
<p><label for="id_birthday">Birthday:</label>
<input type="text" name="birthday" id="id_birthday" aria-invalid="true" required
aria-describedby="id_birthday_error">
</p>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_unbound_form
//
// CleanedData of an unbound form is nil where Django raises AttributeError.
func TestUnboundForm(t *testing.T) {
	p := personForm()

	equal(t, p.IsBound(), false)
	equal(t, p.Errors().Len(), 0)
	equal(t, p.IsValid(), false)
	if p.CleanedData() != nil {
		t.Errorf("CleanedData of an unbound form = %v, want nil", p.CleanedData())
	}

	asDiv := `<div><label for="id_first_name">First name:</label><input type="text" ` +
		`name="first_name" id="id_first_name" required></div><div><label ` +
		`for="id_last_name">Last name:</label><input type="text" name="last_name" ` +
		`id="id_last_name" required></div><div><label for="id_birthday">` +
		`Birthday:</label><input type="text" name="birthday" id="id_birthday" ` +
		`required></div>`
	djangotest.AssertHTMLEqual(t, p.String(), asDiv)
	djangotest.AssertHTMLEqual(t, p.AsDiv(), asDiv)
	djangotest.AssertHTMLEqual(t, p.AsTable(), `<tr><th><label for="id_first_name">First name:</label></th><td>
<input type="text" name="first_name" id="id_first_name" required></td></tr>
<tr><th><label for="id_last_name">Last name:</label></th><td>
<input type="text" name="last_name" id="id_last_name" required></td></tr>
<tr><th><label for="id_birthday">Birthday:</label></th><td>
<input type="text" name="birthday" id="id_birthday" required></td></tr>`)
	djangotest.AssertHTMLEqual(t, p.AsUL(), `<li><label for="id_first_name">First name:</label>
<input type="text" name="first_name" id="id_first_name" required></li>
<li><label for="id_last_name">Last name:</label>
<input type="text" name="last_name" id="id_last_name" required></li>
<li><label for="id_birthday">Birthday:</label>
<input type="text" name="birthday" id="id_birthday" required></li>`)
	djangotest.AssertHTMLEqual(t, p.AsP(), `<p><label for="id_first_name">First name:</label>
<input type="text" name="first_name" id="id_first_name" required></p>
<p><label for="id_last_name">Last name:</label>
<input type="text" name="last_name" id="id_last_name" required></p>
<p><label for="id_birthday">Birthday:</label>
<input type="text" name="birthday" id="id_birthday" required></p>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_cleaned_data_only_fields
func TestCleanedDataOnlyFields(t *testing.T) {
	p := personForm().Bind(vals(
		"first_name", "John", "last_name", "Lennon", "birthday", "1940-10-9",
		"extra1", "hello", "extra2", "hello"))

	equal(t, p.IsValid(), true)
	equal(t, p.CleanedData(), map[string]any{
		"first_name": "John", "last_name": "Lennon", "birthday": date(1940, 10, 9),
	})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_optional_data
func TestOptionalData(t *testing.T) {
	f := forms.New(
		forms.F("first_name", forms.NewCharField()),
		forms.F("last_name", forms.NewCharField()),
		forms.F("nick_name", forms.NewCharField(forms.Required(false))),
	).Bind(vals("first_name", "John", "last_name", "Lennon"))
	equal(t, f.IsValid(), true)
	equal(t, f.CleanedData(), map[string]any{"first_name": "John", "last_name": "Lennon", "nick_name": ""})

	// For a DateField the value is nil.
	f = forms.New(
		forms.F("first_name", forms.NewCharField()),
		forms.F("last_name", forms.NewCharField()),
		forms.F("birth_date", forms.NewDateField(forms.Required(false))),
	).Bind(vals("first_name", "John", "last_name", "Lennon"))
	equal(t, f.IsValid(), true)
	equal(t, f.CleanedData(), map[string]any{"first_name": "John", "last_name": "Lennon", "birth_date": nil})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_auto_id
func TestAutoID(t *testing.T) {
	p := personForm()
	p.AutoID = "%s_id"

	djangotest.AssertHTMLEqual(t, p.AsTable(), `<tr><th><label for="first_name_id">First name:</label></th><td>
<input type="text" name="first_name" id="first_name_id" required></td></tr>
<tr><th><label for="last_name_id">Last name:</label></th><td>
<input type="text" name="last_name" id="last_name_id" required></td></tr>
<tr><th><label for="birthday_id">Birthday:</label></th><td>
<input type="text" name="birthday" id="birthday_id" required></td></tr>`)
	djangotest.AssertHTMLEqual(t, p.AsUL(), `<li><label for="first_name_id">First name:</label>
<input type="text" name="first_name" id="first_name_id" required></li>
<li><label for="last_name_id">Last name:</label>
<input type="text" name="last_name" id="last_name_id" required></li>
<li><label for="birthday_id">Birthday:</label>
<input type="text" name="birthday" id="birthday_id" required></li>`)
	djangotest.AssertHTMLEqual(t, p.AsP(), `<p><label for="first_name_id">First name:</label>
<input type="text" name="first_name" id="first_name_id" required></p>
<p><label for="last_name_id">Last name:</label>
<input type="text" name="last_name" id="last_name_id" required></p>
<p><label for="birthday_id">Birthday:</label>
<input type="text" name="birthday" id="birthday_id" required></p>`)
	djangotest.AssertHTMLEqual(t, p.AsDiv(),
		`<div><label for="first_name_id">First name:</label><input type="text" `+
			`name="first_name" id="first_name_id" required></div><div><label `+
			`for="last_name_id">Last name:</label><input type="text" `+
			`name="last_name" id="last_name_id" required></div><div><label `+
			`for="birthday_id">Birthday:</label><input type="text" name="birthday" `+
			`id="birthday_id" required></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_auto_id_true
//
// Django's auto_id=True is the pattern "%s".
func TestAutoIDTrue(t *testing.T) {
	p := personForm()
	p.AutoID = "%s"

	djangotest.AssertHTMLEqual(t, p.AsUL(), `<li><label for="first_name">First name:</label>
<input type="text" name="first_name" id="first_name" required></li>
<li><label for="last_name">Last name:</label>
<input type="text" name="last_name" id="last_name" required></li>
<li><label for="birthday">Birthday:</label>
<input type="text" name="birthday" id="birthday" required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_auto_id_false
//
// Django's auto_id=False is the empty pattern.
func TestAutoIDFalse(t *testing.T) {
	p := personForm()
	p.AutoID = ""

	djangotest.AssertHTMLEqual(t, p.AsUL(), `<li>First name: <input type="text" name="first_name" required></li>
<li>Last name: <input type="text" name="last_name" required></li>
<li>Birthday: <input type="text" name="birthday" required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_id_on_field
func TestIDOnField(t *testing.T) {
	p := personNewForm()
	p.AutoID = ""

	djangotest.AssertHTMLEqual(t, p.AsUL(), `<li><label for="first_name_id">First name:</label>
<input type="text" id="first_name_id" name="first_name" required></li>
<li>Last name: <input type="text" name="last_name" required></li>
<li>Birthday: <input type="text" name="birthday" required></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_auto_id_on_form_and_field
func TestAutoIDOnFormAndField(t *testing.T) {
	p := personNewForm()
	p.AutoID = "%s"

	djangotest.AssertHTMLEqual(t, p.AsUL(), `<li><label for="first_name_id">First name:</label>
<input type="text" id="first_name_id" name="first_name" required></li>
<li><label for="last_name">Last name:</label>
<input type="text" name="last_name" id="last_name" required></li>
<li><label for="birthday">Birthday:</label>
<input type="text" name="birthday" id="birthday" required></li>`)
}
