package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// ordAuthor is Django's ordering.Author, without its Meta.ordering.
type ordAuthor struct {
	ID     int64
	Name   string            `orm:"max_length=63;null;blank"`
	Editor orm.FK[ordAuthor] `orm:"null"`
}

// ordArticle is Django's ordering.Article, without its Meta.ordering.
type ordArticle struct {
	ID           int64
	Author       orm.FK[ordAuthor] `orm:"null;on_delete=set_null"`
	SecondAuthor orm.FK[ordAuthor] `orm:"null;on_delete=set_null"`
	Headline     string            `orm:"max_length=100"`
	PubDate      time.Time
}

// ordModels returns the models of Django's ordering test app.
func ordModels() []any {
	return []any{djangotest.App("ordering", &ordAuthor{}, &ordArticle{})}
}

// ordFixture is the data of Django's OrderingTests.
type ordFixture struct {
	a1, a2, a3, a4     *ordArticle
	author1, author2   *ordAuthor
	unnamed1, unnamed2 *ordAuthor
}

// ordSetUp creates the OrderingTests fixture.
func ordSetUp(t *testing.T, db *orm.DB) ordFixture {
	t.Helper()
	return ordFixture{
		a1:       create(t, db, &ordArticle{Headline: "Article 1", PubDate: day(2005, 7, 26)}),
		a2:       create(t, db, &ordArticle{Headline: "Article 2", PubDate: day(2005, 7, 27)}),
		a3:       create(t, db, &ordArticle{Headline: "Article 3", PubDate: day(2005, 7, 27)}),
		a4:       create(t, db, &ordArticle{Headline: "Article 4", PubDate: day(2005, 7, 28)}),
		author1:  create(t, db, &ordAuthor{Name: "Name 1"}),
		author2:  create(t, db, &ordAuthor{Name: "Name 2"}),
		unnamed1: create(t, db, &ordAuthor{}),
		unnamed2: create(t, db, &ordAuthor{}),
	}
}

// ordHeadline is the transform of the ordering ports.
func ordHeadline(a ordArticle) string { return a.Headline }

// ordAssert checks the headlines of a queryset, in order.
func ordAssert(t *testing.T, qs *orm.QuerySet[ordArticle], want ...string) {
	t.Helper()
	djangotest.AssertQuerySetEqual(t, qs, want, ordHeadline)
}

// ordSetAuthor points an article at an author and saves that one field.
func ordSetAuthor(t *testing.T, db *orm.DB, article *ordArticle, author *ordAuthor) {
	t.Helper()
	article.Author = fkTo[ordAuthor](author.ID)
	if err := orm.Query[ordArticle](db).Save(context.Background(), article, orm.UpdateFields("author")); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_default_ordering_override
func TestOrderingDefaultOrderingOverride(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("headline"),
			"Article 1", "Article 2", "Article 3", "Article 4")
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("pub_date", "-headline"),
			"Article 1", "Article 3", "Article 2", "Article 4")
	})
}

// The choices lack Django's reverse relations "article" and "author", which
// belong to another work package.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_default_ordering_override_unknown_field
func TestOrderingDefaultOrderingOverrideUnknownField(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		_, err := orm.Query[ordArticle](db).OrderBy("author__unknown_field").All(context.Background())
		assertFieldError(t, err,
			"resolve keyword 'unknown_field' into field. Choices are: editor, editor_id, id, name")
	})
}

// Django's order_by replaces the ordering. OrderBy appends to it, and an
// empty OrderBy clears it: the port asserts both.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_override
func TestOrderingOrderByOverride(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("id"),
			"Article 1", "Article 2", "Article 3", "Article 4")
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("id").OrderBy().OrderBy("-headline"),
			"Article 4", "Article 3", "Article 2", "Article 1")
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("pub_date").OrderBy("-headline"),
			"Article 1", "Article 3", "Article 2", "Article 4")
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_stop_slicing
func TestOrderingStopSlicing(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("headline").Limit(2), "Article 1", "Article 2")
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_stop_start_slicing
func TestOrderingStopStartSlicing(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("headline").Offset(1).Limit(2), "Article 2", "Article 3")
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_random_ordering
func TestOrderingRandomOrdering(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		djangotest.AssertEqual(t, len(mustAll(t, orm.Query[ordArticle](db).OrderBy("?"))), 4)
	})
}

// Django reverses the Meta.ordering of Article here. Without one, the port
// spells that ordering out.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_reversed_ordering
func TestOrderingReversedOrdering(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("-pub_date", "headline").Reverse().Limit(2),
			"Article 1", "Article 3")
	})
}

// Django orders by F("headline").asc(). OrderBy takes keys only.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_reverse_ordering_pure
func TestOrderingReverseOrderingPure(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		qs1 := orm.Query[ordArticle](db).OrderBy("headline")
		qs2 := qs1.Reverse()
		ordAssert(t, qs2, "Article 4", "Article 3", "Article 2", "Article 1")
		ordAssert(t, qs1, "Article 1", "Article 2", "Article 3", "Article 4")
	})
}

// Django compares the Meta.ordering "-pk" of Author with order_by("-pk").
// Here "pk" is checked against the primary key column.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_pk
func TestOrderingOrderByPK(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		id := func(a ordAuthor) int64 { return a.ID }
		djangotest.AssertQuerySetEqual(t, orm.Query[ordAuthor](db).OrderBy("-pk"),
			[]int64{fx.unnamed2.ID, fx.unnamed1.ID, fx.author2.ID, fx.author1.ID}, id)
		djangotest.AssertQuerySetEqual(t, orm.Query[ordAuthor](db).OrderBy("pk"),
			[]int64{fx.author1.ID, fx.author2.ID, fx.unnamed1.ID, fx.unnamed2.ID}, id)
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_fk_attname
func TestOrderingOrderByFKAttname(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		authors := mustAll(t, orm.Query[ordAuthor](db).OrderBy("id"))
		for i, article := range []*ordArticle{fx.a4, fx.a3, fx.a2, fx.a1} {
			ordSetAuthor(t, db, article, &authors[i])
		}
		ordAssert(t, orm.Query[ordArticle](db).OrderBy("author_id"),
			"Article 4", "Article 3", "Article 2", "Article 1")
	})
}

// Django orders "author__editor" by the Meta.ordering of Author. Without one
// the relation key orders by the foreign key column, like "author__editor_id".
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_self_referential_fk
func TestOrderingOrderBySelfReferentialFK(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		ordSetAuthor(t, db, fx.a1, create(t, db, &ordAuthor{Editor: fkTo[ordAuthor](fx.author1.ID)}))
		ordSetAuthor(t, db, fx.a2, create(t, db, &ordAuthor{Editor: fkTo[ordAuthor](fx.author2.ID)}))
		articles := orm.Query[ordArticle](db).Filter("author__isnull", false)
		ordAssert(t, articles.OrderBy("author__editor"), "Article 1", "Article 2")
		ordAssert(t, articles.OrderBy("-author__editor"), "Article 2", "Article 1")
		ordAssert(t, articles.OrderBy("author__editor_id"), "Article 1", "Article 2")
	})
}

// Django orders by F("headline").asc() and F("headline").desc(). OrderBy
// takes keys only.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_f_expression_duplicates
func TestOrderingOrderByFExpressionDuplicates(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		count := func(qs *orm.QuerySet[ordArticle]) int {
			query, _, err := orm.CompileSelectForTest(qs)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			query = strings.ToUpper(query)
			return strings.Count(query[strings.Index(query, "ORDER BY"):], "HEADLINE")
		}
		qs := orm.Query[ordArticle](db).OrderBy("headline", "-headline")
		djangotest.AssertEqual(t, count(qs), 1)
		ordAssert(t, qs, "Article 1", "Article 2", "Article 3", "Article 4")
		qs = orm.Query[ordArticle](db).OrderBy("-headline", "headline")
		djangotest.AssertEqual(t, count(qs), 1)
		ordAssert(t, qs, "Article 4", "Article 3", "Article 2", "Article 1")
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_default_ordering_does_not_affect_group_by
func TestOrderingDefaultOrderingDoesNotAffectGroupBy(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		ctx := context.Background()
		if _, err := orm.Query[ordArticle](db).Exclude("headline", "Article 4").Update(ctx, "author", fx.author1); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if _, err := orm.Query[ordArticle](db).Filter("headline", "Article 4").Update(ctx, "author", fx.author2); err != nil {
			t.Fatalf("Update: %v", err)
		}
		assertValues(t, orm.Query[ordArticle](db).Values("author").Annotate("count", orm.Count("author")).OrderBy("author"),
			[]map[string]any{
				{"author": fx.author1.ID, "count": int64(3)},
				{"author": fx.author2.ID, "count": int64(1)},
			})
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_default_ordering
func TestOrderingDefaultOrdering(t *testing.T) {
	t.Skip("needs Meta.ordering, which belongs to the model options work package")
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_reverse_meta_ordering_pure
func TestOrderingReverseMetaOrderingPure(t *testing.T) {
	t.Skip("needs Meta.ordering, which belongs to the model options work package")
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_related_ordering_duplicate_table_reference
func TestOrderingRelatedOrderingDuplicateTableReference(t *testing.T) {
	t.Skip("needs Meta.ordering and O2O[T], which belong to other work packages")
}

// A term that spans a nullable relation keeps the rows without one.
func TestOrderingByNullableRelationKeepsRows(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		ordSetAuthor(t, db, fx.a3, fx.author2)
		ordSetAuthor(t, db, fx.a4, fx.author1)
		ordAssert(t, orm.Query[ordArticle](db).Filter("author__isnull", false).OrderBy("author__name"),
			"Article 4", "Article 3")
		djangotest.AssertEqual(t, len(mustAll(t, orm.Query[ordArticle](db).OrderBy("author__name", "headline"))), 4)
	})
}
