package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/urls"
)

// Article is the demo model registered with the admin in these tests.
type Article struct {
	ID        int64
	Title     string `orm:"max_length=200"`
	Views     int64
	Published bool
}

// newArticleDB builds a frozen registry, opens an in-memory SQLite database,
// creates the articles table, seeds rows, and returns the DB handle.
func newArticleDB(t *testing.T) *orm.DB {
	t.Helper()

	reg := orm.NewRegistry()
	if _, err := reg.Register(&Article{}); err != nil {
		t.Fatalf("Register(Article): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	model, ok := reg.Get("Article")
	if !ok {
		t.Fatal("Article model not found in registry")
	}
	if err := db.CreateTable(context.Background(), model); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	return db
}

// seedArticles creates three Article rows and returns them with their IDs.
func seedArticles(t *testing.T, db *orm.DB) []Article {
	t.Helper()
	ctx := context.Background()
	arts := []Article{
		{Title: "Hello World", Views: 10, Published: true},
		{Title: "Go Generics", Views: 20, Published: false},
		{Title: "Admin Site", Views: 30, Published: true},
	}
	for i := range arts {
		if err := orm.Query[Article](db).Create(ctx, &arts[i]); err != nil {
			t.Fatalf("Create(%s): %v", arts[i].Title, err)
		}
	}
	return arts
}

// newSite builds an AdminSite over db with Article registered and a fixed
// ListDisplay/Ordering, failing the test on error.
func newSite(t *testing.T, db *orm.DB) *admin.AdminSite {
	t.Helper()
	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	admin.Register[Article](site, admin.ModelAdmin{
		ListDisplay: []string{"ID", "Title", "Views"},
		Ordering:    []string{"id"},
	})
	return site
}

// staffInjector wraps next, placing a staff user into each request's context to
// simulate an authenticated staff session.
func staffInjector(next http.Handler) http.Handler {
	u := &auth.User{ID: 1, Username: "admin", IsStaff: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
	})
}

// staffCSRF wraps next with a staff user, a fresh session, and the CSRF
// middleware so a token is seeded and exposed via csrf.Token(ctx). It models the
// real middleware chain the admin runs under so handlers can read the token and
// render {% csrf_token %}.
func staffCSRF(next http.Handler) http.Handler {
	u := &auth.User{ID: 1, Username: "admin", IsStaff: true}
	withSession := func(inner http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithUser(r.Context(), u)
			ctx = sessions.NewContext(ctx, &sessions.Session{})
			inner.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	return withSession(csrf.Middleware(next))
}

func TestIndexListsModels(t *testing.T) {
	db := newArticleDB(t)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Article") {
		t.Fatalf("index body missing %q:\n%s", "Article", body)
	}
	if !strings.Contains(body, "/admin/article/") {
		t.Fatalf("index body missing changelist link:\n%s", body)
	}
}

func TestChangelistRendersRowsAndColumns(t *testing.T) {
	db := newArticleDB(t)
	seedArticles(t, db)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/article/", nil)
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("changelist status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, col := range []string{"ID", "Title", "Views"} {
		if !strings.Contains(body, col) {
			t.Errorf("changelist body missing column %q:\n%s", col, body)
		}
	}
	for _, title := range []string{"Hello World", "Go Generics", "Admin Site"} {
		if !strings.Contains(body, title) {
			t.Errorf("changelist body missing title %q:\n%s", title, body)
		}
	}
	// Per-row change link uses the pk.
	if !strings.Contains(body, "/admin/article/1/change/") {
		t.Errorf("changelist body missing change link:\n%s", body)
	}
}

func TestChangelistRedirectsNonStaff(t *testing.T) {
	db := newArticleDB(t)
	seedArticles(t, db)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	// Anonymous (no user injected).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/article/", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("anonymous changelist status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, site.LoginURL) {
		t.Fatalf("anonymous redirect Location = %q, want prefix %q", loc, site.LoginURL)
	}

	// Authenticated but not staff.
	nonStaff := &auth.User{ID: 2, Username: "joe", IsStaff: false}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/admin/article/", nil).
		WithContext(auth.WithUser(context.Background(), nonStaff))
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Fatalf("non-staff changelist status = %d, want 302", rec2.Code)
	}
}

func TestChangelistUnknownModel404(t *testing.T) {
	db := newArticleDB(t)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/nope/", nil)
	staffInjector(router).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown model status = %d, want 404", rec.Code)
	}
}

// Author and Post exercise the FK-display path: a Post has an FK to an Author,
// and the changelist must render that FK as its related primary key.
type Author struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type Post struct {
	ID     int64
	Title  string `orm:"max_length=200"`
	Author orm.FK[Author]
}

func TestChangelistRendersFKAsPK(t *testing.T) {
	reg := orm.NewRegistry()
	if _, err := reg.Register(&Author{}); err != nil {
		t.Fatalf("Register(Author): %v", err)
	}
	if _, err := reg.Register(&Post{}); err != nil {
		t.Fatalf("Register(Post): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	ctx := context.Background()
	for _, name := range []string{"Author", "Post"} {
		m, _ := reg.Get(name)
		if err := db.CreateTable(ctx, m); err != nil {
			t.Fatalf("CreateTable(%s): %v", name, err)
		}
	}
	author := &Author{Name: "Ada"}
	if err := orm.Query[Author](db).Create(ctx, author); err != nil {
		t.Fatalf("Create(Author): %v", err)
	}
	post := &Post{Title: "On Computing"}
	post.Author.SetPK(author.ID)
	if err := orm.Query[Post](db).Create(ctx, post); err != nil {
		t.Fatalf("Create(Post): %v", err)
	}

	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	admin.Register[Post](site, admin.ModelAdmin{})
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/post/", nil)
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("post changelist status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// The Author column header and the FK's related PK (the author's id) appear.
	if !strings.Contains(body, "Author") {
		t.Errorf("post changelist missing Author column:\n%s", body)
	}
	if !strings.Contains(body, "On Computing") {
		t.Errorf("post changelist missing title:\n%s", body)
	}
}

func TestRegisterUnregisteredModelPanics(t *testing.T) {
	db := newArticleDB(t)
	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Register of an unregistered model did not panic")
		}
	}()
	type Unregistered struct{ ID int64 }
	admin.Register[Unregistered](site, admin.ModelAdmin{})
}

// --- modelOps unit tests via the exported test hooks ---

func TestOpsAllReturnsReadablePointers(t *testing.T) {
	db := newArticleDB(t)
	seeded := seedArticles(t, db)
	ops := admin.OpsFor[Article](db)

	objs, err := ops.All(context.Background(), []string{"id"})
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(objs) != len(seeded) {
		t.Fatalf("all returned %d rows, want %d", len(objs), len(seeded))
	}
	for i, obj := range objs {
		a, ok := obj.(*Article)
		if !ok {
			t.Fatalf("all[%d] is %T, want *Article", i, obj)
		}
		if a.Title != seeded[i].Title {
			t.Errorf("all[%d].Title = %q, want %q", i, a.Title, seeded[i].Title)
		}
	}
}

func TestOpsGet(t *testing.T) {
	db := newArticleDB(t)
	seeded := seedArticles(t, db)
	ops := admin.OpsFor[Article](db)

	obj, err := ops.Get(context.Background(), seeded[1].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	a := obj.(*Article)
	if a.Title != seeded[1].Title {
		t.Errorf("get.Title = %q, want %q", a.Title, seeded[1].Title)
	}

	if _, err := ops.Get(context.Background(), 99999); !errors.Is(err, orm.ErrDoesNotExist) {
		t.Fatalf("get(missing) err = %v, want ErrDoesNotExist", err)
	}
}

func TestOpsCreate(t *testing.T) {
	db := newArticleDB(t)
	ops := admin.OpsFor[Article](db)

	ptr := ops.NewPtr()
	a := ptr.(*Article)
	a.Title = "Created via ops"
	a.Views = 5
	if err := ops.Create(context.Background(), ptr); err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("create did not write back an auto PK")
	}

	got, err := orm.Query[Article](db).Get(context.Background(), "id", a.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Title != "Created via ops" {
		t.Errorf("reloaded Title = %q, want %q", got.Title, "Created via ops")
	}
}

func TestOpsUpdate(t *testing.T) {
	db := newArticleDB(t)
	seeded := seedArticles(t, db)
	ops := admin.OpsFor[Article](db)

	updated := &Article{ID: seeded[0].ID, Title: "Updated", Views: 999, Published: false}
	if err := ops.Update(context.Background(), seeded[0].ID, updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := orm.Query[Article](db).Get(context.Background(), "id", seeded[0].ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Title != "Updated" || got.Views != 999 {
		t.Errorf("after update = {Title:%q Views:%d}, want {Updated 999}", got.Title, got.Views)
	}
}

func TestOpsDelete(t *testing.T) {
	db := newArticleDB(t)
	seeded := seedArticles(t, db)
	ops := admin.OpsFor[Article](db)

	if err := ops.Del(context.Background(), seeded[0].ID); err != nil {
		t.Fatalf("del: %v", err)
	}
	if _, err := orm.Query[Article](db).Get(context.Background(), "id", seeded[0].ID); !errors.Is(err, orm.ErrDoesNotExist) {
		t.Fatalf("after delete, Get err = %v, want ErrDoesNotExist", err)
	}
}

// --- write views: add / change / delete ---

func TestAddGETRendersForm(t *testing.T) {
	db := newArticleDB(t)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/article/add/", nil)
	staffCSRF(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("add GET status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<form method="post"`) {
		t.Errorf("add GET body missing post form:\n%s", body)
	}
	for _, name := range []string{`name="Title"`, `name="Views"`, `name="Published"`} {
		if !strings.Contains(body, name) {
			t.Errorf("add GET body missing input %q:\n%s", name, body)
		}
	}
	// The auto PK must not be an editable input.
	if strings.Contains(body, `name="ID"`) {
		t.Errorf("add GET body should not expose the auto PK input:\n%s", body)
	}
	if !strings.Contains(body, `name="csrfmiddlewaretoken"`) {
		t.Errorf("add GET body missing csrf hidden input:\n%s", body)
	}
}

func TestAddPOSTValidCreatesAndRedirects(t *testing.T) {
	db := newArticleDB(t)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	form := url.Values{}
	form.Set("Title", "Posted Title")
	form.Set("Views", "42")
	form.Set("Published", "on")
	form.Set(csrf.FormField, "ignored-by-handler")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/article/add/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("add POST status = %d, want 302\n%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/article/" {
		t.Errorf("add POST redirect Location = %q, want %q", loc, "/admin/article/")
	}

	got, err := orm.Query[Article](db).Filter("title", "Posted Title").All(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after add POST, found %d rows with Title %q, want 1", len(got), "Posted Title")
	}
	if got[0].Views != 42 || !got[0].Published {
		t.Errorf("created row = {Views:%d Published:%v}, want {42 true}", got[0].Views, got[0].Published)
	}
}

func TestAddPOSTInvalidReRendersNoRow(t *testing.T) {
	db := newArticleDB(t)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	form := url.Values{}
	// Title exceeds max_length=200, so validation fails.
	form.Set("Title", strings.Repeat("x", 201))
	form.Set("Views", "1")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/article/add/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("invalid add POST status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "at most 200 characters") {
		t.Errorf("invalid add POST body missing error message:\n%s", body)
	}

	all, err := orm.Query[Article](db).All(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("invalid add POST created %d rows, want 0", len(all))
	}
}

func TestChangeGETPrefillsForm(t *testing.T) {
	db := newArticleDB(t)
	seeded := seedArticles(t, db)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/article/1/change/", nil)
	staffCSRF(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("change GET status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="`+seeded[0].Title+`"`) {
		t.Errorf("change GET body not pre-filled with Title %q:\n%s", seeded[0].Title, body)
	}
	if !strings.Contains(body, `name="csrfmiddlewaretoken"`) {
		t.Errorf("change GET body missing csrf hidden input:\n%s", body)
	}
}

func TestChangeGETMissing404(t *testing.T) {
	db := newArticleDB(t)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/article/99999/change/", nil)
	staffCSRF(router).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("change GET missing status = %d, want 404", rec.Code)
	}
}

func TestChangePOSTUpdatesAndRedirects(t *testing.T) {
	db := newArticleDB(t)
	seedArticles(t, db)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	form := url.Values{}
	form.Set("Title", "Renamed")
	form.Set("Views", "7")
	// Published omitted -> false.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/article/1/change/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("change POST status = %d, want 302\n%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/article/" {
		t.Errorf("change POST redirect Location = %q, want %q", loc, "/admin/article/")
	}

	got, err := orm.Query[Article](db).Get(context.Background(), "id", 1)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Title != "Renamed" || got.Views != 7 {
		t.Errorf("after change POST = {Title:%q Views:%d}, want {Renamed 7}", got.Title, got.Views)
	}
}

func TestDeleteGETConfirms(t *testing.T) {
	db := newArticleDB(t)
	seeded := seedArticles(t, db)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/article/1/delete/", nil)
	staffCSRF(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete GET status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Are you sure") {
		t.Errorf("delete GET body missing confirmation prompt:\n%s", body)
	}
	if !strings.Contains(body, seeded[0].Title) && !strings.Contains(body, "Article 1") {
		t.Errorf("delete GET body does not mention the object:\n%s", body)
	}
	if !strings.Contains(body, `name="csrfmiddlewaretoken"`) {
		t.Errorf("delete GET body missing csrf hidden input:\n%s", body)
	}
}

func TestDeletePOSTRemovesAndRedirects(t *testing.T) {
	db := newArticleDB(t)
	seedArticles(t, db)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/article/1/delete/", nil)
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("delete POST status = %d, want 302\n%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/article/" {
		t.Errorf("delete POST redirect Location = %q, want %q", loc, "/admin/article/")
	}
	if _, err := orm.Query[Article](db).Get(context.Background(), "id", 1); !errors.Is(err, orm.ErrDoesNotExist) {
		t.Fatalf("after delete POST, Get err = %v, want ErrDoesNotExist", err)
	}
}

func TestWriteViewsRedirectNonStaff(t *testing.T) {
	db := newArticleDB(t)
	seedArticles(t, db)
	site := newSite(t, db)
	router := urls.NewRouter(site.Routes()...)

	cases := []struct {
		method, path string
	}{
		{http.MethodGet, "/admin/article/add/"},
		{http.MethodPost, "/admin/article/add/"},
		{http.MethodGet, "/admin/article/1/change/"},
		{http.MethodPost, "/admin/article/1/change/"},
		{http.MethodGet, "/admin/article/1/delete/"},
		{http.MethodPost, "/admin/article/1/delete/"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(c.method, c.path, nil)
		router.ServeHTTP(rec, req) // anonymous: no user injected.
		if rec.Code != http.StatusFound {
			t.Errorf("%s %s anonymous status = %d, want 302", c.method, c.path, rec.Code)
			continue
		}
		if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, site.LoginURL) {
			t.Errorf("%s %s redirect Location = %q, want prefix %q", c.method, c.path, loc, site.LoginURL)
		}
	}
}

// SecretDoc is the model used to verify that excluded fields are not clobbered
// on a change POST. The Secret field is excluded from the admin form; EditTitle
// is the field the test edits.
type SecretDoc struct {
	ID        int64
	EditTitle string `orm:"max_length=200"`
	Secret    string `orm:"max_length=200"`
}

// newSecretDocDB creates an in-memory DB with SecretDoc registered and its
// table created.
func newSecretDocDB(t *testing.T) *orm.DB {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&SecretDoc{}); err != nil {
		t.Fatalf("Register(SecretDoc): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	m, ok := reg.Get("SecretDoc")
	if !ok {
		t.Fatal("SecretDoc model not found in registry")
	}
	if err := db.CreateTable(context.Background(), m); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	return db
}

// TestChangePOSTDoesNotClobberExcludedFields is a regression test for the
// data-loss bug where a change POST would zero out any field not present in
// the submitted form (excluded/readonly fields).
func TestChangePOSTDoesNotClobberExcludedFields(t *testing.T) {
	db := newSecretDocDB(t)
	ctx := context.Background()

	// Seed a row with a non-zero Secret value.
	original := &SecretDoc{EditTitle: "original title", Secret: "s3cr3t"}
	if err := orm.Query[SecretDoc](db).Create(ctx, original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	site, err := admin.NewAdminSite(db)
	if err != nil {
		t.Fatalf("NewAdminSite: %v", err)
	}
	admin.Register[SecretDoc](site, admin.ModelAdmin{
		ExcludeFields: []string{"Secret"},
	})
	router := urls.NewRouter(site.Routes()...)

	// POST a change that edits only EditTitle; Secret is absent from the form.
	form := url.Values{}
	form.Set("EditTitle", "updated title")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/admin/secretdoc/"+strconv.FormatInt(original.ID, 10)+"/change/",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	staffInjector(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("change POST status = %d, want 302\n%s", rec.Code, rec.Body.String())
	}

	// Reload from DB and assert both the edited and the excluded fields.
	got, err := orm.Query[SecretDoc](db).Get(ctx, "id", original.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.EditTitle != "updated title" {
		t.Errorf("EditTitle = %q, want %q", got.EditTitle, "updated title")
	}
	if got.Secret != "s3cr3t" {
		t.Errorf("Secret = %q after change POST, want %q (excluded field was clobbered)", got.Secret, "s3cr3t")
	}
}
