package auth_test

// Ports of tests/auth_tests/test_forms.py: the user creation and change
// forms, AdminPasswordChangeForm and the read-only password hash field.
//
// FIXME-PORT: AuthenticationFormTest, SetPasswordFormTest,
// PasswordChangeFormTest and PasswordResetFormTest. The auth views validate
// these forms without a form type; django_views_test.go covers them through
// the views.
//
// No Go counterpart: SensitiveVariablesTest, about the sensitive_variables
// decorator of Django's debug pages.

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Messages of the user forms, in Django's wording.
const (
	msgUsernameTaken    = "A user with that username already exists."
	msgInvalidUsername  = "Enter a valid username. This value may contain only letters, numbers, and @/./+/-/_ characters."
	msgPasswordMismatch = "The two password fields didn’t match."
	msgFieldRequired    = "This field is required."
)

// setUpFormsTestData creates the users of TestDataMixin in Django's
// test_forms.py.
func setUpFormsTestData(t *testing.T, db *orm.DB) {
	t.Helper()
	ctx := context.Background()
	createUser(t, db, "testclient", "password")
	_, err := auth.CreateUser(ctx, db, "inactive", "", "password", false, false, false)
	must(t, err)
	createUser(t, db, "staff", "password")
	unusable, err := auth.MakeUnusablePassword()
	must(t, err)
	for username, password := range map[string]string{
		"empty_password":        "",
		"unmanageable_password": "$",
		"unknown_password":      "foo$bar",
		"unusable_password":     unusable,
	} {
		must(t, orm.Query[auth.User](db).Create(ctx, &auth.User{Username: username, Password: password}))
	}
}

// formsTestDB runs fn on every dialect with the users of
// setUpFormsTestData, cheap hashers and no password validators, Django's
// test settings.
func formsTestDB(t *testing.T, fn func(t *testing.T, db *orm.DB)) {
	t.Helper()
	useHashers(t, cheapDefaults()...)
	usePasswordValidators(t)
	eachDB(t, func(t *testing.T, db *orm.DB) {
		setUpFormsTestData(t, db)
		fn(t, db)
	})
}

// userNamed loads the user with the given username.
func userNamed(t *testing.T, db *orm.DB, username string) *auth.User {
	t.Helper()
	u, err := orm.Query[auth.User](db).Get(context.Background(), "username", username)
	if err != nil {
		t.Fatalf("load user %q: %v", username, err)
	}
	return &u
}

// newCreationForm builds a user creation form, as auth.NewUserCreationForm.
type newCreationForm func(db *orm.DB, opts ...forms.ModelFormOption) *auth.UserCreationForm

// eachCreationForm runs fn for the forms that inherit Django's
// BaseUserCreationFormTest: UserCreationFormTest and
// AdminUserCreationFormTest. Each form gets a database of its own.
//
// No Go counterpart: BaseUserCreationForm, the base class without the check
// of the username case. The tests of the base class run for its subclasses.
func eachCreationForm(t *testing.T, fn func(t *testing.T, db *orm.DB, newForm newCreationForm)) {
	t.Helper()
	for _, variant := range []struct {
		name    string
		newForm newCreationForm
	}{
		{"UserCreationForm", auth.NewUserCreationForm},
		{"AdminUserCreationForm", auth.NewAdminUserCreationForm},
	} {
		t.Run(variant.name, func(t *testing.T) {
			formsTestDB(t, func(t *testing.T, db *orm.DB) { fn(t, db, variant.newForm) })
		})
	}
}

// formErrors returns the error messages of a form field.
func formErrors(f *forms.Form, field string) []string {
	return f.Errors().Get(field).Messages()
}

// widgetAttrs returns the attributes of the widget of a form field with
// those the field adds, Django's widget.attrs.
func widgetAttrs(t *testing.T, f *forms.Form, name string) forms.Attrs {
	t.Helper()
	field, ok := f.FieldDef(name)
	if !ok {
		t.Fatalf("form has no field %q", name)
	}
	w := field.Base().Widget
	attrs := forms.Attrs{}
	for k, v := range w.Attributes() {
		attrs[k] = v
	}
	for k, v := range field.WidgetAttrs(w) {
		attrs[k] = v
	}
	return attrs
}

// values returns form data from pairs of names and values.
func values(pairs ...string) url.Values {
	data := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		data.Set(pairs[i], pairs[i+1])
	}
	return data
}

// failingField returns a clean function that fails with "Extra validation
// for <field>." when failing lists the field, the ExtraValidationFormMixin of
// Django's tests.
func failingField(field string, failing []string) func(*forms.Form, any) (any, error) {
	return func(_ *forms.Form, value any) (any, error) {
		if slices.Contains(failing, field) {
			return nil, forms.ValidationErrors(forms.NewValidationError("Extra validation for "+field+".", "invalid"))
		}
		return value, nil
	}
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_form_fields
func TestUserCreationFormFields(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewUserCreationForm(db)
		djangotest.AssertEqual(t, form.FieldNames(), []string{"username", "password1", "password2"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_user_already_exists
func TestUserCreationFormUserAlreadyExists(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		form := newForm(db).Bind(values("username", "testclient", "password1", "test123", "password2", "test123"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "username"), []string{msgUsernameTaken})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_invalid_data
func TestUserCreationFormInvalidData(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		form := newForm(db).Bind(values("username", "jsmith!", "password1", "test123", "password2", "test123"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "username"), []string{msgInvalidUsername})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_password_verification
func TestUserCreationFormPasswordVerification(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		form := newForm(db).Bind(values("username", "jsmith", "password1", "test123", "password2", "test"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "password2"), []string{msgPasswordMismatch})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_both_passwords
func TestUserCreationFormBothPasswords(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		data := values("username", "jsmith")
		form := newForm(db).Bind(data)
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "password1"), []string{msgFieldRequired})
		djangotest.AssertEqual(t, formErrors(form.Form, "password2"), []string{msgFieldRequired})

		data.Set("password2", "test123")
		form = newForm(db).Bind(data)
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "password1"), []string{msgFieldRequired})
		djangotest.AssertEqual(t, len(formErrors(form.Form, "password2")), 0)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_success
//
// No Go counterpart: password_changed, the hook of the password validators.
// The test checks the stored password instead.
func TestUserCreationFormSuccess(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		form := newForm(db).Bind(values("username", "jsmith@example.com", "password1", "test123", "password2", "test123"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		u, err := form.Save(context.Background())
		must(t, err)
		djangotest.AssertEqual(t, u.String(), "jsmith@example.com")
		djangotest.AssertEqual(t, reload(t, db, u.ID).CheckPassword("test123"), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_unicode_username
func TestUserCreationFormUnicodeUsername(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		form := newForm(db).Bind(values("username", "宝", "password1", "test123", "password2", "test123"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		u, err := form.Save(context.Background())
		must(t, err)
		djangotest.AssertEqual(t, u.Username, "宝")
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_normalize_username
func TestUserCreationFormNormalizeUsername(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		ohmUsername := "test\u2126" // OHM SIGN
		form := newForm(db).Bind(values("username", ohmUsername, "password1", "pwd2", "password2", "pwd2"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		u, err := form.Save(context.Background())
		must(t, err)
		if u.Username == ohmUsername {
			t.Errorf("username %q was not normalized", u.Username)
		}
		djangotest.AssertEqual(t, u.Username, "test\u03a9") // GREEK CAPITAL LETTER OMEGA
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_invalid_username_no_normalize
func TestUsernameFieldInvalidUsernameNoNormalize(t *testing.T) {
	field := auth.NewUsernameField(forms.MaxLength(254))
	// Usernames are not normalized if they are too long.
	got, err := field.ToGo(strings.Repeat("½", 255))
	must(t, err)
	djangotest.AssertEqual(t, got, any(strings.Repeat("½", 255)))
	got, err = field.ToGo(strings.Repeat("ﬀ", 254))
	must(t, err)
	djangotest.AssertEqual(t, got, any(strings.Repeat("ff", 254)))
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_duplicate_normalized_unicode
func TestUserCreationFormDuplicateNormalizedUnicode(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		omegaUsername := "iamthe\u03a9" // GREEK CAPITAL LETTER OMEGA
		ohmUsername := "iamthe\u2126"   // OHM SIGN
		createUser(t, db, omegaUsername, "pwd")
		form := newForm(db).Bind(values("username", ohmUsername, "password1", "pwd2", "password2", "pwd2"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "username"), []string{msgUsernameTaken})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_validates_password
func TestUserCreationFormValidatesPassword(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		usePasswordValidators(t, auth.UserAttributeSimilarityValidator{}, auth.MinimumLengthValidator{MinLength: 12})
		form := newForm(db).Bind(values("username", "otherclient", "password1", "otherclient", "password2", "otherclient"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		errs := formErrors(form.Form, "password2")
		djangotest.AssertEqual(t, len(errs), 2)
		djangotest.AssertEqual(t, slices.Contains(errs, "The password is too similar to the username."), true)
		djangotest.AssertEqual(t, slices.Contains(errs,
			"This password is too short. It must contain at least 12 characters."), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_password_whitespace_not_stripped
func TestUserCreationFormPasswordWhitespaceNotStripped(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		data := values("username", "testuser", "password1", "   testpassword   ", "password2", "   testpassword   ")
		form := newForm(db).Bind(data)
		djangotest.AssertEqual(t, form.IsValid(), true)
		djangotest.AssertEqual(t, form.CleanedData()["password1"], any(data.Get("password1")))
		djangotest.AssertEqual(t, form.CleanedData()["password2"], any(data.Get("password2")))
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_password_help_text
func TestUserCreationFormPasswordHelpText(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		usePasswordValidators(t, auth.UserAttributeSimilarityValidator{})
		field, _ := newForm(db).FieldDef("password1")
		djangotest.AssertEqual(t, field.Base().HelpText,
			"<ul><li>Your password can’t be too similar to your other personal information.</li></ul>")
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_password_extra_validations
func TestUserCreationFormPasswordExtraValidations(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		data := values("username", "extra", "password1", "abc", "password2", "abc")
		for _, failing := range [][]string{{"password1"}, {"password2"}, {"password1", "password2"}} {
			t.Run(strings.Join(failing, ","), func(t *testing.T) {
				want := map[string][]string{}
				for _, field := range failing {
					want[field] = []string{"Extra validation for " + field + "."}
				}
				form := newForm(db,
					forms.WithCleanField("password1", failingField("password1", failing)),
					forms.WithCleanField("password2", failingField("password2", failing)),
				).Bind(data)
				djangotest.AssertEqual(t, form.IsValid(), false)
				djangotest.AssertEqual(t, form.Errors().Messages(), want)
			})
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_user_create_form_validates_password_with_all_data
func TestUserCreationFormValidatesPasswordWithAllData(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		usePasswordValidators(t, auth.UserAttributeSimilarityValidator{})
		form := newForm(db, forms.WithFields("email", "first_name", "last_name")).Bind(values(
			"username", "testuser",
			"password1", "testpassword",
			"password2", "testpassword",
			"first_name", "testpassword",
			"last_name", "lastname",
		))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "password2"),
			[]string{"The password is too similar to the first name."})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_username_field_autocapitalize_none
func TestUserCreationFormUsernameFieldAutocapitalizeNone(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		djangotest.AssertEqual(t, widgetAttrs(t, newForm(db).Form, "username")["autocapitalize"], any("none"))
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::BaseUserCreationFormTest.test_html_autocomplete_attributes
func TestUserCreationFormHTMLAutocompleteAttributes(t *testing.T) {
	eachCreationForm(t, func(t *testing.T, db *orm.DB, newForm newCreationForm) {
		form := newForm(db)
		for field, autocomplete := range map[string]string{
			"username":  "username",
			"password1": "new-password",
			"password2": "new-password",
		} {
			djangotest.AssertEqual(t, widgetAttrs(t, form.Form, field)["autocomplete"], any(autocomplete))
		}
	})
}

// No Go counterpart: BaseUserCreationFormTest.test_user_creation_form_class_getitem,
// about __class_getitem__.

// FIXME-PORT: CustomUserCreationFormTest. The forms work with the User model
// only; there is no custom user model.

// Ported from Django: tests/auth_tests/test_forms.py::UserCreationFormTest.test_case_insensitive_username
func TestUserCreationFormCaseInsensitiveUsername(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewUserCreationForm(db).Bind(values("username", "TeStClIeNt", "password1", "test123", "password2", "test123"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "username"), []string{msgUsernameTaken})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::UserCreationFormTest.test_case_insensitive_username_custom_user_and_error_message
//
// FIXME-PORT: ExtensionUser and its date_of_birth field. The test checks the
// custom message with the User model.
func TestUserCreationFormCaseInsensitiveUsernameErrorMessage(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewUserCreationForm(db,
			forms.WithErrorMessages("username", map[string]string{"unique": "This username has already been taken."}),
		).Bind(values("username", "TeStClIeNt", "password1", "test123", "password2", "test123"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "username"), []string{"This username has already been taken."})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_username_validity
func TestUserChangeFormUsernameValidity(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewUserChangeForm(db, user).Bind(values("username", "not valid"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, formErrors(form.Form, "username"), []string{msgInvalidUsername})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_bug_14242
func TestUserChangeFormBug14242(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		opts := append(auth.UserChangeFormOptions(nil),
			forms.WithFields("groups"),
			forms.WithHelpText("groups", "These groups give users different permissions"))
		// Just check we can create it.
		form := forms.NewModelForm[auth.User](db, opts...).Bind(url.Values{})
		must(t, form.Err())
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_unusable_password
// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_bug_17944_empty_password
// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_bug_17944_unmanageable_password
// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_bug_17944_unknown_password_algorithm
func TestUserChangeFormPasswordSummary(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		for username, want := range map[string]string{
			"unusable_password":     "No password set.",
			"empty_password":        "No password set.",
			"unmanageable_password": "Invalid password format or unknown hashing algorithm.",
			"unknown_password":      "Invalid password format or unknown hashing algorithm.",
		} {
			t.Run(username, func(t *testing.T) {
				form := auth.NewUserChangeForm(db, userNamed(t, db, username))
				if html := form.AsTable(); !strings.Contains(html, want) {
					t.Errorf("form does not contain %q:\n%s", want, html)
				}
			})
		}
	})
}

// initialAsData returns the initial values of a form as the form data a
// browser posts for them, the dictionary that Django's test_bug_19133 posts.
func initialAsData(initial map[string]any) url.Values {
	data := url.Values{}
	for name, value := range initial {
		switch v := value.(type) {
		case nil:
		case bool:
			if v {
				data.Set(name, "on")
			}
		case time.Time:
			data.Set(name, v.Format(time.DateTime))
		case *time.Time:
			if v != nil {
				data.Set(name, v.Format(time.DateTime))
			}
		case []any:
			for _, item := range v {
				data.Add(name, fmt.Sprint(item))
			}
		default:
			data.Set(name, fmt.Sprint(v))
		}
	}
	return data
}

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_bug_19133
func TestUserChangeFormBug19133(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		// The change form does not return the password value.
		user := userNamed(t, db, "testclient")
		postData := initialAsData(auth.NewUserChangeForm(db, user).Initial)
		// The password field is read-only, so anything posted here is ignored;
		// the form is valid and gives back the initial value for the password.
		postData.Set("password", "new password")
		form := auth.NewUserChangeForm(db, user).Bind(postData)
		djangotest.AssertEqual(t, form.IsValid(), true)
		password, _ := form.CleanedData()["password"].(string)
		djangotest.AssertEqual(t, strings.Contains(password, "$"), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_bug_19349_bound_password_field
func TestUserChangeFormBug19349BoundPasswordField(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewUserChangeForm(db, user).Bind(url.Values{})
		// When it renders the bound password field, ReadOnlyPasswordHashWidget
		// needs the initial value.
		djangotest.AssertEqual(t, form.Field("password").Value(), form.Initial["password"])
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_link_to_password_reset_in_user_change_form
//
// The admin tests check that the link leads to the password page of the
// user admin.
func TestUserChangeFormLinkToPasswordReset(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		for _, tc := range []struct{ username, helpText, buttonLabel string }{
			{"testclient", "Raw passwords are not stored, so there is no way to see the user’s password.", "Reset password"},
			{"unusable_password", "Enable password-based authentication for this user by setting a password.", "Set password"},
		} {
			t.Run(tc.username, func(t *testing.T) {
				form := auth.NewUserChangeForm(db, userNamed(t, db, tc.username)).Bind(url.Values{})
				field, _ := form.FieldDef("password")
				djangotest.AssertEqual(t, field.Base().HelpText, tc.helpText)
				link := `<a role="button" class="button" href="../password/">` + tc.buttonLabel + `</a>`
				if html := form.AsP(); !strings.Contains(html, link) {
					t.Errorf("form does not contain %q:\n%s", link, html)
				}
			})
		}
	})
}

// FIXME-PORT: UserChangeFormTest.test_custom_form, with ExtensionUser. There
// is no custom user model.

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_password_excluded
//
// No Go counterpart: password = None, which removes a declared field in a
// subclass. RemoveField removes it from the built form.
func TestUserChangeFormPasswordExcluded(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		opts := append(auth.UserChangeFormOptions(nil), forms.WithExclude("password"))
		form := forms.NewModelForm[auth.User](db, opts...)
		form.RemoveField("password")
		djangotest.AssertEqual(t, slices.Contains(form.FieldNames(), "password"), false)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::UserChangeFormTest.test_username_field_autocapitalize_none
func TestUserChangeFormUsernameFieldAutocapitalizeNone(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewUserChangeForm(db, nil)
		djangotest.AssertEqual(t, widgetAttrs(t, form.Form, "username")["autocapitalize"], any("none"))
	})
}

// pbkdf2Hash is the encoded password of Django's ReadOnlyPasswordHashTest.
const pbkdf2Hash = "pbkdf2_sha256$100000$a6Pucb1qSFcD$WmCkn9Hqidj48NVe5x0FEM6A9YiOqQcl/83m2Z5udm0="

// Ported from Django: tests/auth_tests/test_forms.py::ReadOnlyPasswordHashTest.test_bug_19349_render_with_none_value
func TestReadOnlyPasswordHashBug19349RenderWithNoneValue(t *testing.T) {
	// Rendering the widget with a nil value must not fail.
	html := auth.ReadOnlyPasswordHashWidget{}.Render("password", nil, forms.Attrs{})
	djangotest.AssertEqual(t, strings.Contains(html, "No password set."), true)
}

// Ported from Django: tests/auth_tests/test_forms.py::ReadOnlyPasswordHashTest.test_render
func TestReadOnlyPasswordHashRender(t *testing.T) {
	useHashers(t, auth.PBKDF2Hasher{})
	djangotest.AssertHTMLEqual(t,
		auth.ReadOnlyPasswordHashWidget{}.Render("name", pbkdf2Hash, forms.Attrs{"id": "id_password"}),
		`<div id="id_password">`+
			"  <p>"+
			"    <strong>algorithm</strong>: <bdi>pbkdf2_sha256</bdi>"+
			"    <strong>iterations</strong>: <bdi>100000</bdi>"+
			"    <strong>salt</strong>: <bdi>a6Pucb******</bdi>"+
			"    <strong>hash</strong>: "+
			"       <bdi>WmCkn9**************************************</bdi>"+
			"  </p>"+
			`  <p><a role="button" class="button" href="../password/">`+
			"Reset password</a></p>"+
			"</div>")
}

// Ported from Django: tests/auth_tests/test_forms.py::ReadOnlyPasswordHashTest.test_render_no_password
func TestReadOnlyPasswordHashRenderNoPassword(t *testing.T) {
	djangotest.AssertHTMLEqual(t,
		auth.ReadOnlyPasswordHashWidget{}.Render("name", nil, forms.Attrs{}),
		"<div><p><strong>No password set.</p><p>"+
			`<a role="button" class="button" href="../password/">Set password</a>`+
			"</p></div>")
}

// Ported from Django: tests/auth_tests/test_forms.py::ReadOnlyPasswordHashTest.test_render_invalid_password_format
func TestReadOnlyPasswordHashRenderInvalidPasswordFormat(t *testing.T) {
	useHashers(t, auth.PBKDF2Hasher{})
	djangotest.AssertHTMLEqual(t,
		auth.ReadOnlyPasswordHashWidget{}.Render("name", "pbkdf2_sh", forms.Attrs{}),
		"<div><p>"+
			"<strong>Invalid password format or unknown hashing algorithm.</strong>"+
			`</p><p><a role="button" class="button" href="../password/">Reset password`+
			"</a></p></div>")
}

// Ported from Django: tests/auth_tests/test_forms.py::ReadOnlyPasswordHashTest.test_readonly_field_has_changed
func TestReadOnlyPasswordHashFieldHasChanged(t *testing.T) {
	field := auth.NewReadOnlyPasswordHashField()
	djangotest.AssertEqual(t, field.Disabled, true)
	djangotest.AssertEqual(t, field.HasChanged("aaa", "bbb"), false)
}

// Ported from Django: tests/auth_tests/test_forms.py::ReadOnlyPasswordHashTest.test_label
func TestReadOnlyPasswordHashLabel(t *testing.T) {
	// The widget gives the <label> no for attribute: it has no element a
	// label can point at.
	form := forms.New()
	form.AddField("hash_field", auth.NewReadOnlyPasswordHashField())
	boundField := form.Field("hash_field")
	djangotest.AssertEqual(t, boundField.Field().Base().Widget.IDForLabel("id"), "")
	djangotest.AssertEqual(t, boundField.LabelTag(), "<label>Hash field:</label>")
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_success
//
// No Go counterpart: password_changed and save(commit=False). The test
// checks the stored password instead.
func TestAdminPasswordChangeFormSuccess(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewAdminPasswordChangeForm(user).Bind(values("password1", "test123", "password2", "test123"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		_, err := form.Save(context.Background(), db)
		must(t, err)
		djangotest.AssertEqual(t, reload(t, db, user.ID).CheckPassword("test123"), true)
		djangotest.AssertEqual(t, form.ChangedData(), []string{"password"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_validates_password
func TestAdminPasswordChangeFormValidatesPassword(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		usePasswordValidators(t, auth.UserAttributeSimilarityValidator{}, auth.MinimumLengthValidator{MinLength: 12})
		user := userNamed(t, db, "testclient")
		form := auth.NewAdminPasswordChangeForm(user).Bind(values("password1", "testclient", "password2", "testclient"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		errs := formErrors(form.Form, "password2")
		djangotest.AssertEqual(t, len(errs), 2)
		djangotest.AssertEqual(t, slices.Contains(errs, "The password is too similar to the username."), true)
		djangotest.AssertEqual(t, slices.Contains(errs,
			"This password is too short. It must contain at least 12 characters."), true)

		// The passwords are not validated when usable_password is unset.
		form = auth.NewAdminPasswordChangeForm(user).Bind(values(
			"password1", "testclient", "password2", "testclient", "usable_password", "false"))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_password_whitespace_not_stripped
func TestAdminPasswordChangeFormPasswordWhitespaceNotStripped(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		data := values("password1", " pass ", "password2", " pass ")
		form := auth.NewAdminPasswordChangeForm(user).Bind(data)
		djangotest.AssertEqual(t, form.IsValid(), true)
		djangotest.AssertEqual(t, form.CleanedData()["password1"], any(data.Get("password1")))
		djangotest.AssertEqual(t, form.CleanedData()["password2"], any(data.Get("password2")))
		djangotest.AssertEqual(t, form.ChangedData(), []string{"password"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_password_extra_validations
func TestAdminPasswordChangeFormPasswordExtraValidations(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		data := values("username", "extra", "password1", "abc", "password2", "abc")
		for _, failing := range [][]string{{"password1"}, {"password2"}, {"password1", "password2"}} {
			t.Run(strings.Join(failing, ","), func(t *testing.T) {
				want := map[string][]string{}
				for _, field := range failing {
					want[field] = []string{"Extra validation for " + field + "."}
				}
				form := auth.NewAdminPasswordChangeForm(user)
				form.CleanFieldWith("password1", failingField("password1", failing))
				form.CleanFieldWith("password2", failingField("password2", failing))
				bound := form.Bind(data)
				djangotest.AssertEqual(t, bound.IsValid(), false)
				djangotest.AssertEqual(t, bound.Errors().Messages(), want)
			})
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_non_matching_passwords
func TestAdminPasswordChangeFormNonMatchingPasswords(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewAdminPasswordChangeForm(user).Bind(values("password1", "password1", "password2", "password2"))
		djangotest.AssertEqual(t, formErrors(form.Form, "password2"), []string{msgPasswordMismatch})
		djangotest.AssertEqual(t, form.ChangedData(), []string{"password"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_missing_passwords
func TestAdminPasswordChangeFormMissingPasswords(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewAdminPasswordChangeForm(user).Bind(values("password1", "", "password2", ""))
		djangotest.AssertEqual(t, formErrors(form.Form, "password1"), []string{msgFieldRequired})
		djangotest.AssertEqual(t, formErrors(form.Form, "password2"), []string{msgFieldRequired})
		djangotest.AssertEqual(t, form.ChangedData(), []string{})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_one_password
func TestAdminPasswordChangeFormOnePassword(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form1 := auth.NewAdminPasswordChangeForm(user).Bind(values("password1", "", "password2", "test"))
		djangotest.AssertEqual(t, formErrors(form1.Form, "password1"), []string{msgFieldRequired})
		djangotest.AssertEqual(t, form1.Errors().Has("password2"), false)
		djangotest.AssertEqual(t, form1.ChangedData(), []string{})
		form2 := auth.NewAdminPasswordChangeForm(user).Bind(values("password1", "test", "password2", ""))
		djangotest.AssertEqual(t, formErrors(form2.Form, "password2"), []string{msgFieldRequired})
		djangotest.AssertEqual(t, form2.Errors().Has("password1"), false)
		djangotest.AssertEqual(t, form2.ChangedData(), []string{})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_html_autocomplete_attributes
func TestAdminPasswordChangeFormHTMLAutocompleteAttributes(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewAdminPasswordChangeForm(userNamed(t, db, "testclient"))
		for _, field := range []string{"password1", "password2"} {
			djangotest.AssertEqual(t, widgetAttrs(t, form.Form, field)["autocomplete"], any("new-password"))
		}
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_enable_password_authentication
func TestAdminPasswordChangeFormEnablePasswordAuthentication(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "unusable_password")
		form := auth.NewAdminPasswordChangeForm(user).Bind(values(
			"password1", "complexpassword", "password2", "complexpassword"))
		djangotest.AssertEqual(t, slices.Contains(form.FieldNames(), "usable_password"), false)
		djangotest.AssertEqual(t, form.IsValid(), true)
		user, err := form.Save(context.Background(), db)
		must(t, err)
		djangotest.AssertEqual(t, user.HasUsablePassword(), true)
		djangotest.AssertEqual(t, reload(t, db, user.ID).HasUsablePassword(), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminPasswordChangeFormTest.test_disable_password_authentication
func TestAdminPasswordChangeFormDisablePasswordAuthentication(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		user := userNamed(t, db, "testclient")
		form := auth.NewAdminPasswordChangeForm(user).Bind(values(
			"usable_password", "false", "password1", "", "password2", "test"))
		djangotest.AssertEqual(t, slices.Contains(form.FieldNames(), "usable_password"), true)
		field, _ := form.FieldDef("usable_password")
		djangotest.AssertEqual(t, strings.Contains(field.Base().HelpText,
			"If disabled, the current password for this user will be lost."), true)
		// The form is valid despite the empty password and the mismatch.
		djangotest.AssertEqual(t, form.IsValid(), true)
		user, err := form.Save(context.Background(), db)
		must(t, err)
		djangotest.AssertEqual(t, user.HasUsablePassword(), false)
		djangotest.AssertEqual(t, reload(t, db, user.ID).HasUsablePassword(), false)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminUserCreationFormTest.test_form_fields
func TestAdminUserCreationFormFields(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewAdminUserCreationForm(db)
		djangotest.AssertEqual(t, form.FieldNames(), []string{"username", "password1", "password2", "usable_password"})
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminUserCreationFormTest.test_no_password_validation_if_unusable_password_set
func TestAdminUserCreationFormNoPasswordValidationIfUnusablePasswordSet(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		usePasswordValidators(t, auth.UserAttributeSimilarityValidator{}, auth.MinimumLengthValidator{MinLength: 12})
		form := auth.NewAdminUserCreationForm(db).Bind(values(
			"username", "otherclient",
			"password1", "otherclient",
			"password2", "otherclient",
			"usable_password", "false",
		))
		// The passwords are not validated when usable_password is unset.
		djangotest.AssertEqual(t, form.IsValid(), true)

		form = auth.NewAdminUserCreationForm(db, forms.WithFields("email", "first_name", "last_name")).Bind(values(
			"username", "testuser",
			"password1", "testpassword",
			"password2", "testpassword",
			"first_name", "testpassword",
			"last_name", "lastname",
			"usable_password", "false",
		))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/auth_tests/test_forms.py::AdminUserCreationFormTest.test_unusable_password
func TestAdminUserCreationFormUnusablePassword(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewAdminUserCreationForm(db).Bind(values(
			"username", "new-user-which-does-not-exist", "usable_password", "false"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		u, err := form.Save(context.Background())
		must(t, err)
		djangotest.AssertEqual(t, u.Username, "new-user-which-does-not-exist")
		djangotest.AssertEqual(t, u.HasUsablePassword(), false)
	})
}

// No Django test exists: the user that UserCreationForm.Save creates is
// active and has a join date, the defaults of Django's user model.
func TestUserCreationFormSaveDefaults(t *testing.T) {
	formsTestDB(t, func(t *testing.T, db *orm.DB) {
		form := auth.NewUserCreationForm(db).Bind(values("username", "jsmith", "password1", "test123", "password2", "test123"))
		u, err := form.Save(context.Background())
		must(t, err)
		stored := reload(t, db, u.ID)
		djangotest.AssertEqual(t, stored.IsActive, true)
		djangotest.AssertEqual(t, stored.DateJoined.IsZero(), false)
	})
}
