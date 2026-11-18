package forms

import (
	"net/netip"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The host name rules below restate Django's regular expressions as code:
// they rely on lookahead and lookbehind, which Go's regexp package does not
// have.

// isHostRune reports whether r may appear in a host name label: an ASCII
// letter or digit, or a character of Django's "Unicode letters" range.
func isHostRune(r rune, ascii bool) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case ascii:
		return false
	default:
		return r >= 0xa1 && r <= 0xffff
	}
}

// validLabel reports whether s is a host name label: 1 to 63 host characters
// or hyphens, neither starting nor ending with a hyphen.
func validLabel(s string, ascii bool) bool {
	n := utf8.RuneCountInString(s)
	if n < 1 || n > 63 || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		if r != '-' && !isHostRune(r, ascii) {
			return false
		}
	}
	return true
}

// validTLD reports whether s is a top-level domain label: 2 to 63 letters or
// hyphens (no digits, unless ascii allows them), or a punycode label.
func validTLD(s string, ascii bool) bool {
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	lower := strings.ToLower(s)
	if !ascii && strings.HasPrefix(lower, "xn--") {
		rest := lower[4:]
		if len(rest) >= 1 && len(rest) <= 59 && strings.IndexFunc(rest, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		}) < 0 {
			return true
		}
	}
	n := utf8.RuneCountInString(s)
	if n < 2 || n > 63 {
		return false
	}
	for _, r := range s {
		if r == '-' {
			continue
		}
		if ascii {
			if !isHostRune(r, true) {
				return false
			}
			continue
		}
		if !isHostRune(r, false) || unicode.IsDigit(r) && r < 0x80 {
			return false
		}
	}
	return true
}

// validDomain reports whether s is a dotted host name with a top-level
// domain. trailingDot allows the root dot of a fully qualified name.
func validDomain(s string, ascii, trailingDot bool) bool {
	if trailingDot {
		s = strings.TrimSuffix(s, ".")
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return false
	}
	// The first label must not contain a hyphen at either end and, like the
	// middle ones, holds at most 63 characters.
	for _, label := range labels[:len(labels)-1] {
		if !validLabel(label, ascii) {
			return false
		}
	}
	return validTLD(labels[len(labels)-1], ascii)
}

// DomainNameValidator returns a validator for a domain name, Django's
// DomainNameValidator. With acceptIDNA false only ASCII names pass.
func DomainNameValidator(acceptIDNA bool) Validator {
	return func(value any) error {
		s, ok := value.(string)
		if !ok || utf8.RuneCountInString(s) > 255 {
			return invalid("Enter a valid domain name.", value)
		}
		if !acceptIDNA && !isASCII(s) {
			return invalid("Enter a valid domain name.", value)
		}
		if !validDomain(s, !acceptIDNA, true) {
			return invalid("Enter a valid domain name.", value)
		}
		return nil
	}
}

// ValidateDomainName accepts a domain name including internationalized ones,
// Django's validate_domain_name.
var ValidateDomainName = DomainNameValidator(true)

// isASCII reports whether s consists of ASCII characters only.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// ipv4Address reports whether s is a dotted IPv4 address without leading
// zeros, as Python's ipaddress.IPv4Address requires.
func ipv4Address(s string) bool {
	addr, err := netip.ParseAddr(s)
	return err == nil && addr.Is4()
}

// ipv6Address reports whether s is an IPv6 address in one of its textual
// forms.
func ipv6Address(s string) bool {
	if len(s) > 39 && !strings.Contains(s, ".") || strings.Contains(s, "%") {
		return false
	}
	addr, err := netip.ParseAddr(s)
	return err == nil && addr.Is6()
}

// ipError is the error of the IP address validators.
func ipError(protocol string, value any) error {
	return NewValidationError("Enter a valid %(protocol)s address.", "invalid",
		map[string]any{"protocol": protocol, "value": value})
}

// ValidateIPv4Address accepts a dotted IPv4 address, Django's
// validate_ipv4_address.
func ValidateIPv4Address(value any) error {
	if s, ok := value.(string); !ok || !ipv4Address(s) {
		return ipError("IPv4", value)
	}
	return nil
}

// ValidateIPv6Address accepts an IPv6 address, Django's validate_ipv6_address.
func ValidateIPv6Address(value any) error {
	if s, ok := value.(string); !ok || !ipv6Address(s) {
		return ipError("IPv6", value)
	}
	return nil
}

// ValidateIPv46Address accepts an IPv4 or IPv6 address, Django's
// validate_ipv46_address.
func ValidateIPv46Address(value any) error {
	if s, ok := value.(string); !ok || !ipv4Address(s) && !ipv6Address(s) {
		return ipError("IPv4 or IPv6", value)
	}
	return nil
}

// Patterns of the local part and the literal domain of an email address, the
// user_regex and literal_regex of Django's EmailValidator.
var (
	emailUserRE = regexp.MustCompile("(?i)^[-!#$%&'*+/=?^_`{}|~0-9A-Z]+(\\.[-!#$%&'*+/=?^_`{}|~0-9A-Z]+)*\\z" +
		`|^"([\x01-\x08\x0b\x0c\x0e-\x1f!#-\[\]-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*"\z`)
	emailLiteralRE = regexp.MustCompile(`(?i)^\[([A-F0-9:.]+)\]\z`)
)

// EmailValidator returns a validator for an email address, Django's
// EmailValidator. The allowlist names domains that pass without a top-level
// domain; without arguments it is "localhost".
func EmailValidator(allowlist ...string) Validator {
	if len(allowlist) == 0 {
		allowlist = []string{"localhost"}
	}
	const message = "Enter a valid email address."
	return func(value any) error {
		s, ok := value.(string)
		// RFC 3696 section 3 limits an address to 320 characters.
		if !ok || s == "" || !strings.Contains(s, "@") || utf8.RuneCountInString(s) > 320 {
			return invalid(message, value)
		}
		at := strings.LastIndex(s, "@")
		user, domain := s[:at], s[at+1:]
		if !emailUserRE.MatchString(user) {
			return invalid(message, value)
		}
		for _, allowed := range allowlist {
			if domain == allowed {
				return nil
			}
		}
		if validDomain(domain, false, false) {
			return nil
		}
		if m := emailLiteralRE.FindStringSubmatch(domain); m != nil && (ipv4Address(m[1]) || ipv6Address(m[1])) {
			return nil
		}
		return invalid(message, value)
	}
}

// ValidateEmail accepts an email address, Django's validate_email.
var ValidateEmail = EmailValidator()

// Patterns of the URL parts that have no lookaround in Django's URLValidator.
var (
	urlSchemeRE   = regexp.MustCompile(`(?i)^[a-z0-9.+-]*\z`)
	urlUserInfoRE = regexp.MustCompile(`^[^\s:@/]+(?::[^\s:@/]*)?\z`)
	urlIPv6RE     = regexp.MustCompile(`(?i)^\[[0-9a-f:.]+\]`)
	urlPortRE     = regexp.MustCompile(`^:[0-9]{1,5}`)
	urlNetlocV6RE = regexp.MustCompile(`^\[(.+)\](?::[0-9]{1,5})?$`)
)

// URLValidator returns a validator for a URL, Django's URLValidator. Without
// arguments the schemes http, https, ftp and ftps are accepted.
func URLValidator(schemes ...string) Validator {
	if len(schemes) == 0 {
		schemes = []string{"http", "https", "ftp", "ftps"}
	}
	const message = "Enter a valid URL."
	return func(value any) error {
		s, ok := value.(string)
		if !ok || utf8.RuneCountInString(s) > 2048 || strings.ContainsAny(s, "\t\r\n") {
			return invalid(message, value)
		}
		scheme, rest, found := strings.Cut(s, "://")
		if !found || !urlSchemeRE.MatchString(scheme) {
			return invalid(message, value)
		}
		scheme = strings.ToLower(scheme)
		known := false
		for _, allowed := range schemes {
			known = known || scheme == allowed
		}
		if !known {
			return invalid(message, value)
		}
		host, ok := urlHost(rest)
		if !ok {
			return invalid(message, value)
		}
		// The pattern only checks the characters of a bracketed host.
		netloc := rest
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			netloc = rest[:i]
		}
		if m := urlNetlocV6RE.FindStringSubmatch(netloc); m != nil && !ipv6Address(m[1]) {
			return invalid(message, value)
		}
		// RFC 1034 section 3.1 limits a full host name to 253 characters.
		if host == "" || utf8.RuneCountInString(host) > 253 {
			return invalid(message, value)
		}
		return nil
	}
}

// ValidateURL accepts an http, https, ftp or ftps URL.
var ValidateURL = URLValidator()

// urlHost matches the part of a URL after "://" against Django's pattern
// (optional user info, host, optional port, optional path) and returns the
// host.
func urlHost(rest string) (string, bool) {
	if at := strings.Index(rest, "@"); at >= 0 && urlUserInfoRE.MatchString(rest[:at]) {
		if host, ok := urlHostPort(rest[at+1:]); ok {
			return host, true
		}
	}
	return urlHostPort(rest)
}

// urlHostPort matches host, optional port and optional path at the start of s.
func urlHostPort(s string) (string, bool) {
	var host string
	if strings.HasPrefix(s, "[") {
		host = urlIPv6RE.FindString(s)
		if host == "" {
			return "", false
		}
	} else {
		end := strings.IndexAny(s, ":/?#")
		if end < 0 {
			end = len(s)
		}
		host = s[:end]
		if !ipv4Address(host) && !validDomain(host, false, true) && !strings.EqualFold(host, "localhost") {
			return "", false
		}
	}
	tail := s[len(host):]
	tail = tail[len(urlPortRE.FindString(tail)):]
	if tail == "" {
		return strings.Trim(host, "[]"), true
	}
	if !strings.ContainsRune("/?#", rune(tail[0])) || strings.IndexFunc(tail, unicode.IsSpace) >= 0 {
		return "", false
	}
	return strings.Trim(host, "[]"), true
}
