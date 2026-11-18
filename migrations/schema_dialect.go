package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// remakeTable rebuilds a SQLite table, the only way to change or drop a
// column there: create the new table under a temporary name, copy the rows,
// drop the old table, rename the new one, and create the indexes again.
//
// exprs maps a column of the new table to the SQL expression that fills it. A
// column without an entry is copied from the old column of the same name, and
// left to its NULL when the old table has no such column. Foreign keys are
// written into the new table from the state, and those of other tables keep
// pointing to the table by name.
func (e *SchemaEditor) remakeTable(ctx context.Context, state *ProjectState, oldMS, newMS *ModelState, exprs map[string]string) error {
	tmp := "new__" + newMS.Table
	create, err := e.createTableSQL(state, tmp, newMS.Fields)
	if err != nil {
		return err
	}
	oldCols := map[string]bool{}
	for _, f := range oldMS.Fields {
		oldCols[f.Column] = true
	}
	var targets, sources []string
	for _, f := range newMS.Fields {
		switch expr, ok := exprs[f.Column]; {
		case ok:
			targets, sources = append(targets, e.q(f.Column)), append(sources, expr)
		case oldCols[f.Column]:
			targets, sources = append(targets, e.q(f.Column)), append(sources, e.q(f.Column))
		}
	}
	stmts := []string{create}
	if len(targets) > 0 {
		stmts = append(stmts, "INSERT INTO "+e.q(tmp)+" ("+strings.Join(targets, ", ")+") SELECT "+
			strings.Join(sources, ", ")+" FROM "+e.q(oldMS.Table))
	}
	stmts = append(stmts, "DROP TABLE "+e.q(oldMS.Table), "ALTER TABLE "+e.q(tmp)+" RENAME TO "+e.q(newMS.Table))
	indexes, err := e.indexStatements(newMS)
	if err != nil {
		return err
	}
	return e.executeAll(ctx, append(stmts, indexes...)...)
}

// constraintNames returns the names of the PostgreSQL constraints of the
// given type ('u' unique, 'f' foreign key) that cover exactly columns. The
// database is asked because a constraint written inline keeps the name it got
// when it was created, even after its table or column is renamed. In collect
// mode, and when the lookup finds nothing, the name PostgreSQL would give by
// default is returned.
func (e *SchemaEditor) constraintNames(ctx context.Context, table string, columns []string, contype string) ([]string, error) {
	suffix := map[string]string{"u": "key", "f": "fkey"}[contype]
	fallback := []string{table + "_" + strings.Join(columns, "_") + "_" + suffix}
	if e.collecting {
		return fallback, nil
	}
	const query = `SELECT c.conname FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
WHERE t.relname = $1 AND pg_table_is_visible(t.oid) AND c.contype::text = $2
AND (SELECT string_agg(a.attname::text, ',' ORDER BY k.ord)
     FROM unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord)
     JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum) = $3
ORDER BY c.conname`
	rows, err := e.run.QueryContext(ctx, query, table, contype, strings.Join(columns, ","))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return fallback, nil
	}
	return names, nil
}

// dropConstraints drops every constraint of the given type on the column.
func (e *SchemaEditor) dropConstraints(ctx context.Context, table, column, contype string) error {
	names, err := e.constraintNames(ctx, table, []string{column}, contype)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := e.Execute(ctx, "ALTER TABLE "+e.q(table)+" DROP CONSTRAINT "+e.q(name)); err != nil {
			return err
		}
	}
	return nil
}

// alterFieldPostgres alters a column in place with ALTER TABLE ... ALTER
// COLUMN, so the table is not rewritten more than PostgreSQL needs. The steps
// follow Django's schema editor: drop what depends on the old definition,
// rename, change the type, change NULL, then add what the new definition needs.
func (e *SchemaEditor) alterFieldPostgres(ctx context.Context, state *ProjectState, oldMS, newMS *ModelState, oldF, newF FieldState, oneOff any) error {
	if oldF.PrimaryKey != newF.PrimaryKey || (oldF.Kind == orm.KindAuto) != (newF.Kind == orm.KindAuto) {
		return errors.New("migrations: changing the primary key of a model is not supported on PostgreSQL")
	}
	table := newMS.Table
	alter := "ALTER TABLE " + e.q(table) + " "
	typeChanged := e.bareType(oldF) != e.bareType(newF)
	relChanged := typeChanged || oldF.IsRelation() != newF.IsRelation() || oldF.RelTo != newF.RelTo ||
		oldF.RelOnDelete != newF.RelOnDelete
	renamed := oldF.Column != newF.Column

	if oldF.IsRelation() && relChanged {
		if err := e.dropConstraints(ctx, table, oldF.Column, "f"); err != nil {
			return err
		}
	}
	if shouldIndex(oldF) && (!shouldIndex(newF) || renamed) {
		if err := e.Execute(ctx, "DROP INDEX "+e.q(e.fieldIndexName(oldMS.Table, oldF.Column))); err != nil {
			return err
		}
	}
	if oldF.Unique && !newF.Unique {
		if err := e.dropConstraints(ctx, table, oldF.Column, "u"); err != nil {
			return err
		}
	}
	col := e.q(newF.Column)
	if renamed {
		if err := e.Execute(ctx, alter+"RENAME COLUMN "+e.q(oldF.Column)+" TO "+col); err != nil {
			return err
		}
	}
	if typeChanged {
		typ := e.bareType(newF)
		if err := e.Execute(ctx, alter+"ALTER COLUMN "+col+" TYPE "+typ+" USING "+col+"::"+typ); err != nil {
			return err
		}
	}
	switch {
	case oldF.Null && !newF.Null:
		if def := effectiveDefault(newF, oneOff); def != nil {
			literal, err := quoteValue(e.dialect, def)
			if err != nil {
				return err
			}
			if err := e.Execute(ctx, "UPDATE "+e.q(table)+" SET "+col+" = "+literal+" WHERE "+col+" IS NULL"); err != nil {
				return err
			}
		}
		if err := e.Execute(ctx, alter+"ALTER COLUMN "+col+" SET NOT NULL"); err != nil {
			return err
		}
	case !oldF.Null && newF.Null:
		if err := e.Execute(ctx, alter+"ALTER COLUMN "+col+" DROP NOT NULL"); err != nil {
			return err
		}
	}
	if !oldF.Unique && newF.Unique {
		name := createIndexName(table, []string{newF.Column}, "_uniq", maxNameLength(e.dialect))
		if err := e.Execute(ctx, alter+"ADD CONSTRAINT "+e.q(name)+" UNIQUE ("+col+")"); err != nil {
			return err
		}
	}
	if shouldIndex(newF) && (!shouldIndex(oldF) || renamed) {
		if err := e.Execute(ctx, e.createIndexSQL(e.fieldIndexName(table, newF.Column), table, []string{newF.Column})); err != nil {
			return err
		}
	}
	if newF.IsRelation() && relChanged {
		if err := e.addForeignKey(ctx, state, table, newF); err != nil {
			return err
		}
	}
	if renamed {
		return e.renameUniqueTogether(ctx, oldMS, newMS)
	}
	return nil
}

// addForeignKey adds a named FOREIGN KEY constraint for f. The name follows
// Django: table, column, digest, and "_fk_<target table>_<target column>".
func (e *SchemaEditor) addForeignKey(ctx context.Context, state *ProjectState, table string, f FieldState) error {
	target, err := state.relatedTo(f)
	if err != nil {
		return err
	}
	pk := target.PrimaryKey()
	if pk == nil {
		return fmt.Errorf("migrations: model %s has no primary key to refer to", target.Label())
	}
	ref, err := e.referencesClause(state, f)
	if err != nil {
		return err
	}
	name := createIndexName(table, []string{f.Column}, "_fk_"+target.Table+"_"+pk.Column, maxNameLength(e.dialect))
	return e.Execute(ctx, "ALTER TABLE "+e.q(table)+" ADD CONSTRAINT "+e.q(name)+
		" FOREIGN KEY ("+e.q(f.Column)+") "+ref)
}

// renameUniqueTogether renames the unique-together constraints whose computed
// name changed with a column. The single-column indexes are handled by the
// caller, so only the trailing unique entries are compared.
func (e *SchemaEditor) renameUniqueTogether(ctx context.Context, oldMS, newMS *ModelState) error {
	if len(oldMS.UniqueTogether) != len(newMS.UniqueTogether) {
		return nil
	}
	for i := range oldMS.UniqueTogether {
		oldCols, err := oldMS.columns(oldMS.UniqueTogether[i])
		if err != nil {
			return err
		}
		newCols, err := newMS.columns(newMS.UniqueTogether[i])
		if err != nil {
			return err
		}
		oldName, newName := e.uniqueTogetherName(oldMS.Table, oldCols), e.uniqueTogetherName(newMS.Table, newCols)
		if oldName == newName {
			continue
		}
		stmt := "ALTER TABLE " + e.q(newMS.Table) + " RENAME CONSTRAINT " + e.q(oldName) + " TO " + e.q(newName)
		if err := e.Execute(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
