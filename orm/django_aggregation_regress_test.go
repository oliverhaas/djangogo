package orm_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of Django's tests/aggregation_regress/tests.py. They use the
// models of the aggregation tests, which have the fields these tests read.

// Ported from Django: tests/aggregation_regress/tests.py::AggregationTests.test_none_call_before_aggregate
func TestAggregationRegressNoneCallBeforeAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		vals := agAggregate(t, orm.Query[agAuthor](db).None(), orm.Avg("age"))
		agAssertMap(t, vals, map[string]any{"age__avg": nil})
	})
}
