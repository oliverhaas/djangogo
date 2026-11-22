package migrations_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// suggestName returns the suggested name of a migration with the operations.
func suggestName(initial bool, ops ...migrations.Operation) string {
	m := migrations.Migration{App: "test_app", Name: "some_migration", Initial: initial, Operations: ops}
	return m.SuggestName()
}

// assertAutoName checks that name is the timestamp fallback.
func assertAutoName(t *testing.T, name string) {
	t.Helper()
	if !strings.HasPrefix(name, "auto_") {
		t.Errorf("suggested name = %q, want the prefix auto_", name)
	}
}

// selectOne is a raw SQL operation, which has no name fragment.
var selectOne = migrations.RunSQL{SQL: []string{"SELECT 1 FROM person;"}}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_no_operations
func TestNoOperations(t *testing.T) {
	assertAutoName(t, suggestName(false))
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_no_operations_initial
func TestNoOperationsInitial(t *testing.T) {
	djangotest.AssertEqual(t, suggestName(true), "initial")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_single_operation
func TestSingleOperation(t *testing.T) {
	djangotest.AssertEqual(t, suggestName(false, migrations.CreateModel{Name: "Person"}), "person")
	djangotest.AssertEqual(t, suggestName(false, migrations.DeleteModel{Name: "Person"}), "delete_person")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_single_operation_long_name
func TestSingleOperationLongName(t *testing.T) {
	djangotest.AssertEqual(t,
		suggestName(false, migrations.CreateModel{Name: strings.Repeat("A", 53)}), strings.Repeat("a", 53))
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_two_operations
func TestTwoOperations(t *testing.T) {
	djangotest.AssertEqual(t,
		suggestName(false, migrations.CreateModel{Name: "Person"}, migrations.DeleteModel{Name: "Animal"}),
		"person_delete_animal")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_two_create_models
func TestTwoCreateModels(t *testing.T) {
	djangotest.AssertEqual(t,
		suggestName(false, migrations.CreateModel{Name: "Person"}, migrations.CreateModel{Name: "Animal"}),
		"person_animal")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_two_create_models_with_initial_true
func TestTwoCreateModelsWithInitialTrue(t *testing.T) {
	djangotest.AssertEqual(t,
		suggestName(true, migrations.CreateModel{Name: "Person"}, migrations.CreateModel{Name: "Animal"}),
		"initial")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_many_operations_suffix
func TestManyOperationsSuffix(t *testing.T) {
	djangotest.AssertEqual(t,
		suggestName(false,
			migrations.CreateModel{Name: "Person1"},
			migrations.CreateModel{Name: "Person2"},
			migrations.CreateModel{Name: "Person3"},
			migrations.DeleteModel{Name: "Person4"},
			migrations.DeleteModel{Name: "Person5"},
		),
		"person1_person2_person3_delete_person4_and_more")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_operation_with_no_suggested_name
func TestOperationWithNoSuggestedName(t *testing.T) {
	assertAutoName(t, suggestName(false, migrations.CreateModel{Name: "Person"}, selectOne))
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_operation_with_invalid_chars_in_suggested_name
func TestOperationWithInvalidCharsInSuggestedName(t *testing.T) {
	op := migrations.AddConstraint{
		Model:      "Person",
		Constraint: orm.UniqueConstraint{Fields: []string{"Name"}, Name: "person.name-*~unique!"},
	}
	djangotest.AssertEqual(t, suggestName(false, op), "person_person_name_unique_")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_none_name
func TestNoneName(t *testing.T) {
	assertAutoName(t, suggestName(false, selectOne))
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_none_name_with_initial_true
func TestNoneNameWithInitialTrue(t *testing.T) {
	djangotest.AssertEqual(t, suggestName(true, selectOne), "initial")
}

// Ported from Django: tests/migrations/test_autodetector.py::MigrationSuggestNameTests.test_auto
func TestAuto(t *testing.T) {
	m := migrations.Migration{App: "test_app", Name: "0001_initial"}
	assertAutoName(t, m.SuggestName())
}
