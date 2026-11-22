package orm_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// fnAuthor is Django's db_functions.Author.
type fnAuthor struct {
	ID     int64
	Name   string  `orm:"max_length=50"`
	Alias  *string `orm:"max_length=50;null;blank"`
	GoesBy *string `orm:"max_length=50;null;blank"`
	Age    int16   `orm:"type=positive_smallint;default=30"`
}

// fnAuthors returns the models of the ports of tests/db_functions/text.
func fnAuthors() []any { return []any{djangotest.App("db_functions", &fnAuthor{})} }

// fnUpdate runs Django's Author.objects.update(**{field: value}).
func fnUpdate(t *testing.T, db *orm.DB, field string, value orm.Expr) {
	t.Helper()
	if _, err := orm.Query[fnAuthor](db).Update(context.Background(), field, value); err != nil {
		t.Fatalf("update: %v", err)
	}
}

// Ported from Django: tests/db_functions/text/test_lower.py::LowerTests.test_basic
func TestLowerBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate("lower_name", orm.Lower("name"))
		assertValuesList(t, authors.OrderBy("name").ValuesList("lower_name"), [][]any{{"john smith"}, {"rhonda"}})
		fnUpdate(t, db, "name", orm.Lower("name"))
		assertValuesList(t, authors.OrderBy("name").ValuesList("lower_name", "name"),
			[][]any{{"john smith", "john smith"}, {"rhonda", "rhonda"}})
	})
}

// No Go counterpart: tests/db_functions/text/test_lower.py::LowerTests.test_num_args
// checks the TypeError of Lower("name", "name"). orm.Lower takes one
// argument, so the call does not compile.

// Ported from Django: tests/db_functions/text/test_lower.py::LowerTests.test_transform
func TestLowerTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "lower", orm.Lower))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Filter("name__lower__exact", "john smith")
		assertValuesList(t, authors.OrderBy("name").ValuesList("name"), [][]any{{"John Smith"}})
	})
}

// Ported from Django: tests/db_functions/text/test_upper.py::UpperTests.test_basic
func TestUpperBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate("upper_name", orm.Upper("name"))
		assertValuesList(t, authors.OrderBy("name").ValuesList("upper_name"), [][]any{{"JOHN SMITH"}, {"RHONDA"}})
		fnUpdate(t, db, "name", orm.Upper("name"))
		assertValuesList(t, authors.OrderBy("name").ValuesList("upper_name", "name"),
			[][]any{{"JOHN SMITH", "JOHN SMITH"}, {"RHONDA", "RHONDA"}})
	})
}

// Ported from Django: tests/db_functions/text/test_upper.py::UpperTests.test_transform
func TestUpperTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "upper", orm.Upper))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Filter("name__upper__exact", "JOHN SMITH")
		assertValuesList(t, authors.OrderBy("name").ValuesList("name"), [][]any{{"John Smith"}})
	})
}

// Ported from Django: tests/db_functions/text/test_length.py::LengthTests.test_basic
func TestLengthBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate(
			"name_length", orm.Length("name"),
			"alias_length", orm.Length("alias"),
		)
		assertValuesList(t, authors.OrderBy("name").ValuesList("name_length", "alias_length"),
			[][]any{{int64(10), int64(6)}, {int64(6), nil}})
		n, err := authors.Filter("alias_length__lte", orm.Length("name")).Count(context.Background())
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(1))
	})
}

// Ported from Django: tests/db_functions/text/test_length.py::LengthTests.test_ordering
func TestLengthOrdering(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj1")})
		create(t, db, &fnAuthor{Name: "Rhonda", Alias: new("ronny")})
		authors := orm.Query[fnAuthor](db).OrderBy(orm.Length("name"), orm.Length("alias"))
		assertValuesList(t, authors.ValuesList("name", "alias"), [][]any{
			{"Rhonda", "ronny"},
			{"John Smith", "smithj"},
			{"John Smith", "smithj1"},
		})
	})
}

// Ported from Django: tests/db_functions/text/test_length.py::LengthTests.test_transform
func TestLengthTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "length", orm.Length))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Filter("name__length__gt", 7)
		assertValuesList(t, authors.OrderBy("name").ValuesList("name"), [][]any{{"John Smith"}})
	})
}

// Ported from Django: tests/db_functions/text/test_trim.py::TrimTests.test_trim
func TestTrim(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "  John ", Alias: new("j")})
		create(t, db, &fnAuthor{Name: "Rhonda", Alias: new("r")})
		authors := orm.Query[fnAuthor](db).Annotate(
			"ltrim", orm.LTrim("name"),
			"rtrim", orm.RTrim("name"),
			"trim", orm.Trim("name"),
		)
		assertValuesList(t, authors.OrderBy("alias").ValuesList("ltrim", "rtrim", "trim"), [][]any{
			{"John ", "  John", "John"},
			{"Rhonda", "Rhonda", "Rhonda"},
		})
	})
}

// Ported from Django: tests/db_functions/text/test_trim.py::TrimTests.test_trim_transform
func TestTrimTransform(t *testing.T) {
	tests := []struct {
		name        string
		transform   func(any) orm.Expr
		trimmedName string
	}{
		{"ltrim", orm.LTrim, "John  "},
		{"rtrim", orm.RTrim, " John"},
		{"trim", orm.Trim, "John"},
	}
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: " John  "})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				unregister := orm.RegisterTransform(orm.KindChar, tt.name, tt.transform)
				defer unregister()
				authors := orm.Query[fnAuthor](db).Filter("name__"+tt.name, tt.trimmedName)
				assertValuesList(t, authors.ValuesList("name"), [][]any{{" John  "}})
			})
		}
	})
}

// A registered transform also works in Values, OrderBy and F, and the
// function that RegisterTransform returns removes it again. No Django test
// exists; Django's register_lookup is a context manager of its test
// utilities.
func TestRegisterTransformUnregister(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith"})
		create(t, db, &fnAuthor{Name: "ann"})
		unregister := orm.RegisterTransform(orm.KindChar, "lower", orm.Lower)
		assertValuesList(t, orm.Query[fnAuthor](db).OrderBy("name__lower").ValuesList("name__lower"),
			[][]any{{"ann"}, {"john smith"}})
		assertValuesList(t, orm.Query[fnAuthor](db).Annotate("n", orm.F("name__lower")).OrderBy("n").ValuesList("n"),
			[][]any{{"ann"}, {"john smith"}})
		unregister()
		_, err := orm.Query[fnAuthor](db).Filter("name__lower", "john smith").All(context.Background())
		assertFieldError(t, err, "unsupported lookup 'lower' for CharField")
	})
}

// A text function is the argument of an aggregate. No Django test exists.
func TestLengthInAggregate(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith"})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		got, err := orm.Query[fnAuthor](db).Aggregate(context.Background(), "longest", orm.Max(orm.Length("name")))
		if err != nil {
			t.Fatalf("aggregate: %v", err)
		}
		djangotest.AssertEqual(t, got["longest"], any(int64(10)))
	})
}

// fnConcatAuthors creates the authors of Django's ConcatTests.
func fnConcatAuthors(t *testing.T, db *orm.DB) {
	t.Helper()
	create(t, db, &fnAuthor{Name: "Jayden"})
	create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj"), GoesBy: new("John")})
	create(t, db, &fnAuthor{Name: "Margaret", GoesBy: new("Maggie")})
	create(t, db, &fnAuthor{Name: "Rhonda", Alias: new("adnohR")})
}

// Ported from Django: tests/db_functions/text/test_concat.py::ConcatTests.test_basic
func TestConcatBasic(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		fnConcatAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("joined", orm.Concat("alias", "goes_by"))
		assertFlat(t, authors.OrderBy("name").ValuesList("joined"), []any{"", "smithjJohn", "Maggie", "adnohR"})
	})
}

// Ported from Django: tests/db_functions/text/test_concat.py::ConcatTests.test_gt_two_expressions
//
// The error comes when the query is compiled, not when Concat is called.
func TestConcatGtTwoExpressions(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		_, err := orm.Query[fnAuthor](db).Annotate("joined", orm.Concat("alias")).All(context.Background())
		if err == nil || !strings.Contains(err.Error(), "Concat must take at least two expressions") {
			t.Fatalf("err = %v, want the Concat error", err)
		}
	})
}

// Ported from Django: tests/db_functions/text/test_concat.py::ConcatTests.test_many
func TestConcatMany(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		fnConcatAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("joined", orm.Concat("name", orm.Value(" ("), "goes_by", orm.Value(")")))
		assertFlat(t, authors.OrderBy("name").ValuesList("joined"),
			[]any{"Jayden ()", "John Smith (John)", "Margaret (Maggie)", "Rhonda ()"})
	})
}

// Ported from Django: tests/db_functions/text/test_concat.py::ConcatTests.test_mixed_char_text
func TestConcatMixedCharText(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnArticle{Title: "The Title", Text: fnLoremIpsum, Written: time.Now()})
		want := "The Title - " + fnLoremIpsum
		article := orm.Query[fnArticle](db).Annotate("title_text", orm.Concat("title", orm.Value(" - "), "text")).
			Filter("title", "The Title")
		assertFlat(t, article.ValuesList("title_text"), []any{want})
		article = orm.Query[fnArticle](db).Annotate("title_text", orm.Upper(orm.Concat("title", orm.Value(" - "), "text"))).
			Filter("title", "The Title")
		assertFlat(t, article.ValuesList("title_text"), []any{strings.ToUpper(want)})
	})
}

// No Go counterpart: tests/db_functions/text/test_concat.py::ConcatTests.test_coalesce_idempotent
// counts the nodes of ConcatPair.flatten before and after coalesce. Concat
// has no pairs, and its node is built when the query is compiled.

// Ported from Django: tests/db_functions/text/test_concat.py::ConcatTests.test_sql_generation_idempotency
func TestConcatSQLGenerationIdempotency(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		qs := orm.Query[fnArticle](db).Annotate("description", orm.Concat("title", orm.Value(": "), "summary"))
		capture := djangotest.CaptureQueries(db)
		mustAll(t, qs)
		mustAll(t, qs)
		capture.Stop()
		if q := capture.Queries(); len(q) != 2 || q[0].SQL != q[1].SQL {
			t.Errorf("queries = %v, want the same SQL twice", q)
		}
	})
}

// Ported from Django: tests/db_functions/text/test_concat.py::ConcatTests.test_concat_non_str
//
// Django casts on PostgreSQL only. Here SQLite casts too, with CAST(age AS
// TEXT), which gives the text its || gives.
func TestConcatNonStr(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "The Name", Age: 42})
		capture := djangotest.CaptureQueries(db)
		authors := orm.Query[fnAuthor](db).Annotate("name_text",
			orm.Concat("name", orm.Value(":"), "alias", orm.Value(":"), "age"))
		assertFlat(t, authors.ValuesList("name_text"), []any{"The Name::42"})
		capture.Stop()
		if capture.Len() != 1 {
			t.Fatalf("queries = %d, want 1", capture.Len())
		}
		cast := "CAST("
		if db.Dialect().Name() == "postgres" {
			cast = "::TEXT"
		}
		if n := strings.Count(capture.Queries()[0].SQL, cast); n != 1 {
			t.Errorf("SQL = %s, want one %s", capture.Queries()[0].SQL, cast)
		}
	})
}

// Ported from Django: tests/db_functions/text/test_concat.py::ConcatTests.test_equal
//
// Concat has no output field, so the case of two output fields has no Go
// counterpart.
func TestConcatEqual(t *testing.T) {
	if !reflect.DeepEqual(orm.Concat("foo", "bar"), orm.Concat("foo", "bar")) {
		t.Error("Concat(foo, bar) != Concat(foo, bar)")
	}
	if reflect.DeepEqual(orm.Concat("foo", "bar"), orm.Concat("bar", "foo")) {
		t.Error("Concat(foo, bar) == Concat(bar, foo)")
	}
}
