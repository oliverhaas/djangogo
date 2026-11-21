package fidelity

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
)

// sqliteSchemaSnapshot holds the semantic SQLite schema extracted via PRAGMAs.
type sqliteSchemaSnapshot struct {
	Tables map[string]sqliteTableSchema `json:"tables"`
}

// sqliteTableSchema holds one table's columns, indexes, and foreign keys.
type sqliteTableSchema struct {
	Columns     []sqliteColumnSchema `json:"columns"`
	Indexes     []sqliteIndexSchema  `json:"indexes"`
	ForeignKeys []sqliteFKSchema     `json:"foreign_keys"`
}

// sqliteColumnSchema represents one row of PRAGMA table_info.
type sqliteColumnSchema struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"notnull"`
	PK      int    `json:"pk"`
}

// sqliteIndexSchema represents one index from PRAGMA index_list + index_xinfo.
type sqliteIndexSchema struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Origin  string   `json:"origin"`
	Columns []string `json:"columns"`
}

// sqliteFKSchema represents one row of PRAGMA foreign_key_list.
type sqliteFKSchema struct {
	Table    string `json:"table"`
	From     string `json:"from"`
	To       string `json:"to"`
	OnDelete string `json:"on_delete"`
}

// normalizeMigrationSQL canonicalizes DDL text before comparing Djan-Go-Go with
// Django.
//
// Documented normalization rules (all applied outside single-quoted SQL string
// literals):
//  1. Identifier quoting: simple double-quoted identifiers ("migfid_article")
//     are unquoted.
//  2. Case folding: SQL keywords, type names, and lowercase snake_case
//     identifiers are lowercased outside string literals so VARCHAR(100) and
//     varchar(100) compare equal.
//  3. Whitespace and trailing semicolon: whitespace runs collapse to a single
//     space, padding inside parentheses / before commas is removed, and
//     trailing semicolons are stripped.
func normalizeMigrationSQL(raw string) string {
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

	for i, seg := range parts {
		if len(seg) > 0 && seg[0] == '\'' {
			continue
		}
		seg = simpleIdentRe.ReplaceAllString(seg, "$1")
		seg = strings.ToLower(seg)
		seg = strings.Join(strings.Fields(seg), " ")
		parts[i] = seg
	}
	joined := strings.TrimSpace(strings.Join(parts, " "))
	joined = strings.TrimRight(joined, ";")
	joined = strings.ReplaceAll(joined, "( ", "(")
	joined = strings.ReplaceAll(joined, " )", ")")
	joined = strings.ReplaceAll(joined, " ,", ",")
	joined = strings.ReplaceAll(joined, " ;", ";")
	return strings.TrimSpace(joined)
}

// normalizeMigrationStatements filters comment-only lines and normalizes each
// SQL statement in stmts.
func normalizeMigrationStatements(stmts []string) []string {
	out := make([]string, 0, len(stmts))
	for _, s := range stmts {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		out = append(out, normalizeMigrationSQL(trimmed))
	}
	return out
}

// snapshotSQLiteSchema inspects db via PRAGMA table_info, index_list,
// index_xinfo, and foreign_key_list, skipping sqlite_* and django_migrations.
func snapshotSQLiteSchema(t *testing.T, sdb *sql.DB) sqliteSchemaSnapshot {
	t.Helper()
	ctx := context.Background()
	rows, err := sdb.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'django_migrations' ORDER BY name")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			t.Fatalf("scan table name: %v", err)
		}
		tableNames = append(tableNames, name)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}

	tables := make(map[string]sqliteTableSchema, len(tableNames))
	for _, table := range tableNames {
		tables[table] = sqliteTableSchema{
			Columns:     sqliteTableColumns(t, sdb, table),
			Indexes:     sqliteTableIndexes(t, sdb, table),
			ForeignKeys: sqliteTableForeignKeys(t, sdb, table),
		}
	}
	return sqliteSchemaSnapshot{Tables: tables}
}

// sqliteTableColumns returns PRAGMA table_info rows for table.
func sqliteTableColumns(t *testing.T, sdb *sql.DB, table string) []sqliteColumnSchema {
	t.Helper()
	rows, err := sdb.QueryContext(context.Background(), fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	var cols []sqliteColumnSchema
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		cols = append(cols, sqliteColumnSchema{
			Name:    name,
			Type:    colType,
			NotNull: notNull != 0,
			PK:      pk,
		})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(%s): %v", table, err)
	}
	return cols
}

// sqliteTableIndexes returns PRAGMA index_list + index_xinfo entries for table.
func sqliteTableIndexes(t *testing.T, sdb *sql.DB, table string) []sqliteIndexSchema {
	t.Helper()
	ctx := context.Background()
	rows, err := sdb.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_list("%s")`, table))
	if err != nil {
		t.Fatalf("PRAGMA index_list(%s): %v", table, err)
	}
	type rawIdx struct {
		name   string
		unique bool
		origin string
	}
	var rawList []rawIdx
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			_ = rows.Close()
			t.Fatalf("scan index_list(%s): %v", table, err)
		}
		rawList = append(rawList, rawIdx{name: name, unique: unique != 0, origin: origin})
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate index_list(%s): %v", table, err)
	}

	indexes := make([]sqliteIndexSchema, 0, len(rawList))
	for _, r := range rawList {
		xrows, err := sdb.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_xinfo("%s")`, r.name))
		if err != nil {
			t.Fatalf("PRAGMA index_xinfo(%s): %v", r.name, err)
		}
		var cols []string
		for xrows.Next() {
			var seqno, cid, desc, key int
			var name sql.NullString
			var coll string
			if err := xrows.Scan(&seqno, &cid, &name, &desc, &coll, &key); err != nil {
				_ = xrows.Close()
				t.Fatalf("scan index_xinfo(%s): %v", r.name, err)
			}
			if key == 1 && name.Valid {
				entry := name.String
				if desc == 1 {
					entry = "-" + entry
				}
				cols = append(cols, entry)
			}
		}
		_ = xrows.Close()
		if err := xrows.Err(); err != nil {
			t.Fatalf("iterate index_xinfo(%s): %v", r.name, err)
		}
		indexes = append(indexes, sqliteIndexSchema{
			Name:    r.name,
			Unique:  r.unique,
			Origin:  r.origin,
			Columns: cols,
		})
	}
	sort.Slice(indexes, func(i, j int) bool {
		if indexes[i].Unique != indexes[j].Unique {
			return !indexes[i].Unique
		}
		ci := strings.Join(indexes[i].Columns, ",")
		cj := strings.Join(indexes[j].Columns, ",")
		if ci != cj {
			return ci < cj
		}
		return indexes[i].Name < indexes[j].Name
	})
	return indexes
}

// sqliteTableForeignKeys returns PRAGMA foreign_key_list rows for table.
func sqliteTableForeignKeys(t *testing.T, sdb *sql.DB, table string) []sqliteFKSchema {
	t.Helper()
	rows, err := sdb.QueryContext(context.Background(), fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, table))
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_list(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	var fks []sqliteFKSchema
	for rows.Next() {
		var id, seq int
		var refTable, fromCol, toCol, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan foreign_key_list(%s): %v", table, err)
		}
		fks = append(fks, sqliteFKSchema{
			Table:    refTable,
			From:     fromCol,
			To:       toCol,
			OnDelete: onDelete,
		})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign_key_list(%s): %v", table, err)
	}
	sort.Slice(fks, func(i, j int) bool {
		if fks[i].From != fks[j].From {
			return fks[i].From < fks[j].From
		}
		if fks[i].Table != fks[j].Table {
			return fks[i].Table < fks[j].Table
		}
		return fks[i].To < fks[j].To
	})
	return fks
}

// normalizeSQLiteColType canonicalizes equivalent SQLite column type spellings
// (case folding and bool vs boolean) for semantic schema comparison.
func normalizeSQLiteColType(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "boolean" {
		return "bool"
	}
	return t
}

// compareSQLiteSchemas checks semantic equivalence between two SQLite schemas:
// identical tables, columns (by name, normalized type, nullability, PK),
// index coverage (unique flag and column list with sort direction), and
// foreign-key targets (ignoring on_delete action, which Django enforces in
// Python while Djan-Go-Go enforces via SQL ON DELETE).
func compareSQLiteSchemas(got, want sqliteSchemaSnapshot) []string {
	var diffs []string

	gotTables := sortedMapKeys(got.Tables)
	wantTables := sortedMapKeys(want.Tables)
	if !slices.Equal(gotTables, wantTables) {
		return []string{fmt.Sprintf("tables differ: got %v, want %v", gotTables, wantTables)}
	}

	for _, name := range wantTables {
		gt := got.Tables[name]
		wt := want.Tables[name]

		// Compare columns by name (SQLite rebuilds may reorder columns).
		gotCols := make(map[string]sqliteColumnSchema, len(gt.Columns))
		for _, c := range gt.Columns {
			gotCols[c.Name] = c
		}
		wantCols := make(map[string]sqliteColumnSchema, len(wt.Columns))
		for _, c := range wt.Columns {
			wantCols[c.Name] = c
		}
		if !slices.Equal(sortedMapKeys(gotCols), sortedMapKeys(wantCols)) {
			diffs = append(diffs, fmt.Sprintf("table %s columns differ: got %v, want %v",
				name, sortedMapKeys(gotCols), sortedMapKeys(wantCols)))
			continue
		}
		for _, colName := range sortedMapKeys(wantCols) {
			gc := gotCols[colName]
			wc := wantCols[colName]
			if normalizeSQLiteColType(gc.Type) != normalizeSQLiteColType(wc.Type) {
				diffs = append(diffs, fmt.Sprintf("table %s.%s type: got %q, want %q", name, colName, gc.Type, wc.Type))
			}
			// SQLite INTEGER PRIMARY KEY is implicitly NOT NULL even when NOT NULL
			// is omitted from the DDL, whereas PRAGMA table_info reports notnull=0.
			if gc.PK != wc.PK {
				diffs = append(diffs, fmt.Sprintf("table %s.%s pk: got %d, want %d", name, colName, gc.PK, wc.PK))
			} else if gc.PK == 0 && gc.NotNull != wc.NotNull {
				diffs = append(diffs, fmt.Sprintf("table %s.%s notnull: got %v, want %v", name, colName, gc.NotNull, wc.NotNull))
			}
		}

		// Compare index signatures (unique + ordered columns with direction).
		gotIdx := indexSignatures(gt.Indexes)
		wantIdx := indexSignatures(wt.Indexes)
		if !slices.Equal(gotIdx, wantIdx) {
			diffs = append(diffs, fmt.Sprintf("table %s indexes: got %v, want %v", name, gotIdx, wantIdx))
		}

		// Compare foreign keys (from -> table.to; on_delete differs by design).
		gotFK := fkSignatures(gt.ForeignKeys)
		wantFK := fkSignatures(wt.ForeignKeys)
		if !slices.Equal(gotFK, wantFK) {
			diffs = append(diffs, fmt.Sprintf("table %s foreign keys: got %v, want %v", name, gotFK, wantFK))
		}
	}
	return diffs
}

// indexSignatures formats indexes as sorted "unique:col1,-col2" strings.
func indexSignatures(indexes []sqliteIndexSchema) []string {
	out := make([]string, len(indexes))
	for i, idx := range indexes {
		kind := "idx"
		if idx.Unique {
			kind = "uniq"
		}
		out[i] = kind + ":" + strings.Join(idx.Columns, ",")
	}
	sort.Strings(out)
	return out
}

// fkSignatures formats foreign keys as sorted "from->table.to" strings.
func fkSignatures(fks []sqliteFKSchema) []string {
	out := make([]string, len(fks))
	for i, fk := range fks {
		out[i] = fmt.Sprintf("%s->%s.%s", fk.From, fk.Table, fk.To)
	}
	sort.Strings(out)
	return out
}

// sortedMapKeys returns the keys of m in sorted order.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestNormalizeMigrationSQL verifies quoting, case folding, literal
// preservation, and trailing semicolon handling.
func TestNormalizeMigrationSQL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "case_and_quoting",
			in:   `CREATE INDEX "migfid_art_title_rank_idx" ON "migfid_article" ("title", "rank" DESC);`,
			want: `create index migfid_art_title_rank_idx on migfid_article (title, rank desc)`,
		},
		{
			name: "preserves_string_literals",
			in:   `ALTER TABLE "migfid_article" ADD COLUMN "status" VARCHAR(20) NOT NULL DEFAULT 'Draft';`,
			want: `alter table migfid_article add column status varchar(20) not null default 'Draft'`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeMigrationSQL(tc.in); got != tc.want {
				t.Errorf("normalizeMigrationSQL mismatch\n  got:  %s\n  want: %s", got, tc.want)
			}
		})
	}
}
