package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/widget_tests/test_textarea.py::TextareaTest.test_render
func TestTextareaRender(t *testing.T) {
	widgetCheckHTML(t, forms.Textarea{}, "msg", "value", nil, `<textarea rows="10" cols="40" name="msg">value</textarea>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textarea.py::TextareaTest.test_render_required
func TestTextareaRenderRequired(t *testing.T) {
	// Django sets widget.is_required; the Go widget has no such state, so the
	// output is the one of an unrequired widget.
	widgetCheckHTML(t, forms.Textarea{}, "msg", "value", nil, `<textarea rows="10" cols="40" name="msg">value</textarea>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textarea.py::TextareaTest.test_render_empty
func TestTextareaRenderEmpty(t *testing.T) {
	widgetCheckHTML(t, forms.Textarea{}, "msg", "", nil, `<textarea rows="10" cols="40" name="msg"></textarea>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textarea.py::TextareaTest.test_render_none
func TestTextareaRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.Textarea{}, "msg", nil, nil, `<textarea rows="10" cols="40" name="msg"></textarea>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textarea.py::TextareaTest.test_escaping
func TestTextareaEscaping(t *testing.T) {
	widgetCheckHTML(t, forms.Textarea{}, "msg", `some "quoted" & ampersanded value`, nil,
		`<textarea rows="10" cols="40" name="msg">some &quot;quoted&quot; &amp; ampersanded value</textarea>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_textarea.py::TextareaTest.test_fieldset
func TestTextareaFieldset(t *testing.T) {
	w := forms.Textarea{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewCharField(forms.UseWidget(w))),
		`<textarea cols="40" id="id_field" name="field" required rows="10"></textarea>`)
}
