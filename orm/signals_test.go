package orm_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
)

func TestSignalsSaveMutateAndSeePK(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()

	var savedPK int64
	cancelPre := orm.OnPreSave(func(_ context.Context, obj *Widget) error {
		obj.Name = strings.ToUpper(obj.Name)
		return nil
	})
	cancelPost := orm.OnPostSave(func(_ context.Context, obj *Widget) error {
		savedPK = obj.ID
		return nil
	})
	t.Cleanup(cancelPre)
	t.Cleanup(cancelPost)

	w := Widget{Name: "gizmo", Qty: 3}
	if err := orm.Query[Widget](db).Create(ctx, &w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if w.Name != "GIZMO" {
		t.Fatalf("in-memory obj name = %q, want GIZMO (PreSave mutation)", w.Name)
	}
	if savedPK == 0 {
		t.Fatal("PostSave saw zero pk, want non-zero")
	}
	if savedPK != w.ID {
		t.Fatalf("PostSave pk = %d, want %d", savedPK, w.ID)
	}

	got, err := orm.Query[Widget](db).Get(ctx, "id", w.ID)
	if err != nil {
		t.Fatalf("Get(id=%d): %v", w.ID, err)
	}
	if got.Name != "GIZMO" {
		t.Fatalf("stored name = %q, want GIZMO", got.Name)
	}
}

func TestSignalPreSaveErrorAbortsCreate(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()

	boom := errors.New("reject")
	cancel := orm.OnPreSave(func(_ context.Context, _ *Widget) error {
		return boom
	})
	t.Cleanup(cancel)

	w := Widget{Name: "nope", Qty: 1}
	if err := orm.Query[Widget](db).Create(ctx, &w); !errors.Is(err, boom) {
		t.Fatalf("Create: got %v, want reject", err)
	}
	if w.ID != 0 {
		t.Fatalf("obj got pk %d, want 0 (no insert)", w.ID)
	}
	if n := countWidgets(ctx, t, db); n != 0 {
		t.Fatalf("after aborted Create, Count = %d, want 0", n)
	}
}

func TestSignalsDelete(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()

	id := createWidget(ctx, t, db, "doomed")
	other := createWidget(ctx, t, db, "keeper")

	var preIDs, postIDs []int64
	cancelPre := orm.OnPreDelete(func(_ context.Context, obj *Widget) error {
		preIDs = append(preIDs, obj.ID)
		return nil
	})
	cancelPost := orm.OnPostDelete(func(_ context.Context, obj *Widget) error {
		postIDs = append(postIDs, obj.ID)
		return nil
	})
	t.Cleanup(cancelPre)
	t.Cleanup(cancelPost)

	n, err := orm.Query[Widget](db).Filter("id", id).Delete(ctx)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("Delete rowsAffected = %d, want 1", n)
	}

	if len(preIDs) != 1 || preIDs[0] != id {
		t.Fatalf("PreDelete ids = %v, want [%d]", preIDs, id)
	}
	if len(postIDs) != 1 || postIDs[0] != id {
		t.Fatalf("PostDelete ids = %v, want [%d]", postIDs, id)
	}

	// The non-matching row must remain.
	if _, err := orm.Query[Widget](db).Get(ctx, "id", other); err != nil {
		t.Fatalf("Get(keeper): %v", err)
	}
	if got := countWidgets(ctx, t, db); got != 1 {
		t.Fatalf("after Delete, Count = %d, want 1", got)
	}
}
