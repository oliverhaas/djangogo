package templates_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
)

// querystringCase is one render of a {% querystring %} test: with target
// set, the template renders for a GET request to it through RenderRequest
// with the request context processor; without, through Render.
type querystringCase struct {
	name    string
	src     string
	target  string
	ctx     map[string]any
	want    string
	wantErr string
}

// runQuerystringCases renders each case and compares the output, or checks
// that the error contains wantErr.
func runQuerystringCases(t *testing.T, cases []querystringCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := templates.New(templates.MapSource("locmem", map[string]string{"t": tc.src}))
			var got string
			var err error
			if tc.target == "" {
				got, err = engine.Render("t", tc.ctx)
			} else {
				engine.AddContextProcessor(templates.RequestProcessor)
				got, err = engine.RenderRequest(httptest.NewRequest(http.MethodGet, tc.target, nil), "t", tc.ctx)
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("%s: got %q, %v; want an error with %q", tc.src, got, err, tc.wantErr)
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

// TestQuerystringTag ports Django's QueryStringTagTests. A QueryDict is a
// url.Values.
//
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_empty_get_params
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_remove_all_params
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_remove_all_params_custom_querydict
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_non_empty_get_params
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_multiple
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_empty_params
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_replace
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_add
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_remove
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_remove_nonexistent
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_remove_from_dict
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_variable
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_dict
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_dict_list_values
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_multiple_args_override
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_request_get_ignored
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_same_arg
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_non_mapping_args
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_non_string_dict_keys
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_add_list
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_add_dict
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_with_explicit_query_dict
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_with_explicit_query_dict_and_no_request
// Ported from Django: tests/template_tests/syntax_tests/test_querystring.py::QueryStringTagTests.test_querystring_without_request_or_explicit_query_dict
func TestQuerystringTag(t *testing.T) {
	queryDict := func(q string) url.Values {
		v, err := url.ParseQuery(q)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	nonMapping := "querystring requires mappings for positional arguments (got %s instead)."
	runQuerystringCases(t, []querystringCase{
		{name: "test_querystring_empty_get_params", src: "{% querystring %}", target: "/", want: "?"},
		{name: "test_querystring_remove_all_params/non_empty", src: "{% querystring a=None %}", target: "/?a=b", want: "?"},
		{name: "test_querystring_remove_all_params/empty", src: "{% querystring a=None %}", target: "/", want: "?"},
		{
			name: "test_querystring_remove_all_params_custom_querydict",
			src:  "{% querystring my_query_dict my_dict a=None %}",
			ctx:  map[string]any{"my_query_dict": queryDict("a=1&b=2"), "my_dict": map[string]any{"b": nil}},
			want: "?",
		},
		{name: "test_querystring_non_empty_get_params", src: "{% querystring %}", target: "/?a=b", want: "?a=b"},
		{name: "test_querystring_multiple", src: "{% querystring %}", target: "/?x=y&a=b", want: "?x=y&amp;a=b"},
		{name: "test_querystring_empty_params/dict", src: "{% querystring qd %}", target: "/", ctx: map[string]any{"qd": map[string]any{}}, want: "?"},
		{name: "test_querystring_empty_params/dict_qs", src: "{% querystring qd %}", target: "/?a=b", ctx: map[string]any{"qd": map[string]any{}}, want: "?"},
		{name: "test_querystring_empty_params/querydict", src: "{% querystring qd %}", target: "/", ctx: map[string]any{"qd": url.Values{}}, want: "?"},
		{name: "test_querystring_empty_params/querydict_qs", src: "{% querystring qd %}", target: "/?a=b", ctx: map[string]any{"qd": url.Values{}}, want: "?"},
		{name: "test_querystring_replace", src: "{% querystring a=1 %}", target: "/?x=y&a=b", want: "?x=y&amp;a=1"},
		{name: "test_querystring_add", src: "{% querystring test_new='something' %}", target: "/?a=b", want: "?a=b&amp;test_new=something"},
		{name: "test_querystring_remove", src: "{% querystring test=None a=1 %}", target: "/?test=value&a=1", want: "?a=1"},
		{name: "test_querystring_remove_nonexistent", src: "{% querystring nonexistent=None a=1 %}", target: "/?x=y&a=1", want: "?x=y&amp;a=1"},
		{
			name: "test_querystring_remove_from_dict", src: "{% querystring my_dict a=1 %}", target: "/?test=value",
			ctx: map[string]any{"my_dict": map[string]any{"test": nil}}, want: "?a=1",
		},
		{name: "test_querystring_variable", src: "{% querystring a=a %}", target: "/", ctx: map[string]any{"a": 1}, want: "?a=1"},
		{name: "test_querystring_dict", src: "{% querystring my_dict %}", ctx: map[string]any{"my_dict": map[string]any{"a": 1}}, want: "?a=1"},
		{
			name: "test_querystring_dict_list_values", src: "{% querystring my_dict %}",
			ctx: map[string]any{"my_dict": map[string]any{"a": []any{1, 2}}}, want: "?a=1&amp;a=2",
		},
		{
			name: "test_querystring_multiple_args_override",
			src:  "{% querystring my_dict my_query_dict x=3 y=None %}",
			ctx:  map[string]any{"my_dict": map[string]any{"x": 0, "y": 42}, "my_query_dict": queryDict("a=1&b=2")},
			want: "?x=3&amp;a=1&amp;b=2",
		},
		{
			name: "test_querystring_request_get_ignored/mapping", src: "{% querystring my_mapping %}", target: "/?x=y&a=b",
			ctx: map[string]any{"my_mapping": map[string]any{"y": "x"}}, want: "?y=x",
		},
		{
			name: "test_querystring_request_get_ignored/empty", src: "{% querystring my_mapping %}", target: "/?x=y&a=b",
			ctx: map[string]any{"my_mapping": map[string]any{}}, want: "?",
		},
		{
			name: "test_querystring_same_arg", src: "{% querystring a=1 a=2 %}",
			wantErr: "'querystring' received multiple values for keyword argument 'a'",
		},
		{
			name: "test_querystring_non_mapping_args/none", src: "{% querystring somevar %}", target: "/",
			ctx: map[string]any{"somevar": nil}, wantErr: strings.Replace(nonMapping, "%s", "None", 1),
		},
		{
			name: "test_querystring_non_mapping_args/zero", src: "{% querystring somevar %}", target: "/",
			ctx: map[string]any{"somevar": 0}, wantErr: strings.Replace(nonMapping, "%s", "0", 1),
		},
		{
			name: "test_querystring_non_mapping_args/empty_string", src: "{% querystring somevar %}", target: "/",
			ctx: map[string]any{"somevar": ""}, wantErr: strings.Replace(nonMapping, "%s", "''", 1),
		},
		{
			name: "test_querystring_non_mapping_args/empty_list", src: "{% querystring somevar %}", target: "/",
			ctx: map[string]any{"somevar": []any{}}, wantErr: strings.Replace(nonMapping, "%s", "[]", 1),
		},
		{
			name: "test_querystring_non_string_dict_keys", src: "{% querystring my_dict %}",
			ctx:     map[string]any{"my_dict": map[int]int{0: 1}},
			wantErr: "querystring requires strings for mapping keys (got 0 instead).",
		},
		{name: "test_querystring_add_list", src: "{% querystring a=my_list %}", target: "/", ctx: map[string]any{"my_list": []any{2, 3}}, want: "?a=2&amp;a=3"},
		{
			name: "test_querystring_add_dict", src: "{% querystring a=my_dict %}", target: "/",
			ctx: map[string]any{"my_dict": map[int]int{0: 0, 1: 2, 2: 4}}, want: "?a=0&amp;a=1&amp;a=2",
		},
		{name: "test_querystring_with_explicit_query_dict", src: "{% querystring request.GET a=2 %}", target: "/?a=1", want: "?a=2"},
		{
			name: "test_querystring_with_explicit_query_dict_and_no_request", src: "{% querystring my_query_dict a=2 %}",
			ctx: map[string]any{"my_query_dict": queryDict("a=1&b=2")}, want: "?a=2&amp;b=2",
		},
		{name: "test_querystring_without_request_or_explicit_query_dict", src: "{% querystring %}", wantErr: "querystring"},
	})
}

// TestQuerystringTagAs stores the query string with "as" and keeps the last
// value of a repeated GET parameter, as Django's QueryDict.items() does.
//
// No Django test exists for these two.
func TestQuerystringTagAs(t *testing.T) {
	runQuerystringCases(t, []querystringCase{
		{name: "as", src: `{% querystring page=2 as next %}<a href="{{ next }}">`, target: "/?q=a+b", want: `<a href="?q=a+b&amp;page=2">`},
		{name: "repeated_key", src: "{% querystring %}", target: "/?a=1&b=2&a=3", want: "?a=3&amp;b=2"},
		{name: "positional_after_keyword", src: "{% querystring a=1 my_dict %}", wantErr: "positional argument(s) after some keyword argument(s)"},
	})
}
