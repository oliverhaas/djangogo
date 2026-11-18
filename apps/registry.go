package apps

import (
	"errors"
	"fmt"
	"strings"
)

// entry is a registered app with the name and the label it reported at
// registration. Both are read once, so a config that computes them cannot
// make the registry inconsistent later.
type entry struct {
	config Config
	name   string
	label  string
}

// state is the progress of the ready pass.
type state int

const (
	// stateLoading: apps are registered, resolved and wired.
	stateLoading state = iota
	// stateStarting: the ready pass is running.
	stateStarting
	// stateReady: the ready pass completed.
	stateReady
	// stateFailed: a Ready hook failed; the registry stays unusable.
	stateFailed
)

// Registry holds the installed apps in registration order (the INSTALLED_APPS
// analog) and starts them in dependency order. Build it from one goroutine;
// after Ready it does not change and is safe for concurrent reads.
type Registry struct {
	entries []entry        // registration order
	byName  map[string]int // index into entries
	byLabel map[string]int // index into entries

	start    []entry // start order, valid while resolved is set
	resolved bool

	state    state
	readyErr error // result of a failed ready pass
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]int), byLabel: make(map[string]int)}
}

// Register adds an app; the order of the calls is the INSTALLED_APPS order.
// With a *conf.ImproperlyConfigured that carries Django's message it rejects
// an empty name, a label that is not a valid identifier, and a label or a
// name that is registered already. A nil config and a registration after the
// ready pass has started are errors as well.
func (r *Registry) Register(c Config) error {
	if c == nil {
		return errors.New("apps: Register requires a config, got nil")
	}
	if r.state != stateLoading {
		return fmt.Errorf("apps: cannot register %T: the ready pass has started", c)
	}
	name := c.Name()
	if name == "" {
		return improperlyConfigured("'%T' must supply a name attribute.", c)
	}
	label := Label(c)
	if !isIdentifier(label) {
		return improperlyConfigured("The app label '%s' is not a valid Python identifier.", label)
	}
	if _, dup := r.byLabel[label]; dup {
		return improperlyConfigured("Application labels aren't unique, duplicates: %s", label)
	}
	if _, dup := r.byName[name]; dup {
		return improperlyConfigured("Application names aren't unique, duplicates: %s", name)
	}

	r.byName[name] = len(r.entries)
	r.byLabel[label] = len(r.entries)
	r.entries = append(r.entries, entry{config: c, name: name, label: label})
	r.start, r.resolved = nil, false
	return nil
}

// Get returns the app registered under name.
func (r *Registry) Get(name string) (Config, bool) {
	i, ok := r.byName[name]
	if !ok {
		return nil, false
	}
	return r.entries[i].config, true
}

// ByLabel returns the app with the label (apps.get_app_config). The error for
// an unknown label is a *LookupError with Django's text, "No installed app
// with label 'x'.", followed by " Did you mean 'y'?" when x is the name of an
// installed app with the label y (Django's rule) or the last component of its
// name, which happens when the app replaces its default label.
func (r *Registry) ByLabel(label string) (Config, error) {
	if i, ok := r.byLabel[label]; ok {
		return r.entries[i].config, nil
	}
	err := &LookupError{Label: label}
	for _, e := range r.entries {
		if e.name == label || strings.HasSuffix(e.name, "."+label) || strings.HasSuffix(e.name, "/"+label) {
			err.Suggestion = e.label
			break
		}
	}
	return nil, err
}

// IsInstalled reports whether an app with that name is registered
// (apps.is_installed). The name is the full name of the app, for example
// "django.contrib.admin", not its label.
func (r *Registry) IsInstalled(name string) bool {
	_, ok := r.byName[name]
	return ok
}

// Names returns app names in registration order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.name
	}
	return out
}

// Labels returns the labels in registration order.
func (r *Registry) Labels() []string {
	out := make([]string, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.label
	}
	return out
}

// Configs returns the app configs in registration (INSTALLED_APPS) order
// (apps.get_app_configs).
func (r *Registry) Configs() []Config {
	return configsOf(r.entries)
}

// configsOf returns the configs of the entries, in order.
func configsOf(entries []entry) []Config {
	out := make([]Config, len(entries))
	for i, e := range entries {
		out[i] = e.config
	}
	return out
}
