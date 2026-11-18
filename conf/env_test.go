package conf_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// envValueFor returns a value for the field in the syntax of the environment,
// one that differs from the default, and what the field must hold afterwards.
// want is nil for a field type the test knows no value for.
func envValueFor(f taggedField) (raw string, want any) {
	switch current := f.value.Interface().(type) {
	case bool:
		if current {
			return "false", false
		}
		return "true", true
	case string:
		return "value of " + f.tag, "value of " + f.tag
	case []string:
		return "one, two", []string{"one", "two"}
	case time.Duration:
		return "90", 90 * time.Second
	case int:
		return "42", 42
	}
	return "", nil
}

// Every field with an env tag is set from prefix+tag, those of the nested
// Templates included, and every other exported field is one the documentation
// names as set in code only.
func TestApplyEnvSetsEveryTaggedField(t *testing.T) {
	s := conf.Defaults()
	vars := make(map[string]string)
	want := make(map[string]any)
	for _, f := range taggedFields(&s) {
		raw, value := envValueFor(f)
		if value == nil {
			t.Fatalf("%s: the test has no value for a field of the type %s", f.tag, f.value.Type())
		}
		if _, twice := want[f.tag]; twice {
			t.Errorf("the env tag %s occurs twice", f.tag)
		}
		vars["P_"+f.tag] = raw
		want[f.tag] = value
	}

	if err := s.ApplyEnvFunc("P_", mapEnv(vars)); err != nil {
		t.Fatal(err)
	}
	for _, f := range taggedFields(&s) {
		if got := f.value.Interface(); !reflect.DeepEqual(got, want[f.tag]) {
			t.Errorf("%s: the field is %#v after the overlay, want %#v", f.tag, got, want[f.tag])
		}
	}

	var untagged []string
	typ := reflect.TypeFor[conf.Settings]()
	for i := range typ.NumField() {
		if f := typ.Field(i); f.IsExported() && f.Tag.Get("env") == "" {
			untagged = append(untagged, f.Name)
		}
	}
	djangotest.AssertEqual(t, untagged, []string{"Database", "Databases", "Templates", "AuthPasswordValidators"})
}

// A value is parsed by the type of its field.
func TestApplyEnvValueSyntax(t *testing.T) {
	tests := []struct {
		name  string
		tag   string
		value string
		got   func(*conf.Settings) any
		want  any
	}{
		{"string as it is", "SESSION_COOKIE_NAME", " sid ", func(s *conf.Settings) any { return s.SessionCookieName }, " sid "},
		{"empty string replaces a default", "STATIC_URL", "", func(s *conf.Settings) any { return s.StaticURL }, ""},

		{"integer", "SECURE_HSTS_SECONDS", "31536000", func(s *conf.Settings) any { return s.SecureHSTSSeconds }, 31536000},
		{"integer with spaces", "SECURE_HSTS_SECONDS", " 60 ", func(s *conf.Settings) any { return s.SecureHSTSSeconds }, 60},
		{"negative integer", "SECURE_HSTS_SECONDS", "-1", func(s *conf.Settings) any { return s.SecureHSTSSeconds }, -1},

		{"duration in seconds", "SESSION_COOKIE_AGE", "3600", func(s *conf.Settings) any { return s.SessionCookieAge }, time.Hour},
		{"duration of zero", "SESSION_COOKIE_AGE", "0", func(s *conf.Settings) any { return s.SessionCookieAge }, time.Duration(0)},
		{"Go duration", "SESSION_COOKIE_AGE", "36h", func(s *conf.Settings) any { return s.SessionCookieAge }, 36 * time.Hour},
		{"Go duration of two units", "CSRF_COOKIE_AGE", "1h30m", func(s *conf.Settings) any { return s.CSRFCookieAge }, 90 * time.Minute},
		{"duration with spaces", "CSRF_COOKIE_AGE", " 90 ", func(s *conf.Settings) any { return s.CSRFCookieAge }, 90 * time.Second},

		{
			"list", "ALLOWED_HOSTS", "a.example,b.example",
			func(s *conf.Settings) any { return s.AllowedHosts },
			[]string{"a.example", "b.example"},
		},
		{
			"list with spaces and empty elements", "ALLOWED_HOSTS", " a.example , ,b.example,, ",
			func(s *conf.Settings) any { return s.AllowedHosts },
			[]string{"a.example", "b.example"},
		},
		{"list of one", "ALLOWED_HOSTS", "*", func(s *conf.Settings) any { return s.AllowedHosts }, []string{"*"}},
		{"empty list", "ALLOWED_HOSTS", "", func(s *conf.Settings) any { return s.AllowedHosts }, []string{}},
		{"list of empty elements", "ALLOWED_HOSTS", " , ", func(s *conf.Settings) any { return s.AllowedHosts }, []string{}},
		{
			"header and value", "SECURE_PROXY_SSL_HEADER", "X-Forwarded-Proto, https",
			func(s *conf.Settings) any { return s.SecureProxySSLHeader },
			[]string{"X-Forwarded-Proto", "https"},
		},
		{
			"nested struct", "TEMPLATES_DIRS", "templates, shared/templates",
			func(s *conf.Settings) any { return s.Templates.Dirs },
			[]string{"templates", "shared/templates"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := conf.Defaults()
			// A code value that the variable must replace.
			s.AllowedHosts = []string{"code.example"}
			if err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{"P_" + tt.tag: tt.value})); err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, tt.got(&s), tt.want)
		})
	}
}

// Every documented spelling of a boolean is accepted, in any letter case.
func TestApplyEnvBooleans(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "True", "t", "T", "yes", "Yes", "y", "Y", "on", "ON", " true "}
	falsy := []string{"0", "false", "FALSE", "False", "f", "F", "no", "No", "n", "N", "off", "OFF", " false "}

	for _, value := range truthy {
		s := conf.Defaults()
		if err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{"P_DEBUG": value})); err != nil {
			t.Errorf("DEBUG=%q: %v", value, err)
		}
		if !s.Debug {
			t.Errorf("DEBUG=%q left Debug false", value)
		}
	}
	// APPEND_SLASH is on by default, so the variable has to switch it off.
	for _, value := range falsy {
		s := conf.Defaults()
		if err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{"P_APPEND_SLASH": value})); err != nil {
			t.Errorf("APPEND_SLASH=%q: %v", value, err)
		}
		if s.AppendSlash {
			t.Errorf("APPEND_SLASH=%q left AppendSlash true", value)
		}
	}
}

// A value that does not fit the type of its field is an error that names the
// variable and quotes the value, and the field keeps its value.
func TestApplyEnvMalformedValue(t *testing.T) {
	const durationHint = ` (want a number of seconds or a Go duration such as "36h")`
	tests := []struct {
		tag   string
		value string
		want  string
	}{
		{"DEBUG", "maybe", `conf: DJANGOGO_DEBUG: invalid boolean "maybe"`},
		{"DEBUG", "", `conf: DJANGOGO_DEBUG: invalid boolean ""`},
		{"DEBUG", "2", `conf: DJANGOGO_DEBUG: invalid boolean "2"`},
		{"TEMPLATES_APP_DIRS", "si", `conf: DJANGOGO_TEMPLATES_APP_DIRS: invalid boolean "si"`},
		{"SECURE_HSTS_SECONDS", "1.5", `conf: DJANGOGO_SECURE_HSTS_SECONDS: invalid integer "1.5"`},
		{"SECURE_HSTS_SECONDS", "", `conf: DJANGOGO_SECURE_HSTS_SECONDS: invalid integer ""`},
		{"SECURE_HSTS_SECONDS", "1h", `conf: DJANGOGO_SECURE_HSTS_SECONDS: invalid integer "1h"`},
		{
			"SECURE_HSTS_SECONDS", "99999999999999999999",
			`conf: DJANGOGO_SECURE_HSTS_SECONDS: invalid integer "99999999999999999999" (out of range)`,
		},
		{"SESSION_COOKIE_AGE", "soon", `conf: DJANGOGO_SESSION_COOKIE_AGE: invalid duration "soon"` + durationHint},
		{"SESSION_COOKIE_AGE", "", `conf: DJANGOGO_SESSION_COOKIE_AGE: invalid duration ""` + durationHint},
		{"SESSION_COOKIE_AGE", "1.5", `conf: DJANGOGO_SESSION_COOKIE_AGE: invalid duration "1.5"` + durationHint},
		{
			"CSRF_COOKIE_AGE", "9223372036854775807",
			`conf: DJANGOGO_CSRF_COOKIE_AGE: invalid duration "9223372036854775807" (out of range)`,
		},
		{
			"CSRF_COOKIE_AGE", "99999999999999999999",
			`conf: DJANGOGO_CSRF_COOKIE_AGE: invalid duration "99999999999999999999" (out of range)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.tag+"="+tt.value, func(t *testing.T) {
			s := conf.Defaults()
			err := s.ApplyEnvFunc("DJANGOGO_", mapEnv(map[string]string{"DJANGOGO_" + tt.tag: tt.value}))

			djangotest.AssertEqual(t, errorLines(err), []string{tt.want})
			unchanged := conf.Defaults()
			djangotest.AssertEqual(t, s.Debug, unchanged.Debug)
			djangotest.AssertEqual(t, s.Templates, unchanged.Templates)
			djangotest.AssertEqual(t, s.SecureHSTSSeconds, unchanged.SecureHSTSSeconds)
			djangotest.AssertEqual(t, s.SessionCookieAge, unchanged.SessionCookieAge)
			djangotest.AssertEqual(t, s.CSRFCookieAge, unchanged.CSRFCookieAge)
		})
	}
}

// Every malformed variable is reported in one error, the fields of the
// malformed ones keep their values and the well-formed ones are applied.
func TestApplyEnvReportsEveryMalformedVariable(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSSeconds = 60
	s.Database = conf.Database{Driver: conf.DriverSQLite, DSN: "app.db"}
	err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{
		"P_DEBUG":               "maybe",
		"P_USE_TZ":              "nope",
		"P_TEMPLATES_APP_DIRS":  "perhaps",
		"P_SESSION_COOKIE_AGE":  "soon",
		"P_SECURE_HSTS_SECONDS": "many",
		"P_DATABASE_URL":        "mysql://app:hunter2@db.example/app",
		"P_ALLOWED_HOSTS":       "example.com",
		"P_CSRF_COOKIE_SECURE":  "yes",
	}))

	got := errorLines(err)
	slices.Sort(got)
	djangotest.AssertEqual(t, got, []string{
		`conf: P_DATABASE_URL: unsupported database URL scheme "mysql" (want sqlite or postgres)`,
		`conf: P_DEBUG: invalid boolean "maybe"`,
		`conf: P_SECURE_HSTS_SECONDS: invalid integer "many"`,
		`conf: P_SESSION_COOKIE_AGE: invalid duration "soon" (want a number of seconds or a Go duration such as "36h")`,
		`conf: P_TEMPLATES_APP_DIRS: invalid boolean "perhaps"`,
		`conf: P_USE_TZ: invalid boolean "nope"`,
	})
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok || len(joined.Unwrap()) != 6 {
		t.Errorf("the error does not join one error per variable: %#v", err)
	}

	djangotest.AssertEqual(t, s.Debug, false)
	djangotest.AssertEqual(t, s.UseTZ, true)
	djangotest.AssertEqual(t, s.Templates.AppDirs, true)
	djangotest.AssertEqual(t, s.SessionCookieAge, 14*24*time.Hour)
	djangotest.AssertEqual(t, s.SecureHSTSSeconds, 60)
	djangotest.AssertEqual(t, s.DefaultDatabase(), conf.Database{Driver: conf.DriverSQLite, DSN: "app.db"})
	djangotest.AssertEqual(t, s.AllowedHosts, []string{"example.com"})
	djangotest.AssertEqual(t, s.CSRFCookieSecure, true)
}

// The overlay completes a literal before it reads a variable, so a variable
// can switch off a setting whose default is on, and the value survives
// Configure.
func TestApplyEnvCompletesALiteralFirst(t *testing.T) {
	s := conf.Settings{SecretKey: "k"}
	err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{
		"P_APPEND_SLASH":            "false",
		"P_SESSION_COOKIE_HTTPONLY": "off",
		"P_STATIC_URL":              "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := activate(t, s)

	djangotest.AssertEqual(t, got.AppendSlash, false)
	djangotest.AssertEqual(t, got.SessionCookieHTTPOnly, false)
	djangotest.AssertEqual(t, got.StaticURL, "")
	djangotest.AssertEqual(t, got.UseTZ, true)
	djangotest.AssertEqual(t, got.SessionCookieName, "sessionid")
	djangotest.AssertEqual(t, got.SecretKey, "k")
}

// A variable that is not set leaves the code value alone.
func TestApplyEnvKeepsCodeValuesOfUnsetVariables(t *testing.T) {
	s := conf.Defaults()
	s.Debug = true
	s.AllowedHosts = []string{"code.example"}
	s.SessionCookieAge = time.Hour
	if err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{"DEBUG": "false", "OTHER_DEBUG": "false"})); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, s.Debug, true)
	djangotest.AssertEqual(t, s.AllowedHosts, []string{"code.example"})
	djangotest.AssertEqual(t, s.SessionCookieAge, time.Hour)
}

// A second overlay, for example of a file after the process environment, wins
// over the first one and keeps what only the first one set.
func TestApplyEnvTwiceTheLaterOverlayWins(t *testing.T) {
	s := conf.Defaults()
	if err := s.ApplyEnvFunc("A_", mapEnv(map[string]string{"A_DEBUG": "true", "A_LOGIN_URL": "/a/"})); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyEnvFunc("B_", mapEnv(map[string]string{"B_LOGIN_URL": "/b/"})); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, s.Debug, true)
	djangotest.AssertEqual(t, s.LoginURL, "/b/")
}

// A nil lookup is an error and not a panic at the first variable.
func TestApplyEnvFuncRefusesANilLookup(t *testing.T) {
	s := conf.Defaults()
	err := s.ApplyEnvFunc("P_", nil)

	djangotest.AssertEqual(t, errorLines(err), []string{"conf: nil environment lookup"})
}

// ApplyEnv reads the variables of the process.
func TestApplyEnvReadsTheProcessEnvironment(t *testing.T) {
	t.Setenv("CONFTEST_DEBUG", "on")
	t.Setenv("CONFTEST_ALLOWED_HOSTS", "a.example, b.example")
	t.Setenv("CONFTEST_SESSION_COOKIE_AGE", "3600")
	t.Setenv("CONFTEST_DATABASE_URL", "sqlite:///env.db")

	s := conf.Defaults()
	s.Debug = false
	s.LoginURL = "/login/"
	if err := s.ApplyEnv("CONFTEST_"); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, s.Debug, true)
	djangotest.AssertEqual(t, s.AllowedHosts, []string{"a.example", "b.example"})
	djangotest.AssertEqual(t, s.SessionCookieAge, time.Hour)
	djangotest.AssertEqual(t, s.Database, conf.Database{Driver: conf.DriverSQLite, DSN: "env.db"})
	djangotest.AssertEqual(t, s.LoginURL, "/login/")
}

// FromEnv is the defaults overlaid with the process environment.
func TestFromEnv(t *testing.T) {
	t.Setenv("CONFTEST_SECRET_KEY", "from the environment")
	t.Setenv("CONFTEST_USE_TZ", "0")
	t.Setenv("CONFTEST_DATABASE_URL", "sqlite://:memory:")

	s, err := conf.FromEnv("CONFTEST_")
	if err != nil {
		t.Fatal(err)
	}

	want := conf.Defaults()
	djangotest.AssertEqual(t, s.SecretKey, "from the environment")
	djangotest.AssertEqual(t, s.UseTZ, false)
	djangotest.AssertEqual(t, s.Database, conf.Database{Driver: conf.DriverSQLite, DSN: ":memory:"})
	djangotest.AssertEqual(t, s.SessionCookieName, want.SessionCookieName)
	djangotest.AssertEqual(t, s.AppendSlash, want.AppendSlash)
}

// FromEnv returns the settings also when a variable is malformed, with the
// defaults in the fields of the malformed variables.
func TestFromEnvMalformed(t *testing.T) {
	t.Setenv("CONFTEST_DEBUG", "maybe")
	t.Setenv("CONFTEST_LOGIN_URL", "/login/")
	t.Setenv("CONFTEST_DATABASE_URL", "sqlite://:memory:")

	s, err := conf.FromEnv("CONFTEST_")

	djangotest.AssertEqual(t, errorLines(err), []string{`conf: CONFTEST_DEBUG: invalid boolean "maybe"`})
	djangotest.AssertEqual(t, s.Debug, false)
	djangotest.AssertEqual(t, s.LoginURL, "/login/")
	djangotest.AssertEqual(t, s.SessionCookieName, "sessionid")
}
