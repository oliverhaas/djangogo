package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Messages and help texts of the user forms, in Django's wording.
const (
	msgNoPasswordSet     = "No password set."
	msgInvalidPassword   = "Invalid password format or unknown hashing algorithm."
	msgUsernameTaken     = "A user with that username already exists."
	helpPasswordConfirm  = "Enter the same password as before, for verification."
	helpPasswordReadOnly = "Raw passwords are not stored, so there is no way to see the user’s password."
	helpPasswordUnusable = "Enable password-based authentication for this user by setting a password."
	helpUsablePassword   = "Whether the user will be able to authenticate using a password or not. " +
		"If disabled, they may still be able to authenticate using other backends, " +
		"such as Single Sign-On or LDAP."
	// helpUsablePasswordWarning is the help text of the usable_password field
	// of AdminPasswordChangeForm.
	helpUsablePasswordWarning = helpUsablePassword +
		`<ul id="id_unusable_warning" class="messagelist"><li class="warning">` +
		"If disabled, the current password for this user will be lost.</li></ul>"
)

// RenderPasswordAsHash returns the HTML summary of an encoded password that
// the user admin shows instead of the hash, Django's render_password_as_hash
// template tag: the parts of Hasher.SafeSummary with the salt and the hash
// masked. An empty or unusable password renders "No password set.", a value
// that no configured hasher reads a notice of the invalid format.
func RenderPasswordAsHash(encoded string) string {
	if encoded == "" || !IsPasswordUsable(encoded) {
		return "<p><strong>" + msgNoPasswordSet + "</strong></p>"
	}
	invalid := "<p><strong>" + msgInvalidPassword + "</strong></p>"
	hasher, err := IdentifyHasher(encoded)
	if err != nil {
		return invalid
	}
	items, ok := hasher.SafeSummary(encoded)
	if !ok {
		return invalid
	}
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = "<strong>" + forms.Escape(item.Key) + "</strong>: <bdi>" + forms.Escape(item.Value) + "</bdi>"
	}
	return "<p>" + strings.Join(parts, " ") + "</p>"
}

// ReadOnlyPasswordHashWidget shows the summary of an encoded password with a
// link to the page that sets a new one, Django's ReadOnlyPasswordHashWidget.
// It renders no input. The embedded TextInput supplies the attributes and the
// handling of submitted data.
type ReadOnlyPasswordHashWidget struct {
	forms.TextInput
}

// Render returns a <div> with the summary of RenderPasswordAsHash and a link
// to "../password/": "Reset password" for a usable password, "Set password"
// otherwise.
func (w ReadOnlyPasswordHashWidget) Render(_ string, value any, attrs forms.Attrs) string {
	encoded := ""
	if value != nil {
		encoded = fmt.Sprint(value)
	}
	label := "Set password"
	if encoded != "" && IsPasswordUsable(encoded) {
		label = "Reset password"
	}
	merged := forms.Attrs{}
	for k, v := range w.Attrs {
		merged[k] = v
	}
	for k, v := range attrs {
		merged[k] = v
	}
	return "<div" + forms.RenderAttrs(merged) + ">\n  " + RenderPasswordAsHash(encoded) +
		"\n  <p><a role=\"button\" class=\"button\" href=\"../password/\">" + label + "</a></p>\n</div>"
}

// IDForLabel returns "": the widget has no element a <label> can point at.
func (ReadOnlyPasswordHashWidget) IDForLabel(string) string { return "" }

// ReadOnlyPasswordHashField shows the encoded password of a user without a
// way to change it, Django's ReadOnlyPasswordHashField. It is disabled, so a
// bound form keeps the initial value.
type ReadOnlyPasswordHashField struct {
	forms.BaseField
}

// NewReadOnlyPasswordHashField returns a ReadOnlyPasswordHashField with a
// ReadOnlyPasswordHashWidget. It is optional and disabled; opts apply after
// these defaults.
func NewReadOnlyPasswordHashField(opts ...forms.FieldOption) *ReadOnlyPasswordHashField {
	f := &ReadOnlyPasswordHashField{}
	defaults := []forms.FieldOption{forms.Required(false), forms.Disabled()}
	f.Init(f, ReadOnlyPasswordHashWidget{}, nil, slices.Concat(defaults, opts)...)
	return f
}

// UsernameField is the CharField of a username, Django's UsernameField: it
// normalizes the value to Unicode NFKC and asks browsers not to capitalize
// or complete it as anything but a username.
type UsernameField struct {
	*forms.CharField

	// autofocus adds the autofocus attribute, as the user creation forms do.
	autofocus bool
}

// NewUsernameField returns a UsernameField. It reads the options of
// forms.NewCharField.
func NewUsernameField(opts ...forms.FieldOption) *UsernameField {
	return &UsernameField{CharField: forms.NewCharField(opts...)}
}

// newAutofocusUsernameField returns a UsernameField with the autofocus
// attribute, the username field of Django's BaseUserCreationForm.
func newAutofocusUsernameField(opts ...forms.FieldOption) *UsernameField {
	f := NewUsernameField(opts...)
	f.autofocus = true
	return f
}

// ToGo returns the value of the CharField in Unicode NFKC. A value longer
// than MaxLength stays as it is: normalization cannot make it shorter, and
// the maximum length rejects it anyway.
func (f *UsernameField) ToGo(value any) (any, error) {
	v, err := f.CharField.ToGo(value)
	if err != nil {
		return v, err
	}
	s, ok := v.(string)
	if !ok || (f.MaxLength > 0 && utf8.RuneCountInString(s) > f.MaxLength) {
		return v, nil
	}
	return norm.NFKC.String(s), nil
}

// Clean converts the value with ToGo and validates it.
func (f *UsernameField) Clean(value any) (any, error) {
	v, err := f.ToGo(value)
	if err != nil {
		return nil, err
	}
	if err := f.Validate(v); err != nil {
		return nil, err
	}
	if err := f.RunValidators(v); err != nil {
		return nil, err
	}
	return v, nil
}

// HasChanged compares the initial value with the data after ToGo.
func (f *UsernameField) HasChanged(initial, data any) bool {
	v, err := f.ToGo(data)
	if err != nil {
		return true
	}
	return f.CharField.HasChanged(initial, v)
}

// WidgetAttrs adds autocapitalize="none" and autocomplete="username" to the
// attributes of the CharField.
func (f *UsernameField) WidgetAttrs(w forms.Widget) forms.Attrs {
	attrs := f.CharField.WidgetAttrs(w)
	attrs["autocapitalize"] = "none"
	attrs["autocomplete"] = "username"
	if f.autofocus {
		attrs["autofocus"] = true
	}
	return attrs
}

// newPasswordFields returns the new password and its confirmation, Django's
// SetPasswordMixin.create_password_fields. The help text of the first lists
// the rules of the configured password validators.
func newPasswordFields() (password1, password2 *forms.CharField) {
	field := func(label, help string) *forms.CharField {
		return forms.NewCharField(
			forms.Label(label),
			forms.Strip(false),
			forms.UseWidget(forms.PasswordInput{Attrs: forms.Attrs{"autocomplete": "new-password"}}),
			forms.HelpText(help),
		)
	}
	return field("Password", PasswordValidatorsHelpTextHTML(nil)),
		field("Password confirmation", helpPasswordConfirm)
}

// newUsablePasswordField returns the choice between a password and no
// password, Django's SetUnusablePasswordMixin.create_usable_password_field.
func newUsablePasswordField(help string) *forms.ChoiceField {
	return forms.NewChoiceField(
		forms.Label("Password-based authentication"),
		forms.Required(false),
		forms.Initial("true"),
		forms.Choices("true", "Enabled", "false", "Disabled"),
		forms.UseWidget(forms.RadioSelect{Attrs: forms.Attrs{"class": "radiolist"}}),
		forms.HelpText(help),
	)
}

// cleanPasswords reports a password2 that differs from password1, Django's
// SetPasswordMixin.validate_passwords.
func cleanPasswords(form *forms.Form) {
	cleaned := form.CleanedData()
	password1, _ := cleaned["password1"].(string)
	password2, _ := cleaned["password2"].(string)
	if password1 != "" && password2 != "" && password1 != password2 {
		form.AddError("password2", forms.NewValidationError(msgPasswordMismatch, "password_mismatch"))
	}
}

// cleanOptionalPasswords is cleanPasswords for a form that can leave the user
// without a password, Django's SetUnusablePasswordMixin.validate_passwords.
// It replaces usable_password in the cleaned data with the bool
// set_usable_password. Without a usable password the password fields are
// not checked; with one they are required.
func cleanOptionalPasswords(form *forms.Form) {
	cleaned := form.CleanedData()
	usable := cleaned["usable_password"] != "false"
	delete(cleaned, "usable_password")
	cleaned["set_usable_password"] = usable
	if !usable {
		return
	}
	for _, name := range []string{"password1", "password2"} {
		if password, _ := cleaned[name].(string); password != "" || form.HasError(name) {
			continue
		}
		field, ok := form.FieldDef(name)
		if !ok {
			continue
		}
		form.AddError(name, forms.NewValidationError(field.Base().ErrorMessages["required"], "required"))
	}
	cleanPasswords(form)
}

// validatePasswordForUser runs the password validators over password2 for
// user and reports their complaints on password2, Django's
// validate_password_for_user. A form that sets no usable password skips it.
func validatePasswordForUser(form *forms.Form, user *User) {
	cleaned := form.CleanedData()
	if usable, ok := cleaned["set_usable_password"].(bool); ok && !usable {
		return
	}
	password, _ := cleaned["password2"].(string)
	if password == "" {
		return
	}
	var refused PasswordErrors
	if err := ValidatePassword(password, user, nil); !errors.As(err, &refused) {
		return
	}
	errs := make([]any, len(refused))
	for i, pe := range refused {
		errs[i] = forms.NewValidationError(pe.Message, pe.Code)
	}
	form.AddError("password2", forms.ValidationErrors(errs...))
}

// SetPasswordFromForm sets the password of user from the cleaned data of a
// valid user creation or admin password form, Django's set_password_and_save
// with commit=False: an unusable password when the form disabled
// password-based authentication, password1 otherwise. It does not save the
// user.
func SetPasswordFromForm(user *User, form *forms.Form) error {
	cleaned := form.CleanedData()
	if usable, ok := cleaned["set_usable_password"].(bool); ok && !usable {
		return user.SetUnusablePassword()
	}
	password, _ := cleaned["password1"].(string)
	return user.SetPassword(password)
}

// UserCreationFormOptions returns the options of a model form of User that
// creates a user from a username and a password, Django's UserCreationForm:
// the username as a UsernameField with autofocus, the fields password1 and
// password2, the rejection of a username that differs from a stored one only
// in case, and the password validators. Add the fields with forms.WithFields;
// NewUserCreationForm does that for the username. Save the user with
// SetPasswordFromForm, as UserCreationForm.Save does.
func UserCreationFormOptions(db *orm.DB) []forms.ModelFormOption {
	return creationFormOptions(db, false)
}

// AdminUserCreationFormOptions returns the options of UserCreationFormOptions
// with the choice of password-based authentication, Django's
// AdminUserCreationForm: the field usable_password after the password
// fields, which are optional. A user created without a usable password gets
// an unusable one.
func AdminUserCreationFormOptions(db *orm.DB) []forms.ModelFormOption {
	return creationFormOptions(db, true)
}

// creationFormOptions returns the options of the user creation forms; admin
// selects those of the admin.
func creationFormOptions(db *orm.DB, admin bool) []forms.ModelFormOption {
	password1, password2 := newPasswordFields()
	opts := []forms.ModelFormOption{
		forms.WithFieldClass("username", newAutofocusUsernameField),
		forms.WithFormField("password1", password1),
		forms.WithFormField("password2", password2),
		forms.WithCleanField("username", func(form *forms.Form, value any) (any, error) {
			return cleanNewUsername(db, form, value)
		}),
	}
	cleanPasswordFields := cleanPasswords
	if admin {
		password1.Required, password2.Required = false, false
		opts = append(opts, forms.WithFormField("usable_password", newUsablePasswordField(helpUsablePassword)))
		cleanPasswordFields = cleanOptionalPasswords
	}
	return append(opts, forms.WithClean(func(form *forms.Form) error {
		cleanPasswordFields(form)
		validatePasswordForUser(form, userFromCleanedData(db, form))
		return nil
	}))
}

// cleanNewUsername rejects a username that differs from a stored one only in
// case, Django's UserCreationForm.clean_username. The message is the one of
// the unique error of the username, or the form field's "unique" message.
func cleanNewUsername(db *orm.DB, form *forms.Form, value any) (any, error) {
	username, _ := value.(string)
	if username == "" {
		return value, nil
	}
	taken, err := orm.Query[User](db).Filter("username__iexact", username).Exists(form.Context())
	if err != nil {
		return nil, fmt.Errorf("auth: check username: %w", err)
	}
	if !taken {
		return value, nil
	}
	msg := msgUsernameTaken
	if field, ok := form.FieldDef("username"); ok {
		if custom, ok := field.Base().ErrorMessages["unique"]; ok {
			msg = custom
		}
	}
	return nil, forms.NewValidationError(msg, "unique")
}

// userFromCleanedData returns a new user with the cleaned data of form, the
// instance that Django's BaseUserCreationForm validates the password for
// after the model form filled it.
func userFromCleanedData(db *orm.DB, form *forms.Form) *User {
	user := &User{}
	if model, ok := db.Registry().ModelOf(user); ok {
		// A value that does not fit leaves the field empty, as the model
		// validation reports it.
		_ = forms.PopulateStruct(model, form.CleanedData(), user)
	}
	return user
}

// UserCreationForm creates a user from a username and a password, Django's
// UserCreationForm and AdminUserCreationForm.
//
//	form := auth.NewUserCreationForm(db).Bind(r.PostForm)
//	user, err := form.Save(r.Context())
type UserCreationForm struct {
	*forms.ModelForm[User]
}

// NewUserCreationForm returns an unbound UserCreationForm with the username
// and the options of UserCreationFormOptions. opts apply after them, so
// forms.WithFields adds fields after the username.
func NewUserCreationForm(db *orm.DB, opts ...forms.ModelFormOption) *UserCreationForm {
	base := slices.Concat([]forms.ModelFormOption{forms.WithFields("username")}, UserCreationFormOptions(db), opts)
	return &UserCreationForm{forms.NewModelForm[User](db, base...)}
}

// NewAdminUserCreationForm is NewUserCreationForm with the options of
// AdminUserCreationFormOptions.
func NewAdminUserCreationForm(db *orm.DB, opts ...forms.ModelFormOption) *UserCreationForm {
	base := slices.Concat([]forms.ModelFormOption{forms.WithFields("username")}, AdminUserCreationFormOptions(db), opts)
	return &UserCreationForm{forms.NewModelForm[User](db, base...)}
}

// Bind returns a copy of the form bound to data.
func (f *UserCreationForm) Bind(data url.Values) *UserCreationForm {
	return &UserCreationForm{f.ModelForm.Bind(data)}
}

// Save validates the form, sets the password with SetPasswordFromForm and
// saves the user, as Django's UserCreationForm.save does. A new user that
// the form gives no value for is_active and date_joined is active and joins
// now, Django's defaults of the user model.
func (f *UserCreationForm) Save(ctx context.Context) (*User, error) {
	f.SetContext(ctx)
	if f.IsValid() {
		user := f.Instance()
		if user.ID == 0 {
			applyUserDefaults(user, f.Form)
		}
		if err := SetPasswordFromForm(user, f.Form); err != nil {
			return nil, err
		}
	}
	return f.ModelForm.Save(ctx)
}

// applyUserDefaults gives a new user the defaults of Django's user model for
// the fields that form does not have: is_active true and date_joined now.
func applyUserDefaults(user *User, form *forms.Form) {
	if _, ok := form.FieldDef("is_active"); !ok {
		user.IsActive = true
	}
	if _, ok := form.FieldDef("date_joined"); !ok && user.DateJoined.IsZero() {
		user.DateJoined = time.Now().UTC()
	}
}

// UserChangeFormOptions returns the options of a model form of User that
// edits user, Django's UserChangeForm: the username as a UsernameField and
// the password as a ReadOnlyPasswordHashField, whose help text tells how to
// set a password for a user without a usable one. Select the fields with
// forms.WithFields; NewUserChangeForm selects all of them.
func UserChangeFormOptions(user *User) []forms.ModelFormOption {
	help := helpPasswordReadOnly
	if user != nil && !user.HasUsablePassword() {
		help = helpPasswordUnusable
	}
	return []forms.ModelFormOption{
		forms.WithFieldClass("username", NewUsernameField),
		forms.WithFormField("password", NewReadOnlyPasswordHashField(forms.Label("Password"), forms.HelpText(help))),
	}
}

// NewUserChangeForm returns a model form that edits user with every field of
// User and the options of UserChangeFormOptions, as Django's UserChangeForm
// with fields = "__all__". opts apply after them.
func NewUserChangeForm(db *orm.DB, user *User, opts ...forms.ModelFormOption) *forms.ModelForm[User] {
	base := slices.Concat([]forms.ModelFormOption{forms.WithFields(forms.AllFields)}, UserChangeFormOptions(user), opts)
	return forms.NewModelForm[User](db, base...).WithInstance(user)
}

// AdminPasswordChangeForm sets the password of a user in the admin, Django's
// AdminPasswordChangeForm. For a user with a usable password the password
// fields are optional and the field usable_password can remove the password.
type AdminPasswordChangeForm struct {
	*forms.Form
	// User is the user whose password the form sets.
	User *User
}

// NewAdminPasswordChangeForm returns an unbound AdminPasswordChangeForm for
// user.
func NewAdminPasswordChangeForm(user *User) *AdminPasswordChangeForm {
	form := forms.New()
	form.RequiredCSSClass = "required"
	password1, password2 := newPasswordFields()
	password1.Widget = forms.PasswordInput{Attrs: forms.Attrs{"autocomplete": "new-password", "autofocus": true}}
	form.AddField("password1", password1)
	form.AddField("password2", password2)
	if user.HasUsablePassword() {
		password1.Required, password2.Required = false, false
		form.AddField("usable_password", newUsablePasswordField(helpUsablePasswordWarning))
	}
	form.CleanWith(func(form *forms.Form) error {
		cleanOptionalPasswords(form)
		validatePasswordForUser(form, user)
		return nil
	})
	return &AdminPasswordChangeForm{Form: form, User: user}
}

// Bind returns a copy of the form bound to data.
func (f *AdminPasswordChangeForm) Bind(data url.Values) *AdminPasswordChangeForm {
	return &AdminPasswordChangeForm{Form: f.Form.Bind(data), User: f.User}
}

// ChangedData returns ["password"] when both password fields changed, and an
// empty list otherwise, Django's AdminPasswordChangeForm.changed_data.
func (f *AdminPasswordChangeForm) ChangedData() []string {
	data := f.Form.ChangedData()
	if slices.Contains(data, "password1") && slices.Contains(data, "password2") {
		return []string{"password"}
	}
	return []string{}
}

// Save validates the form, sets the password of User with
// SetPasswordFromForm and stores it, as Django's AdminPasswordChangeForm.save
// does. Sessions of the user that were logged in with the old password end;
// UpdateSessionAuthHash keeps the current one. For a form that is not valid
// it returns an error and changes nothing.
func (f *AdminPasswordChangeForm) Save(ctx context.Context, db *orm.DB) (*User, error) {
	if !f.IsValid() {
		return nil, errors.New("auth: the password could not be changed because the data didn't validate")
	}
	if err := SetPasswordFromForm(f.User, f.Form); err != nil {
		return nil, err
	}
	if _, err := orm.Query[User](db).Filter("id", f.User.ID).Update(ctx, "password", f.User.Password); err != nil {
		return nil, fmt.Errorf("auth: store password: %w", err)
	}
	return f.User, nil
}
