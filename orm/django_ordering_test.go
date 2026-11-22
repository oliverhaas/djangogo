package orm_test

import (
	"context"
	"fmt"
	"slices"
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
	SecondAuthor orm.FK[ordAuthor] `orm:"null;on_delete=set_null;related_name=+"`
	Headline     string            `orm:"max_length=100"`
	PubDate      time.Time
}

// Meta sets Article's default ordering matching Django's ordering.Article.
func (ordArticle) Meta() orm.Meta {
	return orm.Meta{
		OrderingExprs: []any{
			"-pub_date",
			orm.F("headline"),
			orm.F("author__name").Asc(),
			orm.F("second_author__name").Asc(),
		},
	}
}

// ordByAuthorArticle mirrors Django's OrderedByAuthorArticle, ordering by
// two FKs to ordAuthor (which itself orders by -pk).
type ordByAuthorArticle struct {
	ID           int64
	Author       orm.FK[ordAuthor] `orm:"null;on_delete=set_null"`
	SecondAuthor orm.FK[ordAuthor] `orm:"null;on_delete=set_null;related_name=+"`
	Headline     string            `orm:"max_length=100"`
	PubDate      time.Time
}

// Meta sets default ordering to author, second_author.
func (ordByAuthorArticle) Meta() orm.Meta {
	return orm.Meta{Ordering: []string{"author", "second_author"}}
}

// ordReference is Django's ordering.Reference as of Django main, where Proof
// is a one-to-one relation to Article. OrderedByAuthorArticle is a proxy of
// Article in Django, so Proof points to ordByAuthorArticle here.
type ordReference struct {
	ID      int64
	Article orm.FK[ordByAuthorArticle]  `orm:"on_delete=cascade"`
	Proof   orm.O2O[ordByAuthorArticle] `orm:"on_delete=cascade;related_name=+"`
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

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_reverse_ordering_pure
func TestOrderingReverseOrderingPure(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		qs1 := orm.Query[ordArticle](db).OrderBy(orm.F("headline").Asc())
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
		qs := orm.Query[ordArticle](db).OrderBy(orm.F("headline").Asc(), orm.F("headline").Desc())
		djangotest.AssertEqual(t, count(qs), 1)
		ordAssert(t, qs, "Article 1", "Article 2", "Article 3", "Article 4")
		qs = orm.Query[ordArticle](db).OrderBy(orm.F("headline").Desc(), orm.F("headline").Asc())
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
		r1 := create(t, db, &ordReference{Article: fkTo[ordByAuthorArticle](a1.ID), Proof: o2oTo[ordByAuthorArticle](a1.ID)})
		r2 := create(t, db, &ordReference{Article: fkTo[ordByAuthorArticle](a2.ID), Proof: o2oTo[ordByAuthorArticle](a2.ID)})
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

// ordBarcodedArticle is Django's ordering.BarcodedArticle.
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

// Meta declares the unique_together set and the unique constraints of
// BarcodedArticle.
func (ordBarcodedArticle) Meta() orm.Meta {
	return orm.Meta{
		UniqueTogether: [][]string{{"Headline", "Slug"}},
		Constraints: []orm.Constraint{
			orm.UniqueConstraint{Fields: []string{"pub_date", "rank"}, Name: "unique_pub_date_rank"},
			orm.UniqueConstraint{Fields: []string{"rank"}, Condition: orm.Q("rank__gt", 0), Name: "unique_rank_conditional"},
			orm.UniqueConstraint{Fields: []string{"barcode"}, Condition: orm.Q(), Name: "unique_barcode_empty_condition"},
		},
	}
}

// ordTotallyOrderedModels returns the models of Django's TotallyOrderedTests.
func ordTotallyOrderedModels() []any {
	return []any{djangotest.App("ordering",
		&ordAuthor{}, &ordArticle{}, &ordByAuthorArticle{}, &ordReference{}, &ordBarcodedArticle{})}
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
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy(orm.F("pk")).TotallyOrdered(), true)
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy(orm.F("name")).TotallyOrdered(), false)
	})
}

// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_one_to_one_relation
func TestTotallyOrderedOneToOneRelation(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordReference](db).OrderBy("proof").TotallyOrdered(), false)
		djangotest.AssertEqual(t, orm.Query[ordReference](db).OrderBy("proof_id").TotallyOrdered(), true)
	})
}

// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_relation_traversal
func TestTotallyOrderedRelationTraversal(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordArticle](db).OrderBy("author__pk").TotallyOrdered(), false)
	})
}

// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_conditional_constraints
func TestTotallyOrderedConditionalConstraints(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).OrderBy("rank").TotallyOrdered(), false)
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).OrderBy("barcode").TotallyOrdered(), true)
	})
}

// The ORM has no alias(), so Annotate stands in for it: Django's
// totally_ordered reads annotations and aliases the same way.
//
// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_alias
func TestTotallyOrderedAlias(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		authors := orm.Query[ordAuthor](db)
		djangotest.AssertEqual(t, authors.Annotate("my_pk", orm.F("pk")).OrderBy("my_pk").TotallyOrdered(), true)
		djangotest.AssertEqual(t, authors.Annotate("my_pk", orm.F("name")).OrderBy("my_pk").TotallyOrdered(), false)
	})
}

// Annotate stands in for alias(); see TestTotallyOrderedAlias.
//
// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_alias_relation
func TestTotallyOrderedAliasRelation(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[ordAuthor](db).Annotate("my_editor", orm.F("editor__pk")).OrderBy("my_editor")
		djangotest.AssertEqual(t, qs.TotallyOrdered(), false)
	})
}

// Annotate stands in for alias(); see TestTotallyOrderedAlias.
//
// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_alias_self_relation
func TestTotallyOrderedAliasSelfRelation(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		articles := orm.Query[ordBarcodedArticle](db)
		qs := articles.Annotate("my_unique_rank", orm.F("unique_rank")).OrderBy("my_unique_rank")
		djangotest.AssertEqual(t, qs.TotallyOrdered(), true)
		qs = articles.Annotate("parent_unique_rank", orm.F("parent__unique_rank")).OrderBy("parent_unique_rank")
		djangotest.AssertEqual(t, qs.TotallyOrdered(), false)
	})
}

// Ported from Django: tests/ordering/tests.py::TotallyOrderedTests.test_totally_ordered_none
func TestTotallyOrderedNone(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy().None().TotallyOrdered(), false)
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy("pk").None().TotallyOrdered(), true)
	})
}

// Ordering returns the terms of OrderBy, or Meta.Ordering without them, with
// an expression as an OrderExpr. No Django test exists; Django reads
// qs.query.order_by and Meta.ordering.
func TestQuerySetOrdering(t *testing.T) {
	djangotest.EachDB(t, ordTotallyOrderedModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).Ordering(), []any{"-pk"})
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy("name", "-id").Ordering(), []any{"name", "-id"})
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy().Ordering(), []any{})
		djangotest.AssertEqual(t, orm.Query[ordBarcodedArticle](db).Ordering(), []any{})
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy(orm.F("name").Desc().NullsLast(), orm.Lower("name")).Ordering(),
			[]any{orm.F("name").Desc().NullsLast(), orm.Lower("name").Asc()})
		djangotest.AssertEqual(t, orm.Query[ordAuthor](db).OrderBy(orm.F("name").Desc()).Reverse().Ordering(),
			[]any{orm.F("name").Asc()})
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_nulls_first_and_last
func TestOrderingOrderByNullsFirstAndLast(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[ordArticle](db).OrderBy(orm.F("author").Desc().NullsLast().NullsFirst()).All(context.Background())
		assertErrorContains(t, err, "NullsFirst and NullsLast are mutually exclusive")
	})
}

// ordAssertReversible checks the articles of qs, and of its Reverse in the
// opposite order: Django's assertQuerySetEqualReversible.
func ordAssertReversible(t *testing.T, qs *orm.QuerySet[ordArticle], want ...*ordArticle) {
	t.Helper()
	id := func(a ordArticle) int64 { return a.ID }
	ids := make([]int64, len(want))
	for i, a := range want {
		ids[i] = a.ID
	}
	djangotest.AssertQuerySetEqual(t, qs, ids, id)
	slices.Reverse(ids)
	djangotest.AssertQuerySetEqual(t, qs.Reverse(), ids, id)
}

// ordSetUpNulls gives Article 3 the first author and Article 4 the second
// one, as the nulls tests of Django's OrderingTests do.
func ordSetUpNulls(t *testing.T, db *orm.DB) ordFixture {
	t.Helper()
	fx := ordSetUp(t, db)
	ordSetAuthor(t, db, fx.a3, fx.author1)
	ordSetAuthor(t, db, fx.a4, fx.author2)
	return fx
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_nulls_last
func TestOrderingOrderByNullsLast(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUpNulls(t, db)
		articles := func() *orm.QuerySet[ordArticle] { return orm.Query[ordArticle](db) }
		ordAssertReversible(t, articles().OrderBy(orm.F("author").Desc().NullsLast(), "headline"),
			fx.a4, fx.a3, fx.a1, fx.a2)
		ordAssertReversible(t, articles().OrderBy(orm.F("author").Asc().NullsLast(), "headline"),
			fx.a3, fx.a4, fx.a1, fx.a2)
		ordAssertReversible(t, articles().OrderBy(orm.Upper("author__name").Desc().NullsLast(), "headline"),
			fx.a4, fx.a3, fx.a1, fx.a2)
		ordAssertReversible(t, articles().OrderBy(orm.Upper("author__name").Asc().NullsLast(), "headline"),
			fx.a3, fx.a4, fx.a1, fx.a2)
		ordAssertReversible(t, articles().Annotate("upper_name", orm.Upper("author__name")).
			OrderBy(orm.F("upper_name").Asc().NullsLast(), "headline"),
			fx.a3, fx.a4, fx.a1, fx.a2)
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_nulls_first
func TestOrderingOrderByNullsFirst(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUpNulls(t, db)
		articles := func() *orm.QuerySet[ordArticle] { return orm.Query[ordArticle](db) }
		ordAssertReversible(t, articles().OrderBy(orm.F("author").Asc().NullsFirst(), "headline"),
			fx.a1, fx.a2, fx.a3, fx.a4)
		ordAssertReversible(t, articles().OrderBy(orm.F("author").Desc().NullsFirst(), "headline"),
			fx.a1, fx.a2, fx.a4, fx.a3)
		ordAssertReversible(t, articles().OrderBy(orm.Upper("author__name").Asc().NullsFirst(), "headline"),
			fx.a1, fx.a2, fx.a3, fx.a4)
		ordAssertReversible(t, articles().OrderBy(orm.Upper("author__name").Desc().NullsFirst(), "headline"),
			fx.a1, fx.a2, fx.a4, fx.a3)
		ordAssertReversible(t, articles().Annotate("upper_name", orm.Upper("author__name")).
			OrderBy(orm.F("upper_name").Desc().NullsFirst(), "headline"),
			fx.a1, fx.a2, fx.a4, fx.a3)
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_orders_nulls_first_on_filtered_subquery
func TestOrderingOrdersNullsFirstOnFilteredSubquery(t *testing.T) {
	t.Skip("needs Subquery and OuterRef, which the ORM does not have")
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_no_reordering_after_slicing
func TestOrderingNoReorderingAfterSlicing(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		qs := orm.Query[ordArticle](db).Limit(2)
		_, err := qs.Reverse().All(ctx)
		assertErrorContains(t, err, "annot reverse a query once a slice has been taken")
		_, err = qs.Last(ctx)
		assertErrorContains(t, err, "annot reverse a query once a slice has been taken")
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_extra_ordering
func TestOrderingExtraOrdering(t *testing.T) {
	t.Skip("needs extra(), which the ORM does not have")
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_extra_ordering_quoting
func TestOrderingExtraOrderingQuoting(t *testing.T) {
	t.Skip("needs extra(), which the ORM does not have")
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_extra_ordering_with_table_name
func TestOrderingExtraOrderingWithTableName(t *testing.T) {
	t.Skip("needs extra(), which the ORM does not have")
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_f_expression
func TestOrderingOrderByFExpression(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		ordAssert(t, orm.Query[ordArticle](db).OrderBy(orm.F("headline")),
			"Article 1", "Article 2", "Article 3", "Article 4")
		ordAssert(t, orm.Query[ordArticle](db).OrderBy(orm.F("headline").Asc()),
			"Article 1", "Article 2", "Article 3", "Article 4")
		ordAssert(t, orm.Query[ordArticle](db).OrderBy(orm.F("headline").Desc()),
			"Article 4", "Article 3", "Article 2", "Article 1")
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_constant_value
func TestOrderingOrderByConstantValue(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		id := func(a ordArticle) int64 { return a.ID }
		want := []int64{fx.a4.ID, fx.a3.ID, fx.a2.ID, fx.a1.ID}
		// Order by annotated constant from selected columns.
		qs := orm.Query[ordArticle](db).Annotate("constant", orm.Value("1")).OrderBy("constant", "-headline")
		djangotest.AssertQuerySetEqual(t, qs, want, id)
		// Order by annotated constant which is out of selected columns.
		rows, err := qs.ValuesList("headline").Flat(context.Background())
		if err != nil {
			t.Fatalf("ValuesList: %v", err)
		}
		djangotest.AssertEqual(t, rows, []any{"Article 4", "Article 3", "Article 2", "Article 1"})
		// Order by constant.
		djangotest.AssertQuerySetEqual(t, orm.Query[ordArticle](db).OrderBy(orm.Value("1"), "-headline"), want, id)
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_case_when_constant_value
func TestOrderingOrderByCaseWhenConstantValue(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		qs := orm.Query[ordArticle](db).OrderBy(
			orm.Case(orm.When{Cond: orm.Q("pk__in", []any{}), Then: orm.Value(1)}).Default(orm.Value(0)).Desc(),
			"pk",
		)
		djangotest.AssertQuerySetEqual(t, qs, []int64{fx.a1.ID, fx.a2.ID, fx.a3.ID, fx.a4.ID},
			func(a ordArticle) int64 { return a.ID })
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_default_ordering_by_f_expression
func TestOrderingDefaultOrderingByFExpression(t *testing.T) {
	models := []any{djangotest.App("ordering", &ordAuthor{}, &ordByFArticle{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		for i, date := range []time.Time{day(2005, 7, 26), day(2005, 7, 27), day(2005, 7, 27), day(2005, 7, 28)} {
			create(t, db, &ordByFArticle{Headline: fmt.Sprintf("Article %d", i+1), PubDate: date})
		}
		author1 := create(t, db, &ordAuthor{Name: "Name 1"})
		author2 := create(t, db, &ordAuthor{Name: "Name 2"})
		articles := orm.Query[ordByFArticle](db)
		if _, err := articles.Filter("headline", "Article 2").Update(ctx, "author", author2); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if _, err := articles.Filter("headline", "Article 3").Update(ctx, "author", author1); err != nil {
			t.Fatalf("Update: %v", err)
		}
		djangotest.AssertQuerySetEqual(t, articles,
			[]string{"Article 1", "Article 4", "Article 3", "Article 2"},
			func(a ordByFArticle) string { return a.Headline })
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_ptr_field_with_default_ordering_by_expression
func TestOrderingOrderByPtrFieldWithDefaultOrderingByExpression(t *testing.T) {
	t.Skip("needs multi-table inheritance, which the ORM does not have")
}

// ordByExpression is Django's ordering.OrderedByExpression.
type ordByExpression struct {
	ID   int64
	Name string `orm:"max_length=30"`
}

// Meta orders by the lowercased name.
func (ordByExpression) Meta() orm.Meta {
	return orm.Meta{OrderingExprs: []any{orm.Lower("name")}}
}

// ordByExpressionChild is Django's ordering.OrderedByExpressionChild.
type ordByExpressionChild struct {
	ID     int64
	Parent orm.FK[ordByExpression] `orm:"on_delete=cascade"`
}

// Meta orders by the parent, so by the parent's ordering.
func (ordByExpressionChild) Meta() orm.Meta { return orm.Meta{Ordering: []string{"parent"}} }

// ordByExpressionGrandChild is Django's
// ordering.OrderedByExpressionGrandChild.
type ordByExpressionGrandChild struct {
	ID     int64
	Parent orm.FK[ordByExpressionChild] `orm:"on_delete=cascade"`
}

// ordExpressionModels returns the models of the tests that order by a parent
// with an expression in its ordering.
func ordExpressionModels() []any {
	return []any{djangotest.App("ordering", &ordByExpression{}, &ordByExpressionChild{}, &ordByExpressionGrandChild{})}
}

// ordExpressionParents creates the parents and children of the expression
// ordering tests, c1 to c3 in the order their parents sort in.
func ordExpressionParents(t *testing.T, db *orm.DB) (c1, c2, c3 *ordByExpressionChild) {
	t.Helper()
	p3 := create(t, db, &ordByExpression{Name: "oBJ 3"})
	p2 := create(t, db, &ordByExpression{Name: "OBJ 2"})
	p1 := create(t, db, &ordByExpression{Name: "obj 1"})
	c3 = create(t, db, &ordByExpressionChild{Parent: fkTo[ordByExpression](p3.ID)})
	c2 = create(t, db, &ordByExpressionChild{Parent: fkTo[ordByExpression](p2.ID)})
	c1 = create(t, db, &ordByExpressionChild{Parent: fkTo[ordByExpression](p1.ID)})
	return c1, c2, c3
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_parent_fk_with_expression_in_default_ordering
func TestOrderingOrderByParentFKWithExpressionInDefaultOrdering(t *testing.T) {
	djangotest.EachDB(t, ordExpressionModels(), func(t *testing.T, db *orm.DB) {
		c1, c2, c3 := ordExpressionParents(t, db)
		djangotest.AssertQuerySetEqual(t, orm.Query[ordByExpressionChild](db).OrderBy("parent"),
			[]int64{c1.ID, c2.ID, c3.ID}, func(c ordByExpressionChild) int64 { return c.ID })
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_grandparent_fk_with_expression_in_default_ordering
func TestOrderingOrderByGrandparentFKWithExpressionInDefaultOrdering(t *testing.T) {
	djangotest.EachDB(t, ordExpressionModels(), func(t *testing.T, db *orm.DB) {
		c1, c2, c3 := ordExpressionParents(t, db)
		g3 := create(t, db, &ordByExpressionGrandChild{Parent: fkTo[ordByExpressionChild](c3.ID)})
		g2 := create(t, db, &ordByExpressionGrandChild{Parent: fkTo[ordByExpressionChild](c2.ID)})
		g1 := create(t, db, &ordByExpressionGrandChild{Parent: fkTo[ordByExpressionChild](c1.ID)})
		djangotest.AssertQuerySetEqual(t, orm.Query[ordByExpressionGrandChild](db).OrderBy("parent"),
			[]int64{g1.ID, g2.ID, g3.ID}, func(g ordByExpressionGrandChild) int64 { return g.ID })
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_expression_ref
func TestOrderingOrderByExpressionRef(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		id := func(a ordAuthor) int64 { return a.ID }
		want := mustAll(t, orm.Query[ordAuthor](db).OrderBy(orm.Length(orm.Upper("name"))))
		ids := make([]int64, len(want))
		for i, a := range want {
			ids[i] = a.ID
		}
		djangotest.AssertQuerySetEqual(t,
			orm.Query[ordAuthor](db).Annotate("upper_name", orm.Upper("name")).OrderBy(orm.Length("upper_name")),
			ids, id)
	})
}

// Ported from Django: tests/ordering/tests.py::OrderingTests.test_ordering_select_related_collision
func TestOrderingOrderingSelectRelatedCollision(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		fx := ordSetUp(t, db)
		ctx := context.Background()
		qs := orm.Query[ordArticle](db).SelectRelated("author").
			Annotate("name", orm.Upper("author__name")).Filter("pk", fx.a1.ID)
		got, err := qs.OrderBy(orm.F("name").Asc()).First(ctx)
		if err != nil {
			t.Fatalf("First: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, fx.a1.ID)
		got, err = qs.OrderBy("name").First(ctx)
		if err != nil {
			t.Fatalf("First: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, fx.a1.ID)
	})
}

// Django compares the queryset with its iterator(); the Go QuerySet has no
// iterator, so the test runs the queryset twice.
//
// Ported from Django: tests/ordering/tests.py::OrderingTests.test_order_by_expr_query_reuse
func TestOrderingOrderByExprQueryReuse(t *testing.T) {
	djangotest.EachDB(t, ordModels(), func(t *testing.T, db *orm.DB) {
		ordSetUp(t, db)
		qs := orm.Query[ordAuthor](db).Annotate("num", orm.Count("ordarticle")).OrderBy(orm.F("num").Desc(), "pk")
		first := mustAll(t, qs)
		djangotest.AssertEqual(t, mustAll(t, qs), first)
		djangotest.AssertEqual(t, len(first), 4)
	})
}

// ordByFArticle is Django's OrderedByFArticle. Django's is a proxy of
// Article; the ORM has no proxy models, so it has a table of its own.
type ordByFArticle struct {
	ID       int64
	Author   orm.FK[ordAuthor] `orm:"null;on_delete=set_null;related_name=+"`
	Headline string            `orm:"max_length=100"`
	PubDate  time.Time
}

// Meta orders by the author with the articles without one first, then by id.
func (ordByFArticle) Meta() orm.Meta {
	return orm.Meta{OrderingExprs: []any{orm.F("author").Asc().NullsFirst(), "id"}}
}
