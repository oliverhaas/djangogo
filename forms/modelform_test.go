package forms

import (
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

// Article is a model with a mix of field kinds for ModelForm derivation tests.
type Article struct {
	ID        int64
	Title     string `orm:"max_length=200"`
	Body      string `orm:"type=text"`
	Views     int64
	Published bool
}

// Author is the target of the FK relation used in PopulateStruct FK tests.
type Author struct {
	ID   int64
	Name string
}

// Comment carries an FK to Author for the FK PopulateStruct test.
type Comment struct {
	ID     int64
	Text   string `orm:"max_length=500"`
	Author orm.FK[Author]
}

func articleModel(t *testing.T) *orm.Model {
	t.Helper()
	r := orm.NewRegistry()
	m, err := r.Register(&Article{})
	if err != nil {
		t.Fatalf("Register(Article): %v", err)
	}
	return m
}

func commentModel(t *testing.T) *orm.Model {
	t.Helper()
	r := orm.NewRegistry()
	if _, err := r.Register(&Author{}); err != nil {
		t.Fatalf("Register(Author): %v", err)
	}
	m, err := r.Register(&Comment{})
	if err != nil {
		t.Fatalf("Register(Comment): %v", err)
	}
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return m
}

func TestFromModel_Fields(t *testing.T) {
	t.Parallel()
	f := FromModel(articleModel(t))
	byName := map[string]*Field{}
	for _, fld := range f.Fields() {
		byName[fld.Name] = fld
	}

	if _, ok := byName["ID"]; ok {
		t.Error("auto PK should be skipped")
	}

	title := byName["Title"]
	if title == nil || title.Kind != CharField {
		t.Fatalf("Title should be CharField, got %+v", title)
	}
	if title.MaxLength != 200 {
		t.Errorf("Title MaxLength: got %d, want 200", title.MaxLength)
	}
	if _, ok := title.Widget.(TextInput); !ok {
		t.Errorf("Title widget: got %T, want TextInput", title.Widget)
	}

	body := byName["Body"]
	if body == nil || body.Kind != TextField {
		t.Fatalf("Body should be TextField, got %+v", body)
	}
	if _, ok := body.Widget.(Textarea); !ok {
		t.Errorf("Body widget: got %T, want Textarea", body.Widget)
	}

	views := byName["Views"]
	if views == nil || views.Kind != IntegerField {
		t.Fatalf("Views should be IntegerField, got %+v", views)
	}
	if _, ok := views.Widget.(NumberInput); !ok {
		t.Errorf("Views widget: got %T, want NumberInput", views.Widget)
	}

	published := byName["Published"]
	if published == nil || published.Kind != BoolField {
		t.Fatalf("Published should be BoolField, got %+v", published)
	}
	if _, ok := published.Widget.(CheckboxInput); !ok {
		t.Errorf("Published widget: got %T, want CheckboxInput", published.Widget)
	}
}

func TestFromModel_FKField(t *testing.T) {
	t.Parallel()
	f := FromModel(commentModel(t))
	var author *Field
	for _, fld := range f.Fields() {
		if fld.Name == "Author" {
			author = fld
		}
	}
	if author == nil {
		t.Fatal("Author FK field missing")
	}
	if author.Kind != ChoiceField {
		t.Errorf("Author kind: got %v, want ChoiceField", author.Kind)
	}
	if _, ok := author.Widget.(Select); !ok {
		t.Errorf("Author widget: got %T, want Select", author.Widget)
	}
}

func TestSetChoicesFillsFKSelect(t *testing.T) {
	t.Parallel()
	f := FromModel(commentModel(t))
	f.SetChoices("Author", [][2]string{{"1", "Ada"}, {"2", "Linus"}})

	var author *Field
	for _, fld := range f.Fields() {
		if fld.Name == "Author" {
			author = fld
		}
	}
	if author == nil {
		t.Fatal("Author FK field missing")
	}
	if len(author.Choices) != 2 || author.Choices[0][1] != "Ada" {
		t.Errorf("Choices = %v, want Ada/Linus pairs", author.Choices)
	}
	html := f.Render()
	if !strings.Contains(html, `value="1"`) || !strings.Contains(html, "Ada") {
		t.Errorf("rendered FK <select> missing populated options:\n%s", html)
	}
}

func TestFKChoiceValidation(t *testing.T) {
	t.Parallel()
	f := FromModel(commentModel(t))
	f.SetChoices("Author", [][2]string{{"1", "Ada"}})

	if f.Bind(url.Values{"Text": {"Hi"}, "Author": {"99"}}).IsValid() {
		t.Error("FK pk absent from the option set should be invalid")
	}

	g := FromModel(commentModel(t))
	g.SetChoices("Author", [][2]string{{"1", "Ada"}})
	if !g.Bind(url.Values{"Text": {"Hi"}, "Author": {"1"}}).IsValid() {
		t.Errorf("FK pk in the option set should be valid; errors: %v", g.Errors())
	}
}

func TestPopulateStruct_FKString(t *testing.T) {
	t.Parallel()
	m := commentModel(t)
	cleaned := map[string]any{
		"Text":   "Nice",
		"Author": "42", // ChoiceField <select> yields a string value
	}
	var dest Comment
	if err := PopulateStruct(m, cleaned, &dest); err != nil {
		t.Fatalf("PopulateStruct: %v", err)
	}
	if dest.Author.PK() != 42 {
		t.Errorf("Author FK pk: got %d, want 42", dest.Author.PK())
	}
}

func TestFromStruct_Prefill(t *testing.T) {
	t.Parallel()
	m := articleModel(t)
	a := &Article{ID: 9, Title: "Hi", Body: "Words", Views: 5, Published: true}
	f := FromStruct(m, a)
	if f.BoundValue("Title") != "Hi" {
		t.Errorf("Title bound value: got %q", f.BoundValue("Title"))
	}
	if f.BoundValue("Views") != "5" {
		t.Errorf("Views bound value: got %q", f.BoundValue("Views"))
	}
	if f.BoundValue("Published") != "true" {
		t.Errorf("Published bound value: got %q", f.BoundValue("Published"))
	}
}

func TestPopulateStruct(t *testing.T) {
	t.Parallel()
	m := articleModel(t)
	cleaned := map[string]any{
		"Title":     "New",
		"Body":      "Content",
		"Views":     int64(11),
		"Published": true,
	}
	var dest Article
	if err := PopulateStruct(m, cleaned, &dest); err != nil {
		t.Fatalf("PopulateStruct: %v", err)
	}
	if dest.Title != "New" || dest.Body != "Content" || dest.Views != 11 || !dest.Published {
		t.Errorf("PopulateStruct result: %+v", dest)
	}
}

func TestPopulateStruct_FK(t *testing.T) {
	t.Parallel()
	m := commentModel(t)
	cleaned := map[string]any{
		"Text":   "Nice",
		"Author": int64(42),
	}
	var dest Comment
	if err := PopulateStruct(m, cleaned, &dest); err != nil {
		t.Fatalf("PopulateStruct: %v", err)
	}
	if dest.Text != "Nice" {
		t.Errorf("Text: got %q", dest.Text)
	}
	if dest.Author.PK() != 42 {
		t.Errorf("Author FK pk: got %d, want 42", dest.Author.PK())
	}
}

func TestHumanize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"Title", "Title"},
		{"CreatedAt", "Created at"},
		{"FirstName", "First name"},
		{"ID", "ID"},
		{"URL", "URL"},
		{"PublishedAt", "Published at"},
		{"Views", "Views"},
		{"IsActive", "Is active"},
	}
	for _, tc := range cases {
		got := humanize(tc.input)
		if got != tc.want {
			t.Errorf("humanize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
