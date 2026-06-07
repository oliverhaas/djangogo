package auth

import (
	"context"
	"net/http"
	"net/url"

	"github.com/oliverhaas/djangogo/orm"
)

// LoginRequired returns middleware that redirects anonymous requests to loginURL
// with a 302 and a ?next=<request path> query parameter, and otherwise calls the
// wrapped handler. A user is considered authenticated when CurrentUser finds one
// in the request context (so auth.Middleware must run first).
func LoginRequired(loginURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := CurrentUser(r.Context()); !ok {
				redirectToLogin(w, r, loginURL)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// redirectToLogin issues a 302 to loginURL carrying the current request path in a
// next query parameter so the login flow can return the user afterwards.
func redirectToLogin(w http.ResponseWriter, r *http.Request, loginURL string) {
	target := loginURL + "?next=" + url.QueryEscape(r.URL.Path)
	http.Redirect(w, r, target, http.StatusFound)
}

// HasPerm reports whether u holds the permission identified by codename.
// Superusers always pass. Otherwise the permission is granted when u has it
// directly (via UserPermission) or through any of u's groups (via UserGroup then
// GroupPermission).
func HasPerm(ctx context.Context, db *orm.DB, u *User, codename string) (bool, error) {
	if u.IsSuperuser {
		return true, nil
	}

	permIDs, err := permissionIDs(ctx, db, codename)
	if err != nil {
		return false, err
	}
	if len(permIDs) == 0 {
		return false, nil
	}

	// Direct user permissions.
	direct, err := orm.Query[UserPermission](db).
		Filter("user_id", u.ID, "permission_id__in", permIDs).
		Exists(ctx)
	if err != nil {
		return false, err
	}
	if direct {
		return true, nil
	}

	// Group permissions: collect the user's groups, then look for a grant.
	groupIDs, err := userGroupIDs(ctx, db, u.ID)
	if err != nil {
		return false, err
	}
	if len(groupIDs) == 0 {
		return false, nil
	}
	return orm.Query[GroupPermission](db).
		Filter("group_id__in", groupIDs, "permission_id__in", permIDs).
		Exists(ctx)
}

// permissionIDs returns the ids of every Permission whose codename matches.
func permissionIDs(ctx context.Context, db *orm.DB, codename string) ([]int64, error) {
	perms, err := orm.Query[Permission](db).Filter("codename", codename).All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(perms))
	for i := range perms {
		ids[i] = perms[i].ID
	}
	return ids, nil
}

// userGroupIDs returns the ids of every Group the user belongs to.
func userGroupIDs(ctx context.Context, db *orm.DB, userID int64) ([]int64, error) {
	links, err := orm.Query[UserGroup](db).Filter("user_id", userID).All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(links))
	for i := range links {
		ids[i] = links[i].Group.PK()
	}
	return ids, nil
}

// PermissionRequired returns middleware that guards the wrapped handler by
// codename: anonymous requests are redirected to loginURL, an authenticated user
// lacking the permission gets a 403, and a user holding it proceeds.
func PermissionRequired(db *orm.DB, codename, loginURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			u, ok := CurrentUser(ctx)
			if !ok {
				redirectToLogin(w, r, loginURL)
				return
			}
			allowed, err := HasPerm(ctx, db, u, codename)
			if err != nil {
				http.Error(w, "permission check failed", http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
