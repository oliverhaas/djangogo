package templates

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderStringInterpolation(t *testing.T) {
	eng, err := NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := eng.RenderString("Hello {{ name }}!", map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if want := "Hello World!"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderStringSyntaxErrorSurfaces(t *testing.T) {
	eng, err := NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	// Unclosed tag is a parse error; it must return an error, not panic.
	if _, err := eng.RenderString("{% if %}", nil); err == nil {
		t.Fatal("expected an error for a malformed template, got nil")
	}
}

func TestNewEngineRenderFromDir(t *testing.T) {
	dir := t.TempDir()
	tplPath := filepath.Join(dir, "greeting.html")
	if err := os.WriteFile(tplPath, []byte("Hi {{ who }}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	eng, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := eng.Render("greeting.html", map[string]any{"who": "Ada"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := "Hi Ada"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderToWritesToWriter(t *testing.T) {
	// RenderTo is exercised against a named template; write one to a temp dir.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.html"), []byte("{{ a }}-{{ b }}"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	eng, err := NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	var buf bytes.Buffer
	if err := eng.RenderTo(&buf, "x.html", map[string]any{"a": 1, "b": 2}); err != nil {
		t.Fatalf("RenderTo: %v", err)
	}
	if want := "1-2"; buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestRenderMissingTemplateErrors(t *testing.T) {
	eng, err := NewEngine(t.TempDir())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if _, err := eng.Render("does-not-exist.html", nil); err == nil {
		t.Fatal("expected an error for a missing template, got nil")
	}
}
