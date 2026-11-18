package templates_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/urls"
)

// requestAttrs is Django's context_processors/request_attrs.html.
const requestAttrs = `{% if request %}
Have request
{% else %}
No request
{% endif %}

{% if request.is_secure %}
Secure
{% else %}
Not secure
{% endif %}

{{ request.path }}
`

// TestRequestAttributes checks that the request is in the template and that
// GET and POST parameters cannot override its attributes (Django ticket 3828).
//
// Ported from Django: tests/context_processors/tests.py::RequestContextProcessorTests.test_request_attributes
func TestRequestAttributes(t *testing.T) {
	engine := templates.New(templates.MapSource("app", map[string]string{
		"context_processors/request_attrs.html": requestAttrs,
	}))
	engine.AddContextProcessor(templates.RequestProcessor)
	render := func(r *http.Request) string {
		t.Helper()
		out, err := engine.RenderRequest(r, "context_processors/request_attrs.html", nil)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return out
	}
	post := func(target string, form url.Values) *http.Request {
		r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	const target = "/request_attrs/"

	out := render(httptest.NewRequest(http.MethodGet, target, nil))
	if !strings.Contains(out, "Have request") || !strings.Contains(out, "Not secure") || !strings.Contains(out, target) {
		t.Errorf("plain GET rendered %q", out)
	}
	for _, r := range []*http.Request{
		httptest.NewRequest(http.MethodGet, target+"?is_secure=blah", nil),
		post(target, url.Values{"is_secure": {"blah"}}),
	} {
		if out := render(r); !strings.Contains(out, "Not secure") {
			t.Errorf("%s with an is_secure parameter rendered %q", r.Method, out)
		}
	}
	for _, r := range []*http.Request{
		httptest.NewRequest(http.MethodGet, target+"?path=/blah/", nil),
		post(target, url.Values{"path": {"/blah/"}}),
	} {
		if out := render(r); !strings.Contains(out, target) || strings.Contains(out, "/blah/") {
			t.Errorf("%s with a path parameter rendered %q", r.Method, out)
		}
	}

	secure := httptest.NewRequest(http.MethodGet, target, nil)
	secure.TLS = &tls.ConnectionState{}
	if out := render(secure); !strings.Contains(out, "Secure") || strings.Contains(out, "Not secure") {
		t.Errorf("a TLS request rendered %q", out)
	}
}

// TestRequestProcessorAttributes checks the Django attribute names of the
// request view.
func TestRequestProcessorAttributes(t *testing.T) {
	const src = `{{ request.method }}|{{ request.scheme }}|{{ request.get_host }}|{{ request.get_full_path }}|` +
		`{{ request.GET.page }}|{{ request.POST.title }}|{{ request.COOKIES.sid }}|{{ request.headers.Accept }}|` +
		`{{ request.resolver_match.url_name }}|{{ request.resolver_match.view_name }}|{{ request.resolver_match.kwargs.year }}`
	engine := templates.New(templates.MapSource("app", map[string]string{"r.html": src}))
	engine.AddContextProcessor(templates.RequestProcessor)

	var out string
	router := urls.NewRouter(urls.IncludeApp("blog/", "blog", "blog",
		urls.PathFunc("<int:year>/", func(_ http.ResponseWriter, r *http.Request) {
			var err error
			if out, err = engine.RenderRequest(r, "r.html", nil); err != nil {
				t.Errorf("render: %v", err)
			}
		}, "archive"),
	)...)
	r := httptest.NewRequest(http.MethodPost, "/blog/2024/?page=2&page=3", strings.NewReader("title=T"))
	r.Host = "example.com"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "text/html")
	r.AddCookie(&http.Cookie{Name: "sid", Value: "abc"})
	router.ServeHTTP(httptest.NewRecorder(), r)

	want := "POST|http|example.com|/blog/2024/?page=2&amp;page=3|3|T|abc|text/html|archive|blog:archive|2024"
	if out != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

// TestRequestProcessorWithoutRouter checks a request that did not come
// through a router: resolver_match is empty.
func TestRequestProcessorWithoutRouter(t *testing.T) {
	engine := templates.New(templates.MapSource("app", map[string]string{
		"r.html": `[{{ request.resolver_match.url_name }}]{% if request.resolver_match %}match{% endif %}`,
	}))
	engine.AddContextProcessor(templates.RequestProcessor)
	out, err := engine.RenderRequest(httptest.NewRequest(http.MethodGet, "/", nil), "r.html", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != "[]" {
		t.Errorf("got %q, want %q", out, "[]")
	}
}

// TestDebug checks the debug flag: set for an internal address while debug is
// on, absent otherwise. Django's test_sql_queries is not ported: the engine
// has no query log to show (sql_queries is not provided).
//
// Ported from Django: tests/context_processors/tests.py::DebugContextProcessorTests.test_debug
func TestDebug(t *testing.T) {
	const src = `{% if debug == true %}Have debug{% else %}No debug{% endif %}`
	tests := []struct {
		name   string
		debug  bool
		ips    []string
		remote string
		want   string
	}{
		{"internal address with debug on", true, []string{"127.0.0.1"}, "127.0.0.1:4711", "Have debug"},
		{"IPv6 loopback", true, []string{"::1"}, "[::1]:4711", "Have debug"},
		{"foreign address", true, []string{"127.0.0.1"}, "10.0.0.7:4711", "No debug"},
		{"debug off", false, []string{"127.0.0.1"}, "127.0.0.1:4711", "No debug"},
		{"no internal addresses", true, nil, "127.0.0.1:4711", "No debug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := templates.New(templates.MapSource("app", map[string]string{"debug.html": src}))
			engine.AddContextProcessor(templates.DebugProcessor(tt.debug, tt.ips...))
			r := httptest.NewRequest(http.MethodGet, "/debug/", nil)
			r.RemoteAddr = tt.remote
			out, err := engine.RenderRequest(r, "debug.html", nil)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if out != tt.want {
				t.Errorf("got %q, want %q", out, tt.want)
			}
		})
	}
}

// TestStaticProcessor checks that STATIC_URL carries the static prefix.
func TestStaticProcessor(t *testing.T) {
	engine := templates.New(templates.MapSource("app", map[string]string{"s.html": `{{ STATIC_URL }}css/site.css`}))
	engine.AddContextProcessor(templates.StaticProcessor)
	out, err := engine.RenderRequest(httptest.NewRequest(http.MethodGet, "/", nil), "s.html", nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := templates.StaticBase + "css/site.css"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
