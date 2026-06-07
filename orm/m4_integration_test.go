package orm_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// m4Author and m4Article are the models for the consolidated M4 exit-criterion
// test. m4Article has a forward FK to m4Author.
type m4Author struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type m4Article struct {
	ID     int64
	Title  string `orm:"max_length=200"`
	Author orm.FK[m4Author]
}

// newM4Registry registers the M4 models, resolves relations, and freezes the
// registry.
func newM4Registry(t *testing.T) *orm.Registry {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&m4Author{}); err != nil {
		t.Fatalf("Register(m4Author): %v", err)
	}
	if _, err := reg.Register(&m4Article{}); err != nil {
		t.Fatalf("Register(m4Article): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()
	return reg
}

// newM4SQLiteDB builds a fresh in-memory SQLite DB pinned to a unique name,
// creates both tables, and returns the handle.
func newM4SQLiteDB(t *testing.T) *orm.DB {
	t.Helper()
	reg := newM4Registry(t)
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	createM4Tables(t, db)
	return db
}

// newM4PostgresDB opens the Postgres DSN (skipping when unset), drops and
// recreates both tables, and returns the handle.
func newM4PostgresDB(t *testing.T) *orm.DB {
	t.Helper()
	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping Postgres M4 integration")
	}
	reg := newM4Registry(t)
	pdb, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = pdb.Close() })

	ctx := context.Background()
	if err := pdb.PingContext(ctx); err != nil {
		t.Fatalf("postgres ping: %v", err)
	}
	drop := func() {
		_, _ = pdb.ExecContext(ctx, `DROP TABLE IF EXISTS "m4article"`)
		_, _ = pdb.ExecContext(ctx, `DROP TABLE IF EXISTS "m4author"`)
	}
	drop()
	t.Cleanup(drop)

	db := orm.NewDB(pdb, postgres.New(), reg)
	createM4Tables(t, db)
	return db
}

// createM4Tables creates the author and article tables (parent before child).
func createM4Tables(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()
	author, _ := db.Registry().Get("m4Author")
	article, _ := db.Registry().Get("m4Article")
	if err := db.CreateTable(ctx, author); err != nil {
		t.Fatalf("CreateTable(m4Author): %v", err)
	}
	if err := db.CreateTable(ctx, article); err != nil {
		t.Fatalf("CreateTable(m4Article): %v", err)
	}
}

// TestM4ExitCriterion is the consolidated M4 exit-criterion integration test. It
// exercises the whole M4 surface (transactions, signals, FK fetch,
// select_related, reverse FK, and prefetch_related) against every available
// dialect: always SQLite, and Postgres when DJANGOGO_TEST_POSTGRES_DSN is set.
func TestM4ExitCriterion(t *testing.T) {
	dialects := []struct {
		name  string
		newDB func(t *testing.T) *orm.DB
	}{
		{name: "sqlite", newDB: newM4SQLiteDB},
		{name: "postgres", newDB: newM4PostgresDB},
	}
	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			db := d.newDB(t)
			runM4ExitCriterion(t, db)
		})
	}
}

// runM4ExitCriterion drives the full M4 feature surface against db and asserts
// each of the five exit criteria.
func runM4ExitCriterion(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()

	// (1) Transactions + Create + signals: count PostSave firings for m4Article.
	var postSaves int64
	cancel := orm.OnPostSave(func(_ context.Context, _ *m4Article) error {
		atomic.AddInt64(&postSaves, 1)
		return nil
	})
	t.Cleanup(cancel)

	var author1, author2 m4Author
	var articles []m4Article
	err := db.Atomic(ctx, func(ctx context.Context) error {
		author1 = m4Author{Name: "Ada"}
		if err := orm.Query[m4Author](db).Create(ctx, &author1); err != nil {
			return err
		}
		author2 = m4Author{Name: "Grace"}
		if err := orm.Query[m4Author](db).Create(ctx, &author2); err != nil {
			return err
		}
		articles = []m4Article{
			{Title: "First"},
			{Title: "Second"},
			{Title: "Third"},
		}
		articles[0].Author.SetPK(author1.ID)
		articles[1].Author.SetPK(author1.ID)
		articles[2].Author.SetPK(author2.ID)
		for i := range articles {
			if err := orm.Query[m4Article](db).Create(ctx, &articles[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("(1) Atomic create: %v", err)
	}

	n, err := orm.Query[m4Article](db).Count(ctx)
	if err != nil {
		t.Fatalf("(1) Count: %v", err)
	}
	if n != 3 {
		t.Fatalf("(1) after commit, article Count = %d, want 3", n)
	}
	if got := atomic.LoadInt64(&postSaves); got != 3 {
		t.Fatalf("(1) PostSave fired %d times, want 3", got)
	}

	// (1b) A failing Atomic must not persist a stray row.
	boom := errors.New("boom")
	err = db.Atomic(ctx, func(ctx context.Context) error {
		stray := m4Article{Title: "Stray"}
		stray.Author.SetPK(author1.ID)
		if err := orm.Query[m4Article](db).Create(ctx, &stray); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("(1b) rollback Atomic: got %v, want boom", err)
	}
	if exists, err := orm.Query[m4Article](db).Filter("title", "Stray").Exists(ctx); err != nil {
		t.Fatalf("(1b) Exists(Stray): %v", err)
	} else if exists {
		t.Fatal("(1b) stray row persisted after rollback, want absent")
	}

	// (2) FK Fetch: read an article and resolve its Author by pk.
	art, err := orm.Query[m4Article](db).Get(ctx, "id", articles[0].ID)
	if err != nil {
		t.Fatalf("(2) Get(article): %v", err)
	}
	loaded, err := art.Author.Fetch(ctx, db)
	if err != nil {
		t.Fatalf("(2) Author.Fetch: %v", err)
	}
	if loaded.Name != "Ada" {
		t.Fatalf("(2) Fetch returned author %q, want Ada", loaded.Name)
	}

	// (3) select_related: each Author is loaded in the single JOIN query.
	srArts, err := orm.Query[m4Article](db).SelectRelated("Author").OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("(3) SelectRelated(Author).All: %v", err)
	}
	if len(srArts) != 3 {
		t.Fatalf("(3) SelectRelated: got %d articles, want 3", len(srArts))
	}
	wantNames := map[string]string{"First": "Ada", "Second": "Ada", "Third": "Grace"}
	for _, a := range srArts {
		obj, ok := a.Author.Object()
		if !ok {
			t.Fatalf("(3) article %q: Author not loaded by select_related", a.Title)
		}
		if obj.Name != wantNames[a.Title] {
			t.Fatalf("(3) article %q: loaded author %q, want %q", a.Title, obj.Name, wantNames[a.Title])
		}
	}

	// (4) reverse FK: author1 has exactly its two articles.
	a1Arts, err := orm.RelatedObjects[m4Article](db, "author_id", author1.ID).OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("(4) RelatedObjects(author1): %v", err)
	}
	if len(a1Arts) != 2 || a1Arts[0].Title != "First" || a1Arts[1].Title != "Second" {
		t.Fatalf("(4) RelatedObjects(author1) = %+v, want [First Second]", a1Arts)
	}

	// (5) prefetch_related: one query groups each author's articles correctly.
	authors, err := orm.Query[m4Author](db).OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("(5) load authors: %v", err)
	}
	pks, err := orm.PKsOf(db, authors)
	if err != nil {
		t.Fatalf("(5) PKsOf: %v", err)
	}
	byAuthor, err := orm.Prefetch[m4Article](ctx, db, "author_id", pks)
	if err != nil {
		t.Fatalf("(5) Prefetch: %v", err)
	}
	if len(byAuthor[author1.ID]) != 2 {
		t.Fatalf("(5) Prefetch author1 -> %d children, want 2", len(byAuthor[author1.ID]))
	}
	if len(byAuthor[author2.ID]) != 1 {
		t.Fatalf("(5) Prefetch author2 -> %d children, want 1", len(byAuthor[author2.ID]))
	}
	gotTitles := map[int64][]string{}
	for pk, kids := range byAuthor {
		for _, k := range kids {
			gotTitles[pk] = append(gotTitles[pk], k.Title)
		}
	}
	if !containsAll(gotTitles[author1.ID], "First", "Second") {
		t.Fatalf("(5) Prefetch author1 titles = %v, want First+Second", gotTitles[author1.ID])
	}
	if !containsAll(gotTitles[author2.ID], "Third") {
		t.Fatalf("(5) Prefetch author2 titles = %v, want Third", gotTitles[author2.ID])
	}

	// (5b) Prefetch with no parents runs no query and returns an empty map.
	empty, err := orm.Prefetch[m4Article](ctx, db, "author_id", nil)
	if err != nil {
		t.Fatalf("(5b) Prefetch(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("(5b) Prefetch(nil) = %v, want empty", empty)
	}
}

// containsAll reports whether got contains every element of want.
func containsAll(got []string, want ...string) bool {
	set := make(map[string]bool, len(got))
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return len(got) == len(want)
}

// TestM4PrefetchSingleQuery asserts that Prefetch issues exactly one SELECT with
// an IN clause (no N+1 per-parent fetch).
func TestM4PrefetchSingleQuery(t *testing.T) {
	db := newM4SQLiteDB(t)

	// Prefetch runs Query[Child](db).Filter(fkColumn+"__in", pks).All; compiling
	// that exact queryset shows it is a single SELECT with an IN predicate.
	sqlText, args, err := orm.CompileSelectForTest(
		orm.Query[m4Article](db).Filter("author_id__in", []int64{1, 2}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := strings.Count(sqlText, "SELECT"); got != 1 {
		t.Fatalf("expected a single SELECT (one query, not N+1), got %d:\n%s", got, sqlText)
	}
	if !strings.Contains(sqlText, " IN (") {
		t.Fatalf("expected an IN clause in prefetch SQL, got:\n%s", sqlText)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 IN args, got %d: %v", len(args), args)
	}
}
