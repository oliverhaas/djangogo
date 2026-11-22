package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// uniqueForDateModel mirrors Django's tests/validation/models.py::UniqueForDateModel.
type uniqueForDateModel struct {
	ID        int64
	StartDate orm.Date
	EndDate   time.Time
	Count     int    `orm:"unique_for_date=start_date;unique_for_year=end_date"`
	Order     int    `orm:"unique_for_month=end_date"`
	Name      string `orm:"max_length=100"`
}

// validationPost mirrors Django's tests/validation/models.py::Post.
type validationPost struct {
	ID       int64
	Title    string `orm:"max_length=50;unique_for_date=posted;blank"`
	Slug     string `orm:"max_length=50;unique_for_year=posted;blank"`
	Subtitle string `orm:"max_length=50;unique_for_month=posted;blank"`
	Posted   orm.Date
}

// flexibleDatePost mirrors Django's tests/validation/models.py::FlexibleDatePost.
type flexibleDatePost struct {
	ID       int64
	Title    string    `orm:"max_length=50;unique_for_date=posted;blank"`
	Slug     string    `orm:"max_length=50;unique_for_year=posted;blank"`
	Subtitle string    `orm:"max_length=50;unique_for_month=posted;blank"`
	Posted   *orm.Date `orm:"blank;null"`
}

// Ported from Django: tests/validation/test_unique.py::GetUniqueCheckTests.test_unique_for_date_gets_picked_up
// Ported from Django: tests/validation/test_unique.py::GetUniqueCheckTests.test_unique_for_date_exclusion
func TestGetDateChecks(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("validation", &uniqueForDateModel{}))
	m, _ := reg.ModelOf(&uniqueForDateModel{})

	djangotest.AssertEqual(t, m.UniqueChecks(), [][]string{{"id"}})
	djangotest.AssertEqual(t, m.DateChecks(), []orm.DateCheck{
		{Lookup: "date", Field: "count", DateField: "start_date"},
		{Lookup: "year", Field: "count", DateField: "end_date"},
		{Lookup: "month", Field: "order", DateField: "end_date"},
	})

	djangotest.AssertEqual(t, m.UniqueChecks("start_date"), [][]string{{"id"}})
	djangotest.AssertEqual(t, m.DateChecks("start_date"), []orm.DateCheck{
		{Lookup: "year", Field: "count", DateField: "end_date"},
		{Lookup: "month", Field: "order", DateField: "end_date"},
	})
}

// Ported from Django: tests/validation/test_unique.py::PerformUniqueChecksTest.test_unique_for_date
func TestPerformUniqueForDateChecks(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("validation", &validationPost{})}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		if err := orm.Query[validationPost](db).Create(ctx, &validationPost{
			Title:    "Django 1.0 is released",
			Slug:     "Django 1.0",
			Subtitle: "Finally",
			Posted:   orm.NewDate(2008, 9, 3),
		}); err != nil {
			t.Fatal(err)
		}
		p := &validationPost{Title: "Django 1.0 is released", Posted: orm.NewDate(2008, 9, 3)}
		assertMessageDict(t, orm.FullClean(ctx, db, p), map[string][]string{
			"title": {"Title must be unique for Posted date."},
		})

		// Should work without errors
		p = &validationPost{Title: "Work on Django 1.1 begins", Posted: orm.NewDate(2008, 9, 3)}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, p), nil)

		// Should work without errors
		p = &validationPost{Title: "Django 1.0 is released", Posted: orm.NewDate(2008, 9, 4)}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, p), nil)

		p = &validationPost{Slug: "Django 1.0", Posted: orm.NewDate(2008, 1, 1)}
		assertMessageDict(t, orm.FullClean(ctx, db, p), map[string][]string{
			"slug": {"Slug must be unique for Posted year."},
		})

		p = &validationPost{Subtitle: "Finally", Posted: orm.NewDate(2008, 9, 30)}
		assertMessageDict(t, orm.FullClean(ctx, db, p), map[string][]string{
			"subtitle": {"Subtitle must be unique for Posted month."},
		})

		p = &validationPost{Title: "Django 1.0 is released"}
		assertMessageDict(t, orm.FullClean(ctx, db, p), map[string][]string{
			"posted": {"This field cannot be null."},
		})
	})
}

// Ported from Django: tests/validation/test_unique.py::PerformUniqueChecksTest.test_unique_for_date_with_nullable_date
//
// unique_for_date/year/month checks shouldn't trigger when the associated
// DateField is None.
func TestPerformUniqueForDateChecksWithNullableDate(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("validation", &flexibleDatePost{})}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		posted := orm.NewDate(2008, 9, 3)
		if err := orm.Query[flexibleDatePost](db).Create(ctx, &flexibleDatePost{
			Title:    "Django 1.0 is released",
			Slug:     "Django 1.0",
			Subtitle: "Finally",
			Posted:   &posted,
		}); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &flexibleDatePost{Title: "Django 1.0 is released"}), nil)
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &flexibleDatePost{Slug: "Django 1.0"}), nil)
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, &flexibleDatePost{Subtitle: "Finally"}), nil)
	})
}

// No Django test exists: a row passes its own date checks when it is changed,
// and a datetime is compared by its date in UTC, the time zone of the date
// transforms.
func TestPerformUniqueForDateChecksOnDatetime(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("validation", &uniqueForDateModel{})}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		row := &uniqueForDateModel{
			StartDate: orm.NewDate(2008, 9, 3),
			EndDate:   time.Date(2008, 12, 31, 23, 30, 0, 0, time.UTC),
			Count:     1,
			Order:     2,
			Name:      "first",
		}
		if err := orm.Query[uniqueForDateModel](db).Create(ctx, row); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, row, orm.WithAdding(false)), nil)

		// 2009-01-01 00:30 at UTC+2 is 2008-12-31 22:30 in UTC: the same year and
		// month as the stored row.
		east := time.FixedZone("UTC+2", 2*60*60)
		other := &uniqueForDateModel{
			StartDate: orm.NewDate(2008, 9, 4),
			EndDate:   time.Date(2009, 1, 1, 0, 30, 0, 0, east),
			Count:     1,
			Order:     2,
			Name:      "second",
		}
		assertMessageDict(t, orm.FullClean(ctx, db, other), map[string][]string{
			"count": {"Count must be unique for End date year."},
			"order": {"Order must be unique for End date month."},
		})

		other.EndDate = time.Date(2009, 1, 1, 0, 30, 0, 0, time.UTC)
		djangotest.AssertEqual(t, orm.FullClean(ctx, db, other), nil)
	})
}

// No Django test exists: Django reports an unknown date field only when it
// validates. The ORM checks the name when the model is registered.
func TestUniqueForDateNamesDateField(t *testing.T) {
	type unknownDate struct {
		ID    int64
		Title string `orm:"max_length=50;unique_for_date=missing"`
	}
	type notADate struct {
		ID    int64
		Title string `orm:"max_length=50;unique_for_month=name"`
		Name  string `orm:"max_length=50"`
	}
	type emptyDate struct {
		ID    int64
		Title string `orm:"max_length=50;unique_for_year="`
	}
	for _, tc := range []struct {
		model any
		want  string
	}{
		{&unknownDate{}, `unique_for_date names unknown field "missing"`},
		{&notADate{}, "unique_for_month names Name, which is not a date or datetime field"},
		{&emptyDate{}, "unique_for_year must name a date field"},
	} {
		_, err := orm.NewRegistry().Register(tc.model)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Register(%T): got %v, want an error with %q", tc.model, err, tc.want)
		}
	}
}
