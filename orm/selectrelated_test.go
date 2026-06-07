package orm_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// srAuthor and srArticle are the models for the select_related tests. srArticle
// has a forward FK to srAuthor.
type srAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type srArticle struct {
	ID     int64
	Title  string `orm:"max_length=200"`
	Author orm.FK[srAuthor]
}

// srNullArticle has a nullable FK to srAuthor so the LEFT JOIN null case can be
// exercised: an article whose Author is unset still appears in the result.
type srNullArticle struct {
	ID     int64
	Title  string           `orm:"max_length=200"`
	Author orm.FK[srAuthor] `orm:"null"`
}

// newSelectRelatedRegistry registers the select_related models, resolves
// relations, and returns the frozen registry.
func newSelectRelatedRegistry(t *testing.T) *orm.Registry {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&srAuthor{}); err != nil {
		t.Fatalf("Register(srAuthor): %v", err)
	}
	if _, err := reg.Register(&srArticle{}); err != nil {
		t.Fatalf("Register(srArticle): %v", err)
	}
	if _, err := reg.Register(&srNullArticle{}); err != nil {
		t.Fatalf("Register(srNullArticle): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()
	return reg
}

// seedSelectRelated creates tables and inserts 2 authors and 3 articles (the
// first two pointing at author one, the third at author two). It returns both
// author IDs.
func seedSelectRelated(t *testing.T, db *orm.DB) (a1, a2 srAuthor) {
	t.Helper()
	ctx := context.Background()

	author, _ := db.Registry().Get("srAuthor")
	article, _ := db.Registry().Get("srArticle")
	nullArticle, _ := db.Registry().Get("srNullArticle")

	if err := db.CreateTable(ctx, author); err != nil {
		t.Fatalf("CreateTable(srAuthor): %v", err)
	}
	if err := db.CreateTable(ctx, article); err != nil {
		t.Fatalf("CreateTable(srArticle): %v", err)
	}
	if err := db.CreateTable(ctx, nullArticle); err != nil {
		t.Fatalf("CreateTable(srNullArticle): %v", err)
	}

	a1 = srAuthor{Name: "Ada"}
	if err := orm.Query[srAuthor](db).Create(ctx, &a1); err != nil {
		t.Fatalf("Create(Ada): %v", err)
	}
	a2 = srAuthor{Name: "Grace"}
	if err := orm.Query[srAuthor](db).Create(ctx, &a2); err != nil {
		t.Fatalf("Create(Grace): %v", err)
	}

	arts := []srArticle{
		{Title: "First"},
		{Title: "Second"},
		{Title: "Third"},
	}
	arts[0].Author.SetPK(a1.ID)
	arts[1].Author.SetPK(a1.ID)
	arts[2].Author.SetPK(a2.ID)
	for i := range arts {
		if err := orm.Query[srArticle](db).Create(ctx, &arts[i]); err != nil {
			t.Fatalf("Create(%s): %v", arts[i].Title, err)
		}
	}
	return a1, a2
}

// runSelectRelated exercises select_related, the reverse-FK helper, and the
// null-FK LEFT JOIN case against the given DB handle.
func runSelectRelated(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()
	a1, a2 := seedSelectRelated(t, db)

	// select_related eager-loads the Author in a single JOIN query.
	arts, err := orm.Query[srArticle](db).SelectRelated("Author").OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("SelectRelated(Author).All: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("SelectRelated: got %d articles, want 3", len(arts))
	}
	wantNames := map[string]string{
		"First":  "Ada",
		"Second": "Ada",
		"Third":  "Grace",
	}
	for _, art := range arts {
		obj, loaded := art.Author.Object()
		if !loaded {
			t.Fatalf("article %q: Author not loaded by select_related", art.Title)
		}
		if obj.Name != wantNames[art.Title] {
			t.Fatalf("article %q: loaded author %q, want %q", art.Title, obj.Name, wantNames[art.Title])
		}
		wantPK := a1.ID
		if art.Title == "Third" {
			wantPK = a2.ID
		}
		if art.Author.PK() != wantPK {
			t.Fatalf("article %q: Author.PK = %d, want %d", art.Title, art.Author.PK(), wantPK)
		}
	}

	// Reverse FK: author one has exactly the first two articles.
	a1Arts, err := orm.RelatedObjects[srArticle](db, "author_id", a1.ID).OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("RelatedObjects(author_id=%d): %v", a1.ID, err)
	}
	if len(a1Arts) != 2 || a1Arts[0].Title != "First" || a1Arts[1].Title != "Second" {
		t.Fatalf("RelatedObjects(a1): got %+v, want [First Second]", a1Arts)
	}
	a2Arts, err := orm.RelatedObjects[srArticle](db, "author_id", a2.ID).All(ctx)
	if err != nil {
		t.Fatalf("RelatedObjects(author_id=%d): %v", a2.ID, err)
	}
	if len(a2Arts) != 1 || a2Arts[0].Title != "Third" {
		t.Fatalf("RelatedObjects(a2): got %+v, want [Third]", a2Arts)
	}

	// Null-FK LEFT JOIN: an article with an unset Author is still returned, and
	// its Author stays unloaded. An article with a set Author is loaded.
	withAuthor := srNullArticle{Title: "HasAuthor"}
	withAuthor.Author.SetPK(a1.ID)
	if err := orm.Query[srNullArticle](db).Create(ctx, &withAuthor); err != nil {
		t.Fatalf("Create(HasAuthor): %v", err)
	}
	orphan := srNullArticle{Title: "Orphan"} // Author left unset -> NULL FK column.
	if err := orm.Query[srNullArticle](db).Create(ctx, &orphan); err != nil {
		t.Fatalf("Create(Orphan): %v", err)
	}

	nulls, err := orm.Query[srNullArticle](db).SelectRelated("Author").OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("SelectRelated null case: %v", err)
	}
	if len(nulls) != 2 {
		t.Fatalf("null-FK select_related: got %d rows, want 2 (LEFT JOIN must keep the orphan)", len(nulls))
	}
	byTitle := map[string]srNullArticle{}
	for _, n := range nulls {
		byTitle[n.Title] = n
	}
	has, ok := byTitle["HasAuthor"]
	if !ok {
		t.Fatal("null-FK select_related: missing HasAuthor row")
	}
	if obj, loaded := has.Author.Object(); !loaded || obj.Name != "Ada" {
		t.Fatalf("HasAuthor: Author = (%v, loaded=%v), want Ada loaded", obj, loaded)
	}
	orph, ok := byTitle["Orphan"]
	if !ok {
		t.Fatal("null-FK select_related: missing Orphan row (LEFT JOIN dropped a null-FK row)")
	}
	if _, loaded := orph.Author.Object(); loaded {
		t.Fatal("Orphan: Author should be unloaded for a null FK")
	}
}

func TestSelectRelatedSQLite(t *testing.T) {
	reg := newSelectRelatedRegistry(t)
	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	runSelectRelated(t, db)
}

func TestSelectRelatedPostgres(t *testing.T) {
	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping Postgres select_related integration")
	}
	reg := newSelectRelatedRegistry(t)
	pdb, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = pdb.Close() })

	ctx := context.Background()
	drop := func() {
		_, _ = pdb.ExecContext(ctx, `DROP TABLE IF EXISTS "srarticle"`)
		_, _ = pdb.ExecContext(ctx, `DROP TABLE IF EXISTS "srnullarticle"`)
		_, _ = pdb.ExecContext(ctx, `DROP TABLE IF EXISTS "srauthor"`)
	}
	drop()
	t.Cleanup(drop)

	if err := pdb.PingContext(ctx); err != nil {
		t.Fatalf("postgres ping: %v", err)
	}
	db := orm.NewDB(pdb, postgres.New(), reg)
	runSelectRelated(t, db)
}

// TestSelectRelatedCompilesJoin asserts the compiled SQL is a single LEFT JOIN
// query (not an N+1 follow-up fetch).
func TestSelectRelatedCompilesJoin(t *testing.T) {
	reg := newSelectRelatedRegistry(t)
	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	sql, _, err := orm.CompileSelectForTest(orm.Query[srArticle](db).SelectRelated("Author").OrderBy("id"))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(sql, "LEFT JOIN") {
		t.Fatalf("expected LEFT JOIN in select_related SQL, got:\n%s", sql)
	}
	if strings.Count(sql, "SELECT") != 1 {
		t.Fatalf("expected a single SELECT (one JOIN query, not N+1), got:\n%s", sql)
	}
	// WHERE/ORDER references must be table-qualified so they are unambiguous.
	if !strings.Contains(sql, `"srarticle"."id"`) {
		t.Fatalf("expected ORDER BY column qualified by main table, got:\n%s", sql)
	}
}

// TestSelectRelatedUnknownField reports an error for a non-FK / unknown field.
func TestSelectRelatedUnknownField(t *testing.T) {
	reg := newSelectRelatedRegistry(t)
	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	ctx := context.Background()
	if _, err := orm.Query[srArticle](db).SelectRelated("Nope").All(ctx); err == nil {
		t.Fatal("SelectRelated(unknown field): expected error, got nil")
	}
	if _, err := orm.Query[srArticle](db).SelectRelated("Title").All(ctx); err == nil {
		t.Fatal("SelectRelated(non-FK field): expected error, got nil")
	}
}
