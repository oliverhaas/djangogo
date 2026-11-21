package orm_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of Django's tests/datetimes/tests.py read the datetimes in UTC,
// the time zone the ORM has.

// datetimesArticle is Django's datetimes.Article.
type datetimesArticle struct {
	ID          int64
	Title       string `orm:"max_length=100"`
	PubDate     time.Time
	PublishedOn *orm.Date                  `orm:"null"`
	Categories  orm.M2M[datetimesCategory] `orm:"related_name=articles"`
}

// String returns the title.
func (a datetimesArticle) String() string { return a.Title }

// datetimesComment is Django's datetimes.Comment.
type datetimesComment struct {
	ID           int64
	Article      orm.FK[datetimesArticle] `orm:"on_delete=cascade;related_name=comments"`
	Text         string                   `orm:"type=text"`
	PubDate      time.Time
	ApprovalDate *time.Time `orm:"null"`
}

// datetimesCategory is Django's datetimes.Category.
type datetimesCategory struct {
	ID   int64
	Name string `orm:"max_length=255"`
}

// datetimesModels returns the models of Django's datetimes test app.
func datetimesModels() []any {
	return []any{djangotest.App("datetimes", &datetimesArticle{}, &datetimesComment{}, &datetimesCategory{})}
}

// at returns the time of day h:m:s of the given date in UTC.
func at(year int, month time.Month, d, h, m, s int) time.Time {
	return time.Date(year, month, d, h, m, s, 0, time.UTC)
}

// assertDateTimes fails the test unless the datetimes are want, in order.
func assertDateTimes(t *testing.T, got []time.Time, err error, want ...time.Time) {
	t.Helper()
	if err != nil {
		t.Fatalf("DateTimes: %v", err)
	}
	if !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("DateTimes = %v, want %v", got, want)
	}
}

// Ported from Django: tests/datetimes/tests.py::DateTimesTests.test_related_model_traverse
func TestDateTimesRelatedModelTraverse(t *testing.T) {
	djangotest.EachDB(t, datetimesModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a1 := create(t, db, &datetimesArticle{Title: "First one", PubDate: at(2005, 7, 28, 9, 0, 0)})
		a2 := create(t, db, &datetimesArticle{Title: "Another one", PubDate: at(2010, 7, 28, 10, 0, 0)})
		a3 := create(t, db, &datetimesArticle{Title: "Third one, in the first day", PubDate: at(2005, 7, 28, 17, 0, 0)})
		create(t, db, &datetimesComment{Article: fkTo[datetimesArticle](a1.ID), Text: "Im the HULK!", PubDate: at(2005, 7, 28, 9, 30, 0)})
		create(t, db, &datetimesComment{Article: fkTo[datetimesArticle](a1.ID), Text: "HULK SMASH!", PubDate: at(2005, 7, 29, 1, 30, 0)})
		create(t, db, &datetimesComment{Article: fkTo[datetimesArticle](a2.ID), Text: "LMAO", PubDate: at(2010, 7, 28, 10, 10, 10)})
		create(t, db, &datetimesComment{Article: fkTo[datetimesArticle](a3.ID), Text: "+1", PubDate: at(2005, 8, 29, 10, 10, 10)})
		c := create(t, db, &datetimesCategory{Name: "serious-news"})
		for _, a := range []*datetimesArticle{a1, a3} {
			if err := orm.Many[datetimesCategory](db, a, "categories").Add(ctx, c); err != nil {
				t.Fatalf("add category: %v", err)
			}
		}

		comments := orm.Query[datetimesComment](db)
		got, err := comments.DateTimes(ctx, "article__pub_date", "year", "ASC")
		assertDateTimes(t, got, err, day(2005, 1, 1), day(2010, 1, 1))
		got, err = comments.DateTimes(ctx, "article__pub_date", "month", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 1), day(2010, 7, 1))
		got, err = comments.DateTimes(ctx, "article__pub_date", "week", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 25), day(2010, 7, 26))
		got, err = comments.DateTimes(ctx, "article__pub_date", "day", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 28), day(2010, 7, 28))
		got, err = orm.Query[datetimesArticle](db).DateTimes(ctx, "comments__pub_date", "day", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 28), day(2005, 7, 29), day(2005, 8, 29), day(2010, 7, 28))
		got, err = orm.Query[datetimesArticle](db).DateTimes(ctx, "comments__approval_date", "day", "ASC")
		assertDateTimes(t, got, err)
		got, err = orm.Query[datetimesCategory](db).DateTimes(ctx, "articles__pub_date", "day", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 28))
	})
}

// Ported from Django: tests/datetimes/tests.py::DateTimesTests.test_21432
func TestDateTimes21432(t *testing.T) {
	djangotest.EachDB(t, datetimesModels(), func(t *testing.T, db *orm.DB) {
		now := time.Now().UTC().Truncate(time.Second)
		create(t, db, &datetimesArticle{Title: "First one", PubDate: now})
		got, err := orm.Query[datetimesArticle](db).DateTimes(context.Background(), "pub_date", "second", "ASC")
		assertDateTimes(t, got, err, now)
	})
}

// Ported from Django: tests/datetimes/tests.py::DateTimesTests.test_datetimes_returns_available_dates_for_given_scope_and_given_field
func TestDateTimesReturnsAvailableDatesForGivenScopeAndGivenField(t *testing.T) {
	djangotest.EachDB(t, datetimesModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for i, pub := range []time.Time{
			at(2005, 7, 28, 12, 15, 0),
			at(2005, 7, 29, 2, 15, 0),
			at(2005, 7, 30, 5, 15, 0),
			at(2005, 7, 31, 19, 15, 0),
		} {
			create(t, db, &datetimesArticle{PubDate: pub, Title: fmt.Sprintf("title #%d", i)})
		}
		articles := orm.Query[datetimesArticle](db)
		got, err := articles.DateTimes(ctx, "pub_date", "year", "ASC")
		assertDateTimes(t, got, err, day(2005, 1, 1))
		got, err = articles.DateTimes(ctx, "pub_date", "month", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 1))
		got, err = articles.DateTimes(ctx, "pub_date", "week", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 25))
		got, err = articles.DateTimes(ctx, "pub_date", "day", "ASC")
		assertDateTimes(t, got, err, day(2005, 7, 28), day(2005, 7, 29), day(2005, 7, 30), day(2005, 7, 31))
		got, err = articles.DateTimes(ctx, "pub_date", "day", "DESC")
		assertDateTimes(t, got, err, day(2005, 7, 31), day(2005, 7, 30), day(2005, 7, 29), day(2005, 7, 28))
	})
}

// No Go counterpart: tests/datetimes/tests.py::DateTimesTests.test_datetimes_has_lazy_iterator:
// DateTimes runs its query when it is called and returns a slice.

// Ported from Django: tests/datetimes/tests.py::DateTimesTests.test_datetimes_disallows_date_fields
func TestDateTimesDisallowsDateFields(t *testing.T) {
	djangotest.EachDB(t, datetimesModels(), func(t *testing.T, db *orm.DB) {
		dt := at(2005, 7, 28, 12, 15, 0)
		on := orm.DateFromTime(dt)
		create(t, db, &datetimesArticle{PubDate: dt, PublishedOn: &on, Title: "Don't put dates into datetime functions!"})
		_, err := orm.Query[datetimesArticle](db).DateTimes(context.Background(), "published_on", "second", "ASC")
		assertErrorContains(t, err, "Cannot truncate DateField 'published_on' to DateTimeField")
	})
}

// Ported from Django: tests/datetimes/tests.py::DateTimesTests.test_datetimes_fails_when_given_invalid_kind_argument
func TestDateTimesFailsWhenGivenInvalidKindArgument(t *testing.T) {
	djangotest.EachDB(t, datetimesModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[datetimesArticle](db).DateTimes(context.Background(), "pub_date", "bad_kind", "ASC")
		assertErrorContains(t, err,
			"'kind' must be one of 'year', 'month', 'week', 'day', 'hour', 'minute', or 'second'")
	})
}

// Ported from Django: tests/datetimes/tests.py::DateTimesTests.test_datetimes_fails_when_given_invalid_order_argument
func TestDateTimesFailsWhenGivenInvalidOrderArgument(t *testing.T) {
	djangotest.EachDB(t, datetimesModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[datetimesArticle](db).DateTimes(context.Background(), "pub_date", "year", "bad order")
		assertErrorContains(t, err, "'order' must be either 'ASC' or 'DESC'")
	})
}
