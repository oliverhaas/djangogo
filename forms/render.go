package forms

import "strings"

// The layouts below produce the HTML of Django's form templates
// (django/forms/templates/django/forms/{div,p,table,ul,field}.html). Line
// breaks and indentation between tags differ; they carry no meaning in HTML.

// renderParts splits the form into what a layout needs: the errors shown on
// top (the form-level ones and those of hidden fields), the visible fields and
// the hidden fields.
func (f *Form) renderParts() (top ErrorList, visible, hidden []*BoundField) {
	nonField := f.NonFieldErrors()
	top = *newErrorList("nonfield", "")
	top.data = append(top.data, nonField.data...)
	for _, bf := range f.Fields() {
		if !bf.IsHidden() {
			visible = append(visible, bf)
			continue
		}
		hidden = append(hidden, bf)
		for _, msg := range bf.Errors().Messages() {
			top.data = append(top.data, NewValidationError("(Hidden field %(name)s) %(error)s", "",
				map[string]any{"name": bf.Name, "error": msg}))
		}
	}
	return top, visible, hidden
}

// TopErrors returns the errors a layout shows above the fields: the
// form-level errors and the errors of hidden fields.
func (f *Form) TopErrors() ErrorList {
	top, _, _ := f.renderParts()
	return top
}

// hiddenHTML returns the widgets of the hidden fields.
func hiddenHTML(hidden []*BoundField) string {
	var b strings.Builder
	for _, bf := range hidden {
		b.WriteString(bf.String())
	}
	return b.String()
}

// classAttr returns a class attribute for the row of a field, or "".
func classAttr(bf *BoundField) string {
	if classes := bf.CSSClasses(); classes != "" {
		return ` class="` + escape(classes) + `"`
	}
	return ""
}

// helpHTML returns the help text of a field in the given element, or "". The
// text is not escaped, so that it can hold markup, as in Django.
func helpHTML(bf *BoundField, tag string) string {
	help := bf.HelpText()
	if help == "" {
		return ""
	}
	id := ""
	if auto := bf.AutoID(); auto != "" {
		id = ` id="` + escape(auto) + `_helptext"`
	}
	return "<" + tag + ` class="helptext"` + id + ">" + help + "</" + tag + ">"
}

// labelHTML returns the label tag of a field, or "" for an empty label.
func labelHTML(bf *BoundField) string {
	if bf.Label() == "" {
		return ""
	}
	return bf.LabelTag()
}

// joinLines joins the non-empty parts with newlines.
func joinLines(parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}

// AsFieldGroup renders the label, help text, errors and widget of the field,
// Django's as_field_group. A grouped widget is wrapped in a <fieldset>.
func (bf *BoundField) AsFieldGroup() string {
	if !bf.UseFieldset() {
		return joinLines(labelHTML(bf), helpHTML(bf, "div"), bf.Errors().AsUL(), bf.String())
	}
	open := "<fieldset"
	if ids := bf.AriaDescribedBy(); ids != "" {
		open += ` aria-describedby="` + escape(ids) + `"`
	}
	open += ">"
	legend := ""
	if bf.Label() != "" {
		legend = bf.LegendTag()
	}
	return joinLines(open, legend, helpHTML(bf, "div"), bf.Errors().AsUL(), bf.String()+"</fieldset>")
}

// AsDiv renders the form as a sequence of <div> elements, one per field. It
// is the default layout.
func (f *Form) AsDiv() string {
	top, visible, hidden := f.renderParts()
	parts := []string{top.AsUL()}
	if top.Len() > 0 && len(visible) == 0 {
		parts = append(parts, "<div>"+hiddenHTML(hidden)+"</div>")
	}
	for i, bf := range visible {
		tail := ""
		if i == len(visible)-1 {
			tail = hiddenHTML(hidden)
		}
		parts = append(parts, joinLines("<div"+classAttr(bf)+">", bf.AsFieldGroup(), tail, "</div>"))
	}
	if len(visible) == 0 && top.Len() == 0 {
		parts = append(parts, hiddenHTML(hidden))
	}
	return joinLines(parts...)
}

// AsP renders the form as a sequence of <p> elements, one per field.
func (f *Form) AsP() string {
	top, visible, hidden := f.renderParts()
	parts := []string{top.AsUL()}
	if top.Len() > 0 && len(visible) == 0 {
		parts = append(parts, "<p>"+hiddenHTML(hidden)+"</p>")
	}
	for i, bf := range visible {
		tail := ""
		if i == len(visible)-1 {
			tail = hiddenHTML(hidden)
		}
		parts = append(parts, bf.Errors().AsUL(),
			joinLines("<p"+classAttr(bf)+">", labelHTML(bf), bf.String(), helpHTML(bf, "span"), tail, "</p>"))
	}
	if len(visible) == 0 && top.Len() == 0 {
		parts = append(parts, hiddenHTML(hidden))
	}
	return joinLines(parts...)
}

// AsTable renders the form as table rows, one per field, without the
// surrounding <table> tag.
func (f *Form) AsTable() string {
	top, visible, hidden := f.renderParts()
	var parts []string
	if top.Len() > 0 {
		inner := top.AsUL()
		if len(visible) == 0 {
			inner = joinLines(inner, hiddenHTML(hidden))
		}
		parts = append(parts, joinLines("<tr>", `<td colspan="2">`, inner, "</td>", "</tr>"))
	}
	for i, bf := range visible {
		tail := ""
		if i == len(visible)-1 {
			tail = hiddenHTML(hidden)
		}
		help := ""
		if h := helpHTML(bf, "span"); h != "" {
			help = "<br>\n" + h
		}
		parts = append(parts, joinLines("<tr"+classAttr(bf)+">", "<th>"+labelHTML(bf)+"</th>",
			"<td>", bf.Errors().AsUL(), bf.String(), help, tail, "</td>", "</tr>"))
	}
	if len(visible) == 0 && top.Len() == 0 {
		parts = append(parts, hiddenHTML(hidden))
	}
	return joinLines(parts...)
}

// AsUL renders the form as list items, one per field, without the surrounding
// <ul> tag.
func (f *Form) AsUL() string {
	top, visible, hidden := f.renderParts()
	var parts []string
	if top.Len() > 0 {
		inner := top.AsUL()
		if len(visible) == 0 {
			inner = joinLines(inner, hiddenHTML(hidden))
		}
		parts = append(parts, joinLines("<li>", inner, "</li>"))
	}
	for i, bf := range visible {
		tail := ""
		if i == len(visible)-1 {
			tail = hiddenHTML(hidden)
		}
		parts = append(parts, joinLines("<li"+classAttr(bf)+">", bf.Errors().AsUL(), labelHTML(bf),
			bf.String(), helpHTML(bf, "span"), tail, "</li>"))
	}
	if len(visible) == 0 && top.Len() == 0 {
		parts = append(parts, hiddenHTML(hidden))
	}
	return joinLines(parts...)
}
