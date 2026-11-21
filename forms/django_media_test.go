package forms_test

import (
	"reflect"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/templates"
)

// Django's FormsMediaTestCase runs under
// @override_settings(STATIC_URL="http://media.example.com/static/").
func mediaStaticURL(t *testing.T) {
	t.Helper()
	prev := templates.StaticBase
	t.Cleanup(func() { templates.StaticBase = prev })
	templates.StaticBase = "http://media.example.com/static/"
}

// The media definitions that Django's tests give MyWidget1 to MyWidget3.
var (
	media1 = forms.NewMedia(
		map[string][]string{"all": {"path/to/css1", "/path/to/css2"}},
		[]string{"/path/to/js1", "http://media.other.com/path/to/js2", "https://secure.other.com/path/to/js3"},
	)
	media2 = forms.NewMedia(map[string][]string{"all": {"/path/to/css2", "/path/to/css3"}},
		[]string{"/path/to/js1", "/path/to/js4"})
	media3 = forms.NewMedia(map[string][]string{"all": {"path/to/css1", "/path/to/css3"}},
		[]string{"/path/to/js1", "/path/to/js4"})
)

// mediaWidget is a TextInput with media, Django's TextInput subclass with a
// class Media.
type mediaWidget struct {
	forms.TextInput
	media forms.Media
}

// Media returns the widget's media.
func (w mediaWidget) Media() forms.Media { return w.media }

// media1HTML is str(MyWidget1().media).
const media1HTML = `<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="http://media.other.com/path/to/js2"></script>
<script src="https://secure.other.com/path/to/js3"></script>`

// combinedHTML is the media of MyWidget1, MyWidget2 and MyWidget3 together.
const combinedHTML = `<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">
<link href="/path/to/css3" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="http://media.other.com/path/to/js2"></script>
<script src="/path/to/js4"></script>
<script src="https://secure.other.com/path/to/js3"></script>`

// assertMedia compares the rendered media exactly, as Django's assertEqual on
// str(media).
func assertMedia(t *testing.T, m forms.Media, want string) {
	t.Helper()
	if got := m.String(); got != want {
		t.Errorf("media =\n%s\nwant\n%s", got, want)
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_construction
// The repr and the Media(Foo) construction from an object are Python only.
func TestMediaConstruction(t *testing.T) {
	mediaStaticURL(t)
	assertMedia(t, media1, media1HTML)

	// A widget can exist without a media definition.
	assertMedia(t, forms.WidgetMedia(forms.TextInput{}), "")
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_dsl
func TestMediaDSL(t *testing.T) {
	mediaStaticURL(t)
	w1 := mediaWidget{media: media1}
	assertMedia(t, forms.WidgetMedia(w1), media1HTML)

	// Media objects can be interrogated by media type.
	assertMedia(t, w1.Media().CSSOnly(), `<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">`)
	assertMedia(t, w1.Media().JSOnly(), `<script src="/path/to/js1"></script>
<script src="http://media.other.com/path/to/js2"></script>
<script src="https://secure.other.com/path/to/js3"></script>`)
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_combine_media
func TestMediaCombine(t *testing.T) {
	mediaStaticURL(t)
	w1, w2, w3 := mediaWidget{media: media1}, mediaWidget{media: media2}, mediaWidget{media: media3}
	assertMedia(t, w1.Media().Add(w2.Media()).Add(w3.Media()), combinedHTML)

	// Media addition hasn't affected the original objects.
	assertMedia(t, w1.Media(), media1HTML)

	// Regression check for #12879: specifying the same CSS or JS file multiple
	// times in a single Media instance should result in that file only being
	// included once.
	w4 := mediaWidget{media: forms.NewMedia(map[string][]string{"all": {"/path/to/css1", "/path/to/css1"}},
		[]string{"/path/to/js1", "/path/to/js1"})}
	assertMedia(t, w4.Media(), `<link href="/path/to/css1" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>`)
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_deduplication
func TestMediaDeduplication(t *testing.T) {
	media := forms.NewMedia(map[string][]string{"all": {"/path/to/css1", "/path/to/css1"}},
		[]string{"/path/to/js1", "/path/to/js1"})
	assertMedia(t, media, `<link href="/path/to/css1" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>`)
}

// propertyWidget is Django's MyWidget4, whose media is a property.
type propertyWidget struct{ forms.TextInput }

// Media returns the widget's media.
func (propertyWidget) Media() forms.Media {
	return forms.NewMedia(map[string][]string{"all": {"/some/path"}}, []string{"/some/js"})
}

// propertyChildWidget is Django's MyWidget5: its media extends the media of
// the widget it embeds.
type propertyChildWidget struct{ propertyWidget }

// Media returns the parent's media followed by the widget's own.
func (w propertyChildWidget) Media() forms.Media {
	return w.propertyWidget.Media().Add(forms.NewMedia(map[string][]string{"all": {"/other/path"}}, []string{"/other/js"}))
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_property
func TestMediaProperty(t *testing.T) {
	// Widget media can be defined as a property.
	assertMedia(t, forms.WidgetMedia(propertyWidget{}), `<link href="/some/path" media="all" rel="stylesheet">
<script src="/some/js"></script>`)

	// Media properties can reference the media of their parents.
	assertMedia(t, forms.WidgetMedia(propertyChildWidget{}), `<link href="/some/path" media="all" rel="stylesheet">
<link href="/other/path" media="all" rel="stylesheet">
<script src="/some/js"></script>
<script src="/other/js"></script>`)
}

// extendingWidget is a widget whose media extends the media of the widget it
// embeds: Django's subclass with a class Media and extend=True.
type extendingWidget struct {
	mediaWidget
	own forms.Media
}

// Media returns the parent's media followed by the widget's own.
func (w extendingWidget) Media() forms.Media { return w.mediaWidget.Media().Add(w.own) }

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_property_parent_references
func TestMediaPropertyParentReferences(t *testing.T) {
	mediaStaticURL(t)
	w6 := extendingWidget{
		mediaWidget: mediaWidget{media: media1},
		own:         forms.NewMedia(map[string][]string{"all": {"/other/path"}}, []string{"/other/js"}),
	}
	assertMedia(t, forms.WidgetMedia(w6), `<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/other/path" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="/other/js"></script>
<script src="http://media.other.com/path/to/js2"></script>
<script src="https://secure.other.com/path/to/js3"></script>`)
}

// inheritedWidgetMedia is the class Media of Django's MyWidget8 and MyWidget11.
var inheritedWidgetMedia = forms.NewMedia(map[string][]string{"all": {"/path/to/css3", "path/to/css1"}},
	[]string{"/path/to/js1", "/path/to/js4"})

// inheritedHTML is the media of MyWidget8 and MyWidget11.
const inheritedHTML = `<link href="/path/to/css3" media="all" rel="stylesheet">
<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="http://media.other.com/path/to/js2"></script>
<script src="/path/to/js4"></script>
<script src="https://secure.other.com/path/to/js3"></script>`

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_inheritance
func TestMediaInheritance(t *testing.T) {
	mediaStaticURL(t)
	// If a widget extends another but provides no media definition, it
	// inherits the parent widget's media.
	w7 := struct{ mediaWidget }{mediaWidget{media: media1}}
	assertMedia(t, forms.WidgetMedia(w7), media1HTML)

	// If a widget extends another but defines media, it extends the parent
	// widget's media by default.
	w8 := extendingWidget{mediaWidget: mediaWidget{media: media1}, own: inheritedWidgetMedia}
	assertMedia(t, forms.WidgetMedia(w8), inheritedHTML)
}

// propertyExtendingWidget is Django's MyWidget9: a class Media over a parent
// whose media is a property.
type propertyExtendingWidget struct{ propertyWidget }

// Media returns the parent's media followed by the widget's own.
func (w propertyExtendingWidget) Media() forms.Media {
	return w.propertyWidget.Media().Add(forms.NewMedia(map[string][]string{"all": {"/other/path"}}, []string{"/other/js"}))
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_inheritance_from_property
func TestMediaInheritanceFromProperty(t *testing.T) {
	mediaStaticURL(t)
	assertMedia(t, forms.WidgetMedia(propertyExtendingWidget{}), `<link href="/some/path" media="all" rel="stylesheet">
<link href="/other/path" media="all" rel="stylesheet">
<script src="/some/js"></script>
<script src="/other/js"></script>`)

	// A widget can disable media inheritance by specifying 'extend=False':
	// in Go its Media method leaves the parent's media out.
	w10 := struct{ mediaWidget }{mediaWidget{media: inheritedWidgetMedia}}
	assertMedia(t, forms.WidgetMedia(w10), `<link href="/path/to/css3" media="all" rel="stylesheet">
<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="/path/to/js4"></script>`)
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_inheritance_extends
func TestMediaInheritanceExtends(t *testing.T) {
	mediaStaticURL(t)
	w11 := extendingWidget{mediaWidget: mediaWidget{media: media1}, own: inheritedWidgetMedia}
	assertMedia(t, forms.WidgetMedia(w11), inheritedHTML)
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_media_inheritance_single_type
// extend=("css",) is the parent's CSSOnly media followed by the widget's own.
func TestMediaInheritanceSingleType(t *testing.T) {
	mediaStaticURL(t)
	parent := mediaWidget{media: media1}
	assertMedia(t, parent.Media().CSSOnly().Add(inheritedWidgetMedia), `<link href="/path/to/css3" media="all" rel="stylesheet">
<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="/path/to/js4"></script>`)
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_multi_media
func TestMediaMultiMedia(t *testing.T) {
	multimedia := mediaWidget{media: forms.NewMedia(map[string][]string{
		"screen, print": {"/file1", "/file2"},
		"screen":        {"/file3"},
		"print":         {"/file4"},
	}, []string{"/path/to/js1", "/path/to/js4"})}
	assertMedia(t, forms.WidgetMedia(multimedia), `<link href="/file4" media="print" rel="stylesheet">
<link href="/file3" media="screen" rel="stylesheet">
<link href="/file1" media="screen, print" rel="stylesheet">
<link href="/file2" media="screen, print" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="/path/to/js4"></script>`)
}

// FIXME-PORT: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_multi_widget
// needs MultiWidget, which the Go forms package does not have.

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_form_media
func TestFormMedia(t *testing.T) {
	mediaStaticURL(t)
	newMyForm := func() *forms.Form {
		return forms.New(
			forms.F("field1", forms.NewCharField(forms.MaxLength(20), forms.UseWidget(mediaWidget{media: media1}))),
			forms.F("field2", forms.NewCharField(forms.MaxLength(20), forms.UseWidget(mediaWidget{media: media2}))),
		)
	}
	// You can ask a form for the media required by its widgets.
	f1 := newMyForm()
	assertMedia(t, f1.Media(), combinedHTML)

	// Form media can be combined to produce a single media definition.
	f2 := forms.New(forms.F("field3", forms.NewCharField(forms.MaxLength(20), forms.UseWidget(mediaWidget{media: media3}))))
	assertMedia(t, f1.Media().Add(f2.Media()), combinedHTML)

	// Forms can also define media, following the same rules as widgets.
	f3 := newMyForm()
	f3.ExtraMedia = forms.NewMedia(map[string][]string{"all": {"/some/form/css"}}, []string{"/some/form/javascript"})
	want := `<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/some/form/css" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">
<link href="/path/to/css3" media="all" rel="stylesheet">
<script src="/path/to/js1"></script>
<script src="/some/form/javascript"></script>
<script src="http://media.other.com/path/to/js2"></script>
<script src="/path/to/js4"></script>
<script src="https://secure.other.com/path/to/js3"></script>`
	assertMedia(t, f3.Media(), want)

	// Media works in templates.
	got := renderTemplate(t, "{{ form.media.js }}{{ form.media.css }}", map[string]any{"form": f3})
	wantTemplate := `<script src="/path/to/js1"></script>
<script src="/some/form/javascript"></script>
<script src="http://media.other.com/path/to/js2"></script>
<script src="/path/to/js4"></script>
<script src="https://secure.other.com/path/to/js3"></script>` +
		`<link href="http://media.example.com/static/path/to/css1" media="all" rel="stylesheet">
<link href="/some/form/css" media="all" rel="stylesheet">
<link href="/path/to/css2" media="all" rel="stylesheet">
<link href="/path/to/css3" media="all" rel="stylesheet">`
	if got != wantTemplate {
		t.Errorf("template =\n%s\nwant\n%s", got, wantTemplate)
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_html_safe
func TestMediaHTMLSafe(t *testing.T) {
	media := forms.NewMedia(map[string][]string{"all": {"/path/to/css"}}, []string{"/path/to/js"})
	if media.String() != media.HTML() {
		t.Errorf("String() = %q, HTML() = %q", media.String(), media.HTML())
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_merge
func TestMediaMerge(t *testing.T) {
	cases := []struct {
		lists [][]string
		want  []string
	}{
		{[][]string{{"1", "2"}, {"3", "4"}}, []string{"1", "3", "2", "4"}},
		{[][]string{{"1", "2"}, {"2", "3"}}, []string{"1", "2", "3"}},
		{[][]string{{"2", "3"}, {"1", "2"}}, []string{"1", "2", "3"}},
		{[][]string{{"1", "3"}, {"2", "3"}}, []string{"1", "2", "3"}},
		{[][]string{{"1", "2"}, {"1", "3"}}, []string{"1", "2", "3"}},
		{[][]string{{"1", "2"}, {"3", "2"}}, []string{"1", "3", "2"}},
		{[][]string{{"1", "2"}, {"1", "2"}}, []string{"1", "2"}},
		{
			[][]string{{"1", "2"}, {"1", "3"}, {"2", "3"}, {"5", "7"}, {"5", "6"}, {"6", "7", "9"}, {"8", "9"}},
			[]string{"1", "5", "8", "2", "6", "3", "7", "9"},
		},
		{nil, []string{}},
		{[][]string{{"1", "2"}}, []string{"1", "2"}},
	}
	for _, c := range cases {
		if got := forms.MergeMedia(c.lists...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("MergeMedia(%v) = %v, want %v", c.lists, got, c.want)
		}
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_merge_warning
// Go has no warnings; the merge falls back to the order of first appearance.
func TestMediaMergeWarning(t *testing.T) {
	if got := forms.MergeMedia([]string{"1", "2"}, []string{"2", "1"}, nil); !reflect.DeepEqual(got, []string{"1", "2"}) {
		t.Errorf("MergeMedia = %v, want [1 2]", got)
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_merge_js_three_way
func TestMediaMergeJSThreeWay(t *testing.T) {
	widget1 := forms.NewMedia(nil, []string{"color-picker.js"})
	widget2 := forms.NewMedia(nil, []string{"text-editor.js"})
	widget3 := forms.NewMedia(nil, []string{"text-editor.js", "text-editor-extras.js", "color-picker.js"})
	merged := widget1.Add(widget2).Add(widget3)
	if got, want := merged.JS(), []string{"text-editor.js", "text-editor-extras.js", "color-picker.js"}; !reflect.DeepEqual(got, want) {
		t.Errorf("JS() = %v, want %v", got, want)
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_merge_js_three_way2
func TestMediaMergeJSThreeWay2(t *testing.T) {
	// The merge prefers to place 'c' before 'b' and 'g' before 'h' to preserve
	// the original order. The preference 'c'->'b' is overridden by widget3's
	// media, but 'g'->'h' survives in the final ordering.
	widget1 := forms.NewMedia(nil, []string{"a", "c", "f", "g", "k"})
	widget2 := forms.NewMedia(nil, []string{"a", "b", "f", "h", "k"})
	widget3 := forms.NewMedia(nil, []string{"b", "c", "f", "k"})
	merged := widget1.Add(widget2).Add(widget3)
	if got, want := merged.JS(), []string{"a", "b", "c", "f", "g", "h", "k"}; !reflect.DeepEqual(got, want) {
		t.Errorf("JS() = %v, want %v", got, want)
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_merge_css_three_way
func TestMediaMergeCSSThreeWay(t *testing.T) {
	widget1 := forms.NewMedia(map[string][]string{"screen": {"c.css"}, "all": {"d.css", "e.css"}}, nil)
	widget2 := forms.NewMedia(map[string][]string{"screen": {"a.css"}}, nil)
	widget3 := forms.NewMedia(map[string][]string{"screen": {"a.css", "b.css", "c.css"}, "all": {"e.css"}}, nil)
	widget4 := forms.NewMedia(map[string][]string{"all": {"d.css", "e.css"}, "screen": {"c.css"}}, nil)
	merged := widget1.Add(widget2)
	// c.css comes before a.css because widget1 + widget2 establishes this
	// order.
	assertCSS(t, merged, map[string][]string{"screen": {"c.css", "a.css"}, "all": {"d.css", "e.css"}})
	merged = merged.Add(widget3)
	// widget3 contains an explicit ordering of c.css and a.css.
	assertCSS(t, merged, map[string][]string{"screen": {"a.css", "b.css", "c.css"}, "all": {"d.css", "e.css"}})
	// Media ordering does not matter.
	assertCSS(t, widget1.Add(widget4), map[string][]string{"screen": {"c.css"}, "all": {"d.css", "e.css"}})
}

// assertCSS compares the merged stylesheets of m, Django's assertEqual on
// media._css.
func assertCSS(t *testing.T, m forms.Media, want map[string][]string) {
	t.Helper()
	if got := m.CSS(); !reflect.DeepEqual(got, want) {
		t.Errorf("CSS() = %v, want %v", got, want)
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_add_js_deduplication
// _js_lists is private in Go; the merged lists show the same.
func TestMediaAddJSDeduplication(t *testing.T) {
	widget1 := forms.NewMedia(nil, []string{"a", "b", "c"})
	widget2 := forms.NewMedia(nil, []string{"a", "b"})
	widget3 := forms.NewMedia(nil, []string{"a", "c", "b"})
	for _, merged := range []forms.Media{widget1.Add(widget1), widget1.Add(widget2)} {
		if got, want := merged.JS(), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
			t.Errorf("JS() = %v, want %v", got, want)
		}
	}
	// Lists with items in a different order are preserved when added, and
	// conflict: the merge falls back to the order of first appearance.
	if got, want := widget1.Add(widget3).JS(), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("JS() = %v, want %v", got, want)
	}
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_add_css_deduplication
func TestMediaAddCSSDeduplication(t *testing.T) {
	widget1 := forms.NewMedia(map[string][]string{"screen": {"a.css"}, "all": {"b.css"}}, nil)
	widget2 := forms.NewMedia(map[string][]string{"screen": {"c.css"}}, nil)
	widget3 := forms.NewMedia(map[string][]string{"screen": {"a.css"}, "all": {"b.css", "c.css"}}, nil)
	widget4 := forms.NewMedia(map[string][]string{"screen": {"a.css"}, "all": {"c.css", "b.css"}}, nil)
	assertCSS(t, widget1.Add(widget1), map[string][]string{"screen": {"a.css"}, "all": {"b.css"}})
	assertCSS(t, widget1.Add(widget2), map[string][]string{"screen": {"a.css", "c.css"}, "all": {"b.css"}})
	// Ordering within lists is preserved, and the conflict falls back to the
	// order of first appearance.
	assertCSS(t, widget3.Add(widget4), map[string][]string{"screen": {"a.css"}, "all": {"b.css", "c.css"}})
}

// Ported from Django: tests/forms_tests/tests/test_media.py::FormsMediaTestCase.test_add_empty
func TestMediaAddEmpty(t *testing.T) {
	media := forms.NewMedia(map[string][]string{"screen": {"a.css"}}, []string{"a"})
	merged := media.Add(forms.NewMedia(nil, nil))
	if !reflect.DeepEqual(merged, media) {
		t.Errorf("media + empty = %#v, want %#v", merged, media)
	}
}

// FIXME-PORT: the MediaAsset, Script and Stylesheet objects, the safe-string
// assets and render(attrs=...) of test_media.py (MediaAssetTestCase,
// ScriptTestCase, StylesheetTestCase, FormsMediaObjectTestCase and the
// test_html_safe_string_* and test_render_*_with_attrs tests of
// FormsMediaTestCase) are not ported: Go media holds paths only.
// test_add_invalid_type is a Python type error that Go's types rule out.
