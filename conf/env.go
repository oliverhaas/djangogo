package conf

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
)

// databaseURLVar is the variable that holds the URL of the default database.
// Hosting platforms set it without a prefix.
const databaseURLVar = "DATABASE_URL"

// envSource is an environment a Settings value was overlaid with: the prefix
// of the variable names and the lookup that reads them.
type envSource struct {
	// prefix is put in front of every variable name, for example "DJANGOGO_".
	prefix string
	// lookup returns the value of a variable and whether it is set.
	lookup func(name string) (string, bool)
	// earlier is the source of the overlay before this one. A source is never
	// changed after it is linked, so copies of a Settings value share the chain.
	earlier *envSource
}

// replay sets the fields of the struct v, the value of a settings namespace,
// from every overlay of the chain that ends in e, the earliest one first, so
// that a later overlay wins. It returns one error per malformed variable. A
// nil source is the empty chain.
func (e *envSource) replay(v reflect.Value, path string) []error {
	if e == nil {
		return nil
	}
	return append(e.earlier.replay(v, path), e.apply(v, path, true)...)
}

// ApplyEnv overlays the settings with the process environment: for every
// field with an env tag, the variable prefix+tag replaces the field when it is
// set. With the prefix "DJANGOGO_", DJANGOGO_DEBUG sets Debug and
// DJANGOGO_SESSION_COOKIE_AGE sets SessionCookieAge. The fields of Templates
// are read through their own tags (DJANGOGO_TEMPLATES_DIRS).
//
// The defaults are applied first, so the order is always defaults, code,
// environment, and a variable that switches off a setting whose default is on
// is honored.
//
// The syntax of a value follows the type of the field:
//
//   - string: the value as it is.
//   - bool: 1, true, t, yes, y, on, or 0, false, f, no, n, off, in any letter case.
//   - int: a decimal integer.
//   - time.Duration: a whole number of seconds, the unit of Django's settings,
//     or a Go duration such as "36h".
//   - []string: a comma-separated list. Spaces around an element are trimmed
//     and empty elements are dropped; an empty value is an empty list. An
//     element cannot hold a comma.
//
// The default database is read from prefix+"DATABASE_URL" and, when that is
// unset, from DATABASE_URL, the name hosting platforms set; see
// ParseDatabaseURL for the syntax. Every other alias of Databases is read from
// prefix+"DATABASE_URL_"+ALIAS, the alias in upper case: DATABASE_URL_REPLICA
// replaces Databases["replica"]. Only an alias that the code declares is read,
// so the environment cannot add a database that nothing uses.
//
// A variable that is set but malformed is an error that names the variable,
// for example `conf: DJANGOGO_DEBUG: invalid boolean "maybe"`, and its field
// keeps its value. The errors of all malformed variables are joined. No error
// text holds a database URL.
//
// The overlay is remembered: a settings namespace read with Get takes its
// values from the same variables.
func (s *Settings) ApplyEnv(prefix string) error {
	return s.ApplyEnvFunc(prefix, os.LookupEnv)
}

// ApplyEnvFunc is ApplyEnv with an explicit lookup in place of the process
// environment, for tests and for values read from a file. lookup has the
// semantics of os.LookupEnv, and it must be safe for concurrent use when Get
// is called from several goroutines.
func (s *Settings) ApplyEnvFunc(prefix string, lookup func(name string) (string, bool)) error {
	if lookup == nil {
		return errors.New("conf: nil environment lookup")
	}
	s.applyDefaults()
	src := &envSource{prefix: prefix, lookup: lookup, earlier: s.overlay}
	s.overlay = src

	errs := src.apply(reflect.ValueOf(s).Elem(), "", false)
	errs = append(errs, s.overlayDatabases(src)...)
	return errors.Join(errs...)
}

// FromEnv returns Defaults overlaid with the process environment, as ApplyEnv
// does. The settings are returned also when a variable is malformed; the
// fields of the malformed variables then hold their defaults.
func FromEnv(prefix string) (Settings, error) {
	s := Defaults()
	err := s.ApplyEnv(prefix)
	return s, err
}

// overlayDatabases replaces the default database with the one DATABASE_URL
// names and every other alias with the one its DATABASE_URL_<ALIAS> names.
func (s *Settings) overlayDatabases(src *envSource) []error {
	var errs []error
	found := make(map[string]Database)
	read := func(alias, name string) bool {
		raw, set := src.lookup(name)
		if !set {
			return false
		}
		db, err := parseDatabaseURL(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("conf: %s: %w", name, err))
			return true
		}
		found[alias] = db
		return true
	}

	if !read(DefaultDatabaseAlias, src.prefix+databaseURLVar) && src.prefix != "" {
		read(DefaultDatabaseAlias, databaseURLVar)
	}
	for _, alias := range slices.Sorted(maps.Keys(s.Databases)) {
		if alias != DefaultDatabaseAlias {
			read(alias, src.prefix+databaseURLVar+"_"+aliasEnvName(alias))
		}
	}
	if len(found) == 0 {
		return errs
	}

	// The map may be shared with a copy of the settings, so it is replaced and
	// not written to.
	databases := make(map[string]Database, len(s.Databases)+len(found))
	maps.Copy(databases, s.Databases)
	maps.Copy(databases, found)
	s.Databases = databases
	if db, ok := found[DefaultDatabaseAlias]; ok {
		s.Database = db
	}
	s.syncDatabases()
	return errs
}

// aliasEnvName returns the part of a variable name that stands for a database
// alias: the alias in upper case, with an underscore for every character that
// a variable name cannot hold.
func aliasEnvName(alias string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			return r
		}
		return '_'
	}, alias)
}
