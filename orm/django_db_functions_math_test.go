package orm_test

import (
	"context"
	"math"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// fnDecimalModel is Django's db_functions.DecimalModel.
type fnDecimalModel struct {
	ID int64
	N1 orm.Decimal  `orm:"max_digits=6;decimal_places=2"`
	N2 *orm.Decimal `orm:"max_digits=9;decimal_places=7;null;blank"`
}

// fnIntegerModel is Django's db_functions.IntegerModel.
type fnIntegerModel struct {
	ID     int64
	Big    *int64 `orm:"null;blank"`
	Normal *int32 `orm:"null;blank"`
	Small  *int16 `orm:"null;blank"`
}

// fnMathModels returns the models of the ports of tests/db_functions/math.
func fnMathModels() []any {
	return []any{djangotest.App("db_functions", &fnDecimalModel{}, &fnFloatModel{}, &fnIntegerModel{})}
}

// fnFirstRow returns the first row of vl.
func fnFirstRow(t *testing.T, vl *orm.ValuesListQuerySet) []any {
	t.Helper()
	rows, err := vl.All(context.Background())
	if err != nil {
		t.Fatalf("ValuesList: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("ValuesList returned no row")
	}
	return rows[0]
}

// Ported from Django: tests/db_functions/math/test_abs.py::AbsTests.test_null
func TestAbsNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_abs", orm.Abs("normal")).
			ValuesList("null_abs"))
		if got != nil {
			t.Errorf("null_abs = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_abs.py::AbsTests.test_decimal
func TestAbsDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		n2 := orm.MustDecimal("1.2")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-0.8"), N2: &n2})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).
			Annotate("n1_abs", orm.Abs("n1"), "n2_abs", orm.Abs("n2")).ValuesList("n1_abs", "n2_abs"))
		for i, want := range []orm.Decimal{orm.MustDecimal("0.8"), n2} {
			got, ok := row[i].(orm.Decimal)
			if !ok {
				t.Fatalf("value %d = %#v, want an orm.Decimal", i, row[i])
			}
			if !got.Equal(want) {
				t.Errorf("value %d = %s, want %s", i, got, want)
			}
		}
	})
}

// Ported from Django: tests/db_functions/math/test_abs.py::AbsTests.test_float
func TestAbsFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		f1, f2 := -0.5, 12.0
		create(t, db, &fnFloatModel{F1: &f1, F2: &f2})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).
			Annotate("f1_abs", orm.Abs("f1"), "f2_abs", orm.Abs("f2")).ValuesList("f1_abs", "f2_abs"))
		djangotest.AssertEqual(t, row, []any{0.5, 12.0})
	})
}

// Ported from Django: tests/db_functions/math/test_abs.py::AbsTests.test_integer
func TestAbsInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		big, normal, small := int64(-45), int32(0), int16(12)
		create(t, db, &fnIntegerModel{Big: &big, Normal: &normal, Small: &small})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).
			Annotate("small_abs", orm.Abs("small"), "normal_abs", orm.Abs("normal"), "big_abs", orm.Abs("big")).
			ValuesList("small_abs", "normal_abs", "big_abs"))
		djangotest.AssertEqual(t, row, []any{int64(12), int64(0), int64(45)})
	})
}

// Ported from Django: tests/db_functions/math/test_abs.py::AbsTests.test_transform
func TestAbsTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "abs", orm.Abs))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-1.5"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-0.5"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__abs__gt", 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("-1.5")) {
			t.Errorf("n1 = %s, want -1.5", obj.N1)
		}
	})
}

// fnAlmostEqual reports whether a and b agree to places decimal places,
// Python's assertAlmostEqual: round(a - b, places) == 0.
func fnAlmostEqual(a, b float64, places int) bool {
	return math.Round((a-b)*math.Pow(10, float64(places))) == 0
}

// fnAssertAlmostFloat fails t unless got is a float64 that agrees with want
// to places decimal places, Django's assertIsInstance(got, float) and
// assertAlmostEqual(got, want, places).
func fnAssertAlmostFloat(t *testing.T, got any, want float64, places int) {
	t.Helper()
	f, ok := got.(float64)
	if !ok {
		t.Errorf("value = %#v, want a float64", got)
		return
	}
	if !fnAlmostEqual(f, want, places) {
		t.Errorf("value = %v, want %v to %d places", f, want, places)
	}
}

// fnAssertAlmostDecimal fails t unless got is an orm.Decimal that agrees
// with want to places decimal places, Django's assertIsInstance(got, Decimal)
// and assertAlmostEqual(got, Decimal(want), places).
func fnAssertAlmostDecimal(t *testing.T, got any, want float64, places int) {
	t.Helper()
	d, ok := got.(orm.Decimal)
	if !ok {
		t.Errorf("value = %#v, want an orm.Decimal", got)
		return
	}
	if !fnAlmostEqual(d.Float64(), want, places) {
		t.Errorf("value = %s, want %v to %d places", d, want, places)
	}
}

// fnAssertNulls fails t unless every value of row is nil.
func fnAssertNulls(t *testing.T, row []any) {
	t.Helper()
	for i, v := range row {
		if v != nil {
			t.Errorf("value %d = %#v, want nil", i, v)
		}
	}
}

// fnSkipNegativePrecision skips t on SQLite, which Django's
// django_test_skips name for the Round tests with a negative precision.
func fnSkipNegativePrecision(t *testing.T, db *orm.DB) {
	t.Helper()
	if db.Dialect().Name() == "sqlite" {
		t.Skip("SQLite doesn't support negative precision for ROUND().")
	}
}

// Ported from Django: tests/db_functions/math/test_atan2.py::ATan2Tests.test_null
func TestATan2Null(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Big: new(int64(100))})
		fnAssertNulls(t, fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"null_atan2_sn", orm.ATan2("small", "normal"),
			"null_atan2_nb", orm.ATan2("normal", "big"),
			"null_atan2_bn", orm.ATan2("big", "normal"),
		).ValuesList("null_atan2_sn", "null_atan2_nb", "null_atan2_bn")))
	})
}

// Ported from Django: tests/db_functions/math/test_atan2.py::ATan2Tests.test_decimal
func TestATan2Decimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-9.9"), N2: new(orm.MustDecimal("4.6"))})
		got := firstValue(t, orm.Query[fnDecimalModel](db).Annotate("n_atan2", orm.ATan2("n1", "n2")).ValuesList("n_atan2"))
		fnAssertAlmostDecimal(t, got, math.Atan2(-9.9, 4.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_atan2.py::ATan2Tests.test_float
func TestATan2Float(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-25.0), F2: new(0.33)})
		got := firstValue(t, orm.Query[fnFloatModel](db).Annotate("f_atan2", orm.ATan2("f1", "f2")).ValuesList("f_atan2"))
		fnAssertAlmostFloat(t, got, math.Atan2(-25, 0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_atan2.py::ATan2Tests.test_integer
func TestATan2Integer(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(0)), Normal: new(int32(1)), Big: new(int64(10))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"atan2_sn", orm.ATan2("small", "normal"), "atan2_nb", orm.ATan2("normal", "big"),
		).ValuesList("atan2_sn", "atan2_nb"))
		fnAssertAlmostFloat(t, row[0], math.Atan2(0, 1), 7)
		fnAssertAlmostFloat(t, row[1], math.Atan2(1, 10), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_log.py::LogTests.test_null
func TestLogNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Big: new(int64(100))})
		fnAssertNulls(t, fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"null_log_small", orm.Log("small", "normal"),
			"null_log_normal", orm.Log("normal", "big"),
			"null_log_big", orm.Log("big", "normal"),
		).ValuesList("null_log_small", "null_log_normal", "null_log_big")))
	})
}

// Ported from Django: tests/db_functions/math/test_log.py::LogTests.test_decimal
func TestLogDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.9"), N2: new(orm.MustDecimal("3.6"))})
		got := firstValue(t, orm.Query[fnDecimalModel](db).Annotate("n_log", orm.Log("n1", "n2")).ValuesList("n_log"))
		fnAssertAlmostDecimal(t, got, math.Log(3.6)/math.Log(12.9), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_log.py::LogTests.test_float
func TestLogFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(2.0), F2: new(4.0)})
		got := firstValue(t, orm.Query[fnFloatModel](db).Annotate("f_log", orm.Log("f1", "f2")).ValuesList("f_log"))
		fnAssertAlmostFloat(t, got, math.Log(4)/math.Log(2), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_log.py::LogTests.test_integer
func TestLogInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(4)), Normal: new(int32(8)), Big: new(int64(2))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_log", orm.Log("small", "big"),
			"normal_log", orm.Log("normal", "big"),
			"big_log", orm.Log("big", "big"),
		).ValuesList("small_log", "normal_log", "big_log"))
		fnAssertAlmostFloat(t, row[0], math.Log(2)/math.Log(4), 7)
		fnAssertAlmostFloat(t, row[1], math.Log(2)/math.Log(8), 7)
		fnAssertAlmostFloat(t, row[2], 1, 7)
	})
}

// Ported from Django: tests/db_functions/math/test_mod.py::ModTests.test_null
func TestModNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Big: new(int64(100))})
		fnAssertNulls(t, fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"null_mod_small", orm.Mod("small", "normal"),
			"null_mod_normal", orm.Mod("normal", "big"),
		).ValuesList("null_mod_small", "null_mod_normal")))
	})
}

// Ported from Django: tests/db_functions/math/test_mod.py::ModTests.test_decimal
func TestModDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-9.9"), N2: new(orm.MustDecimal("4.6"))})
		got := firstValue(t, orm.Query[fnDecimalModel](db).Annotate("n_mod", orm.Mod("n1", "n2")).ValuesList("n_mod"))
		fnAssertAlmostDecimal(t, got, math.Mod(-9.9, 4.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_mod.py::ModTests.test_float
func TestModFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-25.0), F2: new(0.33)})
		got := firstValue(t, orm.Query[fnFloatModel](db).Annotate("f_mod", orm.Mod("f1", "f2")).ValuesList("f_mod"))
		fnAssertAlmostFloat(t, got, math.Mod(-25, 0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_mod.py::ModTests.test_integer
func TestModInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(20)), Normal: new(int32(15)), Big: new(int64(1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_mod", orm.Mod("small", "normal"),
			"normal_mod", orm.Mod("normal", "big"),
			"big_mod", orm.Mod("big", "small"),
		).ValuesList("small_mod", "normal_mod", "big_mod"))
		djangotest.AssertEqual(t, row, []any{math.Mod(20, 15), math.Mod(15, 1), math.Mod(1, 20)})
	})
}

// Ported from Django: tests/db_functions/math/test_pi.py::PiTests.test
func TestPi(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(2.5), F2: new(15.9)})
		got := firstValue(t, orm.Query[fnFloatModel](db).Annotate("pi", orm.Pi()).ValuesList("pi"))
		fnAssertAlmostFloat(t, got, math.Pi, 5)
	})
}

// Ported from Django: tests/db_functions/math/test_power.py::PowerTests.test_null
func TestPowerNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Big: new(int64(100))})
		fnAssertNulls(t, fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"null_power_small", orm.Power("small", "normal"),
			"null_power_normal", orm.Power("normal", "big"),
			"null_power_big", orm.Power("big", "normal"),
		).ValuesList("null_power_small", "null_power_normal", "null_power_big")))
	})
}

// Ported from Django: tests/db_functions/math/test_power.py::PowerTests.test_decimal
func TestPowerDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.0"), N2: new(orm.MustDecimal("-0.6"))})
		got := firstValue(t, orm.Query[fnDecimalModel](db).Annotate("n_power", orm.Power("n1", "n2")).ValuesList("n_power"))
		fnAssertAlmostDecimal(t, got, math.Pow(1.0, -0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_power.py::PowerTests.test_float
func TestPowerFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(2.3), F2: new(1.1)})
		got := firstValue(t, orm.Query[fnFloatModel](db).Annotate("f_power", orm.Power("f1", "f2")).ValuesList("f_power"))
		fnAssertAlmostFloat(t, got, math.Pow(2.3, 1.1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_power.py::PowerTests.test_integer
func TestPowerInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-1)), Normal: new(int32(20)), Big: new(int64(3))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_power", orm.Power("small", "normal"),
			"normal_power", orm.Power("normal", "big"),
			"big_power", orm.Power("big", "small"),
		).ValuesList("small_power", "normal_power", "big_power"))
		fnAssertAlmostFloat(t, row[0], math.Pow(-1, 20), 7)
		fnAssertAlmostFloat(t, row[1], 8000, 7)
		fnAssertAlmostFloat(t, row[2], math.Pow(3, -1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_random.py::RandomTests.test
func TestRandom(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{})
		got, ok := firstValue(t, orm.Query[fnFloatModel](db).Annotate("random", orm.Random()).ValuesList("random")).(float64)
		if !ok || got < 0 || got >= 1 {
			t.Errorf("random = %v, want a float64 in [0, 1)", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_null
func TestRoundNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		fnAssertNulls(t, fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate("null_round", orm.Round("normal", 0)).ValuesList("null_round")))
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_null_with_precision
func TestRoundNullWithPrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		fnAssertNulls(t, fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate("null_round", orm.Round("normal", 5)).ValuesList("null_round")))
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_null_with_negative_precision
func TestRoundNullWithNegativePrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		fnSkipNegativePrecision(t, db)
		create(t, db, &fnIntegerModel{})
		fnAssertNulls(t, fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate("null_round", orm.Round("normal", -1)).ValuesList("null_round")))
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_decimal
func TestRoundDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_round", orm.Round("n1", 0), "n2_round", orm.Round("n2", 0)).ValuesList("n1_round", "n2_round"))
		fnAssertAlmostDecimal(t, row[0], -12.9, 0)
		fnAssertAlmostDecimal(t, row[1], 0.6, 0)
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_decimal_with_precision
//
// A Go field cannot hold an expression, so n2 is set to Pi() with an update.
func TestRoundDecimalWithPrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-5.75")})
		if _, err := orm.Query[fnDecimalModel](db).Update(context.Background(), "n2", orm.Pi()); err != nil {
			t.Fatalf("Update: %v", err)
		}
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_round", orm.Round("n1", 1), "n2_round", orm.Round("n2", 5)).ValuesList("n1_round", "n2_round", "n2"))
		fnAssertAlmostDecimal(t, row[0], -5.75, 1)
		fnAssertAlmostDecimal(t, row[1], row[2].(orm.Decimal).Float64(), 5)
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_decimal_with_negative_precision
func TestRoundDecimalWithNegativePrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		fnSkipNegativePrecision(t, db)
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("365.25")})
		got := firstValue(t, orm.Query[fnDecimalModel](db).Annotate("n1_round", orm.Round("n1", -1)).ValuesList("n1_round"))
		d, ok := got.(orm.Decimal)
		if !ok || !d.Equal(orm.MustDecimal("370")) {
			t.Errorf("n1_round = %#v, want the Decimal 370", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_float
func TestRoundFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.55), F2: new(0.55)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_round", orm.Round("f1", 0), "f2_round", orm.Round("f2", 0)).ValuesList("f1_round", "f2_round"))
		fnAssertAlmostFloat(t, row[0], -27.55, 0)
		fnAssertAlmostFloat(t, row[1], 0.55, 0)
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_float_with_precision
//
// A Go field cannot hold an expression, so f2 is set to Pi() with an update.
func TestRoundFloatWithPrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-5.75)})
		if _, err := orm.Query[fnFloatModel](db).Update(context.Background(), "f2", orm.Pi()); err != nil {
			t.Fatalf("Update: %v", err)
		}
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_round", orm.Round("f1", 1), "f2_round", orm.Round("f2", 5)).ValuesList("f1_round", "f2_round"))
		fnAssertAlmostFloat(t, row[0], -5.75, 1)
		fnAssertAlmostFloat(t, row[1], math.Pi, 5)
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_float_with_negative_precision
func TestRoundFloatWithNegativePrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		fnSkipNegativePrecision(t, db)
		create(t, db, &fnFloatModel{F1: new(365.25)})
		got := firstValue(t, orm.Query[fnFloatModel](db).Annotate("f1_round", orm.Round("f1", -1)).ValuesList("f1_round"))
		djangotest.AssertEqual(t, got, any(370.0))
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_integer
func TestRoundInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_round", orm.Round("small", 0), "normal_round", orm.Round("normal", 0), "big_round", orm.Round("big", 0),
		).ValuesList("small_round", "normal_round", "big_round"))
		djangotest.AssertEqual(t, row, []any{int64(-20), int64(15), int64(-1)})
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_integer_with_precision
func TestRoundIntegerWithPrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-5)), Normal: new(int32(3)), Big: new(int64(-100))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_round", orm.Round("small", 1), "normal_round", orm.Round("normal", 5), "big_round", orm.Round("big", 2),
		).ValuesList("small_round", "normal_round", "big_round"))
		djangotest.AssertEqual(t, row, []any{int64(-5), int64(3), int64(-100)})
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_integer_with_negative_precision
//
// Neither database rounds to even (rounds_to_even).
func TestRoundIntegerWithNegativePrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		fnSkipNegativePrecision(t, db)
		create(t, db, &fnIntegerModel{Normal: new(int32(365))})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("normal_round", orm.Round("normal", -1)).ValuesList("normal_round"))
		djangotest.AssertEqual(t, got, any(int64(370)))
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_transform
func TestRoundTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "round", func(arg any) orm.Expr { return orm.Round(arg, 0) }))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("2.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-1.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__round__gt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("2.0")) {
			t.Errorf("n1 = %s, want 2.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_round.py::RoundTests.test_unsupported_negative_precision
//
// Django raises the error when the query is compiled; the query fails with
// it.
func TestRoundUnsupportedNegativePrecision(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() != "sqlite" {
			t.Skip("SQLite doesn't support negative precision.")
		}
		create(t, db, &fnFloatModel{F1: new(123.45)})
		fnAssertQueryError(t, orm.Query[fnFloatModel](db).Annotate("value", orm.Round("f1", -1)).ValuesList("value"),
			"SQLite does not support negative precision.")
	})
}

// Ported from Django: tests/db_functions/math/test_acos.py::ACosTests.test_null
func TestACosNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_acos", orm.ACos("normal")).ValuesList("null_acos"))
		if got != nil {
			t.Errorf("null_acos = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_acos.py::ACosTests.test_decimal
func TestACosDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-0.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_acos", orm.ACos("n1"), "n2_acos", orm.ACos("n2")).ValuesList("n1_acos", "n2_acos"))
		fnAssertAlmostDecimal(t, row[0], math.Acos(-0.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Acos(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_acos.py::ACosTests.test_float
func TestACosFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-0.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_acos", orm.ACos("f1"), "f2_acos", orm.ACos("f2")).ValuesList("f1_acos", "f2_acos"))
		fnAssertAlmostFloat(t, row[0], math.Acos(-0.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Acos(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_acos.py::ACosTests.test_integer
func TestACosInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(0)), Normal: new(int32(1)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_acos", orm.ACos("small"), "normal_acos", orm.ACos("normal"), "big_acos", orm.ACos("big"),
		).ValuesList("small_acos", "normal_acos", "big_acos"))
		fnAssertAlmostFloat(t, row[0], math.Acos(0), 7)
		fnAssertAlmostFloat(t, row[1], math.Acos(1), 7)
		fnAssertAlmostFloat(t, row[2], math.Acos(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_acos.py::ACosTests.test_transform
func TestACosTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "acos", orm.ACos))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("0.5"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-0.9"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__acos__lt", 2)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("0.5")) {
			t.Errorf("n1 = %s, want 0.5", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_asin.py::ASinTests.test_null
func TestASinNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_asin", orm.ASin("normal")).ValuesList("null_asin"))
		if got != nil {
			t.Errorf("null_asin = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_asin.py::ASinTests.test_decimal
func TestASinDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("0.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_asin", orm.ASin("n1"), "n2_asin", orm.ASin("n2")).ValuesList("n1_asin", "n2_asin"))
		fnAssertAlmostDecimal(t, row[0], math.Asin(0.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Asin(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_asin.py::ASinTests.test_float
func TestASinFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-0.5), F2: new(0.87)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_asin", orm.ASin("f1"), "f2_asin", orm.ASin("f2")).ValuesList("f1_asin", "f2_asin"))
		fnAssertAlmostFloat(t, row[0], math.Asin(-0.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Asin(0.87), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_asin.py::ASinTests.test_integer
func TestASinInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(0)), Normal: new(int32(1)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_asin", orm.ASin("small"), "normal_asin", orm.ASin("normal"), "big_asin", orm.ASin("big"),
		).ValuesList("small_asin", "normal_asin", "big_asin"))
		fnAssertAlmostFloat(t, row[0], math.Asin(0), 7)
		fnAssertAlmostFloat(t, row[1], math.Asin(1), 7)
		fnAssertAlmostFloat(t, row[2], math.Asin(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_asin.py::ASinTests.test_transform
func TestASinTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "asin", orm.ASin))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("0.1"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__asin__gt", 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("1.0")) {
			t.Errorf("n1 = %s, want 1.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_atan.py::ATanTests.test_null
func TestATanNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_atan", orm.ATan("normal")).ValuesList("null_atan"))
		if got != nil {
			t.Errorf("null_atan = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_atan.py::ATanTests.test_decimal
func TestATanDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_atan", orm.ATan("n1"), "n2_atan", orm.ATan("n2")).ValuesList("n1_atan", "n2_atan"))
		fnAssertAlmostDecimal(t, row[0], math.Atan(-12.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Atan(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_atan.py::ATanTests.test_float
func TestATanFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_atan", orm.ATan("f1"), "f2_atan", orm.ATan("f2")).ValuesList("f1_atan", "f2_atan"))
		fnAssertAlmostFloat(t, row[0], math.Atan(-27.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Atan(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_atan.py::ATanTests.test_integer
func TestATanInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_atan", orm.ATan("small"), "normal_atan", orm.ATan("normal"), "big_atan", orm.ATan("big"),
		).ValuesList("small_atan", "normal_atan", "big_atan"))
		fnAssertAlmostFloat(t, row[0], math.Atan(-20), 7)
		fnAssertAlmostFloat(t, row[1], math.Atan(15), 7)
		fnAssertAlmostFloat(t, row[2], math.Atan(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_atan.py::ATanTests.test_transform
func TestATanTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "atan", orm.ATan))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("3.12"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-5"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__atan__gt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("3.12")) {
			t.Errorf("n1 = %s, want 3.12", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_ceil.py::CeilTests.test_null
func TestCeilNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_ceil", orm.Ceil("normal")).ValuesList("null_ceil"))
		if got != nil {
			t.Errorf("null_ceil = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_ceil.py::CeilTests.test_decimal
func TestCeilDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_ceil", orm.Ceil("n1"), "n2_ceil", orm.Ceil("n2")).ValuesList("n1_ceil", "n2_ceil"))
		for i, want := range []orm.Decimal{orm.MustDecimal("13"), orm.MustDecimal("1")} {
			got, ok := row[i].(orm.Decimal)
			if !ok {
				t.Fatalf("value %d = %#v, want an orm.Decimal", i, row[i])
			}
			if !got.Equal(want) {
				t.Errorf("value %d = %s, want %s", i, got, want)
			}
		}
	})
}

// Ported from Django: tests/db_functions/math/test_ceil.py::CeilTests.test_float
func TestCeilFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-12.5), F2: new(21.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_ceil", orm.Ceil("f1"), "f2_ceil", orm.Ceil("f2")).ValuesList("f1_ceil", "f2_ceil"))
		djangotest.AssertEqual(t, row, []any{math.Ceil(-12.5), math.Ceil(21.33)})
	})
}

// Ported from Django: tests/db_functions/math/test_ceil.py::CeilTests.test_integer
func TestCeilInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-11)), Normal: new(int32(0)), Big: new(int64(-100))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_ceil", orm.Ceil("small"), "normal_ceil", orm.Ceil("normal"), "big_ceil", orm.Ceil("big"),
		).ValuesList("small_ceil", "normal_ceil", "big_ceil"))
		djangotest.AssertEqual(t, row, []any{int64(-11), int64(0), int64(-100)})
	})
}

// Ported from Django: tests/db_functions/math/test_ceil.py::CeilTests.test_transform
func TestCeilTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "ceil", orm.Ceil))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("3.12"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.25"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__ceil__gt", 3)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("3.12")) {
			t.Errorf("n1 = %s, want 3.12", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_cos.py::CosTests.test_null
func TestCosNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_cos", orm.Cos("normal")).ValuesList("null_cos"))
		if got != nil {
			t.Errorf("null_cos = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_cos.py::CosTests.test_decimal
func TestCosDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_cos", orm.Cos("n1"), "n2_cos", orm.Cos("n2")).ValuesList("n1_cos", "n2_cos"))
		fnAssertAlmostDecimal(t, row[0], math.Cos(-12.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Cos(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_cos.py::CosTests.test_float
func TestCosFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_cos", orm.Cos("f1"), "f2_cos", orm.Cos("f2")).ValuesList("f1_cos", "f2_cos"))
		fnAssertAlmostFloat(t, row[0], math.Cos(-27.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Cos(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_cos.py::CosTests.test_integer
func TestCosInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_cos", orm.Cos("small"), "normal_cos", orm.Cos("normal"), "big_cos", orm.Cos("big"),
		).ValuesList("small_cos", "normal_cos", "big_cos"))
		fnAssertAlmostFloat(t, row[0], math.Cos(-20), 7)
		fnAssertAlmostFloat(t, row[1], math.Cos(15), 7)
		fnAssertAlmostFloat(t, row[2], math.Cos(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_cos.py::CosTests.test_transform
func TestCosTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "cos", orm.Cos))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-8.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("3.14"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__cos__gt", -0.2)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("-8.0")) {
			t.Errorf("n1 = %s, want -8.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_cot.py::CotTests.test_null
func TestCotNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_cot", orm.Cot("normal")).ValuesList("null_cot"))
		if got != nil {
			t.Errorf("null_cot = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_cot.py::CotTests.test_decimal
func TestCotDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_cot", orm.Cot("n1"), "n2_cot", orm.Cot("n2")).ValuesList("n1_cot", "n2_cot"))
		fnAssertAlmostDecimal(t, row[0], 1/math.Tan(-12.9), 7)
		fnAssertAlmostDecimal(t, row[1], 1/math.Tan(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_cot.py::CotTests.test_float
func TestCotFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_cot", orm.Cot("f1"), "f2_cot", orm.Cot("f2")).ValuesList("f1_cot", "f2_cot"))
		fnAssertAlmostFloat(t, row[0], 1/math.Tan(-27.5), 7)
		fnAssertAlmostFloat(t, row[1], 1/math.Tan(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_cot.py::CotTests.test_integer
func TestCotInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-5)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_cot", orm.Cot("small"), "normal_cot", orm.Cot("normal"), "big_cot", orm.Cot("big"),
		).ValuesList("small_cot", "normal_cot", "big_cot"))
		fnAssertAlmostFloat(t, row[0], 1/math.Tan(-5), 7)
		fnAssertAlmostFloat(t, row[1], 1/math.Tan(15), 7)
		fnAssertAlmostFloat(t, row[2], 1/math.Tan(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_cot.py::CotTests.test_transform
func TestCotTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "cot", orm.Cot))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__cot__gt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("1.0")) {
			t.Errorf("n1 = %s, want 1.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_degrees.py::DegreesTests.test_null
func TestDegreesNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_degrees", orm.Degrees("normal")).ValuesList("null_degrees"))
		if got != nil {
			t.Errorf("null_degrees = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_degrees.py::DegreesTests.test_decimal
func TestDegreesDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_degrees", orm.Degrees("n1"), "n2_degrees", orm.Degrees("n2")).ValuesList("n1_degrees", "n2_degrees"))
		fnAssertAlmostDecimal(t, row[0], -12.9*180/math.Pi, 7)
		fnAssertAlmostDecimal(t, row[1], 0.6*180/math.Pi, 7)
	})
}

// Ported from Django: tests/db_functions/math/test_degrees.py::DegreesTests.test_float
func TestDegreesFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_degrees", orm.Degrees("f1"), "f2_degrees", orm.Degrees("f2")).ValuesList("f1_degrees", "f2_degrees"))
		fnAssertAlmostFloat(t, row[0], -27.5*180/math.Pi, 7)
		fnAssertAlmostFloat(t, row[1], 0.33*180/math.Pi, 7)
	})
}

// Ported from Django: tests/db_functions/math/test_degrees.py::DegreesTests.test_integer
func TestDegreesInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_degrees", orm.Degrees("small"), "normal_degrees", orm.Degrees("normal"), "big_degrees", orm.Degrees("big"),
		).ValuesList("small_degrees", "normal_degrees", "big_degrees"))
		fnAssertAlmostFloat(t, row[0], -20*180/math.Pi, 7)
		fnAssertAlmostFloat(t, row[1], 15*180/math.Pi, 7)
		fnAssertAlmostFloat(t, row[2], -1*180/math.Pi, 7)
	})
}

// Ported from Django: tests/db_functions/math/test_degrees.py::DegreesTests.test_transform
func TestDegreesTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "degrees", orm.Degrees))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("5.4"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-30"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__degrees__gt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("5.4")) {
			t.Errorf("n1 = %s, want 5.4", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_exp.py::ExpTests.test_null
func TestExpNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_exp", orm.Exp("normal")).ValuesList("null_exp"))
		if got != nil {
			t.Errorf("null_exp = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_exp.py::ExpTests.test_decimal
func TestExpDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_exp", orm.Exp("n1"), "n2_exp", orm.Exp("n2")).ValuesList("n1_exp", "n2_exp"))
		fnAssertAlmostDecimal(t, row[0], math.Exp(-12.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Exp(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_exp.py::ExpTests.test_float
func TestExpFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_exp", orm.Exp("f1"), "f2_exp", orm.Exp("f2")).ValuesList("f1_exp", "f2_exp"))
		fnAssertAlmostFloat(t, row[0], math.Exp(-27.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Exp(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_exp.py::ExpTests.test_integer
func TestExpInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_exp", orm.Exp("small"), "normal_exp", orm.Exp("normal"), "big_exp", orm.Exp("big"),
		).ValuesList("small_exp", "normal_exp", "big_exp"))
		fnAssertAlmostFloat(t, row[0], math.Exp(-20), 7)
		fnAssertAlmostFloat(t, row[1], math.Exp(15), 7)
		fnAssertAlmostFloat(t, row[2], math.Exp(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_exp.py::ExpTests.test_transform
func TestExpTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "exp", orm.Exp))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-1.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__exp__gt", 10)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("12.0")) {
			t.Errorf("n1 = %s, want 12.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_floor.py::FloorTests.test_null
func TestFloorNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_floor", orm.Floor("normal")).ValuesList("null_floor"))
		if got != nil {
			t.Errorf("null_floor = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_floor.py::FloorTests.test_decimal
func TestFloorDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_floor", orm.Floor("n1"), "n2_floor", orm.Floor("n2")).ValuesList("n1_floor", "n2_floor"))
		for i, want := range []orm.Decimal{orm.MustDecimal("-13"), orm.MustDecimal("0")} {
			got, ok := row[i].(orm.Decimal)
			if !ok {
				t.Fatalf("value %d = %#v, want an orm.Decimal", i, row[i])
			}
			if !got.Equal(want) {
				t.Errorf("value %d = %s, want %s", i, got, want)
			}
		}
	})
}

// Ported from Django: tests/db_functions/math/test_floor.py::FloorTests.test_float
func TestFloorFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_floor", orm.Floor("f1"), "f2_floor", orm.Floor("f2")).ValuesList("f1_floor", "f2_floor"))
		djangotest.AssertEqual(t, row, []any{math.Floor(-27.5), math.Floor(0.33)})
	})
}

// Ported from Django: tests/db_functions/math/test_floor.py::FloorTests.test_integer
func TestFloorInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_floor", orm.Floor("small"), "normal_floor", orm.Floor("normal"), "big_floor", orm.Floor("big"),
		).ValuesList("small_floor", "normal_floor", "big_floor"))
		djangotest.AssertEqual(t, row, []any{int64(-20), int64(15), int64(-1)})
	})
}

// Ported from Django: tests/db_functions/math/test_floor.py::FloorTests.test_transform
func TestFloorTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "floor", orm.Floor))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("5.4"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("3.4"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__floor__gt", 4)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("5.4")) {
			t.Errorf("n1 = %s, want 5.4", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_ln.py::LnTests.test_null
func TestLnNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_ln", orm.Ln("normal")).ValuesList("null_ln"))
		if got != nil {
			t.Errorf("null_ln = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_ln.py::LnTests.test_decimal
func TestLnDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_ln", orm.Ln("n1"), "n2_ln", orm.Ln("n2")).ValuesList("n1_ln", "n2_ln"))
		fnAssertAlmostDecimal(t, row[0], math.Log(12.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Log(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_ln.py::LnTests.test_float
func TestLnFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_ln", orm.Ln("f1"), "f2_ln", orm.Ln("f2")).ValuesList("f1_ln", "f2_ln"))
		fnAssertAlmostFloat(t, row[0], math.Log(27.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Log(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_ln.py::LnTests.test_integer
func TestLnInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(20)), Normal: new(int32(15)), Big: new(int64(1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_ln", orm.Ln("small"), "normal_ln", orm.Ln("normal"), "big_ln", orm.Ln("big"),
		).ValuesList("small_ln", "normal_ln", "big_ln"))
		fnAssertAlmostFloat(t, row[0], math.Log(20), 7)
		fnAssertAlmostFloat(t, row[1], math.Log(15), 7)
		fnAssertAlmostFloat(t, row[2], math.Log(1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_ln.py::LnTests.test_transform
func TestLnTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "ln", orm.Ln))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__ln__gt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("12.0")) {
			t.Errorf("n1 = %s, want 12.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_radians.py::RadiansTests.test_null
func TestRadiansNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_radians", orm.Radians("normal")).ValuesList("null_radians"))
		if got != nil {
			t.Errorf("null_radians = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_radians.py::RadiansTests.test_decimal
func TestRadiansDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_radians", orm.Radians("n1"), "n2_radians", orm.Radians("n2")).ValuesList("n1_radians", "n2_radians"))
		fnAssertAlmostDecimal(t, row[0], -12.9*math.Pi/180, 7)
		fnAssertAlmostDecimal(t, row[1], 0.6*math.Pi/180, 7)
	})
}

// Ported from Django: tests/db_functions/math/test_radians.py::RadiansTests.test_float
func TestRadiansFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_radians", orm.Radians("f1"), "f2_radians", orm.Radians("f2")).ValuesList("f1_radians", "f2_radians"))
		fnAssertAlmostFloat(t, row[0], -27.5*math.Pi/180, 7)
		fnAssertAlmostFloat(t, row[1], 0.33*math.Pi/180, 7)
	})
}

// Ported from Django: tests/db_functions/math/test_radians.py::RadiansTests.test_integer
func TestRadiansInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_radians", orm.Radians("small"), "normal_radians", orm.Radians("normal"), "big_radians", orm.Radians("big"),
		).ValuesList("small_radians", "normal_radians", "big_radians"))
		fnAssertAlmostFloat(t, row[0], -20*math.Pi/180, 7)
		fnAssertAlmostFloat(t, row[1], 15*math.Pi/180, 7)
		fnAssertAlmostFloat(t, row[2], -1*math.Pi/180, 7)
	})
}

// Ported from Django: tests/db_functions/math/test_radians.py::RadiansTests.test_transform
func TestRadiansTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "radians", orm.Radians))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("2.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-1.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__radians__gt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("2.0")) {
			t.Errorf("n1 = %s, want 2.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_sign.py::SignTests.test_null
func TestSignNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_sign", orm.Sign("normal")).ValuesList("null_sign"))
		if got != nil {
			t.Errorf("null_sign = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_sign.py::SignTests.test_decimal
func TestSignDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_sign", orm.Sign("n1"), "n2_sign", orm.Sign("n2")).ValuesList("n1_sign", "n2_sign"))
		for i, want := range []orm.Decimal{orm.MustDecimal("-1"), orm.MustDecimal("1")} {
			got, ok := row[i].(orm.Decimal)
			if !ok {
				t.Fatalf("value %d = %#v, want an orm.Decimal", i, row[i])
			}
			if !got.Equal(want) {
				t.Errorf("value %d = %s, want %s", i, got, want)
			}
		}
	})
}

// Ported from Django: tests/db_functions/math/test_sign.py::SignTests.test_float
func TestSignFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_sign", orm.Sign("f1"), "f2_sign", orm.Sign("f2")).ValuesList("f1_sign", "f2_sign"))
		djangotest.AssertEqual(t, row, []any{-1.0, 1.0})
	})
}

// Ported from Django: tests/db_functions/math/test_sign.py::SignTests.test_integer
func TestSignInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(0)), Big: new(int64(20))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_sign", orm.Sign("small"), "normal_sign", orm.Sign("normal"), "big_sign", orm.Sign("big"),
		).ValuesList("small_sign", "normal_sign", "big_sign"))
		djangotest.AssertEqual(t, row, []any{int64(-1), int64(0), int64(1)})
	})
}

// Ported from Django: tests/db_functions/math/test_sign.py::SignTests.test_transform
func TestSignTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "sign", orm.Sign))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("5.4"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-0.1"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__sign__lt", 0, "n2__sign", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("-0.1")) {
			t.Errorf("n1 = %s, want -0.1", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_sin.py::SinTests.test_null
func TestSinNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_sin", orm.Sin("normal")).ValuesList("null_sin"))
		if got != nil {
			t.Errorf("null_sin = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_sin.py::SinTests.test_decimal
func TestSinDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_sin", orm.Sin("n1"), "n2_sin", orm.Sin("n2")).ValuesList("n1_sin", "n2_sin"))
		fnAssertAlmostDecimal(t, row[0], math.Sin(-12.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Sin(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_sin.py::SinTests.test_float
func TestSinFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_sin", orm.Sin("f1"), "f2_sin", orm.Sin("f2")).ValuesList("f1_sin", "f2_sin"))
		fnAssertAlmostFloat(t, row[0], math.Sin(-27.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Sin(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_sin.py::SinTests.test_integer
func TestSinInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_sin", orm.Sin("small"), "normal_sin", orm.Sin("normal"), "big_sin", orm.Sin("big"),
		).ValuesList("small_sin", "normal_sin", "big_sin"))
		fnAssertAlmostFloat(t, row[0], math.Sin(-20), 7)
		fnAssertAlmostFloat(t, row[1], math.Sin(15), 7)
		fnAssertAlmostFloat(t, row[2], math.Sin(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_sin.py::SinTests.test_transform
func TestSinTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "sin", orm.Sin))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("5.4"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("0.1"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__sin__lt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("5.4")) {
			t.Errorf("n1 = %s, want 5.4", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_sqrt.py::SqrtTests.test_null
func TestSqrtNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_sqrt", orm.Sqrt("normal")).ValuesList("null_sqrt"))
		if got != nil {
			t.Errorf("null_sqrt = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_sqrt.py::SqrtTests.test_decimal
func TestSqrtDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_sqrt", orm.Sqrt("n1"), "n2_sqrt", orm.Sqrt("n2")).ValuesList("n1_sqrt", "n2_sqrt"))
		fnAssertAlmostDecimal(t, row[0], math.Sqrt(12.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Sqrt(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_sqrt.py::SqrtTests.test_float
func TestSqrtFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_sqrt", orm.Sqrt("f1"), "f2_sqrt", orm.Sqrt("f2")).ValuesList("f1_sqrt", "f2_sqrt"))
		fnAssertAlmostFloat(t, row[0], math.Sqrt(27.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Sqrt(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_sqrt.py::SqrtTests.test_integer
func TestSqrtInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(20)), Normal: new(int32(15)), Big: new(int64(1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_sqrt", orm.Sqrt("small"), "normal_sqrt", orm.Sqrt("normal"), "big_sqrt", orm.Sqrt("big"),
		).ValuesList("small_sqrt", "normal_sqrt", "big_sqrt"))
		fnAssertAlmostFloat(t, row[0], math.Sqrt(20), 7)
		fnAssertAlmostFloat(t, row[1], math.Sqrt(15), 7)
		fnAssertAlmostFloat(t, row[2], math.Sqrt(1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_sqrt.py::SqrtTests.test_transform
func TestSqrtTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "sqrt", orm.Sqrt))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("6.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("1.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__sqrt__gt", 2)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("6.0")) {
			t.Errorf("n1 = %s, want 6.0", obj.N1)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_tan.py::TanTests.test_null
func TestTanNull(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{})
		got := firstValue(t, orm.Query[fnIntegerModel](db).Annotate("null_tan", orm.Tan("normal")).ValuesList("null_tan"))
		if got != nil {
			t.Errorf("null_tan = %#v, want nil", got)
		}
	})
}

// Ported from Django: tests/db_functions/math/test_tan.py::TanTests.test_decimal
func TestTanDecimal(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("-12.9"), N2: new(orm.MustDecimal("0.6"))})
		row := fnFirstRow(t, orm.Query[fnDecimalModel](db).Annotate(
			"n1_tan", orm.Tan("n1"), "n2_tan", orm.Tan("n2")).ValuesList("n1_tan", "n2_tan"))
		fnAssertAlmostDecimal(t, row[0], math.Tan(-12.9), 7)
		fnAssertAlmostDecimal(t, row[1], math.Tan(0.6), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_tan.py::TanTests.test_float
func TestTanFloat(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnFloatModel{F1: new(-27.5), F2: new(0.33)})
		row := fnFirstRow(t, orm.Query[fnFloatModel](db).Annotate(
			"f1_tan", orm.Tan("f1"), "f2_tan", orm.Tan("f2")).ValuesList("f1_tan", "f2_tan"))
		fnAssertAlmostFloat(t, row[0], math.Tan(-27.5), 7)
		fnAssertAlmostFloat(t, row[1], math.Tan(0.33), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_tan.py::TanTests.test_integer
func TestTanInteger(t *testing.T) {
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnIntegerModel{Small: new(int16(-20)), Normal: new(int32(15)), Big: new(int64(-1))})
		row := fnFirstRow(t, orm.Query[fnIntegerModel](db).Annotate(
			"small_tan", orm.Tan("small"), "normal_tan", orm.Tan("normal"), "big_tan", orm.Tan("big"),
		).ValuesList("small_tan", "normal_tan", "big_tan"))
		fnAssertAlmostFloat(t, row[0], math.Tan(-20), 7)
		fnAssertAlmostFloat(t, row[1], math.Tan(15), 7)
		fnAssertAlmostFloat(t, row[2], math.Tan(-1), 7)
	})
}

// Ported from Django: tests/db_functions/math/test_tan.py::TanTests.test_transform
func TestTanTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindDecimal, "tan", orm.Tan))
	djangotest.EachDB(t, fnMathModels(), func(t *testing.T, db *orm.DB) {
		zero := orm.MustDecimal("0")
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("0.0"), N2: &zero})
		create(t, db, &fnDecimalModel{N1: orm.MustDecimal("12.0"), N2: &zero})
		obj, err := orm.Query[fnDecimalModel](db).Get(context.Background(), "n1__tan__lt", 0)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !obj.N1.Equal(orm.MustDecimal("12.0")) {
			t.Errorf("n1 = %s, want 12.0", obj.N1)
		}
	})
}
