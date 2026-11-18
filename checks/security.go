package checks

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/oliverhaas/djangogo/conf"
)

// The limits of the secret key checks (django.core.checks.security.base).
const (
	secretKeyInsecurePrefix      = "django-insecure-"
	secretKeyMinLength           = 50
	secretKeyMinUniqueCharacters = 5
)

// The values the two policy headers accept, in the order of the hints.
var (
	referrerPolicyValues = []string{
		"no-referrer", "no-referrer-when-downgrade", "origin", "origin-when-cross-origin",
		"same-origin", "strict-origin", "strict-origin-when-cross-origin", "unsafe-url",
	}
	crossOriginOpenerPolicyValues = []string{"same-origin", "same-origin-allow-popups", "unsafe-none"}
)

// The messages of the security checks, with Django's ids and texts
// (django/core/checks/security/base.py, csrf.py and sessions.py). A text names
// a setting in Django's spelling, which is also the name of its environment
// variable; the field of conf.Settings is the same name in Go spelling.
var (
	securityW004 = Warning("You have not set a value for the SECURE_HSTS_SECONDS setting. If your entire site is served only over SSL, you may want to consider setting a value and enabling HTTP Strict Transport Security. Be sure to read the documentation first; enabling HSTS carelessly can cause serious, irreversible problems.", ID("security.W004"))
	securityW005 = Warning("You have not set the SECURE_HSTS_INCLUDE_SUBDOMAINS setting to True. Without this, your site is potentially vulnerable to attack via an insecure connection to a subdomain. Only set this to True if you are certain that all subdomains of your domain should be served exclusively via SSL.", ID("security.W005"))
	securityW006 = Warning("Your SECURE_CONTENT_TYPE_NOSNIFF setting is not set to True, so your pages will not be served with an 'X-Content-Type-Options: nosniff' header. You should consider enabling this header to prevent the browser from identifying content types incorrectly.", ID("security.W006"))
	securityW008 = Warning("Your SECURE_SSL_REDIRECT setting is not set to True. Unless your site should be available over both SSL and non-SSL connections, you may want to either set this setting True or configure a load balancer or reverse-proxy server to redirect all connections to HTTPS.", ID("security.W008"))
	securityW012 = Warning("SESSION_COOKIE_SECURE is not set to True. Using a secure-only session cookie makes it more difficult for network traffic sniffers to hijack user sessions.", ID("security.W012"))
	securityW015 = Warning("SESSION_COOKIE_HTTPONLY is not set to True. Using an HttpOnly session cookie makes it more difficult for cross-site scripting attacks to hijack user sessions.", ID("security.W015"))
	securityW016 = Warning("You have 'django.middleware.csrf.CsrfViewMiddleware' in your MIDDLEWARE, but you have not set CSRF_COOKIE_SECURE to True. Using a secure-only CSRF cookie makes it more difficult for network traffic sniffers to steal the CSRF token.", ID("security.W016"))
	securityW018 = Warning("You should not have DEBUG set to True in deployment.", ID("security.W018"))
	securityW019 = Warning("You have 'django.middleware.clickjacking.XFrameOptionsMiddleware' in your MIDDLEWARE, but X_FRAME_OPTIONS is not set to 'DENY'. Unless there is a good reason for your site to serve other parts of itself in a frame, you should change it to 'DENY'.", ID("security.W019"))
	securityW020 = Warning("ALLOWED_HOSTS must not be empty in deployment.", ID("security.W020"))
	securityW021 = Warning("You have not set the SECURE_HSTS_PRELOAD setting to True. Without this, your site cannot be submitted to the browser preload list.", ID("security.W021"))
	securityW022 = Warning("You have not set the SECURE_REFERRER_POLICY setting. Without this, your site will not send a Referrer-Policy header. You should consider enabling this header to protect user privacy.", ID("security.W022"))
	securityE023 = Error("You have set the SECURE_REFERRER_POLICY setting to an invalid value.", Hint(validValuesHint(referrerPolicyValues)), ID("security.E023"))
	securityE024 = Error("You have set the SECURE_CROSS_ORIGIN_OPENER_POLICY setting to an invalid value.", Hint(validValuesHint(crossOriginOpenerPolicyValues)), ID("security.E024"))
)

// validValuesHint returns the hint of a message about a value outside values.
func validValuesHint(values []string) string {
	return "Valid values are: " + strings.Join(values, ", ") + "."
}

// weakSecretKey returns the warning about a weak key: security.W009 for
// SECRET_KEY, security.W025 for an entry of SECRET_KEY_FALLBACKS. name is the
// setting as the text names it.
func weakSecretKey(name, id string) Message {
	return Warning(fmt.Sprintf(
		"Your %s has less than %d characters, less than %d unique characters, or it's prefixed with '%s' "+
			"indicating that it was generated automatically by Django. Please generate a long and random "+
			"value, otherwise many of Django's security-critical features will be vulnerable to attack.",
		name, secretKeyMinLength, secretKeyMinUniqueCharacters, secretKeyInsecurePrefix,
	), ID(id))
}

// securityChecks lists the deployment checks tagged security, in the order
// Django defines them: base.py, csrf.py, sessions.py.
//
// Django's checks for a missing middleware (security.W001, W002, W003) and its
// variants of the cookie checks for a project without the session app or the
// session middleware (W010, W011, W013, W014) are left out: the middleware
// stack of a project always holds the security, clickjacking, session and CSRF
// middleware. So are the checks of settings that do not exist here: the
// content security policy (E026, W027) and CSRF_FAILURE_VIEW (E101, E102).
var securityChecks = []func(s *conf.Settings) []Message{
	checkSTS,
	checkSTSIncludeSubdomains,
	checkSTSPreload,
	checkContentTypeNosniff,
	checkSSLRedirect,
	checkSecretKey,
	checkSecretKeyFallbacks,
	checkDebug,
	checkXFrameDeny,
	checkAllowedHosts,
	checkReferrerPolicy,
	checkCrossOriginOpenerPolicy,
	checkCSRFCookieSecure,
	checkSessionCookieSecure,
	checkSessionCookieHTTPOnly,
}

// onSettings adapts a check of the settings to a Func. A Context without
// settings has nothing to check, so the check reports nothing for it.
func onSettings(fn func(s *conf.Settings) []Message) Func {
	return func(ctx *Context) []Message {
		if ctx.Settings == nil {
			return nil
		}
		return fn(ctx.Settings)
	}
}

// unless returns msg as the result of a check that did not pass.
func unless(passed bool, msg Message) []Message {
	if passed {
		return nil
	}
	return []Message{msg}
}

// checkSTS warns when SecureHSTSSeconds does not turn the
// Strict-Transport-Security header on (security.W004).
func checkSTS(s *conf.Settings) []Message {
	return unless(s.SecureHSTSSeconds > 0, securityW004)
}

// checkSTSIncludeSubdomains warns when the header is on without
// SecureHSTSIncludeSubdomains (security.W005).
func checkSTSIncludeSubdomains(s *conf.Settings) []Message {
	return unless(s.SecureHSTSSeconds <= 0 || s.SecureHSTSIncludeSubdomains, securityW005)
}

// checkSTSPreload warns when the header is on without SecureHSTSPreload
// (security.W021).
func checkSTSPreload(s *conf.Settings) []Message {
	return unless(s.SecureHSTSSeconds <= 0 || s.SecureHSTSPreload, securityW021)
}

// checkContentTypeNosniff warns when SecureContentTypeNosniff is off
// (security.W006).
func checkContentTypeNosniff(s *conf.Settings) []Message {
	return unless(s.SecureContentTypeNosniff, securityW006)
}

// checkSSLRedirect warns when SecureSSLRedirect is off (security.W008).
func checkSSLRedirect(s *conf.Settings) []Message {
	return unless(s.SecureSSLRedirect, securityW008)
}

// strongSecretKey reports whether key passes Django's _check_secret_key: at
// least 50 characters, at least 5 distinct ones, and not the prefix of a key
// that startproject generated.
func strongSecretKey(key string) bool {
	distinct := make(map[rune]struct{})
	for _, r := range key {
		distinct[r] = struct{}{}
	}
	return len(distinct) >= secretKeyMinUniqueCharacters &&
		utf8.RuneCountInString(key) >= secretKeyMinLength &&
		!strings.HasPrefix(key, secretKeyInsecurePrefix)
}

// checkSecretKey warns about a weak or missing SecretKey (security.W009).
func checkSecretKey(s *conf.Settings) []Message {
	return unless(strongSecretKey(s.SecretKey), weakSecretKey("SECRET_KEY", "security.W009"))
}

// checkSecretKeyFallbacks warns about every weak entry of SecretKeyFallbacks,
// naming its index (security.W025).
func checkSecretKeyFallbacks(s *conf.Settings) []Message {
	var messages []Message
	for i, key := range s.SecretKeyFallbacks {
		if !strongSecretKey(key) {
			messages = append(messages, weakSecretKey(fmt.Sprintf("SECRET_KEY_FALLBACKS[%d]", i), "security.W025"))
		}
	}
	return messages
}

// checkDebug warns when Debug is on (security.W018).
func checkDebug(s *conf.Settings) []Message {
	return unless(!s.Debug, securityW018)
}

// checkXFrameDeny warns when the X-Frame-Options header is not "DENY"
// (security.W019). Django compares the setting with "DENY" as written; the
// clickjacking middleware here upper-cases the value and sends "DENY" for the
// empty string, so the check reads the setting the same way.
func checkXFrameDeny(s *conf.Settings) []Message {
	value := strings.ToUpper(s.XFrameOptions)
	return unless(value == "" || value == "DENY", securityW019)
}

// checkAllowedHosts warns when AllowedHosts is empty (security.W020).
func checkAllowedHosts(s *conf.Settings) []Message {
	return unless(len(s.AllowedHosts) > 0, securityW020)
}

// checkReferrerPolicy warns when SecureReferrerPolicy is empty, which sends no
// Referrer-Policy header (security.W022), and reports a value that is no
// referrer policy (security.E023). The setting may hold several policies
// separated by commas, for browsers to fall back on.
func checkReferrerPolicy(s *conf.Settings) []Message {
	if s.SecureReferrerPolicy == "" {
		return []Message{securityW022}
	}
	for value := range strings.SplitSeq(s.SecureReferrerPolicy, ",") {
		if !slices.Contains(referrerPolicyValues, strings.TrimSpace(value)) {
			return []Message{securityE023}
		}
	}
	return nil
}

// checkCrossOriginOpenerPolicy reports a SecureCrossOriginOpenerPolicy that is
// no cross-origin opener policy (security.E024). The empty string sends no
// header and is valid.
func checkCrossOriginOpenerPolicy(s *conf.Settings) []Message {
	policy := s.SecureCrossOriginOpenerPolicy
	return unless(policy == "" || slices.Contains(crossOriginOpenerPolicyValues, policy), securityE024)
}

// checkCSRFCookieSecure warns when CSRFCookieSecure is off (security.W016).
// With CSRFUseSessions the token is kept in the session and no CSRF cookie is
// sent, so there is nothing to warn about.
func checkCSRFCookieSecure(s *conf.Settings) []Message {
	return unless(s.CSRFUseSessions || s.CSRFCookieSecure, securityW016)
}

// checkSessionCookieSecure warns when SessionCookieSecure is off
// (security.W012).
func checkSessionCookieSecure(s *conf.Settings) []Message {
	return unless(s.SessionCookieSecure, securityW012)
}

// checkSessionCookieHTTPOnly warns when SessionCookieHTTPOnly is off
// (security.W015).
func checkSessionCookieHTTPOnly(s *conf.Settings) []Message {
	return unless(s.SessionCookieHTTPOnly, securityW015)
}
