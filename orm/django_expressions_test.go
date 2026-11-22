package orm_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// expEmployee is Django's expressions.Employee without its manager foreign
// key: Manager and Employee point at each other, which PostgreSQL cannot create
// without deferred constraints, and no port reads the manager. A nullable int
// is a plain int64 here: a NULL salary reads back as 0 in the struct and as nil
// in a Values row, and Create stores 0 for an unset salary.
type expEmployee struct {
	ID        int64
	Firstname string `orm:"max_length=50"`
	Lastname  string `orm:"max_length=50"`
	Salary    int64  `orm:"null;blank"`
	BasedInEU bool   `orm:"default=false"`
}

// expCompany is Django's expressions.Company.
type expCompany struct {
	ID             int64
	Name           string `orm:"max_length=100"`
	NumEmployees   int64
	NumChairs      int64
	CEO            orm.FK[expEmployee] `orm:"on_delete=cascade;related_name=company_ceo_set"`
	PointOfContact orm.FK[expEmployee] `orm:"null;on_delete=set_null;related_name=company_point_of_contact_set"`
	BasedInEU      bool                `orm:"default=false"`
}

// expNumber is Django's expressions.Number without its float and
// decimal_value fields, which have no kind here.
type expNumber struct {
	ID      int64
	Integer int64 `orm:"column=the_integer"`
}

// expExperiment is Django's expressions.Experiment.
type expExperiment struct {
	ID            int64
	Name          string `orm:"max_length=24"`
	Assigned      orm.Date
	Completed     orm.Date
	EstimatedTime time.Duration
	Start         time.Time
	End           time.Time
	Scalar        int64 `orm:"null"`
}

// Meta gives the table Django's mixed-case name and orders by name.
func (expExperiment) Meta() orm.Meta {
	return orm.Meta{Table: "expressions_ExPeRiMeNt", Ordering: []string{"name"}}
}

// expResult is Django's expressions.Result.
type expResult struct {
	ID         int64
	Experiment orm.FK[expExperiment]
	ResultTime time.Time
}

// expTime is Django's expressions.Time.
type expTime struct {
	ID   int64
	Time *orm.Time `orm:"null"`
}

// expSimulationRun is Django's expressions.SimulationRun.
type expSimulationRun struct {
	ID       int64
	Start    orm.FK[expTime] `orm:"null"`
	End      orm.FK[expTime] `orm:"null"`
	Midpoint orm.Time
}

// expModels returns the models of Django's expressions test app that the
// ports use.
func expModels() []any {
	return []any{djangotest.App("expressions",
		&expEmployee{}, &expCompany{}, &expNumber{},
		&expExperiment{}, &expResult{}, &expTime{}, &expSimulationRun{})}
}

// expDialectIsSQLite reports whether the database is SQLite.
func expDialectIsSQLite(db *orm.DB) bool { return db.Dialect().Name() == "sqlite" }

// expUpdate runs a queryset update and checks the row count.
func expUpdate[T any](t *testing.T, qs *orm.QuerySet[T], want int64, assignments ...any) {
	t.Helper()
	n, err := qs.Update(context.Background(), assignments...)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if n != want {
		t.Fatalf("Update affected %d rows, want %d", n, want)
	}
}

// expCompanyName is the transform that compares companies by name.
func expCompanyName(c expCompany) string { return c.Name }

// expEmployeeID is the transform that compares employees by primary key.
func expEmployeeID(e expEmployee) int64 { return e.ID }

// expCompanyRow is a row of the company_query of BasicExpressionsTests.
func expCompanyRow(chairs int64, name string, employees int64) map[string]any {
	return map[string]any{"num_chairs": chairs, "name": name, "num_employees": employees}
}

// expBasic is the fixture of Django's BasicExpressionsTests.
type expBasic struct {
	exampleInc, foobarLtd, gmbh *expCompany
	max                         *expEmployee
}

// expSetUpBasic creates the BasicExpressionsTests fixture.
func expSetUpBasic(t *testing.T, db *orm.DB) expBasic {
	t.Helper()
	joe := create(t, db, &expEmployee{Firstname: "Joe", Lastname: "Smith", Salary: 10})
	frank := create(t, db, &expEmployee{Firstname: "Frank", Lastname: "Meyer", Salary: 20})
	fx := expBasic{
		exampleInc: create(t, db, &expCompany{
			Name: "Example Inc.", NumEmployees: 2300, NumChairs: 5, CEO: fkTo[expEmployee](joe.ID),
		}),
		foobarLtd: create(t, db, &expCompany{
			Name: "Foobar Ltd.", NumEmployees: 3, NumChairs: 4, BasedInEU: true, CEO: fkTo[expEmployee](frank.ID),
		}),
		max: create(t, db, &expEmployee{Firstname: "Max", Lastname: "Mustermann", Salary: 30}),
	}
	fx.gmbh = create(t, db, &expCompany{
		Name: "Test GmbH", NumEmployees: 32, NumChairs: 1, CEO: fkTo[expEmployee](fx.max.ID),
	})
	return fx
}

// expCompanyQuery is the company_query of BasicExpressionsTests.setUp.
func expCompanyQuery(db *orm.DB) *orm.ValuesQuerySet {
	return orm.Query[expCompany](db).Values("name", "num_employees", "num_chairs").
		OrderBy("name", "num_employees", "num_chairs")
}

// expCountJoins counts the JOINs of the compiled SELECT, the Go form of
// str(qs.query).count("JOIN").
func expCountJoins[T any](t *testing.T, qs *orm.QuerySet[T]) int {
	t.Helper()
	query, _, err := orm.CompileSelectForTest(qs)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return strings.Count(strings.ToUpper(query), "JOIN")
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotate_values_aggregate
func TestExpressionsAnnotateValuesAggregate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		companies, err := orm.Query[expCompany](db).
			Annotate("salaries", orm.F("ceo__salary")).
			Values("num_employees", "salaries").
			Aggregate(context.Background(), "result", orm.Sum(orm.F("salaries").Add(orm.F("num_employees"))))
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, companies["result"], any(int64(2395)))
	})
}

// Django annotates with RawSQL("%s", ["value"]). RawSQL is out of scope, so the
// port annotates a Value, which is also a bound parameter.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotate_values_filter
func TestExpressionsAnnotateValuesFilter(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		companies := orm.Query[expCompany](db).
			Annotate("foo", orm.Value("value")).
			Filter("foo", "value").
			OrderBy("name")
		djangotest.AssertQuerySetEqual(t, companies,
			[]string{"Example Inc.", "Foobar Ltd.", "Test GmbH"}, expCompanyName)
	})
}

// Django annotates with RawSQL("%s", ["value"]). The port annotates a Value.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotate_values_count
func TestExpressionsAnnotateValuesCount(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		djangotest.AssertEqual(t,
			mustCount(t, orm.Query[expCompany](db).Annotate("foo", orm.Value("value"))), 3)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_filter_inter_attribute
func TestExpressionsFilterInterAttribute(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		assertValues(t, expCompanyQuery(db).Filter("num_employees__gt", orm.F("num_chairs")), []map[string]any{
			expCompanyRow(5, "Example Inc.", 2300),
			expCompanyRow(1, "Test GmbH", 32),
		})
	})
}

// Django updates the values() queryset. Update lives on the model queryset
// here, which carries the same filters.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_update
func TestExpressionsUpdate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		expUpdate(t, orm.Query[expCompany](db), 3, "num_chairs", orm.F("num_employees"))
		assertValues(t, expCompanyQuery(db), []map[string]any{
			expCompanyRow(2300, "Example Inc.", 2300),
			expCompanyRow(3, "Foobar Ltd.", 3),
			expCompanyRow(32, "Test GmbH", 32),
		})
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_arithmetic
func TestExpressionsArithmetic(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		expUpdate(t, orm.Query[expCompany](db), 3, "num_chairs", orm.F("num_employees").Add(2))
		assertValues(t, expCompanyQuery(db), []map[string]any{
			expCompanyRow(2302, "Example Inc.", 2300),
			expCompanyRow(5, "Foobar Ltd.", 3),
			expCompanyRow(34, "Test GmbH", 32),
		})
	})
}

// Python's precedence is spelled out as the expression tree:
// F("num_employees") + 2 * F("num_employees").
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_order_of_operations
func TestExpressionsOrderOfOperations(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		expUpdate(t, orm.Query[expCompany](db), 3, "num_chairs",
			orm.F("num_employees").Add(orm.Value(2).Mul(orm.F("num_employees"))))
		assertValues(t, expCompanyQuery(db), []map[string]any{
			expCompanyRow(6900, "Example Inc.", 2300),
			expCompanyRow(9, "Foobar Ltd.", 3),
			expCompanyRow(96, "Test GmbH", 32),
		})
	})
}

// The parentheses of (F("num_employees") + 2) * F("num_employees") are the
// nesting of the expression tree.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_parenthesis_priority
func TestExpressionsParenthesisPriority(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		expUpdate(t, orm.Query[expCompany](db), 3, "num_chairs",
			orm.F("num_employees").Add(2).Mul(orm.F("num_employees")))
		assertValues(t, expCompanyQuery(db), []map[string]any{
			expCompanyRow(5294600, "Example Inc.", 2300),
			expCompanyRow(15, "Foobar Ltd.", 3),
			expCompanyRow(1088, "Test GmbH", 32),
		})
	})
}

// Django compares str(c.point_of_contact). The port compares the first and last
// name of the employee that the foreign key points at.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_update_with_fk
func TestExpressionsUpdateWithFK(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		expUpdate(t, orm.Query[expCompany](db), 3, "point_of_contact", orm.F("ceo"))
		rows, err := orm.Query[expCompany](db).
			Values("point_of_contact__firstname", "point_of_contact__lastname").All(context.Background())
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		got := make([]string, 0, len(rows))
		for _, row := range rows {
			got = append(got, row["point_of_contact__firstname"].(string)+" "+row["point_of_contact__lastname"].(string))
		}
		djangotest.AssertSequenceEqual(t, got,
			[]string{"Joe Smith", "Frank Meyer", "Max Mustermann"}, djangotest.Unordered())
	})
}

// Django sets Number.float to NULL. Number has no float here, so the port
// updates Employee.salary, a nullable integer, with Value(nil).
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_update_with_none
func TestExpressionsUpdateWithNone(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &expEmployee{Firstname: "A", Lastname: "A", Salary: 1})
		expCreateNullSalary(t, db, "B", "B")
		expUpdate(t, orm.Query[expEmployee](db).Filter("salary__isnull", false), 1,
			"salary", orm.Value(nil))
		assertFlat(t, orm.Query[expEmployee](db).ValuesList("salary").OrderBy("pk"), []any{nil, nil})
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_filter_with_join
func TestExpressionsFilterWithJoin(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		ctx := context.Background()
		expUpdate(t, orm.Query[expCompany](db), 3, "point_of_contact", orm.F("ceo"))
		c, err := orm.Query[expCompany](db).OrderBy("pk").First(ctx)
		if err != nil {
			t.Fatalf("First: %v", err)
		}
		guido := create(t, db, &expEmployee{Firstname: "Guido", Lastname: "van Rossum"})
		c.PointOfContact = fkTo[expEmployee](guido.ID)
		if err := orm.Query[expCompany](db).Save(ctx, &c); err != nil {
			t.Fatalf("Save: %v", err)
		}

		djangotest.AssertQuerySetEqual(t,
			orm.Query[expCompany](db).Filter("ceo__firstname", orm.F("point_of_contact__firstname")),
			[]string{"Foobar Ltd.", "Test GmbH"}, expCompanyName, djangotest.Unordered())

		differs := orm.Query[expCompany](db).Exclude("ceo__firstname", orm.F("point_of_contact__firstname"))
		expUpdate(t, differs, 1, "name", "foo")
		got, err := differs.Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.Name, "foo")

		_, err = differs.Update(ctx, "name", orm.F("point_of_contact__lastname"))
		assertFieldError(t, err, "oined field references are not permitted in this query")
	})
}

// Django assigns an F expression to a model attribute and saves. A Go struct
// field cannot hold an expression, so the update is a queryset Update on the
// primary key, and the new value is read back with a query. Django's refresh
// costs no query where the database returns rows from an update; the Go read
// is one query.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_object_update
func TestExpressionsObjectUpdate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		expUpdate(t, orm.Query[expCompany](db).Filter("pk", fx.gmbh.ID), 1,
			"num_employees", orm.F("num_employees").Add(4))
		got, err := orm.Query[expCompany](db).Get(context.Background(), "pk", fx.gmbh.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.NumEmployees, 36)
	})
}

// Django inserts name=Lower(Value("UPPER")). Func and Lower are out of scope
// and a struct field cannot hold an expression, so the port saves the lower
// cased Go string and checks it through a re-read.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_new_object_save
func TestExpressionsNewObjectSave(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		ctx := context.Background()
		testCo := expCompany{
			Name: strings.ToLower("UPPER"), NumEmployees: 32, NumChairs: 1, CEO: fkTo[expEmployee](fx.max.ID),
		}
		if err := orm.Query[expCompany](db).Save(ctx, &testCo); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := orm.Query[expCompany](db).Get(ctx, "pk", testCo.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.Name, "upper")
	})
}

// Django inserts name=Lower(Value("UPPER")). The port creates the object with
// the lower cased Go string, for the reasons of test_new_object_save.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_new_object_create
func TestExpressionsNewObjectCreate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		testCo := create(t, db, &expCompany{
			Name: strings.ToLower("UPPER"), NumEmployees: 32, NumChairs: 1, CEO: fkTo[expEmployee](fx.max.ID),
		})
		got, err := orm.Query[expCompany](db).Get(context.Background(), "pk", testCo.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.Name, "upper")
	})
}

// Django creates a company with num_employees=Max(Value(1)). A struct field
// cannot hold an aggregate, so the port assigns the aggregate in an Update,
// which rejects it the same way.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_object_create_with_aggregate
func TestExpressionsObjectCreateWithAggregate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		_, err := orm.Query[expCompany](db).Filter("pk", fx.gmbh.ID).
			Update(context.Background(), "num_employees", orm.Max(orm.Value(1)))
		assertErrorContains(t, err, "ggregate functions are not allowed in this query")
	})
}

// Django cannot assign F("ceo") to a foreign key attribute (a ValueError), and
// the typed FK of Go cannot either, so that part has no port. The queryset
// update of point_of_contact is checked in test_update_with_fk. The
// name=F("ceo__lastname") assignment is an Update on the primary key.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_object_update_fk
func TestExpressionsObjectUpdateFK(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		ctx := context.Background()
		testGmbh, err := orm.Query[expCompany](db).Get(ctx, "pk", fx.gmbh.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		testGmbh.PointOfContact = fkTo[expEmployee](fx.max.ID)
		if err := orm.Query[expCompany](db).Save(ctx, &testGmbh); err != nil {
			t.Fatalf("Save: %v", err)
		}
		_, err = orm.Query[expCompany](db).Filter("pk", testGmbh.ID).Update(ctx, "name", orm.F("ceo__lastname"))
		assertFieldError(t, err, "oined field references are not permitted in this query")
	})
}

// Django raises a ValueError when an unsaved object holds an F expression and
// is saved: F() can only update, not insert. A struct field cannot hold an
// expression, so no Go code can insert one. The port asserts what remains:
// an Update with an F expression on the primary key of an unsaved object
// (primary key 0) touches no row and inserts none.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_object_update_unsaved_objects
func TestExpressionsObjectUpdateUnsavedObjects(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		acme := expCompany{
			Name: "The Acme Widget Co.", NumEmployees: 12, NumChairs: 5, CEO: fkTo[expEmployee](fx.max.ID),
		}
		expUpdate(t, orm.Query[expCompany](db).Filter("pk", acme.ID), 0,
			"num_employees", orm.F("num_employees").Add(16))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[expCompany](db)), 3)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_ticket_11722_iexact_lookup
func TestExpressionsTicket11722IexactLookup(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &expEmployee{Firstname: "John", Lastname: "Doe"})
		test := create(t, db, &expEmployee{Firstname: "Test", Lastname: "test"})
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("firstname__iexact", orm.F("lastname")),
			[]int64{test.ID}, expEmployeeID)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_ticket_16731_startswith_lookup
func TestExpressionsTicket16731StartswithLookup(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &expEmployee{Firstname: "John", Lastname: "Doe"})
		e2 := create(t, db, &expEmployee{Firstname: "Jack", Lastname: "Jackson"})
		e3 := create(t, db, &expEmployee{Firstname: "Jack", Lastname: "jackson"})
		// SQLite's LIKE ignores case, PostgreSQL's does not.
		want := []int64{e2.ID}
		if expDialectIsSQLite(db) {
			want = []int64{e2.ID, e3.ID}
		}
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("lastname__startswith", orm.F("firstname")),
			want, expEmployeeID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("lastname__istartswith", orm.F("firstname")).OrderBy("pk"),
			[]int64{e2.ID, e3.ID}, expEmployeeID)
	})
}

// Django follows the reverse relation company_ceo_set from Employee, which
// does not exist here (no reverse relations). The port follows the forward
// relation ceo from Company, which joins one table the same way. Django
// compares the SQL text, and so does the port.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_ticket_18375_join_reuse
func TestExpressionsTicket18375JoinReuse(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[expCompany](db).Filter("ceo__salary", orm.F("ceo__id"))
		djangotest.AssertEqual(t, expCountJoins(t, qs), 1)
	})
}

// The forward relation ceo of Company stands in for Django's reverse relation
// company_ceo_set, as in test_ticket_18375_join_reuse.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_ticket_18375_kwarg_ordering
func TestExpressionsTicket18375KwargOrdering(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[expCompany](db).Filter(
			"ceo__salary", orm.F("ceo__id"),
			"ceo__salary__gte", 1,
		)
		djangotest.AssertEqual(t, expCountJoins(t, qs), 1)
		qs = orm.Query[expCompany](db).Filter(
			"ceo__salary__gte", 1,
			"ceo__salary", orm.F("ceo__id"),
		)
		djangotest.AssertEqual(t, expCountJoins(t, qs), 1)
	})
}

// The forward relation ceo of Company stands in for Django's reverse relation
// company_ceo_set, as in test_ticket_18375_join_reuse.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_ticket_18375_kwarg_ordering_2
func TestExpressionsTicket18375KwargOrdering2(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[expCompany](db).Filter(
			"ceo__salary", orm.F("pk"),
			"pk", orm.F("ceo__salary"),
		)
		djangotest.AssertEqual(t, expCountJoins(t, qs), 1)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_ticket_18375_chained_filters
func TestExpressionsTicket18375ChainedFilters(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		// F() expressions do not reuse joins from previous filter.
		qs := orm.Query[expEmployee](db).Filter("company_ceo_set__num_employees", orm.F("pk")).
			Filter("company_ceo_set__num_employees", orm.F("company_ceo_set__num_employees"))
		djangotest.AssertEqual(t, expCountJoins(t, qs), 2)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_incorrect_field_in_F_expression
func TestExpressionsIncorrectFieldInFExpression(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[expEmployee](db).Filter("firstname", orm.F("nope")).All(context.Background())
		assertFieldError(t, err, "annot resolve keyword 'nope' into field.")
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_incorrect_joined_field_in_F_expression
func TestExpressionsIncorrectJoinedFieldInFExpression(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[expCompany](db).Filter("ceo__pk", orm.F("point_of_contact__nope")).All(context.Background())
		assertFieldError(t, err, "annot resolve keyword 'nope' into field.")
	})
}

// expIterable is the fixture of Django's IterableLookupInnerExpressionsTests.
type expIterable struct {
	c5020, c5040, c5050, c5060, c99300 *expCompany
}

// expSetUpIterable creates the IterableLookupInnerExpressionsTests fixture.
func expSetUpIterable(t *testing.T, db *orm.DB) expIterable {
	t.Helper()
	ceo := create(t, db, &expEmployee{Firstname: "Just", Lastname: "Doit", Salary: 30})
	company := func(name string, employees, chairs int64) *expCompany {
		return create(t, db, &expCompany{
			Name: name, NumEmployees: employees, NumChairs: chairs, CEO: fkTo[expEmployee](ceo.ID),
		})
	}
	return expIterable{
		c5020:  company("5020 Ltd", 50, 20),
		c5040:  company("5040 Ltd", 50, 40),
		c5050:  company("5050 Ltd", 50, 50),
		c5060:  company("5060 Ltd", 50, 60),
		c99300: company("99300 Ltd", 99, 300),
	}
}

// Ported from Django: tests/expressions/tests.py::IterableLookupInnerExpressionsTests.test_in_lookup_allows_F_expressions_and_expressions_for_integers
func TestExpressionsInLookupAllowsFExpressionsForIntegers(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpIterable(t, db)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expCompany](db).Filter("num_employees__in", []any{orm.F("num_chairs").Sub(10)}),
			[]string{fx.c5060.Name}, expCompanyName)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expCompany](db).Filter("num_employees__in",
				[]any{orm.F("num_chairs").Sub(10), orm.F("num_chairs").Add(10)}),
			[]string{fx.c5040.Name, fx.c5060.Name}, expCompanyName, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expCompany](db).Filter("num_employees__in",
				[]any{orm.F("num_chairs").Sub(10), orm.F("num_chairs"), orm.F("num_chairs").Add(10)}),
			[]string{fx.c5040.Name, fx.c5050.Name, fx.c5060.Name}, expCompanyName, djangotest.Unordered())
	})
}

// The join type is read from the SQL text: the filter joins with INNER JOIN,
// and the exclude with LEFT OUTER JOIN.
//
// Ported from Django: tests/expressions/tests.py::IterableLookupInnerExpressionsTests.test_expressions_range_lookups_join_choice
func TestExpressionsRangeLookupsJoinChoice(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		midpoint := orm.NewTime(13, 0, 0, 0)
		t1 := create(t, db, &expTime{Time: new(orm.NewTime(12, 0, 0, 0))})
		t2 := create(t, db, &expTime{Time: new(orm.NewTime(14, 0, 0, 0))})
		s1 := create(t, db, &expSimulationRun{
			Start: fkTo[expTime](t1.ID), End: fkTo[expTime](t2.ID), Midpoint: midpoint,
		})
		create(t, db, &expSimulationRun{Start: fkTo[expTime](t1.ID), Midpoint: midpoint})
		create(t, db, &expSimulationRun{End: fkTo[expTime](t2.ID), Midpoint: midpoint})
		create(t, db, &expSimulationRun{Midpoint: midpoint})

		bounds := []any{orm.F("start__time"), orm.F("end__time")}
		queryset := orm.Query[expSimulationRun](db).Filter("midpoint__range", bounds)
		djangotest.AssertQuerySetEqual(t, queryset, []int64{s1.ID}, func(r expSimulationRun) int64 { return r.ID })
		query, _, err := orm.CompileSelectForTest(queryset)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if strings.Contains(strings.ToUpper(query), "LEFT OUTER JOIN") || !strings.Contains(strings.ToUpper(query), "INNER JOIN") {
			t.Errorf("filter should use INNER joins only: %s", query)
		}

		queryset = orm.Query[expSimulationRun](db).Exclude("midpoint__range", bounds)
		djangotest.AssertQuerySetEqual(t, queryset, []int64{}, func(r expSimulationRun) int64 { return r.ID },
			djangotest.Unordered())
		query, _, err = orm.CompileSelectForTest(queryset)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if strings.Contains(strings.ToUpper(query), "INNER JOIN") || !strings.Contains(strings.ToUpper(query), "LEFT OUTER JOIN") {
			t.Errorf("exclude should use LEFT OUTER joins only: %s", query)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::IterableLookupInnerExpressionsTests.test_range_lookup_allows_F_expressions_and_expressions_for_integers
func TestExpressionsRangeLookupAllowsFExpressionsForIntegers(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpIterable(t, db)
		// Django builds this queryset without running it.
		orm.Query[expCompany](db).Filter("num_employees__exact", orm.F("num_chairs"))
		check := func(bounds []any, want ...*expCompany) {
			t.Helper()
			names := make([]string, 0, len(want))
			for _, c := range want {
				names = append(names, c.Name)
			}
			djangotest.AssertQuerySetEqual(t,
				orm.Query[expCompany](db).Filter("num_employees__range", bounds),
				names, expCompanyName, djangotest.Unordered())
		}
		check([]any{orm.F("num_chairs"), 100}, fx.c5020, fx.c5040, fx.c5050)
		check([]any{orm.F("num_chairs").Sub(10), orm.F("num_chairs").Add(10)}, fx.c5040, fx.c5050, fx.c5060)
		check([]any{orm.F("num_chairs").Sub(10), 100}, fx.c5020, fx.c5040, fx.c5050, fx.c5060)
		check([]any{1, 100}, fx.c5020, fx.c5040, fx.c5050, fx.c5060, fx.c99300)
	})
}

// The query adds a string to an integer column and filters name IN the result.
// The string must reach the database as a bound parameter and never as SQL
// text. Django runs it on SQLite only, because PostgreSQL rejects the type
// mismatch before it matters.
//
// Ported from Django: tests/expressions/tests.py::IterableLookupInnerExpressionsTests.test_expressions_not_introduce_sql_injection_via_untrusted_string_inclusion
func TestExpressionsNotIntroduceSQLInjectionViaUntrustedStringInclusion(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		if !expDialectIsSQLite(db) {
			t.Skip("This defensive test only works on databases that don't validate parameter types")
		}
		expSetUpIterable(t, db)
		queryset := orm.Query[expCompany](db).Filter("name__in",
			[]any{orm.F("num_chairs").Add("1)) OR ((1==1")})
		djangotest.AssertQuerySetEqual(t, queryset, []string{}, expCompanyName, djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions/tests.py::IterableLookupInnerExpressionsTests.test_range_lookup_allows_F_expressions_and_expressions_for_dates
func TestExpressionsRangeLookupAllowsFExpressionsForDates(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		start := time.Date(2016, 2, 3, 15, 0, 0, 0, time.UTC)
		end := time.Date(2016, 2, 5, 15, 0, 0, 0, time.UTC)
		experiment := func(name string) *expExperiment {
			return create(t, db, &expExperiment{
				Name: name, Assigned: orm.NewDate(2016, 2, 3), Start: start, End: end, Completed: orm.NewDate(2016, 2, 5),
			})
		}
		experiment1 := experiment("Integrity testing")
		experiment2 := experiment("Taste testing")
		r1 := create(t, db, &expResult{
			Experiment: fkTo[expExperiment](experiment1.ID), ResultTime: time.Date(2016, 2, 4, 15, 0, 0, 0, time.UTC),
		})
		create(t, db, &expResult{
			Experiment: fkTo[expExperiment](experiment1.ID), ResultTime: time.Date(2016, 3, 10, 2, 0, 0, 0, time.UTC),
		})
		create(t, db, &expResult{
			Experiment: fkTo[expExperiment](experiment2.ID), ResultTime: time.Date(2016, 1, 8, 5, 0, 0, 0, time.UTC),
		})
		tests := []struct {
			name, lookup string
			within       []any
		}{
			{"Datetimes", "result_time__range", []any{orm.F("experiment__start"), orm.F("experiment__end")}},
			{"Dates", "result_time__date__range", []any{orm.F("experiment__start__date"), orm.F("experiment__end__date")}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				queryset := orm.Query[expResult](db).Filter(tc.lookup, tc.within)
				djangotest.AssertQuerySetEqual(t, queryset, []int64{r1.ID}, func(r expResult) int64 { return r.ID })
			})
		}
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionsTests.test_F_reuse
func TestExpressionsFReuse(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		f := orm.F("id")
		n := create(t, db, &expNumber{Integer: -1})
		joe := create(t, db, &expEmployee{Firstname: "Joe", Lastname: "Smith"})
		c := create(t, db, &expCompany{
			Name: "Example Inc.", NumEmployees: 2300, NumChairs: 5, CEO: fkTo[expEmployee](joe.ID),
		})
		ctx := context.Background()
		cQS := orm.Query[expCompany](db).Filter("id", f)
		got, err := cQS.Get(ctx)
		if err != nil || got.ID != c.ID {
			t.Fatalf("company Get = %+v, %v; want id %d", got, err, c.ID)
		}
		// Reuse the same F value for another queryset.
		nQS := orm.Query[expNumber](db).Filter("id", f)
		gotN, err := nQS.Get(ctx)
		if err != nil || gotN.ID != n.ID {
			t.Fatalf("number Get = %+v, %v; want id %d", gotN, err, n.ID)
		}
		// The original query still works correctly.
		got, err = cQS.Get(ctx)
		if err != nil || got.ID != c.ID {
			t.Fatalf("company Get = %+v, %v; want id %d", got, err, c.ID)
		}
	})
}

// expBulkEmployees creates employees in one BulkCreate, as Django's
// Employee.objects.bulk_create does.
func expBulkEmployees(t *testing.T, db *orm.DB, rows [][2]string) {
	t.Helper()
	objs := make([]*expEmployee, len(rows))
	for i, r := range rows {
		objs[i] = &expEmployee{Firstname: r[0], Lastname: r[1]}
	}
	if err := orm.Query[expEmployee](db).BulkCreate(context.Background(), objs); err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
}

// Ported from Django: tests/expressions/tests.py::ExpressionsTests.test_patterns_escape
func TestExpressionsPatternsEscape(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expBulkEmployees(t, db, [][2]string{
			{"Johnny", "%John"},
			{"Jean-Claude", "Claud_"},
			{"Jean-Claude", "Claude%"},
			{"Johnny", "Joh\\n"},
			{"Johnny", "_ohn"},
			// Regex characters, which a backend that matches by regex must escape.
			{"Johnny", "^Joh"},
			{"Johnny", "Johnny$"},
			{"Johnny", "Joh."},
			{"Johnny", "[J]ohnny"},
			{"Johnny", "(J)ohnny"},
			{"Johnny", "J*ohnny"},
			{"Johnny", "J+ohnny"},
			{"Johnny", "J?ohnny"},
			{"Johnny", "J{1}ohnny"},
			{"Johnny", "J|ohnny"},
			{"Johnny", "J-ohnny"},
		})
		claude := create(t, db, &expEmployee{Firstname: "Jean-Claude", Lastname: "Claude"})
		john := create(t, db, &expEmployee{Firstname: "Johnny", Lastname: "John"})
		johnSign := create(t, db, &expEmployee{Firstname: "%Joh\\nny", Lastname: "%Joh\\n"})

		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("firstname__contains", orm.F("lastname")),
			[]int64{johnSign.ID, john.ID, claude.ID}, expEmployeeID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("firstname__startswith", orm.F("lastname")),
			[]int64{johnSign.ID, john.ID}, expEmployeeID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("firstname__endswith", orm.F("lastname")),
			[]int64{claude.ID}, expEmployeeID)
	})
}

// Ported from Django: tests/expressions/tests.py::ExpressionsTests.test_insensitive_patterns_escape
func TestExpressionsInsensitivePatternsEscape(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expBulkEmployees(t, db, [][2]string{
			{"Johnny", "%john"},
			{"Jean-Claude", "claud_"},
			{"Jean-Claude", "claude%"},
			{"Johnny", "joh\\n"},
			{"Johnny", "_ohn"},
		})
		claude := create(t, db, &expEmployee{Firstname: "Jean-Claude", Lastname: "claude"})
		john := create(t, db, &expEmployee{Firstname: "Johnny", Lastname: "john"})
		johnSign := create(t, db, &expEmployee{Firstname: "%Joh\\nny", Lastname: "%joh\\n"})

		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("firstname__icontains", orm.F("lastname")),
			[]int64{johnSign.ID, john.ID, claude.ID}, expEmployeeID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("firstname__istartswith", orm.F("lastname")),
			[]int64{johnSign.ID, john.ID}, expEmployeeID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expEmployee](db).Filter("firstname__iendswith", orm.F("lastname")),
			[]int64{claude.ID}, expEmployeeID)
	})
}

// expFieldTransformSetUp creates the experiment of
// FieldTransformTests.setUpTestData.
func expFieldTransformSetUp(t *testing.T, db *orm.DB) {
	t.Helper()
	sday := orm.NewDate(2010, 6, 25)
	stime := time.Date(2010, 6, 25, 12, 15, 30, 747000000, time.UTC)
	create(t, db, &expExperiment{
		Name: "Experiment 1", Assigned: sday, Completed: orm.NewDate(2010, 6, 27),
		Start: stime, End: stime.AddDate(0, 0, 2),
	})
}

// Ported from Django: tests/expressions/tests.py::FieldTransformTests.test_month_aggregation
func TestExpressionsMonthAggregation(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expFieldTransformSetUp(t, db)
		vals, err := orm.Query[expExperiment](db).Aggregate(context.Background(),
			"month_count", orm.Count("assigned__month"))
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, vals, map[string]any{"month_count": int64(1)})
	})
}

// Ported from Django: tests/expressions/tests.py::FieldTransformTests.test_transform_in_values
func TestExpressionsTransformInValues(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expFieldTransformSetUp(t, db)
		assertValues(t, orm.Query[expExperiment](db).Values("assigned__month"),
			[]map[string]any{{"assigned__month": int64(6)}})
	})
}

// Ported from Django: tests/expressions/tests.py::FieldTransformTests.test_multiple_transforms_in_values
func TestExpressionsMultipleTransformsInValues(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expFieldTransformSetUp(t, db)
		assertValues(t, orm.Query[expExperiment](db).Values("end__date__month"),
			[]map[string]any{{"end__date__month": int64(6)}})
	})
}

// expSetUpNegated creates the NegatedExpressionTests fixture: a company based
// in the EU and one that is not.
func expSetUpNegated(t *testing.T, db *orm.DB) (eu, nonEU *expCompany) {
	t.Helper()
	ceo := create(t, db, &expEmployee{Firstname: "Joe", Lastname: "Smith", Salary: 10})
	eu = create(t, db, &expCompany{
		Name: "Example Inc.", NumEmployees: 2300, NumChairs: 5, CEO: fkTo[expEmployee](ceo.ID), BasedInEU: true,
	})
	nonEU = create(t, db, &expCompany{
		Name: "Foobar Ltd.", NumEmployees: 3, NumChairs: 4, CEO: fkTo[expEmployee](ceo.ID),
	})
	return eu, nonEU
}

// Django compares the expression objects. Go compares the values of Expr.
//
// Ported from Django: tests/expressions/tests.py::NegatedExpressionTests.test_invert
func TestNegatedExpressionInvert(t *testing.T) {
	f := orm.F("field")
	if reflect.DeepEqual(f.Not(), f) {
		t.Errorf("F.Not() equals F")
	}
	if !reflect.DeepEqual(f.Not(), orm.F("field").Not()) {
		t.Errorf("two negations of the same F differ")
	}
	if !reflect.DeepEqual(f.Not().Not(), f) {
		t.Errorf("F.Not().Not() = %+v, want F", f.Not().Not())
	}
}

// Filter takes no boolean expression as a condition, so the first assertion,
// filter(~F("based_in_eu")), is ported with the condition based_in_eu=False.
//
// Ported from Django: tests/expressions/tests.py::NegatedExpressionTests.test_filter
func TestNegatedExpressionFilter(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		eu, nonEU := expSetUpNegated(t, db)
		ids := func(qs *orm.QuerySet[expCompany]) []int64 {
			rows := mustAll(t, qs)
			out := make([]int64, 0, len(rows))
			for _, c := range rows {
				out = append(out, c.ID)
			}
			return out
		}
		djangotest.AssertEqual(t, ids(orm.Query[expCompany](db).Filter("based_in_eu", false)), []int64{nonEU.ID})
		qs := func() *orm.QuerySet[expCompany] {
			return orm.Query[expCompany](db).Annotate("eu_required", orm.Value(false).Not())
		}
		djangotest.AssertEqual(t, ids(qs().Filter("based_in_eu", orm.F("eu_required")).OrderBy("eu_required")),
			[]int64{eu.ID})
		djangotest.AssertEqual(t, ids(qs().Filter("based_in_eu", orm.F("eu_required").Not().Not())), []int64{eu.ID})
		djangotest.AssertEqual(t, ids(qs().Filter("based_in_eu", orm.F("eu_required").Not())), []int64{nonEU.ID})
		djangotest.AssertEqual(t, len(ids(qs().Filter("based_in_eu", orm.F("based_in_eu").Not()))), 0)
	})
}

// Ported from Django: tests/expressions/tests.py::NegatedExpressionTests.test_values
func TestNegatedExpressionValues(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpNegated(t, db)
		rows, err := orm.Query[expCompany](db).Annotate("negated", orm.F("based_in_eu").Not()).
			ValuesList("name", "negated").OrderBy("name").All(context.Background())
		if err != nil {
			t.Fatalf("ValuesList: %v", err)
		}
		djangotest.AssertEqual(t, rows, [][]any{{"Example Inc.", false}, {"Foobar Ltd.", true}})
	})
}

// No Go counterpart: tests/expressions/tests.py::OrderByTests.test_nulls_false
// passes nulls_first=False and nulls_last=False, which Django rejects.
// OrderExpr.NullsFirst and OrderExpr.NullsLast take no flag.
