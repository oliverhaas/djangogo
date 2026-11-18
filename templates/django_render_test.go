package templates_test

import (
	"testing"

	"github.com/flosch/pongo2/v6"
)

// renderCase is one row of a ported Django template test: the template src,
// rendered under its Django test name next to the named templates it
// includes or extends, with a context, and the output Django renders or that
// rendering fails. Django's @setup runs each test once more with
// string_if_invalid set; only the default engine is ported. A filter test
// that calls the filter function directly is ported as {{ a|filter }} or
// {{ a|filter:b }}.
type renderCase struct {
	name      string
	src       string
	templates map[string]string
	ctx       map[string]any
	want      string
	wantErr   bool
	skip      string
}

// runRenderCases renders each row and compares the output or the failure.
func runRenderCases(t *testing.T, cases []renderCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			named := map[string]string{tc.name: tc.src}
			for k, v := range tc.templates {
				named[k] = v
			}
			got, err := memEngine(named).Render(tc.name, tc.ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("%s rendered %q, want an error", tc.src, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// init registers noop, the filter of Django's "custom" test library that
// some ported tests load: it returns its input and ignores its argument.
func init() {
	if pongo2.FilterExists("noop") {
		return
	}
	err := pongo2.RegisterFilter("noop", func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		return in, nil
	})
	if err != nil {
		panic(err)
	}
}
