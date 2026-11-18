package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The operation tests follow Django's OperationTests. Models and fields carry
// Go names here ("Pony", "Pink"); descriptions print the lowercase model name
// and the snake_case field name, as Django's generated migrations do. Django's
// deconstruct() checks have no counterpart: the writer tests cover how an
// operation is serialized.

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_create_model
func TestCreateModel(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		pink := pinkField
		pink.Default = int64(1)
		operation := migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{idField, pink}}
		djangotest.AssertEqual(t, operation.Describe(), "Create model Pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "+ Create model Pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "pony")
		// Test the state alteration.
		projectState := migrations.NewProjectState()
		newState := stateForwards(t, "test_crmo", projectState, operation)
		pony := modelOf(t, newState, "test_crmo", "Pony")
		djangotest.AssertEqual(t, pony.Name, "Pony")
		djangotest.AssertEqual(t, len(pony.Fields), 2)
		// Test the database alteration.
		djangotest.AssertTableNotExists(t, db, "test_crmo_pony")
		databaseForwards(t, db, "test_crmo", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "test_crmo_pony")
		// And test reversal.
		databaseBackwards(t, db, "test_crmo", operation, newState, projectState)
		djangotest.AssertTableNotExists(t, db, "test_crmo_pony")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_create_model_with_duplicate_field_name
//
// Django rejects the duplicate when the operation is constructed. A Go struct
// literal cannot fail, so the state change reports it.
func TestCreateModelWithDuplicateFieldName(t *testing.T) {
	text := migrations.FieldState{Name: "Pink", Column: "pink", Kind: orm.KindText}
	operation := migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{idField, text, pinkField}}
	err := operation.StateForwards("test_crmo", migrations.NewProjectState())
	if err == nil || !strings.Contains(err.Error(), "found duplicate value Pink in CreateModel fields argument") {
		t.Errorf("got error %v, want the duplicate field error", err)
	}
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_create_model_with_unique_after
func TestCreateModelWithUniqueAfter(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		pink := pinkField
		pink.Default = int64(1)
		number := migrations.FieldState{Name: "Number", Column: "number", Kind: orm.KindInt, Default: int64(1)}
		djangotest.AssertTableNotExists(t, db, "test_crmoua_pony")
		djangotest.AssertTableNotExists(t, db, "test_crmoua_rider")
		applyOps(t, db, "test_crmoua", migrations.NewProjectState(),
			migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{idField, pink}},
			migrations.CreateModel{Name: "Rider", Fields: []migrations.FieldState{
				idField, number, fkField("Pony", "pony_id", "test_crmoua.Pony"),
			}},
			migrations.AlterUniqueTogether{Name: "Rider", UniqueTogether: [][]string{{"Number", "Pony"}}},
		)
		djangotest.AssertTableExists(t, db, "test_crmoua_pony")
		djangotest.AssertTableExists(t, db, "test_crmoua_rider")
		djangotest.AssertUniqueConstraintExists(t, db, "test_crmoua_rider", "number", "pony_id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_delete_model
func TestDeleteModel(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_dlmo", testModelOptions{})
		// Test the state alteration.
		operation := migrations.DeleteModel{Name: "Pony"}
		djangotest.AssertEqual(t, operation.Describe(), "Delete model Pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "- Delete model Pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "delete_pony")
		newState := stateForwards(t, "test_dlmo", projectState, operation)
		if _, ok := newState.Model("test_dlmo", "Pony"); ok {
			t.Error("the deleted model is still in the state")
		}
		// Test the database alteration.
		djangotest.AssertTableExists(t, db, "test_dlmo_pony")
		databaseForwards(t, db, "test_dlmo", operation, projectState, newState)
		djangotest.AssertTableNotExists(t, db, "test_dlmo_pony")
		// And test reversal.
		databaseBackwards(t, db, "test_dlmo", operation, newState, projectState)
		djangotest.AssertTableExists(t, db, "test_dlmo_pony")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model
func TestRenameModel(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rnmo", testModelOptions{relatedModel: true})
		// Test the state alteration.
		operation := migrations.RenameModel{OldName: "Pony", NewName: "Horse"}
		djangotest.AssertEqual(t, operation.Describe(), "Rename model Pony to Horse")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "~ Rename model Pony to Horse")
		djangotest.AssertEqual(t, operation.NameFragment(), "rename_pony_horse")
		// Test initial state and database.
		modelOf(t, projectState, "test_rnmo", "Pony")
		if _, ok := projectState.Model("test_rnmo", "Horse"); ok {
			t.Error("Horse is in the state before the rename")
		}
		djangotest.AssertTableExists(t, db, "test_rnmo_pony")
		djangotest.AssertTableNotExists(t, db, "test_rnmo_horse")
		djangotest.AssertFKExists(t, db, "test_rnmo_rider", "pony_id", "test_rnmo_pony", "id")
		djangotest.AssertFKNotExists(t, db, "test_rnmo_rider", "pony_id", "test_rnmo_horse", "id")
		// Migrate forwards.
		newState := applyOps(t, db, "test_rnmo", projectState, operation)
		// Test new state and database.
		if _, ok := newState.Model("test_rnmo", "Pony"); ok {
			t.Error("Pony is still in the state after the rename")
		}
		modelOf(t, newState, "test_rnmo", "Horse")
		// RenameModel also repoints all incoming foreign keys.
		djangotest.AssertEqual(t, fieldOf(t, newState, "test_rnmo", "Rider", "Pony").RelTo, "test_rnmo.Horse")
		djangotest.AssertTableNotExists(t, db, "test_rnmo_pony")
		djangotest.AssertTableExists(t, db, "test_rnmo_horse")
		djangotest.AssertFKNotExists(t, db, "test_rnmo_rider", "pony_id", "test_rnmo_pony", "id")
		djangotest.AssertFKExists(t, db, "test_rnmo_rider", "pony_id", "test_rnmo_horse", "id")
		// Migrate backwards.
		unapplyOps(t, db, "test_rnmo", projectState, operation)
		// Test original state and database.
		djangotest.AssertEqual(t, fieldOf(t, projectState, "test_rnmo", "Rider", "Pony").RelTo, "test_rnmo.Pony")
		djangotest.AssertTableExists(t, db, "test_rnmo_pony")
		djangotest.AssertTableNotExists(t, db, "test_rnmo_horse")
		djangotest.AssertFKExists(t, db, "test_rnmo_rider", "pony_id", "test_rnmo_pony", "id")
		djangotest.AssertFKNotExists(t, db, "test_rnmo_rider", "pony_id", "test_rnmo_horse", "id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model_state_forwards
//
// The Go state has no rendered apps to cache; the rename in both directions
// is what remains of the test.
func TestRenameModelStateForwards(t *testing.T) {
	state := migrations.NewProjectState()
	state.AddModel(&migrations.ModelState{App: "migrations", Name: "Foo", Table: "migrations_foo"})
	operation := migrations.RenameModel{OldName: "Foo", NewName: "Bar"}
	if err := operation.StateForwards("migrations", state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Model("migrations", "Foo"); ok {
		t.Error("Foo is still in the state")
	}
	djangotest.AssertEqual(t, modelOf(t, state, "migrations", "Bar").Table, "migrations_bar")
	operation = migrations.RenameModel{OldName: "Bar", NewName: "Foo"}
	if err := operation.StateForwards("migrations", state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Model("migrations", "Bar"); ok {
		t.Error("Bar is still in the state")
	}
	modelOf(t, state, "migrations", "Foo")
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_model_with_self_referential_fk
func TestRenameModelWithSelfReferentialFK(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rmwsrf", testModelOptions{relatedModel: true})
		// Test the state alteration.
		operation := migrations.RenameModel{OldName: "Rider", NewName: "HorseRider"}
		djangotest.AssertEqual(t, operation.Describe(), "Rename model Rider to HorseRider")
		newState := stateForwards(t, "test_rmwsrf", projectState, operation)
		if _, ok := newState.Model("test_rmwsrf", "Rider"); ok {
			t.Error("Rider is still in the state")
		}
		// RenameModel also repoints the foreign key of the model to itself.
		djangotest.AssertEqual(t,
			fieldOf(t, newState, "test_rmwsrf", "HorseRider", "Friend").RelTo, "test_rmwsrf.HorseRider")
		// Test the database alteration.
		djangotest.AssertTableExists(t, db, "test_rmwsrf_rider")
		djangotest.AssertTableNotExists(t, db, "test_rmwsrf_horserider")
		djangotest.AssertFKExists(t, db, "test_rmwsrf_rider", "friend_id", "test_rmwsrf_rider", "id")
		djangotest.AssertFKNotExists(t, db, "test_rmwsrf_rider", "friend_id", "test_rmwsrf_horserider", "id")
		databaseForwards(t, db, "test_rmwsrf", operation, projectState, newState)
		djangotest.AssertTableNotExists(t, db, "test_rmwsrf_rider")
		djangotest.AssertTableExists(t, db, "test_rmwsrf_horserider")
		djangotest.AssertFKNotExists(t, db, "test_rmwsrf_horserider", "friend_id", "test_rmwsrf_rider", "id")
		djangotest.AssertFKExists(t, db, "test_rmwsrf_horserider", "friend_id", "test_rmwsrf_horserider", "id")
		// And test reversal.
		databaseBackwards(t, db, "test_rmwsrf", operation, newState, projectState)
		djangotest.AssertTableExists(t, db, "test_rmwsrf_rider")
		djangotest.AssertTableNotExists(t, db, "test_rmwsrf_horserider")
		djangotest.AssertFKExists(t, db, "test_rmwsrf_rider", "friend_id", "test_rmwsrf_rider", "id")
		djangotest.AssertFKNotExists(t, db, "test_rmwsrf_rider", "friend_id", "test_rmwsrf_horserider", "id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_field
func TestAddField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		// Test the state alteration.
		height := migrations.FieldState{Name: "Height", Column: "height", Kind: orm.KindInt, Null: true, Default: int64(5)}
		operation := migrations.AddField{Model: "Pony", Field: height}
		djangotest.AssertEqual(t, operation.Describe(), "Add field height to pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "+ Add field height to pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "pony_height")
		projectState := setUpTestModel(t, db, "test_adfl", testModelOptions{})
		newState := stateForwards(t, "test_adfl", projectState, operation)
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_adfl", "Pony").Fields), 6)
		djangotest.AssertEqual(t, fieldOf(t, newState, "test_adfl", "Pony", "Height").Default, any(int64(5)))
		// Test the database alteration.
		djangotest.AssertColumnNotExists(t, db, "test_adfl_pony", "height")
		databaseForwards(t, db, "test_adfl", operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, "test_adfl_pony", "height")
		// And test reversal.
		databaseBackwards(t, db, "test_adfl", operation, newState, projectState)
		djangotest.AssertColumnNotExists(t, db, "test_adfl_pony", "height")
	})
}

// addTextColumns is the shared body of the char and text field tests: add
// four columns with string defaults to a table with one row and check that
// the row got the defaults unchanged.
func addTextColumns(t *testing.T, db *orm.DB, app string, kind orm.Kind, maxLength int) {
	t.Helper()
	projectState := setUpTestModel(t, db, app, testModelOptions{})
	table := app + "_pony"
	mustExec(t, db, "INSERT INTO "+table+" (pink, weight) VALUES (3, 42)")
	field := func(name, column, def string) migrations.FieldState {
		return migrations.FieldState{Name: name, Column: column, Kind: kind, MaxLength: maxLength, Default: def}
	}
	applyOps(t, db, app, projectState,
		migrations.AddField{Model: "Pony", Field: field("Text", "text", "some text")},
		migrations.AddField{Model: "Pony", Field: field("Empty", "empty", "")},
		// If not properly quoted digits would be interpreted as an int.
		migrations.AddField{Model: "Pony", Field: field("Digits", "digits", "42")},
		// Manual quoting is fragile and could trip on quotes.
		migrations.AddField{Model: "Pony", Field: field("Quotes", "quotes", `"'"`)},
	)
	var text, empty, digits, quotes string
	row := db.SQL().QueryRowContext(context.Background(),
		`SELECT "text", "empty", "digits", "quotes" FROM `+table+` WHERE weight = 42`)
	if err := row.Scan(&text, &empty, &digits, &quotes); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, text, "some text")
	djangotest.AssertEqual(t, empty, "")
	djangotest.AssertEqual(t, digits, "42")
	djangotest.AssertEqual(t, quotes, `"'"`)
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_charfield
func TestAddCharfield(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		addTextColumns(t, db, "test_adchfl", orm.KindChar, 10)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_textfield
func TestAddTextfield(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		addTextColumns(t, db, "test_adtxtfl", orm.KindText, 0)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_column_name_quoting
func TestColumnNameQuoting(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_regr22168", testModelOptions{})
		operation := migrations.AddField{Model: "Pony", Field: migrations.FieldState{
			Name: "Order", Column: "order", Kind: orm.KindInt, Default: int64(0),
		}}
		newState := stateForwards(t, "test_regr22168", projectState, operation)
		databaseForwards(t, db, "test_regr22168", operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, "test_regr22168_pony", "order")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_field_preserve_default
//
// Django's preserve_default=False is the one-off AddField.Default here: it
// fills the existing rows and stays out of the state.
func TestAddFieldPreserveDefault(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_adflpd", testModelOptions{})
		// Test the state alteration.
		operation := migrations.AddField{
			Model:   "Pony",
			Field:   migrations.FieldState{Name: "Height", Column: "height", Kind: orm.KindInt, Null: true},
			Default: int64(4),
		}
		newState := stateForwards(t, "test_adflpd", projectState, operation)
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_adflpd", "Pony").Fields), 6)
		djangotest.AssertEqual(t, fieldOf(t, newState, "test_adflpd", "Pony", "Height").Default, nil)
		// Test the database alteration.
		mustExec(t, db, "INSERT INTO test_adflpd_pony (pink, weight) VALUES (3, 4)")
		djangotest.AssertColumnNotExists(t, db, "test_adflpd_pony", "height")
		databaseForwards(t, db, "test_adflpd", operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, "test_adflpd_pony", "height")
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT height FROM test_adflpd_pony"), 4)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_field
func TestRemoveField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rmfl", testModelOptions{})
		// Test the state alteration.
		operation := migrations.RemoveField{Model: "Pony", Field: "Pink"}
		djangotest.AssertEqual(t, operation.Describe(), "Remove field pink from pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "- Remove field pink from pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "remove_pony_pink")
		newState := stateForwards(t, "test_rmfl", projectState, operation)
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_rmfl", "Pony").Fields), 4)
		// Test the database alteration.
		djangotest.AssertColumnExists(t, db, "test_rmfl_pony", "pink")
		databaseForwards(t, db, "test_rmfl", operation, projectState, newState)
		djangotest.AssertColumnNotExists(t, db, "test_rmfl_pony", "pink")
		// And test reversal.
		databaseBackwards(t, db, "test_rmfl", operation, newState, projectState)
		djangotest.AssertColumnExists(t, db, "test_rmfl_pony", "pink")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_fk
func TestRemoveFK(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rfk", testModelOptions{relatedModel: true})
		djangotest.AssertColumnExists(t, db, "test_rfk_rider", "pony_id")
		operation := migrations.RemoveField{Model: "Rider", Field: "Pony"}
		newState := stateForwards(t, "test_rfk", projectState, operation)
		databaseForwards(t, db, "test_rfk", operation, projectState, newState)
		djangotest.AssertColumnNotExists(t, db, "test_rfk_rider", "pony_id")
		databaseBackwards(t, db, "test_rfk", operation, newState, projectState)
		djangotest.AssertColumnExists(t, db, "test_rfk_rider", "pony_id")
		djangotest.AssertFKExists(t, db, "test_rfk_rider", "pony_id", "test_rfk_pony", "id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_model_table
func TestAlterModelTable(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_almota", testModelOptions{})
		// Test the state alteration.
		operation := migrations.AlterModelTable{Name: "Pony", Table: "test_almota_pony_2"}
		djangotest.AssertEqual(t, operation.Describe(), "Rename table for pony to test_almota_pony_2")
		djangotest.AssertEqual(t,
			migrations.FormattedDescription(operation), "~ Rename table for pony to test_almota_pony_2")
		djangotest.AssertEqual(t, operation.NameFragment(), "alter_pony_table")
		newState := stateForwards(t, "test_almota", projectState, operation)
		djangotest.AssertEqual(t, modelOf(t, newState, "test_almota", "Pony").Table, "test_almota_pony_2")
		// Test the database alteration.
		djangotest.AssertTableExists(t, db, "test_almota_pony")
		djangotest.AssertTableNotExists(t, db, "test_almota_pony_2")
		databaseForwards(t, db, "test_almota", operation, projectState, newState)
		djangotest.AssertTableNotExists(t, db, "test_almota_pony")
		djangotest.AssertTableExists(t, db, "test_almota_pony_2")
		// And test reversal.
		databaseBackwards(t, db, "test_almota", operation, newState, projectState)
		djangotest.AssertTableExists(t, db, "test_almota_pony")
		djangotest.AssertTableNotExists(t, db, "test_almota_pony_2")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_model_table_none
//
// Django passes None for the default table; the Go operation takes "".
func TestAlterModelTableNone(t *testing.T) {
	operation := migrations.AlterModelTable{Name: "Pony"}
	djangotest.AssertEqual(t, operation.Describe(), "Rename table for pony to (default)")
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_model_table_noop
func TestAlterModelTableNoop(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_almota", testModelOptions{})
		// Test the state alteration.
		operation := migrations.AlterModelTable{Name: "Pony", Table: "test_almota_pony"}
		newState := stateForwards(t, "test_almota", projectState, operation)
		djangotest.AssertEqual(t, modelOf(t, newState, "test_almota", "Pony").Table, "test_almota_pony")
		// Test the database alteration.
		djangotest.AssertTableExists(t, db, "test_almota_pony")
		databaseForwards(t, db, "test_almota", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "test_almota_pony")
		// And test reversal.
		databaseBackwards(t, db, "test_almota", operation, newState, projectState)
		djangotest.AssertTableExists(t, db, "test_almota_pony")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_field
func TestAlterField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_alfl", testModelOptions{})
		// Test the state alteration.
		operation := migrations.AlterField{Model: "Pony", Field: migrations.FieldState{
			Name: "Pink", Column: "pink", Kind: orm.KindInt, Null: true,
		}}
		djangotest.AssertEqual(t, operation.Describe(), "Alter field pink on pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "~ Alter field pink on pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "alter_pony_pink")
		newState := stateForwards(t, "test_alfl", projectState, operation)
		djangotest.AssertEqual(t, fieldOf(t, projectState, "test_alfl", "Pony", "Pink").Null, false)
		djangotest.AssertEqual(t, fieldOf(t, newState, "test_alfl", "Pony", "Pink").Null, true)
		// Test the database alteration.
		djangotest.AssertColumnNotNull(t, db, "test_alfl_pony", "pink")
		databaseForwards(t, db, "test_alfl", operation, projectState, newState)
		djangotest.AssertColumnNull(t, db, "test_alfl_pony", "pink")
		// And test reversal.
		databaseBackwards(t, db, "test_alfl", operation, newState, projectState)
		djangotest.AssertColumnNotNull(t, db, "test_alfl_pony", "pink")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_field_pk
//
// SQLite rebuilds the table and so can change the primary key. PostgreSQL
// would have to drop the identity of the column; the ORM gives every model an
// auto primary key, so the alteration is refused there with a clear error.
func TestAlterFieldPK(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_alflpk", testModelOptions{})
		// Test the state alteration.
		operation := migrations.AlterField{Model: "Pony", Field: migrations.FieldState{
			Name: "ID", Column: "id", Kind: orm.KindInt, PrimaryKey: true,
		}}
		newState := stateForwards(t, "test_alflpk", projectState, operation)
		djangotest.AssertEqual(t, fieldOf(t, projectState, "test_alflpk", "Pony", "ID").Kind, orm.KindAuto)
		djangotest.AssertEqual(t, fieldOf(t, newState, "test_alflpk", "Pony", "ID").Kind, orm.KindInt)
		if db.Dialect().Name() == "postgres" {
			_, err := tryApplyOps(db, "test_alflpk", projectState, operation)
			if err == nil || !strings.Contains(err.Error(), "changing the primary key of a model is not supported") {
				t.Errorf("got error %v, want the primary key error", err)
			}
			return
		}
		// Test the database alteration.
		databaseForwards(t, db, "test_alflpk", operation, projectState, newState)
		// And test reversal.
		databaseBackwards(t, db, "test_alflpk", operation, newState, projectState)
	})
}
