package forms_test

import (
	"context"
	"encoding/json"
	"math"
	"net/netip"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// BigIntModel mirrors Django's BigInt model in tests/model_forms/models.py.
type BigIntModel struct {
	ID     int64
	Biggie int64 `orm:"type=bigint"`
}

// HomepageModel mirrors Django's Homepage model in tests/model_forms/models.py.
type HomepageModel struct {
	ID  int64
	URL string `orm:"type=url"`
}

// AllKindsModel exercises every model field kind in ModelForm.
type AllKindsModel struct {
	ID        int64
	Small     int16
	Regular   int
	Big       int64  `orm:"type=bigint"`
	PosSmall  uint16 `orm:"type=positive_smallint"`
	PosInt    uint32 `orm:"type=positive_int"`
	PosBig    uint64 `orm:"type=positive_bigint"`
	Score     float64
	Amount    orm.Decimal `orm:"max_digits=8;decimal_places=2"`
	Label     string      `orm:"max_length=40"`
	Notes     string      `orm:"type=text;blank"`
	Slug      string      `orm:"type=slug"`
	UniSlug   string      `orm:"type=slug;allow_unicode;blank"`
	Email     string      `orm:"type=email"`
	Website   string      `orm:"type=url"`
	IPStr     string      `orm:"type=ip;protocol=both;unpack_ipv4"`
	IPAddr    netip.Addr  `orm:"protocol=ipv4"`
	Active    bool
	MaybeFlag *bool `orm:"blank;null"`
	BirthDate orm.Date
	Alarm     orm.Time
	CreatedAt time.Time
	Elapsed   time.Duration
	Token     orm.UUID
	Payload   json.RawMessage
	RawBytes  []byte
	EditBytes []byte `orm:"editable=true;blank"`
}

// NullableKindsModel tests pointer fields storing NULL via ModelForm.
type NullableKindsModel struct {
	ID     int64
	Dec    *orm.Decimal     `orm:"max_digits=6;decimal_places=2;blank;null"`
	Day    *orm.Date        `orm:"blank;null"`
	Clock  *orm.Time        `orm:"blank;null"`
	Dur    *time.Duration   `orm:"blank;null"`
	UID    *orm.UUID        `orm:"blank;null"`
	IP     *netip.Addr      `orm:"blank;null"`
	IPText *string          `orm:"type=ip;blank;null"`
	Data   *json.RawMessage `orm:"blank;null"`
	Count  *int             `orm:"blank;null"`
	Flag   *bool            `orm:"blank;null"`
}

// TypedChoicesModel tests choices on multiple kinds including optgroups and typed values.
type TypedChoicesModel struct {
	ID       int64
	Priority int         `orm:"default=2"`
	Rate     orm.Decimal `orm:"max_digits=5;decimal_places=2;blank;null"`
	Category string      `orm:"max_length=20;blank"`
}

// Meta declares typed choices and optgroups on TypedChoicesModel.
func (TypedChoicesModel) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"Priority": {
				orm.ChoiceOf(1, "Low"),
				orm.ChoiceOf(2, "Normal"),
				orm.ChoiceOf(3, "High"),
			},
			"Rate": {
				orm.ChoiceOf(orm.MustDecimal("0.05"), "5%"),
				orm.ChoiceOf(orm.MustDecimal("0.10"), "10%"),
			},
			"Category": {
				orm.ChoiceGroup("Audio", orm.ChoiceOf("vinyl", "Vinyl"), orm.ChoiceOf("cd", "CD")),
				orm.ChoiceGroup("Video", orm.ChoiceOf("vhs", "VHS")),
			},
		},
	}
}

// JSONStructPayload is a Go struct stored in a KindJSON column.
type JSONStructPayload struct {
	Title string `json:"title"`
	Count int    `json:"count"`
}

// StructJSONModel tests a non-RawMessage Go struct tagged orm:"type=json".
type StructJSONModel struct {
	ID      int64
	Details JSONStructPayload `orm:"type=json"`
}

// Ported from Django: tests/model_forms/tests.py::ModelOtherFieldTests.test_big_integer_field
func TestModelOtherFieldBigIntegerField(t *testing.T) {
	djangotest.EachDB(t, []any{&BigIntModel{}}, func(t *testing.T, db *orm.DB) {
		newForm := func() *forms.ModelForm[BigIntModel] {
			return forms.NewModelForm[BigIntModel](db, forms.WithFields(forms.AllFields))
		}

		bif := newForm().Bind(vals("biggie", "-9223372036854775808"))
		djangotest.AssertEqual(t, bif.IsValid(), true)
		djangotest.AssertEqual(t, bif.Instance().Biggie, int64(math.MinInt64))

		bif = newForm().Bind(vals("biggie", "-9223372036854775809"))
		djangotest.AssertEqual(t, bif.IsValid(), false)
		djangotest.AssertEqual(t, bif.Errors().Messages(), map[string][]string{
			"biggie": {"Ensure this value is greater than or equal to -9223372036854775808."},
		})

		bif = newForm().Bind(vals("biggie", "9223372036854775807"))
		djangotest.AssertEqual(t, bif.IsValid(), true)
		djangotest.AssertEqual(t, bif.Instance().Biggie, int64(math.MaxInt64))

		bif = newForm().Bind(vals("biggie", "9223372036854775808"))
		djangotest.AssertEqual(t, bif.IsValid(), false)
		djangotest.AssertEqual(t, bif.Errors().Messages(), map[string][]string{
			"biggie": {"Ensure this value is less than or equal to 9223372036854775807."},
		})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelOtherFieldTests.test_url_on_modelform
func TestModelOtherFieldURLOnModelForm(t *testing.T) {
	djangotest.EachDB(t, []any{&HomepageModel{}}, func(t *testing.T, db *orm.DB) {
		newForm := func(u string) *forms.ModelForm[HomepageModel] {
			return forms.NewModelForm[HomepageModel](db, forms.WithFields(forms.AllFields)).Bind(vals("url", u))
		}
		for _, bad := range []string{"foo", "http://", "http://example", "http://example.", "http://com."} {
			if newForm(bad).IsValid() {
				t.Errorf("HomepageModel form accepted invalid URL %q", bad)
			}
		}
		for _, good := range []string{
			"http://localhost",
			"http://example.com",
			"http://www.example.com",
			"http://www.example.com:8000",
			"http://www.example.com/test",
			"http://www.example.com:8000/test",
			"http://example.com/foo/bar",
		} {
			if f := newForm(good); !f.IsValid() {
				t.Errorf("HomepageModel form rejected valid URL %q: %v", good, f.Errors().Messages())
			}
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelOtherFieldTests.test_https_prefixing
func TestModelOtherFieldHTTPSPrefixing(t *testing.T) {
	djangotest.EachDB(t, []any{&HomepageModel{}}, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[HomepageModel](db, forms.WithFields(forms.AllFields)).Bind(vals("url", "example.com"))
		djangotest.AssertEqual(t, f.IsValid(), true)
		djangotest.AssertEqual(t, f.CleanedData()["url"], any("https://example.com"))
		djangotest.AssertEqual(t, f.Instance().URL, "https://example.com")

		f = forms.NewModelForm[HomepageModel](db, forms.WithFields(forms.AllFields)).Bind(vals("url", "example.com/test"))
		djangotest.AssertEqual(t, f.IsValid(), true)
		djangotest.AssertEqual(t, f.CleanedData()["url"], any("https://example.com/test"))
	})
}

// Ported from Django: tests/model_forms/tests.py::CustomCleanTests.test_override_clean
func TestModelFormOverrideCleanStillValidatesUnique(t *testing.T) {
	djangotest.EachDB(t, []any{&Triple{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		if err := orm.Query[Triple](db).Create(ctx, &Triple{Left: 1, Middle: 2, Right: 1}); err != nil {
			t.Fatal(err)
		}
		f := forms.NewModelForm[Triple](db, forms.WithFields(forms.AllFields))
		f.CleanWith(func(form *forms.Form) error {
			if form.CleanedData()["left"] != form.CleanedData()["right"] {
				return forms.NewValidationError("Left and right should be equal", "")
			}
			return nil
		})
		bound := f.Bind(vals("left", "1", "middle", "2", "right", "1"))
		djangotest.AssertEqual(t, bound.IsValid(), false)
		djangotest.AssertEqual(t, len(bound.NonFieldErrors().Messages()) > 0, true)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBasicTests.test_recleaning_model_form_instance
func TestModelFormRecleaningInstance(t *testing.T) {
	djangotest.EachDB(t, []any{&Person{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		form := forms.NewModelForm[Person](db, forms.WithFields(forms.AllFields)).Bind(vals("name", "Bob"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		obj, err := form.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := orm.FullClean(ctx, db, obj); err != nil {
			t.Errorf("FullClean after Save failed: %v", err)
		}
	})
}

// ZooKeeperModel tests Meta.ChoicesFunc in ModelForm.
type ZooKeeperModel struct {
	ID     int64
	Animal string `orm:"max_length=10;blank"`
}

var zooCallCount int

// Meta declares a callable ChoicesFunc on Animal.
func (ZooKeeperModel) Meta() orm.Meta {
	return orm.Meta{
		ChoicesFunc: map[string]func() []orm.Choice{
			"Animal": func() []orm.Choice {
				zooCallCount++
				return []orm.Choice{
					orm.ChoiceOf("LION", "Lion"),
					orm.ChoiceOf("ZEBRA", "Zebra"),
				}
			},
		},
	}
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBasicTests.test_callable_choices_are_lazy
func TestModelFormCallableChoices(t *testing.T) {
	djangotest.EachDB(t, []any{&ZooKeeperModel{}}, func(t *testing.T, db *orm.DB) {
		zooCallCount = 0
		form := forms.NewModelForm[ZooKeeperModel](db, forms.WithFields("animal"))
		if zooCallCount == 0 {
			t.Fatal("expected ChoicesFunc to be evaluated when building form choices")
		}
		field, ok := form.FieldDef("animal")
		if !ok {
			t.Fatal("missing animal field")
		}
		tcf, ok := field.(*forms.TypedChoiceField)
		if !ok {
			t.Fatalf("animal field is %T, want *forms.TypedChoiceField", field)
		}
		djangotest.AssertEqual(t, tcf.Choices, []forms.Choice{
			{Value: "", Label: forms.EmptyChoiceLabel},
			{Value: "LION", Label: "Lion"},
			{Value: "ZEBRA", Label: "Zebra"},
		})
	})
}

func TestModelFormFieldMappingAllKinds(t *testing.T) {
	djangotest.EachDB(t, []any{&AllKindsModel{}}, func(t *testing.T, db *orm.DB) {
		form := forms.NewModelForm[AllKindsModel](db, forms.WithFields(forms.AllFields))
		if form.Err() != nil {
			t.Fatalf("form.Err() = %v", form.Err())
		}
		// BinaryField defaults to editable=false, so raw_bytes is excluded while edit_bytes is included.
		if _, ok := form.FieldDef("raw_bytes"); ok {
			t.Error("non-editable BinaryField raw_bytes should be excluded from ModelForm")
		}
		wantTypes := map[string]string{
			"small":      "*forms.IntegerField",
			"regular":    "*forms.IntegerField",
			"big":        "*forms.IntegerField",
			"pos_small":  "*forms.IntegerField",
			"pos_int":    "*forms.IntegerField",
			"pos_big":    "*forms.IntegerField",
			"score":      "*forms.FloatField",
			"amount":     "*forms.DecimalField",
			"label":      "*forms.CharField",
			"notes":      "*forms.CharField",
			"slug":       "*forms.SlugField",
			"uni_slug":   "*forms.SlugField",
			"email":      "*forms.EmailField",
			"website":    "*forms.URLField",
			"ip_str":     "*forms.GenericIPAddressField",
			"ip_addr":    "*forms.GenericIPAddressField",
			"active":     "*forms.BooleanField",
			"maybe_flag": "*forms.NullBooleanField",
			"birth_date": "*forms.DateField",
			"alarm":      "*forms.TimeField",
			"created_at": "*forms.DateTimeField",
			"elapsed":    "*forms.DurationField",
			"token":      "*forms.UUIDField",
			"payload":    "*forms.JSONField",
			"edit_bytes": "*forms.CharField",
		}
		for name, wantType := range wantTypes {
			f, ok := form.FieldDef(name)
			if !ok {
				t.Errorf("missing field %q", name)
				continue
			}
			gotType := typeNameOf(f)
			if gotType != wantType {
				t.Errorf("field %q is %s, want %s", name, gotType, wantType)
			}
		}

		// Verify DecimalField parameters carried over.
		df := mustFieldDef[*forms.DecimalField](t, form.Form, "amount")
		djangotest.AssertEqual(t, df.MaxDigits, 8)
		djangotest.AssertEqual(t, df.DecimalPlaces, 2)

		// Verify GenericIPAddressField protocol and unpack_ipv4 carried over.
		ipStr := mustFieldDef[*forms.GenericIPAddressField](t, form.Form, "ip_str")
		djangotest.AssertEqual(t, ipStr.Protocol, "both")
		djangotest.AssertEqual(t, ipStr.UnpackIPv4, true)
		ipAddr := mustFieldDef[*forms.GenericIPAddressField](t, form.Form, "ip_addr")
		djangotest.AssertEqual(t, ipAddr.Protocol, "ipv4")

		// Verify integer range validation catches out-of-range values as form errors.
		small := mustFieldDef[*forms.IntegerField](t, form.Form, "small")
		rejects(t, small, "32768", "Ensure this value is less than or equal to 32767.")
		rejects(t, small, "-32769", "Ensure this value is greater than or equal to -32768.")
		posSmall := mustFieldDef[*forms.IntegerField](t, form.Form, "pos_small")
		rejects(t, posSmall, "-1", "Ensure this value is greater than or equal to 0.")
		rejects(t, posSmall, "32768", "Ensure this value is less than or equal to 32767.")
		posInt := mustFieldDef[*forms.IntegerField](t, form.Form, "pos_int")
		rejects(t, posInt, "-1", "Ensure this value is greater than or equal to 0.")
	})
}

func TestModelFormAllKindsRoundTrip(t *testing.T) {
	djangotest.EachDB(t, []any{&AllKindsModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		uid := "550e8400-e29b-41d4-a716-446655440000"
		form := forms.NewModelForm[AllKindsModel](db, forms.WithFields(forms.AllFields)).Bind(vals(
			"small", "-12",
			"regular", "42000",
			"big", "9000000000000",
			"pos_small", "100",
			"pos_int", "50000",
			"pos_big", "1000000000000",
			"score", "3.25",
			"amount", "12345.67",
			"label", "Widget",
			"notes", "Line 1\nLine 2",
			"slug", "my-widget",
			"uni_slug", "héllo-wörld",
			"email", "user@example.com",
			"website", "example.com/widget",
			"ip_str", "::ffff:192.0.2.1",
			"ip_addr", "10.0.0.5",
			"active", "on",
			"maybe_flag", "true",
			"birth_date", "2024-05-15",
			"alarm", "07:30:45",
			"created_at", "2024-05-15 08:00:00",
			"elapsed", "1 02:03:04",
			"token", uid,
			"payload", `{"k": "v", "n": 2}`,
			"edit_bytes", "hello",
		))
		if !form.IsValid() {
			t.Fatalf("form invalid: %v (err=%v)", form.Errors().Messages(), form.Err())
		}
		saved, err := form.Save(ctx)
		if err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, err := orm.Query[AllKindsModel](db).Get(ctx, "id", saved.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.Small, int16(-12))
		djangotest.AssertEqual(t, got.Regular, 42000)
		djangotest.AssertEqual(t, got.Big, int64(9000000000000))
		djangotest.AssertEqual(t, got.PosSmall, uint16(100))
		djangotest.AssertEqual(t, got.PosInt, uint32(50000))
		djangotest.AssertEqual(t, got.PosBig, uint64(1000000000000))
		djangotest.AssertEqual(t, got.Score, 3.25)
		if !got.Amount.ExactEqual(orm.MustDecimal("12345.67")) {
			t.Errorf("Amount = %q, want 12345.67", got.Amount.String())
		}
		djangotest.AssertEqual(t, got.Slug, "my-widget")
		djangotest.AssertEqual(t, got.UniSlug, "héllo-wörld")
		djangotest.AssertEqual(t, got.Email, "user@example.com")
		djangotest.AssertEqual(t, got.Website, "https://example.com/widget")
		djangotest.AssertEqual(t, got.IPStr, "192.0.2.1")
		djangotest.AssertEqual(t, got.IPAddr, netip.MustParseAddr("10.0.0.5"))
		djangotest.AssertEqual(t, got.Active, true)
		if got.MaybeFlag == nil || !*got.MaybeFlag {
			t.Errorf("MaybeFlag = %v, want *true", got.MaybeFlag)
		}
		djangotest.AssertEqual(t, got.BirthDate, orm.NewDate(2024, 5, 15))
		djangotest.AssertEqual(t, got.Alarm, orm.NewTime(7, 30, 45, 0))
		djangotest.AssertEqual(t, got.Elapsed, 26*time.Hour+3*time.Minute+4*time.Second)
		djangotest.AssertEqual(t, got.Token, orm.MustUUID(uid))
		djangotest.AssertJSONEqual(t, string(got.Payload), `{"k": "v", "n": 2}`)
		djangotest.AssertEqual(t, string(got.EditBytes), "hello")

		// Re-open with WithInstance and verify initial values render cleanly.
		editForm := forms.NewModelForm[AllKindsModel](db, forms.WithFields(forms.AllFields)).WithInstance(&got)
		djangotest.AssertEqual(t, editForm.Field("birth_date").Value(), any(date(2024, 5, 15)))
		djangotest.AssertEqual(t, editForm.Field("token").Value(), any(uid))
		djangotest.AssertEqual(t, editForm.Field("elapsed").Value(), any("1 02:03:04"))
		djangotest.AssertEqual(t, editForm.Field("ip_addr").Value(), any("10.0.0.5"))
	})
}

func TestModelFormNullablePointerKinds(t *testing.T) {
	djangotest.EachDB(t, []any{&NullableKindsModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		form := forms.NewModelForm[NullableKindsModel](db, forms.WithFields(forms.AllFields)).Bind(vals(
			"dec", "",
			"day", "",
			"clock", "",
			"dur", "",
			"uid", "",
			"ip", "",
			"ip_text", "",
			"data", "",
			"count", "",
			"flag", "unknown",
		))
		if !form.IsValid() {
			t.Fatalf("nullable form invalid: %v (err=%v)", form.Errors().Messages(), form.Err())
		}
		saved, err := form.Save(ctx)
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := orm.Query[NullableKindsModel](db).Get(ctx, "id", saved.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Dec != nil || got.Day != nil || got.Clock != nil || got.Dur != nil ||
			got.UID != nil || got.IP != nil || got.IPText != nil || got.Data != nil ||
			got.Count != nil || got.Flag != nil {
			t.Errorf("expected all pointer fields to be nil, got %+v", got)
		}

		// Now update the same row with non-null values.
		uid := "11111111-2222-3333-4444-555555555555"
		edit := forms.NewModelForm[NullableKindsModel](db, forms.WithFields(forms.AllFields)).
			WithInstance(&got).Bind(vals(
			"dec", "99.50",
			"day", "2025-01-02",
			"clock", "14:20:00",
			"dur", "00:05:00",
			"uid", uid,
			"ip", "2001:db8::1",
			"ip_text", "127.0.0.1",
			"data", `[1, 2, 3]`,
			"count", "7",
			"flag", "false",
		))
		if !edit.IsValid() {
			t.Fatalf("edit form invalid: %v (err=%v)", edit.Errors().Messages(), edit.Err())
		}
		if _, err := edit.Save(ctx); err != nil {
			t.Fatalf("Save update: %v", err)
		}
		fresh, err := orm.Query[NullableKindsModel](db).Get(ctx, "id", saved.ID)
		if err != nil {
			t.Fatalf("Get updated: %v", err)
		}
		if fresh.Dec == nil || !fresh.Dec.ExactEqual(orm.MustDecimal("99.50")) {
			t.Errorf("Dec = %v, want 99.50", fresh.Dec)
		}
		if fresh.Day == nil || *fresh.Day != orm.NewDate(2025, 1, 2) {
			t.Errorf("Day = %v, want 2025-01-02", fresh.Day)
		}
		if fresh.Clock == nil || *fresh.Clock != orm.NewTime(14, 20, 0, 0) {
			t.Errorf("Clock = %v, want 14:20:00", fresh.Clock)
		}
		if fresh.Dur == nil || *fresh.Dur != 5*time.Minute {
			t.Errorf("Dur = %v, want 5m", fresh.Dur)
		}
		if fresh.UID == nil || *fresh.UID != orm.MustUUID(uid) {
			t.Errorf("UID = %v, want %s", fresh.UID, uid)
		}
		if fresh.IP == nil || *fresh.IP != netip.MustParseAddr("2001:db8::1") {
			t.Errorf("IP = %v, want 2001:db8::1", fresh.IP)
		}
		if fresh.IPText == nil || *fresh.IPText != "127.0.0.1" {
			t.Errorf("IPText = %v, want 127.0.0.1", fresh.IPText)
		}
		if fresh.Data == nil {
			t.Fatal("Data is nil")
		}
		djangotest.AssertJSONEqual(t, string(*fresh.Data), `[1, 2, 3]`)
		if fresh.Count == nil || *fresh.Count != 7 {
			t.Errorf("Count = %v, want 7", fresh.Count)
		}
		if fresh.Flag == nil || *fresh.Flag != false {
			t.Errorf("Flag = %v, want *false", fresh.Flag)
		}
	})
}

func TestModelFormTypedChoicesAndStructJSON(t *testing.T) {
	djangotest.EachDB(t, []any{&TypedChoicesModel{}, &StructJSONModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()

		// Priority has a default and is required -> no empty choice.
		f := forms.NewModelForm[TypedChoicesModel](db, forms.WithFields(forms.AllFields))
		prio := mustFieldDef[*forms.TypedChoiceField](t, f.Form, "priority")
		djangotest.AssertEqual(t, len(prio.Choices), 3)
		// Rate is blank=true -> includes empty choice.
		rate := mustFieldDef[*forms.TypedChoiceField](t, f.Form, "rate")
		djangotest.AssertEqual(t, rate.Choices[0], forms.Choice{Value: "", Label: forms.EmptyChoiceLabel})

		bound := f.Bind(vals("priority", "3", "rate", "0.10", "category", "vinyl"))
		if !bound.IsValid() {
			t.Fatalf("TypedChoicesModel form invalid: %v", bound.Errors().Messages())
		}
		saved, err := bound.Save(ctx)
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		djangotest.AssertEqual(t, saved.Priority, 3)
		if !saved.Rate.ExactEqual(orm.MustDecimal("0.10")) {
			t.Errorf("Rate = %q, want 0.10", saved.Rate.String())
		}
		djangotest.AssertEqual(t, saved.Category, "vinyl")

		// Struct-typed JSON field converts cleaned map into Go struct.
		jf := forms.NewModelForm[StructJSONModel](db, forms.WithFields(forms.AllFields)).
			Bind(vals("details", `{"title": "Spec", "count": 4}`))
		if !jf.IsValid() {
			t.Fatalf("StructJSONModel form invalid: %v", jf.Errors().Messages())
		}
		jsaved, err := jf.Save(ctx)
		if err != nil {
			t.Fatalf("Save StructJSONModel: %v", err)
		}
		djangotest.AssertEqual(t, jsaved.Details, JSONStructPayload{Title: "Spec", Count: 4})
	})
}

// typeNameOf returns the Go type name of v.
func typeNameOf(v any) string {
	switch v.(type) {
	case *forms.CharField:
		return "*forms.CharField"
	case *forms.IntegerField:
		return "*forms.IntegerField"
	case *forms.FloatField:
		return "*forms.FloatField"
	case *forms.DecimalField:
		return "*forms.DecimalField"
	case *forms.BooleanField:
		return "*forms.BooleanField"
	case *forms.NullBooleanField:
		return "*forms.NullBooleanField"
	case *forms.EmailField:
		return "*forms.EmailField"
	case *forms.URLField:
		return "*forms.URLField"
	case *forms.SlugField:
		return "*forms.SlugField"
	case *forms.UUIDField:
		return "*forms.UUIDField"
	case *forms.DateField:
		return "*forms.DateField"
	case *forms.TimeField:
		return "*forms.TimeField"
	case *forms.DateTimeField:
		return "*forms.DateTimeField"
	case *forms.DurationField:
		return "*forms.DurationField"
	case *forms.GenericIPAddressField:
		return "*forms.GenericIPAddressField"
	case *forms.JSONField:
		return "*forms.JSONField"
	case *forms.TypedChoiceField:
		return "*forms.TypedChoiceField"
	default:
		return "unknown"
	}
}

// mustFieldDef returns the named field asserted to type F.
func mustFieldDef[F forms.Field](t *testing.T, form *forms.Form, name string) F {
	t.Helper()
	raw, ok := form.FieldDef(name)
	if !ok {
		t.Fatalf("field %q not found", name)
	}
	typed, ok := raw.(F)
	if !ok {
		t.Fatalf("field %q is %T, want %T", name, raw, typed)
	}
	return typed
}
