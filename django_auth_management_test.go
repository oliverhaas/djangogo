package djangogo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/manage"
)

// mockInputs emulates Django's mock_inputs decorator. A key is a shortcut
// ("username", "email", "bypass", "password") or a full prompt; a value is a
// string or a func() string that returns the answer.
type mockInputs map[string]any

// answer returns the value of v: v itself for a string, its result for a
// func() string.
func answer(v any) string {
	if f, ok := v.(func() string); ok {
		return f()
	}
	return v.(string)
}

// install sets the input, getpass and systemUsername of c from the mocks
// (the decorator's replacement of builtins.input and getpass.getpass). system
// is the patched get_system_username; nil keeps the real one.
func (m mockInputs) install(t *testing.T, c *createsuperuserCommand, system func() string) {
	t.Helper()
	if system == nil {
		system = auth.SystemUsername
	}
	c.systemUsername = system
	c.getpass = func(string) (string, error) {
		v, ok := m["password"]
		if !ok {
			t.Fatal(`getpass was called, but the mock inputs have no "password"`)
		}
		return answer(v), nil
	}
	c.input = func(prompt string) (string, error) {
		for key, val := range m {
			var prompts []string
			switch key {
			case "bypass":
				prompts = []string{"Bypass password validation and create user anyway? [y/N]: "}
			case "email":
				prompts = []string{"Email address: "}
			case "username":
				def, err := auth.DefaultUsername(context.Background(), c.app.DB, system())
				if err != nil {
					return "", err
				}
				prompts = []string{"Username: ", fmt.Sprintf("Username (leave blank to use '%s'): ", def)}
			default:
				prompts = []string{key}
			}
			for _, p := range prompts {
				if p == prompt {
					return answer(val), nil
				}
			}
		}
		return "", mockInputNotFound(prompt)
	}
}

// mockInputNotFound is the error of a prompt that no mock input answers
// (the ValueError of Django's mock_input).
type mockInputNotFound string

// Error returns Django's message.
func (e mockInputNotFound) Error() string {
	return "Mock input for '" + string(e) + "' not found."
}

// popper returns a func that hands out items in order, like the
// entered_passwords.pop(0) of the Django tests.
func popper(items ...string) func() string {
	return func() string {
		v := items[0]
		items = items[1:]
		return v
	}
}

// withPasswordValidators sets the password validators for the test, as
// override_settings(AUTH_PASSWORD_VALIDATORS=...). Django's test settings
// have none, so a test without an override passes none.
func withPasswordValidators(t *testing.T, validators ...auth.PasswordValidator) {
	t.Helper()
	cfg := auth.CurrentConfig()
	cfg.PasswordValidators = append([]auth.PasswordValidator{}, validators...)
	t.Cleanup(auth.Configure(cfg))
}

// clearSuperuserEnv unsets the DJANGO_SUPERUSER_* variables for the test.
func clearSuperuserEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"DJANGO_SUPERUSER_PASSWORD", "DJANGO_SUPERUSER_USERNAME", "DJANGO_SUPERUSER_EMAIL"} {
		unsetEnv(t, name)
	}
}

// newCreatesuperuser returns a createsuperuser command whose stdout and
// stderr are one buffer, as the Django tests that pass the same StringIO for
// both. Its mock inputs and the real system username are installed.
func newCreatesuperuser(t *testing.T, inputs mockInputs) (*createsuperuserCommand, *bytes.Buffer) {
	t.Helper()
	clearSuperuserEnv(t)
	app := newSuperuserApp(t)
	out := app.Out.(*bytes.Buffer)
	c := &createsuperuserCommand{app: app, stderr: out}
	inputs.install(t, c, nil)
	return c, out
}

// assertCommandError checks that err is a command error whose message is
// want.
func assertCommandError(t *testing.T, err error, want string) {
	t.Helper()
	var ce *manage.CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v (%T), want a command error %q", err, err, want)
	}
	if ce.Error() != want {
		t.Errorf("command error = %q, want %q", ce.Error(), want)
	}
}

// passwordOf returns a pointer to the stored user, whose password methods
// need one.
func passwordOf(t *testing.T, app *Application, username string) *auth.User {
	t.Helper()
	u := loadUser(t, app, username)
	return &u
}

// assertTrimmed checks the output, trimmed of surrounding white space like
// Django's getvalue().strip().
func assertTrimmed(t *testing.T, out *bytes.Buffer, want string) {
	t.Helper()
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// Ported from Django: tests/auth_tests/test_management.py::MockInputTests.test_input_not_found
func TestMockInputsInputNotFound(t *testing.T) {
	c, _ := newCreatesuperuser(t, mockInputs{"username": "alice"})
	err := c.Run(nil)
	if err == nil || err.Error() != "Mock input for 'Email address: ' not found." {
		t.Errorf("Run = %v, want the mock input error for the email prompt", err)
	}
}

// changepasswordFixture returns a changepassword command and its stdout and
// stderr. The user joe with the password qwerty exists, and the numeric
// validator is the only one (the class decorator and setUpTestData).
func changepasswordFixture(t *testing.T, getpass func(string) (string, error)) (c *changepasswordCommand, stdout, stderr *bytes.Buffer) {
	t.Helper()
	withPasswordValidators(t, auth.NumericPasswordValidator{})
	app := newSuperuserApp(t)
	if _, err := auth.CreateUser(context.Background(), app.DB, "joe", "", "qwerty", false, false, true); err != nil {
		t.Fatal(err)
	}
	stderr = &bytes.Buffer{}
	return &changepasswordCommand{app: app, stderr: stderr, getpass: getpass}, app.Out.(*bytes.Buffer), stderr
}

// fixedPassword returns a getpass that answers every prompt with password.
func fixedPassword(password string) func(string) (string, error) {
	return func(string) (string, error) { return password, nil }
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_get_pass
func TestChangepasswordGetPass(t *testing.T) {
	c, _, _ := changepasswordFixture(t, fixedPassword("password"))
	if err := c.Run([]string{"joe"}); err != nil {
		t.Fatal(err)
	}
	if !passwordOf(t, c.app, "joe").CheckPassword("password") {
		t.Error("the password was not changed")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_get_pass_no_input
func TestChangepasswordGetPassNoInput(t *testing.T) {
	c, _, _ := changepasswordFixture(t, fixedPassword(""))
	assertCommandError(t, c.Run([]string{"joe"}), "aborted")
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_system_username
//
// The system username is used if no username is provided.
func TestChangepasswordSystemUsername(t *testing.T) {
	c, _, _ := changepasswordFixture(t, fixedPassword("new_password"))
	username := auth.SystemUsername()
	if username == "" {
		t.Skip("the system username is not known")
	}
	if _, err := auth.CreateUser(context.Background(), c.app.DB, username, "", "qwerty", false, false, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	if !passwordOf(t, c.app, username).CheckPassword("new_password") {
		t.Error("the password of the system user was not changed")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_nonexistent_username
func TestChangepasswordNonexistentUsername(t *testing.T) {
	c, _, _ := changepasswordFixture(t, nil)
	assertCommandError(t, c.Run([]string{"test"}), "user 'test' does not exist")
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_that_changepassword_command_changes_joes_password
//
// Executing the changepassword management command should change joe's
// password.
func TestChangepasswordCommandChangesJoesPassword(t *testing.T) {
	c, stdout, _ := changepasswordFixture(t, fixedPassword("not qwerty"))
	if !passwordOf(t, c.app, "joe").CheckPassword("qwerty") {
		t.Fatal("joe's password is not qwerty")
	}
	if err := c.Run([]string{"joe"}); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, stdout, "Changing password for user 'joe'\nPassword changed successfully for user 'joe'")
	if !passwordOf(t, c.app, "joe").CheckPassword("not qwerty") {
		t.Error("the password was not changed")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_that_max_tries_exits_1
//
// A command error is returned if the user enters mismatched passwords three
// times.
func TestChangepasswordMaxTriesExits1(t *testing.T) {
	// The answer is the prompt, as str(args) in Django, so the two passwords
	// of a try differ.
	c, _, _ := changepasswordFixture(t, func(prompt string) (string, error) { return "(" + prompt + ")", nil })
	assertCommandError(t, c.Run([]string{"joe"}), "Aborting password change for user 'joe' after 3 attempts")
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_password_validation
//
// A command error is returned if the user enters passwords which fail
// validation three times.
func TestChangepasswordPasswordValidation(t *testing.T) {
	c, _, stderr := changepasswordFixture(t, fixedPassword("1234567890"))
	assertCommandError(t, c.Run([]string{"joe"}), "Aborting password change for user 'joe' after 3 attempts")
	if !strings.Contains(stderr.String(), "This password is entirely numeric.") {
		t.Errorf("stderr = %q, want it to contain the numeric message", stderr.String())
	}
}

// Ported from Django: tests/auth_tests/test_management.py::ChangepasswordManagementCommandTestCase.test_that_changepassword_command_works_with_nonascii_output
//
// #21627 -- Executing the changepassword management command should allow
// non-ASCII characters from the User object representation.
func TestChangepasswordCommandWorksWithNonASCIIOutput(t *testing.T) {
	c, _, _ := changepasswordFixture(t, fixedPassword("not qwerty"))
	if _, err := auth.CreateUser(context.Background(), c.app.DB, "J\xc3\xbalia", "", "qwerty", false, false, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Run([]string{"J\xc3\xbalia"}); err != nil {
		t.Fatal(err)
	}
}

// Ported from Django: tests/auth_tests/test_management.py::MultiDBChangepasswordManagementCommandTestCase.test_that_changepassword_command_with_database_option_uses_given_db
func TestChangepasswordCommandWithDatabaseOptionUsesGivenDB(t *testing.T) {
	t.Skip("Djan-Go-Go has one database")
}
