package middleware

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// AllowedHostsOptions configures the validation of the host of a request.
type AllowedHostsOptions struct {
	// AllowedHosts lists the hosts the site serves, Django's ALLOWED_HOSTS. An
	// entry is a host name or IP address without a port, compared ignoring
	// case. An entry that starts with a dot matches the domain and all its
	// subdomains, and "*" matches every host. IPv6 addresses are written in
	// brackets.
	AllowedHosts []string
	// Debug allows ".localhost", "127.0.0.1", and "[::1]" while AllowedHosts is
	// empty, and makes the AllowedHosts middleware name the reason on its 400
	// page.
	Debug bool
	// UseXForwardedHost makes the X-Forwarded-Host header take precedence over
	// the Host header (Django's USE_X_FORWARDED_HOST). Switch it on only behind
	// a proxy that sets the header.
	UseXForwardedHost bool
	// UseXForwardedPort makes GetPort prefer the X-Forwarded-Port header
	// (Django's USE_X_FORWARDED_PORT). Switch it on only behind a proxy that
	// sets the header.
	UseXForwardedPort bool
}

// DisallowedHost is the error GetHost returns for a host that fails
// validation, the counterpart of django.core.exceptions.DisallowedHost.
type DisallowedHost struct {
	// Msg is Django's message for the host, for example
	// "Invalid HTTP_HOST header: 'x'. You may need to add 'x' to ALLOWED_HOSTS."
	Msg string
}

// Error returns the message.
func (e *DisallowedHost) Error() string {
	return e.Msg
}

// debugAllowedHosts are the hosts allowed when Debug is on and no host is
// configured: the variants of localhost.
var debugAllowedHosts = []string{".localhost", "127.0.0.1", "[::1]"}

// hostValidationRE is host_validation_re of django/http/request.py. It matches
// a lower-cased host: a domain or IPv4 address, or an IPv6 address in
// brackets, and an optional port.
var hostValidationRE = regexp.MustCompile(`^([a-z0-9.-]+|\[[a-f0-9]*:[a-f0-9.:]+\])(?::([0-9]+))?$`)

// validatedHostKey is the context key under which the AllowedHosts middleware
// stores the host it validated.
type validatedHostKey struct{}

// AllowedHosts is the host validation Django runs when a request asks for its
// host: a request whose host is not allowed is answered with 400 Bad Request
// and does not reach the wrapped handler. The body is Django's plain 400 page.
// With Debug on it also carries the message of the DisallowedHost error, which
// names the host to add.
//
// Put it outside every middleware and handler that uses the host of the
// request, for example to build a redirect or a link.
func AllowedHosts(opts AllowedHostsOptions) func(http.Handler) http.Handler {
	// Copy the list: the caller keeps its slice and may change it later.
	opts.AllowedHosts = slices.Clone(opts.AllowedHosts)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, err := GetHost(r, opts)
			if err != nil {
				details := ""
				if opts.Debug {
					details = escapeHTML(err.Error())
				}
				writeErrorPage(w, http.StatusBadRequest, "Bad Request (400)", details)
				return
			}
			ctx := context.WithValue(r.Context(), validatedHostKey{}, host)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validatedHost returns the host the AllowedHosts middleware validated for the
// request, if the request passed one.
func validatedHost(r *http.Request) (string, bool) {
	host, ok := r.Context().Value(validatedHostKey{}).(string)
	return host, ok
}

// rawHost is Django's HttpRequest._get_raw_host: the host as the client or the
// proxy sent it, not validated. A Go request has no SERVER_NAME to fall back
// to, so the result is empty when the request names no host.
func rawHost(r *http.Request, useXForwardedHost bool) string {
	if useXForwardedHost {
		if forwarded := r.Header.Values("X-Forwarded-Host"); len(forwarded) > 0 {
			return forwarded[0]
		}
	}
	if r.Host != "" {
		return r.Host
	}
	if r.URL != nil {
		return r.URL.Host
	}
	return ""
}

// GetHost is Django's HttpRequest.get_host. It returns the host of the request
// as it was sent, with its port if it has one: the X-Forwarded-Host header when
// opts.UseXForwardedHost is set and the header is present, else the Host
// header. The host is validated against opts.AllowedHosts. A host that is
// malformed or not allowed gives a *DisallowedHost error with Django's message.
//
// Django rebuilds the host of a request without a Host header from SERVER_NAME
// and SERVER_PORT. A Go request has neither, so such a request is rejected.
func GetHost(r *http.Request, opts AllowedHostsOptions) (string, error) {
	host := rawHost(r, opts.UseXForwardedHost)

	allowedHosts := opts.AllowedHosts
	if opts.Debug && len(allowedHosts) == 0 {
		allowedHosts = debugAllowedHosts
	}

	domain, _ := SplitDomainPort(host)
	if domain != "" && ValidateHost(domain, allowedHosts) {
		return host, nil
	}

	msg := "Invalid HTTP_HOST header: " + pyRepr(host) + "."
	if domain != "" {
		msg += " You may need to add " + pyRepr(domain) + " to ALLOWED_HOSTS."
	} else {
		msg += " The domain name provided is not valid according to RFC 1034/1035."
	}
	return "", &DisallowedHost{Msg: msg}
}

// GetPort is Django's HttpRequest.get_port. It returns the X-Forwarded-Port
// header when opts.UseXForwardedPort is set and the header is present.
//
// Otherwise Django returns SERVER_PORT, which a Go request does not carry. The
// port is taken from the Host header instead. When the Host header names no
// port, the result is the default port of the scheme: "443" for a request that
// arrived over TLS (r.TLS is set), else "80". A proxy that terminates TLS is
// not taken into account here. Behind one, have it send X-Forwarded-Port.
func GetPort(r *http.Request, opts AllowedHostsOptions) string {
	if opts.UseXForwardedPort {
		if forwarded := r.Header.Values("X-Forwarded-Port"); len(forwarded) > 0 {
			return forwarded[0]
		}
	}
	if _, port := SplitDomainPort(r.Host); port != "" {
		return port
	}
	if r.TLS != nil {
		return "443"
	}
	return "80"
}

// ValidateHost is django.http.request.validate_host. It reports whether host,
// a lower-cased host name or IP address without a port, matches one of the
// allowed hosts. A pattern that starts with a dot matches the domain and all
// its subdomains (".example.com" matches "example.com" and "www.example.com"),
// "*" matches anything, and any other pattern must equal the host. Patterns
// are compared ignoring their case.
func ValidateHost(host string, allowedHosts []string) bool {
	for _, pattern := range allowedHosts {
		if pattern == "*" || isSameDomain(host, pattern) {
			return true
		}
	}
	return false
}

// isSameDomain is django.utils.http.is_same_domain: it reports whether host is
// an exact match of pattern or, for a pattern that starts with a dot, the
// domain after the dot or a subdomain of it.
func isSameDomain(host, pattern string) bool {
	if pattern == "" {
		return false
	}
	pattern = strings.ToLower(pattern)
	if pattern[0] == '.' && (strings.HasSuffix(host, pattern) || host == pattern[1:]) {
		return true
	}
	return pattern == host
}

// SplitDomainPort is django.http.request.split_domain_port. It splits a host
// into its domain, lower-cased and without a trailing dot, and its port. The
// port is empty when the host names none. Both results are empty when the host
// is not valid, which keeps a caller from validating a host by its prefix. An
// IPv6 address keeps its brackets.
//
// Only ASCII letters are lower-cased. Django lower-cases with Unicode rules,
// under which a few characters outside ASCII (the Kelvin sign, for one) turn
// into ASCII letters and pass. Such a host is rejected here.
func SplitDomainPort(host string) (domain, port string) {
	match := hostValidationRE.FindStringSubmatch(asciiLower(host))
	if match == nil {
		return "", ""
	}
	return strings.TrimSuffix(match[1], "."), match[2]
}

// asciiLower lower-cases the ASCII letters of s and leaves every other byte as
// it is.
func asciiLower(s string) string {
	for i := range len(s) {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if c := b[j]; c >= 'A' && c <= 'Z' {
					b[j] = c + ('a' - 'A')
				}
			}
			return string(b)
		}
	}
	return s
}
