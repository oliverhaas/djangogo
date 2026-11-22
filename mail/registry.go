package mail

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// headerKind is the parser of a header, as the classes of Python's
// email.headerregistry choose it by name.
type headerKind int

// The header kinds.
const (
	kindUnstructured headerKind = iota
	kindAddress
	kindDate
	kindMessageID
	kindReferences
)

// headerKindOf returns the kind of the header called name.
func headerKindOf(name string) headerKind {
	switch strings.ToLower(name) {
	case "to", "cc", "bcc", "from", "reply-to", "sender",
		"resent-to", "resent-cc", "resent-bcc", "resent-from", "resent-sender":
		return kindAddress
	case "date", "orig-date", "resent-date":
		return kindDate
	case "message-id":
		return kindMessageID
	case "in-reply-to", "references":
		return kindReferences
	}
	return kindUnstructured
}

// uniqueHeader reports whether a message holds at most one header called
// name, as the max_count of Python's header classes says.
func uniqueHeader(name string) bool {
	switch strings.ToLower(name) {
	case "subject", "date", "orig-date", "sender", "to", "cc", "bcc", "from", "reply-to",
		"mime-version", "content-type", "content-disposition", "content-transfer-encoding", "message-id",
		"in-reply-to", "references":
		return true
	}
	return false
}

// errHeaderNewline reports a header value with a line break, which Python's
// policy refuses to store.
var errHeaderNewline = errors.New("Header values may not contain linefeed or carriage return characters")

// checkHeaderValue fails when value holds a line break, as Python's
// header_store_parse does: a value that str.splitlines() splits into more
// than one line is refused, so a single trailing line break passes.
func checkHeaderValue(value string) error {
	lines := 0
	rest := value
	for rest != "" {
		i, size := pyLineBreak(rest)
		lines++
		if i < 0 {
			break
		}
		rest = rest[i+size:]
	}
	if lines > 1 {
		return errHeaderNewline
	}
	return nil
}

// validateHeaderName fails for a name that is empty or holds a character
// other than printable ASCII without the colon, as Python's
// validate_header_name does. Its pattern ends in "$", which also accepts
// one trailing newline.
func validateHeaderName(name string) error {
	n := strings.TrimSuffix(name, "\n")
	valid := n != ""
	for i := 0; i < len(n); i++ {
		if c := n[i]; c < '!' || c > '~' || c == ':' {
			valid = false
			break
		}
	}
	if !valid {
		return fmt.Errorf("Header field name contains invalid characters: %s", pyRepr(name))
	}
	return nil
}

// pyLineBreak returns the byte offset and size of the first line break of s
// that Python's str.splitlines() splits at, or -1.
func pyLineBreak(s string) (int, int) {
	for i, r := range s {
		switch r {
		case '\r':
			if strings.HasPrefix(s[i:], "\r\n") {
				return i, 2
			}
			return i, 1
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e:
			return i, 1
		case 0x85, 0x2028, 0x2029:
			return i, len(string(r))
		}
	}
	return -1, 0
}

// parseHeader returns the parse tree of the header name: value, as the
// header class that Python's registry picks for name builds it. It fails
// where Python's parser raises an IndexError.
func parseHeader(name, value string) (tree *tok, err error) {
	switch headerKindOf(name) {
	case kindAddress:
		return parseAddressList(value), nil
	case kindDate:
		t, naive, ok := parsePyDate(value)
		if !ok {
			return newList("token-list"), nil
		}
		return parseUnstructured(formatPyDate(t, naive)), nil
	case kindMessageID:
		return parseMessageID(value), nil
	case kindReferences:
		return parseMessageIDs(value), nil
	}
	return parseUnstructured(value), nil
}

// monthNames are the month names Python's date parser accepts, short and
// long.
var monthNames = []string{
	"jan", "feb", "mar", "apr", "may", "jun", "jul",
	"aug", "sep", "oct", "nov", "dec",
	"january", "february", "march", "april", "may", "june", "july",
	"august", "september", "october", "november", "december",
}

// dayNames are the day names Python's date parser skips.
var dayNames = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// zoneOffsets are the zone names Python's date parser knows, as hhmm.
var zoneOffsets = map[string]int{
	"UT": 0, "UTC": 0, "GMT": 0, "Z": 0,
	"AST": -400, "ADT": -300, "EST": -500, "EDT": -400, "CST": -600,
	"CDT": -500, "MST": -700, "MDT": -600, "PST": -800, "PDT": -700,
}

// parsePyDate parses an RFC 2822 date as Python's parsedate_to_datetime
// does. naive is true for "-0000" and unknown zones, which Python returns as
// a datetime without a zone.
func parsePyDate(value string) (t time.Time, naive, ok bool) {
	data := strings.FieldsFunc(value, pyIsSpace)
	if len(data) == 0 {
		return time.Time{}, false, false
	}
	if strings.HasSuffix(data[0], ",") || indexOf(dayNames, strings.ToLower(data[0])) >= 0 {
		data = data[1:]
	} else if i := strings.LastIndex(data[0], ","); i >= 0 {
		data[0] = data[0][i+1:]
	}
	if len(data) == 3 {
		if stuff := strings.Split(data[0], "-"); len(stuff) == 3 {
			data = append(stuff, data[1:]...)
		}
	}
	if len(data) == 4 {
		s := data[3]
		i := strings.Index(s, "+")
		if i == -1 {
			i = strings.Index(s, "-")
		}
		if i > 0 {
			data = append(data[:3:3], s[:i], s[i:])
		} else {
			data = append(data, "")
		}
	}
	if len(data) < 5 {
		return time.Time{}, false, false
	}
	dd, mm, yy, tm, tz := data[0], data[1], data[2], data[3], data[4]
	if dd == "" || mm == "" || yy == "" {
		return time.Time{}, false, false
	}
	mm = strings.ToLower(mm)
	if indexOf(monthNames, mm) < 0 {
		dd, mm = mm, strings.ToLower(dd)
		if indexOf(monthNames, mm) < 0 {
			return time.Time{}, false, false
		}
	}
	month := indexOf(monthNames, mm) + 1
	if month > 12 {
		month -= 12
	}
	dd = strings.TrimSuffix(dd, ",")
	if strings.Index(yy, ":") > 0 {
		yy, tm = tm, yy
	}
	if strings.HasSuffix(yy, ",") {
		yy = yy[:len(yy)-1]
		if yy == "" {
			return time.Time{}, false, false
		}
	}
	if yy == "" || yy[0] < '0' || yy[0] > '9' {
		yy, tz = tz, yy
	}
	tm = strings.TrimSuffix(tm, ",")
	parts := strings.Split(tm, ":")
	switch {
	case len(parts) == 2:
		parts = append(parts, "0")
	case len(parts) == 3:
	case len(parts) == 1 && strings.Contains(tm, "."):
		parts = strings.Split(tm, ".")
		switch len(parts) {
		case 2:
			parts = append(parts, "0")
		case 3:
		default:
			return time.Time{}, false, false
		}
	default:
		return time.Time{}, false, false
	}
	nums := make([]int, 0, 5)
	for _, s := range []string{yy, dd, parts[0], parts[1], parts[2]} {
		n, err := pyInt(s)
		if err != nil {
			return time.Time{}, false, false
		}
		nums = append(nums, n)
	}
	year, day, hour, minute, second := nums[0], nums[1], nums[2], nums[3], nums[4]
	if year < 100 {
		if year > 68 {
			year += 1900
		} else {
			year += 2000
		}
	}
	tz = strings.ToUpper(tz)
	offset, known := zoneOffsets[tz]
	if !known {
		if n, err := pyInt(tz); err == nil {
			offset, known = n, true
			if n == 0 && strings.HasPrefix(tz, "-") {
				known = false
			}
		}
	}
	if !validDate(year, month, day, hour, minute, second) {
		return time.Time{}, false, false
	}
	if !known {
		return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC), true, true
	}
	sign := 1
	if offset < 0 {
		sign, offset = -1, -offset
	}
	seconds := sign * ((offset/100)*3600 + (offset%100)*60)
	if seconds <= -86400 || seconds >= 86400 {
		return time.Time{}, false, false
	}
	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.FixedZone("", seconds)), false, true
}

// validDate reports whether the fields make a datetime that Python accepts.
func validDate(year, month, day, hour, minute, second int) bool {
	if year < 1 || year > 9999 || day < 1 || hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
		return false
	}
	return day <= time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// pyInt parses a decimal integer as Python's int() parses a str: surrounding
// whitespace and a sign are allowed.
func pyInt(s string) (int, error) {
	s = strings.TrimFunc(s, pyIsSpace)
	digits := strings.TrimLeft(s, "+-")
	if len(s)-len(digits) > 1 || digits == "" || strings.Trim(digits, "0123456789") != "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.Atoi(strings.TrimPrefix(s, "+"))
}

// indexOf returns the index of s in list, or -1.
func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// formatPyDate formats t as Python's email.utils.format_datetime does. A
// naive time gets the zone "-0000".
func formatPyDate(t time.Time, naive bool) string {
	zone := "-0000"
	if !naive {
		zone = t.Format("-0700")
	}
	return t.Format("Mon, 02 Jan ") + fmt.Sprintf("%04d", t.Year()) + t.Format(" 15:04:05 ") + zone
}
