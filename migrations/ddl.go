package migrations

import (
	"strings"

	"github.com/oliverhaas/djangogo/orm"
)

// toOrmField reconstructs an *orm.Field from a FieldState for dialect DDL rendering.
func toOrmField(fs FieldState) *orm.Field {
	return &orm.Field{
		Name:       fs.Name,
		Column:     fs.Column,
		Kind:       fs.Kind,
		PrimaryKey: fs.PrimaryKey,
		Null:       fs.Null,
		Unique:     fs.Unique,
		MaxLength:  fs.MaxLength,
	}
}

// createTableSQL renders a CREATE TABLE for the given table and fields using d.
// Each FK field also emits a table-level FOREIGN KEY constraint after the column
// definitions.
func createTableSQL(d orm.Dialect, table string, fields []FieldState) string {
	defs := make([]string, 0, len(fields))
	for _, f := range fields {
		defs = append(defs, d.Quote(f.Column)+" "+d.ColumnType(toOrmField(f)))
	}
	for _, f := range fields {
		if f.RelKind == orm.RelFK && f.RelTargetTable != "" {
			defs = append(defs, "FOREIGN KEY ("+d.Quote(f.Column)+") REFERENCES "+
				d.Quote(f.RelTargetTable)+" ("+d.Quote(f.RelTargetColumn)+")")
		}
	}
	return "CREATE TABLE " + d.Quote(table) + " (" + strings.Join(defs, ", ") + ")"
}

// rebuildTableSQL renders the SQLite temp-table rebuild that transforms a table from
// oldFields to newFields, preserving the data of columns common to both. It emits the
// CREATE/INSERT/DROP/RENAME sequence, omitting the INSERT when no columns are common.
func rebuildTableSQL(d orm.Dialect, table string, oldFields, newFields []FieldState) []string {
	tmp := table + "__new"

	oldCols := make(map[string]struct{}, len(oldFields))
	for _, f := range oldFields {
		oldCols[f.Column] = struct{}{}
	}

	var commonCols []string
	for _, f := range newFields {
		if _, ok := oldCols[f.Column]; ok {
			commonCols = append(commonCols, d.Quote(f.Column))
		}
	}

	stmts := make([]string, 0, 4)
	stmts = append(stmts, createTableSQL(d, tmp, newFields))
	if len(commonCols) > 0 {
		quotedCommon := strings.Join(commonCols, ", ")
		stmts = append(stmts, "INSERT INTO "+d.Quote(tmp)+" ("+quotedCommon+") SELECT "+quotedCommon+" FROM "+d.Quote(table))
	}
	stmts = append(stmts, "DROP TABLE "+d.Quote(table))
	stmts = append(stmts, "ALTER TABLE "+d.Quote(tmp)+" RENAME TO "+d.Quote(table))

	return stmts
}
