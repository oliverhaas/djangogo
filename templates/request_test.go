package templates_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/templates"
)

func newRequestEngine(t *testing.T) *templates.Engine {
	t.Helper()
	eng, err := templates.NewEngineFS(fstest.MapFS{
		"hello.html": {Data: []byte("{{ greeting }}, {{ name }} via {{ path }}")},
	})
	if err != nil {
		t.Fatalf("NewEngineFS: %v", err)
	}
	return eng
}

func TestRenderRequestMergesContextProcessors(t *testing.T) {
	eng := newRequestEngine(t)
	eng.AddContextProcessor(
		func(r *http.Request) map[string]any {
			return map[string]any{"path": r.URL.Path, "greeting": "from-processor", "name": "p1"}
		},
		func(*http.Request) map[string]any { return map[string]any{"name": "p2"} },
	)
	r := httptest.NewRequest(http.MethodGet, "/x/", nil)

	out, err := eng.RenderRequest(r, "hello.html", map[string]any{"greeting": "Hi"})
	if err != nil {
		t.Fatalf("RenderRequest: %v", err)
	}
	// data overrides processors; a later processor overrides an earlier one.
	if want := "Hi, p2 via /x/"; out != want {
		t.Errorf("RenderRequest = %q, want %q", out, want)
	}
}

func TestRenderRequestRecordsTemplateAndContext(t *testing.T) {
	eng := newRequestEngine(t)
	eng.AddContextProcessor(func(*http.Request) map[string]any { return map[string]any{"path": "/p/"} })

	rec := &templates.Recorder{}
	r := httptest.NewRequest(http.MethodGet, "/x/", nil)
	r = r.WithContext(templates.WithRecorder(r.Context(), rec))

	if _, err := eng.RenderRequest(r, "hello.html", map[string]any{"name": "World"}); err != nil {
		t.Fatalf("RenderRequest: %v", err)
	}
	got := rec.Rendered()
	if len(got) != 1 {
		t.Fatalf("recorded %d renders, want 1", len(got))
	}
	if got[0].Name != "hello.html" {
		t.Errorf("recorded name = %q, want hello.html", got[0].Name)
	}
	if got[0].Context["name"] != "World" || got[0].Context["path"] != "/p/" {
		t.Errorf("recorded context = %v", got[0].Context)
	}
}

func TestRenderRequestRejectsNilRequestAndSkipsFailedRenders(t *testing.T) {
	eng := newRequestEngine(t)
	if _, err := eng.RenderRequest(nil, "hello.html", nil); err == nil {
		t.Error("RenderRequest(nil request) returned no error")
	}
	rec := &templates.Recorder{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(templates.WithRecorder(r.Context(), rec))
	if _, err := eng.RenderRequest(r, "missing.html", nil); err == nil {
		t.Error("RenderRequest(missing template) returned no error")
	}
	if n := len(rec.Rendered()); n != 0 {
		t.Errorf("failed render was recorded (%d entries)", n)
	}
}
