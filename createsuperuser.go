package djangogo

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oliverhaas/djangogo/auth"
)

// createsuperuserCommand creates a superuser account, mirroring Django's
// createsuperuser. By default it prompts interactively for the username, email,
// and password (twice); with --noinput it reads the username and email from
// flags (or DJANGO_SUPERUSER_USERNAME/DJANGO_SUPERUSER_EMAIL) and the password
// from DJANGO_SUPERUSER_PASSWORD.
type createsuperuserCommand struct {
	app *Application
}

func (*createsuperuserCommand) Name() string { return "createsuperuser" }
func (*createsuperuserCommand) Help() string { return "Create a superuser account" }

func (c *createsuperuserCommand) Run(args []string) error {
	app := c.app
	if app.DB == nil {
		return errors.New("djangogo: no database configured (set Settings.Database.DSN)")
	}

	fs := flag.NewFlagSet("createsuperuser", flag.ContinueOnError)
	fs.SetOutput(app.Out)
	username := fs.String("username", "", "username for the new superuser")
	email := fs.String("email", "", "email for the new superuser")
	noInput := fs.Bool("noinput", false, "do not prompt; take the password from DJANGO_SUPERUSER_PASSWORD")
	if err := fs.Parse(args); err != nil {
		return err
	}

	uname, mail, password, err := c.collect(*username, *email, *noInput)
	if err != nil {
		return err
	}

	if _, err := auth.CreateSuperuser(context.Background(), app.DB, uname, mail, password); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(app.Out, "Superuser created successfully.")
	return nil
}

// collect resolves the username, email, and password either from the
// environment (under --noinput) or by prompting interactively.
func (c *createsuperuserCommand) collect(username, email string, noInput bool) (string, string, string, error) {
	if noInput {
		return collectNoInput(username, email)
	}
	return c.collectInteractive(username, email)
}

// collectNoInput fills any unset username/email from DJANGO_SUPERUSER_* env vars
// and reads the password from DJANGO_SUPERUSER_PASSWORD, requiring a username
// and a password.
func collectNoInput(username, email string) (string, string, string, error) {
	if username == "" {
		username = os.Getenv("DJANGO_SUPERUSER_USERNAME")
	}
	if email == "" {
		email = os.Getenv("DJANGO_SUPERUSER_EMAIL")
	}
	password := os.Getenv("DJANGO_SUPERUSER_PASSWORD")
	if username == "" {
		return "", "", "", errors.New("djangogo: --noinput requires --username or DJANGO_SUPERUSER_USERNAME")
	}
	if password == "" {
		return "", "", "", errors.New("djangogo: --noinput requires DJANGO_SUPERUSER_PASSWORD")
	}
	return username, email, password, nil
}

// collectInteractive prompts for any unset username/email and for the password
// (entered twice). Input is read from the Application's configured reader.
func (c *createsuperuserCommand) collectInteractive(username, email string) (string, string, string, error) {
	r := bufio.NewReader(c.app.in())
	out := c.app.Out
	var err error
	if username == "" {
		if username, err = prompt(r, out, "Username: "); err != nil {
			return "", "", "", err
		}
	}
	if email == "" {
		if email, err = prompt(r, out, "Email address: "); err != nil {
			return "", "", "", err
		}
	}
	password, err := promptPasswordTwice(r, out)
	if err != nil {
		return "", "", "", err
	}
	return username, email, password, nil
}

// prompt writes label to out and returns the next trimmed line from r.
func prompt(r *bufio.Reader, out io.Writer, label string) (string, error) {
	_, _ = fmt.Fprint(out, label)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptPasswordTwice reads a password and a confirmation, requiring a non-empty
// value that matches. Input is line-based and not masked.
func promptPasswordTwice(r *bufio.Reader, out io.Writer) (string, error) {
	first, err := prompt(r, out, "Password: ")
	if err != nil {
		return "", err
	}
	second, err := prompt(r, out, "Password (again): ")
	if err != nil {
		return "", err
	}
	if first == "" {
		return "", errors.New("djangogo: password must not be empty")
	}
	if first != second {
		return "", errors.New("djangogo: passwords do not match")
	}
	return first, nil
}
