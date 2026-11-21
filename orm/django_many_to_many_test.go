package orm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/many_to_many/models.py.

type m2mPublication struct {
	ID    int64
	Title string `orm:"max_length=30"`
}

// Meta sets default title ordering as in Django's many_to_many.Publication.
func (m2mPublication) Meta() orm.Meta { return orm.Meta{Ordering: []string{"title"}} }

type m2mTag struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

type m2mArticle struct {
	ID           int64
	Headline     string `orm:"max_length=100"`
	Publications orm.M2M[m2mPublication]
	Tags         orm.M2M[m2mTag] `orm:"related_name=tags"`
}

// Meta sets default headline ordering as in Django's many_to_many.Article.
func (m2mArticle) Meta() orm.Meta { return orm.Meta{Ordering: []string{"headline"}} }

type m2mHiddenArticleA struct {
	ID           int64
	Publications orm.M2M[m2mPublication] `orm:"related_name=+"`
}

type m2mHiddenArticleB struct {
	ID           int64
	Publications orm.M2M[m2mPublication] `orm:"related_name=+"`
}

type m2mNullableTargetArticle struct {
	ID           int64
	Headline     string                  `orm:"max_length=100"`
	Publications orm.M2M[m2mPublication] `orm:"through=m2mNullablePublicationThrough"`
}

type m2mNullablePublicationThrough struct {
	ID          int64
	Article     orm.FK[m2mNullableTargetArticle] `orm:"on_delete=cascade"`
	Publication orm.FK[m2mPublication]           `orm:"null;on_delete=cascade"`
}

// m2mModels returns the model list for the many_to_many test app.
func m2mModels() []any {
	return []any{djangotest.App("many_to_many",
		&m2mPublication{}, &m2mTag{}, &m2mArticle{},
		&m2mHiddenArticleA{}, &m2mHiddenArticleB{},
		&m2mNullableTargetArticle{}, &m2mNullablePublicationThrough{},
	)}
}

// m2mFixture holds the shared setUpTestData rows for ManyToManyTests.
type m2mFixture struct {
	p1, p2, p3, p4 *m2mPublication
	a1, a2, a3, a4 *m2mArticle
}

// m2mSetUp populates the base publications and articles from ManyToManyTests.setUpTestData.
func m2mSetUp(t *testing.T, db *orm.DB) m2mFixture {
	t.Helper()
	ctx := context.Background()
	p1 := create(t, db, &m2mPublication{Title: "The Python Journal"})
	p2 := create(t, db, &m2mPublication{Title: "Science News"})
	p3 := create(t, db, &m2mPublication{Title: "Science Weekly"})
	p4 := create(t, db, &m2mPublication{Title: "Highlights for Children"})

	a1 := create(t, db, &m2mArticle{Headline: "Django lets you build web apps easily"})
	if err := orm.Many[m2mPublication](db, a1, "publications").Add(ctx, p1); err != nil {
		t.Fatalf("add a1.publications: %v", err)
	}

	a2 := create(t, db, &m2mArticle{Headline: "NASA uses Python"})
	if err := orm.Many[m2mPublication](db, a2, "publications").Add(ctx, p1, p2, p3, p4); err != nil {
		t.Fatalf("add a2.publications: %v", err)
	}

	a3 := create(t, db, &m2mArticle{Headline: "NASA finds intelligent life on Earth"})
	if err := orm.Many[m2mPublication](db, a3, "publications").Add(ctx, p2); err != nil {
		t.Fatalf("add a3.publications: %v", err)
	}

	a4 := create(t, db, &m2mArticle{Headline: "Oxygen-free diet works wonders"})
	if err := orm.Many[m2mPublication](db, a4, "publications").Add(ctx, p2); err != nil {
		t.Fatalf("add a4.publications: %v", err)
	}

	return m2mFixture{p1: p1, p2: p2, p3: p3, p4: p4, a1: a1, a2: a2, a3: a3, a4: a4}
}

// pubTitles extracts titles from a publication slice for assertions.
func pubTitles(pubs []m2mPublication) []string {
	out := make([]string, len(pubs))
	for i, p := range pubs {
		out[i] = p.Title
	}
	return out
}

// articleHeadlines extracts headlines from an article slice for assertions.
func articleHeadlines(articles []m2mArticle) []string {
	out := make([]string, len(articles))
	for i, a := range articles {
		out[i] = a.Headline
	}
	return out
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_add
func TestManyToManyAdd(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		// Unsaved instance cannot use M2M relationship.
		a5 := &m2mArticle{Headline: "Django lets you create web apps easily"}
		err := orm.Many[m2mPublication](db, a5, "publications").Add(ctx, f.p1)
		if err == nil || !strings.Contains(err.Error(), "needs to have a primary key value") {
			t.Fatalf("unsaved Add error = %v, want primary key error", err)
		}

		// Save and associate.
		if err := orm.Query[m2mArticle](db).Save(ctx, a5); err != nil {
			t.Fatalf("save a5: %v", err)
		}
		if err := orm.Many[m2mPublication](db, a5, "publications").Add(ctx, f.p1); err != nil {
			t.Fatalf("add p1: %v", err)
		}
		pubs, err := orm.Many[m2mPublication](db, a5, "publications").All(ctx)
		if err != nil {
			t.Fatalf("all a5.publications: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p1.Title})

		// Adding multiple and duplicates is idempotent.
		a6 := create(t, db, &m2mArticle{Headline: "ESA uses Python"})
		if err := orm.Many[m2mPublication](db, a6, "publications").Add(ctx, f.p1, f.p2); err != nil {
			t.Fatalf("add p1, p2: %v", err)
		}
		if err := orm.Many[m2mPublication](db, a6, "publications").Add(ctx, f.p3); err != nil {
			t.Fatalf("add p3: %v", err)
		}
		if err := orm.Many[m2mPublication](db, a6, "publications").Add(ctx, f.p3); err != nil {
			t.Fatalf("re-add p3: %v", err)
		}
		pubs, err = orm.Many[m2mPublication](db, a6, "publications").All(ctx)
		if err != nil {
			t.Fatalf("all a6.publications: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p2.Title, f.p3.Title, f.p1.Title})

		// Wrong target type is rejected at runtime when constructing the manager.
		if _, err := orm.Many[m2mArticle](db, a6, "publications").All(ctx); err == nil {
			t.Fatal("expected error for mismatched target type")
		}

		// Create directly through the M2M manager.
		p5 := &m2mPublication{Title: "Highlights for Adults"}
		if err := orm.Many[m2mPublication](db, a6, "publications").Create(ctx, p5); err != nil {
			t.Fatalf("create p5: %v", err)
		}
		pubs, err = orm.Many[m2mPublication](db, a6, "publications").All(ctx)
		if err != nil {
			t.Fatalf("all after create: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{p5.Title, f.p2.Title, f.p3.Title, f.p1.Title})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_add_remove_set_by_pk
func TestManyToManyAddRemoveSetByPK(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		a5 := create(t, db, &m2mArticle{Headline: "Django lets you create web apps easily"})
		mgr := orm.Many[m2mPublication](db, a5, "publications")
		if err := mgr.AddPKs(ctx, f.p1.ID); err != nil {
			t.Fatalf("AddPKs: %v", err)
		}
		pubs, err := mgr.All(ctx)
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p1.Title})

		if err := mgr.SetPKs(ctx, []any{f.p2.ID}); err != nil {
			t.Fatalf("SetPKs: %v", err)
		}
		pubs, err = mgr.All(ctx)
		if err != nil {
			t.Fatalf("All after SetPKs: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p2.Title})

		if err := mgr.RemovePKs(ctx, f.p2.ID); err != nil {
			t.Fatalf("RemovePKs: %v", err)
		}
		pubs, err = mgr.All(ctx)
		if err != nil {
			t.Fatalf("All after RemovePKs: %v", err)
		}
		djangotest.AssertEqual(t, len(pubs), 0)
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_add_remove_invalid_type
func TestManyToManyAddRemoveInvalidType(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)
		mgr := orm.Many[m2mPublication](db, f.a1, "publications")

		if err := mgr.AddPKs(ctx, "invalid"); err == nil || !strings.Contains(err.Error(), "Field 'id' expected a number but got 'invalid'") {
			t.Errorf("AddPKs(invalid) = %v, want number conversion error", err)
		}
		if err := mgr.RemovePKs(ctx, "invalid"); err == nil || !strings.Contains(err.Error(), "Field 'id' expected a number but got 'invalid'") {
			t.Errorf("RemovePKs(invalid) = %v, want number conversion error", err)
		}
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_reverse_add
func TestManyToManyReverseAdd(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		a5 := create(t, db, &m2mArticle{Headline: "NASA finds intelligent life on Mars"})
		if err := orm.Many[m2mArticle](db, f.p2, "m2marticle_set").Add(ctx, a5); err != nil {
			t.Fatalf("reverse Add: %v", err)
		}
		arts, err := orm.Many[m2mArticle](db, f.p2, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("reverse All: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{
			f.a3.Headline, a5.Headline, f.a2.Headline, f.a4.Headline,
		})
		pubs, err := orm.Many[m2mPublication](db, a5, "publications").All(ctx)
		if err != nil {
			t.Fatalf("a5.publications: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p2.Title})

		// Reverse Create.
		a6 := &m2mArticle{Headline: "Carbon-free diet works wonders"}
		if err := orm.Many[m2mArticle](db, f.p2, "m2marticle_set").Create(ctx, a6); err != nil {
			t.Fatalf("reverse Create: %v", err)
		}
		arts, err = orm.Many[m2mArticle](db, f.p2, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("reverse All after Create: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{
			a6.Headline, f.a3.Headline, a5.Headline, f.a2.Headline, f.a4.Headline,
		})
		pubs, err = orm.Many[m2mPublication](db, &arts[3], "publications").All(ctx)
		if err != nil {
			t.Fatalf("arts[3].publications: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p4.Title, f.p2.Title, f.p3.Title, f.p1.Title})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_fast_add_ignore_conflicts
func TestManyToManyFastAddIgnoreConflicts(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 1, func() {
			if err := orm.Many[m2mPublication](db, f.a1, "publications").Add(ctx, f.p1, f.p2); err != nil {
				t.Fatalf("fast Add: %v", err)
			}
		})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_related_sets
func TestManyToManyRelatedSets(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		pubs, err := orm.Many[m2mPublication](db, f.a1, "publications").All(ctx)
		if err != nil {
			t.Fatalf("a1.publications: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p1.Title})

		pubs, err = orm.Many[m2mPublication](db, f.a2, "publications").All(ctx)
		if err != nil {
			t.Fatalf("a2.publications: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p4.Title, f.p2.Title, f.p3.Title, f.p1.Title})

		arts, err := orm.Many[m2mArticle](db, f.p2, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("p2.m2marticle_set: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a3.Headline, f.a2.Headline, f.a4.Headline})

		arts, err = orm.Many[m2mArticle](db, f.p1, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("p1.m2marticle_set: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a1.Headline, f.a2.Headline})

		p4Reloaded, err := orm.Query[m2mPublication](db).Get(ctx, "id", f.p4.ID)
		if err != nil {
			t.Fatalf("get p4: %v", err)
		}
		arts, err = orm.Many[m2mArticle](db, &p4Reloaded, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("p4.m2marticle_set: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a2.Headline})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_selects
func TestManyToManySelects(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		for _, tc := range []struct {
			key string
			val any
		}{
			{"publications__id__exact", f.p1.ID},
			{"publications__pk", f.p1.ID},
			{"publications", f.p1.ID},
			{"publications", f.p1},
		} {
			arts, err := orm.Query[m2mArticle](db).Filter(tc.key, tc.val).All(ctx)
			if err != nil {
				t.Fatalf("filter %s: %v", tc.key, err)
			}
			djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a1.Headline, f.a2.Headline})
		}

		// Multi-valued filter without and with Distinct.
		arts, err := orm.Query[m2mArticle](db).Filter("publications__title__startswith", "Science").All(ctx)
		if err != nil {
			t.Fatalf("filter startswith Science: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{
			f.a3.Headline, f.a2.Headline, f.a2.Headline, f.a4.Headline,
		})

		arts, err = orm.Query[m2mArticle](db).Filter("publications__title__startswith", "Science").Distinct().All(ctx)
		if err != nil {
			t.Fatalf("filter startswith Science distinct: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{
			f.a3.Headline, f.a2.Headline, f.a4.Headline,
		})

		// Count respects Distinct.
		n, err := orm.Query[m2mArticle](db).Filter("publications__title__startswith", "Science").Count(ctx)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(4))

		n, err = orm.Query[m2mArticle](db).Filter("publications__title__startswith", "Science").Distinct().Count(ctx)
		if err != nil {
			t.Fatalf("distinct count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(3))

		// IN lookup with IDs, mixed IDs/objects, and objects.
		for _, inVals := range []any{
			[]int64{f.p1.ID, f.p2.ID},
			[]any{f.p1.ID, f.p2},
			[]*m2mPublication{f.p1, f.p2},
		} {
			arts, err = orm.Query[m2mArticle](db).Filter("publications__in", inVals).Distinct().All(ctx)
			if err != nil {
				t.Fatalf("filter publications__in: %v", err)
			}
			djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{
				f.a1.Headline, f.a3.Headline, f.a2.Headline, f.a4.Headline,
			})
		}

		// Exclude across M2M uses correlated EXISTS.
		arts, err = orm.Query[m2mArticle](db).Exclude("publications", f.p2).All(ctx)
		if err != nil {
			t.Fatalf("exclude publications=p2: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a1.Headline})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_reverse_selects
func TestManyToManyReverseSelects(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		pubs, err := orm.Query[m2mPublication](db).Filter("m2marticle__headline__startswith", "NASA").All(ctx)
		if err != nil {
			t.Fatalf("reverse filter startswith NASA: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{
			f.p4.Title, f.p2.Title, f.p2.Title, f.p3.Title, f.p1.Title,
		})

		for _, tc := range []struct {
			key string
			val any
		}{
			{"m2marticle__id__exact", f.a1.ID},
			{"m2marticle__pk", f.a1.ID},
			{"m2marticle", f.a1.ID},
			{"m2marticle", f.a1},
		} {
			pubs, err = orm.Query[m2mPublication](db).Filter(tc.key, tc.val).All(ctx)
			if err != nil {
				t.Fatalf("reverse filter %s: %v", tc.key, err)
			}
			djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p1.Title})
		}

		for _, inVals := range []any{
			[]int64{f.a1.ID, f.a2.ID},
			[]any{f.a1.ID, f.a2},
			[]*m2mArticle{f.a1, f.a2},
		} {
			pubs, err = orm.Query[m2mPublication](db).Filter("m2marticle__in", inVals).Distinct().All(ctx)
			if err != nil {
				t.Fatalf("reverse filter in: %v", err)
			}
			djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{
				f.p4.Title, f.p2.Title, f.p3.Title, f.p1.Title,
			})
		}
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_delete
func TestManyToManyDelete(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		if _, err := orm.Query[m2mPublication](db).Filter("pk", f.p1.ID).Delete(ctx); err != nil {
			t.Fatalf("delete p1: %v", err)
		}
		pubs, err := orm.Query[m2mPublication](db).All(ctx)
		if err != nil {
			t.Fatalf("all pubs: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p4.Title, f.p2.Title, f.p3.Title})

		a1Pubs, err := orm.Many[m2mPublication](db, f.a1, "publications").All(ctx)
		if err != nil {
			t.Fatalf("a1.publications: %v", err)
		}
		djangotest.AssertEqual(t, len(a1Pubs), 0)

		if _, err := orm.Query[m2mArticle](db).Filter("pk", f.a2.ID).Delete(ctx); err != nil {
			t.Fatalf("delete a2: %v", err)
		}
		arts, err := orm.Query[m2mArticle](db).All(ctx)
		if err != nil {
			t.Fatalf("all articles: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a1.Headline, f.a3.Headline, f.a4.Headline})

		p2Arts, err := orm.Many[m2mArticle](db, f.p2, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("p2.m2marticle_set: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(p2Arts), []string{f.a3.Headline, f.a4.Headline})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_bulk_delete
func TestManyToManyBulkDelete(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		if _, err := orm.Query[m2mPublication](db).Filter("title__startswith", "Science").Delete(ctx); err != nil {
			t.Fatalf("bulk delete pubs: %v", err)
		}
		pubs, err := orm.Query[m2mPublication](db).All(ctx)
		if err != nil {
			t.Fatalf("all pubs: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p4.Title, f.p1.Title})

		a2Pubs, err := orm.Many[m2mPublication](db, f.a2, "publications").All(ctx)
		if err != nil {
			t.Fatalf("a2.publications: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(a2Pubs), []string{f.p4.Title, f.p1.Title})

		q := orm.Query[m2mArticle](db).Filter("headline__startswith", "Django")
		if _, err := q.Delete(ctx); err != nil {
			t.Fatalf("bulk delete articles: %v", err)
		}
		rem, err := q.All(ctx)
		if err != nil {
			t.Fatalf("re-query: %v", err)
		}
		djangotest.AssertEqual(t, len(rem), 0)

		p1Arts, err := orm.Many[m2mArticle](db, f.p1, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("p1.m2marticle_set: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(p1Arts), []string{f.a2.Headline})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_remove
func TestManyToManyRemove(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		if err := orm.Many[m2mPublication](db, f.a4, "publications").Remove(ctx, f.p2); err != nil {
			t.Fatalf("forward Remove: %v", err)
		}
		arts, err := orm.Many[m2mArticle](db, f.p2, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("p2.m2marticle_set: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a3.Headline, f.a2.Headline})

		pubs, err := orm.Many[m2mPublication](db, f.a4, "publications").All(ctx)
		if err != nil {
			t.Fatalf("a4.publications: %v", err)
		}
		djangotest.AssertEqual(t, len(pubs), 0)

		// Reverse remove.
		if err := orm.Many[m2mArticle](db, f.p2, "m2marticle_set").Remove(ctx, f.a3); err != nil {
			t.Fatalf("reverse Remove: %v", err)
		}
		arts, err = orm.Many[m2mArticle](db, f.p2, "m2marticle_set").All(ctx)
		if err != nil {
			t.Fatalf("p2.m2marticle_set after reverse remove: %v", err)
		}
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a2.Headline})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_set
func TestManyToManySet(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		p2Mgr := orm.Many[m2mArticle](db, f.p2, "m2marticle_set")
		a4Mgr := orm.Many[m2mPublication](db, f.a4, "publications")

		if err := p2Mgr.Set(ctx, []*m2mArticle{f.a4, f.a3}); err != nil {
			t.Fatalf("p2.Set: %v", err)
		}
		arts, _ := p2Mgr.All(ctx)
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a3.Headline, f.a4.Headline})

		if err := a4Mgr.SetPKs(ctx, []any{f.p3.ID}); err != nil {
			t.Fatalf("a4.SetPKs: %v", err)
		}
		arts, _ = p2Mgr.All(ctx)
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a3.Headline})
		pubs, _ := a4Mgr.All(ctx)
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p3.Title})

		// Empty set clears.
		if err := p2Mgr.Set(ctx, nil); err != nil {
			t.Fatalf("p2.Set(nil): %v", err)
		}
		arts, _ = p2Mgr.All(ctx)
		djangotest.AssertEqual(t, len(arts), 0)

		// With SetClear() option.
		if err := p2Mgr.Set(ctx, []*m2mArticle{f.a4, f.a3}, orm.SetClear()); err != nil {
			t.Fatalf("p2.Set(clear): %v", err)
		}
		arts, _ = p2Mgr.All(ctx)
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a3.Headline, f.a4.Headline})

		if err := a4Mgr.SetPKs(ctx, []any{f.p3.ID}, orm.SetClear()); err != nil {
			t.Fatalf("a4.SetPKs(clear): %v", err)
		}
		pubs, _ = a4Mgr.All(ctx)
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p3.Title})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_clear
func TestManyToManyClear(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		p2Mgr := orm.Many[m2mArticle](db, f.p2, "m2marticle_set")
		a4Mgr := orm.Many[m2mPublication](db, f.a4, "publications")

		if err := p2Mgr.Clear(ctx); err != nil {
			t.Fatalf("p2.Clear: %v", err)
		}
		arts, _ := p2Mgr.All(ctx)
		djangotest.AssertEqual(t, len(arts), 0)
		pubs, _ := a4Mgr.All(ctx)
		djangotest.AssertEqual(t, len(pubs), 0)

		if err := p2Mgr.Add(ctx, f.a3, f.a4); err != nil {
			t.Fatalf("p2.Add: %v", err)
		}
		if err := a4Mgr.Clear(ctx); err != nil {
			t.Fatalf("a4.Clear: %v", err)
		}
		pubs, _ = a4Mgr.All(ctx)
		djangotest.AssertEqual(t, len(pubs), 0)
		arts, _ = p2Mgr.All(ctx)
		djangotest.AssertSequenceEqual(t, articleHeadlines(arts), []string{f.a3.Headline})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyTests.test_inherited_models_selects
func TestManyToManyHiddenReverseSelects(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := m2mSetUp(t, db)

		a := create(t, db, &m2mHiddenArticleA{})
		b := create(t, db, &m2mHiddenArticleB{})
		if err := orm.Many[m2mPublication](db, a, "publications").Add(ctx, f.p1, f.p2); err != nil {
			t.Fatalf("a.publications.Add: %v", err)
		}
		pubs, err := orm.Many[m2mPublication](db, a, "publications").All(ctx)
		if err != nil {
			t.Fatalf("a.publications.All: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(pubs), []string{f.p2.Title, f.p1.Title})

		bPubs, err := orm.Many[m2mPublication](db, b, "publications").All(ctx)
		if err != nil {
			t.Fatalf("b.publications.All: %v", err)
		}
		djangotest.AssertEqual(t, len(bPubs), 0)

		if err := orm.Many[m2mPublication](db, b, "publications").Add(ctx, f.p3); err != nil {
			t.Fatalf("b.publications.Add: %v", err)
		}
		bPubs, err = orm.Many[m2mPublication](db, b, "publications").All(ctx)
		if err != nil {
			t.Fatalf("b.publications.All after add: %v", err)
		}
		djangotest.AssertSequenceEqual(t, pubTitles(bPubs), []string{f.p3.Title})
	})
}

// Ported from Django: tests/many_to_many/tests.py::ManyToManyQueryTests.test_count_join_optimization
// Ported from Django: tests/many_to_many/tests.py::ManyToManyQueryTests.test_exists_join_optimization
func TestManyToManyCountAndExistsOptimization(t *testing.T) {
	djangotest.EachDB(t, m2mModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		art := create(t, db, &m2mArticle{Headline: "Django lets you build Web apps easily"})
		nullableArt := create(t, db, &m2mNullableTargetArticle{Headline: "The python is good"})
		create(t, db, &m2mNullablePublicationThrough{Article: fkTo[m2mNullableTargetArticle](nullableArt.ID)})

		qc := djangotest.CaptureQueries(db)
		n, err := orm.Many[m2mPublication](db, art, "publications").Count(ctx)
		qc.Stop()
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(0))
		djangotest.AssertEqual(t, len(qc.Queries()), 1)
		if strings.Contains(qc.Queries()[0].SQL, "JOIN") {
			t.Errorf("Count SQL unexpectedly contains JOIN: %s", qc.Queries()[0].SQL)
		}

		// Nullable target FK row with NULL publication must not be counted.
		n, err = orm.Many[m2mPublication](db, nullableArt, "publications").Count(ctx)
		if err != nil {
			t.Fatalf("nullable Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(0))

		qc = djangotest.CaptureQueries(db)
		ok, err := orm.Many[m2mPublication](db, art, "publications").Exists(ctx)
		qc.Stop()
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		djangotest.AssertEqual(t, ok, false)
		djangotest.AssertEqual(t, len(qc.Queries()), 1)
		if strings.Contains(qc.Queries()[0].SQL, "JOIN") {
			t.Errorf("Exists SQL unexpectedly contains JOIN: %s", qc.Queries()[0].SQL)
		}

		ok, err = orm.Many[m2mPublication](db, nullableArt, "publications").Exists(ctx)
		if err != nil {
			t.Fatalf("nullable Exists: %v", err)
		}
		djangotest.AssertEqual(t, ok, false)

		// Verify SelectRelated rejects M2M fields.
		_, err = orm.Query[m2mArticle](db).SelectRelated("publications").All(ctx)
		if !errors.Is(err, orm.ErrFieldError) {
			t.Errorf("SelectRelated(publications) error = %v, want ErrFieldError", err)
		}
	})
}
