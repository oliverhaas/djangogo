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
var testFormats = []string{"json", "jsonl"}

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

// NaturalKeyAnchor is Django's serializers.NaturalKeyAnchor.
type NaturalKeyAnchor struct {
	orm.RelCache
	ID    int64
	Data  string  `orm:"max_length=100;unique"`
	Title *string `orm:"max_length=100;null"`
}

// NaturalKey returns the single-element natural key (data,) of NaturalKeyAnchor.
func (n NaturalKeyAnchor) NaturalKey() []any {
	return []any{n.Data}
}

// GetByNaturalKey looks up a NaturalKeyAnchor row by data.
func (*NaturalKeyAnchor) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[NaturalKeyAnchor](db).Get(ctx, "data", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// FKDataNaturalKey is Django's serializers.FKDataNaturalKey.
type FKDataNaturalKey struct {
	orm.RelCache
	ID   int64
	Data orm.FK[NaturalKeyAnchor] `orm:"null;on_delete=set_null"`
}

// NaturalKeyThing is Django's serializers.NaturalKeyThing.
type NaturalKeyThing struct {
	orm.RelCache
	ID          int64
	Key         string                   `orm:"max_length=100;unique"`
	OtherThing  orm.FK[NaturalKeyThing]  `orm:"null;on_delete=cascade"`
	OtherThings orm.M2M[NaturalKeyThing] `orm:"related_name=thing_m2m_set;symmetrical=false"`
}

// String returns the key of NaturalKeyThing.
func (n NaturalKeyThing) String() string { return n.Key }

// NaturalKey returns the single-element natural key (key,) of NaturalKeyThing.
func (n NaturalKeyThing) NaturalKey() []any {
	return []any{n.Key}
}

// GetByNaturalKey looks up a NaturalKeyThing row by key.
func (*NaturalKeyThing) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[NaturalKeyThing](db).Get(ctx, "key", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// NaturalPKWithDefault is Django's serializers.NaturalPKWithDefault.
type NaturalPKWithDefault struct {
	orm.RelCache
	ID   orm.UUID `orm:"pk;default=uuid4;editable=false"`
	Name string   `orm:"max_length=100;unique"`
}

// NaturalKey returns the single-element natural key (name,) of NaturalPKWithDefault.
func (n NaturalPKWithDefault) NaturalKey() []any {
	return []any{n.Name}
}

// GetByNaturalKey looks up a NaturalPKWithDefault row by name.
func (*NaturalPKWithDefault) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[NaturalPKWithDefault](db).Get(ctx, "name", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
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
		&NaturalKeyAnchor{},
		&FKDataNaturalKey{},
		&NaturalKeyThing{},
		&NaturalPKWithDefault{},
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
	case "jsonl":
		for _, line := range strings.Split(serialStr, "\n") {
			if line == "" {
				continue
			}
			if _, err := pyjson.Decode([]byte(line)); err != nil {
				return false
			}
		}
		return true
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
	case "jsonl":
		var out []any
		for _, line := range strings.Split(serialStr, "\n") {
			if line == "" {
				continue
			}
			v, err := pyjson.Decode([]byte(line))
			if err != nil {
				t.Fatalf("Decode JSONL: %v", err)
			}
			obj, ok := v.(pyjson.Object)
			if !ok {
				t.Fatalf("expected JSON object, got %T", v)
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
	case "jsonl":
		var out []any
		for _, line := range strings.Split(serialStr, "\n") {
			if line == "" {
				continue
			}
			v, err := pyjson.Decode([]byte(line))
			if err != nil {
				t.Fatalf("Decode JSONL: %v", err)
			}
			obj, ok := v.(pyjson.Object)
			if !ok {
				t.Fatalf("expected JSON object, got %T", v)
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

// pkLessSerializedString returns the format-specific pkless fixture string for
// TestSerializersPKLessSerializedStrings.
func pkLessSerializedString(t *testing.T, format string) string {
	t.Helper()
	switch format {
	case "json":
		return `[
    {
        "pk": null,
        "model": "serializers.category",
        "fields": {"name": "Reference"}
    }, {
        "model": "serializers.category",
        "fields": {"name": "Non-fiction"}
    }]`
	case "jsonl":
		return "{\"pk\": null,\"model\": \"serializers.category\",\"fields\": {\"name\": \"Reference\"}}\n" +
			"{\"model\": \"serializers.category\",\"fields\": {\"name\": \"Non-fiction\"}}"
	default:
		t.Fatalf("unsupported format %q", format)
		return ""
	}
}

// mappingOrderingTemplate returns the format-specific expected output template
// for TestSerializersDeterministicMappingOrdering.
func mappingOrderingTemplate(t *testing.T, format string) string {
	t.Helper()
	switch format {
	case "json":
		return `[
{
  "model": "serializers.article",
  "pk": %d,
  "fields": {
    "author": %d,
    "headline": "Poker has no place on ESPN",
    "pub_date": "2006-06-16T11:00:00Z",
    "categories": [
      %d,
      %d
    ],
    "meta_data": [],
    "topics": []
  }
}
]
`
	case "jsonl":
		return "{\"model\": \"serializers.article\",\"pk\": %d," +
			"\"fields\": {" +
			"\"author\": %d," +
			"\"headline\": \"Poker has no place on ESPN\"," +
			"\"pub_date\": \"2006-06-16T11:00:00Z\"," +
			"\"categories\": [%d,%d]," +
			"\"meta_data\": []," +
			"\"topics\": []}}\n"
	default:
		t.Fatalf("unsupported format %q", format)
		return ""
	}
}

// forwardRefsSerializedString returns the format-specific fixture string for
// TestSerializersForwardRefs.
func forwardRefsSerializedString(t *testing.T, format string) string {
	t.Helper()
	switch format {
	case "json":
		return `[
    {
        "pk": 1,
        "model": "serializers.article",
        "fields": {
            "headline": "Forward references pose no problem",
            "pub_date": "2006-06-16T15:00:00",
            "categories": [1],
            "author": 1
        }
    },
    {
        "pk": 1,
        "model": "serializers.category",
        "fields": {
            "name": "Reference"
        }
    },
    {
        "pk": 1,
        "model": "serializers.author",
        "fields": {
            "name": "Agnes"
        }
    }]`
	case "jsonl":
		return "{\"pk\": 1,\"model\": \"serializers.article\",\"fields\": {\"headline\": \"Forward references pose no problem\",\"pub_date\": \"2006-06-16T15:00:00\",\"categories\": [1],\"author\": 1}}\n" +
			"{\"pk\": 1,\"model\": \"serializers.category\",\"fields\": {\"name\": \"Reference\"}}\n" +
			"{\"pk\": 1,\"model\": \"serializers.author\",\"fields\": {\"name\": \"Agnes\"}}"
	default:
		t.Fatalf("unsupported format %q", format)
		return ""
	}
}
