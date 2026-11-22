package sessions

import (
	"time"
)

// DefaultCookieAge is Django's SESSION_COOKIE_AGE: two weeks.
const DefaultCookieAge = 14 * 24 * time.Hour

// expiryKey is the session value that holds a custom expiry (Django's
// "_session_expiry"): a number of seconds or a date in RFC 3339 form.
const expiryKey = "_session_expiry"

// SetExpiry makes the session expire after d of inactivity (Django's
// set_expiry with a number of seconds). A zero d makes the session cookie
// expire when the browser closes. A negative d expires the session at once.
func (s *Session) SetExpiry(d time.Duration) {
	s.Set(expiryKey, int64(d/time.Second))
}

// SetExpiryAt makes the session expire at the time t (Django's set_expiry
// with a datetime).
func (s *Session) SetExpiryAt(t time.Time) {
	s.Set(expiryKey, t.UTC().Format(time.RFC3339Nano))
}

// ClearExpiry removes a custom expiry, so that the session follows the policy
// of the middleware again (Django's set_expiry(None)).
func (s *Session) ClearExpiry() {
	s.Delete(expiryKey)
}

// GetExpiryAge returns the time until the session expires (Django's
// get_expiry_age). Without a custom expiry this is the cookie age of the
// middleware.
func (s *Session) GetExpiryAge() time.Duration {
	_ = s.load()
	s.accessed = true
	return s.expiryAge(time.Now())
}

// GetExpiryDate returns the time at which the session expires (Django's
// get_expiry_date).
func (s *Session) GetExpiryDate() time.Time {
	_ = s.load()
	s.accessed = true
	return s.expiryDate(time.Now())
}

// GetExpireAtBrowserClose reports whether the session cookie is to expire when
// the browser closes (Django's get_expire_at_browser_close). A custom expiry
// decides: only SetExpiry(0) says yes. Without one the policy of the
// middleware applies.
func (s *Session) GetExpireAtBrowserClose() bool {
	_ = s.load()
	s.accessed = true
	seconds, _, kind := s.expiry()
	if kind == kindNone {
		return s.browserClose
	}
	return kind == kindSeconds && seconds == 0
}

// Kinds of custom expiry held under expiryKey.
const (
	kindNone = iota
	kindSeconds
	kindDate
)

// expiry decodes the custom expiry. A value read back from JSON is a float64
// or a string; a value set during this request is an int64 or a string.
func (s *Session) expiry() (seconds int64, date time.Time, kind int) {
	switch v := s.data[expiryKey].(type) {
	case int64:
		return v, time.Time{}, kindSeconds
	case int:
		return int64(v), time.Time{}, kindSeconds
	case float64:
		return int64(v), time.Time{}, kindSeconds
	case string:
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return 0, time.Time{}, kindNone
		}
		return 0, t, kindDate
	default:
		return 0, time.Time{}, kindNone
	}
}

// age returns the cookie age of the middleware that loaded the session.
func (s *Session) age() time.Duration {
	if s.cookieAge > 0 {
		return s.cookieAge
	}
	return DefaultCookieAge
}

// expiryAge is GetExpiryAge for the given current time.
func (s *Session) expiryAge(now time.Time) time.Duration {
	seconds, date, kind := s.expiry()
	switch {
	case kind == kindDate:
		return date.Sub(now)
	case kind == kindSeconds && seconds != 0:
		return time.Duration(seconds) * time.Second
	default:
		return s.age()
	}
}

// expiryDate is GetExpiryDate for the given current time.
func (s *Session) expiryDate(now time.Time) time.Time {
	if _, date, kind := s.expiry(); kind == kindDate {
		return date
	}
	return now.Add(s.expiryAge(now))
}
