package orm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// idxArticle mirrors Django's indexes.Article with an unnamed composite index.
type idxArticle struct {
	ID        int64
	Headline  string `orm:"max_length=100"`
	PubDate   time.Time
	Published bool `orm:"default=false"`
}

// Meta declares the composite index from Django's indexes.Article.
func (idxArticle) Meta() orm.Meta {
	return orm.Meta{
		Table:   "indexes_article",
		Indexes: []orm.Index{{Fields: []string{"headline", "pub_date"}}},
	}
}

// idxDescArticle declares an explicit descending index as in
// SchemaIndexesTests.test_descending_columns_list_sql.
type idxDescArticle struct {
	ID       int64
	Headline string `orm:"max_length=100"`
}

// Meta declares a named descending index.
func (idxDescArticle) Meta() orm.Meta {
	return orm.Meta{
		Table:   "indexes_desc_article",
		Indexes: []orm.Index{{Name: "whitespace_idx", Fields: []string{"-headline"}}},
	}
}

// idxUniqueItem declares Meta.UniqueTogether for database enforcement checks.
type idxUniqueItem struct {
	ID   int64
	Code string `orm:"max_length=20"`
	Kind string `orm:"max_length=20"`
}

// Meta declares a unique_together constraint over Code and Kind.
func (idxUniqueItem) Meta() orm.Meta {
	return orm.Meta{
		UniqueTogether: [][]string{{"code", "kind"}},
	}
}

// Ported from Django: tests/indexes/tests.py::SchemaIndexesTests.test_index_name_hash
func TestIndexesNameHash(t *testing.T) {
	got := orm.CreateIndexName("indexes_article", []string{"c1"}, "123", 200)
	djangotest.AssertEqual(t, got, "indexes_article_c1_a52bd80b123")
}

// Ported from Django: tests/indexes/tests.py::SchemaIndexesTests.test_index_name
func TestIndexesNameTruncation(t *testing.T) {
	longName := "l" + strings.Repeat("o", 100) + "ng"
	pgGot := orm.CreateIndexName("indexes_article", []string{"c1", "c2", longName}, "ix", 63)
	djangotest.AssertEqual(t, pgGot, "indexes_article_c1_c2_loooooooooooooooooo_255179b2ix")

	sqliteGot := orm.CreateIndexName("indexes_article", []string{"c1", "c2", longName}, "ix", 200)
	djangotest.AssertEqual(t, sqliteGot, "indexes_article_c1_c2_"+longName+"_255179b2ix")
}

// Ported from Django: tests/indexes/tests.py::SchemaIndexesTests.test_quoted_index_name
func TestIndexesQuotedIndexName(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("indexes", &idxArticle{})}, func(t *testing.T, db *orm.DB) {
		m, _ := db.Registry().ModelOf(&idxArticle{})
		indexes := m.Indexes()
		djangotest.AssertEqual(t, len(indexes), 1)
		// Matches Django's Index.set_name_with_model for indexes_article(headline, pub_date).
		djangotest.AssertEqual(t, indexes[0].Name, "indexes_art_headlin_64bb9e_idx")
		djangotest.AssertIndexExists(t, db, "indexes_article", "headline", "pub_date")
		djangotest.AssertIndexNameExists(t, db, "indexes_article", "indexes_art_headlin_64bb9e_idx")
	})
}

// Ported from Django: tests/indexes/tests.py::SchemaIndexesTests.test_columns_list_sql
func TestIndexesColumnsList(t *testing.T) {
	type singleCol struct {
		ID       int64
		Headline string `orm:"max_length=100"`
	}
	// Verify IndexName matches Django for descending vs ascending columns.
	djangotest.AssertEqual(t, orm.IndexName("indexes_article", []string{"headline", "pub_date"}), "indexes_art_headlin_64bb9e_idx")
	djangotest.AssertEqual(t, orm.IndexName("indexes_article", []string{"-headline"}), "indexes_art_headlin_c9ade5_idx")

	djangotest.EachDB(t, []any{djangotest.App("indexes", &singleCol{}, &idxArticle{})}, func(t *testing.T, db *orm.DB) {
		djangotest.AssertIndexExists(t, db, "indexes_article", "headline", "pub_date")
	})
}

// Ported from Django: tests/indexes/tests.py::SchemaIndexesTests.test_descending_columns_list_sql
func TestIndexesDescendingColumns(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("indexes", &idxDescArticle{})}, func(t *testing.T, db *orm.DB) {
		djangotest.AssertIndexExists(t, db, "indexes_desc_article", "headline")
		djangotest.AssertIndexNameExists(t, db, "indexes_desc_article", "whitespace_idx")
	})
}

// idxArticleTranslation mirrors Django's indexes.ArticleTranslation to verify
// automatic FK column indexing with Django's naming.
type idxArticleTranslation struct {
	ID       int64
	Article  orm.FK[idxArticle] `orm:"on_delete=cascade"`
	UniqueFK orm.FK[idxArticle] `orm:"null;unique;on_delete=set_null"`
	Language string             `orm:"max_length=10;unique"`
	Content  string             `orm:"type=text"`
}

// Meta pins the table name to Django's indexes_articletranslation.
func (idxArticleTranslation) Meta() orm.Meta {
	return orm.Meta{Table: "indexes_articletranslation"}
}

// TestForeignKeyAutoIndex checks that CreateTable creates a Django-named index
// on every non-unique FK column and skips unique FKs.
func TestForeignKeyAutoIndex(t *testing.T) {
	models := []any{djangotest.App("indexes", &idxArticle{}, &idxArticleTranslation{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		// Matches Django: indexes_articletranslation_article_id_8d800880.
		wantName := "indexes_articletranslation_article_id_8d800880"
		djangotest.AssertIndexExists(t, db, "indexes_articletranslation", "article_id")
		djangotest.AssertIndexNameExists(t, db, "indexes_articletranslation", wantName)

		// A unique FK already has a unique index; no extra non-unique index is created.
		uniqueIdxName := orm.CreateIndexName("indexes_articletranslation", []string{"unique_fk_id"}, "", 200)
		if db.Dialect().Name() == "postgres" {
			uniqueIdxName = orm.CreateIndexName("indexes_articletranslation", []string{"unique_fk_id"}, "", 63)
		}
		djangotest.AssertIndexNameNotExists(t, db, "indexes_articletranslation", uniqueIdxName)
	})
}

// TestUniqueTogetherEnforcedByCreateTable checks that Meta.UniqueTogether
// creates a unique constraint that rejects duplicate pairs on both dialects.
func TestUniqueTogetherEnforcedByCreateTable(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("indexes", &idxUniqueItem{})}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		djangotest.AssertUniqueConstraintExists(t, db, "indexes_idxuniqueitem", "code", "kind")
		create(t, db, &idxUniqueItem{Code: "a", Kind: "x"})
		create(t, db, &idxUniqueItem{Code: "a", Kind: "y"})
		err := orm.Query[idxUniqueItem](db).Create(ctx, &idxUniqueItem{Code: "a", Kind: "x"})
		djangotest.AssertErrorIs(t, err, orm.ErrUniqueViolation)
	})
}
