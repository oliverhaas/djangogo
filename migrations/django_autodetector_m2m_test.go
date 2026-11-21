package migrations_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_many_to_many
func TestAutodetectorAddManyToMany(t *testing.T) {
	authorWithM2M := model("testapp", "Author", idField, m2mField("Publishers", "testapp.Publisher"))
	q := &countingQuestioner{}
	changes := getChanges(t, models{authorEmpty, publisher}, models{authorWithM2M, publisher}, q)
	djangotest.AssertEqual(t, q.additions, 0)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AddField")
	op := operationAt[migrations.AddField](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, op.Field.Name, "Publishers")
	djangotest.AssertEqual(t, op.Field.RelKind, orm.RelM2M)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_alter_many_to_many
func TestAutodetectorAlterManyToMany(t *testing.T) {
	authorWithM2M := model("testapp", "Author", idField, m2mField("Publishers", "testapp.Publisher"))
	blankM2M := m2mField("Publishers", "testapp.Publisher")
	blankM2M.Blank = true
	authorWithM2MBlank := model("testapp", "Author", idField, blankM2M)

	changes := getChanges(t, models{authorWithM2M, publisher}, models{authorWithM2MBlank, publisher}, nil)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "AlterField")
	op := operationAt[migrations.AlterField](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, op.Field.Name, "Publishers")
	djangotest.AssertEqual(t, op.Field.Blank, true)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_create_with_through_model
func TestAutodetectorCreateWithThroughModel(t *testing.T) {
	authorWithThrough := model("testapp", "Author", idField,
		m2mThroughField("Publishers", "testapp.Publisher", "testapp.Contract"),
	)
	contract := model("testapp", "Contract", idField,
		fkField("Author", "author_id", "testapp.Author"),
		fkField("Publisher", "publisher_id", "testapp.Publisher"),
	)

	changes := getChanges(t, nil, models{authorWithThrough, publisher, contract}, nil)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel", "CreateModel", "CreateModel", "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Author")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 1).Name, "Publisher")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 2).Name, "Contract")
	addOp := operationAt[migrations.AddField](t, changes, "testapp", 0, 3)
	djangotest.AssertEqual(t, addOp.Model, "Author")
	djangotest.AssertEqual(t, addOp.Field.Name, "Publishers")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_create_with_through_model_separate_apps
func TestAutodetectorCreateWithThroughModelSeparateApps(t *testing.T) {
	authorWithThrough := model("authors", "Author", idField,
		m2mThroughField("Publishers", "testapp.Publisher", "contract.Contract"),
	)
	contract := model("contract", "Contract", idField,
		fkField("Author", "author_id", "authors.Author"),
		fkField("Publisher", "publisher_id", "testapp.Publisher"),
	)

	changes := getChanges(t, nil, models{authorWithThrough, publisher, contract}, nil)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertNumberMigrations(t, changes, "contract", 1)
	assertNumberMigrations(t, changes, "authors", 2)
	assertMigrationDependencies(t, changes, "authors", 1,
		migrations.Key{App: "authors", Name: "auto_1"},
		migrations.Key{App: "contract", Name: "auto_1"},
		migrations.Key{App: "testapp", Name: "auto_1"},
	)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel")
	assertOperationTypes(t, changes, "contract", 0, "CreateModel")
	assertOperationTypes(t, changes, "authors", 0, "CreateModel")
	assertOperationTypes(t, changes, "authors", 1, "AddField")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_many_to_many_removed_before_through_model
func TestAutodetectorM2MRemovedBeforeThroughModel(t *testing.T) {
	bookWithThrough := model("otherapp", "Book", idField,
		m2mThroughField("Authors", "testapp.Author", "otherapp.Attribution"),
	)
	bookNoAuthor := model("otherapp", "Book", idField)
	attribution := model("otherapp", "Attribution", idField,
		fkField("Author", "author_id", "testapp.Author"),
		fkField("Book", "book_id", "otherapp.Book"),
	)

	changes := getChanges(t,
		models{bookWithThrough, authorName, attribution},
		models{bookNoAuthor, authorName},
		nil,
	)
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "RemoveField", "DeleteModel")
	rem := operationAt[migrations.RemoveField](t, changes, "otherapp", 0, 0)
	djangotest.AssertEqual(t, rem.Model, "Book")
	djangotest.AssertEqual(t, rem.Field, "Authors")
	del := operationAt[migrations.DeleteModel](t, changes, "otherapp", 0, 1)
	djangotest.AssertEqual(t, del.Name, "Attribution")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_many_to_many_removed_before_through_model_2
func TestAutodetectorM2MRemovedBeforeThroughModel2(t *testing.T) {
	bookWithThrough := model("otherapp", "Book", idField,
		m2mThroughField("Authors", "testapp.Author", "otherapp.Attribution"),
	)
	attribution := model("otherapp", "Attribution", idField,
		fkField("Author", "author_id", "testapp.Author"),
		fkField("Book", "book_id", "otherapp.Book"),
	)

	changes := getChanges(t,
		models{bookWithThrough, authorName, attribution},
		models{authorName},
		nil,
	)
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "RemoveField", "DeleteModel", "DeleteModel")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "otherapp", 0, 0).Model, "Book")
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "otherapp", 0, 1).Name, "Attribution")
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "otherapp", 0, 2).Name, "Book")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_rename_m2m_through_model
func TestAutodetectorRenameM2MThroughModel(t *testing.T) {
	authorBefore := model("testapp", "Author", idField,
		m2mThroughField("Publishers", "testapp.Publisher", "testapp.Contract"),
	)
	contractBefore := model("testapp", "Contract", idField,
		fkField("Author", "author_id", "testapp.Author"),
		fkField("Publisher", "publisher_id", "testapp.Publisher"),
	)
	authorAfter := model("testapp", "Author", idField,
		m2mThroughField("Publishers", "testapp.Publisher", "testapp.Deal"),
	)
	dealAfter := model("testapp", "Deal", idField,
		fkField("Author", "author_id", "testapp.Author"),
		fkField("Publisher", "publisher_id", "testapp.Publisher"),
	)

	changes := getChanges(t,
		models{authorBefore, publisher, contractBefore},
		models{authorAfter, publisher, dealAfter},
		migrations.FixedQuestioner{RenameModel: true},
	)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RenameModel")
	rn := operationAt[migrations.RenameModel](t, changes, "testapp", 0, 0)
	djangotest.AssertEqual(t, rn.OldName, "Contract")
	djangotest.AssertEqual(t, rn.NewName, "Deal")
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_concrete_field_changed_to_many_to_many
func TestAutodetectorConcreteFieldChangedToM2M(t *testing.T) {
	authorFormer := model("testapp", "Author", idField, charField("Publishers", "publishers", 100))
	authorM2M := model("testapp", "Author", idField, m2mField("Publishers", "testapp.Publisher"))

	changes := getChanges(t, models{authorFormer}, models{authorM2M, publisher}, nil)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "CreateModel", "RemoveField", "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.CreateModel](t, changes, "testapp", 0, 0).Name, "Publisher")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "testapp", 0, 1).Field, "Publishers")
	djangotest.AssertEqual(t, operationAt[migrations.AddField](t, changes, "testapp", 0, 2).Field.RelKind, orm.RelM2M)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_many_to_many_changed_to_concrete_field
func TestAutodetectorM2MChangedToConcreteField(t *testing.T) {
	authorM2M := model("testapp", "Author", idField, m2mField("Publishers", "testapp.Publisher"))
	authorFormer := model("testapp", "Author", idField, charField("Publishers", "publishers", 100))
	q := migrations.FixedQuestioner{NotNullAddition: "none"}

	changes := getChanges(t, models{authorM2M, publisher}, models{authorFormer}, q)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveField", "DeleteModel", "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "testapp", 0, 0).Field, "Publishers")
	djangotest.AssertEqual(t, operationAt[migrations.DeleteModel](t, changes, "testapp", 0, 1).Name, "Publisher")
	addOp := operationAt[migrations.AddField](t, changes, "testapp", 0, 2)
	djangotest.AssertEqual(t, addOp.Field.MaxLength, 100)
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_m2m_target_change_generates_remove_and_add
func TestAutodetectorM2MTargetChange(t *testing.T) {
	otherPublisher := model("testapp", "OtherPublisher", idField, charField("Name", "name", 100))
	authorBefore := model("testapp", "Author", idField, m2mField("Publishers", "testapp.Publisher"))
	authorAfter := model("testapp", "Author", idField, m2mField("Publishers", "testapp.OtherPublisher"))

	changes := getChanges(t,
		models{publisher, otherPublisher, authorBefore},
		models{publisher, otherPublisher, authorAfter},
		nil,
	)
	assertNumberMigrations(t, changes, "testapp", 1)
	assertOperationTypes(t, changes, "testapp", 0, "RemoveField", "AddField")
	djangotest.AssertEqual(t, operationAt[migrations.RemoveField](t, changes, "testapp", 0, 0).Field, "Publishers")
	addOp := operationAt[migrations.AddField](t, changes, "testapp", 0, 1)
	djangotest.AssertEqual(t, addOp.Field.RelTo, "testapp.OtherPublisher")
}

// TestM2MStateFromRegistryAndWriter verifies that StateFromRegistry snapshots
// M2M fields from orm.Registry and RenderMigration serializes orm.RelM2M.
func TestM2MStateFromRegistryAndWriter(t *testing.T) {
	type regTag struct {
		ID   int64
		Name string `orm:"max_length=40"`
	}
	type regPost struct {
		ID    int64
		Title string          `orm:"max_length=80"`
		Tags  orm.M2M[regTag] `orm:"blank"`
	}

	reg := orm.NewRegistry()
	if _, err := reg.RegisterApp("blog", &regTag{}); err != nil {
		t.Fatalf("RegisterApp regTag: %v", err)
	}
	if _, err := reg.RegisterApp("blog", &regPost{}); err != nil {
		t.Fatalf("RegisterApp regPost: %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	reg.Freeze()

	state := migrations.StateFromRegistry(reg)
	post, ok := state.Model("blog", "regPost")
	if !ok {
		t.Fatal("regPost missing from ProjectState")
	}
	tags, ok := post.FieldByName("Tags")
	if !ok {
		t.Fatal("Tags M2M field missing from ModelState")
	}
	djangotest.AssertEqual(t, tags.RelKind, orm.RelM2M)
	djangotest.AssertEqual(t, tags.RelTo, "blog.regTag")
	djangotest.AssertEqual(t, tags.Blank, true)

	changes, err := migrations.NewAutodetector(migrations.NewProjectState(), state, nil).Changes(nil, nil, "")
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	assertNumberMigrations(t, changes, "blog", 1)
	src, err := migrations.RenderMigration("blog", *changes["blog"][0])
	if err != nil {
		t.Fatalf("RenderMigration: %v", err)
	}
	if !strings.Contains(src, "orm.RelM2M") {
		t.Errorf("rendered migration missing orm.RelM2M:\n%s", src)
	}
}
