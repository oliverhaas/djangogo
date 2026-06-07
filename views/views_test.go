package views_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/views"
)

// vArticle is the demo model exercised by the generic view tests.
type vArticle struct {
	ID    int64
	Title string `orm:"max_length=200"`
}

// newViewsDB builds a fresh in-memory SQLite DB registering vArticle, creates the
// table, and returns the handle.
func newViewsDB(t *testing.T) *orm.DB {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&vArticle{}); err != nil {
		t.Fatalf("Register(vArticle): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	m, ok := reg.Get("vArticle")
	if !ok {
		t.Fatal("vArticle not registered")
	}
	if err := db.CreateTable(context.Background(), m); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	return db
}

// newViewsEngine writes the named templates into a temp dir and returns an Engine.
func newViewsEngine(t *testing.T, files map[string]string) *templates.Engine {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	eng, err := templates.NewEngine(dir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
}

func TestRender(t *testing.T) {
	eng := newViewsEngine(t, map[string]string{
		"hello.html": "Hello {{ name }}!",
	})
	rec := httptest.NewRecorder()
	if err := views.Render(rec, eng, "hello.html", map[string]any{"name": "world"}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if got := rec.Body.String(); got != "Hello world!" {
		t.Errorf("body = %q, want %q", got, "Hello world!")
	}
}

func TestRenderMissingTemplate(t *testing.T) {
	eng := newViewsEngine(t, map[string]string{"a.html": "ok"})
	rec := httptest.NewRecorder()
	err := views.Render(rec, eng, "missing.html", nil)
	if err == nil {
		t.Fatal("Render: want error for missing template")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestRedirect(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	views.Redirect(rec, req, "/login/", http.StatusFound)
	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login/" {
		t.Errorf("Location = %q, want /login/", loc)
	}
}

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := views.JSON(rec, http.StatusCreated, map[string]any{"ok": true}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["ok"] != true {
		t.Errorf("body = %v, want ok:true", got)
	}
}

func TestDetailView(t *testing.T) {
	db := newViewsDB(t)
	eng := newViewsEngine(t, map[string]string{
		"article.html": "Title: {{ object.Title }}",
	})
	art := vArticle{Title: "Hello"}
	if err := orm.Query[vArticle](db).Create(context.Background(), &art); err != nil {
		t.Fatalf("Create: %v", err)
	}

	view := views.DetailView[vArticle]{DB: db, Engine: eng, Template: "article.html"}
	mux := http.NewServeMux()
	mux.Handle("GET /articles/{pk}/", view)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/articles/1/", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "Title: Hello" {
		t.Errorf("body = %q, want %q", got, "Title: Hello")
	}
}

func TestDetailViewNotFound(t *testing.T) {
	db := newViewsDB(t)
	eng := newViewsEngine(t, map[string]string{"article.html": "{{ object.Title }}"})

	view := views.DetailView[vArticle]{DB: db, Engine: eng, Template: "article.html"}
	mux := http.NewServeMux()
	mux.Handle("GET /articles/{pk}/", view)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/articles/999/", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDetailViewBadPK(t *testing.T) {
	db := newViewsDB(t)
	eng := newViewsEngine(t, map[string]string{"article.html": "{{ object.Title }}"})

	view := views.DetailView[vArticle]{DB: db, Engine: eng, Template: "article.html"}
	mux := http.NewServeMux()
	mux.Handle("GET /articles/{pk}/", view)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/articles/notanint/", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for non-integer pk", rec.Code)
	}
}

func TestDetailViewCustomNames(t *testing.T) {
	db := newViewsDB(t)
	eng := newViewsEngine(t, map[string]string{
		"article.html": "{{ art.Title }}|{{ extra }}",
	})
	art := vArticle{Title: "Custom"}
	if err := orm.Query[vArticle](db).Create(context.Background(), &art); err != nil {
		t.Fatalf("Create: %v", err)
	}

	view := views.DetailView[vArticle]{
		DB:          db,
		Engine:      eng,
		Template:    "article.html",
		PKParam:     "id",
		ContextName: "art",
		Extra:       func(_ *http.Request) map[string]any { return map[string]any{"extra": "X"} },
	}
	mux := http.NewServeMux()
	mux.Handle("GET /articles/{id}/", view)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/articles/1/", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "Custom|X" {
		t.Errorf("body = %q, want %q", got, "Custom|X")
	}
}

func TestListView(t *testing.T) {
	db := newViewsDB(t)
	eng := newViewsEngine(t, map[string]string{
		"list.html": "{% for o in objects %}{{ o.Title }};{% endfor %}",
	})
	for _, title := range []string{"A", "B", "C"} {
		art := vArticle{Title: title}
		if err := orm.Query[vArticle](db).Create(context.Background(), &art); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	view := views.ListView[vArticle]{DB: db, Engine: eng, Template: "list.html"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/articles/", nil)
	view.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "A;B;C;" {
		t.Errorf("body = %q, want %q", got, "A;B;C;")
	}
}

func TestListViewCustomQuery(t *testing.T) {
	db := newViewsDB(t)
	eng := newViewsEngine(t, map[string]string{
		"list.html": "{% for o in rows %}{{ o.Title }};{% endfor %}",
	})
	for _, title := range []string{"keep", "drop", "keep"} {
		art := vArticle{Title: title}
		if err := orm.Query[vArticle](db).Create(context.Background(), &art); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	view := views.ListView[vArticle]{
		DB:          db,
		Engine:      eng,
		Template:    "list.html",
		ContextName: "rows",
		Query: func(d *orm.DB) *orm.QuerySet[vArticle] {
			return orm.Query[vArticle](d).Filter("title", "keep")
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/articles/", nil)
	view.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "keep;keep;" {
		t.Errorf("body = %q, want %q", got, "keep;keep;")
	}
}
