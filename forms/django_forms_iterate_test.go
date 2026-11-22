package forms_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// beatleForm is the BeatleForm of Django's tests of an iterable BoundField:
// a choice field name with four Beatles.
func beatleForm(opts ...forms.FieldOption) *forms.Form {
	opts = append([]forms.FieldOption{forms.Choices("john", "John", "paul", "Paul", "george", "George", "ringo", "Ringo")}, opts...)
	return forms.New(forms.F("name", forms.NewChoiceField(opts...)))
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_form_with_iterable_boundfield
func TestFormWithIterableBoundfield(t *testing.T) {
	subwidgets := withoutAutoID(beatleForm(forms.UseWidget(forms.RadioSelect{}))).Field("name").Subwidgets()
	lines := make([]string, 0, len(subwidgets))
	divs := make([]string, 0, len(subwidgets))
	for _, w := range subwidgets {
		lines = append(lines, w.String())
		divs = append(divs, "<div>"+w.String()+"</div>")
	}
	assertHTMLEqual(t, strings.Join(lines, "\n"),
		`<label><input type="radio" name="name" value="john" required> John</label>`+
			`<label><input type="radio" name="name" value="paul" required> Paul</label>`+
			`<label><input type="radio" name="name" value="george" required> George`+
			"</label>"+
			`<label><input type="radio" name="name" value="ringo" required> Ringo`+
			"</label>")
	assertHTMLEqual(t, strings.Join(divs, "\n"), `
<div><label>
<input type="radio" name="name" value="john" required> John</label></div>
<div><label>
<input type="radio" name="name" value="paul" required> Paul</label></div>
<div><label>
<input type="radio" name="name" value="george" required> George
</label></div>
<div><label>
<input type="radio" name="name" value="ringo" required> Ringo</label></div>
`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_form_with_iterable_boundfield_id
func TestFormWithIterableBoundfieldID(t *testing.T) {
	fields := beatleForm(forms.UseWidget(forms.RadioSelect{})).Field("name").Subwidgets()
	equal(t, len(fields), 4)

	equal(t, fields[0].IDForLabel(), "id_name_0")
	equal(t, fields[0].Label, "John")
	assertHTMLEqual(t, fields[0].Tag(), `<input type="radio" name="name" value="john" id="id_name_0" required>`)
	assertHTMLEqual(t, fields[0].String(),
		`<label for="id_name_0"><input type="radio" name="name" `+
			`value="john" id="id_name_0" required> John</label>`)

	equal(t, fields[1].IDForLabel(), "id_name_1")
	equal(t, fields[1].Label, "Paul")
	assertHTMLEqual(t, fields[1].Tag(), `<input type="radio" name="name" value="paul" id="id_name_1" required>`)
	assertHTMLEqual(t, fields[1].String(),
		`<label for="id_name_1"><input type="radio" name="name" `+
			`value="paul" id="id_name_1" required> Paul</label>`)
}

// Django's id_for_label is None for an option without an id; IDForLabel
// returns "".
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_iterable_boundfield_select
func TestIterableBoundfieldSelect(t *testing.T) {
	fields := withoutAutoID(beatleForm()).Field("name").Subwidgets()
	equal(t, len(fields), 4)

	equal(t, fields[0].IDForLabel(), "")
	equal(t, fields[0].Label, "John")
	assertHTMLEqual(t, fields[0].Tag(), `<option value="john">John</option>`)
	assertHTMLEqual(t, fields[0].String(), `<option value="john">John</option>`)
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_form_with_noniterable_boundfield
func TestFormWithNoniterableBoundfield(t *testing.T) {
	// Any bound field has subwidgets, not just those with a RadioSelect.
	subwidgets := withoutAutoID(forms.New(forms.F("name", forms.NewCharField()))).Field("name").Subwidgets()
	lines := make([]string, 0, len(subwidgets))
	for _, w := range subwidgets {
		lines = append(lines, w.String())
	}
	assertHTMLEqual(t, strings.Join(lines, "\n"), `<input type="text" name="name" required>`)
}

// No Go counterpart: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_slice
// and test_boundfield_invalid_index: a BoundField has no index operator, and
// Subwidgets returns a slice.

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_subwidget_id_for_label
func TestBoundfieldSubwidgetIDForLabel(t *testing.T) {
	// The ids of the subwidgets follow the AutoID of the form.
	form := forms.New(forms.F("field", forms.NewMultipleChoiceField(
		forms.Choices("a", "A", "b", "B"), forms.UseWidget(forms.CheckboxSelectMultiple{}))))
	form.AutoID = "prefix_%s"
	subwidgets := form.Field("field").Subwidgets()
	equal(t, subwidgets[0].IDForLabel(), "prefix_field_0")
	equal(t, subwidgets[1].IDForLabel(), "prefix_field_1")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::FormsTestCase.test_boundfield_bool
func TestBoundfieldBool(t *testing.T) {
	form := forms.New(forms.F("name", forms.NewChoiceField()))
	equal(t, renderTemplate(t, "{% if form.name %}true{% else %}false{% endif %}", map[string]any{"form": form}), "true")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::TemplateTests.test_iterate_radios
func TestIterateRadios(t *testing.T) {
	f := frameworkForm()
	f.AutoID = "id_%s"
	src := "{% for radio in form.language %}" +
		`<div class="myradio">{{ radio }}</div>` +
		"{% endfor %}"
	assertHTMLEqual(t, renderTemplate(t, src, map[string]any{"form": f}),
		`<div class="myradio"><label for="id_language_0">`+
			`<input id="id_language_0" name="language" type="radio" value="P" `+
			"required> Python</label></div>"+
			`<div class="myradio"><label for="id_language_1">`+
			`<input id="id_language_1" name="language" type="radio" value="J" `+
			"required> Java</label></div>")
}

// Ported from Django: tests/forms_tests/tests/test_forms.py::TemplateTests.test_iterate_checkboxes
func TestIterateCheckboxes(t *testing.T) {
	f := withoutAutoID(songForm(forms.NewMultipleChoiceField(composerChoices(),
		forms.UseWidget(forms.CheckboxSelectMultiple{}))).Bind(vals("composers", "J", "composers", "P")))
	src := "{% for checkbox in form.composers %}" +
		`<div class="mycheckbox">{{ checkbox }}</div>` +
		"{% endfor %}"
	assertHTMLEqual(t, renderTemplate(t, src, map[string]any{"form": f}),
		`<div class="mycheckbox"><label>`+
			`<input checked name="composers" type="checkbox" value="J"> `+
			"John Lennon</label></div>"+
			`<div class="mycheckbox"><label>`+
			`<input checked name="composers" type="checkbox" value="P"> `+
			"Paul McCartney</label></div>")
}

// No Django test exists for the attributes of a subwidget in a template,
// which Django documents with a loop over the radio buttons of a field.
func TestIterateRadiosAttributes(t *testing.T) {
	f := frameworkForm().Bind(vals("name", "x", "language", "J"))
	src := "{% for radio in form.language %}" +
		`<label for="{{ radio.id_for_label }}">{{ radio.choice_label }}` +
		`<span class="radio">{{ radio.tag }}</span></label>` +
		"{{ radio.data.value }}{{ radio.data.index }}{% if radio.data.selected %}*{% endif %};" +
		"{% endfor %}"
	assertHTMLEqual(t, renderTemplate(t, src, map[string]any{"form": f}),
		`<label for="id_language_0">Python<span class="radio">`+
			`<input type="radio" name="language" value="P" required id="id_language_0"></span></label>P0;`+
			`<label for="id_language_1">Java<span class="radio">`+
			`<input type="radio" name="language" value="J" required id="id_language_1" checked></span></label>J1*;`)
}

// formSnippet is Django's forms_tests/form_snippet.html.
const formSnippet = `{% for field in form %}
  <div class="fieldWrapper">
    {{ field.errors }}
    {{ field.label_tag }} {{ field }}
  </div>
{% endfor %}
`

// A form has no template_name of its own, so the template renders
// form_snippet.html with the form in its context, as Django's form does.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::TemplateTests.test_per_form_template_name
func TestPerFormTemplateName(t *testing.T) {
	person := forms.New(forms.F("first_name", forms.NewCharField()))
	assertHTMLEqual(t, renderTemplate(t, formSnippet, map[string]any{"form": person}), `
<div class="fieldWrapper"><label for="id_first_name">First name:</label>
<input type="text" name="first_name" required id="id_first_name"></div>
`)
}

// As in TestPerFormTemplateName, the template renders legend_test.html with
// the form in its context.
//
// Ported from Django: tests/forms_tests/tests/test_forms.py::TemplateTests.test_legend_tag
func TestLegendTag(t *testing.T) {
	legendTest := `{% for field in form %}
  {% if field.widget_type == 'radioselect' %}
    {{ field.legend_tag }}
  {% else %}
    {{ field.label_tag }}
  {% endif %}
{% endfor %}
`
	f := frameworkForm()
	f.RequiredCSSClass = "required"
	assertHTMLEqual(t, renderTemplate(t, legendTest, map[string]any{"form": f}),
		`<label for="id_name" class="required">Name:</label>`+
			`<legend class="required">Language:</legend>`)
}
