package orm

import (
	"testing"
)

// stubDialect is a minimal sqlite-like Dialect for lookup tests.
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

// alwaysNext returns a next() function that always returns "?".
func alwaysNext() func() string {
	return func() string { return "?" }
}

// mustPersonModel registers Person in a fresh registry and returns the Model.
func mustPersonModel(t *testing.T) *Model {
	t.Helper()
	r := NewRegistry()
	m, err := r.Register(&Person{})
	if err != nil {
		t.Fatalf("Register(Person) unexpected error: %v", err)
	}
	return m
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
		{"some__column__gte", "some__column", "gte"},
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
	d := stubDialect{}
	m := mustPersonModel(t)

	// exact with value
	sql, args, err := buildPredicate(d, m, "name", "bob", alwaysNext())
	if err != nil {
		t.Fatalf("exact value: unexpected error: %v", err)
	}
	if sql != `"name" = ?` {
		t.Errorf("exact value sql: got %q, want %q", sql, `"name" = ?`)
	}
	if len(args) != 1 || args[0] != "bob" {
		t.Errorf("exact value args: got %v, want [bob]", args)
	}

	// exact nil -> IS NULL
	sql, args, err = buildPredicate(d, m, "name", nil, alwaysNext())
	if err != nil {
		t.Fatalf("exact nil: unexpected error: %v", err)
	}
	if sql != `"name" IS NULL` {
		t.Errorf("exact nil sql: got %q, want %q", sql, `"name" IS NULL`)
	}
	if len(args) != 0 {
		t.Errorf("exact nil args: got %v, want []", args)
	}
}

// TestBuildPredicate_comparisons checks gt/gte/lt/lte.
func TestBuildPredicate_comparisons(t *testing.T) {
	t.Parallel()
	d := stubDialect{}
	m := mustPersonModel(t)

	cases := []struct {
		key     string
		wantSQL string
	}{
		{"age__gt", `"age" > ?`},
		{"age__gte", `"age" >= ?`},
		{"age__lt", `"age" < ?`},
		{"age__lte", `"age" <= ?`},
	}
	for _, tc := range cases {
		sql, args, err := buildPredicate(d, m, tc.key, int64(18), alwaysNext())
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

// TestBuildPredicate_like checks contains/icontains/startswith/endswith.
func TestBuildPredicate_like(t *testing.T) {
	t.Parallel()
	d := stubDialect{}
	m := mustPersonModel(t)

	cases := []struct {
		key     string
		value   string
		wantSQL string
		wantArg string
	}{
		{"name__contains", "ob", `"name" LIKE ? ESCAPE '\'`, "%ob%"},
		{"name__icontains", "ob", `"name" LIKE ? ESCAPE '\'`, "%ob%"},
		{"name__startswith", "ob", `"name" LIKE ? ESCAPE '\'`, "ob%"},
		{"name__endswith", "ob", `"name" LIKE ? ESCAPE '\'`, "%ob"},
	}
	for _, tc := range cases {
		sql, args, err := buildPredicate(d, m, tc.key, tc.value, alwaysNext())
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.key, err)
			continue
		}
		if sql != tc.wantSQL {
			t.Errorf("%s sql: got %q, want %q", tc.key, sql, tc.wantSQL)
		}
		if len(args) != 1 || args[0] != tc.wantArg {
			t.Errorf("%s args: got %v, want [%s]", tc.key, args, tc.wantArg)
		}
	}
}

// TestBuildPredicate_like_escape checks that %, _, and \ in the value are escaped.
func TestBuildPredicate_like_escape(t *testing.T) {
	t.Parallel()
	d := stubDialect{}
	m := mustPersonModel(t)

	// "a%b" in contains -> "%a\%b%"
	_, args, err := buildPredicate(d, m, "name__contains", "a%b", alwaysNext())
	if err != nil {
		t.Fatalf("contains escape: unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != `%a\%b%` {
		t.Errorf("contains escape: got %v, want [%%a\\%%b%%]", args)
	}

	// "a_b" in startswith -> "a\_b%"
	_, args, err = buildPredicate(d, m, "name__startswith", "a_b", alwaysNext())
	if err != nil {
		t.Fatalf("startswith escape: unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != `a\_b%` {
		t.Errorf("startswith escape: got %v, want [a\\_b%%]", args)
	}

	// "a\b" in endswith -> "%a\\b"
	_, args, err = buildPredicate(d, m, "name__endswith", `a\b`, alwaysNext())
	if err != nil {
		t.Fatalf("endswith escape: unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != `%a\\b` {
		t.Errorf("endswith escape: got %v, want [%%a\\\\b]", args)
	}
}

// TestBuildPredicate_in checks in lookup: normal, empty, non-slice.
func TestBuildPredicate_in(t *testing.T) {
	t.Parallel()
	d := stubDialect{}
	m := mustPersonModel(t)

	// normal in
	sql, args, err := buildPredicate(d, m, "age__in", []int64{1, 2, 3}, alwaysNext())
	if err != nil {
		t.Fatalf("in: unexpected error: %v", err)
	}
	if sql != `"age" IN (?, ?, ?)` {
		t.Errorf("in sql: got %q, want %q", sql, `"age" IN (?, ?, ?)`)
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

	// empty slice -> 0 = 1
	sql, args, err = buildPredicate(d, m, "age__in", []int64{}, alwaysNext())
	if err != nil {
		t.Fatalf("in empty: unexpected error: %v", err)
	}
	if sql != "0 = 1" {
		t.Errorf("in empty sql: got %q, want %q", sql, "0 = 1")
	}
	if len(args) != 0 {
		t.Errorf("in empty args: got %v, want []", args)
	}

	// non-slice -> error
	_, _, err = buildPredicate(d, m, "age__in", 42, alwaysNext())
	if err == nil {
		t.Error("in non-slice: expected error, got nil")
	}
}

// TestBuildPredicate_isnull checks isnull lookup.
func TestBuildPredicate_isnull(t *testing.T) {
	t.Parallel()
	d := stubDialect{}
	m := mustPersonModel(t)

	// true -> IS NULL
	sql, args, err := buildPredicate(d, m, "name__isnull", true, alwaysNext())
	if err != nil {
		t.Fatalf("isnull true: unexpected error: %v", err)
	}
	if sql != `"name" IS NULL` {
		t.Errorf("isnull true sql: got %q, want %q", sql, `"name" IS NULL`)
	}
	if len(args) != 0 {
		t.Errorf("isnull true args: got %v, want []", args)
	}

	// false -> IS NOT NULL
	sql, args, err = buildPredicate(d, m, "name__isnull", false, alwaysNext())
	if err != nil {
		t.Fatalf("isnull false: unexpected error: %v", err)
	}
	if sql != `"name" IS NOT NULL` {
		t.Errorf("isnull false sql: got %q, want %q", sql, `"name" IS NOT NULL`)
	}
	if len(args) != 0 {
		t.Errorf("isnull false args: got %v, want []", args)
	}

	// non-bool -> error
	_, _, err = buildPredicate(d, m, "name__isnull", "yes", alwaysNext())
	if err == nil {
		t.Error("isnull non-bool: expected error, got nil")
	}
}

// TestBuildPredicate_unknownField checks that an unknown column yields an error.
func TestBuildPredicate_unknownField(t *testing.T) {
	t.Parallel()
	d := stubDialect{}
	m := mustPersonModel(t)

	_, _, err := buildPredicate(d, m, "bogus", "x", alwaysNext())
	if err == nil {
		t.Error("unknown field: expected error, got nil")
	}
}

// TestBuildPredicate_unknownLookup checks that an unknown lookup yields an error.
func TestBuildPredicate_unknownLookup(t *testing.T) {
	t.Parallel()
	d := stubDialect{}
	m := mustPersonModel(t)

	_, _, err := buildPredicate(d, m, "name__weird", "x", alwaysNext())
	if err == nil {
		t.Error("unknown lookup: expected error, got nil")
	}
}
