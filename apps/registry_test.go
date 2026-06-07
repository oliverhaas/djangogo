// apps/registry_test.go
package apps

import "testing"

type fakeApp struct{ name string }

func (f fakeApp) Name() string { return f.name }

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeApp{"blog"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(fakeApp{"shop"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := r.Get("blog")
	if !ok || got.Name() != "blog" {
		t.Fatalf("Get(blog) = %v, %v", got, ok)
	}

	want := []string{"blog", "shop"} // registration order preserved
	names := r.Names()
	if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("Names() = %v, want %v", names, want)
	}
}

func TestRegistryDuplicate(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(fakeApp{"blog"})
	if err := r.Register(fakeApp{"blog"}); err == nil {
		t.Error("duplicate app name should error")
	}
}

type readyApp struct {
	name string
	log  *[]string
}

func (a readyApp) Name() string { return a.name }
func (a readyApp) Ready() error { *a.log = append(*a.log, a.name); return nil }

type modelApp struct {
	name   string
	models []any
}

func (a modelApp) Name() string  { return a.name }
func (a modelApp) Models() []any { return a.models }

func TestModelProviderAssertion(t *testing.T) {
	var c Config = modelApp{name: "blog", models: []any{struct{}{}}}

	mp, ok := c.(ModelProvider)
	if !ok {
		t.Fatal("modelApp should satisfy ModelProvider")
	}
	if len(mp.Models()) != 1 {
		t.Errorf("Models() length = %d, want 1", len(mp.Models()))
	}

	var plain Config = fakeApp{"plain"}
	if _, ok := plain.(ModelProvider); ok {
		t.Error("fakeApp should not satisfy ModelProvider")
	}
}

func TestRegistryReadyOrder(t *testing.T) {
	var log []string
	r := NewRegistry()
	_ = r.Register(readyApp{"first", &log})
	_ = r.Register(fakeApp{"plain"}) // no Ready(), must be skipped without error
	_ = r.Register(readyApp{"second", &log})

	if err := r.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if len(log) != 2 || log[0] != "first" || log[1] != "second" {
		t.Errorf("Ready order = %v, want [first second]", log)
	}
}
