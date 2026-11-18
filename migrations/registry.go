package migrations

// Registry holds the known migrations grouped by app, in insertion order. It
// takes the place of the migrations directories Django scans: a generated
// migration file registers itself from init.
type Registry struct {
	byApp    map[string][]Migration
	appOrder []string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byApp: map[string][]Migration{}}
}

// Add records m under its app. A migration with the same app and name
// replaces the earlier one.
func (r *Registry) Add(m Migration) {
	if _, ok := r.byApp[m.App]; !ok {
		r.appOrder = append(r.appOrder, m.App)
	}
	for i := range r.byApp[m.App] {
		if r.byApp[m.App][i].Name == m.Name {
			r.byApp[m.App][i] = m
			return
		}
	}
	r.byApp[m.App] = append(r.byApp[m.App], m)
}

// ForApp returns the migrations registered for app in insertion order.
func (r *Registry) ForApp(app string) []Migration {
	return r.byApp[app]
}

// All returns every migration, ordered by app (first seen) and then by
// insertion.
func (r *Registry) All() []Migration {
	var out []Migration
	for _, app := range r.appOrder {
		out = append(out, r.byApp[app]...)
	}
	return out
}

// Apps returns the app labels that have at least one migration, first seen first.
func (r *Registry) Apps() []string {
	out := make([]string, len(r.appOrder))
	copy(out, r.appOrder)
	return out
}
