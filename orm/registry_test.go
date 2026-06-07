package orm

import (
	"strings"
	"testing"
)

// TestRegistry_GetAndModelOf verifies Get and ModelOf after registration.
func TestRegistry_GetAndModelOf(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	mustRegister(t, r, &Author{})

	m, ok := r.Get("Author")
	if !ok {
		t.Fatal("Get(\"Author\") returned ok=false")
	}
	if m.Name() != "Author" {
		t.Errorf("Get returned wrong model: %q", m.Name())
	}

	m2, ok := r.ModelOf(Author{})
	if !ok {
		t.Fatal("ModelOf(Author{}) returned ok=false")
	}
	if m2 != m {
		t.Error("ModelOf returned a different *Model than Get")
	}

	// Pointer form should also work.
	m3, ok := r.ModelOf(&Author{})
	if !ok {
		t.Fatal("ModelOf(&Author{}) returned ok=false")
	}
	if m3 != m {
		t.Error("ModelOf(&Author{}) returned a different *Model than Get")
	}
}

// TestRegistry_ModelOf_Unregistered checks that ModelOf returns false for unknown types.
func TestRegistry_ModelOf_Unregistered(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	type Stranger struct{ ID int64 }
	_, ok := r.ModelOf(Stranger{})
	if ok {
		t.Error("ModelOf should return false for an unregistered type")
	}
}

// TestRegistry_GetUnknown verifies Get returns false for unknown names.
func TestRegistry_GetUnknown(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, ok := r.Get("Nope")
	if ok {
		t.Error("Get(\"Nope\") should return false")
	}
}

// TestRegistry_DuplicateRegistration checks the duplicate-registration error.
func TestRegistry_DuplicateRegistration(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	mustRegister(t, r, &Author{})

	_, err := r.Register(&Author{})
	if err == nil {
		t.Fatal("expected error on duplicate registration")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error should mention 'already registered': %v", err)
	}
}

// TestRegistry_FreezeBlocksRegister verifies that Register fails after Freeze.
func TestRegistry_FreezeBlocksRegister(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Freeze()

	_, err := r.Register(&Author{})
	if err == nil {
		t.Fatal("expected error when registering after Freeze")
	}
	if !strings.Contains(err.Error(), "frozen") {
		t.Errorf("error should mention 'frozen': %v", err)
	}
}

// TestRegistry_NonPointer checks that a non-pointer value is rejected.
func TestRegistry_NonPointer(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, err := r.Register(Author{})
	if err == nil {
		t.Fatal("expected error for non-pointer argument")
	}
	if !strings.Contains(err.Error(), "non-nil pointer to a struct") {
		t.Errorf("error should mention 'non-nil pointer to a struct': %v", err)
	}
}

// TestRegistry_NilPointer checks that a nil pointer is rejected.
func TestRegistry_NilPointer(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	var a *Author
	_, err := r.Register(a)
	if err == nil {
		t.Fatal("expected error for nil pointer argument")
	}
	if !strings.Contains(err.Error(), "non-nil pointer to a struct") {
		t.Errorf("error should mention 'non-nil pointer to a struct': %v", err)
	}
}

// TestRegistry_NoPrimaryKey checks the error when no pk can be resolved.
func TestRegistry_NoPrimaryKey(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	type NoPK struct {
		Title string
		Body  string
	}
	_, err := r.Register(&NoPK{})
	if err == nil {
		t.Fatal("expected error for model with no primary key")
	}
	if !strings.Contains(err.Error(), "no primary key") {
		t.Errorf("error should mention 'no primary key': %v", err)
	}
}

// TestRegistry_MultiplePKTags checks that two pk-tagged fields are an error.
func TestRegistry_MultiplePKTags(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	type TwoPK struct {
		A string `orm:"pk"`
		B string `orm:"pk"`
	}
	_, err := r.Register(&TwoPK{})
	if err == nil {
		t.Fatal("expected error for model with two pk-tagged fields")
	}
	if !strings.Contains(err.Error(), "multiple primary keys") {
		t.Errorf("error should mention 'multiple primary keys': %v", err)
	}
}

// TestRegistry_StringIDNoPK verifies that an ID field of string type is NOT
// auto-promoted and that the model fails with a no-primary-key error.
func TestRegistry_StringIDNoPK(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	type StringID struct {
		ID   string // string ID -- not auto-promoted
		Name string
	}
	_, err := r.Register(&StringID{})
	if err == nil {
		t.Fatal("expected no-primary-key error for model with string ID and no pk tag")
	}
	if !strings.Contains(err.Error(), "no primary key") {
		t.Errorf("error should mention 'no primary key': %v", err)
	}
}

// TestRegistry_DuplicateColumn verifies the duplicate column error.
func TestRegistry_DuplicateColumn(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	// Two fields that resolve to the same column name.
	type DupCol struct {
		ID    int64
		Name  string `orm:"column=name"`
		Alias string `orm:"column=name"`
	}
	_, err := r.Register(&DupCol{})
	if err == nil {
		t.Fatal("expected error for duplicate column name")
	}
	if !strings.Contains(err.Error(), "duplicate column") {
		t.Errorf("error should mention 'duplicate column': %v", err)
	}
}
