package orm_test

import (
	"context"
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
