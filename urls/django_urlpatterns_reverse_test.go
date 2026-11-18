package urls_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/oliverhaas/djangogo/urls"
)

// viewClass stands in for Django's class-based view instance: a handler that
// can be compared by identity.
type viewClass struct{}

// ServeHTTP answers with an empty 200.
func (*viewClass) ServeHTTP(http.ResponseWriter, *http.Request) {}

// viewClassInstance is the one instance the URLconfs below share.
var viewClassInstance = &viewClass{}

// noReverse marks a table row that expects NoReverseMatch.
const noReverse = "NoReverseMatch"

// reverseCase is one row of the reverse tables Django's tests are written as.
type reverseCase struct {
	name       string
	args       []any
	kwargs     urls.Kwargs
	currentApp string
	want       string
}

// runReverseCases reverses every row and compares the URL, or checks for
// NoReverseMatch when the row expects it.
func runReverseCases(t *testing.T, router *urls.Router, cases []reverseCase) {
	t.Helper()
	for _, c := range cases {
		got, err := router.ReverseWith(c.name, urls.ReverseOptions{Args: c.args, Kwargs: c.kwargs, CurrentApp: c.currentApp})
		if c.want == noReverse {
			if !errors.Is(err, urls.ErrNoReverseMatch) {
				t.Errorf("Reverse(%q, %v, %v) = %q, %v; want NoReverseMatch", c.name, c.args, c.kwargs, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("Reverse(%q, %v, %v, app %q): %v", c.name, c.args, c.kwargs, c.currentApp, err)
			continue
		}
		if got != c.want {
			t.Errorf("Reverse(%q, %v, %v, app %q) = %q, want %q", c.name, c.args, c.kwargs, c.currentApp, got, c.want)
		}
	}
}

// revIncludedURLs is tests/urlpatterns_reverse/included_urls.py.
func revIncludedURLs() []urls.Route {
	return []urls.Route{
		urls.Path("", emptyView, "inner-nothing"),
		urls.RePath(`extra/(?P<extra>\w+)/$`, emptyView, "inner-extra"),
		urls.RePath(`(?P<one>[0-9]+)|(?P<two>[0-9]+)/$`, emptyView, "inner-disjunction"),
	}
}

// revIncludedURLs2 is tests/urlpatterns_reverse/included_urls2.py.
func revIncludedURLs2() []urls.Route {
	return []urls.Route{
		urls.RePath(`^part/(?P<value>\w+)/$`, emptyView, "part"),
		urls.RePath(`^part2/(?:(?P<value>\w+)/)?$`, emptyView, "part2"),
	}
}

// revExtraURLs is tests/urlpatterns_reverse/extra_urls.py.
func revExtraURLs() []urls.Route {
	return slices.Concat(
		[]urls.Route{
			urls.RePath(`^e-places/([0-9]+)/$`, emptyView, "extra-places"),
			urls.RePath(`^e-people/(?P<name>\w+)/$`, emptyView, "extra-people"),
		},
		urls.Include("", "", revIncludedURLs2()...),
		urls.IncludeWith(urls.IncludeOptions{Prefix: `^prefix/(?P<prefix>\w+)/`, Regex: true}, revIncludedURLs2()...),
	)
}

// reverseURLs is tests/urlpatterns_reverse/urls.py. Go's regexp package has
// neither backreferences nor lookaround, so the routes people_backref,
// lookahead-negative, lookahead-positive, lookbehind-negative and
// lookbehind-positive are left out; a router does not build with them.
var reverseURLs = sync.OnceValue(func() *urls.Router {
	re := func(expr, name string) urls.Route { return urls.RePath(expr, emptyView, name) }
	return urls.NewRouter(slices.Concat([]urls.Route{
		re(`^places/([0-9]+)/$`, "places"),
		re(`^places?/$`, "places?"),
		re(`^places+/$`, "places+"),
		re(`^places*/$`, "places*"),
		re(`^(?:places/)?$`, "places2?"),
		re(`^(?:places/)+$`, "places2+"),
		re(`^(?:places/)*$`, "places2*"),
		re(`^places/([0-9]+|[a-z_]+)/`, "places3"),
		re(`^places/(?P<id>[0-9]+)/$`, "places4"),
		re(`^people/(?P<name>\w+)/$`, "people"),
		re(`^people/(?:name/)$`, "people2"),
		re(`^people/(?:name/(\w+)/)?$`, "people2a"),
		re(`^optional/(?P<name>.*)/(?:.+/)?`, "optional"),
		re(`^optional/(?P<arg1>\d+)/(?:(?P<arg2>\d+)/)?`, "named_optional"),
		re(`^optional/(?P<arg1>\d+)/(?:(?P<arg2>\d+)/)?$`, "named_optional_terminated"),
		re(`^nested/noncapture/(?:(?P<p>\w+))$`, "nested-noncapture"),
		re(`^nested/capture/((\w+)/)?$`, "nested-capture"),
		re(`^nested/capture/mixed/((?P<p>\w+))$`, "nested-mixedcapture"),
		re(`^nested/capture/named/(?P<outer>(?P<inner>\w+)/)?$`, "nested-namedcapture"),
		re(`^hardcoded/$`, "hardcoded"),
		re(`^hardcoded/doc\.pdf$`, "hardcoded2"),
		re(`^people/(?P<state>\w\w)/(?P<name>\w+)/$`, "people3"),
		re(`^people/(?P<state>\w\w)/(?P<name>[0-9])/$`, "people4"),
		re(`^people/((?P<state>\w\w)/test)?/(\w+)/$`, "people6"),
		re(`^character_set/[abcdef0-9]/$`, "range"),
		re(`^character_set/[\w]/$`, "range2"),
		re(`^price/\$([0-9]+)/$`, "price"),
		re(`^price/[$]([0-9]+)/$`, "price2"),
		re(`^price/[\$]([0-9]+)/$`, "price3"),
		re(`^product/(?P<product>\w+)\+\(\$(?P<price>[0-9]+(\.[0-9]+)?)\)/$`, "product"),
		re(`^headlines/(?P<year>[0-9]+)\.(?P<month>[0-9]+)\.(?P<day>[0-9]+)/$`, "headlines"),
		re(`^windows_path/(?P<drive_name>[A-Z]):\\(?P<path>.+)/$`, "windows"),
		re(`^special_chars/(?P<chars>.+)/$`, "special"),
		re(`^resolved/(?P<arg>\d+)/$`, "resolved").WithKwargs(urls.Kwargs{"extra": true}),
		re(`^resolved-overridden/(?P<arg>\d+)/(?P<overridden>\w+)/$`, "resolved-overridden").
			WithKwargs(urls.Kwargs{"extra": true, "overridden": "default"}),
		re(`^(?P<name>.+)/[0-9]+/$`, "mixed"),
		re(`^repeats/a{1,2}/$`, "repeats"),
		re(`^repeats/a{2,4}/$`, "repeats2"),
		re(`^repeats/a{2}/$`, "repeats3"),
		re(`^test/1/?`, "test"),
	},
		urls.IncludeWith(urls.IncludeOptions{Prefix: `^outer/(?P<outer>[0-9]+)/`, Regex: true}, revIncludedURLs()...),
		urls.IncludeWith(
			urls.IncludeOptions{Prefix: `^outer-no-kwargs/([0-9]+)/`, Regex: true},
			urls.RePath(`^inner-no-kwargs/([0-9]+)/$`, emptyView, "inner-no-kwargs")),
		urls.IncludeWith(urls.IncludeOptions{Prefix: "", Regex: true}, revExtraURLs()...),
		[]urls.Route{
			urls.Path("partial/", emptyView, "partial"),
			urls.Path("partial_nested/", emptyView, "partial_nested"),
			urls.Path("partial_wrapped/", emptyView, "partial_wrapped"),
			// Not reversible, but building the router must cope with it.
			re(`^(?:foo|bar)(\w+)/$`, "disjunction"),
			urls.Path("absolute_arg_view/", emptyView, ""),
			re(`^defaults_view1/(?P<arg1>[0-9]+)/$`, "defaults").WithKwargs(urls.Kwargs{"arg2": 1}),
			re(`^defaults_view2/(?P<arg1>[0-9]+)/$`, "defaults").WithKwargs(urls.Kwargs{"arg2": 2}),
		},
		urls.Include("includes/", "",
			urls.Path("non_path_include/", emptyView, "non_path_include"),
			urls.Path("nested_path/", emptyView, "")),
		[]urls.Route{
			re(`(.+)/security/$`, "security"),
			urls.Path("hello/<slug:name>/", emptyView, ""),
		},
	)...)
})

// TestURLPatternReverse runs Django's table of reverse lookups against the
// port of its URLconf. The rows for people_backref are left out: the route
// needs a backreference, which Go's regexp package does not have.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_urlpattern_reverse
func TestURLPatternReverse(t *testing.T) {
	kw := func(pairs ...any) urls.Kwargs {
		m := urls.Kwargs{}
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i].(string)] = pairs[i+1]
		}
		return m
	}
	a := func(v ...any) []any { return v }
	cases := []reverseCase{
		{name: "places", want: "/places/3/", args: a(3)},
		{name: "places", want: "/places/3/", args: a("3")},
		{name: "places", want: noReverse, args: a("a")},
		{name: "places", want: noReverse},
		{name: "places?", want: "/place/"},
		{name: "places+", want: "/places/"},
		{name: "places*", want: "/place/"},
		{name: "places2?", want: "/"},
		{name: "places2+", want: "/places/"},
		{name: "places2*", want: "/"},
		{name: "places3", want: "/places/4/", args: a(4)},
		{name: "places3", want: "/places/harlem/", args: a("harlem")},
		{name: "places3", want: noReverse, args: a("harlem64")},
		{name: "places4", want: "/places/3/", kwargs: kw("id", 3)},
		{name: "people", want: noReverse},
		{name: "people", want: "/people/adrian/", args: a("adrian")},
		{name: "people", want: "/people/adrian/", kwargs: kw("name", "adrian")},
		{name: "people", want: noReverse, args: a("name with spaces")},
		{name: "people", want: noReverse, kwargs: kw("name", "name with spaces")},
		{name: "people2", want: "/people/name/"},
		{name: "people2a", want: "/people/name/fred/", args: a("fred")},
		{name: "optional", want: "/optional/fred/", kwargs: kw("name", "fred")},
		{name: "optional", want: "/optional/fred/", args: a("fred")},
		{name: "named_optional", want: "/optional/1/", args: a(1)},
		{name: "named_optional", want: "/optional/1/", kwargs: kw("arg1", 1)},
		{name: "named_optional", want: "/optional/1/2/", args: a(1, 2)},
		{name: "named_optional", want: "/optional/1/2/", kwargs: kw("arg1", 1, "arg2", 2)},
		{name: "named_optional_terminated", want: "/optional/1/", args: a(1)},
		{name: "named_optional_terminated", want: "/optional/1/", kwargs: kw("arg1", 1)},
		{name: "named_optional_terminated", want: "/optional/1/2/", args: a(1, 2)},
		{name: "named_optional_terminated", want: "/optional/1/2/", kwargs: kw("arg1", 1, "arg2", 2)},
		{name: "hardcoded", want: "/hardcoded/"},
		{name: "hardcoded2", want: "/hardcoded/doc.pdf"},
		{name: "people3", want: "/people/il/adrian/", kwargs: kw("state", "il", "name", "adrian")},
		{name: "people3", want: noReverse, kwargs: kw("state", "il")},
		{name: "people3", want: noReverse, kwargs: kw("name", "adrian")},
		{name: "people4", want: noReverse, kwargs: kw("state", "il", "name", "adrian")},
		{name: "people6", want: "/people/il/test/adrian/", args: a("il/test", "adrian")},
		{name: "people6", want: "/people//adrian/", args: a("adrian")},
		{name: "range", want: "/character_set/a/"},
		{name: "range2", want: "/character_set/x/"},
		{name: "price", want: "/price/$10/", args: a("10")},
		{name: "price2", want: "/price/$10/", args: a("10")},
		{name: "price3", want: "/price/$10/", args: a("10")},
		{name: "product", want: "/product/chocolate+($2.00)/", kwargs: kw("price", "2.00", "product", "chocolate")},
		{name: "headlines", want: "/headlines/2007.5.21/", kwargs: kw("year", 2007, "month", 5, "day", 21)},
		{
			name: "windows", want: `/windows_path/C:%5CDocuments%20and%20Settings%5Cspam/`,
			kwargs: kw("drive_name", "C", "path", `Documents and Settings\spam`),
		},
		{name: "special", want: `/special_chars/~@+%5C$*%7C/`, args: a(`~@+\$*|`)},
		{name: "special", want: `/special_chars/some%20resource/`, args: a(`some resource`)},
		{name: "special", want: `/special_chars/10%25%20complete/`, args: a(`10% complete`)},
		{name: "special", want: `/special_chars/some%20resource/`, kwargs: kw("chars", `some resource`)},
		{name: "special", want: `/special_chars/10%25%20complete/`, kwargs: kw("chars", `10% complete`)},
		{name: "special", want: noReverse, args: a("")},
		{name: "mixed", want: "/john/0/", kwargs: kw("name", "john")},
		{name: "repeats", want: "/repeats/a/"},
		{name: "repeats2", want: "/repeats/aa/"},
		{name: "repeats3", want: "/repeats/aa/"},
		{name: "test", want: "/test/1"},
		{name: "inner-nothing", want: "/outer/42/", kwargs: kw("outer", "42")},
		{name: "inner-nothing", want: "/outer/42/", args: a("42")},
		{name: "inner-nothing", want: noReverse, args: a("foo")},
		{name: "inner-extra", want: "/outer/42/extra/inner/", kwargs: kw("extra", "inner", "outer", "42")},
		{name: "inner-extra", want: "/outer/42/extra/inner/", args: a("42", "inner")},
		{name: "inner-extra", want: noReverse, args: a("fred", "inner")},
		{name: "inner-no-kwargs", want: "/outer-no-kwargs/42/inner-no-kwargs/1/", args: a("42", "1")},
		{name: "disjunction", want: noReverse, args: a("foo")},
		{name: "inner-disjunction", want: noReverse, args: a("10", "11")},
		{name: "extra-places", want: "/e-places/10/", args: a("10")},
		{name: "extra-people", want: "/e-people/fred/", args: a("fred")},
		{name: "extra-people", want: "/e-people/fred/", kwargs: kw("name", "fred")},
		{name: "part", want: "/part/one/", kwargs: kw("value", "one")},
		{name: "part", want: "/prefix/xx/part/one/", kwargs: kw("value", "one", "prefix", "xx")},
		{name: "part2", want: "/part2/one/", kwargs: kw("value", "one")},
		{name: "part2", want: "/part2/"},
		{name: "part2", want: "/prefix/xx/part2/one/", kwargs: kw("value", "one", "prefix", "xx")},
		{name: "part2", want: "/prefix/xx/part2/", kwargs: kw("prefix", "xx")},
		// Nested groups only reverse when just the outer group is supplied.
		{name: "nested-noncapture", want: "/nested/noncapture/opt", kwargs: kw("p", "opt")},
		{name: "nested-capture", want: "/nested/capture/opt/", args: a("opt/")},
		{name: "nested-capture", want: noReverse, kwargs: kw("p", "opt")},
		{name: "nested-mixedcapture", want: "/nested/capture/mixed/opt", args: a("opt")},
		{name: "nested-mixedcapture", want: noReverse, kwargs: kw("p", "opt")},
		{name: "nested-namedcapture", want: "/nested/capture/named/opt/", kwargs: kw("outer", "opt/")},
		{name: "nested-namedcapture", want: noReverse, kwargs: kw("outer", "opt/", "inner", "opt")},
		{name: "nested-namedcapture", want: noReverse, kwargs: kw("inner", "opt")},
		{name: "non_path_include", want: "/includes/non_path_include/"},
		// Django ticket 13154: routes that differ only in their default arguments.
		{name: "defaults", want: "/defaults_view1/3/", kwargs: kw("arg1", 3, "arg2", 1)},
		{name: "defaults", want: "/defaults_view2/3/", kwargs: kw("arg1", 3, "arg2", 2)},
		{name: "defaults", want: noReverse, kwargs: kw("arg1", 3, "arg2", 3)},
		{name: "defaults", want: noReverse, kwargs: kw("arg2", 1)},
		// A leading slash in an argument must not produce a scheme-relative URL.
		{name: "security", want: "/%2Fexample.com/security/", args: a("/example.com")},
	}
	runReverseCases(t, reverseURLs(), cases)
}

// TestReverseNone checks that an empty name finds nothing instead of the last
// unnamed route.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_none
func TestReverseNone(t *testing.T) {
	if _, err := reverseURLs().Reverse(""); !errors.Is(err, urls.ErrNoReverseMatch) {
		t.Fatalf("Reverse(\"\") error = %v, want NoReverseMatch", err)
	}
}

// TestMixingArgsAndKwargs checks that positional and keyword arguments cannot
// be combined.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_mixing_args_and_kwargs
func TestMixingArgsAndKwargs(t *testing.T) {
	_, err := reverseURLs().ReverseWith("name", urls.ReverseOptions{Args: []any{"a"}, Kwargs: urls.Kwargs{"b": "c"}})
	if !errors.Is(err, urls.ErrMixedArgs) {
		t.Fatalf("error = %v, want ErrMixedArgs", err)
	}
	if got, want := err.Error(), "Don't mix *args and **kwargs in call to reverse()!"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestReverseMessages checks the texts of NoReverseMatch.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_patterns_reported
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_view_not_found_message
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_no_args_message
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_illegal_args_message
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_illegal_kwargs_message
func TestReverseMessages(t *testing.T) {
	tests := []struct {
		django string
		name   string
		opts   urls.ReverseOptions
		want   string
	}{
		{"test_patterns_reported", "people", urls.ReverseOptions{}, `1 pattern(s) tried: ['people/(?P<name>\\w+)/$']`},
		{
			"test_view_not_found_message", "nonexistent-view",
			urls.ReverseOptions{},
			"Reverse for 'nonexistent-view' not found. 'nonexistent-view' is not a valid view function or pattern name.",
		},
		{
			"test_no_args_message", "places",
			urls.ReverseOptions{},
			"Reverse for 'places' with no arguments not found. 1 pattern(s) tried:",
		},
		{
			"test_illegal_args_message", "places",
			urls.ReverseOptions{Args: []any{1, 2}},
			"Reverse for 'places' with arguments '(1, 2)' not found. 1 pattern(s) tried:",
		},
		{
			"test_illegal_kwargs_message", "places",
			urls.ReverseOptions{Kwargs: urls.Kwargs{"arg1": 2}},
			"Reverse for 'places' with keyword arguments '{'arg1': 2}' not found. 1 pattern(s) tried:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.django, func(t *testing.T) {
			_, err := reverseURLs().ReverseWith(tt.name, tt.opts)
			var nrm *urls.NoReverseMatch
			if !errors.As(err, &nrm) {
				t.Fatalf("error = %v, want *NoReverseMatch", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("message = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestReverseWithQueryAndFragment checks the query string and the fragment of
// a reversed URL. The query is a url.Values, which has no order of its own, so
// the keys come out sorted where Django keeps the order of its dict; the
// expected strings are the Go ones.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_with_query
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_with_query_sequences
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_with_fragment
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_with_fragment_not_encoded
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_with_query_and_fragment
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_with_empty_fragment
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_with_empty_query
// Ported from Django: tests/urlpatterns_reverse/tests.py::URLPatternReverse.test_reverse_encodes_query_string
func TestReverseWithQueryAndFragment(t *testing.T) {
	tests := []struct {
		django string
		opts   urls.ReverseOptions
		want   string
	}{
		{
			"test_reverse_with_query",
			urls.ReverseOptions{Query: url.Values{"hello": {"world"}, "foo": {"123"}}},
			"/test/1?foo=123&hello=world",
		},
		{
			"test_reverse_with_query_sequences",
			urls.ReverseOptions{Query: url.Values{"hello": {"world"}, "foo": {"123", "456"}}},
			"/test/1?foo=123&foo=456&hello=world",
		},
		{"test_reverse_with_fragment", urls.ReverseOptions{Fragment: "tab-1"}, "/test/1#tab-1"},
		{
			"test_reverse_with_fragment_not_encoded",
			urls.ReverseOptions{Fragment: "tab 1 is the best!"},
			"/test/1#tab 1 is the best!",
		},
		{
			"test_reverse_with_query_and_fragment",
			urls.ReverseOptions{Query: url.Values{"hello": {"world"}, "foo": {"123"}}, Fragment: "tab-1"},
			"/test/1?foo=123&hello=world#tab-1",
		},
		// Go has no None: the empty fragment means "no fragment", so Django's
		// "/test/1#" for fragment="" cannot be asked for.
		{"test_reverse_with_empty_fragment", urls.ReverseOptions{Fragment: ""}, "/test/1"},
		{"test_reverse_with_empty_query", urls.ReverseOptions{Query: url.Values{}}, "/test/1"},
		{
			"test_reverse_encodes_query_string",
			urls.ReverseOptions{Query: url.Values{
				"hello world": {"django project"},
				"foo":         {"123", "456"},
				"@invalid":    {"?", "!", "a b"},
			}},
			"/test/1?%40invalid=%3F&%40invalid=%21&%40invalid=a+b&foo=123&foo=456&hello+world=django+project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.django, func(t *testing.T) {
			got, err := reverseURLs().ReverseWith("test", tt.opts)
			if err != nil {
				t.Fatalf("ReverseWith: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// namedURLs is tests/urlpatterns_reverse/named_urls.py with the two URLconfs
// it includes.
func namedURLs() *urls.Router {
	disjunction := `^(?P<one>[0-9]+)|(?P<two>[0-9]+)/$`
	inner2 := []urls.Route{
		urls.Path("", emptyView, "named-url5"),
		urls.RePath(`^extra/(?P<extra>\w+)/$`, emptyView, "named-url6"),
		urls.RePath(disjunction, emptyView, ""),
	}
	inner := slices.Concat([]urls.Route{
		urls.Path("", emptyView, "named-url3"),
		urls.RePath(`^extra/(?P<extra>\w+)/$`, emptyView, "named-url4"),
		urls.RePath(disjunction, emptyView, ""),
	}, urls.Include("included/", "", inner2...))
	return urls.NewRouter(slices.Concat([]urls.Route{
		urls.Path("", emptyView, "named-url1"),
		urls.RePath(`^extra/(?P<extra>\w+)/$`, emptyView, "named-url2"),
		urls.RePath(disjunction, emptyView, ""),
	}, urls.Include("included/", "", inner...))...)
}

// TestResolverReverse checks reversing on a router. Django's resolver.reverse
// returns the URL without the leading slash; Router.Reverse returns the URL
// as it is requested, so the expected values carry the slash.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ResolverTests.test_resolver_reverse
func TestResolverReverse(t *testing.T) {
	runReverseCases(t, namedURLs(), []reverseCase{
		{name: "named-url1", want: "/"},
		{name: "named-url2", args: []any{"arg"}, want: "/extra/arg/"},
		{name: "named-url2", kwargs: urls.Kwargs{"extra": "arg"}, want: "/extra/arg/"},
	})
}

// TestResolverReverseConflict checks that routes may share a name and that the
// last one declared wins among those that fit the arguments.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ResolverTests.test_resolver_reverse_conflict
func TestResolverReverseConflict(t *testing.T) {
	const name = "name-conflict"
	router := urls.NewRouter(
		urls.Path("conflict/cannot-go-here/", emptyView, name),
		urls.Path("conflict/", emptyView, name),
		urls.RePath(`^conflict-first/(?P<first>\w+)/$`, emptyView, name),
		urls.RePath(`^conflict-cannot-go-here/(?P<middle>\w+)/$`, emptyView, name),
		urls.RePath(`^conflict-middle/(?P<middle>\w+)/$`, emptyView, name),
		urls.RePath(`^conflict-last/(?P<last>\w+)/$`, emptyView, name),
		urls.RePath(`^conflict/(?P<another>\w+)/(?P<extra>\w+)/cannot-go-here/$`, emptyView, name),
		urls.RePath(`^conflict/(?P<extra>\w+)/(?P<another>\w+)/$`, emptyView, name),
	)
	runReverseCases(t, router, []reverseCase{
		{name: name, want: "/conflict/"},
		{name: name, args: []any{"arg"}, want: "/conflict-last/arg/"},
		{name: name, kwargs: urls.Kwargs{"first": "arg"}, want: "/conflict-first/arg/"},
		{name: name, kwargs: urls.Kwargs{"middle": "arg"}, want: "/conflict-middle/arg/"},
		{name: name, kwargs: urls.Kwargs{"last": "arg"}, want: "/conflict-last/arg/"},
		{name: name, args: []any{"arg", "arg"}, want: "/conflict/arg/arg/"},
	})
}

// TestNonRegex checks that a path that does not even start with a slash is a
// Resolver404 and never a nil match.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ResolverTests.test_non_regex
func TestNonRegex(t *testing.T) {
	for _, p := range []string{"", "a", `\`, "."} {
		assertResolver404(t, reverseURLs(), p)
	}
}

// Test404TriedURLsHaveNames checks that a Resolver404 lists the tried patterns
// with their kind and name, which is what a debug 404 page prints.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ResolverTests.test_404_tried_urls_have_names
func Test404TriedURLsHaveNames(t *testing.T) {
	inc := urls.TriedPattern{Pattern: "included/", Include: true}
	disjunction := `^(?P<one>[0-9]+)|(?P<two>[0-9]+)/$`
	want := [][]urls.TriedPattern{
		{{Pattern: "", Name: "named-url1"}},
		{{Pattern: `^extra/(?P<extra>\w+)/$`, Name: "named-url2"}},
		{{Pattern: disjunction}},
		{inc, {Pattern: "", Name: "named-url3"}},
		{inc, {Pattern: `^extra/(?P<extra>\w+)/$`, Name: "named-url4"}},
		{inc, {Pattern: disjunction}},
		{inc, inc},
	}
	_, err := namedURLs().Resolve("/included/nonexistent-url")
	var r404 *urls.Resolver404
	if !errors.As(err, &r404) {
		t.Fatalf("error = %v, want *Resolver404", err)
	}
	if !reflect.DeepEqual(r404.Tried, want) {
		t.Errorf("Tried = %v\nwant    %v", r404.Tried, want)
	}
	if got, wantText := want[1][0].String(), `^extra/(?P<extra>\w+)/$ [name='named-url2']`; got != wantText {
		t.Errorf("String() = %q, want %q", got, wantText)
	}
}

// urlObject is the urlpatterns of tests/urlpatterns_reverse/utils.py::URLObject.
func urlObject() []urls.Route {
	return []urls.Route{
		urls.Path("inner/", emptyView, "urlobject-view"),
		urls.RePath(`^inner/(?P<arg1>[0-9]+)/(?P<arg2>[0-9]+)/$`, emptyView, "urlobject-view"),
		urls.RePath(`^inner/\+\\\$\*/$`, emptyView, "urlobject-special-view"),
	}
}

// incRoutes is the list of routes included_namespace_urls.py and
// included_app_urls.py share.
func incRoutes() []urls.Route {
	return []urls.Route{
		urls.Path("normal/", emptyView, "inc-normal-view"),
		urls.RePath(`^normal/(?P<arg1>[0-9]+)/(?P<arg2>[0-9]+)/$`, emptyView, "inc-normal-view"),
		urls.RePath(`^\+\\\$\*/$`, emptyView, "inc-special-view"),
		urls.RePath(`^mixed_args/([0-9]+)/(?P<arg2>[0-9]+)/$`, emptyView, "inc-mixed-args"),
		urls.RePath(`^no_kwargs/([0-9]+)/([0-9]+)/$`, emptyView, "inc-no-kwargs"),
		urls.RePath(`^view_class/(?P<arg1>[0-9]+)/(?P<arg2>[0-9]+)/$`, viewClassInstance, "inc-view-class"),
	}
}

// namespaceDepth caps the mutual inclusion of namespace_urls.py and
// included_namespace_urls.py, which Django resolves lazily without end. Three
// levels cover every URL the tests ask for.
const namespaceDepth = 3

// includedNamespaceURLs is tests/urlpatterns_reverse/included_namespace_urls.py.
func includedNamespaceURLs(depth int) []urls.Route {
	routes := incRoutes()
	add := func(opts urls.IncludeOptions, sub []urls.Route) {
		routes = append(routes, urls.IncludeWith(opts, sub...)...)
	}
	add(urls.IncludeOptions{Prefix: "test3/", AppName: "testapp", Namespace: "test-ns3"}, urlObject())
	add(urls.IncludeOptions{Prefix: "test4/", AppName: "testapp", Namespace: "test-ns4"}, urlObject())
	add(urls.IncludeOptions{Prefix: "ns-included3/", AppName: "included_urls", Namespace: "inc-ns3"}, revIncludedURLs())
	if depth < namespaceDepth {
		add(urls.IncludeOptions{Prefix: "ns-included4/", AppName: "namespace_urls", Namespace: "inc-ns4"},
			namespaceRoutes(depth+1))
	}
	return routes
}

// namespaceRoutes is tests/urlpatterns_reverse/namespace_urls.py.
func namespaceRoutes(depth int) []urls.Route {
	routes := []urls.Route{
		urls.Path("normal/", emptyView, "normal-view"),
		urls.RePath(`^normal/(?P<arg1>[0-9]+)/(?P<arg2>[0-9]+)/$`, emptyView, "normal-view"),
		urls.Path("resolver_match/", emptyView, "test-resolver-match"),
		urls.RePath(`^\+\\\$\*/$`, emptyView, "special-view"),
		urls.RePath(`^mixed_args/([0-9]+)/(?P<arg2>[0-9]+)/$`, emptyView, "mixed-args").
			WithKwargs(urls.Kwargs{"extra": true}),
		urls.RePath(`^no_kwargs/([0-9]+)/([0-9]+)/$`, emptyView, "no-kwargs"),
		urls.RePath(`^view_class/(?P<arg1>[0-9]+)/(?P<arg2>[0-9]+)/$`, viewClassInstance, "view-class"),
		urls.RePath(`^unnamed/normal/(?P<arg1>[0-9]+)/(?P<arg2>[0-9]+)/$`, emptyView, ""),
		urls.RePath(`^unnamed/view_class/(?P<arg1>[0-9]+)/(?P<arg2>[0-9]+)/$`, viewClassInstance, ""),
	}
	add := func(opts urls.IncludeOptions, sub []urls.Route) {
		routes = append(routes, urls.IncludeWith(opts, sub...)...)
	}
	const incApp = "included_namespace_urls"
	add(urls.IncludeOptions{Prefix: "test1/", AppName: "testapp", Namespace: "test-ns1"}, urlObject())
	add(urls.IncludeOptions{Prefix: "test2/", AppName: "testapp", Namespace: "test-ns2"}, urlObject())
	add(urls.IncludeOptions{Prefix: "default/", AppName: "testapp", Namespace: "testapp"}, urlObject())
	add(urls.IncludeOptions{Prefix: "other1/", AppName: "nodefault", Namespace: "other-ns1"}, urlObject())
	add(urls.IncludeOptions{Prefix: `^other[246]/`, Regex: true, AppName: "nodefault", Namespace: "other-ns2"}, urlObject())
	add(urls.IncludeOptions{Prefix: "newapp1/", AppName: "newapp", Namespace: "new-ns1"}, urlObject())
	add(urls.IncludeOptions{Prefix: "new-default/", AppName: "newapp"}, urlObject())
	add(urls.IncludeOptions{Prefix: `^app-included[135]/`, Regex: true, AppName: "inc-app", Namespace: "app-ns1"}, incRoutes())
	add(urls.IncludeOptions{Prefix: "app-included2/", AppName: "inc-app", Namespace: "app-ns2"}, incRoutes())
	var inc []urls.Route
	if depth < namespaceDepth {
		inc = includedNamespaceURLs(depth + 1)
	}
	add(urls.IncludeOptions{Prefix: `^ns-included[135]/`, Regex: true, AppName: incApp, Namespace: "inc-ns1"}, inc)
	add(urls.IncludeOptions{Prefix: "ns-included2/", AppName: incApp, Namespace: "inc-ns2"}, inc)
	add(urls.IncludeOptions{Prefix: "app-included/", AppName: incApp, Namespace: "inc-app"}, inc)
	add(urls.IncludeOptions{Prefix: "included/", AppName: incApp}, inc)
	add(urls.IncludeOptions{Prefix: `^inc(?P<outer>[0-9]+)/`, Regex: true, AppName: "included_urls", Namespace: "inc-ns5"},
		revIncludedURLs())
	add(urls.IncludeOptions{Prefix: `^included/([0-9]+)/`, Regex: true, AppName: incApp}, inc)
	add(urls.IncludeOptions{Prefix: `^ns-outer/(?P<outer>[0-9]+)/`, Regex: true, AppName: incApp, Namespace: "inc-outer"}, inc)
	if depth < namespaceDepth {
		add(urls.IncludeOptions{Prefix: `^\+\\\$\*/`, Regex: true, AppName: "namespace_urls", Namespace: "special"},
			namespaceRoutes(depth+1))
	}
	return routes
}

// namespaceURLs is the router over namespace_urls.py.
var namespaceURLs = sync.OnceValue(func() *urls.Router { return urls.NewRouter(namespaceRoutes(0)...) })

// four expands one Django test of the usual shape: the plain URL, the URL
// with two positional arguments, with two keyword arguments, and the URL of
// the special-characters view.
func four(view, special, currentApp, base string) []reverseCase {
	return []reverseCase{
		{name: view, currentApp: currentApp, want: base + "inner/"},
		{name: view, currentApp: currentApp, args: []any{37, 42}, want: base + "inner/37/42/"},
		{name: view, currentApp: currentApp, kwargs: urls.Kwargs{"arg1": 42, "arg2": 37}, want: base + "inner/42/37/"},
		{name: special, currentApp: currentApp, want: base + `inner/+%5C$*/`},
	}
}

// TestNamespaceLookupFailures checks the names that must not reverse.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_ambiguous_object
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_ambiguous_urlpattern
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_non_existent_namespace
func TestNamespaceLookupFailures(t *testing.T) {
	both := urls.Kwargs{"arg1": 42, "arg2": 37}
	runReverseCases(t, namespaceURLs(), []reverseCase{
		{name: "urlobject-view", want: noReverse},
		{name: "urlobject-view", args: []any{37, 42}, want: noReverse},
		{name: "urlobject-view", kwargs: both, want: noReverse},
		{name: "inner-nothing", want: noReverse},
		{name: "inner-nothing", args: []any{37, 42}, want: noReverse},
		{name: "inner-nothing", kwargs: both, want: noReverse},
		{name: "blahblah:urlobject-view", want: noReverse},
		{name: "test-ns1:blahblah:urlobject-view", want: noReverse},
	})
	_, err := namespaceURLs().Reverse("blahblah:urlobject-view")
	if err == nil || err.Error() != "'blahblah' is not a registered namespace" {
		t.Errorf("error = %v, want Django's unregistered-namespace message", err)
	}
	_, err = namespaceURLs().Reverse("test-ns1:blahblah:urlobject-view")
	if err == nil || err.Error() != "'blahblah' is not a registered namespace inside 'test-ns1'" {
		t.Errorf("error = %v, want Django's nested unregistered-namespace message", err)
	}
}

// TestNamespaceReverse runs the reverse tables of Django's NamespaceTests,
// one subtest per Django test.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_normal_name
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_simple_included_name
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_namespace_object
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_app_object
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_app_object_default_namespace
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_embedded_namespace_object
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_namespace_pattern
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_app_name_pattern
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_namespace_pattern_with_variable_prefix
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_multiple_namespace_pattern
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_nested_namespace_pattern
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_app_lookup_object
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_app_lookup_object_with_default
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_app_lookup_object_without_default
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_special_chars_namespace
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_namespaces_with_variables
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_nested_app_lookup
// Ported from Django: tests/urlpatterns_reverse/tests.py::NamespaceTests.test_current_app_no_partial_match
func TestNamespaceReverse(t *testing.T) {
	both := urls.Kwargs{"arg1": 42, "arg2": 37}
	pair := []any{37, 42}
	// normal expands a test over the normal and special views of a URLconf.
	normal := func(view, special, base string) []reverseCase {
		return []reverseCase{
			{name: view, want: base + "normal/"},
			{name: view, args: pair, want: base + "normal/37/42/"},
			{name: view, kwargs: both, want: base + "normal/42/37/"},
			{name: special, want: base + `+%5C$*/`},
		}
	}
	const nested = "inc-ns1:inc-ns4:inc-ns1:test-ns3:"
	tests := map[string][]reverseCase{
		"test_normal_name": normal("normal-view", "special-view", "/"),
		"test_simple_included_name": normal(
			"included_namespace_urls:inc-normal-view", "included_namespace_urls:inc-special-view", "/included/"),
		"test_namespace_object": four("test-ns1:urlobject-view", "test-ns1:urlobject-special-view", "", "/test1/"),
		"test_app_object":       four("new-ns1:urlobject-view", "new-ns1:urlobject-special-view", "", "/newapp1/"),
		"test_app_object_default_namespace": four(
			"newapp:urlobject-view", "newapp:urlobject-special-view", "", "/new-default/"),
		"test_embedded_namespace_object": four(
			"included_namespace_urls:test-ns3:urlobject-view",
			"included_namespace_urls:test-ns3:urlobject-special-view", "", "/included/test3/"),
		"test_namespace_pattern": normal("inc-ns1:inc-normal-view", "inc-ns1:inc-special-view", "/ns-included1/"),
		"test_app_name_pattern":  normal("app-ns1:inc-normal-view", "app-ns1:inc-special-view", "/app-included1/"),
		"test_namespace_pattern_with_variable_prefix": {
			{name: "inc-outer:inc-normal-view", kwargs: urls.Kwargs{"outer": 42}, want: "/ns-outer/42/normal/"},
			{name: "inc-outer:inc-normal-view", args: []any{42}, want: "/ns-outer/42/normal/"},
			{
				name: "inc-outer:inc-normal-view", kwargs: urls.Kwargs{"arg1": 37, "arg2": 4, "outer": 42},
				want: "/ns-outer/42/normal/37/4/",
			},
			{name: "inc-outer:inc-normal-view", args: []any{42, 37, 4}, want: "/ns-outer/42/normal/37/4/"},
			{name: "inc-outer:inc-special-view", kwargs: urls.Kwargs{"outer": 42}, want: `/ns-outer/42/+%5C$*/`},
			{name: "inc-outer:inc-special-view", args: []any{42}, want: `/ns-outer/42/+%5C$*/`},
		},
		"test_multiple_namespace_pattern": four(
			"inc-ns1:test-ns3:urlobject-view", "inc-ns1:test-ns3:urlobject-special-view", "", "/ns-included1/test3/"),
		"test_nested_namespace_pattern": four(
			nested+"urlobject-view", nested+"urlobject-special-view", "",
			"/ns-included1/ns-included4/ns-included1/test3/"),
		"test_app_lookup_object": four("testapp:urlobject-view", "testapp:urlobject-special-view", "", "/default/"),
		"test_app_lookup_object_with_default": four(
			"testapp:urlobject-view", "testapp:urlobject-special-view", "test-ns3", "/default/"),
		"test_app_lookup_object_without_default": append(
			four("nodefault:urlobject-view", "nodefault:urlobject-special-view", "", "/other2/"),
			four("nodefault:urlobject-view", "nodefault:urlobject-special-view", "other-ns1", "/other1/")...),
		"test_special_chars_namespace": normal(
			"special:included_namespace_urls:inc-normal-view",
			"special:included_namespace_urls:inc-special-view", `/+%5C$*/included/`),
		"test_namespaces_with_variables": {
			{name: "inc-ns5:inner-nothing", kwargs: urls.Kwargs{"outer": "70"}, want: "/inc70/"},
			{
				name: "inc-ns5:inner-extra", kwargs: urls.Kwargs{"extra": "foobar", "outer": "78"},
				want: "/inc78/extra/foobar/",
			},
			{name: "inc-ns5:inner-nothing", args: []any{"70"}, want: "/inc70/"},
			{name: "inc-ns5:inner-extra", args: []any{"78", "foobar"}, want: "/inc78/extra/foobar/"},
		},
		"test_nested_app_lookup": append(
			four("inc-ns1:testapp:urlobject-view", "inc-ns1:testapp:urlobject-special-view", "",
				"/ns-included1/test4/"),
			four("inc-ns1:testapp:urlobject-view", "inc-ns1:testapp:urlobject-special-view", "inc-ns1:test-ns3",
				"/ns-included1/test3/")...),
		"test_current_app_no_partial_match": four(
			"inc-ns1:testapp:urlobject-view", "inc-ns1:testapp:urlobject-special-view", "nonexistent:test-ns3",
			"/ns-included1/test4/"),
	}
	for name, cases := range tests {
		t.Run(name, func(t *testing.T) { runReverseCases(t, namespaceURLs(), cases) })
	}
}

// TestURLPatternResolve runs Django's table of resolved paths. Django names
// an unnamed route after the dotted path of its view function; Go has no such
// path, so ViewName is empty there.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ResolverMatchTests.test_urlpattern_resolve
func TestURLPatternResolve(t *testing.T) {
	both := urls.Kwargs{"arg1": "42", "arg2": "37"}
	const incNS = "included_namespace_urls"
	tests := []struct {
		path, urlName, appName, namespace, viewName string
		class                                       bool
		args                                        []any
		kwargs                                      urls.Kwargs
	}{
		{"/normal/42/37/", "normal-view", "", "", "normal-view", false, nil, both},
		{"/view_class/42/37/", "view-class", "", "", "view-class", true, nil, both},
		{"/included/normal/42/37/", "inc-normal-view", incNS, incNS, incNS + ":inc-normal-view", false, nil, both},
		{"/included/view_class/42/37/", "inc-view-class", incNS, incNS, incNS + ":inc-view-class", true, nil, both},
		// Unnamed groups are dropped as soon as a pattern has a named one.
		{"/mixed_args/42/37/", "mixed-args", "", "", "mixed-args", false, nil, urls.Kwargs{"extra": true, "arg2": "37"}},
		{
			"/included/mixed_args/42/37/", "inc-mixed-args", incNS, incNS, incNS + ":inc-mixed-args", false, nil,
			urls.Kwargs{"arg2": "37"},
		},
		{
			"/included/12/mixed_args/42/37/", "inc-mixed-args", incNS, incNS, incNS + ":inc-mixed-args", false, nil,
			urls.Kwargs{"arg2": "37"},
		},
		{"/unnamed/normal/42/37/", "", "", "", "", false, nil, both},
		{"/unnamed/view_class/42/37/", "", "", "", "", true, nil, both},
		// Without named groups the unnamed ones are the positional arguments.
		{"/no_kwargs/42/37/", "no-kwargs", "", "", "no-kwargs", false, []any{"42", "37"}, nil},
		{
			"/included/no_kwargs/42/37/", "inc-no-kwargs", incNS, incNS, incNS + ":inc-no-kwargs", false,
			[]any{"42", "37"},
			nil,
		},
		{
			"/included/12/no_kwargs/42/37/", "inc-no-kwargs", incNS, incNS, incNS + ":inc-no-kwargs", false,
			[]any{"12", "42", "37"},
			nil,
		},
		{"/test1/inner/42/37/", "urlobject-view", "testapp", "test-ns1", "test-ns1:urlobject-view", false, nil, both},
		{
			"/included/test3/inner/42/37/", "urlobject-view", incNS + ":testapp", incNS + ":test-ns3",
			incNS + ":test-ns3:urlobject-view", false, nil, both,
		},
		{"/ns-included1/normal/42/37/", "inc-normal-view", incNS, "inc-ns1", "inc-ns1:inc-normal-view", false, nil, both},
		{"/default/inner/42/37/", "urlobject-view", "testapp", "testapp", "testapp:urlobject-view", false, nil, both},
		{"/other2/inner/42/37/", "urlobject-view", "nodefault", "other-ns2", "other-ns2:urlobject-view", false, nil, both},
		{"/other1/inner/42/37/", "urlobject-view", "nodefault", "other-ns1", "other-ns1:urlobject-view", false, nil, both},
		{
			"/ns-included1/test3/inner/42/37/", "urlobject-view", incNS + ":testapp", "inc-ns1:test-ns3",
			"inc-ns1:test-ns3:urlobject-view", false, nil, both,
		},
		{
			"/ns-included1/ns-included4/ns-included2/test3/inner/42/37/", "urlobject-view",
			incNS + ":namespace_urls:" + incNS + ":testapp", "inc-ns1:inc-ns4:inc-ns2:test-ns3",
			"inc-ns1:inc-ns4:inc-ns2:test-ns3:urlobject-view", false, nil, both,
		},
		{
			"/app-included/test3/inner/42/37/", "urlobject-view", incNS + ":testapp", "inc-app:test-ns3",
			"inc-app:test-ns3:urlobject-view", false, nil, both,
		},
		{
			"/app-included/ns-included4/ns-included2/test3/inner/42/37/", "urlobject-view",
			incNS + ":namespace_urls:" + incNS + ":testapp", "inc-app:inc-ns4:inc-ns2:test-ns3",
			"inc-app:inc-ns4:inc-ns2:test-ns3:urlobject-view", false, nil, both,
		},
		{"/inc70/", "inner-nothing", "included_urls", "inc-ns5", "inc-ns5:inner-nothing", false, nil, urls.Kwargs{"outer": "70"}},
		{
			"/inc78/extra/foobar/", "inner-extra", "included_urls", "inc-ns5", "inc-ns5:inner-extra", false, nil,
			urls.Kwargs{"outer": "78", "extra": "foobar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			m := mustResolve(t, namespaceURLs(), tt.path)
			if m.URLName != tt.urlName {
				t.Errorf("URLName = %q, want %q", m.URLName, tt.urlName)
			}
			if m.AppName() != tt.appName {
				t.Errorf("AppName() = %q, want %q", m.AppName(), tt.appName)
			}
			if m.Namespace() != tt.namespace {
				t.Errorf("Namespace() = %q, want %q", m.Namespace(), tt.namespace)
			}
			if m.ViewName != tt.viewName {
				t.Errorf("ViewName = %q, want %q", m.ViewName, tt.viewName)
			}
			if got := m.Handler == http.Handler(viewClassInstance); got != tt.class {
				t.Errorf("handler is the view class instance: %v, want %v", got, tt.class)
			}
			if len(m.Args) != len(tt.args) || (len(tt.args) > 0 && !reflect.DeepEqual(m.Args, tt.args)) {
				t.Errorf("Args = %v, want %v", m.Args, tt.args)
			}
			assertKwargs(t, "Kwargs", m.Kwargs, tt.kwargs)
		})
	}
}

// TestResolverMatchOnRequestBeforeResolution checks that a request that did
// not come through a router has no match.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ResolverMatchTests.test_resolver_match_on_request_before_resolution
func TestResolverMatchOnRequestBeforeResolution(t *testing.T) {
	if m := urls.Match(httptest.NewRequest(http.MethodGet, "/", nil)); m != nil {
		t.Fatalf("Match = %+v, want nil", m)
	}
}

// TestRereverse checks that the result of Resolve reverses back to the path.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ReverseResolvedTests.test_rereverse
func TestRereverse(t *testing.T) {
	router := reverseURLs()
	m := mustResolve(t, router, "/resolved/12/")
	got, err := router.ReverseWith(m.URLName, urls.ReverseOptions{Args: m.Args, Kwargs: m.Kwargs})
	if err != nil || got != "/resolved/12/" {
		t.Errorf("reverse of the resolved match = %q, %v; want /resolved/12/", got, err)
	}
	m = mustResolve(t, router, "/resolved-overridden/12/url/")
	got, err = router.ReverseWith(m.URLName, urls.ReverseOptions{Args: m.Args, Kwargs: m.CapturedKwargs})
	if err != nil || got != "/resolved-overridden/12/url/" {
		t.Errorf("reverse of the captured arguments = %q, %v; want /resolved-overridden/12/url/", got, err)
	}
}

// TestIncludeNamespaces checks which namespaces an include gives its routes.
// Django spells the cases as tuples passed to include(); Go has IncludeApp and
// IncludeWith. A namespace without an application name is an error in Django
// and names both in Go. The 3-tuple and 4-tuple errors have no Go counterpart.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::IncludeTests.test_include_urls
// Ported from Django: tests/urlpatterns_reverse/tests.py::IncludeTests.test_include_namespace
// Ported from Django: tests/urlpatterns_reverse/tests.py::IncludeTests.test_include_2_tuple
// Ported from Django: tests/urlpatterns_reverse/tests.py::IncludeTests.test_include_2_tuple_namespace
// Ported from Django: tests/urlpatterns_reverse/tests.py::IncludeTests.test_include_app_name
// Ported from Django: tests/urlpatterns_reverse/tests.py::IncludeTests.test_include_app_name_namespace
func TestIncludeNamespaces(t *testing.T) {
	tests := []struct {
		django             string
		routes             []urls.Route
		appName, namespace string
	}{
		{"test_include_urls", urls.Include("x/", "", urlObject()...), "", ""},
		{"test_include_namespace", urls.Include("x/", "namespace", urlObject()...), "namespace", "namespace"},
		{"test_include_2_tuple", urls.IncludeApp("x/", "app_name", "", urlObject()...), "app_name", "app_name"},
		{
			"test_include_2_tuple_namespace", urls.IncludeApp("x/", "app_name", "namespace", urlObject()...),
			"app_name", "namespace",
		},
		{"test_include_app_name", urls.IncludeApp("x/", "inc-app", "", urlObject()...), "inc-app", "inc-app"},
		{
			"test_include_app_name_namespace", urls.IncludeApp("x/", "inc-app", "namespace", urlObject()...),
			"inc-app", "namespace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.django, func(t *testing.T) {
			m := mustResolve(t, urls.NewRouter(tt.routes...), "/x/inner/")
			if m.AppName() != tt.appName || m.Namespace() != tt.namespace {
				t.Errorf("app %q, namespace %q; want %q, %q", m.AppName(), m.Namespace(), tt.appName, tt.namespace)
			}
		})
	}
}

// TestInvalidRegex checks that an expression that does not compile is a
// configuration error naming the expression. The detail after the colon is
// the Go regexp package's, not Python's.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ErroneousViewTests.test_invalid_regex
func TestInvalidRegex(t *testing.T) {
	assertConfigError(t, urls.RePath(`(regex_error/$`, emptyView, ""),
		`"(regex_error/$" is not a valid regular expression: `)
}

// TestNoncallableView checks that a route needs a handler. Django rejects a
// view that is not callable; the Go type system leaves only the nil handler.
//
// Ported from Django: tests/urlpatterns_reverse/tests.py::ErroneousViewTests.test_noncallable_view
func TestNoncallableView(t *testing.T) {
	if _, err := urls.NewRouterChecked(urls.Path("uncallable-object/", nil, "")); !errors.Is(err, urls.ErrImproperlyConfigured) {
		t.Fatalf("error = %v, want ImproperlyConfigured", err)
	}
}
