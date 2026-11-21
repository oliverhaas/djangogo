package forms

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/templates"
)

// Media is the stylesheets and scripts a widget or a form needs, Django's
// forms.Media. Media values add up with Add, which keeps every file once and
// keeps the relative order of the files of each part. A page renders the media
// of its forms in its <head>:
//
//	forms.NewMedia(map[string][]string{"all": {"app/forms.css"}}, []string{"app/forms.js"})
//
// A path that starts with "http://", "https://" or "/" is used as it is. Any
// other path names a static file and gets the static URL in front
// (templates.Static). The zero Media holds no files.
type Media struct {
	cssLists []map[string][]string
	jsLists  [][]string
}

// NewMedia returns the media of the stylesheets css, keyed by the media types
// they apply to ("all", "screen", "screen, print"), and the scripts js.
func NewMedia(css map[string][]string, js []string) Media {
	return Media{cssLists: []map[string][]string{css}, jsLists: [][]string{js}}
}

// Add returns the media of m followed by the media of other, Django's
// Media.__add__. A part of other that m already holds is left out, and m is
// not changed.
func (m Media) Add(other Media) Media {
	out := Media{cssLists: slices.Clone(m.cssLists), jsLists: slices.Clone(m.jsLists)}
	for _, css := range other.cssLists {
		if len(css) > 0 && !slices.ContainsFunc(m.cssLists, func(have map[string][]string) bool {
			return maps.EqualFunc(have, css, slices.Equal)
		}) {
			out.cssLists = append(out.cssLists, css)
		}
	}
	for _, js := range other.jsLists {
		if len(js) > 0 && !slices.ContainsFunc(m.jsLists, func(have []string) bool { return slices.Equal(have, js) }) {
			out.jsLists = append(out.jsLists, js)
		}
	}
	return out
}

// JS returns the scripts in the order they load, every script once.
func (m Media) JS() []string { return MergeMedia(m.jsLists...) }

// CSS returns the stylesheets by media type, in the order they load, every
// stylesheet once per media type.
func (m Media) CSS() map[string][]string {
	byMedium := map[string][][]string{}
	for _, css := range m.cssLists {
		for medium, list := range css {
			byMedium[medium] = append(byMedium[medium], list)
		}
	}
	out := make(map[string][]string, len(byMedium))
	for medium, lists := range byMedium {
		out[medium] = MergeMedia(lists...)
	}
	return out
}

// CSSOnly returns the stylesheets of m without its scripts, Django's
// media["css"].
func (m Media) CSSOnly() Media { return NewMedia(m.CSS(), nil) }

// JSOnly returns the scripts of m without its stylesheets, Django's
// media["js"].
func (m Media) JSOnly() Media { return NewMedia(nil, m.JS()) }

// RenderJS returns a <script> tag for each script.
func (m Media) RenderJS() []string {
	js := m.JS()
	out := make([]string, len(js))
	for i, path := range js {
		out[i] = fmt.Sprintf(`<script src="%s"></script>`, escape(mediaPath(path)))
	}
	return out
}

// RenderCSS returns a <link> tag for each stylesheet, the media types in
// alphabetical order.
func (m Media) RenderCSS() []string {
	css := m.CSS()
	var out []string
	for _, medium := range slices.Sorted(maps.Keys(css)) {
		for _, path := range css[medium] {
			out = append(out, fmt.Sprintf(`<link href="%s" media="%s" rel="stylesheet">`,
				escape(mediaPath(path)), escape(medium)))
		}
	}
	return out
}

// Render returns the <link> tags of the stylesheets, then the <script> tags of
// the scripts, one per line.
func (m Media) Render() string {
	return strings.Join(append(m.RenderCSS(), m.RenderJS()...), "\n")
}

// String returns Render.
func (m Media) String() string { return m.Render() }

// HTML returns Render; templates print it without escaping.
func (m Media) HTML() string { return m.Render() }

// TemplateValue returns the media in the form a template uses: printed it is
// Render, and {{ media.js }} and {{ media.css }} print one kind of tag.
func (m Media) TemplateValue() any {
	return TemplateMedia{
		"js":  SafeHTML(m.JSOnly().Render()),
		"css": SafeHTML(m.CSSOnly().Render()),
		"all": SafeHTML(m.Render()),
	}
}

// TemplateMedia is media in a template. Printed, it is all the tags; its keys
// "js" and "css" hold one kind of tag.
type TemplateMedia map[string]any

// HTML returns all the tags; templates print it without escaping.
func (t TemplateMedia) HTML() string {
	s, _ := t["all"].(SafeHTML)
	return string(s)
}

// String returns HTML.
func (t TemplateMedia) String() string { return t.HTML() }

// mediaPath returns the URL of a media file, Django's Media.absolute_path.
func mediaPath(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "/") {
		return path
	}
	return templates.Static(path)
}

// MergeMedia merges lists of files into one list that keeps the relative order
// of the files of each list, every file once, Django's Media.merge. Files that
// no list orders come in the order they first appear. When two lists order the
// same files the opposite way, the files come in the order they first appear,
// where Django also warns with MediaOrderConflictWarning.
func MergeMedia(lists ...[]string) []string {
	type node struct {
		preds int
		succs []string
	}
	nodes := map[string]*node{}
	var order []string
	get := func(name string) *node {
		if n, ok := nodes[name]; ok {
			return n
		}
		n := &node{}
		nodes[name] = n
		order = append(order, name)
		return n
	}
	for _, list := range lists {
		if len(list) == 0 {
			continue
		}
		head := list[0]
		get(head)
		for _, item := range list[1:] {
			if item != head {
				get(item).preds++
				h := get(head)
				h.succs = append(h.succs, item)
			}
			head = item
		}
	}
	// Kahn's algorithm in batches, as Python's graphlib.TopologicalSorter:
	// each round emits every file whose predecessors are all out.
	var ready []string
	for _, name := range order {
		if nodes[name].preds == 0 {
			ready = append(ready, name)
		}
	}
	out := make([]string, 0, len(order))
	for len(ready) > 0 {
		batch := ready
		ready = nil
		out = append(out, batch...)
		for _, name := range batch {
			for _, succ := range nodes[name].succs {
				n := nodes[succ]
				n.preds--
				if n.preds == 0 {
					ready = append(ready, succ)
				}
			}
		}
	}
	if len(out) == len(order) {
		return out
	}
	out = out[:0]
	seen := map[string]bool{}
	for _, list := range lists {
		for _, item := range list {
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out
}

// MediaWidget is a widget that needs stylesheets or scripts, Django's
// Widget.media. A widget that embeds another one inherits its Media method; to
// extend it, it returns the embedded widget's media Add its own.
type MediaWidget interface {
	Widget
	// Media returns the files the widget needs.
	Media() Media
}

// WidgetMedia returns the media of w, the zero Media for a widget that needs
// none.
func WidgetMedia(w Widget) Media {
	if mw, ok := w.(MediaWidget); ok {
		return mw.Media()
	}
	return Media{}
}

// Media returns the media of the widgets of the form's fields, followed by the
// form's ExtraMedia, Django's Form.media.
func (f *Form) Media() Media {
	var out Media
	for _, name := range f.names {
		out = out.Add(WidgetMedia(f.fields[name].Base().Widget))
	}
	return out.Add(f.ExtraMedia)
}
