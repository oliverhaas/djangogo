// Package fidelity holds a Django-as-oracle differential test harness. It renders
// a fixed set of canonical template+context cases with the project's pongo2 engine
// and asserts byte-equality against golden outputs produced by real Django DTL
// (see oracle/render.py). Cases with a documented divergence are skipped and
// catalogued in divergences.md; the rest prove genuine fidelity.
package fidelity

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/templates"
)

// fidelityCase is one canonical template+context pair shared with the oracle via
// cases.json. Templates holds the named templates an {% include %} or
// {% extends %} of the case refers to. Context values are plain JSON, decoded by
// decodeValue.
type fidelityCase struct {
	Name      string            `json:"name"`
	Template  string            `json:"template"`
	Context   map[string]any    `json:"context"`
	Templates map[string]string `json:"templates"`
}

// skippedCases maps a case name to the reason it diverges from DTL. Each one is
// described in divergences.md. These cases are skipped rather than asserted,
// so a mismatch nobody knows about can never hide here.
var skippedCases = map[string]string{
	"print_float": "a bare float prints with six decimals (pongo2) vs the shortest form (Django)",
	"print_none":  "a nil value prints empty (pongo2) vs None (Django)",
	"print_list":  "a list prints in Go syntax vs Python's repr",
	"lorem_words": "pongo2's lorem words start with a capital letter and keep punctuation",
}

// loadCases reads and decodes cases.json from the harness directory.
func loadCases(t *testing.T) []fidelityCase {
	t.Helper()
	raw, err := os.ReadFile("cases.json")
	if err != nil {
		t.Fatalf("read cases.json: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	// Without UseNumber every JSON number would become a float64 and the
	// integer cases would print as floats.
	dec.UseNumber()
	var cases []fidelityCase
	if err := dec.Decode(&cases); err != nil {
		t.Fatalf("decode cases.json: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("cases.json contained no cases")
	}
	for i := range cases {
		ctx, _ := decodeValue(t, cases[i].Context).(map[string]any)
		cases[i].Context = ctx
	}
	return cases
}

// decodeValue turns a decoded JSON value into the Go value the oracle's
// Python counterpart stands for: integers stay integers, and the two object
// conventions of oracle/render.py become a time.Time and a safe string.
func decodeValue(t *testing.T, v any) any {
	t.Helper()
	switch x := v.(type) {
	case json.Number:
		if !strings.ContainsAny(x.String(), ".eE") {
			n, err := x.Int64()
			if err != nil {
				t.Fatalf("decode integer %q: %v", x, err)
			}
			return int(n)
		}
		f, err := x.Float64()
		if err != nil {
			t.Fatalf("decode float %q: %v", x, err)
		}
		return f
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = decodeValue(t, item)
		}
		return out
	case map[string]any:
		if len(x) == 1 {
			if s, ok := x["__datetime__"].(string); ok {
				tm, err := time.Parse("2006-01-02T15:04:05", s)
				if err != nil {
					t.Fatalf("decode datetime %q: %v", s, err)
				}
				return tm
			}
			if s, ok := x["__safe__"].(string); ok {
				return templates.MarkSafe(s)
			}
		}
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = decodeValue(t, item)
		}
		return out
	}
	return v
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
// compares it byte-for-byte against the Django golden output. Documented
// divergences are skipped (with a reason) instead of asserted.
func TestFidelity(t *testing.T) {
	for _, c := range loadCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			if reason, skip := skippedCases[c.Name]; skip {
				t.Skipf("documented divergence: %s", reason)
			}
			// A panic in one case must not hide the results of the others.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("render of %q panicked: %v", c.Template, r)
				}
			}()
			eng := templates.New(templates.MapSource("case", c.Templates))
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

// TestSkippedCasesExist keeps the skip list honest: every entry must name a
// case, so a renamed or removed case cannot leave a stale excuse behind.
func TestSkippedCasesExist(t *testing.T) {
	names := make(map[string]bool)
	for _, c := range loadCases(t) {
		names[c.Name] = true
	}
	for name := range skippedCases {
		if !names[name] {
			t.Errorf("skippedCases names %q, which is not a case", name)
		}
	}
}
