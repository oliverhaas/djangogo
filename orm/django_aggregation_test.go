package orm_test

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// agAuthor is tests/aggregation/models.py::Author.
type agAuthor struct {
	ID      int64
	Name    string `orm:"max_length=100"`
	Age     int64
	Friends orm.M2M[agAuthor] `orm:"blank"`
	Rating  *float64          `orm:"null"`
}

// agPublisher is tests/aggregation/models.py::Publisher.
type agPublisher struct {
	ID        int64
	Name      string `orm:"max_length=255"`
	NumAwards int64
	Duration  *time.Duration `orm:"blank;null"`
}

// agBook is tests/aggregation/models.py::Book. It embeds orm.RelCache for the
// defer tests.
type agBook struct {
	orm.RelCache
	ID        int64
	Isbn      string `orm:"max_length=9"`
	Name      string `orm:"max_length=255"`
	Pages     int64
	Rating    float64
	Price     orm.Decimal         `orm:"max_digits=6;decimal_places=2"`
	Authors   orm.M2M[agAuthor]   `orm:"related_query_name=book"`
	Contact   orm.FK[agAuthor]    `orm:"on_delete=cascade;related_name=book_contact_set"`
	Publisher orm.FK[agPublisher] `orm:"on_delete=cascade;related_query_name=book"`
	Pubdate   orm.Date
}

// agStore is tests/aggregation/models.py::Store.
type agStore struct {
	ID                 int64
	Name               string          `orm:"max_length=255"`
	Books              orm.M2M[agBook] `orm:"related_query_name=store"`
	OriginalOpening    time.Time
	FridayNightClosing orm.Time
}

// agModels returns the models of the aggregation app, without Employee, whose
// JSON field only a StringAgg test reads.
func agModels() []any {
	return []any{djangotest.App("aggregation", &agAuthor{}, &agPublisher{}, &agBook{}, &agStore{})}
}

// agFixture holds the rows of AggregateTestCase.setUpTestData.
type agFixture struct {
	a1, a2, a3, a4, a5, a6, a7, a8, a9 *agAuthor
	p1, p2, p3, p4, p5                 *agPublisher
	b1, b2, b3, b4, b5, b6             *agBook
}

// agSetUp creates the rows of AggregateTestCase.setUpTestData.
func agSetUp(t *testing.T, db *orm.DB) agFixture {
	t.Helper()
	ctx := context.Background()
	var f agFixture
	author := func(name string, age int64) *agAuthor { return create(t, db, &agAuthor{Name: name, Age: age}) }
	f.a1 = author("Adrian Holovaty", 34)
	f.a2 = author("Jacob Kaplan-Moss", 35)
	f.a3 = author("Brad Dayley", 45)
	f.a4 = author("James Bennett", 29)
	f.a5 = author("Jeffrey Forcier", 37)
	f.a6 = author("Paul Bissex", 29)
	f.a7 = author("Wesley J. Chun", 25)
	f.a8 = author("Peter Norvig", 57)
	f.a9 = author("Stuart Russell", 46)
	add := func(owner any, name string, targets ...*agAuthor) {
		t.Helper()
		if err := orm.Many[agAuthor](db, owner, name).Add(ctx, targets...); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}
	add(f.a1, "friends", f.a2, f.a4)
	add(f.a2, "friends", f.a1, f.a7)
	add(f.a4, "friends", f.a1)
	add(f.a5, "friends", f.a6, f.a7)
	add(f.a6, "friends", f.a5, f.a7)
	add(f.a7, "friends", f.a2, f.a5, f.a6)
	add(f.a8, "friends", f.a9)
	add(f.a9, "friends", f.a8)

	oneDay, twoDays := 24*time.Hour, 48*time.Hour
	f.p1 = create(t, db, &agPublisher{Name: "Apress", NumAwards: 3, Duration: &oneDay})
	f.p2 = create(t, db, &agPublisher{Name: "Sams", NumAwards: 1, Duration: &twoDays})
	f.p3 = create(t, db, &agPublisher{Name: "Prentice Hall", NumAwards: 7})
	f.p4 = create(t, db, &agPublisher{Name: "Morgan Kaufmann", NumAwards: 9})
	f.p5 = create(t, db, &agPublisher{Name: "Jonno's House of Books", NumAwards: 0})

	book := func(isbn, name string, pages int64, rating float64, price string, contact *agAuthor, publisher *agPublisher, pubdate orm.Date) *agBook {
		b := &agBook{Isbn: isbn, Name: name, Pages: pages, Rating: rating, Price: orm.MustDecimal(price), Pubdate: pubdate}
		b.Contact.SetPK(contact.ID)
		b.Publisher.SetPK(publisher.ID)
		return create(t, db, b)
	}
	f.b1 = book("159059725", "The Definitive Guide to Django: Web Development Done Right", 447, 4.5, "30.00",
		f.a1, f.p1, orm.Date{Year: 2007, Month: 12, Day: 6})
	f.b2 = book("067232959", "Sams Teach Yourself Django in 24 Hours", 528, 3.0, "23.09",
		f.a3, f.p2, orm.Date{Year: 2008, Month: 3, Day: 3})
	f.b3 = book("159059996", "Practical Django Projects", 300, 4.0, "29.69",
		f.a4, f.p1, orm.Date{Year: 2008, Month: 6, Day: 23})
	f.b4 = book("013235613", "Python Web Development with Django", 350, 4.0, "29.69",
		f.a5, f.p3, orm.Date{Year: 2008, Month: 11, Day: 3})
	f.b5 = book("013790395", "Artificial Intelligence: A Modern Approach", 1132, 4.0, "82.80",
		f.a8, f.p3, orm.Date{Year: 1995, Month: 1, Day: 15})
	f.b6 = book("155860191", "Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp", 946, 5.0, "75.00",
		f.a8, f.p4, orm.Date{Year: 1991, Month: 10, Day: 15})
	add(f.b1, "authors", f.a1, f.a2)
	add(f.b2, "authors", f.a3)
	add(f.b3, "authors", f.a4)
	add(f.b4, "authors", f.a5, f.a6, f.a7)
	add(f.b5, "authors", f.a8, f.a9)
	add(f.b6, "authors", f.a8)

	s1 := create(t, db, &agStore{
		Name:            "Amazon.com",
		OriginalOpening: time.Date(1994, 4, 23, 9, 17, 42, 0, time.UTC), FridayNightClosing: orm.NewTime(23, 59, 59, 0),
	})
	s2 := create(t, db, &agStore{
		Name:            "Books.com",
		OriginalOpening: time.Date(2001, 3, 15, 11, 23, 37, 0, time.UTC), FridayNightClosing: orm.NewTime(23, 59, 59, 0),
	})
	s3 := create(t, db, &agStore{
		Name:            "Mamma and Pappa's Books",
		OriginalOpening: time.Date(1945, 4, 25, 16, 24, 14, 0, time.UTC), FridayNightClosing: orm.NewTime(21, 30, 0, 0),
	})
	for _, s := range []struct {
		store *agStore
		books []*agBook
	}{
		{s1, []*agBook{f.b1, f.b2, f.b3, f.b4, f.b5, f.b6}},
		{s2, []*agBook{f.b1, f.b3, f.b5, f.b6}},
		{s3, []*agBook{f.b3, f.b4, f.b6}},
	} {
		if err := orm.Many[agBook](db, s.store, "books").Add(ctx, s.books...); err != nil {
			t.Fatalf("add books: %v", err)
		}
	}
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

// agApproximate is Django's Approximate: a float64 or an orm.Decimal that
// equals val rounded to places decimal places.
type agApproximate struct {
	val    float64
	places int
}

// agMatch reports whether got equals want. An agApproximate matches a nearby
// float64 or orm.Decimal, an orm.Decimal matches a decimal of the same value
// whatever its scale, and equalValue compares any other value.
func agMatch(got, want any) bool {
	switch w := want.(type) {
	case agApproximate:
		var f float64
		switch g := got.(type) {
		case int64:
			f = float64(g)
		case float64:
			f = g
		case orm.Decimal:
			f, _ = g.Rat().Float64()
		default:
			return false
		}
		return math.Abs(f-w.val) < 0.5*math.Pow(10, -float64(w.places))
	case orm.Decimal:
		g, ok := got.(orm.Decimal)
		return ok && g.Rat().Cmp(w.Rat()) == 0
	}
	return equalValue(got, want)
}

// agAssertMap checks that got has exactly the keys of want, with values that
// agMatch accepts.
func agAssertMap(t *testing.T, got, want map[string]any) {
	t.Helper()
	equal := len(got) == len(want)
	for k, w := range want {
		g, ok := got[k]
		equal = equal && ok && agMatch(g, w)
	}
	if !equal {
		t.Errorf("values differ:\n got: %v\nwant: %v", got, want)
	}
}

// agAssertRows checks that got holds the rows of want in order, with values
// that agMatch accepts.
func agAssertRows(t *testing.T, got, want [][]any) {
	t.Helper()
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		equal = len(got[i]) == len(want[i])
		for k := 0; equal && k < len(got[i]); k++ {
			equal = agMatch(got[i][k], want[i][k])
		}
	}
	if !equal {
		t.Errorf("rows differ:\n got: %v\nwant: %v", got, want)
	}
}

// agAssertValues runs vs and checks its rows in order with agAssertMap.
func agAssertValues(t *testing.T, vs *orm.ValuesQuerySet, want []map[string]any) {
	t.Helper()
	got, err := vs.All(context.Background())
	if err != nil {
		t.Errorf("Values: %v", err)
		return
	}
	if len(got) != len(want) {
		t.Errorf("got %d rows, want %d:\n got: %v\nwant: %v", len(got), len(want), got, want)
		return
	}
	for i := range got {
		agAssertMap(t, got[i], want[i])
	}
}

// agAssertValuesList runs vl and checks its rows in order with agAssertRows.
func agAssertValuesList(t *testing.T, vl *orm.ValuesListQuerySet, want [][]any) {
	t.Helper()
	got, err := vl.All(context.Background())
	if err != nil {
		t.Errorf("ValuesList: %v", err)
		return
	}
	agAssertRows(t, got, want)
}

// agNamed runs qs and returns the name of each row with the value of the
// annotation key, Django's lambda b: (b.name, b.<key>).
func agNamed[T any](t *testing.T, qs *orm.QuerySet[T], name func(T) string, key string) [][]any {
	t.Helper()
	rows := agAnnotated(t, qs)
	out := make([][]any, len(rows))
	for i, r := range rows {
		out[i] = []any{name(r.Object), r.Annotations[key]}
	}
	return out
}

// agBookName returns the name of b.
func agBookName(b agBook) string { return b.Name }

// agAuthorName returns the name of a.
func agAuthorName(a agAuthor) string { return a.Name }

// agPublisherName returns the name of p.
func agPublisherName(p agPublisher) string { return p.Name }

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_empty_aggregate
func TestAggregationEmptyAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		djangotest.AssertEqual(t, len(agAggregate(t, orm.Query[agAuthor](db))), 0)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_in_order_by
func TestAggregationAggregateInOrderBy(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[agAuthor](db).Values("age").OrderBy(orm.Avg("book__rating")).All(context.Background())
		assertErrorContains(t, err, "using an aggregate in OrderBy without also including it in Annotate is not allowed")
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
func TestAggregationRelatedAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		agAssertMap(t, agAggregate(t, orm.Query[agAuthor](db), orm.Avg("friends__age")),
			map[string]any{"friends__age__avg": agApproximate{34.07, 2}})
		agAssertMap(t, agAggregate(t, orm.Query[agBook](db).Filter("rating__lt", 4.5), orm.Avg("authors__age")),
			map[string]any{"authors__age__avg": agApproximate{38.2857, 2}})
		agAssertMap(t, agAggregate(t, orm.Query[agAuthor](db).Filter("name__contains", "a"), orm.Avg("book__rating")),
			map[string]any{"book__rating__avg": 4.0})
		agAssertMap(t, agAggregate(t, orm.Query[agBook](db), orm.Sum("publisher__num_awards")),
			map[string]any{"publisher__num_awards__sum": int64(30)})
		agAssertMap(t, agAggregate(t, orm.Query[agPublisher](db), orm.Sum("book__price")),
			map[string]any{"book__price__sum": orm.MustDecimal("270.27")})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_multi_join
func TestAggregationAggregateMultiJoin(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		agAssertMap(t, agAggregate(t, orm.Query[agStore](db), orm.Max("books__authors__age")),
			map[string]any{"books__authors__age__max": int64(57)})
		agAssertMap(t, agAggregate(t, orm.Query[agAuthor](db), orm.Min("book__publisher__num_awards")),
			map[string]any{"book__publisher__num_awards__min": int64(1)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_alias
func TestAggregationAggregateAlias(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agStore](db).Filter("name", "Amazon.com"), "amazon_mean", orm.Avg("books__rating"))
		agAssertMap(t, vals, map[string]any{"amazon_mean": agApproximate{4.08, 2}})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_transform
func TestAggregationAggregateTransform(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agStore](db), "min_month", orm.Min("original_opening__month"))
		agAssertMap(t, vals, map[string]any{"min_month": int64(3)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_join_transform
func TestAggregationAggregateJoinTransform(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agPublisher](db), "min_year", orm.Min("book__pubdate__year"))
		djangotest.AssertEqual(t, vals, map[string]any{"min_year": int64(1991)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_basic
func TestAggregationAnnotateBasic(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		djangotest.AssertQuerySetEqual(t, orm.Query[agBook](db).Annotate().OrderBy("pk"),
			[]string{
				"The Definitive Guide to Django: Web Development Done Right",
				"Sams Teach Yourself Django in 24 Hours",
				"Practical Django Projects",
				"Python Web Development with Django",
				"Artificial Intelligence: A Modern Approach",
				"Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp",
			},
			agBookName)

		books := orm.Query[agBook](db).Annotate("mean_age", orm.Avg("authors__age"))
		agAssertRows(t, agNamed(t, books.Filter("pk", f.b1.ID), agBookName, "mean_age"),
			[][]any{{"The Definitive Guide to Django: Web Development Done Right", 34.5}})
	})
}

// agDeferredRows runs qs, whose rows defer the name of the book, loads the
// name with RefreshFromDB, where Django loads it when read, and returns
// row(b, annotations) for each row.
func agDeferredRows(t *testing.T, db *orm.DB, qs *orm.QuerySet[agBook], row func(*agBook, map[string]any) []any) [][]any {
	t.Helper()
	rows := agAnnotated(t, qs)
	out := make([][]any, len(rows))
	for i := range rows {
		b := &rows[i].Object
		djangotest.AssertEqual(t, orm.DeferredFields(b), []string{"name"})
		if err := orm.Query[agBook](db).RefreshFromDB(context.Background(), b, "name"); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		out[i] = row(b, rows[i].Annotations)
	}
	return out
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_defer
func TestAggregationAnnotateDefer(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		qs := orm.Query[agBook](db).Annotate("page_sum", orm.Sum("pages")).Defer("name").Filter("pk", f.b1.ID)
		got := agDeferredRows(t, db, qs.OrderBy("pk"), func(b *agBook, ann map[string]any) []any {
			return []any{b.ID, b.Isbn, ann["page_sum"], b.Name}
		})
		agAssertRows(t, got, [][]any{
			{f.b1.ID, "159059725", int64(447), "The Definitive Guide to Django: Web Development Done Right"},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_defer_select_related
func TestAggregationAnnotateDeferSelectRelated(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		qs := orm.Query[agBook](db).SelectRelated("contact").Annotate("page_sum", orm.Sum("pages")).
			Defer("name").Filter("pk", f.b1.ID)
		got := agDeferredRows(t, db, qs.OrderBy("pk"), func(b *agBook, ann map[string]any) []any {
			contact, ok := b.Contact.Object()
			if !ok {
				t.Fatal("the contact is not loaded")
			}
			return []any{b.ID, b.Isbn, ann["page_sum"], contact.Name, b.Name}
		})
		agAssertRows(t, got, [][]any{
			{f.b1.ID, "159059725", int64(447), "Adrian Holovaty", "The Definitive Guide to Django: Web Development Done Right"},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_m2m
func TestAggregationAnnotateM2M(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		books := orm.Query[agBook](db).Filter("rating__lt", 4.5).Annotate(orm.Avg("authors__age")).OrderBy("name")
		agAssertRows(t, agNamed(t, books, agBookName, "authors__age__avg"), [][]any{
			{"Artificial Intelligence: A Modern Approach", 51.5},
			{"Practical Django Projects", 29.0},
			{"Python Web Development with Django", agApproximate{30.3, 1}},
			{"Sams Teach Yourself Django in 24 Hours", 45.0},
		})

		books = orm.Query[agBook](db).Annotate("num_authors", orm.Count("authors")).OrderBy("name")
		agAssertRows(t, agNamed(t, books, agBookName, "num_authors"), [][]any{
			{"Artificial Intelligence: A Modern Approach", int64(2)},
			{"Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp", int64(1)},
			{"Practical Django Projects", int64(1)},
			{"Python Web Development with Django", int64(3)},
			{"Sams Teach Yourself Django in 24 Hours", int64(1)},
			{"The Definitive Guide to Django: Web Development Done Right", int64(2)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_backwards_m2m_annotate
func TestAggregationBackwardsM2MAnnotate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		authors := orm.Query[agAuthor](db).Filter("name__contains", "a").Annotate(orm.Avg("book__rating")).OrderBy("name")
		agAssertRows(t, agNamed(t, authors, agAuthorName, "book__rating__avg"), [][]any{
			{"Adrian Holovaty", 4.5},
			{"Brad Dayley", 3.0},
			{"Jacob Kaplan-Moss", 4.5},
			{"James Bennett", 4.0},
			{"Paul Bissex", 4.0},
			{"Stuart Russell", 4.0},
		})

		authors = orm.Query[agAuthor](db).Annotate("num_books", orm.Count("book")).OrderBy("name")
		agAssertRows(t, agNamed(t, authors, agAuthorName, "num_books"), [][]any{
			{"Adrian Holovaty", int64(1)},
			{"Brad Dayley", int64(1)},
			{"Jacob Kaplan-Moss", int64(1)},
			{"James Bennett", int64(1)},
			{"Jeffrey Forcier", int64(1)},
			{"Paul Bissex", int64(1)},
			{"Peter Norvig", int64(2)},
			{"Stuart Russell", int64(1)},
			{"Wesley J. Chun", int64(1)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_reverse_fkey_annotate
func TestAggregationReverseFkeyAnnotate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		books := orm.Query[agBook](db).Annotate(orm.Sum("publisher__num_awards")).OrderBy("name")
		agAssertRows(t, agNamed(t, books, agBookName, "publisher__num_awards__sum"), [][]any{
			{"Artificial Intelligence: A Modern Approach", int64(7)},
			{"Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp", int64(9)},
			{"Practical Django Projects", int64(3)},
			{"Python Web Development with Django", int64(7)},
			{"Sams Teach Yourself Django in 24 Hours", int64(1)},
			{"The Definitive Guide to Django: Web Development Done Right", int64(3)},
		})

		publishers := orm.Query[agPublisher](db).Annotate(orm.Sum("book__price")).OrderBy("name")
		agAssertRows(t, agNamed(t, publishers, agPublisherName, "book__price__sum"), [][]any{
			{"Apress", orm.MustDecimal("59.69")},
			{"Jonno's House of Books", nil},
			{"Morgan Kaufmann", orm.MustDecimal("75.00")},
			{"Prentice Hall", orm.MustDecimal("112.49")},
			{"Sams", orm.MustDecimal("23.09")},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_values
func TestAggregationAnnotateValues(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		b1 := orm.Query[agBook](db).Filter("pk", f.b1.ID)
		meanAge := orm.Avg("authors__age")
		b1Row := map[string]any{
			"contact_id":   f.a1.ID,
			"id":           f.b1.ID,
			"isbn":         "159059725",
			"mean_age":     34.5,
			"name":         "The Definitive Guide to Django: Web Development Done Right",
			"pages":        int64(447),
			"price":        orm.MustDecimal("30"),
			"pubdate":      orm.Date{Year: 2007, Month: 12, Day: 6},
			"publisher_id": f.p1.ID,
			"rating":       4.5,
		}
		agAssertValues(t, b1.Annotate("mean_age", meanAge).Values(), []map[string]any{b1Row})
		agAssertValues(t, b1.Annotate("mean_age", meanAge).Values("pk", "isbn", "mean_age"),
			[]map[string]any{{"pk": f.b1.ID, "isbn": "159059725", "mean_age": 34.5}})
		agAssertValues(t, b1.Annotate("mean_age", meanAge).Values("name"),
			[]map[string]any{{"name": "The Definitive Guide to Django: Web Development Done Right"}})
		agAssertValues(t, b1.Values().Annotate("mean_age", meanAge), []map[string]any{b1Row})

		books := orm.Query[agBook](db).Values("rating").
			Annotate("n_authors", orm.Count("authors__id"), "mean_age", orm.Avg("authors__age")).OrderBy("rating")
		agAssertValues(t, books, []map[string]any{
			{"rating": 3.0, "n_authors": int64(1), "mean_age": 45.0},
			{"rating": 4.0, "n_authors": int64(6), "mean_age": agApproximate{37.16, 1}},
			{"rating": 4.5, "n_authors": int64(2), "mean_age": 34.5},
			{"rating": 5.0, "n_authors": int64(1), "mean_age": 57.0},
		})

		authors := orm.Query[agAuthor](db).Annotate(orm.Avg("friends__age")).OrderBy("name")
		agAssertRows(t, agNamed(t, authors, agAuthorName, "friends__age__avg"), [][]any{
			{"Adrian Holovaty", 32.0},
			{"Brad Dayley", nil},
			{"Jacob Kaplan-Moss", 29.5},
			{"James Bennett", 34.0},
			{"Jeffrey Forcier", 27.0},
			{"Paul Bissex", 31.0},
			{"Peter Norvig", 46.0},
			{"Stuart Russell", 57.0},
			{"Wesley J. Chun", agApproximate{33.66, 1}},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_count
func TestAggregationCount(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		agAssertMap(t, agAggregate(t, orm.Query[agBook](db), orm.Count("rating")), map[string]any{"rating__count": int64(6)})
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

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_count_distinct_expression
func TestAggregationCountDistinctExpression(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		aggs := agAggregate(t, orm.Query[agBook](db), "distinct_ratings",
			orm.Count(orm.Case(orm.When{Cond: orm.Q("pages__gt", 300), Then: "rating"})).Distinct())
		djangotest.AssertEqual(t, aggs["distinct_ratings"], any(int64(4)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_distinct_on_aggregate
func TestAggregationDistinctOnAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		for _, tc := range []struct {
			name      string
			aggregate func(any) orm.Expr
			want      any
		}{
			{"Avg", orm.Avg, 4.125},
			{"Count", orm.Count, int64(4)},
			{"Sum", orm.Sum, 16.5},
		} {
			t.Run(tc.name, func(t *testing.T) {
				books := agAggregate(t, orm.Query[agBook](db), "ratings", tc.aggregate("rating").Distinct())
				djangotest.AssertEqual(t, books["ratings"], tc.want)
			})
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_distinct_on_stringagg
func TestAggregationDistinctOnStringagg(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("PostgreSQL test: SQLite has no distinct aggregate of several arguments")
		}
		agSetUp(t, db)
		books := agAggregate(t, orm.Query[agBook](db),
			"ratings", orm.StringAgg(orm.Cast(orm.F("rating"), orm.Field{Kind: orm.KindChar}), orm.Value(",")).Distinct())
		ratings := strings.Split(books["ratings"].(string), ",")
		slices.Sort(ratings)
		djangotest.AssertEqual(t, ratings, []string{"3", "4", "4.5", "5"})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_raises_error_on_multiple_argument_distinct
func TestAggregationRaisesErrorOnMultipleArgumentDistinct(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() != "sqlite" {
			t.Skip("SQLite test: PostgreSQL has distinct aggregates of several arguments")
		}
		_, err := orm.Query[agBook](db).Aggregate(context.Background(),
			"ratings", orm.StringAgg(orm.Cast(orm.F("rating"), orm.Field{Kind: orm.KindChar}), orm.Value(",")).Distinct())
		assertErrorContains(t, err, "StringAgg does not support distinct with multiple expressions on this database backend")
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_non_grouped_annotation_not_in_group_by
func TestAggregationNonGroupedAnnotationNotInGroupBy(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		qs := orm.Query[agBook](db).Annotate("xprice", orm.F("price")).Filter("rating", 4.0).Values("rating").
			Annotate("count", orm.Count("publisher_id").Distinct()).OrderBy("count")
		agAssertValues(t, qs, []map[string]any{{"rating": 4.0, "count": int64(2)}})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_grouped_annotation_in_group_by
//
// The final values("count", "rating") of the Django test, which leaves out
// the grouped xprice, has no Go counterpart: a ValuesQuerySet cannot choose
// its keys again. The rows keep xprice, which the test compares too.
func TestAggregationGroupedAnnotationInGroupBy(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		qs := orm.Query[agBook](db).Annotate("xprice", orm.F("price")).Filter("rating", 4.0).Values("rating", "xprice").
			Annotate("count", orm.Count("publisher_id").Distinct()).OrderBy("count")
		agAssertValues(t, qs, []map[string]any{
			{"rating": 4.0, "xprice": orm.MustDecimal("82.80"), "count": int64(1)},
			{"rating": 4.0, "xprice": orm.MustDecimal("29.69"), "count": int64(2)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_fkey_aggregate
func TestAggregationFkeyAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		explicit := agNamed(t, orm.Query[agAuthor](db).Annotate(orm.Count("book__id")).OrderBy("pk"), agAuthorName, "book__id__count")
		implicit := agNamed(t, orm.Query[agAuthor](db).Annotate(orm.Count("book")).OrderBy("pk"), agAuthorName, "book__count")
		djangotest.AssertEqual(t, explicit, implicit)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_ordering
func TestAggregationAnnotateOrdering(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		books := orm.Query[agBook](db).Values("rating").Annotate("oldest", orm.Max("authors__age"))
		agAssertValues(t, books.OrderBy("oldest", "rating"), []map[string]any{
			{"rating": 4.5, "oldest": int64(35)},
			{"rating": 3.0, "oldest": int64(45)},
			{"rating": 4.0, "oldest": int64(57)},
			{"rating": 5.0, "oldest": int64(57)},
		})
		agAssertValues(t, books.OrderBy("-oldest", "-rating"), []map[string]any{
			{"rating": 5.0, "oldest": int64(57)},
			{"rating": 4.0, "oldest": int64(57)},
			{"rating": 3.0, "oldest": int64(45)},
			{"rating": 4.5, "oldest": int64(35)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_annotation
func TestAggregationAggregateAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agBook](db).Annotate("num_authors", orm.Count("authors__id")), orm.Avg("num_authors"))
		agAssertMap(t, vals, map[string]any{"num_authors__avg": agApproximate{1.66, 1}})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_avg_duration_field
//
// The Go API has no output_field, so the two forms of the Django test are
// one.
func TestAggregationAvgDurationField(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		agAssertMap(t, agAggregate(t, orm.Query[agPublisher](db), orm.Avg("duration")),
			map[string]any{"duration__avg": 36 * time.Hour})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_sum_duration_field
func TestAggregationSumDurationField(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		agAssertMap(t, agAggregate(t, orm.Query[agPublisher](db), orm.Sum("duration")),
			map[string]any{"duration__sum": 72 * time.Hour})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_sum_distinct_aggregate
func TestAggregationSumDistinctAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		ctx := context.Background()
		authors := orm.Query[agAuthor](db).Filter("book__in", []int64{f.b5.ID, f.b6.ID})
		n, err := authors.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(3))

		distinctAuthors := authors.Distinct()
		n, err = distinctAuthors.Count(ctx)
		if err != nil {
			t.Fatalf("Count distinct: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(2))

		// Selected author ages are 57 and 46.
		ageSum := agAggregate(t, distinctAuthors, orm.Sum("age"))
		djangotest.AssertEqual(t, ageSum["age__sum"], any(int64(103)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_filtering
func TestAggregationFiltering(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		ctx := context.Background()
		p := create(t, db, &agPublisher{Name: "Expensive Publisher", NumAwards: 0})
		for _, b := range []struct {
			name, isbn, price string
			rating            float64
			day               int
		}{
			{"ExpensiveBook1", "111", "1000", 3.5, 1},
			{"ExpensiveBook2", "222", "1000", 4.0, 2},
			{"ExpensiveBook3", "333", "35", 4.5, 3},
		} {
			book := &agBook{
				Name: b.name, Pages: 1, Isbn: b.isbn, Rating: b.rating, Price: orm.MustDecimal(b.price),
				Pubdate: orm.Date{Year: 2008, Month: 12, Day: b.day},
			}
			book.Publisher.SetPK(p.ID)
			book.Contact.SetPK(f.a1.ID)
			create(t, db, book)
		}
		cheap := orm.MustDecimal("40.0")
		publishers := orm.Query[agPublisher](db)

		djangotest.AssertQuerySetEqual(t,
			publishers.Annotate("num_books", orm.Count("book__id")).Filter("num_books__gt", 1).OrderBy("pk"),
			[]string{"Apress", "Prentice Hall", "Expensive Publisher"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t, publishers.Filter("book__price__lt", cheap).OrderBy("pk"),
			[]string{"Apress", "Apress", "Sams", "Prentice Hall", "Expensive Publisher"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t,
			publishers.Annotate("num_books", orm.Count("book__id")).
				Filter("num_books__gt", 1, "book__price__lt", cheap).OrderBy("pk"),
			[]string{"Apress", "Prentice Hall", "Expensive Publisher"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t,
			publishers.Filter("book__price__lt", cheap).Annotate("num_books", orm.Count("book__id")).
				Filter("num_books__gt", 1).OrderBy("pk"),
			[]string{"Apress"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t,
			publishers.Annotate("num_books", orm.Count("book")).Filter("num_books__range", []int64{1, 3}).OrderBy("pk"),
			[]string{"Apress", "Sams", "Prentice Hall", "Morgan Kaufmann", "Expensive Publisher"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t,
			publishers.Annotate("num_books", orm.Count("book")).Filter("num_books__range", []int64{1, 2}).OrderBy("pk"),
			[]string{"Apress", "Sams", "Prentice Hall", "Morgan Kaufmann"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t,
			publishers.Annotate("num_books", orm.Count("book")).Filter("num_books__in", []int64{1, 3}).OrderBy("pk"),
			[]string{"Sams", "Morgan Kaufmann", "Expensive Publisher"}, agPublisherName)

		n, err := publishers.Annotate("num_books", orm.Count("book")).Filter("num_books__isnull", true).Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(0))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotation
func TestAggregationAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db).Filter("pk", f.a1.ID), orm.Count("friends__id"))
		agAssertMap(t, vals, map[string]any{"friends__id__count": int64(2)})

		djangotest.AssertQuerySetEqual(t,
			orm.Query[agBook](db).Annotate("num_authors", orm.Count("authors__name")).
				Filter("num_authors__exact", 2).OrderBy("pk"),
			[]string{
				"The Definitive Guide to Django: Web Development Done Right",
				"Artificial Intelligence: A Modern Approach",
			}, agBookName)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[agAuthor](db).Annotate("num_friends", orm.Count("friends__id").Distinct()).
				Filter("num_friends", 0).OrderBy("pk"),
			[]string{"Brad Dayley"}, agAuthorName)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[agPublisher](db).Annotate("num_books", orm.Count("book__id")).Filter("num_books__gt", 1).OrderBy("pk"),
			[]string{"Apress", "Prentice Hall"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[agPublisher](db).Filter("book__price__lt", orm.MustDecimal("40.0")).
				Annotate("num_books", orm.Count("book__id")).Filter("num_books__gt", 1),
			[]string{"Apress"}, agPublisherName)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[agBook](db).Annotate("num_authors", orm.Count("authors__id")).
				Filter("authors__name__contains", "Norvig", "num_authors__gt", 1),
			[]string{"Artificial Intelligence: A Modern Approach"}, agBookName)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_more_aggregation
func TestAggregationMoreAggregation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()
		a, err := orm.Query[agAuthor](db).Get(ctx, "name__contains", "Norvig")
		if err != nil {
			t.Fatalf("Get author: %v", err)
		}
		b, err := orm.Query[agBook](db).Get(ctx, "name__contains", "Done Right")
		if err != nil {
			t.Fatalf("Get book: %v", err)
		}
		if err := orm.Many[agAuthor](db, &b, "authors").Add(ctx, &a); err != nil {
			t.Fatalf("add author: %v", err)
		}

		vals := agAggregate(t, orm.Query[agBook](db).Annotate("num_authors", orm.Count("authors__id")).
			Filter("authors__name__contains", "Norvig", "num_authors__gt", 1), orm.Avg("rating"))
		agAssertMap(t, vals, map[string]any{"rating__avg": 4.25})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_even_more_aggregate
func TestAggregationEvenMoreAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		publishers := orm.Query[agPublisher](db).Annotate("earliest_book", orm.Min("book__pubdate")).
			Exclude("earliest_book", nil).OrderBy("earliest_book").Values("earliest_book", "num_awards", "id", "name")
		agAssertValues(t, publishers, []map[string]any{
			{"earliest_book": orm.Date{Year: 1991, Month: 10, Day: 15}, "num_awards": int64(9), "id": f.p4.ID, "name": "Morgan Kaufmann"},
			{"earliest_book": orm.Date{Year: 1995, Month: 1, Day: 15}, "num_awards": int64(7), "id": f.p3.ID, "name": "Prentice Hall"},
			{"earliest_book": orm.Date{Year: 2007, Month: 12, Day: 6}, "num_awards": int64(3), "id": f.p1.ID, "name": "Apress"},
			{"earliest_book": orm.Date{Year: 2008, Month: 3, Day: 3}, "num_awards": int64(1), "id": f.p2.ID, "name": "Sams"},
		})

		vals := agAggregate(t, orm.Query[agStore](db), orm.Max("friday_night_closing"), orm.Min("original_opening"))
		agAssertMap(t, vals, map[string]any{
			"friday_night_closing__max": orm.NewTime(23, 59, 59, 0),
			"original_opening__min":     time.Date(1945, 4, 25, 16, 24, 14, 0, time.UTC),
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotate_values_list
func TestAggregationAnnotateValuesList(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		books := orm.Query[agBook](db).Filter("pk", f.b1.ID).Annotate("mean_age", orm.Avg("authors__age"))
		agAssertValuesList(t, books.ValuesList("pk", "isbn", "mean_age"), [][]any{{f.b1.ID, "159059725", 34.5}})
		agAssertValuesList(t, books.ValuesList("isbn"), [][]any{{"159059725"}})
		agAssertValuesList(t, books.ValuesList("mean_age"), [][]any{{34.5}})

		flat, err := books.ValuesList("mean_age").Flat(context.Background())
		if err != nil {
			t.Fatalf("Flat: %v", err)
		}
		agAssertRows(t, [][]any{flat}, [][]any{{34.5}})

		agAssertValuesList(t,
			orm.Query[agBook](db).ValuesList("price").Annotate("count", orm.Count("price")).OrderBy("-count", "price"),
			[][]any{
				{orm.MustDecimal("29.69"), int64(2)},
				{orm.MustDecimal("23.09"), int64(1)},
				{orm.MustDecimal("30"), int64(1)},
				{orm.MustDecimal("75"), int64(1)},
				{orm.MustDecimal("82.8"), int64(1)},
			})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_dates_with_aggregation
func TestAggregationDatesWithAggregation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		dates, err := orm.Query[agBook](db).Annotate("num_authors", orm.Count("authors")).
			Dates(context.Background(), "pubdate", "year", "ASC")
		if err != nil {
			t.Fatalf("Dates: %v", err)
		}
		djangotest.AssertEqual(t, dates, []orm.Date{
			{Year: 1991, Month: 1, Day: 1},
			{Year: 1995, Month: 1, Day: 1},
			{Year: 2007, Month: 1, Day: 1},
			{Year: 2008, Month: 1, Day: 1},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_values_aggregation
func TestAggregationValuesAggregation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()
		maxRating, err := orm.Query[agBook](db).Values("rating").Aggregate(ctx, "max_rating", orm.Max("rating"))
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, maxRating["max_rating"], any(5.0))

		maxBooksPerRating, err := orm.Query[agBook](db).Values("rating").
			Annotate("books_per_rating", orm.Count("id")).Aggregate(ctx, orm.Max("books_per_rating"))
		if err != nil {
			t.Fatalf("Aggregate over the annotation: %v", err)
		}
		agAssertMap(t, maxBooksPerRating, map[string]any{"books_per_rating__max": int64(3)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_ticket17424
//
// The check of the join types in the alias map of the Django test has no Go
// counterpart: the query keeps no alias map.
func TestAggregationTicket17424(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		allBooks, err := orm.Query[agBook](db).ValuesList("pk").OrderBy("pk").Flat(context.Background())
		if err != nil {
			t.Fatalf("Flat: %v", err)
		}
		annotatedBooks := orm.Query[agBook](db).OrderBy("pk").Annotate("one", orm.Count("id"))
		// The value doesn't matter, we just need any negative constraint on a
		// related model that's a noop.
		excludedBooks := annotatedBooks.Exclude("publisher__name", "__UNLIKELY_VALUE__")
		djangotest.AssertQuerySetEqual(t, excludedBooks, allBooks, func(b agBook) any { return b.ID })
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_ticket12886
func TestAggregationTicket12886(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agBook](db).OrderBy("-rating").Limit(3), "average_top3_rating", orm.Avg("rating"))
		agApprox(t, vals["average_top3_rating"], 4.5, 2)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_decimal_max_digits_has_no_effect
func TestAggregationDecimalMaxDigitsHasNoEffect(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()
		if _, err := orm.Query[agBook](db).Delete(ctx); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		a1, err := orm.Query[agAuthor](db).First(ctx)
		if err != nil {
			t.Fatalf("First author: %v", err)
		}
		p1, err := orm.Query[agPublisher](db).First(ctx)
		if err != nil {
			t.Fatalf("First publisher: %v", err)
		}
		today := orm.NewDate(time.Now().Date())
		for i := range 10 {
			b := &agBook{
				Isbn: fmt.Sprintf("abcde%d", i), Name: "none", Pages: 10, Rating: 4.0,
				Price: orm.MustDecimal("9999.98"), Pubdate: today,
			}
			b.Contact.SetPK(a1.ID)
			b.Publisher.SetPK(p1.ID)
			create(t, db, b)
		}

		book := agAggregate(t, orm.Query[agBook](db), "price_sum", orm.Sum("price"))
		agAssertMap(t, book, map[string]any{"price_sum": orm.MustDecimal("99999.80")})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_nonaggregate_aggregation_throws
func TestAggregationNonaggregateAggregationThrows(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[agBook](db).Aggregate(context.Background(), "fail", orm.F("price"))
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

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_annotation_expressions
func TestAggregationAnnotationExpressions(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		authors := orm.Query[agAuthor](db).Annotate("combined_ages", orm.Sum(orm.F("age").Add(orm.F("friends__age")))).
			OrderBy("name")
		authors2 := orm.Query[agAuthor](db).Annotate("combined_ages", orm.Sum("age").Add(orm.Sum("friends__age"))).
			OrderBy("name")
		for _, qs := range []*orm.QuerySet[agAuthor]{authors, authors2} {
			agAssertRows(t, agNamed(t, qs, agAuthorName, "combined_ages"), [][]any{
				{"Adrian Holovaty", int64(132)},
				{"Brad Dayley", nil},
				{"Jacob Kaplan-Moss", int64(129)},
				{"James Bennett", int64(63)},
				{"Jeffrey Forcier", int64(128)},
				{"Paul Bissex", int64(120)},
				{"Peter Norvig", int64(103)},
				{"Stuart Russell", int64(103)},
				{"Wesley J. Chun", int64(176)},
			})
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_expressions
func TestAggregationAggregationExpressions(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		authors := orm.Query[agAuthor](db)
		agAssertMap(t, agAggregate(t, authors, "av_age", orm.Sum("age").Div(orm.Count("*"))),
			map[string]any{"av_age": int64(37)})
		agAssertMap(t, agAggregate(t, authors, "av_age", orm.Sum("age").Div(orm.Count("age"))),
			map[string]any{"av_age": int64(37)})
		agAssertMap(t, agAggregate(t, authors, "av_age", orm.Avg("age")),
			map[string]any{"av_age": agApproximate{37.4, 1}})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_avg_decimal_field
func TestAggregationAvgDecimalField(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		v := agAggregate(t, orm.Query[agBook](db).Filter("rating", 4), "avg_price", orm.Avg("price"))["avg_price"]
		if _, ok := v.(orm.Decimal); !ok {
			t.Errorf("avg_price = %v (%T), want an orm.Decimal", v, v)
		}
		agAssertRows(t, [][]any{{v}}, [][]any{{agApproximate{47.39, 2}}})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_order_of_precedence
func TestAggregationOrderOfPrecedence(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		books := orm.Query[agBook](db).Filter("rating", 4)
		agAssertMap(t, agAggregate(t, books, "avg_price", orm.Avg("price").Add(2).Mul(3)),
			map[string]any{"avg_price": agApproximate{148.18, 2}})
		agAssertMap(t, agAggregate(t, books, "avg_price", orm.Avg("price").Add(2*3)),
			map[string]any{"avg_price": agApproximate{53.39, 2}})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_combine_different_types
//
// The second half of the Django test sets the output_field of a Sum over
// mixed types, which has no Go counterpart: an expression has no
// output_field, and Cast converts in SQL.
func TestAggregationCombineDifferentTypes(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		msg := "cannot infer type of '+' expression involving these types: FloatField, DecimalField"
		qs := orm.Query[agBook](db).Annotate("sums", orm.Sum("rating").Add(orm.Sum("pages")).Add(orm.Sum("price")))
		_, err := qs.First(context.Background())
		assertFieldError(t, err, msg)
		_, err = qs.First(context.Background())
		assertFieldError(t, err, msg)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_complex_aggregations_require_kwarg
func TestAggregationComplexAggregationsRequireKwarg(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		authors := orm.Query[agAuthor](db)

		_, err := authors.Annotate(orm.Sum(orm.F("age").Add(orm.F("friends__age")))).All(ctx)
		assertErrorContains(t, err, "complex annotations require an alias")
		_, err = authors.Aggregate(ctx, orm.Sum("age").Div(orm.Count("age")))
		assertErrorContains(t, err, "complex aggregates require an alias")
		_, err = authors.Aggregate(ctx, orm.Sum(1))
		assertErrorContains(t, err, "complex aggregates require an alias")
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_over_complex_annotation
func TestAggregationAggregateOverComplexAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		qs := orm.Query[agAuthor](db).Annotate("combined_ages", orm.Sum(orm.F("age").Add(orm.F("friends__age"))))

		age := agAggregate(t, qs, "max_combined_age", orm.Max("combined_ages"))
		djangotest.AssertEqual(t, age["max_combined_age"], any(int64(176)))

		age = agAggregate(t, qs, "max_combined_age_doubled", orm.Max("combined_ages").Mul(2))
		djangotest.AssertEqual(t, age["max_combined_age_doubled"], any(int64(176*2)))

		age = agAggregate(t, qs, "max_combined_age_doubled", orm.Max("combined_ages").Add(orm.Max("combined_ages")))
		djangotest.AssertEqual(t, age["max_combined_age_doubled"], any(int64(176*2)))

		age = agAggregate(t, qs,
			"max_combined_age_doubled", orm.Max("combined_ages").Add(orm.Max("combined_ages")),
			"sum_combined_age", orm.Sum("combined_ages"))
		djangotest.AssertEqual(t, age["max_combined_age_doubled"], any(int64(176*2)))
		djangotest.AssertEqual(t, age["sum_combined_age"], any(int64(954)))

		age = agAggregate(t, qs,
			"max_combined_age_doubled", orm.Max("combined_ages").Add(orm.Max("combined_ages")),
			"sum_combined_age_doubled", orm.Sum("combined_ages").Add(orm.Sum("combined_ages")))
		djangotest.AssertEqual(t, age["max_combined_age_doubled"], any(int64(176*2)))
		djangotest.AssertEqual(t, age["sum_combined_age_doubled"], any(int64(954*2)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_values_annotation_with_expression
//
// A ValuesQuerySet has no Get; a filter on the name reads the one row. The
// final values("name", "combined_age") of the Django test, which leaves out
// the other annotations, has no Go counterpart: a ValuesQuerySet cannot
// choose its keys again.
func TestAggregationValuesAnnotationWithExpression(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()
		get := func(vs *orm.ValuesQuerySet) map[string]any {
			t.Helper()
			rows, err := vs.Filter("name", "Adrian Holovaty").All(ctx)
			if err != nil {
				t.Fatalf("All: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("got %d rows, want 1: %v", len(rows), rows)
			}
			return rows[0]
		}
		// Ensure the F() is promoted to the group by clause.
		qs := orm.Query[agAuthor](db).Values("name").Annotate("another_age", orm.Sum("age").Add(orm.F("age")))
		djangotest.AssertEqual(t, get(qs)["another_age"], any(int64(68)))

		qs = qs.Annotate("friend_count", orm.Count("friends"))
		djangotest.AssertEqual(t, get(qs)["friend_count"], any(int64(2)))

		qs = qs.Annotate("combined_age", orm.Sum("age").Add(orm.F("friends__age"))).
			Filter("name", "Adrian Holovaty").OrderBy("-combined_age")
		agAssertValues(t, qs, []map[string]any{
			{"name": "Adrian Holovaty", "another_age": int64(68), "friend_count": int64(1), "combined_age": int64(69)},
			{"name": "Adrian Holovaty", "another_age": int64(68), "friend_count": int64(1), "combined_age": int64(63)},
		})
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

// No Go counterpart: tests/aggregation/tests.py::AggregateTestCase.test_multi_arg_aggregate
// subclasses Max to take two arguments, and test_add_implementation replaces
// the SQL of a Sum subclass. An aggregate is not a type that Go code extends.

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_complex_values_aggregation
func TestAggregationComplexValuesAggregation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()
		maxRating, err := orm.Query[agBook](db).Values("rating").
			Aggregate(ctx, "double_max_rating", orm.Max("rating").Add(orm.Max("rating")))
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, maxRating["double_max_rating"], any(5.0*2))

		maxBooksPerRating, err := orm.Query[agBook](db).Values("rating").
			Annotate("books_per_rating", orm.Count("id").Add(5)).Aggregate(ctx, orm.Max("books_per_rating"))
		if err != nil {
			t.Fatalf("Aggregate over the annotation: %v", err)
		}
		agAssertMap(t, maxBooksPerRating, map[string]any{"books_per_rating__max": int64(3 + 5)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_expression_on_aggregation
//
// A Cast to a decimal stands in for the output_field=DecimalField() of the
// first Avg, and the second Greatest is a float without an output_field.
func TestAggregationExpressionOnAggregation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		awards := func(p agPublisher) int64 { return p.NumAwards }
		qs := orm.Query[agPublisher](db).
			Annotate("price_or_median", orm.Greatest(
				orm.Cast(orm.Avg("book__rating"), orm.Field{Kind: orm.KindDecimal, MaxDigits: 6, DecimalPlaces: 2}),
				orm.Avg("book__price"))).
			Filter("price_or_median__gte", orm.F("num_awards")).OrderBy("num_awards")
		djangotest.AssertQuerySetEqual(t, qs, []int64{1, 3, 7, 9}, awards)

		qs2 := orm.Query[agPublisher](db).
			Annotate("rating_or_num_awards", orm.Greatest(orm.Avg("book__rating"), orm.F("num_awards"))).
			Filter("rating_or_num_awards__gt", orm.F("num_awards")).OrderBy("num_awards")
		djangotest.AssertQuerySetEqual(t, qs2, []int64{1, 3}, awards)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_arguments_must_be_expressions
//
// An argument of Aggregate is a name followed by an orm.Expr, or an Expr with
// a default alias; anything else is refused with the Go type it has.
func TestAggregationArgumentsMustBeExpressions(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		books := orm.Query[agBook](db)
		_, err := books.Aggregate(ctx, orm.Field{Kind: orm.KindFloat})
		assertErrorContains(t, err, "annotations are names followed by an orm.Expr, got orm.Field")
		_, err = books.Aggregate(ctx, "is_book", true)
		assertErrorContains(t, err, `annotation "is_book" requires an orm.Expr, got bool`)
		_, err = books.Aggregate(ctx, orm.Field{Kind: orm.KindFloat}, orm.Avg("price"), "is_book", true)
		assertErrorContains(t, err, "annotations are names followed by an orm.Expr, got orm.Field")
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

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_subquery_annotation
func TestAggregationAggregationSubqueryAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		// Subquery annotations are excluded from the GROUP BY if they are not
		// explicitly grouped against.
		latest := orm.Query[agBook](db).Filter("publisher", orm.OuterRef("pk")).OrderBy("-pubdate").
			Values("pubdate").Limit(1)
		qs := orm.Query[agPublisher](db).Annotate("latest_book_pubdate", orm.Subquery(latest)).
			Annotate("count", orm.Count("book"))
		capture := djangotest.CaptureQueries(db)
		if _, err := qs.All(context.Background()); err != nil {
			t.Fatalf("All: %v", err)
		}
		capture.Stop()
		djangotest.AssertEqual(t, capture.Len(), 1)
		sql := capture.Queries()[0].SQL
		djangotest.AssertEqual(t, strings.Count(sql, "SELECT"), 2)
		// The GROUP BY should not be by alias either.
		djangotest.AssertEqual(t, strings.Count(strings.ToLower(sql), "latest_book_pubdate"), 1)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_subquery_annotation_exists
func TestAggregationAggregationSubqueryAnnotationExists(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		latest := orm.Query[agBook](db).Filter("publisher", orm.OuterRef("pk")).OrderBy("-pubdate").
			Values("pubdate").Limit(1)
		qs := orm.Query[agPublisher](db).Annotate(
			"latest_book_pubdate", orm.Subquery(latest),
			"count", orm.Count("book"),
		)
		exists, err := qs.Exists(context.Background())
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		djangotest.AssertEqual(t, exists, true)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_filter_exists
//
// Django inspects the GROUP BY of Query.exists() directly. The port reads it
// from the statement of an Exists of the queryset.
func TestAggregationAggregationFilterExists(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		havingMoreThanOneBook := orm.Query[agBook](db).Values("publisher").
			Annotate("cnt", orm.Count("isbn")).Filter("cnt__gt", 1)
		qs := orm.Query[agPublisher](db).Filter(orm.Exists(havingMoreThanOneBook))
		capture := djangotest.CaptureQueries(db)
		n, err := qs.Count(context.Background())
		capture.Stop()
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(5))
		sql := capture.Queries()[0].SQL
		_, groupBy, ok := strings.Cut(sql, " GROUP BY ")
		if !ok {
			t.Fatalf("SQL has no GROUP BY: %s", sql)
		}
		groupBy, _, _ = strings.Cut(groupBy, " HAVING ")
		djangotest.AssertEqual(t, len(strings.Split(groupBy, ", ")), 1)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_exists_annotation
func TestAggregationAggregationExistsAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		published := orm.Query[agBook](db).Filter("publisher", orm.OuterRef("pk"))
		names, err := orm.Query[agPublisher](db).Annotate(
			"published_book", orm.Exists(published),
			"count", orm.Count("book"),
		).ValuesList("name").Flat(context.Background())
		if err != nil {
			t.Fatalf("Flat: %v", err)
		}
		agAssertSameStrings(t, names, []string{
			"Apress", "Morgan Kaufmann", "Jonno's House of Books", "Prentice Hall", "Sams",
		})
	})
}

// agAssertSameStrings checks that got holds the strings of want in any
// order, Django's assertCountEqual.
func agAssertSameStrings(t *testing.T, got []any, want []string) {
	t.Helper()
	strs := make([]string, len(got))
	for i, v := range got {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("value %d is %#v, want a string", i, v)
		}
		strs[i] = s
	}
	slices.Sort(strs)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(strs, want) {
		t.Errorf("got %q, want %q", strs, want)
	}
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_subquery_annotation_values
func TestAggregationAggregationSubqueryAnnotationValues(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		// Subquery annotations and external aliases are excluded from the
		// GROUP BY if they are not selected.
		sameAge := orm.Query[agAuthor](db).Filter("age", orm.OuterRef("contact__friends__age")).
			OrderBy("age").Values("id").Limit(1)
		books := orm.Query[agBook](db).Annotate("first_author_the_same_age", orm.Subquery(sameAge)).
			Filter("publisher", f.p1, "first_author_the_same_age__isnull", false).
			Annotate("min_age", orm.Min("contact__friends__age")).
			Values("name", "min_age").OrderBy("name")
		assertValues(t, books, []map[string]any{
			{"name": "Practical Django Projects", "min_age": int64(34)},
			{"name": "The Definitive Guide to Django: Web Development Done Right", "min_age": int64(29)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_subquery_annotation_values_collision
func TestAggregationAggregationSubqueryAnnotationValuesCollision(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		rating := orm.Query[agBook](db).Filter("pk", orm.OuterRef("book")).Values("rating")
		qs := orm.Query[agPublisher](db).Filter("book__contact__age__gt", 20).
			Annotate("rating", orm.Subquery(rating)).
			Values("rating").Annotate("total_count", orm.Count("*")).OrderBy("rating")
		assertValues(t, qs, []map[string]any{
			{"rating": 3.0, "total_count": int64(1)},
			{"rating": 4.0, "total_count": int64(3)},
			{"rating": 4.5, "total_count": int64(1)},
			{"rating": 5.0, "total_count": int64(1)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_subquery_annotation_multivalued
func TestAggregationAggregationSubqueryAnnotationMultivalued(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()
		// Subquery annotations must be included in the GROUP BY if they use
		// potentially multivalued relations (contain the lookup separator).
		sub := orm.Query[agAuthor](db).Filter(
			"pk", orm.OuterRef("pk"),
			"book__name", orm.OuterRef("book__name"),
		).Values("pk")
		got, err := orm.Query[agAuthor](db).Annotate("subquery_id", orm.Subquery(sub)).
			Annotate("count", orm.Count("book")).Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		want, err := orm.Query[agAuthor](db).Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, got, want)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_order_by_not_selected_annotation_values
func TestAggregationAggregationOrderByNotSelectedAnnotationValues(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		asc := []any{f.b4.ID, f.b3.ID, f.b1.ID, f.b2.ID, f.b5.ID, f.b6.ID}
		desc := slices.Clone(asc)
		slices.Reverse(desc)
		for _, tc := range []struct {
			ordering any
			want     []any
		}{
			{"min_related_age", asc},
			{"-min_related_age", desc},
			{orm.F("min_related_age"), asc},
			{orm.F("min_related_age").Asc(), asc},
			{orm.F("min_related_age").Desc(), desc},
		} {
			t.Run(fmt.Sprint(tc.ordering), func(t *testing.T) {
				books := orm.Query[agBook](db).Annotate("min_age", orm.Min("authors__age")).
					Annotate("min_related_age", orm.Coalesce("min_age", "contact__age")).
					OrderBy(tc.ordering).ValuesList("pk")
				assertFlat(t, books, tc.want)
			})
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_group_by_subquery_annotation
//
// Django also groups by the queryset and by its Query, which Python resolves
// to the same subquery. A Go queryset becomes an expression through Subquery
// only.
func TestAggregationGroupBySubqueryAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		// Subquery annotations are included in the GROUP BY if they are
		// grouped against.
		longBooksCount := orm.Query[agBook](db).Filter("publisher", orm.OuterRef("pk"), "pages__gt", 400).
			Values("publisher").Annotate("count", orm.Count("pk")).Values("count")
		breakdown := orm.Query[agPublisher](db).Annotate("long_books_count", orm.Subquery(longBooksCount)).
			ValuesList("long_books_count").Annotate("total", orm.Count("*"))
		agAssertBreakdown(t, breakdown, map[any]int64{nil: 1, int64(1): 4})
	})
}

// agAssertBreakdown checks the rows of a ValuesList of a key and a count,
// in any order, as Django compares dict() of them.
func agAssertBreakdown(t *testing.T, vl *orm.ValuesListQuerySet, want map[any]int64) {
	t.Helper()
	rows, err := vl.All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	got := make(map[any]int64, len(rows))
	for _, row := range rows {
		n, ok := row[1].(int64)
		if !ok {
			t.Fatalf("count %#v is not an int64", row[1])
		}
		got[row[0]] = n
	}
	if !maps.Equal(got, want) {
		t.Errorf("breakdown = %v, want %v", got, want)
	}
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_group_by_exists_annotation
func TestAggregationGroupByExistsAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		// Exists annotations are included in the GROUP BY if they are grouped
		// against.
		longBooks := orm.Query[agBook](db).Filter("publisher", orm.OuterRef("pk"), "pages__gt", 800)
		breakdown := orm.Query[agPublisher](db).Annotate("has_long_books", orm.Exists(longBooks)).
			ValuesList("has_long_books").Annotate("total", orm.Count("*"))
		agAssertBreakdown(t, breakdown, map[any]int64{true: 2, false: 3})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_group_by_nested_expression_with_params
//
// The final values_list("least", flat=True) of the Django test reads the
// "least" key of the rows.
func TestAggregationGroupByNestedExpressionWithParams(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "postgres" {
			t.Skip("PostgreSQL binds parameters on the server, where Django marks this test an expected failure: " +
				"the parameter of the GROUP BY expression and the one in the SELECT are different parameters (#34255)")
		}
		agSetUp(t, db)
		rows, err := orm.Query[agBook](db).Annotate("greatest_pages", orm.Greatest("pages", orm.Value(600))).
			Values("greatest_pages").
			Annotate("min_pages", orm.Min("pages"), "least", orm.Least("min_pages", "greatest_pages")).
			All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		got := make([]int64, len(rows))
		for i, r := range rows {
			got[i], _ = r["least"].(int64)
		}
		slices.Sort(got)
		djangotest.AssertEqual(t, got, []int64{300, 946, 1132})
	})
}

// agGreatestPages returns the books grouped by the larger of their pages and
// 600, the values(greatest_pages=Greatest("pages", 600)) of the AnyValue
// tests.
func agGreatestPages(db *orm.DB) *orm.ValuesQuerySet {
	return orm.Query[agBook](db).Annotate("greatest_pages", orm.Greatest("pages", orm.Value(600))).
		Values("greatest_pages")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_any_value
//
// The values_list("pubdate_year", flat=True) of the Django test reads the
// "pubdate_year" key of the rows.
func TestAggregationAnyValue(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		rows, err := agGreatestPages(db).Annotate("pubdate_year", orm.AnyValue("pubdate__year")).
			OrderBy("pubdate_year").All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("got %d rows, want 3: %v", len(rows), rows)
		}
		first := []any{rows[0]["pubdate_year"], rows[1]["pubdate_year"]}
		djangotest.AssertEqual(t, first, []any{int64(1991), int64(1995)})
		if y := rows[2]["pubdate_year"]; y != int64(2007) && y != int64(2008) {
			t.Errorf("pubdate_year = %v, want 2007 or 2008", y)
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_any_value_filter
func TestAggregationAnyValueFilter(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		rows, err := agGreatestPages(db).
			Annotate("pubdate_year", orm.AnyValue("pubdate__year").Filter("rating__lte", 4.5)).
			OrderBy("greatest_pages").All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		got := make([]any, len(rows))
		for i, r := range rows {
			got[i] = r["pubdate_year"]
		}
		djangotest.AssertEqual(t, got, []any{int64(2007), nil, int64(1995)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_any_value_aggregate_clause
//
// The values_list("pages_per_author", flat=True) of the Django test reads the
// "pages_per_author" key of the rows.
func TestAggregationAnyValueAggregateClause(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		books := agGreatestPages(db).Annotate(
			"num_authors", orm.Count("authors"),
			"pages_per_author", orm.AnyValue("greatest_pages").Div(orm.Cast("num_authors", orm.Field{Kind: orm.KindFloat})),
		).OrderBy("pages_per_author")
		rows, err := books.All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("got %d rows, want 3: %v", len(rows), rows)
		}
		agApprox(t, rows[0]["pages_per_author"], 600.0/7, 4)
		agApprox(t, rows[1]["pages_per_author"], 1132.0/2, 4)
		agApprox(t, rows[2]["pages_per_author"], 946.0/1, 4)

		vals, err := books.Aggregate(context.Background(), orm.Avg("pages_per_author"))
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		agApprox(t, vals["pages_per_author__avg"], (600.0/7+1132.0/2+946.0/1)/3, 4)
	})
}

// No Go counterpart: tests/aggregation/tests.py::AggregateTestCase.test_any_value_not_supported
// runs on a backend without ANY_VALUE. PostgreSQL has it from version 16 on,
// and the SQLite backend registers one.

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_subquery_annotation_related_field
func TestAggregationAggregationSubqueryAnnotationRelatedField(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		ctx := context.Background()
		publisher := create(t, db, &agPublisher{Name: f.a9.Name, NumAwards: 2})
		book := &agBook{
			Isbn: "159059999", Name: "Test book.", Pages: 819, Rating: 2.5, Price: orm.MustDecimal("14.44"),
			Pubdate: orm.Date{Year: 2019, Month: 12, Day: 6},
		}
		book.Contact.SetPK(f.a9.ID)
		book.Publisher.SetPK(publisher.ID)
		create(t, db, book)
		if err := orm.Many[agAuthor](db, book, "authors").Add(ctx, f.a5, f.a6, f.a7); err != nil {
			t.Fatalf("add authors: %v", err)
		}
		contactPublisher := orm.Query[agPublisher](db).Filter(
			"pk", orm.OuterRef("publisher"),
			"name", orm.OuterRef("contact__name"),
		).Values("name").Limit(1)
		books := orm.Query[agBook](db).Annotate("contact_publisher", orm.Subquery(contactPublisher)).
			Filter("contact_publisher__isnull", false).
			Annotate("count", orm.Count("authors"))
		capture := djangotest.CaptureQueries(db)
		got, err := books.All(ctx)
		capture.Stop()
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(got) != 1 || got[0].ID != book.ID {
			t.Fatalf("books = %v, want the test book", got)
		}
		djangotest.AssertEqual(t, capture.Len(), 1)
		djangotest.AssertEqual(t, strings.Count(capture.Queries()[0].SQL, "SELECT"), 3)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_nested_subquery_outerref
//
// Django's values(publisher_count=Count("id")) annotates and selects in one
// call; the port calls Annotate and Values.
func TestAggregationAggregationNestedSubqueryOuterref(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		sameName := orm.Query[agPublisher](db).Filter("id__in", orm.Subquery(
			orm.Query[agPublisher](db).Filter("name", orm.OuterRef(orm.OuterRef("publisher__name"))).Values("id"),
		)).Annotate("publisher_count", orm.Count("id")).Values("publisher_count").Limit(1)
		breakdown := orm.Query[agBook](db).Annotate(
			"publisher_count", orm.Subquery(sameName),
			"authors_count", orm.Count("authors"),
		).ValuesList("publisher_count")
		assertFlat(t, breakdown, []any{int64(1), int64(1), int64(1), int64(1), int64(1), int64(1)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_exists_multivalued_outeref
func TestAggregationAggregationExistsMultivaluedOuteref(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		ctx := context.Background()
		got, err := orm.Query[agPublisher](db).Annotate(
			"books_exists", orm.Exists(orm.Query[agBook](db).Filter("publisher", orm.OuterRef("book__publisher"))),
			"books_count", orm.Count("book"),
		).ValuesList("name").Flat(ctx)
		if err != nil {
			t.Fatalf("Flat: %v", err)
		}
		agAssertSameStrings(t, got, []string{
			"Apress", "Morgan Kaufmann", "Jonno's House of Books", "Prentice Hall", "Sams",
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_filter_in_subquery_or_aggregation
func TestAggregationFilterInSubqueryOrAggregation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		authors := orm.Query[agAuthor](db).Annotate(orm.Count("book")).
			Filter(orm.Q("book__count__gt", 0).Or(orm.Q("pk__in", orm.Query[agBook](db).Values("authors"))))
		djangotest.AssertQuerySetEqual(t, authors, []string{
			"Adrian Holovaty", "Jacob Kaplan-Moss", "Brad Dayley", "James Bennett", "Jeffrey Forcier",
			"Paul Bissex", "Wesley J. Chun", "Peter Norvig", "Stuart Russell",
		}, agAuthorName, djangotest.Unordered())
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_random_ordering
func TestAggregationAggregationRandomOrdering(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		got := agNamed(t, orm.Query[agAuthor](db).Annotate("contact_count", orm.Count("book")).OrderBy("?"),
			agAuthorName, "contact_count")
		slices.SortFunc(got, func(a, b []any) int { return strings.Compare(a[0].(string), b[0].(string)) })
		agAssertRows(t, got, [][]any{
			{"Adrian Holovaty", int64(1)},
			{"Brad Dayley", int64(1)},
			{"Jacob Kaplan-Moss", int64(1)},
			{"James Bennett", int64(1)},
			{"Jeffrey Forcier", int64(1)},
			{"Paul Bissex", int64(1)},
			{"Peter Norvig", int64(2)},
			{"Stuart Russell", int64(1)},
			{"Wesley J. Chun", int64(1)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_order_by_aggregate_transform
//
// The Django test registers the transform on the output field of the Sum;
// here it is registered on the big integer kind, the kind of an integer
// annotation.
func TestAggregationOrderByAggregateTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindBigInt, "mod100", func(arg any) orm.Expr {
		return arg.(orm.Expr).Mod(100)
	}))
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		publisherPages := orm.Query[agBook](db).Values("publisher").
			Annotate("sum_pages", orm.Sum("pages")).OrderBy("sum_pages__mod100")
		agAssertValues(t, publisherPages, []map[string]any{
			{"publisher": f.p2.ID, "sum_pages": int64(528)},
			{"publisher": f.p4.ID, "sum_pages": int64(946)},
			{"publisher": f.p1.ID, "sum_pages": int64(747)},
			{"publisher": f.p3.ID, "sum_pages": int64(1482)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_order_by_aggregate_default_alias
//
// A ValuesQuerySet has no ValuesList; the rows keep the count next to the
// "book" key that the final values_list("book", flat=True) reads.
func TestAggregationOrderByAggregateDefaultAlias(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		publisherBooks := orm.Query[agPublisher](db).Values("book").Annotate(orm.Count("book")).
			OrderBy("book__count", "book__id")
		want := make([]map[string]any, 0, 7)
		want = append(want, map[string]any{"book": nil, "book__count": int64(0)})
		for _, b := range []*agBook{f.b1, f.b2, f.b3, f.b4, f.b5, f.b6} {
			want = append(want, map[string]any{"book": b.ID, "book__count": int64(1)})
		}
		agAssertValues(t, publisherBooks, want)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_empty_result_optimization
//
// An arithmetic expression stands in for the Func whose value over no rows
// Django cannot tell.
func TestAggregationEmptyResultOptimization(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		none := orm.Query[agPublisher](db).None()

		djangotest.AssertNumQueries(t, db, 0, func() {
			vals := agAggregate(t, none, "sum_awards", orm.Sum("num_awards"), "books_count", orm.Count("book"),
				"all_names", orm.StringAgg("name", orm.Value(",")))
			djangotest.AssertEqual(t, len(vals), 3)
			djangotest.AssertEqual(t, vals["sum_awards"], nil)
			djangotest.AssertEqual(t, vals["books_count"], any(int64(0)))
			djangotest.AssertEqual(t, vals["all_names"], nil)
		})
		// Expression without empty_result_set_value forces queries to be
		// executed even if they would return an empty result set.
		djangotest.AssertNumQueries(t, db, 1, func() {
			vals := agAggregate(t, none,
				"sum_awards", orm.Sum("num_awards"), "books_count", orm.Count("book").Add(0))
			djangotest.AssertEqual(t, len(vals), 2)
			djangotest.AssertEqual(t, vals["sum_awards"], nil)
			djangotest.AssertEqual(t, vals["books_count"], any(int64(0)))
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_coalesced_empty_result_set
//
// The last part of the Django test needs RawSQL, which the ORM does not have;
// TestAggregationEmptyResultOptimization covers an expression that forces the
// query to run.
func TestAggregationCoalescedEmptyResultSet(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		none := orm.Query[agPublisher](db).None()
		for _, tc := range []struct {
			name string
			expr orm.Expr
		}{
			{"one expression", orm.Coalesce(orm.Sum("num_awards"), 0)},
			{"multiple expressions", orm.Coalesce(orm.Sum("num_awards"), nil, 0)},
			{"nested coalesce", orm.Coalesce(orm.Coalesce(orm.Sum("num_awards"), nil), 0)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				djangotest.AssertNumQueries(t, db, 0, func() {
					vals := agAggregate(t, none, "sum_awards", tc.expr)
					djangotest.AssertEqual(t, vals["sum_awards"], any(int64(0)))
				})
			})
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_unsupported_by_count
//
// Django refuses the default when Count is built; the ORM refuses it when
// the query runs.
func TestAggregationAggregationDefaultUnsupportedByCount(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[agAuthor](db).Aggregate(context.Background(), "value", orm.Count("age").Default(0))
		assertErrorContains(t, err, "Count does not allow default")
	})
}

// agDefaultAggregates are the aggregates of the default tests: Avg, Max, Min,
// StdDev, Sum and Variance.
var agDefaultAggregates = []struct {
	name string
	agg  func(arg any) orm.Expr
}{
	{"Avg", orm.Avg},
	{"Max", orm.Max},
	{"Min", orm.Min},
	{"StdDev", func(arg any) orm.Expr { return orm.StdDev(arg, false) }},
	{"Sum", orm.Sum},
	{"Variance", func(arg any) orm.Expr { return orm.Variance(arg, false) }},
}

// agAggregateDefaults runs each of agDefaultAggregates over the authors older
// than 100, with the default that withDefault adds, and checks the value
// against want, a number or nil.
func agAggregateDefaults(t *testing.T, withDefault func(orm.Expr) orm.Expr, want any) {
	t.Helper()
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		for _, a := range agDefaultAggregates {
			t.Run(a.name, func(t *testing.T) {
				vals := agAggregate(t, orm.Query[agAuthor](db).Filter("age__gt", 100),
					"value", withDefault(a.agg("age")))
				if want == nil {
					djangotest.AssertEqual(t, vals["value"], nil)
					return
				}
				agAssertMap(t, vals, map[string]any{"value": want})
			})
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_unset
func TestAggregationAggregationDefaultUnset(t *testing.T) {
	agAggregateDefaults(t, func(e orm.Expr) orm.Expr { return e }, nil)
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_zero
func TestAggregationAggregationDefaultZero(t *testing.T) {
	agAggregateDefaults(t, func(e orm.Expr) orm.Expr { return e.Default(0) }, agApproximate{0, 6})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_integer
func TestAggregationAggregationDefaultInteger(t *testing.T) {
	agAggregateDefaults(t, func(e orm.Expr) orm.Expr { return e.Default(21) }, agApproximate{21, 6})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_expression
func TestAggregationAggregationDefaultExpression(t *testing.T) {
	agAggregateDefaults(t, func(e orm.Expr) orm.Expr {
		return e.Default(orm.Value(5).Mul(orm.Value(7)))
	}, agApproximate{35, 6})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_stringagg_default_value
func TestAggregationStringaggDefaultValue(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db).Filter("age__gt", 100),
			"value", orm.StringAgg("name", orm.Value(";")).Default(orm.Value("<empty>")))
		djangotest.AssertEqual(t, vals["value"], any("<empty>"))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_group_by
func TestAggregationAggregationDefaultGroupBy(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		qs := orm.Query[agPublisher](db).Values("name").
			Annotate("books", orm.Count("book"), "pages", orm.Sum("book__pages").Default(0)).
			Filter("books", 0)
		assertValues(t, qs, []map[string]any{
			{"name": "Jonno's House of Books", "books": int64(0), "pages": int64(0)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_compound_expression
func TestAggregationAggregationDefaultCompoundExpression(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		// Scale rating to a percentage; default to 50% if no books published.
		formula := orm.Avg("book__rating").Default(2.5).Mul(20.0)
		qs := orm.Query[agPublisher](db).Annotate("rating", formula).OrderBy("name")
		assertValues(t, qs.Values("name", "rating"), []map[string]any{
			{"name": "Apress", "rating": 85.0},
			{"name": "Jonno's House of Books", "rating": 50.0},
			{"name": "Morgan Kaufmann", "rating": 100.0},
			{"name": "Prentice Hall", "rating": 80.0},
			{"name": "Sams", "rating": 60.0},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_time_from_python
func TestAggregationAggregationDefaultUsingTimeFromPython(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		expr := orm.Min("store__friday_night_closing").Filter(orm.Not(orm.Q("store__name", "Amazon.com"))).
			Default(orm.NewTime(17, 0, 0, 0))
		qs := orm.Query[agBook](db).Annotate("oldest_store_opening", expr).OrderBy("isbn")
		assertValues(t, qs.Values("isbn", "oldest_store_opening"), []map[string]any{
			{"isbn": "013235613", "oldest_store_opening": orm.NewTime(21, 30, 0, 0)},
			{"isbn": "013790395", "oldest_store_opening": orm.NewTime(23, 59, 59, 0)},
			{"isbn": "067232959", "oldest_store_opening": orm.NewTime(17, 0, 0, 0)},
			{"isbn": "155860191", "oldest_store_opening": orm.NewTime(21, 30, 0, 0)},
			{"isbn": "159059725", "oldest_store_opening": orm.NewTime(23, 59, 59, 0)},
			{"isbn": "159059996", "oldest_store_opening": orm.NewTime(21, 30, 0, 0)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_time_from_database
func TestAggregationAggregationDefaultUsingTimeFromDatabase(t *testing.T) {
	t.Skip("needs TruncHour and Now, which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_date_from_python
func TestAggregationAggregationDefaultUsingDateFromPython(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		expr := orm.Min("book__pubdate").Default(orm.Date{Year: 1970, Month: 1, Day: 1})
		qs := orm.Query[agPublisher](db).Annotate("earliest_pubdate", expr).OrderBy("name")
		assertValues(t, qs.Values("name", "earliest_pubdate"), []map[string]any{
			{"name": "Apress", "earliest_pubdate": orm.Date{Year: 2007, Month: 12, Day: 6}},
			{"name": "Jonno's House of Books", "earliest_pubdate": orm.Date{Year: 1970, Month: 1, Day: 1}},
			{"name": "Morgan Kaufmann", "earliest_pubdate": orm.Date{Year: 1991, Month: 10, Day: 15}},
			{"name": "Prentice Hall", "earliest_pubdate": orm.Date{Year: 1995, Month: 1, Day: 15}},
			{"name": "Sams", "earliest_pubdate": orm.Date{Year: 2008, Month: 3, Day: 3}},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_date_from_database
func TestAggregationAggregationDefaultUsingDateFromDatabase(t *testing.T) {
	t.Skip("needs TruncDate and Now, which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_datetime_from_python
func TestAggregationAggregationDefaultUsingDatetimeFromPython(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		expr := orm.Min("store__original_opening").Filter(orm.Not(orm.Q("store__name", "Amazon.com"))).
			Default(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		qs := orm.Query[agBook](db).Annotate("oldest_store_opening", expr).OrderBy("isbn")
		mamma := time.Date(1945, 4, 25, 16, 24, 14, 0, time.UTC)
		books := time.Date(2001, 3, 15, 11, 23, 37, 0, time.UTC)
		assertValues(t, qs.Values("isbn", "oldest_store_opening"), []map[string]any{
			{"isbn": "013235613", "oldest_store_opening": mamma},
			{"isbn": "013790395", "oldest_store_opening": books},
			{"isbn": "067232959", "oldest_store_opening": time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)},
			{"isbn": "155860191", "oldest_store_opening": mamma},
			{"isbn": "159059725", "oldest_store_opening": books},
			{"isbn": "159059996", "oldest_store_opening": mamma},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_datetime_from_database
func TestAggregationAggregationDefaultUsingDatetimeFromDatabase(t *testing.T) {
	t.Skip("needs TruncHour and Now, which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_duration_from_python
func TestAggregationAggregationDefaultUsingDurationFromPython(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agPublisher](db).Filter("num_awards__gt", 3),
			"value", orm.Sum("duration").Default(time.Duration(0)))
		djangotest.AssertEqual(t, vals["value"], any(time.Duration(0)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_duration_from_database
func TestAggregationAggregationDefaultUsingDurationFromDatabase(t *testing.T) {
	t.Skip("needs Now, which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_decimal_from_python
func TestAggregationAggregationDefaultUsingDecimalFromPython(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agBook](db).Filter("rating__lt", 3.0),
			"value", orm.Sum("price").Default(orm.MustDecimal("0.00")))
		agAssertMap(t, vals, map[string]any{"value": orm.MustDecimal("0.00")})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_using_decimal_from_database
func TestAggregationAggregationDefaultUsingDecimalFromDatabase(t *testing.T) {
	t.Skip("needs Pi, which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_passed_another_aggregate
func TestAggregationAggregationDefaultPassedAnotherAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agBook](db),
			"value", orm.Sum("price").Filter("rating__lt", 3.0).Default(orm.Avg("pages").Div(10.0)))
		agAssertMap(t, vals, map[string]any{"value": agApproximate{61.72, 2}})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_after_annotation
func TestAggregationAggregationDefaultAfterAnnotation(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agPublisher](db).Annotate("double_num_awards", orm.F("num_awards").Mul(2)),
			"value", orm.Sum("double_num_awards").Default(0))
		djangotest.AssertEqual(t, vals["value"], any(int64(40)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_aggregation_default_not_in_aggregate
func TestAggregationAggregationDefaultNotInAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agPublisher](db).Annotate("avg_rating", orm.Avg("book__rating").Default(2.5)),
			orm.Sum("num_awards"))
		djangotest.AssertEqual(t, vals["num_awards__sum"], any(int64(20)))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_exists_none_with_aggregate
func TestAggregationExistsNoneWithAggregate(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		books, err := orm.Query[agBook](db).Annotate(
			"count", orm.Count("id"),
			"exists", orm.Exists(orm.Query[agAuthor](db).None()),
		).All(context.Background())
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		djangotest.AssertEqual(t, len(books), 6)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_alias_sql_injection
func TestAggregationAliasSQLInjection(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		crafted := `injected_name" from "aggregation_author"; --`
		_, err := orm.Query[agAuthor](db).Aggregate(context.Background(), crafted, orm.Avg("age"))
		assertErrorContains(t, err, "column aliases cannot contain whitespace characters, hashes, "+
			"quotation marks, semicolons, or SQL comments")
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_exists_extra_where_with_aggregate
func TestAggregationExistsExtraWhereWithAggregate(t *testing.T) {
	t.Skip("needs extra(), which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_multiple_aggregate_references
func TestAggregationMultipleAggregateReferences(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db),
			"total_books", orm.Count("book"),
			"coalesced_total_books", orm.Coalesce("total_books", 0))
		agAssertMap(t, vals, map[string]any{"total_books": int64(10), "coalesced_total_books": int64(10)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_group_by_transform_column
func TestAggregationGroupByTransformColumn(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		qs := orm.Query[agStore](db).Values("original_opening__date", "name").
			Annotate(orm.Count("books")).OrderBy("name")
		assertValues(t, qs, []map[string]any{
			{"original_opening__date": orm.Date{Year: 1994, Month: 4, Day: 23}, "name": "Amazon.com", "books__count": int64(6)},
			{"original_opening__date": orm.Date{Year: 2001, Month: 3, Day: 15}, "name": "Books.com", "books__count": int64(4)},
			{"original_opening__date": orm.Date{Year: 1945, Month: 4, Day: 25}, "name": "Mamma and Pappa's Books", "books__count": int64(3)},
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_group_by_reference_subquery
//
// The Django subquery groups by the annotation with a count and then selects
// the annotation alone. A ValuesQuerySet cannot choose its keys again, so the
// subquery here selects the annotation without the count.
func TestAggregationGroupByReferenceSubquery(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agSetUp(t, db)
		authors := orm.Query[agAuthor](db).Annotate("publisher_id", orm.F("book__publisher")).
			Values("publisher_id")
		qs := orm.Query[agPublisher](db).Filter("pk__in", authors)
		djangotest.AssertQuerySetEqual(t, qs, []int64{f.p1.ID, f.p2.ID, f.p3.ID, f.p4.ID},
			func(p agPublisher) int64 { return p.ID }, djangotest.Unordered())
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

// No Go counterpart: tests/aggregation/tests.py::AggregateTestCase.test_aggregate_arity
// and test_string_agg_requires_delimiter pass the wrong number of arguments
// to an aggregate, which the Go compiler refuses.

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_string_agg_escapes_delimiter
func TestAggregationStringAggEscapesDelimiter(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agPublisher](db), "stringagg", orm.StringAgg("name", orm.Value("'")))
		agAssertMap(t, vals, map[string]any{
			"stringagg": "Apress'Sams'Prentice Hall'Morgan Kaufmann'Jonno's House of Books",
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_string_agg_order_by
func TestAggregationStringAggOrderBy(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		const oldestFirst = "Mamma and Pappa's Books;Amazon.com;Books.com"
		const newestFirst = "Books.com;Amazon.com;Mamma and Pappa's Books"
		for _, tc := range []struct {
			orderBy any
			want    string
		}{
			{orm.F("original_opening").Desc(), newestFirst},
			{orm.F("original_opening").Asc(), oldestFirst},
			{orm.F("original_opening"), oldestFirst},
			{"original_opening", oldestFirst},
			{"-original_opening", newestFirst},
			{orm.Concat("original_opening", orm.Value("@")), oldestFirst},
			{orm.Concat("original_opening", orm.Value("@")).Desc(), newestFirst},
		} {
			t.Run(fmt.Sprint(tc.orderBy), func(t *testing.T) {
				vals := agAggregate(t, orm.Query[agStore](db),
					"stringagg", orm.StringAgg("name", orm.Value(";")).OrderBy(tc.orderBy))
				agAssertMap(t, vals, map[string]any{"stringagg": tc.want})
			})
		}
	})
}

// No Go counterpart: tests/aggregation/tests.py::AggregateTestCase.test_string_agg_order_by_is_not_supported
// runs on a backend that cannot order the values of an aggregate. PostgreSQL
// and SQLite both can.

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_string_agg_filter
func TestAggregationStringAggFilter(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		vals := agAggregate(t, orm.Query[agBook](db),
			"stringagg", orm.StringAgg("name", orm.Value(";")).Filter("name__startswith", "P"))
		agAssertMap(t, vals, map[string]any{
			"stringagg": "Practical Django Projects;Python Web Development with Django;" +
				"Paradigms of Artificial Intelligence Programming: Case Studies in Common Lisp",
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_string_agg_filter_outerref
func TestAggregationStringAggFilterOuterref(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		names := orm.Query[agBook](db).Annotate("stringagg",
			orm.StringAgg("name", orm.Value(";")).OrderBy(orm.OuterRef("num_awards")),
		).Values("stringagg").Limit(1)
		values := orm.Query[agPublisher](db).Annotate("stringagg", orm.Subquery(names)).
			Values("stringagg").OrderBy("id")
		row := map[string]any{"stringagg": "The Definitive Guide to Django: Web Development Done Right"}
		assertValues(t, values, []map[string]any{row, row, row, row, row})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_string_agg_jsonfield_order_by
func TestAggregationStringAggJsonfieldOrderBy(t *testing.T) {
	t.Skip("needs the key transforms of JSONField, which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_string_agg_filter_in_subquery
func TestAggregationStringAggFilterInSubquery(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		agg := orm.StringAgg("authors__name", orm.Value(";")).Filter(orm.Not(orm.Q("authors__name__startswith", "J")))
		sub := orm.Query[agBook](db).Filter("pk", orm.OuterRef("pk")).Annotate("agg", agg).Values("agg")
		values, err := orm.Query[agBook](db).Annotate("agg", orm.Subquery(sub)).ValuesList("agg").
			Flat(context.Background())
		if err != nil {
			t.Fatalf("Flat: %v", err)
		}
		var nulls int
		strs := make([]any, 0, len(values))
		for _, v := range values {
			if v == nil {
				nulls++
				continue
			}
			strs = append(strs, v)
		}
		djangotest.AssertEqual(t, nulls, 1)
		agAssertSameStrings(t, strs, []string{
			"Adrian Holovaty",
			"Brad Dayley",
			"Paul Bissex;Wesley J. Chun",
			"Peter Norvig;Stuart Russell",
			"Peter Norvig",
		})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateTestCase.test_order_by_in_subquery
func TestAggregationOrderByInSubquery(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agSetUp(t, db)
		agg := orm.StringAgg("authors__name", orm.Value(";")).OrderBy("authors__name")
		sub := orm.Query[agBook](db).Filter("pk", orm.OuterRef("pk")).Annotate("agg", agg).Values("agg")
		values := orm.Query[agBook](db).Annotate("agg", orm.Subquery(sub)).OrderBy("agg").ValuesList("agg")
		assertFlat(t, values, []any{
			"Adrian Holovaty;Jacob Kaplan-Moss",
			"Brad Dayley",
			"James Bennett",
			"Jeffrey Forcier;Paul Bissex;Wesley J. Chun",
			"Peter Norvig",
			"Peter Norvig;Stuart Russell",
		})
	})
}

// agPruningFixture holds the rows of AggregateAnnotationPruningTests.setUpTestData.
type agPruningFixture struct {
	a1, a2 *agAuthor
	p1, p2 *agPublisher
}

// agPruningSetUp creates the rows of
// AggregateAnnotationPruningTests.setUpTestData.
func agPruningSetUp(t *testing.T, db *orm.DB) agPruningFixture {
	t.Helper()
	var f agPruningFixture
	f.a1 = create(t, db, &agAuthor{Age: 1})
	f.a2 = create(t, db, &agAuthor{Age: 2})
	f.p1 = create(t, db, &agPublisher{NumAwards: 1})
	f.p2 = create(t, db, &agPublisher{NumAwards: 0})
	b1 := agPruningBook(t, db, "b1", f.p1, 100, 4.5, "10", f.a1)
	if err := orm.Many[agAuthor](db, b1, "authors").Add(context.Background(), f.a1); err != nil {
		t.Fatalf("add authors: %v", err)
	}
	b2 := agPruningBook(t, db, "b2", f.p2, 1000, 3.2, "50", f.a2)
	if err := orm.Many[agAuthor](db, b2, "authors").Add(context.Background(), f.a1, f.a2); err != nil {
		t.Fatalf("add authors: %v", err)
	}
	return f
}

// agPruningBook creates a book of the pruning tests, published today.
func agPruningBook(t *testing.T, db *orm.DB, name string, publisher *agPublisher, pages int64, rating float64,
	price string, contact *agAuthor,
) *agBook {
	t.Helper()
	now := time.Now()
	b := &agBook{
		Name: name, Pages: pages, Rating: rating, Price: orm.MustDecimal(price),
		Pubdate: orm.Date{Year: now.Year(), Month: now.Month(), Day: now.Day()},
	}
	b.Publisher.SetPK(publisher.ID)
	b.Contact.SetPK(contact.ID)
	return create(t, db, b)
}

// agCount runs Count and fails the test on an error.
func agCount[T any](t *testing.T, qs *orm.QuerySet[T]) int64 {
	t.Helper()
	n, err := qs.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	return n
}

// agOnlySQL returns the lowercased SQL of the one query that fn runs.
func agOnlySQL(t *testing.T, db *orm.DB, fn func()) string {
	t.Helper()
	capture := djangotest.CaptureQueries(db)
	fn()
	capture.Stop()
	if capture.Len() != 1 {
		t.Fatalf("ran %d queries, want 1", capture.Len())
	}
	return strings.ToLower(capture.Queries()[0].SQL)
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_unused_aliased_aggregate_pruned
func TestAggregateAnnotationPruningUnusedAliasedAggregatePruned(t *testing.T) {
	t.Skip("needs alias(), which the ORM does not have")
}

// agGoodBook is the Case of the reverse foreign key pruning tests: whether a
// book of the publisher is rated above 4.
func agGoodBook() orm.Expr {
	return orm.Case(orm.When{Cond: orm.Q("book__rating__gt", 4.0), Then: orm.Value(true)}).Default(orm.Value(false))
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_unused_aliased_aggregate_and_annotation_reverse_fk
func TestAggregateAnnotationPruningUnusedAliasedAggregateAndAnnotationReverseFk(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agPruningSetUp(t, db)
		agPruningBook(t, db, "b3", f.p2, 1000, 4.2, "50", f.a2)
		qs := orm.Query[agPublisher](db).Annotate("total_pages", orm.Sum("book__pages"), "good_book", agGoodBook())
		djangotest.AssertEqual(t, agCount(t, qs), int64(3))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_unused_aliased_aggregate_and_annotation_reverse_fk_grouped
func TestAggregateAnnotationPruningUnusedAliasedAggregateAndAnnotationReverseFkGrouped(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		f := agPruningSetUp(t, db)
		agPruningBook(t, db, "b3", f.p2, 1000, 4.2, "50", f.a2)
		qs := orm.Query[agPublisher](db).Values("id", "name").
			Annotate("total_pages", orm.Sum("book__pages")).
			Annotate("good_book", agGoodBook())
		n, err := qs.Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(3))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_non_aggregate_annotation_pruned
func TestAggregateAnnotationPruningNonAggregateAnnotationPruned(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		sql := agOnlySQL(t, db, func() {
			agCount(t, orm.Query[agBook](db).Annotate("name_lower", orm.Lower("name")))
		})
		djangotest.AssertEqual(t, strings.Count(sql, "select"), 1)
		if strings.Contains(sql, "name_lower") {
			t.Errorf("sql = %q, want it without name_lower", sql)
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_unreferenced_aggregate_annotation_pruned
func TestAggregateAnnotationPruningUnreferencedAggregateAnnotationPruned(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		var n int64
		sql := agOnlySQL(t, db, func() {
			n = agCount(t, orm.Query[agBook](db).Annotate("authors_count", orm.Count("authors")))
		})
		djangotest.AssertEqual(t, n, int64(2))
		djangotest.AssertEqual(t, strings.Count(sql, "select"), 2)
		if strings.Contains(sql, "authors_count") {
			t.Errorf("sql = %q, want it without authors_count", sql)
		}
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_referenced_aggregate_annotation_kept
func TestAggregateAnnotationPruningReferencedAggregateAnnotationKept(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		sql := agOnlySQL(t, db, func() {
			agAggregate(t, orm.Query[agBook](db).Annotate("authors_count", orm.Count("authors")),
				orm.Avg("authors_count"))
		})
		djangotest.AssertEqual(t, strings.Count(sql, "select"), 2)
		djangotest.AssertEqual(t, strings.Count(sql, "authors_count"), 2)
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_referenced_group_by_annotation_kept
func TestAggregateAnnotationPruningReferencedGroupByAnnotationKept(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		qs := orm.Query[agBook](db).Annotate("pages_mod", orm.F("pages").Mod(10)).Values("pages_mod").
			Annotate("mod_count", orm.Count("*"))
		n, err := qs.Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(1))
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_referenced_subquery_requires_wrapping
//
// Django counts the books of an author on the through model of Book.authors,
// which has no Go type; the port counts them on Author through the reverse
// relation, which reads the same through table.
func TestAggregateAnnotationPruningReferencedSubqueryRequiresWrapping(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		totalBooks := orm.Query[agAuthor](db).Values("pk").Filter("pk", orm.OuterRef("pk")).
			Annotate("total", orm.Count("book"))
		qs := orm.Query[agAuthor](db).Annotate("total_books", orm.Subquery(totalBooks.Values("total"))).Values("pk", "total_books")
		capture := djangotest.CaptureQueries(db)
		aggregate, err := qs.Aggregate(context.Background(), "sum_total_books", orm.Sum("total_books"))
		capture.Stop()
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, capture.Len(), 1)
		sql := strings.ToLower(capture.Queries()[0].SQL)
		if n := strings.Count(sql, "select"); n != 3 {
			t.Errorf("SQL has %d SELECTs, want 3 (subquery wrapping required): %s", n, sql)
		}
		agAssertMap(t, aggregate, map[string]any{"sum_total_books": int64(3)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_referenced_composed_subquery_requires_wrapping
//
// Django counts the books of an author on the through model of Book.authors,
// which has no Go type; the port counts them on Author through the reverse
// relation, which reads the same through table.
func TestAggregateAnnotationPruningReferencedComposedSubqueryRequiresWrapping(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		totalBooks := orm.Query[agAuthor](db).Values("pk").Filter("pk", orm.OuterRef("pk")).
			Annotate("total", orm.Count("book"))
		qs := orm.Query[agAuthor](db).Annotate(
			"total_books", orm.Subquery(totalBooks.Values("total")),
			"total_books_ref", orm.F("total_books").Div(1),
		).Values("pk", "total_books_ref")
		capture := djangotest.CaptureQueries(db)
		aggregate, err := qs.Aggregate(context.Background(), "sum_total_books", orm.Sum("total_books_ref"))
		capture.Stop()
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		djangotest.AssertEqual(t, capture.Len(), 1)
		sql := strings.ToLower(capture.Queries()[0].SQL)
		if n := strings.Count(sql, "select"); n != 3 {
			t.Errorf("SQL has %d SELECTs, want 3 (subquery wrapping required): %s", n, sql)
		}
		agAssertMap(t, aggregate, map[string]any{"sum_total_books": int64(3)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_referenced_window_requires_wrapping
func TestAggregateAnnotationPruningReferencedWindowRequiresWrapping(t *testing.T) {
	t.Skip("needs Window, which the ORM does not have")
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_aggregate_reference_lookup_rhs
func TestAggregateAnnotationPruningAggregateReferenceLookupRhs(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db).Annotate("max_book_author", orm.Max("book__authors")),
			"count", orm.Count("id").Filter("id", orm.F("max_book_author")))
		agAssertMap(t, vals, map[string]any{"count": int64(1)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_aggregate_reference_lookup_rhs_iter
func TestAggregateAnnotationPruningAggregateReferenceLookupRhsIter(t *testing.T) {
	djangotest.EachDB(t, agModels(), func(t *testing.T, db *orm.DB) {
		agPruningSetUp(t, db)
		vals := agAggregate(t, orm.Query[agAuthor](db).Annotate("max_book_author", orm.Max("book__authors")),
			"count", orm.Count("id").Filter("id__in", []any{orm.F("max_book_author"), 0}))
		agAssertMap(t, vals, map[string]any{"count": int64(1)})
	})
}

// Ported from Django: tests/aggregation/tests.py::AggregateAnnotationPruningTests.test_aggregate_combined_queries
func TestAggregateAnnotationPruningAggregateCombinedQueries(t *testing.T) {
	t.Skip("needs union(), which the ORM does not have")
}

// No Django test exists: the Go types of Min and Max over a date, a text and
// an integer field, and grouping, filtering and ordering together on one
// queryset.
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
				{"publisher_id": f.p1.ID, "n": int64(1)},
				{"publisher_id": f.p2.ID, "n": int64(1)},
				{"publisher_id": f.p3.ID, "n": int64(1)},
				{"publisher_id": f.p4.ID, "n": int64(1)},
			})

		vals := agAggregate(t, books,
			"contacts", orm.Count("contact").Distinct(),
			"first", orm.Min("pubdate"), "last_name", orm.Max("name"), "least", orm.Min("pages"))
		djangotest.AssertEqual(t, vals["contacts"], any(int64(5)))
		djangotest.AssertEqual(t, vals["last_name"],
			any("The Definitive Guide to Django: Web Development Done Right"))
		djangotest.AssertEqual(t, vals["least"], any(int64(300)))
		djangotest.AssertEqual(t, vals["first"], any(orm.Date{Year: 1991, Month: 10, Day: 15}))

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
