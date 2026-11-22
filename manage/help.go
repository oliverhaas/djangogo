package manage

import (
	"flag"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

// The layout of a command help follows argparse on a terminal of 80 columns:
// the texts are wrapped two columns before the edge and the help of an option
// starts at column 24.
const (
	helpWidth    = 78
	helpPosition = 24
	helpIndent   = 2
)

// MainHelp is Django's main help text (ManagementUtility.main_help_text): the
// commands grouped by app, the groups and the commands of a group sorted by
// name. With commandsOnly it is the bare sorted list of command names, one
// per line. The text has no trailing newline.
func (r *Registry) MainHelp(commandsOnly bool) string {
	if commandsOnly {
		return strings.Join(r.Names(), "\n")
	}
	lines := []string{
		"",
		fmt.Sprintf("Type '%s help <subcommand>' for help on a specific subcommand.", r.prog()),
		"",
		"Available subcommands:",
	}
	groups := make(map[string][]string)
	for name, e := range r.commands {
		groups[e.app] = append(groups[e.app], name)
	}
	apps := make([]string, 0, len(groups))
	for app := range groups {
		apps = append(apps, app)
	}
	sort.Strings(apps)
	for _, app := range apps {
		lines = append(lines, "", "["+app+"]")
		names := groups[app]
		sort.Strings(names)
		for _, name := range names {
			lines = append(lines, "    "+name)
		}
	}
	return strings.Join(lines, "\n")
}

// CommandHelp is the help of one command (BaseCommand.print_help): the usage
// line, the help text of the command and the list of its options. The text
// ends with a newline. The error for a name that is not registered matches
// ErrUnknownCommand.
func (r *Registry) CommandHelp(name string) (string, error) {
	e, ok := r.commands[name]
	if !ok {
		return "", r.unknownCommand(name)
	}
	return r.commandHelp(e, r.newCommandFlags(e)), nil
}

// commandHelp renders the help of a command from its flag set.
func (r *Registry) commandHelp(e entry, cf *commandFlags) string {
	opts := r.listedOptions(cf)

	var b strings.Builder
	b.WriteString(r.usageLine(e, cf))
	b.WriteString("\n\n")
	if description := wrap(e.cmd.Help(), helpWidth); len(description) > 0 {
		b.WriteString(strings.Join(description, "\n"))
		b.WriteString("\n\n")
	}
	b.WriteString("options:\n")
	if names := helpNames(cf.fs); len(names) > 0 {
		writeOption(&b, strings.Join(dashed(names), ", "), "show this help message and exit")
	}
	for _, o := range opts {
		writeOption(&b, o.invocation(), o.help())
	}
	return b.String()
}

// usageLine returns the one-line usage of a command: "usage: prog name",
// "[options]" when the help lists options beside -h, and the synopsis of the
// positional arguments.
func (r *Registry) usageLine(e entry, cf *commandFlags) string {
	usage := "usage: " + r.prog() + " " + e.name
	if len(r.listedOptions(cf)) > 0 {
		usage += " [options]"
	}
	if u, ok := e.cmd.(Usager); ok && u.Usage() != "" {
		usage += " " + u.Usage()
	}
	return usage
}

// listedOptions returns the options the help shows: those of the command by
// name, then --skip-checks, as Django lists the common options last.
// --skip-checks is left out while no system checks are configured, since it
// would do nothing.
func (r *Registry) listedOptions(cf *commandFlags) []option {
	all := cf.options()
	opts := make([]option, 0, len(all))
	var common []option
	for _, o := range all {
		if cf.skipChecks != nil && o.name() == skipChecksFlag {
			if r.SystemChecks != nil {
				common = append(common, o)
			}
			continue
		}
		opts = append(opts, o)
	}
	return append(opts, common...)
}

// helpNames returns the names that ask for the help of a command: h and
// help, without the ones the command uses for a flag of its own.
func helpNames(fs *flag.FlagSet) []string {
	var names []string
	for _, name := range []string{"h", "help"} {
		if fs.Lookup(name) == nil {
			names = append(names, name)
		}
	}
	return names
}

// invocation returns the option as the option list shows it: the names, and
// for an option that takes a value its metavariable ("-t, --tag TAG"), in
// brackets when the value is optional ("-x, --exclude [EXCLUDE]").
func (o option) invocation() string {
	s := strings.Join(dashed(o.names), ", ")
	metavar, _ := o.metavar()
	switch {
	case metavar == "":
	case isOptional(o.flag):
		s += " [" + metavar + "]"
	default:
		s += " " + metavar
	}
	return s
}

// metavar returns the name of the value in the help and the usage text
// without its back quotes. The name is the back-quoted word of the usage
// text (the flag.UnquoteUsage convention) or the name of the flag in upper
// case; a boolean flag has none.
func (o option) metavar() (name, usage string) {
	usage = o.flag.Usage
	if before, rest, found := strings.Cut(usage, "`"); found {
		if quoted, after, closed := strings.Cut(rest, "`"); closed {
			name, usage = quoted, before+quoted+after
		}
	}
	switch {
	case isBoolFlag(o.flag):
		name = ""
	case name == "":
		name = strings.ToUpper(strings.ReplaceAll(o.name(), "-", "_"))
	}
	return name, usage
}

// help returns the usage text of the option followed by its default when
// that is not the zero value. An empty default has no text to show.
func (o option) help() string {
	_, usage := o.metavar()
	if o.flag.DefValue == "" || isZeroDefault(o.flag) {
		return usage
	}
	return strings.TrimSpace(usage + " (default: " + o.flag.DefValue + ")")
}

// isZeroDefault reports whether the default of f is the zero value of its
// type, with the test the flag package uses for PrintDefaults: the default
// equals the text of a zero value of the type.
func isZeroDefault(f *flag.Flag) (zero bool) {
	typ := reflect.TypeOf(f.Value)
	var z reflect.Value
	if typ.Kind() == reflect.Pointer {
		z = reflect.New(typ.Elem())
	} else {
		z = reflect.Zero(typ)
	}
	// A String method that fails on a zero value must not break the help;
	// the default is shown then.
	defer func() {
		if recover() != nil {
			zero = false
		}
	}()
	v, ok := z.Interface().(flag.Value)
	return ok && f.DefValue == v.String()
}

// writeOption writes one entry of the option list in argparse's layout: the
// help starts at helpPosition, on the line of the invocation when that is
// short enough and on the next line otherwise.
func writeOption(b *strings.Builder, invocation, help string) {
	lines := wrap(help, helpWidth-helpPosition)
	if len(lines) == 0 {
		fmt.Fprintf(b, "%*s%s\n", helpIndent, "", invocation)
		return
	}
	// Two spaces separate the invocation from the help.
	room := helpPosition - helpIndent - 2
	if utf8.RuneCountInString(invocation) <= room {
		fmt.Fprintf(b, "%*s%-*s  %s\n", helpIndent, "", room, invocation, lines[0])
		lines = lines[1:]
	} else {
		fmt.Fprintf(b, "%*s%s\n", helpIndent, "", invocation)
	}
	for _, line := range lines {
		fmt.Fprintf(b, "%*s%s\n", helpPosition, "", line)
	}
}

// wrap breaks text into lines of at most width characters at white space,
// after collapsing every run of white space, as argparse does with a help
// text. A word longer than width stays whole.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
