package apps_test

import (
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/djangotest"
)

func TestLabels(t *testing.T) {
	reg := newRegistry(t, append(someInstalledApps(), plainApp{"github.com/acme/blog"})...)
	djangotest.AssertSequenceEqual(t, reg.Labels(), []string{
		"admin", "myauth", "contenttypes", "sessions", "messages", "staticfiles", "blog",
	})
}

func TestGetIsByName(t *testing.T) {
	reg := newRegistry(t, someInstalledApps()...)

	got, ok := reg.Get("django.contrib.auth")
	if !ok {
		t.Fatal(`Get("django.contrib.auth") found no app`)
	}
	djangotest.AssertEqual[apps.Config](t, got, myAuth{})

	// A label is not a name.
	_, ok = reg.Get("myauth")
	djangotest.AssertEqual(t, ok, false)
	djangotest.AssertEqual(t, reg.IsInstalled("myauth"), false)
	djangotest.AssertEqual(t, reg.IsInstalled("admin"), false)
}

func TestByLabelReturnsTheConfig(t *testing.T) {
	reg := newRegistry(t, someInstalledApps()...)
	got, err := reg.ByLabel("myauth")
	if err != nil {
		t.Fatalf("ByLabel(myauth): %v", err)
	}
	djangotest.AssertEqual[apps.Config](t, got, myAuth{})
}

// TestByLabelSuggestion covers the hint of the lookup error. Django suggests
// a label when the lookup used the name of an installed app; the hint is also
// given for the last component of a name, which is the label the app would
// have without its own.
func TestByLabelSuggestion(t *testing.T) {
	reg := newRegistry(t,
		myAdmin{},
		myAuth{},
		labeledApp{"github.com/acme/shop", "store"},
		plainApp{"flatpages"},
	)
	tests := []struct {
		label string
		want  string
	}{
		{"django.contrib.auth", "No installed app with label 'django.contrib.auth'. Did you mean 'myauth'?"},
		{"django.contrib.admin", "No installed app with label 'django.contrib.admin'. Did you mean 'admin'?"},
		{"auth", "No installed app with label 'auth'. Did you mean 'myauth'?"},
		{"contrib.auth", "No installed app with label 'contrib.auth'. Did you mean 'myauth'?"},
		{"shop", "No installed app with label 'shop'. Did you mean 'store'?"},
		{"github.com/acme/shop", "No installed app with label 'github.com/acme/shop'. Did you mean 'store'?"},
		{"admindocs", "No installed app with label 'admindocs'."},
		{"uth", "No installed app with label 'uth'."},
		{"Admin", "No installed app with label 'Admin'."},
		{"", "No installed app with label ''."},
	}
	for _, tt := range tests {
		config, err := reg.ByLabel(tt.label)
		if err == nil {
			t.Errorf("ByLabel(%q) = %v, want an error", tt.label, config)
			continue
		}
		djangotest.AssertEqual(t, err.Error(), tt.want)
		var lookup *apps.LookupError
		if !errors.As(err, &lookup) {
			t.Errorf("ByLabel(%q) error is a %T, want a *apps.LookupError", tt.label, err)
			continue
		}
		djangotest.AssertEqual(t, lookup.Label, tt.label)
	}
}

// TestRegisterReadsNameAndLabelOnce covers a config whose name changes after
// registration: the registry keeps what it validated.
func TestRegisterReadsNameAndLabelOnce(t *testing.T) {
	app := &node{name: "blog"}
	reg := newRegistry(t, app)
	app.name = "renamed"

	djangotest.AssertSequenceEqual(t, reg.Names(), []string{"blog"})
	djangotest.AssertSequenceEqual(t, reg.Labels(), []string{"blog"})
	djangotest.AssertEqual(t, reg.IsInstalled("blog"), true)
	djangotest.AssertEqual(t, reg.IsInstalled("renamed"), false)
}
