package orm

import (
	"errors"
	"fmt"

	"github.com/oliverhaas/djangogo/internal/naming"
)

// NewStateModel returns a model without a Go type, built from fields: the
// model of a migration state, on which a schema editor renders the SQL of a
// constraint (Django's historical models). The field with PrimaryKey set is
// the primary key, and a field without Attr gets the snake_case of its Name.
// The model cannot load or save rows.
func NewStateModel(app, name, table string, fields []*Field) *Model {
	m := &Model{
		app: app, name: name, table: table, cacheIndex: -1,
		byName:   make(map[string]*Field, len(fields)),
		byAttr:   make(map[string]*Field, len(fields)),
		byColumn: make(map[string]*Field, len(fields)),
	}
	for _, f := range fields {
		cp := *f
		cp.Index = -1
		if cp.Attr == "" {
			cp.Attr = naming.SnakeCase(cp.Name)
		}
		m.fields = append(m.fields, &cp)
		m.byName[cp.Name] = &cp
		m.byAttr[cp.Attr] = &cp
		if cp.Column != "" {
			m.byColumn[cp.Column] = &cp
		}
		if cp.PrimaryKey && m.pk == nil {
			m.pk = &cp
		}
	}
	return m
}

// ConstraintSQL returns the SQL of constraint c on model m in dialect d: a
// table constraint for CREATE TABLE or ALTER TABLE ADD, or a statement that
// creates a unique index. Both are empty when the database cannot enforce
// the constraint, as for NullsNotDistinct on SQLite.
func ConstraintSQL(d Dialect, m *Model, c Constraint) (tableDef, stmt string, err error) {
	if c == nil {
		return "", "", errors.New("orm: ConstraintSQL of a nil constraint")
	}
	prepared, err := c.prepare(m, c.ConstraintName())
	if err != nil {
		return "", "", fmt.Errorf("orm: model %s: %w", m.Label(), err)
	}
	return prepared.ddl(NewDB(nil, d, NewRegistry()), m)
}
