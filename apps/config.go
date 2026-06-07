// Package apps provides the application registry (Django's AppConfig + apps registry).
package apps

// Config is the minimal app contract: every app reports a unique name.
type Config interface {
	Name() string
}

// Initializer is an optional hook run after all apps are registered (Django's ready()).
type Initializer interface {
	Ready() error
}
