package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// widgetNumChoices is the choices (1, 1), (2, 2), (3, 3) of Django's select
// tests, with the values as text.
var widgetNumChoices = []forms.Choice{{Value: "1", Label: "1"}, {Value: "2", Label: "2"}, {Value: "3", Label: "3"}}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_render
func TestSelectRender(t *testing.T) {
	widgetCheckHTML(t, forms.Select{Choices: widgetBeatles}, "beatle", "J", nil, `
        <select name="beatle">
          <option value="J" selected>John</option>
          <option value="P">Paul</option>
          <option value="G">George</option>
          <option value="R">Ringo</option>
        </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_render_none
func TestSelectRenderNone(t *testing.T) {
	// If the value is None, none of the options are selected.
	widgetCheckHTML(t, forms.Select{Choices: widgetBeatles}, "beatle", nil, nil, `<select name="beatle">
            <option value="J">John</option>
            <option value="P">Paul</option>
            <option value="G">George</option>
            <option value="R">Ringo</option>
            </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_render_label_value
func TestSelectRenderLabelValue(t *testing.T) {
	// A value that matches a label but no option value selects nothing.
	widgetCheckHTML(t, forms.Select{Choices: widgetBeatles}, "beatle", "John", nil, `<select name="beatle">
            <option value="J">John</option>
            <option value="P">Paul</option>
            <option value="G">George</option>
            <option value="R">Ringo</option>
            </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_render_selected
func TestSelectRenderSelected(t *testing.T) {
	// Only one option can be selected (#8103).
	choices := []forms.Choice{{Value: "0", Label: "0"}, {Value: "1", Label: "1"}, {Value: "2", Label: "2"}, {Value: "3", Label: "3"}, {Value: "0", Label: "extra"}}
	widgetCheckHTML(t, forms.Select{Choices: choices}, "choices", "0", nil, `<select name="choices">
            <option value="0" selected>0</option>
            <option value="1">1</option>
            <option value="2">2</option>
            <option value="3">3</option>
            <option value="0">extra</option>
            </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_constructor_attrs
func TestSelectConstructorAttrs(t *testing.T) {
	// Select options do not inherit the parent widget attrs.
	w := forms.Select{Attrs: forms.Attrs{"class": "super", "id": "super"}, Choices: widgetNumChoices}
	widgetCheckHTML(t, w, "num", 2, nil, `<select name="num" class="super" id="super">
              <option value="1">1</option>
              <option value="2" selected>2</option>
              <option value="3">3</option>
            </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_compare_to_str
func TestSelectCompareToStr(t *testing.T) {
	// The value is compared to its str().
	html := `<select name="num">
                <option value="1">1</option>
                <option value="2" selected>2</option>
                <option value="3">3</option>
                </select>`
	w := forms.Select{Choices: widgetNumChoices}
	widgetCheckHTML(t, w, "num", 2, nil, html)
	widgetCheckHTML(t, w, "num", "2", nil, html)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_choices_constructor
func TestSelectChoicesConstructor(t *testing.T) {
	widgetCheckHTML(t, forms.Select{Choices: widgetNumChoices}, "num", 2, nil, `<select name="num">
            <option value="1">1</option>
            <option value="2" selected>2</option>
            <option value="3">3</option>
            </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_choices_unicode
func TestSelectChoicesUnicode(t *testing.T) {
	w := forms.Select{Choices: []forms.Choice{{Value: "ŠĐĆŽćžšđ", Label: "ŠĐabcĆŽćžšđ"}, {Value: "ćžšđ", Label: "abcćžšđ"}}}
	widgetCheckHTML(t, w, "email", "ŠĐĆŽćžšđ", nil, `
                <select name="email">
                <option value="ŠĐĆŽćžšđ" selected>ŠĐabcĆŽćžšđ</option>
                <option value="ćžšđ">abcćžšđ</option>
                </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_doesnt_render_required_when_impossible_to_select_empty_field
func TestSelectDoesntRenderRequiredWhenImpossibleToSelectEmptyField(t *testing.T) {
	w := forms.Select{Choices: []forms.Choice{{Value: "J", Label: "John"}, {Value: "P", Label: "Paul"}}}
	equal(t, w.UseRequiredAttribute(nil), false)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_doesnt_render_required_when_no_choices_are_available
func TestSelectDoesntRenderRequiredWhenNoChoicesAreAvailable(t *testing.T) {
	equal(t, forms.Select{Choices: []forms.Choice{}}.UseRequiredAttribute(nil), false)
}

// The Django template wrapping the field (label and div) is not ported; the
// widget HTML is what the form renders.
// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_fieldset
func TestSelectFieldset(t *testing.T) {
	w := forms.Select{}
	equal(t, w.UseFieldset(), false)
	assertHTMLEqual(t, widgetFieldHTML(forms.NewChoiceField(forms.UseWidget(w), forms.Choices("J", "John", "P", "Paul", "G", "George", "R", "Ringo"))),
		`<select name="field" id="id_field">
		<option value="J">John</option>  <option value="P">Paul</option>
		<option value="G">George</option><option value="R">Ringo</option></select>`)
}

// widgetNestedChoices is nested_widgets of Django's choice widget tests: an
// option and a group of two options. Django builds the widgets from a tuple,
// a dict and a dict of tuples, which are all one list of Choice in Go.
var widgetNestedChoices = []forms.Choice{
	{Value: "outer1", Label: "Outer 1"},
	{Label: `Group "1"`, Group: []forms.Choice{{Value: "inner1", Label: "Inner 1"}, {Value: "inner2", Label: "Inner 2"}}},
}

// widgetMediaChoices is nested_choices of Django's radio and checkbox tests.
var widgetMediaChoices = []forms.Choice{
	{Value: "unknown", Label: "Unknown"},
	{Label: "Audio", Group: []forms.Choice{{Value: "vinyl", Label: "Vinyl"}, {Value: "cd", Label: "CD"}}},
	{Label: "Video", Group: []forms.Choice{{Value: "vhs", Label: "VHS"}, {Value: "dvd", Label: "DVD"}}},
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_choices_optgroup
func TestSelectChoicesOptgroup(t *testing.T) {
	// Choices nest one level to create HTML option groups.
	widgetCheckHTML(t, forms.Select{Choices: widgetNestedChoices}, "nestchoice", nil, nil, `
        <select name="nestchoice">
          <option value="outer1">Outer 1</option>
          <optgroup label="Group &quot;1&quot;">
          <option value="inner1">Inner 1</option>
          <option value="inner2">Inner 2</option>
          </optgroup>
        </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_choices_select_outer
func TestSelectChoicesSelectOuter(t *testing.T) {
	widgetCheckHTML(t, forms.Select{Choices: widgetNestedChoices}, "nestchoice", "outer1", nil, `
        <select name="nestchoice">
          <option value="outer1" selected>Outer 1</option>
          <optgroup label="Group &quot;1&quot;">
          <option value="inner1">Inner 1</option>
          <option value="inner2">Inner 2</option>
          </optgroup>
        </select>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_choices_select_inner
func TestSelectChoicesSelectInner(t *testing.T) {
	widgetCheckHTML(t, forms.Select{Choices: widgetNestedChoices}, "nestchoice", "inner1", nil, `
        <select name="nestchoice">
          <option value="outer1">Outer 1</option>
          <optgroup label="Group &quot;1&quot;">
          <option value="inner1" selected>Inner 1</option>
          <option value="inner2">Inner 2</option>
          </optgroup>
        </select>`)
}

// Go has no optgroups(): Subwidgets returns the options of a bound field
// without their groups, as Django's options() does. The test checks the
// options that optgroups() puts in the groups.
//
// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_optgroups
func TestSelectOptgroups(t *testing.T) {
	choices := []forms.Choice{
		{Label: "Audio", Group: []forms.Choice{{Value: "vinyl", Label: "Vinyl"}, {Value: "cd", Label: "CD"}}},
		{Label: "Video", Group: []forms.Choice{{Value: "vhs", Label: "VHS Tape"}, {Value: "dvd", Label: "DVD"}}},
		{Value: "unknown", Label: "Unknown"},
	}
	form := withoutAutoID(forms.New(forms.F("name", forms.NewChoiceField(forms.ChoiceList(choices),
		forms.UseWidget(forms.Select{Attrs: forms.Attrs{"class": "super"}})))))
	form.Initial = map[string]any{"name": "vhs"}
	options := form.Field("name").Subwidgets()
	want := []struct {
		value, label string
		selected     bool
	}{
		{"vinyl", "Vinyl", false},
		{"cd", "CD", false},
		{"vhs", "VHS Tape", true},
		{"dvd", "DVD", false},
		{"unknown", "Unknown", false},
	}
	equal(t, len(options), len(want))
	for i, w := range want {
		o := options[i]
		equal(t, o.Name, "name")
		equal(t, o.Value, any(w.value))
		equal(t, o.Label, w.label)
		equal(t, o.Selected, w.selected)
		equal(t, o.Index, i)
		if w.selected {
			equal(t, len(o.Attrs), 1)
			equal(t, o.Attrs["selected"], any(true))
		} else {
			equal(t, len(o.Attrs), 0)
		}
	}
}

// Ported from Django: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_render_as_subwidget
func TestSelectRenderAsSubwidget(t *testing.T) {
	// A Select as a subwidget of a MultiWidget.
	choices := append([]forms.Choice{{Value: "", Label: "------"}}, widgetBeatles...)
	w := forms.MultiWidget{Widgets: []forms.Widget{forms.Select{Choices: choices}, forms.TextInput{}}}
	widgetCheckHTML(t, w, "beatle", []any{"J", "Some text"}, nil, `
                <select name="beatle_0">
                  <option value="">------</option>
                  <option value="J" selected>John</option>
                  <option value="P">Paul</option>
                  <option value="G">George</option>
                  <option value="R">Ringo</option>
                </select>
                <input name="beatle_1" type="text" value="Some text">`)
}

// No Go counterpart: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_choices_constructor_generator:
// Choices is a slice, which a render does not use up.

// No Go counterpart: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_choices_escaping:
// a label is plain text, which the widget always escapes. There is no safe
// string to leave unescaped.

// No Go counterpart: tests/forms_tests/widget_tests/test_select.py::SelectTest.test_doesnt_localize_option_value:
// an option value is a string, and there is no localization.
