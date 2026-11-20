package orm

import (
	"context"
	"fmt"
	"strings"
)

// createTableExtras executes the post-CREATE TABLE statements for m: field
// indexes (db_index and foreign keys), model-level Meta.Indexes, and
// Meta.UniqueTogether constraints.
func (db *DB) createTableExtras(ctx context.Context, m *Model) error {
	d := db.dialect
	maxLen := maxNameLength(d)
	table := m.Table()

	for _, f := range m.fields {
		if (f.DBIndex || f.Rel != nil) && !f.Unique && !f.PrimaryKey {
			name := CreateIndexName(table, []string{f.Column}, "", maxLen)
			stmt := "CREATE INDEX " + d.Quote(name) + " ON " + d.Quote(table) + " (" + d.Quote(f.Column) + ")"
			if _, err := db.conn(ctx).ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("orm: create index %s on %s: %w", name, table, err)
			}
		}
	}

	for _, idx := range m.indexes {
		colExprs := make([]string, len(idx.Fields))
		for i, raw := range idx.Fields {
			desc := strings.HasPrefix(raw, "-")
			bare := strings.TrimPrefix(raw, "-")
			f, ok := m.byName[bare]
			if !ok {
				return fmt.Errorf("orm: create index %s on %s: unknown field %q", idx.Name, table, raw)
			}
			if desc {
				colExprs[i] = d.Quote(f.Column) + " DESC"
			} else {
				colExprs[i] = d.Quote(f.Column)
			}
		}
		stmt := "CREATE INDEX " + d.Quote(idx.Name) + " ON " + d.Quote(table) + " (" + strings.Join(colExprs, ", ") + ")"
		if _, err := db.conn(ctx).ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("orm: create index %s on %s: %w", idx.Name, table, err)
		}
	}

	for _, set := range m.uniqueTogether {
		cols := make([]string, len(set))
		quoted := make([]string, len(set))
		for i, name := range set {
			f, ok := m.byName[name]
			if !ok {
				return fmt.Errorf("orm: create unique_together on %s: unknown field %q", table, name)
			}
			cols[i] = f.Column
			quoted[i] = d.Quote(f.Column)
		}
		cname := CreateIndexName(table, cols, "_uniq", maxLen)
		var stmt string
		if d.Name() == "sqlite" {
			stmt = "CREATE UNIQUE INDEX " + d.Quote(cname) + " ON " + d.Quote(table) + " (" + strings.Join(quoted, ", ") + ")"
		} else {
			stmt = "ALTER TABLE " + d.Quote(table) + " ADD CONSTRAINT " + d.Quote(cname) + " UNIQUE (" + strings.Join(quoted, ", ") + ")"
		}
		if _, err := db.conn(ctx).ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("orm: create unique_together %s on %s: %w", cname, table, err)
		}
	}

	return nil
}
