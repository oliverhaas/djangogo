package orm

import (
	"testing"
)

// Author is a standard model with an auto-int primary key.
type Author struct {
	ID    int64
	Name  string `orm:"max_length=100"`
	Email string `orm:"unique"`
}

// Tag uses a string pk tag (non-integer pk).
type Tag struct {
	Slug  string `orm:"pk;max_length=50"`
	Label string
}

// Custom overrides the table name via Meta().
type Custom struct {
	ID   int64
	Name string
}

func (Custom) Meta() Meta { return Meta{Table: "custom_things"} }

// helpers -------------------------------------------------------------------

func mustRegister(t *testing.T, r *Registry, model any) *Model {
	t.Helper()
	m, err := r.Register(model)
	if err != nil {
		t.Fatalf("Register(%T) unexpected error: %v", model, err)
	}
	return m
}

// TestModelAuthor_BasicProperties checks Name, Table, PrimaryKey and Columns.
func TestModelAuthor_BasicProperties(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	m := mustRegister(t, r, &Author{})

	if m.Name() != "Author" {
		t.Errorf("Name: got %q, want %q", m.Name(), "Author")
	}
	if m.Table() != "author" {
		t.Errorf("Table: got %q, want %q", m.Table(), "author")
	}
	pk := m.PrimaryKey()
	if pk == nil {
		t.Fatal("PrimaryKey is nil")
	}
	if pk.Name != "ID" {
		t.Errorf("PrimaryKey.Name: got %q, want %q", pk.Name, "ID")
	}
	if pk.Kind != KindAuto {
		t.Errorf("PrimaryKey.Kind: got %v, want KindAuto", pk.Kind)
	}

	want := []string{"id", "name", "email"}
	got := m.Columns()
	if len(got) != len(want) {
		t.Fatalf("Columns len: got %d, want %d", len(got), len(want))
	}
	for i, c := range want {
		if got[i] != c {
			t.Errorf("Columns[%d]: got %q, want %q", i, got[i], c)
		}
	}
}

// TestModelAuthor_FieldsReturnsCopy verifies that mutating the returned slice
// does not affect a second call.
func TestModelAuthor_FieldsReturnsCopy(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	m := mustRegister(t, r, &Author{})

	first := m.Fields()
	first[0] = nil // clobber first element

	second := m.Fields()
	if second[0] == nil {
		t.Error("Fields() did not return a copy: mutation of first slice affected second call")
	}
}

// TestModelAuthor_FieldByName checks FieldByName for a known and unknown field.
func TestModelAuthor_FieldByName(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	m := mustRegister(t, r, &Author{})

	f, ok := m.FieldByName("Name")
	if !ok {
		t.Fatal("FieldByName(\"Name\") returned ok=false")
	}
	if f.Name != "Name" {
		t.Errorf("field.Name: got %q, want %q", f.Name, "Name")
	}

	_, ok = m.FieldByName("Missing")
	if ok {
		t.Error("FieldByName(\"Missing\") should return ok=false")
	}
}

// TestModelTag checks that a string pk tag keeps KindChar (not promoted to KindAuto).
func TestModelTag(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	m := mustRegister(t, r, &Tag{})

	if m.Table() != "tag" {
		t.Errorf("Table: got %q, want %q", m.Table(), "tag")
	}
	pk := m.PrimaryKey()
	if pk == nil {
		t.Fatal("PrimaryKey is nil")
	}
	if pk.Name != "Slug" {
		t.Errorf("PrimaryKey.Name: got %q, want %q", pk.Name, "Slug")
	}
	if pk.Kind != KindChar {
		t.Errorf("PrimaryKey.Kind: got %v, want KindChar (non-integer pk must not be promoted)", pk.Kind)
	}
}

// TestModelCustomTable checks that Meta().Table overrides the default.
func TestModelCustomTable(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	m := mustRegister(t, r, &Custom{})

	if m.Table() != "custom_things" {
		t.Errorf("Table: got %q, want %q", m.Table(), "custom_things")
	}
}

// TestModelGoType checks that GoType returns the struct type (not a pointer).
func TestModelGoType(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	m := mustRegister(t, r, &Author{})

	gt := m.GoType()
	if gt.Kind().String() != "struct" {
		t.Errorf("GoType should be a struct, got %v", gt.Kind())
	}
	if gt.Name() != "Author" {
		t.Errorf("GoType.Name: got %q, want %q", gt.Name(), "Author")
	}
}
