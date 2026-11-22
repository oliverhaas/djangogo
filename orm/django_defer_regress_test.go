package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/defer_regress/models.py. ProxyRelated,
// Proxy, Base and Derived need proxy models or multi-table inheritance, which
// the ORM does not have.

// drItem is Django's defer_regress.Item.
type drItem struct {
	orm.RelCache
	ID         int64
	Name       string          `orm:"max_length=15"`
	Text       string          `orm:"type=text;default=xyzzy"`
	Value      int64           //
	OtherValue int64           `orm:"default=0"`
	Source     orm.O2O[drItem] `orm:"null;on_delete=cascade;related_name=destination"`
}

// drRelatedItem is Django's defer_regress.RelatedItem.
type drRelatedItem struct {
	orm.RelCache
	ID   int64
	Item orm.FK[drItem] `orm:"on_delete=cascade"`
}

// drChild is Django's defer_regress.Child.
type drChild struct {
	orm.RelCache
	ID    int64
	Name  string `orm:"max_length=10"`
	Value int64
}

// drLeaf is Django's defer_regress.Leaf.
type drLeaf struct {
	orm.RelCache
	ID          int64
	Name        string          `orm:"max_length=10"`
	Child       orm.FK[drChild] `orm:"on_delete=cascade"`
	SecondChild orm.FK[drChild] `orm:"null;on_delete=set_null;related_name=other"`
	Value       int64           `orm:"default=42"`
}

// drResolveThis is Django's defer_regress.ResolveThis.
type drResolveThis struct {
	orm.RelCache
	ID   int64
	Num  float64
	Name string `orm:"max_length=16"`
}

// drSimpleItem is Django's defer_regress.SimpleItem.
type drSimpleItem struct {
	orm.RelCache
	ID    int64
	Name  string `orm:"max_length=15"`
	Value int64
}

// drFeature is Django's defer_regress.Feature.
type drFeature struct {
	orm.RelCache
	ID   int64
	Item orm.FK[drSimpleItem] `orm:"on_delete=cascade;related_name=feature"`
}

// drSpecialFeature is Django's defer_regress.SpecialFeature.
type drSpecialFeature struct {
	orm.RelCache
	ID      int64
	Feature orm.FK[drFeature] `orm:"on_delete=cascade"`
}

// drOneToOneItem is Django's defer_regress.OneToOneItem.
type drOneToOneItem struct {
	orm.RelCache
	ID   int64
	Item orm.O2O[drItem] `orm:"on_delete=cascade;related_name=one_to_one_item"`
	Name string          `orm:"max_length=15"`
}

// drItemAndSimpleItem is Django's defer_regress.ItemAndSimpleItem.
type drItemAndSimpleItem struct {
	orm.RelCache
	ID     int64
	Item   orm.FK[drItem]       `orm:"on_delete=cascade"`
	Simple orm.FK[drSimpleItem] `orm:"on_delete=cascade"`
}

// drProfile is Django's defer_regress.Profile.
type drProfile struct {
	orm.RelCache
	ID       int64
	Profile1 string `orm:"max_length=255;default=profile1"`
}

// drLocation is Django's defer_regress.Location.
type drLocation struct {
	orm.RelCache
	ID        int64
	Location1 string `orm:"max_length=255;default=location1"`
}

// drRequest is Django's defer_regress.Request.
type drRequest struct {
	orm.RelCache
	ID       int64
	Profile  orm.FK[drProfile]  `orm:"null;blank;on_delete=set_null"`
	Location orm.FK[drLocation] `orm:"on_delete=cascade"`
	Items    orm.M2M[drItem]
	Request1 string `orm:"max_length=255;default=request1"`
	Request2 string `orm:"max_length=255;default=request2"`
	Request3 string `orm:"max_length=255;default=request3"`
	Request4 string `orm:"max_length=255;default=request4"`
}

// drModels returns the models of the defer_regress tests.
func drModels() []any {
	return []any{djangotest.App("defer_regress",
		&drItem{}, &drRelatedItem{}, &drChild{}, &drLeaf{}, &drResolveThis{}, &drSimpleItem{},
		&drFeature{}, &drSpecialFeature{}, &drOneToOneItem{}, &drItemAndSimpleItem{},
		&drProfile{}, &drLocation{}, &drRequest{},
	)}
}

// drRefresh loads the fields keys of obj, which Django loads when a deferred
// field is read, and checks that it takes one query.
func drRefresh[T any](t *testing.T, db *orm.DB, obj *T, keys ...string) {
	t.Helper()
	djangotest.AssertNumQueries(t, db, 1, func() {
		if err := orm.Query[T](db).RefreshFromDB(context.Background(), obj, keys...); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
	})
}

// drObject returns the object that a relation field holds.
func drObject[T any](t *testing.T, rel interface{ Object() (*T, bool) }) *T {
	t.Helper()
	obj, ok := rel.Object()
	if !ok {
		t.Fatal("the related object is not loaded")
	}
	return obj
}

// drCount checks the number of rows of qs.
func drCount[T any](t *testing.T, qs *orm.QuerySet[T], want int) {
	t.Helper()
	djangotest.AssertEqual(t, len(mustAll(t, qs)), want)
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_basic
func TestDeferRegressBasic(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &drItem{Name: "first", Value: 42})
		obj, err := orm.Query[drItem](db).Only("name", "other_value").Get(ctx, "name", "first")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, obj.Name, "first")
		djangotest.AssertEqual(t, obj.OtherValue, int64(0))
		djangotest.AssertEqual(t, orm.DeferredFields(obj), []string{"text", "value", "source_id"})
		// Django loads a deferred field when it is read; Go loads it with
		// RefreshFromDB.
		drRefresh(t, db, &obj, "value")
		djangotest.AssertEqual(t, obj.Value, int64(42))
		drRefresh(t, db, &obj, "text")
		djangotest.AssertEqual(t, obj.Text, "xyzzy")
		djangotest.AssertEqual(t, orm.DeferredFields(obj), []string{"source_id"})

		// Different instances do not share their deferred fields.
		i := create(t, db, &drItem{Name: "no I'm first", Value: 37})
		items := mustAll(t, orm.Query[drItem](db).Only("value").OrderBy("-value"))
		drRefresh(t, db, &items[0], "name")
		djangotest.AssertEqual(t, items[0].Name, "first")
		djangotest.AssertEqual(t, orm.DeferredFields(items[1]), []string{"name", "text", "other_value", "source_id"})
		drRefresh(t, db, &items[1], "name")
		djangotest.AssertEqual(t, items[1].Name, "no I'm first")

		create(t, db, &drRelatedItem{Item: fkTo[drItem](i.ID)})
		r, err := orm.Query[drRelatedItem](db).Defer("item").Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		drRefresh(t, db, &r, "item_id")
		djangotest.AssertEqual(t, r.Item.PK(), i.ID)

		// select_related() follows the foreign keys that are not null; Go
		// names them.
		c1 := create(t, db, &drChild{Name: "c1", Value: 42})
		c2 := create(t, db, &drChild{Name: "c2", Value: 37})
		create(t, db, &drLeaf{Name: "l1", Child: fkTo[drChild](c1.ID), SecondChild: fkTo[drChild](c2.ID)})

		leaf := dfFirst(t, orm.Query[drLeaf](db).Only("name", "child").SelectRelated("child"))
		djangotest.AssertEqual(t, drObject[drChild](t, leaf.Child).Name, "c1")

		leaves := mustAll(t, orm.Query[drLeaf](db).SelectRelated("child").Only("child__name", "second_child__name"))
		djangotest.AssertEqual(t, len(leaves), 1)
		djangotest.AssertEqual(t, orm.DeferredFields(leaves[0]), []string{"name", "value"})
		drRefresh(t, db, &leaves[0], "name")
		djangotest.AssertEqual(t, leaves[0].Name, "l1")

		// Django checks here that ContentType.objects.get_for_model gives
		// the same content type for a deferred instance; Go has no deferred
		// model classes.

		// only() on a model with two foreign keys.
		leaves = mustAll(t, orm.Query[drLeaf](db).Only("name", "child", "second_child").
			SelectRelated("child", "second_child"))
		djangotest.AssertEqual(t, drObject[drChild](t, leaves[0].Child).Name, "c1")
		djangotest.AssertEqual(t, drObject[drChild](t, leaves[0].SecondChild).Name, "c2")

		leaves = mustAll(t, orm.Query[drLeaf](db).
			Only("name", "child", "second_child", "child__name", "second_child__name").
			SelectRelated("child", "second_child"))
		djangotest.AssertEqual(t, drObject[drChild](t, leaves[0].Child).Name, "c1")
		djangotest.AssertEqual(t, drObject[drChild](t, leaves[0].SecondChild).Name, "c2")

		// defer() and only() work with annotate().
		mustAll(t, orm.Query[drSimpleItem](db).Annotate(orm.Count("feature")).Defer("name"))
		mustAll(t, orm.Query[drSimpleItem](db).Annotate(orm.Count("feature")).Only("name"))
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_ticket_16409
func TestDeferRegressTicket16409(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		mustAll(t, orm.Query[drSimpleItem](db).Annotate(orm.Count("feature")).Defer("name"))
		mustAll(t, orm.Query[drSimpleItem](db).Annotate(orm.Count("feature")).Only("name"))
	})
}

// Skipped: tests/defer_regress/tests.py::DeferRegressionTest.test_ticket_23270
func TestDeferRegressTicket23270(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// Skipped: tests/defer_regress/tests.py::DeferRegressionTest.test_only_and_defer_usage_on_proxy_models
func TestDeferRegressOnlyAndDeferUsageOnProxyModels(t *testing.T) {
	t.Skip("needs proxy models, which the ORM does not have")
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_resolve_columns
func TestDeferRegressResolveColumns(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &drResolveThis{Num: 5.0, Name: "Foobar"})
		qs := orm.Query[drResolveThis](db).Defer("num")
		n, err := qs.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(1))
		djangotest.AssertEqual(t, dfFirst(t, qs).Name, "Foobar")
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_reverse_one_to_one_relations
func TestDeferRegressReverseOneToOneRelations(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		item := create(t, db, &drItem{Name: "first", Value: 42})
		o2o := create(t, db, &drOneToOneItem{Item: o2oTo[drItem](item.ID), Name: "second"})
		qs := orm.Query[drItem](db)
		drCount(t, qs.Defer("one_to_one_item__name"), 1)
		drCount(t, qs.SelectRelated("one_to_one_item"), 1)
		drCount(t, qs.SelectRelated("one_to_one_item").Defer("one_to_one_item__name"), 1)
		drCount(t, qs.SelectRelated("one_to_one_item").Defer("value"), 1)
		// only() with a unique relation, rather than a field of it.
		drCount(t, qs.Only("one_to_one_item"), 1)

		related := func(i *drItem) *drOneToOneItem {
			t.Helper()
			obj, err := orm.RelatedOne[drOneToOneItem](ctx, db, i, "one_to_one_item")
			if err != nil {
				t.Fatalf("RelatedOne: %v", err)
			}
			return obj
		}
		var i drItem
		djangotest.AssertNumQueries(t, db, 1, func() {
			i = dfFirst(t, qs.SelectRelated("one_to_one_item"))
			djangotest.AssertEqual(t, related(&i).ID, o2o.ID)
			djangotest.AssertEqual(t, related(&i).Name, "second")
		})
		djangotest.AssertNumQueries(t, db, 1, func() {
			i = dfFirst(t, qs.SelectRelated("one_to_one_item").Defer("value", "one_to_one_item__name"))
			djangotest.AssertEqual(t, related(&i).ID, o2o.ID)
			djangotest.AssertEqual(t, i.Name, "first")
		})
		drRefresh(t, db, related(&i), "name")
		djangotest.AssertEqual(t, related(&i).Name, "second")
		drRefresh(t, db, &i, "value")
		djangotest.AssertEqual(t, i.Value, int64(42))
		djangotest.AssertNumQueries(t, db, 1, func() {
			i = dfFirst(t, qs.SelectRelated("one_to_one_item").Only("name", "one_to_one_item__item"))
			djangotest.AssertEqual(t, related(&i).ID, o2o.ID)
			djangotest.AssertEqual(t, i.Name, "first")
		})
		drRefresh(t, db, related(&i), "name")
		djangotest.AssertEqual(t, related(&i).Name, "second")
		drRefresh(t, db, &i, "value")
		djangotest.AssertEqual(t, i.Value, int64(42))
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_defer_with_select_related
func TestDeferRegressDeferWithSelectRelated(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		item1 := create(t, db, &drItem{Name: "first", Value: 47})
		item2 := create(t, db, &drItem{Name: "second", Value: 42})
		simple := create(t, db, &drSimpleItem{Name: "simple", Value: 23})
		create(t, db, &drItemAndSimpleItem{Item: fkTo[drItem](item1.ID), Simple: fkTo[drSimpleItem](simple.ID)})

		qs := orm.Query[drItemAndSimpleItem](db).Defer("item").SelectRelated("simple")
		obj, err := qs.Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		drRefresh(t, db, &obj, "item")
		djangotest.AssertEqual(t, obj.Item.PK(), item1.ID)

		obj.Item = fkTo[drItem](item2.ID)
		if err := orm.Query[drItemAndSimpleItem](db).Save(ctx, &obj); err != nil {
			t.Fatalf("Save: %v", err)
		}

		obj, err = qs.Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		drRefresh(t, db, &obj, "item")
		djangotest.AssertEqual(t, obj.Item.PK(), item2.ID)
	})
}

// Skipped: tests/defer_regress/tests.py::DeferRegressionTest.test_proxy_model_defer_with_select_related
func TestDeferRegressProxyModelDeferWithSelectRelated(t *testing.T) {
	t.Skip("needs proxy models, which the ORM does not have")
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_only_with_select_related
func TestDeferRegressOnlyWithSelectRelated(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		item := create(t, db, &drSimpleItem{Name: "first", Value: 47})
		feature := create(t, db, &drFeature{Item: fkTo[drSimpleItem](item.ID)})
		create(t, db, &drSpecialFeature{Feature: fkTo[drFeature](feature.ID)})

		drCount(t, orm.Query[drFeature](db).Only("item__name").SelectRelated("item"), 1)
		drCount(t, orm.Query[drSpecialFeature](db).Only("feature__item__name").SelectRelated("feature__item"), 1)
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_defer_annotate_select_related
func TestDeferRegressDeferAnnotateSelectRelated(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		location := create(t, db, &drLocation{})
		create(t, db, &drRequest{Location: fkTo[drLocation](location.ID)})
		qs := orm.Query[drRequest](db).Annotate(orm.Count("items")).SelectRelated("profile", "location")
		drCount(t, qs.Only("profile", "location"), 1)
		drCount(t, qs.Only("profile__profile1", "location__location1"), 1)
		drCount(t, qs.Defer("request1", "request2", "request3", "request4"), 1)
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_common_model_different_mask
func TestDeferRegressCommonModelDifferentMask(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		child := create(t, db, &drChild{Name: "Child", Value: 42})
		secondChild := create(t, db, &drChild{Name: "Second", Value: 64})
		create(t, db, &drLeaf{Child: fkTo[drChild](child.ID), SecondChild: fkTo[drChild](secondChild.ID)})
		var leaf drLeaf
		djangotest.AssertNumQueries(t, db, 1, func() {
			var err error
			leaf, err = orm.Query[drLeaf](db).SelectRelated("child", "second_child").
				Defer("child__name", "second_child__value").Get(ctx)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
		})
		first := drObject[drChild](t, leaf.Child)
		second := drObject[drChild](t, leaf.SecondChild)
		djangotest.AssertEqual(t, first.ID, child.ID)
		djangotest.AssertEqual(t, second.ID, secondChild.ID)
		djangotest.AssertEqual(t, orm.DeferredFields(first), []string{"name"})
		djangotest.AssertEqual(t, orm.DeferredFields(second), []string{"value"})
		djangotest.AssertEqual(t, first.Value, int64(42))
		djangotest.AssertEqual(t, second.Name, "Second")
		drRefresh(t, db, first, "name")
		djangotest.AssertEqual(t, first.Name, "Child")
		drRefresh(t, db, second, "value")
		djangotest.AssertEqual(t, second.Value, int64(64))
	})
}

// drGetOne gets the single row of qs and checks that it takes one query.
func drGetOne[T any](t *testing.T, db *orm.DB, qs *orm.QuerySet[T]) T {
	t.Helper()
	var got T
	djangotest.AssertNumQueries(t, db, 1, func() {
		var err error
		got, err = qs.Get(context.Background())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
	})
	return got
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_defer_many_to_many_ignored
func TestDeferRegressDeferManyToManyIgnored(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		location := create(t, db, &drLocation{})
		request := create(t, db, &drRequest{Location: fkTo[drLocation](location.ID)})
		djangotest.AssertEqual(t, drGetOne(t, db, orm.Query[drRequest](db).Defer("items")).ID, request.ID)
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_only_many_to_many_ignored
func TestDeferRegressOnlyManyToManyIgnored(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		location := create(t, db, &drLocation{})
		request := create(t, db, &drRequest{Location: fkTo[drLocation](location.ID)})
		djangotest.AssertEqual(t, drGetOne(t, db, orm.Query[drRequest](db).Only("items")).ID, request.ID)
	})
}

// drRequestWithItem creates a request with one item and returns the item.
func drRequestWithItem(t *testing.T, db *orm.DB) *drItem {
	t.Helper()
	location := create(t, db, &drLocation{})
	request := create(t, db, &drRequest{Location: fkTo[drLocation](location.ID)})
	item := create(t, db, &drItem{Value: 1})
	if err := orm.Many[drItem](db, request, "items").Add(context.Background(), item); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return item
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_defer_reverse_many_to_many_ignored
func TestDeferRegressDeferReverseManyToManyIgnored(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		item := drRequestWithItem(t, db)
		djangotest.AssertEqual(t, drGetOne(t, db, orm.Query[drItem](db).Defer("drrequest")).ID, item.ID)
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_only_reverse_many_to_many_ignored
func TestDeferRegressOnlyReverseManyToManyIgnored(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		item := drRequestWithItem(t, db)
		djangotest.AssertEqual(t, drGetOne(t, db, orm.Query[drItem](db).Only("drrequest")).ID, item.ID)
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferRegressionTest.test_self_referential_one_to_one
func TestDeferRegressSelfReferentialOneToOne(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		first := create(t, db, &drItem{Name: "first", Value: 1})
		second := create(t, db, &drItem{Name: "second", Value: 2, Source: o2oTo[drItem](first.ID)})
		var items []drItem
		djangotest.AssertNumQueries(t, db, 1, func() {
			items = mustAll(t, orm.Query[drItem](db).SelectRelated("source", "destination").
				Only("name", "source__name", "destination__value").OrderBy("pk"))
		})
		djangotest.AssertEqual(t, len(items), 2)
		deferredFirst, deferredSecond := &items[0], &items[1]
		var destination *drItem
		djangotest.AssertNumQueries(t, db, 0, func() {
			djangotest.AssertEqual(t, deferredFirst.Name, first.Name)
			djangotest.AssertEqual(t, deferredSecond.Name, second.Name)
			djangotest.AssertEqual(t, drObject[drItem](t, deferredSecond.Source).Name, first.Name)
			var err error
			destination, err = orm.RelatedOne[drItem](ctx, db, deferredFirst, "destination")
			if err != nil {
				t.Fatalf("RelatedOne: %v", err)
			}
			djangotest.AssertEqual(t, destination.Value, second.Value)
		})
		drRefresh(t, db, deferredFirst, "value")
		djangotest.AssertEqual(t, deferredFirst.Value, first.Value)
		source := drObject[drItem](t, deferredSecond.Source)
		drRefresh(t, db, source, "value")
		djangotest.AssertEqual(t, source.Value, first.Value)
		drRefresh(t, db, destination, "name")
		djangotest.AssertEqual(t, destination.Name, second.Name)
	})
}

// No Go counterpart: tests/defer_regress/tests.py::DeferDeletionSignalsTests.test_delete_defered_model
// checks that the sender of the delete signals of a deferred instance is the
// model, not Django's deferred subclass of it; Go has no deferred classes.

// Skipped: tests/defer_regress/tests.py::DeferDeletionSignalsTests.test_delete_defered_proxy_model
func TestDeferRegressDeleteDeferedProxyModel(t *testing.T) {
	t.Skip("needs proxy models, which the ORM does not have")
}

// drDeferredCopy returns a SimpleItem loaded with value deferred, with its
// primary key cleared, Django's DeferCopyInstanceTests.setUpTestData.
func drDeferredCopy(t *testing.T, db *orm.DB) *drSimpleItem {
	t.Helper()
	create(t, db, &drSimpleItem{Name: "test", Value: 42})
	item := dfFirst(t, orm.Query[drSimpleItem](db).Defer("value"))
	item.ID = 0
	return &item
}

// Ported from Django: tests/defer_regress/tests.py::DeferCopyInstanceTests.test_save
func TestDeferRegressCopyInstanceSave(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		item := drDeferredCopy(t, db)
		const msg = "cannot retrieve deferred field 'value' from an unsaved model"
		assertErrorContains(t, orm.Query[drSimpleItem](db).Create(ctx, item), msg)
		assertErrorContains(t, orm.Query[drSimpleItem](db).Save(ctx, item), msg)
	})
}

// Ported from Django: tests/defer_regress/tests.py::DeferCopyInstanceTests.test_bulk_create
func TestDeferRegressCopyInstanceBulkCreate(t *testing.T) {
	djangotest.EachDB(t, drModels(), func(t *testing.T, db *orm.DB) {
		item := drDeferredCopy(t, db)
		err := orm.Query[drSimpleItem](db).BulkCreate(context.Background(), []*drSimpleItem{item})
		assertErrorContains(t, err, "cannot retrieve deferred field 'value' from an unsaved model")
	})
}
