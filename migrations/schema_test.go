package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/orm/backends/postgres"
	"github.com/oliverhaas/djangogo/orm/backends/sqlite"
)

// heightField returns a NOT NULL integer field named Height.
func heightField() migrations.FieldState {
	return migrations.FieldState{Name: "Height", Column: "height", Kind: orm.KindInt}
}

// A NOT NULL column added to a table with rows is filled from the field
// default, and from the one-off default of the operation when the field has
// none. The database keeps no default: the ORM sets defaults itself.
func TestAddFieldNotNullFillsExistingRows(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := setUpTestModel(t, db, "test_adnn", testModelOptions{})
		mustExec(t, db, "INSERT INTO test_adnn_pony (pink, weight) VALUES (1, 10)")
		mustExec(t, db, "INSERT INTO test_adnn_pony (pink, weight) VALUES (2, 20)")

		withDefault := heightField()
		withDefault.Default = int64(5)
		length := migrations.FieldState{Name: "Length", Column: "length", Kind: orm.KindInt}
		applyOps(t, db, "test_adnn", state,
			migrations.AddField{Model: "Pony", Field: withDefault},
			migrations.AddField{Model: "Pony", Field: length, Default: int64(7)},
		)
		djangotest.AssertColumnNotNull(t, db, "test_adnn_pony", "height")
		djangotest.AssertColumnNotNull(t, db, "test_adnn_pony", "length")
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT count(*) FROM test_adnn_pony WHERE height = 5 AND length = 7"), int64(2))
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT sum(weight) FROM test_adnn_pony"), int64(30))

		// A row written without the columns is refused, so the default did
		// not stay in the database.
		if err := execSQL(db, "INSERT INTO test_adnn_pony (pink, weight) VALUES (3, 30)"); err == nil {
			t.Error("the insert without height and length succeeded; the columns kept a database default")
		}
	})
}

// Without any default there is nothing to fill the rows with. The operation
// fails before it changes the table and names the ways out.
func TestAddFieldNotNullWithoutDefault(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := setUpTestModel(t, db, "test_adnd", testModelOptions{})
		operation := migrations.AddField{Model: "Pony", Field: heightField()}

		// An empty table needs no value.
		newState := applyOps(t, db, "test_adnd", state, operation)
		djangotest.AssertColumnNotNull(t, db, "test_adnd_pony", "height")
		unapplyOps(t, db, "test_adnd", state, operation)
		djangotest.AssertColumnNotExists(t, db, "test_adnd_pony", "height")
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_adnd", "Pony").Fields), 6)

		mustExec(t, db, "INSERT INTO test_adnd_pony (pink, weight) VALUES (1, 10)")
		_, err := tryApplyOps(db, "test_adnd", state, operation)
		if err == nil {
			t.Fatal("adding a NOT NULL column without a default to a table with rows succeeded")
		}
		const want = `migrations: cannot add the non-nullable field "Height" to test_adnd.Pony: the table has rows ` +
			`and the field has no default to fill them with. Give the field a default (default=...), make it ` +
			`nullable (null), or set AddField.Default to a one-off value for the existing rows`
		djangotest.AssertEqual(t, err.Error(), want)
		djangotest.AssertColumnNotExists(t, db, "test_adnd_pony", "height")
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT count(*) FROM test_adnd_pony"), int64(1))
	})
}

// Rows that hold NULL get the one-off default when a column becomes NOT NULL.
// Without one the database refuses the change and the migration is rolled
// back.
func TestAlterFieldToNotNull(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := setUpTestModel(t, db, "test_alnn", testModelOptions{})
		mustExec(t, db, "INSERT INTO test_alnn_pony (pink, weight, green) VALUES (1, 10, NULL)")
		mustExec(t, db, "INSERT INTO test_alnn_pony (pink, weight, green) VALUES (2, 20, 4)")
		notNull := greenField
		notNull.Null = false

		if _, err := tryApplyOps(db, "test_alnn", state, migrations.AlterField{Model: "Pony", Field: notNull}); err == nil {
			t.Fatal("making a column with NULL rows NOT NULL without a default succeeded")
		}
		djangotest.AssertColumnNull(t, db, "test_alnn_pony", "green")
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT count(*) FROM test_alnn_pony"), int64(2))

		operation := migrations.AlterField{Model: "Pony", Field: notNull, Default: int64(9)}
		applyOps(t, db, "test_alnn", state, operation)
		djangotest.AssertColumnNotNull(t, db, "test_alnn_pony", "green")
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT green FROM test_alnn_pony WHERE pink = 1"), int64(9))
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT green FROM test_alnn_pony WHERE pink = 2"), int64(4))

		unapplyOps(t, db, "test_alnn", state, operation)
		djangotest.AssertColumnNull(t, db, "test_alnn_pony", "green")
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT green FROM test_alnn_pony WHERE pink = 1"), int64(9))
	})
}

// collectAlter returns the statements that changing the field oldF of a model
// into newF produces for the dialect d.
func collectAlter(t *testing.T, d orm.Dialect, oldF, newF migrations.FieldState, oneOff any) []string {
	t.Helper()
	state := migrations.NewProjectState()
	state.AddModel(&migrations.ModelState{
		App: "zoo", Name: "Pony", Table: "zoo_pony", Fields: []migrations.FieldState{idField, oldF},
	})
	operation := migrations.AlterField{Model: "Pony", Field: newF, Default: oneOff}
	newState := stateForwards(t, "zoo", state, operation)
	ctx := context.Background()
	statements, err := migrations.CollectSQL(d, func(e *migrations.SchemaEditor) error {
		return operation.DatabaseForwards(ctx, "zoo", e, state, newState)
	})
	if err != nil {
		t.Fatalf("collect SQL: %v", err)
	}
	return statements
}

// PostgreSQL alters a column in place. No statement creates, copies or drops a
// table.
func TestAlterFieldPostgresAltersInPlace(t *testing.T) {
	name := migrations.FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 20, Null: true}
	with := func(change func(f *migrations.FieldState)) migrations.FieldState {
		f := name
		change(&f)
		return f
	}
	tests := []struct {
		title  string
		newF   migrations.FieldState
		oneOff any
		want   []string
	}{
		{
			title: "wider column",
			newF:  with(func(f *migrations.FieldState) { f.MaxLength = 50 }),
			want:  []string{`ALTER TABLE "zoo_pony" ALTER COLUMN "name" TYPE VARCHAR(50);`},
		},
		{
			title: "text column",
			newF:  with(func(f *migrations.FieldState) { f.Kind, f.MaxLength = orm.KindText, 0 }),
			want:  []string{`ALTER TABLE "zoo_pony" ALTER COLUMN "name" TYPE TEXT USING "name"::TEXT;`},
		},
		{
			title:  "not null with a one-off default",
			newF:   with(func(f *migrations.FieldState) { f.Null = false }),
			oneOff: "it's unknown",
			want: []string{
				`UPDATE "zoo_pony" SET "name" = 'it''s unknown' WHERE "name" IS NULL;`,
				`ALTER TABLE "zoo_pony" ALTER COLUMN "name" SET NOT NULL;`,
			},
		},
		{
			title: "not null without a default",
			newF:  with(func(f *migrations.FieldState) { f.Null = false }),
			want:  []string{`ALTER TABLE "zoo_pony" ALTER COLUMN "name" SET NOT NULL;`},
		},
		{
			title: "renamed column",
			newF:  with(func(f *migrations.FieldState) { f.Column, f.MaxLength = "title", 30 }),
			want: []string{
				`ALTER TABLE "zoo_pony" RENAME COLUMN "name" TO "title";`,
				`ALTER TABLE "zoo_pony" ALTER COLUMN "title" TYPE VARCHAR(30);`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			djangotest.AssertSequenceEqual(t, collectAlter(t, postgres.New(), name, tt.newF, tt.oneOff), tt.want)
		})
	}

	// An index and a unique constraint are added and dropped by name.
	t.Run("index and unique", func(t *testing.T) {
		indexed := with(func(f *migrations.FieldState) { f.DBIndex = true })
		unique := with(func(f *migrations.FieldState) { f.Unique = true })
		for _, statements := range [][]string{
			collectAlter(t, postgres.New(), name, indexed, nil),
			collectAlter(t, postgres.New(), indexed, name, nil),
			collectAlter(t, postgres.New(), name, unique, nil),
			collectAlter(t, postgres.New(), unique, name, nil),
		} {
			djangotest.AssertEqual(t, len(statements), 1)
			for _, statement := range statements {
				if strings.Contains(statement, " TABLE \"new__") || strings.HasPrefix(statement, "DROP TABLE") {
					t.Errorf("PostgreSQL rebuilds the table: %s", statement)
				}
			}
		}
		create := collectAlter(t, postgres.New(), name, indexed, nil)[0]
		drop := collectAlter(t, postgres.New(), indexed, name, nil)[0]
		if !strings.HasPrefix(create, `CREATE INDEX "zoo_pony_name_`) || !strings.HasSuffix(create, `ON "zoo_pony" ("name");`) {
			t.Errorf("unexpected index statement: %s", create)
		}
		indexName := strings.Fields(create)[2]
		djangotest.AssertEqual(t, drop, "DROP INDEX IF EXISTS "+indexName+";")
		djangotest.AssertEqual(t, collectAlter(t, postgres.New(), unique, name, nil)[0],
			`ALTER TABLE "zoo_pony" DROP CONSTRAINT "zoo_pony_name_key";`)
	})

	// SQLite has no ALTER COLUMN. The same change copies the rows into a new
	// table.
	t.Run("sqlite rebuild", func(t *testing.T) {
		wider := with(func(f *migrations.FieldState) { f.MaxLength = 50 })
		djangotest.AssertSequenceEqual(t, collectAlter(t, sqlite.New(), name, wider, nil), []string{
			`CREATE TABLE "new__zoo_pony" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" VARCHAR(50));`,
			`INSERT INTO "new__zoo_pony" ("id", "name") SELECT "id", "name" FROM "zoo_pony";`,
			`DROP TABLE "zoo_pony";`,
			`ALTER TABLE "new__zoo_pony" RENAME TO "zoo_pony";`,
		})
	})
}

// A SQLite rebuild keeps rows, indexes, unique constraints and the foreign
// keys in both directions, also on a connection that enforces foreign keys.
func TestSQLiteRebuildKeepsDataIndexesAndForeignKeys(t *testing.T) {
	db := djangotest.NewDB(t)
	mustExec(t, db, "PRAGMA foreign_keys = ON")
	state := setUpTestModel(t, db, "test_rebuild", testModelOptions{
		relatedModel: true, index: true, uniqueTogether: true,
	})
	mustExec(t, db, "INSERT INTO test_rebuild_pony (id, pink, weight, yellow) VALUES (1, 3, 10, 'sun')")
	mustExec(t, db, "INSERT INTO test_rebuild_pony (id, pink, weight) VALUES (2, 4, 20)")
	mustExec(t, db, "INSERT INTO test_rebuild_rider (id, pony_id) VALUES (1, 1)")
	mustExec(t, db, "INSERT INTO test_rebuild_rider (id, pony_id, friend_id) VALUES (2, 2, 1)")

	// Rebuild the table that others refer to, then the one with the foreign
	// keys.
	nullablePink := pinkField
	nullablePink.Null = true
	nullablePony := fkField("Pony", "pony_id", "test_rebuild.Pony")
	nullablePony.Null = true
	operations := []migrations.Operation{
		migrations.AlterField{Model: "Pony", Field: nullablePink},
		migrations.AlterField{Model: "Rider", Field: nullablePony},
	}
	applyOps(t, db, "test_rebuild", state, operations...)
	check := func() {
		t.Helper()
		djangotest.AssertEqual(t, queryInt(t, db, "PRAGMA foreign_keys"), int64(1))
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT count(*) FROM test_rebuild_pony"), int64(2))
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT count(*) FROM test_rebuild_pony WHERE id = 1 AND yellow = 'sun'"), int64(1))
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT pony_id FROM test_rebuild_rider WHERE friend_id = 1"), int64(2))
		djangotest.AssertIndexNameExists(t, db, "test_rebuild_pony", "pony_pink_idx")
		djangotest.AssertUniqueConstraintExists(t, db, "test_rebuild_pony", "pink", "weight")
		djangotest.AssertFKExists(t, db, "test_rebuild_rider", "pony_id", "test_rebuild_pony", "id")
		djangotest.AssertFKExists(t, db, "test_rebuild_rider", "friend_id", "test_rebuild_rider", "id")
		if err := execSQL(db, "INSERT INTO test_rebuild_rider (pony_id) VALUES (99)"); err == nil {
			t.Error("a rider of a pony that does not exist was accepted")
		}
		if err := execSQL(db, "INSERT INTO test_rebuild_pony (pink, weight) VALUES (3, 10)"); err == nil {
			t.Error("a second pony with the same pink and weight was accepted")
		}
	}
	check()
	djangotest.AssertColumnNull(t, db, "test_rebuild_pony", "pink")
	djangotest.AssertColumnNull(t, db, "test_rebuild_rider", "pony_id")

	unapplyOps(t, db, "test_rebuild", state, operations...)
	check()
	djangotest.AssertColumnNotNull(t, db, "test_rebuild_pony", "pink")
	djangotest.AssertColumnNotNull(t, db, "test_rebuild_rider", "pony_id")
}

// Foreign keys are not enforced while a SQLite migration runs, because a
// rebuild drops a table that others refer to. They are checked before the
// commit, so a migration cannot leave a dangling reference behind.
func TestSQLiteForeignKeysCheckedBeforeCommit(t *testing.T) {
	db := djangotest.NewDB(t)
	mustExec(t, db, "PRAGMA foreign_keys = ON")
	state := setUpTestModel(t, db, "test_fkcheck", testModelOptions{relatedModel: true})
	mustExec(t, db, "INSERT INTO test_fkcheck_pony (id, pink, weight) VALUES (1, 3, 10)")
	mustExec(t, db, "INSERT INTO test_fkcheck_rider (id, pony_id) VALUES (1, 1)")

	_, err := tryApplyOps(db, "test_fkcheck", state, migrations.RunSQL{SQL: []string{"DELETE FROM test_fkcheck_pony"}})
	if err == nil || !strings.Contains(err.Error(), `of table "test_fkcheck_rider" violates a foreign key to table "test_fkcheck_pony"`) {
		t.Fatalf("got error %v, want the foreign key violation", err)
	}
	djangotest.AssertEqual(t, queryInt(t, db, "SELECT count(*) FROM test_fkcheck_pony"), int64(1))
	djangotest.AssertEqual(t, queryInt(t, db, "PRAGMA foreign_keys"), int64(1))
}
