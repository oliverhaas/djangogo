package apps_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// errBoom is the error a failing hook returns.
var errBoom = errors.New("boom")

// reentrantApp calls Ready of its registry from its own Ready hook.
type reentrantApp struct {
	reg *apps.Registry
	// inner is the result of the nested call.
	inner error
}

// Name returns the app name.
func (a *reentrantApp) Name() string { return "reentrant" }

// Ready calls the registry again, as a hook must not.
func (a *reentrantApp) Ready() error {
	a.inner = a.reg.Ready()
	return nil
}

// TestThreePasses runs the passes as the root package does and checks that
// each one visits the apps in start order: the models are collected for
// registration, then the apps are wired, then readied.
func TestThreePasses(t *testing.T) {
	var log []string
	reg := graph{{"blog", "auth"}, {"auth", "contenttypes"}, {"flatpages"}, {"contenttypes"}}.registry(t, &log)
	if len(log) != 0 {
		t.Fatalf("registration called hooks: %v", log)
	}

	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	models := reg.Models()
	labels := make([]string, len(models))
	for i, m := range models {
		labels[i] = m.App
	}
	if err := reg.Wire(&apps.Context{}); err != nil {
		t.Fatalf("Wire: %v", err)
	}
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	djangotest.AssertSequenceEqual(t, labels, []string{"flatpages", "contenttypes", "auth", "blog"})
	djangotest.AssertSequenceEqual(t, log, []string{
		"models:flatpages", "models:contenttypes", "models:auth", "models:blog",
		"wire:flatpages", "wire:contenttypes", "wire:auth", "wire:blog",
		"ready:flatpages", "ready:contenttypes", "ready:auth", "ready:blog",
	})
}

// TestPassesSkipAppsWithoutTheHook mixes apps that implement no optional
// interface with apps that do.
func TestPassesSkipAppsWithoutTheHook(t *testing.T) {
	var log []string
	reg := newRegistry(t,
		plainApp{"first"},
		&node{name: "second", log: &log},
		labeledApp{"django.contrib.third", "third"},
	)
	if err := reg.Wire(nil); err != nil {
		t.Fatalf("Wire: %v", err)
	}
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	djangotest.AssertSequenceEqual(t, log, []string{"wire:second", "ready:second"})
}

func TestWirePassesTheContext(t *testing.T) {
	app := &node{name: "blog"}
	reg := newRegistry(t, app)
	ctx := &apps.Context{Settings: &conf.Settings{SecretKey: "secret"}}
	if err := reg.Wire(ctx); err != nil {
		t.Fatalf("Wire: %v", err)
	}
	if app.ctx != ctx {
		t.Errorf("Wire got the context %p, want %p", app.ctx, ctx)
	}

	// A nil context reaches the app as an empty one.
	if err := reg.Wire(nil); err != nil {
		t.Fatalf("Wire(nil): %v", err)
	}
	if app.ctx == nil {
		t.Fatal("Wire(nil) handed the app a nil context")
	}
	djangotest.AssertEqual(t, *app.ctx, apps.Context{})
}

func TestWireResolvesFirst(t *testing.T) {
	var log []string
	reg := graph{{"blog", "auth"}, {"auth"}}.registry(t, &log)
	if err := reg.Wire(&apps.Context{}); err != nil {
		t.Fatalf("Wire: %v", err)
	}
	djangotest.AssertSequenceEqual(t, log, []string{"wire:auth", "wire:blog"})

	log = nil
	reg = graph{{"blog", "auth"}}.registry(t, &log)
	err := reg.Wire(&apps.Context{})
	assertImproperlyConfigured(t, err, "App 'blog' depends on 'auth', which is not installed.")
	if len(log) != 0 {
		t.Errorf("Wire ran hooks of an unresolved registry: %v", log)
	}
}

func TestFailingWire(t *testing.T) {
	var log []string
	first := &node{name: "first", log: &log}
	failing := &node{name: "shop.orders", log: &log, wireErr: errBoom}
	last := &node{name: "last", log: &log}
	reg := newRegistry(t, first, failing, last)

	err := reg.Wire(&apps.Context{})
	if err == nil {
		t.Fatal("Wire = nil, want the error of the app")
	}
	// The error names the app by its name, not by its label "orders".
	djangotest.AssertEqual(t, err.Error(), "apps: shop.orders.Wire: boom")
	djangotest.AssertErrorIs(t, err, errBoom)
	// The pass stops at the failing app.
	djangotest.AssertSequenceEqual(t, log, []string{"wire:first", "wire:shop.orders"})
	djangotest.AssertEqual(t, reg.IsReady(), false)
}

func TestFailingReady(t *testing.T) {
	var log []string
	first := &node{name: "first", log: &log}
	failing := &node{name: "shop.orders", log: &log, readyErr: errBoom}
	last := &node{name: "last", log: &log}
	reg := newRegistry(t, first, failing, last)

	err := reg.Ready()
	if err == nil {
		t.Fatal("Ready = nil, want the error of the app")
	}
	djangotest.AssertEqual(t, err.Error(), "apps: shop.orders.Ready: boom")
	djangotest.AssertErrorIs(t, err, errBoom)
	djangotest.AssertSequenceEqual(t, log, []string{"ready:first", "ready:shop.orders"})
	djangotest.AssertEqual(t, reg.IsReady(), false)

	// A second call reports the failure again and runs no hook twice.
	again := reg.Ready()
	djangotest.AssertErrorIs(t, again, errBoom)
	djangotest.AssertSequenceEqual(t, log, []string{"ready:first", "ready:shop.orders"})
	djangotest.AssertEqual(t, reg.IsReady(), false)
}

func TestReadyRunsOnce(t *testing.T) {
	var log []string
	reg := graph{{"a"}, {"b"}}.registry(t, &log)
	for range 3 {
		if err := reg.Ready(); err != nil {
			t.Fatalf("Ready: %v", err)
		}
	}
	djangotest.AssertSequenceEqual(t, log, []string{"ready:a", "ready:b"})
	djangotest.AssertEqual(t, reg.IsReady(), true)
}

func TestReadyResolvesFirst(t *testing.T) {
	var log []string
	reg := graph{{"blog", "auth"}, {"auth"}}.registry(t, &log)
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	djangotest.AssertSequenceEqual(t, log, []string{"ready:auth", "ready:blog"})
	djangotest.AssertEqual(t, startNames(reg), "auth blog")
}

// TestReadyWithUnresolvableApps checks that a registry whose dependencies do
// not resolve runs no hook and can still be repaired.
func TestReadyWithUnresolvableApps(t *testing.T) {
	var log []string
	reg := graph{{"blog", "auth"}}.registry(t, &log)
	err := reg.Ready()
	assertImproperlyConfigured(t, err, "App 'blog' depends on 'auth', which is not installed.")
	djangotest.AssertEqual(t, reg.IsReady(), false)
	if len(log) != 0 {
		t.Errorf("Ready ran hooks of an unresolved registry: %v", log)
	}

	if err := reg.Register(&node{name: "auth", log: &log}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	djangotest.AssertSequenceEqual(t, log, []string{"ready:auth", "ready:blog"})
	djangotest.AssertEqual(t, reg.IsReady(), true)
}

func TestReadyIsNotReentrant(t *testing.T) {
	reg := apps.NewRegistry()
	app := &reentrantApp{reg: reg}
	if err := reg.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if app.inner == nil || !strings.Contains(app.inner.Error(), "reentrant") {
		t.Errorf("Ready from inside a Ready hook = %v, want the reentrancy error", app.inner)
	}
	djangotest.AssertEqual(t, reg.IsReady(), true)
}

func TestRegisterAfterReady(t *testing.T) {
	reg := newRegistry(t, plainApp{"blog"})
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if err := reg.Register(plainApp{"late"}); err == nil {
		t.Error("Register after Ready = nil, want an error")
	}
	djangotest.AssertSequenceEqual(t, reg.Names(), []string{"blog"})
	djangotest.AssertEqual(t, reg.IsInstalled("late"), false)
}

func TestWireAfterReady(t *testing.T) {
	var log []string
	reg := graph{{"blog"}}.registry(t, &log)
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if err := reg.Wire(&apps.Context{}); err == nil {
		t.Error("Wire after Ready = nil, want an error")
	}
	djangotest.AssertSequenceEqual(t, log, []string{"ready:blog"})
}

// TestConcurrentReadsAfterReady reads a ready registry from several
// goroutines; the race detector reports a read that writes.
func TestConcurrentReadsAfterReady(t *testing.T) {
	reg := newRegistry(t, append(someInstalledApps(), fullApp{}, &node{name: "blog", deps: []string{"full"}})...)
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				_ = reg.Names()
				_ = reg.Labels()
				_ = reg.Configs()
				_ = reg.StartOrder()
				_ = reg.IsReady()
				_ = reg.IsInstalled("django.contrib.admin")
				_, _ = reg.Get("django.contrib.admin")
				_, _ = reg.ByLabel("myauth")
				_, _ = reg.ByLabel("nope")
				_ = reg.TemplateFS()
				_ = reg.StaticFS()
				_ = reg.Commands()
				_ = reg.Checks()
				_ = reg.Models()
				_ = reg.Routes()
				_ = reg.Resolve()
				_ = reg.Ready()
			}
		})
	}
	wg.Wait()
}
