package migrations_test

import (
	"context"
	"errors"
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

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_field
func TestRenameField(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rnfl", testModelOptions{})
		operation := migrations.RenameField{Model: "Pony", OldName: "Pink", NewName: "Blue"}
		djangotest.AssertEqual(t, operation.Describe(), "Rename field pink on pony to blue")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "~ Rename field pink on pony to blue")
		djangotest.AssertEqual(t, operation.NameFragment(), "rename_pink_pony_blue")
		newState := stateForwards(t, "test_rnfl", projectState, operation)
		pony := modelOf(t, newState, "test_rnfl", "Pony")
		if _, ok := pony.FieldByName("Blue"); !ok {
			t.Error("the state has no field Blue")
		}
		if _, ok := pony.FieldByName("Pink"); ok {
			t.Error("the state still has the field Pink")
		}
		// Rename field.
		djangotest.AssertColumnExists(t, db, "test_rnfl_pony", "pink")
		djangotest.AssertColumnNotExists(t, db, "test_rnfl_pony", "blue")
		databaseForwards(t, db, "test_rnfl", operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, "test_rnfl_pony", "blue")
		djangotest.AssertColumnNotExists(t, db, "test_rnfl_pony", "pink")
		// Reversal.
		databaseBackwards(t, db, "test_rnfl", operation, newState, projectState)
		djangotest.AssertColumnExists(t, db, "test_rnfl_pony", "pink")
		djangotest.AssertColumnNotExists(t, db, "test_rnfl_pony", "blue")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_field_unique_together
func TestRenameFieldUniqueTogether(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rnflut", testModelOptions{uniqueTogether: true})
		operation := migrations.RenameField{Model: "Pony", OldName: "Pink", NewName: "Blue"}
		newState := stateForwards(t, "test_rnflut", projectState, operation)
		// unique_together has the renamed column.
		djangotest.AssertEqual(t,
			modelOf(t, newState, "test_rnflut", "Pony").UniqueTogether, [][]string{{"Blue", "Weight"}})
		// The state before the operation is untouched.
		djangotest.AssertEqual(t,
			modelOf(t, projectState, "test_rnflut", "Pony").UniqueTogether, [][]string{{"Pink", "Weight"}})
		// Rename field.
		djangotest.AssertColumnExists(t, db, "test_rnflut_pony", "pink")
		djangotest.AssertColumnNotExists(t, db, "test_rnflut_pony", "blue")
		databaseForwards(t, db, "test_rnflut", operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, "test_rnflut_pony", "blue")
		djangotest.AssertColumnNotExists(t, db, "test_rnflut_pony", "pink")
		// The unique constraint has been ported over.
		mustExec(t, db, "INSERT INTO test_rnflut_pony (blue, weight) VALUES (1, 1)")
		if err := execSQL(db, "INSERT INTO test_rnflut_pony (blue, weight) VALUES (1, 1)"); err == nil {
			t.Error("the duplicate row was accepted")
		}
		mustExec(t, db, "DELETE FROM test_rnflut_pony")
		// Reversal.
		databaseBackwards(t, db, "test_rnflut", operation, newState, projectState)
		djangotest.AssertColumnExists(t, db, "test_rnflut_pony", "pink")
		djangotest.AssertColumnNotExists(t, db, "test_rnflut_pony", "blue")
		djangotest.AssertUniqueConstraintExists(t, db, "test_rnflut_pony", "pink", "weight")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_field_with_db_column
//
// Django counts the queries; the Go test collects the SQL of the operation
// and expects none, then runs it and checks that the column kept its name.
func TestRenameFieldWithDBColumn(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		fk := fkField("FKField", "db_fk_field", "test_rfwdbc.Pony")
		projectState := applyOps(t, db, "test_rfwdbc", migrations.NewProjectState(),
			migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{
				idField,
				{Name: "Field", Column: "db_field", Kind: orm.KindInt},
				fk,
			}},
		)
		check := func(operation migrations.RenameField, column string) {
			t.Helper()
			newState := stateForwards(t, "test_rfwdbc", projectState, operation)
			pony := modelOf(t, newState, "test_rfwdbc", "Pony")
			if _, ok := pony.FieldByName(operation.NewName); !ok {
				t.Errorf("the state has no field %s", operation.NewName)
			}
			if _, ok := pony.FieldByName(operation.OldName); ok {
				t.Errorf("the state still has the field %s", operation.OldName)
			}
			djangotest.AssertColumnExists(t, db, "test_rfwdbc_pony", column)
			ctx := context.Background()
			statements, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
				if err := operation.DatabaseForwards(ctx, "test_rfwdbc", e, projectState, newState); err != nil {
					return err
				}
				return operation.DatabaseBackwards(ctx, "test_rfwdbc", e, newState, projectState)
			})
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, len(statements), 0)
			databaseForwards(t, db, "test_rfwdbc", operation, projectState, newState)
			djangotest.AssertColumnExists(t, db, "test_rfwdbc_pony", column)
			databaseBackwards(t, db, "test_rfwdbc", operation, newState, projectState)
			djangotest.AssertColumnExists(t, db, "test_rfwdbc_pony", column)
		}
		check(migrations.RenameField{Model: "Pony", OldName: "Field", NewName: "RenamedField"}, "db_field")
		check(migrations.RenameField{Model: "Pony", OldName: "FKField", NewName: "RenamedFKField"}, "db_fk_field")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_rename_missing_field
func TestRenameMissingField(t *testing.T) {
	state := migrations.NewProjectState()
	state.AddModel(&migrations.ModelState{App: "app", Name: "Model", Table: "app_model"})
	err := migrations.RenameField{Model: "Model", OldName: "Field", NewName: "NewField"}.StateForwards("app", state)
	if err == nil || !strings.Contains(err.Error(), `app.Model has no field named "Field"`) {
		t.Errorf("got error %v, want the missing field error", err)
	}
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_unique_together
func TestAlterUniqueTogether(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_alunto", testModelOptions{})
		// Test the state alteration.
		operation := migrations.AlterUniqueTogether{Name: "Pony", UniqueTogether: [][]string{{"Pink", "Weight"}}}
		djangotest.AssertEqual(t, operation.Describe(), "Alter unique_together for pony (1 constraint(s))")
		djangotest.AssertEqual(t,
			migrations.FormattedDescription(operation), "~ Alter unique_together for pony (1 constraint(s))")
		djangotest.AssertEqual(t, operation.NameFragment(), "alter_pony_unique_together")
		newState := stateForwards(t, "test_alunto", projectState, operation)
		djangotest.AssertEqual(t, len(modelOf(t, projectState, "test_alunto", "Pony").UniqueTogether), 0)
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_alunto", "Pony").UniqueTogether), 1)
		// Make sure duplicate rows can be inserted.
		const insert = "INSERT INTO test_alunto_pony (pink, weight) VALUES (1, 1)"
		mustExec(t, db, insert)
		mustExec(t, db, insert)
		mustExec(t, db, "DELETE FROM test_alunto_pony")
		// Test the database alteration.
		databaseForwards(t, db, "test_alunto", operation, projectState, newState)
		mustExec(t, db, insert)
		if err := execSQL(db, insert); err == nil {
			t.Error("the duplicate row was accepted")
		}
		mustExec(t, db, "DELETE FROM test_alunto_pony")
		// And test reversal.
		databaseBackwards(t, db, "test_alunto", operation, newState, projectState)
		mustExec(t, db, insert)
		mustExec(t, db, insert)
		mustExec(t, db, "DELETE FROM test_alunto_pony")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_unique_together_remove
func TestAlterUniqueTogetherRemove(t *testing.T) {
	operation := migrations.AlterUniqueTogether{Name: "Pony"}
	djangotest.AssertEqual(t, operation.Describe(), "Alter unique_together for pony (0 constraint(s))")
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_index
func TestAddIndex(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_adin", testModelOptions{})
		unnamed := migrations.AddIndex{Model: "Pony", Index: migrations.Index{Fields: []string{"Pink"}}}
		err := unnamed.StateForwards("test_adin", projectState.Clone())
		if err == nil || !strings.Contains(err.Error(), "indexes passed to AddIndex operations require a name argument") {
			t.Errorf("got error %v, want the missing name error", err)
		}
		index := migrations.Index{Name: "test_adin_pony_pink_idx", Fields: []string{"Pink"}}
		operation := migrations.AddIndex{Model: "Pony", Index: index}
		djangotest.AssertEqual(t,
			operation.Describe(), "Create index test_adin_pony_pink_idx on field(s) pink of model pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation),
			"+ Create index test_adin_pony_pink_idx on field(s) pink of model pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "pony_test_adin_pony_pink_idx")
		newState := stateForwards(t, "test_adin", projectState, operation)
		// Test the database alteration.
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_adin", "Pony").Indexes), 1)
		djangotest.AssertIndexNotExists(t, db, "test_adin_pony", "pink")
		databaseForwards(t, db, "test_adin", operation, projectState, newState)
		djangotest.AssertIndexExists(t, db, "test_adin_pony", "pink")
		djangotest.AssertIndexNameExists(t, db, "test_adin_pony", "test_adin_pony_pink_idx")
		// And test reversal.
		databaseBackwards(t, db, "test_adin", operation, newState, projectState)
		djangotest.AssertIndexNotExists(t, db, "test_adin_pony", "pink")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_index
func TestRemoveIndex(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_rmin", testModelOptions{multicolIndex: true})
		djangotest.AssertTableExists(t, db, "test_rmin_pony")
		djangotest.AssertIndexExists(t, db, "test_rmin_pony", "pink", "weight")
		operation := migrations.RemoveIndex{Model: "Pony", Name: "pony_test_idx"}
		djangotest.AssertEqual(t, operation.Describe(), "Remove index pony_test_idx from pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "- Remove index pony_test_idx from pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "remove_pony_pony_test_idx")
		newState := stateForwards(t, "test_rmin", projectState, operation)
		// Test the state alteration.
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_rmin", "Pony").Indexes), 0)
		djangotest.AssertIndexExists(t, db, "test_rmin_pony", "pink", "weight")
		// Test the database alteration.
		databaseForwards(t, db, "test_rmin", operation, projectState, newState)
		djangotest.AssertIndexNotExists(t, db, "test_rmin_pony", "pink", "weight")
		// And test reversal.
		databaseBackwards(t, db, "test_rmin", operation, newState, projectState)
		djangotest.AssertIndexExists(t, db, "test_rmin_pony", "pink", "weight")

		// Also test a field dropped with its index: the SQLite rebuild must cope.
		operations := []migrations.Operation{
			migrations.RemoveIndex{Model: "Pony", Name: "pony_test_idx"},
			migrations.RemoveField{Model: "Pony", Field: "Pink"},
		}
		djangotest.AssertColumnExists(t, db, "test_rmin_pony", "pink")
		djangotest.AssertIndexExists(t, db, "test_rmin_pony", "pink", "weight")
		// Test database alteration.
		applyOps(t, db, "test_rmin", projectState, operations...)
		djangotest.AssertColumnNotExists(t, db, "test_rmin_pony", "pink")
		djangotest.AssertIndexNotExists(t, db, "test_rmin_pony", "pink", "weight")
		// And test reversal.
		unapplyOps(t, db, "test_rmin", projectState, operations...)
		djangotest.AssertIndexExists(t, db, "test_rmin_pony", "pink", "weight")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_index_state_forwards
//
// Django checks that the rendered model class is replaced. The Go state has
// no rendered models: the operation must change the copy and leave the
// original alone.
func TestAddIndexStateForwards(t *testing.T) {
	projectState := migrations.NewProjectState()
	create := migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{idField, pinkField}}
	projectState = stateForwards(t, "test_adinsf", projectState, create)
	operation := migrations.AddIndex{
		Model: "Pony", Index: migrations.Index{Name: "test_adinsf_pony_pink_idx", Fields: []string{"Pink"}},
	}
	newState := stateForwards(t, "test_adinsf", projectState, operation)
	djangotest.AssertEqual(t, len(modelOf(t, projectState, "test_adinsf", "Pony").Indexes), 0)
	djangotest.AssertEqual(t, len(modelOf(t, newState, "test_adinsf", "Pony").Indexes), 1)
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_index_state_forwards
//
// See TestAddIndexStateForwards for what replaces the model identity check.
func TestRemoveIndexStateForwards(t *testing.T) {
	projectState := migrations.NewProjectState()
	create := migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{idField, pinkField}}
	projectState = stateForwards(t, "test_rminsf", projectState, create)
	projectState = stateForwards(t, "test_rminsf", projectState, migrations.AddIndex{
		Model: "Pony", Index: migrations.Index{Name: "test_rminsf_pony_pink_idx", Fields: []string{"Pink"}},
	})
	operation := migrations.RemoveIndex{Model: "Pony", Name: "test_rminsf_pony_pink_idx"}
	newState := stateForwards(t, "test_rminsf", projectState, operation)
	djangotest.AssertEqual(t, len(modelOf(t, projectState, "test_rminsf", "Pony").Indexes), 1)
	djangotest.AssertEqual(t, len(modelOf(t, newState, "test_rminsf", "Pony").Indexes), 0)
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_field_with_index
func TestAlterFieldWithIndex(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_alflin", testModelOptions{index: true})
		operation := migrations.AlterField{Model: "Pony", Field: migrations.FieldState{
			Name: "Pink", Column: "pink", Kind: orm.KindInt, Null: true,
		}}
		newState := stateForwards(t, "test_alflin", projectState, operation)
		// Test the database alteration.
		djangotest.AssertColumnNotNull(t, db, "test_alflin_pony", "pink")
		databaseForwards(t, db, "test_alflin", operation, projectState, newState)
		// Index hasn't been dropped.
		djangotest.AssertIndexExists(t, db, "test_alflin_pony", "pink")
		// And test reversal.
		databaseBackwards(t, db, "test_alflin", operation, newState, projectState)
		// Ensure the index is still there.
		djangotest.AssertIndexExists(t, db, "test_alflin_pony", "pink")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_fk
//
// Django alters the option editable, which the Go state does not track. The
// alteration here changes the ON DELETE action, in the same schema editor as
// the creation of the table.
func TestAlterFK(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_alfk", testModelOptions{})
		restricted := fkField("Pony", "pony_id", "test_alfk.Pony")
		restricted.RelOnDelete = orm.OnDeleteRestrict
		applyOps(t, db, "test_alfk", projectState,
			migrations.CreateModel{Name: "Rider", Fields: []migrations.FieldState{
				idField, fkField("Pony", "pony_id", "test_alfk.Pony"),
			}},
			migrations.AlterField{Model: "Rider", Field: restricted},
		)
		djangotest.AssertFKExists(t, db, "test_alfk_rider", "pony_id", "test_alfk_pony", "id")
		if db.Dialect().Name() == "sqlite" {
			// A SQLite connection enforces foreign keys only on request, so
			// read the action from the table definition.
			var ddl string
			row := db.SQL().QueryRowContext(context.Background(),
				"SELECT sql FROM sqlite_master WHERE name = 'test_alfk_rider'")
			if err := row.Scan(&ddl); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(ddl, "ON DELETE RESTRICT") {
				t.Errorf("the foreign key does not restrict deletes: %s", ddl)
			}
			return
		}
		mustExec(t, db, "INSERT INTO test_alfk_pony (id, pink, weight) VALUES (1, 3, 1)")
		mustExec(t, db, "INSERT INTO test_alfk_rider (pony_id) VALUES (1)")
		if err := execSQL(db, "DELETE FROM test_alfk_pony"); err == nil {
			t.Error("the referenced row was deleted although the foreign key restricts it")
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_fk_non_fk
func TestAlterFKNonFK(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_afknfk", testModelOptions{relatedModel: true})
		// Test the state alteration.
		operation := migrations.AlterField{Model: "Rider", Field: migrations.FieldState{
			Name: "Pony", Column: "pony", Kind: orm.KindInt,
		}}
		newState := stateForwards(t, "test_afknfk", projectState, operation)
		// Test the database alteration.
		djangotest.AssertColumnExists(t, db, "test_afknfk_rider", "pony_id")
		djangotest.AssertColumnNotExists(t, db, "test_afknfk_rider", "pony")
		databaseForwards(t, db, "test_afknfk", operation, projectState, newState)
		djangotest.AssertColumnExists(t, db, "test_afknfk_rider", "pony")
		djangotest.AssertColumnNotExists(t, db, "test_afknfk_rider", "pony_id")
		djangotest.AssertFKNotExists(t, db, "test_afknfk_rider", "pony", "test_afknfk_pony", "id")
		// And test reversal.
		databaseBackwards(t, db, "test_afknfk", operation, newState, projectState)
		djangotest.AssertColumnExists(t, db, "test_afknfk_rider", "pony_id")
		djangotest.AssertColumnNotExists(t, db, "test_afknfk_rider", "pony")
		djangotest.AssertFKExists(t, db, "test_afknfk_rider", "pony_id", "test_afknfk_pony", "id")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_sql
//
// Django splits a multi-statement string. RunSQL takes one statement per
// element, so nothing is split and "%" needs no escaping.
func TestRunSQL(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_runsql", testModelOptions{})
		// Create the operation.
		operation := migrations.RunSQL{
			SQL: []string{
				"CREATE TABLE i_love_ponies (id int, special_thing varchar(15));",
				"INSERT INTO i_love_ponies (id, special_thing) VALUES (1, 'i love ponies'); -- this is magic!",
				"INSERT INTO i_love_ponies (id, special_thing) VALUES (2, 'i love django');",
				"UPDATE i_love_ponies SET special_thing = 'Ponies' WHERE special_thing LIKE '%ponies';",
				"UPDATE i_love_ponies SET special_thing = 'Django' WHERE special_thing LIKE '%django';",
			},
			ReverseSQL: []string{
				"DELETE FROM i_love_ponies WHERE special_thing LIKE '%Django%';",
				"DELETE FROM i_love_ponies WHERE special_thing LIKE '%Ponies%';",
				"DROP TABLE i_love_ponies",
			},
			StateOperations: []migrations.Operation{
				migrations.CreateModel{Name: "SomethingElse", Fields: []migrations.FieldState{idField}},
			},
		}
		djangotest.AssertEqual(t, operation.Describe(), "Raw SQL operation")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "s Raw SQL operation")
		// Test the state alteration.
		newState := stateForwards(t, "test_runsql", projectState, operation)
		djangotest.AssertEqual(t, len(modelOf(t, newState, "test_runsql", "SomethingElse").Fields), 1)
		// Make sure there's no table.
		djangotest.AssertTableNotExists(t, db, "i_love_ponies")
		// Test SQL collection.
		ctx := context.Background()
		collected, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
			if err := operation.DatabaseForwards(ctx, "test_runsql", e, projectState, newState); err != nil {
				return err
			}
			return operation.DatabaseBackwards(ctx, "test_runsql", e, newState, projectState)
		})
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(collected, "\n")
		if !strings.Contains(joined, "LIKE '%ponies';") || !strings.Contains(joined, "LIKE '%Ponies%';") {
			t.Errorf("collected SQL lacks the statements: %s", joined)
		}
		// Test the database alteration.
		databaseForwards(t, db, "test_runsql", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "i_love_ponies")
		// Make sure all the SQL was processed.
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM i_love_ponies"), 2)
		djangotest.AssertEqual(t,
			queryInt(t, db, "SELECT COUNT(*) FROM i_love_ponies WHERE special_thing = 'Django'"), 1)
		djangotest.AssertEqual(t,
			queryInt(t, db, "SELECT COUNT(*) FROM i_love_ponies WHERE special_thing = 'Ponies'"), 1)
		// And test reversal.
		djangotest.AssertEqual(t, operation.Reversible(), true)
		databaseBackwards(t, db, "test_runsql", operation, newState, projectState)
		djangotest.AssertTableNotExists(t, db, "i_love_ponies")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_sql_noop
//
// Django's RunSQL.noop is an empty, non-nil statement list here.
func TestRunSQLNoop(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		operation := migrations.RunSQL{SQL: []string{}, ReverseSQL: []string{}}
		state := migrations.NewProjectState()
		databaseForwards(t, db, "test_runsql", operation, state, state)
		databaseBackwards(t, db, "test_runsql", operation, state, state)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_sql_add_missing_semicolon_on_collect_sql
func TestRunSQLAddMissingSemicolonOnCollectSQL(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		state := migrations.NewProjectState()
		tests := []string{
			"INSERT INTO test_runsql_pony (pink, weight) VALUES (1, 1);\n",
			"INSERT INTO test_runsql_pony (pink, weight) VALUES (1, 1)\n",
		}
		for _, sql := range tests {
			operation := migrations.RunSQL{SQL: []string{sql}, ReverseSQL: []string{}}
			collected, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
				return operation.DatabaseForwards(context.Background(), "test_runsql", e, state, state)
			})
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, strings.Count(strings.Join(collected, "\n"), ";"), 1)
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_sql_backward_reverse_sql_required
func TestRunSQLBackwardReverseSQLRequired(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		operation := migrations.RunSQL{SQL: []string{}}
		djangotest.AssertEqual(t, operation.Reversible(), false)
		state := migrations.NewProjectState()
		ctx := context.Background()
		err := migrations.WithSchemaEditor(ctx, db, func(e *migrations.SchemaEditor) error {
			return operation.DatabaseBackwards(ctx, "test_runsql", e, state, state)
		})
		if err == nil || !strings.Contains(err.Error(), "you cannot reverse this operation") {
			t.Errorf("got error %v, want the irreversible error", err)
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_python
//
// RunGo is the counterpart of RunPython. The functions receive a database
// handle bound to the migration transaction and the historical state.
func TestRunPython(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_runpython", testModelOptions{})
		var seen *migrations.ProjectState
		// Create the operation.
		innerMethod := func(ctx context.Context, tx *orm.DB, state *migrations.ProjectState) error {
			seen = state
			if _, err := tx.SQL().ExecContext(ctx, "INSERT INTO test_runpython_pony (pink, weight) VALUES (1, 3)"); err != nil {
				return err
			}
			_, err := tx.SQL().ExecContext(ctx, "INSERT INTO test_runpython_pony (pink, weight) VALUES (3, 5)")
			return err
		}
		innerMethodReverse := func(ctx context.Context, tx *orm.DB, _ *migrations.ProjectState) error {
			_, err := tx.SQL().ExecContext(ctx, "DELETE FROM test_runpython_pony WHERE weight IN (3, 5)")
			return err
		}
		operation := migrations.RunGo{Forward: innerMethod, Reverse: innerMethodReverse}
		djangotest.AssertEqual(t, operation.Describe(), "Raw Go operation")
		djangotest.AssertEqual(t, migrations.FormattedDescription(operation), "g Raw Go operation")
		// Test the state alteration does nothing.
		newState := stateForwards(t, "test_runpython", projectState, operation)
		djangotest.AssertEqual(t, newState, projectState)
		// Test the database alteration.
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_runpython_pony"), 0)
		databaseForwards(t, db, "test_runpython", operation, projectState, newState)
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_runpython_pony"), 2)
		if seen != projectState {
			t.Error("the function did not receive the state before the operation")
		}
		// Now test reversal.
		djangotest.AssertEqual(t, operation.Reversible(), true)
		databaseBackwards(t, db, "test_runpython", operation, newState, projectState)
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_runpython_pony"), 0)

		// Also test reversal fails, with an operation identical to above but
		// without the reverse function set.
		noReverse := migrations.RunGo{Forward: innerMethod}
		djangotest.AssertEqual(t, noReverse.Reversible(), false)
		ctx := context.Background()
		err := migrations.WithSchemaEditor(ctx, db, func(e *migrations.SchemaEditor) error {
			if err := noReverse.DatabaseForwards(ctx, "test_runpython", e, projectState, newState); err != nil {
				return err
			}
			return noReverse.DatabaseBackwards(ctx, "test_runpython", e, newState, projectState)
		})
		if err == nil || !strings.Contains(err.Error(), "you cannot reverse this operation") {
			t.Errorf("got error %v, want the irreversible error", err)
		}
		// The failed editor rolled the forward step back.
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_runpython_pony"), 0)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_python_atomic
//
// Both dialects roll back DDL and Go migrations are always atomic, so only the
// branch for fully transactional databases applies: the insert of a failing
// function is rolled back.
func TestRunPythonAtomic(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := setUpTestModel(t, db, "test_runpythonatomic", testModelOptions{})
		errAdrian := errors.New("adrian hates ponies")
		innerMethod := func(ctx context.Context, tx *orm.DB, _ *migrations.ProjectState) error {
			_, err := tx.SQL().ExecContext(ctx, "INSERT INTO test_runpythonatomic_pony (pink, weight) VALUES (1, 3)")
			if err != nil {
				return err
			}
			return errAdrian
		}
		operation := migrations.RunGo{Forward: innerMethod, Reverse: innerMethod}
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_runpythonatomic_pony"), 0)
		_, err := tryApplyOps(db, "test_runpythonatomic", projectState, operation)
		djangotest.AssertErrorIs(t, err, errAdrian)
		djangotest.AssertEqual(t, queryInt(t, db, "SELECT COUNT(*) FROM test_runpythonatomic_pony"), 0)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_run_python_noop
func TestRunPythonNoop(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		projectState := migrations.NewProjectState()
		newState := projectState.Clone()
		operation := migrations.RunGo{Forward: migrations.Noop, Reverse: migrations.Noop}
		databaseForwards(t, db, "test_runpython", operation, projectState, newState)
		databaseBackwards(t, db, "test_runpython", operation, newState, projectState)
	})
}

// plainOperation is a custom operation that declares no category.
type plainOperation struct{}

func (plainOperation) StateForwards(string, *migrations.ProjectState) error { return nil }

func (plainOperation) DatabaseForwards(context.Context, string, *migrations.SchemaEditor, *migrations.ProjectState, *migrations.ProjectState) error {
	return nil
}

func (plainOperation) DatabaseBackwards(context.Context, string, *migrations.SchemaEditor, *migrations.ProjectState, *migrations.ProjectState) error {
	return nil
}

func (plainOperation) Describe() string { return "Operation: ((), {})" }

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_formatted_description_no_category
func TestFormattedDescriptionNoCategory(t *testing.T) {
	djangotest.AssertEqual(t, migrations.FormattedDescription(plainOperation{}), "? Operation: ((), {})")
}
