// Package forms provides HTML form rendering, field validation, a bound Form,
// and a ModelForm derived from orm models.
package forms

import (
	"html"
	"sort"
	"strings"
)

// Widget renders an HTML form control.
type Widget interface {
	Render(name, value string, attrs map[string]string) string
}

// renderAttrs renders attrs as a space-prefixed, escaped, key-sorted string so
// the output is deterministic and testable. Both keys and values are escaped.
func renderAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteByte(' ')
		b.WriteString(html.EscapeString(k))
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(attrs[k]))
		b.WriteByte('"')
	}
	return b.String()
}

// renderInput renders a single <input> element of the given type.
func renderInput(inputType, name, value string, attrs map[string]string) string {
	return `<input type="` + inputType + `" name="` + html.EscapeString(name) +
		`" value="` + html.EscapeString(value) + `"` + renderAttrs(attrs) + `>`
}

// TextInput renders a text <input>.
type TextInput struct{}

// Render renders a text input element.
func (TextInput) Render(name, value string, attrs map[string]string) string {
	return renderInput("text", name, value, attrs)
}

// Textarea renders a <textarea>.
type Textarea struct{}

// Render renders a textarea element with the escaped value as its content.
func (Textarea) Render(name, value string, attrs map[string]string) string {
	return `<textarea name="` + html.EscapeString(name) + `"` + renderAttrs(attrs) +
		`>` + html.EscapeString(value) + `</textarea>`
}

// NumberInput renders a number <input>.
type NumberInput struct{}

// Render renders a number input element.
func (NumberInput) Render(name, value string, attrs map[string]string) string {
	return renderInput("number", name, value, attrs)
}

// EmailInput renders an email <input>.
type EmailInput struct{}

// Render renders an email input element.
func (EmailInput) Render(name, value string, attrs map[string]string) string {
	return renderInput("email", name, value, attrs)
}

// PasswordInput renders a password <input>. It never echoes the submitted value.
type PasswordInput struct{}

// Render renders a password input element with an empty value.
func (PasswordInput) Render(name, _ string, attrs map[string]string) string {
	return renderInput("password", name, "", attrs)
}

// CheckboxInput renders a checkbox <input>, marked checked when the value is truthy.
type CheckboxInput struct{}

// Render renders a checkbox input element. It adds the "checked" attribute when
// value is truthy ("true", "on", or "1").
func (CheckboxInput) Render(name, value string, attrs map[string]string) string {
	checked := ""
	if isTruthy(value) {
		checked = " checked"
	}
	return `<input type="checkbox" name="` + html.EscapeString(name) + `"` +
		renderAttrs(attrs) + checked + `>`
}

// Select renders a <select> element from a list of (value, label) choices.
type Select struct {
	// Choices is the list of options as {value, label} pairs.
	Choices [][2]string
}

// Render renders a select element. The option whose value equals value is
// marked selected.
func (s Select) Render(name, value string, attrs map[string]string) string {
	var b strings.Builder
	b.WriteString(`<select name="`)
	b.WriteString(html.EscapeString(name))
	b.WriteString(`"`)
	b.WriteString(renderAttrs(attrs))
	b.WriteString(`>`)
	for _, c := range s.Choices {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(c[0]))
		b.WriteString(`"`)
		if c[0] == value {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(c[1]))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select>`)
	return b.String()
}

// isTruthy reports whether a raw form value represents boolean true.
func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "on", "1":
		return true
	default:
		return false
	}
}
