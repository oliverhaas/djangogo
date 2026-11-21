package forms_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// assertPanics fails t if fn does not panic.
func assertPanics(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", label)
		}
	}()
	fn()
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_invalid_arguments
func TestGenericIPAddressInvalidArguments(t *testing.T) {
	assertPanics(t, "invalid protocol", func() {
		forms.NewGenericIPAddressField(forms.Protocol("hamster"))
	})
	assertPanics(t, "ipv4 with unpack_ipv4", func() {
		forms.NewGenericIPAddressField(forms.Protocol("ipv4"), forms.UnpackIPv4())
	})
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_as_generic
func TestGenericIPAddressAsGeneric(t *testing.T) {
	f := forms.NewGenericIPAddressField()
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, " 127.0.0.1 ", "127.0.0.1")
	rejects(t, f, "foo", "Enter a valid IPv4 or IPv6 address.")
	rejects(t, f, "127.0.0.", "Enter a valid IPv4 or IPv6 address.")
	rejects(t, f, "1.2.3.4.5", "Enter a valid IPv4 or IPv6 address.")
	rejects(t, f, "256.125.1.5", "Enter a valid IPv4 or IPv6 address.")
	cleans(t, f, " fe80::223:6cff:fe8a:2e8a ", "fe80::223:6cff:fe8a:2e8a")
	cleans(t, f, " 2a02::223:6cff:fe8a:2e8a ", "2a02::223:6cff:fe8a:2e8a")
	rejects(t, f, "12345:2:3:4", "This is not a valid IPv6 address.")
	rejects(t, f, "1::2:3::4", "This is not a valid IPv6 address.")
	rejects(t, f, "foo::223:6cff:fe8a:2e8a", "This is not a valid IPv6 address.")
	rejects(t, f, "1::2:3:4:5:6:7:8", "This is not a valid IPv6 address.")
	rejects(t, f, "1:2", "This is not a valid IPv6 address.")
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_as_ipv4_only
func TestGenericIPAddressAsIPv4Only(t *testing.T) {
	f := forms.NewGenericIPAddressField(forms.Protocol("IPv4"))
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, " 127.0.0.1 ", "127.0.0.1")
	rejects(t, f, "foo", "Enter a valid IPv4 address.")
	rejects(t, f, "127.0.0.", "Enter a valid IPv4 address.")
	rejects(t, f, "1.2.3.4.5", "Enter a valid IPv4 address.")
	rejects(t, f, "256.125.1.5", "Enter a valid IPv4 address.")
	rejects(t, f, "fe80::223:6cff:fe8a:2e8a", "Enter a valid IPv4 address.")
	rejects(t, f, "2a02::223:6cff:fe8a:2e8a", "Enter a valid IPv4 address.")
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_as_ipv6_only
func TestGenericIPAddressAsIPv6Only(t *testing.T) {
	f := forms.NewGenericIPAddressField(forms.Protocol("IPv6"))
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, "127.0.0.1", "Enter a valid IPv6 address.")
	rejects(t, f, "foo", "Enter a valid IPv6 address.")
	rejects(t, f, "127.0.0.", "Enter a valid IPv6 address.")
	rejects(t, f, "1.2.3.4.5", "Enter a valid IPv6 address.")
	rejects(t, f, "256.125.1.5", "Enter a valid IPv6 address.")
	cleans(t, f, " fe80::223:6cff:fe8a:2e8a ", "fe80::223:6cff:fe8a:2e8a")
	cleans(t, f, " 2a02::223:6cff:fe8a:2e8a ", "2a02::223:6cff:fe8a:2e8a")
	rejects(t, f, "12345:2:3:4", "This is not a valid IPv6 address.")
	rejects(t, f, "1::2:3::4", "This is not a valid IPv6 address.")
	rejects(t, f, "foo::223:6cff:fe8a:2e8a", "This is not a valid IPv6 address.")
	rejects(t, f, "1::2:3:4:5:6:7:8", "This is not a valid IPv6 address.")
	rejects(t, f, "1:2", "This is not a valid IPv6 address.")
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_max_length_custom
func TestGenericIPAddressMaxLengthCustom(t *testing.T) {
	// Valid IPv4-mapped IPv6 address, length 45.
	addr := "0000:0000:0000:0000:0000:ffff:192.168.100.228"
	f := forms.NewGenericIPAddressField(forms.MaxLength(len(addr)))
	cleans(t, f, addr, "::ffff:192.168.100.228")
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_max_length_validation_error
func TestGenericIPAddressMaxLengthValidationError(t *testing.T) {
	addr := "0000:0000:0000:0000:0000:ffff:192.168.100.228"
	cases := []struct {
		opts      []forms.FieldOption
		maxLength int
	}{
		{nil, forms.MaxIPv6AddressLength},
		{[]forms.FieldOption{forms.MaxLength(len(addr) - 1)}, len(addr) - 1},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("max_length=%d", tc.maxLength), func(t *testing.T) {
			maxPlusOne := tc.maxLength + 1
			wantMsg := fmt.Sprintf("Ensure this value has at most %d characters (it has %d).", tc.maxLength, maxPlusOne)
			f := forms.NewGenericIPAddressField(tc.opts...)
			got := cleanErr(f, strings.Repeat("x", maxPlusOne))
			if !slices.Contains(got, wantMsg) {
				t.Errorf("Clean(long string) errors = %q, want %q", got, wantMsg)
			}
			rejects(t, f, addr, "This is not a valid IPv6 address.")
		})
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_as_generic_not_required
func TestGenericIPAddressAsGenericNotRequired(t *testing.T) {
	f := forms.NewGenericIPAddressField(forms.Required(false))
	cleans(t, f, "", "")
	cleans(t, f, nil, "")
	cleans(t, f, "127.0.0.1", "127.0.0.1")
	rejects(t, f, "foo", "Enter a valid IPv4 or IPv6 address.")
	rejects(t, f, "127.0.0.", "Enter a valid IPv4 or IPv6 address.")
	rejects(t, f, "1.2.3.4.5", "Enter a valid IPv4 or IPv6 address.")
	rejects(t, f, "256.125.1.5", "Enter a valid IPv4 or IPv6 address.")
	cleans(t, f, " fe80::223:6cff:fe8a:2e8a ", "fe80::223:6cff:fe8a:2e8a")
	cleans(t, f, strings.Repeat(" ", forms.MaxIPv6AddressLength)+" 2a02::223:6cff:fe8a:2e8a ", "2a02::223:6cff:fe8a:2e8a")
	rejects(t, f, "12345:2:3:4", "This is not a valid IPv6 address.")
	rejects(t, f, "1::2:3::4", "This is not a valid IPv6 address.")
	rejects(t, f, "foo::223:6cff:fe8a:2e8a", "This is not a valid IPv6 address.")
	rejects(t, f, "1::2:3:4:5:6:7:8", "This is not a valid IPv6 address.")
	rejects(t, f, "1:2", "This is not a valid IPv6 address.")
}

// Ported from Django: tests/forms_tests/field_tests/test_genericipaddressfield.py::GenericIPAddressFieldTest.test_generic_ipaddress_normalization
func TestGenericIPAddressNormalization(t *testing.T) {
	f := forms.NewGenericIPAddressField()
	cleans(t, f, " ::ffff:0a0a:0a0a  ", "::ffff:10.10.10.10")
	cleans(t, f, " ::ffff:10.10.10.10  ", "::ffff:10.10.10.10")
	cleans(t, f, " 2001:000:a:0000:0:fe:fe:beef  ", "2001:0:a::fe:fe:beef")
	cleans(t, f, " 2001::a:0000:0:fe:fe:beef  ", "2001:0:a::fe:fe:beef")

	f = forms.NewGenericIPAddressField(forms.UnpackIPv4())
	cleans(t, f, " ::ffff:0a0a:0a0a", "10.10.10.10")
}
