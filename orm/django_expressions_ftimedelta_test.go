package orm_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of FTimeDeltaTests in Django's tests/expressions/tests.py: date,
// datetime, time of day and duration arithmetic with F expressions.

// ftdDay is Python's timedelta(1).
const ftdDay = 24 * time.Hour

// ftdFixture is the data of FTimeDeltaTests.setUpTestData. The deltas,
// delays and days_long of the experiments increase strictly, in the order of
// the experiment names.
type ftdFixture struct {
	sday     orm.Date
	stime    time.Time
	e0       *expExperiment
	deltas   []time.Duration
	delays   []time.Duration
	daysLong []time.Duration
	expnames []string
}

// ftdSetUp creates the experiments e0 to e5 of
// FTimeDeltaTests.setUpTestData.
func ftdSetUp(t *testing.T, db *orm.DB) *ftdFixture {
	t.Helper()
	fx := &ftdFixture{
		sday:  orm.NewDate(2010, 6, 25),
		stime: time.Date(2010, 6, 25, 12, 15, 30, 747000000, time.UTC),
	}
	daysBefore := func(d orm.Date, days int) orm.Date { return orm.DateFromTime(d.Time().AddDate(0, 0, -days)) }
	add := func(name string, assigned orm.Date, start time.Time, delta, estimated time.Duration) *expExperiment {
		end := start.Add(delta)
		e := create(t, db, &expExperiment{
			Name: name, Assigned: assigned, Start: start, End: end,
			Completed: orm.DateFromTime(end), EstimatedTime: estimated,
		})
		fx.deltas = append(fx.deltas, delta)
		fx.delays = append(fx.delays, e.Start.Sub(e.Assigned.Time()))
		fx.daysLong = append(fx.daysLong, e.Completed.Time().Sub(e.Assigned.Time()))
		return e
	}
	delta1 := 253000 * time.Microsecond
	delta3 := 21*time.Hour + 8*time.Minute
	delta4 := 10 * ftdDay
	delta5 := 90 * ftdDay
	// e0 starts on the day it is assigned and takes no time.
	fx.e0 = add("e0", fx.sday, fx.stime, 0, 0)
	// e1 starts a day after it is assigned and ends on a whole second, an
	// edge case on SQLite.
	add("e1", fx.sday, fx.stime.Add(ftdDay), delta1, delta1)
	// e2 starts three days after it is assigned.
	add("e2", daysBefore(fx.sday, 3), fx.stime, 44*time.Second, time.Hour)
	// e3 starts four days after it is assigned.
	add("e3", fx.sday, fx.stime.Add(4*ftdDay), delta3, delta3)
	// e4 starts ten days after it is assigned.
	add("e4", daysBefore(fx.sday, 10), fx.stime, delta4, delta4-ftdDay)
	// e5 starts a month after it is assigned.
	add("e5", fx.sday, fx.stime.Add(30*ftdDay), delta5, delta5)
	for _, e := range ftdAll(t, orm.Query[expExperiment](db)) {
		fx.expnames = append(fx.expnames, e.Name)
	}
	return fx
}

// ftdAll returns the rows of qs.
func ftdAll(t *testing.T, qs *orm.QuerySet[expExperiment]) []expExperiment {
	t.Helper()
	rows, err := qs.All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	return rows
}

// ftdAnnotated returns the rows of qs with their annotations.
func ftdAnnotated[T any](t *testing.T, qs *orm.QuerySet[T]) []orm.Annotated[T] {
	t.Helper()
	rows, err := qs.AllAnnotated(context.Background())
	if err != nil {
		t.Fatalf("AllAnnotated: %v", err)
	}
	return rows
}

// ftdFirstAnnotation returns the annotation name of the first row of qs.
func ftdFirstAnnotation[T any](t *testing.T, qs *orm.QuerySet[T], name string) any {
	t.Helper()
	rows := ftdAnnotated(t, qs.Limit(1))
	if len(rows) == 0 {
		t.Fatal("the queryset has no rows")
	}
	return rows[0].Annotations[name]
}

// ftdName is the name of an experiment.
func ftdName(e expExperiment) string { return e.Name }

// ftdNull is Django's Value(None, output_field=...): a NULL of the given kind.
func ftdNull(kind orm.Kind) orm.Expr {
	return orm.ExpressionWrapper(orm.Value(nil), orm.Field{Kind: kind})
}

// ftdAssertTime fails when got is not a time.Time equal to want.
func ftdAssertTime(t *testing.T, got any, want time.Time) {
	t.Helper()
	if g, ok := got.(time.Time); !ok || !g.Equal(want) {
		t.Errorf("got %v (%T), want %v", got, got, want)
	}
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_multiple_query_compilation
func TestFTimeDeltaMultipleQueryCompilation(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		queryset := orm.Query[expExperiment](db).Filter("end__lt", orm.F("start").Add(time.Hour))
		q1, _, err := orm.CompileSelectForTest(queryset)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		q2, _, err := orm.CompileSelectForTest(queryset)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		djangotest.AssertEqual(t, q2, q1)
	})
}

// Django's qs.all() is a clone; Filter without conditions is one here.
//
// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_query_clone
func TestFTimeDeltaQueryClone(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db).Filter("end__lt", orm.F("start").Add(time.Hour))
		qs2 := qs.Filter()
		ftdAll(t, qs)
		ftdAll(t, qs2)
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_delta_add
func TestFTimeDeltaDeltaAdd(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		for i, delta := range fx.deltas {
			djangotest.AssertQuerySetEqual(t, qs.Filter("end__lt", orm.F("start").Add(delta)),
				fx.expnames[:i], ftdName)
			djangotest.AssertQuerySetEqual(t, qs.Filter("end__lt", orm.Value(delta).Add(orm.F("start"))),
				fx.expnames[:i], ftdName)
			djangotest.AssertQuerySetEqual(t, qs.Filter("end__lte", orm.F("start").Add(delta)),
				fx.expnames[:i+1], ftdName)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_delta_subtract
func TestFTimeDeltaDeltaSubtract(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		for i, delta := range fx.deltas {
			djangotest.AssertQuerySetEqual(t, qs.Filter("start__gt", orm.F("end").Sub(delta)),
				fx.expnames[:i], ftdName)
			djangotest.AssertQuerySetEqual(t, qs.Filter("start__gte", orm.F("end").Sub(delta)),
				fx.expnames[:i+1], ftdName)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_exclude
func TestFTimeDeltaExclude(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		for i, delta := range fx.deltas {
			djangotest.AssertQuerySetEqual(t, qs.Exclude("end__lt", orm.F("start").Add(delta)),
				fx.expnames[i:], ftdName)
			djangotest.AssertQuerySetEqual(t, qs.Exclude("end__lte", orm.F("start").Add(delta)),
				fx.expnames[i+1:], ftdName)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_date_comparison
func TestFTimeDeltaDateComparison(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		for i, days := range fx.daysLong {
			djangotest.AssertQuerySetEqual(t, qs.Filter("completed__lt", orm.F("assigned").Add(days)),
				fx.expnames[:i], ftdName)
			djangotest.AssertQuerySetEqual(t, qs.Filter("completed__lte", orm.F("assigned").Add(days)),
				fx.expnames[:i+1], ftdName)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_datetime_and_durationfield_addition_with_filter
func TestFTimeDeltaDatetimeAndDurationfieldAdditionWithFilter(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		testSet := orm.Query[expExperiment](db).Filter("end", orm.F("start").Add(orm.F("estimated_time")))
		n, err := testSet.Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if n == 0 {
			t.Fatal("no experiment ends at its start plus its estimated time")
		}
		var want []string
		for _, e := range ftdAll(t, orm.Query[expExperiment](db)) {
			if e.End.Equal(e.Start.Add(e.EstimatedTime)) {
				want = append(want, e.Name)
			}
		}
		djangotest.AssertQuerySetEqual(t, testSet, want, ftdName)
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_datetime_and_duration_field_addition_with_annotate_and_no_output_field
func TestFTimeDeltaDatetimeAndDurationFieldAdditionWithAnnotateAndNoOutputField(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		testSet := orm.Query[expExperiment](db).Annotate(
			"estimated_end", orm.F("start").Add(orm.F("estimated_time")))
		for _, row := range ftdAnnotated(t, testSet) {
			e := row.Object
			ftdAssertTime(t, row.Annotations["estimated_end"], e.Start.Add(e.EstimatedTime))
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_datetime_subtraction_with_annotate_and_no_output_field
func TestFTimeDeltaDatetimeSubtractionWithAnnotateAndNoOutputField(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		testSet := orm.Query[expExperiment](db).Annotate(
			"calculated_duration", orm.F("end").Sub(orm.F("start")))
		for _, row := range ftdAnnotated(t, testSet) {
			e := row.Object
			djangotest.AssertEqual(t, row.Annotations["calculated_duration"], any(e.End.Sub(e.Start)))
		}
	})
}

// SQLite compares the date with the datetime as text, where the date is a
// prefix of the datetime on the same day, so Django marks the test an
// expected failure there.
//
// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_mixed_comparisons1
func TestFTimeDeltaMixedComparisons1(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		if expDialectIsSQLite(db) {
			t.Skip("Django expects this test to fail on SQLite, which compares a date with a datetime as text")
		}
		fx := ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		for i, delay := range fx.delays {
			djangotest.AssertQuerySetEqual(t, qs.Filter("assigned__gt", orm.F("start").Sub(delay)),
				fx.expnames[:i], ftdName)
			djangotest.AssertQuerySetEqual(t, qs.Filter("assigned__gte", orm.F("start").Sub(delay)),
				fx.expnames[:i+1], ftdName)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_mixed_comparisons2
func TestFTimeDeltaMixedComparisons2(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		for i, delay := range fx.delays {
			delay = delay / ftdDay * ftdDay
			djangotest.AssertQuerySetEqual(t, qs.Filter("start__lt", orm.F("assigned").Add(delay)),
				fx.expnames[:i], ftdName)
			djangotest.AssertQuerySetEqual(t, qs.Filter("start__lte", orm.F("assigned").Add(delay).Add(ftdDay)),
				fx.expnames[:i+1], ftdName)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_delta_update
func TestFTimeDeltaDeltaUpdate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		for _, delta := range fx.deltas {
			exps := ftdAll(t, orm.Query[expExperiment](db))
			if _, err := orm.Query[expExperiment](db).Update(context.Background(),
				"start", orm.F("start").Add(delta), "end", orm.F("end").Add(delta)); err != nil {
				t.Fatalf("Update: %v", err)
			}
			updated := ftdAll(t, orm.Query[expExperiment](db))
			if len(updated) != len(exps) {
				t.Fatalf("got %d experiments, want %d", len(updated), len(exps))
			}
			for i, e := range exps {
				got := updated[i]
				ftdAssertTime(t, got.Start, e.Start.Add(delta))
				ftdAssertTime(t, got.End, e.End.Add(delta))
				djangotest.AssertEqual(t, got.End.Sub(got.Start), e.End.Sub(e.Start))
			}
		}
	})
}

// Django expects a DatabaseError: SQLite has no product of a datetime and a
// duration, and PostgreSQL has no such operator.
//
// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_invalid_operator
func TestFTimeDeltaInvalidOperator(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		_, err := orm.Query[expExperiment](db).Filter("start", orm.F("start").Mul(time.Duration(0))).
			All(context.Background())
		if err == nil {
			t.Fatal("a datetime times a duration should fail")
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_durationfield_add
func TestFTimeDeltaDurationfieldAdd(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		djangotest.AssertQuerySetEqual(t, qs.Filter("start", orm.F("start").Add(orm.F("estimated_time"))),
			[]string{"e0"}, ftdName)
		djangotest.AssertQuerySetEqual(t, qs.Filter("end__lt", orm.F("start").Add(orm.F("estimated_time"))),
			[]string{"e2"}, ftdName)
		djangotest.AssertQuerySetEqual(t,
			qs.Filter("end__gte", orm.F("start").Add(orm.F("estimated_time")).Add(time.Hour)),
			[]string{"e4"}, ftdName)

		queryset := qs.Annotate("shifted", orm.ExpressionWrapper(
			orm.F("start").Add(ftdNull(orm.KindDuration)), orm.Field{Kind: orm.KindDateTime}))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "shifted"), nil)
	})
}

// Python's timedelta * 3.2 and timedelta / 3.2 round to the nearest
// microsecond, half to even, as the expected values here do.
//
// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_durationfield_multiply_divide
func TestFTimeDeltaDurationfieldMultiplyDivide(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		if _, err := orm.Query[expExperiment](db).Update(context.Background(), "scalar", 2); err != nil {
			t.Fatalf("Update: %v", err)
		}
		scale := func(d time.Duration, f func(float64) float64) time.Duration {
			return time.Duration(math.RoundToEven(f(float64(d.Microseconds())))) * time.Microsecond
		}
		tests := []struct {
			name   string
			expr   orm.Expr
			scalar float64
		}{
			{"Decimal", orm.Value(orm.MustDecimal("2")), 2},
			{"F", orm.F("scalar"), 2},
			{"int", orm.Value(2), 2},
			{"float", orm.Value(3.2), 3.2},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				qs := orm.Query[expExperiment](db).Annotate(
					"multiplied", orm.ExpressionWrapper(tc.expr.Mul(orm.F("estimated_time")),
						orm.Field{Kind: orm.KindDuration}),
					"divided", orm.ExpressionWrapper(orm.F("estimated_time").Div(tc.expr),
						orm.Field{Kind: orm.KindDuration}),
				)
				for _, row := range ftdAnnotated(t, qs) {
					est := row.Object.EstimatedTime
					djangotest.AssertEqual(t, row.Annotations["multiplied"],
						any(scale(est, func(us float64) float64 { return us * tc.scalar })))
					djangotest.AssertEqual(t, row.Annotations["divided"],
						any(scale(est, func(us float64) float64 { return us / tc.scalar })))
				}
			})
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_duration_expressions
func TestFTimeDeltaDurationExpressions(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		for _, delta := range fx.deltas {
			qs := orm.Query[expExperiment](db).Annotate("duration", orm.F("estimated_time").Add(delta))
			for _, row := range ftdAnnotated(t, qs) {
				djangotest.AssertEqual(t, row.Annotations["duration"], any(row.Object.EstimatedTime+delta))
			}
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_date_subtraction
func TestFTimeDeltaDateSubtraction(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		queryset := orm.Query[expExperiment](db).Annotate(
			"completion_duration", orm.F("completed").Sub(orm.F("assigned")))
		djangotest.AssertQuerySetEqual(t, queryset.Filter("completion_duration__gte", 5*ftdDay),
			[]string{"e3", "e4", "e5"}, ftdName, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, queryset.Filter("completion_duration__gte", 120*ftdDay),
			[]string{"e5"}, ftdName, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, queryset.Filter("completion_duration__lt", 5*ftdDay),
			[]string{"e0", "e1", "e2"}, ftdName, djangotest.Unordered())

		queryset = orm.Query[expExperiment](db).Annotate(
			"difference", orm.F("completed").Sub(ftdNull(orm.KindDate)))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "difference"), nil)

		queryset = orm.Query[expExperiment](db).Annotate("shifted", orm.ExpressionWrapper(
			orm.F("completed").Sub(ftdNull(orm.KindDuration)), orm.Field{Kind: orm.KindDate}))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "shifted"), nil)
	})
}

// No Go counterpart: tests/expressions/tests.py::FTimeDeltaTests.test_date_subquery_subtraction
// needs Subquery and OuterRef, which the ORM does not have.

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_date_case_subtraction
func TestFTimeDeltaDateCaseSubtraction(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		// Case takes no output field; ExpressionWrapper gives it one, as
		// Django's Case(..., output_field=DateField()) does.
		queryset := orm.Query[expExperiment](db).Annotate(
			"date_case", orm.ExpressionWrapper(
				orm.Case(orm.When{Cond: orm.Q("name", "e0"), Then: orm.F("completed")}),
				orm.Field{Kind: orm.KindDate}),
			"completed_value", orm.Value(fx.e0.Completed),
			"difference", orm.F("date_case").Sub(orm.F("completed_value")),
		).Filter("difference", time.Duration(0))
		e, err := queryset.Get(context.Background())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, e.ID, fx.e0.ID)
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_time_subtraction
func TestFTimeDeltaTimeSubtraction(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &expTime{Time: new(orm.NewTime(12, 30, 15, 2345000))})
		queryset := orm.Query[expTime](db).Annotate(
			"difference", orm.F("time").Sub(orm.Value(orm.NewTime(11, 15, 0, 0))))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "difference"),
			any(time.Hour+15*time.Minute+15*time.Second+2345*time.Microsecond))

		queryset = orm.Query[expTime](db).Annotate("difference", orm.F("time").Sub(ftdNull(orm.KindTime)))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "difference"), nil)

		queryset = orm.Query[expTime](db).Annotate("shifted", orm.ExpressionWrapper(
			orm.F("time").Sub(ftdNull(orm.KindDuration)), orm.Field{Kind: orm.KindTime}))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "shifted"), nil)
	})
}

// No Go counterpart: tests/expressions/tests.py::FTimeDeltaTests.test_time_subquery_subtraction
// needs Subquery and OuterRef, which the ORM does not have.

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_datetime_subtraction
func TestFTimeDeltaDatetimeSubtraction(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		qs := orm.Query[expExperiment](db)
		djangotest.AssertQuerySetEqual(t, qs.Filter("estimated_time__gt", orm.F("end").Sub(orm.F("start"))),
			[]string{"e2"}, ftdName)
		djangotest.AssertQuerySetEqual(t, qs.Filter("estimated_time__lt", orm.F("end").Sub(orm.F("start"))),
			[]string{"e4"}, ftdName)

		queryset := qs.Annotate("difference", orm.F("start").Sub(ftdNull(orm.KindDateTime)))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "difference"), nil)

		queryset = qs.Annotate("shifted", orm.ExpressionWrapper(
			orm.F("start").Sub(ftdNull(orm.KindDuration)), orm.Field{Kind: orm.KindDateTime}))
		djangotest.AssertEqual(t, ftdFirstAnnotation(t, queryset, "shifted"), nil)
	})
}

// No Go counterpart: tests/expressions/tests.py::FTimeDeltaTests.test_datetime_subquery_subtraction
// needs Subquery and OuterRef, which the ORM does not have.

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_datetime_subtraction_microseconds
func TestFTimeDeltaDatetimeSubtractionMicroseconds(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		delta := 8999999999999999 * time.Microsecond
		if _, err := orm.Query[expExperiment](db).Update(context.Background(),
			"end", orm.F("start").Add(delta)); err != nil {
			t.Fatalf("Update: %v", err)
		}
		qs := orm.Query[expExperiment](db).Annotate("delta", orm.F("end").Sub(orm.F("start")))
		for _, row := range ftdAnnotated(t, qs) {
			djangotest.AssertEqual(t, row.Annotations["delta"], any(delta))
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_duration_with_datetime
func TestFTimeDeltaDurationWithDatetime(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := ftdSetUp(t, db)
		// Leave out e1, whose microseconds not every backend stores.
		overEstimate := orm.Query[expExperiment](db).Exclude("name", "e1").
			Filter("completed__gt", orm.Value(fx.stime).Add(orm.F("estimated_time"))).
			OrderBy("name")
		djangotest.AssertQuerySetEqual(t, overEstimate, []string{"e3", "e4", "e5"}, ftdName)
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_duration_with_datetime_microseconds
func TestFTimeDeltaDurationWithDatetimeMicroseconds(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		delta := 8999999999999999 * time.Microsecond
		qs := orm.Query[expExperiment](db).Annotate("dt", orm.ExpressionWrapper(
			orm.F("start").Add(delta), orm.Field{Kind: orm.KindDateTime}))
		for _, row := range ftdAnnotated(t, qs) {
			ftdAssertTime(t, row.Annotations["dt"], row.Object.Start.Add(delta))
		}
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_date_minus_duration
func TestFTimeDeltaDateMinusDuration(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		moreThan4Days := orm.Query[expExperiment](db).Filter(
			"assigned__lt", orm.F("completed").Sub(orm.Value(4*ftdDay)))
		djangotest.AssertQuerySetEqual(t, moreThan4Days, []string{"e3", "e4", "e5"}, ftdName)
	})
}

// Ported from Django: tests/expressions/tests.py::FTimeDeltaTests.test_negative_timedelta_update
func TestFTimeDeltaNegativeTimedeltaUpdate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ftdSetUp(t, db)
		ctx := context.Background()
		// Subtract 30 seconds, 30 minutes, 2 hours and 2 days.
		experiments := orm.Query[expExperiment](db).Filter("name", "e0").
			Annotate("start_sub_seconds", orm.F("start").Add(-30*time.Second)).
			Annotate("start_sub_minutes", orm.F("start_sub_seconds").Add(-30*time.Minute)).
			Annotate("start_sub_hours", orm.F("start_sub_minutes").Add(-2*time.Hour)).
			Annotate("new_start", orm.F("start_sub_hours").Add(-2*ftdDay))
		expectedStart := time.Date(2010, 6, 23, 9, 45, 0, 0, time.UTC)
		// Subtract 30 microseconds.
		experiments = experiments.Annotate("new_start", orm.F("new_start").Add(-30*time.Microsecond))
		expectedStart = expectedStart.Add(746970 * time.Microsecond)
		if _, err := experiments.Update(ctx, "start", orm.F("new_start")); err != nil {
			t.Fatalf("Update: %v", err)
		}
		e0, err := orm.Query[expExperiment](db).Get(ctx, "name", "e0")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		ftdAssertTime(t, e0.Start, expectedStart)
	})
}
