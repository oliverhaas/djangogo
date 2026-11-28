package orm_test

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of Django's tests/db_functions/datetime/test_extract_trunc.py run
// the DateFunctionWithTimeZoneTests variant: USE_TZ = True in UTC. The ORM
// has no current time zone, so the parts of the tests that set one with
// timezone.override are skipped. A Trunc value of a datetime is an instant,
// which the asserts compare as instants, where Django compares the wall time
// in the zone of the tzinfo argument.

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

// dtTruncated asserts the (start_datetime, truncated) rows of qs ordered by
// start_datetime, Django's assertSequenceEqual of
// queryset.values_list("start_datetime", "truncated").
func dtTruncated(t *testing.T, qs *orm.QuerySet[dtModel], want ...[2]any) {
	t.Helper()
	rows := make([][]any, 0, len(want))
	for _, w := range want {
		rows = append(rows, []any{w[0], w[1]})
	}
	assertValuesList(t, qs.OrderBy("start_datetime").ValuesList("start_datetime", "truncated"), rows)
}

// dtAssertGet asserts that qs has one row with the values of want, Django's
// assertEqual of each attribute of qs.get().
func dtAssertGet(t *testing.T, qs *orm.QuerySet[dtModel], want map[string]any) {
	t.Helper()
	assertValues(t, qs.Values(slices.Sorted(maps.Keys(want))...), []map[string]any{want})
}

// dtAssertAnnotateError fails t unless annotating the rows with expr fails
// with an error that contains want, Django's assertRaisesMessage around
// list(DTModel.objects.annotate(truncated=expr)).
func dtAssertAnnotateError(t *testing.T, db *orm.DB, expr orm.Expr, want string) {
	t.Helper()
	fnAssertQueryError(t, orm.Query[dtModel](db).Annotate("truncated", expr).ValuesList("truncated"), want)
}

// dtLocation returns the zone of the IANA database, Django's
// zoneinfo.ZoneInfo(name).
func dtLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

// dtStartEnd returns the datetimes the extract and trunc tests use.
func dtStartEnd() (start, end time.Time) {
	return time.Date(2015, 6, 15, 14, 30, 50, 321000, time.UTC),
		time.Date(2016, 6, 15, 14, 10, 50, 123000, time.UTC)
}

// dtTruncateTo is Django's truncate_to of a datetime: value converted to
// loc, UTC when loc is nil, and truncated to kind there. The result is a
// wall time of loc, as Django's make_aware gives it after a daylight saving
// change.
func dtTruncateTo(value time.Time, kind string, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	v := value.In(loc)
	y, m, d := v.Date()
	h, mi, s := v.Clock()
	switch kind {
	case "second":
		return time.Date(y, m, d, h, mi, s, 0, loc)
	case "minute":
		return time.Date(y, m, d, h, mi, 0, 0, loc)
	case "hour":
		return time.Date(y, m, d, h, 0, 0, 0, loc)
	case "day":
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	case "week":
		// Python's weekday() counts from Monday = 0.
		return time.Date(y, m, d-(int(v.Weekday())+6)%7, 0, 0, 0, 0, loc)
	case "month":
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	case "quarter":
		return time.Date(y, m-(m-1)%3, 1, 0, 0, 0, 0, loc)
	}
	return time.Date(y, time.January, 1, 0, 0, 0, 0, loc)
}

// dtTruncateDate is Django's truncate_to of a date.
func dtTruncateDate(value orm.Date, kind string) orm.Date {
	return orm.DateFromTime(dtTruncateTo(value.Time(), kind, nil))
}

// dtTruncateTime is Django's truncate_to of a time of day.
func dtTruncateTime(value orm.Time, kind string) orm.Time {
	at := time.Date(2000, time.January, 1, value.Hour, value.Minute, value.Second, value.Nanosecond, time.UTC)
	return orm.TimeFromTime(dtTruncateTo(at, kind, nil))
}

// dtYearRows creates the two rows of the year lookup tests.
func dtYearRows(t *testing.T, db *orm.DB) {
	t.Helper()
	start := time.Date(2015, 6, 15, 14, 10, 0, 0, time.UTC)
	end := time.Date(2016, 6, 15, 14, 10, 0, 0, time.UTC)
	dtCreate(t, db, start, end)
	dtCreate(t, db, end, start)
}

// dtExtractFunc asserts Django's test_extract_<lookup>_func of fn, the
// Extract function of lookup: of the two rows of start and end, fn of
// start_datetime and of other (start_date or start_time) gives want, and fn
// of start_datetime matches the lookup of start_datetime in both rows.
func dtExtractFunc(t *testing.T, db *orm.DB, start, end time.Time, fn func(any) orm.Expr, lookup, other string, want [2]int64) {
	t.Helper()
	dtCreate(t, db, start, end)
	dtCreate(t, db, end, start)
	qs := orm.Query[dtModel](db)
	for _, field := range []string{"start_datetime", other} {
		dtExtracted(t, qs.Annotate("extracted", fn(field)), [2]any{start, want[0]}, [2]any{end, want[1]})
	}
	djangotest.AssertEqual(t, mustCount(t, qs.Filter("start_datetime__"+lookup, fn("start_datetime"))), int64(2))
}

// dtTruncFunc asserts Django's test_trunc_<kind>_func of fn, the Trunc
// function of kind: of two rows, the start is truncated in start_datetime
// and in each field of others (start_date or start_time); only the row whose
// start is truncated already matches start_datetime=fn("start_datetime");
// and fn of the field bad fails, also with Output of the kind of bad.
func dtTruncFunc(t *testing.T, db *orm.DB, kind string, fn func(any) orm.Expr, others []string, bad string) {
	t.Helper()
	start, end := dtStartEnd()
	end = dtTruncateTo(end, kind, nil)
	dtCreate(t, db, start, end)
	dtCreate(t, db, end, start)
	qs := orm.Query[dtModel](db)
	dtExtracted(t, qs.Annotate("extracted", fn("start_datetime")),
		[2]any{start, dtTruncateTo(start, kind, nil)}, [2]any{end, dtTruncateTo(end, kind, nil)})
	for _, field := range others {
		truncated := func(v time.Time) any { return dtTruncateTime(orm.TimeFromTime(v), kind) }
		if field == "start_date" {
			truncated = func(v time.Time) any { return dtTruncateDate(orm.DateFromTime(v), kind) }
		}
		dtExtracted(t, qs.Annotate("extracted", fn(field)), [2]any{start, truncated(start)}, [2]any{end, truncated(end)})
	}
	djangotest.AssertEqual(t, mustCount(t, qs.Filter("start_datetime", fn("start_datetime"))), int64(1))
	badKind, msg := orm.KindTime, "Cannot truncate TimeField 'start_time' to DateTimeField"
	if bad == "start_date" {
		badKind, msg = orm.KindDate, "Cannot truncate DateField 'start_date' to DateTimeField"
	}
	dtAssertAnnotateError(t, db, fn(bad), msg)
	dtAssertAnnotateError(t, db, fn(bad).Output(badKind), msg)
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
					Annotate("start_year", orm.ExtractYear("start_datetime")).
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
					Annotate("start_year", orm.ExtractYear("start_datetime")).
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
					Annotate("end_year", orm.ExtractYear("end_datetime")).
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

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_lookup_name_sql_injection
func TestDBFunctionsExtractLookupNameSQLInjection(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		_, err := orm.Query[dtModel](db).
			Filter("start_datetime__year", orm.Extract("start_datetime", "day' FROM start_datetime)) OR 1=1;--")).
			Exists(context.Background())
		if err == nil {
			t.Fatal("Exists: no error, want one")
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_func
func TestDBFunctionsExtractFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		dtAssertAnnotateError(t, db, orm.Extract("start_datetime", ""), "lookup_name must be provided")
		dtAssertAnnotateError(t, db, orm.Extract("name", "hour"),
			"Extract input expression must be DateField, DateTimeField, TimeField, or DurationField.")
		dtAssertAnnotateError(t, db, orm.Extract("start_date", "second"),
			"Cannot extract time component 'second' from DateField 'start_date'.")
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
				qs := orm.Query[dtModel](db).Annotate("extracted", orm.Extract("start_datetime", tc.name))
				dtExtracted(t, qs, [2]any{start, tc.start}, [2]any{end, tc.end})
			})
		}
		for _, tc := range [][2]string{
			{"start_datetime", "year"}, {"start_datetime", "hour"}, {"start_date", "month"}, {"start_time", "hour"},
		} {
			qs := orm.Query[dtModel](db).Filter(tc[0]+"__"+tc[1], orm.Extract(tc[0], tc[1]))
			if n := mustCount(t, qs); n != 2 {
				t.Fatalf("%s__%s=Extract: count = %d, want 2", tc[0], tc[1], n)
			}
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_none
func TestDBFunctionsExtractNone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtCreate(t, db, time.Time{}, time.Time{})
		for _, tc := range [][2]string{{"start_datetime", "year"}, {"start_date", "year"}, {"start_time", "hour"}} {
			t.Run(tc[0], func(t *testing.T) {
				qs := orm.Query[dtModel](db).Annotate("extracted", orm.Extract(tc[0], tc[1])).Values("extracted")
				assertValues(t, qs, []map[string]any{{"extracted": nil}})
			})
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_outerref_validation
func TestDBFunctionsExtractOuterrefValidation(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		inner := orm.Query[dtModel](db).Filter("name", orm.ExtractMonth(orm.OuterRef("name")))
		qs := orm.Query[dtModel](db).Annotate("related_name", orm.Subquery(inner.Values("name").Limit(1)))
		fnAssertQueryError(t, qs.ValuesList("related_name"),
			"Extract input expression must be DateField, DateTimeField, TimeField, or DurationField.")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_duration
//
// Python's timedelta keeps its seconds part positive, so Django reads 0 as
// the second of the negative duration of the second row. A Go
// time.Duration is negative as a whole, and its second is -59.
func TestDBFunctionsExtractDuration(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "sqlite" {
			t.Skip("SQLite has no native duration field (has_native_duration_field)")
		}
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		second := func(d time.Duration) int64 { return int64(d/time.Second) % 60 }
		qs := orm.Query[dtModel](db).Annotate("extracted", orm.Extract("duration", "second"))
		dtExtracted(t, qs, [2]any{start, second(end.Sub(start))}, [2]any{end, second(start.Sub(end))})
		qs = orm.Query[dtModel](db).
			Annotate("duration_days", orm.Extract("duration", "day")).
			Filter("duration_days__gt", 200)
		djangotest.AssertEqual(t, mustCount(t, qs), int64(1))
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_duration_without_native_duration_field
func TestDBFunctionsExtractDurationWithoutNativeDurationField(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "postgres" {
			t.Skip("PostgreSQL has a native duration field (has_native_duration_field)")
		}
		dtAssertAnnotateError(t, db, orm.Extract("duration", "second"),
			"Extract requires native DurationField database support.")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_duration_unsupported_lookups
func TestDBFunctionsExtractDurationUnsupportedLookups(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		for _, lookup := range []string{"year", "iso_year", "month", "week", "week_day", "iso_week_day", "quarter"} {
			t.Run(lookup, func(t *testing.T) {
				dtAssertAnnotateError(t, db, orm.Extract("duration", lookup),
					fmt.Sprintf("Cannot extract component '%s' from DurationField 'duration'.", lookup))
			})
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_year_func
func TestDBFunctionsExtractYearFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtExtractFunc(t, db, start, end, orm.ExtractYear, "year", "start_date", [2]int64{2015, 2016})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_iso_year_func
func TestDBFunctionsExtractIsoYearFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		// Both dates are from the same week year.
		dtExtractFunc(t, db, start, end, orm.ExtractIsoYear, "iso_year", "start_date", [2]int64{2015, 2016})
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
			Annotate("extracted", orm.ExtractIsoYear("start_datetime"))
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

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_month_func
func TestDBFunctionsExtractMonthFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtExtractFunc(t, db, start, end, orm.ExtractMonth, "month", "start_date", [2]int64{6, 6})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_day_func
func TestDBFunctionsExtractDayFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtExtractFunc(t, db, start, end, orm.ExtractDay, "day", "start_date", [2]int64{15, 15})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_week_func
func TestDBFunctionsExtractWeekFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtExtractFunc(t, db, start, end, orm.ExtractWeek, "week", "start_date", [2]int64{25, 24})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_quarter_func
func TestDBFunctionsExtractQuarterFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, _ := dtStartEnd()
		end := time.Date(2016, 8, 15, 14, 10, 50, 123000, time.UTC)
		dtExtractFunc(t, db, start, end, orm.ExtractQuarter, "quarter", "start_date", [2]int64{2, 3})
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
			Annotate("extracted", orm.ExtractQuarter("start_datetime"))
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
			Annotate("extracted", orm.ExtractWeek("start_datetime"))
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
		// (isoweekday() % 7) + 1 of a Monday and a Wednesday.
		dtExtractFunc(t, db, start, end, orm.ExtractWeekDay, "week_day", "start_date", [2]int64{2, 4})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_iso_weekday_func
func TestDBFunctionsExtractIsoWeekdayFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		for _, field := range []string{"start_datetime", "start_date"} {
			qs := orm.Query[dtModel](db).Annotate("extracted", orm.ExtractIsoWeekDay(field))
			dtExtracted(t, qs, [2]any{start, int64(1)}, [2]any{end, int64(3)})
		}
		qs := orm.Query[dtModel](db).Filter("start_datetime__week_day", orm.ExtractWeekDay("start_datetime"))
		djangotest.AssertEqual(t, mustCount(t, qs), int64(2))
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_hour_func
func TestDBFunctionsExtractHourFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtExtractFunc(t, db, start, end, orm.ExtractHour, "hour", "start_time", [2]int64{14, 14})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_minute_func
func TestDBFunctionsExtractMinuteFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtExtractFunc(t, db, start, end, orm.ExtractMinute, "minute", "start_time", [2]int64{30, 10})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_second_func
func TestDBFunctionsExtractSecondFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtExtractFunc(t, db, start, end, orm.ExtractSecond, "second", "start_time", [2]int64{50, 50})
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

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_lookup_name_sql_injection
func TestDBFunctionsTruncLookupNameSQLInjection(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		// Database backends raise an exception or don't return any results.
		exists, err := orm.Query[dtModel](db).
			Filter("start_datetime__date", orm.Trunc("start_datetime", "year', start_datetime)) OR 1=1;--")).
			Exists(context.Background())
		if err == nil && exists {
			t.Fatal("Exists = true, want false or an error")
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_func
func TestDBFunctionsTruncFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start := time.Date(999, 6, 15, 14, 30, 50, 321000, time.UTC)
		_, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		qs := orm.Query[dtModel](db)
		startDate, endDate := orm.DateFromTime(start), orm.DateFromTime(end)
		startTime, endTime := orm.TimeFromTime(start), orm.TimeFromTime(end)
		dateTruncations := []string{"year", "quarter", "month", "day"}
		timeTruncations := []string{"hour", "minute", "second"}
		for _, kind := range dateTruncations {
			t.Run("assertDateKind/"+kind, func(t *testing.T) {
				dtTruncated(t, qs.Annotate("truncated", orm.Trunc("start_date", kind).Output(orm.KindDate)),
					[2]any{start, dtTruncateDate(startDate, kind)}, [2]any{end, dtTruncateDate(endDate, kind)})
			})
		}
		for _, kind := range timeTruncations {
			t.Run("assertTimeKind/"+kind, func(t *testing.T) {
				dtTruncated(t, qs.Annotate("truncated", orm.Trunc("start_time", kind).Output(orm.KindTime)),
					[2]any{start, dtTruncateTime(startTime, kind)}, [2]any{end, dtTruncateTime(endTime, kind)})
			})
		}
		for _, kind := range slices.Concat(dateTruncations, timeTruncations) {
			t.Run("assertDatetimeKind/"+kind, func(t *testing.T) {
				dtTruncated(t, qs.Annotate("truncated", orm.Trunc("start_datetime", kind).Output(orm.KindDateTime)),
					[2]any{start, dtTruncateTo(start, kind, nil)}, [2]any{end, dtTruncateTo(end, kind, nil)})
			})
		}
		for _, kind := range timeTruncations {
			t.Run("assertDatetimeToTimeKind/"+kind, func(t *testing.T) {
				dtTruncated(t, qs.Annotate("truncated", orm.Trunc("start_datetime", kind).Output(orm.KindTime)),
					[2]any{start, dtTruncateTime(startTime, kind)}, [2]any{end, dtTruncateTime(endTime, kind)})
			})
		}
		n := mustCount(t, qs.Filter("start_datetime__date", orm.Trunc("start_datetime", "day").Output(orm.KindDate)))
		djangotest.AssertEqual(t, n, int64(2))
	})
}

// dtTruncWeek is Django's _test_trunc_week: Trunc to the week of
// start_datetime and of start_date in two rows of start and end.
func dtTruncWeek(t *testing.T, db *orm.DB, start, end time.Time) {
	t.Helper()
	dtCreate(t, db, start, end)
	dtCreate(t, db, end, start)
	qs := orm.Query[dtModel](db)
	dtTruncated(t, qs.Annotate("truncated", orm.Trunc("start_datetime", "week").Output(orm.KindDateTime)),
		[2]any{start, dtTruncateTo(start, "week", nil)}, [2]any{end, dtTruncateTo(end, "week", nil)})
	dtTruncated(t, qs.Annotate("truncated", orm.Trunc("start_date", "week").Output(orm.KindDate)),
		[2]any{start, dtTruncateDate(orm.DateFromTime(start), "week")},
		[2]any{end, dtTruncateDate(orm.DateFromTime(end), "week")})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_week
func TestDBFunctionsTruncWeek(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtTruncWeek(t, db, start, end)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_week_before_1000
func TestDBFunctionsTruncWeekBefore1000(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		_, end := dtStartEnd()
		dtTruncWeek(t, db, time.Date(999, 6, 15, 14, 30, 50, 321000, time.UTC), end)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_invalid_arguments
func TestDBFunctionsTruncInvalidArguments(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtAssertAnnotateError(t, db, orm.Trunc("start_datetime", "year").Output(orm.KindInt),
			"output_field must be either DateField, TimeField, or DateTimeField")
		dtAssertAnnotateError(t, db, orm.Trunc("name", "year").Output(orm.KindDateTime),
			"'name' isn't a DateField, TimeField, or DateTimeField.")
		msg := "Cannot truncate DateField 'start_date' to DateTimeField"
		dtAssertAnnotateError(t, db, orm.Trunc("start_date", "second"), msg)
		dtAssertAnnotateError(t, db, orm.Trunc("start_date", "month").Output(orm.KindDateTime), msg)
		msg = "Cannot truncate TimeField 'start_time' to DateTimeField"
		dtAssertAnnotateError(t, db, orm.Trunc("start_time", "month"), msg)
		dtAssertAnnotateError(t, db, orm.Trunc("start_time", "second").Output(orm.KindDateTime), msg)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_none
func TestDBFunctionsTruncNone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtCreate(t, db, time.Time{}, time.Time{})
		for _, tc := range [][2]string{{"start_datetime", "year"}, {"start_date", "year"}, {"start_time", "hour"}} {
			t.Run(tc[0], func(t *testing.T) {
				qs := orm.Query[dtModel](db).Annotate("truncated", orm.Trunc(tc[0], tc[1])).Values("truncated")
				assertValues(t, qs, []map[string]any{{"truncated": nil}})
			})
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_year_func
func TestDBFunctionsTruncYearFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtTruncFunc(t, db, "year", orm.TruncYear, []string{"start_date"}, "start_time")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_quarter_func
func TestDBFunctionsTruncQuarterFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, _ := dtStartEnd()
		end := dtTruncateTo(time.Date(2016, 10, 15, 14, 10, 50, 123000, time.UTC), "quarter", nil)
		lastQuarter2015 := dtTruncateTo(time.Date(2015, 12, 31, 14, 10, 50, 123000, time.UTC), "quarter", nil)
		firstQuarter2016 := dtTruncateTo(time.Date(2016, 1, 1, 14, 10, 50, 123000, time.UTC), "quarter", nil)
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		dtCreate(t, db, lastQuarter2015, end)
		dtCreate(t, db, firstQuarter2016, end)
		qs := orm.Query[dtModel](db)
		date := func(v time.Time) orm.Date { return dtTruncateDate(orm.DateFromTime(v), "quarter") }
		dtExtracted(t, qs.Annotate("extracted", orm.TruncQuarter("start_date")),
			[2]any{start, date(start)},
			[2]any{lastQuarter2015, date(lastQuarter2015)},
			[2]any{firstQuarter2016, date(firstQuarter2016)},
			[2]any{end, date(end)})
		datetime := func(v time.Time) time.Time { return dtTruncateTo(v, "quarter", nil) }
		dtExtracted(t, qs.Annotate("extracted", orm.TruncQuarter("start_datetime")),
			[2]any{start, datetime(start)},
			[2]any{lastQuarter2015, datetime(lastQuarter2015)},
			[2]any{firstQuarter2016, datetime(firstQuarter2016)},
			[2]any{end, datetime(end)})
		msg := "Cannot truncate TimeField 'start_time' to DateTimeField"
		dtAssertAnnotateError(t, db, orm.TruncQuarter("start_time"), msg)
		dtAssertAnnotateError(t, db, orm.TruncQuarter("start_time").Output(orm.KindTime), msg)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_month_func
func TestDBFunctionsTruncMonthFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtTruncFunc(t, db, "month", orm.TruncMonth, []string{"start_date"}, "start_time")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_week_func
func TestDBFunctionsTruncWeekFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtTruncFunc(t, db, "week", orm.TruncWeek, nil, "start_time")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_date_func
func TestDBFunctionsTruncDateFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		qs := orm.Query[dtModel](db).Annotate("extracted", orm.TruncDate("start_datetime"))
		dtExtracted(t, qs, [2]any{start, orm.DateFromTime(start)}, [2]any{end, orm.DateFromTime(end)})
		qs = orm.Query[dtModel](db).Filter("start_datetime__date", orm.TruncDate("start_datetime"))
		djangotest.AssertEqual(t, mustCount(t, qs), int64(2))
		msg := "Cannot truncate TimeField 'start_time' to DateField"
		dtAssertAnnotateError(t, db, orm.TruncDate("start_time"), msg)
		dtAssertAnnotateError(t, db, orm.TruncDate("start_time").Output(orm.KindTime), msg)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_date_none
func TestDBFunctionsTruncDateNone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtCreate(t, db, time.Time{}, time.Time{})
		qs := orm.Query[dtModel](db).Annotate("truncated", orm.TruncDate("start_datetime")).Values("truncated")
		assertValues(t, qs, []map[string]any{{"truncated": nil}})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_time_func
func TestDBFunctionsTruncTimeFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		qs := orm.Query[dtModel](db).Annotate("extracted", orm.TruncTime("start_datetime"))
		dtExtracted(t, qs, [2]any{start, orm.TimeFromTime(start)}, [2]any{end, orm.TimeFromTime(end)})
		qs = orm.Query[dtModel](db).Filter("start_datetime__time", orm.TruncTime("start_datetime"))
		djangotest.AssertEqual(t, mustCount(t, qs), int64(2))
		msg := "Cannot truncate DateField 'start_date' to TimeField"
		dtAssertAnnotateError(t, db, orm.TruncTime("start_date"), msg)
		dtAssertAnnotateError(t, db, orm.TruncTime("start_date").Output(orm.KindDate), msg)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_time_none
func TestDBFunctionsTruncTimeNone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtCreate(t, db, time.Time{}, time.Time{})
		qs := orm.Query[dtModel](db).Annotate("truncated", orm.TruncTime("start_datetime")).Values("truncated")
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
			Annotate("extracted_start", orm.TruncTime("start_datetime"), "extracted_end", orm.TruncTime("end_datetime")).
			Filter("extracted_start", orm.TimeFromTime(start)).
			Filter("extracted_end", orm.TimeFromTime(end)).
			Exists(ctx)
		if err != nil || !exists {
			t.Fatalf("filter on annotations: exists = %v, %v; want true", exists, err)
		}
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_day_func
func TestDBFunctionsTruncDayFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtTruncFunc(t, db, "day", orm.TruncDay, nil, "start_time")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_hour_func
func TestDBFunctionsTruncHourFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtTruncFunc(t, db, "hour", orm.TruncHour, []string{"start_time"}, "start_date")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_minute_func
func TestDBFunctionsTruncMinuteFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtTruncFunc(t, db, "minute", orm.TruncMinute, []string{"start_time"}, "start_date")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_second_func
func TestDBFunctionsTruncSecondFunc(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		dtTruncFunc(t, db, "second", orm.TruncSecond, []string{"start_time"}, "start_date")
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_subquery_with_parameters
func TestDBFunctionsTruncSubqueryWithParameters(t *testing.T) {
	models := []any{djangotest.App("db_functions", &fnAuthor{}, &fnFan{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		author1 := create(t, db, &fnAuthor{Name: "J. R. R. Tolkien"})
		author2 := create(t, db, &fnAuthor{Name: "G. R. R. Martin"})
		fanSince1 := time.Date(2016, 2, 3, 15, 0, 0, 0, time.UTC)
		fanSince2 := time.Date(2015, 2, 3, 15, 0, 0, 0, time.UTC)
		fanSince3 := time.Date(2017, 2, 3, 15, 0, 0, 0, time.UTC)
		create(t, db, &fnFan{Author: fkTo[fnAuthor](author1.ID), Name: "Tom", FanSince: &fanSince1})
		create(t, db, &fnFan{Author: fkTo[fnAuthor](author1.ID), Name: "Emma", FanSince: &fanSince2})
		create(t, db, &fnFan{Author: fkTo[fnAuthor](author2.ID), Name: "Isabella", FanSince: &fanSince3})

		inner := orm.Query[fnFan](db).
			Filter("author", orm.OuterRef("pk"), "name__in", []string{"Emma", "Isabella", "Tom"}).
			Values("author").
			Annotate("newest_fan", orm.Max("fan_since")).
			Values("newest_fan")
		outer := orm.Query[fnAuthor](db).Annotate("newest_fan_year", orm.TruncYear(orm.Subquery(inner)))
		assertValues(t, outer.OrderBy("name").Values("name", "newest_fan_year"), []map[string]any{
			{"name": "G. R. R. Martin", "newest_fan_year": time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)},
			{"name": "J. R. R. Tolkien", "newest_fan_year": time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)},
		})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_outerref
func TestDBFunctionsExtractOuterref(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		datetime1 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		datetime2 := time.Date(2001, 3, 5, 0, 0, 0, 0, time.UTC)
		datetime3 := time.Date(2002, 1, 3, 0, 0, 0, 0, time.UTC)
		obj1 := dtCreate(t, db, datetime1, datetime3)
		obj2 := dtCreate(t, db, datetime2, datetime1)
		obj3 := dtCreate(t, db, datetime3, datetime2)

		inner := orm.Query[dtModel](db).Filter(
			"start_datetime__year", 2000,
			"start_datetime__month", orm.ExtractMonth(orm.OuterRef("end_datetime")),
		)
		qs := orm.Query[dtModel](db).Annotate("related_pk", orm.Subquery(inner.Values("pk").Limit(1)))
		assertValues(t, qs.OrderBy("name").Values("pk", "related_pk"), []map[string]any{
			{"pk": obj1.ID, "related_pk": obj1.ID},
			{"pk": obj2.ID, "related_pk": obj1.ID},
			{"pk": obj3.ID, "related_pk": nil},
		})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_func_with_timezone
func TestDBFunctionsExtractFuncWithTimezone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start := time.Date(2015, 6, 15, 23, 30, 1, 321000, time.UTC)
		end := time.Date(2015, 6, 16, 13, 11, 27, 123000, time.UTC)
		dtCreate(t, db, start, end)
		deltaPos := time.FixedZone("", 5*3600)
		deltaNeg := time.FixedZone("", -(4*3600 + 43*60))
		melb := dtLocation(t, "Australia/Melbourne")

		qs := orm.Query[dtModel](db).Annotate(
			"day", orm.Extract("start_datetime", "day"),
			"day_melb", orm.Extract("start_datetime", "day").In(melb),
			"week", orm.Extract("start_datetime", "week").In(melb),
			"isoyear", orm.ExtractIsoYear("start_datetime").In(melb),
			"weekday", orm.ExtractWeekDay("start_datetime"),
			"weekday_melb", orm.ExtractWeekDay("start_datetime").In(melb),
			"isoweekday", orm.ExtractIsoWeekDay("start_datetime"),
			"isoweekday_melb", orm.ExtractIsoWeekDay("start_datetime").In(melb),
			"quarter", orm.ExtractQuarter("start_datetime").In(melb),
			"hour", orm.ExtractHour("start_datetime"),
			"hour_melb", orm.ExtractHour("start_datetime").In(melb),
			"hour_with_delta_pos", orm.ExtractHour("start_datetime").In(deltaPos),
			"hour_with_delta_neg", orm.ExtractHour("start_datetime").In(deltaNeg),
			"minute_with_delta_neg", orm.ExtractMinute("start_datetime").In(deltaNeg),
		).OrderBy("start_datetime")
		dtAssertGet(t, qs, map[string]any{
			"day":                   int64(15),
			"day_melb":              int64(16),
			"week":                  int64(25),
			"isoyear":               int64(2015),
			"weekday":               int64(2),
			"weekday_melb":          int64(3),
			"isoweekday":            int64(1),
			"isoweekday_melb":       int64(2),
			"quarter":               int64(2),
			"hour":                  int64(23),
			"hour_melb":             int64(9),
			"hour_with_delta_pos":   int64(4),
			"hour_with_delta_neg":   int64(18),
			"minute_with_delta_neg": int64(47),
		})
		t.Run("override", func(t *testing.T) {
			t.Skip("FIXME-PORT: the ORM has no current time zone")
		})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_func_with_timezone_minus_no_offset
func TestDBFunctionsExtractFuncWithTimezoneMinusNoOffset(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start := time.Date(2015, 6, 15, 23, 30, 1, 321000, time.UTC)
		end := time.Date(2015, 6, 16, 13, 11, 27, 123000, time.UTC)
		dtCreate(t, db, start, end)
		ustNera := dtLocation(t, "Asia/Ust-Nera")

		qs := orm.Query[dtModel](db).Annotate(
			"hour", orm.ExtractHour("start_datetime"),
			"hour_tz", orm.ExtractHour("start_datetime").In(ustNera),
		).OrderBy("start_datetime")
		dtAssertGet(t, qs, map[string]any{"hour": int64(23), "hour_tz": int64(9)})
		t.Run("override", func(t *testing.T) {
			t.Skip("FIXME-PORT: the ORM has no current time zone")
		})
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_func_explicit_timezone_priority
func TestDBFunctionsExtractFuncExplicitTimezonePriority(t *testing.T) {
	t.Skip("FIXME-PORT: the ORM has no current time zone")
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_extract_invalid_field_with_timezone
func TestDBFunctionsExtractInvalidFieldWithTimezone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		melb := dtLocation(t, "Australia/Melbourne")
		msg := "tzinfo can only be used with DateTimeField."
		dtAssertAnnotateError(t, db, orm.Extract("start_date", "day").In(melb), msg)
		dtAssertAnnotateError(t, db, orm.Extract("start_time", "hour").In(melb), msg)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_timezone_applied_before_truncation
func TestDBFunctionsTruncTimezoneAppliedBeforeTruncation(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start := time.Date(2016, 1, 1, 1, 30, 50, 321000, time.UTC)
		end := time.Date(2016, 6, 15, 14, 10, 50, 123000, time.UTC)
		dtCreate(t, db, start, end)
		melb := dtLocation(t, "Australia/Melbourne")
		pacific := dtLocation(t, "America/Los_Angeles")

		qs := orm.Query[dtModel](db).Annotate(
			"melb_year", orm.TruncYear("start_datetime").In(melb),
			"pacific_year", orm.TruncYear("start_datetime").In(pacific),
			"melb_date", orm.TruncDate("start_datetime").In(melb),
			"pacific_date", orm.TruncDate("start_datetime").In(pacific),
			"melb_time", orm.TruncTime("start_datetime").In(melb),
			"pacific_time", orm.TruncTime("start_datetime").In(pacific),
		).OrderBy("start_datetime")
		melbStart, pacificStart := start.In(melb), start.In(pacific)
		melbYear, pacificYear := dtTruncateTo(start, "year", melb), dtTruncateTo(start, "year", pacific)
		dtAssertGet(t, qs, map[string]any{
			"start_datetime": start,
			"melb_year":      melbYear,
			"pacific_year":   pacificYear,
			"melb_date":      orm.DateFromTime(melbStart),
			"pacific_date":   orm.DateFromTime(pacificStart),
			"melb_time":      orm.TimeFromTime(melbStart),
			"pacific_time":   orm.TimeFromTime(pacificStart),
		})
		djangotest.AssertEqual(t, start.Year(), 2016)
		djangotest.AssertEqual(t, melbYear.Year(), 2016)
		djangotest.AssertEqual(t, pacificYear.Year(), 2015)
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_func_with_timezone
//
// If the truncated datetime transitions to a different offset (daylight
// saving) then the returned value will have that new timezone/offset.
func TestDBFunctionsTruncFuncWithTimezone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		start, end := dtStartEnd()
		dtCreate(t, db, start, end)
		dtCreate(t, db, end, start)
		qs := orm.Query[dtModel](db)
		dateTruncations := []string{"year", "quarter", "month", "week", "day"}
		timeTruncations := []string{"hour", "minute", "second"}
		for _, name := range []string{"Australia/Melbourne", "Etc/GMT+10"} {
			tz := dtLocation(t, name)
			for _, kind := range dateTruncations {
				t.Run("assertDatetimeToDateKind/"+kind+"/"+name, func(t *testing.T) {
					expr := orm.Trunc("start_datetime", kind).Output(orm.KindDate).In(tz)
					dtTruncated(t, qs.Annotate("truncated", expr),
						[2]any{start, dtTruncateDate(orm.DateFromTime(start.In(tz)), kind)},
						[2]any{end, dtTruncateDate(orm.DateFromTime(end.In(tz)), kind)})
				})
			}
			for _, kind := range timeTruncations {
				t.Run("assertDatetimeToTimeKind/"+kind+"/"+name, func(t *testing.T) {
					expr := orm.Trunc("start_datetime", kind).Output(orm.KindTime).In(tz)
					dtTruncated(t, qs.Annotate("truncated", expr),
						[2]any{start, dtTruncateTime(orm.TimeFromTime(start.In(tz)), kind)},
						[2]any{end, dtTruncateTime(orm.TimeFromTime(end.In(tz)), kind)})
				})
			}
			for _, kind := range slices.Concat(dateTruncations, timeTruncations) {
				t.Run("assertDatetimeKind/"+kind+"/"+name, func(t *testing.T) {
					expr := orm.Trunc("start_datetime", kind).Output(orm.KindDateTime).In(tz)
					dtTruncated(t, qs.Annotate("truncated", expr),
						[2]any{start, dtTruncateTo(start, kind, tz)}, [2]any{end, dtTruncateTo(end, kind, tz)})
				})
			}
		}
		n := mustCount(t, qs.Filter("start_datetime__date", orm.Trunc("start_datetime", "day").Output(orm.KindDate)))
		djangotest.AssertEqual(t, n, int64(2))
	})
}

// Ported from Django: tests/db_functions/datetime/test_extract_trunc.py::DateFunctionWithTimeZoneTests.test_trunc_invalid_field_with_timezone
func TestDBFunctionsTruncInvalidFieldWithTimezone(t *testing.T) {
	djangotest.EachDB(t, dtModels(), func(t *testing.T, db *orm.DB) {
		melb := dtLocation(t, "Australia/Melbourne")
		msg := "tzinfo can only be used with DateTimeField."
		dtAssertAnnotateError(t, db, orm.Trunc("start_date", "day").In(melb), msg)
		dtAssertAnnotateError(t, db, orm.Trunc("start_time", "hour").In(melb), msg)
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
