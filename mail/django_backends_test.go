package mail

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

// must fails the test on an error.
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// testMailbox reads back what a backend sent, as get_mailbox_content and
// flush_mailbox of Django's backend test classes do.
type testMailbox interface {
	content(t *testing.T) []*part
	flush(t *testing.T)
}

// forEachBackend runs test once for each of the locmem, file, console and
// SMTP backends, with that backend in the settings, as the subclasses of
// BaseEmailBackendTests do.
func forEachBackend(t *testing.T, test func(t *testing.T, mb testMailbox)) {
	for _, name := range []string{"locmem", "file", "console", "smtp"} {
		t.Run(name, func(t *testing.T) {
			withConfig(t, func(c *Config) { c.Backend = name })
			var mb testMailbox
			switch name {
			case "locmem":
				mb = newLocmemMailbox(t)
			case "file":
				mb = newFileMailbox(t)
			case "console":
				mb = newConsoleMailbox(t)
			case "smtp":
				mb = newSMTPMailbox(t)
			}
			test(t, mb)
		})
	}
}

// theMessage returns the one message in the mailbox.
func theMessage(t *testing.T, mb testMailbox) *part {
	t.Helper()
	messages := mb.content(t)
	if len(messages) != 1 {
		t.Fatalf("expected exactly one message, got %d", len(messages))
	}
	return messages[0]
}

// splitSession returns the messages that the console and file backends
// wrote to one stream.
func splitSession(t *testing.T, data []byte) []*part {
	t.Helper()
	var out []*part
	for _, m := range bytes.Split(data, []byte("\n"+separator)) {
		if len(m) > 0 {
			out = append(out, parse(t, m))
		}
	}
	return out
}

// locmemMailbox reads the outbox.
type locmemMailbox struct{}

// newLocmemMailbox empties the outbox now and when the test ends.
func newLocmemMailbox(t *testing.T) locmemMailbox {
	ClearOutbox()
	t.Cleanup(ClearOutbox)
	return locmemMailbox{}
}

// content parses the messages of the outbox.
func (locmemMailbox) content(t *testing.T) []*part {
	t.Helper()
	outbox := Outbox()
	out := make([]*part, 0, len(outbox))
	for _, m := range outbox {
		out = append(out, parse(t, mustBytes(t, m)))
	}
	return out
}

// flush empties the outbox.
func (locmemMailbox) flush(*testing.T) { ClearOutbox() }

// fileMailbox reads the files of the file backend.
type fileMailbox struct{ dir string }

// newFileMailbox makes a new directory the FilePath setting for the test.
func newFileMailbox(t *testing.T) fileMailbox {
	dir := t.TempDir()
	withConfig(t, func(c *Config) { c.FilePath = dir })
	return fileMailbox{dir}
}

// content parses the messages of all files, in file name order.
func (f fileMailbox) content(t *testing.T) []*part {
	t.Helper()
	entries, err := os.ReadDir(f.dir)
	must(t, err)
	out := make([]*part, 0, len(entries))
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(f.dir, e.Name()))
		must(t, err)
		out = append(out, splitSession(t, data)...)
	}
	return out
}

// flush removes the files.
func (f fileMailbox) flush(t *testing.T) {
	entries, err := os.ReadDir(f.dir)
	must(t, err)
	for _, e := range entries {
		must(t, os.Remove(filepath.Join(f.dir, e.Name())))
	}
}

// consoleMailbox reads what the console backend wrote to standard output.
type consoleMailbox struct{ f *os.File }

// newConsoleMailbox points standard output at a file for the test, as
// Django's ConsoleBackendTests replace sys.stdout.
func newConsoleMailbox(t *testing.T) consoleMailbox {
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "stdout"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	must(t, err)
	prev := os.Stdout
	os.Stdout = f
	t.Cleanup(func() {
		os.Stdout = prev
		f.Close()
	})
	return consoleMailbox{f}
}

// content parses the messages written so far.
func (c consoleMailbox) content(t *testing.T) []*part {
	t.Helper()
	data, err := os.ReadFile(c.f.Name())
	must(t, err)
	return splitSession(t, data)
}

// flush empties the file.
func (c consoleMailbox) flush(t *testing.T) { must(t, c.f.Truncate(0)) }

// smtpMailbox reads the messages of a test SMTP server.
type smtpMailbox struct{ s *smtpServer }

// newSMTPMailbox starts a server and makes it the SMTP host of the settings.
func newSMTPMailbox(t *testing.T) smtpMailbox {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	return smtpMailbox{s}
}

// content parses the messages the server received.
func (m smtpMailbox) content(t *testing.T) []*part {
	t.Helper()
	messages := m.s.messages()
	out := make([]*part, 0, len(messages))
	for _, data := range messages {
		out = append(out, parse(t, data))
	}
	return out
}

// flush empties the server's mailbox.
func (m smtpMailbox) flush(*testing.T) { m.s.flush() }

// newConn returns a backend of the settings.
func newConn(t *testing.T, opts ...Option) Backend {
	t.Helper()
	conn, err := NewConnection(opts...)
	must(t, err)
	return conn
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_send
func TestBackendSend(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		m := &Message{Subject: "Subject", Body: "Content\n", From: "from@example.com", To: []string{"to@example.com"}}
		n, err := newConn(t).SendMessages([]*Message{m})
		must(t, err)
		check(t, n, 1)
		p := theMessage(t, mb)
		check(t, p.get("Subject"), "Subject")
		check(t, p.text(t), "Content\n")
		check(t, p.get("From"), "from@example.com")
		check(t, p.getAll("To"), []string{"to@example.com"})
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_send_unicode
func TestBackendSendUnicode(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		m := &Message{Subject: "Chère maman", Body: "Je t'aime très fort\n", From: "from@example.com", To: []string{"to@example.com"}}
		n, err := newConn(t).SendMessages([]*Message{m})
		must(t, err)
		check(t, n, 1)
		p := theMessage(t, mb)
		check(t, p.get("Subject"), "Chère maman")
		check(t, p.text(t), "Je t'aime très fort\n")
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_send_long_lines
//
// RFC 5322 2.1.1 limits lines to 998 octets. A body with a longer line uses
// the shorter of quoted-printable and base64, so no line break changes the
// text.
func TestBackendSendLongLines(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		for _, c := range []struct{ body, cte string }{
			{strings.Repeat("В южных морях ", 60), "base64"},
			{strings.Repeat("I de sørlige hav ", 58), "quoted-printable"},
		} {
			mb.flush(t)
			if utf8.RuneCountInString(c.body) >= 998 || len(c.body) <= 998 {
				t.Fatalf("the body of the %s case does not test long lines", c.cte)
			}
			_, err := (&Message{Body: c.body, To: []string{"to@example.com"}}).Send()
			must(t, err)
			hasHeaders(t, theMessage(t, mb),
				field{"MIME-Version", "1.0"},
				field{"Content-Type", `text/plain; charset="utf-8"`},
				field{"Content-Transfer-Encoding", c.cte},
			)
		}
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_send_many
//
// The case with an iterator is left out: SendMessages takes a slice.
func TestBackendSendMany(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		m1 := &Message{To: []string{"to-1@example.com"}}
		m2 := &Message{To: []string{"to-2@example.com"}}
		n, err := newConn(t).SendMessages([]*Message{m1, m2})
		must(t, err)
		check(t, n, 2)
		messages := mb.content(t)
		check(t, len(messages), 2)
		check(t, messages[0].get("To"), "to-1@example.com")
		check(t, messages[1].get("To"), "to-2@example.com")
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_send_verbose_name
func TestBackendSendVerboseName(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		_, err := (&Message{From: `"Firstname Sürname" <from@example.com>`, To: []string{"to@example.com"}}).Send()
		must(t, err)
		check(t, theMessage(t, mb).get("From"), "Firstname Sürname <from@example.com>")
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_plaintext_send_mail
//
// SendMail without WithHTML sends a single text part.
func TestBackendPlaintextSendMail(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		_, err := SendMail("Subject", "Content\n", "sender@example.com", []string{"nobody@example.com"})
		must(t, err)
		p := theMessage(t, mb)
		check(t, p.get("Subject"), "Subject")
		check(t, p.getAll("To"), []string{"nobody@example.com"})
		check(t, p.isMultipart(), false)
		check(t, p.text(t), "Content\n")
		check(t, p.contentType(), "text/plain")
	})
}

// checkHTMLMessage checks a message with the text "Content\n" and the HTML
// alternative "HTML Content\n".
func checkHTMLMessage(t *testing.T, p *part, subject, to string) {
	t.Helper()
	check(t, p.get("Subject"), subject)
	check(t, p.getAll("To"), []string{to})
	check(t, p.isMultipart(), true)
	if len(p.parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(p.parts))
	}
	check(t, p.parts[0].text(t), "Content\n")
	check(t, p.parts[0].contentType(), "text/plain")
	check(t, p.parts[1].text(t), "HTML Content\n")
	check(t, p.parts[1].contentType(), "text/html")
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_html_send_mail
func TestBackendHTMLSendMail(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		_, err := SendMail("Subject", "Content\n", "sender@example.com", []string{"nobody@example.com"}, WithHTML("HTML Content\n"))
		must(t, err)
		checkHTMLMessage(t, theMessage(t, mb), "Subject", "nobody@example.com")
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_mail_admins_and_managers
//
// The cases with tuples and lazy strings are left out: the settings are
// []string.
func TestBackendMailAdminsAndManagers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		for _, setting := range []string{"Admins", "Managers"} {
			for _, value := range [][]string{
				{`"Name, Full" <test@example.com>`},
				{"test@example.com", "other@example.com"},
			} {
				t.Run(fmt.Sprintf("%s %q", setting, value), func(t *testing.T) {
					mb.flush(t)
					send := MailAdmins
					withConfig(t, func(c *Config) { c.Admins = value })
					if setting == "Managers" {
						send = MailManagers
						withConfig(t, func(c *Config) { c.Admins, c.Managers = nil, value })
					}
					must(t, send("subject", "content"))
					check(t, theMessage(t, mb).getAll("To"), []string{strings.Join(value, ", ")})
				})
			}
		}
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_html_mail_managers
func TestBackendHTMLMailManagers(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		withConfig(t, func(c *Config) { c.Managers = []string{"nobody@example.com"} })
		must(t, MailManagers("Subject", "Content\n", WithHTML("HTML Content\n")))
		checkHTMLMessage(t, theMessage(t, mb), "[Django] Subject", "nobody@example.com")
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_html_mail_admins
func TestBackendHTMLMailAdmins(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		withConfig(t, func(c *Config) { c.Admins = []string{"nobody@example.com"} })
		must(t, MailAdmins("Subject", "Content\n", WithHTML("HTML Content\n")))
		checkHTMLMessage(t, theMessage(t, mb), "[Django] Subject", "nobody@example.com")
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_manager_and_admin_mail_prefix
//
// The subject starts with the prefix (#13494). Django passes a lazy
// subject; Go has no lazy strings.
func TestBackendManagerAndAdminMailPrefix(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		withConfig(t, func(c *Config) {
			c.Admins = []string{"nobody+admin@example.com"}
			c.Managers = []string{"nobody+manager@example.com"}
		})
		for _, send := range []func(string, string, ...Option) error{MailManagers, MailAdmins} {
			must(t, send("Subject", "Content"))
			check(t, theMessage(t, mb).get("Subject"), "[Django] Subject")
			mb.flush(t)
		}
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_empty_admins
//
// MailAdmins and MailManagers do not connect to the server without
// recipients (#9383).
func TestBackendEmptyAdmins(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		withConfig(t, func(c *Config) { c.Admins, c.Managers = nil, nil })
		for _, send := range []func(string, string, ...Option) error{MailManagers, MailAdmins} {
			must(t, send("hi", "there"))
			check(t, len(mb.content(t)), 0)
		}
	})
}

// No Go counterpart: tests/mail/tests.py::BaseEmailBackendTests.test_deprecated_admins_managers_tuples
// checks the deprecated (name, address) pairs of ADMINS and MANAGERS. The
// settings are []string.

// No Go counterpart: tests/mail/tests.py::BaseEmailBackendTests.test_wrong_admins_managers
// checks the error for ADMINS and MANAGERS values that are no list of
// strings. The settings are []string.

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_message_cc_header
//
// The Cc header reaches the backend (#7722).
func TestBackendMessageCcHeader(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		m := &Message{Subject: "Subject", Body: "Content", From: "from@example.com", To: []string{"to@example.com"}, Cc: []string{"cc@example.com"}}
		_, err := newConn(t).SendMessages([]*Message{m})
		must(t, err)
		p := theMessage(t, mb)
		hasHeaders(t, p,
			field{"MIME-Version", "1.0"},
			field{"Content-Type", `text/plain; charset="utf-8"`},
			field{"Content-Transfer-Encoding", "7bit"},
			field{"Subject", "Subject"},
			field{"From", "from@example.com"},
			field{"To", "to@example.com"},
			field{"Cc", "cc@example.com"},
		)
		check(t, p.has("Date"), true)
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_idn_send
//
// Non-ASCII domains are sent IDNA encoded (#14301).
func TestBackendIDNSend(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		n, err := SendMail("Subject", "Content", "from@öäü.com", []string{"to@öäü.com"})
		must(t, err)
		check(t, n, 1)
		p := theMessage(t, mb)
		check(t, p.get("From"), "from@xn--4ca9at.com")
		check(t, p.get("To"), "to@xn--4ca9at.com")

		mb.flush(t)
		_, err = (&Message{From: "from@öäü.com", To: []string{"to@öäü.com"}, Cc: []string{"cc@öäü.com"}}).Send()
		must(t, err)
		p = theMessage(t, mb)
		check(t, p.get("From"), "from@xn--4ca9at.com")
		check(t, p.get("To"), "to@xn--4ca9at.com")
		check(t, p.get("Cc"), "cc@xn--4ca9at.com")
	})
}

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_recipient_without_domain
//
// Addresses without a domain are sent (#15042).
func TestBackendRecipientWithoutDomain(t *testing.T) {
	forEachBackend(t, func(t *testing.T, mb testMailbox) {
		n, err := SendMail("Subject", "Content", "tester", []string{"django"})
		must(t, err)
		check(t, n, 1)
		p := theMessage(t, mb)
		check(t, p.get("From"), "tester")
		check(t, p.get("To"), "django")
	})
}

// No Go counterpart: tests/mail/tests.py::BaseEmailBackendTests.test_lazy_addresses
// passes lazy translation strings. Go has no lazy strings.

// Ported from Django: tests/mail/tests.py::BaseEmailBackendTests.test_close_connection
//
// A connection that was never opened can be closed.
func TestBackendCloseConnection(t *testing.T) {
	forEachBackend(t, func(t *testing.T, _ testMailbox) {
		must(t, newConn(t, WithUsername(""), WithPassword("")).Close())
	})
}

// No Go counterpart: tests/mail/tests.py::BaseEmailBackendTests.test_use_as_contextmanager
// uses the backend as a context manager. In Go, Open the backend and defer
// Close.

// Ported from Django: tests/mail/tests.py::LocmemBackendTests.test_locmem_shared_messages
//
// All locmem backends share one outbox.
func TestLocmemSharedMessages(t *testing.T) {
	newLocmemMailbox(t)
	m := &Message{To: []string{"to@example.com"}}
	_, err := NewLocmemBackend(Options{}).SendMessages([]*Message{m})
	must(t, err)
	_, err = NewLocmemBackend(Options{}).SendMessages([]*Message{m})
	must(t, err)
	check(t, len(Outbox()), 2)
}

// Ported from Django: tests/mail/tests.py::LocmemBackendTests.test_validate_multiline_headers
//
// The locmem backend checks the headers as the other backends do (#18861).
func TestLocmemValidateMultilineHeaders(t *testing.T) {
	newLocmemMailbox(t)
	_, err := SendMail("Subject\nMultiline", "Content", "from@example.com", []string{"to@example.com"})
	if err == nil {
		t.Fatal("a subject with a line break was sent")
	}
}

// Ported from Django: tests/mail/tests.py::LocmemBackendTests.test_outbox_not_mutated_after_send
func TestLocmemOutboxNotMutatedAfterSend(t *testing.T) {
	newLocmemMailbox(t)
	m := &Message{Subject: "correct subject", To: []string{"to@example.com"}}
	_, err := m.Send()
	must(t, err)
	m.Subject = "other subject"
	m.To = append(m.To, "other@example.com")
	check(t, Outbox()[0].Subject, "correct subject")
	check(t, Outbox()[0].To, []string{"to@example.com"})
}

// Ported from Django: tests/mail/tests.py::FileBackendTests.test_file_sessions
//
// Each backend writes its own file.
func TestFileSessions(t *testing.T) {
	withConfig(t, func(c *Config) { c.Backend = "file" })
	mb := newFileMailbox(t)
	count := func() int {
		entries, err := os.ReadDir(mb.dir)
		must(t, err)
		return len(entries)
	}
	m := &Message{
		Subject: "Subject", Body: "Content", From: "bounce@example.com", To: []string{"to@example.com"},
		Headers: []Header{{"From", "from@example.com"}},
	}
	conn := newConn(t)
	_, err := conn.SendMessages([]*Message{m})
	must(t, err)
	check(t, count(), 1)
	entries, err := os.ReadDir(mb.dir)
	must(t, err)
	data, err := os.ReadFile(filepath.Join(mb.dir, entries[0].Name()))
	must(t, err)
	p := parse(t, data)
	check(t, p.contentType(), "text/plain")
	check(t, p.get("Subject"), "Subject")
	check(t, p.get("From"), "from@example.com")
	check(t, p.get("To"), "to@example.com")

	_, err = newConn(t).SendMessages([]*Message{m})
	must(t, err)
	check(t, count(), 2)

	_, err = conn.SendMessages([]*Message{m})
	must(t, err)
	check(t, count(), 2)

	m.Connection = newConn(t)
	opened, err := conn.Open()
	must(t, err)
	check(t, opened, true)
	_, err = m.Send()
	must(t, err)
	check(t, count(), 3)
	_, err = m.Send()
	must(t, err)
	check(t, count(), 3)

	must(t, conn.Close())
}

// No Go counterpart: tests/mail/tests.py::FileBackendPathLibTests repeats
// FileBackendTests with a pathlib.Path. FilePath is a string.

// Ported from Django: tests/mail/tests.py::ConsoleBackendTests.test_console_stream_kwarg
//
// The console backend writes to the stream of WithStream.
func TestConsoleStreamKwarg(t *testing.T) {
	var buf bytes.Buffer
	conn := newConn(t, WithBackend("console"), WithStream(&buf))
	_, err := SendMail("Subject", "Content", "from@example.com", []string{"to@example.com"}, WithConnection(conn))
	must(t, err)
	message, _, _ := strings.Cut(buf.String(), "\n"+separator)
	hasHeaders(t, parse(t, []byte(message)),
		field{"MIME-Version", "1.0"},
		field{"Content-Type", `text/plain; charset="utf-8"`},
		field{"Content-Transfer-Encoding", "7bit"},
		field{"Subject", "Subject"},
		field{"From", "from@example.com"},
		field{"To", "to@example.com"},
	)
	if !strings.Contains(message, "\nDate: ") {
		t.Errorf("the message has no Date header:\n%s", message)
	}
}

// smtpBackend returns an SMTP backend built from the settings and opts.
func smtpBackend(t *testing.T, opts ...Option) *SMTPBackend {
	t.Helper()
	return newConn(t, append(opts, WithBackend("smtp"))...).(*SMTPBackend)
}

// fakeConnection stands in for an open connection that the test does not
// use, as Django's tests set backend.connection to a mock.
func fakeConnection() *smtpClient { return &smtpClient{} }

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_authentication_use_settings
func TestSMTPEmailAuthenticationUseSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.HostUser, c.HostPassword = "not empty username", "not empty password" })
	b := smtpBackend(t)
	check(t, b.Options().Username, "not empty username")
	check(t, b.Options().Password, "not empty password")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_authentication_override_settings
func TestSMTPEmailAuthenticationOverrideSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.HostUser, c.HostPassword = "not empty username", "not empty password" })
	b := smtpBackend(t, WithUsername("username"), WithPassword("password"))
	check(t, b.Options().Username, "username")
	check(t, b.Options().Password, "password")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_disabled_authentication
func TestSMTPEmailDisabledAuthentication(t *testing.T) {
	withConfig(t, func(c *Config) { c.HostUser, c.HostPassword = "not empty username", "not empty password" })
	b := smtpBackend(t, WithUsername(""), WithPassword(""))
	check(t, b.Options().Username, "")
	check(t, b.Options().Password, "")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_auth_attempted
//
// Open logs in when the user name and password are set.
func TestSMTPAuthAttempted(t *testing.T) {
	useSMTPServer(t, startSMTPServer(t))
	b := smtpBackend(t, WithUsername("not empty username"), WithPassword("not empty password"))
	_, err := b.Open()
	errorContains(t, err, "SMTP AUTH extension not supported by server.")
	must(t, b.Close())
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_server_open
//
// Open reports whether it opened a connection.
func TestSMTPServerOpen(t *testing.T) {
	useSMTPServer(t, startSMTPServer(t))
	b := smtpBackend(t, WithUsername(""), WithPassword(""))
	check(t, b.Connected(), false)
	opened, err := b.Open()
	must(t, err)
	must(t, b.Close())
	check(t, opened, true)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_reopen_connection
func TestSMTPReopenConnection(t *testing.T) {
	b := smtpBackend(t)
	b.conn = fakeConnection()
	opened, err := b.Open()
	must(t, err)
	check(t, opened, false)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_tls_use_settings
func TestSMTPEmailTLSUseSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.UseTLS = true })
	check(t, smtpBackend(t).Options().UseTLS, true)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_tls_override_settings
func TestSMTPEmailTLSOverrideSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.UseTLS = true })
	check(t, smtpBackend(t, WithTLS(false)).Options().UseTLS, false)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_tls_default_disabled
func TestSMTPEmailTLSDefaultDisabled(t *testing.T) {
	check(t, smtpBackend(t).Options().UseTLS, false)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_ssl_tls_mutually_exclusive
func TestSMTPSSLTLSMutuallyExclusive(t *testing.T) {
	_, err := NewConnection(WithBackend("smtp"), WithSSL(true), WithTLS(true))
	errorContains(t, err, "EMAIL_USE_TLS/EMAIL_USE_SSL are mutually exclusive, so only set one of those settings to True.")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_use_settings
func TestSMTPEmailSSLUseSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.UseSSL = true })
	check(t, smtpBackend(t).Options().UseSSL, true)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_override_settings
func TestSMTPEmailSSLOverrideSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.UseSSL = true })
	check(t, smtpBackend(t, WithSSL(false)).Options().UseSSL, false)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_default_disabled
func TestSMTPEmailSSLDefaultDisabled(t *testing.T) {
	check(t, smtpBackend(t).Options().UseSSL, false)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_certfile_use_settings
func TestSMTPEmailSSLCertfileUseSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.SSLCertfile = "foo" })
	check(t, smtpBackend(t).Options().SSLCertfile, "foo")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_certfile_override_settings
func TestSMTPEmailSSLCertfileOverrideSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.SSLCertfile = "foo" })
	check(t, smtpBackend(t, WithSSLCertfile("bar")).Options().SSLCertfile, "bar")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_certfile_default_disabled
func TestSMTPEmailSSLCertfileDefaultDisabled(t *testing.T) {
	check(t, smtpBackend(t).Options().SSLCertfile, "")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_keyfile_use_settings
func TestSMTPEmailSSLKeyfileUseSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.SSLKeyfile = "foo" })
	check(t, smtpBackend(t).Options().SSLKeyfile, "foo")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_keyfile_override_settings
func TestSMTPEmailSSLKeyfileOverrideSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.SSLKeyfile = "foo" })
	check(t, smtpBackend(t, WithSSLKeyfile("bar")).Options().SSLKeyfile, "bar")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_keyfile_default_disabled
func TestSMTPEmailSSLKeyfileDefaultDisabled(t *testing.T) {
	check(t, smtpBackend(t).Options().SSLKeyfile, "")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_tls_attempts_starttls
func TestSMTPEmailTLSAttemptsStartTLS(t *testing.T) {
	useSMTPServer(t, startSMTPServer(t))
	withConfig(t, func(c *Config) { c.UseTLS = true })
	b := smtpBackend(t)
	check(t, b.Options().UseTLS, true)
	_, err := b.Open()
	errorContains(t, err, "STARTTLS extension not supported by server.")
	must(t, b.Close())
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_ssl_attempts_ssl_connection
func TestSMTPEmailSSLAttemptsSSLConnection(t *testing.T) {
	useSMTPServer(t, startSMTPServer(t))
	withConfig(t, func(c *Config) { c.UseSSL = true })
	b := smtpBackend(t)
	check(t, b.Options().UseSSL, true)
	_, err := b.Open()
	var rh tls.RecordHeaderError
	if !errors.As(err, &rh) {
		t.Errorf("error %v, want a TLS record error", err)
	}
	must(t, b.Close())
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_connection_timeout_default
func TestSMTPConnectionTimeoutDefault(t *testing.T) {
	check(t, smtpBackend(t).Options().Timeout, time.Duration(0))
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_connection_timeout_custom
//
// The connection uses the timeout of the backend.
func TestSMTPConnectionTimeoutCustom(t *testing.T) {
	useSMTPServer(t, startSMTPServer(t))
	b := smtpBackend(t, WithTimeout(42*time.Second))
	_, err := b.Open()
	must(t, err)
	check(t, b.Options().Timeout, 42*time.Second)
	check(t, b.conn.timeout, 42*time.Second)
	must(t, b.Close())
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_timeout_override_settings
func TestSMTPEmailTimeoutOverrideSettings(t *testing.T) {
	withConfig(t, func(c *Config) { c.Timeout = 10 * time.Second })
	check(t, smtpBackend(t).Options().Timeout, 10*time.Second)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_email_msg_uses_crlf
//
// The message is sent with CRLF line endings only (#23063).
func TestSMTPEmailMsgUsesCRLF(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	m := &Message{Subject: "Subject", Body: "Content", From: "from@example.com", To: []string{"to@example.com"}}
	_, err := newConn(t, WithBackend("smtp")).SendMessages([]*Message{m})
	must(t, err)
	s.mu.Lock()
	data := slices.Clone(s.data)
	s.mu.Unlock()
	if len(data) != 1 {
		t.Fatalf("got %d messages, want 1", len(data))
	}
	rest := strings.ReplaceAll(string(data[0]), "\r\n", "")
	if strings.ContainsAny(rest, "\r\n") {
		t.Errorf("the message has bare CR or LF:\n%q", data[0])
	}
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_send_messages_after_open_failed
//
// SendMessages sends nothing when the connection failed with FailSilently.
// Django replaces open() to fail after the connection is made; here a login
// to a server without AUTH fails the same way.
func TestSMTPSendMessagesAfterOpenFailed(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	b := smtpBackend(t, FailSilently(), WithUsername("user"), WithPassword("password"))
	n, err := b.SendMessages([]*Message{{To: []string{"to@example.com"}}})
	must(t, err)
	check(t, n, 0)
	check(t, len(s.messages()), 0)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_send_messages_empty_list
func TestSMTPSendMessagesEmptyList(t *testing.T) {
	b := smtpBackend(t)
	b.conn = fakeConnection()
	n, err := b.SendMessages(nil)
	must(t, err)
	check(t, n, 0)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_send_messages_zero_sent
//
// A message without recipients is not sent.
func TestSMTPSendMessagesZeroSent(t *testing.T) {
	b := smtpBackend(t)
	b.conn = fakeConnection()
	n, err := b.SendMessages([]*Message{{Subject: "Subject", Body: "Content", From: "from@example.com"}})
	must(t, err)
	check(t, n, 0)
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_avoids_sending_to_invalid_addresses
//
// Invalid addresses do not reach the SMTP commands through Recipients,
// which is apart from the header fields. Bcc is only read by the SMTP
// backend, so the error comes from there.
func TestSMTPAvoidsSendingToInvalidAddresses(t *testing.T) {
	b := smtpBackend(t)
	b.conn = fakeConnection()
	for _, addr := range []string{
		// Two @ signs.
		"to@other.com@example.com",
		// No quotes.
		"to@other.com <to@example.com>",
		// Two mailboxes in one address.
		"to@example.com, other@example.com",
		"@",
		"to@",
		"@example.com",
		// CR or LF in the addr-spec.
		"\"evil@example.com\r\nto\"@example.com",
		"to\nevil@example.com",
	} {
		_, err := b.SendMessages([]*Message{{Bcc: []string{addr}}})
		var ie *inputError
		if !errors.As(err, &ie) || !strings.Contains(err.Error(), "Invalid address") {
			t.Errorf("%q: error %v, want an invalid address error", addr, err)
		}
	}
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_encodes_idna_in_smtp_commands
//
// The envelope, which differs from the headers, has non-ASCII domains IDNA
// encoded.
func TestSMTPEncodesIDNAInSMTPCommands(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	m := &Message{
		From: "lists@discussão.example.org",
		To:   []string{"To Example <to@漢字.example.com>"},
		Bcc:  []string{"monitor@discussão.example.org"},
		Headers: []Header{
			{"From", "Gestor de listas <lists@discussão.example.org>"},
			{"To", "Discussão Django <django@discussão.example.org>"},
		},
	}
	_, err := smtpBackend(t).SendMessages([]*Message{m})
	must(t, err)
	envelopes := s.receivedEnvelopes()
	if len(envelopes) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(envelopes))
	}
	check(t, envelopes[0].mailFrom, "lists@xn--discusso-xza.example.org")
	check(t, envelopes[0].rcptTos, []string{"to@xn--p8s937b.example.com", "monitor@xn--discusso-xza.example.org"})
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_does_not_reencode_idna
//
// IDNA 2008 domains that differ in IDNA 2003 stay as they are
// (https://www.unicode.org/reports/tr46/#Deviations).
func TestSMTPDoesNotReencodeIDNA(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	m := &Message{
		From: `"βόλος" <from@xn--fa-hia.example.com>`,
		To:   []string{`"faß" <to@xn--10cl1a0b660p.example.com>`},
		Cc:   []string{"\"ශ්\u200dරී\" <cc@xn--nxasmm1c.example.com>"},
		Bcc:  []string{"\"نامه\u200cای.\" <bcc@xn--mgba3gch31f060k.example.com>"},
	}
	_, err := smtpBackend(t).SendMessages([]*Message{m})
	must(t, err)
	envelopes := s.receivedEnvelopes()
	if len(envelopes) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(envelopes))
	}
	check(t, envelopes[0].mailFrom, "from@xn--fa-hia.example.com")
	check(t, envelopes[0].rcptTos, []string{
		"to@xn--10cl1a0b660p.example.com",
		"cc@xn--nxasmm1c.example.com",
		"bcc@xn--mgba3gch31f060k.example.com",
	})
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_rejects_non_ascii_local_part
//
// A non-ASCII local part needs the SMTPUTF8 extension of RFC 6532, which
// the backend does not use (#35713).
func TestSMTPRejectsNonASCIILocalPart(t *testing.T) {
	b := smtpBackend(t)
	b.conn = fakeConnection()
	_, err := b.SendMessages([]*Message{{To: []string{"nø@example.dk"}}})
	errorContains(t, err, "Invalid address 'nø@example.dk': local-part contains non-ASCII characters")
}

// Ported from Django: tests/mail/tests.py::SMTPBackendTests.test_prep_address_without_force_ascii
func TestSMTPPrepAddressWithoutForceASCII(t *testing.T) {
	for _, c := range []string{"åh@example.dk", "oh@åh.example.dk", "åh@åh.example.dk"} {
		got, err := PrepAddress(c, false)
		must(t, err)
		check(t, got, c)
	}
}

// Ported from Django: tests/mail/tests.py::SMTPBackendStoppedServerTests.test_server_stopped
//
// Close does not fail while the server is stopped.
func TestSMTPServerStopped(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	b := smtpBackend(t, WithUsername(""), WithPassword(""))
	s.stop()
	must(t, b.Close())
}

// Ported from Django: tests/mail/tests.py::SMTPBackendStoppedServerTests.test_fail_silently_on_connection_error
func TestSMTPFailSilentlyOnConnectionError(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	b := smtpBackend(t, WithUsername(""), WithPassword(""))
	s.stop()
	if _, err := b.Open(); !errors.Is(err, syscall.ECONNREFUSED) {
		t.Errorf("error %v, want connection refused", err)
	}
	b.opts.FailSilently = true
	_, err := b.Open()
	must(t, err)
}

// No Go counterpart: tests/mail/tests.py::LegacyAPINotUsedTests.test_no_legacy_apis_imported
// checks which Python email modules Django imports.

// No Django test exists: the AUTH mechanisms of smtplib, which Django's
// tests cannot reach because aiosmtpd offers AUTH only over TLS.
func TestSMTPLogin(t *testing.T) {
	for _, mechanism := range []string{"CRAM-MD5", "PLAIN", "LOGIN"} {
		t.Run(mechanism, func(t *testing.T) {
			s := startSMTPServer(t, func(s *smtpServer) {
				s.auth, s.user, s.password = []string{mechanism}, "user", "secret"
			})
			useSMTPServer(t, s)
			m := &Message{From: "from@example.com", To: []string{"to@example.com"}}
			n, err := smtpBackend(t, WithUsername("user"), WithPassword("secret")).SendMessages([]*Message{m})
			must(t, err)
			check(t, n, 1)
			s.mu.Lock()
			check(t, s.logins, []string{mechanism})
			s.mu.Unlock()
		})
	}
}

// No Django test exists: a wrong password is an SMTPAuthenticationError
// with the reply of the server.
func TestSMTPLoginWrongPassword(t *testing.T) {
	s := startSMTPServer(t, func(s *smtpServer) {
		s.auth, s.user, s.password = []string{"PLAIN", "LOGIN"}, "user", "secret"
	})
	useSMTPServer(t, s)
	_, err := smtpBackend(t, WithUsername("user"), WithPassword("wrong")).Open()
	var se *SMTPError
	if !errors.As(err, &se) || se.Kind != "SMTPAuthenticationError" || se.Code != 535 {
		t.Errorf("error %v, want SMTPAuthenticationError 535", err)
	}
}

// No Django test exists: messages go over STARTTLS and over TLS from the
// start, which Django's tests only see fail.
func TestSMTPTLS(t *testing.T) {
	cert, pool := testCertificate(t)
	for _, c := range []struct {
		name     string
		implicit bool
		opt      Option
	}{
		{"STARTTLS", false, WithTLS(true)},
		{"SSL", true, WithSSL(true)},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := startSMTPServer(t, func(s *smtpServer) {
				s.tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
				s.implicitTLS = c.implicit
			})
			useSMTPServer(t, s)
			b := smtpBackend(t, c.opt)
			b.tlsConfig = &tls.Config{ServerName: "127.0.0.1", RootCAs: pool}
			m := &Message{Subject: "Secure", From: "from@example.com", To: []string{"to@example.com"}}
			n, err := b.SendMessages([]*Message{m})
			must(t, err)
			check(t, n, 1)
			messages := s.messages()
			check(t, len(messages), 1)
			check(t, parse(t, messages[0]).get("Subject"), "Secure")
		})
	}
}

// No Django test exists: the backend checks the certificate of the server
// against the system roots.
func TestSMTPStartTLSUntrustedCertificate(t *testing.T) {
	cert, _ := testCertificate(t)
	s := startSMTPServer(t, func(s *smtpServer) {
		s.tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	})
	useSMTPServer(t, s)
	_, err := smtpBackend(t, WithTLS(true)).Open()
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("error %v, want a certificate error", err)
	}
}

// No Django test exists: a refused message is an SMTPDataError, which
// FailSilently skips.
func TestSMTPRefusedMessage(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	// The test server refuses a sender that differs from the From header.
	m := &Message{From: "from@example.com", To: []string{"to@example.com"}, Headers: []Header{{"From", "other@example.com"}}}
	_, err := smtpBackend(t).SendMessages([]*Message{m})
	var se *SMTPError
	if !errors.As(err, &se) || se.Kind != "SMTPDataError" || se.Code != 553 {
		t.Errorf("error %v, want SMTPDataError 553", err)
	}
	n, err := smtpBackend(t, FailSilently()).SendMessages([]*Message{m})
	must(t, err)
	check(t, n, 0)
}

// No Django test exists: lines that start with a dot arrive as they were
// written.
func TestSMTPDotStuffing(t *testing.T) {
	s := startSMTPServer(t)
	useSMTPServer(t, s)
	body := ".hidden\n..two dots\n.\nend\n"
	m := &Message{Body: body, From: "from@example.com", To: []string{"to@example.com"}}
	_, err := smtpBackend(t).SendMessages([]*Message{m})
	must(t, err)
	messages := s.messages()
	check(t, len(messages), 1)
	check(t, parse(t, messages[0]).text(t), body)
}
