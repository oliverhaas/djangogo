package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/m2m_signals/models.py.

type m2msPart struct {
	ID   int64
	Name string `orm:"max_length=20"`
}

// Meta sets name ordering as in Django's m2m_signals.Part.
func (m2msPart) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

type m2msCar struct {
	ID            int64
	Name          string `orm:"max_length=20"`
	DefaultParts  orm.M2M[m2msPart]
	OptionalParts orm.M2M[m2msPart] `orm:"related_name=cars_optional"`
}

// Meta sets name ordering as in Django's m2m_signals.Car.
func (m2msCar) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

type m2msPerson struct {
	ID      int64
	Name    string              `orm:"max_length=20"`
	Fans    orm.M2M[m2msPerson] `orm:"symmetrical=false;related_name=idols"`
	Friends orm.M2M[m2msPerson]
}

// Meta sets name ordering as in Django's m2m_signals.Person.
func (m2msPerson) Meta() orm.Meta { return orm.Meta{Ordering: []string{"name"}} }

// m2msModels returns the model list for the m2m_signals test app.
func m2msModels() []any {
	return []any{djangotest.App("m2m_signals",
		&m2msPart{}, &m2msCar{}, &m2msPerson{},
	)}
}

// m2msRecord captures one m2m_changed notification for assertions.
type m2msRecord struct {
	instanceName string
	action       orm.M2MAction
	reverse      bool
	modelName    string
	pkSetNil     bool
	pks          []int64
}

// m2msCapture builds a signal handler appending to records.
func m2msCapture(records *[]m2msRecord) func(context.Context, orm.M2MChangedEvent) error {
	return func(_ context.Context, ev orm.M2MChangedEvent) error {
		var name string
		switch v := ev.Instance.(type) {
		case *m2msCar:
			name = v.Name
		case *m2msPart:
			name = v.Name
		case *m2msPerson:
			name = v.Name
		}
		pks := make([]int64, 0, len(ev.PKSet))
		for _, pk := range ev.PKSet {
			pks = append(pks, pk.(int64))
		}
		*records = append(*records, m2msRecord{
			instanceName: name,
			action:       ev.Action,
			reverse:      ev.Reverse,
			modelName:    ev.Model.Name(),
			pkSetNil:     ev.PKSet == nil,
			pks:          pks,
		})
		return nil
	}
}

// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_pk_set_on_repeated_add_remove
func TestM2MSignalsPKSetOnRepeatedAddRemove(t *testing.T) {
	djangotest.EachDB(t, m2msModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		vw := create(t, db, &m2msCar{Name: "VW"})
		wheelset := create(t, db, &m2msPart{Name: "Wheelset"})

		var pkSets [][]int64
		cancel := orm.OnM2MChanged[m2msCar]("default_parts", func(_ context.Context, ev orm.M2MChangedEvent) error {
			if ev.Action == orm.M2MPreAdd || ev.Action == orm.M2MPreRemove {
				got := make([]int64, len(ev.PKSet))
				for i, pk := range ev.PKSet {
					got[i] = pk.(int64)
				}
				pkSets = append(pkSets, got)
			}
			return nil
		})
		defer cancel()

		mgr := orm.Many[m2msPart](db, vw, "default_parts")
		if err := mgr.Add(ctx, wheelset); err != nil {
			t.Fatalf("first Add: %v", err)
		}
		if err := mgr.Add(ctx, wheelset); err != nil {
			t.Fatalf("second Add: %v", err)
		}
		if err := mgr.Remove(ctx, wheelset); err != nil {
			t.Fatalf("first Remove: %v", err)
		}
		if err := mgr.Remove(ctx, wheelset); err != nil {
			t.Fatalf("second Remove: %v", err)
		}

		djangotest.AssertEqual(t, len(pkSets), 4)
		djangotest.AssertSequenceEqual(t, pkSets[0], []int64{wheelset.ID})
		djangotest.AssertEqual(t, len(pkSets[1]), 0)
		djangotest.AssertSequenceEqual(t, pkSets[2], []int64{wheelset.ID})
		djangotest.AssertSequenceEqual(t, pkSets[3], []int64{wheelset.ID})
	})
}

// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_add_remove_clear
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_remove_relation
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_give_the_self_vw_some_optional_parts
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_reverse_relation_with_custom_related_name
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_clear_all_parts_of_the_self_vw
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_all_the_doors_off_of_cars
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_reverse_relation
func TestM2MSignalsAddRemoveClearForwardAndReverse(t *testing.T) {
	djangotest.EachDB(t, m2msModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		vw := create(t, db, &m2msCar{Name: "VW"})
		bmw := create(t, db, &m2msCar{Name: "BMW"})
		toyota := create(t, db, &m2msCar{Name: "Toyota"})
		wheelset := create(t, db, &m2msPart{Name: "Wheelset"})
		doors := create(t, db, &m2msPart{Name: "Doors"})
		engine := create(t, db, &m2msPart{Name: "Engine"})
		airbag := create(t, db, &m2msPart{Name: "Airbag"})
		sunroof := create(t, db, &m2msPart{Name: "Sunroof"})

		// Add sunroof before listener is installed.
		if err := orm.Many[m2msPart](db, vw, "default_parts").Add(ctx, sunroof); err != nil {
			t.Fatalf("seed sunroof: %v", err)
		}

		var records []m2msRecord
		c1 := orm.OnM2MChanged[m2msCar]("default_parts", m2msCapture(&records))
		defer c1()
		c2 := orm.OnM2MChanged[m2msCar]("optional_parts", m2msCapture(&records))
		defer c2()

		// Forward add.
		if err := orm.Many[m2msPart](db, vw, "default_parts").Add(ctx, wheelset, doors, engine); err != nil {
			t.Fatalf("vw.default_parts.Add: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreAdd)
		djangotest.AssertEqual(t, records[0].reverse, false)
		djangotest.AssertEqual(t, records[0].modelName, "m2msPart")
		djangotest.AssertEqual(t, records[0].instanceName, "VW")
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{wheelset.ID, doors.ID, engine.ID})
		djangotest.AssertEqual(t, records[1].action, orm.M2MPostAdd)

		// Reverse add.
		records = nil
		if err := orm.Many[m2msCar](db, doors, "m2mscar_set").Add(ctx, bmw, toyota); err != nil {
			t.Fatalf("doors.m2mscar_set.Add: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreAdd)
		djangotest.AssertEqual(t, records[0].reverse, true)
		djangotest.AssertEqual(t, records[0].modelName, "m2msCar")
		djangotest.AssertEqual(t, records[0].instanceName, "Doors")
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{bmw.ID, toyota.ID})

		// Forward remove (including a part not currently attached).
		records = nil
		if err := orm.Many[m2msPart](db, vw, "default_parts").Remove(ctx, engine, airbag); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreRemove)
		djangotest.AssertEqual(t, records[0].reverse, false)
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{engine.ID, airbag.ID})
		djangotest.AssertEqual(t, records[1].action, orm.M2MPostRemove)

		// Optional parts forward + reverse (filters already-attached vw out of pk_set).
		records = nil
		if err := orm.Many[m2msPart](db, vw, "optional_parts").Add(ctx, airbag, sunroof); err != nil {
			t.Fatalf("optional_parts.Add: %v", err)
		}
		if err := orm.Many[m2msCar](db, airbag, "cars_optional").Add(ctx, vw, bmw, toyota); err != nil {
			t.Fatalf("cars_optional.Add: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 4)
		djangotest.AssertEqual(t, records[2].action, orm.M2MPreAdd)
		djangotest.AssertEqual(t, records[2].reverse, true)
		djangotest.AssertSequenceEqual(t, records[2].pks, []int64{bmw.ID, toyota.ID})

		// Reverse remove on custom related_name.
		records = nil
		if err := orm.Many[m2msCar](db, airbag, "cars_optional").Remove(ctx, vw); err != nil {
			t.Fatalf("cars_optional.Remove: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreRemove)
		djangotest.AssertEqual(t, records[0].reverse, true)
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{vw.ID})

		// Forward clear (pk_set is nil).
		records = nil
		if err := orm.Many[m2msPart](db, vw, "default_parts").Clear(ctx); err != nil {
			t.Fatalf("default_parts.Clear: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreClear)
		djangotest.AssertEqual(t, records[0].reverse, false)
		djangotest.AssertEqual(t, records[0].pkSetNil, true)
		djangotest.AssertEqual(t, records[1].action, orm.M2MPostClear)
		djangotest.AssertEqual(t, records[1].pkSetNil, true)

		// Reverse clear.
		records = nil
		if err := orm.Many[m2msCar](db, doors, "m2mscar_set").Clear(ctx); err != nil {
			t.Fatalf("doors.Clear: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreClear)
		djangotest.AssertEqual(t, records[0].reverse, true)
		djangotest.AssertEqual(t, records[0].modelName, "m2msCar")
		djangotest.AssertEqual(t, records[0].pkSetNil, true)
	})
}

// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_alternative_ways
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_signals_clearing_removing
func TestM2MSignalsCreateAndSet(t *testing.T) {
	djangotest.EachDB(t, m2msModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		vw := create(t, db, &m2msCar{Name: "VW"})
		wheelset := create(t, db, &m2msPart{Name: "Wheelset"})
		doors := create(t, db, &m2msPart{Name: "Doors"})
		engine := create(t, db, &m2msPart{Name: "Engine"})

		var records []m2msRecord
		cancel := orm.OnM2MChanged[m2msCar]("default_parts", m2msCapture(&records))
		defer cancel()

		// Create fires pre_add + post_add.
		windows := &m2msPart{Name: "Windows"}
		if err := orm.Many[m2msPart](db, vw, "default_parts").Create(ctx, windows); err != nil {
			t.Fatalf("Create: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreAdd)
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{windows.ID})

		// Set without clear fires pre_remove/post_remove then pre_add/post_add.
		records = nil
		if err := orm.Many[m2msPart](db, vw, "default_parts").Set(ctx, []*m2msPart{wheelset, doors, engine}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 4)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreRemove)
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{windows.ID})
		djangotest.AssertEqual(t, records[1].action, orm.M2MPostRemove)
		djangotest.AssertEqual(t, records[2].action, orm.M2MPreAdd)
		djangotest.AssertSequenceEqual(t, records[2].pks, []int64{wheelset.ID, doors.ID, engine.ID})
		djangotest.AssertEqual(t, records[3].action, orm.M2MPostAdd)

		// Set with SetClear() fires pre_clear/post_clear then pre_add/post_add.
		records = nil
		if err := orm.Many[m2msPart](db, vw, "default_parts").Set(ctx, []*m2msPart{wheelset, doors}, orm.SetClear()); err != nil {
			t.Fatalf("Set(SetClear): %v", err)
		}
		djangotest.AssertEqual(t, len(records), 4)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreClear)
		djangotest.AssertEqual(t, records[0].pkSetNil, true)
		djangotest.AssertEqual(t, records[1].action, orm.M2MPostClear)
		djangotest.AssertEqual(t, records[2].action, orm.M2MPreAdd)
		djangotest.AssertSequenceEqual(t, records[2].pks, []int64{wheelset.ID, doors.ID})
		djangotest.AssertEqual(t, records[3].action, orm.M2MPostAdd)

		// Removing only one item with Set(clear=false) fires only pre_remove/post_remove.
		records = nil
		if err := orm.Many[m2msPart](db, vw, "default_parts").Set(ctx, []*m2msPart{wheelset}); err != nil {
			t.Fatalf("Set remove only: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreRemove)
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{doors.ID})
		djangotest.AssertEqual(t, records[1].action, orm.M2MPostRemove)
	})
}

// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_with_self_add_friends
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_with_self_add_fan
// Ported from Django: tests/m2m_signals/tests.py::ManyToManySignalsTest.test_m2m_relations_with_self_add_idols
func TestM2MSignalsSelfReferential(t *testing.T) {
	djangotest.EachDB(t, m2msModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		alice := create(t, db, &m2msPerson{Name: "Alice"})
		bob := create(t, db, &m2msPerson{Name: "Bob"})
		chuck := create(t, db, &m2msPerson{Name: "Chuck"})
		daisy := create(t, db, &m2msPerson{Name: "Daisy"})

		var records []m2msRecord
		c1 := orm.OnM2MChanged[m2msPerson]("fans", m2msCapture(&records))
		defer c1()
		c2 := orm.OnM2MChanged[m2msPerson]("friends", m2msCapture(&records))
		defer c2()

		// Symmetrical self-ref fires pre_add/post_add only once (not for mirror rows).
		if err := orm.Many[m2msPerson](db, alice, "friends").Set(ctx, []*m2msPerson{bob, chuck}); err != nil {
			t.Fatalf("alice.friends.Set: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreAdd)
		djangotest.AssertEqual(t, records[0].reverse, false)
		djangotest.AssertEqual(t, records[0].instanceName, "Alice")
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{bob.ID, chuck.ID})

		// Non-symmetrical forward.
		records = nil
		if err := orm.Many[m2msPerson](db, alice, "fans").Set(ctx, []*m2msPerson{daisy}); err != nil {
			t.Fatalf("alice.fans.Set: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreAdd)
		djangotest.AssertEqual(t, records[0].reverse, false)
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{daisy.ID})

		// Non-symmetrical reverse.
		records = nil
		if err := orm.Many[m2msPerson](db, chuck, "idols").Set(ctx, []*m2msPerson{alice, bob}); err != nil {
			t.Fatalf("chuck.idols.Set: %v", err)
		}
		djangotest.AssertEqual(t, len(records), 2)
		djangotest.AssertEqual(t, records[0].action, orm.M2MPreAdd)
		djangotest.AssertEqual(t, records[0].reverse, true)
		djangotest.AssertEqual(t, records[0].instanceName, "Chuck")
		djangotest.AssertSequenceEqual(t, records[0].pks, []int64{alice.ID, bob.ID})
	})
}
