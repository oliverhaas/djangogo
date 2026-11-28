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

// fnChrAuthors creates the authors of Django's ChrTests and OrdTests.
func fnChrAuthors(t *testing.T, db *orm.DB) (john, elena, rhonda *fnAuthor) {
	t.Helper()
	john = create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
	elena = create(t, db, &fnAuthor{Name: "Élena Jordan", Alias: new("elena")})
	rhonda = create(t, db, &fnAuthor{Name: "Rhonda"})
	return john, elena, rhonda
}

// Ported from Django: tests/db_functions/text/test_chr.py::ChrTests.test_basic
func TestChrBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, elena, rhonda := fnChrAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("first_initial", orm.Left("name", 1))
		djangotest.AssertQuerySetEqual(t, authors.Filter("first_initial", orm.Chr(int('J'))),
			[]fnAuthor{*john}, nil, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, authors.Exclude("first_initial", orm.Chr(int('J'))),
			[]fnAuthor{*elena, *rhonda}, nil, djangotest.Unordered())
	})
}

// Ported from Django: tests/db_functions/text/test_chr.py::ChrTests.test_non_ascii
func TestChrNonASCII(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, elena, rhonda := fnChrAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("first_initial", orm.Left("name", 1))
		djangotest.AssertQuerySetEqual(t, authors.Filter("first_initial", orm.Chr(int('É'))),
			[]fnAuthor{*elena}, nil, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, authors.Exclude("first_initial", orm.Chr(int('É'))),
			[]fnAuthor{*john, *rhonda}, nil, djangotest.Unordered())
	})
}

// Ported from Django: tests/db_functions/text/test_chr.py::ChrTests.test_transform
func TestChrTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindInt, "chr", orm.Chr))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, elena, rhonda := fnChrAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("name_code_point", orm.Ord("name"))
		djangotest.AssertQuerySetEqual(t, authors.Filter("name_code_point__chr", orm.Chr(int('J'))),
			[]fnAuthor{*john}, nil, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, authors.Exclude("name_code_point__chr", orm.Chr(int('J'))),
			[]fnAuthor{*elena, *rhonda}, nil, djangotest.Unordered())
	})
}

// Ported from Django: tests/db_functions/text/test_chr.py::ChrTests.test_annotate
func TestChrAnnotate(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, _, _ := fnChrAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate(
			"first_initial", orm.Left("name", 1),
			"initial_chr", orm.Chr(int('J')),
		)
		djangotest.AssertQuerySetEqual(t, authors.Filter("first_initial", orm.F("initial_chr")),
			[]fnAuthor{*john}, nil)
	})
}

// Ported from Django: tests/db_functions/text/test_left.py::LeftTests.test_basic
func TestLeftBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate("name_part", orm.Left("name", 5))
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{"John ", "Rhond"})
		if _, err := orm.Query[fnAuthor](db).Filter("alias__isnull", true).
			Update(context.Background(), "alias", orm.Lower(orm.Left("name", 2))); err != nil {
			t.Fatalf("Update: %v", err)
		}
		assertFlat(t, authors.OrderBy("name").ValuesList("alias"), []any{"smithj", "rh"})
	})
}

// Ported from Django: tests/db_functions/text/test_left.py::LeftTests.test_invalid_length
func TestLeftInvalidLength(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnAssertQueryError(t, orm.Query[fnAuthor](db).Annotate("raises", orm.Left("name", 0)).ValuesList("raises"),
			"'length' must be greater than 0")
	})
}

// Ported from Django: tests/db_functions/text/test_left.py::LeftTests.test_expressions
func TestLeftExpressions(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate(
			"name_part", orm.Left("name", orm.ExpressionWrapper(orm.Value(3), orm.Field{Kind: orm.KindInt})),
		)
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{"Joh", "Rho"})
	})
}

// fnHashAuthors creates the authors of Django's MD5 and SHA tests.
func fnHashAuthors(t *testing.T, db *orm.DB) {
	t.Helper()
	for _, alias := range []*string{new("John Smith"), new("Jordan Élena"), new("皇帝"), new(""), nil} {
		create(t, db, &fnAuthor{Alias: alias})
	}
}

// Ported from Django: tests/db_functions/text/test_md5.py::MD5Tests.test_basic
func TestMD5Basic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("md5_alias", orm.MD5("alias")).
			OrderBy("pk").ValuesList("md5_alias")
		assertFlat(t, authors, []any{
			"6117323d2cabbc17d44c2b44587f682c",
			"ca6d48f6772000141e66591aee49d56c",
			"bf2c13bc1154e3d2e7df848cbc8be73d",
			"d41d8cd98f00b204e9800998ecf8427e",
			nil,
		})
	})
}

// Ported from Django: tests/db_functions/text/test_md5.py::MD5Tests.test_transform
func TestMD5Transform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "md5", orm.MD5))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Filter("alias__md5", "6117323d2cabbc17d44c2b44587f682c").
			ValuesList("alias")
		assertFlat(t, authors, []any{"John Smith"})
	})
}

// Ported from Django: tests/db_functions/text/test_ord.py::OrdTests.test_basic
func TestOrdBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, elena, rhonda := fnChrAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("name_part", orm.Ord("name"))
		djangotest.AssertQuerySetEqual(t, authors.Filter("name_part__gt", orm.Ord(orm.Value("John"))),
			[]fnAuthor{*elena, *rhonda}, nil, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, authors.Exclude("name_part__gt", orm.Ord(orm.Value("John"))),
			[]fnAuthor{*john}, nil, djangotest.Unordered())
	})
}

// Ported from Django: tests/db_functions/text/test_ord.py::OrdTests.test_transform
func TestOrdTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "ord", orm.Ord))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, elena, rhonda := fnChrAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("first_initial", orm.Left("name", 1))
		djangotest.AssertQuerySetEqual(t, authors.Filter("first_initial__ord", int('J')),
			[]fnAuthor{*john}, nil, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, authors.Exclude("first_initial__ord", int('J')),
			[]fnAuthor{*elena, *rhonda}, nil, djangotest.Unordered())
	})
}

// Ported from Django: tests/db_functions/text/test_pad.py::PadTests.test_pad
func TestPad(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John", Alias: new("j")})
		space := orm.Value(" ")
		tests := []struct {
			function   orm.Expr
			paddedName any
		}{
			{orm.LPad("name", 7, orm.Value("xy")), "xyxJohn"},
			{orm.RPad("name", 7, orm.Value("xy")), "Johnxyx"},
			{orm.LPad("name", 6, orm.Value("x")), "xxJohn"},
			{orm.RPad("name", 6, orm.Value("x")), "Johnxx"},
			{orm.LPad("name", 6, space), "  John"},
			{orm.RPad("name", 6, space), "John  "},
			{orm.LPad("name", 2, space), "Jo"},
			{orm.RPad("name", 2, space), "Jo"},
			{orm.LPad("name", 0, space), ""},
			{orm.RPad("name", 0, space), ""},
			{orm.LPad("name", nil, space), nil},
			{orm.RPad("name", nil, space), nil},
			{orm.LPad(orm.Value(nil), 1, space), nil},
			{orm.RPad(orm.Value(nil), 1, space), nil},
			{orm.LPad("goes_by", 1, space), nil},
			{orm.RPad("goes_by", 1, space), nil},
		}
		for _, tt := range tests {
			authors := orm.Query[fnAuthor](db).Annotate("padded_name", tt.function)
			assertFlat(t, authors.ValuesList("padded_name"), []any{tt.paddedName})
		}
	})
}

// Ported from Django: tests/db_functions/text/test_pad.py::PadTests.test_pad_negative_length
func TestPadNegativeLength(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		for _, fn := range []func(any, any, any) orm.Expr{orm.LPad, orm.RPad} {
			fnAssertQueryError(t,
				orm.Query[fnAuthor](db).Annotate("padded", fn("name", -1, orm.Value(" "))).ValuesList("padded"),
				"'length' must be greater or equal to 0.")
		}
	})
}

// Ported from Django: tests/db_functions/text/test_pad.py::PadTests.test_combined_with_length
func TestPadCombinedWithLength(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Rhonda", Alias: new("john_smith")})
		create(t, db, &fnAuthor{Name: "♥♣♠", Alias: new("bytes")})
		authors := orm.Query[fnAuthor](db).Annotate("filled", orm.LPad("name", orm.Length("alias"), orm.Value(" ")))
		assertFlat(t, authors.OrderBy("alias").ValuesList("filled"), []any{"  ♥♣♠", "    Rhonda"})
	})
}

// Ported from Django: tests/db_functions/text/test_repeat.py::RepeatTests.test_basic
func TestRepeatBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John", Alias: new("xyz")})
		tests := []struct {
			function     orm.Expr
			repeatedText any
		}{
			{orm.Repeat("name", 0), ""},
			{orm.Repeat("name", 2), "JohnJohn"},
			{orm.Repeat("name", orm.Length("alias")), "JohnJohnJohn"},
			{orm.Repeat(orm.Value("x"), 3), "xxx"},
			{orm.Repeat("name", nil), nil},
			{orm.Repeat(orm.Value(nil), 4), nil},
			{orm.Repeat("goes_by", 1), nil},
		}
		for _, tt := range tests {
			authors := orm.Query[fnAuthor](db).Annotate("repeated_text", tt.function)
			assertFlat(t, authors.ValuesList("repeated_text"), []any{tt.repeatedText})
		}
	})
}

// Ported from Django: tests/db_functions/text/test_repeat.py::RepeatTests.test_negative_number
func TestRepeatNegativeNumber(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnAssertQueryError(t, orm.Query[fnAuthor](db).Annotate("repeated", orm.Repeat("name", -1)).ValuesList("repeated"),
			"'number' must be greater or equal to 0.")
	})
}

// Ported from Django: tests/db_functions/text/test_replace.py::ReplaceTests.test_replace_with_empty_string
func TestReplaceWithEmptyString(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "George R. R. Martin"})
		create(t, db, &fnAuthor{Name: "J. R. R. Tolkien"})
		qs := orm.Query[fnAuthor](db).Annotate(
			"without_middlename", orm.Replace(orm.F("name"), orm.Value("R. R. "), orm.Value("")),
		)
		assertValuesList(t, qs.OrderBy("name").ValuesList("name", "without_middlename"), [][]any{
			{"George R. R. Martin", "George Martin"},
			{"J. R. R. Tolkien", "J. Tolkien"},
		})
	})
}

// Ported from Django: tests/db_functions/text/test_replace.py::ReplaceTests.test_case_sensitive
func TestReplaceCaseSensitive(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "George R. R. Martin"})
		create(t, db, &fnAuthor{Name: "J. R. R. Tolkien"})
		qs := orm.Query[fnAuthor](db).Annotate(
			"same_name", orm.Replace(orm.F("name"), orm.Value("r. r."), orm.Value("")),
		)
		assertValuesList(t, qs.OrderBy("name").ValuesList("name", "same_name"), [][]any{
			{"George R. R. Martin", "George R. R. Martin"},
			{"J. R. R. Tolkien", "J. R. R. Tolkien"},
		})
	})
}

// Ported from Django: tests/db_functions/text/test_replace.py::ReplaceTests.test_replace_expression
func TestReplaceExpression(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "George R. R. Martin"})
		create(t, db, &fnAuthor{Name: "J. R. R. Tolkien"})
		qs := orm.Query[fnAuthor](db).Annotate(
			"same_name", orm.Replace(
				orm.Concat(orm.Value("Author: "), orm.F("name")), orm.Value("Author: "), orm.Value(""),
			),
		)
		assertValuesList(t, qs.OrderBy("name").ValuesList("name", "same_name"), [][]any{
			{"George R. R. Martin", "George R. R. Martin"},
			{"J. R. R. Tolkien", "J. R. R. Tolkien"},
		})
	})
}

// Ported from Django: tests/db_functions/text/test_replace.py::ReplaceTests.test_update
func TestReplaceUpdate(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "George R. R. Martin"})
		create(t, db, &fnAuthor{Name: "J. R. R. Tolkien"})
		fnUpdate(t, db, "name", orm.Replace(orm.F("name"), orm.Value("R. R. "), orm.Value("")))
		assertFlat(t, orm.Query[fnAuthor](db).OrderBy("name").ValuesList("name"),
			[]any{"George Martin", "J. Tolkien"})
	})
}

// No Go counterpart: tests/db_functions/text/test_replace.py::ReplaceTests.test_replace_with_default_arg
// checks the default replacement of Replace(name, text). orm.Replace takes
// three arguments.

// fnReverseAuthors creates the authors of Django's ReverseTests.
func fnReverseAuthors(t *testing.T, db *orm.DB) (john, elena, python *fnAuthor) {
	t.Helper()
	john = create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
	elena = create(t, db, &fnAuthor{Name: "Élena Jordan", Alias: new("elena")})
	python = create(t, db, &fnAuthor{Name: "パイソン"})
	return john, elena, python
}

// Ported from Django: tests/db_functions/text/test_reverse.py::ReverseTests.test_null
func TestReverseNull(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		_, _, python := fnReverseAuthors(t, db)
		got := firstValue(t, orm.Query[fnAuthor](db).Annotate("backward", orm.Reverse("alias")).
			Filter("pk", python.ID).ValuesList("backward"))
		djangotest.AssertEqual(t, got, nil)
	})
}

// Ported from Django: tests/db_functions/text/test_reverse.py::ReverseTests.test_basic
func TestReverseBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnReverseAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate(
			"backward", orm.Reverse("name"),
			"constant", orm.Reverse(orm.Value("static string")),
		)
		assertValuesList(t, authors.OrderBy("id").ValuesList("name", "backward", "constant"), [][]any{
			{"John Smith", "htimS nhoJ", "gnirts citats"},
			{"Élena Jordan", "nadroJ anelÉ", "gnirts citats"},
			{"パイソン", "ンソイパ", "gnirts citats"},
		})
	})
}

// Ported from Django: tests/db_functions/text/test_reverse.py::ReverseTests.test_transform
func TestReverseTransform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "reverse", orm.Reverse))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, elena, python := fnReverseAuthors(t, db)
		authors := orm.Query[fnAuthor](db)
		djangotest.AssertQuerySetEqual(t, authors.Filter("name__reverse", "htimS nhoJ"),
			[]fnAuthor{*john}, nil, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, authors.Exclude("name__reverse", "htimS nhoJ"),
			[]fnAuthor{*elena, *python}, nil, djangotest.Unordered())
	})
}

// Ported from Django: tests/db_functions/text/test_reverse.py::ReverseTests.test_expressions
func TestReverseExpressions(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "reverse", orm.Reverse))
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "length", orm.Length))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		john, elena, python := fnReverseAuthors(t, db)
		got := firstValue(t, orm.Query[fnAuthor](db).Annotate("backward", orm.Reverse(orm.Trim("name"))).
			Filter("pk", john.ID).ValuesList("backward"))
		djangotest.AssertEqual(t, got, any("htimS nhoJ"))
		authors := orm.Query[fnAuthor](db)
		djangotest.AssertQuerySetEqual(t, authors.Filter("name__reverse__length__gt", 7),
			[]fnAuthor{*john, *elena}, nil, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t, authors.Exclude("name__reverse__length__gt", 7),
			[]fnAuthor{*python}, nil, djangotest.Unordered())
	})
}

// Ported from Django: tests/db_functions/text/test_right.py::RightTests.test_basic
func TestRightBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate("name_part", orm.Right("name", 5))
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{"Smith", "honda"})
		if _, err := orm.Query[fnAuthor](db).Filter("alias__isnull", true).
			Update(context.Background(), "alias", orm.Lower(orm.Right("name", 2))); err != nil {
			t.Fatalf("Update: %v", err)
		}
		assertFlat(t, authors.OrderBy("name").ValuesList("alias"), []any{"smithj", "da"})
	})
}

// Ported from Django: tests/db_functions/text/test_right.py::RightTests.test_invalid_length
func TestRightInvalidLength(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnAssertQueryError(t, orm.Query[fnAuthor](db).Annotate("raises", orm.Right("name", 0)).ValuesList("raises"),
			"'length' must be greater than 0")
	})
}

// Ported from Django: tests/db_functions/text/test_right.py::RightTests.test_zero_length
func TestRightZeroLength(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		create(t, db, &fnAuthor{Name: "Tom", Alias: new("tom")})
		authors := orm.Query[fnAuthor](db).Annotate(
			"name_part", orm.Right("name", orm.Length("name").Sub(orm.Length("alias"))),
		)
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{"mith", nil, ""})
	})
}

// Ported from Django: tests/db_functions/text/test_right.py::RightTests.test_expressions
func TestRightExpressions(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate(
			"name_part", orm.Right("name", orm.ExpressionWrapper(orm.Value(3), orm.Field{Kind: orm.KindInt})),
		)
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{"ith", "nda"})
	})
}

// fnEnablePgcrypto installs PostgreSQL's pgcrypto extension in the public
// schema so that SHA1 can call DIGEST.
func fnEnablePgcrypto(t *testing.T, db *orm.DB) {
	t.Helper()
	if db.Dialect().Name() != "postgres" {
		return
	}
	if _, err := db.Exec(context.Background(), "CREATE EXTENSION IF NOT EXISTS pgcrypto SCHEMA public"); err != nil {
		t.Fatalf("create pgcrypto: %v", err)
	}
}

// Ported from Django: tests/db_functions/text/test_sha1.py::SHA1Tests.test_basic
func TestSHA1Basic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnEnablePgcrypto(t, db)
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("sha1_alias", orm.SHA1("alias")).
			OrderBy("pk").ValuesList("sha1_alias")
		assertFlat(t, authors, []any{
			"e61a3587b3f7a142b8c7b9263c82f8119398ecb7",
			"0781e0745a2503e6ded05ed5bc554c421d781b0c",
			"198d15ea139de04060caf95bc3e0ec5883cba881",
			"da39a3ee5e6b4b0d3255bfef95601890afd80709",
			nil,
		})
	})
}

// Ported from Django: tests/db_functions/text/test_sha1.py::SHA1Tests.test_transform
func TestSHA1Transform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "sha1", orm.SHA1))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnEnablePgcrypto(t, db)
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Filter("alias__sha1", "e61a3587b3f7a142b8c7b9263c82f8119398ecb7").
			ValuesList("alias")
		assertFlat(t, authors, []any{"John Smith"})
	})
}

// Ported from Django: tests/db_functions/text/test_sha224.py::SHA224Tests.test_basic
func TestSHA224Basic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("sha224_alias", orm.SHA224("alias")).
			OrderBy("pk").ValuesList("sha224_alias")
		assertFlat(t, authors, []any{
			"a61303c220731168452cb6acf3759438b1523e768f464e3704e12f70",
			"2297904883e78183cb118fc3dc21a610d60daada7b6ebdbc85139f4d",
			"eba942746e5855121d9d8f79e27dfdebed81adc85b6bf41591203080",
			"d14a028c2a3a2bc9476102bb288234c415a2b01f828ea62ac5b3e42f",
			nil,
		})
	})
}

// Ported from Django: tests/db_functions/text/test_sha224.py::SHA224Tests.test_transform
func TestSHA224Transform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "sha224", orm.SHA224))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Filter(
			"alias__sha224", "a61303c220731168452cb6acf3759438b1523e768f464e3704e12f70",
		).ValuesList("alias")
		assertFlat(t, authors, []any{"John Smith"})
	})
}

// No Go counterpart: tests/db_functions/text/test_sha224.py::SHA224Tests.test_unsupported
// runs on Oracle only.

// Ported from Django: tests/db_functions/text/test_sha256.py::SHA256Tests.test_basic
func TestSHA256Basic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("sha256_alias", orm.SHA256("alias")).
			OrderBy("pk").ValuesList("sha256_alias")
		assertFlat(t, authors, []any{
			"ef61a579c907bbed674c0dbcbcf7f7af8f851538eef7b8e58c5bee0b8cfdac4a",
			"6e4cce20cd83fc7c202f21a8b2452a68509cf24d1c272a045b5e0cfc43f0d94e",
			"3ad2039e3ec0c88973ae1c0fce5a3dbafdd5a1627da0a92312c54ebfcf43988e",
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			nil,
		})
	})
}

// Ported from Django: tests/db_functions/text/test_sha256.py::SHA256Tests.test_transform
func TestSHA256Transform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "sha256", orm.SHA256))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Filter(
			"alias__sha256", "ef61a579c907bbed674c0dbcbcf7f7af8f851538eef7b8e58c5bee0b8cfdac4a",
		).ValuesList("alias")
		assertFlat(t, authors, []any{"John Smith"})
	})
}

// Ported from Django: tests/db_functions/text/test_sha384.py::SHA384Tests.test_basic
func TestSHA384Basic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("sha384_alias", orm.SHA384("alias")).
			OrderBy("pk").ValuesList("sha384_alias")
		assertFlat(t, authors, []any{
			"9df976bfbcf96c66fbe5cba866cd4deaa8248806f15b69c4010a404112906e4ca7b57e53b9967b80d77d4f5c2982cbc8",
			"72202c8005492016cc670219cce82d47d6d2d4273464c742ab5811d691b1e82a7489549e3a73ffa119694f90678ba2e3",
			"eda87fae41e59692c36c49e43279c8111a00d79122a282a944e8ba9a403218f049a48326676a43c7ba378621175853b0",
			"38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
			nil,
		})
	})
}

// Ported from Django: tests/db_functions/text/test_sha384.py::SHA384Tests.test_transform
func TestSHA384Transform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "sha384", orm.SHA384))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Filter(
			"alias__sha384",
			"9df976bfbcf96c66fbe5cba866cd4deaa8248806f15b69c4010a404112906e4ca7b57e53b9967b80d77d4f5c2982cbc8",
		).ValuesList("alias")
		assertFlat(t, authors, []any{"John Smith"})
	})
}

// Ported from Django: tests/db_functions/text/test_sha512.py::SHA512Tests.test_basic
func TestSHA512Basic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Annotate("sha512_alias", orm.SHA512("alias")).
			OrderBy("pk").ValuesList("sha512_alias")
		assertFlat(t, authors, []any{
			"ed014a19bb67a85f9c8b1d81e04a0e7101725be8627d79d02ca4f3bd803f33cf3b8fed53e80d2a12c0d0e426824d99d110f0919298a5055efff040a3fc091518",
			"b09c449f3ba49a32ab44754982d4749ac938af293e4af2de28858858080a16112b719514b5e48cb6ce54687e843a4b3e69a04cdb2a9dc99c3b99bdee419fa7d0",
			"b554d182e25fb487a3f2b4285bb8672f98956b5369138e681b467d1f079af116172d88798345a3a7666faf5f35a144c60812d3234dcd35f444624f2faee16857",
			"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
			nil,
		})
	})
}

// Ported from Django: tests/db_functions/text/test_sha512.py::SHA512Tests.test_transform
func TestSHA512Transform(t *testing.T) {
	t.Cleanup(orm.RegisterTransform(orm.KindChar, "sha512", orm.SHA512))
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnHashAuthors(t, db)
		authors := orm.Query[fnAuthor](db).Filter(
			"alias__sha512",
			"ed014a19bb67a85f9c8b1d81e04a0e7101725be8627d79d02ca4f3bd803f33cf3b8fed53e80d2a12c0d0e426824d99d110f0919298a5055efff040a3fc091518",
		).ValuesList("alias")
		assertFlat(t, authors, []any{"John Smith"})
	})
}

// Ported from Django: tests/db_functions/text/test_strindex.py::StrIndexTests.test_annotate_charfield
func TestStrIndexAnnotateCharField(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "George. R. R. Martin"})
		create(t, db, &fnAuthor{Name: "J. R. R. Tolkien"})
		create(t, db, &fnAuthor{Name: "Terry Pratchett"})
		authors := orm.Query[fnAuthor](db).Annotate("fullstop", orm.StrIndex("name", orm.Value("R.")))
		assertFlat(t, authors.OrderBy("name").ValuesList("fullstop"), []any{int64(9), int64(4), int64(0)})
	})
}

// Ported from Django: tests/db_functions/text/test_strindex.py::StrIndexTests.test_annotate_textfield
func TestStrIndexAnnotateTextField(t *testing.T) {
	djangotest.EachDB(t, fnComparisonModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnArticle{
			Title:   "How to Django",
			Text:    "This is about How to Django.",
			Written: time.Now(),
		})
		create(t, db, &fnArticle{
			Title:   "How to Tango",
			Text:    "Won't find anything here.",
			Written: time.Now(),
		})
		articles := orm.Query[fnArticle](db).Annotate("title_pos", orm.StrIndex("text", "title"))
		assertFlat(t, articles.OrderBy("title").ValuesList("title_pos"), []any{int64(15), int64(0)})
	})
}

// Ported from Django: tests/db_functions/text/test_strindex.py::StrIndexTests.test_order_by
func TestStrIndexOrderBy(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "Terry Pratchett"})
		create(t, db, &fnAuthor{Name: "J. R. R. Tolkien"})
		create(t, db, &fnAuthor{Name: "George. R. R. Martin"})
		authors := orm.Query[fnAuthor](db)
		assertFlat(t, authors.OrderBy(orm.StrIndex("name", orm.Value("R.")).Asc()).ValuesList("name"), []any{
			"Terry Pratchett",
			"J. R. R. Tolkien",
			"George. R. R. Martin",
		})
		assertFlat(t, authors.OrderBy(orm.StrIndex("name", orm.Value("R.")).Desc()).ValuesList("name"), []any{
			"George. R. R. Martin",
			"J. R. R. Tolkien",
			"Terry Pratchett",
		})
	})
}

// Ported from Django: tests/db_functions/text/test_strindex.py::StrIndexTests.test_unicode_values
func TestStrIndexUnicodeValues(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "ツリー"})
		create(t, db, &fnAuthor{Name: "皇帝"})
		create(t, db, &fnAuthor{Name: "皇帝 ツリー"})
		authors := orm.Query[fnAuthor](db).Annotate("sb", orm.StrIndex("name", orm.Value("リ")))
		assertFlat(t, authors.OrderBy("name").ValuesList("sb"), []any{int64(2), int64(0), int64(5)})
	})
}

// Ported from Django: tests/db_functions/text/test_strindex.py::StrIndexTests.test_filtering
func TestStrIndexFiltering(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "George. R. R. Martin"})
		create(t, db, &fnAuthor{Name: "Terry Pratchett"})
		authors := orm.Query[fnAuthor](db).Annotate("middle_name", orm.StrIndex("name", orm.Value("R."))).
			Filter("middle_name__gt", 0)
		assertFlat(t, authors.ValuesList("name"), []any{"George. R. R. Martin"})
	})
}

// Ported from Django: tests/db_functions/text/test_substr.py::SubstrTests.test_basic
func TestSubstrBasic(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		authors := orm.Query[fnAuthor](db).Annotate("name_part", orm.Substr("name", 5, 3))
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{" Sm", "da"})
		authors = orm.Query[fnAuthor](db).Annotate("name_part", orm.Substr("name", 2, nil))
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{"ohn Smith", "honda"})
		if _, err := orm.Query[fnAuthor](db).Filter("alias__isnull", true).
			Update(context.Background(), "alias", orm.Lower(orm.Substr("name", 1, 5))); err != nil {
			t.Fatalf("Update: %v", err)
		}
		assertFlat(t, authors.OrderBy("name").ValuesList("alias"), []any{"smithj", "rhond"})
	})
}

// Ported from Django: tests/db_functions/text/test_substr.py::SubstrTests.test_start
func TestSubstrStart(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		row := fnFirstRow(t, orm.Query[fnAuthor](db).Annotate(
			"name_part_1", orm.Substr("name", 1, nil),
			"name_part_2", orm.Substr("name", 2, nil),
		).Filter("alias", "smithj").ValuesList("name_part_1", "name_part_2"))
		djangotest.AssertEqual(t, row[0].(string)[1:], row[1].(string))
	})
}

// Ported from Django: tests/db_functions/text/test_substr.py::SubstrTests.test_pos_gt_zero
func TestSubstrPosGtZero(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		fnAssertQueryError(t, orm.Query[fnAuthor](db).Annotate("raises", orm.Substr("name", 0, nil)).ValuesList("raises"),
			"'pos' must be greater than 0")
	})
}

// Ported from Django: tests/db_functions/text/test_substr.py::SubstrTests.test_expressions
func TestSubstrExpressions(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "Rhonda"})
		substr := orm.Substr(orm.Upper("name"), orm.StrIndex("name", orm.Value("h")), 5)
		authors := orm.Query[fnAuthor](db).Annotate("name_part", substr)
		assertFlat(t, authors.OrderBy("name").ValuesList("name_part"), []any{"HN SM", "HONDA"})
	})
}
