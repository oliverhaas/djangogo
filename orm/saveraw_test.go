package orm_test

import (
	"context"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// sigPerson is Django's signals.Person. It embeds orm.RelCache for the Defer
// step of TestSaveSignalsRaw.
type sigPerson struct {
	orm.RelCache
	ID        int64
	FirstName string `orm:"max_length=20"`
	LastName  string `orm:"max_length=20"`
}

// rawStampedItem carries a field with a default, an auto_now_add timestamp, and
// an auto_now timestamp so tests can verify that SaveRaw skips defaults and
// automatic timestamp stamping.
type rawStampedItem struct {
	ID        int64
	Title     string    `orm:"max_length=50;default=Untitled"`
	Count     int64     `orm:"default=7"`
	CreatedAt time.Time `orm:"auto_now_add"`
	UpdatedAt time.Time `orm:"auto_now"`
}

// rawSignalEvent records one PreSave or PostSave call in a save signal test.
type rawSignalEvent struct {
	post      bool
	id        int64
	firstName string
	lastName  string
	created   bool
	raw       bool
}

// saveRawModels returns the models for the SaveRaw and save-signal tests.
func saveRawModels() []any {
	return []any{djangotest.App("signals", &sigPerson{}, &rawStampedItem{}, &rawCodedItem{})}
}

// rawCodedItem has a small integer primary key, which is not an auto field.
type rawCodedItem struct {
	Code  int16  `orm:"pk"`
	Label string `orm:"max_length=20"`
}

// Ported from Django: tests/signals/tests.py::SignalTests.test_save_signals
// Check that PreSave and PostSave report IsRaw true for SaveRaw and false for Create and Save, with Created set for inserts.
func TestSaveSignalsRaw(t *testing.T) {
	djangotest.EachDB(t, saveRawModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		var data []rawSignalEvent

		cancelPre := orm.OnPreSave(func(ctx context.Context, p *sigPerson) error {
			data = append(data, rawSignalEvent{
				post:      false,
				id:        p.ID,
				firstName: p.FirstName,
				lastName:  p.LastName,
				raw:       orm.IsRaw(ctx),
			})
			return nil
		})
		t.Cleanup(cancelPre)

		cancelPost := orm.OnPostSave(func(ctx context.Context, p *sigPerson) error {
			data = append(data, rawSignalEvent{
				post:      true,
				id:        p.ID,
				firstName: p.FirstName,
				lastName:  p.LastName,
				created:   orm.Created(ctx),
				raw:       orm.IsRaw(ctx),
			})
			return nil
		})
		t.Cleanup(cancelPost)

		p1 := create(t, db, &sigPerson{FirstName: "John", LastName: "Smith"})
		djangotest.AssertEqual(t, data, []rawSignalEvent{
			{post: false, id: 0, firstName: "John", lastName: "Smith", raw: false},
			{post: true, id: p1.ID, firstName: "John", lastName: "Smith", created: true, raw: false},
		})
		data = nil

		p1.FirstName = "Tom"
		if err := orm.Query[sigPerson](db).Save(ctx, p1); err != nil {
			t.Fatalf("Save(p1): %v", err)
		}
		djangotest.AssertEqual(t, data, []rawSignalEvent{
			{post: false, id: p1.ID, firstName: "Tom", lastName: "Smith", raw: false},
			{post: true, id: p1.ID, firstName: "Tom", lastName: "Smith", created: false, raw: false},
		})
		data = nil

		if err := orm.SaveRaw(ctx, db, p1); err != nil {
			t.Fatalf("SaveRaw(p1): %v", err)
		}
		djangotest.AssertEqual(t, data, []rawSignalEvent{
			{post: false, id: p1.ID, firstName: "Tom", lastName: "Smith", raw: true},
			{post: true, id: p1.ID, firstName: "Tom", lastName: "Smith", created: false, raw: true},
		})
		data = nil

		p2 := &sigPerson{ID: 99999, FirstName: "James", LastName: "Jones"}
		if err := orm.Query[sigPerson](db).Save(ctx, p2); err != nil {
			t.Fatalf("Save(p2 99999): %v", err)
		}
		djangotest.AssertEqual(t, data, []rawSignalEvent{
			{post: false, id: 99999, firstName: "James", lastName: "Jones", raw: false},
			{post: true, id: 99999, firstName: "James", lastName: "Jones", created: true, raw: false},
		})
		data = nil

		p2.ID = 99998
		if err := orm.Query[sigPerson](db).Save(ctx, p2); err != nil {
			t.Fatalf("Save(p2 99998): %v", err)
		}
		djangotest.AssertEqual(t, data, []rawSignalEvent{
			{post: false, id: 99998, firstName: "James", lastName: "Jones", raw: false},
			{post: true, id: 99998, firstName: "James", lastName: "Jones", created: true, raw: false},
		})
		data = nil

		p3, err := orm.Query[sigPerson](db).Defer("first_name").Get(ctx, "pk", p1.ID)
		if err != nil {
			t.Fatalf("Defer Get: %v", err)
		}
		p3.LastName = "Reese"
		if err := orm.Query[sigPerson](db).Save(ctx, &p3); err != nil {
			t.Fatalf("Save(p3): %v", err)
		}
		djangotest.AssertEqual(t, data, []rawSignalEvent{
			{post: false, id: p1.ID, firstName: "", lastName: "Reese", raw: false},
			{post: true, id: p1.ID, firstName: "", lastName: "Reese", created: false, raw: false},
		})
	})
}

// No Django test exists: SaveRaw inserts without a pk, updates an existing pk,
// and inserts a missing explicit pk without applying field defaults or auto_now
// stamps, and fires PreSave and PostSave with IsRaw true and Created set.
func TestSaveRawInsertUpdateAndExplicitPK(t *testing.T) {
	djangotest.EachDB(t, saveRawModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		var events []rawSignalEvent

		cancelPre := orm.OnPreSave(func(ctx context.Context, item *rawStampedItem) error {
			events = append(events, rawSignalEvent{
				post:    false,
				id:      item.ID,
				raw:     orm.IsRaw(ctx),
				created: orm.Created(ctx),
			})
			return nil
		})
		t.Cleanup(cancelPre)

		cancelPost := orm.OnPostSave(func(ctx context.Context, item *rawStampedItem) error {
			events = append(events, rawSignalEvent{
				post:    true,
				id:      item.ID,
				raw:     orm.IsRaw(ctx),
				created: orm.Created(ctx),
			})
			return nil
		})
		t.Cleanup(cancelPost)

		t1 := day(2020, 1, 2)
		t2 := day(2020, 3, 4)

		first := &rawStampedItem{CreatedAt: t1, UpdatedAt: t2}
		if err := orm.SaveRaw(ctx, db, first); err != nil {
			t.Fatalf("SaveRaw(first): %v", err)
		}
		if first.ID == 0 {
			t.Fatal("SaveRaw did not write back generated primary key")
		}
		djangotest.AssertEqual(t, events, []rawSignalEvent{
			{post: false, id: 0, raw: true, created: false},
			{post: true, id: first.ID, raw: true, created: true},
		})
		events = nil

		gotFirst, err := orm.Query[rawStampedItem](db).Get(ctx, "pk", first.ID)
		if err != nil {
			t.Fatalf("Get(first): %v", err)
		}
		djangotest.AssertEqual(t, gotFirst.Title, "")
		djangotest.AssertEqual(t, gotFirst.Count, int64(0))
		djangotest.AssertEqual(t, gotFirst.CreatedAt.Equal(t1), true)
		djangotest.AssertEqual(t, gotFirst.UpdatedAt.Equal(t2), true)

		first.Title = "Updated raw"
		if err := orm.SaveRaw(ctx, db, first); err != nil {
			t.Fatalf("SaveRaw(update first): %v", err)
		}
		djangotest.AssertEqual(t, events, []rawSignalEvent{
			{post: false, id: first.ID, raw: true, created: false},
			{post: true, id: first.ID, raw: true, created: false},
		})
		events = nil

		gotUpdated, err := orm.Query[rawStampedItem](db).Get(ctx, "pk", first.ID)
		if err != nil {
			t.Fatalf("Get(updated first): %v", err)
		}
		djangotest.AssertEqual(t, gotUpdated.Title, "Updated raw")
		djangotest.AssertEqual(t, gotUpdated.Count, int64(0))
		djangotest.AssertEqual(t, gotUpdated.CreatedAt.Equal(t1), true)
		djangotest.AssertEqual(t, gotUpdated.UpdatedAt.Equal(t2), true)

		t3 := day(2021, 5, 6)
		explicit := &rawStampedItem{ID: 42, CreatedAt: t3, UpdatedAt: t3}
		if err := orm.SaveRaw(ctx, db, explicit); err != nil {
			t.Fatalf("SaveRaw(explicit): %v", err)
		}
		djangotest.AssertEqual(t, explicit.ID, int64(42))
		djangotest.AssertEqual(t, events, []rawSignalEvent{
			{post: false, id: 42, raw: true, created: false},
			{post: true, id: 42, raw: true, created: true},
		})

		gotExplicit, err := orm.Query[rawStampedItem](db).Get(ctx, "pk", 42)
		if err != nil {
			t.Fatalf("Get(explicit): %v", err)
		}
		djangotest.AssertEqual(t, gotExplicit.Title, "")
		djangotest.AssertEqual(t, gotExplicit.Count, int64(0))
		djangotest.AssertEqual(t, gotExplicit.CreatedAt.Equal(t3), true)
		djangotest.AssertEqual(t, gotExplicit.UpdatedAt.Equal(t3), true)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[rawStampedItem](db)), int64(2))
	})
}

// No Django test exists: SaveRaw treats a zero primary key that is not an auto
// field as set, so a second raw save of the key 0 updates its row instead of
// inserting a duplicate.
func TestSaveRawZeroExplicitPK(t *testing.T) {
	djangotest.EachDB(t, saveRawModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		item := &rawCodedItem{Code: 0, Label: "first"}
		if err := orm.SaveRaw(ctx, db, item); err != nil {
			t.Fatalf("SaveRaw(insert): %v", err)
		}
		item.Label = "second"
		if err := orm.SaveRaw(ctx, db, item); err != nil {
			t.Fatalf("SaveRaw(update): %v", err)
		}
		got, err := orm.Query[rawCodedItem](db).Get(ctx, "pk", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.Label, "second")
		djangotest.AssertEqual(t, mustCount(t, orm.Query[rawCodedItem](db)), int64(1))
	})
}
