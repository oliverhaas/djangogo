package conf

import (
	"reflect"
	"time"
)

const (
	defaultHost   = "127.0.0.1"
	defaultPort   = "8000"
	defaultDriver = DriverSQLite
)

// Defaults returns the settings with Django's default values. A project
// changes the fields it needs; a false or empty value set on the result is
// kept, which a plain literal passed to Configure cannot express.
//
// The values are those of django.conf.global_settings, where Django's None is
// the empty string or the nil slice, with these deliberate differences:
//
//   - TimeZone is "UTC", the value of Django's project template; the global
//     default is "America/Chicago".
//   - StaticURL is "/static/"; Django has no global default and its project
//     template sets "static/".
//   - Templates.AppDirs is true, as in the project template; the global
//     TEMPLATES is empty.
//   - CSRFHeaderName is "X-CSRFToken", the HTTP spelling of Django's WSGI name
//     "HTTP_X_CSRFTOKEN".
//   - PasswordHashers is empty, which selects the hashers of the auth package;
//     Django lists five dotted class paths.
//   - SessionEngine is SessionEngineDB, the short name of
//     "django.contrib.sessions.backends.db".
//   - Database has the driver "sqlite" and no DSN; Django's DATABASES is empty.
//   - Host and Port are the defaults of runserver, which are not settings in
//     Django.
func Defaults() Settings {
	return Settings{
		Host:      defaultHost,
		Port:      defaultPort,
		Database:  Database{Driver: defaultDriver},
		Templates: Templates{AppDirs: true},
		TimeZone:  "UTC",
		UseTZ:     true,

		StaticURL: "/static/",

		LoginURL:         "/accounts/login/",
		LoginRedirectURL: "/accounts/profile/",
		AppendSlash:      true,

		SessionEngine:         SessionEngineDB,
		SessionCookieName:     "sessionid",
		SessionCookieAge:      14 * 24 * time.Hour,
		SessionCookieHTTPOnly: true,
		SessionCookieSameSite: "Lax",
		SessionCookiePath:     "/",

		CSRFCookieName:     "csrftoken",
		CSRFCookieAge:      52 * 7 * 24 * time.Hour,
		CSRFCookieSameSite: "Lax",
		CSRFCookiePath:     "/",
		CSRFHeaderName:     "X-CSRFToken",

		SecureContentTypeNosniff:      true,
		SecureReferrerPolicy:          "same-origin",
		SecureCrossOriginOpenerPolicy: "same-origin",
		XFrameOptions:                 "DENY",

		defaulted: true,
	}
}

// applyDefaults fills every zero-valued field with its default, once. Settings
// that came from Defaults, or that already went through here, are left alone,
// so a value switched off afterwards stays off.
func (s *Settings) applyDefaults() {
	if !s.defaulted {
		d := Defaults()
		fillZero(reflect.ValueOf(s).Elem(), reflect.ValueOf(&d).Elem())
		s.defaulted = true
	}
	s.syncDatabases()
}

// fillZero copies src into every zero-valued exported field of dst,
// descending into nested structs so a partly set struct keeps its values.
func fillZero(dst, src reflect.Value) {
	for i := range dst.NumField() {
		if !dst.Type().Field(i).IsExported() {
			continue
		}
		df, sf := dst.Field(i), src.Field(i)
		if df.Kind() == reflect.Struct {
			fillZero(df, sf)
			continue
		}
		if df.IsZero() {
			df.Set(sf)
		}
	}
}

// syncDatabases keeps Database and Databases["default"] the same value and
// gives every database without a driver the default one.
//
// When the two differ, the one that was changed since the last call wins, so a
// change to either of them is kept. On the first call, and when both were
// changed, the entry of Databases wins, unless it has no DSN and the Database
// field has one.
func (s *Settings) syncDatabases() {
	entry, hasEntry := s.Databases[DefaultDatabaseAlias]
	switch {
	case !hasEntry:
	case s.syncedDatabase != nil && entry == *s.syncedDatabase:
		// The entry is as the last call left it, so a difference comes from
		// the Database field.
	case entry.DSN != "" || s.Database.DSN == "":
		s.Database = entry
	}
	if s.Database.Driver == "" {
		s.Database.Driver = defaultDriver
	}
	synced := s.Database
	s.syncedDatabase = &synced
	if s.Database.DSN == "" && s.Databases == nil {
		return
	}

	// The map may be shared with a copy of the settings or belong to the
	// caller, so it is replaced and not written to.
	databases := make(map[string]Database, len(s.Databases)+1)
	for alias, db := range s.Databases {
		if db.Driver == "" {
			db.Driver = defaultDriver
		}
		databases[alias] = db
	}
	databases[DefaultDatabaseAlias] = s.Database
	s.Databases = databases
}
