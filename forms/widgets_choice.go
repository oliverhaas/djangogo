package forms

import (
	"net/url"
	"strconv"
	"strings"
)

// Choice is one option of a choice field or widget: the submitted value and
// the text shown for it. A choice with options in Group is an option group:
// Label names the group and Value is not used.
//
//	forms.Choice{Label: "Audio", Group: []forms.Choice{{Value: "cd", Label: "CD"}}}
type Choice struct {
	// Value is the value submitted when the option is chosen.
	Value string
	// Label is the text shown to the user, or the name of a group.
	Label string
	// Group holds the options of an option group, Django's nested choices.
	// A Select shows them in an <optgroup>, a RadioSelect and a
	// CheckboxSelectMultiple under the name of the group.
	Group []Choice
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

// optGroup is the options of one option group, or one option outside any
// group with an empty label, an entry of Django's ChoiceWidget.optgroups.
type optGroup struct {
	label   string
	options []BoundWidget
}

// optGroups returns the options of choices in their groups, Django's
// ChoiceWidget.optgroups. makeOption builds an option from its choice, its
// index ("2", or "1_0" for the first option of the second choice, a group)
// and whether it is selected. A single-valued widget selects only the first
// match.
func optGroups(choices []Choice, value any, multiple bool, makeOption func(c Choice, index string, selected bool) BoundWidget) []optGroup {
	chosen := map[string]bool{}
	for _, v := range choiceValues(value, multiple) {
		chosen[v] = true
	}
	found, position := false, 0
	option := func(c Choice, index string) BoundWidget {
		selected := (multiple || !found) && chosen[c.Value]
		found = found || selected
		o := makeOption(c, index, selected)
		o.Index = position
		position++
		return o
	}
	groups := make([]optGroup, len(choices))
	for i, c := range choices {
		if len(c.Group) == 0 {
			groups[i].options = []BoundWidget{option(c, strconv.Itoa(i))}
			continue
		}
		groups[i].label = c.Label
		for j, sub := range c.Group {
			groups[i].options = append(groups[i].options, option(sub, strconv.Itoa(i)+"_"+strconv.Itoa(j)))
		}
	}
	return groups
}

// flatOptions returns the options of groups in order, Django's
// ChoiceWidget.options.
func flatOptions(groups []optGroup) []BoundWidget {
	var out []BoundWidget
	for _, g := range groups {
		out = append(out, g.options...)
	}
	return out
}

// renderSelect returns a <select> element with its options, Django's
// select.html.
func renderSelect(name string, choices []Choice, value any, multiple bool, layers ...Attrs) string {
	attrs := mergeAttrs(layers...)
	if multiple {
		attrs["multiple"] = true
	}
	var b strings.Builder
	b.WriteString(`<select name="` + escape(name) + `"` + RenderAttrs(attrs) + ">")
	for _, g := range selectGroups(name, choices, value, multiple) {
		if g.label != "" {
			b.WriteString("\n  <optgroup label=\"" + escape(g.label) + "\">")
		}
		for _, o := range g.options {
			b.WriteString("\n  " + o.String())
		}
		if g.label != "" {
			b.WriteString("\n  </optgroup>")
		}
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

// options implements optionWidget.
func (w Select) options(name string, value any, _ Attrs) []BoundWidget {
	return flatOptions(selectGroups(name, w.Choices, value, false))
}

// UseRequiredAttribute allows the required attribute only when the first
// option is the empty one: a required <select> whose first option has a value
// is invalid HTML. As in Django, a first choice that is a group counts as
// empty when its name is.
func (w Select) UseRequiredAttribute(any) bool {
	if len(w.Choices) == 0 {
		return false
	}
	first := w.Choices[0]
	if len(first.Group) > 0 {
		return first.Label == ""
	}
	return first.Value == ""
}

// NullBooleanSelect is a <select> with the options Unknown, Yes and No.
type NullBooleanSelect struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// nullBooleanChoices are the fixed options of a NullBooleanSelect.
var nullBooleanChoices = []Choice{
	{Value: "unknown", Label: "Unknown"}, {Value: "true", Label: "Yes"}, {Value: "false", Label: "No"},
}

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

// nullBooleanOption returns the value of the option a NullBooleanSelect
// selects for value.
func nullBooleanOption(value any) string {
	if b, ok := nullBoolean(value).(bool); ok {
		return strconv.FormatBool(b)
	}
	return "unknown"
}

// Render implements Widget.
func (w NullBooleanSelect) Render(name string, value any, attrs Attrs) string {
	return renderSelect(name, nullBooleanChoices, nullBooleanOption(value), false, w.Attrs, attrs)
}

// options implements optionWidget.
func (NullBooleanSelect) options(name string, value any, _ Attrs) []BoundWidget {
	return flatOptions(selectGroups(name, nullBooleanChoices, nullBooleanOption(value), false))
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

// options implements optionWidget.
func (w SelectMultiple) options(name string, value any, _ Attrs) []BoundWidget {
	return flatOptions(selectGroups(name, w.Choices, value, true))
}

// ValueFromData returns every value submitted under name.
func (SelectMultiple) ValueFromData(data url.Values, name string) any {
	return append([]string{}, data[name]...)
}

// ValueOmittedFromData reports false: a list with nothing selected is absent
// from the data.
func (SelectMultiple) ValueOmittedFromData(url.Values, string) bool { return false }

// renderInputGroup returns a group of radio buttons or checkboxes, one per
// choice, each wrapped in its label, Django's multiple_input.html. The
// options of an option group follow the name of the group.
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
	for _, g := range inputGroups(inputType, name, choices, value, multiple, attrs) {
		if g.label != "" {
			b.WriteString("\n  <div><label>" + escape(g.label) + "</label>")
		}
		for _, option := range g.options {
			b.WriteString("<div>\n    " + option.String() + "</div>")
		}
		if g.label != "" {
			b.WriteString("\n  </div>")
		}
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

// options implements optionWidget.
func (w RadioSelect) options(name string, value any, attrs Attrs) []BoundWidget {
	return flatOptions(inputGroups("radio", name, w.Choices, value, false, mergeAttrs(w.Attrs, attrs)))
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

// options implements optionWidget.
func (w CheckboxSelectMultiple) options(name string, value any, attrs Attrs) []BoundWidget {
	return flatOptions(inputGroups("checkbox", name, w.Choices, value, true, mergeAttrs(w.Attrs, attrs)))
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
