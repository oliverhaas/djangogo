package orm_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// rawAuthor is Django's raw_query.Author. dob is a DateField in Django; here
// it is a time at midnight UTC.
type rawAuthor struct {
	ID        int64
	FirstName string `orm:"max_length=255"`
	LastName  string `orm:"max_length=255"`
	Dob       time.Time
}

// Meta names the table as Django does.
func (rawAuthor) Meta() orm.Meta { return orm.Meta{Table: "raw_query_author"} }

// rawBook is Django's raw_query.Book.
type rawBook struct {
	ID          int64
	Title       string            `orm:"max_length=255"`
	Author      orm.FK[rawAuthor] `orm:"on_delete=cascade"`
	Paperback   bool
	OpeningLine string `orm:"type=text"`
}

// Meta names the table as Django does.
func (rawBook) Meta() orm.Meta { return orm.Meta{Table: "raw_query_book"} }

// rawCoffee is Django's raw_query.Coffee without its price: the ORM has no
// decimal kind.
type rawCoffee struct {
	ID    int64
	Brand string `orm:"max_length=255;column=name"`
}

// Meta names the table as Django does.
func (rawCoffee) Meta() orm.Meta { return orm.Meta{Table: "raw_query_coffee"} }

// rawMixedCaseIDColumn is Django's raw_query.MixedCaseIDColumn.
type rawMixedCaseIDColumn struct {
	ID int64 `orm:"column=MiXeD_CaSe_Id"`
}

// Meta names the table as Django does.
func (rawMixedCaseIDColumn) Meta() orm.Meta {
	return orm.Meta{Table: "raw_query_mixedcaseidcolumn"}
}

// rawModels returns the models of Django's raw_query test app. BookFkAsPk,
// Reviewer and FriendlyAuthor are left out: see the skipped tests.
func rawModels() []any {
	return []any{djangotest.App("raw_query",
		&rawAuthor{}, &rawBook{}, &rawCoffee{}, &rawMixedCaseIDColumn{})}
}

// rawFixture is the data of Django's RawQueryTests.
type rawFixture struct {
	a1, a2, a3, a4 *rawAuthor
	b1, b2, b3, b4 *rawBook
}

// rawSetUp creates the RawQueryTests fixture (without its reviewers).
func rawSetUp(t *testing.T, db *orm.DB) rawFixture {
	t.Helper()
	var f rawFixture
	f.a1 = create(t, db, &rawAuthor{FirstName: "Joe", LastName: "Smith", Dob: day(1950, 9, 20)})
	f.a2 = create(t, db, &rawAuthor{FirstName: "Jill", LastName: "Doe", Dob: day(1920, 4, 2)})
	f.a3 = create(t, db, &rawAuthor{FirstName: "Bob", LastName: "Smith", Dob: day(1986, 1, 25)})
	f.a4 = create(t, db, &rawAuthor{FirstName: "Bill", LastName: "Jones", Dob: day(1932, 5, 10)})
	f.b1 = create(t, db, &rawBook{
		Title: "The awesome book", Author: fkTo[rawAuthor](f.a1.ID), Paperback: false,
		OpeningLine: "It was a bright cold day in April and the clocks were striking thirteen.",
	})
	f.b2 = create(t, db, &rawBook{
		Title: "The horrible book", Author: fkTo[rawAuthor](f.a1.ID), Paperback: true,
		OpeningLine: "On an evening in the latter part of May a middle-aged man " +
			"was walking homeward from Shaston to the village of Marlott, " +
			"in the adjoining Vale of Blakemore, or Blackmoor.",
	})
	f.b3 = create(t, db, &rawBook{
		Title: "Another awesome book", Author: fkTo[rawAuthor](f.a1.ID), Paperback: false,
		OpeningLine: "A squat gray building of only thirty-four stories.",
	})
	f.b4 = create(t, db, &rawBook{
		Title: "Some other book", Author: fkTo[rawAuthor](f.a3.ID), Paperback: true,
		OpeningLine: "It was the day my grandmother exploded.",
	})
	create(t, db, &rawCoffee{Brand: "dunkin doughnuts"})
	create(t, db, &rawCoffee{Brand: "starbucks"})
	return f
}

// rawSameValue compares two model values field by field, as Django's
// assertProcessed does: equal values of equal types. Times compare as instants.
func rawSameValue(t *testing.T, got, want any) {
	t.Helper()
	gv, wv := reflect.ValueOf(got), reflect.ValueOf(want)
	if gv.Type() != wv.Type() {
		t.Errorf("type = %T, want %T", got, want)
		return
	}
	for i := 0; i < gv.NumField(); i++ {
		g, w := gv.Field(i).Interface(), wv.Field(i).Interface()
		if !equalValue(g, w) {
			t.Errorf("%s.%s = %v, want %v", gv.Type().Name(), gv.Type().Field(i).Name, g, w)
		}
	}
}

// rawSameRows checks that the raw results equal the expected rows, in order.
func rawSameRows[T any](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range got {
		rawSameValue(t, got[i], want[i])
	}
}

// rawAll runs a raw queryset and fails the test on an error.
func rawAll[T any](t *testing.T, r *orm.RawQuerySet[T]) []T {
	t.Helper()
	rows, err := r.All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	return rows
}

// rawCheck is Django's assertSuccessfulRawQuery: it runs the raw query and
// compares its rows with the ORM rows.
func rawCheck[T any](t *testing.T, db *orm.DB, query string, args ...any) {
	t.Helper()
	rawSameRows(t, rawAll(t, orm.Raw[T](db, query, args...)), mustAll(t, orm.Query[T](db).OrderBy("pk")))
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_rawqueryset_repr
func TestRawQueryRawQuerySetRepr(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author")
		djangotest.AssertEqual(t, qs.String(), "<RawQuerySet: SELECT * FROM raw_query_author>")
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_simple_raw_query
func TestRawQuerySimpleRawQuery(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		rawCheck[rawAuthor](t, db, "SELECT * FROM raw_query_author ORDER BY id")
	})
}

// Raw queries are lazy: no statement runs before All. Django checks the
// cursor of the query; Go counts the statements.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_raw_query_lazy
func TestRawQueryRawQueryLazy(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		var q *orm.RawQuerySet[rawAuthor]
		djangotest.AssertNumQueries(t, db, 0, func() {
			q = orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author")
		})
		djangotest.AssertNumQueries(t, db, 1, func() { rawAll(t, q) })
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_FK_raw_query
func TestRawQueryFKRawQuery(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		rawCheck[rawBook](t, db, "SELECT * FROM raw_query_book ORDER BY id")
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_fk_fetch_mode_peers
func TestRawQueryFkFetchModePeers(t *testing.T) {
	t.Skip("needs fetch modes, which the ORM does not have")
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_fk_fetch_mode_raise
func TestRawQueryFkFetchModeRaise(t *testing.T) {
	t.Skip("needs fetch modes, which the ORM does not have")
}

// Coffee.price is left out: there is no decimal kind.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_db_column_handler
func TestRawQueryDbColumnHandler(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		rawCheck[rawCoffee](t, db, "SELECT * FROM raw_query_coffee ORDER BY id")
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_pk_with_mixed_case_db_column
func TestRawQueryPkWithMixedCaseDbColumn(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &rawMixedCaseIDColumn{})
		create(t, db, &rawMixedCaseIDColumn{})
		rawSameRows(t,
			rawAll(t, orm.Raw[rawMixedCaseIDColumn](db, "SELECT * FROM raw_query_mixedcaseidcolumn")),
			mustAll(t, orm.Query[rawMixedCaseIDColumn](db)))
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_order_handler
func TestRawQueryOrderHandler(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		for _, sel := range []string{
			"dob, last_name, first_name, id",
			"last_name, dob, first_name, id",
			"first_name, last_name, dob, id",
		} {
			rawCheck[rawAuthor](t, db, "SELECT "+sel+" FROM raw_query_author ORDER BY id")
		}
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_translations
func TestRawQueryTranslations(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		q := orm.Raw[rawAuthor](db,
			"SELECT first_name AS first, last_name AS last, dob, id FROM raw_query_author ORDER BY id").
			Translations(map[string]string{"first": "first_name", "last": "last_name"})
		rawSameRows(t, rawAll(t, q), mustAll(t, orm.Query[rawAuthor](db).OrderBy("pk")))
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_params
func TestRawQueryParams(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		author := mustAll(t, orm.Query[rawAuthor](db).OrderBy("pk"))[2]
		q := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author WHERE first_name = ?", author.FirstName)
		res, err := q.AllAnnotated(context.Background())
		if err != nil {
			t.Fatalf("AllAnnotated: %v", err)
		}
		djangotest.AssertEqual(t, len(res), 1)
		rawSameValue(t, res[0].Object, author)
		djangotest.AssertEqual(t, len(res[0].Annotations), 0)
		djangotest.AssertEqual(t, q.String() != "", true)
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_params_none
func TestRawQueryParamsNone(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		q := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author WHERE first_name like 'J%'")
		djangotest.AssertEqual(t, len(rawAll(t, q)), 2)
	})
}

// In Go a literal percent sign needs no doubling, so this uses the same SQL as
// test_params_none.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_escaped_percent
func TestRawQueryEscapedPercent(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		q := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author WHERE first_name like 'J%'")
		djangotest.AssertEqual(t, len(rawAll(t, q)), 2)
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_pyformat_params
func TestRawQueryPyformatParams(t *testing.T) {
	t.Skip("Raw takes positional ? parameters only")
}

// Only the positional form is ported, and Go has no RawQuery object to repr.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_query_representation
func TestRawQueryQueryRepresentation(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author WHERE last_name = ?", "foo")
		djangotest.AssertEqual(t, qs.String(),
			"<RawQuerySet: SELECT * FROM raw_query_author WHERE last_name = foo>")
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_many_to_many
func TestRawQueryManyToMany(t *testing.T) {
	t.Skip("needs M2M[T], which belongs to the relations work package")
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_extra_conversions
func TestRawQueryExtraConversions(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		q := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author ORDER BY id").
			Translations(map[string]string{"something": "else"})
		rawSameRows(t, rawAll(t, q), mustAll(t, orm.Query[rawAuthor](db).OrderBy("pk")))
	})
}

// A field missing from the result keeps its Go zero value. Django defers it
// and loads it on first access.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_missing_fields
func TestRawQueryMissingFields(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		for _, a := range rawAll(t, orm.Raw[rawAuthor](db, "SELECT id, first_name, dob FROM raw_query_author")) {
			djangotest.AssertNotEqual(t, a.FirstName, "")
			djangotest.AssertEqual(t, a.LastName, "")
		}
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_missing_fields_without_PK
func TestRawQueryMissingFieldsWithoutPK(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		_, err := orm.Raw[rawAuthor](db, "SELECT first_name, dob FROM raw_query_author").All(context.Background())
		assertErrorContains(t, err, "raw query must include the primary key")
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_missing_fields_fetch_mode_peers
func TestRawQueryMissingFieldsFetchModePeers(t *testing.T) {
	t.Skip("needs fetch modes, which the ORM does not have")
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_missing_fields_fetch_mode_raise
func TestRawQueryMissingFieldsFetchModeRaise(t *testing.T) {
	t.Skip("needs fetch modes, which the ORM does not have")
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_annotations
func TestRawQueryAnnotations(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		q := orm.Raw[rawAuthor](db, "SELECT a.*, count(b.id) as book_count "+
			"FROM raw_query_author a "+
			"LEFT JOIN raw_query_book b ON a.id = b.author_id "+
			"GROUP BY a.id, a.first_name, a.last_name, a.dob ORDER BY a.id")
		res, err := q.AllAnnotated(context.Background())
		if err != nil {
			t.Fatalf("AllAnnotated: %v", err)
		}
		authors := mustAll(t, orm.Query[rawAuthor](db).OrderBy("pk"))
		want := []int64{3, 0, 1, 0}
		djangotest.AssertEqual(t, len(res), len(authors))
		for i, r := range res {
			rawSameValue(t, r.Object, authors[i])
			if got, ok := r.Annotations["book_count"].(int64); !ok || got != want[i] {
				t.Errorf("book_count[%d] = %#v, want int64(%d)", i, r.Annotations["book_count"], want[i])
			}
		}
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_white_space_query
func TestRawQueryWhiteSpaceQuery(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		rawCheck[rawAuthor](t, db, "    SELECT * FROM raw_query_author ORDER BY id")
	})
}

// Go has no iterable cache: each All call runs the query, so two All calls
// are the two iterations.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_multiple_iterations
func TestRawQueryMultipleIterations(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		normal := mustAll(t, orm.Query[rawAuthor](db).OrderBy("pk"))
		q := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author ORDER BY id")
		first, second := rawAll(t, q), rawAll(t, q)
		rawSameRows(t, first, normal)
		rawSameRows(t, second, normal)
		djangotest.AssertEqual(t, len(first), len(second))
	})
}

// Go indexes the slice All returns. The TypeError for a string index has no
// Go counterpart: the compiler rejects it.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_get_item
func TestRawQueryGetItem(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		rows := rawAll(t, orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author ORDER BY id ASC"))
		djangotest.AssertEqual(t, rows[2].FirstName, "Bob")
		djangotest.AssertEqual(t, len(rows[0:2]), 2)
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_inheritance
func TestRawQueryInheritance(t *testing.T) {
	t.Skip("needs model inheritance, which the ORM does not have")
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_query_count
func TestRawQueryQueryCount(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			rawAll(t, orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author"))
		})
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_subquery_in_raw_sql
func TestRawQuerySubqueryInRawSQL(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		rows := rawAll(t, orm.Raw[rawBook](db,
			"SELECT id FROM (SELECT * FROM raw_query_book WHERE paperback IS NOT NULL) sq"))
		djangotest.AssertEqual(t, len(rows), 4)
	})
}

// BookFkAsPk needs a foreign key that is the primary key, which the ORM cannot
// declare (the pk option is not valid on a relation field).
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_db_column_name_is_used_in_raw_query
func TestRawQueryDbColumnNameIsUsedInRawQuery(t *testing.T) {
	t.Skip("the ORM cannot declare a foreign key as the primary key (pk is not valid on a relation field)")
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_decimal_parameter
func TestRawQueryDecimalParameter(t *testing.T) {
	t.Skip("needs DecimalField, which belongs to the field kinds work package")
}

// In Go the slice from one All call is the cache: one query.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_result_caching
func TestRawQueryResultCaching(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			books := rawAll(t, orm.Raw[rawBook](db, "SELECT * FROM raw_query_book"))
			_ = len(books)
			_ = len(books)
		})
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_iterator
func TestRawQueryIterator(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		djangotest.AssertNumQueries(t, db, 2, func() {
			q := orm.Raw[rawBook](db, "SELECT * FROM raw_query_book")
			rawAll(t, q)
			rawAll(t, q)
		})
	})
}

// Go has no truthiness of a queryset: the truth of a result is its length.
//
// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_bool
func TestRawQueryBool(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		djangotest.AssertEqual(t, len(rawAll(t, orm.Raw[rawBook](db, "SELECT * FROM raw_query_book"))) > 0, true)
		djangotest.AssertEqual(t,
			len(rawAll(t, orm.Raw[rawBook](db, "SELECT * FROM raw_query_book WHERE id = 0"))) > 0, false)
	})
}

// Ported from Django: tests/raw_query/tests.py::RawQueryTests.test_len
func TestRawQueryLen(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		rawSetUp(t, db)
		djangotest.AssertEqual(t, len(rawAll(t, orm.Raw[rawBook](db, "SELECT * FROM raw_query_book"))), 4)
		djangotest.AssertEqual(t,
			len(rawAll(t, orm.Raw[rawBook](db, "SELECT * FROM raw_query_book WHERE id = 0"))), 0)
	})
}

// rawCountAuthors counts the authors with a first name through QueryRows.
func rawCountAuthors(ctx context.Context, t *testing.T, db *orm.DB, first string) int {
	t.Helper()
	rows, err := db.QueryRows(ctx, "SELECT count(*) FROM raw_query_author WHERE first_name = ?", first)
	if err != nil {
		t.Fatalf("QueryRows: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatalf("QueryRows: no row")
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return n
}

// TestRawInsideAtomicSeesUncommittedRowsAndRollsBack checks that Exec and Raw
// run on the transaction of Atomic: they see its rows, and its rollback
// removes what they wrote.
func TestRawInsideAtomicSeesUncommittedRowsAndRollsBack(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		errRollback := errors.New("rollback")
		err := db.Atomic(ctx, func(ctx context.Context) error {
			if _, err := db.Exec(ctx,
				"INSERT INTO raw_query_author (first_name, last_name, dob) VALUES (?, ?, ?)",
				"Tx", "Row", day(2000, 1, 1)); err != nil {
				t.Fatalf("Exec: %v", err)
			}
			rows, err := orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author WHERE first_name = ?", "Tx").All(ctx)
			if err != nil {
				t.Fatalf("Raw: %v", err)
			}
			djangotest.AssertEqual(t, len(rows), 1)
			djangotest.AssertEqual(t, rawCountAuthors(ctx, t, db, "Tx"), 1)
			return errRollback
		})
		djangotest.AssertErrorIs(t, err, errRollback)
		djangotest.AssertEqual(t, rawCountAuthors(ctx, t, db, "Tx"), 0)
		djangotest.AssertEqual(t, len(rawAll(t, orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author"))), 0)
	})
}

// TestRawExecAndRawReachQueryHooks checks that the statements of Exec and Raw
// reach the query hooks, so CaptureQueries and AssertNumQueries count them.
func TestRawExecAndRawReachQueryHooks(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		qc := djangotest.CaptureQueries(db)
		defer qc.Stop()
		if _, err := db.Exec(ctx,
			"INSERT INTO raw_query_author (first_name, last_name, dob) VALUES (?, ?, ?)",
			"Hook", "Row", day(2000, 1, 1)); err != nil {
			t.Fatalf("Exec: %v", err)
		}
		rawAll(t, orm.Raw[rawAuthor](db, "SELECT * FROM raw_query_author"))
		queries := qc.Queries()
		djangotest.AssertEqual(t, len(queries), 2)
		if len(queries) == 2 {
			if !strings.HasPrefix(queries[0].SQL, "INSERT INTO raw_query_author") {
				t.Errorf("first statement = %q, want the INSERT", queries[0].SQL)
			}
			if !strings.HasPrefix(queries[1].SQL, "SELECT") {
				t.Errorf("second statement = %q, want the SELECT", queries[1].SQL)
			}
		}
	})
}

// TestRawRebind checks that Rebind numbers the ? placeholders for PostgreSQL
// and leaves those in string literals and comments alone.
func TestRawRebind(t *testing.T) {
	djangotest.EachDB(t, rawModels(), func(t *testing.T, db *orm.DB) {
		const in = "SELECT ? , '?' -- ?"
		want := in
		if db.Dialect().Name() == "postgres" {
			want = "SELECT $1 , '?' -- ?"
		}
		djangotest.AssertEqual(t, db.Rebind(in), want)
	})
}
