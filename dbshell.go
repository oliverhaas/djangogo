package djangogo

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
)

// dbshellCommand is Django's dbshell: it runs the command-line client of a
// database, sqlite3 or psql, connected with the settings of its DSN. The
// parameters after "--" are passed on to the client.
type dbshellCommand struct {
	app      *Application
	database string
}

// Name returns the command name.
func (*dbshellCommand) Name() string { return "dbshell" }

// Help returns the help text.
func (*dbshellCommand) Help() string {
	return "Runs the command-line client for specified database, or the default database if none is provided."
}

// Usage names the positional arguments.
func (*dbshellCommand) Usage() string { return "[-- parameters ...]" }

// SkipSystemChecks reports that dbshell runs no checks first
// (requires_system_checks = []).
func (*dbshellCommand) SkipSystemChecks() bool { return true }

// Flags declares Django's options of dbshell.
func (c *dbshellCommand) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.database, "database", conf.DefaultDatabaseAlias,
		"Nominates a database onto which to open a shell. Defaults to the \"default\" database.")
}

// Run runs the client and waits for it to end.
func (c *dbshellCommand) Run(parameters []string) error {
	a := c.app
	if err := checkDatabaseAliases(a.Settings, []string{c.database}); err != nil {
		return err
	}
	db := a.Settings.DefaultDatabase()
	if c.database != conf.DefaultDatabaseAlias {
		db = a.Settings.Databases[c.database]
	}
	args, env, err := dbshellArgs(db, parameters)
	if err != nil {
		return manage.Errorf("%w", err)
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		return manage.Errorf("You appear not to have the '%s' program installed or on your path.", args[0])
	}
	cmd := exec.Command(path, args[1:]...)
	cmd.Args[0] = args[0]
	cmd.Stdin, cmd.Stdout, cmd.Stderr = a.in(), a.out(), a.errOut()
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for _, k := range slices.Sorted(maps.Keys(env)) {
			cmd.Env = append(cmd.Env, k+"="+env[k])
		}
	}

	// Catch SIGINT while the client runs, so Ctrl-C aborts a query in the
	// client (which gets the signal from the terminal) and not this process.
	// A caught signal, unlike an ignored one, is not inherited.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code := exitErr.ExitCode()
		return &manage.CommandError{
			Msg:  fmt.Sprintf("\"%s\" returned non-zero exit status %d.", strings.Join(args, " "), code),
			Code: code,
		}
	case err != nil:
		return fmt.Errorf("djangogo: run %s: %w", args[0], err)
	}
	return nil
}

// dbshellArgs returns the command line of the client of db and the
// environment variables to add for it (Django's settings_to_cmd_args_env).
// The password of a PostgreSQL DSN goes into PGPASSWORD and never into the
// arguments, which other users of the machine can read.
func dbshellArgs(db conf.Database, parameters []string) (args []string, env map[string]string, err error) {
	if db.DSN == "" {
		return nil, nil, errors.New("no database is configured: set Settings.Database.DSN or DATABASE_URL")
	}
	driver, ok := conf.NormalizeDriver(db.Driver)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported database driver %q", db.Driver)
	}
	if driver == conf.DriverSQLite {
		return sqliteShellArgs(db.DSN, parameters), nil, nil
	}
	params, err := parsePostgresDSN(db.DSN)
	if err != nil {
		return nil, nil, err
	}
	args, env = psqlArgs(params, parameters)
	return args, env, nil
}

// sqliteShellArgs returns the sqlite3 command line for a SQLite DSN. A URI
// ("file:...") is passed as it is, since sqlite3 reads URIs; the query of a
// plain path holds options of the Go driver and is dropped.
func sqliteShellArgs(dsn string, parameters []string) []string {
	name := dsn
	if !strings.HasPrefix(dsn, "file:") {
		name, _, _ = strings.Cut(dsn, "?")
	}
	return append([]string{"sqlite3", name}, parameters...)
}

// psqlEnv maps the libpq connection keywords that psql reads from the
// environment to their variables, in the order of Django's client.
var psqlEnv = []struct{ keyword, variable string }{
	{"password", "PGPASSWORD"},
	{"service", "PGSERVICE"},
	{"sslmode", "PGSSLMODE"},
	{"sslrootcert", "PGSSLROOTCERT"},
	{"sslcert", "PGSSLCERT"},
	{"sslkey", "PGSSLKEY"},
	{"passfile", "PGPASSFILE"},
}

// psqlArgs builds the psql command line from libpq connection keywords, as
// Django's PostgreSQL client does: user, host and port as options, the
// parameters, then the database name ("postgres" when neither a name nor a
// service is given).
func psqlArgs(params map[string]string, parameters []string) (args []string, env map[string]string) {
	args = []string{"psql"}
	dbname := params["dbname"]
	if dbname == "" && params["service"] == "" {
		dbname = "postgres"
	}
	if user := params["user"]; user != "" {
		args = append(args, "-U", user)
	}
	if host := params["host"]; host != "" {
		args = append(args, "-h", host)
	}
	if port := params["port"]; port != "" {
		args = append(args, "-p", port)
	}
	args = append(args, parameters...)
	if dbname != "" {
		args = append(args, dbname)
	}
	for _, e := range psqlEnv {
		if v := params[e.keyword]; v != "" {
			if env == nil {
				env = map[string]string{}
			}
			env[e.variable] = v
		}
	}
	return args, env
}

// errMalformedDSN is the error for a PostgreSQL DSN that cannot be parsed. It
// never quotes the DSN, which usually holds a password.
var errMalformedDSN = errors.New("malformed PostgreSQL DSN (not shown, it may hold a password)")

// parsePostgresDSN returns the libpq connection keywords of a DSN in URL
// form ("postgres://user:password@host:port/dbname?sslmode=require") or in
// keyword/value form ("host=localhost dbname=app").
func parsePostgresDSN(dsn string) (map[string]string, error) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return parseKeywordDSN(dsn)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, errMalformedDSN
	}
	params := map[string]string{
		"host":   u.Hostname(),
		"port":   u.Port(),
		"dbname": strings.TrimPrefix(u.Path, "/"),
	}
	if u.User != nil {
		params["user"] = u.User.Username()
		params["password"], _ = u.User.Password()
	}
	for k, v := range u.Query() {
		if len(v) > 0 {
			params[k] = v[len(v)-1]
		}
	}
	return params, nil
}

// parseKeywordDSN parses libpq's keyword/value form: "key=value" pairs
// separated by spaces, where a value may be single-quoted and a backslash
// escapes the next character.
func parseKeywordDSN(dsn string) (map[string]string, error) {
	params := map[string]string{}
	s := strings.TrimSpace(dsn)
	for s != "" {
		key, rest, found := strings.Cut(s, "=")
		if !found {
			return nil, errMalformedDSN
		}
		key = strings.TrimSpace(key)
		rest = strings.TrimLeft(rest, " \t\n\r")
		value, rest, err := keywordValue(rest)
		if err != nil {
			return nil, err
		}
		params[key] = value
		s = strings.TrimSpace(rest)
	}
	return params, nil
}

// keywordValue reads one value of a keyword/value DSN from the start of s
// and returns it with the rest of s.
func keywordValue(s string) (value, rest string, err error) {
	var b strings.Builder
	quoted := strings.HasPrefix(s, "'")
	if quoted {
		s = s[1:]
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case quoted && ch == '\'':
			return b.String(), s[i+1:], nil
		case !quoted && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'):
			return b.String(), s[i:], nil
		default:
			b.WriteByte(ch)
		}
	}
	if quoted {
		return "", "", errMalformedDSN
	}
	return b.String(), "", nil
}
