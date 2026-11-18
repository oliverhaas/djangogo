package migrations

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/oliverhaas/djangogo/orm"
)

// Loader builds the migration graph from a Registry and reads which
// migrations are applied, the counterpart of Django's MigrationLoader.
type Loader struct {
	// Graph holds every registered migration with its dependencies.
	Graph *Graph
	// Applied maps each applied migration to the time it was applied.
	Applied map[Key]time.Time

	migrations     map[Key]*Migration
	migratedApps   map[string]bool
	unmigratedApps map[string]bool
}

// NewLoader builds the graph of the migrations in reg. With a non-nil db it
// also loads the applied migrations. installedApps lists the app labels of
// the project; an app in it without migrations counts as unmigrated, which
// lets another app depend on its first migration before that exists.
func NewLoader(ctx context.Context, reg *Registry, db *orm.DB, installedApps []string) (*Loader, error) {
	l := &Loader{
		Graph:          NewGraph(),
		Applied:        map[Key]time.Time{},
		migrations:     map[Key]*Migration{},
		migratedApps:   map[string]bool{},
		unmigratedApps: map[string]bool{},
	}
	all := reg.All()
	for i := range all {
		m := &all[i]
		l.migrations[m.Key()] = m
		l.migratedApps[m.App] = true
	}
	for _, app := range installedApps {
		if !l.migratedApps[app] {
			l.unmigratedApps[app] = true
		}
	}
	if db != nil {
		applied, err := NewRecorder(db).AppliedMigrations(ctx)
		if err != nil {
			return nil, err
		}
		l.Applied = applied
	}
	if err := l.buildGraph(); err != nil {
		return nil, err
	}
	return l, nil
}

// sortedMigrationKeys returns the keys of the registered migrations in order.
func (l *Loader) sortedMigrationKeys() []Key {
	keys := make([]Key, 0, len(l.migrations))
	for k := range l.migrations {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareKeys)
	return keys
}

// buildGraph adds all nodes, then the dependencies inside each app, then the
// dependencies between apps, and checks the result.
func (l *Loader) buildGraph() error {
	keys := l.sortedMigrationKeys()
	for _, key := range keys {
		l.Graph.AddNode(key, l.migrations[key])
	}
	for _, key := range keys {
		m := l.migrations[key]
		for _, parent := range m.Dependencies {
			// A reference to the first migration of the own app is ignored.
			if parent.App == key.App && parent.Name != FirstMigration {
				_ = l.Graph.AddDependency(m.String(), key, parent, true)
			}
		}
	}
	for _, key := range keys {
		m := l.migrations[key]
		for _, parent := range m.Dependencies {
			if parent.App == key.App {
				continue
			}
			resolved, ok, err := l.checkKey(parent, key.App)
			if err != nil {
				return err
			}
			if ok {
				_ = l.Graph.AddDependency(m.String(), key, resolved, true)
			}
		}
		for _, child := range m.RunBefore {
			resolved, ok, err := l.checkKey(child, key.App)
			if err != nil {
				return err
			}
			if ok {
				_ = l.Graph.AddDependency(m.String(), resolved, key, true)
			}
		}
	}
	if err := l.Graph.ValidateConsistency(); err != nil {
		return err
	}
	return l.Graph.EnsureNotCyclic()
}

// checkKey resolves the special names "__first__" and "__latest__" to a real
// migration. It reports false when the dependency is to be ignored.
func (l *Loader) checkKey(key Key, currentApp string) (Key, bool, error) {
	if (key.Name != FirstMigration && key.Name != LatestMigration) || l.Graph.Contains(key) {
		return key, true, nil
	}
	if key.App == currentApp || l.unmigratedApps[key.App] {
		return Key{}, false, nil
	}
	if l.migratedApps[key.App] {
		nodes := l.Graph.RootNodes(key.App)
		if key.Name == LatestMigration {
			nodes = l.Graph.LeafNodes(key.App)
		}
		if len(nodes) == 0 {
			return Key{}, false, &BadMigrationError{Message: "Dependency on app with no migrations: " + key.App}
		}
		return nodes[0], true, nil
	}
	return Key{}, false, &BadMigrationError{Message: "Dependency on unknown app: " + key.App}
}

// MigratedApps returns the sorted labels of the apps that have migrations.
func (l *Loader) MigratedApps() []string {
	out := make([]string, 0, len(l.migratedApps))
	for app := range l.migratedApps {
		out = append(out, app)
	}
	sort.Strings(out)
	return out
}

// HasMigrations reports whether app has at least one migration.
func (l *Loader) HasMigrations(app string) bool { return l.migratedApps[app] }

// Migration returns the migration registered under key.
func (l *Loader) Migration(key Key) (*Migration, bool) {
	m, ok := l.migrations[key]
	return m, ok
}

// MigrationByPrefix returns the migration of app whose name starts with
// prefix. More than one match is an AmbiguityError; no match wraps
// ErrMigrationNotFound.
func (l *Loader) MigrationByPrefix(app, prefix string) (*Migration, error) {
	var found []*Migration
	for _, key := range l.sortedMigrationKeys() {
		if key.App == app && strings.HasPrefix(key.Name, prefix) {
			found = append(found, l.migrations[key])
		}
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("%w: There is no migration for '%s' with the prefix '%s'", ErrMigrationNotFound, app, prefix)
	case 1:
		return found[0], nil
	default:
		return nil, &AmbiguityError{App: app, Prefix: prefix}
	}
}

// CheckConsistentHistory fails with an InconsistentMigrationHistoryError when
// a migration is applied but one of its dependencies is not.
func (l *Loader) CheckConsistentHistory() error {
	applied := make([]Key, 0, len(l.Applied))
	for key := range l.Applied {
		applied = append(applied, key)
	}
	slices.SortFunc(applied, compareKeys)
	for _, key := range applied {
		if !l.Graph.Contains(key) {
			continue
		}
		for _, parent := range l.Graph.Parents(key) {
			if _, ok := l.Applied[parent]; !ok {
				return &InconsistentMigrationHistoryError{Migration: key, Dependency: parent}
			}
		}
	}
	return nil
}

// DetectConflicts returns the apps that have more than one leaf migration,
// with the sorted names of those leaves. Such an app needs a merge migration.
func (l *Loader) DetectConflicts() map[string][]string {
	seen := map[string][]string{}
	for _, leaf := range l.Graph.LeafNodes() {
		seen[leaf.App] = append(seen[leaf.App], leaf.Name)
	}
	conflicts := map[string][]string{}
	for app, names := range seen {
		if len(names) > 1 {
			sort.Strings(names)
			conflicts[app] = names
		}
	}
	return conflicts
}

// ProjectState returns the state after the given migrations, or after all
// leaf migrations when targets is nil. With atEnd false it is the state right
// before the targets.
func (l *Loader) ProjectState(targets []Key, atEnd bool) (*ProjectState, error) {
	return l.Graph.MakeState(targets, atEnd)
}

// CollectSQL returns the SQL the plan would execute on dialect d, with a
// comment in front of every operation, as sqlmigrate prints it.
func (l *Loader) CollectSQL(ctx context.Context, d orm.Dialect, plan []PlanStep) ([]string, error) {
	var state *ProjectState
	return CollectSQL(d, func(e *SchemaEditor) error {
		for _, step := range plan {
			if state == nil {
				var err error
				if state, err = l.ProjectState([]Key{step.Migration.Key()}, false); err != nil {
					return err
				}
			}
			if step.Backwards {
				if err := step.Migration.unapply(ctx, state, e); err != nil {
					return err
				}
				continue
			}
			if err := step.Migration.apply(ctx, state, e); err != nil {
				return err
			}
		}
		return nil
	})
}
