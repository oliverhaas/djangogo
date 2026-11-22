package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_db_table_add
func TestAlterDBTableAdd(t *testing.T) {
	changes := getChanges(t, models{authorEmpty}, models{authorWithDBTableOptions}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterModelTable")
	djangotest.AssertEqual(t, operationAt[migrations.AlterModelTable](t, changes, "testapp", 0, 0),
		migrations.AlterModelTable{Name: "Author", Table: "author_one"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_db_table_change
func TestAlterDBTableChange(t *testing.T) {
	changes := getChanges(t, models{authorWithDBTableOptions}, models{authorWithNewDBTableOptions}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterModelTable")
	djangotest.AssertEqual(t, operationAt[migrations.AlterModelTable](t, changes, "testapp", 0, 0),
		migrations.AlterModelTable{Name: "Author", Table: "author_two"})
}

// Django's table=None is the empty table name in Go.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_db_table_remove
func TestAlterDBTableRemove(t *testing.T) {
	changes := getChanges(t, models{authorWithDBTableOptions}, models{authorEmpty}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterModelTable")
	djangotest.AssertEqual(t, operationAt[migrations.AlterModelTable](t, changes, "testapp", 0, 0),
		migrations.AlterModelTable{Name: "Author", Table: ""})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_db_table_no_changes
func TestAlterDBTableNoChanges(t *testing.T) {
	changes := getChanges(t, models{authorWithDBTableOptions}, models{authorWithDBTableOptions}, nil)
	// Right number of migrations?
	djangotest.AssertEqual(t, len(changes), 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_keep_db_table_with_model_change
func TestKeepDBTableWithModelChange(t *testing.T) {
	changes := getChanges(t,
		models{authorWithDBTableOptions}, models{authorRenamedWithDBTableOptions},
		migrations.FixedQuestioner{RenameModel: true})
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel")
	djangotest.AssertEqual(t, operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0),
		migrations.RenameModel{OldName: "Author", NewName: "NewAuthor"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_db_table_with_model_change
func TestAlterDBTableWithModelChange(t *testing.T) {
	changes := getChanges(t,
		models{authorWithDBTableOptions}, models{authorRenamedWithNewDBTableOptions},
		migrations.FixedQuestioner{RenameModel: true})
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel", "AlterModelTable")
	djangotest.AssertEqual(t, operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0),
		migrations.RenameModel{OldName: "Author", NewName: "NewAuthor"})
	djangotest.AssertEqual(t, operationAt[migrations.AlterModelTable](t, changes, "testapp", 0, 1),
		migrations.AlterModelTable{Name: "NewAuthor", Table: "author_three"})
}

// Django knows three spellings of "no unique_together": not specified, None
// and an empty set. Go has two, a nil and an empty slice.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_empty_unique_together
func TestEmptyUniqueTogether(t *testing.T) {
	notSpecified := model("a", "Model", idField)
	empty := withUniqueTogether(model("a", "Model", idField))
	empty.UniqueTogether = [][]string{}
	tests := []struct {
		name     string
		from, to *migrations.ModelState
	}{
		{`"not specified" to "not specified"`, notSpecified, notSpecified},
		{`"not specified" to "empty"`, notSpecified, empty},
		{`"empty" to "not specified"`, empty, notSpecified},
		{`"empty" to "empty"`, empty, empty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := getChanges(t, models{tt.from}, models{tt.to}, nil)
			if len(changes) > 0 {
				t.Errorf("created operations from %s:\n%s", tt.name, reprChanges(changes))
			}
		})
	}
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_create_model_with_indexes
func TestCreateModelWithIndexes(t *testing.T) {
	addedIndex := migrations.Index{Name: "create_model_with_indexes_idx", Fields: []string{"Name"}}
	author := withIndexes(model("otherapp", "Author", idField, nameField), addedIndex)
	changes := getChanges(t, nil, models{author}, nil)
	// Right number of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	// Right number of actions?
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel")
	// Right actions order?
	created := operationAt[migrations.CreateModel](t, changes, "otherapp", 0, 0)
	djangotest.AssertEqual(t, created.Name, "Author")
	djangotest.AssertEqual(t, created.Indexes, []migrations.Index{addedIndex})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_indexes
func TestAddIndexes(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, book}, models{authorEmpty, bookIndexes}, nil)
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AddIndex")
	djangotest.AssertEqual(t, operationAt[migrations.AddIndex](t, changes, "otherapp", 0, 0), migrations.AddIndex{
		Model: "Book", Index: migrations.Index{Name: "book_title_author_idx", Fields: []string{"Author", "Title"}},
	})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_indexes
func TestRemoveIndexes(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookIndexes}, models{authorEmpty, book}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "RemoveIndex")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveIndex](t, changes, "otherapp", 0, 0),
		migrations.RemoveIndex{Model: "Book", Name: "book_title_author_idx"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_field_with_model_options
func TestRemoveFieldWithModelOptions(t *testing.T) {
	dog := withIndexes(
		model("testapp", "Dog", idField, charField("Name", "name", 100), fkField("Animal", "animal_id", "testapp.Animal")),
		migrations.Index{Name: "animal_name_idx", Fields: []string{"Animal", "Name"}})
	before := models{
		model("testapp", "Animal", idField),
		withConstraints(dog, orm.UniqueConstraint{Fields: []string{"Animal", "Name"}, Name: "animal_name_idx"}),
	}
	changes := getChanges(t, before, nil, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0,
		"RemoveIndex", "RemoveConstraint", "RemoveField", "DeleteModel", "DeleteModel")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_field_with_remove_index_or_constraint_dependency
func TestRemoveFieldWithRemoveIndexOrConstraintDependency(t *testing.T) {
	date := autoNow(dateTimeField("Date", "date"))
	category := nullable(fkField("Category", "category_id", "testapp.Category"))
	category.RelOnDelete = orm.OnDeleteSetNull
	before := models{
		model("testapp", "Category", idField),
		withConstraints(model("testapp", "Model", idField, date, category),
			orm.UniqueConstraint{Fields: []string{"Date", "Category"}, Name: "unique_category_for_date"}),
	}
	changes := getChanges(t, before, models{model("testapp", "Model", idField, date)}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveConstraint", "RemoveField", "DeleteModel")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_order_fields_indexes
func TestOrderFieldsIndexes(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookIndexes}, models{authorEmpty, bookUnorderedIndexes}, nil)
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "RemoveIndex", "AddIndex")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveIndex](t, changes, "otherapp", 0, 0),
		migrations.RemoveIndex{Model: "Book", Name: "book_title_author_idx"})
	djangotest.AssertEqual(t, operationAt[migrations.AddIndex](t, changes, "otherapp", 0, 1), migrations.AddIndex{
		Model: "Book", Index: migrations.Index{Name: "book_author_title_idx", Fields: []string{"Title", "Author"}},
	})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_index_with_new_model
func TestAddIndexWithNewModel(t *testing.T) {
	bookWithIndexTitleAndPony := withIndexes(
		model("otherapp", "Book", idField, titleField, fkField("Pony", "pony_id", "otherapp.Pony")),
		migrations.Index{Name: "index_title_pony", Fields: []string{"Title", "Pony"}})
	changes := getChanges(t, models{bookWithNoAuthor}, models{bookWithIndexTitleAndPony, otherPony}, nil)
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel", "AddField", "AddIndex")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_unique_together
func TestAddUniqueTogether(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, book}, models{authorEmpty, bookUniqueTogether}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterUniqueTogether")
	djangotest.AssertEqual(t, operationAt[migrations.AlterUniqueTogether](t, changes, "otherapp", 0, 0),
		migrations.AlterUniqueTogether{Name: "Book", UniqueTogether: [][]string{{"Author", "Title"}}})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_unique_together
func TestRemoveUniqueTogether(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookUniqueTogether}, models{authorEmpty, book}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterUniqueTogether")
	op := operationAt[migrations.AlterUniqueTogether](t, changes, "otherapp", 0, 0)
	djangotest.AssertEqual(t, op.Name, "Book")
	djangotest.AssertEqual(t, len(op.UniqueTogether), 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_unique_together_remove_fk
func TestUniqueTogetherRemoveFK(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookUniqueTogether}, models{authorEmpty, bookWithNoAuthor}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterUniqueTogether", "RemoveField")
	op := operationAt[migrations.AlterUniqueTogether](t, changes, "otherapp", 0, 0)
	djangotest.AssertEqual(t, op.Name, "Book")
	djangotest.AssertEqual(t, len(op.UniqueTogether), 0)
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "otherapp", 0, 1),
		migrations.RemoveField{Model: "Book", Field: "Author"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_unique_together_no_changes
func TestUniqueTogetherNoChanges(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookUniqueTogether}, models{authorEmpty, bookUniqueTogether}, nil)
	// Right number of migrations?
	djangotest.AssertEqual(t, len(changes), 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_unique_together_ordering
func TestUniqueTogetherOrdering(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookUniqueTogether}, models{authorEmpty, bookUniqueTogether2}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterUniqueTogether")
	djangotest.AssertEqual(t, operationAt[migrations.AlterUniqueTogether](t, changes, "otherapp", 0, 0),
		migrations.AlterUniqueTogether{Name: "Book", UniqueTogether: [][]string{{"Title", "Author"}}})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_field_and_unique_together
func TestAddFieldAndUniqueTogether(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, book}, models{authorEmpty, bookUniqueTogether3}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AddField", "AlterUniqueTogether")
	djangotest.AssertEqual(t, operationAt[migrations.AlterUniqueTogether](t, changes, "otherapp", 0, 1),
		migrations.AlterUniqueTogether{Name: "Book", UniqueTogether: [][]string{{"Title", "Newfield"}}})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_create_model_and_unique_together
func TestCreateModelAndUniqueTogether(t *testing.T) {
	author := model("otherapp", "Author", idField, nameField)
	bookWithAuthor := withUniqueTogether(
		model("otherapp", "Book", idField, fkField("Author", "author_id", "otherapp.Author"), titleField),
		[]string{"Title", "Author"})
	changes := getChanges(t, models{bookWithNoAuthor}, models{author, bookWithAuthor}, nil)
	// Right number of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	// Right number of actions?
	// Right actions order?
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel", "AddField", "AlterUniqueTogether")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_field_and_unique_together
func TestRemoveFieldAndUniqueTogether(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookUniqueTogether3}, models{authorEmpty, bookUniqueTogether}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterUniqueTogether", "RemoveField")
	djangotest.AssertEqual(t, operationAt[migrations.AlterUniqueTogether](t, changes, "otherapp", 0, 0),
		migrations.AlterUniqueTogether{Name: "Book", UniqueTogether: [][]string{{"Author", "Title"}}})
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "otherapp", 0, 1),
		migrations.RemoveField{Model: "Book", Field: "Newfield"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_field_and_unique_together
func TestAlterFieldAndUniqueTogether(t *testing.T) {
	age := intField("Age", "age")
	indexedAge := age
	indexedAge.DBIndex = true
	uniqueName := nameField
	uniqueName.Unique = true
	initialAuthor := withUniqueTogether(model("testapp", "Author", idField, nameField, indexedAge), []string{"Name"})
	authorReversedConstraints := withUniqueTogether(model("testapp", "Author", idField, uniqueName, age), []string{"Age"})
	changes := getChanges(t, models{initialAuthor}, models{authorReversedConstraints}, nil)

	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0,
		"AlterUniqueTogether", "AlterField", "AlterField", "AlterUniqueTogether")
	first := operationAt[migrations.AlterUniqueTogether](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, first.Name, "Author")
	djangotest.AssertEqual(t, len(first.UniqueTogether), 0)
	djangotest.AssertEqual(t, operationAt[migrations.AlterField](t, changes, "testapp", 0, 1).Field.Name, "Age")
	djangotest.AssertEqual(t, operationAt[migrations.AlterField](t, changes, "testapp", 0, 2).Field.Name, "Name")
	djangotest.AssertEqual(t, operationAt[migrations.AlterUniqueTogether](t, changes, "testapp", 0, 3),
		migrations.AlterUniqueTogether{Name: "Author", UniqueTogether: [][]string{{"Age"}}})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_partly_alter_unique_together_increase
func TestPartlyAlterUniqueTogetherIncrease(t *testing.T) {
	author := model("testapp", "Author", idField, nameField, intField("Age", "age"))
	initialAuthor := withUniqueTogether(author, []string{"Name"})
	authorNewConstraints := withUniqueTogether(author, []string{"Name"}, []string{"Age"})
	changes := getChanges(t, models{initialAuthor}, models{authorNewConstraints}, nil)

	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterUniqueTogether")
	djangotest.AssertEqual(t, operationAt[migrations.AlterUniqueTogether](t, changes, "testapp", 0, 0),
		migrations.AlterUniqueTogether{Name: "Author", UniqueTogether: [][]string{{"Name"}, {"Age"}}})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_partly_alter_unique_together_decrease
func TestPartlyAlterUniqueTogetherDecrease(t *testing.T) {
	author := model("testapp", "Author", idField, nameField, intField("Age", "age"))
	initialAuthor := withUniqueTogether(author, []string{"Name"}, []string{"Age"})
	authorNewConstraints := withUniqueTogether(author, []string{"Name"})
	changes := getChanges(t, models{initialAuthor}, models{authorNewConstraints}, nil)

	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterUniqueTogether")
	djangotest.AssertEqual(t, operationAt[migrations.AlterUniqueTogether](t, changes, "testapp", 0, 0),
		migrations.AlterUniqueTogether{Name: "Author", UniqueTogether: [][]string{{"Name"}}})
}

// Django also generates an AlterUniqueTogether with the renamed field, which
// repeats what RenameField already does to the state. Go compares the sets
// after the rename and generates only the RenameField.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_field_and_unique_together
func TestRenameFieldAndUniqueTogether(t *testing.T) {
	changes := getChanges(t,
		models{authorEmpty, bookUniqueTogether3}, models{authorEmpty, bookUniqueTogether4},
		migrations.FixedQuestioner{Rename: true})
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "RenameField")
	djangotest.AssertEqual(t, operationAt[migrations.RenameField](t, changes, "otherapp", 0, 0),
		migrations.RenameField{Model: "Book", OldName: "Newfield", NewName: "Newfield2"})
}

// As in TestRenameFieldAndUniqueTogether, Go generates only the RenameField.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_does_not_crash_after_rename_on_unique_together
func TestDoesNotCrashAfterRenameOnUniqueTogether(t *testing.T) {
	before := withUniqueTogether(
		model("app", "Foo", idField, intField("First", "first"), intField("Second", "second")),
		[]string{"First", "Second"})
	after := withUniqueTogether(
		model("app", "Foo", idField, intField("FirstRenamed", "first_renamed"), intField("Second", "second")),
		[]string{"FirstRenamed", "Second"})
	changes := getChanges(t, models{before}, models{after}, migrations.FixedQuestioner{Rename: true})
	assertNumberMigrations(t, changes, "app", 1)
	assertOperationTypes(t, changes, "app", 0, "RenameField")
	djangotest.AssertEqual(t, operationAt[migrations.RenameField](t, changes, "app", 0, 0),
		migrations.RenameField{Model: "Foo", OldName: "First", NewName: "FirstRenamed"})
}
