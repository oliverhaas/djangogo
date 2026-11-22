package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// nqPoll is Poll of Django's tests/null_queries/models.py.
type nqPoll struct {
	ID       int64
	Question string `orm:"max_length=200"`
}

// nqChoice is Choice of Django's tests/null_queries/models.py.
type nqChoice struct {
	ID     int64
	Poll   orm.FK[nqPoll]
	Choice string `orm:"max_length=200;null"`
}

// nqOuterA is OuterA of Django's tests/null_queries/models.py.
type nqOuterA struct {
	ID int64
}

// nqOuterB is OuterB of Django's tests/null_queries/models.py.
type nqOuterB struct {
	ID   int64
	Data string `orm:"max_length=10"`
}

// nqInner is Inner of Django's tests/null_queries/models.py. Its foreign keys
// keep Django's reverse query name "inner", which the Go type name does not
// give.
type nqInner struct {
	ID    int64
	First orm.FK[nqOuterA] `orm:"related_query_name=inner"`
	Third orm.FK[nqOuterB] `orm:"null;on_delete=set_null;related_query_name=inner"`
}

// nqModels returns the models of the null_queries app.
func nqModels() []any {
	return []any{djangotest.App("null_queries", &nqPoll{}, &nqChoice{}, &nqOuterA{}, &nqOuterB{}, &nqInner{})}
}

// Ported from Django: tests/null_queries/tests.py::NullQueriesTests.test_none_as_null
func TestNullQueriesNoneAsNull(t *testing.T) {
	djangotest.EachDB(t, nqModels(), func(t *testing.T, db *orm.DB) {
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
	djangotest.EachDB(t, nqModels(), func(t *testing.T, db *orm.DB) {
		poll := &nqPoll{Question: "How?"}
		_, err := orm.RelatedSet[nqChoice](db, poll, "nqchoice_set").All(context.Background())
		assertErrorContains(t, err,
			"'nqPoll' instance needs to have a primary key value before this relationship can be used")
	})
}

// Querying across reverse relations and then another relation should insert
// outer joins correctly so as not to exclude results.
//
// Ported from Django: tests/null_queries/tests.py::NullQueriesTests.test_reverse_relations
func TestNullQueriesReverseRelations(t *testing.T) {
	djangotest.EachDB(t, nqModels(), func(t *testing.T, db *orm.DB) {
		id := func(o nqOuterA) int64 { return o.ID }
		obj := create(t, db, &nqOuterA{})
		djangotest.AssertQuerySetEqual(t, orm.Query[nqOuterA](db).Filter("inner__third", nil), []int64{obj.ID}, id)
		djangotest.AssertQuerySetEqual(t, orm.Query[nqOuterA](db).Filter("inner__third__data", nil), []int64{obj.ID}, id)

		inner := create(t, db, &nqInner{First: fkTo[nqOuterA](obj.ID)})
		djangotest.AssertQuerySetEqual(t, orm.Query[nqInner](db).Filter("first__inner__third", nil),
			[]int64{inner.ID}, func(i nqInner) int64 { return i.ID })

		// Ticket #13815: check if <reverse>_isnull=False does not produce
		// faulty empty lists.
		outerb := create(t, db, &nqOuterB{Data: "reverse"})
		bID := func(o nqOuterB) int64 { return o.ID }
		djangotest.AssertQuerySetEqual(t, orm.Query[nqOuterB](db).Filter("inner__isnull", false), []int64{}, bID)
		create(t, db, &nqInner{First: fkTo[nqOuterA](obj.ID)})
		djangotest.AssertQuerySetEqual(t, orm.Query[nqOuterB](db).Exclude("inner__isnull", false),
			[]int64{outerb.ID}, bID)
	})
}
