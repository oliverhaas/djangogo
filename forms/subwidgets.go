package forms

import (
	"strconv"
	"strings"
)

// BoundWidget is a part of the widget of a bound field, Django's
// BoundWidget: an option of a choice widget, such as one radio button of a
// RadioSelect, or the whole widget of any other field. BoundField.Subwidgets
// returns them, and a template gets them from a loop over a field:
//
//	{% for radio in form.language %}<div class="radio">{{ radio }}</div>{% endfor %}
type BoundWidget struct {
	// Name is the name attribute.
	Name string
	// Value is the value of the option, or the value of the field for a
	// widget without options.
	Value any
	// Label is the label of the option, "" for a widget without options.
	Label string
	// Selected reports whether the option is selected or checked.
	Selected bool
	// Index is the position of the option among the options, from 0.
	Index int
	// Attrs are the HTML attributes of the option.
	Attrs Attrs

	render func(wrapLabel bool) string
}

// String returns the HTML of the part: an option input in its <label>, an
// <option>, or the whole widget.
func (w BoundWidget) String() string {
	if w.render == nil {
		return ""
	}
	return w.render(true)
}

// HTML returns the rendered markup; templates print it without escaping.
func (w BoundWidget) HTML() string { return w.String() }

// Tag returns the HTML of the part without the <label> that String wraps an
// option input in.
func (w BoundWidget) Tag() string {
	if w.render == nil {
		return ""
	}
	return w.render(false)
}

// IDForLabel returns the id of the part, "" when it has none.
func (w BoundWidget) IDForLabel() string {
	id, _ := w.Attrs["id"].(string)
	return id
}

// optionWidget is a widget made of options, Django's ChoiceWidget: its
// subwidgets are its options.
type optionWidget interface {
	// options returns the options for the name, the value of the field and
	// attrs, the attributes the bound field adds to those of the widget.
	options(name string, value any, attrs Attrs) []BoundWidget
}

// Subwidgets returns the parts of the widget, Django's BoundField.subwidgets:
// the options of a choice widget, such as the radio buttons of a RadioSelect,
// and the widget itself for any other field.
func (bf *BoundField) Subwidgets() []BoundWidget {
	w, attrs, _ := bf.widgetAttrs(nil, nil)
	name, value := bf.HTMLName(), bf.Value()
	if ow, ok := w.(optionWidget); ok {
		return ow.options(name, value, attrs)
	}
	return []BoundWidget{{
		Name:   name,
		Value:  value,
		Attrs:  mergeAttrs(w.Attributes(), attrs),
		render: func(bool) string { return bf.String() },
	}}
}

// selectOption returns one <option> of a <select>, Django's
// select_option.html.
func selectOption(c Choice, selected bool) string {
	s := `<option value="` + escape(c.Value) + `"`
	if selected {
		s += " selected"
	}
	return s + ">" + escape(c.Label) + "</option>"
}

// selectOptions returns the options of a <select>. An option does not take
// the attributes of the widget, as in Django.
func selectOptions(name string, choices []Choice, value any, multiple bool) []BoundWidget {
	selected := selectedIndexes(choices, value, multiple)
	out := make([]BoundWidget, len(choices))
	for i, c := range choices {
		attrs := Attrs{}
		if selected[i] {
			attrs["selected"] = true
		}
		out[i] = BoundWidget{
			Name: name, Value: c.Value, Label: c.Label, Selected: selected[i], Index: i, Attrs: attrs,
			render: func(bool) string { return selectOption(c, selected[i]) },
		}
	}
	return out
}

// inputOption returns one radio button or checkbox, in its <label> when
// wrapLabel is true, Django's input_option.html.
func inputOption(inputType, name string, c Choice, attrs Attrs, wrapLabel bool) string {
	var b strings.Builder
	if wrapLabel {
		if id, _ := attrs["id"].(string); id != "" {
			b.WriteString(`<label for="` + escape(id) + `">`)
		} else {
			b.WriteString("<label>")
		}
	}
	// The value attribute is written even when empty, so that the empty
	// choice submits "" and not the browser default "on".
	b.WriteString(`<input type="` + inputType + `" name="` + escape(name) + `" value="` + escape(c.Value) + `"`)
	b.WriteString(RenderAttrs(attrs) + ">")
	if wrapLabel {
		b.WriteString(" " + escape(c.Label) + "</label>")
	}
	return b.String()
}

// inputOptions returns the radio buttons or checkboxes of a group, one per
// choice. Each takes the attributes of the group, with the id suffixed by
// its index.
func inputOptions(inputType, name string, choices []Choice, value any, multiple bool, attrs Attrs) []BoundWidget {
	id, _ := attrs["id"].(string)
	selected := selectedIndexes(choices, value, multiple)
	out := make([]BoundWidget, len(choices))
	for i, c := range choices {
		option := mergeAttrs(attrs)
		if id != "" {
			option["id"] = id + "_" + strconv.Itoa(i)
		}
		if selected[i] {
			option["checked"] = true
		}
		out[i] = BoundWidget{
			Name: name, Value: c.Value, Label: c.Label, Selected: selected[i], Index: i, Attrs: option,
			render: func(wrapLabel bool) string { return inputOption(inputType, name, c, option, wrapLabel) },
		}
	}
	return out
}
