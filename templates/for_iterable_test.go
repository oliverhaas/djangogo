package templates_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// deck is a map whose loop yields its cards in deck order, not its keys.
type deck map[string]any

// TemplateIter implements templates.Iterable.
func (d deck) TemplateIter() []any {
	cards, _ := d["\x00cards"].([]any)
	return cards
}

var _ templates.Iterable = deck{}

// No Django test exists: a Python object with __iter__ needs no hook. This
// checks that {% for %} takes the items of an Iterable, in its order, and
// that reversed and {% empty %} apply to them.
func TestForIterable(t *testing.T) {
	engine := templates.New()
	d := deck{"\x00cards": []any{"queen", "ace", "king"}, "size": 3}
	empty := deck{"size": 0}
	for _, tc := range []struct {
		src  string
		ctx  map[string]any
		want string
	}{
		{"{% for c in d %}{{ forloop.counter }}{{ c }} {% endfor %}", map[string]any{"d": d}, "1queen 2ace 3king "},
		{"{% for c in d reversed %}{{ c }}{% if not forloop.last %},{% endif %}{% endfor %}", map[string]any{"d": d}, "king,ace,queen"},
		{"{% for c in d %}{{ c }}{% empty %}none{% endfor %}", map[string]any{"d": empty}, "none"},
		{"{{ d.size }}", map[string]any{"d": d}, "3"},
		{"{% for x in rows %}{% for c in x %}{{ c }}{% endfor %};{% endfor %}", map[string]any{"rows": []any{d, empty}}, "queenaceking;;"},
	} {
		if got := renderStr(t, engine, tc.src, tc.ctx); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
