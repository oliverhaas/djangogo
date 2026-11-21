package orm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/many_to_one/models.py.

type m2oReporter struct {
	ID        int64
	FirstName string `orm:"max_length=30"`
	LastName  string `orm:"max_length=30"`
	Email     string `orm:"type=email"`
}

type m2oArticle struct {
	ID       int64
	Headline string              `orm:"max_length=100"`
	PubDate  orm.Date            `orm:"type=date"`
	Reporter orm.FK[m2oReporter] `orm:"on_delete=cascade"`
}

// Meta sets default headline ordering as in Django's many_to_one.Article.
func (m2oArticle) Meta() orm.Meta { return orm.Meta{Ordering: []string{"headline"}} }

type m2oCountry struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type m2oCity struct {
	ID      int64
	Country orm.FK[m2oCountry] `orm:"null;on_delete=cascade;related_name=cities"`
	Name    string             `orm:"max_length=50"`
}

type m2oDistrict struct {
	ID   int64
	City orm.FK[m2oCity] `orm:"null;on_delete=cascade;related_name=districts"`
	Name string          `orm:"max_length=50"`
}

type m2oFirst struct {
	ID     int64
	Second int64
}

type m2oSecond struct {
	ID    int64
	First orm.FK[m2oFirst] `orm:"on_delete=cascade;related_name=the_first"`
}

type m2oThird struct {
	ID    int64
	Name  string           `orm:"max_length=20"`
	Third orm.FK[m2oThird] `orm:"null;on_delete=set_null;related_name=child_set"`
}

type m2oParent struct {
	ID   int64
	Name string `orm:"max_length=20;unique"`
}

type m2oChild struct {
	ID     int64
	Name   string            `orm:"max_length=20"`
	Parent orm.FK[m2oParent] `orm:"on_delete=cascade"`
}

type m2oChildNullableParent struct {
	ID     int64
	Parent orm.FK[m2oParent] `orm:"null;on_delete=cascade"`
}

type m2oCategory struct {
	ID   int64
	Name string `orm:"max_length=20"`
}

type m2oRecord struct {
	ID       int64
	Category orm.FK[m2oCategory] `orm:"on_delete=cascade"`
}

type m2oRelation struct {
	ID    int64
	Left  orm.FK[m2oRecord] `orm:"on_delete=cascade;related_name=left_set"`
	Right orm.FK[m2oRecord] `orm:"on_delete=cascade;related_name=right_set"`
}

type m2oHidden struct {
	ID       int64
	Reporter orm.FK[m2oReporter] `orm:"on_delete=cascade;related_name=+"`
}

// m2oModels returns the model list for the many_to_one test app.
func m2oModels() []any {
	return []any{djangotest.App("many_to_one",
		&m2oReporter{}, &m2oArticle{}, &m2oCountry{}, &m2oCity{}, &m2oDistrict{},
		&m2oFirst{}, &m2oSecond{}, &m2oThird{}, &m2oParent{}, &m2oChild{},
		&m2oChildNullableParent{}, &m2oCategory{}, &m2oRecord{}, &m2oRelation{},
		&m2oHidden{},
	)}
}

// m2oFixture holds the shared setUpTestData rows for ManyToOneTests.
type m2oFixture struct {
	r, r2 *m2oReporter
	a     *m2oArticle
}

// m2oSetUp creates the base reporters and article from ManyToOneTests.setUpTestData.
func m2oSetUp(t *testing.T, db *orm.DB) m2oFixture {
	t.Helper()
	r := create(t, db, &m2oReporter{FirstName: "John", LastName: "Smith", Email: "john@example.com"})
	r2 := create(t, db, &m2oReporter{FirstName: "Paul", LastName: "Jones", Email: "paul@example.com"})
	a := create(t, db, &m2oArticle{
		Headline: "This is a test",
		PubDate:  orm.NewDate(2005, 7, 27),
		Reporter: fkTo[m2oReporter](r.ID),
	})
	return m2oFixture{r: r, r2: r2, a: a}
}

// m2oHeadline extracts the headline of an article for AssertQuerySetEqual.
func m2oHeadline(a m2oArticle) string { return a.Headline }

// m2oReporterID extracts the ID of a reporter for AssertQuerySetEqual.
func m2oReporterID(r m2oReporter) int64 { return r.ID }

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_get
func TestManyToOneGet(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		f := m2oSetUp(t, db)
		r, err := f.a.Reporter.Fetch(context.Background(), db)
		if err != nil {
			t.Fatalf("Fetch reporter: %v", err)
		}
		djangotest.AssertEqual(t, r.ID, f.r.ID)
		djangotest.AssertEqual(t, r.FirstName, "John")
		djangotest.AssertEqual(t, r.LastName, "Smith")
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_add
func TestManyToOneAdd(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2oSetUp(t, db)

		// Create an article via the reverse manager.
		newArticle := &m2oArticle{Headline: "John's second story", PubDate: orm.NewDate(2005, 7, 29)}
		if err := orm.RelatedSet[m2oArticle](db, f.r, "m2oarticle_set").Create(ctx, newArticle); err != nil {
			t.Fatalf("RelatedSet.Create: %v", err)
		}
		djangotest.AssertEqual(t, newArticle.Reporter.PK(), f.r.ID)

		// Adding an unsaved article via Add fails; AddSave saves it.
		newArticle2 := &m2oArticle{Headline: "Paul's story", PubDate: orm.NewDate(2006, 1, 17)}
		err := orm.RelatedSet[m2oArticle](db, f.r, "m2oarticle_set").Add(ctx, newArticle2)
		assertErrorContains(t, err, "isn't saved")

		if err := orm.RelatedSet[m2oArticle](db, f.r, "m2oarticle_set").AddSave(ctx, newArticle2); err != nil {
			t.Fatalf("AddSave: %v", err)
		}
		djangotest.AssertEqual(t, newArticle2.Reporter.PK(), f.r.ID)
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set"),
			[]string{"John's second story", "Paul's story", "This is a test"}, m2oHeadline)

		// Moving an article to another reporter updates its FK.
		if err := orm.RelatedSet[m2oArticle](db, f.r2, "m2oarticle_set").Add(ctx, newArticle2); err != nil {
			t.Fatalf("Add move: %v", err)
		}
		djangotest.AssertEqual(t, newArticle2.Reporter.PK(), f.r2.ID)
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r2, "m2oarticle_set"),
			[]string{"Paul's story"}, m2oHeadline)
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set"),
			[]string{"John's second story", "This is a test"}, m2oHeadline)
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_set
func TestManyToOneSet(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2oSetUp(t, db)

		newArticle := &m2oArticle{Headline: "John's second story", PubDate: orm.NewDate(2005, 7, 29)}
		if err := orm.RelatedSet[m2oArticle](db, f.r, "m2oarticle_set").Create(ctx, newArticle); err != nil {
			t.Fatalf("Create newArticle: %v", err)
		}
		newArticle2 := &m2oArticle{Headline: "Paul's story", PubDate: orm.NewDate(2006, 1, 17)}
		if err := orm.RelatedSet[m2oArticle](db, f.r2, "m2oarticle_set").Create(ctx, newArticle2); err != nil {
			t.Fatalf("Create newArticle2: %v", err)
		}

		// Reassign via direct save.
		newArticle2.Reporter.SetPK(f.r.ID)
		if err := orm.Query[m2oArticle](db).Save(ctx, newArticle2); err != nil {
			t.Fatalf("Save newArticle2: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set"),
			[]string{"John's second story", "Paul's story", "This is a test"}, m2oHeadline)
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r2, "m2oarticle_set"),
			[]string{}, m2oHeadline)

		// Set moves both articles to r2.
		if err := orm.RelatedSet[m2oArticle](db, f.r2, "m2oarticle_set").Set(ctx, []*m2oArticle{newArticle, newArticle2}); err != nil {
			t.Fatalf("Set r2: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set"),
			[]string{"This is a test"}, m2oHeadline)
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r2, "m2oarticle_set"),
			[]string{"John's second story", "Paul's story"}, m2oHeadline)

		// Because Reporter FK is not nullable, existing members of r remain.
		if err := orm.RelatedSet[m2oArticle](db, f.r, "m2oarticle_set").Set(ctx, []*m2oArticle{newArticle}); err != nil {
			t.Fatalf("Set r: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set"),
			[]string{"John's second story", "This is a test"}, m2oHeadline)
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r2, "m2oarticle_set"),
			[]string{"Paul's story"}, m2oHeadline)

		// Non-nullable FK rejects Remove and Clear.
		assertErrorContains(t, orm.RelatedSet[m2oArticle](db, f.r2, "m2oarticle_set").Remove(ctx, newArticle2), "nullable")
		assertErrorContains(t, orm.RelatedSet[m2oArticle](db, f.r2, "m2oarticle_set").Clear(ctx), "nullable")
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_selects
func TestManyToOneSelects(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2oSetUp(t, db)
		newArticle1 := &m2oArticle{Headline: "John's second story", PubDate: orm.NewDate(2005, 7, 29)}
		if err := orm.RelatedSet[m2oArticle](db, f.r, "m2oarticle_set").Create(ctx, newArticle1); err != nil {
			t.Fatalf("Create: %v", err)
		}
		newArticle2 := &m2oArticle{Headline: "Paul's story", PubDate: orm.NewDate(2006, 1, 17)}
		if err := orm.RelatedSet[m2oArticle](db, f.r2, "m2oarticle_set").Create(ctx, newArticle2); err != nil {
			t.Fatalf("Create: %v", err)
		}

		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set"),
			[]string{"John's second story", "This is a test"}, m2oHeadline)
		djangotest.AssertQuerySetEqual(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set").Filter("headline__startswith", "This"),
			[]string{"This is a test"}, m2oHeadline)
		djangotest.AssertEqual(t, mustCount(t, orm.Related[m2oArticle](db, f.r, "m2oarticle_set")), int64(2))
		djangotest.AssertEqual(t, mustCount(t, orm.Related[m2oArticle](db, f.r2, "m2oarticle_set")), int64(1))
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_joined_sql
func TestManyToOneJoinedSQL(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		m2oSetUp(t, db)
		qs := orm.Query[m2oArticle](db).Filter("reporter__first_name__exact", "John", "reporter__last_name__exact", "Smith")
		capture := djangotest.CaptureQueries(db)
		_ = mustAll(t, qs)
		capture.Stop()
		djangotest.AssertEqual(t, capture.Len(), 1)
		djangotest.AssertEqual(t, strings.Count(capture.Queries()[0].SQL, "INNER JOIN"), 1)
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_reverse_selects
func TestManyToOneReverseSelects(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		f := m2oSetUp(t, db)
		a3 := create(t, db, &m2oArticle{
			Headline: "Third article",
			PubDate:  orm.NewDate(2005, 7, 27),
			Reporter: fkTo[m2oReporter](f.r.ID),
		})
		create(t, db, &m2oArticle{
			Headline: "Fourth article",
			PubDate:  orm.NewDate(2005, 7, 27),
			Reporter: fkTo[m2oReporter](f.r.ID),
		})

		johnSmith := []int64{f.r.ID}
		reporters := func() *orm.QuerySet[m2oReporter] { return orm.Query[m2oReporter](db) }

		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__id__exact", f.a.ID), johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__pk", f.a.ID), johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle", f.a.ID), johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle", f.a), johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__in", []int64{f.a.ID, a3.ID}).Distinct(), johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__in", []any{f.a.ID, a3}).Distinct(), johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__in", []any{f.a, a3}).Distinct(), johnSmith, m2oReporterID)

		// Without Distinct(), multi-valued reverse FK returns one row per match.
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__headline__startswith", "T"),
			[]int64{f.r.ID, f.r.ID}, m2oReporterID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__headline__startswith", "T").Distinct(),
			johnSmith, m2oReporterID)

		// Count respects Distinct().
		djangotest.AssertEqual(t, mustCount(t, reporters().Filter("m2oarticle__headline__startswith", "T")), int64(2))
		djangotest.AssertEqual(t, mustCount(t, reporters().Filter("m2oarticle__headline__startswith", "T").Distinct()), int64(1))

		// Round-trip queries across reverse then forward FK.
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__reporter__first_name__startswith", "John"),
			[]int64{f.r.ID, f.r.ID, f.r.ID}, m2oReporterID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__reporter__first_name__startswith", "John").Distinct(),
			johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__reporter__exact", f.r).Distinct(),
			johnSmith, m2oReporterID)
		djangotest.AssertQuerySetEqual(t, reporters().Filter("m2oarticle__reporter", f.r).Distinct(),
			johnSmith, m2oReporterID)
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_delete
func TestManyToOneDelete(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2oSetUp(t, db)
		create(t, db, &m2oArticle{Headline: "John's second story", PubDate: orm.NewDate(2005, 7, 29), Reporter: fkTo[m2oReporter](f.r.ID)})
		create(t, db, &m2oArticle{Headline: "Paul's story", PubDate: orm.NewDate(2006, 1, 17), Reporter: fkTo[m2oReporter](f.r2.ID)})

		// Deleting r2 cascades to Paul's story.
		if _, err := orm.Query[m2oReporter](db).Filter("pk", f.r2.ID).Delete(ctx); err != nil {
			t.Fatalf("Delete r2: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[m2oArticle](db),
			[]string{"John's second story", "This is a test"}, m2oHeadline)

		// Delete using a reverse-relation JOIN in the filter.
		if _, err := orm.Query[m2oReporter](db).Filter("m2oarticle__headline__startswith", "This").Delete(ctx); err != nil {
			t.Fatalf("Delete via reverse join: %v", err)
		}
		djangotest.AssertEqual(t, mustCount(t, orm.Query[m2oReporter](db)), int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[m2oArticle](db)), int64(0))
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_values_list_exception
func TestManyToOneValuesListException(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[m2oArticle](db).ValuesList("reporter__notafield").All(context.Background())
		assertFieldError(t, err, "cannot resolve keyword 'notafield' into field. Choices are: email, first_name, id, last_name, m2oarticle")
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_save_parent_after_assign
func TestManyToOneSaveParentAfterAssign(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		category := &m2oCategory{Name: "cats"}
		record := &m2oRecord{}
		record.Category.SetObject(category, 0)

		// Saving child before parent fails with unsaved related object error.
		err := orm.Query[m2oRecord](db).Save(ctx, record)
		assertErrorContains(t, err, "unsaved related object 'category'")

		if err := orm.Query[m2oCategory](db).Save(ctx, category); err != nil {
			t.Fatalf("Save category: %v", err)
		}
		if err := orm.Query[m2oRecord](db).Save(ctx, record); err != nil {
			t.Fatalf("Save record: %v", err)
		}
		category.Name = "dogs"
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertEqual(t, record.Category.PK(), category.ID)
			got, ok := record.Category.Object()
			djangotest.AssertEqual(t, ok, true)
			djangotest.AssertEqual(t, got.Name, "dogs")
		})
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_save_nullable_fk_after_parent
func TestManyToOneSaveNullableFKAfterParent(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		parent := &m2oParent{Name: "p"}
		child := &m2oChildNullableParent{}
		child.Parent.SetObject(parent, 0)
		if err := orm.Query[m2oParent](db).Save(ctx, parent); err != nil {
			t.Fatalf("Save parent: %v", err)
		}
		if err := orm.Query[m2oChildNullableParent](db).Save(ctx, child); err != nil {
			t.Fatalf("Save child: %v", err)
		}
		loaded, err := orm.Query[m2oChildNullableParent](db).Get(ctx, "pk", child.ID)
		if err != nil {
			t.Fatalf("Get child: %v", err)
		}
		djangotest.AssertEqual(t, loaded.Parent.PK(), parent.ID)
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_multiple_foreignkeys
func TestManyToOneMultipleForeignKeys(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		c1 := create(t, db, &m2oCategory{Name: "First"})
		c2 := create(t, db, &m2oCategory{Name: "Second"})
		c3 := create(t, db, &m2oCategory{Name: "Third"})
		r1 := create(t, db, &m2oRecord{Category: fkTo[m2oCategory](c1.ID)})
		r2 := create(t, db, &m2oRecord{Category: fkTo[m2oCategory](c1.ID)})
		r3 := create(t, db, &m2oRecord{Category: fkTo[m2oCategory](c2.ID)})
		r4 := create(t, db, &m2oRecord{Category: fkTo[m2oCategory](c2.ID)})
		r5 := create(t, db, &m2oRecord{Category: fkTo[m2oCategory](c3.ID)})
		create(t, db, &m2oRelation{Left: fkTo[m2oRecord](r1.ID), Right: fkTo[m2oRecord](r2.ID)})
		create(t, db, &m2oRelation{Left: fkTo[m2oRecord](r3.ID), Right: fkTo[m2oRecord](r4.ID)})
		rel := create(t, db, &m2oRelation{Left: fkTo[m2oRecord](r1.ID), Right: fkTo[m2oRecord](r3.ID)})
		create(t, db, &m2oRelation{Left: fkTo[m2oRecord](r5.ID), Right: fkTo[m2oRecord](r2.ID)})
		create(t, db, &m2oRelation{Left: fkTo[m2oRecord](r3.ID), Right: fkTo[m2oRecord](r2.ID)})

		q1 := orm.Query[m2oRelation](db).Filter(
			"left__category__name__in", []string{"First"},
			"right__category__name__in", []string{"Second"},
		)
		djangotest.AssertQuerySetEqual(t, q1, []int64{rel.ID}, func(r m2oRelation) int64 { return r.ID })

		q2 := orm.Query[m2oCategory](db).Filter(
			"m2orecord__left_set__right__category__name", "Second",
		).OrderBy("name")
		djangotest.AssertQuerySetEqual(t, q2, []string{"First", "Second"}, func(c m2oCategory) string { return c.Name })
	})
}

// Ported from Django: tests/many_to_one/tests.py::ManyToOneTests.test_relation_unsaved
func TestManyToOneRelationUnsaved(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &m2oThird{Name: "Third 1"})
		create(t, db, &m2oThird{Name: "Third 2"})
		th := &m2oThird{Name: "testing"}

		_, err := orm.Related[m2oThird](db, th, "child_set").Count(ctx)
		assertErrorContains(t, err, "'m2oThird' instance needs to have a primary key value before this relationship can be used")

		if err := orm.Query[m2oThird](db).Save(ctx, th); err != nil {
			t.Fatalf("Save th: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			djangotest.AssertEqual(t, mustCount(t, orm.Related[m2oThird](db, th, "child_set")), int64(0))
		})
	})
}

// TestManyToOneRemoveAndClear exercises Remove, Clear, Set, and custom
// related_name on nullable foreign keys.
func TestManyToOneRemoveAndClear(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		us := create(t, db, &m2oCountry{Name: "United States"})
		chicago := create(t, db, &m2oCity{Name: "Chicago", Country: fkTo[m2oCountry](us.ID)})
		nyc := create(t, db, &m2oCity{Name: "New York", Country: fkTo[m2oCountry](us.ID)})
		la := create(t, db, &m2oCity{Name: "Los Angeles"})

		mgr := orm.RelatedSet[m2oCity](db, us, "cities")
		djangotest.AssertEqual(t, mustCount(t, mgr.QuerySet()), int64(2))

		// Remove sets Country FK to NULL.
		if err := mgr.Remove(ctx, chicago); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, mgr.QuerySet().OrderBy("name"),
			[]string{"New York"}, func(c m2oCity) string { return c.Name })
		reloaded, err := orm.Query[m2oCity](db).Get(ctx, "pk", chicago.ID)
		if err != nil {
			t.Fatalf("Get chicago: %v", err)
		}
		djangotest.AssertEqual(t, reloaded.Country.PK(), int64(0))

		// Set on nullable FK replaces the set (nulling nyc and attaching la).
		if err := mgr.Set(ctx, []*m2oCity{la}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, mgr.QuerySet(),
			[]string{"Los Angeles"}, func(c m2oCity) string { return c.Name })
		reloadedNYC, err := orm.Query[m2oCity](db).Get(ctx, "pk", nyc.ID)
		if err != nil {
			t.Fatalf("Get nyc: %v", err)
		}
		djangotest.AssertEqual(t, reloadedNYC.Country.PK(), int64(0))

		// Clear nulls all remaining cities.
		if err := mgr.Clear(ctx); err != nil {
			t.Fatalf("Clear: %v", err)
		}
		djangotest.AssertEqual(t, mustCount(t, mgr.QuerySet()), int64(0))

		// Filter via custom related_name.
		if err := mgr.Add(ctx, chicago); err != nil {
			t.Fatalf("Add chicago: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[m2oCountry](db).Filter("cities__name", "Chicago"),
			[]string{"United States"}, func(c m2oCountry) string { return c.Name })
	})
}

// TestManyToOneHiddenRelatedName verifies that related_name=+ suppresses the
// reverse accessor and excludes it from ReverseRelations and filter choices.
func TestManyToOneHiddenRelatedName(t *testing.T) {
	djangotest.EachDB(t, m2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2oSetUp(t, db)
		m, ok := db.Registry().ModelOf(m2oReporter{})
		if !ok {
			t.Fatal("m2oReporter not registered")
		}
		if _, found := m.ReverseRelation("m2ohidden_set"); found {
			t.Fatal("expected hidden reverse relation to be absent")
		}
		_, err := orm.Related[m2oHidden](db, f.r, "m2ohidden_set").All(ctx)
		if err == nil {
			t.Fatal("expected error when accessing hidden reverse relation")
		}
	})
}
