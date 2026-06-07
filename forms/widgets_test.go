package forms

import (
	"strings"
	"testing"
)

// hostile is a value containing HTML-significant characters used to confirm
// that every widget escapes the rendered value.
const hostile = `a"<>&b`

func TestTextInput_Render(t *testing.T) {
	t.Parallel()
	w := TextInput{}
	got := w.Render("title", "hello", nil)
	want := `<input type="text" name="title" value="hello">`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
}

func TestTextInput_RenderAttrsSorted(t *testing.T) {
	t.Parallel()
	w := TextInput{}
	got := w.Render("title", "hi", map[string]string{"id": "x", "class": "c"})
	want := `<input type="text" name="title" value="hi" class="c" id="x">`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
}

func TestTextInput_RenderEscapes(t *testing.T) {
	t.Parallel()
	w := TextInput{}
	got := w.Render("n"+hostile, hostile, map[string]string{"data-x": hostile})
	if strings.Contains(got, "<>") || strings.Contains(got, `"<`) {
		t.Errorf("Render did not escape: %q", got)
	}
	if !strings.Contains(got, "&#34;&lt;&gt;&amp;") {
		t.Errorf("Render missing escaped value: %q", got)
	}
}

func TestTextarea_Render(t *testing.T) {
	t.Parallel()
	w := Textarea{}
	got := w.Render("body", "line", nil)
	want := `<textarea name="body">line</textarea>`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
}

func TestTextarea_RenderEscapes(t *testing.T) {
	t.Parallel()
	w := Textarea{}
	got := w.Render("body", hostile, nil)
	if !strings.Contains(got, "a&#34;&lt;&gt;&amp;b") {
		t.Errorf("Textarea did not escape value: %q", got)
	}
}

func TestNumberInput_Render(t *testing.T) {
	t.Parallel()
	got := NumberInput{}.Render("views", "42", nil)
	want := `<input type="number" name="views" value="42">`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
}

func TestEmailInput_Render(t *testing.T) {
	t.Parallel()
	got := EmailInput{}.Render("email", "a@b.com", nil)
	want := `<input type="email" name="email" value="a@b.com">`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
}

func TestPasswordInput_NeverEchoesValue(t *testing.T) {
	t.Parallel()
	got := PasswordInput{}.Render("pw", "secret", nil)
	want := `<input type="password" name="pw" value="">`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
	if strings.Contains(got, "secret") {
		t.Errorf("PasswordInput echoed the value: %q", got)
	}
}

func TestCheckboxInput_Checked(t *testing.T) {
	t.Parallel()
	for _, truthy := range []string{"true", "on", "1"} {
		got := CheckboxInput{}.Render("ok", truthy, nil)
		if !strings.Contains(got, " checked") {
			t.Errorf("Render(%q): expected checked, got %q", truthy, got)
		}
	}
	got := CheckboxInput{}.Render("ok", "", nil)
	if strings.Contains(got, "checked") {
		t.Errorf("Render(empty): unexpected checked, got %q", got)
	}
	want := `<input type="checkbox" name="ok">`
	if got != want {
		t.Errorf("Render(empty): got %q, want %q", got, want)
	}
}

func TestCheckboxInput_CheckedHTML(t *testing.T) {
	t.Parallel()
	got := CheckboxInput{}.Render("ok", "true", nil)
	want := `<input type="checkbox" name="ok" checked>`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
}

func TestSelect_Render(t *testing.T) {
	t.Parallel()
	w := Select{Choices: [][2]string{{"a", "Apple"}, {"b", "Banana"}}}
	got := w.Render("fruit", "b", nil)
	want := `<select name="fruit">` +
		`<option value="a">Apple</option>` +
		`<option value="b" selected>Banana</option>` +
		`</select>`
	if got != want {
		t.Errorf("Render: got %q, want %q", got, want)
	}
}

func TestSelect_RenderEscapes(t *testing.T) {
	t.Parallel()
	w := Select{Choices: [][2]string{{hostile, hostile}}}
	got := w.Render("x", "v", nil)
	if strings.Contains(got, "<>") {
		t.Errorf("Select did not escape: %q", got)
	}
}
