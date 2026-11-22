package orm_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// fnAuthor is Django's db_functions.Author, with the fields the ports of
// tests/db_functions/text use.
type fnAuthor struct {
	ID    int64
	Name  string  `orm:"max_length=50"`
	Alias *string `orm:"max_length=50;null;blank"`
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
//
// OrderBy takes keys, not expressions, so the lengths are annotated and the
// rows ordered by the annotations.
func TestLengthOrdering(t *testing.T) {
	djangotest.EachDB(t, fnAuthors(), func(t *testing.T, db *orm.DB) {
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj")})
		create(t, db, &fnAuthor{Name: "John Smith", Alias: new("smithj1")})
		create(t, db, &fnAuthor{Name: "Rhonda", Alias: new("ronny")})
		authors := orm.Query[fnAuthor](db).
			Annotate("name_length", orm.Length("name"), "alias_length", orm.Length("alias")).
			OrderBy("name_length", "alias_length")
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
