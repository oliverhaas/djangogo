package manage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// builtinApp is the help group of the commands added with Register, where
// Django says "django".
const builtinApp = "djangogo"

// entry is a registered command with the help group it is listed under.
type entry struct {
	name string
	cmd  Command
	app  string
}

// Registry holds management commands and dispatches them by name, the
// counterpart of Django's ManagementUtility. A registry is not safe for
// concurrent use; separate registries are independent.
type Registry struct {
	Out io.Writer // stdout of the commands and of help; default os.Stdout
	Err io.Writer // stderr; default os.Stderr
	// Prog is the program name shown in help and usage texts (Django's
	// "manage.py"); default: the base name of os.Args[0].
	Prog string
	// Version is what --version prints.
	Version string
	// SystemChecks, when set, runs before a command that requires the system
	// checks: every command except those that implement CheckSkipper (returning
	// true), those named with ExemptFromChecks, and a run with --skip-checks.
	// A non-nil error stops the command.
	SystemChecks func(c Command) error

	commands map[string]entry
	exempt   map[string]bool
}

// NewRegistry returns an empty registry that writes to the streams of the
// process.
func NewRegistry() *Registry {
	return &Registry{Out: os.Stdout, Err: os.Stderr, commands: make(map[string]entry)}
}

// Register adds a built-in command (help group "djangogo"). It errors on a
// duplicate name.
func (r *Registry) Register(c Command) error {
	name := c.Name()
	if _, dup := r.commands[name]; dup {
		return fmt.Errorf("manage: duplicate command %q", name)
	}
	r.add(entry{name: name, cmd: c, app: builtinApp})
	return nil
}

// RegisterApp adds the command of an app (help group: the app label). It
// replaces a command of the same name, whoever registered it: an app may
// override a built-in command, and the caller registers the apps in reverse
// INSTALLED_APPS order so that the app listed first wins, as in Django.
func (r *Registry) RegisterApp(app string, c Command) {
	r.add(entry{name: c.Name(), cmd: c, app: app})
}

// add stores a command under its name.
func (r *Registry) add(e entry) {
	if r.commands == nil {
		r.commands = make(map[string]entry)
	}
	r.commands[e.name] = e
}

// ExemptFromChecks names commands that never run the system checks.
func (r *Registry) ExemptFromChecks(names ...string) {
	if r.exempt == nil {
		r.exempt = make(map[string]bool)
	}
	for _, name := range names {
		r.exempt[name] = true
	}
}

// Get returns the command registered under name.
func (r *Registry) Get(name string) (Command, bool) {
	e, ok := r.commands[name]
	return e.cmd, ok
}

// App returns the help group of a command ("djangogo" for a built-in one) and
// "" for a name that is not registered.
func (r *Registry) App(name string) string {
	return r.commands[name].app
}

// Names returns the registered command names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.commands))
	for n := range r.commands {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Execute dispatches the arguments and returns the command's error. args[0]
// names the command and the rest are its arguments.
//
// As Django's ManagementUtility does, it prints the main help for no
// arguments, for "help", "--help" and "-h", the help of a command for
// "help <command>" and for a command given --help or -h, the list of
// commands for "help --commands", and Version for "--version" and for
// "version" unless a command of that name is registered. All of these go to
// Out and return nil.
//
// The error for an unknown command matches ErrUnknownCommand, a mistake in
// the arguments is a *UsageError, and a failure of the system checks is the
// error of SystemChecks.
func (r *Registry) Execute(args []string) error {
	if len(args) == 0 {
		r.print(r.MainHelp(false) + "\n")
		return nil
	}
	name, rest := args[0], args[1:]
	e, registered := r.commands[name]
	switch {
	case name == "help":
		return r.help(rest)
	case name == "--help" || name == "-h":
		r.print(r.MainHelp(false) + "\n")
		return nil
	case name == "--version" || (name == "version" && !registered):
		r.print(r.Version + "\n")
		return nil
	case !registered:
		return r.unknownCommand(name)
	}
	return r.runCommand(e, rest)
}

// Run is Execute for a main function: it prints an error to Err the way
// Django does and returns the process exit code (see ExitCode).
//
// A *CommandError is printed as "CommandError: <msg>", a *UsageError as the
// usage line of the command followed by the message, and an unknown command
// as Django's message followed by "Type 'prog help' for usage.". An error
// that implements interface{ Quiet() bool } and returns true is not printed:
// its source has reported it already. Any other error is printed as
// "error: <msg>".
func (r *Registry) Run(args []string) int {
	err := r.Execute(args)
	if err != nil {
		r.report(err)
	}
	return ExitCode(err)
}

// report prints an error of Execute to Err.
func (r *Registry) report(err error) {
	var quiet interface{ Quiet() bool }
	if errors.As(err, &quiet) && quiet.Quiet() {
		return
	}
	w := r.stderr()
	var (
		unknown  *unknownCommandError
		usageErr *UsageError
		cmdErr   *CommandError
	)
	switch {
	case errors.As(err, &unknown):
		_, _ = fmt.Fprintf(w, "%s\nType '%s help' for usage.\n", unknown.Error(), r.prog())
	case errors.As(err, &usageErr):
		if usageErr.usage != "" {
			_, _ = fmt.Fprintln(w, usageErr.usage)
		}
		_, _ = fmt.Fprintln(w, usageErr.Msg)
	case errors.As(err, &cmdErr):
		_, _ = fmt.Fprintf(w, "CommandError: %s\n", err.Error())
	default:
		_, _ = fmt.Fprintf(w, "error: %s\n", err.Error())
	}
}

// help runs the help subcommand: the list of commands for --commands, the
// help of the first command named, the main help otherwise.
func (r *Registry) help(args []string) error {
	if slices.Contains(args, "--commands") {
		r.print(r.MainHelp(true) + "\n")
		return nil
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		text, err := r.CommandHelp(arg)
		if err != nil {
			return err
		}
		r.print(text)
		return nil
	}
	r.print(r.MainHelp(false) + "\n")
	return nil
}

// runCommand parses the arguments of a command, runs the system checks it
// requires and runs it.
func (r *Registry) runCommand(e entry, args []string) error {
	cf := r.newCommandFlags(e)
	skipChecks := false
	if _, ok := e.cmd.(Flagger); ok {
		_, takesArgs := e.cmd.(Usager)
		positional, help, err := cf.parse(args, takesArgs)
		switch {
		case err != nil:
			return r.completeUsageError(e, cf, err)
		case help:
			r.print(r.commandHelp(e, cf))
			return nil
		}
		args = positional
		skipChecks = cf.skipChecks != nil && *cf.skipChecks
	} else {
		// A command without Flags parses its own arguments; only the help
		// request and the common flag are taken out.
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			r.print(r.commandHelp(e, cf))
			return nil
		}
		if cf.requiresChecks {
			args, skipChecks = cutSkipChecks(args)
		}
	}

	if s, ok := e.cmd.(StreamSetter); ok {
		s.SetStreams(r.stdout(), r.stderr())
	}
	if cf.requiresChecks && !skipChecks && r.SystemChecks != nil {
		if err := r.SystemChecks(e.cmd); err != nil {
			return err
		}
	}
	return r.completeUsageError(e, cf, e.cmd.Run(args))
}

// requiresChecks reports whether a command runs the system checks.
func (r *Registry) requiresChecks(e entry) bool {
	if s, ok := e.cmd.(CheckSkipper); ok && s.SkipSystemChecks() {
		return false
	}
	return !r.exempt[e.name]
}

// completeUsageError turns a usage error of a command into the form argparse
// prints: the message gets the prefix "prog name: error: " unless it has it,
// and the usage line of the command is attached for Run. Another error, and a
// usage error that was completed already (by a command that runs another
// one), is returned as it is. The result is a new error, so a command may
// return the same *UsageError from several runs.
func (r *Registry) completeUsageError(e entry, cf *commandFlags, err error) error {
	var usageErr *UsageError
	if !errors.As(err, &usageErr) || usageErr.usage != "" {
		return err
	}
	msg := usageErr.Msg
	if prefix := r.prog() + " " + e.name + ": error: "; !strings.HasPrefix(msg, prefix) {
		msg = prefix + msg
	}
	return &UsageError{Msg: msg, usage: r.usageLine(e, cf)}
}

// unknownCommand builds the error for a name that is not registered, with
// the suggestion Django gives for a name close to a registered one.
func (r *Registry) unknownCommand(name string) error {
	suggestion, _ := closeMatch(name, r.Names())
	return &unknownCommandError{name: name, suggestion: suggestion}
}

// prog returns the program name for the help and usage texts.
func (r *Registry) prog() string {
	if r.Prog != "" {
		return r.Prog
	}
	if len(os.Args) == 0 {
		return ""
	}
	return filepath.Base(os.Args[0])
}

// stdout returns the writer for the output of the commands and of help.
func (r *Registry) stdout() io.Writer {
	if r.Out == nil {
		return os.Stdout
	}
	return r.Out
}

// stderr returns the writer for errors.
func (r *Registry) stderr() io.Writer {
	if r.Err == nil {
		return os.Stderr
	}
	return r.Err
}

// print writes a help or version text to Out. A failed write is not
// reported: there is no better place to report it to.
func (r *Registry) print(text string) {
	_, _ = io.WriteString(r.stdout(), text)
}
