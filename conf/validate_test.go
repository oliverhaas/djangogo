package conf_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The rules that Django tests have their ports in django_validate_test.go and
// django_settings_test.go: STATIC_URL, CSRF_TRUSTED_ORIGINS, the SameSite
// settings and TIME_ZONE. This file covers the rules without a Django test.

// Each rule accepts the valid values and reports an invalid one under the
// Django name of its setting.
func TestValidate(t *testing.T) {
	const (
		proxyHeader = "The SECURE_PROXY_SSL_HEADER setting must be a tuple containing two values."
		drivers     = `the drivers are "sqlite" and "postgres".`
	)
	tests := []struct {
		name   string
		change func(*conf.Settings)
		want   []conf.Problem
	}{
		{name: "the defaults", change: func(*conf.Settings) {}},

		{name: "port 0", change: func(s *conf.Settings) { s.Port = "0" }},
		{name: "port 65535", change: func(s *conf.Settings) { s.Port = "65535" }},
		{
			name:   "port above the range",
			change: func(s *conf.Settings) { s.Port = "65536" },
			want:   []conf.Problem{{Setting: "PORT", Msg: `The PORT setting must be a port number from 0 to 65535, but is "65536".`}},
		},
		{
			name:   "negative port",
			change: func(s *conf.Settings) { s.Port = "-1" },
			want:   []conf.Problem{{Setting: "PORT", Msg: `The PORT setting must be a port number from 0 to 65535, but is "-1".`}},
		},
		{
			name:   "port that is a service name",
			change: func(s *conf.Settings) { s.Port = "http" },
			want:   []conf.Problem{{Setting: "PORT", Msg: `The PORT setting must be a port number from 0 to 65535, but is "http".`}},
		},
		{
			name:   "port emptied after the defaults",
			change: func(s *conf.Settings) { s.Port = "" },
			want:   []conf.Problem{{Setting: "PORT", Msg: `The PORT setting must be a port number from 0 to 65535, but is "".`}},
		},

		{
			name: "every spelling of a known driver",
			change: func(s *conf.Settings) {
				s.Database = conf.Database{Driver: "postgresql", DSN: "postgres://db.example/app"}
				s.Databases = map[string]conf.Database{
					"a": {Driver: "pgx"}, "b": {Driver: "sqlite3"}, "c": {Driver: "SQLite"}, "d": {},
				}
			},
		},
		{
			name:   "unknown driver of the Database field",
			change: func(s *conf.Settings) { s.Database = conf.Database{Driver: "mysql", DSN: "mysql://db.example/app"} },
			want: []conf.Problem{{
				Setting: "DATABASES",
				Msg:     `The DATABASES setting names the unknown driver "mysql" for the database "default"; ` + drivers,
			}},
		},
		{
			name:   "unknown driver of the Database field without a DSN",
			change: func(s *conf.Settings) { s.Database = conf.Database{Driver: "mysql"} },
			want: []conf.Problem{{
				Setting: "DATABASES",
				Msg:     `The DATABASES setting names the unknown driver "mysql" for the database "default"; ` + drivers,
			}},
		},
		{
			name: "unknown drivers of two aliases, in the order of the aliases",
			change: func(s *conf.Settings) {
				s.Databases = map[string]conf.Database{
					"replica": {Driver: "oracle", DSN: "x"},
					"archive": {Driver: "mssql", DSN: "y"},
				}
			},
			want: []conf.Problem{
				{
					Setting: "DATABASES",
					Msg:     `The DATABASES setting names the unknown driver "mssql" for the database "archive"; ` + drivers,
				},
				{
					Setting: "DATABASES",
					Msg:     `The DATABASES setting names the unknown driver "oracle" for the database "replica"; ` + drivers,
				},
			},
		},

		{name: "file sessions", change: func(s *conf.Settings) { s.SessionEngine = conf.SessionEngineFile }},
		{name: "signed cookie sessions", change: func(s *conf.Settings) { s.SessionEngine = conf.SessionEngineSignedCookies }},
		{
			name:   "unknown session engine",
			change: func(s *conf.Settings) { s.SessionEngine = "cache" },
			want: []conf.Problem{{
				Setting: "SESSION_ENGINE",
				Msg:     `The SESSION_ENGINE setting must be "db", "file" or "signed_cookies", but is "cache".`,
			}},
		},
		{
			name:   "Django's dotted path of a session engine",
			change: func(s *conf.Settings) { s.SessionEngine = "django.contrib.sessions.backends.db" },
			want: []conf.Problem{{
				Setting: "SESSION_ENGINE",
				Msg: `The SESSION_ENGINE setting must be "db", "file" or "signed_cookies", ` +
					`but is "django.contrib.sessions.backends.db".`,
			}},
		},

		{name: "cookie message storage", change: func(s *conf.Settings) { s.MessageStorage = conf.MessageStorageCookie }},
		{name: "session message storage", change: func(s *conf.Settings) { s.MessageStorage = conf.MessageStorageSession }},
		{
			name:   "Django's dotted path of a message storage",
			change: func(s *conf.Settings) { s.MessageStorage = "django.contrib.messages.storage.session.SessionStorage" },
			want: []conf.Problem{{
				Setting: "MESSAGE_STORAGE",
				Msg: `The MESSAGE_STORAGE setting must be "fallback", "cookie" or "session", ` +
					`but is "django.contrib.messages.storage.session.SessionStorage".`,
			}},
		},

		{
			name: "ages of zero",
			change: func(s *conf.Settings) {
				s.SessionCookieAge, s.CSRFCookieAge, s.SecureHSTSSeconds = 0, 0, 0
			},
		},
		{
			name:   "negative session cookie age",
			change: func(s *conf.Settings) { s.SessionCookieAge = -time.Second },
			want:   []conf.Problem{{Setting: "SESSION_COOKIE_AGE", Msg: "The SESSION_COOKIE_AGE setting must not be negative."}},
		},
		{
			name:   "negative CSRF cookie age",
			change: func(s *conf.Settings) { s.CSRFCookieAge = -time.Hour },
			want:   []conf.Problem{{Setting: "CSRF_COOKIE_AGE", Msg: "The CSRF_COOKIE_AGE setting must not be negative."}},
		},
		{
			name:   "negative HSTS seconds",
			change: func(s *conf.Settings) { s.SecureHSTSSeconds = -1 },
			want:   []conf.Problem{{Setting: "SECURE_HSTS_SECONDS", Msg: "The SECURE_HSTS_SECONDS setting must not be negative."}},
		},

		{name: "no SameSite attribute", change: func(s *conf.Settings) { s.SessionCookieSameSite, s.CSRFCookieSameSite = "", "" }},

		{
			name:   "proxy header with its value",
			change: func(s *conf.Settings) { s.SecureProxySSLHeader = []string{"X-Forwarded-Proto", "https"} },
		},
		{
			name:   "proxy header without a value",
			change: func(s *conf.Settings) { s.SecureProxySSLHeader = []string{"X-Forwarded-Proto"} },
			want:   []conf.Problem{{Setting: "SECURE_PROXY_SSL_HEADER", Msg: proxyHeader}},
		},
		{
			name:   "proxy header with three values",
			change: func(s *conf.Settings) { s.SecureProxySSLHeader = []string{"X-Forwarded-Proto", "https", "http"} },
			want:   []conf.Problem{{Setting: "SECURE_PROXY_SSL_HEADER", Msg: proxyHeader}},
		},

		{name: "frame options SAMEORIGIN", change: func(s *conf.Settings) { s.XFrameOptions = "SAMEORIGIN" }},
		{name: "frame options in lower case", change: func(s *conf.Settings) { s.XFrameOptions = "sameorigin" }},
		{name: "empty frame options", change: func(s *conf.Settings) { s.XFrameOptions = "" }},
		{
			name:   "unknown frame options",
			change: func(s *conf.Settings) { s.XFrameOptions = "ALLOWALL" },
			want: []conf.Problem{{
				Setting: "X_FRAME_OPTIONS",
				Msg:     `The X_FRAME_OPTIONS setting must be "DENY" or "SAMEORIGIN", but is "ALLOWALL".`,
			}},
		},

		{
			name: "every trusted origin without a scheme",
			change: func(s *conf.Settings) {
				s.CSRFTrustedOrigins = []string{"https://ok.example", "a.example", "*.b.example", "https://*.ok.example"}
			},
			want: []conf.Problem{
				{
					Setting: "CSRF_TRUSTED_ORIGINS",
					Msg: "The values in the CSRF_TRUSTED_ORIGINS setting must start with a scheme " +
						"(usually http:// or https://) but found a.example.",
				},
				{
					Setting: "CSRF_TRUSTED_ORIGINS",
					Msg: "The values in the CSRF_TRUSTED_ORIGINS setting must start with a scheme " +
						"(usually http:// or https://) but found *.b.example.",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := conf.Defaults()
			tt.change(&s)
			djangotest.AssertEqual(t, s.Validate(), tt.want)
		})
	}
}

// A literal is validated as Configure would complete it, and Validate leaves
// it a literal.
func TestValidateALiteral(t *testing.T) {
	s := conf.Settings{}
	djangotest.AssertEqual(t, s.Validate(), nil)
	djangotest.AssertEqual(t, s.Port, "")
	djangotest.AssertEqual(t, s.Databases, nil)

	// The literal is still completed with the defaults afterwards.
	s.XFrameOptions = "ALLOWALL"
	djangotest.AssertEqual(t, len(s.Validate()), 1)
	djangotest.AssertEqual(t, activate(t, s).Port, "8000")
}

// Every invalid setting is reported in one call.
func TestValidateReportsEveryProblem(t *testing.T) {
	s := conf.Defaults()
	s.Port = "http"
	s.Database.Driver = "mysql"
	s.TimeZone = "Mars/Olympus"
	s.StaticURL = "static"
	s.SessionEngine = "cache"
	s.MessageStorage = "cache"
	s.SessionCookieAge = -1
	s.SessionCookieSameSite = "Loose"
	s.CSRFTrustedOrigins = []string{"example.com"}
	s.CSRFCookieAge = -1
	s.CSRFCookieSameSite = "Loose"
	s.SecureHSTSSeconds = -1
	s.SecureProxySSLHeader = []string{"X-Forwarded-Proto"}
	s.XFrameOptions = "ALLOWALL"

	problems := s.Validate()
	settings := make([]string, 0, len(problems))
	for _, problem := range problems {
		settings = append(settings, problem.Setting)
	}
	djangotest.AssertEqual(t, settings, []string{
		"PORT", "DATABASES", "TIME_ZONE", "STATIC_URL", "SESSION_ENGINE", "MESSAGE_STORAGE", "SESSION_COOKIE_AGE",
		"SESSION_COOKIE_SAMESITE", "CSRF_TRUSTED_ORIGINS", "CSRF_COOKIE_AGE", "CSRF_COOKIE_SAMESITE",
		"SECURE_HSTS_SECONDS", "SECURE_PROXY_SSL_HEADER", "X_FRAME_OPTIONS",
	})
}
