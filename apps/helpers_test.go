package apps_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
)

// plainApp is an app config with a name and nothing else, the counterpart of
// an INSTALLED_APPS entry without an AppConfig subclass.
type plainApp struct{ name string }

// Name returns the app name.
func (a plainApp) Name() string { return a.name }

// labeledApp is an app config with an explicit label (AppConfig.label).
type labeledApp struct{ name, label string }

// Name returns the app name.
func (a labeledApp) Name() string { return a.name }

// Label returns the explicit label.
func (a labeledApp) Label() string { return a.label }

// node is an app of a dependency graph. It records every pass it takes part
// in as "<pass>:<name>" in the shared log, and fails a pass on request.
type node struct {
	name     string
	deps     []string
	log      *[]string
	wireErr  error
	readyErr error
	// ctx is the context of the last wire pass.
	ctx *apps.Context
}

// Name returns the app name.
func (n *node) Name() string { return n.name }

// Dependencies returns the declared dependencies.
func (n *node) Dependencies() []string { return n.deps }

// Models records the model collection of the register pass.
func (n *node) Models() []any {
	n.record("models")
	return nil
}

// Wire records the wire pass.
func (n *node) Wire(ctx *apps.Context) error {
	n.record("wire")
	n.ctx = ctx
	return n.wireErr
}

// Ready records the ready pass.
func (n *node) Ready() error {
	n.record("ready")
	return n.readyErr
}

// record appends the pass to the shared log, when there is one.
func (n *node) record(pass string) {
	if n.log != nil {
		*n.log = append(*n.log, pass+":"+n.name)
	}
}

// graph describes apps in registration order: the first element of each entry
// is the app name, the rest are its dependencies.
type graph [][]string

// nodes builds the apps of the graph, recording into log.
func (g graph) nodes(log *[]string) []*node {
	out := make([]*node, len(g))
	for i, entry := range g {
		out[i] = &node{name: entry[0], deps: entry[1:], log: log}
	}
	return out
}

// registry registers the apps of the graph and returns the registry.
func (g graph) registry(t *testing.T, log *[]string) *apps.Registry {
	t.Helper()
	reg := apps.NewRegistry()
	for _, n := range g.nodes(log) {
		if err := reg.Register(n); err != nil {
			t.Fatalf("Register(%s): %v", n.name, err)
		}
	}
	return reg
}

// newRegistry returns a registry holding the configs in the given order, the
// counterpart of settings(INSTALLED_APPS=[...]).
func newRegistry(t *testing.T, configs ...apps.Config) *apps.Registry {
	t.Helper()
	reg := apps.NewRegistry()
	if err := registerAll(reg, configs...); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return reg
}

// registerAll registers the configs in order and returns the first error, as
// Django stops at the first entry of INSTALLED_APPS it cannot load.
func registerAll(reg *apps.Registry, configs ...apps.Config) error {
	for _, c := range configs {
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// assertImproperlyConfigured asserts that err is a *conf.ImproperlyConfigured
// whose message contains msg, the counterpart of
// assertRaisesMessage(ImproperlyConfigured, msg).
func assertImproperlyConfigured(t *testing.T, err error, msg string) {
	t.Helper()
	var ic *conf.ImproperlyConfigured
	if !errors.As(err, &ic) {
		t.Fatalf("error = %v (%T), want a *conf.ImproperlyConfigured", err, err)
	}
	if !strings.Contains(ic.Msg, msg) {
		t.Errorf("message = %q, want it to contain %q", ic.Msg, msg)
	}
}

// configNames returns the names of the configs, in order.
func configNames(configs []apps.Config) []string {
	out := make([]string, len(configs))
	for i, c := range configs {
		out[i] = c.Name()
	}
	return out
}
