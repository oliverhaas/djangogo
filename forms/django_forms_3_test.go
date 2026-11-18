package forms_test

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/templates"
)

// jsonLikeField is the CustomJSONField of Django's test: a CharField whose
// to_python turns "{}" into an empty object.
type jsonLikeField struct {
	forms.CharField
}

// newJSONLikeField returns a jsonLikeField with a text widget.
func newJSONLikeField() *jsonLikeField {
	f := &jsonLikeField{}
	f.Init(f, forms.TextInput{}, nil)
	return f
}

// ToGo returns an empty map for "{}", as the fake json.loads of the test does.
func (f *jsonLikeField) ToGo(value any) (any, error) {
	if value == "{}" {
		return map[string]any{}, nil
	}
	return f.CharField.ToGo(value)
}

// customIDForLabelInput is a TextInput whose label points at "custom_" + id.
type customIDForLabelInput struct {
	forms.TextInput
}

// IDForLabel prefixes the id with "custom_".
func (customIDForLabelInput) IDForLabel(id string) string { return "custom_" + id }

// emptyIDForLabelInput is a TextInput whose label has no for attribute.
type emptyIDForLabelInput struct {
	forms.TextInput
}

// IDForLabel returns no id.
func (emptyIDForLabelInput) IDForLabel(string) string { return "" }

// singleFieldForm returns a form with one required CharField named "field".
func singleFieldForm() *forms.Form {
	return forms.New(forms.F("field", forms.NewCharField()))
}

// userForm is the UserForm of the clean() tests: a username and a password.
func userForm() *forms.Form {
	return forms.New(
		forms.F("username", forms.NewCharField(forms.MaxLength(10))),
		forms.F("password", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
	)
}

// userRegistrationForm is the UserRegistration form of the template tests;
// help adds the help text of the username field.
func userRegistrationForm(help bool) *forms.Form {
	var opts []forms.FieldOption
	if help {
		opts = append(opts, forms.HelpText("Good luck picking a username that doesn't already exist."))
	}
	f := forms.New(
		forms.F("username", forms.NewCharField(append(opts, forms.MaxLength(10))...)),
		forms.F("password1", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
		forms.F("password2", forms.NewCharField(forms.UseWidget(forms.PasswordInput{}))),
	)
	f.CleanWith(func(form *forms.Form) error {
		c := form.CleanedData()
		p1, _ := c["password1"].(string)
		p2, _ := c["password2"].(string)
		if p1 != "" && p2 != "" && p1 != p2 {
			return forms.NewValidationError("Please make sure your passwords match.", "")
		}
		return nil
	})
	return f
}

// renderTemplate renders src with the forms of ctx converted for the engine.
func renderTemplate(t *testing.T, src string, ctx map[string]any) string {
	t.Helper()
	eng, err := templates.NewEngineFS(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range ctx {
		if f, ok := v.(*forms.Form); ok {
			ctx[k] = f.TemplateValue()
		}
	}
	out, err := eng.RenderString(src, ctx)
	if err != nil {
		t.Fatalf("RenderString(%q): %v", src, err)
	}
	return out
}

// Django's empty_values is fixed in Go (nil, "" and empty lists), and an empty
// map is not one of them, so the check is that the cleaned empty object stays.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_custom_empty_values
func TestCustomEmptyValues(t *testing.T) {
	form := forms.New(forms.F("json", newJSONLikeField())).Bind(vals("json", "{}"))
	form.Errors()
	equal(t, form.CleanedData(), map[string]any{"json": map[string]any{}})
}

// The mark_safe("custom&") case is omitted: the contents of LabelTagWith are
// always text and escaped.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_label_tag
func TestBoundfieldLabelTag(t *testing.T) {
	bf := singleFieldForm().Field("field")
	cases := []struct {
		name, contents string
		attrs          forms.Attrs
		label, legend  string
	}{
		{"plain", "", nil, `<label for="id_field">Field:</label>`, "<legend>Field:</legend>"},
		{"custom", "custom", nil, `<label for="id_field">custom:</label>`, "<legend>custom:</legend>"},
		{"escaped", "custom&", nil, `<label for="id_field">custom&amp;:</label>`, "<legend>custom&amp;:</legend>"},
		{
			"attrs", "",
			forms.Attrs{"class": "pretty"},
			`<label for="id_field" class="pretty">Field:</label>`, `<legend class="pretty">Field:</legend>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertHTMLEqual(t, bf.LabelTagWith(c.contents, c.attrs, nil, "label"), c.label)
			assertHTMLEqual(t, bf.LabelTagWith(c.contents, c.attrs, nil, "legend"), c.legend)
		})
	}
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_label_tag_no_id
func TestBoundfieldLabelTagNoID(t *testing.T) {
	form := singleFieldForm()
	form.AutoID = ""
	bf := form.Field("field")
	assertHTMLEqual(t, bf.LabelTag(), "Field:")
	assertHTMLEqual(t, bf.LegendTag(), "Field:")
	assertHTMLEqual(t, bf.LabelTagWith("Custom&", nil, nil, "label"), "Custom&amp;:")
	assertHTMLEqual(t, bf.LabelTagWith("Custom&", nil, nil, "legend"), "Custom&amp;:")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_label_tag_custom_widget_id_for_label
func TestBoundfieldLabelTagCustomWidgetIDForLabel(t *testing.T) {
	form := forms.New(
		forms.F("custom", forms.NewCharField(forms.UseWidget(customIDForLabelInput{}))),
		forms.F("empty", forms.NewCharField(forms.UseWidget(emptyIDForLabelInput{}))),
	)
	assertHTMLEqual(t, form.Field("custom").LabelTag(), `<label for="custom_id_custom">Custom:</label>`)
	assertHTMLEqual(t, form.Field("custom").LegendTag(), "<legend>Custom:</legend>")
	assertHTMLEqual(t, form.Field("empty").LabelTag(), "<label>Empty:</label>")
	assertHTMLEqual(t, form.Field("empty").LegendTag(), "<legend>Empty:</legend>")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_empty_label
func TestBoundfieldEmptyLabel(t *testing.T) {
	form := forms.New(forms.F("field", forms.NewCharField(forms.Label(""))))
	bf := form.Field("field")
	assertHTMLEqual(t, bf.LabelTag(), `<label for="id_field"></label>`)
	assertHTMLEqual(t, bf.LegendTag(), "<legend></legend>")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_id_for_label
func TestBoundfieldIDForLabel(t *testing.T) {
	form := forms.New(forms.F("field", forms.NewCharField(forms.Label(""))))
	equal(t, form.Field("field").IDForLabel(), "id_field")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_id_for_label_override_by_attrs
func TestBoundfieldIDForLabelOverrideByAttrs(t *testing.T) {
	form := forms.New(
		forms.F("field", forms.NewCharField(forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"id": "myCustomID"}}))),
		forms.F("field_none", forms.NewCharField(forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"id": nil}}))),
	)
	equal(t, form.Field("field").IDForLabel(), "myCustomID")
	equal(t, form.Field("field_none").IDForLabel(), "id_field_none")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_css_classes
func TestBoundfieldCSSClasses(t *testing.T) {
	field := personForm().Field("first_name")
	equal(t, field.CSSClasses(), "")
	equal(t, field.CSSClasses(""), "")
	equal(t, field.CSSClasses("test"), "test")
	equal(t, field.CSSClasses("test test"), "test")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_label_suffix_override
func TestLabelSuffixOverride(t *testing.T) {
	form := singleFieldForm()
	form.LabelSuffix = "!"
	bf := form.Field("field")
	suffix := "$"
	assertHTMLEqual(t, bf.LabelTagWith("", nil, &suffix, "label"), `<label for="id_field">Field$</label>`)
	assertHTMLEqual(t, bf.LabelTagWith("", nil, &suffix, "legend"), "<legend>Field$</legend>")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_list_class_not_specified
func TestErrorListClassNotSpecified(t *testing.T) {
	e := forms.NewErrorList("Foo", forms.NewValidationError("Foo%(bar)s", "foobar", map[string]any{"bar": "bar"}))
	equal(t, e.AsUL(), `<ul class="errorlist"><li>Foo</li><li>Foobar</li></ul>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_list_with_hidden_field_errors_has_correct_class
func TestErrorListWithHiddenFieldErrorsHasCorrectClass(t *testing.T) {
	newForm := func() *forms.Form {
		return forms.New(
			forms.F("first_name", forms.NewCharField()),
			forms.F("last_name", forms.NewCharField(forms.UseWidget(forms.HiddenInput{}))),
		).Bind(vals("first_name", "John"))
	}
	assertHTMLEqual(t, newForm().AsUL(), `<li><ul class="errorlist nonfield">
<li>(Hidden field last_name) This field is required.</li></ul></li><li>
<label for="id_first_name">First name:</label>
<input id="id_first_name" name="first_name" type="text" value="John" required>
<input id="id_last_name" name="last_name" type="hidden"></li>`)
	assertHTMLEqual(t, newForm().AsP(), `
<ul class="errorlist nonfield">
<li>(Hidden field last_name) This field is required.</li></ul>
<p><label for="id_first_name">First name:</label>
<input id="id_first_name" name="first_name" type="text" value="John"
    required>
<input id="id_last_name" name="last_name" type="hidden"></p>`)
	assertHTMLEqual(t, newForm().AsTable(), `<tr><td colspan="2"><ul class="errorlist nonfield">
<li>(Hidden field last_name) This field is required.</li></ul></td></tr>
<tr><th><label for="id_first_name">First name:</label></th><td>
<input id="id_first_name" name="first_name" type="text" value="John" required>
<input id="id_last_name" name="last_name" type="hidden"></td></tr>`)
	assertHTMLEqual(t, newForm().AsDiv(),
		`<ul class="errorlist nonfield"><li>(Hidden field last_name) This field `+
			`is required.</li></ul><div><label for="id_first_name">First name:</label>`+
			`<input id="id_first_name" name="first_name" type="text" value="John" `+
			`required><input id="id_last_name" name="last_name" type="hidden"></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_error_list_with_non_field_errors_has_correct_class
func TestErrorListWithNonFieldErrorsHasCorrectClass(t *testing.T) {
	newForm := func() *forms.Form {
		f := forms.New(
			forms.F("first_name", forms.NewCharField()),
			forms.F("last_name", forms.NewCharField()),
		)
		f.CleanWith(func(*forms.Form) error {
			return forms.NewValidationError("Generic validation error", "")
		})
		return f.Bind(vals("first_name", "John", "last_name", "Lennon"))
	}
	p := newForm()
	assertHTMLEqual(t, p.NonFieldErrors().String(),
		`<ul class="errorlist nonfield"><li>Generic validation error</li></ul>`)
	assertHTMLEqual(t, p.AsUL(), `<li>
<ul class="errorlist nonfield"><li>Generic validation error</li></ul></li>
<li><label for="id_first_name">First name:</label>
<input id="id_first_name" name="first_name" type="text" value="John" required></li>
<li><label for="id_last_name">Last name:</label>
<input id="id_last_name" name="last_name" type="text" value="Lennon" required></li>`)
	assertHTMLEqual(t, p.NonFieldErrors().AsText(), "* Generic validation error")
	assertHTMLEqual(t, p.AsP(), `<ul class="errorlist nonfield"><li>Generic validation error</li></ul>
<p><label for="id_first_name">First name:</label>
<input id="id_first_name" name="first_name" type="text" value="John" required></p>
<p><label for="id_last_name">Last name:</label>
<input id="id_last_name" name="last_name" type="text" value="Lennon" required></p>`)
	assertHTMLEqual(t, p.AsTable(), `
<tr><td colspan="2"><ul class="errorlist nonfield">
<li>Generic validation error</li></ul></td></tr>
<tr><th><label for="id_first_name">First name:</label></th><td>
<input id="id_first_name" name="first_name" type="text" value="John"
    required>
</td></tr>
<tr><th><label for="id_last_name">Last name:</label></th><td>
<input id="id_last_name" name="last_name" type="text" value="Lennon"
    required>
</td></tr>`)
	assertHTMLEqual(t, p.AsDiv(),
		`<ul class="errorlist nonfield"><li>Generic validation error</li></ul>`+
			`<div><label for="id_first_name">First name:</label><input `+
			`id="id_first_name" name="first_name" type="text" value="John" required>`+
			`</div><div><label for="id_last_name">Last name:</label><input `+
			`id="id_last_name" name="last_name" type="text" value="Lennon" required>`+
			"</div>")
}

// Python's repr() has no Go counterpart, so only the claim that the form is
// not validated by looking at it is kept: nothing validates until asked.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_baseform_repr_dont_trigger_validation
func TestBaseformReprDontTriggerValidation(t *testing.T) {
	p := personForm().Bind(vals("first_name", "John", "last_name", "Lennon", "birthday", "fakedate"))
	if p.CleanedData() != nil {
		t.Errorf("CleanedData before validation = %v, want nil", p.CleanedData())
	}
	if p.IsValid() {
		t.Error("IsValid() = true, want false")
	}
	equal(t, p.CleanedData(), map[string]any{"first_name": "John", "last_name": "Lennon"})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_accessing_clean
func TestAccessingClean(t *testing.T) {
	f := userForm()
	f.CleanWith(func(form *forms.Form) error {
		data := form.CleanedData()
		if form.Errors().Len() == 0 {
			data["username"] = strings.ToLower(data["username"].(string))
		}
		return nil
	})
	f = f.Bind(vals("username", "SirRobin", "password", "blue"))
	if !f.IsValid() {
		t.Fatalf("form invalid: %v", f.Errors().Messages())
	}
	equal(t, f.CleanedData()["username"], "sirrobin")
}

// Go's clean() has no return value, so the Django case of a clean() that
// returns nothing is the same code path as TestAccessingClean.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_changing_cleaned_data_nothing_returned
func TestChangingCleanedDataNothingReturned(t *testing.T) {
	f := userForm()
	f.CleanWith(func(form *forms.Form) error {
		form.CleanedData()["username"] = strings.ToLower(form.CleanedData()["username"].(string))
		return nil
	})
	f = f.Bind(vals("username", "SirRobin", "password", "blue"))
	if !f.IsValid() {
		t.Fatalf("form invalid: %v", f.Errors().Messages())
	}
	equal(t, f.CleanedData()["username"], "sirrobin")
}

// The check that str(form) equals form.__html__() has no Go meaning; the Go
// counterpart of both is String, which renders the default layout.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_html_safe
func TestHTMLSafe(t *testing.T) {
	form := forms.New(forms.F("username", forms.NewCharField()))
	equal(t, form.String(), form.Render())
	equal(t, form.Field("username").String(), form.Field("username").AsWidget(nil, nil))
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_use_required_attribute_true
func TestUseRequiredAttributeTrue(t *testing.T) {
	form := useRequiredForm(true)
	assertHTMLEqual(t, form.AsP(),
		`<p><label for="id_f1">F1:</label>`+
			`<input id="id_f1" maxlength="30" name="f1" type="text" required></p>`+
			`<p><label for="id_f2">F2:</label>`+
			`<input id="id_f2" maxlength="30" name="f2" type="text"></p>`+
			`<p><label for="id_f3">F3:</label>`+
			`<textarea cols="40" id="id_f3" name="f3" rows="10" required>`+
			`</textarea></p>`+
			`<p><label for="id_f4">F4:</label> <select id="id_f4" name="f4">`+
			`<option value="P">Python</option>`+
			`<option value="J">Java</option>`+
			`</select></p>`)
	assertHTMLEqual(t, form.AsUL(),
		`<li><label for="id_f1">F1:</label> `+
			`<input id="id_f1" maxlength="30" name="f1" type="text" required></li>`+
			`<li><label for="id_f2">F2:</label>`+
			`<input id="id_f2" maxlength="30" name="f2" type="text"></li>`+
			`<li><label for="id_f3">F3:</label>`+
			`<textarea cols="40" id="id_f3" name="f3" rows="10" required>`+
			`</textarea></li>`+
			`<li><label for="id_f4">F4:</label> <select id="id_f4" name="f4">`+
			`<option value="P">Python</option>`+
			`<option value="J">Java</option>`+
			`</select></li>`)
	assertHTMLEqual(t, form.AsTable(),
		`<tr><th><label for="id_f1">F1:</label></th>`+
			`<td><input id="id_f1" maxlength="30" name="f1" type="text" required>`+
			`</td></tr>`+
			`<tr><th><label for="id_f2">F2:</label></th>`+
			`<td><input id="id_f2" maxlength="30" name="f2" type="text"></td></tr>`+
			`<tr><th><label for="id_f3">F3:</label></th>`+
			`<td><textarea cols="40" id="id_f3" name="f3" rows="10" required>`+
			`</textarea></td></tr>`+
			`<tr><th><label for="id_f4">F4:</label></th><td>`+
			`<select id="id_f4" name="f4">`+
			`<option value="P">Python</option>`+
			`<option value="J">Java</option>`+
			`</select></td></tr>`)
	assertHTMLEqual(t, form.AsDiv(),
		`<div><label for="id_f1">F1:</label><input id="id_f1" maxlength="30" `+
			`name="f1" type="text" required></div><div><label for="id_f2">F2:</label>`+
			`<input id="id_f2" maxlength="30" name="f2" type="text"></div><div><label `+
			`for="id_f3">F3:</label><textarea cols="40" id="id_f3" name="f3" `+
			`rows="10" required></textarea></div><div><label for="id_f4">F4:</label>`+
			`<select id="id_f4" name="f4"><option value="P">Python</option>`+
			`<option value="J">Java</option></select></div>`)
}

// useRequiredForm is the MyForm of the use_required_attribute tests.
func useRequiredForm(use bool) *forms.Form {
	f := forms.New(
		forms.F("f1", forms.NewCharField(forms.MaxLength(30))),
		forms.F("f2", forms.NewCharField(forms.MaxLength(30), forms.Required(false))),
		forms.F("f3", forms.NewCharField(forms.UseWidget(forms.Textarea{}))),
		forms.F("f4", forms.NewChoiceField(forms.Choices("P", "Python", "J", "Java"))),
	)
	f.UseRequiredAttribute = use
	return f
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_use_required_attribute_false
func TestUseRequiredAttributeFalse(t *testing.T) {
	form := useRequiredForm(false)
	assertHTMLEqual(t, form.AsP(),
		`<p><label for="id_f1">F1:</label>`+
			`<input id="id_f1" maxlength="30" name="f1" type="text"></p>`+
			`<p><label for="id_f2">F2:</label>`+
			`<input id="id_f2" maxlength="30" name="f2" type="text"></p>`+
			`<p><label for="id_f3">F3:</label>`+
			`<textarea cols="40" id="id_f3" name="f3" rows="10"></textarea></p>`+
			`<p><label for="id_f4">F4:</label> <select id="id_f4" name="f4">`+
			`<option value="P">Python</option>`+
			`<option value="J">Java</option>`+
			`</select></p>`)
	assertHTMLEqual(t, form.AsUL(),
		`<li><label for="id_f1">F1:</label>`+
			`<input id="id_f1" maxlength="30" name="f1" type="text"></li>`+
			`<li><label for="id_f2">F2:</label>`+
			`<input id="id_f2" maxlength="30" name="f2" type="text"></li>`+
			`<li><label for="id_f3">F3:</label>`+
			`<textarea cols="40" id="id_f3" name="f3" rows="10"></textarea></li>`+
			`<li><label for="id_f4">F4:</label> <select id="id_f4" name="f4">`+
			`<option value="P">Python</option>`+
			`<option value="J">Java</option>`+
			`</select></li>`)
	assertHTMLEqual(t, form.AsTable(),
		`<tr><th><label for="id_f1">F1:</label></th>`+
			`<td><input id="id_f1" maxlength="30" name="f1" type="text"></td></tr>`+
			`<tr><th><label for="id_f2">F2:</label></th>`+
			`<td><input id="id_f2" maxlength="30" name="f2" type="text"></td></tr>`+
			`<tr><th><label for="id_f3">F3:</label></th><td>`+
			`<textarea cols="40" id="id_f3" name="f3" rows="10">`+
			`</textarea></td></tr>`+
			`<tr><th><label for="id_f4">F4:</label></th><td>`+
			`<select id="id_f4" name="f4">`+
			`<option value="P">Python</option>`+
			`<option value="J">Java</option>`+
			`</select></td></tr>`)
	assertHTMLEqual(t, form.AsDiv(),
		`<div><label for="id_f1">F1:</label> <input id="id_f1" maxlength="30" `+
			`name="f1" type="text"></div><div><label for="id_f2">F2:</label>`+
			`<input id="id_f2" maxlength="30" name="f2" type="text"></div><div>`+
			`<label for="id_f3">F3:</label> <textarea cols="40" id="id_f3" name="f3" `+
			`rows="10"></textarea></div><div><label for="id_f4">F4:</label>`+
			`<select id="id_f4" name="f4"><option value="P">Python</option>`+
			`<option value="J">Java</option></select></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_field_named_data
func TestFieldNamedData(t *testing.T) {
	f := forms.New(forms.F("data", forms.NewCharField(forms.MaxLength(10)))).Bind(vals("data", "xyzzy"))
	if !f.IsValid() {
		t.Fatalf("form invalid: %v", f.Errors().Messages())
	}
	equal(t, f.CleanedData(), map[string]any{"data": "xyzzy"})
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_label_does_not_include_new_line
func TestLabelDoesNotIncludeNewLine(t *testing.T) {
	field := personForm().Field("first_name")
	equal(t, field.LabelTag(), `<label for="id_first_name">First name:</label>`)
	equal(t, field.LegendTag(), "<legend>First name:</legend>")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_label_attrs_not_localized
func TestLabelAttrsNotLocalized(t *testing.T) {
	field := personForm().Field("first_name")
	attrs := forms.Attrs{"number": 9999}
	assertHTMLEqual(t, field.LabelTagWith("", attrs, nil, "label"),
		`<label number="9999" for="id_first_name">First name:</label>`)
	assertHTMLEqual(t, field.LabelTagWith("", attrs, nil, "legend"),
		`<legend number="9999">First name:</legend>`)
}

// Django raises KeyError for the removed field; Field returns nil.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_remove_cached_field
func TestRemoveCachedField(t *testing.T) {
	f := forms.New(forms.F("name", forms.NewCharField(forms.MaxLength(10))))
	f.Fields()
	f.RemoveField("name")
	f = f.Bind(vals("name", "abcde"))
	if bf := f.Field("name"); bf != nil {
		t.Errorf("Field(%q) = %v, want nil", "name", bf)
	}
}

// Django returns None for a widget that sets aria-describedby itself; Go has
// no None for a string and returns "".
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_aria_describedby_property
func TestAriaDescribedbyProperty(t *testing.T) {
	newForm := func(opts ...forms.FieldOption) *forms.Form {
		return forms.New(forms.F("name", forms.NewCharField(opts...)))
	}
	help := forms.HelpText("Some help text")
	equal(t, newForm(help).Bind(vals("name", "MyName")).Field("name").AriaDescribedBy(), "id_name_helptext")

	noID := newForm(help)
	noID.AutoID = ""
	equal(t, noID.Field("name").AriaDescribedBy(), "")

	equal(t, newForm(help, forms.UseWidget(forms.HiddenInput{})).Field("name").AriaDescribedBy(), "")

	own := newForm(forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"aria-describedby": "my-id"}}))
	equal(t, own.Bind(vals("name", "MyName")).Field("name").AriaDescribedBy(), "")

	equal(t, newForm().Field("name").AriaDescribedBy(), "")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::TemplateTests.test_templates_with_forms
//
// The error lists are printed as {{ form.<field>.errors }}: a template sees
// an error list as a slice, which has no as_ul attribute, and printing it
// gives the same HTML as as_ul in Django.
func TestTemplatesWithForms(t *testing.T) {
	autoID := func(f *forms.Form, id string) *forms.Form { f.AutoID = id; return f }
	render := func(src string, f *forms.Form) string {
		return renderTemplate(t, src, map[string]any{"form": f})
	}
	errorsTmpl := "<form>" +
		"{{ form.username.errors }}" +
		"<p><label>Your username: {{ form.username }}</label></p>" +
		"{{ form.password1.errors }}" +
		"<p><label>Password: {{ form.password1 }}</label></p>" +
		"{{ form.password2.errors }}" +
		"<p><label>Password (again): {{ form.password2 }}</label></p>" +
		`<input type="submit" required>` +
		"</form>"
	assertHTMLEqual(t, render(errorsTmpl, autoID(userRegistrationForm(true), "")),
		"<form>"+
			"<p><label>Your username: "+
			`<input type="text" name="username" maxlength="10" required></label></p>`+
			"<p><label>Password: "+
			`<input type="password" name="password1" required></label></p>`+
			"<p><label>Password (again): "+
			`<input type="password" name="password2" required></label></p>`+
			`<input type="submit" required>`+
			"</form>")
	assertHTMLEqual(t, render(errorsTmpl, autoID(userRegistrationForm(true).Bind(vals("username", "django")), "")),
		"<form>"+
			"<p><label>Your username: "+
			`<input type="text" name="username" value="django" maxlength="10" required>`+
			"</label></p>"+
			`<ul class="errorlist"><li>This field is required.</li></ul><p>`+
			"<label>Password: "+
			`<input type="password" name="password1" aria-invalid="true" required>`+
			"</label></p>"+
			`<ul class="errorlist"><li>This field is required.</li></ul>`+
			"<p><label>Password (again): "+
			`<input type="password" name="password2" aria-invalid="true" required>`+
			"</label></p>"+
			`<input type="submit" required>`+
			"</form>")

	labelTmpl := "<form>" +
		"<p><label>{{ form.username.label }}: {{ form.username }}</label></p>" +
		"<p><label>{{ form.password1.label }}: {{ form.password1 }}</label></p>" +
		"<p><label>{{ form.password2.label }}: {{ form.password2 }}</label></p>" +
		`<input type="submit" required>` +
		"</form>"
	assertHTMLEqual(t, render(labelTmpl, autoID(userRegistrationForm(true), "")),
		"<form>"+
			"<p><label>Username: "+
			`<input type="text" name="username" maxlength="10" required></label></p>`+
			"<p><label>Password1: "+
			`<input type="password" name="password1" required></label></p>`+
			"<p><label>Password2: "+
			`<input type="password" name="password2" required></label></p>`+
			`<input type="submit" required>`+
			"</form>")

	labelTagTmpl := "<form>" +
		"<p>{{ form.username.label_tag }} {{ form.username }}" +
		`<span {% if form.username.id_for_label %}id="` +
		`{{ form.username.id_for_label }}_helptext"{% endif %}>` +
		"{{ form.username.help_text}}</span></p>" +
		"<p>{{ form.password1.label_tag }} {{ form.password1 }}</p>" +
		"<p>{{ form.password2.label_tag }} {{ form.password2 }}</p>" +
		`<input type="submit" required>` +
		"</form>"
	assertHTMLEqual(t, render(labelTagTmpl, autoID(userRegistrationForm(true), "")),
		"<form>"+
			"<p>Username: "+
			`<input type="text" name="username" maxlength="10" required>`+
			"<span>Good luck picking a username that doesn't already exist.</span></p>"+
			`<p>Password1: <input type="password" name="password1" required></p>`+
			`<p>Password2: <input type="password" name="password2" required></p>`+
			`<input type="submit" required>`+
			"</form>")
	assertHTMLEqual(t, render(labelTagTmpl, userRegistrationForm(true)),
		"<form>"+
			`<p><label for="id_username">Username:</label>`+
			`<input id="id_username" type="text" name="username" maxlength="10" `+
			`aria-describedby="id_username_helptext" required>`+
			`<span id="id_username_helptext">`+
			"Good luck picking a username that doesn't already exist.</span></p>"+
			`<p><label for="id_password1">Password1:</label>`+
			`<input type="password" name="password1" id="id_password1" required></p>`+
			`<p><label for="id_password2">Password2:</label>`+
			`<input type="password" name="password2" id="id_password2" required></p>`+
			`<input type="submit" required>`+
			"</form>")

	legendTmpl := "<form>" +
		"<p>{{ form.username.legend_tag }} {{ form.username }}</p>" +
		"<p>{{ form.password1.legend_tag }} {{ form.password1 }}</p>" +
		"<p>{{ form.password2.legend_tag }} {{ form.password2 }}</p>" +
		`<input type="submit" required>` +
		"</form>"
	assertHTMLEqual(t, render(legendTmpl, autoID(userRegistrationForm(true), "")),
		"<form>"+
			"<p>Username: "+
			`<input type="text" name="username" maxlength="10" required></p>`+
			`<p>Password1: <input type="password" name="password1" required></p>`+
			`<p>Password2: <input type="password" name="password2" required></p>`+
			`<input type="submit" required>`+
			"</form>")
	assertHTMLEqual(t, render(legendTmpl, userRegistrationForm(true)),
		"<form>"+
			"<p><legend>Username:</legend>"+
			`<input id="id_username" type="text" name="username" maxlength="10" `+
			`aria-describedby="id_username_helptext" required></p>`+
			"<p><legend>Password1:</legend>"+
			`<input type="password" name="password1" id="id_password1" required></p>`+
			"<p><legend>Password2:</legend>"+
			`<input type="password" name="password2" id="id_password2" required></p>`+
			`<input type="submit" required>`+
			"</form>")

	helpTmpl := "<form>" +
		"<p>{{ form.username.label_tag }} {{ form.username }}<br>" +
		"{{ form.username.help_text }}</p>" +
		"<p>{{ form.password1.label_tag }} {{ form.password1 }}</p>" +
		"<p>{{ form.password2.label_tag }} {{ form.password2 }}</p>" +
		`<input type="submit" required>` +
		"</form>"
	assertHTMLEqual(t, render(helpTmpl, autoID(userRegistrationForm(true), "")),
		"<form>"+
			"<p>Username: "+
			`<input type="text" name="username" maxlength="10" required><br>`+
			"Good luck picking a username that doesn&#x27;t already exist.</p>"+
			`<p>Password1: <input type="password" name="password1" required></p>`+
			`<p>Password2: <input type="password" name="password2" required></p>`+
			`<input type="submit" required>`+
			"</form>")
	equal(t, render("{{ form.password1.help_text }}", autoID(userRegistrationForm(true), "")), "")

	bound := autoID(userRegistrationForm(true).Bind(vals("username", "django", "password1", "foo", "password2", "bar")), "")
	assertHTMLEqual(t, render(errorsTmpl, bound),
		"<form>"+
			"<p><label>Your username: "+
			`<input type="text" name="username" value="django" maxlength="10" required>`+
			"</label></p>"+
			"<p><label>Password: "+
			`<input type="password" name="password1" required></label></p>`+
			"<p><label>Password (again): "+
			`<input type="password" name="password2" required></label></p>`+
			`<input type="submit" required>`+
			"</form>")
	nonFieldTmpl := strings.Replace(errorsTmpl, "<form>", "<form>{{ form.non_field_errors }}", 1)
	assertHTMLEqual(t, render(nonFieldTmpl, bound),
		"<form>"+
			`<ul class="errorlist nonfield">`+
			"<li>Please make sure your passwords match.</li></ul>"+
			"<p><label>Your username: "+
			`<input type="text" name="username" value="django" maxlength="10" required>`+
			"</label></p>"+
			"<p><label>Password: "+
			`<input type="password" name="password1" required></label></p>`+
			"<p><label>Password (again): "+
			`<input type="password" name="password2" required></label></p>`+
			`<input type="submit" required>`+
			"</form>")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::TemplateTests.test_basic_processing_in_view
func TestBasicProcessingInView(t *testing.T) {
	myFunction := func(method string, post url.Values) string {
		form := userRegistrationForm(false)
		form.AutoID = ""
		if method == "POST" {
			form = form.Bind(post)
		}
		if form.IsValid() {
			cleaned := form.CleanedData()
			keys := make([]string, 0, len(cleaned))
			for k := range cleaned {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			items := make([]string, len(keys))
			for i, k := range keys {
				items[i] = fmt.Sprintf("('%s', '%s')", k, cleaned[k])
			}
			return "VALID: [" + strings.Join(items, ", ") + "]"
		}
		return renderTemplate(t, `<form method="post">{{ form }}<input type="submit" required></form>`,
			map[string]any{"form": form})
	}
	assertHTMLEqual(t, myFunction("GET", nil),
		`<form method="post">`+
			"<div>Username:"+
			`<input type="text" name="username" maxlength="10" required></div>`+
			"<div>Password1:"+
			`<input type="password" name="password1" required></div>`+
			"<div>Password2:"+
			`<input type="password" name="password2" required></div>`+
			`<input type="submit" required>`+
			"</form>")
	assertHTMLEqual(t, myFunction("POST", vals(
		"username", "this-is-a-long-username", "password1", "foo", "password2", "bar")),
		`<form method="post">`+
			`<ul class="errorlist nonfield">`+
			"<li>Please make sure your passwords match.</li></ul>"+
			`<div>Username:<ul class="errorlist">`+
			"<li>Ensure this value has at most 10 characters (it has 23).</li></ul>"+
			`<input type="text" name="username" aria-invalid="true" `+
			`value="this-is-a-long-username" maxlength="10" required></div>`+
			"<div>Password1:"+
			`<input type="password" name="password1" required></div>`+
			"<div>Password2:"+
			`<input type="password" name="password2" required></div>`+
			`<input type="submit" required>`+
			"</form>")
	equal(t, myFunction("POST", vals("username", "adrian", "password1", "secret", "password2", "secret")),
		"VALID: [('password1', 'secret'), ('password2', 'secret'), ('username', 'adrian')]")
}
