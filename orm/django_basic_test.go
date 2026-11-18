package orm_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// basArticle is Django's basic.Article, without its Meta.ordering.
type basArticle struct {
	ID       int64
	Headline string `orm:"max_length=100;default=Default headline"`
	PubDate  time.Time
}

// basModels returns the models of Django's basic test app.
func basModels() []any {
	return []any{djangotest.App("basic", &basArticle{})}
}

// basSave saves an article and fails the test on an error.
func basSave(t *testing.T, db *orm.DB, a *basArticle) {
	t.Helper()
	if err := orm.Query[basArticle](db).Save(context.Background(), a); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// basGet returns the one article the arguments match.
func basGet(t *testing.T, db *orm.DB, args ...any) basArticle {
	t.Helper()
	a, err := orm.Query[basArticle](db).Get(context.Background(), args...)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return a
}

// basAssertSame checks that two articles are the same row with the same
// values. Times are compared as instants.
func basAssertSame(t *testing.T, got basArticle, want *basArticle) {
	t.Helper()
	if got.ID != want.ID || got.Headline != want.Headline || !got.PubDate.Equal(want.PubDate) {
		t.Errorf("got %+v, want %+v", got, *want)
	}
}

// basSetUpLookup creates the article of Django's ModelLookupTest.
func basSetUpLookup(t *testing.T, db *orm.DB) *basArticle {
	t.Helper()
	a := &basArticle{Headline: "Swallow programs in Python", PubDate: day(2005, 7, 28)}
	basSave(t, db, a)
	return a
}

// Ported from Django: tests/basic/tests.py::ModelInstanceCreationTests.test_object_is_not_written_to_database_until_save_was_called
func TestBasicObjectIsNotWrittenToDatabaseUntilSaveWasCalled(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := &basArticle{Headline: "Parrot programs in Python", PubDate: day(2005, 7, 28)}
		djangotest.AssertEqual(t, a.ID, int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[basArticle](db)), int64(0))
		basSave(t, db, a)
		if a.ID == 0 {
			t.Error("Save did not set the primary key")
		}
		djangotest.AssertEqual(t, mustCount(t, orm.Query[basArticle](db)), int64(1))
	})
}

// Ported from Django: tests/basic/tests.py::ModelInstanceCreationTests.test_autofields_generate_different_values_for_each_instance
func TestBasicAutofieldsGenerateDifferentValuesForEachInstance(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a1 := create(t, db, &basArticle{Headline: "First", PubDate: day(2005, 7, 30)})
		a2 := create(t, db, &basArticle{Headline: "First", PubDate: day(2005, 7, 30)})
		a3 := create(t, db, &basArticle{Headline: "First", PubDate: day(2005, 7, 30)})
		if a3.ID == a1.ID || a3.ID == a2.ID {
			t.Errorf("ids = %d, %d, %d, want three different ones", a1.ID, a2.ID, a3.ID)
		}
	})
}

// Ported from Django: tests/basic/tests.py::ModelInstanceCreationTests.test_can_leave_off_value_for_autofield_and_it_gets_value_on_save
func TestBasicCanLeaveOffValueForAutofieldAndItGetsValueOnSave(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := &basArticle{Headline: "Article 5", PubDate: day(2005, 7, 31)}
		basSave(t, db, a)
		djangotest.AssertEqual(t, a.Headline, "Article 5")
		if a.ID == 0 {
			t.Error("Save did not set the primary key")
		}
	})
}

// Ported from Django: tests/basic/tests.py::ModelInstanceCreationTests.test_leaving_off_a_field_with_default_set_the_default_will_be_saved
func TestBasicLeavingOffAFieldWithDefaultSetTheDefaultWillBeSaved(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := &basArticle{PubDate: day(2005, 7, 31)}
		basSave(t, db, a)
		djangotest.AssertEqual(t, a.Headline, "Default headline")
		djangotest.AssertEqual(t, basGet(t, db, "pk", a.ID).Headline, "Default headline")
	})
}

// Ported from Django: tests/basic/tests.py::ModelInstanceCreationTests.test_for_datetimefields_saves_as_much_precision_as_was_given
func TestBasicForDatetimefieldsSavesAsMuchPrecisionAsWasGiven(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		for _, pubDate := range []time.Time{
			time.Date(2005, 7, 31, 12, 30, 0, 0, time.UTC),
			time.Date(2005, 7, 31, 12, 30, 45, 0, time.UTC),
		} {
			a := &basArticle{Headline: "Article", PubDate: pubDate}
			basSave(t, db, a)
			if got := basGet(t, db, "id__exact", a.ID).PubDate; !got.Equal(pubDate) {
				t.Errorf("pub_date = %v, want %v", got, pubDate)
			}
		}
	})
}

// Ported from Django: tests/basic/tests.py::ModelInstanceCreationTests.test_saving_an_object_again_does_not_create_a_new_object
func TestBasicSavingAnObjectAgainDoesNotCreateANewObject(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := &basArticle{Headline: "original", PubDate: day(2014, 5, 16)}
		basSave(t, db, a)
		currentID := a.ID

		basSave(t, db, a)
		djangotest.AssertEqual(t, a.ID, currentID)

		a.Headline = "Updated headline"
		basSave(t, db, a)
		djangotest.AssertEqual(t, a.ID, currentID)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[basArticle](db)), int64(1))
		djangotest.AssertEqual(t, basGet(t, db).Headline, "Updated headline")
	})
}

// Ported from Django: tests/basic/tests.py::ModelInstanceCreationTests.test_querysets_checking_for_membership
func TestBasicQuerysetsCheckingForMembership(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		pubDate := time.Date(2014, 5, 16, 12, 1, 0, 0, time.UTC)
		for _, headline := range []string{"Parrot programs in Python", "Second article", "Third article"} {
			basSave(t, db, &basArticle{Headline: headline, PubDate: pubDate})
		}
		a := &basArticle{Headline: "Some headline", PubDate: pubDate}
		basSave(t, db, a)

		found := false
		for _, row := range mustAll(t, orm.Query[basArticle](db)) {
			found = found || row.ID == a.ID
		}
		djangotest.AssertEqual(t, found, true)
		exists, err := orm.Query[basArticle](db).Filter("id", a.ID).Exists(context.Background())
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		djangotest.AssertEqual(t, exists, true)
	})
}

// Ported from Django: tests/basic/tests.py::ModelTest.test_queryset_delete_removes_all_items_in_that_queryset
func TestBasicQuerysetDeleteRemovesAllItemsInThatQueryset(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		pubDate := time.Date(2014, 5, 16, 12, 1, 0, 0, time.UTC)
		for _, headline := range []string{"An article", "Article One", "Amazing article", "Boring article"} {
			basSave(t, db, &basArticle{Headline: headline, PubDate: pubDate})
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[basArticle](db).OrderBy("headline"),
			[]string{"Amazing article", "An article", "Article One", "Boring article"},
			func(a basArticle) string { return a.Headline })
		if _, err := orm.Query[basArticle](db).Filter("headline__startswith", "A").Delete(context.Background()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		djangotest.AssertEqual(t, basGet(t, db).Headline, "Boring article")
	})
}

// Ported from Django: tests/basic/tests.py::ModelTest.test_microsecond_precision
func TestBasicMicrosecondPrecision(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		pubDate := time.Date(2005, 7, 31, 12, 30, 45, 180*int(time.Microsecond), time.UTC)
		a9 := &basArticle{Headline: "Article 9", PubDate: pubDate}
		basSave(t, db, a9)
		if got := basGet(t, db, "pk", a9.ID).PubDate; !got.Equal(pubDate) {
			t.Errorf("pub_date = %v, want %v", got, pubDate)
		}
	})
}

// Ported from Django: tests/basic/tests.py::ModelTest.test_manually_specify_primary_key
func TestBasicManuallySpecifyPrimaryKey(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a101 := &basArticle{ID: 101, Headline: "Article 101", PubDate: time.Date(2005, 7, 31, 12, 30, 45, 0, time.UTC)}
		basSave(t, db, a101)
		djangotest.AssertEqual(t, basGet(t, db, "pk", 101).Headline, "Article 101")
	})
}

// Ported from Django: tests/basic/tests.py::ModelTest.test_create_method
func TestBasicCreateMethod(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a10 := create(t, db, &basArticle{Headline: "Article 10", PubDate: time.Date(2005, 7, 31, 12, 30, 45, 0, time.UTC)})
		basAssertSame(t, basGet(t, db, "headline", "Article 10"), a10)
	})
}

// Ported from Django: tests/basic/tests.py::ModelTest.test_unicode_data
func TestBasicUnicodeData(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := &basArticle{Headline: "林原 めぐみ", PubDate: day(2005, 7, 28)}
		basSave(t, db, a)
		djangotest.AssertEqual(t, basGet(t, db, "pk", a.ID).Headline, "林原 めぐみ")
	})
}

// Ported from Django: tests/basic/tests.py::ModelTest.test_multiple_objects_max_num_fetched
func TestBasicMultipleObjectsMaxNumFetched(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		// One row fewer than Django's MAX_GET_RESULTS.
		const maxResults = 20
		rows := make([]*basArticle, maxResults)
		for i := range rows {
			rows[i] = &basArticle{Headline: fmt.Sprintf("Area %d", i), PubDate: day(2005, 7, 28)}
		}
		if err := orm.Query[basArticle](db).BulkCreate(ctx, rows); err != nil {
			t.Fatalf("BulkCreate: %v", err)
		}
		_, err := orm.Query[basArticle](db).Get(ctx, "headline__startswith", "Area")
		if !errors.Is(err, orm.ErrMultipleObjectsReturned) {
			t.Fatalf("error = %v, want ErrMultipleObjectsReturned", err)
		}
		assertErrorContains(t, err, "get() returned more than one basArticle -- it returned 20!")

		create(t, db, &basArticle{Headline: "Area 20", PubDate: day(2005, 7, 28)})
		_, err = orm.Query[basArticle](db).Get(ctx, "headline__startswith", "Area")
		if !errors.Is(err, orm.ErrMultipleObjectsReturned) {
			t.Fatalf("error = %v, want ErrMultipleObjectsReturned", err)
		}
		assertErrorContains(t, err, "get() returned more than one basArticle -- it returned more than 20!")
	})
}

// Ported from Django: tests/basic/tests.py::ModelLookupTest.test_all_lookup
func TestBasicAllLookup(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := basSetUpLookup(t, db)
		a.Headline = "Parrot programs in Python"
		basSave(t, db, a)
		rows := mustAll(t, orm.Query[basArticle](db))
		if len(rows) != 1 {
			t.Fatalf("rows = %+v, want one", rows)
		}
		basAssertSame(t, rows[0], a)
	})
}

// The date part lookups (year, month, day, week_day) of the Django test are
// transforms, which do not exist here.
//
// Ported from Django: tests/basic/tests.py::ModelLookupTest.test_rich_lookup
func TestBasicRichLookup(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := basSetUpLookup(t, db)
		basAssertSame(t, basGet(t, db, "id__exact", a.ID), a)
		basAssertSame(t, basGet(t, db, "headline__startswith", "Swallow"), a)
		basAssertSame(t, basGet(t, db, "pub_date__range", []time.Time{day(2005, 1, 1), day(2005, 12, 31)}), a)
	})
}

// The date part lookups of the Django test are left out, see above.
//
// Ported from Django: tests/basic/tests.py::ModelLookupTest.test_equal_lookup
func TestBasicEqualLookup(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := basSetUpLookup(t, db)
		basAssertSame(t, basGet(t, db, "id", a.ID), a)
		basAssertSame(t, basGet(t, db, "headline", "Swallow programs in Python"), a)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[basArticle](db).Filter("pub_date", day(2005, 7, 28))), int64(1))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[basArticle](db).Filter("pub_date", day(2004, 7, 28))), int64(0))
	})
}

// The date part lookups of the Django test are left out, see above.
//
// Ported from Django: tests/basic/tests.py::ModelLookupTest.test_does_not_exist
func TestBasicDoesNotExist(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		basSetUpLookup(t, db)
		ctx := context.Background()
		_, err := orm.Query[basArticle](db).Get(ctx, "id__exact", 2000)
		if !errors.Is(err, orm.ErrDoesNotExist) {
			t.Fatalf("error = %v, want ErrDoesNotExist", err)
		}
		assertErrorContains(t, err, "basArticle matching query does not exist.")
		_, err = orm.Query[basArticle](db).Get(ctx, "headline__startswith", "Swallow", "pub_date", day(2005, 8, 28))
		if !errors.Is(err, orm.ErrDoesNotExist) {
			t.Errorf("error = %v, want ErrDoesNotExist", err)
		}
	})
}

// Ported from Django: tests/basic/tests.py::ModelLookupTest.test_lookup_by_primary_key
func TestBasicLookupByPrimaryKey(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := basSetUpLookup(t, db)
		basAssertSame(t, basGet(t, db, "pk", a.ID), a)
		rows := mustAll(t, orm.Query[basArticle](db).Filter("pk__in", []int64{a.ID}))
		if len(rows) != 1 {
			t.Fatalf("rows = %+v, want one", rows)
		}
		basAssertSame(t, rows[0], a)
		first, second := basGet(t, db, "pk", a.ID), basGet(t, db, "pk", a.ID)
		basAssertSame(t, first, &second)
	})
}

// The date part lookups of the Django test are left out, see above.
//
// Ported from Django: tests/basic/tests.py::ModelLookupTest.test_too_many
func TestBasicTooMany(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		basSetUpLookup(t, db)
		basSave(t, db, &basArticle{Headline: "Swallow bites Python", PubDate: day(2005, 7, 28)})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[basArticle](db)), int64(2))

		ctx := context.Background()
		const msg = "get() returned more than one basArticle -- it returned 2!"
		for _, args := range [][]any{
			{"headline__startswith", "Swallow"},
			{"pub_date", day(2005, 7, 28)},
			{"pub_date", day(2005, 7, 28), "headline__startswith", "Swallow"},
		} {
			_, err := orm.Query[basArticle](db).Get(ctx, args...)
			if !errors.Is(err, orm.ErrMultipleObjectsReturned) {
				t.Errorf("Get(%v): error = %v, want ErrMultipleObjectsReturned", args, err)
				continue
			}
			assertErrorContains(t, err, msg)
		}
	})
}

// Django deletes the row on a second connection of a thread. The outcome is
// the same without one: saving an object whose row is gone inserts it again.
//
// Ported from Django: tests/basic/tests.py::ConcurrentSaveTests.test_concurrent_delete_with_save
func TestBasicConcurrentDeleteWithSave(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		a := create(t, db, &basArticle{Headline: "foo", PubDate: day(2020, 1, 1)})
		if _, err := orm.Query[basArticle](db).Filter("pk", a.ID).Delete(context.Background()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		a.Headline = "foo"
		basSave(t, db, a)
		djangotest.AssertEqual(t, basGet(t, db, "pk", a.ID).Headline, "foo")
	})
}

// A slice of the caller bounds the rows Get looks at, so the count in the
// message is exact.
func TestGetOnSlicedQuerysetCountsItsRows(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		for i := range 5 {
			create(t, db, &basArticle{Headline: fmt.Sprintf("Area %d", i), PubDate: day(2005, 7, 28)})
		}
		ctx := context.Background()
		_, err := orm.Query[basArticle](db).OrderBy("headline").Limit(3).Get(ctx)
		assertErrorContains(t, err, "it returned 3!")
		got, err := orm.Query[basArticle](db).OrderBy("-headline").Limit(1).Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.Headline, "Area 4")
	})
}

// First and Last report an empty queryset with the message of Get.
func TestFirstAndLastOnEmptyQueryset(t *testing.T) {
	djangotest.EachDB(t, basModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := orm.Query[basArticle](db).First(ctx)
		if !errors.Is(err, orm.ErrDoesNotExist) {
			t.Errorf("First: error = %v, want ErrDoesNotExist", err)
		}
		_, err = orm.Query[basArticle](db).Last(ctx)
		if !errors.Is(err, orm.ErrDoesNotExist) {
			t.Errorf("Last: error = %v, want ErrDoesNotExist", err)
		}
		assertErrorContains(t, err, "basArticle matching query does not exist.")
	})
}
