package templates

import (
	"errors"
	"html/template"
	"testing"
	"time"

	"github.com/flosch/pongo2/v6"
)

// TestIsSafe guards the reflection behind isSafe. Should pongo2 rename the
// unexported field that holds the safe mark, every value would count as not
// safe and be escaped twice; this test reports that before a template does.
func TestIsSafe(t *testing.T) {
	if !isSafe(pongo2.AsSafeValue("<b>")) {
		t.Error("isSafe(AsSafeValue) = false; pongo2 renamed the safe field")
	}
	if isSafe(pongo2.AsValue("<b>")) {
		t.Error("isSafe(AsValue) = true, want false")
	}
	if isSafe(nil) {
		t.Error("isSafe(nil) = true, want false")
	}
}

// TestNowTagUsesClock renders {% now %} at a fixed time, which the ported
// Django tests cannot do because they compare with the real clock.
func TestNowTagUsesClock(t *testing.T) {
	saved := timeNow
	t.Cleanup(func() { timeNow = saved })
	timeNow = func() time.Time { return time.Date(2008, 1, 2, 3, 4, 5, 0, time.UTC) }

	cases := []struct{ src, want string }{
		{`{% now "Y-m-d H:i:s" %}`, "2008-01-02 03:04:05"},
		{`{% now "jS F Y" %}`, "2nd January 2008"},
		{`{% now "Y" as year %}[{{ year }}]`, "[2008]"},
	}
	for _, c := range cases {
		got, err := New().RenderString(c.src, nil)
		if err != nil {
			t.Fatalf("RenderString(%q): %v", c.src, err)
		}
		if got != c.want {
			t.Errorf("RenderString(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

// htmlStringer is a value pongo2 prints through its String method.
type htmlStringer struct{ s string }

// String returns the markup the value stands for.
func (h htmlStringer) String() string { return h.s }

// TestStringerEscaped guards against a value of a non-string type with a
// String method reaching the page unescaped: pongo2 escapes only strings.
// Django escapes str(value) of any object.
func TestStringerEscaped(t *testing.T) {
	ctx := map[string]any{"v": htmlStringer{"<b>"}}
	cases := []struct{ src, want string }{
		{`{{ v }}`, "&lt;b&gt;"},
		{`{{ v|default:"x" }}`, "&lt;b&gt;"},
		{`{% firstof v %}`, "&lt;b&gt;"},
		{`{% cycle v v %}`, "&lt;b&gt;"},
		{`{% with w=v %}{{ w }}{% endwith %}`, "&lt;b&gt;"},
		{`{% autoescape off %}{{ v }}{% endautoescape %}`, "<b>"},
	}
	for _, c := range cases {
		got, err := New().RenderString(c.src, ctx)
		if err != nil {
			t.Fatalf("RenderString(%q): %v", c.src, err)
		}
		if got != c.want {
			t.Errorf("RenderString(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestPanicBecomesError checks that a template pongo2 panics on, a modulo
// by zero, fails with an error instead of taking the program down.
func TestPanicBecomesError(t *testing.T) {
	_, err := New().RenderString(`{{ a % b }}`, map[string]any{"a": 1, "b": 0})
	if !errors.Is(err, errPanic) {
		t.Fatalf("RenderString error = %v, want one wrapping errPanic", err)
	}
}

// TestFloatPrintsLikePython checks the text of a float, which pongo2 prints
// with six decimal places and Django with Python's str().
func TestFloatPrintsLikePython(t *testing.T) {
	cases := []struct {
		f    float64
		want string
	}{
		{1.5, "1.5"},
		{2, "2.0"},
		{-0.25, "-0.25"},
		{1e-5, "0.00001"},
		{1e16, "10000000000000000"},
		{0.30000000000000004, "0.30000000000000004"},
	}
	for _, c := range cases {
		got, err := New().RenderString(`{{ f }}`, map[string]any{"f": c.f})
		if err != nil {
			t.Fatalf("RenderString: %v", err)
		}
		if got != c.want {
			t.Errorf("{{ f }} with %v = %q, want %q", c.f, got, c.want)
		}
	}
}

// htmlBlock is a value that renders itself as HTML.
type htmlBlock struct{ html string }

// HTML returns the markup.
func (h htmlBlock) HTML() string { return h.html }

// String returns the markup too, as the String of a form does.
func (h htmlBlock) String() string { return h.html }

// TestHTMLerNotEscaped checks that a value implementing HTMLer, and a
// html/template.HTML value, print as written, like a Django object with
// __html__.
func TestHTMLerNotEscaped(t *testing.T) {
	ctx := map[string]any{"v": htmlBlock{"<b>x</b>\ny"}, "h": template.HTML("<i>")}
	cases := []struct{ src, want string }{
		{`{{ v }}`, "<b>x</b>\ny"},
		{`{{ v|linebreaksbr }}`, "<b>x</b><br>y"},
		{`{% firstof v %}`, "<b>x</b>\ny"},
		// upper drops the safe mark, as in Django.
		{`{{ h }}{{ h|upper }}`, "<i>&lt;I&gt;"},
	}
	for _, c := range cases {
		got, err := New().RenderString(c.src, ctx)
		if err != nil {
			t.Fatalf("RenderString(%q): %v", c.src, err)
		}
		if got != c.want {
			t.Errorf("RenderString(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}
