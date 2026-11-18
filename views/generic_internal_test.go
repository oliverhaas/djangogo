package views

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
)

// urlAuthor is a model whose GetAbsoluteURL has a value receiver.
type urlAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// GetAbsoluteURL returns the detail URL of the author.
func (a urlAuthor) GetAbsoluteURL() string { return "/authors/" + escapeURLValue(a.ID) + "/" }

// urlBook is a model whose GetAbsoluteURL has a pointer receiver.
type urlBook struct {
	ID    int64
	Title string `orm:"max_length=100"`
}

// GetAbsoluteURL returns the detail URL of the book.
func (b *urlBook) GetAbsoluteURL() string { return "/books/" + escapeURLValue(b.ID) + "/" }

// urlHTTPLogEntry is a model without GetAbsoluteURL and with a name of several
// words.
type urlHTTPLogEntry struct {
	ID int64
}

func TestEscapeURLValue(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{42, "42"},
		{"Az09-_.~", "Az09-_.~"},
		{"a b", "a%20b"},
		{"a/b?c=d&e#f", "a%2Fb%3Fc%3Dd%26e%23f"},
		{"//example.com", "%2F%2Fexample.com"},
		{"é", "%C3%A9"},
		{"%2F", "%252F"},
	} {
		if got := escapeURLValue(tc.value); got != tc.want {
			t.Errorf("escapeURLValue(%#v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestFormatURL(t *testing.T) {
	values := map[string]any{"id": 7, "name": "a/b"}
	lookup := func(name string) (any, bool) {
		v, ok := values[name]
		return v, ok
	}
	for _, tc := range []struct{ pattern, want string }{
		{"/authors/", "/authors/"},
		{"/authors/{id}/", "/authors/7/"},
		{"/authors/{id}/?name={name}&again={id}", "/authors/7/?name=a%2Fb&again=7"},
		{"/authors/{id", "/authors/{id"},
		{"/%C3%A9dit/{id}/", "/%C3%A9dit/7/"},
	} {
		got, err := formatURL(tc.pattern, lookup)
		if err != nil {
			t.Errorf("formatURL(%q): %v", tc.pattern, err)
			continue
		}
		if got != tc.want {
			t.Errorf("formatURL(%q) = %q, want %q", tc.pattern, got, tc.want)
		}
	}

	_, err := formatURL("/authors/{missing}/", lookup)
	if !errors.Is(err, ErrImproperlyConfigured) {
		t.Errorf("formatURL with an unknown placeholder: err = %v, want ErrImproperlyConfigured", err)
	}
}

func TestSuccessURL(t *testing.T) {
	db := djangotest.NewDB(t, &urlAuthor{}, &urlBook{}, &urlHTTPLogEntry{})

	got, err := successURL(db, "/authors/{pk}/edit/?name={name}", urlAuthor{ID: 3, Name: "a&b"})
	if err != nil || got != "/authors/3/edit/?name=a%26b" {
		t.Errorf("explicit URL: got %q, %v", got, err)
	}
	got, err = successURL(db, "", urlAuthor{ID: 3})
	if err != nil || got != "/authors/3/" {
		t.Errorf("GetAbsoluteURL with a value receiver: got %q, %v", got, err)
	}
	got, err = successURL(db, "", urlBook{ID: 4})
	if err != nil || got != "/books/4/" {
		t.Errorf("GetAbsoluteURL with a pointer receiver: got %q, %v", got, err)
	}
	if _, err = successURL(db, "", urlHTTPLogEntry{ID: 5}); !errors.Is(err, ErrImproperlyConfigured) {
		t.Errorf("no URL and no GetAbsoluteURL: err = %v, want ErrImproperlyConfigured", err)
	}
	if _, err = successURL(db, "/authors/{nope}/", urlAuthor{ID: 3}); !errors.Is(err, ErrImproperlyConfigured) {
		t.Errorf("unknown column: err = %v, want ErrImproperlyConfigured", err)
	}
}

func TestModelNames(t *testing.T) {
	db := djangotest.NewDB(t, djangotest.App("library", &urlAuthor{}), &urlHTTPLogEntry{})

	author := modelFor[urlAuthor](db)
	if got := modelName(author); got != "urlauthor" {
		t.Errorf("modelName = %q, want %q", got, "urlauthor")
	}
	if got := verboseName(author); got != "url author" {
		t.Errorf("verboseName = %q, want %q", got, "url author")
	}
	if got := modelTemplateName(author, "_detail"); got != "library/urlauthor_detail.html" {
		t.Errorf("modelTemplateName = %q, want %q", got, "library/urlauthor_detail.html")
	}

	entry := modelFor[urlHTTPLogEntry](db)
	if got := verboseName(entry); got != "url http log entry" {
		t.Errorf("verboseName = %q, want %q", got, "url http log entry")
	}
	if got := modelTemplateName(entry, "_list"); got != "urlhttplogentry_list.html" {
		t.Errorf("modelTemplateName = %q, want %q", got, "urlhttplogentry_list.html")
	}

	if modelFor[urlBook](db) != nil || modelFor[urlAuthor](nil) != nil {
		t.Error("modelFor returned a model for an unregistered type or a nil DB")
	}
}

func TestPathValues(t *testing.T) {
	var got []pathValue
	record := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = pathValues(r) })
	mux := http.NewServeMux()
	mux.Handle("GET /authors/{pk}/books/{slug}/{$}", record)
	mux.Handle("/files/{path...}", record)
	mux.Handle("/plain/", record)

	for _, tc := range []struct {
		target string
		want   []pathValue
	}{
		{"/authors/7/books/a%2Fb/", []pathValue{{"pk", "7"}, {"slug", "a/b"}}},
		{"/files/a/b.txt", []pathValue{{"path", "a/b.txt"}}},
		{"/plain/", nil},
	} {
		got = nil
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.target, nil))
		if !slices.Equal(got, tc.want) {
			t.Errorf("pathValues for %s = %v, want %v", tc.target, got, tc.want)
		}
	}

	unrouted := pathValues(httptest.NewRequest(http.MethodGet, "/authors/7/", nil))
	if len(unrouted) != 0 {
		t.Errorf("pathValues of an unrouted request = %v, want none", unrouted)
	}
}
