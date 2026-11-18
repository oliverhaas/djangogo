package orm

import (
	"errors"
	"strings"
	"testing"
)

// stubDialect is a minimal sqlite-like Dialect for lookup tests. It has no
// optional capability, so the pattern lookups use the portable LIKE form.
type stubDialect struct{}

func (stubDialect) Name() string                 { return "stub" }
func (stubDialect) Placeholder(int) string       { return "?" }
func (stubDialect) Quote(s string) string        { return `"` + s + `"` }
func (stubDialect) ColumnType(*Field) string     { return "" }
func (stubDialect) CreateTableSQL(*Model) string { return "" }
func (stubDialect) SupportsReturning() bool      { return false }

// Person is the model used for all lookup tests.
type Person struct {
	ID   int64
	Name string
	Age  int64
	Bio  string `orm:"type=text"`
}

// personSelect is the SELECT every Person query starts with.
const personSelect = `SELECT "person"."id", "person"."name", "person"."age", "person"."bio" FROM "person"`

// newStubDB registers Person in a fresh registry and returns a DB handle on
// stubDialect. The *sql.DB is nil because compiling never touches it.
func newStubDB(t *testing.T) *DB {
	t.Helper()
	r := NewRegistry()
	if _, err := r.Register(&Person{}); err != nil {
		t.Fatalf("Register(Person) unexpected error: %v", err)
	}
	r.Freeze()
	return NewDB(nil, stubDialect{}, r)
}

// wherePredicate compiles Person.Filter(key, value) and returns the WHERE
// clause and its arguments.
func wherePredicate(t *testing.T, key string, value any) (string, []any, error) {
	t.Helper()
	sql, args, err := Query[Person](newStubDB(t)).Filter(key, value).compileSelect()
	if err != nil {
		return "", nil, err
	}
	where, ok := strings.CutPrefix(sql, personSelect+" WHERE ")
	if !ok {
		t.Fatalf("%s: unexpected statement %q", key, sql)
	}
	return where, args, nil
}

// TestParseLookup checks parseLookup unit cases.
func TestParseLookup(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key        string
		wantCol    string
		wantLookup string
	}{
		{"name", "name", "exact"},
		{"age__gt", "age", "gt"},
		{"name__contains", "name", "contains"},
		{"name__icontains", "name", "icontains"},
		{"name__iexact", "name", "iexact"},
		{"age__range", "age", "range"},
		{"name__iregex", "name", "iregex"},
		{"some__column__gte", "some__column", "gte"},
		{"author__name", "author__name", "exact"},
	}
	for _, tc := range cases {
		col, lk := parseLookup(tc.key)
		if col != tc.wantCol || lk != tc.wantLookup {
			t.Errorf("parseLookup(%q) = (%q, %q), want (%q, %q)",
				tc.key, col, lk, tc.wantCol, tc.wantLookup)
		}
	}
}

// TestBuildPredicate_exact checks exact lookups with a value and with nil.
func TestBuildPredicate_exact(t *testing.T) {
	t.Parallel()

	sql, args, err := wherePredicate(t, "name", "bob")
	if err != nil {
		t.Fatalf("exact value: unexpected error: %v", err)
	}
	if sql != `"person"."name" = ?` {
		t.Errorf("exact value sql: got %q, want %q", sql, `"person"."name" = ?`)
	}
	if len(args) != 1 || args[0] != "bob" {
		t.Errorf("exact value args: got %v, want [bob]", args)
	}

	// A nil value means IS NULL, since "= NULL" is never true.
	for _, key := range []string{"name", "name__exact", "name__iexact"} {
		sql, args, err = wherePredicate(t, key, nil)
		if err != nil {
			t.Fatalf("%s nil: unexpected error: %v", key, err)
		}
		if sql != `"person"."name" IS NULL` {
			t.Errorf("%s nil sql: got %q, want %q", key, sql, `"person"."name" IS NULL`)
		}
		if len(args) != 0 {
			t.Errorf("%s nil args: got %v, want []", key, args)
		}
	}

	if _, _, err = wherePredicate(t, "age__gt", nil); err == nil {
		t.Error("gt nil: expected an error, got nil")
	}
}

// TestBuildPredicate_comparisons checks gt/gte/lt/lte.
func TestBuildPredicate_comparisons(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key     string
		wantSQL string
	}{
		{"age__gt", `"person"."age" > ?`},
		{"age__gte", `"person"."age" >= ?`},
		{"age__lt", `"person"."age" < ?`},
		{"age__lte", `"person"."age" <= ?`},
		{"age__exact", `"person"."age" = ?`},
		{"pk", `"person"."id" = ?`},
		{"pk__gt", `"person"."id" > ?`},
	}
	for _, tc := range cases {
		sql, args, err := wherePredicate(t, tc.key, int64(18))
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.key, err)
			continue
		}
		if sql != tc.wantSQL {
			t.Errorf("%s sql: got %q, want %q", tc.key, sql, tc.wantSQL)
		}
		if len(args) != 1 {
			t.Errorf("%s args len: got %d, want 1", tc.key, len(args))
		}
	}
}

// TestBuildPredicate_like checks the LIKE based lookups on a dialect without
// its own lookup SQL.
func TestBuildPredicate_like(t *testing.T) {
	t.Parallel()

	const like = `"person"."name" LIKE ? ESCAPE '\'`
	cases := []struct {
		key     string
		value   string
		wantArg string
	}{
		{"name__contains", "ob", "%ob%"},
		{"name__icontains", "ob", "%ob%"},
		{"name__startswith", "ob", "ob%"},
		{"name__istartswith", "ob", "ob%"},
		{"name__endswith", "ob", "%ob"},
		{"name__iendswith", "ob", "%ob"},
		{"name__iexact", "ob", "ob"},
	}
	for _, tc := range cases {
		sql, args, err := wherePredicate(t, tc.key, tc.value)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.key, err)
			continue
		}
		if sql != like {
			t.Errorf("%s sql: got %q, want %q", tc.key, sql, like)
		}
		if len(args) != 1 || args[0] != tc.wantArg {
			t.Errorf("%s args: got %v, want [%s]", tc.key, args, tc.wantArg)
		}
	}
}

// TestBuildPredicate_like_escape checks that %, _, and \ in the value are escaped.
func TestBuildPredicate_like_escape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key, value, wantArg string
	}{
		{"name__contains", "a%b", `%a\%b%`},
		{"name__startswith", "a_b", `a\_b%`},
		{"name__endswith", `a\b`, `%a\\b`},
		{"name__iexact", "50%_off", `50\%\_off`},
	}
	for _, tc := range cases {
		_, args, err := wherePredicate(t, tc.key, tc.value)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.key, err)
		}
		if len(args) != 1 || args[0] != tc.wantArg {
			t.Errorf("%s: got %v, want [%s]", tc.key, args, tc.wantArg)
		}
	}
}

// TestBuildPredicate_regexUnsupported checks that a dialect without regex SQL
// rejects the regex lookups instead of emitting SQL that cannot run.
func TestBuildPredicate_regexUnsupported(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"name__regex", "name__iregex"} {
		if _, _, err := wherePredicate(t, key, "^b"); err == nil {
			t.Errorf("%s: expected an error on a dialect without regex support", key)
		}
	}
}

// TestBuildPredicate_in checks in lookup: normal, empty, non-slice.
func TestBuildPredicate_in(t *testing.T) {
	t.Parallel()

	sql, args, err := wherePredicate(t, "age__in", []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("in: unexpected error: %v", err)
	}
	if sql != `"person"."age" IN (?, ?, ?)` {
		t.Errorf("in sql: got %q, want %q", sql, `"person"."age" IN (?, ?, ?)`)
	}
	if len(args) != 3 {
		t.Errorf("in args len: got %d, want 3", len(args))
	} else {
		for i, want := range []int64{1, 2, 3} {
			if args[i] != want {
				t.Errorf("in args[%d]: got %v, want %v", i, args[i], want)
			}
		}
	}

	// An empty list matches nothing: no statement is built, so "IN ()" is never
	// rendered.
	if _, _, err = wherePredicate(t, "age__in", []int64{}); !errors.Is(err, errEmptyResultSet) {
		t.Errorf("in empty: got error %v, want errEmptyResultSet", err)
	}

	// NULL is equal to nothing, so nil elements are dropped.
	sql, args, err = wherePredicate(t, "age__in", []any{int64(1), nil})
	if err != nil {
		t.Fatalf("in with nil: unexpected error: %v", err)
	}
	if sql != `"person"."age" IN (?)` || len(args) != 1 {
		t.Errorf("in with nil: got %q %v, want one placeholder", sql, args)
	}

	if _, _, err = wherePredicate(t, "age__in", 42); err == nil {
		t.Error("in non-slice: expected error, got nil")
	}
}

// TestBuildPredicate_range checks the range lookup.
func TestBuildPredicate_range(t *testing.T) {
	t.Parallel()

	sql, args, err := wherePredicate(t, "age__range", []int{18, 30})
	if err != nil {
		t.Fatalf("range: unexpected error: %v", err)
	}
	if sql != `"person"."age" BETWEEN ? AND ?` {
		t.Errorf("range sql: got %q", sql)
	}
	if len(args) != 2 || args[0] != 18 || args[1] != 30 {
		t.Errorf("range args: got %v, want [18 30]", args)
	}

	for name, value := range map[string]any{"one bound": []int{1}, "scalar": 5, "nil bound": []any{1, nil}} {
		if _, _, err := wherePredicate(t, "age__range", value); err == nil {
			t.Errorf("range %s: expected error, got nil", name)
		}
	}
}

// TestBuildPredicate_isnull checks isnull lookup.
func TestBuildPredicate_isnull(t *testing.T) {
	t.Parallel()

	sql, args, err := wherePredicate(t, "name__isnull", true)
	if err != nil {
		t.Fatalf("isnull true: unexpected error: %v", err)
	}
	if sql != `"person"."name" IS NULL` {
		t.Errorf("isnull true sql: got %q, want %q", sql, `"person"."name" IS NULL`)
	}
	if len(args) != 0 {
		t.Errorf("isnull true args: got %v, want []", args)
	}

	sql, args, err = wherePredicate(t, "name__isnull", false)
	if err != nil {
		t.Fatalf("isnull false: unexpected error: %v", err)
	}
	if sql != `"person"."name" IS NOT NULL` {
		t.Errorf("isnull false sql: got %q, want %q", sql, `"person"."name" IS NOT NULL`)
	}
	if len(args) != 0 {
		t.Errorf("isnull false args: got %v, want []", args)
	}

	if _, _, err = wherePredicate(t, "name__isnull", "yes"); err == nil {
		t.Error("isnull non-bool: expected error, got nil")
	}
}

// TestBuildPredicate_unknownField checks that an unknown key yields a
// FieldError that names the model and the valid choices.
func TestBuildPredicate_unknownField(t *testing.T) {
	t.Parallel()

	_, _, err := wherePredicate(t, "bogus", "x")
	if !errors.Is(err, ErrFieldError) {
		t.Fatalf("unknown field: got %v, want a FieldError", err)
	}
	var fe *FieldError
	if !errors.As(err, &fe) || fe.Model != "Person" {
		t.Errorf("unknown field: got %#v, want a FieldError for Person", err)
	}
	const want = "cannot resolve keyword 'bogus' into field. Choices are: age, bio, id, name"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("unknown field: got %q, want it to contain %q", err.Error(), want)
	}
}

// TestBuildPredicate_unknownLookup checks that an unknown lookup yields an error.
func TestBuildPredicate_unknownLookup(t *testing.T) {
	t.Parallel()

	_, _, err := wherePredicate(t, "name__weird", "x")
	if !errors.Is(err, ErrFieldError) {
		t.Fatalf("unknown lookup: got %v, want a FieldError", err)
	}
	const want = "unsupported lookup 'weird' for CharField or join on the field not permitted."
	if !strings.Contains(err.Error(), want) {
		t.Errorf("unknown lookup: got %q, want it to contain %q", err.Error(), want)
	}
}
