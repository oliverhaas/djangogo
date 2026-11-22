package mail

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"sync"
	"time"
)

// Backend sends messages, as Django's email backends do.
type Backend interface {
	// Open makes the connection to the server, if the backend uses one. It
	// reports whether it made a new connection. Messages can be sent
	// without it: SendMessages then opens and closes a connection itself.
	Open() (bool, error)
	// Close ends the connection that Open made.
	Close() error
	// SendMessages sends the messages and returns how many were sent.
	SendMessages(messages []*Message) (int, error)
}

// Options are the arguments of a backend, as NewConnection resolves them
// from the settings and the Option values.
type Options struct {
	// FailSilently makes the backend skip errors instead of returning
	// them.
	FailSilently bool

	// The SMTP backend uses the fields from Host to SSLCertfile, as the
	// EMAIL_* settings of the same names.
	Host        string
	Port        int
	Username    string
	Password    string
	UseTLS      bool
	UseSSL      bool
	Timeout     time.Duration
	SSLKeyfile  string
	SSLCertfile string

	// FilePath is the directory of the file backend.
	FilePath string
	// Stream receives the messages of the console backend. Nil means
	// standard output.
	Stream io.Writer

	// backend names the backend that NewConnection builds.
	backend string
	// connection is the backend that SendMail and the other helpers use
	// instead of a new one.
	connection Backend
	// html is the HTML alternative of SendMail, MailAdmins and
	// MailManagers.
	html string
}

// Option changes an argument of NewConnection or of the send helpers, as
// the keyword arguments of Django's get_connection and send_mail.
type Option func(*Options)

// FailSilently makes the backend skip errors.
func FailSilently() Option { return func(o *Options) { o.FailSilently = true } }

// WithHost sets the SMTP host.
func WithHost(host string) Option { return func(o *Options) { o.Host = host } }

// WithPort sets the SMTP port.
func WithPort(port int) Option { return func(o *Options) { o.Port = port } }

// WithUsername sets the SMTP user name. An empty name turns login off.
func WithUsername(username string) Option { return func(o *Options) { o.Username = username } }

// WithPassword sets the SMTP password. An empty password turns login off.
func WithPassword(password string) Option { return func(o *Options) { o.Password = password } }

// WithTLS turns STARTTLS on or off.
func WithTLS(on bool) Option { return func(o *Options) { o.UseTLS = on } }

// WithSSL turns TLS from the start of the connection on or off.
func WithSSL(on bool) Option { return func(o *Options) { o.UseSSL = on } }

// WithTimeout limits each network operation. Zero means no limit.
func WithTimeout(d time.Duration) Option { return func(o *Options) { o.Timeout = d } }

// WithSSLKeyfile sets the key file of the client certificate.
func WithSSLKeyfile(path string) Option { return func(o *Options) { o.SSLKeyfile = path } }

// WithSSLCertfile sets the client certificate file.
func WithSSLCertfile(path string) Option { return func(o *Options) { o.SSLCertfile = path } }

// WithFilePath sets the directory of the file backend.
func WithFilePath(path string) Option { return func(o *Options) { o.FilePath = path } }

// WithStream sets the writer of the console backend.
func WithStream(w io.Writer) Option { return func(o *Options) { o.Stream = w } }

// WithConnection makes SendMail, SendMassMail, MailAdmins and MailManagers
// send through conn; the options that build a connection are then unused.
func WithConnection(conn Backend) Option { return func(o *Options) { o.connection = conn } }

// WithHTML adds an HTML alternative to the message of SendMail, MailAdmins
// and MailManagers.
func WithHTML(html string) Option { return func(o *Options) { o.html = html } }

// Factory makes a backend from resolved options.
type Factory func(Options) (Backend, error)

// backends holds the factories by name.
var backends = struct {
	sync.RWMutex
	m map[string]Factory
}{m: map[string]Factory{
	"smtp":    func(o Options) (Backend, error) { return NewSMTPBackend(o) },
	"console": func(o Options) (Backend, error) { return NewConsoleBackend(o), nil },
	"file":    func(o Options) (Backend, error) { return NewFileBackend(o) },
	"locmem":  func(o Options) (Backend, error) { return NewLocmemBackend(o), nil },
	"dummy":   func(Options) (Backend, error) { return DummyBackend{}, nil },
}}

// RegisterBackend makes a backend available under name, for the Backend
// setting and WithBackend. It replaces a backend of the same name.
func RegisterBackend(name string, f Factory) {
	backends.Lock()
	defer backends.Unlock()
	backends.m[name] = f
}

// BackendNames returns the names of the available backends, sorted.
func BackendNames() []string {
	backends.RLock()
	defer backends.RUnlock()
	names := make([]string, 0, len(backends.m))
	for name := range backends.m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HasBackend reports whether a backend of that name is available.
func HasBackend(name string) bool {
	return slices.Contains(BackendNames(), name)
}

// WithBackend makes NewConnection build the backend of that name instead of
// the one the Backend setting names.
func WithBackend(name string) Option { return func(o *Options) { o.backend = name } }

// resolve returns the options of the settings with opts applied.
func resolve(opts []Option) Options {
	cfg := CurrentConfig()
	o := Options{
		Host: cfg.Host, Port: cfg.Port, Username: cfg.HostUser, Password: cfg.HostPassword,
		UseTLS: cfg.UseTLS, UseSSL: cfg.UseSSL, Timeout: cfg.Timeout,
		SSLKeyfile: cfg.SSLKeyfile, SSLCertfile: cfg.SSLCertfile, FilePath: cfg.FilePath,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// NewConnection returns a new backend, as Django's get_connection does: the
// one WithBackend names, or the one of the Backend setting.
func NewConnection(opts ...Option) (Backend, error) {
	o := resolve(opts)
	name := o.backend
	if name == "" {
		name = CurrentConfig().Backend
	}
	backends.RLock()
	f, ok := backends.m[name]
	backends.RUnlock()
	if !ok {
		return nil, fmt.Errorf("mail: unknown email backend %q", name)
	}
	return f(o)
}

// connectionFor returns the connection of the send helpers: the one
// WithConnection gave, or a new one.
func connectionFor(opts []Option) (Backend, Options, error) {
	o := resolve(opts)
	if o.connection != nil {
		return o.connection, o, nil
	}
	conn, err := NewConnection(opts...)
	return conn, o, err
}

// DummyBackend sends nothing and reports every message as sent.
type DummyBackend struct{}

// Open does nothing.
func (DummyBackend) Open() (bool, error) { return false, nil }

// Close does nothing.
func (DummyBackend) Close() error { return nil }

// SendMessages returns the number of messages.
func (DummyBackend) SendMessages(messages []*Message) (int, error) { return len(messages), nil }
