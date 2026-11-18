package templates_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// urlTagRouter is tests/template_tests/urls.py. RE2's \w is ASCII only, so the
// route of "метка_оператора" spells the Unicode word class out.
func urlTagRouter() *urls.Router {
	view := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	ns := []urls.Route{
		urls.Path("", view, "index"),
		urls.RePath(`^client/([0-9,]+)/$`, view, "client"),
		urls.RePath(`^client/(?P<id>[0-9]+)/(?P<action>[^/]+)/$`, view, "client_action"),
		urls.RePath(`^client/(?P<client_id>[0-9]+)/(?P<action>[^/]+)/$`, view, "client_action"),
		urls.RePath(`^named-client/([0-9]+)/$`, view, "named.client"),
	}
	return urls.NewRouter(slices.Concat(
		ns,
		[]urls.Route{
			urls.RePath(`^Юникод/([\p{L}\p{N}_]+)/$`, view, "метка_оператора"),
			urls.RePath(`^Юникод/(?P<tag>\S+)/$`, view, "метка_оператора_2"),
		},
		urls.IncludeApp("ns1/", "app", "ns1", ns...),
		urls.IncludeApp("ns2/", "app", "app", ns...),
	)...)
}

// urlTagEngine returns an engine whose {% url %} reverses against
// urlTagRouter and that has the given named templates.
func urlTagEngine(named map[string]string) (*templates.Engine, *urls.Router) {
	router := urlTagRouter()
	engine := templates.New(templates.MapSource("test", named))
	engine.SetResolver(router.Reverse)
	return engine, router
}

// TestURLTag runs the cases of Django's UrlTagTests that render a URL.
//
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url01
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url02
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url02a
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url02b
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url02c
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url03
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url04
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url05
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url06
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url08
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url09
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url10
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url11
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url12
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url13
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url14
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url15
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url18
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url19
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url20
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url21
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_asvar01
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_asvar02
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_asvar03
func TestURLTag(t *testing.T) {
	client := map[string]any{"client": map[string]any{"id": 1}}
	tests := []struct {
		django   string
		template string
		context  map[string]any
		want     string
	}{
		{"url01", `{% url "client" client.id %}`, client, "/client/1/"},
		{"url02", `{% url "client_action" id=client.id action="update" %}`, client, "/client/1/update/"},
		{"url02a", `{% url "client_action" client.id "update" %}`, client, "/client/1/update/"},
		{"url02b", `{% url 'client_action' id=client.id action='update' %}`, client, "/client/1/update/"},
		{"url02c", `{% url 'client_action' client.id 'update' %}`, client, "/client/1/update/"},
		{"url03", `{% url "index" %}`, nil, "/"},
		{"url04", `{% url "named.client" client.id %}`, client, "/named-client/1/"},
		{"url05", `{% url "метка_оператора" v %}`, map[string]any{"v": "Ω"}, "/%D0%AE%D0%BD%D0%B8%D0%BA%D0%BE%D0%B4/%CE%A9/"},
		{"url06", `{% url "метка_оператора_2" tag=v %}`, map[string]any{"v": "Ω"}, "/%D0%AE%D0%BD%D0%B8%D0%BA%D0%BE%D0%B4/%CE%A9/"},
		{"url08", `{% url "метка_оператора" v %}`, map[string]any{"v": "Ω"}, "/%D0%AE%D0%BD%D0%B8%D0%BA%D0%BE%D0%B4/%CE%A9/"},
		{"url09", `{% url "метка_оператора_2" tag=v %}`, map[string]any{"v": "Ω"}, "/%D0%AE%D0%BD%D0%B8%D0%BA%D0%BE%D0%B4/%CE%A9/"},
		{"url10", `{% url "client_action" id=client.id action="two words" %}`, client, "/client/1/two%20words/"},
		{"url11", `{% url "client_action" id=client.id action="==" %}`, client, "/client/1/==/"},
		{
			"url12", `{% url "client_action" id=client.id action="!$&'()*+,;=~:@," %}`, client,
			"/client/1/!$&amp;&#x27;()*+,;=~:@,/",
		},
		{
			"url13", `{% url "client_action" id=client.id action=arg|join:"-" %}`,
			map[string]any{"client": map[string]any{"id": 1}, "arg": []string{"a", "b"}},
			"/client/1/a-b/",
		},
		{
			"url14", `{% url "client_action" client.id arg|join:"-" %}`,
			map[string]any{"client": map[string]any{"id": 1}, "arg": []string{"a", "b"}},
			"/client/1/a-b/",
		},
		{"url15", `{% url "client_action" 12 "test" %}`, nil, "/client/12/test/"},
		{"url18", `{% url "client" "1,2" %}`, nil, "/client/1,2/"},
		{
			"url19", `{% url named_url client.id %}`,
			map[string]any{"client": map[string]any{"id": 1}, "named_url": "client"},
			"/client/1/",
		},
		{
			"url20", `{% url url_name_in_var client.id %}`,
			map[string]any{"client": map[string]any{"id": 1}, "url_name_in_var": "named.client"},
			"/named-client/1/",
		},
		{
			"url21",
			`{% autoescape off %}{% url "client_action" id=client.id action="!$&'()*+,;=~:@," %}{% endautoescape %}`,
			client, "/client/1/!$&'()*+,;=~:@,/",
		},
		{"url-asvar01", `{% url "index" as url %}`, nil, ""},
		{"url-asvar02", `{% url "index" as url %}{{ url }}`, nil, "/"},
		{"url-asvar03", `{% url "no_such_view" as url %}{{ url }}`, nil, ""},
	}
	engine, _ := urlTagEngine(nil)
	for _, tt := range tests {
		t.Run(tt.django, func(t *testing.T) {
			got, err := engine.RenderString(tt.template, tt.context)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestURLTagSyntaxErrors runs the cases Django rejects with
// TemplateSyntaxError. Here the template fails to compile, with an error that
// is not a NoReverseMatch. Django's url-fail07 and url-fail17 ("a.id!id") are
// left out: pongo2 reads "!" as its negation operator, so the tag is valid.
//
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail01
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail04
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail05
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail06
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail08
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail09
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail14
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail15
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail16
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail18
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail19
func TestURLTagSyntaxErrors(t *testing.T) {
	tests := []struct {
		django   string
		template string
	}{
		{"url-fail01", `{% url %}`},
		{"url-fail04", `{% url "view" id, %}`},
		{"url-fail05", `{% url "view" id= %}`},
		{"url-fail06", `{% url "view" a.id=id %}`},
		{"url-fail08", `{% url "view" id="unterminatedstring %}`},
		{"url-fail09", `{% url "view" id=", %}`},
		{"url-fail14", `{% url named_url id, %}`},
		{"url-fail15", `{% url named_url id= %}`},
		{"url-fail16", `{% url named_url a.id=id %}`},
		{"url-fail18", `{% url named_url id="unterminatedstring %}`},
		{"url-fail19", `{% url named_url id=", %}`},
	}
	engine, _ := urlTagEngine(nil)
	for _, tt := range tests {
		t.Run(tt.django, func(t *testing.T) {
			_, err := engine.RenderString(tt.template, map[string]any{"named_url": "view"})
			if err == nil {
				t.Fatal("the template rendered, want a syntax error")
			}
			if errors.Is(err, urls.ErrNoReverseMatch) {
				t.Errorf("got a NoReverseMatch, want a syntax error: %v", err)
			}
		})
	}
}

// TestURLTagNoReverseMatch checks that a route that does not reverse fails the
// render with the router's NoReverseMatch.
//
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail02
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail03
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail11
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail12
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_fail13
func TestURLTagNoReverseMatch(t *testing.T) {
	tests := []struct {
		django   string
		template string
		context  map[string]any
	}{
		{"url-fail02", `{% url "no_such_view" %}`, nil},
		{"url-fail03", `{% url "client" %}`, nil},
		{"url-fail11", `{% url named_url %}`, nil},
		{"url-fail12", `{% url named_url %}`, map[string]any{"named_url": "no_such_view"}},
		{"url-fail13", `{% url named_url %}`, map[string]any{"named_url": "template_tests.views.badview"}},
	}
	engine, _ := urlTagEngine(nil)
	for _, tt := range tests {
		t.Run(tt.django, func(t *testing.T) {
			_, err := engine.RenderString(tt.template, tt.context)
			if !errors.Is(err, urls.ErrNoReverseMatch) {
				t.Fatalf("error = %v, want a NoReverseMatch", err)
			}
			var nrm *urls.NoReverseMatch
			if !errors.As(err, &nrm) {
				t.Errorf("error %v does not carry *urls.NoReverseMatch", err)
			}
		})
	}
}

// TestURLTagNamespaces checks which instance of a namespaced app {% url %}
// picks. Django reads request.resolver_match and request.current_app; here the
// match travels in the request context (urls.WithMatch, set by the router)
// and the explicit application is set with templates.WithCurrentApp.
//
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_namespace01
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_namespace02
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_namespace03
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_namespace_no_current_app
// Ported from Django: tests/template_tests/syntax_tests/test_url.py::UrlTagTests.test_url_namespace_explicit_current_app
func TestURLTagNamespaces(t *testing.T) {
	none := "<none>"
	tests := []struct {
		django     string
		resolved   string
		currentApp string
		want       string
	}{
		{"url-namespace01", "/ns1/", none, "/ns1/named-client/42/"},
		{"url-namespace02", "/ns2/", none, "/ns2/named-client/42/"},
		{"url-namespace03", "", none, "/ns2/named-client/42/"},
		{"url-namespace-no-current-app", "/ns1/", "", "/ns2/named-client/42/"},
		{"url-namespace-explicit-current-app", "/ns1/", "app", "/ns2/named-client/42/"},
	}
	engine, router := urlTagEngine(map[string]string{"tpl": `{% url "app:named.client" 42 %}`})
	for _, tt := range tests {
		t.Run(tt.django, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.resolved != "" {
				m, err := router.Resolve(tt.resolved)
				if err != nil {
					t.Fatalf("resolve %s: %v", tt.resolved, err)
				}
				r = r.WithContext(urls.WithMatch(r.Context(), m))
			}
			if tt.currentApp != none {
				r = r.WithContext(templates.WithCurrentApp(r.Context(), tt.currentApp))
			}
			got, err := engine.RenderRequest(r, "tpl", nil)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
