package orm_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The deconstruction tests of Django's constraint tests check the arguments a
// migration writes for a constraint. orm.ConstraintSource is the Go
// counterpart: it renders the Go source that builds the constraint.

// sourceLiteral renders the literals of the test conditions.
func sourceLiteral(v any) (string, error) {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x), nil
	case int:
		return fmt.Sprintf("int64(%d)", x), nil
	case int64:
		return fmt.Sprintf("int64(%d)", x), nil
	case string:
		return strconv.Quote(x), nil
	}
	return "", fmt.Errorf("no source for %T", v)
}

// assertSource checks the source of c.
func assertSource(t *testing.T, c orm.Constraint, want string) {
	t.Helper()
	src, err := orm.ConstraintSource(c, sourceLiteral)
	if err != nil {
		t.Fatalf("ConstraintSource: %v", err)
	}
	djangotest.AssertEqual(t, src, want)
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_deconstruction
func TestConstraintsCheckDeconstruction(t *testing.T) {
	assertSource(t,
		orm.CheckConstraint{Condition: orm.Q("price__gt", orm.F("discounted_price")), Name: "price_gt_discounted_price"},
		`orm.CheckConstraint{Name: "price_gt_discounted_price", Condition: orm.Q("price__gt", orm.F("discounted_price"))}`)
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_deconstruction
func TestConstraintsUniqueDeconstruction(t *testing.T) {
	assertSource(t,
		orm.UniqueConstraint{Fields: []string{"foo", "bar"}, Name: "unique_fields"},
		`orm.UniqueConstraint{Name: "unique_fields", Fields: []string{"foo", "bar"}}`)
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_deconstruction_with_condition
func TestConstraintsUniqueDeconstructionWithCondition(t *testing.T) {
	assertSource(t,
		orm.UniqueConstraint{Fields: []string{"foo", "bar"}, Name: "unique_fields", Condition: orm.Q("foo", orm.F("bar"))},
		`orm.UniqueConstraint{Name: "unique_fields", Fields: []string{"foo", "bar"}, Condition: orm.Q("foo", orm.F("bar"))}`)
}

// No Go counterpart: UniqueConstraintTests.test_deconstruction_with_deferrable,
// test_deconstruction_with_include and test_deconstruction_with_opclasses test
// options that orm.UniqueConstraint does not have.

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_deconstruction_with_nulls_distinct
//
// Django writes nulls_distinct=True when it is given. NULL values are
// distinct by default in Go, so the test sets the opposite option, which is
// the one the source names.
func TestConstraintsUniqueDeconstructionWithNullsDistinct(t *testing.T) {
	assertSource(t,
		orm.UniqueConstraint{Fields: []string{"foo", "bar"}, Name: "unique_fields", NullsNotDistinct: true},
		`orm.UniqueConstraint{Name: "unique_fields", Fields: []string{"foo", "bar"}, NullsNotDistinct: true}`)
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_deconstruction_with_expressions
func TestConstraintsUniqueDeconstructionWithExpressions(t *testing.T) {
	assertSource(t,
		orm.UniqueConstraint{Expressions: []orm.Expr{orm.Lower("title")}, Name: "unique_fields"},
		`orm.UniqueConstraint{Name: "unique_fields", Expressions: []orm.Expr{orm.Lower(orm.F("title"))}}`)
}

// No Django test exists: the source of a condition is the code that builds
// the same condition tree, so writing a constraint into a migration and
// reading it back gives the same source.
func TestConstraintSourceRebuildsConditions(t *testing.T) {
	tests := []struct {
		cond orm.Cond
		src  string
	}{
		{orm.Q("a", int64(1), "b__lt", int64(2)), `orm.Q("a", int64(1), "b__lt", int64(2))`},
		{orm.Q("a", int64(1)).Or(orm.Q("b", int64(2))), `orm.Q("a", int64(1)).Or(orm.Q("b", int64(2)))`},
		{
			orm.Q("a", int64(1), "b", int64(2)).Or(orm.Q("c", true)),
			`orm.Q("a", int64(1), "b", int64(2)).Or(orm.Q("c", true))`,
		},
		{
			orm.Q(orm.Q("a", int64(1)).Or(orm.Q("b", int64(2)))),
			`orm.Q(orm.Q("a", int64(1)).Or(orm.Q("b", int64(2))))`,
		},
		{
			orm.Q("a", int64(1)).Or(orm.Q("b", int64(2))).Or(orm.Not(orm.Q("c", int64(3)))),
			`orm.Q("a", int64(1)).Or(orm.Q("b", int64(2)), orm.Not(orm.Q("c", int64(3))))`,
		},
		{orm.Not(orm.Q("a", "x")), `orm.Not(orm.Q("a", "x"))`},
		{
			orm.Q("a", int64(1), orm.Not(orm.Q("b", int64(2)))),
			`orm.Q("a", int64(1), orm.Not(orm.Q("b", int64(2))))`,
		},
		{
			orm.Q("a__in", []int{1, 2}, orm.Q("b", int64(1)).Or(orm.Q("c", int64(2)))),
			`orm.Q("a__in", []any{int64(1), int64(2)}, orm.Q("b", int64(1)).Or(orm.Q("c", int64(2))))`,
		},
		{
			orm.Q("read", orm.Value(100).Sub(orm.F("unread")), "n__gt", orm.Length("name").Mul(orm.F("k"))),
			`orm.Q("read", orm.Value(int64(100)).Sub(orm.F("unread")), "n__gt", orm.Length(orm.F("name")).Mul(orm.F("k")))`,
		},
		{orm.Q("a__isnull", nil), `orm.Q("a__isnull", nil)`},
		{
			orm.Q("total__gt", orm.Coalesce("a", orm.Value(0)), "label", orm.Concat("a", orm.Value("-"), "b")),
			`orm.Q("total__gt", orm.Coalesce(orm.F("a"), orm.Value(int64(0))), "label", orm.Concat(orm.F("a"), orm.Value("-"), orm.F("b")))`,
		},
		{
			orm.Q("n__gt", orm.Cast("text", orm.Field{Kind: orm.KindDecimal, MaxDigits: 5, DecimalPlaces: 2, Null: true})),
			`orm.Q("n__gt", orm.Cast(orm.F("text"), orm.Field{Kind: orm.KindDecimal, MaxDigits: 5, DecimalPlaces: 2}))`,
		},
		{
			orm.Q("n__gte", orm.Case(orm.When{Cond: orm.Q("kind", "a"), Then: 1}, orm.When{Cond: orm.Q("kind", "b"), Then: "m"}).Default(orm.Value(0))),
			`orm.Q("n__gte", orm.Case(orm.When{Cond: orm.Q("kind", "a"), Then: orm.Value(int64(1))}, orm.When{Cond: orm.Q("kind", "b"), Then: orm.F("m")}).Default(orm.Value(int64(0))))`,
		},
		{
			orm.Q("n__lte", orm.Abs("m"), "active", orm.F("archived").Not()),
			`orm.Q("n__lte", orm.Abs(orm.F("m")), "active", orm.F("archived").Not())`,
		},
		{
			orm.Q("n__lte", orm.Greatest("a", orm.Value(1)), "m__gte", orm.Least("a", "b")),
			`orm.Q("n__lte", orm.Greatest(orm.F("a"), orm.Value(int64(1))), "m__gte", orm.Least(orm.F("a"), orm.F("b")))`,
		},
		{
			orm.Q("n", orm.NullIf("a", orm.Value(0)), "name", orm.Collate("label", "nocase"), "due__lte", orm.Now()),
			`orm.Q("n", orm.NullIf(orm.F("a"), orm.Value(int64(0))), "name", orm.Collate(orm.F("label"), "nocase"), "due__lte", orm.Now())`,
		},
		{
			orm.Q("data", orm.JSONObject("k", "a", "list", orm.JSONArray("b", orm.Value(1)))),
			`orm.Q("data", orm.JSONObject("k", orm.F("a"), "list", orm.JSONArray(orm.F("b"), orm.Value(int64(1)))))`,
		},
		{
			orm.Q("n__lte", orm.Round(orm.Sqrt("a"), 2), "m__gt", orm.Log(orm.Pi(), orm.Ceil("b")), "x__lt", orm.Random()),
			`orm.Q("n__lte", orm.Round(orm.Sqrt(orm.F("a")), 2), "m__gt", orm.Log(orm.Pi(), orm.Ceil(orm.F("b"))), "x__lt", orm.Random())`,
		},
		{
			orm.Q("due__lte", orm.ExpressionWrapper(orm.F("start").Add(orm.F("span")), orm.Field{Kind: orm.KindDateTime, Null: true})),
			`orm.Q("due__lte", orm.ExpressionWrapper(orm.F("start").Add(orm.F("span")), orm.Field{Kind: orm.KindDateTime}))`,
		},
	}
	for _, tt := range tests {
		assertSource(t, orm.CheckConstraint{Name: "c", Condition: tt.cond}, `orm.CheckConstraint{Name: "c", Condition: `+tt.src+`}`)
	}
}
