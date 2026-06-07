// Package conf holds Djan-Go-Go application settings (the analog of Django's settings module).
package conf

import "errors"

// Settings is the typed configuration for an application. Later milestones add
// Templates and an app-extensible registry; the Spine keeps it minimal.
type Settings struct {
	Debug         bool
	SecretKey     string
	AllowedHosts  []string
	InstalledApps []string
	Host          string
	Port          string
	Database      Database
}

// Database identifies the database backend and its connection string.
type Database struct {
	Driver string // e.g. "sqlite"
	DSN    string // driver-specific connection string
}

const (
	defaultHost   = "127.0.0.1"
	defaultPort   = "8000"
	defaultDriver = "sqlite"
)

// applyDefaults fills empty fields with their defaults. Called by Configure.
func (s *Settings) applyDefaults() {
	if s.Host == "" {
		s.Host = defaultHost
	}
	if s.Port == "" {
		s.Port = defaultPort
	}
	if len(s.AllowedHosts) == 0 {
		s.AllowedHosts = []string{"localhost", "127.0.0.1"}
	}
	if s.Database.Driver == "" {
		s.Database.Driver = defaultDriver
	}
}

// Check validates settings at boot. It returns a non-nil error for misconfiguration.
func (s *Settings) Check() error {
	if s.SecretKey == "" {
		return errors.New("conf: SecretKey must be set")
	}
	return nil
}
