package orm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/delete/models.py. Model names match
// Django's classes so ProtectedError and RestrictedError messages can be
// checked verbatim.

// P is Django's delete.P.
type P struct {
	ID int64
}

// R is Django's delete.R.
type R struct {
	ID        int64
	IsDefault bool      `orm:"default=false"`
	P         orm.FK[P] `orm:"null;on_delete=cascade"`
}

// S is Django's delete.S.
type S struct {
	ID int64
	R  orm.FK[R] `orm:"on_delete=cascade"`
}

// T is Django's delete.T.
type T struct {
	ID int64
	S  orm.FK[S] `orm:"on_delete=cascade"`
}

// U is Django's delete.U.
type U struct {
	ID int64
	T  orm.FK[T] `orm:"on_delete=cascade"`
}

// A is Django's delete.A with the on_delete modes supported by the ORM.
type A struct {
	ID              int64
	Name            string    `orm:"max_length=30"`
	Auto            orm.FK[R] `orm:"on_delete=cascade"`
	AutoNullable    orm.FK[R] `orm:"null;on_delete=cascade"`
	SetNull         orm.FK[R] `orm:"null;on_delete=set_null"`
	Cascade         orm.FK[R] `orm:"on_delete=cascade"`
	CascadeNullable orm.FK[R] `orm:"null;on_delete=cascade"`
	Protect         orm.FK[R] `orm:"null;on_delete=protect"`
	Restrict        orm.FK[R] `orm:"null;on_delete=restrict"`
	DoNothing       orm.FK[R] `orm:"null;on_delete=do_nothing"`
	CascadeP        orm.FK[P] `orm:"null;on_delete=cascade"`
}

// B is Django's delete.B.
type B struct {
	ID      int64
	Protect orm.FK[R] `orm:"on_delete=protect"`
}

// B3 is Django's delete.B3.
type B3 struct {
	ID       int64
	Restrict orm.FK[R] `orm:"on_delete=restrict"`
}

// Base is Django's delete.Base.
type Base struct {
	ID int64
}

// RelToBase is Django's delete.RelToBase.
type RelToBase struct {
	ID   int64
	Base orm.FK[Base] `orm:"on_delete=do_nothing"`
}

// DeleteTop is Django's delete.DeleteTop.
type DeleteTop struct {
	ID int64
}

// B1 is Django's delete.B1.
type B1 struct {
	ID        int64
	DeleteTop orm.FK[DeleteTop] `orm:"on_delete=cascade"`
}

// B2 is Django's delete.B2.
type B2 struct {
	ID        int64
	DeleteTop orm.FK[DeleteTop] `orm:"on_delete=cascade"`
}

// DeleteBottom is Django's delete.DeleteBottom.
type DeleteBottom struct {
	ID int64
	B1 orm.FK[B1] `orm:"on_delete=restrict"`
	B2 orm.FK[B2] `orm:"on_delete=cascade"`
}

// HiddenUser is Django's delete.HiddenUser.
type HiddenUser struct {
	ID int64
	R  orm.FK[R] `orm:"on_delete=cascade"`
}

// HiddenUserProfile is Django's delete.HiddenUserProfile.
type HiddenUserProfile struct {
	ID   int64
	User orm.FK[HiddenUser] `orm:"on_delete=cascade"`
}

// deleteModels returns the models of Django's delete test app.
func deleteModels() []any {
	return []any{djangotest.App("delete",
		&P{}, &R{}, &S{}, &T{}, &U{}, &A{}, &B{}, &B3{},
		&Base{}, &RelToBase{},
		&DeleteTop{}, &B1{}, &B2{}, &DeleteBottom{},
		&HiddenUser{}, &HiddenUserProfile{},
	)}
}

// createA mirrors Django's tests/delete/models.py::create_a.
func createA(t *testing.T, db *orm.DB, name string) *A {
	t.Helper()
	newR := func() orm.FK[R] {
		return fkTo[R](create(t, db, &R{}).ID)
	}
	return create(t, db, &A{
		Name:            name,
		Auto:            newR(),
		AutoNullable:    newR(),
		SetNull:         newR(),
		Cascade:         newR(),
		CascadeNullable: newR(),
		Protect:         newR(),
		Restrict:        newR(),
		DoNothing:       newR(),
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_auto
func TestDeleteAuto(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "auto")
		if _, err := orm.Query[R](db).Filter("pk", a.Auto.PK()).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		exists, err := orm.Query[A](db).Filter("name", "auto").Exists(ctx)
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		djangotest.AssertEqual(t, exists, false)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_auto_nullable
func TestDeleteAutoNullable(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "auto_nullable")
		if _, err := orm.Query[R](db).Filter("pk", a.AutoNullable.PK()).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		exists, err := orm.Query[A](db).Filter("name", "auto_nullable").Exists(ctx)
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		djangotest.AssertEqual(t, exists, false)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_setnull
func TestDeleteSetNull(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "setnull")
		if _, err := orm.Query[R](db).Filter("pk", a.SetNull.PK()).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		got, err := orm.Query[A](db).Get(ctx, "pk", a.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.SetNull.PK(), int64(0))
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_cascade
func TestDeleteCascade(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "cascade")
		if _, err := orm.Query[R](db).Filter("pk", a.Cascade.PK()).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		exists, err := orm.Query[A](db).Filter("name", "cascade").Exists(ctx)
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		djangotest.AssertEqual(t, exists, false)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_cascade_nullable
func TestDeleteCascadeNullable(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "cascade_nullable")
		if _, err := orm.Query[R](db).Filter("pk", a.CascadeNullable.PK()).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		exists, err := orm.Query[A](db).Filter("name", "cascade_nullable").Exists(ctx)
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		djangotest.AssertEqual(t, exists, false)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_protect
func TestDeleteProtect(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "protect")
		_, err := orm.Query[R](db).Filter("pk", a.Protect.PK()).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrProtected)
		djangotest.AssertErrorIs(t, err, orm.ErrIntegrity)
		var pe *orm.ProtectedError
		if !errors.As(err, &pe) {
			t.Fatalf("err = %v (%T), want *orm.ProtectedError", err, err)
		}
		const want = "Cannot delete some instances of model 'R' because they are referenced through protected foreign keys: 'A.protect'."
		djangotest.AssertEqual(t, pe.Error(), want)
		djangotest.AssertEqual(t, len(pe.ProtectedObjects), 1)
		djangotest.AssertEqual[any](t, pe.ProtectedObjects[0], *a)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_protect_multiple
func TestDeleteProtectMultiple(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "protect")
		b := create(t, db, &B{Protect: a.Protect})
		_, err := orm.Query[R](db).Filter("pk", a.Protect.PK()).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrProtected)
		var pe *orm.ProtectedError
		if !errors.As(err, &pe) {
			t.Fatalf("err = %v (%T), want *orm.ProtectedError", err, err)
		}
		const want = "Cannot delete some instances of model 'R' because they are referenced through protected foreign keys: 'A.protect', 'B.protect'."
		djangotest.AssertEqual(t, pe.Error(), want)
		djangotest.AssertEqual(t, len(pe.ProtectedObjects), 2)
		djangotest.AssertEqual[any](t, pe.ProtectedObjects[0], *a)
		djangotest.AssertEqual[any](t, pe.ProtectedObjects[1], *b)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_protect_path
func TestDeleteProtectPath(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "protect")
		p := create(t, db, &P{})
		if _, err := orm.Query[R](db).Filter("pk", a.Protect.PK()).Update(ctx, "p", p); err != nil {
			t.Fatalf("Update: %v", err)
		}
		_, err := orm.Query[P](db).Filter("pk", p.ID).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrProtected)
		var pe *orm.ProtectedError
		if !errors.As(err, &pe) {
			t.Fatalf("err = %v (%T), want *orm.ProtectedError", err, err)
		}
		const want = "Cannot delete some instances of model 'P' because they are referenced through protected foreign keys: 'R.p'."
		djangotest.AssertEqual(t, pe.Error(), want)
		djangotest.AssertEqual(t, len(pe.ProtectedObjects), 1)
		djangotest.AssertEqual[any](t, pe.ProtectedObjects[0], *a)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_do_nothing
func TestDeleteDoNothing(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "do_nothing")

		// Without a pre-delete fixup, the database FK constraint rejects deletion.
		_, err := orm.Query[R](db).Filter("pk", a.DoNothing.PK()).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrForeignKeyViolation)

		replacement := create(t, db, &R{})
		cancel := orm.OnPreDelete[R](func(ctx context.Context, obj *R) error {
			_, err := orm.Query[A](db).Filter("do_nothing", obj.ID).Update(ctx, "do_nothing", replacement)
			return err
		})
		defer cancel()

		if _, err := orm.Query[R](db).Filter("pk", a.DoNothing.PK()).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		got, err := orm.Query[A](db).Get(ctx, "pk", a.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.DoNothing.PK(), replacement.ID)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_do_nothing_qscount
func TestDeleteDoNothingQSCount(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		b := create(t, db, &Base{})
		djangotest.AssertNumQueries(t, db, 1, func() {
			if _, err := orm.Query[Base](db).Filter("pk", b.ID).Delete(ctx); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[Base](db)), int64(0))
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_restrict
func TestDeleteRestrict(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "restrict")
		_, err := orm.Query[R](db).Filter("pk", a.Restrict.PK()).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrRestricted)
		djangotest.AssertErrorIs(t, err, orm.ErrIntegrity)
		var re *orm.RestrictedError
		if !errors.As(err, &re) {
			t.Fatalf("err = %v (%T), want *orm.RestrictedError", err, err)
		}
		const want = "Cannot delete some instances of model 'R' because they are referenced through restricted foreign keys: 'A.restrict'."
		djangotest.AssertEqual(t, re.Error(), want)
		djangotest.AssertEqual(t, len(re.RestrictedObjects), 1)
		djangotest.AssertEqual[any](t, re.RestrictedObjects[0], *a)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_restrict_multiple
func TestDeleteRestrictMultiple(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "restrict")
		b3 := create(t, db, &B3{Restrict: a.Restrict})
		_, err := orm.Query[R](db).Filter("pk", a.Restrict.PK()).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrRestricted)
		var re *orm.RestrictedError
		if !errors.As(err, &re) {
			t.Fatalf("err = %v (%T), want *orm.RestrictedError", err, err)
		}
		const want = "Cannot delete some instances of model 'R' because they are referenced through restricted foreign keys: 'A.restrict', 'B3.restrict'."
		djangotest.AssertEqual(t, re.Error(), want)
		djangotest.AssertEqual(t, len(re.RestrictedObjects), 2)
		djangotest.AssertEqual[any](t, re.RestrictedObjects[0], *a)
		djangotest.AssertEqual[any](t, re.RestrictedObjects[1], *b3)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_restrict_path_cascade_indirect
func TestDeleteRestrictPathCascadeIndirect(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "restrict")
		p := create(t, db, &P{})
		if _, err := orm.Query[R](db).Filter("pk", a.Restrict.PK()).Update(ctx, "p", p); err != nil {
			t.Fatalf("Update restrict.p: %v", err)
		}
		_, err := orm.Query[P](db).Filter("pk", p.ID).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrRestricted)
		var re *orm.RestrictedError
		if !errors.As(err, &re) {
			t.Fatalf("err = %v (%T), want *orm.RestrictedError", err, err)
		}
		const want = "Cannot delete some instances of model 'P' because they are referenced through restricted foreign keys: 'A.restrict'."
		djangotest.AssertEqual(t, re.Error(), want)
		djangotest.AssertEqual(t, len(re.RestrictedObjects), 1)
		djangotest.AssertEqual[any](t, re.RestrictedObjects[0], *a)

		// Object referenced also via a CASCADE path can be deleted.
		if _, err := orm.Query[R](db).Filter("pk", a.Cascade.PK()).Update(ctx, "p", p); err != nil {
			t.Fatalf("Update cascade.p: %v", err)
		}
		if _, err := orm.Query[P](db).Filter("pk", p.ID).Delete(ctx); err != nil {
			t.Fatalf("Delete P with cascade path: %v", err)
		}
		existsA, _ := orm.Query[A](db).Filter("name", "restrict").Exists(ctx)
		existsR, _ := orm.Query[R](db).Filter("pk", a.Restrict.PK()).Exists(ctx)
		djangotest.AssertEqual(t, existsA, false)
		djangotest.AssertEqual(t, existsR, false)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_restrict_path_cascade_direct
func TestDeleteRestrictPathCascadeDirect(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := createA(t, db, "restrict")
		p := create(t, db, &P{})
		if _, err := orm.Query[R](db).Filter("pk", a.Restrict.PK()).Update(ctx, "p", p); err != nil {
			t.Fatalf("Update restrict.p: %v", err)
		}
		if _, err := orm.Query[A](db).Filter("pk", a.ID).Update(ctx, "cascade_p", p); err != nil {
			t.Fatalf("Update cascade_p: %v", err)
		}
		if _, err := orm.Query[P](db).Filter("pk", p.ID).Delete(ctx); err != nil {
			t.Fatalf("Delete P: %v", err)
		}
		existsA, _ := orm.Query[A](db).Filter("name", "restrict").Exists(ctx)
		existsR, _ := orm.Query[R](db).Filter("pk", a.Restrict.PK()).Exists(ctx)
		djangotest.AssertEqual(t, existsA, false)
		djangotest.AssertEqual(t, existsR, false)
	})
}

// Ported from Django: tests/delete/tests.py::OnDeleteTests.test_restrict_path_cascade_indirect_diamond
func TestDeleteRestrictPathCascadeIndirectDiamond(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		top := create(t, db, &DeleteTop{})
		b1 := create(t, db, &B1{DeleteTop: fkTo[DeleteTop](top.ID)})
		b2 := create(t, db, &B2{DeleteTop: fkTo[DeleteTop](top.ID)})
		bottom := create(t, db, &DeleteBottom{B1: fkTo[B1](b1.ID), B2: fkTo[B2](b2.ID)})

		_, err := orm.Query[B1](db).Filter("pk", b1.ID).Delete(ctx)
		djangotest.AssertErrorIs(t, err, orm.ErrRestricted)
		var re *orm.RestrictedError
		if !errors.As(err, &re) {
			t.Fatalf("err = %v (%T), want *orm.RestrictedError", err, err)
		}
		const want = "Cannot delete some instances of model 'B1' because they are referenced through restricted foreign keys: 'DeleteBottom.b1'."
		djangotest.AssertEqual(t, re.Error(), want)
		djangotest.AssertEqual(t, len(re.RestrictedObjects), 1)
		djangotest.AssertEqual[any](t, re.RestrictedObjects[0], *bottom)

		djangotest.AssertEqual(t, mustCount(t, orm.Query[DeleteTop](db)), int64(1))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[B1](db)), int64(1))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[B2](db)), int64(1))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[DeleteBottom](db)), int64(1))

		// Deleting DeleteTop cascades to B2 and DeleteBottom, allowing B1's deletion.
		if _, err := orm.Query[DeleteTop](db).Filter("pk", top.ID).Delete(ctx); err != nil {
			t.Fatalf("Delete DeleteTop: %v", err)
		}
		djangotest.AssertEqual(t, mustCount(t, orm.Query[DeleteTop](db)), int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[B1](db)), int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[B2](db)), int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[DeleteBottom](db)), int64(0))
	})
}

// Ported from Django: tests/delete/tests.py::DeletionTests.test_sliced_queryset
func TestDeleteSlicedQuerySet(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := orm.Query[R](db).Limit(5).Delete(ctx)
		if err == nil {
			t.Fatal("Delete on sliced QuerySet succeeded, want error")
		}
		djangotest.AssertEqual(t, err.Error(), "orm: cannot use 'limit' or 'offset' with delete()")
	})
}

// Ported from Django: tests/delete/tests.py::DeletionTests.test_bulk
func TestDeleteBulk(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		r := create(t, db, &R{})
		s := create(t, db, &S{R: fkTo[R](r.ID)})
		const total = 1200
		batch := make([]*T, total)
		for i := range batch {
			batch[i] = &T{S: fkTo[S](s.ID)}
		}
		if err := orm.Query[T](db).BulkCreate(ctx, batch); err != nil {
			t.Fatalf("BulkCreate: %v", err)
		}
		n, err := orm.Query[S](db).Filter("pk", s.ID).Delete(ctx)
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(total+1))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[S](db)), int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[T](db)), int64(0))
	})
}

// Ported from Django: tests/delete/tests.py::DeletionTests.test_queryset_delete_returns_num_rows
func TestDeleteQuerySetReturnsNumRows(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		r := create(t, db, &R{})
		h1 := create(t, db, &HiddenUser{R: fkTo[R](r.ID)})
		create(t, db, &HiddenUser{R: fkTo[R](r.ID)})
		create(t, db, &HiddenUserProfile{User: fkTo[HiddenUser](h1.ID)})

		deleted, err := orm.Query[R](db).Delete(ctx)
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		// 1 R + 2 HiddenUser + 1 HiddenUserProfile = 4 rows deleted.
		djangotest.AssertEqual(t, deleted, int64(4))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[R](db)), int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[HiddenUser](db)), int64(0))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[HiddenUserProfile](db)), int64(0))
	})
}

// Ported from Django: tests/delete/tests.py::DeletionTests.test_relational_post_delete_signals_happen_before_parent_object
func TestDeleteRelationalPostDeleteBeforeParent(t *testing.T) {
	djangotest.EachDB(t, deleteModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		r := create(t, db, &R{})
		s := create(t, db, &S{R: fkTo[R](r.ID)})

		var seen []int64
		cancel := orm.OnPostDelete[S](func(ctx context.Context, obj *S) error {
			exists, err := orm.Query[R](db).Filter("pk", obj.R.PK()).Exists(ctx)
			if err != nil {
				return err
			}
			djangotest.AssertEqual(t, exists, true)
			seen = append(seen, obj.ID)
			return nil
		})
		defer cancel()

		if _, err := orm.Query[R](db).Filter("pk", r.ID).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		djangotest.AssertEqual(t, seen, []int64{s.ID})
	})
}
