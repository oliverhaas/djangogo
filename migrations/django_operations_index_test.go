package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file continue Django's OperationTests with field
// renames, unique-together sets, indexes and foreign key alterations.

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
