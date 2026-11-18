package djangogo

import (
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/manage"
)

// registerCoreCommands registers the commands of the project core after the
// built-in ones of New, then the commands of the apps, and installs the
// system checks that run before a command.
func (a *Application) registerCoreCommands() {
	r := a.Commands
	r.Version = Version
	r.Err = a.errOut()
	r.SystemChecks = a.systemChecks
	// Django's TemplateCommand and version need no project to check.
	r.ExemptFromChecks("version", "startproject", "startapp")

	_ = r.Register(&checkCommand{app: a})
	_ = r.Register(&collectstaticCommand{app: a})
	_ = r.Register(&findstaticCommand{app: a})
	_ = r.Register(&shellCommand{app: a})
	_ = r.Register(&dbshellCommand{app: a})

	// Apps come in reverse INSTALLED_APPS order, so the command of the app
	// listed first replaces the others, as in Django.
	for _, ac := range a.Apps.Commands() {
		r.RegisterApp(ac.App, ac.Command)
	}
}

// checkTagger is implemented by a command that runs only the system checks
// with one of the given tags before it runs (Django's
// requires_system_checks = [tags]). A command without it runs all checks.
type checkTagger interface{ CheckTags() []string }

// systemChecks is the hook of manage.Registry.SystemChecks: the check
// Django's BaseCommand.execute runs before handle. Errors stop the command
// with a *checks.SystemCheckError; warnings are printed and the command runs.
func (a *Application) systemChecks(c manage.Command) error {
	var tags []string
	if t, ok := c.(checkTagger); ok {
		tags = t.CheckTags()
	}
	return a.runChecks(checkOptions{tags: tags})
}

// checkOptions are the arguments of Django's BaseCommand.check.
type checkOptions struct {
	apps      []string // app labels; nil for every app
	tags      []string // nil for every tag
	databases []string // database aliases the database checks may use
	deploy    bool     // include the deployment checks
	// failLevel is the level that fails the run; zero means ERROR.
	failLevel checks.Level
	// displayNumErrors adds the "System check identified ..." line.
	displayNumErrors bool
}

// runChecks runs the system checks and reports them as Django's
// BaseCommand.check does: a run with a message at or above the fail level
// returns a *checks.SystemCheckError and prints nothing; otherwise the report
// goes to Err when it lists issues and to Out when it does not.
func (a *Application) runChecks(o checkOptions) error {
	ctx := &checks.Context{Settings: a.Settings, Models: a.Models, Apps: o.apps, Databases: o.databases}
	messages := a.Checks.Run(ctx, checks.RunOptions{Tags: o.tags, Deploy: o.deploy})
	failLevel := o.failLevel
	if failLevel == 0 {
		failLevel = checks.LevelError
	}
	report := checks.Summarize(messages, a.Settings.SilencedSystemChecks, failLevel, o.displayNumErrors)
	switch {
	case report.Failed:
		return &checks.SystemCheckError{Report: report}
	case report.Issues > 0:
		_, _ = fmt.Fprint(a.errOut(), report.Text)
	case report.Text != "":
		_, _ = fmt.Fprint(a.out(), report.Text)
	}
	return nil
}

// checkCommand is Django's check command: it runs the system checks and
// reports every message.
type checkCommand struct {
	app       *Application
	tags      stringList
	databases stringList
	listTags  bool
	deploy    bool
	failLevel checks.Level
}

// Name returns the command name.
func (*checkCommand) Name() string { return "check" }

// Help returns the help text.
func (*checkCommand) Help() string {
	return "Checks the entire Djan-Go-Go project for potential problems."
}

// Usage names the positional arguments.
func (*checkCommand) Usage() string { return "[app_label ...]" }

// SkipSystemChecks reports that check runs no checks before it runs, since
// running them is its job (requires_system_checks = []).
func (*checkCommand) SkipSystemChecks() bool { return true }

// Flags declares Django's options of check. It resets the values of an
// earlier run first.
func (c *checkCommand) Flags(fs *flag.FlagSet) {
	*c = checkCommand{app: c.app, failLevel: checks.LevelError}
	const tagHelp = "Run only checks labeled with given tag."
	fs.Var(&c.tags, "tag", tagHelp)
	fs.Var(&c.tags, "t", tagHelp)
	fs.BoolVar(&c.listTags, "list-tags", false,
		"List available tags. Specify --deploy to include available deployment tags.")
	fs.BoolVar(&c.deploy, "deploy", false, "Check deployment settings.")
	fs.Var((*levelValue)(&c.failLevel), "fail-level",
		"Message level that will cause the command to exit with a non-zero status. Default is ERROR.")
	fs.Var(&c.databases, "database", "Run database related checks against these aliases.")
}

// Run runs the checks of the apps with the given labels, or of all apps.
func (c *checkCommand) Run(labels []string) error {
	a := c.app
	if c.listTags {
		_, _ = fmt.Fprintln(a.out(), strings.Join(a.Checks.TagsAvailable(c.deploy), "\n"))
		return nil
	}
	if err := checkDatabaseAliases(a.Settings, c.databases); err != nil {
		return err
	}
	for _, label := range labels {
		if _, err := a.Apps.ByLabel(label); err != nil {
			return manage.Errorf("%w", err)
		}
	}
	for _, tag := range c.tags {
		if !a.Checks.TagExists(tag, c.deploy) {
			return manage.Errorf("There is no system check with the \"%s\" tag.", tag)
		}
	}
	return a.runChecks(checkOptions{
		apps:             labels,
		tags:             c.tags,
		databases:        c.databases,
		deploy:           c.deploy,
		failLevel:        c.failLevel,
		displayNumErrors: true,
	})
}

// databaseAliases returns the database aliases a --database option accepts,
// sorted: those of Settings.Databases and always "default", as Django's
// connections always hold the default alias.
func databaseAliases(s *conf.Settings) []string {
	aliases := []string{conf.DefaultDatabaseAlias}
	for alias := range s.Databases {
		if alias != conf.DefaultDatabaseAlias {
			aliases = append(aliases, alias)
		}
	}
	slices.Sort(aliases)
	return aliases
}

// checkDatabaseAliases returns argparse's usage error for a --database value
// that names no database.
func checkDatabaseAliases(s *conf.Settings, given []string) error {
	aliases := databaseAliases(s)
	for _, alias := range given {
		if !slices.Contains(aliases, alias) {
			return &manage.UsageError{Msg: fmt.Sprintf("argument --database: invalid choice: '%s' (choose from %s)",
				alias, quoteChoices(aliases))}
		}
	}
	return nil
}

// quoteChoices renders choices as argparse lists them: 'a', 'b'.
func quoteChoices(choices []string) string {
	quoted := make([]string, len(choices))
	for i, c := range choices {
		quoted[i] = "'" + c + "'"
	}
	return strings.Join(quoted, ", ")
}

// stringList is a flag that may be given several times, argparse's
// action="append".
type stringList []string

// String returns the values joined by commas.
func (l *stringList) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(*l, ",")
}

// Set appends a value.
func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// levelValue is the flag of a check message level, argparse's choices of
// --fail-level.
type levelValue checks.Level

// String returns the level name.
func (l *levelValue) String() string {
	if l == nil {
		return ""
	}
	return checks.Level(*l).String()
}

// Set parses a level name.
func (l *levelValue) Set(v string) error {
	level, err := checks.ParseLevel(v)
	if err != nil {
		return err
	}
	*l = levelValue(level)
	return nil
}
