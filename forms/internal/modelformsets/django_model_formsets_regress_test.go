package modelformsets_test

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_formset_over_to_field
func TestFormsetOverToField(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a relation points at the primary key of its target, there is no to_field; " +
		"Django: UserSite.user has to_field=\"username\"")
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_formset_over_inherited_model
func TestFormsetOverInheritedModel(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance; Django: Restaurant inherits from Place")
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_inline_model_with_to_field
func TestInlineModelWithToField(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a relation points at the primary key of its target, there is no to_field; " +
		"Django: UserSite.user has to_field=\"username\"")
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_inline_model_with_primary_to_field
func TestInlineModelWithPrimaryToField(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a relation points at the primary key of its target, there is no to_field; " +
		"Django: UserPreferences.user has to_field=\"username\"")
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_inline_model_with_to_field_to_rel
func TestInlineModelWithToFieldToRel(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a relation points at the primary key of its target, there is no to_field; " +
		"Django: ProfileNetwork.profile has to_field=\"user\"")
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_formset_with_none_instance
func TestFormsetWithNoneInstance(t *testing.T) {
	// A formset with instance=None can be created. Regression for #11872.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		form := forms.NewModelForm[User](db, forms.WithFields(forms.AllFields))
		formSet := forms.NewInlineFormSet[User, UserSite](db, "", forms.WithFields(forms.AllFields))

		// Instantiate the Form and FormSet to prove you can create a
		// formset with an instance of None.
		djangotest.AssertEqual(t, form.WithInstance(nil).Err(), nil)
		withNone := formSet.WithInstance(nil)
		djangotest.AssertEqual(t, len(withNone.Forms()), 3)
		djangotest.AssertEqual(t, withNone.Err(), nil)
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_empty_fields_on_modelformset
func TestEmptyFieldsOnModelformset(t *testing.T) {
	// No fields passed to modelformset_factory() should result in no fields
	// on returned forms except for the id (#14119).
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewModelFormSet[User](db, forms.WithFields())
		for _, form := range formset.Forms() {
			djangotest.AssertEqual(t, form.FieldNames(), []string{"id"})
		}
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_save_as_new_with_new_inlines
func TestSaveAsNewWithNewInlines(t *testing.T) {
	// Existing and new inlines are saved with save_as_new. Regression for
	// #14938.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		efnet := create(t, db, &Network{Name: "EFNet"})
		host1 := create(t, db, &Host{Hostname: "irc.he.net", Network: fk[Network](efnet.ID)})

		hostFormSet := forms.NewInlineFormSet[Network, Host](db, "", forms.WithFields(forms.AllFields))

		// Add a new host, modify previous host, and save-as-new.
		data := postData(
			"host_set-TOTAL_FORMS", "2",
			"host_set-INITIAL_FORMS", "1",
			"host_set-MAX_NUM_FORMS", "0",
			"host_set-0-id", strconv.FormatInt(host1.ID, 10),
			"host_set-0-hostname", "tranquility.hub.dal.net",
			"host_set-1-hostname", "matrix.de.eu.dal.net",
		)

		// To save a formset as new, it needs a new hub instance.
		dalnet := create(t, db, &Network{Name: "DALnet"})
		withNew := hostFormSet.WithInstance(dalnet)
		withNew.SaveAsNew = true
		formset := withNew.Bind(data)

		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		hostIDs := func(qs *orm.QuerySet[Host]) []int64 {
			rows := all(t, qs.OrderBy("hostname"))
			ids := make([]int64, 0, len(rows))
			for _, h := range rows {
				ids = append(ids, h.ID)
			}
			return ids
		}
		djangotest.AssertEqual(t,
			hostIDs(orm.Query[Host](db).Filter("network", dalnet.ID)),
			hostIDs(orm.Query[Host](db).Filter("hostname__in", []any{
				"matrix.de.eu.dal.net",
				"tranquility.hub.dal.net",
			})),
		)
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::InlineFormsetTests.test_initial_data
func TestInlineFormsetInitialData(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		user := create(t, db, &User{Username: "bibi", Serial: 1})
		create(t, db, &UserSite{User: fk[User](user.ID), Data: 7})
		formSet := forms.NewInlineFormSet[User, UserSite](db, "", forms.WithFields(forms.AllFields))
		formSet.Extra = 2

		withUser := formSet.WithInstance(user)
		withUser.Initial = []map[string]any{{"data": 41}, {"data": 42}}
		djangotest.AssertEqual(t, fmt.Sprint(withUser.Forms()[0].Initial["data"]), "7")
		djangotest.AssertEqual(t, fmt.Sprint(withUser.ExtraForms()[0].Initial["data"]), "41")
		html := withUser.ExtraForms()[1].AsP()
		if !strings.Contains(html, `value="42"`) {
			t.Errorf("as_p() of the second extra form misses value=\"42\":\n%s", html)
		}
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormsetTests.test_error_class
func TestFormsetErrorClass(t *testing.T) {
	// Test the type of Formset and Form error attributes.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := forms.NewModelFormSet[User](db, forms.WithFields(forms.AllFields))
		data := postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "0",
			"form-0-id", "",
			"form-0-username", "apollo13",
			"form-0-serial", "1",
			"form-1-id", "",
			"form-1-username", "apollo13",
			"form-1-serial", "2",
		)
		formset := formSet.Bind(data)
		// Check if the returned error classes are correct. Note:
		// formset.Errors() returns a list as documented.
		djangotest.AssertEqual(t, fmt.Sprintf("%T", formset.Errors()), "[]*forms.ErrorDict")
		djangotest.AssertEqual(t, fmt.Sprintf("%T", formset.NonFormErrors()), "forms.ErrorList")
		for _, form := range formset.Forms() {
			djangotest.AssertEqual(t, fmt.Sprintf("%T", form.Errors()), "*forms.ErrorDict")
			djangotest.AssertEqual(t, fmt.Sprintf("%T", form.NonFieldErrors()), "forms.ErrorList")
		}
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormsetTests.test_initial_data
func TestFormsetInitialData(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		create(t, db, &User{Username: "bibi", Serial: 1})
		formSet := forms.NewModelFormSet[User](db, forms.WithFields(forms.AllFields))
		formSet.Extra = 2
		formSet.Initial = []map[string]any{{"username": "apollo11"}, {"username": "apollo12"}}
		djangotest.AssertEqual(t, formSet.Forms()[0].Initial["username"], any("bibi"))
		djangotest.AssertEqual(t, formSet.ExtraForms()[0].Initial["username"], any("apollo11"))
		html := formSet.ExtraForms()[1].AsP()
		if !strings.Contains(html, `value="apollo12"`) {
			t.Errorf("as_p() of the second extra form misses value=\"apollo12\":\n%s", html)
		}
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormsetTests.test_extraneous_query_is_not_run
func TestExtraneousQueryIsNotRun(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := forms.NewModelFormSet[Network](db, forms.WithFields(forms.AllFields))
		formSet.Prefix = "test"
		data := postData(
			"test-TOTAL_FORMS", "1",
			"test-INITIAL_FORMS", "0",
			"test-MAX_NUM_FORMS", "",
			"test-0-name", "Random Place",
		)
		djangotest.AssertNumQueries(t, db, 1, func() {
			formset := formSet.Bind(data)
			if _, err := formset.Save(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	})
}

// customWidget is the CustomWidget of Django's tests: a TextInput of its own
// type.
type customWidget struct{ forms.TextInput }

// userSiteForm returns the options of the UserSiteForm of Django's tests:
// every field, with CustomWidget for id and data. Its localized_fields has no
// Go counterpart.
func userSiteForm(extra ...forms.ModelFormOption) []forms.ModelFormOption {
	return append([]forms.ModelFormOption{
		forms.WithFields(forms.AllFields),
		forms.WithWidget("id", customWidget{}),
		forms.WithWidget("data", customWidget{}),
	}, extra...)
}

// assertCustomWidgets checks the widgets of the form of FormfieldCallbackTests.
func assertCustomWidgets(t *testing.T, form *forms.Form) {
	t.Helper()
	for _, name := range []string{"id", "data"} {
		field, ok := form.FieldDef(name)
		if !ok {
			t.Fatalf("the form has no field %q", name)
		}
		if _, ok := field.Base().Widget.(customWidget); !ok {
			t.Errorf("the widget of %q is %T, want customWidget", name, field.Base().Widget)
		}
	}
	// FIXME-PORT: Go: forms have no localize; Django: assertFalse(form.fields["id"].localize)
	// and assertTrue(form.fields["data"].localize).
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldCallbackTests.test_inlineformset_factory_default
func TestInlineformsetFactoryDefault(t *testing.T) {
	// Regression for #13095 and #17683: Using base forms with widgets defined
	// in Meta should not raise errors and BaseModelForm should respect the
	// specified pk widget.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewInlineFormSet[User, UserSite](db, "", userSiteForm()...)
		assertCustomWidgets(t, formset.Forms()[0])
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldCallbackTests.test_modelformset_factory_default
func TestModelformsetFactoryDefault(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewModelFormSet[UserSite](db, userSiteForm()...)
		assertCustomWidgets(t, formset.Forms()[0])
	})
}

// callbackLog records the model fields that a formfield callback was called
// with, the Callback of Django's tests.
type callbackLog struct{ fields []string }

// callback records the model field and keeps its form field.
func (c *callbackLog) callback(mf *orm.Field, f forms.Field) forms.Field {
	c.fields = append(c.fields, mf.Attr)
	return f
}

// assertCallbackCalled checks the log of FormfieldCallbackTests.
//
// FIXME-PORT: Go: the callback is not called for the auto primary key and
// receives the default form field, WithWidget applies to the field it
// returns; Django: the log holds (id_field, {"widget": CustomWidget}),
// (user_field, {}) and (data_field, {"widget": CustomWidget, "localize": True}).
func assertCallbackCalled(t *testing.T, log *callbackLog) {
	t.Helper()
	djangotest.AssertEqual(t, log.fields, []string{"user", "data"})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldCallbackTests.test_inlineformset_custom_callback
func TestInlineformsetCustomCallback(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		log := &callbackLog{}
		formset := forms.NewInlineFormSet[User, UserSite](db, "",
			userSiteForm(forms.WithFormFieldCallback(log.callback))...)
		// Django's factory builds the form class, and with it the fields,
		// once; NewForm builds them for each form.
		formset.NewForm()
		assertCallbackCalled(t, log)
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldCallbackTests.test_modelformset_custom_callback
func TestModelformsetCustomCallback(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		log := &callbackLog{}
		formset := forms.NewModelFormSet[UserSite](db,
			userSiteForm(forms.WithFormFieldCallback(log.callback))...)
		formset.NewForm()
		assertCallbackCalled(t, log)
	})
}

// shouldDeleteData is the data of FormfieldShouldDeleteFormTests.
func shouldDeleteData() map[string]string {
	return map[string]string{
		"form-TOTAL_FORMS":   "4",
		"form-INITIAL_FORMS": "0",
		"form-MAX_NUM_FORMS": "4",
		"form-0-username":    "John",
		"form-0-serial":      "1",
		"form-1-username":    "Paul",
		"form-1-serial":      "2",
		"form-2-username":    "George",
		"form-2-serial":      "3",
		"form-3-username":    "Ringo",
		"form-3-serial":      "5",
	}
}

// deleteAllIDs marks the four forms of FormfieldShouldDeleteFormTests for
// deletion.
var deleteAllIDs = map[string]string{
	"form-0-DELETE": "1",
	"form-1-DELETE": "1",
	"form-2-DELETE": "1",
	"form-3-DELETE": "1",
}

// normalUserFormSet is the NormalFormset of FormfieldShouldDeleteFormTests.
func normalUserFormSet(db *orm.DB) *forms.ModelFormSet[User] {
	fs := forms.NewModelFormSet[User](db, forms.WithFields(forms.AllFields))
	fs.CanDelete = true
	return fs
}

// initShouldDeleteDatabase adds the test data of
// FormfieldShouldDeleteFormTests through a formset, its test_init_database.
func initShouldDeleteDatabase(t *testing.T, db *orm.DB) {
	t.Helper()
	formset := normalUserFormSet(db).Bind(postData(flatten(shouldDeleteData())...))
	djangotest.AssertEqual(t, formset.IsValid(), true)
	saved, err := formset.Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	djangotest.AssertEqual(t, len(saved), 4)
}

// flatten returns the keys and values of m as alternating items, in key
// order.
func flatten(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		out = append(out, k, m[k])
	}
	return out
}

// withUserIDs returns data with INITIAL_FORMS 4 and the primary keys of the
// users in order, as FormfieldShouldDeleteFormTests builds it.
func withUserIDs(t *testing.T, db *orm.DB, data map[string]string) map[string]string {
	t.Helper()
	data["form-INITIAL_FORMS"] = "4"
	for i, user := range all(t, orm.Query[User](db).OrderBy("pk")) {
		data[fmt.Sprintf("form-%d-id", i)] = strconv.FormatInt(user.ID, 10)
	}
	return data
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldShouldDeleteFormTests.test_init_database
func TestShouldDeleteInitDatabase(t *testing.T) {
	// Add test data to database via formset.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		initShouldDeleteDatabase(t, db)
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldShouldDeleteFormTests.test_no_delete
func TestShouldDeleteNoDelete(t *testing.T) {
	// Verify base formset doesn't modify database.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// Reload database.
		initShouldDeleteDatabase(t, db)

		// Pass standard data dict & see none updated.
		data := withUserIDs(t, db, shouldDeleteData())
		fs := normalUserFormSet(db)
		fs.QuerySet = orm.Query[User](db)
		formset := fs.Bind(postData(flatten(data)...))
		djangotest.AssertEqual(t, formset.IsValid(), true)
		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 0)
		djangotest.AssertEqual(t, count[User](t, db), int64(4))
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldShouldDeleteFormTests.test_all_delete
func TestShouldDeleteAllDelete(t *testing.T) {
	// Verify base formset honors DELETE field.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// Reload database.
		initShouldDeleteDatabase(t, db)

		// Create data dict with all fields marked for deletion.
		data := withUserIDs(t, db, shouldDeleteData())
		for k, v := range deleteAllIDs {
			data[k] = v
		}
		fs := normalUserFormSet(db)
		fs.QuerySet = orm.Query[User](db)
		formset := fs.Bind(postData(flatten(data)...))
		djangotest.AssertEqual(t, formset.IsValid(), true)
		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 0)
		djangotest.AssertEqual(t, count[User](t, db), int64(0))
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::FormfieldShouldDeleteFormTests.test_custom_delete
func TestShouldDeleteCustomDelete(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a formset has no hook that decides whether a form is deleted and none that " +
		"drops the DELETE field; Django: BaseCustomDeleteFormSet overrides _should_delete_form and add_fields")
}

// redeleteData is the data of RedeleteTests for the user u and the row us.
// The user value is the primary key of the user, as UserSite points at it.
func redeleteData(u *User, us *UserSite) []string {
	return []string{
		"serial", "1",
		"username", "foo",
		"usersite_set-TOTAL_FORMS", "1",
		"usersite_set-INITIAL_FORMS", "1",
		"usersite_set-MAX_NUM_FORMS", "1",
		"usersite_set-0-id", strconv.FormatInt(us.ID, 10),
		"usersite_set-0-data", "7",
		"usersite_set-0-user", strconv.FormatInt(u.ID, 10),
		"usersite_set-0-DELETE", "1",
	}
}

// Ported from Django: tests/model_formsets_regress/tests.py::RedeleteTests.test_resubmit
func TestResubmit(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		u := create(t, db, &User{Username: "foo", Serial: 1})
		us := create(t, db, &UserSite{User: fk[User](u.ID), Data: 7})
		formsetCls := forms.NewInlineFormSet[User, UserSite](db, "", forms.WithFields(forms.AllFields))
		data := postData(redeleteData(u, us)...)
		formset := formsetCls.WithInstance(u).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count[UserSite](t, db), int64(0))
		formset = formsetCls.WithInstance(u).Bind(data)
		// Even if the "us" object isn't in the DB any more, the form
		// validates.
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count[UserSite](t, db), int64(0))
	})
}

// Ported from Django: tests/model_formsets_regress/tests.py::RedeleteTests.test_delete_already_deleted
func TestDeleteAlreadyDeleted(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		u := create(t, db, &User{Username: "foo", Serial: 1})
		us := create(t, db, &UserSite{User: fk[User](u.ID), Data: 7})
		formsetCls := forms.NewInlineFormSet[User, UserSite](db, "", forms.WithFields(forms.AllFields))
		formset := formsetCls.WithInstance(u).Bind(postData(redeleteData(u, us)...))
		if _, err := orm.Query[UserSite](db).Filter("pk", us.ID).Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count[UserSite](t, db), int64(0))
	})
}
