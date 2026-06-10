package forms

import (
	"strings"
	"testing"
)

func TestWithFieldsSelectsAndOrders(t *testing.T) {
	t.Parallel()
	f := FromModel(articleModel(t), WithFields("Body", "Title"))
	names := fieldNames(f)
	want := []string{"Body", "Title"}
	if len(names) != len(want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("field %d = %q, want %q (order must follow WithFields)", i, names[i], want[i])
		}
	}
}

func TestWithFieldsIgnoresUnknown(t *testing.T) {
	t.Parallel()
	f := FromModel(articleModel(t), WithFields("Title", "Nonexistent", "ID"))
	// "Nonexistent" is dropped; "ID" is the skipped auto primary key.
	names := fieldNames(f)
	if len(names) != 1 || names[0] != "Title" {
		t.Errorf("fields = %v, want [Title]", names)
	}
}

func TestWithExcludeDropsFields(t *testing.T) {
	t.Parallel()
	f := FromModel(articleModel(t), WithExclude("Body", "Published"))
	names := fieldNames(f)
	for _, n := range names {
		if n == "Body" || n == "Published" {
			t.Errorf("excluded field %q still present: %v", n, names)
		}
	}
	if !contains(names, "Title") {
		t.Errorf("non-excluded field Title missing: %v", names)
	}
}

func TestExcludeWinsOverInclude(t *testing.T) {
	t.Parallel()
	f := FromModel(articleModel(t), WithFields("Title", "Body"), WithExclude("Body"))
	names := fieldNames(f)
	if len(names) != 1 || names[0] != "Title" {
		t.Errorf("fields = %v, want [Title] (exclude must win over include)", names)
	}
}

func TestWithLabelWidgetHelpRequired(t *testing.T) {
	t.Parallel()
	f := FromModel(articleModel(t),
		WithLabel("Title", "Headline"),
		WithWidget("Title", Textarea{}),
		WithHelpText("Title", "Keep it short."),
		WithRequired("Title", false),
	)
	title := fieldNamed(f, "Title")
	if title == nil {
		t.Fatal("Title field missing")
	}
	if title.Label != "Headline" {
		t.Errorf("Label = %q, want %q", title.Label, "Headline")
	}
	if _, ok := title.Widget.(Textarea); !ok {
		t.Errorf("Widget = %T, want Textarea", title.Widget)
	}
	if title.HelpText != "Keep it short." {
		t.Errorf("HelpText = %q, want %q", title.HelpText, "Keep it short.")
	}
	if title.Required {
		t.Error("Required = true, want false after WithRequired(false)")
	}
}

func TestFromStructHonorsExclude(t *testing.T) {
	t.Parallel()
	m := articleModel(t)
	a := &Article{ID: 3, Title: "Hi", Body: "Words", Views: 9, Published: true}
	f := FromStruct(m, a, WithExclude("Body"))
	if contains(fieldNames(f), "Body") {
		t.Errorf("Body should be excluded from the form: %v", fieldNames(f))
	}
	if f.BoundValue("Title") != "Hi" {
		t.Errorf("Title bound value = %q, want Hi", f.BoundValue("Title"))
	}
	// Excluded fields are not bound.
	if v := f.BoundValue("Body"); v != "" {
		t.Errorf("Body bound value = %q, want empty (excluded)", v)
	}
}

func TestWithFieldsRendersOnlyChosen(t *testing.T) {
	t.Parallel()
	html := FromModel(articleModel(t), WithFields("Title")).Render()
	if !strings.Contains(html, `name="Title"`) {
		t.Errorf("rendered form missing Title:\n%s", html)
	}
	if strings.Contains(html, `name="Body"`) {
		t.Errorf("rendered form should not contain excluded Body:\n%s", html)
	}
}

// fieldNames returns the form's field names in order.
func fieldNames(f *Form) []string {
	out := make([]string, 0, len(f.Fields()))
	for _, fld := range f.Fields() {
		out = append(out, fld.Name)
	}
	return out
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
