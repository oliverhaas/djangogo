package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/prefetch_related/models.py (House, Room, Person).

type prHouse struct {
	orm.RelCache
	ID        int64
	Name      string            `orm:"max_length=50"`
	Address   string            `orm:"max_length=255"`
	Owner     orm.FK[prPerson]  `orm:"null;on_delete=set_null"`
	Occupants orm.M2M[prPerson] `orm:"related_name=houses"`
}

func (prHouse) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prRoom struct {
	orm.RelCache
	ID         int64
	Name       string           `orm:"max_length=50"`
	House      orm.FK[prHouse]  `orm:"on_delete=cascade;related_name=rooms"`
	MainRoomOf orm.O2O[prHouse] `orm:"null;on_delete=set_null;related_name=main_room"`
}

func (prRoom) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prPerson struct {
	orm.RelCache
	ID   int64
	Name string `orm:"max_length=50"`
}

func (prPerson) Meta() orm.Meta { return orm.Meta{Ordering: []string{"id"}} }

type prCustomFixture struct {
	person1, person2               *prPerson
	house1, house2, house3, house4 *prHouse
	room1_1, room1_2, room1_3      *prRoom
	room2_1, room3_1, room4_1      *prRoom
}

// prCustomSetUp populates the House/Room/Person fixture from CustomPrefetchTests.setUpTestData.
func prCustomSetUp(t *testing.T, db *orm.DB) prCustomFixture {
	t.Helper()
	ctx := context.Background()
	p1 := create(t, db, &prPerson{Name: "Joe"})
	p2 := create(t, db, &prPerson{Name: "Mary"})

	makeHouse := func(name, addr string, owner *prPerson) (*prHouse, *prRoom, *prRoom, *prRoom) {
		h := create(t, db, &prHouse{Name: name, Address: addr, Owner: fkTo[prPerson](owner.ID)})
		r1 := create(t, db, &prRoom{Name: "Dining room", House: fkTo[prHouse](h.ID), MainRoomOf: o2oTo[prHouse](h.ID)})
		r2 := create(t, db, &prRoom{Name: "Lounge", House: fkTo[prHouse](h.ID)})
		r3 := create(t, db, &prRoom{Name: "Kitchen", House: fkTo[prHouse](h.ID)})
		if err := orm.Many[prHouse](db, owner, "houses").Add(ctx, h); err != nil {
			t.Fatalf("owner.houses.Add: %v", err)
		}
		return h, r1, r2, r3
	}

	h1, r11, r12, r13 := makeHouse("House 1", "123 Main St", p1)
	h2, r21, _, _ := makeHouse("House 2", "45 Side St", p1)
	h3, r31, _, _ := makeHouse("House 3", "6 Downing St", p2)
	h4, r41, _, _ := makeHouse("house 4", "7 Regents St", p2)

	return prCustomFixture{
		person1: p1, person2: p2,
		house1: h1, house2: h2, house3: h3, house4: h4,
		room1_1: r11, room1_2: r12, room1_3: r13,
		room2_1: r21, room3_1: r31, room4_1: r41,
	}
}

func prHouseNames(rows []prHouse) []string {
	out := make([]string, len(rows))
	for i, h := range rows {
		out[i] = h.Name
	}
	return out
}

func prRoomNames(rows []prRoom) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}

func prPersonNames(rows []prPerson) []string {
	out := make([]string, len(rows))
	for i, p := range rows {
		out[i] = p.Name
	}
	return out
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_ambiguous
func TestPrefetchRelatedCustomAmbiguous(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prCustomSetUp(t, db)

		// Lookup was already seen with a different queryset; custom queryset should not be evaluated.
		djangotest.AssertNumQueries(t, db, 3, func() {
			_, err := orm.Query[prPerson](db).PrefetchRelated(
				"houses__rooms",
				orm.Prefetch{Lookup: "houses", QuerySet: orm.Query[prHouse](db)},
			).All(ctx)
			assertErrorContains(t, err, "'houses' lookup was already seen with a different queryset")
		})

		// Lookup houses_lst does not yet exist when performing houses_lst__rooms.
		_, err := orm.Query[prPerson](db).PrefetchRelated(
			"houses_lst__rooms",
			orm.Prefetch{Lookup: "houses", QuerySet: orm.Query[prHouse](db), ToAttr: "houses_lst"},
		).All(ctx)
		assertErrorContains(t, err, "Cannot find 'houses_lst' on prPerson object, 'houses_lst__rooms' is an invalid parameter to prefetch_related()")

		// Non-ambiguous orderings succeed.
		mustAll(t, orm.Query[prPerson](db).PrefetchRelated("houses__rooms", "houses"))
		mustAll(t, orm.Query[prPerson](db).PrefetchRelated(
			"houses__rooms",
			orm.Prefetch{Lookup: "houses", QuerySet: orm.Query[prHouse](db), ToAttr: "houses_lst"},
		))
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_m2m
func TestPrefetchRelatedCustomM2M(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prCustomSetUp(t, db)

		var want [][]string
		djangotest.AssertNumQueries(t, db, 2, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated("houses"))
			for i := range people {
				houses, err := orm.Many[prHouse](db, &people[i], "houses").All(ctx)
				if err != nil {
					t.Fatalf("houses.All: %v", err)
				}
				want = append(want, prHouseNames(houses))
			}
		})

		djangotest.AssertNumQueries(t, db, 2, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{Lookup: "houses"}))
			got := make([][]string, 0, len(people))
			for i := range people {
				houses, err := orm.Many[prHouse](db, &people[i], "houses").All(ctx)
				if err != nil {
					t.Fatalf("houses.All: %v", err)
				}
				got = append(got, prHouseNames(houses))
			}
			djangotest.AssertEqual(t, got, want)
		})

		djangotest.AssertNumQueries(t, db, 2, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{Lookup: "houses", ToAttr: "houses_lst"}))
			got := make([][]string, 0, len(people))
			for i := range people {
				houses, ok := orm.Prefetched[prHouse](&people[i], "houses_lst")
				djangotest.AssertEqual(t, ok, true)
				got = append(got, prHouseNames(houses))
			}
			djangotest.AssertEqual(t, got, want)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_reverse_m2m
func TestPrefetchRelatedCustomReverseM2M(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prCustomSetUp(t, db)

		var want [][]string
		djangotest.AssertNumQueries(t, db, 2, func() {
			houses := mustAll(t, orm.Query[prHouse](db).PrefetchRelated("occupants"))
			for i := range houses {
				occ, err := orm.Many[prPerson](db, &houses[i], "occupants").All(ctx)
				if err != nil {
					t.Fatalf("occupants.All: %v", err)
				}
				want = append(want, prPersonNames(occ))
			}
		})

		djangotest.AssertNumQueries(t, db, 2, func() {
			houses := mustAll(t, orm.Query[prHouse](db).PrefetchRelated(orm.Prefetch{Lookup: "occupants", ToAttr: "occupants_lst"}))
			got := make([][]string, 0, len(houses))
			for i := range houses {
				occ, ok := orm.Prefetched[prPerson](&houses[i], "occupants_lst")
				djangotest.AssertEqual(t, ok, true)
				got = append(got, prPersonNames(occ))
			}
			djangotest.AssertEqual(t, got, want)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_m2m_through_fk
func TestPrefetchRelatedCustomM2MThroughFK(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prCustomSetUp(t, db)

		var want [][]string
		djangotest.AssertNumQueries(t, db, 3, func() {
			rooms := mustAll(t, orm.Query[prRoom](db).PrefetchRelated("house__occupants"))
			for i := range rooms {
				h, err := rooms[i].House.Fetch(ctx, db)
				if err != nil {
					t.Fatalf("House.Fetch: %v", err)
				}
				occ, err := orm.Many[prPerson](db, h, "occupants").All(ctx)
				if err != nil {
					t.Fatalf("occupants.All: %v", err)
				}
				want = append(want, prPersonNames(occ))
			}
		})

		djangotest.AssertNumQueries(t, db, 3, func() {
			rooms := mustAll(t, orm.Query[prRoom](db).PrefetchRelated(orm.Prefetch{
				Lookup: "house__occupants",
				ToAttr: "occupants_lst",
			}))
			got := make([][]string, 0, len(rooms))
			for i := range rooms {
				h, err := rooms[i].House.Fetch(ctx, db)
				if err != nil {
					t.Fatalf("House.Fetch: %v", err)
				}
				occ, ok := orm.Prefetched[prPerson](h, "occupants_lst")
				djangotest.AssertEqual(t, ok, true)
				got = append(got, prPersonNames(occ))
			}
			djangotest.AssertEqual(t, got, want)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_o2m_through_m2m
func TestPrefetchRelatedCustomO2MThroughM2M(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prCustomSetUp(t, db)

		var want [][][]string
		djangotest.AssertNumQueries(t, db, 3, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated("houses", "houses__rooms"))
			for i := range people {
				houses, err := orm.Many[prHouse](db, &people[i], "houses").All(ctx)
				if err != nil {
					t.Fatalf("houses.All: %v", err)
				}
				byHouse := make([][]string, 0, len(houses))
				for j := range houses {
					rooms, err := orm.RelatedSet[prRoom](db, &houses[j], "rooms").All(ctx)
					if err != nil {
						t.Fatalf("rooms.All: %v", err)
					}
					byHouse = append(byHouse, prRoomNames(rooms))
				}
				want = append(want, byHouse)
			}
		})

		djangotest.AssertNumQueries(t, db, 3, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated(
				orm.Prefetch{Lookup: "houses", ToAttr: "houses_lst"},
				orm.Prefetch{Lookup: "houses_lst__rooms", ToAttr: "rooms_lst"},
			))
			got := make([][][]string, 0, len(people))
			for i := range people {
				houses, ok := orm.Prefetched[prHouse](&people[i], "houses_lst")
				djangotest.AssertEqual(t, ok, true)
				var byHouse [][]string
				for j := range houses {
					rooms, ok := orm.Prefetched[prRoom](&houses[j], "rooms_lst")
					djangotest.AssertEqual(t, ok, true)
					byHouse = append(byHouse, prRoomNames(rooms))
				}
				got = append(got, byHouse)
			}
			djangotest.AssertEqual(t, got, want)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_custom_qs
func TestPrefetchRelatedCustomQS(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prCustomSetUp(t, db)

		// Filtered custom queryset with to_attr.
		djangotest.AssertNumQueries(t, db, 2, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{
				Lookup:   "houses",
				QuerySet: orm.Query[prHouse](db).Filter("pk__in", []int64{fx.house1.ID, fx.house3.ID}),
				ToAttr:   "houses_lst",
			}))
			h0, ok0 := orm.Prefetched[prHouse](&people[0], "houses_lst")
			h1, ok1 := orm.Prefetched[prHouse](&people[1], "houses_lst")
			djangotest.AssertEqual(t, ok0, true)
			djangotest.AssertEqual(t, ok1, true)
			djangotest.AssertEqual(t, prHouseNames(h0), []string{"House 1"})
			djangotest.AssertEqual(t, prHouseNames(h1), []string{"House 3"})
		})

		// Inner select_related on custom queryset cuts queries from 3 to 2.
		djangotest.AssertNumQueries(t, db, 2, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{
				Lookup:   "houses",
				QuerySet: orm.Query[prHouse](db).SelectRelated("owner"),
			}))
			houses, err := orm.Many[prHouse](db, &people[0], "houses").All(ctx)
			if err != nil {
				t.Fatalf("houses.All: %v", err)
			}
			owner, err := houses[0].Owner.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Owner.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, owner.Name, "Joe")
		})

		// Inner prefetch_related on custom queryset followed by outer prefetch on to_attr chain.
		innerRoomsQS := orm.Query[prRoom](db).Filter("pk__in", []int64{fx.room1_1.ID, fx.room1_2.ID})
		housesQS := orm.Query[prHouse](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "rooms",
			QuerySet: innerRoomsQS,
			ToAttr:   "rooms_lst",
		})
		var people []prPerson
		djangotest.AssertNumQueries(t, db, 4, func() {
			people = mustAll(t, orm.Query[prPerson](db).PrefetchRelated(
				orm.Prefetch{
					Lookup:   "houses",
					QuerySet: housesQS.Filter("pk", fx.house1.ID),
					ToAttr:   "houses_lst",
				},
				orm.Prefetch{Lookup: "houses_lst__rooms_lst__main_room_of"},
			))
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			h0, _ := orm.Prefetched[prHouse](&people[0], "houses_lst")
			djangotest.AssertEqual(t, len(h0), 1)
			r0, _ := orm.Prefetched[prRoom](&h0[0], "rooms_lst")
			djangotest.AssertEqual(t, len(r0), 2)
			djangotest.AssertEqual(t, r0[0].ID, fx.room1_1.ID)
			djangotest.AssertEqual(t, r0[1].ID, fx.room1_2.ID)
			mainOf, err := r0[0].MainRoomOf.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("MainRoomOf.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, mainOf.ID, fx.house1.ID)
			h1, _ := orm.Prefetched[prHouse](&people[1], "houses_lst")
			djangotest.AssertEqual(t, len(h1), 0)
		})

		// Forward FK custom queryset with SelectRelated and to_attr.
		housesWithOwner := orm.Query[prHouse](db).SelectRelated("owner")
		djangotest.AssertNumQueries(t, db, 2, func() {
			rooms := mustAll(t, orm.Query[prRoom](db).PrefetchRelated(orm.Prefetch{
				Lookup:   "house",
				QuerySet: housesWithOwner,
			}))
			h, err := rooms[0].House.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("House.Fetch: %v", err)
			}
			owner, err := h.Owner.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Owner.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, owner.Name, "Joe")
		})
		djangotest.AssertNumQueries(t, db, 2, func() {
			rooms := mustAll(t, orm.Query[prRoom](db).PrefetchRelated(orm.Prefetch{
				Lookup:   "house",
				QuerySet: housesWithOwner,
				ToAttr:   "house_attr",
			}))
			h, ok := orm.PrefetchedOne[prHouse](&rooms[0], "house_attr")
			djangotest.AssertEqual(t, ok, true)
			owner, err := h.Owner.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Owner.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, owner.Name, "Joe")
		})

		// Forward FK custom queryset filtering out target: Fetch returns ErrDoesNotExist (0 queries), to_attr is nil.
		room, err := orm.Query[prRoom](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "house",
			QuerySet: housesWithOwner.Filter("address", "DoesNotExist"),
		}).First(ctx)
		if err != nil {
			t.Fatalf("First: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := room.House.Fetch(ctx, db)
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
		roomAttr, err := orm.Query[prRoom](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "house",
			QuerySet: housesWithOwner.Filter("address", "DoesNotExist"),
			ToAttr:   "house_attr",
		}).First(ctx)
		if err != nil {
			t.Fatalf("First to_attr: %v", err)
		}
		gotHouse, ok := orm.PrefetchedOne[prHouse](&roomAttr, "house_attr")
		djangotest.AssertEqual(t, ok, true)
		djangotest.AssertEqual(t, gotHouse, nil)

		// Forward O2O (main_room_of) and Reverse O2O (main_room) custom querysets with missing targets.
		djangotest.AssertNumQueries(t, db, 2, func() {
			rooms := mustAll(t, orm.Query[prRoom](db).PrefetchRelated(orm.Prefetch{
				Lookup:   "main_room_of",
				QuerySet: housesWithOwner,
				ToAttr:   "main_room_of_attr",
			}))
			h, ok := orm.PrefetchedOne[prHouse](&rooms[0], "main_room_of_attr")
			djangotest.AssertEqual(t, ok, true)
			owner, err := h.Owner.Fetch(ctx, db)
			if err != nil {
				t.Fatalf("Owner.Fetch: %v", err)
			}
			djangotest.AssertEqual(t, owner.Name, "Joe")
		})
		roomMissing, err := orm.Query[prRoom](db).
			Filter("main_room_of__isnull", false).
			PrefetchRelated(orm.Prefetch{
				Lookup:   "main_room_of",
				QuerySet: housesWithOwner.Filter("address", "DoesNotExist"),
			}).First(ctx)
		if err != nil {
			t.Fatalf("First forward O2O missing: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := roomMissing.MainRoomOf.Fetch(ctx, db)
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
		houseMissingRev, err := orm.Query[prHouse](db).
			PrefetchRelated(orm.Prefetch{
				Lookup:   "main_room",
				QuerySet: orm.Query[prRoom](db).Filter("name", "DoesNotExist"),
			}).First(ctx)
		if err != nil {
			t.Fatalf("First reverse O2O missing: %v", err)
		}
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, err := orm.RelatedOne[prRoom](ctx, db, &houseMissingRev, "main_room")
			djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
		})
		houseMissingRevAttr, err := orm.Query[prHouse](db).
			PrefetchRelated(orm.Prefetch{
				Lookup:   "main_room",
				QuerySet: orm.Query[prRoom](db).Filter("name", "DoesNotExist"),
				ToAttr:   "main_room_attr",
			}).First(ctx)
		if err != nil {
			t.Fatalf("First reverse O2O missing to_attr: %v", err)
		}
		gotMain, ok := orm.PrefetchedOne[prRoom](&houseMissingRevAttr, "main_room_attr")
		djangotest.AssertEqual(t, ok, true)
		djangotest.AssertEqual(t, gotMain, nil)
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_nested_prefetch_related_are_not_overwritten
func TestPrefetchRelatedNestedNotOverwritten(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prCustomSetUp(t, db)

		houses2 := orm.Query[prHouse](db).PrefetchRelated(orm.Prefetch{Lookup: "rooms"})
		persons := orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{Lookup: "houses", QuerySet: houses2})
		housesQS := orm.Query[prHouse](db).PrefetchRelated(orm.Prefetch{Lookup: "occupants", QuerySet: persons})

		houses := mustAll(t, housesQS)
		djangotest.AssertNumQueries(t, db, 0, func() {
			occ, err := orm.Many[prPerson](db, &houses[0], "occupants").All(ctx)
			if err != nil {
				t.Fatalf("occupants.All: %v", err)
			}
			pHouses, err := orm.Many[prHouse](db, &occ[0], "houses").All(ctx)
			if err != nil {
				t.Fatalf("houses.All: %v", err)
			}
			rooms, err := orm.RelatedSet[prRoom](db, &pHouses[1], "rooms").All(ctx)
			if err != nil {
				t.Fatalf("rooms.All: %v", err)
			}
			djangotest.AssertEqual(t, rooms[0].ID, fx.room2_1.ID)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_nested_prefetch_related_with_duplicate_prefetcher
func TestPrefetchRelatedNestedDuplicatePrefetcher(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prCustomSetUp(t, db)

		occupants := orm.Query[prPerson](db).PrefetchRelated(
			orm.Prefetch{Lookup: "houses", ToAttr: "some_attr_name"},
			orm.Prefetch{Lookup: "houses", QuerySet: orm.Query[prHouse](db).PrefetchRelated("main_room")},
		)
		housesQS := orm.Query[prHouse](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "occupants",
			QuerySet: occupants,
		})
		djangotest.AssertNumQueries(t, db, 5, func() {
			houses := mustAll(t, housesQS)
			occ, err := orm.Many[prPerson](db, &houses[0], "occupants").All(ctx)
			if err != nil {
				t.Fatalf("occupants.All: %v", err)
			}
			pHouses, err := orm.Many[prHouse](db, &occ[0], "houses").All(ctx)
			if err != nil {
				t.Fatalf("houses.All: %v", err)
			}
			mr, err := orm.RelatedOne[prRoom](ctx, db, &pHouses[0], "main_room")
			if err != nil {
				t.Fatalf("RelatedOne main_room: %v", err)
			}
			djangotest.AssertEqual(t, mr.Name, "Dining room")
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_nested_prefetch_related_with_duplicate_prefetch_and_depth
func TestPrefetchRelatedNestedDuplicateAndDepth(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fx := prCustomSetUp(t, db)

		djangotest.AssertNumQueries(t, db, 4, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated(
				orm.Prefetch{
					Lookup:   "houses__main_room",
					QuerySet: orm.Query[prRoom](db).Filter("name", "Dining room"),
					ToAttr:   "dining_room",
				},
				"houses__main_room",
			))
			houses, err := orm.Many[prHouse](db, &people[0], "houses").All(ctx)
			if err != nil {
				t.Fatalf("houses.All: %v", err)
			}
			mr, err := orm.RelatedOne[prRoom](ctx, db, &houses[0], "main_room")
			if err != nil {
				t.Fatalf("RelatedOne main_room: %v", err)
			}
			djangotest.AssertEqual(t, mr.ID, fx.room1_1.ID)
			dr, ok := orm.PrefetchedOne[prRoom](&houses[0], "dining_room")
			djangotest.AssertEqual(t, ok, true)
			djangotest.AssertEqual(t, dr.ID, fx.room1_1.ID)
		})

		djangotest.AssertNumQueries(t, db, 4, func() {
			people := mustAll(t, orm.Query[prPerson](db).PrefetchRelated(
				"houses__main_room",
				orm.Prefetch{
					Lookup:   "houses__main_room",
					QuerySet: orm.Query[prRoom](db).Filter("name", "Dining room"),
					ToAttr:   "dining_room",
				},
			))
			houses, err := orm.Many[prHouse](db, &people[0], "houses").All(ctx)
			if err != nil {
				t.Fatalf("houses.All: %v", err)
			}
			mr, err := orm.RelatedOne[prRoom](ctx, db, &houses[0], "main_room")
			if err != nil {
				t.Fatalf("RelatedOne main_room: %v", err)
			}
			djangotest.AssertEqual(t, mr.ID, fx.room1_1.ID)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_values_queryset
// Ported from Django: tests/prefetch_related/tests.py::CustomPrefetchTests.test_raw_queryset
func TestPrefetchRelatedInvalidCustomQuerySet(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prCustomSetUp(t, db)
		const wantMsg = "Prefetch querysets cannot use raw(), values(), and values_list()"

		_, err := orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "houses",
			QuerySet: orm.Query[prHouse](db).Values("pk"),
		}).All(ctx)
		assertErrorContains(t, err, wantMsg)

		_, err = orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "houses",
			QuerySet: orm.Query[prHouse](db).ValuesList("pk"),
		}).All(ctx)
		assertErrorContains(t, err, wantMsg)

		_, err = orm.Query[prPerson](db).PrefetchRelated(orm.Prefetch{
			Lookup:   "houses",
			QuerySet: orm.Raw[prHouse](db, "SELECT id FROM prefetch_related_prhouse"),
		}).All(ctx)
		assertErrorContains(t, err, wantMsg)
	})
}

// Ported from Django: tests/prefetch_related/tests.py::DirectPrefetchedObjectCacheReuseTests.test_detect_is_fetched
// Ported from Django: tests/prefetch_related/tests.py::DirectPrefetchedObjectCacheReuseTests.test_detect_is_fetched_with_to_attr
func TestPrefetchRelatedDetectIsFetched(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		b1 := create(t, db, &prBook{Title: "book1"})
		b2 := create(t, db, &prBook{Title: "book2"})
		a11 := create(t, db, &prAuthor{Name: "Author11", FirstBook: fkTo[prBook](b1.ID)})
		a12 := create(t, db, &prAuthor{Name: "Author12", FirstBook: fkTo[prBook](b1.ID)})
		a21 := create(t, db, &prAuthor{Name: "Author21", FirstBook: fkTo[prBook](b2.ID)})
		addr1 := create(t, db, &prAuthorAddress{Author: fkTo[prAuthor](a11.ID), Address: "Happy place"})
		create(t, db, &prAuthorAddress{Author: fkTo[prAuthor](a12.ID), Address: "Haunted house"})
		addr3 := create(t, db, &prAuthorAddress{Author: fkTo[prAuthor](a21.ID), Address: "Happy place"})

		var books []prBook
		djangotest.AssertNumQueries(t, db, 3, func() {
			books = mustAll(t, orm.Query[prBook](db).
				Filter("title__in", []string{"book1", "book2"}).
				PrefetchRelated(orm.Prefetch{
					Lookup: "first_time_authors",
					QuerySet: orm.Query[prAuthor](db).PrefetchRelated(orm.Prefetch{
						Lookup:   "addresses",
						QuerySet: orm.Query[prAuthorAddress](db).Filter("address", "Happy place"),
					}),
				}))
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			fa1, err := orm.RelatedSet[prAuthor](db, &books[0], "first_time_authors").All(ctx)
			if err != nil {
				t.Fatalf("fa1: %v", err)
			}
			fa2, err := orm.RelatedSet[prAuthor](db, &books[1], "first_time_authors").All(ctx)
			if err != nil {
				t.Fatalf("fa2: %v", err)
			}
			djangotest.AssertEqual(t, prAuthorNames(fa1), []string{"Author11", "Author12"})
			djangotest.AssertEqual(t, prAuthorNames(fa2), []string{"Author21"})

			ad11, _ := orm.RelatedSet[prAuthorAddress](db, &fa1[0], "addresses").All(ctx)
			ad12, _ := orm.RelatedSet[prAuthorAddress](db, &fa1[1], "addresses").All(ctx)
			ad21, _ := orm.RelatedSet[prAuthorAddress](db, &fa2[0], "addresses").All(ctx)
			djangotest.AssertEqual(t, len(ad11), 1)
			djangotest.AssertEqual(t, ad11[0].ID, addr1.ID)
			djangotest.AssertEqual(t, len(ad12), 0)
			djangotest.AssertEqual(t, len(ad21), 1)
			djangotest.AssertEqual(t, ad21[0].ID, addr3.ID)
		})

		// Same with to_attr at both levels.
		djangotest.AssertNumQueries(t, db, 3, func() {
			books = mustAll(t, orm.Query[prBook](db).
				Filter("title__in", []string{"book1", "book2"}).
				PrefetchRelated(orm.Prefetch{
					Lookup: "first_time_authors",
					QuerySet: orm.Query[prAuthor](db).PrefetchRelated(orm.Prefetch{
						Lookup:   "addresses",
						QuerySet: orm.Query[prAuthorAddress](db).Filter("address", "Happy place"),
						ToAttr:   "happy_place",
					}),
					ToAttr: "first_authors",
				}))
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			fa1, ok1 := orm.Prefetched[prAuthor](&books[0], "first_authors")
			fa2, ok2 := orm.Prefetched[prAuthor](&books[1], "first_authors")
			djangotest.AssertEqual(t, ok1, true)
			djangotest.AssertEqual(t, ok2, true)
			djangotest.AssertEqual(t, prAuthorNames(fa1), []string{"Author11", "Author12"})
			djangotest.AssertEqual(t, prAuthorNames(fa2), []string{"Author21"})

			ad11, _ := orm.Prefetched[prAuthorAddress](&fa1[0], "happy_place")
			ad12, _ := orm.Prefetched[prAuthorAddress](&fa1[1], "happy_place")
			ad21, _ := orm.Prefetched[prAuthorAddress](&fa2[0], "happy_place")
			djangotest.AssertEqual(t, len(ad11), 1)
			djangotest.AssertEqual(t, ad11[0].ID, addr1.ID)
			djangotest.AssertEqual(t, len(ad12), 0)
			djangotest.AssertEqual(t, len(ad21), 1)
			djangotest.AssertEqual(t, ad21[0].ID, addr3.ID)
		})
	})
}

// Ported from Django: tests/prefetch_related/tests.py::NestedPrefetchTests.test_nested_prefetch_is_not_overwritten_by_related_object
func TestPrefetchRelatedNestedNotOverwrittenByBackRef(t *testing.T) {
	djangotest.EachDB(t, prModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		h := create(t, db, &prHouse{Name: "Big house", Address: "123 Main St"})
		create(t, db, &prRoom{Name: "Kitchen", House: fkTo[prHouse](h.ID)})

		var house prHouse
		djangotest.AssertNumQueries(t, db, 3, func() {
			var err error
			house, err = orm.Query[prHouse](db).PrefetchRelated(orm.Prefetch{
				Lookup: "rooms",
				QuerySet: orm.Query[prRoom](db).PrefetchRelated(orm.Prefetch{
					Lookup:   "house",
					QuerySet: orm.Query[prHouse](db),
				}),
			}).First(ctx)
			if err != nil {
				t.Fatalf("First: %v", err)
			}
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			firstRoom, err := orm.RelatedSet[prRoom](db, &house, "rooms").First(ctx)
			if err != nil {
				t.Fatalf("rooms.First: %v", err)
			}
			gotHouse, loaded := firstRoom.House.Object()
			djangotest.AssertEqual(t, loaded, true)
			djangotest.AssertEqual(t, gotHouse.Address, "123 Main St")
		})
	})
}
