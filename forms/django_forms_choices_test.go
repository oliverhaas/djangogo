package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_various_boolean_values
//
// Submitted data is always text here, so Python's True is sent as "True".
func TestVariousBooleanValues(t *testing.T) {
	signup := func(data ...string) *forms.Form {
		f := forms.New(
			forms.F("email", forms.NewEmailField()),
			forms.F("get_spam", forms.NewBooleanField()),
		)
		f.AutoID = ""
		if data == nil {
			return f
		}
		return f.Bind(vals(data...))
	}

	f := signup()
	djangotest.AssertHTMLEqual(t, f.Field("email").String(),
		`<input type="email" name="email" maxlength="320" required>`)
	djangotest.AssertHTMLEqual(t, f.Field("get_spam").String(),
		`<input type="checkbox" name="get_spam" required>`)

	f = signup("email", "test@example.com", "get_spam", "True")
	djangotest.AssertHTMLEqual(t, f.Field("email").String(),
		`<input type="email" name="email" maxlength="320" value="test@example.com" required>`)
	djangotest.AssertHTMLEqual(t, f.Field("get_spam").String(),
		`<input checked type="checkbox" name="get_spam" required>`)

	// "True" and "true" render without a value attribute.
	f = signup("email", "test@example.com", "get_spam", "true")
	djangotest.AssertHTMLEqual(t, f.Field("get_spam").String(),
		`<input checked type="checkbox" name="get_spam" required>`)

	// "False" and "false" render unchecked.
	for _, value := range []string{"False", "false"} {
		f = signup("email", "test@example.com", "get_spam", value)
		djangotest.AssertHTMLEqual(t, f.Field("get_spam").String(),
			`<input type="checkbox" name="get_spam" aria-invalid="true" required>`)
	}

	// "0" is a true value (#16820).
	f = signup("email", "test@example.com", "get_spam", "0")
	equal(t, f.IsValid(), true)
	equal(t, f.CleanedData()["get_spam"], any(true))
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_widget_output
func TestWidgetOutput(t *testing.T) {
	contact := func(message forms.Widget) *forms.Form {
		f := forms.New(
			forms.F("subject", forms.NewCharField()),
			forms.F("message", forms.NewCharField(forms.UseWidget(message))),
		)
		f.AutoID = ""
		return f
	}

	f := contact(forms.Textarea{})
	djangotest.AssertHTMLEqual(t, f.Field("subject").String(), `<input type="text" name="subject" required>`)
	djangotest.AssertHTMLEqual(t, f.Field("message").String(),
		`<textarea name="message" rows="10" cols="40" required></textarea>`)

	// AsTextarea, AsText and AsHidden change the widget type.
	djangotest.AssertHTMLEqual(t, f.Field("subject").AsTextarea(nil),
		`<textarea name="subject" rows="10" cols="40" required></textarea>`)
	djangotest.AssertHTMLEqual(t, f.Field("message").AsText(nil), `<input type="text" name="message" required>`)
	djangotest.AssertHTMLEqual(t, f.Field("message").AsHidden(nil), `<input type="hidden" name="message">`)

	f = contact(forms.Textarea{Attrs: forms.Attrs{"rows": 80, "cols": 20}})
	djangotest.AssertHTMLEqual(t, f.Field("message").String(),
		`<textarea name="message" rows="80" cols="20" required></textarea>`)

	// The attributes of the widget are not carried over to another widget.
	djangotest.AssertHTMLEqual(t, f.Field("message").AsText(nil), `<input type="text" name="message" required>`)

	f = f.Bind(vals("subject", "Hello", "message", "I love you."))
	djangotest.AssertHTMLEqual(t, f.Field("subject").AsTextarea(nil),
		`<textarea rows="10" cols="40" name="subject" required>Hello</textarea>`)
	djangotest.AssertHTMLEqual(t, f.Field("message").AsText(nil),
		`<input type="text" name="message" value="I love you." required>`)
	djangotest.AssertHTMLEqual(t, f.Field("message").AsHidden(nil),
		`<input type="hidden" name="message" value="I love you.">`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_forms_with_choices
func TestFormsWithChoices(t *testing.T) {
	framework := func(language forms.Field) *forms.Form {
		f := forms.New(forms.F("name", forms.NewCharField()), forms.F("language", language))
		f.AutoID = ""
		return f
	}
	bound := vals("name", "Django", "language", "P")

	f := framework(forms.NewChoiceField(forms.Choices("P", "Python", "J", "Java")))
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), `<select name="language">
<option value="P">Python</option>
<option value="J">Java</option>
</select>`)
	djangotest.AssertHTMLEqual(t, f.Bind(bound).Field("language").String(), `<select name="language">
<option value="P" selected>Python</option>
<option value="J">Java</option>
</select>`)

	// The option of the empty string is selected in an unbound form.
	f = framework(forms.NewChoiceField(forms.Choices("", "------", "P", "Python", "J", "Java")))
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), `<select name="language" required>
<option value="" selected>------</option>
<option value="P">Python</option>
<option value="J">Java</option>
</select>`)

	// Widget attributes are given with the widget.
	f = framework(forms.NewChoiceField(forms.Choices("P", "Python", "J", "Java"),
		forms.UseWidget(forms.Select{Attrs: forms.Attrs{"class": "foo"}})))
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), `<select class="foo" name="language">
<option value="P">Python</option>
<option value="J">Java</option>
</select>`)
	djangotest.AssertHTMLEqual(t, f.Bind(bound).Field("language").String(), `<select class="foo" name="language">
<option value="P" selected>Python</option>
<option value="J">Java</option>
</select>`)

	// The widget uses the choices of the field, not its own.
	f = framework(forms.NewChoiceField(forms.Choices("P", "Python", "J", "Java"),
		forms.UseWidget(forms.Select{
			Attrs:   forms.Attrs{"class": "foo"},
			Choices: []forms.Choice{{Value: "R", Label: "Ruby"}, {Value: "P", Label: "Perl"}},
		})))
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), `<select class="foo" name="language">
<option value="P">Python</option>
<option value="J">Java</option>
</select>`)
	djangotest.AssertHTMLEqual(t, f.Bind(bound).Field("language").String(), `<select class="foo" name="language">
<option value="P" selected>Python</option>
<option value="J">Java</option>
</select>`)

	// The choices of a ChoiceField can be set after the fact.
	f = framework(forms.NewChoiceField())
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), `<select name="language">
</select>`)
	f.SetChoices("language", [][2]string{{"P", "Python"}, {"J", "Java"}})
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), `<select name="language">
<option value="P">Python</option>
<option value="J">Java</option>
</select>`)
}

// frameworkForm is the FrameworkForm of Django's forms tests.
func frameworkForm() *forms.Form {
	return forms.New(
		forms.F("name", forms.NewCharField()),
		forms.F("language", forms.NewChoiceField(
			forms.Choices("P", "Python", "J", "Java"), forms.UseWidget(forms.RadioSelect{}))),
	)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_forms_with_radio
func TestFormsWithRadio(t *testing.T) {
	f := frameworkForm()
	f.AutoID = ""
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), `<div>
<div><label><input type="radio" name="language" value="P" required> Python</label></div>
<div><label><input type="radio" name="language" value="J" required> Java</label></div>
</div>`)
	djangotest.AssertHTMLEqual(t, f.AsTable(), `<tr><th>Name:</th><td><input type="text" name="name" required></td></tr>
<tr><th>Language:</th><td><div>
<div><label><input type="radio" name="language" value="P" required> Python</label></div>
<div><label><input type="radio" name="language" value="J" required> Java</label></div>
</div></td></tr>`)
	djangotest.AssertHTMLEqual(t, f.AsUL(), `<li>Name: <input type="text" name="name" required></li>
<li>Language: <div>
<div><label><input type="radio" name="language" value="P" required> Python</label></div>
<div><label><input type="radio" name="language" value="J" required> Java</label></div>
</div></li>`)
	// Without ids there is no legend.
	djangotest.AssertHTMLEqual(t, f.AsDiv(),
		`<div> Name: <input type="text" name="name" required></div><div><fieldset>`+
			`Language:<div><div><label><input type="radio" name="language" value="P" `+
			`required> Python</label></div><div><label><input type="radio" `+
			`name="language" value="J" required> Java</label></div></div></fieldset>`+
			`</div>`)

	// Each radio button gets an id of its own: the field's id, an underscore
	// and the zero-based index.
	f = frameworkForm()
	radios := `<div id="id_language">
		<div><label for="id_language_0">
		<input type="radio" id="id_language_0" value="P" name="language" required>
		Python</label></div>
		<div><label for="id_language_1">
		<input type="radio" id="id_language_1" value="J" name="language" required>
		Java</label></div>
		</div>`
	djangotest.AssertHTMLEqual(t, f.Field("language").String(), radios)

	// The label of the group does not point at the first radio button.
	djangotest.AssertHTMLEqual(t, f.AsTable(), `<tr><th><label for="id_name">Name:</label></th><td>
		<input type="text" name="name" id="id_name" required></td></tr>
		<tr><th><label>Language:</label></th><td>`+radios+`</td></tr>`)
	djangotest.AssertHTMLEqual(t, f.AsUL(), `<li><label for="id_name">Name:</label>
		<input type="text" name="name" id="id_name" required></li>
		<li><label>Language:</label> `+radios+`</li>`)
	djangotest.AssertHTMLEqual(t, f.AsP(), `<p><label for="id_name">Name:</label>
		<input type="text" name="name" id="id_name" required></p>
		<p><label>Language:</label> `+radios+`</p>`)
	djangotest.AssertHTMLEqual(t, f.AsDiv(),
		`<div><label for="id_name">Name:</label><input type="text" name="name" `+
			`required id="id_name"></div><div><fieldset><legend>Language:</legend>`+
			`<div id="id_language"><div><label for="id_language_0"><input `+
			`type="radio" name="language" value="P" required id="id_language_0">`+
			`Python</label></div><div><label for="id_language_1"><input type="radio" `+
			`name="language" value="J" required id="id_language_1">Java</label></div>`+
			`</div></fieldset></div>`)
}
