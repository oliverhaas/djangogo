package csrf_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/sessions"
)

// chain builds sessions.Middleware(store, "sessionid")(csrf.Middleware(h)).
func chain(store *sessions.SignedCookieStore, h http.Handler) http.Handler {
	return sessions.Middleware(store, "sessionid")(csrf.Middleware(h))
}

// TestGETSeedsToken verifies that a GET request causes a CSRF token to be
// bootstrapped, visible to the handler via csrf.Token, and persisted in the
// session cookie.
func TestGETSeedsToken(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("test-secret"))

	var gotToken string
	h := chain(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = csrf.Token(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if gotToken == "" {
		t.Fatal("Token(ctx) returned empty string on GET")
	}

	// A Set-Cookie header must be present carrying the session.
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	var sessionCookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "sessionid" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("GET did not set a session cookie")
	}
}

// TestPOSTNoToken verifies that a POST with no token is rejected with 403 and
// the downstream handler is not reached.
func TestPOSTNoToken(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("test-secret"))

	reached := false
	h := chain(store, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if reached {
		t.Fatal("handler was reached despite missing CSRF token")
	}
}

// TestPOSTWrongToken verifies that a POST with an incorrect token is rejected
// with 403 and the downstream handler is not reached.
func TestPOSTWrongToken(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("test-secret"))

	// Seed a session with a known token.
	sess := store.New()
	sess.Set(csrf.SessionKey, "correct-token")
	cookieVal, err := store.Encode(context.Background(), sess)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	reached := false
	h := chain(store, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	body := url.Values{csrf.FormField: {"wrong-token"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "sessionid", Value: cookieVal})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if reached {
		t.Fatal("handler was reached despite wrong CSRF token")
	}
}

// TestPOSTCorrectToken verifies that a POST carrying the correct token in the
// form field is accepted (200) and the handler is reached.
func TestPOSTCorrectToken(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("test-secret"))

	// Step 1: GET to obtain a session cookie and the seeded token.
	var seedToken string
	h := chain(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seedToken = csrf.Token(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec1, req1)

	if seedToken == "" {
		t.Fatal("GET did not seed a token")
	}
	res1 := rec1.Result()
	defer func() { _ = res1.Body.Close() }()
	var sessionCookie *http.Cookie
	for _, c := range res1.Cookies() {
		if c.Name == "sessionid" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("GET did not set a session cookie")
	}

	// Step 2: POST with the correct token and the session cookie.
	reached := false
	h2 := chain(store, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	rec2 := httptest.NewRecorder()
	body := url.Values{csrf.FormField: {seedToken}}.Encode()
	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(sessionCookie)
	h2.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	if !reached {
		t.Fatal("handler was not reached despite correct CSRF token")
	}
}

// TestHeaderTokenPath verifies that the X-CSRFToken header is accepted as an
// alternative to the form field.
func TestHeaderTokenPath(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("test-secret"))

	// GET to seed a token and obtain the session cookie.
	var seedToken string
	h := chain(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seedToken = csrf.Token(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec1, req1)

	res1 := rec1.Result()
	defer func() { _ = res1.Body.Close() }()
	var sessionCookie *http.Cookie
	for _, c := range res1.Cookies() {
		if c.Name == "sessionid" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil || seedToken == "" {
		t.Fatal("GET setup failed")
	}

	// POST with token in header, no form body.
	reached := false
	h2 := chain(store, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set(csrf.HeaderName, seedToken)
	req2.AddCookie(sessionCookie)
	h2.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 via header path, got %d", rec2.Code)
	}
	if !reached {
		t.Fatal("handler not reached via header token path")
	}
}

// TestSafeMethodsBypass verifies that HEAD and OPTIONS are never challenged.
func TestSafeMethodsBypass(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("test-secret"))

	for _, method := range []string{http.MethodHead, http.MethodOptions} {
		reached := false
		h := chain(store, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/", nil)
		h.ServeHTTP(rec, req)

		if rec.Code == http.StatusForbidden {
			t.Errorf("method %s got 403; safe methods must bypass CSRF", method)
		}
		if !reached {
			t.Errorf("method %s: handler not reached", method)
		}
	}
}

// TestCookieJarFlow runs a full GET-then-POST cycle via an httptest.Server and
// a cookie jar to exercise realistic cookie handling.
func TestCookieJarFlow(t *testing.T) {
	store := sessions.NewSignedCookieStore([]byte("jar-secret"))

	var capturedToken string
	handler := chain(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			capturedToken = csrf.Token(r.Context())
		}
		w.WriteHeader(http.StatusOK)
	}))

	srv := httptest.NewServer(handler)
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	// GET: seeds the token.
	resp, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()

	if capturedToken == "" {
		t.Fatal("GET did not capture a token")
	}

	// POST: submit the correct token via the form field.
	body := url.Values{csrf.FormField: {capturedToken}}.Encode()
	resp2, err := client.Post(srv.URL+"/", "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST with correct token: expected 200, got %d", resp2.StatusCode)
	}
}
