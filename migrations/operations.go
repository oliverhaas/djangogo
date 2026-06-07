package migrations

import (
	"fmt"

	"github.com/oliverhaas/djangogo/orm"
)

// Operation is one schema change. Apply advances the project state; SQL renders the
// DDL against the PRE-op state ps; Describe is a short label.
type Operation interface {
	Apply(ps *ProjectState)
	SQL(d orm.Dialect, ps *ProjectState) ([]string, error)
	Describe() string
}

// Compile-time checks that every concrete operation satisfies Operation.
var (
	_ Operation = CreateModel{}
	_ Operation = DeleteModel{}
	_ Operation = AddField{}
	_ Operation = RemoveField{}
	_ Operation = AlterField{}
)

// CreateModel adds a new model table.
type CreateModel struct {
	Name   string
	Table  string
	Fields []FieldState
}

// Apply registers the new model in ps, copying its fields and recording its order.
func (op CreateModel) Apply(ps *ProjectState) {
	fields := make([]FieldState, len(op.Fields))
	copy(fields, op.Fields)
	ps.Models[op.Name] = &ModelState{
		Name:   op.Name,
		Table:  op.Table,
		Fields: fields,
	}
	for _, name := range ps.Order {
		if name == op.Name {
			return
		}
	}
	ps.Order = append(ps.Order, op.Name)
}

// SQL renders the CREATE TABLE statement for the new model.
func (op CreateModel) SQL(d orm.Dialect, _ *ProjectState) ([]string, error) {
	return []string{createTableSQL(d, op.Table, op.Fields)}, nil
}

// Describe returns a short label for the operation.
func (op CreateModel) Describe() string { return "CreateModel " + op.Name }

// DeleteModel drops an existing model table.
type DeleteModel struct {
	Name string
}

// Apply removes the model from ps.Models and ps.Order.
func (op DeleteModel) Apply(ps *ProjectState) {
	delete(ps.Models, op.Name)
	for i, name := range ps.Order {
		if name == op.Name {
			ps.Order = append(ps.Order[:i], ps.Order[i+1:]...)
			return
		}
	}
}

// SQL renders the DROP TABLE statement using the table from the pre-op state.
func (op DeleteModel) SQL(d orm.Dialect, ps *ProjectState) ([]string, error) {
	ms, ok := ps.Models[op.Name]
	if !ok {
		return nil, fmt.Errorf("migrations: DeleteModel %s: model not found in state", op.Name)
	}
	return []string{"DROP TABLE " + d.Quote(ms.Table)}, nil
}

// Describe returns a short label for the operation.
func (op DeleteModel) Describe() string { return "DeleteModel " + op.Name }

// AddField appends a nullable column to an existing model.
type AddField struct {
	Model string
	Field FieldState
}

// Apply appends the field to the model's fields in ps.
func (op AddField) Apply(ps *ProjectState) {
	ms := ps.Models[op.Model]
	ms.Fields = append(ms.Fields, op.Field)
}

// SQL renders an ALTER TABLE ADD COLUMN. SQLite cannot add a NOT NULL column without
// a default, so the field must be nullable.
func (op AddField) SQL(d orm.Dialect, ps *ProjectState) ([]string, error) {
	ms, ok := ps.Models[op.Model]
	if !ok {
		return nil, fmt.Errorf("migrations: AddField %s.%s: model not found in state", op.Model, op.Field.Name)
	}
	if !op.Field.Null {
		return nil, fmt.Errorf("migrations: AddField %s.%s must be nullable (NOT NULL column needs a default)", op.Model, op.Field.Name)
	}
	return []string{
		"ALTER TABLE " + d.Quote(ms.Table) + " ADD COLUMN " + d.Quote(op.Field.Column) + " " + d.ColumnType(toOrmField(op.Field)),
	}, nil
}

// Describe returns a short label for the operation.
func (op AddField) Describe() string { return "AddField " + op.Model + "." + op.Field.Name }

// RemoveField drops a column from an existing model via a temp-table rebuild. Field is
// the Go field Name.
type RemoveField struct {
	Model string
	Field string
}

// Apply removes the named field from the model's fields in ps.
func (op RemoveField) Apply(ps *ProjectState) {
	ms := ps.Models[op.Model]
	for i := range ms.Fields {
		if ms.Fields[i].Name == op.Field {
			ms.Fields = append(ms.Fields[:i], ms.Fields[i+1:]...)
			return
		}
	}
}

// SQL renders the SQLite temp-table rebuild that drops the named column.
func (op RemoveField) SQL(d orm.Dialect, ps *ProjectState) ([]string, error) {
	ms, ok := ps.Models[op.Model]
	if !ok {
		return nil, fmt.Errorf("migrations: RemoveField %s.%s: model not found in state", op.Model, op.Field)
	}
	oldFields := ms.Fields
	newFields := make([]FieldState, 0, len(oldFields))
	found := false
	for _, f := range oldFields {
		if f.Name == op.Field {
			found = true
			continue
		}
		newFields = append(newFields, f)
	}
	if !found {
		return nil, fmt.Errorf("migrations: RemoveField %s.%s: field not found in state", op.Model, op.Field)
	}
	return rebuildTableSQL(d, ms.Table, oldFields, newFields), nil
}

// Describe returns a short label for the operation.
func (op RemoveField) Describe() string { return "RemoveField " + op.Model + "." + op.Field }

// AlterField changes a column's definition via a temp-table rebuild. Field is the new
// state, matched against existing fields by Name.
type AlterField struct {
	Model string
	Field FieldState
}

// Apply replaces the matching field (by Name) in the model's fields in ps.
func (op AlterField) Apply(ps *ProjectState) {
	ms := ps.Models[op.Model]
	for i := range ms.Fields {
		if ms.Fields[i].Name == op.Field.Name {
			ms.Fields[i] = op.Field
			return
		}
	}
}

// SQL renders the SQLite temp-table rebuild that applies the new field definition.
func (op AlterField) SQL(d orm.Dialect, ps *ProjectState) ([]string, error) {
	ms, ok := ps.Models[op.Model]
	if !ok {
		return nil, fmt.Errorf("migrations: AlterField %s.%s: model not found in state", op.Model, op.Field.Name)
	}
	oldFields := ms.Fields
	newFields := make([]FieldState, len(oldFields))
	copy(newFields, oldFields)
	found := false
	for i := range newFields {
		if newFields[i].Name == op.Field.Name {
			newFields[i] = op.Field
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("migrations: AlterField %s.%s: field not found in state", op.Model, op.Field.Name)
	}
	return rebuildTableSQL(d, ms.Table, oldFields, newFields), nil
}

// Describe returns a short label for the operation.
func (op AlterField) Describe() string { return "AlterField " + op.Model + "." + op.Field.Name }
