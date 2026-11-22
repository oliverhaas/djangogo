package templates_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// debugRender renders src with {% debug %} on an engine with Debug set as
// given and the test models registered.
func debugRender(t *testing.T, debug bool, src string, ctx map[string]any) string {
	t.Helper()
	engine := templates.New(templates.MapSource("locmem", map[string]string{"t": src}))
	engine.SetModels(ctxRegistry(t))
	engine.SetDebug(debug)
	out, err := engine.Render("t", ctx)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return out
}

// TestDebugTag ports Django's DebugTests. Go has no sys.modules: the tag
// prints the modules of the build, path and version, in its place.
//
// Ported from Django: tests/template_tests/syntax_tests/test_debug.py::DebugTests.test_non_debug
// Ported from Django: tests/template_tests/syntax_tests/test_debug.py::DebugTests.test_modules
// Ported from Django: tests/template_tests/syntax_tests/test_debug.py::DebugTests.test_plain
// Ported from Django: tests/template_tests/syntax_tests/test_debug.py::DebugTests.test_non_ascii
// Ported from Django: tests/template_tests/syntax_tests/test_debug.py::DebugTests.test_script
func TestDebugTag(t *testing.T) {
	t.Run("test_non_debug", func(t *testing.T) {
		if out := debugRender(t, false, "{% debug %}", nil); out != "" {
			t.Errorf("{%% debug %%} without Debug = %q, want nothing", out)
		}
	})
	t.Run("test_modules", func(t *testing.T) {
		out := debugRender(t, true, "{% debug %}", nil)
		if want := "&#x27;github.com/oliverhaas/djangogo&#x27;: &#x27;"; !strings.Contains(out, want) {
			t.Errorf("{%% debug %%} = %q, want it to contain %q", out, want)
		}
	})
	t.Run("test_plain", func(t *testing.T) {
		out := debugRender(t, true, "{% debug %}", map[string]any{"a": 1})
		want := "{&#x27;a&#x27;: 1}" +
			"{&#x27;False&#x27;: False, &#x27;None&#x27;: None, &#x27;True&#x27;: True}\n\n{"
		if !strings.HasPrefix(out, want) {
			t.Errorf("{%% debug %%} = %q, want it to start with %q", out, want)
		}
	})
	t.Run("test_non_ascii", func(t *testing.T) {
		out := debugRender(t, true, "{% debug %}", map[string]any{"group": ctxAuthor{ID: 1, Name: "清風"}})
		if want := "{&#x27;group&#x27;: &lt;ctxAuthor: 清風&gt;}"; !strings.HasPrefix(out, want) {
			t.Errorf("{%% debug %%} = %q, want it to start with %q", out, want)
		}
	})
	t.Run("test_script", func(t *testing.T) {
		out := debugRender(t, true, "{% debug %}", map[string]any{"frag": "<script>"})
		if want := "{&#x27;frag&#x27;: &#x27;&lt;script&gt;&#x27;}"; !strings.HasPrefix(out, want) {
			t.Errorf("{%% debug %%} = %q, want it to start with %q", out, want)
		}
	})
}

// TestDebugTagInLoop prints the variables of the loop before the ones of
// the render, as Django prints the innermost context first.
//
// No Django test exists for a {% debug %} inside a block.
func TestDebugTagInLoop(t *testing.T) {
	out := debugRender(t, true, "{% for x in items %}{% debug %}{% endfor %}", map[string]any{"items": []any{7}})
	if want := "&#x27;x&#x27;: 7}{&#x27;items&#x27;: [7]}{&#x27;False&#x27;"; !strings.Contains(out, want) {
		t.Errorf("{%% debug %%} = %q, want it to contain %q", out, want)
	}
}
