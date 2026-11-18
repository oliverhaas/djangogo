package migrations

import (
	"errors"
	"fmt"
	"strings"
)

// ErrMigrationNotFound is the sentinel wrapped by a failed prefix lookup that
// matches no migration (Django raises KeyError there).
var ErrMigrationNotFound = errors.New("migrations: migration not found")

// NodeNotFoundError reports a migration that a plan or a dependency refers to
// but that is not part of the graph (Django's NodeNotFoundError).
type NodeNotFoundError struct {
	// Message is Django's message text.
	Message string
	// Node is the missing migration.
	Node Key
	// Origin names the migration whose dependency led to Node, or "".
	Origin string
}

// Error returns Django's message.
func (e *NodeNotFoundError) Error() string { return e.Message }

// CircularDependencyError reports a dependency cycle between migrations
// (Django's CircularDependencyError).
type CircularDependencyError struct {
	// Cycle lists the migrations on the cycle in dependency order.
	Cycle []Key
}

// Error lists the cycle as "app.name, app.name", as Django does.
func (e *CircularDependencyError) Error() string {
	parts := make([]string, len(e.Cycle))
	for i, k := range e.Cycle {
		parts[i] = k.String()
	}
	return strings.Join(parts, ", ")
}

// AmbiguityError reports a migration name prefix that matches more than one
// migration of an app (Django's AmbiguityError).
type AmbiguityError struct {
	App    string
	Prefix string
}

// Error returns Django's message.
func (e *AmbiguityError) Error() string {
	return fmt.Sprintf("There is more than one migration for '%s' with the prefix '%s'", e.App, e.Prefix)
}

// InconsistentMigrationHistoryError reports an applied migration whose
// dependency is not applied (Django's InconsistentMigrationHistory).
type InconsistentMigrationHistoryError struct {
	Migration  Key
	Dependency Key
}

// Error returns Django's message. Django names the database alias at the end;
// a Djan-Go-Go project has one database, reported as 'default'.
func (e *InconsistentMigrationHistoryError) Error() string {
	return fmt.Sprintf("Migration %s is applied before its dependency %s on database 'default'.",
		e.Migration, e.Dependency)
}

// IrreversibleError reports an attempt to unapply a migration that contains an
// operation without a backward step (Django's IrreversibleError).
type IrreversibleError struct {
	Operation string
	Migration Key
}

// Error returns Django's message.
func (e *IrreversibleError) Error() string {
	return fmt.Sprintf("Operation %s in %s is not reversible", e.Operation, e.Migration)
}

// InvalidMigrationPlanError reports a plan that mixes forward and backward
// steps (Django's InvalidMigrationPlan).
type InvalidMigrationPlanError struct {
	Plan []PlanStep
}

// Error returns Django's message.
func (e *InvalidMigrationPlanError) Error() string {
	return "Migration plans with both forwards and backwards migrations are not supported. " +
		"Please split your migration process into separate plans of only forwards OR backwards migrations."
}

// BadMigrationError reports a migration that cannot be used as registered, for
// example one that depends on an app without migrations (Django's
// BadMigrationError and the ValueError of the loader).
type BadMigrationError struct {
	Message string
}

// Error returns the message.
func (e *BadMigrationError) Error() string { return e.Message }
