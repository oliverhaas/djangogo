package forms_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_validating_multiple_fields
//
// AddError records a programming mistake (an unknown field, a dictionary error
// for a named field) in Form.Err where Django raises ValueError and TypeError.
func TestValidatingMultipleFields(t *testing.T) {
	registration := func() *forms.Form {
		f := forms.New(
			forms.F("username", forms.NewCharField(forms.MaxLength(10))),
			forms.F("password1", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
			forms.F("password2", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
		)
		f.AutoID = ""
		return f
	}
	text := func(f *forms.Form, name string) string {
		s, _ := f.CleanedData()[name].(string)
		return s
	}

	// A function registered for a field is the clean_<name>() method. It sees
	// the data cleaned so far, the field itself included.
	perField := func(data ...string) *forms.Form {
		f := registration().CleanFieldWith("password2", func(f *forms.Form, value any) (any, error) {
			if p1, p2 := text(f, "password1"), text(f, "password2"); p1 != "" && p2 != "" && p1 != p2 {
				return nil, forms.NewValidationError("Please make sure your passwords match.", "")
			}
			return value, nil
		})
		if data == nil {
			return f
		}
		return f.Bind(vals(data...))
	}
	equal(t, perField().Errors().Len(), 0)
	f := perField("", "")
	equal(t, errorsOf(f, "username"), []string{"This field is required."})
	equal(t, errorsOf(f, "password1"), []string{"This field is required."})
	equal(t, errorsOf(f, "password2"), []string{"This field is required."})
	f = perField("username", "adrian", "password1", "foo", "password2", "bar")
	equal(t, errorsOf(f, "password2"), []string{"Please make sure your passwords match."})
	f = perField("username", "adrian", "password1", "foo", "password2", "foo")
	equal(t, f.Errors().Len(), 0)
	equal(t, f.CleanedData(), map[string]any{"username": "adrian", "password1": "foo", "password2": "foo"})

	// A function registered for the form is the clean() method. Its error
	// belongs to the form, a dictionary error to the named fields.
	whole := func(data ...string) *forms.Form {
		f := registration().CleanWith(func(f *forms.Form) error {
			p1, p2 := text(f, "password1"), text(f, "password2")
			if p1 != "" && p2 != "" && p1 != p2 {
				return forms.NewValidationError("Please make sure your passwords match.", "")
			}
			errs := map[string]any{}
			if p1 == "FORBIDDEN_VALUE" {
				errs["password1"] = "Forbidden value."
			}
			if p2 == "FORBIDDEN_VALUE" {
				errs["password2"] = []string{"Forbidden value."}
			}
			if len(errs) > 0 {
				return forms.ValidationErrorDict(errs)
			}
			if p1 == "FORBIDDEN_VALUE2" {
				f.AddError("", "Non-field error 1.")
				f.AddError("password1", "Forbidden value 2.")
			}
			if p2 == "FORBIDDEN_VALUE2" {
				f.AddError("password2", "Forbidden value 2.")
				return forms.NewValidationError("Non-field error 2.", "")
			}
			return nil
		})
		if data == nil {
			return f
		}
		return f.Bind(vals(data...))
	}
	equal(t, whole().Errors().Len(), 0)

	f = whole("", "")
	djangotest.AssertHTMLEqual(t, f.AsTable(), `<tr><th>Username:</th><td>
<ul class="errorlist"><li>This field is required.</li></ul>
<input type="text" name="username" maxlength="10" aria-invalid="true" required>
</td></tr>
<tr><th>Password1:</th><td><ul class="errorlist"><li>This field is required.</li></ul>
<input type="password" name="password1" aria-invalid="true" required></td></tr>
<tr><th>Password2:</th><td><ul class="errorlist"><li>This field is required.</li></ul>
<input type="password" name="password2" aria-invalid="true" required></td></tr>`)
	equal(t, errorsOf(f, "username"), []string{"This field is required."})
	equal(t, errorsOf(f, "password1"), []string{"This field is required."})
	equal(t, errorsOf(f, "password2"), []string{"This field is required."})

	f = whole("username", "adrian", "password1", "foo", "password2", "bar")
	equal(t, errorsOf(f, "__all__"), []string{"Please make sure your passwords match."})
	djangotest.AssertHTMLEqual(t, f.AsTable(), `
		<tr><td colspan="2">
		<ul class="errorlist nonfield">
		<li>Please make sure your passwords match.</li></ul></td></tr>
		<tr><th>Username:</th><td>
		<input type="text" name="username" value="adrian" maxlength="10" required>
		</td></tr>
		<tr><th>Password1:</th><td>
		<input type="password" name="password1" required></td></tr>
		<tr><th>Password2:</th><td>
		<input type="password" name="password2" required></td></tr>`)
	djangotest.AssertHTMLEqual(t, f.AsUL(), `
		<li><ul class="errorlist nonfield">
		<li>Please make sure your passwords match.</li></ul></li>
		<li>Username:
		<input type="text" name="username" value="adrian" maxlength="10" required>
		</li>
		<li>Password1: <input type="password" name="password1" required></li>
		<li>Password2: <input type="password" name="password2" required></li>`)
	djangotest.AssertHTMLEqual(t, f.AsDiv(),
		`<ul class="errorlist nonfield"><li>Please make sure your passwords match.`+
			`</li></ul><div>Username: <input type="text" name="username" `+
			`value="adrian" maxlength="10" required></div><div>Password1: <input `+
			`type="password" name="password1" required></div><div>Password2: <input `+
			`type="password" name="password2" required></div>`)

	f = whole("username", "adrian", "password1", "foo", "password2", "foo")
	equal(t, f.Errors().Len(), 0)
	equal(t, f.CleanedData(), map[string]any{"username": "adrian", "password1": "foo", "password2": "foo"})

	f = whole("username", "adrian", "password1", "FORBIDDEN_VALUE", "password2", "FORBIDDEN_VALUE")
	equal(t, errorsOf(f, "password1"), []string{"Forbidden value."})
	equal(t, errorsOf(f, "password2"), []string{"Forbidden value."})

	f = whole("username", "adrian", "password1", "FORBIDDEN_VALUE2", "password2", "FORBIDDEN_VALUE2")
	equal(t, errorsOf(f, "__all__"), []string{"Non-field error 1.", "Non-field error 2."})
	equal(t, errorsOf(f, "password1"), []string{"Forbidden value 2."})
	equal(t, errorsOf(f, "password2"), []string{"Forbidden value 2."})

	equal(t, f.Err(), error(nil))
	f.AddError("missing_field", "Some error.")
	if f.Err() == nil || !strings.Contains(f.Err().Error(), "has no field named") {
		t.Errorf("AddError for an unknown field: Err() = %v", f.Err())
	}
	f = whole("username", "adrian", "password1", "foo", "password2", "foo")
	f.AddError("password1", forms.ValidationErrorDict(map[string]any{"password1": "Some error."}))
	if f.Err() == nil || !strings.Contains(f.Err().Error(), "dictionary error") {
		t.Errorf("AddError of a dictionary for a named field: Err() = %v", f.Err())
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_dict
func TestErrorDict(t *testing.T) {
	form := forms.New(
		forms.F("foo", forms.NewCharField()),
		forms.F("bar", forms.NewCharField()),
	).CleanWith(func(*forms.Form) error {
		return forms.NewValidationError("Non-field error.", "secret", map[string]any{"a": 1, "b": 2})
	}).Bind(vals())
	equal(t, form.IsValid(), false)

	text := form.Errors().AsText()
	for _, want := range []string{
		"* foo\n  * This field is required.",
		"* bar\n  * This field is required.",
		"* __all__\n  * Non-field error.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("AsText() = %q, missing %q", text, want)
		}
	}

	ul := form.Errors().AsUL()
	for _, want := range []string{
		`<li>foo<ul class="errorlist" id="id_foo_error"><li>This field is required.</li></ul></li>`,
		`<li>bar<ul class="errorlist" id="id_bar_error"><li>This field is required.</li></ul></li>`,
		`<li>__all__<ul class="errorlist nonfield"><li>Non-field error.</li></ul></li>`,
	} {
		djangotest.AssertInHTML(t, want, ul)
	}

	equal(t, form.Errors().JSONData(false), map[string][]forms.ErrorJSON{
		"foo":     {{Code: "required", Message: "This field is required."}},
		"bar":     {{Code: "required", Message: "This field is required."}},
		"__all__": {{Code: "secret", Message: "Non-field error."}},
	})
	djangotest.AssertJSONEqual(t, form.Errors().AsJSON(false), map[string]any{
		"foo":     []any{map[string]any{"code": "required", "message": "This field is required."}},
		"bar":     []any{map[string]any{"code": "required", "message": "This field is required."}},
		"__all__": []any{map[string]any{"code": "secret", "message": "Non-field error."}},
	})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_dict_as_json_escape_html
func TestErrorDictAsJSONEscapeHTML(t *testing.T) {
	form := forms.New(
		forms.F("foo", forms.NewCharField()),
		forms.F("bar", forms.NewCharField()),
	).CleanWith(func(*forms.Form) error {
		return forms.NewValidationError("<p>Non-field error.</p>", "secret", map[string]any{"a": 1, "b": 2})
	}).Bind(vals())
	equal(t, form.IsValid(), false)

	control := func(message string) map[string]any {
		return map[string]any{
			"foo":     []any{map[string]any{"code": "required", "message": "This field is required."}},
			"bar":     []any{map[string]any{"code": "required", "message": "This field is required."}},
			"__all__": []any{map[string]any{"code": "secret", "message": message}},
		}
	}
	djangotest.AssertJSONEqual(t, form.Errors().AsJSON(false), control("<p>Non-field error.</p>"))

	escaped := "&lt;p&gt;Non-field error.&lt;/p&gt;"
	equal(t, form.Errors().JSONData(true)["__all__"][0].Message, escaped)
	djangotest.AssertJSONEqual(t, form.Errors().AsJSON(true), control(escaped))
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_list
//
// AsJSON is compact; Django's has a space after each separator.
func TestErrorList(t *testing.T) {
	e := forms.NewErrorList("Foo",
		forms.NewValidationError("Foo%(bar)s", "foobar", map[string]any{"bar": "bar"}))

	equal(t, e.Contains("Foo"), true)
	equal(t, forms.ValidationErrors(e.AsData()).Messages()[0], "Foo")
	equal(t, e.AsText(), "* Foo\n* Foobar")
	equal(t, e.AsUL(), `<ul class="errorlist"><li>Foo</li><li>Foobar</li></ul>`)
	equal(t, e.JSONData(false), []forms.ErrorJSON{{Message: "Foo", Code: ""}, {Message: "Foobar", Code: "foobar"}})
	equal(t, e.AsJSON(false), `[{"message":"Foo","code":""},{"message":"Foobar","code":"foobar"}]`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_escaping
func TestErrorEscaping(t *testing.T) {
	fail := func(*forms.Form, any) (any, error) {
		return nil, forms.NewValidationError(`Foo & "bar"!`, "")
	}
	form := forms.New(
		forms.F("hidden", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}), forms.Required(false))),
		forms.F("visible", forms.NewCharField()),
	).CleanFieldWith("hidden", fail).CleanFieldWith("visible", fail).Bind(vals("hidden", "a", "visible", "b"))

	equal(t, form.IsValid(), false)
	djangotest.AssertHTMLEqual(t, form.AsUL(),
		`<li><ul class="errorlist nonfield">`+
			`<li>(Hidden field hidden) Foo &amp; &quot;bar&quot;!</li></ul></li>`+
			`<li><ul class="errorlist" id="id_visible_error"><li>Foo &amp; `+
			`&quot;bar&quot;!</li></ul>`+
			`<label for="id_visible">Visible:</label> `+
			`<input type="text" name="visible" aria-invalid="true" value="b" `+
			`id="id_visible" required aria-describedby="id_visible_error">`+
			`<input type="hidden" name="hidden" value="a" id="id_hidden"></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_only_hidden_fields
func TestOnlyHiddenFields(t *testing.T) {
	f := forms.New(forms.F("data", forms.NewIntegerField(forms.UseWidget(forms.HiddenInput{})))).Bind(vals())

	djangotest.AssertHTMLEqual(t, f.AsP(),
		`<ul class="errorlist nonfield">`+
			`<li>(Hidden field data) This field is required.</li></ul>`+"\n<p> "+
			`<input type="hidden" name="data" id="id_data"></p>`)
	djangotest.AssertHTMLEqual(t, f.AsTable(),
		`<tr><td colspan="2"><ul class="errorlist nonfield">`+
			`<li>(Hidden field data) This field is required.</li></ul>`+
			`<input type="hidden" name="data" id="id_data"></td></tr>`)
}
