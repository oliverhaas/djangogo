package templates

import (
	"errors"
	"fmt"
	"testing"
)

func newStringEngine(t *testing.T) *Engine {
	t.Helper()
	eng, err := NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
}

func TestStaticTag(t *testing.T) {
	eng := newStringEngine(t)
	got, err := eng.RenderString(`{% static "css/app.css" %}`, nil)
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if want := "/static/css/app.css"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStaticTagRespectsStaticBase(t *testing.T) {
	prev := StaticBase
	t.Cleanup(func() { StaticBase = prev })
	StaticBase = "https://cdn.example.com/assets/"

	eng := newStringEngine(t)
	got, err := eng.RenderString(`{% static "js/app.js" %}`, nil)
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if want := "https://cdn.example.com/assets/js/app.js"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStaticTagVariableArg(t *testing.T) {
	eng := newStringEngine(t)
	got, err := eng.RenderString(`{% static path %}`, map[string]any{"path": "img/logo.png"})
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if want := "/static/img/logo.png"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestURLTag(t *testing.T) {
	prev := URLResolver
	t.Cleanup(func() { URLResolver = prev })
	URLResolver = func(name string, args ...any) (string, error) {
		if name == "article-detail" && len(args) == 1 && fmt.Sprint(args[0]) == "42" {
			return "/articles/42/", nil
		}
		return "", fmt.Errorf("unexpected reverse: %s %v", name, args)
	}

	eng := newStringEngine(t)
	got, err := eng.RenderString(`{% url "article-detail" 42 %}`, nil)
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if want := "/articles/42/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestURLTagResolverErrorSurfaces(t *testing.T) {
	prev := URLResolver
	t.Cleanup(func() { URLResolver = prev })
	sentinel := errors.New("no such route")
	URLResolver = func(_ string, _ ...any) (string, error) {
		return "", sentinel
	}

	eng := newStringEngine(t)
	_, err := eng.RenderString(`{% url "missing" %}`, nil)
	if err == nil {
		t.Fatal("expected an error from the url tag, got nil")
	}
}

func TestCSRFTokenTag(t *testing.T) {
	eng := newStringEngine(t)
	got, err := eng.RenderString(`{% csrf_token %}`, map[string]any{"csrf_token": "abc123"})
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	want := `<input type="hidden" name="csrfmiddlewaretoken" value="abc123">`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCSRFTokenTagEscapes(t *testing.T) {
	eng := newStringEngine(t)
	got, err := eng.RenderString(`{% csrf_token %}`, map[string]any{"csrf_token": `a"<>&b`})
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	want := `<input type="hidden" name="csrfmiddlewaretoken" value="a&#34;&lt;&gt;&amp;b">`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCSRFTokenTagAbsentToken(t *testing.T) {
	eng := newStringEngine(t)
	got, err := eng.RenderString(`{% csrf_token %}`, nil)
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	want := `<input type="hidden" name="csrfmiddlewaretoken" value="">`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
