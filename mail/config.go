package mail

import (
	"sync/atomic"
	"time"
)

// Config holds the email settings of Django. Start from DefaultConfig,
// which holds Django's defaults.
type Config struct {
	// Backend names the backend that NewConnection builds when no
	// WithBackend option names one (EMAIL_BACKEND): "smtp", "console",
	// "file", "locmem", "dummy" or a name passed to RegisterBackend.
	Backend string

	// Host and Port are the address of the SMTP server (EMAIL_HOST and
	// EMAIL_PORT).
	Host string
	Port int
	// HostUser and HostPassword log in to the SMTP server when both are set
	// (EMAIL_HOST_USER and EMAIL_HOST_PASSWORD).
	HostUser     string
	HostPassword string
	// UseTLS starts TLS on the plain connection with STARTTLS
	// (EMAIL_USE_TLS). UseSSL connects with TLS from the start
	// (EMAIL_USE_SSL). They exclude each other.
	UseTLS bool
	UseSSL bool
	// SSLCertfile and SSLKeyfile are the PEM files of a client certificate
	// for TLS (EMAIL_SSL_CERTFILE and EMAIL_SSL_KEYFILE).
	SSLCertfile string
	SSLKeyfile  string
	// Timeout limits each network operation of the SMTP backend
	// (EMAIL_TIMEOUT). Zero means no limit.
	Timeout time.Duration

	// FilePath is the directory of the file backend (EMAIL_FILE_PATH).
	FilePath string

	// DefaultFromEmail is the sender of a message without one
	// (DEFAULT_FROM_EMAIL).
	DefaultFromEmail string
	// ServerEmail is the sender of MailAdmins and MailManagers
	// (SERVER_EMAIL).
	ServerEmail string
	// Admins and Managers are the recipients of MailAdmins and MailManagers
	// (ADMINS and MANAGERS), one address each.
	Admins   []string
	Managers []string
	// SubjectPrefix starts the subject of MailAdmins and MailManagers
	// (EMAIL_SUBJECT_PREFIX).
	SubjectPrefix string

	// UseLocaltime writes the Date header in Location instead of UTC
	// (EMAIL_USE_LOCALTIME).
	UseLocaltime bool
	// Location is the current time zone (TIME_ZONE). Nil means time.Local.
	Location *time.Location

	// DefaultCharset encodes the text of a message without an Encoding
	// (DEFAULT_CHARSET).
	DefaultCharset string
}

// DefaultConfig returns Django's default email settings.
func DefaultConfig() Config {
	return Config{
		Backend:          "smtp",
		Host:             "localhost",
		Port:             25,
		DefaultFromEmail: "webmaster@localhost",
		ServerEmail:      "root@localhost",
		SubjectPrefix:    "[Django] ",
		DefaultCharset:   "utf-8",
	}
}

// current is the active configuration. It is replaced as a whole, so a
// reader never sees a half-applied change.
var current atomic.Pointer[Config]

// Configure replaces the email settings and returns a function that puts the
// previous ones back. Call it at startup, before mail is sent; a test that
// changes the settings defers the restore function and must not run in
// parallel with other tests.
func Configure(cfg Config) (restore func()) {
	next := cfg
	prev := current.Swap(&next)
	return func() { current.Store(prev) }
}

// CurrentConfig returns the active settings: those passed to Configure, or
// DefaultConfig before the first call.
func CurrentConfig() Config {
	if cfg := current.Load(); cfg != nil {
		return *cfg
	}
	return DefaultConfig()
}

// location returns the current time zone.
func (c Config) location() *time.Location {
	if c.Location != nil {
		return c.Location
	}
	return time.Local
}
