package orm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// expPKs runs a Values("pk") queryset and returns the primary keys.
func expPKs(t *testing.T, vs *orm.ValuesQuerySet) []int64 {
	t.Helper()
	rows, err := vs.All(context.Background())
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	pks := make([]int64, len(rows))
	for i, row := range rows {
		pks[i] = row["pk"].(int64)
	}
	return pks
}

// expEmployeeIDs runs an employee queryset and returns the primary keys.
func expEmployeeIDs(t *testing.T, qs *orm.QuerySet[expEmployee]) []int64 {
	t.Helper()
	rows := mustAll(t, qs)
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}

// expMustExist runs Exists on qs and checks the result.
func expMustExist[T any](t *testing.T, qs *orm.QuerySet[T], want bool) {
	t.Helper()
	got, err := qs.Exists(context.Background())
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if got != want {
		t.Errorf("Exists() = %v, want %v", got, want)
	}
}

// expGetEmployee returns the employee with the given first name.
func expGetEmployee(t *testing.T, db *orm.DB, firstname string) expEmployee {
	t.Helper()
	e, err := orm.Query[expEmployee](db).Get(context.Background(), "firstname", firstname)
	if err != nil {
		t.Fatalf("Get %s: %v", firstname, err)
	}
	return e
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_order_by_exists
func TestExpressionsOrderByExists(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		mary := create(t, db, &expEmployee{Firstname: "Mary", Lastname: "Mustermann", Salary: 20})
		qs := orm.Query[expEmployee](db).Filter("lastname", "Mustermann").OrderBy(
			// Order by whether the employee is the CEO of a company.
			orm.Exists(orm.Query[expCompany](db).Filter("ceo", orm.OuterRef("pk"))).Desc(),
		)
		djangotest.AssertQuerySetEqual(t, qs, []int64{fx.max.ID, mary.ID}, expEmployeeID)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_outerref
func TestExpressionsOuterRef(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		inner := orm.Query[expCompany](db).Filter("point_of_contact", orm.OuterRef("pk"))
		_, err := inner.Exists(context.Background())
		assertErrorContains(t, err,
			"this queryset contains a reference to an outer query and may only be used in a subquery")

		outer := orm.Query[expEmployee](db).Annotate("is_point_of_contact", orm.Exists(inner))
		expMustExist(t, outer, true)
	})
}

// Django checks that the output field of Exists is a BooleanField. The port
// reads the annotation back and checks that it is a bool.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_exist_single_field_output_field
func TestExpressionsExistSingleFieldOutputField(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		queryset := orm.Query[expCompany](db).Values("pk")
		got := firstValue(t, orm.Query[expEmployee](db).
			Annotate("exists", orm.Exists(queryset)).ValuesList("exists"))
		if got != true {
			t.Errorf("Exists annotation = %#v, want true", got)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_subquery
func TestExpressionsSubquery(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		joe := expGetEmployee(t, db, "Joe")
		expUpdate(t, orm.Query[expCompany](db).Filter("name", "Example Inc."), 1,
			"point_of_contact", joe.ID, "ceo", fx.max.ID)
		create(t, db, &expEmployee{Firstname: "Bob", Lastname: "Brown", Salary: 40})
		isPointOfContact := orm.Exists(orm.Query[expCompany](db).Filter("point_of_contact", orm.OuterRef("pk")))
		isCEOOfSmallCompany := orm.Exists(orm.Query[expCompany](db).
			Filter("num_employees__lt", 200, "ceo", orm.OuterRef("pk")))
		qs := orm.Query[expEmployee](db).Annotate(
			"is_point_of_contact", isPointOfContact,
			"is_not_point_of_contact", isPointOfContact.Not(),
			"is_ceo_of_small_company", isCEOOfSmallCompany,
			"is_ceo_small_2", isCEOOfSmallCompany.Not().Not(),
			"largest_company", orm.ExpressionWrapper(orm.Subquery(
				orm.Query[expCompany](db).OrderBy("-num_employees").
					Filter(orm.Q("ceo", orm.OuterRef("pk")).Or(orm.Q("point_of_contact", orm.OuterRef("pk")))).
					Values("name").Limit(1),
			), orm.Field{Kind: orm.KindChar}),
		).Values(
			"firstname",
			"is_point_of_contact",
			"is_not_point_of_contact",
			"is_ceo_of_small_company",
			"is_ceo_small_2",
			"largest_company",
		).OrderBy("firstname")
		assertValues(t, qs, []map[string]any{
			{
				"firstname":               "Bob",
				"is_point_of_contact":     false,
				"is_not_point_of_contact": true,
				"is_ceo_of_small_company": false,
				"is_ceo_small_2":          false,
				"largest_company":         nil,
			},
			{
				"firstname":               "Frank",
				"is_point_of_contact":     false,
				"is_not_point_of_contact": true,
				"is_ceo_of_small_company": true,
				"is_ceo_small_2":          true,
				"largest_company":         "Foobar Ltd.",
			},
			{
				"firstname":               "Joe",
				"is_point_of_contact":     true,
				"is_not_point_of_contact": false,
				"is_ceo_of_small_company": false,
				"is_ceo_small_2":          false,
				"largest_company":         "Example Inc.",
			},
			{
				"firstname":               "Max",
				"is_point_of_contact":     false,
				"is_not_point_of_contact": true,
				"is_ceo_of_small_company": true,
				"is_ceo_small_2":          true,
				"largest_company":         "Example Inc.",
			},
		})
		// A less direct way to write the same query: this uses a LEFT OUTER
		// JOIN and an IS NULL, inside a WHERE NOT IN which is probably less
		// efficient than EXISTS.
		djangotest.AssertSequenceEqual(t,
			expPKs(t, qs.Filter("is_point_of_contact", true).Values("pk")),
			expPKs(t, orm.Query[expEmployee](db).Exclude("company_point_of_contact_set", nil).Values("pk")),
			djangotest.Unordered())
	})
}

// No Go counterpart: test_subquery_eq compares expression objects with ==;
// Go expressions have no equality method.

// Django also checks the subquery flag of the query behind Subquery, which is
// internal here. The port compiles a query that annotates the subquery.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_subquery_sql
func TestExpressionsSubquerySQL(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		employees := orm.Query[expEmployee](db)
		sql, _, err := orm.CompileSelectForTest(orm.Query[expEmployee](db).
			Annotate("sub", orm.Subquery(employees)))
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if !strings.Contains(sql, "(SELECT ") {
			t.Errorf("SQL %q does not contain %q", sql, "(SELECT ")
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_in_subquery
func TestExpressionsInSubquery(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		// This is a contrived test (and you really wouldn't write this query),
		// but it is a succinct way to test the __in=Subquery() construct.
		smallCompanies := orm.Query[expCompany](db).Filter("num_employees__lt", 200).Values("pk")
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expCompany](db).Filter("pk__in", orm.Subquery(smallCompanies)),
			[]string{"Foobar Ltd.", "Test GmbH"}, expCompanyName, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[expCompany](db).Filter("pk",
				orm.Subquery(smallCompanies.Filter("num_employees", 3).Limit(1))),
			[]string{"Foobar Ltd."}, expCompanyName, djangotest.Unordered())
	})
}

// The lookups are those of Django's CharField without isnull, whose right-hand
// side must be a literal boolean.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_lookups_subquery
func TestExpressionsLookupsSubquery(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		smallestCompany := orm.Query[expCompany](db).OrderBy("num_employees").Values("name").Limit(1)
		lookups := []string{
			"exact", "iexact", "gt", "gte", "lt", "lte", "in", "contains", "icontains",
			"startswith", "istartswith", "endswith", "iendswith", "range", "regex", "iregex",
		}
		for _, lookup := range lookups {
			t.Run(lookup, func(t *testing.T) {
				var rhs any = orm.Subquery(smallestCompany)
				if lookup == "range" {
					rhs = []any{orm.Subquery(smallestCompany), orm.Subquery(smallestCompany)}
				}
				if n := mustCount(t, orm.Query[expCompany](db).Filter("name__"+lookup, rhs)); n == 0 {
					t.Errorf("name__%s matched no company", lookup)
				}
			})
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_uuid_pk_subquery
func TestExpressionsUUIDPKSubquery(t *testing.T) {
	t.Skip("needs a foreign key to a UUID primary key, which the ORM does not have")
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_nested_subquery
func TestExpressionsNestedSubquery(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		ctx := context.Background()
		inner := orm.Query[expCompany](db).Filter("point_of_contact", orm.OuterRef("pk"))
		outer := orm.Query[expEmployee](db).Annotate("is_point_of_contact", orm.Exists(inner))
		contrived := orm.Query[expEmployee](db).Annotate(
			"is_point_of_contact", orm.ExpressionWrapper(orm.Subquery(
				outer.Filter("pk", orm.OuterRef("pk")).Values("is_point_of_contact"),
			), orm.Field{Kind: orm.KindBool}),
		)
		got, err := contrived.ValuesList().All(ctx)
		if err != nil {
			t.Fatalf("contrived: %v", err)
		}
		want, err := outer.ValuesList().All(ctx)
		if err != nil {
			t.Fatalf("outer: %v", err)
		}
		djangotest.AssertSequenceEqual(t, got, want, djangotest.Unordered())
	})
}

// The port orders by pk, the order Django gets from the table without an ORDER
// BY.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_nested_subquery_join_outer_ref
func TestExpressionsNestedSubqueryJoinOuterRef(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		inner := orm.Query[expEmployee](db).Filter("pk", orm.OuterRef("ceo__pk")).Values("pk")
		qs := orm.Query[expEmployee](db).Annotate(
			"ceo_company", orm.Subquery(orm.Query[expCompany](db).Filter(
				"ceo__in", inner,
				"ceo__pk", orm.OuterRef("pk"),
			).Values("pk")),
		)
		assertFlat(t, qs.OrderBy("pk").ValuesList("ceo_company"),
			[]any{fx.exampleInc.ID, fx.foobarLtd.ID, fx.gmbh.ID})
	})
}

// expCreateTime creates a Time row at the given time of day.
func expCreateTime(t *testing.T, db *orm.DB, s string) *expTime {
	t.Helper()
	return create(t, db, &expTime{Time: new(orm.MustTime(s))})
}

// expTimeID is the transform that compares Time rows by primary key.
func expTimeID(x expTime) int64 { return x.ID }

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_nested_subquery_outer_ref_2
func TestExpressionsNestedSubqueryOuterRef2(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		first := expCreateTime(t, db, "09:00")
		second := expCreateTime(t, db, "17:00")
		third := expCreateTime(t, db, "21:00")
		runs := []*expSimulationRun{
			{Start: fkTo[expTime](first.ID), End: fkTo[expTime](second.ID), Midpoint: orm.MustTime("12:00")},
			{Start: fkTo[expTime](first.ID), End: fkTo[expTime](third.ID), Midpoint: orm.MustTime("15:00")},
			{Start: fkTo[expTime](second.ID), End: fkTo[expTime](first.ID), Midpoint: orm.MustTime("00:00")},
		}
		if err := orm.Query[expSimulationRun](db).BulkCreate(context.Background(), runs); err != nil {
			t.Fatalf("BulkCreate: %v", err)
		}
		inner := orm.Query[expTime](db).Filter(
			"time", orm.OuterRef(orm.OuterRef("time")), "pk", orm.OuterRef("start"),
		).Values("time")
		middle := orm.Query[expSimulationRun](db).Annotate("other", orm.Subquery(inner)).
			Values("other").Limit(1)
		outer := orm.Query[expTime](db).Annotate("other",
			orm.ExpressionWrapper(orm.Subquery(middle), orm.Field{Kind: orm.KindTime}))
		// This is a contrived example. It exercises the double OuterRef form.
		djangotest.AssertQuerySetEqual(t, outer, []int64{first.ID, second.ID, third.ID},
			expTimeID, djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_nested_subquery_outer_ref_with_autofield
func TestExpressionsNestedSubqueryOuterRefWithAutofield(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		first := expCreateTime(t, db, "09:00")
		second := expCreateTime(t, db, "17:00")
		create(t, db, &expSimulationRun{
			Start: fkTo[expTime](first.ID), End: fkTo[expTime](second.ID), Midpoint: orm.MustTime("12:00"),
		})
		inner := orm.Query[expSimulationRun](db).Filter("start", orm.OuterRef(orm.OuterRef("pk"))).
			Values("start")
		middle := orm.Query[expTime](db).Annotate("other", orm.Subquery(inner)).Values("other").Limit(1)
		outer := orm.Query[expTime](db).Annotate("other",
			orm.ExpressionWrapper(orm.Subquery(middle), orm.Field{Kind: orm.KindInt}))
		// This exercises the double OuterRef form with AutoField as pk.
		djangotest.AssertQuerySetEqual(t, outer, []int64{first.ID, second.ID},
			expTimeID, djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotations_within_subquery
func TestExpressionsAnnotationsWithinSubquery(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		frank := expGetEmployee(t, db, "Frank")
		expUpdate(t, orm.Query[expCompany](db).Filter("num_employees__lt", 50), 2, "ceo", frank.ID)
		inner := orm.Query[expCompany](db).Filter("ceo", orm.OuterRef("pk")).
			Values("ceo").
			Annotate("total_employees", orm.Sum("num_employees")).
			Values("total_employees")
		outer := orm.Query[expEmployee](db).Annotate("total_employees", orm.Subquery(inner)).
			Filter("salary__lte", orm.Subquery(inner))
		assertValues(t, outer.OrderBy("-total_employees").Values("salary", "total_employees"),
			[]map[string]any{
				{"salary": int64(10), "total_employees": int64(2300)},
				{"salary": int64(20), "total_employees": int64(35)},
			})
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_subquery_references_joined_table_twice
func TestExpressionsSubqueryReferencesJoinedTableTwice(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		inner := orm.Query[expCompany](db).Filter(
			"num_chairs__gte", orm.OuterRef("ceo__salary"),
			"num_employees__gte", orm.OuterRef("point_of_contact__salary"),
		)
		// Another contrived example (there is no need to have a subquery here).
		outer := orm.Query[expCompany](db).Filter("pk__in", orm.Subquery(inner.Values("pk")))
		expMustExist(t, outer, false)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_subquery_filter_by_aggregate
func TestExpressionsSubqueryFilterByAggregate(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		create(t, db, &expNumber{Integer: 1000, Float: new(1.2)})
		create(t, db, &expEmployee{Salary: 1000})
		qs := orm.Query[expNumber](db).Annotate(
			"min_valuable_count", orm.Subquery(
				orm.Query[expEmployee](db).Filter("salary", orm.OuterRef("integer")).
					Annotate("cnt", orm.Count("salary")).
					Filter("cnt__gt", 0).
					Values("cnt").Limit(1),
			),
		)
		got, err := qs.Get(context.Background())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Float == nil || *got.Float != 1.2 {
			t.Errorf("float = %v, want 1.2", got.Float)
		}
	})
}

// No Go counterpart: test_subquery_filter_by_lazy filters by a
// SimpleLazyObject, a Python proxy that Go has no use for.

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_aggregate_subquery_annotation
func TestExpressionsAggregateSubqueryAnnotation(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		capture := djangotest.CaptureQueries(db)
		aggregate, err := orm.Query[expCompany](db).Annotate(
			"ceo_salary", orm.Subquery(
				orm.Query[expEmployee](db).Filter("id", orm.OuterRef("ceo_id")).Values("salary"),
			),
		).Aggregate(context.Background(),
			"ceo_salary_gt_20", orm.Count("pk").Filter("ceo_salary__gt", 20),
		)
		capture.Stop()
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if aggregate["ceo_salary_gt_20"] != int64(1) {
			t.Errorf("ceo_salary_gt_20 = %#v, want 1", aggregate["ceo_salary_gt_20"])
		}
		queries := capture.Queries()
		if len(queries) != 1 {
			t.Fatalf("ran %d queries, want 1", len(queries))
		}
		// Aggregation over a subquery annotation doesn't annotate the subquery
		// twice.
		sql := queries[0].SQL
		if n := strings.Count(sql, "SELECT"); n != 3 {
			t.Errorf("SQL has %d SELECTs, want 3: %s", n, sql)
		}
		// GROUP BY isn't required to aggregate over a query that doesn't
		// contain nested aggregates.
		if strings.Contains(sql, "GROUP BY") {
			t.Errorf("SQL has a GROUP BY: %s", sql)
		}
	})
}

// Django passes the subquery to objects.create. A Go struct field cannot hold
// an expression, so the port creates the company and then sets the field with
// Update, which runs the same subquery.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_object_create_with_f_expression_in_subquery
func TestExpressionsObjectCreateWithFExpressionInSubquery(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		ceo := fkTo[expEmployee](fx.max.ID)
		create(t, db, &expCompany{Name: "Big company", NumEmployees: 100000, NumChairs: 1, CEO: ceo})
		biggest := create(t, db, &expCompany{Name: "Biggest company", NumChairs: 1, CEO: ceo})
		expUpdate(t, orm.Query[expCompany](db).Filter("pk", biggest.ID), 1,
			"num_employees", orm.Subquery(
				orm.Query[expCompany](db).OrderBy("-num_employees").
					Annotate("max_num_employees", orm.Max("num_employees")).
					Annotate("new_num_employees", orm.F("max_num_employees").Add(1)).
					Values("new_num_employees").Limit(1),
			))
		if err := orm.Query[expCompany](db).RefreshFromDB(context.Background(), biggest); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		if biggest.NumEmployees != 100001 {
			t.Errorf("num_employees = %d, want 100001", biggest.NumEmployees)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_outerref_mixed_case_table_name
func TestExpressionsOuterRefMixedCaseTableName(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		inner := orm.Query[expResult](db).Filter("result_time__gte", orm.OuterRef("experiment__assigned"))
		outer := orm.Query[expResult](db).Filter("pk__in", orm.Subquery(inner.Values("pk")))
		expMustExist(t, outer, false)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_outerref_with_operator
func TestExpressionsOuterRefWithOperator(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		inner := orm.Query[expCompany](db).Filter("num_employees", orm.OuterRef("ceo__salary").Add(2))
		outer := orm.Query[expCompany](db).Filter("pk__in", orm.Subquery(inner.Values("pk")))
		got, err := outer.Get(context.Background())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Name != "Test GmbH" {
			t.Errorf("name = %q, want %q", got.Name, "Test GmbH")
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_nested_outerref_with_function
func TestExpressionsNestedOuterRefWithFunction(t *testing.T) {
	t.Skip("needs the Left function, which the ORM does not have")
}

// expMaxCEOSalaryRaise annotates each company with the largest sum of its own
// employee count and another company's, the query of Django's
// test_annotation_with_outerref.
func expMaxCEOSalaryRaise(db *orm.DB) orm.Expr {
	return orm.Subquery(
		orm.Query[expCompany](db).Annotate(
			"salary_raise", orm.OuterRef("num_employees").Add(orm.F("num_employees")),
		).OrderBy("-salary_raise").Values("salary_raise").Limit(1),
	)
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotation_with_outerref
func TestExpressionsAnnotationWithOuterRef(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		got := firstValue(t, orm.Query[expCompany](db).
			Annotate("max_ceo_salary_raise", expMaxCEOSalaryRaise(db)).
			Filter("pk", fx.gmbh.ID).ValuesList("max_ceo_salary_raise"))
		if got != int64(2332) {
			t.Errorf("max_ceo_salary_raise = %#v, want 2332", got)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotation_with_outerref_and_output_field
func TestExpressionsAnnotationWithOuterRefAndOutputField(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		got := firstValue(t, orm.Query[expCompany](db).
			Annotate("max_ceo_salary_raise",
				orm.ExpressionWrapper(expMaxCEOSalaryRaise(db), orm.Field{Kind: orm.KindDecimal})).
			Filter("pk", fx.gmbh.ID).ValuesList("max_ceo_salary_raise"))
		d, ok := got.(orm.Decimal)
		if !ok {
			t.Fatalf("max_ceo_salary_raise = %#v, want an orm.Decimal", got)
		}
		if !d.Equal(orm.DecimalFromInt(2332)) {
			t.Errorf("max_ceo_salary_raise = %s, want 2332", d)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotation_with_nested_outerref
func TestExpressionsAnnotationWithNestedOuterRef(t *testing.T) {
	t.Skip("needs the Left function, which the ORM does not have")
}

// Django also makes the new manager the manager of Max, which the query does
// not read; Employee has no manager field here.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_annotation_with_deeply_nested_outerref
func TestExpressionsAnnotationWithDeeplyNestedOuterRef(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		bob := create(t, db, &expEmployee{Firstname: "Bob", BasedInEU: true})
		create(t, db, &expManager{Name: "Rock", Secretary: fkTo[expEmployee](bob.ID)})
		qs := orm.Query[expEmployee](db).Filter(
			orm.Exists(orm.Query[expManager](db).Filter(
				orm.Exists(orm.Query[expEmployee](db).Filter("pk", orm.OuterRef("secretary__pk")).
					Annotate("secretary_based_in_eu", orm.OuterRef(orm.OuterRef("based_in_eu"))).
					Filter(orm.Exists(orm.Query[expCompany](db).Filter(
						// Inner OuterRef refers to an outer OuterRef (not
						// ResolvedOuterRef).
						"based_in_eu", orm.OuterRef("secretary_based_in_eu"),
					))),
				),
				"secretary__pk", orm.OuterRef("pk"),
			)),
		)
		got, err := qs.Get(context.Background())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != bob.ID {
			t.Errorf("got employee %d, want %d", got.ID, bob.ID)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_exists_in_filter
func TestExpressionsExistsInFilter(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		inner := orm.Query[expCompany](db).Filter("ceo", orm.OuterRef("pk")).Values("pk")
		qs1 := orm.Query[expEmployee](db).Filter(orm.Exists(inner))
		qs2 := orm.Query[expEmployee](db).Annotate("found", orm.Exists(inner)).Filter("found", true)
		djangotest.AssertSequenceEqual(t, expEmployeeIDs(t, qs1), expEmployeeIDs(t, qs2), djangotest.Unordered())
		expMustExist(t, orm.Query[expEmployee](db).Exclude(orm.Exists(inner)), false)
		djangotest.AssertSequenceEqual(t, expEmployeeIDs(t, qs2),
			expEmployeeIDs(t, orm.Query[expEmployee](db).Exclude(orm.Exists(inner).Not())),
			djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_subquery_in_filter
func TestExpressionsSubqueryInFilter(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		inner := orm.Query[expCompany](db).Filter("ceo", orm.OuterRef("pk")).Values("based_in_eu")
		djangotest.AssertQuerySetEqual(t, orm.Query[expEmployee](db).Filter(orm.Subquery(inner)),
			[]int64{fx.foobarLtd.CEO.PK()}, expEmployeeID)
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_subquery_group_by_outerref_in_filter
func TestExpressionsSubqueryGroupByOuterRefInFilter(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		expSetUpBasic(t, db)
		inner := orm.Query[expCompany](db).
			Annotate("employee", orm.OuterRef("pk")).
			Values("employee").
			Annotate("min_num_chairs", orm.Min("num_chairs")).
			Values("ceo")
		expMustExist(t, orm.Query[expEmployee](db).Filter("pk__in", orm.Subquery(inner)), true)
	})
}

// Django passes output_field=BooleanField() to Case; ExpressionWrapper gives
// the Case the same output field here.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_case_in_filter_if_boolean_output_field
func TestExpressionsCaseInFilterIfBooleanOutputField(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		isCEO := orm.Query[expCompany](db).Filter("ceo", orm.OuterRef("pk"))
		isPOC := orm.Query[expCompany](db).Filter("point_of_contact", orm.OuterRef("pk"))
		qs := orm.Query[expEmployee](db).Filter(orm.ExpressionWrapper(
			orm.Case(
				orm.When{Cond: orm.Q(orm.Exists(isCEO)), Then: true},
				orm.When{Cond: orm.Q(orm.Exists(isPOC)), Then: true},
			).Default(false),
			orm.Field{Kind: orm.KindBool},
		))
		djangotest.AssertQuerySetEqual(t, qs,
			[]int64{fx.exampleInc.CEO.PK(), fx.foobarLtd.CEO.PK(), fx.max.ID},
			expEmployeeID, djangotest.Unordered())
	})
}

// expSetGmbhPointOfContact makes Max the point of contact of Test GmbH.
func expSetGmbhPointOfContact(t *testing.T, db *orm.DB, fx expBasic) {
	t.Helper()
	fx.gmbh.PointOfContact = fkTo[expEmployee](fx.max.ID)
	if err := orm.Query[expCompany](db).Save(context.Background(), fx.gmbh); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// Django combines Exists with | and &, which turns each operand into a Q. Go
// has no operator overloading, so the port writes the Q objects out.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_boolean_expression_combined
func TestExpressionsBooleanExpressionCombined(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		isCEO := orm.Query[expCompany](db).Filter("ceo", orm.OuterRef("pk"))
		isPOC := orm.Query[expCompany](db).Filter("point_of_contact", orm.OuterRef("pk"))
		expSetGmbhPointOfContact(t, db, fx)
		joe, frank := fx.exampleInc.CEO.PK(), fx.foobarLtd.CEO.PK()
		employees := orm.Query[expEmployee](db)
		cases := []struct {
			cond orm.Cond
			want []int64
		}{
			{orm.Q(orm.Exists(isCEO)).Or(orm.Q(orm.Exists(isPOC))), []int64{joe, frank, fx.max.ID}},
			{orm.Q(orm.Exists(isCEO)).And(orm.Q(orm.Exists(isPOC))), []int64{fx.max.ID}},
			{orm.Q(orm.Exists(isCEO)).And(orm.Q("salary__gte", 30)), []int64{fx.max.ID}},
			{orm.Q(orm.Exists(isPOC)).Or(orm.Q("salary__lt", 15)), []int64{joe, fx.max.ID}},
			{orm.Q("salary__gte", 30).And(orm.Q(orm.Exists(isCEO))), []int64{fx.max.ID}},
			{orm.Q("salary__lt", 15).Or(orm.Q(orm.Exists(isPOC))), []int64{joe, fx.max.ID}},
		}
		for _, c := range cases {
			djangotest.AssertQuerySetEqual(t, employees.Filter(c.cond), c.want, expEmployeeID,
				djangotest.Unordered())
		}
	})
}

// Django's eight combinations reduce to four in Go: Exists(x) & Q() and
// Q(Exists(x)) & Q() are both Q(Exists(x)).And(Q()) here.
//
// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_boolean_expression_combined_with_empty_Q
func TestExpressionsBooleanExpressionCombinedWithEmptyQ(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		isPOC := orm.Query[expCompany](db).Filter("point_of_contact", orm.OuterRef("pk"))
		expSetGmbhPointOfContact(t, db, fx)
		tests := []orm.Cond{
			orm.Q(orm.Exists(isPOC)).And(orm.Q()),
			orm.Q().And(orm.Q(orm.Exists(isPOC))),
			orm.Q(orm.Exists(isPOC)).Or(orm.Q()),
			orm.Q().Or(orm.Q(orm.Exists(isPOC))),
		}
		for _, conditions := range tests {
			t.Run(conditions.String(), func(t *testing.T) {
				djangotest.AssertQuerySetEqual(t, orm.Query[expEmployee](db).Filter(conditions),
					[]int64{fx.max.ID}, expEmployeeID, djangotest.Unordered())
			})
		}
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_boolean_expression_in_Q
func TestExpressionsBooleanExpressionInQ(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		fx := expSetUpBasic(t, db)
		isPOC := orm.Query[expCompany](db).Filter("point_of_contact", orm.OuterRef("pk"))
		expSetGmbhPointOfContact(t, db, fx)
		djangotest.AssertQuerySetEqual(t, orm.Query[expEmployee](db).Filter(orm.Q(orm.Exists(isPOC))),
			[]int64{fx.max.ID}, expEmployeeID, djangotest.Unordered())
	})
}

// Ported from Django: tests/expressions/tests.py::BasicExpressionsTests.test_slicing_of_outerref
func TestExpressionsSlicingOfOuterRef(t *testing.T) {
	t.Skip("needs slicing of F and OuterRef, which the ORM does not have")
}

// Ported from Django: tests/expressions/tests.py::ExistsTests.test_optimizations
func TestExistsOptimizations(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		capture := djangotest.CaptureQueries(db)
		_, err := orm.Query[expExperiment](db).
			Annotate("exists", orm.Exists(orm.Query[expExperiment](db).OrderBy("pk"))).
			Values("exists").OrderBy().All(context.Background())
		capture.Stop()
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		queries := capture.Queries()
		if len(queries) != 1 {
			t.Fatalf("ran %d queries, want 1", len(queries))
		}
		sql := queries[0].SQL
		if strings.Contains(sql, `"id"`) {
			t.Errorf("SQL selects the primary key: %s", sql)
		}
		if !strings.Contains(sql, "LIMIT 1") {
			t.Errorf("SQL has no LIMIT 1: %s", sql)
		}
		if strings.Contains(sql, "ORDER BY") {
			t.Errorf("SQL has an ORDER BY: %s", sql)
		}
	})
}

// expManagerID is the transform that compares managers by primary key.
func expManagerID(m expManager) int64 { return m.ID }

// Ported from Django: tests/expressions/tests.py::ExistsTests.test_negated_empty_exists
func TestExistsNegatedEmptyExists(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		manager := create(t, db, &expManager{})
		qs := orm.Query[expManager](db).Filter(
			orm.Q(orm.Exists(orm.Query[expManager](db).None()).Not()).And(orm.Q("pk", manager.ID)),
		)
		djangotest.AssertQuerySetEqual(t, qs, []int64{manager.ID}, expManagerID)
	})
}

// Ported from Django: tests/expressions/tests.py::ExistsTests.test_select_negated_empty_exists
func TestExistsSelectNegatedEmptyExists(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		manager := create(t, db, &expManager{})
		qs := orm.Query[expManager](db).Annotate(
			"not_exists", orm.Exists(orm.Query[expManager](db).None()).Not(),
		).Filter("pk", manager.ID)
		djangotest.AssertQuerySetEqual(t, qs, []int64{manager.ID}, expManagerID)
		if got := firstValue(t, qs.ValuesList("not_exists")); got != true {
			t.Errorf("not_exists = %#v, want true", got)
		}
	})
}

// Ported from Django: tests/expressions/tests.py::ExistsTests.test_filter_by_empty_exists
func TestExistsFilterByEmptyExists(t *testing.T) {
	djangotest.EachDB(t, expModels(), func(t *testing.T, db *orm.DB) {
		manager := create(t, db, &expManager{})
		qs := orm.Query[expManager](db).Annotate(
			"exists", orm.Exists(orm.Query[expManager](db).None()),
		).Filter("pk", manager.ID, "exists", false)
		djangotest.AssertQuerySetEqual(t, qs, []int64{manager.ID}, expManagerID)
		if got := firstValue(t, qs.ValuesList("exists")); got != false {
			t.Errorf("exists = %#v, want false", got)
		}
	})
}

// No Go counterpart: test_annotate_by_empty_custom_exists subclasses Exists
// with the template of Subquery; Go expressions have no templates to swap.
