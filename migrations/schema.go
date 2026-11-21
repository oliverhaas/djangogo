package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// sqlRunner is the part of *sql.Tx the schema editor uses.
type sqlRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SchemaEditor turns schema changes into DDL for one dialect and executes it
// inside the transaction of the running migration, the counterpart of
// Django's schema editor. In collect mode it records the statements instead
// of executing them, which is how sqlmigrate works.
//
// Identifiers are always quoted by the dialect. Values (defaults) are written
// as escaped literals, because DDL statements take no bind parameters.
type SchemaEditor struct {
	dialect    orm.Dialect
	run        sqlRunner
	base       *orm.DB
	txDB       *orm.DB
	collecting bool
	collected  []string
}

// Dialect returns the dialect the editor writes SQL for.
func (e *SchemaEditor) Dialect() orm.Dialect { return e.dialect }

// Collecting reports whether the editor records SQL instead of executing it.
func (e *SchemaEditor) Collecting() bool { return e.collecting }

// CollectedSQL returns the statements and comments recorded in collect mode.
func (e *SchemaEditor) CollectedSQL() []string { return e.collected }

// Execute runs one statement in the migration transaction, or records it in
// collect mode. Custom operations use it for their own SQL.
func (e *SchemaEditor) Execute(ctx context.Context, query string, args ...any) error {
	if e.collecting {
		stmt := strings.TrimRight(strings.TrimSpace(query), ";")
		e.collected = append(e.collected, stmt+";")
		return nil
	}
	if _, err := e.run.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("%w (statement: %s)", err, query)
	}
	return nil
}

// DB returns a database handle bound to the migration transaction, for data
// migrations. It fails in collect mode, where no transaction exists.
func (e *SchemaEditor) DB() (*orm.DB, error) {
	if e.collecting || e.base == nil {
		return nil, errors.New("migrations: no database is available while collecting SQL")
	}
	if e.txDB == nil {
		e.txDB = newTxBoundDB(e.base, e.run)
	}
	return e.txDB, nil
}

// close releases the transaction-bound handle, if one was made.
func (e *SchemaEditor) close() {
	if e.txDB != nil {
		_ = e.txDB.SQL().Close()
		e.txDB = nil
	}
}

// comment records comment lines in collect mode.
func (e *SchemaEditor) comment(lines ...string) {
	e.collected = append(e.collected, lines...)
}

// q quotes an identifier.
func (e *SchemaEditor) q(ident string) string { return e.dialect.Quote(ident) }

// qAll quotes identifiers and joins them with commas.
func (e *SchemaEditor) qAll(idents []string) string {
	quoted := make([]string, len(idents))
	for i, id := range idents {
		quoted[i] = e.q(id)
	}
	return strings.Join(quoted, ", ")
}

// isSQLite reports whether the editor targets SQLite.
func (e *SchemaEditor) isSQLite() bool { return e.dialect.Name() == "sqlite" }

// toOrmField rebuilds an orm field from a field state, so the dialect can
// render its column type.
func toOrmField(f FieldState) *orm.Field {
	return &orm.Field{
		Name: f.Name, Column: f.Column, Kind: f.Kind, PrimaryKey: f.PrimaryKey,
		Null: f.Null, Unique: f.Unique, MaxLength: f.MaxLength,
		MaxDigits: f.MaxDigits, DecimalPlaces: f.DecimalPlaces,
	}
}

// columnDef returns the quoted column name with its full type definition.
func (e *SchemaEditor) columnDef(f FieldState) string {
	return e.q(f.Column) + " " + e.dialect.ColumnType(toOrmField(f))
}

// bareType returns the column type without NULL, UNIQUE, PRIMARY KEY, or CHECK.
func (e *SchemaEditor) bareType(f FieldState) string {
	of := toOrmField(f)
	of.Column = ""
	of.Null, of.Unique, of.PrimaryKey = true, false, false
	return e.dialect.ColumnType(of)
}

// referencesClause returns `REFERENCES "table" ("pk") ON DELETE ...` for a
// foreign-key field.
func (e *SchemaEditor) referencesClause(state *ProjectState, f FieldState) (string, error) {
	target, err := state.relatedTo(f)
	if err != nil {
		return "", err
	}
	pk := target.PrimaryKey()
	if pk == nil {
		return "", fmt.Errorf("migrations: model %s has no primary key to refer to", target.Label())
	}
	return "REFERENCES " + e.q(target.Table) + " (" + e.q(pk.Column) + ")" + f.RelOnDelete.Clause(), nil
}

// createTableSQL renders CREATE TABLE for the fields, with a FOREIGN KEY
// constraint per relation. It is the statement orm.DB.CreateTable issues, so a
// migrated schema equals a directly created one.
func (e *SchemaEditor) createTableSQL(state *ProjectState, table string, fields []FieldState) (string, error) {
	defs := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.IsManyToMany() {
			continue
		}
		defs = append(defs, e.columnDef(f))
	}
	for _, f := range fields {
		if !f.IsRelation() || f.IsManyToMany() {
			continue
		}
		ref, err := e.referencesClause(state, f)
		if err != nil {
			return "", err
		}
		defs = append(defs, "FOREIGN KEY ("+e.q(f.Column)+") "+ref)
	}
	return "CREATE TABLE " + e.q(table) + " (" + strings.Join(defs, ", ") + ")", nil
}

// shouldIndex reports whether a field gets its own index. Foreign keys are
// indexed by default, as in Django. A unique, primary-key, or many-to-many
// field does not get a single-column index on the declaring table.
func shouldIndex(f FieldState) bool {
	if f.IsManyToMany() {
		return false
	}
	return (f.DBIndex || f.IsRelation()) && !f.Unique && !f.PrimaryKey
}

// fieldIndexName returns the name of the single-column index of a field.
func (e *SchemaEditor) fieldIndexName(table, column string) string {
	return createIndexName(table, []string{column}, "", maxNameLength(e.dialect))
}

// uniqueTogetherName returns the name of a unique-together constraint.
func (e *SchemaEditor) uniqueTogetherName(table string, columns []string) string {
	return createIndexName(table, columns, "_uniq", maxNameLength(e.dialect))
}

// createIndexSQL renders CREATE INDEX.
func (e *SchemaEditor) createIndexSQL(name, table string, columns []string) string {
	return "CREATE INDEX " + e.q(name) + " ON " + e.q(table) + " (" + e.qAll(columns) + ")"
}

// createOrderedIndexSQL renders CREATE INDEX with optional DESC per column.
func (e *SchemaEditor) createOrderedIndexSQL(name, table string, cols []indexCol) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		if c.desc {
			parts[i] = e.q(c.column) + " DESC"
		} else {
			parts[i] = e.q(c.column)
		}
	}
	return "CREATE INDEX " + e.q(name) + " ON " + e.q(table) + " (" + strings.Join(parts, ", ") + ")"
}

// addUniqueSQL renders the statement that makes columns unique together: a
// unique index on SQLite, a table constraint on PostgreSQL, as in Django.
func (e *SchemaEditor) addUniqueSQL(name, table string, columns []string) string {
	if e.isSQLite() {
		return "CREATE UNIQUE INDEX " + e.q(name) + " ON " + e.q(table) + " (" + e.qAll(columns) + ")"
	}
	return "ALTER TABLE " + e.q(table) + " ADD CONSTRAINT " + e.q(name) + " UNIQUE (" + e.qAll(columns) + ")"
}

// dropUniqueSQL renders the statement that drops a unique-together constraint.
func (e *SchemaEditor) dropUniqueSQL(name, table string) string {
	if e.isSQLite() {
		return "DROP INDEX " + e.q(name)
	}
	return "ALTER TABLE " + e.q(table) + " DROP CONSTRAINT " + e.q(name)
}

// indexStatements returns the statements that create every index and
// unique-together constraint of a model.
func (e *SchemaEditor) indexStatements(ms *ModelState) ([]string, error) {
	var out []string
	for _, f := range ms.Fields {
		if shouldIndex(f) {
			out = append(out, e.createIndexSQL(e.fieldIndexName(ms.Table, f.Column), ms.Table, []string{f.Column}))
		}
	}
	for _, idx := range ms.Indexes {
		cols, err := ms.indexColumns(idx.Fields)
		if err != nil {
			return nil, err
		}
		out = append(out, e.createOrderedIndexSQL(idx.Name, ms.Table, cols))
	}
	for _, set := range ms.UniqueTogether {
		cols, err := ms.columns(set)
		if err != nil {
			return nil, err
		}
		out = append(out, e.addUniqueSQL(e.uniqueTogetherName(ms.Table, cols), ms.Table, cols))
	}
	return out, nil
}

// executeAll runs the statements in order.
func (e *SchemaEditor) executeAll(ctx context.Context, stmts ...string) error {
	for _, s := range stmts {
		if err := e.Execute(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// createModel creates the table of ms with its indexes and any auto-created
// M2M join tables declared on ms.
func (e *SchemaEditor) createModel(ctx context.Context, state *ProjectState, ms *ModelState) error {
	create, err := e.createTableSQL(state, ms.Table, ms.Fields)
	if err != nil {
		return err
	}
	indexes, err := e.indexStatements(ms)
	if err != nil {
		return err
	}
	if err := e.executeAll(ctx, append([]string{create}, indexes...)...); err != nil {
		return err
	}
	return e.createAutoThroughTables(ctx, state, ms)
}

// deleteTable drops table. PostgreSQL needs CASCADE to drop the foreign keys
// of other tables that point to it.
func (e *SchemaEditor) deleteTable(ctx context.Context, table string) error {
	stmt := "DROP TABLE " + e.q(table)
	if !e.isSQLite() {
		stmt += " CASCADE"
	}
	return e.Execute(ctx, stmt)
}

// deleteModel drops any auto-created M2M join tables of ms and then drops ms's table.
func (e *SchemaEditor) deleteModel(ctx context.Context, state *ProjectState, ms *ModelState) error {
	if err := e.dropAutoThroughTables(ctx, state, ms); err != nil {
		return err
	}
	return e.deleteTable(ctx, ms.Table)
}

// addField adds the column of f to the table of newMS (or creates its
// auto-created M2M join table). oldMS is the model without the field.
func (e *SchemaEditor) addField(ctx context.Context, state *ProjectState, oldMS, newMS *ModelState, f FieldState, oneOff any) error {
	if f.IsManyToMany() {
		if !f.HasAutoThrough() {
			return nil
		}
		th, err := autoThroughModelState(state, newMS, f)
		if err != nil {
			return err
		}
		return e.createModel(ctx, state, th)
	}
	def := effectiveDefault(f, oneOff)
	if !f.Null && def == nil && !e.collecting {
		hasRows, err := e.tableHasRows(ctx, oldMS.Table)
		if err != nil {
			return err
		}
		if hasRows {
			return fmt.Errorf("migrations: cannot add the non-nullable field %q to %s: the table has rows and the "+
				"field has no default to fill them with. Give the field a default (default=...), make it nullable "+
				"(null), or set AddField.Default to a one-off value for the existing rows",
				f.Name, newMS.Label())
		}
	}
	literal, err := quoteValue(e.dialect, def)
	if err != nil {
		return err
	}
	ref := ""
	if f.IsRelation() {
		clause, err := e.referencesClause(state, f)
		if err != nil {
			return err
		}
		ref = " " + clause
	}
	table := e.q(newMS.Table)
	if e.isSQLite() {
		// SQLite can add a column in place only when it needs no backfill and
		// no constraint; everything else goes through a rebuild.
		if !f.Null || def != nil || f.Unique || f.PrimaryKey {
			exprs := map[string]string{}
			if def != nil {
				exprs[f.Column] = literal
			}
			return e.remakeTable(ctx, state, oldMS, newMS, exprs)
		}
		if err := e.Execute(ctx, "ALTER TABLE "+table+" ADD COLUMN "+e.columnDef(f)+ref); err != nil {
			return err
		}
	} else {
		stmt := "ALTER TABLE " + table + " ADD COLUMN " + e.columnDef(f)
		if def != nil {
			stmt += " DEFAULT " + literal
		}
		if err := e.Execute(ctx, stmt+ref); err != nil {
			return err
		}
		if def != nil {
			// The default only fills the existing rows; the ORM sets defaults
			// itself, so the column keeps none.
			if err := e.Execute(ctx, "ALTER TABLE "+table+" ALTER COLUMN "+e.q(f.Column)+" DROP DEFAULT"); err != nil {
				return err
			}
		}
	}
	if shouldIndex(f) {
		return e.Execute(ctx, e.createIndexSQL(e.fieldIndexName(newMS.Table, f.Column), newMS.Table, []string{f.Column}))
	}
	return nil
}

// removeField drops the column of f (or its auto-created M2M join table).
// newMS is the model without the field.
func (e *SchemaEditor) removeField(ctx context.Context, from, to *ProjectState, oldMS, newMS *ModelState, f FieldState) error {
	if f.IsManyToMany() {
		if !f.HasAutoThrough() {
			return nil
		}
		th, err := autoThroughModelState(from, oldMS, f)
		if err != nil {
			return err
		}
		return e.deleteTable(ctx, th.Table)
	}
	if e.isSQLite() {
		return e.remakeTable(ctx, to, oldMS, newMS, nil)
	}
	return e.Execute(ctx, "ALTER TABLE "+e.q(oldMS.Table)+" DROP COLUMN "+e.q(f.Column))
}

// schemaEqual reports whether two fields produce the same column apart from
// its name. Options that live only in Go (default, blank, auto_now) are ignored.
func schemaEqual(a, b FieldState) bool {
	return a.Kind == b.Kind && a.PrimaryKey == b.PrimaryKey && a.Null == b.Null && a.Unique == b.Unique &&
		a.MaxLength == b.MaxLength && a.MaxDigits == b.MaxDigits && a.DecimalPlaces == b.DecimalPlaces &&
		shouldIndex(a) == shouldIndex(b) && a.IsRelation() == b.IsRelation() &&
		a.RelTo == b.RelTo && a.RelOnDelete == b.RelOnDelete
}

// alterField changes the column of oldF into the column of newF, or updates
// the auto-created join table for a many-to-many field.
func (e *SchemaEditor) alterField(ctx context.Context, from, to *ProjectState, oldMS, newMS *ModelState, oldF, newF FieldState, oneOff any) error {
	if oldF.IsManyToMany() || newF.IsManyToMany() {
		return e.alterM2MField(ctx, from, to, oldMS, newMS, oldF, newF)
	}
	same := schemaEqual(oldF, newF)
	switch {
	case same && oldF.Column == newF.Column:
		return nil
	case same:
		stmt := "ALTER TABLE " + e.q(newMS.Table) + " RENAME COLUMN " + e.q(oldF.Column) + " TO " + e.q(newF.Column)
		if err := e.Execute(ctx, stmt); err != nil {
			return err
		}
		return e.renameDerived(ctx, oldMS, newMS)
	case e.isSQLite():
		exprs := map[string]string{newF.Column: e.q(oldF.Column)}
		if oldF.Null && !newF.Null {
			if def := effectiveDefault(newF, oneOff); def != nil {
				literal, err := quoteValue(e.dialect, def)
				if err != nil {
					return err
				}
				exprs[newF.Column] = "coalesce(" + e.q(oldF.Column) + ", " + literal + ")"
			}
		}
		return e.remakeTable(ctx, to, oldMS, newMS, exprs)
	default:
		return e.alterFieldPostgres(ctx, to, oldMS, newMS, oldF, newF, oneOff)
	}
}

// derivedName is one index or constraint whose name is computed from the
// table and its columns.
type derivedName struct {
	name    string
	columns []string
	unique  bool
}

// derivedNames lists the computed names of a model in a fixed order: one per
// concrete field (empty when the field has no index), then one per
// unique-together set.
func (e *SchemaEditor) derivedNames(ms *ModelState) ([]derivedName, error) {
	out := make([]derivedName, 0, len(ms.Fields)+len(ms.UniqueTogether))
	for _, f := range ms.Fields {
		if f.IsManyToMany() {
			continue
		}
		d := derivedName{columns: []string{f.Column}}
		if shouldIndex(f) {
			d.name = e.fieldIndexName(ms.Table, f.Column)
		}
		out = append(out, d)
	}
	for _, set := range ms.UniqueTogether {
		cols, err := ms.columns(set)
		if err != nil {
			return nil, err
		}
		out = append(out, derivedName{name: e.uniqueTogetherName(ms.Table, cols), columns: cols, unique: true})
	}
	return out, nil
}

// renameDerived renames the indexes and constraints whose computed name
// changes when a table or column is renamed. Keeping the names in step with
// the state lets later operations find them without introspection.
func (e *SchemaEditor) renameDerived(ctx context.Context, oldMS, newMS *ModelState) error {
	oldNames, err := e.derivedNames(oldMS)
	if err != nil {
		return err
	}
	newNames, err := e.derivedNames(newMS)
	if err != nil {
		return err
	}
	if len(oldNames) != len(newNames) {
		return nil
	}
	for i, o := range oldNames {
		n := newNames[i]
		if o.name == "" || n.name == "" || o.name == n.name {
			continue
		}
		var stmts []string
		switch {
		case e.isSQLite() && n.unique:
			stmts = []string{"DROP INDEX " + e.q(o.name), e.addUniqueSQL(n.name, newMS.Table, n.columns)}
		case e.isSQLite():
			stmts = []string{"DROP INDEX " + e.q(o.name), e.createIndexSQL(n.name, newMS.Table, n.columns)}
		case n.unique:
			stmts = []string{"ALTER TABLE " + e.q(newMS.Table) + " RENAME CONSTRAINT " + e.q(o.name) + " TO " + e.q(n.name)}
		default:
			stmts = []string{"ALTER INDEX " + e.q(o.name) + " RENAME TO " + e.q(n.name)}
		}
		if err := e.executeAll(ctx, stmts...); err != nil {
			return err
		}
	}
	return nil
}

// alterTable renames the table of a model when its name differs between the
// two states, and updates any auto-created M2M join tables affected by a table
// or model rename.
func (e *SchemaEditor) alterTable(ctx context.Context, from, to *ProjectState, oldMS, newMS *ModelState) error {
	if oldMS.Table != newMS.Table {
		if err := e.Execute(ctx, "ALTER TABLE "+e.q(oldMS.Table)+" RENAME TO "+e.q(newMS.Table)); err != nil {
			return err
		}
		if err := e.renameDerived(ctx, oldMS, newMS); err != nil {
			return err
		}
	}
	return e.alterTableM2MTables(ctx, from, to, oldMS, newMS)
}

// alterUniqueTogether drops the constraints of the sets only oldMS has and
// creates those only newMS has.
func (e *SchemaEditor) alterUniqueTogether(ctx context.Context, oldMS, newMS *ModelState) error {
	keyed := func(ms *ModelState) (map[string][]string, []string, error) {
		byKey := map[string][]string{}
		var order []string
		for _, set := range ms.UniqueTogether {
			cols, err := ms.columns(set)
			if err != nil {
				return nil, nil, err
			}
			key := strings.Join(cols, ",")
			if _, dup := byKey[key]; !dup {
				order = append(order, key)
			}
			byKey[key] = cols
		}
		return byKey, order, nil
	}
	oldSets, oldOrder, err := keyed(oldMS)
	if err != nil {
		return err
	}
	newSets, newOrder, err := keyed(newMS)
	if err != nil {
		return err
	}
	for _, key := range oldOrder {
		if _, kept := newSets[key]; kept {
			continue
		}
		name := e.uniqueTogetherName(oldMS.Table, oldSets[key])
		if err := e.Execute(ctx, e.dropUniqueSQL(name, oldMS.Table)); err != nil {
			return err
		}
	}
	for _, key := range newOrder {
		if _, had := oldSets[key]; had {
			continue
		}
		name := e.uniqueTogetherName(newMS.Table, newSets[key])
		if err := e.Execute(ctx, e.addUniqueSQL(name, newMS.Table, newSets[key])); err != nil {
			return err
		}
	}
	return nil
}

// addIndex creates a named index on the table of ms.
func (e *SchemaEditor) addIndex(ctx context.Context, ms *ModelState, idx Index) error {
	cols, err := ms.indexColumns(idx.Fields)
	if err != nil {
		return err
	}
	return e.Execute(ctx, e.createOrderedIndexSQL(idx.Name, ms.Table, cols))
}

// removeIndex drops a named index.
func (e *SchemaEditor) removeIndex(ctx context.Context, name string) error {
	return e.Execute(ctx, e.dropIndexSQL(name))
}

// dropIndexSQL renders the statement that drops a named index. PostgreSQL adds
// IF EXISTS, as Django's PostgreSQL schema editor does.
func (e *SchemaEditor) dropIndexSQL(name string) string {
	if e.isSQLite() {
		return "DROP INDEX " + e.q(name)
	}
	return "DROP INDEX IF EXISTS " + e.q(name)
}
