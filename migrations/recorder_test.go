package migrations_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// legacyTable is the tracking table of the releases before per-app migration
// histories. Its timestamp column is "applied_at".
const legacyTable = "djangogo_migrations"

// createLegacyTable creates the tracking table with the statement the earlier
// releases used.
func createLegacyTable(t *testing.T, db *orm.DB) {
	t.Helper()
	d := db.Dialect()
	cols := []string{
		d.Quote("id") + " " + d.ColumnType(&orm.Field{Kind: orm.KindAuto, PrimaryKey: true}),
		d.Quote("app") + " " + d.ColumnType(&orm.Field{Kind: orm.KindChar, MaxLength: 255}),
		d.Quote("name") + " " + d.ColumnType(&orm.Field{Kind: orm.KindChar, MaxLength: 255}),
		d.Quote("applied_at") + " " + d.ColumnType(&orm.Field{Kind: orm.KindDateTime}),
	}
	mustExec(t, db, "CREATE TABLE "+d.Quote(legacyTable)+" ("+strings.Join(cols, ", ")+")")
}

// hasTable returns what Recorder.HasTable reports.
func hasTable(t *testing.T, r *migrations.Recorder) bool {
	t.Helper()
	ok, err := r.HasTable(context.Background())
	if err != nil {
		t.Fatalf("HasTable: %v", err)
	}
	return ok
}

// The tracking table has Django's name and columns. Reading the applied
// migrations of a new database does not create it.
func TestRecorderEnsureSchema(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		recorder := migrations.NewRecorder(db)
		djangotest.AssertEqual(t, hasTable(t, recorder), false)
		djangotest.AssertEqual(t, len(appliedFor(t, recorder, "myapp")), 0)
		djangotest.AssertTableNotExists(t, db, "django_migrations")

		// The second call finds the table and leaves it alone.
		for range 2 {
			if err := recorder.EnsureSchema(ctx); err != nil {
				t.Fatalf("EnsureSchema: %v", err)
			}
		}
		djangotest.AssertEqual(t, hasTable(t, recorder), true)
		for _, column := range []string{"id", "app", "name", "applied"} {
			djangotest.AssertColumnExists(t, db, "django_migrations", column)
		}
		djangotest.AssertTableNotExists(t, db, legacyTable)
	})
}

// A record survives the round trip with the time it was written.
func TestRecorderAppliedTime(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		recorder := migrations.NewRecorder(db)
		before := time.Now().Add(-time.Second)
		if err := recorder.RecordApplied(ctx, k("myapp", "0001_initial")); err != nil {
			t.Fatalf("RecordApplied: %v", err)
		}
		applied, err := recorder.AppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("AppliedMigrations: %v", err)
		}
		at, ok := applied[k("myapp", "0001_initial")]
		if !ok {
			t.Fatalf("applied = %v, want myapp.0001_initial", applied)
		}
		if at.Before(before) || at.After(time.Now().Add(time.Second)) {
			t.Errorf("applied at %v, want about %v", at, time.Now().UTC())
		}

		if err := recorder.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		djangotest.AssertEqual(t, len(appliedFor(t, recorder, "myapp")), 0)
	})
}

// The table of an earlier release is read as it is, and the first write
// renames it and its timestamp column. The records are kept.
func TestRecorderUpgradesLegacyTable(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		at := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
		createLegacyTable(t, db)
		mustExec(t, db, "INSERT INTO djangogo_migrations (app, name, applied_at) VALUES (?, ?, ?)",
			"blog", "0001_initial", at)

		recorder := migrations.NewRecorder(db)
		djangotest.AssertEqual(t, hasTable(t, recorder), true)
		applied, err := recorder.AppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("AppliedMigrations: %v", err)
		}
		djangotest.AssertEqual(t, len(applied), 1)
		if got := applied[k("blog", "0001_initial")]; !got.Equal(at) {
			t.Errorf("applied at %v, want %v", got, at)
		}
		// Reading changes nothing.
		djangotest.AssertTableExists(t, db, legacyTable)
		djangotest.AssertTableNotExists(t, db, "django_migrations")

		if err := recorder.RecordApplied(ctx, k("blog", "0002_second")); err != nil {
			t.Fatalf("RecordApplied: %v", err)
		}
		djangotest.AssertTableNotExists(t, db, legacyTable)
		djangotest.AssertColumnExists(t, db, "django_migrations", "applied")
		djangotest.AssertColumnNotExists(t, db, "django_migrations", "applied_at")
		djangotest.AssertSequenceEqual(t, appliedFor(t, recorder, "blog"),
			[]migrations.Key{k("blog", "0001_initial"), k("blog", "0002_second")})
		applied, err = recorder.AppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("AppliedMigrations: %v", err)
		}
		if got := applied[k("blog", "0001_initial")]; !got.Equal(at) {
			t.Errorf("applied at %v after the upgrade, want %v", got, at)
		}
		// The new record got the next id of the old table.
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT max(id) FROM django_migrations"), int64(2))
	})
}

// A database that an earlier release migrated keeps its history: the plan
// holds only the migrations that are not applied, and applying them upgrades
// the tracking table.
func TestMigrateUpgradesLegacyTable(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		migrate(t, newExecutor(t, db, testMigrations()), k("migrations", "0001_initial"))
		// Put the tracking table back into the layout of the earlier release.
		mustExec(t, db, "ALTER TABLE django_migrations RENAME COLUMN applied TO applied_at")
		mustExec(t, db, "ALTER TABLE django_migrations RENAME TO djangogo_migrations")

		executor := newExecutor(t, db, testMigrations())
		djangotest.AssertSequenceEqual(t,
			steps(migrationPlan(t, executor, k("migrations", "0002_second"))),
			[]planned{forwards("migrations", "0002_second")})
		djangotest.AssertTableExists(t, db, legacyTable)

		migrate(t, executor, k("migrations", "0002_second"))
		djangotest.AssertTableNotExists(t, db, legacyTable)
		djangotest.AssertTableExists(t, db, "migrations_book")
		djangotest.AssertTableNotExists(t, db, "migrations_tribble")
		djangotest.AssertSequenceEqual(t, appliedFor(t, migrations.NewRecorder(db), "migrations"),
			[]migrations.Key{k("migrations", "0001_initial"), k("migrations", "0002_second")})

		// Going back to zero works on the upgraded table as well.
		migrate(t, newExecutor(t, db, testMigrations()), zero("migrations"))
		djangotest.AssertTableNotExists(t, db, "migrations_author")
		djangotest.AssertEqual(t, len(appliedFor(t, migrations.NewRecorder(db), "migrations")), 0)
	})
}
