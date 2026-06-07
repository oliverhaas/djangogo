package sessions_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oliverhaas/djangogo/sessions"
)

const testCookieName = "sessionid"

func TestMiddlewareSetsAndReadsCookie(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))
	mw := sessions.Middleware(store, testCookieName)

	// h1 sets a value and writes 200; the response must carry a Set-Cookie.
	h1 := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := sessions.FromContext(r.Context())
		if !ok {
			t.Error("h1: no session in context")
		}
		s.Set("uid", float64(7))
		w.WriteHeader(http.StatusOK)
	}))

	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	h1.ServeHTTP(rec1, req1)

	res1 := rec1.Result()
	defer func() { _ = res1.Body.Close() }()
	cookies := res1.Cookies()
	if len(cookies) == 0 {
		t.Fatal("h1 did not set a session cookie")
	}
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == testCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no cookie named sessionid was set")
	}
	if !sessionCookie.HttpOnly {
		t.Error("cookie is not HttpOnly")
	}
	if sessionCookie.Path != "/" {
		t.Errorf("cookie path = %q; want /", sessionCookie.Path)
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v; want Lax", sessionCookie.SameSite)
	}

	// h2 reads the value back from a second request carrying that cookie.
	var seen any
	var sawSession bool
	h2 := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := sessions.FromContext(r.Context())
		sawSession = ok
		if ok {
			seen, _ = s.Get("uid")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(sessionCookie)
	h2.ServeHTTP(rec2, req2)

	if !sawSession {
		t.Fatal("h2 had no session in context")
	}
	if seen != float64(7) {
		t.Fatalf("h2 read uid = %v; want 7", seen)
	}
}

func TestMiddlewareNoCookieWhenUnmodified(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))
	mw := sessions.Middleware(store, testCookieName)

	// Handler reads but never modifies the session.
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessions.FromContext(r.Context()); !ok {
			t.Error("no session in context")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	for _, c := range res.Cookies() {
		if c.Name == testCookieName {
			t.Fatalf("a cookie was set despite no session modification: %v", c)
		}
	}
}

func TestMiddlewareNeverWritesStillSaves(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("secret"))
	mw := sessions.Middleware(store, testCookieName)

	// Handler modifies the session but never calls Write/WriteHeader.
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		s, _ := sessions.FromContext(r.Context())
		s.Set("uid", float64(9))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	found := false
	for _, c := range res.Cookies() {
		if c.Name == testCookieName {
			found = true
		}
	}
	if !found {
		t.Fatal("session modified but no cookie set when handler never wrote")
	}
}
