package checks_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/conf"
)

// TestXFrameOptionsAsTheMiddlewareSendsIt warns about the X-Frame-Options
// value the clickjacking middleware sends, not about the spelling of the
// setting: the middleware upper-cases the value and sends DENY for the empty
// string.
func TestXFrameOptionsAsTheMiddlewareSendsIt(t *testing.T) {
	tests := []struct {
		value string
		warns bool
	}{
		{"deny", false},
		{"", false},
		{"sameorigin", true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			s := conf.Defaults()
			s.XFrameOptions = tt.value
			if tt.warns {
				assertMessages(t, deployMessages(s, "security.W019"), secW019)
			} else {
				assertMessages(t, deployMessages(s, "security.W019"))
			}
		})
	}
}

// TestNegativeHSTSSecondsIsNoHSTS treats a negative SECURE_HSTS_SECONDS as the
// security middleware does: no Strict-Transport-Security header is sent, so
// the check asks for a value and does not ask for the options of the header.
func TestNegativeHSTSSecondsIsNoHSTS(t *testing.T) {
	s := conf.Defaults()
	s.SecureHSTSSeconds = -1

	assertMessages(t, deployMessages(s, "security.W004", "security.W005", "security.W021"), secW004)
}
