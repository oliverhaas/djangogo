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

// relAuthor and relArticle are the models used by the relation tests. relArticle
// has a forward FK to relAuthor.
type relAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type relArticle struct {
	ID     int64
	Title  string `orm:"max_length=200"`
	Author orm.FK[relAuthor]
}

// odCascadeArticle and odSetNullArticle exercise the on_delete tag.
type odCascadeArticle struct {
	ID     int64
	Author orm.FK[relAuthor] `orm:"on_delete=cascade"`
}

type odSetNullArticle struct {
	ID     int64
	Author orm.FK[relAuthor] `orm:"on_delete=set_null;null"`
}

// onDeleteDDL registers relAuthor plus model (which must carry a FK to it),
// resolves, and returns model's CreateTableSQL for the sqlite and postgres
// dialects.
func onDeleteDDL(t *testing.T, model any, name string) (sqliteDDL, postgresDDL string) {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&relAuthor{}); err != nil {
		t.Fatalf("Register(relAuthor): %v", err)
	}
	if _, err := reg.Register(model); err != nil {
		t.Fatalf("Register(%s): %v", name, err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()
	m, ok := reg.Get(name)
	if !ok {
		t.Fatalf("model %s not found in registry", name)
	}
	return sqlite.New().CreateTableSQL(m), postgres.New().CreateTableSQL(m)
}

func TestFKOnDeleteCascadeDDL(t *testing.T) {
	t.Parallel()
	s, p := onDeleteDDL(t, &odCascadeArticle{}, "odCascadeArticle")
	for _, ddl := range []string{s, p} {
		if !strings.Contains(ddl, `REFERENCES "relauthor" ("id") ON DELETE CASCADE`) {
			t.Fatalf("DDL missing ON DELETE CASCADE:\n%s", ddl)
		}
	}
}

func TestFKOnDeleteSetNullDDL(t *testing.T) {
	t.Parallel()
	s, p := onDeleteDDL(t, &odSetNullArticle{}, "odSetNullArticle")
	for _, ddl := range []string{s, p} {
		if !strings.Contains(ddl, `REFERENCES "relauthor" ("id") ON DELETE SET NULL`) {
			t.Fatalf("DDL missing ON DELETE SET NULL:\n%s", ddl)
		}
	}
}

func TestFKOnDeleteDefaultEmitsNoClause(t *testing.T) {
	t.Parallel()
	// A FK that does not set on_delete keeps the prior behavior (SQL NO ACTION),
	// so no ON DELETE clause is emitted.
	_, _, article := newRelRegistry(t)
	for _, ddl := range []string{sqlite.New().CreateTableSQL(article), postgres.New().CreateTableSQL(article)} {
		if strings.Contains(ddl, "ON DELETE") {
			t.Fatalf("default FK should emit no ON DELETE clause:\n%s", ddl)
		}
	}
}

func TestFKOnDeleteInvalidRejected(t *testing.T) {
	t.Parallel()
	type badOnDelete struct {
		ID     int64
		Author orm.FK[relAuthor] `orm:"on_delete=bogus"`
	}
	reg := orm.NewRegistry()
	if _, err := reg.Register(&badOnDelete{}); err == nil {
		t.Fatal("Register(badOnDelete): expected error for invalid on_delete, got nil")
	}
}

func TestFKOnDeleteSetNullRequiresNull(t *testing.T) {
	t.Parallel()
	type setNullNotNull struct {
		ID     int64
		Author orm.FK[relAuthor] `orm:"on_delete=set_null"`
	}
	reg := orm.NewRegistry()
	if _, err := reg.Register(&setNullNotNull{}); err == nil {
		t.Fatal("Register(setNullNotNull): expected error (set_null requires null), got nil")
	}
}

// newRelRegistry registers relAuthor and relArticle, resolves relations, and
// returns the frozen registry along with both models.
func newRelRegistry(t *testing.T) (*orm.Registry, *orm.Model, *orm.Model) {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&relAuthor{}); err != nil {
		t.Fatalf("Register(relAuthor): %v", err)
	}
	if _, err := reg.Register(&relArticle{}); err != nil {
		t.Fatalf("Register(relArticle): %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()
	author, _ := reg.Get("relAuthor")
	article, _ := reg.Get("relArticle")
	return reg, author, article
}

func TestFKScanValueRoundTrip(t *testing.T) {
	t.Parallel()

	// Scan int64.
	var f orm.FK[relAuthor]
	if err := f.Scan(int64(5)); err != nil {
		t.Fatalf("Scan(5): %v", err)
	}
	if f.PK() != 5 {
		t.Fatalf("after Scan(5), PK = %d, want 5", f.PK())
	}
	v, err := f.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if v != int64(5) {
		t.Fatalf("Value = %v, want int64(5)", v)
	}

	// Scan nil -> pk 0 -> Value nil.
	if err := f.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if f.PK() != 0 {
		t.Fatalf("after Scan(nil), PK = %d, want 0", f.PK())
	}
	v, err = f.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if v != nil {
		t.Fatalf("Value of zero FK = %v, want nil", v)
	}

	// Zero FK Value is nil.
	var zero orm.FK[relAuthor]
	v, err = zero.Value()
	if err != nil {
		t.Fatalf("Value(zero): %v", err)
	}
	if v != nil {
		t.Fatalf("Value(zero) = %v, want nil", v)
	}
}

func TestFKSetPKSetObject(t *testing.T) {
	t.Parallel()

	var f orm.FK[relAuthor]
	f.SetPK(7)
	if f.PK() != 7 {
		t.Fatalf("after SetPK(7), PK = %d, want 7", f.PK())
	}
	if obj, loaded := f.Object(); loaded || obj != nil {
		t.Fatalf("after SetPK, Object = (%v, %v), want (nil, false)", obj, loaded)
	}

	a := &relAuthor{ID: 9, Name: "Ann"}
	f.SetObject(a, 9)
	if f.PK() != 9 {
		t.Fatalf("after SetObject, PK = %d, want 9", f.PK())
	}
	obj, loaded := f.Object()
	if !loaded || obj != a {
		t.Fatalf("after SetObject, Object = (%v, %v), want (%v, true)", obj, loaded, a)
	}

	// SetPK clears the loaded object.
	f.SetPK(3)
	if obj, loaded := f.Object(); loaded || obj != nil {
		t.Fatalf("SetPK should clear object, got (%v, %v)", obj, loaded)
	}
}

func TestFKReflectionAndResolve(t *testing.T) {
	t.Parallel()

	_, author, article := newRelRegistry(t)

	f, ok := article.FieldByName("Author")
	if !ok {
		t.Fatal("relArticle has no Author field")
	}
	if f.Column != "author_id" {
		t.Fatalf("Author column = %q, want author_id", f.Column)
	}
	if f.Kind != orm.KindInt {
		t.Fatalf("Author kind = %v, want KindInt", f.Kind)
	}
	if f.Rel == nil {
		t.Fatal("Author field has nil Rel")
	}
	if f.Rel.Kind != orm.RelFK {
		t.Fatalf("Author Rel.Kind = %v, want RelFK", f.Rel.Kind)
	}
	if f.Rel.Column != "author_id" {
		t.Fatalf("Author Rel.Column = %q, want author_id", f.Rel.Column)
	}
	if f.Rel.Target != author {
		t.Fatalf("Author Rel.Target = %v, want relAuthor model", f.Rel.Target)
	}

	// Relations() returns exactly the Author field.
	rels := article.Relations()
	if len(rels) != 1 || rels[0] != f {
		t.Fatalf("Relations() = %v, want [Author]", rels)
	}

	// A scalar field keeps Rel == nil.
	title, _ := article.FieldByName("Title")
	if title.Rel != nil {
		t.Fatalf("scalar field Title has non-nil Rel: %+v", title.Rel)
	}
}

func TestResolveUnregisteredTarget(t *testing.T) {
	t.Parallel()

	reg := orm.NewRegistry()
	if _, err := reg.Register(&relArticle{}); err != nil {
		t.Fatalf("Register(relArticle): %v", err)
	}
	// relAuthor is intentionally not registered.
	err := reg.Resolve()
	if err == nil {
		t.Fatal("Resolve: expected error for unregistered target, got nil")
	}
	if !strings.Contains(err.Error(), "unregistered model") {
		t.Fatalf("Resolve error = %q, want it to mention unregistered model", err)
	}
}

func TestFKRejectsScalarTags(t *testing.T) {
	t.Parallel()

	type badMaxLen struct {
		ID     int64
		Author orm.FK[relAuthor] `orm:"max_length=10"`
	}
	reg := orm.NewRegistry()
	if _, err := reg.Register(&badMaxLen{}); err == nil {
		t.Fatal("Register(badMaxLen): expected error for max_length on FK, got nil")
	}

	type badPK struct {
		ID     int64
		Author orm.FK[relAuthor] `orm:"pk"`
	}
	reg = orm.NewRegistry()
	if _, err := reg.Register(&badPK{}); err == nil {
		t.Fatal("Register(badPK): expected error for pk on FK, got nil")
	}
}

func TestFKDDLSQLite(t *testing.T) {
	t.Parallel()

	_, _, article := newRelRegistry(t)
	ddl := sqlite.New().CreateTableSQL(article)
	if !strings.Contains(ddl, `FOREIGN KEY ("author_id") REFERENCES "relauthor" ("id")`) {
		t.Fatalf("sqlite DDL missing FK clause:\n%s", ddl)
	}
	if !strings.Contains(ddl, `"author_id" INTEGER NOT NULL`) {
		t.Fatalf("sqlite DDL missing author_id integer column:\n%s", ddl)
	}
}

func TestFKDDLPostgres(t *testing.T) {
	t.Parallel()

	_, _, article := newRelRegistry(t)
	ddl := postgres.New().CreateTableSQL(article)
	if !strings.Contains(ddl, `FOREIGN KEY ("author_id") REFERENCES "relauthor" ("id")`) {
		t.Fatalf("postgres DDL missing FK clause:\n%s", ddl)
	}
	if !strings.Contains(ddl, `"author_id" BIGINT NOT NULL`) {
		t.Fatalf("postgres DDL missing author_id BIGINT column:\n%s", ddl)
	}
}

// runFKIntegration exercises the full FK round-trip against the given dialect
// and database handle: create tables, insert an author and an article, read the
// article back (FK pk preserved), then Fetch the related author.
func runFKIntegration(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()

	author, _ := db.Registry().Get("relAuthor")
	article, _ := db.Registry().Get("relArticle")

	if err := db.CreateTable(ctx, author); err != nil {
		t.Fatalf("CreateTable(relAuthor): %v", err)
	}
	if err := db.CreateTable(ctx, article); err != nil {
		t.Fatalf("CreateTable(relArticle): %v", err)
	}

	a := relAuthor{Name: "Ada"}
	if err := orm.Query[relAuthor](db).Create(ctx, &a); err != nil {
		t.Fatalf("Create(relAuthor): %v", err)
	}
	if a.ID == 0 {
		t.Fatal("Create(relAuthor): expected non-zero auto PK")
	}

	art := relArticle{Title: "Hello"}
	art.Author.SetPK(a.ID)
	if err := orm.Query[relArticle](db).Create(ctx, &art); err != nil {
		t.Fatalf("Create(relArticle): %v", err)
	}
	if art.ID == 0 {
		t.Fatal("Create(relArticle): expected non-zero auto PK")
	}

	got, err := orm.Query[relArticle](db).Get(ctx, "id", art.ID)
	if err != nil {
		t.Fatalf("Get(relArticle): %v", err)
	}
	if got.Author.PK() != a.ID {
		t.Fatalf("read-back Author.PK = %d, want %d", got.Author.PK(), a.ID)
	}

	loaded, err := got.Author.Fetch(ctx, db)
	if err != nil {
		t.Fatalf("Author.Fetch: %v", err)
	}
	if loaded.Name != "Ada" {
		t.Fatalf("Fetch returned author %q, want Ada", loaded.Name)
	}
}

func TestFKIntegrationSQLite(t *testing.T) {
	reg, _, _ := newRelRegistry(t)
	sdb, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	runFKIntegration(t, db)
}

func TestFKIntegrationPostgres(t *testing.T) {
	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping Postgres FK integration")
	}
	reg, _, _ := newRelRegistry(t)
	pdb, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = pdb.Close() })

	ctx := context.Background()
	// Drop tables first (child before parent) for idempotency, and on cleanup.
	drop := func() {
		_, _ = pdb.ExecContext(ctx, `DROP TABLE IF EXISTS "relarticle"`)
		_, _ = pdb.ExecContext(ctx, `DROP TABLE IF EXISTS "relauthor"`)
	}
	drop()
	t.Cleanup(drop)

	if err := pdb.PingContext(ctx); err != nil {
		t.Fatalf("postgres ping: %v", err)
	}

	db := orm.NewDB(pdb, postgres.New(), reg)
	runFKIntegration(t, db)
}
