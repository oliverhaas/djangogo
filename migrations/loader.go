package migrations

import (
	"context"
	"errors"
	"fmt"
	"maps"
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
	// Applied maps each applied migration to the time it was applied. A
	// squashed migration counts as applied when all the migrations it
	// replaces are applied.
	Applied map[Key]time.Time
	// ReplaceMigrations makes the graph use a squashed migration in place of
	// the migrations it replaces, where the applied migrations allow it.
	// NewLoader sets it; call BuildGraph after a change. Without it, the
	// squashed and the replaced migrations are all in the graph.
	ReplaceMigrations bool

	db *orm.DB
	// recorded holds the rows of the tracking table.
	recorded       map[Key]time.Time
	migrations     map[Key]*Migration
	replacements   map[Key]*Migration
	migratedApps   map[string]bool
	unmigratedApps map[string]bool
}

// NewLoader builds the graph of the migrations in reg. With a non-nil db it
// also loads the applied migrations. installedApps lists the app labels of
// the project; an app in it without migrations counts as unmigrated, which
// lets another app depend on its first migration before that exists.
func NewLoader(ctx context.Context, reg *Registry, db *orm.DB, installedApps []string) (*Loader, error) {
	l := &Loader{
		ReplaceMigrations: true,
		db:                db,
		migrations:        map[Key]*Migration{},
		migratedApps:      map[string]bool{},
		unmigratedApps:    map[string]bool{},
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
	if err := l.BuildGraph(ctx); err != nil {
		return nil, err
	}
	return l, nil
}

// BuildGraph reads the applied migrations from the database again and
// rebuilds the graph. Call it after migrations are applied or unapplied, or
// after ReplaceMigrations changes.
func (l *Loader) BuildGraph(ctx context.Context) error {
	l.recorded = map[Key]time.Time{}
	if l.db != nil {
		applied, err := NewRecorder(l.db).AppliedMigrations(ctx)
		if err != nil {
			return err
		}
		l.recorded = applied
	}
	return l.buildGraph()
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
// dependencies between apps, puts squashed migrations in place, and checks the
// result. The applied migrations come from the recorded rows.
func (l *Loader) buildGraph() error {
	l.Graph = NewGraph()
	l.Applied = maps.Clone(l.recorded)
	if l.Applied == nil {
		l.Applied = map[Key]time.Time{}
	}
	l.replacements = map[Key]*Migration{}
	keys := l.sortedMigrationKeys()
	for _, key := range keys {
		m := l.migrations[key]
		l.Graph.AddNode(key, m)
		if len(m.Replaces) > 0 {
			l.replacements[key] = m
		}
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
	if l.ReplaceMigrations {
		done := map[Key]bool{}
		for _, key := range sortedMigrationMapKeys(l.replacements) {
			if err := l.replaceMigration(key, done); err != nil {
				return err
			}
		}
	} else {
		// A squashed migration then stands beside the migrations it
		// replaces. Only those count as applied, or the project state of
		// the applied migrations would hold their models twice.
		for key := range l.replacements {
			delete(l.Applied, key)
		}
	}
	if err := l.Graph.ValidateConsistency(); err != nil {
		return l.explainMissingNode(err)
	}
	return l.Graph.EnsureNotCyclic()
}

// sortedMigrationMapKeys returns the keys of set in ascending order.
func sortedMigrationMapKeys(set map[Key]*Migration) []Key {
	keys := slices.Collect(maps.Keys(set))
	slices.SortFunc(keys, compareKeys)
	return keys
}

// replaceMigration puts the squashed migration under key in place of the
// migrations it replaces when all or none of them are applied, and removes it
// from the graph otherwise. A squashed migration that replaces another one is
// handled after that one. done records the finished replacements; a key that
// is in it with false is in progress, which means a cycle.
func (l *Loader) replaceMigration(key Key, done map[Key]bool) error {
	if finished, seen := done[key]; seen {
		if finished {
			return nil
		}
		return &CommandError{Code: 1, Message: "Cyclical squash replacement found, starting at " + key.repr()}
	}
	done[key] = false
	m := l.replacements[key]
	for _, replaced := range m.Replaces {
		if _, ok := l.replacements[replaced]; ok {
			if err := l.replaceMigration(replaced, done); err != nil {
				return err
			}
		}
	}
	targets := l.resolveReplacedKeys(m)
	applied := 0
	var last time.Time
	for _, target := range targets {
		if at, ok := l.Applied[target]; ok {
			applied++
			if at.After(last) {
				last = at
			}
		}
	}
	// The squashed migration counts as applied only when all the migrations
	// it replaces are.
	if applied == len(targets) {
		if at, ok := l.recorded[key]; ok {
			last = at
		}
		l.Applied[key] = last
	} else {
		delete(l.Applied, key)
	}
	var err error
	if applied == len(targets) || applied == 0 {
		err = l.Graph.RemoveReplacedNodes(key, m.Replaces)
	} else {
		// Partly applied: the replaced migrations stay, and what depends on
		// the squashed migration moves to them.
		err = l.Graph.RemoveReplacementNode(key, m.Replaces)
	}
	if err != nil {
		return err
	}
	done[key] = true
	return nil
}

// resolveReplacedKeys returns the migrations m replaces, with each squashed
// migration among them resolved to the migrations it replaces in turn.
func (l *Loader) resolveReplacedKeys(m *Migration) []Key {
	seen := map[Key]bool{}
	var out []Key
	for _, key := range m.Replaces {
		if seen[key] {
			continue
		}
		seen[key] = true
		if inner, ok := l.migrations[key]; ok && len(inner.Replaces) > 0 {
			out = append(out, l.resolveReplacedKeys(inner)...)
			continue
		}
		out = append(out, key)
	}
	return out
}

// explainMissingNode turns the error for a migration that depends on a
// replaced migration that is gone into a message that names the squashed
// migration that could not be used because it is partly applied.
func (l *Loader) explainMissingNode(err error) error {
	var missing *NodeNotFoundError
	if !errors.As(err, &missing) {
		return err
	}
	var candidates []string
	for _, key := range sortedMigrationMapKeys(l.replacements) {
		if !slices.Contains(l.replacements[key].Replaces, missing.Node) {
			continue
		}
		if l.Graph.Contains(key) {
			return err
		}
		candidates = append(candidates, key.String())
	}
	if len(candidates) == 0 {
		return err
	}
	return &NodeNotFoundError{
		Message: fmt.Sprintf("Migration %s depends on nonexistent node %s. djangogo tried to replace migration %s "+
			"with any of [%s] but wasn't able to because some of the replaced migrations are already applied.",
			missing.Origin, missing.Node.repr(), missing.Node, strings.Join(candidates, ", ")),
		Node: missing.Node,
	}
}

// allReplacedApplied reports whether key is a squashed migration whose
// replaced migrations are all in applied, each directly or through a
// squashed migration of its own.
func (l *Loader) allReplacedApplied(key Key, applied map[Key]time.Time) bool {
	m, ok := l.replacements[key]
	if !ok {
		return false
	}
	for _, replaced := range m.Replaces {
		if _, ok := applied[replaced]; !ok && !l.allReplacedApplied(replaced, applied) {
			return false
		}
	}
	return true
}

// noteRecorded updates the applied migrations after the row of key was
// written (forward) or deleted.
func (l *Loader) noteRecorded(key Key, forward bool) {
	if !forward {
		delete(l.Applied, key)
		delete(l.recorded, key)
		return
	}
	now := time.Now().UTC()
	l.Applied[key] = now
	if l.recorded != nil {
		l.recorded[key] = now
	}
}

// Replacement returns the squashed migration under key, which is registered
// whether or not the graph uses it.
func (l *Loader) Replacement(key Key) (*Migration, bool) {
	m, ok := l.replacements[key]
	return m, ok
}

// Replacements returns the keys of the squashed migrations in ascending order.
func (l *Loader) Replacements() []Key { return sortedMigrationMapKeys(l.replacements) }

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
// a migration is applied but one of its dependencies is not. It checks the
// rows of the tracking table, or Applied for a loader without a database.
func (l *Loader) CheckConsistentHistory() error {
	set := l.Applied
	if l.db != nil {
		set = l.recorded
	}
	applied := slices.Collect(maps.Keys(set))
	slices.SortFunc(applied, compareKeys)
	for _, key := range applied {
		if !l.Graph.Contains(key) {
			continue
		}
		for _, parent := range l.Graph.Parents(key) {
			if _, ok := set[parent]; ok {
				continue
			}
			// A squashed migration whose replaced migrations are all applied
			// is not recorded until the next migrate.
			if l.allReplacedApplied(parent, set) {
				continue
			}
			return &InconsistentMigrationHistoryError{Migration: key, Dependency: parent}
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
