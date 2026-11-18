package djangogo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/manage"
	"github.com/oliverhaas/djangogo/orm"
)

// changepasswordMaxTries is how often changepassword asks for a new password
// before it gives up (Django's MAX_TRIES).
const changepasswordMaxTries = 3

// changepasswordCommand sets the password of a user from the terminal
// (Django's changepassword). The username is the optional argument and
// defaults to the operating system user.
type changepasswordCommand struct {
	app *Application
	// stderr receives the messages of the password validators; nil means
	// os.Stderr.
	stderr io.Writer
	// getpass replaces the password reading in tests; nil means the
	// application's input, without echo on a terminal.
	getpass func(prompt string) (string, error)
	// systemUsername replaces auth.SystemUsername in tests.
	systemUsername func() string
}

// Name returns "changepassword".
func (*changepasswordCommand) Name() string { return "changepassword" }

// Help returns Django's help text of the command.
func (*changepasswordCommand) Help() string {
	return "Change a user's password for django.contrib.auth."
}

// Run asks for the new password of the user named in args.
func (c *changepasswordCommand) Run(args []string) error {
	if c.app.DB == nil {
		return errNoDatabase
	}
	fs := flag.NewFlagSet("changepassword", flag.ContinueOnError)
	fs.SetOutput(c.app.Out)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), "usage: changepassword [username]")
		_, _ = fmt.Fprintln(fs.Output(), "  username  Username to change password for; by default, it's the current username.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("changepassword: unrecognized arguments: %s", strings.Join(fs.Args()[1:], " "))
	}
	username := fs.Arg(0)
	if username == "" {
		system := auth.SystemUsername
		if c.systemUsername != nil {
			system = c.systemUsername
		}
		username = system()
	}

	ctx := context.Background()
	u, err := orm.Query[auth.User](c.app.DB).Get(ctx, "username", username)
	if errors.Is(err, orm.ErrDoesNotExist) {
		return &manage.CommandError{Msg: fmt.Sprintf("user '%s' does not exist", username)}
	}
	if err != nil {
		return fmt.Errorf("djangogo: load user: %w", err)
	}
	_, _ = fmt.Fprintf(c.app.Out, "Changing password for user '%s'\n", u.Username)

	password, err := c.promptPassword(&u)
	if err != nil {
		return err
	}
	if err := auth.ChangePassword(ctx, c.app.DB, &u, password); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.app.Out, "Password changed successfully for user '%s'\n", u.Username)
	return nil
}

// promptPassword asks up to changepasswordMaxTries times for a password that
// is typed the same twice and passes the password validators. An empty
// answer aborts.
func (c *changepasswordCommand) promptPassword(u *auth.User) (string, error) {
	getpass := commandPrompts(c.app, nil, c.getpass).getpass
	read := func(prompt string) (string, error) {
		p, err := getpass(prompt)
		if err != nil {
			return "", err
		}
		if p == "" {
			return "", &manage.CommandError{Msg: "aborted"}
		}
		return p, nil
	}
	for count := 0; count < changepasswordMaxTries; {
		p1, err := read("Password: ")
		if err != nil {
			return "", err
		}
		p2, err := read("Password (again): ")
		if err != nil {
			return "", err
		}
		if p1 != p2 {
			_, _ = fmt.Fprintln(c.app.Out, "Passwords do not match. Please try again.")
			count++
			continue
		}
		var perrs auth.PasswordErrors
		if err := auth.ValidatePassword(p2, u, nil); errors.As(err, &perrs) {
			_, _ = fmt.Fprintln(stderrOr(c.stderr), strings.Join(perrs.Messages(), "\n"))
			count++
			continue
		} else if err != nil {
			return "", err
		}
		return p1, nil
	}
	return "", &manage.CommandError{Msg: fmt.Sprintf(
		"Aborting password change for user '%s' after %d attempts", u.Username, changepasswordMaxTries)}
}
