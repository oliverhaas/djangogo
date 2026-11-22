package sessions_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/sessions"
)

// interruptedText is the body Django's SessionInterrupted carries.
const interruptedText = "The request's session was deleted before the request completed. " +
	"The user may have logged out in a concurrent request, for example."

// exchange is one request through the session middleware.
type exchange struct {
	// rec holds the response.
	rec *httptest.ResponseRecorder
	// session is the session the handler saw.
	session *sessions.Session
}

// cookie returns the session cookie of the response, or nil.
func (e *exchange) cookie() *http.Cookie {
	for _, c := range e.rec.Result().Cookies() {
		if c.Name == sessions.DefaultCookieName {
			return c
		}
	}
	return nil
}

// serve sends a GET through the middleware. A non-empty cookieValue is sent as
// the session cookie. view runs with the session of the request.
func serve(
	t *testing.T, store sessions.Store, opts sessions.Options, cookieValue string,
	view func(w http.ResponseWriter, s *sessions.Session),
) *exchange {
	t.Helper()
	e := &exchange{rec: httptest.NewRecorder()}
	handler := sessions.MiddlewareWithOptions(store, opts)(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			s, ok := sessions.FromContext(r.Context())
			if !ok {
				t.Error("no session in the request context")
				return
			}
			e.session = s
			view(w, s)
		}))
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: sessions.DefaultCookieName, Value: cookieValue})
	}
	handler.ServeHTTP(e.rec, req)
	return e
}

// touchingSession is Django's get_response_touching_session.
func touchingSession(w http.ResponseWriter, s *sessions.Session) {
	s.Set("hello", "world")
	_, _ = w.Write([]byte("Session test"))
}

// sessionFromMiddleware returns the session the middleware hands to a handler
// under the given options.
func sessionFromMiddleware(t *testing.T, store sessions.Store, opts sessions.Options) *sessions.Session {
	t.Helper()
	return serve(t, store, opts, "", func(http.ResponseWriter, *sessions.Session) {}).session
}

// eachMiddlewareStore runs fn with the database store on every dialect.
func eachMiddlewareStore(t *testing.T, fn func(t *testing.T, store *sessions.DBStore, db *orm.DB)) {
	t.Helper()
	eachDBStore(t, fn)
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_secure_session_cookie
func TestSecureSessionCookie(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		e := serve(t, store, sessions.Options{CookieSecure: true}, "", touchingSession)
		djangotest.AssertEqual(t, e.cookie().Secure, true)
		djangotest.AssertEqual(t, e.rec.Header().Get("Vary"), "Cookie")
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_httponly_session_cookie
func TestHTTPOnlySessionCookie(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		e := serve(t, store, sessions.Options{}, "", touchingSession)
		djangotest.AssertEqual(t, e.cookie().HttpOnly, true)
		if raw := e.rec.Header().Get("Set-Cookie"); !strings.Contains(raw, "HttpOnly") {
			t.Errorf("Set-Cookie = %q, want the HttpOnly attribute", raw)
		}
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_samesite_session_cookie
func TestSameSiteSessionCookie(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		e := serve(t, store, sessions.Options{CookieSameSite: http.SameSiteStrictMode}, "", touchingSession)
		djangotest.AssertEqual(t, e.cookie().SameSite, http.SameSiteStrictMode)
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_no_httponly_session_cookie
func TestNoHTTPOnlySessionCookie(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		e := serve(t, store, sessions.Options{DisableHTTPOnly: true}, "", touchingSession)
		djangotest.AssertEqual(t, e.cookie().HttpOnly, false)
		if raw := e.rec.Header().Get("Set-Cookie"); strings.Contains(raw, "HttpOnly") {
			t.Errorf("Set-Cookie = %q, want no HttpOnly attribute", raw)
		}
	})
}

// assertNotSavedOn checks that a response with the status does not save the
// session.
func assertNotSavedOn(t *testing.T, status int) {
	t.Helper()
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, db *orm.DB) {
		e := serve(t, store, sessions.Options{}, "", func(w http.ResponseWriter, s *sessions.Session) {
			s.Set("hello", "world")
			w.WriteHeader(status)
			_, _ = w.Write([]byte("Horrible error"))
		})
		djangotest.AssertEqual(t, e.rec.Code, status)
		if c := e.cookie(); c != nil {
			t.Errorf("session cookie %v set on a %d response", c, status)
		}
		n, err := orm.Query[sessions.Record](db).Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(0))
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_session_save_on_500
func TestSessionSaveOn500(t *testing.T) {
	assertNotSavedOn(t, http.StatusInternalServerError)
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_session_save_on_5xx
func TestSessionSaveOn5xx(t *testing.T) {
	assertNotSavedOn(t, http.StatusServiceUnavailable)
}

// Django raises SessionInterrupted, which its handler turns into a 400. The Go
// middleware writes the 400 itself and drops the body of the handler.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_session_update_error_redirect
func TestSessionUpdateErrorRedirect(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		e := serve(t, store, sessions.Options{}, "", func(w http.ResponseWriter, s *sessions.Session) {
			ctx := context.Background()
			s.Set("hello", "world")
			if _, err := store.Encode(ctx, s); err != nil {
				t.Errorf("Encode: %v", err)
			}
			if err := store.Delete(ctx, s); err != nil {
				t.Errorf("Delete: %v", err)
			}
			_, _ = w.Write([]byte("body of the view"))
		})
		djangotest.AssertEqual(t, e.rec.Code, http.StatusBadRequest)
		djangotest.AssertEqual(t, strings.TrimSpace(e.rec.Body.String()), interruptedText)
		if c := e.cookie(); c != nil {
			t.Errorf("session cookie %v set for an interrupted session", c)
		}
	})
}

// flushing is Django's response_ending_session.
func flushing(w http.ResponseWriter, s *sessions.Session) {
	s.Flush()
	_, _ = w.Write([]byte("Session test"))
}

// net/http writes the attributes of a cookie in another order than Python and
// leaves out the quotes of the empty value. The attributes are the same.
//
// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_session_delete_on_end
func TestSessionDeleteOnEnd(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		e := serve(t, store, sessions.Options{}, "abc", flushing)

		djangotest.AssertEqual(t, e.rec.Header().Get("Set-Cookie"),
			"sessionid=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=0; SameSite=Lax")
		djangotest.AssertEqual(t, e.rec.Header().Get("Vary"), "Cookie")
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_session_delete_on_end_with_custom_domain_and_path
func TestSessionDeleteOnEndWithCustomDomainAndPath(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		opts := sessions.Options{CookieDomain: ".example.local", CookiePath: "/example/"}
		e := serve(t, store, opts, "abc", flushing)

		djangotest.AssertEqual(t, e.rec.Header().Get("Set-Cookie"),
			"sessionid=; Path=/example/; Domain=example.local; "+
				"Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=0; SameSite=Lax")
		djangotest.AssertEqual(t, e.rec.Header().Get("Vary"), "Cookie")
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_flush_empty_without_session_cookie_doesnt_set_cookie
func TestFlushEmptyWithoutSessionCookieDoesntSetCookie(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		e := serve(t, store, sessions.Options{}, "", flushing)

		djangotest.AssertEqual(t, len(e.rec.Result().Cookies()), 0)
		djangotest.AssertEqual(t, e.rec.Header().Get("Vary"), "Cookie")
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_save_every_request_with_non_empty_session_renews_session_cookie
func TestSaveEveryRequestWithNonEmptySessionRenewsSessionCookie(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		opts := sessions.Options{SaveEveryRequest: true}
		first := serve(t, store, opts, "", touchingSession)
		djangotest.AssertEqual(t, first.session.IsEmpty(), false)
		v, _ := first.session.Get("hello")
		djangotest.AssertEqual(t, v, any("world"))

		second := serve(t, store, opts, first.session.Key(), func(w http.ResponseWriter, _ *sessions.Session) {
			_, _ = w.Write([]byte("Session test"))
		})
		if second.cookie() == nil {
			t.Error("no session cookie although every request saves the session")
		}
		djangotest.AssertEqual(t, second.rec.Header().Get("Vary"), "Cookie")
	})
}

// Ported from Django: tests/sessions_tests/tests.py::SessionMiddlewareTests.test_empty_session_saved
func TestEmptySessionSaved(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, db *orm.DB) {
		first := serve(t, store, sessions.Options{}, "", func(w http.ResponseWriter, s *sessions.Session) {
			s.Set("foo", "bar")
			_, _ = w.Write([]byte("Session test"))
		})
		djangotest.AssertEqual(t, first.session.Data(), map[string]any{"foo": "bar"})
		key := first.session.Key()
		djangotest.AssertEqual(t, first.cookie().Value, key)
		djangotest.AssertEqual(t, first.rec.Header().Get("Vary"), "Cookie")

		second := serve(t, store, sessions.Options{}, key, func(w http.ResponseWriter, s *sessions.Session) {
			s.Delete("foo")
			_, _ = w.Write([]byte("Session test"))
		})
		djangotest.AssertEqual(t, second.session.Data(), map[string]any{})
		rec, err := orm.Query[sessions.Record](db).Get(context.Background(), "session_key", key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		data, err := rec.GetDecoded()
		if err != nil {
			t.Fatalf("GetDecoded: %v", err)
		}
		djangotest.AssertEqual(t, data, map[string]any{})
		if len(second.session.Key()) <= 8 {
			t.Errorf("session key %q is too short", second.session.Key())
		}
		djangotest.AssertEqual(t, second.cookie().Value, key)
		djangotest.AssertEqual(t, second.rec.Header().Get("Vary"), "Cookie")
	})
}

func TestMiddlewareCookieLifetime(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))

	t.Run("default age", func(t *testing.T) {
		c := serve(t, store, sessions.Options{}, "", touchingSession).cookie()
		djangotest.AssertEqual(t, c.MaxAge, int(sessions.DefaultCookieAge/time.Second))
		if until := time.Until(c.Expires); until < 13*24*time.Hour || until > sessions.DefaultCookieAge {
			t.Errorf("Expires in %v, want about two weeks", until)
		}
		djangotest.AssertEqual(t, c.Path, "/")
		djangotest.AssertEqual(t, c.SameSite, http.SameSiteLaxMode)
		djangotest.AssertEqual(t, c.Secure, false)
	})
	t.Run("custom age", func(t *testing.T) {
		c := serve(t, store, sessions.Options{CookieAge: time.Hour}, "", touchingSession).cookie()
		djangotest.AssertEqual(t, c.MaxAge, 3600)
	})
	t.Run("expire at browser close", func(t *testing.T) {
		c := serve(t, store, sessions.Options{ExpireAtBrowserClose: true}, "", touchingSession).cookie()
		djangotest.AssertEqual(t, c.MaxAge, 0)
		djangotest.AssertEqual(t, c.Expires.IsZero(), true)
	})
	t.Run("custom expiry of the session", func(t *testing.T) {
		c := serve(t, store, sessions.Options{}, "", func(w http.ResponseWriter, s *sessions.Session) {
			s.SetExpiry(90 * time.Second)
			_, _ = w.Write([]byte("ok"))
		}).cookie()
		djangotest.AssertEqual(t, c.MaxAge, 90)
	})
	t.Run("no samesite attribute", func(t *testing.T) {
		e := serve(t, store, sessions.Options{CookieSameSite: http.SameSiteDefaultMode}, "", touchingSession)
		if raw := e.rec.Header().Get("Set-Cookie"); strings.Contains(raw, "SameSite") {
			t.Errorf("Set-Cookie = %q, want no SameSite attribute", raw)
		}
	})
}

func TestMiddlewareUntouchedSessionSendsNothing(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))
	e := serve(t, store, sessions.Options{}, "", func(w http.ResponseWriter, _ *sessions.Session) {
		_, _ = w.Write([]byte("ok"))
	})
	djangotest.AssertEqual(t, len(e.rec.Result().Cookies()), 0)
	djangotest.AssertEqual(t, e.rec.Header().Get("Vary"), "")
}

func TestMiddlewareKeepsExistingVary(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))
	e := serve(t, store, sessions.Options{}, "", func(w http.ResponseWriter, s *sessions.Session) {
		w.Header().Set("Vary", "Accept-Encoding, cookie")
		s.Set("a", "b")
		_, _ = w.Write([]byte("ok"))
	})
	djangotest.AssertEqual(t, e.rec.Header().Values("Vary"), []string{"Accept-Encoding, cookie"})
}

// A panic after Flush must not leave the old session row in the store: the
// user asked to be logged out.
func TestMiddlewareDropsFlushedSessionOnPanic(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		first := serve(t, store, sessions.Options{}, "", touchingSession)
		key := first.session.Key()
		djangotest.AssertEqual(t, exists(t, store, key), true)

		func() {
			defer func() { _ = recover() }()
			serve(t, store, sessions.Options{}, key, func(http.ResponseWriter, *sessions.Session) {
				panic("view failed")
			})
		}()
		djangotest.AssertEqual(t, exists(t, store, key), true)

		func() {
			defer func() { _ = recover() }()
			serve(t, store, sessions.Options{}, key, func(_ http.ResponseWriter, s *sessions.Session) {
				s.Flush()
				panic("view failed after logout")
			})
		}()
		djangotest.AssertEqual(t, exists(t, store, key), false)
	})
}

func TestMiddlewareCycleKeyReplacesRow(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		first := serve(t, store, sessions.Options{}, "", touchingSession)
		oldKey := first.session.Key()

		second := serve(t, store, sessions.Options{}, oldKey, func(w http.ResponseWriter, s *sessions.Session) {
			s.CycleKey()
			_, _ = w.Write([]byte("ok"))
		})
		newKey := second.cookie().Value
		djangotest.AssertNotEqual(t, newKey, oldKey)
		djangotest.AssertEqual(t, exists(t, store, oldKey), false)
		djangotest.AssertEqual(t, exists(t, store, newKey), true)
		djangotest.AssertEqual(t, load(t, store, newKey).Data(), map[string]any{"hello": "world"})
	})
}

// countingStore counts the Decode calls of the store it wraps. When err is set,
// Decode fails with it.
type countingStore struct {
	sessions.Store
	// decodes is the number of Decode calls.
	decodes int
	// err is the error of Decode, or nil.
	err error
}

// Decode counts the call and fails with s.err when it is set.
func (s *countingStore) Decode(ctx context.Context, cookieValue string) (*sessions.Session, error) {
	s.decodes++
	if s.err != nil {
		return nil, s.err
	}
	return s.Store.Decode(ctx, cookieValue)
}

// The middleware reads the session from the store when the handler first uses
// it, as Django loads the session cache on first access.
//
// No Django test exists: Django's tests do not count the loads of the
// middleware.
func TestMiddlewareLoadsSessionOnFirstUse(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, db *sessions.DBStore, _ *orm.DB) {
		key := serve(t, db, sessions.Options{}, "", touchingSession).session.Key()
		store := &countingStore{Store: db}

		untouched := serve(t, store, sessions.Options{}, key, func(w http.ResponseWriter, _ *sessions.Session) {
			_, _ = w.Write([]byte("ok"))
		})
		djangotest.AssertEqual(t, store.decodes, 0)
		djangotest.AssertEqual(t, len(untouched.rec.Result().Cookies()), 0)
		djangotest.AssertEqual(t, untouched.rec.Header().Get("Vary"), "")

		read := serve(t, store, sessions.Options{}, key, func(w http.ResponseWriter, s *sessions.Session) {
			v, _ := s.Get("hello")
			djangotest.AssertEqual(t, v, any("world"))
			djangotest.AssertEqual(t, s.Has("hello"), true)
			_, _ = w.Write([]byte("ok"))
		})
		djangotest.AssertEqual(t, store.decodes, 1)
		djangotest.AssertEqual(t, read.rec.Header().Get("Vary"), "Cookie")
	})
}

// A session that the handler only marks modified is read from the store
// before the save, which keeps its values, as Django's save() loads them.
//
// No Django test exists: Django's tests set modified only on sessions they
// read.
func TestMiddlewareSavesUnreadModifiedSession(t *testing.T) {
	eachMiddlewareStore(t, func(t *testing.T, store *sessions.DBStore, _ *orm.DB) {
		key := serve(t, store, sessions.Options{}, "", touchingSession).session.Key()

		e := serve(t, store, sessions.Options{}, key, func(w http.ResponseWriter, s *sessions.Session) {
			s.SetModified(true)
			_, _ = w.Write([]byte("ok"))
		})
		djangotest.AssertEqual(t, e.cookie().Value, key)
		djangotest.AssertEqual(t, load(t, store, key).Data(), map[string]any{"hello": "world"})
	})
}

// A store that cannot load the session fails only the requests that use the
// session. The handler sees an empty session.
//
// No Django test exists: Django raises the error of the store in the view.
func TestMiddlewareLoadError(t *testing.T) {
	store := &countingStore{Store: sessions.NewSignedCookieStore([]byte("secret")), err: errors.New("store down")}
	untouched := serve(t, store, sessions.Options{}, "value", func(w http.ResponseWriter, _ *sessions.Session) {
		_, _ = w.Write([]byte("ok"))
	})
	djangotest.AssertEqual(t, untouched.rec.Code, http.StatusOK)
	djangotest.AssertEqual(t, untouched.rec.Body.String(), "ok")

	read := serve(t, store, sessions.Options{}, "value", func(w http.ResponseWriter, s *sessions.Session) {
		_, ok := s.Get("hello")
		djangotest.AssertEqual(t, ok, false)
		_, _ = w.Write([]byte("ok"))
	})
	djangotest.AssertEqual(t, read.rec.Code, http.StatusInternalServerError)
	djangotest.AssertEqual(t, read.rec.Body.String(), "session error\n")

	everyRequest := serve(t, store, sessions.Options{SaveEveryRequest: true}, "value",
		func(w http.ResponseWriter, _ *sessions.Session) {
			_, _ = w.Write([]byte("ok"))
		})
	djangotest.AssertEqual(t, everyRequest.rec.Code, http.StatusInternalServerError)
}
