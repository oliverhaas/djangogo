package messages

import (
	"context"
	"net/http"
	"strings"
)

// failure is the error type of Add (Django's MessageFailure).
type failure string

// Error returns the text of the error.
func (e failure) Error() string { return string(e) }

// ErrMiddlewareMissing is the error of Add for a request that did not pass
// through Middleware (Django's MessageFailure). A caller that does not mind,
// Django's fail_silently, ignores it.
var ErrMiddlewareMissing error = failure("You cannot add messages without installing " +
	"django.contrib.messages.middleware.MessageMiddleware")

// contextKey is the request context key of the storage.
type contextKey struct{}

// NewContext returns a copy of ctx that carries the storage s (Django's
// request._messages). Middleware does this for every request; a test of a
// handler can do it by hand.
func NewContext(ctx context.Context, s *Storage) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// FromContext returns the storage in ctx and whether there is one.
func FromContext(ctx context.Context) (*Storage, bool) {
	s, ok := ctx.Value(contextKey{}).(*Storage)
	return s, ok
}

// Add adds a message for the user of the request (Django's add_message). The
// extra tags are joined with spaces. A message with an empty text or a level
// below GetLevel is dropped. Add returns ErrMiddlewareMissing when the
// request did not pass through Middleware.
func Add(r *http.Request, level int, message string, extraTags ...string) error {
	s, ok := FromContext(r.Context())
	if !ok {
		return ErrMiddlewareMissing
	}
	s.Add(level, message, strings.Join(extraTags, " "))
	return nil
}

// Debug adds a message with LevelDebug, which the default Config.Level
// drops (Django's debug).
func Debug(r *http.Request, message string, extraTags ...string) error {
	return Add(r, LevelDebug, message, extraTags...)
}

// Info adds a message with LevelInfo (Django's info).
func Info(r *http.Request, message string, extraTags ...string) error {
	return Add(r, LevelInfo, message, extraTags...)
}

// Success adds a message with LevelSuccess (Django's success).
func Success(r *http.Request, message string, extraTags ...string) error {
	return Add(r, LevelSuccess, message, extraTags...)
}

// Warning adds a message with LevelWarning (Django's warning).
func Warning(r *http.Request, message string, extraTags ...string) error {
	return Add(r, LevelWarning, message, extraTags...)
}

// Error adds a message with LevelError (Django's error).
func Error(r *http.Request, message string, extraTags ...string) error {
	return Add(r, LevelError, message, extraTags...)
}

// Get returns the messages of the request and marks them consumed, so that
// the next response does not show them again (Django's iteration of
// get_messages). Without Middleware it returns nil.
func Get(r *http.Request) []Message {
	s, ok := FromContext(r.Context())
	if !ok {
		return nil
	}
	return s.Messages()
}

// GetLevel returns the lowest level that Add records for the request
// (Django's get_level): the one of SetLevel, or else Config.Level.
func GetLevel(r *http.Request) int {
	if s, ok := FromContext(r.Context()); ok {
		return s.Level()
	}
	return config().level
}

// SetLevel sets the lowest level that Add records for the request and reports
// whether it could (Django's set_level). It cannot without Middleware.
func SetLevel(r *http.Request, level int) bool {
	s, ok := FromContext(r.Context())
	if !ok {
		return false
	}
	s.SetLevel(level)
	return true
}

// ContextProcessor adds the messages of the request to the template context
// (Django's messages context processor):
//
//   - "messages" is a function that a template calls when it uses the
//     variable. It returns the messages as TemplateMessage values and marks
//     them consumed like Get, so a page that does not show them keeps them
//     for the next one. Unlike in Django, {% if messages %} marks them
//     consumed too.
//   - "DEFAULT_MESSAGE_LEVELS" maps the names of the standard levels to their
//     values, for {% if message.level == DEFAULT_MESSAGE_LEVELS.ERROR %}.
func ContextProcessor(r *http.Request) map[string]any {
	return map[string]any{
		"messages": func() []TemplateMessage {
			ms := Get(r)
			out := make([]TemplateMessage, len(ms))
			for i, m := range ms {
				out[i] = templateMessage(m)
			}
			return out
		},
		"DEFAULT_MESSAGE_LEVELS": DefaultLevels(),
	}
}
