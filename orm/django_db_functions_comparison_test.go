package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
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
	Name     string `orm:"max_length=50"`
	Age      int16  `orm:"type=positive_smallint;default=30"`
	Author   orm.FK[fnAuthor]
	FanSince *time.Time `orm:"null;blank"`
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
	return []any{djangotest.App("db_functions", &fnAuthor{}, &fnArticle{}, &fnDTModel{}, &fnFan{}, &fnFloatModel{})}
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
//
// OrderBy takes keys, not expressions, so the value is annotated and the rows
// ordered by the annotation.
func TestCoalesceOrdering(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate("display_name", orm.Coalesce("alias", "name"))
		assertFlat(t, authors.OrderBy("display_name").ValuesList("name"), []any{"Rhonda", "John Smith"})
		assertFlat(t, authors.OrderBy("-display_name").ValuesList("name"), []any{"John Smith", "Rhonda"})
	})
}

// No Go counterpart: tests/db_functions/comparison/test_coalesce.py::CoalesceTests.test_empty_queryset
// passes a queryset and a Subquery as an argument of Coalesce. The ORM has no
// subquery expressions.

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
