package migrations

import (
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

// ---------------------------------------------------------------------------
// Test models -- registered fresh in each test via a local registry.
// ---------------------------------------------------------------------------

type Author struct {
	ID    int64
	Name  string `orm:"max_length=100"`
	Email string `orm:"unique"`
}

type Post struct {
	ID    int64
	Title string `orm:"max_length=200"`
	Body  string `orm:"type=text"`
}

func newTestRegistry(t *testing.T) *orm.Registry {
	t.Helper()
	r := orm.NewRegistry()
	if _, err := r.Register(&Author{}); err != nil {
		t.Fatalf("Register(Author): %v", err)
	}
	if _, err := r.Register(&Post{}); err != nil {
		t.Fatalf("Register(Post): %v", err)
	}
	return r
}

// ---------------------------------------------------------------------------
// StateFromRegistry
// ---------------------------------------------------------------------------

func TestStateFromRegistry_ModelNames(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	ps := StateFromRegistry(r)

	if len(ps.Models) != 2 {
		t.Fatalf("Models len: got %d, want 2", len(ps.Models))
	}
	if _, ok := ps.Models["Author"]; !ok {
		t.Error("Models[\"Author\"] missing")
	}
	if _, ok := ps.Models["Post"]; !ok {
		t.Error("Models[\"Post\"] missing")
	}
}

func TestStateFromRegistry_Order(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	ps := StateFromRegistry(r)

	if len(ps.Order) != 2 {
		t.Fatalf("Order len: got %d, want 2", len(ps.Order))
	}
	if ps.Order[0] != "Author" {
		t.Errorf("Order[0] = %q, want %q", ps.Order[0], "Author")
	}
	if ps.Order[1] != "Post" {
		t.Errorf("Order[1] = %q, want %q", ps.Order[1], "Post")
	}
}

func TestStateFromRegistry_AuthorModelState(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	ps := StateFromRegistry(r)

	ms := ps.Models["Author"]
	if ms.Name != "Author" {
		t.Errorf("Name = %q, want %q", ms.Name, "Author")
	}
	if ms.Table != "author" {
		t.Errorf("Table = %q, want %q", ms.Table, "author")
	}
	if len(ms.Fields) != 3 {
		t.Fatalf("Fields len: got %d, want 3 (ID, Name, Email)", len(ms.Fields))
	}

	// ID field -- auto pk
	id := ms.Fields[0]
	if id.Name != "ID" {
		t.Errorf("Fields[0].Name = %q, want %q", id.Name, "ID")
	}
	if id.Column != "id" {
		t.Errorf("Fields[0].Column = %q, want %q", id.Column, "id")
	}
	if id.Kind != orm.KindAuto {
		t.Errorf("Fields[0].Kind = %v, want KindAuto", id.Kind)
	}
	if !id.PrimaryKey {
		t.Error("Fields[0].PrimaryKey should be true")
	}

	// Name field -- char with max_length=100
	name := ms.Fields[1]
	if name.Name != "Name" {
		t.Errorf("Fields[1].Name = %q, want %q", name.Name, "Name")
	}
	if name.Column != "name" {
		t.Errorf("Fields[1].Column = %q, want %q", name.Column, "name")
	}
	if name.Kind != orm.KindChar {
		t.Errorf("Fields[1].Kind = %v, want KindChar", name.Kind)
	}
	if name.MaxLength != 100 {
		t.Errorf("Fields[1].MaxLength = %d, want 100", name.MaxLength)
	}
	if name.PrimaryKey {
		t.Error("Fields[1].PrimaryKey should be false")
	}

	// Email field -- unique char
	email := ms.Fields[2]
	if email.Name != "Email" {
		t.Errorf("Fields[2].Name = %q, want %q", email.Name, "Email")
	}
	if !email.Unique {
		t.Error("Fields[2].Unique should be true")
	}
}

func TestStateFromRegistry_PostModelState(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	ps := StateFromRegistry(r)

	ms := ps.Models["Post"]
	if ms.Table != "post" {
		t.Errorf("Table = %q, want %q", ms.Table, "post")
	}
	if len(ms.Fields) != 3 {
		t.Fatalf("Fields len: got %d, want 3 (ID, Title, Body)", len(ms.Fields))
	}

	// Title: char max_length=200
	title := ms.Fields[1]
	if title.Name != "Title" {
		t.Errorf("Fields[1].Name = %q, want %q", title.Name, "Title")
	}
	if title.Kind != orm.KindChar {
		t.Errorf("Fields[1].Kind = %v, want KindChar", title.Kind)
	}
	if title.MaxLength != 200 {
		t.Errorf("Fields[1].MaxLength = %d, want 200", title.MaxLength)
	}

	// Body: text
	body := ms.Fields[2]
	if body.Name != "Body" {
		t.Errorf("Fields[2].Name = %q, want %q", body.Name, "Body")
	}
	if body.Kind != orm.KindText {
		t.Errorf("Fields[2].Kind = %v, want KindText", body.Kind)
	}
}

// ---------------------------------------------------------------------------
// FieldState.Equal
// ---------------------------------------------------------------------------

func TestFieldState_Equal_Identical(t *testing.T) {
	t.Parallel()
	f := FieldState{
		Name:       "Name",
		Column:     "name",
		Kind:       orm.KindChar,
		PrimaryKey: false,
		Null:       false,
		Unique:     false,
		MaxLength:  100,
	}
	if !f.Equal(f) {
		t.Error("Equal(self) should be true")
	}
}

func TestFieldState_Equal_EachFieldDiffers(t *testing.T) {
	t.Parallel()
	base := FieldState{
		Name:       "Name",
		Column:     "name",
		Kind:       orm.KindChar,
		PrimaryKey: false,
		Null:       false,
		Unique:     false,
		MaxLength:  100,
	}

	cases := []struct {
		label string
		other FieldState
	}{
		{"Name", func() FieldState { f := base; f.Name = "Other"; return f }()},
		{"Column", func() FieldState { f := base; f.Column = "other"; return f }()},
		{"Kind", func() FieldState { f := base; f.Kind = orm.KindText; return f }()},
		{"PrimaryKey", func() FieldState { f := base; f.PrimaryKey = true; return f }()},
		{"Null", func() FieldState { f := base; f.Null = true; return f }()},
		{"Unique", func() FieldState { f := base; f.Unique = true; return f }()},
		{"MaxLength", func() FieldState { f := base; f.MaxLength = 200; return f }()},
	}

	for _, tc := range cases {
		if base.Equal(tc.other) {
			t.Errorf("Equal should be false when %s differs", tc.label)
		}
	}
}

// ---------------------------------------------------------------------------
// FieldByName
// ---------------------------------------------------------------------------

func TestModelState_FieldByName(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	ps := StateFromRegistry(r)
	ms := ps.Models["Author"]

	f, ok := ms.FieldByName("Name")
	if !ok {
		t.Fatal("FieldByName(\"Name\") returned ok=false")
	}
	if f.Name != "Name" {
		t.Errorf("FieldByName(\"Name\").Name = %q, want %q", f.Name, "Name")
	}

	_, ok = ms.FieldByName("Missing")
	if ok {
		t.Error("FieldByName(\"Missing\") should return ok=false")
	}
}

// ---------------------------------------------------------------------------
// Clone -- genuine deep copy
// ---------------------------------------------------------------------------

func TestProjectState_Clone_DeepCopy(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	original := StateFromRegistry(r)

	clone := original.Clone()

	// Mutate clone's Order slice.
	clone.Order[0] = "Mutated"
	if original.Order[0] == "Mutated" {
		t.Error("mutating clone.Order affected original.Order")
	}

	// Mutate clone's Models map (add a new key).
	clone.Models["Extra"] = &ModelState{Name: "Extra", Table: "extra"}
	if _, exists := original.Models["Extra"]; exists {
		t.Error("adding key to clone.Models affected original.Models")
	}

	// Mutate a cloned ModelState's Fields slice.
	clonedAuthor := clone.Models["Author"]
	clonedAuthor.Fields[0] = FieldState{Name: "Replaced"}
	origAuthor := original.Models["Author"]
	if origAuthor.Fields[0].Name == "Replaced" {
		t.Error("mutating clone's Fields slice affected original's Fields")
	}

	// Mutate the Table string on the clone's ModelState.
	clonedAuthor.Table = "mutated_table"
	if original.Models["Author"].Table == "mutated_table" {
		t.Error("mutating clone ModelState.Table affected original")
	}
}

// ---------------------------------------------------------------------------
// NewProjectState
// ---------------------------------------------------------------------------

func TestNewProjectState(t *testing.T) {
	t.Parallel()
	ps := NewProjectState()
	if ps == nil {
		t.Fatal("NewProjectState() returned nil")
	}
	if ps.Models == nil {
		t.Error("NewProjectState().Models is nil, want empty map")
	}
	if len(ps.Models) != 0 {
		t.Errorf("NewProjectState().Models len = %d, want 0", len(ps.Models))
	}
	if len(ps.Order) != 0 {
		t.Errorf("NewProjectState().Order should be empty, got %v", ps.Order)
	}
}
