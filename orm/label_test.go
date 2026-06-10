package orm

import "testing"

// labelStrValue has a value-receiver String(), so both a value and a pointer
// instance satisfy fmt.Stringer directly.
type labelStrValue struct {
	ID    int64
	Title string
}

func (p labelStrValue) String() string { return "value:" + p.Title }

// labelStrPtr has a pointer-receiver String(), so a value instance only
// satisfies fmt.Stringer after Label copies it into an addressable value.
type labelStrPtr struct {
	ID   int64
	Name string
}

func (p *labelStrPtr) String() string { return "ptr:" + p.Name }

// labelPlain has no String(), exercising Django's default label form.
type labelPlain struct {
	ID   int64
	Name string
}

func TestLabelUsesValueReceiverStringer(t *testing.T) {
	r := NewRegistry()
	m, err := r.Register(&labelStrValue{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := Label(m, labelStrValue{ID: 1, Title: "Hi"}); got != "value:Hi" {
		t.Errorf("Label(value) = %q, want %q", got, "value:Hi")
	}
	if got := Label(m, &labelStrValue{ID: 2, Title: "Yo"}); got != "value:Yo" {
		t.Errorf("Label(pointer) = %q, want %q", got, "value:Yo")
	}
}

func TestLabelUsesPointerReceiverStringer(t *testing.T) {
	r := NewRegistry()
	m, err := r.Register(&labelStrPtr{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := Label(m, &labelStrPtr{ID: 1, Name: "A"}); got != "ptr:A" {
		t.Errorf("Label(pointer) = %q, want %q", got, "ptr:A")
	}
	// A value instance must still reach a pointer-receiver String().
	if got := Label(m, labelStrPtr{ID: 2, Name: "B"}); got != "ptr:B" {
		t.Errorf("Label(value) = %q, want %q", got, "ptr:B")
	}
}

func TestLabelDefaultForm(t *testing.T) {
	r := NewRegistry()
	m, err := r.Register(&labelPlain{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	want := "labelPlain object (7)"
	if got := Label(m, &labelPlain{ID: 7}); got != want {
		t.Errorf("Label = %q, want %q", got, want)
	}
	if got := Label(m, labelPlain{ID: 7}); got != want {
		t.Errorf("Label(value) = %q, want %q", got, want)
	}
}
