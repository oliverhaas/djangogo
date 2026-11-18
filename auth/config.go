package auth

import (
	"sync/atomic"
)

// Config holds the settings of the auth app. The zero value is Django's
// default for every field.
type Config struct {
	// Backends is the ordered list of authentication backends (Django's
	// AUTHENTICATION_BACKENDS). Nil means ModelBackend alone.
	Backends []Backend
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
