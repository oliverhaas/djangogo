package admin_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// State is Django's admin_views.State.
type State struct {
	ID   int64
	Name string `orm:"max_length=100;verbose_name=State verbose_name"`
}

// City is Django's admin_views.City.
type City struct {
	ID    int64
	State orm.FK[State] `orm:"on_delete=cascade"`
	Name  string        `orm:"max_length=100;verbose_name=City verbose_name"`
}

// Ported from Django: tests/admin_views/test_history_view.py::AdminHistoryViewTests.test_changed_message_uses_form_labels
//
// Django's StateAdminForm declares nolabel_form_field and the label of name in
// Meta.labels; the Go admin sets them with FormOptions. Its changed_data also
// adds the name "not_a_form_field" to a changed form. The admin has no form
// class to override, so the message lists only the fields of the form.
func TestHistoryChangedMessageUsesFormLabels(t *testing.T) {
	models := []any{auth.App{}, contenttypes.App{}, admin.App{}, djangotest.App("admin_views", &State{}, &City{})}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[State](site, admin.ModelAdmin{
				FormOptions: []forms.ModelFormOption{
					forms.WithFormField("nolabel_form_field", forms.NewBooleanField(forms.Required(false))),
					forms.WithLabel("name", "State name (from form’s Meta.labels)"),
				},
				Inlines: []admin.Inline{admin.TabularInline[State, City](admin.InlineModelAdmin{})},
			})
			admin.Register[City](site, admin.ModelAdmin{})
		})
		env.client.ForceLogin(createTestUser(t, db, "super", "secret", true, true))

		state := mustCreate(t, db, &State{Name: "My State Name"})
		city := &City{Name: "My City Name"}
		city.State.SetPK(state.ID)
		mustCreate(t, db, city)
		changeDict := map[string]string{
			"name":                   "My State Name 2",
			"nolabel_form_field":     "True",
			"city_set-0-name":        "My City name 2",
			"city_set-0-id":          fmt.Sprint(city.ID),
			"city_set-TOTAL_FORMS":   "3",
			"city_set-INITIAL_FORMS": "1",
			"city_set-MAX_NUM_FORMS": "0",
		}
		stateChangeURL := mustReverse(t, env.router, "admin:admin_views_state_change", state.ID)
		env.client.Post(stateChangeURL, changeDict)
		logentry := latestLogEntry(t, db, "state")
		djangotest.AssertEqual(t, logentry.GetChangeMessage(),
			"Changed State name (from form’s Meta.labels) and nolabel_form_field. "+
				fmt.Sprintf("Changed City verbose_name for city “City object (%d)”.", city.ID))
	})
}

// Django drives a browser through the pages; the Go test reads the HTML.
// Django logs the entries one by one; the Go test inserts them at once, a
// minute apart, so that their order is fixed.
//
// Ported from Django: tests/admin_views/test_history_view.py::PlaywrightTests.test_pagination
func TestHistoryPagination(t *testing.T) {
	models := []any{auth.App{}, contenttypes.App{}, admin.App{}}
	djangotest.EachDB(t, models, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		env := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			admin.Register[auth.User](site, admin.UserAdmin(db))
		})
		superuser := createTestUser(t, db, "super", "secret", true, true)
		env.client.ForceLogin(superuser)
		userModel, _ := db.Registry().ModelOf(&auth.User{})
		userCT, err := contenttypes.GetForModel(ctx, db, userModel)
		if err != nil {
			t.Fatalf("GetForModel: %v", err)
		}
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		objectID := fmt.Sprint(superuser.ID)
		entries := make([]*admin.LogEntry, 0, 1100)
		for i := 1; i <= 1100; i++ {
			e := &admin.LogEntry{
				ActionTime:    start.Add(time.Duration(i) * time.Minute),
				ObjectID:      &objectID,
				ObjectRepr:    "super",
				ActionFlag:    admin.Change,
				ChangeMessage: fmt.Sprintf("Changed something %d", i),
			}
			e.User.SetPK(superuser.ID)
			e.ContentType.SetPK(userCT.ID)
			entries = append(entries, e)
		}
		if err := orm.Query[admin.LogEntry](db).BulkCreate(ctx, entries); err != nil {
			t.Fatalf("BulkCreate: %v", err)
		}

		userHistoryURL := mustReverse(t, env.router, "admin:auth_user_history", superuser.ID)
		resp := env.client.Get(userHistoryURL)
		djangotest.AssertContains(t, resp, `<nav class="paginator" aria-labelledby="pagination">`)
		djangotest.AssertContains(t, resp,
			`<h2 id="pagination" class="visually-hidden">Pagination user entries</h2>`, djangotest.HTML())
		_, paginator, _ := strings.Cut(resp.Content(), `class="paginator"`)
		paginator, _, _ = strings.Cut(paginator, "</nav>")
		if n := strings.Count(paginator, "aria-current"); n != 1 {
			t.Errorf("aria-current occurs %d times in the paginator, want 1", n)
		}
		djangotest.AssertContains(t, resp, `<a role="button" href="" aria-current="page">1</a>`, djangotest.HTML())
		for _, want := range []string{"1100 entries", "…"} {
			if !strings.Contains(paginator, want) {
				t.Errorf("the paginator lacks %q:\n%s", want, paginator)
			}
		}
		// The last page.
		_, list, _ := strings.Cut(paginator, "<ul>")
		list, _, _ = strings.Cut(list, "</ul>")
		items := strings.Split(strings.TrimSpace(list), "<li>")
		if last := items[len(items)-1]; !strings.Contains(last, `href="?p=11">11</a>`) {
			t.Errorf("the last page link is %q, want page 11", last)
		}
		// Select the second page.
		if second := items[2]; !strings.Contains(second, `href="?p=2">2</a>`) {
			t.Errorf("the second page link is %q, want page 2", second)
		}
		resp = env.client.Get(userHistoryURL + "?p=2")
		_, body, _ := strings.Cut(resp.Content(), "<tbody>")
		body, _, _ = strings.Cut(body, "</tbody>")
		rows := strings.Split(body, "<tr>")[1:]
		if len(rows) != 100 {
			t.Fatalf("page 2 has %d rows, want 100", len(rows))
		}
		if !strings.Contains(rows[0], "Changed something 101") {
			t.Errorf("the first row of page 2 is %q, want entry 101", rows[0])
		}
		if !strings.Contains(rows[len(rows)-1], "Changed something 200") {
			t.Errorf("the last row of page 2 is %q, want entry 200", rows[len(rows)-1])
		}
	})
}
