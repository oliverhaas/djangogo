package forms_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// widgetBlankChoice is BLANK_CHOICE of the radio tests.
var widgetBlankChoice = []forms.Choice{{Value: "", Label: "- Select an option -"}}

// widgetBeatlesRadio is the expected radio group for the beatles with the
// given id and class on the container, and the checked value; the empty
// value in checked selects the blank choice.
func widgetBeatlesRadio(blank bool, checked string) string {
	out := "<div>"
	row := func(value, label string, isChecked bool) {
		mark := ""
		if isChecked {
			mark = " checked"
		}
		out += `<div><label><input` + mark + ` type="radio" name="beatle" value="` + value + `">` + label + "</label></div>"
	}
	if blank {
		row("", "- Select an option -", checked == "")
	}
	for _, c := range widgetBeatles {
		row(c.Value, c.Label, c.Value == checked)
	}
	return out + "</div>"
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_render
func TestRadioSelectRender(t *testing.T) {
	w := forms.RadioSelect{Choices: append(append([]forms.Choice{}, widgetBlankChoice...), widgetBeatles...)}
	widgetCheckHTML(t, w, "beatle", "J", nil, widgetBeatlesRadio(true, "J"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_render_none
func TestRadioSelectRenderNone(t *testing.T) {
	// If value is None, none of the options are selected.
	w := forms.RadioSelect{Choices: append(append([]forms.Choice{}, widgetBlankChoice...), widgetBeatles...)}
	widgetCheckHTML(t, w, "beatle", nil, nil, widgetBeatlesRadio(true, ""))
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_render_label_value
func TestRadioSelectRenderLabelValue(t *testing.T) {
	// A value that matches a label but no option value selects nothing.
	widgetCheckHTML(t, forms.RadioSelect{Choices: widgetBeatles}, "beatle", "Ringo", nil, widgetBeatlesRadio(false, "Ringo"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_render_selected
func TestRadioSelectRenderSelected(t *testing.T) {
	// Only one option can be selected.
	choices := []forms.Choice{{Value: "0", Label: "0"}, {Value: "1", Label: "1"}, {Value: "2", Label: "2"}, {Value: "3", Label: "3"}, {Value: "0", Label: "extra"}}
	widgetCheckHTML(t, forms.RadioSelect{Choices: choices}, "choices", "0", nil, `
        <div>
          <div>
            <label><input checked type="radio" name="choices" value="0">0</label>
          </div>
          <div>
            <label><input type="radio" name="choices" value="1">1</label>
          </div>
          <div>
            <label><input type="radio" name="choices" value="2">2</label>
          </div>
          <div>
            <label><input type="radio" name="choices" value="3">3</label>
          </div>
          <div>
            <label><input type="radio" name="choices" value="0">extra</label>
          </div>
        </div>`)
}

// widgetBeatlesRadioID is the expected radio group of the beatles with ids.
const widgetBeatlesRadioID = `
        <div id="ID">
          <div>
            <label for="ID_0">
            <input checked type="radio" id="ID_0" value="J" name="beatle">John</label>
          </div>
          <div><label for="ID_1">
            <input type="radio" id="ID_1" value="P" name="beatle">Paul</label>
          </div>
          <div><label for="ID_2">
            <input type="radio" id="ID_2" value="G" name="beatle">George</label>
          </div>
          <div><label for="ID_3">
            <input type="radio" id="ID_3" value="R" name="beatle">Ringo</label>
          </div>
        </div>`

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_constructor_attrs
func TestRadioSelectConstructorAttrs(t *testing.T) {
	// Attributes provided at instantiation are passed to the constituent inputs.
	w := forms.RadioSelect{Attrs: forms.Attrs{"id": "foo"}, Choices: widgetBeatles}
	widgetCheckHTML(t, w, "beatle", "J", nil, strings.ReplaceAll(widgetBeatlesRadioID, "ID", "foo"))
}

// widgetNumRadio is the expected radio group of the numbers 1 to 3 with 3 checked.
const widgetNumRadio = `
        <div>
          <div>
            <label><input type="radio" name="num" value="1">1</label>
          </div>
          <div>
            <label><input type="radio" name="num" value="2">2</label>
          </div>
          <div>
            <label><input checked type="radio" name="num" value="3">3</label>
          </div>
        </div>`

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_compare_to_str
func TestRadioSelectCompareToStr(t *testing.T) {
	// The value is compared to its str().
	w := forms.RadioSelect{Choices: widgetNumChoices}
	widgetCheckHTML(t, w, "num", 3, nil, widgetNumRadio)
	widgetCheckHTML(t, w, "num", "3", nil, widgetNumRadio)
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_choices_constructor
func TestRadioSelectChoicesConstructor(t *testing.T) {
	widgetCheckHTML(t, forms.RadioSelect{Choices: widgetNumChoices}, "num", 3, nil, widgetNumRadio)
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_choices_unicode
func TestRadioSelectChoicesUnicode(t *testing.T) {
	w := forms.RadioSelect{Choices: []forms.Choice{{Value: "ŠĐĆŽćžšđ", Label: "ŠĐabcĆŽćžšđ"}, {Value: "ćžšđ", Label: "abcćžšđ"}}}
	widgetCheckHTML(t, w, "email", "ŠĐĆŽćžšđ", nil, `
        <div>
          <div>
            <label>
            <input checked type="radio" name="email"
              value="ŠĐĆŽćžšđ">
            ŠĐabcĆŽćžšđ</label>
          </div>
          <div>
            <label>
            <input type="radio" name="email" value="ćžšđ">
            abcćžšđ</label>
          </div>
        </div>`)
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_render_attrs
func TestRadioSelectRenderAttrs(t *testing.T) {
	// Attributes provided at render-time are passed to the constituent inputs.
	widgetCheckHTML(t, forms.RadioSelect{Choices: widgetBeatles}, "beatle", "J", forms.Attrs{"id": "bar"},
		strings.ReplaceAll(widgetBeatlesRadioID, "ID", "bar"))
}

// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_class_attrs
func TestRadioSelectClassAttrs(t *testing.T) {
	// The outer <div> of the widget template includes the class attribute.
	widgetCheckHTML(t, forms.RadioSelect{Choices: widgetBeatles}, "beatle", "J", forms.Attrs{"class": "bar"}, `
        <div class="bar">
          <div><label>
            <input checked type="radio" class="bar" value="J" name="beatle">John</label>
          </div>
          <div><label>
            <input type="radio" class="bar" value="P" name="beatle">Paul</label>
          </div>
          <div><label>
            <input type="radio" class="bar" value="G" name="beatle">George</label>
          </div>
          <div><label>
            <input type="radio" class="bar" value="R" name="beatle">Ringo</label>
          </div>
        </div>`)
}

// The Django template wrapping the field (label and fieldset) is not ported;
// the widget HTML is what the form renders.
// Ported from Django: tests/forms_tests/widget_tests/test_radioselect.py::RadioSelectTest.test_fieldset
func TestRadioSelectFieldset(t *testing.T) {
	w := forms.RadioSelect{}
	equal(t, w.UseFieldset(), true)
	field := forms.NewChoiceField(forms.UseWidget(w), forms.Required(false), forms.Choices("J", "John", "P", "Paul", "G", "George", "R", "Ringo"))
	assertHTMLEqual(t, widgetFieldHTML(field),
		`<div id="id_field">
		<div><label for="id_field_0"><input type="radio" name="field" value="J" id="id_field_0"> John</label></div>
		<div><label for="id_field_1"><input type="radio" name="field" value="P" id="id_field_1">Paul</label></div>
		<div><label for="id_field_2"><input type="radio" name="field" value="G" id="id_field_2"> George</label></div>
		<div><label for="id_field_3"><input type="radio" name="field" value="R" id="id_field_3">Ringo</label></div></div>`)
}
