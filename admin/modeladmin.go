// Package admin provides a Django-style admin site: an AdminSite that registers
// models, gates access to staff users, and renders index, changelist, and the
// form-driven add, change, and delete write views from embedded pongo2 templates.
package admin

// ModelAdmin is the per-model admin customization surface. A zero value is a
// valid configuration that displays every field with no default ordering.
type ModelAdmin struct {
	// ListDisplay names the Field.Name columns shown on the changelist;
	// an empty slice means show every field.
	ListDisplay []string
	// Ordering is the default OrderBy applied to the changelist
	// (e.g. "-id"); an empty slice means no ordering.
	Ordering []string
	// SearchFields names the fields searched by the changelist search box.
	// Search execution may be added later; the value is stored for now.
	SearchFields []string
	// ReadonlyFields names fields rendered read-only on the change form.
	ReadonlyFields []string
	// ExcludeFields names fields omitted from the add and change forms.
	ExcludeFields []string
}
