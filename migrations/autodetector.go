package migrations

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// modelKey identifies a model by app and name.
type modelKey struct{ app, name string }

// label returns "app.Name".
func (k modelKey) label() string { return ModelLabel(k.app, k.name) }

// compareModelKeys orders by app and then by model name, ignoring case first,
// as Django's lowercase model keys sort.
func compareModelKeys(a, b modelKey) int {
	return cmp.Or(
		cmp.Compare(a.app, b.app),
		cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)),
		cmp.Compare(a.name, b.name),
	)
}

// fieldKey identifies a field by app, model and Go field name.
type fieldKey struct{ app, model, field string }

// compareFieldKeys orders by model and then by the snake_case field name,
// which is the name Django sorts by.
func compareFieldKeys(a, b fieldKey) int {
	return cmp.Or(
		compareModelKeys(modelKey{a.app, a.model}, modelKey{b.app, b.model}),
		cmp.Compare(snakeCase(a.field), snakeCase(b.field)),
		cmp.Compare(a.field, b.field),
	)
}

// sortedDiff returns the keys of a that are not in b, sorted.
func sortedDiff[K comparable](a, b map[K]bool, compare func(x, y K) int) []K {
	var out []K
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	slices.SortFunc(out, compare)
	return out
}

// sortedCommon returns the keys that are in both sets, sorted.
func sortedCommon[K comparable](a, b map[K]bool, compare func(x, y K) int) []K {
	var out []K
	for k := range a {
		if b[k] {
			out = append(out, k)
		}
	}
	slices.SortFunc(out, compare)
	return out
}

// depKind is the kind of thing a generated operation waits for.
type depKind int

// The dependency kinds, as in Django's OperationDependency.Type.
const (
	// depCreate: the model, or the field when one is named, is created.
	depCreate depKind = iota + 1
	// depRemove: the model, or the field when one is named, is removed.
	depRemove
	// depAlter: the field is altered.
	depAlter
	// depAlterTogether: the unique-together sets of the model are altered.
	depAlterTogether
	// depRemoveIndex: an index of the model is removed.
	depRemoveIndex
)

// opDep says that an operation must come after the operation that does kind
// to the model, or to the field when field is not empty.
type opDep struct {
	app, model, field string
	kind              depKind
}

// genOp is a generated operation with what it depends on.
type genOp struct {
	op   Operation
	deps []opDep
}

// satisfies reports whether op is the operation that dep waits for.
func satisfies(op Operation, dep opDep) bool {
	switch dep.kind {
	case depCreate:
		switch o := op.(type) {
		case CreateModel:
			if o.Name != dep.model {
				return false
			}
			return dep.field == "" || slices.ContainsFunc(o.Fields, func(f FieldState) bool { return f.Name == dep.field })
		case AddField:
			return dep.field != "" && o.Model == dep.model && o.Field.Name == dep.field
		}
	case depRemove:
		switch o := op.(type) {
		case DeleteModel:
			return dep.field == "" && o.Name == dep.model
		case RemoveField:
			return dep.field != "" && o.Model == dep.model && o.Field == dep.field
		}
	case depAlter:
		o, ok := op.(AlterField)
		return ok && o.Model == dep.model && o.Field.Name == dep.field
	case depAlterTogether:
		o, ok := op.(AlterUniqueTogether)
		return ok && o.Name == dep.model
	case depRemoveIndex:
		o, ok := op.(RemoveIndex)
		return ok && o.Model == dep.model
	}
	return false
}

// Autodetector compares two project states and works out the migrations that
// lead from one to the other, the counterpart of Django's
// MigrationAutodetector.
//
// It cannot tell a renamed field or model from a removed and an added one,
// so it asks its Questioner when a pair looks like a rename.
type Autodetector struct {
	// PreserveModelOptions keeps the indexes and unique-together sets that
	// the migrations declare for a model that stays, instead of comparing them
	// with the target state. Set it when the target state comes from a source
	// that cannot express them; without it, a hand-written AddIndex would be
	// taken back by the next generated migration.
	PreserveModelOptions bool

	from, to   *ProjectState
	questioner Questioner

	// generated holds the operations per app before they are cut into
	// migrations.
	generated map[string][]*genOp
	// migrations holds the result per app.
	migrations map[string][]*Migration
	// existingApps are the apps that have a model in the from state. A
	// migration of any other app is an initial one.
	existingApps map[string]bool

	oldModels, newModels map[modelKey]bool
	keptModels           []modelKey
	// renamedModels maps a renamed model to its old name.
	renamedModels map[modelKey]string
	// renamedModelsRel maps the old label of a renamed model to the new one.
	renamedModelsRel map[string]string

	oldFields, newFields map[fieldKey]bool
	// renamedFields maps a renamed field to its old name.
	renamedFields map[fieldKey]string
	renames       []fieldRename
	indexChanges  []indexChange
}

// NewAutodetector returns an autodetector for the change from the state from
// to the state to. A nil questioner answers no to every question.
func NewAutodetector(from, to *ProjectState, q Questioner) *Autodetector {
	if q == nil {
		q = FixedQuestioner{}
	}
	return &Autodetector{from: from, to: to, questioner: q}
}

// Changes returns the new migrations per app label. Their names and
// dependencies continue the leaf nodes of graph, which may be nil for a
// project without migrations. With trimToApps, only the migrations of those
// apps and of the apps they depend on are returned. migrationName, when not
// empty, replaces the generated part of the names.
func (a *Autodetector) Changes(graph *Graph, trimToApps []string, migrationName string) (map[string][]*Migration, error) {
	if graph == nil {
		graph = NewGraph()
	}
	if err := a.detectChanges(graph); err != nil {
		return nil, err
	}
	changes := ArrangeForGraph(a.migrations, graph, migrationName)
	if len(trimToApps) > 0 {
		changes = trimChanges(changes, trimToApps)
	}
	return changes, nil
}

// detectChanges generates the operations and cuts them into migrations with
// placeholder names. The order of the steps is Django's: renames first, then
// models, then indexes and fields.
func (a *Autodetector) detectChanges(graph *Graph) error {
	a.generated = map[string][]*genOp{}
	a.renamedFields = map[fieldKey]string{}
	a.renames = nil
	a.indexChanges = nil
	a.existingApps = map[string]bool{}
	a.oldModels = map[modelKey]bool{}
	a.newModels = map[modelKey]bool{}
	for _, ms := range a.from.Models {
		a.oldModels[modelKey{ms.App, ms.Name}] = true
		a.existingApps[ms.App] = true
	}
	for _, ms := range a.to.Models {
		a.newModels[modelKey{ms.App, ms.Name}] = true
	}

	if err := a.generateRenamedModels(); err != nil {
		return err
	}
	a.prepareFieldLists()
	a.generateDeletedModels()
	a.generateCreatedModels()
	if err := a.createRenamedFields(); err != nil {
		return err
	}
	a.createAlteredIndexes()
	a.generateRemovedIndexes()
	a.generateRenamedFields()
	a.generateAlteredUniqueTogether(true)
	a.generateRemovedFields()
	if err := a.generateAddedFields(); err != nil {
		return err
	}
	if err := a.generateAlteredFields(); err != nil {
		return err
	}
	a.generateAlteredUniqueTogether(false)
	a.generateAddedIndexes()
	a.generateAlteredTables()

	if err := a.sortOperations(); err != nil {
		return err
	}
	if err := a.buildMigrationList(graph); err != nil {
		return err
	}
	a.optimizeMigrations()
	return nil
}

// addOperation appends a generated operation to its app, or puts it in front.
func (a *Autodetector) addOperation(app string, op Operation, deps []opDep, beginning bool) {
	g := &genOp{op: op, deps: deps}
	if beginning {
		a.generated[app] = append([]*genOp{g}, a.generated[app]...)
		return
	}
	a.generated[app] = append(a.generated[app], g)
}

// sortedApps returns the apps that have generated operations, sorted.
func (a *Autodetector) sortedApps() []string {
	return slices.Sorted(maps.Keys(a.generated))
}

// sortOperations reorders the operations of each app so that every operation
// comes after the operations of the same app it depends on. It is the
// algorithm of Python's graphlib.TopologicalSorter, which Django uses: nodes
// keep the order of their first mention and are emitted level by level, so
// the result matches Django's.
func (a *Autodetector) sortOperations() error {
	for _, app := range a.sortedApps() {
		ops := a.generated[app]
		type info struct {
			pending    int
			successors []*genOp
		}
		infos := map[*genOp]*info{}
		var order []*genOp
		node := func(g *genOp) *info {
			if in, ok := infos[g]; ok {
				return in
			}
			in := &info{}
			infos[g] = in
			order = append(order, g)
			return in
		}
		for _, g := range ops {
			node(g)
			for _, dep := range g.deps {
				if dep.app != app {
					continue
				}
				for _, x := range ops {
					if satisfies(x.op, dep) {
						node(g).pending++
						px := node(x)
						px.successors = append(px.successors, g)
					}
				}
			}
		}
		var ready []*genOp
		for _, g := range order {
			if infos[g].pending == 0 {
				ready = append(ready, g)
			}
		}
		sorted := make([]*genOp, 0, len(ops))
		for len(ready) > 0 {
			batch := ready
			ready = nil
			sorted = append(sorted, batch...)
			for _, g := range batch {
				for _, s := range infos[g].successors {
					infos[s].pending--
					if infos[s].pending == 0 {
						ready = append(ready, s)
					}
				}
			}
		}
		if len(sorted) != len(ops) {
			return fmt.Errorf("migrations: the generated operations of app %q depend on each other in a cycle", app)
		}
		a.generated[app] = sorted
	}
	return nil
}

// pendingOps returns the number of operations not yet put into a migration.
func (a *Autodetector) pendingOps() int {
	n := 0
	for _, ops := range a.generated {
		n += len(ops)
	}
	return n
}

// buildMigrationList cuts the operation lists into migrations that depend on
// each other. It goes through the operations of an app until one waits for an
// operation of another app that is not in a migration yet. When no app makes
// progress that way, two apps wait for each other; the operations that are
// ready are then cut off into a migration of their own, which is how an
// initial migration is split when two apps refer to each other.
func (a *Autodetector) buildMigrationList(graph *Graph) error {
	a.migrations = map[string][]*Migration{}
	pending := a.pendingOps()
	chopMode := false
	for pending > 0 {
		for _, app := range a.sortedApps() {
			var chopped []*genOp
			var dependencies []Key
			for _, g := range slices.Clone(a.generated[app]) {
				opDeps, ok := a.externalDependencies(app, g, graph, chopMode)
				if !ok {
					break
				}
				chopped = append(chopped, g)
				dependencies = append(dependencies, opDeps...)
				a.generated[app] = a.generated[app][1:]
			}
			if len(chopped) == 0 {
				continue
			}
			if len(a.generated[app]) > 0 && !chopMode {
				// Not everything is ready: put the operations back and wait.
				a.generated[app] = append(chopped, a.generated[app]...)
				continue
			}
			ops := make([]Operation, len(chopped))
			for i, g := range chopped {
				ops[i] = g.op
			}
			a.migrations[app] = append(a.migrations[app], &Migration{
				App:          app,
				Name:         "auto_" + strconv.Itoa(len(a.migrations[app])+1),
				Dependencies: dependencies,
				Operations:   ops,
				Initial:      !a.existingApps[app],
			})
			chopMode = false
		}
		now := a.pendingOps()
		if now == pending {
			if chopMode {
				return errors.New("migrations: cannot resolve the dependencies between the generated operations")
			}
			chopMode = true
		}
		pending = now
	}
	return nil
}

// externalDependencies returns the migrations of other apps that the
// operation g of app needs. ok is false while one of them is not available:
// the operation it waits for is not in a migration yet, or, outside chop
// mode, the other app has no new migration to point to.
func (a *Autodetector) externalDependencies(app string, g *genOp, graph *Graph, chopMode bool) (deps []Key, ok bool) {
	for _, dep := range g.deps {
		if dep.app == app {
			continue
		}
		for _, other := range a.generated[dep.app] {
			if satisfies(other.op, dep) {
				return nil, false
			}
		}
		switch migs := a.migrations[dep.app]; {
		case len(migs) > 0:
			deps = append(deps, Key{App: dep.app, Name: migs[len(migs)-1].Name})
		case chopMode:
			// The other app has no new migration. Depend on its last
			// existing one, because the migration that holds the target is
			// not known, or on its first one when it has none yet.
			if leaves := graph.LeafNodes(dep.app); len(leaves) > 0 {
				deps = append(deps, leaves[0])
			} else {
				deps = append(deps, Key{App: dep.app, Name: FirstMigration})
			}
		default:
			return nil, false
		}
	}
	return deps, true
}

// optimizeMigrations chains the migrations of each app, removes duplicate
// dependencies and optimizes the operations.
func (a *Autodetector) optimizeMigrations() {
	for app, migs := range a.migrations {
		for i := 1; i < len(migs); i++ {
			migs[i].Dependencies = append(migs[i].Dependencies, Key{App: app, Name: migs[i-1].Name})
		}
		for _, m := range migs {
			slices.SortFunc(m.Dependencies, compareKeys)
			m.Dependencies = slices.Compact(m.Dependencies)
			m.Operations = Optimize(m.Operations, app)
		}
	}
}
