package djangogo_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/manage"
)

// The tests in this file are ports of Django's tests/dbshell/test_postgresql.py,
// test_sqlite.py and tests.py. Django passes a settings dict (NAME, USER,
// PASSWORD, HOST, PORT, OPTIONS) to settings_to_cmd_args_env; Djan-Go-Go takes
// a conf.Database, so each dict is written as a DSN: the URL form, with the
// OPTIONS in the query, or the libpq keyword form where a value needs quoting.
// Django's None environment is a nil map. test_mysql.py and test_oracle.py are
// not ported, there are no such backends.

// pgDB returns a PostgreSQL database with the DSN dsn.
func pgDB(dsn string) conf.Database {
	return conf.Database{Driver: "postgres", DSN: dsn}
}

// sqliteDB returns a SQLite database with the DSN dsn.
func sqliteDB(dsn string) conf.Database {
	return conf.Database{Driver: "sqlite", DSN: dsn}
}

// fakeClient writes an executable script named name that exits with status 1
// into a new directory and makes that directory the whole PATH.
func fakeClient(t *testing.T, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script cannot stand in for the client on Windows")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, name)
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake client: %v", err)
	}
	t.Setenv("PATH", dir)
}

// dbshellApp returns an application on db with buffers for its streams. The
// database is opened lazily, so a DSN of a host or file that does not exist
// is fine as long as nothing queries it.
func dbshellApp(t *testing.T, db conf.Database) *djangogo.Application {
	t.Helper()
	app, err := djangogo.New(conf.Settings{SecretKey: "k", Database: db})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	app.Out, app.Err, app.In = &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	return app
}

// checkShellArgs asserts the command line and the environment dbshell builds
// for db and parameters.
func checkShellArgs(t *testing.T, db conf.Database, parameters, wantArgs []string, wantEnv map[string]string) {
	t.Helper()
	args, env, err := djangogo.DBShellArgs(db, parameters)
	if err != nil {
		t.Fatalf("DBShellArgs: %v", err)
	}
	djangotest.AssertEqual(t, args, wantArgs)
	djangotest.AssertEqual(t, env, wantEnv)
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_basic
func TestPostgreSQLDbshellBasic(t *testing.T) {
	checkShellArgs(t,
		pgDB("postgres://someuser:somepassword@somehost:444/dbname"), nil,
		[]string{"psql", "-U", "someuser", "-h", "somehost", "-p", "444", "dbname"},
		map[string]string{"PGPASSWORD": "somepassword"})
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_nopass
func TestPostgreSQLDbshellNopass(t *testing.T) {
	checkShellArgs(t,
		pgDB("postgres://someuser@somehost:444/dbname"), nil,
		[]string{"psql", "-U", "someuser", "-h", "somehost", "-p", "444", "dbname"},
		nil)
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_ssl_certificate
func TestPostgreSQLDbshellSSLCertificate(t *testing.T) {
	checkShellArgs(t,
		pgDB("postgres://someuser@somehost:444/dbname?sslmode=verify-ca&sslrootcert=root.crt&sslcert=client.crt&sslkey=client.key"), nil,
		[]string{"psql", "-U", "someuser", "-h", "somehost", "-p", "444", "dbname"},
		map[string]string{
			"PGSSLCERT":     "client.crt",
			"PGSSLKEY":      "client.key",
			"PGSSLMODE":     "verify-ca",
			"PGSSLROOTCERT": "root.crt",
		})
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_service
func TestPostgreSQLDbshellService(t *testing.T) {
	checkShellArgs(t,
		pgDB("service=django_test"), nil,
		[]string{"psql"},
		map[string]string{"PGSERVICE": "django_test"})
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_passfile
func TestPostgreSQLDbshellPassfile(t *testing.T) {
	checkShellArgs(t,
		pgDB("postgres://someuser@somehost:444/dbname?passfile=~/.custompgpass"), nil,
		[]string{"psql", "-U", "someuser", "-h", "somehost", "-p", "444", "dbname"},
		map[string]string{"PGPASSFILE": "~/.custompgpass"})
	checkShellArgs(t,
		pgDB("service=django_test passfile=~/.custompgpass"), nil,
		[]string{"psql"},
		map[string]string{"PGSERVICE": "django_test", "PGPASSFILE": "~/.custompgpass"})
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_column
func TestPostgreSQLDbshellColumn(t *testing.T) {
	checkShellArgs(t,
		pgDB("postgres://some%3Auser:some%3Apassword@[::1]:444/dbname"), nil,
		[]string{"psql", "-U", "some:user", "-h", "::1", "-p", "444", "dbname"},
		map[string]string{"PGPASSWORD": "some:password"})
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_accent
func TestPostgreSQLDbshellAccent(t *testing.T) {
	username := "rôle"
	password := "sésame"
	checkShellArgs(t,
		pgDB("host=somehost port=444 dbname=dbname user="+username+" password="+password), nil,
		[]string{"psql", "-U", username, "-h", "somehost", "-p", "444", "dbname"},
		map[string]string{"PGPASSWORD": password})
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_parameters
func TestPostgreSQLDbshellParameters(t *testing.T) {
	checkShellArgs(t,
		pgDB("dbname=dbname"), []string{"--help"},
		[]string{"psql", "--help", "dbname"},
		nil)
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_sigint_handler
func TestPostgreSQLDbshellSigintHandler(t *testing.T) {
	t.Skip("Go cannot read the SIGINT handler of the process the way Python's signal.getsignal does; " +
		"the command catches SIGINT with signal.Notify while the client runs and stops catching it after")
}

// Ported from Django: tests/dbshell/test_postgresql.py::PostgreSqlDbshellCommandTestCase.test_crash_password_does_not_leak
func TestPostgreSQLDbshellCrashPasswordDoesNotLeak(t *testing.T) {
	fakeClient(t, "psql")
	app := dbshellApp(t, pgDB("postgres://someuser:somepassword@somehost:444/dbname"))
	err := app.Execute([]string{"dbshell"})
	if err == nil {
		t.Fatal("Execute(dbshell) succeeded, want the exit status of the client")
	}
	want := `"psql -U someuser -h somehost -p 444 dbname" returned non-zero exit status 1.`
	djangotest.AssertEqual(t, err.Error(), want)
	if strings.Contains(err.Error(), "somepassword") {
		t.Errorf("error %q leaks the password", err)
	}
}

// Ported from Django: tests/dbshell/test_sqlite.py::SqliteDbshellCommandTestCase.test_path_name
func TestSqliteDbshellPathName(t *testing.T) {
	checkShellArgs(t, sqliteDB("test.db.sqlite3"), nil,
		[]string{"sqlite3", "test.db.sqlite3"}, nil)
}

// Ported from Django: tests/dbshell/test_sqlite.py::SqliteDbshellCommandTestCase.test_parameters
func TestSqliteDbshellParameters(t *testing.T) {
	checkShellArgs(t, sqliteDB("test.db.sqlite3"), []string{"-help"},
		[]string{"sqlite3", "test.db.sqlite3", "-help"}, nil)
}

// Ported from Django: tests/dbshell/test_sqlite.py::SqliteDbshellCommandTestCase.test_non_zero_exit_status_when_path_to_db_is_path
func TestSqliteDbshellNonZeroExitStatusWhenPathToDBIsPath(t *testing.T) {
	fakeClient(t, "sqlite3")
	app := dbshellApp(t, sqliteDB("test.db.sqlite3"))
	err := app.Execute([]string{"dbshell"})
	var cmdErr *manage.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Execute(dbshell) = %v, want a *manage.CommandError", err)
	}
	djangotest.AssertEqual(t, cmdErr.Msg, `"sqlite3 test.db.sqlite3" returned non-zero exit status 1.`)
	djangotest.AssertEqual(t, cmdErr.Code, 1)
}

// Ported from Django: tests/dbshell/tests.py::DbshellCommandTestCase.test_command_missing
func TestDbshellCommandMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	app := dbshellApp(t, sqliteDB("test.db.sqlite3"))
	err := app.Execute([]string{"dbshell"})
	if err == nil {
		t.Fatal("Execute(dbshell) succeeded, want an error")
	}
	djangotest.AssertEqual(t, err.Error(), "You appear not to have the 'sqlite3' program installed or on your path.")
}
