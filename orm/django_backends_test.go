package orm_test

import (
	"context"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// bkReporter is Django's backends.Reporter.
type bkReporter struct {
	ID        int64
	FirstName string `orm:"max_length=30"`
	LastName  string `orm:"max_length=30"`
}

// bkArticle is Django's backends.Article without its proxy relation.
type bkArticle struct {
	ID       int64
	Headline string             `orm:"max_length=100"`
	PubDate  time.Time          `orm:"type=date"`
	Reporter orm.FK[bkReporter] `orm:"on_delete=cascade"`
}

// bkSQLKeywordsModel is Django's backends.SQLKeywordsModel.
type bkSQLKeywordsModel struct {
	ID       int64              `orm:"column=select"`
	Reporter orm.FK[bkReporter] `orm:"on_delete=cascade;column=where"`
}

// Meta sets the SQL keyword table name of Django's backends.SQLKeywordsModel.
func (bkSQLKeywordsModel) Meta() orm.Meta { return orm.Meta{Table: "order"} }

// bkPost is Django's backends.Post.
type bkPost struct {
	ID   int64
	Name string `orm:"max_length=30"`
	Text string `orm:"type=text"`
}

// Meta sets the mixed-case table name of Django's backends.Post.
func (bkPost) Meta() orm.Meta { return orm.Meta{Table: "CaseSensitive_Post"} }

// bkAuthor is Django's backends.Author.
type bkAuthor struct {
	ID   int64
	Name string `orm:"max_length=255;unique"`
}

// bkBook is Django's backends.Book with a primary-key foreign key to bkAuthor.
type bkBook struct {
	ID     int64
	Author orm.FK[bkAuthor] `orm:"on_delete=cascade"`
}

// bkModels returns the models of Django's backends test app.
func bkModels() []any {
	return []any{djangotest.App("backends",
		&bkReporter{}, &bkArticle{}, &bkSQLKeywordsModel{}, &bkPost{}, &bkAuthor{}, &bkBook{},
	)}
}

// Ported from Django: tests/backends/tests.py::FkConstraintsTests.test_check_constraints
// Check that CheckForeignKeys reports a row whose foreign key refers to a missing row inside Atomic.
func TestBackendsCheckConstraints(t *testing.T) {
	djangotest.EachDB(t, bkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		r := create(t, db, &bkReporter{FirstName: "John", LastName: "Smith"})

		err := db.Atomic(ctx, func(txCtx context.Context) error {
			err := orm.Query[bkArticle](db).Create(txCtx, &bkArticle{
				Headline: "Test article",
				PubDate:  day(2010, 9, 4),
				Reporter: fkTo[bkReporter](r.ID),
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			a, err := orm.Query[bkArticle](db).Get(txCtx, "headline", "Test article")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			a.Reporter = fkTo[bkReporter](30)
			if err := orm.Query[bkArticle](db).Save(txCtx, &a); err != nil {
				t.Fatalf("Save inside Atomic: %v", err)
			}
			checkErr := orm.CheckForeignKeys(txCtx, db, "backends_bkarticle")
			djangotest.AssertErrorIs(t, checkErr, orm.ErrIntegrity)
			djangotest.AssertErrorIs(t, checkErr, orm.ErrForeignKeyViolation)
			if db.Dialect().Name() == "sqlite" && checkErr != nil {
				want := "The row in table 'backends_bkarticle' with primary key '1' has an invalid foreign key: " +
					"backends_bkarticle.reporter_id contains a value '30' that does not have a corresponding value in backends_bkreporter.id."
				djangotest.AssertEqual(t, checkErr.Error(), want)
			}
			return db.SetRollback(txCtx, true)
		})
		if err != nil {
			t.Fatalf("Atomic: %v", err)
		}
	})
}

// Ported from Django: tests/backends/tests.py::FkConstraintsTests.test_check_constraints_sql_keywords
// Check that CheckForeignKeys quotes table and column names that are SQL keywords.
func TestBackendsCheckConstraintsSQLKeywords(t *testing.T) {
	djangotest.EachDB(t, bkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		r := create(t, db, &bkReporter{FirstName: "John", LastName: "Smith"})

		err := db.Atomic(ctx, func(txCtx context.Context) error {
			obj := &bkSQLKeywordsModel{Reporter: fkTo[bkReporter](r.ID)}
			if err := orm.Query[bkSQLKeywordsModel](db).Create(txCtx, obj); err != nil {
				t.Fatalf("Create: %v", err)
			}
			loaded, err := orm.Query[bkSQLKeywordsModel](db).Get(txCtx, "pk", obj.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			loaded.Reporter = fkTo[bkReporter](30)
			if err := orm.Query[bkSQLKeywordsModel](db).Save(txCtx, &loaded); err != nil {
				t.Fatalf("Save inside Atomic: %v", err)
			}
			checkErr := orm.CheckForeignKeys(txCtx, db, "order")
			djangotest.AssertErrorIs(t, checkErr, orm.ErrIntegrity)
			djangotest.AssertErrorIs(t, checkErr, orm.ErrForeignKeyViolation)
			if db.Dialect().Name() == "sqlite" && checkErr != nil {
				want := "The row in table 'order' with primary key '1' has an invalid foreign key: " +
					"order.where contains a value '30' that does not have a corresponding value in backends_bkreporter.id."
				djangotest.AssertEqual(t, checkErr.Error(), want)
			}
			return db.SetRollback(txCtx, true)
		})
		if err != nil {
			t.Fatalf("Atomic: %v", err)
		}
	})
}

// Ported from Django: tests/backends/tests.py::SequenceResetTest.test_generic_relation
// Check that ResetSequences advances the primary-key sequence past an explicit key on a mixed-case table.
func TestBackendsSequenceResetGenericRelation(t *testing.T) {
	djangotest.EachDB(t, bkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &bkPost{ID: 10, Name: "1st post", Text: "hello world"})

		postModel, ok := db.Registry().ModelOf(bkPost{})
		if !ok {
			t.Fatal("bkPost not registered")
		}
		if err := orm.ResetSequences(ctx, db, postModel); err != nil {
			t.Fatalf("ResetSequences: %v", err)
		}

		obj := create(t, db, &bkPost{Name: "New post", Text: "goodbye world"})
		djangotest.AssertEqual(t, obj.ID, int64(11))
	})
}

// Ported from Django: tests/backends/base/test_operations.py::SqlFlushTests.test_sql_flush_no_tables
// Check that Flush with no tables does nothing and leaves existing rows intact.
func TestBackendsSQLFlushNoTables(t *testing.T) {
	djangotest.EachDB(t, bkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &bkAuthor{Name: "George Orwell"})
		if err := orm.Flush(ctx, db); err != nil {
			t.Fatalf("Flush(): %v", err)
		}
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bkAuthor](db)), int64(1))
	})
}

// Ported from Django: tests/backends/base/test_operations.py::SqlFlushTests.test_execute_sql_flush_statements
// Check that Flush empties the named tables and restarts their auto primary keys at 1.
func TestBackendsExecuteSQLFlushStatements(t *testing.T) {
	djangotest.EachDB(t, bkModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		err := db.Atomic(ctx, func(txCtx context.Context) error {
			orwell := &bkAuthor{Name: "George Orwell"}
			if err := orm.Query[bkAuthor](db).Create(txCtx, orwell); err != nil {
				return err
			}
			if err := orm.Query[bkBook](db).Create(txCtx, &bkBook{Author: fkTo[bkAuthor](orwell.ID)}); err != nil {
				return err
			}
			lee := &bkAuthor{Name: "Harper Lee"}
			if err := orm.Query[bkAuthor](db).Create(txCtx, lee); err != nil {
				return err
			}
			if err := orm.Query[bkBook](db).Create(txCtx, &bkBook{Author: fkTo[bkAuthor](lee.ID)}); err != nil {
				return err
			}
			if err := orm.Query[bkBook](db).Create(txCtx, &bkBook{Author: fkTo[bkAuthor](lee.ID)}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("seed Atomic: %v", err)
		}
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bkAuthor](db)), int64(2))
		djangotest.AssertEqual(t, mustCount(t, orm.Query[bkBook](db)), int64(3))

		err = db.Atomic(ctx, func(txCtx context.Context) error {
			return orm.Flush(txCtx, db, "backends_bkauthor", "backends_bkbook")
		})
		if err != nil {
			t.Fatalf("Flush: %v", err)
		}

		err = db.Atomic(ctx, func(txCtx context.Context) error {
			nAuthors, err := orm.Query[bkAuthor](db).Count(txCtx)
			if err != nil {
				return err
			}
			djangotest.AssertEqual(t, nAuthors, int64(0))
			nBooks, err := orm.Query[bkBook](db).Count(txCtx)
			if err != nil {
				return err
			}
			djangotest.AssertEqual(t, nBooks, int64(0))

			author := &bkAuthor{Name: "F. Scott Fitzgerald"}
			if err := orm.Query[bkAuthor](db).Create(txCtx, author); err != nil {
				return err
			}
			djangotest.AssertEqual(t, author.ID, int64(1))
			book := &bkBook{Author: fkTo[bkAuthor](author.ID)}
			if err := orm.Query[bkBook](db).Create(txCtx, book); err != nil {
				return err
			}
			djangotest.AssertEqual(t, book.ID, int64(1))
			return nil
		})
		if err != nil {
			t.Fatalf("verify Atomic: %v", err)
		}
	})
}
