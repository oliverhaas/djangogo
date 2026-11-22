package contenttypes_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// TaggedItem is tests/generic_relations/models.py::TaggedItem: a tag on any
// object.
type TaggedItem struct {
	ID            int64
	Tag           string `orm:"type=slug"`
	ContentType   orm.FK[contenttypes.ContentType]
	ObjectID      int64 `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// Meta orders the tags as Django's TaggedItem.Meta does.
func (TaggedItem) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"tag", "content_type__model"}}
}

// Comparison is tests/generic_relations/models.py::Comparison with the fields
// of AbstractComparison: a model with two generic foreign keys.
type Comparison struct {
	ID           int64
	Comparative  string                           `orm:"max_length=50"`
	ContentType1 orm.FK[contenttypes.ContentType] `orm:"related_name=comparative1_set"`
	ObjectID1    int64                            `orm:"type=positive_int"`
	FirstObj     contenttypes.GenericForeignKey   `orm:"ct_field=content_type1;fk_field=object_id1"`
	ContentType2 orm.FK[contenttypes.ContentType] `orm:"related_name=comparative2_set"`
	ObjectID2    int64                            `orm:"type=positive_int"`
	OtherObj     contenttypes.GenericForeignKey   `orm:"ct_field=content_type2;fk_field=object_id2"`
}

// Animal is tests/generic_relations/models.py::Animal.
type Animal struct {
	ID          int64
	CommonName  string                                   `orm:"max_length=150"`
	LatinName   string                                   `orm:"max_length=150"`
	Tags        contenttypes.GenericRelation[TaggedItem] `orm:"related_query_name=animal"`
	Comparisons contenttypes.GenericRelation[Comparison] `orm:"object_id_field=object_id1;content_type_field=content_type1"`
	orm.RelCache
}

// Vegetable is tests/generic_relations/models.py::Vegetable.
type Vegetable struct {
	ID      int64
	Name    string `orm:"max_length=150"`
	IsYucky bool   `orm:"default=true"`
	Tags    contenttypes.GenericRelation[TaggedItem]
	orm.RelCache
}

// Mineral is tests/generic_relations/models.py::Mineral. It has no generic
// relation.
type Mineral struct {
	ID       int64
	Name     string `orm:"max_length=150"`
	Hardness int    `orm:"type=positive_smallint"`
}

// ManualPK is tests/generic_relations/models.py::ManualPK.
type ManualPK struct {
	ID   int64                                    `orm:"pk"`
	Tags contenttypes.GenericRelation[TaggedItem] `orm:"related_query_name=manualpk"`
}

// AllowsNullGFK is tests/generic_relations/models.py::AllowsNullGFK.
type AllowsNullGFK struct {
	ID            int64
	ContentType   orm.FK[contenttypes.ContentType] `orm:"null;on_delete=set_null"`
	ObjectID      *int64                           `orm:"type=positive_int"`
	ContentObject contenttypes.GenericForeignKey
}

// genericModels is the registry of the ported generic relation tests.
func genericModels() []any {
	return []any{
		contenttypes.App{},
		djangotest.App("generic_relations",
			&TaggedItem{}, &Comparison{}, &Animal{}, &Vegetable{}, &Mineral{}, &ManualPK{}, &AllowsNullGFK{}),
	}
}

// genericData holds the rows of GenericRelationsTests.setUpTestData.
type genericData struct {
	lion, platypus              *Animal
	bacon                       *Vegetable
	quartz                      *Mineral
	fatty, salty, yellow, hairy *TaggedItem
}

// genericSetUp creates the rows of GenericRelationsTests.setUpTestData.
func genericSetUp(t *testing.T, db *orm.DB) genericData {
	t.Helper()
	var d genericData
	d.lion = mustCreate(t, db, &Animal{CommonName: "Lion", LatinName: "Panthera leo"})
	d.platypus = mustCreate(t, db, &Animal{CommonName: "Platypus", LatinName: "Ornithorhynchus anatinus"})
	mustCreate(t, db, &Vegetable{Name: "Eggplant", IsYucky: true})
	d.bacon = mustCreate(t, db, &Vegetable{Name: "Bacon", IsYucky: false})
	d.quartz = mustCreate(t, db, &Mineral{Name: "Quartz", Hardness: 7})
	d.fatty = createTag(t, db, d.bacon, "fatty")
	d.salty = createTag(t, db, d.bacon, "salty")
	d.yellow = createTag(t, db, d.lion, "yellow")
	d.hairy = createTag(t, db, d.lion, "hairy")
	return d
}

// tagsOf returns the manager of the tags of owner (Django's owner.tags).
func tagsOf(db *orm.DB, owner any) *orm.GenericRelatedManager[TaggedItem] {
	return orm.GenericSet[TaggedItem](db, owner, "tags")
}

// comparisonsOf returns the manager of the comparisons of owner.
func comparisonsOf(db *orm.DB, owner any) *orm.GenericRelatedManager[Comparison] {
	return orm.GenericSet[Comparison](db, owner, "comparisons")
}

// createTag creates the tag named tag on owner through its tags manager
// (Django's owner.tags.create(tag=tag)).
func createTag(t *testing.T, db *orm.DB, owner any, tag string) *TaggedItem {
	t.Helper()
	item := &TaggedItem{Tag: tag}
	if err := tagsOf(db, owner).Create(context.Background(), item); err != nil {
		t.Fatalf("create tag %q: %v", tag, err)
	}
	return item
}

// newTag returns an unsaved tag on obj (Django's TaggedItem(content_object=obj,
// tag=tag)).
func newTag(t *testing.T, db *orm.DB, obj any, tag string) *TaggedItem {
	t.Helper()
	item := &TaggedItem{Tag: tag}
	setObject(t, db, item, "content_object", obj)
	return item
}

// createTagFor creates a tag on obj through the generic foreign key (Django's
// TaggedItem.objects.create(content_object=obj, tag=tag)).
func createTagFor(t *testing.T, db *orm.DB, obj any, tag string) *TaggedItem {
	t.Helper()
	return mustCreate(t, db, newTag(t, db, obj, tag))
}

// setObject assigns obj to the generic foreign key name of instance.
func setObject(t *testing.T, db *orm.DB, instance any, name string, obj any) {
	t.Helper()
	if err := contenttypes.SetObject(context.Background(), db, instance, name, obj); err != nil {
		t.Fatalf("SetObject(%s): %v", name, err)
	}
}

// getObject returns the object of the generic foreign key name of instance.
func getObject(t *testing.T, db *orm.DB, instance any, name string) any {
	t.Helper()
	obj, err := contenttypes.GetObject(context.Background(), db, instance, name)
	if err != nil {
		t.Fatalf("GetObject(%s): %v", name, err)
	}
	return obj
}

// assertSameObject asserts that got and want are instances of the same model
// with the same primary key, Django's model instance equality.
func assertSameObject(t *testing.T, got, want any) {
	t.Helper()
	gv, wv := reflect.ValueOf(got), reflect.ValueOf(want)
	if !gv.IsValid() || !wv.IsValid() || gv.Type() != wv.Type() || (gv.Kind() == reflect.Pointer && gv.IsNil()) {
		t.Fatalf("object %#v, want %#v", got, want)
	}
	gid := reflect.Indirect(gv).FieldByName("ID").Interface()
	wid := reflect.Indirect(wv).FieldByName("ID").Interface()
	if gid != wid {
		t.Fatalf("object %T with ID %v, want ID %v", got, gid, wid)
	}
}

// ids returns the primary keys of items.
func ids(items ...*TaggedItem) []int64 {
	out := make([]int64, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

// tagID maps a tag to its primary key, for AssertQuerySetEqual.
func tagID(item TaggedItem) int64 { return item.ID }

// assertTags asserts that the tags manager of owner holds want, in order.
func assertTags(t *testing.T, db *orm.DB, owner any, want ...*TaggedItem) {
	t.Helper()
	djangotest.AssertQuerySetEqual(t, tagsOf(db, owner).QuerySet(), ids(want...), tagID)
}

// countTags returns the number of tags of owner.
func countTags(t *testing.T, db *orm.DB, owner any) int64 {
	t.Helper()
	n, err := tagsOf(db, owner).Count(context.Background())
	if err != nil {
		t.Fatalf("count tags: %v", err)
	}
	return n
}

// tagRow is GenericRelationsTests.comp_func: the tag, the name of the model
// of its content type and its object id.
type tagRow struct {
	tag      string
	model    string
	objectID int64
}

// compFunc returns the comp_func transform of GenericRelationsTests.
func compFunc(t *testing.T, db *orm.DB) func(TaggedItem) tagRow {
	return func(item TaggedItem) tagRow {
		ct, err := contenttypes.GetForID(context.Background(), db, item.ContentType.PK())
		if err != nil {
			t.Fatalf("content type of %q: %v", item.Tag, err)
		}
		m, ok := contenttypes.ModelClass(db, ct)
		if !ok {
			t.Fatalf("model of content type %s", ct)
		}
		return tagRow{item.Tag, m.Name(), item.ObjectID}
	}
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_generic_update_or_create_when_created
func TestGenericUpdateOrCreateWhenCreated(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		count := countTags(t, db, d.bacon)
		_, created, err := tagsOf(db, d.bacon).UpdateOrCreate(ctx, nil, "tag", "stinky")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, countTags(t, db, d.bacon), count+1)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_generic_update_or_create_when_updated
func TestGenericUpdateOrCreateWhenUpdated(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		count := countTags(t, db, d.bacon)
		tag := createTag(t, db, d.bacon, "stinky")
		djangotest.AssertEqual(t, countTags(t, db, d.bacon), count+1)
		got, created, err := tagsOf(db, d.bacon).UpdateOrCreate(ctx, orm.Defaults{"tag": "juicy"}, "id", tag.ID)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, countTags(t, db, d.bacon), count+1)
		djangotest.AssertEqual(t, got.Tag, "juicy")
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_generic_get_or_create_when_created
func TestGenericGetOrCreateWhenCreated(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		count := countTags(t, db, d.bacon)
		_, created, err := tagsOf(db, d.bacon).GetOrCreate(ctx, nil, "tag", "stinky")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, countTags(t, db, d.bacon), count+1)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_generic_get_or_create_when_exists
func TestGenericGetOrCreateWhenExists(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		count := countTags(t, db, d.bacon)
		tag := createTag(t, db, d.bacon, "stinky")
		djangotest.AssertEqual(t, countTags(t, db, d.bacon), count+1)
		got, created, err := tagsOf(db, d.bacon).GetOrCreate(ctx, orm.Defaults{"tag": "juicy"}, "id", tag.ID)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, countTags(t, db, d.bacon), count+1)
		// The existing tag is returned unchanged.
		djangotest.AssertEqual(t, got.Tag, "stinky")
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_generic_relations_m2m_mimic
func TestGenericRelationsM2MMimic(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		assertTags(t, db, d.lion, d.hairy, d.yellow)
		assertTags(t, db, d.bacon, d.fatty, d.salty)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_access_content_object
func TestAccessContentObject(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		item, err := orm.Query[TaggedItem](db).Get(context.Background(), "tag", "salty")
		if err != nil {
			t.Fatal(err)
		}
		assertSameObject(t, getObject(t, db, &item, "content_object"), d.bacon)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_query_content_object
func TestQueryContentObject(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		qs := orm.Query[TaggedItem](db).Filter("animal__isnull", false).OrderBy("animal__common_name", "tag")
		djangotest.AssertQuerySetEqual(t, qs, ids(d.hairy, d.yellow), tagID)

		mpk := mustCreate(t, db, &ManualPK{ID: 1})
		createTag(t, db, mpk, "mpk")
		qs = orm.Query[TaggedItem](db).
			Filter(orm.Q("animal__isnull", false).Or(orm.Q("manualpk__id", 1))).
			OrderBy("tag")
		djangotest.AssertQuerySetEqual(t, qs, []string{"hairy", "mpk", "yellow"},
			func(item TaggedItem) string { return item.Tag })
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_exclude_generic_relations
func TestExcludeGenericRelations(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		shiny := createTagFor(t, db, d.quartz, "shiny")
		clearish := createTagFor(t, db, d.quartz, "clearish")

		ctype := mustGet(t, db, d.quartz)
		qs := orm.Query[TaggedItem](db).Filter("content_type__pk", ctype.ID, "object_id", d.quartz.ID)
		djangotest.AssertQuerySetEqual(t, qs, ids(clearish, shiny), tagID)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_access_via_content_type
func TestAccessViaContentType(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		if _, err := orm.Query[Animal](db).Filter("pk", d.lion.ID).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		createTag(t, db, d.platypus, "fatty")

		ctype := mustGet(t, db, d.platypus)
		qs := orm.Query[Animal](db).Filter("tags__content_type", ctype.ID)
		djangotest.AssertQuerySetEqual(t, qs, []int64{d.platypus.ID}, func(a Animal) int64 { return a.ID })
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_set_foreign_key
func TestSetForeignKey(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		tag1 := createTagFor(t, db, d.quartz, "shiny")
		setObject(t, db, tag1, "content_object", d.platypus)
		if err := orm.Query[TaggedItem](db).Save(context.Background(), tag1); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, d.platypus, tag1)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_queries_across_generic_relations
//
// Queries across generic relations respect the content types: of the two
// "fatty" tags, only the one of an animal matches.
func TestQueriesAcrossGenericRelations(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		qs := orm.Query[Animal](db).OrderBy("common_name")
		djangotest.AssertQuerySetEqual(t, qs, []int64{d.lion.ID, d.platypus.ID}, func(a Animal) int64 { return a.ID })
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_queries_content_type_restriction
//
// A "fatty" tag of a ManualPK row with the primary key of the lion shows that
// the queries restrict the content type.
func TestQueriesContentTypeRestriction(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		mpk := mustCreate(t, db, &ManualPK{ID: d.lion.ID})
		createTag(t, db, mpk, "fatty")
		createTag(t, db, d.platypus, "fatty")

		animalID := func(a Animal) int64 { return a.ID }
		djangotest.AssertQuerySetEqual(t, orm.Query[Animal](db).Filter("tags__tag", "fatty"),
			[]int64{d.platypus.ID}, animalID)
		djangotest.AssertQuerySetEqual(t, orm.Query[Animal](db).Exclude("tags__tag", "fatty"),
			[]int64{d.lion.ID}, animalID)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_object_deletion_with_generic_relation
//
// Deleting an object with a generic relation deletes its related objects.
func TestObjectDeletionWithGenericRelation(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		comp := compFunc(t, db)
		djangotest.AssertQuerySetEqual(t, orm.Query[TaggedItem](db), []tagRow{
			{"fatty", "Vegetable", d.bacon.ID},
			{"hairy", "Animal", d.lion.ID},
			{"salty", "Vegetable", d.bacon.ID},
			{"yellow", "Animal", d.lion.ID},
		}, comp)
		if _, err := orm.Query[Animal](db).Filter("pk", d.lion.ID).Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[TaggedItem](db), []tagRow{
			{"fatty", "Vegetable", d.bacon.ID},
			{"salty", "Vegetable", d.bacon.ID},
		}, comp)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_object_deletion_without_generic_relation
//
// Without a generic relation, the related objects of a deleted object remain.
func TestObjectDeletionWithoutGenericRelation(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		createTagFor(t, db, d.quartz, "clearish")
		quartzPK := d.quartz.ID
		if _, err := orm.Query[Mineral](db).Filter("pk", d.quartz.ID).Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[TaggedItem](db), []tagRow{
			{"clearish", "Mineral", quartzPK},
			{"fatty", "Vegetable", d.bacon.ID},
			{"hairy", "Animal", d.lion.ID},
			{"salty", "Vegetable", d.bacon.ID},
			{"yellow", "Animal", d.lion.ID},
		}, compFunc(t, db))
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_tag_deletion_related_objects_unaffected
func TestTagDeletionRelatedObjectsUnaffected(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		ctype := mustGet(t, db, d.lion)
		tag, err := orm.Query[TaggedItem](db).Get(ctx, "content_type__pk", ctype.ID, "object_id", d.lion.ID, "tag", "hairy")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := orm.Query[TaggedItem](db).Filter("pk", tag.ID).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, d.lion, d.yellow)
		djangotest.AssertQuerySetEqual(t, orm.Query[TaggedItem](db), []tagRow{
			{"fatty", "Vegetable", d.bacon.ID},
			{"salty", "Vegetable", d.bacon.ID},
			{"yellow", "Animal", d.lion.ID},
		}, compFunc(t, db))
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_add_bulk
func TestGenericAddBulk(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		bacon := mustCreate(t, db, &Vegetable{Name: "Bacon", IsYucky: false})
		t1 := createTagFor(t, db, d.quartz, "shiny")
		t2 := createTagFor(t, db, d.quartz, "clearish")
		// One UPDATE.
		djangotest.AssertNumQueries(t, db, 1, func() {
			if err := tagsOf(db, bacon).Add(context.Background(), t1, t2); err != nil {
				t.Fatal(err)
			}
		})
		assertSameObject(t, getObject(t, db, t1, "content_object"), bacon)
		assertSameObject(t, getObject(t, db, t2, "content_object"), bacon)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_add_bulk_false
func TestGenericAddBulkFalse(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		bacon := mustCreate(t, db, &Vegetable{Name: "Bacon", IsYucky: false})
		t1 := createTagFor(t, db, d.quartz, "shiny")
		t2 := createTagFor(t, db, d.quartz, "clearish")
		// One save for each object.
		djangotest.AssertNumQueries(t, db, 2, func() {
			if err := tagsOf(db, bacon).AddSave(context.Background(), t1, t2); err != nil {
				t.Fatal(err)
			}
		})
		assertSameObject(t, getObject(t, db, t1, "content_object"), bacon)
		assertSameObject(t, getObject(t, db, t2, "content_object"), bacon)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_add_rejects_unsaved_objects
func TestGenericAddRejectsUnsavedObjects(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		t1 := newTag(t, db, d.quartz, "shiny")
		err := tagsOf(db, d.bacon).Add(context.Background(), t1)
		if err == nil || !strings.Contains(err.Error(), "TaggedItem instance isn't saved: use AddSave or save the object first") {
			t.Fatalf("Add of an unsaved tag: %v", err)
		}
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_set
func TestGenericSet(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		genericSetUp(t, db)
		bacon := mustCreate(t, db, &Vegetable{Name: "Bacon", IsYucky: false})
		fatty := createTag(t, db, bacon, "fatty")
		salty := createTag(t, db, bacon, "salty")
		tags := tagsOf(db, bacon)
		set := func(objs []*TaggedItem, opts ...orm.ManyOption) {
			t.Helper()
			if err := tags.Set(ctx, objs, opts...); err != nil {
				t.Fatal(err)
			}
		}

		set([]*TaggedItem{fatty, salty})
		assertTags(t, db, bacon, fatty, salty)

		set([]*TaggedItem{fatty})
		assertTags(t, db, bacon, fatty)

		set(nil)
		assertTags(t, db, bacon)

		set([]*TaggedItem{fatty, salty}, orm.Bulk(false), orm.SetClear())
		assertTags(t, db, bacon, fatty, salty)

		set([]*TaggedItem{fatty}, orm.Bulk(false), orm.SetClear())
		assertTags(t, db, bacon, fatty)

		set(nil, orm.SetClear())
		assertTags(t, db, bacon)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_assign
func TestGenericAssign(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		genericSetUp(t, db)
		bacon := mustCreate(t, db, &Vegetable{Name: "Bacon", IsYucky: false})
		fatty := createTag(t, db, bacon, "fatty")
		salty := createTag(t, db, bacon, "salty")
		tags := tagsOf(db, bacon)

		if err := tags.Set(ctx, []*TaggedItem{fatty, salty}); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, bacon, fatty, salty)

		if err := tags.Set(ctx, []*TaggedItem{fatty}); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, bacon, fatty)

		if err := tags.Set(ctx, nil); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, bacon)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_assign_with_queryset
//
// The rows of a queryset passed to Set are read before Set changes the
// relation. Go evaluates the queryset when the caller turns it into objects,
// so the ported test reads it with All.
func TestGenericAssignWithQueryset(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		genericSetUp(t, db)
		bacon := mustCreate(t, db, &Vegetable{Name: "Bacon", IsYucky: false})
		createTag(t, db, bacon, "fatty")
		createTag(t, db, bacon, "salty")
		djangotest.AssertEqual(t, countTags(t, db, bacon), 2)

		qs := tagsOf(db, bacon).Filter("tag", "fatty")
		rows, err := qs.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		objs := make([]*TaggedItem, len(rows))
		for i := range rows {
			objs[i] = &rows[i]
		}
		if err := tagsOf(db, bacon).Set(ctx, objs); err != nil {
			t.Fatal(err)
		}

		djangotest.AssertEqual(t, countTags(t, db, bacon), 1)
		n, err := qs.Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, 1)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_clear
func TestGenericClear(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		byTag := orm.Query[TaggedItem](db).OrderBy("tag")
		djangotest.AssertQuerySetEqual(t, byTag, ids(d.fatty, d.hairy, d.salty, d.yellow), tagID)
		if err := tagsOf(db, d.bacon).Clear(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, d.bacon)
		djangotest.AssertQuerySetEqual(t, byTag, ids(d.hairy, d.yellow), tagID)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_remove
func TestGenericRemove(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		byTag := orm.Query[TaggedItem](db).OrderBy("tag")
		djangotest.AssertQuerySetEqual(t, byTag, ids(d.fatty, d.hairy, d.salty, d.yellow), tagID)
		if err := tagsOf(db, d.bacon).Remove(context.Background(), d.fatty); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, d.bacon, d.salty)
		djangotest.AssertQuerySetEqual(t, byTag, ids(d.hairy, d.salty, d.yellow), tagID)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_generic_relation_related_name_default
//
// A generic relation leads back from the related model only through its
// related_query_name. Django's list of choices also names
// valuabletaggeditem, a multi-table child of TaggedItem that Go does not
// have.
func TestGenericRelationRelatedNameDefault(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[TaggedItem](db).Filter("vegetable__isnull", true).All(context.Background())
		var fe *orm.FieldError
		if !errors.As(err, &fe) {
			t.Fatalf("filter on vegetable: %v, want a FieldError", err)
		}
		const msg = "annot resolve keyword 'vegetable' into field. Choices are: " +
			"animal, content_object, content_type, content_type_id, id, manualpk, object_id, tag"
		if !strings.Contains(err.Error(), msg) {
			t.Fatalf("error %q does not contain %q", err, msg)
		}
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_multiple_gfk
func TestMultipleGFK(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		genericSetUp(t, db)
		tiger := mustCreate(t, db, &Animal{CommonName: "tiger"})
		cheetah := mustCreate(t, db, &Animal{CommonName: "cheetah"})
		bear := mustCreate(t, db, &Animal{CommonName: "bear"})

		newComparison := func(first, other any, comparative string) *Comparison {
			c := &Comparison{Comparative: comparative}
			if first != nil {
				setObject(t, db, c, "first_obj", first)
			}
			setObject(t, db, c, "other_obj", other)
			return c
		}
		// Create directly.
		c1 := mustCreate(t, db, newComparison(cheetah, tiger, "faster"))
		c2 := mustCreate(t, db, newComparison(tiger, cheetah, "cooler"))

		// Create through the generic relation.
		c3 := newComparison(nil, bear, "cooler")
		if err := comparisonsOf(db, tiger).Create(ctx, c3); err != nil {
			t.Fatal(err)
		}
		c4 := newComparison(nil, cheetah, "stronger")
		if err := comparisonsOf(db, tiger).Create(ctx, c4); err != nil {
			t.Fatal(err)
		}
		compID := func(c Comparison) int64 { return c.ID }
		djangotest.AssertQuerySetEqual(t, comparisonsOf(db, cheetah).QuerySet(), []int64{c1.ID}, compID)

		// Filtering works.
		djangotest.AssertQuerySetEqual(t, comparisonsOf(db, tiger).Filter("comparative", "cooler"),
			[]int64{c2.ID, c3.ID}, compID, djangotest.Unordered())

		// Filtering and deleting works.
		subjective := []string{"cooler"}
		if _, err := comparisonsOf(db, tiger).Filter("comparative__in", subjective).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[Comparison](db), []int64{c1.ID, c4.ID}, compID, djangotest.Unordered())

		// Deleting cheetah deletes the comparisons whose first_obj it is, through
		// the generic relation of Animal, and keeps the ones whose other_obj it
		// is.
		if _, err := orm.Query[Animal](db).Filter("pk", cheetah.ID).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertQuerySetEqual(t, orm.Query[Comparison](db), []int64{c4.ID}, compID)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_get_or_create
//
// GetOrCreate assigns a generic foreign key from its defaults.
func TestGenericGetOrCreateVirtualField(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		genericSetUp(t, db)
		quartz := mustCreate(t, db, &Mineral{Name: "Quartz", Hardness: 7})
		tag, created, err := orm.Query[TaggedItem](db).GetOrCreate(context.Background(),
			orm.Defaults{"content_object": quartz}, "tag", "shiny")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, tag.Tag, "shiny")
		assertSameObject(t, getObject(t, db, &tag, "content_object"), quartz)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_update_or_create_defaults
//
// UpdateOrCreate assigns a generic foreign key from its defaults.
func TestGenericUpdateOrCreateDefaults(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		genericSetUp(t, db)
		quartz := mustCreate(t, db, &Mineral{Name: "Quartz", Hardness: 7})
		diamond := mustCreate(t, db, &Mineral{Name: "Diamond", Hardness: 7})
		tag, created, err := orm.Query[TaggedItem](db).UpdateOrCreate(ctx,
			orm.Defaults{"content_object": quartz}, "tag", "shiny")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, created, true)
		assertSameObject(t, getObject(t, db, &tag, "content_object"), quartz)

		tag, created, err = orm.Query[TaggedItem](db).UpdateOrCreate(ctx,
			orm.Defaults{"content_object": diamond}, "tag", "shiny")
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, created, false)
		assertSameObject(t, getObject(t, db, &tag, "content_object"), diamond)
		// The update reached the database.
		stored, err := orm.Query[TaggedItem](db).Get(ctx, "pk", tag.ID)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, stored.ObjectID, diamond.ID)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_query_content_type
func TestQueryContentType(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[TaggedItem](db).Get(context.Background(), "content_object", "")
		var fe *orm.FieldError
		const msg = "Field 'content_object' does not generate an automatic reverse relation"
		if !errors.As(err, &fe) || !strings.Contains(err.Error(), msg) {
			t.Fatalf("filter on content_object: %v, want a FieldError containing %q", err, msg)
		}
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_unsaved_generic_foreign_key_parent_save
func TestUnsavedGenericForeignKeyParentSave(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		quartz := &Mineral{Name: "Quartz", Hardness: 7}
		tag := newTag(t, db, quartz, "shiny")
		err := orm.Query[TaggedItem](db).Save(context.Background(), tag)
		const msg = "save() prohibited to prevent data loss due to unsaved related object 'content_object'"
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatalf("Save: %v, want an error containing %q", err, msg)
		}
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_unsaved_generic_foreign_key_parent_bulk_create
func TestUnsavedGenericForeignKeyParentBulkCreate(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		quartz := &Mineral{Name: "Quartz", Hardness: 7}
		tag := newTag(t, db, quartz, "shiny")
		err := orm.Query[TaggedItem](db).BulkCreate(context.Background(), []*TaggedItem{tag})
		const msg = "bulk_create() prohibited to prevent data loss due to unsaved related object 'content_object'"
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatalf("BulkCreate: %v, want an error containing %q", err, msg)
		}
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_cache_invalidation_for_content_type_id
func TestCacheInvalidationForContentTypeID(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		genericSetUp(t, db)
		// A Vegetable and a Mineral with the same id.
		lastVegetable, err := orm.Query[Vegetable](db).OrderBy("-id").First(ctx)
		if err != nil {
			t.Fatal(err)
		}
		lastMineral, err := orm.Query[Mineral](db).OrderBy("-id").First(ctx)
		if err != nil {
			t.Fatal(err)
		}
		newID := max(lastVegetable.ID, lastMineral.ID) + 1
		broccoli := mustCreate(t, db, &Vegetable{ID: newID, Name: "Broccoli"})
		diamond := mustCreate(t, db, &Mineral{ID: newID, Name: "Diamond", Hardness: 7})
		tag := createTagFor(t, db, broccoli, "yummy")
		tag.ContentType.SetPK(mustGet(t, db, diamond).ID)
		assertSameObject(t, getObject(t, db, tag, "content_object"), diamond)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_cache_invalidation_for_object_id
func TestCacheInvalidationForObjectID(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		genericSetUp(t, db)
		broccoli := mustCreate(t, db, &Vegetable{Name: "Broccoli"})
		cauliflower := mustCreate(t, db, &Vegetable{Name: "Cauliflower"})
		tag := createTagFor(t, db, broccoli, "yummy")
		tag.ObjectID = cauliflower.ID
		assertSameObject(t, getObject(t, db, tag, "content_object"), cauliflower)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_assign_content_object_in_init
func TestAssignContentObjectInInit(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		spinach := &Vegetable{Name: "spinach"}
		tag := &TaggedItem{}
		setObject(t, db, tag, "content_object", spinach)
		got := getObject(t, db, tag, "content_object")
		if got != any(spinach) {
			t.Fatalf("content object %#v, want the unsaved spinach", got)
		}
	})
}

// prefetchedPlatypus returns the platypus read with its tags prefetched.
func prefetchedPlatypus(t *testing.T, db *orm.DB, d genericData) *Animal {
	t.Helper()
	platypus, err := orm.Query[Animal](db).PrefetchRelated("tags").Get(context.Background(), "pk", d.platypus.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &platypus
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_create_after_prefetch
func TestGenericCreateAfterPrefetch(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		platypus := prefetchedPlatypus(t, db, d)
		assertTags(t, db, platypus)
		weird := createTag(t, db, platypus, "weird")
		assertTags(t, db, platypus, weird)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_add_after_prefetch
func TestGenericAddAfterPrefetch(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		platypus := prefetchedPlatypus(t, db, d)
		assertTags(t, db, platypus)
		weird := createTagFor(t, db, platypus, "weird")
		if err := tagsOf(db, platypus).Add(context.Background(), weird); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, platypus, weird)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_remove_after_prefetch
func TestGenericRemoveAfterPrefetch(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		weird := createTag(t, db, d.platypus, "weird")
		platypus := prefetchedPlatypus(t, db, d)
		assertTags(t, db, platypus, weird)
		if err := tagsOf(db, platypus).Remove(context.Background(), weird); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, platypus)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_clear_after_prefetch
func TestGenericClearAfterPrefetch(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		weird := createTag(t, db, d.platypus, "weird")
		platypus := prefetchedPlatypus(t, db, d)
		assertTags(t, db, platypus, weird)
		if err := tagsOf(db, platypus).Clear(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, platypus)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_set_after_prefetch
func TestGenericSetAfterPrefetch(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		platypus := prefetchedPlatypus(t, db, d)
		assertTags(t, db, platypus)
		furry := createTagFor(t, db, platypus, "furry")
		if err := tagsOf(db, platypus).Set(ctx, []*TaggedItem{furry}); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, platypus, furry)
		weird := createTagFor(t, db, platypus, "weird")
		if err := tagsOf(db, platypus).Set(ctx, []*TaggedItem{weird}); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, platypus, weird)
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_add_then_remove_after_prefetch
func TestGenericAddThenRemoveAfterPrefetch(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		furry := createTag(t, db, d.platypus, "furry")
		platypus := prefetchedPlatypus(t, db, d)
		assertTags(t, db, platypus, furry)
		weird := createTag(t, db, d.platypus, "weird")
		if err := tagsOf(db, platypus).Add(ctx, weird); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, platypus, furry, weird)
		if err := tagsOf(db, platypus).Remove(ctx, weird); err != nil {
			t.Fatal(err)
		}
		assertTags(t, db, platypus, furry)
	})
}

// No Django test exists: a prefetched generic relation serves All without a
// query, as a prefetched reverse foreign key does.
func TestGenericPrefetchServesAll(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		animals, err := orm.Query[Animal](db).PrefetchRelated("tags").OrderBy("common_name").All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got [][]int64
		djangotest.AssertNumQueries(t, db, 0, func() {
			for i := range animals {
				tags, err := tagsOf(db, &animals[i]).All(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var tagIDs []int64
				for _, tag := range tags {
					tagIDs = append(tagIDs, tag.ID)
				}
				got = append(got, tagIDs)
			}
		})
		want := [][]int64{ids(d.hairy, d.yellow), nil}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("prefetched tags %v, want %v", got, want)
		}
	})
}

// Ported from Django: tests/generic_relations/tests.py::TestInitWithNoneArgument.test_none_allowed
//
// Assigning no object to a generic foreign key is allowed, whether or not
// its fields are nullable.
func TestGenericForeignKeyNoneAllowed(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		setObject(t, db, &AllowsNullGFK{}, "content_object", nil)
		setObject(t, db, &TaggedItem{}, "content_object", nil)
	})
}

// No Django test exists: assigning nil clears the content type and the object
// id, and the generic foreign key then holds no object.
func TestGenericForeignKeySetNil(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		d := genericSetUp(t, db)
		item := &AllowsNullGFK{}
		setObject(t, db, item, "content_object", d.bacon)
		if item.ObjectID == nil || *item.ObjectID != d.bacon.ID || item.ContentType.PK() == 0 {
			t.Fatalf("after SetObject(bacon): content type %d, object id %v", item.ContentType.PK(), item.ObjectID)
		}
		setObject(t, db, item, "content_object", nil)
		if item.ObjectID != nil || item.ContentType.PK() != 0 {
			t.Fatalf("after SetObject(nil): content type %d, object id %v", item.ContentType.PK(), item.ObjectID)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			if obj := getObject(t, db, item, "content_object"); obj != nil {
				t.Fatalf("content object %#v, want nil", obj)
			}
		})
	})
}

// No Django test exists: a generic foreign key whose object row is gone holds
// no object, as in Django's GenericForeignKey.__get__.
func TestGenericForeignKeyMissingObject(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		tag := createTagFor(t, db, d.quartz, "clearish")
		if _, err := orm.Query[Mineral](db).Filter("pk", d.quartz.ID).Delete(ctx); err != nil {
			t.Fatal(err)
		}
		stored, err := orm.Query[TaggedItem](db).Get(ctx, "pk", tag.ID)
		if err != nil {
			t.Fatal(err)
		}
		if obj := getObject(t, db, &stored, "content_object"); obj != nil {
			t.Fatalf("content object %#v, want nil", obj)
		}
	})
}

// No Django test exists: a model that declares a generic relation to a model
// without the named fields is rejected when the registry is resolved.
func TestGenericRelationMissingField(t *testing.T) {
	type badOwner struct {
		ID   int64
		Tags contenttypes.GenericRelation[Mineral]
	}
	reg := orm.NewRegistry()
	for _, m := range []any{&contenttypes.ContentType{}, &Mineral{}, &badOwner{}} {
		if _, err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	err := reg.Resolve()
	if err == nil || !strings.Contains(err.Error(), "Mineral.content_type is not a foreign key to the content types") {
		t.Fatalf("Resolve: %v", err)
	}
}

// No Django test exists: a generic foreign key tag that names a missing field
// is rejected when the registry is resolved.
func TestGenericForeignKeyMissingField(t *testing.T) {
	type badItem struct {
		ID            int64
		ContentType   orm.FK[contenttypes.ContentType]
		ContentObject contenttypes.GenericForeignKey `orm:"fk_field=missing"`
	}
	reg := orm.NewRegistry()
	for _, m := range []any{&contenttypes.ContentType{}, &badItem{}} {
		if _, err := reg.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	err := reg.Resolve()
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("Resolve: %v", err)
	}
}

// tagIDsOf returns the primary keys of the tags of owner, read through its
// tags manager.
func tagIDsOf(t *testing.T, db *orm.DB, owner any) []int64 {
	t.Helper()
	tags, err := tagsOf(db, owner).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int64, len(tags))
	for i, tag := range tags {
		out[i] = tag.ID
	}
	return out
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_prefetch_related_different_content_types
func TestGenericPrefetchRelatedDifferentContentTypes(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		createTagFor(t, db, d.platypus, "prefetch_tag_1")
		createTagFor(t, db, mustCreate(t, db, &Vegetable{Name: "Broccoli"}), "prefetch_tag_2")
		createTagFor(t, db, mustCreate(t, db, &Animal{CommonName: "Bear"}), "prefetch_tag_3")
		qs := orm.Query[TaggedItem](db).Filter("tag__startswith", "prefetch_tag_").
			PrefetchRelated("content_object", "content_object__tags")
		var tags []TaggedItem
		djangotest.AssertNumQueries(t, db, 4, func() {
			var err error
			if tags, err = qs.All(ctx); err != nil {
				t.Fatal(err)
			}
		})
		if len(tags) != 3 {
			t.Fatalf("%d tags, want 3", len(tags))
		}
		for i := range tags {
			obj := getObject(t, db, &tags[i], "content_object")
			if got, want := tagIDsOf(t, db, obj), []int64{tags[i].ID}; !reflect.DeepEqual(got, want) {
				t.Fatalf("tags of the object of %q: %v, want %v", tags[i].Tag, got, want)
			}
		}
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_prefetch_related_custom_object_id
func TestGenericPrefetchRelatedCustomObjectID(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		genericSetUp(t, db)
		tiger := mustCreate(t, db, &Animal{CommonName: "tiger"})
		cheetah := mustCreate(t, db, &Animal{CommonName: "cheetah"})
		for _, c := range []struct {
			first, other *Animal
			comparative  string
		}{{cheetah, tiger, "faster"}, {tiger, cheetah, "cooler"}} {
			comparison := &Comparison{Comparative: c.comparative}
			setObject(t, db, comparison, "first_obj", c.first)
			setObject(t, db, comparison, "other_obj", c.other)
			mustCreate(t, db, comparison)
		}
		comparisons, err := orm.Query[Comparison](db).PrefetchRelated("first_obj__comparisons").All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(comparisons) != 2 {
			t.Fatalf("%d comparisons, want 2", len(comparisons))
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			for i := range comparisons {
				first := getObject(t, db, &comparisons[i], "first_obj")
				got, err := comparisonsOf(db, first).All(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != 1 || got[0].ID != comparisons[i].ID {
					t.Fatalf("comparisons of first_obj of %q: %v, want [%d]",
						comparisons[i].Comparative, got, comparisons[i].ID)
				}
			}
		})
	})
}

// Ported from Django: tests/generic_relations/tests.py::GenericRelationsTests.test_generic_prefetch
//
// Django passes Animal.objects.only("common_name") and then reads the deferred
// latin_name with one query. The Go ORM has no deferred fields, so the
// queryset loads every field and the test checks that the custom querysets
// served the objects.
func TestGenericPrefetch(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		taggedVegetable := createTagFor(t, db, d.bacon, "great")
		taggedAnimal := createTagFor(t, db, d.platypus, "awesome")
		// Read the tags again so that the content objects are not cached.
		vegetableTag, err := orm.Query[TaggedItem](db).Get(ctx, "pk", taggedVegetable.ID)
		if err != nil {
			t.Fatal(err)
		}
		animalTag, err := orm.Query[TaggedItem](db).Get(ctx, "pk", taggedAnimal.ID)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertNumQueries(t, db, 2, func() {
			err := orm.PrefetchRelatedObjects(ctx, db, []*TaggedItem{&vegetableTag, &animalTag}, orm.Prefetch{
				Lookup:    "content_object",
				QuerySets: []any{orm.Query[Vegetable](db), orm.Query[Animal](db).Filter("common_name", d.platypus.CommonName)},
			})
			if err != nil {
				t.Fatal(err)
			}
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			if got := getObject(t, db, &vegetableTag, "content_object").(*Vegetable).Name; got != d.bacon.Name {
				t.Fatalf("vegetable %q, want %q", got, d.bacon.Name)
			}
			animal := getObject(t, db, &animalTag, "content_object").(*Animal)
			if animal.CommonName != d.platypus.CommonName || animal.LatinName != d.platypus.LatinName {
				t.Fatalf("animal %q (%q), want %q (%q)",
					animal.CommonName, animal.LatinName, d.platypus.CommonName, d.platypus.LatinName)
			}
		})
	})
}

// No Django test exists: a custom queryset of a generic foreign key prefetch
// filters the objects, and an instance whose object it filters out caches
// nil, as Django's prefetch does.
func TestGenericPrefetchQuerySetFilters(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		tags, err := orm.Query[TaggedItem](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "content_object",
			QuerySet: orm.Query[Animal](db).Exclude("pk", d.lion.ID),
		}).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			for i := range tags {
				obj := getObject(t, db, &tags[i], "content_object")
				switch tags[i].Tag {
				case "fatty", "salty":
					assertSameObject(t, obj, d.bacon)
				default:
					if obj != nil {
						t.Fatalf("object of %q: %v, want nil", tags[i].Tag, obj)
					}
				}
			}
		})
	})
}

// Ported from Django: tests/contenttypes_tests/test_fields.py::GetPrefetchQuerySetsTests.test_duplicate_querysets
func TestGenericPrefetchDuplicateQuerySets(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		d := genericSetUp(t, db)
		tag, err := orm.Query[TaggedItem](db).Get(ctx, "pk", d.fatty.ID)
		if err != nil {
			t.Fatal(err)
		}
		err = orm.PrefetchRelatedObjects(ctx, db, &tag, orm.Prefetch{
			Lookup: "content_object",
			QuerySets: []any{
				orm.Query[Vegetable](db),
				orm.Query[Vegetable](db).Filter("name__startswith", "test"),
			},
		})
		if err == nil || !strings.Contains(err.Error(), "only one queryset is allowed for each content type") {
			t.Fatalf("PrefetchRelatedObjects: %v", err)
		}
	})
}

// Ported from Django: tests/contenttypes_tests/test_fields.py::GetPrefetchQuerySetsTests.test_generic_relation_invalid_length
//
// Django calls get_prefetch_querysets() of the manager with two querysets.
// The Go counterpart is a Prefetch of a generic relation with two querysets.
func TestGenericRelationPrefetchInvalidLength(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		genericSetUp(t, db)
		_, err := orm.Query[Animal](db).PrefetchRelated(orm.Prefetch{
			Lookup:    "tags",
			QuerySets: []any{orm.Query[TaggedItem](db), orm.Query[Animal](db)},
		}).All(context.Background())
		if err == nil || !strings.Contains(err.Error(), "takes one queryset, got 2") {
			t.Fatalf("All: %v", err)
		}
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::GenericPrefetchTests.test_values_queryset
func TestGenericPrefetchValuesQuerySet(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		const msg = "Prefetch querysets cannot use raw(), values(), and values_list()"
		for _, qs := range []any{
			orm.Query[Animal](db).Values("pk"),
			orm.Query[Animal](db).ValuesList("pk"),
		} {
			_, err := orm.Query[TaggedItem](db).PrefetchRelated(orm.Prefetch{
				Lookup:    "content_object",
				QuerySets: []any{qs},
			}).All(context.Background())
			if err == nil || !strings.Contains(err.Error(), msg) {
				t.Fatalf("%T: %v", qs, err)
			}
		}
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::GenericPrefetchTests.test_raw_queryset
func TestGenericPrefetchRawQuerySet(t *testing.T) {
	djangotest.EachDB(t, genericModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[TaggedItem](db).PrefetchRelated(orm.Prefetch{
			Lookup:    "content_object",
			QuerySets: []any{orm.Raw[Animal](db, `SELECT "id" FROM "generic_relations_animal"`)},
		}).All(context.Background())
		if err == nil || !strings.Contains(err.Error(), "Prefetch querysets cannot use raw(), values(), and values_list()") {
			t.Fatalf("All: %v", err)
		}
	})
}

// No Go counterpart: these GenericRelationsTests and the other test classes of
// tests/generic_relations/tests.py cover Python concepts.
//   - The async tests (test_generic_async_acreate, test_aadd, test_aset,
//     test_aclear, test_aremove and the aget_or_create and aupdate_or_create
//     tests): the Go ORM is synchronous, and a context carries cancellation.
//   - test_generic_update_or_create_when_created_with_create_defaults,
//     test_generic_update_or_create_when_updated_with_defaults and
//     test_update_or_create_defaults_with_create_defaults: UpdateOrCreate has
//     no create_defaults.
//   - test_add_rejects_wrong_instances: the type parameter of the manager
//     rejects another model at compile time.
//   - test_gfk_subclasses, test_generic_relation_to_inherited_child,
//     test_subclasses_with_gen_rel and test_subclasses_with_parent_gen_rel:
//     there is no multi-table inheritance.
//   - test_gfk_manager: a model has no custom default manager.
//   - ProxyRelatedModelTest: there are no proxy models.
//
// No Go counterpart: tests/contenttypes_tests/test_models.py::GenericPrefetchTests.test_querysets_required
// checks a Python TypeError for a missing argument; Prefetch.QuerySets is an
// optional field.
