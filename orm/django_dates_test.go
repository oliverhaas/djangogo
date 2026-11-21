package orm_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// datesArticle is Django's dates.Article.
type datesArticle struct {
	ID          int64
	Title       string `orm:"max_length=100"`
	PubDate     orm.Date
	PubDatetime time.Time
	Categories  orm.M2M[datesCategory] `orm:"related_name=articles"`
}

// datesComment is Django's dates.Comment.
type datesComment struct {
	ID           int64
	Article      orm.FK[datesArticle] `orm:"on_delete=cascade;related_name=comments"`
	Text         string               `orm:"type=text"`
	PubDate      orm.Date
	ApprovalDate *orm.Date `orm:"null"`
}

// datesCategory is Django's dates.Category.
type datesCategory struct {
	ID   int64
	Name string `orm:"max_length=255"`
}

// datesModels returns the models of Django's dates test app.
func datesModels() []any {
	return []any{djangotest.App("dates", &datesArticle{}, &datesComment{}, &datesCategory{})}
}

// assertDates fails the test unless the dates are want, in order.
func assertDates(t *testing.T, got []orm.Date, err error, want ...orm.Date) {
	t.Helper()
	if err != nil {
		t.Fatalf("Dates: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Dates = %v, want %v", got, want)
	}
}

// Ported from Django: tests/dates/tests.py::DatesTests.test_related_model_traverse
func TestDatesRelatedModelTraverse(t *testing.T) {
	djangotest.EachDB(t, datesModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a1 := create(t, db, &datesArticle{Title: "First one", PubDate: orm.NewDate(2005, 7, 28)})
		a2 := create(t, db, &datesArticle{Title: "Another one", PubDate: orm.NewDate(2010, 7, 28)})
		a3 := create(t, db, &datesArticle{Title: "Third one, in the first day", PubDate: orm.NewDate(2005, 7, 28)})
		create(t, db, &datesComment{Article: fkTo[datesArticle](a1.ID), Text: "Im the HULK!", PubDate: orm.NewDate(2005, 7, 28)})
		create(t, db, &datesComment{Article: fkTo[datesArticle](a1.ID), Text: "HULK SMASH!", PubDate: orm.NewDate(2005, 7, 29)})
		create(t, db, &datesComment{Article: fkTo[datesArticle](a2.ID), Text: "LMAO", PubDate: orm.NewDate(2010, 7, 28)})
		create(t, db, &datesComment{Article: fkTo[datesArticle](a3.ID), Text: "+1", PubDate: orm.NewDate(2005, 8, 29)})
		c := create(t, db, &datesCategory{Name: "serious-news"})
		for _, a := range []*datesArticle{a1, a3} {
			if err := orm.Many[datesCategory](db, a, "categories").Add(ctx, c); err != nil {
				t.Fatalf("add category: %v", err)
			}
		}

		comments := orm.Query[datesComment](db)
		got, err := comments.Dates(ctx, "article__pub_date", "year", "ASC")
		assertDates(t, got, err, orm.NewDate(2005, 1, 1), orm.NewDate(2010, 1, 1))
		got, err = comments.Dates(ctx, "article__pub_date", "month", "ASC")
		assertDates(t, got, err, orm.NewDate(2005, 7, 1), orm.NewDate(2010, 7, 1))
		got, err = comments.Dates(ctx, "article__pub_date", "week", "ASC")
		assertDates(t, got, err, orm.NewDate(2005, 7, 25), orm.NewDate(2010, 7, 26))
		got, err = comments.Dates(ctx, "article__pub_date", "day", "ASC")
		assertDates(t, got, err, orm.NewDate(2005, 7, 28), orm.NewDate(2010, 7, 28))
		got, err = orm.Query[datesArticle](db).Dates(ctx, "comments__pub_date", "day", "ASC")
		assertDates(t, got, err,
			orm.NewDate(2005, 7, 28), orm.NewDate(2005, 7, 29), orm.NewDate(2005, 8, 29), orm.NewDate(2010, 7, 28))
		got, err = orm.Query[datesArticle](db).Dates(ctx, "comments__approval_date", "day", "ASC")
		assertDates(t, got, err)
		got, err = orm.Query[datesCategory](db).Dates(ctx, "articles__pub_date", "day", "ASC")
		assertDates(t, got, err, orm.NewDate(2005, 7, 28))
	})
}

// No Go counterpart: tests/dates/tests.py::DatesTests.test_dates_fails_when_no_arguments_are_provided:
// the compiler rejects the call.

// Ported from Django: tests/dates/tests.py::DatesTests.test_dates_fails_when_given_invalid_field_argument
func TestDatesFailsWhenGivenInvalidFieldArgument(t *testing.T) {
	djangotest.EachDB(t, datesModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[datesArticle](db).Dates(context.Background(), "invalid_field", "year", "ASC")
		assertFieldError(t, err, "cannot resolve keyword 'invalid_field' into field. Choices are: "+
			"categories, comments, id, pub_date, pub_datetime, title")
	})
}

// Ported from Django: tests/dates/tests.py::DatesTests.test_dates_fails_when_given_invalid_kind_argument
func TestDatesFailsWhenGivenInvalidKindArgument(t *testing.T) {
	djangotest.EachDB(t, datesModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[datesArticle](db).Dates(context.Background(), "pub_date", "bad_kind", "ASC")
		assertErrorContains(t, err, "'kind' must be one of 'year', 'month', 'week', or 'day'")
	})
}

// Ported from Django: tests/dates/tests.py::DatesTests.test_dates_fails_when_given_invalid_order_argument
func TestDatesFailsWhenGivenInvalidOrderArgument(t *testing.T) {
	djangotest.EachDB(t, datesModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[datesArticle](db).Dates(context.Background(), "pub_date", "year", "bad order")
		assertErrorContains(t, err, "'order' must be either 'ASC' or 'DESC'")
	})
}

// Django runs the test with USE_TZ = False; the datetimes are in UTC, the
// time zone the ORM reads them in.
//
// Ported from Django: tests/dates/tests.py::DatesTests.test_dates_trunc_datetime_fields
func TestDatesTruncDatetimeFields(t *testing.T) {
	djangotest.EachDB(t, datesModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, pub := range []time.Time{
			time.Date(2015, 10, 21, 18, 1, 0, 0, time.UTC),
			time.Date(2015, 10, 21, 18, 2, 0, 0, time.UTC),
			time.Date(2015, 10, 22, 18, 1, 0, 0, time.UTC),
			time.Date(2015, 10, 22, 18, 2, 0, 0, time.UTC),
		} {
			create(t, db, &datesArticle{PubDate: orm.DateFromTime(pub), PubDatetime: pub})
		}
		got, err := orm.Query[datesArticle](db).Dates(ctx, "pub_datetime", "day", "ASC")
		assertDates(t, got, err, orm.NewDate(2015, 10, 21), orm.NewDate(2015, 10, 22))
	})
}

// No Go counterpart: tests/dates/tests.py::DatesTests.test_dates_avoid_datetime_cast
// checks the SQL of MySQL, which the ORM has no dialect for.
