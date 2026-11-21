package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// ordAuthor is Django's ordering.Author with Meta.ordering = ("-pk",).
type ordAuthor struct {
	ID     int64
	Name   string            `orm:"max_length=63;null;blank"`
	Editor orm.FK[ordAuthor] `orm:"null;on_delete=cascade"`
}

// Meta sets Author's default ordering to descending primary key.
func (ordAuthor) Meta() orm.Meta { return orm.Meta{Ordering: []string{"-pk"}} }

// ordArticle is Django's ordering.Article with its Meta.ordering.
type ordArticle struct {
	ID           int64
	Author       orm.FK[ordAuthor] `orm:"null;on_delete=set_null"`
	SecondAuthor orm.FK[ordAuthor] `orm:"null;on_delete=set_null"`
	Headline     string            `orm:"max_length=100"`
	PubDate      time.Time
}

// Meta sets Article's default ordering matching Django's ordering.Article.
func (ordArticle) Meta() orm.Meta {
	return orm.Meta{
		Ordering: []string{"-pub_date", "headline", "author__name", "second_author__name"},
	}
}

// ordByAuthorArticle mirrors Django's OrderedByAuthorArticle, ordering by
// two FKs to ordAuthor (which itself orders by -pk).
type ordByAuthorArticle struct {
	ID           int64
	Author       orm.FK[ordAuthor] `orm:"null;on_delete=set_null"`
	SecondAuthor orm.FK[ordAuthor] `orm:"null;on_delete=set_null"`
	Headline     string            `orm:"max_length=100"`
	PubDate      time.Time
}

// Meta sets default ordering to author, second_author.
func (ordByAuthorArticle) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"author", "second_author"}}
}

// ordReference mirrors Django's ordering.Reference (using FK for Proof since
// O2O belongs to a later item).
type ordReference struct {
	ID      int64
	Article orm.FK[ordByAuthorArticle] `orm:"on_delete=cascade"`
	Proof   orm.FK[ordByAuthorArticle] `orm:"on_delete=cascade"`
}

// Meta sets default ordering to article.
func (ordReference) Meta() orm.Meta { return orm.Meta{Ordering: []string{"article"}} }

// ordModels returns the models of Django's ordering test app.
func ordModels() []any {
	return []any{djangotest.App("ordering", &ordAuthor{}, &ordArticle{}, &ordByAuthorArticle{}, &ordReference{})}
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

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_default_ordering
func TestOrderingDefaultOrdering(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db),
			"Article 4", "Article 2", "Article 3", "Article 1")

		first, err := orm.Query[ordArticle](db).First(context.Background())
		if err != nil {
			t.Fatalf("First: %v", err)
		}
		djangotest.AssertEqual(t, first.ID, fx.a4.ID)

		last, err := orm.Query[ordArticle](db).Last(context.Background())
		if err != nil {
			t.Fatalf("Last: %v", err)
		}
		djangotest.AssertEqual(t, last.ID, fx.a1.ID)

		// OrderBy() with no arguments clears Meta.ordering.
		sqlText, _, err := orm.CompileSelectForTest(orm.Query[ordArticle](db).OrderBy())
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if strings.Contains(strings.ToUpper(sqlText), "ORDER BY") {
			t.Errorf("OrderBy() did not clear Meta.ordering: %s", sqlText)
		}
	})
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

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_default_ordering_override_unknown_field
func TestOrderingDefaultOrderingOverrideUnknownField(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		_, err := orm.Query[ordArticle](db).OrderBy("author__unknown_field").All(context.Background())
		assertFieldError(t, err,
			"resolve keyword 'unknown_field' into field. Choices are: editor, editor_id, id, name, ordarticle, ordauthor")
	})
}

// Django's order_by replaces the ordering. OrderBy overrides Meta.ordering on
// the first call, appends on subsequent calls, and clears when called empty.
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

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_reversed_ordering
func TestOrderingReversedOrdering(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db).Reverse().Limit(2),
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

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_reverse_meta_ordering_pure
func TestOrderingReverseMetaOrderingPure(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		create(t, db, &ordArticle{
			Headline:     "Article 5",
			PubDate:      day(2005, 7, 30),
			Author:       fkTo[ordAuthor](fx.author1.ID),
			SecondAuthor: fkTo[ordAuthor](fx.author2.ID),
		})
		create(t, db, &ordArticle{
			Headline:     "Article 5",
			PubDate:      day(2005, 7, 30),
			Author:       fkTo[ordAuthor](fx.author2.ID),
			SecondAuthor: fkTo[ordAuthor](fx.author1.ID),
		})
		authorName := func(a ordArticle) string {
			author, _ := a.Author.Object()
			return author.Name
		}
		djangotest.AssertQuerySetEqual(t,
			orm.Query[ordArticle](db).SelectRelated("author").Filter("headline", "Article 5").Reverse(),
			[]string{"Name 2", "Name 1"}, authorName)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[ordArticle](db).SelectRelated("author").Filter("headline", "Article 5"),
			[]string{"Name 1", "Name 2"}, authorName)
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_pk
func TestOrderingOrderByPK(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		id := func(a ordAuthor) int64 { return a.ID }
		djangotest.AssertQuerySetEqual(t, orm.Query[ordAuthor](db),
			[]int64{fx.unnamed2.ID, fx.unnamed1.ID, fx.author2.ID, fx.author1.ID}, id)
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

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_self_referential_fk
func TestOrderingOrderBySelfReferentialFK(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		ordSetAuthor(t, db, fx.a1, create(t, db, &ordAuthor{Editor: fkTo[ordAuthor](fx.author1.ID)}))
		ordSetAuthor(t, db, fx.a2, create(t, db, &ordAuthor{Editor: fkTo[ordAuthor](fx.author2.ID)}))
		articles := orm.Query[ordArticle](db).Filter("author__isnull", false)
		ordAssert(t, articles.OrderBy("author__editor"), "Article 2", "Article 1")
		ordAssert(t, articles.OrderBy("-author__editor"), "Article 1", "Article 2")
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
		assertValues(t, orm.Query[ordArticle](db).Values("author").Annotate("count", orm.Count("author")).OrderBy("author_id"),
			[]map[string]any{
				{"author": fx.author1.ID, "count": int64(3)},
				{"author": fx.author2.ID, "count": int64(1)},
			})
		// Ordering by "author" inherits ordAuthor's Meta.Ordering ("-pk").
		assertValues(t, orm.Query[ordArticle](db).Values("author").Annotate("count", orm.Count("author")).OrderBy("author"),
			[]map[string]any{
				{"author": fx.author2.ID, "count": int64(1)},
				{"author": fx.author1.ID, "count": int64(3)},
			})
		// Without an explicit OrderBy, Meta.ordering still must not leak into GROUP BY.
		rows, err := orm.Query[ordArticle](db).Values("author").Annotate("count", orm.Count("author")).All(ctx)
		if err != nil {
			t.Fatalf("Values.Annotate: %v", err)
		}
		djangotest.AssertEqual(t, len(rows), 2)
	})
}

// Django's Reference.proof is a OneToOneField; here it is a ForeignKey because
// O2O belongs to a later item. The duplicate-table ordering traversal is the
// same.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_related_ordering_duplicate_table_reference
func TestOrderingRelatedOrderingDuplicateTableReference(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		firstAuthor := create(t, db, &ordAuthor{})
		secondAuthor := create(t, db, &ordAuthor{})
		a1 := create(t, db, &ordByAuthorArticle{
			Headline:     "A1",
			PubDate:      day(2005, 7, 26),
			Author:       fkTo[ordAuthor](firstAuthor.ID),
			SecondAuthor: fkTo[ordAuthor](secondAuthor.ID),
		})
		a2 := create(t, db, &ordByAuthorArticle{
			Headline:     "A2",
			PubDate:      day(2005, 7, 27),
			Author:       fkTo[ordAuthor](secondAuthor.ID),
			SecondAuthor: fkTo[ordAuthor](firstAuthor.ID),
		})
		r1 := create(t, db, &ordReference{Article: fkTo[ordByAuthorArticle](a1.ID), Proof: fkTo[ordByAuthorArticle](a1.ID)})
		r2 := create(t, db, &ordReference{Article: fkTo[ordByAuthorArticle](a2.ID), Proof: fkTo[ordByAuthorArticle](a2.ID)})
		djangotest.AssertQuerySetEqual(t, orm.Query[ordReference](db),
			[]int64{r2.ID, r1.ID}, func(r ordReference) int64 { return r.ID })
	})
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

// ordBarcodedArticle is Django's ordering.BarcodedArticle. Its UniqueConstraint
// declarations have no counterpart in orm.Meta; the unique_together set does.
type ordBarcodedArticle struct {
	ID         int64
	Rank       *int                       `orm:"unique;null"`
	UniqueRank int                        `orm:"unique"`
	Headline   string                     `orm:"max_length=100"`
	Slug       string                     `orm:"max_length=100;default=slug"`
	PubDate    *orm.Date                  `orm:"type=date;null"`
	Barcode    string                     `orm:"max_length=30;default=bar"`
	Parent     orm.FK[ordBarcodedArticle] `orm:"null;on_delete=set_null"`
}

// Meta declares the unique_together set of BarcodedArticle.
func (ordBarcodedArticle) Meta() orm.Meta {
	return orm.Meta{UniqueTogether: [][]string{{"Headline", "Slug"}}}
}

// ordTotallyOrderedModels returns the models of Django's TotallyOrderedTests.
func ordTotallyOrderedModels() []any {
	return []any{djangotest.App("ordering", &ordAuthor{}, &ordBarcodedArticle{})}
}

// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_basic_ordering
func TestTotallyOrderedBasicOrdering(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).TotallyOrdered(), true)
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy("name").TotallyOrdered(), false)
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).OrderBy("rank").TotallyOrdered(), false)
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).OrderBy("rank", "pk").TotallyOrdered(), true)
	})
}

// Django's first case relies on a UniqueConstraint over nullable fields,
// which the ORM cannot declare; it is false here as well.
//
// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_composite_constraints
func TestTotallyOrderedCompositeConstraints(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).OrderBy("pub_date", "rank").TotallyOrdered(), false)
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).OrderBy("headline", "slug").TotallyOrdered(), true)
	})
}

// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_reverse_ordering
func TestTotallyOrderedReverseOrdering(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy("-pk").TotallyOrdered(), true)
	})
}

// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_f_expressions
func TestTotallyOrderedFExpressions(t *testing.T) {
	t.Skip("FIXME-PORT: Go: OrderBy takes keys, not F() expressions; " +
		"Django: tests/ordering/tests.py::TotallyOrderedTests.test_f_expressions")
}

// Ordering returns the keys of OrderBy, or Meta.Ordering without them. No
// Django test exists; Django reads qs.query.order_by and Meta.ordering.
func TestQuerySetOrdering(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).Ordering(), []string{"-pk"})
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy("name", "-id").Ordering(), []string{"name", "-id"})
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy().Ordering(), []string{})
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).Ordering(), []string{})
	})
}
