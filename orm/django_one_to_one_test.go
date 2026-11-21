package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/one_to_one/models.py.

type o2oPlace struct {
	orm.RelCache
	ID      int64
	Name    string `orm:"max_length=50"`
	Address string `orm:"max_length=80"`
}

type o2oRestaurant struct {
	orm.RelCache
	Place         orm.O2O[o2oPlace] `orm:"pk;on_delete=cascade;related_name=restaurant"`
	ServesHotDogs bool              `orm:"default=false"`
	ServesPizza   bool              `orm:"default=false"`
}

type o2oBar struct {
	ID              int64
	Place           orm.O2O[o2oPlace] `orm:"on_delete=cascade;related_name=bar"`
	ServesCocktails bool              `orm:"default=true"`
}

type o2oUndergroundBar struct {
	ID              int64
	Place           orm.O2O[o2oPlace] `orm:"null;on_delete=set_null;related_name=undergroundbar"`
	ServesCocktails bool              `orm:"default=true"`
}

type o2oWaiter struct {
	ID         int64
	Restaurant orm.FK[o2oRestaurant] `orm:"on_delete=cascade;related_name=waiter_set"`
	Name       string                `orm:"max_length=50"`
}

type o2oOtherPlace struct {
	orm.RelCache
	ID   int64
	Name string `orm:"max_length=50"`
}

type o2oMultiModel struct {
	ID    int64
	Link1 orm.O2O[o2oPlace]      `orm:"on_delete=cascade;related_name=multimodel"`
	Link2 orm.O2O[o2oOtherPlace] `orm:"on_delete=cascade;related_name=multimodel"`
	Name  string                 `orm:"max_length=50"`
}

type o2oTarget struct {
	orm.RelCache
	ID   int64
	Name string `orm:"max_length=50;unique"`
}

type o2oPointer struct {
	Other orm.O2O[o2oTarget] `orm:"pk;on_delete=cascade;related_name=pointer"`
}

type o2oPointer2 struct {
	ID    int64
	Other orm.O2O[o2oTarget] `orm:"on_delete=cascade;related_name=second_pointer"`
}

type o2oHiddenPointer struct {
	ID     int64
	Target orm.O2O[o2oTarget] `orm:"on_delete=cascade;related_name=hidden+"`
}

// o2oTo returns a OneToOne value pointing at pk.
func o2oTo[T any](pk int64) orm.O2O[T] {
	var o orm.O2O[T]
	o.SetPK(pk)
	return o
}

// o2oModels returns the model list for the one_to_one test app.
func o2oModels() []any {
	return []any{djangotest.App("one_to_one",
		&o2oPlace{}, &o2oRestaurant{}, &o2oBar{}, &o2oUndergroundBar{},
		&o2oWaiter{}, &o2oOtherPlace{}, &o2oMultiModel{}, &o2oTarget{},
		&o2oPointer{}, &o2oPointer2{}, &o2oHiddenPointer{},
	)}
}

// o2oFixture holds the shared setUpTestData rows for OneToOneTests.
type o2oFixture struct {
	p1, p2 *o2oPlace
	r1     *o2oRestaurant
	b1     *o2oBar
}

// o2oSetUp creates the base rows from OneToOneTests.setUpTestData.
func o2oSetUp(t *testing.T, db *orm.DB) o2oFixture {
	t.Helper()
	p1 := create(t, db, &o2oPlace{Name: "Demon Dogs", Address: "944 W. Fullerton"})
	p2 := create(t, db, &o2oPlace{Name: "Ace Hardware", Address: "1013 N. Ashland"})
	r1 := create(t, db, &o2oRestaurant{Place: o2oTo[o2oPlace](p1.ID), ServesHotDogs: true, ServesPizza: false})
	b1 := create(t, db, &o2oBar{Place: o2oTo[o2oPlace](p1.ID), ServesCocktails: false})
	return o2oFixture{p1: p1, p2: p2, r1: r1, b1: b1}
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_getter
func TestOneToOneGetter(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)

		// Forward O2O fetch.
		p, err := f.r1.Place.Fetch(ctx, db)
		if err != nil {
			t.Fatalf("Fetch place: %v", err)
		}
		djangotest.AssertEqual(t, p.Name, "Demon Dogs")

		// Reverse O2O fetch.
		r, err := orm.RelatedOne[o2oRestaurant](ctx, db, f.p1, "restaurant")
		if err != nil {
			t.Fatalf("RelatedOne restaurant: %v", err)
		}
		djangotest.AssertEqual(t, r.Place.PK(), f.p1.ID)
		djangotest.AssertEqual(t, r.ServesHotDogs, true)

		// Missing reverse O2O returns ErrDoesNotExist with Django's wording.
		_, err = orm.RelatedOne[o2oRestaurant](ctx, db, f.p2, "restaurant")
		djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		assertErrorContains(t, err, "o2oPlace has no restaurant")
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_setter
func TestOneToOneSetter(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)

		// Reassigning PK O2O and calling Save inserts a new row for p2.
		f.r1.Place.SetPK(f.p2.ID)
		if err := orm.Query[o2oRestaurant](db).Save(ctx, f.r1); err != nil {
			t.Fatalf("Save r1 on p2: %v", err)
		}
		r2, err := orm.RelatedOne[o2oRestaurant](ctx, db, f.p2, "restaurant")
		if err != nil {
			t.Fatalf("RelatedOne p2.restaurant: %v", err)
		}
		djangotest.AssertEqual(t, r2.Place.PK(), f.p2.ID)
		p, err := f.r1.Place.Fetch(ctx, db)
		if err != nil {
			t.Fatalf("r1.Place.Fetch: %v", err)
		}
		djangotest.AssertEqual(t, p.Name, "Ace Hardware")

		// Original restaurant for p1 still exists.
		r1Loaded, err := orm.Query[o2oRestaurant](db).Get(ctx, "pk", f.p1.ID)
		if err != nil {
			t.Fatalf("Get r1: %v", err)
		}
		p1Loaded, err := r1Loaded.Place.Fetch(ctx, db)
		if err != nil {
			t.Fatalf("r1Loaded.Place.Fetch: %v", err)
		}
		djangotest.AssertEqual(t, p1Loaded.Name, "Demon Dogs")
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_manager_all
func TestOneToOneManagerAll(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		f := o2oSetUp(t, db)
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oRestaurant](db),
			[]int64{f.r1.Place.PK()}, func(r o2oRestaurant) int64 { return r.Place.PK() })
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oPlace](db).OrderBy("name"),
			[]string{"Ace Hardware", "Demon Dogs"}, func(p o2oPlace) string { return p.Name })
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_manager_get
func TestOneToOneManagerGet(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)

		assertGetRestaurant := func(args ...any) {
			t.Helper()
			r, err := orm.Query[o2oRestaurant](db).Get(ctx, args...)
			if err != nil {
				t.Fatalf("Restaurant.Get(%v): %v", args, err)
			}
			djangotest.AssertEqual(t, r.Place.PK(), f.p1.ID)
		}

		assertGetRestaurant("place__id__exact", f.p1.ID)
		assertGetRestaurant("place__id", f.p1.ID)
		assertGetRestaurant("place__exact", f.p1.ID)
		assertGetRestaurant("place__exact", f.p1)
		assertGetRestaurant("place", f.p1.ID)
		assertGetRestaurant("place", f.p1)
		assertGetRestaurant("pk", f.p1.ID)
		assertGetRestaurant("place__pk__exact", f.p1.ID)
		assertGetRestaurant("place__pk", f.p1.ID)
		assertGetRestaurant("place__name__startswith", "Demon")

		assertGetPlace := func(args ...any) {
			t.Helper()
			p, err := orm.Query[o2oPlace](db).Get(ctx, args...)
			if err != nil {
				t.Fatalf("Place.Get(%v): %v", args, err)
			}
			djangotest.AssertEqual(t, p.ID, f.p1.ID)
		}

		assertGetPlace("restaurant__place__exact", f.p1.ID)
		assertGetPlace("restaurant__place__exact", f.p1)
		assertGetPlace("restaurant__place__pk", f.p1.ID)
		assertGetPlace("restaurant__exact", f.p1.ID)
		assertGetPlace("restaurant__exact", f.r1)
		assertGetPlace("restaurant__pk", f.p1.ID)
		assertGetPlace("restaurant", f.p1.ID)
		assertGetPlace("restaurant", f.r1)
		assertGetPlace("id__exact", f.p1.ID)
		assertGetPlace("pk", f.p1.ID)
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_foreign_key
func TestOneToOneForeignKey(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)
		w := &o2oWaiter{Name: "Joe"}
		if err := orm.RelatedSet[o2oWaiter](db, f.r1, "waiter_set").Create(ctx, w); err != nil {
			t.Fatalf("Create waiter: %v", err)
		}

		assertWaiters := func(args ...any) {
			t.Helper()
			djangotest.AssertQuerySetEqual(t, orm.Query[o2oWaiter](db).Filter(args...),
				[]int64{w.ID}, func(x o2oWaiter) int64 { return x.ID })
		}

		assertWaiters("restaurant__place__exact", f.p1.ID)
		assertWaiters("restaurant__place__exact", f.p1)
		assertWaiters("restaurant__place__pk", f.p1.ID)
		assertWaiters("restaurant__exact", f.r1.Place.PK())
		assertWaiters("restaurant__exact", f.r1)
		assertWaiters("restaurant__pk", f.r1.Place.PK())
		assertWaiters("restaurant", f.r1.Place.PK())
		assertWaiters("restaurant", f.r1)
		assertWaiters("id__exact", w.ID)
		assertWaiters("pk", w.ID)

		// Deleting the restaurant cascades to the waiter.
		if _, err := orm.Query[o2oRestaurant](db).Filter("pk", f.r1.Place.PK()).Delete(ctx); err != nil {
			t.Fatalf("Delete restaurant: %v", err)
		}
		djangotest.AssertEqual(t, mustCount(t, orm.Query[o2oWaiter](db)), int64(0))
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_multiple_o2o
func TestOneToOneMultipleO2O(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)
		o1 := create(t, db, &o2oOtherPlace{Name: "primary"})
		x1 := create(t, db, &o2oMultiModel{
			Link1: o2oTo[o2oPlace](f.p1.ID),
			Link2: o2oTo[o2oOtherPlace](o1.ID),
			Name:  "x1",
		})
		got, err := orm.RelatedOne[o2oMultiModel](ctx, db, o1, "multimodel")
		if err != nil {
			t.Fatalf("RelatedOne multimodel: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, x1.ID)

		// Violating uniqueness on Link2 raises ErrIntegrity / ErrUniqueViolation.
		dup := &o2oMultiModel{
			Link1: o2oTo[o2oPlace](f.p2.ID),
			Link2: o2oTo[o2oOtherPlace](o1.ID),
			Name:  "dup",
		}
		err = orm.Query[o2oMultiModel](db).Create(ctx, dup)
		djangotest.AssertErrorIs(t, err, orm.ErrUniqueViolation)
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_unsaved_object
func TestOneToOneUnsavedObject(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		place := &o2oPlace{Name: "User", Address: "London"}
		_, err := orm.RelatedOne[o2oRestaurant](ctx, db, place, "restaurant")
		djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)

		r := &o2oRestaurant{ServesHotDogs: true}
		r.Place.SetObject(place, 0)
		err = orm.Query[o2oRestaurant](db).Create(ctx, r)
		assertErrorContains(t, err, "unsaved related object 'place'")
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_reverse_object_cache
func TestOneToOneReverseObjectCache(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)
		r, err := orm.RelatedOne[o2oRestaurant](ctx, db, f.p1, "restaurant")
		if err != nil {
			t.Fatalf("restaurant: %v", err)
		}
		b, err := orm.RelatedOne[o2oBar](ctx, db, f.p1, "bar")
		if err != nil {
			t.Fatalf("bar: %v", err)
		}
		djangotest.AssertEqual(t, r.Place.PK(), f.r1.Place.PK())
		djangotest.AssertEqual(t, b.ID, f.b1.ID)
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_related_object_cache
func TestOneToOneRelatedObjectCache(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		o2oSetUp(t, db)
		p, err := orm.Query[o2oPlace](db).Get(ctx, "name", "Demon Dogs")
		if err != nil {
			t.Fatalf("Get place: %v", err)
		}
		r1, err := orm.RelatedOne[o2oRestaurant](ctx, db, &p, "restaurant")
		if err != nil {
			t.Fatalf("RelatedOne 1: %v", err)
		}
		// Second access uses cache without hitting the database.
		djangotest.AssertNumQueries(t, db, 0, func() {
			r2, err := orm.RelatedOne[o2oRestaurant](ctx, db, &p, "restaurant")
			if err != nil {
				t.Fatalf("RelatedOne 2: %v", err)
			}
			if r1 != r2 {
				t.Fatal("expected cached pointer identity")
			}
		})
		// Clearing cache triggers a fresh fetch.
		p.ClearRelation("restaurant")
		djangotest.AssertNumQueries(t, db, 1, func() {
			r3, err := orm.RelatedOne[o2oRestaurant](ctx, db, &p, "restaurant")
			if err != nil {
				t.Fatalf("RelatedOne 3: %v", err)
			}
			if r1 == r3 {
				t.Fatal("expected a fresh object after clearing cache")
			}
		})
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_filter_one_to_one_relations
func TestOneToOneFilterOneToOneRelations(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		target := create(t, db, &o2oTarget{Name: "t1"})
		tid := func(x o2oTarget) int64 { return x.ID }
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oTarget](db).Filter("pointer", nil), []int64{target.ID}, tid)
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oTarget](db).Exclude("pointer", nil), []int64{}, tid)
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oTarget](db).Filter("second_pointer", nil), []int64{target.ID}, tid)
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oTarget](db).Exclude("second_pointer", nil), []int64{}, tid)
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_o2o_primary_key_delete
func TestOneToOnePrimaryKeyDelete(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		target := create(t, db, &o2oTarget{Name: "name"})
		create(t, db, &o2oPointer{Other: o2oTo[o2oTarget](target.ID)})
		n, err := orm.Query[o2oPointer](db).Filter("other__name", "name").Delete(ctx)
		if err != nil {
			t.Fatalf("Delete pointer: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(1))
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_save_nullable_o2o_after_parent
func TestOneToOneSaveNullableO2OAfterParent(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		place := &o2oPlace{Name: "Rose tattoo"}
		bar := &o2oUndergroundBar{}
		bar.Place.SetObject(place, 0)
		if err := orm.Query[o2oPlace](db).Save(ctx, place); err != nil {
			t.Fatalf("Save place: %v", err)
		}
		if err := orm.Query[o2oUndergroundBar](db).Save(ctx, bar); err != nil {
			t.Fatalf("Save bar: %v", err)
		}
		loaded, err := orm.Query[o2oUndergroundBar](db).Get(ctx, "pk", bar.ID)
		if err != nil {
			t.Fatalf("Get bar: %v", err)
		}
		djangotest.AssertEqual(t, loaded.Place.PK(), place.ID)
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_reverse_object_does_not_exist_cache
func TestOneToOneReverseObjectDoesNotExistCache(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p := create(t, db, &o2oPlace{Name: "Zombie Cats", Address: "Not sure"})
		djangotest.AssertNumQueries(t, db, 1, func() {
			_, err := orm.RelatedOne[o2oRestaurant](ctx, db, p, "restaurant")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := orm.RelatedOne[o2oRestaurant](ctx, db, p, "restaurant")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_related_object_cached_when_reverse_is_accessed
func TestOneToOneRelatedObjectCachedWhenReverseAccessed(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)
		p, err := orm.Query[o2oPlace](db).Get(ctx, "pk", f.p1.ID)
		if err != nil {
			t.Fatalf("Get place: %v", err)
		}
		r, err := orm.RelatedOne[o2oRestaurant](ctx, db, &p, "restaurant")
		if err != nil {
			t.Fatalf("RelatedOne: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			back, err := r.Place.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("r.Place.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, back.Name, p.Name)
		})
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_get_reverse_on_unsaved_object
func TestOneToOneGetReverseOnUnsavedObject(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p := &o2oPlace{}
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := orm.RelatedOne[o2oUndergroundBar](ctx, db, p, "undergroundbar")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
		create(t, db, &o2oUndergroundBar{})
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := orm.RelatedOne[o2oUndergroundBar](ctx, db, p, "undergroundbar")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
		create(t, db, &o2oUndergroundBar{})
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := orm.RelatedOne[o2oUndergroundBar](ctx, db, p, "undergroundbar")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_nullable_o2o_delete
func TestOneToOneNullableO2ODelete(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)
		u := create(t, db, &o2oUndergroundBar{Place: o2oTo[o2oPlace](f.p1.ID)})
		u.Place.SetPK(0)
		if err := orm.Query[o2oUndergroundBar](db).Save(ctx, u); err != nil {
			t.Fatalf("Save u: %v", err)
		}
		if _, err := orm.Query[o2oPlace](db).Filter("pk", f.p1.ID).Delete(ctx); err != nil {
			t.Fatalf("Delete p1: %v", err)
		}
		loaded, err := orm.Query[o2oUndergroundBar](db).Get(ctx, "pk", u.ID)
		if err != nil {
			t.Fatalf("Get u: %v", err)
		}
		djangotest.AssertEqual(t, loaded.Place.PK(), int64(0))
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_hidden_accessor
func TestOneToOneHiddenAccessor(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		m, ok := db.Registry().ModelOf(o2oTarget{})
		if !ok {
			t.Fatal("o2oTarget not registered")
		}
		if _, found := m.ReverseRelation("hidden+"); found {
			t.Fatal("expected hidden+ reverse relation not to be exposed")
		}
		if _, found := m.ReverseRelation("hidden"); found {
			t.Fatal("expected hidden reverse relation not to be exposed")
		}
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_get_or_create_reverse_o2o_error
func TestOneToOneGetOrCreateReverseO2OError(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)
		r2 := create(t, db, &o2oRestaurant{Place: o2oTo[o2oPlace](f.p2.ID), ServesHotDogs: true})
		_, _, err := orm.Query[o2oPlace](db).GetOrCreate(ctx, orm.Defaults{"restaurant": r2}, "name", "nonexistent")
		assertFieldError(t, err, "restaurant")
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_update_or_create_reverse_o2o_error
func TestOneToOneUpdateOrCreateReverseO2OError(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		f := o2oSetUp(t, db)
		r2 := create(t, db, &o2oRestaurant{Place: o2oTo[o2oPlace](f.p2.ID), ServesHotDogs: true})
		_, _, err := orm.Query[o2oPlace](db).UpdateOrCreate(ctx, orm.Defaults{"restaurant": r2}, "name", "nonexistent")
		assertFieldError(t, err, "restaurant")
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_update_one_to_one_pk
func TestOneToOneUpdateOneToOnePK(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p1 := create(t, db, &o2oPlace{Name: "p1"})
		p2 := create(t, db, &o2oPlace{Name: "p2"})
		r1 := create(t, db, &o2oRestaurant{Place: o2oTo[o2oPlace](p1.ID)})
		r2 := create(t, db, &o2oRestaurant{Place: o2oTo[o2oPlace](p2.ID)})
		w := create(t, db, &o2oWaiter{Restaurant: fkTo[o2oRestaurant](r1.Place.PK()), Name: "Bob"})

		if _, err := orm.Query[o2oWaiter](db).Update(ctx, "restaurant", r2); err != nil {
			t.Fatalf("Update waiter: %v", err)
		}
		loaded, err := orm.Query[o2oWaiter](db).Get(ctx, "pk", w.ID)
		if err != nil {
			t.Fatalf("Get waiter: %v", err)
		}
		djangotest.AssertEqual(t, loaded.Restaurant.PK(), r2.Place.PK())
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_rel_pk_subquery
func TestOneToOneRelPKSubquery(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		o2oSetUp(t, db)
		r, err := orm.Query[o2oRestaurant](db).First(ctx)
		if err != nil {
			t.Fatalf("First restaurant: %v", err)
		}
		rid := func(x o2oRestaurant) int64 { return x.Place.PK() }
		want := []int64{r.Place.PK()}

		q1 := orm.Query[o2oRestaurant](db).Filter("place_id", r.Place.PK())
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oRestaurant](db).Filter("place_id__in", q1), want, rid)
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oRestaurant](db).Filter("pk__in", orm.Query[o2oRestaurant](db).Filter("place__id", r.Place.PK())), want, rid)
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oRestaurant](db).Filter("place__in", orm.Query[o2oPlace](db)), want, rid)
		djangotest.AssertQuerySetEqual(t, orm.Query[o2oRestaurant](db).Filter("place__in", orm.Query[o2oPlace](db).Filter("id", r.Place.PK())), want, rid)
	})
}

// Ported from Django: tests/one_to_one/tests.py::OneToOneTests.test_rel_pk_exact
func TestOneToOneRelPKExact(t *testing.T) {
	djangotest.EachDB(t, o2oModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		o2oSetUp(t, db)
		r, err := orm.Query[o2oRestaurant](db).First(ctx)
		if err != nil {
			t.Fatalf("First: %v", err)
		}
		r2, err := orm.Query[o2oRestaurant](db).Filter("pk__exact", r).First(ctx)
		if err != nil {
			t.Fatalf("Filter pk__exact r: %v", err)
		}
		djangotest.AssertEqual(t, r2.Place.PK(), r.Place.PK())
	})
}
