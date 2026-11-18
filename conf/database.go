package conf

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Canonical driver names returned by NormalizeDriver.
const (
	// DriverSQLite is the SQLite backend.
	DriverSQLite = "sqlite"
	// DriverPostgres is the PostgreSQL backend.
	DriverPostgres = "postgres"
)

// NormalizeDriver maps the accepted spellings of a database driver to its
// canonical name: "sqlite" and "sqlite3" to DriverSQLite, "postgres",
// "postgresql" and "pgx" to DriverPostgres. An empty name is the default
// driver, SQLite. It reports false for an unknown driver.
func NormalizeDriver(driver string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "sqlite", "sqlite3":
		return DriverSQLite, true
	case "postgres", "postgresql", "pgx":
		return DriverPostgres, true
	}
	return "", false
}

// ParseDatabaseURL turns a DATABASE_URL-style value into a Database.
//
// PostgreSQL: "postgres://user:password@host:5432/name?sslmode=disable"; the
// schemes "postgresql" and "pgx" are accepted too. The DSN is the URL with the
// scheme "postgres", which the driver understands as it is.
//
// SQLite: "sqlite:///db.sqlite3" is a path relative to the working directory,
// "sqlite:////var/lib/app/db.sqlite3" an absolute one, and "sqlite://:memory:"
// (or "sqlite://") an in-memory database. A query string is passed on to the
// driver. A value that starts with "file:" is taken as a SQLite URI as it is.
//
// The error of a malformed value never quotes the value, because a database
// URL usually holds a password.
func ParseDatabaseURL(raw string) (Database, error) {
	db, err := parseDatabaseURL(raw)
	if err != nil {
		return Database{}, fmt.Errorf("conf: %w", err)
	}
	return db, nil
}

// parseDatabaseURL is ParseDatabaseURL with errors that lack the package
// prefix, for a caller that names the variable the value came from.
func parseDatabaseURL(raw string) (Database, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Database{}, errors.New("empty database URL")
	}
	if strings.HasPrefix(raw, "file:") {
		return Database{Driver: DriverSQLite, DSN: raw}, nil
	}
	scheme, rest, found := strings.Cut(raw, "://")
	if !found || !isURLScheme(scheme) {
		// What stands in front of "://" may be a user name and a password, so
		// it is not quoted.
		return Database{}, errors.New("database URL has no scheme (want sqlite:// or postgres://)")
	}
	driver, ok := NormalizeDriver(scheme)
	if !ok {
		return Database{}, fmt.Errorf("unsupported database URL scheme %q (want sqlite or postgres)", scheme)
	}
	if driver == DriverSQLite {
		return parseSQLiteURL(rest), nil
	}
	if _, err := url.Parse(raw); err != nil {
		// The error of url.Parse quotes the URL or a part of it, and an
		// unescaped password is a likely cause, so the error is not passed on.
		return Database{}, errors.New("malformed PostgreSQL URL: check the port, and percent-encode " +
			"special characters in the user name and the password (the URL is not shown, it may hold a password)")
	}
	return Database{Driver: DriverPostgres, DSN: "postgres://" + rest}, nil
}

// isURLScheme reports whether s has the syntax of a URL scheme: a letter, then
// letters, digits, "+", "-" and ".".
func isURLScheme(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.'):
		default:
			return false
		}
	}
	return s != ""
}

// parseSQLiteURL builds the SQLite database for the part of a sqlite:// URL
// after the scheme: "/relative.db", "//absolute.db", ":memory:" or "".
func parseSQLiteURL(rest string) Database {
	path, query, _ := strings.Cut(rest, "?")
	// The first slash only ends the empty host; a second one makes the path
	// absolute.
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = ":memory:"
	}
	if query == "" {
		return Database{Driver: DriverSQLite, DSN: path}
	}
	// SQLite reads its parameters only from the URI form of a file name.
	return Database{Driver: DriverSQLite, DSN: "file:" + path + "?" + query}
}
