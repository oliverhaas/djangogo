package contenttypes_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// PrefetchBook is tests/prefetch_related/models.py::Book without the authors,
// which GenericRelationTests does not use.
type PrefetchBook struct {
	ID    int64
	Title string `orm:"max_length=255"`
	orm.RelCache
}

// Meta orders the books as Django's Book.Meta does.
func (PrefetchBook) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// PrefetchReader is tests/prefetch_related/models.py::Reader.
type PrefetchReader struct {
	ID        int64
	Name      string                `orm:"max_length=50"`
	BooksRead orm.M2M[PrefetchBook] `orm:"related_name=read_by"`
	orm.RelCache
}

// Meta orders the readers as Django's Reader.Meta does.
func (PrefetchReader) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// PrefetchTaggedItem is tests/prefetch_related/models.py::TaggedItem: a tag
// with three generic foreign keys, two of them nullable and one with a text
// object id.
type PrefetchTaggedItem struct {
	ID            int64
	Tag           string                           `orm:"type=slug"`
	ContentType   orm.FK[contenttypes.ContentType] `orm:"related_name=taggeditem_set2"`
	ObjectID      int64                            `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
	CreatedByCt   orm.FK[contenttypes.ContentType] `orm:"null;on_delete=set_null;related_name=taggeditem_set3"`
	CreatedByFkey *int64                           `orm:"type=positive_int"`
	CreatedBy     contenttypes.GenericForeignKey   `orm:"ct_field=created_by_ct;fk_field=created_by_fkey"`
	FavoriteCt    orm.FK[contenttypes.ContentType] `orm:"null;on_delete=set_null;related_name=taggeditem_set4"`
	FavoriteFkey  *string                          `orm:"max_length=64"`
	Favorite      contenttypes.GenericForeignKey   `orm:"ct_field=favorite_ct;fk_field=favorite_fkey"`
	orm.RelCache
}

// Meta orders the tags as Django's TaggedItem.Meta does.
func (PrefetchTaggedItem) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// PrefetchArticle is tests/prefetch_related/models.py::Article, a model with
// a UUID primary key.
type PrefetchArticle struct {
	ID   orm.UUID `orm:"pk;default=uuid4"`
	Name string   `orm:"max_length=20"`
}

// PrefetchBookmark is tests/prefetch_related/models.py::Bookmark: two generic
// relations to the tags, one of them through the text object id.
type PrefetchBookmark struct {
	ID           int64
	URL          string                                           `orm:"type=url"`
	Tags         contenttypes.GenericRelation[PrefetchTaggedItem] `orm:"related_query_name=bookmarks"`
	FavoriteTags contenttypes.GenericRelation[PrefetchTaggedItem] `orm:"content_type_field=favorite_ct;object_id_field=favorite_fkey;related_query_name=favorite_bookmarks"`
	orm.RelCache
}

// Meta orders the bookmarks as Django's Bookmark.Meta does.
func (PrefetchBookmark) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// PrefetchComment is tests/prefetch_related/models.py::Comment: two generic
// foreign keys with text object ids.
type PrefetchComment struct {
	ID                int64
	Comment           string                           `orm:"type=text"`
	ContentType       orm.FK[contenttypes.ContentType] `orm:"null"`
	ObjectPK          string                           `orm:"type=text"`
	ContentObject     contenttypes.GenericForeignKey   `orm:"fk_field=object_pk"`
	ContentTypeUUID   orm.FK[contenttypes.ContentType] `orm:"null;related_name=comments"`
	ObjectPkUUID      string                           `orm:"type=text"`
	ContentObjectUUID contenttypes.GenericForeignKey   `orm:"ct_field=content_type_uuid;fk_field=object_pk_uuid"`
}

// Meta orders the comments as Django's Comment.Meta does.
func (PrefetchComment) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

// prefetchModels is the registry of the ported GenericRelationTests.
func prefetchModels() []any {
	return []any{
		contenttypes.App{},
		djangotest.App("prefetch_related",
			&PrefetchBook{}, &PrefetchReader{}, &PrefetchTaggedItem{}, &PrefetchArticle{},
			&PrefetchBookmark{}, &PrefetchComment{}),
	}
}

// prefetchData holds the rows of GenericRelationTests.setUpTestData.
type prefetchData struct {
	book1, book2, book3       *PrefetchBook
	reader1, reader2, reader3 *PrefetchReader
}

// prefetchSetUp creates the rows of GenericRelationTests.setUpTestData.
func prefetchSetUp(t *testing.T, db *orm.DB) prefetchData {
	t.Helper()
	ctx := context.Background()
	var d prefetchData
	d.book1 = mustCreate(t, db, &PrefetchBook{Title: "Winnie the Pooh"})
	d.book2 = mustCreate(t, db, &PrefetchBook{Title: "Do you like green eggs and spam?"})
	d.book3 = mustCreate(t, db, &PrefetchBook{Title: "Three Men In A Boat"})
	d.reader1 = mustCreate(t, db, &PrefetchReader{Name: "me"})
	d.reader2 = mustCreate(t, db, &PrefetchReader{Name: "you"})
	d.reader3 = mustCreate(t, db, &PrefetchReader{Name: "someone"})
	for _, add := range []struct {
		book    *PrefetchBook
		readers []*PrefetchReader
	}{{d.book1, []*PrefetchReader{d.reader1, d.reader2}}, {d.book2, []*PrefetchReader{d.reader2}}, {d.book3, []*PrefetchReader{d.reader3}}} {
		if err := orm.Many[PrefetchReader](db, add.book, "read_by").Add(ctx, add.readers...); err != nil {
			t.Fatalf("add readers of %q: %v", add.book.Title, err)
		}
	}
	return d
}

// createPrefetchTag creates the tag tag with the generic foreign keys that
// objs names, given as name and object pairs (Django's
// TaggedItem.objects.create(tag=tag, content_object=obj, ...)).
func createPrefetchTag(t *testing.T, db *orm.DB, tag string, objs ...any) *PrefetchTaggedItem {
	t.Helper()
	item := &PrefetchTaggedItem{Tag: tag}
	for i := 0; i+1 < len(objs); i += 2 {
		setObject(t, db, item, objs[i].(string), objs[i+1])
	}
	return mustCreate(t, db, item)
}

// prefetchTags returns the tags of the GenericRelation name of bookmark in
// order.
func prefetchTags(t *testing.T, db *orm.DB, bookmark *PrefetchBookmark, name string) []string {
	t.Helper()
	items, err := orm.GenericSet[PrefetchTaggedItem](db, bookmark, name).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tags := make([]string, len(items))
	for i, item := range items {
		tags[i] = item.Tag
	}
	return tags
}

// assertSameObjects asserts that got and want hold the same objects, with nil
// for an instance without one.
func assertSameObjects(t *testing.T, got, want []any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d objects, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] == nil || want[i] == nil {
			if got[i] != want[i] {
				t.Fatalf("object %d: %#v, want %#v", i, got[i], want[i])
			}
			continue
		}
		assertSameObject(t, got[i], want[i])
	}
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_prefetch_GFK
func TestPrefetchGFK(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := prefetchSetUp(t, db)
		createPrefetchTag(t, db, "awesome", "content_object", d.book1)
		createPrefetchTag(t, db, "great", "content_object", d.reader1)
		createPrefetchTag(t, db, "outstanding", "content_object", d.book2)
		createPrefetchTag(t, db, "amazing", "content_object", d.reader3)
		// One query for the tags, one for the books and one for the readers.
		djangotest.AssertNumQueries(t, db, 3, func() {
			if _, err := orm.Query[PrefetchTaggedItem](db).PrefetchRelated("content_object").All(ctx); err != nil {
				t.Fatal(err)
			}
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_prefetch_GFK_nonint_pk
func TestPrefetchGFKNonintPK(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := prefetchSetUp(t, db)
		comment := &PrefetchComment{Comment: "awesome"}
		setObject(t, db, comment, "content_object", d.book1)
		mustCreate(t, db, comment)
		// One query for the comments and one for the books.
		var objs []any
		djangotest.AssertNumQueries(t, db, 2, func() {
			comments, err := orm.Query[PrefetchComment](db).PrefetchRelated("content_object").All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for i := range comments {
				objs = append(objs, getObject(t, db, &comments[i], "content_object"))
			}
		})
		assertSameObjects(t, objs, []any{d.book1})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_prefetch_GFK_uuid_pk
func TestPrefetchGFKUUIDPK(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prefetchSetUp(t, db)
		article := mustCreate(t, db, &PrefetchArticle{Name: "Django"})
		comment := &PrefetchComment{Comment: "awesome"}
		setObject(t, db, comment, "content_object_uuid", article)
		mustCreate(t, db, comment)
		comments, err := orm.Query[PrefetchComment](db).PrefetchRelated("content_object_uuid").All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		objs := make([]any, 0, len(comments))
		for i := range comments {
			objs = append(objs, getObject(t, db, &comments[i], "content_object_uuid"))
		}
		assertSameObjects(t, objs, []any{article})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_traverse_GFK
func TestTraverseGFK(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := prefetchSetUp(t, db)
		for _, obj := range []any{d.book1, d.book2, d.book3, d.reader1, d.reader2} {
			createPrefetchTag(t, db, "awesome", "content_object", obj)
		}
		ct, err := contenttypes.GetForModel(ctx, db, mustModel(t, db, PrefetchBook{}))
		if err != nil {
			t.Fatal(err)
		}
		// One query for the tags, one for the books, which share a table, and
		// one for the read_by relation. The filter on the content type makes
		// every object a book, which has read_by.
		readers := map[string]bool{}
		djangotest.AssertNumQueries(t, db, 3, func() {
			tags, err := orm.Query[PrefetchTaggedItem](db).Filter("content_type", ct.ID, "tag", "awesome").
				PrefetchRelated("content_object__read_by").All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for i := range tags {
				book := getObject(t, db, &tags[i], "content_object").(*PrefetchBook)
				rs, err := orm.Related[PrefetchReader](db, book, "read_by").All(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range rs {
					readers[r.Name] = true
				}
			}
		})
		djangotest.AssertEqual(t, readers, map[string]bool{"me": true, "you": true, "someone": true})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_nullable_GFK
func TestNullableGFK(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := prefetchSetUp(t, db)
		createPrefetchTag(t, db, "awesome", "content_object", d.book1, "created_by", d.reader1)
		createPrefetchTag(t, db, "great", "content_object", d.book2)
		createPrefetchTag(t, db, "rubbish", "content_object", d.book3)
		var result []any
		djangotest.AssertNumQueries(t, db, 2, func() {
			tags, err := orm.Query[PrefetchTaggedItem](db).PrefetchRelated("created_by").All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for i := range tags {
				result = append(result, getObject(t, db, &tags[i], "created_by"))
			}
		})
		tags, err := orm.Query[PrefetchTaggedItem](db).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := make([]any, 0, len(tags))
		for i := range tags {
			want = append(want, getObject(t, db, &tags[i], "created_by"))
		}
		assertSameObjects(t, result, want)
		assertSameObjects(t, result, []any{d.reader1, nil, nil})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_generic_relation
func TestPrefetchGenericRelation(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prefetchSetUp(t, db)
		bookmark := mustCreate(t, db, &PrefetchBookmark{URL: "http://www.djangoproject.com/"})
		createPrefetchTag(t, db, "django", "content_object", bookmark)
		createPrefetchTag(t, db, "python", "content_object", bookmark)
		djangotest.AssertNumQueries(t, db, 2, func() {
			bookmarks, err := orm.Query[PrefetchBookmark](db).PrefetchRelated("tags").All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			tags := make([]string, 0, 2)
			for i := range bookmarks {
				tags = append(tags, prefetchTags(t, db, &bookmarks[i], "tags")...)
			}
			slices.Sort(tags)
			djangotest.AssertEqual(t, tags, []string{"django", "python"})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_charfield_GFK
func TestCharfieldGFK(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prefetchSetUp(t, db)
		b := mustCreate(t, db, &PrefetchBookmark{URL: "http://www.djangoproject.com/"})
		createPrefetchTag(t, db, "django", "content_object", b)
		createPrefetchTag(t, db, "python", "content_object", b, "favorite", b)
		djangotest.AssertNumQueries(t, db, 3, func() {
			bookmark, err := orm.Query[PrefetchBookmark](db).Filter("pk", b.ID).
				PrefetchRelated("tags", "favorite_tags").First(ctx)
			if err != nil {
				t.Fatal(err)
			}
			tags := prefetchTags(t, db, &bookmark, "tags")
			slices.Sort(tags)
			djangotest.AssertEqual(t, tags, []string{"django", "python"})
			djangotest.AssertEqual(t, prefetchTags(t, db, &bookmark, "favorite_tags"), []string{"python"})
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_custom_queryset
//
// Django's prefetch caches a queryset that carries the custom filters, so
// bookmark.tags.all().all() returns the same rows. Go caches the rows: the
// QuerySet of the manager serves them until it is refined.
func TestPrefetchGenericRelationCustomQuerySet(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prefetchSetUp(t, db)
		bookmark := mustCreate(t, db, &PrefetchBookmark{URL: "http://www.djangoproject.com/"})
		djangoTag := createPrefetchTag(t, db, "django", "content_object", bookmark)
		createPrefetchTag(t, db, "python", "content_object", bookmark)
		var got PrefetchBookmark
		djangotest.AssertNumQueries(t, db, 2, func() {
			var err error
			got, err = orm.Query[PrefetchBookmark](db).PrefetchRelated(orm.Prefetch{
				Lookup:   "tags",
				QuerySet: orm.Query[PrefetchTaggedItem](db).Filter("tag", "django"),
			}).Get(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})
		tagsQS := orm.GenericSet[PrefetchTaggedItem](db, &got, "tags").QuerySet()
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertQuerySetEqual(t, tagsQS, []int64{djangoTag.ID},
				func(item PrefetchTaggedItem) int64 { return item.ID })
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_deleted_GFK
func TestDeletedGFK(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := prefetchSetUp(t, db)
		createPrefetchTag(t, db, "awesome", "content_object", d.book1)
		createPrefetchTag(t, db, "awesome", "content_object", d.book2)
		ct, err := contenttypes.GetForModel(ctx, db, mustModel(t, db, PrefetchBook{}))
		if err != nil {
			t.Fatal(err)
		}
		book1PK := d.book1.ID
		if _, err := orm.Query[PrefetchBook](db).Filter("pk", book1PK).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		type row struct {
			objectID, contentTypeID int64
			obj                     any
		}
		var result []row
		djangotest.AssertNumQueries(t, db, 2, func() {
			tags, err := orm.Query[PrefetchTaggedItem](db).Filter("tag", "awesome").
				PrefetchRelated("content_object").All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for i := range tags {
				result = append(result, row{tags[i].ObjectID, tags[i].ContentType.PK(), getObject(t, db, &tags[i], "content_object")})
			}
		})
		if len(result) != 2 {
			t.Fatalf("%d tags, want 2", len(result))
		}
		djangotest.AssertEqual(t, [2]int64{result[0].objectID, result[0].contentTypeID}, [2]int64{book1PK, ct.ID})
		djangotest.AssertEqual(t, [2]int64{result[1].objectID, result[1].contentTypeID}, [2]int64{d.book2.ID, ct.ID})
		assertSameObjects(t, []any{result[0].obj, result[1].obj}, []any{nil, d.book2})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::GenericRelationTests.test_reverse_generic_relation
func TestReverseGenericRelation(t *testing.T) {
	djangotest.EachDB(t, prefetchModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prefetchSetUp(t, db)
		// Create two bookmarks so that the primary keys of the bookmarks and
		// the tags differ.
		first := mustCreate(t, db, &PrefetchBookmark{})
		second := mustCreate(t, db, &PrefetchBookmark{})
		createPrefetchTag(t, db, "", "content_object", first, "favorite", second)
		var obj PrefetchTaggedItem
		djangotest.AssertNumQueries(t, db, 2, func() {
			var err error
			if obj, err = orm.Query[PrefetchTaggedItem](db).PrefetchRelated("favorite_bookmarks").Get(ctx); err != nil {
				t.Fatal(err)
			}
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertQuerySetEqual(t, orm.Related[PrefetchBookmark](db, &obj, "favorite_bookmarks"),
				[]int64{second.ID}, func(b PrefetchBookmark) int64 { return b.ID })
		})
	})
}

// mustModel returns the registered model of v.
func mustModel(t *testing.T, db *orm.DB, v any) *orm.Model {
	t.Helper()
	m, ok := db.Registry().ModelOf(v)
	if !ok {
		t.Fatalf("model of %s is not registered", reflect.TypeOf(v))
	}
	return m
}

// No Go counterpart: tests/prefetch_related/tests.py::GenericRelationTests.test_prefetch_GFK_uses_prepped_primary_key
// subclasses UUIDField to override get_prep_value; Go fields have no
// per-field prep hook.
//
// No Go counterpart: tests/prefetch_related/tests.py::GenericRelationTests.test_prefetch_GFK_fk_pk
// points a generic foreign key at BookWithYear, a multi-table inheritance
// child, and the orm has no model inheritance.
