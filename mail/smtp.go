package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// SMTPBackend sends the messages to an SMTP server, as Django's SMTP
// backend does.
type SMTPBackend struct {
	mu   sync.Mutex
	opts Options
	conn *smtpClient
	// tlsConfig caches the TLS settings, as Django's ssl_context.
	tlsConfig *tls.Config
}

// errTLSAndSSL is the error for UseTLS and UseSSL together.
var errTLSAndSSL = errors.New("EMAIL_USE_TLS/EMAIL_USE_SSL are mutually exclusive, so only set one of those settings to True.") //nolint:revive,staticcheck // Django's message

// NewSMTPBackend returns an SMTP backend for the options. An empty host or a
// zero port means the Host or Port setting. UseTLS and UseSSL together are
// an error.
func NewSMTPBackend(o Options) (*SMTPBackend, error) {
	cfg := CurrentConfig()
	if o.Host == "" {
		o.Host = cfg.Host
	}
	if o.Port == 0 {
		o.Port = cfg.Port
	}
	if o.UseTLS && o.UseSSL {
		return nil, errTLSAndSSL
	}
	return &SMTPBackend{opts: o}, nil
}

// Options returns the options the backend uses.
func (b *SMTPBackend) Options() Options { return b.opts }

// Connected reports whether the backend holds an open connection.
func (b *SMTPBackend) Connected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conn != nil
}

// tlsSettings returns the TLS settings: the client certificate of SSLCertfile and
// SSLKeyfile, if one is set, and the server check of the system roots.
func (b *SMTPBackend) tlsSettings() (*tls.Config, error) {
	if b.tlsConfig != nil {
		return b.tlsConfig, nil
	}
	cfg := &tls.Config{ServerName: b.opts.Host}
	if b.opts.SSLCertfile != "" || b.opts.SSLKeyfile != "" {
		keyfile := b.opts.SSLKeyfile
		if keyfile == "" {
			keyfile = b.opts.SSLCertfile
		}
		cert, err := tls.LoadX509KeyPair(b.opts.SSLCertfile, keyfile)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	b.tlsConfig = cfg
	return cfg, nil
}

// Open connects to the server and logs in when Username and Password are
// set. It reports false when the connection is open already. With
// FailSilently, a failed connection reports false and no error.
func (b *SMTPBackend) Open() (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	created, _, err := b.open()
	return created, err
}

// open opens the connection; b.mu is held. ok is false when the
// connection failed and FailSilently skipped the error.
func (b *SMTPBackend) open() (created, ok bool, err error) {
	if b.conn != nil {
		return false, true, nil
	}
	var sslConfig *tls.Config
	if b.opts.UseSSL {
		// Django builds the SSL context before it connects, so a bad
		// certificate file is an error even with FailSilently.
		if sslConfig, err = b.tlsSettings(); err != nil {
			return false, false, err
		}
	}
	port := b.opts.Port
	if port == 0 {
		port = 25
		if b.opts.UseSSL {
			port = 465
		}
	}
	err = b.connect(port, sslConfig)
	if err != nil {
		if b.conn != nil {
			b.conn.close()
			b.conn = nil
		}
		if b.opts.FailSilently && isOSError(err) {
			return false, false, nil
		}
		return false, false, err
	}
	return true, true, nil
}

// connect makes the connection, switches to TLS and logs in.
func (b *SMTPBackend) connect(port int, sslConfig *tls.Config) error {
	c, err := dialSMTP(b.opts.Host, port, DNSName(), b.opts.Timeout, sslConfig)
	if err != nil {
		return err
	}
	b.conn = c
	if !b.opts.UseSSL && b.opts.UseTLS {
		cfg, err := b.tlsSettings()
		if err != nil {
			return err
		}
		if err := c.starttls(cfg); err != nil {
			return err
		}
	}
	if b.opts.Username != "" && b.opts.Password != "" {
		return c.login(b.opts.Username, b.opts.Password)
	}
	return nil
}

// isOSError reports whether Django's open() catches the error, which is an
// OSError in Python: an SMTP error, a network error or a TLS error. Errors
// from invalid input, such as a non-ASCII password, are no OSError.
func isOSError(err error) bool {
	var ie *inputError
	return !errors.As(err, &ie)
}

// inputError is an error from invalid input, which Python raises as a
// ValueError and Django passes on even with fail_silently.
type inputError struct{ msg string }

// Error returns the message.
func (e *inputError) Error() string { return e.msg }

// Close sends QUIT and closes the connection. With FailSilently, a failed
// QUIT reports no error.
func (b *SMTPBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.close()
}

// close closes the connection; b.mu is held.
func (b *SMTPBackend) close() error {
	if b.conn == nil {
		return nil
	}
	c := b.conn
	b.conn = nil
	err := c.quit()
	var se *SMTPError
	if err == nil || !errors.As(err, &se) || se.Kind == "SMTPServerDisconnected" {
		return nil
	}
	if b.opts.FailSilently {
		return nil
	}
	return err
}

// SendMessages sends the messages over the open connection, or over a new
// connection that it closes afterwards, and returns how many were sent. A
// message without recipients is not sent. With FailSilently, a message the
// server refuses is skipped.
func (b *SMTPBackend) SendMessages(messages []*Message) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	created, ok, err := b.open()
	if err != nil {
		return 0, err
	}
	if b.conn == nil || !ok {
		return 0, nil
	}
	sent := 0
	for _, m := range messages {
		var done bool
		if done, err = b.send(m); err != nil {
			break
		}
		if done {
			sent++
		}
	}
	if created {
		if cerr := b.close(); err == nil {
			err = cerr
		}
	}
	return sent, err
}

// send sends one message and reports whether it was sent.
func (b *SMTPBackend) send(m *Message) (bool, error) {
	recipients := m.Recipients()
	if len(recipients) == 0 {
		return false, nil
	}
	from, err := prepAddress(m.FromEmail(), true)
	if err != nil {
		return false, err
	}
	to := make([]string, len(recipients))
	for i, r := range recipients {
		if to[i], err = prepAddress(r, true); err != nil {
			return false, err
		}
	}
	data, err := m.SMTPBytes()
	if err != nil {
		return false, err
	}
	if err := b.conn.sendmail(from, to, data); err != nil {
		var se *SMTPError
		if b.opts.FailSilently && errors.As(err, &se) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PrepAddress returns the addr-spec of an address for the SMTP envelope, as
// the prep_address method of Django's SMTP backend does. A local mailbox
// without a domain is valid. With forceASCII, a non-ASCII domain is encoded
// with IDNA and a non-ASCII local part is an error. An address with defects,
// such as a line break, or more than one address is an error.
func PrepAddress(address string, forceASCII bool) (string, error) {
	return prepAddress(address, forceASCII)
}

// prepAddress implements PrepAddress.
func prepAddress(address string, forceASCII bool) (string, error) {
	parsed := parseAddressList(address)
	var defects []string
	seen := map[string]bool{}
	for _, d := range parsed.allDefects() {
		switch {
		case d.msg == "addr-spec local part with no domain":
		case !forceASCII && d.msg == "local-part contains non-ASCII characters)":
		case !seen[d.msg]:
			seen[d.msg] = true
			defects = append(defects, d.msg)
		}
	}
	if len(defects) > 0 {
		return "", &inputError{fmt.Sprintf("Invalid address %s: %s", pyRepr(address), strings.Join(defects, "; "))}
	}
	mailboxes := parsed.allMailboxes()
	if len(mailboxes) != 1 {
		return "", &inputError{fmt.Sprintf("Invalid address %s: must be a single address", pyRepr(address))}
	}
	mb := mailboxes[0]
	if domain, ok := mb.domain(); forceASCII && ok && domain != "" && !isASCII(domain) {
		encoded, err := punycode(domain)
		if err != nil {
			return "", &inputError{err.Error()}
		}
		local, _ := mb.localPart()
		return addressString("", local, encoded), nil
	}
	spec, _ := mb.addrSpec()
	return spec, nil
}
