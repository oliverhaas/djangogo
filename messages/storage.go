package messages

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/oliverhaas/djangogo/sessions"
)

// SessionKey is the session key that holds the messages (Django's
// SessionStorage.session_key).
const SessionKey = "_messages"

// messageMarker is the first element of a stored message (Django's
// MessageEncoder.message_key).
const messageMarker = "__json_message"

// noSessionMessage is Django's text for a request without a session.
const noSessionMessage = "The session-based temporary message storage requires session " +
	"middleware to be installed, and come before the message " +
	"middleware in the MIDDLEWARE list."

// configError is the error of a wrong setup (Django's ImproperlyConfigured).
type configError string

// Error returns the text of the error.
func (e configError) Error() string { return string(e) }

// Storage holds the messages of one request (Django's BaseStorage). The
// messages that the request brought are read on first use, and Update stores
// the ones still unread for the next request. Where they are kept depends on
// the constructor: NewSessionStorage, NewCookieStorage or NewFallbackStorage.
// A Storage is not safe for concurrent use.
type Storage struct {
	b backend
	// loaded holds the messages that the request brought, once loadedOK is
	// set, and the messages that a read moved over from queued.
	loaded   []Message
	loadedOK bool
	// queued holds the messages added since the last read.
	queued []Message
	// used reports that the messages were read; addedNew that one was added.
	used     bool
	addedNew bool
	// level is the lowest recorded level, once levelSet is set.
	level    int
	levelSet bool
}

// backend loads and stores the messages of one request: the _get and _store
// methods of Django's storage classes.
type backend interface {
	// load returns the stored messages, nil when the backend holds no data,
	// and whether they are all the messages that were stored (Django's
	// all_retrieved). bad reports stored data that could not be read.
	load() (ms []Message, all, bad bool)
	// store keeps ms for the next request and returns the messages it could
	// not keep. removeOldest leaves out the oldest messages first, otherwise
	// the newest. A backend that writes a cookie adds its header to h.
	store(ms []Message, h http.Header, removeOldest bool) []Message
}

// NewSessionStorage returns the storage of the messages in sess (Django's
// SessionStorage). Middleware makes one for every request; a test of a
// handler can put one into the request context with NewContext. It panics
// when sess is nil, as Django's SessionStorage raises ImproperlyConfigured
// for a request without a session.
func NewSessionStorage(sess *sessions.Session) *Storage {
	return &Storage{b: newSessionBackend(sess)}
}

// NewCookieStorage returns the storage of the messages in a signed cookie
// (Django's CookieStorage): the messages that r brought in the cookie, and
// Update writes the cookie into the response header. Messages that do not
// fit into MaxCookieSize are dropped, the oldest first. It panics when
// opts.Secret is empty.
func NewCookieStorage(r *http.Request, opts CookieOptions) *Storage {
	return &Storage{b: newCookieBackend(r, opts)}
}

// NewFallbackStorage returns the storage of the messages in a signed cookie
// and, for the messages that do not fit there, in sess (Django's
// FallbackStorage, its default storage). It panics when sess is nil or
// opts.Secret is empty.
func NewFallbackStorage(r *http.Request, sess *sessions.Session, opts CookieOptions) *Storage {
	backends := []backend{newCookieBackend(r, opts), newSessionBackend(sess)}
	return &Storage{b: &fallbackBackend{backends: backends, used: make([]bool, len(backends))}}
}

// loadedMessages returns the messages that the request brought, loading them
// on the first call. Data that cannot be read marks the storage used, so that
// Update removes it, as Django's CookieStorage._decode does.
func (s *Storage) loadedMessages() []Message {
	if !s.loadedOK {
		var bad bool
		s.loaded, _, bad = s.b.load()
		s.used = s.used || bad
		s.loadedOK = true
	}
	return s.loaded
}

// Len returns the number of messages, the ones the request brought and the
// ones added since.
func (s *Storage) Len() int { return len(s.loadedMessages()) + len(s.queued) }

// Messages returns every message and marks the storage used, so that Update
// keeps only the messages added afterwards (Django's iteration of the
// storage).
func (s *Storage) Messages() []Message {
	s.used = true
	s.loadedMessages()
	if len(s.queued) > 0 {
		s.loaded = append(s.loaded, s.queued...)
		s.queued = nil
	}
	return slices.Clone(s.loaded)
}

// Used reports whether the messages were read.
func (s *Storage) Used() bool { return s.used }

// AddedNew reports whether a message was added.
func (s *Storage) AddedNew() bool { return s.addedNew }

// Add queues a message (Django's BaseStorage.add). A message with an empty
// text or a level below Level is dropped.
func (s *Storage) Add(level int, message, extraTags string) {
	s.add(Message{Level: level, Message: message, ExtraTags: extraTags})
}

// AddSafe queues a message whose text is HTML that templates print as it is,
// as Add does.
func (s *Storage) AddSafe(level int, html, extraTags string) {
	s.add(Message{Level: level, Message: html, ExtraTags: extraTags, Safe: true})
}

// add queues m unless its text is empty or its level is below Level.
func (s *Storage) add(m Message) {
	if m.Message == "" || m.Level < s.Level() {
		return
	}
	s.addedNew = true
	s.queued = append(s.queued, m)
}

// Level returns the lowest level that Add records: the one of SetLevel, or
// else Config.Level.
func (s *Storage) Level() int {
	if s.levelSet {
		return s.level
	}
	return config().level
}

// SetLevel sets the lowest level that Add records for this request.
func (s *Storage) SetLevel(level int) {
	s.level = level
	s.levelSet = true
}

// ResetLevel makes Level return Config.Level again (Django's
// storage.level = None).
func (s *Storage) ResetLevel() { s.levelSet = false }

// Update stores the unread messages for the next request and returns the
// ones that could not be stored (Django's update). After a read only the
// messages added since are kept. Without a read the messages that the request
// brought are kept too, but nothing is written unless a message was added. A
// storage that writes a cookie adds its Set-Cookie header to h, the header of
// the response; the session storage does not use h. Middleware calls it
// before the response headers go out.
func (s *Storage) Update(h http.Header) []Message {
	switch {
	case s.used:
		return s.b.store(s.queued, h, true)
	case s.addedNew:
		return s.b.store(append(slices.Clone(s.loadedMessages()), s.queued...), h, true)
	}
	return nil
}

// sessionBackend keeps the messages in the session (Django's
// SessionStorage).
type sessionBackend struct {
	sess *sessions.Session
}

// newSessionBackend returns the backend of sess. It panics when sess is nil.
func newSessionBackend(sess *sessions.Session) sessionBackend {
	if sess == nil {
		panic(configError(noSessionMessage))
	}
	return sessionBackend{sess: sess}
}

// load reads the messages of the session, which always holds all of them.
func (b sessionBackend) load() (ms []Message, all, bad bool) {
	v, ok := b.sess.Get(SessionKey)
	if !ok {
		return nil, true, false
	}
	return decodeMessages(v), true, false
}

// store writes ms to the session, or removes the key when ms is empty. The
// session keeps every message.
func (b sessionBackend) store(ms []Message, _ http.Header, _ bool) []Message {
	if len(ms) == 0 {
		b.sess.Pop(SessionKey)
		return nil
	}
	b.sess.Set(SessionKey, encodeMessages(ms))
	return nil
}

// encodeMessages returns ms in Django's JSON format: a list of
// ["__json_message", is_safe, level, message, extra_tags] entries, with
// is_safe 1 for a safe message and 0 otherwise.
func encodeMessages(ms []Message) string {
	rows := make([][]any, len(ms))
	for i, m := range ms {
		safe := 0
		if m.Safe {
			safe = 1
		}
		rows[i] = []any{messageMarker, safe, m.Level, m.Message, m.ExtraTags}
	}
	// A list of strings and integers always encodes.
	b, _ := json.Marshal(rows)
	return string(b)
}

// decodeMessages parses the session value v that encodeMessages or Django
// wrote. A value that is not such a list yields no messages, and an entry
// that is not a message is skipped.
func decodeMessages(v any) []Message {
	data, ok := v.(string)
	if !ok || data == "" {
		return nil
	}
	var rows []any
	if err := json.Unmarshal([]byte(data), &rows); err != nil {
		return nil
	}
	ms := make([]Message, 0, len(rows))
	for _, row := range rows {
		if m, ok := decodeMessage(row); ok {
			ms = append(ms, m)
		}
	}
	return ms
}

// decodeMessage parses one entry of the stored list.
func decodeMessage(row any) (Message, bool) {
	fields, ok := row.([]any)
	if !ok || len(fields) < 4 || len(fields) > 5 || fields[0] != messageMarker {
		return Message{}, false
	}
	level, ok := fields[2].(float64)
	if !ok {
		return Message{}, false
	}
	text, ok := fields[3].(string)
	if !ok {
		return Message{}, false
	}
	// Django's decoder marks the text safe for any true is_safe.
	m := Message{Level: int(level), Message: text, Safe: truthy(fields[1])}
	if len(fields) == 5 {
		// Django writes null for extra tags of None.
		m.ExtraTags, _ = fields[4].(string)
	}
	return m, true
}

// truthy reports whether the JSON value v is true for Python: a nonzero
// number, true, or a nonempty string, list or object.
func truthy(v any) bool {
	switch v := v.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return false
}
