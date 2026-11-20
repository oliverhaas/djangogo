package orm_test

import (
	"context"
	"math"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

type floatModel struct {
	ID   int64
	Size float64
}

type decimalFoo struct {
	ID int64
	A  string      `orm:"max_length=10"`
	D  orm.Decimal `orm:"max_digits=5;decimal_places=3"`
}

type bigD struct {
	ID       int64
	D        orm.Decimal  `orm:"max_digits=32;decimal_places=30"`
	LargeInt *orm.Decimal `orm:"max_digits=16;decimal_places=0;null"`
}

type decimalWithoutPrecision struct {
	ID    int64
	Value orm.Decimal
}

type intModel struct {
	ID    int64
	Value int32
}

type smallIntModel struct {
	ID    int64
	Value int16
}

type bigIntModel struct {
	ID    int64
	Value int64 `orm:"type=bigint"`
}

type positiveSmallIntModel struct {
	ID    int64
	Value int16 `orm:"type=positive_smallint"`
}

type positiveIntModel struct {
	ID    int64
	Value int32 `orm:"type=positive_int"`
}

type positiveBigIntModel struct {
	ID    int64
	Value int64 `orm:"type=positive_bigint"`
}

type unsignedIntModel struct {
	ID    int64
	Small uint16
	Med   uint32
	Big   uint64
}

// Ported from Django: tests/model_fields/test_floatfield.py::TestFloatField.test_float_validates_object
func TestFloatFieldSaveLoad(t *testing.T) {
	// Go's static type system rejects assigning a model instance to a float64
	// field at compile time; verify save, reload, and query roundtrip.
	djangotest.EachDB(t, []any{&floatModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		inst := floatModel{Size: 2.5}
		djangotest.AssertEqual(t, orm.Query[floatModel](db).Create(ctx, &inst), nil)
		if inst.ID == 0 {
			t.Fatal("expected non-zero ID after Create")
		}
		got, err := orm.Query[floatModel](db).Get(ctx, "pk", inst.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, got.Size, 2.5)

		got.Size = -3.75
		djangotest.AssertEqual(t, orm.Query[floatModel](db).Save(ctx, &got), nil)
		reloaded, err := orm.Query[floatModel](db).Get(ctx, "pk", inst.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, reloaded.Size, -3.75)
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_to_python
func TestDecimalFieldToGo(t *testing.T) {
	d, err := orm.ParseDecimal("3.14")
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, d.String(), "3.14")

	di := orm.DecimalFromInt(3)
	djangotest.AssertEqual(t, di.String(), "3")

	// ROUND_HALF_EVEN quantization matches Django's Decimal context.
	djangotest.AssertEqual(t, orm.MustDecimal("2.0625").Quantize(3).String(), "2.062")
	djangotest.AssertEqual(t, orm.MustDecimal("2.1875").Quantize(3).String(), "2.188")
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_filter_with_strings
func TestDecimalFieldFilterWithStrings(t *testing.T) {
	djangotest.EachDB(t, []any{&decimalFoo{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		foo := decimalFoo{A: "abc", D: orm.MustDecimal("12.34")}
		djangotest.AssertEqual(t, orm.Query[decimalFoo](db).Create(ctx, &foo), nil)

		rows, err := orm.Query[decimalFoo](db).Filter("d", "12.34").All(ctx)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(rows), 1)
		djangotest.AssertEqual(t, rows[0].ID, foo.ID)
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_save_without_float_conversion
func TestDecimalFieldSaveWithoutFloatConversion(t *testing.T) {
	djangotest.EachDB(t, []any{&bigD{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		bd := bigD{D: orm.MustDecimal("12.9")}
		djangotest.AssertEqual(t, orm.Query[bigD](db).Create(ctx, &bd), nil)
		loaded, err := orm.Query[bigD](db).Get(ctx, "pk", bd.ID)
		djangotest.AssertEqual(t, err, nil)
		if !loaded.D.Equal(orm.MustDecimal("12.9")) {
			t.Fatalf("got %s, want 12.9", loaded.D.String())
		}
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_save_nan_invalid
func TestDecimalFieldSaveNaNInvalid(t *testing.T) {
	if _, err := orm.ParseDecimal("nan"); err == nil {
		t.Fatal("expected error parsing nan")
	}
	if _, err := orm.DecimalFromFloat(math.NaN()); err == nil {
		t.Fatal("expected error converting math.NaN")
	}
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_save_inf_invalid
func TestDecimalFieldSaveInfInvalid(t *testing.T) {
	for _, s := range []string{"inf", "-inf", "+inf", "Infinity", "-Infinity"} {
		if _, err := orm.ParseDecimal(s); err == nil {
			t.Fatalf("expected error parsing %q", s)
		}
	}
	if _, err := orm.DecimalFromFloat(math.Inf(1)); err == nil {
		t.Fatal("expected error converting +Inf")
	}
	if _, err := orm.DecimalFromFloat(math.Inf(-1)); err == nil {
		t.Fatal("expected error converting -Inf")
	}
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_fetch_from_db_without_float_rounding
func TestDecimalFieldFetchWithoutFloatRounding(t *testing.T) {
	djangotest.EachDB(t, []any{&bigD{}}, func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "sqlite" {
			t.Skip("SQLite stores values rounded to 15 significant digits (matches Django sqlite3 features)")
		}
		ctx := context.Background()
		want := orm.MustDecimal(".100000000000000000000000000005")
		bd := bigD{D: want}
		djangotest.AssertEqual(t, orm.Query[bigD](db).Create(ctx, &bd), nil)
		loaded, err := orm.Query[bigD](db).Get(ctx, "pk", bd.ID)
		djangotest.AssertEqual(t, err, nil)
		if !loaded.D.Equal(want) {
			t.Fatalf("got %s, want %s", loaded.D.String(), want.String())
		}
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_lookup_really_big_value
func TestDecimalFieldLookupReallyBigValue(t *testing.T) {
	djangotest.EachDB(t, []any{&decimalFoo{}}, func(t *testing.T, db *orm.DB) {
		rows, err := orm.Query[decimalFoo](db).Filter("d__gte", int64(100000000000)).All(context.Background())
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(rows), 0)
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_lookup_decimal_larger_than_max_digits
func TestDecimalFieldLookupLargerThanMaxDigits(t *testing.T) {
	djangotest.EachDB(t, []any{&decimalFoo{}}, func(t *testing.T, db *orm.DB) {
		rows, err := orm.Query[decimalFoo](db).Filter("d__lte", orm.MustDecimal("123456")).All(context.Background())
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, len(rows), 0)
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_roundtrip_with_trailing_zeros
func TestDecimalFieldRoundtripWithTrailingZeros(t *testing.T) {
	djangotest.EachDB(t, []any{&decimalFoo{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		obj := decimalFoo{A: "bar", D: orm.MustDecimal("8.320")}
		djangotest.AssertEqual(t, orm.Query[decimalFoo](db).Create(ctx, &obj), nil)
		loaded, err := orm.Query[decimalFoo](db).Get(ctx, "pk", obj.ID)
		djangotest.AssertEqual(t, err, nil)
		if !loaded.D.ExactEqual(orm.MustDecimal("8.320")) {
			t.Fatalf("got %q, want exact 8.320", loaded.D.String())
		}
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_roundtrip_integer_with_trailing_zeros
func TestDecimalFieldRoundtripIntegerWithTrailingZeros(t *testing.T) {
	djangotest.EachDB(t, []any{&decimalFoo{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		obj := decimalFoo{A: "bar", D: orm.MustDecimal("8")}
		djangotest.AssertEqual(t, orm.Query[decimalFoo](db).Create(ctx, &obj), nil)
		loaded, err := orm.Query[decimalFoo](db).Get(ctx, "pk", obj.ID)
		djangotest.AssertEqual(t, err, nil)
		if !loaded.D.ExactEqual(orm.MustDecimal("8.000")) {
			t.Fatalf("got %q, want exact 8.000", loaded.D.String())
		}
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_large_integer_precision
func TestDecimalFieldLargeIntegerPrecision(t *testing.T) {
	djangotest.EachDB(t, []any{&bigD{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		large := orm.MustDecimal("9999999999999999")
		obj := bigD{D: orm.MustDecimal("0"), LargeInt: &large}
		djangotest.AssertEqual(t, orm.Query[bigD](db).Create(ctx, &obj), nil)
		loaded, err := orm.Query[bigD](db).Get(ctx, "pk", obj.ID)
		djangotest.AssertEqual(t, err, nil)
		if loaded.LargeInt == nil || !loaded.LargeInt.Equal(large) {
			t.Fatalf("got %v, want %s", loaded.LargeInt, large.String())
		}
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_large_integer_precision_aggregation
func TestDecimalFieldLargeIntegerPrecisionAggregation(t *testing.T) {
	djangotest.EachDB(t, []any{&bigD{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		large := orm.MustDecimal("9999999999999999")
		obj := bigD{D: orm.MustDecimal("0"), LargeInt: &large}
		djangotest.AssertEqual(t, orm.Query[bigD](db).Create(ctx, &obj), nil)

		res, err := orm.Query[bigD](db).Aggregate(ctx, "max_val", orm.Max("large_int"))
		djangotest.AssertEqual(t, err, nil)
		got, ok := res["max_val"].(orm.Decimal)
		if !ok {
			t.Fatalf("max_val has type %T, want orm.Decimal", res["max_val"])
		}
		if !got.Equal(large) {
			t.Fatalf("max_val = %s, want %s", got.String(), large.String())
		}
	})
}

// Ported from Django: tests/model_fields/test_decimalfield.py::DecimalFieldTests.test_decimalfield_without_precision
func TestDecimalFieldWithoutPrecision(t *testing.T) {
	djangotest.EachDB(t, []any{&decimalWithoutPrecision{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		want := orm.MustDecimal("0.99")
		obj := decimalWithoutPrecision{Value: want}
		djangotest.AssertEqual(t, orm.Query[decimalWithoutPrecision](db).Create(ctx, &obj), nil)
		loaded, err := orm.Query[decimalWithoutPrecision](db).Get(ctx, "pk", obj.ID)
		djangotest.AssertEqual(t, err, nil)
		if !loaded.Value.Equal(want) {
			t.Fatalf("got %s, want %s", loaded.Value.String(), want.String())
		}
	})
}

// Ported from Django: tests/model_fields/test_integerfield.py::IntegerFieldTests.test_documented_range
func TestIntegerFieldDocumentedRange(t *testing.T) {
	djangotest.EachDB(t, []any{&intModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, val := range []int32{-2147483648, 2147483647} {
			inst := intModel{Value: val}
			djangotest.AssertEqual(t, orm.Query[intModel](db).Create(ctx, &inst), nil)
			loaded, err := orm.Query[intModel](db).Get(ctx, "pk", inst.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, loaded.Value, val)
		}
	})
}

// Ported from Django: tests/model_fields/test_integerfield.py::SmallIntegerFieldTests.test_documented_range
func TestSmallIntegerFieldDocumentedRange(t *testing.T) {
	djangotest.EachDB(t, []any{&smallIntModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, val := range []int16{-32768, 32767} {
			inst := smallIntModel{Value: val}
			djangotest.AssertEqual(t, orm.Query[smallIntModel](db).Create(ctx, &inst), nil)
			loaded, err := orm.Query[smallIntModel](db).Get(ctx, "pk", inst.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, loaded.Value, val)
		}
	})
}

// Ported from Django: tests/model_fields/test_integerfield.py::BigIntegerFieldTests.test_documented_range
func TestBigIntegerFieldDocumentedRange(t *testing.T) {
	djangotest.EachDB(t, []any{&bigIntModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, val := range []int64{-9223372036854775808, 9223372036854775807} {
			inst := bigIntModel{Value: val}
			djangotest.AssertEqual(t, orm.Query[bigIntModel](db).Create(ctx, &inst), nil)
			loaded, err := orm.Query[bigIntModel](db).Get(ctx, "pk", inst.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, loaded.Value, val)
		}
	})
}

// Ported from Django: tests/model_fields/test_integerfield.py::PositiveSmallIntegerFieldTests.test_documented_range
func TestPositiveSmallIntegerFieldDocumentedRange(t *testing.T) {
	djangotest.EachDB(t, []any{&positiveSmallIntModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, val := range []int16{0, 32767} {
			inst := positiveSmallIntModel{Value: val}
			djangotest.AssertEqual(t, orm.Query[positiveSmallIntModel](db).Create(ctx, &inst), nil)
			loaded, err := orm.Query[positiveSmallIntModel](db).Get(ctx, "pk", inst.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, loaded.Value, val)
		}
		neg := positiveSmallIntModel{Value: -1}
		err := orm.Query[positiveSmallIntModel](db).Create(ctx, &neg)
		djangotest.AssertErrorIs(t, err, orm.ErrCheckViolation)
	})
}

// Ported from Django: tests/model_fields/test_integerfield.py::PositiveIntegerFieldTests.test_documented_range
// Ported from Django: tests/model_fields/test_integerfield.py::PositiveIntegerFieldTests.test_negative_values
func TestPositiveIntegerFieldDocumentedRangeAndNegative(t *testing.T) {
	djangotest.EachDB(t, []any{&positiveIntModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, val := range []int32{0, 2147483647} {
			inst := positiveIntModel{Value: val}
			djangotest.AssertEqual(t, orm.Query[positiveIntModel](db).Create(ctx, &inst), nil)
			loaded, err := orm.Query[positiveIntModel](db).Get(ctx, "pk", inst.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, loaded.Value, val)
		}
		p := positiveIntModel{Value: 0}
		djangotest.AssertEqual(t, orm.Query[positiveIntModel](db).Create(ctx, &p), nil)
		_, err := orm.Query[positiveIntModel](db).Filter("pk", p.ID).Update(ctx, "value", orm.F("value").Sub(1))
		djangotest.AssertErrorIs(t, err, orm.ErrCheckViolation)
	})
}

// Ported from Django: tests/model_fields/test_integerfield.py::PositiveBigIntegerFieldTests.test_documented_range
func TestPositiveBigIntegerFieldDocumentedRange(t *testing.T) {
	djangotest.EachDB(t, []any{&positiveBigIntModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for _, val := range []int64{0, 9223372036854775807} {
			inst := positiveBigIntModel{Value: val}
			djangotest.AssertEqual(t, orm.Query[positiveBigIntModel](db).Create(ctx, &inst), nil)
			loaded, err := orm.Query[positiveBigIntModel](db).Get(ctx, "pk", inst.ID)
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, loaded.Value, val)
		}
		neg := positiveBigIntModel{Value: -1}
		err := orm.Query[positiveBigIntModel](db).Create(ctx, &neg)
		djangotest.AssertErrorIs(t, err, orm.ErrCheckViolation)
	})
}

func TestUnsignedIntegerTypesRoundtrip(t *testing.T) {
	djangotest.EachDB(t, []any{&unsignedIntModel{}}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		m := unsignedIntModel{Small: 32767, Med: 2147483647, Big: 9223372036854775807}
		djangotest.AssertEqual(t, orm.Query[unsignedIntModel](db).Create(ctx, &m), nil)
		loaded, err := orm.Query[unsignedIntModel](db).Get(ctx, "pk", m.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, loaded, m)
	})
}
