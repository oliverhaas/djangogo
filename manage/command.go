package manage

import (
	"flag"
	"io"
	"os"
)

// Command is a management subcommand. A command gains further behavior by
// implementing the optional interfaces Flagger, Usager, StreamSetter and
// CheckSkipper, which the registry discovers by type assertion.
type Command interface {
	Name() string
	Help() string
	Run(args []string) error
}

// Flagger is implemented by a command that takes flags. The registry builds a
// flag.FlagSet for the command, lets it declare its flags, parses the
// arguments (flags and positional arguments may be mixed, as with argparse;
// "--" ends the flags), and calls Run with the positional arguments.
//
// The common flag --skip-checks is defined before Flags is called when the
// command requires the system checks, so defining it again panics, as
// defining any flag twice does. A command that does not also implement Usager
// takes no positional arguments.
type Flagger interface{ Flags(fs *flag.FlagSet) }

// Usager is implemented by a command with positional arguments; Usage returns
// their synopsis, for example "[addrport]" or "app_label [app_label ...]".
type Usager interface{ Usage() string }

// StreamSetter is implemented by a command that writes through the registry's
// streams; the registry calls SetStreams before every Run.
type StreamSetter interface {
	SetStreams(stdout, stderr io.Writer)
}

// CheckSkipper is implemented by a command that never runs the system checks
// (Django's requires_system_checks = []).
type CheckSkipper interface{ SkipSystemChecks() bool }

// Base is embedded in a command to get Stdout, Stderr and SetStreams
// (Django's self.stdout and self.stderr). Its zero value writes to os.Stdout
// and os.Stderr.
type Base struct {
	stdout, stderr io.Writer
}

// SetStreams sets the writers Stdout and Stderr return. A nil writer selects
// the stream of the process.
func (b *Base) SetStreams(stdout, stderr io.Writer) {
	b.stdout, b.stderr = stdout, stderr
}

// Stdout returns the writer for the output of the command.
func (b *Base) Stdout() io.Writer {
	if b.stdout == nil {
		return os.Stdout
	}
	return b.stdout
}

// Stderr returns the writer for the diagnostics of the command.
func (b *Base) Stderr() io.Writer {
	if b.stderr == nil {
		return os.Stderr
	}
	return b.stderr
}
