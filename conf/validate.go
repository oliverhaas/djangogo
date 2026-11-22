package conf

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Problem is one invalid setting.
type Problem struct {
	// Setting is Django's name of the setting, for example
	// "SESSION_COOKIE_SAMESITE".
	Setting string
	// Msg says what is wrong, as a full sentence.
	Msg string
}

// Validate returns the problems of the setting values themselves, for the
// system checks to report; nil when there is none. It checks what a value may
// be, not whether the values fit a deployment (the security checks do that),
// and not SecretKey (Check does that). The settings are validated as Configure
// would complete them and are not changed.
//
// A problem is reported for: a SessionEngine other than SessionEngineDB,
// SessionEngineFile and SessionEngineSignedCookies; a MessageStorage other than
// MessageStorageFallback, MessageStorageCookie and MessageStorageSession; a
// SessionCookieSameSite or CSRFCookieSameSite
// other than "Lax", "Strict", "None" (in any letter case) or the empty string,
// which sends no attribute; a database whose driver NormalizeDriver does not
// know; a TimeZone that Location cannot load; a StaticURL that is not empty
// and does not end with a slash; a SecureProxySSLHeader that is not empty and
// does not hold exactly two values; a negative SessionCookieAge, CSRFCookieAge
// or SecureHSTSSeconds; an XFrameOptions other than "DENY", "SAMEORIGIN" (in
// any letter case) or the empty string, which means "DENY"; a
// CSRFTrustedOrigins entry without a scheme; a Port that is not a number from
// 0 to 65535.
//
// The messages are Django's where Django has one.
func (s *Settings) Validate() []Problem {
	// The copy shares no map with s that applyDefaults writes to.
	c := *s
	c.applyDefaults()

	var v validation
	if _, err := strconv.ParseUint(c.Port, 10, 16); err != nil {
		v.addf("PORT", "The PORT setting must be a port number from 0 to 65535, but is %q.", c.Port)
	}
	c.validateDatabases(&v)
	if _, err := c.Location(); err != nil {
		v.addf("TIME_ZONE", "Incorrect timezone setting: %s", c.TimeZone)
	}
	if c.StaticURL != "" && !strings.HasSuffix(c.StaticURL, "/") {
		v.addf("STATIC_URL", "The STATIC_URL setting must end with a slash.")
	}
	switch c.SessionEngine {
	case SessionEngineDB, SessionEngineFile, SessionEngineSignedCookies:
	default:
		v.addf("SESSION_ENGINE", "The SESSION_ENGINE setting must be %q, %q or %q, but is %q.",
			SessionEngineDB, SessionEngineFile, SessionEngineSignedCookies, c.SessionEngine)
	}
	switch c.MessageStorage {
	case MessageStorageFallback, MessageStorageCookie, MessageStorageSession:
	default:
		v.addf("MESSAGE_STORAGE", "The MESSAGE_STORAGE setting must be %q, %q or %q, but is %q.",
			MessageStorageFallback, MessageStorageCookie, MessageStorageSession, c.MessageStorage)
	}
	v.notNegative("SESSION_COOKIE_AGE", int64(c.SessionCookieAge))
	v.sameSite("SESSION_COOKIE_SAMESITE", c.SessionCookieSameSite)
	for _, origin := range c.CSRFTrustedOrigins {
		if !strings.Contains(origin, "://") {
			v.addf("CSRF_TRUSTED_ORIGINS", "The values in the CSRF_TRUSTED_ORIGINS setting must start "+
				"with a scheme (usually http:// or https://) but found %s.", origin)
		}
	}
	v.notNegative("CSRF_COOKIE_AGE", int64(c.CSRFCookieAge))
	v.sameSite("CSRF_COOKIE_SAMESITE", c.CSRFCookieSameSite)
	v.notNegative("SECURE_HSTS_SECONDS", int64(c.SecureHSTSSeconds))
	if n := len(c.SecureProxySSLHeader); n != 0 && n != 2 {
		v.addf("SECURE_PROXY_SSL_HEADER", "The SECURE_PROXY_SSL_HEADER setting must be a tuple containing two values.")
	}
	if x := strings.ToUpper(c.XFrameOptions); x != "" && x != "DENY" && x != "SAMEORIGIN" {
		v.addf("X_FRAME_OPTIONS", `The X_FRAME_OPTIONS setting must be "DENY" or "SAMEORIGIN", but is %q.`, c.XFrameOptions)
	}
	return v.problems
}

// validateDatabases reports every database whose driver is unknown, in the
// order of the aliases.
func (s *Settings) validateDatabases(v *validation) {
	databases := maps.Clone(s.Databases)
	if databases == nil {
		databases = make(map[string]Database, 1)
	}
	if _, ok := databases[DefaultDatabaseAlias]; !ok {
		databases[DefaultDatabaseAlias] = s.Database
	}
	for _, alias := range slices.Sorted(maps.Keys(databases)) {
		driver := databases[alias].Driver
		if _, ok := NormalizeDriver(driver); !ok {
			v.addf("DATABASES", "The DATABASES setting names the unknown driver %q for the database %q; "+
				"the drivers are %q and %q.", driver, alias, DriverSQLite, DriverPostgres)
		}
	}
}

// validation collects the problems of one Validate call.
type validation struct {
	problems []Problem
}

// addf adds a problem of the setting with a formatted message.
func (v *validation) addf(setting, format string, args ...any) {
	v.problems = append(v.problems, Problem{Setting: setting, Msg: fmt.Sprintf(format, args...)})
}

// notNegative reports a negative number, which a cookie lifetime or a max-age
// cannot be.
func (v *validation) notNegative(setting string, n int64) {
	if n < 0 {
		v.addf(setting, "The %s setting must not be negative.", setting)
	}
}

// sameSite reports a SameSite value that a browser does not know. The empty
// string sends no attribute, as None or False does in Django.
func (v *validation) sameSite(setting, value string) {
	switch strings.ToLower(value) {
	case "", "lax", "strict", "none":
		return
	}
	v.addf(setting, `The %s setting must be "Lax", "Strict" or "None", but is %q.`, setting, value)
}
