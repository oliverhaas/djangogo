package auth

import (
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// maxURLLength is Django's MAX_URL_LENGTH: longer redirect targets are refused.
const maxURLLength = 2048

// URLHasAllowedHostAndScheme reports whether rawURL is a safe target for a
// redirect that a visitor can choose, such as the next parameter of the login
// page (Django's url_has_allowed_host_and_scheme). A URL without a host is
// allowed; one with a host only when the host is in allowedHosts, compared as
// written, and only with the scheme http or https. With requireHTTPS, http is
// refused as well. The empty URL is never allowed.
//
// A true result does not mean that the URL is well formed; it means that a
// browser will not leave the allowed hosts when it follows it.
func URLHasAllowedHostAndScheme(rawURL string, allowedHosts []string, requireHTTPS bool) bool {
	rawURL = strings.TrimFunc(rawURL, isPythonSpace)
	if rawURL == "" {
		return false
	}
	// Chrome reads a backslash as a slash in paths, but it may also be part
	// of basic auth credentials, so both readings have to be allowed.
	return urlAllowed(rawURL, allowedHosts, requireHTTPS) &&
		urlAllowed(strings.ReplaceAll(rawURL, `\`, "/"), allowedHosts, requireHTTPS)
}

// urlAllowed is the check of URLHasAllowedHostAndScheme for one reading of
// the URL.
func urlAllowed(rawURL string, allowedHosts []string, requireHTTPS bool) bool {
	// Chrome treats a URL with more than two leading slashes as absolute.
	if strings.HasPrefix(rawURL, "///") || utf8.RuneCountInString(rawURL) > maxURLLength {
		return false
	}
	scheme, netloc, err := splitURL(rawURL)
	if err != nil {
		return false
	}
	// "http:///example.com" has a scheme and no host, but Chrome takes
	// example.com for the host.
	if netloc == "" && scheme != "" {
		return false
	}
	// Browsers skip control characters in front of a URL and may then read
	// it as scheme relative.
	if first, _ := utf8.DecodeRuneInString(rawURL); !unicode.IsGraphic(first) {
		return false
	}
	if scheme == "" && netloc != "" {
		scheme = "http"
	}
	if netloc != "" && !slices.Contains(allowedHosts, netloc) {
		return false
	}
	switch scheme {
	case "", "https":
		return true
	case "http":
		return !requireHTTPS
	default:
		return false
	}
}

// isPythonSpace reports whether Python's str.strip removes r: Unicode white
// space and the four ASCII separators.
func isPythonSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// errInvalidURL is the ValueError of Python's urlsplit.
var errInvalidURL = errors.New("auth: invalid URL")

// Character sets of Python's urllib.parse.
const (
	// schemeChars are the characters of a URL scheme.
	schemeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-."
	// c0ControlOrSpace is removed from the front of a URL, as the WHATWG
	// URL parser of browsers does.
	c0ControlOrSpace = "\x00\x01\x02\x03\x04\x05\x06\x07\x08\t\n\x0b\x0c\r\x0e\x0f" +
		"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f "
)

// unsafeURLBytes removes the characters browsers drop from anywhere in a URL.
var unsafeURLBytes = strings.NewReplacer("\t", "", "\r", "", "\n", "")

// splitURL returns the scheme and the network location of rawURL the way
// Python's urllib.parse.urlsplit does. The allowed-host check depends on that
// reading: net/url rejects or reads differently several inputs that browsers
// accept.
func splitURL(rawURL string) (scheme, netloc string, err error) {
	u := unsafeURLBytes.Replace(strings.TrimLeft(rawURL, c0ControlOrSpace))
	if i := strings.IndexByte(u, ':'); i > 0 && isASCIILetter(u[0]) && strings.Trim(u[:i], schemeChars) == "" {
		scheme, u = strings.ToLower(u[:i]), u[i+1:]
	}
	if rest, ok := strings.CutPrefix(u, "//"); ok {
		netloc = rest
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			netloc = rest[:i]
		}
		if !validBrackets(netloc) {
			return "", "", errInvalidURL
		}
	}
	if !validNetloc(netloc) {
		return "", "", errInvalidURL
	}
	return scheme, netloc, nil
}

// isASCIILetter reports whether c is an ASCII letter.
func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ipvFuture matches the IPvFuture form of a bracketed host.
var ipvFuture = regexp.MustCompile(`^v[a-fA-F0-9]+\..+$`)

// validBrackets checks the square brackets of a network location: they come
// in a pair around an IPv6 or IPvFuture address, with nothing before them
// except credentials and nothing after them except a port.
func validBrackets(netloc string) bool {
	hasOpen, hasClose := strings.Contains(netloc, "["), strings.Contains(netloc, "]")
	if hasOpen != hasClose {
		return false
	}
	if !hasOpen {
		return true
	}
	hostPort := netloc[strings.LastIndex(netloc, "@")+1:]
	before, bracketed, _ := strings.Cut(hostPort, "[")
	if before != "" {
		return false
	}
	host, port, _ := strings.Cut(bracketed, "]")
	if port != "" && !strings.HasPrefix(port, ":") {
		return false
	}
	if strings.HasPrefix(host, "v") {
		return ipvFuture.MatchString(host)
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Is6()
}

// validNetloc refuses a network location with characters that turn into URL
// delimiters under NFKC normalization, such as U+2100, which becomes "a/c".
// Browsers normalize host names that way.
func validNetloc(netloc string) bool {
	if isASCII(netloc) {
		return true
	}
	n := strings.NewReplacer("@", "", ":", "", "#", "", "?", "").Replace(netloc)
	normalized := norm.NFKC.String(n)
	return n == normalized || !strings.ContainsAny(normalized, "/?#@:")
}

// isASCII reports whether s holds only ASCII bytes.
func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// RedirectToLoginURL returns the URL of the login page with next as the page
// to return to after the login (the URL of Django's redirect_to_login). An
// empty loginURL means Config.LoginURL. The parameter is named
// redirectFieldName; with an empty name the login URL is returned without it.
// A parameter of that name in loginURL is replaced.
//
// next is quoted the way Django quotes it: "/" stays readable.
func RedirectToLoginURL(next, loginURL, redirectFieldName string) string {
	if loginURL == "" {
		loginURL = configuredLoginURL()
	}
	if redirectFieldName == "" {
		return loginURL
	}
	base, fragment, hasFragment := strings.Cut(loginURL, "#")
	path, query, _ := strings.Cut(base, "?")

	var out []string
	replaced := false
	for pair := range strings.SplitSeq(query, "&") {
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		key, value = unquotePlus(key), unquotePlus(value)
		if key == redirectFieldName {
			if replaced {
				continue
			}
			replaced, value = true, next
		}
		out = append(out, quote(key)+"="+quote(value))
	}
	if !replaced {
		out = append(out, quote(redirectFieldName)+"="+quote(next))
	}
	target := path + "?" + strings.Join(out, "&")
	if hasFragment {
		target += "#" + fragment
	}
	return target
}

// RedirectToLogin answers with a redirect to the login page, passing next as
// the page to return to (Django's redirect_to_login). An empty loginURL means
// Config.LoginURL.
func RedirectToLogin(w http.ResponseWriter, next, loginURL string) {
	redirect(w, RedirectToLoginURL(next, loginURL, DefaultRedirectFieldName))
}

// redirect answers 302 with target as the location, escaped like Django's
// HttpResponseRedirect and otherwise unchanged. http.Redirect is not used on
// purpose: it cleans the path, which turns a checked target such as
// "/./\example.com" into "/\example.com", and browsers read that as another
// host.
func redirect(w http.ResponseWriter, target string) {
	w.Header().Set("Location", iriToURI(target))
	w.WriteHeader(http.StatusFound)
}

// iriToURI is Django's iri_to_uri: it percent-encodes every byte that may not
// appear in a URI and keeps existing escapes. The backslash is among the
// encoded bytes, so a browser cannot read it as a slash.
func iriToURI(s string) string {
	return quoteExcept(s, "/#%[]=:;$&()+,!?*@'~")
}

// unquotePlus decodes a query string component. A malformed escape is kept as
// it is, as Python's unquote_plus does.
func unquotePlus(s string) string {
	if decoded, err := url.QueryUnescape(s); err == nil {
		return decoded
	}
	return strings.ReplaceAll(s, "+", " ")
}

// quote is Python's urllib.parse.quote with safe="/": letters, digits and
// "_.-~/" stay, every other byte is percent encoded.
func quote(s string) string { return quoteExcept(s, "/") }

// quoteExcept is Python's urllib.parse.quote: letters, digits, "_.-~" and the
// bytes of safe stay, every other byte is percent encoded.
func quoteExcept(s, safe string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if isASCIILetter(c) || (c >= '0' && c <= '9') || strings.IndexByte("_.-~", c) >= 0 ||
			strings.IndexByte(safe, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0f])
	}
	return b.String()
}
