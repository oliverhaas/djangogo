package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
)

// The ports of Django's AutodetectorTests use Go names for models and fields:
// Django's field "name" is "Name" here. A test whose name is taken by a test
// of the operation with the same name carries the prefix TestAutodetector.

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_arrange_for_graph
func TestArrangeForGraph(t *testing.T) {
	// Make a fake graph.
	graph := graphOf(t,
		[]migrations.Key{k("testapp", "0001_initial"), k("testapp", "0002_foobar"), k("otherapp", "0001_initial")},
		[][2]migrations.Key{
			{k("testapp", "0002_foobar"), k("testapp", "0001_initial")},
			{k("testapp", "0002_foobar"), k("otherapp", "0001_initial")},
		})
	// Use project state to make a new migration change set.
	changes := getChanges(t, models{publisher, otherPony}, models{authorEmpty, publisher, otherPony, otherStable}, nil)
	changes = migrations.ArrangeForGraph(changes, graph, "")
	// Make sure there's a new name, deps match, etc.
	djangotest.AssertEqual(t, changes["testapp"][0].Name, "0003_author")
	djangotest.AssertEqual(t, changes["testapp"][0].Dependencies, []migrations.Key{k("testapp", "0002_foobar")})
	djangotest.AssertEqual(t, changes["otherapp"][0].Name, "0002_stable")
	djangotest.AssertEqual(t, changes["otherapp"][0].Dependencies, []migrations.Key{k("otherapp", "0001_initial")})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_arrange_for_graph_with_multiple_initial
func TestArrangeForGraphWithMultipleInitial(t *testing.T) {
	changes := getChanges(t, nil, models{authorWithBook, book, attribution}, nil)
	changes = migrations.ArrangeForGraph(changes, migrations.NewGraph(), "")

	djangotest.AssertEqual(t, changes["otherapp"][0].Name, "0001_initial")
	assertMigrationDependencies(t, changes, "otherapp", 0)
	djangotest.AssertEqual(t, changes["otherapp"][1].Name, "0002_initial")
	assertMigrationDependencies(t, changes, "otherapp", 1, k("testapp", "0001_initial"), k("otherapp", "0001_initial"))
	djangotest.AssertEqual(t, changes["testapp"][0].Name, "0001_initial")
	assertMigrationDependencies(t, changes, "testapp", 0, k("otherapp", "0001_initial"))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_trim_apps
func TestTrimApps(t *testing.T) {
	changes := getChanges(t, nil, models{authorEmpty, otherPony, otherStable, thirdThing}, nil)
	changes = migrations.ArrangeForGraph(changes, migrations.NewGraph(), "")
	changes["testapp"][0].Dependencies = append(changes["testapp"][0].Dependencies, k("otherapp", "0001_initial"))
	changes = migrations.TrimToApps(changes, []string{"testapp"})
	// Make sure there's the right set of migrations.
	djangotest.AssertEqual(t, changes["testapp"][0].Name, "0001_initial")
	djangotest.AssertEqual(t, changes["otherapp"][0].Name, "0001_initial")
	if _, ok := changes["thirdapp"]; ok {
		t.Errorf("thirdapp was not trimmed:\n%s", reprChanges(changes))
	}
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_custom_migration_name
func TestCustomMigrationName(t *testing.T) {
	// Make a fake graph.
	graph := graphOf(t,
		[]migrations.Key{k("testapp", "0001_initial"), k("testapp", "0002_foobar"), k("otherapp", "0001_initial")},
		[][2]migrations.Key{{k("testapp", "0002_foobar"), k("testapp", "0001_initial")}})
	// Use project state to make a new migration change set.
	changes := getChanges(t, nil, models{authorEmpty, otherPony, otherStable}, nil)
	// Run through arrange_for_graph.
	changes = migrations.ArrangeForGraph(changes, graph, "custom_name")
	// Make sure there's a new name, deps match, etc.
	djangotest.AssertEqual(t, changes["testapp"][0].Name, "0003_custom_name")
	djangotest.AssertEqual(t, changes["testapp"][0].Dependencies, []migrations.Key{k("testapp", "0002_foobar")})
	djangotest.AssertEqual(t, changes["otherapp"][0].Name, "0002_custom_name")
	djangotest.AssertEqual(t, changes["otherapp"][0].Dependencies, []migrations.Key{k("otherapp", "0001_initial")})
}

// The Django test also checks the managers of the new model. A Go migration
// state holds no managers.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_new_model
func TestNewModel(t *testing.T) {
	changes := getChanges(t, nil, models{otherPony}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "CreateModel")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "otherapp", 0, 0).Name, "Pony")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_old_model
func TestOldModel(t *testing.T) {
	changes := getChanges(t, models{authorEmpty}, nil, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "DeleteModel")
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "testapp", 0, 0).Name, "Author")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_field
func TestAutodetectorAddField(t *testing.T) {
	changes := getChanges(t, models{authorEmpty}, models{authorName}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, 0).Field.Name, "Name")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_date_fields_with_auto_now_not_asking_for_default
func TestAddDateFieldsWithAutoNowNotAskingForDefault(t *testing.T) {
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorEmpty}, models{authorDatesOfBirthAutoNow}, q)
	djangotest.AssertEqual(t, q.additions, 0)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField", "AddField", "AddField")
	for i := range 3 {
		djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, i).Field.AutoNow, true)
	}
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_date_fields_with_auto_now_add_not_asking_for_null_addition
func TestAddDateFieldsWithAutoNowAddNotAskingForNullAddition(t *testing.T) {
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorEmpty}, models{authorDatesOfBirthAutoNowAdd}, q)
	djangotest.AssertEqual(t, q.additions, 0)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField", "AddField", "AddField")
	for i := range 3 {
		djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, i).Field.AutoNowAdd, true)
	}
}

// Django asks for a one-off default for each auto_now_add field and offers
// timezone.now. Go asks nothing: existing rows get the time of the migration,
// so the operations carry no one-off default.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_date_fields_with_auto_now_add_asking_for_default
func TestAddDateFieldsWithAutoNowAddAskingForDefault(t *testing.T) {
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorEmpty}, models{authorDatesOfBirthAutoNowAdd}, q)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField", "AddField", "AddField")
	for i := range 3 {
		op := operationAt[migrations.AddField](t, changes, "testapp", 0, i)
		djangotest.AssertEqual(t, op.Field.AutoNowAdd, true)
		djangotest.AssertEqual(t, op.Default, nil)
	}
	djangotest.AssertEqual(t, q.additions, 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_field
func TestAutodetectorRemoveField(t *testing.T) {
	changes := getChanges(t, models{authorName}, models{authorEmpty}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveField")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "testapp", 0, 0).Field, "Name")
}

// Django's preserve_default=True is an AlterField without a one-off default.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_field
func TestAutodetectorAlterField(t *testing.T) {
	changes := getChanges(t, models{authorName}, models{authorNameLonger}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterField")
	op := operationAt[migrations.AlterField](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, op.Field.Name, "Name")
	djangotest.AssertEqual(t, op.Default, nil)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_field_to_not_null_with_default
func TestAlterFieldToNotNullWithDefault(t *testing.T) {
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorNameNull}, models{authorNameDefault}, q)
	djangotest.AssertEqual(t, q.alterations, 0)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterField")
	op := operationAt[migrations.AlterField](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, op.Field.Name, "Name")
	djangotest.AssertEqual(t, op.Default, nil)
	djangotest.AssertEqual(t, op.Field.Default, any("Ada Lovelace"))
}

// Django's NOT_PROVIDED answer is a nil one-off default in Go.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_field_to_not_null_without_default
func TestAlterFieldToNotNullWithoutDefault(t *testing.T) {
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorNameNull}, models{authorName}, q)
	djangotest.AssertEqual(t, q.alterations, 1)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterField")
	op := operationAt[migrations.AlterField](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, op.Field.Name, "Name")
	djangotest.AssertEqual(t, op.Default, nil)
	djangotest.AssertEqual(t, op.Field.Default, nil)
}

// Django stores the one-off default on the field and sets
// preserve_default=False. Go keeps the field as the model declares it and
// stores the one-off default on the operation.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_field_to_not_null_oneoff_default
func TestAlterFieldToNotNullOneoffDefault(t *testing.T) {
	q := &countingQuestioner{FixedQuestioner: migrations.FixedQuestioner{NotNullAlteration: "Some Name"}}
	changes := getChanges(t, models{authorNameNull}, models{authorName}, q)
	djangotest.AssertEqual(t, q.alterations, 1)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterField")
	op := operationAt[migrations.AlterField](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, op.Field.Name, "Name")
	djangotest.AssertEqual(t, op.Default, any("Some Name"))
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_field
func TestAutodetectorRenameField(t *testing.T) {
	changes := getChanges(t, models{authorName}, models{authorNameRenamed}, migrations.FixedQuestioner{Rename: true})
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameField")
	op := operationAt[migrations.RenameField](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, op.OldName, "Name")
	djangotest.AssertEqual(t, op.NewName, "Names")
	djangotest.AssertEqual(t, op.Column, "")
}

// Django generates AlterField, which sets db_column on the old field, and
// then RenameField. A Go RenameField carries the column to keep, so one
// operation does both.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_field_preserved_db_column
func TestRenameFieldPreservedDBColumn(t *testing.T) {
	before := models{model("app", "Foo", idField, intField("Field", "field"))}
	after := models{model("app", "Foo", idField, intField("RenamedField", "field"))}
	changes := getChanges(t, before, after, migrations.FixedQuestioner{Rename: true})
	assertNumberMigrations(t, changes, "app", 1)
	assertOperationTypes(t, changes, "app", 0, "RenameField")
	djangotest.AssertEqual(t, operationAt[migrations.RenameField](t, changes, "app", 0, 0), migrations.RenameField{
		Model: "Foo", OldName: "Field", NewName: "RenamedField", Column: "field",
	})
}

// As in TestRenameFieldPreservedDBColumn, one RenameField keeps the column.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_related_field_preserved_db_column
func TestRenameRelatedFieldPreservedDBColumn(t *testing.T) {
	before := models{model("app", "Foo", idField), model("app", "Bar", idField, fkField("Foo", "foo_id", "app.Foo"))}
	after := models{model("app", "Foo", idField), model("app", "Bar", idField, fkField("RenamedFoo", "foo_id", "app.Foo"))}
	changes := getChanges(t, before, after, migrations.FixedQuestioner{Rename: true})
	assertNumberMigrations(t, changes, "app", 1)
	assertOperationTypes(t, changes, "app", 0, "RenameField")
	djangotest.AssertEqual(t, operationAt[migrations.RenameField](t, changes, "app", 0, 0), migrations.RenameField{
		Model: "Bar", OldName: "Foo", NewName: "RenamedFoo", Column: "foo_id",
	})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_field_with_renamed_model
func TestRenameFieldWithRenamedModel(t *testing.T) {
	after := models{model("testapp", "RenamedAuthor", idField, charField("RenamedName", "renamed_name", 200))}
	changes := getChanges(t, models{authorName}, after, migrations.FixedQuestioner{RenameModel: true, Rename: true})
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel", "RenameField")
	djangotest.AssertEqual(t, operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0),
		migrations.RenameModel{OldName: "Author", NewName: "RenamedAuthor"})
	rename := operationAt[migrations.RenameField](t, changes, "testapp", 0, 1)
	djangotest.AssertEqual(t, rename.OldName, "Name")
	djangotest.AssertEqual(t, rename.NewName, "RenamedName")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_model
func TestAutodetectorRenameModel(t *testing.T) {
	changes := getChanges(t,
		models{authorWithBook, book},
		models{authorRenamedWithBook, bookWithAuthorRenamed},
		migrations.FixedQuestioner{RenameModel: true})
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel")
	djangotest.AssertEqual(t, operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0),
		migrations.RenameModel{OldName: "Author", NewName: "Writer"})
	// Now that RenameModel handles related fields too, there should be
	// no AlterField for the related field.
	assertNumberMigrations(t, changes, "otherapp", 0)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_model_with_renamed_rel_field
func TestRenameModelWithRenamedRelField(t *testing.T) {
	changes := getChanges(t,
		models{authorWithBook, book},
		models{authorRenamedWithBook, bookWithFieldAndAuthor},
		migrations.FixedQuestioner{Rename: true, RenameModel: true})
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel")
	djangotest.AssertEqual(t, operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0),
		migrations.RenameModel{OldName: "Author", NewName: "Writer"})
	// Right number/type of migrations for related field rename?
	// Alter is already taken care of.
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "RenameField")
	rename := operationAt[migrations.RenameField](t, changes, "otherapp", 0, 0)
	djangotest.AssertEqual(t, rename.OldName, "Author")
	djangotest.AssertEqual(t, rename.NewName, "Writer")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_model_with_fks_in_different_position
func TestRenameModelWithFKsInDifferentPosition(t *testing.T) {
	before := models{
		model("testapp", "EntityA", idField),
		model("testapp", "EntityB", idField,
			charField("SomeLabel", "some_label", 255), fkField("EntityA", "entity_a_id", "testapp.EntityA")),
	}
	after := models{
		model("testapp", "EntityA", idField),
		model("testapp", "RenamedEntityB", idField,
			fkField("EntityA", "entity_a_id", "testapp.EntityA"), charField("SomeLabel", "some_label", 255)),
	}
	changes := getChanges(t, before, after, migrations.FixedQuestioner{RenameModel: true})
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel")
	djangotest.AssertEqual(t, operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0),
		migrations.RenameModel{OldName: "EntityB", NewName: "RenamedEntityB"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_model_reverse_relation_dependencies
func TestRenameModelReverseRelationDependencies(t *testing.T) {
	before := models{
		model("testapp", "EntityA", idField),
		model("otherapp", "EntityB", idField, fkField("EntityA", "entity_a_id", "testapp.EntityA")),
	}
	after := models{
		model("testapp", "RenamedEntityA", idField),
		model("otherapp", "EntityB", idField, fkField("EntityA", "entity_a_id", "testapp.RenamedEntityA")),
	}
	changes := getChanges(t, before, after, migrations.FixedQuestioner{RenameModel: true})
	assertNumberMigrations(t, changes, "testapp", 1)
	assertMigrationDependencies(t, changes, "testapp", 0, k("otherapp", migrations.FirstMigration))
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel")
	djangotest.AssertEqual(t, operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0),
		migrations.RenameModel{OldName: "EntityA", NewName: "RenamedEntityA"})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_field_with_default
func TestAddFieldWithDefault(t *testing.T) {
	changes := getChanges(t, models{authorEmpty}, models{authorNameDefault}, nil)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, 0).Field.Name, "Name")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_blank_textfield_and_charfield
func TestAddBlankTextfieldAndCharfield(t *testing.T) {
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorEmpty}, models{authorWithBiographyBlank}, q)
	djangotest.AssertEqual(t, q.additions, 0)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField", "AddField")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_non_blank_textfield_and_charfield
func TestAddNonBlankTextfieldAndCharfield(t *testing.T) {
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorEmpty}, models{authorWithBiographyNonBlank}, q)
	// need to check for questioner call count
	djangotest.AssertEqual(t, q.additions, 2)
	// Right number/type of migrations?
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField", "AddField")
}

// Django returns None for a name without a number, Go returns 0.
//
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_parse_number
func TestParseNumber(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"no_number", 0},
		{"0001_initial", 1},
		{"0002_model3", 2},
		{"0002_auto_20380101_1112", 2},
		{"0002_squashed_0003", 3},
		{"0002_model2_squashed_0003_other4", 3},
		{"0002_squashed_0003_squashed_0004", 4},
		{"0002_model2_squashed_0003_other4_squashed_0005_other6", 5},
		{"0002_custom_name_20380101_1112_squashed_0003_model", 3},
		{"2_squashed_4", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			djangotest.AssertEqual(t, migrations.ParseNumber(tt.name), tt.want)
		})
	}
}
