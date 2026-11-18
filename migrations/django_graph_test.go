package migrations_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/migrations"
)

// k builds a migration key, the tuple ("app", "name") of the Django tests.
func k(app, name string) migrations.Key { return migrations.Key{App: app, Name: name} }

// graphOf builds a graph from node keys and (child, parent) dependency pairs.
// The declaring migration of a dependency is its child, as in the Django tests.
func graphOf(t *testing.T, nodes []migrations.Key, deps [][2]migrations.Key) *migrations.Graph {
	t.Helper()
	g := migrations.NewGraph()
	for _, n := range nodes {
		g.AddNode(n, nil)
	}
	for _, d := range deps {
		if err := g.AddDependency(d[0].String(), d[0], d[1], false); err != nil {
			t.Fatalf("AddDependency(%v, %v): %v", d[0], d[1], err)
		}
	}
	return g
}

// plan returns the forwards or backwards plan and fails the test on an error.
func plan(t *testing.T, fn func(migrations.Key) ([]migrations.Key, error), target migrations.Key) []migrations.Key {
	t.Helper()
	got, err := fn(target)
	if err != nil {
		t.Fatalf("plan(%v): %v", target, err)
	}
	return got
}

// assertNodeNotFound checks that err is a NodeNotFoundError with Django's message.
func assertNodeNotFound(t *testing.T, err error, msg string) {
	t.Helper()
	var nf *migrations.NodeNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error = %v, want NodeNotFoundError", err)
	}
	djangotest.AssertEqual(t, nf.Error(), msg)
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_simple_graph
func TestSimpleGraph(t *testing.T) {
	g := graphOf(t,
		[]migrations.Key{k("app_a", "0001"), k("app_a", "0002"), k("app_a", "0003"), k("app_a", "0004"), k("app_b", "0001"), k("app_b", "0002")},
		[][2]migrations.Key{
			{k("app_a", "0004"), k("app_a", "0003")},
			{k("app_a", "0003"), k("app_a", "0002")},
			{k("app_a", "0002"), k("app_a", "0001")},
			{k("app_a", "0003"), k("app_b", "0002")},
			{k("app_b", "0002"), k("app_b", "0001")},
		})
	djangotest.AssertEqual(t, plan(t, g.ForwardsPlan, k("app_a", "0001")), []migrations.Key{k("app_a", "0001")})
	djangotest.AssertEqual(t, plan(t, g.ForwardsPlan, k("app_b", "0002")), []migrations.Key{k("app_b", "0001"), k("app_b", "0002")})
	djangotest.AssertEqual(t, plan(t, g.ForwardsPlan, k("app_a", "0004")), []migrations.Key{
		k("app_b", "0001"), k("app_b", "0002"), k("app_a", "0001"), k("app_a", "0002"), k("app_a", "0003"), k("app_a", "0004"),
	})
	djangotest.AssertEqual(t, plan(t, g.BackwardsPlan, k("app_b", "0002")), []migrations.Key{
		k("app_a", "0004"), k("app_a", "0003"), k("app_b", "0002"),
	})
	djangotest.AssertEqual(t, g.RootNodes(), []migrations.Key{k("app_a", "0001"), k("app_b", "0001")})
	djangotest.AssertEqual(t, g.LeafNodes(), []migrations.Key{k("app_a", "0004"), k("app_b", "0002")})
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_complex_graph
func TestComplexGraph(t *testing.T) {
	g := graphOf(t,
		[]migrations.Key{
			k("app_a", "0001"), k("app_a", "0002"), k("app_a", "0003"), k("app_a", "0004"),
			k("app_b", "0001"), k("app_b", "0002"), k("app_c", "0001"), k("app_c", "0002"),
		},
		[][2]migrations.Key{
			{k("app_a", "0004"), k("app_a", "0003")},
			{k("app_a", "0003"), k("app_a", "0002")},
			{k("app_a", "0002"), k("app_a", "0001")},
			{k("app_a", "0003"), k("app_b", "0002")},
			{k("app_b", "0002"), k("app_b", "0001")},
			{k("app_a", "0004"), k("app_c", "0002")},
			{k("app_c", "0002"), k("app_c", "0001")},
			{k("app_c", "0001"), k("app_b", "0001")},
			{k("app_c", "0002"), k("app_a", "0002")},
		})
	djangotest.AssertEqual(t, plan(t, g.ForwardsPlan, k("app_c", "0002")), []migrations.Key{
		k("app_b", "0001"), k("app_c", "0001"), k("app_a", "0001"), k("app_a", "0002"), k("app_c", "0002"),
	})
	djangotest.AssertEqual(t, plan(t, g.ForwardsPlan, k("app_a", "0004")), []migrations.Key{
		k("app_b", "0001"), k("app_c", "0001"), k("app_a", "0001"), k("app_a", "0002"),
		k("app_c", "0002"), k("app_b", "0002"), k("app_a", "0003"), k("app_a", "0004"),
	})
	djangotest.AssertEqual(t, plan(t, g.BackwardsPlan, k("app_b", "0001")), []migrations.Key{
		k("app_a", "0004"), k("app_c", "0002"), k("app_c", "0001"), k("app_a", "0003"), k("app_b", "0002"), k("app_b", "0001"),
	})
	djangotest.AssertEqual(t, g.RootNodes(), []migrations.Key{k("app_a", "0001"), k("app_b", "0001"), k("app_c", "0001")})
	djangotest.AssertEqual(t, g.LeafNodes(), []migrations.Key{k("app_a", "0004"), k("app_b", "0002"), k("app_c", "0002")})
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_circular_graph
func TestCircularGraph(t *testing.T) {
	g := graphOf(t,
		[]migrations.Key{k("app_a", "0001"), k("app_a", "0002"), k("app_a", "0003"), k("app_b", "0001"), k("app_b", "0002")},
		[][2]migrations.Key{
			{k("app_a", "0003"), k("app_a", "0002")},
			{k("app_a", "0002"), k("app_a", "0001")},
			{k("app_a", "0001"), k("app_b", "0002")},
			{k("app_b", "0002"), k("app_b", "0001")},
			{k("app_b", "0001"), k("app_a", "0003")},
		})
	var cyc *migrations.CircularDependencyError
	if err := g.EnsureNotCyclic(); !errors.As(err, &cyc) {
		t.Fatalf("EnsureNotCyclic() = %v, want CircularDependencyError", err)
	}
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_circular_graph_2
func TestCircularGraph2(t *testing.T) {
	g := graphOf(t,
		[]migrations.Key{k("A", "0001"), k("C", "0001"), k("B", "0001")},
		[][2]migrations.Key{
			{k("A", "0001"), k("B", "0001")},
			{k("B", "0001"), k("A", "0001")},
			{k("C", "0001"), k("B", "0001")},
		})
	var cyc *migrations.CircularDependencyError
	if err := g.EnsureNotCyclic(); !errors.As(err, &cyc) {
		t.Fatalf("EnsureNotCyclic() = %v, want CircularDependencyError", err)
	}
	djangotest.AssertEqual(t, cyc.Error(), "A.0001, B.0001")
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_iterative_dfs
func TestIterativeDFS(t *testing.T) {
	g := migrations.NewGraph()
	root := k("app_a", "1")
	g.AddNode(root, nil)
	expected := []migrations.Key{root}
	for i := 2; i < 750; i++ {
		parent, child := k("app_a", strconv.Itoa(i-1)), k("app_a", strconv.Itoa(i))
		g.AddNode(child, nil)
		if err := g.AddDependency(strconv.Itoa(i), child, parent, false); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, child)
	}
	leaf := expected[len(expected)-1]
	djangotest.AssertEqual(t, plan(t, g.ForwardsPlan, leaf), expected)
	reversed := make([]migrations.Key, len(expected))
	for i, key := range expected {
		reversed[len(expected)-1-i] = key
	}
	djangotest.AssertEqual(t, plan(t, g.BackwardsPlan, root), reversed)
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_iterative_dfs_complexity
func TestIterativeDFSComplexity(t *testing.T) {
	const n = 50
	g := migrations.NewGraph()
	for i := 1; i <= n; i++ {
		for _, app := range []string{"app_a", "app_b", "app_c"} {
			g.AddNode(k(app, strconv.Itoa(i)), nil)
		}
	}
	for i := 1; i < n; i++ {
		cur, next := strconv.Itoa(i), strconv.Itoa(i+1)
		for _, d := range [][2]migrations.Key{
			{k("app_b", cur), k("app_a", cur)},
			{k("app_c", cur), k("app_a", cur)},
			{k("app_a", next), k("app_b", cur)},
			{k("app_a", next), k("app_c", cur)},
		} {
			if err := g.AddDependency("", d[0], d[1], false); err != nil {
				t.Fatal(err)
			}
		}
	}
	var expected []migrations.Key
	for i := 1; i < n; i++ {
		for _, app := range []string{"app_a", "app_c", "app_b"} {
			expected = append(expected, k(app, strconv.Itoa(i)))
		}
	}
	expected = append(expected, k("app_a", strconv.Itoa(n)))
	djangotest.AssertEqual(t, plan(t, g.ForwardsPlan, k("app_a", strconv.Itoa(n))), expected)
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_plan_invalid_node
func TestPlanInvalidNode(t *testing.T) {
	g := migrations.NewGraph()
	const msg = "Node ('app_b', '0001') not a valid node"
	_, err := g.ForwardsPlan(k("app_b", "0001"))
	assertNodeNotFound(t, err, msg)
	_, err = g.BackwardsPlan(k("app_b", "0001"))
	assertNodeNotFound(t, err, msg)
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_missing_parent_nodes
func TestMissingParentNodes(t *testing.T) {
	g := graphOf(t,
		[]migrations.Key{k("app_a", "0001"), k("app_a", "0002"), k("app_a", "0003"), k("app_b", "0001")},
		[][2]migrations.Key{
			{k("app_a", "0003"), k("app_a", "0002")},
			{k("app_a", "0002"), k("app_a", "0001")},
		})
	err := g.AddDependency("app_a.0001", k("app_a", "0001"), k("app_b", "0002"), false)
	assertNodeNotFound(t, err, "Migration app_a.0001 dependencies reference nonexistent parent node ('app_b', '0002')")
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_missing_child_nodes
func TestMissingChildNodes(t *testing.T) {
	g := graphOf(t, []migrations.Key{k("app_a", "0001")}, nil)
	err := g.AddDependency("app_a.0002", k("app_a", "0002"), k("app_a", "0001"), false)
	assertNodeNotFound(t, err, "Migration app_a.0002 dependencies reference nonexistent child node ('app_a', '0002')")
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_validate_consistency_missing_parent
func TestValidateConsistencyMissingParent(t *testing.T) {
	g := graphOf(t, []migrations.Key{k("app_a", "0001")}, nil)
	if err := g.AddDependency("app_a.0001", k("app_a", "0001"), k("app_b", "0002"), true); err != nil {
		t.Fatal(err)
	}
	assertNodeNotFound(t, g.ValidateConsistency(),
		"Migration app_a.0001 dependencies reference nonexistent parent node ('app_b', '0002')")
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_validate_consistency_missing_child
func TestValidateConsistencyMissingChild(t *testing.T) {
	g := graphOf(t, []migrations.Key{k("app_b", "0002")}, nil)
	if err := g.AddDependency("app_b.0002", k("app_a", "0001"), k("app_b", "0002"), true); err != nil {
		t.Fatal(err)
	}
	assertNodeNotFound(t, g.ValidateConsistency(),
		"Migration app_b.0002 dependencies reference nonexistent child node ('app_a', '0001')")
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_validate_consistency_no_error
func TestValidateConsistencyNoError(t *testing.T) {
	g := graphOf(t, []migrations.Key{k("app_a", "0001"), k("app_b", "0002")}, nil)
	if err := g.AddDependency("app_a.0001", k("app_a", "0001"), k("app_b", "0002"), true); err != nil {
		t.Fatal(err)
	}
	if err := g.ValidateConsistency(); err != nil {
		t.Fatalf("ValidateConsistency() = %v, want nil", err)
	}
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_validate_consistency_dummy
func TestValidateConsistencyDummy(t *testing.T) {
	const msg = "app_a.0001 (req'd by app_b.0002) is missing!"
	g := migrations.NewGraph()
	g.AddDummyNode(k("app_a", "0001"), "app_b.0002", msg)
	assertNodeNotFound(t, g.ValidateConsistency(), msg)
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_infinite_loop
func TestInfiniteLoop(t *testing.T) {
	g := graphOf(t,
		[]migrations.Key{k("app_a", "0001"), k("app_b", "0001"), k("app_b", "0002"), k("app_c", "0001_squashed_0002")},
		[][2]migrations.Key{
			{k("app_b", "0001"), k("app_c", "0001_squashed_0002")},
			{k("app_b", "0002"), k("app_a", "0001")},
			{k("app_b", "0002"), k("app_b", "0001")},
			{k("app_c", "0001_squashed_0002"), k("app_b", "0002")},
		})
	var cyc *migrations.CircularDependencyError
	if err := g.EnsureNotCyclic(); !errors.As(err, &cyc) {
		t.Fatalf("EnsureNotCyclic() = %v, want CircularDependencyError", err)
	}
}

// Ported from Django: tests/migrations/test_graph.py::GraphTests.test_stringify
//
// Django also checks repr(graph); Go has one textual form, String.
func TestStringify(t *testing.T) {
	g := migrations.NewGraph()
	djangotest.AssertEqual(t, g.String(), "Graph: 0 nodes, 0 edges")
	g = graphOf(t,
		[]migrations.Key{k("app_a", "0001"), k("app_a", "0002"), k("app_a", "0003"), k("app_b", "0001"), k("app_b", "0002")},
		[][2]migrations.Key{
			{k("app_a", "0002"), k("app_a", "0001")},
			{k("app_a", "0003"), k("app_a", "0002")},
			{k("app_a", "0003"), k("app_b", "0002")},
		})
	djangotest.AssertEqual(t, g.String(), "Graph: 5 nodes, 3 edges")
}
