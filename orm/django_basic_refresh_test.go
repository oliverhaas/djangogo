package orm_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// basRefresh calls RefreshFromDB on qs and fails the test on an error.
func basRefresh[T any](t *testing.T, qs *orm.QuerySet[T], obj *T, keys ...string) {
	t.Helper()
	if err := qs.RefreshFromDB(context.Background(), obj, keys...); err != nil {
		t.Fatalf("RefreshFromDB: %v", err)
	}
}

// basNow returns the current time at the precision that both backends store.
func basNow() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh
func TestBasicRefresh(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		articles := orm.Query[basArticle](db)
		a := create(t, db, &basArticle{PubDate: basNow()})
		create(t, db, &basArticle{PubDate: basNow()})
		if _, err := articles.Filter("pk", a.ID).Update(ctx, "headline", "new headline"); err != nil {
			t.Fatalf("Update: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, articles, a)
			djangotest.AssertEqual(t, a.Headline, "new headline")
		})

		origPubDate := a.PubDate
		newPubDate := a.PubDate.AddDate(0, 0, 10)
		if _, err := articles.Update(ctx, "headline", "new headline 2", "pub_date", newPubDate); err != nil {
			t.Fatalf("Update: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, articles, a, "headline")
			djangotest.AssertEqual(t, a.Headline, "new headline 2")
			djangotest.AssertEqual(t, a.PubDate.Equal(origPubDate), true)
		})
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, articles, a)
			djangotest.AssertEqual(t, a.PubDate.Equal(newPubDate), true)
		})
	})
}

// No Go counterpart: tests/basic/tests.py::ModelRefreshTests.test_unknown_kwarg
// checks the TypeError of an unknown keyword argument, which the Go compiler
// rejects.

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_lookup_in_fields
func TestBasicRefreshLookupInFields(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		s := create(t, db, &basSelfRef{})
		err := orm.Query[basSelfRef](db).RefreshFromDB(context.Background(), s, "foo__bar")
		assertErrorContains(t, err, `found "__" in the keys: relations and transforms are not allowed`)
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_fk
func TestBasicRefreshFK(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		refs := orm.Query[basSelfRef](db)
		s1 := create(t, db, &basSelfRef{})
		s2 := create(t, db, &basSelfRef{})
		s3 := create(t, db, &basSelfRef{Selfref: fkTo[basSelfRef](s1.ID)})
		// Django loads s3_copy.selfref on access; Go loads it with
		// SelectRelated.
		s3Copy, err := refs.SelectRelated("selfref").Get(ctx, "pk", s3.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		s3.Selfref = fkTo[basSelfRef](s2.ID)
		if err := refs.Save(ctx, s3); err != nil {
			t.Fatalf("Save: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, refs, &s3Copy)
		})
		djangotest.AssertNumQueries(t, db, 1, func() {
			// The old related object was dropped: the selfref_id changed.
			// Fetch loads the new one, so one query runs.
			if _, ok := s3Copy.Selfref.Object(); ok {
				t.Error("the old related object is still cached")
			}
			got, err := s3Copy.Selfref.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			djangotest.AssertEqual(t, got.ID, s2.ID)
		})
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_null_fk
func TestBasicRefreshNullFK(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		s1 := create(t, db, &basSelfRef{})
		s2 := create(t, db, &basSelfRef{Selfref: fkTo[basSelfRef](s1.ID)})
		s2.Selfref = orm.FK[basSelfRef]{}
		basRefresh(t, orm.Query[basSelfRef](db), s2)
		djangotest.AssertEqual(t, s2.Selfref.PK(), s1.ID)
	})
}

// Django also checks that the refreshed object records the database it came
// from; Go has no database routing.
//
// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_unsaved
func TestBasicRefreshUnsaved(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		pubDate := basNow()
		a := create(t, db, &basArticle{PubDate: pubDate})
		a2 := &basArticle{ID: a.ID}
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, orm.Query[basArticle](db), a2)
		})
		djangotest.AssertEqual(t, a2.PubDate.Equal(pubDate), true)
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_fk_on_delete_set_null
func TestBasicRefreshFKOnDeleteSetNull(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := create(t, db, &basArticle{Headline: "Parrot programs in Python", PubDate: day(2005, 7, 28)})
		s1 := create(t, db, &basSelfRef{Article: fkTo[basArticle](a.ID)})
		if _, err := orm.Query[basArticle](db).Filter("pk", a.ID).Delete(context.Background()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		basRefresh(t, orm.Query[basSelfRef](db), s1)
		// A null foreign key holds the primary key 0 and no object.
		djangotest.AssertEqual(t, s1.Article.PK(), int64(0))
		if _, ok := s1.Article.Object(); ok {
			t.Error("the deleted article is still cached")
		}
	})
}

// No Go counterpart: tests/basic/tests.py::ModelRefreshTests.test_refresh_no_fields
// checks that refresh_from_db(fields=[]) runs no query. RefreshFromDB without
// keys loads every field, and a caller does not tell an empty list of keys
// from none.

// basAssertFeatured checks whether RelatedOne finds the featured article of
// article.
func basAssertFeatured(t *testing.T, db *orm.DB, article *basArticle, want bool) {
	t.Helper()
	_, err := orm.RelatedOne[basFeaturedArticle](context.Background(), db, article, "featured")
	switch {
	case want && err != nil:
		t.Errorf("RelatedOne: %v", err)
	case !want && !errors.Is(err, orm.ErrDoesNotExist):
		t.Errorf("RelatedOne: error = %v, want ErrDoesNotExist", err)
	}
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_clears_reverse_related
func TestBasicRefreshClearsReverseRelated(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		article := create(t, db, &basArticle{Headline: "Parrot programs in Python", PubDate: day(2005, 7, 28)})
		basAssertFeatured(t, db, article, false)
		create(t, db, &basFeaturedArticle{Article: o2oTo[basArticle](article.ID)})
		basRefresh(t, orm.Query[basArticle](db), article)
		basAssertFeatured(t, db, article, true)
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_clears_reverse_related_explicit_fields
func TestBasicRefreshClearsReverseRelatedExplicitFields(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		article := create(t, db, &basArticle{Headline: "Test", PubDate: day(2024, 2, 4)})
		basAssertFeatured(t, db, article, false)
		create(t, db, &basFeaturedArticle{Article: o2oTo[basArticle](article.ID)})
		basRefresh(t, orm.Query[basArticle](db), article, "featured")
		basAssertFeatured(t, db, article, true)
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_clears_one_to_one_field
func TestBasicRefreshClearsOneToOneField(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		article := create(t, db, &basArticle{Headline: "Parrot programs in Python", PubDate: day(2005, 7, 28)})
		featured := create(t, db, &basFeaturedArticle{Article: o2oTo[basArticle](article.ID)})
		headline := func() string {
			t.Helper()
			got, err := featured.Article.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			return got.Headline
		}
		djangotest.AssertEqual(t, headline(), "Parrot programs in Python")
		article.Headline = "Parrot programs in Python 2.0"
		basSave(t, db, article)
		basRefresh(t, orm.Query[basFeaturedArticle](db), featured)
		djangotest.AssertEqual(t, headline(), "Parrot programs in Python 2.0")
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_prefetched_cache_cleared
func TestBasicRefreshPrefetchedCacheCleared(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		articles := orm.Query[basArticle](db)
		a := create(t, db, &basArticle{PubDate: day(2005, 7, 28)})
		s := create(t, db, &basSelfRef{Article: fkTo[basArticle](a.ID), ArticleCited: fkTo[basArticle](a.ID)})
		// assertRefs checks the selfrefs that the two reverse relations of a
		// hold.
		assertRefs := func(a *basArticle, want ...int64) {
			t.Helper()
			for _, name := range []string{"basselfref_set", "cited"} {
				var got []int64
				for _, r := range mustAll(t, orm.Related[basSelfRef](db, a, name)) {
					got = append(got, r.ID)
				}
				djangotest.AssertEqual(t, got, want)
			}
		}
		prefetched := func() *basArticle {
			t.Helper()
			got, err := articles.PrefetchRelated("basselfref_set", "cited").First(ctx)
			if err != nil {
				t.Fatalf("First: %v", err)
			}
			return &got
		}
		saveRef := func(article orm.FK[basArticle]) {
			t.Helper()
			s.Article, s.ArticleCited = article, article
			if err := orm.Query[basSelfRef](db).Save(ctx, s); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}

		// RefreshFromDB without keys.
		a1 := prefetched()
		assertRefs(a1, s.ID)
		saveRef(orm.FK[basArticle]{})
		// The relation is cleared and the prefetched lists are stale.
		assertRefs(a1, s.ID)
		basRefresh(t, articles, a1)
		// The lists were dropped and the new rows are read.
		assertRefs(a1)

		// RefreshFromDB with keys.
		a2 := prefetched()
		assertRefs(a2)
		saveRef(fkTo[basArticle](a.ID))
		// The relation is added and the prefetched lists are stale.
		assertRefs(a2)
		fields := []string{"basselfref_set", "cited"}
		basRefresh(t, articles, a2, fields...)
		djangotest.AssertEqual(t, fields, []string{"basselfref_set", "cited"})
		// The lists were dropped and the new rows are read.
		assertRefs(a2, s.ID)
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_for_update
func TestBasicRefreshForUpdate(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "sqlite" {
			t.Skip("needs SELECT ... FOR UPDATE, which SQLite does not have")
		}
		a := create(t, db, &basArticle{PubDate: basNow()})
		capture := djangotest.CaptureQueries(db)
		err := db.Atomic(context.Background(), func(ctx context.Context) error {
			return orm.Query[basArticle](db).SelectForUpdate().RefreshFromDB(ctx, a)
		})
		capture.Stop()
		if err != nil {
			t.Fatalf("Atomic: %v", err)
		}
		if !slices.ContainsFunc(capture.Queries(), func(q djangotest.CapturedQuery) bool {
			return strings.Contains(q.SQL, "FOR UPDATE")
		}) {
			t.Errorf("no query selects FOR UPDATE: %v", capture.Queries())
		}
	})
}

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_with_related
func TestBasicRefreshWithRelated(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := create(t, db, &basArticle{PubDate: basNow()})
		fa := create(t, db, &basFeaturedArticle{Article: o2oTo[basArticle](a.ID)})
		pubDate := func() time.Time {
			t.Helper()
			got, err := fa.Article.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			return got.PubDate
		}

		fromQuerySet := orm.Query[basFeaturedArticle](db).SelectRelated("article")
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, fromQuerySet, fa)
			djangotest.AssertEqual(t, pubDate().Equal(a.PubDate), true)
		})
		djangotest.AssertNumQueries(t, db, 2, func() {
			basRefresh(t, orm.Query[basFeaturedArticle](db), fa)
			djangotest.AssertEqual(t, pubDate().Equal(a.PubDate), true)
		})
	})
}

// No Go counterpart: tests/basic/tests.py::ModelRefreshTests.test_refresh_overwrites_queryset_using
// checks that the using argument replaces the database of from_queryset;
// RefreshFromDB has no using argument, and the queryset's DB is the database.

// Ported from Django: tests/basic/tests.py::ModelRefreshTests.test_refresh_overwrites_queryset_fields
func TestBasicRefreshOverwritesQuerysetFields(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := create(t, db, &basArticle{PubDate: basNow()})
		const headline = "headline"
		if _, err := orm.Query[basArticle](db).Filter("pk", a.ID).Update(ctx, "headline", headline); err != nil {
			t.Fatalf("Update: %v", err)
		}

		fromQuerySet := orm.Query[basArticle](db).Only("pub_date")
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, fromQuerySet, a)
			if a.Headline == headline {
				t.Errorf("Headline = %q, want the old value", a.Headline)
			}
		})
		djangotest.AssertNumQueries(t, db, 1, func() {
			basRefresh(t, fromQuerySet, a, "headline")
			djangotest.AssertEqual(t, a.Headline, headline)
		})
	})
}
