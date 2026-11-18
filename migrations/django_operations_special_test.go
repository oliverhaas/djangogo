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

// The tests in this file continue Django's OperationTests with the
// operations that run SQL or Go code, and with operation descriptions.

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
