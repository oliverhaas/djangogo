package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// eachStore runs fn for the database store on every dialect, for the file
// store and for the signed-cookie store, as Django runs SessionTestsMixin once
// per backend.
func eachStore(t *testing.T, fn func(t *testing.T, store sessions.Store)) {
	t.Helper()
	eachKeyStore(t, func(t *testing.T, store keyStore) { fn(t, store) })
	t.Run("signed_cookies", func(t *testing.T) {
		fn(t, sessions.NewSignedCookieStore([]byte("session-test-secret")))
	})
}

// keyStore is a store that keeps the sessions on the server under their keys:
// the database store and the file store.
type keyStore interface {
	sessions.Store
	// Exists reports whether a session with the key is stored.
	Exists(ctx context.Context, key string) (bool, error)
}

// eachKeyStore runs fn for the database store on every dialect and for the
// file store.
func eachKeyStore(t *testing.T, fn func(t *testing.T, store keyStore)) {
	t.Helper()
	djangotest.EachDB(t, []any{sessions.App{}}, func(t *testing.T, db *orm.DB) {
		fn(t, sessions.NewDBStore(db))
	})
	t.Run("file", func(t *testing.T) {
		fn(t, newFileStore(t))
	})
}

// eachDBStore runs fn for the database store on every dialect.
func eachDBStore(t *testing.T, fn func(t *testing.T, store *sessions.DBStore, db *orm.DB)) {
	t.Helper()
	djangotest.EachDB(t, []any{sessions.App{}}, func(t *testing.T, db *orm.DB) {
		fn(t, sessions.NewDBStore(db), db)
	})
}

// save stores s and returns the cookie value.
func save(t *testing.T, store sessions.Store, s *sessions.Session) string {
	t.Helper()
	value, err := store.Encode(context.Background(), s)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return value
}

// load returns the session for a cookie value.
func load(t *testing.T, store sessions.Store, value string) *sessions.Session {
	t.Helper()
	s, err := store.Decode(context.Background(), value)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return s
}

// reloaded saves s and loads it again. The result holds the same values with
// the accessed and modified flags of a new request, which Django's tests get
// by resetting the two attributes.
func reloaded(t *testing.T, store sessions.Store, s *sessions.Session) *sessions.Session {
	t.Helper()
	return load(t, store, save(t, store, s))
}

// exists reports whether the store holds a session with the key.
func exists(t *testing.T, store keyStore, key string) bool {
	t.Helper()
	ok, err := store.Exists(context.Background(), key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	return ok
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_new_session
func TestNewSession(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		djangotest.AssertEqual(t, s.Modified(), false)
		djangotest.AssertEqual(t, s.Accessed(), false)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_get_empty
func TestGetEmpty(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		_, ok := store.New().Get("cat")
		djangotest.AssertEqual(t, ok, false)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_store
func TestStore(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("cat", "dog")
		djangotest.AssertEqual(t, s.Modified(), true)
		v, _ := s.Pop("cat")
		djangotest.AssertEqual(t, v, any("dog"))
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_pop
func TestPop(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("some key", "exists")
		s = reloaded(t, store, s)

		v, _ := s.Pop("some key")
		djangotest.AssertEqual(t, v, any("exists"))
		djangotest.AssertEqual(t, s.Accessed(), true)
		djangotest.AssertEqual(t, s.Modified(), true)
		_, ok := s.Get("some key")
		djangotest.AssertEqual(t, ok, false)
	})
}

// Go has no default argument: Pop reports a missing key through its boolean.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_pop_default
func TestPopDefault(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		v, ok := s.Pop("some key")
		djangotest.AssertEqual(t, v, nil)
		djangotest.AssertEqual(t, ok, false)
		djangotest.AssertEqual(t, s.Accessed(), true)
		djangotest.AssertEqual(t, s.Modified(), false)
	})
}

// Django raises KeyError. Go returns false and does not panic.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_pop_no_default_keyerror_raised
func TestPopMissingKey(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		_, ok := store.New().Pop("some key")
		djangotest.AssertEqual(t, ok, false)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_setdefault
func TestSetDefault(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		djangotest.AssertEqual(t, s.SetDefault("foo", "bar"), any("bar"))
		djangotest.AssertEqual(t, s.SetDefault("foo", "baz"), any("bar"))
		djangotest.AssertEqual(t, s.Accessed(), true)
		djangotest.AssertEqual(t, s.Modified(), true)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_update
func TestUpdate(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Update(map[string]any{"update key": 1})
		djangotest.AssertEqual(t, s.Accessed(), true)
		djangotest.AssertEqual(t, s.Modified(), true)
		v, _ := s.Get("update key")
		djangotest.AssertEqual(t, v, any(1))
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_has_key
func TestHasKey(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("some key", 1)
		s = reloaded(t, store, s)

		djangotest.AssertEqual(t, s.Has("some key"), true)
		djangotest.AssertEqual(t, s.Accessed(), true)
		djangotest.AssertEqual(t, s.Modified(), false)
	})
}

// Data stands for Django's items: Go has one accessor for all values.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_items
func TestItems(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("x", "1")
		s = reloaded(t, store, s)

		djangotest.AssertEqual(t, s.Data(), map[string]any{"x": "1"})
		djangotest.AssertEqual(t, s.Accessed(), true)
		djangotest.AssertEqual(t, s.Modified(), false)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_clear
func TestClear(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("x", "1")
		s = reloaded(t, store, s)
		djangotest.AssertEqual(t, s.Data(), map[string]any{"x": "1"})

		s = reloaded(t, store, s)
		s.Clear()
		djangotest.AssertEqual(t, s.Data(), map[string]any{})
		djangotest.AssertEqual(t, s.Accessed(), true)
		djangotest.AssertEqual(t, s.Modified(), true)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_save
func TestSave(t *testing.T) {
	eachKeyStore(t, func(t *testing.T, store keyStore) {
		s := store.New()
		save(t, store, s)
		djangotest.AssertEqual(t, exists(t, store, s.Key()), true)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_delete
func TestDelete(t *testing.T) {
	eachKeyStore(t, func(t *testing.T, store keyStore) {
		s := store.New()
		save(t, store, s)
		if err := store.Delete(context.Background(), s); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		djangotest.AssertEqual(t, exists(t, store, s.Key()), false)
	})
}

// Django's flush deletes the stored session at once. In Go the session does
// not know its store: Flush queues the key and the middleware removes the
// stored session at the end of the request, which the Delete call stands for
// here.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_flush
func TestFlush(t *testing.T) {
	eachKeyStore(t, func(t *testing.T, store keyStore) {
		s := store.New()
		s.Set("foo", "bar")
		save(t, store, s)
		prevKey := s.Key()

		s.Flush()
		if err := store.Delete(context.Background(), s); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		djangotest.AssertEqual(t, exists(t, store, prevKey), false)
		djangotest.AssertNotEqual(t, s.Key(), prevKey)
		djangotest.AssertEqual(t, s.Key(), "")
		djangotest.AssertEqual(t, s.Modified(), true)
		djangotest.AssertEqual(t, s.Accessed(), true)
	})
}

// Django's cycle_key stores the session under the new key at once. In Go the
// next save does that and deletes what was stored under the old key.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_cycle
func TestCycle(t *testing.T) {
	eachKeyStore(t, func(t *testing.T, store keyStore) {
		s := store.New()
		s.Set("a", "c")
		s.Set("b", "d")
		save(t, store, s)
		prevKey := s.Key()
		prevData := s.Data()

		s.CycleKey()
		save(t, store, s)
		djangotest.AssertEqual(t, exists(t, store, prevKey), false)
		djangotest.AssertEqual(t, exists(t, store, s.Key()), true)
		djangotest.AssertNotEqual(t, s.Key(), prevKey)
		djangotest.AssertEqual(t, s.Data(), prevData)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_cycle_with_no_session_cache
func TestCycleWithNoSessionCache(t *testing.T) {
	eachKeyStore(t, func(t *testing.T, store keyStore) {
		s := store.New()
		s.Set("a", "c")
		s.Set("b", "d")
		key := save(t, store, s)
		prevData := s.Data()

		s = load(t, store, key)
		s.CycleKey()
		djangotest.AssertEqual(t, s.Data(), prevData)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_save_doesnt_clear_data
func TestSaveDoesntClearData(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("a", "b")
		save(t, store, s)
		v, _ := s.Get("a")
		djangotest.AssertEqual(t, v, any("b"))
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_invalid_key
func TestInvalidKey(t *testing.T) {
	eachKeyStore(t, func(t *testing.T, store keyStore) {
		s := load(t, store, "1")
		save(t, store, s)
		djangotest.AssertNotEqual(t, s.Key(), "1")
		_, ok := s.Get("cat")
		djangotest.AssertEqual(t, ok, false)
		djangotest.AssertEqual(t, exists(t, store, "1"), false)
	})
}

// Django rejects the value in the setter of session_key. Go has no setter: the
// only way in is the cookie value, and an empty one yields a session without a
// key.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_session_key_empty_string_invalid
func TestSessionKeyEmptyStringInvalid(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		djangotest.AssertEqual(t, load(t, store, "").Key(), "")
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_session_key_too_short_invalid
func TestSessionKeyTooShortInvalid(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		djangotest.AssertEqual(t, load(t, store, "1234567").Key(), "")
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_default_expiry
func TestDefaultExpiry(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		djangotest.AssertEqual(t, s.GetExpiryAge(), sessions.DefaultCookieAge)

		s.SetExpiry(0)
		djangotest.AssertEqual(t, s.GetExpiryAge(), sessions.DefaultCookieAge)
	})
}

// assertExpiresIn checks that the expiry date of s lies d after a moment
// between before and now. Django passes a fixed modification time instead.
func assertExpiresIn(t *testing.T, s *sessions.Session, before time.Time, d time.Duration) {
	t.Helper()
	date := s.GetExpiryDate()
	if date.Before(before.Add(d)) || date.After(time.Now().Add(d)) {
		t.Errorf("GetExpiryDate() = %v, want %v after a time between %v and now", date, d, before)
	}
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_custom_expiry_seconds
func TestCustomExpirySeconds(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		before := time.Now()
		s := store.New()
		s.SetExpiry(10 * time.Second)

		assertExpiresIn(t, s, before, 10*time.Second)
		djangotest.AssertEqual(t, s.GetExpiryAge(), 10*time.Second)
	})
}

// Django turns a timedelta into a date. A Go duration is the same thing as a
// number of seconds, so the expiry stays relative to the last save.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_custom_expiry_timedelta
func TestCustomExpiryTimedelta(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		before := time.Now()
		s := store.New()
		s.SetExpiry(10 * time.Second)
		s = reloaded(t, store, s)

		assertExpiresIn(t, s, before, 10*time.Second)
		djangotest.AssertEqual(t, s.GetExpiryAge(), 10*time.Second)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_custom_expiry_datetime
func TestCustomExpiryDatetime(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		modification := time.Now()
		s := store.New()
		s.SetExpiryAt(modification.Add(10 * time.Second))
		s = reloaded(t, store, s)

		if date := s.GetExpiryDate(); !date.Equal(modification.Add(10 * time.Second)) {
			t.Errorf("GetExpiryDate() = %v, want %v", date, modification.Add(10*time.Second))
		}
		if age := s.GetExpiryAge(); age <= 9*time.Second || age > 10*time.Second {
			t.Errorf("GetExpiryAge() = %v, want just under 10s", age)
		}
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_custom_expiry_reset
func TestCustomExpiryReset(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.ClearExpiry()
		s.SetExpiry(10 * time.Second)
		s.ClearExpiry()
		djangotest.AssertEqual(t, s.GetExpiryAge(), sessions.DefaultCookieAge)
	})
}

// The setting is an option of the middleware in Go, so the session under test
// is the one the middleware hands to a handler.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_get_expire_at_browser_close
func TestGetExpireAtBrowserClose(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		for _, setting := range []bool{false, true} {
			s := sessionFromMiddleware(t, store, sessions.Options{ExpireAtBrowserClose: setting})

			s.SetExpiry(10 * time.Second)
			djangotest.AssertEqual(t, s.GetExpireAtBrowserClose(), false)

			s.SetExpiry(0)
			djangotest.AssertEqual(t, s.GetExpireAtBrowserClose(), true)

			s.ClearExpiry()
			djangotest.AssertEqual(t, s.GetExpireAtBrowserClose(), setting)
		}
	})
}

// Django tests encode and decode of the stored string. The Go stores expose
// the round trip of a whole session.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_decode
func TestDecode(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("a test key", "a test value")
		got := reloaded(t, store, s)
		djangotest.AssertEqual(t, got.Data(), map[string]any{"a test key": "a test value"})
	})
}

// Django also logs the failure to django.security.SuspiciousSession. Go has no
// such logger and only starts an empty session.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_decode_failure_logged_to_security
func TestDecodeFailure(t *testing.T) {
	t.Run("signed_cookies", func(t *testing.T) {
		store := sessions.NewSignedCookieStore([]byte("session-test-secret"))
		for _, value := range []string{"Zmxhc2tkajphbGtkamY=", "bad:encoded:value", "a.b.c", "..", ""} {
			s := load(t, store, value)
			djangotest.AssertEqual(t, s.Data(), map[string]any{})
			djangotest.AssertEqual(t, s.IsEmpty(), true)
		}
	})
	eachDBStore(t, func(t *testing.T, store *sessions.DBStore, db *orm.DB) {
		rec := sessions.Record{
			SessionKey:  "corruptedsessionkey",
			SessionData: "bad:encoded:value",
			ExpireDate:  time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		}
		if err := orm.Query[sessions.Record](db).Create(context.Background(), &rec); err != nil {
			t.Fatalf("Create: %v", err)
		}
		s := load(t, store, "corruptedsessionkey")
		djangotest.AssertEqual(t, s.Data(), map[string]any{})
		djangotest.AssertEqual(t, s.Key(), "")
	})
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		store, err := sessions.NewFileStore(dir, "sessionid")
		if err != nil {
			t.Fatalf("NewFileStore: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sessionidcorruptedsessionkey"), []byte("bad:encoded:value"), 0o600); err != nil {
			t.Fatal(err)
		}
		s := load(t, store, "corruptedsessionkey")
		djangotest.AssertEqual(t, s.Data(), map[string]any{})
		djangotest.AssertEqual(t, s.Key(), "")
	})
}

// Django marks this test as an expected failure for the signed-cookie backend,
// which ignores a custom expiry. The Go cookie store honors it.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_actual_expiry
func TestActualExpiry(t *testing.T) {
	eachStore(t, func(t *testing.T, store sessions.Store) {
		s := store.New()
		s.Set("foo", "bar")
		s.SetExpiry(-10 * time.Second)
		value := save(t, store, s)

		fresh := load(t, store, value)
		djangotest.AssertEqual(t, fresh.Has("foo"), false)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_session_load_does_not_create_record
func TestSessionLoadDoesNotCreateRecord(t *testing.T) {
	eachDBStore(t, func(t *testing.T, store *sessions.DBStore, db *orm.DB) {
		s := load(t, store, "someunknownkey")

		djangotest.AssertEqual(t, s.Key(), "")
		djangotest.AssertEqual(t, exists(t, store, "someunknownkey"), false)
		n, err := orm.Query[sessions.Record](db).Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(0))
	})
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		store, err := sessions.NewFileStore(dir, "sessionid")
		if err != nil {
			t.Fatalf("NewFileStore: %v", err)
		}
		s := load(t, store, "someunknownkey")

		djangotest.AssertEqual(t, s.Key(), "")
		djangotest.AssertEqual(t, exists(t, store, "someunknownkey"), false)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(entries), 0)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionTestsMixin.test_session_save_does_not_resurrect_session_logged_out_in_other_context
func TestSessionSaveDoesNotResurrectSessionLoggedOutInOtherContext(t *testing.T) {
	eachKeyStore(t, func(t *testing.T, store keyStore) {
		ctx := context.Background()
		s1 := store.New()
		s1.Set("test_data", "value1")
		key := save(t, store, s1)

		s2 := load(t, store, key)
		if err := store.Delete(ctx, s2); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		s1.Set("test_data", "value2")
		_, err := store.Encode(ctx, s1)
		djangotest.AssertErrorIs(t, err, sessions.ErrUpdate)

		djangotest.AssertEqual(t, load(t, store, key).Data(), map[string]any{})
	})
}

// Ported from Django: tests/sessions_tests/tests.py::DatabaseSessionTests.test_session_str
func TestSessionStr(t *testing.T) {
	eachDBStore(t, func(t *testing.T, store *sessions.DBStore, db *orm.DB) {
		s := store.New()
		s.Set("x", 1)
		key := save(t, store, s)

		rec, err := orm.Query[sessions.Record](db).Get(context.Background(), "session_key", key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, rec.String(), key)
	})
}

// A number comes back from JSON as a float64.
//
// Ported from Django: tests/sessions_tests/tests.py::DatabaseSessionTests.test_session_get_decoded
func TestSessionGetDecoded(t *testing.T) {
	eachDBStore(t, func(t *testing.T, store *sessions.DBStore, db *orm.DB) {
		s := store.New()
		s.Set("x", 1)
		key := save(t, store, s)

		rec, err := orm.Query[sessions.Record](db).Get(context.Background(), "session_key", key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		data, err := rec.GetDecoded()
		if err != nil {
			t.Fatalf("GetDecoded: %v", err)
		}
		djangotest.AssertEqual(t, data, map[string]any{"x": float64(1)})
	})
}

// The clearsessions command has its own test next to the command. This port
// covers what the command calls.
//
// Ported from Django: tests/sessions_tests/tests.py::DatabaseSessionTests.test_aclear_expired
func TestClearExpired(t *testing.T) {
	eachDBStore(t, func(t *testing.T, store *sessions.DBStore, db *orm.DB) {
		ctx := context.Background()
		count := func() int64 {
			n, err := orm.Query[sessions.Record](db).Count(ctx)
			if err != nil {
				t.Fatalf("Count: %v", err)
			}
			return n
		}
		djangotest.AssertEqual(t, count(), int64(0))

		future := store.New()
		future.Set("key", "value")
		future.SetExpiry(time.Hour)
		save(t, store, future)

		past := store.New()
		past.Set("key", "value")
		past.SetExpiry(-time.Hour)
		save(t, store, past)

		djangotest.AssertEqual(t, count(), int64(2))
		if err := store.ClearExpired(ctx); err != nil {
			t.Fatalf("ClearExpired: %v", err)
		}
		djangotest.AssertEqual(t, count(), int64(1))
		djangotest.AssertEqual(t, exists(t, store, future.Key()), true)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionBaseTests.test_test_cookie
func TestTestCookie(t *testing.T) {
	s := &sessions.Session{}
	djangotest.AssertEqual(t, s.Has("testcookie"), false)
	s.SetTestCookie()
	djangotest.AssertEqual(t, s.TestCookieWorked(), true)
	s.DeleteTestCookie()
	djangotest.AssertEqual(t, s.Has("testcookie"), false)
}

// Ported from Django: tests/sessions_tests/tests.py::SessionBaseTests.test_is_empty
func TestIsEmpty(t *testing.T) {
	djangotest.AssertEqual(t, (&sessions.Session{}).IsEmpty(), true)
}

// Go values have no truth value. IsEmpty is the counterpart of Django's
// __bool__, negated.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionBaseTests.test_bool
func TestBool(t *testing.T) {
	s := &sessions.Session{}
	djangotest.AssertEqual(t, s.IsEmpty(), true)

	s.Set("foo", "bar")
	djangotest.AssertEqual(t, s.IsEmpty(), false)

	keyed := &sessions.Session{}
	keyed.CycleKey()
	djangotest.AssertEqual(t, keyed.IsEmpty(), false)
}
