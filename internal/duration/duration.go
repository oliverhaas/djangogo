// Package duration parses and formats time.Duration values using Django's
// django.utils.dateparse and django.utils.duration rules.
package duration

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Patterns of the accepted duration spellings, after Django's
// standard_duration_re and iso8601_duration_re.
var (
	standardDurationRE = regexp.MustCompile(`^(?:(-?\d+) (?:days?, )?)?(-?)(?:(?:(\d+):)?(\d+):)?(\d+)(?:[.,](\d{1,6})\d{0,6})?$`)
	isoDurationRE      = regexp.MustCompile(`^([-+]?)P(?:(\d+(?:[.,]\d+)?)D)?(?:T(?:(\d+(?:[.,]\d+)?)H)?(?:(\d+(?:[.,]\d+)?)M)?(?:(\d+(?:[.,]\d+)?)S)?)?$`)
)

// Parse parses a duration in Django's standard "[days] [[hours:]minutes:]seconds"
// or ISO 8601 format; ok is false for an unknown spelling and overflow is true
// for a value outside the range of time.Duration.
func Parse(s string) (d time.Duration, ok, overflow bool) {
	var seconds float64
	if m := standardDurationRE.FindStringSubmatch(s); m != nil {
		days, _ := strconv.ParseFloat(orZero(m[1]), 64)
		hours, _ := strconv.ParseFloat(orZero(m[3]), 64)
		minutes, _ := strconv.ParseFloat(orZero(m[4]), 64)
		secs, _ := strconv.ParseFloat(orZero(m[5]), 64)
		micro, _ := strconv.ParseFloat(orZero((m[6] + "000000")[:6]), 64)
		rest := hours*3600 + minutes*60 + secs + micro/1e6
		if m[2] == "-" {
			rest = -rest
		}
		seconds = days*86400 + rest
	} else if m := isoDurationRE.FindStringSubmatch(s); m != nil && s != "P" && !strings.HasSuffix(s, "T") {
		part := func(i int) float64 {
			v, _ := strconv.ParseFloat(strings.Replace(orZero(m[i]), ",", ".", 1), 64)
			return v
		}
		seconds = part(2)*86400 + part(3)*3600 + part(4)*60 + part(5)
		if m[1] == "-" {
			seconds = -seconds
		}
	} else {
		return 0, false, false
	}
	if math.Abs(seconds) > float64(math.MaxInt64)/float64(time.Second) {
		return 0, true, true
	}
	return time.Duration(math.Round(seconds * float64(time.Second))), true, false
}

// orZero maps the empty string to "0".
func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// String renders d as Django's duration_string does:
// "[days ]HH:MM:SS[.ffffff]".
func String(d time.Duration) string {
	sign := ""
	if d < 0 {
		sign, d = "-", -d
	}
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	micro := (d - seconds*time.Second) / time.Microsecond
	out := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	if days != 0 {
		out = fmt.Sprintf("%d %s", days, out)
	}
	if micro != 0 {
		out += fmt.Sprintf(".%06d", micro)
	}
	return sign + out
}

// ISOString renders d as Django's duration_iso_string does:
// "[-]P<days>DT<HH>H<MM>M<SS>[.<ffffff>]S".
func ISOString(d time.Duration) string {
	sign := ""
	if d < 0 {
		sign, d = "-", -d
	}
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	micro := (d - seconds*time.Second) / time.Microsecond
	ms := ""
	if micro != 0 {
		ms = fmt.Sprintf(".%06d", micro)
	}
	return fmt.Sprintf("%sP%dDT%02dH%02dM%02d%sS", sign, days, hours, minutes, seconds, ms)
}
