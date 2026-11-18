package forms

import (
	"reflect"
	"strings"
)

// The types of this file are what a template sees of a form. The template
// engine (pongo2) resolves {{ form.title }} only on maps and iterates only
// over slices. So a form becomes a map with Django's attribute names as keys,
// HTML is carried in a type whose HTML method returns the markup (Django's
// __html__ protocol), which the engine prints without escaping, and plain text
// stays a string and is escaped by the engine.
// views.Render converts a context value through its TemplateValue method.

// SafeHTML is HTML that a template writes without escaping, the counterpart of
// a Django SafeString. An empty SafeHTML is false in a template condition.
type SafeHTML []byte

// String returns the HTML.
func (s SafeHTML) String() string { return string(s) }

// HTML returns the rendered markup; templates print it without escaping.
func (s SafeHTML) HTML() string { return string(s) }

// TemplateError is one error message in a template. Printed, it is the
// escaped message.
type TemplateError struct {
	// Message is the unescaped message.
	Message string
	// Code is the error code.
	Code string

	class, fieldID string
}

// String returns the escaped message.
func (e TemplateError) String() string { return escape(e.Message) }

// HTML returns the rendered markup; templates print it without escaping.
func (e TemplateError) HTML() string { return e.String() }

// TemplateErrors is an error list in a template. Printed, it is Django's
// <ul class="errorlist">; iterated, it yields the messages; an empty list is
// false in a condition and prints nothing.
type TemplateErrors []TemplateError

// String returns the list as HTML.
func (l TemplateErrors) String() string {
	if len(l) == 0 {
		return ""
	}
	out := ErrorList{errorClass: l[0].class, fieldID: l[0].fieldID}
	for _, e := range l {
		out.data = append(out.data, &ValidationError{Message: e.Message, Code: e.Code})
	}
	return out.AsUL()
}

// HTML returns the rendered markup; templates print it without escaping.
func (l TemplateErrors) HTML() string { return l.String() }

// TemplateValue returns the list in the form a template uses.
func (l ErrorList) TemplateValue() any {
	out := make(TemplateErrors, len(l.data))
	for i, e := range l.data {
		out[i] = TemplateError{Message: e.format(), Code: e.Code, class: l.errorClass, fieldID: l.fieldID}
	}
	return out
}

// TemplateErrorDict is the errors of a form in a template, by field name.
// Printed, it is Django's nested error list; an empty one is false in a
// condition.
type TemplateErrorDict map[string]TemplateErrors

// String returns the errors as a nested HTML list. A map keeps no order, so
// the field names are sorted.
func (d TemplateErrorDict) String() string {
	if len(d) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul class="errorlist">`)
	for _, name := range sortedKeys(d) {
		b.WriteString("<li>" + escape(name) + d[name].String() + "</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

// HTML returns the rendered markup; templates print it without escaping.
func (d TemplateErrorDict) HTML() string { return d.String() }

// TemplateValue returns the dictionary in the form a template uses.
func (d *ErrorDict) TemplateValue() any {
	out := TemplateErrorDict{}
	if d == nil {
		return out
	}
	for _, name := range d.order {
		out[name], _ = d.lists[name].TemplateValue().(TemplateErrors)
	}
	return out
}

// TemplateField is a bound field in a template. Printed, it is the widget's
// HTML. Its keys are the attributes of Django's BoundField: name, html_name,
// label, help_text, auto_id, id_for_label, errors, value, initial, data,
// is_hidden, use_fieldset, css_classes, aria_describedby, widget_type,
// label_tag, legend_tag, as_widget, as_text, as_textarea, as_hidden,
// as_field_group, and field (required, disabled, label, help_text, initial,
// widget.attrs, widget.is_hidden).
type TemplateField map[string]any

// templateSelf is the key under which a template map keeps the object it was
// built from. It is not an identifier, so a template cannot reach it.
const templateSelf = "\x00self"

// String returns the widget's HTML.
func (t TemplateField) String() string {
	if bf, ok := t[templateSelf].(*BoundField); ok {
		return bf.String()
	}
	return ""
}

// HTML returns the rendered markup; templates print it without escaping.
func (t TemplateField) HTML() string { return t.String() }

// lazyHTML defers the rendering of a part until a template asks for it.
func lazyHTML(fn func() string) func() SafeHTML {
	return func() SafeHTML { return SafeHTML(fn()) }
}

// widgetType returns the name a template branches on, Django's widget_type:
// the lowercased widget name without a trailing "input" or "widget".
func widgetType(w Widget) string {
	name := strings.ToLower(reflect.TypeOf(w).Name())
	if s := strings.TrimSuffix(name, "widget"); s != name {
		return s
	}
	return strings.TrimSuffix(name, "input")
}

// TemplateValue returns the field in the form a template uses.
func (bf *BoundField) TemplateValue() any {
	base := bf.field.Base()
	attrs := map[string]any{}
	for k, v := range base.Widget.Attributes() {
		attrs[k] = v
	}
	return TemplateField{
		templateSelf:       bf,
		"name":             bf.Name,
		"html_name":        bf.HTMLName(),
		"label":            bf.Label(),
		"help_text":        bf.HelpText(),
		"auto_id":          bf.AutoID(),
		"id_for_label":     bf.IDForLabel(),
		"errors":           bf.Errors().TemplateValue(),
		"value":            bf.Value(),
		"initial":          bf.Initial(),
		"data":             bf.Data(),
		"is_hidden":        bf.IsHidden(),
		"use_fieldset":     bf.UseFieldset(),
		"css_classes":      bf.CSSClasses(),
		"aria_describedby": bf.AriaDescribedBy(),
		"widget_type":      widgetType(base.Widget),
		"label_tag":        lazyHTML(bf.LabelTag),
		"legend_tag":       lazyHTML(bf.LegendTag),
		"as_widget":        lazyHTML(bf.String),
		"as_text":          lazyHTML(func() string { return bf.AsText(nil) }),
		"as_textarea":      lazyHTML(func() string { return bf.AsTextarea(nil) }),
		"as_hidden":        lazyHTML(func() string { return bf.AsHidden(nil) }),
		"as_field_group":   lazyHTML(bf.AsFieldGroup),
		"field": map[string]any{
			"required":  base.Required,
			"disabled":  base.Disabled,
			"label":     bf.Label(),
			"help_text": base.HelpText,
			"initial":   base.Initial,
			"widget": map[string]any{
				"attrs":     attrs,
				"is_hidden": base.Widget.IsHidden(),
			},
		},
	}
}

// TemplateForm is a form in a template. Printed, it is the form in its default
// layout. Its keys are the field names, each a TemplateField, and the
// attributes of Django's Form: errors, non_field_errors, hidden_fields,
// visible_fields, as_div, as_p, as_table, as_ul, is_bound, is_valid,
// is_multipart, prefix and cleaned_data. A field name wins over an attribute
// of the same name, as in Django.
//
// The engine cannot iterate a map in field order, so {% for field in form %}
// is written {% for field in form.fields %}; fields lists every bound field.
type TemplateForm map[string]any

// String returns the form in its default layout.
func (t TemplateForm) String() string {
	if f, ok := t[templateSelf].(*Form); ok {
		return f.AsDiv()
	}
	return ""
}

// HTML returns the rendered markup; templates print it without escaping.
func (t TemplateForm) HTML() string { return t.String() }

// templateFields converts bound fields for a template.
func templateFields(fields []*BoundField) []TemplateField {
	out := make([]TemplateField, len(fields))
	for i, bf := range fields {
		out[i], _ = bf.TemplateValue().(TemplateField)
	}
	return out
}

// TemplateValue returns the form in the form a template uses. A bound form is
// validated by this call, as its errors are part of the value.
func (f *Form) TemplateValue() any {
	all := templateFields(f.Fields())
	var hidden, visible []TemplateField
	for _, tf := range all {
		if isHidden, _ := tf["is_hidden"].(bool); isHidden {
			hidden = append(hidden, tf)
		} else {
			visible = append(visible, tf)
		}
	}
	out := TemplateForm{
		templateSelf:       f,
		"errors":           f.Errors().TemplateValue(),
		"non_field_errors": f.NonFieldErrors().TemplateValue(),
		"fields":           all,
		"hidden_fields":    hidden,
		"visible_fields":   visible,
		"as_div":           lazyHTML(f.AsDiv),
		"as_p":             lazyHTML(f.AsP),
		"as_table":         lazyHTML(f.AsTable),
		"as_ul":            lazyHTML(f.AsUL),
		"is_bound":         f.bound,
		"is_valid":         f.IsValid,
		"is_multipart":     false,
		"prefix":           f.Prefix,
		"cleaned_data":     f.CleanedData,
	}
	for _, tf := range all {
		name, _ := tf["name"].(string)
		out[name] = tf
	}
	return out
}

// BaseForm returns the form itself. A type that embeds *Form, such as
// ModelForm, inherits the method, which lets FormOf find the form.
func (f *Form) BaseForm() *Form { return f }

// FormOf returns the form behind v: a *Form, a type embedding one, or the
// TemplateForm that a rendered template context holds in its place.
func FormOf(v any) (*Form, bool) {
	switch t := v.(type) {
	case *Form:
		return t, t != nil
	case TemplateForm:
		f, ok := t[templateSelf].(*Form)
		return f, ok
	case map[string]any:
		f, ok := t[templateSelf].(*Form)
		return f, ok
	case interface{ BaseForm() *Form }:
		f := t.BaseForm()
		return f, f != nil
	}
	return nil, false
}
