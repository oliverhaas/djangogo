package urls_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/urls"
)

// emptyView is the handler every ported URLconf routes to (Django's
// views.empty_view).
var emptyView http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// base64Converter ports tests/urlpatterns/converters.py::Base64Converter.
type base64Converter struct{}

func (base64Converter) Regex() string { return `[a-zA-Z0-9+/]*={0,2}` }

func (base64Converter) ToGo(value string) (any, error) {
	return base64.StdEncoding.Strict().DecodeString(value)
}

func (base64Converter) ToURL(value any) (string, error) {
	b, ok := value.([]byte)
	if !ok {
		return "", fmt.Errorf("base64 converter: want []byte, got %T", value)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// dynamicConverter ports tests/urlpatterns/converters.py::DynamicConverter:
// its conversions are whatever the test sets.
type dynamicConverter struct {
	toGo  func(string) (any, error)
	toURL func(any) (string, error)
}

func (*dynamicConverter) Regex() string { return `[0-9a-zA-Z]+` }

func (c *dynamicConverter) ToGo(value string) (any, error) { return c.toGo(value) }

func (c *dynamicConverter) ToURL(value any) (string, error) { return c.toURL(value) }

// registerConverter registers c for the duration of the test.
func registerConverter(t *testing.T, name string, c urls.Converter) {
	t.Helper()
	if err := urls.RegisterConverter(name, c); err != nil {
		t.Fatalf("RegisterConverter(%q): %v", name, err)
	}
	t.Cleanup(func() { urls.UnregisterConverter(name) })
}

// moreURLs ports tests/urlpatterns/more_urls.py.
func moreURLs() []urls.Route {
	return []urls.Route{
		urls.RePath(`^more/(?P<extra>\w+)/$`, emptyView, "inner-more").WithKwargs(urls.Kwargs{"sub-extra": true}),
	}
}

// includedURLs ports tests/urlpatterns/included_urls.py.
func includedURLs() []urls.Route {
	return slices.Concat(
		[]urls.Route{urls.Path("extra/<extra>/", emptyView, "inner-extra")},
		urls.IncludeWith(urls.IncludeOptions{}, moreURLs()...),
	)
}

// pathURLs ports tests/urlpatterns/path_urls.py.
func pathURLs(t *testing.T) *urls.Router {
	t.Helper()
	return urls.NewRouter(slices.Concat([]urls.Route{
		urls.Path("articles/2003/", emptyView, "articles-2003"),
		urls.Path("articles/<int:year>/", emptyView, "articles-year"),
		urls.Path("articles/<int:year>/<int:month>/", emptyView, "articles-year-month"),
		urls.Path("articles/<int:year>/<int:month>/<int:day>/", emptyView, "articles-year-month-day"),
		urls.Path("books/2007/", emptyView, "books-2007").WithKwargs(urls.Kwargs{"extra": true}),
		urls.Path("books/<int:year>/<int:month>/<int:day>/", emptyView, "books-year-month-day").
			WithKwargs(urls.Kwargs{"extra": true}),
		urls.Path("users/", emptyView, "users"),
		urls.Path("users/<id>/", emptyView, "user-with-id"),
	},
		urls.IncludeWith(urls.IncludeOptions{Prefix: "included_urls/"}, includedURLs()...),
		[]urls.Route{
			urls.RePath(`^regex/(?P<pk>[0-9]+)/$`, emptyView, "regex"),
			urls.RePath(`^regex_optional/(?P<arg1>\d+)/(?:(?P<arg2>\d+)/)?`, emptyView, "regex_optional"),
			urls.RePath(`^regex_only_optional/(?:(?P<arg1>\d+)/)?`, emptyView, "regex_only_optional"),
		},
		urls.IncludeWith(urls.IncludeOptions{Kwargs: urls.Kwargs{"sub-extra": false}}, moreURLs()...),
		[]urls.Route{urls.Path("<lang>/<path:url>/", emptyView, "lang-and-path")},
	)...)
}

// base64URLs ports tests/urlpatterns/path_base64_urls.py.
func base64URLs(t *testing.T) *urls.Router {
	t.Helper()
	registerConverter(t, "base64", base64Converter{})
	subsub := []urls.Route{urls.Path("<base64:last_value>/", emptyView, "subsubpattern-base64")}
	sub := slices.Concat(
		[]urls.Route{urls.Path("<base64:value>/", emptyView, "subpattern-base64")},
		urls.IncludeWith(urls.IncludeOptions{
			Prefix: "<base64:value>/", AppName: "second-layer-namespaced-base64", Namespace: "instance-ns-base64",
		}, subsub...),
	)
	return urls.NewRouter(slices.Concat(
		[]urls.Route{urls.Path("base64/<base64:value>/", emptyView, "base64")},
		urls.IncludeWith(urls.IncludeOptions{Prefix: "base64/<base64:base>/subpatterns/"}, sub...),
		urls.IncludeWith(
			urls.IncludeOptions{Prefix: "base64/<base64:base>/namespaced/", AppName: "namespaced-base64"}, sub...),
	)...)
}

// mustResolve resolves path or fails the test.
func mustResolve(t *testing.T, router *urls.Router, path string) *urls.ResolverMatch {
	t.Helper()
	m, err := router.Resolve(path)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", path, err)
	}
	return m
}

// assertKwargs compares keyword arguments, treating nil and empty alike as
// Django's {}.
func assertKwargs(t *testing.T, what string, got, want urls.Kwargs) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// assertMatch checks the fields of a ResolverMatch the Django tests compare.
func assertMatch(t *testing.T, m *urls.ResolverMatch, urlName, route string, kwargs, captured, extra urls.Kwargs) {
	t.Helper()
	if m.URLName != urlName {
		t.Errorf("URLName = %q, want %q", m.URLName, urlName)
	}
	if len(m.Args) != 0 {
		t.Errorf("Args = %v, want none", m.Args)
	}
	if m.Route != route {
		t.Errorf("Route = %q, want %q", m.Route, route)
	}
	assertKwargs(t, "Kwargs", m.Kwargs, kwargs)
	assertKwargs(t, "CapturedKwargs", m.CapturedKwargs, captured)
	assertKwargs(t, "ExtraKwargs", m.ExtraKwargs, extra)
}

// assertResolver404 checks that path does not resolve.
func assertResolver404(t *testing.T, router *urls.Router, path string) {
	t.Helper()
	m, err := router.Resolve(path)
	if err == nil {
		t.Errorf("Resolve(%q) = %q, want Resolver404", path, m.Route)
		return
	}
	if !errors.Is(err, urls.ErrResolver404) {
		t.Errorf("Resolve(%q) error = %v, want Resolver404", path, err)
	}
}

// assertReverse checks the URL a name reverses to.
func assertReverse(t *testing.T, router *urls.Router, want, name string, args ...any) {
	t.Helper()
	got, err := router.Reverse(name, args...)
	if err != nil {
		t.Errorf("Reverse(%q, %v): %v", name, args, err)
		return
	}
	if got != want {
		t.Errorf("Reverse(%q, %v) = %q, want %q", name, args, got, want)
	}
}

// assertConfigError checks that building a router from route fails with msg.
func assertConfigError(t *testing.T, route urls.Route, msg string) {
	t.Helper()
	_, err := urls.NewRouterChecked(route)
	if err == nil {
		t.Errorf("NewRouterChecked(%q) succeeded, want error %q", route.Pattern, msg)
		return
	}
	if !errors.Is(err, urls.ErrImproperlyConfigured) {
		t.Errorf("error %v is not ImproperlyConfigured", err)
	}
	if !strings.Contains(err.Error(), msg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), msg)
	}
}

// TestPathLookupWithoutParameters checks a static route.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_without_parameters
func TestPathLookupWithoutParameters(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/articles/2003/")
	assertMatch(t, m, "articles-2003", "articles/2003/", nil, nil, nil)
}

// TestPathLookupWithTypedParameters checks that the int converter hands over
// an int.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_with_typed_parameters
func TestPathLookupWithTypedParameters(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/articles/2015/")
	kw := urls.Kwargs{"year": 2015}
	assertMatch(t, m, "articles-year", "articles/<int:year>/", kw, kw, nil)
}

// TestPathLookupWithMultipleParameters checks several typed parameters.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_with_multiple_parameters
func TestPathLookupWithMultipleParameters(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/articles/2015/04/12/")
	kw := urls.Kwargs{"year": 2015, "month": 4, "day": 12}
	assertMatch(t, m, "articles-year-month-day", "articles/<int:year>/<int:month>/<int:day>/", kw, kw, nil)
}

// TestPathLookupWithMultipleParametersAndExtraKwarg checks that extra
// arguments are merged into the captured ones.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_with_multiple_parameters_and_extra_kwarg
func TestPathLookupWithMultipleParametersAndExtraKwarg(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/books/2015/04/12/")
	assertMatch(t, m, "books-year-month-day", "books/<int:year>/<int:month>/<int:day>/",
		urls.Kwargs{"year": 2015, "month": 4, "day": 12, "extra": true},
		urls.Kwargs{"year": 2015, "month": 4, "day": 12},
		urls.Kwargs{"extra": true})
}

// TestPathLookupWithExtraKwarg checks extra arguments on a static route.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_with_extra_kwarg
func TestPathLookupWithExtraKwarg(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/books/2007/")
	assertMatch(t, m, "books-2007", "books/2007/", urls.Kwargs{"extra": true}, nil, urls.Kwargs{"extra": true})
}

// TestTwoVariableAtStartOfPathPattern checks a route that starts with a
// parameter.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_two_variable_at_start_of_path_pattern
func TestTwoVariableAtStartOfPathPattern(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/en/foo/")
	kw := urls.Kwargs{"lang": "en", "url": "foo"}
	assertMatch(t, m, "lang-and-path", "<lang>/<path:url>/", kw, kw, nil)
}

// TestRePath checks a regular-expression route; its groups stay text.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_re_path
func TestRePath(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/regex/1/")
	kw := urls.Kwargs{"pk": "1"}
	assertMatch(t, m, "regex", "^regex/(?P<pk>[0-9]+)/$", kw, kw, nil)
}

// TestRePathWithOptionalParameter checks that an optional group that did not
// match is left out of the arguments.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_re_path_with_optional_parameter
func TestRePathWithOptionalParameter(t *testing.T) {
	router := pathURLs(t)
	for url, kw := range map[string]urls.Kwargs{
		"/regex_optional/1/2/": {"arg1": "1", "arg2": "2"},
		"/regex_optional/1/":   {"arg1": "1"},
	} {
		t.Run(url, func(t *testing.T) {
			m := mustResolve(t, router, url)
			assertMatch(t, m, "regex_optional", `^regex_optional/(?P<arg1>\d+)/(?:(?P<arg2>\d+)/)?`, kw, kw, nil)
		})
	}
}

// TestRePathWithMissingOptionalParameter checks an expression whose only
// group is optional and absent.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_re_path_with_missing_optional_parameter
func TestRePathWithMissingOptionalParameter(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/regex_only_optional/")
	assertMatch(t, m, "regex_only_optional", `^regex_only_optional/(?:(?P<arg1>\d+)/)?`, nil, nil, nil)
}

// TestPathLookupWithInclusion checks the joined route of an included route.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_with_inclusion
func TestPathLookupWithInclusion(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/included_urls/extra/something/")
	if m.URLName != "inner-extra" {
		t.Errorf("URLName = %q", m.URLName)
	}
	if m.Route != "included_urls/extra/<extra>/" {
		t.Errorf("Route = %q", m.Route)
	}
}

// TestPathLookupWithEmptyStringInclusion checks an include with an empty
// prefix: the route's own extra argument wins over the include's.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_with_empty_string_inclusion
func TestPathLookupWithEmptyStringInclusion(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/more/99/")
	assertMatch(t, m, "inner-more", `^more/(?P<extra>\w+)/$`,
		urls.Kwargs{"extra": "99", "sub-extra": true},
		urls.Kwargs{"extra": "99"},
		urls.Kwargs{"sub-extra": true})
}

// TestPathLookupWithDoubleInclusion checks a route two includes deep.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_lookup_with_double_inclusion
func TestPathLookupWithDoubleInclusion(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/included_urls/more/some_value/")
	if m.URLName != "inner-more" {
		t.Errorf("URLName = %q", m.URLName)
	}
	if m.Route != `included_urls/more/(?P<extra>\w+)/$` {
		t.Errorf("Route = %q", m.Route)
	}
}

// TestPathReverseWithoutParameter reverses a static route.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_reverse_without_parameter
func TestPathReverseWithoutParameter(t *testing.T) {
	assertReverse(t, pathURLs(t), "/articles/2003/", "articles-2003")
}

// TestPathReverseWithParameter reverses with keyword arguments.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_reverse_with_parameter
func TestPathReverseWithParameter(t *testing.T) {
	assertReverse(t, pathURLs(t), "/articles/2015/4/12/", "articles-year-month-day",
		urls.Kwargs{"year": 2015, "month": 4, "day": 12})
}

// converterTestData ports converter_test_data: URL, route name, application
// namespace and arguments. aGVsbG8= is "hello" in base64.
var converterTestData = []struct {
	url, urlName, appName string
	kwargs                urls.Kwargs
}{
	{"/base64/aGVsbG8=/", "base64", "", urls.Kwargs{"value": []byte("hello")}},
	{
		"/base64/aGVsbG8=/subpatterns/d29ybGQ=/", "subpattern-base64", "",
		urls.Kwargs{"base": []byte("hello"), "value": []byte("world")},
	},
	{
		"/base64/aGVsbG8=/namespaced/d29ybGQ=/", "subpattern-base64", "namespaced-base64",
		urls.Kwargs{"base": []byte("hello"), "value": []byte("world")},
	},
}

// TestConverterResolve checks a registered converter in routes and include
// prefixes.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_converter_resolve
func TestConverterResolve(t *testing.T) {
	router := base64URLs(t)
	for _, tc := range converterTestData {
		t.Run(tc.url, func(t *testing.T) {
			m := mustResolve(t, router, tc.url)
			if m.URLName != tc.urlName {
				t.Errorf("URLName = %q, want %q", m.URLName, tc.urlName)
			}
			if m.AppName() != tc.appName {
				t.Errorf("AppName = %q, want %q", m.AppName(), tc.appName)
			}
			assertKwargs(t, "Kwargs", m.Kwargs, tc.kwargs)
		})
	}
}

// TestConverterReverse checks that Reverse writes values with the converter.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_converter_reverse
func TestConverterReverse(t *testing.T) {
	router := base64URLs(t)
	for _, tc := range converterTestData {
		name := tc.urlName
		if tc.appName != "" {
			name = tc.appName + ":" + name
		}
		assertReverse(t, router, tc.url, name, tc.kwargs)
	}
}

// TestConverterReverseWithSecondLayerInstanceNamespace reverses through an
// instance namespace that sits below a plain include.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_converter_reverse_with_second_layer_instance_namespace
func TestConverterReverseWithSecondLayerInstanceNamespace(t *testing.T) {
	kwargs := urls.Kwargs{"base": []byte("hello"), "value": []byte("world"), "last_value": []byte("world")}
	assertReverse(t, base64URLs(t), "/base64/aGVsbG8=/subpatterns/d29ybGQ=/d29ybGQ=/",
		"instance-ns-base64:subsubpattern-base64", kwargs)
}

// TestPathInclusionIsMatchable checks the arguments of an included route.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_inclusion_is_matchable
func TestPathInclusionIsMatchable(t *testing.T) {
	m := mustResolve(t, pathURLs(t), "/included_urls/extra/something/")
	if m.URLName != "inner-extra" {
		t.Errorf("URLName = %q", m.URLName)
	}
	assertKwargs(t, "Kwargs", m.Kwargs, urls.Kwargs{"extra": "something"})
}

// TestPathInclusionIsReversible reverses an included route.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_inclusion_is_reversible
func TestPathInclusionIsReversible(t *testing.T) {
	assertReverse(t, pathURLs(t), "/included_urls/extra/something/", "inner-extra", urls.Kwargs{"extra": "something"})
}

// TestInvalidConverter checks the message for an unknown converter. Django
// raises it from path(); here it surfaces when the router is built, because
// Path cannot return an error.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_invalid_converter
func TestInvalidConverter(t *testing.T) {
	assertConfigError(t, urls.Path("foo/<nonexistent:var>/", emptyView, ""),
		"URL route 'foo/<nonexistent:var>/' uses invalid converter 'nonexistent'.")
}

// TestWarningOverrideDefaultConverter checks that a built-in converter cannot
// be replaced.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_warning_override_default_converter
func TestWarningOverrideDefaultConverter(t *testing.T) {
	err := urls.RegisterConverter("int", base64Converter{})
	if err == nil || err.Error() != "Converter 'int' is already registered." {
		t.Errorf("RegisterConverter error = %v", err)
	}
}

// TestWarningOverrideConverter checks that a registered converter cannot be
// registered again.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_warning_override_converter
func TestWarningOverrideConverter(t *testing.T) {
	registerConverter(t, "base64", base64Converter{})
	err := urls.RegisterConverter("base64", base64Converter{})
	if err == nil || err.Error() != "Converter 'base64' is already registered." {
		t.Errorf("RegisterConverter error = %v", err)
	}
}

// pyWhitespace is Python's string.whitespace.
const pyWhitespace = " \t\n\r\x0b\x0c"

// TestWhitespaceInRoute checks that whitespace is refused inside the angle
// brackets and accepted in the literal part of a route.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_whitespace_in_route
func TestWhitespaceInRoute(t *testing.T) {
	for _, ws := range pyWhitespace {
		route := "space/<int:num>/extra/<str:" + string(ws) + "test>"
		t.Run(fmt.Sprintf("%q", ws), func(t *testing.T) {
			_, err := urls.NewRouterChecked(urls.Path(route, emptyView, ""))
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), "cannot contain whitespace in angle brackets <…>") ||
				!strings.HasPrefix(err.Error(), "URL route ") {
				t.Errorf("error = %q", err.Error())
			}
		})
	}
	// Whitespace is valid in paths.
	router := urls.NewRouter(urls.Path("space"+pyWhitespace+"/<int:num>/", emptyView, ""))
	m := mustResolve(t, router, "/space"+pyWhitespace+"/1/")
	assertKwargs(t, "Kwargs", m.Kwargs, urls.Kwargs{"num": 1})
}

// TestPathTrailingNewlines checks that a trailing newline never matches.
//
// Ported from Django: tests/urlpatterns/tests.py::SimplifiedURLTests.test_path_trailing_newlines
func TestPathTrailingNewlines(t *testing.T) {
	router := pathURLs(t)
	for _, url := range []string{
		"/articles/2003/\n",
		"/articles/2010/\n",
		"/en/foo/\n",
		"/included_urls/extra/\n",
		"/regex/1/\n",
		"/users/1/\n",
	} {
		assertResolver404(t, router, url)
	}
}

// converterURLs ports tests/urlpatterns/converter_urls.py.
func converterURLs() *urls.Router {
	names := []string{"int", "path", "slug", "str", "uuid"}
	routes := make([]urls.Route, len(names))
	for i, name := range names {
		routes[i] = urls.Path(name+"/<"+name+":"+name+">/", emptyView, name)
	}
	return urls.NewRouter(routes...)
}

// TestMatchingURLs checks what each built-in converter accepts, what it hands
// over, and that Reverse takes both text and converted values. The uuid
// converter produces a urls.UUID where Django produces uuid.UUID.
//
// Ported from Django: tests/urlpatterns/tests.py::ConverterTests.test_matching_urls
func TestMatchingURLs(t *testing.T) {
	router := converterURLs()
	keep := func(s string) any { return s }
	toInt := func(s string) any {
		n := 0
		for _, c := range s {
			n = n*10 + int(c-'0')
		}
		return n
	}
	toUUID := func(s string) any {
		u, err := urls.ParseUUID(s)
		if err != nil {
			t.Fatalf("ParseUUID(%q): %v", s, err)
		}
		return u
	}
	tests := []struct {
		name     string
		suffixes []string
		convert  func(string) any
		native   bool
	}{
		{"int", []string{"0", "1", "01", "1234567890"}, toInt, true},
		{"str", []string{"abcxyz"}, keep, false},
		{"path", []string{"allows.ANY*characters"}, keep, false},
		{"slug", []string{"abcxyz-ABCXYZ_01234567890"}, keep, false},
		{"uuid", []string{"39da9369-838e-4750-91a5-f7805cd82839"}, toUUID, true},
	}
	for _, tc := range tests {
		for _, suffix := range tc.suffixes {
			url := "/" + tc.name + "/" + suffix + "/"
			t.Run(url, func(t *testing.T) {
				m := mustResolve(t, router, url)
				if m.URLName != tc.name {
					t.Errorf("URLName = %q, want %q", m.URLName, tc.name)
				}
				assertKwargs(t, "Kwargs", m.Kwargs, urls.Kwargs{tc.name: tc.convert(suffix)})
				// Reverse works with string parameters.
				assertReverse(t, router, url, tc.name, urls.Kwargs{tc.name: suffix})
				// Reverse also works with native types. The converted value
				// may differ: a leading zero is lost in the int conversion.
				if tc.native {
					converted := m.Kwargs[tc.name]
					assertReverse(t, router, fmt.Sprintf("/%s/%v/", tc.name, converted), tc.name,
						urls.Kwargs{tc.name: converted})
				}
			})
		}
	}
}

// TestNonmatchingURLs checks what each built-in converter rejects.
//
// Ported from Django: tests/urlpatterns/tests.py::ConverterTests.test_nonmatching_urls
func TestNonmatchingURLs(t *testing.T) {
	router := converterURLs()
	tests := map[string][]string{
		"int":  {"-1", "letters"},
		"str":  {"", "/"},
		"path": {""},
		"slug": {"", "stars*notallowed"},
		"uuid": {
			"",
			"9da9369-838e-4750-91a5-f7805cd82839",
			"39da9369-838-4750-91a5-f7805cd82839",
			"39da9369-838e-475-91a5-f7805cd82839",
			"39da9369-838e-4750-91a-f7805cd82839",
			"39da9369-838e-4750-91a5-f7805cd8283",
		},
	}
	for name, suffixes := range tests {
		for _, suffix := range suffixes {
			assertResolver404(t, router, "/"+name+"/"+suffix+"/")
		}
	}
}

// TestMatchingURLsSameName checks how Reverse chooses among routes that share
// a name: by the number of arguments, by the names of the keyword arguments,
// by what the converter and the expression accept, and by a converter that
// refuses a value.
//
// Ported from Django: tests/urlpatterns/tests.py::SameNameTests.test_matching_urls_same_name
func TestMatchingURLsSameName(t *testing.T) {
	registerConverter(t, "to_url_value_error", &dynamicConverter{
		toGo: func(s string) (any, error) { return s, nil },
		toURL: func(v any) (string, error) {
			if n, ok := v.(int); ok && n > 5 {
				return "", errors.New("value too large")
			}
			return fmt.Sprint(v), nil
		},
	})
	// Ports tests/urlpatterns/path_same_name_urls.py.
	router := urls.NewRouter(
		// Different number of arguments.
		urls.Path("number_of_args/0/", emptyView, "number_of_args"),
		urls.Path("number_of_args/1/<value>/", emptyView, "number_of_args"),
		// Different names of the keyword arguments.
		urls.Path("kwargs_names/a/<a>/", emptyView, "kwargs_names"),
		urls.Path("kwargs_names/b/<b>/", emptyView, "kwargs_names"),
		// Different path converters.
		urls.Path("converter/path/<path:value>/", emptyView, "converter"),
		urls.Path("converter/str/<str:value>/", emptyView, "converter"),
		urls.Path("converter/slug/<slug:value>/", emptyView, "converter"),
		urls.Path("converter/int/<int:value>/", emptyView, "converter"),
		urls.Path("converter/uuid/<uuid:value>/", emptyView, "converter"),
		// Different regular expressions.
		urls.RePath(`^regex/uppercase/([A-Z]+)/`, emptyView, "regex"),
		urls.RePath(`^regex/lowercase/([a-z]+)/`, emptyView, "regex"),
		// The converter's ToURL returns an error (no match).
		urls.Path("converter_to_url/int/<value>/", emptyView, "converter_to_url"),
		urls.Path("converter_to_url/tiny_int/<to_url_value_error:value>/", emptyView, "converter_to_url"),
	)
	tests := []struct {
		name   string
		args   []any
		suffix string
	}{
		{"number_of_args", nil, "0/"},
		{"number_of_args", []any{1}, "1/1/"},
		{"kwargs_names", []any{urls.Kwargs{"a": 1}}, "a/1/"},
		{"kwargs_names", []any{urls.Kwargs{"b": 1}}, "b/1/"},
		{"converter", []any{"a/b"}, "path/a/b/"},
		{"converter", []any{"a b"}, "str/a%20b/"},
		{"converter", []any{"a-b"}, "slug/a-b/"},
		{"converter", []any{"2"}, "int/2/"},
		{"converter", []any{"39da9369-838e-4750-91a5-f7805cd82839"}, "uuid/39da9369-838e-4750-91a5-f7805cd82839/"},
		{"regex", []any{"ABC"}, "uppercase/ABC/"},
		{"regex", []any{"abc"}, "lowercase/abc/"},
		{"converter_to_url", []any{6}, "int/6/"},
		{"converter_to_url", []any{1}, "tiny_int/1/"},
	}
	for _, tc := range tests {
		assertReverse(t, router, "/"+tc.name+"/"+tc.suffix, tc.name, tc.args...)
	}
}

// TestIntegerParameterNameCausesException checks the message for a parameter
// name that starts with a digit.
//
// Ported from Django: tests/urlpatterns/tests.py::ParameterRestrictionTests.test_integer_parameter_name_causes_exception
func TestIntegerParameterNameCausesException(t *testing.T) {
	assertConfigError(t, urls.Path("hello/<int:1>/", emptyView, ""),
		"URL route 'hello/<int:1>/' uses parameter name '1' which isn't a valid Python identifier.")
}

// TestNonIdentifierParameterNameCausesException checks the message for a
// parameter name with punctuation.
//
// Ported from Django: tests/urlpatterns/tests.py::ParameterRestrictionTests.test_non_identifier_parameter_name_causes_exception
func TestNonIdentifierParameterNameCausesException(t *testing.T) {
	assertConfigError(t, urls.Path("b/<int:book.id>/", emptyView, ""),
		"URL route 'b/<int:book.id>/' uses parameter name 'book.id' which isn't a valid Python identifier.")
}

// TestAllowsNonASCIIButValidIdentifiers asserts the Go behavior, which
// differs from Django: a parameter name must be an ASCII identifier, because
// Go's regexp package accepts only ASCII group names. Django accepts any
// Unicode identifier.
//
// Ported from Django: tests/urlpatterns/tests.py::ParameterRestrictionTests.test_allows_non_ascii_but_valid_identifiers
func TestAllowsNonASCIIButValidIdentifiers(t *testing.T) {
	// Δ is "GREEK CAPITAL LETTER DELTA", a valid Python identifier.
	assertConfigError(t, urls.Path("hello/<str:Δ>/", emptyView, ""),
		"uses parameter name 'Δ' which isn't a valid Python identifier.")
}

// dynamicURLs ports tests/urlpatterns/path_dynamic_urls.py with a converter
// whose conversions the test supplies.
func dynamicURLs(t *testing.T, c *dynamicConverter) *urls.Router {
	t.Helper()
	registerConverter(t, "dynamic", c)
	return urls.NewRouter(urls.Path("dynamic/<dynamic:value>/", emptyView, "dynamic"))
}

// TestResolveValueErrorMeansNoMatch checks that an error from ToGo makes the
// route not match.
//
// Ported from Django: tests/urlpatterns/tests.py::ConversionExceptionTests.test_resolve_value_error_means_no_match
func TestResolveValueErrorMeansNoMatch(t *testing.T) {
	router := dynamicURLs(t, &dynamicConverter{
		toGo: func(string) (any, error) { return nil, errors.New("value error") },
	})
	assertResolver404(t, router, "/dynamic/abc/")
}

// TestResolveTypeErrorPropagates asserts the Go behavior, which differs from
// Django. Python tells a ValueError (no match) from every other exception
// (propagates). A Go converter has one error result, and any error means the
// route does not match.
//
// Ported from Django: tests/urlpatterns/tests.py::ConversionExceptionTests.test_resolve_type_error_propagates
func TestResolveTypeErrorPropagates(t *testing.T) {
	router := dynamicURLs(t, &dynamicConverter{
		toGo: func(string) (any, error) { return nil, errors.New("this type error propagates") },
	})
	assertResolver404(t, router, "/dynamic/abc/")
}

// TestReverseValueErrorMeansNoMatch checks that an error from ToURL makes the
// route not reverse.
//
// Ported from Django: tests/urlpatterns/tests.py::ConversionExceptionTests.test_reverse_value_error_means_no_match
func TestReverseValueErrorMeansNoMatch(t *testing.T) {
	router := dynamicURLs(t, &dynamicConverter{
		toURL: func(any) (string, error) { return "", errors.New("value error") },
	})
	_, err := router.Reverse("dynamic", urls.Kwargs{"value": struct{}{}})
	if !errors.Is(err, urls.ErrNoReverseMatch) {
		t.Errorf("Reverse error = %v, want NoReverseMatch", err)
	}
}

// TestReverseTypeErrorPropagates asserts the Go behavior, which differs from
// Django for the reason given at TestResolveTypeErrorPropagates: any error
// from ToURL ends in NoReverseMatch.
//
// Ported from Django: tests/urlpatterns/tests.py::ConversionExceptionTests.test_reverse_type_error_propagates
func TestReverseTypeErrorPropagates(t *testing.T) {
	router := dynamicURLs(t, &dynamicConverter{
		toURL: func(any) (string, error) { return "", errors.New("this type error propagates") },
	})
	_, err := router.Reverse("dynamic", urls.Kwargs{"value": struct{}{}})
	if !errors.Is(err, urls.ErrNoReverseMatch) {
		t.Errorf("Reverse error = %v, want NoReverseMatch", err)
	}
}
