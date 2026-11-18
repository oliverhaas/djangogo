package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// updDataPoint is Django's update.DataPoint.
type updDataPoint struct {
	ID           int64
	Name         string `orm:"max_length=20"`
	Value        string `orm:"max_length=20"`
	AnotherValue string `orm:"max_length=20;blank"`
	IsActive     bool   `orm:"default=true"`
}

// updRelatedPoint is Django's update.RelatedPoint.
type updRelatedPoint struct {
	ID   int64
	Name string `orm:"max_length=20"`
	Data orm.FK[updDataPoint]
}

// updA is Django's update.A.
type updA struct {
	ID int64
	X  int64 `orm:"default=10"`
}

// updB is Django's update.B.
type updB struct {
	ID int64
	A  orm.FK[updA]
	Y  int64 `orm:"default=10"`
}

// updModels returns the models of Django's update test app.
func updModels() []any {
	return []any{djangotest.App("update", &updDataPoint{}, &updRelatedPoint{}, &updA{}, &updB{})}
}

// updSimple is the fixture of Django's update SimpleTest: two A rows, and
// twenty B rows that point at the first.
type updSimple struct {
	a1, a2 *updA
}

// updSetUpSimple creates the SimpleTest fixture.
func updSetUpSimple(t *testing.T, db *orm.DB) updSimple {
	t.Helper()
	fx := updSimple{a1: create(t, db, &updA{}), a2: create(t, db, &updA{})}
	rows := make([]*updB, 20)
	for i := range rows {
		rows[i] = &updB{A: fkTo[updA](fx.a1.ID)}
	}
	if err := orm.Query[updB](db).BulkCreate(context.Background(), rows); err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
	return fx
}

// updAdvanced is the fixture of Django's update AdvancedTests.
type updAdvanced struct {
	d0, d2, d3 *updDataPoint
	r1         *updRelatedPoint
}

// updSetUpAdvanced creates the AdvancedTests fixture. A false is_active cannot
// be created against default=true, so d3 is switched off after its insert.
func updSetUpAdvanced(t *testing.T, db *orm.DB) updAdvanced {
	t.Helper()
	fx := updAdvanced{
		d0: create(t, db, &updDataPoint{Name: "d0", Value: "apple"}),
		d2: create(t, db, &updDataPoint{Name: "d2", Value: "banana"}),
		d3: create(t, db, &updDataPoint{Name: "d3", Value: "banana"}),
	}
	updUpdate(t, 1, orm.Query[updDataPoint](db).Filter("pk", fx.d3.ID), "is_active", false)
	fx.d3.IsActive = false
	fx.r1 = create(t, db, &updRelatedPoint{Name: "r1", Data: fkTo[updDataPoint](fx.d3.ID)})
	return fx
}

// updUpdate runs an update and checks the number of rows it reports.
func updUpdate[T any](t *testing.T, want int64, qs *orm.QuerySet[T], assignments ...any) {
	t.Helper()
	got, err := qs.Update(context.Background(), assignments...)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	djangotest.AssertEqual(t, got, want)
}

// The reverse manager a1.b_set belongs to another work package: the rows are
// selected through the forward key instead.
//
// Ported from Django: tests/update/tests.py::SimpleTest.test_nonempty_update
func TestUpdateNonemptyUpdate(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		fx := updSetUpSimple(t, db)
		updUpdate(t, 20, orm.Query[updB](db).Filter("a", fx.a1), "y", 100)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[updB](db).Filter("y", 100)), int64(20))
	})
}

// Ported from Django: tests/update/tests.py::SimpleTest.test_empty_update
func TestUpdateEmptyUpdate(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		fx := updSetUpSimple(t, db)
		updUpdate(t, 0, orm.Query[updB](db).Filter("a", fx.a2), "y", 100)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[updB](db).Filter("y", 100)), int64(0))
	})
}

// Django runs this on the inherited model D. There is no model inheritance
// here, so the plain model B takes its place.
//
// Ported from Django: tests/update/tests.py::SimpleTest.test_foreign_key_update_with_id
func TestUpdateForeignKeyUpdateWithID(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		fx := updSetUpSimple(t, db)
		updUpdate(t, 20, orm.Query[updB](db).Filter("a", fx.a1), "a_id", fx.a2)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[updB](db).Filter("a", fx.a2)), int64(20))
	})
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update
func TestUpdateUpdate(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		fx := updSetUpAdvanced(t, db)
		updUpdate(t, 1, orm.Query[updDataPoint](db).Filter("value", "apple"), "name", "d1")
		rows := mustAll(t, orm.Query[updDataPoint](db).Filter("value", "apple"))
		if len(rows) != 1 || rows[0].ID != fx.d0.ID {
			t.Errorf("rows = %+v, want only d0", rows)
		}
	})
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_multiple_objects
func TestUpdateUpdateMultipleObjects(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		updSetUpAdvanced(t, db)
		updUpdate(t, 2, orm.Query[updDataPoint](db).Filter("value", "banana"), "value", "pineapple")
		d2, err := orm.Query[updDataPoint](db).Get(context.Background(), "name", "d2")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, d2.Value, "pineapple")
	})
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_fk
func TestUpdateUpdateFK(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		fx := updSetUpAdvanced(t, db)
		updUpdate(t, 1, orm.Query[updRelatedPoint](db).Filter("name", "r1"), "data", fx.d0)
		rows := mustAll(t, orm.Query[updRelatedPoint](db).Filter("data__name", "d0"))
		if len(rows) != 1 || rows[0].ID != fx.r1.ID {
			t.Errorf("rows = %+v, want only r1", rows)
		}
	})
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_multiple_fields
func TestUpdateUpdateMultipleFields(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		updSetUpAdvanced(t, db)
		updUpdate(t, 1, orm.Query[updDataPoint](db).Filter("value", "apple"),
			"value", "fruit", "another_value", "peach")
		d, err := orm.Query[updDataPoint](db).Get(context.Background(), "name", "d0")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, d.Value, "fruit")
		djangotest.AssertEqual(t, d.AnotherValue, "peach")
	})
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_all
func TestUpdateUpdateAll(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		updSetUpAdvanced(t, db)
		updUpdate(t, 3, orm.Query[updDataPoint](db), "value", "thing")
		assertValues(t, orm.Query[updDataPoint](db).Values("value").Distinct(),
			[]map[string]any{{"value": "thing"}})
	})
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_slice_fail
func TestUpdateUpdateSliceFail(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		updSetUpAdvanced(t, db)
		_, err := orm.Query[updDataPoint](db).Limit(2).
			Update(context.Background(), "another_value", "another thing")
		assertErrorContains(t, err, "annot update a query once a slice has been taken")
	})
}

// Django names the offending expression after its message. The Go error stops
// at the sentence.
//
// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_annotated_queryset
func TestUpdateUpdateAnnotatedQueryset(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		updSetUpAdvanced(t, db)
		ctx := context.Background()
		annotated := func() *orm.QuerySet[updDataPoint] {
			return orm.Query[updDataPoint](db).Annotate("alias", orm.F("value"))
		}
		// A trivial annotated update.
		updUpdate(t, 3, annotated(), "another_value", "foo")
		// The annotation filters the rows.
		updUpdate(t, 1, annotated().Filter("alias", "apple"), "another_value", "foo")
		// The annotation is the assigned value.
		updUpdate(t, 3, annotated(), "another_value", orm.F("alias"))
		assertFlat(t, orm.Query[updDataPoint](db).OrderBy("name").ValuesList("another_value"),
			[]any{"apple", "banana", "banana"})
		// An aggregate annotation cannot be assigned.
		_, err := orm.Query[updDataPoint](db).Annotate("max", orm.Max("value")).
			Update(ctx, "another_value", orm.F("max"))
		assertFieldError(t, err, "ggregate functions are not allowed in this query")
	})
}

// Only the F("data__name") case is ported: there are no transforms or
// database functions to build the other three annotations from. The direct
// reference without an annotation is rejected the same way.
//
// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_with_joined_field_annotation
func TestUpdateUpdateWithJoinedFieldAnnotation(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		updSetUpAdvanced(t, db)
		ctx := context.Background()
		const msg = "oined field references are not permitted in this query"
		_, err := orm.Query[updRelatedPoint](db).Annotate("new_name", orm.F("data__name")).
			Update(ctx, "name", orm.F("new_name"))
		assertFieldError(t, err, msg)
		_, err = orm.Query[updRelatedPoint](db).Update(ctx, "name", orm.F("data__name"))
		assertFieldError(t, err, msg)
		// Neither statement wrote anything.
		assertFlat(t, orm.Query[updRelatedPoint](db).ValuesList("name"), []any{"r1"})
	})
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_m2m_field
func TestUpdateUpdateM2MField(t *testing.T) {
	t.Skip("needs M2M[T], which belongs to the relations work package")
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_reverse_m2m_descriptor
func TestUpdateUpdateReverseM2MDescriptor(t *testing.T) {
	t.Skip("needs M2M[T], which belongs to the relations work package")
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_reverse_fk_descriptor
func TestUpdateUpdateReverseFKDescriptor(t *testing.T) {
	t.Skip("needs reverse relations, which belong to the relations work package")
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_reverse_o2o_descriptor
func TestUpdateUpdateReverseO2ODescriptor(t *testing.T) {
	t.Skip("needs O2O[T], which belongs to the relations work package")
}

// Ported from Django: tests/update/tests.py::AdvancedTests.test_update_annotated_multi_table_queryset
func TestUpdateUpdateAnnotatedMultiTableQueryset(t *testing.T) {
	t.Skip("needs reverse relations, which belong to the relations work package")
}

// An expression on the right-hand side reads the row that is being updated.
func TestUpdateWithExpression(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		fx := updSetUpSimple(t, db)
		create(t, db, &updB{A: fkTo[updA](fx.a2.ID), Y: 3})
		updUpdate(t, 21, orm.Query[updB](db), "y", orm.F("y").Mul(2).Add(1))
		assertFlat(t, orm.Query[updB](db).Distinct().OrderBy("y").ValuesList("y"),
			[]any{int64(7), int64(21)})
	})
}

// A filter that spans a relation moves into a subquery, because an UPDATE has
// no joins.
func TestUpdateFilteredThroughRelation(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		fx := updSetUpAdvanced(t, db)
		create(t, db, &updRelatedPoint{Name: "r0", Data: fkTo[updDataPoint](fx.d0.ID)})
		updUpdate(t, 1, orm.Query[updRelatedPoint](db).Filter("data__value", "banana"), "name", "renamed")
		assertFlat(t, orm.Query[updRelatedPoint](db).OrderBy("name").ValuesList("name"),
			[]any{"r0", "renamed"})
	})
}

// An unknown field is reported before anything is written.
func TestUpdateUnknownField(t *testing.T) {
	djangotest.EachDB(t, updModels(), func(t *testing.T, db *orm.DB) {
		updSetUpAdvanced(t, db)
		_, err := orm.Query[updDataPoint](db).Update(context.Background(), "nope", 1)
		assertFieldError(t, err, "annot resolve keyword 'nope' into field")
	})
}
