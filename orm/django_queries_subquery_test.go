package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of the tests in Django's tests/queries/tests.py on conditions
// whose outcome is known without the database, on sliced subqueries and on
// slices of no rows. Django slices a queryset with [low:high]; here that is
// Offset(low).Limit(high - low).

// qWhere compiles a condition tree on the SQLite dialect.
func qWhere(t *testing.T, n orm.WhereForTest) string {
	t.Helper()
	return orm.CompileWhereForTest(djangotest.NewDB(t).Dialect(), n)
}

// qLeaves returns one leaf per kind given.
func qLeaves(kinds ...string) []orm.WhereForTest {
	out := make([]orm.WhereForTest, len(kinds))
	for i, kind := range kinds {
		out[i] = orm.WhereForTest{Leaf: kind}
	}
	return out
}

// Ported from Django: tests/queries/tests.py::WhereNodeTest.test_empty_full_handling_conjunction
//
// Django's dummy node renders as "dummy"; the leaf here is a real condition,
// "dummy IS NULL".
func TestQueriesWhereEmptyFullHandlingConjunction(t *testing.T) {
	const dummy, nothing = orm.WhereDummy, orm.WhereNothing
	for _, tc := range []struct {
		leaves        []string
		want, negated string
	}{
		{[]string{nothing}, orm.WhereMatchesNothing, orm.WhereMatchesAll},
		{[]string{dummy, dummy}, "(dummy IS NULL AND dummy IS NULL)", "NOT (dummy IS NULL AND dummy IS NULL)"},
		{[]string{nothing, dummy}, orm.WhereMatchesNothing, orm.WhereMatchesAll},
	} {
		w := orm.WhereForTest{Children: qLeaves(tc.leaves...)}
		djangotest.AssertEqual(t, qWhere(t, w), tc.want)
		w.Negated = true
		djangotest.AssertEqual(t, qWhere(t, w), tc.negated)
	}
}

// Ported from Django: tests/queries/tests.py::WhereNodeTest.test_empty_full_handling_disjunction
func TestQueriesWhereEmptyFullHandlingDisjunction(t *testing.T) {
	const dummy, nothing = orm.WhereDummy, orm.WhereNothing
	for _, tc := range []struct {
		leaves        []string
		want, negated string
	}{
		{[]string{nothing}, orm.WhereMatchesNothing, orm.WhereMatchesAll},
		{[]string{dummy, dummy}, "(dummy IS NULL OR dummy IS NULL)", "NOT (dummy IS NULL OR dummy IS NULL)"},
		{[]string{nothing, dummy}, "dummy IS NULL", "NOT (dummy IS NULL)"},
	} {
		w := orm.WhereForTest{Or: true, Children: qLeaves(tc.leaves...)}
		djangotest.AssertEqual(t, qWhere(t, w), tc.want)
		w.Negated = true
		djangotest.AssertEqual(t, qWhere(t, w), tc.negated)
	}
}

// Ported from Django: tests/queries/tests.py::WhereNodeTest.test_empty_nodes
func TestQueriesWhereEmptyNodes(t *testing.T) {
	// A node without children restricts nothing.
	emptyNode := orm.WhereForTest{}
	nothing := orm.WhereForTest{Leaf: orm.WhereNothing}
	w := orm.WhereForTest{Children: []orm.WhereForTest{emptyNode, emptyNode}}
	djangotest.AssertEqual(t, qWhere(t, w), orm.WhereMatchesAll)
	w.Negated = true
	djangotest.AssertEqual(t, qWhere(t, w), orm.WhereMatchesNothing)
	w.Or = true
	djangotest.AssertEqual(t, qWhere(t, w), orm.WhereMatchesNothing)
	w.Negated = false
	djangotest.AssertEqual(t, qWhere(t, w), orm.WhereMatchesAll)

	w = orm.WhereForTest{Or: true, Children: []orm.WhereForTest{emptyNode, nothing}}
	djangotest.AssertEqual(t, qWhere(t, w), orm.WhereMatchesAll)
	w.Or = false
	djangotest.AssertEqual(t, qWhere(t, w), orm.WhereMatchesNothing)
}

// qSetUpCategories creates the rows of SubqueryTests.setUpTestData: four
// categories with the primary keys 1 to 4.
func qSetUpCategories(t *testing.T, db *orm.DB) *orm.QuerySet[qNamedCategory] {
	t.Helper()
	for i, name := range []string{"first", "second", "third", "fourth"} {
		create(t, db, &qNamedCategory{ID: int64(i + 1), Name: name})
	}
	return orm.Query[qNamedCategory](db)
}

// qAssertIDs checks the primary keys of the rows of qs, in any order unless
// ordered is set.
func qAssertIDs(t *testing.T, qs *orm.QuerySet[qNamedCategory], ordered bool, want ...int64) {
	t.Helper()
	got, err := orm.Pluck[int64](context.Background(), qs, "id")
	if err != nil {
		t.Errorf("Pluck: %v", err)
		return
	}
	if ordered {
		djangotest.AssertSequenceEqual(t, got, want)
		return
	}
	djangotest.AssertSequenceEqual(t, got, want, djangotest.Unordered())
}

// Ported from Django: tests/queries/tests.py::SubqueryTests.test_ordered_subselect
func TestQueriesOrderedSubselect(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		cats := qSetUpCategories(t, db)
		// A sliced subselect keeps its ordering, which decides the rows it holds.
		newest := cats.OrderBy("-id")
		qAssertIDs(t, cats.Filter("id__in", newest.Offset(0).Limit(2)), false, 3, 4)
		qAssertIDs(t, cats.Filter("id__in", newest.Limit(2)), false, 3, 4)
		qAssertIDs(t, cats.Filter("id__in", newest.Offset(1).Limit(1)), false, 3)
		qAssertIDs(t, cats.Filter("id__in", newest.Offset(2)), false, 1, 2)
	})
}

// Ported from Django: tests/queries/tests.py::SubqueryTests.test_slice_subquery_and_query
func TestQueriesSliceSubqueryAndQuery(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		cats := qSetUpCategories(t, db)
		newest := cats.OrderBy("-id")
		qAssertIDs(t, cats.Filter("id__in", newest.Limit(2)).OrderBy("id").Limit(2), true, 3, 4)
		qAssertIDs(t, cats.Filter("id__in", newest.Offset(1).Limit(2)).OrderBy("id").Offset(1).Limit(2), true, 3)
		qAssertIDs(t, cats.Filter("id__in", newest.Offset(2)).OrderBy("id").Offset(1), true, 2)
	})
}

// Ported from Django: tests/queries/tests.py::SubqueryTests.test_sliced_delete
func TestQueriesSlicedDelete(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		cats := qSetUpCategories(t, db)
		newest := cats.OrderBy("-id")
		remove := func(sub *orm.QuerySet[qNamedCategory]) {
			t.Helper()
			if _, err := cats.Filter("id__in", sub).Delete(context.Background()); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		}
		// A delete may be restricted by a sliced subquery.
		remove(newest.Limit(1))
		qAssertIDs(t, cats, false, 1, 2, 3)
		remove(newest.Offset(1).Limit(1))
		qAssertIDs(t, cats, false, 1, 3)
		remove(newest.Offset(1))
		qAssertIDs(t, cats, false, 3)
	})
}

// Ported from Django: tests/queries/tests.py::WeirdQuerysetSlicingTests.test_tickets_7698_10202
//
// The assertions on a slice of a slice and on latest() are not ported: Limit
// replaces an earlier limit, and the ORM has no latest().
func TestQueriesTickets7698And10202(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &qArticle{Name: "one"})
		articles := orm.Query[qArticle](db)
		// A slice of no rows is known to be empty, so no query runs.
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertEqual(t, len(mustAll(t, articles.Limit(0))), 0)
			djangotest.AssertEqual(t, mustCount(t, articles.Limit(0)), 0)
		})
	})
}

// Ported from Django: tests/queries/tests.py::WeirdQuerysetSlicingTests.test_empty_sliced_subquery
//
// Django's Eaten and Food are joined through to_field; Item and Note stand in
// for them.
func TestQueriesEmptySlicedSubquery(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		qs := orm.Query[qItem](db).Filter("note__in", orm.Query[qNote](db).Limit(0))
		djangotest.AssertEqual(t, mustCount(t, qs), 0)
	})
}

// Ported from Django: tests/queries/tests.py::Queries6Tests.test_distinct_ordered_sliced_subquery_aggregation
func TestQueriesDistinctOrderedSlicedSubqueryAggregation(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		qSetUp(t, db)
		qs := orm.Query[qTag](db).Distinct().OrderBy("category__name").Limit(3)
		djangotest.AssertEqual(t, mustCount(t, qs), 3)
	})
}

// Ported from Django: tests/queries/tests.py::ValuesQuerysetTests.test_flat_values_list
//
// Django asks for the flat form with a second values_list call; here Flat
// runs the ValuesList queryset.
func TestQueriesFlatValuesList(t *testing.T) {
	djangotest.EachDB(t, qModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &qNumber{Num: 72})
		assertFlat(t, orm.Query[qNumber](db).ValuesList("num"), []any{int64(72)})
	})
}
