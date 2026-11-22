package messages_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
	"github.com/oliverhaas/djangogo/validators"
	"github.com/oliverhaas/djangogo/views"
)

// SomeObject is the SomeObject model of Django's tests/messages_tests/models.py.
type SomeObject struct {
	ID   int64
	Name string `orm:"max_length=255"`
}

// contactSuccessMessage is the success_message of ContactFormViewWithMsg.
const contactSuccessMessage = "%(name)s was created successfully"

// deleteSuccessMessage is the success_message of DeleteFormViewWithMsg.
const deleteSuccessMessage = "Object was deleted successfully"

// newMixinSite returns the views of Django's messages_tests.urls that use
// SuccessMessageMixin, behind the session middleware and the message
// middleware with the fallback storage, Django's default MESSAGE_STORAGE.
func newMixinSite(t *testing.T, db *orm.DB) http.Handler {
	t.Helper()
	engine, err := templates.NewEngineFS(fstest.MapFS{
		"show.html": {Data: []byte(showTemplate)},
		"messages_tests/someobject_confirm_delete.html": {Data: []byte("Delete {{ object.name }}?")},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine.AddContextProcessor(messages.ContextProcessor)
	mux := http.NewServeMux()
	mux.Handle("/add/msg/", views.FormView{
		Engine: engine,
		Form: func(*http.Request) (*forms.Form, error) {
			return forms.New(
				forms.F("name", forms.NewCharField()),
				forms.F("slug", forms.NewSlugField()),
			), nil
		},
		SuccessURL:     "/show/",
		SuccessMessage: contactSuccessMessage,
	})
	mux.Handle("/deletion/{pk}/", views.DeleteView[SomeObject]{
		DB:             db,
		Engine:         engine,
		SuccessURL:     "/show/",
		SuccessMessage: deleteSuccessMessage,
	})
	mux.HandleFunc("GET /show/", func(w http.ResponseWriter, r *http.Request) {
		out, err := engine.RenderRequest(r, "show.html", nil)
		if err != nil {
			t.Errorf("render: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(out))
	})
	h := messages.MiddlewareWithOptions(messages.Options{Storage: messages.StorageFallback, Cookie: testCookieOptions})(mux)
	return sessions.Middleware(sessions.NewSignedCookieStore([]byte("messages-tests")), sessions.DefaultCookieName)(h)
}

// Ported from Django: tests/messages_tests/test_mixins.py::SuccessMessageMixinTests.test_set_messages_success
func TestSuccessMessageMixinTestsSetMessagesSuccess(t *testing.T) {
	author := map[string]string{"name": "John Doe", "slug": "success-msg"}
	c := djangotest.NewClient(t, newMixinSite(t, nil))
	resp := c.Post("/add/msg/", author)
	var value string
	for _, ck := range resp.Cookies() {
		if ck.Name == messages.CookieName {
			value = ck.Value
		}
	}
	// Uncompressed message is stored in the cookie.
	payload, _, _ := strings.Cut(value, ":")
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode %q: %v", value, err)
	}
	want := validators.FormatMessage(contactSuccessMessage, map[string]any{"name": author["name"], "slug": author["slug"]})
	if !strings.Contains(string(decoded), want) {
		t.Errorf("cookie = %q, want it to contain %q", decoded, want)
	}
}

// Ported from Django: tests/messages_tests/test_mixins.py::SuccessMessageMixinTests.test_set_messages_success_on_delete
func TestSuccessMessageMixinTestsSetMessagesSuccessOnDelete(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("messages_tests", &SomeObject{})}, func(t *testing.T, db *orm.DB) {
		objectToDelete := SomeObject{Name: "MyObject"}
		if err := orm.Query[SomeObject](db).Create(context.Background(), &objectToDelete); err != nil {
			t.Fatal(err)
		}
		c := djangotest.NewClient(t, newMixinSite(t, db))
		resp := c.Post("/deletion/"+strconv.FormatInt(objectToDelete.ID, 10)+"/", nil, djangotest.Follow())
		djangotest.AssertContains(t, resp, deleteSuccessMessage)
	})
}
