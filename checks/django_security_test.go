package checks_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/conf"
)

// The tests in this file are ports of Django's
// tests/check_framework/test_security.py. A Django test overrides settings and
// calls one check function. A port changes fields of conf.Defaults(), runs the
// deployment checks tagged security through the Default registry and keeps the
// messages with the ids of that function.
//
// The MIDDLEWARE and INSTALLED_APPS overrides have no counterpart: the session,
// CSRF, security and clickjacking middleware are always installed. A Django
// test with the session app and the session middleware installed is the case
// that exists here.

// The messages of Django's security checks, printed by Django 6.0.3 from
// django/core/checks/security/base.py, sessions.py and csrf.py.
var (
	secW004 = checks.Warning("You have not set a value for the SECURE_HSTS_SECONDS setting. If your entire site is served only over SSL, you may want to consider setting a value and enabling HTTP Strict Transport Security. Be sure to read the documentation first; enabling HSTS carelessly can cause serious, irreversible problems.", checks.ID("security.W004"))
	secW005 = checks.Warning("You have not set the SECURE_HSTS_INCLUDE_SUBDOMAINS setting to True. Without this, your site is potentially vulnerable to attack via an insecure connection to a subdomain. Only set this to True if you are certain that all subdomains of your domain should be served exclusively via SSL.", checks.ID("security.W005"))
	secW006 = checks.Warning("Your SECURE_CONTENT_TYPE_NOSNIFF setting is not set to True, so your pages will not be served with an 'X-Content-Type-Options: nosniff' header. You should consider enabling this header to prevent the browser from identifying content types incorrectly.", checks.ID("security.W006"))
	secW008 = checks.Warning("Your SECURE_SSL_REDIRECT setting is not set to True. Unless your site should be available over both SSL and non-SSL connections, you may want to either set this setting True or configure a load balancer or reverse-proxy server to redirect all connections to HTTPS.", checks.ID("security.W008"))
	secW009 = checks.Warning("Your SECRET_KEY has less than 50 characters, less than 5 unique characters, or it's prefixed with 'django-insecure-' indicating that it was generated automatically by Django. Please generate a long and random value, otherwise many of Django's security-critical features will be vulnerable to attack.", checks.ID("security.W009"))
	secW012 = checks.Warning("SESSION_COOKIE_SECURE is not set to True. Using a secure-only session cookie makes it more difficult for network traffic sniffers to hijack user sessions.", checks.ID("security.W012"))
	secW015 = checks.Warning("SESSION_COOKIE_HTTPONLY is not set to True. Using an HttpOnly session cookie makes it more difficult for cross-site scripting attacks to hijack user sessions.", checks.ID("security.W015"))
	secW016 = checks.Warning("You have 'django.middleware.csrf.CsrfViewMiddleware' in your MIDDLEWARE, but you have not set CSRF_COOKIE_SECURE to True. Using a secure-only CSRF cookie makes it more difficult for network traffic sniffers to steal the CSRF token.", checks.ID("security.W016"))
	secW018 = checks.Warning("You should not have DEBUG set to True in deployment.", checks.ID("security.W018"))
	secW019 = checks.Warning("You have 'django.middleware.clickjacking.XFrameOptionsMiddleware' in your MIDDLEWARE, but X_FRAME_OPTIONS is not set to 'DENY'. Unless there is a good reason for your site to serve other parts of itself in a frame, you should change it to 'DENY'.", checks.ID("security.W019"))
	secW020 = checks.Warning("ALLOWED_HOSTS must not be empty in deployment.", checks.ID("security.W020"))
	secW021 = checks.Warning("You have not set the SECURE_HSTS_PRELOAD setting to True. Without this, your site cannot be submitted to the browser preload list.", checks.ID("security.W021"))
	secW022 = checks.Warning("You have not set the SECURE_REFERRER_POLICY setting. Without this, your site will not send a Referrer-Policy header. You should consider enabling this header to protect user privacy.", checks.ID("security.W022"))
	secE023 = checks.Error("You have set the SECURE_REFERRER_POLICY setting to an invalid value.", checks.Hint("Valid values are: no-referrer, no-referrer-when-downgrade, origin, origin-when-cross-origin, same-origin, strict-origin, strict-origin-when-cross-origin, unsafe-url."), checks.ID("security.E023"))
	secE024 = checks.Error("You have set the SECURE_CROSS_ORIGIN_OPENER_POLICY setting to an invalid value.", checks.Hint("Valid values are: same-origin, same-origin-allow-popups, unsafe-none."), checks.ID("security.E024"))
)

// TestSessionCookieSecureBoth warns when SESSION_COOKIE_SECURE is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSessionCookieSecureTest.test_session_cookie_secure_both
func TestSessionCookieSecureBoth(t *testing.T) {
	s := conf.Defaults()
	s.SessionCookieSecure = false
	assertMessages(t, deployMessages(s, "security.W012"), secW012)
}

// TestSessionCookieSecureTrue does not warn when SESSION_COOKIE_SECURE is on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSessionCookieSecureTest.test_session_cookie_secure_true
func TestSessionCookieSecureTrue(t *testing.T) {
	s := conf.Defaults()
	s.SessionCookieSecure = true
	assertMessages(t, deployMessages(s, "security.W012"))
}

// TestSessionCookieHTTPOnlyBoth warns when SESSION_COOKIE_HTTPONLY is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSessionCookieHttpOnlyTest.test_session_cookie_httponly_both
func TestSessionCookieHTTPOnlyBoth(t *testing.T) {
	s := conf.Defaults()
	s.SessionCookieHTTPOnly = false
	assertMessages(t, deployMessages(s, "security.W015"), secW015)
}

// TestSessionCookieHTTPOnlyTrue does not warn when SESSION_COOKIE_HTTPONLY is
// on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSessionCookieHttpOnlyTest.test_session_cookie_httponly_true
func TestSessionCookieHTTPOnlyTrue(t *testing.T) {
	s := conf.Defaults()
	s.SessionCookieHTTPOnly = true
	assertMessages(t, deployMessages(s, "security.W015"))
}

// TestWithCSRFCookieSecureFalse warns when CSRF_COOKIE_SECURE is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckCSRFCookieSecureTest.test_with_csrf_cookie_secure_false
func TestWithCSRFCookieSecureFalse(t *testing.T) {
	s := conf.Defaults()
	s.CSRFCookieSecure = false
	assertMessages(t, deployMessages(s, "security.W016"), secW016)
}

// TestUseSessionsWithCSRFCookieSecureFalse does not warn about
// CSRF_COOKIE_SECURE when CSRF_USE_SESSIONS is on: no CSRF cookie is set.
//
// Ported from Django: tests/check_framework/test_security.py::CheckCSRFCookieSecureTest.test_use_sessions_with_csrf_cookie_secure_false
func TestUseSessionsWithCSRFCookieSecureFalse(t *testing.T) {
	s := conf.Defaults()
	s.CSRFUseSessions = true
	s.CSRFCookieSecure = false
	assertMessages(t, deployMessages(s, "security.W016"))
}

// TestWithCSRFCookieSecureTrue does not warn when CSRF_COOKIE_SECURE is on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckCSRFCookieSecureTest.test_with_csrf_cookie_secure_true
func TestWithCSRFCookieSecureTrue(t *testing.T) {
	s := conf.Defaults()
	s.CSRFCookieSecure = true
	assertMessages(t, deployMessages(s, "security.W016"))
}

// TestNoSTS warns when SECURE_HSTS_SECONDS is not above 0.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecurityTest.test_no_sts
func TestNoSTS(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSSeconds = 0
	assertMessages(t, deployMessages(s, "security.W004"), secW004)
}

// TestWithSTS does not warn when SECURE_HSTS_SECONDS is set.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecurityTest.test_with_sts
func TestWithSTS(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSSeconds = 3600
	assertMessages(t, deployMessages(s, "security.W004"))
}

// TestNoSTSSubdomains warns when SECURE_HSTS_INCLUDE_SUBDOMAINS is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecuritySubdomainsTest.test_no_sts_subdomains
func TestNoSTSSubdomains(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSIncludeSubdomains = false
	s.SecureHSTSSeconds = 3600
	assertMessages(t, deployMessages(s, "security.W005"), secW005)
}

// TestNoSTSSubdomainsNoSeconds does not warn about the subdomains when
// SECURE_HSTS_SECONDS is not set. Django's None is 0 here.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecuritySubdomainsTest.test_no_sts_subdomains_no_seconds
func TestNoSTSSubdomainsNoSeconds(t *testing.T) {
	s := conf.Defaults()
	s.SecureSSLRedirect = false
	s.SecureHSTSSeconds = 0
	assertMessages(t, deployMessages(s, "security.W005"))
}

// TestWithSTSSubdomains does not warn when SECURE_HSTS_INCLUDE_SUBDOMAINS is
// on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecuritySubdomainsTest.test_with_sts_subdomains
func TestWithSTSSubdomains(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSIncludeSubdomains = true
	s.SecureHSTSSeconds = 3600
	assertMessages(t, deployMessages(s, "security.W005"))
}

// TestNoSTSPreload warns when SECURE_HSTS_PRELOAD is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecurityPreloadTest.test_no_sts_preload
func TestNoSTSPreload(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSPreload = false
	s.SecureHSTSSeconds = 3600
	assertMessages(t, deployMessages(s, "security.W021"), secW021)
}

// TestNoSTSPreloadNoSeconds does not warn about the preload flag when
// SECURE_HSTS_SECONDS is not set. Django's None is 0 here.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecurityPreloadTest.test_no_sts_preload_no_seconds
func TestNoSTSPreloadNoSeconds(t *testing.T) {
	s := conf.Defaults()
	s.SecureSSLRedirect = false
	s.SecureHSTSSeconds = 0
	assertMessages(t, deployMessages(s, "security.W021"))
}

// TestWithSTSPreload does not warn when SECURE_HSTS_PRELOAD is on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckStrictTransportSecurityPreloadTest.test_with_sts_preload
func TestWithSTSPreload(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSPreload = true
	s.SecureHSTSSeconds = 3600
	assertMessages(t, deployMessages(s, "security.W021"))
}

// TestXFrameOptionsNotDeny warns when X_FRAME_OPTIONS is not 'DENY'.
//
// Ported from Django: tests/check_framework/test_security.py::CheckXFrameOptionsDenyTest.test_x_frame_options_not_deny
func TestXFrameOptionsNotDeny(t *testing.T) {
	s := conf.Defaults()
	s.XFrameOptions = "SAMEORIGIN"
	assertMessages(t, deployMessages(s, "security.W019"), secW019)
}

// TestXFrameDeny does not warn when X_FRAME_OPTIONS is 'DENY'.
//
// Ported from Django: tests/check_framework/test_security.py::CheckXFrameOptionsDenyTest.test_xframe_deny
func TestXFrameDeny(t *testing.T) {
	s := conf.Defaults()
	s.XFrameOptions = "DENY"
	assertMessages(t, deployMessages(s, "security.W019"))
}

// TestNoContentTypeNosniff warns when SECURE_CONTENT_TYPE_NOSNIFF is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckContentTypeNosniffTest.test_no_content_type_nosniff
func TestNoContentTypeNosniff(t *testing.T) {
	s := conf.Defaults()
	s.SecureContentTypeNosniff = false
	assertMessages(t, deployMessages(s, "security.W006"), secW006)
}

// TestWithContentTypeNosniff does not warn when SECURE_CONTENT_TYPE_NOSNIFF is
// on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckContentTypeNosniffTest.test_with_content_type_nosniff
func TestWithContentTypeNosniff(t *testing.T) {
	s := conf.Defaults()
	s.SecureContentTypeNosniff = true
	assertMessages(t, deployMessages(s, "security.W006"))
}

// TestNoSSLRedirect warns when SECURE_SSL_REDIRECT is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSSLRedirectTest.test_no_ssl_redirect
func TestNoSSLRedirect(t *testing.T) {
	s := conf.Defaults()
	s.SecureSSLRedirect = false
	assertMessages(t, deployMessages(s, "security.W008"), secW008)
}

// TestWithSSLRedirect does not warn when SECURE_SSL_REDIRECT is on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckSSLRedirectTest.test_with_ssl_redirect
func TestWithSSLRedirect(t *testing.T) {
	s := conf.Defaults()
	s.SecureSSLRedirect = true
	assertMessages(t, deployMessages(s, "security.W008"))
}

// TestDebugTrue warns when DEBUG is on.
//
// Ported from Django: tests/check_framework/test_security.py::CheckDebugTest.test_debug_true
func TestDebugTrue(t *testing.T) {
	s := conf.Defaults()
	s.Debug = true
	assertMessages(t, deployMessages(s, "security.W018"), secW018)
}

// TestDebugFalse does not warn when DEBUG is off.
//
// Ported from Django: tests/check_framework/test_security.py::CheckDebugTest.test_debug_false
func TestDebugFalse(t *testing.T) {
	s := conf.Defaults()
	s.Debug = false
	assertMessages(t, deployMessages(s, "security.W018"))
}

// TestAllowedHostsEmpty warns when ALLOWED_HOSTS is empty.
//
// Ported from Django: tests/check_framework/test_security.py::CheckAllowedHostsTest.test_allowed_hosts_empty
func TestAllowedHostsEmpty(t *testing.T) {
	s := conf.Defaults()
	s.AllowedHosts = []string{}
	assertMessages(t, deployMessages(s, "security.W020"), secW020)
}

// TestAllowedHostsSet does not warn when ALLOWED_HOSTS names a host.
//
// Ported from Django: tests/check_framework/test_security.py::CheckAllowedHostsTest.test_allowed_hosts_set
func TestAllowedHostsSet(t *testing.T) {
	s := conf.Defaults()
	s.AllowedHosts = []string{".example.com"}
	assertMessages(t, deployMessages(s, "security.W020"))
}

// TestNoReferrerPolicy warns when SECURE_REFERRER_POLICY is not set. Django's
// None is the empty string here.
//
// Ported from Django: tests/check_framework/test_security.py::CheckReferrerPolicyTest.test_no_referrer_policy
func TestNoReferrerPolicy(t *testing.T) {
	s := conf.Defaults()
	s.SecureReferrerPolicy = ""
	assertMessages(t, deployMessages(s, "security.W022", "security.E023"), secW022)
}

// TestWithReferrerPolicy accepts one valid value and several separated by
// commas. Django also takes a list or a tuple; the Go setting is a string, so
// the port keeps the three string forms.
//
// Ported from Django: tests/check_framework/test_security.py::CheckReferrerPolicyTest.test_with_referrer_policy
func TestWithReferrerPolicy(t *testing.T) {
	tests := []string{
		"strict-origin",
		"strict-origin,origin",
		"strict-origin, origin",
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			s := conf.Defaults()
			s.SecureReferrerPolicy = value
			assertMessages(t, deployMessages(s, "security.W022", "security.E023"))
		})
	}
}

// TestWithInvalidReferrerPolicy reports a value that is no referrer policy.
//
// Ported from Django: tests/check_framework/test_security.py::CheckReferrerPolicyTest.test_with_invalid_referrer_policy
func TestWithInvalidReferrerPolicy(t *testing.T) {
	s := conf.Defaults()
	s.SecureReferrerPolicy = "invalid-value"
	assertMessages(t, deployMessages(s, "security.W022", "security.E023"), secE023)
}

// TestNoCOOP accepts an unset SECURE_CROSS_ORIGIN_OPENER_POLICY. Django's None
// is the empty string here.
//
// Ported from Django: tests/check_framework/test_security.py::CheckCrossOriginOpenerPolicyTest.test_no_coop
func TestNoCOOP(t *testing.T) {
	s := conf.Defaults()
	s.SecureCrossOriginOpenerPolicy = ""
	assertMessages(t, deployMessages(s, "security.E024"))
}

// TestWithCOOP accepts every valid cross-origin opener policy.
//
// Ported from Django: tests/check_framework/test_security.py::CheckCrossOriginOpenerPolicyTest.test_with_coop
func TestWithCOOP(t *testing.T) {
	tests := []string{"same-origin", "same-origin-allow-popups", "unsafe-none"}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			s := conf.Defaults()
			s.SecureCrossOriginOpenerPolicy = value
			assertMessages(t, deployMessages(s, "security.E024"))
		})
	}
}

// TestWithInvalidCOOP reports a value that is no cross-origin opener policy.
//
// Ported from Django: tests/check_framework/test_security.py::CheckCrossOriginOpenerPolicyTest.test_with_invalid_coop
func TestWithInvalidCOOP(t *testing.T) {
	s := conf.Defaults()
	s.SecureCrossOriginOpenerPolicy = "invalid-value"
	assertMessages(t, deployMessages(s, "security.E024"), secE024)
}
