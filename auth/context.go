package auth

import (
	"net/http"
	"strings"
	"sync"
)

// ContextProcessor adds the user of the request and the permissions of that
// user to the context of a template rendered for r (Django's
// django.contrib.auth.context_processors.auth). Register it with the template
// engine:
//
//   - "user" is the user as a TemplateUser, the anonymous user when nobody is
//     logged in: {{ user.username }}, {% if user.is_authenticated %}.
//   - "perms" is a TemplatePerms: {% if perms.blog %} tests whether the user
//     holds any permission of the app labeled blog, and
//     {% if perms.blog.add_post %} or {% if 'blog.add_post' in perms %} tests
//     one permission.
//
// Both values are loaded when a template first uses them, so a template that
// does not use them costs no query and leaves the session unread. The user
// comes from GetUser and the permissions from GetAllPermissions, read from the
// database given to Middleware; without Middleware the request has no
// permissions. A failure to load either fails the rendering.
//
// Django asks has_perm for every lookup, so an active superuser holds even a
// permission that does not exist. Here perms holds the permissions the
// backends list, which for an active superuser are all permissions in the
// database. A backend that decides permissions itself (PermissionChecker)
// without listing them is not reflected; call HasPerm in the view for those.
func ContextProcessor(r *http.Request) map[string]any {
	ctx := r.Context()
	user := sync.OnceValues(func() (*User, error) { return GetUser(r) })
	return map[string]any{
		"user": sync.OnceValues(func() (TemplateUser, error) {
			u, err := user()
			if err != nil {
				return nil, err
			}
			return templateUser(u), nil
		}),
		"perms": sync.OnceValues(func() (TemplatePerms, error) {
			u, err := user()
			if err != nil {
				return nil, err
			}
			db := requestDB(ctx)
			if db == nil {
				return TemplatePerms{}, nil
			}
			perms, err := GetAllPermissions(ctx, db, u)
			if err != nil {
				return nil, err
			}
			return templatePerms(perms), nil
		}),
	}
}

// TemplateUser is a user as templates see it: a map with the attribute names
// of Django's User, so that {{ user.username }} and
// {% if user.is_authenticated %} work. The keys are "pk", "id", "username",
// "first_name", "last_name", "email", "is_staff", "is_active",
// "is_superuser", "is_authenticated", "is_anonymous", "last_login",
// "date_joined", "get_username", "get_full_name" and "get_short_name". The
// password hash is left out. For the anonymous user "pk", "id", "last_login"
// and "date_joined" are nil.
//
// {{ user }} prints the username, or "AnonymousUser" (Django's str(user)),
// escaped for HTML.
type TemplateUser map[string]any

// templateUser returns the template view of u.
func templateUser(u *User) TemplateUser {
	t := TemplateUser{
		"username":         u.Username,
		"first_name":       u.FirstName,
		"last_name":        u.LastName,
		"email":            u.Email,
		"is_staff":         u.IsStaff,
		"is_active":        u.IsActive,
		"is_superuser":     u.IsSuperuser,
		"is_authenticated": u.IsAuthenticated(),
		"is_anonymous":     u.IsAnonymous(),
		"get_username":     u.GetUsername(),
		"get_full_name":    u.GetFullName(),
		"get_short_name":   u.GetShortName(),
		"pk":               nil,
		"id":               nil,
		"last_login":       nil,
		"date_joined":      nil,
	}
	if u.IsAuthenticated() {
		t["pk"], t["id"] = u.ID, u.ID
		t["date_joined"] = u.DateJoined
		if !u.LastLogin.IsZero() {
			t["last_login"] = u.LastLogin
		}
	}
	return t
}

// label returns Django's str(user): the username, or "AnonymousUser".
func (t TemplateUser) label() string {
	if anon, _ := t["is_anonymous"].(bool); anon {
		return "AnonymousUser"
	}
	s, _ := t["username"].(string)
	return s
}

// HTML returns the username, or "AnonymousUser", escaped for HTML. A template
// engine that prints a value with an HTML method as it is (Django's __html__
// protocol) prints this.
func (t TemplateUser) HTML() string { return escapeHTML(t.label()) }

// String returns the same text as HTML, so that a template engine that prints
// a fmt.Stringer without escaping it still prints escaped text. Apply filters
// to user.username rather than to user.
func (t TemplateUser) String() string { return t.HTML() }

// TemplatePerms is the permissions of a user as templates see them (Django's
// PermWrapper). It maps the label of every app in which the user holds a
// permission to a map of the codenames held there, and every held permission
// "<app_label>.<codename>" to true. So perms.blog is true when the user holds
// any permission of blog, perms.blog.add_post when it holds that one, and the
// in operator accepts both forms. A lookup of anything else gives nothing,
// which counts as false.
type TemplatePerms map[string]any

// templatePerms builds the template view of a list of permissions given as
// "<app_label>.<codename>".
func templatePerms(perms []string) TemplatePerms {
	out := make(TemplatePerms, 2*len(perms))
	for _, perm := range perms {
		app, codename, ok := strings.Cut(perm, ".")
		if !ok {
			continue
		}
		codenames, _ := out[app].(map[string]bool)
		if codenames == nil {
			codenames = make(map[string]bool)
			out[app] = codenames
		}
		codenames[codename] = true
		out[perm] = true
	}
	return out
}

// htmlEscaper escapes the five characters Django's escape() does, with
// Django's entities.
var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;")

// escapeHTML escapes s for HTML text and attribute values like Django.
func escapeHTML(s string) string { return htmlEscaper.Replace(s) }
