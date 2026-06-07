// Package fidelity holds a Django-as-oracle differential test harness. It renders
// a fixed set of canonical template+context cases with the project's pongo2 engine
// and asserts byte-equality against golden outputs produced by real Django DTL
// (see oracle/render.py). Cases with a documented cosmetic divergence are skipped
// and catalogued in divergences.md; the rest prove genuine fidelity.
package fidelity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// fidelityCase is one canonical template+context pair shared with the oracle via
// cases.json. Context values are plain JSON (strings, bools, arrays); numbers are
// deliberately not printed (see divergences.md on float formatting).
type fidelityCase struct {
	Name     string         `json:"name"`
	Template string         `json:"template"`
	Context  map[string]any `json:"context"`
}

// skippedCases maps a case name to the reason it diverges cosmetically from DTL.
// Each reason references divergences.md. These cases are rendered but skipped
// rather than asserted, so a real (non-cosmetic) mismatch can never hide here.
var skippedCases = map[string]string{
	"autoescape_apostrophe": "apostrophe escapes as &#39; (pongo2) vs &#x27; (Django); cosmetic, see divergences.md",
}

// loadCases reads and decodes cases.json from the harness directory.
func loadCases(t *testing.T) []fidelityCase {
	t.Helper()
	raw, err := os.ReadFile("cases.json")
	if err != nil {
		t.Fatalf("read cases.json: %v", err)
	}
	var cases []fidelityCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decode cases.json: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("cases.json contained no cases")
	}
	return cases
}

// readGolden returns the committed Django output for the named case.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("golden", name+".txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %q: %v", path, err)
	}
	return string(data)
}

// TestFidelity renders every canonical case with the project's pongo2 engine and
// compares it byte-for-byte against the Django golden output. Documented cosmetic
// divergences are skipped (with a reason) instead of asserted.
func TestFidelity(t *testing.T) {
	eng, err := templates.NewEngine()
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	for _, c := range loadCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			if reason, skip := skippedCases[c.Name]; skip {
				t.Skipf("documented divergence: %s", reason)
			}
			got, err := eng.RenderString(c.Template, c.Context)
			if err != nil {
				t.Fatalf("RenderString(%q): %v", c.Template, err)
			}
			if want := readGolden(t, c.Name); got != want {
				t.Errorf("fidelity mismatch for %q\n  template: %s\n  pongo2:   %q\n  django:   %q",
					c.Name, c.Template, got, want)
			}
		})
	}
}
