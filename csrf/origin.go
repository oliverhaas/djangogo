package csrf

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// checkOrigin verifies the Origin header (Django's _origin_verified). It
// returns the reason for a rejection, or "".
func (c *config) checkOrigin(r *http.Request) string {
	origin := r.Header.Get("Origin")
	scheme := "http"
	if c.IsSecure(r) {
		scheme = "https"
	}
	if r.Host != "" && strings.EqualFold(origin, scheme+"://"+r.Host) {
		return ""
	}
	if c.trustedExact[origin] {
		return ""
	}
	parsed, err := url.Parse(origin)
	if err == nil {
		for _, suffix := range c.trustedSubdomains[parsed.Scheme] {
			if isSameDomain(parsed.Host, suffix) {
				return ""
			}
		}
	}
	return fmt.Sprintf(reasonBadOrigin, origin)
}

// checkReferer verifies the Referer header of an HTTPS request that has no
// Origin header (Django's _check_referer). It returns the reason for a
// rejection, or "".
func (c *config) checkReferer(r *http.Request) string {
	values, ok := r.Header["Referer"]
	if !ok || len(values) == 0 {
		return ReasonNoReferer
	}
	referer, err := url.Parse(values[0])
	if err != nil || referer.Scheme == "" || referer.Host == "" {
		return ReasonMalformedReferer
	}
	if referer.Scheme != "https" {
		return ReasonInsecureReferer
	}
	// Python keeps user information in the host part, so Django never
	// matches such a Referer. Browsers do not send one.
	if referer.User == nil {
		for _, host := range c.trustedHosts {
			if isSameDomain(referer.Host, host) {
				return ""
			}
		}
		good := r.Host
		if c.CookieDomain != "" {
			good = c.CookieDomain
			if port := requestPort(r, true); port != "443" && port != "80" {
				good += ":" + port
			}
		}
		if isSameDomain(referer.Host, good) {
			return ""
		}
	}
	return fmt.Sprintf(reasonBadReferer, referer.String())
}

// requestPort returns the port the request was sent to: the one in the Host
// header, or the default of the scheme.
func requestPort(r *http.Request, secure bool) string {
	if _, port, err := net.SplitHostPort(r.Host); err == nil && port != "" {
		return port
	}
	if secure {
		return "443"
	}
	return "80"
}

// isSameDomain reports whether host matches pattern (Django's
// is_same_domain). A pattern that starts with a dot matches the domain behind
// the dot and all its subdomains. Any other pattern must equal the host. The
// comparison ignores case.
func isSameDomain(host, pattern string) bool {
	if pattern == "" {
		return false
	}
	host, pattern = strings.ToLower(host), strings.ToLower(pattern)
	if pattern[0] == '.' {
		return strings.HasSuffix(host, pattern) || host == pattern[1:]
	}
	return pattern == host
}
