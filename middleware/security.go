package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// SecurityOptions configures the Security middleware. Every field is the
// counterpart of a Django setting, named in its comment.
type SecurityOptions struct {
	// SSLRedirect redirects every request that is not secure to HTTPS with a
	// 301 (SECURE_SSL_REDIRECT).
	SSLRedirect bool
	// SSLHost is the host to redirect to (SECURE_SSL_HOST). When it is empty
	// the redirect goes to the host of the request.
	SSLHost string
	// RedirectExempt lists regular expressions for paths that are not
	// redirected (SECURE_REDIRECT_EXEMPT). A pattern is searched in the path
	// without its leading slashes, so "^health/" exempts /health/.
	RedirectExempt []string
	// HSTSSeconds is the max-age of the Strict-Transport-Security header added
	// to secure responses (SECURE_HSTS_SECONDS). Zero or less adds no header.
	HSTSSeconds int
	// HSTSIncludeSubdomains adds the includeSubDomains directive
	// (SECURE_HSTS_INCLUDE_SUBDOMAINS).
	HSTSIncludeSubdomains bool
	// HSTSPreload adds the preload directive (SECURE_HSTS_PRELOAD).
	HSTSPreload bool
	// ContentTypeNosniff adds "X-Content-Type-Options: nosniff"
	// (SECURE_CONTENT_TYPE_NOSNIFF, which Django switches on by default).
	ContentTypeNosniff bool
	// ReferrerPolicy is the value of the Referrer-Policy header
	// (SECURE_REFERRER_POLICY, "same-origin" by default in Django). Several
	// policies can be given separated by commas, as a fallback list for
	// browsers that do not know the first one. Empty adds no header.
	ReferrerPolicy string
	// CrossOriginOpenerPolicy is the value of the Cross-Origin-Opener-Policy
	// header (SECURE_CROSS_ORIGIN_OPENER_POLICY, "same-origin" by default in
	// Django). Empty adds no header.
	CrossOriginOpenerPolicy string
	// ProxySSLHeader names a request header and the value that marks a request
	// as secure, for a site behind a proxy that terminates TLS
	// (SECURE_PROXY_SSL_HEADER), for example {"X-Forwarded-Proto", "https"}.
	// Empty means unset. See IsSecure for the trust this implies.
	ProxySSLHeader []string
}

// security is the state of one Security middleware: the options, validated,
// with everything precomputed that does not depend on the request.
type security struct {
	opts SecurityOptions
	// redirectExempt are the compiled RedirectExempt patterns.
	redirectExempt []*regexp.Regexp
	// hsts is the Strict-Transport-Security value, or empty for no header.
	hsts string
	// referrerPolicy is the Referrer-Policy value, or empty for no header.
	referrerPolicy string
}

// Security is Django's SecurityMiddleware. It redirects requests that are not
// secure to HTTPS (SSLRedirect) and adds the headers Strict-Transport-Security
// (to secure responses only), X-Content-Type-Options, Referrer-Policy, and
// Cross-Origin-Opener-Policy to every response, the redirect included. A header
// the handler already set is left as it is.
//
// Whether a request is secure is decided by IsSecure with opts.ProxySSLHeader.
//
// The redirect goes to opts.SSLHost. When that is empty it goes to the host the
// AllowedHosts middleware validated, provided that middleware is outside this
// one, and to the Host header of the request otherwise. A Host header that is
// not a valid host is answered with Django's plain 400 page.
//
// Security returns an error for a RedirectExempt pattern that does not compile
// and for a ProxySSLHeader that does not hold exactly two values.
func Security(opts SecurityOptions) (func(http.Handler) http.Handler, error) {
	s := &security{opts: opts}

	for _, pattern := range opts.RedirectExempt {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("middleware: RedirectExempt pattern %q: %w", pattern, err)
		}
		s.redirectExempt = append(s.redirectExempt, re)
	}

	if n := len(opts.ProxySSLHeader); n != 0 && n != 2 {
		return nil, errors.New("middleware: ProxySSLHeader (SECURE_PROXY_SSL_HEADER) must hold exactly two values, the header and its secure value")
	}
	// Copy the pair: the caller keeps its slice and may change it later.
	s.opts.ProxySSLHeader = append([]string(nil), opts.ProxySSLHeader...)

	if opts.HSTSSeconds > 0 {
		s.hsts = "max-age=" + strconv.Itoa(opts.HSTSSeconds)
		if opts.HSTSIncludeSubdomains {
			s.hsts += "; includeSubDomains"
		}
		if opts.HSTSPreload {
			s.hsts += "; preload"
		}
	}

	// Django accepts a comma-separated string, strips the values, and joins
	// them with a bare comma.
	if opts.ReferrerPolicy != "" {
		policies := strings.Split(opts.ReferrerPolicy, ",")
		for i, policy := range policies {
			policies[i] = strings.TrimSpace(policy)
		}
		s.referrerPolicy = strings.Join(policies, ",")
	}

	return s.wrap, nil
}

// wrap is the middleware function Security returns.
func (s *security) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secure := IsSecure(r, s.opts.ProxySSLHeader)

		if s.opts.SSLRedirect && !secure && !s.exempt(r) {
			s.redirect(w, r)
			return
		}

		rw := newResponseWriter(w, func() { s.setHeaders(w.Header(), secure) })
		// A handler that returns or panics without writing still gets the
		// headers: its response is sent later, by net/http or by Recover.
		defer rw.runBeforeHeader()
		next.ServeHTTP(rw, r)
	})
}

// exempt reports whether the path of the request matches a RedirectExempt
// pattern. Like Django it searches the path without its leading slashes.
func (s *security) exempt(r *http.Request) bool {
	path := strings.TrimLeft(r.URL.Path, "/")
	for _, re := range s.redirectExempt {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

// redirect answers a request that is not secure with a permanent redirect to
// the same path and query on HTTPS.
func (s *security) redirect(w http.ResponseWriter, r *http.Request) {
	host := s.opts.SSLHost
	if host == "" {
		var ok bool
		if host, ok = validatedHost(r); !ok {
			host = r.Host
			// Without AllowedHosts in front the host is whatever the client
			// sent. Refuse one that is not a host at all before it becomes
			// part of a Location header.
			if domain, _ := SplitDomainPort(host); domain == "" {
				writeErrorPage(w, http.StatusBadRequest, "Bad Request (400)", "")
				return
			}
		}
	}

	// Start the path with a slash in every case, so that the request target
	// cannot extend the host ("OPTIONS *" has a path without one).
	path := r.URL.EscapedPath()
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target := "https://" + host + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	h := w.Header()
	s.setHeaders(h, false)
	h.Set("Location", target)
	h.Set("Content-Type", htmlContentType)
	w.WriteHeader(http.StatusMovedPermanently)
}

// setHeaders adds the security headers to a response, Django's
// SecurityMiddleware.process_response. secure tells whether the request was
// secure. A header that is already present is never replaced.
func (s *security) setHeaders(h http.Header, secure bool) {
	if s.hsts != "" && secure {
		setDefaultHeader(h, "Strict-Transport-Security", s.hsts)
	}
	if s.opts.ContentTypeNosniff {
		setDefaultHeader(h, "X-Content-Type-Options", "nosniff")
	}
	if s.referrerPolicy != "" {
		setDefaultHeader(h, "Referrer-Policy", s.referrerPolicy)
	}
	if s.opts.CrossOriginOpenerPolicy != "" {
		setDefaultHeader(h, "Cross-Origin-Opener-Policy", s.opts.CrossOriginOpenerPolicy)
	}
}

// setDefaultHeader sets the header unless the response already has it, the
// counterpart of HttpResponse.setdefault.
func setDefaultHeader(h http.Header, name, value string) {
	if len(h.Values(name)) == 0 {
		h.Set(name, value)
	}
}

// IsSecure is Django's HttpRequest.is_secure: it reports whether the request
// was made over HTTPS.
//
// Without a proxy header that is the case when the request arrived over TLS.
// proxySSLHeader is Django's SECURE_PROXY_SSL_HEADER, a header name and the
// value that marks a secure request. The name may be given in Django's META
// spelling ("HTTP_X_FORWARDED_PROTO") or as an HTTP header name
// ("X-Forwarded-Proto"). When the request carries that header, the header alone
// decides, whatever the connection to the proxy looks like: the request is
// secure when the header has the secure value. As in Django, a comma-separated
// header value counts by its first element, which is the one the proxy closest
// to the client reported.
//
// Pass a proxy header only when the site runs behind a proxy that sets or
// strips the header on every request. Otherwise a client can send it and claim
// a secure connection. A proxySSLHeader that does not hold exactly two values
// is treated as unset. Security reports it as an error.
func IsSecure(r *http.Request, proxySSLHeader []string) bool {
	if len(proxySSLHeader) == 2 {
		if values := r.Header.Values(headerName(proxySSLHeader[0])); len(values) > 0 {
			first, _, _ := strings.Cut(values[0], ",")
			return strings.TrimSpace(first) == proxySSLHeader[1]
		}
	}
	return r.TLS != nil
}

// headerName turns a header given in Django's META spelling, such as
// "HTTP_X_FORWARDED_PROTO", into an HTTP header name. Any other name is
// returned as it is.
func headerName(name string) string {
	if meta, ok := strings.CutPrefix(name, "HTTP_"); ok {
		return strings.ReplaceAll(meta, "_", "-")
	}
	return name
}
