package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of the tests of Django's tests/queries/tests.py on none() and on
// the ordered attribute of a queryset.

// qoTag is the Tag model of QuerysetOrderedTests, which Django orders by
// name.
type qoTag struct {
	ID   int64
	Name string `orm:"max_length=10"`
}

// Meta orders the tags by name, as Django's Tag does.
func (qoTag) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// qoAnnotation is the Annotation model of QuerysetOrderedTests, which has no
// default ordering.
type qoAnnotation struct {
	ID   int64
	Name string `orm:"max_length=10"`
	Tag  orm.FK[qoTag]
}

// qoDB returns a database with the models of QuerysetOrderedTests.
func qoDB(t *testing.T) *orm.DB {
	t.Helper()
	return djangotest.NewDB(t, djangotest.App("queries", &qoTag{}, &qoAnnotation{}))
}

// Ported from Django: tests/queries/tests.py::QuerysetOrderedTests.test_no_default_or_explicit_ordering
func TestQuerysetOrderedNoDefaultOrExplicitOrdering(t *testing.T) {
	db := qoDB(t)
	djangotest.AssertEqual(t, orm.Query[qoAnnotation](db).Ordered(), false)
}

// Ported from Django: tests/queries/tests.py::QuerysetOrderedTests.test_cleared_default_ordering
func TestQuerysetOrderedClearedDefaultOrdering(t *testing.T) {
	db := qoDB(t)
	djangotest.AssertEqual(t, orm.Query[qoTag](db).Ordered(), true)
	djangotest.AssertEqual(t, orm.Query[qoTag](db).OrderBy().Ordered(), false)
}

// Ported from Django: tests/queries/tests.py::QuerysetOrderedTests.test_explicit_ordering
func TestQuerysetOrderedExplicitOrdering(t *testing.T) {
	db := qoDB(t)
	djangotest.AssertEqual(t, orm.Query[qoAnnotation](db).OrderBy("id").Ordered(), true)
}

// Ported from Django: tests/queries/tests.py::QuerysetOrderedTests.test_empty_queryset
func TestQuerysetOrderedEmptyQueryset(t *testing.T) {
	db := qoDB(t)
	djangotest.AssertEqual(t, orm.Query[qoAnnotation](db).None().Ordered(), true)
}

// No Go counterpart: tests/queries/tests.py::QuerysetOrderedTests.test_order_by_extra
// orders by extra(order_by=...); the ORM has no extra().

// Ported from Django: tests/queries/tests.py::QuerysetOrderedTests.test_annotated_ordering
//
// Annotation has no many-to-many notes here; the count of its tag stands in
// for the count of its notes.
func TestQuerysetOrderedAnnotatedOrdering(t *testing.T) {
	db := qoDB(t)
	qs := orm.Query[qoAnnotation](db).Annotate("num_notes", orm.Count("tag"))
	djangotest.AssertEqual(t, qs.Ordered(), false)
	djangotest.AssertEqual(t, qs.OrderBy("num_notes").Ordered(), true)
}

// Ported from Django: tests/queries/tests.py::QuerysetOrderedTests.test_annotated_default_ordering
func TestQuerysetOrderedAnnotatedDefaultOrdering(t *testing.T) {
	db := qoDB(t)
	qs := orm.Query[qoTag](db).Annotate("num_notes", orm.Count("pk"))
	djangotest.AssertEqual(t, qs.Ordered(), false)
	djangotest.AssertEqual(t, qs.OrderBy("name").Ordered(), true)
}

// Ported from Django: tests/queries/tests.py::QuerysetOrderedTests.test_annotated_values_default_ordering
func TestQuerysetOrderedAnnotatedValuesDefaultOrdering(t *testing.T) {
	db := qoDB(t)
	qs := orm.Query[qoTag](db).Values("name").Annotate("num_notes", orm.Count("pk"))
	djangotest.AssertEqual(t, qs.Ordered(), false)
	djangotest.AssertEqual(t, qs.OrderBy("name").Ordered(), true)
}

// Ported from Django: tests/queries/tests.py::Queries1Tests.test_ticket7235
//
// Django's Eaten has a foreign key to Food; a note with its tag stands in.
// The calls of complex_filter, extra, defer and only are left out: the ORM
// has none of them, and Filter takes the condition of complex_filter.
func TestQueriesTicket7235(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &qNote{Note: "m", Misc: "m"})
		q := orm.Query[qNote](db).None()
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertQuerySetEqual(t, q, []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.Filter("note", "m"), []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.Exclude("note", "m"), []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.Filter("pk", 1), []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.SelectRelated("tag"), []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.Annotate(orm.Count("tag")), []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.OrderBy("note", "tag"), []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.Distinct(), []string{}, qNoteName)
			djangotest.AssertQuerySetEqual(t, q.Reverse(), []string{}, qNoteName)
		})
	})
}

// Ported from Django: tests/queries/tests.py::EmptyQuerySetTests.test_emptyqueryset_values
func TestEmptyQuerySetValues(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		rows, err := orm.Query[qNumber](db).None().Values("num").OrderBy("num").All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertEqual(t, len(rows), 0)
	})
}

// Ported from Django: tests/queries/tests.py::EmptyQuerySetTests.test_values_subquery
func TestEmptyQuerySetValuesSubquery(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &qNumber{Num: 1})
		numbers := orm.Query[qNumber](db)
		djangotest.AssertQuerySetEqual(t, numbers.Filter("pk__in", numbers.None().Values("pk")), []int64{}, qNumberNum)
		djangotest.AssertQuerySetEqual(t, numbers.Filter("pk__in", numbers.None().ValuesList("pk")), []int64{}, qNumberNum)
	})
}

// Ported from Django: tests/queries/tests.py::EmptyQuerySetTests.test_ticket_19151
func TestEmptyQuerySetTicket19151(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		q := orm.Query[qAuthor](db).None()
		rows, err := q.Values().All(ctx)
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		djangotest.AssertEqual(t, len(rows), 0)
		tuples, err := q.ValuesList().All(ctx)
		if err != nil {
			t.Fatalf("ValuesList: %v", err)
		}
		djangotest.AssertEqual(t, len(tuples), 0)
	})
}
