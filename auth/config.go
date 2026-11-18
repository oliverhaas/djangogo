package auth

import (
	"crypto/rand"
	"sync"
	"sync/atomic"
)

// Defaults of the URL settings, as in Django's global settings.
const (
	// DefaultLoginURL is Django's LOGIN_URL.
	DefaultLoginURL = "/accounts/login/"
	// DefaultLoginRedirectURL is Django's LOGIN_REDIRECT_URL.
	DefaultLoginRedirectURL = "/accounts/profile/"
	// DefaultRedirectFieldName is Django's REDIRECT_FIELD_NAME.
	DefaultRedirectFieldName = "next"
)

// Config holds the settings of the auth app. The zero value is Django's
// default for every field.
type Config struct {
	// Backends is the ordered list of authentication backends (Django's
	// AUTHENTICATION_BACKENDS). Nil means ModelBackend alone.
	Backends []Backend

	// PasswordHashers is the ordered list of password hashers (Django's
	// PASSWORD_HASHERS). The first one hashes new passwords; the others
	// only verify, and a password they verify is hashed again with the
	// first one at login. Empty means DefaultPasswordHashers.
	PasswordHashers []Hasher

	// SecretKey signs the value that ties a session to the password of its
	// user (Django's SECRET_KEY). Set it to the secret key of the project.
	// When it is empty a random key is made per process: logins then end
	// when the process restarts and do not carry over to other processes.
	SecretKey string

	// SecretKeyFallbacks are former values of SecretKey (Django's
	// SECRET_KEY_FALLBACKS). A session signed with one of them stays valid
	// and is signed again with SecretKey on its next request.
	SecretKeyFallbacks []string

	// LoginURL is where a guard sends a user that has to log in (Django's
	// LOGIN_URL). Empty means "/accounts/login/".
	LoginURL string

	// LoginRedirectURL is where LoginView sends a user after login when the
	// request names no other place (Django's LOGIN_REDIRECT_URL). Empty
	// means "/accounts/profile/".
	LoginRedirectURL string

	// LogoutRedirectURL is where LogoutView sends a user after logout
	// (Django's LOGOUT_REDIRECT_URL). Empty means that LogoutView renders
	// the logged-out page.
	LogoutRedirectURL string
}

// current is the active configuration. It is replaced as a whole, so a reader
// never sees a half-applied change.
var current atomic.Pointer[Config]

// Configure replaces the settings of the auth app and returns a function that
// puts the previous ones back. Call it once at startup, before requests are
// served; a test that changes the configuration defers the restore function
// and must not run in parallel with other tests.
func Configure(cfg Config) (restore func()) {
	next := cfg
	prev := current.Swap(&next)
	return func() { current.Store(prev) }
}

// CurrentConfig returns the active settings, as passed to Configure. Change
// the copy and pass it to Configure to replace a single setting.
func CurrentConfig() Config { return settings() }

// settings returns the active configuration.
func settings() Config {
	if cfg := current.Load(); cfg != nil {
		return *cfg
	}
	return Config{}
}

// backends returns the configured backends, or the default list.
func backends() []Backend {
	if b := settings().Backends; b != nil {
		return b
	}
	return defaultBackends
}

// defaultBackends is Django's default AUTHENTICATION_BACKENDS.
var defaultBackends = []Backend{ModelBackend{}}

// backendByName returns the configured backend of that name, or nil.
func backendByName(name string) Backend {
	for _, b := range backends() {
		if b.Name() == name {
			return b
		}
	}
	return nil
}

// processSecret is the key in use while Config.SecretKey is empty: 32 random
// bytes that live as long as the process.
var processSecret = sync.OnceValue(func() []byte {
	key := make([]byte, 32)
	// rand.Read cannot fail: it aborts the program instead of returning a
	// predictable value.
	_, _ = rand.Read(key)
	return key
})

// secretKey returns the key that signs session auth hashes.
func secretKey() []byte {
	if key := settings().SecretKey; key != "" {
		return []byte(key)
	}
	return processSecret()
}
