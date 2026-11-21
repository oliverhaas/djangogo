package admin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from tests/admin_views/models.py for action tests.

type ActSubscriber struct {
	ID    int64
	Name  string `orm:"max_length=100"`
	Email string `orm:"max_length=254"`
}

func (s ActSubscriber) String() string { return s.Name }

func (ActSubscriber) Meta() orm.Meta {
	return orm.Meta{VerboseName: "subscriber", VerboseNamePlural: "subscribers"}
}

type ActSubscriptionNote struct {
	ID         int64
	Subscriber orm.FK[ActSubscriber] `orm:"on_delete=cascade"`
	Body       string                `orm:"max_length=100"`
}

func (n ActSubscriptionNote) String() string { return n.Body }

func (ActSubscriptionNote) Meta() orm.Meta {
	return orm.Meta{VerboseName: "subscription note", VerboseNamePlural: "subscription notes"}
}

type ActQuestion struct {
	ID       int64
	Question string `orm:"max_length=100"`
}

func (q ActQuestion) String() string { return q.Question }

func (ActQuestion) Meta() orm.Meta {
	return orm.Meta{VerboseName: "question", VerboseNamePlural: "questions"}
}

type ActAnswer struct {
	ID       int64
	Question orm.FK[ActQuestion] `orm:"on_delete=protect"`
	Answer   string              `orm:"max_length=100"`
}

func (a ActAnswer) String() string { return a.Answer }

func (ActAnswer) Meta() orm.Meta {
	return orm.Meta{VerboseName: "answer", VerboseNamePlural: "answers"}
}

type ActBook struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

func (b ActBook) String() string { return b.Name }

func (ActBook) Meta() orm.Meta {
	return orm.Meta{VerboseName: "book", VerboseNamePlural: "books"}
}

type ActOldSubscriber struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

func (o ActOldSubscriber) String() string { return o.Name }

func adminActionModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		djangotest.App("admin_views",
			&ActSubscriber{},
			&ActSubscriptionNote{},
			&ActQuestion{},
			&ActAnswer{},
			&ActBook{},
			&ActOldSubscriber{},
		),
	}
}

// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_model_admin_custom_action
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_custom_function_action_with_redirect
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_default_redirect
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_custom_function_action_streaming_response
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_custom_function_action_no_perm_response
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_actions_ordering
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_multiple_actions_form
func TestCustomActionsAndOrdering(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		s1 := &ActSubscriber{Name: "John Doe", Email: "john@example.org"}
		s2 := &ActSubscriber{Name: "Max Mustermann", Email: "max@example.org"}
		_ = orm.Query[ActSubscriber](db).Create(ctx, s1)
		_ = orm.Query[ActSubscriber](db).Create(ctx, s2)

		var mailed []string
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActSubscriber](site, admin.ModelAdmin{
				ListDisplay:     []string{"name", "email"},
				ActionsOnBottom: true,
				Actions: []admin.Action{
					{
						Name:        "redirect_to",
						Description: "Redirect to (Awesome action)",
						Handler: func(w http.ResponseWriter, r *http.Request, _ *admin.ActionContext) (bool, error) {
							http.Redirect(w, r, "/custom-target/", http.StatusFound)
							return true, nil
						},
					},
					{
						Name:        "external_mail",
						Description: "External mail (Another awesome action)",
						Handler: func(_ http.ResponseWriter, r *http.Request, act *admin.ActionContext) (bool, error) {
							objs, err := act.Objects(r.Context())
							if err != nil {
								return false, err
							}
							for _, o := range objs {
								mailed = append(mailed, o.(*ActSubscriber).Name)
							}
							return false, nil
						},
					},
					{
						Name:        "download",
						Description: "Download selected subscriptions",
						Handler: func(w http.ResponseWriter, r *http.Request, act *admin.ActionContext) (bool, error) {
							objs, err := act.Objects(r.Context())
							if err != nil {
								return false, err
							}
							sub := objs[0].(*ActSubscriber)
							w.WriteHeader(http.StatusOK)
							_, _ = fmt.Fprintf(w, "This is the content of the file written by %s", sub.Name)
							return true, nil
						},
					},
					{
						Name:        "no_perm",
						Description: "No permission to run",
						Handler: func(w http.ResponseWriter, _ *http.Request, _ *admin.ActionContext) (bool, error) {
							w.WriteHeader(http.StatusForbidden)
							_, _ = w.Write([]byte("No permission to perform this action"))
							return true, nil
						},
					},
				},
			})
		})
		env.client.ForceLogin(super)
		clURL := mustReverse(t, env.router, "admin:admin_views_actsubscriber_changelist")

		// Action dropdown ordering: blank option, delete_selected, then ModelAdmin actions in order.
		resp := env.client.Get(clURL)
		djangotest.AssertContains(t, resp,
			`<label>Action: <select name="action" required>`+
				`<option value="" selected>- Select an option -</option>`+
				`<option value="delete_selected">Delete selected subscribers</option>`+
				`<option value="redirect_to">Redirect to (Awesome action)</option>`+
				`<option value="external_mail">External mail (Another awesome action)</option>`+
				`<option value="download">Download selected subscriptions</option>`+
				`<option value="no_perm">No permission to run</option>`+
				`</select></label>`,
			djangotest.HTML(),
		)

		// Custom action returning handled=false redirects back to same URL preserving querystring.
		mailed = nil
		urlWithSort := clURL + "?o=1"
		resp = env.client.Post(urlWithSort, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10)},
			"action":                 {"external_mail"},
			"index":                  {"0"},
		})
		djangotest.AssertRedirects(t, resp, urlWithSort)
		djangotest.AssertSequenceEqual(t, mailed, []string{"John Doe"})

		// Multiple actions form: index=0 picks first action ("external_mail"), not "delete_selected".
		mailed = nil
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10)},
			"action":                 {"external_mail", "delete_selected"},
			"index":                  {"0"},
		})
		djangotest.AssertRedirects(t, resp, clURL)
		djangotest.AssertSequenceEqual(t, mailed, []string{"John Doe"})
		cnt, _ := orm.Query[ActSubscriber](db).Count(ctx)
		djangotest.AssertEqual(t, cnt, int64(2))

		// Custom redirect response.
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10)},
			"action":                 {"redirect_to"},
			"index":                  {"0"},
		})
		djangotest.AssertStatus(t, resp, http.StatusFound)
		djangotest.AssertEqual(t, resp.Header.Get("Location"), "/custom-target/")

		// Custom 200 download response.
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10)},
			"action":                 {"download"},
			"index":                  {"0"},
		})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertEqual(t, string(resp.Body), "This is the content of the file written by John Doe")

		// Custom 403 response.
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10)},
			"action":                 {"no_perm"},
			"index":                  {"0"},
		})
		djangotest.AssertStatus(t, resp, http.StatusForbidden)
		djangotest.AssertEqual(t, string(resp.Body), "No permission to perform this action")
	})
}

// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_model_admin_default_delete_action
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_default_delete_action_nonexistent_pk
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_delete_queryset_hook
func TestDefaultDeleteSelectedAction(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		s1 := &ActSubscriber{Name: "John Doe", Email: "john@example.org"}
		s2 := &ActSubscriber{Name: "Max Mustermann", Email: "max@example.org"}
		_ = orm.Query[ActSubscriber](db).Create(ctx, s1)
		_ = orm.Query[ActSubscriber](db).Create(ctx, s2)
		note := &ActSubscriptionNote{Body: "VIP note"}
		note.Subscriber.SetPK(s1.ID)
		_ = orm.Query[ActSubscriptionNote](db).Create(ctx, note)

		hookCalled := false
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActSubscriber](site, admin.ModelAdmin{
				DeleteQueryset: func(r *http.Request, act *admin.ActionContext) error {
					hookCalled = true
					_, err := act.Delete(r.Context())
					return err
				},
			})
			admin.Register[ActSubscriptionNote](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(super)
		clURL := mustReverse(t, env.router, "admin:admin_views_actsubscriber_changelist")

		// Nonexistent PK shows empty confirmation list.
		resp := env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {"9998"},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		})
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertTemplateUsed(t, resp, "admin/delete_selected_confirmation.html")
		djangotest.AssertContains(t, resp, "Are you sure you want to delete the selected subscribers?")

		// Confirmation page for s1 and s2 shows summary including cascaded subscription notes.
		s1ID := strconv.FormatInt(s1.ID, 10)
		s2ID := strconv.FormatInt(s2.ID, 10)
		confirm := env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {s1ID, s2ID},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		})
		djangotest.AssertStatus(t, confirm, http.StatusOK)
		djangotest.AssertTemplateUsed(t, confirm, "admin/delete_selected_confirmation.html")
		djangotest.AssertContains(t, confirm, "Are you sure you want to delete the selected subscribers?")
		djangotest.AssertContains(t, confirm, "<h1>Delete multiple objects</h1>")
		djangotest.AssertContains(t, confirm, "<h2>Summary</h2>")
		djangotest.AssertContains(t, confirm, "<li>Subscribers: 2</li>")
		djangotest.AssertContains(t, confirm, "<li>Subscription notes: 1</li>")
		djangotest.AssertContains(t, confirm, admin.ActionCheckboxName, djangotest.Count(2))
		djangotest.AssertContains(t, confirm, `value="Yes, I’m sure"`)

		// Confirm deletion with post=yes.
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {s1ID, s2ID},
			"action":                 {"delete_selected"},
			"post":                   {"yes"},
		}, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, clURL)
		djangotest.AssertEqual(t, hookCalled, true)
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelSuccess, Message: "Successfully deleted 2 subscribers."},
		})
		cnt, _ := orm.Query[ActSubscriber](db).Count(ctx)
		djangotest.AssertEqual(t, cnt, int64(0))
		noteCnt, _ := orm.Query[ActSubscriptionNote](db).Count(ctx)
		djangotest.AssertEqual(t, noteCnt, int64(0))
	})
}

// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_model_admin_default_delete_action_protected
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_delete_protected_message_lists_nothing_when_limit_zero
func TestDefaultDeleteSelectedProtected(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		q1 := &ActQuestion{Question: "Why?"}
		q2 := &ActQuestion{Question: "Wherefore?"}
		_ = orm.Query[ActQuestion](db).Create(ctx, q1)
		_ = orm.Query[ActQuestion](db).Create(ctx, q2)
		a1 := &ActAnswer{Answer: "Because."}
		a1.Question.SetPK(q1.ID)
		a2 := &ActAnswer{Answer: "Yes."}
		a2.Question.SetPK(q1.ID)
		_ = orm.Query[ActAnswer](db).Create(ctx, a1)
		_ = orm.Query[ActAnswer](db).Create(ctx, a2)

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActQuestion](site, admin.ModelAdmin{})
			admin.Register[ActAnswer](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(super)
		qURL := mustReverse(t, env.router, "admin:admin_views_actquestion_changelist")
		a1Change := mustReverse(t, env.router, "admin:admin_views_actanswer_change", a1.ID)
		a2Change := mustReverse(t, env.router, "admin:admin_views_actanswer_change", a2.ID)

		data := url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(q1.ID, 10), strconv.FormatInt(q2.ID, 10)},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		}
		resp := env.client.Post(qURL, data)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, "would require deleting the following protected related objects")
		djangotest.AssertContains(t, resp, `<li>Answer: <a href="`+a1Change+`">Because.</a></li>`, djangotest.HTML())
		djangotest.AssertContains(t, resp, `<li>Answer: <a href="`+a2Change+`">Yes.</a></li>`, djangotest.HTML())

		// Confirming POST with post=yes still refuses deletion when protected objects exist.
		data.Set("post", "yes")
		resp = env.client.Post(qURL, data)
		djangotest.AssertStatus(t, resp, http.StatusOK)
		djangotest.AssertContains(t, resp, "would require deleting the following protected related objects")
		cnt, _ := orm.Query[ActQuestion](db).Count(ctx)
		djangotest.AssertEqual(t, cnt, int64(2))

		// Limit 0 hides protected list and uses the shorter message.
		zero := 0
		envZero := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActQuestion](site, admin.ModelAdmin{
				DeleteConfirmationMaxDisplay: &zero,
			})
			admin.Register[ActAnswer](site, admin.ModelAdmin{})
		})
		envZero.client.ForceLogin(super)
		resp = envZero.client.Post(mustReverse(t, envZero.router, "admin:admin_views_actquestion_changelist"), url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(q1.ID, 10)},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		})
		djangotest.AssertContains(t, resp, "Deleting the selected question would require deleting some protected related objects.")
		djangotest.AssertNotContains(t, resp, "Because.")
	})
}

// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_delete_selected_uses_get_deleted_objects
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_delete_selected_uses_delete_confirmation_max_display
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_delete_selected_hides_objects_when_limit_zero
func TestDeleteSelectedMaxDisplayAndHook(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		book := &ActBook{Name: "Test Book"}
		_ = orm.Query[ActBook](db).Create(ctx, book)

		customDeleted := func(*http.Request, []any) (*admin.DeletedObjects, error) {
			return &admin.DeletedObjects{
				Items:      []string{"a deletable object", "another object", "last object"},
				ModelCount: []admin.DeletedModelCount{{Name: "books", Count: 1}},
			}, nil
		}

		one := 1
		envOne := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActBook](site, admin.ModelAdmin{
				DeleteConfirmationMaxDisplay: &one,
				GetDeletedObjects:            customDeleted,
			})
		})
		envOne.client.ForceLogin(super)
		resp := envOne.client.Post(mustReverse(t, envOne.router, "admin:admin_views_actbook_changelist"), url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(book.ID, 10)},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		})
		djangotest.AssertContains(t, resp, "a deletable object")
		djangotest.AssertContains(t, resp, "…and 2 more objects.")
		djangotest.AssertNotContains(t, resp, "another object")
		djangotest.AssertNotContains(t, resp, "last object")

		zero := 0
		envZero := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActBook](site, admin.ModelAdmin{
				DeleteConfirmationMaxDisplay: &zero,
				GetDeletedObjects:            customDeleted,
			})
		})
		envZero.client.ForceLogin(super)
		resp = envZero.client.Post(mustReverse(t, envZero.router, "admin:admin_views_actbook_changelist"), url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(book.ID, 10)},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		})
		djangotest.AssertContains(t, resp, "<h2>Summary</h2>", djangotest.HTML())
		djangotest.AssertNotContains(t, resp, "<h2>Objects</h2>", djangotest.HTML())
		djangotest.AssertNotContains(t, resp, "a deletable object")
	})
}

// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_model_without_action
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_action_column_class
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_user_message_on_none_selected
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_user_message_on_no_action
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_selection_counter
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsTest.test_popup_actions
func TestActionsUIValidationAndPopups(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		s1 := &ActSubscriber{Name: "John Doe", Email: "john@example.org"}
		s2 := &ActSubscriber{Name: "Max Mustermann", Email: "max@example.org"}
		_ = orm.Query[ActSubscriber](db).Create(ctx, s1)
		_ = orm.Query[ActSubscriber](db).Create(ctx, s2)
		_ = orm.Query[ActOldSubscriber](db).Create(ctx, &ActOldSubscriber{Name: "Legacy"})

		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActSubscriber](site, admin.ModelAdmin{})
			admin.Register[ActOldSubscriber](site, admin.ModelAdmin{
				DisableActions: true,
			})
		})
		env.client.ForceLogin(super)

		// Model without actions has no checkboxes or action bar.
		oldResp := env.client.Get(mustReverse(t, env.router, "admin:admin_views_actoldsubscriber_changelist"))
		djangotest.AssertEqual(t, ctxVal(t, oldResp, "has_actions"), any(false))
		djangotest.AssertNotContains(t, oldResp, `class="action-select"`)
		djangotest.AssertNotContains(t, oldResp, `id="action-toggle"`)

		// Model with actions has action-checkbox-column and "0 of 2 selected".
		subURL := mustReverse(t, env.router, "admin:admin_views_actsubscriber_changelist")
		subResp := env.client.Get(subURL)
		djangotest.AssertEqual(t, ctxVal(t, subResp, "has_actions"), any(true))
		djangotest.AssertContains(t, subResp, "action-checkbox-column")
		djangotest.AssertContains(t, subResp, "0 of 2 selected")

		// Popup hides actions.
		popupResp := env.client.Get(subURL, djangotest.Query(url.Values{"_popup": {"1"}}))
		djangotest.AssertEqual(t, ctxVal(t, popupResp, "has_actions"), any(false))
		djangotest.AssertNotContains(t, popupResp, `class="actions"`)

		// Warning when no items are selected.
		resp := env.client.Post(subURL, url.Values{
			"action": {"delete_selected"},
			"index":  {"0"},
		}, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, subURL)
		djangotest.AssertContains(t, resp, "Items must be selected in order to perform actions on them. No items have been changed.")
		cnt, _ := orm.Query[ActSubscriber](db).Count(ctx)
		djangotest.AssertEqual(t, cnt, int64(2))

		// Warning when no action is selected.
		resp = env.client.Post(subURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10), strconv.FormatInt(s2.ID, 10)},
			"action":                 {""},
			"index":                  {"0"},
		}, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, subURL)
		djangotest.AssertContains(t, resp, "No action selected.")
		cnt, _ = orm.Query[ActSubscriber](db).Count(ctx)
		djangotest.AssertEqual(t, cnt, int64(2))
	})
}

// Ported from Django: tests/admin_views/test_actions.py::AdminActionsPermissionTests.test_model_admin_no_delete_permission
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsPermissionTests.test_model_admin_no_delete_permission_externalsubscriber
// Ported from Django: tests/admin_views/test_actions.py::AdminActionsPermissionTests.test_minimum_permission
func TestActionPermissions(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		s1 := &ActSubscriber{Name: "John Doe", Email: "john@example.org"}
		s2 := &ActSubscriber{Name: "Max Mustermann", Email: "max@example.org"}
		_ = orm.Query[ActSubscriber](db).Create(ctx, s1)
		_ = orm.Query[ActSubscriber](db).Create(ctx, s2)
		note := &ActSubscriptionNote{Body: "Child note"}
		note.Subscriber.SetPK(s1.ID)
		_ = orm.Query[ActSubscriptionNote](db).Create(ctx, note)

		ranCustom := 0
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActSubscriber](site, admin.ModelAdmin{
				Actions: []admin.Action{
					{
						Name:        "mail_admin",
						Description: "Mail admin",
						Handler: func(_ http.ResponseWriter, _ *http.Request, _ *admin.ActionContext) (bool, error) {
							ranCustom++
							return false, nil
						},
					},
					{
						Name:        "restricted_change_action",
						Description: "Restricted change action",
						Permissions: []string{"change"},
						Handler: func(_ http.ResponseWriter, _ *http.Request, _ *admin.ActionContext) (bool, error) {
							ranCustom++
							return false, nil
						},
					},
				},
			})
			admin.Register[ActSubscriptionNote](site, admin.ModelAdmin{})
		})

		user := createTestUser(t, db, "staffuser", "secret", true, false)
		grantPerm(t, db, user, "admin_views", "view_actsubscriber")
		env.client.ForceLogin(user)

		clURL := mustReverse(t, env.router, "admin:admin_views_actsubscriber_changelist")

		// View-only user sees mail_admin, NOT delete_selected and NOT restricted_change_action.
		resp := env.client.Get(clURL)
		djangotest.AssertContains(t, resp, `<option value="mail_admin">Mail admin</option>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `value="delete_selected"`)
		djangotest.AssertNotContains(t, resp, `value="restricted_change_action"`)

		// View-only user can execute permissionless custom action.
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10)},
			"action":                 {"mail_admin"},
			"index":                  {"0"},
		})
		djangotest.AssertRedirects(t, resp, clURL)
		djangotest.AssertEqual(t, ranCustom, 1)

		// User without delete permission cannot execute delete_selected -> "No action selected."
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10)},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		}, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, clURL)
		djangotest.AssertContains(t, resp, "No action selected.")

		// Grant delete_actsubscriber, but NOT delete_actsubscriptionnote:
		// confirmation shows lacking permission and confirmed POST returns 403.
		grantPerm(t, db, user, "admin_views", "delete_actsubscriber")
		confirm := env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10), strconv.FormatInt(s2.ID, 10)},
			"action":                 {"delete_selected"},
			"index":                  {"0"},
		})
		djangotest.AssertStatus(t, confirm, http.StatusOK)
		djangotest.AssertContains(t, confirm, "Cannot delete subscribers")
		djangotest.AssertContains(t, confirm, "subscription note")

		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(s1.ID, 10), strconv.FormatInt(s2.ID, 10)},
			"action":                 {"delete_selected"},
			"post":                   {"yes"},
		})
		djangotest.AssertStatus(t, resp, http.StatusForbidden)
		cnt, _ := orm.Query[ActSubscriber](db).Count(ctx)
		djangotest.AssertEqual(t, cnt, int64(2))
	})
}

// TestSelectAcrossPagesAndWithFilters verifies select_across=1 runs bulk actions
// across every matching page while honoring active changelist search/filters.
func TestSelectAcrossPagesAndWithFilters(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		var alphaIDs []int64
		for i := 1; i <= 5; i++ {
			s := &ActSubscriber{Name: fmt.Sprintf("Alpha %d", i), Email: "alpha@example.org"}
			_ = orm.Query[ActSubscriber](db).Create(ctx, s)
			alphaIDs = append(alphaIDs, s.ID)
		}
		for i := 1; i <= 3; i++ {
			s := &ActSubscriber{Name: fmt.Sprintf("Beta %d", i), Email: "beta@example.org"}
			_ = orm.Query[ActSubscriber](db).Create(ctx, s)
		}

		var seenCount int64
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[ActSubscriber](site, admin.ModelAdmin{
				ListDisplay:  []string{"name", "email"},
				SearchFields: []string{"name"},
				ListPerPage:  2,
				Actions: []admin.Action{
					{
						Name:        "count_selected",
						Description: "Count selected",
						Handler: func(_ http.ResponseWriter, r *http.Request, act *admin.ActionContext) (bool, error) {
							n, err := act.Count(r.Context())
							if err != nil {
								return false, err
							}
							seenCount = n
							return false, nil
						},
					},
				},
			})
		})
		env.client.ForceLogin(super)
		clURL := mustReverse(t, env.router, "admin:admin_views_actsubscriber_changelist")

		// Page 1 shows "0 of 2 selected" and "Select all 8 subscribers".
		resp := env.client.Get(clURL)
		djangotest.AssertContains(t, resp, "0 of 2 selected")
		djangotest.AssertContains(t, resp, "Select all 8 subscribers")
		djangotest.AssertContains(t, resp, "All 8 selected")

		// Select across all 8 rows via custom action.
		resp = env.client.Post(clURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(alphaIDs[0], 10)},
			"action":                 {"count_selected"},
			"select_across":          {"1"},
			"index":                  {"0"},
		})
		djangotest.AssertRedirects(t, resp, clURL)
		djangotest.AssertEqual(t, seenCount, int64(8))

		// Select across with active search (?q=Alpha) selects only the 5 Alpha rows.
		filteredURL := clURL + "?q=Alpha"
		confirm := env.client.Post(filteredURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(alphaIDs[0], 10)},
			"action":                 {"delete_selected"},
			"select_across":          {"1"},
			"index":                  {"0"},
		})
		djangotest.AssertStatus(t, confirm, http.StatusOK)
		djangotest.AssertContains(t, confirm, "<li>Subscribers: 5</li>")
		djangotest.AssertContains(t, confirm, admin.ActionCheckboxName, djangotest.Count(5))

		// Confirm delete_selected with the 5 Alpha IDs deletes only Alpha rows, leaving 3 Beta rows.
		confirmIDs := make([]string, len(alphaIDs))
		for i, id := range alphaIDs {
			confirmIDs[i] = strconv.FormatInt(id, 10)
		}
		resp = env.client.Post(filteredURL, url.Values{
			admin.ActionCheckboxName: confirmIDs,
			"action":                 {"delete_selected"},
			"post":                   {"yes"},
		}, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, filteredURL)
		djangotest.AssertMessages(t, resp, []messages.Message{
			{Level: messages.LevelSuccess, Message: "Successfully deleted 5 subscribers."},
		})
		remaining, _ := orm.Query[ActSubscriber](db).Count(ctx)
		djangotest.AssertEqual(t, remaining, int64(3))
	})
}

// TestSiteActionsAndCustomPermissions verifies AdminSite.AddAction,
// AdminSite.DisableAction, ModelAdmin.GetActions, and custom action permissions.
func TestSiteActionsAndCustomPermissions(t *testing.T) {
	djangotest.EachDB(t, adminActionModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		super := createTestUser(t, db, "super", "secret", true, true)
		staff := createTestUser(t, db, "staff", "secret", true, false)
		b := &ActBook{Name: "Go Book"}
		_ = orm.Query[ActBook](db).Create(ctx, b)

		siteActionRan := false
		customActionRan := false
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.DisableAction("delete_selected")
			site.AddAction(admin.Action{
				Name:        "site_ping",
				Description: "Ping %(verbose_name_plural)s",
				Handler: func(_ http.ResponseWriter, _ *http.Request, _ *admin.ActionContext) (bool, error) {
					siteActionRan = true
					return false, nil
				},
			})
			admin.Register[ActBook](site, admin.ModelAdmin{
				Actions: []admin.Action{
					{
						Name:        "publish_books",
						Description: "Publish selected %(verbose_name_plural)s",
						Permissions: []string{"publish"},
						Handler: func(_ http.ResponseWriter, _ *http.Request, _ *admin.ActionContext) (bool, error) {
							customActionRan = true
							return false, nil
						},
					},
				},
				CustomPermissions: map[string]func(*http.Request) bool{
					"publish": func(r *http.Request) bool {
						u, _ := auth.GetUser(r)
						return u != nil && u.IsSuperuser
					},
				},
			})
		})
		grantPerm(t, db, staff, "admin_views", "view_actbook")

		bookURL := mustReverse(t, env.router, "admin:admin_views_actbook_changelist")

		// Staff user sees site_ping (not delete_selected, not publish_books).
		env.client.ForceLogin(staff)
		resp := env.client.Get(bookURL)
		djangotest.AssertContains(t, resp, `<option value="site_ping">Ping books</option>`, djangotest.HTML())
		djangotest.AssertNotContains(t, resp, `value="delete_selected"`)
		djangotest.AssertNotContains(t, resp, `value="publish_books"`)

		// Staff user runs site_ping successfully.
		resp = env.client.Post(bookURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(b.ID, 10)},
			"action":                 {"site_ping"},
			"index":                  {"0"},
		})
		djangotest.AssertRedirects(t, resp, bookURL)
		djangotest.AssertEqual(t, siteActionRan, true)

		// Superuser sees and runs publish_books.
		env.client.ForceLogin(super)
		resp = env.client.Get(bookURL)
		djangotest.AssertContains(t, resp, `<option value="publish_books">Publish selected books</option>`, djangotest.HTML())
		resp = env.client.Post(bookURL, url.Values{
			admin.ActionCheckboxName: {strconv.FormatInt(b.ID, 10)},
			"action":                 {"publish_books"},
			"index":                  {"0"},
		})
		djangotest.AssertRedirects(t, resp, bookURL)
		djangotest.AssertEqual(t, customActionRan, true)
	})
}
