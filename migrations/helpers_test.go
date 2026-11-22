package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/migrations"
	"github.com/oliverhaas/djangogo/orm"
)

// Field definitions of the test models. They mirror the models Django's
// OperationTestBase.set_up_test_model builds, with Go field names.
var (
	idField     = migrations.FieldState{Name: "ID", Column: "id", Kind: orm.KindAuto, PrimaryKey: true}
	pinkField   = migrations.FieldState{Name: "Pink", Column: "pink", Kind: orm.KindInt, Default: int64(3)}
	weightField = migrations.FieldState{Name: "Weight", Column: "weight", Kind: orm.KindInt}
	greenField  = migrations.FieldState{Name: "Green", Column: "green", Kind: orm.KindInt, Null: true}
	yellowField = migrations.FieldState{
		Name: "Yellow", Column: "yellow", Kind: orm.KindChar, MaxLength: 20, Null: true, Blank: true,
	}
)

// fkField returns a cascading foreign key to the model with the given label.
func fkField(name, column, to string) migrations.FieldState {
	return migrations.FieldState{
		Name: name, Column: column, Kind: orm.KindInt,
		RelKind: orm.RelFK, RelTo: to, RelOnDelete: orm.OnDeleteCascade,
	}
}

// testModelOptions selects what setUpTestModel creates besides the Pony
// model. The names follow the keyword arguments of Django's
// set_up_test_model.
type testModelOptions struct {
	secondModel    bool
	thirdModel     bool
	relatedModel   bool
	index          bool
	multicolIndex  bool
	uniqueTogether bool
	table          string
	constraints    []orm.Constraint
}

// applyOps runs the operations forwards in one schema editor, starting from
// state, and returns the state they lead to (Django's apply_operations).
func applyOps(t *testing.T, db *orm.DB, app string, state *migrations.ProjectState, ops ...migrations.Operation) *migrations.ProjectState {
	t.Helper()
	state, err := tryApplyOps(db, app, state, ops...)
	if err != nil {
		t.Fatalf("apply operations: %v", err)
	}
	return state
}

// tryApplyOps is applyOps for tests that expect an error.
func tryApplyOps(db *orm.DB, app string, state *migrations.ProjectState, ops ...migrations.Operation) (*migrations.ProjectState, error) {
	ctx := context.Background()
	err := migrations.WithSchemaEditor(ctx, db, func(e *migrations.SchemaEditor) error {
		for _, op := range ops {
			next := state.Clone()
			if err := op.StateForwards(app, next); err != nil {
				return err
			}
			if err := op.DatabaseForwards(ctx, app, e, state, next); err != nil {
				return err
			}
			state = next
		}
		return nil
	})
	return state, err
}

// unapplyOps runs the operations backwards in one schema editor, last one
// first. state is the state before the first operation (Django's
// unapply_operations).
func unapplyOps(t *testing.T, db *orm.DB, app string, state *migrations.ProjectState, ops ...migrations.Operation) {
	t.Helper()
	ctx := context.Background()
	states := make([]*migrations.ProjectState, 1, len(ops)+1)
	states[0] = state
	for _, op := range ops {
		next := states[len(states)-1].Clone()
		if err := op.StateForwards(app, next); err != nil {
			t.Fatalf("state forwards: %v", err)
		}
		states = append(states, next)
	}
	err := migrations.WithSchemaEditor(ctx, db, func(e *migrations.SchemaEditor) error {
		for i := len(ops) - 1; i >= 0; i-- {
			if err := ops[i].DatabaseBackwards(ctx, app, e, states[i+1], states[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unapply operations: %v", err)
	}
}

// stateForwards returns a copy of state with the state change of op applied.
func stateForwards(t *testing.T, app string, state *migrations.ProjectState, op migrations.Operation) *migrations.ProjectState {
	t.Helper()
	next := state.Clone()
	if err := op.StateForwards(app, next); err != nil {
		t.Fatalf("state forwards: %v", err)
	}
	return next
}

// databaseForwards runs the database change of op from state from to state to.
func databaseForwards(t *testing.T, db *orm.DB, app string, op migrations.Operation, from, to *migrations.ProjectState) {
	t.Helper()
	ctx := context.Background()
	err := migrations.WithSchemaEditor(ctx, db, func(e *migrations.SchemaEditor) error {
		return op.DatabaseForwards(ctx, app, e, from, to)
	})
	if err != nil {
		t.Fatalf("database forwards: %v", err)
	}
}

// databaseBackwards undoes the database change of op. from is the state
// after the operation, to the state before it.
func databaseBackwards(t *testing.T, db *orm.DB, app string, op migrations.Operation, from, to *migrations.ProjectState) {
	t.Helper()
	ctx := context.Background()
	err := migrations.WithSchemaEditor(ctx, db, func(e *migrations.SchemaEditor) error {
		return op.DatabaseBackwards(ctx, app, e, from, to)
	})
	if err != nil {
		t.Fatalf("database backwards: %v", err)
	}
}

// setUpTestModel creates the Pony model and, on request, the Stable, Van and
// Rider models, and returns the state after it (Django's set_up_test_model).
// Django's weight field is a FloatField; the ORM has no float kind, so it is an
// integer here.
func setUpTestModel(t *testing.T, db *orm.DB, app string, opts testModelOptions) *migrations.ProjectState {
	t.Helper()
	pony := migrations.CreateModel{
		Name:   "Pony",
		Table:  opts.table,
		Fields: []migrations.FieldState{idField, pinkField, weightField, greenField, yellowField},
	}
	if opts.uniqueTogether {
		pony.UniqueTogether = [][]string{{"Pink", "Weight"}}
	}
	ops := []migrations.Operation{pony}
	if opts.index {
		ops = append(ops, migrations.AddIndex{
			Model: "Pony", Index: migrations.Index{Name: "pony_pink_idx", Fields: []string{"Pink"}},
		})
	}
	if opts.multicolIndex {
		ops = append(ops, migrations.AddIndex{
			Model: "Pony", Index: migrations.Index{Name: "pony_test_idx", Fields: []string{"Pink", "Weight"}},
		})
	}
	for _, c := range opts.constraints {
		ops = append(ops, migrations.AddConstraint{Model: "Pony", Constraint: c})
	}
	if opts.secondModel {
		ops = append(ops, migrations.CreateModel{Name: "Stable", Fields: []migrations.FieldState{idField}})
	}
	if opts.thirdModel {
		ops = append(ops, migrations.CreateModel{Name: "Van", Fields: []migrations.FieldState{idField}})
	}
	if opts.relatedModel {
		friend := fkField("Friend", "friend_id", app+".Rider")
		friend.Null = true
		ops = append(ops, migrations.CreateModel{
			Name:   "Rider",
			Fields: []migrations.FieldState{idField, fkField("Pony", "pony_id", app+".Pony"), friend},
		})
	}
	return applyOps(t, db, app, migrations.NewProjectState(), ops...)
}

// rebind rewrites the "?" placeholders of query for the dialect of db.
func rebind(db *orm.DB, query string) string {
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString(db.Dialect().Placeholder(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// execSQL runs a statement written with "?" placeholders and returns its
// error.
func execSQL(db *orm.DB, query string, args ...any) error {
	_, err := db.SQL().ExecContext(context.Background(), rebind(db, query), args...)
	return err
}

// mustExec runs a statement written with "?" placeholders.
func mustExec(t *testing.T, db *orm.DB, query string, args ...any) {
	t.Helper()
	if err := execSQL(db, query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// queryInt returns the single integer a query yields.
func queryInt(t *testing.T, db *orm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.SQL().QueryRowContext(context.Background(), rebind(db, query), args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

// modelOf returns a model of the state and fails the test when it is missing.
func modelOf(t *testing.T, state *migrations.ProjectState, app, name string) *migrations.ModelState {
	t.Helper()
	ms, ok := state.Model(app, name)
	if !ok {
		t.Fatalf("model %s.%s is not in the state", app, name)
	}
	return ms
}

// fieldOf returns a field of a model in the state and fails the test when it
// is missing.
func fieldOf(t *testing.T, state *migrations.ProjectState, app, model, field string) migrations.FieldState {
	t.Helper()
	f, ok := modelOf(t, state, app, model).FieldByName(field)
	if !ok {
		t.Fatalf("field %s is not in model %s.%s", field, app, model)
	}
	return *f
}
