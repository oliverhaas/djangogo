package views_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// Artist is the Artist model of Django's tests/generic_views/models.py.
type Artist struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Author is the Author model of Django's tests/generic_views/models.py. The Go
// ORM has no Meta.ordering, so the list views of the ports order by name
// themselves.
type Author struct {
	ID   int64
	Name string `orm:"max_length=100"`
	Slug string `orm:"max_length=50"`
}

// Book is the Book model of Django's tests/generic_views/models.py, without
// its many-to-many relation to Author, which no ported test reads.
type Book struct {
	ID      int64
	Name    string `orm:"max_length=255"`
	Slug    string `orm:"max_length=50"`
	Pages   int
	Pubdate time.Time
}

// Page is the Page model of Django's tests/generic_views/models.py.
type Page struct {
	ID       int64
	Content  string `orm:"type=text"`
	Template string `orm:"max_length=255"`
}

// genericModels returns the models under Django's app label, which makes the
// default template names "generic_views/<model>_<suffix>.html".
func genericModels() []any {
	return []any{djangotest.App("generic_views", &Artist{}, &Author{}, &Book{}, &Page{})}
}

// genericEngine returns an engine with the templates of Django's
// tests/generic_views/templates the ported tests render. 404.html is added so
// a test can read the message of a 404 from its context.
func genericEngine(t *testing.T) *templates.Engine {
	t.Helper()
	object := "{{ object.name }}"
	list := "{% for item in object_list %}{{ item.name }}{{ item.first }};{% endfor %}"
	return memEngine(t, map[string]string{
		"404.html":                                 "{{ exception }}",
		"generic_views/about.html":                 "<h1>About</h1>",
		"generic_views/robots.txt":                 "User-Agent: *\n",
		"generic_views/list.html":                  list,
		"generic_views/author_list.html":           list,
		"generic_views/author_objects.html":        list,
		"generic_views/book_list.html":             list,
		"generic_views/detail.html":                "{{ object.foo }}",
		"generic_views/artist_detail.html":         object,
		"generic_views/author_detail.html":         object,
		"generic_views/author_view.html":           object,
		"generic_views/page_template.html":         "{{ object.content }}",
		"generic_views/confirm_delete.html":        object,
		"generic_views/author_confirm_delete.html": object,
	})
}

// genericData holds the rows that setUpTestData of Django's generic view tests
// creates.
type genericData struct {
	artist1          Artist
	author1, author2 Author
	book1, book2     Book
	page1            Page
}

// create inserts obj and fails the test on an error.
func create[T any](t *testing.T, db *orm.DB, obj *T) {
	t.Helper()
	if err := orm.Query[T](db).Create(context.Background(), obj); err != nil {
		t.Fatalf("create %T: %v", obj, err)
	}
}

// genericFixture creates the rows of setUpTestData.
func genericFixture(t *testing.T, db *orm.DB) genericData {
	t.Helper()
	d := genericData{
		artist1: Artist{Name: "Rene Magritte"},
		author1: Author{Name: "Roberto Bolaño", Slug: "roberto-bolano"},
		author2: Author{Name: "Scott Rosenberg", Slug: "scott-rosenberg"},
		book1: Book{
			Name: "2066", Slug: "2066", Pages: 800,
			Pubdate: time.Date(2008, 10, 1, 0, 0, 0, 0, time.UTC),
		},
		book2: Book{
			Name: "Dreaming in Code", Slug: "dreaming-in-code", Pages: 300,
			Pubdate: time.Date(2006, 5, 1, 0, 0, 0, 0, time.UTC),
		},
		page1: Page{Content: "I was once bitten by a moose.", Template: "generic_views/page_template.html"},
	}
	create(t, db, &d.artist1)
	create(t, db, &d.author1)
	create(t, db, &d.author2)
	create(t, db, &d.book1)
	create(t, db, &d.book2)
	create(t, db, &d.page1)
	return d
}

// eachGenericDB runs fn once per database dialect, each time on a fresh
// database with the rows of setUpTestData.
func eachGenericDB(t *testing.T, fn func(t *testing.T, db *orm.DB, d genericData)) {
	t.Helper()
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		fn(t, db, genericFixture(t, db))
	})
}

// makeAuthors replaces all authors by n authors named "Author 00", "Author
// 01" and so on, Django's _make_authors.
func makeAuthors(t *testing.T, db *orm.DB, n int) {
	t.Helper()
	if _, err := orm.Query[Author](db).Delete(context.Background()); err != nil {
		t.Fatalf("delete authors: %v", err)
	}
	for i := range n {
		create(t, db, &Author{Name: fmt.Sprintf("Author %02d", i), Slug: fmt.Sprintf("a%d", i)})
	}
}

// allOf returns every row of T ordered by the given columns.
func allOf[T any](t *testing.T, db *orm.DB, ordering ...string) []T {
	t.Helper()
	rows, err := orm.Query[T](db).OrderBy(ordering...).All(context.Background())
	if err != nil {
		t.Fatalf("load rows: %v", err)
	}
	return rows
}

// mount returns a test client for views mounted on route patterns, the
// counterpart of the lines of Django's tests/generic_views/urls.py a test
// uses. routes alternates pattern and handler.
func mount(t *testing.T, routes ...any) *djangotest.Client {
	t.Helper()
	mux := http.NewServeMux()
	for i := 0; i+1 < len(routes); i += 2 {
		mux.Handle(routes[i].(string), routes[i+1].(http.Handler))
	}
	return djangotest.NewClient(t, mux)
}

// contextValue returns the entry of the template context and fails the test
// when the key is missing.
func contextValue(t *testing.T, resp *djangotest.Response, key string) any {
	t.Helper()
	v, ok := resp.Context(key)
	if !ok {
		t.Fatalf("context has no key %q", key)
	}
	return v
}

// contextList returns a list entry of the template context.
func contextList(t *testing.T, resp *djangotest.Response, key string) []any {
	t.Helper()
	list, ok := contextValue(t, resp, key).([]any)
	if !ok {
		t.Fatalf("context[%q] is %T, want a list", key, contextValue(t, resp, key))
	}
	return list
}

// contextMap returns a map entry of the template context: the template form of
// a page or a paginator.
func contextMap(t *testing.T, resp *djangotest.Response, key string) map[string]any {
	t.Helper()
	m, ok := contextValue(t, resp, key).(map[string]any)
	if !ok {
		t.Fatalf("context[%q] is %T, want a map", key, contextValue(t, resp, key))
	}
	return m
}

// assertNotInContext asserts that the template context has no such key.
func assertNotInContext(t *testing.T, resp *djangotest.Response, key string) {
	t.Helper()
	if _, ok := resp.Context(key); ok {
		t.Errorf("context has the key %q", key)
	}
}

// assertContextObject asserts that the context entry is the model instance
// want. A template receives a model instance as its templates.ModelMap, so
// that is what the entries are compared by.
func assertContextObject[T any](t *testing.T, db *orm.DB, resp *djangotest.Response, key string, want T) {
	t.Helper()
	m, ok := db.Registry().ModelOf(want)
	if !ok {
		t.Fatalf("%T is not a registered model", want)
	}
	djangotest.AssertEqual(t, contextValue(t, resp, key), any(templates.ModelContext(m, want)))
}

// assertContextObjects asserts that the context entry is the list of the model
// instances want, in order.
func assertContextObjects[T any](t *testing.T, db *orm.DB, resp *djangotest.Response, key string, want []T) {
	t.Helper()
	var zero T
	m, ok := db.Registry().ModelOf(zero)
	if !ok {
		t.Fatalf("%T is not a registered model", zero)
	}
	wantMaps := make([]any, len(want))
	for i, obj := range want {
		wantMaps[i] = templates.ModelContext(m, obj)
	}
	djangotest.AssertSequenceEqual(t, contextList(t, resp, key), wantMaps)
}

// assertSameList asserts that two context keys hold the same list, Django's
// assertIs on the two entries.
func assertSameList(t *testing.T, resp *djangotest.Response, a, b string) {
	t.Helper()
	la, lb := contextList(t, resp, a), contextList(t, resp, b)
	if len(la) != len(lb) || (len(la) > 0 && &la[0] != &lb[0]) {
		t.Errorf("context[%q] and context[%q] are not the same list", a, b)
	}
}

// objectName returns the name column of a model instance in a template
// context.
func objectName(t *testing.T, obj any) string {
	t.Helper()
	m, ok := obj.(templates.ModelMap)
	if !ok {
		t.Fatalf("list item is %T, want templates.ModelMap", obj)
	}
	name, _ := m["name"].(string)
	return name
}

// assertImproperlyConfigured asserts that the response is the 500 page and
// that its logged cause is a views.ErrImproperlyConfigured whose message
// contains want. Django raises ImproperlyConfigured out of the test client
// where the Go view answers with the 500 page and logs the cause.
func assertImproperlyConfigured(t *testing.T, logs *logRecords, resp *djangotest.Response, want string) {
	t.Helper()
	djangotest.AssertStatus(t, resp, http.StatusInternalServerError)
	errs := logs.errs()
	if len(errs) != 1 {
		t.Fatalf("%d errors logged, want 1: %v", len(errs), errs)
	}
	djangotest.AssertErrorIs(t, errs[0], views.ErrImproperlyConfigured)
	if !strings.Contains(errs[0].Error(), want) {
		t.Errorf("error %q does not contain %q", errs[0], want)
	}
}
