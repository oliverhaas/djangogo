package messages_test

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
)

// The tests in this file are the tests of Django's messages_tests/base.py
// BaseTests as SessionTests runs them, and the tests of SessionTests itself.

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_repr
func TestBaseTestsRepr(t *testing.T) {
	t.Skip("a Go storage has no Python repr")
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_add
func TestBaseTestsAdd(t *testing.T) {
	storage := storageWith()
	if storage.AddedNew() {
		t.Error("AddedNew is set before an add")
	}
	storage.Add(messages.LevelInfo, "Test message 1", "")
	if !storage.AddedNew() {
		t.Error("AddedNew is not set after an add")
	}
	storage.Add(messages.LevelInfo, "Test message 2", "tag")
	if storage.Len() != 2 {
		t.Errorf("Len() = %d, want 2", storage.Len())
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_add_lazy_translation
func TestBaseTestsAddLazyTranslation(t *testing.T) {
	t.Skip("Djan-Go-Go has no lazy translations")
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_no_update
func TestBaseTestsNoUpdate(t *testing.T) {
	storage, sess := storageAndSession()
	storage.Update()
	if got := storedCount(sess); got != 0 {
		t.Errorf("stored messages = %d, want 0", got)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_add_update
func TestBaseTestsAddUpdate(t *testing.T) {
	storage, sess := storageAndSession()
	storage.Add(messages.LevelInfo, "Test message 1", "")
	storage.Add(messages.LevelInfo, "Test message 1", "tag")
	storage.Update()
	if got := storedCount(sess); got != 2 {
		t.Errorf("stored messages = %d, want 2", got)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_existing_add_read_update
func TestBaseTestsExistingAddReadUpdate(t *testing.T) {
	storage, sess := storageAndSession(existingMessages()...)
	storage.Add(messages.LevelInfo, "Test message 3", "")
	storage.Messages() // Simulates a read
	storage.Update()
	if got := storedCount(sess); got != 0 {
		t.Errorf("stored messages = %d, want 0", got)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_existing_read_add_update
func TestBaseTestsExistingReadAddUpdate(t *testing.T) {
	storage, sess := storageAndSession(existingMessages()...)
	storage.Messages() // Simulates a read
	storage.Add(messages.LevelInfo, "Test message 3", "")
	storage.Update()
	if got := storedCount(sess); got != 1 {
		t.Errorf("stored messages = %d, want 1", got)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_full_request_response_cycle
//
// With the message middleware enabled, messages are properly stored and
// retrieved across the full request/redirect/response cycle.
func TestBaseTestsFullRequestResponseCycle(t *testing.T) {
	withConfig(t, messages.Config{Level: messages.LevelDebug})
	c := newClient(t, siteOptions{})
	data := postData(false)
	for _, level := range levelNames {
		resp := c.Post("/add/"+level+"/", data, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, "/show/")
		want := make([]messages.Message, 0, len(data["messages"]))
		for _, msg := range data["messages"] {
			want = append(want, messages.Message{Level: levels[level], Message: msg})
		}
		assertMessagesEqual(t, contextMessages(t, resp), want)
		for _, msg := range data["messages"] {
			djangotest.AssertContains(t, resp, msg)
		}
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_with_template_response
func TestBaseTestsWithTemplateResponse(t *testing.T) {
	withConfig(t, messages.Config{Level: messages.LevelDebug})
	c := newClient(t, siteOptions{})
	data := postData(false)
	for _, level := range levelNames {
		resp := c.Post("/template_response/add/"+level+"/", data, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, "/template_response/show/")
		if _, ok := resp.Context("messages"); !ok {
			t.Error(`"messages" is not in the context`)
		}
		for _, msg := range data["messages"] {
			djangotest.AssertContains(t, resp, msg)
		}

		// there shouldn't be any messages on second GET request
		resp = c.Get("/template_response/show/")
		for _, msg := range data["messages"] {
			djangotest.AssertNotContains(t, resp, msg)
		}
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_context_processor_message_levels
func TestBaseTestsContextProcessorMessageLevels(t *testing.T) {
	c := newClient(t, siteOptions{})
	resp := c.Get("/template_response/show/")
	v, ok := resp.Context("DEFAULT_MESSAGE_LEVELS")
	if !ok {
		t.Fatal(`"DEFAULT_MESSAGE_LEVELS" is not in the context`)
	}
	got, ok := v.(map[string]int)
	if !ok || !maps.Equal(got, messages.DefaultLevels()) {
		t.Errorf("DEFAULT_MESSAGE_LEVELS = %v, want %v", v, messages.DefaultLevels())
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_multiple_posts
//
// Messages persist properly when multiple POSTs are made before a GET.
func TestBaseTestsMultiplePosts(t *testing.T) {
	withConfig(t, messages.Config{Level: messages.LevelDebug})
	c := newClient(t, siteOptions{})
	data := postData(false)
	want := make([]messages.Message, 0, len(data["messages"])*len(levelNames))
	for _, level := range levelNames {
		for _, msg := range data["messages"] {
			want = append(want, messages.Message{Level: levels[level], Message: msg})
		}
		c.Post("/add/"+level+"/", data)
	}
	resp := c.Get("/show/")
	assertMessagesEqual(t, contextMessages(t, resp), want)
	for _, msg := range data["messages"] {
		djangotest.AssertContains(t, resp, msg)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_middleware_disabled
//
// When the middleware is disabled, an exception is raised when one attempts
// to store a message. The view panics with the error of Add.
func TestBaseTestsMiddlewareDisabled(t *testing.T) {
	withConfig(t, messages.Config{Level: messages.LevelDebug})
	c := newClient(t, siteOptions{withoutMessages: true})
	data := postData(false)
	for _, level := range levelNames {
		v := mustPanic(t, func() { c.Post("/add/"+level+"/", data, djangotest.Follow()) })
		if err, ok := v.(error); !ok || !errors.Is(err, messages.ErrMiddlewareMissing) {
			t.Errorf("panic = %v, want ErrMiddlewareMissing", v)
		}
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_middleware_disabled_fail_silently
//
// When the middleware is disabled, an exception is not raised if
// 'fail_silently' is True.
func TestBaseTestsMiddlewareDisabledFailSilently(t *testing.T) {
	c := newClient(t, siteOptions{withoutMessages: true})
	data := postData(true)
	for _, level := range levelNames {
		resp := c.Post("/add/"+level+"/", data, djangotest.Follow())
		djangotest.AssertRedirects(t, resp, "/show/")
		if _, ok := resp.Context("messages"); ok {
			t.Error(`"messages" is in the context`)
		}
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_existing_read
//
// Reading the existing storage doesn't cause the data to be lost.
func TestBaseTestsExistingRead(t *testing.T) {
	storage := storageWith(existingMessages()...)
	if storage.Used() {
		t.Error("Used is set before a read")
	}
	// After reading the storage engine directly, the used flag is set.
	data := storage.Messages()
	if !storage.Used() {
		t.Error("Used is not set after a read")
	}
	// The data does not disappear because it has been read.
	if !slices.Equal(data, storage.Messages()) {
		t.Errorf("second read = %v, want %v", storage.Messages(), data)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_existing_add
func TestBaseTestsExistingAdd(t *testing.T) {
	storage := storageWith(existingMessages()...)
	if storage.AddedNew() {
		t.Error("AddedNew is set before an add")
	}
	storage.Add(messages.LevelInfo, "Test message 3", "")
	if !storage.AddedNew() {
		t.Error("AddedNew is not set after an add")
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_default_level
func TestBaseTestsDefaultLevel(t *testing.T) {
	// GetLevel works even with no storage on the request.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := messages.GetLevel(req); got != messages.LevelInfo {
		t.Errorf("GetLevel = %d, want %d", got, messages.LevelInfo)
	}

	// GetLevel returns the default level if it hasn't been set.
	storage := storageWith()
	req = req.WithContext(messages.NewContext(req.Context(), storage))
	if got := messages.GetLevel(req); got != messages.LevelInfo {
		t.Errorf("GetLevel = %d, want %d", got, messages.LevelInfo)
	}

	// Only messages of sufficient level get recorded.
	addLevelMessages(storage)
	if storage.Len() != 5 {
		t.Errorf("Len() = %d, want 5", storage.Len())
	}
}

// levelRequest returns a request that carries a fresh storage and the storage.
func levelRequest() (*http.Request, *messages.Storage) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	storage := newStorage()
	return req.WithContext(messages.NewContext(req.Context(), storage)), storage
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_low_level
func TestBaseTestsLowLevel(t *testing.T) {
	req, storage := levelRequest()
	if !messages.SetLevel(req, 5) {
		t.Error("SetLevel = false, want true")
	}
	if got := messages.GetLevel(req); got != 5 {
		t.Errorf("GetLevel = %d, want 5", got)
	}
	addLevelMessages(storage)
	if storage.Len() != 6 {
		t.Errorf("Len() = %d, want 6", storage.Len())
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_high_level
func TestBaseTestsHighLevel(t *testing.T) {
	req, storage := levelRequest()
	if !messages.SetLevel(req, 30) {
		t.Error("SetLevel = false, want true")
	}
	if got := messages.GetLevel(req); got != 30 {
		t.Errorf("GetLevel = %d, want 30", got)
	}
	addLevelMessages(storage)
	if storage.Len() != 2 {
		t.Errorf("Len() = %d, want 2", storage.Len())
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_settings_level
func TestBaseTestsSettingsLevel(t *testing.T) {
	withConfig(t, messages.Config{Level: 29})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	storage := newStorage()
	if got := messages.GetLevel(req); got != 29 {
		t.Errorf("GetLevel = %d, want 29", got)
	}
	addLevelMessages(storage)
	if storage.Len() != 3 {
		t.Errorf("Len() = %d, want 3", storage.Len())
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_tags
//
// The message that Django adds with extra_tags=None has empty extra tags in
// Go.
func TestBaseTestsTags(t *testing.T) {
	storage := storageWith()
	storage.SetLevel(0)
	addLevelMessages(storage)
	storage.Add(messages.LevelInfo, "A generic info message", "")
	read := storage.Messages()
	tags := make([]string, 0, len(read))
	for _, msg := range read {
		tags = append(tags, msg.Tags())
	}
	want := []string{"info", "", "extra-tag debug", "warning", "error", "success", "info"}
	if !slices.Equal(tags, want) {
		t.Errorf("tags = %q, want %q", tags, want)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_level_tag
func TestBaseTestsLevelTag(t *testing.T) {
	storage := storageWith()
	storage.SetLevel(0)
	addLevelMessages(storage)
	read := storage.Messages()
	tags := make([]string, 0, len(read))
	for _, msg := range read {
		tags = append(tags, msg.LevelTag())
	}
	want := []string{"info", "", "debug", "warning", "error", "success"}
	if !slices.Equal(tags, want) {
		t.Errorf("level tags = %q, want %q", tags, want)
	}
}

// Ported from Django: tests/messages_tests/base.py::BaseTests.test_custom_tags
func TestBaseTestsCustomTags(t *testing.T) {
	withConfig(t, messages.Config{Tags: map[int]string{
		messages.LevelInfo:    "info",
		messages.LevelDebug:   "",
		messages.LevelWarning: "",
		messages.LevelError:   "bad",
		29:                    "custom",
	}})
	storage := storageWith()
	storage.SetLevel(0)
	addLevelMessages(storage)
	read := storage.Messages()
	tags := make([]string, 0, len(read))
	for _, msg := range read {
		tags = append(tags, msg.Tags())
	}
	want := []string{"info", "custom", "extra-tag", "", "bad", "success"}
	if !slices.Equal(tags, want) {
		t.Errorf("tags = %q, want %q", tags, want)
	}
}

// Ported from Django: tests/messages_tests/test_session.py::SessionTests.test_no_session
func TestSessionTestsNoSession(t *testing.T) {
	msg := "The session-based temporary message storage requires session " +
		"middleware to be installed, and come before the message " +
		"middleware in the MIDDLEWARE list."
	v := mustPanic(t, func() { messages.NewSessionStorage(nil) })
	if err, ok := v.(error); !ok || err.Error() != msg {
		t.Errorf("panic = %v, want an error with the text %q", v, msg)
	}
}

// Ported from Django: tests/messages_tests/test_session.py::SessionTests.test_get
//
// Django stores the plain strings "test" and "me" in the session. The Go
// storage reads only the JSON of stored messages, so the test stores
// messages.
func TestSessionTestsGet(t *testing.T) {
	storage, sess := storageAndSession()
	example := []messages.Message{{Level: messages.LevelInfo, Message: "test"}, {Level: messages.LevelInfo, Message: "me"}}
	sess.Set(messages.SessionKey, messages.EncodeMessages(example))
	storage.ClearLoaded()
	if got := storage.Messages(); !slices.Equal(got, example) {
		t.Errorf("messages = %v, want %v", got, example)
	}
}

// Ported from Django: tests/messages_tests/test_session.py::SessionTests.test_safedata
func TestSessionTestsSafedata(t *testing.T) {
	// A message containing SafeData keeps its safe status when retrieved from
	// the message storage.
	storage, sess := storageAndSession()
	message := messages.Message{Level: messages.LevelDebug, Message: "<b>Hello Django!</b>", Safe: true}
	sess.Set(messages.SessionKey, messages.EncodeMessages([]messages.Message{message}))
	storage.ClearLoaded()
	if got := storage.Messages(); len(got) != 1 || !got[0].Safe {
		t.Errorf("messages = %#v, want one safe message", got)
	}
}
