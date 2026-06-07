// Package conf holds Djan-Go-Go application settings (the analog of Django's settings module).
package conf

import "errors"

// Settings is the typed configuration for an application. Later milestones add
// Databases, Templates, and an app-extensible registry; the Spine keeps it minimal.
type Settings struct {
	Debug         bool
	SecretKey     string
	AllowedHosts  []string
	InstalledApps []string
	Host          string
	Port          string
}

const (
	defaultHost = "127.0.0.1"
	defaultPort = "8000"
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
}

// Check validates settings at boot. It returns a non-nil error for misconfiguration.
func (s *Settings) Check() error {
	if s.SecretKey == "" {
		return errors.New("conf: SecretKey must be set")
	}
	return nil
}
