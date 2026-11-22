package mail

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultAttachmentMIMEType is the type of an attachment whose type is
// neither given nor guessed from its filename, and of text whose bytes are
// not UTF-8.
const DefaultAttachmentMIMEType = "application/octet-stream"

// Header is an extra header of a message.
type Header struct {
	Name  string
	Value string
}

// Alternative is another representation of the body of a message, such as
// HTML, as Django's EmailAlternative.
type Alternative struct {
	Content  string
	MIMEType string
}

// Attachment is a file attached to a message, as Django's EmailAttachment.
// Exactly one of Content, Message and Part holds the attachment.
type Attachment struct {
	// Filename is the name the recipient sees. Empty means none.
	Filename string
	// Content holds the bytes of the file. For a text type it holds UTF-8
	// text; for message/rfc822 it holds a whole message.
	Content []byte
	// MIMEType is the type of Content, such as "text/plain". Empty means
	// the type guessed from Filename, or DefaultAttachmentMIMEType.
	MIMEType string
	// Message is a message attached as message/rfc822.
	Message *Message
	// Part is a MIME part attached as it is.
	Part *Part
}

// Part is a MIME part that a message carries as it is, as a Python
// email.message.MIMEPart that Django's attach() accepts. Use it for inline
// images with a Content-ID.
type Part struct {
	// MIMEType is the type of Content. Empty means "text/plain".
	MIMEType string
	// Content is the payload. A text type holds UTF-8 text.
	Content []byte
	// Charset encodes a text type. Empty means "utf-8".
	Charset string
	// Filename adds a filename parameter to Content-Disposition.
	Filename string
	// Disposition is the Content-Disposition, such as "inline". Empty means
	// none, or "attachment" when Filename is set.
	Disposition string
	// ContentID is the Content-ID header, such as "<logo@example.com>".
	ContentID string
	// Headers are more headers of the part, added after the content
	// headers.
	Headers []Header
}

// Message is an email message, as Django's EmailMultiAlternatives: an
// EmailMessage that can also carry alternatives.
type Message struct {
	Subject string
	Body    string
	// From is the sender. Empty means the DefaultFromEmail setting.
	From string
	// To, Cc and Bcc are the recipients. Bcc never appears in a header.
	To  []string
	Cc  []string
	Bcc []string
	// ReplyTo fills the Reply-To header.
	ReplyTo []string
	// Headers are extra headers. A header named From, To, Cc or Reply-To
	// replaces the header built from the field of that name; the names of
	// these four are matched by exact case, as in Django.
	Headers []Header
	// Attachments are the attached files.
	Attachments []Attachment
	// Alternatives are other representations of the body.
	Alternatives []Alternative
	// ContentSubtype is the subtype of the body. Empty means "plain"; set
	// "html" to send an HTML body.
	ContentSubtype string
	// Encoding is the charset of text bodies and attachments. Empty means
	// the DefaultCharset setting.
	Encoding string
	// Connection sends the message. Nil means a connection that Send makes
	// and keeps here.
	Connection Backend
}

// Attach adds an attachment. An empty mimeType is guessed from the filename
// and falls back to DefaultAttachmentMIMEType. Text content that is not
// UTF-8 gets DefaultAttachmentMIMEType. Content must not be nil.
func (m *Message) Attach(filename string, content []byte, mimeType string) error {
	if content == nil {
		return errors.New("content must be provided.") //nolint:revive,staticcheck // Django's message
	}
	if mimeType == "" {
		mimeType = GuessType(filename)
	}
	if mimeType == "" {
		mimeType = DefaultAttachmentMIMEType
	}
	if maintype, _, _ := strings.Cut(mimeType, "/"); maintype == "text" && !utf8.Valid(content) {
		mimeType = DefaultAttachmentMIMEType
	}
	m.Attachments = append(m.Attachments, Attachment{Filename: filename, Content: content, MIMEType: mimeType})
	return nil
}

// AttachMessage attaches msg as a message/rfc822 part.
func (m *Message) AttachMessage(filename string, msg *Message) {
	m.Attachments = append(m.Attachments, Attachment{Filename: filename, Message: msg, MIMEType: "message/rfc822"})
}

// AttachPart attaches a MIME part as it is.
func (m *Message) AttachPart(p *Part) {
	m.Attachments = append(m.Attachments, Attachment{Part: p})
}

// AttachFile attaches the file at path under its base name. An empty
// mimeType is guessed as in Attach.
func (m *Message) AttachFile(path, mimeType string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if content == nil {
		content = []byte{}
	}
	return m.Attach(filepath.Base(path), content, mimeType)
}

// AttachAlternative adds another representation of the body, such as
// AttachAlternative(html, "text/html").
func (m *Message) AttachAlternative(content, mimeType string) error {
	if mimeType == "" {
		return errors.New("Both content and mimetype must be provided.") //nolint:revive,staticcheck // Django's message
	}
	m.Alternatives = append(m.Alternatives, Alternative{Content: content, MIMEType: mimeType})
	return nil
}

// Recipients returns the non-empty addresses of To, Cc and Bcc.
func (m *Message) Recipients() []string {
	var out []string
	for _, list := range [][]string{m.To, m.Cc, m.Bcc} {
		for _, a := range list {
			if a != "" {
				out = append(out, a)
			}
		}
	}
	return out
}

// BodyContains reports whether text occurs in the body and in every text
// alternative.
func (m *Message) BodyContains(text string) bool {
	if !strings.Contains(m.Body, text) {
		return false
	}
	for _, alt := range m.Alternatives {
		if strings.HasPrefix(alt.MIMEType, "text/") && !strings.Contains(alt.Content, text) {
			return false
		}
	}
	return true
}

// FromEmail returns the sender: From, or the DefaultFromEmail setting.
func (m *Message) FromEmail() string {
	if m.From != "" {
		return m.From
	}
	return CurrentConfig().DefaultFromEmail
}

// Send sends the message through its Connection, or through a new
// connection made with opts, and returns the number of messages sent. A
// message without recipients is not sent.
func (m *Message) Send(opts ...Option) (int, error) {
	if len(m.Recipients()) == 0 {
		return 0, nil
	}
	if m.Connection == nil {
		conn, err := NewConnection(opts...)
		if err != nil {
			return 0, err
		}
		m.Connection = conn
	}
	return m.Connection.SendMessages([]*Message{m})
}

// Bytes returns the message as Django's message().as_bytes() does, with
// "\n" line endings. Each call makes a new Date, Message-ID and multipart
// boundary.
func (m *Message) Bytes() ([]byte, error) {
	return m.render(policyDefault)
}

// SMTPBytes returns the message with "\r\n" line endings, as the SMTP
// backend sends it.
func (m *Message) SMTPBytes() ([]byte, error) {
	return m.render(policySMTP)
}

// render builds and generates the message.
func (m *Message) render(p policy) ([]byte, error) {
	e, err := m.build(p)
	if err != nil {
		return nil, err
	}
	return e.generate(p)
}

// encoding returns the charset of the text parts.
func (m *Message) encoding(cfg Config) string {
	if m.Encoding != "" {
		return m.Encoding
	}
	return cfg.DefaultCharset
}

// header returns the value of the extra header with exactly the given name.
func (m *Message) header(name string) (string, bool) {
	for _, h := range m.Headers {
		if h.Name == name {
			return h.Value, true
		}
	}
	return "", false
}

// build returns the MIME entity of the message, as Django's message() does.
func (m *Message) build(p policy) (*entity, error) {
	cfg := CurrentConfig()
	msg := &entity{emailMessage: true}
	if err := m.addBodies(msg, cfg, p); err != nil {
		return nil, err
	}
	if err := m.addAttachments(msg, cfg, p); err != nil {
		return nil, err
	}
	if err := msg.set("Subject", m.Subject); err != nil {
		return nil, err
	}
	from, ok := m.header("From")
	if !ok {
		from = m.FromEmail()
	}
	if err := msg.set("From", from); err != nil {
		return nil, err
	}
	for _, list := range []struct {
		name   string
		values []string
	}{{"To", m.To}, {"Cc", m.Cc}, {"Reply-To", m.ReplyTo}} {
		value, ok := m.header(list.name)
		if !ok {
			if len(list.values) == 0 {
				continue
			}
			value = strings.Join(list.values, ", ")
		}
		if err := msg.set(list.name, value); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(m.Headers))
	for _, h := range m.Headers {
		names = append(names, strings.ToLower(h.Name))
	}
	if !slices.Contains(names, "date") {
		date := now().UTC()
		if cfg.UseLocaltime {
			date = date.In(cfg.location())
		}
		if err := msg.set("Date", formatPyDate(date, false)); err != nil {
			return nil, err
		}
	}
	if !slices.Contains(names, "message-id") {
		if err := msg.set("Message-ID", MakeMsgID("", DNSName())); err != nil {
			return nil, err
		}
	}
	for i, h := range m.Headers {
		switch names[i] {
		case "from", "to", "cc", "reply-to":
			continue
		}
		if err := msg.set(h.Name, h.Value); err != nil {
			return nil, err
		}
	}
	if err := idnaEncodeAddressHeaders(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// now returns the current time. Tests replace it.
var now = time.Now

// addBodies sets the body and the alternatives.
func (m *Message) addBodies(msg *entity, cfg Config, p policy) error {
	encoding := m.encoding(cfg)
	if (m.Body != "" || len(m.Alternatives) == 0) && (m.Body != "" || len(m.Attachments) == 0) {
		subtype := m.ContentSubtype
		if subtype == "" {
			subtype = "plain"
		}
		if err := msg.setTextContent(m.Body, subtype, encoding, contentOptions{}, p); err != nil {
			return err
		}
	}
	if len(m.Alternatives) == 0 {
		return nil
	}
	if err := msg.makeAlternative(); err != nil {
		return err
	}
	for _, alt := range m.Alternatives {
		maintype, subtype, _ := strings.Cut(alt.MIMEType, "/")
		part := &entity{emailMessage: true}
		var err error
		if maintype == "text" {
			if !utf8.ValidString(alt.Content) {
				return fmt.Errorf("'utf-8' codec can't decode the %s alternative", alt.MIMEType)
			}
			err = part.setTextContent(alt.Content, subtype, encoding, contentOptions{}, p)
		} else {
			err = part.setBytesContent([]byte(alt.Content), maintype, subtype, contentOptions{})
		}
		if err != nil {
			return err
		}
		if err := msg.addPart("alternative", part, ""); err != nil {
			return err
		}
	}
	return nil
}

// addAttachments adds the attachments in a multipart/mixed entity.
func (m *Message) addAttachments(msg *entity, cfg Config, p policy) error {
	if len(m.Attachments) == 0 {
		return nil
	}
	if err := msg.makeMixed(); err != nil {
		return err
	}
	for _, a := range m.Attachments {
		if a.Part != nil {
			part, err := a.Part.entity(p)
			if err != nil {
				return err
			}
			msg.parts = append(msg.parts, part)
			continue
		}
		part, err := m.attachmentEntity(a, cfg, p)
		if err != nil {
			return err
		}
		if err := msg.addPart("mixed", part, "attachment"); err != nil {
			return err
		}
	}
	return nil
}

// attachmentEntity returns the MIME part of an attachment, as Django's
// _add_attachment and Python's add_attachment build it.
func (m *Message) attachmentEntity(a Attachment, cfg Config, p policy) (*entity, error) {
	mimeType := a.MIMEType
	if mimeType == "" {
		mimeType = GuessType(a.Filename)
	}
	if mimeType == "" {
		mimeType = DefaultAttachmentMIMEType
	}
	maintype, subtype, _ := strings.Cut(mimeType, "/")
	if maintype == "text" && a.Message == nil && !utf8.Valid(a.Content) {
		maintype, subtype, _ = strings.Cut(DefaultAttachmentMIMEType, "/")
	}
	o := contentOptions{filename: a.Filename, hasFilename: a.Filename != ""}
	part := &entity{emailMessage: true}
	switch maintype {
	case "text":
		return part, part.setTextContent(string(a.Content), subtype, m.encoding(cfg), o, p)
	case "message":
		var inner *entity
		if a.Message != nil {
			var err error
			if inner, err = a.Message.build(p); err != nil {
				return nil, err
			}
		} else {
			inner = parseRawMessage(a.Content)
		}
		return part, part.setMessageContent(inner, subtype, o)
	}
	return part, part.setBytesContent(a.Content, maintype, subtype, o)
}

// entity returns the MIME entity of the part. It has no MIME-Version
// header, as a Python MIMEPart has none.
func (p *Part) entity(pol policy) (*entity, error) {
	mimeType := p.MIMEType
	if mimeType == "" {
		mimeType = "text/plain"
	}
	maintype, subtype, _ := strings.Cut(mimeType, "/")
	o := contentOptions{disposition: p.Disposition, filename: p.Filename, hasFilename: p.Filename != "", cid: p.ContentID}
	e := &entity{}
	var err error
	if maintype == "text" {
		charset := p.Charset
		if charset == "" {
			charset = "utf-8"
		}
		err = e.setTextContent(string(p.Content), subtype, charset, o, pol)
	} else {
		err = e.setBytesContent(p.Content, maintype, subtype, o)
	}
	if err != nil {
		return nil, err
	}
	for _, h := range p.Headers {
		if err := e.set(h.Name, h.Value); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// parseRawMessage reads a message from bytes, as Python's
// message_from_bytes does for the headers. The body stays as it is and is
// written back with the line breaks of the policy; a multipart body keeps
// its parts and boundaries as they are.
func parseRawMessage(data []byte) *entity {
	e := &entity{emailMessage: true, raw: true}
	rest := string(data)
	var current []string
	flush := func() {
		if current == nil {
			return
		}
		name, value, _ := strings.Cut(current[0], ":")
		value = strings.TrimLeft(value+strings.Join(current[1:], ""), " \t\r\n")
		e.headers = append(e.headers, &mimeHeader{name: name, isSource: true, source: strings.TrimRight(value, "\r\n")})
		current = nil
	}
	for rest != "" {
		line := rest
		if i := strings.IndexAny(rest, "\r\n"); i >= 0 {
			end := i + 1
			if rest[i] == '\r' && strings.HasPrefix(rest[i:], "\r\n") {
				end++
			}
			line = rest[:end]
		}
		content := strings.TrimRight(line, "\r\n")
		switch {
		case content == "":
			rest = rest[len(line):]
			flush()
			e.text, e.hasText = rest, true
			return e
		case content[0] == ' ' || content[0] == '\t':
			if current != nil {
				current = append(current, line)
			}
		case isHeaderLine(content):
			flush()
			if !strings.HasPrefix(content, ":") {
				current = []string{line}
			}
		default:
			flush()
			e.text, e.hasText = rest, true
			return e
		}
		rest = rest[len(line):]
	}
	flush()
	return e
}

// isHeaderLine reports whether line starts a header field, as the header
// pattern of Python's feed parser decides.
func isHeaderLine(line string) bool {
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == ':' {
			return true
		}
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return false
}

// MakeMsgID returns a new Message-ID value, as Python's make_msgid does. An
// empty domain means the fully qualified name of this host.
func MakeMsgID(idstring, domain string) string {
	if idstring != "" {
		idstring = "." + idstring
	}
	if domain == "" {
		domain = DNSName()
	}
	t := now()
	timeval := t.Unix()*100 + int64(t.Nanosecond()/1e7)
	return fmt.Sprintf("<%d.%d.%d%s@%s>", timeval, os.Getpid(), rand.Uint64(), idstring, domain)
}

// idnaEncodeAddressHeaders writes the non-ASCII domains of the address
// headers in punycode, as Django's _idna_encode_address_header_domains
// does.
func idnaEncodeAddressHeaders(msg *entity) error {
	for _, h := range slices.Clone(msg.headers) {
		if h.isMedia || h.isSource || headerKindOf(h.name) != kindAddress || h.tree == nil {
			continue
		}
		mailboxes := h.tree.allMailboxes()
		if !slices.ContainsFunc(mailboxes, func(mb *tok) bool {
			d, _ := mb.domain()
			return !isASCII(d)
		}) {
			continue
		}
		addrs := make([]string, 0, len(mailboxes))
		for _, mb := range mailboxes {
			display, _ := mb.displayName()
			local, _ := mb.localPart()
			domain, _ := mb.domain()
			encoded, err := punycode(domain)
			if err != nil {
				return err
			}
			addrs = append(addrs, addressString(display, local, encoded))
		}
		tree := parseAddressList(strings.Join(addrs, ", "))
		for _, target := range msg.headers {
			if strings.EqualFold(target.name, h.name) {
				target.tree = tree
				break
			}
		}
	}
	return nil
}

// addressString formats a mailbox, as str() of Python's
// headerregistry.Address does.
func addressString(display, username, domain string) string {
	if strings.ContainsAny(display, specialsChars) {
		display = quoteString(display)
	}
	local := username
	if strings.ContainsAny(local, dotAtomEnds) {
		local = quoteString(local)
	}
	spec := local
	switch {
	case domain != "":
		spec = local + "@" + domain
	case local == "":
		spec = "<>"
	}
	if display == "" {
		return spec
	}
	if spec == "<>" {
		spec = ""
	}
	return display + " <" + spec + ">"
}
