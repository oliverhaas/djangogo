package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// labeledThing has a String() method, so LabeledRows must return that text as the
// label rather than the default "labeledThing object (pk)" form.
type labeledThing struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

func (l labeledThing) String() string { return l.Name }

// plainThing has no String(), exercising LabeledRows' default-label fallback.
type plainThing struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

func newLabeledRowsDB(t *testing.T) (*orm.DB, *orm.Model, *orm.Model) {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&labeledThing{}); err != nil {
		t.Fatalf("Register(labeledThing): %v", err)
	}
	if _, err := reg.Register(&plainThing{}); err != nil {
		t.Fatalf("Register(plainThing): %v", err)
	}
	reg.Freeze()

	sdb, err := sqlite.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)

	labeled, _ := reg.Get("labeledThing")
	plain, _ := reg.Get("plainThing")
	for _, m := range []*orm.Model{labeled, plain} {
		if err := db.CreateTable(context.Background(), m); err != nil {
			t.Fatalf("CreateTable(%s): %v", m.Name(), err)
		}
	}
	return db, labeled, plain
}

func TestLabeledRowsUsesStringMethod(t *testing.T) {
	db, labeled, _ := newLabeledRowsDB(t)
	ctx := context.Background()
	for _, name := range []string{"Beta", "Alpha"} {
		if err := orm.Query[labeledThing](db).Create(ctx, &labeledThing{Name: name}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}

	got, err := orm.LabeledRows(ctx, db, labeled)
	if err != nil {
		t.Fatalf("LabeledRows: %v", err)
	}
	// Ordered by primary key, so insertion order: Beta (1), Alpha (2).
	want := [][2]string{{"1", "Beta"}, {"2", "Alpha"}}
	if len(got) != len(want) {
		t.Fatalf("LabeledRows returned %d rows, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestLabeledRowsDefaultLabel(t *testing.T) {
	db, _, plain := newLabeledRowsDB(t)
	ctx := context.Background()
	if err := orm.Query[plainThing](db).Create(ctx, &plainThing{Name: "x"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := orm.LabeledRows(ctx, db, plain)
	if err != nil {
		t.Fatalf("LabeledRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("LabeledRows returned %d rows, want 1: %v", len(got), got)
	}
	if got[0][0] != "1" {
		t.Errorf("pk = %q, want %q", got[0][0], "1")
	}
	// No String() method, so the label is the default "<Name> object (<pk>)" form.
	if want := "plainThing object (1)"; got[0][1] != want {
		t.Errorf("label = %q, want %q", got[0][1], want)
	}
}

func TestLabeledRowsEmpty(t *testing.T) {
	db, labeled, _ := newLabeledRowsDB(t)
	got, err := orm.LabeledRows(context.Background(), db, labeled)
	if err != nil {
		t.Fatalf("LabeledRows: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("LabeledRows on an empty table = %v, want no rows", got)
	}
}
