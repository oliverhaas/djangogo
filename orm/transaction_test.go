package orm_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// Widget is the model used by the transaction and signal tests.
type Widget struct {
	ID   int64
	Name string `orm:"max_length=100"`
	Qty  int64
}

// newWidgetDB builds a frozen registry, opens an isolated in-memory SQLite
// database pinned to the test name, creates the widget table, and returns the
// DB handle.
func newWidgetDB(t *testing.T) *orm.DB {
	t.Helper()

	reg := orm.NewRegistry()
	if _, err := reg.Register(&Widget{}); err != nil {
		t.Fatalf("Register(Widget): %v", err)
	}
	reg.Freeze()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	sdb, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	db := orm.NewDB(sdb, sqlite.New(), reg)
	t.Cleanup(func() { _ = sdb.Close() })

	model, ok := reg.Get("Widget")
	if !ok {
		t.Fatal("Widget model not found in registry")
	}
	if err := db.CreateTable(context.Background(), model); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	return db
}

// newWidgetPostgresDB reads DJANGOGO_TEST_POSTGRES_DSN (skipping when unset),
// opens the database, drops and recreates the widget table, and returns the DB.
func newWidgetPostgresDB(t *testing.T) *orm.DB {
	t.Helper()

	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping PostgreSQL transaction test")
	}

	reg := orm.NewRegistry()
	if _, err := reg.Register(&Widget{}); err != nil {
		t.Fatalf("Register(Widget): %v", err)
	}
	reg.Freeze()

	sdb, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })

	ctx := context.Background()
	if err := sdb.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	dropWidgetTable(t, sdb)

	model, ok := reg.Get("Widget")
	if !ok {
		t.Fatal("Widget model not found in registry")
	}
	db := orm.NewDB(sdb, postgres.New(), reg)
	if err := db.CreateTable(ctx, model); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	t.Cleanup(func() { dropWidgetTable(t, sdb) })
	return db
}

// dropWidgetTable drops the widget table if present.
func dropWidgetTable(t *testing.T, sdb *sql.DB) {
	t.Helper()
	if _, err := sdb.ExecContext(context.Background(), `DROP TABLE IF EXISTS "widget"`); err != nil {
		t.Fatalf("DROP TABLE widget: %v", err)
	}
}

// createWidget inserts a widget with the given name and returns its ID.
func createWidget(ctx context.Context, t *testing.T, db *orm.DB, name string) int64 {
	t.Helper()
	w := Widget{Name: name, Qty: 1}
	if err := orm.Query[Widget](db).Create(ctx, &w); err != nil {
		t.Fatalf("Create(%s): %v", name, err)
	}
	return w.ID
}

// countWidgets returns the total number of widget rows.
func countWidgets(ctx context.Context, t *testing.T, db *orm.DB) int64 {
	t.Helper()
	n, err := orm.Query[Widget](db).Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	return n
}

func TestAtomicCommit(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()

	err := db.Atomic(ctx, func(ctx context.Context) error {
		createWidget(ctx, t, db, "a")
		createWidget(ctx, t, db, "b")
		return nil
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
	if n := countWidgets(ctx, t, db); n != 2 {
		t.Fatalf("after commit, Count = %d, want 2", n)
	}
}

func TestAtomicRollbackOnError(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()

	boom := errors.New("boom")
	err := db.Atomic(ctx, func(ctx context.Context) error {
		createWidget(ctx, t, db, "a")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Atomic: got %v, want boom", err)
	}
	if n := countWidgets(ctx, t, db); n != 0 {
		t.Fatalf("after rollback, Count = %d, want 0", n)
	}
}

func TestAtomicPanicRollsBackAndRepanics(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("Atomic did not re-panic")
			}
			if s, ok := r.(string); !ok || s != "kaboom" {
				t.Fatalf("recovered %v, want \"kaboom\"", r)
			}
		}()
		_ = db.Atomic(ctx, func(ctx context.Context) error {
			createWidget(ctx, t, db, "a")
			panic("kaboom")
		})
	}()

	if n := countWidgets(ctx, t, db); n != 0 {
		t.Fatalf("after panic rollback, Count = %d, want 0", n)
	}
}

func TestAtomicVisibilityWithinTx(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()

	err := db.Atomic(ctx, func(ctx context.Context) error {
		createWidget(ctx, t, db, "a")
		// A read on the SAME ctx must see the uncommitted row via the tx.
		n, err := orm.Query[Widget](db).Count(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("within tx, Count = %d, want 1", n)
		}
		got, err := orm.Query[Widget](db).Get(ctx, "name", "a")
		if err != nil {
			return err
		}
		if got.Name != "a" {
			t.Fatalf("within tx, Get(name=a) = %q", got.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Atomic: %v", err)
	}
}

func TestAtomicNestedSavepoint(t *testing.T) {
	db := newWidgetDB(t)
	ctx := context.Background()
	runNestedSavepointTest(ctx, t, db)
}

func TestAtomicNestedSavepointPostgres(t *testing.T) {
	db := newWidgetPostgresDB(t)
	ctx := context.Background()
	runNestedSavepointTest(ctx, t, db)
}

// runNestedSavepointTest exercises a savepoint-backed nested Atomic: the outer
// transaction creates A and C while a nested Atomic creates B then fails, so the
// inner block rolls back independently and only A and C survive.
func runNestedSavepointTest(ctx context.Context, t *testing.T, db *orm.DB) {
	t.Helper()

	err := db.Atomic(ctx, func(ctx context.Context) error {
		createWidget(ctx, t, db, "A")

		innerErr := db.Atomic(ctx, func(ctx context.Context) error {
			createWidget(ctx, t, db, "B")
			return errors.New("inner boom")
		})
		if innerErr == nil {
			t.Fatal("nested Atomic returned nil, want inner boom")
		}

		createWidget(ctx, t, db, "C")
		return nil
	})
	if err != nil {
		t.Fatalf("outer Atomic: %v", err)
	}

	names := widgetNames(ctx, t, db)
	if got := names["A"]; !got {
		t.Errorf("A missing, want present")
	}
	if got := names["B"]; got {
		t.Errorf("B present, want absent (rolled back via savepoint)")
	}
	if got := names["C"]; !got {
		t.Errorf("C missing, want present")
	}
}

// widgetNames returns the set of widget names currently stored.
func widgetNames(ctx context.Context, t *testing.T, db *orm.DB) map[string]bool {
	t.Helper()
	all, err := orm.Query[Widget](db).All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	names := make(map[string]bool, len(all))
	for _, w := range all {
		names[w.Name] = true
	}
	return names
}
