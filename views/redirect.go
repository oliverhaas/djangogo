package views

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/templates"
)

// MaxURLRedirectLength is the longest Location a redirect may carry by
// default, Django's MAX_URL_REDIRECT_LENGTH.
const MaxURLRedirectLength = 16384

// allowedRedirectSchemes lists the URL schemes a redirect may point to, as in
// Django's HttpResponseRedirectBase. Anything else (javascript:, data:, ...)
// is rejected.
var allowedRedirectSchemes = []string{"http", "https", "ftp"}

// AbsoluteURLer is implemented by a model (or any value) that knows its own
// URL, the counterpart of Django's get_absolute_url method. Redirects and the
// success URL of the edit views use it.
type AbsoluteURLer interface {
	GetAbsoluteURL() string
}

// Redirect issues an HTTP redirect to url with the given status (e.g. 302).
// It is a thin wrapper around http.Redirect and applies no checks. Use
// RedirectTo for Django's redirect shortcut.
func Redirect(w http.ResponseWriter, r *http.Request, url string, status int) {
	http.Redirect(w, r, url, status)
}

// ResolveURL returns the URL for "to", Django's resolve_url, using the
// process-wide URL resolver (templates.URLResolverFunc). See
// Redirector.Resolve for the accepted values.
func ResolveURL(to any, args ...any) (string, error) {
	return Redirector{}.Resolve(to, args...)
}

// RedirectTo answers with a temporary redirect (302) to "to", Django's
// redirect(to, *args). See Redirector.Resolve for the accepted values and
// Redirector for the permanent and preserve-request variants.
func RedirectTo(w http.ResponseWriter, r *http.Request, to any, args ...any) error {
	return Redirector{}.To(w, r, to, args...)
}

// RedirectPermanent answers with a permanent redirect (301) to "to", Django's
// redirect(to, *args, permanent=True).
func RedirectPermanent(w http.ResponseWriter, r *http.Request, to any, args ...any) error {
	return Redirector{Permanent: true}.To(w, r, to, args...)
}

// Redirector holds the keyword arguments of Django's redirect shortcut. The
// zero value issues a temporary redirect and resolves URL names through the
// process-wide resolver.
type Redirector struct {
	// Permanent selects 301 (or 308) instead of 302 (or 307).
	Permanent bool
	// PreserveRequest selects 307 or 308, which make the client repeat the
	// request with the same method and body.
	PreserveRequest bool
	// Resolver reverses URL names. Nil uses templates.URLResolverFunc.
	Resolver templates.Resolver
	// MaxLength caps the length of the Location header. Zero means
	// MaxURLRedirectLength and a negative value disables the check.
	MaxLength int
}

// Resolve returns the URL for "to", Django's resolve_url:
//
//   - a value with a GetAbsoluteURL method resolves to that URL
//   - a string starting with "./" or "../" is a relative URL and returned as it is
//   - any other string is first tried as a URL name, reversed with args
//   - a string that is not a known name but contains "/" or "." is a URL and
//     returned as it is
//
// A *url.URL or fmt.Stringer is treated like its string. Any other value, and
// a name that cannot be reversed, is an error.
func (rd Redirector) Resolve(to any, args ...any) (string, error) {
	var target string
	switch v := to.(type) {
	case AbsoluteURLer:
		return v.GetAbsoluteURL(), nil
	case string:
		target = v
	case fmt.Stringer:
		target = v.String()
	default:
		return "", fmt.Errorf("views: cannot resolve a URL from %T", to)
	}
	if strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") {
		return target, nil
	}
	resolver := rd.Resolver
	if resolver == nil {
		resolver = templates.URLResolverFunc()
	}
	var reverseErr error
	if resolver == nil {
		reverseErr = fmt.Errorf("views: no URL resolver to reverse %q", target)
	} else {
		resolved, err := resolver(target, args...)
		if err == nil {
			return resolved, nil
		}
		reverseErr = fmt.Errorf("views: reverse %q: %w", target, err)
	}
	// A string that does not look like a URL was meant as a name.
	if !strings.ContainsAny(target, "/.") {
		return "", reverseErr
	}
	return target, nil
}

// To answers with a redirect to "to" (see Resolve). The status is 302, 301
// with Permanent, 307 with PreserveRequest and 308 with both. The Location is
// percent-encoded like Django's iri_to_uri and checked before anything is
// written: a target longer than MaxLength or with a scheme other than http,
// https or ftp is refused with an ErrDisallowedRedirect, which maps to a 400.
// The check does not decide whether a host is trusted. Validate a redirect
// target taken from user input before passing it here.
func (rd Redirector) To(w http.ResponseWriter, r *http.Request, to any, args ...any) error {
	target, err := rd.Resolve(to, args...)
	if err != nil {
		return err
	}
	return rd.redirect(w, r, target)
}

// status returns the redirect status for the Permanent and PreserveRequest
// combination.
func (rd Redirector) status() int {
	switch {
	case rd.Permanent && rd.PreserveRequest:
		return http.StatusPermanentRedirect
	case rd.Permanent:
		return http.StatusMovedPermanently
	case rd.PreserveRequest:
		return http.StatusTemporaryRedirect
	default:
		return http.StatusFound
	}
}

// redirect checks an already resolved target and writes the redirect.
func (rd Redirector) redirect(w http.ResponseWriter, _ *http.Request, target string) error {
	location, err := checkedLocation(target, rd.MaxLength)
	if err != nil {
		return err
	}
	w.Header().Set("Location", location)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(rd.status())
	return nil
}

// checkedLocation returns the Location header value for target after Django's
// redirect checks. maxLength follows Redirector.MaxLength.
func checkedLocation(target string, maxLength int) (string, error) {
	location := iriToURI(target)
	if maxLength == 0 {
		maxLength = MaxURLRedirectLength
	}
	if maxLength > 0 && len(location) > maxLength {
		return "", &messageError{
			kind: ErrDisallowedRedirect,
			msg:  fmt.Sprintf("Unsafe redirect exceeding %d characters", maxLength),
		}
	}
	scheme := redirectScheme(target)
	if scheme != "" && !slices.Contains(allowedRedirectSchemes, scheme) {
		return "", &messageError{
			kind: ErrDisallowedRedirect,
			msg:  fmt.Sprintf("Unsafe redirect to URL with protocol '%s'", scheme),
		}
	}
	return location, nil
}

// redirectScheme returns the lowercased scheme of target, or "" for a URL
// without one. A target net/url cannot parse is scanned by hand, so that a
// malformed URL with a disallowed scheme is not treated as having none.
func redirectScheme(target string) string {
	trimmed := strings.TrimLeft(target, " \t\r\n\f\v")
	if u, err := url.Parse(trimmed); err == nil {
		return strings.ToLower(u.Scheme)
	}
	head, _, found := strings.Cut(trimmed, ":")
	if !found || strings.ContainsAny(head, "/?#") {
		return ""
	}
	return strings.ToLower(head)
}

// iriToURI percent-encodes a URL the way Django's iri_to_uri does: ASCII
// letters, digits and the characters that carry meaning in a URL stay, every
// other byte (spaces, quotes, control characters, non-ASCII text) becomes
// %XX. Encoding control characters also keeps a crafted target from breaking
// out of the Location header.
func iriToURI(iri string) string {
	const safe = "/#%[]=:;$&()+,!?*@'~_.-"
	const upperHex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(iri); i++ {
		c := iri[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			strings.IndexByte(safe, c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0x0f])
		}
	}
	return b.String()
}
