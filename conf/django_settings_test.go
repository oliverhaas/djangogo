package conf_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The tests in this file are ports of Django's tests/settings_tests/tests.py.
// Django's settings are a lazy object with dynamic attributes that tests
// override for a block; here they are a typed struct, and an override is a
// changed copy. The tests of override_settings and modify_settings as
// decorators, of the setting_changed signal, of deleting a setting, of
// is_overridden, of the repr and of the settings module import have no
// counterpart and are not ported. SecureProxySslHeaderTest is ported with the
// middleware package.

// TestOverride overrides a setting and restores it. Django overrides for a
// block with self.settings(TEST="override"); here the override is a changed
// copy that is made active, and the original is made active again afterwards.
// The typed SessionCookieName stands in for the dynamic TEST.
//
// Ported from Django: tests/settings_tests/tests.py::SettingsTests.test_override
func TestOverride(t *testing.T) {
	base := conf.Defaults()
	base.SessionCookieName = "test"
	original := activate(t, base)
	djangotest.AssertEqual(t, conf.Active().SessionCookieName, "test")

	override := *original
	override.SessionCookieName = "override"
	conf.Configure(override)
	djangotest.AssertEqual(t, conf.Active().SessionCookieName, "override")

	conf.Configure(*original)
	djangotest.AssertEqual(t, conf.Active().SessionCookieName, "test")
}

// TestOverrideDoesntLeak changes an override and finds the original untouched.
// In Django the setting TEST exists only inside the block. Here every change
// made to a copy (in code, from the environment, to a database, to a settings
// namespace) stays in the copy, also where the two share a map or a slice.
//
// Ported from Django: tests/settings_tests/tests.py::SettingsTests.test_override_doesnt_leak
func TestOverrideDoesntLeak(t *testing.T) {
	type leakSettings struct{ Test string }
	if err := conf.Register("override_leak", leakSettings{}); err != nil {
		t.Fatal(err)
	}
	base := conf.Defaults()
	base.AllowedHosts = []string{"example.com"}
	base.Databases = map[string]conf.Database{
		"replica": {Driver: conf.DriverPostgres, DSN: "postgres://replica/app"},
	}
	if err := base.SetNamespace("override_leak", leakSettings{Test: "base"}); err != nil {
		t.Fatal(err)
	}

	override := base
	err := override.ApplyEnvFunc("T_", mapEnv(map[string]string{
		"T_ALLOWED_HOSTS":         "override.example",
		"T_DATABASE_URL":          "sqlite:///override.db",
		"T_DATABASE_URL_REPLICA":  "postgres://other/app",
		"T_OVERRIDE_LEAK_TEST":    "from the environment",
		"T_SESSION_COOKIE_SECURE": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := override.SetNamespace("override_leak", leakSettings{Test: "override"}); err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, override.AllowedHosts, []string{"override.example"})
	djangotest.AssertEqual(t, override.Databases["replica"].DSN, "postgres://other/app")

	djangotest.AssertEqual(t, base.AllowedHosts, []string{"example.com"})
	djangotest.AssertEqual(t, base.SessionCookieSecure, false)
	djangotest.AssertEqual(t, base.Database, conf.Database{Driver: conf.DriverSQLite})
	djangotest.AssertEqual(t, base.Databases, map[string]conf.Database{
		"replica": {Driver: conf.DriverPostgres, DSN: "postgres://replica/app"},
	})
	got, err := conf.Get[leakSettings](&base, "override_leak")
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, got, leakSettings{Test: "base"})
}

// TestNoSecretKey refuses an empty SECRET_KEY. Django raises on the first
// access to the setting; here Check reports it when the application boots.
//
// Ported from Django: tests/settings_tests/tests.py::SettingsTests.test_no_secret_key
func TestNoSecretKey(t *testing.T) {
	s := conf.Defaults()
	s.SecretKey = ""
	err := s.Check()

	var improperlyConfigured *conf.ImproperlyConfigured
	if !errors.As(err, &improperlyConfigured) {
		t.Fatalf("Check() = %v, want an *ImproperlyConfigured", err)
	}
	djangotest.AssertEqual(t, err.Error(), "The SECRET_KEY setting must not be empty.")

	s.SecretKey = "set"
	djangotest.AssertEqual(t, s.Check(), error(nil))
}

// TestNoSettingsModule reads the settings before they are configured. Django
// raises ImproperlyConfigured and names DJANGO_SETTINGS_MODULE; here there is
// no settings module, and Active panics with a message that names Configure.
//
// Ported from Django: tests/settings_tests/tests.py::SettingsTests.test_no_settings_module
func TestNoSettingsModule(t *testing.T) {
	previous := conf.SwapActive(nil)
	t.Cleanup(func() { conf.SwapActive(previous) })

	defer func() {
		got := fmt.Sprint(recover())
		djangotest.AssertEqual(t, got, "conf: settings not configured; call conf.Configure first")
	}()
	conf.Active()
	t.Error("Active() returned without configured settings")
}

// TestAlreadyConfigured configures the settings twice. Django raises
// RuntimeError("Settings already configured."); here the second call replaces
// the active settings, which is what a test that reconfigures relies on.
//
// Ported from Django: tests/settings_tests/tests.py::SettingsTests.test_already_configured
func TestAlreadyConfigured(t *testing.T) {
	first := activate(t, conf.Settings{SecretKey: "first"})
	second := conf.Configure(conf.Settings{SecretKey: "second"})

	djangotest.AssertEqual(t, conf.Active() == second, true)
	djangotest.AssertEqual(t, conf.Active().SecretKey, "second")
	djangotest.AssertEqual(t, first.SecretKey, "first")
}

// TestIncorrectTimezone sets an unknown time zone. Django raises ValueError
// while it loads the settings; here Validate reports the problem with the same
// message for the system checks, and Location returns an error.
//
// Ported from Django: tests/settings_tests/tests.py::SettingsTests.test_incorrect_timezone
func TestIncorrectTimezone(t *testing.T) {
	s := conf.Defaults()
	s.TimeZone = "test"

	djangotest.AssertEqual(t, s.Validate(), []conf.Problem{
		{Setting: "TIME_ZONE", Msg: "Incorrect timezone setting: test"},
	})
	if _, err := s.Location(); err == nil {
		t.Error("Location() of the time zone \"test\" returned no error")
	}
}

// TestTupleSettings gives a list setting a plain string. Django raises
// ImproperlyConfigured("The ALLOWED_HOSTS setting must be a list or a tuple.")
// because a string would be iterated by character. Here the field type rules
// the mistake out in code, and a plain string in the environment is a list of
// one element. TEMPLATE_DIRS is Templates.Dirs; LOCALE_PATHS has no field.
//
// Ported from Django: tests/settings_tests/tests.py::TestListSettings.test_tuple_settings
func TestTupleSettings(t *testing.T) {
	listSettings := map[string]func(*conf.Settings) []string{
		"ALLOWED_HOSTS":        func(s *conf.Settings) []string { return s.AllowedHosts },
		"INSTALLED_APPS":       func(s *conf.Settings) []string { return s.InstalledApps },
		"TEMPLATES_DIRS":       func(s *conf.Settings) []string { return s.Templates.Dirs },
		"SECRET_KEY_FALLBACKS": func(s *conf.Settings) []string { return s.SecretKeyFallbacks },
	}
	for setting, field := range listSettings {
		t.Run(setting, func(t *testing.T) {
			s := conf.Defaults()
			err := s.ApplyEnvFunc("", mapEnv(map[string]string{setting: "non_list_or_tuple_value"}))
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, field(&s), []string{"non_list_or_tuple_value"})
		})
	}
}
