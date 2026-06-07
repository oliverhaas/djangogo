package orm

import (
	"testing"
)

// TestRegistry_Models_RegistrationOrder verifies that Models returns models in
// the order they were registered and that the returned slice is a defensive copy.
func TestRegistry_Models_RegistrationOrder(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mustRegister(t, r, &Author{})
	mustRegister(t, r, &Tag{})
	mustRegister(t, r, &Custom{})

	got := r.Models()
	if len(got) != 3 {
		t.Fatalf("Models() len: got %d, want 3", len(got))
	}

	wantNames := []string{"Author", "Tag", "Custom"}
	for i, m := range got {
		if m.Name() != wantNames[i] {
			t.Errorf("Models()[%d].Name() = %q, want %q", i, m.Name(), wantNames[i])
		}
	}
}

// TestRegistry_Models_ReturnsCopy verifies that mutating the returned slice
// does not affect subsequent calls.
func TestRegistry_Models_ReturnsCopy(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mustRegister(t, r, &Author{})
	mustRegister(t, r, &Tag{})

	first := r.Models()
	first[0] = nil // clobber first element

	second := r.Models()
	if second[0] == nil {
		t.Error("Models() did not return a copy: mutation of returned slice affected second call")
	}
	if len(second) != 2 {
		t.Errorf("Models() len after mutation: got %d, want 2", len(second))
	}
}

// TestRegistry_Models_Empty verifies that Models on a fresh registry returns a
// non-nil empty slice.
func TestRegistry_Models_Empty(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	got := r.Models()
	if got == nil {
		t.Error("Models() on empty registry returned nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("Models() on empty registry: got len %d, want 0", len(got))
	}
}
