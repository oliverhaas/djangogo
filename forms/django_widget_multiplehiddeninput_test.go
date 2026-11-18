package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_single
func TestMultipleHiddenInputRenderSingle(t *testing.T) {
	widgetCheckHTML(t, forms.MultipleHiddenInput{}, "email", []string{"test@example.com"}, nil,
		`<input type="hidden" name="email" value="test@example.com">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_multiple
func TestMultipleHiddenInputRenderMultiple(t *testing.T) {
	widgetCheckHTML(t, forms.MultipleHiddenInput{}, "email", []string{"test@example.com", "foo@example.com"}, nil,
		`<input type="hidden" name="email" value="test@example.com">`+"\n"+
			`<input type="hidden" name="email" value="foo@example.com">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_attrs
func TestMultipleHiddenInputRenderAttrs(t *testing.T) {
	widgetCheckHTML(t, forms.MultipleHiddenInput{}, "email", []string{"test@example.com"}, forms.Attrs{"class": "fun"},
		`<input type="hidden" name="email" value="test@example.com" class="fun">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_attrs_multiple
func TestMultipleHiddenInputRenderAttrsMultiple(t *testing.T) {
	widgetCheckHTML(t, forms.MultipleHiddenInput{}, "email", []string{"test@example.com", "foo@example.com"}, forms.Attrs{"class": "fun"},
		`<input type="hidden" name="email" value="test@example.com" class="fun">`+"\n"+
			`<input type="hidden" name="email" value="foo@example.com" class="fun">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_attrs_constructor
func TestMultipleHiddenInputRenderAttrsConstructor(t *testing.T) {
	w := forms.MultipleHiddenInput{Attrs: forms.Attrs{"class": "fun"}}
	widgetCheckHTML(t, w, "email", []string{}, nil, "")
	widgetCheckHTML(t, w, "email", []string{"foo@example.com"}, nil,
		`<input type="hidden" class="fun" value="foo@example.com" name="email">`)
	widgetCheckHTML(t, w, "email", []string{"foo@example.com", "test@example.com"}, nil,
		`<input type="hidden" class="fun" value="foo@example.com" name="email">`+"\n"+
			`<input type="hidden" class="fun" value="test@example.com" name="email">`)
	widgetCheckHTML(t, w, "email", []string{"foo@example.com"}, forms.Attrs{"class": "special"},
		`<input type="hidden" class="special" value="foo@example.com" name="email">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_empty
func TestMultipleHiddenInputRenderEmpty(t *testing.T) {
	widgetCheckHTML(t, forms.MultipleHiddenInput{}, "email", []string{}, nil, "")
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_none
func TestMultipleHiddenInputRenderNone(t *testing.T) {
	widgetCheckHTML(t, forms.MultipleHiddenInput{}, "email", nil, nil, "")
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_render_increment_id
func TestMultipleHiddenInputRenderIncrementID(t *testing.T) {
	// Each input gets a separate ID.
	widgetCheckHTML(t, forms.MultipleHiddenInput{}, "letters", []string{"a", "b", "c"}, forms.Attrs{"id": "hideme"},
		`<input type="hidden" name="letters" value="a" id="hideme_0">`+"\n"+
			`<input type="hidden" name="letters" value="b" id="hideme_1">`+"\n"+
			`<input type="hidden" name="letters" value="c" id="hideme_2">`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_multiplehiddeninput.py::MultipleHiddenInputTest.test_fieldset
func TestMultipleHiddenInputFieldset(t *testing.T) {
	w := forms.MultipleHiddenInput{}
	equal(t, w.UseFieldset(), false)
	form := forms.New(forms.F("composers", forms.NewMultipleChoiceField(
		forms.Choices("J", "John Lennon", "P", "Paul McCartney"), forms.UseWidget(w)))).Bind(vals("composers", "J", "composers", "P"))
	assertHTMLEqual(t, form.Field("composers").String(),
		`<input type="hidden" name="composers" value="J" id="id_composers_0">`+
			`<input type="hidden" name="composers" value="P" id="id_composers_1">`)
}
