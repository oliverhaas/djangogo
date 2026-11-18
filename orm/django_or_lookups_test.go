package orm_test

import (
	"context"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// orlArticle is Article of Django's tests/or_lookups/models.py. Its Meta
// ordering ("pub_date") is not available yet, so the ports order explicitly.
type orlArticle struct {
	ID       int64
	Headline string `orm:"max_length=50"`
	PubDate  time.Time
}

// orlFixture is the data of OrLookupsTests.setUpTestData.
type orlFixture struct {
	a1, a2, a3 int64
}

// orlModels returns the models of the or_lookups app.
func orlModels() []any {
	return []any{djangotest.App("or_lookups", &orlArticle{})}
}

// orlSetUp creates the three articles of OrLookupsTests.setUpTestData.
func orlSetUp(t *testing.T, db *orm.DB) orlFixture {
	t.Helper()
	return orlFixture{
		a1: create(t, db, &orlArticle{Headline: "Hello", PubDate: day(2005, 11, 27)}).ID,
		a2: create(t, db, &orlArticle{Headline: "Goodbye", PubDate: day(2005, 11, 28)}).ID,
		a3: create(t, db, &orlArticle{Headline: "Hello and goodbye", PubDate: day(2005, 11, 29)}).ID,
	}
}

// orlArticles returns the articles in the model's default order.
func orlArticles(db *orm.DB) *orm.QuerySet[orlArticle] {
	return orm.Query[orlArticle](db).OrderBy("pub_date")
}

// orlHeadline is the transform of the ports: attrgetter("headline").
func orlHeadline(a orlArticle) string { return a.Headline }

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_filter_or
//
// Django ORs two querysets with "|". A Go queryset has no such operator, so
// the port ORs the conditions with Q(...).Or(...), which is the same query.
func TestFilterOr(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		orlSetUp(t, db)
		all := []string{"Hello", "Goodbye", "Hello and goodbye"}

		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("headline__startswith", "Hello").Or(orm.Q("headline__startswith", "Goodbye"))),
			all, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("headline__contains", "Hello").Or(orm.Q("headline__contains", "bye"))),
			all, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("headline__iexact", "Hello").Or(orm.Q("headline__contains", "ood"))),
			all, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("headline__startswith", "Hello").Or(orm.Q("headline__startswith", "Goodbye"))),
			all, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_stages
//
// Django ANDs two querysets with "&"; chaining Filter calls builds the same
// query in stages.
func TestStages(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		orlSetUp(t, db)
		articles := orlArticles(db)

		djangotest.AssertQuerySetEqual(t,
			articles.Filter("headline__startswith", "Hello").Filter("headline__startswith", "Goodbye"),
			[]string{}, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			articles.Filter("headline__startswith", "Hello").Filter("headline__contains", "bye"),
			[]string{"Hello and goodbye"}, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_pk_q
func TestPkQ(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		f := orlSetUp(t, db)

		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("pk", f.a1).Or(orm.Q("pk", f.a2))),
			[]string{"Hello", "Goodbye"}, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("pk", f.a1).Or(orm.Q("pk", f.a2), orm.Q("pk", f.a3))),
			[]string{"Hello", "Goodbye", "Hello and goodbye"}, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_pk_in
//
// The list and tuple forms of Django are a slice and an array here.
func TestPkIn(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		f := orlSetUp(t, db)
		all := []string{"Hello", "Goodbye", "Hello and goodbye"}

		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter("pk__in", []int64{f.a1, f.a2, f.a3}), all, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter("pk__in", [3]int64{f.a1, f.a2, f.a3}), all, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter("pk__in", []int64{f.a1, f.a2, f.a3, 40000}), all, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_q_repr
//
// The Go form is Cond.String, which prints the tree inside Django's
// "<Q: ...>" wrapper. The value is a string here rather than a model instance,
// whose Python repr has no Go counterpart.
func TestQRepr(t *testing.T) {
	orExpr := orm.Q("baz", "Foö")
	djangotest.AssertEqual(t, orExpr.String(), "(AND: ('baz', 'Foö'))")
	negatedOr := orm.Not(orm.Q("baz", "Foö"))
	djangotest.AssertEqual(t, negatedOr.String(), "(NOT (AND: ('baz', 'Foö')))")
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_q_negated
func TestQNegated(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		f := orlSetUp(t, db)

		// Q objects can be negated.
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("pk", f.a1).Or(orm.Not(orm.Q("pk", f.a2)))),
			[]string{"Hello", "Hello and goodbye"}, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Not(orm.Q("pk", f.a1)).And(orm.Not(orm.Q("pk", f.a2)))),
			[]string{"Hello and goodbye"}, orlHeadline)
		// This allows for more complex queries than filter() and exclude() alone
		// would allow.
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(
				orm.Q("pk", f.a1).And(orm.Not(orm.Q("pk", f.a2)).Or(orm.Q("pk", f.a3)))),
			[]string{"Hello"}, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_empty_in
func TestEmptyIn(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		orlSetUp(t, db)

		// Passing "in" an empty list returns no results ...
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter("pk__in", []int64{}), []string{}, orlHeadline)
		// ... but can return results if we OR it with another query.
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("pk__in", []int64{}).Or(orm.Q("headline__icontains", "goodbye"))),
			[]string{"Goodbye", "Hello and goodbye"}, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_q_and
func TestQAnd(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		orlSetUp(t, db)

		// Q arg objects are ANDed.
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("headline__startswith", "Hello"), orm.Q("headline__contains", "bye")),
			[]string{"Hello and goodbye"}, orlHeadline)
		// Q arg AND order is irrelevant.
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(orm.Q("headline__contains", "bye"), "headline__startswith", "Hello"),
			[]string{"Hello and goodbye"}, orlHeadline)
		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Filter(
				orm.Q("headline__startswith", "Hello").And(orm.Q("headline__startswith", "Goodbye"))),
			[]string{}, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_q_exclude
func TestQExclude(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		orlSetUp(t, db)

		djangotest.AssertQuerySetEqual(t,
			orlArticles(db).Exclude(orm.Q("headline__startswith", "Hello")),
			[]string{"Goodbye"}, orlHeadline)
	})
}

// Ported from Django: tests/or_lookups/tests.py::OrLookupsTests.test_other_arg_queries
//
// in_bulk() is the function orm.InBulk, since a Go method cannot be generic
// over the key type.
func TestOtherArgQueries(t *testing.T) {
	djangotest.EachDB(t, orlModels(), func(t *testing.T, db *orm.DB) {
		f := orlSetUp(t, db)
		ctx := context.Background()

		// Try some arg queries with operations other than filter.
		got, err := orm.Query[orlArticle](db).Get(ctx,
			orm.Q("headline__startswith", "Hello"), orm.Q("headline__contains", "bye"))
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.Headline, "Hello and goodbye")

		djangotest.AssertEqual(t, mustCount(t, orm.Query[orlArticle](db).Filter(
			orm.Q("headline__startswith", "Hello").Or(orm.Q("headline__contains", "bye")))), int64(3))

		rows, err := orm.Query[orlArticle](db).Filter(
			orm.Q("headline__startswith", "Hello"), orm.Q("headline__contains", "bye")).Values().All(ctx)
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("Values returned %d rows, want 1", len(rows))
		}
		djangotest.AssertEqual(t, len(rows[0]), 3)
		djangotest.AssertEqual(t, rows[0]["headline"], any("Hello and goodbye"))
		djangotest.AssertEqual(t, rows[0]["id"], any(f.a3))
		pubDate, ok := rows[0]["pub_date"].(time.Time)
		if !ok || !pubDate.Equal(day(2005, 11, 29)) {
			t.Errorf("pub_date = %#v, want %v", rows[0]["pub_date"], day(2005, 11, 29))
		}

		bulk, err := orm.InBulk(ctx,
			orm.Query[orlArticle](db).Filter(orm.Q("headline__startswith", "Hello")), []int64{f.a1, f.a2})
		if err != nil {
			t.Fatalf("InBulk: %v", err)
		}
		djangotest.AssertEqual(t, len(bulk), 1)
		djangotest.AssertEqual(t, bulk[f.a1].Headline, "Hello")
	})
}
