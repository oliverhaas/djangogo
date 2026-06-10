package orm_test

import (
	"context"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// wp3Widget exercises default=, auto_now_add, and auto_now together.
type wp3Widget struct {
	ID        int64
	Name      string    `orm:"max_length=50;default=unnamed"`
	Count     int64     `orm:"default=7"`
	Active    bool      `orm:"default=true"`
	CreatedAt time.Time `orm:"auto_now_add"`
	UpdatedAt time.Time `orm:"auto_now"`
}

func newWp3WidgetDB(t *testing.T) *orm.DB {
	t.Helper()
	reg := orm.NewRegistry()
	if _, err := reg.Register(&wp3Widget{}); err != nil {
		t.Fatalf("Register(wp3Widget): %v", err)
	}
	reg.Freeze()
	sdb, err := sqlite.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := orm.NewDB(sdb, sqlite.New(), reg)
	m, _ := reg.Get("wp3Widget")
	if err := db.CreateTable(context.Background(), m); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	return db
}

func reloadWp3Widget(t *testing.T, db *orm.DB, id int64) wp3Widget {
	t.Helper()
	w, err := orm.Query[wp3Widget](db).Get(context.Background(), "id", id)
	if err != nil {
		t.Fatalf("reload wp3Widget %d: %v", id, err)
	}
	return w
}

func TestCreateAppliesDefaults(t *testing.T) {
	db := newWp3WidgetDB(t)
	w := &wp3Widget{}
	if err := orm.Query[wp3Widget](db).Create(context.Background(), w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Visible on *obj immediately (Django parity).
	if w.Name != "unnamed" || w.Count != 7 || !w.Active {
		t.Errorf("on obj: Name=%q Count=%d Active=%v, want unnamed/7/true", w.Name, w.Count, w.Active)
	}
	got := reloadWp3Widget(t, db, w.ID)
	if got.Name != "unnamed" || got.Count != 7 || !got.Active {
		t.Errorf("reloaded: Name=%q Count=%d Active=%v, want unnamed/7/true", got.Name, got.Count, got.Active)
	}
}

func TestCreateDefaultDoesNotOverrideNonZero(t *testing.T) {
	db := newWp3WidgetDB(t)
	w := &wp3Widget{Name: "custom", Count: 3}
	if err := orm.Query[wp3Widget](db).Create(context.Background(), w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := reloadWp3Widget(t, db, w.ID)
	if got.Name != "custom" || got.Count != 3 {
		t.Errorf("reloaded: Name=%q Count=%d, want custom/3", got.Name, got.Count)
	}
}

func TestCreateAutoNowStampsBoth(t *testing.T) {
	db := newWp3WidgetDB(t)
	fixed := time.Date(2021, 6, 1, 12, 0, 0, 0, time.UTC)
	db.Now = func() time.Time { return fixed }

	w := &wp3Widget{}
	if err := orm.Query[wp3Widget](db).Create(context.Background(), w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !w.CreatedAt.Equal(fixed) || !w.UpdatedAt.Equal(fixed) {
		t.Errorf("on obj: CreatedAt=%v UpdatedAt=%v, want both %v", w.CreatedAt, w.UpdatedAt, fixed)
	}
	got := reloadWp3Widget(t, db, w.ID)
	if got.CreatedAt.Unix() != fixed.Unix() || got.UpdatedAt.Unix() != fixed.Unix() {
		t.Errorf("reloaded: CreatedAt=%v UpdatedAt=%v, want both %v", got.CreatedAt, got.UpdatedAt, fixed)
	}
}

func TestUpdateTouchesAutoNowButNotAutoNowAdd(t *testing.T) {
	db := newWp3WidgetDB(t)
	created := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	db.Now = func() time.Time { return created }
	w := &wp3Widget{Name: "x"}
	if err := orm.Query[wp3Widget](db).Create(context.Background(), w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := time.Date(2022, 2, 2, 0, 0, 0, 0, time.UTC)
	db.Now = func() time.Time { return updated }
	n, err := orm.Query[wp3Widget](db).Filter("id", w.ID).Update(context.Background(), "name", "y")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows affected = %d, want 1", n)
	}

	got := reloadWp3Widget(t, db, w.ID)
	if got.UpdatedAt.Unix() != updated.Unix() {
		t.Errorf("UpdatedAt = %v, want %v (auto_now should advance)", got.UpdatedAt, updated)
	}
	if got.CreatedAt.Unix() != created.Unix() {
		t.Errorf("CreatedAt = %v, want %v (auto_now_add must not change on update)", got.CreatedAt, created)
	}
}

func TestUpdateExplicitAutoNowOverrideWins(t *testing.T) {
	db := newWp3WidgetDB(t)
	db.Now = func() time.Time { return time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC) }
	w := &wp3Widget{Name: "x"}
	if err := orm.Query[wp3Widget](db).Create(context.Background(), w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	explicit := time.Date(2030, 5, 5, 0, 0, 0, 0, time.UTC)
	if _, err := orm.Query[wp3Widget](db).Filter("id", w.ID).Update(context.Background(), "updated_at", explicit); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := reloadWp3Widget(t, db, w.ID)
	if got.UpdatedAt.Unix() != explicit.Unix() {
		t.Errorf("UpdatedAt = %v, want explicit %v (caller override should win)", got.UpdatedAt, explicit)
	}
}
