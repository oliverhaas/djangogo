package messages_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/sessions"
	"github.com/oliverhaas/djangogo/templates"
)

// showTemplate is the TEMPLATE of Django's tests/messages_tests/urls.py.
const showTemplate = `{% if messages %}
<ul class="messages">
    {% for message in messages %}
    <li{% if message.tags %} class="{{ message.tags }}"{% endif %}>
        {{ message }}
    </li>
    {% endfor %}
</ul>
{% endif %}
`

// shortcuts maps the level names of the URLs to the shortcut functions, as
// getattr(messages, message_type) in Django's views.
var shortcuts = map[string]func(*http.Request, string, ...string) error{
	"debug":   messages.Debug,
	"info":    messages.Info,
	"success": messages.Success,
	"warning": messages.Warning,
	"error":   messages.Error,
}

// levels maps the level names to their values (BaseTests.levels).
var levels = map[string]int{
	"debug":   messages.LevelDebug,
	"info":    messages.LevelInfo,
	"success": messages.LevelSuccess,
	"warning": messages.LevelWarning,
	"error":   messages.LevelError,
}

// levelNames lists the level names in Django's order.
var levelNames = []string{"debug", "info", "success", "warning", "error"}

// siteOptions selects the setup of the test site.
type siteOptions struct {
	// withoutMessages leaves out the message middleware and the context
	// processor (the modify_settings of test_middleware_disabled).
	withoutMessages bool
}

// newSite returns the handler of Django's messages_tests.urls behind the
// session and message middleware, with the messages context processor. A
// view that cannot add a message panics with the error, as the Django view
// raises MessageFailure.
func newSite(t *testing.T, opts siteOptions) http.Handler {
	t.Helper()
	engine, err := templates.NewEngineFS(fstest.MapFS{"show.html": {Data: []byte(showTemplate)}})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.withoutMessages {
		engine.AddContextProcessor(messages.ContextProcessor)
	}
	add := func(showURL string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_, failSilently := r.PostForm["fail_silently"]
			for _, msg := range r.PostForm["messages"] {
				if err := shortcuts[r.PathValue("level")](r, msg); err != nil && !failSilently {
					panic(err)
				}
			}
			http.Redirect(w, r, showURL, http.StatusFound)
		}
	}
	show := func(w http.ResponseWriter, r *http.Request) {
		out, err := engine.RenderRequest(r, "show.html", nil)
		if err != nil {
			t.Errorf("render: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, out)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /add/{level}/", add("/show/"))
	mux.HandleFunc("GET /show/", show)
	mux.HandleFunc("POST /template_response/add/{level}/", add("/template_response/show/"))
	mux.HandleFunc("GET /template_response/show/", show)

	var h http.Handler = mux
	if !opts.withoutMessages {
		h = messages.Middleware(h)
	}
	return sessions.Middleware(sessions.NewSignedCookieStore([]byte("messages-tests")), sessions.DefaultCookieName)(h)
}

// newClient returns a test client for newSite that records the messages of
// every request.
func newClient(t *testing.T, opts siteOptions) *djangotest.Client {
	t.Helper()
	return djangotest.NewClient(t, newSite(t, opts), djangotest.WithMessages())
}

// withConfig applies cfg for the duration of the test (override_settings of
// MESSAGE_LEVEL and MESSAGE_TAGS).
func withConfig(t *testing.T, cfg messages.Config) {
	t.Helper()
	t.Cleanup(messages.Configure(cfg))
}

// newStorage returns a session storage on a fresh session.
func newStorage() *messages.Storage {
	return messages.NewSessionStorage(sessions.NewSignedCookieStore([]byte("k")).New())
}

// storageWith returns a storage whose loaded messages are data, without
// reading the session (BaseTests.get_storage).
func storageWith(data ...messages.Message) *messages.Storage {
	s := newStorage()
	s.SetLoaded(data)
	return s
}

// contextMessages calls the lazy "messages" value of the last rendered
// context, as list(response.context["messages"]), and returns the messages.
func contextMessages(t *testing.T, resp *djangotest.Response) []messages.Message {
	t.Helper()
	v, ok := resp.Context("messages")
	if !ok {
		t.Fatal(`"messages" is not in the context`)
	}
	fn, ok := v.(func() []messages.TemplateMessage)
	if !ok {
		t.Fatalf(`"messages" is a %T, want func() []messages.TemplateMessage`, v)
	}
	tms := fn()
	out := make([]messages.Message, 0, len(tms))
	for _, m := range tms {
		out = append(out, messages.Message{
			Level:     m["level"].(int),
			Message:   m["message"].(string),
			ExtraTags: m["extra_tags"].(string),
		})
	}
	return out
}

// assertMessagesEqual checks got against want by level and text, as
// assertEqual on lists of Django messages.
func assertMessagesEqual(t *testing.T, got, want []messages.Message) {
	t.Helper()
	equal := len(got) == len(want)
	for i := 0; equal && i < len(got); i++ {
		equal = got[i].Equal(want[i])
	}
	if !equal {
		t.Errorf("messages = %v, want %v", reprsOf(got), reprsOf(want))
	}
}

// reprsOf returns the Django repr of each message.
func reprsOf(ms []messages.Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Repr()
	}
	return out
}

// mustPanic runs f and returns the value it panics with; the test fails when
// f returns normally.
func mustPanic(t *testing.T, f func()) (v any) {
	t.Helper()
	defer func() { v = recover() }()
	f()
	t.Fatal("no panic")
	return nil
}

// storageAndSession returns a storage whose loaded messages are data, as
// storageWith does, together with the session it writes to, so that a test
// can count the stored messages (stored_session_messages_count).
func storageAndSession(data ...messages.Message) (*messages.Storage, *sessions.Session) {
	sess := sessions.NewSignedCookieStore([]byte("k")).New()
	s := messages.NewSessionStorage(sess)
	s.SetLoaded(data)
	return s, sess
}

// storedCount returns the number of messages in the session after an Update
// (BaseTests.stored_messages_count of SessionTests).
func storedCount(sess *sessions.Session) int {
	v, _ := sess.Get(messages.SessionKey)
	return len(messages.DecodeMessages(v))
}

// addLevelMessages adds six messages of different levels, one of them
// custom, to s (add_level_messages of Django's messages_tests/base.py).
func addLevelMessages(s *messages.Storage) {
	s.Add(messages.LevelInfo, "A generic info message", "")
	s.Add(29, "Some custom level", "")
	s.Add(messages.LevelDebug, "A debugging message", "extra-tag")
	s.Add(messages.LevelWarning, "A warning", "")
	s.Add(messages.LevelError, "An error", "")
	s.Add(messages.LevelSuccess, "This was a triumph.", "")
}

// existingMessages are the messages of get_existing_storage.
func existingMessages() []messages.Message {
	return []messages.Message{
		{Level: messages.LevelInfo, Message: "Test message 1"},
		{Level: messages.LevelInfo, Message: "Test message 2", ExtraTags: "tag"},
	}
}

// postData returns the form data of the cycle tests: five messages and
// optionally the fail_silently flag.
func postData(failSilently bool) url.Values {
	data := url.Values{}
	for x := range 5 {
		data.Add("messages", fmt.Sprintf("Test message %d", x))
	}
	if failSilently {
		data.Set("fail_silently", "True")
	}
	return data
}
