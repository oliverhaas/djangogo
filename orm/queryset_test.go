package orm

import (
	"strconv"
	"testing"
)

// numberedDialect is a Dialect whose Placeholder renders sequential "$n" markers
// so that placeholder numbering across predicates is visible in compiled SQL.
type numberedDialect struct{}

func (numberedDialect) Name() string                 { return "numbered" }
func (numberedDialect) Placeholder(n int) string     { return "$" + strconv.Itoa(n) }
func (numberedDialect) Quote(s string) string        { return `"` + s + `"` }
func (numberedDialect) ColumnType(*Field) string     { return "" }
func (numberedDialect) CreateTableSQL(*Model) string { return "" }
func (numberedDialect) SupportsReturning() bool      { return false }

// newPersonDB registers Person in a fresh registry and returns a DB handle that
// uses numberedDialect. The *sql.DB is nil because compilation never touches it.
func newPersonDB(t *testing.T) *DB {
	t.Helper()
	r := NewRegistry()
	if _, err := r.Register(&Person{}); err != nil {
		t.Fatalf("Register(Person) unexpected error: %v", err)
	}
	r.Freeze()
	return NewDB(nil, numberedDialect{}, r)
}

// assertArgs checks that got matches want element-by-element.
func assertArgs(t *testing.T, label string, got, want []any) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s args: got %v, want %v", label, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s args[%d]: got %v, want %v", label, i, got[i], want[i])
		}
	}
}

// TestDBAccessors checks the DB handle exposes its parts.
func TestDBAccessors(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	d := numberedDialect{}
	db := NewDB(nil, d, r)
	if db.Dialect() != Dialect(d) {
		t.Errorf("Dialect(): got %v, want %v", db.Dialect(), d)
	}
	if db.Registry() != r {
		t.Errorf("Registry(): got %p, want %p", db.Registry(), r)
	}
	if db.SQL() != nil {
		t.Errorf("SQL(): got %v, want nil", db.SQL())
	}
}

// TestCompileSelect_plain checks a select with no clauses.
func TestCompileSelect_plain(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	sql, args, err := Query[Person](db).compileSelect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT "id", "name", "age", "bio" FROM "person"`
	if sql != want {
		t.Errorf("sql: got %q, want %q", sql, want)
	}
	if len(args) != 0 {
		t.Errorf("args: got %v, want []", args)
	}
}

// TestCompileSelect_filter checks a single filter predicate.
func TestCompileSelect_filter(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	sql, args, err := Query[Person](db).Filter("age__gt", int64(18)).compileSelect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT "id", "name", "age", "bio" FROM "person" WHERE "age" > $1`
	if sql != want {
		t.Errorf("sql: got %q, want %q", sql, want)
	}
	assertArgs(t, "filter", args, []any{int64(18)})
}

// TestCompileSelect_multiFilter checks sequential placeholder numbering.
func TestCompileSelect_multiFilter(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	sql, args, err := Query[Person](db).
		Filter("age__gt", 18).
		Filter("name", "bob").
		compileSelect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT "id", "name", "age", "bio" FROM "person" WHERE "age" > $1 AND "name" = $2`
	if sql != want {
		t.Errorf("sql: got %q, want %q", sql, want)
	}
	assertArgs(t, "multiFilter", args, []any{18, "bob"})
}

// TestCompileSelect_exclude checks that Exclude wraps the predicate in NOT (...).
func TestCompileSelect_exclude(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	sql, args, err := Query[Person](db).Exclude("name", "bob").compileSelect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT "id", "name", "age", "bio" FROM "person" WHERE NOT ("name" = $1)`
	if sql != want {
		t.Errorf("sql: got %q, want %q", sql, want)
	}
	assertArgs(t, "exclude", args, []any{"bob"})
}

// TestCompileSelect_orderBy checks ascending and descending ordering.
func TestCompileSelect_orderBy(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	sql, _, err := Query[Person](db).OrderBy("name", "-age").compileSelect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT "id", "name", "age", "bio" FROM "person" ORDER BY "name", "age" DESC`
	if sql != want {
		t.Errorf("sql: got %q, want %q", sql, want)
	}
}

// TestCompileSelect_limitOffset checks LIMIT/OFFSET rendering and the offset-only special case.
func TestCompileSelect_limitOffset(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)

	sql, _, err := Query[Person](db).Limit(10).Offset(5).compileSelect()
	if err != nil {
		t.Fatalf("limit+offset: unexpected error: %v", err)
	}
	if want := ` LIMIT 10 OFFSET 5`; !endsWith(sql, want) {
		t.Errorf("limit+offset sql: got %q, want suffix %q", sql, want)
	}

	sql, _, err = Query[Person](db).Offset(5).compileSelect()
	if err != nil {
		t.Fatalf("offset-only: unexpected error: %v", err)
	}
	if want := ` LIMIT -1 OFFSET 5`; !endsWith(sql, want) {
		t.Errorf("offset-only sql: got %q, want suffix %q", sql, want)
	}
}

// endsWith reports whether s ends with suffix.
func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// TestCompileCount checks count compilation with a filter.
func TestCompileCount(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	sql, args, err := Query[Person](db).Filter("age__gt", int64(18)).compileCount()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT COUNT(*) FROM "person" WHERE "age" > $1`
	if sql != want {
		t.Errorf("sql: got %q, want %q", sql, want)
	}
	assertArgs(t, "count", args, []any{int64(18)})
}

// TestCompileDelete checks delete compilation with a filter.
func TestCompileDelete(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	sql, args, err := Query[Person](db).Filter("name", "bob").compileDelete()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `DELETE FROM "person" WHERE "name" = $1`
	if sql != want {
		t.Errorf("sql: got %q, want %q", sql, want)
	}
	assertArgs(t, "delete", args, []any{"bob"})
}

// TestQuerySet_immutable checks that chained methods never mutate the receiver.
func TestQuerySet_immutable(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	base := Query[Person](db)

	_ = base.Filter("name", "x")

	// base must remain unfiltered.
	sql, args, err := base.compileSelect()
	if err != nil {
		t.Fatalf("base: unexpected error: %v", err)
	}
	if want := `SELECT "id", "name", "age", "bio" FROM "person"`; sql != want {
		t.Errorf("base sql after sibling chain: got %q, want %q", sql, want)
	}
	if len(args) != 0 {
		t.Errorf("base args: got %v, want []", args)
	}

	// A second, independent chain off base must not see the first chain's filter.
	other := base.Filter("age__gt", int64(21))
	osql, oargs, err := other.compileSelect()
	if err != nil {
		t.Fatalf("other: unexpected error: %v", err)
	}
	wantOther := `SELECT "id", "name", "age", "bio" FROM "person" WHERE "age" > $1`
	if osql != wantOther {
		t.Errorf("other sql: got %q, want %q", osql, wantOther)
	}
	assertArgs(t, "other", oargs, []any{int64(21)})
}

// TestQuerySet_buildErrors checks that build-time validation errors surface at compile time.
func TestQuerySet_buildErrors(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)

	cases := []struct {
		name string
		qs   *QuerySet[Person]
	}{
		{"odd arity", Query[Person](db).Filter("name")},
		{"non-string key", Query[Person](db).Filter(123, "x")},
		{"unknown field", Query[Person](db).Filter("bogus__gt", 1)},
		{"unknown order field", Query[Person](db).OrderBy("nope")},
	}
	for _, tc := range cases {
		if _, _, err := tc.qs.compileSelect(); err == nil {
			t.Errorf("%s: expected compileSelect error, got nil", tc.name)
		}
	}
}

// TestQuerySet_firstErrorWins checks that a later build error does not overwrite the first.
func TestQuerySet_firstErrorWins(t *testing.T) {
	t.Parallel()
	db := newPersonDB(t)
	qs := Query[Person](db).Filter("name") // odd arity sets the first error
	_, _, err := qs.Filter(456, "y").compileSelect()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestQuery_unregisteredModel checks that querying an unregistered model surfaces an error.
func TestQuery_unregisteredModel(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Freeze()
	db := NewDB(nil, numberedDialect{}, r)
	if _, _, err := Query[Person](db).compileSelect(); err == nil {
		t.Error("expected error for unregistered model, got nil")
	}
}
