package apps_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/apps"
)

// acute is the combining acute accent, a mark that may follow the first
// character of an identifier and may not start one.
const acute = string(rune(0x0301))

// verboseApp is an app config with an explicit verbose name.
type verboseApp struct{ name, verbose string }

// Name returns the app name.
func (a verboseApp) Name() string { return a.name }

// VerboseName returns the explicit verbose name.
func (a verboseApp) VerboseName() string { return a.verbose }

func TestLabel(t *testing.T) {
	tests := []struct {
		config apps.Config
		want   string
	}{
		{plainApp{"blog"}, "blog"},
		{plainApp{"django.contrib.admin"}, "admin"},
		{plainApp{"github.com/acme/blog"}, "blog"},
		{plainApp{"example.com/shop.orders"}, "orders"},
		{plainApp{"acme.shop/orders"}, "orders"},
		{labeledApp{"django.contrib.auth", "myauth"}, "myauth"},
		// An empty label selects the default, so a config type with an optional
		// label field needs no second type.
		{labeledApp{"django.contrib.auth", ""}, "auth"},
		{myAuth{}, "myauth"},
		{myAdmin{}, "admin"},
	}
	for _, tt := range tests {
		if got := apps.Label(tt.config); got != tt.want {
			t.Errorf("Label(%#v) = %q, want %q", tt.config, got, tt.want)
		}
	}
}

// TestVerboseName checks the default against Python's str.title, which Django
// applies to the label; the expected values are the output of Python.
func TestVerboseName(t *testing.T) {
	tests := []struct {
		config apps.Config
		want   string
	}{
		{plainApp{"blog"}, "Blog"},
		{plainApp{"django.contrib.contenttypes"}, "Contenttypes"},
		{plainApp{"my_app"}, "My_App"},
		{plainApp{"myAuth"}, "Myauth"},
		{plainApp{"app2go"}, "App2Go"},
		{plainApp{"été"}, "Été"},
		{plainApp{"_x"}, "_X"},
		{labeledApp{"django.contrib.auth", "myauth"}, "Myauth"},
		{verboseApp{"blog", "Web log"}, "Web log"},
		{verboseApp{"blog", ""}, "Blog"},
		{myAdmin{}, "Admin sweet admin."},
		{myAuth{}, "All your password are belong to us."},
	}
	for _, tt := range tests {
		if got := apps.VerboseName(tt.config); got != tt.want {
			t.Errorf("VerboseName(%#v) = %q, want %q", tt.config, got, tt.want)
		}
	}
}

// TestRegisterValidatesLabel checks labels against Python's str.isidentifier,
// which Django applies; a keyword is an identifier for it.
func TestRegisterValidatesLabel(t *testing.T) {
	valid := []string{"blog", "_private", "app2", "my_app", "été", "日本語", "class", "a" + acute}
	for _, label := range valid {
		if err := apps.NewRegistry().Register(labeledApp{"some.app", label}); err != nil {
			t.Errorf("Register with the label %q: %v", label, err)
		}
	}

	invalid := []string{"2app", "my-app", "my app", "my.app", "my/app", "app!", acute + "a", "\xff"}
	for _, label := range invalid {
		err := apps.NewRegistry().Register(labeledApp{"some.app", label})
		assertImproperlyConfigured(t, err, "The app label '"+label+"' is not a valid Python identifier.")
	}
}

// TestRegisterValidatesDefaultLabel checks that the label derived from the
// name is validated like an explicit one.
func TestRegisterValidatesDefaultLabel(t *testing.T) {
	err := apps.NewRegistry().Register(plainApp{"github.com/acme/my-app"})
	assertImproperlyConfigured(t, err, "The app label 'my-app' is not a valid Python identifier.")

	// A name that ends in a separator has an empty last component.
	err = apps.NewRegistry().Register(plainApp{"github.com/acme/"})
	assertImproperlyConfigured(t, err, "The app label '' is not a valid Python identifier.")
}
