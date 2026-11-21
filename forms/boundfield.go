package forms

import (
	"strings"
	"time"
)

// BoundField is a field together with the form it belongs to and that form's
// data, the counterpart of Django's BoundField. It renders the widget, the
// label and the errors of the field.
type BoundField struct {
	// Name is the name of the field in the form.
	Name string

	form  *Form
	field Field

	initial    any
	hasInitial bool
}

// Field returns the field definition.
func (bf *BoundField) Field() Field { return bf.field }

// Form returns the form the field belongs to.
func (bf *BoundField) Form() *Form { return bf.form }

// HTMLName returns the name attribute of the widget: the field name with the
// form's prefix.
func (bf *BoundField) HTMLName() string { return bf.form.AddPrefix(bf.Name) }

// AutoID returns the HTML id of the field according to the form's AutoID
// pattern, "" when ids are switched off.
func (bf *BoundField) AutoID() string {
	pattern := bf.form.AutoID
	if pattern == "" {
		return ""
	}
	return strings.ReplaceAll(pattern, "%s", bf.HTMLName())
}

// widgetID returns the id the widget is rendered with: its own id attribute or
// the automatic one.
func (bf *BoundField) widgetID() string {
	if id, ok := bf.field.Base().Widget.Attributes()["id"]; ok && id != nil {
		if s := pyString(id); s != "" {
			return s
		}
	}
	return bf.AutoID()
}

// IDForLabel returns the id a <label for> of the field points at.
func (bf *BoundField) IDForLabel() string {
	return bf.field.Base().Widget.IDForLabel(bf.widgetID())
}

// Label returns the label of the field: the configured one, or one derived
// from the name.
func (bf *BoundField) Label() string {
	if label := bf.field.Base().Label; label != nil {
		return *label
	}
	return prettyName(bf.Name)
}

// HelpText returns the help text of the field.
func (bf *BoundField) HelpText() string { return bf.field.Base().HelpText }

// IsHidden reports whether the widget of the field is hidden.
func (bf *BoundField) IsHidden() bool { return bf.field.Base().Widget.IsHidden() }

// UseFieldset reports whether the widget is a group of inputs.
func (bf *BoundField) UseFieldset() bool { return bf.field.Base().Widget.UseFieldset() }

// Errors returns the errors of the field.
func (bf *BoundField) Errors() ErrorList {
	if l, ok := bf.form.Errors().lists[bf.Name]; ok {
		return *l
	}
	return *newErrorList("", bf.AutoID())
}

// Data returns the value the widget extracted from the submitted data, nil
// for an unbound form.
func (bf *BoundField) Data() any {
	if !bf.form.bound {
		return nil
	}
	return bf.field.Base().Widget.ValueFromData(bf.form.data, bf.HTMLName())
}

// Initial returns the initial value of the field: the form's entry or the
// field's own. A func() any is called once; the result is kept, as Django's
// cached BoundField.initial.
func (bf *BoundField) Initial() any {
	if bf.hasInitial {
		return bf.initial
	}
	value, ok := bf.form.Initial[bf.Name]
	if !ok {
		value = bf.field.Base().Initial
	}
	if fn, ok := value.(func() any); ok {
		value = fn()
	}
	// A date or time widget shows whole seconds, so a fraction would make an
	// unchanged value look changed.
	if t, ok := value.(time.Time); ok && !supportsMicroseconds(bf.field.Base().Widget) {
		value = t.Truncate(time.Second)
	}
	bf.initial, bf.hasInitial = value, true
	return value
}

// Value returns the value the widget displays: the submitted data of a bound
// form, the initial value otherwise.
func (bf *BoundField) Value() any {
	data := bf.Initial()
	if bf.form.bound {
		data = bf.field.BoundData(bf.Data(), data)
	}
	return bf.field.PrepareValue(data)
}

// AriaDescribedBy returns the ids of the help text and the error list, for
// the aria-describedby attribute of the widget.
func (bf *BoundField) AriaDescribedBy() string {
	if v, ok := bf.field.Base().Widget.Attributes()["aria-describedby"]; ok && v != nil && v != "" {
		return ""
	}
	id := bf.AutoID()
	if id == "" || bf.IsHidden() {
		return ""
	}
	var ids []string
	if bf.HelpText() != "" {
		ids = append(ids, id+"_helptext")
	}
	if bf.Errors().Len() > 0 {
		ids = append(ids, id+"_error")
	}
	return strings.Join(ids, " ")
}

// widget returns the field's widget, supplied with the field's choices.
func (bf *BoundField) widget(w Widget) Widget {
	if w == nil {
		w = bf.field.Base().Widget
	}
	if cf, ok := bf.field.(choiceField); ok {
		if cw, ok := w.(ChoiceWidget); ok {
			return cw.WithChoices(cf.widgetChoices())
		}
	}
	return w
}

// AsWidget renders the field with the given widget, or with its own when w is
// nil. attrs are added to the widget's attributes.
func (bf *BoundField) AsWidget(w Widget, attrs Attrs) string {
	w = bf.widget(w)
	base := bf.field.Base()
	built := mergeAttrs(bf.field.WidgetAttrs(w), attrs)
	if w.UseRequiredAttribute(bf.Initial()) && base.Required && bf.form.UseRequiredAttribute {
		built["required"] = true
	}
	if base.Disabled {
		built["disabled"] = true
	}
	if !w.IsHidden() && bf.Errors().Len() > 0 {
		built["aria-invalid"] = "true"
	}
	if _, set := built["aria-describedby"]; !set && !w.UseFieldset() {
		if ids := bf.AriaDescribedBy(); ids != "" {
			built["aria-describedby"] = ids
		}
	}
	if id := bf.AutoID(); id != "" {
		if _, own := w.Attributes()["id"]; !own {
			if _, given := built["id"]; !given {
				built["id"] = id
			}
		}
	}
	return w.Render(bf.HTMLName(), bf.Value(), built)
}

// String renders the field with its own widget.
func (bf *BoundField) String() string { return bf.AsWidget(nil, nil) }

// HTML returns the rendered markup; templates print it without escaping.
func (bf *BoundField) HTML() string { return bf.String() }

// AsText renders the field as <input type="text">.
func (bf *BoundField) AsText(attrs Attrs) string { return bf.AsWidget(TextInput{}, attrs) }

// AsTextarea renders the field as <textarea>.
func (bf *BoundField) AsTextarea(attrs Attrs) string { return bf.AsWidget(Textarea{}, attrs) }

// AsHidden renders the field as <input type="hidden">, one input per value
// for a field with several values.
func (bf *BoundField) AsHidden(attrs Attrs) string {
	switch bf.field.(type) {
	case *MultipleChoiceField, *TypedMultipleChoiceField, *ModelMultipleChoiceField:
		return bf.AsWidget(MultipleHiddenInput{}, attrs)
	}
	return bf.AsWidget(HiddenInput{}, attrs)
}

// LabelTag returns the label of the field wrapped in a <label> tag that points
// at the widget, with the label suffix appended. Without an id the bare label
// text is returned.
func (bf *BoundField) LabelTag() string { return bf.LabelTagWith("", nil, nil, "label") }

// LegendTag returns the label wrapped in a <legend> tag, for a widget that is
// rendered in a <fieldset>.
func (bf *BoundField) LegendTag() string { return bf.LabelTagWith("", nil, nil, "legend") }

// LabelTagWith is LabelTag with every part overridable: contents replaces the
// label text when not empty, attrs are added to the tag, suffix replaces the
// label suffix when not nil, and tag names the element ("label" when empty).
// The contents are text and are escaped.
func (bf *BoundField) LabelTagWith(contents string, attrs Attrs, suffix *string, tag string) string {
	if contents == "" {
		contents = bf.Label()
	}
	contents = escape(contents)
	labelSuffix := bf.form.LabelSuffix
	if s := bf.field.Base().LabelSuffix; s != nil {
		labelSuffix = *s
	}
	if suffix != nil {
		labelSuffix = *suffix
	}
	// A label that already ends in punctuation gets no suffix.
	if labelSuffix != "" && contents != "" && !strings.ContainsRune(":?.!", lastRune(contents)) {
		contents += escape(labelSuffix)
	}
	id := bf.widgetID()
	if id == "" {
		return contents
	}
	tagAttrs := mergeAttrs(attrs)
	if forID := bf.field.Base().Widget.IDForLabel(id); forID != "" && tag != "legend" {
		tagAttrs["for"] = forID
	}
	if bf.field.Base().Required && bf.form.RequiredCSSClass != "" {
		if class, ok := tagAttrs["class"]; ok && class != nil {
			tagAttrs["class"] = pyString(class) + " " + bf.form.RequiredCSSClass
		} else {
			tagAttrs["class"] = bf.form.RequiredCSSClass
		}
	}
	if tag == "" {
		tag = "label"
	}
	return "<" + tag + renderAttrs(tagAttrs) + ">" + contents + "</" + tag + ">"
}

// lastRune returns the last character of s.
func lastRune(s string) rune {
	r := []rune(s)
	return r[len(r)-1]
}

// CSSClasses returns the CSS classes of the field's row: the extra ones, the
// form's error class when the field has errors, and its required class when
// the field is required.
func (bf *BoundField) CSSClasses(extra ...string) string {
	var classes []string
	seen := map[string]bool{}
	add := func(c string) {
		if c != "" && !seen[c] {
			seen[c] = true
			classes = append(classes, c)
		}
	}
	for _, e := range extra {
		for _, c := range strings.Fields(e) {
			add(c)
		}
	}
	if bf.form.ErrorCSSClass != "" && bf.Errors().Len() > 0 {
		add(bf.form.ErrorCSSClass)
	}
	if bf.form.RequiredCSSClass != "" && bf.field.Base().Required {
		add(bf.form.RequiredCSSClass)
	}
	return strings.Join(classes, " ")
}
