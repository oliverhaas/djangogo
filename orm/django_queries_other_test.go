package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of the smaller test classes of Django's tests/queries/tests.py:
// the numbered Queries classes after the first, the tests on NULL and on the
// empty string under exclude, and the tests on the SQL of Exists.

// Ported from Django: tests/queries/tests.py::Queries2Tests.test_ticket4289
func TestQueriesTicket4289(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		for _, n := range []int64{4, 8, 12} {
			create(t, db, &qNumber{Num: n})
		}
		numbers := orm.Query[qNumber](db)
		between := orm.Q("num__gt", 8, "num__lt", 12)
		below := orm.Q("num__lt", 4)
		djangotest.AssertQuerySetEqual(t, numbers.Filter("num__lt", 4), []int64{}, qNumberNum)
		djangotest.AssertQuerySetEqual(t, numbers.Filter("num__gt", 8, "num__lt", 12), []int64{}, qNumberNum)
		djangotest.AssertQuerySetEqual(t, numbers.Filter("num__gt", 8, "num__lt", 13), []int64{12}, qNumberNum)
		djangotest.AssertQuerySetEqual(t, numbers.Filter(below.Or(between)), []int64{}, qNumberNum)
		djangotest.AssertQuerySetEqual(t, numbers.Filter(between.Or(below)), []int64{}, qNumberNum)
		djangotest.AssertQuerySetEqual(t,
			numbers.Filter(orm.Q("num__gt", 8).And(orm.Q("num__lt", 12)).Or(below)), []int64{}, qNumberNum)
		djangotest.AssertQuerySetEqual(t,
			numbers.Filter(orm.Q("num__gt", 7).And(orm.Q("num__lt", 12)).Or(below)), []int64{8}, qNumberNum)
	})
}

// Ported from Django: tests/queries/tests.py::Queries4Tests.test_ticket10181
func TestQueriesTicket10181(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		tags := orm.Query[qTag](db)
		// An inner query that is known to be empty is not run, and neither is
		// the outer one.
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertQuerySetEqual(t,
				tags.Filter("id__in", tags.Filter("id__in", []int64{})), []string{}, qTagName)
		})
	})
}

// qSetUp5 creates the rows of Queries5Tests.setUpTestData and returns the
// rankings in creation order: rank 2, rank 1, rank 3.
func qSetUp5(t *testing.T, db *orm.DB) []*qRanking {
	t.Helper()
	n1 := create(t, db, &qNote{Note: "n1", Misc: "foo", Negate: true})
	n2 := create(t, db, &qNote{Note: "n2", Misc: "bar", Negate: true})
	e1 := create(t, db, &qExtraInfo{Info: "e1", Note: fkTo[qNote](n1.ID), Filterable: true})
	e2 := create(t, db, &qExtraInfo{Info: "e2", Note: fkTo[qNote](n2.ID), Filterable: true})
	a1 := create(t, db, &qAuthor{Name: "a1", Num: 1001, Extra: fkTo[qExtraInfo](e1.ID)})
	a2 := create(t, db, &qAuthor{Name: "a2", Num: 2002, Extra: fkTo[qExtraInfo](e1.ID)})
	a3 := create(t, db, &qAuthor{Name: "a3", Num: 3003, Extra: fkTo[qExtraInfo](e2.ID)})
	return []*qRanking{
		create(t, db, &qRanking{Rank: 2, Author: fkTo[qAuthor](a2.ID)}),
		create(t, db, &qRanking{Rank: 1, Author: fkTo[qAuthor](a3.ID)}),
		create(t, db, &qRanking{Rank: 3, Author: fkTo[qAuthor](a1.ID)}),
	}
}

// Ported from Django: tests/queries/tests.py::Queries5Tests.test_ticket5261
//
// The assertion on an exclusive or of two empty Q objects is not ported: Q has
// no XOR.
func TestQueriesTicket5261(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp5(t, db)
		notes := orm.Query[qNote](db).OrderBy("note")
		want := []string{"n1", "n2"}
		// An empty Q, negated or not, restricts nothing.
		djangotest.AssertQuerySetEqual(t, notes.Exclude(orm.Q()), want, qNoteName)
		djangotest.AssertQuerySetEqual(t, notes.Filter(orm.Not(orm.Q())), want, qNoteName)
		djangotest.AssertQuerySetEqual(t, notes.Filter(orm.Not(orm.Q()).Or(orm.Not(orm.Q()))), want, qNoteName)
		djangotest.AssertQuerySetEqual(t, notes.Exclude(orm.Not(orm.Q()).And(orm.Not(orm.Q()))), want, qNoteName)
	})
}

// Ported from Django: tests/queries/tests.py::Queries5Tests.test_ticket9848
func TestQueriesTicket9848(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		ranks := qSetUp5(t, db)
		ctx := context.Background()
		rankings := orm.Query[qRanking](db)
		rank := func(r qRanking) int64 { return r.Rank }
		authorStart, err := orm.Query[qAuthor](db).Get(ctx, "name", "a1")
		djangotest.AssertEqual(t, err, nil)
		rankingStart, err := rankings.Get(ctx, "author__name", "a1")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, rankingStart.ID, ranks[2].ID)

		// An update that filters only on a joined table must change the rows of
		// its own table that match, and no others.
		n, err := rankings.Filter("author__name", "a1").Update(ctx, "rank", 4636)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, n, 1)
		r, err := rankings.Get(ctx, "author__name", "a1")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, r.ID, rankingStart.ID)
		djangotest.AssertEqual(t, r.Author.PK(), authorStart.ID)
		djangotest.AssertEqual(t, r.Rank, 4636)

		r.Rank = 3
		djangotest.AssertEqual(t, rankings.Save(ctx, &r), nil)
		djangotest.AssertQuerySetEqual(t, rankings.OrderBy("author__extra__note", "author__name", "rank"),
			[]int64{3, 2, 1}, rank)
	})
}

// Ported from Django: tests/queries/tests.py::NullInExcludeTest.test_null_in_exclude_qs
//
// A NULL name reads back as the empty string, the zero value of the Go field.
func TestQueriesNullInExcludeQuerySet(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUpNullableNames(t, db)
		names := orm.Query[qNullableName](db).OrderBy("id")
		djangotest.AssertQuerySetEqual(t, names.Exclude("name__in", []string{}), []string{"i1", ""}, qNullableNameName)
		djangotest.AssertQuerySetEqual(t, names.Exclude("name__in", []string{"i1"}), []string{""}, qNullableNameName)
		djangotest.AssertQuerySetEqual(t, names.Exclude("name__in", []string{"i3"}),
			[]string{"i1", ""}, qNullableNameName)
		inner := names.Filter("name", "i1").ValuesList("name")
		// The inner queryset becomes a subquery; it does not run by itself.
		djangotest.AssertNumQueries(t, db, 1, func() {
			djangotest.AssertQuerySetEqual(t, names.Exclude("name__in", inner), []string{""}, qNullableNameName)
		})
	})
}

// qSetUpNullableNames creates the rows of NullInExcludeTest.setUpTestData: one
// name and one NULL.
func qSetUpNullableNames(t *testing.T, db *orm.DB) {
	t.Helper()
	create(t, db, &qNullableName{Name: "i1"})
	null := create(t, db, &qNullableName{})
	lkSetNull[qNullableName](t, db, null.ID, "name")
}

// Ported from Django: tests/queries/tests.py::NullInExcludeTest.test_col_not_in_list_containing_null
func TestQueriesColNotInListContainingNull(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUpNullableNames(t, db)
		names := orm.Query[qNullableName](db).OrderBy("id")
		// A nil in the list excludes the rows that hold NULL.
		djangotest.AssertQuerySetEqual(t, names.Exclude("name__in", []any{nil}), []string{"i1"}, qNullableNameName)
		djangotest.AssertQuerySetEqual(t, names.Exclude("name__in", []any{"i1", nil}), []string{}, qNullableNameName)
	})
}

// Ported from Django: tests/queries/tests.py::NullInExcludeTest.test_double_exclude
func TestQueriesNullDoubleExclude(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUpNullableNames(t, db)
		names := orm.Query[qNullableName](db).OrderBy("id")
		double := names.Filter(orm.Not(orm.Not(orm.Q("name", "i1"))))
		djangotest.AssertQuerySetEqual(t, double, []string{"i1"}, qNullableNameName)
		djangotest.AssertQuerySetEqual(t, names.Filter(orm.Q("name", "i1")), []string{"i1"}, qNullableNameName)
		// Two negations cancel out, so the NULL guard of a negation is not needed.
		sql, _, err := orm.CompileSelectForTest(double)
		djangotest.AssertEqual(t, err, nil)
		if strings.Contains(sql, "IS NOT NULL") {
			t.Errorf("a double negation kept the NULL guard: %s", sql)
		}
	})
}

// Ported from Django: tests/queries/tests.py::ExcludeTests.test_exclude_nullable_fields
func TestQueriesExcludeNullableFields(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		number := create(t, db, &qNumber{Num: 1, OtherNum: 1})
		lkSetNull[qNumber](t, db, number.ID, "another_num")
		create(t, db, &qNumber{Num: 2, OtherNum: 2, AnotherNum: 2})
		numbers := orm.Query[qNumber](db)
		// A NULL on either side of the comparison keeps the row.
		djangotest.AssertQuerySetEqual(t, numbers.Exclude("other_num", orm.F("another_num")), []int64{1}, qNumberNum)
		djangotest.AssertQuerySetEqual(t, numbers.Exclude("num", orm.F("another_num")), []int64{1}, qNumberNum)
	})
}

// Ported from Django: tests/queries/tests.py::ComparisonTests.test_ticket8597
func TestQueriesTicket8597(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		n1 := create(t, db, &qNote{Note: "n1", Misc: "foo", Negate: true})
		e1 := create(t, db, &qExtraInfo{Info: "e1", Note: fkTo[qNote](n1.ID), Filterable: true})
		a2 := create(t, db, &qAuthor{Name: "a2", Num: 2002, Extra: fkTo[qExtraInfo](e1.ID)})
		for _, name := range []string{"a_b", "x%y"} {
			create(t, db, &qItem{
				Name: name, Created: time.Now().UTC(), Creator: fkTo[qAuthor](a2.ID), Note: fkTo[qNote](n1.ID),
			})
		}
		items := orm.Query[qItem](db)
		// The LIKE wildcards in a value are matched literally.
		djangotest.AssertQuerySetEqual(t, items.Filter("name__iexact", "A_b"), []string{"a_b"}, qItemName)
		djangotest.AssertQuerySetEqual(t, items.Filter("name__iexact", "x%Y"), []string{"x%y"}, qItemName)
		djangotest.AssertQuerySetEqual(t, items.Filter("name__istartswith", "A_b"), []string{"a_b"}, qItemName)
		djangotest.AssertQuerySetEqual(t, items.Filter("name__iendswith", "A_b"), []string{"a_b"}, qItemName)
	})
}

// Ported from Django: tests/queries/tests.py::EmptyStringsAsNullTest.test_direct_exclude
func TestQueriesEmptyStringDirectExclude(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		nc := create(t, db, &qNamedCategory{Name: ""})
		pk := func(c qNamedCategory) int64 { return c.ID }
		djangotest.AssertQuerySetEqual(t,
			orm.Query[qNamedCategory](db).Exclude("name__in", []string{"nonexistent"}), []int64{nc.ID}, pk)
	})
}

// Ported from Django: tests/queries/tests.py::EmptyStringsAsNullTest.test_21001
func TestQueriesEmptyString21001(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &qNamedCategory{Name: ""})
		foo := create(t, db, &qNamedCategory{Name: "foo"})
		pk := func(c qNamedCategory) int64 { return c.ID }
		djangotest.AssertQuerySetEqual(t, orm.Query[qNamedCategory](db).Exclude("name", ""), []int64{foo.ID}, pk)
	})
}

// Ported from Django: tests/queries/tests.py::ExistsSql.test_exists
func TestQueriesExistsSQL(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		capture := djangotest.CaptureQueries(db)
		exists, err := orm.Query[qTag](db).Exists(context.Background())
		capture.Stop()
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, exists, false)
		djangotest.AssertEqual(t, capture.Len(), 1)
		// An existence check selects no column of the model.
		sql := capture.Queries()[0].SQL
		for _, column := range []string{`"id"`, `"name"`} {
			if strings.Contains(sql, column) {
				t.Errorf("Exists selects %s: %s", column, sql)
			}
		}
	})
}

// Ported from Django: tests/queries/tests.py::ExistsSql.test_ticket_18414
func TestQueriesTicket18414(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		for _, name := range []string{"one", "one", "two"} {
			create(t, db, &qArticle{Name: name, Created: time.Now().UTC()})
		}
		articles := orm.Query[qArticle](db)
		exists := func(qs *orm.QuerySet[qArticle]) bool {
			t.Helper()
			ok, err := qs.Exists(context.Background())
			if err != nil {
				t.Fatalf("Exists: %v", err)
			}
			return ok
		}
		djangotest.AssertEqual(t, exists(articles), true)
		djangotest.AssertEqual(t, exists(articles.Distinct()), true)
		// Django's [1:3] and [1:1] slices.
		djangotest.AssertEqual(t, exists(articles.Distinct().Offset(1).Limit(2)), true)
		djangotest.AssertEqual(t, exists(articles.Distinct().Offset(1).Limit(0)), false)
	})
}

// Ported from Django: tests/queries/tests.py::Ticket12807Tests.test_ticket_12807
//
// Django's Paragraph has no field of its own; LeafA stands in for it.
func TestQueriesTicket12807(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		p1 := create(t, db, &qLeafA{})
		p2 := create(t, db, &qLeafA{})
		pk := func(a qLeafA) int64 { return a.ID }
		// The negated empty "in" is always true, so the disjunction restricts
		// nothing.
		cond := orm.Q("pk", p2.ID).Or(orm.Not(orm.Q("pk__in", []int64{}))).And(orm.Q("pk", p1.ID))
		djangotest.AssertQuerySetEqual(t, orm.Query[qLeafA](db).Filter(cond), []int64{p1.ID}, pk)
	})
}

// Ported from Django: tests/queries/tests.py::DoubleInSubqueryTests.test_double_subquery_in
func TestQueriesDoubleSubqueryIn(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		lfa1 := create(t, db, &qLeafA{Data: "foo"})
		lfa2 := create(t, db, &qLeafA{Data: "bar"})
		lfb1 := create(t, db, &qLeafB{Data: "lfb1"})
		lfb2 := create(t, db, &qLeafB{Data: "lfb2"})
		create(t, db, &qJoin{A: fkTo[qLeafA](lfa1.ID), B: fkTo[qLeafB](lfb1.ID)})
		create(t, db, &qJoin{A: fkTo[qLeafA](lfa2.ID), B: fkTo[qLeafB](lfb2.ID)})
		leafAs := orm.Query[qLeafA](db).Filter("data", "foo").ValuesList("pk")
		joins := orm.Query[qJoin](db).Filter("a__in", leafAs).ValuesList("b__id")
		data := func(b qLeafB) string { return b.Data }
		djangotest.AssertNumQueries(t, db, 1, func() {
			djangotest.AssertQuerySetEqual(t, orm.Query[qLeafB](db).Filter("pk__in", joins), []string{"lfb1"}, data)
		})
	})
}

// Ported from Django: tests/queries/tests.py::Ticket18785Tests.test_ticket_18785
func TestQueriesTicket18785(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[qItem](db).Exclude("note__isnull", false).
			Filter("name", "something", "creator__extra__isnull", true)
		// The join to ExtraInfo is trimmed, the one to Author stays INNER, and
		// Note is not joined at all.
		qAssertJoins(t, qs, 1, 0)
	})
}
