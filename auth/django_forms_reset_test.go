package auth_test

// Ports of SetPasswordFormTest, PasswordChangeFormTest and
// PasswordResetFormTest of tests/auth_tests/test_forms.py.
//
// Django's tests save the reset form without domain_override and get the
// domain "example.com" from contrib.sites. djangogo has no sites app, so the
// ports pass DomainOverride "example.com".

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/mail"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/templates"
)

// resetTestTemplates returns an engine with the registration templates of
// tests/auth_tests/templates that the password reset tests render.
func resetTestTemplates(t *testing.T) *templates.Engine {
	t.Helper()
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	engine, err := templates.NewEngineFS(fstest.MapFS{
		"registration/password_reset_email.html": file("{{ protocol }}://{{ domain }}/reset/{{ uid }}/{{ token }}/\n" +
			"Email email context: \"{{ greeting }}\"\n"),
		"registration/html_password_reset_email.html": file(
			"<html><a href=\"{{ protocol }}://{{ domain }}/reset/{{ uid }}/{{ token }}/\">Link</a></html>\n"),
		"registration/password_reset_subject.txt": file(
			"{% autoescape off %}Custom password reset on {{ site_name }}{% endautoescape %}"),
		"registration/password_reset_form.html": file("{{ form }}\n"),
		"registration/password_reset_done.html": file("Email sent\n"),
		"registration/password_reset_confirm.html": file("Hello, {{ form.user }}.\n\n{% if validlink %}\n" +
			"Please enter your new password: {{ form }}\n{% else %}\nThe password reset link was invalid\n{% endif %}\n"),
		"registration/password_reset_complete.html": file("Password reset successfully\n"),
	})
	must(t, err)
	return engine
}

// saveResetForm saves form with the test templates and the domain
// example.com, Django's form.save() in the test settings, after applying
// change to the options.
func saveResetForm(t *testing.T, db *orm.DB, form *auth.PasswordResetForm, change func(*auth.PasswordResetOptions)) {
	t.Helper()
	opts := auth.PasswordResetOptions{DomainOverride: "example.com", Templates: resetTestTemplates(t)}
	if change != nil {
		change(&opts)
	}
	must(t, form.Save(context.Background(), db, opts))
}

// assertEmailMessageSent fails unless the outbox holds exactly one message
// and returns it (PasswordResetFormTest.assertEmailMessageSent).
func assertEmailMessageSent(t *testing.T) *mail.Message {
	t.Helper()
	outbox := mail.Outbox()
	if len(outbox) != 1 {
		t.Fatalf("the outbox holds %d messages, want 1", len(outbox))
	}
	return outbox[0]
}

// assertNoEmailSent fails unless the outbox is empty.
func assertNoEmailSent(t *testing.T) {
	t.Helper()
	if n := len(mail.Outbox()); n != 0 {
		t.Fatalf("the outbox holds %d messages, want none", n)
	}
}

// createDummyUser creates the user jsmith (PasswordResetFormTest.create_dummy_user).
func createDummyUser(t *testing.T, db *orm.DB) (user *auth.User, email string) {
	t.Helper()
	email = "jsmith@example.com"
	return createUserWithEmail(t, db, "jsmith", "test123", email), email
}

// resetFormTest runs fn with the users of TestDataMixin and the locmem
// email backend.
func resetFormTest(t *testing.T, fn func(t *testing.T, db *orm.DB)) {
	t.Helper()
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		djangotest.LocmemMail(t)
		fn(t, db)
	})
}

// parsedMessage is an email as the receiver reads it.
type parsedMessage struct {
	header netmail.Header
	// mediaType and params are the parsed Content-Type.
	mediaType string
	params    map[string]string
	body      []byte
}

// parseMessage parses the bytes of msg (Django's msg.message()).
func parseMessage(t *testing.T, msg *mail.Message) parsedMessage {
	t.Helper()
	raw, err := msg.Bytes()
	must(t, err)
	m, err := netmail.ReadMessage(bytes.NewReader(raw))
	must(t, err)
	body, err := io.ReadAll(m.Body)
	must(t, err)
	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	must(t, err)
	return parsedMessage{header: m.Header, mediaType: mediaType, params: params, body: body}
}

// parts returns the content type and the content of each part of a
// multipart message.
func (p parsedMessage) parts(t *testing.T) (types, contents []string) {
	t.Helper()
	r := multipart.NewReader(bytes.NewReader(p.body), p.params["boundary"])
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			return types, contents
		}
		must(t, err)
		content, err := io.ReadAll(part)
		must(t, err)
		mediaType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		must(t, err)
		types = append(types, mediaType)
		contents = append(contents, string(content))
	}
}

// Ported from Django: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_password_verification
func TestDjangoSetPasswordFormPasswordVerification(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewSetPasswordForm(user).Bind(values("new_password1", "abc123", "new_password2", "abc"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "new_password2"), []string{msgPasswordMismatch})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_success
//
// No Go counterpart: password_changed and save(commit=False). The test
// checks the stored password instead.
func TestDjangoSetPasswordFormSuccess(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewSetPasswordForm(user).Bind(values("new_password1", "abc123", "new_password2", "abc123"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		_, err := form.Save(context.Background(), db)
		must(t, err)
		djangotest.AssertEqual(t, reload(t, db, user.ID).CheckPassword("abc123"), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_validates_password
func TestDjangoSetPasswordFormValidatesPassword(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		usePasswordValidators(t, auth.UserAttributeSimilarityValidator{}, auth.MinimumLengthValidator{MinLength: 12})
		user := userNamed(t, db, "testclient")
		for _, data := range [][]string{
			{"new_password1", "testclient", "new_password2", "testclient"},
			// SetPasswordForm does not consider usable_password for form
			// validation.
			{"new_password1", "testclient", "new_password2", "testclient", "usable_password", "false"},
		} {
			form := auth.NewSetPasswordForm(user).Bind(values(data...))
			djangotest.AssertEqual(t, form.IsValid(), false)
			errs := formErrors(form.Form, "new_password2")
			djangotest.AssertEqual(t, len(errs), 2)
			djangotest.AssertEqual(t, slices.Contains(errs, "The password is too similar to the username."), true)
			djangotest.AssertEqual(t, slices.Contains(errs,
				"This password is too short. It must contain at least 12 characters."), true)
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_no_password
func TestDjangoSetPasswordFormNoPassword(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewSetPasswordForm(user).Bind(values("new_password1", "new-password"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "new_password2"), []string{msgFieldRequired})
		form = auth.NewSetPasswordForm(user).Bind(values())
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "new_password1"), []string{msgFieldRequired})
		djangotest.AssertEqual(t, formErrors(form.Form, "new_password2"), []string{msgFieldRequired})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_password_whitespace_not_stripped
func TestDjangoSetPasswordFormPasswordWhitespaceNotStripped(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		data := values("new_password1", "   password   ", "new_password2", "   password   ")
		form := auth.NewSetPasswordForm(user).Bind(data)
		djangotest.AssertEqual(t, form.IsValid(), true)
		djangotest.AssertEqual(t, form.CleanedData()["new_password1"], any(data.Get("new_password1")))
		djangotest.AssertEqual(t, form.CleanedData()["new_password2"], any(data.Get("new_password2")))
	})
}

// No Go counterpart: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_help_text_translation
// The test renders French help texts; translation is outside the scope of
// djangogo.

// Ported from Django: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_html_autocomplete_attributes
func TestDjangoSetPasswordFormHTMLAutocompleteAttributes(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewSetPasswordForm(userNamed(t, db, "testclient"))
		for _, field := range []string{"new_password1", "new_password2"} {
			djangotest.AssertEqual(t, widgetAttrs(t, form.Form, field)["autocomplete"], any("new-password"))
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::SetPasswordFormTest.test_password_extra_validations
func TestDjangoSetPasswordFormPasswordExtraValidations(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		data := values("new_password1", "abc", "new_password2", "abc")
		for _, failing := range [][]string{{"new_password1"}, {"new_password2"}, {"new_password1", "new_password2"}} {
			t.Run(strings.Join(failing, ","), func(t *testing.T) {
				want := map[string][]string{}
				for _, field := range failing {
					want[field] = []string{"Extra validation for " + field + "."}
				}
				form := auth.NewSetPasswordForm(user)
				form.CleanFieldWith("new_password1", failingField("new_password1", failing))
				form.CleanFieldWith("new_password2", failingField("new_password2", failing))
				bound := form.Bind(data)
				djangotest.AssertEqual(t, bound.IsValid(), false)
				djangotest.AssertEqual(t, bound.Errors().Messages(), want)
			})
		}
	})
}

// No Django test exists: {{ form.user }} in a template prints the username,
// as the confirm page of Django's tests shows it.
func TestSetPasswordFormTemplateUser(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewSetPasswordForm(userNamed(t, db, "testclient"))
		engine, err := templates.NewEngineFS(fstest.MapFS{"t.html": {Data: []byte("Hello, {{ form.user }}.")}})
		must(t, err)
		out, err := engine.Render("t.html", map[string]any{"form": form})
		must(t, err)
		djangotest.AssertEqual(t, out, "Hello, testclient.")
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordChangeFormTest.test_incorrect_password
func TestDjangoPasswordChangeFormIncorrectPassword(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewPasswordChangeForm(user).Bind(values(
			"old_password", "test", "new_password1", "abc123", "new_password2", "abc123"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "old_password"),
			[]string{"Your old password was entered incorrectly. Please enter it again."})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordChangeFormTest.test_password_verification
func TestDjangoPasswordChangeFormPasswordVerification(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewPasswordChangeForm(user).Bind(values(
			"old_password", "password", "new_password1", "abc123", "new_password2", "abc"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "new_password2"), []string{msgPasswordMismatch})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordChangeFormTest.test_success
//
// No Go counterpart: password_changed and save(commit=False). The test
// checks the stored password instead.
func TestDjangoPasswordChangeFormSuccess(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewPasswordChangeForm(user).Bind(values(
			"old_password", "password", "new_password1", "abc123", "new_password2", "abc123"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		_, err := form.Save(context.Background(), db)
		must(t, err)
		djangotest.AssertEqual(t, reload(t, db, user.ID).CheckPassword("abc123"), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordChangeFormTest.test_field_order
func TestDjangoPasswordChangeFormFieldOrder(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewPasswordChangeForm(userNamed(t, db, "testclient")).Bind(values())
		djangotest.AssertEqual(t, form.FieldNames(), []string{"old_password", "new_password1", "new_password2"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordChangeFormTest.test_password_whitespace_not_stripped
func TestDjangoPasswordChangeFormPasswordWhitespaceNotStripped(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		must(t, user.SetPassword("   oldpassword   "))
		data := values("old_password", "   oldpassword   ", "new_password1", " pass ", "new_password2", " pass ")
		form := auth.NewPasswordChangeForm(user).Bind(data)
		djangotest.AssertEqual(t, form.IsValid(), true)
		for _, field := range []string{"old_password", "new_password1", "new_password2"} {
			djangotest.AssertEqual(t, form.CleanedData()[field], any(data.Get(field)))
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordChangeFormTest.test_html_autocomplete_attributes
func TestDjangoPasswordChangeFormHTMLAutocompleteAttributes(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewPasswordChangeForm(userNamed(t, db, "testclient"))
		djangotest.AssertEqual(t, widgetAttrs(t, form.Form, "old_password")["autocomplete"], any("current-password"))
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_invalid_email
func TestDjangoPasswordResetFormInvalidEmail(t *testing.T) {
	form := auth.NewPasswordResetForm().Bind(values("email", "not valid"))
	djangotest.AssertEqual(t, form.IsValid(), false)
	djangotest.AssertEqual(t, formErrors(form.Form, "email"), []string{"Enter a valid email address."})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_user_email_unicode_collision
func TestDjangoPasswordResetFormUserEmailUnicodeCollision(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		createUserWithEmail(t, db, "mike123", "test123", "mike@example.org")
		createUserWithEmail(t, db, "mike456", "test123", "mıke@example.org")
		form := auth.NewPasswordResetForm().Bind(values("email", "mıke@example.org"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		djangotest.AssertEqual(t, assertEmailMessageSent(t).To, []string{"mıke@example.org"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_user_email_domain_unicode_collision
func TestDjangoPasswordResetFormUserEmailDomainUnicodeCollision(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		createUserWithEmail(t, db, "mike123", "test123", "mike@ixample.org")
		createUserWithEmail(t, db, "mike456", "test123", "mike@ıxample.org")
		form := auth.NewPasswordResetForm().Bind(values("email", "mike@ıxample.org"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		djangotest.AssertEqual(t, assertEmailMessageSent(t).To, []string{"mike@ıxample.org"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_user_email_unicode_collision_nonexistent
func TestDjangoPasswordResetFormUserEmailUnicodeCollisionNonexistent(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		createUserWithEmail(t, db, "mike123", "test123", "mike@example.org")
		form := auth.NewPasswordResetForm().Bind(values("email", "mıke@example.org"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		assertNoEmailSent(t)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_user_email_domain_unicode_collision_nonexistent
func TestDjangoPasswordResetFormUserEmailDomainUnicodeCollisionNonexistent(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		createUserWithEmail(t, db, "mike123", "test123", "mike@ixample.org")
		form := auth.NewPasswordResetForm().Bind(values("email", "mike@ıxample.org"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		assertNoEmailSent(t)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_nonexistent_email
//
// Test nonexistent email address. This should not fail because it would
// expose information about registered users.
func TestDjangoPasswordResetFormNonexistentEmail(t *testing.T) {
	resetFormTest(t, func(t *testing.T, _ *orm.DB) {
		form := auth.NewPasswordResetForm().Bind(values("email", "foo@bar.com"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		assertNoEmailSent(t)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_cleaned_data
func TestDjangoPasswordResetFormCleanedData(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		_, email := createDummyUser(t, db)
		form := auth.NewPasswordResetForm().Bind(values("email", email))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		djangotest.AssertEqual(t, form.CleanedData()["email"], any(email))
		assertEmailMessageSent(t)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_custom_email_subject
func TestDjangoPasswordResetFormCustomEmailSubject(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewPasswordResetForm().Bind(values("email", "testclient@example.com"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		djangotest.AssertEqual(t, assertEmailMessageSent(t).Subject, "Custom password reset on example.com")
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_custom_email_constructor
//
// The subclass that overrides send_mail is PasswordResetOptions.SendMail.
func TestDjangoPasswordResetFormCustomEmailConstructor(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewPasswordResetForm().Bind(values("email", "testclient@example.com"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, func(o *auth.PasswordResetOptions) {
			o.SendMail = func(_ context.Context, _ auth.PasswordResetOptions, _ map[string]any, to string) error {
				msg := &mail.Message{
					Subject:      "Forgot your password?",
					Body:         "Sorry to hear you forgot your password.",
					To:           []string{to},
					Bcc:          []string{"site_monitor@example.com"},
					Headers:      []mail.Header{{Name: "Reply-To", Value: "webmaster@example.com"}},
					Alternatives: []mail.Alternative{{Content: "Really sorry to hear you forgot your password.", MIMEType: "text/html"}},
				}
				_, err := msg.Send()
				return err
			}
		})
		msg := assertEmailMessageSent(t)
		djangotest.AssertEqual(t, msg.Subject, "Forgot your password?")
		djangotest.AssertEqual(t, msg.Bcc, []string{"site_monitor@example.com"})
		djangotest.AssertEqual(t, cmp.Or(msg.ContentSubtype, "plain"), "plain")
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_preserve_username_case
//
// Preserve the case of the user name (before the @ in the email address) when
// creating a user (#5605).
func TestDjangoPasswordResetFormPreserveUsernameCase(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		user := createUserWithEmail(t, db, "forms_test2", "test", "tesT@EXAMple.com")
		djangotest.AssertEqual(t, user.Email, "tesT@example.com")
		user = createUserWithEmail(t, db, "forms_test3", "test", "tesT")
		djangotest.AssertEqual(t, user.Email, "tesT")
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_inactive_user
//
// Inactive user cannot receive password reset email.
func TestDjangoPasswordResetFormInactiveUser(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		user, email := createDummyUser(t, db)
		user.IsActive = false
		saveFlags(t, db, user)
		form := auth.NewPasswordResetForm().Bind(values("email", email))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		assertNoEmailSent(t)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_unusable_password
func TestDjangoPasswordResetFormUnusablePassword(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		user := createUserWithEmail(t, db, "testuser", "test", "test@example.com")
		data := values("email", "test@example.com")
		djangotest.AssertEqual(t, auth.NewPasswordResetForm().Bind(data).IsValid(), true)
		must(t, user.SetUnusablePassword())
		_, err := orm.Query[auth.User](db).Filter("id", user.ID).Update(context.Background(), "password", user.Password)
		must(t, err)
		form := auth.NewPasswordResetForm().Bind(data)
		// The form itself is valid, but no email is sent.
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		assertNoEmailSent(t)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_save_plaintext_email
//
// Test the PasswordResetForm.save() method with no html_email_template_name
// parameter passed in.
func TestDjangoPasswordResetFormSavePlaintextEmail(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		_, email := createDummyUser(t, db)
		form := auth.NewPasswordResetForm().Bind(values("email", email))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, nil)
		msg := assertEmailMessageSent(t)
		djangotest.AssertEqual(t, len(msg.Alternatives), 0)
		message := parseMessage(t, msg)
		djangotest.AssertEqual(t, message.mediaType, "text/plain")
		djangotest.AssertEqual(t, message.header.Get("Subject"), "Custom password reset on example.com")
		djangotest.AssertEqual(t, message.header.Get("To"), email)
		if !regexp.MustCompile(`^http://example.com/reset/[\w+/-]`).Match(message.body) {
			t.Errorf("the body %q does not start with the reset link", message.body)
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_save_html_email_template_name
//
// A multipart email is sent with both text/plain and text/html parts.
func TestDjangoPasswordResetFormSaveHTMLEmailTemplateName(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		_, email := createDummyUser(t, db)
		form := auth.NewPasswordResetForm().Bind(values("email", email))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saveResetForm(t, db, form, func(o *auth.PasswordResetOptions) {
			o.HTMLEmailTemplateName = "registration/html_password_reset_email.html"
		})
		msg := assertEmailMessageSent(t)
		djangotest.AssertEqual(t, len(msg.Alternatives), 1)
		message := parseMessage(t, msg)
		djangotest.AssertEqual(t, message.header.Get("Subject"), "Custom password reset on example.com")
		djangotest.AssertEqual(t, strings.HasPrefix(message.mediaType, "multipart/"), true)
		types, contents := message.parts(t)
		djangotest.AssertEqual(t, types, []string{"text/plain", "text/html"})
		djangotest.AssertEqual(t, message.header.Get("To"), email)
		if !regexp.MustCompile(`^http://example.com/reset/[\w/-]+`).MatchString(contents[0]) {
			t.Errorf("the text part %q does not start with the reset link", contents[0])
		}
		if !regexp.MustCompile(`^<html><a href="http://example.com/reset/[\w/-]+/">Link</a></html>\n?$`).MatchString(contents[1]) {
			t.Errorf("the HTML part is %q", contents[1])
		}
	})
}

// failingBackendOnce registers the backend "failing" of the tests, Django's
// mail.custombackend.FailingEmailBackend.
var failingBackendOnce sync.Once

// failingEmailBackend fails to send every message.
type failingEmailBackend struct{}

// Open does nothing.
func (failingEmailBackend) Open() (bool, error) { return false, nil }

// Close does nothing.
func (failingEmailBackend) Close() error { return nil }

// SendMessages fails.
func (failingEmailBackend) SendMessages([]*mail.Message) (int, error) {
	return 0, errors.New("FailingEmailBackend is doomed to fail.") //nolint:revive,staticcheck // Django's message
}

// useFailingEmailBackend makes the mail of the test go to failingEmailBackend.
func useFailingEmailBackend(t *testing.T) {
	t.Helper()
	failingBackendOnce.Do(func() {
		mail.RegisterBackend("failing", func(mail.Options) (mail.Backend, error) { return failingEmailBackend{}, nil })
	})
	cfg := mail.CurrentConfig()
	cfg.Backend = "failing"
	t.Cleanup(mail.Configure(cfg))
}

// captureLogs sends the records of the default logger to a buffer for the
// rest of the test (Django's assertLogs).
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_save_send_email_exceptions_are_catched_and_logged
func TestDjangoPasswordResetFormSaveSendEmailExceptionsAreCatchedAndLogged(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		user, email := createDummyUser(t, db)
		form := auth.NewPasswordResetForm().Bind(values("email", email))
		djangotest.AssertEqual(t, form.IsValid(), true)
		useFailingEmailBackend(t)
		logs := captureLogs(t)
		saveResetForm(t, db, form, nil)
		assertNoEmailSent(t)
		lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
		djangotest.AssertEqual(t, len(lines), 1)
		want := `level=ERROR msg="Failed to send password reset email to ` + strconv.FormatInt(user.ID, 10) +
			`" error="FailingEmailBackend is doomed to fail."`
		if !strings.HasSuffix(lines[0], want) {
			t.Errorf("log line %q does not end with %q", lines[0], want)
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_custom_email_field
func TestDjangoPasswordResetFormCustomEmailField(t *testing.T) {
	t.Skip("needs a custom user model with another email field, which djangogo does not have")
}

// Ported from Django: tests/auth_tests/test_forms.py::PasswordResetFormTest.test_html_autocomplete_attributes
func TestDjangoPasswordResetFormHTMLAutocompleteAttributes(t *testing.T) {
	form := auth.NewPasswordResetForm()
	djangotest.AssertEqual(t, widgetAttrs(t, form.Form, "email")["autocomplete"], any("email"))
}

// No Django test exists: the default templates of the auth package render
// the email, with the link of the password_reset_confirm route, and a line
// break in the subject is removed.
func TestPasswordResetFormDefaultTemplates(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		prev := templates.URLResolverFunc()
		templates.SetURLResolver(func(name string, _ ...any) (string, error) {
			if name != "password_reset_confirm" {
				return "", errors.New("no route " + name)
			}
			return "/accounts/reset/MQ/token/", nil
		})
		t.Cleanup(func() { templates.SetURLResolver(prev) })
		form := auth.NewPasswordResetForm().Bind(values("email", "testclient@example.com"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		must(t, form.Save(context.Background(), db, auth.PasswordResetOptions{DomainOverride: "example.com"}))
		msg := assertEmailMessageSent(t)
		djangotest.AssertEqual(t, msg.Subject, "Password reset on example.com")
		djangotest.AssertEqual(t, strings.Contains(msg.Body, "http://example.com/accounts/reset/MQ/token/\n"), true)
		djangotest.AssertEqual(t, strings.Contains(msg.Body, "In case you’ve forgotten, you are: testclient\n"), true)
	})
}

// No Django test exists: Save without a request needs DomainOverride, where
// Django falls back to the sites app.
func TestPasswordResetFormSaveNeedsDomain(t *testing.T) {
	resetFormTest(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewPasswordResetForm().Bind(values("email", "testclient@example.com"))
		if err := form.Save(context.Background(), db, auth.PasswordResetOptions{}); err == nil {
			t.Error("Save without a request and a domain succeeds, want an error")
		}
		assertNoEmailSent(t)
	})
}
