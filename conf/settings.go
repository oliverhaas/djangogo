package conf

import "time"

// Session engines accepted by Settings.SessionEngine.
const (
	// SessionEngineDB stores sessions in the database (Django's default,
	// django.contrib.sessions.backends.db).
	SessionEngineDB = "db"
	// SessionEngineSignedCookies stores the session data in a signed cookie
	// (django.contrib.sessions.backends.signed_cookies).
	SessionEngineSignedCookies = "signed_cookies"
)

// Message storages accepted by Settings.MessageStorage.
const (
	// MessageStorageFallback keeps the messages in a signed cookie and the
	// ones that do not fit there in the session (Django's default,
	// django.contrib.messages.storage.fallback.FallbackStorage).
	MessageStorageFallback = "fallback"
	// MessageStorageCookie keeps the messages in a signed cookie and drops
	// the oldest ones that do not fit
	// (django.contrib.messages.storage.cookie.CookieStorage).
	MessageStorageCookie = "cookie"
	// MessageStorageSession keeps the messages in the session
	// (django.contrib.messages.storage.session.SessionStorage).
	MessageStorageSession = "session"
)

// DefaultDatabaseAlias is the alias of the database used when none is named
// (Django's DEFAULT_DB_ALIAS).
const DefaultDatabaseAlias = "default"

// Settings is the typed configuration of a project.
type Settings struct {
	// Debug turns on the technical error pages, static file serving and the
	// localhost host defaults (DEBUG, default false).
	Debug bool `env:"DEBUG"`
	// SecretKey signs sessions and tokens (SECRET_KEY). It must not be empty.
	SecretKey string `env:"SECRET_KEY"`
	// SecretKeyFallbacks lists former secret keys still accepted for
	// verification (SECRET_KEY_FALLBACKS).
	SecretKeyFallbacks []string `env:"SECRET_KEY_FALLBACKS"`
	// AllowedHosts lists the host names the site serves (ALLOWED_HOSTS). A
	// leading dot matches a domain and its subdomains, "*" matches any host.
	// When it is empty and Debug is on, localhost names are allowed.
	AllowedHosts []string `env:"ALLOWED_HOSTS"`
	// InstalledApps names the installed apps (INSTALLED_APPS). It is
	// informational: the apps passed to djangogo.New are the installed apps.
	InstalledApps []string `env:"INSTALLED_APPS"`
	// SilencedSystemChecks lists the ids of system check messages to hide
	// (SILENCED_SYSTEM_CHECKS).
	SilencedSystemChecks []string `env:"SILENCED_SYSTEM_CHECKS"`

	// Host is the default address runserver binds to (default "127.0.0.1").
	// Django has no such setting; it is the default of runserver's addrport.
	Host string `env:"HOST"`
	// Port is the default port runserver binds to (default "8000").
	Port string `env:"PORT"`

	// Database is the default database. It is the same value as
	// Databases["default"]: Configure and ApplyEnv copy whichever of the two
	// is set, or was changed last, into the other. The environment sets it
	// through DATABASE_URL.
	Database Database
	// Databases maps aliases to databases (DATABASES). The alias "default"
	// names the database the framework opens.
	Databases map[string]Database

	// Templates configures the template engine (the first entry of TEMPLATES).
	Templates Templates

	// TimeZone is the IANA name of the default time zone (TIME_ZONE, default "UTC").
	TimeZone string `env:"TIME_ZONE"`
	// UseTZ makes datetimes timezone-aware (USE_TZ, default true).
	UseTZ bool `env:"USE_TZ"`

	// StaticURL is the URL prefix of static files (STATIC_URL, default "/static/").
	StaticURL string `env:"STATIC_URL"`
	// StaticRoot is the directory collectstatic copies static files into (STATIC_ROOT).
	StaticRoot string `env:"STATIC_ROOT"`
	// StaticFilesDirs lists extra directories searched for static files
	// before the apps' own (STATICFILES_DIRS).
	StaticFilesDirs []string `env:"STATICFILES_DIRS"`

	// LoginURL is where a login-required view sends an anonymous user
	// (LOGIN_URL, default "/accounts/login/").
	LoginURL string `env:"LOGIN_URL"`
	// LoginRedirectURL is where a login without a next parameter leads
	// (LOGIN_REDIRECT_URL, default "/accounts/profile/").
	LoginRedirectURL string `env:"LOGIN_REDIRECT_URL"`
	// LogoutRedirectURL is where a logout leads; empty renders the logged-out
	// page (LOGOUT_REDIRECT_URL, default unset).
	LogoutRedirectURL string `env:"LOGOUT_REDIRECT_URL"`

	// AppendSlash redirects a URL without a trailing slash to the one with it
	// when only that one resolves (APPEND_SLASH, default true).
	AppendSlash bool `env:"APPEND_SLASH"`
	// UseXForwardedHost trusts the X-Forwarded-Host header over Host
	// (USE_X_FORWARDED_HOST, default false).
	UseXForwardedHost bool `env:"USE_X_FORWARDED_HOST"`
	// UseXForwardedPort trusts the X-Forwarded-Port header (USE_X_FORWARDED_PORT,
	// default false).
	UseXForwardedPort bool `env:"USE_X_FORWARDED_PORT"`

	// SessionEngine selects the session backend: SessionEngineDB (default) or
	// SessionEngineSignedCookies (SESSION_ENGINE). The database engine needs
	// a database and the installed sessions app; without them an
	// Application keeps the sessions in signed cookies.
	SessionEngine string `env:"SESSION_ENGINE"`
	// SessionCookieName is the name of the session cookie (SESSION_COOKIE_NAME,
	// default "sessionid").
	SessionCookieName string `env:"SESSION_COOKIE_NAME"`
	// SessionCookieAge is the lifetime of the session cookie (SESSION_COOKIE_AGE,
	// default two weeks). The environment value is a number of seconds, as in
	// Django, or a Go duration such as "336h".
	SessionCookieAge time.Duration `env:"SESSION_COOKIE_AGE"`
	// SessionCookieSecure marks the session cookie as HTTPS only
	// (SESSION_COOKIE_SECURE, default false).
	SessionCookieSecure bool `env:"SESSION_COOKIE_SECURE"`
	// SessionCookieHTTPOnly hides the session cookie from JavaScript
	// (SESSION_COOKIE_HTTPONLY, default true).
	SessionCookieHTTPOnly bool `env:"SESSION_COOKIE_HTTPONLY"`
	// SessionCookieSameSite is the SameSite attribute of the session cookie:
	// "Lax" (default), "Strict" or "None" (SESSION_COOKIE_SAMESITE).
	SessionCookieSameSite string `env:"SESSION_COOKIE_SAMESITE"`
	// SessionCookieDomain is the Domain attribute of the session cookie; empty
	// means a host-only cookie (SESSION_COOKIE_DOMAIN).
	SessionCookieDomain string `env:"SESSION_COOKIE_DOMAIN"`
	// SessionCookiePath is the Path attribute of the session cookie
	// (SESSION_COOKIE_PATH, default "/").
	SessionCookiePath string `env:"SESSION_COOKIE_PATH"`
	// SessionSaveEveryRequest saves the session on every request, not only
	// when it was modified (SESSION_SAVE_EVERY_REQUEST, default false).
	SessionSaveEveryRequest bool `env:"SESSION_SAVE_EVERY_REQUEST"`
	// SessionExpireAtBrowserClose makes the session cookie a browser-session
	// cookie (SESSION_EXPIRE_AT_BROWSER_CLOSE, default false).
	SessionExpireAtBrowserClose bool `env:"SESSION_EXPIRE_AT_BROWSER_CLOSE"`

	// MessageStorage selects where the messages framework keeps the messages
	// between requests: MessageStorageFallback (default),
	// MessageStorageCookie or MessageStorageSession (MESSAGE_STORAGE). The
	// messages cookie takes the domain and the Secure, HttpOnly and SameSite
	// attributes of the session cookie, as in Django.
	MessageStorage string `env:"MESSAGE_STORAGE"`

	// CSRFTrustedOrigins lists the origins trusted for unsafe requests, each
	// with its scheme (CSRF_TRUSTED_ORIGINS).
	CSRFTrustedOrigins []string `env:"CSRF_TRUSTED_ORIGINS"`
	// CSRFCookieName is the name of the CSRF cookie (CSRF_COOKIE_NAME, default
	// "csrftoken").
	CSRFCookieName string `env:"CSRF_COOKIE_NAME"`
	// CSRFCookieAge is the lifetime of the CSRF cookie (CSRF_COOKIE_AGE,
	// default 52 weeks). The environment value is seconds or a Go duration.
	CSRFCookieAge time.Duration `env:"CSRF_COOKIE_AGE"`
	// CSRFCookieSecure marks the CSRF cookie as HTTPS only (CSRF_COOKIE_SECURE,
	// default false).
	CSRFCookieSecure bool `env:"CSRF_COOKIE_SECURE"`
	// CSRFCookieHTTPOnly hides the CSRF cookie from JavaScript
	// (CSRF_COOKIE_HTTPONLY, default false).
	CSRFCookieHTTPOnly bool `env:"CSRF_COOKIE_HTTPONLY"`
	// CSRFCookieSameSite is the SameSite attribute of the CSRF cookie
	// (CSRF_COOKIE_SAMESITE, default "Lax").
	CSRFCookieSameSite string `env:"CSRF_COOKIE_SAMESITE"`
	// CSRFCookieDomain is the Domain attribute of the CSRF cookie
	// (CSRF_COOKIE_DOMAIN).
	CSRFCookieDomain string `env:"CSRF_COOKIE_DOMAIN"`
	// CSRFCookiePath is the Path attribute of the CSRF cookie
	// (CSRF_COOKIE_PATH, default "/").
	CSRFCookiePath string `env:"CSRF_COOKIE_PATH"`
	// CSRFHeaderName is the request header that carries the token, as an HTTP
	// header name (default "X-CSRFToken"). Django's CSRF_HEADER_NAME holds the
	// WSGI spelling of the same header, "HTTP_X_CSRFTOKEN".
	CSRFHeaderName string `env:"CSRF_HEADER_NAME"`
	// CSRFUseSessions stores the token in the session and sets no CSRF cookie
	// (CSRF_USE_SESSIONS, default false).
	CSRFUseSessions bool `env:"CSRF_USE_SESSIONS"`

	// SecureSSLRedirect redirects HTTP requests to HTTPS (SECURE_SSL_REDIRECT,
	// default false).
	SecureSSLRedirect bool `env:"SECURE_SSL_REDIRECT"`
	// SecureSSLHost is the host the SSL redirect goes to; empty keeps the
	// requested host (SECURE_SSL_HOST).
	SecureSSLHost string `env:"SECURE_SSL_HOST"`
	// SecureRedirectExempt lists regular expressions of paths, without the
	// leading slash, that are not redirected to HTTPS (SECURE_REDIRECT_EXEMPT).
	SecureRedirectExempt []string `env:"SECURE_REDIRECT_EXEMPT"`
	// SecureHSTSSeconds is the max-age of the Strict-Transport-Security
	// header; 0 sends none (SECURE_HSTS_SECONDS, default 0).
	SecureHSTSSeconds int `env:"SECURE_HSTS_SECONDS"`
	// SecureHSTSIncludeSubdomains adds includeSubDomains to the HSTS header
	// (SECURE_HSTS_INCLUDE_SUBDOMAINS, default false).
	SecureHSTSIncludeSubdomains bool `env:"SECURE_HSTS_INCLUDE_SUBDOMAINS"`
	// SecureHSTSPreload adds preload to the HSTS header (SECURE_HSTS_PRELOAD,
	// default false).
	SecureHSTSPreload bool `env:"SECURE_HSTS_PRELOAD"`
	// SecureContentTypeNosniff sends X-Content-Type-Options: nosniff
	// (SECURE_CONTENT_TYPE_NOSNIFF, default true).
	SecureContentTypeNosniff bool `env:"SECURE_CONTENT_TYPE_NOSNIFF"`
	// SecureReferrerPolicy is the Referrer-Policy header; empty sends none
	// (SECURE_REFERRER_POLICY, default "same-origin").
	SecureReferrerPolicy string `env:"SECURE_REFERRER_POLICY"`
	// SecureCrossOriginOpenerPolicy is the Cross-Origin-Opener-Policy header;
	// empty sends none (SECURE_CROSS_ORIGIN_OPENER_POLICY, default "same-origin").
	SecureCrossOriginOpenerPolicy string `env:"SECURE_CROSS_ORIGIN_OPENER_POLICY"`
	// SecureProxySSLHeader names the header and the value by which a trusted
	// proxy marks a request as HTTPS, as two elements, for example
	// {"X-Forwarded-Proto", "https"} (SECURE_PROXY_SSL_HEADER). Django's WSGI
	// spelling "HTTP_X_FORWARDED_PROTO" is accepted for the header.
	SecureProxySSLHeader []string `env:"SECURE_PROXY_SSL_HEADER"`
	// XFrameOptions is the X-Frame-Options header: "DENY" (default) or
	// "SAMEORIGIN" (X_FRAME_OPTIONS).
	XFrameOptions string `env:"X_FRAME_OPTIONS"`

	// PasswordHashers lists password hashers in order of preference: the
	// first one hashes new passwords, all of them verify (PASSWORD_HASHERS).
	// An entry is the algorithm ("pbkdf2_sha256") or Django's class, with or
	// without its module ("django.contrib.auth.hashers.PBKDF2PasswordHasher").
	// Empty keeps the hashers of the auth package.
	PasswordHashers []string `env:"PASSWORD_HASHERS"`
	// AuthPasswordValidators lists the validators a new password must pass
	// (AUTH_PASSWORD_VALIDATORS), by Django's class with or without its
	// module. Empty keeps the validators of the auth package, the four of a
	// new Django project.
	AuthPasswordValidators []PasswordValidator

	// defaulted records that the defaults were applied, so a false or empty
	// value set afterwards is kept.
	defaulted bool
	// syncedDatabase is the default database as the last sync of Database and
	// Databases["default"] left it, which tells the next sync which of the two
	// was changed since. It is replaced, never written through.
	syncedDatabase *Database
	// overlay is the environment the settings were overlaid with, kept so that
	// settings namespaces registered later read the same source.
	overlay *envSource
	// namespaces holds the values of app settings namespaces set in code.
	namespaces map[string]any
}

// Database identifies a database backend and its connection string (an entry
// of Django's DATABASES).
type Database struct {
	// Driver is the backend: "sqlite" (default) or "postgres" (also spelled
	// "postgresql" or "pgx").
	Driver string
	// DSN is the driver-specific connection string: a SQLite file name or
	// "file:" URI, or a PostgreSQL URL or keyword string.
	DSN string
}

// Templates configures the template engine (an entry of Django's TEMPLATES).
type Templates struct {
	// Dirs lists directories searched for templates before the apps' own (DIRS).
	Dirs []string `env:"TEMPLATES_DIRS"`
	// AppDirs makes the engine search the templates of the installed apps
	// (APP_DIRS, default true as in Django's project template).
	AppDirs bool `env:"TEMPLATES_APP_DIRS"`
	// ContextProcessors names the context processors run for every
	// request-bound render (OPTIONS["context_processors"]), by Django's
	// dotted path or its last segment: request, static, debug, auth,
	// messages and csrf, which always runs. Nil selects those of a new Django
	// project: request, auth when the auth app is installed, and messages.
	ContextProcessors []string `env:"TEMPLATES_CONTEXT_PROCESSORS"`
}

// PasswordValidator names one password validator and its options (an entry of
// Django's AUTH_PASSWORD_VALIDATORS).
type PasswordValidator struct {
	// Name identifies the validator, for example "MinimumLengthValidator".
	Name string
	// Options holds the validator's options, for example {"min_length": 12}.
	Options map[string]any
}

// Check reports a misconfiguration that makes the settings unusable: an empty
// SecretKey, as Django refuses to start without SECRET_KEY. The error is an
// *ImproperlyConfigured. Everything else is validated by the system checks.
func (s *Settings) Check() error {
	if s.SecretKey == "" {
		return &ImproperlyConfigured{Msg: "The SECRET_KEY setting must not be empty."}
	}
	return nil
}

// DefaultDatabase returns the database under the alias "default": the entry of
// Databases when there is one, else the Database field.
func (s *Settings) DefaultDatabase() Database {
	if db, ok := s.Databases[DefaultDatabaseAlias]; ok {
		return db
	}
	return s.Database
}

// Location returns the time zone named by TimeZone. An empty name is UTC.
func (s *Settings) Location() (*time.Location, error) {
	if s.TimeZone == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(s.TimeZone)
}
