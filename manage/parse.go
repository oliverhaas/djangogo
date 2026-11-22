package manage

import (
	"flag"
	"io"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// skipChecksFlag is the name of the flag common to every command that
// requires the system checks.
const skipChecksFlag = "skip-checks"

// commandFlags is the flag set of one command for one run or one help text.
type commandFlags struct {
	fs *flag.FlagSet
	// requiresChecks reports whether the command runs the system checks.
	requiresChecks bool
	// skipChecks is the value of --skip-checks; nil when the command does not
	// require the system checks.
	skipChecks *bool
}

// newCommandFlags builds the flag set of a command: the common flag first,
// then the flags the command declares, so a command finds the common one
// defined, as a Django command finds the common arguments in add_arguments.
func (r *Registry) newCommandFlags(e entry) *commandFlags {
	fs := flag.NewFlagSet(e.name, flag.ContinueOnError)
	// The registry reports the errors and prints the usage itself.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	cf := &commandFlags{fs: fs, requiresChecks: r.requiresChecks(e)}
	if cf.requiresChecks {
		cf.skipChecks = fs.Bool(skipChecksFlag, false, "Skip system checks.")
	}
	if f, ok := e.cmd.(Flagger); ok {
		f.Flags(fs)
	}
	return cf
}

// parse sets the flags found in args and returns the positional arguments.
// Flags and positional arguments may be mixed; "--" ends the flags. help is
// true when -h or --help is found and is not a flag of the command; parsing
// stops there. takesArgs tells whether the command has positional arguments:
// without them a positional argument is as unrecognized as an unknown flag.
//
// The flag package stops at the first positional argument, so the arguments
// are split here with its rules (one or two dashes, "-name=value", a boolean
// flag takes no separate value) and every flag is set through FlagSet.Set. An
// OptionalValue takes the next argument only when that is no flag.
func (cf *commandFlags) parse(args []string, takesArgs bool) (positional []string, help bool, err error) {
	// unrecognized collects the unknown flags and, for a command without
	// positional arguments, those as well, in the order given: argparse
	// reports them together at the end.
	var unrecognized []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if takesArgs {
				positional = append(positional, args[i+1:]...)
			} else {
				// Nothing may follow the terminator here, and argparse
				// reports the terminator with the rest.
				unrecognized = append(unrecognized, args[i:]...)
			}
			break
		}
		name, value, hasValue, isFlag := splitFlag(arg)
		if !isFlag {
			if takesArgs {
				positional = append(positional, arg)
			} else {
				unrecognized = append(unrecognized, arg)
			}
			continue
		}
		f := cf.fs.Lookup(name)
		switch {
		case f == nil && (name == "h" || name == "help"):
			return nil, true, nil
		case f == nil:
			unrecognized = append(unrecognized, arg)
			continue
		case isBoolFlag(f):
			if !hasValue {
				value = "true"
			}
		case !hasValue && isOptional(f):
			value = f.Value.(OptionalValue).NoValue()
			if i+1 < len(args) && args[i+1] != "--" {
				if _, _, _, nextIsFlag := splitFlag(args[i+1]); !nextIsFlag {
					i++
					value = args[i]
				}
			}
		case !hasValue:
			if i+1 == len(args) {
				return nil, false, &UsageError{Msg: "argument " + cf.optionNames(f) + ": expected one argument"}
			}
			i++
			value = args[i]
		}
		if err := cf.fs.Set(name, value); err != nil {
			return nil, false, &UsageError{Msg: "argument " + cf.optionNames(f) + ": " + valueProblem(f, value, err)}
		}
	}
	if len(unrecognized) > 0 {
		return nil, false, &UsageError{Msg: "unrecognized arguments: " + strings.Join(unrecognized, " ")}
	}
	// Mark the set as parsed, so the command may use fs.Args and fs.Parsed on
	// a set it kept. The error is always nil: nothing is left to parse after
	// the terminator.
	_ = cf.fs.Parse(append([]string{"--"}, positional...))
	return positional, false, nil
}

// splitFlag splits a command-line argument that is a flag into the name and
// the value given with "=". An argument that does not start with a dash and
// the single dash are positional arguments (isFlag is false); the caller
// handles the terminator "--" first.
func splitFlag(arg string) (name, value string, hasValue, isFlag bool) {
	if len(arg) < 2 || arg[0] != '-' {
		return "", "", false, false
	}
	name = strings.TrimPrefix(arg[1:], "-")
	name, value, hasValue = strings.Cut(name, "=")
	return name, value, hasValue, true
}

// isBoolFlag reports whether f takes no value, by the convention of the flag
// package.
func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// isOptional reports whether f may be given without a value.
func isOptional(f *flag.Flag) bool {
	_, ok := f.Value.(OptionalValue)
	return ok
}

// valueProblem words the error of setting f to value. For the value types of
// the flag package the text is argparse's ("invalid int value: 'x'"), since
// their own error is only "parse error"; for another type it is the error of
// its Set method.
func valueProblem(f *flag.Flag, value string, err error) string {
	// UnquoteUsage names the type when the usage text names nothing.
	kind, _ := flag.UnquoteUsage(&flag.Flag{Name: f.Name, Value: f.Value})
	switch kind {
	case "value":
		return err.Error()
	case "":
		kind = "boolean"
	}
	return "invalid " + kind + " value: " + pyRepr(value)
}

// option is one entry of the option list of a command: a flag together with
// its aliases, the flags bound to the same variable.
type option struct {
	names []string   // one-letter names first, each group sorted
	flag  *flag.Flag // the flag of the first long name: usage, default, metavariable
}

// name returns the name the option is sorted and documented under.
func (o option) name() string { return o.flag.Name }

// options groups the flags of the set into options, sorted by name.
func (cf *commandFlags) options() []option {
	var groups [][]*flag.Flag
	cf.fs.VisitAll(func(f *flag.Flag) {
		for i, g := range groups {
			if sameVariable(g[0].Value, f.Value) {
				groups[i] = append(g, f)
				return
			}
		}
		groups = append(groups, []*flag.Flag{f})
	})

	opts := make([]option, 0, len(groups))
	for _, g := range groups {
		// VisitAll walks the flags by name; a stable sort keeps that order
		// behind the one-letter names.
		sort.SliceStable(g, func(i, j int) bool { return len(g[i].Name) == 1 && len(g[j].Name) > 1 })
		o := option{flag: g[0]}
		for _, f := range g {
			o.names = append(o.names, f.Name)
			if len(o.flag.Name) == 1 && len(f.Name) > 1 {
				o.flag = f
			}
		}
		opts = append(opts, o)
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].name() < opts[j].name() })
	return opts
}

// optionNames returns the names of the option f belongs to as argparse shows
// them in an error, for example "-s/--style".
func (cf *commandFlags) optionNames(f *flag.Flag) string {
	names := []string{f.Name}
	for _, o := range cf.options() {
		if slices.Contains(o.names, f.Name) {
			names = o.names
		}
	}
	return strings.Join(dashed(names), "/")
}

// dashed puts one dash in front of a one-letter name and two in front of any
// other.
func dashed(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		if len(name) == 1 {
			out[i] = "-" + name
		} else {
			out[i] = "--" + name
		}
	}
	return out
}

// sameVariable reports whether two flag values are bound to the same
// variable, which makes their flags aliases. The Var functions of the flag
// package turn the pointer to the variable into the value, so two values are
// compared as pointers; a value of another kind is never an alias.
func sameVariable(a, b flag.Value) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Kind() != reflect.Pointer || vb.Kind() != reflect.Pointer || va.Type() != vb.Type() {
		return false
	}
	// Pointers to distinct zero-size variables may be equal.
	return va.Type().Elem().Size() > 0 && va.Pointer() == vb.Pointer()
}

// cutSkipChecks removes --skip-checks from the arguments of a command that
// parses its own, up to the terminator "--", and reports whether it was
// there.
func cutSkipChecks(args []string) (rest []string, skip bool) {
	rest = make([]string, 0, len(args))
	for i, arg := range args {
		if arg == "--" {
			return append(rest, args[i:]...), skip
		}
		if arg == "--"+skipChecksFlag || arg == "-"+skipChecksFlag {
			skip = true
			continue
		}
		rest = append(rest, arg)
	}
	return rest, skip
}
