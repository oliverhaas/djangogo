package orm_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// ewClassification is Django's expressions_window.Classification.
type ewClassification struct {
	ID   int64
	Code string `orm:"max_length=10"`
}

// ewEmployee is Django's expressions_window.Employee.
type ewEmployee struct {
	ID             int64
	Name           string `orm:"max_length=40"`
	Salary         uint32
	Department     string `orm:"max_length=40"`
	HireDate       orm.Date
	Age            int32
	Classification orm.FK[ewClassification] `orm:"null;on_delete=cascade"`
	Bonus          *orm.Decimal             `orm:"max_digits=15;decimal_places=2;null"`
}

// ewPastEmployeeDepartment is Django's
// expressions_window.PastEmployeeDepartment.
type ewPastEmployeeDepartment struct {
	ID         int64
	Employee   orm.FK[ewEmployee] `orm:"on_delete=cascade;related_name=past_departments"`
	Department string             `orm:"max_length=40"`
}

// ewModels returns the models of the ports of tests/expressions_window.
func ewModels() []any {
	return []any{djangotest.App("expressions_window",
		&ewClassification{}, &ewEmployee{}, &ewPastEmployeeDepartment{})}
}

// ewQS returns the queryset of every employee.
func ewQS(db *orm.DB) *orm.QuerySet[ewEmployee] { return orm.Query[ewEmployee](db) }

// ewName is the transform that maps an employee to its name.
func ewName(e ewEmployee) string { return e.Name }

// ewSetUp creates the rows of WindowFunctionTests.setUpTestData.
func ewSetUp(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()
	classification := create(t, db, &ewClassification{})
	d := orm.NewDate
	rows := []struct {
		name       string
		salary     uint32
		department string
		hireDate   orm.Date
		age        int32
	}{
		{"Jones", 45000, "Accounting", d(2005, 11, 1), 20},
		{"Williams", 37000, "Accounting", d(2009, 6, 1), 20},
		{"Jenson", 45000, "Accounting", d(2008, 4, 1), 20},
		{"Adams", 50000, "Accounting", d(2013, 7, 1), 50},
		{"Smith", 55000, "Sales", d(2007, 6, 1), 30},
		{"Brown", 53000, "Sales", d(2009, 9, 1), 30},
		{"Johnson", 40000, "Marketing", d(2012, 3, 1), 30},
		{"Smith", 38000, "Marketing", d(2009, 10, 1), 20},
		{"Wilkinson", 60000, "IT", d(2011, 3, 1), 40},
		{"Moore", 34000, "IT", d(2013, 8, 1), 40},
		{"Miller", 100000, "Management", d(2005, 6, 1), 40},
		{"Johnson", 80000, "Management", d(2005, 7, 1), 50},
	}
	employees := make([]*ewEmployee, len(rows))
	for i, r := range rows {
		bonus := orm.MustDecimal(fmt.Sprintf("%.2f", float64(r.salary)/400))
		employees[i] = &ewEmployee{
			Name: r.name, Salary: r.salary, Department: r.department,
			HireDate: r.hireDate, Age: r.age, Bonus: &bonus,
			Classification: fkTo[ewClassification](classification.ID),
		}
	}
	if err := ewQS(db).BulkCreate(ctx, employees); err != nil {
		t.Fatalf("bulk create employees: %v", err)
	}
	ordered := mustAll(t, ewQS(db).OrderBy("pk"))
	past := []*ewPastEmployeeDepartment{
		{Employee: fkTo[ewEmployee](ordered[6].ID), Department: "Sales"},
		{Employee: fkTo[ewEmployee](ordered[10].ID), Department: "IT"},
	}
	if err := orm.Query[ewPastEmployeeDepartment](db).BulkCreate(ctx, past); err != nil {
		t.Fatalf("bulk create past departments: %v", err)
	}
}

// ewEach runs fn on every database, with the rows of ewSetUp.
func ewEach(t *testing.T, fn func(t *testing.T, db *orm.DB)) {
	djangotest.EachDB(t, ewModels(), func(t *testing.T, db *orm.DB) {
		ewSetUp(t, db)
		fn(t, db)
	})
}

// ewNormalize maps a value of a values row to the type that the expected
// rows use, so that Python's numeric equality carries over. Every integer
// becomes an int. A Decimal or a float becomes an int when it is whole, and
// a float64 rounded to 10 places otherwise, the rounding of Django's
// CUME_DIST and PERCENT_RANK tests.
func ewNormalize(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case orm.Decimal:
		f, _ := x.Rat().Float64()
		return ewNumber(f)
	case *orm.Decimal:
		if x == nil {
			return nil
		}
		return ewNormalize(*x)
	case float64:
		return ewNumber(x)
	case *orm.Date:
		if x == nil {
			return nil
		}
		return *x
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint())
	}
	return v
}

// ewNumber returns f as an int when it is whole, and rounded to 10 places
// otherwise.
func ewNumber(f float64) any {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return int(f)
	}
	return math.Round(f*1e10) / 1e10
}

// ewRows runs a values list and returns its rows with normalized values.
func ewRows(t *testing.T, vl *orm.ValuesListQuerySet) [][]any {
	t.Helper()
	rows, err := vl.All(context.Background())
	if err != nil {
		t.Fatalf("values list: %v", err)
	}
	for _, row := range rows {
		for i, v := range row {
			row[i] = ewNormalize(v)
		}
	}
	return rows
}

// ewAssert checks the normalized rows of a values list, in order unless
// djangotest.Unordered is given.
func ewAssert(t *testing.T, vl *orm.ValuesListQuerySet, want [][]any, opts ...djangotest.QuerySetOption) {
	t.Helper()
	djangotest.AssertSequenceEqual(t, ewRows(t, vl), want, opts...)
}

// ewFlat runs a values list of one column and returns its normalized values.
func ewFlat(t *testing.T, vl *orm.ValuesListQuerySet) []any {
	t.Helper()
	values, err := vl.Flat(context.Background())
	if err != nil {
		t.Fatalf("flat values list: %v", err)
	}
	for i, v := range values {
		values[i] = ewNormalize(v)
	}
	return values
}

// ewSQL returns the SELECT statement of qs.
func ewSQL(t *testing.T, qs *orm.QuerySet[ewEmployee]) string {
	t.Helper()
	sql, _, err := orm.CompileSelectForTest(qs)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return sql
}

// ewAssertSQLContains checks that the SELECT statement of qs contains want.
func ewAssertSQLContains(t *testing.T, qs *orm.QuerySet[ewEmployee], want string) {
	t.Helper()
	if sql := ewSQL(t, qs); !strings.Contains(sql, want) {
		t.Errorf("SQL %q does not contain %q", sql, want)
	}
}

// ewAssertRunError checks that running qs fails with an error that contains
// want.
func ewAssertRunError(t *testing.T, qs *orm.QuerySet[ewEmployee], want string) {
	t.Helper()
	_, err := qs.All(context.Background())
	assertErrorContains(t, err, want)
}

// ewDate is orm.NewDate under a shorter name for the expected rows.
var ewDate = orm.NewDate

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_dense_rank
func TestWindowDenseRank(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		tests := map[string]any{
			"ExtractYear asc":     orm.ExtractYear(orm.F("hire_date")).Asc(),
			"F hire_date__year":   orm.F("hire_date__year").Asc(),
			"key hire_date__year": "hire_date__year",
		}
		for name, orderBy := range tests {
			t.Run(name, func(t *testing.T) {
				qs := ewQS(db).Annotate("rank", orm.Window(orm.DenseRank()).OrderBy(orderBy))
				ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "rank"), [][]any{
					{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 1},
					{"Miller", 100000, "Management", ewDate(2005, 6, 1), 1},
					{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 1},
					{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 2},
					{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 3},
					{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 4},
					{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 4},
					{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 4},
					{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 5},
					{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 6},
					{"Moore", 34000, "IT", ewDate(2013, 8, 1), 7},
					{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 7},
				}, djangotest.Unordered())
			})
		}
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_department_salary
func TestWindowDepartmentSalary(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("department_sum", orm.Window(orm.Sum("salary")).
			PartitionBy(orm.F("department")).OrderBy(orm.F("hire_date").Asc())).
			OrderBy("department", "department_sum")
		ewAssert(t, qs.ValuesList("name", "department", "salary", "department_sum"), [][]any{
			{"Jones", "Accounting", 45000, 45000},
			{"Jenson", "Accounting", 45000, 90000},
			{"Williams", "Accounting", 37000, 127000},
			{"Adams", "Accounting", 50000, 177000},
			{"Wilkinson", "IT", 60000, 60000},
			{"Moore", "IT", 34000, 94000},
			{"Miller", "Management", 100000, 100000},
			{"Johnson", "Management", 80000, 180000},
			{"Smith", "Marketing", 38000, 38000},
			{"Johnson", "Marketing", 40000, 78000},
			{"Smith", "Sales", 55000, 55000},
			{"Brown", "Sales", 53000, 108000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_rank
func TestWindowRank(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("rank", orm.Window(orm.Rank()).OrderBy(orm.F("hire_date__year").Asc()))
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "rank"), [][]any{
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 1},
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 1},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 1},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 4},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 5},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 6},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 6},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 6},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 9},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 10},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), 11},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 11},
		}, djangotest.Unordered())
	})
}

// ewRowNumbers is the expected result of the row number tests.
var ewRowNumbers = [][]any{
	{"Jones", "Accounting", 1},
	{"Williams", "Accounting", 2},
	{"Jenson", "Accounting", 3},
	{"Adams", "Accounting", 4},
	{"Smith", "Sales", 5},
	{"Brown", "Sales", 6},
	{"Johnson", "Marketing", 7},
	{"Smith", "Marketing", 8},
	{"Wilkinson", "IT", 9},
	{"Moore", "IT", 10},
	{"Miller", "Management", 11},
	{"Johnson", "Management", 12},
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_number
func TestWindowRowNumber(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("row_number", orm.Window(orm.RowNumber()).OrderBy(orm.F("pk").Asc())).OrderBy("pk")
		ewAssert(t, qs.ValuesList("name", "department", "row_number"), ewRowNumbers)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_number_no_ordering
func TestWindowRowNumberNoOrdering(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("row_number", orm.Window(orm.RowNumber())).OrderBy("pk")
		ewAssert(t, qs.ValuesList("name", "department", "row_number"), ewRowNumbers)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_avg_salary_department
func TestWindowAvgSalaryDepartment(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("avg_salary", orm.Window(orm.Avg("salary")).
			OrderBy(orm.F("department").Asc()).PartitionBy("department")).
			OrderBy("department", "-salary", "name")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "avg_salary"), [][]any{
			{"Adams", 50000, "Accounting", 44250},
			{"Jenson", 45000, "Accounting", 44250},
			{"Jones", 45000, "Accounting", 44250},
			{"Williams", 37000, "Accounting", 44250},
			{"Wilkinson", 60000, "IT", 47000},
			{"Moore", 34000, "IT", 47000},
			{"Miller", 100000, "Management", 90000},
			{"Johnson", 80000, "Management", 90000},
			{"Johnson", 40000, "Marketing", 39000},
			{"Smith", 38000, "Marketing", 39000},
			{"Smith", 55000, "Sales", 54000},
			{"Brown", 53000, "Sales", 54000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_lag
func TestWindowLag(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("lag", orm.Window(orm.Lag("salary", 1)).
			PartitionBy(orm.F("department")).OrderBy(orm.F("salary").Asc(), orm.F("name").Asc())).
			OrderBy("department", orm.F("salary").Asc(), orm.F("name").Asc())
		ewAssert(t, qs.ValuesList("name", "salary", "department", "lag"), [][]any{
			{"Williams", 37000, "Accounting", nil},
			{"Jenson", 45000, "Accounting", 37000},
			{"Jones", 45000, "Accounting", 45000},
			{"Adams", 50000, "Accounting", 45000},
			{"Moore", 34000, "IT", nil},
			{"Wilkinson", 60000, "IT", 34000},
			{"Johnson", 80000, "Management", nil},
			{"Miller", 100000, "Management", 80000},
			{"Smith", 38000, "Marketing", nil},
			{"Johnson", 40000, "Marketing", 38000},
			{"Brown", 53000, "Sales", nil},
			{"Smith", 55000, "Sales", 53000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_lag_decimalfield
func TestWindowLagDecimalField(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("lag", orm.Window(orm.Lag("bonus", 1)).
			PartitionBy(orm.F("department")).OrderBy(orm.F("bonus").Asc(), orm.F("name").Asc())).
			OrderBy("department", orm.F("bonus").Asc(), orm.F("name").Asc())
		ewAssert(t, qs.ValuesList("name", "bonus", "department", "lag"), [][]any{
			{"Williams", 92.5, "Accounting", nil},
			{"Jenson", 112.5, "Accounting", 92.5},
			{"Jones", 112.5, "Accounting", 112.5},
			{"Adams", 125, "Accounting", 112.5},
			{"Moore", 85, "IT", nil},
			{"Wilkinson", 150, "IT", 85},
			{"Johnson", 200, "Management", nil},
			{"Miller", 250, "Management", 200},
			{"Smith", 95, "Marketing", nil},
			{"Johnson", 100, "Marketing", 95},
			{"Brown", 132.5, "Sales", nil},
			{"Smith", 137.5, "Sales", 132.5},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_order_by_decimalfield
func TestWindowOrderByDecimalField(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("rank", orm.Window(orm.Rank()).OrderBy("bonus")).OrderBy("-bonus", "id")
		ewAssert(t, qs.ValuesList("name", "bonus", "rank"), [][]any{
			{"Miller", 250, 12},
			{"Johnson", 200, 11},
			{"Wilkinson", 150, 10},
			{"Smith", 137.5, 9},
			{"Brown", 132.5, 8},
			{"Adams", 125, 7},
			{"Jones", 112.5, 5},
			{"Jenson", 112.5, 5},
			{"Johnson", 100, 4},
			{"Smith", 95, 3},
			{"Williams", 92.5, 2},
			{"Moore", 85, 1},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_first_value
func TestWindowFirstValue(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("first_value", orm.Window(orm.FirstValue("salary")).
			PartitionBy(orm.F("department")).OrderBy(orm.F("hire_date").Asc())).
			OrderBy("department", "hire_date")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "first_value"), [][]any{
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 45000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 45000},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 45000},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 45000},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 60000},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), 60000},
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 100000},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 100000},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 38000},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 38000},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 55000},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 55000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_last_value
func TestWindowLastValue(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("last_value", orm.Window(orm.LastValue("hire_date")).
			PartitionBy(orm.F("department")).OrderBy(orm.F("hire_date").Asc()))
		ewAssert(t, qs.ValuesList("name", "department", "hire_date", "salary", "last_value"), [][]any{
			{"Adams", "Accounting", ewDate(2013, 7, 1), 50000, ewDate(2013, 7, 1)},
			{"Jenson", "Accounting", ewDate(2008, 4, 1), 45000, ewDate(2008, 4, 1)},
			{"Jones", "Accounting", ewDate(2005, 11, 1), 45000, ewDate(2005, 11, 1)},
			{"Williams", "Accounting", ewDate(2009, 6, 1), 37000, ewDate(2009, 6, 1)},
			{"Moore", "IT", ewDate(2013, 8, 1), 34000, ewDate(2013, 8, 1)},
			{"Wilkinson", "IT", ewDate(2011, 3, 1), 60000, ewDate(2011, 3, 1)},
			{"Miller", "Management", ewDate(2005, 6, 1), 100000, ewDate(2005, 6, 1)},
			{"Johnson", "Management", ewDate(2005, 7, 1), 80000, ewDate(2005, 7, 1)},
			{"Johnson", "Marketing", ewDate(2012, 3, 1), 40000, ewDate(2012, 3, 1)},
			{"Smith", "Marketing", ewDate(2009, 10, 1), 38000, ewDate(2009, 10, 1)},
			{"Brown", "Sales", ewDate(2009, 9, 1), 53000, ewDate(2009, 9, 1)},
			{"Smith", "Sales", ewDate(2007, 6, 1), 55000, ewDate(2007, 6, 1)},
		}, djangotest.Unordered())
	})
}

// ewLeads is the expected result of the Lead tests ordered by department,
// hire date and descending name.
var ewLeads = [][]any{
	{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 45000},
	{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 37000},
	{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 50000},
	{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), nil},
	{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 34000},
	{"Moore", 34000, "IT", ewDate(2013, 8, 1), nil},
	{"Miller", 100000, "Management", ewDate(2005, 6, 1), 80000},
	{"Johnson", 80000, "Management", ewDate(2005, 7, 1), nil},
	{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 40000},
	{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), nil},
	{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 53000},
	{"Brown", 53000, "Sales", ewDate(2009, 9, 1), nil},
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_function_list_of_values
func TestWindowFunctionListOfValues(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("lead", orm.Window(orm.Lead("salary", 1)).
			OrderBy(orm.F("hire_date").Asc(), orm.F("name").Desc()).PartitionBy("department")).
			OrderBy("department", orm.F("hire_date").Asc(), orm.F("name").Desc())
		if sql := ewSQL(t, qs); strings.Contains(sql, "GROUP BY") {
			t.Errorf("SQL %q contains GROUP BY", sql)
		}
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "lead"), ewLeads)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_min_department
func TestWindowMinDepartment(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("min_salary", orm.Window(orm.Min("salary")).
			PartitionBy(orm.F("department")).OrderBy(orm.F("salary").Asc(), orm.F("name").Asc())).
			OrderBy("department", "salary", "name")
		ewAssert(t, qs.ValuesList("name", "department", "salary", "min_salary"), [][]any{
			{"Williams", "Accounting", 37000, 37000},
			{"Jenson", "Accounting", 45000, 37000},
			{"Jones", "Accounting", 45000, 37000},
			{"Adams", "Accounting", 50000, 37000},
			{"Moore", "IT", 34000, 34000},
			{"Wilkinson", "IT", 60000, 34000},
			{"Johnson", "Management", 80000, 80000},
			{"Miller", "Management", 100000, 80000},
			{"Smith", "Marketing", 38000, 38000},
			{"Johnson", "Marketing", 40000, 38000},
			{"Brown", "Sales", 53000, 53000},
			{"Smith", "Sales", 55000, 53000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_max_per_year
func TestWindowMaxPerYear(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("max_salary_year", orm.Window(orm.Max("salary")).
			OrderBy(orm.ExtractYear("hire_date").Asc()).PartitionBy(orm.ExtractYear("hire_date"))).
			OrderBy(orm.ExtractYear("hire_date"), "salary")
		rows := ewRows(t, qs.ValuesList("name", "department", "salary", "hire_date", "max_salary_year"))
		for _, row := range rows {
			row[3] = row[3].(orm.Date).Year
		}
		djangotest.AssertSequenceEqual(t, rows, [][]any{
			{"Jones", "Accounting", 45000, 2005, 100000},
			{"Johnson", "Management", 80000, 2005, 100000},
			{"Miller", "Management", 100000, 2005, 100000},
			{"Smith", "Sales", 55000, 2007, 55000},
			{"Jenson", "Accounting", 45000, 2008, 45000},
			{"Williams", "Accounting", 37000, 2009, 53000},
			{"Smith", "Marketing", 38000, 2009, 53000},
			{"Brown", "Sales", 53000, 2009, 53000},
			{"Wilkinson", "IT", 60000, 2011, 60000},
			{"Johnson", "Marketing", 40000, 2012, 40000},
			{"Moore", "IT", 34000, 2013, 50000},
			{"Adams", "Accounting", 50000, 2013, 50000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_cume_dist
func TestWindowCumeDist(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("cume_dist", orm.Window(orm.CumeDist()).OrderBy(orm.F("salary").Asc())).
			OrderBy("salary", "name")
		ewAssert(t, qs.ValuesList("name", "department", "salary", "cume_dist"), [][]any{
			{"Moore", "IT", 34000, 0.0833333333},
			{"Williams", "Accounting", 37000, 0.1666666667},
			{"Smith", "Marketing", 38000, 0.25},
			{"Johnson", "Marketing", 40000, 0.3333333333},
			{"Jenson", "Accounting", 45000, 0.5},
			{"Jones", "Accounting", 45000, 0.5},
			{"Adams", "Accounting", 50000, 0.5833333333},
			{"Brown", "Sales", 53000, 0.6666666667},
			{"Smith", "Sales", 55000, 0.75},
			{"Wilkinson", "IT", 60000, 0.8333333333},
			{"Johnson", "Management", 80000, 0.9166666667},
			{"Miller", "Management", 100000, 1},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_nthvalue
func TestWindowNthValue(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("nth_value", orm.Window(orm.NthValue("salary", 2)).
			OrderBy(orm.F("hire_date").Asc(), orm.F("name").Desc()).PartitionBy(orm.F("department"))).
			OrderBy("department", "hire_date", "name")
		ewAssert(t, qs.ValuesList("name", "department", "hire_date", "salary", "nth_value"), [][]any{
			{"Jones", "Accounting", ewDate(2005, 11, 1), 45000, nil},
			{"Jenson", "Accounting", ewDate(2008, 4, 1), 45000, 45000},
			{"Williams", "Accounting", ewDate(2009, 6, 1), 37000, 45000},
			{"Adams", "Accounting", ewDate(2013, 7, 1), 50000, 45000},
			{"Wilkinson", "IT", ewDate(2011, 3, 1), 60000, nil},
			{"Moore", "IT", ewDate(2013, 8, 1), 34000, 34000},
			{"Miller", "Management", ewDate(2005, 6, 1), 100000, nil},
			{"Johnson", "Management", ewDate(2005, 7, 1), 80000, 80000},
			{"Smith", "Marketing", ewDate(2009, 10, 1), 38000, nil},
			{"Johnson", "Marketing", ewDate(2012, 3, 1), 40000, 40000},
			{"Smith", "Sales", ewDate(2007, 6, 1), 55000, nil},
			{"Brown", "Sales", ewDate(2009, 9, 1), 53000, 53000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_lead
func TestWindowLead(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("lead", orm.Window(orm.Lead("salary", 1)).
			OrderBy(orm.F("hire_date").Asc(), orm.F("name").Desc()).PartitionBy("department")).
			OrderBy("department", orm.F("hire_date").Asc(), orm.F("name").Desc())
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "lead"), ewLeads)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_lead_offset
func TestWindowLeadOffset(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("lead", orm.Window(orm.Lead("salary", 2)).
			PartitionBy("department").OrderBy(orm.F("hire_date").Asc()))
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "lead"), [][]any{
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 37000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 50000},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), nil},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), nil},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), nil},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), nil},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), nil},
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), nil},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), nil},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), nil},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), nil},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), nil},
		}, djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_lead_default
func TestWindowLeadDefault(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("lead_default", orm.Window(orm.Lead("salary", 5).Default(60000)).
			PartitionBy(orm.F("department")).OrderBy(orm.F("department").Asc()))
		djangotest.AssertSequenceEqual(t, ewFlat(t, qs.ValuesList("lead_default").Distinct()), []any{60000})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_ntile
func TestWindowNtile(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("ntile", orm.Window(orm.Ntile(4)).OrderBy("-salary")).
			OrderBy("ntile", "-salary", "name")
		ewAssert(t, qs.ValuesList("name", "department", "salary", "ntile"), [][]any{
			{"Miller", "Management", 100000, 1},
			{"Johnson", "Management", 80000, 1},
			{"Wilkinson", "IT", 60000, 1},
			{"Smith", "Sales", 55000, 2},
			{"Brown", "Sales", 53000, 2},
			{"Adams", "Accounting", 50000, 2},
			{"Jenson", "Accounting", 45000, 3},
			{"Jones", "Accounting", 45000, 3},
			{"Johnson", "Marketing", 40000, 3},
			{"Smith", "Marketing", 38000, 4},
			{"Williams", "Accounting", 37000, 4},
			{"Moore", "IT", 34000, 4},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_percent_rank
func TestWindowPercentRank(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("percent_rank", orm.Window(orm.PercentRank()).
			OrderBy(orm.F("salary").Asc(), orm.F("name").Asc())).OrderBy("percent_rank")
		ewAssert(t, qs.ValuesList("name", "department", "salary", "percent_rank"), [][]any{
			{"Moore", "IT", 34000, 0},
			{"Williams", "Accounting", 37000, 0.0909090909},
			{"Smith", "Marketing", 38000, 0.1818181818},
			{"Johnson", "Marketing", 40000, 0.2727272727},
			{"Jenson", "Accounting", 45000, 0.3636363636},
			{"Jones", "Accounting", 45000, 0.4545454545},
			{"Adams", "Accounting", 50000, 0.5454545455},
			{"Brown", "Sales", 53000, 0.6363636364},
			{"Smith", "Sales", 55000, 0.7272727273},
			{"Wilkinson", "IT", 60000, 0.8181818182},
			{"Johnson", "Management", 80000, 0.9090909091},
			{"Miller", "Management", 100000, 1},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_nth_returns_null
func TestWindowNthReturnsNull(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("nth_value", orm.Window(orm.NthValue("salary", 20)).OrderBy(orm.F("salary").Asc()))
		djangotest.AssertSequenceEqual(t, ewFlat(t, qs.ValuesList("nth_value").Distinct()), []any{nil})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_multiple_partitioning
func TestWindowMultiplePartitioning(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate(
			"max", orm.Window(orm.Max("salary")).PartitionBy(orm.F("department"), orm.F("hire_date__year")),
			"past_department_count", orm.Count("past_departments"),
		).OrderBy("department", "hire_date", "name")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "max", "past_department_count"), [][]any{
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 45000, 0},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 45000, 0},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 37000, 0},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 50000, 0},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 60000, 0},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), 34000, 0},
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 100000, 1},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 100000, 0},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 38000, 0},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 40000, 1},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 55000, 0},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 53000, 0},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_multiple_ordering
func TestWindowMultipleOrdering(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).
			PartitionBy("department").OrderBy(orm.F("hire_date").Asc(), orm.F("name").Asc())).
			OrderBy("department", "sum")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum"), [][]any{
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 45000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 90000},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 127000},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 177000},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 60000},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), 94000},
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 100000},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 180000},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 38000},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 78000},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 55000},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 108000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_empty_ordering
func TestWindowEmptyOrdering(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).PartitionBy("department").OrderBy()).
			OrderBy("department", "sum")
		djangotest.AssertEqual(t, len(mustAll(t, qs)), 12)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_related_ordering_with_count
func TestWindowRelatedOrderingWithCount(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("department_sum", orm.Window(orm.Sum("salary")).
			PartitionBy(orm.F("department")).OrderBy("classification__code"))
		djangotest.AssertEqual(t, mustCount(t, qs), int64(12))
	})
}

// ewRankedQS returns the employees annotated with Django's
// department_salary_rank and department_avg_age_diff of test_filter.
func ewRankedQS(db *orm.DB) *orm.QuerySet[ewEmployee] {
	return ewQS(db).Annotate(
		"department_salary_rank", orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary"),
		"department_avg_age_diff", orm.Window(orm.Avg("age")).PartitionBy("department").Sub(orm.F("age")),
	).OrderBy("department", "name")
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter
func TestWindowFilter(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewRankedQS(db)
		// Direct window reference.
		djangotest.AssertQuerySetEqual(t, qs.Filter("department_salary_rank", 1),
			[]string{"Adams", "Wilkinson", "Miller", "Johnson", "Smith"}, ewName)
		// Through a combined expression containing a window.
		djangotest.AssertQuerySetEqual(t, qs.Filter("department_avg_age_diff__gt", 0),
			[]string{"Jenson", "Jones", "Williams", "Miller", "Smith"}, ewName)
		// Intersection of multiple windows.
		djangotest.AssertQuerySetEqual(t, qs.Filter("department_salary_rank", 1, "department_avg_age_diff__gt", 0),
			[]string{"Miller"}, ewName)
		// Union of multiple windows.
		djangotest.AssertQuerySetEqual(t,
			qs.Filter(orm.Q("department_salary_rank", 1).Or(orm.Q("department_avg_age_diff__gt", 0))),
			[]string{"Adams", "Jenson", "Jones", "Williams", "Wilkinson", "Miller", "Johnson", "Smith", "Smith"},
			ewName)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter_conditional_annotation
// Django also annotates q_first_rank=Q(rank=1). Annotate takes an Expr, not
// a Cond, so the port has only the Case annotation.
func TestWindowFilterConditionalAnnotation(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate(
			"rank", orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary"),
			"case_first_rank", orm.Case(orm.When{Cond: orm.Q("rank", 1), Then: true}).Default(false),
		).OrderBy("name").ValuesList("name")
		djangotest.AssertSequenceEqual(t, ewFlat(t, qs.Filter("case_first_rank", true)),
			[]any{"Adams", "Johnson", "Miller", "Smith", "Wilkinson"})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter_conditional_expression
func TestWindowFilterConditionalExpression(t *testing.T) {
	t.Skip("FIXME-PORT: Go: lookup expressions such as Exact(lhs, rhs) as filter conditions, Django: test_filter_conditional_expression filters on Exact(Window(...), 1)")
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter_column_ref_rhs
func TestWindowFilterColumnRefRHS(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("max_dept_salary", orm.Window(orm.Max("salary")).PartitionBy("department")).
			Filter("max_dept_salary", orm.F("salary")).OrderBy("name").ValuesList("name")
		djangotest.AssertSequenceEqual(t, ewFlat(t, qs), []any{"Adams", "Johnson", "Miller", "Smith", "Wilkinson"})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter_values
// Django selects values_list(Upper("name")). ValuesList takes keys, so the
// port annotates the Upper first.
func TestWindowFilterValues(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate(
			"department_salary_rank", orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary"),
			"upper_name", orm.Upper("name"),
		).OrderBy("department", "name").ValuesList("upper_name")
		djangotest.AssertSequenceEqual(t, ewFlat(t, qs.Filter("department_salary_rank", 1)),
			[]any{"ADAMS", "WILKINSON", "MILLER", "JOHNSON", "SMITH"})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter_alias
func TestWindowFilterAlias(t *testing.T) {
	t.Skip("FIXME-PORT: Go: QuerySet.Alias, Django: test_filter_alias filters on an alias() of a window expression")
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter_select_related
// Django uses alias(). The port uses Annotate, as the ORM has no Alias.
func TestWindowFilterSelectRelated(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("department_avg_age_diff",
			orm.Window(orm.Avg("age")).PartitionBy("department").Sub(orm.F("age"))).
			SelectRelated("classification").Filter("department_avg_age_diff__gt", 0).
			OrderBy("department", "name")
		djangotest.AssertQuerySetEqual(t, qs, []string{"Jenson", "Jones", "Williams", "Miller", "Smith"}, ewName)
		first := mustAll(t, qs)[0]
		if _, loaded := first.Classification.Object(); !loaded {
			t.Errorf("classification of the first row is not loaded")
		}
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_exclude
func TestWindowExclude(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewRankedQS(db)
		// Direct window reference.
		djangotest.AssertQuerySetEqual(t, qs.Exclude("department_salary_rank__gt", 1),
			[]string{"Adams", "Wilkinson", "Miller", "Johnson", "Smith"}, ewName)
		// Through a combined expression containing a window.
		djangotest.AssertQuerySetEqual(t, qs.Exclude("department_avg_age_diff__lte", 0),
			[]string{"Jenson", "Jones", "Williams", "Miller", "Smith"}, ewName)
		// Union of multiple windows.
		djangotest.AssertQuerySetEqual(t,
			qs.Exclude(orm.Q("department_salary_rank__gt", 1).Or(orm.Q("department_avg_age_diff__lte", 0))),
			[]string{"Miller"}, ewName)
		// Intersection of multiple windows.
		djangotest.AssertQuerySetEqual(t, qs.Exclude("department_salary_rank__gt", 1, "department_avg_age_diff__lte", 0),
			[]string{"Adams", "Jenson", "Jones", "Williams", "Wilkinson", "Miller", "Johnson", "Smith", "Smith"},
			ewName)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_heterogeneous_filter
func TestWindowHeterogeneousFilter(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("department_salary_rank",
			orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary")).
			OrderBy("name").ValuesList("name")
		departments := []string{"Accounting", "Management"}
		// Heterogeneous filter between window function and aggregates pushes
		// the WHERE clause to the QUALIFY outer query.
		djangotest.AssertSequenceEqual(t,
			ewFlat(t, qs.Filter("department_salary_rank", 1, "department__in", departments)),
			[]any{"Adams", "Miller"})
		djangotest.AssertSequenceEqual(t,
			ewFlat(t, qs.Filter(orm.Q("department_salary_rank", 1).Or(orm.Q("department__in", departments)))),
			[]any{"Adams", "Jenson", "Johnson", "Johnson", "Jones", "Miller", "Smith", "Wilkinson", "Williams"})
		// Heterogeneous filter between window function and aggregates pushes
		// the HAVING clause to the QUALIFY outer query.
		qs = qs.Annotate("past_department_count", orm.Count("past_departments"))
		djangotest.AssertSequenceEqual(t,
			ewFlat(t, qs.Filter("department_salary_rank", 1, "past_department_count__gte", 1)),
			[]any{"Johnson", "Miller"})
		djangotest.AssertSequenceEqual(t,
			ewFlat(t, qs.Filter(orm.Q("department_salary_rank", 1).Or(orm.Q("past_department_count__gte", 1)))),
			[]any{"Adams", "Johnson", "Miller", "Smith", "Wilkinson"})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_limited_filter
func TestWindowLimitedFilter(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("department_salary_rank",
			orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary")).
			Filter("department_salary_rank", 1).OrderBy("department").Limit(3)
		djangotest.AssertQuerySetEqual(t, qs, []string{"Adams", "Wilkinson", "Miller"}, ewName)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_filter_count
func TestWindowFilterCount(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		capture := djangotest.CaptureQueries(db)
		count := mustCount(t, ewQS(db).Annotate("department_salary_rank",
			orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary")).
			Filter("department_salary_rank", 1))
		capture.Stop()
		djangotest.AssertEqual(t, count, int64(5))
		queries := capture.Queries()
		djangotest.AssertEqual(t, len(queries), 1)
		sql := strings.ToLower(queries[0].SQL)
		djangotest.AssertEqual(t, strings.Count(sql, "select"), 3)
		if strings.Contains(sql, "group by") {
			t.Errorf("SQL %q contains group by", sql)
		}
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_range_n_preceding_and_following
// Django skips the test on PostgreSQL, whose backend allows only UNBOUNDED
// in a RANGE frame. PostgreSQL supports offsets since version 11, so the
// port runs on both databases.
func TestWindowRangeNPrecedingAndFollowing(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).OrderBy(orm.F("salary").Asc()).
			PartitionBy("department").Frame(orm.ValueRange(-2, 2)))
		ewAssertSQLContains(t, qs, "RANGE BETWEEN 2 PRECEDING AND 2 FOLLOWING")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum"), [][]any{
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 37000},
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 90000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 90000},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 50000},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 53000},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 55000},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 40000},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 38000},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 60000},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), 34000},
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 100000},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 80000},
		}, djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_range_exclude_current
// The port runs on PostgreSQL too, as test_range_n_preceding_and_following
// explains. Jones and Jenson earn the same, so the port also orders by
// hire_date, which gives the order that Django's test gets from SQLite.
func TestWindowRangeExcludeCurrent(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).OrderBy(orm.F("salary").Asc()).
			PartitionBy("department").Frame(orm.ValueRange(nil, 2).Exclusion(orm.ExcludeCurrentRow))).
			OrderBy("department", "salary", "hire_date")
		ewAssertSQLContains(t, qs, "RANGE BETWEEN UNBOUNDED PRECEDING AND 2 FOLLOWING EXCLUDE CURRENT ROW")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum"), [][]any{
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), nil},
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 82000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 82000},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 127000},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), nil},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 34000},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), nil},
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 80000},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), nil},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 38000},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), nil},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 53000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_range_unbound
func TestWindowRangeUnbound(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).PartitionBy("age").
			OrderBy(orm.F("age").Asc()).Frame(orm.ValueRange(nil, nil))).
			OrderBy("department", "hire_date", "name")
		ewAssertSQLContains(t, qs, "RANGE BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING")
		ewAssert(t, qs.ValuesList("name", "department", "salary", "hire_date", "sum"), [][]any{
			{"Jones", "Accounting", 45000, ewDate(2005, 11, 1), 165000},
			{"Jenson", "Accounting", 45000, ewDate(2008, 4, 1), 165000},
			{"Williams", "Accounting", 37000, ewDate(2009, 6, 1), 165000},
			{"Adams", "Accounting", 50000, ewDate(2013, 7, 1), 130000},
			{"Wilkinson", "IT", 60000, ewDate(2011, 3, 1), 194000},
			{"Moore", "IT", 34000, ewDate(2013, 8, 1), 194000},
			{"Miller", "Management", 100000, ewDate(2005, 6, 1), 194000},
			{"Johnson", "Management", 80000, ewDate(2005, 7, 1), 130000},
			{"Smith", "Marketing", 38000, ewDate(2009, 10, 1), 165000},
			{"Johnson", "Marketing", 40000, ewDate(2012, 3, 1), 148000},
			{"Smith", "Sales", 55000, ewDate(2007, 6, 1), 148000},
			{"Brown", "Sales", 53000, ewDate(2009, 9, 1), 148000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_subquery_row_range_rank
func TestWindowSubqueryRowRangeRank(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		highest := ewQS(db).Filter("department", orm.OuterRef("department")).
			Annotate("avg_salary", orm.Window(orm.Avg("salary")).OrderBy(orm.F("hire_date").Asc()).
				Frame(orm.RowRange(-1, 1))).
			OrderBy("-avg_salary", "hire_date").Values("hire_date").Limit(1)
		qs := ewQS(db).Annotate("highest_avg_salary_date", orm.Subquery(highest)).OrderBy("department", "name")
		ewAssert(t, qs.ValuesList("name", "department", "highest_avg_salary_date"), [][]any{
			{"Adams", "Accounting", ewDate(2005, 11, 1)},
			{"Jenson", "Accounting", ewDate(2005, 11, 1)},
			{"Jones", "Accounting", ewDate(2005, 11, 1)},
			{"Williams", "Accounting", ewDate(2005, 11, 1)},
			{"Moore", "IT", ewDate(2011, 3, 1)},
			{"Wilkinson", "IT", ewDate(2011, 3, 1)},
			{"Johnson", "Management", ewDate(2005, 6, 1)},
			{"Miller", "Management", ewDate(2005, 6, 1)},
			{"Johnson", "Marketing", ewDate(2009, 10, 1)},
			{"Smith", "Marketing", ewDate(2009, 10, 1)},
			{"Brown", "Sales", ewDate(2007, 6, 1)},
			{"Smith", "Sales", ewDate(2007, 6, 1)},
		})
	})
}

// ewCohortQS returns the employees annotated with an aggregate over the rows
// of hire date and descending name next to each row, with the exclusion x.
func ewCohortQS(db *orm.DB, name string, agg orm.Expr, x orm.WindowFrameExclusion) *orm.QuerySet[ewEmployee] {
	return ewQS(db).Annotate(name, orm.Window(agg).OrderBy(orm.F("hire_date").Asc(), orm.F("name").Desc()).
		Frame(orm.RowRange(-1, 1).Exclusion(x))).OrderBy("hire_date")
}

// ewAvgCohort is the expected result of the Avg tests of the row range
// exclusions.
var ewAvgCohort = [][]any{
	{"Miller", 100000, "Management", ewDate(2005, 6, 1), 80000},
	{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 72500},
	{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 67500},
	{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 45000},
	{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 46000},
	{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 49000},
	{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 37500},
	{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 56500},
	{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 39000},
	{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 55000},
	{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 37000},
	{"Moore", 34000, "IT", ewDate(2013, 8, 1), 50000},
}

// ewSumCohort is the expected result of the Sum tests of the row range
// exclusions.
var ewSumCohort = [][]any{
	{"Miller", 100000, "Management", ewDate(2005, 6, 1), 180000},
	{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 225000},
	{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 180000},
	{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 145000},
	{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 137000},
	{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 135000},
	{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 128000},
	{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 151000},
	{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 138000},
	{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 150000},
	{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 124000},
	{"Moore", 34000, "IT", ewDate(2013, 8, 1), 84000},
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_range_rank_exclude_current_row
func TestWindowRowRangeRankExcludeCurrentRow(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewCohortQS(db, "avg_salary_cohort", orm.Avg("salary"), orm.ExcludeCurrentRow)
		ewAssertSQLContains(t, qs, "ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING EXCLUDE CURRENT ROW")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "avg_salary_cohort"), ewAvgCohort)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_range_rank_exclude_group
func TestWindowRowRangeRankExcludeGroup(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewCohortQS(db, "avg_salary_cohort", orm.Avg("salary"), orm.ExcludeGroup)
		ewAssertSQLContains(t, qs, "ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING EXCLUDE GROUP")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "avg_salary_cohort"), ewAvgCohort)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_range_rank_exclude_ties
func TestWindowRowRangeRankExcludeTies(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewCohortQS(db, "sum_salary_cohort", orm.Sum("salary"), orm.ExcludeTies)
		ewAssertSQLContains(t, qs, "ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING EXCLUDE TIES")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum_salary_cohort"), ewSumCohort)
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_range_rank_exclude_no_others
func TestWindowRowRangeRankExcludeNoOthers(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewCohortQS(db, "sum_salary_cohort", orm.Sum("salary"), orm.ExcludeNoOthers)
		ewAssertSQLContains(t, qs, "ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING EXCLUDE NO OTHERS")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum_salary_cohort"), ewSumCohort)
	})
}

// No Go counterpart: tests/expressions_window/tests.py::WindowFunctionTests.test_unsupported_frame_exclusion_raises_error
// runs on backends without frame exclusions. SQLite and PostgreSQL both
// support them.

// No Go counterpart: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_frame_exclusion_value_raises_error
// passes a string as the exclusion. Exclusion takes a WindowFrameExclusion,
// so the call does not compile.

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_range_rank
func TestWindowRowRangeRank(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc(), orm.F("name").Desc()).Frame(orm.RowRange(nil, 3))).
			OrderBy("sum", "hire_date")
		ewAssertSQLContains(t, qs, "ROWS BETWEEN UNBOUNDED PRECEDING AND 3 FOLLOWING")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum"), [][]any{
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 280000},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 325000},
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 362000},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 415000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 453000},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 513000},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 553000},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 603000},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 637000},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 637000},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 637000},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), 637000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_range_both_preceding
func TestWindowRowRangeBothPreceding(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc(), orm.F("name").Desc()).Frame(orm.RowRange(-2, -1))).
			OrderBy("hire_date")
		ewAssertSQLContains(t, qs, "ROWS BETWEEN 2 PRECEDING AND 1 PRECEDING")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum"), [][]any{
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), nil},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 100000},
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 180000},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 125000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 100000},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 100000},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 82000},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 90000},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 91000},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 98000},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 100000},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), 90000},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_row_range_both_following
func TestWindowRowRangeBothFollowing(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("sum", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc(), orm.F("name").Desc()).Frame(orm.RowRange(1, 2))).
			OrderBy("hire_date")
		ewAssertSQLContains(t, qs, "ROWS BETWEEN 1 FOLLOWING AND 2 FOLLOWING")
		ewAssert(t, qs.ValuesList("name", "salary", "department", "hire_date", "sum"), [][]any{
			{"Miller", 100000, "Management", ewDate(2005, 6, 1), 125000},
			{"Johnson", 80000, "Management", ewDate(2005, 7, 1), 100000},
			{"Jones", 45000, "Accounting", ewDate(2005, 11, 1), 100000},
			{"Smith", 55000, "Sales", ewDate(2007, 6, 1), 82000},
			{"Jenson", 45000, "Accounting", ewDate(2008, 4, 1), 90000},
			{"Williams", 37000, "Accounting", ewDate(2009, 6, 1), 91000},
			{"Brown", 53000, "Sales", ewDate(2009, 9, 1), 98000},
			{"Smith", 38000, "Marketing", ewDate(2009, 10, 1), 100000},
			{"Wilkinson", 60000, "IT", ewDate(2011, 3, 1), 90000},
			{"Johnson", 40000, "Marketing", ewDate(2012, 3, 1), 84000},
			{"Adams", 50000, "Accounting", ewDate(2013, 7, 1), 34000},
			{"Moore", 34000, "IT", ewDate(2013, 8, 1), nil},
		})
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_distinct_window_function
func TestWindowDistinctWindowFunction(t *testing.T) {
	t.Skip("FIXME-PORT: Go: Distinct on fields (DISTINCT ON), Django: test_distinct_window_function runs distinct(\"year\")")
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_fail_update
// Django's message ends with the repr of the Window. The port checks the
// part up to the field name.
func TestWindowFailUpdate(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		_, err := ewQS(db).Filter("department", "Management").Update(context.Background(),
			"salary", orm.Window(orm.Max("salary")).PartitionBy("department"))
		assertFieldError(t, err, "window expressions are not allowed in this query (salary")
	})
}

// TestWindowFilterUpdateDelete updates and deletes the rows a filter against
// a window matches. Django writes the filter into the WHERE clause of the
// UPDATE or DELETE, which the database rejects; the port selects the
// matching primary keys through the emulation of QUALIFY.
func TestWindowFilterUpdateDelete(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		top := func() *orm.QuerySet[ewEmployee] {
			return ewQS(db).Annotate("department_salary_rank",
				orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary")).
				Filter("department_salary_rank", 1)
		}
		updated, err := top().Update(ctx, "age", 99)
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		djangotest.AssertEqual(t, updated, int64(5))
		djangotest.AssertQuerySetEqual(t, ewQS(db).Filter("age", 99).OrderBy("name"),
			[]string{"Adams", "Johnson", "Miller", "Smith", "Wilkinson"}, ewName)
		if _, err := top().Delete(ctx); err != nil {
			t.Fatalf("delete: %v", err)
		}
		djangotest.AssertEqual(t, mustCount(t, ewQS(db)), int64(7))
		djangotest.AssertEqual(t, mustCount(t, ewQS(db).Filter("age", 99)), int64(0))
	})
}

// No Go counterpart: tests/expressions_window/tests.py::WindowFunctionTests.test_fail_insert
// creates a row whose field holds a Window. A struct field holds a value,
// not an expression.

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_window_expression_within_subquery
func TestWindowExpressionWithinSubquery(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		subquery := ewQS(db).Annotate("highest", orm.Window(orm.FirstValue("id")).
			PartitionBy(orm.F("department")).OrderBy(orm.F("salary").Desc())).Values("highest")
		highestSalary := ewQS(db).Filter("pk__in", subquery)
		djangotest.AssertSequenceEqual(t, ewRows(t, highestSalary.ValuesList("department", "salary")), [][]any{
			{"Accounting", 50000},
			{"Sales", 55000},
			{"Marketing", 40000},
			{"IT", 60000},
			{"Management", 100000},
		}, djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_key_transform
func TestWindowKeyTransform(t *testing.T) {
	t.Skip("FIXME-PORT: Go: JSONField key transforms, Django: test_key_transform partitions by KeyTransform(\"department\", \"value\")")
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_start_value_range
func TestWindowInvalidStartValueRange(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc()).Frame(orm.ValueRange(3, nil))),
			"start argument must be a negative integer, zero, or nil, but got '3'.")
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_end_value_range
func TestWindowInvalidEndValueRange(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc()).Frame(orm.ValueRange(nil, -3))),
			"end argument must be a positive integer, zero, or nil, but got '-3'.")
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_start_end_value_for_row_range
func TestWindowInvalidStartEndValueForRowRange(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc()).Frame(orm.RowRange(4, -3))),
			"start cannot be greater than end.")
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_type_end_value_range
func TestWindowInvalidTypeEndValueRange(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc()).Frame(orm.ValueRange(nil, "a"))),
			"end argument must be a positive integer, zero, or nil, but got 'a'.")
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_type_start_value_range
func TestWindowInvalidTypeStartValueRange(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Sum("salary")).
			Frame(orm.ValueRange("a", nil))),
			"start argument must be a negative integer, zero, or nil, but got 'a'.")
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_type_end_row_range
func TestWindowInvalidTypeEndRowRange(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Sum("salary")).
			Frame(orm.RowRange(nil, "a"))),
			"end argument must be an integer, zero, or nil, but got 'a'.")
	})
}

// No Go counterpart: tests/expressions_window/tests.py::WindowFunctionTests.test_unsupported_range_frame_start
// runs on backends that allow only UNBOUNDED in a RANGE frame. The ORM
// allows offsets on SQLite and PostgreSQL.

// No Go counterpart: tests/expressions_window/tests.py::WindowFunctionTests.test_unsupported_range_frame_end
// runs on backends that allow only UNBOUNDED in a RANGE frame, as
// test_unsupported_range_frame_start does.

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_type_start_row_range
func TestWindowInvalidTypeStartRowRange(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Sum("salary")).
			OrderBy(orm.F("hire_date").Asc()).Frame(orm.RowRange("a", nil))),
			"start argument must be an integer, zero, or nil, but got 'a'.")
	})
}

// Ported from Django: tests/expressions_window/tests.py::WindowFunctionTests.test_invalid_filter
func TestWindowInvalidFilter(t *testing.T) {
	ewEach(t, func(t *testing.T, db *orm.DB) {
		const msg = "Heterogeneous disjunctive predicates against window functions are not " +
			"implemented when performing conditional aggregation."
		qs := ewQS(db).Annotate(
			"window", orm.Window(orm.Rank()),
			"past_dept_cnt", orm.Count("past_departments"),
		)
		ewAssertRunError(t, qs.Filter(orm.Q("window", 1).Or(orm.Q("department", "Accounting"))), msg)
		ewAssertRunError(t, qs.Exclude("window", 1, "department", "Accounting"), msg)
	})
}

// No Go counterpart: tests/expressions_window/tests.py::WindowUnsupportedTests.test_unsupported_backend
// patches supports_over_clause off. SQLite and PostgreSQL both support
// window expressions.

// Ported from Django: tests/expressions_window/tests.py::WindowUnsupportedTests.test_filter_subquery
// Django raises in annotate(). The ORM reports the error when the query
// runs.
func TestWindowUnsupportedFilterSubquery(t *testing.T) {
	djangotest.EachDB(t, ewModels(), func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("department_salary_rank",
			orm.Window(orm.Rank()).PartitionBy("department").OrderBy("-salary"))
		inner := ewQS(db).Filter("age", orm.OuterRef("department_salary_rank")).Values("name").Limit(1)
		ewAssertRunError(t, qs.Annotate("employee_name", orm.Subquery(inner)),
			"Referencing outer query window expression is not supported: department_salary_rank.")
	})
}

// No Go counterpart: tests/expressions_window/tests.py::NonQueryWindowTests.test_window_repr
// checks the Python repr of Window. An Expr has no string form.

// Ported from Django: tests/expressions_window/tests.py::NonQueryWindowTests.test_window_frame_repr
// Django checks the repr, which wraps the frame clause in "<RowRange: ...>".
// The port checks the frame clause that String returns.
func TestNonQueryWindowFrameRepr(t *testing.T) {
	tests := []struct {
		frame orm.WindowFrame
		want  string
	}{
		{orm.RowRange(-1, nil), "ROWS BETWEEN 1 PRECEDING AND UNBOUNDED FOLLOWING"},
		{orm.ValueRange(nil, 1), "RANGE BETWEEN UNBOUNDED PRECEDING AND 1 FOLLOWING"},
		{orm.ValueRange(0, 0), "RANGE BETWEEN CURRENT ROW AND CURRENT ROW"},
		{orm.RowRange(0, 0), "ROWS BETWEEN CURRENT ROW AND CURRENT ROW"},
		{orm.RowRange(-2, -1), "ROWS BETWEEN 2 PRECEDING AND 1 PRECEDING"},
		{orm.RowRange(1, 2), "ROWS BETWEEN 1 FOLLOWING AND 2 FOLLOWING"},
		{orm.RowRange(1, 2).Exclusion(orm.ExcludeCurrentRow), "ROWS BETWEEN 1 FOLLOWING AND 2 FOLLOWING EXCLUDE CURRENT ROW"},
	}
	for _, tt := range tests {
		djangotest.AssertEqual(t, tt.frame.String(), tt.want)
	}
}

// Ported from Django: tests/expressions_window/tests.py::NonQueryWindowTests.test_window_frame_exclusion_repr
// Django checks the repr "WindowFrameExclusion.TIES". The String of a
// WindowFrameExclusion is its SQL text.
func TestNonQueryWindowFrameExclusionRepr(t *testing.T) {
	djangotest.AssertEqual(t, orm.ExcludeTies.String(), "TIES")
}

// Ported from Django: tests/expressions_window/tests.py::NonQueryWindowTests.test_empty_group_by_cols
// Django checks that a Window adds no GROUP BY columns and does not contain
// an aggregate. The port checks that the query has no GROUP BY.
func TestNonQueryWindowEmptyGroupByCols(t *testing.T) {
	djangotest.EachDB(t, ewModels(), func(t *testing.T, db *orm.DB) {
		qs := ewQS(db).Annotate("total", orm.Window(orm.Sum("pk")))
		if sql := ewSQL(t, qs); strings.Contains(sql, "GROUP BY") {
			t.Errorf("SQL %q contains GROUP BY", sql)
		}
	})
}

// No Go counterpart: tests/expressions_window/tests.py::NonQueryWindowTests.test_frame_empty_group_by_cols
// checks get_group_by_cols of a bare WindowFrame. A WindowFrame is not an
// expression.

// No Go counterpart: tests/expressions_window/tests.py::NonQueryWindowTests.test_frame_window_frame_notimplemented
// checks the abstract method of the WindowFrame base class. RowRange and
// ValueRange are the only frames.

// Ported from Django: tests/expressions_window/tests.py::NonQueryWindowTests.test_invalid_order_by
// Django raises in the Window constructor. The ORM reports the error when
// the query runs.
func TestNonQueryWindowInvalidOrderBy(t *testing.T) {
	djangotest.EachDB(t, ewModels(), func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test",
			orm.Window(orm.Sum("salary")).OrderBy(map[string]struct{}{"-horse": {}})),
			"Window.OrderBy must be either a string reference to a field or an expression, not map[-horse:{}].")
	})
}

// Ported from Django: tests/expressions_window/tests.py::NonQueryWindowTests.test_invalid_source_expression
func TestNonQueryWindowInvalidSourceExpression(t *testing.T) {
	djangotest.EachDB(t, ewModels(), func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(orm.Upper("name"))),
			"Expression 'Upper' isn't compatible with OVER clauses.")
	})
}

// ewAssertWindowError checks that annotating employees with Window(source)
// fails with an error that contains want.
func ewAssertWindowError(t *testing.T, source orm.Expr, want string) {
	t.Helper()
	djangotest.EachDB(t, ewModels(), func(t *testing.T, db *orm.DB) {
		ewAssertRunError(t, ewQS(db).Annotate("test", orm.Window(source)), want)
	})
}

// Ported from Django: tests/db_functions/window/test_validation.py::ValidationTests.test_nth_negative_nth_value
func TestWindowValidationNthNegativeNthValue(t *testing.T) {
	ewAssertWindowError(t, orm.NthValue("salary", -1), "NthValue requires a positive integer as for nth")
}

// Ported from Django: tests/db_functions/window/test_validation.py::ValidationTests.test_nth_null_expression
func TestWindowValidationNthNullExpression(t *testing.T) {
	ewAssertWindowError(t, orm.NthValue(nil, 1), "NthValue requires a non-null source expression")
}

// Ported from Django: tests/db_functions/window/test_validation.py::ValidationTests.test_lag_negative_offset
func TestWindowValidationLagNegativeOffset(t *testing.T) {
	ewAssertWindowError(t, orm.Lag("salary", -1), "Lag requires a positive integer for the offset")
}

// Ported from Django: tests/db_functions/window/test_validation.py::ValidationTests.test_lead_negative_offset
func TestWindowValidationLeadNegativeOffset(t *testing.T) {
	ewAssertWindowError(t, orm.Lead("salary", -1), "Lead requires a positive integer for the offset")
}

// Ported from Django: tests/db_functions/window/test_validation.py::ValidationTests.test_null_source_lead
func TestWindowValidationNullSourceLead(t *testing.T) {
	ewAssertWindowError(t, orm.Lead(nil, 1), "Lead requires a non-null source expression")
}

// Ported from Django: tests/db_functions/window/test_validation.py::ValidationTests.test_null_source_lag
func TestWindowValidationNullSourceLag(t *testing.T) {
	ewAssertWindowError(t, orm.Lag(nil, 1), "Lag requires a non-null source expression")
}

// Ported from Django: tests/db_functions/window/test_validation.py::ValidationTests.test_negative_num_buckets_ntile
func TestWindowValidationNegativeNumBucketsNtile(t *testing.T) {
	ewAssertWindowError(t, orm.Ntile(-1), "num_buckets must be greater than 0")
}
