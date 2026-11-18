package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// collectForwards collects the SQL that the forwards step of op produces
// without executing it.
func collectForwards(t *testing.T, db *orm.DB, app string, op migrations.Operation, from, to *migrations.ProjectState) []string {
	t.Helper()
	statements, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
		return op.DatabaseForwards(context.Background(), app, e, from, to)
	})
	if err != nil {
		t.Fatalf("collect forwards: %v", err)
	}
	return statements
}

// collectBackwards collects the SQL that the backwards step of op produces
// without executing it.
func collectBackwards(t *testing.T, db *orm.DB, app string, op migrations.Operation, from, to *migrations.ProjectState) []string {
	t.Helper()
	statements, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
		return op.DatabaseBackwards(context.Background(), app, e, from, to)
	})
	if err != nil {
		t.Fatalf("collect backwards: %v", err)
	}
	return statements
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model_with_self_referential_fk_collect_sql
//
// Both dialects rename the table and let the database repoint the
// self-referential key, so the collected SQL names the new table but, unlike
// Django's, not the friend_id column.
func TestRenameModelWithSelfReferentialFKCollectSQL(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rmwsrfcs", testModelOptions{relatedModel: true})
		operation := migrations.RenameModel{OldName: "Rider", NewName: "HorseRider"}
		newState := stateForwards(t, "test_rmwsrfcs", projectState, operation)
		quote := db.Dialect().Quote
		// Forwards: only the old table exists, so the renamed table cannot be
		// introspected.
		collected := strings.Join(collectForwards(t, db, "test_rmwsrfcs", operation, projectState, newState), "\n")
		if !strings.Contains(collected, quote("test_rmwsrfcs_horserider")) {
			t.Errorf("forwards SQL lacks the new table: %s", collected)
		}
		// Backwards: apply the rename for real, then collect the reverse SQL.
		databaseForwards(t, db, "test_rmwsrfcs", operation, projectState, newState)
		collected = strings.Join(collectBackwards(t, db, "test_rmwsrfcs", operation, newState, projectState), "\n")
		if !strings.Contains(collected, quote("test_rmwsrfcs_rider")) {
			t.Errorf("backwards SQL lacks the old table: %s", collected)
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model_no_relations_with_db_table_noop
//
// Django counts queries; the Go test collects the SQL and expects none.
func TestRenameModelNoRelationsWithDBTableNoop(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		appLabel := "test_rmwdbtnoop"
		projectState := setUpTestModel(t, db, appLabel, testModelOptions{table: "my_pony"})
		operation := migrations.RenameModel{OldName: "Pony", NewName: "LittleHorse"}
		newState := stateForwards(t, appLabel, projectState, operation)
		djangotest.AssertEqual(t, len(collectForwards(t, db, appLabel, operation, projectState, newState)), 0)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model_with_db_table_and_fk_noop
//
// Django counts queries; the Go test collects the SQL and expects none.
func TestRenameModelWithDBTableAndFKNoop(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		appLabel := "test_rmwdbtfk"
		projectState := setUpTestModel(t, db, appLabel, testModelOptions{table: "my_pony", relatedModel: true})
		operation := migrations.RenameModel{OldName: "Pony", NewName: "LittleHorse"}
		newState := stateForwards(t, appLabel, projectState, operation)
		djangotest.AssertEqual(t, len(collectForwards(t, db, appLabel, operation, projectState, newState)), 0)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_field_add_db_column_noop
//
// A Go FieldState always carries its column, so there is no state with an
// unset db_column: the column is "weight" before and after. Django's
// FloatField is an integer here. The operation must still be a no-op that
// leaves the column in place. Django counts queries; the Go test collects the
// SQL and expects none.
func TestAlterFieldAddDBColumnNoop(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		appLabel := "test_afadbn"
		projectState := setUpTestModel(t, db, appLabel, testModelOptions{relatedModel: true})
		ponyTable := appLabel + "_pony"
		operation := migrations.AlterField{Model: "Pony", Field: weightField}
		newState := stateForwards(t, appLabel, projectState, operation)
		djangotest.AssertEqual(t, fieldOf(t, projectState, appLabel, "Pony", "Weight").Column, "weight")
		djangotest.AssertEqual(t, fieldOf(t, newState, appLabel, "Pony", "Weight").Column, "weight")
		djangotest.AssertColumnExists(t, db, ponyTable, "weight")
		djangotest.AssertEqual(t, len(collectForwards(t, db, appLabel, operation, projectState, newState)), 0)
		databaseForwards(t, db, appLabel, operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, ponyTable, "weight")
		djangotest.AssertEqual(t, len(collectBackwards(t, db, appLabel, operation, newState, projectState)), 0)
		databaseBackwards(t, db, appLabel, operation, newState, projectState)
		djangotest.AssertColumnExists(t, db, ponyTable, "weight")
	})
}

// intPK returns a non-auto integer primary key field with the given default.
// Django's char primary keys cannot be foreign key targets here: the foreign
// key column of the Go state is an integer. PostgreSQL cannot change the
// primary key kind either, so the tests alter the default of integer keys.
func intPK(name, column string, def any) migrations.FieldState {
	return migrations.FieldState{Name: name, Column: column, Kind: orm.KindInt, PrimaryKey: true, Default: def}
}

// setUpFKChain creates Rider, Pony (FK to Rider) and PonyRider (FK to Pony),
// the models of the "reloads state on fk target changes" tests.
func setUpFKChain(t *testing.T, db *orm.DB, app string) *migrations.ProjectState {
	t.Helper()
	return applyOps(t, db, app, migrations.NewProjectState(),
		migrations.CreateModel{Name: "Rider", Fields: []migrations.FieldState{intPK("ID", "id", nil)}},
		migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{
			intPK("ID", "id", nil), fkField("Rider", "rider_id", app+".Rider"),
		}},
		migrations.CreateModel{Name: "PonyRider", Fields: []migrations.FieldState{
			idField, fkField("Pony", "pony_id", app+".Pony"),
		}},
	)
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_field_reloads_state_on_fk_target_changes
//
// Django's regression is about MySQL and uses char primary keys (max_length
// 100 changed to 99); the Go test uses integer keys and alters their default.
// It checks that altering the target keys of a chain of foreign keys succeeds
// and that the foreign keys survive on both dialects. The foreign key columns
// of the Go state carry no type of their own, so there is nothing to reload.
func TestAlterFieldReloadsStateOnFKTargetChanges(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		appLabel := "alter_alter_field_reloads_state_on_fk_target_changes"
		projectState := setUpFKChain(t, db, appLabel)
		projectState = applyOps(t, db, appLabel, projectState,
			migrations.AlterField{Model: "Rider", Field: intPK("ID", "id", int64(99))},
			migrations.AlterField{Model: "Pony", Field: intPK("ID", "id", int64(99))},
		)
		djangotest.AssertEqual(t, fieldOf(t, projectState, appLabel, "Rider", "ID").Default, any(int64(99)))
		djangotest.AssertEqual(t, fieldOf(t, projectState, appLabel, "Pony", "ID").Default, any(int64(99)))
		djangotest.AssertFKExists(t, db, appLabel+"_pony", "rider_id", appLabel+"_rider", "id")
		djangotest.AssertFKExists(t, db, appLabel+"_ponyrider", "pony_id", appLabel+"_pony", "id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_field_reloads_state_on_fk_target_changes
//
// Django's regression is about MySQL and uses char primary keys (max_length
// 100 changed to 99); the Go test uses integer keys and alters their default.
// It checks that renaming the target key of a foreign key and then altering
// the next key in the chain succeeds.
func TestRenameFieldReloadsStateOnFKTargetChanges(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		appLabel := "alter_rename_field_reloads_state_on_fk_target_changes"
		projectState := setUpFKChain(t, db, appLabel)
		projectState = applyOps(t, db, appLabel, projectState,
			migrations.RenameField{Model: "Rider", OldName: "ID", NewName: "ID2"},
			migrations.AlterField{Model: "Pony", Field: intPK("ID", "id", int64(99))},
		)
		rider := modelOf(t, projectState, appLabel, "Rider")
		if _, ok := rider.FieldByName("ID2"); !ok {
			t.Error("the state has no field ID2")
		}
		djangotest.AssertEqual(t, fieldOf(t, projectState, appLabel, "Pony", "ID").Default, any(int64(99)))
		djangotest.AssertFKExists(t, db, appLabel+"_ponyrider", "pony_id", appLabel+"_pony", "id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_field_case
//
// The renamed field is "FiElD". The test reads the column that the state
// derives for it and expects that column in the table after the rename. The
// reversal must restore "field".
func TestRenameFieldCase(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := applyOps(t, db, "test_rfmx", migrations.NewProjectState(),
			migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{
				idField, {Name: "Field", Column: "field", Kind: orm.KindInt},
			}},
		)
		operation := migrations.RenameField{Model: "Pony", OldName: "Field", NewName: "FiElD"}
		newState := stateForwards(t, "test_rfmx", projectState, operation)
		if _, ok := modelOf(t, newState, "test_rfmx", "Pony").FieldByName("FiElD"); !ok {
			t.Error("the state has no field FiElD")
		}
		column := fieldOf(t, newState, "test_rfmx", "Pony", "FiElD").Column
		djangotest.AssertColumnExists(t, db, "test_rfmx_pony", "field")
		databaseForwards(t, db, "test_rfmx", operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, "test_rfmx_pony", column)
		databaseBackwards(t, db, "test_rfmx", operation, newState, projectState)
		djangotest.AssertColumnExists(t, db, "test_rfmx_pony", "field")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_referenced_field_state_forward
//
// The Go state has no to_field and no from_fields, so only the part that
// renames the referenced field is ported: the foreign key to the model keeps
// pointing at it and the field is renamed.
func TestRenameReferencedFieldStateForward(t *testing.T) {
	state := migrations.NewProjectState()
	state.AddModel(&migrations.ModelState{App: "app", Name: "Model", Table: "app_model", Fields: []migrations.FieldState{
		idField,
		{Name: "Field", Column: "field", Kind: orm.KindInt, Unique: true},
	}})
	state.AddModel(&migrations.ModelState{App: "app", Name: "OtherModel", Table: "app_othermodel", Fields: []migrations.FieldState{
		idField, fkField("FK", "fk_id", "app.Model"),
	}})
	operation := migrations.RenameField{Model: "Model", OldName: "Field", NewName: "Renamed"}
	newState := stateForwards(t, "app", state, operation)
	if _, ok := modelOf(t, newState, "app", "Model").FieldByName("Renamed"); !ok {
		t.Error("the state has no field Renamed")
	}
	djangotest.AssertEqual(t, fieldOf(t, newState, "app", "OtherModel", "FK").RelTo, "app.Model")
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_unique_together_on_pk_field
//
// Django checks the primary key by its PostgreSQL constraint name. SQLite has
// no named primary key constraint, so the Go test inserts a duplicate key
// instead; the unique-together constraint is checked by Django's name.
func TestRemoveUniqueTogetherOnPKField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		appLabel := "test_rutopkf"
		projectState := applyOps(t, db, appLabel, migrations.NewProjectState(),
			migrations.CreateModel{
				Name: "Pony", Fields: []migrations.FieldState{idField}, UniqueTogether: [][]string{{"ID"}},
			},
		)
		tableName := appLabel + "_pony"
		uniqueTogetherConstraintName := tableName + "_id_fb61f881_uniq"
		djangotest.AssertUniqueConstraintExists(t, db, tableName, "id")
		djangotest.AssertIndexNameExists(t, db, tableName, uniqueTogetherConstraintName)
		operation := migrations.AlterUniqueTogether{Name: "Pony"}
		newState := stateForwards(t, appLabel, projectState, operation)
		databaseForwards(t, db, appLabel, operation, projectState, newState)
		djangotest.AssertIndexNameNotExists(t, db, tableName, uniqueTogetherConstraintName)
		// The primary key is still unique. SQLite does not introspect the
		// primary key of a rowid table as a unique constraint, so insert.
		mustExec(t, db, "INSERT INTO "+tableName+" (id) VALUES (1)")
		if err := execSQL(db, "INSERT INTO "+tableName+" (id) VALUES (1)"); err == nil {
			t.Error("the duplicate primary key was accepted")
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_unique_together_on_unique_field
//
// SQLite names the constraint of a unique column itself
// (sqlite_autoindex_...), so Django's name of it is checked on PostgreSQL
// only; on both dialects a duplicate insert must still fail.
func TestRemoveUniqueTogetherOnUniqueField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		appLabel := "test_rutouf"
		projectState := applyOps(t, db, appLabel, migrations.NewProjectState(),
			migrations.CreateModel{
				Name: "Pony",
				Fields: []migrations.FieldState{
					idField, {Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 30, Unique: true},
				},
				UniqueTogether: [][]string{{"Name"}},
			},
		)
		tableName := appLabel + "_pony"
		uniqueConstraintName := tableName + "_name_key"
		uniqueTogetherConstraintName := tableName + "_name_694f3b9f_uniq"
		postgres := db.Dialect().Name() == "postgres"
		if postgres {
			djangotest.AssertIndexNameExists(t, db, tableName, uniqueConstraintName)
		}
		djangotest.AssertIndexNameExists(t, db, tableName, uniqueTogetherConstraintName)
		operation := migrations.AlterUniqueTogether{Name: "Pony"}
		newState := stateForwards(t, appLabel, projectState, operation)
		databaseForwards(t, db, appLabel, operation, projectState, newState)
		if postgres {
			djangotest.AssertIndexNameExists(t, db, tableName, uniqueConstraintName)
		}
		djangotest.AssertIndexNameNotExists(t, db, tableName, uniqueTogetherConstraintName)
		djangotest.AssertUniqueConstraintExists(t, db, tableName, "name")
		mustExec(t, db, "INSERT INTO "+tableName+" (name) VALUES ('a')")
		if err := execSQL(db, "INSERT INTO "+tableName+" (name) VALUES ('a')"); err == nil {
			t.Error("the duplicate name was accepted")
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_python_related_assignment
//
// Django's function creates rows through the historical models. RunGo gets a
// database handle and the state, so the function inserts with raw SQL into the
// tables of the two apps.
func TestRunPythonRelatedAssignment(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		innerMethod := func(ctx context.Context, tx *orm.DB, state *migrations.ProjectState) error {
			for _, field := range []string{"Name", "Hometown"} {
				if _, ok := modelOf(t, state, "test_authors", "Author").FieldByName(field); !ok {
					t.Errorf("the state has no field %s on Author", field)
				}
			}
			statements := []string{
				"INSERT INTO test_authors_author (name, hometown) VALUES ('Hemingway', '')",
				"INSERT INTO test_books_book (title, author_id) " +
					"SELECT 'Old Man and The Sea', id FROM test_authors_author WHERE name = 'Hemingway'",
			}
			for _, s := range statements {
				if _, err := tx.SQL().ExecContext(ctx, s); err != nil {
					return err
				}
			}
			return nil
		}
		createAuthor := migrations.CreateModel{Name: "Author", Fields: []migrations.FieldState{
			idField, {Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
		}}
		createBook := migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{
			idField,
			{Name: "Title", Column: "title", Kind: orm.KindChar, MaxLength: 100},
			fkField("Author", "author_id", "test_authors.Author"),
		}}
		addHometown := migrations.AddField{Model: "Author", Field: migrations.FieldState{
			Name: "Hometown", Column: "hometown", Kind: orm.KindChar, MaxLength: 100, Blank: true,
		}}
		createOldMan := migrations.RunGo{Forward: innerMethod, Reverse: innerMethod}

		state := applyOps(t, db, "test_authors", migrations.NewProjectState(), createAuthor)
		state = applyOps(t, db, "test_books", state, createBook)
		state = applyOps(t, db, "test_authors", state, addHometown)
		applyOps(t, db, "test_books", state, createOldMan)
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_authors_author"), 1)
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_books_book"), 1)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_separate_database_and_state
//
// Go has no SeparateDatabaseAndState. RunSQL.StateOperations gives the same
// split between a database change and a state change, so the test runs the
// operation against RunSQL with StateOperations.
func TestSeparateDatabaseAndState(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_separatedatabaseandstate", testModelOptions{})
		operation := migrations.RunSQL{
			SQL:        []string{"CREATE TABLE i_love_ponies (id int, special_thing int);"},
			ReverseSQL: []string{"DROP TABLE i_love_ponies;"},
			StateOperations: []migrations.Operation{
				migrations.CreateModel{Name: "SomethingElse", Fields: []migrations.FieldState{idField}},
			},
		}
		// Test the state alteration.
		newState := stateForwards(t, "test_separatedatabaseandstate", projectState, operation)
		djangotest.AssertEqual(t,
			len(modelOf(t, newState, "test_separatedatabaseandstate", "SomethingElse").Fields), 1)
		// Make sure there's no table, neither for the SQL nor for the model.
		djangotest.AssertTableNotExists(t, db, "i_love_ponies")
		// Test the database alteration.
		databaseForwards(t, db, "test_separatedatabaseandstate", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "i_love_ponies")
		djangotest.AssertTableNotExists(t, db, "test_separatedatabaseandstate_somethingelse")
		// And test reversal.
		djangotest.AssertEqual(t, operation.Reversible(), true)
		databaseBackwards(t, db, "test_separatedatabaseandstate", operation, newState, projectState)
		djangotest.AssertTableNotExists(t, db, "i_love_ponies")
	})
}

// Ported from Django: tests/migrations/test_operations.py::FieldOperationTests.test_references_model
//
// Django compares model names case-insensitively. The Go optimizer compares
// them exactly, so the test uses the exact names. The operation is an AddField,
// Django's FieldOperation counterpart.
func TestReferencesModel(t *testing.T) {
	operation := migrations.AddField{Model: "Model", Field: fkField("Field", "field_id", "migrations.Other")}
	// Model name match.
	djangotest.AssertEqual(t, migrations.ReferencesModel(operation, "Model", "migrations"), true)
	// Referenced field.
	djangotest.AssertEqual(t, migrations.ReferencesModel(operation, "Other", "migrations"), true)
	// Doesn't reference.
	djangotest.AssertEqual(t, migrations.ReferencesModel(operation, "Whatever", "migrations"), false)
}

// Ported from Django: tests/migrations/test_operations.py::FieldOperationTests.test_references_field_by_name
//
// Django's BooleanField is an orm.KindBool field here, and names are compared
// exactly.
func TestReferencesFieldByName(t *testing.T) {
	operation := migrations.AddField{Model: "Model", Field: migrations.FieldState{
		Name: "Field", Column: "field", Kind: orm.KindBool, Default: false,
	}}
	djangotest.AssertEqual(t, migrations.ReferencesField(operation, "Model", "Field", "migrations"), true)
}

// Ported from Django: tests/migrations/test_operations.py::FieldOperationTests.test_references_field_by_remote_field_model
func TestReferencesFieldByRemoteFieldModel(t *testing.T) {
	operation := migrations.AddField{Model: "Model", Field: fkField("Field", "field_id", "migrations.Other")}
	djangotest.AssertEqual(t, migrations.ReferencesField(operation, "Other", "whatever", "migrations"), true)
	djangotest.AssertEqual(t, migrations.ReferencesField(operation, "Missing", "whatever", "migrations"), false)
}
