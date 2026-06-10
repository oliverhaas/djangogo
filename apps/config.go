// Package apps provides the application registry (Django's AppConfig + apps registry).
package apps

import "github.com/oliverhaas/djangogo/urls"

// Config is the minimal app contract: every app reports a unique name.
type Config interface {
	Name() string
}

// Initializer is an optional hook run after all apps are registered (Django's ready()).
type Initializer interface {
	Ready() error
}

// ModelProvider is implemented by apps that declare ORM models. Models returns
// pointers to zero-valued model structs (e.g. &Article{}).
type ModelProvider interface {
	Models() []any
}

// URLProvider is implemented by apps that contribute URL routes (Django's
// urls.py). The framework collects every provider's routes into the root router.
type URLProvider interface {
	URLs() []urls.Route
}
