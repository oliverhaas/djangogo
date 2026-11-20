package migrations_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// metaStateBookBase has no Meta indexes or unique_together.
type metaStateAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// metaStateBookPlain is a Book without indexes or unique_together.
type metaStateBookPlain struct {
	ID     int64
	Author orm.FK[metaStateAuthor]
	Title  string `orm:"max_length=200"`
}

// Meta sets the shared model table for plain Book.
func (metaStateBookPlain) Meta() orm.Meta { return orm.Meta{Table: "otherapp_book"} }

// metaStateBookWithOptions declares both Meta.Indexes (named and unnamed) and
// Meta.UniqueTogether.
type metaStateBookWithOptions struct {
	ID     int64
	Author orm.FK[metaStateAuthor]
	Title  string `orm:"max_length=200"`
}

// Meta sets indexes and unique_together on Book.
func (metaStateBookWithOptions) Meta() orm.Meta {
	return orm.Meta{
		Table: "otherapp_book",
		Indexes: []orm.Index{
			{Name: "book_title_author_idx", Fields: []string{"Author", "Title"}},
			{Fields: []string{"-title"}},
		},
		UniqueTogether: [][]string{{"author", "title"}},
	}
}

// stateFromModels builds a ProjectState directly from registered orm models.
func stateFromModels(t *testing.T, app string, models ...any) *migrations.ProjectState {
	t.Helper()
	reg := orm.NewRegistry()
	for _, m := range models {
		if _, err := reg.RegisterApp(app, m); err != nil {
			t.Fatalf("RegisterApp: %v", err)
		}
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return migrations.StateFromRegistry(reg)
}

// Ported from Django: tests/migrations/test_state.py::StateTests.test_create
func TestStateFromRegistryReadsMetaOptions(t *testing.T) {
	ps := stateFromModels(t, "otherapp", &metaStateAuthor{}, &metaStateBookWithOptions{})
	book := modelOf(t, ps, "otherapp", "metaStateBookWithOptions")

	djangotest.AssertEqual(t, book.UniqueTogether, [][]string{{"Author", "Title"}})
	djangotest.AssertEqual(t, len(book.Indexes), 2)
	djangotest.AssertEqual(t, book.Indexes[0], migrations.Index{
		Name:   "book_title_author_idx",
		Fields: []string{"Author", "Title"},
	})
	djangotest.AssertEqual(t, book.Indexes[1], migrations.Index{
		Name:   orm.IndexName("otherapp_book", []string{"-title"}),
		Fields: []string{"-Title"},
	})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_indexes
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_add_unique_together
func TestAutodetectorReadsMetaOptionsFromRegistry(t *testing.T) {
	// Rename the model key to "Book" in both states so the autodetector diffs
	// the same model before and after adding Meta.Indexes and Meta.UniqueTogether.
	from := stateFromModels(t, "otherapp", &metaStateAuthor{}, &metaStateBookPlain{})
	to := stateFromModels(t, "otherapp", &metaStateAuthor{}, &metaStateBookWithOptions{})

	renameKey := func(ps *migrations.ProjectState, oldName, newName string) {
		ms := modelOf(t, ps, "otherapp", oldName)
		ps.RemoveModel("otherapp", oldName)
		ms.Name = newName
		ps.AddModel(ms)
	}
	renameKey(from, "metaStateBookPlain", "Book")
	renameKey(to, "metaStateBookWithOptions", "Book")

	detector := migrations.NewAutodetector(from, to, nil)
	changes, err := detector.Changes(nil, nil, "")
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "AlterUniqueTogether", "AddIndex", "AddIndex")

	ut := operationAt[migrations.AlterUniqueTogether](t, changes, "otherapp", 0, 0)
	djangotest.AssertEqual(t, ut, migrations.AlterUniqueTogether{
		Name:           "Book",
		UniqueTogether: [][]string{{"Author", "Title"}},
	})
	idx1 := operationAt[migrations.AddIndex](t, changes, "otherapp", 0, 1)
	djangotest.AssertEqual(t, idx1, migrations.AddIndex{
		Model: "Book",
		Index: migrations.Index{Name: "book_title_author_idx", Fields: []string{"Author", "Title"}},
	})
	idx2 := operationAt[migrations.AddIndex](t, changes, "otherapp", 0, 2)
	djangotest.AssertEqual(t, idx2, migrations.AddIndex{
		Model: "Book",
		Index: migrations.Index{
			Name:   orm.IndexName("otherapp_book", []string{"-title"}),
			Fields: []string{"-Title"},
		},
	})
}

// Ported from Django: tests/migrations/test_operations.py::OperationTests.test_add_index
func TestSchemaEditorDescendingIndex(t *testing.T) {
	djangotest.EachDB(t, nil, func(t *testing.T, db *orm.DB) {
		ps := stateFromModels(t, "descapp", &metaStateAuthor{}, &metaStateBookWithOptions{})
		author := modelOf(t, ps, "descapp", "metaStateAuthor")
		book := modelOf(t, ps, "descapp", "metaStateBookWithOptions")
		state := applyOps(t, db, "descapp", migrations.NewProjectState(),
			migrations.CreateModel{Name: author.Name, Table: author.Table, Fields: author.Fields},
			migrations.CreateModel{
				Name:           book.Name,
				Table:          book.Table,
				Fields:         book.Fields,
				Indexes:        book.Indexes,
				UniqueTogether: book.UniqueTogether,
			},
		)
		djangotest.AssertIndexNameExists(t, db, "otherapp_book", "book_title_author_idx")
		djangotest.AssertIndexNameExists(t, db, "otherapp_book", orm.IndexName("otherapp_book", []string{"-title"}))
		djangotest.AssertUniqueConstraintExists(t, db, "otherapp_book", "author_id", "title")
		// Foreign key column gets Django's auto index on CreateModel.
		maxLen := 200
		if db.Dialect().Name() == "postgres" {
			maxLen = 63
		}
		fkIdx := orm.CreateIndexName("otherapp_book", []string{"author_id"}, "", maxLen)
		djangotest.AssertIndexExists(t, db, "otherapp_book", "author_id")
		djangotest.AssertIndexNameExists(t, db, "otherapp_book", fkIdx)
		_ = state
	})
}

// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_indexes
// Ported from Django: tests/migrations/test_autodetector.py::AutodetectorTests.test_remove_unique_together
func TestAutodetectorRemovesMetaOptionsFromRegistry(t *testing.T) {
	from := stateFromModels(t, "otherapp", &metaStateAuthor{}, &metaStateBookWithOptions{})
	to := stateFromModels(t, "otherapp", &metaStateAuthor{}, &metaStateBookPlain{})

	renameKey := func(ps *migrations.ProjectState, oldName, newName string) {
		ms := modelOf(t, ps, "otherapp", oldName)
		ps.RemoveModel("otherapp", oldName)
		ms.Name = newName
		ps.AddModel(ms)
	}
	renameKey(from, "metaStateBookWithOptions", "Book")
	renameKey(to, "metaStateBookPlain", "Book")

	detector := migrations.NewAutodetector(from, to, nil)
	changes, err := detector.Changes(nil, nil, "")
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	assertNumberMigrations(t, changes, "otherapp", 1)
	assertOperationTypes(t, changes, "otherapp", 0, "RemoveIndex", "RemoveIndex", "AlterUniqueTogether")
}
