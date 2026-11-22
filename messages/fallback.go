package messages

import "net/http"

// fallbackBackend keeps the messages in the first backend and the ones that
// do not fit there in the next (Django's FallbackStorage).
type fallbackBackend struct {
	backends []backend
	// used marks the backends that held messages when the request came in,
	// so that store empties them.
	used []bool
}

// load reads the messages of each backend in turn until one held no data or
// all the messages that were left for it.
func (b *fallbackBackend) load() (ms []Message, all, bad bool) {
	all = true
	for i, sub := range b.backends {
		var got []Message
		got, all, _ = sub.load()
		if got == nil {
			break
		}
		if len(got) > 0 {
			b.used[i] = true
		}
		ms = append(ms, got...)
		if all {
			break
		}
	}
	return ms, all, false
}

// store hands ms to the first backend and the messages that it could not
// store to the next, each keeping the oldest messages. A backend that gets no
// messages is emptied when it held some. store returns the messages that no
// backend stored.
func (b *fallbackBackend) store(ms []Message, h http.Header, _ bool) []Message {
	for i, sub := range b.backends {
		switch {
		case len(ms) > 0:
			ms = sub.store(ms, h, false)
		case b.used[i]:
			sub.store(nil, h, true)
			b.used[i] = false
		}
	}
	return ms
}
