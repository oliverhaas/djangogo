package csrf_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/sessions"
)

// Secrets of Django's test suite: one secret and two masked forms of it.
const (
	testSecret        = "lcccccccX2kcccccccY2jcccccccssIC"
	maskedTestSecret1 = "1bcdefghij2bcdefghij3bcdefghij4bcdefghij5bcdefghij6bcdefghijABCD"
	maskedTestSecret2 = "2JgchWvM1tpxT2lfz9aydoXW9yT1DN3NdLiejYxOOlzzV4nhBbYqmqZYbAV3V5Bf"
)

// sessionCookie is the name of the session cookie in session mode.
const sessionCookie = "sessionid"

// harness runs requests through the CSRF middleware, with the secret in a
// cookie or in the session. It stands for Django's CsrfViewMiddlewareTestMixin
// and its two subclasses.
type harness struct {
	t *testing.T
	// useSessions selects where the secret lives.
	useSessions bool
	// opts are the options of the middleware. UseSessions and FailureHandler
	// are set by the harness.
	opts csrf.Options
	// store holds the sessions in session mode.
	store *sessions.SignedCookieStore
}

// eachMode runs fn for cookie-based and for session-based secrets.
func eachMode(t *testing.T, fn func(t *testing.T, h *harness)) {
	t.Helper()
	t.Run("cookie", func(t *testing.T) {
		fn(t, &harness{t: t})
	})
	t.Run("session", func(t *testing.T) {
		fn(t, sessionMode(t))
	})
}

// cookieMode returns a harness for cookie-based secrets.
func cookieMode(t *testing.T) *harness {
	t.Helper()
	return &harness{t: t}
}

// request describes a request of a test.
type request struct {
	// method defaults to GET.
	method string
	// secret is the secret the browser holds, in the cookie or the session.
	// nil means there is none.
	secret *string
	// postToken is sent as the form field when not empty.
	postToken string
	// headerToken is sent in the header named headerName, which defaults to
	// X-CSRFToken. nil means no header.
	headerToken *string
	headerName  string
	// secure marks the request as received over HTTPS.
	secure bool
	// host overrides the Host header.
	host string
	// origin and referer are sent as headers when not nil.
	origin, referer *string
	// brokenBody makes reading the body fail.
	brokenBody bool
}

// str returns a pointer to s, for the optional fields of request.
func str(s string) *string { return &s }

// get is Django's _get_request: a GET without a secret.
func get() request { return request{method: http.MethodGet} }

// withSecret is Django's _get_csrf_cookie_request: the default test secret and
// no token.
func withSecret(method string) request {
	return request{method: method, secret: str(maskedTestSecret1)}
}

// postWithToken is Django's _get_POST_request_with_token.
func postWithToken() request {
	return request{method: http.MethodPost, secret: str(maskedTestSecret1), postToken: maskedTestSecret2}
}

// result is the outcome of one request.
type result struct {
	// rec holds the response.
	rec *httptest.ResponseRecorder
	// reason is the reason of the rejection, or "" when the view ran.
	reason string
	// called reports that the view ran.
	called bool
	// secret is the secret the browser holds afterwards, and sent reports
	// that the response changed or renewed it.
	secret string
	sent   bool
}

// failingReader fails every read.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// do sends q through the middleware to view.
func (h *harness) do(q request, view http.Handler) *result {
	h.t.Helper()
	res := &result{rec: httptest.NewRecorder()}
	opts := h.opts
	opts.UseSessions = h.useSessions
	opts.FailureHandler = func(w http.ResponseWriter, _ *http.Request, reason string) {
		res.reason = reason
		http.Error(w, reason, http.StatusForbidden)
	}
	handler := csrf.MiddlewareWithOptions(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res.called = true
		view.ServeHTTP(w, r)
	}))
	if h.useSessions {
		handler = sessions.Middleware(h.store, sessionCookie)(handler)
	}

	if q.method == "" {
		q.method = http.MethodGet
	}
	var body io.Reader = http.NoBody
	switch {
	case q.brokenBody:
		body = failingReader{}
	case q.postToken != "":
		body = strings.NewReader(url.Values{csrf.FormField: {q.postToken}}.Encode())
	}
	req := httptest.NewRequest(q.method, "/", body)
	if q.method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if q.host != "" {
		req.Host = q.host
	}
	if q.secure {
		req.TLS = &tls.ConnectionState{}
	}
	if q.headerToken != nil {
		name := q.headerName
		if name == "" {
			name = csrf.HeaderName
		}
		req.Header.Set(name, *q.headerToken)
	}
	if q.origin != nil {
		req.Header.Set("Origin", *q.origin)
	}
	if q.referer != nil {
		req.Header.Set("Referer", *q.referer)
	}
	if q.secret != nil {
		res.secret = *q.secret
		req.Header.Add("Cookie", h.secretCookie(*q.secret))
	}

	handler.ServeHTTP(res.rec, req)
	h.readSecret(res)
	return res
}

// cookieName returns the name of the CSRF cookie under the options.
func (h *harness) cookieName() string {
	if h.opts.CookieName != "" {
		return h.opts.CookieName
	}
	return csrf.DefaultCookieName
}

// secretCookie returns the Cookie header that gives the browser the secret.
func (h *harness) secretCookie(secret string) string {
	h.t.Helper()
	if !h.useSessions {
		return h.cookieName() + "=" + secret
	}
	sess := h.store.New()
	sess.Set(csrf.SessionKey, secret)
	value, err := h.store.Encode(context.Background(), sess)
	if err != nil {
		h.t.Fatalf("Encode: %v", err)
	}
	return sessionCookie + "=" + value
}

// readSecret fills in the secret the browser holds after the response
// (Django's _read_csrf_cookie).
func (h *harness) readSecret(res *result) {
	h.t.Helper()
	name := h.cookieName()
	if h.useSessions {
		name = sessionCookie
	}
	for _, c := range res.rec.Result().Cookies() {
		if c.Name != name {
			continue
		}
		res.sent = true
		res.secret = c.Value
		if h.useSessions {
			sess, err := h.store.Decode(context.Background(), c.Value)
			if err != nil {
				h.t.Fatalf("Decode: %v", err)
			}
			v, _ := sess.Get(csrf.SessionKey)
			res.secret, _ = v.(string)
		}
	}
}

// csrfCookie returns the CSRF cookie of the response, or nil.
func (res *result) csrfCookie(name string) *http.Cookie {
	for _, c := range res.rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// postFormView is the view of Django's tests that needs no token.
var postFormView = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, "OK")
})

// tokenView renders the hidden form field with a token, as the csrf_token
// template tag does.
var tokenView = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, `<input type="hidden" name="%s" value="%s">`, csrf.FormField, csrf.GetToken(r))
})

// tokenPattern finds the token tokenView rendered.
var tokenPattern = regexp.MustCompile(`name="csrfmiddlewaretoken" value="(.*?)"`)

// assertMaskedSecretCorrect is Django's assertMaskedSecretCorrect.
func assertMaskedSecretCorrect(t *testing.T, masked, secret string) {
	t.Helper()
	if len(masked) != csrf.TokenLength {
		t.Fatalf("token %q has %d characters, want %d", masked, len(masked), csrf.TokenLength)
	}
	if len(secret) != csrf.SecretLength {
		t.Fatalf("secret %q has %d characters, want %d", secret, len(secret), csrf.SecretLength)
	}
	if reason := csrf.CheckFormat(masked); reason != "" {
		t.Fatalf("token %q %s", masked, reason)
	}
	if got := csrf.UnmaskToken(masked); got != secret {
		t.Errorf("token %q unmasks to %q, want %q", masked, got, secret)
	}
}

// checkTokenPresent is Django's _check_token_present: the page holds a token
// for secret.
func checkTokenPresent(t *testing.T, res *result, secret string) {
	t.Helper()
	m := tokenPattern.FindStringSubmatch(res.rec.Body.String())
	if m == nil {
		t.Fatalf("could not find a csrfmiddlewaretoken value in: %s", res.rec.Body.String())
	}
	assertMaskedSecretCorrect(t, m[1], secret)
}

// assertAccepted checks that the view ran.
func assertAccepted(t *testing.T, res *result) {
	t.Helper()
	if res.reason != "" || !res.called {
		t.Errorf("request was rejected: %q (status %d)", res.reason, res.rec.Code)
	}
}

// assertForbiddenReason is Django's assertForbiddenReason without the log
// record, which Go does not write.
func assertForbiddenReason(t *testing.T, res *result, reason string) {
	t.Helper()
	if res.reason != reason {
		t.Errorf("reason = %q, want %q", res.reason, reason)
	}
	if res.rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.rec.Code)
	}
	if res.called {
		t.Error("the view ran for a rejected request")
	}
}
