package messages

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
