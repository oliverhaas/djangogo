package orm_test

import (
	"context"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// caseTestModel is Django's expressions_case.CaseTestModel without its file,
// file_path and image fields: the ORM has no FileField, FilePathField or
// ImageField.
type caseTestModel struct {
	ID                   int64
	Integer              int64
	Integer2             *int64                `orm:"null"`
	String               string                `orm:"max_length=100"`
	BigInteger           *int64                `orm:"type=bigint;null"`
	Binary               []byte                ``
	Boolean              bool                  ``
	Date                 *orm.Date             `orm:"null;column=date_field"`
	DateTime             *time.Time            `orm:"null"`
	Decimal              *orm.Decimal          `orm:"max_digits=2;decimal_places=1;null;column=decimal_field"`
	Duration             *time.Duration        `orm:"null"`
	Email                string                `orm:"type=email"`
	Float                *float64              `orm:"null;column=float_field"`
	GenericIPAddress     netip.Addr            `orm:"null"`
	NullBoolean          *bool                 `orm:"null"`
	PositiveInteger      *int64                `orm:"type=positive_int;null"`
	PositiveSmallInteger *int16                `orm:"type=positive_smallint;null"`
	PositiveBigInteger   *int16                `orm:"type=positive_smallint;null"`
	Slug                 string                `orm:"type=slug"`
	SmallInteger         *int16                `orm:"type=smallint;null"`
	Text                 string                `orm:"type=text"`
	Time                 *orm.Time             `orm:"null;column=time_field"`
	URL                  string                `orm:"type=url"`
	UUID                 *orm.UUID             `orm:"null"`
	FK                   orm.FK[caseTestModel] `orm:"null;on_delete=cascade"`
}

// caseO2OModel is Django's expressions_case.O2OCaseTestModel.
type caseO2OModel struct {
	ID      int64
	O2O     orm.O2O[caseTestModel] `orm:"on_delete=cascade;related_name=o2o_rel"`
	Integer int64
}

// caseFKModel is Django's expressions_case.FKCaseTestModel.
type caseFKModel struct {
	ID      int64
	FK      orm.FK[caseTestModel] `orm:"on_delete=cascade;related_name=fk_rel"`
	Integer int64
}

// caseClient is Django's expressions_case.Client. The account type is "R"
// (regular), "G" (gold) or "P" (platinum).
type caseClient struct {
	ID           int64
	Name         string `orm:"max_length=50"`
	RegisteredOn orm.Date
	AccountType  string `orm:"max_length=1;default=R"`
}

// caseModels returns the models of the ports of tests/expressions_case.
func caseModels() []any {
	return []any{djangotest.App("expressions_case", &caseTestModel{}, &caseO2OModel{}, &caseFKModel{}, &caseClient{})}
}

// caseSetUp creates the rows of CaseExpressionTests.setUpTestData: seven
// caseTestModel rows, each with a caseO2OModel row and caseFKModel rows.
func caseSetUp(t *testing.T, db *orm.DB) {
	t.Helper()
	for _, r := range []struct {
		integer, integer2, o2o int64
		fks                    []int64
	}{
		{1, 1, 1, []int64{1}},
		{2, 3, 2, []int64{2, 3}},
		{3, 4, 3, []int64{3, 4}},
		{2, 2, 2, []int64{2, 3}},
		{3, 4, 3, []int64{3, 4}},
		{3, 3, 3, []int64{3, 4}},
		{4, 5, 1, []int64{5}},
	} {
		o := create(t, db, &caseTestModel{Integer: r.integer, Integer2: new(r.integer2), String: strconv.FormatInt(r.integer, 10)})
		create(t, db, &caseO2OModel{O2O: o2oTo[caseTestModel](o.ID), Integer: r.o2o})
		for _, n := range r.fks {
			create(t, db, &caseFKModel{FK: fkTo[caseTestModel](o.ID), Integer: n})
		}
	}
}

// caseIntegers are the integer values of the rows of caseSetUp, in pk order.
var caseIntegers = []int64{1, 2, 3, 2, 3, 3, 4}

// caseGroupByFields are the keys of the fields of caseTestModel, Django's
// group_by_fields: SQLite and PostgreSQL group by binary and text columns.
var caseGroupByFields = []string{
	"id", "integer", "integer2", "string", "big_integer", "binary", "boolean", "date", "date_time",
	"decimal", "duration", "email", "float", "generic_ip_address", "null_boolean", "positive_integer",
	"positive_small_integer", "positive_big_integer", "slug", "small_integer", "text", "time", "url",
	"uuid", "fk",
}

// caseRows returns rows with each int as an int64, the Go type of the
// integer values the ORM reads.
func caseRows(rows [][]any) [][]any {
	for _, row := range rows {
		for i, v := range row {
			if n, ok := v.(int); ok {
				row[i] = int64(n)
			}
		}
	}
	return rows
}

// assertProjected runs a Values queryset and checks the values of keys in
// its rows, in order, as Django's itemgetter transform does.
func assertProjected(t *testing.T, vs *orm.ValuesQuerySet, keys []string, want [][]any) {
	t.Helper()
	rows, err := vs.All(context.Background())
	if err != nil {
		t.Errorf("Values: %v", err)
		return
	}
	got := make([][]any, len(rows))
	for i, row := range rows {
		for _, k := range keys {
			got[i] = append(got[i], row[k])
		}
	}
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		for k := range keys {
			equal = equal && equalValue(got[i][k], want[i][k])
		}
	}
	if !equal {
		t.Errorf("values differ:\n got: %v\nwant: %v", got, want)
	}
}

// assertCaseUpdate sets field of every caseTestModel row to value and checks
// the integer and the field of the rows, in pk order. want holds the field
// values.
func assertCaseUpdate(t *testing.T, db *orm.DB, field string, value orm.Expr, want []any) {
	t.Helper()
	if _, err := orm.Query[caseTestModel](db).Update(context.Background(), field, value); err != nil {
		t.Fatalf("Update: %v", err)
	}
	rows := make([][]any, len(want))
	for i, w := range want {
		rows[i] = []any{caseIntegers[i], w}
	}
	assertValuesList(t, orm.Query[caseTestModel](db).OrderBy("pk").ValuesList("integer", field), rows)
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate
func TestCaseAnnotate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: orm.Value("one")},
			orm.When{Cond: orm.Q("integer", 2), Then: orm.Value("two")},
		).Default(orm.Value("other")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "test"), caseRows([][]any{
			{1, "one"}, {2, "two"}, {3, "other"}, {2, "two"}, {3, "other"}, {3, "other"}, {4, "other"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_without_default
func TestCaseAnnotateWithoutDefault(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: 1},
			orm.When{Cond: orm.Q("integer", 2), Then: 2},
		))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "test"), caseRows([][]any{
			{1, 1}, {2, 2}, {3, nil}, {2, 2}, {3, nil}, {3, nil}, {4, nil},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_expression_as_value
func TestCaseAnnotateWithExpressionAsValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("f_test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: orm.F("integer").Add(1)},
			orm.When{Cond: orm.Q("integer", 2), Then: orm.F("integer").Add(3)},
		).Default("integer"))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "f_test"), caseRows([][]any{
			{1, 2}, {2, 5}, {3, 3}, {2, 5}, {3, 3}, {3, 3}, {4, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_expression_as_condition
func TestCaseAnnotateWithExpressionAsCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("f_test", orm.Case(
			orm.When{Cond: orm.Q("integer2", orm.F("integer")), Then: orm.Value("equal")},
			orm.When{Cond: orm.Q("integer2", orm.F("integer").Add(1)), Then: orm.Value("+1")},
		))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "f_test"), caseRows([][]any{
			{1, "equal"}, {2, "+1"}, {3, "+1"}, {2, "equal"}, {3, "+1"}, {3, "equal"}, {4, "+1"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_join_in_value
func TestCaseAnnotateWithJoinInValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("join_test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: orm.F("o2o_rel__integer").Add(1)},
			orm.When{Cond: orm.Q("integer", 2), Then: orm.F("o2o_rel__integer").Add(3)},
		).Default("o2o_rel__integer"))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "join_test"), caseRows([][]any{
			{1, 2}, {2, 5}, {3, 3}, {2, 5}, {3, 3}, {3, 3}, {4, 1},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_in_clause
//
// ValuesList reads the two values that Django's only("pk", "integer") loads.
func TestCaseAnnotateWithInClause(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		fkRels := orm.Query[caseFKModel](db).Filter("integer__in", []int64{5})
		qs := orm.Query[caseTestModel](db).Annotate("in_test", orm.Sum(orm.Case(
			orm.When{Cond: orm.Q("fk_rel__in", fkRels), Then: orm.F("fk_rel__integer")},
		).Default(orm.Value(0))))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "in_test"), caseRows([][]any{
			{1, 0}, {2, 0}, {3, 0}, {2, 0}, {3, 0}, {3, 0}, {4, 5},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_join_in_condition
func TestCaseAnnotateWithJoinInCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("join_test", orm.Case(
			orm.When{Cond: orm.Q("integer2", orm.F("o2o_rel__integer")), Then: orm.Value("equal")},
			orm.When{Cond: orm.Q("integer2", orm.F("o2o_rel__integer").Add(1)), Then: orm.Value("+1")},
		).Default(orm.Value("other")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "join_test"), caseRows([][]any{
			{1, "equal"}, {2, "+1"}, {3, "+1"}, {2, "equal"}, {3, "+1"}, {3, "equal"}, {4, "other"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_join_in_predicate
func TestCaseAnnotateWithJoinInPredicate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("join_test", orm.Case(
			orm.When{Cond: orm.Q("o2o_rel__integer", 1), Then: orm.Value("one")},
			orm.When{Cond: orm.Q("o2o_rel__integer", 2), Then: orm.Value("two")},
			orm.When{Cond: orm.Q("o2o_rel__integer", 3), Then: orm.Value("three")},
		).Default(orm.Value("other")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "join_test"), caseRows([][]any{
			{1, "one"}, {2, "two"}, {3, "three"}, {2, "two"}, {3, "three"}, {3, "three"}, {4, "one"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_annotation_in_value
func TestCaseAnnotateWithAnnotationInValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).
			Annotate("f_plus_1", orm.F("integer").Add(1), "f_plus_3", orm.F("integer").Add(3)).
			Annotate("f_test", orm.Case(
				orm.When{Cond: orm.Q("integer", 1), Then: "f_plus_1"},
				orm.When{Cond: orm.Q("integer", 2), Then: "f_plus_3"},
			).Default("integer"))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "f_test"), caseRows([][]any{
			{1, 2}, {2, 5}, {3, 3}, {2, 5}, {3, 3}, {3, 3}, {4, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_annotation_in_condition
func TestCaseAnnotateWithAnnotationInCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).
			Annotate("f_plus_1", orm.F("integer").Add(1)).
			Annotate("f_test", orm.Case(
				orm.When{Cond: orm.Q("integer2", orm.F("integer")), Then: orm.Value("equal")},
				orm.When{Cond: orm.Q("integer2", orm.F("f_plus_1")), Then: orm.Value("+1")},
			))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "f_test"), caseRows([][]any{
			{1, "equal"}, {2, "+1"}, {3, "+1"}, {2, "equal"}, {3, "+1"}, {3, "equal"}, {4, "+1"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_annotation_in_predicate
func TestCaseAnnotateWithAnnotationInPredicate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).
			Annotate("f_minus_2", orm.F("integer").Sub(2)).
			Annotate("test", orm.Case(
				orm.When{Cond: orm.Q("f_minus_2", -1), Then: orm.Value("negative one")},
				orm.When{Cond: orm.Q("f_minus_2", 0), Then: orm.Value("zero")},
				orm.When{Cond: orm.Q("f_minus_2", 1), Then: orm.Value("one")},
			).Default(orm.Value("other")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "test"), caseRows([][]any{
			{1, "negative one"}, {2, "zero"}, {3, "one"}, {2, "zero"}, {3, "one"}, {3, "one"}, {4, "other"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_aggregation_in_value
func TestCaseAnnotateWithAggregationInValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		vs := orm.Query[caseTestModel](db).Values(caseGroupByFields...).
			Annotate("min", orm.Min("fk_rel__integer"), "max", orm.Max("fk_rel__integer")).
			Annotate("test", orm.Case(
				orm.When{Cond: orm.Q("integer", 2), Then: "min"},
				orm.When{Cond: orm.Q("integer", 3), Then: "max"},
			)).
			OrderBy("pk")
		assertProjected(t, vs, []string{"integer", "test", "min", "max"}, caseRows([][]any{
			{1, nil, 1, 1}, {2, 2, 2, 3}, {3, 4, 3, 4}, {2, 2, 2, 3}, {3, 4, 3, 4}, {3, 4, 3, 4}, {4, nil, 5, 5},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_aggregation_in_condition
func TestCaseAnnotateWithAggregationInCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		vs := orm.Query[caseTestModel](db).Values(caseGroupByFields...).
			Annotate("min", orm.Min("fk_rel__integer"), "max", orm.Max("fk_rel__integer")).
			Annotate("test", orm.Case(
				orm.When{Cond: orm.Q("integer2", orm.F("min")), Then: orm.Value("min")},
				orm.When{Cond: orm.Q("integer2", orm.F("max")), Then: orm.Value("max")},
			)).
			OrderBy("pk")
		assertProjected(t, vs, []string{"integer", "integer2", "test"}, caseRows([][]any{
			{1, 1, "min"}, {2, 3, "max"}, {3, 4, "max"}, {2, 2, "min"}, {3, 4, "max"}, {3, 3, "min"}, {4, 5, "min"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_aggregation_in_predicate
func TestCaseAnnotateWithAggregationInPredicate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		vs := orm.Query[caseTestModel](db).Values(caseGroupByFields...).
			Annotate("max", orm.Max("fk_rel__integer")).
			Annotate("test", orm.Case(
				orm.When{Cond: orm.Q("max", 3), Then: orm.Value("max = 3")},
				orm.When{Cond: orm.Q("max", 4), Then: orm.Value("max = 4")},
			).Default(orm.Value(""))).
			OrderBy("pk")
		assertProjected(t, vs, []string{"integer", "max", "test"}, caseRows([][]any{
			{1, 1, ""}, {2, 3, "max = 3"}, {3, 4, "max = 4"}, {2, 3, "max = 3"}, {3, 4, "max = 4"}, {3, 4, "max = 4"}, {4, 5, ""},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_exclude
func TestCaseAnnotateExclude(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: orm.Value("one")},
			orm.When{Cond: orm.Q("integer", 2), Then: orm.Value("two")},
		).Default(orm.Value("other")))
		assertValuesList(t, qs.Exclude("test", "other").OrderBy("pk").ValuesList("integer", "test"), caseRows([][]any{
			{1, "one"}, {2, "two"}, {2, "two"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_filter_decimal
func TestCaseAnnotateFilterDecimal(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		obj := create(t, db, &caseTestModel{Integer: 0, Decimal: new(orm.MustDecimal("1"))})
		qs := orm.Query[caseTestModel](db).Annotate(
			"x", orm.Case(orm.When{Cond: orm.Q("integer", 0), Then: orm.F("decimal")}),
			"y", orm.Case(orm.When{Cond: orm.Q("integer", 0), Then: orm.Value(orm.MustDecimal("1"))}),
		)
		assertFlat(t, qs.Filter(orm.Q("x", 1).And(orm.Q("x", orm.MustDecimal("1")))).ValuesList("id"), []any{obj.ID})
		assertFlat(t, qs.Filter(orm.Q("y", 1).And(orm.Q("y", orm.MustDecimal("1")))).ValuesList("id"), []any{obj.ID})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_values_not_in_order_by
func TestCaseAnnotateValuesNotInOrderBy(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: orm.Value("one")},
			orm.When{Cond: orm.Q("integer", 2), Then: orm.Value("two")},
			orm.When{Cond: orm.Q("integer", 3), Then: orm.Value("three")},
		).Default(orm.Value("other")))
		assertFlat(t, qs.OrderBy("test").ValuesList("integer"), []any{int64(1), int64(4), int64(3), int64(3), int64(3), int64(2), int64(2)})
	})
}

// assertAllAnnotated checks that qs returns every caseTestModel row, each
// with the annotation key set to want.
func assertAllAnnotated(t *testing.T, db *orm.DB, qs *orm.QuerySet[caseTestModel], key string, want any) {
	t.Helper()
	objects, err := qs.AllAnnotated(context.Background())
	if err != nil {
		t.Fatalf("AllAnnotated: %v", err)
	}
	if n := mustCount(t, orm.Query[caseTestModel](db)); int64(len(objects)) != n {
		t.Errorf("got %d rows, want %d", len(objects), n)
	}
	for _, obj := range objects {
		if obj.Annotations[key] != want {
			t.Errorf("%s = %v, want %v", key, obj.Annotations[key], want)
		}
	}
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_empty_when
func TestCaseAnnotateWithEmptyWhen(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("selected", orm.Case(
			orm.When{Cond: orm.Q("pk__in", []int64{}), Then: orm.Value("selected")},
		).Default(orm.Value("not selected")))
		assertAllAnnotated(t, db, qs, "selected", "not selected")
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_annotate_with_full_when
func TestCaseAnnotateWithFullWhen(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("selected", orm.Case(
			orm.When{Cond: orm.Not(orm.Q("pk__in", []int64{})), Then: orm.Value("selected")},
		).Default(orm.Value("not selected")))
		assertAllAnnotated(t, db, qs, "selected", "selected")
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_combined_expression
func TestCaseCombinedExpression(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: 2},
			orm.When{Cond: orm.Q("integer", 2), Then: 1},
		).Default(3).Add(1))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "test"), caseRows([][]any{
			{1, 3}, {2, 2}, {3, 4}, {2, 2}, {3, 4}, {3, 4}, {4, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_in_subquery
func TestCaseInSubquery(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		sub := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", orm.F("integer2")), Then: "pk"},
			orm.When{Cond: orm.Q("integer", 4), Then: "pk"},
		)).Values("test")
		qs := orm.Query[caseTestModel](db).Filter("pk__in", sub)
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{1, 1}, {2, 2}, {3, 3}, {4, 5},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_condition_with_lookups
//
// The values are booleans, so the Case needs no output field.
func TestCaseConditionWithLookups(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer2", 1, "string", "2"), Then: orm.Value(false)},
			orm.When{Cond: orm.Q("integer2", 1, "string", "1"), Then: orm.Value(true)},
		).Default(orm.Value(false)))
		djangotest.AssertEqual(t, firstValue(t, qs.Filter("integer", 1).ValuesList("test")), any(true))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_case_reuse
func TestCaseReuse(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		caseSetUp(t, db)
		someCase := orm.Case(orm.When{Cond: orm.Q("pk", 0), Then: orm.Value("0")}).Default(orm.Value("1"))
		objects, err := orm.Query[caseTestModel](db).Annotate("somecase", someCase).OrderBy("pk").AllAnnotated(ctx)
		if err != nil {
			t.Fatalf("AllAnnotated: %v", err)
		}
		got := make([][]any, len(objects))
		for i, obj := range objects {
			got[i] = []any{obj.Object.ID, obj.Annotations["somecase"]}
		}
		assertValuesList(t, orm.Query[caseTestModel](db).Annotate("somecase", someCase).OrderBy("pk").ValuesList("pk", "somecase"), got)
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_aggregate
func TestCaseAggregate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		got, err := orm.Query[caseTestModel](db).Aggregate(context.Background(),
			"one", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer", 1), Then: 1})),
			"two", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer", 2), Then: 1})),
			"three", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer", 3), Then: 1})),
			"four", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer", 4), Then: 1})),
		)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, got, map[string]any{"one": int64(1), "two": int64(2), "three": int64(3), "four": int64(1)})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_aggregate_with_expression_as_value
func TestCaseAggregateWithExpressionAsValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		got, err := orm.Query[caseTestModel](db).Aggregate(context.Background(),
			"one", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer", 1), Then: "integer"})),
			"two", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer", 2), Then: orm.F("integer").Sub(1)})),
			"three", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer", 3), Then: orm.F("integer").Add(1)})),
		)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, got, map[string]any{"one": int64(1), "two": int64(2), "three": int64(12)})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_aggregate_with_expression_as_condition
func TestCaseAggregateWithExpressionAsCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		got, err := orm.Query[caseTestModel](db).Aggregate(context.Background(),
			"equal", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer2", orm.F("integer")), Then: 1})),
			"plus_one", orm.Sum(orm.Case(orm.When{Cond: orm.Q("integer2", orm.F("integer").Add(1)), Then: 1})),
		)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, got, map[string]any{"equal": int64(3), "plus_one": int64(4)})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter
func TestCaseFilter(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("integer2", orm.Case(
			orm.When{Cond: orm.Q("integer", 2), Then: 3},
			orm.When{Cond: orm.Q("integer", 3), Then: 4},
		).Default(1))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{1, 1}, {2, 3}, {3, 4}, {3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_without_default
func TestCaseFilterWithoutDefault(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("integer2", orm.Case(
			orm.When{Cond: orm.Q("integer", 2), Then: 3},
			orm.When{Cond: orm.Q("integer", 3), Then: 4},
		))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{2, 3}, {3, 4}, {3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_expression_as_value
func TestCaseFilterWithExpressionAsValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("integer2", orm.Case(
			orm.When{Cond: orm.Q("integer", 2), Then: orm.F("integer").Add(1)},
			orm.When{Cond: orm.Q("integer", 3), Then: orm.F("integer")},
		).Default("integer"))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{1, 1}, {2, 3}, {3, 3},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_expression_as_condition
func TestCaseFilterWithExpressionAsCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("string", orm.Case(
			orm.When{Cond: orm.Q("integer2", orm.F("integer")), Then: orm.Value("2")},
			orm.When{Cond: orm.Q("integer2", orm.F("integer").Add(1)), Then: orm.Value("3")},
		))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2", "string"), caseRows([][]any{
			{3, 4, "3"}, {2, 2, "2"}, {3, 4, "3"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_join_in_value
func TestCaseFilterWithJoinInValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("integer2", orm.Case(
			orm.When{Cond: orm.Q("integer", 2), Then: orm.F("o2o_rel__integer").Add(1)},
			orm.When{Cond: orm.Q("integer", 3), Then: orm.F("o2o_rel__integer")},
		).Default("o2o_rel__integer"))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{1, 1}, {2, 3}, {3, 3},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_join_in_condition
func TestCaseFilterWithJoinInCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("integer", orm.Case(
			orm.When{Cond: orm.Q("integer2", orm.F("o2o_rel__integer").Add(1)), Then: 2},
			orm.When{Cond: orm.Q("integer2", orm.F("o2o_rel__integer")), Then: 3},
		))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{2, 3}, {3, 3},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_join_in_predicate
func TestCaseFilterWithJoinInPredicate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("integer2", orm.Case(
			orm.When{Cond: orm.Q("o2o_rel__integer", 1), Then: 1},
			orm.When{Cond: orm.Q("o2o_rel__integer", 2), Then: 3},
			orm.When{Cond: orm.Q("o2o_rel__integer", 3), Then: 4},
		))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{1, 1}, {2, 3}, {3, 4}, {3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_annotation_in_value
func TestCaseFilterWithAnnotationInValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).
			Annotate("f", orm.F("integer"), "f_plus_1", orm.F("integer").Add(1)).
			Filter("integer2", orm.Case(
				orm.When{Cond: orm.Q("integer", 2), Then: "f_plus_1"},
				orm.When{Cond: orm.Q("integer", 3), Then: "f"},
			))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{2, 3}, {3, 3},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_annotation_in_condition
func TestCaseFilterWithAnnotationInCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).
			Annotate("f_plus_1", orm.F("integer").Add(1)).
			Filter("integer", orm.Case(
				orm.When{Cond: orm.Q("integer2", orm.F("integer")), Then: 2},
				orm.When{Cond: orm.Q("integer2", orm.F("f_plus_1")), Then: 3},
			))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{3, 4}, {2, 2}, {3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_annotation_in_predicate
func TestCaseFilterWithAnnotationInPredicate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).
			Annotate("f_plus_1", orm.F("integer").Add(1)).
			Filter("integer2", orm.Case(
				orm.When{Cond: orm.Q("f_plus_1", 3), Then: 3},
				orm.When{Cond: orm.Q("f_plus_1", 4), Then: 4},
			).Default(1))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2"), caseRows([][]any{
			{1, 1}, {2, 3}, {3, 4}, {3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_aggregation_in_value
func TestCaseFilterWithAggregationInValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		vs := orm.Query[caseTestModel](db).Values(caseGroupByFields...).
			Annotate("min", orm.Min("fk_rel__integer"), "max", orm.Max("fk_rel__integer")).
			Filter("integer2", orm.Case(
				orm.When{Cond: orm.Q("integer", 2), Then: "min"},
				orm.When{Cond: orm.Q("integer", 3), Then: "max"},
			)).
			OrderBy("pk")
		assertProjected(t, vs, []string{"integer", "integer2", "min", "max"}, caseRows([][]any{
			{3, 4, 3, 4}, {2, 2, 2, 3}, {3, 4, 3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_aggregation_in_condition
func TestCaseFilterWithAggregationInCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		vs := orm.Query[caseTestModel](db).Values(caseGroupByFields...).
			Annotate("min", orm.Min("fk_rel__integer"), "max", orm.Max("fk_rel__integer")).
			Filter("integer", orm.Case(
				orm.When{Cond: orm.Q("integer2", orm.F("min")), Then: 2},
				orm.When{Cond: orm.Q("integer2", orm.F("max")), Then: 3},
			)).
			OrderBy("pk")
		assertProjected(t, vs, []string{"integer", "integer2", "min", "max"}, caseRows([][]any{
			{3, 4, 3, 4}, {2, 2, 2, 3}, {3, 4, 3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_filter_with_aggregation_in_predicate
func TestCaseFilterWithAggregationInPredicate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		vs := orm.Query[caseTestModel](db).Values(caseGroupByFields...).
			Annotate("max", orm.Max("fk_rel__integer")).
			Filter("integer", orm.Case(
				orm.When{Cond: orm.Q("max", 3), Then: 2},
				orm.When{Cond: orm.Q("max", 4), Then: 3},
			)).
			OrderBy("pk")
		assertProjected(t, vs, []string{"integer", "integer2", "max"}, caseRows([][]any{
			{2, 3, 3}, {3, 4, 4}, {2, 2, 3}, {3, 4, 4}, {3, 3, 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update
func TestCaseUpdate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "string", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: orm.Value("one")},
			orm.When{Cond: orm.Q("integer", 2), Then: orm.Value("two")},
		).Default(orm.Value("other")), []any{"one", "two", "other", "two", "other", "other", "other"})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_without_default
func TestCaseUpdateWithoutDefault(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "integer2", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: 1},
			orm.When{Cond: orm.Q("integer", 2), Then: 2},
		), []any{int64(1), int64(2), nil, int64(2), nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_with_expression_as_value
func TestCaseUpdateWithExpressionAsValue(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		_, err := orm.Query[caseTestModel](db).Update(context.Background(), "integer", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: orm.F("integer").Add(1)},
			orm.When{Cond: orm.Q("integer", 2), Then: orm.F("integer").Add(3)},
		).Default("integer"))
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		assertValuesList(t, orm.Query[caseTestModel](db).OrderBy("pk").ValuesList("string", "integer"), caseRows([][]any{
			{"1", 2}, {"2", 5}, {"3", 3}, {"2", 5}, {"3", 3}, {"3", 3}, {"4", 4},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_with_expression_as_condition
func TestCaseUpdateWithExpressionAsCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "string", orm.Case(
			orm.When{Cond: orm.Q("integer2", orm.F("integer")), Then: orm.Value("equal")},
			orm.When{Cond: orm.Q("integer2", orm.F("integer").Add(1)), Then: orm.Value("+1")},
		), []any{"equal", "+1", "+1", "equal", "+1", "equal", "+1"})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_with_join_in_condition_raise_field_error
func TestCaseUpdateWithJoinInConditionRaiseFieldError(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		_, err := orm.Query[caseTestModel](db).Update(context.Background(), "integer", orm.Case(
			orm.When{Cond: orm.Q("integer2", orm.F("o2o_rel__integer").Add(1)), Then: 2},
			orm.When{Cond: orm.Q("integer2", orm.F("o2o_rel__integer")), Then: 3},
		))
		assertFieldError(t, err, "joined field references are not permitted in this query")
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_with_join_in_predicate_raise_field_error
func TestCaseUpdateWithJoinInPredicateRaiseFieldError(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		_, err := orm.Query[caseTestModel](db).Update(context.Background(), "string", orm.Case(
			orm.When{Cond: orm.Q("o2o_rel__integer", 1), Then: orm.Value("one")},
			orm.When{Cond: orm.Q("o2o_rel__integer", 2), Then: orm.Value("two")},
			orm.When{Cond: orm.Q("o2o_rel__integer", 3), Then: orm.Value("three")},
		).Default(orm.Value("other")))
		assertFieldError(t, err, "joined field references are not permitted in this query")
	})
}

// caseOneTwo returns the Case that is v1 for the rows with integer 1 and v2
// for those with integer 2, the Case of most of Django's test_update_*
// tests.
func caseOneTwo(v1, v2 any) orm.Expr {
	return orm.Case(
		orm.When{Cond: orm.Q("integer", 1), Then: v1},
		orm.When{Cond: orm.Q("integer", 2), Then: v2},
	)
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_big_integer
func TestCaseUpdateBigInteger(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "big_integer", caseOneTwo(1, 2), []any{int64(1), int64(2), nil, int64(2), nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_binary
func TestCaseUpdateBinary(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		one, two, empty := []byte("one"), []byte("two"), []byte{}
		assertCaseUpdate(t, db, "binary", caseOneTwo(one, two).Default(empty), []any{one, two, empty, two, empty, empty, empty})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_boolean
func TestCaseUpdateBoolean(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "boolean", caseOneTwo(true, true).Default(false), []any{true, true, false, true, false, false, false})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_date
func TestCaseUpdateDate(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		d1, d2 := orm.NewDate(2015, 1, 1), orm.NewDate(2015, 1, 2)
		assertCaseUpdate(t, db, "date", caseOneTwo(d1, d2), []any{d1, d2, nil, d2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_date_time
func TestCaseUpdateDateTime(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		d1, d2 := day(2015, 1, 1), day(2015, 1, 2)
		assertCaseUpdate(t, db, "date_time", caseOneTwo(d1, d2), []any{d1, d2, nil, d2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_decimal
func TestCaseUpdateDecimal(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		d1, d2 := orm.MustDecimal("1.1"), orm.MustDecimal("2.2")
		assertCaseUpdate(t, db, "decimal", caseOneTwo(d1, orm.Value(d2)), []any{d1, d2, nil, d2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_duration
func TestCaseUpdateDuration(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		d1, d2 := 24*time.Hour, 48*time.Hour
		assertCaseUpdate(t, db, "duration", caseOneTwo(d1, d2), []any{d1, d2, nil, d2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_email
func TestCaseUpdateEmail(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		e1, e2 := "1@example.com", "2@example.com"
		assertCaseUpdate(t, db, "email", caseOneTwo(orm.Value(e1), orm.Value(e2)).Default(orm.Value("")), []any{e1, e2, "", e2, "", "", ""})
	})
}

// No Go counterpart: tests/expressions_case/tests.py::CaseExpressionTests.test_update_file,
// test_update_file_path and test_update_image update a FileField, a
// FilePathField and an ImageField. The ORM has none of these fields.

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_float
func TestCaseUpdateFloat(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "float", caseOneTwo(1.1, 2.2), []any{1.1, 2.2, nil, 2.2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_generic_ip_address
//
// The values are netip.Addr, the Go type of the field, so the Case needs no
// output field. Values reads an IP address as text.
func TestCaseUpdateGenericIPAddress(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		a1, a2 := netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2.2.2.2")
		assertCaseUpdate(t, db, "generic_ip_address", caseOneTwo(a1, a2), []any{"1.1.1.1", "2.2.2.2", nil, "2.2.2.2", nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_null_boolean
func TestCaseUpdateNullBoolean(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "null_boolean", caseOneTwo(true, false), []any{true, false, nil, false, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_positive_big_integer
func TestCaseUpdatePositiveBigInteger(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "positive_big_integer", caseOneTwo(1, 2), []any{int64(1), int64(2), nil, int64(2), nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_positive_integer
func TestCaseUpdatePositiveInteger(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "positive_integer", caseOneTwo(1, 2), []any{int64(1), int64(2), nil, int64(2), nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_positive_small_integer
func TestCaseUpdatePositiveSmallInteger(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "positive_small_integer", caseOneTwo(1, 2), []any{int64(1), int64(2), nil, int64(2), nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_slug
func TestCaseUpdateSlug(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "slug", caseOneTwo(orm.Value("1"), orm.Value("2")).Default(orm.Value("")), []any{"1", "2", "", "2", "", "", ""})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_small_integer
func TestCaseUpdateSmallInteger(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "small_integer", caseOneTwo(1, 2), []any{int64(1), int64(2), nil, int64(2), nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_string
func TestCaseUpdateString(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("string__in", []string{"1", "2"})
		if _, err := qs.Update(context.Background(), "string", caseOneTwo(orm.Value("1"), orm.Value("2"))); err != nil {
			t.Fatalf("Update: %v", err)
		}
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "string"), caseRows([][]any{
			{1, "1"}, {2, "2"}, {2, "2"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_text
func TestCaseUpdateText(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		assertCaseUpdate(t, db, "text", caseOneTwo(orm.Value("1"), orm.Value("2")).Default(orm.Value("")), []any{"1", "2", "", "2", "", "", ""})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_time
func TestCaseUpdateTime(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		t1, t2 := orm.NewTime(1, 0, 0, 0), orm.NewTime(2, 0, 0, 0)
		assertCaseUpdate(t, db, "time", caseOneTwo(t1, t2), []any{t1, t2, nil, t2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_url
func TestCaseUpdateURL(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		u1, u2 := "http://1.example.com/", "http://2.example.com/"
		assertCaseUpdate(t, db, "url", caseOneTwo(orm.Value(u1), orm.Value(u2)).Default(orm.Value("")), []any{u1, u2, "", u2, "", "", ""})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_uuid
func TestCaseUpdateUUID(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		u1, u2 := orm.MustUUID("11111111111111111111111111111111"), orm.MustUUID("22222222222222222222222222222222")
		assertCaseUpdate(t, db, "uuid", caseOneTwo(u1, u2), []any{u1, u2, nil, u2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_update_fk
func TestCaseUpdateFK(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		objs := mustAll(t, orm.Query[caseTestModel](db).OrderBy("pk").Limit(2))
		pk1, pk2 := objs[0].ID, objs[1].ID
		assertCaseUpdate(t, db, "fk", caseOneTwo(pk1, pk2), []any{pk1, pk2, nil, pk2, nil, nil, nil})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_lookup_in_condition
func TestCaseLookupInCondition(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer__lt", 2), Then: orm.Value("less than 2")},
			orm.When{Cond: orm.Q("integer__gt", 2), Then: orm.Value("greater than 2")},
		).Default(orm.Value("equal to 2")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "test"), caseRows([][]any{
			{1, "less than 2"},
			{2, "equal to 2"},
			{3, "greater than 2"},
			{2, "equal to 2"},
			{3, "greater than 2"},
			{3, "greater than 2"},
			{4, "greater than 2"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_lookup_different_fields
func TestCaseLookupDifferentFields(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 2, "integer2", 3), Then: orm.Value("when")},
		).Default(orm.Value("default")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2", "test"), caseRows([][]any{
			{1, 1, "default"},
			{2, 3, "when"},
			{3, 4, "default"},
			{2, 2, "default"},
			{3, 4, "default"},
			{3, 3, "default"},
			{4, 5, "default"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_combined_q_object
func TestCaseCombinedQObject(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 2).Or(orm.Q("integer2", 3)), Then: orm.Value("when")},
		).Default(orm.Value("default")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("integer", "integer2", "test"), caseRows([][]any{
			{1, 1, "default"},
			{2, 3, "when"},
			{3, 4, "default"},
			{2, 2, "when"},
			{3, 4, "default"},
			{3, 3, "when"},
			{4, 5, "default"},
		}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_order_by_conditional_implicit
func TestCaseOrderByConditionalImplicit(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		qs := orm.Query[caseTestModel](db).Filter("integer__lte", 2).Annotate("test", orm.Case(
			orm.When{Cond: orm.Q("integer", 1), Then: 2},
			orm.When{Cond: orm.Q("integer", 2), Then: 1},
		).Default(3))
		assertValuesList(t, qs.OrderBy("test", "pk").ValuesList("integer", "test"), caseRows([][]any{
			{2, 1}, {2, 1}, {1, 2},
		}))
	})
}

// No Go counterpart: tests/expressions_case/tests.py::CaseExpressionTests.test_order_by_conditional_explicit
// orders by F("test").asc(). OrderBy takes keys, and the key "test" is
// test_order_by_conditional_implicit.

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_join_promotion
func TestCaseJoinPromotion(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		o := create(t, db, &caseTestModel{Integer: 1, Integer2: new(int64(1)), String: "1"})
		qs := orm.Query[caseTestModel](db).Filter("pk", o.ID)
		// The row o has no fk_rel row, so an inner join would remove it, and
		// the Case gives its default.
		foo := orm.Case(orm.When{Cond: orm.Q("fk_rel__pk", 1), Then: 2}).Default(3)
		assertValuesList(t, qs.Annotate("foo", foo).ValuesList("pk", "foo"), [][]any{{o.ID, int64(3)}})
		// The fk_rel of o is NULL.
		foo = orm.Case(orm.When{Cond: orm.Q("fk_rel__isnull", true), Then: 2}).Default(3)
		assertValuesList(t, qs.Annotate("foo", foo).ValuesList("pk", "foo"), [][]any{{o.ID, int64(2)}})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_join_promotion_multiple_annotations
func TestCaseJoinPromotionMultipleAnnotations(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		o := create(t, db, &caseTestModel{Integer: 1, Integer2: new(int64(1)), String: "1"})
		qs := orm.Query[caseTestModel](db).Filter("pk", o.ID)
		annotated := qs.Annotate(
			"foo", orm.Case(orm.When{Cond: orm.Q("fk_rel__pk", 1), Then: 2}).Default(3),
			"bar", orm.Case(orm.When{Cond: orm.Q("fk_rel__pk", 1), Then: 4}).Default(5),
		)
		assertValuesList(t, annotated.ValuesList("pk", "foo", "bar"), [][]any{{o.ID, int64(3), int64(5)}})
		annotated = qs.Annotate(
			"foo", orm.Case(orm.When{Cond: orm.Q("fk_rel__isnull", true), Then: 2}).Default(3),
			"bar", orm.Case(orm.When{Cond: orm.Q("fk_rel__isnull", true), Then: 4}).Default(5),
		)
		assertValuesList(t, annotated.ValuesList("pk", "foo", "bar"), [][]any{{o.ID, int64(2), int64(4)}})
	})
}

// caseCountNotOne returns the sum of 1 for each fk_rel row whose integer is
// not 1, and of 2 for the others, the cnt of Django's m2m tests.
func caseCountNotOne() orm.Expr {
	return orm.Sum(orm.Case(orm.When{Cond: orm.Not(orm.Q("fk_rel__integer", 1)), Then: 1}).Default(2))
}

// assertTail runs a ValuesList queryset and checks its rows without their
// first value, in order: Django's x[1:] transform.
func assertTail(t *testing.T, vl *orm.ValuesListQuerySet, want [][]any) {
	t.Helper()
	rows, err := vl.All(context.Background())
	if err != nil {
		t.Fatalf("ValuesList: %v", err)
	}
	got := make([][]any, len(rows))
	for i, row := range rows {
		got[i] = row[1:]
	}
	djangotest.AssertEqual(t, got, want)
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_m2m_exclude
//
// The row with integer 1 has a fk_rel row with integer 1 and gets the
// default 2. The others have two fk_rel rows, except the rows with integer 4
// and 10: the first has one, and the second none, which ~Q() matches as
// well.
func TestCaseM2MExclude(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		create(t, db, &caseTestModel{Integer: 10, Integer2: new(int64(1)), String: "1"})
		vl := orm.Query[caseTestModel](db).ValuesList("id", "integer").Annotate("cnt", caseCountNotOne()).OrderBy("integer")
		assertTail(t, vl, caseRows([][]any{{1, 2}, {2, 2}, {2, 2}, {3, 2}, {3, 2}, {3, 2}, {4, 1}, {10, 1}}))
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_m2m_reuse
func TestCaseM2MReuse(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		create(t, db, &caseTestModel{Integer: 10, Integer2: new(int64(1)), String: "1"})
		vl := orm.Query[caseTestModel](db).ValuesList("id", "integer").
			Annotate("cnt", caseCountNotOne()).
			Annotate("cnt2", caseCountNotOne()).
			OrderBy("integer")
		ctq := djangotest.CaptureQueries(db)
		assertTail(t, vl, caseRows([][]any{
			{1, 2, 2}, {2, 2, 2}, {2, 2, 2}, {3, 2, 2}, {3, 2, 2}, {3, 2, 2}, {4, 1, 1}, {10, 1, 1},
		}))
		ctq.Stop()
		djangotest.AssertEqual(t, strings.Count(ctq.Queries()[0].SQL, " JOIN "), 1)
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseExpressionTests.test_aggregation_empty_cases
//
// A Case without branches and default is NULL, which needs no output field.
func TestCaseAggregationEmptyCases(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseSetUp(t, db)
		for _, tc := range []struct {
			name  string
			c     orm.Expr
			value any
		}{
			{"empty cases and default", orm.Case(), nil},
			{"empty cases and a constant default", orm.Case().Default(orm.Value("empty")), "empty"},
			{"empty cases and column in the default", orm.Case().Default(orm.F("url")), ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				vs := orm.Query[caseTestModel](db).Values("string").
					Annotate("case", tc.c, "integer_sum", orm.Sum("integer")).
					OrderBy("string")
				assertProjected(t, vs, []string{"string", "case", "integer_sum"}, caseRows([][]any{
					{"1", tc.value, 1}, {"2", tc.value, 4}, {"3", tc.value, 9}, {"4", tc.value, 4},
				}))
			})
		}
	})
}

// caseDocumentationSetUp creates the clients of
// CaseDocumentationExamples.setUpTestData.
func caseDocumentationSetUp(t *testing.T, db *orm.DB) {
	t.Helper()
	create(t, db, &caseClient{Name: "Jane Doe", AccountType: "R", RegisteredOn: caseDaysAgo(36)})
	create(t, db, &caseClient{Name: "James Smith", AccountType: "G", RegisteredOn: caseDaysAgo(5)})
	create(t, db, &caseClient{Name: "Jack Black", AccountType: "P", RegisteredOn: caseDaysAgo(10 * 365)})
}

// caseDaysAgo returns the date days before today.
func caseDaysAgo(days int) orm.Date {
	return orm.DateFromTime(time.Now().AddDate(0, 0, -days))
}

// Ported from Django: tests/expressions_case/tests.py::CaseDocumentationExamples.test_simple_example
func TestCaseDocumentationSimpleExample(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseDocumentationSetUp(t, db)
		qs := orm.Query[caseClient](db).Annotate("discount", orm.Case(
			orm.When{Cond: orm.Q("account_type", "G"), Then: orm.Value("5%")},
			orm.When{Cond: orm.Q("account_type", "P"), Then: orm.Value("10%")},
		).Default(orm.Value("0%")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("name", "discount"), [][]any{
			{"Jane Doe", "0%"}, {"James Smith", "5%"}, {"Jack Black", "10%"},
		})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseDocumentationExamples.test_lookup_example
func TestCaseDocumentationLookupExample(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseDocumentationSetUp(t, db)
		aMonthAgo, aYearAgo := caseDaysAgo(30), caseDaysAgo(365)
		qs := orm.Query[caseClient](db).Annotate("discount", orm.Case(
			orm.When{Cond: orm.Q("registered_on__lte", aYearAgo), Then: orm.Value("10%")},
			orm.When{Cond: orm.Q("registered_on__lte", aMonthAgo), Then: orm.Value("5%")},
		).Default(orm.Value("0%")))
		assertValuesList(t, qs.OrderBy("pk").ValuesList("name", "discount"), [][]any{
			{"Jane Doe", "5%"}, {"James Smith", "0%"}, {"Jack Black", "10%"},
		})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseDocumentationExamples.test_conditional_update_example
func TestCaseDocumentationConditionalUpdateExample(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseDocumentationSetUp(t, db)
		aMonthAgo, aYearAgo := caseDaysAgo(30), caseDaysAgo(365)
		_, err := orm.Query[caseClient](db).Update(context.Background(), "account_type", orm.Case(
			orm.When{Cond: orm.Q("registered_on__lte", aYearAgo), Then: orm.Value("P")},
			orm.When{Cond: orm.Q("registered_on__lte", aMonthAgo), Then: orm.Value("G")},
		).Default(orm.Value("R")))
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		assertValuesList(t, orm.Query[caseClient](db).OrderBy("pk").ValuesList("name", "account_type"), [][]any{
			{"Jane Doe", "G"}, {"James Smith", "R"}, {"Jack Black", "P"},
		})
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseDocumentationExamples.test_conditional_aggregation_example
func TestCaseDocumentationConditionalAggregationExample(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		caseDocumentationSetUp(t, db)
		today := caseDaysAgo(0)
		create(t, db, &caseClient{Name: "Jean Grey", AccountType: "R", RegisteredOn: today})
		create(t, db, &caseClient{Name: "James Bond", AccountType: "P", RegisteredOn: today})
		create(t, db, &caseClient{Name: "Jane Porter", AccountType: "P", RegisteredOn: today})
		want := map[string]any{"regular": int64(2), "gold": int64(1), "platinum": int64(3)}
		got, err := orm.Query[caseClient](db).Aggregate(ctx,
			"regular", orm.Count("pk").Filter("account_type", "R"),
			"gold", orm.Count("pk").Filter("account_type", "G"),
			"platinum", orm.Count("pk").Filter("account_type", "P"),
		)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, got, want)
		// The example before aggregates had a filter.
		got, err = orm.Query[caseClient](db).Aggregate(ctx,
			"regular", orm.Sum(orm.Case(orm.When{Cond: orm.Q("account_type", "R"), Then: 1})),
			"gold", orm.Sum(orm.Case(orm.When{Cond: orm.Q("account_type", "G"), Then: 1})),
			"platinum", orm.Sum(orm.Case(orm.When{Cond: orm.Q("account_type", "P"), Then: 1})),
		)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, got, want)
	})
}

// Ported from Django: tests/expressions_case/tests.py::CaseDocumentationExamples.test_filter_example
func TestCaseDocumentationFilterExample(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		caseDocumentationSetUp(t, db)
		aMonthAgo, aYearAgo := caseDaysAgo(30), caseDaysAgo(365)
		qs := orm.Query[caseClient](db).Filter("registered_on__lte", orm.Case(
			orm.When{Cond: orm.Q("account_type", "G"), Then: aMonthAgo},
			orm.When{Cond: orm.Q("account_type", "P"), Then: aYearAgo},
		))
		assertValuesList(t, qs.ValuesList("name", "account_type"), [][]any{{"Jack Black", "P"}})
	})
}

// No Go counterpart: tests/expressions_case/tests.py::CaseDocumentationExamples.test_hash
// hashes Case expressions. An Expr is not hashable.

// No Go counterpart: tests/expressions_case/tests.py::CaseWhenTests.test_only_when_arguments
// and test_invalid_when_constructor_args pass Case a value that is not a
// When, and When a condition that is not a Q object or lookups. Case takes
// When values and When.Cond is a Cond, so neither compiles; a When without a
// condition has the empty Cond of test_empty_q_object.

// Ported from Django: tests/expressions_case/tests.py::CaseWhenTests.test_empty_q_object
//
// Django refuses the When when it is built, the ORM when the query is.
func TestCaseWhenEmptyQObject(t *testing.T) {
	djangotest.EachDB(t, caseModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[caseTestModel](db).Annotate("test", orm.Case(orm.When{Cond: orm.Q(), Then: orm.Value(true)}))
		_, err := qs.All(context.Background())
		assertErrorContains(t, err, "an empty Q() can't be used as a When() condition")
	})
}
