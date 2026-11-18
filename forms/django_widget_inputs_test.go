package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// widgetBeatles is WidgetTest.beatles of Django's widget tests.
var widgetBeatles = []forms.Choice{{Value: "J", Label: "John"}, {Value: "P", Label: "Paul"}, {Value: "G", Label: "George"}, {Value: "R", Label: "Ringo"}}

// widgetCheckHTML renders the widget and compares the result with want as HTML,
// Django's WidgetTest.check_html.
func widgetCheckHTML(t *testing.T, w forms.Widget, name string, value any, attrs forms.Attrs, want string) {
	t.Helper()
	assertHTMLEqual(t, w.Render(name, value, attrs), want)
}

// widgetFieldHTML renders a single-field form's field "field" with the widget
// and a required CharField, the body of Django's test_fieldset tests.
func widgetFieldHTML(field forms.Field) string {
	return forms.New(forms.F("field", field)).Field("field").String()
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_render
func TestTextInputRender(t *testing.T) {
	widgetCheckHTML(t, forms.TextInput{}, "email", "", nil, `<input type="text" name="email">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_render_none
func TestTextInputRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.TextInput{}, "email", nil, nil, `<input type="text" name="email">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_render_value
func TestTextInputRenderValue(t *testing.T) {
	widgetCheckHTML(t, forms.TextInput{}, "email", "test@example.com", nil,
		`<input type="text" name="email" value="test@example.com">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_render_boolean
func TestTextInputRenderBoolean(t *testing.T) {
	// Boolean values are rendered to their string forms ("True" and "False").
	widgetCheckHTML(t, forms.TextInput{}, "get_spam", false, nil, `<input type="text" name="get_spam" value="False">`)
	widgetCheckHTML(t, forms.TextInput{}, "get_spam", true, nil, `<input type="text" name="get_spam" value="True">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_render_quoted
func TestTextInputRenderQuoted(t *testing.T) {
	widgetCheckHTML(t, forms.TextInput{}, "email", `some "quoted" & ampersanded value`, nil,
		`<input type="text" name="email" value="some &quot;quoted&quot; &amp; ampersanded value">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_render_custom_attrs
func TestTextInputRenderCustomAttrs(t *testing.T) {
	widgetCheckHTML(t, forms.TextInput{}, "email", "test@example.com", forms.Attrs{"class": "fun"},
		`<input type="text" name="email" value="test@example.com" class="fun">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_render_unicode
func TestTextInputRenderUnicode(t *testing.T) {
	widgetCheckHTML(t, forms.TextInput{}, "email", "ŠĐĆŽćžšđ", forms.Attrs{"class": "fun"},
		`<input type="text" name="email" value="ŠĐĆŽćžšđ" class="fun">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_constructor_attrs
func TestTextInputConstructorAttrs(t *testing.T) {
	w := forms.TextInput{Attrs: forms.Attrs{"class": "fun", "type": "email"}}
	widgetCheckHTML(t, w, "email", "", nil, `<input type="email" class="fun" name="email">`)
	widgetCheckHTML(t, w, "email", "foo@example.com", nil,
		`<input type="email" class="fun" value="foo@example.com" name="email">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_attrs_precedence
func TestTextInputAttrsPrecedence(t *testing.T) {
	// attrs passed to Render get precedence over those of the widget.
	w := forms.TextInput{Attrs: forms.Attrs{"class": "pretty"}}
	widgetCheckHTML(t, w, "email", "", forms.Attrs{"class": "special"}, `<input type="text" class="special" name="email">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_use_required_attribute
func TestTextInputUseRequiredAttribute(t *testing.T) {
	w := forms.TextInput{}
	equal(t, w.UseRequiredAttribute(nil), true)
	equal(t, w.UseRequiredAttribute(""), true)
	equal(t, w.UseRequiredAttribute("resume.txt"), true)
}

// The Django template wrapping the field (label and div) is not ported; the
// widget HTML is what the form renders.
// Ported from Django: tests/forms_tests/widget_tests/test_textinput.py::TextInputTest.test_fieldset
func TestTextInputFieldset(t *testing.T) {
	w := forms.TextInput{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<input type="text" name="field" required id="id_field">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_numberinput.py::NumberInputTests.test_attrs_not_localized
func TestNumberInputAttrsNotLocalized(t *testing.T) {
	w := forms.NumberInput{Attrs: forms.Attrs{"max": 12345, "min": 1234, "step": 9999}}
	widgetCheckHTML(t, w, "name", "value", nil,
		`<input type="number" name="name" value="value" max="12345" min="1234" step="9999">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_numberinput.py::NumberInputTests.test_fieldset
func TestNumberInputFieldset(t *testing.T) {
	w := forms.NumberInput{Attrs: forms.Attrs{"max": 12345, "min": 1234, "step": 9999}}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<input id="id_field" max="12345" min="1234" name="field" required step="9999" type="number">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_passwordinput.py::PasswordInputTest.test_render
func TestPasswordInputRender(t *testing.T) {
	widgetCheckHTML(t, forms.PasswordInput{}, "password", "", nil, `<input type="password" name="password">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_passwordinput.py::PasswordInputTest.test_render_ignore_value
func TestPasswordInputRenderIgnoreValue(t *testing.T) {
	widgetCheckHTML(t, forms.PasswordInput{}, "password", "secret", nil, `<input type="password" name="password">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_passwordinput.py::PasswordInputTest.test_render_value_true
func TestPasswordInputRenderValueTrue(t *testing.T) {
	w := forms.PasswordInput{RenderValue: true}
	widgetCheckHTML(t, w, "password", "", nil, `<input type="password" name="password">`)
	widgetCheckHTML(t, w, "password", nil, nil, `<input type="password" name="password">`)
	widgetCheckHTML(t, w, "password", "test@example.com", nil,
		`<input type="password" name="password" value="test@example.com">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_passwordinput.py::PasswordInputTest.test_fieldset
func TestPasswordInputFieldset(t *testing.T) {
	w := forms.PasswordInput{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<input type="password" name="field" required id="id_field">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_hiddeninput.py::HiddenInputTest.test_render
func TestHiddenInputRender(t *testing.T) {
	widgetCheckHTML(t, forms.HiddenInput{}, "email", "", nil, `<input type="hidden" name="email">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_hiddeninput.py::HiddenInputTest.test_use_required_attribute
func TestHiddenInputUseRequiredAttribute(t *testing.T) {
	// Always false to avoid browser validation on inputs hidden from the user.
	w := forms.HiddenInput{}
	equal(t, w.UseRequiredAttribute(nil), false)
	equal(t, w.UseRequiredAttribute(""), false)
	equal(t, w.UseRequiredAttribute("foo"), false)
}

// Ported from Django: tests/forms_tests/widget_tests/test_hiddeninput.py::HiddenInputTest.test_fieldset
func TestHiddenInputFieldset(t *testing.T) {
	w := forms.HiddenInput{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<input type="hidden" name="field" id="id_field">`)
}
