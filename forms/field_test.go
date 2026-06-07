package forms

import (
	"testing"
	"time"
)

func TestField_CleanRequiredEmpty(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "title", Kind: CharField, Required: true}
	_, err := f.Clean("")
	if err == nil {
		t.Fatal("expected required error, got nil")
	}
	if err.Error() != "This field is required." {
		t.Errorf("error: got %q", err.Error())
	}
}

func TestField_CleanCharTrimAndMaxLength(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "title", Kind: CharField, MaxLength: 5}
	got, err := f.Clean("  hi  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hi" {
		t.Errorf("clean: got %v, want %q", got, "hi")
	}
	if _, err := f.Clean("toolong"); err == nil {
		t.Error("expected max_length error, got nil")
	}
}

func TestField_CleanInteger(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "views", Kind: IntegerField}
	got, err := f.Clean("42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != int64(42) {
		t.Errorf("clean: got %v (%T), want int64(42)", got, got)
	}
	if _, err := f.Clean("nope"); err == nil {
		t.Error("expected parse error, got nil")
	}
}

func TestField_CleanBoolTruthy(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "ok", Kind: BoolField, Required: true}
	for _, in := range []string{"true", "on", "1"} {
		got, err := f.Clean(in)
		if err != nil {
			t.Fatalf("Clean(%q) error: %v", in, err)
		}
		if got != true {
			t.Errorf("Clean(%q): got %v, want true", in, got)
		}
	}
	// BoolField never errors on Required-empty: unchecked means false.
	got, err := f.Clean("")
	if err != nil {
		t.Fatalf("Clean(empty) error: %v", err)
	}
	if got != false {
		t.Errorf("Clean(empty): got %v, want false", got)
	}
}

func TestField_CleanEmail(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "email", Kind: EmailField}
	if _, err := f.Clean("user@example.com"); err != nil {
		t.Errorf("valid email rejected: %v", err)
	}
	if _, err := f.Clean("not-an-email"); err == nil {
		t.Error("invalid email accepted")
	}
}

func TestField_CleanChoice(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "c", Kind: ChoiceField, Choices: [][2]string{{"a", "A"}, {"b", "B"}}}
	got, err := f.Clean("a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a" {
		t.Errorf("clean: got %v, want %q", got, "a")
	}
	if _, err := f.Clean("z"); err == nil {
		t.Error("expected membership error, got nil")
	}
}

func TestField_CleanDateTime(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "at", Kind: DateTimeField}
	got, err := f.Clean("2023-01-02T15:04:05Z")
	if err != nil {
		t.Fatalf("RFC3339 rejected: %v", err)
	}
	if _, ok := got.(time.Time); !ok {
		t.Fatalf("clean: got %T, want time.Time", got)
	}
	if _, err := f.Clean("2023-01-02 15:04:05"); err != nil {
		t.Errorf("alt layout rejected: %v", err)
	}
	if _, err := f.Clean("nope"); err == nil {
		t.Error("invalid datetime accepted")
	}
}

func TestField_CleanOptionalEmptyReturnsZero(t *testing.T) {
	t.Parallel()
	f := &Field{Name: "title", Kind: CharField}
	got, err := f.Clean("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("clean: got %v, want empty string", got)
	}
}
