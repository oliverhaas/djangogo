package messages

import (
	"fmt"
	"maps"
	"strings"
	"sync/atomic"
)

// The message levels of Django. Any other integer is a custom level; give it
// a tag with Config.Tags.
const (
	// LevelDebug marks messages for development that a production site
	// leaves out (Django's DEBUG).
	LevelDebug = 10
	// LevelInfo marks information for the user (Django's INFO).
	LevelInfo = 20
	// LevelSuccess marks an action that succeeded (Django's SUCCESS).
	LevelSuccess = 25
	// LevelWarning marks a failure that did not occur yet but may be near
	// (Django's WARNING).
	LevelWarning = 30
	// LevelError marks an action that failed (Django's ERROR).
	LevelError = 40
)

// DefaultTags returns the tag of each standard level (Django's DEFAULT_TAGS).
// The map is a copy.
func DefaultTags() map[int]string {
	return map[int]string{
		LevelDebug:   "debug",
		LevelInfo:    "info",
		LevelSuccess: "success",
		LevelWarning: "warning",
		LevelError:   "error",
	}
}

// DefaultLevels returns the standard levels by their names (Django's
// DEFAULT_LEVELS), the value of DEFAULT_MESSAGE_LEVELS in templates. The map
// is a copy.
func DefaultLevels() map[string]int {
	return map[string]int{
		"DEBUG":   LevelDebug,
		"INFO":    LevelInfo,
		"SUCCESS": LevelSuccess,
		"WARNING": LevelWarning,
		"ERROR":   LevelError,
	}
}

// Config holds the settings of the messages app. The zero value is Django's
// default for every field.
type Config struct {
	// Level is the lowest level that is recorded (Django's MESSAGE_LEVEL).
	// Zero means LevelInfo. A negative level records every message.
	Level int
	// Tags maps a level to its tag, which templates use as the CSS class of
	// a message (Django's MESSAGE_TAGS). The entries are merged over
	// DefaultTags: an entry changes the tag of a standard level or names a
	// custom one.
	Tags map[int]string
}

// settings is a Config with the defaults applied.
type settings struct {
	// level is the lowest recorded level.
	level int
	// tags is DefaultTags with Config.Tags merged over it.
	tags map[int]string
}

// defaults is the configuration before Configure is called.
var defaults = &settings{level: LevelInfo, tags: DefaultTags()}

// current is the active configuration, replaced as a whole so that a reader
// never sees a half-applied change.
var current atomic.Pointer[settings]

// Configure replaces the settings of the messages app and returns a function
// that puts the previous ones back. Call it once at startup, before requests
// are served; a test that changes the configuration defers the restore
// function and must not run in parallel with other tests.
func Configure(cfg Config) (restore func()) {
	next := &settings{level: cfg.Level, tags: DefaultTags()}
	if next.level == 0 {
		next.level = LevelInfo
	}
	maps.Copy(next.tags, cfg.Tags)
	prev := current.Swap(next)
	return func() { current.Store(prev) }
}

// config returns the active configuration.
func config() *settings {
	if s := current.Load(); s != nil {
		return s
	}
	return defaults
}

// Message is one message for the user (Django's Message).
type Message struct {
	// Level is the importance of the message: one of the Level constants or
	// a custom level.
	Level int
	// Message is the text. Templates escape it.
	Message string
	// ExtraTags are further tags, separated by spaces, that Tags puts before
	// the level tag.
	ExtraTags string
}

// Equal reports whether m and other have the same level and text. Like
// Django's Message.__eq__ it ignores the extra tags.
func (m Message) Equal(other Message) bool {
	return m.Level == other.Level && m.Message == other.Message
}

// String returns the text of the message, as Django's str(message).
func (m Message) String() string { return m.Message }

// LevelTag returns the tag of the level from DefaultTags and Config.Tags, or
// "" for a level without one.
func (m Message) LevelTag() string { return config().tags[m.Level] }

// Tags returns the extra tags and the level tag, separated by a space. An
// empty one is left out.
func (m Message) Tags() string {
	tags := make([]string, 0, 2)
	for _, t := range []string{m.ExtraTags, m.LevelTag()} {
		if t != "" {
			tags = append(tags, t)
		}
	}
	return strings.Join(tags, " ")
}

// Repr returns Django's repr of the message, such as
// Message(level=20, message='thing'). The failure messages of
// djangotest.AssertMessages show it.
func (m Message) Repr() string {
	extra := ""
	if m.ExtraTags != "" {
		extra = ", extra_tags=" + pyRepr(m.ExtraTags)
	}
	return fmt.Sprintf("Message(level=%d, message=%s%s)", m.Level, pyRepr(m.Message), extra)
}

// TemplateMessage is a message as templates see it: a map with Django's
// attribute names "message", "level", "tags", "extra_tags" and "level_tag",
// so that {{ message.tags }} works. Its String method returns the escaped
// text, so {{ message }} prints it. Apply filters to message.message rather
// than to message, which would escape the text twice.
type TemplateMessage map[string]any

// htmlEscaper escapes text as pongo2's escape filter does.
var htmlEscaper = strings.NewReplacer("&", "&amp;", ">", "&gt;", "<", "&lt;", `"`, "&quot;", "'", "&#39;")

// String returns the text of the message, HTML-escaped. pongo2 escapes only
// values of a string type and prints any other Stringer as it is, so the
// escaping happens here, as Django escapes {{ message }}.
func (t TemplateMessage) String() string {
	s, _ := t["message"].(string)
	return htmlEscaper.Replace(s)
}

// templateMessage returns the template view of m.
func templateMessage(m Message) TemplateMessage {
	return TemplateMessage{
		"message":    m.Message,
		"level":      m.Level,
		"tags":       m.Tags(),
		"extra_tags": m.ExtraTags,
		"level_tag":  m.LevelTag(),
	}
}
