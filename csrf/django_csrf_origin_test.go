package csrf_test

import (
	"net/http"
	"testing"

	"github.com/oliverhaas/djangogo/csrf"
)

// securePost is a POST over HTTPS with a secret and a matching token.
func securePost() request {
	q := postWithToken()
	q.secure = true
	return q
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_bad_referer
func TestHTTPSBadReferer(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := securePost()
		q.host, q.referer = "www.example.com", str("https://www.evil.org/somepage")
		assertForbiddenReason(t, h.do(q, postFormView),
			"Referer checking failed - https://www.evil.org/somepage does not match any trusted origins.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_no_referer
func TestHTTPSNoReferer(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		assertForbiddenReason(t, h.do(securePost(), postFormView), "Referer checking failed - no Referer.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_malformed_host
func TestHTTPSMalformedHost(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := request{method: http.MethodPost, secure: true, host: "@malformed"}
		q.referer = str("https://www.evil.org/somepage")
		assertForbiddenReason(t, h.do(q, tokenView),
			"Referer checking failed - https://www.evil.org/somepage does not match any trusted origins.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_origin_malformed_host
func TestOriginMalformedHost(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := request{method: http.MethodPost, secure: true, host: "@malformed"}
		q.origin = str("https://www.evil.org")
		assertForbiddenReason(t, h.do(q, tokenView),
			"Origin checking failed - https://www.evil.org does not match any trusted origins.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_malformed_referer
func TestHTTPSMalformedReferer(t *testing.T) {
	cases := []struct{ referer, want string }{
		{"http://http://www.example.com/", csrf.ReasonInsecureReferer},
		{"", csrf.ReasonMalformedReferer},
		{"ØBöIß", csrf.ReasonMalformedReferer},
		// A missing scheme, a missing host and a URL that does not parse.
		{"//example.com/", csrf.ReasonMalformedReferer},
		{"https://", csrf.ReasonMalformedReferer},
		{"https://[", csrf.ReasonMalformedReferer},
	}
	eachMode(t, func(t *testing.T, h *harness) {
		for _, tc := range cases {
			q := securePost()
			q.referer = &tc.referer
			assertForbiddenReason(t, h.do(q, postFormView), tc.want)
		}
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_good_referer
func TestHTTPSGoodReferer(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := securePost()
		q.host, q.referer = "www.example.com", str("https://www.example.com/somepage")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_good_referer_2
func TestHTTPSGoodReferer2(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := securePost()
		q.host, q.referer = "www.example.com", str("https://www.example.com")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_good_referer_malformed_host
func TestHTTPSGoodRefererMalformedHost(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.TrustedOrigins = []string{"https://dashboard.example.com"}
		q := securePost()
		q.host, q.referer = "@malformed", str("https://dashboard.example.com/somepage")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_csrf_trusted_origin_allowed
func TestHTTPSCSRFTrustedOriginAllowed(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.TrustedOrigins = []string{"https://dashboard.example.com"}
		q := securePost()
		q.host, q.referer = "www.example.com", str("https://dashboard.example.com")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_https_csrf_wildcard_trusted_origin_allowed
func TestHTTPSCSRFWildcardTrustedOriginAllowed(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.TrustedOrigins = []string{"https://*.example.com"}
		q := securePost()
		q.host, q.referer = "www.example.com", str("https://dashboard.example.com")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_bad_origin_bad_domain
func TestBadOriginBadDomain(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := postWithToken()
		q.host, q.origin = "www.example.com", str("https://www.evil.org")
		assertForbiddenReason(t, h.do(q, postFormView),
			"Origin checking failed - https://www.evil.org does not match any trusted origins.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_bad_origin_null_origin
func TestBadOriginNullOrigin(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := postWithToken()
		q.host, q.origin = "www.example.com", str("null")
		assertForbiddenReason(t, h.do(q, postFormView),
			"Origin checking failed - null does not match any trusted origins.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_bad_origin_bad_protocol
func TestBadOriginBadProtocol(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := securePost()
		q.host, q.origin = "www.example.com", str("http://example.com")
		assertForbiddenReason(t, h.do(q, postFormView),
			"Origin checking failed - http://example.com does not match any trusted origins.")
	})
}

// The assertions on the parsed settings of Django's middleware object have no
// counterpart; the accepted and rejected origins show the same split.
//
// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_bad_origin_csrf_trusted_origin_bad_protocol
func TestBadOriginCSRFTrustedOriginBadProtocol(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.TrustedOrigins = []string{
			"http://no-match.com",
			"https://*.example.com",
			"http://*.no-match.com",
			"http://*.no-match-2.com",
		}
		q := securePost()
		q.host, q.origin = "www.example.com", str("http://foo.example.com")
		assertForbiddenReason(t, h.do(q, postFormView),
			"Origin checking failed - http://foo.example.com does not match any trusted origins.")

		for _, origin := range []string{
			"http://no-match.com",
			"https://foo.example.com",
			"http://foo.no-match.com",
			"http://foo.no-match-2.com",
		} {
			q.origin = &origin
			assertAccepted(t, h.do(q, postFormView))
		}
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_bad_origin_cannot_be_parsed
func TestBadOriginCannotBeParsed(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := postWithToken()
		q.host, q.origin = "www.example.com", str("https://[")
		assertForbiddenReason(t, h.do(q, postFormView),
			"Origin checking failed - https://[ does not match any trusted origins.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_good_origin_insecure
func TestGoodOriginInsecure(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := postWithToken()
		q.host, q.origin = "www.example.com", str("http://www.example.com")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_good_origin_secure
func TestGoodOriginSecure(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		q := securePost()
		q.host, q.origin = "www.example.com", str("https://www.example.com")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_good_origin_csrf_trusted_origin_allowed
func TestGoodOriginCSRFTrustedOriginAllowed(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.TrustedOrigins = []string{"https://dashboard.example.com"}
		q := securePost()
		q.host, q.origin = "www.example.com", str("https://dashboard.example.com")
		assertAccepted(t, h.do(q, postFormView))

		// The entry has no wildcard, so no other subdomain is trusted.
		q.origin = str("https://foo.example.com")
		assertForbiddenReason(t, h.do(q, postFormView),
			"Origin checking failed - https://foo.example.com does not match any trusted origins.")
	})
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTestMixin.test_good_origin_wildcard_csrf_trusted_origin_allowed
func TestGoodOriginWildcardCSRFTrustedOriginAllowed(t *testing.T) {
	eachMode(t, func(t *testing.T, h *harness) {
		h.opts.TrustedOrigins = []string{"https://*.example.com"}
		q := securePost()
		q.host, q.origin = "www.example.com", str("https://foo.example.com")
		assertAccepted(t, h.do(q, postFormView))
	})
}

// refererMatchesCookieDomain is Django's
// _test_https_good_referer_matches_cookie_domain.
func refererMatchesCookieDomain(t *testing.T, h *harness) {
	t.Helper()
	h.opts.CookieDomain = ".example.com"
	q := securePost()
	q.referer = str("https://foo.example.com/")
	assertAccepted(t, h.do(q, postFormView))
}

// refererMatchesCookieDomainWithPort is Django's
// _test_https_good_referer_matches_cookie_domain_with_different_port. Go has no
// SERVER_PORT, so the port of the request is the one in the Host header.
func refererMatchesCookieDomainWithPort(t *testing.T, h *harness) {
	t.Helper()
	h.opts.CookieDomain = ".example.com"
	q := securePost()
	q.host, q.referer = "www.example.com:4443", str("https://foo.example.com:4443/")
	assertAccepted(t, h.do(q, postFormView))

	q.referer = str("https://foo.example.com/")
	assertForbiddenReason(t, h.do(q, postFormView),
		"Referer checking failed - https://foo.example.com/ does not match any trusted origins.")
}

// rejectInsecureReferer is the body of Django's
// test_https_reject_insecure_referer.
func rejectInsecureReferer(t *testing.T, h *harness) {
	t.Helper()
	h.opts.CookieDomain = ".example.com"
	q := securePost()
	q.referer = str("http://example.com/")
	assertForbiddenReason(t, h.do(q, postFormView),
		"Referer checking failed - Referer is insecure while host is secure.")
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_https_good_referer_matches_cookie_domain
func TestHTTPSGoodRefererMatchesCookieDomain(t *testing.T) {
	refererMatchesCookieDomain(t, cookieMode(t))
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_https_good_referer_matches_cookie_domain_with_different_port
func TestHTTPSGoodRefererMatchesCookieDomainWithDifferentPort(t *testing.T) {
	refererMatchesCookieDomainWithPort(t, cookieMode(t))
}

// Ported from Django: tests/csrf_tests/tests.py::CsrfViewMiddlewareTests.test_https_reject_insecure_referer
func TestHTTPSRejectInsecureReferer(t *testing.T) {
	rejectInsecureReferer(t, cookieMode(t))
}
