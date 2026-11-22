package mail

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The code below ports the client side of Python's smtplib that Django's
// SMTP backend uses: the greeting, EHLO and HELO, STARTTLS, AUTH with
// CRAM-MD5, PLAIN and LOGIN, and the MAIL, RCPT and DATA commands.

// SMTPError is a failure of the SMTP conversation, as the exceptions of
// Python's smtplib. A backend with FailSilently skips these errors when it
// sends.
type SMTPError struct {
	// Kind is the name of the smtplib exception, such as
	// "SMTPSenderRefused" or "SMTPServerDisconnected".
	Kind string
	// Code and Message hold the reply of the server, if there was one.
	// Code is -1 for a reply without a valid code.
	Code    int
	Message string
	// Refused holds the reply to each refused recipient, for
	// "SMTPRecipientsRefused".
	Refused map[string]SMTPReply
}

// SMTPReply is the reply of the server to a command.
type SMTPReply struct {
	Code    int
	Message string
}

// Error returns the kind of the error and the reply.
func (e *SMTPError) Error() string {
	switch {
	case e.Refused != nil:
		return fmt.Sprintf("%s: %v", e.Kind, e.Refused)
	case e.Code != 0:
		return fmt.Sprintf("%s: (%d, b%s)", e.Kind, e.Code, pyRepr(e.Message))
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// smtpMaxLine is the longest reply line smtplib accepts.
const smtpMaxLine = 8192

// smtpMaxChallenge is the number of AUTH challenges after which smtplib
// gives up.
const smtpMaxChallenge = 5

// smtpClient is a connection to an SMTP server, as smtplib.SMTP.
type smtpClient struct {
	conn          net.Conn
	r             *bufio.Reader
	host          string
	localHostname string
	timeout       time.Duration
	// greeted is set once EHLO or HELO got a reply.
	greeted   bool
	doesESMTP bool
	features  map[string]string
	// challenges counts the AUTH challenges of the current login.
	challenges int
}

// dialSMTP connects to the server and reads its greeting. With tlsConfig
// the connection uses TLS from the start, as smtplib.SMTP_SSL.
func dialSMTP(host string, port int, localHostname string, timeout time.Duration, tlsConfig *tls.Config) (*smtpClient, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	c := &smtpClient{conn: conn, host: host, localHostname: localHostname, timeout: timeout, features: map[string]string{}}
	if tlsConfig != nil {
		if err := c.handshake(tlsConfig); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	c.r = bufio.NewReader(c.conn)
	code, msg, err := c.getreply()
	if err != nil {
		return nil, err
	}
	if code != 220 {
		c.close()
		return nil, &SMTPError{Kind: "SMTPConnectError", Code: code, Message: msg}
	}
	return c, nil
}

// handshake wraps the connection in TLS.
func (c *smtpClient) handshake(cfg *tls.Config) error {
	tc := tls.Client(c.conn, cfg)
	c.deadline()
	if err := tc.Handshake(); err != nil {
		return err
	}
	c.conn = tc
	return nil
}

// deadline limits the next network operation to the timeout.
func (c *smtpClient) deadline() {
	if c.timeout > 0 {
		_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
	}
}

// close closes the connection.
func (c *smtpClient) close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// send writes raw data.
func (c *smtpClient) send(data []byte) error {
	if c.conn == nil {
		return &SMTPError{Kind: "SMTPServerDisconnected", Message: "please run connect() first"}
	}
	c.deadline()
	if _, err := c.conn.Write(data); err != nil {
		c.close()
		return &SMTPError{Kind: "SMTPServerDisconnected", Message: "Server not connected"}
	}
	return nil
}

// putcmd sends a command line.
func (c *smtpClient) putcmd(cmd, args string) error {
	s := cmd
	if args != "" {
		s = cmd + " " + args
	}
	if strings.ContainsAny(s, "\r\n") {
		s = strings.NewReplacer("\n", `\n`, "\r", `\r`).Replace(s)
		return &inputError{"command and arguments contain prohibited newline characters: " + s}
	}
	if !isASCII(s) {
		return &inputError{"'ascii' codec can't encode the command " + pyRepr(s)}
	}
	return c.send([]byte(s + "\r\n"))
}

// getreply reads a reply, joining the lines of a multiline reply with
// "\n".
func (c *smtpClient) getreply() (int, string, error) {
	if c.conn == nil {
		return 0, "", &SMTPError{Kind: "SMTPServerDisconnected", Message: "please run connect() first"}
	}
	var resp []string
	code := -1
	for {
		c.deadline()
		line, err := c.r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			rest, err2 := c.r.ReadBytes('\n')
			line, err = append(append([]byte(nil), line...), rest...), err2
		}
		if err != nil && (!errors.Is(err, io.EOF) || len(line) == 0) {
			c.close()
			if errors.Is(err, io.EOF) {
				return 0, "", &SMTPError{Kind: "SMTPServerDisconnected", Message: "Connection unexpectedly closed"}
			}
			return 0, "", &SMTPError{Kind: "SMTPServerDisconnected", Message: "Connection unexpectedly closed: " + err.Error()}
		}
		if len(line) > smtpMaxLine {
			c.close()
			return 0, "", &SMTPError{Kind: "SMTPResponseException", Code: 500, Message: "Line too long."}
		}
		text := ""
		if len(line) > 4 {
			text = strings.Trim(string(line[4:]), " \t\r\n")
		}
		resp = append(resp, text)
		n, err := pyInt(string(line[:min(3, len(line))]))
		if err != nil {
			code = -1
			break
		}
		code = n
		if len(line) < 4 || line[3] != '-' {
			break
		}
	}
	return code, strings.Join(resp, "\n"), nil
}

// docmd sends a command and reads the reply.
func (c *smtpClient) docmd(cmd, args string) (int, string, error) {
	if err := c.putcmd(cmd, args); err != nil {
		return 0, "", err
	}
	return c.getreply()
}

// oldstyleAuth matches the AUTH= lines of old servers.
var oldstyleAuth = regexp.MustCompile(`(?i)^auth=(.*)`)

// ehloFeature matches a line of the EHLO reply.
var ehloFeature = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9\-]*) ?`)

// ehlo sends EHLO and records the extensions of the server.
func (c *smtpClient) ehlo() (int, string, error) {
	c.features = map[string]string{}
	code, msg, err := c.docmd("ehlo", c.localHostname)
	if err != nil {
		return 0, "", err
	}
	if code == -1 && msg == "" {
		c.close()
		return 0, "", &SMTPError{Kind: "SMTPServerDisconnected", Message: "Server not connected"}
	}
	c.greeted = true
	if code != 250 {
		return code, msg, nil
	}
	c.doesESMTP = true
	lines := strings.Split(msg, "\n")
	for _, line := range lines[1:] {
		if m := oldstyleAuth.FindStringSubmatch(line); m != nil {
			c.features["auth"] += " " + m[1]
			continue
		}
		m := ehloFeature.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		feature := strings.ToLower(line[m[2]:m[3]])
		params := strings.TrimFunc(line[m[3]:], pyIsSpace)
		if feature == "auth" {
			c.features[feature] += " " + params
		} else {
			c.features[feature] = params
		}
	}
	return code, msg, nil
}

// helo sends HELO.
func (c *smtpClient) helo() (int, string, error) {
	code, msg, err := c.docmd("helo", c.localHostname)
	if err == nil {
		c.greeted = true
	}
	return code, msg, err
}

// greet sends EHLO, or HELO when EHLO fails, unless one was sent.
func (c *smtpClient) greet() error {
	if c.greeted {
		return nil
	}
	code, _, err := c.ehlo()
	if err != nil {
		return err
	}
	if code >= 200 && code <= 299 {
		return nil
	}
	code, msg, err := c.helo()
	if err != nil {
		return err
	}
	if code < 200 || code > 299 {
		return &SMTPError{Kind: "SMTPHeloError", Code: code, Message: msg}
	}
	return nil
}

// hasExtn reports whether the server announced the extension.
func (c *smtpClient) hasExtn(name string) bool {
	_, ok := c.features[strings.ToLower(name)]
	return ok
}

// starttls switches the connection to TLS.
func (c *smtpClient) starttls(cfg *tls.Config) error {
	if err := c.greet(); err != nil {
		return err
	}
	if !c.hasExtn("starttls") {
		return &SMTPError{Kind: "SMTPNotSupportedError", Message: "STARTTLS extension not supported by server."}
	}
	code, reply, err := c.docmd("STARTTLS", "")
	if err != nil {
		return err
	}
	if code != 220 {
		return &SMTPError{Kind: "SMTPResponseException", Code: code, Message: reply}
	}
	if err := c.handshake(cfg); err != nil {
		return err
	}
	c.r = bufio.NewReader(c.conn)
	c.greeted, c.doesESMTP, c.features = false, false, map[string]string{}
	return nil
}

// login authenticates with the first of CRAM-MD5, PLAIN and LOGIN that the
// server announces and accepts.
func (c *smtpClient) login(user, password string) error {
	if err := c.greet(); err != nil {
		return err
	}
	if !c.hasExtn("auth") {
		return &SMTPError{Kind: "SMTPNotSupportedError", Message: "SMTP AUTH extension not supported by server."}
	}
	advertised := strings.Fields(c.features["auth"])
	var methods []string
	for _, m := range []string{"CRAM-MD5", "PLAIN", "LOGIN"} {
		for _, a := range advertised {
			if a == m {
				methods = append(methods, m)
				break
			}
		}
	}
	if len(methods) == 0 {
		return &SMTPError{Kind: "SMTPException", Message: "No suitable authentication method found."}
	}
	var last error
	for _, m := range methods {
		err := c.auth(m, c.authObject(m, user, password))
		if err == nil {
			return nil
		}
		var se *SMTPError
		if !errors.As(err, &se) || se.Kind != "SMTPAuthenticationError" {
			return err
		}
		last = err
	}
	return last
}

// authObject returns the responses of an AUTH mechanism. A nil challenge
// asks for the initial response, which is nil when the mechanism has none.
func (c *smtpClient) authObject(mechanism, user, password string) func([]byte) *string {
	return func(challenge []byte) *string {
		var s string
		switch mechanism {
		case "CRAM-MD5":
			if challenge == nil {
				return nil
			}
			mac := hmac.New(md5.New, []byte(password))
			mac.Write(challenge)
			s = user + " " + hex.EncodeToString(mac.Sum(nil))
		case "PLAIN":
			s = "\x00" + user + "\x00" + password
		default:
			s = password
			if challenge == nil || c.challenges < 2 {
				s = user
			}
		}
		return &s
	}
}

// auth runs one AUTH exchange.
func (c *smtpClient) auth(mechanism string, object func([]byte) *string) error {
	var code int
	var resp string
	var err error
	if initial := object(nil); initial != nil {
		if !isASCII(*initial) {
			return &inputError{"'ascii' codec can't encode the credentials of AUTH " + mechanism}
		}
		code, resp, err = c.docmd("AUTH", mechanism+" "+base64.StdEncoding.EncodeToString([]byte(*initial)))
		c.challenges = 1
	} else {
		code, resp, err = c.docmd("AUTH", mechanism)
		c.challenges = 0
	}
	for err == nil && code == 334 {
		c.challenges++
		challenge, _ := base64.StdEncoding.DecodeString(strings.Map(func(r rune) rune {
			if strings.ContainsRune(" \t\r\n", r) {
				return -1
			}
			return r
		}, resp))
		if challenge == nil {
			challenge = []byte{}
		}
		answer := object(challenge)
		if !isASCII(*answer) {
			return &inputError{"'ascii' codec can't encode the credentials of AUTH " + mechanism}
		}
		code, resp, err = c.docmd(base64.StdEncoding.EncodeToString([]byte(*answer)), "")
		if err == nil && c.challenges > smtpMaxChallenge {
			return &SMTPError{Kind: "SMTPException", Message: fmt.Sprintf("Server AUTH mechanism infinite loop. Last response: (%d, b%s)", code, pyRepr(resp))}
		}
	}
	if err != nil {
		return err
	}
	if code == 235 || code == 503 {
		return nil
	}
	return &SMTPError{Kind: "SMTPAuthenticationError", Code: code, Message: resp}
}

// quoteAddr puts an address in angle brackets for MAIL and RCPT.
func quoteAddr(addr string) string {
	if strings.HasPrefix(strings.TrimSpace(addr), "<") {
		return addr
	}
	return "<" + addr + ">"
}

// rset sends RSET and ignores a closed connection.
func (c *smtpClient) rset() {
	_, _, _ = c.docmd("rset", "")
}

// sendmail sends one message to the recipients. It succeeds when the server
// accepts at least one recipient.
func (c *smtpClient) sendmail(from string, to []string, msg []byte) error {
	if err := c.greet(); err != nil {
		return err
	}
	options := ""
	if c.doesESMTP && c.hasExtn("size") {
		options = fmt.Sprintf(" size=%d", len(msg))
	}
	code, resp, err := c.docmd("mail", "FROM:"+quoteAddr(from)+options)
	if err != nil {
		return err
	}
	if code != 250 {
		if code == 421 {
			c.close()
		} else {
			c.rset()
		}
		return &SMTPError{Kind: "SMTPSenderRefused", Code: code, Message: resp}
	}
	refused := map[string]SMTPReply{}
	for _, rcpt := range to {
		code, resp, err := c.docmd("rcpt", "TO:"+quoteAddr(rcpt))
		if err != nil {
			return err
		}
		if code != 250 && code != 251 {
			refused[rcpt] = SMTPReply{code, resp}
		}
		if code == 421 {
			c.close()
			return &SMTPError{Kind: "SMTPRecipientsRefused", Refused: refused}
		}
	}
	if len(refused) == len(to) {
		c.rset()
		return &SMTPError{Kind: "SMTPRecipientsRefused", Refused: refused}
	}
	code, resp, err = c.data(msg)
	if err != nil {
		return err
	}
	if code != 250 {
		if code == 421 {
			c.close()
		} else {
			c.rset()
		}
		return &SMTPError{Kind: "SMTPDataError", Code: code, Message: resp}
	}
	return nil
}

// leadingDot finds the lines that start with a dot.
var leadingDot = regexp.MustCompile(`(?m)^\.`)

// data sends the message with DATA, doubling the dots that start a line.
func (c *smtpClient) data(msg []byte) (int, string, error) {
	code, resp, err := c.docmd("data", "")
	if err != nil {
		return 0, "", err
	}
	if code != 354 {
		return 0, "", &SMTPError{Kind: "SMTPDataError", Code: code, Message: resp}
	}
	q := leadingDot.ReplaceAll(msg, []byte(".."))
	if !bytes.HasSuffix(q, []byte("\r\n")) {
		q = append(q, "\r\n"...)
	}
	q = append(q, ".\r\n"...)
	if err := c.send(q); err != nil {
		return 0, "", err
	}
	return c.getreply()
}

// quit sends QUIT and closes the connection.
func (c *smtpClient) quit() error {
	_, _, err := c.docmd("quit", "")
	c.greeted, c.doesESMTP, c.features = false, false, map[string]string{}
	c.close()
	return err
}
