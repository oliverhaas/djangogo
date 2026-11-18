package conf_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// Every documented form of a database URL gives the driver and the DSN that the
// backend of the driver opens.
func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want conf.Database
	}{
		{
			name: "sqlite, relative path",
			raw:  "sqlite:///db.sqlite3",
			want: conf.Database{Driver: "sqlite", DSN: "db.sqlite3"},
		},
		{
			name: "sqlite, relative path in a directory",
			raw:  "sqlite:///data/db.sqlite3",
			want: conf.Database{Driver: "sqlite", DSN: "data/db.sqlite3"},
		},
		{
			name: "sqlite, absolute path",
			raw:  "sqlite:////var/lib/app/db.sqlite3",
			want: conf.Database{Driver: "sqlite", DSN: "/var/lib/app/db.sqlite3"},
		},
		{
			name: "sqlite, memory",
			raw:  "sqlite://:memory:",
			want: conf.Database{Driver: "sqlite", DSN: ":memory:"},
		},
		{
			name: "sqlite, memory after three slashes",
			raw:  "sqlite:///:memory:",
			want: conf.Database{Driver: "sqlite", DSN: ":memory:"},
		},
		{
			name: "sqlite, no path is memory",
			raw:  "sqlite://",
			want: conf.Database{Driver: "sqlite", DSN: ":memory:"},
		},
		{
			name: "sqlite, an empty path is memory",
			raw:  "sqlite:///",
			want: conf.Database{Driver: "sqlite", DSN: ":memory:"},
		},
		{
			name: "sqlite3 scheme",
			raw:  "sqlite3:///db.sqlite3",
			want: conf.Database{Driver: "sqlite", DSN: "db.sqlite3"},
		},
		{
			name: "sqlite, query string",
			raw:  "sqlite:///db.sqlite3?cache=shared&mode=rwc",
			want: conf.Database{Driver: "sqlite", DSN: "file:db.sqlite3?cache=shared&mode=rwc"},
		},
		{
			name: "sqlite, absolute path and query string",
			raw:  "sqlite:////var/lib/app/db.sqlite3?_pragma=busy_timeout(5000)",
			want: conf.Database{Driver: "sqlite", DSN: "file:/var/lib/app/db.sqlite3?_pragma=busy_timeout(5000)"},
		},
		{
			name: "sqlite, memory and query string",
			raw:  "sqlite://:memory:?cache=shared",
			want: conf.Database{Driver: "sqlite", DSN: "file::memory:?cache=shared"},
		},
		{
			name: "file URI",
			raw:  "file:db.sqlite3?cache=shared",
			want: conf.Database{Driver: "sqlite", DSN: "file:db.sqlite3?cache=shared"},
		},
		{
			name: "file URI, absolute path",
			raw:  "file:///var/lib/app/db.sqlite3",
			want: conf.Database{Driver: "sqlite", DSN: "file:///var/lib/app/db.sqlite3"},
		},
		{
			name: "postgres with a port",
			raw:  "postgres://app:secret@db.example:5432/app",
			want: conf.Database{Driver: "postgres", DSN: "postgres://app:secret@db.example:5432/app"},
		},
		{
			name: "postgres without a port",
			raw:  "postgres://app@db.example/app",
			want: conf.Database{Driver: "postgres", DSN: "postgres://app@db.example/app"},
		},
		{
			name: "postgres with a password and a query string",
			raw:  "postgres://app:s3cr%40t@db.example:5432/app?sslmode=disable&connect_timeout=5",
			want: conf.Database{
				Driver: "postgres",
				DSN:    "postgres://app:s3cr%40t@db.example:5432/app?sslmode=disable&connect_timeout=5",
			},
		},
		{
			name: "postgres without a host",
			raw:  "postgres:///app",
			want: conf.Database{Driver: "postgres", DSN: "postgres:///app"},
		},
		{
			name: "postgresql scheme",
			raw:  "postgresql://app@db.example/app",
			want: conf.Database{Driver: "postgres", DSN: "postgres://app@db.example/app"},
		},
		{
			name: "pgx scheme",
			raw:  "pgx://app@db.example/app",
			want: conf.Database{Driver: "postgres", DSN: "postgres://app@db.example/app"},
		},
		{
			name: "scheme in upper case",
			raw:  "POSTGRES://app@db.example/app",
			want: conf.Database{Driver: "postgres", DSN: "postgres://app@db.example/app"},
		},
		{
			name: "space around the value",
			raw:  "  sqlite:///db.sqlite3\n",
			want: conf.Database{Driver: "sqlite", DSN: "db.sqlite3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := conf.ParseDatabaseURL(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, got, tt.want)
		})
	}
}

// A value that is no database URL is refused with an error that never repeats
// the value, because it may hold a password.
func TestParseDatabaseURLErrors(t *testing.T) {
	const (
		noScheme  = "conf: database URL has no scheme (want sqlite:// or postgres://)"
		malformed = "conf: malformed PostgreSQL URL: check the port, and percent-encode special characters " +
			"in the user name and the password (the URL is not shown, it may hold a password)"
	)
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: "conf: empty database URL"},
		{name: "blank", raw: "  \t", want: "conf: empty database URL"},
		{
			name: "unknown scheme",
			raw:  "mysql://app:hunter2@db.example/app",
			want: `conf: unsupported database URL scheme "mysql" (want sqlite or postgres)`,
		},
		{name: "missing scheme", raw: "app:hunter2@db.example:5432/app", want: noScheme},
		{name: "a file name", raw: "db.sqlite3", want: noScheme},
		{name: "a keyword string", raw: "host=db.example user=app password=hunter2", want: noScheme},
		{name: "a password in the place of the scheme", raw: "app:hunter2@db.example://app", want: noScheme},
		{name: "malformed port", raw: "postgres://app:hunter2@db.example:port/app", want: malformed},
		{name: "malformed escape", raw: "postgres://app:hunter2%zz@db.example/app", want: malformed},
		{name: "space in the host", raw: "postgres://app:hunter2@db example/app", want: malformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := conf.ParseDatabaseURL(tt.raw)
			if err == nil {
				t.Fatalf("ParseDatabaseURL returned %+v and no error", got)
			}
			djangotest.AssertEqual(t, err.Error(), tt.want)
			djangotest.AssertEqual(t, got, conf.Database{})
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("the error holds the password: %v", err)
			}
		})
	}
}

// The spellings of a driver that the settings accept map to the two canonical
// names.
func TestNormalizeDriver(t *testing.T) {
	tests := []struct {
		driver string
		want   string
		wantOK bool
	}{
		{"", conf.DriverSQLite, true},
		{"sqlite", conf.DriverSQLite, true},
		{"sqlite3", conf.DriverSQLite, true},
		{"SQLite", conf.DriverSQLite, true},
		{"postgres", conf.DriverPostgres, true},
		{"postgresql", conf.DriverPostgres, true},
		{"pgx", conf.DriverPostgres, true},
		{" Postgres ", conf.DriverPostgres, true},
		{"mysql", "", false},
	}
	for _, tt := range tests {
		got, ok := conf.NormalizeDriver(tt.driver)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("NormalizeDriver(%q) = %q, %t, want %q, %t", tt.driver, got, ok, tt.want, tt.wantOK)
		}
	}
}
