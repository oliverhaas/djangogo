// Package admin provides a Django-style admin site: an AdminSite that registers
// models, gates access to active staff users, enforces per-model permissions,
// and renders the app-grouped index, app index, changelist, and add, change,
// delete, and history views from embedded pongo2 templates.
package admin

import "net/http"

// ModelAdmin is the per-model admin customization surface. A zero value is a
// valid configuration that displays every field with no default ordering and
// checks Django's default auth permissions (<app>.add_<model>, etc.).
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
	// ReadonlyFields names fields the change form displays as read-only values
	// and omits from editable inputs (preserving their stored values on save).
	ReadonlyFields []string
	// ExcludeFields names fields omitted from the add and change forms.
	ExcludeFields []string

	// HasAddPermission overrides the default "<app>.add_<model>" check
	// (Django's ModelAdmin.has_add_permission).
	HasAddPermission func(r *http.Request) bool
	// HasChangePermission overrides the default "<app>.change_<model>" check
	// (Django's ModelAdmin.has_change_permission). obj is nil for model-level
	// checks and *T for object-level checks.
	HasChangePermission func(r *http.Request, obj any) bool
	// HasDeletePermission overrides the default "<app>.delete_<model>" check
	// (Django's ModelAdmin.has_delete_permission). obj is nil for model-level
	// checks and *T for object-level checks.
	HasDeletePermission func(r *http.Request, obj any) bool
	// HasViewPermission overrides the default "<app>.view_<model>" or
	// "<app>.change_<model>" check (Django's ModelAdmin.has_view_permission).
	// obj is nil for model-level checks and *T for object-level checks.
	HasViewPermission func(r *http.Request, obj any) bool
	// HasModulePermission overrides the default app-level module permission
	// check (Django's ModelAdmin.has_module_permission).
	HasModulePermission func(r *http.Request) bool
}
