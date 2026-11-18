package djangogo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/migrations"
)

// The tests in this file are ports of the runserver tests of Django's
// tests/admin_scripts/tests.py. Django stubs run() and calls the command with
// call_command; here RunserverAddr returns the host and port the command
// listens on, and the on_bind tests run the real server on a free port.
// Django's WSGI wording ("Starting WSGI development server", "Django
// version", its production warning with a docs URL) differs from the
// Djan-Go-Go banner, so the ports assert the Go text.

// lockedBuffer is a bytes.Buffer that is safe to write from the server
// goroutine while a test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p to the buffer.
func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the text written so far.
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Reset empties the buffer.
func (b *lockedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// bannerTime is the fixed time of the banner in the tests.
var bannerTime = time.Date(2026, 11, 18, 9, 5, 7, 0, time.UTC)

// runserverApp returns an application for s with buffers for Out and Err.
func runserverApp(t *testing.T, s conf.Settings) (app *djangogo.Application, out, errOut *lockedBuffer) {
	t.Helper()
	if s.SecretKey == "" {
		s.SecretKey = "k"
	}
	app, err := djangogo.New(s)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	out, errOut = &lockedBuffer{}, &lockedBuffer{}
	app.Out, app.Err = out, errOut
	return app, out, errOut
}

// serveRunserver runs "runserver" with args on app, calls do with the
// listening address from a goroutine (the server stops only after the ready
// hook returns; its writes are visible once the command returns), stops the
// server when do returns, and returns the error of the command.
func serveRunserver(t *testing.T, app *djangogo.Application, do func(addr net.Addr), args ...string) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	djangogo.SetRunserverHooks(ctx, app, func(addr net.Addr) {
		go func() {
			defer cancel()
			if do != nil {
				do(addr)
			}
		}()
	}, func() time.Time { return bannerTime })
	err := app.Execute(append([]string{"runserver"}, args...))
	return err
}

// assertServerSettings checks the host and port runserver listens on for
// addrport. Django also compares use_ipv6 and _raw_ipv6; Go has no
// _raw_ipv6, and the host of an IPv6 address comes without brackets.
func assertServerSettings(t *testing.T, app *djangogo.Application, addrport string, ipv6 bool, addr, port string) {
	t.Helper()
	host, gotPort, err := djangogo.RunserverAddr(app, addrport, ipv6)
	if err != nil {
		t.Fatalf("RunserverAddr(%q, %v): %v", addrport, ipv6, err)
	}
	if host != addr || gotPort != port {
		t.Errorf("RunserverAddr(%q, %v) = %q, %q; want %q, %q", addrport, ipv6, host, gotPort, addr, port)
	}
}

// plainRunserverApp returns an application with the default settings.
func plainRunserverApp(t *testing.T) *djangogo.Application {
	t.Helper()
	app, _, _ := runserverApp(t, conf.Settings{Debug: true})
	return app
}

// TestRunserverAddrport checks the address of runserver without arguments,
// with an address and port, and with a port only.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_runserver_addrport
func TestRunserverAddrport(t *testing.T) {
	app := plainRunserverApp(t)
	assertServerSettings(t, app, "", false, "127.0.0.1", "8000")
	assertServerSettings(t, app, "1.2.3.4:8000", false, "1.2.3.4", "8000")
	assertServerSettings(t, app, "7000", false, "127.0.0.1", "7000")
}

// TestZeroIPAddr checks that the banner of the address "0" shows 0.0.0.0 and
// that the production warning shows unless it is hidden.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_zero_ip_addr
func TestZeroIPAddr(t *testing.T) {
	t.Setenv("DJANGOGO_RUNSERVER_HIDE_WARNING", "anything-but-true")
	app, out, _ := runserverApp(t, conf.Settings{Debug: true})
	var port int
	err := serveRunserver(t, app, func(addr net.Addr) { port = addr.(*net.TCPAddr).Port }, "0:0")
	if err != nil {
		t.Fatalf("runserver: %v", err)
	}
	want := fmt.Sprintf("Starting development server at http://0.0.0.0:%d/", port)
	if !strings.Contains(out.String(), want) {
		t.Errorf("Out = %q, want it to contain %q", out.String(), want)
	}
	if !strings.Contains(out.String(), "WARNING: This is a development server. Do not use it in a production setting.") {
		t.Errorf("Out = %q, want the production warning", out.String())
	}
}

// TestOnBind checks the startup lines of the server on a free port.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_on_bind
func TestOnBind(t *testing.T) {
	t.Setenv("DJANGOGO_RUNSERVER_HIDE_WARNING", "anything-but-true")
	app, out, _ := runserverApp(t, conf.Settings{Debug: true})
	var port int
	err := serveRunserver(t, app, func(addr net.Addr) { port = addr.(*net.TCPAddr).Port }, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("runserver: %v", err)
	}
	want := fmt.Sprintf("Starting development server at http://127.0.0.1:%d/", port)
	if !strings.Contains(out.String(), want) {
		t.Errorf("Out = %q, want it to contain %q", out.String(), want)
	}
	if !strings.Contains(out.String(), "WARNING: This is a development server. Do not use it in a production setting.") {
		t.Errorf("Out = %q, want the production warning", out.String())
	}
}

// TestHideProductionWarningWithEnvironmentVariable checks that
// DJANGOGO_RUNSERVER_HIDE_WARNING=true hides the production warning.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_hide_production_warning_with_environment_variable
func TestHideProductionWarningWithEnvironmentVariable(t *testing.T) {
	t.Setenv("DJANGOGO_RUNSERVER_HIDE_WARNING", "true")
	app, out, _ := runserverApp(t, conf.Settings{Debug: true})
	var port int
	err := serveRunserver(t, app, func(addr net.Addr) { port = addr.(*net.TCPAddr).Port }, "0:0")
	if err != nil {
		t.Fatalf("runserver: %v", err)
	}
	want := fmt.Sprintf("Starting development server at http://0.0.0.0:%d/", port)
	if !strings.Contains(out.String(), want) {
		t.Errorf("Out = %q, want it to contain %q", out.String(), want)
	}
	if strings.Contains(out.String(), "WARNING: This is a development server.") {
		t.Errorf("Out = %q, want no production warning", out.String())
	}
}

// TestRunnerAddrportIPv6 checks the IPv6 defaults and a bracketed address.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_runner_addrport_ipv6
func TestRunnerAddrportIPv6(t *testing.T) {
	app := plainRunserverApp(t)
	assertServerSettings(t, app, "", true, "::1", "8000")
	assertServerSettings(t, app, "7000", true, "::1", "7000")
	assertServerSettings(t, app, "[2001:0db8:1234:5678::9]:7000", false, "2001:0db8:1234:5678::9", "7000")
}

// TestRunnerHostname checks host names as the address.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_runner_hostname
func TestRunnerHostname(t *testing.T) {
	app := plainRunserverApp(t)
	assertServerSettings(t, app, "localhost:8000", false, "localhost", "8000")
	assertServerSettings(t, app, "test.domain.local:7000", false, "test.domain.local", "7000")
}

// TestRunnerHostnameIPv6 checks that a host name is accepted with --ipv6.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_runner_hostname_ipv6
func TestRunnerHostnameIPv6(t *testing.T) {
	app := plainRunserverApp(t)
	assertServerSettings(t, app, "test.domain.local:7000", true, "test.domain.local", "7000")
}

// TestRunnerCustomDefaults checks the default address and port. Django sets
// the attributes default_addr and default_port of the command; Djan-Go-Go
// reads Settings.Host and Settings.Port.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_runner_custom_defaults
func TestRunnerCustomDefaults(t *testing.T) {
	app, _, _ := runserverApp(t, conf.Settings{Debug: true, Host: "0.0.0.0", Port: "5000"})
	assertServerSettings(t, app, "", false, "0.0.0.0", "5000")
}

// TestRunnerCustomDefaultsIPv6 is skipped: Django's default_addr_ipv6 has no
// setting here, the IPv6 default address is always ::1.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_runner_custom_defaults_ipv6
func TestRunnerCustomDefaultsIPv6(t *testing.T) {
	t.Skip("there is no setting for the IPv6 default address; it is always ::1")
}

// TestRunnerAmbiguous checks that a host name made of hexadecimal digits is
// not read as an IPv6 address.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_runner_ambiguous
func TestRunnerAmbiguous(t *testing.T) {
	app := plainRunserverApp(t)
	// Only 4 characters, all of which could be in an ipv6 address.
	assertServerSettings(t, app, "beef:7654", false, "beef", "7654")
	// Uses only characters that could be in an ipv6 address.
	assertServerSettings(t, app, "deadbeef:7654", false, "deadbeef", "7654")
}

// TestNoDatabase checks that the migrations notice does not choke on a
// project without a database.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_no_database
func TestNoDatabase(t *testing.T) {
	app, out, _ := runserverApp(t, conf.Settings{Debug: true})
	app.Migrations.Add(migrations.Migration{App: "blog", Name: "0001_initial"})
	djangogo.CheckMigrations(context.Background(), app)
	if out.String() != "" {
		t.Errorf("Out = %q, want no output", out.String())
	}
}

// sqliteMemoryDSN returns the DSN of a shared in-memory SQLite database that
// is unique to the test.
func sqliteMemoryDSN(t *testing.T) string {
	t.Helper()
	return "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
}

// migrationApp returns an application with a database and no applied
// migrations, and the migrations 0001_initial of the given apps.
func migrationApp(t *testing.T, apps ...string) (app *djangogo.Application, out *lockedBuffer) {
	t.Helper()
	app, out, _ = runserverApp(t, conf.Settings{
		Debug:    true,
		Database: conf.Database{Driver: conf.DriverSQLite, DSN: sqliteMemoryDSN(t)},
	})
	for _, name := range apps {
		app.Migrations.Add(migrations.Migration{App: name, Name: "0001_initial"})
	}
	return app, out
}

// TestReadonlyDatabase checks that the notice appears when the table of
// applied migrations does not exist, as for a read-only database where
// Django's has_table is False.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_readonly_database
func TestReadonlyDatabase(t *testing.T) {
	app, out := migrationApp(t, "blog")
	djangogo.CheckMigrations(context.Background(), app)
	if !strings.Contains(out.String(), "unapplied migration(s)") {
		t.Errorf("Out = %q, want the unapplied migrations notice", out.String())
	}
}

// TestSkipChecks checks that --skip-checks leaves out the system checks and
// that they run without it.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_skip_checks
func TestSkipChecks(t *testing.T) {
	app, out, _ := runserverApp(t, conf.Settings{Debug: true})
	if err := serveRunserver(t, app, nil, "--skip-checks", "127.0.0.1:0"); err != nil {
		t.Fatalf("runserver --skip-checks: %v", err)
	}
	if strings.Contains(out.String(), "Performing system checks...") {
		t.Errorf("Out = %q, want no system checks", out.String())
	}

	out.Reset()
	if err := serveRunserver(t, app, nil, "127.0.0.1:0"); err != nil {
		t.Fatalf("runserver: %v", err)
	}
	if !strings.Contains(out.String(), "Performing system checks...\n\n") {
		t.Errorf("Out = %q, want the system checks", out.String())
	}
}

// TestCustomSystemChecks checks that a failing system check stops runserver
// before it listens. Django registers a check with the signals tag and
// expects SystemCheckError; here the check is in a fresh registry.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserver.test_custom_system_checks
func TestCustomSystemChecks(t *testing.T) {
	app, _, _ := runserverApp(t, conf.Settings{Debug: true})
	app.Checks = checks.NewRegistry()
	app.Checks.Register(func(*checks.Context) []checks.Message {
		return []checks.Message{{Level: checks.LevelError, Msg: "my error"}}
	}, checks.TagSignals)

	called := false
	err := serveRunserver(t, app, func(net.Addr) { called = true }, "127.0.0.1:0")
	var sce *checks.SystemCheckError
	if !errors.As(err, &sce) {
		t.Fatalf("runserver error = %v, want a *checks.SystemCheckError", err)
	}
	if called {
		t.Error("the server listened although a check failed")
	}
}

// TestMigrationWarningOneApp checks the notice for one app with a migration
// that is not applied.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserverMigrationWarning.test_migration_warning_one_app
func TestMigrationWarningOneApp(t *testing.T) {
	app, out := migrationApp(t, "app_waiting_migration")
	djangogo.CheckMigrations(context.Background(), app)
	for _, want := range []string{
		"You have 1 unapplied migration(s)",
		"apply the migrations for app(s): app_waiting_migration.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Out = %q, want it to contain %q", out.String(), want)
		}
	}
}

// TestMigrationWarningMultipleApps checks the notice for two apps, sorted.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserverMigrationWarning.test_migration_warning_multiple_apps
func TestMigrationWarningMultipleApps(t *testing.T) {
	app, out := migrationApp(t, "app_waiting_migration", "another_app_waiting_migration")
	djangogo.CheckMigrations(context.Background(), app)
	for _, want := range []string{
		"You have 2 unapplied migration(s)",
		"apply the migrations for app(s): another_app_waiting_migration, app_waiting_migration.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Out = %q, want it to contain %q", out.String(), want)
		}
	}
}

// TestEmptyAllowedHostsError checks that runserver refuses to start with
// Debug off and no allowed hosts. Django runs manage.py in a subprocess;
// here Run returns the exit code.
//
// Ported from Django: tests/admin_scripts/tests.py::ManageRunserverEmptyAllowedHosts.test_empty_allowed_hosts_error
func TestEmptyAllowedHostsError(t *testing.T) {
	app, out, errOut := runserverApp(t, conf.Settings{})
	if code := app.Run([]string{"runserver"}); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if out.String() != "" {
		t.Errorf("Out = %q, want no output", out.String())
	}
	want := "CommandError: You must set settings.ALLOWED_HOSTS if DEBUG is False.\n"
	if errOut.String() != want {
		t.Errorf("Err = %q, want %q", errOut.String(), want)
	}
}
