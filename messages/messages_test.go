package messages_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/sessions"
)

// TestRedirectThenShow adds a message, follows the redirect and checks that
// the page shows it with its tags once, and that the next page does not.
func TestRedirectThenShow(t *testing.T) {
	c := newClient(t, siteOptions{})
	resp := c.Post("/add/success/", url.Values{"messages": {"Saved <b>it</b>"}}, djangotest.Follow())
	djangotest.AssertRedirects(t, resp, "/show/")
	djangotest.AssertContains(t, resp, `<li class="success">`)
	djangotest.AssertContains(t, resp, "Saved &lt;b&gt;it&lt;/b&gt;")
	djangotest.AssertMessages(t, resp, []messages.Message{{Level: messages.LevelSuccess, Message: "Saved <b>it</b>"}})

	resp = c.Get("/show/")
	djangotest.AssertNotContains(t, resp, "Saved")
	djangotest.AssertMessages(t, resp, nil)
}

// TestUnreadMessagesStay checks that a page that does not read the messages
// keeps them in the session for the next one.
func TestUnreadMessagesStay(t *testing.T) {
	c := newClient(t, siteOptions{})
	c.Post("/add/info/", url.Values{"messages": {"one"}})
	c.Post("/add/warning/", url.Values{"messages": {"two"}})
	resp := c.Get("/show/")
	djangotest.AssertMessages(t, resp, []messages.Message{
		{Level: messages.LevelInfo, Message: "one"},
		{Level: messages.LevelWarning, Message: "two"},
	})
	djangotest.AssertContains(t, resp, `<li class="warning">`)
}

// TestSessionFormat checks that the session holds the messages in Django's
// JSON format, so a Django process sharing the session reads them.
func TestSessionFormat(t *testing.T) {
	sess := sessions.NewSignedCookieStore([]byte("k")).New()
	s := messages.NewSessionStorage(sess)
	s.Add(messages.LevelInfo, "héllo \"x\"", "")
	s.Add(messages.LevelError, "bad", "tag1 tag2")
	s.Update(http.Header{})
	v, _ := sess.Get(messages.SessionKey)
	want := `[["__json_message",0,20,"héllo \"x\"",""],["__json_message",0,40,"bad","tag1 tag2"]]`
	if v != want {
		t.Errorf("session value = %v, want %s", v, want)
	}

	// A list that Django wrote, with spaces, a safe flag and no extra tags.
	sess.Set(messages.SessionKey, `[["__json_message", 1, 25, "<b>ok</b>"], ["__json_message", 0, 30, "w", "x"]]`)
	got := messages.NewSessionStorage(sess).Messages()
	wantMsgs := []messages.Message{{Level: 25, Message: "<b>ok</b>", Safe: true}, {Level: 30, Message: "w", ExtraTags: "x"}}
	if len(got) != 2 || got[0] != wantMsgs[0] || got[1] != wantMsgs[1] {
		t.Errorf("decoded %#v, want %#v", got, wantMsgs)
	}
}

// TestCookieFormat checks the messages cookie against values that Django
// 6.0's CookieStorage._encode wrote with SECRET_KEY "messages-tests-secret" at
// the Unix time 1795000000, so a Django process that shares the secret reads
// the cookie. Go's zlib output differs from Python's, so only a value that is
// too short to compress is compared byte for byte.
func TestCookieFormat(t *testing.T) {
	const (
		plain      = "W1siX19qc29uX21lc3NhZ2UiLDAsNDAsIkhpIiwiIl1d:1xTdUu:yuKp-oIhgr-lC-VGNaUcHWA1CjmpsqvDeelqC9jeCKA"
		pair       = ".eJyLjlaKj88qzs-Lz00tLk5MT1XSMdAxMtBR8kjNyclXiCk1MjC0VIhRKs8vykmJUVLSUVKK1cHUY6hjZKqjZJNkl59to59kB1RWkpiuFBsLAJTPHOE:1xTdUu:wh-93u1f6o2S8TZ6XzGnxnNSSUHUW0RbZMKalNNc1m0"
		unfinished = ".eJxly0EKgzAQBdCryF8PNAZcFILr3sHIoJim1pgBJ-L1m4W77t8bBjB_VTLvQXWKAWTIGsIrpCSNP61pn43HJUdaPEDASP-nJdsR3NzL5h5zX1mZYpUV3kazlPeaV_2EhRnjDwdDJsA:1xTdUu:uaYk8ZULVRTwhwuNIaL324E40r6pcULErlSSn32GXq0"
		// oldSecret is plain signed with the SECRET_KEY "old-secret".
		oldSecret = "W1siX19qc29uX21lc3NhZ2UiLDAsNDAsIkhpIiwiIl1d:1xTdUu:84cbDmG0yvwIsbXCo2XgJpZZ9GMokK27H7tSPInChdc"
	)
	opts := messages.CookieOptions{Secret: []byte("messages-tests-secret")}
	hi := []messages.Message{{Level: messages.LevelError, Message: "Hi"}}
	if got := messages.EncodeCookieAt(opts, hi, time.Unix(1795000000, 0)); got != plain {
		t.Errorf("encoded = %s, want %s", got, plain)
	}

	twoMessages := []messages.Message{
		{Level: messages.LevelInfo, Message: "Hello \u2019 \"world\""},
		{Level: messages.LevelSuccess, Message: "<b>ok</b>", ExtraTags: "tag", Safe: true},
	}
	tests := []struct {
		name  string
		value string
		opts  messages.CookieOptions
		want  []messages.Message
		all   bool
	}{
		{"plain", plain, opts, hi, true},
		{"compressed", pair, opts, twoMessages, true},
		{"unfinished", unfinished, opts, twoMessages, false},
		{"fallback secret", oldSecret, messages.CookieOptions{Secret: []byte("new"), Fallbacks: [][]byte{[]byte("old-secret")}}, hi, true},
	}
	for _, tt := range tests {
		ms, all, ok := messages.DecodeCookie(tt.opts, tt.value)
		if !ok || all != tt.all || !slices.Equal(ms, tt.want) {
			t.Errorf("%s: decoded %v, all %v, ok %v; want %v, all %v", tt.name, ms, all, ok, tt.want, tt.all)
		}
	}
	if _, _, ok := messages.DecodeCookie(opts, oldSecret); ok {
		t.Error("a value of another secret decoded")
	}
}

// No Django test exists: a safe message, Django's message of a SafeString,
// is printed as it is, and the text of other messages is escaped.
func TestSafeMessageRendersAsHTML(t *testing.T) {
	// The session marks the safe message with is_safe 1, as Django's
	// MessageEncoder does.
	sess := sessions.NewSignedCookieStore([]byte("k")).New()
	s := messages.NewSessionStorage(sess)
	s.AddSafe(messages.LevelSuccess, "<b>safe</b>", "")
	s.Add(messages.LevelInfo, "<b>plain</b>", "")
	s.Update(http.Header{})
	v, _ := sess.Get(messages.SessionKey)
	if str, _ := v.(string); !strings.HasPrefix(str, `[["__json_message",1,25,`) || !strings.Contains(str, `],["__json_message",0,20,`) {
		t.Errorf("session value = %v, want the safe flag on the first message only", v)
	}

	c := newClient(t, siteOptions{})
	c.Post("/add/info/", url.Values{"messages": {"<b>plain</b>"}})
	resp := c.Post("/add/safe/", url.Values{"messages": {`<a href="/p/1/">Hi</a>`}}, djangotest.Follow())
	djangotest.AssertContains(t, resp, `<a href="/p/1/">Hi</a>`)
	djangotest.AssertContains(t, resp, "&lt;b&gt;plain&lt;/b&gt;")
	djangotest.AssertMessages(t, resp, []messages.Message{
		{Level: messages.LevelInfo, Message: "<b>plain</b>"},
		{Level: messages.LevelSuccess, Message: `<a href="/p/1/">Hi</a>`},
	})
}

// TestDecodeMalformed checks that a session value that is not a list of
// messages yields no messages instead of failing the request.
func TestDecodeMalformed(t *testing.T) {
	for _, v := range []any{nil, 5, "", "{", `{"a":1}`, `[["x",0,20,"m"]]`, `[["__json_message",0,"20","m"]]`, `[["__json_message",0,20]]`} {
		if got := messages.DecodeMessages(v); len(got) != 0 {
			t.Errorf("DecodeMessages(%#v) = %v, want none", v, got)
		}
	}
}

// TestMiddlewareStoresBeforeHeaders checks that a handler that writes its
// response directly still has its messages saved with the session.
func TestMiddlewareStoresBeforeHeaders(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("k"))
	h := sessions.Middleware(store, "sessionid")(messages.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := messages.Info(r, "streamed"); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte("body"))
	})))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v, want the session cookie", cookies)
	}
	sess, err := store.Decode(t.Context(), cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := sess.Get(messages.SessionKey); !strings.Contains(v.(string), "streamed") {
		t.Errorf("session value = %v, want the message", v)
	}
}

// TestAddWithoutMiddleware checks the error and the level functions of a
// request without the middleware.
func TestAddWithoutMiddleware(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	djangotest.AssertErrorIs(t, messages.Info(r, "x"), messages.ErrMiddlewareMissing)
	if messages.Get(r) != nil {
		t.Error("Get without middleware returned messages")
	}
	if messages.SetLevel(r, 5) {
		t.Error("SetLevel without middleware reported success")
	}
	if got := messages.GetLevel(r); got != messages.LevelInfo {
		t.Errorf("GetLevel = %d, want %d", got, messages.LevelInfo)
	}
}

// TestExtraTagsJoined checks that several extra tags are joined with spaces.
func TestExtraTagsJoined(t *testing.T) {
	s := newStorage(messages.StorageSession)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(messages.NewContext(r.Context(), s.Storage))
	if err := messages.Warning(r, "x", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if got := messages.Get(r)[0].Tags(); got != "a b warning" {
		t.Errorf("Tags() = %q, want %q", got, "a b warning")
	}
}

// TestContextProcessorLevels checks the template view of a message and the
// level names.
func TestContextProcessorLevels(t *testing.T) {
	s := storageWith(messages.StorageSession, messages.Message{Level: messages.LevelError, Message: "bad", ExtraTags: "x"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(messages.NewContext(r.Context(), s.Storage))
	ctx := messages.ContextProcessor(r)
	if s.Used() {
		t.Error("the context processor read the messages before a template used them")
	}
	ms := ctx["messages"].(func() []messages.TemplateMessage)()
	want := messages.TemplateMessage{"message": "bad", "level": 40, "tags": "x error", "extra_tags": "x", "level_tag": "error"}
	if len(ms) != 1 || len(ms[0]) != len(want) {
		t.Fatalf("messages = %v, want [%v]", ms, want)
	}
	for k, v := range want {
		if ms[0][k] != v {
			t.Errorf("message[%q] = %v, want %v", k, ms[0][k], v)
		}
	}
	if ms[0].String() != "bad" {
		t.Errorf("String() = %q", ms[0].String())
	}
	if !s.Used() {
		t.Error("calling messages did not mark the storage used")
	}
	if lv := ctx["DEFAULT_MESSAGE_LEVELS"].(map[string]int); lv["ERROR"] != 40 || len(lv) != 5 {
		t.Errorf("DEFAULT_MESSAGE_LEVELS = %v", lv)
	}
}

// TestPyRepr checks the Python quoting of the reprs.
func TestPyRepr(t *testing.T) {
	for in, want := range map[string]string{
		"thing":    `'thing'`,
		"it's":     `"it's"`,
		`it's "x"`: `'it\'s "x"'`,
		"a\\b\n\t": `'a\\b\n\t'`,
		"\x00\x7f": `'\x00\x7f'`,
		"\u00a0é":  `'\xa0é'`,
		"\u2028":   `'\u2028'`,
	} {
		if got := messages.PyRepr(in); got != want {
			t.Errorf("pyRepr(%q) = %s, want %s", in, got, want)
		}
	}
}
