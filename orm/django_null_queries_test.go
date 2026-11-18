package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/null_queries/models.py that the ported test
// uses.

type nqPoll struct {
	ID       int64
	Question string `orm:"max_length=200"`
}

type nqChoice struct {
	ID     int64
	Poll   orm.FK[nqPoll]
	Choice string `orm:"max_length=200;null"`
}

// Ported from Django: tests/null_queries/tests.py::NullQueriesTests.test_none_as_null
func TestNullQueriesNoneAsNull(t *testing.T) {
	models := []any{djangotest.App("null_queries", &nqPoll{}, &nqChoice{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		p1 := create(t, db, &nqPoll{Question: "Why?"})
		create(t, db, &nqChoice{Poll: fkTo[nqPoll](p1.ID), Choice: "Because."})
		create(t, db, &nqChoice{Poll: fkTo[nqPoll](p1.ID), Choice: "Why Not?"})
		choices := orm.Query[nqChoice](db)
		text := func(c nqChoice) string { return c.Choice }

		// A nil value means IS NULL, in exact and iexact lookups only. Every
		// choice has a text, so nothing matches.
		djangotest.AssertQuerySetEqual(t, choices.Filter("choice__exact", nil), []string{}, text)
		djangotest.AssertQuerySetEqual(t, choices.Filter("choice__iexact", nil), []string{}, text)
		// Excluding the previous result returns everything.
		djangotest.AssertQuerySetEqual(t, choices.Exclude("choice", nil).OrderBy("id"),
			[]string{"Because.", "Why Not?"}, text)

		// A valid lookup on a key that is not a field.
		_, err := choices.Filter("foo__exact", nil).All(context.Background())
		assertFieldError(t, err, "annot resolve keyword 'foo' into field. Choices are: choice, id, poll, poll_id")

		// No other lookup takes nil.
		_, err = choices.Filter("id__gt", nil).All(context.Background())
		assertErrorContains(t, err, "annot use nil as a query value")
	})
}

// Ported from Django: tests/null_queries/tests.py::NullQueriesTests.test_unsaved
func TestNullQueriesUnsaved(t *testing.T) {
	t.Skip("needs reverse related managers (poll.choice_set), which another work package adds")
}

// Ported from Django: tests/null_queries/tests.py::NullQueriesTests.test_reverse_relations
func TestNullQueriesReverseRelations(t *testing.T) {
	t.Skip("needs lookups across reverse relations (inner__third), which another work package adds")
}
