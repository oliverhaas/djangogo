package forms

import (
	"net/url"
	"sort"
	"strings"
	"time"
)

// Attrs are the HTML attributes of a widget. A true value renders the bare
// attribute name (required), false and nil omit the attribute, and any other
// value renders name="value" with the value escaped.
type Attrs map[string]any

// Widget renders a form field as HTML and reads its value from submitted
// data, the counterpart of Django's Widget. The widgets of this package are
// plain structs, usable as literals: forms.Textarea{Attrs: forms.Attrs{"rows": 3}}.
type Widget interface {
	// Render returns the HTML of the widget. value is the value to display (the
	// submitted data or the initial value) and attrs are merged over the
	// widget's own attributes.
	Render(name string, value any, attrs Attrs) string
	// ValueFromData extracts the widget's value from submitted data: a string
	// for a single-valued widget (nil when the name is absent), []string for a
	// multiple-valued one, a bool for a checkbox.
	ValueFromData(data url.Values, name string) any
	// ValueOmittedFromData reports whether the data holds no value for the
	// widget. A widget that browsers leave out when unset returns false.
	ValueOmittedFromData(data url.Values, name string) bool
	// IDForLabel returns the id a <label for> should point at, given the id of
	// the field; the empty string means the label gets no for attribute.
	IDForLabel(id string) string
	// UseRequiredAttribute reports whether the HTML required attribute may be
	// rendered for a required field with the given initial value.
	UseRequiredAttribute(initial any) bool
	// IsHidden reports whether the widget has no visible part.
	IsHidden() bool
	// UseFieldset reports whether the widget is a group of inputs that belongs
	// in a <fieldset> with a <legend>.
	UseFieldset() bool
	// Attributes returns the widget's own attributes.
	Attributes() Attrs
}

// attrTail lists the attributes written after all others, in this order. It
// reproduces the position Django gives them, so that the common output is
// byte-identical; the remaining attributes are written in alphabetical order.
var attrTail = []string{"required", "disabled", "aria-invalid", "aria-describedby", "id", "multiple", "checked", "selected"}

// isTailAttr reports whether name is one of attrTail.
func isTailAttr(name string) bool {
	for _, t := range attrTail {
		if t == name {
			return true
		}
	}
	return false
}

// renderAttrs returns attrs as HTML attributes, each preceded by a space.
func renderAttrs(attrs Attrs) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		if !isTailAttr(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range attrTail {
		if _, ok := attrs[k]; ok {
			keys = append(keys, k)
		}
	}
	var b strings.Builder
	for _, k := range keys {
		switch v := attrs[k].(type) {
		case nil:
		case bool:
			if v {
				b.WriteString(" " + k)
			}
		default:
			b.WriteString(" " + k + `="` + escape(pyString(v)) + `"`)
		}
	}
	return b.String()
}

// mergeAttrs returns a new map holding the attributes of every layer; a later
// layer overrides an earlier one.
func mergeAttrs(layers ...Attrs) Attrs {
	out := Attrs{}
	for _, layer := range layers {
		for k, v := range layer {
			out[k] = v
		}
	}
	return out
}

// lastValue returns the last value submitted under name, as Django's
// QueryDict.get does, or nil when the name is absent.
func lastValue(data url.Values, name string) any {
	if vs, ok := data[name]; ok && len(vs) > 0 {
		return vs[len(vs)-1]
	}
	return nil
}

// formatValue returns the text of a value attribute. Nil and the empty string
// have none.
func formatValue(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	s := pyString(value)
	return s, s != ""
}

// renderInput returns an <input> element.
func renderInput(inputType, name string, value any, layers ...Attrs) string {
	attrs := mergeAttrs(layers...)
	// A "type" attribute replaces the input type, as Django's Input takes it
	// from the attrs it is constructed with.
	if t, ok := attrs["type"]; ok {
		inputType = pyString(t)
		delete(attrs, "type")
	}
	var b strings.Builder
	b.WriteString(`<input type="` + escape(inputType) + `" name="` + escape(name) + `"`)
	if s, ok := formatValue(value); ok {
		b.WriteString(` value="` + escape(s) + `"`)
	}
	b.WriteString(renderAttrs(attrs))
	b.WriteString(">")
	return b.String()
}

// widgetDefaults supplies the behavior most widgets share. Every widget embeds
// it and overrides what differs.
type widgetDefaults struct{}

// ValueFromData returns the last value submitted under name.
func (widgetDefaults) ValueFromData(data url.Values, name string) any { return lastValue(data, name) }

// ValueOmittedFromData reports whether name is absent from the data.
func (widgetDefaults) ValueOmittedFromData(data url.Values, name string) bool {
	_, ok := data[name]
	return !ok
}

// IDForLabel returns id unchanged.
func (widgetDefaults) IDForLabel(id string) string { return id }

// UseRequiredAttribute allows the required attribute.
func (widgetDefaults) UseRequiredAttribute(any) bool { return true }

// IsHidden reports false.
func (widgetDefaults) IsHidden() bool { return false }

// UseFieldset reports false.
func (widgetDefaults) UseFieldset() bool { return false }

// TextInput is <input type="text">.
type TextInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// Render implements Widget.
func (w TextInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("text", name, value, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w TextInput) Attributes() Attrs { return w.Attrs }

// NumberInput is <input type="number">.
type NumberInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// Render implements Widget.
func (w NumberInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("number", name, value, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w NumberInput) Attributes() Attrs { return w.Attrs }

// EmailInput is <input type="email">.
type EmailInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// Render implements Widget.
func (w EmailInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("email", name, value, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w EmailInput) Attributes() Attrs { return w.Attrs }

// URLInput is <input type="url">.
type URLInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// Render implements Widget.
func (w URLInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("url", name, value, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w URLInput) Attributes() Attrs { return w.Attrs }

// PasswordInput is <input type="password">. The value is not rendered back
// unless RenderValue is set, so that a password never travels to the browser
// again after a failed submission.
type PasswordInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// RenderValue renders the submitted value back into the page.
	RenderValue bool
}

// Render implements Widget.
func (w PasswordInput) Render(name string, value any, attrs Attrs) string {
	if !w.RenderValue {
		value = nil
	}
	return renderInput("password", name, value, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w PasswordInput) Attributes() Attrs { return w.Attrs }

// HiddenInput is <input type="hidden">.
type HiddenInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// Render implements Widget.
func (w HiddenInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("hidden", name, value, w.Attrs, attrs)
}

// Attributes implements Widget.
func (w HiddenInput) Attributes() Attrs { return w.Attrs }

// IsHidden reports true.
func (HiddenInput) IsHidden() bool { return true }

// UseRequiredAttribute reports false: a hidden input cannot be filled in.
func (HiddenInput) UseRequiredAttribute(any) bool { return false }

// formatTemporal formats a time value with the widget's format (a Go layout or
// a strftime string); any other value is passed through.
func formatTemporal(value any, format, fallback string) any {
	t, ok := value.(time.Time)
	if !ok {
		return value
	}
	if format == "" {
		format = fallback
	}
	return t.Format(goLayout(format))
}

// DateInput is a text input showing a date, by default as 2006-01-02.
type DateInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// Format is the layout a time.Time is displayed with, a Go layout or a
	// strftime string such as "%d/%m/%Y".
	Format string
}

// Render implements Widget.
func (w DateInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("text", name, formatTemporal(value, w.Format, "2006-01-02"), w.Attrs, attrs)
}

// Attributes implements Widget.
func (w DateInput) Attributes() Attrs { return w.Attrs }

// DateTimeInput is a text input showing a date and time, by default as
// 2006-01-02 15:04:05.
type DateTimeInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// Format is the layout a time.Time is displayed with.
	Format string
}

// Render implements Widget.
func (w DateTimeInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("text", name, formatTemporal(value, w.Format, "2006-01-02 15:04:05"), w.Attrs, attrs)
}

// Attributes implements Widget.
func (w DateTimeInput) Attributes() Attrs { return w.Attrs }

// TimeInput is a text input showing a time of day, by default as 15:04:05.
type TimeInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// Format is the layout a time.Time is displayed with.
	Format string
}

// Render implements Widget.
func (w TimeInput) Render(name string, value any, attrs Attrs) string {
	return renderInput("text", name, formatTemporal(value, w.Format, "15:04:05"), w.Attrs, attrs)
}

// Attributes implements Widget.
func (w TimeInput) Attributes() Attrs { return w.Attrs }

// Textarea is <textarea>, 40 columns by 10 rows unless Attrs say otherwise.
type Textarea struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
}

// Render implements Widget.
func (w Textarea) Render(name string, value any, attrs Attrs) string {
	all := mergeAttrs(Attrs{"cols": "40", "rows": "10"}, w.Attrs, attrs)
	text, _ := formatValue(value)
	// The newline after the tag is dropped by browsers; without it a value
	// that starts with a newline would lose that newline.
	return `<textarea name="` + escape(name) + `"` + renderAttrs(all) + ">\n" + escape(text) + "</textarea>"
}

// Attributes implements Widget.
func (w Textarea) Attributes() Attrs { return w.Attrs }

// CheckboxInput is <input type="checkbox">.
type CheckboxInput struct {
	widgetDefaults
	// Attrs are the widget's HTML attributes.
	Attrs Attrs
	// CheckTest decides whether the box is checked for a value. The default
	// checks it for every value except false, nil and the empty string.
	CheckTest func(value any) bool
}

// booleanCheck is the default CheckTest.
func booleanCheck(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	}
	return true
}

// Render implements Widget.
func (w CheckboxInput) Render(name string, value any, attrs Attrs) string {
	check := w.CheckTest
	if check == nil {
		check = booleanCheck
	}
	extra := Attrs{}
	if check(value) {
		extra["checked"] = true
	}
	// A boolean has no text to send; the browser submits "on".
	if _, isBool := value.(bool); isBool {
		value = nil
	}
	return renderInput("checkbox", name, value, w.Attrs, attrs, extra)
}

// Attributes implements Widget.
func (w CheckboxInput) Attributes() Attrs { return w.Attrs }

// ValueFromData returns false when the name is absent, because a browser does
// not submit an unchecked box, and otherwise the truth of the submitted text.
func (CheckboxInput) ValueFromData(data url.Values, name string) any {
	v, ok := lastValue(data, name).(string)
	if !ok {
		return false
	}
	switch strings.ToLower(v) {
	case "true":
		return true
	case "false":
		return false
	}
	return v != ""
}

// ValueOmittedFromData reports false: an unchecked box is absent from the
// data, so absence does not mean the field was left out of the form.
func (CheckboxInput) ValueOmittedFromData(url.Values, string) bool { return false }
