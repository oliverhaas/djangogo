package orm_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// metaRelation mirrors Django's model_meta.Relation (default verbose names).
type metaRelation struct {
	ID int64
}

// metaPerson mirrors Django's model_meta.Person with explicit verbose_name.
type metaPerson struct {
	ID   int64
	Name string `orm:"max_length=10"`
}

// Meta sets verbose_name as in Django's model_meta.Person.
func (metaPerson) Meta() orm.Meta {
	return orm.Meta{VerboseName: "Person"}
}

// metaBlogPost exercises camel_case_to_spaces and explicit options.
type metaBlogPost struct {
	ID       int64
	Title    string `orm:"max_length=100"`
	Slug     string `orm:"max_length=100"`
	Category string `orm:"max_length=50"`
}

// Meta declares every supported model option.
func (metaBlogPost) Meta() orm.Meta {
	return orm.Meta{
		Table:             "custom_blog_posts",
		Ordering:          []string{"-title", "id"},
		UniqueTogether:    [][]string{{"category", "Slug"}},
		Indexes:           []orm.Index{{Fields: []string{"title", "-slug"}}, {Name: "blog_cat_idx", Fields: []string{"Category"}}},
		VerboseName:       "blog entry",
		VerboseNamePlural: "blog entries",
	}
}

// metaHTMLDocument checks acronym splitting in default VerboseName.
type metaHTMLDocument struct {
	ID int64
}

// Ported from Django: tests/model_meta/tests.py::LabelTests.test_label
func TestModelMetaLabel(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_meta", &metaRelation{}, &metaPerson{}))
	rel, _ := reg.ModelOf(&metaRelation{})
	person, _ := reg.ModelOf(&metaPerson{})
	djangotest.AssertEqual(t, rel.Label(), "model_meta.metaRelation")
	djangotest.AssertEqual(t, person.Label(), "model_meta.metaPerson")
}

// Ported from Django: tests/model_meta/tests.py::LabelTests.test_label_lower
func TestModelMetaLabelLower(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_meta", &metaRelation{}, &metaPerson{}))
	rel, _ := reg.ModelOf(&metaRelation{})
	person, _ := reg.ModelOf(&metaPerson{})
	djangotest.AssertEqual(t, rel.LabelLower(), "model_meta.metarelation")
	djangotest.AssertEqual(t, person.LabelLower(), "model_meta.metaperson")
}

// Ported from Django: tests/model_meta/tests.py::VerboseNameRawTests.test_string
func TestModelMetaVerboseNameDefault(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_meta", &metaRelation{}, &metaHTMLDocument{}))
	rel, _ := reg.ModelOf(&metaRelation{})
	doc, _ := reg.ModelOf(&metaHTMLDocument{})
	djangotest.AssertEqual(t, rel.VerboseName(), "meta relation")
	djangotest.AssertEqual(t, rel.VerboseNamePlural(), "meta relations")
	djangotest.AssertEqual(t, doc.VerboseName(), "meta html document")
	djangotest.AssertEqual(t, doc.VerboseNamePlural(), "meta html documents")
}

// Go has no gettext_lazy; the explicit VerboseName string stands in for it.
//
// Ported from Django: tests/model_meta/tests.py::VerboseNameRawTests.test_gettext
func TestModelMetaVerboseNameExplicit(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_meta", &metaPerson{}))
	person, _ := reg.ModelOf(&metaPerson{})
	djangotest.AssertEqual(t, person.VerboseName(), "Person")
	djangotest.AssertEqual(t, person.VerboseNamePlural(), "Persons")
}

// TestModelMetaAllOptions verifies table, ordering, unique_together, indexes,
// verbose names, field normalization, and defensive copying on Model.
func TestModelMetaAllOptions(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("blog", &metaBlogPost{}))
	m, _ := reg.ModelOf(&metaBlogPost{})

	djangotest.AssertEqual(t, m.Table(), "custom_blog_posts")
	djangotest.AssertEqual(t, m.VerboseName(), "blog entry")
	djangotest.AssertEqual(t, m.VerboseNamePlural(), "blog entries")
	djangotest.AssertEqual(t, m.Ordering(), []string{"-title", "id"})
	djangotest.AssertEqual(t, m.UniqueTogether(), [][]string{{"Category", "Slug"}})

	indexes := m.Indexes()
	djangotest.AssertEqual(t, len(indexes), 2)
	djangotest.AssertEqual(t, indexes[0].Fields, []string{"Title", "-Slug"})
	djangotest.AssertEqual(t, indexes[0].Name, orm.IndexName("custom_blog_posts", []string{"title", "-slug"}))
	djangotest.AssertEqual(t, indexes[1], orm.Index{Name: "blog_cat_idx", Fields: []string{"Category"}})

	// Mutating returned slices must not affect the frozen model descriptor.
	ord := m.Ordering()
	ord[0] = "mutated"
	djangotest.AssertEqual(t, m.Ordering()[0], "-title")

	ut := m.UniqueTogether()
	ut[0][0] = "mutated"
	djangotest.AssertEqual(t, m.UniqueTogether()[0][0], "Category")

	indexes[0].Fields[0] = "mutated"
	djangotest.AssertEqual(t, m.Indexes()[0].Fields[0], "Title")
}

// metaBadUniqueTogether references an unknown field in UniqueTogether.
type metaBadUniqueTogether struct {
	ID   int64
	Name string
}

// Meta declares an invalid unique_together field.
func (metaBadUniqueTogether) Meta() orm.Meta {
	return orm.Meta{UniqueTogether: [][]string{{"Name", "Missing"}}}
}

// metaBadIndexField references an unknown field in Indexes.
type metaBadIndexField struct {
	ID   int64
	Name string
}

// Meta declares an invalid index field.
func (metaBadIndexField) Meta() orm.Meta {
	return orm.Meta{Indexes: []orm.Index{{Fields: []string{"Missing"}}}}
}

// metaEmptyIndex declares an index with no fields.
type metaEmptyIndex struct {
	ID int64
}

// Meta declares an empty index.
func (metaEmptyIndex) Meta() orm.Meta {
	return orm.Meta{Indexes: []orm.Index{{Name: "empty_idx"}}}
}

// metaDupIndex declares two indexes with the same name.
type metaDupIndex struct {
	ID   int64
	Name string
}

// Meta declares duplicate index names.
func (metaDupIndex) Meta() orm.Meta {
	return orm.Meta{Indexes: []orm.Index{
		{Name: "same_idx", Fields: []string{"Name"}},
		{Name: "same_idx", Fields: []string{"ID"}},
	}}
}

// TestModelMetaValidationErrors verifies that invalid Meta options fail at
// registration.
func TestModelMetaValidationErrors(t *testing.T) {
	cases := []struct {
		name  string
		model any
		want  string
	}{
		{"unknown unique_together field", &metaBadUniqueTogether{}, "unknown field"},
		{"unknown index field", &metaBadIndexField{}, "unknown field"},
		{"empty index fields", &metaEmptyIndex{}, "at least one field"},
		{"duplicate index name", &metaDupIndex{}, "duplicate index name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := orm.NewRegistry()
			_, err := reg.Register(tc.model)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Register(%s): err = %v, want substring %q", tc.name, err, tc.want)
			}
		})
	}
}
