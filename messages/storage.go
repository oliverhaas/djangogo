package messages

import (
	"encoding/json"
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

// Storage holds the messages of one request in its session (Django's
// SessionStorage). The messages that the request brought are read from the
// session on first use, and Update writes back the ones still unread. A
// Storage is not safe for concurrent use.
type Storage struct {
	sess *sessions.Session
	// loaded holds the messages read from the session, once loadedOK is
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

// NewSessionStorage returns the storage of the messages in sess. Middleware
// makes one for every request; a test of a handler can put one into the
// request context with NewContext. It panics when sess is nil, as Django's
// SessionStorage raises ImproperlyConfigured for a request without a session.
func NewSessionStorage(sess *sessions.Session) *Storage {
	if sess == nil {
		panic(configError(noSessionMessage))
	}
	return &Storage{sess: sess}
}

// loadedMessages returns the messages that the request brought, reading them
// from the session on the first call.
func (s *Storage) loadedMessages() []Message {
	if !s.loadedOK {
		v, _ := s.sess.Get(SessionKey)
		s.loaded = decodeMessages(v)
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

// Update writes the unread messages to the session (Django's update). After a
// read only the messages added since are kept. Without a read the messages
// that the request brought are kept too, but the session is only touched when
// a message was added. Middleware calls it before the response goes out.
func (s *Storage) Update() {
	switch {
	case s.used:
		s.store(s.queued)
	case s.addedNew:
		s.store(append(slices.Clone(s.loadedMessages()), s.queued...))
	}
}

// store writes ms to the session, or removes the key when ms is empty.
func (s *Storage) store(ms []Message) {
	if len(ms) == 0 {
		s.sess.Pop(SessionKey)
		return
	}
	s.sess.Set(SessionKey, encodeMessages(ms))
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
