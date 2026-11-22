package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// withConstraints returns ms with check and unique constraints, Django's
// constraints option.
func withConstraints(ms *migrations.ModelState, constraints ...orm.Constraint) *migrations.ModelState {
	return variant(ms, func(m *migrations.ModelState) { m.Constraints = constraints })
}

// nameContainsBob is the constraint of Django's author_name_check_constraint
// state.
var nameContainsBob = orm.CheckConstraint{Condition: orm.Q("name__contains", "Bob"), Name: "name_contains_bob"}

// authorNameCheckConstraint is Django's author_name_check_constraint state.
var authorNameCheckConstraint = withConstraints(authorName, nameContainsBob)

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_create_model_with_check_constraint
func TestCreateModelWithCheckConstraint(t *testing.T) {
	author := withConstraints(model("otherapp", "Author", idField, nameField), nameContainsBob)
	changes := getChanges(t, nil, models{author}, nil)
	constraint := orm.CheckConstraint{Condition: orm.Q("name__contains", "Bob"), Name: "name_contains_bob"}
	// Right number of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	// Right number of actions?
	djangotest.AssertEqual(t, len(migrationAt(t, changes, "otherapp", 0).Operations), 1)
	// Right actions order?
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel")
	created := operationAt[migrations.CreateModel](t, changes, "otherapp", 0, 0)
	djangotest.AssertEqual(t, created.Name, "Author")
	djangotest.AssertEqual(t, created.Constraints, []orm.Constraint{constraint})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_constraints
func TestAddConstraints(t *testing.T) {
	changes := getChanges(t, models{authorName}, models{authorNameCheckConstraint}, nil)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddConstraint")
	addedConstraint := orm.CheckConstraint{Condition: orm.Q("name__contains", "Bob"), Name: "name_contains_bob"}
	djangotest.AssertEqual(t, operationAt[migrations.AddConstraint](t, changes, "testapp", 0, 0),
		migrations.AddConstraint{Model: "Author", Constraint: addedConstraint})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_constraints_with_new_model
func TestAddConstraintsWithNewModel(t *testing.T) {
	bookWithUniqueTitleAndPony := withConstraints(
		model("otherapp", "Book", idField, titleField, fkField("Pony", "pony_id", "otherapp.Pony")),
		orm.UniqueConstraint{Fields: []string{"Title", "Pony"}, Name: "unique_title_pony"})
	changes := getChanges(t, models{bookWithNoAuthor}, models{bookWithUniqueTitleAndPony, otherPony}, nil)
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel", "AddField", "AddConstraint")
}

// Django builds one condition from dict keys and the other from a tuple of
// the same keys; the two deconstruct differently but compare equal. The Go
// counterpart builds the in-list once from a []string and once from a []any,
// which write the same source.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_constraints_with_dict_keys
func TestAddConstraintsWithDictKeys(t *testing.T) {
	book := model("testapp", "Book", idField, charField("Type", "type", 1))
	bookWithType := withConstraints(book,
		orm.CheckConstraint{Condition: orm.Q("type__in", []string{"F", "M"}), Name: "book_type_check"})
	bookWithResolvedType := withConstraints(book,
		orm.CheckConstraint{Condition: orm.Q("type__in", []any{"F", "M"}), Name: "book_type_check"})
	changes := getChanges(t, models{bookWithType}, models{bookWithResolvedType}, nil)
	djangotest.AssertEqual(t, len(changes), 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_constraint
func TestAutodetectorAlterConstraint(t *testing.T) {
	bookConstraint := orm.CheckConstraint{Condition: orm.Q("title__contains", "title"), Name: "title_contains_title"}
	bookAlteredConstraint := orm.CheckConstraint{
		Condition:          orm.Q("title__contains", "title"),
		Name:               "title_contains_title",
		ViolationErrorCode: "error_code",
	}
	authorAlteredConstraint := orm.CheckConstraint{
		Condition:             orm.Q("name__contains", "Bob"),
		Name:                  "name_contains_bob",
		ViolationErrorMessage: "Name doesn't contain Bob",
	}
	bookCheckConstraint := withConstraints(book, bookConstraint)
	bookCheckConstraintWithErrorMessage := withConstraints(book, bookAlteredConstraint)
	authorNameCheckConstraintWithErrorMessage := withConstraints(authorName, authorAlteredConstraint)

	changes := getChanges(t,
		models{authorNameCheckConstraint, bookCheckConstraint},
		models{authorNameCheckConstraintWithErrorMessage, bookCheckConstraintWithErrorMessage}, nil)

	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterConstraint")
	djangotest.AssertEqual(t, operationAt[migrations.AlterConstraint](t, changes, "testapp", 0, 0),
		migrations.AlterConstraint{Model: "Author", Name: "name_contains_bob", Constraint: authorAlteredConstraint})

	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterConstraint")
	djangotest.AssertEqual(t, operationAt[migrations.AlterConstraint](t, changes, "otherapp", 0, 0),
		migrations.AlterConstraint{Model: "Book", Name: "title_contains_title", Constraint: bookAlteredConstraint})
	assertMigrationDependencies(t, changes, "otherapp", 0, k("testapp", "auto_1"))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_constraints
func TestRemoveConstraints(t *testing.T) {
	changes := getChanges(t, models{authorNameCheckConstraint}, models{authorName}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveConstraint")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveConstraint](t, changes, "testapp", 0, 0),
		migrations.RemoveConstraint{Model: "Author", Name: "name_contains_bob"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_constraint_dropped_and_recreated
func TestConstraintDroppedAndRecreated(t *testing.T) {
	alteredConstraint := orm.CheckConstraint{Condition: orm.Q("name__contains", "bob"), Name: "name_contains_bob"}
	authorNameCheckConstraintLowercased := withConstraints(authorName, alteredConstraint)
	changes := getChanges(t, models{authorNameCheckConstraint}, models{authorNameCheckConstraintLowercased}, nil)

	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveConstraint", "AddConstraint")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveConstraint](t, changes, "testapp", 0, 0),
		migrations.RemoveConstraint{Model: "Author", Name: "name_contains_bob"})
	djangotest.AssertEqual(t, operationAt[migrations.AddConstraint](t, changes, "testapp", 0, 1),
		migrations.AddConstraint{Model: "Author", Constraint: alteredConstraint})
}

// Renaming a field that already had a db_column attribute and a constraint
// generates two no-op operations: RenameField and AlterConstraint. In Go the
// RenameField keeps the column.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_field_preserve_db_column_preserve_constraint
func TestRenameFieldPreserveDBColumnPreserveConstraint(t *testing.T) {
	before := models{withConstraints(
		model("app", "Foo", idField, intField("Field", "full_field1_name"), intField("Field2", "field2")),
		orm.UniqueConstraint{Fields: []string{"Field", "Field2"}, Name: "unique_field"})}
	afterConstraint := orm.UniqueConstraint{Fields: []string{"FullField1Name", "Field2"}, Name: "unique_field"}
	after := models{withConstraints(
		model("app", "Foo", idField, intField("FullField1Name", "full_field1_name"), intField("Field2", "field2")),
		afterConstraint)}
	changes := getChanges(t, before, after, migrations.FixedQuestioner{Rename: true})
	assertNumberMigrations(t, changes, "app", 1)
	assertOperationTypes(t, changes, "app", 0, "RenameField", "AlterConstraint")
	djangotest.AssertEqual(t, operationAt[migrations.AlterConstraint](t, changes, "app", 0, 1),
		migrations.AlterConstraint{Model: "Foo", Name: "unique_field", Constraint: afterConstraint})
}

// Renaming a field without given db_column recreates a constraint.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_field_without_db_column_recreate_constraint
func TestRenameFieldWithoutDBColumnRecreateConstraint(t *testing.T) {
	before := models{withConstraints(model("app", "Foo", idField, intField("Field", "field")),
		orm.UniqueConstraint{Fields: []string{"Field"}, Name: "unique_field"})}
	after := models{withConstraints(model("app", "Foo", idField, intField("FullField1Name", "full_field1_name")),
		orm.UniqueConstraint{Fields: []string{"FullField1Name"}, Name: "unique_field"})}
	changes := getChanges(t, before, after, migrations.FixedQuestioner{Rename: true})
	assertNumberMigrations(t, changes, "app", 1)
	assertOperationTypes(t, changes, "app", 0, "RemoveConstraint", "RenameField", "AddConstraint")
}

// Removing a field from the constraint triggers recreation.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_field_preserve_db_column_recreate_constraint
func TestRenameFieldPreserveDBColumnRecreateConstraint(t *testing.T) {
	before := models{withConstraints(
		model("app", "Foo", idField, intField("Field1", "field1"), intField("Field2", "field2")),
		orm.UniqueConstraint{Fields: []string{"Field1", "Field2"}, Name: "unique_fields"})}
	after := models{withConstraints(
		model("app", "Foo", idField, intField("RenamedField1", "field1"), intField("RenamedField2", "field2")),
		orm.UniqueConstraint{Fields: []string{"RenamedField1"}, Name: "unique_fields"})}
	changes := getChanges(t, before, after, migrations.FixedQuestioner{Rename: true})
	assertNumberMigrations(t, changes, "app", 1)
	assertOperationTypes(t, changes, "app", 0, "RemoveConstraint", "RenameField", "RenameField", "AddConstraint")
}
