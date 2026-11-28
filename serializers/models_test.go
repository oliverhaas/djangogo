package serializers_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/internal/pyjson"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// testFormats holds the serializer formats exercised by SerializersTestBase and
// SerializersTransactionTestBase tests.
var testFormats = []string{"json"}

// CategoryMetaData is Django's serializers.CategoryMetaData.
type CategoryMetaData struct {
	orm.RelCache
	ID    int64
	Kind  string `orm:"max_length=10"`
	Name  string `orm:"max_length=10"`
	Value string `orm:"max_length=10"`
}

// Meta sets the unique_together constraint on CategoryMetaData.
func (CategoryMetaData) Meta() orm.Meta {
	return orm.Meta{UniqueTogether: [][]string{{"kind", "name"}}}
}

// String formats CategoryMetaData as "[kind:name]=value".
func (c CategoryMetaData) String() string {
	return fmt.Sprintf("[%s:%s]=%s", c.Kind, c.Name, c.Value)
}

// NaturalKey returns the (kind, name) natural key of CategoryMetaData.
func (c CategoryMetaData) NaturalKey() []any {
	return []any{c.Kind, c.Name}
}

// GetByNaturalKey looks up a CategoryMetaData row by (kind, name).
func (*CategoryMetaData) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 2 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[CategoryMetaData](db).Get(ctx, "kind", key[0], "name", key[1])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Category is Django's serializers.Category.
type Category struct {
	orm.RelCache
	ID       int64
	Name     string                   `orm:"max_length=20"`
	MetaData orm.FK[CategoryMetaData] `orm:"null;on_delete=set_null"`
}

// Meta orders Category rows by name.
func (Category) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"name"}}
}

// String returns the category name.
func (c Category) String() string { return c.Name }

// Author is Django's serializers.Author.
type Author struct {
	orm.RelCache
	ID   int64
	Name string `orm:"max_length=20"`
}

// Meta orders Author rows by name.
func (Author) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"name"}}
}

// String returns the author name.
func (a Author) String() string { return a.Name }

// Topic is Django's serializers.Topic.
type Topic struct {
	orm.RelCache
	ID       int64
	Name     string           `orm:"max_length=255"`
	Category orm.FK[Category] `orm:"on_delete=cascade"`
}

// Article is Django's serializers.Article.
type Article struct {
	orm.RelCache
	ID         int64
	Author     orm.FK[Author] `orm:"on_delete=cascade"`
	Headline   string         `orm:"max_length=50"`
	PubDate    time.Time
	Categories orm.M2M[Category]
	MetaData   orm.M2M[CategoryMetaData]
	Topics     orm.M2M[Topic]
}

// Meta orders Article rows by pub_date.
func (Article) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"pub_date"}}
}

// String returns the article headline.
func (a Article) String() string { return a.Headline }

// AuthorProfile is Django's serializers.AuthorProfile.
type AuthorProfile struct {
	orm.RelCache
	Author      orm.O2O[Author] `orm:"pk;on_delete=cascade"`
	DateOfBirth orm.Date
}

// Score is Django's serializers.Score.
type Score struct {
	ID    int64
	Score float64
}

// Player is Django's serializers.Player with Team stored as a string field for
// primary-key and integer-field error message tests.
type Player struct {
	ID   int64
	Name string `orm:"max_length=50"`
	Rank int64
	Team string `orm:"max_length=100"`
}

// ComplexModel is Django's serializers.ComplexModel.
type ComplexModel struct {
	ID     int64
	Field1 string `orm:"max_length=10"`
	Field2 string `orm:"max_length=10"`
	Field3 string `orm:"max_length=10"`
}

// Anchor is Django's serializers.Anchor.
type Anchor struct {
	ID   int64
	Data string `orm:"max_length=30"`
}

// Meta orders Anchor rows by id.
func (Anchor) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"id"}}
}

// M2MData is Django's serializers.M2MData.
type M2MData struct {
	ID   int64
	Data orm.M2M[Anchor]
}

// serializerModels returns the test models registered under the "serializers" app.
func serializerModels() []any {
	return []any{djangotest.App("serializers",
		&CategoryMetaData{},
		&Category{},
		&Author{},
		&Topic{},
		&Article{},
		&AuthorProfile{},
		&Score{},
		&Player{},
		&ComplexModel{},
		&Anchor{},
		&M2MData{},
	)}
}

// baseTestData holds the rows created by SerializersTestBase.setUpTestData.
type baseTestData struct {
	sports *Category
	music  *Category
	opEd   *Category
	joe    *Author
	jane   *Author
	a1     *Article
	a2     *Article
}

// seedBaseTestData populates db with the rows of SerializersTestBase.setUpTestData.
func seedBaseTestData(t *testing.T, db *orm.DB) baseTestData {
	t.Helper()
	ctx := context.Background()

	sports := mustCreate(t, db, &Category{Name: "Sports"})
	music := mustCreate(t, db, &Category{Name: "Music"})
	opEd := mustCreate(t, db, &Category{Name: "Op-Ed"})

	joe := mustCreate(t, db, &Author{Name: "Joe"})
	jane := mustCreate(t, db, &Author{Name: "Jane"})

	a1 := mustCreate(t, db, &Article{
		Author:   fkTo[Author](jane.ID),
		Headline: "Poker has no place on ESPN",
		PubDate:  time.Date(2006, 6, 16, 11, 0, 0, 0, time.UTC),
	})
	if err := orm.Many[Category](db, a1, "categories").Set(ctx, []*Category{sports, opEd}); err != nil {
		t.Fatalf("Set a1 categories: %v", err)
	}

	a2 := mustCreate(t, db, &Article{
		Author:   fkTo[Author](joe.ID),
		Headline: "Time to reform copyright",
		PubDate:  time.Date(2006, 6, 16, 13, 0, 11, 345*1000, time.UTC),
	})
	if err := orm.Many[Category](db, a2, "categories").Set(ctx, []*Category{music, opEd}); err != nil {
		t.Fatalf("Set a2 categories: %v", err)
	}

	return baseTestData{
		sports: sports,
		music:  music,
		opEd:   opEd,
		joe:    joe,
		jane:   jane,
		a1:     a1,
		a2:     a2,
	}
}

// mustCreate inserts obj into db and fails the test on error.
func mustCreate[T any](t *testing.T, db *orm.DB, obj *T) *T {
	t.Helper()
	if err := orm.Query[T](db).Create(context.Background(), obj); err != nil {
		t.Fatalf("Create(%T): %v", obj, err)
	}
	return obj
}

// fkTo returns an orm.FK[T] with pk set.
func fkTo[T any](pk int64) orm.FK[T] {
	var f orm.FK[T]
	f.SetPK(pk)
	return f
}

// o2oTo returns an orm.O2O[T] with pk set.
func o2oTo[T any](pk int64) orm.O2O[T] {
	var o orm.O2O[T]
	o.SetPK(pk)
	return o
}

// collectDeserialized drains an iter.Seq2[*serializers.DeserializedObject, error]
// into a slice or returns the first error encountered.
func collectDeserialized(
	ctx context.Context,
	db *orm.DB,
	format string,
	data string,
	opts ...serializers.Option,
) ([]*serializers.DeserializedObject, error) {
	var out []*serializers.DeserializedObject
	for obj, err := range serializers.Deserialize(ctx, db, format, strings.NewReader(data), opts...) {
		if err != nil {
			return out, err
		}
		out = append(out, obj)
	}
	return out, nil
}

// validateOutput reports whether serialStr is valid serialized output for format.
func validateOutput(format, serialStr string) bool {
	switch format {
	case "json":
		_, err := pyjson.Decode([]byte(serialStr))
		return err == nil
	default:
		return false
	}
}

// getPKValues extracts the pk values from serialStr for format.
func getPKValues(t *testing.T, format, serialStr string) []any {
	t.Helper()
	switch format {
	case "json":
		v, err := pyjson.Decode([]byte(serialStr))
		if err != nil {
			t.Fatalf("Decode JSON: %v", err)
		}
		list, ok := v.([]any)
		if !ok {
			t.Fatalf("expected JSON list, got %T", v)
		}
		out := make([]any, 0, len(list))
		for _, item := range list {
			obj, ok := item.(pyjson.Object)
			if !ok {
				t.Fatalf("expected JSON object, got %T", item)
			}
			pk, _ := obj.Get("pk")
			out = append(out, pk)
		}
		return out
	default:
		t.Fatalf("unsupported format %q", format)
		return nil
	}
}

// getFieldValues extracts the values of fieldName across all objects in
// serialStr for format.
func getFieldValues(t *testing.T, format, serialStr, fieldName string) []any {
	t.Helper()
	switch format {
	case "json":
		v, err := pyjson.Decode([]byte(serialStr))
		if err != nil {
			t.Fatalf("Decode JSON: %v", err)
		}
		list, ok := v.([]any)
		if !ok {
			t.Fatalf("expected JSON list, got %T", v)
		}
		var out []any
		for _, item := range list {
			obj, ok := item.(pyjson.Object)
			if !ok {
				t.Fatalf("expected JSON object, got %T", item)
			}
			fieldsRaw, ok := obj.Get("fields")
			if !ok {
				continue
			}
			fieldsObj, ok := fieldsRaw.(pyjson.Object)
			if !ok {
				continue
			}
			if val, found := fieldsObj.Get(fieldName); found {
				out = append(out, val)
			}
		}
		return out
	default:
		t.Fatalf("unsupported format %q", format)
		return nil
	}
}
