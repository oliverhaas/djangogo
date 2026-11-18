package migrations

import (
	"context"
	"fmt"
	"strings"
)

// Operation is one step of a migration, the counterpart of a Django migration
// operation. It changes the project state and makes the matching change in
// the database, forwards and backwards.
//
// An operation may implement these optional methods:
//
//	Reversible() bool    // false: the migration cannot be unapplied
//	ReducesToSQL() bool  // false: sqlmigrate prints a comment instead of SQL
//	Category() Category  // the symbol makemigrations prints in front
//	NameFragment() string // the part it adds to a suggested migration name
//
// Without them an operation is reversible, reduces to SQL, has the mixed
// category and adds no name fragment.
type Operation interface {
	// StateForwards applies the change to state in place. The app label is
	// the app of the migration that holds the operation.
	StateForwards(app string, state *ProjectState) error
	// DatabaseForwards makes the change in the database. from is the state
	// before the operation and to the state after it.
	DatabaseForwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error
	// DatabaseBackwards undoes the change in the database. from is the state
	// with the operation applied and to the state without it.
	DatabaseBackwards(ctx context.Context, app string, e *SchemaEditor, from, to *ProjectState) error
	// Describe returns the one-line description shown by the commands.
	Describe() string
}

// Category classifies an operation; its value is the symbol makemigrations
// prints in front of the description (Django's OperationCategory).
type Category string

// The operation categories.
const (
	CategoryAddition   Category = "+"
	CategoryRemoval    Category = "-"
	CategoryAlteration Category = "~"
	CategoryGo         Category = "g"
	CategorySQL        Category = "s"
	CategoryMixed      Category = "?"
)

// isReversible reports whether op has a backward step.
func isReversible(op Operation) bool {
	if r, ok := op.(interface{ Reversible() bool }); ok {
		return r.Reversible()
	}
	return true
}

// reducesToSQL reports whether op can be shown as SQL.
func reducesToSQL(op Operation) bool {
	if r, ok := op.(interface{ ReducesToSQL() bool }); ok {
		return r.ReducesToSQL()
	}
	return true
}

// nameFragment returns the fragment op adds to a suggested migration name.
func nameFragment(op Operation) string {
	if r, ok := op.(interface{ NameFragment() string }); ok {
		return r.NameFragment()
	}
	return ""
}

// FormattedDescription returns the description of op with its category symbol
// in front, e.g. "+ Create model Post".
func FormattedDescription(op Operation) string {
	cat := CategoryMixed
	if c, ok := op.(interface{ Category() Category }); ok {
		cat = c.Category()
	}
	return string(cat) + " " + op.Describe()
}

// DescribeStep returns the line migrate --plan prints for op, and whether the
// step is a backward step that cannot be undone. A backward step of a schema
// operation is prefixed with "Undo "; RunSQL and RunGo show their action.
func DescribeStep(op Operation, backwards bool) (string, bool) {
	prefix := ""
	action := ""
	isError := false
	switch o := op.(type) {
	case RunSQL:
		stmts := o.SQL
		if backwards {
			stmts = o.ReverseSQL
			if o.ReverseSQL == nil {
				action, isError = "IRREVERSIBLE", true
			}
		}
		if len(stmts) > 0 {
			action = strings.ReplaceAll(strings.Join(stmts, " "), "\n", "")
		}
	case RunGo:
		if backwards && o.Reverse == nil {
			action, isError = "IRREVERSIBLE", true
		}
	default:
		if backwards {
			prefix = "Undo "
			if !isReversible(op) {
				action, isError = "IRREVERSIBLE", true
			}
		}
	}
	if action != "" {
		action = " -> " + action
	}
	if r := []rune(action); len(r) > 40 {
		action = string(r[:39]) + "…"
	}
	return prefix + op.Describe() + action, isError
}

// lowerName renders a model name the way Django prints it in the description
// of a field operation.
func lowerName(model string) string { return strings.ToLower(model) }

// modelPair fetches the same model from two states.
func modelPair(app, name string, from, to *ProjectState) (fromMS, toMS *ModelState, err error) {
	if fromMS, err = from.mustModel(app, name); err != nil {
		return nil, nil, err
	}
	if toMS, err = to.mustModel(app, name); err != nil {
		return nil, nil, err
	}
	return fromMS, toMS, nil
}

// errNoField builds the error for a field an operation names but the model lacks.
func errNoField(ms *ModelState, field string) error {
	return fmt.Errorf("migrations: %s has no field named %q", ms.Label(), field)
}
