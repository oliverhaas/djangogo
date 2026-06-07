package migrations

// Registry holds the known migrations grouped by app, in insertion order.
type Registry struct {
	byApp    map[string][]Migration
	appOrder []string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byApp: map[string][]Migration{}}
}

// Add records m under its app, preserving insertion order and tracking first-seen app order.
func (r *Registry) Add(m Migration) {
	if _, ok := r.byApp[m.App]; !ok {
		r.appOrder = append(r.appOrder, m.App)
	}
	r.byApp[m.App] = append(r.byApp[m.App], m)
}

// ForApp returns the migrations registered for app in insertion order, or nil if none.
func (r *Registry) ForApp(app string) []Migration {
	return r.byApp[app]
}

// All returns every migration, ordered by app (first-seen) then per-app insertion order.
func (r *Registry) All() []Migration {
	var out []Migration
	for _, app := range r.appOrder {
		out = append(out, r.byApp[app]...)
	}
	return out
}
