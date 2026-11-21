package fidelity

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// simpleIdentRe matches double-quoted SQL identifiers that need no quoting once
// normalized (letters, digits, underscores, starting with a letter or underscore).
var simpleIdentRe = regexp.MustCompile(`"([A-Za-z_][A-Za-z0-9_]*)"`)

// placeholderRe matches positional parameter placeholders across dialects:
// PostgreSQL ($1, $2, ...), Django (%s), and SQLite (?).
var placeholderRe = regexp.MustCompile(`\$\d+|%s|\?`)

// subqueryAliasDeclRe matches a table followed by a Django-style subquery alias
// (U0, U1, ...) in a FROM or JOIN clause.
var subqueryAliasDeclRe = regexp.MustCompile(`\b(FROM|JOIN)\s+([A-Za-z_][A-Za-z0-9_]*)\s+(U\d+)\b`)

// normalizeSQL canonicalizes SQL text before comparing Djan-Go-Go with Django.
//
// Documented normalization rules (all applied outside single-quoted SQL string
// literals so literal patterns such as LIKE ESCAPE '\' stay intact):
//  1. Placeholder style: %s (Django), ? (SQLite), and $1/$2/... (PostgreSQL)
//     all normalize to ?.
//  2. Identifier quoting: simple double-quoted identifiers ("fid_book", "T3",
//     "subquery") are unquoted so quoted and unquoted aliases compare equal.
//  3. Subquery table aliases: Django's generated U0, U1, ... aliases inside
//     IN (SELECT ...) subqueries are expanded back to their table names when
//     each table appears only once in the subquery.
//  4. Whitespace: runs of whitespace collapse to a single space.
func normalizeSQL(raw string) string {
	// Split on single-quoted SQL string literals (handling '' escapes) so only
	// code segments are normalized.
	var parts []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inQuote {
			cur.WriteByte(ch)
			if ch == '\'' {
				if i+1 < len(raw) && raw[i+1] == '\'' {
					i++
					cur.WriteByte('\'')
				} else {
					parts = append(parts, cur.String())
					cur.Reset()
					inQuote = false
				}
			}
			continue
		}
		if ch == '\'' {
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
			cur.WriteByte(ch)
			inQuote = true
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}

	// Pass 1: normalize placeholders, simple quoted identifiers, and whitespace
	// on every non-literal segment.
	for i, seg := range parts {
		if len(seg) > 0 && seg[0] == '\'' {
			continue
		}
		seg = placeholderRe.ReplaceAllString(seg, "?")
		seg = simpleIdentRe.ReplaceAllString(seg, "$1")
		seg = strings.Join(strings.Fields(seg), " ")
		parts[i] = seg
	}
	joined := strings.TrimSpace(strings.Join(parts, " "))
	// Re-stitch spacing around punctuation introduced by segment joining.
	joined = strings.ReplaceAll(joined, "( ", "(")
	joined = strings.ReplaceAll(joined, " )", ")")
	joined = strings.ReplaceAll(joined, " ,", ",")
	joined = strings.ReplaceAll(joined, " .", ".")
	joined = strings.ReplaceAll(joined, ". ", ".")

	// Pass 2: expand unambiguous U0/U1/... subquery aliases back to table names.
	joined = expandSubqueryAliases(joined)
	return joined
}

// expandSubqueryAliases replaces Django's U0, U1, ... subquery aliases with
// their underlying table names when each table appears once in the statement.
func expandSubqueryAliases(sql string) string {
	matches := subqueryAliasDeclRe.FindAllStringSubmatch(sql, -1)
	if len(matches) == 0 {
		return sql
	}
	tableCount := make(map[string]int, len(matches))
	aliasToTable := make(map[string]string, len(matches))
	for _, m := range matches {
		table, alias := m[2], m[3]
		tableCount[table]++
		aliasToTable[alias] = table
	}
	for _, count := range tableCount {
		if count > 1 {
			return sql
		}
	}
	out := subqueryAliasDeclRe.ReplaceAllString(sql, "$1 $2")
	for alias, table := range aliasToTable {
		refRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.`)
		out = refRe.ReplaceAllString(out, table+".")
	}
	return out
}

// normalizeParam converts a bound query parameter (from Go or decoded JSON)
// into a comparable canonical value. Integers normalize to int64, timestamps
// (including Django SQLite's "YYYY-MM-DD HH:MM:SS" strings) normalize to UTC
// RFC3339 strings wrapped in a datetimeMarker.
func normalizeParam(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case uint:
		return int64(x)
	case uint64:
		return int64(x)
	case float32:
		return float64(x)
	case float64:
		return x
	case json.Number:
		if !strings.ContainsAny(x.String(), ".eE") {
			if n, err := x.Int64(); err == nil {
				return n
			}
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	case time.Time:
		return "dt:" + x.UTC().Format(time.RFC3339Nano)
	case string:
		// Django's SQLite backend formats aware UTC datetimes as naive strings.
		if tm, err := time.Parse("2006-01-02 15:04:05", x); err == nil {
			return "dt:" + tm.UTC().Format(time.RFC3339Nano)
		}
		return x
	case map[string]any:
		if len(x) == 1 {
			if s, ok := x["__datetime__"].(string); ok {
				if tm, err := time.Parse(time.RFC3339, s); err == nil {
					return "dt:" + tm.UTC().Format(time.RFC3339Nano)
				}
			}
		}
		return x
	default:
		return fmt.Sprint(x)
	}
}

// normalizeParams normalizes every element of params.
func normalizeParams(params []any) []any {
	out := make([]any, len(params))
	for i, p := range params {
		out[i] = normalizeParam(p)
	}
	return out
}

// normalizeResult converts a Go or JSON-decoded semantic result into a uniform
// structure (int64 for integers, float64 for floats, []any for slices,
// map[string]any for maps) so deep comparison works cleanly.
func normalizeResult(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case bool:
		return x
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return x
	case string:
		return x
	case json.Number:
		if !strings.ContainsAny(x.String(), ".eE") {
			if n, err := x.Int64(); err == nil {
				return n
			}
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	case []int64:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = item
		}
		return out
	case []map[string]any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeResult(item)
		}
		return out
	case [][]any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeResult(item)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeResult(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = normalizeResult(item)
		}
		return out
	default:
		return normalizeParam(v)
	}
}

// resultsEqual compares two normalized semantic results, tolerating tiny
// floating-point differences (e.g. AVG formatting across dialects).
func resultsEqual(got, want any) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	if gf, ok := asFloat(got); ok {
		if wf, ok := asFloat(want); ok {
			return math.Abs(gf-wf) <= 1e-9
		}
	}
	switch g := got.(type) {
	case []any:
		w, ok := want.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range g {
			if !resultsEqual(g[i], w[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		w, ok := want.(map[string]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for k, gv := range g {
			wv, exists := w[k]
			if !exists || !resultsEqual(gv, wv) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}

// asFloat converts float64 or int64 to float64 when either side is a float64.
func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	default:
		return 0, false
	}
}

// TestNormalizeSQL verifies placeholder, quoting, literal preservation, and
// subquery alias normalization rules.
func TestNormalizeSQL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "placeholders_and_quoting",
			in:   `SELECT "fid_book"."id" FROM "fid_book" WHERE "fid_book"."pages" > $1 AND "fid_book"."title" LIKE $2 ESCAPE '\'`,
			want: `SELECT fid_book.id FROM fid_book WHERE fid_book.pages > ? AND fid_book.title LIKE ? ESCAPE '\'`,
		},
		{
			name: "preserves_string_literals",
			in:   `SELECT "fid_book"."id" FROM "fid_book" WHERE "fid_book"."title" = '%s and $1 and "quoted"'`,
			want: `SELECT fid_book.id FROM fid_book WHERE fid_book.title = '%s and $1 and "quoted"'`,
		},
		{
			name: "expands_subquery_aliases",
			in:   `DELETE FROM "fid_book" WHERE "fid_book"."id" IN (SELECT U0."id" FROM "fid_book" U0 INNER JOIN "fid_author" U1 ON (U0."author_id" = U1."id") WHERE U1."name" = %s)`,
			want: `DELETE FROM fid_book WHERE fid_book.id IN (SELECT fid_book.id FROM fid_book INNER JOIN fid_author ON (fid_book.author_id = fid_author.id) WHERE fid_author.name = ?)`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeSQL(tc.in); got != tc.want {
				t.Errorf("normalizeSQL mismatch\n  got:  %s\n  want: %s", got, tc.want)
			}
		})
	}
}
