package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
)

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_fk_dependency
func TestFKDependency(t *testing.T) {
	// Note that testapp (author) has no dependencies,
	// otherapp (book) depends on testapp (author),
	// thirdapp (edition) depends on otherapp (book).
	edition := model("thirdapp", "Edition", idField, fkField("Book", "book_id", "otherapp.Book"))
	changes := getChanges(t, nil, models{authorName, book, edition}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Author")
	assertMigrationDependencies(t, changes, "testapp", 0)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "otherapp", 0, 0).Name, "Book")
	assertMigrationDependencies(t, changes, "otherapp", 0, k("testapp", "auto_1"))
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "thirdapp", 1)
	assertOperationTypes(t, changes, "thirdapp", 0, "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "thirdapp", 0, 0).Name, "Edition")
	assertMigrationDependencies(t, changes, "thirdapp", 0, k("otherapp", "auto_1"))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_same_app_no_fk_dependency
func TestSameAppNoFKDependency(t *testing.T) {
	changes := getChanges(t, nil, models{authorWithPublisher, publisher}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel", "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Publisher")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 1).Name, "Author")
	assertMigrationDependencies(t, changes, "testapp", 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_circular_fk_dependency
func TestCircularFKDependency(t *testing.T) {
	changes := getChanges(t, nil, models{authorWithBook, book, publisherWithBook}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel", "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Author")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 1).Name, "Publisher")
	assertMigrationDependencies(t, changes, "testapp", 0, k("otherapp", "auto_1"))
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 2)
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel")
	assertOperationTypes(t, changes, "otherapp", 1, "AddField")
	assertMigrationDependencies(t, changes, "otherapp", 0)
	assertMigrationDependencies(t, changes, "otherapp", 1, k("otherapp", "auto_1"), k("testapp", "auto_1"))
	// both split migrations should be `initial`
	djangotest.AssertEqual(t, changes["otherapp"][0].Initial, true)
	djangotest.AssertEqual(t, changes["otherapp"][1].Initial, true)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_same_app_circular_fk_dependency
func TestSameAppCircularFKDependency(t *testing.T) {
	changes := getChanges(t, nil, models{authorWithPublisher, publisherWithAuthor}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel", "CreateModel", "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Author")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 1).Name, "Publisher")
	djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, 2).Field.Name, "Publisher")
	assertMigrationDependencies(t, changes, "testapp", 0)
}

// Django asserts that the first CreateModel, of Knight, has no
// unique_together. The Go port also checks where the options end up: on the
// CreateModel of Rabbit, after its self-referential foreign key.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_same_app_circular_fk_dependency_with_unique_together_and_indexes
func TestSameAppCircularFKDependencyWithUniqueTogetherAndIndexes(t *testing.T) {
	changes := getChanges(t, nil, models{knight, rabbit}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "eggs", 1)
	assertOperationTypes(t, changes, "eggs", 0, "CreateModel", "CreateModel")
	djangotest.AssertEqual(t, len(operationAt[migrations.CreateModel](t, changes, "eggs", 0, 0).UniqueTogether), 0)
	created := operationAt[migrations.CreateModel](t, changes, "eggs", 0, 1)
	djangotest.AssertEqual(t, created.Name, "Rabbit")
	djangotest.AssertEqual(t, created.UniqueTogether, [][]string{{"Parent", "Knight"}})
	djangotest.AssertEqual(t, created.Indexes,
		[]migrations.Index{{Name: "rabbit_circular_fk_index", Fields: []string{"Parent", "Knight"}}})
	assertMigrationDependencies(t, changes, "eggs", 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_replace_string_with_foreignkey
func TestReplaceStringWithForeignkey(t *testing.T) {
	changes := getChanges(t, models{authorWithPublisherName}, models{authorWithPublisher, publisher}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel", "RemoveField", "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Publisher")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "testapp", 0, 1).Field, "PublisherName")
	djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, 2).Field.Name, "Publisher")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_foreign_key_removed_before_target_model
func TestForeignKeyRemovedBeforeTargetModel(t *testing.T) {
	// removes both the model and FK
	changes := getChanges(t, models{authorWithPublisher, publisher}, models{authorName}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveField", "DeleteModel")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "testapp", 0, 0).Field, "Publisher")
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "testapp", 0, 1).Name, "Publisher")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_non_circular_foreignkey_dependency_removal
func TestNonCircularForeignkeyDependencyRemoval(t *testing.T) {
	changes := getChanges(t, models{authorWithPublisher, publisherWithAuthor}, nil, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveField", "DeleteModel", "DeleteModel")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "testapp", 0, 0),
		migrations.RemoveField{Model: "Publisher", Field: "Author"})
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "testapp", 0, 1).Name, "Author")
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "testapp", 0, 2).Name, "Publisher")
}

// Django marks the "migrations" app as a real app without migrations. In Go
// the target model is simply not part of the state.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_first_dependency
func TestFirstDependency(t *testing.T) {
	// Load graph.
	loader := newLoader(t, nil)
	a := migrations.NewAutodetector(makeProjectState(), makeProjectState(bookMigrationsFK), nil)
	changes, err := a.DetectChanges(loader.Graph)
	if err != nil {
		t.Fatalf("DetectChanges: %v", err)
	}
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "otherapp", 0, 0).Name, "Book")
	assertMigrationDependencies(t, changes, "otherapp", 0, k("migrations", migrations.FirstMigration))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_last_dependency
func TestLastDependency(t *testing.T) {
	// Load graph.
	loader := newLoader(t, nil, testMigrations())
	a := migrations.NewAutodetector(makeProjectState(), makeProjectState(bookMigrationsFK), nil)
	changes, err := a.DetectChanges(loader.Graph)
	if err != nil {
		t.Fatalf("DetectChanges: %v", err)
	}
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "otherapp", 0, 0).Name, "Book")
	assertMigrationDependencies(t, changes, "otherapp", 0, k("migrations", "0002_second"))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_fk_before_model_deletion
func TestAlterFKBeforeModelDeletion(t *testing.T) {
	changes := getChanges(t,
		models{authorName, publisherWithAuthor},
		models{aardvarkTestapp, publisherWithAardvarkAuthor}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel", "AlterField", "DeleteModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Aardvark")
	djangotest.AssertEqual(t, operationAt[migrations.AlterField](t, changes, "testapp", 0, 1).Field.Name, "Author")
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "testapp", 0, 2).Name, "Author")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_fk_dependency_other_app
func TestFKDependencyOtherApp(t *testing.T) {
	changes := getChanges(t, models{authorName, book}, models{authorWithBook, book}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, 0).Field.Name, "Book")
	assertMigrationDependencies(t, changes, "testapp", 0, k("otherapp", migrations.FirstMigration))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_field_to_fk_dependency_other_app
func TestAlterFieldToFKDependencyOtherApp(t *testing.T) {
	changes := getChanges(t, models{authorEmpty, bookWithNoAuthorFK}, models{authorEmpty, book}, nil)
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterField")
	assertMigrationDependencies(t, changes, "otherapp", 0, k("testapp", migrations.FirstMigration))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_circular_dependency_mixed_addcreate
func TestCircularDependencyMixedAddcreate(t *testing.T) {
	address := model("a", "Address", idField, fkField("Country", "country_id", "b.DeliveryCountry"))
	person := model("a", "Person", idField)
	apackage := model("b", "APackage", idField, fkField("Person", "person_id", "a.Person"))
	country := model("b", "DeliveryCountry", idField)
	changes := getChanges(t, nil, models{address, person, apackage, country}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "a", 2)
	assertNumberMigrations(t, changes, "b", 1)
	assertOperationTypes(t, changes, "a", 0, "CreateModel", "CreateModel")
	assertOperationTypes(t, changes, "a", 1, "AddField")
	assertOperationTypes(t, changes, "b", 0, "CreateModel", "CreateModel")
}
