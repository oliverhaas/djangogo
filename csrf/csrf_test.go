package csrf_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oliverhaas/djangogo/csrf"
	"github.com/oliverhaas/djangogo/templates"
)

// serveDefault sends req through csrf.Middleware to view.
func serveDefault(req *http.Request, view http.Handler) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	csrf.Middleware(view).ServeHTTP(rec, req)
	return rec
}

func TestMiddlewareUsesDjangoCookieDefaults(t *testing.T) {
	rec := serveDefault(httptest.NewRequest(http.MethodGet, "/", http.NoBody), tokenView)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != "csrftoken" || c.Path != "/" || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = %s, want csrftoken with Path=/ and SameSite=Lax", c)
	}
	if c.MaxAge != 31449600 || c.Expires.IsZero() {
		t.Errorf("Max-Age = %d, Expires = %v, want 52 weeks", c.MaxAge, c.Expires)
	}
	if c.Secure || c.HttpOnly || c.Domain != "" {
		t.Errorf("cookie = %s, want no Secure, HttpOnly or Domain", c)
	}
	if got := rec.Header().Get("Vary"); got != "Cookie" {
		t.Errorf("Vary = %q, want Cookie", got)
	}
}

func TestDefaultFailureResponseHidesReason(t *testing.T) {
	rec := serveDefault(httptest.NewRequest(http.MethodPost, "/", http.NoBody), postFormView)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "CSRF verification failed. Request aborted.") {
		t.Errorf("body = %q, want Django's headline", body)
	}
	if strings.Contains(body, csrf.ReasonNoCSRFCookie) {
		t.Errorf("body = %q, want the reason left out", body)
	}
}

func TestSafeMethodsPassWithoutSecret(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
		rec := serveDefault(httptest.NewRequest(method, "/", http.NoBody), postFormView)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", method, rec.Code)
		}
	}
}

func TestUnsafeMethodsAreChecked(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, "PURGE"} {
		rec := serveDefault(httptest.NewRequest(method, "/", http.NoBody), postFormView)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", method, rec.Code)
		}
	}
}

func TestWithoutEnforcementSkipsChecksButKeepsTokens(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.Header.Set("Origin", "https://www.evil.org")
	req = req.WithContext(csrf.WithoutEnforcement(req.Context()))
	rec := serveDefault(req, tokenView)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !tokenPattern.MatchString(rec.Body.String()) || len(rec.Result().Cookies()) != 1 {
		t.Errorf("body = %q with %d cookies, want a token and its cookie", rec.Body.String(), len(rec.Result().Cookies()))
	}
}

func TestExemptWithoutResolveIsChecked(t *testing.T) {
	rec := serveDefault(httptest.NewRequest(http.MethodPost, "/", http.NoBody), csrf.Exempt(postFormView))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403: the middleware cannot see the mark without Options.Resolve", rec.Code)
	}
}

func TestExemptFollowsTheResolvedHandler(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/hook/", csrf.Exempt(postFormView))
	mux.Handle("/form/", postFormView)
	handler := csrf.MiddlewareWithOptions(csrf.Options{
		Resolve: func(r *http.Request) http.Handler {
			h, _ := mux.Handler(r)
			return h
		},
	})(mux)

	for path, want := range map[string]int{"/hook/": http.StatusOK, "/form/": http.StatusForbidden} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, http.NoBody))
		if rec.Code != want {
			t.Errorf("POST %s: status = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestIsExempt(t *testing.T) {
	if csrf.IsExempt(postFormView) {
		t.Error("IsExempt(plain handler) = true")
	}
	if !csrf.IsExempt(csrf.Exempt(postFormView)) {
		t.Error("IsExempt(Exempt(handler)) = false")
	}
	if csrf.IsExempt(nil) {
		t.Error("IsExempt(nil) = true")
	}
	// A wrapper around the mark hides it, so the request is checked.
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { csrf.Exempt(postFormView).ServeHTTP(w, r) })
	if csrf.IsExempt(wrapped) {
		t.Error("IsExempt(wrapper around Exempt) = true")
	}
	// A wrapper with an Unwrap method shows the mark.
	if !csrf.IsExempt(unwrapper{csrf.Exempt(postFormView)}) {
		t.Error("IsExempt(unwrapper around Exempt) = false")
	}
}

// unwrapper is a wrapper that shows the handler it wraps.
type unwrapper struct{ http.Handler }

func (u unwrapper) Unwrap() http.Handler { return u.Handler }

func TestIsSecureOptionDecidesTheScheme(t *testing.T) {
	behindProxy := func(r *http.Request) bool { return r.Header.Get("X-Forwarded-Proto") == "https" }
	post := func(opts csrf.Options) int {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(csrf.FormField+"="+maskedTestSecret2))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", "csrftoken="+testSecret)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		csrf.MiddlewareWithOptions(opts)(postFormView).ServeHTTP(rec, req)
		return rec.Code
	}

	if got := post(csrf.Options{}); got != http.StatusForbidden {
		t.Errorf("without IsSecure: status = %d, want 403 for an https origin on a plain connection", got)
	}
	if got := post(csrf.Options{IsSecure: behindProxy}); got != http.StatusOK {
		t.Errorf("with IsSecure: status = %d, want 200", got)
	}
}

func TestTrustedOriginWithoutSchemeIsIgnored(t *testing.T) {
	h := cookieMode(t)
	h.opts.TrustedOrigins = []string{"www.evil.org", "*.evil.org"}
	q := postWithToken()
	q.origin = str("https://www.evil.org")
	assertForbiddenReason(t, h.do(q, postFormView),
		"Origin checking failed - https://www.evil.org does not match any trusted origins.")
}

func TestWildcardDoesNotTrustLookalikeDomains(t *testing.T) {
	h := cookieMode(t)
	h.opts.TrustedOrigins = []string{"https://*.example.com"}
	for _, origin := range []string{
		"https://evilexample.com",
		"https://example.com.evil.org",
		"https://foo.example.com.evil.org",
		"https://foo.example.com@evil.org",
	} {
		q := securePost()
		q.host, q.origin = "www.example.com", &origin
		if res := h.do(q, postFormView); res.called {
			t.Errorf("origin %q was accepted", origin)
		}
	}
}

func TestRefererWithUserInfoIsRejected(t *testing.T) {
	h := cookieMode(t)
	q := securePost()
	q.host, q.referer = "www.example.com", str("https://www.example.com@www.evil.org/")
	if res := h.do(q, postFormView); res.called {
		t.Error("a referer with user information was accepted")
	}
	q.referer = str("https://www.evil.org@www.example.com/")
	if res := h.do(q, postFormView); res.called {
		t.Error("a referer with user information was accepted")
	}
}

func TestRejectedRequestStillReplacesInvalidCookie(t *testing.T) {
	h := cookieMode(t)
	res := h.do(request{method: http.MethodPost, secret: str("badvalue")}, postFormView)
	assertForbiddenReason(t, res, "CSRF cookie has incorrect length.")
	if !res.sent || len(res.secret) != csrf.SecretLength {
		t.Errorf("cookie after the response = %q (sent = %v), want a new secret", res.secret, res.sent)
	}
}

func TestTokenWithoutMiddlewareIsEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if got := csrf.Token(context.Background()); got != "" {
		t.Errorf("Token = %q, want empty", got)
	}
	if got := csrf.GetToken(req); got != "" {
		t.Errorf("GetToken = %q, want empty", got)
	}
	csrf.RotateToken(req)
}

func TestTokenDiffersPerCallForOneSecret(t *testing.T) {
	var first, second string
	serveDefault(httptest.NewRequest(http.MethodGet, "/", http.NoBody),
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			first, second = csrf.GetToken(r), csrf.Token(r.Context())
		}))
	if first == second {
		t.Errorf("both tokens are %q, want a fresh mask per call", first)
	}
	if csrf.UnmaskToken(first) != csrf.UnmaskToken(second) {
		t.Error("the two tokens belong to different secrets")
	}
}

func TestNewSecretsAreRandomAndWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for range 50 {
		rec := serveDefault(httptest.NewRequest(http.MethodGet, "/", http.NoBody), tokenView)
		secret := rec.Result().Cookies()[0].Value
		if len(secret) != csrf.SecretLength || strings.Trim(secret, csrf.AllowedChars) != "" {
			t.Fatalf("secret %q is not %d characters of the alphabet", secret, csrf.SecretLength)
		}
		if seen[secret] {
			t.Fatalf("secret %q was issued twice", secret)
		}
		seen[secret] = true
	}
}

func TestVaryIsNotRepeated(t *testing.T) {
	rec := serveDefault(httptest.NewRequest(http.MethodGet, "/", http.NoBody),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Vary", "Accept-Encoding, cookie")
			_, _ = io.WriteString(w, csrf.GetToken(r))
		}))
	if got := rec.Header().Values("Vary"); len(got) != 1 || got[0] != "Accept-Encoding, cookie" {
		t.Errorf("Vary = %q, want the header of the view unchanged", got)
	}
}

func TestCookieIsSentBeforeAnExplicitStatusAndOnFlush(t *testing.T) {
	views := map[string]http.HandlerFunc{
		"WriteHeader": func(w http.ResponseWriter, r *http.Request) {
			_ = csrf.GetToken(r)
			w.WriteHeader(http.StatusCreated)
		},
		"Flush": func(w http.ResponseWriter, r *http.Request) {
			_ = csrf.GetToken(r)
			_ = http.NewResponseController(w).Flush()
		},
		"NoWrite": func(_ http.ResponseWriter, r *http.Request) {
			_ = csrf.GetToken(r)
		},
	}
	for name, view := range views {
		rec := serveDefault(httptest.NewRequest(http.MethodGet, "/", http.NoBody), view)
		if len(rec.Result().Cookies()) != 1 {
			t.Errorf("%s: got %d cookies, want 1", name, len(rec.Result().Cookies()))
		}
	}
}

func TestSameSiteDefaultModeOmitsTheAttribute(t *testing.T) {
	h := cookieMode(t)
	h.opts.CookieSameSite = http.SameSiteDefaultMode
	res := h.do(get(), tokenView)
	if raw := res.rec.Header().Get("Set-Cookie"); raw == "" || strings.Contains(raw, "SameSite") {
		t.Errorf("Set-Cookie = %q, want a cookie without SameSite", raw)
	}
}

func TestFailureHandlerMayAskForAToken(t *testing.T) {
	handler := csrf.MiddlewareWithOptions(csrf.Options{
		FailureHandler: func(w http.ResponseWriter, r *http.Request, reason string) {
			token := csrf.GetToken(r)
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprintf(w, "%s token=%s", reason, token)
		},
	})(postFormView)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", http.NoBody))

	if rec.Code != http.StatusForbidden || !strings.HasPrefix(rec.Body.String(), csrf.ReasonNoCSRFCookie+" token=") {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	token := strings.TrimPrefix(rec.Body.String(), csrf.ReasonNoCSRFCookie+" token=")
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	assertMaskedSecretCorrect(t, token, cookies[0].Value)
}

// renderPage renders src for a request under the middleware with the CSRF
// context processor installed.
func renderPage(t *testing.T, src string) *httptest.ResponseRecorder {
	t.Helper()
	engine, err := templates.NewEngineFS(fstest.MapFS{"page.html": {Data: []byte(src)}})
	if err != nil {
		t.Fatalf("NewEngineFS: %v", err)
	}
	engine.AddContextProcessor(csrf.ContextProcessor)
	return serveDefault(httptest.NewRequest(http.MethodGet, "/", http.NoBody),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			out, err := engine.RenderRequest(r, "page.html", nil)
			if err != nil {
				t.Errorf("RenderRequest: %v", err)
			}
			_, _ = io.WriteString(w, out)
		}))
}

func TestContextProcessorFeedsTheTemplateTag(t *testing.T) {
	rec := renderPage(t, `<form>{% csrf_token %}</form> js:{{ csrf_token }}`)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1; body = %q", len(cookies), rec.Body.String())
	}
	m := tokenPattern.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("no token field in %q", rec.Body.String())
	}
	assertMaskedSecretCorrect(t, m[1], cookies[0].Value)
	if !strings.HasSuffix(rec.Body.String(), "js:"+m[1]) {
		t.Errorf("body = %q, want the variable to print the token of the tag", rec.Body.String())
	}
}

func TestContextProcessorIsLazy(t *testing.T) {
	rec := renderPage(t, `<p>no form here</p>`)
	if n := len(rec.Result().Cookies()); n != 0 {
		t.Errorf("got %d cookies, want none for a page without a token", n)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q, want none", got)
	}
}
