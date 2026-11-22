package messages

import "time"

// SetLoaded sets the messages that the request brought without reading the
// session, as the get_storage helper of Django's tests sets _loaded_data.
func (s *Storage) SetLoaded(ms []Message) {
	s.loaded = ms
	s.loadedOK = true
}

// ClearLoaded drops the messages read so far, so that the next use reads the
// session again (the del storage._loaded_data of Django's set_session_data).
func (s *Storage) ClearLoaded() {
	s.loaded = nil
	s.loadedOK = false
}

// Queued returns the messages added since the last read.
func (s *Storage) Queued() []Message { return s.queued }

// EncodeMessages exposes encodeMessages (Django's serialize_messages).
var EncodeMessages = encodeMessages

// DecodeMessages exposes decodeMessages (Django's deserialize_messages).
var DecodeMessages = decodeMessages

// PyRepr exposes pyRepr.
var PyRepr = pyRepr

// EncodeCookie returns the signed cookie value of ms, with the notFinished
// marker at the end when unfinished is set, as the set_cookie_data helper of
// Django's tests writes it. No messages give "" unless encodeEmpty is set.
func EncodeCookie(opts CookieOptions, ms []Message, unfinished, encodeEmpty bool) string {
	parts := make([]string, 0, len(ms)+1)
	for _, m := range ms {
		parts = append(parts, encodeMessage(m))
	}
	if unfinished {
		parts = append(parts, notFinishedJSON)
	}
	return (&cookieBackend{opts: opts, now: time.Now}).encodeParts(parts, encodeEmpty)
}

// DecodeCookie exposes decodeCookie (Django's CookieStorage._decode).
var DecodeCookie = decodeCookie

// BisectKeepLeft exposes bisectKeepLeft for a list of numbers.
var BisectKeepLeft = bisectKeepLeft[int]

// BisectKeepRight exposes bisectKeepRight for a list of numbers.
var BisectKeepRight = bisectKeepRight[int]

// EncodeCookieAt returns the value of EncodeCookie signed at t.
func EncodeCookieAt(opts CookieOptions, ms []Message, t time.Time) string {
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		parts = append(parts, encodeMessage(m))
	}
	return (&cookieBackend{opts: opts, now: func() time.Time { return t }}).encodeParts(parts, false)
}
