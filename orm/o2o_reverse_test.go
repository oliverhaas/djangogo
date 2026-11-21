package orm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

type revUnitAuthor struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type revUnitBook struct {
	ID     int64
	Title  string                `orm:"max_length=50"`
	Author orm.FK[revUnitAuthor] `orm:"on_delete=cascade;related_name=authored_books;related_query_name=book_item"`
}

type revUnitBio struct {
	ID     int64
	Author orm.O2O[revUnitAuthor] `orm:"on_delete=cascade;related_name=bio"`
	Text   string                 `orm:"max_length=100"`
}

type revUnitPKBio struct {
	Author orm.O2O[revUnitAuthor] `orm:"pk;on_delete=cascade;related_name=pk_bio"`
	Text   string                 `orm:"max_length=100"`
}

type revUnitAmbiguous struct {
	ID     int64
	First  orm.FK[revUnitAuthor]
	Second orm.FK[revUnitAuthor]
}

// TestO2ODDLAndMetadata checks DDL on both dialects, Unique/PrimaryKey flags,
// and ReverseRelations metadata including Resolve idempotency.
func TestO2ODDLAndMetadata(t *testing.T) {
	t.Parallel()

	reg := orm.NewRegistry()
	authorM, err := reg.Register(&revUnitAuthor{})
	if err != nil {
		t.Fatalf("Register author: %v", err)
	}
	bookM, err := reg.Register(&revUnitBook{})
	if err != nil {
		t.Fatalf("Register book: %v", err)
	}
	bioM, err := reg.Register(&revUnitBio{})
	if err != nil {
		t.Fatalf("Register bio: %v", err)
	}
	pkBioM, err := reg.Register(&revUnitPKBio{})
	if err != nil {
		t.Fatalf("Register pkBio: %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve 1: %v", err)
	}
	// Resolve must be idempotent.
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve 2: %v", err)
	}

	// Verify field flags.
	bioAuthor, ok := bioM.FieldByName("Author")
	if !ok || bioAuthor.Rel == nil || bioAuthor.Rel.Kind != orm.RelO2O || !bioAuthor.Unique || bioAuthor.PrimaryKey {
		t.Fatalf("unexpected bio Author field: %+v", bioAuthor)
	}
	pkBioAuthor, ok := pkBioM.FieldByName("Author")
	if !ok || pkBioAuthor.Rel == nil || pkBioAuthor.Rel.Kind != orm.RelO2O || !pkBioAuthor.PrimaryKey || pkBioAuthor.Kind != orm.KindInt {
		t.Fatalf("unexpected pkBio Author field: %+v", pkBioAuthor)
	}

	// Verify DDL on SQLite and Postgres.
	sqlLiteBio := sqlite.New().CreateTableSQL(bioM)
	if !strings.Contains(sqlLiteBio, `"author_id" INTEGER NOT NULL UNIQUE`) ||
		!strings.Contains(sqlLiteBio, `FOREIGN KEY ("author_id") REFERENCES "revunitauthor" ("id") ON DELETE CASCADE`) {
		t.Fatalf("unexpected SQLite bio DDL: %s", sqlLiteBio)
	}
	sqlPGBio := postgres.New().CreateTableSQL(bioM)
	if !strings.Contains(sqlPGBio, `"author_id" BIGINT NOT NULL UNIQUE`) ||
		!strings.Contains(sqlPGBio, `FOREIGN KEY ("author_id") REFERENCES "revunitauthor" ("id") ON DELETE CASCADE`) {
		t.Fatalf("unexpected Postgres bio DDL: %s", sqlPGBio)
	}
	sqlLitePKBio := sqlite.New().CreateTableSQL(pkBioM)
	if !strings.Contains(sqlLitePKBio, `"author_id" INTEGER NOT NULL PRIMARY KEY`) ||
		strings.Contains(sqlLitePKBio, "AUTOINCREMENT") {
		t.Fatalf("unexpected SQLite pkBio DDL: %s", sqlLitePKBio)
	}
	sqlPGPKBio := postgres.New().CreateTableSQL(pkBioM)
	if !strings.Contains(sqlPGPKBio, `"author_id" BIGINT NOT NULL PRIMARY KEY`) ||
		strings.Contains(sqlPGPKBio, "BIGSERIAL") {
		t.Fatalf("unexpected Postgres pkBio DDL: %s", sqlPGPKBio)
	}

	// Verify ReverseRelations metadata.
	revs := authorM.ReverseRelations()
	if len(revs) != 3 {
		t.Fatalf("got %d reverse relations, want 3", len(revs))
	}
	bookRev, ok := authorM.ReverseRelation("authored_books")
	if !ok || bookRev.QueryName != "book_item" || bookRev.Source != bookM || bookRev.Kind != orm.RelFK {
		t.Fatalf("unexpected authored_books reverse relation: %+v", bookRev)
	}
	bioRev, ok := authorM.ReverseRelation("bio")
	if !ok || bioRev.QueryName != "bio" || bioRev.Source != bioM || bioRev.Kind != orm.RelO2O {
		t.Fatalf("unexpected bio reverse relation: %+v", bioRev)
	}
}

// TestRelationTagValidation checks empty related_name and related_query_name
// tags are rejected during registration.
func TestRelationTagValidation(t *testing.T) {
	t.Parallel()

	type badRelName struct {
		ID     int64
		Author orm.FK[revUnitAuthor] `orm:"related_name="`
	}
	reg := orm.NewRegistry()
	if _, err := reg.Register(&badRelName{}); err == nil {
		t.Fatal("expected error for empty related_name")
	}

	type badQueryName struct {
		ID     int64
		Author orm.O2O[revUnitAuthor] `orm:"related_query_name="`
	}
	reg = orm.NewRegistry()
	if _, err := reg.Register(&badQueryName{}); err == nil {
		t.Fatal("expected error for empty related_query_name")
	}
}

// TestO2OValueAndScan checks O2O Scan, Value, PK, SetPK, SetObject, and Object.
func TestO2OValueAndScan(t *testing.T) {
	t.Parallel()

	var o orm.O2O[revUnitAuthor]
	v, err := o.Value()
	if err != nil || v != nil {
		t.Fatalf("zero O2O Value = (%v, %v), want (nil, nil)", v, err)
	}
	if err := o.Scan(int64(7)); err != nil || o.PK() != 7 {
		t.Fatalf("Scan(int64(7)) = (%d, %v)", o.PK(), err)
	}
	if err := o.Scan(int(8)); err != nil || o.PK() != 8 {
		t.Fatalf("Scan(int(8)) = (%d, %v)", o.PK(), err)
	}
	if err := o.Scan(int32(9)); err != nil || o.PK() != 9 {
		t.Fatalf("Scan(int32(9)) = (%d, %v)", o.PK(), err)
	}
	if err := o.Scan([]byte("10")); err != nil || o.PK() != 10 {
		t.Fatalf("Scan([]byte(10)) = (%d, %v)", o.PK(), err)
	}
	if err := o.Scan("11"); err != nil || o.PK() != 11 {
		t.Fatalf("Scan(string(11)) = (%d, %v)", o.PK(), err)
	}
	if err := o.Scan(nil); err != nil || o.PK() != 0 {
		t.Fatalf("Scan(nil) = (%d, %v)", o.PK(), err)
	}
	if err := o.Scan(true); err == nil {
		t.Fatal("expected error scanning bool into O2O")
	}
	a := &revUnitAuthor{ID: 42, Name: "Ada"}
	o.SetObject(a, 42)
	gotObj, loaded := o.Object()
	if !loaded || gotObj != a {
		t.Fatalf("Object = (%v, %v), want (%v, true)", gotObj, loaded, a)
	}
	v, err = o.Value()
	if err != nil || v != int64(42) {
		t.Fatalf("Value = (%v, %v), want (42, nil)", v, err)
	}
	o.SetPK(5)
	if obj, loaded := o.Object(); loaded || obj != nil || o.PK() != 5 {
		t.Fatalf("after SetPK(5): pk=%d obj=%v loaded=%v", o.PK(), obj, loaded)
	}
}

// TestReverseQueryNameAndUncachedO2O exercises distinct related_name vs
// related_query_name, RelatedOne and SelectRelated on models without RelCache,
// PKsOf on an O2O-PK model, and reverse error cases.
func TestReverseQueryNameAndUncachedO2O(t *testing.T) {
	models := []any{djangotest.App("rev_unit",
		&revUnitAuthor{}, &revUnitBook{}, &revUnitBio{}, &revUnitPKBio{}, &revUnitAmbiguous{},
	)}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a1 := create(t, db, &revUnitAuthor{Name: "Ada"})
		a2 := create(t, db, &revUnitAuthor{Name: "Grace"})
		create(t, db, &revUnitBook{Title: "Notes", Author: fkTo[revUnitAuthor](a1.ID)})
		create(t, db, &revUnitBio{Author: o2oTo[revUnitAuthor](a1.ID), Text: "First programmer"})
		pkBio := create(t, db, &revUnitPKBio{Author: o2oTo[revUnitAuthor](a1.ID), Text: "PK bio"})

		// Query using related_query_name, access using related_name.
		djangotest.AssertQuerySetEqual(t,
			orm.Query[revUnitAuthor](db).Filter("book_item__title", "Notes"),
			[]string{"Ada"}, func(a revUnitAuthor) string { return a.Name })
		djangotest.AssertQuerySetEqual(t,
			orm.Related[revUnitBook](db, a1, "authored_books"),
			[]string{"Notes"}, func(b revUnitBook) string { return b.Title })

		// Wrong model instance passed to reverse filter fails.
		_, err := orm.Query[revUnitAuthor](db).Filter("book_item", a1).All(ctx)
		assertErrorContains(t, err, "must be")

		// RelatedOne works on a model without RelCache (passes value or pointer).
		bio, err := orm.RelatedOne[revUnitBio](ctx, db, *a1, "bio")
		if err != nil {
			t.Fatalf("RelatedOne bio: %v", err)
		}
		djangotest.AssertEqual(t, bio.Text, "First programmer")
		_, err = orm.RelatedOne[revUnitBio](ctx, db, a2, "bio")
		djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)

		// SelectRelated on reverse O2O works even without RelCache and reuses a filter join.
		sql := mvSQL(t, db, orm.Query[revUnitAuthor](db).Filter("bio__text", "First programmer").SelectRelated("bio"))
		djangotest.AssertEqual(t, strings.Count(sql, "INNER JOIN"), 1)
		djangotest.AssertEqual(t, strings.Count(sql, "LEFT OUTER JOIN"), 0)

		// PKsOf works on a model whose PK is an O2O[T].
		pks, err := orm.PKsOf(db, []revUnitPKBio{*pkBio})
		if err != nil {
			t.Fatalf("PKsOf pkBio: %v", err)
		}
		djangotest.AssertEqual(t, pks, []int64{a1.ID})

		// Error cases on Related and RelatedOne.
		var nilAuthor *revUnitAuthor
		_, err = orm.Related[revUnitBook](db, nil, "authored_books").All(ctx)
		assertErrorContains(t, err, "nil")
		_, err = orm.Related[revUnitBook](db, nilAuthor, "authored_books").All(ctx)
		assertErrorContains(t, err, "nil")
		_, err = orm.Related[revUnitBook](db, a1, "unknown_rel").All(ctx)
		assertFieldError(t, err, "cannot resolve reverse relation 'unknown_rel'")
		_, err = orm.Related[revUnitAmbiguous](db, a1, "revunitambiguous_set").All(ctx)
		assertErrorContains(t, err, "ambiguous")
		_, err = orm.Related[revUnitBio](db, a1, "authored_books").All(ctx)
		assertErrorContains(t, err, "targets revUnitBook")
		_, err = orm.RelatedOne[revUnitBook](ctx, db, a1, "authored_books")
		assertErrorContains(t, err, "has kind")

		// Unset O2O.Fetch returns ErrDoesNotExist with 0 queries.
		var emptyO2O orm.O2O[revUnitAuthor]
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := emptyO2O.Fetch(ctx, db)
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
	})
}

// TestReverseFKExcludeAndJoinReuseEdgeCases exercises split_exclude with
// isnull=True, isnull=False, exact=nil, empty __in, Q OR ~Q, and OrderBy/Values
// reusing a reverse FK join.
func TestReverseFKExcludeAndJoinReuseEdgeCases(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2oSetUp(t, db)
		// f.r has 1 article ("This is a test"), f.r2 has 0 articles.
		rid := func(r m2oReporter) int64 { return r.ID }

		// Exclude(article__isnull=True) keeps only reporters WITH articles.
		djangotest.AssertQuerySetEqual(t,
			orm.Query[m2oReporter](db).Exclude("m2oarticle__isnull", true),
			[]int64{f.r.ID}, rid)
		// Exclude(article=nil) is equivalent to Exclude(article__isnull=True).
		djangotest.AssertQuerySetEqual(t,
			orm.Query[m2oReporter](db).Exclude("m2oarticle", nil),
			[]int64{f.r.ID}, rid)
		// Exclude(article__isnull=False) keeps only reporters WITHOUT articles.
		djangotest.AssertQuerySetEqual(t,
			orm.Query[m2oReporter](db).Exclude("m2oarticle__isnull", false),
			[]int64{f.r2.ID}, rid)
		// Exclude with empty __in excludes nobody.
		djangotest.AssertQuerySetEqual(t,
			orm.Query[m2oReporter](db).Exclude("m2oarticle__headline__in", []string{}).OrderBy("id"),
			[]int64{f.r.ID, f.r2.ID}, rid)
		// Filter with Q OR ~Q over a reverse FK.
		djangotest.AssertQuerySetEqual(t,
			orm.Query[m2oReporter](db).Filter(
				orm.Q("first_name", "John").Or(orm.Not(orm.Q("m2oarticle__headline", "This is a test"))),
			).OrderBy("id"),
			[]int64{f.r.ID, f.r2.ID}, rid)

		// Filter + OrderBy on same reverse FK reuses the single join.
		sql := mvSQL(t, db, orm.Query[m2oReporter](db).Filter("m2oarticle__headline", "This is a test").OrderBy("m2oarticle__headline"))
		djangotest.AssertEqual(t, strings.Count(sql, "JOIN"), 1)

		// Filter + Values on same reverse FK reuses the single join.
		capture := djangotest.CaptureQueries(db)
		rows, err := orm.Query[m2oReporter](db).Filter("m2oarticle__headline", "This is a test").Values("first_name", "m2oarticle__headline").All(ctx)
		capture.Stop()
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		djangotest.AssertEqual(t, len(rows), 1)
		djangotest.AssertEqual(t, strings.Count(capture.Queries()[0].SQL, "JOIN"), 1)
	})
}
