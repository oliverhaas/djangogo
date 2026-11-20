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

type allKindsModel struct {
	ID       int64
	Small    int16
	Big      int64 `orm:"type=bigint"`
	PosSmall uint16
	PosInt   uint32
	PosBig   uint64
	Ratio    float64
	Price    orm.Decimal `orm:"max_digits=10;decimal_places=2"`
	Born     orm.Date
	Alarm    orm.Time
	Elapsed  time.Duration
	UID      orm.UUID
	Meta     json.RawMessage
	Blob     []byte
	Slug     string `orm:"type=slug"`
	Email    string `orm:"type=email"`
	Website  string `orm:"type=url"`
	IP       netip.Addr
	IPText   string `orm:"type=ip;protocol=ipv4;null"`
}

func TestKindInferenceAndDefaults(t *testing.T) {
	reg := djangotest.NewRegistry(t, &allKindsModel{})
	m, ok := reg.ModelOf(&allKindsModel{})
	if !ok {
		t.Fatal("allKindsModel not registered")
	}

	wantKinds := map[string]orm.Kind{
		"ID":       orm.KindAuto,
		"Small":    orm.KindSmallInt,
		"Big":      orm.KindBigInt,
		"PosSmall": orm.KindPositiveSmallInt,
		"PosInt":   orm.KindPositiveInt,
		"PosBig":   orm.KindPositiveBigInt,
		"Ratio":    orm.KindFloat,
		"Price":    orm.KindDecimal,
		"Born":     orm.KindDate,
		"Alarm":    orm.KindTime,
		"Elapsed":  orm.KindDuration,
		"UID":      orm.KindUUID,
		"Meta":     orm.KindJSON,
		"Blob":     orm.KindBinary,
		"Slug":     orm.KindSlug,
		"Email":    orm.KindEmail,
		"Website":  orm.KindURL,
		"IP":       orm.KindGenericIPAddress,
		"IPText":   orm.KindGenericIPAddress,
	}
	for name, want := range wantKinds {
		f, found := m.FieldByName(name)
		if !found {
			t.Fatalf("missing field %s", name)
		}
		if f.Kind != want {
			t.Errorf("field %s: Kind = %v, want %v", name, f.Kind, want)
		}
	}

	slugF, _ := m.FieldByName("Slug")
	djangotest.AssertEqual(t, slugF.MaxLength, 50)
	djangotest.AssertEqual(t, slugF.DBIndex, true)

	emailF, _ := m.FieldByName("Email")
	djangotest.AssertEqual(t, emailF.MaxLength, 254)

	urlF, _ := m.FieldByName("Website")
	djangotest.AssertEqual(t, urlF.MaxLength, 200)

	priceF, _ := m.FieldByName("Price")
	djangotest.AssertEqual(t, priceF.MaxDigits, 10)
	djangotest.AssertEqual(t, priceF.DecimalPlaces, 2)

	blobF, _ := m.FieldByName("Blob")
	djangotest.AssertEqual(t, blobF.Editable, false)

	ipTextF, _ := m.FieldByName("IPText")
	djangotest.AssertEqual(t, ipTextF.Protocol, "ipv4")
}

func TestKindStringAllKinds(t *testing.T) {
	cases := []struct {
		k    orm.Kind
		want string
	}{
		{orm.KindSmallInt, "SmallInt"},
		{orm.KindBigInt, "BigInt"},
		{orm.KindPositiveSmallInt, "PositiveSmallInt"},
		{orm.KindPositiveInt, "PositiveInt"},
		{orm.KindPositiveBigInt, "PositiveBigInt"},
		{orm.KindFloat, "Float"},
		{orm.KindDecimal, "Decimal"},
		{orm.KindDate, "Date"},
		{orm.KindTime, "Time"},
		{orm.KindDuration, "Duration"},
		{orm.KindUUID, "UUID"},
		{orm.KindJSON, "JSON"},
		{orm.KindBinary, "Binary"},
		{orm.KindSlug, "Slug"},
		{orm.KindEmail, "Email"},
		{orm.KindURL, "URL"},
		{orm.KindGenericIPAddress, "GenericIPAddress"},
	}
	for _, tc := range cases {
		djangotest.AssertEqual(t, tc.k.String(), tc.want)
	}
}

func TestFieldIntRange(t *testing.T) {
	reg := djangotest.NewRegistry(t, &allKindsModel{})
	m, _ := reg.ModelOf(&allKindsModel{})

	cases := []struct {
		field     string
		wantMin   int64
		wantMax   int64
		wantHasOk bool
	}{
		{"Small", -32768, 32767, true},
		{"Big", -9223372036854775808, 9223372036854775807, true},
		{"PosSmall", 0, 32767, true},
		{"PosInt", 0, 2147483647, true},
		{"PosBig", 0, 9223372036854775807, true},
		{"Ratio", 0, 0, false},
	}
	for _, tc := range cases {
		f, _ := m.FieldByName(tc.field)
		lo, hi, ok := f.IntRange()
		djangotest.AssertEqual(t, ok, tc.wantHasOk)
		djangotest.AssertEqual(t, lo, tc.wantMin)
		djangotest.AssertEqual(t, hi, tc.wantMax)
	}
}

func TestKindTagValidationErrors(t *testing.T) {
	type badDecimal struct {
		ID int64
		D  orm.Decimal `orm:"max_digits=2;decimal_places=3"`
	}
	type badProtocol struct {
		ID int64
		IP string `orm:"type=ip;protocol=ipv5"`
	}
	type badAllowUnicode struct {
		ID   int64
		Name string `orm:"allow_unicode"`
	}
	type badPosDefault struct {
		ID int64
		N  int32 `orm:"type=positive_int;default=-1"`
	}
	type badJSONDefault struct {
		ID int64
		V  json.RawMessage `orm:"default={bad"`
	}
	type badTypeMismatch struct {
		ID int64
		N  int64 `orm:"type=slug"`
	}

	for _, ptr := range []any{
		&badDecimal{},
		&badProtocol{},
		&badAllowUnicode{},
		&badPosDefault{},
		&badJSONDefault{},
		&badTypeMismatch{},
	} {
		if _, err := orm.NewRegistry().Register(ptr); err == nil {
			t.Errorf("expected registration error for %T", ptr)
		}
	}
}

type defaultsAndAutoModel struct {
	ID       int64
	Ratio    float64         `orm:"default=1.5"`
	Price    orm.Decimal     `orm:"max_digits=6;decimal_places=2;default=9.95"`
	Born     orm.Date        `orm:"default=2024-01-15"`
	Alarm    orm.Time        `orm:"default=08:30:00"`
	Elapsed  time.Duration   `orm:"default=5m"`
	UID      orm.UUID        `orm:"default=550e8400-e29b-41d4-a716-446655440000"`
	Meta     json.RawMessage `orm:"default={}"`
	Raw      []byte          `orm:"default=0x0a0b"`
	Slug     string          `orm:"type=slug;index=false;default=hello-world"`
	Email    string          `orm:"type=email;default=a@example.com"`
	Website  string          `orm:"type=url;default=https://example.com"`
	CreatedD orm.Date        `orm:"auto_now_add"`
	UpdatedT orm.Time        `orm:"auto_now"`
}

func TestDefaultsAndAutoNowOnNewKinds(t *testing.T) {
	djangotest.EachDB(t, []any{&defaultsAndAutoModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fixed := time.Date(2025, time.June, 10, 14, 25, 36, 123000000, time.UTC)
		db.Now = func() time.Time { return fixed }

		m := defaultsAndAutoModel{}
		djangotest.AssertEqual(t, orm.Query[defaultsAndAutoModel](db).Create(ctx, &m), nil)

		got, err := orm.Query[defaultsAndAutoModel](db).Get(ctx, "pk", m.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got.Ratio, 1.5)
		if !got.Price.ExactEqual(orm.MustDecimal("9.95")) {
			t.Errorf("Price = %s, want 9.95", got.Price.String())
		}
		djangotest.AssertEqual(t, got.Born, orm.NewDate(2024, time.January, 15))
		djangotest.AssertEqual(t, got.Alarm, orm.NewTime(8, 30, 0, 0))
		djangotest.AssertEqual(t, got.Elapsed, 5*time.Minute)
		djangotest.AssertEqual(t, got.UID, orm.MustUUID("550e8400-e29b-41d4-a716-446655440000"))
		djangotest.AssertJSONEqual(t, string(got.Meta), "{}")
		if !bytes.Equal(got.Raw, []byte{0x0a, 0x0b}) {
			t.Errorf("Raw = %x, want 0a0b", got.Raw)
		}
		djangotest.AssertEqual(t, got.Slug, "hello-world")
		djangotest.AssertEqual(t, got.Email, "a@example.com")
		djangotest.AssertEqual(t, got.Website, "https://example.com")
		djangotest.AssertEqual(t, got.CreatedD, orm.NewDate(2025, time.June, 10))
		djangotest.AssertEqual(t, got.UpdatedT, orm.NewTime(14, 25, 36, 123000000))

		// Verify Values() returns typed Go values on both dialects.
		vals, err := orm.Query[defaultsAndAutoModel](db).Values(
			"ratio", "price", "born", "alarm", "elapsed", "uid", "meta", "raw",
		).All(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(vals), 1)
		djangotest.AssertEqual(t, vals[0]["ratio"], any(1.5))
		djangotest.AssertEqual(t, vals[0]["born"], any(orm.NewDate(2024, time.January, 15)))
		djangotest.AssertEqual(t, vals[0]["alarm"], any(orm.NewTime(8, 30, 0, 0)))
		djangotest.AssertEqual(t, vals[0]["elapsed"], any(5*time.Minute))
		djangotest.AssertEqual(t, vals[0]["uid"], any(orm.MustUUID("550e8400-e29b-41d4-a716-446655440000")))
		if _, ok := vals[0]["price"].(orm.Decimal); !ok {
			t.Errorf("Values price type = %T, want orm.Decimal", vals[0]["price"])
		}
		if _, ok := vals[0]["meta"].(json.RawMessage); !ok {
			t.Errorf("Values meta type = %T, want json.RawMessage", vals[0]["meta"])
		}
		if b, ok := vals[0]["raw"].([]byte); !ok || !bytes.Equal(b, []byte{0x0a, 0x0b}) {
			t.Errorf("Values raw = %#v, want []byte{0x0a, 0x0b}", vals[0]["raw"])
		}
	})
}

func TestParsePGIntervalAndTemporalHelpers(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"00:00:00", 0},
		{"00:00:01", time.Second},
		{"-00:00:01", -time.Second},
		{"00:00:02.05", 2*time.Second + 50*time.Millisecond},
		{"1 day", 24 * time.Hour},
		{"-2 days", -48 * time.Hour},
		{"1 day 01:02:03.000004", 25*time.Hour + 2*time.Minute + 3*time.Second + 4*time.Microsecond},
		{"2499999:59:59.999999", 8999999999999999 * time.Microsecond},
	}
	for _, tc := range cases {
		got, err := orm.ParsePGInterval(tc.in)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got, tc.want)
	}
	if _, err := orm.ParsePGInterval(""); err == nil {
		t.Error("expected error for empty interval")
	}

	// Text marshal/unmarshal roundtrips.
	u := orm.NewUUID()
	if u[6]>>4 != 4 || u[8]>>6 != 2 {
		t.Errorf("UUID %s does not have v4/RFC4122 bits", u.String())
	}
	if len(u.Hex()) != 32 || strings.Contains(u.Hex(), "-") {
		t.Errorf("UUID.Hex() = %q, want 32 hex chars", u.Hex())
	}
	var u2 orm.UUID
	b, err := u.MarshalText()
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, u2.UnmarshalText(b), nil)
	djangotest.AssertEqual(t, u2, u)

	dec := orm.MustDecimal("123.450")
	var dec2 orm.Decimal
	db, err := dec.MarshalText()
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, dec2.UnmarshalText(db), nil)
	djangotest.AssertEqual(t, dec2.ExactEqual(dec), true)
	digits, places := dec.DigitCounts()
	djangotest.AssertEqual(t, digits, 6)
	djangotest.AssertEqual(t, places, 3)

	dt := orm.NewDate(2026, time.November, 20)
	var dt2 orm.Date
	dtb, err := dt.MarshalText()
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, dt2.UnmarshalText(dtb), nil)
	djangotest.AssertEqual(t, dt2, dt)

	tm := orm.NewTime(9, 8, 7, 6000)
	var tm2 orm.Time
	tmb, err := tm.MarshalText()
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, tm2.UnmarshalText(tmb), nil)
	djangotest.AssertEqual(t, tm2, tm)
}
