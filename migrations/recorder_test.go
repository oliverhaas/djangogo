package migrations

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// ---------------------------------------------------------------------------
// trackingTableSQL: per-dialect DDL unit tests
// ---------------------------------------------------------------------------

func TestTrackingTableSQL_SQLite(t *testing.T) {
	t.Parallel()
	got := trackingTableSQL(sqlite.New())
	if !strings.Contains(got, "AUTOINCREMENT") {
		t.Errorf("sqlite DDL: want AUTOINCREMENT; got:\n%s", got)
	}
	if !strings.Contains(got, "DATETIME") {
		t.Errorf("sqlite DDL: want DATETIME; got:\n%s", got)
	}
	if !strings.Contains(got, "CREATE TABLE IF NOT EXISTS") {
		t.Errorf("sqlite DDL: want CREATE TABLE IF NOT EXISTS; got:\n%s", got)
	}
	if !strings.Contains(got, `"djangogo_migrations"`) {
		t.Errorf("sqlite DDL: want quoted table name; got:\n%s", got)
	}
}

func TestTrackingTableSQL_Postgres(t *testing.T) {
	t.Parallel()
	got := trackingTableSQL(postgres.New())
	if !strings.Contains(got, "BIGSERIAL") {
		t.Errorf("postgres DDL: want BIGSERIAL; got:\n%s", got)
	}
	if !strings.Contains(got, "TIMESTAMPTZ") {
		t.Errorf("postgres DDL: want TIMESTAMPTZ; got:\n%s", got)
	}
	if !strings.Contains(got, "CREATE TABLE IF NOT EXISTS") {
		t.Errorf("postgres DDL: want CREATE TABLE IF NOT EXISTS; got:\n%s", got)
	}
	if !strings.Contains(got, `"djangogo_migrations"`) {
		t.Errorf("postgres DDL: want quoted table name; got:\n%s", got)
	}
	// Must not contain SQLite-specific syntax.
	if strings.Contains(got, "AUTOINCREMENT") {
		t.Errorf("postgres DDL: must not contain AUTOINCREMENT; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Postgres runner integration test (skipped without DSN)
// ---------------------------------------------------------------------------

// pgMigration returns a CreateModel migration for a model named "MigrationPerson"
// stored in the table "migration_person". Using a distinct table name avoids
// collisions with the orm/backends/postgres integration tests that also use a
// "person" table when the packages are tested concurrently via go test ./...
func pgMigration() Migration {
	fields := []FieldState{
		{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true},
		{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
		{Name: "Age", Column: "age", Kind: orm.KindInt},
	}
	return Migration{
		App:        "app",
		Name:       "0001",
		Operations: []Operation{CreateModel{Name: "MigrationPerson", Table: "migration_person", Fields: fields}},
	}
}

func TestApplyPostgres(t *testing.T) {
	dsn := os.Getenv("DJANGOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DJANGOGO_TEST_POSTGRES_DSN not set; skipping Postgres runner integration test")
	}

	sdb, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })

	ctx := context.Background()
	if err := sdb.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// Clean slate before and after: drop model table and tracking table.
	cleanup := func() {
		for _, stmt := range []string{
			`DROP TABLE IF EXISTS "migration_person"`,
			`DROP TABLE IF EXISTS "djangogo_migrations"`,
		} {
			if _, err := sdb.ExecContext(context.Background(), stmt); err != nil {
				t.Logf("cleanup %q: %v", stmt, err)
			}
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	db := orm.NewDB(sdb, postgres.New(), orm.NewRegistry())
	migs := []Migration{pgMigration()}

	// First Apply: must create the tracking table and the migration_person table.
	done, err := Apply(ctx, db, migs)
	if err != nil {
		t.Fatalf("Apply (first): %v", err)
	}
	if len(done) != 1 || done[0] != "app/0001" {
		t.Errorf("Apply (first) done: got %#v, want [app/0001]", done)
	}

	// The tracking row must be present.
	applied, err := AppliedSet(ctx, db)
	if err != nil {
		t.Fatalf("AppliedSet: %v", err)
	}
	if !applied["app/0001"] {
		t.Errorf("AppliedSet: app/0001 not recorded; got %#v", applied)
	}

	// Second Apply must be a no-op.
	done, err = Apply(ctx, db, migs)
	if err != nil {
		t.Fatalf("Apply (second): %v", err)
	}
	if len(done) != 0 {
		t.Errorf("Apply (second) done: got %#v, want empty (idempotent)", done)
	}

	// The migration_person table exists: insert + select round-trip using $N placeholders.
	if _, err := sdb.ExecContext(ctx,
		`INSERT INTO "migration_person" ("name", "age") VALUES ($1, $2)`, "Ada", 36); err != nil {
		t.Fatalf("insert into migration_person: %v", err)
	}
	var name string
	var age int
	if err := sdb.QueryRowContext(ctx, `SELECT "name", "age" FROM "migration_person"`).Scan(&name, &age); err != nil {
		t.Fatalf("select from migration_person: %v", err)
	}
	if name != "Ada" || age != 36 {
		t.Errorf("round-trip: got (%q, %d), want (Ada, 36)", name, age)
	}
}
