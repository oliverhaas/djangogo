package djangogo

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/auth"
)

// withCreatesuperuserValidators is the class decorator of
// CreatesuperuserManagementCommandTestCase: the numeric validator only.
func withCreatesuperuserValidators(t *testing.T) {
	t.Helper()
	withPasswordValidators(t, auth.NumericPasswordValidator{})
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_no_email_argument
func TestCreatesuperuserNoEmailArgument(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	assertCommandError(t, c.Run([]string{"--noinput", "--username", "joe"}), "You must use --email with --noinput.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_basic_usage
//
// Check the operation of the createsuperuser management command.
func TestCreatesuperuserBasicUsage(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, nil)
	if err := c.Run([]string{"--noinput", "--username", "joe", "--email", "joe@somewhere.org"}); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Superuser created successfully.")
	u := loadUser(t, c.app, "joe")
	if u.Email != "joe@somewhere.org" {
		t.Errorf("email = %q", u.Email)
	}
	// created password should be unusable
	if u.HasUsablePassword() {
		t.Error("the password should be unusable")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validate_username
func TestCreatesuperuserValidateUsername(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	err := c.Run([]string{"--noinput", "--username", "\U0001F920", "--email", "joe@somewhere.org"})
	assertCommandError(t, err, "Enter a valid username. This value may contain only letters, numbers, "+
		"and @/./+/-/_ characters.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_non_ascii_verbose_name
func TestCreatesuperuserNonASCIIVerboseName(t *testing.T) {
	t.Skip("Djan-Go-Go has no translatable verbose names; the username prompt is fixed")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_verbosity_zero
//
// Output of the management command can be suppressed.
func TestCreatesuperuserVerbosityZero(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, nil)
	if err := c.Run([]string{"--noinput", "--username", "joe2", "--email", "joe2@somewhere.org", "--verbosity", "0"}); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "")
	u := loadUser(t, c.app, "joe2")
	if u.Email != "joe2@somewhere.org" {
		t.Errorf("email = %q", u.Email)
	}
	if u.HasUsablePassword() {
		t.Error("the password should be unusable")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_email_in_username
func TestCreatesuperuserEmailInUsername(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	err := c.Run([]string{"--noinput", "--username", "joe+admin@somewhere.org", "--email", "joe@somewhere.org", "--verbosity", "0"})
	if err != nil {
		t.Fatal(err)
	}
	u := loadUser(t, c.app, "joe+admin@somewhere.org")
	if u.Email != "joe@somewhere.org" {
		t.Errorf("email = %q", u.Email)
	}
	if u.HasUsablePassword() {
		t.Error("the password should be unusable")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_skip_if_not_in_TTY
func TestCreatesuperuserSkipIfNotInTTY(t *testing.T) {
	t.Skip("Djan-Go-Go reads answers line by line when the input is not a terminal")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_passing_stdin
func TestCreatesuperuserPassingStdin(t *testing.T) {
	t.Skip("Djan-Go-Go commands have no stdin option; the input is the In field of the application")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_default_username
//
// createsuperuser uses a default username when one isn't provided. Django
// sets a custom user model here without need; the port uses the user model.
func TestCreatesuperuserDefaultUsername(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper("password", "password"), "username": "", "email": "",
	})
	// Get the default username before creating a user.
	defaultUsername, err := auth.DefaultUsername(context.Background(), c.app.DB, auth.SystemUsername())
	if err != nil {
		t.Fatal(err)
	}
	if defaultUsername == "" {
		t.Skip("the system username gives no default username")
	}
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Superuser created successfully.")
	loadUser(t, c.app, defaultUsername)
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_password_validation
//
// Creation should fail if the password fails validation.
func TestCreatesuperuserPasswordValidation(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper("1234567890", "1234567890", "password", "password"),
		"username": "joe1234567890", "email": "", "bypass": "n",
	})
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "This password is entirely numeric.\nSuperuser created successfully.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validate_password_against_username
func TestCreatesuperuserValidatePasswordAgainstUsername(t *testing.T) {
	withPasswordValidators(t, auth.UserAttributeSimilarityValidator{})
	username := "supremelycomplex"
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper(username, username, "superduperunguessablepassword", "superduperunguessablepassword"),
		"username": username, "email": "", "bypass": "n",
	})
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "The password is too similar to the username.\nSuperuser created successfully.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_blank_username
//
// Creation fails if --username is blank.
func TestCreatesuperuserBlankUsername(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	assertCommandError(t, c.Run([]string{"--username", ""}), "Username cannot be blank.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_blank_username_non_interactive
func TestCreatesuperuserBlankUsernameNonInteractive(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	assertCommandError(t, c.Run([]string{"--username", "", "--noinput"}), "Username cannot be blank.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_blank_email_allowed_non_interactive
func TestCreatesuperuserBlankEmailAllowedNonInteractive(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, nil)
	if err := c.Run([]string{"--email", "", "--username", "joe", "--noinput"}); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Superuser created successfully.")
	if u := loadUser(t, c.app, "joe"); u.Email != "" {
		t.Errorf("email = %q, want empty", u.Email)
	}
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_blank_email_allowed_non_interactive_environment_variable
func TestCreatesuperuserBlankEmailAllowedNonInteractiveEnvironmentVariable(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, nil)
	t.Setenv("DJANGO_SUPERUSER_EMAIL", "")
	if err := c.Run([]string{"--username", "joe", "--noinput"}); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Superuser created successfully.")
	if u := loadUser(t, c.app, "joe"); u.Email != "" {
		t.Errorf("email = %q, want empty", u.Email)
	}
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_password_validation_bypass
//
// Password validation can be bypassed by entering 'y' at the prompt.
func TestCreatesuperuserPasswordValidationBypass(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, mockInputs{
		"password": "1234567890", "username": "joe1234567890", "email": "", "bypass": "y",
	})
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "This password is entirely numeric.\nSuperuser created successfully.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_invalid_username
//
// Creation fails if the username fails validation.
func TestCreatesuperuserInvalidUsername(t *testing.T) {
	withCreatesuperuserValidators(t)
	// Enter an invalid (too long) username first and then a valid one.
	invalidUsername := strings.Repeat("x", auth.UsernameMaxLength) + "y"
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper("password", "password"),
		"username": popper(invalidUsername, "janet"), "email": "",
	})
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, fmt.Sprintf("Error: Ensure this value has at most %d characters (it has %d).\n"+
		"Superuser created successfully.", auth.UsernameMaxLength, len(invalidUsername)))
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_keyboard_interrupt
func TestCreatesuperuserKeyboardInterrupt(t *testing.T) {
	t.Skip("Go has no KeyboardInterrupt; the end of the input stops the command with an error")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_existing_username
//
// Creation fails if the username already exists.
func TestCreatesuperuserExistingUsername(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper("password", "password"),
		"username": popper("janet", "joe"), "email": "",
	})
	if _, err := auth.CreateUser(context.Background(), c.app.DB, "janet", "", "", false, false, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Error: That username is already taken.\nSuperuser created successfully.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_existing_username_non_interactive
//
// Creation fails if the username already exists. Django sets a custom user
// model with a unique constraint here; the port uses the user model.
func TestCreatesuperuserExistingUsernameNonInteractive(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	if _, err := auth.CreateUser(context.Background(), c.app.DB, "janet", "", "", false, false, true); err != nil {
		t.Fatal(err)
	}
	err := c.Run([]string{"--username", "janet", "--email", "", "--noinput"})
	assertCommandError(t, err, "Error: That username is already taken.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_existing_username_provided_via_option_and_interactive
//
// The command gets --username janet and runs interactively.
func TestCreatesuperuserExistingUsernameProvidedViaOptionAndInteractive(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper("password", "password"), "username": "janet1", "email": "test@test.com",
	})
	if _, err := auth.CreateUser(context.Background(), c.app.DB, "janet", "", "", false, false, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Run([]string{"--username", "janet"}); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Error: That username is already taken.\nSuperuser created successfully.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validation_mismatched_passwords
//
// Creation should fail if the user enters mismatched passwords.
func TestCreatesuperuserValidationMismatchedPasswords(t *testing.T) {
	withCreatesuperuserValidators(t)
	// The first two passwords do not match, but the second two do match and
	// are valid.
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper("password", "not password", "password2", "password2"),
		"username": "joe1234567890", "email": "",
	})
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Error: Your passwords didn't match.\nSuperuser created successfully.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validation_blank_password_entered
//
// Creation should fail if the user enters blank passwords.
func TestCreatesuperuserValidationBlankPasswordEntered(t *testing.T) {
	withCreatesuperuserValidators(t)
	// The first two passwords are empty strings, but the second two are
	// valid.
	c, out := newCreatesuperuser(t, mockInputs{
		"password": popper("", "", "password2", "password2"),
		"username": "joe1234567890", "email": "",
	})
	if err := c.Run(nil); err != nil {
		t.Fatal(err)
	}
	assertTrimmed(t, out, "Error: Blank passwords aren't allowed.\nSuperuser created successfully.")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_ignore_environment_variable_non_interactive
//
// Environment variables are ignored in non-interactive mode, if provided by
// command line arguments.
func TestCreatesuperuserIgnoreEnvironmentVariableNonInteractive(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	t.Setenv("DJANGO_SUPERUSER_USERNAME", "test_superuser")
	t.Setenv("DJANGO_SUPERUSER_EMAIL", "joe@somewhere.org")
	err := c.Run([]string{"--noinput", "--username", "cmd_superuser", "--email", "cmd@somewhere.org", "--verbosity", "0"})
	if err != nil {
		t.Fatal(err)
	}
	u := loadUser(t, c.app, "cmd_superuser")
	if u.Email != "cmd@somewhere.org" {
		t.Errorf("email = %q", u.Email)
	}
	if u.HasUsablePassword() {
		t.Error("the password should be unusable")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_ignore_environment_variable_interactive
//
// Environment variables are ignored in interactive mode.
func TestCreatesuperuserIgnoreEnvironmentVariableInteractive(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, mockInputs{"password": "cmd_password"})
	t.Setenv("DJANGO_SUPERUSER_PASSWORD", "test_password")
	t.Setenv("DJANGO_SUPERUSER_USERNAME", "test_superuser")
	t.Setenv("DJANGO_SUPERUSER_EMAIL", "joe@somewhere.org")
	err := c.Run([]string{"--username", "cmd_superuser", "--email", "cmd@somewhere.org", "--verbosity", "0"})
	if err != nil {
		t.Fatal(err)
	}
	u := loadUser(t, c.app, "cmd_superuser")
	if u.Email != "cmd@somewhere.org" {
		t.Errorf("email = %q", u.Email)
	}
	if !u.CheckPassword("cmd_password") {
		t.Error("the password of the prompt was not set")
	}
}

// Ported from Django: tests/auth_tests/test_management.py::MultiDBCreatesuperuserTestCase.test_createsuperuser_command_with_database_option
func TestCreatesuperuserCommandWithDatabaseOption(t *testing.T) {
	t.Skip("Djan-Go-Go has one database")
}

// Ported from Django: tests/auth_tests/test_management.py::MultiDBCreatesuperuserTestCase.test_createsuperuser_command_suggested_username_with_database_option
func TestCreatesuperuserCommandSuggestedUsernameWithDatabaseOption(t *testing.T) {
	t.Skip("Djan-Go-Go has one database")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_swappable_user
func TestCreatesuperuserSwappableUser(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_swappable_user_missing_required_field
func TestCreatesuperuserSwappableUserMissingRequiredField(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_swappable_user_username_non_unique
func TestCreatesuperuserSwappableUserUsernameNonUnique(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_swappable_user_no_natural_key_non_interactive
func TestCreatesuperuserSwappableUserNoNaturalKeyNonInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_swappable_user_no_natural_key_interactive
func TestCreatesuperuserSwappableUserNoNaturalKeyInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_swappable_user_no_natural_key_duplicate_allowed
func TestCreatesuperuserSwappableUserNoNaturalKeyDuplicateAllowed(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_fields_with_fk
func TestCreatesuperuserFieldsWithFK(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_fields_with_fk_interactive
func TestCreatesuperuserFieldsWithFKInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_fields_with_fk_via_option_interactive
func TestCreatesuperuserFieldsWithFKViaOptionInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validate_fk
func TestCreatesuperuserValidateFK(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validate_fk_environment_variable
func TestCreatesuperuserValidateFKEnvironmentVariable(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validate_fk_via_option_interactive
func TestCreatesuperuserValidateFKViaOptionInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_fields_with_m2m
func TestCreatesuperuserFieldsWithM2M(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_fields_with_m2m_interactive
func TestCreatesuperuserFieldsWithM2MInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_fields_with_m2m_interactive_blank
func TestCreatesuperuserFieldsWithM2MInteractiveBlank(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_fields_with_m2m_and_through
func TestCreatesuperuserFieldsWithM2MAndThrough(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validate_password_against_required_fields
func TestCreatesuperuserValidatePasswordAgainstRequiredFields(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_validate_password_against_required_fields_via_option
func TestCreatesuperuserValidatePasswordAgainstRequiredFieldsViaOption(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_existing_username_meta_unique_constraint
func TestCreatesuperuserExistingUsernameMetaUniqueConstraint(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_usermodel_without_password
func TestCreatesuperuserUsermodelWithoutPassword(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_usermodel_without_password_interactive
func TestCreatesuperuserUsermodelWithoutPasswordInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_environment_variable_non_interactive
func TestCreatesuperuserEnvironmentVariableNonInteractive(t *testing.T) {
	withCreatesuperuserValidators(t)
	c, _ := newCreatesuperuser(t, nil)
	t.Setenv("DJANGO_SUPERUSER_PASSWORD", "test_password")
	t.Setenv("DJANGO_SUPERUSER_USERNAME", "test_superuser")
	t.Setenv("DJANGO_SUPERUSER_EMAIL", "joe@somewhere.org")
	t.Setenv("DJANGO_SUPERUSER_FIRST_NAME", "ignored_first_name")
	if err := c.Run([]string{"--noinput", "--verbosity", "0"}); err != nil {
		t.Fatal(err)
	}
	u := loadUser(t, c.app, "test_superuser")
	if u.Email != "joe@somewhere.org" {
		t.Errorf("email = %q", u.Email)
	}
	if !u.CheckPassword("test_password") {
		t.Error("the password of DJANGO_SUPERUSER_PASSWORD was not set")
	}
	// Environment variables are ignored for non-required fields.
	if u.FirstName != "" {
		t.Errorf("first name = %q, want empty", u.FirstName)
	}
}

// Ported from Django: tests/auth_tests/test_management.py::CreatesuperuserManagementCommandTestCase.test_environment_variable_m2m_non_interactive
func TestCreatesuperuserEnvironmentVariableM2MNonInteractive(t *testing.T) {
	t.Skip("Djan-Go-Go has no custom user models")
}
