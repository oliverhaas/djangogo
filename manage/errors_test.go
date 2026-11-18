package manage_test

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/manage"
)

// checkError stands in for the error of failed system checks: it carries its
// exit code and tells whether it was reported already.
type checkError struct {
	code  int
	quiet bool
}

func (*checkError) Error() string   { return "System check identified some issues" }
func (e *checkError) ExitCode() int { return e.code }
func (e *checkError) Quiet() bool   { return e.quiet }

// formattedError stands in for checks.SystemCheckError, which prints itself.
type formattedError struct{}

func (formattedError) Error() string { return "System check identified some issues:" }
func (formattedError) Formatted() string {
	return "SystemCheckError: System check identified some issues:\n"
}

// TestExitCode covers the mapping from an error to an exit code, for the
// error itself and for an error that wraps it.
func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"plain error", errors.New("boom"), 1},
		{"command error without a code", &manage.CommandError{Msg: "boom"}, 1},
		{"command error with a code", &manage.CommandError{Msg: "boom", Code: 3}, 3},
		{"Errorf", manage.Errorf("boom %d", 1), 1},
		{"usage error", &manage.UsageError{Msg: "boom"}, 2},
		{"error with an ExitCode method", &checkError{code: 7}, 7},
		{"wrapped command error", fmt.Errorf("run: %w", &manage.CommandError{Code: 4}), 4},
		{"wrapped usage error", fmt.Errorf("run: %w", &manage.UsageError{}), 2},
		{"wrapped error with an ExitCode method", fmt.Errorf("run: %w", &checkError{code: 5}), 5},
		{"command error that wraps a usage error", manage.Errorf("run: %w", &manage.UsageError{}), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertEqual(t, manage.ExitCode(tt.err), tt.want)
		})
	}
}

// TestErrorf: the message is formatted as fmt.Errorf formats it, and an error
// wrapped with %w stays matchable.
func TestErrorf(t *testing.T) {
	err := manage.Errorf("cannot read %q: %w", "settings.env", fs.ErrNotExist)

	assertEqual(t, err.Msg, `cannot read "settings.env": file does not exist`)
	assertEqual(t, err.Error(), err.Msg)
	assertEqual(t, err.Code, 0)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the error does not match the one it wraps")
	}
	if errors.Unwrap(&manage.CommandError{Msg: "literal"}) != nil {
		t.Errorf("a literal CommandError wraps an error")
	}
}

// TestRunReportsErrors pins what Run prints for every kind of error and the
// exit code it returns.
func TestRunReportsErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantErr  string
		wantCode int
	}{
		{"no error", nil, "", 0},
		{"plain error", errors.New("database is locked"), "error: database is locked\n", 1},
		{"command error", manage.Errorf("Unknown app: %s", "polls"), "CommandError: Unknown app: polls\n", 1},
		{"command error with a code", &manage.CommandError{Msg: "No fixture", Code: 5}, "CommandError: No fixture\n", 5},
		{"command error without a message", &manage.CommandError{Code: 3}, "CommandError: \n", 3},
		{
			"wrapped command error",
			fmt.Errorf("loaddata: %w", manage.Errorf("No fixture")),
			"CommandError: loaddata: No fixture\n", 1,
		},
		{
			"usage error",
			&manage.UsageError{Msg: "Enter at least one label."},
			"usage: manage.py failing\nmanage.py failing: error: Enter at least one label.\n", 2,
		},
		{
			"usage error with the prefix",
			&manage.UsageError{Msg: "manage.py failing: error: Enter at least one label."},
			"usage: manage.py failing\nmanage.py failing: error: Enter at least one label.\n", 2,
		},
		{"quiet error", &checkError{code: 1, quiet: true}, "", 1},
		{"wrapped quiet error", fmt.Errorf("check: %w", &checkError{code: 6, quiet: true}), "", 6},
		{
			"error that is not quiet",
			&checkError{code: 1},
			"error: System check identified some issues\n", 1,
		},
		{
			"error that formats itself",
			fmt.Errorf("check: %w", formattedError{}),
			"SystemCheckError: System check identified some issues:\n", 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistry()
			mustRegister(t, r, failingCommand{err: tt.err})

			// The second run shows that reporting an error leaves it as it was.
			for range 2 {
				got := run(r, "failing")

				assertNoOutput(t, got.out)
				assertEqual(t, got.err, tt.wantErr)
				assertEqual(t, got.code, tt.wantCode)
			}
		})
	}
}

// TestExecuteReturnsErrors: Execute prints no error; it returns the error
// for the caller to branch on.
func TestExecuteReturnsErrors(t *testing.T) {
	r := newRegistry()
	cmdErr := manage.Errorf("boom")
	mustRegister(t, r, &checkCommand{}, failingCommand{err: cmdErr})
	var out, errOut strings.Builder
	r.Out, r.Err = &out, &errOut

	err := r.Execute([]string{"chek"})
	if !errors.Is(err, manage.ErrUnknownCommand) {
		t.Errorf("Execute error = %v, want ErrUnknownCommand", err)
	}
	assertEqual(t, err.Error(), "Unknown command: 'chek'. Did you mean check?")
	assertEqual(t, manage.ExitCode(err), 1)

	var usageErr *manage.UsageError
	err = r.Execute([]string{"check", "--invalid"})
	if !errors.As(err, &usageErr) {
		t.Fatalf("Execute error = %v, want a *UsageError", err)
	}
	assertEqual(t, usageErr.Msg, "manage.py check: error: unrecognized arguments: --invalid")
	assertEqual(t, usageErr.Error(), usageErr.Msg)

	if err := r.Execute([]string{"failing"}); !errors.Is(err, cmdErr) {
		t.Errorf("Execute error = %v, want the error of the command", err)
	}

	assertNoOutput(t, out.String())
	assertNoOutput(t, errOut.String())
}

// TestUsageErrorOfCommandIsCompletedInCopy: a command may return the same
// usage error from every run, so Execute leaves it as it is and returns a
// completed copy.
func TestUsageErrorOfCommandIsCompletedInCopy(t *testing.T) {
	r := newRegistry()
	usageErr := &manage.UsageError{Msg: "Enter at least one label."}
	mustRegister(t, r, failingCommand{err: usageErr})

	err := r.Execute([]string{"failing"})

	var got *manage.UsageError
	if !errors.As(err, &got) {
		t.Fatalf("Execute error = %v, want a *UsageError", err)
	}
	assertEqual(t, got.Msg, "manage.py failing: error: Enter at least one label.")
	assertEqual(t, usageErr.Msg, "Enter at least one label.")
}

// TestSystemChecksErrorStopsCommand: the error of the system checks is the
// result of the run, and the command does not run.
func TestSystemChecksErrorStopsCommand(t *testing.T) {
	r := newRegistry()
	checks := &checkCounter{err: &checkError{code: 1, quiet: true}}
	r.SystemChecks = checks.hook
	cmd := &rawCommand{name: "migrate"}
	mustRegister(t, r, cmd)

	got := run(r, "migrate")

	assertEqual(t, got, result{code: 1})
	assertEqual(t, cmd.runs, 0)
	if err := r.Execute([]string{"migrate"}); !errors.Is(err, checks.err) {
		t.Errorf("Execute error = %v, want the error of the checks", err)
	}

	got = run(r, "migrate", "--skip-checks")

	assertEqual(t, got, result{})
	assertEqual(t, cmd.runs, 1)
}
