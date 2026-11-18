package apps_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file are ports of Django's tests/apps/tests.py. Django
// installs an app by its dotted path and imports its modules; here an
// installed app is a Go value handed to Registry.Register. The tests of what
// follows from the import (finding the AppConfig class of a module, the
// filesystem path of an app, namespace packages, the model cache, lazy model
// operations, queries during app loading) have no counterpart and are not
// ported.

// myAdmin ports apps.apps.MyAdmin. It owns LogEntry, as django.contrib.admin
// does.
type myAdmin struct{}

// Name returns the dotted name of the app.
func (myAdmin) Name() string { return "django.contrib.admin" }

// VerboseName returns the human-readable name.
func (myAdmin) VerboseName() string { return "Admin sweet admin." }

// Models returns the models of the app.
func (myAdmin) Models() []any { return []any{&LogEntry{}} }

// LogEntry stands in for django.contrib.admin.models.LogEntry.
type LogEntry struct {
	ID         int64
	ObjectRepr string `orm:"max_length=200"`
}

// myAuth ports apps.apps.MyAuth.
type myAuth struct{}

// Name returns the dotted name of the app.
func (myAuth) Name() string { return "django.contrib.auth" }

// Label returns the label that replaces the default "auth".
func (myAuth) Label() string { return "myauth" }

// VerboseName returns the human-readable name.
func (myAuth) VerboseName() string { return "All your password are belong to us." }

// badConfig ports apps.apps.BadConfig: it does not supply the mandatory name.
type badConfig struct{}

// Name returns no name.
func (badConfig) Name() string { return "" }

// noSuchApp ports apps.apps.NoSuchApp.
type noSuchApp struct{}

// Name returns a name that is not the path of an app.
func (noSuchApp) Name() string { return "there is no such app" }

// plainAppsConfig ports apps.apps.PlainAppsConfig. It owns TotallyNormal, the
// model of tests/apps/models.py.
type plainAppsConfig struct{}

// Name returns the app name.
func (plainAppsConfig) Name() string { return "apps" }

// Models returns the models of the app.
func (plainAppsConfig) Models() []any { return []any{&TotallyNormal{}} }

// relabeledAppsConfig ports apps.apps.RelabeledAppsConfig.
type relabeledAppsConfig struct{}

// Name returns the app name.
func (relabeledAppsConfig) Name() string { return "apps" }

// Label returns the label that replaces the default "apps".
func (relabeledAppsConfig) Label() string { return "relabeled" }

// alternativeAppsConfig is the app "apps" of the second registry of
// tests/apps/models.py (new_apps), which holds SoAlternative.
type alternativeAppsConfig struct{}

// Name returns the app name.
func (alternativeAppsConfig) Name() string { return "apps" }

// Models returns the models of the app.
func (alternativeAppsConfig) Models() []any { return []any{&SoAlternative{}} }

// TotallyNormal ports the model of the same name in tests/apps/models.py.
type TotallyNormal struct {
	ID   int64
	Name string `orm:"max_length=255"`
}

// SoAlternative ports the model of the same name in tests/apps/models.py,
// which belongs to a registry of its own.
type SoAlternative struct {
	ID   int64
	Name string `orm:"max_length=255"`
}

// invalidLabelConfig ports the MyAppConfig of test_invalid_label.
type invalidLabelConfig struct{}

// Name returns the app name.
func (invalidLabelConfig) Name() string { return "test_app" }

// Label returns a label that is not an identifier.
func (invalidLabelConfig) Label() string { return "invalid.label" }

// someInstalledApps ports SOME_INSTALLED_APPS: a short list with a variety of
// cases, intentionally not in alphabetical order to check that the order is
// preserved.
func someInstalledApps() []apps.Config {
	return []apps.Config{
		myAdmin{},
		myAuth{},
		plainApp{"django.contrib.contenttypes"},
		plainApp{"django.contrib.sessions"},
		plainApp{"django.contrib.messages"},
		plainApp{"django.contrib.staticfiles"},
	}
}

// someInstalledAppsNames ports SOME_INSTALLED_APPS_NAMES.
var someInstalledAppsNames = []string{
	"django.contrib.admin",
	"django.contrib.auth",
	"django.contrib.contenttypes",
	"django.contrib.sessions",
	"django.contrib.messages",
	"django.contrib.staticfiles",
}

// modelRegistry registers the models of the apps under their labels, as the
// root package does, and returns the model registry (the part of Django's
// apps registry that get_model reads).
func modelRegistry(t *testing.T, reg *apps.Registry) *orm.Registry {
	t.Helper()
	models := orm.NewRegistry()
	for _, app := range reg.Models() {
		for _, m := range app.Models {
			if _, err := models.RegisterApp(app.App, m); err != nil {
				t.Fatalf("RegisterApp(%s): %v", app.App, err)
			}
		}
	}
	return models
}

// assertLookupError asserts that err is the error of a failed app lookup and
// that its message contains msg (assertRaisesMessage(LookupError, msg)).
func assertLookupError(t *testing.T, err error, msg string) {
	t.Helper()
	var le *apps.LookupError
	if !errors.As(err, &le) {
		t.Fatalf("error = %v (%T), want a *apps.LookupError", err, err)
	}
	if !strings.Contains(err.Error(), msg) {
		t.Errorf("message = %q, want it to contain %q", err.Error(), msg)
	}
}

// TestReady tests the ready flag of a registry. Django populates a registry in
// its constructor; a Go registry is ready when Ready has returned, so the flag
// is false before. The ready_event of Django's registry serves its
// autoreloader and has no counterpart.
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_ready
func TestReady(t *testing.T) {
	reg := newRegistry(t, someInstalledApps()...)
	djangotest.AssertEqual(t, reg.IsReady(), false)
	if err := reg.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	djangotest.AssertEqual(t, reg.IsReady(), true)

	// A registry without apps (Apps()) becomes ready as well.
	empty := apps.NewRegistry()
	if err := empty.Ready(); err != nil {
		t.Fatalf("Ready of an empty registry: %v", err)
	}
	djangotest.AssertEqual(t, empty.IsReady(), true)
}

// TestBadAppConfig tests an app config that does not supply a name. Django
// names the dotted path of the class in the message; the Go message names the
// type of the config.
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_bad_app_config
func TestBadAppConfig(t *testing.T) {
	err := apps.NewRegistry().Register(badConfig{})
	assertImproperlyConfigured(t, err, "'apps_test.badConfig' must supply a name attribute.")
}

// TestNoSuchApp tests an entry that names no app. Django fails to import such
// an entry, given directly or as the name of an app config. An installed app
// is a Go value here, so the entry that does not exist is a nil config, and
// the name Django cannot import is rejected because its last component is not
// a valid label.
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_no_such_app
func TestNoSuchApp(t *testing.T) {
	if err := apps.NewRegistry().Register(nil); err == nil {
		t.Error("Register(nil) = nil, want an error")
	}
	err := apps.NewRegistry().Register(noSuchApp{})
	assertImproperlyConfigured(t, err, "The app label 'there is no such app' is not a valid Python identifier.")
}

// TestGetAppConfigs tests apps.get_app_configs(), which is Registry.Configs.
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_get_app_configs
func TestGetAppConfigs(t *testing.T) {
	reg := newRegistry(t, someInstalledApps()...)
	djangotest.AssertSequenceEqual(t, configNames(reg.Configs()), someInstalledAppsNames)
	djangotest.AssertSequenceEqual(t, reg.Names(), someInstalledAppsNames)
}

// TestGetAppConfig tests apps.get_app_config(), which is Registry.ByLabel.
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_get_app_config
func TestGetAppConfig(t *testing.T) {
	reg := newRegistry(t, someInstalledApps()...)

	appConfig, err := reg.ByLabel("admin")
	if err != nil {
		t.Fatalf("ByLabel(admin): %v", err)
	}
	djangotest.AssertEqual(t, appConfig.Name(), "django.contrib.admin")

	appConfig, err = reg.ByLabel("staticfiles")
	if err != nil {
		t.Fatalf("ByLabel(staticfiles): %v", err)
	}
	djangotest.AssertEqual(t, appConfig.Name(), "django.contrib.staticfiles")

	_, err = reg.ByLabel("admindocs")
	assertLookupError(t, err, "No installed app with label 'admindocs'.")

	_, err = reg.ByLabel("django.contrib.auth")
	assertLookupError(t, err, "No installed app with label 'django.contrib.auth'. Did you mean 'myauth'")
}

// TestIsInstalled tests apps.is_installed().
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_is_installed
func TestIsInstalled(t *testing.T) {
	reg := newRegistry(t, someInstalledApps()...)
	djangotest.AssertEqual(t, reg.IsInstalled("django.contrib.admin"), true)
	djangotest.AssertEqual(t, reg.IsInstalled("django.contrib.auth"), true)
	djangotest.AssertEqual(t, reg.IsInstalled("django.contrib.staticfiles"), true)
	djangotest.AssertEqual(t, reg.IsInstalled("django.contrib.admindocs"), false)
}

// TestGetModel tests the part of apps.get_model() that the app registry
// decides: the models of an app are handed out under its label, and a model
// registry built from them finds a model by "<label>.<Model>" with a
// case-sensitive label. The lookup itself is orm.Registry.Get, whose model
// name is case-sensitive and which reports a missing model with false, so the
// "loGentrY" and ValueError assertions of the Django test are not ported.
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_get_model
func TestGetModel(t *testing.T) {
	reg := newRegistry(t, someInstalledApps()...)
	djangotest.AssertEqual(t, reg.Models(), []apps.AppModels{{App: "admin", Models: []any{&LogEntry{}}}})

	models := modelRegistry(t, reg)
	model, ok := models.Get("admin.LogEntry")
	if !ok {
		t.Fatal(`Get("admin.LogEntry") found no model`)
	}
	want, _ := models.ModelOf(&LogEntry{})
	djangotest.AssertEqual(t, model, want)

	_, ok = models.Get("admin.LogExit")
	djangotest.AssertEqual(t, ok, false)

	// App label is case-sensitive.
	_, ok = models.Get("Admin.LogEntry")
	djangotest.AssertEqual(t, ok, false)
}

// Ported from Django: tests/apps/tests.py::AppsTests.test_relabeling
func TestRelabeling(t *testing.T) {
	reg := newRegistry(t, relabeledAppsConfig{})
	appConfig, err := reg.ByLabel("relabeled")
	if err != nil {
		t.Fatalf("ByLabel(relabeled): %v", err)
	}
	djangotest.AssertEqual(t, appConfig.Name(), "apps")
}

// Ported from Django: tests/apps/tests.py::AppsTests.test_duplicate_labels
func TestDuplicateLabels(t *testing.T) {
	err := registerAll(apps.NewRegistry(), plainAppsConfig{}, plainApp{"apps"})
	assertImproperlyConfigured(t, err, "Application labels aren't unique")
	assertImproperlyConfigured(t, err, "Application labels aren't unique, duplicates: apps")
}

// Ported from Django: tests/apps/tests.py::AppsTests.test_duplicate_names
func TestDuplicateNames(t *testing.T) {
	err := registerAll(apps.NewRegistry(), relabeledAppsConfig{}, plainApp{"apps"})
	assertImproperlyConfigured(t, err, "Application names aren't unique")
	assertImproperlyConfigured(t, err, "Application names aren't unique, duplicates: apps")
}

// TestModelsPy tests that the models of an app are loaded into the registry
// the app is installed in and into no other: TotallyNormal belongs to the app
// "apps" of one registry, SoAlternative to the app "apps" of a second one
// (new_apps in tests/apps/models.py).
//
// Ported from Django: tests/apps/tests.py::AppsTests.test_models_py
func TestModelsPy(t *testing.T) {
	mainModels := modelRegistry(t, newRegistry(t, plainAppsConfig{}))
	newModels := modelRegistry(t, newRegistry(t, alternativeAppsConfig{}))

	model, ok := mainModels.Get("apps.TotallyNormal")
	if !ok {
		t.Fatal(`Get("apps.TotallyNormal") found no model`)
	}
	want, _ := mainModels.ModelOf(&TotallyNormal{})
	djangotest.AssertEqual(t, model, want)
	_, ok = mainModels.Get("apps.SoAlternative")
	djangotest.AssertEqual(t, ok, false)

	_, ok = newModels.Get("apps.TotallyNormal")
	djangotest.AssertEqual(t, ok, false)
	model, ok = newModels.Get("apps.SoAlternative")
	if !ok {
		t.Fatal(`Get("apps.SoAlternative") found no model`)
	}
	want, _ = newModels.ModelOf(&SoAlternative{})
	djangotest.AssertEqual(t, model, want)
}

// TestInvalidLabel tests a label that is not an identifier. Django validates
// the label when the AppConfig is instantiated; a Go config is validated when
// it is registered.
//
// Ported from Django: tests/apps/tests.py::AppConfigTests.test_invalid_label
func TestInvalidLabel(t *testing.T) {
	msg := "The app label 'invalid.label' is not a valid Python identifier."
	err := apps.NewRegistry().Register(invalidLabelConfig{})
	assertImproperlyConfigured(t, err, msg)
}
