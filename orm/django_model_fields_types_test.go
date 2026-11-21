package orm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

type durationModel struct {
	ID    int64
	Field time.Duration
}

type nullDurationModel struct {
	ID    int64
	Field *time.Duration `orm:"null"`
}

type uuidModel struct {
	ID    int64
	Field orm.UUID
}

type nullableUUIDModel struct {
	ID    int64
	Field *orm.UUID `orm:"null"`
}

type primaryKeyUUIDModel struct {
	ID orm.UUID `orm:"pk;default=uuid4"`
}

type jsonModel struct {
	ID    int64
	Value json.RawMessage
}

type nullableJSONModel struct {
	ID    int64
	Value json.RawMessage `orm:"null"`
}

type structJSONPayload struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type structJSONModel struct {
	ID      int64
	Payload structJSONPayload `orm:"type=json"`
}

type dataModel struct {
	ID        int64
	ShortData []byte `orm:"max_length=10;default=0x08"`
	Data      []byte
}

type genericIPAddressModel struct {
	ID int64
	IP *string `orm:"type=ip;null"`
}

type netipAddressModel struct {
	ID int64
	IP netip.Addr
}

type slugModel struct {
	ID int64
	S  string `orm:"type=slug;max_length=255"`
}

type unicodeSlugModel struct {
	ID int64
	S  string `orm:"type=slug;max_length=255;allow_unicode"`
}

type charTextPost struct {
	ID    int64
	Title string `orm:"max_length=100"`
	Body  string `orm:"type=text"`
}

type boolModel struct {
	ID     int64
	BField bool
}

type nullBoolModel struct {
	ID      int64
	NBField *bool `orm:"null"`
}

type fkToBools struct {
	ID  int64
	BF  orm.FK[boolModel]
	NBF orm.FK[nullBoolModel]
}

type dateTimeAllModel struct {
	ID int64
	D  orm.Date
	DT time.Time
	T  orm.Time
}

// Ported from Django: tests/model_fields/test_durationfield.py::TestSaveLoad.test_simple_roundtrip
func TestDurationFieldSimpleRoundtrip(t *testing.T) {
	djangotest.EachDB(t, []any{&durationModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		dur := 8999999999999999 * time.Microsecond
		m := durationModel{Field: dur}
		djangotest.AssertEqual(t, orm.Query[durationModel](db).Create(ctx, &m), nil)
		loaded, err := orm.Query[durationModel](db).Get(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, loaded.Field, dur)
	})
}

// Ported from Django: tests/model_fields/test_durationfield.py::TestSaveLoad.test_create_empty
func TestDurationFieldCreateEmpty(t *testing.T) {
	djangotest.EachDB(t, []any{&nullDurationModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		m := nullDurationModel{}
		djangotest.AssertEqual(t, orm.Query[nullDurationModel](db).Create(ctx, &m), nil)
		loaded, err := orm.Query[nullDurationModel](db).Get(ctx)
		djangotest.AssertEqual(t, err, nil)
		if loaded.Field != nil {
			t.Fatalf("got %v, want nil", *loaded.Field)
		}
	})
}

// Ported from Django: tests/model_fields/test_durationfield.py::TestSaveLoad.test_fractional_seconds
func TestDurationFieldFractionalSeconds(t *testing.T) {
	djangotest.EachDB(t, []any{&durationModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		want := 2*time.Second + 50*time.Millisecond
		m := durationModel{Field: want}
		djangotest.AssertEqual(t, orm.Query[durationModel](db).Create(ctx, &m), nil)
		loaded, err := orm.Query[durationModel](db).Get(ctx, "pk", m.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, loaded.Field, want)
	})
}

// Ported from Django: tests/model_fields/test_durationfield.py::TestQuerying.test_exact
// Ported from Django: tests/model_fields/test_durationfield.py::TestQuerying.test_gt
func TestDurationFieldQuerying(t *testing.T) {
	djangotest.EachDB(t, []any{&durationModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		a := durationModel{Field: 24 * time.Hour}
		b := durationModel{Field: time.Second}
		c := durationModel{Field: -time.Second}
		djangotest.AssertEqual(t, orm.Query[durationModel](db).BulkCreate(ctx, []*durationModel{&a, &b, &c}), nil)

		exact, err := orm.Query[durationModel](db).Filter("field", 24*time.Hour).All(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(exact), 1)
		djangotest.AssertEqual(t, exact[0].ID, a.ID)

		gt, err := orm.Query[durationModel](db).Filter("field__gt", time.Duration(0)).OrderBy("id").All(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(gt), 2)
		djangotest.AssertEqual(t, gt[0].ID, a.ID)
		djangotest.AssertEqual(t, gt[1].ID, b.ID)
	})
}

// Ported from Django: tests/model_fields/test_uuid.py::TestSaveLoad.test_uuid_instance
// Ported from Django: tests/model_fields/test_uuid.py::TestSaveLoad.test_str_instance_no_hyphens
// Ported from Django: tests/model_fields/test_uuid.py::TestSaveLoad.test_str_instance_hyphens
// Ported from Django: tests/model_fields/test_uuid.py::TestSaveLoad.test_str_instance_bad_hyphens
// Ported from Django: tests/model_fields/test_uuid.py::TestSaveLoad.test_null_handling
func TestUUIDFieldSaveLoad(t *testing.T) {
	want := orm.MustUUID("550e8400-e29b-41d4-a716-446655440000")
	for _, raw := range []string{
		"550e8400e29b41d4a716446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e84-00-e29b-41d4-a716-4-466-55440000",
	} {
		parsed, err := orm.ParseUUID(raw)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, parsed, want)
	}

	djangotest.EachDB(t, []any{&uuidModel{}, &nullableUUIDModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		u := orm.NewUUID()
		inst := uuidModel{Field: u}
		djangotest.AssertEqual(t, orm.Query[uuidModel](db).Create(ctx, &inst), nil)
		loaded, err := orm.Query[uuidModel](db).Get(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, loaded.Field, u)

		nm := nullableUUIDModel{}
		djangotest.AssertEqual(t, orm.Query[nullableUUIDModel](db).Create(ctx, &nm), nil)
		nloaded, err := orm.Query[nullableUUIDModel](db).Get(ctx)
		djangotest.AssertEqual(t, err, nil)
		if nloaded.Field != nil {
			t.Fatalf("got %v, want nil", *nloaded.Field)
		}
	})
}

// Ported from Django: tests/model_fields/test_uuid.py::TestSaveLoad.test_wrong_value
func TestUUIDFieldWrongValue(t *testing.T) {
	djangotest.EachDB(t, []any{&uuidModel{}}, func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[uuidModel](db).Get(context.Background(), "field", "not-a-uuid")
		if err == nil || !strings.Contains(err.Error(), "is not a valid UUID") {
			t.Fatalf("got err=%v, want 'is not a valid UUID'", err)
		}
	})
}

// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_exact
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_iexact
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_isnull
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_contains
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_icontains
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_startswith
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_istartswith
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_endswith
// Ported from Django: tests/model_fields/test_uuid.py::TestQuerying.test_iendswith
func TestUUIDFieldQuerying(t *testing.T) {
	djangotest.EachDB(t, []any{&nullableUUIDModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		u0 := orm.MustUUID("25d405be-4895-4d50-9b2e-d6695359ce47")
		u1 := orm.MustUUID("550e8400e29b41d4a716446655440000")
		o0 := nullableUUIDModel{Field: &u0}
		o1 := nullableUUIDModel{Field: &u1}
		o2 := nullableUUIDModel{Field: nil}
		djangotest.AssertEqual(t, orm.Query[nullableUUIDModel](db).BulkCreate(ctx, []*nullableUUIDModel{&o0, &o1, &o2}), nil)

		native := db.Dialect().Name() == "postgres"
		checkCount := func(name string, qs *orm.QuerySet[nullableUUIDModel], want int) {
			t.Helper()
			rows, err := qs.All(ctx)
			djangotest.AssertEqual(t, err, nil)
			if len(rows) != want {
				t.Errorf("%s: got %d rows, want %d", name, len(rows), want)
			}
		}
		noHyphenWant := 1
		if native {
			noHyphenWant = 0
		}

		checkCount("exact_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__exact", "550e8400e29b41d4a716446655440000"), 1)
		checkCount("exact_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__exact", "550e8400-e29b-41d4-a716-446655440000"), 1)
		checkCount("iexact_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__iexact", "550E8400E29B41D4A716446655440000"), noHyphenWant)
		checkCount("iexact_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__iexact", "550E8400-E29B-41D4-A716-446655440000"), 1)
		checkCount("isnull", orm.Query[nullableUUIDModel](db).Filter("field__isnull", true), 1)
		checkCount("contains_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__contains", "8400e29b"), noHyphenWant)
		checkCount("contains_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__contains", "8400-e29b"), 1)
		checkCount("icontains_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__icontains", "8400E29B"), noHyphenWant)
		checkCount("icontains_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__icontains", "8400-E29B"), 1)
		checkCount("startswith_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__startswith", "550e8400e29b4"), noHyphenWant)
		checkCount("startswith_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__startswith", "550e8400-e29b-4"), 1)
		checkCount("istartswith_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__istartswith", "550E8400E29B4"), noHyphenWant)
		checkCount("istartswith_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__istartswith", "550E8400-E29B-4"), 1)
		checkCount("endswith_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__endswith", "a716446655440000"), noHyphenWant)
		checkCount("endswith_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__endswith", "a716-446655440000"), 1)
		checkCount("iendswith_no_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__iendswith", "A716446655440000"), noHyphenWant)
		checkCount("iendswith_hyphens", orm.Query[nullableUUIDModel](db).Filter("field__iendswith", "A716-446655440000"), 1)
	})
}

// Ported from Django: tests/model_fields/test_uuid.py::TestAsPrimaryKey.test_creation
// Ported from Django: tests/model_fields/test_uuid.py::TestAsPrimaryKey.test_uuid_pk_on_bulk_create
func TestUUIDFieldAsPrimaryKey(t *testing.T) {
	djangotest.EachDB(t, []any{&primaryKeyUUIDModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p := primaryKeyUUIDModel{}
		djangotest.AssertEqual(t, orm.Query[primaryKeyUUIDModel](db).Create(ctx, &p), nil)
		if p.ID.IsZero() {
			t.Fatal("expected generated UUID primary key")
		}
		loaded, err := orm.Query[primaryKeyUUIDModel](db).Get(ctx, "pk", p.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, loaded.ID, p.ID)

		u1, u2 := primaryKeyUUIDModel{}, primaryKeyUUIDModel{}
		djangotest.AssertEqual(t, orm.Query[primaryKeyUUIDModel](db).BulkCreate(ctx, []*primaryKeyUUIDModel{&u1, &u2}), nil)
		if u1.ID.IsZero() || u2.ID.IsZero() || u1.ID == u2.ID {
			t.Fatalf("expected two distinct UUIDs, got %v and %v", u1.ID, u2.ID)
		}
		count, err := orm.Query[primaryKeyUUIDModel](db).Count(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, count, int64(3))
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSaveLoad.test_null
// Ported from Django: tests/model_fields/test_jsonfield.py::TestSaveLoad.test_json_null_different_from_sql_null
// Ported from Django: tests/model_fields/test_jsonfield.py::JSONFieldTests.test_db_check_constraints
func TestJSONFieldSaveLoadAndConstraints(t *testing.T) {
	djangotest.EachDB(t, []any{&jsonModel{}, &nullableJSONModel{}, &structJSONModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		// SQL NULL vs JSON null.
		sqlNull := nullableJSONModel{Value: nil}
		djangotest.AssertEqual(t, orm.Query[nullableJSONModel](db).Create(ctx, &sqlNull), nil)
		loadedNull, err := orm.Query[nullableJSONModel](db).Get(ctx, "pk", sqlNull.ID)
		djangotest.AssertEqual(t, err, nil)
		if loadedNull.Value != nil {
			t.Fatalf("got %q, want nil for SQL NULL", string(loadedNull.Value))
		}

		jsonNull := nullableJSONModel{Value: json.RawMessage("null")}
		djangotest.AssertEqual(t, orm.Query[nullableJSONModel](db).Create(ctx, &jsonNull), nil)
		loadedJSONNull, err := orm.Query[nullableJSONModel](db).Get(ctx, "pk", jsonNull.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertJSONEqual(t, string(loadedJSONNull.Value), "null")

		nullRows, err := orm.Query[nullableJSONModel](db).Filter("value__isnull", true).All(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(nullRows), 1)
		djangotest.AssertEqual(t, nullRows[0].ID, sqlNull.ID)

		// Invalid JSON rejected by CHECK constraint (SQLite) or JSONB parser (PostgreSQL).
		bad := nullableJSONModel{Value: json.RawMessage("{@!invalid json value 123 $!@#")}
		if err := orm.Query[nullableJSONModel](db).Create(ctx, &bad); err == nil {
			t.Fatal("expected error inserting invalid JSON")
		}

		// Go struct with orm:"type=json".
		sm := structJSONModel{Payload: structJSONPayload{Name: "alpha", Tags: []string{"a", "b"}}}
		djangotest.AssertEqual(t, orm.Query[structJSONModel](db).Create(ctx, &sm), nil)
		loadedSM, err := orm.Query[structJSONModel](db).Get(ctx, "pk", sm.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, loadedSM.Payload.Name, "alpha")
		djangotest.AssertEqual(t, len(loadedSM.Payload.Tags), 2)
		djangotest.AssertEqual(t, loadedSM.Payload.Tags[1], "b")
	})
}

// Ported from Django: tests/model_fields/test_binaryfield.py::BinaryFieldTests.test_set_and_retrieve
// Ported from Django: tests/model_fields/test_binaryfield.py::BinaryFieldTests.test_editable
// Ported from Django: tests/model_fields/test_binaryfield.py::BinaryFieldTests.test_filter
func TestBinaryField(t *testing.T) {
	reg := djangotest.NewRegistry(t, &dataModel{})
	m, _ := reg.ModelOf(&dataModel{})
	dataField, _ := m.FieldByName("Data")
	if dataField.Editable {
		t.Error("BinaryField should default to Editable=false")
	}

	djangotest.EachDB(t, []any{&dataModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		binaryData := []byte{0x00, 0x46, 0xfe}
		dm := dataModel{Data: binaryData}
		djangotest.AssertEqual(t, orm.Query[dataModel](db).Create(ctx, &dm), nil)

		loaded, err := orm.Query[dataModel](db).Get(ctx, "pk", dm.ID)
		djangotest.AssertEqual(t, err, nil)
		if !bytes.Equal(loaded.Data, binaryData) {
			t.Fatalf("Data = %x, want %x", loaded.Data, binaryData)
		}
		if !bytes.Equal(loaded.ShortData, []byte{0x08}) {
			t.Fatalf("ShortData default = %x, want 08", loaded.ShortData)
		}

		// Resave (=update).
		djangotest.AssertEqual(t, orm.Query[dataModel](db).Save(ctx, &loaded), nil)
		loaded2, err := orm.Query[dataModel](db).Get(ctx, "pk", dm.ID)
		djangotest.AssertEqual(t, err, nil)
		if !bytes.Equal(loaded2.Data, binaryData) {
			t.Fatalf("after resave Data = %x, want %x", loaded2.Data, binaryData)
		}

		// Filter.
		other := dataModel{Data: []byte{0xef, 0xbb, 0xbf}}
		djangotest.AssertEqual(t, orm.Query[dataModel](db).Create(ctx, &other), nil)
		matched, err := orm.Query[dataModel](db).Filter("data", binaryData).All(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(matched), 1)
		djangotest.AssertEqual(t, matched[0].ID, dm.ID)
	})
}

// Ported from Django: tests/model_fields/test_genericipaddressfield.py::GenericIPAddressFieldTests.test_null_value
// Ported from Django: tests/model_fields/test_genericipaddressfield.py::GenericIPAddressFieldTests.test_blank_string_saved_as_null
// Ported from Django: tests/model_fields/test_genericipaddressfield.py::GenericIPAddressFieldTests.test_save_load
// Ported from Django: tests/model_fields/test_genericipaddressfield.py::GenericIPAddressFieldTests.test_save_load_ipaddress
// Ported from Django: tests/model_fields/test_genericipaddressfield.py::GenericIPAddressFieldTests.test_save_load_strips_whitespace
// Ported from Django: tests/model_fields/test_genericipaddressfield.py::GenericIPAddressFieldTests.test_save_invalid_ipv6
func TestGenericIPAddressField(t *testing.T) {
	djangotest.EachDB(t, []any{&genericIPAddressModel{}, &netipAddressModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		// Null value.
		n := genericIPAddressModel{}
		djangotest.AssertEqual(t, orm.Query[genericIPAddressModel](db).Create(ctx, &n), nil)
		loadedN, err := orm.Query[genericIPAddressModel](db).Get(ctx, "pk", n.ID)
		djangotest.AssertEqual(t, err, nil)
		if loadedN.IP != nil {
			t.Fatalf("got %q, want nil", *loadedN.IP)
		}

		// Blank string saved as NULL on create and update.
		empty := ""
		b := genericIPAddressModel{IP: &empty}
		djangotest.AssertEqual(t, orm.Query[genericIPAddressModel](db).Create(ctx, &b), nil)
		loadedB, err := orm.Query[genericIPAddressModel](db).Get(ctx, "pk", b.ID)
		djangotest.AssertEqual(t, err, nil)
		if loadedB.IP != nil {
			t.Fatalf("blank string saved as %q, want nil", *loadedB.IP)
		}
		_, err = orm.Query[genericIPAddressModel](db).Filter("pk", b.ID).Update(ctx, "ip", "")
		djangotest.AssertEqual(t, err, nil)

		// IPv6 save/load and compression.
		v6 := "::1"
		m6 := genericIPAddressModel{IP: &v6}
		djangotest.AssertEqual(t, orm.Query[genericIPAddressModel](db).Create(ctx, &m6), nil)
		loaded6, err := orm.Query[genericIPAddressModel](db).Get(ctx, "pk", m6.ID)
		djangotest.AssertEqual(t, err, nil)
		if loaded6.IP == nil || *loaded6.IP != "::1" {
			t.Fatalf("got %v, want ::1", loaded6.IP)
		}

		// Whitespace stripped on save and lookup.
		padded := " 192.0.2.1 "
		mp := genericIPAddressModel{IP: &padded}
		djangotest.AssertEqual(t, orm.Query[genericIPAddressModel](db).Create(ctx, &mp), nil)
		loadedP, err := orm.Query[genericIPAddressModel](db).Get(ctx, "pk", mp.ID)
		djangotest.AssertEqual(t, err, nil)
		if loadedP.IP == nil || *loadedP.IP != "192.0.2.1" {
			t.Fatalf("got %v, want 192.0.2.1", loadedP.IP)
		}
		foundP, err := orm.Query[genericIPAddressModel](db).Filter("ip", " 192.0.2.1 ").All(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(foundP), 1)

		// Invalid IPv6 rejected.
		bad := "not:valid"
		if err := orm.Query[genericIPAddressModel](db).Create(ctx, &genericIPAddressModel{IP: &bad}); err == nil {
			t.Fatal("expected error for invalid IPv6 address")
		}

		// netip.Addr roundtrip and filter.
		for _, pair := range [][2]netip.Addr{
			{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")},
			{netip.MustParseAddr("2001:db8::1"), netip.MustParseAddr("2001:db8::2")},
		} {
			nm := netipAddressModel{IP: pair[0]}
			djangotest.AssertEqual(t, orm.Query[netipAddressModel](db).Create(ctx, &nm), nil)
			gotNM, err := orm.Query[netipAddressModel](db).Get(ctx, "pk", nm.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, gotNM.IP, pair[0])

			gotNM.IP = pair[1]
			djangotest.AssertEqual(t, orm.Query[netipAddressModel](db).Save(ctx, &gotNM), nil)
			gotNM2, err := orm.Query[netipAddressModel](db).Get(ctx, "pk", nm.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, gotNM2.IP, pair[1])

			_, err = orm.Query[netipAddressModel](db).Filter("pk", nm.ID).Update(ctx, "ip", pair[0])
			djangotest.AssertEqual(t, err, nil)
			matched, err := orm.Query[netipAddressModel](db).Filter("ip", pair[0]).All(ctx)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, len(matched), 1)
		}
	})
}

// Ported from Django: tests/model_fields/test_slugfield.py::SlugFieldTests.test_slugfield_max_length
// Ported from Django: tests/model_fields/test_slugfield.py::SlugFieldTests.test_slugfield_unicode_max_length
func TestSlugFieldMaxLength(t *testing.T) {
	djangotest.EachDB(t, []any{&slugModel{}, &unicodeSlugModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		longSlug := strings.Repeat("slug", 50)
		bs := slugModel{S: longSlug}
		djangotest.AssertEqual(t, orm.Query[slugModel](db).Create(ctx, &bs), nil)
		got, err := orm.Query[slugModel](db).Get(ctx, "pk", bs.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got.S, longSlug)

		unicodeSlug := strings.Repeat("你好你好", 50)
		us := unicodeSlugModel{S: unicodeSlug}
		djangotest.AssertEqual(t, orm.Query[unicodeSlugModel](db).Create(ctx, &us), nil)
		gotU, err := orm.Query[unicodeSlugModel](db).Get(ctx, "pk", us.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, gotU.S, unicodeSlug)
	})
}

// Ported from Django: tests/model_fields/test_charfield.py::TestCharField.test_emoji
// Ported from Django: tests/model_fields/test_textfield.py::TextFieldTests.test_emoji
func TestCharAndTextFieldEmoji(t *testing.T) {
	djangotest.EachDB(t, []any{&charTextPost{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p := charTextPost{Title: "Smile 😀", Body: "Smile 😀."}
		djangotest.AssertEqual(t, orm.Query[charTextPost](db).Create(ctx, &p), nil)
		got, err := orm.Query[charTextPost](db).Get(ctx, "pk", p.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got.Title, "Smile 😀")
		djangotest.AssertEqual(t, got.Body, "Smile 😀.")
	})
}

// Ported from Django: tests/model_fields/test_booleanfield.py::BooleanFieldTests.test_return_type
// Ported from Django: tests/model_fields/test_booleanfield.py::BooleanFieldTests.test_select_related
func TestBooleanFieldReturnTypeAndSelectRelated(t *testing.T) {
	djangotest.EachDB(t, []any{&boolModel{}, &nullBoolModel{}, &fkToBools{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		bmt := boolModel{BField: true}
		bmf := boolModel{BField: false}
		tr, fa := true, false
		nbmt := nullBoolModel{NBField: &tr}
		nbmf := nullBoolModel{NBField: &fa}
		nbmn := nullBoolModel{NBField: nil}
		djangotest.AssertEqual(t, orm.Query[boolModel](db).BulkCreate(ctx, []*boolModel{&bmt, &bmf}), nil)
		djangotest.AssertEqual(t, orm.Query[nullBoolModel](db).BulkCreate(ctx, []*nullBoolModel{&nbmt, &nbmf, &nbmn}), nil)

		gotNull, err := orm.Query[nullBoolModel](db).Get(ctx, "pk", nbmn.ID)
		djangotest.AssertEqual(t, err, nil)
		if gotNull.NBField != nil {
			t.Fatalf("got %v, want nil", *gotNull.NBField)
		}

		m1 := fkToBools{}
		m1.BF.SetPK(bmt.ID)
		m1.NBF.SetPK(nbmt.ID)
		djangotest.AssertEqual(t, orm.Query[fkToBools](db).Create(ctx, &m1), nil)

		loaded, err := orm.Query[fkToBools](db).SelectRelated("bf", "nbf").Get(ctx, "pk", m1.ID)
		djangotest.AssertEqual(t, err, nil)
		bfObj, ok := loaded.BF.Object()
		djangotest.AssertEqual(t, ok, true)
		djangotest.AssertEqual(t, bfObj.BField, true)
		nbfObj, ok := loaded.NBF.Object()
		djangotest.AssertEqual(t, ok, true)
		if nbfObj.NBField == nil || !*nbfObj.NBField {
			t.Fatalf("got %v, want true", nbfObj.NBField)
		}
	})
}

// Ported from Django: tests/model_fields/test_datetimefield.py::DateTimeFieldTests.test_datetimes_save_completely
// Ported from Django: tests/model_fields/test_datetimefield.py::DateTimeFieldTests.test_timefield_to_python_microseconds
func TestDateTimeDateTimeFieldsSaveCompletely(t *testing.T) {
	t1, err := orm.ParseTime("01:02:03.000004")
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, t1, orm.NewTime(1, 2, 3, 4000))
	t2, err := orm.ParseTime("01:02:03.999999")
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, t2, orm.NewTime(1, 2, 3, 999999000))

	djangotest.EachDB(t, []any{&dateTimeAllModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		dat := orm.NewDate(2014, time.March, 12)
		dt := time.Date(2014, time.March, 12, 21, 22, 23, 240000000, time.UTC)
		tim := orm.NewTime(21, 22, 23, 240000000)

		m := dateTimeAllModel{D: dat, DT: dt, T: tim}
		djangotest.AssertEqual(t, orm.Query[dateTimeAllModel](db).Create(ctx, &m), nil)
		loaded, err := orm.Query[dateTimeAllModel](db).First(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, loaded.D, dat)
		if !loaded.DT.Equal(dt) {
			t.Fatalf("DT = %v, want %v", loaded.DT, dt)
		}
		djangotest.AssertEqual(t, loaded.T, tim)
	})
}
