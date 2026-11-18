package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
)

// songForm is the SongForm of Django's forms tests with the given composers
// field.
func songForm(composers forms.Field) *forms.Form {
	return forms.New(forms.F("name", forms.NewCharField()), forms.F("composers", composers))
}

// composerChoices are the choices of the composers field.
func composerChoices() forms.FieldOption {
	return forms.Choices("J", "John Lennon", "P", "Paul McCartney")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_forms_with_multiple_choice
func TestFormsWithMultipleChoice(t *testing.T) {
	f := songForm(forms.NewMultipleChoiceField())
	f.AutoID = ""
	djangotest.AssertHTMLEqual(t, f.Field("composers").String(), `<select multiple name="composers" required>
</select>`)

	f = songForm(forms.NewMultipleChoiceField(composerChoices()))
	f.AutoID = ""
	djangotest.AssertHTMLEqual(t, f.Field("composers").String(), `<select multiple name="composers" required>
<option value="J">John Lennon</option>
<option value="P">Paul McCartney</option>
</select>`)

	f = f.Bind(vals("name", "Yesterday", "composers", "P"))
	djangotest.AssertHTMLEqual(t, f.Field("name").String(), `<input type="text" name="name" value="Yesterday" required>`)
	djangotest.AssertHTMLEqual(t, f.Field("composers").String(), `<select multiple name="composers" required>
<option value="J">John Lennon</option>
<option value="P" selected>Paul McCartney</option>
</select>`)

	f = songForm(forms.NewMultipleChoiceField(composerChoices()))
	name := `<label for="id_name">Name:</label><input type="text" name="name" required id="id_name">`
	composers := `<label for="id_composers">Composers:</label>` +
		`<select name="composers" required id="id_composers" multiple>` +
		`<option value="J">John Lennon</option>` +
		`<option value="P">Paul McCartney</option></select>`
	djangotest.AssertHTMLEqual(t, f.AsTable(),
		`<tr><th><label for="id_name">Name:</label></th>`+
			`<td><input type="text" name="name" required id="id_name"></td>`+
			`</tr><tr><th><label for="id_composers">Composers:</label></th>`+
			`<td><select name="composers" required id="id_composers" multiple>`+
			`<option value="J">John Lennon</option>`+
			`<option value="P">Paul McCartney</option>`+
			`</select></td></tr>`)
	djangotest.AssertHTMLEqual(t, f.AsUL(), `<li>`+name+`</li><li>`+composers+`</li>`)
	djangotest.AssertHTMLEqual(t, f.AsP(), `<p>`+name+`</p><p>`+composers+`</p>`)
	djangotest.AssertHTMLEqual(t, f.AsDiv(), `<div>`+name+`</div><div>`+composers+`</div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_multiple_checkbox_render
func TestMultipleCheckboxRender(t *testing.T) {
	f := songForm(forms.NewMultipleChoiceField(composerChoices(), forms.UseWidget(forms.CheckboxSelectMultiple{})))
	name := `<label for="id_name">Name:</label><input type="text" name="name" required id="id_name">`
	boxes := `<div id="id_composers">` +
		`<div><label for="id_composers_0">` +
		`<input type="checkbox" name="composers" value="J" id="id_composers_0">John Lennon</label></div>` +
		`<div><label for="id_composers_1">` +
		`<input type="checkbox" name="composers" value="P" id="id_composers_1">Paul McCartney</label></div>` +
		`</div>`

	djangotest.AssertHTMLEqual(t, f.AsTable(),
		`<tr><th><label for="id_name">Name:</label></th><td>`+
			`<input type="text" name="name" required id="id_name"></td></tr>`+
			`<tr><th><label>Composers:</label></th><td>`+boxes+`</td></tr>`)
	djangotest.AssertHTMLEqual(t, f.AsUL(), `<li>`+name+`</li><li><label>Composers:</label>`+boxes+`</li>`)
	djangotest.AssertHTMLEqual(t, f.AsP(), `<p>`+name+`</p><p><label>Composers:</label>`+boxes+`</p>`)
	djangotest.AssertHTMLEqual(t, f.AsDiv(),
		`<div>`+name+`</div><div><fieldset><legend>Composers:</legend>`+boxes+`</fieldset></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_form_with_disabled_fields
func TestFormWithDisabledFields(t *testing.T) {
	birthday := date(1974, 8, 16)
	// personInitial takes the initial value from the form, fieldInitial from
	// the field.
	personInitial := func(data ...string) *forms.Form {
		f := forms.New(
			forms.F("name", forms.NewCharField()),
			forms.F("birthday", forms.NewDateField(forms.Disabled())),
		)
		f.Initial = map[string]any{"birthday": birthday}
		return f.Bind(vals(data...))
	}
	fieldInitial := func(data ...string) *forms.Form {
		return forms.New(
			forms.F("name", forms.NewCharField()),
			forms.F("birthday", forms.NewDateField(forms.Disabled(), forms.Initial(birthday))),
		).Bind(vals(data...))
	}
	want := map[string]any{"birthday": birthday, "name": "John Doe"}

	// A browser does not send a disabled field; the initial value is used.
	for _, form := range []*forms.Form{personInitial("name", "John Doe"), fieldInitial("name", "John Doe")} {
		equal(t, form.IsValid(), true)
		equal(t, form.CleanedData(), want)
	}
	// A submitted value is ignored.
	for _, form := range []*forms.Form{
		personInitial("name", "John Doe", "birthday", "1984-11-10"),
		fieldInitial("name", "John Doe", "birthday", "1984-11-10"),
	} {
		equal(t, form.IsValid(), true)
		equal(t, form.CleanedData(), want)
	}
	// The initial value stays on an invalid form.
	for _, form := range []*forms.Form{personInitial(), fieldInitial()} {
		equal(t, form.IsValid(), false)
		equal(t, form.Field("birthday").Value(), any(birthday))
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_hidden_data
//
// The SplitDateTimeField part is left out; the package has no such field.
func TestHiddenData(t *testing.T) {
	song := func(data ...string) *forms.Form {
		f := songForm(forms.NewMultipleChoiceField(composerChoices()))
		f.AutoID = ""
		return f.Bind(vals(data...))
	}

	// A field with several values renders one hidden input per value.
	f := song("name", "Yesterday", "composers", "P")
	djangotest.AssertHTMLEqual(t, f.Field("composers").AsHidden(nil),
		`<input type="hidden" name="composers" value="P">`)
	f = song("name", "From Me To You", "composers", "P", "composers", "J")
	djangotest.AssertHTMLEqual(t, f.Field("composers").AsHidden(nil), `<input type="hidden" name="composers" value="P">
<input type="hidden" name="composers" value="J">`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_escaping
//
// There is no safe-string type for labels and messages, so the mark_safe half
// of the test is left out: a label and an error message are always escaped.
func TestEscaping(t *testing.T) {
	escaping := func(value string) *forms.Form {
		f := forms.New(forms.F("special_name", forms.NewCharField(forms.Label("<em>Special</em> Field"))))
		f.AutoID = ""
		f.CleanFieldWith("special_name", func(_ *forms.Form, value any) (any, error) {
			return nil, forms.NewValidationError("Something's wrong with '"+value.(string)+"'", "")
		})
		return f.Bind(vals("special_name", value))
	}

	djangotest.AssertHTMLEqual(t, escaping("Nothing to escape").AsTable(), `
		<tr><th>&lt;em&gt;Special&lt;/em&gt; Field:</th><td>
		<ul class="errorlist">
		<li>Something&#x27;s wrong with &#x27;Nothing to escape&#x27;</li></ul>
		<input type="text" name="special_name" value="Nothing to escape"
		aria-invalid="true" required></td></tr>`)
	djangotest.AssertHTMLEqual(t, escaping("Should escape < & > and <script>alert('xss')</script>").AsTable(),
		`<tr><th>&lt;em&gt;Special&lt;/em&gt; Field:</th><td>`+
			`<ul class="errorlist"><li>`+
			`Something&#x27;s wrong with &#x27;Should escape &lt; &amp; &gt; and `+
			`&lt;script&gt;alert(&#x27;xss&#x27;)&lt;/script&gt;&#x27;</li></ul>`+
			`<input type="text" name="special_name" value="Should escape &lt; &amp; `+
			`&gt; and &lt;script&gt;alert(&#x27;xss&#x27;)&lt;/script&gt;" `+
			`aria-invalid="true" required></td></tr>`)
}

// userRegistration is the UserRegistration form of Django's forms tests: the
// two passwords have to match.
func userRegistration() *forms.Form {
	f := forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(10))),
		forms.F("password1", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}), forms.MinLength(5))),
		forms.F("password2", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
	)
	return f.CleanWith(func(f *forms.Form) error {
		p1, _ := f.CleanedData()["password1"].(string)
		p2, _ := f.CleanedData()["password2"].(string)
		if p1 != "" && p2 != "" && p1 != p2 {
			return forms.NewValidationError("Please make sure your passwords match.", "password_mismatch")
		}
		return nil
	})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_has_error
func TestHasError(t *testing.T) {
	f := userRegistration().Bind(vals())
	equal(t, f.HasError("password1"), true)
	equal(t, f.HasError("password1", "required"), true)
	equal(t, f.HasError("password1", "anything"), false)

	f = userRegistration().Bind(vals("password1", "Hi", "password2", "Hi"))
	equal(t, f.HasError("password1"), true)
	equal(t, f.HasError("password1", "min_length"), true)
	equal(t, f.HasError("password1", "anything"), false)
	equal(t, f.HasError("password2"), false)
	equal(t, f.HasError("password2", "anything"), false)

	f = userRegistration().Bind(vals("password1", "Bonjour", "password2", "Hello"))
	equal(t, f.HasError("password1"), false)
	equal(t, f.HasError("password1", "required"), false)
	equal(t, f.HasError(forms.NonFieldErrors), true)
	equal(t, f.HasError(forms.NonFieldErrors, "password_mismatch"), true)
	equal(t, f.HasError(forms.NonFieldErrors, "anything"), false)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_html_output_with_hidden_input_field_errors
func TestHTMLOutputWithHiddenInputFieldErrors(t *testing.T) {
	f := forms.New(forms.F("hidden_input", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}))))
	f.CleanWith(func(f *forms.Form) error {
		f.AddError("", "Form error")
		return nil
	})
	f = f.Bind(vals())

	want := map[string][]string{
		"hidden_input": {"This field is required."},
		"__all__":      {"Form error"},
	}
	equal(t, f.Errors().Messages(), want)
	_ = f.AsTable()
	equal(t, f.Errors().Messages(), want) // rendering does not change the errors

	top := `<ul class="errorlist nonfield"><li>Form error</li>` +
		`<li>(Hidden field hidden_input) This field is required.</li></ul>`
	hidden := `<input type="hidden" name="hidden_input" id="id_hidden_input">`
	djangotest.AssertHTMLEqual(t, f.AsTable(), `<tr><td colspan="2">`+top+hidden+`</td></tr>`)
	djangotest.AssertHTMLEqual(t, f.AsUL(), `<li>`+top+hidden+`</li>`)
	djangotest.AssertHTMLEqual(t, f.AsP(), top+`<p>`+hidden+`</p>`)
	djangotest.AssertHTMLEqual(t, f.AsDiv(), top+`<div>`+hidden+`</div>`)
}
