package conf_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// The default database comes from the prefixed variable, else from the bare
// DATABASE_URL that hosting platforms set, else from the code.
func TestApplyEnvDatabaseURL(t *testing.T) {
	var (
		code     = conf.Database{Driver: conf.DriverSQLite, DSN: "code.db"}
		prefixed = conf.Database{Driver: conf.DriverPostgres, DSN: "postgres://app@db.example/prefixed"}
		bare     = conf.Database{Driver: conf.DriverSQLite, DSN: "bare.db"}
	)
	const (
		prefixedURL = "postgresql://app@db.example/prefixed"
		bareURL     = "sqlite:///bare.db"
	)
	tests := []struct {
		name   string
		prefix string
		vars   map[string]string
		want   conf.Database
	}{
		{"the prefixed variable", "P_", map[string]string{"P_DATABASE_URL": prefixedURL}, prefixed},
		{"the bare variable when the prefixed one is unset", "P_", map[string]string{"DATABASE_URL": bareURL}, bare},
		{
			"the prefixed variable wins over the bare one", "P_",
			map[string]string{"P_DATABASE_URL": prefixedURL, "DATABASE_URL": bareURL},
			prefixed,
		},
		{"no prefix", "", map[string]string{"DATABASE_URL": bareURL}, bare},
		{"neither is set", "P_", map[string]string{"OTHER_DATABASE_URL": bareURL}, code},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := conf.Defaults()
			s.Database = code
			if err := s.ApplyEnvFunc(tt.prefix, mapEnv(tt.vars)); err != nil {
				t.Fatal(err)
			}

			djangotest.AssertEqual(t, s.Database, tt.want)
			djangotest.AssertEqual(t, s.Databases, map[string]conf.Database{"default": tt.want})
			djangotest.AssertEqual(t, activate(t, s).DefaultDatabase(), tt.want)
		})
	}
}

// The variable also replaces a default database that the code declared in
// Databases, which otherwise wins over the Database field.
func TestApplyEnvDatabaseURLReplacesTheDefaultAlias(t *testing.T) {
	s := conf.Defaults()
	s.Databases = map[string]conf.Database{
		"default": {Driver: conf.DriverSQLite, DSN: "code.db"},
	}
	if err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{"P_DATABASE_URL": "sqlite:///env.db"})); err != nil {
		t.Fatal(err)
	}

	djangotest.AssertEqual(t, activate(t, s).DefaultDatabase(), conf.Database{Driver: conf.DriverSQLite, DSN: "env.db"})
}

// A malformed database URL is reported under the name of its variable, without
// the URL, and the database of the code is kept.
func TestApplyEnvMalformedDatabaseURL(t *testing.T) {
	code := conf.Database{Driver: conf.DriverSQLite, DSN: "code.db"}
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{
			"unknown scheme",
			map[string]string{"P_DATABASE_URL": "mysql://app:hunter2@db.example/app"},
			`conf: P_DATABASE_URL: unsupported database URL scheme "mysql" (want sqlite or postgres)`,
		},
		{
			"no scheme",
			map[string]string{"P_DATABASE_URL": "app:hunter2@db.example/app"},
			"conf: P_DATABASE_URL: database URL has no scheme (want sqlite:// or postgres://)",
		},
		{
			"set but empty",
			map[string]string{"P_DATABASE_URL": ""},
			"conf: P_DATABASE_URL: empty database URL",
		},
		{
			"the bare variable",
			map[string]string{"DATABASE_URL": "postgres://app:hunter2@db.example:port/app"},
			"conf: DATABASE_URL: malformed PostgreSQL URL: check the port, and percent-encode special characters " +
				"in the user name and the password (the URL is not shown, it may hold a password)",
		},
		{
			// A typo in the prefixed variable must not select another database.
			"a malformed prefixed variable does not fall back to the bare one",
			map[string]string{"P_DATABASE_URL": "db.sqlite3", "DATABASE_URL": "sqlite:///bare.db"},
			"conf: P_DATABASE_URL: database URL has no scheme (want sqlite:// or postgres://)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := conf.Defaults()
			s.Database = code
			err := s.ApplyEnvFunc("P_", mapEnv(tt.vars))

			djangotest.AssertEqual(t, errorLines(err), []string{tt.want})
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("the error holds the password: %v", err)
			}
			djangotest.AssertEqual(t, s.DefaultDatabase(), code)
		})
	}
}

// An alias that the code declares is read from prefix+"DATABASE_URL_"+ALIAS;
// the environment cannot add an alias.
func TestApplyEnvDatabaseAliases(t *testing.T) {
	s := conf.Defaults()
	s.Databases = map[string]conf.Database{
		"replica":   {Driver: conf.DriverPostgres, DSN: "postgres://replica.example/app"},
		"read-only": {DSN: "read-only.db"},
		"archive":   {Driver: conf.DriverSQLite, DSN: "archive.db"},
		"broken":    {Driver: conf.DriverSQLite, DSN: "broken.db"},
	}
	err := s.ApplyEnvFunc("P_", mapEnv(map[string]string{
		"P_DATABASE_URL_REPLICA":   "postgres://other.example/app",
		"P_DATABASE_URL_READ_ONLY": "sqlite:///env.db",
		"P_DATABASE_URL_BROKEN":    "mysql://app:hunter2@db.example/app",
		// Not read: an alias that the code does not declare, and an alias
		// variable without the prefix.
		"P_DATABASE_URL_ANALYTICS": "sqlite:///analytics.db",
		"DATABASE_URL_ARCHIVE":     "sqlite:///bare.db",
	}))

	djangotest.AssertEqual(t, errorLines(err), []string{
		`conf: P_DATABASE_URL_BROKEN: unsupported database URL scheme "mysql" (want sqlite or postgres)`,
	})
	djangotest.AssertEqual(t, s.Databases, map[string]conf.Database{
		"default":   {Driver: conf.DriverSQLite},
		"replica":   {Driver: conf.DriverPostgres, DSN: "postgres://other.example/app"},
		"read-only": {Driver: conf.DriverSQLite, DSN: "env.db"},
		"archive":   {Driver: conf.DriverSQLite, DSN: "archive.db"},
		"broken":    {Driver: conf.DriverSQLite, DSN: "broken.db"},
	})
}
