package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/urls"
)

// adminHooks hold the behavior of the admin of users that the fields of
// ModelAdmin do not express, the methods Django's UserAdmin overrides.
type adminHooks struct {
	// routes returns views of the admin, mounted before its standard ones
	// (Django's get_urls).
	routes func(s *AdminSite, e *entry) []urls.Route
	// addFieldsets are the fieldsets of the add page when GetFieldsets is
	// not set (UserAdmin.add_fieldsets).
	addFieldsets []Fieldset
	// allowAdd answers a request of the add page that must not go on and
	// reports whether the view goes on (UserAdmin._add_view).
	allowAdd func(s *AdminSite, w http.ResponseWriter, r *http.Request, e *entry) bool
	// beforeCreate readies the object that the valid add form filled before
	// it is saved.
	beforeCreate func(form *forms.Form, obj any) error
	// continueAfterAdd makes the Save button of the add page continue
	// editing, as UserAdmin.response_add does, unless the user adds another
	// or works in a popup.
	continueAfterAdd bool
	// lookupAllowed rejects lookups of the changelist that the admin of the
	// model does not allow (lookup_allowed).
	lookupAllowed func(lookup string) bool
}

// userFieldsets are the fieldsets of the change page of a user, Django's
// UserAdmin.fieldsets.
var userFieldsets = []Fieldset{
	{Fields: []any{"username", "password"}},
	{Name: "Personal info", Fields: []any{"first_name", "last_name", "email"}},
	{Name: "Permissions", Fields: []any{"is_active", "is_staff", "is_superuser", "groups", "user_permissions"}},
	{Name: "Important dates", Fields: []any{"last_login", "date_joined"}},
}

// userAddFieldsets are the fieldsets of the add page of a user, Django's
// UserAdmin.add_fieldsets.
var userAddFieldsets = []Fieldset{
	{Classes: []string{"wide"}, Fields: []any{"username", "usable_password", "password1", "password2"}},
}

// UserAdmin returns the admin of auth.User, Django's UserAdmin. The add page
// asks for a username and a password, with the form of
// auth.AdminUserCreationFormOptions; Save goes on to the change page, where
// the form of auth.UserChangeFormOptions shows the password as a summary of
// its hash with a link to the page that sets a new one. That page has the URL
// name "auth_user_password_change". Only a user who can change users can add
// them. The changelist rejects lookups on the password.
//
// The application registers it when the auth app is installed and an app
// registers models with the admin. To customize it, change the returned value
// and register it again, which replaces the earlier registration:
//
//	ma := admin.UserAdmin(db)
//	ma.ListDisplay = append(ma.ListDisplay, "is_active")
//	admin.Register[auth.User](site, ma)
func UserAdmin(db *orm.DB) ModelAdmin {
	return ModelAdmin{
		Fieldsets: userFieldsets,
		GetFormOptions: func(_ *http.Request, obj any) []forms.ModelFormOption {
			if obj == nil {
				return auth.AdminUserCreationFormOptions(db)
			}
			user, _ := obj.(*auth.User)
			return auth.UserChangeFormOptions(user)
		},
		ListDisplay:      []string{"username", "email", "first_name", "last_name", "is_staff"},
		ListFilter:       []any{"is_staff", "is_superuser", "is_active", "groups"},
		SearchFields:     []string{"username", "first_name", "last_name", "email"},
		Ordering:         []string{"username"},
		FilterHorizontal: []string{"groups", "user_permissions"},
		AddFormTemplate:  "admin/auth/user/add_form.html",
		hooks: &adminHooks{
			routes:           userRoutes,
			addFieldsets:     userAddFieldsets,
			allowAdd:         allowUserAdd,
			beforeCreate:     prepareNewUser,
			continueAfterAdd: true,
			lookupAllowed: func(lookup string) bool {
				return !strings.HasPrefix(lookup, "password")
			},
		},
	}
}

// GroupAdmin returns the admin of auth.Group, Django's GroupAdmin: groups
// are searched and ordered by name, and the permissions are chosen from two
// lists side by side.
func GroupAdmin() ModelAdmin {
	return ModelAdmin{
		SearchFields:     []string{"name"},
		Ordering:         []string{"name"},
		FilterHorizontal: []string{"permissions"},
	}
}

// userRoutes returns the page that sets the password of a user, Django's
// UserAdmin.get_urls.
func userRoutes(s *AdminSite, e *entry) []urls.Route {
	return []urls.Route{
		urls.Path(e.routePrefix()+"<id>/password/", s.adminView(func(w http.ResponseWriter, r *http.Request) {
			s.userChangePassword(w, r, e)
		}, false), "auth_user_password_change"),
	}
}

// allowUserAdd answers a request of the add page of users from a user who
// cannot change users, Django's UserAdmin._add_view: such a user could
// create a superuser. In Debug mode a user who can add users gets a 404 that
// explains the rule, any other user a 403.
func allowUserAdd(s *AdminSite, w http.ResponseWriter, r *http.Request, e *entry) bool {
	if e.hasChangePermission(s, r, nil) {
		return true
	}
	if e.hasAddPermission(s, r) && s.Debug {
		http.Error(w, `Your user does not have the "Change user" permission. In order to add users, `+
			`Django requires that your user account have both the "Add user" and "Change user" permissions set.`,
			http.StatusNotFound)
		return false
	}
	forbidden(w)
	return false
}

// prepareNewUser readies the user of a valid add form, with
// auth.PrepareNewUser.
func prepareNewUser(form *forms.Form, obj any) error {
	user, ok := obj.(*auth.User)
	if !ok {
		return fmt.Errorf("admin: the admin of users got a %T", obj)
	}
	return auth.PrepareNewUser(user, form)
}

// userChangePassword handles the page that sets the password of a user,
// Django's UserAdmin.user_change_password, with the form of
// auth.AdminPasswordChangeForm. Disabling password-based authentication
// takes the button "unset-password"; a form that asks for it with the other
// button is rejected with a message, as the two can be submitted together
// without JavaScript.
func (s *AdminSite) userChangePassword(w http.ResponseWriter, r *http.Request, e *entry) {
	id := r.PathValue("id")
	obj, _, err := s.loadObject(r.Context(), e, id)
	if err != nil {
		http.Error(w, "failed to load row", http.StatusInternalServerError)
		return
	}
	if !e.hasChangePermission(s, r, obj) {
		forbidden(w)
		return
	}
	user, ok := obj.(*auth.User)
	if !ok {
		singular, _ := modelVerboseNames(e.model)
		s.notFound(w, r, fmt.Sprintf("%s object with primary key %s does not exist.", singular, pyRepr(id)))
		return
	}
	data, ok := postData(w, r)
	if !ok {
		return
	}
	form := auth.NewAdminPasswordChangeForm(user)
	if data != nil {
		form = form.Bind(data)
		form.SetContext(r.Context())
		if form.IsValid() {
			if usable, _ := form.CleanedData()["set_usable_password"].(bool); !usable && !data.Has("unset-password") {
				_ = messages.Error(r, "Conflicting form data submitted. Please try again.")
				http.Redirect(w, r, r.URL.RequestURI(), http.StatusFound)
				return
			}
			if _, err := form.Save(r.Context(), s.db); err != nil {
				http.Error(w, "failed to change the password", http.StatusInternalServerError)
				return
			}
			if err := s.logPasswordChange(r, user, form); err != nil {
				http.Error(w, "failed to log the change", http.StatusInternalServerError)
				return
			}
			msg := "Password changed successfully."
			if !user.HasUsablePassword() {
				msg = "Password-based authentication was disabled."
			}
			_ = messages.Success(r, msg)
			if err := auth.UpdateSessionAuthHash(r, user); err != nil {
				http.Error(w, "failed to update the session", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, s.changeURL(e, user.ID), http.StatusFound)
			return
		}
	}
	s.renderUserChangePassword(w, r, e, user, form)
}

// renderUserChangePassword renders the page of userChangePassword.
func (s *AdminSite) renderUserChangePassword(w http.ResponseWriter, r *http.Request, e *entry, user *auth.User, form *auth.AdminPasswordChangeForm) {
	_, plural := modelVerboseNames(e.model)
	usable := user.HasUsablePassword()
	ctx := s.EachContext(r)
	if usable {
		ctx["title"] = "Change password: " + user.GetUsername()
	} else {
		ctx["title"] = "Set password: " + user.GetUsername()
	}
	ctx["subtitle"] = ""
	ctx["has_usable_password"] = usable
	ctx["username"] = user.GetUsername()
	ctx["original"] = user
	ctx["original_label"] = displayLabel(e.model, user)
	ctx["original_url"] = s.changeURL(e, user.ID)
	ctx["form"] = form.Form
	ctx["form_url"] = ""
	ctx["errors"] = 0
	if form.IsBound() {
		ctx["errors"] = form.Errors().Len()
	}
	popup := isPopup(r)
	ctx["is_popup"] = popup
	ctx["is_popup_var"] = isPopupVar
	ctx["app_label"] = e.app
	ctx["app_verbose_name"] = s.appVerboseName(e.app)
	ctx["app_url"] = s.appURLOrEmpty(e.app)
	ctx["model_name"] = e.slug
	ctx["verbose_name_plural"] = capFirst(plural)
	ctx["changelist_url"] = s.changelistURL(e)
	s.render(w, r, "admin/auth/user/change_password.html", ctx)
}

// notFound answers 404 with msg in Debug mode, Django's Http404 with a
// message on the technical 404 page, and with the plain 404 otherwise.
func (s *AdminSite) notFound(w http.ResponseWriter, r *http.Request, msg string) {
	if s.Debug {
		http.Error(w, msg, http.StatusNotFound)
		return
	}
	http.NotFound(w, r)
}

// pyRepr returns the printable text s quoted as Python's repr does, the %r
// of Django's messages: in single quotes, or in double quotes when s holds a
// single quote and no double quote, with the quote and backslashes escaped.
// The message is plain text, so the HTML escaping of Django's message is
// left out.
func pyRepr(s string) string {
	quote := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = `"`
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	return quote + strings.ReplaceAll(s, quote, `\`+quote) + quote
}

// logPasswordChange records the change of the password of user, as Django's
// user_change_password does with construct_change_message and log_change:
// the message is "Changed password.".
func (s *AdminSite) logPasswordChange(r *http.Request, user *auth.User, form *auth.AdminPasswordChangeForm) error {
	if !s.logEnabled() {
		return nil
	}
	cm, err := constructChangeMessage(r.Context(), s.db, form.Form, form.ChangedData(), nil, false)
	if err != nil {
		return err
	}
	return s.logActions(r.Context(), r, []any{user}, Change, cm.String())
}
