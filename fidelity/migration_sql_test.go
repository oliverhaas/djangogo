package fidelity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// migrationSQLGoldenFile is the JSON structure written by oracle/migration_sql.py.
type migrationSQLGoldenFile struct {
	DjangoVersion string                   `json:"django_version"`
	Dialect       string                   `json:"dialect"`
	Cases         []migrationSQLGoldenCase `json:"cases"`
}

// migrationSQLGoldenCase is one expected sqlmigrate entry for a dialect.
type migrationSQLGoldenCase struct {
	Name          string                `json:"name"`
	Statements    []string              `json:"statements"`
	SQLiteRebuild bool                  `json:"sqlite_rebuild,omitempty"`
	Schema        *sqliteSchemaSnapshot `json:"schema,omitempty"`
}

// migrationSQLDivergences is the reviewed allowlist of known migration DDL
// differences between Djan-Go-Go and Django 6.0.3 after normalization. SQLite
// table-rebuild cases are compared semantically via PRAGMA schema inspection
// instead of textually. Full details live in fidelity/migration_sql_divergences.md.
var migrationSQLDivergences = map[dialectCaseKey]string{
	// CreateModel column types, PK syntax, implicit NULL, and PG _like indexes.
	{dialect: "sqlite", name: "create_model_basic"}:       "SQLite CreateModel omits redundant NOT NULL on INTEGER PRIMARY KEY and implicit NULL on nullable columns, and writes BOOLEAN instead of bool",
	{dialect: "sqlite", name: "create_model_all_kinds"}:   "SQLite CreateModel omits redundant NOT NULL on INTEGER PRIMARY KEY, writes BOOLEAN instead of bool, and uses single parens in JSON_VALID CHECK",
	{dialect: "postgres", name: "create_model_basic"}:     "PostgreSQL CreateModel uses BIGSERIAL PRIMARY KEY, omits implicit NULL, and omits varchar_pattern_ops _like indexes",
	{dialect: "postgres", name: "create_model_all_kinds"}: "PostgreSQL CreateModel uses BIGSERIAL PRIMARY KEY, BIGINT for KindInt, TIMESTAMPTZ synonym, and omits varchar_pattern_ops _like indexes",

	// Foreign-key DDL style: Djan-Go-Go enforces ON DELETE in SQL (inline table
	// FOREIGN KEY in CreateModel, inline REFERENCES in AddField); Django uses
	// DEFERRABLE INITIALLY DEFERRED without ON DELETE and defers PG FKs.
	{dialect: "*", name: "create_model_fk_and_meta"}: "Djan-Go-Go emits inline table FOREIGN KEY with SQL ON DELETE clauses and orders unique_together after indexes; Django uses DEFERRABLE INITIALLY DEFERRED without ON DELETE",
	{dialect: "*", name: "add_field_fk"}:             "Djan-Go-Go emits REFERENCES with SQL ON DELETE and omits implicit NULL; Django emits DEFERRABLE INITIALLY DEFERRED (and SET CONSTRAINTS IMMEDIATE on PG)",

	// Implicit NULL on nullable columns: Django writes explicit NULL; Djan-Go-Go omits it.
	{dialect: "*", name: "add_field_nullable"}: "Django writes explicit NULL on nullable columns; Djan-Go-Go omits implicit NULL",

	// PostgreSQL ADD COLUMN clause order: Django emits DEFAULT ... NOT NULL;
	// Djan-Go-Go emits NOT NULL DEFAULT ... (and BIGINT for KindInt).
	{dialect: "postgres", name: "add_field_default_int"}:  "PostgreSQL AddField emits BIGINT NOT NULL DEFAULT 5 vs Django's integer DEFAULT 5 NOT NULL",
	{dialect: "postgres", name: "add_field_default_char"}: "PostgreSQL AddField emits NOT NULL DEFAULT 'draft' vs Django's DEFAULT 'draft' NOT NULL",
	{dialect: "postgres", name: "add_field_default_bool"}: "PostgreSQL AddField emits NOT NULL DEFAULT true vs Django's DEFAULT true NOT NULL",

	// PostgreSQL AlterField NULL -> NOT NULL backfill omits SET DEFAULT / SET CONSTRAINTS ALL IMMEDIATE / DROP DEFAULT.
	{dialect: "postgres", name: "alter_field_null_to_not_null"}: "Djan-Go-Go backfills with UPDATE then SET NOT NULL; Django also emits SET DEFAULT, SET CONSTRAINTS ALL IMMEDIATE, and DROP DEFAULT",

	// PostgreSQL varchar_pattern_ops (_like) indexes on unique CharField changes.
	{dialect: "postgres", name: "alter_field_add_unique"}:    "Django PostgreSQL creates a companion varchar_pattern_ops _like index for unique varchar columns; Djan-Go-Go omits it",
	{dialect: "postgres", name: "alter_field_remove_unique"}: "Django PostgreSQL drops the companion varchar_pattern_ops _like index when removing unique; Djan-Go-Go omits it",

	// Derived index rename on RenameModel when the model has FK/field indexes.
	{dialect: "*", name: "rename_model_with_fk"}: "Djan-Go-Go renames derived indexes when a table is renamed so later operations find them without introspection; Django leaves existing index names untouched",
}

// loadMigrationSQLGolden reads and decodes golden/migration_sql/<dialect>.json.
func loadMigrationSQLGolden(t *testing.T, dialect string) map[string]migrationSQLGoldenCase {
	t.Helper()
	path := filepath.Join("golden", "migration_sql", dialect+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var file migrationSQLGoldenFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	byName := make(map[string]migrationSQLGoldenCase, len(file.Cases))
	for _, c := range file.Cases {
		byName[c.Name] = c
	}
	return byName
}

// migrationDivergenceReason returns the allowlisted DDL divergence reason for
// (dialect, name), if any.
func migrationDivergenceReason(dialect, name string) (string, bool) {
	if reason, ok := migrationSQLDivergences[dialectCaseKey{dialect: dialect, name: name}]; ok {
		return reason, true
	}
	if reason, ok := migrationSQLDivergences[dialectCaseKey{dialect: "*", name: name}]; ok {
		return reason, true
	}
	return "", false
}

// buildCaseMigrations turns one migrationCase into a 1- or 2-step migration
// slice registered under app "migfid".
func buildCaseMigrations(tc migrationCase) []migrations.Migration {
	var migs []migrations.Migration
	var deps []migrations.Key
	if len(tc.setup) > 0 {
		migs = append(migs, migrations.Migration{
			App:        "migfid",
			Name:       "0001_setup",
			Initial:    true,
			Operations: tc.setup,
		})
		deps = []migrations.Key{{App: "migfid", Name: "0001_setup"}}
	}
	migs = append(migs, migrations.Migration{
		App:          "migfid",
		Name:         "0002_target",
		Dependencies: deps,
		Initial:      len(deps) == 0,
		Operations:   tc.target,
	})
	return migs
}

// collectGoMigrationSQL collects Djan-Go-Go's sqlmigrate statements for the
// target migration of tc on dialect d without executing them.
func collectGoMigrationSQL(t *testing.T, d orm.Dialect, tc migrationCase) []string {
	t.Helper()
	ctx := context.Background()
	reg := migrations.NewRegistry()
	migs := buildCaseMigrations(tc)
	for _, m := range migs {
		reg.Add(m)
	}
	loader, err := migrations.NewLoader(ctx, reg, nil, nil)
	if err != nil {
		t.Fatalf("NewLoader(%s): %v", tc.name, err)
	}
	target, ok := loader.Migration(migrations.Key{App: "migfid", Name: "0002_target"})
	if !ok {
		t.Fatalf("target migration missing for %s", tc.name)
	}
	raw, err := loader.CollectSQL(ctx, d, []migrations.PlanStep{{Migration: target}})
	if err != nil {
		t.Fatalf("CollectSQL(%s, %s): %v", d.Name(), tc.name, err)
	}
	var stmts []string
	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		stmts = append(stmts, trimmed)
	}
	return stmts
}

// TestMigrationSQLFidelity compares Djan-Go-Go's collected migration SQL
// against Django 6.0.3 goldens on both SQLite and PostgreSQL, and executes
// every case on SQLite to assert resulting schema equivalence (including all
// SQLite table rebuilds).
func TestMigrationSQLFidelity(t *testing.T) {
	cases := canonicalMigrationCases()
	dialects := []struct {
		name    string
		dialect orm.Dialect
		golden  map[string]migrationSQLGoldenCase
	}{
		{name: "sqlite", dialect: sqlite.New(), golden: loadMigrationSQLGolden(t, "sqlite")},
		{name: "postgres", dialect: postgres.New(), golden: loadMigrationSQLGolden(t, "postgres")},
	}

	for _, ds := range dialects {
		t.Run(ds.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					want, ok := ds.golden[tc.name]
					if !ok {
						t.Fatalf("case %q missing from golden/migration_sql/%s.json", tc.name, ds.name)
					}

					gotRaw := collectGoMigrationSQL(t, ds.dialect, tc)
					gotNorm := normalizeMigrationStatements(gotRaw)
					wantNorm := normalizeMigrationStatements(want.Statements)

					// On SQLite, always execute the migrations on a fresh in-memory DB
					// and verify resulting schema equivalence via PRAGMAs.
					isSQLiteRebuild := false
					if ds.name == "sqlite" {
						db := djangotest.NewDB(t)
						if _, err := migrations.Apply(context.Background(), db, buildCaseMigrations(tc)); err != nil {
							t.Fatalf("migrations.Apply(sqlite, %s): %v", tc.name, err)
						}
						if want.Schema == nil {
							t.Fatalf("sqlite golden %q missing schema snapshot", tc.name)
						}
						gotSchema := snapshotSQLiteSchema(t, db.SQL())
						if diffs := compareSQLiteSchemas(gotSchema, *want.Schema); len(diffs) > 0 {
							t.Errorf("SQLite schema mismatch for %q:\n  %s", tc.name, strings.Join(diffs, "\n  "))
						}
						// SQLite table rebuilds are compared semantically via the schema
						// snapshot above rather than textually.
						goRebuild := slices.ContainsFunc(gotRaw, func(s string) bool {
							return strings.Contains(s, `CREATE TABLE "new__`)
						})
						isSQLiteRebuild = want.SQLiteRebuild || goRebuild
					}

					if isSQLiteRebuild {
						return
					}

					match := slices.Equal(gotNorm, wantNorm)
					if reason, allowlisted := migrationDivergenceReason(ds.name, tc.name); allowlisted {
						if match {
							t.Errorf("allowlisted migration divergence %q on %s now matches Django; remove from migrationSQLDivergences (reason was: %s)",
								tc.name, ds.name, reason)
						}
						return
					}

					if !match {
						t.Errorf("migration SQL mismatch on %s for %q\n  got:  %q\n  want: %q\n  raw got:\n    %s\n  raw want:\n    %s",
							ds.name, tc.name, gotNorm, wantNorm,
							strings.Join(gotRaw, "\n    "),
							strings.Join(want.Statements, "\n    "))
					}
				})
			}
		})
	}
}

// TestMigrationPostgresExecution applies every canonical migration case to a
// real PostgreSQL schema when DJANGOGO_TEST_POSTGRES_DSN is configured, proving
// the generated PostgreSQL DDL executes cleanly end to end.
func TestMigrationPostgresExecution(t *testing.T) {
	if os.Getenv(djangotest.PostgresDSNEnv) == "" {
		t.Skipf("%s not set; skipping live PostgreSQL migration execution", djangotest.PostgresDSNEnv)
	}
	for _, tc := range canonicalMigrationCases() {
		t.Run(tc.name, func(t *testing.T) {
			db := djangotest.NewPostgresDB(t)
			if _, err := migrations.Apply(context.Background(), db, buildCaseMigrations(tc)); err != nil {
				t.Fatalf("migrations.Apply(postgres, %s): %v", tc.name, err)
			}
		})
	}
}

// TestMigrationSQLCasesAndAllowlistConsistent ensures Go migration cases, both
// golden files, and the divergence allowlist stay in sync.
func TestMigrationSQLCasesAndAllowlistConsistent(t *testing.T) {
	sqliteGolden := loadMigrationSQLGolden(t, "sqlite")
	postgresGolden := loadMigrationSQLGolden(t, "postgres")
	cases := canonicalMigrationCases()

	goNames := make(map[string]bool, len(cases))
	for _, c := range cases {
		if goNames[c.name] {
			t.Errorf("duplicate Go migration case %q", c.name)
		}
		goNames[c.name] = true
		if _, ok := sqliteGolden[c.name]; !ok {
			t.Errorf("Go migration case %q missing from golden/migration_sql/sqlite.json", c.name)
		}
		if _, ok := postgresGolden[c.name]; !ok {
			t.Errorf("Go migration case %q missing from golden/migration_sql/postgres.json", c.name)
		}
	}
	for name := range sqliteGolden {
		if !goNames[name] {
			t.Errorf("sqlite migration golden %q has no Go test case", name)
		}
	}
	for name := range postgresGolden {
		if !goNames[name] {
			t.Errorf("postgres migration golden %q has no Go test case", name)
		}
	}
	for key := range migrationSQLDivergences {
		if key.dialect != "*" && key.dialect != "sqlite" && key.dialect != "postgres" {
			t.Errorf("migrationSQLDivergences has unknown dialect %q for %q", key.dialect, key.name)
		}
		if !goNames[key.name] {
			t.Errorf("migrationSQLDivergences names %q, which is not a migration case", key.name)
		}
	}
}
