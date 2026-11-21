package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// FoodDelivery stands in for tests/admin_views/models.py::FoodDelivery.
type FoodDelivery struct {
	ID         int64
	Reference  string `orm:"max_length=100"`
	Driver     string `orm:"max_length=100;blank"`
	Restaurant string `orm:"max_length=100;blank"`
}

// Meta declares the choices and the unique pair of the driver and the
// restaurant.
func (FoodDelivery) Meta() orm.Meta {
	return orm.Meta{
		UniqueTogether: [][]string{{"Driver", "Restaurant"}},
		Choices: map[string][]orm.Choice{
			"Driver": {orm.ChoiceOf("bill", "Bill G"), orm.ChoiceOf("steve", "Steve J")},
			"Restaurant": {
				orm.ChoiceOf("indian", "A Taste of India"),
				orm.ChoiceOf("thai", "Thai Pography"),
				orm.ChoiceOf("pizza", "Pizza Mama"),
			},
		},
	}
}

// Collector stands in for tests/admin_views/models.py::Collector.
type Collector struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Category stands in for tests/admin_views/models.py::Category.
type Category struct {
	ID        int64
	Collector orm.FK[Collector] `orm:"on_delete=cascade"`
	Order     uint32
}

// Meta orders by order.
func (Category) Meta() orm.Meta { return orm.Meta{Ordering: []string{"order"}} }

// String returns "<id>:o<order>".
func (c Category) String() string { return fmt.Sprintf("%d:o%d", c.ID, c.Order) }

// UnorderedObject stands in for tests/admin_views/models.py::UnorderedObject.
type UnorderedObject struct {
	ID   int64
	Name string `orm:"max_length=255"`
	Bool bool   `orm:"default=true"`
}

// Story stands in for tests/admin_views/models.py::Story.
type Story struct {
	ID      int64
	Title   string `orm:"max_length=100"`
	Content string `orm:"type=text"`
}

// OtherStory stands in for tests/admin_views/models.py::OtherStory.
type OtherStory struct {
	ID      int64
	Title   string `orm:"max_length=100"`
	Content string `orm:"type=text"`
}

// ParentWithUUIDPK stands in for tests/admin_views/models.py::ParentWithUUIDPK.
// The admin needs an integer primary key, so its primary key is an integer.
type ParentWithUUIDPK struct {
	ID    int64
	Title string `orm:"max_length=100"`
}

// String returns the primary key.
func (p ParentWithUUIDPK) String() string { return fmt.Sprint(p.ID) }

// listEditableModels returns the models of AdminViewListEditable.
func listEditableModels() []any {
	return append(saveAsModels(), djangotest.App("admin_views",
		&FoodDelivery{}, &Collector{}, &Category{}, &UnorderedObject{}, &Story{}, &OtherStory{}, &ParentWithUUIDPK{}))
}

// personAdmin returns PersonAdmin of tests/admin_views/admin.py. Its
// get_queryset orders by age, which the ordering of the changelist replaces,
// so it has no Go counterpart.
func personAdmin() admin.ModelAdmin {
	return admin.ModelAdmin{
		ListDisplay:  []string{"name", "gender", "alive"},
		ListEditable: []string{"gender", "alive"},
		ListFilter:   []any{"gender"},
		SearchFields: []string{"^name"},
		SaveAs:       true,
		ChangelistFormSet: func(_ *http.Request, fs *forms.FormSet) {
			fs.CleanWith(personFormSetClean)
		},
	}
}

// personFormSetClean is the clean of BasePersonModelFormSet of
// tests/admin_views/admin.py.
func personFormSetClean(fs *forms.FormSet) error {
	for _, form := range fs.Forms() {
		cleaned := form.Cleaned()
		person, _ := cleaned["id"].(*Person)
		alive, _ := cleaned["alive"].(bool)
		if person != nil && alive && person.Name == "Grace Hopper" {
			return forms.NewValidationError("Grace is not a Zombie", "")
		}
	}
	return nil
}

// listEditableEnv registers the admins of tests/admin_views/admin.py that
// AdminViewListEditable uses on the site "admin", creates per1, per2 and
// per3 and logs in the superuser, as AdminViewListEditable.setUpTestData and
// setUp do. The section, the articles and the post of setUpTestData are left
// out: no test of the class reads them.
func listEditableEnv(t *testing.T, db *orm.DB) (*adminTestEnv, [3]*Person) {
	t.Helper()
	env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		admin.Register[Person](site, personAdmin())
		admin.Register[FoodDelivery](site, admin.ModelAdmin{
			ListDisplay:  []string{"reference", "driver", "restaurant"},
			ListEditable: []string{"driver", "restaurant"},
		})
		admin.Register[Category](site, admin.ModelAdmin{
			ListDisplay:  []string{"id", "collector", "order"},
			ListEditable: []string{"order"},
		})
		admin.Register[UnorderedObject](site, admin.ModelAdmin{
			ListDisplay:      []string{"id", "name"},
			ListDisplayLinks: []string{"id"},
			ListEditable:     []string{"name"},
			ListPerPage:      2,
		})
		// StoryAdmin's form hides the title, which the changelist does not
		// edit, so the form has no Go counterpart.
		admin.Register[Story](site, admin.ModelAdmin{
			ListDisplay:      []string{"id", "title", "content"},
			ListDisplayLinks: []string{"title"},
			ListEditable:     []string{"content"},
			Ordering:         []string{"-id"},
		})
		admin.Register[OtherStory](site, admin.ModelAdmin{
			ListDisplay:      []string{"id", "title", "content"},
			ListDisplayLinks: []string{"title", "id"},
			ListEditable:     []string{"content"},
			Ordering:         []string{"-id"},
		})
	})
	env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
	per := [3]*Person{
		mustCreate(t, db, &Person{Name: "John Mauchly", Gender: 1, Alive: true}),
		mustCreate(t, db, &Person{Name: "Grace Hopper", Gender: 1, Alive: false}),
		mustCreate(t, db, &Person{Name: "Guido van Rossum", Gender: 1, Alive: true}),
	}
	// Create gives a false Alive its default, true.
	if _, err := orm.Query[Person](db).Filter("id", per[1].ID).Update(context.Background(), "alive", false); err != nil {
		t.Fatalf("Grace Hopper: %v", err)
	}
	per[1].Alive = false
	return env, per
}

// personFormData returns the data of the three forms of the Person
// changelist that most tests of AdminViewListEditable post: per1 dies, per2
// becomes female and per3 stays.
func personFormData(per [3]*Person) url.Values {
	return url.Values{
		"form-TOTAL_FORMS":   {"3"},
		"form-INITIAL_FORMS": {"3"},
		"form-MAX_NUM_FORMS": {"0"},
		"form-0-gender":      {"1"},
		"form-0-id":          {fmt.Sprint(per[0].ID)},
		"form-1-gender":      {"2"},
		"form-1-id":          {fmt.Sprint(per[1].ID)},
		"form-2-alive":       {"checked"},
		"form-2-gender":      {"1"},
		"form-2-id":          {fmt.Sprint(per[2].ID)},
	}
}

// getPerson returns the person named name.
func getPerson(t *testing.T, db *orm.DB, name string) *Person {
	t.Helper()
	p, err := orm.Query[Person](db).Filter("name", name).Get(context.Background())
	if err != nil {
		t.Fatalf("person %q: %v", name, err)
	}
	return &p
}

// FIXME-PORT: tests/admin_views/tests.py::AdminViewListEditable.test_inheritance
// and test_inheritance_2 need multi-table inheritance.

// FIXME-PORT: tests/admin_views/tests.py::AdminViewListEditable.test_custom_pk
// needs an admin for a model with a primary key that is not an integer.

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_changelist_input_html
func TestListEditableChangelistInputHTML(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, _ := listEditableEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_person_changelist"))
		// 2 inputs per object(the field and the hidden id field) = 6
		// 4 management hidden fields = 4
		// 4 action inputs (3 regular checkboxes, 1 checkbox to select all)
		// main form submit button = 1
		// search field and search submit button = 2
		// CSRF field = 2
		// field to track 'select all' across paginated views = 1
		// 6 + 4 + 4 + 1 + 2 + 2 + 1 = 20 inputs
		djangotest.AssertContains(t, resp, "<input", djangotest.Count(21))
		// 1 select per object = 3 selects
		djangotest.AssertContains(t, resp, "<select", djangotest.Count(4))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_actions_counter_is_live_region
func TestListEditableActionsCounterIsLiveRegion(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, _ := listEditableEnv(t, db)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_person_changelist"))
		djangotest.AssertContains(t, resp,
			`class="action-counter" data-actions-icnt="3" aria-live="polite" aria-atomic="true"`)
	})
}

// Django counts the messages; the test compares the one message.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_post_messages
func TestListEditablePostMessages(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, per := listEditableEnv(t, db)
		// Ticket 12707: Saving inline editable should not show admin
		// action warnings
		data := personFormData(per)
		data.Set("_save", "Save")
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_person_changelist"), data, djangotest.Follow())
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelSuccess, Message: "2 persons were changed successfully."},
		})
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_post_submission
func TestListEditablePostSubmission(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, per := listEditableEnv(t, db)
		u := mustReverse(t, env.router, "admin:admin_views_person_changelist")
		data := personFormData(per)
		data.Set("_save", "Save")
		env.client.Post(u, data)

		djangotest.AssertEqual(t, getPerson(t, db, "John Mauchly").Alive, false)
		djangotest.AssertEqual(t, getPerson(t, db, "Grace Hopper").Gender, int64(2))

		// test a filtered page
		data = url.Values{
			"form-TOTAL_FORMS":   {"2"},
			"form-INITIAL_FORMS": {"2"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-id":          {fmt.Sprint(per[0].ID)},
			"form-0-gender":      {"1"},
			"form-0-alive":       {"checked"},
			"form-1-id":          {fmt.Sprint(per[2].ID)},
			"form-1-gender":      {"1"},
			"form-1-alive":       {"checked"},
			"_save":              {"Save"},
		}
		env.client.Post(u+"?gender__exact=1", data)

		djangotest.AssertEqual(t, getPerson(t, db, "John Mauchly").Alive, true)

		// test a searched page
		data = url.Values{
			"form-TOTAL_FORMS":   {"1"},
			"form-INITIAL_FORMS": {"1"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-id":          {fmt.Sprint(per[0].ID)},
			"form-0-gender":      {"1"},
			"_save":              {"Save"},
		}
		env.client.Post(u+"?q=john", data)

		djangotest.AssertEqual(t, getPerson(t, db, "John Mauchly").Alive, false)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_forged_post_submission_when_no_add_permission
func TestForgedPostSubmissionWhenNoAddPermission(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env7 := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.Name = "admin7"
			site.Prefix = "/test_admin/admin7"
			// ParentWithUUIDPKNoAddAdmin of tests/admin_views/admin.py.
			admin.Register[ParentWithUUIDPK](site, admin.ModelAdmin{
				ListDisplay:      []string{"id", "title"},
				ListEditable:     []string{"title"},
				HasAddPermission: func(*http.Request) bool { return false },
			})
		})
		env7.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))
		beforeCount, _ := orm.Query[ParentWithUUIDPK](db).Count(ctx)
		data := url.Values{
			"form-TOTAL_FORMS":   {"1"},
			"form-INITIAL_FORMS": {"0"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-title":       {"The News"},
			"form-0-id":          {""},
			"_save":              {"Save"},
		}
		// This model admin allows no add permissions.
		resp := env7.client.Post(mustReverse(t, env7.router, "admin7:admin_views_parentwithuuidpk_changelist"), data)
		djangotest.AssertStatus(t, resp, http.StatusBadRequest)
		n, _ := orm.Query[ParentWithUUIDPK](db).Count(ctx)
		djangotest.AssertEqual(t, n, beforeCount)
	})
}

// Non-field errors are displayed for each of the forms in the changelist's
// formset.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_non_field_errors
func TestListEditableNonFieldErrors(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, _ := listEditableEnv(t, db)
		fd1 := mustCreate(t, db, &FoodDelivery{Reference: "123", Driver: "bill", Restaurant: "thai"})
		fd2 := mustCreate(t, db, &FoodDelivery{Reference: "456", Driver: "bill", Restaurant: "india"})
		fd3 := mustCreate(t, db, &FoodDelivery{Reference: "789", Driver: "bill", Restaurant: "pizza"})
		u := mustReverse(t, env.router, "admin:admin_views_fooddelivery_changelist")
		const nonFieldError = `<tr><td colspan="4"><ul class="errorlist nonfield"><li>Food delivery ` +
			`with this Driver and Restaurant already exists.</li></ul></td></tr>`

		data := url.Values{
			"form-TOTAL_FORMS":   {"3"},
			"form-INITIAL_FORMS": {"3"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-id":          {fmt.Sprint(fd1.ID)},
			"form-0-reference":   {"123"},
			"form-0-driver":      {"bill"},
			"form-0-restaurant":  {"thai"},
			// Same data as above: Forbidden because of unique_together!
			"form-1-id":         {fmt.Sprint(fd2.ID)},
			"form-1-reference":  {"456"},
			"form-1-driver":     {"bill"},
			"form-1-restaurant": {"thai"},
			"form-2-id":         {fmt.Sprint(fd3.ID)},
			"form-2-reference":  {"789"},
			"form-2-driver":     {"bill"},
			"form-2-restaurant": {"pizza"},
			"_save":             {"Save"},
		}
		resp := env.client.Post(u, data)
		djangotest.AssertContains(t, resp, nonFieldError, djangotest.Count(1), djangotest.HTML())

		// Same data also.
		data.Set("form-2-restaurant", "thai")
		resp = env.client.Post(u, data)
		djangotest.AssertContains(t, resp, nonFieldError, djangotest.Count(2), djangotest.HTML())
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_non_form_errors
func TestListEditableNonFormErrors(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, per := listEditableEnv(t, db)
		// test if non-form errors are handled; ticket #12716
		data := url.Values{
			"form-TOTAL_FORMS":   {"1"},
			"form-INITIAL_FORMS": {"1"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-id":          {fmt.Sprint(per[1].ID)},
			"form-0-alive":       {"1"},
			"form-0-gender":      {"2"},
			// The form processing understands this as a list_editable "Save"
			// and not an action "Run".
			"_save": {"Save"},
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_person_changelist"), data)
		djangotest.AssertContains(t, resp, "Grace is not a Zombie")
	})
}

// The page context holds the rendered errors, not the ErrorList.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_non_form_errors_is_errorlist
func TestListEditableNonFormErrorsIsErrorList(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, per := listEditableEnv(t, db)
		// test if non-form errors are correctly handled; ticket #12878
		data := url.Values{
			"form-TOTAL_FORMS":   {"1"},
			"form-INITIAL_FORMS": {"1"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-id":          {fmt.Sprint(per[1].ID)},
			"form-0-alive":       {"1"},
			"form-0-gender":      {"2"},
			"_save":              {"Save"},
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_person_changelist"), data)
		djangotest.AssertEqual(t, fmt.Sprint(ctxVal(t, resp, "non_form_errors")),
			`<ul class="errorlist nonform"><li>Grace is not a Zombie</li></ul>`)
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_list_editable_ordering
func TestListEditableOrdering(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env, _ := listEditableEnv(t, db)
		collector := mustCreate(t, db, &Collector{Name: "Frederick Clegg"})
		newCategory := func(order uint32) *Category {
			c := &Category{Order: order}
			c.Collector.SetPK(collector.ID)
			return mustCreate(t, db, c)
		}
		category1, category2, category3, category4 := newCategory(1), newCategory(2), newCategory(0), newCategory(0)
		// NB: The order values must be changed so that the items are reordered.
		data := url.Values{
			"form-TOTAL_FORMS":   {"4"},
			"form-INITIAL_FORMS": {"4"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-order":       {"14"},
			"form-0-id":          {fmt.Sprint(category1.ID)},
			"form-0-collector":   {fmt.Sprint(collector.ID)},
			"form-1-order":       {"13"},
			"form-1-id":          {fmt.Sprint(category2.ID)},
			"form-1-collector":   {fmt.Sprint(collector.ID)},
			"form-2-order":       {"1"},
			"form-2-id":          {fmt.Sprint(category3.ID)},
			"form-2-collector":   {fmt.Sprint(collector.ID)},
			"form-3-order":       {"0"},
			"form-3-id":          {fmt.Sprint(category4.ID)},
			"form-3-collector":   {fmt.Sprint(collector.ID)},
			// The form processing understands this as a list_editable "Save"
			// and not an action "Run".
			"_save": {"Save"},
		}
		resp := env.client.Post(mustReverse(t, env.router, "admin:admin_views_category_changelist"), data)
		// Successful post will redirect
		djangotest.AssertStatus(t, resp, http.StatusFound)

		// The order values have been applied to the right objects
		for _, tc := range []struct {
			c     *Category
			order uint32
		}{{category1, 14}, {category2, 13}, {category3, 1}, {category4, 0}} {
			got, err := orm.Query[Category](db).Filter("id", tc.c.ID).Get(ctx)
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, got.Order, tc.order)
		}
	})
}

// Pagination works for list_editable items.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_list_editable_pagination
func TestListEditablePagination(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, _ := listEditableEnv(t, db)
		mustCreate(t, db, &UnorderedObject{Name: "Unordered object #1"})
		mustCreate(t, db, &UnorderedObject{Name: "Unordered object #2"})
		mustCreate(t, db, &UnorderedObject{Name: "Unordered object #3"})
		u := mustReverse(t, env.router, "admin:admin_views_unorderedobject_changelist")
		resp := env.client.Get(u)
		djangotest.AssertContains(t, resp, "Unordered object #3")
		djangotest.AssertContains(t, resp, "Unordered object #2")
		djangotest.AssertNotContains(t, resp, "Unordered object #1")
		resp = env.client.Get(u + "?p=2")
		djangotest.AssertNotContains(t, resp, "Unordered object #3")
		djangotest.AssertNotContains(t, resp, "Unordered object #2")
		djangotest.AssertContains(t, resp, "Unordered object #1")
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_list_editable_action_submit
func TestListEditableActionSubmit(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, per := listEditableEnv(t, db)
		// List editable changes should not be executed if the action "Run"
		// button is used to submit the form.
		data := personFormData(per)
		data.Set("index", "0")
		data["_selected_action"] = []string{fmt.Sprint(per[2].ID)}
		data["action"] = []string{"", "delete_selected"}
		env.client.Post(mustReverse(t, env.router, "admin:admin_views_person_changelist"), data)

		djangotest.AssertEqual(t, getPerson(t, db, "John Mauchly").Alive, true)
		djangotest.AssertEqual(t, getPerson(t, db, "Grace Hopper").Gender, int64(1))
	})
}

// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_list_editable_action_choices
func TestListEditableActionChoices(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, per := listEditableEnv(t, db)
		// List editable changes should be executed if the "Save" button is
		// used to submit the form - any action choices should be ignored.
		data := personFormData(per)
		data.Set("_save", "Save")
		data["_selected_action"] = []string{fmt.Sprint(per[0].ID)}
		data["action"] = []string{"", "delete_selected"}
		env.client.Post(mustReverse(t, env.router, "admin:admin_views_person_changelist"), data)

		djangotest.AssertEqual(t, getPerson(t, db, "John Mauchly").Alive, false)
		djangotest.AssertEqual(t, getPerson(t, db, "Grace Hopper").Gender, int64(2))
	})
}

// Fields should not be list-editable in popups.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_list_editable_popup
func TestListEditablePopup(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, _ := listEditableEnv(t, db)
		u := mustReverse(t, env.router, "admin:admin_views_person_changelist")
		resp := env.client.Get(u)
		djangotest.AssertEqual(t, ctxVal(t, resp, "list_editable"), any([]string{"gender", "alive"}))
		resp = env.client.Get(u + "?_popup")
		djangotest.AssertEqual(t, ctxVal(t, resp, "list_editable"), any([]string(nil)))
	})
}

// hidden pk fields aren't displayed in the table body and their
// corresponding human-readable value is displayed instead. The hidden pk
// fields are displayed but separately (not in the table) and only once.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_pk_hidden_fields
func TestListEditablePkHiddenFields(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, _ := listEditableEnv(t, db)
		story1 := mustCreate(t, db, &Story{Title: "The adventures of Guido", Content: "Once upon a time in Djangoland..."})
		story2 := mustCreate(t, db, &Story{Title: "Crouching Tiger, Hidden Python", Content: "The Python was sneaking into..."})
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_story_changelist"))
		// Only one hidden field, in a separate place than the table.
		djangotest.AssertContains(t, resp, `id="id_form-0-id"`, djangotest.Count(1))
		djangotest.AssertContains(t, resp, `id="id_form-1-id"`, djangotest.Count(1))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<div class="hiddenfields">
<input type="hidden" name="form-0-id" value="%d" id="id_form-0-id"><input type="hidden" name="form-1-id" value="%d" id="id_form-1-id">
</div>`, story2.ID, story1.ID), djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<td class="field-id">%d</td>`, story1.ID), djangotest.Count(1))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<td class="field-id">%d</td>`, story2.ID), djangotest.Count(1))
	})
}

// Similarly as test_pk_hidden_fields, but when the hidden pk fields are
// referenced in list_display_links. Refs #12475.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_pk_hidden_fields_with_list_display_links
func TestListEditablePkHiddenFieldsWithListDisplayLinks(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		env, _ := listEditableEnv(t, db)
		story1 := mustCreate(t, db, &OtherStory{Title: "The adventures of Guido", Content: "Once upon a time in Djangoland..."})
		story2 := mustCreate(t, db, &OtherStory{Title: "Crouching Tiger, Hidden Python", Content: "The Python was sneaking into..."})
		link1 := mustReverse(t, env.router, "admin:admin_views_otherstory_change", story1.ID)
		link2 := mustReverse(t, env.router, "admin:admin_views_otherstory_change", story2.ID)
		resp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_otherstory_changelist"))
		// Only one hidden field, in a separate place than the table.
		djangotest.AssertContains(t, resp, `id="id_form-0-id"`, djangotest.Count(1))
		djangotest.AssertContains(t, resp, `id="id_form-1-id"`, djangotest.Count(1))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<div class="hiddenfields">
<input type="hidden" name="form-0-id" value="%d" id="id_form-0-id"><input type="hidden" name="form-1-id" value="%d" id="id_form-1-id">
</div>`, story2.ID, story1.ID), djangotest.HTML())
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<th class="field-id"><a href="%s">%d</a></th>`, link1, story1.ID), djangotest.Count(1))
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<th class="field-id"><a href="%s">%d</a></th>`, link2, story2.ID), djangotest.Count(1))
	})
}

// listEditableEnv9 registers PersonNoChangePermissionsAdmin9 of
// tests/admin_views/admin.py on the site "admin9" and logs in the superuser.
func listEditableEnv9(t *testing.T, db *orm.DB) *adminTestEnv {
	t.Helper()
	env9 := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
		site.Name = "admin9"
		site.Prefix = "/test_admin/admin9"
		admin.Register[Person](site, admin.ModelAdmin{
			ListDisplay:  []string{"name", "gender", "alive"},
			ListEditable: []string{"gender", "alive"},
			Ordering:     []string{"id"},
			HasChangePermission: func(_ *http.Request, obj any) bool {
				if obj == nil {
					return true
				}
				return obj.(*Person).Alive
			},
		})
	})
	env9.client.ForceLogin(createTestUser(t, db, "super9", "secret", true, true))
	return env9
}

// List_editable fields are stripped for objects where the user lacks change
// permissions, and retained for objects where the user has permissions.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_list_editable_per_object_permissions
func TestListEditablePerObjectPermissions(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		listEditableEnv(t, db)
		env9 := listEditableEnv9(t, db)
		resp := env9.client.Get(mustReverse(t, env9.router, "admin9:admin_views_person_changelist"))
		// Non-editable fields should NOT have inputs.
		djangotest.AssertNotContains(t, resp, `name="form-1-gender"`)
		djangotest.AssertNotContains(t, resp, `name="form-1-alive"`)
		// Editable fields are present.
		djangotest.AssertContains(t, resp, `name="form-0-gender"`)
		djangotest.AssertContains(t, resp, `name="form-0-alive"`)
		djangotest.AssertContains(t, resp, `name="form-2-gender"`)
		djangotest.AssertContains(t, resp, `name="form-2-alive"`)
	})
}

// Form submission updates only objects where the user has change
// permissions, ignoring changes to unauthorized objects.
//
// Django counts the messages; the test compares the one message.
//
// Ported from Django: tests/admin_views/tests.py::AdminViewListEditable.test_list_editable_per_object_permissions_submission
func TestListEditablePerObjectPermissionsSubmission(t *testing.T) {
	djangotest.EachDB(t, listEditableModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, per := listEditableEnv(t, db)
		env9 := listEditableEnv9(t, db)
		// Skip the instance lacking edit permission (include only its id).
		data := url.Values{
			"form-TOTAL_FORMS":   {"3"},
			"form-INITIAL_FORMS": {"3"},
			"form-MAX_NUM_FORMS": {"0"},
			"form-0-gender":      {"2"},
			"form-0-alive":       {"checked"},
			"form-0-id":          {fmt.Sprint(per[0].ID)},
			"form-1-id":          {fmt.Sprint(per[1].ID)}, // not editable
			"form-2-gender":      {"2"},
			"form-2-alive":       {"checked"},
			"form-2-id":          {fmt.Sprint(per[2].ID)},
			"_save":              {"Save"},
		}
		resp := env9.client.Post(mustReverse(t, env9.router, "admin9:admin_views_person_changelist"), data, djangotest.Follow())
		// per1 and per3 were updated, but per2 was not.
		for _, tc := range []struct {
			p      *Person
			gender int64
		}{{per[0], 2}, {per[1], 1}, {per[2], 2}} {
			got, err := orm.Query[Person](db).Filter("id", tc.p.ID).Get(ctx)
			if err != nil {
				t.Fatal(err)
			}
			djangotest.AssertEqual(t, got.Gender, tc.gender)
		}

		// Check for success message
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelSuccess, Message: "2 persons were changed successfully."},
		})
	})
}
