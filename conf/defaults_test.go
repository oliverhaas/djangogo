package conf_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// djangoDefaults holds the default of every setting the environment can set,
// under its env tag. The values are those of django.conf.global_settings in
// Django 6.0.3, with None as the empty string or the nil slice and seconds as
// a time.Duration. A row marked "differs" is a deliberate difference that the
// doc comment of Defaults lists; the comment gives Django's value.
var djangoDefaults = map[string]any{
	"DEBUG":                  false,
	"SECRET_KEY":             "",
	"SECRET_KEY_FALLBACKS":   []string(nil),
	"ALLOWED_HOSTS":          []string(nil),
	"INSTALLED_APPS":         []string(nil),
	"SILENCED_SYSTEM_CHECKS": []string(nil),

	"HOST": "127.0.0.1", // differs: the default of runserver, not a setting
	"PORT": "8000",      // differs: the default of runserver, not a setting

	"TEMPLATES_DIRS":               []string(nil),
	"TEMPLATES_APP_DIRS":           true, // differs: TEMPLATES is empty
	"TEMPLATES_CONTEXT_PROCESSORS": []string(nil),

	"TIME_ZONE": "UTC", // differs: "America/Chicago"
	"USE_TZ":    true,

	"STATIC_URL":       "/static/", // differs: None
	"STATIC_ROOT":      "",
	"STATICFILES_DIRS": []string(nil),

	"LOGIN_URL":           "/accounts/login/",
	"LOGIN_REDIRECT_URL":  "/accounts/profile/",
	"LOGOUT_REDIRECT_URL": "",

	"PASSWORD_RESET_TIMEOUT": 259200 * time.Second,

	"APPEND_SLASH":         true,
	"USE_X_FORWARDED_HOST": false,
	"USE_X_FORWARDED_PORT": false,

	"SESSION_ENGINE":                  "db", // differs: "django.contrib.sessions.backends.db"
	"SESSION_FILE_PATH":               "",
	"SESSION_COOKIE_NAME":             "sessionid",
	"SESSION_COOKIE_AGE":              1209600 * time.Second,
	"SESSION_COOKIE_SECURE":           false,
	"SESSION_COOKIE_HTTPONLY":         true,
	"SESSION_COOKIE_SAMESITE":         "Lax",
	"SESSION_COOKIE_DOMAIN":           "",
	"SESSION_COOKIE_PATH":             "/",
	"SESSION_SAVE_EVERY_REQUEST":      false,
	"SESSION_EXPIRE_AT_BROWSER_CLOSE": false,

	"MESSAGE_STORAGE": "fallback", // differs: "django.contrib.messages.storage.fallback.FallbackStorage"

	"CSRF_TRUSTED_ORIGINS":   []string(nil),
	"CSRF_COOKIE_NAME":       "csrftoken",
	"CSRF_COOKIE_AGE":        31449600 * time.Second,
	"CSRF_COOKIE_SECURE":     false,
	"CSRF_COOKIE_HTTPONLY":   false,
	"CSRF_COOKIE_SAMESITE":   "Lax",
	"CSRF_COOKIE_DOMAIN":     "",
	"CSRF_COOKIE_PATH":       "/",
	"CSRF_HEADER_NAME":       "X-CSRFToken", // differs: "HTTP_X_CSRFTOKEN"
	"CSRF_USE_SESSIONS":      false,
	"SECURE_SSL_REDIRECT":    false,
	"SECURE_SSL_HOST":        "",
	"SECURE_REDIRECT_EXEMPT": []string(nil),
	"SECURE_HSTS_SECONDS":    0,

	"SECURE_HSTS_INCLUDE_SUBDOMAINS":    false,
	"SECURE_HSTS_PRELOAD":               false,
	"SECURE_CONTENT_TYPE_NOSNIFF":       true,
	"SECURE_REFERRER_POLICY":            "same-origin",
	"SECURE_CROSS_ORIGIN_OPENER_POLICY": "same-origin",
	"SECURE_PROXY_SSL_HEADER":           []string(nil),
	"X_FRAME_OPTIONS":                   "DENY",

	"EMAIL_BACKEND":        "smtp", // differs: "django.core.mail.backends.smtp.EmailBackend"
	"EMAIL_HOST":           "localhost",
	"EMAIL_PORT":           25,
	"EMAIL_HOST_USER":      "",
	"EMAIL_HOST_PASSWORD":  "",
	"EMAIL_USE_TLS":        false,
	"EMAIL_USE_SSL":        false,
	"EMAIL_SSL_CERTFILE":   "",
	"EMAIL_SSL_KEYFILE":    "",
	"EMAIL_TIMEOUT":        time.Duration(0),
	"EMAIL_FILE_PATH":      "",
	"EMAIL_SUBJECT_PREFIX": "[Django] ",
	"EMAIL_USE_LOCALTIME":  false,
	"DEFAULT_FROM_EMAIL":   "webmaster@localhost",
	"SERVER_EMAIL":         "root@localhost",
	"ADMINS":               []string(nil),
	"MANAGERS":             []string(nil),
	"DEFAULT_CHARSET":      "utf-8",

	"PASSWORD_HASHERS": []string(nil), // differs: five dotted class paths
}

// The default of every setting is Django's, apart from the documented
// differences. A new setting needs a row in djangoDefaults.
func TestDefaultsAreDjangos(t *testing.T) {
	s := conf.Defaults()

	seen := make(map[string]bool)
	for _, f := range taggedFields(&s) {
		want, ok := djangoDefaults[f.tag]
		if !ok {
			t.Errorf("%s: the table of defaults has no row for this setting", f.tag)
			continue
		}
		seen[f.tag] = true
		if got := f.value.Interface(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: default is %#v, want %#v", f.tag, got, want)
		}
	}
	for tag := range djangoDefaults {
		if !seen[tag] {
			t.Errorf("%s: the table of defaults has a row, but no field has this env tag", tag)
		}
	}

	// Django's DATABASES and AUTH_PASSWORD_VALIDATORS are empty.
	djangotest.AssertEqual(t, s.Database, conf.Database{Driver: conf.DriverSQLite})
	djangotest.AssertEqual(t, s.Databases, nil)
	djangotest.AssertEqual(t, s.AuthPasswordValidators, nil)
}

// A literal passed to Configure is completed to the same values as Defaults.
func TestLiteralIsCompletedWithTheDefaults(t *testing.T) {
	got := activate(t, conf.Settings{SecretKey: "k", Debug: true, TimeZone: "Europe/Berlin"})

	want := conf.Defaults()
	want.SecretKey = "k"
	want.Debug = true
	want.TimeZone = "Europe/Berlin"
	wantFields := taggedFields(&want)
	for i, f := range taggedFields(got) {
		if got, want := f.value.Interface(), wantFields[i].value.Interface(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: a completed literal has %#v, want %#v", f.tag, got, want)
		}
	}
	djangotest.AssertEqual(t, got.Database, conf.Database{Driver: conf.DriverSQLite})
}

// A literal cannot express a false or empty value for a setting whose default
// is not the zero value; settings that start from Defaults can.
func TestOnlyDefaultsKeepAFalseOrEmptyValue(t *testing.T) {
	literal := activate(t, conf.Settings{
		AppendSlash: false,
		StaticURL:   "",
		Templates:   conf.Templates{AppDirs: false, Dirs: []string{"templates"}},
	})
	djangotest.AssertEqual(t, literal.AppendSlash, true)
	djangotest.AssertEqual(t, literal.StaticURL, "/static/")
	djangotest.AssertEqual(t, literal.Templates, conf.Templates{AppDirs: true, Dirs: []string{"templates"}})

	s := conf.Defaults()
	s.AppendSlash = false
	s.StaticURL = ""
	s.Templates.AppDirs = false
	s.SessionCookieHTTPOnly = false
	fromDefaults := conf.Configure(s)
	djangotest.AssertEqual(t, fromDefaults.AppendSlash, false)
	djangotest.AssertEqual(t, fromDefaults.StaticURL, "")
	djangotest.AssertEqual(t, fromDefaults.Templates.AppDirs, false)
	djangotest.AssertEqual(t, fromDefaults.SessionCookieHTTPOnly, false)

	// A second pass (Configure of the configured value) changes nothing either.
	again := conf.Configure(*fromDefaults)
	djangotest.AssertEqual(t, again.AppendSlash, false)
	djangotest.AssertEqual(t, again.StaticURL, "")
}

// Completing the settings brings the Database field and Databases["default"]
// in line, whichever of them the project set.
func TestDatabaseAndDefaultAliasAreTheSameValue(t *testing.T) {
	var (
		pg     = conf.Database{Driver: conf.DriverPostgres, DSN: "postgres://db.example/app"}
		lite   = conf.Database{Driver: conf.DriverSQLite, DSN: "app.db"}
		noFile = conf.Database{Driver: conf.DriverSQLite}
	)
	tests := []struct {
		name          string
		database      conf.Database
		databases     map[string]conf.Database
		want          conf.Database
		wantDatabases map[string]conf.Database
	}{
		{
			name: "neither is set",
			want: noFile,
		},
		{
			name:          "the field alone fills the alias",
			database:      pg,
			want:          pg,
			wantDatabases: map[string]conf.Database{"default": pg},
		},
		{
			name:          "the alias alone fills the field",
			databases:     map[string]conf.Database{"default": pg},
			want:          pg,
			wantDatabases: map[string]conf.Database{"default": pg},
		},
		{
			name:          "both are set: the alias wins",
			database:      lite,
			databases:     map[string]conf.Database{"default": pg},
			want:          pg,
			wantDatabases: map[string]conf.Database{"default": pg},
		},
		{
			name:          "the alias has no DSN: the field wins",
			database:      pg,
			databases:     map[string]conf.Database{"default": {Driver: conf.DriverPostgres}},
			want:          pg,
			wantDatabases: map[string]conf.Database{"default": pg},
		},
		{
			name:          "a field without a driver gets the default driver",
			database:      conf.Database{DSN: "app.db"},
			want:          lite,
			wantDatabases: map[string]conf.Database{"default": lite},
		},
		{
			name:      "another alias without a driver gets the default driver",
			databases: map[string]conf.Database{"replica": {DSN: "replica.db"}},
			want:      noFile,
			wantDatabases: map[string]conf.Database{
				"default": noFile,
				"replica": {Driver: conf.DriverSQLite, DSN: "replica.db"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := conf.Defaults()
			s.Database = tt.database
			s.Databases = tt.databases
			got := activate(t, s)

			djangotest.AssertEqual(t, got.Database, tt.want)
			djangotest.AssertEqual(t, got.Databases, tt.wantDatabases)
			djangotest.AssertEqual(t, got.DefaultDatabase(), tt.want)
		})
	}
}

// A project builds its settings once and a test then points them at another
// database: whichever of the field and the alias was changed last must win.
func TestDatabaseChangedAfterTheFirstSyncIsKept(t *testing.T) {
	var (
		dev  = conf.Database{Driver: conf.DriverSQLite, DSN: "dev.db"}
		test = conf.Database{Driver: conf.DriverPostgres, DSN: "postgres://db.example/test"}
	)
	// built returns settings that already went through an overlay, which
	// brings the field and the alias in line.
	built := func(t *testing.T) conf.Settings {
		t.Helper()
		s := conf.Defaults()
		s.Database = dev
		if err := s.ApplyEnvFunc("", mapEnv(nil)); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, s.Databases, map[string]conf.Database{"default": dev})
		return s
	}

	t.Run("the field", func(t *testing.T) {
		s := built(t)
		s.Database = test
		got := activate(t, s)

		djangotest.AssertEqual(t, got.Database, test)
		djangotest.AssertEqual(t, got.DefaultDatabase(), test)
	})
	t.Run("the alias", func(t *testing.T) {
		s := built(t)
		s.Databases = map[string]conf.Database{"default": test}
		got := activate(t, s)

		djangotest.AssertEqual(t, got.Database, test)
		djangotest.AssertEqual(t, got.DefaultDatabase(), test)
	})
	t.Run("the field, to a database without a DSN", func(t *testing.T) {
		s := built(t)
		s.Database = conf.Database{Driver: conf.DriverPostgres}
		got := activate(t, s)

		djangotest.AssertEqual(t, got.Database, conf.Database{Driver: conf.DriverPostgres})
		djangotest.AssertEqual(t, got.DefaultDatabase(), conf.Database{Driver: conf.DriverPostgres})
	})
}

// Completing the settings must not write into a map that the caller, or a
// copy of the settings, still holds.
func TestCompletingTheSettingsLeavesTheCallersMapAlone(t *testing.T) {
	databases := map[string]conf.Database{"replica": {DSN: "replica.db"}}
	s := conf.Defaults()
	s.Database = conf.Database{DSN: "app.db"}
	s.Databases = databases
	got := activate(t, s)

	djangotest.AssertEqual(t, len(got.Databases), 2)
	djangotest.AssertEqual(t, databases, map[string]conf.Database{"replica": {DSN: "replica.db"}})
}
