package duration_test

import (
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/internal/duration"
)

// Ported from Django: tests/utils_tests/test_duration.py::TestDurationString
// Check that String formats simple, multi-day, microsecond, and negative durations.
func TestDurationString(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{time.Hour + 3*time.Minute + 5*time.Second, "01:03:05"},
		{24*time.Hour + time.Hour + 3*time.Minute + 5*time.Second, "1 01:03:05"},
		{time.Hour + 3*time.Minute + 5*time.Second + 12345*time.Microsecond, "01:03:05.012345"},
		{-(24*time.Hour + time.Hour + 3*time.Minute + 5*time.Second), "-1 01:03:05"},
	}
	for _, tc := range cases {
		if got := duration.String(tc.d); got != tc.want {
			t.Errorf("String(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// Ported from Django: tests/utils_tests/test_duration.py::TestISODurationString
// Check that ISOString formats simple, multi-day, microsecond, and negative durations.
func TestISODurationString(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{time.Hour + 3*time.Minute + 5*time.Second, "P0DT01H03M05S"},
		{24*time.Hour + time.Hour + 3*time.Minute + 5*time.Second, "P1DT01H03M05S"},
		{time.Hour + 3*time.Minute + 5*time.Second + 12345*time.Microsecond, "P0DT01H03M05.012345S"},
		{-(24*time.Hour + time.Hour + 3*time.Minute + 5*time.Second), "-P1DT01H03M05S"},
	}
	for _, tc := range cases {
		if got := duration.ISOString(tc.d); got != tc.want {
			t.Errorf("ISOString(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// Ported from Django: tests/utils_tests/test_duration.py::TestParseDurationRoundtrip
// Check that Parse roundtrips String and ISOString representations and reports overflow and invalid input.
func TestParseDurationRoundtrip(t *testing.T) {
	deltas := []time.Duration{
		0,
		time.Hour + 3*time.Minute + 5*time.Second,
		24*time.Hour + time.Hour + 3*time.Minute + 5*time.Second,
		time.Hour + 3*time.Minute + 5*time.Second + 12345*time.Microsecond,
	}
	for _, d := range deltas {
		got, ok, overflow := duration.Parse(duration.String(d))
		if !ok || overflow || got != d {
			t.Errorf("Parse(String(%v)) = (%v, %v, %v), want (%v, true, false)", d, got, ok, overflow, d)
		}
		gotISO, okISO, overflowISO := duration.Parse(duration.ISOString(d))
		if !okISO || overflowISO || gotISO != d {
			t.Errorf("Parse(ISOString(%v)) = (%v, %v, %v), want (%v, true, false)", d, gotISO, okISO, overflowISO, d)
		}
	}

	neg := -(24*time.Hour + time.Hour + 3*time.Minute + 5*time.Second)
	gotNegISO, okNegISO, overflowNegISO := duration.Parse(duration.ISOString(neg))
	if !okNegISO || overflowNegISO || gotNegISO != neg {
		t.Errorf("Parse(ISOString(%v)) = (%v, %v, %v), want (%v, true, false)", neg, gotNegISO, okNegISO, overflowNegISO, neg)
	}

	if _, ok, _ := duration.Parse("not_a_duration"); ok {
		t.Error("Parse(not_a_duration) ok = true, want false")
	}
	if _, ok, overflow := duration.Parse("1000000000 00:00:00"); !ok || !overflow {
		t.Errorf("Parse(overflow) = (ok=%v, overflow=%v), want (true, true)", ok, overflow)
	}
}
