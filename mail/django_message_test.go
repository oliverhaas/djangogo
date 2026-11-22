package mail

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// check fails the test when got and want differ.
func check(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// errorContains fails the test unless err holds msg.
func errorContains(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Errorf("error %v, want one containing %q", err, msg)
	}
}

// customBackend keeps the messages it sends, as the EmailBackend of
// Django's tests/mail/custombackend.py.
type customBackend struct {
	DummyBackend
	testOutbox []*Message
}

// SendMessages keeps the messages.
func (b *customBackend) SendMessages(messages []*Message) (int, error) {
	b.testOutbox = append(b.testOutbox, messages...)
	return len(messages), nil
}

// newCustomBackend returns a connection of the "custom" backend.
func newCustomBackend(t *testing.T) *customBackend {
	t.Helper()
	RegisterBackend("custom", func(Options) (Backend, error) { return &customBackend{}, nil })
	conn, err := NewConnection(WithBackend("custom"))
	if err != nil {
		t.Fatal(err)
	}
	return conn.(*customBackend)
}

// Ported from Django: tests/mail/tests.py::MailTests.test_ascii
func TestMailASCII(t *testing.T) {
	m := &Message{Subject: "Subject", Body: "Content\n", From: "from@example.com", To: []string{"to@example.com"}}
	p := parse(t, mustBytes(t, m))
	check(t, p.get("Subject"), "Subject")
	check(t, p.payload(), "Content\n")
	check(t, p.get("From"), "from@example.com")
	check(t, p.get("To"), "to@example.com")
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_nonascii_as_string_with_ascii_charset
// tests the deprecated SafeMIMEText class of Python's legacy email API.

// Ported from Django: tests/mail/tests.py::MailTests.test_multiple_recipients
func TestMailMultipleRecipients(t *testing.T) {
	m := &Message{Subject: "Subject", Body: "Content\n", From: "from@example.com", To: []string{"to@example.com", "other@example.com"}}
	p := parse(t, mustBytes(t, m))
	check(t, p.get("Subject"), "Subject")
	check(t, p.payload(), "Content\n")
	check(t, p.get("From"), "from@example.com")
	check(t, p.get("To"), "to@example.com, other@example.com")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_header_omitted_for_no_to_recipients
func TestMailHeaderOmittedForNoToRecipients(t *testing.T) {
	m := &Message{Subject: "Subject", Body: "Content", From: "from@example.com", Cc: []string{"cc@example.com"}}
	if parse(t, mustBytes(t, m)).has("To") {
		t.Error("the message has a To header")
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_recipients_with_empty_strings
//
// Empty strings in the recipient fields are left out of the recipients.
func TestMailRecipientsWithEmptyStrings(t *testing.T) {
	m := &Message{
		Subject: "Subject", Body: "Content", From: "from@example.com",
		To: []string{"to@example.com", ""}, Cc: []string{"cc@example.com", ""},
		Bcc: []string{"", "bcc@example.com"}, ReplyTo: []string{"", ""},
	}
	check(t, m.Recipients(), []string{"to@example.com", "cc@example.com", "bcc@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_cc
func TestMailCc(t *testing.T) {
	m := &Message{Subject: "Subject", Body: "Content", From: "from@example.com", To: []string{"to@example.com"}, Cc: []string{"cc@example.com"}}
	check(t, parse(t, mustBytes(t, m)).get("Cc"), "cc@example.com")
	check(t, m.Recipients(), []string{"to@example.com", "cc@example.com"})

	// Multiple Cc with multiple To.
	m = &Message{
		Subject: "Subject", Body: "Content", From: "from@example.com",
		To: []string{"to@example.com", "other@example.com"}, Cc: []string{"cc@example.com", "cc.other@example.com"},
	}
	check(t, parse(t, mustBytes(t, m)).get("Cc"), "cc@example.com, cc.other@example.com")
	check(t, m.Recipients(), []string{"to@example.com", "other@example.com", "cc@example.com", "cc.other@example.com"})

	// With Bcc.
	m.Bcc = []string{"bcc@example.com"}
	check(t, parse(t, mustBytes(t, m)).get("Cc"), "cc@example.com, cc.other@example.com")
	check(t, m.Recipients(), []string{"to@example.com", "other@example.com", "cc@example.com", "cc.other@example.com", "bcc@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_cc_headers
func TestMailCcHeaders(t *testing.T) {
	m := &Message{
		Subject: "Subject", Body: "Content", From: "bounce@example.com", To: []string{"to@example.com"},
		Cc: []string{"foo@example.com"}, Headers: []Header{{"Cc", "override@example.com"}},
	}
	check(t, parse(t, mustBytes(t, m)).getAll("Cc"), []string{"override@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_cc_in_headers_only
func TestMailCcInHeadersOnly(t *testing.T) {
	m := &Message{
		Subject: "Subject", Body: "Content", From: "bounce@example.com", To: []string{"to@example.com"},
		Headers: []Header{{"Cc", "foo@example.com"}},
	}
	check(t, parse(t, mustBytes(t, m)).getAll("Cc"), []string{"foo@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_bcc_not_in_headers
//
// A Bcc address is in the recipients but not in the headers.
func TestMailBccNotInHeaders(t *testing.T) {
	m := &Message{To: []string{"to@example.com"}, Bcc: []string{"bcc@example.com"}}
	data := mustBytes(t, m)
	if parse(t, data).has("Bcc") {
		t.Error("the message has a Bcc header")
	}
	if bytes.Contains(data, []byte("bcc@example.com")) {
		t.Error("the message contains the Bcc address")
	}
	check(t, m.Recipients(), []string{"to@example.com", "bcc@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_reply_to
func TestMailReplyTo(t *testing.T) {
	m := &Message{Subject: "Subject", Body: "Content", From: "from@example.com", To: []string{"to@example.com"}, ReplyTo: []string{"reply_to@example.com"}}
	check(t, parse(t, mustBytes(t, m)).get("Reply-To"), "reply_to@example.com")
	m.ReplyTo = []string{"reply_to1@example.com", "reply_to2@example.com"}
	check(t, parse(t, mustBytes(t, m)).get("Reply-To"), "reply_to1@example.com, reply_to2@example.com")
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_recipients_as_tuple
// passes tuples instead of lists. The recipient fields are []string.

// No Go counterpart: tests/mail/tests.py::MailTests.test_recipients_as_string
// checks the TypeError for a str recipient argument. The recipient fields
// are []string.

// Ported from Django: tests/mail/tests.py::MailTests.test_header_injection
//
// The case with a lazy subject is left out: Go has no lazy strings.
func TestMailHeaderInjection(t *testing.T) {
	msg := "Header values may not contain linefeed or carriage return characters"
	for _, m := range []*Message{
		{Subject: "Subject\nInjection Test"},
		{To: []string{"Name\nInjection test <to@example.com>"}},
	} {
		_, err := m.Bytes()
		errorContains(t, err, msg)
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_folding_white_space
//
// Long headers fold at white space (#7747).
func TestMailFoldingWhiteSpace(t *testing.T) {
	m := &Message{Subject: "Long subject lines that get wrapped should contain a space continuation character to comply with RFC 822"}
	want := "Subject: Long subject lines that get wrapped should contain a space\n continuation character to comply with RFC 822"
	if data := mustBytes(t, m); !bytes.Contains(data, []byte(want)) {
		t.Errorf("message lacks %q:\n%s", want, data)
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_message_header_overrides
//
// A Date or Message-ID in the extra headers replaces the default (#9233).
func TestMailMessageHeaderOverrides(t *testing.T) {
	m := &Message{Headers: []Header{{"date", "Fri, 09 Nov 2001 01:08:47 -0000"}, {"Message-ID", "foo"}}}
	hasHeaders(t, parse(t, mustBytes(t, m)), field{"Message-ID", "foo"}, field{"date", "Fri, 09 Nov 2001 01:08:47 -0000"})
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_datetime_in_date_header
// passes a datetime as a header value. Header values are strings; format a
// time.Time with time.RFC1123Z.

// Ported from Django: tests/mail/tests.py::MailTests.test_from_header
func TestMailFromHeader(t *testing.T) {
	m := &Message{From: "bounce@example.com", Headers: []Header{{"From", "from@example.com"}}}
	check(t, parse(t, mustBytes(t, m)).getAll("From"), []string{"from@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_to_header
func TestMailToHeader(t *testing.T) {
	to := []string{"list-subscriber@example.com", "list-subscriber2@example.com"}
	m := &Message{To: to, Headers: []Header{{"To", "mailing-list@example.com"}}}
	check(t, parse(t, mustBytes(t, m)).getAll("To"), []string{"mailing-list@example.com"})
	check(t, m.To, to)

	// Without a To header, the header lists To.
	m = &Message{To: to}
	check(t, parse(t, mustBytes(t, m)).getAll("To"), []string{"list-subscriber@example.com, list-subscriber2@example.com"})
	check(t, m.To, to)
}

// Ported from Django: tests/mail/tests.py::MailTests.test_to_in_headers_only
func TestMailToInHeadersOnly(t *testing.T) {
	m := &Message{Headers: []Header{{"To", "to@example.com"}}}
	check(t, parse(t, mustBytes(t, m)).getAll("To"), []string{"to@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_reply_to_header
func TestMailReplyToHeader(t *testing.T) {
	m := &Message{ReplyTo: []string{"foo@example.com"}, Headers: []Header{{"Reply-To", "override@example.com"}}}
	check(t, parse(t, mustBytes(t, m)).getAll("Reply-To"), []string{"override@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_reply_to_in_headers_only
func TestMailReplyToInHeadersOnly(t *testing.T) {
	m := &Message{Headers: []Header{{"Reply-To", "reply_to@example.com"}}}
	check(t, parse(t, mustBytes(t, m)).getAll("Reply-To"), []string{"reply_to@example.com"})
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_lazy_headers passes
// lazy translation strings. Go has no lazy strings.

// Ported from Django: tests/mail/tests.py::MailTests.test_multiple_message_call
//
// Building a message twice leaves the headers as they are (#13259).
func TestMailMultipleMessageCall(t *testing.T) {
	m := &Message{From: "bounce@example.com", Headers: []Header{{"From", "from@example.com"}}}
	check(t, parse(t, mustBytes(t, m)).getAll("From"), []string{"from@example.com"})
	check(t, parse(t, mustBytes(t, m)).getAll("From"), []string{"from@example.com"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_unicode_address_header
//
// Address headers with non-ASCII text parse back to the same addresses,
// commas included (#11144).
func TestMailUnicodeAddressHeader(t *testing.T) {
	m := &Message{To: []string{`"Firstname Sürname" <to@example.com>`, "other@example.com"}}
	check(t, parse(t, mustBytes(t, m)).addresses(t, "To"), []address{{"Firstname Sürname", "to@example.com"}, {"", "other@example.com"}})

	m = &Message{To: []string{`"Sürname, Firstname" <to@example.com>`, "other@example.com"}}
	check(t, parse(t, mustBytes(t, m)).addresses(t, "To"), []address{{"Sürname, Firstname", "to@example.com"}, {"", "other@example.com"}})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_unicode_headers
//
// Non-ASCII headers use RFC 2047 encoded words that parse back to the
// values.
func TestMailUnicodeHeaders(t *testing.T) {
	m := &Message{
		Subject: "Gżegżółka", To: []string{"to@example.com"},
		Headers: []Header{{"Sender", `"Firstname Sürname" <sender@example.com>`}, {"Comments", "My Sürname is non-ASCII"}},
	}
	data := mustBytes(t, m)
	if !isASCII(string(data)) {
		t.Errorf("the message is not ASCII:\n%s", data)
	}
	p := parse(t, data)
	check(t, p.get("Subject"), "Gżegżółka")
	check(t, p.addresses(t, "Sender"), []address{{"Firstname Sürname", "sender@example.com"}})
	check(t, p.get("Comments"), "My Sürname is non-ASCII")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_non_utf8_headers_multipart
//
// A message with alternatives and another encoding than UTF-8 encodes its
// headers too.
func TestMailNonUTF8HeadersMultipart(t *testing.T) {
	m := &Message{
		Subject: "Message from Firstname Sürname", Body: "This is an important message.",
		From: "from@example.com", To: []string{`"Sürname, Firstname" <to@example.com>`},
		Headers:  []Header{{"Date", "Fri, 09 Nov 2001 01:08:47 -0000"}, {"Message-ID", "foo"}},
		Encoding: "iso-8859-1",
	}
	if err := m.AttachAlternative("<p>This is an <strong>important</strong> message.</p>", "text/html"); err != nil {
		t.Fatal(err)
	}
	data := mustBytes(t, m)
	if !isASCII(string(data)) {
		t.Errorf("the message is not ASCII:\n%s", data)
	}
	p := parse(t, data)
	check(t, p.get("Subject"), "Message from Firstname Sürname")
	check(t, p.addresses(t, "To"), []address{{"Sürname, Firstname", "to@example.com"}})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_multipart_with_attachments
//
// A message with an empty body and attachments keeps its alternatives.
func TestMailMultipartWithAttachments(t *testing.T) {
	m := &Message{}
	html := "<p>This is <strong>html</strong></p>"
	if err := m.AttachAlternative(html, "text/html"); err != nil {
		t.Fatal(err)
	}
	if err := m.Attach("example.txt", []byte("Text file content"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if data := mustBytes(t, m); !bytes.Contains(data, []byte(html)) {
		t.Errorf("the message lacks the HTML:\n%s", data)
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_alternatives
func TestMailAlternatives(t *testing.T) {
	m := &Message{}
	html := "<p>This is <strong>html</strong></p>"
	if err := m.AttachAlternative(html, "text/html"); err != nil {
		t.Fatal(err)
	}
	check(t, m.Alternatives[0], Alternative{Content: html, MIMEType: "text/html"})
	if data := mustBytes(t, m); !bytes.Contains(data, []byte(html)) {
		t.Errorf("the message lacks the HTML:\n%s", data)
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_alternatives_constructor
func TestMailAlternativesConstructor(t *testing.T) {
	html := "<p>This is <strong>html</strong></p>"
	m := &Message{Alternatives: []Alternative{{Content: html, MIMEType: "text/html"}}}
	check(t, m.Alternatives[0].Content, html)
	check(t, m.Alternatives[0].MIMEType, "text/html")
	if data := mustBytes(t, m); !bytes.Contains(data, []byte(html)) {
		t.Errorf("the message lacks the HTML:\n%s", data)
	}
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_alternatives_constructor_from_tuple
// passes plain tuples. Alternatives are Alternative values.

// Ported from Django: tests/mail/tests.py::MailTests.test_alternative_alternatives
//
// Alternatives need not use a text type. The cases with bytes content are
// the same as the string cases: Alternative.Content is a string.
func TestMailAlternativeAlternatives(t *testing.T) {
	for _, c := range []struct{ mimeType, content string }{
		{"application/x-ccmail-rtf", "non-text\x07string"},
		{"text/x-amp-html", "text string\n"},
	} {
		m := &Message{}
		if err := m.AttachAlternative(c.content, c.mimeType); err != nil {
			t.Fatal(err)
		}
		p := parse(t, mustBytes(t, m))
		check(t, p.contentType(), "multipart/alternative")
		check(t, p.parts[0].contentType(), c.mimeType)
		check(t, string(p.parts[0].decoded(t)), c.content)
	}
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_alternatives_and_attachment_serializable
// pickles a message. Messages are plain structs.

// Ported from Django: tests/mail/tests.py::MailTests.test_none_body
//
// Python's email package ends every text part with a newline, an empty body
// too.
func TestMailNoneBody(t *testing.T) {
	m := &Message{Subject: "subject", From: "from@example.com", To: []string{"to@example.com"}}
	check(t, parse(t, mustBytes(t, m)).payload(), "\n")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_non_ascii_dns_non_unicode_email
func TestMailNonASCIIDNSNonUnicodeEmail(t *testing.T) {
	prev := getfqdn
	getfqdn = func() string { return "漢字" }
	resetDNSName()
	t.Cleanup(func() {
		getfqdn = prev
		resetDNSName()
	})
	m := &Message{Encoding: "iso-8859-1"}
	if id := parse(t, mustBytes(t, m)).get("Message-ID"); !strings.Contains(id, "@xn--p8s937b>") {
		t.Errorf("Message-ID %q lacks the punycode domain", id)
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_encoding
//
// The body uses the encoding of the message (#12791).
func TestMailEncoding(t *testing.T) {
	m := &Message{Body: "Firstname Sürname is a great guy.\n", Encoding: "iso-8859-1"}
	data := mustBytes(t, m)
	p := parse(t, data)
	check(t, p.get("Content-Type"), `text/plain; charset="iso-8859-1"`)
	check(t, p.get("Content-Transfer-Encoding"), "8bit")
	if !bytes.Contains(data, []byte("Firstname S\xfc")) {
		t.Errorf("the body is not ISO-8859-1:\n%q", data)
	}
	check(t, p.text(t), "Firstname Sürname is a great guy.\n")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_encoding_alternatives
func TestMailEncodingAlternatives(t *testing.T) {
	text := "Firstname Sürname is a great guy.\n"
	html := "<p>Firstname Sürname is a <strong>great</strong> guy.</p>\n"
	m := &Message{Body: text, Encoding: "iso-8859-1"}
	if err := m.AttachAlternative(html, "text/html"); err != nil {
		t.Fatal(err)
	}
	p := parse(t, mustBytes(t, m))
	check(t, p.parts[0].get("Content-Type"), `text/plain; charset="iso-8859-1"`)
	check(t, p.parts[1].get("Content-Type"), `text/html; charset="iso-8859-1"`)
	check(t, p.parts[0].text(t), text)
	check(t, p.parts[1].text(t), html)
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attachments
func TestMailAttachments(t *testing.T) {
	m := &Message{}
	if err := m.Attach("example.txt", []byte("Text file content\n"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	check(t, m.Attachments[0].Filename, "example.txt")
	check(t, m.Attachments[0].Content, []byte("Text file content\n"))
	check(t, m.Attachments[0].MIMEType, "text/plain")
	check(t, decodedAttachments(t, m)[0], decodedAttachment{"example.txt", "Text file content\n", "text/plain"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attachments_constructor
func TestMailAttachmentsConstructor(t *testing.T) {
	m := &Message{Attachments: []Attachment{{Filename: "example.txt", Content: []byte("Text file content\n"), MIMEType: "text/plain"}}}
	check(t, m.Attachments[0].Filename, "example.txt")
	check(t, decodedAttachments(t, m)[0], decodedAttachment{"example.txt", "Text file content\n", "text/plain"})
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_attachments_constructor_from_tuple
// passes plain tuples. Attachments are Attachment values.

// Ported from Django: tests/mail/tests.py::MailTests.test_attachments_constructor_omit_mimetype
//
// An attachment without a MIME type gets the type guessed from its name,
// or DefaultAttachmentMIMEType.
func TestMailAttachmentsConstructorOmitMIMEType(t *testing.T) {
	m := &Message{Attachments: []Attachment{{Filename: "filename1", Content: []byte("content1")}}}
	check(t, decodedAttachments(t, m)[0], decodedAttachment{"filename1", "content1", "application/octet-stream"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attachments_with_alternative_parts
//
// A message with attachments and alternatives nests the alternatives in
// the mixed part (#9367).
func TestMailAttachmentsWithAlternativeParts(t *testing.T) {
	m := &Message{Body: "This is an important message."}
	if err := m.AttachAlternative("<p>This is an <strong>important</strong> message.</p>", "text/html"); err != nil {
		t.Fatal(err)
	}
	if err := m.Attach("an attachment.pdf", []byte("%PDF-1.4.%..."), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	p := parse(t, mustBytes(t, m))
	check(t, p.isMultipart(), true)
	check(t, p.contentType(), "multipart/mixed")
	check(t, p.parts[0].contentType(), "multipart/alternative")
	check(t, p.parts[1].contentType(), "application/pdf")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_decoded_attachment_text_MIMEPart
//
// A Part is added as it is, without a MIME-Version header.
func TestMailDecodedAttachmentTextMIMEPart(t *testing.T) {
	m := &Message{Attachments: []Attachment{{Part: &Part{Content: []byte("content1")}}}}
	p := parse(t, mustBytes(t, m))
	check(t, p.parts[0].items(), []field{
		{"Content-Type", `text/plain; charset="utf-8"`},
		{"Content-Transfer-Encoding", "7bit"},
	})
	check(t, p.parts[0].payload(), "content1\n")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_non_ascii_attachment_filename
func TestMailNonASCIIAttachmentFilename(t *testing.T) {
	m := &Message{Body: "Content"}
	if err := m.Attach("une pièce jointe.pdf", []byte("%PDF-1.4.%..."), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	check(t, decodedAttachments(t, m)[0].filename, "une pièce jointe.pdf")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_file
//
// A file is attached under its name with a given or guessed type; text that
// is not UTF-8 falls back to DefaultAttachmentMIMEType.
func TestMailAttachFile(t *testing.T) {
	files := []struct{ basename, realType string }{
		{"file.txt", "text/plain"},
		{"file.png", "image/png"},
		{"file_txt", ""},
		{"file_png", ""},
		{"file_txt.png", "image/png"},
		{"file_png.txt", "text/plain"},
		{"file.eml", "message/rfc822"},
	}
	for _, f := range files {
		for _, mimeType := range []string{"text/plain", "image/png", ""} {
			check(t, GuessType(f.basename), f.realType)
			want := mimeType
			if want == "" {
				want = f.realType
			}
			if want == "" {
				want = DefaultAttachmentMIMEType
			}
			path := filepath.Join("testdata", "attachments", f.basename)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(want, "text/") && !isUTF8(content) {
				want = DefaultAttachmentMIMEType
			}
			m := &Message{}
			if err := m.AttachFile(path, mimeType); err != nil {
				t.Fatal(err)
			}
			check(t, len(m.Attachments), 1)
			check(t, m.Attachments[0].Filename, f.basename)
			check(t, m.Attachments[0].MIMEType, want)
			check(t, m.Attachments[0].Content, content)

			got := decodedAttachments(t, m)
			check(t, len(got), 1)
			check(t, got[0].filename, f.basename)
			check(t, got[0].mimeType, want)
		}
	}
}

// isUTF8 reports whether data is valid UTF-8.
func isUTF8(data []byte) bool { return strings.ToValidUTF8(string(data), "\uFFFD") == string(data) }

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_text_as_bytes
//
// Text attachments that are not UTF-8 get DefaultAttachmentMIMEType.
func TestMailAttachTextAsBytes(t *testing.T) {
	m := &Message{}
	if err := m.Attach("utf8.txt", []byte("ütƒ-8\n"), ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Attach("not-utf8.txt", []byte("\x86unknown-encoding\n"), ""); err != nil {
		t.Fatal(err)
	}
	got := decodedAttachments(t, m)
	check(t, got[0], decodedAttachment{"utf8.txt", "ütƒ-8\n", "text/plain"})
	check(t, got[1], decodedAttachment{"not-utf8.txt", "\x86unknown-encoding\n", "application/octet-stream"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_text_as_bytes_using_property
//
// The rule of TestMailAttachTextAsBytes also applies to attachments set on
// the field.
func TestMailAttachTextAsBytesUsingProperty(t *testing.T) {
	m := &Message{Attachments: []Attachment{
		{Filename: "utf8.txt", Content: []byte("ütƒ-8\n"), MIMEType: "text/plain"},
		{Filename: "not-utf8.txt", Content: []byte("\x86unknown-encoding\n"), MIMEType: "text/plain"},
	}}
	got := decodedAttachments(t, m)
	check(t, len(got), 2)
	check(t, got[0], decodedAttachment{"utf8.txt", "ütƒ-8\n", "text/plain"})
	check(t, got[1], decodedAttachment{"not-utf8.txt", "\x86unknown-encoding\n", "application/octet-stream"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_utf8_text_as_bytes
func TestMailAttachUTF8TextAsBytes(t *testing.T) {
	m := &Message{}
	if err := m.Attach("file.txt", []byte("\xc3\xa4\n"), ""); err != nil {
		t.Fatal(err)
	}
	check(t, decodedAttachments(t, m)[0], decodedAttachment{"file.txt", "ä\n", "text/plain"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_non_utf8_text_as_bytes
func TestMailAttachNonUTF8TextAsBytes(t *testing.T) {
	m := &Message{}
	if err := m.Attach("file.txt", []byte("\xff"), ""); err != nil {
		t.Fatal(err)
	}
	check(t, decodedAttachments(t, m)[0], decodedAttachment{"file.txt", "\xff", "application/octet-stream"})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_8bit_rfc822_message_non_ascii
//
// An attached message with 8bit content keeps it (#36119).
func TestMailAttach8bitRFC822MessageNonASCII(t *testing.T) {
	attachment := "Subject: A message using 8bit CTE\n" +
		"Content-Type: text/plain; charset=utf-8\n" +
		"Content-Transfer-Encoding: 8bit\n" +
		"\n" +
		"¡8-bit content!\n"
	m := &Message{}
	if err := m.Attach("attachment.eml", []byte(attachment), "message/rfc822"); err != nil {
		t.Fatal(err)
	}
	got := rawAttachments(t, m)
	check(t, len(got), 1)
	check(t, got[0].contentType(), "message/rfc822")
	inner := got[0].parts[0]
	check(t, strings.TrimRight(inner.text(t), " \t\n"), "¡8-bit content!")
	check(t, inner.get("Content-Transfer-Encoding"), "8bit")
	check(t, inner.contentType(), "text/plain")
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_attach_mime_image
// attaches a MIMEBase, which Django deprecates. Part takes its place.

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_mime_part
//
// A Part carries its own headers, such as an inline image with a
// Content-ID.
func TestMailAttachMIMEPart(t *testing.T) {
	m := &Message{}
	m.AttachPart(&Part{MIMEType: "image/gif", Content: []byte("GIF89a..."), Disposition: "inline", ContentID: "<content-id@example.org>"})
	got := rawAttachments(t, m)
	check(t, len(got), 1)
	check(t, got[0].contentType(), "image/gif")
	check(t, got[0].disposition(), "inline")
	check(t, got[0].get("Content-ID"), "<content-id@example.org>")
	check(t, string(got[0].decoded(t)), "GIF89a...")
	check(t, got[0].filename(), "")
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_attach_mime_image_in_constructor
// passes a MIMEBase, which Django deprecates.

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_mime_part_in_constructor
func TestMailAttachMIMEPartInConstructor(t *testing.T) {
	m := &Message{Attachments: []Attachment{{Part: &Part{MIMEType: "image/png", Content: []byte("\x89PNG..."), Filename: "test.png"}}}}
	got := rawAttachments(t, m)
	check(t, len(got), 1)
	check(t, got[0].contentType(), "image/png")
	check(t, string(got[0].decoded(t)), "\x89PNG...")
	check(t, got[0].disposition(), "attachment")
	check(t, got[0].filename(), "test.png")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_rfc822_message
//
// A message/rfc822 attachment holds a Message or the bytes of a message.
// The bytes cases are what Python's legacy Message and modern EmailMessage
// serialize to.
func TestMailAttachRFC822Message(t *testing.T) {
	cases := []struct {
		name string
		add  func(m *Message) error
	}{
		{"Message", func(m *Message) error {
			m.AttachMessage("", &Message{Subject: "child subject", Body: "child body"})
			return nil
		}},
		{"legacy message bytes", func(m *Message) error {
			return m.Attach("", []byte("Subject: child subject\n\nchild body"), "message/rfc822")
		}},
		{"email message bytes", func(m *Message) error {
			return m.Attach("", []byte("Subject: child subject\nContent-Type: text/plain; charset=\"utf-8\"\n"+
				"Content-Transfer-Encoding: 7bit\nMIME-Version: 1.0\n\nchild body\n"), "message/rfc822")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &Message{Subject: "parent message", Body: "parent body"}
			if err := c.add(m); err != nil {
				t.Fatal(err)
			}
			check(t, len(m.Attachments), 1)
			check(t, m.Attachments[0].MIMEType, "message/rfc822")

			got := rawAttachments(t, m)
			check(t, len(got), 1)
			check(t, got[0].contentType(), "message/rfc822")
			inner := got[0].parts[0]
			check(t, inner.get("Subject"), "child subject")
			check(t, strings.TrimRight(inner.text(t), " \t\n"), "child body")

			// RFC 2046 5.2.1 allows only 7bit, 8bit and binary for a
			// message/rfc822 entity (#18967).
			cte := got[0].get("Content-Transfer-Encoding")
			if cte == "" {
				cte = "7bit"
			}
			if cte != "7bit" && cte != "8bit" && cte != "binary" {
				t.Errorf("Content-Transfer-Encoding %q", cte)
			}
			childCTE := inner.get("Content-Transfer-Encoding")
			if childCTE == "" {
				childCTE = "7bit"
			}
			check(t, childCTE, "7bit")
			check(t, inner.contentType(), "text/plain")
		})
	}
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_attach_mimepart_prohibits_other_params
// passes content and a MIME type with a MIMEPart. AttachPart takes only the
// part.

// Ported from Django: tests/mail/tests.py::MailTests.test_attach_content_is_required
func TestMailAttachContentIsRequired(t *testing.T) {
	m := &Message{}
	errorContains(t, m.Attach("file.txt", nil, "application/pdf"), "content must be provided.")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_dummy_backend
func TestMailDummyBackend(t *testing.T) {
	m := &Message{To: []string{"to@example.com"}}
	n, err := DummyBackend{}.SendMessages([]*Message{m, m, m})
	check(t, err, nil)
	check(t, n, 3)
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_arbitrary_keyword
// passes an unknown keyword argument to get_connection. Options are typed.

// Ported from Django: tests/mail/tests.py::MailTests.test_custom_backend
func TestMailCustomBackend(t *testing.T) {
	conn := newCustomBackend(t)
	if _, err := conn.SendMessages([]*Message{{To: []string{"to@example.com"}}}); err != nil {
		t.Fatal(err)
	}
	check(t, len(conn.testOutbox), 1)
}

// Ported from Django: tests/mail/tests.py::MailTests.test_backend_arg
//
// The check that a file path of the wrong type is a TypeError is left out:
// WithFilePath takes a string.
func TestMailBackendArg(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []Option
		want any
	}{
		{"smtp", nil, &SMTPBackend{}},
		{"locmem", nil, LocmemBackend{}},
		{"dummy", nil, DummyBackend{}},
		{"console", nil, &ConsoleBackend{}},
		{"file", []Option{WithFilePath(t.TempDir())}, &FileBackend{}},
	} {
		conn, err := NewConnection(append(c.opts, WithBackend(c.name))...)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if reflect.TypeOf(conn) != reflect.TypeOf(c.want) {
			t.Errorf("%s: got %T, want %T", c.name, conn, c.want)
		}
	}
	conn, err := NewConnection()
	check(t, err, nil)
	check(t, reflect.TypeOf(conn), reflect.TypeOf(LocmemBackend{}))
}

// Ported from Django: tests/mail/tests.py::MailTests.test_connection_arg_send_mail
func TestMailConnectionArgSendMail(t *testing.T) {
	ClearOutbox()
	conn := newCustomBackend(t)
	if _, err := SendMail("Subject", "Content", "from@example.com", []string{"to@example.com"}, WithConnection(conn)); err != nil {
		t.Fatal(err)
	}
	check(t, len(Outbox()), 0)
	check(t, len(conn.testOutbox), 1)
	check(t, conn.testOutbox[0].Subject, "Subject")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_connection_arg_send_mass_mail
func TestMailConnectionArgSendMassMail(t *testing.T) {
	ClearOutbox()
	conn := newCustomBackend(t)
	_, err := SendMassMail([]MassMessage{
		{"Subject1", "Content1", "from1@example.com", []string{"to1@example.com"}},
		{"Subject2", "Content2", "from2@example.com", []string{"to2@example.com"}},
	}, WithConnection(conn))
	if err != nil {
		t.Fatal(err)
	}
	check(t, len(Outbox()), 0)
	check(t, len(conn.testOutbox), 2)
	check(t, conn.testOutbox[0].Subject, "Subject1")
	check(t, conn.testOutbox[1].Subject, "Subject2")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_connection_arg_mail_admins
func TestMailConnectionArgMailAdmins(t *testing.T) {
	withConfig(t, func(c *Config) { c.Admins = []string{"nobody@example.com"} })
	ClearOutbox()
	conn := newCustomBackend(t)
	if err := MailAdmins("Admin message", "Content", WithConnection(conn)); err != nil {
		t.Fatal(err)
	}
	check(t, len(Outbox()), 0)
	check(t, len(conn.testOutbox), 1)
	check(t, conn.testOutbox[0].Subject, "[Django] Admin message")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_connection_arg_mail_managers
func TestMailConnectionArgMailManagers(t *testing.T) {
	withConfig(t, func(c *Config) { c.Managers = []string{"nobody@example.com"} })
	ClearOutbox()
	conn := newCustomBackend(t)
	if err := MailManagers("Manager message", "Content", WithConnection(conn)); err != nil {
		t.Fatal(err)
	}
	check(t, len(Outbox()), 0)
	check(t, len(conn.testOutbox), 1)
	check(t, conn.testOutbox[0].Subject, "[Django] Manager message")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_dont_mangle_from_in_body
//
// "From " at the start of a body line stays as it is (#13433).
func TestMailDontMangleFromInBody(t *testing.T) {
	m := &Message{Body: "From the future"}
	if data := mustBytes(t, m); bytes.Contains(data, []byte(">From the future")) {
		t.Errorf("the body is mangled:\n%s", data)
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_body_content_transfer_encoding
//
// The body uses 7bit or 8bit when it can, and otherwise the shorter of
// quoted-printable and base64 (#3472, #11212).
func TestMailBodyContentTransferEncoding(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"Body with only ASCII characters.", "Content-Transfer-Encoding: 7bit"},
		{"Body with latin characters: àáä.", "Content-Transfer-Encoding: 8bit"},
		{
			"Body with non latin characters: А Б В Г Д Е Ж Ѕ З И І К Л М Н О П.\n" +
				"Because it has a line > 78 utf-8 octets, it should be folded, and " +
				"must then be encoded using the shorter of quoted-printable or base64.",
			"Content-Transfer-Encoding: quoted-printable",
		},
	} {
		m := &Message{Body: c.body}
		if data := mustBytes(t, m); !bytes.Contains(data, []byte(c.want)) {
			t.Errorf("the message lacks %q:\n%s", c.want, data)
		}
	}
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_sanitize_address,
// test_sanitize_address_invalid and test_sanitize_address_header_injection
// test sanitize_address(), which Django deprecates. The address headers are
// covered by TestMailAddressHeaderHandling and TestMailAddressHeaderInjection.

// Ported from Django: tests/mail/tests.py::MailTests.test_address_header_handling
func TestMailAddressHeaderHandling(t *testing.T) {
	long := strings.Repeat("Tó Example very long", 4)
	longASCII := strings.Repeat("To Example very long", 4)
	for _, c := range []struct{ address, name, addrSpec string }{
		{"to@example.com", "", "to@example.com"},
		// Display names.
		{"A name <to@example.com>", "A name", "to@example.com"},
		{`"A name" <to@example.com>`, "A name", "to@example.com"},
		{`"Comma, requires quotes" <to@example.com>`, "Comma, requires quotes", "to@example.com"},
		{`"to@other.com" <to@example.com>`, "to@other.com", "to@example.com"},
		// A non-ASCII domain is encoded with IDNA.
		{"to@éxample.com", "", "to@xn--xample-9ua.com"},
		{"To Example <to@éxample.com>", "To Example", "to@xn--xample-9ua.com"},
		// An IDNA domain stays as it is.
		{"to@xn--fa-hia.example.com", "", "to@xn--fa-hia.example.com"},
		{"<to@xn--10cl1a0b660p.example.com>", "", "to@xn--10cl1a0b660p.example.com"},
		{`"Display, Name" <to@xn--nxasmm1c.example.com>`, "Display, Name", "to@xn--nxasmm1c.example.com"},
		// A non-ASCII display name.
		{"Tó Example <to@example.com>", "Tó Example", "to@example.com"},
		// Two @ signs in a quoted local part.
		{`"to@other.com"@example.com`, "", `"to@other.com"@example.com`},
		{`To Example <"to@other.com"@example.com>`, "To Example", `"to@other.com"@example.com`},
		// Long non-ASCII display names.
		{long + " <to@example.com>", long, "to@example.com"},
		// A long display name and a non-ASCII domain.
		{longASCII + " <to@exampl€.com>", longASCII, "to@xn--exampl-nc1c.com"},
	} {
		t.Run(c.address, func(t *testing.T) {
			m := &Message{To: []string{c.address}}
			check(t, parse(t, mustBytes(t, m)).addresses(t, "To"), []address{{c.name, c.addrSpec}})
		})
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_address_header_injection
func TestMailAddressHeaderInjection(t *testing.T) {
	msg := "Header values may not contain linefeed or carriage return characters"
	cases := []string{
		"Name\nInjection <to@example.com>",
		"\"Name\nInjection\" <to@example.com>",
		"\"Name\rInjection\" <to@example.com>",
		"\"Name\r\nInjection\" <to@example.com>",
		"Name <to\ninjection@example.com>",
		"to\ninjection@example.com",
	}
	// The structured address headers of RFC 5322 3.6. Bcc is not in the
	// message; the SMTP backend checks it.
	headers := []string{"From", "Sender", "Reply-To", "To", "Cc", "Resent-From", "Resent-Sender", "Resent-To", "Resent-Cc", "Resent-Bcc"}
	for _, header := range headers {
		for _, addr := range cases {
			var m *Message
			switch header {
			case "From":
				m = &Message{From: addr}
			case "To":
				m = &Message{To: []string{addr}}
			case "Cc":
				m = &Message{Cc: []string{addr}}
			case "Reply-To":
				m = &Message{ReplyTo: []string{addr}}
			default:
				m = &Message{Headers: []Header{{header, addr}}}
			}
			_, err := m.Bytes()
			if err == nil || !strings.Contains(err.Error(), msg) {
				t.Errorf("%s %q: error %v, want %q", header, addr, err, msg)
			}
		}
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_localpart_only_address
//
// An address without a domain is allowed for local delivery (#15042).
func TestMailLocalpartOnlyAddress(t *testing.T) {
	m := &Message{To: []string{"localpartonly"}}
	check(t, parse(t, mustBytes(t, m)).addresses(t, "To"), []address{{"", "localpartonly"}})
}

// Ported from Django: tests/mail/tests.py::MailTests.test_email_multi_alternatives_content_mimetype_none
//
// Only the missing MIME type is checked: Alternative content is a string,
// which has no None.
func TestMailEmailMultiAlternativesContentMIMETypeNone(t *testing.T) {
	m := &Message{}
	errorContains(t, m.AttachAlternative("<p>content</p>", ""), "Both content and mimetype must be provided.")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_mime_structure
func TestMailMIMEStructure(t *testing.T) {
	html := Alternative{Content: "<p>HTML</p>", MIMEType: "text/html"}
	image := Attachment{Filename: "image.gif", Content: []byte("\x89PNG..."), MIMEType: "image/png"}
	rfc822 := Attachment{Message: &Message{Body: "text"}, MIMEType: "message/rfc822"}
	cases := []struct {
		name string
		m    *Message
		want string
	}{
		{"single body", &Message{Body: "text"}, "text/plain\n"},
		{
			"single body with attachment", &Message{Body: "text", Attachments: []Attachment{image}},
			"multipart/mixed\n    text/plain\n    image/png\n",
		},
		{
			"alternative bodies", &Message{Body: "text", Alternatives: []Alternative{html}},
			"multipart/alternative\n    text/plain\n    text/html\n",
		},
		{
			"alternative bodies with attachments", &Message{Body: "text", Alternatives: []Alternative{html}, Attachments: []Attachment{image}},
			"multipart/mixed\n    multipart/alternative\n        text/plain\n        text/html\n    image/png\n",
		},
		{
			"alternative bodies with rfc822 attachment", &Message{Body: "text", Alternatives: []Alternative{html}, Attachments: []Attachment{rfc822}},
			"multipart/mixed\n    multipart/alternative\n        text/plain\n        text/html\n    message/rfc822\n        text/plain\n",
		},
		{
			"attachment only", &Message{Attachments: []Attachment{image}},
			"multipart/mixed\n    image/png\n",
		},
		{
			"alternative only", &Message{Alternatives: []Alternative{html}},
			"multipart/alternative\n    text/html\n",
		},
		{
			"alternative and attachment only", &Message{Alternatives: []Alternative{html}, Attachments: []Attachment{image}},
			"multipart/mixed\n    multipart/alternative\n        text/html\n    image/png\n",
		},
		{"empty message", &Message{}, "text/plain\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check(t, parse(t, mustBytes(t, c.m)).structure(0), c.want)
		})
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_body_contains
func TestMailBodyContains(t *testing.T) {
	m := &Message{Body: "I am content."}
	check(t, m.BodyContains("I am"), true)
	check(t, m.BodyContains("I am content."), true)

	if err := m.AttachAlternative("<p>I am different content.</p>", "text/html"); err != nil {
		t.Fatal(err)
	}
	check(t, m.BodyContains("I am"), true)
	check(t, m.BodyContains("I am content."), false)
	check(t, m.BodyContains("<p>I am different content.</p>"), false)
}

// Ported from Django: tests/mail/tests.py::MailTests.test_body_contains_alternative_non_text
func TestMailBodyContainsAlternativeNonText(t *testing.T) {
	m := &Message{Body: "I am content."}
	if err := m.AttachAlternative("I am content.", "text/html"); err != nil {
		t.Fatal(err)
	}
	if err := m.AttachAlternative("I am a song.", "audio/mpeg"); err != nil {
		t.Fatal(err)
	}
	check(t, m.BodyContains("I am content"), true)
}

// Ported from Django: tests/mail/tests.py::MailTests.test_all_params_optional
func TestMailAllParamsOptional(t *testing.T) {
	if _, err := (&Message{}).Bytes(); err != nil {
		t.Fatal(err)
	}
}

// Ported from Django: tests/mail/tests.py::MailTests.test_positional_arguments_order
//
// Go has no positional arguments; the test sets every field and checks the
// message.
func TestMailPositionalArgumentsOrder(t *testing.T) {
	conn, err := NewConnection()
	if err != nil {
		t.Fatal(err)
	}
	m := &Message{
		Subject: "subject", Body: "body\n", From: "from@example.com", To: []string{"to@example.com"},
		Bcc: []string{"bcc@example.com"}, Connection: conn,
		Attachments: []Attachment{{Filename: "file.txt", Content: []byte("attachment\n"), MIMEType: "text/plain"}},
		Headers:     []Header{{"X-Header", "custom header"}},
		Cc:          []string{"cc@example.com"}, ReplyTo: []string{"reply-to@example.com"},
	}
	p := parse(t, mustBytes(t, m))
	check(t, p.getAll("Subject"), []string{"subject"})
	check(t, p.getAll("From"), []string{"from@example.com"})
	check(t, p.getAll("To"), []string{"to@example.com"})
	check(t, p.getAll("X-Header"), []string{"custom header"})
	check(t, p.getAll("Cc"), []string{"cc@example.com"})
	check(t, p.getAll("Reply-To"), []string{"reply-to@example.com"})
	check(t, p.parts[0].payload(), "body\n")
	check(t, decodedAttachments(t, m), []decodedAttachment{{"file.txt", "attachment\n", "text/plain"}})
	check(t, m.Recipients(), []string{"to@example.com", "cc@example.com", "bcc@example.com"})
	check(t, m.Connection, conn)
}

// Ported from Django: tests/mail/tests.py::MailTests.test_all_params_can_be_set_before_send
//
// Every field can change before the message is sent.
func TestMailAllParamsCanBeSetBeforeSend(t *testing.T) {
	original := newCustomBackend(t)
	replacement := newCustomBackend(t)
	m := &Message{
		Subject: "original subject", Body: "original body\n", From: "original-from@example.com",
		To: []string{"original-to@example.com"}, Bcc: []string{"original-bcc@example.com"}, Connection: original,
		Attachments: []Attachment{{Filename: "original.txt", Content: []byte("original attachment\n"), MIMEType: "text/plain"}},
		Headers:     []Header{{"X-Header", "original header"}},
		Cc:          []string{"original-cc@example.com"}, ReplyTo: []string{"original-reply-to@example.com"},
	}
	m.Subject = "new subject"
	m.Body = "new body\n"
	m.From = "new-from@example.com"
	m.To = []string{"new-to@example.com"}
	m.Bcc = []string{"new-bcc@example.com"}
	m.Connection = replacement
	m.Attachments = []Attachment{
		{Filename: "new1.txt", Content: []byte("new attachment 1\n"), MIMEType: "text/plain"},
		{Filename: "new2.txt", Content: []byte("new attachment 2\n"), MIMEType: "text/csv"},
		{Part: &Part{MIMEType: "image/gif", Content: []byte("GIF89a...")}},
	}
	m.Headers = []Header{{"X-Header", "new header"}}
	m.Cc = []string{"new-cc@example.com"}
	m.ReplyTo = []string{"new-reply-to@example.com"}

	data := mustBytes(t, m)
	p := parse(t, data)
	check(t, p.getAll("Subject"), []string{"new subject"})
	check(t, p.getAll("From"), []string{"new-from@example.com"})
	check(t, p.getAll("To"), []string{"new-to@example.com"})
	check(t, p.getAll("X-Header"), []string{"new header"})
	check(t, p.getAll("Cc"), []string{"new-cc@example.com"})
	check(t, p.getAll("Reply-To"), []string{"new-reply-to@example.com"})
	check(t, p.parts[0].payload(), "new body\n")
	check(t, decodedAttachments(t, m), []decodedAttachment{
		{"new1.txt", "new attachment 1\n", "text/plain"},
		{"new2.txt", "new attachment 2\n", "text/csv"},
		{"", "GIF89a...", "image/gif"},
	})
	check(t, m.Recipients(), []string{"new-to@example.com", "new-cc@example.com", "new-bcc@example.com"})
	if m.Connection != Backend(replacement) {
		t.Error("the message keeps the original connection")
	}
	if bytes.Contains(data, []byte("original")) {
		t.Errorf("the message contains an original value:\n%s", data)
	}
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_message_is_python_email_message
// checks the Python class of message().

// Ported from Django: tests/mail/tests.py::MailTests.test_message_policy_smtputf8
func TestMailMessagePolicySMTPUTF8(t *testing.T) {
	t.Skip("needs the SMTPUTF8 policy, which the mail package does not have")
}

// Ported from Django: tests/mail/tests.py::MailTests.test_message_policy_cte_7bit
func TestMailMessagePolicyCTE7bit(t *testing.T) {
	t.Skip("needs a policy with cte_type 7bit, which the mail package does not have")
}

// No Go counterpart: tests/mail/tests.py::MailTests.test_message_policy_compat32
// serializes the message with Python's compat32 policy.

// No Go counterpart: tests/mail/tests.py::MailDeprecatedPositionalArgsTests
// checks the deprecation of positional arguments. Go has no keyword
// arguments.

// Ported from Django: tests/mail/tests.py::MailTimeZoneTests.test_date_header_utc
//
// Without UseLocaltime, the Date header is in UTC.
func TestMailDateHeaderUTC(t *testing.T) {
	algiers, err := time.LoadLocation("Africa/Algiers")
	if err != nil {
		t.Skip("needs the Africa/Algiers time zone")
	}
	withConfig(t, func(c *Config) { c.UseLocaltime, c.Location = false, algiers })
	if date := parse(t, mustBytes(t, &Message{})).get("Date"); !strings.HasSuffix(date, "+0000") {
		t.Errorf("Date %q is not in UTC", date)
	}
}

// Ported from Django: tests/mail/tests.py::MailTimeZoneTests.test_date_header_localtime
//
// With UseLocaltime, the Date header is in the current time zone.
func TestMailDateHeaderLocaltime(t *testing.T) {
	algiers, err := time.LoadLocation("Africa/Algiers")
	if err != nil {
		t.Skip("needs the Africa/Algiers time zone")
	}
	withConfig(t, func(c *Config) { c.UseLocaltime, c.Location = true, algiers })
	// Africa/Algiers is UTC+1 all year.
	if date := parse(t, mustBytes(t, &Message{})).get("Date"); !strings.HasSuffix(date, "+0100") {
		t.Errorf("Date %q is not in local time", date)
	}
}

// No Go counterpart: tests/mail/tests.py::PythonGlobalState checks that
// Django leaves the charset registry of Python's email package alone.
