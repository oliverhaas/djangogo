package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/validators"
)

// fnArticle is Django's db_functions.Article without its many-to-many field
// to Author, which no ported test reads.
type fnArticle struct {
	ID        int64
	Title     string     `orm:"max_length=50"`
	Summary   *string    `orm:"max_length=200;null;blank"`
	Text      string     `orm:"type=text"`
	Written   time.Time  ``
	Published *time.Time `orm:"null;blank"`
	Updated   *time.Time `orm:"null;blank"`
	Views     int64      `orm:"type=positive_int;default=0"`
}

// fnDTModel is Django's db_functions.DTModel with the fields the ports of
// test_cast.py use.
type fnDTModel struct {
	ID            int64
	Name          string         `orm:"max_length=32"`
	StartDatetime *time.Time     `orm:"null;blank"`
	StartDate     *orm.Date      `orm:"null;blank"`
	Duration      *time.Duration `orm:"null;blank"`
}

// fnFan is Django's db_functions.Fan.
type fnFan struct {
	ID       int64
	Name     string           `orm:"max_length=50"`
	Age      int16            `orm:"type=positive_smallint;default=30"`
	Author   orm.FK[fnAuthor] `orm:"related_name=fans"`
	FanSince *time.Time       `orm:"null;blank"`
}

// fnFloatModel is Django's db_functions.FloatModel.
type fnFloatModel struct {
	ID int64
	F1 *float64 `orm:"null;blank"`
	F2 *float64 `orm:"null;blank"`
}

// fnComparisonModels returns the models of the ports of
// tests/db_functions/comparison.
func fnComparisonModels() []any {
	return []any{djangotest.App("db_functions", &fnAuthor{}, &fnArticle{}, &fnDTModel{}, &fnFan{}, &fnFloatModel{}, &fnDecimalModel{})}
}

// fnLoremIpsum is the text of Django's db_functions tests.
const fnLoremIpsum = `
    Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod
    tempor incididunt ut labore et dolore magna aliqua.`

// firstValue returns the first value of the first row of vl.
func firstValue(t *testing.T, vl *orm.ValuesListQuerySet) any {
	t.Helper()
	rows, err := vl.All(context.Background())
	if err != nil {
		t.Fatalf("ValuesList: %v", err)
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		t.Fatalf("ValuesList returned no value")
	}
	return rows[0][0]
}

// Ported from Django: tests/db_functions/comparison/test_coalesce.py::CoalesceTests.test_basic
func TestCoalesceBasic(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate("display_name", orm.Coalesce("alias", "name"))
		assertFlat(t, authors.OrderBy("name").ValuesList("display_name"), []any{"smithj", "Rhonda"})
	})
}

// Ported from Django: tests/db_functions/comparison/test_coalesce.py::CoalesceTests.test_gt_two_expressions
func TestCoalesceGtTwoExpressions(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[fnAuthor](db).Annotate("display_name", orm.Coalesce("alias")).All(context.Background())
		if err == nil || !strings.Contains(err.Error(), "Coalesce must take at least two expressions") {
			t.Fatalf("err = %v, want the Coalesce error", err)
		}
	})
}

// Ported from Django: tests/db_functions/comparison/test_coalesce.py::CoalesceTests.test_mixed_values
//
// A char and a text field are both read as strings, so no output field is
// needed.
func TestCoalesceMixedValues(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		create(t, db, &fnArticle{Title: "How to Django", Text: fnLoremIpsum, Written: time.Now()})
		article := orm.Query[fnArticle](db).Annotate("headline", orm.Coalesce("summary", "text"))
		assertFlat(t, article.OrderBy("title").ValuesList("headline"), []any{fnLoremIpsum})
		article = orm.Query[fnArticle](db).Annotate("headline", orm.Coalesce(orm.Lower("summary"), orm.Lower("text")))
		assertFlat(t, article.OrderBy("title").ValuesList("headline"), []any{strings.ToLower(fnLoremIpsum)})
	})
}

// Ported from Django: tests/db_functions/comparison/test_coalesce.py::CoalesceTests.test_ordering
func TestCoalesceOrdering(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db)
		assertFlat(t, authors.OrderBy(orm.Coalesce("alias", "name")).ValuesList("name"), []any{"Rhonda", "John Smith"})
		assertFlat(t, authors.OrderBy(orm.Coalesce("alias", "name").Asc()).ValuesList("name"), []any{"Rhonda", "John Smith"})
		assertFlat(t, authors.OrderBy(orm.Coalesce("alias", "name").Desc()).ValuesList("name"), []any{"John Smith", "Rhonda"})
	})
}

// Ported from Django: tests/db_functions/comparison/test_coalesce.py::CoalesceTests.test_empty_queryset
func TestCoalesceEmptyQueryset(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith"})
		queryset := orm.Query[fnAuthor](db).Values("id")
		tests := []struct {
			description string
			emptyQuery  any
		}{
			{"QuerySet.none()", queryset.None()},
			{"QuerySet.filter(id=0)", queryset.Filter("id", 0)},
			{"Subquery(QuerySet.none())", orm.Subquery(queryset.None())},
			{"Subquery(Queryset.filter(id=0)", orm.Subquery(queryset.Filter("id", 0))},
		}
		for _, tt := range tests {
			t.Run(tt.description, func(t *testing.T) {
				qs := orm.Query[fnAuthor](db).Annotate("annotation", orm.Coalesce(tt.emptyQuery, 42))
				djangotest.AssertNumQueries(t, db, 1, func() {
					djangotest.AssertEqual(t, firstValue(t, qs.ValuesList("annotation")), any(int64(42)))
				})
			})
		}
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_value
func TestCastFromValue(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		numbers := orm.Query[fnAuthor](db).Annotate("cast_integer", orm.Cast(orm.Value("0"), orm.Field{Kind: orm.KindInt}))
		djangotest.AssertEqual(t, firstValue(t, numbers.ValuesList("cast_integer")), any(int64(0)))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_field
func TestCastFromField(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		numbers := orm.Query[fnAuthor](db).Annotate("cast_string", orm.Cast("age", orm.Field{Kind: orm.KindChar, MaxLength: 255}))
		djangotest.AssertEqual(t, firstValue(t, numbers.ValuesList("cast_string")), any("1"))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_to_char_field_without_max_length
func TestCastToCharFieldWithoutMaxLength(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		numbers := orm.Query[fnAuthor](db).Annotate("cast_string", orm.Cast("age", orm.Field{Kind: orm.KindChar}))
		djangotest.AssertEqual(t, firstValue(t, numbers.ValuesList("cast_string")), any("1"))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_to_char_field_with_max_length
//
// Django skips the test on SQLite, which does not cut a VARCHAR.
func TestCastToCharFieldWithMaxLength(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "sqlite" {
			t.Skip("SQLite does not cut a cast to VARCHAR (supports_cast_with_precision)")
		}
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		names := orm.Query[fnAuthor](db).Annotate("cast_string", orm.Cast("name", orm.Field{Kind: orm.KindChar, MaxLength: 1}))
		djangotest.AssertEqual(t, firstValue(t, names.ValuesList("cast_string")), any("B"))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_to_decimal_field
//
// Django skips the test on SQLite, which does not round a cast to DECIMAL.
func TestCastToDecimalField(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "sqlite" {
			t.Skip("SQLite does not round a cast to DECIMAL (supports_cast_with_precision)")
		}
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		create(t, db, &fnFloatModel{F1: new(-1.934), F2: new(3.467)})
		floats := orm.Query[fnFloatModel](db).Annotate(
			"cast_f1_decimal", orm.Cast("f1", orm.Field{Kind: orm.KindDecimal, MaxDigits: 8, DecimalPlaces: 2}),
			"cast_f2_decimal", orm.Cast("f2", orm.Field{Kind: orm.KindDecimal, MaxDigits: 8, DecimalPlaces: 1}),
		)
		// PostgreSQL rounds half away from zero (rounds_to_even is false).
		assertValuesList(t, floats.ValuesList("cast_f1_decimal", "cast_f2_decimal"),
			[][]any{{orm.MustDecimal("-1.93"), orm.MustDecimal("3.5")}})
		authors := orm.Query[fnAuthor](db).Annotate(
			"cast_alias_decimal", orm.Cast("alias", orm.Field{Kind: orm.KindDecimal, MaxDigits: 8, DecimalPlaces: 2}),
		)
		got, _ := firstValue(t, authors.ValuesList("cast_alias_decimal")).(orm.Decimal)
		if got.Cmp(orm.MustDecimal("1")) != 0 {
			t.Errorf("cast_alias_decimal = %v, want 1", got)
		}
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_to_integer
func TestCastToInteger(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		for _, k := range []orm.Kind{
			orm.KindAuto, orm.KindInt, orm.KindSmallInt, orm.KindBigInt,
			orm.KindPositiveBigInt, orm.KindPositiveInt, orm.KindPositiveSmallInt,
		} {
			t.Run(k.String(), func(t *testing.T) {
				numbers := orm.Query[fnAuthor](db).Annotate("cast_int", orm.Cast("alias", orm.Field{Kind: k}))
				djangotest.AssertEqual(t, firstValue(t, numbers.ValuesList("cast_int")), any(int64(1)))
			})
		}
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_to_integer_foreign_key
//
// A foreign key casts to the integer type of the primary key it points at,
// which a Field of KindBigInt names.
func TestCastToIntegerForeignKey(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		numbers := orm.Query[fnAuthor](db).Annotate("cast_fk", orm.Cast(orm.Value("0"), orm.Field{Kind: orm.KindBigInt}))
		djangotest.AssertEqual(t, firstValue(t, numbers.ValuesList("cast_fk")), any(int64(0)))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_to_duration
func TestCastToDuration(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		duration := 24*time.Hour + 2*time.Second + 3*time.Microsecond
		create(t, db, &fnDTModel{Duration: &duration})
		dtm := orm.Query[fnDTModel](db).Annotate(
			"cast_duration", orm.Cast("duration", orm.Field{Kind: orm.KindDuration}),
			"cast_neg_duration", orm.Cast(orm.Value(-duration), orm.Field{Kind: orm.KindDuration}),
		)
		assertValuesList(t, dtm.ValuesList("cast_duration", "cast_neg_duration"), [][]any{{duration, -duration}})
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_db_datetime_to_date
func TestCastFromDBDatetimeToDate(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		dtValue := time.Date(2018, 9, 28, 12, 42, 10, 234567000, time.UTC)
		create(t, db, &fnDTModel{StartDatetime: &dtValue})
		dtm := orm.Query[fnDTModel](db).Annotate("start_datetime_as_date", orm.Cast("start_datetime", orm.Field{Kind: orm.KindDate}))
		djangotest.AssertEqual(t, firstValue(t, dtm.ValuesList("start_datetime_as_date")), any(orm.MustDate("2018-09-28")))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_db_datetime_to_time
//
// Django's SQLite cast keeps milliseconds (time_cast_precision 3); this one
// keeps the microseconds on both dialects.
func TestCastFromDBDatetimeToTime(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		dtValue := time.Date(2018, 9, 28, 12, 42, 10, 234567000, time.UTC)
		create(t, db, &fnDTModel{StartDatetime: &dtValue})
		dtm := orm.Query[fnDTModel](db).Annotate("start_datetime_as_time", orm.Cast("start_datetime", orm.Field{Kind: orm.KindTime}))
		djangotest.AssertEqual(t, firstValue(t, dtm.ValuesList("start_datetime_as_time")), any(orm.MustTime("12:42:10.234567")))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_db_date_to_datetime
func TestCastFromDBDateToDatetime(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDTModel{StartDate: new(orm.MustDate("2018-09-28"))})
		dtm := orm.Query[fnDTModel](db).Annotate("start_as_datetime", orm.Cast("start_date", orm.Field{Kind: orm.KindDateTime}))
		djangotest.AssertEqual(t, firstValue(t, dtm.ValuesList("start_as_datetime")), any(day(2018, time.September, 28)))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_db_datetime_to_date_group_by
func TestCastFromDBDatetimeToDateGroupBy(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		author := create(t, db, &fnAuthor{Name: "John Smith", Age: 45})
		dtValue := time.Date(2018, 9, 28, 12, 42, 10, 234567000, time.UTC)
		create(t, db, &fnFan{Name: "Margaret", Age: 50, Author: fkTo[fnAuthor](author.ID), FanSince: &dtValue})
		fans := orm.Query[fnFan](db).Values("author").Annotate(
			"fan_for_day", orm.Cast("fan_since", orm.Field{Kind: orm.KindDate}),
			"fans", orm.Count("*"),
		)
		assertValues(t, fans, []map[string]any{{"author": author.ID, "fan_for_day": orm.MustDate("2018-09-28"), "fans": int64(1)}})
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_python_to_date
func TestCastFromPythonToDate(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		today := orm.DateFromTime(time.Now().UTC())
		dates := orm.Query[fnAuthor](db).Annotate("cast_date", orm.Cast(today, orm.Field{Kind: orm.KindDate}))
		djangotest.AssertEqual(t, firstValue(t, dates.ValuesList("cast_date")), any(today))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_python_to_datetime
func TestCastFromPythonToDatetime(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		now := time.Now().UTC()
		dates := orm.Query[fnAuthor](db).Annotate("cast_datetime", orm.Cast(now, orm.Field{Kind: orm.KindDateTime}))
		got, _ := firstValue(t, dates.ValuesList("cast_datetime")).(time.Time)
		if d := got.Sub(now); d < -time.Microsecond || d > time.Microsecond {
			t.Errorf("cast_datetime = %v, want %v", got, now)
		}
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_from_python
func TestCastFromPython(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		numbers := orm.Query[fnAuthor](db).Annotate("cast_float", orm.Cast(orm.MustDecimal("0.125"), orm.Field{Kind: orm.KindFloat}))
		djangotest.AssertEqual(t, firstValue(t, numbers.ValuesList("cast_float")), any(0.125))
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_expression_wrapped_with_parentheses_on_postgresql
//
// The type is written in upper case.
func TestCastExpressionWrappedOnPostgreSQL(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("PostgreSQL test")
		}
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		capture := djangotest.CaptureQueries(db)
		_, err := orm.Query[fnAuthor](db).Annotate("cast_float", orm.Cast(orm.Avg("age"), orm.Field{Kind: orm.KindFloat})).
			ValuesList("cast_float").All(context.Background())
		capture.Stop()
		if err != nil {
			t.Fatalf("ValuesList: %v", err)
		}
		sql := capture.Queries()[0].SQL
		if !strings.Contains(sql, `(AVG("db_functions_fnauthor"."age"))::DOUBLE PRECISION`) {
			t.Errorf("SQL = %s, want the cast of the average", sql)
		}
	})
}

// Ported from Django: tests/db_functions/comparison/test_cast.py::CastTests.test_cast_to_text_field
func TestCastToTextField(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Bob", Age: 1, Alias: new("1")})
		authors := orm.Query[fnAuthor](db).Annotate("age_text", orm.Cast("age", orm.Field{Kind: orm.KindText}))
		assertFlat(t, authors.ValuesList("age_text"), []any{"1"})
	})
}

// fnGreatestLeastIgnoresNulls is Django's greatest_least_ignores_nulls
// feature: PostgreSQL skips a NULL argument of GREATEST and LEAST.
func fnGreatestLeastIgnoresNulls(db *orm.DB) bool { return db.Dialect().Name() == "postgres" }

// fnAssertQueryError fails t unless running vl fails with an error that
// contains want.
func fnAssertQueryError(t *testing.T, vl *orm.ValuesListQuerySet, want string) {
	t.Helper()
	_, err := vl.All(context.Background())
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want an error with %q", err, want)
	}
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_basic
func TestGreatestBasic(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		before := now.Add(-time.Hour)
		create(t, db, &fnArticle{Title: "Testing with Django", Written: before, Published: &now})
		articles := orm.Query[fnArticle](db).Annotate("last_updated", orm.Greatest("written", "published"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("last_updated")).(time.Time).Equal(now), true)
	})
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_ignores_null
func TestGreatestIgnoresNull(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		if !fnGreatestLeastIgnoresNulls(db) {
			t.Skip("SQLite does not skip NULL arguments (greatest_least_ignores_nulls)")
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		create(t, db, &fnArticle{Title: "Testing with Django", Written: now})
		articles := orm.Query[fnArticle](db).Annotate("last_updated", orm.Greatest("written", "published"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("last_updated")).(time.Time).Equal(now), true)
	})
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_propagates_null
func TestGreatestPropagatesNull(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		if fnGreatestLeastIgnoresNulls(db) {
			t.Skip("PostgreSQL skips NULL arguments (greatest_least_ignores_nulls)")
		}
		create(t, db, &fnArticle{Title: "Testing with Django", Written: time.Now()})
		articles := orm.Query[fnArticle](db).Annotate("last_updated", orm.Greatest("written", "published"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("last_updated")), nil)
	})
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_coalesce_workaround
func TestGreatestCoalesceWorkaround(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		past := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		now := time.Now().UTC().Truncate(time.Microsecond)
		create(t, db, &fnArticle{Title: "Testing with Django", Written: now})
		articles := orm.Query[fnArticle](db).Annotate("last_updated",
			orm.Greatest(orm.Coalesce("written", past), orm.Coalesce("published", past)))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("last_updated")).(time.Time).Equal(now), true)
	})
}

// No Go counterpart: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_coalesce_workaround_mysql
// runs on MySQL only.

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_all_null
func TestGreatestAllNull(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnArticle{Title: "Testing with Django", Written: time.Now()})
		articles := orm.Query[fnArticle](db).Annotate("last_updated", orm.Greatest("published", "updated"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("last_updated")), nil)
	})
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_one_expressions
//
// Django raises the error when Greatest is built; the query fails with it.
func TestGreatestOneExpressions(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		fnAssertQueryError(t, orm.Query[fnArticle](db).Annotate("g", orm.Greatest("written")).ValuesList("g"),
			"Greatest must take at least two expressions")
	})
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_related_field
func TestGreatestRelatedField(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		author := create(t, db, &fnAuthor{Name: "John Smith", Age: 45})
		create(t, db, &fnFan{Name: "Margaret", Age: 50, Author: fkTo[fnAuthor](author.ID)})
		authors := orm.Query[fnAuthor](db).Annotate("highest_age", orm.Greatest("age", "fans__age"))
		djangotest.AssertEqual(t, firstValue(t, authors.ValuesList("highest_age")), any(int64(50)))
	})
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_update
func TestGreatestUpdate(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		author := create(t, db, &fnAuthor{Name: "James Smith", GoesBy: new("Jim")})
		if _, err := orm.Query[fnAuthor](db).Update(ctx, "alias", orm.Greatest("name", "goes_by")); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := orm.Query[fnAuthor](db).RefreshFromDB(ctx, author); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		djangotest.AssertEqual(t, *author.Alias, "Jim")
	})
}

// Ported from Django: tests/db_functions/comparison/test_greatest.py::GreatestTests.test_decimal_filter
func TestGreatestDecimalFilter(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		obj := create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.1"), N2: new(orm.MustDecimal("1.2"))})
		rows := orm.Query[fnDecimalModel](db).Annotate("greatest", orm.Greatest("n1", "n2")).
			Filter("greatest", orm.MustDecimal("1.2")).ValuesList("id")
		assertFlat(t, rows, []any{obj.ID})
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_basic
func TestLeastBasic(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		before := now.Add(-time.Hour)
		create(t, db, &fnArticle{Title: "Testing with Django", Written: before, Published: &now})
		articles := orm.Query[fnArticle](db).Annotate("first_updated", orm.Least("written", "published"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("first_updated")).(time.Time).Equal(before), true)
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_ignores_null
func TestLeastIgnoresNull(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		if !fnGreatestLeastIgnoresNulls(db) {
			t.Skip("SQLite does not skip NULL arguments (greatest_least_ignores_nulls)")
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		create(t, db, &fnArticle{Title: "Testing with Django", Written: now})
		articles := orm.Query[fnArticle](db).Annotate("first_updated", orm.Least("written", "published"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("first_updated")).(time.Time).Equal(now), true)
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_propagates_null
func TestLeastPropagatesNull(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		if fnGreatestLeastIgnoresNulls(db) {
			t.Skip("PostgreSQL skips NULL arguments (greatest_least_ignores_nulls)")
		}
		create(t, db, &fnArticle{Title: "Testing with Django", Written: time.Now()})
		articles := orm.Query[fnArticle](db).Annotate("first_updated", orm.Least("written", "published"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("first_updated")), nil)
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_coalesce_workaround
func TestLeastCoalesceWorkaround(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
		now := time.Now().UTC().Truncate(time.Microsecond)
		create(t, db, &fnArticle{Title: "Testing with Django", Written: now})
		articles := orm.Query[fnArticle](db).Annotate("last_updated",
			orm.Least(orm.Coalesce("written", future), orm.Coalesce("published", future)))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("last_updated")).(time.Time).Equal(now), true)
	})
}

// No Go counterpart: tests/db_functions/comparison/test_least.py::LeastTests.test_coalesce_workaround_mysql
// runs on MySQL only.

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_all_null
func TestLeastAllNull(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnArticle{Title: "Testing with Django", Written: time.Now()})
		articles := orm.Query[fnArticle](db).Annotate("first_updated", orm.Least("published", "updated"))
		djangotest.AssertEqual(t, firstValue(t, articles.ValuesList("first_updated")), nil)
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_one_expressions
//
// Django raises the error when Least is built; the query fails with it.
func TestLeastOneExpressions(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		fnAssertQueryError(t, orm.Query[fnArticle](db).Annotate("l", orm.Least("written")).ValuesList("l"),
			"Least must take at least two expressions")
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_related_field
func TestLeastRelatedField(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		author := create(t, db, &fnAuthor{Name: "John Smith", Age: 45})
		create(t, db, &fnFan{Name: "Margaret", Age: 50, Author: fkTo[fnAuthor](author.ID)})
		authors := orm.Query[fnAuthor](db).Annotate("lowest_age", orm.Least("age", "fans__age"))
		djangotest.AssertEqual(t, firstValue(t, authors.ValuesList("lowest_age")), any(int64(45)))
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_update
func TestLeastUpdate(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		author := create(t, db, &fnAuthor{Name: "James Smith", GoesBy: new("Jim")})
		if _, err := orm.Query[fnAuthor](db).Update(ctx, "alias", orm.Least("name", "goes_by")); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := orm.Query[fnAuthor](db).RefreshFromDB(ctx, author); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		djangotest.AssertEqual(t, *author.Alias, "James Smith")
	})
}

// Ported from Django: tests/db_functions/comparison/test_least.py::LeastTests.test_decimal_filter
func TestLeastDecimalFilter(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		obj := create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.1"), N2: new(orm.MustDecimal("1.2"))})
		rows := orm.Query[fnDecimalModel](db).Annotate("least", orm.Least("n1", "n2")).
			Filter("least", orm.MustDecimal("1.1")).ValuesList("id")
		assertFlat(t, rows, []any{obj.ID})
	})
}

// Ported from Django: tests/db_functions/comparison/test_nullif.py::NullIfTests.test_basic
func TestNullIfBasic(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda", Alias: new("Rhonda")})
		authors := orm.Query[fnAuthor](db).Annotate("nullif", orm.NullIf("alias", "name"))
		assertFlat(t, authors.OrderBy("name").ValuesList("nullif"), []any{"smithj", nil})
	})
}

// Ported from Django: tests/db_functions/comparison/test_nullif.py::NullIfTests.test_null_argument
func TestNullIfNullArgument(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda", Alias: new("Rhonda")})
		authors := orm.Query[fnAuthor](db).Annotate("nullif", orm.NullIf("name", orm.Value(nil)))
		assertFlat(t, authors.OrderBy("name").ValuesList("nullif"), []any{"John Smith", "Rhonda"})
	})
}

// No Go counterpart: tests/db_functions/comparison/test_nullif.py::NullIfTests.test_too_few_args
// NullIf takes exactly two arguments, so the compiler rejects other counts.

// No Go counterpart: tests/db_functions/comparison/test_nullif.py::NullIfTests.test_null_literal
// runs on Oracle only.

// fnTestCollation returns Django's test_collations entry purpose of the
// database, or "" when the database has none.
func fnTestCollation(db *orm.DB, purpose string) string {
	if db.Dialect().Name() == "sqlite" {
		return map[string]string{"ci": "nocase", "cs": "binary", "non_default": "nocase", "virtual": "nocase"}[purpose]
	}
	return map[string]string{"deterministic": "C", "non_default": "sv-x-icu", "swedish_ci": "sv-x-icu", "virtual": "sv-x-icu"}[purpose]
}

// fnCollateAuthors creates the authors of Django's CollateTests.
func fnCollateAuthors(t *testing.T, db *orm.DB) (*fnAuthor, *fnAuthor) {
	t.Helper()
	return create(t, db, &fnAuthor{Alias: new("a"), Name: "Jones 1"}), create(t, db, &fnAuthor{Alias: new("A"), Name: "Jones 2"})
}

// Ported from Django: tests/db_functions/comparison/test_collate.py::CollateTests.test_collate_filter_ci
func TestCollateFilterCI(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		collation := fnTestCollation(db, "ci")
		if collation == "" {
			t.Skip("This backend does not support case-insensitive collations.")
		}
		fnCollateAuthors(t, db)
		n, err := orm.Query[fnAuthor](db).Filter("alias", orm.Collate(orm.Value("a"), collation)).Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(2))
	})
}

// Ported from Django: tests/db_functions/comparison/test_collate.py::CollateTests.test_collate_order_by_cs
func TestCollateOrderByCS(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		collation := fnTestCollation(db, "cs")
		if collation == "" {
			t.Skip("This backend does not support case-sensitive collations.")
		}
		author1, author2 := fnCollateAuthors(t, db)
		qs := orm.Query[fnAuthor](db).OrderBy(orm.Collate("alias", collation))
		assertFlat(t, qs.ValuesList("id"), []any{author2.ID, author1.ID})
	})
}

// Ported from Django: tests/db_functions/comparison/test_collate.py::CollateTests.test_language_collation_order_by
func TestCollateLanguageCollationOrderBy(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		collation := fnTestCollation(db, "swedish_ci")
		if collation == "" {
			t.Skip("This backend does not support language collations.")
		}
		author1, author2 := fnCollateAuthors(t, db)
		author3 := create(t, db, &fnAuthor{Alias: new("O"), Name: "Jones"})
		author4 := create(t, db, &fnAuthor{Alias: new("Ö"), Name: "Jones"})
		author5 := create(t, db, &fnAuthor{Alias: new("P"), Name: "Jones"})
		qs := orm.Query[fnAuthor](db).OrderBy(orm.Collate(orm.F("alias"), collation), "name")
		assertFlat(t, qs.ValuesList("id"), []any{author1.ID, author2.ID, author3.ID, author5.ID, author4.ID})
	})
}

// Ported from Django: tests/db_functions/comparison/test_collate.py::CollateTests.test_invalid_collation
//
// Django raises the error when Collate is built; the query fails with it. A
// Go collation is a string, so the None case has no counterpart.
func TestCollateInvalidCollation(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		for _, value := range []string{"", `et-x-icu" OR `, `"schema"."collation"`} {
			t.Run(value, func(t *testing.T) {
				qs := orm.Query[fnAuthor](db).OrderBy(orm.Collate(orm.F("alias"), value))
				fnAssertQueryError(t, qs.ValuesList("id"), "nvalid collation name: "+validators.PyRepr(value)+".")
			})
		}
	})
}
