package orm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/aggregation/models.py that
// tests/aggregation/test_filter_argument.py uses, with the many-to-many
// relations and the rating, without the fields the tests do not read: the
// price, the publisher's duration and the stores.

type faAuthor struct {
	ID      int64
	Name    string `orm:"max_length=100"`
	Age     int64
	Friends orm.M2M[faAuthor] `orm:"blank"`
}

type faPublisher struct {
	ID        int64
	Name      string `orm:"max_length=255"`
	NumAwards int64
}

type faBook struct {
	ID        int64
	Isbn      string `orm:"max_length=9"`
	Name      string `orm:"max_length=255"`
	Pages     int64
	Rating    float64
	Authors   orm.M2M[faAuthor]   `orm:"related_query_name=book"`
	Contact   orm.FK[faAuthor]    `orm:"related_name=book_contact_set"`
	Publisher orm.FK[faPublisher] `orm:"related_query_name=book"`
	Pubdate   orm.Date
}

// faModels returns the models of the aggregation app.
func faModels() []any {
	return []any{djangotest.App("aggregation", &faAuthor{}, &faPublisher{}, &faBook{})}
}

// faFixture holds the rows of FilteredAggregateTests.setUpTestData.
type faFixture struct {
	a1, a2, a3 *faAuthor
	b1, b2, b3 *faBook
}

// faSetUp creates the rows of FilteredAggregateTests.setUpTestData.
func faSetUp(t *testing.T, db *orm.DB) faFixture {
	t.Helper()
	ctx := context.Background()
	var f faFixture
	f.a1 = create(t, db, &faAuthor{Name: "test", Age: 40})
	f.a2 = create(t, db, &faAuthor{Name: "test2", Age: 60})
	f.a3 = create(t, db, &faAuthor{Name: "test3", Age: 100})
	p1 := create(t, db, &faPublisher{Name: "Apress", NumAwards: 3})
	book := func(isbn, name string, pages int64, rating float64, contact *faAuthor, pubdate orm.Date) *faBook {
		b := &faBook{Isbn: isbn, Name: name, Pages: pages, Rating: rating, Pubdate: pubdate}
		b.Contact.SetPK(contact.ID)
		b.Publisher.SetPK(p1.ID)
		return create(t, db, b)
	}
	f.b1 = book("159059725", "The Definitive Guide to Django: Web Development Done Right", 447, 4.5, f.a1,
		orm.Date{Year: 2007, Month: 12, Day: 6})
	f.b2 = book("067232959", "Sams Teach Yourself Django in 24 Hours", 528, 3.0, f.a2,
		orm.Date{Year: 2008, Month: 3, Day: 3})
	f.b3 = book("159059996", "Practical Django Projects", 600, 4.5, f.a3,
		orm.Date{Year: 2008, Month: 6, Day: 23})
	add := func(owner any, name string, targets ...*faAuthor) {
		t.Helper()
		if err := orm.Many[faAuthor](db, owner, name).Add(ctx, targets...); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}
	add(f.a1, "friends", f.a2)
	add(f.a1, "friends", f.a3)
	add(f.b1, "authors", f.a1)
	add(f.b1, "authors", f.a3)
	add(f.b2, "authors", f.a2)
	add(f.b3, "authors", f.a3)
	return f
}

// faAggregate runs Aggregate and fails the test on an error.
func faAggregate[T any](t *testing.T, qs *orm.QuerySet[T], args ...any) map[string]any {
	t.Helper()
	res, err := qs.Aggregate(context.Background(), args...)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	return res
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregates
func TestFilteredAggregates(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		agg := orm.Sum("age").Filter("name__startswith", "test")
		res := faAggregate(t, orm.Query[faAuthor](db), "age", agg)
		djangotest.AssertEqual(t, res["age"], any(int64(200)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_numerical_aggregates
func TestFilteredNumericalAggregates(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		for _, tc := range []struct {
			name      string
			aggregate func(arg any) orm.Expr
			want      float64
		}{
			{"Avg", orm.Avg, 66.7},
			{"StdDev", func(arg any) orm.Expr { return orm.StdDev(arg, false) }, 24.9},
			{"Variance", func(arg any) orm.Expr { return orm.Variance(arg, false) }, 622.2},
		} {
			t.Run(tc.name, func(t *testing.T) {
				agg := tc.aggregate("age").Filter("name__startswith", "test")
				res := faAggregate(t, orm.Query[faAuthor](db), "age", agg)
				agApprox(t, res["age"], tc.want, 1)
			})
		}
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_empty_filtered_aggregates
func TestEmptyFilteredAggregates(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		agg := orm.Count("pk").Filter(orm.Q())
		res := faAggregate(t, orm.Query[faAuthor](db), "count", agg)
		djangotest.AssertEqual(t, res["count"], any(int64(3)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_double_filtered_aggregates
func TestDoubleFilteredAggregates(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		agg := orm.Sum("age").Filter(orm.Q("name", "test2").And(orm.Not(orm.Q("name", "test"))))
		res := faAggregate(t, orm.Query[faAuthor](db), "age", agg)
		djangotest.AssertEqual(t, res["age"], any(int64(60)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_excluded_aggregates
func TestExcludedAggregates(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		agg := orm.Sum("age").Filter(orm.Not(orm.Q("name", "test2")))
		res := faAggregate(t, orm.Query[faAuthor](db), "age", agg)
		djangotest.AssertEqual(t, res["age"], any(int64(140)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_related_aggregates_m2m
func TestRelatedAggregatesM2M(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		agg := orm.Sum("friends__age").Filter(orm.Not(orm.Q("friends__name", "test")))
		res := faAggregate(t, orm.Query[faAuthor](db).Filter("name", "test"), "age", agg)
		djangotest.AssertEqual(t, res["age"], any(int64(160)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_related_aggregates_m2m_and_fk
func TestRelatedAggregatesM2MAndFK(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		q := orm.Q("friends__book__publisher__name", "Apress").And(orm.Not(orm.Q("friends__name", "test3")))
		agg := orm.Sum("friends__book__pages").Filter(q)
		res := faAggregate(t, orm.Query[faAuthor](db).Filter("name", "test"), "pages", agg)
		djangotest.AssertEqual(t, res["pages"], any(int64(528)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_plain_annotate
func TestFilteredPlainAnnotate(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		agg := orm.Sum("book__pages").Filter("book__rating__gt", 3)
		rows := agAnnotated(t, orm.Query[faAuthor](db).Annotate("pages", agg).OrderBy("pk"))
		got := make([]any, len(rows))
		for i, r := range rows {
			got[i] = r.Annotations["pages"]
		}
		djangotest.AssertSequenceEqual(t, got, []any{int64(447), nil, int64(1047)})
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregate_on_annotate
func TestFilteredAggregateOnAnnotate(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		pagesAnnotate := orm.Sum("book__pages").Filter("book__rating__gt", 3)
		ageAgg := orm.Sum("age").Filter("total_pages__gte", 400)
		res := faAggregate(t, orm.Query[faAuthor](db).Annotate("total_pages", pagesAnnotate), "summed_age", ageAgg)
		djangotest.AssertEqual(t, res, map[string]any{"summed_age": int64(140)})
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_case_aggregate
func TestFilteredCaseAggregate(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		agg := orm.Sum(orm.Case(orm.When{Cond: orm.Q("friends__age", 40), Then: orm.F("friends__age")})).
			Filter("friends__name__startswith", "test")
		res := faAggregate(t, orm.Query[faAuthor](db), "age", agg)
		djangotest.AssertEqual(t, res["age"], any(int64(80)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_sum_star_exception
func TestFilteredCountStarError(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[faAuthor](db).Aggregate(context.Background(), "n", orm.Count("*").Filter("age", 40))
		msg := "Star cannot be used with filter. Please specify a field."
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatalf("err = %v, want %q", err, msg)
		}
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_reused_subquery
func TestFilteredReusedSubquery(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		f := faSetUp(t, db)
		qs := orm.Query[faAuthor](db).
			Annotate("older_friends_count", orm.Count("friends").Filter("friends__age__gt", orm.F("age"))).
			Filter("older_friends_count__gte", 2)
		got, err := qs.Get(context.Background(), "pk__in", qs.ValuesList("pk"))
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, f.a1.ID)
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregate_ref_annotation
func TestFilteredAggregateRefAnnotation(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		qs := orm.Query[faAuthor](db).Annotate("double_age", orm.F("age").Mul(2))
		res := faAggregate(t, qs, "cnt", orm.Count("pk").Filter("double_age__gt", 100))
		djangotest.AssertEqual(t, res["cnt"], any(int64(2)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregate_ref_subquery_annotation
func TestFilteredAggregateRefSubqueryAnnotation(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		earliest := orm.Query[faBook](db).Filter("contact__pk", orm.OuterRef("pk")).OrderBy("pubdate").
			Values("pubdate__year").Limit(1)
		qs := orm.Query[faAuthor](db).Annotate("earliest_book_year", orm.Subquery(earliest))
		res := faAggregate(t, qs, "cnt", orm.Count("pk").Filter("earliest_book_year", 2008))
		djangotest.AssertEqual(t, res["cnt"], any(int64(2)))
	})
}

// The through model of Book.authors has no Go type. Where Django filters it
// by book, the port filters Author by its reverse relation to Book, which
// reads the same through table.

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregate_ref_multiple_subquery_annotation
func TestFilteredAggregateRefMultipleSubqueryAnnotation(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		otherBooks := orm.Query[faBook](db).Filter("authors__in",
			orm.Query[faAuthor](db).Filter("book_contact_set", orm.OuterRef(orm.OuterRef("pk"))),
		).Exclude("pk", orm.OuterRef("pk"))
		aggregate, err := orm.Query[faBook](db).Values("publisher").Annotate(
			"has_authors", orm.Exists(orm.Query[faAuthor](db).Filter("book", orm.OuterRef("pk"))),
			"authors_have_other_books", orm.Exists(otherBooks),
		).Aggregate(context.Background(),
			"max_rating", orm.Max("rating").Filter("has_authors", true, "authors_have_other_books", false),
		)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, aggregate, map[string]any{"max_rating": 4.5})
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregrate_ref_in_subquery_annotation
func TestFilteredAggregateRefInSubqueryAnnotation(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		count := orm.Query[faBook](db).Annotate("weird_count", orm.Count("pk").Filter("pages", orm.OuterRef("age"))).
			Values("weird_count").Limit(1)
		qs := orm.Query[faAuthor](db).Annotate("count", orm.Subquery(count)).OrderBy("pk")
		res := faAggregate(t, qs, "sum", orm.Sum("count"))
		djangotest.AssertEqual(t, res["sum"], any(int64(0)))
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregate_on_exists
func TestFilteredAggregateOnExists(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		faSetUp(t, db)
		aggregate, err := orm.Query[faBook](db).Values("publisher").Aggregate(context.Background(),
			"max_rating", orm.Max("rating").Filter(orm.Exists(orm.Query[faAuthor](db).Filter("book", orm.OuterRef("pk")))),
		)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, aggregate, map[string]any{"max_rating": 4.5})
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregate_empty_condition
func TestFilteredAggregateEmptyCondition(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		f := faSetUp(t, db)
		rows := agAnnotated(t, orm.Query[faBook](db).
			Annotate("authors_count", orm.Count("authors").Filter("authors__in", []int64{})).
			Filter("pk", f.b1.ID))
		djangotest.AssertEqual(t, len(rows), 1)
		djangotest.AssertEqual(t, rows[0].Annotations["authors_count"], any(int64(0)))
		res := faAggregate(t, orm.Query[faBook](db), "max_rating", orm.Max("rating").Filter("rating__in", []float64{}))
		djangotest.AssertEqual(t, res, map[string]any{"max_rating": nil})
	})
}

// Ported from Django: tests/aggregation/test_filter_argument.py::FilteredAggregateTests.test_filtered_aggregate_full_condition
func TestFilteredAggregateFullCondition(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		f := faSetUp(t, db)
		rows := agAnnotated(t, orm.Query[faBook](db).
			Annotate("authors_count", orm.Count("authors").Filter(orm.Not(orm.Q("authors__in", []int64{})))).
			Filter("pk", f.b1.ID))
		djangotest.AssertEqual(t, len(rows), 1)
		djangotest.AssertEqual(t, rows[0].Annotations["authors_count"], any(int64(2)))
		res := faAggregate(t, orm.Query[faBook](db), "max_rating",
			orm.Max("rating").Filter(orm.Not(orm.Q("rating__in", []float64{}))))
		djangotest.AssertEqual(t, res, map[string]any{"max_rating": 4.5})
	})
}

// No Django test exists: Django refuses filter= on a non-aggregate when the
// expression is built, and an aggregate with a filter has no default alias
// (default_alias raises "Complex expressions require an alias").
func TestAggregateFilterErrors(t *testing.T) {
	djangotest.EachDB(t, faModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := orm.Query[faAuthor](db).Annotate("x", orm.F("age").Filter("age", 1)).All(ctx)
		if err == nil || !strings.Contains(err.Error(), "Filter applies to aggregates only") {
			t.Errorf("Filter on F: err = %v", err)
		}
		_, err = orm.Query[faAuthor](db).Aggregate(ctx, orm.Sum("age").Filter("age", 1))
		if err == nil || !strings.Contains(err.Error(), "require an alias") {
			t.Errorf("Aggregate without an alias: err = %v", err)
		}
	})
}
