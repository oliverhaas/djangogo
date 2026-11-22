package fidelity

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// querySQLGoldenFile is the JSON structure written by oracle/query_sql.py.
type querySQLGoldenFile struct {
	DjangoVersion string               `json:"django_version"`
	Dialect       string               `json:"dialect"`
	Cases         []querySQLGoldenCase `json:"cases"`
}

// querySQLGoldenCase is one expected SQL/params/result entry for a dialect.
type querySQLGoldenCase struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	SQL     string `json:"sql"`
	Params  []any  `json:"params"`
	Result  any    `json:"result"`
	NoQuery bool   `json:"no_query"`
}

// dialectCaseKey identifies a SQL divergence entry scoped to a specific dialect
// ("sqlite" or "postgres") or to both dialects ("*").
type dialectCaseKey struct {
	dialect string
	name    string
}

// querySQLDivergences is the reviewed allowlist of known SQL/param differences
// between Djan-Go-Go and Django 6.0.3 after normalization. Every entry must
// still pass semantic equivalence on the fixture dataset, and the test fails if
// an allowlisted entry begins matching Django so stale excuses are removed.
// Full details live in fidelity/query_sql_divergences.md.
var querySQLDivergences = map[dialectCaseKey]string{
	// Boolean exact shorthand: Django 6.0 compiles BooleanField exact filters to
	// bare "col" / "NOT col" with no parameter; Djan-Go-Go binds "col = ?".
	{dialect: "*", name: "filter_exact_bool"}:       "Django uses bare boolean column test with no parameter; Djan-Go-Go emits col = ? with true",
	{dialect: "*", name: "filter_exact_bool_false"}: "Django uses bare NOT col with no parameter; Djan-Go-Go emits col = ? with false",

	// Random ordering: Django appends ASC and uses RAND() on SQLite; Djan-Go-Go
	// emits ORDER BY RANDOM() on both dialects.
	{dialect: "*", name: "order_by_random"}: "Django emits ORDER BY RAND() ASC (SQLite) / RANDOM() ASC (PostgreSQL); Djan-Go-Go emits ORDER BY RANDOM()",

	// Subquery COUNT(*) aliases: Django omits outer __count and aliases the inner
	// DISTINCT column; Djan-Go-Go aliases outer COUNT(*) AS __count and omits the
	// redundant inner column alias.
	{dialect: "*", name: "count_distinct"}: "Django omits outer __count alias and aliases inner DISTINCT column; Djan-Go-Go aliases outer __count only",

	// EXISTS shape: Django selects a literal 1 with LIMIT 1; Djan-Go-Go wraps the
	// query in SELECT EXISTS(SELECT 1 ...).
	{dialect: "*", name: "exists_all"}:      "Django emits SELECT 1 AS a ... LIMIT 1; Djan-Go-Go emits SELECT EXISTS(SELECT 1 ...)",
	{dialect: "*", name: "exists_filtered"}: "Django emits SELECT 1 AS a ... LIMIT 1; Djan-Go-Go emits SELECT EXISTS(SELECT 1 ...)",
	{dialect: "*", name: "exists_none"}:     "Django emits SELECT 1 AS a ... LIMIT 1; Djan-Go-Go emits SELECT EXISTS(SELECT 1 ...)",

	// Values / ValuesList column aliases and Django 6.0 positional GROUP BY 1 /
	// ORDER BY 1 references: Djan-Go-Go omits redundant plain-column aliases and
	// writes explicit column references in GROUP BY and ORDER BY.
	{dialect: "*", name: "values_fields"}:            "Django aliases selected values() columns and uses positional ORDER BY 1; Djan-Go-Go uses explicit columns",
	{dialect: "*", name: "values_list"}:              "Django aliases selected values_list() columns; Djan-Go-Go omits redundant column aliases",
	{dialect: "*", name: "values_list_flat"}:         "Django aliases selected values_list(flat=True) column; Djan-Go-Go omits redundant column alias",
	{dialect: "*", name: "distinct_values"}:          "Django aliases selected column and uses positional ORDER BY 1; Djan-Go-Go uses explicit column",
	{dialect: "*", name: "distinct_values_relation"}: "Django aliases selected relation column and uses positional ORDER BY 1; Djan-Go-Go uses explicit column",
	{dialect: "*", name: "distinct_extra_order"}:     "Django aliases selected values() column; Djan-Go-Go omits redundant column alias",
	{dialect: "*", name: "annotate_values_groupby"}:  "Django aliases selected column and uses positional GROUP BY 1 / ORDER BY 1; Djan-Go-Go uses explicit columns",
	{dialect: "*", name: "annotate_having"}:          "Django aliases selected column and uses positional GROUP BY 1 / ORDER BY 1; Djan-Go-Go uses explicit columns",
	{dialect: "*", name: "annotate_expr"}:            "Django aliases selected column (and omits ::bigint cast on PostgreSQL); Djan-Go-Go omits redundant alias and casts PG literals",
	{dialect: "*", name: "values_transform_date"}:    "Django aliases selected values() columns and uses positional ORDER BY 1; Djan-Go-Go uses explicit columns",

	// PostgreSQL iexact: Django uses UPPER(col::text) = UPPER(%s); Djan-Go-Go
	// routes iexact through LIKE ... ESCAPE '\'.
	{dialect: "postgres", name: "filter_iexact"}: "Django PostgreSQL uses UPPER(col::text) = UPPER(%s); Djan-Go-Go emits UPPER(col::text) LIKE UPPER($1) ESCAPE '\\'",

	// PostgreSQL LIKE ESCAPE: Django relies on PostgreSQL's default backslash
	// escape and omits ESCAPE '\'; Djan-Go-Go emits ESCAPE '\' explicitly.
	{dialect: "postgres", name: "filter_contains"}:    "Django PostgreSQL omits default ESCAPE '\\' on LIKE; Djan-Go-Go emits ESCAPE '\\' explicitly",
	{dialect: "postgres", name: "filter_icontains"}:   "Django PostgreSQL omits default ESCAPE '\\' on LIKE; Djan-Go-Go emits ESCAPE '\\' explicitly",
	{dialect: "postgres", name: "filter_startswith"}:  "Django PostgreSQL omits default ESCAPE '\\' on LIKE; Djan-Go-Go emits ESCAPE '\\' explicitly",
	{dialect: "postgres", name: "filter_istartswith"}: "Django PostgreSQL omits default ESCAPE '\\' on LIKE; Djan-Go-Go emits ESCAPE '\\' explicitly",
	{dialect: "postgres", name: "filter_endswith"}:    "Django PostgreSQL omits default ESCAPE '\\' on LIKE; Djan-Go-Go emits ESCAPE '\\' explicitly",
	{dialect: "postgres", name: "filter_iendswith"}:   "Django PostgreSQL omits default ESCAPE '\\' on LIKE; Djan-Go-Go emits ESCAPE '\\' explicitly",

	// PostgreSQL expression literal cast: Djan-Go-Go appends ::bigint via
	// LiteralSQL so untyped parameter contexts resolve; Django binds bare %s.
	{dialect: "postgres", name: "filter_f_expr"}:        "Djan-Go-Go PostgreSQL casts expression literals ($1::bigint); Django binds bare %s",
	{dialect: "postgres", name: "update_relation_expr"}: "Djan-Go-Go PostgreSQL casts expression literals ($1::bigint); Django binds bare %s",
}

// postgresNullLastOverrides records the expected row order on PostgreSQL for
// ORDER BY clauses on nullable joined columns, where SQLite defaults to NULLS
// FIRST and PostgreSQL defaults to NULLS LAST for the identical SQL text.
var postgresNullLastOverrides = map[string][]int64{
	"order_by_nullable_relation": {3, 1, 5, 2, 4},
	"order_by_chain":             {1, 4, 2, 3, 5},
}

// loadQuerySQLGolden reads and decodes golden/query_sql/<dialect>.json.
func loadQuerySQLGolden(t *testing.T, dialect string) map[string]querySQLGoldenCase {
	t.Helper()
	path := filepath.Join("golden", "query_sql", dialect+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var file querySQLGoldenFile
	if err := dec.Decode(&file); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	byName := make(map[string]querySQLGoldenCase, len(file.Cases))
	for _, c := range file.Cases {
		byName[c.Name] = c
	}
	return byName
}

// divergenceReason returns the allowlisted SQL divergence reason for (dialect, name), if any.
func divergenceReason(dialect, name string) (string, bool) {
	if reason, ok := querySQLDivergences[dialectCaseKey{dialect: dialect, name: name}]; ok {
		return reason, true
	}
	if reason, ok := querySQLDivergences[dialectCaseKey{dialect: "*", name: name}]; ok {
		return reason, true
	}
	return "", false
}

// TestQuerySQLFidelity runs every canonical ORM query case on SQLite and
// PostgreSQL, comparing normalized SQL and bound parameters against Django's
// checked-in goldens and asserting semantic equivalence against Django's
// executed fixture results.
func TestQuerySQLFidelity(t *testing.T) {
	sqliteGolden := loadQuerySQLGolden(t, "sqlite")
	postgresGolden := loadQuerySQLGolden(t, "postgres")
	cases := canonicalQueryCases()

	djangotest.EachDB(t, queryFixtureModels(), func(t *testing.T, db *orm.DB) {
		seedQueryFixture(t, db)
		dialect := db.Dialect().Name()
		goldenByName := sqliteGolden
		if dialect == "postgres" {
			goldenByName = postgresGolden
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				want, ok := goldenByName[tc.name]
				if !ok {
					t.Fatalf("case %q missing from golden/query_sql/%s.json", tc.name, dialect)
				}
				wantResult := sqliteGolden[tc.name].Result
				if dialect == "postgres" {
					if pgOrder, hasOverride := postgresNullLastOverrides[tc.name]; hasOverride {
						wantResult = pgOrder
					}
				}

				outcome := tc.run(t, db)

				// Step 1: Always verify semantic equivalence against Django's result.
				gotRes := normalizeResult(outcome.result)
				expRes := normalizeResult(wantResult)
				if !resultsEqual(gotRes, expRes) {
					t.Errorf("semantic mismatch on %s for %q\n  got:  %#v\n  want: %#v",
						dialect, tc.name, gotRes, expRes)
				}

				// Step 2: Compare captured SQL and bound parameters against Django.
				if want.NoQuery {
					if len(outcome.queries) != 0 {
						t.Errorf("expected no query for %q, got %d: %v",
							tc.name, len(outcome.queries), outcome.queries)
					}
					return
				}
				if len(outcome.queries) != 1 {
					t.Fatalf("expected 1 query for %q, captured %d: %+v",
						tc.name, len(outcome.queries), outcome.queries)
				}
				gotSQL := normalizeSQL(outcome.queries[0].SQL)
				wantSQL := normalizeSQL(want.SQL)
				gotParams := normalizeParams(outcome.queries[0].Args)
				wantParams := normalizeParams(want.Params)

				sqlMatch := gotSQL == wantSQL
				paramsMatch := reflect.DeepEqual(gotParams, wantParams)

				if reason, allowlisted := divergenceReason(dialect, tc.name); allowlisted {
					if sqlMatch && paramsMatch {
						t.Errorf("allowlisted divergence %q on %s now matches Django; remove from querySQLDivergences (reason was: %s)",
							tc.name, dialect, reason)
					}
					return
				}

				if !sqlMatch {
					t.Errorf("SQL mismatch on %s for %q\n  got:  %s\n  want: %s\n  raw got:  %s\n  raw want: %s",
						dialect, tc.name, gotSQL, wantSQL, outcome.queries[0].SQL, want.SQL)
				}
				if !paramsMatch {
					t.Errorf("params mismatch on %s for %q\n  got:  %#v\n  want: %#v",
						dialect, tc.name, gotParams, wantParams)
				}
			})
		}
	})
}

// TestQuerySQLCasesAndAllowlistConsistent ensures Go cases, both golden files,
// and the divergence allowlist stay in sync with no orphaned entries.
func TestQuerySQLCasesAndAllowlistConsistent(t *testing.T) {
	sqliteGolden := loadQuerySQLGolden(t, "sqlite")
	postgresGolden := loadQuerySQLGolden(t, "postgres")
	cases := canonicalQueryCases()

	goNames := make(map[string]bool, len(cases))
	for _, c := range cases {
		if goNames[c.name] {
			t.Errorf("duplicate Go query case %q", c.name)
		}
		goNames[c.name] = true
		if _, ok := sqliteGolden[c.name]; !ok {
			t.Errorf("Go case %q missing from golden/query_sql/sqlite.json", c.name)
		}
		if _, ok := postgresGolden[c.name]; !ok {
			t.Errorf("Go case %q missing from golden/query_sql/postgres.json", c.name)
		}
	}
	for name := range sqliteGolden {
		if !goNames[name] {
			t.Errorf("sqlite golden case %q has no Go test case", name)
		}
	}
	for name := range postgresGolden {
		if !goNames[name] {
			t.Errorf("postgres golden case %q has no Go test case", name)
		}
	}
	for key := range querySQLDivergences {
		if key.dialect != "*" && key.dialect != "sqlite" && key.dialect != "postgres" {
			t.Errorf("querySQLDivergences has unknown dialect %q for %q", key.dialect, key.name)
		}
		if !goNames[key.name] {
			t.Errorf("querySQLDivergences names %q, which is not a query case", key.name)
		}
	}
}
