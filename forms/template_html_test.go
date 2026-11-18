package forms_test

import (
	"fmt"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// htmler is the interface of templates.HTMLer, Django's __html__ protocol: a
// template prints what HTML returns without escaping it.
type htmler interface {
	HTML() string
	fmt.Stringer
}

// Every value that renders markup gives the templates its HTML through the
// HTML method, the same text as String, so a template does not escape it.
func TestTemplateValuesRenderHTML(t *testing.T) {
	form := forms.New(forms.F("name", forms.NewCharField())).Bind(vals("name", "<b>"))
	form.AddError("", "Wrong & <bad>.")
	tmpl, _ := form.TemplateValue().(forms.TemplateForm)
	field, _ := tmpl["name"].(forms.TemplateField)
	errs, _ := form.Errors().TemplateValue().(forms.TemplateErrorDict)
	nonField, _ := form.NonFieldErrors().TemplateValue().(forms.TemplateErrors)
	values := map[string]any{
		"Form":              form,
		"BoundField":        form.Field("name"),
		"ErrorList":         form.NonFieldErrors(),
		"ErrorDict":         form.Errors(),
		"SafeHTML":          forms.SafeHTML("<p>"),
		"TemplateForm":      tmpl,
		"TemplateField":     field,
		"TemplateErrorDict": errs,
		"TemplateErrors":    nonField,
		"TemplateError":     nonField[0],
	}
	for name, v := range values {
		h, ok := v.(htmler)
		if !ok {
			t.Errorf("%s has no HTML method", name)
			continue
		}
		if h.HTML() != h.String() {
			t.Errorf("%s: HTML() = %q, String() = %q", name, h.HTML(), h.String())
		}
	}
	if got, want := nonField[0].HTML(), "Wrong &amp; &lt;bad&gt;."; got != want {
		t.Errorf("TemplateError.HTML() = %q, want %q", got, want)
	}
}
