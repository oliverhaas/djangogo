package manage

import (
	"errors"
	"fmt"
)

// ErrUnknownCommand is matched (errors.Is) by the error Execute and
// CommandHelp return for a name that is not registered. The text of that
// error is Django's: "Unknown command: 'x'. Did you mean y?".
var ErrUnknownCommand = errors.New("unknown command")

// CommandError is an error a command returns to stop with a message and an
// exit code and without a stack trace (django.core.management.CommandError).
type CommandError struct {
	Msg  string
	Code int // exit code; 0 means 1

	// err keeps what Errorf wrapped with %w reachable for errors.Is and
	// errors.As.
	err error
}

// Error returns the message.
func (e *CommandError) Error() string { return e.Msg }

// Unwrap returns the error Errorf formatted, so the errors it wraps with %w
// stay matchable; it is nil for a CommandError built as a literal.
func (e *CommandError) Unwrap() error { return e.err }

// Errorf builds a *CommandError with exit code 1. The format follows
// fmt.Errorf, including %w.
func Errorf(format string, args ...any) *CommandError {
	err := fmt.Errorf(format, args...)
	return &CommandError{Msg: err.Error(), err: err}
}

// UsageError is a command-line usage error; its exit code is 2 (argparse's).
//
// A command returns &UsageError{Msg: "Enter at least one label."}, the
// counterpart of parser.error(...). Registry.Execute completes it: the
// message becomes "prog name: error: Enter at least one label.", and
// Registry.Run prints the usage of the command in front of it.
type UsageError struct {
	Msg string

	// usage is the usage line Run prints before the message; Execute sets it.
	usage string
}

// Error returns the message.
func (e *UsageError) Error() string { return e.Msg }

// ExitCode maps an error to a process exit code: nil 0, *UsageError 2,
// *CommandError its code, an error with an ExitCode() int method that value,
// anything else 1. Wrapped errors are found with errors.As.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return 2
	}
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		if cmdErr.Code == 0 {
			return 1
		}
		return cmdErr.Code
	}
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

// unknownCommandError reports a name that is not registered, with the closest
// registered name when there is one.
type unknownCommandError struct {
	name       string
	suggestion string
}

// Error returns Django's message, the name quoted as Python's %r does.
func (e *unknownCommandError) Error() string {
	msg := "Unknown command: " + pyRepr(e.name)
	if e.suggestion != "" {
		msg += ". Did you mean " + e.suggestion + "?"
	}
	return msg
}

// Is makes the error match ErrUnknownCommand.
func (e *unknownCommandError) Is(target error) bool { return target == ErrUnknownCommand }
