package forms

import (
	"net/url"
	"strings"
	"testing"
)

func sampleForm() *Form {
	return New(
		&Field{Name: "title", Label: "Title", Kind: CharField, Required: true, MaxLength: 10},
		&Field{Name: "views", Label: "Views", Kind: IntegerField},
		&Field{Name: "ok", Label: "Ok", Kind: BoolField, Widget: CheckboxInput{}},
	)
}

func TestForm_ValidData(t *testing.T) {
	t.Parallel()
	f := sampleForm().Bind(url.Values{
		"title": {"Hello"},
		"views": {"7"},
		"ok":    {"on"},
	})
	if !f.IsValid() {
		t.Fatalf("expected valid, errors: %v", f.Errors())
	}
	cl := f.Cleaned()
	if cl["title"] != "Hello" {
		t.Errorf("title: got %v", cl["title"])
	}
	if cl["views"] != int64(7) {
		t.Errorf("views: got %v (%T)", cl["views"], cl["views"])
	}
	if cl["ok"] != true {
		t.Errorf("ok: got %v", cl["ok"])
	}
}

func TestForm_InvalidData(t *testing.T) {
	t.Parallel()
	f := sampleForm().Bind(url.Values{
		"title": {""},
		"views": {"notanumber"},
	})
	if f.IsValid() {
		t.Fatal("expected invalid form")
	}
	errs := f.Errors()
	if len(errs["title"]) == 0 {
		t.Error("expected title required error")
	}
	if len(errs["views"]) == 0 {
		t.Error("expected views parse error")
	}
}

func TestForm_AddNonFieldError(t *testing.T) {
	t.Parallel()
	f := sampleForm().Bind(url.Values{"title": {"x"}})
	f.AddError("", "global problem")
	if !strings.Contains(f.Render(), "global problem") {
		t.Error("non-field error not rendered")
	}
	if len(f.Errors()[""]) == 0 {
		t.Error("non-field error not stored")
	}
}

func TestForm_Render(t *testing.T) {
	t.Parallel()
	f := sampleForm().Bind(url.Values{"title": {"Hi"}, "views": {"3"}})
	html := f.Render()
	if !strings.Contains(html, "<label>Title</label>") {
		t.Errorf("missing label: %s", html)
	}
	if !strings.Contains(html, `name="title"`) {
		t.Errorf("missing title input: %s", html)
	}
	if !strings.Contains(html, `value="Hi"`) {
		t.Errorf("bound value not re-shown: %s", html)
	}
	if !strings.Contains(html, `type="checkbox"`) {
		t.Errorf("checkbox widget not used: %s", html)
	}
}

func TestForm_RenderShowsErrorsAndEscapes(t *testing.T) {
	t.Parallel()
	f := New(&Field{Name: "title", Label: "T", Kind: CharField, Required: true})
	f.Bind(url.Values{"title": {""}})
	f.IsValid()
	f.AddError("", `<script>alert(1)</script>`)
	html := f.Render()
	if !strings.Contains(html, "This field is required.") {
		t.Errorf("field error not rendered: %s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("non-field error not escaped: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Errorf("expected escaped script tag: %s", html)
	}
}

func TestForm_BoundValue(t *testing.T) {
	t.Parallel()
	f := sampleForm().Bind(url.Values{"title": {"abc"}})
	if f.BoundValue("title") != "abc" {
		t.Errorf("BoundValue: got %q", f.BoundValue("title"))
	}
	if f.BoundValue("missing") != "" {
		t.Errorf("BoundValue(missing): got %q", f.BoundValue("missing"))
	}
}

func TestForm_Fields(t *testing.T) {
	t.Parallel()
	f := sampleForm()
	if len(f.Fields()) != 3 {
		t.Errorf("Fields: got %d, want 3", len(f.Fields()))
	}
}
