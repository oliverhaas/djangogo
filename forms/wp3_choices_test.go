package forms

import (
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

// statusModel declares choices on its Status field.
type statusModel struct {
	ID     int64
	Title  string `orm:"max_length=100"`
	Status string `orm:"max_length=20"`
}

func (statusModel) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{
		"Status": {{Value: "draft", Label: "Draft"}, {Value: "published", Label: "Published"}},
	}
}

func statusModelMeta(t *testing.T) *orm.Model {
	t.Helper()
	r := orm.NewRegistry()
	m, err := r.Register(&statusModel{})
	if err != nil {
		t.Fatalf("Register(statusModel): %v", err)
	}
	return m
}

func fieldNamed(f *Form, name string) *Field {
	for _, fld := range f.Fields() {
		if fld.Name == name {
			return fld
		}
	}
	return nil
}

func TestFormFieldForCharWithChoices(t *testing.T) {
	f := FromModel(statusModelMeta(t))
	status := fieldNamed(f, "Status")
	if status == nil {
		t.Fatal("Status field missing from form")
	}
	if status.Kind != ChoiceField {
		t.Errorf("Kind = %v, want ChoiceField", status.Kind)
	}
	if _, ok := status.Widget.(Select); !ok {
		t.Errorf("Widget = %T, want Select", status.Widget)
	}
	if len(status.Choices) != 2 || status.Choices[0][0] != "draft" || status.Choices[0][1] != "Draft" {
		t.Errorf("Choices = %v, want draft/published pairs", status.Choices)
	}
}

func TestCharWithoutChoicesUnchanged(t *testing.T) {
	f := FromModel(statusModelMeta(t))
	title := fieldNamed(f, "Title")
	if title == nil {
		t.Fatal("Title field missing from form")
	}
	if title.Kind != CharField {
		t.Errorf("Kind = %v, want CharField", title.Kind)
	}
	if _, ok := title.Widget.(TextInput); !ok {
		t.Errorf("Widget = %T, want TextInput", title.Widget)
	}
}

func TestSelectRendersChoiceOptions(t *testing.T) {
	html := FromModel(statusModelMeta(t)).Render()
	if !strings.Contains(html, "<select") {
		t.Errorf("rendered form has no <select>:\n%s", html)
	}
	if !strings.Contains(html, `value="draft"`) || !strings.Contains(html, "Draft") {
		t.Errorf("rendered form missing the draft option:\n%s", html)
	}
}

func TestChoiceValidationRejectsBadValue(t *testing.T) {
	f := FromModel(statusModelMeta(t)).Bind(url.Values{
		"Title":  {"Hi"},
		"Status": {"bogus"},
	})
	if f.IsValid() {
		t.Error("form with an out-of-set choice should be invalid")
	}
}

func TestChoiceValidationAcceptsValidValue(t *testing.T) {
	f := FromModel(statusModelMeta(t)).Bind(url.Values{
		"Title":  {"Hi"},
		"Status": {"published"},
	})
	if !f.IsValid() {
		t.Errorf("form with a valid choice should be valid; errors: %v", f.Errors())
	}
}
