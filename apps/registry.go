package apps

import "fmt"

// Registry holds the installed apps in registration order (the INSTALLED_APPS analog).
type Registry struct {
	order  []string
	byName map[string]Config
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Config)}
}

// Register adds an app. It errors on an empty or duplicate name.
func (r *Registry) Register(c Config) error {
	name := c.Name()
	if name == "" {
		return fmt.Errorf("apps: config %T has an empty Name()", c)
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("apps: duplicate app %q", name)
	}
	r.byName[name] = c
	r.order = append(r.order, name)
	return nil
}

// Get returns the app registered under name.
func (r *Registry) Get(name string) (Config, bool) {
	c, ok := r.byName[name]
	return c, ok
}

// Names returns app names in registration order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Ready runs Ready() on every app implementing Initializer, in registration order.
func (r *Registry) Ready() error {
	for _, name := range r.order {
		if init, ok := r.byName[name].(Initializer); ok {
			if err := init.Ready(); err != nil {
				return fmt.Errorf("apps: %s.Ready: %w", name, err)
			}
		}
	}
	return nil
}
