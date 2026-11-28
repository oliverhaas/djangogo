package orm_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of Django's tests/db_functions/datetime/test_extract_trunc.py run
// the DateFunctionWithTimeZoneTests variant: USE_TZ = True in UTC, the only
// time zone the ORM has. Django's Extract and Trunc functions have no Go form;
// F with a transform ("start_datetime__year") builds the same SQL, which is
// what Extract("start_datetime", "year") and TruncDate("start_datetime")
// build. The asserts on the errors of those functions are left out.

// dtModel is Django's db_functions.DTModel.
type dtModel struct {
	ID            int64
	Name          string         `orm:"max_length=32"`
	StartDatetime *time.Time     `orm:"null;blank"`
	EndDatetime   *time.Time     `orm:"null;blank"`
	StartDate     *orm.Date      `orm:"null;blank"`
	EndDate       *orm.Date      `orm:"null;blank"`
	StartTime     *orm.Time      `orm:"null;blank"`
	EndTime       *orm.Time      `orm:"null;blank"`
	Duration      *time.Duration `orm:"null;blank"`
}

// dtModels returns the models of Django's db_functions test app that the
// ports use.
func dtModels() []any { return []any{djangotest.App("db_functions", &dtModel{})} }

// dtCreate is Django's create_model: a row with the two datetimes and their
// dates, times, and difference. A zero time is Django's None.
func dtCreate(t *testing.T, db *orm.DB, start, end time.Time) *dtModel {
	t.Helper()
	m := &dtModel{Name: "None"}
	if !start.IsZero() {
		d, tod := orm.DateFromTime(start), orm.TimeFromTime(start)
		m.Name, m.StartDatetime, m.StartDate, m.StartTime = start.Format(time.RFC3339Nano), &start, &d, &tod
	}
	if !end.IsZero() {
		d, tod := orm.DateFromTime(end), orm.TimeFromTime(end)
		m.EndDatetime, m.EndDate, m.EndTime = &end, &d, &tod
	}
	if !start.IsZero() && !end.IsZero() {
		dur := end.Sub(start)
		m.Duration = &dur
	}
	return create(t, db, m)
}

// dtSQL returns the lowercased SELECT of qs, Django's str(qs.query).lower().
func dtSQL[T any](t *testing.T, qs *orm.QuerySet[T]) string {
	t.Helper()
	sql, _, err := orm.CompileSelectForTest(qs)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return strings.ToLower(sql)
}

// dtIDs returns the primary key of a row.
func dtIDs(m dtModel) int64 { return m.ID }

// dtExtracted asserts the (start_datetime, extracted) pairs of the rows of qs
// ordered by start_datetime, Django's assertQuerySetEqual with the lambda
// m: (m.start_datetime, m.extracted).
func dtExtracted(t *testing.T, qs *orm.QuerySet[dtModel], want ...[2]any) {
	t.Helper()
	rows := make([]map[string]any, 0, len(want))
	for _, w := range want {
		rows = append(rows, map[string]any{"start_datetime": w[0], "extracted": w[1]})
	}
	assertValues(t, qs.OrderBy("start_datetime").Values("start_datetime", "extracted"), rows)
}

// dtStartEnd returns the datetimes the extract and trunc tests use.
func dtStartEnd() (start, end time.Time) {
	return time.Date(2015, 6, 15, 14, 30, 50, 321000, time.UTC),
		time.Date(2016, 6, 15, 14, 10, 50, 123000, time.UTC)
}

// dtYearRows creates the two rows of the year lookup tests.
func dtYearRows(t *testing.T, db *orm.DB) {
	t.Helper()
	start := time.Date(2015, 6, 15, 14, 10, 0, 0, time.UTC)
	end := time.Date(2016, 6, 15, 14, 10, 0, 0, time.UTC)
	dtCreate(t, db, start, end)
	dtCreate(t, db, end, start)
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_year_exact_lookup
func TestDBFunctionsExtractYearExactLookup(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtYearRows(t, db)
		for _, lookup := range []string{"year", "iso_year"} {
			t.Run(lookup, func(t *testing.T) {
				for _, key := range []string{
					"start_datetime__" + lookup + "__exact",
					// exact is implied and should be the same
					"start_datetime__" + lookup,
					// date and datetime fields should behave the same
					"start_date__" + lookup,
				} {
					qs := orm.Query[dtModel](db).Filter(key, 2015)
					if n := mustCount(t, qs); n != 1 {
						t.Fatalf("%s: count = %d, want 1", key, n)
					}
					sql := dtSQL(t, qs)
					if got := strings.Count(sql, " between "); got != 1 {
						t.Fatalf("%s: %d BETWEEN in %s, want 1", key, got, sql)
					}
					if got := strings.Count(sql, "extract"); got != 0 {
						t.Fatalf("%s: %d extract in %s, want 0", key, got, sql)
					}
				}
				// an expression rhs cannot use the between optimization.
				qs := orm.Query[dtModel](db).
					Annotate("start_year", orm.F("start_datetime__year")).
					Filter("end_datetime__year", orm.F("start_year").Add(1))
				if n := mustCount(t, qs); n != 1 {
					t.Fatalf("expression rhs: count = %d, want 1", n)
				}
				sql := dtSQL(t, qs)
				if got := strings.Count(sql, " between "); got != 0 {
					t.Fatalf("expression rhs: %d BETWEEN in %s, want 0", got, sql)
				}
				if got := strings.Count(sql, "extract"); got != 3 {
					t.Fatalf("expression rhs: %d extract in %s, want 3", got, sql)
				}
			})
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_year_greaterthan_lookup
func TestDBFunctionsExtractYearGreaterThanLookup(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtYearRows(t, db)
		for _, lookup := range []string{"year", "iso_year"} {
			t.Run(lookup, func(t *testing.T) {
				for _, tc := range []struct {
					key  string
					want int64
				}{
					{"start_datetime__" + lookup + "__gt", 1},
					{"start_datetime__" + lookup + "__gte", 2},
				} {
					qs := orm.Query[dtModel](db).Filter(tc.key, 2015)
					if n := mustCount(t, qs); n != tc.want {
						t.Fatalf("%s: count = %d, want %d", tc.key, n, tc.want)
					}
					if sql := dtSQL(t, qs); strings.Contains(sql, "extract") {
						t.Fatalf("%s: extract in %s", tc.key, sql)
					}
				}
				qs := orm.Query[dtModel](db).
					Annotate("start_year", orm.F("start_datetime__year")).
					Filter("end_datetime__"+lookup+"__gte", orm.F("start_year"))
				if n := mustCount(t, qs); n != 1 {
					t.Fatalf("expression rhs: count = %d, want 1", n)
				}
				if sql := dtSQL(t, qs); strings.Count(sql, "extract") < 2 {
					t.Fatalf("expression rhs: fewer than 2 extract in %s", sql)
				}
			})
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_year_lessthan_lookup
func TestDBFunctionsExtractYearLessThanLookup(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtYearRows(t, db)
		for _, lookup := range []string{"year", "iso_year"} {
			t.Run(lookup, func(t *testing.T) {
				for _, tc := range []struct {
					key  string
					want int64
				}{
					{"start_datetime__" + lookup + "__lt", 1},
					{"start_datetime__" + lookup + "__lte", 2},
				} {
					qs := orm.Query[dtModel](db).Filter(tc.key, 2016)
					if n := mustCount(t, qs); n != tc.want {
						t.Fatalf("%s: count = %d, want %d", tc.key, n, tc.want)
					}
					if sql := dtSQL(t, qs); strings.Contains(sql, "extract") {
						t.Fatalf("%s: extract in %s", tc.key, sql)
					}
				}
				qs := orm.Query[dtModel](db).
					Annotate("end_year", orm.F("end_datetime__year")).
					Filter("start_datetime__"+lookup+"__lte", orm.F("end_year"))
				if n := mustCount(t, qs); n != 1 {
					t.Fatalf("expression rhs: count = %d, want 1", n)
				}
				if sql := dtSQL(t, qs); strings.Count(sql, "extract") < 2 {
					t.Fatalf("expression rhs: fewer than 2 extract in %s", sql)
				}
			})
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_func
func TestDBFunctionsExtractFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		for _, tc := range []struct {
			name       string
			start, end int64
		}{
			{"year", 2015, 2016},
			{"quarter", 2, 2},
			{"month", 6, 6},
			{"day", 15, 15},
			{"week", 25, 24},
			// (isoweekday() % 7) + 1 of a Monday and a Wednesday.
			{"week_day", 2, 4},
			{"iso_week_day", 1, 3},
			{"hour", 14, 14},
			{"minute", 30, 10},
			{"second", 50, 50},
		} {
			t.Run(tc.name, func(t *testing.T) {
				qs := orm.Query[dtModel](db).Annotate("extracted", orm.F("start_datetime__"+tc.name))
				dtExtracted(t, qs, [2]any{start, tc.start}, [2]any{end, tc.end})
			})
		}
		for _, key := range []string{
			"start_datetime__year", "start_datetime__hour", "start_date__month", "start_time__hour",
		} {
			qs := orm.Query[dtModel](db).Filter(key, orm.F(key))
			if n := mustCount(t, qs); n != 2 {
				t.Fatalf("%s=F(%s): count = %d, want 2", key, key, n)
			}
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_none
func TestDBFunctionsExtractNone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtCreate(t, db, time.Time{}, time.Time{})
		for _, key := range []string{"start_datetime__year", "start_date__year", "start_time__hour"} {
			t.Run(key, func(t *testing.T) {
				qs := orm.Query[dtModel](db).Annotate("extracted", orm.F(key)).Values("extracted")
				assertValues(t, qs, []map[string]any{{"extracted": nil}})
			})
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_iso_year_func_boundaries
func TestDBFunctionsExtractIsoYearFuncBoundaries(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		end := time.Date(2016, 6, 15, 14, 10, 50, 123000, time.UTC)
		week52Day2014 := time.Date(2014, 12, 27, 13, 0, 0, 0, time.UTC)    // Sunday
		week1Day20142015 := time.Date(2014, 12, 31, 13, 0, 0, 0, time.UTC) // Wednesday
		week53Day2015 := time.Date(2015, 12, 31, 13, 0, 0, 0, time.UTC)    // Thursday
		days := []time.Time{week52Day2014, week1Day20142015, week53Day2015}
		obj1Iso2014 := dtCreate(t, db, week52Day2014, end)
		obj1Iso2015 := dtCreate(t, db, week1Day20142015, end)
		obj2Iso2015 := dtCreate(t, db, week53Day2015, end)
		qs := orm.Query[dtModel](db).
			Filter("start_datetime__in", days).
			Annotate("extracted", orm.F("start_datetime__iso_year"))
		dtExtracted(t, qs,
			[2]any{week52Day2014, int64(2014)},
			[2]any{week1Day20142015, int64(2015)},
			[2]any{week53Day2015, int64(2015)})

		for _, tc := range []struct {
			key  string
			year int
			want []int64
		}{
			{"start_datetime__iso_year", 2015, []int64{obj1Iso2015.ID, obj2Iso2015.ID}},
			{"start_datetime__iso_year__gt", 2014, []int64{obj1Iso2015.ID, obj2Iso2015.ID}},
			{"start_datetime__iso_year__lte", 2014, []int64{obj1Iso2014.ID}},
		} {
			qs := orm.Query[dtModel](db).Filter(tc.key, tc.year).OrderBy("start_datetime")
			djangotest.AssertQuerySetEqual(t, qs, tc.want, dtIDs)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_quarter_func_boundaries
func TestDBFunctionsExtractQuarterFuncBoundaries(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		end := time.Date(2016, 6, 15, 14, 10, 50, 123000, time.UTC)
		lastQuarter2014 := time.Date(2014, 12, 31, 13, 0, 0, 0, time.UTC)
		firstQuarter2015 := time.Date(2015, 1, 1, 13, 0, 0, 0, time.UTC)
		dtCreate(t, db, lastQuarter2014, end)
		dtCreate(t, db, firstQuarter2015, end)
		qs := orm.Query[dtModel](db).
			Filter("start_datetime__in", []time.Time{lastQuarter2014, firstQuarter2015}).
			Annotate("extracted", orm.F("start_datetime__quarter"))
		dtExtracted(t, qs, [2]any{lastQuarter2014, int64(4)}, [2]any{firstQuarter2015, int64(1)})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_week_func_boundaries
func TestDBFunctionsExtractWeekFuncBoundaries(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		end := time.Date(2016, 6, 15, 14, 10, 50, 123000, time.UTC)
		week52Day2014 := time.Date(2014, 12, 27, 13, 0, 0, 0, time.UTC)    // Sunday
		week1Day20142015 := time.Date(2014, 12, 31, 13, 0, 0, 0, time.UTC) // Wednesday
		week53Day2015 := time.Date(2015, 12, 31, 13, 0, 0, 0, time.UTC)    // Thursday
		dtCreate(t, db, week53Day2015, end)
		dtCreate(t, db, week52Day2014, end)
		dtCreate(t, db, week1Day20142015, end)
		qs := orm.Query[dtModel](db).
			Filter("start_datetime__in", []time.Time{week52Day2014, week1Day20142015, week53Day2015}).
			Annotate("extracted", orm.F("start_datetime__week"))
		dtExtracted(t, qs,
			[2]any{week52Day2014, int64(52)},
			[2]any{week1Day20142015, int64(1)},
			[2]any{week53Day2015, int64(53)})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_weekday_func
func TestDBFunctionsExtractWeekdayFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		// (isoweekday() % 7) + 1 of a Monday and a Wednesday.
		for _, key := range []string{"start_datetime__week_day", "start_date__week_day"} {
			qs := orm.Query[dtModel](db).Annotate("extracted", orm.F(key))
			dtExtracted(t, qs, [2]any{start, int64(2)}, [2]any{end, int64(4)})
		}
		qs := orm.Query[dtModel](db).Filter("start_datetime__week_day", orm.F("start_datetime__week_day"))
		if n := mustCount(t, qs); n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_iso_weekday_func
func TestDBFunctionsExtractIsoWeekdayFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		for _, key := range []string{"start_datetime__iso_week_day", "start_date__iso_week_day"} {
			qs := orm.Query[dtModel](db).Annotate("extracted", orm.F(key))
			dtExtracted(t, qs, [2]any{start, int64(1)}, [2]any{end, int64(3)})
		}
		qs := orm.Query[dtModel](db).Filter("start_datetime__week_day", orm.F("start_datetime__week_day"))
		if n := mustCount(t, qs); n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_second_func
func TestDBFunctionsExtractSecondFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		for _, key := range []string{"start_datetime__second", "start_time__second"} {
			qs := orm.Query[dtModel](db).Annotate("extracted", orm.F(key))
			dtExtracted(t, qs, [2]any{start, int64(50)}, [2]any{end, int64(50)})
		}
		qs := orm.Query[dtModel](db).Filter("start_datetime__second", orm.F("start_datetime__second"))
		if n := mustCount(t, qs); n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_second_func_no_fractional
func TestDBFunctionsExtractSecondFuncNoFractional(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start := time.Date(2015, 6, 15, 14, 30, 50, 321000, time.UTC)
		end := time.Date(2016, 6, 15, 14, 30, 50, 783000, time.UTC)
		obj := dtCreate(t, db, start, end)
		for _, tc := range [][2]string{
			{"start_datetime__second", "end_datetime__second"},
			{"start_time__second", "end_time__second"},
		} {
			qs := orm.Query[dtModel](db).Filter(tc[0], orm.F(tc[1]))
			djangotest.AssertQuerySetEqual(t, qs, []int64{obj.ID}, dtIDs)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_date_func
func TestDBFunctionsTruncDateFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		qs := orm.Query[dtModel](db).Annotate("extracted", orm.F("start_datetime__date"))
		dtExtracted(t, qs, [2]any{start, orm.DateFromTime(start)}, [2]any{end, orm.DateFromTime(end)})
		qs = orm.Query[dtModel](db).Filter("start_datetime__date", orm.F("start_datetime__date"))
		if n := mustCount(t, qs); n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_date_none
func TestDBFunctionsTruncDateNone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtCreate(t, db, time.Time{}, time.Time{})
		qs := orm.Query[dtModel](db).Annotate("truncated", orm.F("start_datetime__date")).Values("truncated")
		assertValues(t, qs, []map[string]any{{"truncated": nil}})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_time_func
func TestDBFunctionsTruncTimeFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		qs := orm.Query[dtModel](db).Annotate("extracted", orm.F("start_datetime__time"))
		dtExtracted(t, qs, [2]any{start, orm.TimeFromTime(start)}, [2]any{end, orm.TimeFromTime(end)})
		qs = orm.Query[dtModel](db).Filter("start_datetime__time", orm.F("start_datetime__time"))
		if n := mustCount(t, qs); n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_time_none
func TestDBFunctionsTruncTimeNone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtCreate(t, db, time.Time{}, time.Time{})
		qs := orm.Query[dtModel](db).Annotate("truncated", orm.F("start_datetime__time")).Values("truncated")
		assertValues(t, qs, []map[string]any{{"truncated": nil}})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_time_comparison
func TestDBFunctionsTruncTimeComparison(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start := time.Date(2015, 6, 15, 14, 30, 26, 0, time.UTC) // 0 microseconds.
		end := time.Date(2015, 6, 15, 14, 30, 26, 321000, time.UTC)
		dtCreate(t, db, start, end)
		ctx := context.Background()
		exists, err := orm.Query[dtModel](db).
			Filter("start_datetime__time", orm.TimeFromTime(start)).
			Filter("end_datetime__time", orm.TimeFromTime(end)).
			Exists(ctx)
		if err != nil || !exists {
			t.Fatalf("filter on __time: exists = %v, %v; want true", exists, err)
		}
		exists, err = orm.Query[dtModel](db).
			Annotate("extracted_start", orm.F("start_datetime__time"), "extracted_end", orm.F("end_datetime__time")).
			Filter("extracted_start", orm.TimeFromTime(start)).
			Filter("extracted_end", orm.TimeFromTime(end)).
			Exists(ctx)
		if err != nil || !exists {
			t.Fatalf("filter on annotations: exists = %v, %v; want true", exists, err)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_now.py::NowTests.test_basic
//
// A Go field cannot hold an expression, so the published date of a2 is set
// with an update instead of a2.published = Now() + timedelta(days=2).
func TestNowBasic(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("db_functions", &fnArticle{})}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		articles := orm.Query[fnArticle](db)
		a1 := create(t, db, &fnArticle{Title: "How to Django", Text: fnLoremIpsum, Written: time.Now()})
		a2 := create(t, db, &fnArticle{Title: "How to Time Travel", Text: fnLoremIpsum, Written: time.Now()})
		for _, want := range []int64{1, 0} {
			n, err := articles.Filter("id", a1.ID, "published", nil).Update(ctx, "published", orm.Now())
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			djangotest.AssertEqual(t, n, want)
		}
		if err := articles.RefreshFromDB(ctx, a1); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		djangotest.AssertEqual(t, a1.Published != nil, true)
		if _, err := articles.Filter("id", a2.ID).Update(ctx, "published", orm.Now().Add(orm.Value(48*time.Hour))); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := articles.RefreshFromDB(ctx, a2); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		djangotest.AssertEqual(t, a2.Published != nil, true)
		title := func(a fnArticle) string { return a.Title }
		djangotest.AssertQuerySetEqual(t, articles.Filter("published__lte", orm.Now()), []string{"How to Django"}, title)
		djangotest.AssertQuerySetEqual(t, articles.Filter("published__gt", orm.Now()), []string{"How to Time Travel"}, title)
	})
}

// Ported from Django: tests/db_functions/datetime/test_now.py::NowTests.test_microseconds
func TestNowMicroseconds(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("db_functions", &fnArticle{})}, func(t *testing.T, db *orm.DB) {
		create(t, db, &fnArticle{Title: "How to Django", Text: fnLoremIpsum, Written: time.Now()})
		articles := orm.Query[fnArticle](db).Annotate("now_string", orm.Cast(orm.Now(), orm.Field{Kind: orm.KindText}))
		nowString, _ := firstValue(t, articles.ValuesList("now_string")).(string)
		precision := 6
		if db.Dialect().Name() == "sqlite" {
			precision = 3
		}
		if !regexp.MustCompile(fmt.Sprintf(`^.*\.\d{1,%d}`, precision)).MatchString(nowString) {
			t.Errorf("now_string = %q, want fractional seconds of at most %d digits", nowString, precision)
		}
	})
}
