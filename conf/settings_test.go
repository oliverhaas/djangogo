package conf_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
)

// Configure completes the settings and makes them the ones Active returns.
func TestConfigureReturnsTheActiveSettings(t *testing.T) {
	got := activate(t, conf.Settings{SecretKey: "k"})

	djangotest.AssertEqual(t, conf.Active() == got, true)
	djangotest.AssertEqual(t, got.SecretKey, "k")
	// Configure completes the literal.
	djangotest.AssertEqual(t, got.Host, "127.0.0.1")
	djangotest.AssertEqual(t, got.Port, "8000")
	djangotest.AssertEqual(t, len(got.AllowedHosts), 0)
}

// DefaultDatabase also answers for settings that were not completed yet.
func TestDefaultDatabaseOfALiteral(t *testing.T) {
	var (
		pg   = conf.Database{Driver: conf.DriverPostgres, DSN: "postgres://db.example/app"}
		lite = conf.Database{Driver: conf.DriverSQLite, DSN: "app.db"}
	)
	field := conf.Settings{Database: lite}
	djangotest.AssertEqual(t, field.DefaultDatabase(), lite)

	alias := conf.Settings{Database: lite, Databases: map[string]conf.Database{"default": pg}}
	djangotest.AssertEqual(t, alias.DefaultDatabase(), pg)

	other := conf.Settings{Database: lite, Databases: map[string]conf.Database{"replica": pg}}
	djangotest.AssertEqual(t, other.DefaultDatabase(), lite)
}

// Location loads the zone that TimeZone names; no name is UTC.
func TestLocation(t *testing.T) {
	unset := conf.Settings{}
	loc, err := unset.Location()
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, loc, time.UTC)

	berlin := conf.Settings{TimeZone: "Europe/Berlin"}
	loc, err = berlin.Location()
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, loc.String(), "Europe/Berlin")
}
