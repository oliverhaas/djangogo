package admin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/admin"
	"github.com/oliverhaas/djangogo/auth"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Models ported from Django's tests/admin_utils/models.py. The Go names carry
// a "Log" prefix, so the model names in the URLs are logsite, logarticle and
// logcar. The verbose names are Django's.

// LogSite is Django's admin_utils.Site.
type LogSite struct {
	ID     int64
	Domain string          `orm:"max_length=100"`
	Parent orm.FK[LogSite] `orm:"null;blank;on_delete=cascade;related_name=children"`
}

// Meta gives the model Django's verbose name.
func (LogSite) Meta() orm.Meta { return orm.Meta{VerboseName: "site"} }

// String returns the domain, as Django's Site.__str__ does.
func (s LogSite) String() string { return s.Domain }

// LogArticle is Django's admin_utils.Article.
type LogArticle struct {
	ID      int64
	Site    orm.FK[LogSite] `orm:"on_delete=cascade;related_name=admin_articles"`
	Title   string          `orm:"max_length=100"`
	Hist    string          `orm:"max_length=100;verbose_name=History;help_text=History help text"`
	Created *time.Time      `orm:"null"`
}

// Meta gives the model Django's verbose name.
func (LogArticle) Meta() orm.Meta { return orm.Meta{VerboseName: "article"} }

// String returns the title, as Django's Article.__str__ does.
func (a LogArticle) String() string { return a.Title }

// LogCar is Django's admin_utils.Car without the multi-table inheritance
// from Vehicle, which the ORM does not support.
type LogCar struct {
	ID int64
}

// logEntryModels returns the apps of Django's LogEntryTests.
func logEntryModels() []any {
	return []any{
		auth.App{},
		contenttypes.App{},
		admin.App{},
		djangotest.App("admin_utils", &LogSite{}, &LogArticle{}, &LogCar{}),
	}
}

// logEntryEnv holds the rows of Django's LogEntryTests.setUpTestData.
type logEntryEnv struct {
	*adminTestEnv
	user       *auth.User
	logSite    *LogSite
	a1, a2, a3 *LogArticle
}

// newLogEntryEnv creates the user, the site, three articles and the log
// entry of the change of the first article, registers the admin of
// admin_utils and logs the user in, Django's setUpTestData and setUp.
func newLogEntryEnv(t *testing.T, db *orm.DB) *logEntryEnv {
	t.Helper()
	env := &logEntryEnv{}
	env.user = createTestUser(t, db, "super", "secret", true, true)
	env.logSite = mustCreate(t, db, &LogSite{Domain: "example.org"})
	articles := make([]*LogArticle, 3)
	for i, title := range []string{"Title", "Title 2", "Title 3"} {
		created := time.Date(2008+i, 3, 12, 11, 54, 0, 0, time.UTC)
		a := &LogArticle{Title: title, Created: &created}
		a.Site.SetPK(env.logSite.ID)
		articles[i] = mustCreate(t, db, a)
	}
	env.a1, env.a2, env.a3 = articles[0], articles[1], articles[2]
	if _, err := admin.LogActions(context.Background(), db, env.user.ID, []any{env.a1}, admin.Change, "Changed something"); err != nil {
		t.Fatalf("LogActions: %v", err)
	}
	env.adminTestEnv = newAdminTestEnv(t, db, registerLogEntryAdmin)
	env.client.ForceLogin(env.user)
	return env
}

// registerLogEntryAdmin registers the models as Django's admin_utils/admin.py
// does on its site "admin". ArticleProxy is missing: the ORM has no proxy
// models.
func registerLogEntryAdmin(site *admin.AdminSite) {
	admin.Register[LogArticle](site, admin.ModelAdmin{})
	admin.Register[LogSite](site, admin.ModelAdmin{
		Inlines: []admin.Inline{admin.TabularInline[LogSite, LogArticle](admin.InlineModelAdmin{
			Fields: []any{"title"},
		})},
	})
}

// latestLogEntry returns the newest log entry of the model, Django's
// LogEntry.objects.filter(content_type__model__iexact=model).latest("id").
func latestLogEntry(t *testing.T, db *orm.DB, model string) *admin.LogEntry {
	t.Helper()
	e, err := orm.Query[admin.LogEntry](db).Filter("content_type__model__iexact", model).OrderBy("-id").First(context.Background())
	if err != nil {
		t.Fatalf("latest log entry of %s: %v", model, err)
	}
	return &e
}

// newestLogEntry returns the newest log entry, Django's
// LogEntry.objects.latest("id").
func newestLogEntry(t *testing.T, db *orm.DB) *admin.LogEntry {
	t.Helper()
	e, err := orm.Query[admin.LogEntry](db).OrderBy("-id").First(context.Background())
	if err != nil {
		t.Fatalf("newest log entry: %v", err)
	}
	return &e
}

// onlyLogEntry returns the only log entry of the model, Django's
// LogEntry.objects.get(content_type__model__iexact=model).
func onlyLogEntry(t *testing.T, db *orm.DB, model string) *admin.LogEntry {
	t.Helper()
	e, err := orm.Query[admin.LogEntry](db).SelectRelated("content_type").Get(context.Background(), "content_type__model__iexact", model)
	if err != nil {
		t.Fatalf("log entry of %s: %v", model, err)
	}
	return &e
}

// saveSignals records the save signals of LogEntry, Django's pre_save and
// post_save listeners of LogEntryTests.setUp. The ORM fires them on create
// only, so each post_save stands for Django's created=True.
func saveSignals(t *testing.T) *[]string {
	t.Helper()
	var signals []string
	cancelPre := orm.OnPreSave(func(_ context.Context, l *admin.LogEntry) error {
		signals = append(signals, fmt.Sprintf("pre_save %s", l.ChangeMessage))
		return nil
	})
	cancelPost := orm.OnPostSave(func(_ context.Context, l *admin.LogEntry) error {
		signals = append(signals, fmt.Sprintf("post_save %d %s", l.ID, l.ChangeMessage))
		return nil
	})
	t.Cleanup(cancelPre)
	t.Cleanup(cancelPost)
	return &signals
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_save
func TestLogEntrySave(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		newLogEntryEnv(t, db)
		logentry := onlyLogEntry(t, db, "logarticle")
		actionTime := logentry.ActionTime
		if err := orm.Query[admin.LogEntry](db).Save(context.Background(), logentry); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if !logentry.ActionTime.Equal(actionTime) {
			t.Errorf("action time = %v, want %v", logentry.ActionTime, actionTime)
		}
		stored := onlyLogEntry(t, db, "logarticle")
		if !stored.ActionTime.Equal(actionTime) {
			t.Errorf("stored action time = %v, want %v", stored.ActionTime, actionTime)
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_change_message
//
// No Go counterpart: the French translations of the messages, as the admin
// has no translations.
func TestLogEntryChangeMessage(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		env := newLogEntryEnv(t, db)
		postData := map[string]string{
			"site":      fmt.Sprint(env.logSite.ID),
			"title":     "Changed",
			"hist":      "Some content",
			"created_0": "2008-03-12",
			"created_1": "11:54",
		}
		changeURL := fmt.Sprintf("/admin/admin_utils/logarticle/%d/change/", env.a1.ID)
		resp := env.client.Post(changeURL, postData)
		djangotest.AssertRedirects(t, resp, "/admin/admin_utils/logarticle/")
		logentry := latestLogEntry(t, db, "logarticle")
		if got, want := logentry.GetChangeMessage(), "Changed Title and History."; got != want {
			t.Errorf("change message = %q, want %q", got, want)
		}

		postData["title"] = "New"
		resp = env.client.Post("/admin/admin_utils/logarticle/add/", postData)
		djangotest.AssertRedirects(t, resp, "/admin/admin_utils/logarticle/")
		logentry = latestLogEntry(t, db, "logarticle")
		if got, want := logentry.GetChangeMessage(), "Added."; got != want {
			t.Errorf("change message = %q, want %q", got, want)
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_change_message_not_json
func TestLogEntryChangeMessageNotJSON(t *testing.T) {
	logentry := admin.LogEntry{ChangeMessage: "non-JSON string"}
	if got := logentry.GetChangeMessage(); got != logentry.ChangeMessage {
		t.Errorf("change message = %q, want %q", got, logentry.ChangeMessage)
	}
}

// FIXME-PORT: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_change_message_localized_datetime_input
// needs localized input formats, which the forms do not have.

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_change_message_formsets
//
// Django's inline uses ArticleAdminForm, whose changed_data adds the name
// "not_a_form_field" to a changed form. An inline has no form class to
// override, so the change of the first article lists only Title.
//
// No Go counterpart: the French translations of the messages.
func TestLogEntryChangeMessageFormsets(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		env := newLogEntryEnv(t, db)
		created := time.Date(2012, 3, 18, 11, 54, 0, 0, time.UTC)
		a2 := &LogArticle{Title: "Title second article", Created: &created}
		a2.Site.SetPK(env.logSite.ID)
		mustCreate(t, db, a2)
		site := fmt.Sprint(env.logSite.ID)
		postData := map[string]string{
			"domain":                       "example.com", // domain changed
			"admin_articles-TOTAL_FORMS":   "5",
			"admin_articles-INITIAL_FORMS": "2",
			"admin_articles-MIN_NUM_FORMS": "0",
			"admin_articles-MAX_NUM_FORMS": "1000",
			"admin_articles-0-id":          fmt.Sprint(env.a1.ID),
			"admin_articles-0-site":        site,
			"admin_articles-0-title":       "Changed Title",
			"admin_articles-1-id":          fmt.Sprint(a2.ID),
			"admin_articles-1-site":        site,
			"admin_articles-1-title":       "Title second article",
			"admin_articles-1-DELETE":      "on",
			"admin_articles-2-site":        site,
			"admin_articles-2-title":       "Added article",
		}
		changeURL := fmt.Sprintf("/admin/admin_utils/logsite/%d/change/", env.logSite.ID)
		resp := env.client.Post(changeURL, postData)
		djangotest.AssertRedirects(t, resp, "/admin/admin_utils/logsite/")
		n, err := orm.Query[LogArticle](db).Filter("pk", a2.ID).Count(context.Background())
		if err != nil || n != 0 {
			t.Fatalf("articles with the pk of a2 = %d, %v; want 0", n, err)
		}
		logentry, err := orm.Query[admin.LogEntry](db).Filter("content_type__model__iexact", "logsite").OrderBy("-action_time", "-id").First(context.Background())
		if err != nil {
			t.Fatalf("latest log entry of the site: %v", err)
		}
		var got []any
		if err := json.Unmarshal([]byte(logentry.ChangeMessage), &got); err != nil {
			t.Fatalf("change message %q: %v", logentry.ChangeMessage, err)
		}
		want := []any{
			map[string]any{"changed": map[string]any{"fields": []any{"Domain"}}},
			map[string]any{"added": map[string]any{"object": "Added article", "name": "article"}},
			map[string]any{"changed": map[string]any{
				"fields": []any{"Title"},
				"object": "Changed Title",
				"name":   "article",
			}},
			map[string]any{"deleted": map[string]any{"object": "Title second article", "name": "article"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("change message = %s, want %v", logentry.ChangeMessage, want)
		}
		wantMsg := "Changed Domain. Added article “Added article”. " +
			"Changed Title for article “Changed Title”. " +
			"Deleted article “Title second article”."
		if got := logentry.GetChangeMessage(); got != wantMsg {
			t.Errorf("GetChangeMessage = %q, want %q", got, wantMsg)
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_get_edited_object
func TestLogEntryGetEditedObject(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		newLogEntryEnv(t, db)
		logentry := onlyLogEntry(t, db, "logarticle")
		edited, err := logentry.GetEditedObject(context.Background(), db)
		if err != nil {
			t.Fatalf("GetEditedObject: %v", err)
		}
		a, ok := edited.(*LogArticle)
		if !ok {
			t.Fatalf("edited object = %T, want *LogArticle", edited)
		}
		if logentry.ObjectID == nil || *logentry.ObjectID != fmt.Sprint(a.ID) {
			t.Errorf("object id = %v, want %d", logentry.ObjectID, a.ID)
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_get_admin_url
func TestLogEntryGetAdminURL(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		env := newLogEntryEnv(t, db)
		ctx := context.Background()
		logentry := onlyLogEntry(t, db, "logarticle")
		expected := fmt.Sprintf("/admin/admin_utils/logarticle/%d/change/", env.a1.ID)
		got, err := logentry.GetAdminURL(ctx, env.site)
		if err != nil {
			t.Fatalf("GetAdminURL: %v", err)
		}
		if got != expected {
			t.Errorf("GetAdminURL = %q, want %q", got, expected)
		}
		if part := fmt.Sprintf("logarticle/%d/change/", env.a1.ID); !strings.Contains(got, part) {
			t.Errorf("GetAdminURL = %q, want it to contain %q", got, part)
		}

		ct, ok := logentry.ContentType.Object()
		if !ok || ct == nil {
			t.Fatal("the content type of the log entry is not loaded")
		}
		ct.Model = "nonexistent"
		if got, err := logentry.GetAdminURL(ctx, env.site); err != nil || got != "" {
			t.Errorf("GetAdminURL of a nonexistent model = %q, %v; want \"\"", got, err)
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_unicode
func TestLogEntryUnicode(t *testing.T) {
	var logEntry admin.LogEntry

	logEntry.ActionFlag = admin.Addition
	if s := logEntry.String(); !strings.HasPrefix(s, "Added ") {
		t.Errorf("String() = %q, want the prefix %q", s, "Added ")
	}

	logEntry.ActionFlag = admin.Change
	if s := logEntry.String(); !strings.HasPrefix(s, "Changed ") {
		t.Errorf("String() = %q, want the prefix %q", s, "Changed ")
	}

	logEntry.ActionFlag = admin.Deletion
	if s := logEntry.String(); !strings.HasPrefix(s, "Deleted ") {
		t.Errorf("String() = %q, want the prefix %q", s, "Deleted ")
	}

	// Make sure custom action flags work.
	logEntry.ActionFlag = 4
	if s := logEntry.String(); s != "LogEntry Object" {
		t.Errorf("String() = %q, want %q", s, "LogEntry Object")
	}
}

// No Go counterpart: tests/admin_utils/test_logentry.py::LogEntryTests.test_logentry_repr
// tests Python's repr.

// logRow is the values_list of Django's test_log_actions.
type logRow struct {
	user, contentType    int64
	objectID, objectRepr string
	actionFlag           int16
	changeMessage        string
}

// rowOf returns the logRow of l.
func rowOf(l *admin.LogEntry) logRow {
	id := ""
	if l.ObjectID != nil {
		id = *l.ObjectID
	}
	return logRow{l.User.PK(), l.ContentType.PK(), id, l.ObjectRepr, l.ActionFlag, l.ChangeMessage}
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_log_actions
func TestLogActions(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		env := newLogEntryEnv(t, db)
		signals := saveSignals(t)
		ctx := context.Background()
		queryset, err := orm.Query[LogArticle](db).OrderBy("-id").All(ctx)
		if err != nil {
			t.Fatalf("articles: %v", err)
		}
		const msg = "Deleted Something"
		m, _ := db.Registry().ModelOf(&LogArticle{})
		contentType, err := contenttypes.GetForModel(ctx, db, m)
		if err != nil {
			t.Fatalf("GetForModel: %v", err)
		}
		if len(queryset) != 3 {
			t.Fatalf("articles = %d, want 3", len(queryset))
		}
		objs := make([]any, len(queryset))
		for i := range queryset {
			objs[i] = &queryset[i]
		}
		var result []*admin.LogEntry
		djangotest.AssertNumQueries(t, db, 1, func() {
			result, err = admin.LogActions(ctx, db, env.user.ID, objs, admin.Deletion, msg)
		})
		if err != nil {
			t.Fatalf("LogActions: %v", err)
		}
		if len(result) != len(queryset) {
			t.Fatalf("entries = %d, want %d", len(result), len(queryset))
		}
		stored, err := orm.Query[admin.LogEntry](db).Filter("action_flag", admin.Deletion).OrderBy("id").All(ctx)
		if err != nil {
			t.Fatalf("log entries: %v", err)
		}
		logs := make([]logRow, len(stored))
		for i := range stored {
			logs[i] = rowOf(&stored[i])
		}
		expected := make([]logRow, len(queryset))
		for i, a := range queryset {
			expected[i] = logRow{env.user.ID, contentType.ID, fmt.Sprint(a.ID), a.String(), admin.Deletion, msg}
		}
		resultLogs := make([]logRow, len(result))
		for i, l := range result {
			resultLogs[i] = rowOf(l)
		}
		if !reflect.DeepEqual(logs, resultLogs) {
			t.Errorf("stored entries = %v, want the result %v", logs, resultLogs)
		}
		if !reflect.DeepEqual(logs, expected) {
			t.Errorf("stored entries = %v, want %v", logs, expected)
		}
		if len(*signals) != 0 {
			t.Errorf("signals = %v, want none", *signals)
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_log_actions_single_object_param
//
// No Go counterpart: single_object=True, as LogActions always returns a
// slice. The test checks the call with single_object=False.
func TestLogActionsSingleObjectParam(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		env := newLogEntryEnv(t, db)
		signals := saveSignals(t)
		ctx := context.Background()
		queryset, err := orm.Query[LogArticle](db).Filter("pk", env.a1.ID).All(ctx)
		if err != nil {
			t.Fatalf("articles: %v", err)
		}
		const msg = "Deleted Something"
		m, _ := db.Registry().ModelOf(&LogArticle{})
		contentType, err := contenttypes.GetForModel(ctx, db, m)
		if err != nil {
			t.Fatalf("GetForModel: %v", err)
		}
		if len(queryset) != 1 {
			t.Fatalf("articles = %d, want 1", len(queryset))
		}
		var result []*admin.LogEntry
		djangotest.AssertNumQueries(t, db, 1, func() {
			result, err = admin.LogActions(ctx, db, env.user.ID, []any{&queryset[0]}, admin.Deletion, msg)
		})
		if err != nil {
			t.Fatalf("LogActions: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("entries = %d, want 1", len(result))
		}
		entry := result[0]
		want := logRow{env.user.ID, contentType.ID, fmt.Sprint(env.a1.ID), env.a1.String(), admin.Deletion, msg}
		if got := rowOf(entry); got != want {
			t.Errorf("entry = %v, want %v", got, want)
		}
		wantSignals := []string{"pre_save " + msg, fmt.Sprintf("post_save %d %s", entry.ID, msg)}
		if !reflect.DeepEqual(*signals, wantSignals) {
			t.Errorf("signals = %v, want %v", *signals, wantSignals)
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_recentactions_without_content_type
func TestRecentActionsWithoutContentType(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		env := newLogEntryEnv(t, db)
		resp := env.client.Get("/admin/")
		link := fmt.Sprintf("/admin/admin_utils/logarticle/%d/change/", env.a1.ID)
		djangotest.AssertContains(t, resp, fmt.Sprintf(`<a href="%s">%s</a>`, link, env.a1.Title))
		const shouldContain = "Article"
		djangotest.AssertContains(t, resp, shouldContain)
		logentry := onlyLogEntry(t, db, "logarticle")
		// If the log entry doesn't have a content type it should still be
		// possible to view the Recent Actions part (#10275).
		logentry.ContentType.SetPK(0)
		if err := orm.Query[admin.LogEntry](db).Save(context.Background(), logentry); err != nil {
			t.Fatalf("Save: %v", err)
		}

		before := strings.Count(resp.Content(), shouldContain)
		resp = env.client.Get("/admin/")
		after := strings.Count(resp.Content(), shouldContain)
		if before-1 != after {
			t.Errorf("%q occurs %d times, want %d", shouldContain, after, before-1)
		}
	})
}

// FIXME-PORT: tests/admin_utils/test_logentry.py::LogEntryTests.test_proxy_model_content_type_is_used_for_log_entries
// needs proxy models, which the ORM does not have.

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_action_flag_choices
func TestLogEntryActionFlagChoices(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		m, ok := db.Registry().ModelOf(&admin.LogEntry{})
		if !ok {
			t.Fatal("LogEntry is not registered")
		}
		tests := []struct {
			actionFlag  int16
			displayName string
		}{{1, "Addition"}, {2, "Change"}, {3, "Deletion"}}
		for _, tt := range tests {
			t.Run(fmt.Sprintf("action_flag=%d", tt.actionFlag), func(t *testing.T) {
				log := &admin.LogEntry{ActionFlag: tt.actionFlag}
				if got := orm.GetDisplay(m, log, "action_flag"); got != tt.displayName {
					t.Errorf("get_action_flag_display() = %v, want %q", got, tt.displayName)
				}
			})
		}
	})
}

// Ported from Django: tests/admin_utils/test_logentry.py::LogEntryTests.test_hook_get_log_entries
func TestHookGetLogEntries(t *testing.T) {
	djangotest.EachDB(t, logEntryModels(), func(t *testing.T, db *orm.DB) {
		env := newLogEntryEnv(t, db)
		ctx := context.Background()
		if _, err := admin.LogActions(ctx, db, env.user.ID, []any{env.a1}, admin.Change, "Article changed message"); err != nil {
			t.Fatalf("LogActions: %v", err)
		}
		c1 := mustCreate(t, db, &LogCar{})
		if _, err := admin.LogActions(ctx, db, env.user.ID, []any{c1}, admin.Addition, "Car created message"); err != nil {
			t.Fatalf("LogActions: %v", err)
		}
		carModel, _ := db.Registry().ModelOf(c1)
		expStrArticle := env.a1.Title
		expStrCar := orm.Label(carModel, c1)

		resp := env.client.Get("/admin/")
		djangotest.AssertContains(t, resp, expStrArticle)
		djangotest.AssertContains(t, resp, expStrCar)

		// The site "custom_admin" only renders the log entries of its
		// registered models.
		articleModel, _ := db.Registry().ModelOf(&LogArticle{})
		articleCT, err := contenttypes.GetForModel(ctx, db, articleModel)
		if err != nil {
			t.Fatalf("GetForModel: %v", err)
		}
		custom := newAdminTestEnv(t, db, func(site *admin.AdminSite) {
			site.GetLogEntries = func(_ *http.Request, entries *orm.QuerySet[admin.LogEntry]) *orm.QuerySet[admin.LogEntry] {
				return entries.Filter("content_type__in", []int64{articleCT.ID})
			}
			admin.Register[LogArticle](site, admin.ModelAdmin{})
		})
		custom.client.ForceLogin(env.user)
		resp = custom.client.Get("/admin/")
		djangotest.AssertContains(t, resp, expStrArticle)
		djangotest.AssertNotContains(t, resp, expStrCar)
	})
}

// FIXME-PORT: tests/admin_views/tests.py::AdminViewStringPrimaryKeyTest.test_get_history_view
// and the other tests of the class need a model with a string primary key,
// which the admin does not support: its URLs take an integer primary key.
// TestChangeViewHistoryLink ports test_change_view_history_link with an
// integer primary key.

// No Go counterpart: tests/admin_views/tests.py::AdminViewBasicTest.test_history_view_custom_qs
// checks that the history view uses ModelAdmin.get_queryset() instead of a
// filtering default manager. The ORM has no custom managers.
