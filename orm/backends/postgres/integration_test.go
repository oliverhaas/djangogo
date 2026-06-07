package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
)

// Person is the model used by the PostgreSQL integration round-trip test. It
// mirrors the model used by the SQLite execution-layer tests so both backends
// exercise the same Create/Get/Filter/Update/Delete surface.
type Person struct {
	ID     int64
	Name   string `orm:"max_length=100"`
	Age    int64
	Active bool
}

// newPersonDB reads DJANGOGO_TEST_POSTGRES_DSN (skipping when unset), opens the
// database, drops and recreates the person table, and returns the DB handle.
func newPersonDB(t *testing.T) *orm.DB {
	t.Helper()

	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}

	reg := orm.NewRegistry()
	if _, err := reg.Register(&Person{}); err != nil {
		t.Fatalf("Register(Person): %v", err)
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

	dropPersonTable(t, sdb)

	model, ok := reg.Get("Person")
	if !ok {
		t.Fatal("Person model not found in registry")
	}

	db := orm.NewDB(sdb, postgres.New(), reg)
	if err := db.CreateTable(ctx, model); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	t.Cleanup(func() { dropPersonTable(t, sdb) })

	return db
}

// dropPersonTable drops the person table if present, ignoring "does not exist".
func dropPersonTable(t *testing.T, sdb *sql.DB) {
	t.Helper()

	if _, err := sdb.ExecContext(context.Background(), `DROP TABLE IF EXISTS "person"`); err != nil {
		t.Fatalf("DROP TABLE person: %v", err)
	}
}

// seedPeople creates three Person rows and returns them with their assigned IDs.
func seedPeople(t *testing.T, db *orm.DB) []Person {
	t.Helper()
	ctx := context.Background()
	people := []Person{
		{Name: "Alice", Age: 30, Active: true},
		{Name: "Bob", Age: 40, Active: false},
		{Name: "Carol", Age: 50, Active: true},
	}
	for i := range people {
		if err := orm.Query[Person](db).Create(ctx, &people[i]); err != nil {
			t.Fatalf("Create(%s): %v", people[i].Name, err)
		}
		if people[i].ID == 0 {
			t.Fatalf("Create(%s): expected non-zero auto PK via RETURNING, got 0", people[i].Name)
		}
	}
	return people
}

func TestIntegration_RoundTrip(t *testing.T) {
	db := newPersonDB(t)
	ctx := context.Background()
	people := seedPeople(t, db)

	// Create wrote back distinct, non-zero auto IDs via RETURNING.
	if people[0].ID == people[1].ID || people[1].ID == people[2].ID {
		t.Fatalf("expected distinct auto IDs, got %d, %d, %d", people[0].ID, people[1].ID, people[2].ID)
	}
	t.Logf("RETURNING auto IDs: %d, %d, %d", people[0].ID, people[1].ID, people[2].ID)

	// All returns every row with correct field values.
	all, err := orm.Query[Person](db).OrderBy("id").All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("All: got %d rows, want 3", len(all))
	}
	if all[0].Name != "Alice" || all[0].Age != 30 || !all[0].Active {
		t.Fatalf("All[0] mismatch: %+v", all[0])
	}
	if all[1].Name != "Bob" || all[1].Age != 40 || all[1].Active {
		t.Fatalf("All[1] mismatch: %+v", all[1])
	}
	if all[2].Name != "Carol" || all[2].Age != 50 || !all[2].Active {
		t.Fatalf("All[2] mismatch: %+v", all[2])
	}

	// Get by id returns the right row.
	got, err := orm.Query[Person](db).Get(ctx, "id", people[1].ID)
	if err != nil {
		t.Fatalf("Get(Bob): %v", err)
	}
	if got.Name != "Bob" {
		t.Fatalf("Get(Bob): got %q", got.Name)
	}

	// Get on a missing id returns ErrDoesNotExist.
	if _, err := orm.Query[Person](db).Get(ctx, "id", int64(99999)); !errors.Is(err, orm.ErrDoesNotExist) {
		t.Fatalf("Get(missing): got %v, want ErrDoesNotExist", err)
	}

	// Get whose filter matches >1 returns ErrMultipleObjectsReturned.
	if _, err := orm.Query[Person](db).Get(ctx, "active", true); !errors.Is(err, orm.ErrMultipleObjectsReturned) {
		t.Fatalf("Get(active=true): got %v, want ErrMultipleObjectsReturned", err)
	}
}

func TestIntegration_FilterOrderLimitOffset(t *testing.T) {
	db := newPersonDB(t)
	ctx := context.Background()
	seedPeople(t, db)

	older, err := orm.Query[Person](db).Filter("age__gt", int64(35)).OrderBy("age").All(ctx)
	if err != nil {
		t.Fatalf("Filter(age__gt=35): %v", err)
	}
	if len(older) != 2 || older[0].Name != "Bob" || older[1].Name != "Carol" {
		t.Fatalf("Filter(age__gt=35): got %+v", older)
	}

	active, err := orm.Query[Person](db).Filter("active", true).OrderBy("age").All(ctx)
	if err != nil {
		t.Fatalf("Filter(active=true): %v", err)
	}
	if len(active) != 2 || active[0].Name != "Alice" || active[1].Name != "Carol" {
		t.Fatalf("Filter(active=true): got %+v", active)
	}

	desc, err := orm.Query[Person](db).OrderBy("-age").All(ctx)
	if err != nil {
		t.Fatalf("OrderBy(-age): %v", err)
	}
	if desc[0].Name != "Carol" || desc[1].Name != "Bob" || desc[2].Name != "Alice" {
		t.Fatalf("OrderBy(-age): got %+v", desc)
	}

	page, err := orm.Query[Person](db).OrderBy("age").Limit(1).Offset(1).All(ctx)
	if err != nil {
		t.Fatalf("Limit/Offset: %v", err)
	}
	if len(page) != 1 || page[0].Name != "Bob" {
		t.Fatalf("Limit(1).Offset(1): got %+v", page)
	}
}

func TestIntegration_CountAndExists(t *testing.T) {
	db := newPersonDB(t)
	ctx := context.Background()
	seedPeople(t, db)

	n, err := orm.Query[Person](db).Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 3 {
		t.Fatalf("Count: got %d, want 3", n)
	}

	n, err = orm.Query[Person](db).Filter("active", true).Count(ctx)
	if err != nil {
		t.Fatalf("Count(active): %v", err)
	}
	if n != 2 {
		t.Fatalf("Count(active): got %d, want 2", n)
	}

	exists, err := orm.Query[Person](db).Filter("name", "Alice").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists(Alice): %v", err)
	}
	if !exists {
		t.Fatal("Exists(Alice): got false, want true")
	}

	exists, err = orm.Query[Person](db).Filter("name", "Nobody").Exists(ctx)
	if err != nil {
		t.Fatalf("Exists(Nobody): %v", err)
	}
	if exists {
		t.Fatal("Exists(Nobody): got true, want false")
	}
}

func TestIntegration_Update(t *testing.T) {
	db := newPersonDB(t)
	ctx := context.Background()
	people := seedPeople(t, db)

	n, err := orm.Query[Person](db).Filter("id", people[0].ID).Update(ctx, "age", int64(99))
	if err != nil {
		t.Fatalf("Update(Alice.age=99): %v", err)
	}
	if n != 1 {
		t.Fatalf("Update rowsAffected: got %d, want 1", n)
	}
	got, err := orm.Query[Person](db).Get(ctx, "id", people[0].ID)
	if err != nil {
		t.Fatalf("Get(Alice): %v", err)
	}
	if got.Age != 99 {
		t.Fatalf("after Update, Alice.Age = %d, want 99", got.Age)
	}

	n, err = orm.Query[Person](db).Update(ctx, "active", false)
	if err != nil {
		t.Fatalf("Update(all active=false): %v", err)
	}
	if n != 3 {
		t.Fatalf("Update(all) rowsAffected: got %d, want 3", n)
	}
	stillActive, err := orm.Query[Person](db).Filter("active", true).Count(ctx)
	if err != nil {
		t.Fatalf("Count(active): %v", err)
	}
	if stillActive != 0 {
		t.Fatalf("after unfiltered Update, %d rows still active, want 0", stillActive)
	}
}

func TestIntegration_Delete(t *testing.T) {
	db := newPersonDB(t)
	ctx := context.Background()
	people := seedPeople(t, db)

	n, err := orm.Query[Person](db).Filter("id", people[1].ID).Delete(ctx)
	if err != nil {
		t.Fatalf("Delete(Bob): %v", err)
	}
	if n != 1 {
		t.Fatalf("Delete rowsAffected: got %d, want 1", n)
	}

	if _, err := orm.Query[Person](db).Get(ctx, "id", people[1].ID); !errors.Is(err, orm.ErrDoesNotExist) {
		t.Fatalf("Get(deleted Bob): got %v, want ErrDoesNotExist", err)
	}

	remaining, err := orm.Query[Person](db).Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("after Delete, Count = %d, want 2", remaining)
	}
}
