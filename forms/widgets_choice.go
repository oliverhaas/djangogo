package forms

import (
	"net/url"
	"strconv"
	"strings"
)

// Choice is one option of a choice field or widget: the submitted value and
// the text shown for it.
type Choice struct {
	// Value is the value submitted when the option is chosen.
	Value string
	// Label is the text shown to the user.
	Label string
}

// EmptyChoiceLabel is the label of the empty option that a model choice field
// puts first, Django's "---------".
const EmptyChoiceLabel = "---------"

// ChoiceWidget is a widget that offers a set of options. A choice field hands
// its choices to the widget through WithChoices each time it renders.
type ChoiceWidget interface {
	Widget
	// WithChoices returns a copy of the widget that offers choices.
	WithChoices(choices []Choice) Widget
}

// choiceValues returns the option values that are selected for a field value,
// Django's ChoiceWidget.format_value. For a single-valued widget nil selects
// the empty option.
func choiceValues(value any, multiple bool) []string {
	switch v := value.(type) {
	case nil:
		if multiple {
			return nil
		}
		return []string{""}
	case []string:
		return v
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			if item != nil {
				out[i] = pyString(item)
			}
		}
		return out
	default:
		return []string{pyString(v)}
	}
}

// selectedIndexes marks the options that are selected. A single-valued widget
// selects only the first match.
func selectedIndexes(choices []Choice, value any, multiple bool) []bool {
	chosen := map[string]bool{}
	for _, v := range choiceValues(value, multiple) {
		chosen[v] = true
	}
	out := make([]bool, len(choices))
	found := false
	for i, c := range choices {
		if chosen[c.Value] && (multiple || !found) {
			out[i] = true
			found = true
		}
	}
	return out
}

// renderSelect returns a <select> element with its options.
func renderSelect(name string, choices []Choice, value any, multiple bool, layers ...Attrs) string {
	attrs := mergeAttrs(layers...)
	if multiple {
		attrs["multiple"] = true
	}
	var b strings.Builder
	b.WriteString(`<select name="` + escape(name) + `"` + RenderAttrs(attrs) + ">")
	selected := selectedIndexes(choices, value, multiple)
	for i, c := range choices {
		b.WriteString("\n  <option value=\"" + escape(c.Value) + `"`)
		if selected[i] {
			b.WriteString(" selected")
		}
		b.WriteString(">" + escape(c.Label) + "</option>")
	}
	b.WriteString("\n</select>")
	return b.String()
}

// Select is a <select> drop-down.
type Select struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// Choices are the options.
	Choices []Choice
}

// Render implements Widget.
func (w Select) Render(name string, value any, attrs Attrs) string {
	return renderSelect(name, w.Choices, value, false, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w Select) Attributes() Attrs { return w.Attrs }

// WithChoices implements ChoiceWidget.
func (w Select) WithChoices(choices []Choice) Widget {
	w.Choices = choices
	return w
}

// UseRequiredAttribute allows the required attribute only when the first
// option is the empty one: a required <select> whose first option has a value
// is invalid HTML.
func (w Select) UseRequiredAttribute(any) bool {
	return len(w.Choices) > 0 && w.Choices[0].Value == ""
}

// NullBooleanSelect is a <select> with the options Unknown, Yes and No.
type NullBooleanSelect struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// nullBooleanChoices are the fixed options of a NullBooleanSelect.
var nullBooleanChoices = []Choice{{"unknown", "Unknown"}, {"true", "Yes"}, {"false", "No"}}

// nullBoolean maps the accepted spellings of a three-state value to true,
// false or nil. The numbers are the values Django rendered before 3.1.
func nullBoolean(value any) any {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch v {
		case "True", "true", "2":
			return true
		case "False", "false", "3":
			return false
		}
	}
	return nil
}

// Render implements Widget.
func (w NullBooleanSelect) Render(name string, value any, attrs Attrs) string {
	selected := "unknown"
	if b, ok := nullBoolean(value).(bool); ok {
		selected = strconv.FormatBool(b)
	}
	return renderSelect(name, nullBooleanChoices, selected, false, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w NullBooleanSelect) Attributes() Attrs { return w.Attrs }

// ValueFromData returns true, false or nil.
func (NullBooleanSelect) ValueFromData(data url.Values, name string) any {
	return nullBoolean(lastValue(data, name))
}

// UseRequiredAttribute reports false: the first option has a value.
func (NullBooleanSelect) UseRequiredAttribute(any) bool { return false }

// SelectMultiple is a <select multiple> list.
type SelectMultiple struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// Choices are the options.
	Choices []Choice
}

// Render implements Widget.
func (w SelectMultiple) Render(name string, value any, attrs Attrs) string {
	return renderSelect(name, w.Choices, value, true, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w SelectMultiple) Attributes() Attrs { return w.Attrs }

// WithChoices implements ChoiceWidget.
func (w SelectMultiple) WithChoices(choices []Choice) Widget {
	w.Choices = choices
	return w
}

// ValueFromData returns every value submitted under name.
func (SelectMultiple) ValueFromData(data url.Values, name string) any {
	return append([]string{}, data[name]...)
}

// ValueOmittedFromData reports false: a list with nothing selected is absent
// from the data.
func (SelectMultiple) ValueOmittedFromData(url.Values, string) bool { return false }

// renderInputGroup returns a group of radio buttons or checkboxes, one per
// choice, each wrapped in its label.
func renderInputGroup(inputType, name string, choices []Choice, value any, multiple bool, layers ...Attrs) string {
	attrs := mergeAttrs(layers...)
	id, _ := attrs["id"].(string)
	var b strings.Builder
	b.WriteString("<div")
	if id != "" {
		b.WriteString(` id="` + escape(id) + `"`)
	}
	if class, ok := attrs["class"]; ok && class != nil {
		b.WriteString(` class="` + escape(pyString(class)) + `"`)
	}
	b.WriteString(">")
	selected := selectedIndexes(choices, value, multiple)
	for i, c := range choices {
		option := mergeAttrs(attrs)
		label := "<label>"
		if id != "" {
			optionID := id + "_" + strconv.Itoa(i)
			option["id"] = optionID
			label = `<label for="` + escape(optionID) + `">`
		}
		if selected[i] {
			option["checked"] = true
		}
		b.WriteString("<div>\n    " + label)
		// The value attribute is written even when empty, so that the empty
		// choice submits "" and not the browser default "on".
		b.WriteString(`<input type="` + inputType + `" name="` + escape(name) + `" value="` + escape(c.Value) + `"`)
		b.WriteString(RenderAttrs(option) + "> " + escape(c.Label) + "</label></div>")
	}
	b.WriteString("\n</div>")
	return b.String()
}

// RadioSelect is a group of radio buttons.
type RadioSelect struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// Choices are the options.
	Choices []Choice
}

// Render implements Widget.
func (w RadioSelect) Render(name string, value any, attrs Attrs) string {
	return renderInputGroup("radio", name, w.Choices, value, false, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w RadioSelect) Attributes() Attrs { return w.Attrs }

// WithChoices implements ChoiceWidget.
func (w RadioSelect) WithChoices(choices []Choice) Widget {
	w.Choices = choices
	return w
}

// IDForLabel returns the empty string: a label pointing at the first button
// would toggle it on a click and mislead a screen reader.
func (RadioSelect) IDForLabel(string) string { return "" }

// UseFieldset reports true.
func (RadioSelect) UseFieldset() bool { return true }

// CheckboxSelectMultiple is a group of checkboxes.
type CheckboxSelectMultiple struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// Choices are the options.
	Choices []Choice
}

// Render implements Widget.
func (w CheckboxSelectMultiple) Render(name string, value any, attrs Attrs) string {
	return renderInputGroup("checkbox", name, w.Choices, value, true, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w CheckboxSelectMultiple) Attributes() Attrs { return w.Attrs }

// WithChoices implements ChoiceWidget.
func (w CheckboxSelectMultiple) WithChoices(choices []Choice) Widget {
	w.Choices = choices
	return w
}

// ValueFromData returns every value submitted under name.
func (CheckboxSelectMultiple) ValueFromData(data url.Values, name string) any {
	return append([]string{}, data[name]...)
}

// ValueOmittedFromData reports false: a group with nothing checked is absent
// from the data.
func (CheckboxSelectMultiple) ValueOmittedFromData(url.Values, string) bool { return false }

// IDForLabel returns the empty string, as for RadioSelect.
func (CheckboxSelectMultiple) IDForLabel(string) string { return "" }

// UseRequiredAttribute reports false: required on every box would demand that
// all of them are checked.
func (CheckboxSelectMultiple) UseRequiredAttribute(any) bool { return false }

// UseFieldset reports true.
func (CheckboxSelectMultiple) UseFieldset() bool { return true }

// MultipleHiddenInput is a set of <input type="hidden"> elements, one per
// value. It carries the values of a multiple choice field through a page.
type MultipleHiddenInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// Render implements Widget. Each input gets the id with its index appended.
func (w MultipleHiddenInput) Render(name string, value any, attrs Attrs) string {
	merged := mergeAttrs(w.Attrs, attrs)
	id, hasID := merged["id"]
	values := choiceValues(value, true)
	inputs := make([]string, 0, len(values))
	for i, v := range values {
		layer := Attrs{}
		if hasID && id != nil {
			layer["id"] = pyString(id) + "_" + strconv.Itoa(i)
		}
		inputs = append(inputs, renderInput("hidden", name, v, merged, layer))
	}
	return strings.Join(inputs, "\n")
}

// Attributes implements Widget.
func (w MultipleHiddenInput) Attributes() Attrs { return w.Attrs }

// ValueFromData returns every value submitted under name.
func (MultipleHiddenInput) ValueFromData(data url.Values, name string) any {
	return append([]string(nil), data[name]...)
}

// IsHidden reports true.
func (MultipleHiddenInput) IsHidden() bool { return true }

// UseRequiredAttribute reports false: a hidden input cannot be filled in.
func (MultipleHiddenInput) UseRequiredAttribute(any) bool { return false }
