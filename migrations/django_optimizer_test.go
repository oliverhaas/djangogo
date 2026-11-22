package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file port Django's OptimizerTests. Django compares the
// serialized operations; the Go tests compare the operation values. Go's
// CreateModel has no options besides the table, no bases and no managers, so
// the ported tests leave them out.

// assertOptimizesTo asserts that the optimizer turns operations into
// expected, in the app "migrations" as in Django's OptimizerTestBase.
func assertOptimizesTo(t *testing.T, operations, expected []migrations.Operation) {
	t.Helper()
	assertOptimizesToIn(t, "migrations", operations, expected)
}

// assertOptimizesToIn is assertOptimizesTo with Django's app_label argument.
func assertOptimizesToIn(t *testing.T, app string, operations, expected []migrations.Operation) {
	t.Helper()
	got := migrations.Optimize(operations, app)
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

// The fields of the models in the optimizer tests.
var (
	optimizerWeight = intField("Weight", "weight")
	optimizerAge    = intField("Age", "age")
	optimizerName   = charField("Name", "name", 255)
	optimizerURL    = migrations.FieldState{Name: "URL", Column: "url", Kind: orm.KindText}
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

// No Go counterpart: OptimizerTests.test_none_app_label passes None as the
// app label. Optimize takes the app as a string.

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_single
//
// Django also asserts that the optimizer needs one pass. Optimize does not
// report its passes.
func TestOptimizerSingle(t *testing.T) {
	assertOptimizesTo(t,
		[]migrations.Operation{migrations.DeleteModel{Name: "Foo"}},
		[]migrations.Operation{migrations.DeleteModel{Name: "Foo"}},
	)
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_delete_model
func TestOptimizerCreateDeleteModel(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.DeleteModel{Name: "Foo"},
	}, nil)
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_rename_model
func TestOptimizerCreateRenameModel(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.RenameModel{OldName: "Foo", NewName: "Bar"},
	}, []migrations.Operation{
		migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{optimizerName}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_rename_model_self
func TestOptimizerRenameModelSelf(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.RenameModel{OldName: "Foo", NewName: "Baa"},
		migrations.RenameModel{OldName: "Baa", NewName: "Bar"},
	}, []migrations.Operation{
		migrations.RenameModel{OldName: "Foo", NewName: "Bar"},
	})
}

// No Go counterpart: OptimizerTests.test_create_alter_model_options,
// test_create_alter_model_managers, test_create_alter_model_table_comment
// and test_create_model_and_remove_model_options test AlterModelOptions,
// AlterModelManagers and AlterModelTableComment. The Go migrations record no
// model options besides the table, no managers and no table comments.

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_alter_model_table
func TestOptimizerCreateAlterModelTable(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo"},
		migrations.AlterModelTable{Name: "Foo", Table: "foo"},
	}, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Table: "foo"},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_alter_unique_delete_model
func TestOptimizerCreateAlterUniqueDeleteModel(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.AlterModelTable{Name: "Foo", Table: "woohoo"},
		migrations.AlterUniqueTogether{Name: "Foo", UniqueTogether: [][]string{{"A", "B"}}},
		migrations.DeleteModel{Name: "Foo"},
	}, nil)
}

// No Go counterpart: OptimizerTests.test_create_alter_index_delete_model,
// test_create_alter_owrt_delete_model, test_alter_alter_index_model,
// test_alter_alter_owrt_model, test_create_alter_index_field and
// test_create_alter_owrt_field test AlterIndexTogether and
// AlterOrderWithRespectTo, which the Go migrations do not have.

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_alter_alter_table_model
func TestOptimizerAlterAlterTableModel(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AlterModelTable{Name: "Foo", Table: "a"},
		migrations.AlterModelTable{Name: "Foo", Table: "b"},
	}, []migrations.Operation{
		migrations.AlterModelTable{Name: "Foo", Table: "b"},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_alter_alter_unique_model
func TestOptimizerAlterAlterUniqueModel(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AlterUniqueTogether{Name: "Foo", UniqueTogether: [][]string{{"A", "B"}}},
		migrations.AlterUniqueTogether{Name: "Foo", UniqueTogether: [][]string{{"A", "C"}}},
	}, []migrations.Operation{
		migrations.AlterUniqueTogether{Name: "Foo", UniqueTogether: [][]string{{"A", "C"}}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_alter_alter_field
//
// The Go field state has no help_text, so the second AlterField makes the
// field nullable instead.
func TestOptimizerAlterAlterField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AlterField{Model: "Foo", Field: intField("Name", "name")},
		migrations.AlterField{Model: "Foo", Field: nullable(intField("Name", "name"))},
	}, []migrations.Operation{
		migrations.AlterField{Model: "Foo", Field: nullable(intField("Name", "name"))},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_optimize_through_create
//
// The cases with bases have no Go counterpart: Go models have no bases.
func TestOptimizerOptimizeThroughCreate(t *testing.T) {
	foo := migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}}
	bar := migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{intField("Size", "size")}}
	deleteFoo := migrations.DeleteModel{Name: "Foo"}
	deleteBar := migrations.DeleteModel{Name: "Bar"}
	assertOptimizesTo(t, []migrations.Operation{foo, bar, deleteFoo}, []migrations.Operation{bar})
	assertOptimizesTo(t, []migrations.Operation{foo, bar, deleteBar, deleteFoo}, nil)
	assertOptimizesTo(t, []migrations.Operation{foo, bar, deleteFoo, deleteBar}, nil)
	// Optimize the operations when the foreign key points to a model of
	// another app.
	barToTestapp := migrations.CreateModel{
		Name: "Bar", Fields: []migrations.FieldState{fkField("Other", "other_id", "testapp.Foo")},
	}
	assertOptimizesToIn(t, "otherapp",
		[]migrations.Operation{foo, barToTestapp, deleteFoo},
		[]migrations.Operation{barToTestapp},
	)
	// Keep them when the foreign key points to a model of the same app.
	assertDoesNotOptimize(t, []migrations.Operation{
		foo,
		migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{fkField("Other", "other_id", "migrations.Foo")}},
		deleteFoo,
	})
	sameApp := []migrations.Operation{foo, barToTestapp, deleteFoo}
	assertOptimizesToIn(t, "testapp", sameApp, sameApp)

	bookFK := fkField("Book", "book_id", "test_app.Book")
	reviewerFK := fkField("Reviewer", "reviewer_id", "test_app.Reviewer")
	book := migrations.CreateModel{Name: "Book", Fields: []migrations.FieldState{optimizerName}}
	reviewer := migrations.CreateModel{Name: "Reviewer", Fields: []migrations.FieldState{optimizerName}}
	assertOptimizesToIn(t, "test_app", []migrations.Operation{
		book,
		migrations.CreateModel{Name: "Person", Fields: []migrations.FieldState{optimizerName}},
		migrations.AddField{Model: "Book", Field: fkField("Author", "author_id", "test_app.Person")},
		migrations.CreateModel{Name: "Review", Fields: []migrations.FieldState{bookFK}},
		reviewer,
		migrations.AddField{Model: "Review", Field: reviewerFK},
		migrations.RemoveField{Model: "Book", Field: "Author"},
		migrations.DeleteModel{Name: "Person"},
	}, []migrations.Operation{
		book,
		reviewer,
		migrations.CreateModel{Name: "Review", Fields: []migrations.FieldState{bookFK, reviewerFK}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_add_field
func TestOptimizerCreateModelAddField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.AddField{Model: "Foo", Field: optimizerAge},
	}, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName, optimizerAge}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_reordering
func TestOptimizerCreateModelReordering(t *testing.T) {
	link := migrations.CreateModel{Name: "Link", Fields: []migrations.FieldState{optimizerURL}}
	linkFK := fkField("Link", "link_id", "migrations.Link")
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		link,
		migrations.AddField{Model: "Foo", Field: linkFK},
	}, []migrations.Operation{
		link,
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName, linkFK}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_reordering_circular_fk
func TestOptimizerCreateModelReorderingCircularFK(t *testing.T) {
	foo := migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}}
	fooFK := fkField("FooFK", "foo_fk_id", "migrations.Foo")
	addBarFK := migrations.AddField{Model: "Foo", Field: fkField("BarFK", "bar_fk_id", "migrations.Bar")}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{optimizerURL}},
		foo,
		migrations.AddField{Model: "Bar", Field: fooFK},
		addBarFK,
	}, []migrations.Operation{
		foo,
		migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{optimizerURL, fooFK}},
		addBarFK,
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_no_reordering_for_unrelated_fk
func TestOptimizerCreateModelNoReorderingForUnrelatedFK(t *testing.T) {
	assertDoesNotOptimize(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.CreateModel{Name: "Link", Fields: []migrations.FieldState{optimizerURL}},
		migrations.AddField{Model: "Other", Field: fkField("Link", "link_id", "migrations.Link")},
	})
}

// No Go counterpart: OptimizerTests.test_create_model_no_reordering_of_inherited_model
// tests a CreateModel with bases. Go models have no bases.

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_add_field_not_through_m2m_through
func TestOptimizerCreateModelAddFieldNotThroughM2MThrough(t *testing.T) {
	assertDoesNotOptimize(t, []migrations.Operation{
		migrations.CreateModel{Name: "Employee"},
		migrations.CreateModel{Name: "Employer"},
		migrations.CreateModel{Name: "Employment", Fields: []migrations.FieldState{
			fkField("Employee", "employee_id", "migrations.Employee"),
			fkField("Employment", "employment_id", "migrations.Employer"),
		}},
		migrations.AddField{
			Model: "Employer",
			Field: m2mThroughField("Employees", "migrations.Employee", "migrations.Employment"),
		},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_alter_field
func TestOptimizerCreateModelAlterField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.AlterField{Model: "Foo", Field: intField("Name", "name")},
	}, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{intField("Name", "name")}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_rename_field
func TestOptimizerCreateModelRenameField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.RenameField{Model: "Foo", OldName: "Name", NewName: "Title"},
	}, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{charField("Title", "title", 255)}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_add_field_rename_field
func TestOptimizerAddFieldRenameField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AddField{Model: "Foo", Field: optimizerName},
		migrations.RenameField{Model: "Foo", OldName: "Name", NewName: "Title"},
	}, []migrations.Operation{
		migrations.AddField{Model: "Foo", Field: charField("Title", "title", 255)},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_alter_field_rename_field
func TestOptimizerAlterFieldRenameField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AlterField{Model: "Foo", Field: optimizerName},
		migrations.RenameField{Model: "Foo", OldName: "Name", NewName: "Title"},
		migrations.RenameField{Model: "Foo", OldName: "Title", NewName: "Nom"},
	}, []migrations.Operation{
		migrations.RenameField{Model: "Foo", OldName: "Name", NewName: "Nom"},
		migrations.AlterField{Model: "Foo", Field: charField("Nom", "nom", 255)},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_swapping_fields_names
//
// RunGo{} stands in for RunPython(RunPython.noop): Go func values never
// compare equal, so a RunGo with Noop would fail the comparison.
func TestOptimizerSwappingFieldsNames(t *testing.T) {
	assertDoesNotOptimize(t, []migrations.Operation{
		migrations.CreateModel{Name: "MyModel", Fields: []migrations.FieldState{
			intField("FieldA", "field_a"), intField("FieldB", "field_b"),
		}},
		migrations.RunGo{},
		migrations.RenameField{Model: "MyModel", OldName: "FieldA", NewName: "FieldC"},
		migrations.RenameField{Model: "MyModel", OldName: "FieldB", NewName: "FieldA"},
		migrations.RenameField{Model: "MyModel", OldName: "FieldC", NewName: "FieldB"},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_model_remove_field
func TestOptimizerCreateModelRemoveField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName, optimizerAge}},
		migrations.RemoveField{Model: "Foo", Field: "Age"},
	}, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_add_field_alter_field
func TestOptimizerAddFieldAlterField(t *testing.T) {
	floatAge := migrations.FieldState{Name: "Age", Column: "age", Kind: orm.KindFloat, Default: 2.4}
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AddField{Model: "Foo", Field: optimizerAge},
		migrations.AlterField{Model: "Foo", Field: floatAge},
	}, []migrations.Operation{
		migrations.AddField{Model: "Foo", Field: floatAge},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_add_field_delete_field
func TestOptimizerAddFieldDeleteField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AddField{Model: "Foo", Field: optimizerAge},
		migrations.RemoveField{Model: "Foo", Field: "Age"},
	}, nil)
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_alter_field_delete_field
func TestOptimizerAlterFieldDeleteField(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.AlterField{Model: "Foo", Field: optimizerAge},
		migrations.RemoveField{Model: "Foo", Field: "Age"},
	}, []migrations.Operation{
		migrations.RemoveField{Model: "Foo", Field: "Age"},
	})
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_create_alter_unique_field
//
// The test inlines Django's _test_create_alter_foo_field for
// AlterUniqueTogether.
func TestOptimizerCreateAlterUniqueField(t *testing.T) {
	a, b, c := intField("A", "a"), intField("B", "b"), intField("C", "c")
	options := [][]string{{"A", "B"}}
	alter := migrations.AlterUniqueTogether{Name: "Foo", UniqueTogether: options}
	foo := func(uniqueTogether [][]string, fields ...migrations.FieldState) migrations.CreateModel {
		return migrations.CreateModel{Name: "Foo", Fields: fields, UniqueTogether: uniqueTogether}
	}
	// AddField
	assertOptimizesTo(t,
		[]migrations.Operation{foo(nil, a, b), alter, migrations.AddField{Model: "Foo", Field: c}},
		[]migrations.Operation{foo(options, a, b, c)},
	)
	// AlterField
	assertOptimizesTo(t,
		[]migrations.Operation{foo(nil, a, b), alter, migrations.AlterField{Model: "Foo", Field: charField("B", "b", 255)}},
		[]migrations.Operation{foo(options, a, charField("B", "b", 255))},
	)
	assertOptimizesTo(t,
		[]migrations.Operation{foo(nil, a, b, c), alter, migrations.AlterField{Model: "Foo", Field: charField("C", "c", 255)}},
		[]migrations.Operation{foo(options, a, b, charField("C", "c", 255))},
	)
	// RenameField
	renamedOptions := [][]string{{"A", "C"}}
	assertOptimizesTo(t,
		[]migrations.Operation{foo(nil, a, b), alter, migrations.RenameField{Model: "Foo", OldName: "B", NewName: "C"}},
		[]migrations.Operation{foo(renamedOptions, a, c)},
	)
	assertOptimizesTo(t,
		[]migrations.Operation{
			foo(nil, a, b), alter,
			migrations.RenameField{Model: "Foo", OldName: "B", NewName: "X"},
			migrations.RenameField{Model: "Foo", OldName: "X", NewName: "C"},
		},
		[]migrations.Operation{foo(renamedOptions, a, c)},
	)
	assertOptimizesTo(t,
		[]migrations.Operation{foo(nil, a, b, c), alter, migrations.RenameField{Model: "Foo", OldName: "C", NewName: "D"}},
		[]migrations.Operation{foo(options, a, b, intField("D", "d"))},
	)
	// RemoveField
	assertOptimizesTo(t,
		[]migrations.Operation{foo(nil, a, b), alter, migrations.RemoveField{Model: "Foo", Field: "B"}},
		[]migrations.Operation{foo([][]string{{"A"}}, a)},
	)
	assertOptimizesTo(t,
		[]migrations.Operation{foo(nil, a, b, c), alter, migrations.RemoveField{Model: "Foo", Field: "C"}},
		[]migrations.Operation{foo(options, a, b)},
	)
}

// Ported from Django: tests/migrations/test_optimizer.py::OptimizerTests.test_optimize_through_fields
func TestOptimizerOptimizeThroughFields(t *testing.T) {
	assertOptimizesTo(t, []migrations.Operation{
		migrations.CreateModel{Name: "Foo", Fields: []migrations.FieldState{optimizerName}},
		migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{intField("Size", "size")}},
		migrations.AddField{Model: "Foo", Field: optimizerAge},
		migrations.AddField{Model: "Bar", Field: intField("Width", "width")},
		migrations.AlterField{Model: "Foo", Field: optimizerAge},
		migrations.RenameField{Model: "Bar", OldName: "Size", NewName: "Dimensions"},
		migrations.RemoveField{Model: "Foo", Field: "Age"},
		migrations.RenameModel{OldName: "Foo", NewName: "Phou"},
		migrations.RemoveField{Model: "Bar", Field: "Dimensions"},
		migrations.RenameModel{OldName: "Phou", NewName: "Fou"},
		migrations.DeleteModel{Name: "Fou"},
	}, []migrations.Operation{
		migrations.CreateModel{Name: "Bar", Fields: []migrations.FieldState{intField("Width", "width")}},
	})
}
