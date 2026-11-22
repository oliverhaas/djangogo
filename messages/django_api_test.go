package messages_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/messages"
	"github.com/oliverhaas/djangogo/sessions"
)

// apiRequest returns a request that carries a storage and the storage (the
// setUp and test_ok of ApiTests: request._messages = DummyStorage()).
func apiRequest() (*http.Request, *messages.Storage) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	storage := newStorage(messages.StorageSession).Storage
	return req.WithContext(messages.NewContext(req.Context(), storage)), storage
}

// Ported from Django: tests/messages_tests/test_api.py::ApiTests.test_ok
//
// Django's DummyStorage records every level; the real storage drops a message
// below Config.Level, so the test lowers the level to DEBUG.
func TestApiTestsOk(t *testing.T) {
	withConfig(t, messages.Config{Level: messages.LevelDebug})
	req, storage := apiRequest()
	msg := "some message"
	if err := messages.Add(req, messages.LevelDebug, msg); err != nil {
		t.Fatal(err)
	}
	store := storage.Queued()
	if len(store) != 1 || store[0].Message != msg {
		t.Errorf("stored = %v, want one message %q", store, msg)
	}
}

// Ported from Django: tests/messages_tests/test_api.py::ApiTests.test_request_is_none
func TestApiTestsRequestIsNone(t *testing.T) {
	t.Skip("the request argument is a typed *http.Request, so the TypeError of Django has no counterpart")
}

// Ported from Django: tests/messages_tests/test_api.py::ApiTests.test_middleware_missing
func TestApiTestsMiddlewareMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	storage := newStorage(messages.StorageSession).Storage
	msg := "You cannot add messages without installing " +
		"django.contrib.messages.middleware.MessageMiddleware"
	err := messages.Add(req, messages.LevelDebug, "some message")
	if !errors.Is(err, messages.ErrMiddlewareMissing) || err.Error() != msg {
		t.Errorf("Add error = %v, want ErrMiddlewareMissing with the text %q", err, msg)
	}
	if got := storage.Queued(); len(got) != 0 {
		t.Errorf("stored = %v, want none", got)
	}
}

// Ported from Django: tests/messages_tests/test_api.py::ApiTests.test_middleware_missing_silently
//
// Go has no fail_silently argument; a caller that does not mind ignores the
// error.
func TestApiTestsMiddlewareMissingSilently(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	storage := newStorage(messages.StorageSession).Storage
	_ = messages.Add(req, messages.LevelDebug, "some message")
	if got := storage.Queued(); len(got) != 0 {
		t.Errorf("stored = %v, want none", got)
	}
}

// Ported from Django: tests/messages_tests/test_api.py::CustomRequestApiTests.test_ok
func TestCustomRequestApiTestsOk(t *testing.T) {
	t.Skip("Go has no request wrappers that duck-type attributes: Add takes an *http.Request")
}

// Ported from Django: tests/messages_tests/test_api.py::CustomRequestApiTests.test_request_is_none
func TestCustomRequestApiTestsRequestIsNone(t *testing.T) {
	t.Skip("the request argument is a typed *http.Request, and Go has no request wrappers that duck-type attributes")
}

// Ported from Django: tests/messages_tests/test_api.py::CustomRequestApiTests.test_middleware_missing
func TestCustomRequestApiTestsMiddlewareMissing(t *testing.T) {
	t.Skip("Go has no request wrappers that duck-type attributes: Add takes an *http.Request")
}

// Ported from Django: tests/messages_tests/test_api.py::CustomRequestApiTests.test_middleware_missing_silently
func TestCustomRequestApiTestsMiddlewareMissingSilently(t *testing.T) {
	t.Skip("Go has no request wrappers that duck-type attributes: Add takes an *http.Request")
}

// Ported from Django: tests/messages_tests/test_middleware.py::MiddlewareTests.test_response_without_messages
//
// MessageMiddleware is tolerant of messages not existing on request. A
// response that passes through the middleware without a message leaves the
// session untouched, so no session cookie is set.
func TestMiddlewareTestsResponseWithoutMessages(t *testing.T) {
	h := sessions.Middleware(sessions.NewSignedCookieStore([]byte("k")), sessions.DefaultCookieName)(
		messages.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		})))
	resp := djangotest.NewClient(t, h).Get("/")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if cookies := resp.Cookies(); len(cookies) != 0 {
		t.Errorf("cookies set = %v, want none", cookies)
	}
}

// Ported from Django: tests/messages_tests/test_mixins.py::SuccessMessageMixinTests.test_set_messages_success
func TestSuccessMessageMixinTestsSetMessagesSuccess(t *testing.T) {
	t.Skip("SuccessMessageMixin needs forms and generic views, which Djan-Go-Go does not have")
}

// Ported from Django: tests/messages_tests/test_mixins.py::SuccessMessageMixinTests.test_set_messages_success_on_delete
func TestSuccessMessageMixinTestsSetMessagesSuccessOnDelete(t *testing.T) {
	t.Skip("SuccessMessageMixin needs forms and generic views, which Djan-Go-Go does not have")
}
