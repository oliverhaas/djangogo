package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/defer/models.py. Child, BigChild,
// ChildProxy, RefreshPrimaryProxy and ShadowChild need multi-table
// inheritance or proxy models, which the ORM does not have.

// dfSecondary is Django's defer.Secondary.
type dfSecondary struct {
	orm.RelCache
	ID     int64
	First  string `orm:"max_length=50"`
	Second string `orm:"max_length=50"`
}

// dfPrimary is Django's defer.Primary.
type dfPrimary struct {
	orm.RelCache
	ID      int64
	Name    string              `orm:"max_length=50"`
	Value   string              `orm:"max_length=50"`
	Related orm.FK[dfSecondary] `orm:"on_delete=cascade;related_name=primary_set"`
}

// String returns the name, as Django's Primary.__str__ does.
func (p dfPrimary) String() string { return p.Name }

// dfPrimaryOneToOne is Django's defer.PrimaryOneToOne.
type dfPrimaryOneToOne struct {
	orm.RelCache
	ID      int64
	Name    string               `orm:"max_length=50"`
	Value   string               `orm:"max_length=50"`
	Related orm.O2O[dfSecondary] `orm:"on_delete=cascade;related_name=primary_o2o"`
}

// dfPlain is dfSecondary without orm.RelCache.
type dfPlain struct {
	ID    int64
	First string `orm:"max_length=50"`
}

// dfModels returns the models of the defer tests.
func dfModels() []any {
	return []any{djangotest.App("defer", &dfSecondary{}, &dfPrimary{}, &dfPrimaryOneToOne{}, &dfPlain{})}
}

// dfSetUp creates the rows of Django's DeferTests.setUpTestData.
func dfSetUp(t *testing.T, db *orm.DB) (*dfSecondary, *dfPrimary) {
	t.Helper()
	s1 := create(t, db, &dfSecondary{First: "x1", Second: "y1"})
	p1 := create(t, db, &dfPrimary{Name: "p1", Value: "xx", Related: fkTo[dfSecondary](s1.ID)})
	return s1, p1
}

// dfAssertDelayed checks the number of deferred fields of obj, Django's
// AssertionMixin.assert_delayed.
func dfAssertDelayed(t *testing.T, obj any, num int) {
	t.Helper()
	if got := orm.DeferredFields(obj); len(got) != num {
		t.Errorf("deferred fields = %v, want %d of them", got, num)
	}
}

// dfFirst returns the first row of qs, Django's qs[0].
func dfFirst[T any](t *testing.T, qs *orm.QuerySet[T]) T {
	t.Helper()
	obj, err := qs.First(context.Background())
	if err != nil {
		t.Fatalf("First: %v", err)
	}
	return obj
}

// dfGet returns the row of qs with the primary key pk.
func dfGet[T any](t *testing.T, qs *orm.QuerySet[T], pk int64) T {
	t.Helper()
	obj, err := qs.Get(context.Background(), "pk", pk)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return obj
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer
func TestDeferDefer(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		_, p1 := dfSetUp(t, db)
		qs := orm.Query[dfPrimary](db)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name")), 1)
		dfAssertDelayed(t, dfGet(t, qs.Defer("name"), p1.ID), 1)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("related__first")), 0)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name").Defer("value")), 2)
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_only
func TestDeferOnly(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		s1, p1 := dfSetUp(t, db)
		qs := orm.Query[dfPrimary](db)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name")), 2)
		dfAssertDelayed(t, dfGet(t, qs.Only("name"), p1.ID), 2)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name").Only("value")), 2)
		dfAssertDelayed(t, dfFirst(t, qs.Only("related__first")), 2)
		// Using "pk" with Only defers every field but the primary key.
		dfAssertDelayed(t, dfFirst(t, qs.Only("pk")), 3)
		// Django reads related_id of each row of the reverse manager to
		// attach the owner, which loads it; the Go manager does not attach
		// the owner, so related stays deferred.
		dfAssertDelayed(t, dfFirst(t, orm.Related[dfPrimary](db, s1, "primary_set").Only("pk")), 3)
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer_only_chaining
func TestDeferOnlyChaining(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		dfSetUp(t, db)
		qs := orm.Query[dfPrimary](db)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name", "value").Defer("name")), 2)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name").Only("value", "name")), 2)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name").Only("name").Only("value")), 2)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name").Only("value")), 2)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name").Defer("value")), 2)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name").Defer("name").Defer("value")), 1)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name").Defer("name", "value")), 1)
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer_only_clear
func TestDeferOnlyClear(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		dfSetUp(t, db)
		qs := orm.Query[dfPrimary](db)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name").Defer("name")), 0)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name").Only("name")), 0)
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer_on_an_already_deferred_field
func TestDeferOnAnAlreadyDeferredField(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		dfSetUp(t, db)
		qs := orm.Query[dfPrimary](db)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name")), 1)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name").Defer("name")), 1)
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer_none_to_clear_deferred_set
func TestDeferNoneToClearDeferredSet(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		dfSetUp(t, db)
		qs := orm.Query[dfPrimary](db)
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name", "value")), 2)
		// Defer without keys is Django's defer(None).
		dfAssertDelayed(t, dfFirst(t, qs.Defer("name", "value").Defer()), 0)
		dfAssertDelayed(t, dfFirst(t, qs.Only("name").Defer()), 0)
	})
}

// No Go counterpart: tests/defer/tests.py::DeferTests.test_only_none_raises_error
// passes None to only(); Go has no None key, and Only without keys loads
// every field.

// No Go counterpart: tests/defer/tests.py::DeferTests.test_defer_extra needs
// extra(), which the ORM does not have.

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer_values_does_not_defer
func TestDeferValuesDoesNotDefer(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		s1, p1 := dfSetUp(t, db)
		rows, err := orm.Query[dfPrimary](db).Defer("name").Values().All(context.Background())
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		djangotest.AssertEqual(t, rows[0], map[string]any{
			"id": p1.ID, "name": "p1", "value": "xx", "related_id": s1.ID,
		})
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_only_values_does_not_defer
func TestDeferOnlyValuesDoesNotDefer(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		s1, p1 := dfSetUp(t, db)
		rows, err := orm.Query[dfPrimary](db).Only("name").Values().All(context.Background())
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		djangotest.AssertEqual(t, rows[0], map[string]any{
			"id": p1.ID, "name": "p1", "value": "xx", "related_id": s1.ID,
		})
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_get
func TestDeferGet(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		_, p1 := dfSetUp(t, db)
		qs := orm.Query[dfPrimary](db)
		dfAssertDelayed(t, dfGet(t, qs.Defer("name"), p1.ID), 1)
		dfAssertDelayed(t, dfGet(t, qs.Only("name"), p1.ID), 2)
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer_with_select_related
func TestDeferWithSelectRelated(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		dfSetUp(t, db)
		obj := dfFirst(t, orm.Query[dfPrimary](db).SelectRelated("related").Defer("related__first", "related__second"))
		related, ok := obj.Related.Object()
		if !ok {
			t.Fatal("related is not loaded")
		}
		dfAssertDelayed(t, related, 2)
		dfAssertDelayed(t, obj, 0)
	})
}

// Ported from Django: tests/defer/tests.py::DeferTests.test_only_with_select_related
func TestDeferOnlyWithSelectRelated(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		s1, _ := dfSetUp(t, db)
		obj := dfFirst(t, orm.Query[dfPrimary](db).SelectRelated("related").Only("related__first"))
		dfAssertDelayed(t, obj, 2)
		related, ok := obj.Related.Object()
		if !ok {
			t.Fatal("related is not loaded")
		}
		dfAssertDelayed(t, related, 1)
		djangotest.AssertEqual(t, obj.Related.PK(), s1.ID)
		djangotest.AssertEqual(t, related.First, "x1")
	})
}

// No Go counterpart: tests/defer/tests.py::DeferTests.test_defer_foreign_keys_are_deferred_and_not_traversed
// calls select_related() without names, which follows every relation;
// SelectRelated needs names, and a named relation cannot be deferred.

// Ported from Django: tests/defer/tests.py::DeferTests.test_saving_object_with_deferred_field
func TestDeferSavingObjectWithDeferredField(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		s1, _ := dfSetUp(t, db)
		ctx := context.Background()
		create(t, db, &dfPrimary{Name: "p2", Value: "xy", Related: fkTo[dfSecondary](s1.ID)})
		obj, err := orm.Query[dfPrimary](db).Defer("value").Get(ctx, "name", "p2")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		obj.Name = "a new name"
		if err := orm.Query[dfPrimary](db).Save(ctx, &obj); err != nil {
			t.Fatalf("Save: %v", err)
		}
		rows := mustAll(t, orm.Query[dfPrimary](db).OrderBy("id"))
		djangotest.AssertEqual(t, []string{rows[0].Name, rows[1].Name}, []string{"p1", "a new name"})
		// The deferred value keeps its stored text.
		djangotest.AssertEqual(t, rows[1].Value, "xy")
	})
}

// No Django test exists: a deferred field that Save finds set is written,
// because a Go field cannot record that it was assigned; one that still holds
// its zero value keeps the stored value.
func TestDeferSaveWritesAssignedDeferredField(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		_, p1 := dfSetUp(t, db)
		ctx := context.Background()
		obj := dfGet(t, orm.Query[dfPrimary](db).Only("name"), p1.ID)
		obj.Value = "new value"
		if err := orm.Query[dfPrimary](db).Save(ctx, &obj); err != nil {
			t.Fatalf("Save: %v", err)
		}
		dfAssertDelayed(t, obj, 1)
		got := dfGet(t, orm.Query[dfPrimary](db), p1.ID)
		djangotest.AssertEqual(t, got.Value, "new value")
		djangotest.AssertEqual(t, got.Related.PK(), p1.Related.PK())
	})
}

// No Django test exists: Save of an instance with deferred fields updates
// the loaded fields only, so a row that was deleted is not inserted again
// with empty deferred fields.
func TestDeferSaveOfDeletedRowFails(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		_, p1 := dfSetUp(t, db)
		ctx := context.Background()
		obj := dfGet(t, orm.Query[dfPrimary](db).Only("name"), p1.ID)
		if _, err := orm.Query[dfPrimary](db).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		err := orm.Query[dfPrimary](db).Save(ctx, &obj)
		assertErrorContains(t, err, "save with update_fields did not affect any rows")
	})
}

// No Django test exists: Defer and Only record the deferred fields in
// orm.RelCache, and Save needs them, so a model without it is refused.
func TestDeferNeedsRelCache(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &dfPlain{First: "a"})
		_, err := orm.Query[dfPlain](db).Defer("first").All(context.Background())
		assertErrorContains(t, err, "dfPlain must embed orm.RelCache")
		rows, err := orm.Query[dfPlain](db).Only("first").Defer("first").All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertEqual(t, rows[0].First, "a")
	})
}

// Skipped: tests/defer/tests.py::DeferTests.test_defer_baseclass_when_subclass_has_no_added_fields
func TestDeferBaseclassWhenSubclassHasNoAddedFields(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// Skipped: tests/defer/tests.py::DeferTests.test_only_baseclass_when_subclass_has_no_added_fields
func TestDeferOnlyBaseclassWhenSubclassHasNoAddedFields(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// No Go counterpart: tests/defer/tests.py::DeferTests.test_defer_of_overridden_scalar
// overrides a class attribute of a parent model with a field; a Go struct has
// no class attributes.

// Ported from Django: tests/defer/tests.py::DeferTests.test_defer_fk_attname
func TestDeferFKAttname(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		_, p1 := dfSetUp(t, db)
		ctx := context.Background()
		primary, err := orm.Query[dfPrimary](db).Defer("related_id").Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, orm.DeferredFields(primary), []string{"related_id"})
		// Django loads the deferred field on access; Go loads it with
		// RefreshFromDB.
		djangotest.AssertNumQueries(t, db, 1, func() {
			if err := orm.Query[dfPrimary](db).RefreshFromDB(ctx, &primary, "related_id"); err != nil {
				t.Fatalf("RefreshFromDB: %v", err)
			}
		})
		djangotest.AssertEqual(t, primary.Related.PK(), p1.Related.PK())
		dfAssertDelayed(t, primary, 0)
	})
}

// Skipped: tests/defer/tests.py::BigChildDeferTests.test_defer_baseclass_when_subclass_has_added_field
func TestDeferBaseclassWhenSubclassHasAddedField(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// Skipped: tests/defer/tests.py::BigChildDeferTests.test_defer_subclass
func TestDeferSubclass(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// Skipped: tests/defer/tests.py::BigChildDeferTests.test_defer_subclass_both
func TestDeferSubclassBoth(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// Skipped: tests/defer/tests.py::BigChildDeferTests.test_only_baseclass_when_subclass_has_added_field
func TestDeferOnlyBaseclassWhenSubclassHasAddedField(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// Skipped: tests/defer/tests.py::BigChildDeferTests.test_only_subclass
func TestDeferOnlySubclass(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// Skipped: tests/defer/tests.py::TestDefer2.test_defer_proxy
func TestDeferProxy(t *testing.T) {
	t.Skip("needs proxy models, which the ORM does not have")
}

// Skipped: tests/defer/tests.py::TestDefer2.test_defer_inheritance_pk_chaining
func TestDeferInheritancePKChaining(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// No Go counterpart: tests/defer/tests.py::TestDefer2.test_eq compares model
// instances by primary key; Go compares structs field by field.

// Ported from Django: tests/defer/tests.py::TestDefer2.test_refresh_not_loading_deferred_fields
func TestDeferRefreshNotLoadingDeferredFields(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		s := create(t, db, &dfSecondary{})
		rf := create(t, db, &dfPrimary{Name: "foo", Value: "bar", Related: fkTo[dfSecondary](s.ID)})
		rf2, err := orm.Query[dfPrimary](db).Only("related", "value").Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		rf.Name = "new foo"
		rf.Value = "new bar"
		if err := orm.Query[dfPrimary](db).Save(ctx, rf); err != nil {
			t.Fatalf("Save: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 1, func() {
			if err := orm.Query[dfPrimary](db).RefreshFromDB(ctx, &rf2); err != nil {
				t.Fatalf("RefreshFromDB: %v", err)
			}
			djangotest.AssertEqual(t, rf2.Value, "new bar")
		})
		djangotest.AssertEqual(t, rf2.Name, "")
		dfAssertDelayed(t, rf2, 1)
		// Django loads the deferred name on access; Go loads it with
		// RefreshFromDB.
		djangotest.AssertNumQueries(t, db, 1, func() {
			if err := orm.Query[dfPrimary](db).RefreshFromDB(ctx, &rf2, "name"); err != nil {
				t.Fatalf("RefreshFromDB: %v", err)
			}
			djangotest.AssertEqual(t, rf2.Name, "new foo")
		})
	})
}

// No Go counterpart: tests/defer/tests.py::TestDefer2.test_custom_refresh_on_deferred_loading
// overrides refresh_from_db, which Django calls when a deferred field is
// read; a Go field loads nothing when it is read.

// Ported from Django: tests/defer/tests.py::TestDefer2.test_refresh_when_one_field_deferred
func TestDeferRefreshWhenOneFieldDeferred(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		s := create(t, db, &dfSecondary{})
		create(t, db, &dfPrimaryOneToOne{Name: "foo", Value: "bar", Related: o2oTo[dfSecondary](s.ID)})
		got, err := orm.Query[dfSecondary](db).Defer("first").Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		pBefore, err := orm.RelatedOne[dfPrimaryOneToOne](ctx, db, &got, "primary_o2o")
		if err != nil {
			t.Fatalf("RelatedOne: %v", err)
		}
		if err := orm.Query[dfSecondary](db).RefreshFromDB(ctx, &got); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		pAfter, err := orm.RelatedOne[dfPrimaryOneToOne](ctx, db, &got, "primary_o2o")
		if err != nil {
			t.Fatalf("RelatedOne: %v", err)
		}
		if pAfter == pBefore {
			t.Error("RefreshFromDB kept the cached reverse one-to-one object")
		}
	})
}

// Ported from Django: tests/defer/tests.py::InvalidDeferTests.test_invalid_defer
func TestDeferInvalidDefer(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := orm.Query[dfPrimary](db).Defer("missing").All(ctx)
		assertErrorContains(t, err, "dfPrimary: has no field named 'missing'")
		_, err = orm.Query[dfPrimary](db).Defer("value__missing").All(ctx)
		assertErrorContains(t, err, "missing")
		_, err = orm.Query[dfPrimary](db).Defer("related__missing").All(ctx)
		assertErrorContains(t, err, "dfSecondary: has no field named 'missing'")
	})
}

// Ported from Django: tests/defer/tests.py::InvalidDeferTests.test_invalid_only
func TestDeferInvalidOnly(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := orm.Query[dfPrimary](db).Only("missing").All(ctx)
		assertErrorContains(t, err, "dfPrimary: has no field named 'missing'")
		_, err = orm.Query[dfPrimary](db).Only("value__missing").All(ctx)
		assertErrorContains(t, err, "missing")
		_, err = orm.Query[dfPrimary](db).Only("related__missing").All(ctx)
		assertErrorContains(t, err, "dfSecondary: has no field named 'missing'")
	})
}

// Ported from Django: tests/defer/tests.py::InvalidDeferTests.test_defer_select_related_raises_invalid_query
func TestDeferSelectRelatedRaisesInvalidQuery(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[dfPrimary](db).Defer("related").SelectRelated("related").First(context.Background())
		assertErrorContains(t, err,
			"Field dfPrimary.related cannot be both deferred and traversed using select_related at the same time.")
	})
}

// Ported from Django: tests/defer/tests.py::InvalidDeferTests.test_only_select_related_raises_invalid_query
func TestDeferOnlySelectRelatedRaisesInvalidQuery(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[dfPrimary](db).Only("name").SelectRelated("related").First(context.Background())
		assertErrorContains(t, err,
			"Field dfPrimary.related cannot be both deferred and traversed using select_related at the same time.")
	})
}

// dfRelationSetUp creates the rows of Django's DeferredRelationTests.setUpTestData.
func dfRelationSetUp(t *testing.T, db *orm.DB) *dfSecondary {
	t.Helper()
	secondary := create(t, db, &dfSecondary{First: "a", Second: "b"})
	create(t, db, &dfPrimaryOneToOne{Name: "Bella", Value: "Baxter", Related: o2oTo[dfSecondary](secondary.ID)})
	return secondary
}

// dfAssertRefreshKeepsRelation loads obj's reverse one-to-one object, loads
// the deferred field key, and checks that the object stays cached, Django's
// DeferredRelationTests.
func dfAssertRefreshKeepsRelation(t *testing.T, db *orm.DB, obj *dfSecondary, key string) {
	t.Helper()
	ctx := context.Background()
	djangotest.AssertNumQueries(t, db, 1, func() {
		if _, err := orm.RelatedOne[dfPrimaryOneToOne](ctx, db, obj, "primary_o2o"); err != nil {
			t.Fatalf("RelatedOne: %v", err)
		}
	})
	// Django loads the deferred field on access; Go loads it with
	// RefreshFromDB.
	if err := orm.Query[dfSecondary](db).RefreshFromDB(ctx, obj, key); err != nil {
		t.Fatalf("RefreshFromDB: %v", err)
	}
	djangotest.AssertNumQueries(t, db, 0, func() {
		if _, err := orm.RelatedOne[dfPrimaryOneToOne](ctx, db, obj, "primary_o2o"); err != nil {
			t.Fatalf("RelatedOne: %v", err)
		}
	})
}

// Ported from Django: tests/defer/tests.py::DeferredRelationTests.test_defer_not_clear_cached_relations
func TestDeferNotClearCachedRelations(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		secondary := dfRelationSetUp(t, db)
		obj := dfGet(t, orm.Query[dfSecondary](db).Defer("first"), secondary.ID)
		dfAssertRefreshKeepsRelation(t, db, &obj, "first")
		djangotest.AssertEqual(t, obj.First, "a")
	})
}

// Ported from Django: tests/defer/tests.py::DeferredRelationTests.test_only_not_clear_cached_relations
func TestDeferOnlyNotClearCachedRelations(t *testing.T) {
	djangotest.EachDB(t, dfModels(), func(t *testing.T, db *orm.DB) {
		secondary := dfRelationSetUp(t, db)
		obj := dfGet(t, orm.Query[dfSecondary](db).Only("first"), secondary.ID)
		dfAssertRefreshKeepsRelation(t, db, &obj, "second")
		djangotest.AssertEqual(t, obj.Second, "b")
	})
}
