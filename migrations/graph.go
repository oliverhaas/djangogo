package migrations

import (
	"fmt"
	"slices"
	"strings"
)

// Key identifies a migration by app label and migration name, the pair Django
// writes as ("blog", "0001_initial").
type Key struct {
	App  string
	Name string
}

// String returns "app.name", the form Django prints for a migration.
func (k Key) String() string { return k.App + "." + k.Name }

// repr renders the key the way Python prints the tuple, for messages that
// reproduce Django's text.
func (k Key) repr() string { return fmt.Sprintf("('%s', '%s')", k.App, k.Name) }

// compareKeys orders keys by app, then name.
func compareKeys(a, b Key) int {
	if c := strings.Compare(a.App, b.App); c != 0 {
		return c
	}
	return strings.Compare(a.Name, b.Name)
}

// graphNode is one migration in the graph with its edges. A dummy node stands
// for a migration that a dependency names but that was never added.
type graphNode struct {
	key      Key
	parents  map[Key]*graphNode
	children map[Key]*graphNode
	dummy    bool
	origin   string
	message  string
}

// sortedKeys returns the keys of set in ascending order.
func sortedKeys(set map[Key]*graphNode) []Key {
	keys := make([]Key, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareKeys)
	return keys
}

// Graph is the directed acyclic graph of all migrations of a project, the
// counterpart of Django's MigrationGraph. A node is a migration; an edge leads
// from a migration to each migration it depends on. The graph answers which
// migrations must run, and in which order, to reach a target.
type Graph struct {
	nodes      map[Key]*graphNode
	migrations map[Key]*Migration
}

// NewGraph returns an empty graph.
func NewGraph() *Graph {
	return &Graph{nodes: map[Key]*graphNode{}, migrations: map[Key]*Migration{}}
}

// AddNode adds the migration m under key. The migration may be nil when only
// the shape of the graph matters.
func (g *Graph) AddNode(key Key, m *Migration) {
	g.nodes[key] = &graphNode{key: key, parents: map[Key]*graphNode{}, children: map[Key]*graphNode{}}
	g.migrations[key] = m
}

// AddDummyNode adds a placeholder for a missing migration. ValidateConsistency
// fails with message while the placeholder is in the graph.
func (g *Graph) AddDummyNode(key Key, origin, message string) {
	g.nodes[key] = &graphNode{
		key: key, parents: map[Key]*graphNode{}, children: map[Key]*graphNode{},
		dummy: true, origin: origin, message: message,
	}
	g.migrations[key] = nil
}

// AddDependency records that child depends on parent. The name of the
// migration that declares the dependency goes into the error message for a
// missing node. With skipValidation the missing node becomes a placeholder and
// the caller runs ValidateConsistency after all nodes are added.
func (g *Graph) AddDependency(migration string, child, parent Key, skipValidation bool) error {
	if _, ok := g.nodes[child]; !ok {
		g.AddDummyNode(child, migration, fmt.Sprintf(
			"Migration %s dependencies reference nonexistent child node %s", migration, child.repr()))
	}
	if _, ok := g.nodes[parent]; !ok {
		g.AddDummyNode(parent, migration, fmt.Sprintf(
			"Migration %s dependencies reference nonexistent parent node %s", migration, parent.repr()))
	}
	g.nodes[child].parents[parent] = g.nodes[parent]
	g.nodes[parent].children[child] = g.nodes[child]
	if skipValidation {
		return nil
	}
	return g.ValidateConsistency()
}

// ValidateConsistency fails with a NodeNotFoundError when a placeholder for a
// missing migration is left in the graph.
func (g *Graph) ValidateConsistency() error {
	for _, key := range sortedKeys(g.nodes) {
		if n := g.nodes[key]; n.dummy {
			return &NodeNotFoundError{Message: n.message, Node: n.key, Origin: n.origin}
		}
	}
	return nil
}

// Contains reports whether the graph has a node for key.
func (g *Graph) Contains(key Key) bool {
	_, ok := g.nodes[key]
	return ok
}

// Migration returns the migration stored under key, or nil.
func (g *Graph) Migration(key Key) *Migration { return g.migrations[key] }

// Keys returns all node keys in ascending order.
func (g *Graph) Keys() []Key { return sortedKeys(g.nodes) }

// Parents returns the keys the migration under key depends on, in ascending order.
func (g *Graph) Parents(key Key) []Key {
	n, ok := g.nodes[key]
	if !ok {
		return nil
	}
	return sortedKeys(n.parents)
}

// Children returns the keys of the migrations that depend on key, in ascending order.
func (g *Graph) Children(key Key) []Key {
	n, ok := g.nodes[key]
	if !ok {
		return nil
	}
	return sortedKeys(n.children)
}

// ForwardsPlan returns the migrations to apply, in order, to reach target from
// an empty database: all its dependencies first, then target itself.
func (g *Graph) ForwardsPlan(target Key) ([]Key, error) {
	n, ok := g.nodes[target]
	if !ok {
		return nil, &NodeNotFoundError{Message: fmt.Sprintf("Node %s not a valid node", target.repr()), Node: target}
	}
	return g.iterativeDFS(n, true), nil
}

// BackwardsPlan returns the migrations to unapply, in order, to remove target
// from a fully migrated database: everything that depends on it first, then
// target itself.
func (g *Graph) BackwardsPlan(target Key) ([]Key, error) {
	n, ok := g.nodes[target]
	if !ok {
		return nil, &NodeNotFoundError{Message: fmt.Sprintf("Node %s not a valid node", target.repr()), Node: target}
	}
	return g.iterativeDFS(n, false), nil
}

// iterativeDFS is a post-order depth-first walk without recursion, so a long
// migration history cannot exhaust the stack. Each node is emitted once.
func (g *Graph) iterativeDFS(start *graphNode, forwards bool) []Key {
	type frame struct {
		node      *graphNode
		processed bool
	}
	var visited []Key
	seen := map[Key]bool{}
	stack := []frame{{node: start}}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch {
		case seen[top.node.key]:
		case top.processed:
			seen[top.node.key] = true
			visited = append(visited, top.node.key)
		default:
			stack = append(stack, frame{node: top.node, processed: true})
			next := top.node.parents
			if !forwards {
				next = top.node.children
			}
			for _, k := range sortedKeys(next) {
				stack = append(stack, frame{node: next[k]})
			}
		}
	}
	return visited
}

// RootNodes returns the migrations that depend on no other migration of their
// own app, in ascending order. With an app label, only that app's roots.
func (g *Graph) RootNodes(app ...string) []Key {
	return g.edgeNodes(app, func(n *graphNode) map[Key]*graphNode { return n.parents })
}

// LeafNodes returns the migrations no other migration of their own app depends
// on, in ascending order. With an app label, only that app's leaves. More than
// one leaf in an app means a conflict that needs a merge migration.
func (g *Graph) LeafNodes(app ...string) []Key {
	return g.edgeNodes(app, func(n *graphNode) map[Key]*graphNode { return n.children })
}

// edgeNodes collects the nodes with no neighbor (parent or child, as chosen by
// side) inside their own app.
func (g *Graph) edgeNodes(app []string, side func(*graphNode) map[Key]*graphNode) []Key {
	var out []Key
	for key, n := range g.nodes {
		if len(app) > 0 && app[0] != "" && app[0] != key.App {
			continue
		}
		edge := true
		for other := range side(n) {
			if other.App == key.App {
				edge = false
				break
			}
		}
		if edge {
			out = append(out, key)
		}
	}
	slices.SortFunc(out, compareKeys)
	return out
}

// EnsureNotCyclic fails with a CircularDependencyError when the dependencies
// form a cycle.
func (g *Graph) EnsureNotCyclic() error {
	todo := map[Key]bool{}
	for k := range g.nodes {
		todo[k] = true
	}
	for _, start := range sortedKeys(g.nodes) {
		if !todo[start] {
			continue
		}
		delete(todo, start)
		stack := []Key{start}
		for len(stack) > 0 {
			top := stack[len(stack)-1]
			descended := false
			for _, child := range sortedKeys(g.nodes[top].children) {
				if i := slices.Index(stack, child); i >= 0 {
					return &CircularDependencyError{Cycle: slices.Clone(stack[i:])}
				}
				if todo[child] {
					stack = append(stack, child)
					delete(todo, child)
					descended = true
					break
				}
			}
			if !descended {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return nil
}

// String summarizes the graph as Django does: "Graph: 5 nodes, 3 edges".
func (g *Graph) String() string {
	edges := 0
	for _, n := range g.nodes {
		edges += len(n.parents)
	}
	return fmt.Sprintf("Graph: %d nodes, %d edges", len(g.nodes), edges)
}

// generatePlan returns the migrations needed to reach every node of targets,
// in order and without duplicates. With atEnd false the targets themselves are
// left out, which yields the state right before them.
func (g *Graph) generatePlan(targets []Key, atEnd bool) ([]Key, error) {
	var plan []Key
	inPlan := map[Key]bool{}
	for _, target := range targets {
		steps, err := g.ForwardsPlan(target)
		if err != nil {
			return nil, err
		}
		for _, k := range steps {
			if inPlan[k] || (!atEnd && slices.Contains(targets, k)) {
				continue
			}
			inPlan[k] = true
			plan = append(plan, k)
		}
	}
	return plan, nil
}

// MakeState returns the project state after the given migrations and all
// their dependencies are applied. With no targets it uses every leaf node,
// which is the state of a fully migrated project. With atEnd false it returns
// the state right before the targets.
func (g *Graph) MakeState(targets []Key, atEnd bool) (*ProjectState, error) {
	if targets == nil {
		targets = g.LeafNodes()
	}
	state := NewProjectState()
	plan, err := g.generatePlan(targets, atEnd)
	if err != nil {
		return nil, err
	}
	for _, k := range plan {
		m := g.migrations[k]
		if m == nil {
			continue
		}
		if err := m.MutateState(state); err != nil {
			return nil, err
		}
	}
	return state, nil
}
