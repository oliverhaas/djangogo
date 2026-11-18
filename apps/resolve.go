package apps

import (
	"slices"
	"strings"
)

// Resolve ends the first pass: it validates the declared dependencies and
// computes the start order. Every app starts after its dependencies; of the
// apps whose dependencies have all started, the one registered first starts
// next (a stable topological sort), so without dependencies the start order
// is the registration order.
//
// A dependency names an app by label or by name; the label wins when both
// match. The errors are of type *conf.ImproperlyConfigured: a dependency that
// is not installed ("App 'blog' depends on 'auth', which is not installed. Add
// it to the installed apps.") and a cycle ("Circular dependency between apps:
// a -> b -> a"). Both name apps by label; a cycle starts with its app that is
// registered first, and an arrow reads "depends on".
//
// Resolve may be called more than once; a registration after it makes the
// next call compute the order again.
func (r *Registry) Resolve() error {
	if r.resolved {
		return nil
	}
	deps, err := r.dependencies()
	if err != nil {
		return err
	}
	order, err := r.sortByDependencies(deps)
	if err != nil {
		return err
	}
	start := make([]entry, len(order))
	for i, idx := range order {
		start[i] = r.entries[idx]
	}
	r.start, r.resolved = start, true
	return nil
}

// StartOrder returns the app configs in start order (after Resolve; before it,
// registration order).
func (r *Registry) StartOrder() []Config {
	return configsOf(r.inStartOrder())
}

// inStartOrder returns the entries in start order, or in registration order
// before Resolve. The result is shared: callers must not change it.
func (r *Registry) inStartOrder() []entry {
	if r.resolved {
		return r.start
	}
	return r.entries
}

// dependencies returns, for every app, the indexes of the apps it depends on,
// in declared order and without repeats.
func (r *Registry) dependencies() ([][]int, error) {
	deps := make([][]int, len(r.entries))
	for i, e := range r.entries {
		provider, ok := e.config.(DependencyProvider)
		if !ok {
			continue
		}
		for _, ref := range provider.Dependencies() {
			dep, ok := r.lookup(ref)
			if !ok {
				return nil, improperlyConfigured(
					"App '%s' depends on '%s', which is not installed. Add it to the installed apps.",
					e.label, ref,
				)
			}
			if !slices.Contains(deps[i], dep) {
				deps[i] = append(deps[i], dep)
			}
		}
	}
	return deps, nil
}

// lookup returns the index of the app that ref names, by label or, failing
// that, by name.
func (r *Registry) lookup(ref string) (int, bool) {
	if i, ok := r.byLabel[ref]; ok {
		return i, true
	}
	i, ok := r.byName[ref]
	return i, ok
}

// sortByDependencies returns the indexes of the apps in start order: at each
// step the first app in registration order whose dependencies have all
// started. It reports a cycle when no remaining app can start.
func (r *Registry) sortByDependencies(deps [][]int) ([]int, error) {
	order := make([]int, 0, len(deps))
	started := make([]bool, len(deps))
	for len(order) < len(deps) {
		next := nextStartable(deps, started)
		if next < 0 {
			return nil, r.cycleError(findCycle(deps, started))
		}
		started[next] = true
		order = append(order, next)
	}
	return order, nil
}

// nextStartable returns the first app in registration order that has not
// started and whose dependencies all have, or -1 when a cycle blocks every
// remaining app.
func nextStartable(deps [][]int, started []bool) int {
	blocked := func(dep int) bool { return !started[dep] }
	for i, appDeps := range deps {
		if !started[i] && !slices.ContainsFunc(appDeps, blocked) {
			return i
		}
	}
	return -1
}

// findCycle returns the apps of one dependency cycle among the apps that
// cannot start, each followed by an app it depends on and the last one
// depending on the first. It walks from the first blocked app along blocked
// dependencies until an app repeats; every blocked app has a blocked
// dependency, so the walk always closes a loop.
func findCycle(deps [][]int, started []bool) []int {
	blocked := func(dep int) bool { return !started[dep] }
	seenAt := make(map[int]int)
	var walk []int
	app := slices.Index(started, false)
	for {
		if at, seen := seenAt[app]; seen {
			return walk[at:]
		}
		seenAt[app] = len(walk)
		walk = append(walk, app)
		app = deps[app][slices.IndexFunc(deps[app], blocked)]
	}
}

// cycleError describes a dependency cycle by the labels of its apps. The cycle
// is rotated to start with the app registered first, so the message does not
// depend on where the search entered the cycle.
func (r *Registry) cycleError(cycle []int) error {
	first := slices.Index(cycle, slices.Min(cycle))
	labels := make([]string, 0, len(cycle)+1)
	for i := range cycle {
		labels = append(labels, r.entries[cycle[(first+i)%len(cycle)]].label)
	}
	labels = append(labels, labels[0])
	return improperlyConfigured("Circular dependency between apps: %s", strings.Join(labels, " -> "))
}
