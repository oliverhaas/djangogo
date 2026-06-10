package templates

import (
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

// tmplPost mirrors a typical model: a pk, scalar fields whose Go names map to
// snake_case columns, and a String() label.
type tmplPost struct {
	ID        int64
	Title     string
	Body      string `orm:"type=text"`
	CreatedAt int64
}

func (p tmplPost) String() string { return "POST:" + p.Title }

func modelFor(t *testing.T, ptr any) *orm.Model {
	t.Helper()
	r := orm.NewRegistry()
	m, err := r.Register(ptr)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return m
}

func TestModelContextExposesSnakeCaseFields(t *testing.T) {
	m := modelFor(t, &tmplPost{})
	mm := ModelContext(m, tmplPost{ID: 3, Title: "Hi", Body: "B", CreatedAt: 99})

	cases := map[string]any{
		"title":      "Hi",
		"body":       "B",
		"created_at": int64(99),
		"id":         int64(3),
		"pk":         int64(3),
	}
	for key, want := range cases {
		if got := mm[key]; got != want {
			t.Errorf("mm[%q] = %#v, want %#v", key, got, want)
		}
	}
}

func TestModelContextStringIsLabel(t *testing.T) {
	m := modelFor(t, &tmplPost{})
	mm := ModelContext(m, tmplPost{ID: 1, Title: "Hello"})
	if got := mm.String(); got != "POST:Hello" {
		t.Errorf("ModelMap.String() = %q, want %q", got, "POST:Hello")
	}
}

// TestModelContextRendersInPongo confirms pongo2 resolves snake_case map keys
// for {{ post.title }} and the Stringer label for a bare {{ post }}.
func TestModelContextRendersInPongo(t *testing.T) {
	eng := newEngine()
	m := modelFor(t, &tmplPost{})
	ctx := map[string]any{"post": ModelContext(m, tmplPost{ID: 2, Title: "Wow"})}

	out, err := eng.RenderString("{{ post.title }}|{{ post }}|{{ post.pk }}", ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := "Wow|POST:Wow|2"; out != want {
		t.Errorf("render = %q, want %q", out, want)
	}
}
