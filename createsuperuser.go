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

	"golang.org/x/term"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
)

// commandError is an error whose message a command reports to the user as it
// is (Django's CommandError).
type commandError struct {
	msg string
}

// Error returns the message.
func (e *commandError) Error() string { return e.msg }

// errNoDatabase is returned by the auth and session commands without a
// configured database.
var errNoDatabase = errors.New("djangogo: no database configured (set Settings.Database.DSN)")

// commandInput reads the answers of an interactive command: lines from the
// application's input, and passwords without echo when the input is a
// terminal.
type commandInput struct {
	out    io.Writer
	in     io.Reader
	reader *bufio.Reader
}

// newCommandInput returns the input of app.
func newCommandInput(app *Application) *commandInput {
	in := app.in()
	return &commandInput{out: app.Out, in: in, reader: bufio.NewReader(in)}
}

// input writes prompt and returns the next line without its line break
// (Python's input). The end of the input before any character is an error
// that wraps io.EOF.
func (c *commandInput) input(prompt string) (string, error) {
	_, _ = fmt.Fprint(c.out, prompt)
	line, err := c.reader.ReadString('\n')
	if err != nil && (line == "" || !errors.Is(err, io.EOF)) {
		return "", fmt.Errorf("djangogo: read input: %w", err)
	}
	line = strings.TrimSuffix(line, "\n")
	return strings.TrimSuffix(line, "\r"), nil
}

// getpass writes prompt and reads a password (Python's getpass.getpass). On a
// terminal the typed characters are not shown; any other input is read line
// by line.
func (c *commandInput) getpass(prompt string) (string, error) {
	f, ok := c.in.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return c.input(prompt)
	}
	_, _ = fmt.Fprint(c.out, prompt)
	raw, err := term.ReadPassword(int(f.Fd()))
	_, _ = fmt.Fprintln(c.out)
	if err != nil {
		return "", fmt.Errorf("djangogo: read password: %w", err)
	}
	return string(raw), nil
}

// optionalString is a string flag that remembers whether it was given, as
// Django's options are None when absent.
type optionalString struct {
	value string
	set   bool
}

// String returns the value.
func (o *optionalString) String() string { return o.value }

// Set stores the value given on the command line.
func (o *optionalString) Set(v string) error {
	o.value, o.set = v, true
	return nil
}

// createsuperuserCommand creates a superuser (Django's createsuperuser). It
// prompts for the username, the email address and the password, or takes
// them from flags and the DJANGO_SUPERUSER_* environment variables with
// --noinput. Unlike Django it does not stop when the input is not a terminal:
// it reads the answers line by line then.
type createsuperuserCommand struct {
	app *Application
	// stderr receives the error messages of the prompts; nil means os.Stderr.
	stderr io.Writer
	// input and getpass replace the line and password reading in tests (the
	// mock_inputs of Django's tests); nil means the application's input.
	input   func(prompt string) (string, error)
	getpass func(prompt string) (string, error)
	// systemUsername replaces auth.SystemUsername in tests.
	systemUsername func() string
}

// Name returns "createsuperuser".
func (*createsuperuserCommand) Name() string { return "createsuperuser" }

// Help returns Django's help text of the command.
func (*createsuperuserCommand) Help() string { return "Used to create a superuser." }

// Run creates the superuser from args, the prompts and the environment.
func (c *createsuperuserCommand) Run(args []string) error {
	if c.app.DB == nil {
		return errNoDatabase
	}
	fs := flag.NewFlagSet("createsuperuser", flag.ContinueOnError)
	fs.SetOutput(c.app.Out)
	var username, email optionalString
	fs.Var(&username, "username", "Specifies the login for the superuser.")
	fs.Var(&email, "email", "Specifies the email for the superuser.")
	var noInput bool
	noInputHelp := "Tells Django to NOT prompt the user for input of any kind. You must use " +
		"--username with --noinput, along with an option for any other required field. " +
		"Superusers created with --noinput will not be able to log in until they're given a valid password."
	fs.BoolVar(&noInput, "noinput", false, noInputHelp)
	fs.BoolVar(&noInput, "no-input", false, noInputHelp)
	verbosity := fs.Int("verbosity", 1, "Verbosity level; 0=minimal output, 1=normal output.")
	fs.IntVar(verbosity, "v", 1, "Verbosity level (short form).")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("createsuperuser: unrecognized arguments: %s", strings.Join(fs.Args(), " "))
	}

	ctx := context.Background()
	var data superuserData
	var err error
	if noInput {
		data, err = c.collectNoInput(ctx, username, email)
	} else {
		data, err = c.collectInteractive(ctx, username, email)
	}
	if err != nil {
		return err
	}
	u, err := auth.CreateSuperuser(ctx, c.app.DB, data.username, data.email, data.password)
	if err != nil {
		return err
	}
	if data.emptyPassword {
		// DJANGO_SUPERUSER_PASSWORD was set to "": Django hashes the empty
		// password, where CreateSuperuser leaves the password unusable.
		if err := auth.ChangePassword(ctx, c.app.DB, u, ""); err != nil {
			return err
		}
	}
	if *verbosity >= 1 {
		_, _ = fmt.Fprintln(c.app.Out, "Superuser created successfully.")
	}
	return nil
}

// superuserData is what createsuperuser collected. An empty password gives
// the user an unusable password unless emptyPassword is set.
type superuserData struct {
	username, email, password string
	emptyPassword             bool
}

// collectNoInput reads the data of --noinput from the flags and the
// environment.
func (c *createsuperuserCommand) collectNoInput(ctx context.Context, username, email optionalString) (superuserData, error) {
	var data superuserData
	if p, ok := os.LookupEnv("DJANGO_SUPERUSER_PASSWORD"); ok {
		data.password, data.emptyPassword = p, p == ""
	}
	if !username.set {
		username.value, username.set = os.LookupEnv("DJANGO_SUPERUSER_USERNAME")
	}
	if !username.set {
		return data, &commandError{msg: "You must use --username with --noinput."}
	}
	msg, err := c.validateUsername(ctx, username.value)
	if err != nil {
		return data, err
	}
	if msg != "" {
		return data, &commandError{msg: msg}
	}
	data.username = username.value

	envEmail, envSet := os.LookupEnv("DJANGO_SUPERUSER_EMAIL")
	value := email.value
	if value == "" {
		value = envEmail
	}
	if value == "" {
		if (email.set && email.value == "") || (envSet && envEmail == "") {
			return data, nil
		}
		return data, &commandError{msg: "You must use --email with --noinput."}
	}
	if msgs := auth.EmailFieldErrors(value); len(msgs) > 0 {
		return data, &commandError{msg: strings.Join(msgs, "; ")}
	}
	data.email = value
	return data, nil
}

// collectInteractive asks for every value that the flags do not give.
func (c *createsuperuserCommand) collectInteractive(ctx context.Context, username, email optionalString) (superuserData, error) {
	var data superuserData
	in := c.inputs()
	system := auth.SystemUsername
	if c.systemUsername != nil {
		system = c.systemUsername
	}
	defaultUsername, err := auth.DefaultUsername(ctx, c.app.DB, system())
	if err != nil {
		return data, err
	}
	if username.set {
		if username.value == "" {
			return data, &commandError{msg: "Username cannot be blank."}
		}
		msg, err := c.validateUsername(ctx, username.value)
		if err != nil {
			return data, err
		}
		if msg != "" {
			c.printErr(msg)
			username.set = false
		}
	}
	for !username.set {
		prompt := "Username: "
		if defaultUsername != "" {
			prompt = fmt.Sprintf("Username (leave blank to use '%s'): ", defaultUsername)
		}
		value, ok, err := c.inputField(in.input, prompt, defaultUsername, auth.UsernameFieldErrors)
		if err != nil {
			return data, err
		}
		if !ok {
			continue
		}
		msg, err := c.validateUsername(ctx, value)
		if err != nil {
			return data, err
		}
		if msg != "" {
			c.printErr(msg)
			continue
		}
		username.value, username.set = value, true
	}
	data.username = username.value

	if email.set {
		if msgs := auth.EmailFieldErrors(email.value); len(msgs) > 0 {
			return data, &commandError{msg: strings.Join(msgs, "; ")}
		}
	}
	for !email.set {
		value, ok, err := c.inputField(in.input, "Email address: ", "", auth.EmailFieldErrors)
		if err != nil {
			return data, err
		}
		email.value, email.set = value, ok
	}
	data.email = email.value

	data.password, err = c.promptPassword(in, &auth.User{Username: data.username, Email: data.email})
	return data, err
}

// promptPassword asks for the password twice until both match, are not
// blank and pass the password validators, or the user bypasses the
// validators.
func (c *createsuperuserCommand) promptPassword(in promptFuncs, fake *auth.User) (string, error) {
	for {
		p1, err := in.getpass("Password: ")
		if err != nil {
			return "", err
		}
		p2, err := in.getpass("Password (again): ")
		if err != nil {
			return "", err
		}
		if p1 != p2 {
			c.printErr("Error: Your passwords didn't match.")
			continue
		}
		if strings.TrimSpace(p1) == "" {
			c.printErr("Error: Blank passwords aren't allowed.")
			continue
		}
		var perrs auth.PasswordErrors
		if err := auth.ValidatePassword(p2, fake, nil); errors.As(err, &perrs) {
			c.printErr(strings.Join(perrs.Messages(), "\n"))
			answer, err := in.input("Bypass password validation and create user anyway? [y/N]: ")
			if err != nil {
				return "", err
			}
			if strings.ToLower(answer) != "y" {
				continue
			}
		} else if err != nil {
			return "", err
		}
		return p1, nil
	}
}

// inputField asks for one field value (Django's get_input_data): an empty
// answer takes def when def is not empty, and a value that fails clean is
// reported as "Error: <messages>" and gives ok false.
func (c *createsuperuserCommand) inputField(input func(string) (string, error), prompt, def string, clean func(string) []string) (value string, ok bool, err error) {
	value, err = input(prompt)
	if err != nil {
		return "", false, err
	}
	if def != "" && value == "" {
		value = def
	}
	if msgs := clean(value); len(msgs) > 0 {
		c.printErr("Error: " + strings.Join(msgs, "; "))
		return "", false, nil
	}
	return value, true, nil
}

// validateUsername returns the message for a username that is taken, blank
// or invalid, and "" for a username that can be created (Django's
// _validate_username).
func (c *createsuperuserCommand) validateUsername(ctx context.Context, username string) (string, error) {
	taken, err := usernameTaken(ctx, c.app, username)
	if err != nil {
		return "", err
	}
	if taken {
		return "Error: That username is already taken.", nil
	}
	if username == "" {
		return "Username cannot be blank.", nil
	}
	return strings.Join(auth.UsernameFieldErrors(username), "; "), nil
}

// usernameTaken reports whether a user with exactly this username exists.
func usernameTaken(ctx context.Context, app *Application, username string) (bool, error) {
	taken, err := orm.Query[auth.User](app.DB).Filter("username", username).Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("djangogo: look up username: %w", err)
	}
	return taken, nil
}

// promptFuncs are the two ways of reading an answer.
type promptFuncs struct {
	input, getpass func(prompt string) (string, error)
}

// inputs returns the reading functions, with the test replacements when set.
func (c *createsuperuserCommand) inputs() promptFuncs {
	return commandPrompts(c.app, c.input, c.getpass)
}

// commandPrompts returns input and getpass, falling back to the input of app
// for a nil one.
func commandPrompts(app *Application, input, getpass func(string) (string, error)) promptFuncs {
	if input != nil && getpass != nil {
		return promptFuncs{input: input, getpass: getpass}
	}
	ci := newCommandInput(app)
	if input == nil {
		input = ci.input
	}
	if getpass == nil {
		getpass = ci.getpass
	}
	return promptFuncs{input: input, getpass: getpass}
}

// printErr writes msg and a line break to the command's error output.
func (c *createsuperuserCommand) printErr(msg string) {
	_, _ = fmt.Fprintln(errWriter(c.stderr), msg)
}

// errWriter returns w, or os.Stderr when w is nil.
func errWriter(w io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return os.Stderr
}
