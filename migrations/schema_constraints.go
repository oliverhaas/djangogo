package migrations

import (
	"context"

	"github.com/oliverhaas/djangogo/orm"
)

// stateModel builds the orm model of ms, on which the SQL of its constraints
// is rendered. A relation field refers to a model that holds only the primary
// key of its target.
func stateModel(state *ProjectState, ms *ModelState) *orm.Model {
	fields := make([]*orm.Field, 0, len(ms.Fields))
	for _, f := range ms.Fields {
		if f.IsManyToMany() {
			continue
		}
		of := toOrmField(f)
		of.Attr = f.Attr()
		if f.IsRelation() {
			of.Rel = &orm.Relation{Kind: f.RelKind, Column: f.Column, OnDelete: f.RelOnDelete}
			if target, err := state.relatedTo(f); err == nil && target.PrimaryKey() != nil {
				pk := toOrmField(*target.PrimaryKey())
				of.Rel.Target = orm.NewStateModel(target.App, target.Name, target.Table, []*orm.Field{pk})
			}
		}
		fields = append(fields, of)
	}
	return orm.NewStateModel(ms.App, ms.Name, ms.Table, fields)
}

// constraintsSQL returns the table constraints of ms.Constraints for CREATE
// TABLE and the statements that create the others after it.
func (e *SchemaEditor) constraintsSQL(state *ProjectState, ms *ModelState) (defs, stmts []string, err error) {
	if len(ms.Constraints) == 0 {
		return nil, nil, nil
	}
	m := stateModel(state, ms)
	for _, c := range ms.Constraints {
		def, stmt, err := orm.ConstraintSQL(e.dialect, m, c)
		if err != nil {
			return nil, nil, err
		}
		if def != "" {
			defs = append(defs, def)
		}
		if stmt != "" {
			stmts = append(stmts, stmt)
		}
	}
	return defs, stmts, nil
}

// addConstraint creates constraint c of ms, the model state that holds it.
// SQLite cannot add a table constraint to a table, so the table is remade
// (Django's DatabaseSchemaEditor.add_constraint).
func (e *SchemaEditor) addConstraint(ctx context.Context, state *ProjectState, ms *ModelState, c orm.Constraint) error {
	def, stmt, err := orm.ConstraintSQL(e.dialect, stateModel(state, ms), c)
	switch {
	case err != nil:
		return err
	case stmt != "":
		return e.Execute(ctx, stmt)
	case def == "":
		return nil
	case e.isSQLite():
		return e.remakeTable(ctx, state, ms, ms, nil)
	default:
		return e.Execute(ctx, "ALTER TABLE "+e.q(ms.Table)+" ADD "+def)
	}
}

// removeConstraint drops constraint c of oldMS. On SQLite a table constraint
// goes away when the table is remade as newMS, the model without it.
func (e *SchemaEditor) removeConstraint(ctx context.Context, state *ProjectState, oldMS, newMS *ModelState, c orm.Constraint) error {
	def, stmt, err := orm.ConstraintSQL(e.dialect, stateModel(state, oldMS), c)
	switch {
	case err != nil:
		return err
	case stmt != "":
		return e.Execute(ctx, e.dropIndexSQL(c.ConstraintName()))
	case def == "":
		return nil
	case e.isSQLite():
		return e.remakeTable(ctx, state, oldMS, newMS, nil)
	default:
		return e.Execute(ctx, "ALTER TABLE "+e.q(oldMS.Table)+" DROP CONSTRAINT "+e.q(c.ConstraintName()))
	}
}
