// Package mail sends email, as Django's django.core.mail does.
//
// A Message builds the same MIME message as Django's EmailMessage and
// EmailMultiAlternatives, byte for byte apart from the Date, the Message-ID
// and the multipart boundaries. The header folding and encoding port the
// email package of Python 3.14 with policy.default, which fixes folding and
// parsing bugs of earlier Python versions.
//
// Backends send the messages:
//
//   - "smtp" sends them to an SMTP server, with STARTTLS or TLS and login.
//   - "console" writes them to standard output.
//   - "file" appends them to a file in a directory.
//   - "locmem" keeps them in the outbox for tests; see Outbox.
//   - "dummy" sends nothing.
//
// Configure sets the settings, such as the backend and the SMTP server.
// SendMail, SendMassMail, MailAdmins and MailManagers are the short forms
// of Django's helpers of the same names.
package mail
