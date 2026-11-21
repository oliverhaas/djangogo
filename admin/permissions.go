package admin

import (
	"net/http"
	"strings"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/orm"
)

// permName returns the auth permission string "<app_label>.<action>_<model>"
// for m and action ("add", "change", "delete", or "view").
func permName(m *orm.Model, action string) string {
	return m.App() + "." + action + "_" + strings.ToLower(m.Name())
}

// userHasPerm reports whether the request's user holds perm in s.db.
func (s *AdminSite) userHasPerm(r *http.Request, perm string) bool {
	u, err := auth.GetUser(r)
	if err != nil || !u.IsAuthenticated() {
		return false
	}
	ok, err := auth.HasPerm(r.Context(), s.db, u, perm)
	return err == nil && ok
}

// userHasModulePerms reports whether the request's user holds any permission
// for appLabel in s.db.
func (s *AdminSite) userHasModulePerms(r *http.Request, appLabel string) bool {
	u, err := auth.GetUser(r)
	if err != nil || !u.IsAuthenticated() {
		return false
	}
	ok, err := auth.HasModulePerms(r.Context(), s.db, u, appLabel)
	return err == nil && ok
}

// hasAddPermission reports whether r may add instances of e's model.
func (e *entry) hasAddPermission(s *AdminSite, r *http.Request) bool {
	if e.admin.HasAddPermission != nil {
		return e.admin.HasAddPermission(r)
	}
	return s.userHasPerm(r, permName(e.model, "add"))
}

// hasChangePermission reports whether r may change obj (or any instance of e's
// model when obj is nil).
func (e *entry) hasChangePermission(s *AdminSite, r *http.Request, obj any) bool {
	if e.admin.HasChangePermission != nil {
		return e.admin.HasChangePermission(r, obj)
	}
	return s.userHasPerm(r, permName(e.model, "change"))
}

// hasDeletePermission reports whether r may delete obj (or any instance of e's
// model when obj is nil).
func (e *entry) hasDeletePermission(s *AdminSite, r *http.Request, obj any) bool {
	if e.admin.HasDeletePermission != nil {
		return e.admin.HasDeletePermission(r, obj)
	}
	return s.userHasPerm(r, permName(e.model, "delete"))
}

// hasViewPermission reports whether r may view obj (or any instance of e's
// model when obj is nil). In Django, change permission implies view permission.
func (e *entry) hasViewPermission(s *AdminSite, r *http.Request, obj any) bool {
	if e.admin.HasViewPermission != nil {
		return e.admin.HasViewPermission(r, obj)
	}
	return s.userHasPerm(r, permName(e.model, "view")) ||
		s.userHasPerm(r, permName(e.model, "change"))
}

// hasViewOrChangePermission reports whether r has either view or change
// permission for obj (or for e's model when obj is nil).
func (e *entry) hasViewOrChangePermission(s *AdminSite, r *http.Request, obj any) bool {
	return e.hasViewPermission(s, r, obj) || e.hasChangePermission(s, r, obj)
}

// hasModulePermission reports whether r may see e's app on the admin index and
// visit that app's index page.
func (e *entry) hasModulePermission(s *AdminSite, r *http.Request) bool {
	if e.admin.HasModulePermission != nil {
		return e.admin.HasModulePermission(r)
	}
	return s.userHasModulePerms(r, e.model.App())
}

// modelPerms returns the four standard permission flags for e's model, matching
// Django's ModelAdmin.get_model_perms.
func (e *entry) modelPerms(s *AdminSite, r *http.Request) map[string]bool {
	return map[string]bool{
		"add":    e.hasAddPermission(s, r),
		"change": e.hasChangePermission(s, r, nil),
		"delete": e.hasDeletePermission(s, r, nil),
		"view":   e.hasViewPermission(s, r, nil),
	}
}
