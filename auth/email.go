package auth

import (
	"net/netip"
	"strings"
	"unicode/utf8"
)

// msgInvalidEmail is the message of ValidateEmail, in Django's wording.
const msgInvalidEmail = "Enter a valid email address."

// EmailError is returned by ValidateEmail for an address that is not valid.
// Its text is Django's message for the user.
type EmailError struct {
	// Message is the text for the user.
	Message string
}

// Error returns the message.
func (e *EmailError) Error() string { return e.Message }

// maxIPv6Length is the longest IPv6 address text Django accepts
// (django.utils.ipv6.MAX_IPV6_ADDRESS_LENGTH).
const maxIPv6Length = 39

// ValidateEmail returns nil for a valid email address and an *EmailError
// otherwise (Django's validate_email, the validator of the email column).
// The part before the last "@" is a dot-atom or a quoted string; the part
// after it is a host name with a top-level domain, "localhost", or an IP
// address in square brackets. The address may be at most 320 characters
// long.
func ValidateEmail(email string) error {
	if email == "" || !utf8.ValidString(email) || utf8.RuneCountInString(email) > 320 {
		return &EmailError{Message: msgInvalidEmail}
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 || !validEmailUser(email[:at]) || !validEmailDomain(email[at+1:]) {
		return &EmailError{Message: msgInvalidEmail}
	}
	return nil
}

// foldsToASCIILetter reports whether r is one of the four non-ASCII
// characters that Python's case-insensitive regular expressions match against
// an ASCII letter: U+0130 and U+0131 (i), U+017F (s) and U+212A (k). Django's
// email patterns are case-insensitive, so they accept them where an ASCII
// letter is allowed.
func foldsToASCIILetter(r rune) bool {
	return r == 0x130 || r == 0x131 || r == 0x17f || r == 0x212a
}

// isASCIIAlnum reports whether r is an ASCII letter or digit, or a character
// that folds to an ASCII letter.
func isASCIIAlnum(r rune) bool {
	if r >= utf8.RuneSelf {
		return foldsToASCIILetter(r)
	}
	return isASCIILetter(byte(r)) || r >= '0' && r <= '9'
}

// atomChars are the characters of a dot-atom besides ASCII letters and digits.
const atomChars = "-!#$%&'*+/=?^_`{}|~"

// validEmailUser reports whether user is a dot-atom or a quoted string, as
// Django's user_regex checks it.
func validEmailUser(user string) bool {
	if len(user) >= 2 && user[0] == '"' && user[len(user)-1] == '"' {
		return validQuotedString(user[1 : len(user)-1])
	}
	for atom := range strings.SplitSeq(user, ".") {
		if atom == "" {
			return false
		}
		for _, r := range atom {
			if !isASCIIAlnum(r) && (r >= utf8.RuneSelf || strings.IndexByte(atomChars, byte(r)) < 0) {
				return false
			}
		}
	}
	return true
}

// validQuotedString checks the inside of a quoted local part. Allowed are the
// ASCII characters from U+0001 to U+007F except tab, line feed, carriage
// return, space, '"' and '\', and a backslash followed by any ASCII character
// from U+0001 to U+007F except line feed and carriage return.
func validQuotedString(s string) bool {
	escaped := false
	for _, r := range s {
		if r >= utf8.RuneSelf {
			if !foldsToASCIILetter(r) {
				return false
			}
			escaped = false
			continue
		}
		switch {
		case escaped:
			if r == 0 || r == '\n' || r == '\r' {
				return false
			}
			escaped = false
		case r == '\\':
			escaped = true
		case r == 0 || r == '\t' || r == '\n' || r == '\r' || r == ' ' || r == '"':
			return false
		}
	}
	return !escaped
}

// validEmailDomain reports whether domain is "localhost", a host name with a
// top-level domain, or a bracketed IPv4 or IPv6 address.
func validEmailDomain(domain string) bool {
	if domain == "localhost" {
		return true
	}
	if inner, ok := strings.CutPrefix(domain, "["); ok {
		inner, ok = strings.CutSuffix(inner, "]")
		if !ok || inner == "" || strings.Trim(inner, "0123456789abcdefABCDEF:.") != "" {
			return false
		}
		if strings.Contains(inner, ":") && len(inner) > maxIPv6Length {
			return false
		}
		_, err := netip.ParseAddr(inner)
		return err == nil
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels[:len(labels)-1] {
		if !validDomainLabel(label, true, 1) {
			return false
		}
	}
	tld := labels[len(labels)-1]
	if rest, ok := cutPrefixFold(tld, "xn--"); ok && validPunycode(rest) {
		return true
	}
	return validDomainLabel(tld, false, 2)
}

// validDomainLabel checks one label of a host name: minLen to 63 characters,
// letters (ASCII and from U+00A1 to U+FFFF), digits when digits is set, and
// hyphens, but no hyphen at either end.
func validDomainLabel(label string, digits bool, minLen int) bool {
	n := utf8.RuneCountInString(label)
	if n < minLen || n > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, r := range label {
		switch {
		case r < utf8.RuneSelf && isASCIILetter(byte(r)), r == '-':
		case r >= '0' && r <= '9':
			if !digits {
				return false
			}
		case r >= 0xa1 && r <= 0xffff:
		default:
			return false
		}
	}
	return true
}

// validPunycode checks the part of a top-level domain after "xn--": 1 to 59
// ASCII letters and digits.
func validPunycode(s string) bool {
	n := 0
	for _, r := range s {
		if !isASCIIAlnum(r) {
			return false
		}
		n++
	}
	return n >= 1 && n <= 59
}

// cutPrefixFold is strings.CutPrefix with an ASCII case-insensitive prefix.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return s, false
	}
	return s[len(prefix):], true
}
