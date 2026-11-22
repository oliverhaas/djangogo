package mail

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// smtpServer is an SMTP server for the tests. It keeps the messages and
// envelopes it receives and refuses a message whose envelope sender is not
// the address of its From header, as the SMTPHandler of Django's tests does
// with aiosmtpd. It offers no AUTH and no STARTTLS unless the fields below
// turn them on.
type smtpServer struct {
	ln   net.Listener
	host string
	port int

	// auth lists the AUTH mechanisms the server offers.
	auth []string
	// user and password are the credentials that AUTH accepts.
	user, password string
	// tlsConfig turns on STARTTLS, or TLS from the start with implicitTLS.
	tlsConfig   *tls.Config
	implicitTLS bool

	wg sync.WaitGroup
	mu sync.Mutex
	// mailbox holds the messages with LF line endings.
	mailbox [][]byte
	// envelopes holds the envelope of each message in mailbox.
	envelopes []envelope
	// data holds each DATA payload as the client sent it, with the final
	// dot line.
	data [][]byte
	// logins holds the mechanism of each successful AUTH.
	logins []string
}

// envelope is the SMTP envelope of a message.
type envelope struct {
	mailFrom string
	rcptTos  []string
}

// startSMTPServer starts a server on a free local port; configure changes
// it before it accepts connections. The server stops when the test ends.
func startSMTPServer(t *testing.T, configure ...func(*smtpServer)) *smtpServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	s := &smtpServer{ln: ln, host: "127.0.0.1", port: addr.Port}
	for _, c := range configure {
		c(s)
	}
	s.wg.Add(1)
	go s.accept()
	t.Cleanup(s.stop)
	return s
}

// useSMTPServer makes the server the SMTP host and port of the settings for
// the rest of the test.
func useSMTPServer(t *testing.T, s *smtpServer) {
	t.Helper()
	withConfig(t, func(c *Config) { c.Host, c.Port = s.host, s.port })
}

// stop closes the listener and waits for the open sessions to end.
func (s *smtpServer) stop() {
	s.ln.Close()
	s.wg.Wait()
}

// accept serves each connection in its own goroutine.
func (s *smtpServer) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serve(conn)
		}()
	}
}

// flush empties the mailbox.
func (s *smtpServer) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mailbox, s.envelopes, s.data, s.logins = nil, nil, nil, nil
}

// messages returns the messages received so far.
func (s *smtpServer) messages() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.mailbox)
}

// receivedEnvelopes returns the envelopes received so far.
func (s *smtpServer) receivedEnvelopes() []envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.envelopes)
}

// serve runs one SMTP session.
func (s *smtpServer) serve(conn net.Conn) {
	defer func() { conn.Close() }()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if s.implicitTLS {
		conn = tls.Server(conn, s.tlsConfig)
	}
	r := bufio.NewReader(conn)
	write := func(lines ...string) bool {
		_, err := io.WriteString(conn, strings.Join(lines, "\r\n")+"\r\n")
		return err == nil
	}
	readLine := func() (string, bool) {
		line, err := r.ReadString('\n')
		return strings.TrimRight(line, "\r\n"), err == nil
	}
	if !write("220 " + s.host + " djangogo test SMTP") {
		return
	}
	var env envelope
	secure := s.implicitTLS
	for {
		line, ok := readLine()
		if !ok {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			lines := []string{"250-" + s.host, "250-SIZE 33554432", "250-8BITMIME", "250-SMTPUTF8"}
			if s.tlsConfig != nil && !secure {
				lines = append(lines, "250-STARTTLS")
			}
			if len(s.auth) > 0 {
				lines = append(lines, "250-AUTH "+strings.Join(s.auth, " "))
			}
			write(append(lines, "250 HELP")...)
		case "HELO":
			write("250 " + s.host)
		case "STARTTLS":
			if s.tlsConfig == nil || secure {
				write("454 TLS not available")
				continue
			}
			write("220 Ready to start TLS")
			tc := tls.Server(conn, s.tlsConfig)
			if tc.Handshake() != nil {
				return
			}
			conn, r, secure, env = tc, bufio.NewReader(tc), true, envelope{}
		case "AUTH":
			mechanism, initial, _ := strings.Cut(arg, " ")
			mechanism = strings.ToUpper(mechanism)
			if !slices.Contains(s.auth, mechanism) {
				write("504 5.5.4 Unrecognized authentication type")
				continue
			}
			if s.authenticate(mechanism, initial, write, readLine) {
				s.mu.Lock()
				s.logins = append(s.logins, mechanism)
				s.mu.Unlock()
				write("235 2.7.0 Authentication successful")
			} else {
				write("535 5.7.8 Authentication credentials invalid")
			}
		case "MAIL":
			env = envelope{mailFrom: angleAddress(arg)}
			write("250 OK")
		case "RCPT":
			env.rcptTos = append(env.rcptTos, angleAddress(arg))
			write("250 OK")
		case "DATA":
			write("354 End data with <CR><LF>.<CR><LF>")
			var raw, content bytes.Buffer
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				raw.WriteString(l)
				if l == ".\r\n" {
					break
				}
				content.WriteString(strings.TrimPrefix(l, "."))
			}
			write(s.deliver(env, raw.Bytes(), content.Bytes()))
			env = envelope{}
		case "RSET":
			env = envelope{}
			write("250 OK")
		case "NOOP":
			write("250 OK")
		case "QUIT":
			write("221 Bye")
			return
		default:
			write(`500 Error: command "` + verb + `" not recognized`)
		}
	}
}

// authenticate runs the AUTH exchange of one mechanism and reports whether
// the credentials match.
func (s *smtpServer) authenticate(mechanism, initial string, write func(...string) bool, readLine func() (string, bool)) bool {
	decode := func(v string) string {
		data, _ := base64.StdEncoding.DecodeString(v)
		return string(data)
	}
	ask := func(prompt string) string {
		write("334 " + base64.StdEncoding.EncodeToString([]byte(prompt)))
		line, _ := readLine()
		return decode(line)
	}
	switch mechanism {
	case "PLAIN":
		resp := decode(initial)
		if initial == "" {
			resp = ask("")
		}
		parts := strings.Split(resp, "\x00")
		return len(parts) == 3 && parts[1] == s.user && parts[2] == s.password
	case "LOGIN":
		user := decode(initial)
		if initial == "" {
			user = ask("Username:")
		}
		return user == s.user && ask("Password:") == s.password
	case "CRAM-MD5":
		challenge := "<1896.697170952@127.0.0.1>"
		user, digest, _ := strings.Cut(ask(challenge), " ")
		mac := hmac.New(md5.New, []byte(s.password))
		mac.Write([]byte(challenge))
		return user == s.user && digest == hex.EncodeToString(mac.Sum(nil))
	}
	return false
}

// deliver keeps a message, unless its envelope sender is not the address of
// its From header, and returns the reply to DATA.
func (s *smtpServer) deliver(env envelope, raw, content []byte) string {
	message := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	if from := headerFromAddress(message); env.mailFrom != from {
		return fmt.Sprintf("553 '%s' != '%s'", env.mailFrom, from)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mailbox = append(s.mailbox, message)
	s.envelopes = append(s.envelopes, env)
	s.data = append(s.data, raw)
	return "250 OK"
}

// angleAddress returns the address between the angle brackets of a MAIL or
// RCPT argument.
func angleAddress(arg string) string {
	_, rest, _ := strings.Cut(arg, "<")
	addr, _, _ := strings.Cut(rest, ">")
	return addr
}

// headerFromAddress returns the addr-spec of the first address of the From
// header, or "" when there is none. A value without "@" is a local mailbox.
func headerFromAddress(message []byte) string {
	head, _, _ := strings.Cut(string(message), "\n\n")
	var value string
	found := false
	for line := range strings.SplitSeq(head, "\n") {
		if found && line != "" && (line[0] == ' ' || line[0] == '\t') {
			value += line
			continue
		}
		if found {
			break
		}
		name, v, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(name, "From") {
			value, found = strings.TrimSpace(v), true
		}
	}
	if !found {
		return ""
	}
	parser := mail.AddressParser{WordDecoder: &mime.WordDecoder{CharsetReader: charsetReader}}
	list, err := parser.ParseList(value)
	if err != nil {
		return value
	}
	if len(list) == 0 {
		return ""
	}
	return list[0].Address
}

// testCertificate returns a self-signed certificate for 127.0.0.1 and a
// pool that trusts it.
func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}, pool
}
