package csrf_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/csrf"
)

// capture returns a view that stores a token of the request in *token.
func capture(token *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*token = csrf.GetToken(r)
		_, _ = io.WriteString(w, "OK")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_unmask_cipher_token
func TestUnmaskCipherToken(t *testing.T) {
	a, b, c := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	cases := []struct{ secret, masked string }{
		{testSecret, maskedTestSecret1},
		{testSecret, maskedTestSecret2},
		{a, "vFioG3XOLyGyGsPRFyB9iYUs341ufzIEvFioG3XOLyGyGsPRFyB9iYUs341ufzIE"},
		{a, a + a},
		{a, b + b},
		{b, a + b},
		{b, b + c},
		{c, a + c},
	}
	for _, tc := range cases {
		if got := csrf.UnmaskToken(tc.masked); got != tc.secret {
			t.Errorf("UnmaskToken(%q) = %q, want %q", tc.masked, got, tc.secret)
		}
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_mask_cipher_secret
func TestMaskCipherSecret(t *testing.T) {
	for _, secret := range []string{
		strings.Repeat("a", 32),
		testSecret,
		"da4SrUiHJYoJ0HYQ0vcgisoIuFOxx4ER",
	} {
		assertMaskedSecretCorrect(t, csrf.MaskSecret(secret), secret)
	}
}

// Go keeps the secret of a request in the context the middleware makes, not
// in request.META, so the test goes through the middleware.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_get_token_csrf_cookie_set
func TestGetTokenCSRFCookieSet(t *testing.T) {
	h := cookieMode(t)
	var token string
	res := h.do(request{secret: str(testSecret)}, capture(&token))
	assertMaskedSecretCorrect(t, token, testSecret)
	if !res.sent || res.secret != testSecret {
		t.Errorf("cookie sent = %v with %q, want the existing secret %q", res.sent, res.secret, testSecret)
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_get_token_csrf_cookie_not_set
func TestGetTokenCSRFCookieNotSet(t *testing.T) {
	h := cookieMode(t)
	var token string
	res := h.do(get(), capture(&token))
	if !res.sent {
		t.Fatal("no CSRF cookie was sent")
	}
	assertMaskedSecretCorrect(t, token, res.secret)
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_rotate_token
func TestRotateToken(t *testing.T) {
	h := cookieMode(t)
	res := h.do(request{secret: str(testSecret)}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csrf.RotateToken(r)
		_, _ = io.WriteString(w, "OK")
	}))
	if !res.sent {
		t.Fatal("no CSRF cookie was sent")
	}
	if len(res.secret) != csrf.SecretLength {
		t.Errorf("secret %q has %d characters, want %d", res.secret, len(res.secret), csrf.SecretLength)
	}
	if res.secret == testSecret {
		t.Error("the secret was not changed")
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_check_token_format_valid
func TestCheckTokenFormatValid(t *testing.T) {
	for _, token := range []string{testSecret, maskedTestSecret1, strings.Repeat("a", 64)} {
		if reason := csrf.CheckFormat(token); reason != "" {
			t.Errorf("CheckFormat(%q) = %q, want no reason", token, reason)
		}
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_check_token_format_invalid
func TestCheckTokenFormatInvalid(t *testing.T) {
	cases := []struct{ token, want string }{
		{strings.Repeat("*", 64), "has invalid characters"},
		{strings.Repeat("a", 16), "has incorrect length"},
	}
	for _, tc := range cases {
		if got := csrf.CheckFormat(tc.token); got != tc.want {
			t.Errorf("CheckFormat(%q) = %q, want %q", tc.token, got, tc.want)
		}
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfFunctionTests.test_does_token_match
func TestDoesTokenMatch(t *testing.T) {
	a32, a64 := strings.Repeat("a", 32), strings.Repeat("a", 64)
	cases := []struct {
		token, secret string
		want          bool
	}{
		{maskedTestSecret1, testSecret, true},
		{maskedTestSecret2, testSecret, true},
		{a64, csrf.UnmaskToken(a64), true},
		{testSecret, testSecret, true},
		{a32, a32, true},
		{a32, testSecret, false},
		{a64, testSecret, false},
	}
	for _, tc := range cases {
		if got := csrf.TokenMatches(tc.token, tc.secret); got != tc.want {
			t.Errorf("TokenMatches(%q, %q) = %v, want %v", tc.token, tc.secret, got, tc.want)
		}
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_process_response_get_token_not_used
func TestProcessResponseGetTokenNotUsed(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		// The view runs the context processor without printing the token,
		// which must not make one: pages without a form stay cacheable.
		res := h.do(get(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = csrf.ContextProcessor(r)
			_, _ = io.WriteString(w, "OK")
		}))
		if res.sent || res.secret != "" {
			t.Errorf("secret %q was stored (sent = %v), want none", res.secret, res.sent)
		}
		// In session mode the sessions middleware adds the header, because
		// looking for the secret reads the session.
		if vary := res.rec.Header().Get("Vary"); vary != "" && !h.useSessions {
			t.Errorf("Vary = %q, want none", vary)
		}
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_no_csrf_cookie
func TestNoCSRFCookie(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		res := h.do(request{method: http.MethodPost}, postFormView)
		assertForbiddenReason(t, res, csrf.ReasonNoCSRFCookie)
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_csrf_cookie_bad_or_missing_token
func TestCSRFCookieBadOrMissingToken(t *testing.T) {
	a16, a64, stars := strings.Repeat("a", 16), strings.Repeat("a", 64), strings.Repeat("*", 64)
	cases := []struct {
		post   string
		header *string
		want   string
	}{
		{"", nil, csrf.ReasonCSRFTokenMissing},
		{a16, nil, "CSRF token from POST has incorrect length."},
		{stars, nil, "CSRF token from POST has invalid characters."},
		{a64, nil, "CSRF token from POST incorrect."},
		{"", &a16, "CSRF token from the 'X-Csrftoken' HTTP header has incorrect length."},
		{"", &stars, "CSRF token from the 'X-Csrftoken' HTTP header has invalid characters."},
		{"", &a64, "CSRF token from the 'X-Csrftoken' HTTP header incorrect."},
	}
	eachMode(t, func(t *testing.T, h *harness) {
		for _, tc := range cases {
			q := withSecret(http.MethodPost)
			q.postToken, q.headerToken = tc.post, tc.header
			assertForbiddenReason(t, h.do(q, postFormView), tc.want)
		}
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_csrf_cookie_bad_token_custom_header
func TestCSRFCookieBadTokenCustomHeader(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.HeaderName = "X-CSRFToken-Customized"
		q := withSecret(http.MethodPost)
		q.headerToken, q.headerName = str(strings.Repeat("a", 16)), "X-CSRFToken-Customized"
		assertForbiddenReason(t, h.do(q, postFormView),
			"CSRF token from the 'X-Csrftoken-Customized' HTTP header has incorrect length.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_process_request_csrf_cookie_and_token
func TestProcessRequestCSRFCookieAndToken(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		assertAccepted(t, h.do(postWithToken(), postFormView))
	})
}

// The middleware learns of the exemption through Options.Resolve, which stands
// for the view argument of Django's process_view.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_process_request_csrf_cookie_no_token_exempt_view
func TestProcessRequestCSRFCookieNoTokenExemptView(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.Resolve = func(*http.Request) http.Handler { return csrf.Exempt(postFormView) }
		assertAccepted(t, h.do(withSecret(http.MethodPost), postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_csrf_token_in_header
func TestCSRFTokenInHeader(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := withSecret(http.MethodPost)
		q.headerToken = str(maskedTestSecret2)
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_csrf_token_in_header_with_customized_name
func TestCSRFTokenInHeaderWithCustomizedName(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.HeaderName = "X-CSRFToken-Customized"
		q := withSecret(http.MethodPost)
		q.headerToken, q.headerName = str(maskedTestSecret2), "X-CSRFToken-Customized"
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_put_and_delete_rejected
func TestPutAndDeleteRejected(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			assertForbiddenReason(t, h.do(request{method: method}, postFormView), csrf.ReasonNoCSRFCookie)
		}
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_put_and_delete_allowed
func TestPutAndDeleteAllowed(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			q := withSecret(method)
			q.headerToken = str(maskedTestSecret2)
			assertAccepted(t, h.do(q, postFormView))
		}
	})
}

// Django stacks decorators, so its middleware stores the secret twice. The Go
// middleware stores it once, when the response starts, and that write must
// carry the rotated secret.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_rotate_token_triggers_second_reset
func TestRotateTokenTriggersSecondReset(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		res := h.do(postWithToken(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = csrf.GetToken(r)
			csrf.RotateToken(r)
			_, _ = io.WriteString(w, "OK")
		}))
		assertAccepted(t, res)
		if got := res.rec.Body.String(); got != "OK" {
			t.Errorf("body = %q, want OK", got)
		}
		if !res.sent || len(res.secret) != csrf.SecretLength || res.secret == testSecret {
			t.Errorf("secret after the response = %q (sent = %v), want a new one", res.secret, res.sent)
		}
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_token_node_no_csrf_cookie
func TestTokenNodeNoCSRFCookie(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		res := h.do(get(), tokenView)
		if !res.sent {
			t.Fatal("no secret was stored")
		}
		checkTokenPresent(t, res, res.secret)
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_token_node_empty_csrf_cookie
func TestTokenNodeEmptyCSRFCookie(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		res := h.do(request{secret: str("")}, tokenView)
		if !res.sent {
			t.Fatal("no secret was stored")
		}
		checkTokenPresent(t, res, res.secret)
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_token_node_with_csrf_cookie
func TestTokenNodeWithCSRFCookie(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		checkTokenPresent(t, h.do(withSecret(http.MethodGet), tokenView), testSecret)
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_get_token_for_exempt_view
func TestGetTokenForExemptView(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.Resolve = func(*http.Request) http.Handler { return csrf.Exempt(tokenView) }
		checkTokenPresent(t, h.do(withSecret(http.MethodGet), tokenView), testSecret)
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_token_node_with_new_csrf_cookie
func TestTokenNodeWithNewCSRFCookie(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		res := h.do(get(), tokenView)
		if !res.sent || len(res.secret) != csrf.SecretLength {
			t.Fatalf("secret after the response = %q (sent = %v), want a new one", res.secret, res.sent)
		}
		checkTokenPresent(t, res, res.secret)
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_cookie_not_reset_on_accepted_request
func TestCookieNotResetOnAcceptedRequest(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		res := h.do(postWithToken(), tokenView)
		assertAccepted(t, res)
		if res.secret != testSecret {
			t.Errorf("CSRF cookie was changed on an accepted request: %q", res.secret)
		}
	})
}

// A request whose body cannot be read has no form token, as in Django.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_reading_post_data_raises_unreadable_post_error
func TestReadingPostDataRaisesUnreadablePostError(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		assertAccepted(t, h.do(postWithToken(), postFormView))

		q := postWithToken()
		q.brokenBody = true
		assertForbiddenReason(t, h.do(q, postFormView), csrf.ReasonCSRFTokenMissing)
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_csrf_cookie_age
func TestCSRFCookieAge(t *testing.T) {
	h := cookieMode(t)
	h.opts = csrf.Options{
		CookieName:     "csrfcookie",
		CookieDomain:   ".example.com",
		CookieAge:      123 * time.Second,
		CookiePath:     "/test/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
	}
	c := h.do(get(), tokenView).csrfCookie("csrfcookie")
	if c == nil {
		t.Fatal("no csrfcookie was sent")
	}
	if c.MaxAge != 123 {
		t.Errorf("Max-Age = %d, want 123", c.MaxAge)
	}
	if c.Domain != "example.com" || c.Path != "/test/" || !c.Secure || !c.HttpOnly {
		t.Errorf("cookie = %s, want the domain, path, Secure and HttpOnly of the options", c)
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_csrf_cookie_age_none
func TestCSRFCookieAgeNone(t *testing.T) {
	h := cookieMode(t)
	h.opts = csrf.Options{
		CookieName:     "csrfcookie",
		CookieDomain:   ".example.com",
		CookieAge:      -1,
		CookiePath:     "/test/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
	}
	res := h.do(get(), tokenView)
	c := res.csrfCookie("csrfcookie")
	if c == nil {
		t.Fatal("no csrfcookie was sent")
	}
	raw := res.rec.Header().Get("Set-Cookie")
	if strings.Contains(raw, "Max-Age") || strings.Contains(raw, "Expires") {
		t.Errorf("Set-Cookie = %q, want a cookie without a lifetime", raw)
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_csrf_cookie_samesite
func TestCSRFCookieSameSite(t *testing.T) {
	h := cookieMode(t)
	h.opts = csrf.Options{CookieName: "csrfcookie", CookieSameSite: http.SameSiteStrictMode}
	c := h.do(get(), tokenView).csrfCookie("csrfcookie")
	if c == nil {
		t.Fatal("no csrfcookie was sent")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", c.SameSite)
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_bad_csrf_cookie_characters
func TestBadCSRFCookieCharacters(t *testing.T) {
	h := cookieMode(t)
	res := h.do(request{method: http.MethodPost, secret: str(strings.Repeat("*", 64))}, postFormView)
	assertForbiddenReason(t, res, "CSRF cookie has invalid characters.")
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_bad_csrf_cookie_length
func TestBadCSRFCookieLength(t *testing.T) {
	h := cookieMode(t)
	res := h.do(request{method: http.MethodPost, secret: str(strings.Repeat("a", 16))}, postFormView)
	assertForbiddenReason(t, res, "CSRF cookie has incorrect length.")
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_process_view_token_too_long
func TestProcessViewTokenTooLong(t *testing.T) {
	h := cookieMode(t)
	res := h.do(request{secret: str(strings.Repeat("x", 100000))}, tokenView)
	if !res.sent || len(res.secret) != csrf.SecretLength {
		t.Errorf("secret after the response has %d characters (sent = %v), want %d",
			len(res.secret), res.sent, csrf.SecretLength)
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_process_view_token_invalid_chars
func TestProcessViewTokenInvalidChars(t *testing.T) {
	h := cookieMode(t)
	token := ("!@#" + maskedTestSecret2)[:csrf.TokenLength]
	res := h.do(request{secret: &token}, tokenView)
	if !res.sent || len(res.secret) != csrf.SecretLength {
		t.Errorf("secret after the response has %d characters (sent = %v), want %d",
			len(res.secret), res.sent, csrf.SecretLength)
	}
	if res.secret == token {
		t.Error("the invalid cookie was kept")
	}
}

// combination is one case of the masked and unmasked tests: where the secret
// and the token come from.
type combination struct {
	secret, post string
	header       *string
}

// assertCombinationsAccepted sends each combination as a POST and expects the
// view to run.
func assertCombinationsAccepted(t *testing.T, h *harness, cases []combination) {
	t.Helper()
	for i, tc := range cases {
		q := request{method: http.MethodPost, secret: &tc.secret, postToken: tc.post, headerToken: tc.header}
		res := h.do(q, tokenView)
		if res.reason != "" || !res.called {
			t.Errorf("case %d (%s) was rejected: %q", i, fmt.Sprint(tc.secret, " ", tc.post), res.reason)
		}
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_masked_unmasked_combinations
func TestMaskedUnmaskedCombinations(t *testing.T) {
	assertCombinationsAccepted(t, cookieMode(t), []combination{
		{testSecret, testSecret, nil},
		{testSecret, maskedTestSecret2, nil},
		{testSecret, "", str(testSecret)},
		{testSecret, "", str(maskedTestSecret2)},
		{maskedTestSecret1, testSecret, nil},
		{maskedTestSecret1, maskedTestSecret2, nil},
		{maskedTestSecret1, "", str(testSecret)},
		{maskedTestSecret1, "", str(maskedTestSecret2)},
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_invalid_cookie_replaced_on_GET
func TestInvalidCookieReplacedOnGET(t *testing.T) {
	h := cookieMode(t)
	res := h.do(request{secret: str("badvalue")}, postFormView)
	if got := res.rec.Body.String(); got != "OK" {
		t.Errorf("body = %q, want OK", got)
	}
	if !res.sent {
		t.Fatal("No CSRF cookie was sent.")
	}
	if len(res.secret) != csrf.SecretLength {
		t.Errorf("secret %q has %d characters, want %d", res.secret, len(res.secret), csrf.SecretLength)
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_valid_secret_not_replaced_on_GET
func TestValidSecretNotReplacedOnGET(t *testing.T) {
	h := cookieMode(t)
	for _, cookie := range []string{testSecret, maskedTestSecret1} {
		res := h.do(request{secret: &cookie}, postFormView)
		if got := res.rec.Body.String(); got != "OK" {
			t.Errorf("body = %q, want OK", got)
		}
		if res.sent {
			t.Errorf("A CSRF cookie was sent for the cookie %q.", cookie)
		}
	}
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_masked_secret_accepted_and_replaced
func TestMaskedSecretAcceptedAndReplaced(t *testing.T) {
	h := cookieMode(t)
	res := h.do(postWithToken(), tokenView)
	assertAccepted(t, res)
	if !res.sent || res.secret != testSecret {
		t.Errorf("cookie after the response = %q (sent = %v), want the unmasked secret", res.secret, res.sent)
	}
	checkTokenPresent(t, res, res.secret)
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_bare_secret_accepted_and_not_replaced
func TestBareSecretAcceptedAndNotReplaced(t *testing.T) {
	h := cookieMode(t)
	q := postWithToken()
	q.secret = str(testSecret)
	res := h.do(q, tokenView)
	assertAccepted(t, res)
	if res.secret != testSecret {
		t.Errorf("cookie after the response = %q, want %q", res.secret, testSecret)
	}
	checkTokenPresent(t, res, res.secret)
}
