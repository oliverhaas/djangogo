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
