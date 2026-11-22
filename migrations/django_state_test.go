package migrations_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// stateAuthor and stateBook are the models of TestCreate.
type stateAuthor struct {
	ID   int64
	Name string `orm:"max_length=255"`
	Bio  string `orm:"type=text"`
	Age  int64  `orm:"null;blank"`
}

type stateBook struct {
	ID     int64
	Title  string `orm:"max_length=1000"`
	Author orm.FK[stateAuthor]
}

// Meta sets the table of stateBook, Django's db_table = "test_tome".
func (stateBook) Meta() orm.Meta { return orm.Meta{Table: "test_tome"} }

// textField is a TextField named Text.
var textField = migrations.FieldState{Name: "Text", Column: "text", Kind: orm.KindText}

// addStateModel adds a model with an id field and the given fields to state.
func addStateModel(t *testing.T, state *migrations.ProjectState, app, name string, fields ...migrations.FieldState) {
	t.Helper()
	op := migrations.CreateModel{Name: name, Fields: append([]migrations.FieldState{idField}, fields...)}
	if err := op.StateForwards(app, state); err != nil {
		t.Fatalf("create model %s.%s: %v", app, name, err)
	}
}

// baseProjectState returns the state of Django's get_base_project_state.
// Django's many-to-many fields have no Go counterpart, so they are foreign
// keys here: Comment.Comments points to Comment and Post.Authors to User.
func baseProjectState(t *testing.T) *migrations.ProjectState {
	t.Helper()
	state := migrations.NewProjectState()
	addStateModel(t, state, "tests", "User")
	addStateModel(t, state, "tests", "Comment", textField,
		fkField("User", "user_id", "tests.User"),
		fkField("Comments", "comments_id", "tests.Comment"))
	addStateModel(t, state, "tests", "Post", textField,
		fkField("Authors", "authors_id", "tests.User"))
	return state
}

// Ported from Django: tests/migrations/test_state.py::StateTests.test_create
//
// The Go state has no managers, proxy or inherited models, many-to-many
// fields or verbose names, and the registry does not expose indexes or
// unique-together sets, so the assertions on those are dropped. The fields,
// null flags, max lengths, tables and the foreign key remain.
func TestCreate(t *testing.T) {
	reg := orm.NewRegistry()
	if _, err := reg.RegisterApp("migrations", &stateAuthor{}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.RegisterApp("migrations", &stateBook{}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatal(err)
	}
	projectState := migrations.StateFromRegistry(reg)
	author := modelOf(t, projectState, "migrations", "stateAuthor")
	book := modelOf(t, projectState, "migrations", "stateBook")

	djangotest.AssertEqual(t, author.App, "migrations")
	djangotest.AssertEqual(t, author.Name, "stateAuthor")
	djangotest.AssertEqual(t, fieldNames(author), []string{"ID", "Name", "Bio", "Age"})
	djangotest.AssertEqual(t, fieldOf(t, projectState, "migrations", "stateAuthor", "Name").MaxLength, 255)
	djangotest.AssertEqual(t, fieldOf(t, projectState, "migrations", "stateAuthor", "Bio").Null, false)
	djangotest.AssertEqual(t, fieldOf(t, projectState, "migrations", "stateAuthor", "Age").Null, true)

	djangotest.AssertEqual(t, book.App, "migrations")
	djangotest.AssertEqual(t, book.Name, "stateBook")
	djangotest.AssertEqual(t, book.Table, "test_tome")
	djangotest.AssertEqual(t, fieldNames(book), []string{"ID", "Title", "Author"})
	djangotest.AssertEqual(t, fieldOf(t, projectState, "migrations", "stateBook", "Title").MaxLength, 1000)
	rel := fieldOf(t, projectState, "migrations", "stateBook", "Author")
	djangotest.AssertEqual(t, rel.Null, false)
	djangotest.AssertEqual(t, rel.RelTo, "migrations.stateAuthor")
}

// Ported from Django: tests/migrations/test_state.py::StateTests.test_add_relations
//
// Django checks that the rendered model classes of a cloned state stay
// untouched while the new state reloads them. The Go state has no rendered
// models, so the test asserts the counterpart: AddField changes the relations
// of the new state only, and the cloned state keeps its own.
func TestAddRelations(t *testing.T) {
	projectState := migrations.NewProjectState()
	addStateModel(t, projectState, "something", "A")
	addStateModel(t, projectState, "something", "C")

	oldState := projectState.Clone()
	djangotest.AssertEqual(t, len(oldState.Referencing("something.A")), 0)

	op := migrations.AddField{Model: "C", Field: fkField("ToA", "to_a_id", "something.A")}
	if err := op.StateForwards("something", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("something.A"), []string{"something.C.ToA"})
	djangotest.AssertEqual(t, len(oldState.Referencing("something.A")), 0)
	if _, ok := oldState.Models["something.C"].FieldByName("ToA"); ok {
		t.Error("the old state gained the new field")
	}
}

// Ported from Django: tests/migrations/test_state.py::StateTests.test_remove_relations
//
// The Go counterpart of the related objects of a rendered model is
// Referencing.
func TestRemoveRelations(t *testing.T) {
	build := func() *migrations.ProjectState {
		state := migrations.NewProjectState()
		addStateModel(t, state, "something", "A")
		addStateModel(t, state, "something", "B", fkField("ToA", "to_a_id", "something.A"))
		return state
	}

	projectState := build()
	djangotest.AssertEqual(t, len(projectState.Referencing("something.A")), 1)
	oldState := projectState.Clone()

	if err := (migrations.RemoveField{Model: "B", Field: "ToA"}).StateForwards("something", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, len(oldState.Referencing("something.A")), 1)
	djangotest.AssertEqual(t, len(projectState.Referencing("something.A")), 0)

	// Same test for a deleted model.
	projectState = build()
	oldState = projectState.Clone()
	if err := (migrations.DeleteModel{Name: "B"}).StateForwards("something", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, len(oldState.Referencing("something.A")), 1)
	djangotest.AssertEqual(t, len(projectState.Referencing("something.A")), 0)
}

// Ported from Django: tests/migrations/test_state.py::StateTests.test_self_relation
//
// Django uses a self-referential many-to-many field and checks the through
// model of the old and the new rendered model. The Go counterpart is a
// self-referential foreign key: AlterField on it keeps it pointing to its own
// model and leaves the cloned state alone.
func TestSelfRelation(t *testing.T) {
	projectState := migrations.NewProjectState()
	addStateModel(t, projectState, "something", "A", fkField("ToA", "to_a_id", "something.A"))
	djangotest.AssertEqual(t, len(projectState.Referencing("something.A")), 1)
	oldState := projectState.Clone()

	altered := fkField("ToA", "to_a_id", "something.A")
	altered.Blank = true
	altered.Null = true
	op := migrations.AlterField{Model: "A", Field: altered}
	if err := op.StateForwards("something", projectState); err != nil {
		t.Fatal(err)
	}

	oldField := fieldOf(t, oldState, "something", "A", "ToA")
	newField := fieldOf(t, projectState, "something", "A", "ToA")
	djangotest.AssertEqual(t, oldField.Null, false)
	djangotest.AssertEqual(t, newField.Null, true)
	djangotest.AssertEqual(t, oldField.RelTo, "something.A")
	djangotest.AssertEqual(t, newField.RelTo, "something.A")
	djangotest.AssertEqual(t, oldState.Referencing("something.A"), []string{"something.A.ToA"})
	djangotest.AssertEqual(t, projectState.Referencing("something.A"), []string{"something.A.ToA"})
}

// Ported from Django: tests/migrations/test_state.py::StateTests.test_equality
//
// ProjectState has no == operator in Go; reflect.DeepEqual compares the
// states, which is what djangotest.AssertEqual does.
func TestEquality(t *testing.T) {
	tag := func(maxLength int) *migrations.ProjectState {
		state := migrations.NewProjectState()
		addStateModel(t, state, "migrations", "Tag",
			migrations.FieldState{Name: "Name", Column: "name", Kind: orm.KindChar, MaxLength: maxLength},
			migrations.FieldState{Name: "Hidden", Column: "hidden", Kind: orm.KindBool})
		return state
	}
	// Test two things that should be equal.
	projectState := tag(100)
	otherState := projectState.Clone()
	djangotest.AssertEqual(t, projectState, projectState)
	djangotest.AssertEqual(t, projectState, otherState)
	if !reflect.DeepEqual(projectState, otherState) {
		t.Error("a clone must equal its state")
	}

	// Make a very small change (max_len 99) and see if that affects it.
	projectState = tag(99)
	djangotest.AssertNotEqual(t, projectState, otherState)
}

// Ported from Django: tests/migrations/test_state.py::StateTests.test_reference_mixed_case_app_label
func TestReferenceMixedCaseAppLabel(t *testing.T) {
	const app = "MiXedCase_migrations"
	projectState := migrations.NewProjectState()
	addStateModel(t, projectState, app, "Author")
	addStateModel(t, projectState, app, "Book", fkField("Author", "author_id", app+".Author"))
	addStateModel(t, projectState, app, "Magazine", fkField("Authors", "authors_id", app+".Author"))
	djangotest.AssertEqual(t, len(projectState.Models), 3)
	djangotest.AssertEqual(t, projectState.Referencing(app+".Author"),
		[]string{app + ".Book.Author", app + ".Magazine.Authors"})
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_add_model
//
// Referencing lists "app.Model.Field" in model creation order, where
// Django's relations map lists (app, model) pairs.
func TestStateRelationsAddModel(t *testing.T) {
	projectState := baseProjectState(t)
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
	djangotest.AssertEqual(t, projectState.Referencing("tests.Comment"), []string{"tests.Comment.Comments"})
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.Post")), 0)
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_add_model_no_relations
func TestStateRelationsAddModelNoRelations(t *testing.T) {
	projectState := migrations.NewProjectState()
	projectState.AddModel(&migrations.ModelState{
		App: "migrations", Name: "Tag", Table: "migrations_tag", Fields: []migrations.FieldState{idField},
	})
	djangotest.AssertEqual(t, len(projectState.Referencing("migrations.Tag")), 0)
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_add_model_other_app
func TestStateRelationsAddModelOtherApp(t *testing.T) {
	projectState := baseProjectState(t)
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
	addStateModel(t, projectState, "tests_other", "Comment", fkField("User", "user_id", "tests.User"))
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors", "tests_other.Comment.User"})
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_remove_model
func TestStateRelationsRemoveModel(t *testing.T) {
	projectState := baseProjectState(t)
	projectState.RemoveModel("tests", "Comment")
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"), []string{"tests.Post.Authors"})
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.Comment")), 0)
	projectState.RemoveModel("tests", "Post")
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.User")), 0)
	projectState.RemoveModel("tests", "User")
	djangotest.AssertEqual(t, len(projectState.Models), 0)
}

// renameModel applies the RenameModel state change.
func renameModel(t *testing.T, state *migrations.ProjectState, app, oldName, newName string) {
	t.Helper()
	if err := (migrations.RenameModel{OldName: oldName, NewName: newName}).StateForwards(app, state); err != nil {
		t.Fatal(err)
	}
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_rename_model
//
// A renamed model keeps its position in the creation order, so the lists
// differ in order from Django's, which reinserts the renamed model.
func TestStateRelationsRenameModel(t *testing.T) {
	projectState := baseProjectState(t)
	renameModel(t, projectState, "tests", "Comment", "Opinion")
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Opinion.User", "tests.Post.Authors"})
	djangotest.AssertEqual(t, projectState.Referencing("tests.Opinion"), []string{"tests.Opinion.Comments"})
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.Comment")), 0)

	renameModel(t, projectState, "tests", "User", "Author")
	djangotest.AssertEqual(t, projectState.Referencing("tests.Author"),
		[]string{"tests.Opinion.User", "tests.Post.Authors"})
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.User")), 0)
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_rename_model_no_relations
func TestStateRelationsRenameModelNoRelations(t *testing.T) {
	projectState := baseProjectState(t)
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.Post")), 0)
	// Rename a model without relations of its own.
	renameModel(t, projectState, "tests", "Post", "Blog")
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Blog.Authors"})
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.Blog")), 0)
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_add_field
func TestStateRelationsAddField(t *testing.T) {
	projectState := baseProjectState(t)
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.Post")), 0)
	// Add a self-referential foreign key.
	op := migrations.AddField{Model: "Post", Field: fkField("NextPost", "next_post_id", "tests.Post")}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.Post"), []string{"tests.Post.NextPost"})
	// Add a foreign key.
	op = migrations.AddField{Model: "Comment", Field: fkField("Post", "post_id", "tests.Post")}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.Post"),
		[]string{"tests.Comment.Post", "tests.Post.NextPost"})
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_remove_field
func TestStateRelationsRemoveField(t *testing.T) {
	projectState := baseProjectState(t)
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
	// Remove the Post.Authors relation (a many-to-many field in Django).
	if err := (migrations.RemoveField{Model: "Post", Field: "Authors"}).StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"), []string{"tests.Comment.User"})
	// Remove a foreign key.
	if err := (migrations.RemoveField{Model: "Comment", Field: "User"}).StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, len(projectState.Referencing("tests.User")), 0)
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_remove_field_no_relations
func TestStateRelationsRemoveFieldNoRelations(t *testing.T) {
	projectState := baseProjectState(t)
	// Remove a non-relation field.
	if err := (migrations.RemoveField{Model: "Post", Field: "Text"}).StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_rename_field
func TestStateRelationsRenameField(t *testing.T) {
	projectState := baseProjectState(t)
	field := fieldOf(t, projectState, "tests", "Comment", "User")
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})

	op := migrations.RenameField{Model: "Comment", OldName: "User", NewName: "Author"}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	renamed := fieldOf(t, projectState, "tests", "Comment", "Author")
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.Author", "tests.Post.Authors"})
	// The renamed field keeps its definition.
	field.Name, field.Column = renamed.Name, renamed.Column
	djangotest.AssertEqual(t, renamed, field)
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_rename_field_no_relations
func TestStateRelationsRenameFieldNoRelations(t *testing.T) {
	projectState := baseProjectState(t)
	// Rename a non-relation field.
	op := migrations.RenameField{Model: "Post", OldName: "Text", NewName: "Description"}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_alter_field
//
// The many-to-many half of Django's test has no Go counterpart; the second
// alteration turns the integer field back into a foreign key.
func TestStateRelationsAlterField(t *testing.T) {
	projectState := baseProjectState(t)
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
	// Alter a foreign key to a non-relation field.
	op := migrations.AlterField{Model: "Comment", Field: migrations.FieldState{
		Name: "User", Column: "user_id", Kind: orm.KindInt,
	}}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"), []string{"tests.Post.Authors"})
	// Alter the non-relation field to a foreign key.
	op = migrations.AlterField{Model: "Comment", Field: fkField("User", "user_id", "tests.User")}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_alter_field_m2m_to_fk
//
// Django turns a many-to-many field into a foreign key. The Go state has no
// many-to-many fields, so the test repoints a foreign key to a model of
// another app.
func TestStateRelationsAlterFieldM2MToFK(t *testing.T) {
	projectState := baseProjectState(t)
	addStateModel(t, projectState, "tests_other", "user_other")
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Post.Authors"})
	djangotest.AssertEqual(t, len(projectState.Referencing("tests_other.user_other")), 0)
	// Alter a foreign key to point to another app.
	op := migrations.AlterField{Model: "Post", Field: fkField("Authors", "authors_id", "tests_other.user_other")}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"), []string{"tests.Comment.User"})
	djangotest.AssertEqual(t, projectState.Referencing("tests_other.user_other"), []string{"tests.Post.Authors"})
}

// Ported from Django: tests/migrations/test_state.py::StateRelationsTests.test_many_relations_to_same_model
func TestStateRelationsManyRelationsToSameModel(t *testing.T) {
	projectState := baseProjectState(t)
	op := migrations.AddField{Model: "Comment", Field: fkField("Reviewer", "reviewer_id", "tests.User")}
	if err := op.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	// Two foreign keys to the same model.
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Comment.Reviewer", "tests.Post.Authors"})
	// Rename the second foreign key.
	rename := migrations.RenameField{Model: "Comment", OldName: "Reviewer", NewName: "Supervisor"}
	if err := rename.StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.User", "tests.Comment.Supervisor", "tests.Post.Authors"})
	// Remove the first foreign key.
	if err := (migrations.RemoveField{Model: "Comment", Field: "User"}).StateForwards("tests", projectState); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, projectState.Referencing("tests.User"),
		[]string{"tests.Comment.Supervisor", "tests.Post.Authors"})
}

// Ported from Django: tests/migrations/test_state.py::ModelStateTests.test_sanity_index_name
//
// Django raises for ModelState(options={"indexes": [...]}) without a name.
// The Go state is built by CreateModel and AddIndex, which refuse an unnamed
// index.
func TestSanityIndexName(t *testing.T) {
	state := migrations.NewProjectState()
	unnamed := migrations.Index{Fields: []string{"Field"}}
	create := migrations.CreateModel{
		Name: "Model", Fields: []migrations.FieldState{idField, pinkField}, Indexes: []migrations.Index{unnamed},
	}
	err := create.StateForwards("app", state)
	if err == nil || !strings.Contains(err.Error(), "require a name argument") {
		t.Errorf("CreateModel with an unnamed index: err = %v", err)
	}
	addStateModel(t, state, "app", "Model", pinkField)
	err = (migrations.AddIndex{Model: "Model", Index: unnamed}).StateForwards("app", state)
	if err == nil || !strings.Contains(err.Error(), "require a name argument") {
		t.Errorf("AddIndex with an unnamed index: err = %v", err)
	}
}

// stateModelWithConstraints is the model of TestFromModelConstraints.
type stateModelWithConstraints struct {
	ID   int64
	Size int64
}

// Meta sets the constraints of stateModelWithConstraints.
func (stateModelWithConstraints) Meta() orm.Meta {
	return orm.Meta{Constraints: []orm.Constraint{
		orm.CheckConstraint{Condition: orm.Q("size__gt", 1), Name: "size_gt_1"},
	}}
}

// Ported from Django: tests/migrations/test_state.py::ModelStateTests.test_from_model_constraints
//
// Django asserts that the state holds copies of the model's constraints. A
// Go constraint is a value, so the test asserts that a change to the state's
// list leaves the model's constraints alone.
func TestFromModelConstraints(t *testing.T) {
	reg := orm.NewRegistry()
	m, err := reg.RegisterApp("migrations", &stateModelWithConstraints{})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatal(err)
	}
	state := migrations.StateFromRegistry(reg)
	modelConstraints := m.Constraints()
	stateConstraints := modelOf(t, state, "migrations", "stateModelWithConstraints").Constraints
	djangotest.AssertEqual(t, stateConstraints, modelConstraints)
	stateConstraints[0] = orm.CheckConstraint{Condition: orm.Q("size__gt", 2), Name: "size_gt_2"}
	djangotest.AssertEqual(t, m.Constraints(), modelConstraints)
}
