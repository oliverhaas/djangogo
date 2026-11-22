package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file port Django's OptimizerTests for indexes and
// constraints. Django compares the serialized operations; the Go tests
// compare the operation values.

// assertOptimizesTo asserts that the optimizer turns operations into
// expected, in the app "migrations" as in Django's OptimizerTestBase.
func assertOptimizesTo(t *testing.T, operations, expected []migrations.Operation) {
	t.Helper()
	got := migrations.Optimize(operations, "migrations")
	if len(got) == 0 && len(expected) == 0 {
		return
	}
	djangotest.AssertEqual(t, got, expected)
}

// assertDoesNotOptimize asserts that the optimizer leaves operations as they
// are.
func assertDoesNotOptimize(t *testing.T, operations []migrations.Operation) {
	t.Helper()
	assertOptimizesTo(t, operations, operations)
}

// optimizerWeight and optimizerAge are the fields of the Pony models of the
// index and constraint tests.
var (
	optimizerWeight = intField("Weight", "weight")
	optimizerAge    = intField("Age", "age")
)

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_add_remove_index
func TestOptimizerAddRemoveIndex(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AddIndex{Model: "Pony", Index: migrations.Index{Fields: []string{"Weight", "Pink"}, Name: "idx_pony_weight_pink"}},
		migrations.RemoveIndex{Model: "Pony", Name: "idx_pony_weight_pink"},
	}, nil)
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_add_remove_constraint
func TestOptimizerAddRemoveConstraint(t *testing.T) {
	gtConstraint := orm.CheckConstraint{Condition: orm.Q("pink__gt", 2), Name: "constraint_pony_pink_gt_2"}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AddConstraint{Model: "Pony", Constraint: gtConstraint},
		migrations.RemoveConstraint{Model: "Pony", Name: gtConstraint.Name},
	}, nil)
	assertDoesNotOptimize(t, []migrations.Operation{
		migrations.AddConstraint{Model: "Pony", Constraint: gtConstraint},
		migrations.RemoveConstraint{Model: "Pony", Name: "other_name"},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_multiple_alter_constraints
func TestOptimizerMultipleAlterConstraints(t *testing.T) {
	gtConstraintViolationMsgAdded := orm.CheckConstraint{
		Condition: orm.Q("pink__gt", 2), Name: "pink_gt_2", ViolationErrorMessage: "ERROR",
	}
	gtConstraintViolationMsgAltered := orm.CheckConstraint{
		Condition: orm.Q("pink__gt", 2), Name: "pink_gt_2", ViolationErrorMessage: "error",
	}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AlterConstraint{Model: "Pony", Name: "pink_gt_2", Constraint: gtConstraintViolationMsgAdded},
		migrations.AlterConstraint{Model: "Pony", Name: "pink_gt_2", Constraint: gtConstraintViolationMsgAltered},
	}, []migrations.Operation{
		migrations.AlterConstraint{Model: "Pony", Name: "pink_gt_2", Constraint: gtConstraintViolationMsgAltered},
	})
	otherConstraintViolationMsg := orm.CheckConstraint{
		Condition: orm.Q("weight__gt", 3), Name: "pink_gt_3", ViolationErrorMessage: "error",
	}
	assertDoesNotOptimize(t, []migrations.Operation{
		migrations.AlterConstraint{Model: "Pony", Name: "pink_gt_2", Constraint: gtConstraintViolationMsgAdded},
		migrations.AlterConstraint{Model: "Pony", Name: "pink_gt_3", Constraint: otherConstraintViolationMsg},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_alter_remove_constraint
func TestOptimizerAlterRemoveConstraint(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AlterConstraint{
			Model: "Pony", Name: "pink_gt_2",
			Constraint: orm.CheckConstraint{Condition: orm.Q("pink__gt", 2), Name: "pink_gt_2"},
		},
		migrations.RemoveConstraint{Model: "Pony", Name: "pink_gt_2"},
	}, []migrations.Operation{migrations.RemoveConstraint{Model: "Pony", Name: "pink_gt_2"}})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_add_alter_constraint
func TestOptimizerAddAlterConstraint(t *testing.T) {
	constraint := orm.CheckConstraint{Condition: orm.Q("pink__gt", 2), Name: "pink_gt_2"}
	constraintWithError := orm.CheckConstraint{
		Condition: orm.Q("pink__gt", 2), Name: "pink_gt_2", ViolationErrorMessage: "error",
	}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AddConstraint{Model: "Pony", Constraint: constraint},
		migrations.AlterConstraint{Model: "Pony", Name: "pink_gt_2", Constraint: constraintWithError},
	}, []migrations.Operation{migrations.AddConstraint{Model: "Pony", Constraint: constraintWithError}})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_add_index
func TestOptimizerCreateModelAddIndex(t *testing.T) {
	ageIndex := migrations.Index{Fields: []string{"Age"}, Name: "idx_pony_age"}
	weightIndex := migrations.Index{Fields: []string{"Weight"}, Name: "idx_pony_weight"}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight, optimizerAge},
			Indexes: []migrations.Index{ageIndex},
		},
		migrations.AddIndex{Model: "Pony", Index: weightIndex},
	}, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight, optimizerAge},
			Indexes: []migrations.Index{ageIndex, weightIndex},
		},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_remove_index
func TestOptimizerCreateModelRemoveIndex(t *testing.T) {
	ageIndex := migrations.Index{Fields: []string{"Age"}, Name: "idx_pony_age"}
	weightIndex := migrations.Index{Fields: []string{"Weight"}, Name: "idx_pony_weight"}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight, optimizerAge},
			Indexes: []migrations.Index{ageIndex, weightIndex},
		},
		migrations.RemoveIndex{Model: "Pony", Name: "idx_pony_age"},
	}, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight, optimizerAge},
			Indexes: []migrations.Index{weightIndex},
		},
	})
}

// No Go counterpart: OptimizerTests.test_rename_index, test_add_rename_index
// and test_create_model_rename_index_no_old_fields test RenameIndex, which
// the Go migrations do not have.

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_add_constraint
func TestOptimizerCreateModelAddConstraint(t *testing.T) {
	gtConstraint := orm.CheckConstraint{Condition: orm.Q("weight__gt", 0), Name: "pony_weight_gt_0"}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Pony", Fields: []migrations.FieldState{optimizerWeight}},
		migrations.AddConstraint{Model: "Pony", Constraint: gtConstraint},
	}, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight},
			Constraints: []orm.Constraint{gtConstraint},
		},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_alter_constraint
func TestOptimizerCreateModelAlterConstraint(t *testing.T) {
	originalConstraint := orm.CheckConstraint{Condition: orm.Q("weight__gt", 0), Name: "pony_weight_gt_0"}
	alteredConstraint := orm.CheckConstraint{
		Condition: orm.Q("weight__gt", 0), Name: "pony_weight_gt_0", ViolationErrorMessage: "incorrect weight",
	}
	unique := orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("weight")}, Name: "pony_weight_unique"}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight},
			Constraints: []orm.Constraint{originalConstraint, unique},
		},
		migrations.AlterConstraint{Model: "Pony", Name: "pony_weight_gt_0", Constraint: alteredConstraint},
	}, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight},
			Constraints: []orm.Constraint{unique, alteredConstraint},
		},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_remove_constraint
func TestOptimizerCreateModelRemoveConstraint(t *testing.T) {
	unique := orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("weight")}, Name: "pony_weight_unique"}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight},
			Constraints: []orm.Constraint{
				orm.CheckConstraint{Condition: orm.Q("weight__gt", 0), Name: "pony_weight_gt_0"},
				unique,
			},
		},
		migrations.RemoveConstraint{Model: "Pony", Name: "pony_weight_gt_0"},
	}, []migrations.Operation{
		migrations.CreateModel{
			Name: "Pony", Fields: []migrations.FieldState{optimizerWeight},
			Constraints: []orm.Constraint{unique},
		},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_optimize_elidable_operation
func TestOptimizerElidableOperation(t *testing.T) {
	elidable := migrations.RunSQL{SQL: []string{"SELECT 1"}, Elidable: true}
	assertOptimizesTo(t,
		[]migrations.Operation{
			elidable,
			migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{charField("Name", "name", 255)}},
			elidable,
			migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{intField("Size", "size")}},
			elidable,
			migrations.RenameModel{OldName: "Foo", NewName: "Phou"},
			migrations.DeleteModel{Name: "Bar"},
			elidable,
		},
		[]migrations.Operation{
			migrations.CreateModel{Name: "Phou", Fields: []migrations.FieldState{charField("Name", "name", 255)}},
		},
	)
}
