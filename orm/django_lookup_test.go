package orm_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// lkAuthor is Author of Django's tests/lookup/models.py.
type lkAuthor struct {
	ID    int64
	Name  string `orm:"max_length=100"`
	Alias string `orm:"max_length=50;null;blank"`
	Bio   string `orm:"type=text;null"`
}

// lkArticle is Article of Django's tests/lookup/models.py. Its Meta ordering
// ("-pub_date", "headline") is not available yet, so the ports order
// explicitly through lkArticles.
type lkArticle struct {
	ID       int64
	Headline string `orm:"max_length=100"`
	PubDate  time.Time
	Author   orm.FK[lkAuthor] `orm:"null;blank;on_delete=set_null"`
	Slug     string           `orm:"unique;blank;null"`
}

// lkSeason is Season of Django's tests/lookup/models.py, without the fields
// that only its custom lookups use.
type lkSeason struct {
	ID   int64
	Year int64
	Gt   int64 `orm:"null;blank"`
}

// lkPlayer is Player of Django's tests/lookup/models.py.
type lkPlayer struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// lkFixture is the data of LookupTests.setUpTestData. The tags are left out:
// they hang on a many-to-many relation.
type lkFixture struct {
	au1, au2                   *lkAuthor
	a1, a2, a3, a4, a5, a6, a7 *lkArticle
}

// lkModels returns the models of the lookup app.
func lkModels() []any {
	return []any{djangotest.App("lookup", &lkAuthor{}, &lkArticle{}, &lkSeason{}, &lkPlayer{})}
}

// lkSetNull stores NULL in one column of one row. A plain Go field cannot
// hold NULL, so the ports clear the column after the row is created.
func lkSetNull[T any](t *testing.T, db *orm.DB, pk int64, key string) {
	t.Helper()
	if _, err := orm.Query[T](db).Filter("pk", pk).Update(context.Background(), key, nil); err != nil {
		t.Fatalf("set %s to NULL: %v", key, err)
	}
}

// lkSetUp creates the authors and articles of LookupTests.setUpTestData.
func lkSetUp(t *testing.T, db *orm.DB) lkFixture {
	t.Helper()
	var f lkFixture
	f.au1 = create(t, db, &lkAuthor{Name: "Author 1", Alias: "a1", Bio: strings.Repeat("x", 4001)})
	f.au2 = create(t, db, &lkAuthor{Name: "Author 2", Alias: "a2"})
	lkSetNull[lkAuthor](t, db, f.au2.ID, "bio")
	article := func(headline string, pubDate time.Time, author *lkAuthor, slug string) *lkArticle {
		return create(t, db, &lkArticle{
			Headline: headline, PubDate: pubDate, Author: fkTo[lkAuthor](author.ID), Slug: slug,
		})
	}
	f.a1 = article("Article 1", day(2005, 7, 26), f.au1, "a1")
	f.a2 = article("Article 2", day(2005, 7, 27), f.au1, "a2")
	f.a3 = article("Article 3", day(2005, 7, 27), f.au1, "a3")
	f.a4 = article("Article 4", day(2005, 7, 28), f.au1, "a4")
	f.a5 = article("Article 5", time.Date(2005, 8, 1, 9, 0, 0, 0, time.UTC), f.au2, "a5")
	f.a6 = article("Article 6", time.Date(2005, 8, 1, 8, 0, 0, 0, time.UTC), f.au2, "a6")
	f.a7 = article("Article 7", day(2005, 7, 27), f.au2, "a7")
	return f
}

// lkArticles returns the articles in the model's default order.
func lkArticles(db *orm.DB) *orm.QuerySet[lkArticle] {
	return orm.Query[lkArticle](db).OrderBy("-pub_date", "headline")
}

// lkAuthors returns the authors in the model's default order.
func lkAuthors(db *orm.DB) *orm.QuerySet[lkAuthor] {
	return orm.Query[lkAuthor](db).OrderBy("name")
}

// lkID is the transform of the ports that compare model instances.
func lkID(a lkArticle) int64 { return a.ID }

// lkHeadline is the transform attrgetter("headline").
func lkHeadline(a lkArticle) string { return a.Headline }

// lkIDs returns the primary keys of the articles.
func lkIDs(articles ...*lkArticle) []int64 {
	ids := make([]int64, len(articles))
	for i, a := range articles {
		ids[i] = a.ID
	}
	return ids
}

// lkSpecials creates the three articles with LIKE wildcards in their headline
// that test_escaping and test_exclude use. Django leaves their slug NULL; a Go
// string cannot be NULL and the slug is unique, so they get slugs here.
func lkSpecials(t *testing.T, db *orm.DB) (a8, a9, a10 *lkArticle) {
	t.Helper()
	a8 = create(t, db, &lkArticle{Headline: "Article_ with underscore", PubDate: day(2005, 11, 20), Slug: "a8"})
	a9 = create(t, db, &lkArticle{Headline: "Article% with percent sign", PubDate: day(2005, 11, 21), Slug: "a9"})
	a10 = create(t, db, &lkArticle{Headline: `Article with \ backslash`, PubDate: day(2005, 11, 22), Slug: "a10"})
	return a8, a9, a10
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_exists
//
// A Go model has no delete() method; each row is deleted through a queryset.
func TestLookupExists(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)
		ctx := context.Background()

		// We can use Exists to check that there are some.
		ok, err := orm.Query[lkArticle](db).Exists(ctx)
		if err != nil || !ok {
			t.Fatalf("Exists = %v, %v; want true", ok, err)
		}
		for _, a := range mustAll(t, orm.Query[lkArticle](db)) {
			if _, err := orm.Query[lkArticle](db).Filter("pk", a.ID).Delete(ctx); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		}
		// There should be none now.
		ok, err = orm.Query[lkArticle](db).Exists(ctx)
		if err != nil || ok {
			t.Fatalf("Exists = %v, %v; want false", ok, err)
		}
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_lookup_int_as_str
func TestLookupIntAsStr(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		// An integer value can be queried using a string.
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("id__iexact", strconv.FormatInt(f.a1.ID, 10)), lkIDs(f.a1), lkID)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_lookup_date_as_str
func TestLookupDateAsStr(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		// A date lookup can be performed using a string search.
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("pub_date__startswith", "2005"),
			lkIDs(f.a5, f.a6, f.a4, f.a2, f.a3, f.a7, f.a1), lkID)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_count
//
// Slices are Limit and Offset. The last assertion of the Django test, a date
// given as a string, is left out: a Go query compares a time.Time.
func TestLookupCount(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)

		// Count returns the number of objects matching search criteria.
		djangotest.AssertEqual(t, mustCount(t, orm.Query[lkArticle](db)), int64(7))
		djangotest.AssertEqual(t,
			mustCount(t, orm.Query[lkArticle](db).Filter("pub_date__exact", day(2005, 7, 27))), int64(3))
		djangotest.AssertEqual(t,
			mustCount(t, orm.Query[lkArticle](db).Filter("headline__startswith", "Blah blah")), int64(0))

		// Count should respect sliced query sets.
		articles := lkArticles(db)
		djangotest.AssertEqual(t, mustCount(t, articles), int64(7))
		djangotest.AssertEqual(t, mustCount(t, articles.Limit(4)), int64(4))
		djangotest.AssertEqual(t, mustCount(t, articles.Offset(1).Limit(99)), int64(6))
		djangotest.AssertEqual(t, mustCount(t, articles.Offset(10).Limit(90)), int64(0))
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_bulk
//
// in_bulk() is the function orm.InBulk and always takes the list of keys, so
// the forms without a list, with a set, or with an iterator have no
// counterpart.
func TestLookupInBulk(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)
		ctx := context.Background()
		inBulk := func(pks []int64) map[int64]lkArticle {
			t.Helper()
			arts, err := orm.InBulk(ctx, orm.Query[lkArticle](db), pks)
			if err != nil {
				t.Fatalf("InBulk: %v", err)
			}
			return arts
		}

		// InBulk takes a list of IDs and returns a map from ID to object.
		arts := inBulk([]int64{f.a1.ID, f.a2.ID})
		djangotest.AssertEqual(t, len(arts), 2)
		djangotest.AssertEqual(t, arts[f.a1.ID].Headline, "Article 1")
		djangotest.AssertEqual(t, arts[f.a2.ID].Headline, "Article 2")

		arts = inBulk([]int64{f.a3.ID})
		djangotest.AssertEqual(t, len(arts), 1)
		djangotest.AssertEqual(t, arts[f.a3.ID].Headline, "Article 3")

		djangotest.AssertEqual(t, len(inBulk([]int64{1000})), 0)
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertEqual(t, len(inBulk([]int64{})), 0)
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_bulk_lots_of_ids
func TestLookupInBulkLotsOfIds(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)
		ctx := context.Background()
		const testRange = 2000

		expectedNumQueries := 1
		if fd, ok := db.Dialect().(orm.FeatureDialect); ok {
			if maxParams := fd.Features().MaxQueryParams; maxParams > 0 {
				expectedNumQueries = (testRange + maxParams - 1) / maxParams
			}
		}
		extra := make([]*lkAuthor, testRange-int(mustCount(t, orm.Query[lkAuthor](db))))
		for i := range extra {
			extra[i] = &lkAuthor{}
		}
		if err := orm.Query[lkAuthor](db).BulkCreate(ctx, extra); err != nil {
			t.Fatalf("BulkCreate: %v", err)
		}
		pks, err := orm.Pluck[int64](ctx, orm.Query[lkAuthor](db), "pk")
		if err != nil {
			t.Fatalf("Pluck: %v", err)
		}
		djangotest.AssertEqual(t, len(pks), testRange)

		djangotest.AssertNumQueries(t, db, expectedNumQueries, func() {
			authors, err := orm.InBulk(ctx, orm.Query[lkAuthor](db), pks)
			if err != nil {
				t.Fatalf("InBulk: %v", err)
			}
			djangotest.AssertEqual(t, len(authors), testRange)
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_bulk_sliced_queryset
func TestLookupInBulkSlicedQueryset(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		_, err := orm.InBulk(context.Background(), lkArticles(db).Limit(5), []int64{f.a1.ID, f.a2.ID})
		assertErrorContains(t, err, "cannot use 'limit' or 'offset' with in_bulk()")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_filter_and_no_fields
func TestLookupValuesFilterAndNoFields(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		// Values returns a list of maps instead of object instances, and you can
		// specify which fields you want to retrieve.
		assertValues(t, lkArticles(db).Filter("id__in", [2]int64{f.a5.ID, f.a6.ID}).Values(), []map[string]any{
			{
				"id":        f.a5.ID,
				"headline":  "Article 5",
				"pub_date":  time.Date(2005, 8, 1, 9, 0, 0, 0, time.UTC),
				"author_id": f.au2.ID,
				"slug":      "a5",
			},
			{
				"id":        f.a6.ID,
				"headline":  "Article 6",
				"pub_date":  time.Date(2005, 8, 1, 8, 0, 0, 0, time.UTC),
				"author_id": f.au2.ID,
				"slug":      "a6",
			},
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_single_field
func TestLookupValuesSingleField(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)

		assertValues(t, lkArticles(db).Values("headline"), []map[string]any{
			{"headline": "Article 5"},
			{"headline": "Article 6"},
			{"headline": "Article 4"},
			{"headline": "Article 2"},
			{"headline": "Article 3"},
			{"headline": "Article 7"},
			{"headline": "Article 1"},
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_filter_and_single_field
func TestLookupValuesFilterAndSingleField(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		assertValues(t, lkArticles(db).Filter("pub_date__exact", day(2005, 7, 27)).Values("id"),
			[]map[string]any{{"id": f.a2.ID}, {"id": f.a3.ID}, {"id": f.a7.ID}})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_two_fields
func TestLookupValuesTwoFields(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		assertValues(t, lkArticles(db).Values("id", "headline"), []map[string]any{
			{"id": f.a5.ID, "headline": "Article 5"},
			{"id": f.a6.ID, "headline": "Article 6"},
			{"id": f.a4.ID, "headline": "Article 4"},
			{"id": f.a2.ID, "headline": "Article 2"},
			{"id": f.a3.ID, "headline": "Article 3"},
			{"id": f.a7.ID, "headline": "Article 7"},
			{"id": f.a1.ID, "headline": "Article 1"},
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_relations
//
// Only the forward relation is ported. The other two assertions of the Django
// test select through the reverse foreign key and the many-to-many relation,
// which the ORM does not have yet.
func TestLookupValuesRelations(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		// You can specify fields from forward relations, just like Filter.
		assertValues(t, lkArticles(db).Values("headline", "author__name"), []map[string]any{
			{"headline": f.a5.Headline, "author__name": f.au2.Name},
			{"headline": f.a6.Headline, "author__name": f.au2.Name},
			{"headline": f.a4.Headline, "author__name": f.au1.Name},
			{"headline": f.a2.Headline, "author__name": f.au1.Name},
			{"headline": f.a3.Headline, "author__name": f.au1.Name},
			{"headline": f.a7.Headline, "author__name": f.au2.Name},
			{"headline": f.a1.Headline, "author__name": f.au1.Name},
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_no_field_names
func TestLookupValuesNoFieldNames(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		// If you don't specify field names to Values, all are returned.
		assertValues(t, orm.Query[lkArticle](db).Filter("id", f.a5.ID).Values(), []map[string]any{{
			"id":        f.a5.ID,
			"author_id": f.au2.ID,
			"headline":  "Article 5",
			"pub_date":  time.Date(2005, 8, 1, 9, 0, 0, 0, time.UTC),
			"slug":      "a5",
		}})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_list_filter_and_no_fields
func TestLookupValuesListFilterAndNoFields(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		// ValuesList is similar to Values, except that the results are slices in
		// the order of the keys, here the order of the model's fields.
		assertValuesList(t, lkArticles(db).Filter("id__in", [2]int64{f.a5.ID, f.a6.ID}).ValuesList(), [][]any{
			{f.a5.ID, "Article 5", time.Date(2005, 8, 1, 9, 0, 0, 0, time.UTC), f.au2.ID, "a5"},
			{f.a6.ID, "Article 6", time.Date(2005, 8, 1, 8, 0, 0, 0, time.UTC), f.au2.ID, "a6"},
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_list_single_field
func TestLookupValuesListSingleField(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)

		assertValuesList(t, lkArticles(db).ValuesList("headline"), [][]any{
			{"Article 5"}, {"Article 6"}, {"Article 4"}, {"Article 2"}, {"Article 3"}, {"Article 7"}, {"Article 1"},
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_list_single_field_order_by
func TestLookupValuesListSingleFieldOrderBy(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		assertValuesList(t, orm.Query[lkArticle](db).ValuesList("id").OrderBy("id"), [][]any{
			{f.a1.ID}, {f.a2.ID}, {f.a3.ID}, {f.a4.ID}, {f.a5.ID}, {f.a6.ID}, {f.a7.ID},
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_list_flat_order_by
//
// flat=True is the method Flat, and orm.Pluck is its typed form.
func TestLookupValuesListFlatOrderBy(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		assertFlat(t, orm.Query[lkArticle](db).ValuesList("id").OrderBy("id"),
			[]any{f.a1.ID, f.a2.ID, f.a3.ID, f.a4.ID, f.a5.ID, f.a6.ID, f.a7.ID})

		ids, err := orm.Pluck[int64](context.Background(), orm.Query[lkArticle](db).OrderBy("id"), "id")
		if err != nil {
			t.Fatalf("Pluck: %v", err)
		}
		djangotest.AssertSequenceEqual(t, ids, lkIDs(f.a1, f.a2, f.a3, f.a4, f.a5, f.a6, f.a7))
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_list_relations
func TestLookupValuesListRelations(t *testing.T) {
	t.Skip("selects through a reverse foreign key and a many-to-many relation, which the ORM does not have yet")
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_values_list_flat_more_than_one_field
//
// Flat is a method here, so the error comes from calling it rather than from
// building the queryset, and names the Go identifiers.
func TestLookupValuesListFlatMoreThanOneField(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)

		_, err := orm.Query[lkArticle](db).ValuesList("id", "headline").Flat(context.Background())
		assertErrorContains(t, err, "Flat is not valid when ValuesList is called with more than one field")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_escaping
func TestLookupEscaping(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)
		// Underscores, percent signs and backslashes have special meaning in the
		// underlying SQL code, but the ORM handles the quoting of them
		// automatically.
		a8, a9, a10 := lkSpecials(t, db)

		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("headline__startswith", "Article"),
			lkIDs(a10, a9, a8, f.a5, f.a6, f.a4, f.a2, f.a3, f.a7, f.a1), lkID)
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("headline__startswith", "Article_"), lkIDs(a8), lkID)
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("headline__startswith", "Article%"), lkIDs(a9), lkID)
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("headline__contains", `\`), lkIDs(a10), lkID)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_exclude
func TestLookupExclude(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)
		a8, a9, a10 := lkSpecials(t, db)

		// Exclude is the opposite of Filter when doing lookups.
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("headline__contains", "Article").Exclude("headline__contains", "with"),
			lkIDs(f.a5, f.a6, f.a4, f.a2, f.a3, f.a7, f.a1), lkID)
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Exclude("headline__startswith", "Article_"),
			lkIDs(a10, a9, f.a5, f.a6, f.a4, f.a2, f.a3, f.a7, f.a1), lkID)
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Exclude("headline", "Article 7"),
			lkIDs(a10, a9, a8, f.a5, f.a6, f.a4, f.a2, f.a3, f.a1), lkID)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in
func TestLookupIn(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Exclude("id__in", []int64{}),
			lkIDs(f.a5, f.a6, f.a4, f.a2, f.a3, f.a7, f.a1), lkID)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_empty_list
func TestLookupInEmptyList(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)

		djangotest.AssertQuerySetEqual(t, lkArticles(db).Filter("id__in", []int64{}), []int64{}, lkID)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_keeps_value_ordering
//
// Django reads the SQL with the values inlined. Here the values are bound
// parameters, so the port checks their order.
func TestLookupInKeepsValueOrdering(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)
		slugs := []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7"}

		qc := djangotest.CaptureQueries(db)
		_, err := orm.Query[lkArticle](db).Filter("slug__in", slugs).Values("pk").All(context.Background())
		qc.Stop()
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		queries := qc.Queries()
		if len(queries) != 1 {
			t.Fatalf("%d queries executed, 1 expected", len(queries))
		}
		want := make([]any, len(slugs))
		for i, s := range slugs {
			want[i] = s
		}
		djangotest.AssertSequenceEqual(t, queries[0].Args, want)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_ignore_none
func TestLookupInIgnoreNone(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		qc := djangotest.CaptureQueries(db)
		djangotest.AssertQuerySetEqual(t,
			lkArticles(db).Filter("id__in", []any{nil, f.a1.ID}), lkIDs(f.a1), lkID)
		qc.Stop()
		queries := qc.Queries()
		if len(queries) != 1 {
			t.Fatalf("%d queries executed, 1 expected", len(queries))
		}
		// Only the primary key is bound: the nil is dropped from the list.
		djangotest.AssertSequenceEqual(t, queries[0].Args, []any{f.a1.ID})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_ignore_solo_none
func TestLookupInIgnoreSoloNone(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertQuerySetEqual(t, lkArticles(db).Filter("id__in", []any{nil}), []int64{}, lkID)
		})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_in_select_mismatch
func TestLookupInSelectMismatch(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)

		_, err := orm.Query[lkArticle](db).
			Filter("id__in", orm.Query[lkArticle](db).Values("id", "headline")).All(context.Background())
		assertErrorContains(t, err,
			"the QuerySet value for the 'in' lookup must have 1 selected fields (received 2)")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_error_messages
//
// The choices do not list "tag": the reverse name of a many-to-many relation
// does not exist yet.
func TestLookupErrorMessages(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		// Programming errors are pointed out with nice error messages.
		_, err := orm.Query[lkArticle](db).Filter("pub_date_year", "2005").Count(context.Background())
		assertFieldError(t, err, "cannot resolve keyword 'pub_date_year' into field. Choices are: "+
			"author, author_id, headline, id, pub_date, slug")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_unsupported_lookups
func TestLookupUnsupportedLookups(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		tests := []struct {
			key   string
			value any
			want  string
		}{
			{"headline__starts", "Article", "unsupported lookup 'starts' for CharField or join on the field " +
				"not permitted, perhaps you meant startswith or istartswith?"},
			{"pub_date__is_null", true, "unsupported lookup 'is_null' for DateTimeField or join on the field " +
				"not permitted, perhaps you meant isnull?"},
			{"pub_date__gobbledygook", "blahblah", "unsupported lookup 'gobbledygook' for DateTimeField or " +
				"join on the field not permitted."},
			{"pub_date__gt__foo", "blahblah", "unsupported lookup 'gt__foo' for DateTimeField or join on the " +
				"field not permitted, perhaps you meant gt or gte?"},
			{"pub_date__gt__", "blahblah", "unsupported lookup 'gt__' for DateTimeField or join on the field " +
				"not permitted, perhaps you meant gt or gte?"},
			{"pub_date__gt__lt", "blahblah", "unsupported lookup 'gt__lt' for DateTimeField or join on the " +
				"field not permitted, perhaps you meant gt or gte?"},
			{"pub_date__gt__lt__foo", "blahblah", "unsupported lookup 'gt__lt__foo' for DateTimeField or join" +
				" on the field not permitted, perhaps you meant gt or gte?"},
		}
		for _, tc := range tests {
			_, err := orm.Query[lkArticle](db).Filter(tc.key, tc.value).All(ctx)
			assertFieldError(t, err, tc.want)
		}
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_relation_nested_lookup_error
//
// The second assertion of the Django test filters through a many-to-many
// relation, which the ORM does not have yet.
func TestLookupRelationNestedLookupError(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		// An invalid nested lookup on a related field raises a useful error.
		_, err := orm.Query[lkArticle](db).Filter("author__editor__name", "James").All(context.Background())
		assertFieldError(t, err,
			"unsupported lookup 'editor__name' for ForeignKey or join on the field not permitted.")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_unsupported_lookup_reverse_foreign_key
func TestLookupUnsupportedLookupReverseForeignKey(t *testing.T) {
	t.Skip("filters through a reverse foreign key, which the ORM does not have yet")
}

// lkRegexSetUp replaces the articles with the first set of headlines of
// test_regex.
func lkRegexSetUp(t *testing.T, db *orm.DB, headlines ...string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	articles := make([]*lkArticle, len(headlines))
	for i, h := range headlines {
		// Django leaves the slug NULL; here it has to be unique.
		articles[i] = &lkArticle{PubDate: now, Headline: h, Slug: "regex-" + h}
	}
	if err := orm.Query[lkArticle](db).BulkCreate(context.Background(), articles); err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_regex
//
// Django compares each regex queryset with an "in" queryset of the expected
// headlines; the port lists the headlines.
func TestLookupRegex(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)
		// Create some articles with a bit more interesting headlines for testing
		// field lookups.
		if _, err := orm.Query[lkArticle](db).Delete(context.Background()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		lkRegexSetUp(t, db, "f", "fo", "foo", "fooo", "hey-Foo", "bar", "AbBa", "baz", "baxZ")
		check := func(lookup, pattern string, want ...string) {
			t.Helper()
			djangotest.AssertQuerySetEqual(t,
				orm.Query[lkArticle](db).Filter("headline__"+lookup, pattern), want, lkHeadline,
				djangotest.Unordered())
		}

		// zero-or-more
		check("regex", `fo*`, "f", "fo", "foo", "fooo")
		check("iregex", `fo*`, "f", "fo", "foo", "fooo", "hey-Foo")
		// one-or-more
		check("regex", `fo+`, "fo", "foo", "fooo")
		// wildcard
		check("regex", `fooo?`, "foo", "fooo")
		// leading anchor
		check("regex", `^b`, "bar", "baxZ", "baz")
		check("iregex", `^a`, "AbBa")
		// trailing anchor
		check("regex", `z$`, "baz")
		check("iregex", `z$`, "baxZ", "baz")
		// character sets
		check("regex", `ba[rz]`, "bar", "baz")
		check("regex", `ba.[RxZ]`, "baxZ")
		check("iregex", `ba[RxZ]`, "bar", "baxZ", "baz")

		// and more articles:
		lkRegexSetUp(t, db, "foobar", "foobaz", "ooF", "foobarbaz", "zoocarfaz", "barfoobaz", "bazbaRFOO")

		// alternation
		check("regex", `oo(f|b)`, "barfoobaz", "foobar", "foobarbaz", "foobaz")
		check("iregex", `oo(f|b)`, "barfoobaz", "foobar", "foobarbaz", "foobaz", "ooF")
		check("regex", `^foo(f|b)`, "foobar", "foobarbaz", "foobaz")

		// greedy matching
		check("regex", `b.*az`, "barfoobaz", "baz", "bazbaRFOO", "foobarbaz", "foobaz")
		check("iregex", `b.*ar`, "bar", "barfoobaz", "bazbaRFOO", "foobar", "foobarbaz")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_regex_backreferencing
//
// Django skips the test on a database without regex backreferences. SQLite
// matches with Go's regexp package here, which has none.
func TestLookupRegexBackreferencing(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "sqlite" {
			t.Skip("the regexp function of the SQLite backend has no backreferences")
		}
		// grouping and backreferences
		lkRegexSetUp(t, db, "foobar", "foobaz", "ooF", "foobarbaz", "zoocarfaz", "barfoobaz", "bazbaRFOO")

		assertFlat(t,
			orm.Query[lkArticle](db).Filter("headline__regex", `b(.).*b\1`).OrderBy("headline").ValuesList("headline"),
			[]any{"barfoobaz", "bazbaRFOO", "foobarbaz"})
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_regex_null
func TestLookupRegexNull(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		// A regex lookup does not fail on NULL values.
		s := create(t, db, &lkSeason{Year: 2012})
		lkSetNull[lkSeason](t, db, s.ID, "gt")

		djangotest.AssertEqual(t, len(mustAll(t, orm.Query[lkSeason](db).Filter("gt__regex", `^$`))), 0)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_textfield_exact_null
func TestLookupTextfieldExactNull(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		qc := djangotest.CaptureQueries(db)
		djangotest.AssertQuerySetEqual(t, lkAuthors(db).Filter("bio", nil), []int64{f.au2.ID},
			func(a lkAuthor) int64 { return a.ID })
		qc.Stop()
		queries := qc.Queries()
		if len(queries) != 1 {
			t.Fatalf("%d queries executed, 1 expected", len(queries))
		}
		// The column of an IS NULL condition is not wrapped.
		if want := db.Dialect().Quote("bio") + " IS NULL"; !strings.Contains(queries[0].SQL, want) {
			t.Errorf("SQL = %s, want it to contain %s", queries[0].SQL, want)
		}
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_regex_non_string
func TestLookupRegexNonString(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		// A regex lookup does not fail on non-string fields.
		s := create(t, db, &lkSeason{Year: 2013, Gt: 444})

		djangotest.AssertQuerySetEqual(t, orm.Query[lkSeason](db).Filter("gt__regex", `^444$`),
			[]int64{s.ID}, func(s lkSeason) int64 { return s.ID })
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_regex_non_ascii
func TestLookupRegexNonASCII(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		// A regex lookup does not trip on non-ASCII characters.
		create(t, db, &lkPlayer{Name: "♠"})

		if _, err := orm.Query[lkPlayer](db).Get(context.Background(), "name__regex", "♠"); err != nil {
			t.Errorf("Get: %v", err)
		}
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_nonfield_lookups
//
// The choices do not list "tag", as in TestLookupErrorMessages.
func TestLookupNonfieldLookups(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		// A lookup query containing non-fields raises the proper error.
		_, err := orm.Query[lkArticle](db).Filter("headline__blahblah", 99).All(ctx)
		assertFieldError(t, err,
			"unsupported lookup 'blahblah' for CharField or join on the field not permitted.")
		_, err = orm.Query[lkArticle](db).Filter("headline__blahblah__exact", 99).All(ctx)
		assertFieldError(t, err,
			"unsupported lookup 'blahblah__exact' for CharField or join on the field not permitted.")
		_, err = orm.Query[lkArticle](db).Filter("blahblah", 99).All(ctx)
		assertFieldError(t, err, "cannot resolve keyword 'blahblah' into field. Choices are: "+
			"author, author_id, headline, id, pub_date, slug")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_exact_sliced_queryset_limit_one
func TestLookupExactSlicedQuerysetLimitOne(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		djangotest.AssertQuerySetEqual(t,
			orm.Query[lkArticle](db).Filter("author", lkAuthors(db).Limit(1)),
			lkIDs(f.a1, f.a2, f.a3, f.a4), lkID, djangotest.Unordered())
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_exact_sliced_queryset_limit_one_offset
func TestLookupExactSlicedQuerysetLimitOneOffset(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)

		djangotest.AssertQuerySetEqual(t,
			orm.Query[lkArticle](db).Filter("author", lkAuthors(db).Offset(1).Limit(1)),
			lkIDs(f.a5, f.a6, f.a7), lkID, djangotest.Unordered())
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_exact_sliced_queryset_not_limited_to_one
func TestLookupExactSlicedQuerysetNotLimitedToOne(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		lkSetUp(t, db)
		ctx := context.Background()
		// A Go error string does not end in a period.
		const msg = "the QuerySet value for an exact lookup must be limited to one result using slicing"

		_, err := orm.Query[lkArticle](db).Filter("author", lkAuthors(db).Limit(2)).All(ctx)
		assertErrorContains(t, err, msg)
		_, err = orm.Query[lkArticle](db).Filter("author", lkAuthors(db).Offset(1)).All(ctx)
		assertErrorContains(t, err, msg)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_exact_query_rhs_with_selected_columns_mismatch
func TestLookupExactQueryRHSWithSelectedColumnsMismatch(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[lkAuthor](db).
			Filter("id", orm.Query[lkAuthor](db).Values("id", "name").Limit(1)).All(context.Background())
		assertErrorContains(t, err,
			"the QuerySet value for the exact lookup must have 1 selected fields (received 2)")
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_isnull_non_boolean_value
//
// The two querysets of the Django test that filter through reverse relations
// are left out.
func TestLookupIsnullNonBooleanValue(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		// A Go error string does not end in a period.
		const msg = "the QuerySet value for an isnull lookup must be True or False"

		_, err := orm.Query[lkAuthor](db).Filter("alias__isnull", 1).Exists(ctx)
		assertErrorContains(t, err, msg)
		_, err = orm.Query[lkArticle](db).Filter("author__isnull", 1).Exists(ctx)
		assertErrorContains(t, err, msg)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_isnull_textfield
func TestLookupIsnullTextfield(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)
		id := func(a lkAuthor) int64 { return a.ID }

		djangotest.AssertQuerySetEqual(t, lkAuthors(db).Filter("bio__isnull", true), []int64{f.au2.ID}, id)
		djangotest.AssertQuerySetEqual(t, lkAuthors(db).Filter("bio__isnull", false), []int64{f.au1.ID}, id)
	})
}

// Ported from Django: tests/lookup/tests.py::LookupTests.test_chain_date_time_lookups
func TestLookupChainDateTimeLookups(t *testing.T) {
	djangotest.EachDB(t, lkModels(), func(t *testing.T, db *orm.DB) {
		f := lkSetUp(t, db)
		for _, tc := range []struct {
			key   string
			value int
			want  []*lkArticle
		}{
			{"pub_date__month__gt", 7, []*lkArticle{f.a5, f.a6}},
			{"pub_date__day__gte", 27, []*lkArticle{f.a2, f.a3, f.a4, f.a7}},
			{"pub_date__hour__lt", 8, []*lkArticle{f.a1, f.a2, f.a3, f.a4, f.a7}},
			{"pub_date__minute__lte", 0, []*lkArticle{f.a1, f.a2, f.a3, f.a4, f.a5, f.a6, f.a7}},
		} {
			qs := orm.Query[lkArticle](db).Filter(tc.key, tc.value)
			djangotest.AssertQuerySetEqual(t, qs, lkIDs(tc.want...), lkID, djangotest.Unordered())
		}
	})
}
