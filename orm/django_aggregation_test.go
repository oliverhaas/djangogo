package orm_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/aggregation/models.py, without the fields the
// ORM cannot hold yet: the many-to-many relations (Author.friends,
// Book.authors, Store.books) and the float, decimal and duration fields
// (rating, price, duration). The tests that need those are not ported.

type agAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
	Age  int64
}

type agPublisher struct {
	ID        int64
	Name      string `orm:"max_length=255"`
	NumAwards int64
}

type agBook struct {
	ID        int64
	Isbn      string `orm:"max_length=9"`
	Name      string `orm:"max_length=255"`
	Pages     int64
	Contact   orm.FK[agAuthor]
	Publisher orm.FK[agPublisher]
	Pubdate   time.Time
}

// agModels returns the models of the aggregation app.
func agModels() []any {
	return []any{djangotest.App("aggregation", &agAuthor{}, &agPublisher{}, &agBook{})}
}

// agFixture holds the rows of AggregateTestCase.setUpTestData.
type agFixture struct {
	authors    []*agAuthor
	publishers []*agPublisher
	books      []*agBook
}

// agSetUp creates the rows of AggregateTestCase.setUpTestData.
func agSetUp(t *testing.T, db *orm.DB) agFixture {
	t.Helper()
	var f agFixture
	for _, a := range []agAuthor{
		{Name: "Adrian Holovaty", Age: 34},
		{Name: "Jacob Kaplan-Moss", Age: 35},
		{Name: "Brad Dayley", Age: 45},
		{Name: "James Bennett", Age: 29},
		{Name: "Jeffrey Forcier", Age: 37},
		{Name: "Paul Bissex", Age: 29},
		{Name: "Wesley J. Chun", Age: 25},
		{Name: "Peter Norvig", Age: 57},
		{Name: "Stuart Russell", Age: 46},
	} {
		f.authors = append(f.authors, create(t, db, &a))
	}
	for _, p := range []agPublisher{
		{Name: "Apress", NumAwards: 3},
		{Name: "Sams", NumAwards: 1},
		{Name: "Prentice Hall", NumAwards: 7},
		{Name: "Morgan Kaufmann", NumAwards: 9},
		{Name: "Jonno's House of Books", NumAwards: 0},
	} {
		f.publishers = append(f.publishers, create(t, db, &p))
	}
	book := func(isbn, name string, pages int64, contact, publisher int, pubdate time.Time) {
		f.books = append(f.books, create(t, db, &agBook{
			Isbn: isbn, Name: name, Pages: pages,
			Contact:   fkTo[agAuthor](f.authors[contact].ID),
			Publisher: fkTo[agPublisher](f.publishers[publisher].ID),
			Pubdate:   pubdate,
		}))
	}
	book("159059725", "The Definitive Guide to Django: Web Development Done Right", 447, 0, 0, day(2007, 12, 6))
	book("067232959", "Sams Teach Yourself Django in 24 Hours", 528, 2, 1, day(2008, 3, 3))
	book("159059996", "Practical Django Projects", 300, 3, 0, day(2008, 6, 23))
	book("013235613", "Python Web Development with Django", 350, 4, 2, day(2008, 11, 3))
	book("013790395", "Artificial Intelligence: A Modern Approach", 1132, 7, 2, day(1995, 1, 15))
	book("155860191",
		"Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp", 946, 7, 3, day(1991, 10, 15))
	return f
}

// agAggregate runs Aggregate and fails the test on an error.
func agAggregate[T any](t *testing.T, qs *orm.QuerySet[T], args ...any) map[string]any {
	t.Helper()
	vals, err := qs.Aggregate(context.Background(), args...)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	return vals
}

// agApprox checks that got is a float64 within half a unit of the given
// decimal place of want, like Django's Approximate.
func agApprox(t *testing.T, got any, want float64, places int) {
	t.Helper()
	f, ok := got.(float64)
	if !ok || math.Abs(f-want) >= 0.5*math.Pow(10, -float64(places)) {
		t.Errorf("got %v (%T), want about %v", got, got, want)
	}
}

// agAnnotated runs the queryset and returns the rows with their annotations.
func agAnnotated[T any](t *testing.T, qs *orm.QuerySet[T]) []orm.Annotated[T] {
	t.Helper()
	rows, err := qs.AllAnnotated(context.Background())
	if err != nil {
		t.Fatalf("AllAnnotated: %v", err)
	}
	return rows
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_empty_aggregate
func TestAggregationEmptyAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		djangotest.AssertEqual(t, len(agAggregate(t, orm.Query[agAuthor](db))), 0)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_single_aggregate
func TestAggregationSingleAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db), orm.Avg("age"))
		djangotest.AssertEqual(t, len(vals), 1)
		agApprox(t, vals["age__avg"], 37.4, 1)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_multiple_aggregates
func TestAggregationMultipleAggregates(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db), orm.Sum("age"), orm.Avg("age"))
		djangotest.AssertEqual(t, len(vals), 2)
		djangotest.AssertEqual(t, vals["age__sum"], any(int64(337)))
		agApprox(t, vals["age__avg"], 37.4, 1)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_filter_aggregate
func TestAggregationFilterAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db).Filter("age__gt", 29), orm.Sum("age"))
		djangotest.AssertEqual(t, len(vals), 1)
		djangotest.AssertEqual(t, vals["age__sum"], any(int64(254)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_related_aggregate
//
// Only the aggregate over a forward foreign key is ported; the others go
// through many-to-many and reverse relations.
func TestAggregationRelatedAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agBook](db), orm.Sum("publisher__num_awards"))
		djangotest.AssertEqual(t, len(vals), 1)
		djangotest.AssertEqual(t, vals["publisher__num_awards__sum"], any(int64(30)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_basic
//
// Only the annotate() without arguments is ported; the rest averages over a
// many-to-many relation.
func TestAggregationAnnotateBasic(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		djangotest.AssertQuerySetEqual(t, orm.Query[agBook](db).Annotate().OrderBy("pk"),
			[]string{
				"The Definitive Guide to Django: Web Development Done Right",
				"Sams Teach Yourself Django in 24 Hours",
				"Practical Django Projects",
				"Python Web Development with Django",
				"Artificial Intelligence: A Modern Approach",
				"Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp",
			},
			func(b agBook) string { return b.Name })
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_reverse_fkey_annotate
//
// Only the annotation over the forward foreign key is ported; the second half
// sums over the reverse relation.
func TestAggregationReverseFkeyAnnotate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		type row struct {
			name   string
			awards any
		}
		books := agAnnotated(t, orm.Query[agBook](db).Annotate(orm.Sum("publisher__num_awards")).OrderBy("name"))
		got := make([]row, len(books))
		for i, b := range books {
			got[i] = row{b.Object.Name, b.Annotations["publisher__num_awards__sum"]}
		}
		djangotest.AssertSequenceEqual(t, got, []row{
			{"Artificial Intelligence: A Modern Approach", int64(7)},
			{"Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp", int64(9)},
			{"Practical Django Projects", int64(3)},
			{"Python Web Development with Django", int64(7)},
			{"Sams Teach Yourself Django in 24 Hours", int64(1)},
			{"The Definitive Guide to Django: Web Development Done Right", int64(3)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_count_star
func TestAggregationCountStar(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		capture := djangotest.CaptureQueries(db)
		vals := agAggregate(t, orm.Query[agBook](db), "n", orm.Count("*"))
		capture.Stop()

		djangotest.AssertEqual(t, capture.Len(), 1)
		if sql := capture.Queries()[0].SQL; !strings.Contains(sql, "SELECT COUNT(*) ") {
			t.Errorf("sql = %q, want it to contain %q", sql, "SELECT COUNT(*) ")
		}
		djangotest.AssertEqual(t, vals["n"], any(int64(6)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_nonaggregate_aggregation_throws
//
// The Django test reads the decimal field price; pages stands in for it.
func TestAggregationNonaggregateAggregationThrows(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[agBook](db).Aggregate(context.Background(), "fail", orm.F("pages"))
		assertErrorContains(t, err, "fail is not an aggregate expression")
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_nonfield_annotation
//
// The Go API has no output_field, so the three forms of the Django test are
// one.
func TestAggregationNonfieldAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		books := agAnnotated(t, orm.Query[agBook](db).Annotate("val", orm.Max(orm.Value(2))).OrderBy("pk").Limit(1))
		djangotest.AssertEqual(t, len(books), 1)
		djangotest.AssertEqual(t, books[0].Annotations["val"], any(int64(2)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_complex_aggregations_require_kwarg
//
// The annotation of the Django test adds the age of the author's friends, a
// many-to-many relation; the author's own age stands in for it.
func TestAggregationComplexAggregationsRequireKwarg(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		authors := orm.Query[agAuthor](db)

		_, err := authors.Annotate(orm.Sum(orm.F("age").Add(orm.F("age")))).All(ctx)
		assertErrorContains(t, err, "complex annotations require an alias")
		_, err = authors.Aggregate(ctx, orm.Sum("age").Div(orm.Count("age")))
		assertErrorContains(t, err, "complex aggregates require an alias")
		_, err = authors.Aggregate(ctx, orm.Sum(1))
		assertErrorContains(t, err, "complex aggregates require an alias")
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_values_aggregate
func TestAggregationAnnotateValuesAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()

		aliasAge, err := orm.Query[agAuthor](db).Annotate("age_alias", orm.F("age")).Values("age_alias").
			Aggregate(ctx, "sum_age", orm.Sum("age_alias"))
		if err != nil {
			t.Fatalf("Aggregate over the alias: %v", err)
		}
		age, err := orm.Query[agAuthor](db).Values("age").Aggregate(ctx, "sum_age", orm.Sum("age"))
		if err != nil {
			t.Fatalf("Aggregate over the field: %v", err)
		}
		djangotest.AssertEqual(t, aliasAge["sum_age"], age["sum_age"])
		djangotest.AssertEqual(t, age["sum_age"], any(int64(337)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_over_annotate
func TestAggregationAnnotateOverAnnotate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)

		author := agAnnotated(t, orm.Query[agAuthor](db).Annotate("age_alias", orm.F("age")).
			Annotate("sum_age", orm.Sum("age_alias")).Filter("name", "Adrian Holovaty"))
		other := agAnnotated(t, orm.Query[agAuthor](db).Annotate("sum_age", orm.Sum("age")).
			Filter("name", "Adrian Holovaty"))
		if len(author) != 1 || len(other) != 1 {
			t.Fatalf("got %d and %d rows, want 1 and 1", len(author), len(other))
		}
		djangotest.AssertEqual(t, author[0].Annotations["sum_age"], other[0].Annotations["sum_age"])
		djangotest.AssertEqual(t, other[0].Annotations["sum_age"], any(int64(34)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_over_aggregate
func TestAggregationAggregateOverAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[agAuthor](db).Aggregate(context.Background(),
			"age_agg", orm.Sum(orm.F("age")),
			"avg_age", orm.Avg(orm.F("age_agg")))
		assertFieldError(t, err, "cannot compute Avg('age_agg'): 'age_agg' is an aggregate")
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotated_aggregate_over_annotated_aggregate
//
// The second half of the Django test subclasses Max, which has no Go
// counterpart.
func TestAggregationAnnotatedAggregateOverAnnotatedAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[agBook](db).Annotate(orm.Max("id")).Annotate(orm.Sum("id__max")).
			All(context.Background())
		assertFieldError(t, err, "cannot compute Sum('id__max'): 'id__max' is an aggregate")
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_ticket11881
func TestAggregationTicket11881(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		// Subqueries do not needlessly contain ORDER BY, SELECT FOR UPDATE or
		// select_related() stuff.
		qs := orm.Query[agBook](db).SelectForUpdate().OrderBy("pk").SelectRelated("publisher").
			Annotate("max_pk", orm.Max("pk"))
		capture := djangotest.CaptureQueries(db)
		agAggregate(t, qs, "avg_pk", orm.Avg("max_pk"))
		capture.Stop()

		djangotest.AssertEqual(t, capture.Len(), 1)
		sql := strings.ToLower(capture.Queries()[0].SQL)
		for _, unwanted := range []string{"for update", "order by", " join "} {
			if strings.Contains(sql, unwanted) {
				t.Errorf("sql = %q, want it without %q", sql, unwanted)
			}
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_having_with_no_group_by
//
// values(static_value=Value(...)) is an annotation followed by Values, and the
// final values_list("sum", flat=True) reads the "sum" key of the rows.
func TestAggregationHavingWithNoGroupBy(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		rows, err := orm.Query[agAuthor](db).Annotate("static_value", orm.Value("static-value")).
			Values("static_value").Annotate("sum", orm.Sum("age")).Filter("sum__gte", 0).
			All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertEqual(t, len(rows), 1)
		djangotest.AssertEqual(t, rows[0]["sum"], any(int64(337)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_empty_result_optimization
//
// There is no none(); a filter on an empty list is the queryset that is known
// to be empty. Count("id") stands in for the count over the reverse relation,
// and an arithmetic expression for the Func whose value over no rows Django
// cannot tell.
func TestAggregationEmptyResultOptimization(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		none := orm.Query[agPublisher](db).Filter("pk__in", []int64{})

		djangotest.AssertNumQueries(t, db, 0, func() {
			vals := agAggregate(t, none, "sum_awards", orm.Sum("num_awards"), "books_count", orm.Count("id"))
			djangotest.AssertEqual(t, len(vals), 2)
			djangotest.AssertEqual(t, vals["sum_awards"], nil)
			djangotest.AssertEqual(t, vals["books_count"], any(int64(0)))
		})
		// An expression whose value over no rows is not known forces the query
		// to run even though it returns an empty result set.
		djangotest.AssertNumQueries(t, db, 1, func() {
			vals := agAggregate(t, none,
				"sum_awards", orm.Sum("num_awards"), "books_count", orm.Count("id").Add(0))
			djangotest.AssertEqual(t, len(vals), 2)
			djangotest.AssertEqual(t, vals["sum_awards"], nil)
			djangotest.AssertEqual(t, vals["books_count"], any(int64(0)))
		})
	})
}

// TestAggregateGroupsFiltersAndOrders covers what Django tests through its
// many-to-many and float fixtures only: grouping by Values keys, a filter and
// an ordering on an aggregate annotation, a distinct count, an aggregate over
// a sliced queryset, and the Go types of Min and Max.
func TestAggregateGroupsFiltersAndOrders(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		ctx := context.Background()
		books := orm.Query[agBook](db)

		// One group per publisher, the groups with more than one book only.
		perPublisher := books.Values("publisher__name").
			Annotate("n", orm.Count("id"), "total_pages", orm.Sum("pages")).
			Filter("n__gt", 1).OrderBy("-total_pages")
		assertValues(t, perPublisher, []map[string]any{
			{"publisher__name": "Prentice Hall", "n": int64(2), "total_pages": int64(1482)},
			{"publisher__name": "Apress", "n": int64(2), "total_pages": int64(747)},
		})

		// A filter on a column narrows the rows before they are grouped.
		assertValues(t, books.Filter("pages__gt", 400).Values("publisher_id").
			Annotate("n", orm.Count("id")).OrderBy("-n", "publisher_id"),
			[]map[string]any{
				{"publisher_id": f.publishers[0].ID, "n": int64(1)},
				{"publisher_id": f.publishers[1].ID, "n": int64(1)},
				{"publisher_id": f.publishers[2].ID, "n": int64(1)},
				{"publisher_id": f.publishers[3].ID, "n": int64(1)},
			})

		vals := agAggregate(t, books,
			"contacts", orm.Count("contact").Distinct(),
			"first", orm.Min("pubdate"), "last_name", orm.Max("name"), "least", orm.Min("pages"))
		djangotest.AssertEqual(t, vals["contacts"], any(int64(5)))
		djangotest.AssertEqual(t, vals["last_name"],
			any("The Definitive Guide to Django: Web Development Done Right"))
		djangotest.AssertEqual(t, vals["least"], any(int64(300)))
		if first, ok := vals["first"].(time.Time); !ok || !first.Equal(day(1991, 10, 15)) {
			t.Errorf("first = %v (%T), want 1991-10-15", vals["first"], vals["first"])
		}

		// The two longest books only: the slice is aggregated, not the table.
		vals = agAggregate(t, books.OrderBy("-pages").Limit(2), orm.Sum("pages"), orm.Avg("pages"))
		djangotest.AssertEqual(t, vals["pages__sum"], any(int64(2078)))
		agApprox(t, vals["pages__avg"], 1039, 1)

		// Without rows every aggregate but Count is nil.
		vals, err := books.Filter("pages__gt", 5000).Aggregate(ctx, orm.Sum("pages"), orm.Count("id"))
		if err != nil {
			t.Fatalf("Aggregate over no rows: %v", err)
		}
		djangotest.AssertEqual(t, vals["pages__sum"], nil)
		djangotest.AssertEqual(t, vals["id__count"], any(int64(0)))
	})
}
