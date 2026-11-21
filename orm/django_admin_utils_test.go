package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/admin_utils/models.py. Django's
// NestedObjects collector is orm.CollectDelete and its nested() result is
// DeletePlan.Tree.

// Cascade is Django's admin_utils.Cascade.
type Cascade struct {
	ID     int64
	Num    int16           `orm:"type=positive_smallint"`
	Parent orm.FK[Cascade] `orm:"null;on_delete=cascade"`
}

// Event is Django's admin_utils.Event.
type Event struct {
	ID int64
}

// Location is Django's admin_utils.Location.
type Location struct {
	ID    int64
	Event orm.O2O[Event] `orm:"on_delete=cascade"`
}

// Guest is Django's admin_utils.Guest.
type Guest struct {
	ID    int64
	Event orm.O2O[Event] `orm:"on_delete=cascade"`
	Name  string         `orm:"max_length=255"`
}

// EventGuide is Django's admin_utils.EventGuide.
type EventGuide struct {
	ID    int64
	Event orm.FK[Event] `orm:"on_delete=do_nothing"`
}

// adminUtilsModels returns the models of Django's admin_utils app.
func adminUtilsModels() []any {
	return []any{djangotest.App("admin_utils", &Cascade{}, &Event{}, &Location{}, &Guest{}, &EventGuide{})}
}

// nestedObjectsEnv holds the five Cascade rows of Django's
// NestedObjectsTests.setUpTestData.
type nestedObjectsEnv struct {
	db   *orm.DB
	objs []*Cascade
}

// newNestedObjectsEnv creates Cascade rows with num 0 to 4.
func newNestedObjectsEnv(t *testing.T, db *orm.DB) *nestedObjectsEnv {
	t.Helper()
	env := &nestedObjectsEnv{db: db}
	for i := range 5 {
		obj := &Cascade{Num: int16(i)}
		if err := orm.Query[Cascade](db).Create(context.Background(), obj); err != nil {
			t.Fatalf("Create: %v", err)
		}
		env.objs = append(env.objs, obj)
	}
	return env
}

// connect makes objs[j] the parent of objs[i], Django's _connect.
func (e *nestedObjectsEnv) connect(t *testing.T, i, j int) {
	t.Helper()
	e.objs[i].Parent.SetPK(e.objs[j].ID)
	if err := orm.Query[Cascade](e.db).Save(context.Background(), e.objs[i]); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// collect returns the plan of deleting the objects at indices, Django's
// _collect. Django calls collect() once per index on one collector; one
// CollectDelete with all of them gives the same roots in the same order.
func (e *nestedObjectsEnv) collect(t *testing.T, indices ...int) *orm.DeletePlan {
	t.Helper()
	pks := make([]any, len(indices))
	for k, i := range indices {
		pks[k] = e.objs[i].ID
	}
	plan, err := orm.CollectDelete(context.Background(), e.db, modelOf(t, e.db, &Cascade{}), pks)
	if err != nil {
		t.Fatalf("CollectDelete: %v", err)
	}
	return plan
}

// modelOf returns the registered model of v.
func modelOf(t *testing.T, db *orm.DB, v any) *orm.Model {
	t.Helper()
	m, ok := db.Registry().ModelOf(v)
	if !ok {
		t.Fatalf("model of %T is not registered", v)
	}
	return m
}

// nestedNums returns the tree as Django's n.nested(lambda obj: obj.num): each
// num followed by the list of its children when it has any.
func nestedNums(nodes []orm.DeleteNode) []any {
	var out []any
	for _, n := range nodes {
		out = append(out, int(n.Object.(*Cascade).Num))
		if len(n.Children) > 0 {
			out = append(out, nestedNums(n.Children))
		}
	}
	return out
}

// Ported from Django: tests/admin_utils/tests.py::NestedObjectsTests.test_unrelated_roots
func TestNestedObjectsUnrelatedRoots(t *testing.T) {
	djangotest.EachDB(t, adminUtilsModels(), func(t *testing.T, db *orm.DB) {
		env := newNestedObjectsEnv(t, db)
		env.connect(t, 2, 1)
		plan := env.collect(t, 0, 1)
		djangotest.AssertEqual(t, nestedNums(plan.Tree), []any{0, 1, []any{2}})
	})
}

// Ported from Django: tests/admin_utils/tests.py::NestedObjectsTests.test_siblings
func TestNestedObjectsSiblings(t *testing.T) {
	djangotest.EachDB(t, adminUtilsModels(), func(t *testing.T, db *orm.DB) {
		env := newNestedObjectsEnv(t, db)
		env.connect(t, 1, 0)
		env.connect(t, 2, 0)
		plan := env.collect(t, 0)
		djangotest.AssertEqual(t, nestedNums(plan.Tree), []any{0, []any{1, 2}})
	})
}

// Ported from Django: tests/admin_utils/tests.py::NestedObjectsTests.test_non_added_parent
func TestNestedObjectsNonAddedParent(t *testing.T) {
	djangotest.EachDB(t, adminUtilsModels(), func(t *testing.T, db *orm.DB) {
		env := newNestedObjectsEnv(t, db)
		env.connect(t, 0, 1)
		plan := env.collect(t, 0)
		djangotest.AssertEqual(t, nestedNums(plan.Tree), []any{0})
	})
}

// Ported from Django: tests/admin_utils/tests.py::NestedObjectsTests.test_cyclic
func TestNestedObjectsCyclic(t *testing.T) {
	djangotest.EachDB(t, adminUtilsModels(), func(t *testing.T, db *orm.DB) {
		env := newNestedObjectsEnv(t, db)
		env.connect(t, 0, 2)
		env.connect(t, 1, 0)
		env.connect(t, 2, 1)
		plan := env.collect(t, 0)
		djangotest.AssertEqual(t, nestedNums(plan.Tree), []any{0, []any{1, []any{2}}})
	})
}

// Ported from Django: tests/admin_utils/tests.py::NestedObjectsTests.test_queries
//
// Django's collect() receives the instances, so it runs only the two queries
// of the children. CollectDelete receives primary keys and adds one query
// that loads the collected Cascade rows; building the tree needs none.
func TestNestedObjectsQueries(t *testing.T) {
	djangotest.EachDB(t, adminUtilsModels(), func(t *testing.T, db *orm.DB) {
		env := newNestedObjectsEnv(t, db)
		env.connect(t, 1, 0)
		env.connect(t, 2, 0)
		// 1 query to fetch all children of 0 (1 and 2)
		// 1 query to fetch all children of 1 and 2 (none)
		// 1 query to load 0, 1 and 2
		djangotest.AssertNumQueries(t, db, 3, func() { env.collect(t, 0) })
	})
}

// Ported from Django: tests/admin_utils/tests.py::NestedObjectsTests.test_on_delete_do_nothing
//
// As in TestNestedObjectsQueries, CollectDelete adds the query that loads the
// Event.
func TestNestedObjectsOnDeleteDoNothing(t *testing.T) {
	djangotest.EachDB(t, adminUtilsModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		event := &Event{}
		if err := orm.Query[Event](db).Create(ctx, event); err != nil {
			t.Fatalf("Create event: %v", err)
		}
		guide := &EventGuide{}
		guide.Event.SetPK(event.ID)
		if err := orm.Query[EventGuide](db).Create(ctx, guide); err != nil {
			t.Fatalf("Create guide: %v", err)
		}
		eventModel := modelOf(t, db, &Event{})
		djangotest.AssertNumQueries(t, db, 3, func() {
			// One for Location, one for Guest, and no query for EventGuide
			if _, err := orm.CollectDelete(ctx, db, eventModel, []any{event.ID}); err != nil {
				t.Fatalf("CollectDelete: %v", err)
			}
		})
	})
}

// FIXME-PORT: tests/admin_utils/tests.py::NestedObjectsTests.test_relation_on_abstract
// needs multi-table inheritance, which the ORM does not support.

// FIXME-PORT: tests/admin_utils/tests.py::DBNestedObjectsTests needs
// on_delete=DB_CASCADE, which the ORM does not support.
