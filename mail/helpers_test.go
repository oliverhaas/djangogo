package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"slices"
	"strings"
	"testing"

	"golang.org/x/text/encoding/htmlindex"
)

// TestMain makes the locmem backend the default, as Django's test runner does
// with EMAIL_BACKEND.
func TestMain(m *testing.M) {
	cfg := DefaultConfig()
	cfg.Backend = "locmem"
	Configure(cfg)
	os.Exit(m.Run())
}

// withConfig applies change to the active configuration for the rest of the
// test, as Django's override_settings does for the email settings.
func withConfig(t *testing.T, change func(*Config)) {
	t.Helper()
	cfg := CurrentConfig()
	change(&cfg)
	t.Cleanup(Configure(cfg))
}

// part is a MIME entity of a generated message, parsed back the way Python's
// email parser reads it in Django's tests.
type part struct {
	// headers holds the fields in order, unfolded, with raw values.
	headers []field
	// body is the raw body, after the blank line that ends the headers.
	body []byte
	// parts holds the parts of a multipart entity, or the one message of a
	// message/rfc822 entity.
	parts []*part
}

// field is one header field of a part.
type field struct{ name, value string }

// parse reads a generated message. It accepts CRLF and LF line endings.
func parse(t *testing.T, raw []byte) *part {
	t.Helper()
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	head, body, found := strings.Cut(text, "\n\n")
	if !found {
		head, body = strings.TrimSuffix(text, "\n"), ""
	}
	p := &part{body: []byte(body)}
	for line := range strings.SplitSeq(head, "\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(p.headers) == 0 {
				t.Fatalf("continuation line before the first header: %q", line)
			}
			p.headers[len(p.headers)-1].value += line
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("header line without a colon: %q", line)
		}
		p.headers = append(p.headers, field{name, strings.TrimLeft(value, " \t")})
	}
	switch p.contentType() {
	case "message/rfc822":
		p.parts = []*part{parse(t, p.body)}
	default:
		if strings.HasPrefix(p.contentType(), "multipart/") {
			boundary := p.param("Content-Type", "boundary")
			for _, sub := range splitMultipart(string(p.body), boundary) {
				p.parts = append(p.parts, parse(t, []byte(sub)))
			}
		}
	}
	return p
}

// splitMultipart returns the body parts of a multipart body. The line break
// before a delimiter belongs to the delimiter.
func splitMultipart(body, boundary string) []string {
	delimiter := "--" + boundary
	var parts []string
	var current *strings.Builder
	for line := range strings.SplitAfterSeq(body, "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if trimmed == delimiter || trimmed == delimiter+"--" {
			if current != nil {
				parts = append(parts, strings.TrimSuffix(current.String(), "\n"))
			}
			if trimmed == delimiter+"--" {
				break
			}
			current = new(strings.Builder)
			continue
		}
		if current != nil {
			current.WriteString(line)
		}
	}
	return parts
}

// raw returns the raw value of the first header called name, matched
// case-insensitively, and whether there is one.
func (p *part) raw(name string) (string, bool) {
	for _, f := range p.headers {
		if strings.EqualFold(f.name, name) {
			return f.value, true
		}
	}
	return "", false
}

// get returns the decoded value of the first header called name, as
// Python's message[name] does, or "" when there is none.
func (p *part) get(name string) string {
	v, _ := p.raw(name)
	return decodeWords(v)
}

// has reports whether the part has a header called name.
func (p *part) has(name string) bool {
	_, ok := p.raw(name)
	return ok
}

// getAll returns the decoded values of all headers called name.
func (p *part) getAll(name string) []string {
	var values []string
	for _, f := range p.headers {
		if strings.EqualFold(f.name, name) {
			values = append(values, decodeWords(f.value))
		}
	}
	return values
}

// items returns the headers as name and decoded value pairs, as Python's
// message.items() does.
func (p *part) items() []field {
	items := make([]field, 0, len(p.headers))
	for _, f := range p.headers {
		items = append(items, field{f.name, decodeWords(f.value)})
	}
	return items
}

// decodeWords decodes the RFC 2047 encoded-words of a header value.
func decodeWords(v string) string {
	d := mime.WordDecoder{CharsetReader: charsetReader}
	decoded, err := d.DecodeHeader(v)
	if err != nil {
		return v
	}
	return decoded
}

// charsetReader decodes the charsets the tests use in headers.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return nil, err
	}
	return enc.NewDecoder().Reader(input), nil
}

// contentType returns the lowercased media type, text/plain when there is
// no Content-Type header.
func (p *part) contentType() string {
	v, ok := p.raw("Content-Type")
	if !ok {
		return "text/plain"
	}
	mediaType, _, err := mime.ParseMediaType(v)
	if err != nil {
		return "text/plain"
	}
	return mediaType
}

// param returns the parameter name of the header called header.
func (p *part) param(header, name string) string {
	v, _ := p.raw(header)
	_, params, err := mime.ParseMediaType(v)
	if err != nil {
		return ""
	}
	return params[name]
}

// isMultipart reports whether the payload is a list of parts, as Python's
// is_multipart() does for multipart and message/rfc822 entities.
func (p *part) isMultipart() bool { return p.parts != nil }

// payload returns the raw payload as a string, as Python's get_payload()
// does for a part that is not multipart.
func (p *part) payload() string { return string(p.body) }

// decoded returns the payload with its Content-Transfer-Encoding undone.
func (p *part) decoded(t *testing.T) []byte {
	t.Helper()
	cte, _ := p.raw("Content-Transfer-Encoding")
	switch strings.ToLower(cte) {
	case "base64":
		data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(p.body)), ""))
		if err != nil {
			t.Fatalf("decode base64: %v", err)
		}
		return data
	case "quoted-printable":
		data, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(p.body)))
		if err != nil {
			t.Fatalf("decode quoted-printable: %v", err)
		}
		return data
	default:
		return p.body
	}
}

// text returns the decoded payload of a text part as a string, as Python's
// get_content() does.
func (p *part) text(t *testing.T) string {
	t.Helper()
	data := p.decoded(t)
	charset := p.param("Content-Type", "charset")
	if charset == "" || strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "us-ascii") {
		return string(data)
	}
	enc, err := htmlindex.Get(charset)
	if err != nil {
		t.Fatalf("charset %q: %v", charset, err)
	}
	s, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		t.Fatalf("decode %s: %v", charset, err)
	}
	return string(s)
}

// filename returns the filename parameter of Content-Disposition, or the
// name parameter of Content-Type, as Python's get_filename() does.
func (p *part) filename() string {
	if name := p.param("Content-Disposition", "filename"); name != "" {
		return name
	}
	return p.param("Content-Type", "name")
}

// disposition returns the lowercased disposition of the part, or "".
func (p *part) disposition() string {
	v, ok := p.raw("Content-Disposition")
	if !ok {
		return ""
	}
	disposition, _, err := mime.ParseMediaType(v)
	if err != nil {
		return ""
	}
	return disposition
}

// structure returns the indented content types of the part and its
// subparts, as MailTestsMixin.get_message_structure does.
func (p *part) structure(level int) string {
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", level*4) + p.contentType() + "\n")
	for _, sub := range p.parts {
		b.WriteString(sub.structure(level + 1))
	}
	return b.String()
}

// attachments returns the top-level attachments, as Python's
// iter_attachments() does: every part of a multipart entity that is not
// multipart/alternative, except the first body part that is not marked as
// an attachment.
func (p *part) attachments() []*part {
	if !strings.HasPrefix(p.contentType(), "multipart/") || p.contentType() == "multipart/alternative" {
		return nil
	}
	bodyTypes := []string{"text/plain", "text/html", "multipart/related", "multipart/alternative"}
	var out []*part
	seen := false
	for _, sub := range p.parts {
		if slices.Contains(bodyTypes, sub.contentType()) && sub.disposition() != "attachment" && !seen {
			seen = true
			continue
		}
		out = append(out, sub)
	}
	return out
}

// decodedAttachment is an attachment read back from a generated message,
// as MailTestsMixin.get_decoded_attachments returns it. Content holds the
// text of a text/* attachment and the bytes of any other.
type decodedAttachment struct {
	filename string
	content  string
	mimeType string
}

// rawAttachments returns the attachments of the generated message.
func rawAttachments(t *testing.T, m *Message) []*part {
	t.Helper()
	return parse(t, mustBytes(t, m)).attachments()
}

// decodedAttachments returns the attachments of the generated message.
func decodedAttachments(t *testing.T, m *Message) []decodedAttachment {
	t.Helper()
	raw := rawAttachments(t, m)
	out := make([]decodedAttachment, 0, len(raw))
	for _, a := range raw {
		content := string(a.decoded(t))
		if strings.HasPrefix(a.contentType(), "text/") {
			content = a.text(t)
		}
		out = append(out, decodedAttachment{a.filename(), content, a.contentType()})
	}
	return out
}

// mustBytes returns the generated message.
func mustBytes(t *testing.T, m *Message) []byte {
	t.Helper()
	data, err := m.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

// address is a parsed mailbox, as Python's headerregistry.Address compares.
type address struct {
	name     string
	addrSpec string
}

// addresses parses the address header called name, as Python's
// message[name].addresses does. A local part that is not a dot-atom is
// quoted, and a value without "@" is one mailbox with only a local part.
func (p *part) addresses(t *testing.T, name string) []address {
	t.Helper()
	v, ok := p.raw(name)
	if !ok {
		t.Fatalf("no %s header", name)
	}
	list, err := mail.ParseAddressList(v)
	if err != nil {
		if !strings.Contains(v, "@") {
			return []address{{"", v}}
		}
		t.Fatalf("parse %s header %q: %v", name, v, err)
	}
	out := make([]address, 0, len(list))
	for _, a := range list {
		at := strings.LastIndex(a.Address, "@")
		local, domain := a.Address[:at], a.Address[at:]
		if strings.ContainsAny(local, "()<>@,:;\\\"[] \t") {
			local = `"` + local + `"`
		}
		out = append(out, address{a.Name, local + domain})
	}
	return out
}

// hasHeaders fails unless the part has every header of want, by name and
// decoded value, as MailTestsMixin.assertMessageHasHeaders does.
func hasHeaders(t *testing.T, p *part, want ...field) {
	t.Helper()
	items := p.items()
	for _, w := range want {
		if !slices.Contains(items, w) {
			t.Errorf("missing header %s: %s; headers: %q", w.name, w.value, items)
		}
	}
}
