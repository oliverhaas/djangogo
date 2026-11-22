package migrations_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file continue Django's OperationTests with check and
// unique constraints. The test models have no Go type, so rows are written
// with SQL, and a statement that fails stands for Django's IntegrityError.

// assertRejected fails the test when the statement succeeds.
func assertRejected(t *testing.T, db *orm.DB, query string, args ...any) {
	t.Helper()
	if err := execSQL(db, query, args...); err == nil {
		t.Errorf("the database accepted %q %v", query, args)
	}
}

// insertPony inserts a row into the Pony table of app.
func insertPony(t *testing.T, db *orm.DB, app string, pink, weight int) {
	t.Helper()
	mustExec(t, db, "INSERT INTO "+app+"_pony (pink, weight) VALUES (?, ?)", pink, weight)
}

// rejectPony asserts that the Pony table of app rejects the row.
func rejectPony(t *testing.T, db *orm.DB, app string, pink, weight int) {
	t.Helper()
	assertRejected(t, db, "INSERT INTO "+app+"_pony (pink, weight) VALUES (?, ?)", pink, weight)
}

// constraintCount returns the number of constraints of a model in the state.
func constraintCount(t *testing.T, state *migrations.ProjectState, app, model string) int {
	t.Helper()
	return len(modelOf(t, state, app, model).Constraints)
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_create_model_with_constraint
func TestCreateModelWithConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		checkConstraint := orm.CheckConstraint{Condition: orm.Q("pink__gt", 2), Name: "test_constraint_pony_pink_gt_2"}
		operation := migrations.CreateModel{
			Name:        "Pony",
			Fields:      []migrations.FieldState{idField, pinkField},
			Constraints: []orm.Constraint{checkConstraint},
		}
		// Test the state alteration.
		projectState := migrations.NewProjectState()
		newState := stateForwards(t, "test_crmo", projectState, operation)
		djangotest.AssertEqual(t, constraintCount(t, newState, "test_crmo", "Pony"), 1)
		// Test database alteration.
		djangotest.AssertTableNotExists(t, db, "test_crmo_pony")
		databaseForwards(t, db, "test_crmo", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "test_crmo_pony")
		assertRejected(t, db, "INSERT INTO test_crmo_pony (id, pink) VALUES (1, 1)")
		// Test reversal.
		databaseBackwards(t, db, "test_crmo", operation, newState, projectState)
		djangotest.AssertTableNotExists(t, db, "test_crmo_pony")
	})
}

// No Go counterpart: OperationTests.test_create_model_with_boolean_expression_in_check_constraint
// builds its conditions from RawSQL and ExpressionWrapper. A Go condition is
// an orm.Cond, which has neither.

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_create_model_with_partial_unique_constraint
func TestCreateModelWithPartialUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		partialUniqueConstraint := orm.UniqueConstraint{
			Fields:    []string{"pink"},
			Condition: orm.Q("weight__gt", 5),
			Name:      "test_constraint_pony_pink_for_weight_gt_5_uniq",
		}
		operation := migrations.CreateModel{
			Name:        "Pony",
			Fields:      []migrations.FieldState{idField, pinkField, weightField},
			Constraints: []orm.Constraint{partialUniqueConstraint},
		}
		// Test the state alteration.
		projectState := migrations.NewProjectState()
		newState := stateForwards(t, "test_crmo", projectState, operation)
		djangotest.AssertEqual(t, constraintCount(t, newState, "test_crmo", "Pony"), 1)
		// Test database alteration.
		djangotest.AssertTableNotExists(t, db, "test_crmo_pony")
		databaseForwards(t, db, "test_crmo", operation, projectState, newState)
		djangotest.AssertTableExists(t, db, "test_crmo_pony")
		// Test constraint works.
		insertPony(t, db, "test_crmo", 1, 4)
		insertPony(t, db, "test_crmo", 1, 4)
		insertPony(t, db, "test_crmo", 1, 6)
		rejectPony(t, db, "test_crmo", 1, 7)
		// Test reversal.
		databaseBackwards(t, db, "test_crmo", operation, newState, projectState)
		djangotest.AssertTableNotExists(t, db, "test_crmo_pony")
	})
}

// No Go counterpart: OperationTests.test_create_model_with_deferred_unique_constraint
// and test_create_model_with_covering_unique_constraint test the deferrable
// and include options, which orm.UniqueConstraint does not have.

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_constraint
func TestAddConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_addconstraint"
		projectState := setUpTestModel(t, db, app, testModelOptions{})
		gtConstraint := orm.CheckConstraint{Condition: orm.Q("pink__gt", 2), Name: "test_add_constraint_pony_pink_gt_2"}
		gtOperation := migrations.AddConstraint{Model: "Pony", Constraint: gtConstraint}
		djangotest.AssertEqual(t, gtOperation.Describe(),
			"Create constraint test_add_constraint_pony_pink_gt_2 on model pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(gtOperation),
			"+ Create constraint test_add_constraint_pony_pink_gt_2 on model pony")
		djangotest.AssertEqual(t, gtOperation.NameFragment(), "pony_test_add_constraint_pony_pink_gt_2")
		// Test the state alteration.
		newState := stateForwards(t, app, projectState, gtOperation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 1)
		// Test the database alteration.
		databaseForwards(t, db, app, gtOperation, projectState, newState)
		rejectPony(t, db, app, 1, 1)
		// Add another one.
		ltConstraint := orm.CheckConstraint{Condition: orm.Q("pink__lt", 100), Name: "test_add_constraint_pony_pink_lt_100"}
		ltOperation := migrations.AddConstraint{Model: "Pony", Constraint: ltConstraint}
		newState = stateForwards(t, app, newState, ltOperation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 2)
		databaseForwards(t, db, app, ltOperation, projectState, newState)
		rejectPony(t, db, app, 100, 1)
		// Test reversal.
		databaseBackwards(t, db, app, gtOperation, newState, projectState)
		insertPony(t, db, app, 1, 1)
	})
}

// percentEscapingChecks are the conditions of Django's percent escaping
// tests, each with a valid and an invalid row of the Author table.
var percentEscapingChecks = []struct {
	check          orm.Cond
	valid, invalid map[string]string
}{
	// "%" generated in startswith lookup should be escaped in a way that is
	// considered a leading wildcard.
	{orm.Q("name__startswith", "Albert"), map[string]string{"name": "Alberta"}, map[string]string{"name": "Artur"}},
	// Literal "%" should be escaped in a way that is not a considered a
	// wildcard.
	{orm.Q("rebate__endswith", "%"), map[string]string{"rebate": "10%"}, map[string]string{"rebate": "10%$"}},
	// Right-hand-side baked "%" literals should not be used for parameters
	// interpolation.
	{
		orm.Not(orm.Q("surname__startswith", orm.F("name"))),
		map[string]string{"name": "Albert"},
		map[string]string{"name": "Albert", "surname": "Alberto"},
	},
	// Exact matches against "%" literals should also be supported.
	{orm.Q("name", "%"), map[string]string{"name": "%"}, map[string]string{"name": "Albert"}},
}

// authorFields are the fields of the Author model of the percent escaping
// tests.
var authorFields = []migrations.FieldState{
	idField,
	{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: 100},
	{Name: "Surname", Column: "surname", Kind: orm.KindChar, MaxLength: 100, Default: ""},
	{Name: "Rebate", Column: "rebate", Kind: orm.KindChar, MaxLength: 100},
}

// insertAuthor inserts an Author row with the given values and "" for the
// other columns. Django's model creates the row with the defaults of the
// fields; the Go test names every column.
func insertAuthor(db *orm.DB, app string, values map[string]string) error {
	row := map[string]string{"name": "", "surname": "", "rebate": ""}
	for k, v := range values {
		row[k] = v
	}
	return execSQL(db, "INSERT INTO "+app+"_author (name, surname, rebate) VALUES (?, ?, ?)",
		row["name"], row["surname"], row["rebate"])
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_create_model_constraint_percent_escaping
func TestCreateModelConstraintPercentEscaping(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "add_constraint_string_quoting"
		fromState := migrations.NewProjectState()
		for i, tt := range percentEscapingChecks {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				constraint := orm.CheckConstraint{Condition: tt.check, Name: "constraint"}
				operation := migrations.CreateModel{Name: "Author", Fields: authorFields, Constraints: []orm.Constraint{constraint}}
				toState := stateForwards(t, app, fromState, operation)
				databaseForwards(t, db, app, operation, fromState, toState)
				defer databaseForwards(t, db, app, migrations.DeleteModel{Name: "Author"}, toState, fromState)
				if err := insertAuthor(db, app, tt.valid); err != nil {
					t.Errorf("the valid row %v was rejected: %v", tt.valid, err)
				}
				mustExec(t, db, "DELETE FROM "+app+"_author")
				if err := insertAuthor(db, app, tt.invalid); err == nil {
					t.Errorf("the invalid row %v was accepted", tt.invalid)
				}
			})
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_constraint_percent_escaping
func TestAddConstraintPercentEscaping(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "add_constraint_string_quoting"
		fromState := applyOps(t, db, app, migrations.NewProjectState(),
			migrations.CreateModel{Name: "Author", Fields: authorFields})
		for i, tt := range percentEscapingChecks {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				constraint := orm.CheckConstraint{Condition: tt.check, Name: "constraint"}
				operation := migrations.AddConstraint{Model: "Author", Constraint: constraint}
				toState := stateForwards(t, app, fromState, operation)
				databaseForwards(t, db, app, operation, fromState, toState)
				defer databaseBackwards(t, db, app, operation, toState, fromState)
				if err := insertAuthor(db, app, tt.valid); err != nil {
					t.Errorf("the valid row %v was rejected: %v", tt.valid, err)
				}
				mustExec(t, db, "DELETE FROM "+app+"_author")
				if err := insertAuthor(db, app, tt.invalid); err == nil {
					t.Errorf("the invalid row %v was accepted", tt.invalid)
				}
			})
		}
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_or_constraint
func TestAddOrConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_addorconstraint"
		fromState := setUpTestModel(t, db, app, testModelOptions{})
		check := orm.Q("pink__gt", 2, "weight__gt", 2).Or(orm.Q("weight__lt", 0))
		operation := migrations.AddConstraint{Model: "Pony", Constraint: orm.CheckConstraint{Condition: check, Name: "add_constraint_or"}}
		toState := stateForwards(t, app, fromState, operation)
		databaseForwards(t, db, app, operation, fromState, toState)
		rejectPony(t, db, app, 2, 3)
		rejectPony(t, db, app, 3, 1)
		insertPony(t, db, app, 3, -1)
		insertPony(t, db, app, 1, -1)
		insertPony(t, db, app, 3, 3)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_constraint_combinable
func TestAddConstraintCombinable(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_addconstraint_combinable"
		fromState := applyOps(t, db, app, migrations.NewProjectState(), migrations.CreateModel{
			Name: "Book",
			Fields: []migrations.FieldState{
				idField,
				{Name: "Read", Column: "read", Kind: orm.KindInt},
				{Name: "Unread", Column: "unread", Kind: orm.KindInt},
			},
		})
		constraint := orm.CheckConstraint{
			Condition: orm.Q("read", orm.Value(100).Sub(orm.F("unread"))),
			Name:      "test_addconstraint_combinable_sum_100",
		}
		operation := migrations.AddConstraint{Model: "Book", Constraint: constraint}
		toState := stateForwards(t, app, fromState, operation)
		databaseForwards(t, db, app, operation, fromState, toState)
		assertRejected(t, db, "INSERT INTO "+app+"_book (read, unread) VALUES (?, ?)", 70, 10)
		mustExec(t, db, "INSERT INTO "+app+"_book (read, unread) VALUES (?, ?)", 70, 30)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_constraint
func TestRemoveConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_removeconstraint"
		projectState := setUpTestModel(t, db, app, testModelOptions{constraints: []orm.Constraint{
			orm.CheckConstraint{Condition: orm.Q("pink__gt", 2), Name: "test_remove_constraint_pony_pink_gt_2"},
			orm.CheckConstraint{Condition: orm.Q("pink__lt", 100), Name: "test_remove_constraint_pony_pink_lt_100"},
		}})
		gtOperation := migrations.RemoveConstraint{Model: "Pony", Name: "test_remove_constraint_pony_pink_gt_2"}
		djangotest.AssertEqual(t, gtOperation.Describe(),
			"Remove constraint test_remove_constraint_pony_pink_gt_2 from model pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(gtOperation),
			"- Remove constraint test_remove_constraint_pony_pink_gt_2 from model pony")
		djangotest.AssertEqual(t, gtOperation.NameFragment(), "remove_pony_test_remove_constraint_pony_pink_gt_2")
		// Test state alteration.
		newState := stateForwards(t, app, projectState, gtOperation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 1)
		// Test database alteration.
		databaseForwards(t, db, app, gtOperation, projectState, newState)
		insertPony(t, db, app, 1, 1)
		mustExec(t, db, "DELETE FROM "+app+"_pony")
		rejectPony(t, db, app, 100, 1)
		// Remove the other one.
		ltOperation := migrations.RemoveConstraint{Model: "Pony", Name: "test_remove_constraint_pony_pink_lt_100"}
		newState = stateForwards(t, app, newState, ltOperation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 0)
		databaseForwards(t, db, app, ltOperation, projectState, newState)
		insertPony(t, db, app, 100, 1)
		mustExec(t, db, "DELETE FROM "+app+"_pony")
		// Test reversal.
		databaseBackwards(t, db, app, gtOperation, newState, projectState)
		rejectPony(t, db, app, 1, 1)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_constraint
//
// Django's assertConstraintExists looks for check constraints only and so
// expects none for the unique constraint. The Go assertion finds every
// constraint, and the test checks that the operation leaves the unique
// constraint in place.
func TestAlterConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_alterconstraint"
		constraint := orm.UniqueConstraint{Fields: []string{"Pink"}, Name: "test_alter_constraint_pony_fields_uq"}
		projectState := setUpTestModel(t, db, app, testModelOptions{constraints: []orm.Constraint{constraint}})
		violationErrorMessage := "Pink isn't unique"
		uqConstraint := orm.UniqueConstraint{
			Fields:                []string{"Pink"},
			Name:                  "test_alter_constraint_pony_fields_uq",
			ViolationErrorMessage: violationErrorMessage,
		}
		uqOperation := migrations.AlterConstraint{Model: "Pony", Name: "test_alter_constraint_pony_fields_uq", Constraint: uqConstraint}
		djangotest.AssertEqual(t, uqOperation.Describe(), "Alter constraint test_alter_constraint_pony_fields_uq on pony")
		djangotest.AssertEqual(t, migrations.FormattedDescription(uqOperation),
			"~ Alter constraint test_alter_constraint_pony_fields_uq on pony")
		djangotest.AssertEqual(t, uqOperation.NameFragment(), "alter_pony_test_alter_constraint_pony_fields_uq")

		newState := stateForwards(t, app, projectState, uqOperation)
		djangotest.AssertEqual(t, modelOf(t, projectState, app, "Pony").Constraints[0].ViolationMessage(),
			"Constraint “test_alter_constraint_pony_fields_uq” is violated.")
		djangotest.AssertEqual(t, modelOf(t, newState, app, "Pony").Constraints[0].ViolationMessage(), violationErrorMessage)

		ctx := context.Background()
		statements, err := migrations.CollectSQL(db.Dialect(), func(e *migrations.SchemaEditor) error {
			if err := uqOperation.DatabaseForwards(ctx, app, e, projectState, newState); err != nil {
				return err
			}
			return uqOperation.DatabaseBackwards(ctx, app, e, projectState, newState)
		})
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(statements), 0)
		databaseForwards(t, db, app, uqOperation, projectState, newState)
		djangotest.AssertConstraintExists(t, db, app+"_pony", "test_alter_constraint_pony_fields_uq")
		databaseBackwards(t, db, app, uqOperation, projectState, newState)
		djangotest.AssertConstraintExists(t, db, app+"_pony", "test_alter_constraint_pony_fields_uq")
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_partial_unique_constraint
func TestAddPartialUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_addpartialuniqueconstraint"
		projectState := setUpTestModel(t, db, app, testModelOptions{})
		partialUniqueConstraint := orm.UniqueConstraint{
			Fields:    []string{"Pink"},
			Condition: orm.Q("weight__gt", 5),
			Name:      "test_constraint_pony_pink_for_weight_gt_5_uniq",
		}
		operation := migrations.AddConstraint{Model: "Pony", Constraint: partialUniqueConstraint}
		djangotest.AssertEqual(t, operation.Describe(),
			"Create constraint test_constraint_pony_pink_for_weight_gt_5_uniq on model pony")
		// Test the state alteration.
		newState := stateForwards(t, app, projectState, operation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 1)
		// Test the database alteration.
		databaseForwards(t, db, app, operation, projectState, newState)
		// Test constraint works.
		insertPony(t, db, app, 1, 4)
		insertPony(t, db, app, 1, 4)
		insertPony(t, db, app, 1, 6)
		rejectPony(t, db, app, 1, 7)
		// Test reversal.
		databaseBackwards(t, db, app, operation, newState, projectState)
		// Test constraint doesn't work.
		insertPony(t, db, app, 1, 7)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_partial_unique_constraint
func TestRemovePartialUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_removepartialuniqueconstraint"
		projectState := setUpTestModel(t, db, app, testModelOptions{constraints: []orm.Constraint{
			orm.UniqueConstraint{
				Fields:    []string{"Pink"},
				Condition: orm.Q("weight__gt", 5),
				Name:      "test_constraint_pony_pink_for_weight_gt_5_uniq",
			},
		}})
		gtOperation := migrations.RemoveConstraint{Model: "Pony", Name: "test_constraint_pony_pink_for_weight_gt_5_uniq"}
		djangotest.AssertEqual(t, gtOperation.Describe(),
			"Remove constraint test_constraint_pony_pink_for_weight_gt_5_uniq from model pony")
		// Test state alteration.
		newState := stateForwards(t, app, projectState, gtOperation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 0)
		// Test database alteration.
		databaseForwards(t, db, app, gtOperation, projectState, newState)
		// Test constraint doesn't work.
		insertPony(t, db, app, 1, 4)
		insertPony(t, db, app, 1, 4)
		insertPony(t, db, app, 1, 6)
		insertPony(t, db, app, 1, 7)
		mustExec(t, db, "DELETE FROM "+app+"_pony WHERE weight = 7")
		// Test reversal.
		databaseBackwards(t, db, app, gtOperation, newState, projectState)
		// Test constraint works.
		rejectPony(t, db, app, 1, 7)
	})
}

// No Go counterpart: OperationTests.test_add_deferred_unique_constraint,
// test_remove_deferred_unique_constraint, test_add_covering_unique_constraint
// and test_remove_covering_unique_constraint test the deferrable and include
// options, which orm.UniqueConstraint does not have.

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_alter_field_with_func_unique_constraint
func TestAlterFieldWithFuncUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_alfuncuc"
		constraintName := app + "_pony_uq"
		tableName := app + "_pony"
		projectState := setUpTestModel(t, db, app, testModelOptions{constraints: []orm.Constraint{
			orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("pink"), orm.F("weight")}, Name: constraintName},
		}})
		pink := pinkField
		pink.Null = true
		operation := migrations.AlterField{Model: "Pony", Field: pink}
		newState := stateForwards(t, app, projectState, operation)
		databaseForwards(t, db, app, operation, projectState, newState)
		djangotest.AssertIndexNameExists(t, db, tableName, constraintName)
		databaseBackwards(t, db, app, operation, newState, projectState)
		djangotest.AssertIndexNameExists(t, db, tableName, constraintName)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_func_unique_constraint
//
// The orm has no Abs. The Go test squares the weight instead, which makes 4
// and -4 collide the same way.
func TestAddFuncUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_adfuncuc"
		constraintName := app + "_pony_abs_uq"
		tableName := app + "_pony"
		projectState := setUpTestModel(t, db, app, testModelOptions{})
		constraint := orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("weight").Mul(orm.F("weight"))}, Name: constraintName}
		operation := migrations.AddConstraint{Model: "Pony", Constraint: constraint}
		djangotest.AssertEqual(t, operation.Describe(), "Create constraint test_adfuncuc_pony_abs_uq on model pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "pony_test_adfuncuc_pony_abs_uq")
		newState := stateForwards(t, app, projectState, operation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 1)
		djangotest.AssertIndexNameNotExists(t, db, tableName, constraintName)
		// Add constraint.
		databaseForwards(t, db, app, operation, projectState, newState)
		insertPony(t, db, app, 3, 4)
		djangotest.AssertIndexNameExists(t, db, tableName, constraintName)
		rejectPony(t, db, app, 3, -4)
		// Reversal.
		databaseBackwards(t, db, app, operation, newState, projectState)
		djangotest.AssertIndexNameNotExists(t, db, tableName, constraintName)
		// Constraint doesn't work.
		insertPony(t, db, app, 3, -4)
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_remove_func_unique_constraint
//
// The Go test squares the weight in place of Django's Abs.
func TestRemoveFuncUniqueConstraint(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		const app = "test_rmfuncuc"
		constraintName := app + "_pony_abs_uq"
		tableName := app + "_pony"
		projectState := setUpTestModel(t, db, app, testModelOptions{constraints: []orm.Constraint{
			orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("weight").Mul(orm.F("weight"))}, Name: constraintName},
		}})
		djangotest.AssertTableExists(t, db, tableName)
		djangotest.AssertIndexNameExists(t, db, tableName, constraintName)
		operation := migrations.RemoveConstraint{Model: "Pony", Name: constraintName}
		djangotest.AssertEqual(t, operation.Describe(), "Remove constraint test_rmfuncuc_pony_abs_uq from model pony")
		djangotest.AssertEqual(t, operation.NameFragment(), "remove_pony_test_rmfuncuc_pony_abs_uq")
		newState := stateForwards(t, app, projectState, operation)
		djangotest.AssertEqual(t, constraintCount(t, newState, app, "Pony"), 0)
		// Remove constraint.
		databaseForwards(t, db, app, operation, projectState, newState)
		djangotest.AssertIndexNameNotExists(t, db, tableName, constraintName)
		// Constraint doesn't work.
		insertPony(t, db, app, 1, 4)
		insertPony(t, db, app, 1, -4)
		mustExec(t, db, "DELETE FROM "+tableName+" WHERE weight = -4")
		// Reversal.
		databaseBackwards(t, db, app, operation, newState, projectState)
		djangotest.AssertIndexNameExists(t, db, tableName, constraintName)
		rejectPony(t, db, app, 3, -4)
	})
}

// No Django test exists: AddConstraint refuses a second constraint of the
// same name, and RemoveConstraint and AlterConstraint a missing one.
func TestConstraintOperationsCheckNames(t *testing.T) {
	state := migrations.NewProjectState()
	state.AddModel(&migrations.ModelState{
		App: "app", Name: "Pony", Table: "app_pony", Fields: []migrations.FieldState{idField, pinkField},
		Constraints: []orm.Constraint{orm.CheckConstraint{Name: "pink_gt_0", Condition: orm.Q("pink__gt", 0)}},
	})
	for _, tt := range []struct {
		op   migrations.Operation
		want string
	}{
		{
			migrations.AddConstraint{Model: "Pony", Constraint: orm.CheckConstraint{Name: "pink_gt_0", Condition: orm.Q("pink__gt", 1)}},
			`app.Pony already has a constraint named "pink_gt_0"`,
		},
		{migrations.RemoveConstraint{Model: "Pony", Name: "missing"}, `app.Pony has no constraint named "missing"`},
		{
			migrations.AlterConstraint{Model: "Pony", Name: "missing", Constraint: orm.CheckConstraint{Name: "missing"}},
			`app.Pony has no constraint named "missing"`,
		},
	} {
		err := tt.op.StateForwards("app", state.Clone())
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%T: got error %v, want %q", tt.op, err, tt.want)
		}
	}
}
