package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/mail"
	"github.com/oliverhaas/djangogo/middleware"
	"github.com/oliverhaas/djangogo/orm"
)

// Template names of the password reset email, as in Django.
const (
	PasswordResetEmailTemplate   = "registration/password_reset_email.html"
	PasswordResetSubjectTemplate = "registration/password_reset_subject.txt"
)

// errFormInvalid is the error of saving a form whose data did not validate.
var errFormInvalid = errors.New("auth: the form data didn't validate")

// SetPasswordForm sets a new password for a user without asking for the old
// one (Django's SetPasswordForm). The fields are new_password1 and
// new_password2; the password validators check the second.
type SetPasswordForm struct {
	*forms.Form
	// User is the user whose password the form sets.
	User *User
}

// NewSetPasswordForm returns an unbound SetPasswordForm for user.
func NewSetPasswordForm(user *User) *SetPasswordForm {
	form := forms.New()
	password1, password2 := newPasswordFields("New password", "New password confirmation")
	form.AddField("new_password1", password1)
	form.AddField("new_password2", password2)
	form.CleanWith(func(form *forms.Form) error {
		cleanPasswordPair(form, "new_password1", "new_password2")
		validatePasswordField(form, user, "new_password2")
		return nil
	})
	return &SetPasswordForm{Form: form, User: user}
}

// Bind returns a copy of the form bound to data.
func (f *SetPasswordForm) Bind(data url.Values) *SetPasswordForm {
	return &SetPasswordForm{Form: f.Form.Bind(data), User: f.User}
}

// TemplateValue returns the form for a template, with the user under "user"
// as a TemplateUser, so that {{ form.user }} prints the username as in
// Django.
func (f *SetPasswordForm) TemplateValue() any {
	tv, ok := f.Form.TemplateValue().(forms.TemplateForm)
	if ok && f.User != nil {
		tv["user"] = templateUser(f.User)
	}
	return tv
}

// Save sets new_password1 as the password of User and stores it. Sessions of
// the user that were logged in with the old password end. For a form that is
// not valid it returns an error and changes nothing.
func (f *SetPasswordForm) Save(ctx context.Context, db *orm.DB) (*User, error) {
	if !f.IsValid() {
		return nil, errFormInvalid
	}
	password, _ := f.CleanedData()["new_password1"].(string)
	if err := ChangePassword(ctx, db, f.User, password); err != nil {
		return nil, err
	}
	return f.User, nil
}

// PasswordChangeForm changes the password of a user who enters the old one
// (Django's PasswordChangeForm): SetPasswordForm with the field old_password
// in front.
type PasswordChangeForm struct {
	*SetPasswordForm
}

// NewPasswordChangeForm returns an unbound PasswordChangeForm for user.
func NewPasswordChangeForm(user *User) *PasswordChangeForm {
	set := NewSetPasswordForm(user)
	set.AddField("old_password", forms.NewCharField(
		forms.Label("Old password"),
		forms.Strip(false),
		forms.UseWidget(forms.PasswordInput{Attrs: forms.Attrs{"autocomplete": "current-password", "autofocus": true}}),
	))
	set.OrderFields("old_password", "new_password1", "new_password2")
	set.CleanFieldWith("old_password", func(_ *forms.Form, value any) (any, error) {
		old, _ := value.(string)
		if !user.CheckPassword(old) {
			return nil, forms.NewValidationError(msgPasswordIncorrect, "password_incorrect")
		}
		return value, nil
	})
	return &PasswordChangeForm{SetPasswordForm: set}
}

// Bind returns a copy of the form bound to data.
func (f *PasswordChangeForm) Bind(data url.Values) *PasswordChangeForm {
	return &PasswordChangeForm{SetPasswordForm: f.SetPasswordForm.Bind(data)}
}

// PasswordResetForm asks for an email address and mails a password reset
// link to each active user with that address and a usable password (Django's
// PasswordResetForm). The form is valid for any email address, so that it
// does not tell which addresses belong to users.
type PasswordResetForm struct {
	*forms.Form
}

// NewPasswordResetForm returns an unbound PasswordResetForm.
func NewPasswordResetForm() *PasswordResetForm {
	form := forms.New()
	form.AddField("email", forms.NewEmailField(
		forms.Label("Email"),
		forms.MaxLength(254),
		forms.UseWidget(forms.EmailInput{Attrs: forms.Attrs{"autocomplete": "email"}}),
	))
	return &PasswordResetForm{Form: form}
}

// Bind returns a copy of the form bound to data.
func (f *PasswordResetForm) Bind(data url.Values) *PasswordResetForm {
	return &PasswordResetForm{Form: f.Form.Bind(data)}
}

// EmailRenderer renders a named template without a request, as Django's
// render_to_string does for emails. *templates.Engine implements it.
type EmailRenderer interface {
	Render(name string, data map[string]any) (string, error)
}

// PasswordResetOptions are the arguments of PasswordResetForm.Save (Django's
// keyword arguments of PasswordResetForm.save).
type PasswordResetOptions struct {
	// Request is the request of the form. Its host is the domain and the
	// site name in the email, unless DomainOverride is set. The host is the
	// one that middleware.AllowedHosts validated, and the Host header for a
	// request that did not pass that middleware.
	Request *http.Request
	// DomainOverride is the domain and the site name in the email.
	DomainOverride string
	// UseHTTPS makes the link in the email an https link.
	UseHTTPS bool
	// SubjectTemplateName renders the subject. Empty means
	// PasswordResetSubjectTemplate.
	SubjectTemplateName string
	// EmailTemplateName renders the body. Empty means
	// PasswordResetEmailTemplate.
	EmailTemplateName string
	// HTMLEmailTemplateName, when set, renders an HTML alternative of the
	// body.
	HTMLEmailTemplateName string
	// ExtraEmailContext is added to the context of the email templates. Its
	// keys win over those of Save.
	ExtraEmailContext map[string]any
	// FromEmail is the sender. Empty means the DefaultFromEmail setting of
	// the mail package.
	FromEmail string
	// TokenGenerator makes the token of the link. Nil means
	// DefaultTokenGenerator.
	TokenGenerator *PasswordResetTokenGenerator
	// Templates renders the email templates. Nil means the embedded default
	// templates.
	Templates EmailRenderer
	// SendMail sends one email in place of the default (Django's
	// PasswordResetForm.send_mail). data is the context of the email
	// templates and to the address of the user. An error it returns ends
	// Save with that error.
	SendMail func(ctx context.Context, opts PasswordResetOptions, data map[string]any, to string) error
}

// Save mails a password reset link to each user that PasswordResetUsers finds
// for the email address of the form. The context of the email templates has
// "email", "domain", "site_name", "uid", "user" (a TemplateUser), "token" and
// "protocol", plus ExtraEmailContext.
//
// The default sender renders the subject, the body and the optional HTML body
// and sends them with the mail package. A failure to render ends Save with
// the error. A failure to send is logged and the next user gets an email, so
// that the result does not tell whether a user has the address. For a form
// that is not valid Save returns an error and sends nothing.
func (f *PasswordResetForm) Save(ctx context.Context, db *orm.DB, opts PasswordResetOptions) error {
	if !f.IsValid() {
		return errFormInvalid
	}
	email, _ := f.CleanedData()["email"].(string)
	domain := opts.DomainOverride
	if domain == "" {
		if opts.Request == nil {
			return errors.New("auth: PasswordResetForm.Save needs a Request or a DomainOverride")
		}
		domain = requestHost(opts.Request)
	}
	generator := opts.TokenGenerator
	if generator == nil {
		generator = DefaultTokenGenerator
	}
	protocol := "http"
	if opts.UseHTTPS {
		protocol = "https"
	}
	users, err := PasswordResetUsers(ctx, db, email)
	if err != nil {
		return err
	}
	send := opts.SendMail
	if send == nil {
		send = sendPasswordResetMail
	}
	for _, u := range users {
		data := map[string]any{
			"email":     u.Email,
			"domain":    domain,
			"site_name": domain,
			"uid":       URLSafeBase64Encode([]byte(strconv.FormatInt(u.ID, 10))),
			"user":      templateUser(u),
			"token":     generator.MakeToken(u),
			"protocol":  protocol,
		}
		for k, v := range opts.ExtraEmailContext {
			data[k] = v
		}
		if err := send(ctx, opts, data, u.Email); err != nil {
			return err
		}
	}
	return nil
}

// sendPasswordResetMail renders the email templates of opts with data and
// sends the email to to (Django's PasswordResetForm.send_mail). The subject
// loses its line breaks. A failure to send is logged, not returned.
func sendPasswordResetMail(ctx context.Context, opts PasswordResetOptions, data map[string]any, to string) error {
	renderer := opts.Templates
	if renderer == nil {
		engine, err := defaultEngine()
		if err != nil {
			return err
		}
		renderer = engine
	}
	subjectName := templateOr(opts.SubjectTemplateName, PasswordResetSubjectTemplate)
	subject, err := renderer.Render(subjectName, data)
	if err != nil {
		return fmt.Errorf("auth: render %s: %w", subjectName, err)
	}
	bodyName := templateOr(opts.EmailTemplateName, PasswordResetEmailTemplate)
	body, err := renderer.Render(bodyName, data)
	if err != nil {
		return fmt.Errorf("auth: render %s: %w", bodyName, err)
	}
	msg := &mail.Message{Subject: withoutLineBreaks(subject), Body: body, From: opts.FromEmail, To: []string{to}}
	if opts.HTMLEmailTemplateName != "" {
		html, err := renderer.Render(opts.HTMLEmailTemplateName, data)
		if err != nil {
			return fmt.Errorf("auth: render %s: %w", opts.HTMLEmailTemplateName, err)
		}
		if err := msg.AttachAlternative(html, "text/html"); err != nil {
			return err
		}
	}
	if _, err := msg.Send(); err != nil {
		user, _ := data["user"].(TemplateUser)
		slog.ErrorContext(ctx, fmt.Sprintf("Failed to send password reset email to %v", user["pk"]), "error", err)
	}
	return nil
}

// withoutLineBreaks removes the characters at which Python's str.splitlines
// splits, as Django's "".join(subject.splitlines()) does: a header must not
// hold a line break.
func withoutLineBreaks(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', ' ', ' ':
			return -1
		}
		return r
	}, s)
}

// PasswordResetUsers returns the users that get a password reset email for
// email (Django's PasswordResetForm.get_users): the active users with a
// usable password whose address equals email under Unicode normalization and
// case folding.
func PasswordResetUsers(ctx context.Context, db *orm.DB, email string) ([]*User, error) {
	candidates, err := orm.Query[User](db).Filter("email__iexact", email).Filter("is_active", true).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: find users by email: %w", err)
	}
	var users []*User
	for i := range candidates {
		u := &candidates[i]
		if u.HasUsablePassword() && unicodeCIEqual(email, u.Email) {
			users = append(users, u)
		}
	}
	return users, nil
}

// unicodeCIEqual reports whether a and b are equal after NFKC normalization
// and case folding (Django's _unicode_ci_compare). The database can match
// addresses such as "mıke@example.org" and "mike@example.org" in a
// case-insensitive lookup, which must not send the link of one user to the
// address of the other.
func unicodeCIEqual(a, b string) bool {
	fold := cases.Fold()
	return fold.String(norm.NFKC.String(a)) == fold.String(norm.NFKC.String(b))
}

// requestHost returns the host that the AllowedHosts middleware validated for
// r, and the Host header otherwise.
func requestHost(r *http.Request) string {
	if host, ok := middleware.ValidatedHost(r); ok {
		return host
	}
	return r.Host
}
