package serializers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// naturalThing is a self-referential natural-key model for forward-reference
// and natural-primary-key tests.
type naturalThing struct {
	orm.RelCache
	ID          int64
	Key         string                `orm:"max_length=100;unique"`
	OtherThing  orm.FK[naturalThing]  `orm:"null;on_delete=cascade"`
	OtherThings orm.M2M[naturalThing] `orm:"related_name=thing_m2m_set;symmetrical=false"`
}

// NaturalKey returns the single-element natural key of naturalThing.
func (n naturalThing) NaturalKey() []any {
	return []any{n.Key}
}

// GetByNaturalKey looks up a naturalThing by key.
func (*naturalThing) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 1 {
		return nil, orm.ErrDoesNotExist
	}
	row, err := orm.Query[naturalThing](db).Get(ctx, "key", key[0])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// depStore is a natural-key model with no dependencies for SortDependencies tests.
type depStore struct {
	ID   int64
	Name string `orm:"max_length=50;unique"`
}

// NaturalKey returns the store name tuple.
func (s depStore) NaturalKey() []any { return []any{s.Name} }

// depPerson is a natural-key model for SortDependencies tests.
type depPerson struct {
	ID   int64
	Name string `orm:"max_length=50;unique"`
}

// NaturalKey returns the person name tuple.
func (p depPerson) NaturalKey() []any { return []any{p.Name} }

// depBook depends on depPerson via FK and depStore via M2M and explicit
// NaturalKeyDependencies.
type depBook struct {
	ID     int64
	Name   string            `orm:"max_length=50"`
	Author orm.FK[depPerson] `orm:"on_delete=cascade"`
	Stores orm.M2M[depStore]
}

// NaturalKey returns the book name tuple.
func (b depBook) NaturalKey() []any { return []any{b.Name} }

// NaturalKeyDependencies declares explicit dependencies for depBook.
func (depBook) NaturalKeyDependencies() []string {
	return []string{"serializers.depStore", "serializers.depPerson"}
}

// allKindsRecord exercises valueFromField and toPython across all ORM field kinds.
type allKindsRecord struct {
	ID       int64
	Small    int16  `orm:"type=smallint"`
	Big      int64  `orm:"type=bigint"`
	PosSmall uint16 `orm:"type=positive_smallint"`
	PosInt   uint32 `orm:"type=positive_int"`
	PosBig   uint64 `orm:"type=positive_bigint"`
	Flag     bool   `orm:"default=true"`
	OptFlag  *bool
	Dec      orm.Decimal `orm:"max_digits=6;decimal_places=2"`
	Day      orm.Date
	Clock    orm.Time
	Span     time.Duration
	UID      orm.UUID
	Payload  json.RawMessage
	Blob     []byte
	IP       netip.Addr
}

// No Django test exists: ToObjects, FromObjects, NaturalPrimaryKeys,
// HandleForwardReferences, SaveDeferredFields, IgnoreNonexistent, and
// DeserializedObject.String roundtrip across SQLite and PostgreSQL.
func TestPythonObjectsNaturalKeysAndDeferredFields(t *testing.T) {
	models := []any{djangotest.App("serializers", &naturalThing{}, &allKindsRecord{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		t1 := mustCreate(t, db, &naturalThing{Key: "t1"})
		t2 := mustCreate(t, db, &naturalThing{Key: "t2", OtherThing: fkTo[naturalThing](t1.ID)})
		t1.OtherThing = fkTo[naturalThing](t2.ID)
		if err := orm.Query[naturalThing](db).Save(ctx, t1); err != nil {
			t.Fatalf("Save(t1): %v", err)
		}
		if err := orm.Many[naturalThing](db, t1, "other_things").Set(ctx, []*naturalThing{t2}); err != nil {
			t.Fatalf("Set(other_things): %v", err)
		}

		objs, err := serializers.ToObjects(
			ctx, db,
			[]*naturalThing{t1, t2},
			serializers.NaturalPrimaryKeys(),
			serializers.NaturalForeignKeys(),
		)
		if err != nil {
			t.Fatalf("ToObjects: %v", err)
		}
		djangotest.AssertEqual(t, len(objs), 2)
		djangotest.AssertEqual(t, objs[0].HasPK, false)

		if _, err := orm.Query[naturalThing](db).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		seq := func(yield func(*serializers.Object, error) bool) {
			for _, o := range objs {
				if !yield(o, nil) {
					return
				}
			}
		}
		var withDeferred []*serializers.DeserializedObject
		for dObj, err := range serializers.FromObjects(ctx, db, seq, serializers.HandleForwardReferences()) {
			if err != nil {
				t.Fatalf("FromObjects: %v", err)
			}
			if err := dObj.Save(ctx, db); err != nil {
				t.Fatalf("Save: %v", err)
			}
			if len(dObj.DeferredFields) > 0 {
				withDeferred = append(withDeferred, dObj)
			}
		}
		djangotest.AssertEqual(t, len(withDeferred), 1)
		for _, dObj := range withDeferred {
			if err := dObj.SaveDeferredFields(ctx, db); err != nil {
				t.Fatalf("SaveDeferredFields: %v", err)
			}
		}

		gotT1, err := orm.Query[naturalThing](db).Get(ctx, "key", "t1")
		if err != nil {
			t.Fatalf("Get(t1): %v", err)
		}
		gotT2, err := orm.Query[naturalThing](db).Get(ctx, "key", "t2")
		if err != nil {
			t.Fatalf("Get(t2): %v", err)
		}
		djangotest.AssertEqual(t, gotT1.OtherThing.PK(), gotT2.ID)
		djangotest.AssertEqual(t, gotT2.OtherThing.PK(), gotT1.ID)
		related, err := orm.Many[naturalThing](db, &gotT1, "other_things").All(ctx)
		if err != nil {
			t.Fatalf("Many(other_things): %v", err)
		}
		djangotest.AssertEqual(t, len(related), 1)
		djangotest.AssertEqual(t, related[0].ID, gotT2.ID)

		thingModel, _ := db.Registry().ModelOf(gotT1)
		dRepr := (&serializers.DeserializedObject{Object: &gotT1, Model: thingModel}).String()
		djangotest.AssertEqual(t, strings.HasPrefix(dRepr, "<DeserializedObject: serializers.naturalThing(pk="), true)

		// IgnoreNonexistent skips unknown models and unknown fields.
		withUnknown := `[
			{"model": "serializers.no_such_model", "pk": 1, "fields": {"key": "x"}},
			{"model": "serializers.naturalthing", "fields": {"key": "t1", "removed_field": "ignored"}}
		]`
		deserialized, err := collectDeserialized(ctx, db, "json", withUnknown, serializers.IgnoreNonexistent())
		if err != nil {
			t.Fatalf("Deserialize IgnoreNonexistent: %v", err)
		}
		djangotest.AssertEqual(t, len(deserialized), 1)
		djangotest.AssertEqual(t, deserialized[0].Object.(*naturalThing).ID, gotT1.ID)
	})
}

// No Django test exists: all extended field kinds (Decimal, Date, Time,
// Duration, UUID, JSON, Binary, GenericIPAddress, and integer variants)
// roundtrip through JSON serialization and deserialization.
func TestAllFieldKindsJSONRoundtrip(t *testing.T) {
	models := []any{djangotest.App("serializers", &allKindsRecord{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		optFalse := false
		uid := orm.MustUUID("12345678-1234-5678-1234-567812345678")
		rec := mustCreate(t, db, &allKindsRecord{
			Small:    -12,
			Big:      9000000000,
			PosSmall: 12,
			PosInt:   345,
			PosBig:   678,
			Flag:     true,
			OptFlag:  &optFalse,
			Dec:      orm.MustDecimal("12.34"),
			Day:      orm.NewDate(2024, time.May, 10),
			Clock:    orm.NewTime(14, 30, 15, 123000000),
			Span:     26*time.Hour + 3*time.Second + 500*time.Microsecond,
			UID:      uid,
			Payload:  json.RawMessage(`{"a":1}`),
			Blob:     []byte{0x05, 0xfd, 0x00},
			IP:       netip.MustParseAddr("2001:db8::1"),
		})

		rawJSON, err := serializers.Serialize(ctx, db, "json", []*allKindsRecord{rec}, serializers.Indent(2))
		if err != nil {
			t.Fatalf("Serialize: %v", err)
		}
		list, err := collectDeserialized(ctx, db, "json", rawJSON)
		if err != nil {
			t.Fatalf("Deserialize: %v", err)
		}
		djangotest.AssertEqual(t, len(list), 1)
		got := list[0].Object.(*allKindsRecord)
		djangotest.AssertEqual(t, got.Small, int16(-12))
		djangotest.AssertEqual(t, got.Big, int64(9000000000))
		djangotest.AssertEqual(t, got.PosSmall, uint16(12))
		djangotest.AssertEqual(t, got.PosInt, uint32(345))
		djangotest.AssertEqual(t, got.PosBig, uint64(678))
		djangotest.AssertEqual(t, got.Flag, true)
		djangotest.AssertEqual(t, *got.OptFlag, false)
		djangotest.AssertEqual(t, got.Dec.Equal(orm.MustDecimal("12.34")), true)
		djangotest.AssertEqual(t, got.Day, orm.NewDate(2024, time.May, 10))
		djangotest.AssertEqual(t, got.Clock, orm.NewTime(14, 30, 15, 123000000))
		djangotest.AssertEqual(t, got.Span, 26*time.Hour+3*time.Second+500*time.Microsecond)
		djangotest.AssertEqual(t, got.UID, uid)
		djangotest.AssertEqual(t, string(got.Payload), `{"a": 1}`)
		djangotest.AssertEqual(t, bytes.Equal(got.Blob, []byte{0x05, 0xfd, 0x00}), true)
		djangotest.AssertEqual(t, got.IP, netip.MustParseAddr("2001:db8::1"))
	})
}

// No Django test exists: SortDependencies orders natural-key dependencies and
// reports unresolvable cycles unless allowCycles is true.
func TestSortDependenciesAndProgressBarSingle(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("serializers",
		&depBook{},
		&depPerson{},
		&depStore{},
		&naturalThing{},
	))
	bookModel, _ := reg.Get("serializers.depBook")
	personModel, _ := reg.Get("serializers.depPerson")
	storeModel, _ := reg.Get("serializers.depStore")
	thingModel, _ := reg.Get("serializers.naturalThing")

	sorted, err := serializers.SortDependencies(reg, []*orm.Model{bookModel, personModel, storeModel})
	if err != nil {
		t.Fatalf("SortDependencies: %v", err)
	}
	djangotest.AssertEqual(t, slices.Index(sorted, storeModel) < slices.Index(sorted, bookModel), true)
	djangotest.AssertEqual(t, slices.Index(sorted, personModel) < slices.Index(sorted, bookModel), true)

	// Self-referential natural-key dependencies are allowed without error.
	sortedSelf, err := serializers.SortDependencies(reg, []*orm.Model{thingModel})
	if err != nil {
		t.Fatalf("SortDependencies(self): %v", err)
	}
	djangotest.AssertEqual(t, len(sortedSelf), 1)

	// ProgressBar with totalCount=1 omits the leading "\r".
	var buf bytes.Buffer
	pb := serializers.NewProgressBar(&buf, 1)
	pb.Update(1)
	djangotest.AssertEqual(t, buf.String(), "["+strings.Repeat(".", serializers.ProgressWidth)+"]\n")
}
