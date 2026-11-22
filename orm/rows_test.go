package orm_test

import (
	"context"
	"fmt"
	"reflect"
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

// contextThing labels itself with a query, as a ContextLabeler that follows a
// relation does.
type contextThing struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

// Label returns the name and the number of rows of the table.
func (c contextThing) Label(ctx context.Context, db *orm.DB) (string, error) {
	n, err := orm.Query[contextThing](db).Count(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s of %d", c.Name, n), nil
}

// No Django test exists: a Django __str__ that follows a relation queries
// without a context. LabeledRows and LabeledRowsOrdered label a row through
// its ContextLabeler, which may query the database.
func TestLabeledRowsContextLabeler(t *testing.T) {
	reg := orm.NewRegistry()
	if _, err := reg.Register(&contextThing{}); err != nil {
		t.Fatal(err)
	}
	reg.Freeze()
	sdb, err := sqlite.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	ctx := context.Background()
	m, _ := reg.Get("contextThing")
	if err := db.CreateTable(ctx, m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		if err := orm.Query[contextThing](db).Create(ctx, &contextThing{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	want := [][2]string{{"1", "a of 2"}, {"2", "b of 2"}}
	err = db.Atomic(ctx, func(ctx context.Context) error {
		got, err := orm.LabeledRows(ctx, db, m)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("LabeledRows = %v, want %v", got, want)
		}
		got, err = orm.LabeledRowsOrdered(ctx, db, m, nil, nil)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("LabeledRowsOrdered = %v, want %v", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	label, err := orm.LabelContext(ctx, db, m, contextThing{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if label != "c of 2" {
		t.Errorf("LabelContext = %q, want %q", label, "c of 2")
	}
}

// No Django test exists: Django's ModelChoiceField iterates its queryset,
// which LabeledQuerySetRows does for code that knows the model only at run
// time, such as the admin's FormfieldQuerySet.
func TestLabeledQuerySetRows(t *testing.T) {
	db, labeled, _ := newLabeledRowsDB(t)
	ctx := context.Background()
	for _, name := range []string{"Beta", "Alpha", "Gamma"} {
		if err := orm.Query[labeledThing](db).Create(ctx, &labeledThing{Name: name}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}
	qs := orm.Query[labeledThing](db).Exclude("name", "Gamma")
	tests := []struct {
		name  string
		qs    orm.AnyQuerySet
		where orm.Cond
		pks   []any
		want  [][2]string
	}{
		{"primary key order", qs, orm.Cond{}, nil, [][2]string{{"1", "Beta"}, {"2", "Alpha"}}},
		{"queryset order", qs.OrderBy("name"), orm.Cond{}, nil, [][2]string{{"2", "Alpha"}, {"1", "Beta"}}},
		{"where", qs, orm.Q("name__startswith", "A"), nil, [][2]string{{"2", "Alpha"}}},
		{"keys", qs, orm.Cond{}, []any{int64(1), int64(3)}, [][2]string{{"1", "Beta"}}},
		{"no keys", qs, orm.Cond{}, []any{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := orm.LabeledQuerySetRows(ctx, labeled, tt.qs, tt.where, tt.pks)
			if err != nil {
				t.Fatalf("LabeledQuerySetRows: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LabeledQuerySetRows = %v, want %v", got, tt.want)
			}
		})
	}
	for name, other := range map[string]orm.AnyQuerySet{
		"other model": orm.Query[plainThing](db),
		"values":      qs.Values("name"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := orm.LabeledQuerySetRows(ctx, labeled, other, orm.Cond{}, nil); err == nil {
				t.Error("LabeledQuerySetRows succeeded, want an error")
			}
		})
	}
}
