package djangogo_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/mail"
)

// sendtestemailApp returns an application with the ADMINS and MANAGERS of
// Django's SendTestEmailManagementCommand and its mail in the outbox.
func sendtestemailApp(t *testing.T) *djangogo.Application {
	t.Helper()
	app, err := djangogo.New(conf.Settings{
		SecretKey: "k",
		Admins:    []string{"admin@example.com", "admin_and_manager@example.com"},
		Managers:  []string{"manager@example.com", "admin_and_manager@example.com"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	app.Out, app.Err = &bytes.Buffer{}, &bytes.Buffer{}
	djangotest.LocmemMail(t)
	return app
}

// sortedRecipients returns the recipients of m, sorted.
func sortedRecipients(m *mail.Message) []string {
	r := m.Recipients()
	slices.Sort(r)
	return r
}

// TestSendtestemailSingleReceiver checks the subject and recipient of one address.
//
// Ported from Django: tests/mail/test_sendtestemail.py::SendTestEmailManagementCommand.test_single_receiver
func TestSendtestemailSingleReceiver(t *testing.T) {
	app := sendtestemailApp(t)
	if err := app.Execute([]string{"sendtestemail", "joe@example.com"}); err != nil {
		t.Fatal(err)
	}
	outbox := mail.Outbox()
	if len(outbox) != 1 {
		t.Fatalf("outbox has %d messages, want 1", len(outbox))
	}
	if !strings.HasPrefix(outbox[0].Subject, "Test email from") {
		t.Errorf("subject = %q, want it to start with %q", outbox[0].Subject, "Test email from")
	}
	djangotest.AssertEqual(t, outbox[0].Recipients(), []string{"joe@example.com"})
}

// TestSendtestemailMultipleReceivers checks one message to several addresses.
//
// Ported from Django: tests/mail/test_sendtestemail.py::SendTestEmailManagementCommand.test_multiple_receivers
func TestSendtestemailMultipleReceivers(t *testing.T) {
	app := sendtestemailApp(t)
	if err := app.Execute([]string{"sendtestemail", "joe@example.com", "jane@example.com"}); err != nil {
		t.Fatal(err)
	}
	outbox := mail.Outbox()
	if len(outbox) != 1 {
		t.Fatalf("outbox has %d messages, want 1", len(outbox))
	}
	if !strings.HasPrefix(outbox[0].Subject, "Test email from") {
		t.Errorf("subject = %q, want it to start with %q", outbox[0].Subject, "Test email from")
	}
	djangotest.AssertEqual(t, sortedRecipients(outbox[0]), []string{"jane@example.com", "joe@example.com"})
}

// TestSendtestemailMissingReceivers checks the error without recipients or flags.
//
// Ported from Django: tests/mail/test_sendtestemail.py::SendTestEmailManagementCommand.test_missing_receivers
func TestSendtestemailMissingReceivers(t *testing.T) {
	app := sendtestemailApp(t)
	const msg = "You must specify some email recipients, or pass the --managers or --admin options."
	err := app.Execute([]string{"sendtestemail"})
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("error = %v, want one containing %q", err, msg)
	}
}

// TestSendtestemailManagerReceivers checks the message to the managers.
//
// Ported from Django: tests/mail/test_sendtestemail.py::SendTestEmailManagementCommand.test_manager_receivers
func TestSendtestemailManagerReceivers(t *testing.T) {
	app := sendtestemailApp(t)
	if err := app.Execute([]string{"sendtestemail", "--managers"}); err != nil {
		t.Fatal(err)
	}
	outbox := mail.Outbox()
	if len(outbox) != 1 {
		t.Fatalf("outbox has %d messages, want 1", len(outbox))
	}
	djangotest.AssertEqual(t, sortedRecipients(outbox[0]), []string{"admin_and_manager@example.com", "manager@example.com"})
}

// TestSendtestemailAdminReceivers checks the message to the admins.
//
// Ported from Django: tests/mail/test_sendtestemail.py::SendTestEmailManagementCommand.test_admin_receivers
func TestSendtestemailAdminReceivers(t *testing.T) {
	app := sendtestemailApp(t)
	if err := app.Execute([]string{"sendtestemail", "--admins"}); err != nil {
		t.Fatal(err)
	}
	outbox := mail.Outbox()
	if len(outbox) != 1 {
		t.Fatalf("outbox has %d messages, want 1", len(outbox))
	}
	djangotest.AssertEqual(t, sortedRecipients(outbox[0]), []string{"admin@example.com", "admin_and_manager@example.com"})
}

// TestSendtestemailManagerAndAdminReceivers checks one message to the
// managers and then one to the admins.
//
// Ported from Django: tests/mail/test_sendtestemail.py::SendTestEmailManagementCommand.test_manager_and_admin_receivers
func TestSendtestemailManagerAndAdminReceivers(t *testing.T) {
	app := sendtestemailApp(t)
	if err := app.Execute([]string{"sendtestemail", "--managers", "--admins"}); err != nil {
		t.Fatal(err)
	}
	outbox := mail.Outbox()
	if len(outbox) != 2 {
		t.Fatalf("outbox has %d messages, want 2", len(outbox))
	}
	djangotest.AssertEqual(t, sortedRecipients(outbox[0]), []string{"admin_and_manager@example.com", "manager@example.com"})
	djangotest.AssertEqual(t, sortedRecipients(outbox[1]), []string{"admin@example.com", "admin_and_manager@example.com"})
}

// No Django test exists: the dotted path of a built-in backend selects it,
// and an unknown backend stops New.
func TestEmailBackendSetting(t *testing.T) {
	prev := mail.CurrentConfig()
	t.Cleanup(func() { mail.Configure(prev) })
	app, err := djangogo.New(conf.Settings{SecretKey: "k", EmailBackend: "django.core.mail.backends.locmem.EmailBackend"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	djangotest.AssertEqual(t, mail.CurrentConfig().Backend, "locmem")

	_, err = djangogo.New(conf.Settings{SecretKey: "k", EmailBackend: "myapp.backends.Missing"})
	if err == nil || !strings.Contains(err.Error(), `EMAIL_BACKEND "myapp.backends.Missing" is not a known email backend`) {
		t.Errorf("New with an unknown backend: error = %v", err)
	}
}
