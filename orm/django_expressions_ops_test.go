package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// expSetUpNumeric creates the ExpressionsNumericTests fixture: three numbers.
// Django also fills the float field from the integer, which has no kind here.
func expSetUpNumeric(t *testing.T, db *orm.DB) {
	t.Helper()
	for _, n := range []int64{-1, 42, 1337} {
		create(t, db, &expNumber{Integer: n})
	}
}

// expIntegers returns the integers of all numbers, ordered by primary key.
func expIntegers(t *testing.T, db *orm.DB) []int64 {
	t.Helper()
	numbers := mustAll(t, orm.Query[expNumber](db).OrderBy("pk"))
	got := make([]int64, 0, len(numbers))
	for _, n := range numbers {
		got = append(got, n.Integer)
	}
	return got
}

// expNumberInteger returns the integer of the number with the given primary key.
func expNumberInteger(t *testing.T, db *orm.DB, pk int64) int64 {
	t.Helper()
	n, err := orm.Query[expNumber](db).Get(context.Background(), "pk", pk)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return n.Integer
}

// Django also compares the float, which has no kind here, so the port compares
// the integers only. The filling of float from integer is not ported.
//
// Ported from Django: tests/expressions/tests.py::ExpressionsNumericTests.test_fill_with_value_from_same_object
func TestExpressionsFillWithValueFromSameObject(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpNumeric(t, db)
		djangotest.AssertEqual(t, expIntegers(t, db), []int64{-1, 42, 1337})
	})
}

// Django also compares the float, which has no kind here, so the port compares
// the integers only.
//
// Ported from Django: tests/expressions/tests.py::ExpressionsNumericTests.test_increment_value
func TestExpressionsIncrementValue(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpNumeric(t, db)
		expUpdate(t, orm.Query[expNumber](db).Filter("integer__gt", 0), 2,
			"integer", orm.F("integer").Add(1))
		djangotest.AssertEqual(t, expIntegers(t, db), []int64{-1, 43, 1338})
	})
}

// Django excludes the rows whose float equals their integer, after an
// increment of the integer. Number has no float here, so the port keeps the
// same shape on the two integer columns of Company: num_chairs holds the old
// value and num_employees the incremented one.
//
// Ported from Django: tests/expressions/tests.py::ExpressionsNumericTests.test_filter_not_equals_other_field
func TestExpressionsFilterNotEqualsOtherField(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		ceo := create(t, db, &expEmployee{Firstname: "Joe", Lastname: "Smith"})
		for _, n := range []int64{1, 42, 1337} {
			create(t, db, &expCompany{
				Name: "n", NumEmployees: n, NumChairs: n, CEO: fkTo[expEmployee](ceo.ID),
			})
		}
		expUpdate(t, orm.Query[expCompany](db).Filter("num_employees__gt", 1), 2,
			"num_employees", orm.F("num_employees").Add(1))
		rows, err := orm.Query[expCompany](db).Exclude("num_chairs", orm.F("num_employees")).
			Values("num_employees", "num_chairs").OrderBy("pk").All(context.Background())
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		djangotest.AssertEqual(t, len(rows), 2)
		djangotest.AssertEqual(t, rows[0]["num_employees"], any(int64(43)))
		djangotest.AssertEqual(t, rows[0]["num_chairs"], any(int64(42)))
		djangotest.AssertEqual(t, rows[1]["num_employees"], any(int64(1338)))
		djangotest.AssertEqual(t, rows[1]["num_chairs"], any(int64(1337)))
	})
}

// Django updates the float with F("integer") + F("float") * 2 and checks that
// the integer is untouched and the float is 256.9. Number has no float here, so
// the port assigns the complex expression F("integer") + F("integer") * 2 to the
// integer itself and checks the result, 30.
//
// Ported from Django: tests/expressions/tests.py::ExpressionsNumericTests.test_complex_expressions
func TestExpressionsComplexExpressions(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpNumeric(t, db)
		n := create(t, db, &expNumber{Integer: 10})
		expUpdate(t, orm.Query[expNumber](db).Filter("pk", n.ID), 1,
			"integer", orm.F("integer").Add(orm.F("integer").Mul(2)))
		djangotest.AssertEqual(t, expNumberInteger(t, db, n.ID), 30)
	})
}

// expOps is the fixture of Django's ExpressionOperatorTests: n holds 42 and n1
// holds -42. The float values 15.5 and -15.5 have no kind here.
type expOps struct {
	n, n1 *expNumber
}

// expSetUpOps creates the ExpressionOperatorTests fixture.
func expSetUpOps(t *testing.T, db *orm.DB) expOps {
	t.Helper()
	return expOps{
		n:  create(t, db, &expNumber{Integer: 42}),
		n1: create(t, db, &expNumber{Integer: -42}),
	}
}

// expCreateNullSalary creates an employee whose salary is NULL. Create stores
// 0 for an unset nullable int, so an Update sets the NULL.
func expCreateNullSalary(t *testing.T, db *orm.DB, first, last string) *expEmployee {
	t.Helper()
	e := create(t, db, &expEmployee{Firstname: first, Lastname: last})
	expUpdate(t, orm.Query[expEmployee](db).Filter("pk", e.ID), 1, "salary", orm.Value(nil))
	return e
}

// expUpdateInteger assigns an expression to the integer of one number.
func expUpdateInteger(t *testing.T, db *orm.DB, pk int64, value any) {
	t.Helper()
	expUpdate(t, orm.Query[expNumber](db).Filter("pk", pk), 1, "integer", value)
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_addition
func TestExpressionsLefthandAddition(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.F("integer").Add(15))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 57)
	})
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_subtraction
func TestExpressionsLefthandSubtraction(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.F("integer").Sub(15))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 27)
	})
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_multiplication
func TestExpressionsLefthandMultiplication(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.F("integer").Mul(15))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 630)
	})
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_division
func TestExpressionsLefthandDivision(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.F("integer").Div(2))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 21)
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_modulo
func TestExpressionsLefthandModulo(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.F("integer").Mod(20))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 2)
	})
}

// Django checks employee.salary of the single row. A NULL salary reads back as
// 0 in a struct, so the port reads the row with Values, where NULL is nil.
// Create writes 0 for an unset nullable int, so the salary is set to NULL
// with an Update first.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_modulo_null
func TestExpressionsLefthandModuloNull(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpOps(t, db)
		expCreateNullSalary(t, db, "John", "Doe")
		qs := orm.Query[expEmployee](db).Annotate("modsalary", orm.F("salary").Mod(20))
		assertValues(t, qs.Values("salary", "modsalary"),
			[]map[string]any{{"salary": nil, "modsalary": nil}})
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_and
func TestExpressionsLefthandBitwiseAnd(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.F("integer").BitAnd(56))
		expUpdateInteger(t, db, fx.n1.ID, orm.F("integer").BitAnd(-56))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 40)
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n1.ID), -64)
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_left_shift_operator
func TestExpressionsLefthandBitwiseLeftShiftOperator(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdate(t, orm.Query[expNumber](db), 2, "integer", orm.F("integer").BitLeftShift(2))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 168)
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n1.ID), -168)
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_right_shift_operator
func TestExpressionsLefthandBitwiseRightShiftOperator(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdate(t, orm.Query[expNumber](db), 2, "integer", orm.F("integer").BitRightShift(2))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 10)
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n1.ID), -11)
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_or
func TestExpressionsLefthandBitwiseOr(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdate(t, orm.Query[expNumber](db), 2, "integer", orm.F("integer").BitOr(48))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 58)
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n1.ID), -10)
	})
}

// Django registers the Length transform on CharField and annotates
// F("lastname__length").bitor(48). The ORM has the date and time transforms
// only.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_transformed_field_bitwise_or
func TestExpressionsLefthandTransformedFieldBitwiseOr(t *testing.T) {
	t.Skip("needs the Length transform; the ORM has the date and time transforms only")
}

// Django also raises the float to the power 1.5, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_power
func TestExpressionsLefthandPower(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.F("integer").Pow(2))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 1764)
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_xor
func TestExpressionsLefthandBitwiseXor(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdate(t, orm.Query[expNumber](db), 2, "integer", orm.F("integer").BitXor(48))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 26)
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n1.ID), -26)
	})
}

// Django checks employee.salary after refresh_from_db(). A NULL salary reads
// back as 0 in a struct, so the port reads it with Values, where NULL is nil.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_xor_null
func TestExpressionsLefthandBitwiseXorNull(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpOps(t, db)
		expCreateNullSalary(t, db, "John", "Doe")
		expUpdate(t, orm.Query[expEmployee](db), 1, "salary", orm.F("salary").BitXor(48))
		assertValues(t, orm.Query[expEmployee](db).Values("salary"),
			[]map[string]any{{"salary": nil}})
	})
}

// Django checks employee.salary after refresh_from_db(). A NULL salary reads
// back as 0 in a struct, so the port reads it with Values, where NULL is nil.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_xor_right_null
func TestExpressionsLefthandBitwiseXorRightNull(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpOps(t, db)
		create(t, db, &expEmployee{Firstname: "John", Lastname: "Doe", Salary: 48})
		expUpdate(t, orm.Query[expEmployee](db), 1, "salary", orm.F("salary").BitXor(nil))
		assertValues(t, orm.Query[expEmployee](db).Values("salary"),
			[]map[string]any{{"salary": nil}})
	})
}

// Django runs this on Oracle only, which raises NotSupportedError "Bitwise XOR
// is not supported in Oracle.". The ORM has no Oracle dialect, and BitXor runs
// on SQLite and PostgreSQL.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_lefthand_bitwise_xor_not_supported
func TestExpressionsLefthandBitwiseXorNotSupported(t *testing.T) {
	t.Skip("Django runs this on Oracle only; the ORM has no Oracle dialect")
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_right_hand_addition
func TestExpressionsRightHandAddition(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.Value(15).Add(orm.F("integer")))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 57)
	})
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_right_hand_subtraction
func TestExpressionsRightHandSubtraction(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.Value(15).Sub(orm.F("integer")))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), -27)
	})
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_right_hand_multiplication
func TestExpressionsRightHandMultiplication(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.Value(15).Mul(orm.F("integer")))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 630)
	})
}

// Django also updates and checks the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_right_hand_division
func TestExpressionsRightHandDivision(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.Value(640).Div(orm.F("integer")))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 15)
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_right_hand_modulo
func TestExpressionsRightHandModulo(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.Value(69).Mod(orm.F("integer")))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 27)
	})
}

// Django also raises 1.5 to the power of the float, which has no kind here.
//
// Ported from Django: tests/expressions/tests.py::ExpressionOperatorTests.test_righthand_power
func TestExpressionsRighthandPower(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpOps(t, db)
		expUpdateInteger(t, db, fx.n.ID, orm.Value(2).Pow(orm.F("integer")))
		djangotest.AssertEqual(t, expNumberInteger(t, db, fx.n.ID), 4398046511104)
	})
}
