package apps_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/djangotest"
)

// startNames returns the names of the apps in start order, space-separated.
func startNames(reg *apps.Registry) string {
	return strings.Join(configNames(reg.StartOrder()), " ")
}

func TestStartOrder(t *testing.T) {
	tests := []struct {
		name  string
		graph graph
		want  string
	}{
		{"no apps", graph{}, ""},
		{"no dependencies keeps the registration order", graph{{"c"}, {"a"}, {"b"}}, "c a b"},
		{"dependencies listed first keep the registration order", graph{{"a"}, {"b", "a"}, {"c", "b"}}, "a b c"},
		{"a chain listed backwards is reversed", graph{{"c", "b"}, {"b", "a"}, {"a"}}, "a b c"},
		{"an app waits for a dependency listed later", graph{{"a", "c"}, {"b"}, {"c"}}, "b c a"},
		{"a waiting app starts as soon as it can", graph{{"x"}, {"y", "z"}, {"z"}, {"w"}}, "x z y w"},
		{"a diamond", graph{{"d", "b", "c"}, {"b", "a"}, {"c", "a"}, {"a"}}, "a b c d"},
		{"a diamond with the branches swapped", graph{{"d", "c", "b"}, {"c", "a"}, {"b", "a"}, {"a"}}, "a c b d"},
		{"independent apps tie-break by registration", graph{{"m", "z"}, {"k", "z"}, {"z"}, {"a"}}, "z m k a"},
		{"two chains interleave by registration", graph{{"b2", "b1"}, {"a2", "a1"}, {"a1"}, {"b1"}}, "a1 a2 b1 b2"},
		{"a repeated dependency counts once", graph{{"b", "a", "a"}, {"a"}}, "a b"},
		{"several dependencies", graph{{"site", "auth", "sessions", "contenttypes"}, {"sessions"}, {"auth", "contenttypes"}, {"contenttypes"}}, "sessions contenttypes auth site"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := tt.graph.registry(t, nil)
			if err := reg.Resolve(); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			djangotest.AssertEqual(t, startNames(reg), tt.want)
		})
	}
}

func TestStartOrderBeforeResolveIsRegistrationOrder(t *testing.T) {
	reg := graph{{"b", "a"}, {"a"}}.registry(t, nil)
	djangotest.AssertEqual(t, startNames(reg), "b a")
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	djangotest.AssertEqual(t, startNames(reg), "a b")
	// The installed order is not the start order.
	djangotest.AssertSequenceEqual(t, reg.Names(), []string{"b", "a"})
	djangotest.AssertSequenceEqual(t, configNames(reg.Configs()), []string{"b", "a"})
}

func TestStartOrderIsACopy(t *testing.T) {
	reg := graph{{"b", "a"}, {"a"}}.registry(t, nil)
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	order := reg.StartOrder()
	order[0], order[1] = order[1], order[0]
	djangotest.AssertEqual(t, startNames(reg), "a b")
}

func TestRegisterAfterResolveComputesTheOrderAgain(t *testing.T) {
	reg := graph{{"b", "a"}, {"a"}}.registry(t, nil)
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	djangotest.AssertEqual(t, startNames(reg), "a b")

	if err := reg.Register(&node{name: "y", deps: []string{"z"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(&node{name: "z"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// The order of the earlier Resolve no longer covers every app.
	djangotest.AssertEqual(t, startNames(reg), "b a y z")
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	djangotest.AssertEqual(t, startNames(reg), "a b z y")
}

func TestDependencyByLabelOrName(t *testing.T) {
	tests := []struct {
		name string
		dep  string
	}{
		{"by label", "myauth"},
		{"by name", "django.contrib.auth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newRegistry(t, &node{name: "blog", deps: []string{tt.dep}}, myAuth{})
			if err := reg.Resolve(); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			djangotest.AssertEqual(t, startNames(reg), "django.contrib.auth blog")
		})
	}
}

// TestDependencyPrefersTheLabel covers a reference that is the label of one
// app and the name of another.
func TestDependencyPrefersTheLabel(t *testing.T) {
	reg := newRegistry(t,
		&node{name: "blog", deps: []string{"auth"}},
		labeledApp{"auth", "legacy"},
		&node{name: "contrib.auth", deps: []string{"blog2"}},
		plainApp{"blog2"},
	)
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// "auth" is the label of contrib.auth, so blog waits for it and not only
	// for the app named auth.
	djangotest.AssertEqual(t, startNames(reg), "auth blog2 contrib.auth blog")
}

func TestMissingDependency(t *testing.T) {
	reg := graph{{"blog", "auth"}}.registry(t, nil)
	err := reg.Resolve()
	assertImproperlyConfigured(t, err, "App 'blog' depends on 'auth', which is not installed. Add it to the installed apps.")
	djangotest.AssertEqual(t, err.Error(), "App 'blog' depends on 'auth', which is not installed. Add it to the installed apps.")

	// Installing the dependency repairs the registry.
	if err := reg.Register(plainApp{"auth"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	djangotest.AssertEqual(t, startNames(reg), "auth blog")
}

// TestMissingDependencyNamesTheLabel covers an app whose label differs from
// its name: the message names the app by label and the dependency as declared.
func TestMissingDependencyNamesTheLabel(t *testing.T) {
	reg := newRegistry(t, &node{name: "github.com/acme/blog", deps: []string{"django.contrib.auth"}})
	err := reg.Resolve()
	assertImproperlyConfigured(t, err, "App 'blog' depends on 'django.contrib.auth', which is not installed. Add it to the installed apps.")
}

func TestCircularDependency(t *testing.T) {
	tests := []struct {
		name  string
		graph graph
		want  string
	}{
		{"two apps", graph{{"a", "b"}, {"b", "a"}}, "a -> b -> a"},
		{"an app that depends on itself", graph{{"a", "a"}}, "a -> a"},
		{"three apps", graph{{"a", "b"}, {"b", "c"}, {"c", "a"}}, "a -> b -> c -> a"},
		{"three apps against the registration order", graph{{"a", "c"}, {"b", "a"}, {"c", "b"}}, "a -> c -> b -> a"},
		{"an app outside the cycle depends on it", graph{{"a", "c"}, {"b", "c"}, {"c", "b"}}, "b -> c -> b"},
		{"started apps are left out", graph{{"ok"}, {"x", "ok", "y"}, {"y", "x"}}, "x -> y -> x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.graph.registry(t, nil).Resolve()
			assertImproperlyConfigured(t, err, "Circular dependency between apps: "+tt.want)
			djangotest.AssertEqual(t, err.Error(), "Circular dependency between apps: "+tt.want)
		})
	}
}
