package forms_test

import (
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_widget
func TestUrlfieldWidget(t *testing.T) {
	f := forms.NewURLField()
	rendersTo(t, f, `<input type="url" name="f" id="id_f" required>`)
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_widget_max_min_length
func TestUrlfieldWidgetMaxMinLength(t *testing.T) {
	f := forms.NewURLField(forms.MinLength(15), forms.MaxLength(20))
	cleans(t, f, "http://example.com", "http://example.com")
	rendersTo(t, f, `<input id="id_f" type="url" name="f" maxlength="20" minlength="15" required>`)
	rejects(t, f, "http://f.com", "Ensure this value has at least 15 characters (it has 12).")
	rejects(t, f, "http://abcdefghijklmnopqrstuvwxyz.com", "Ensure this value has at most 20 characters (it has 37).")
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_clean
func TestUrlfieldClean(t *testing.T) {
	f := forms.NewURLField(forms.Required(false))
	tests := [][2]string{
		{"http://localhost", "http://localhost"},
		{"http://example.com", "http://example.com"},
		{"http://example.com/test", "http://example.com/test"},
		{"http://example.com.", "http://example.com."},
		{"http://www.example.com", "http://www.example.com"},
		{"http://www.example.com:8000/test", "http://www.example.com:8000/test"},
		{"http://example.com?some_param=some_value", "http://example.com?some_param=some_value"},
		{"valid-with-hyphens.com", "https://valid-with-hyphens.com"},
		{"subdomain.domain.com", "https://subdomain.domain.com"},
		{"http://200.8.9.10", "http://200.8.9.10"},
		{"http://200.8.9.10:8000/test", "http://200.8.9.10:8000/test"},
		{"http://valid-----hyphens.com", "http://valid-----hyphens.com"},
		{"http://some.idn.xyzäöüßabc.domain.com:123/blah", "http://some.idn.xyz\xc3\xa4\xc3\xb6\xc3\xbc\xc3\x9fabc.domain.com:123/blah"},
		{
			"www.example.com/s/http://code.djangoproject.com/ticket/13804",
			"https://www.example.com/s/http://code.djangoproject.com/ticket/13804",
		},
		// Normalization.
		{"http://example.com/     ", "http://example.com/"},
		// Valid IDN.
		{"http://עברית.idn.icann.org/", "http://עברית.idn.icann.org/"},
		{"http://sãopaulo.com/", "http://sãopaulo.com/"},
		{"http://sãopaulo.com.br/", "http://sãopaulo.com.br/"},
		{"http://пример.испытание/", "http://пример.испытание/"},
		{"http://مثال.إختبار/", "http://مثال.إختبار/"},
		{"http://例子.测试/", "http://例子.测试/"},
		{"http://例子.測試/", "http://例子.測試/"},
		{"http://उदाहरण.परीक्षा/", "http://उदाहरण.परीक्षा/"},
		{"http://例え.テスト/", "http://例え.テスト/"},
		{"http://مثال.آزمایشی/", "http://مثال.آزمایشی/"},
		{"http://실례.테스트/", "http://실례.테스트/"},
		{"http://العربية.idn.icann.org/", "http://العربية.idn.icann.org/"},
		// IPv6.
		{"http://[12:34::3a53]/", "http://[12:34::3a53]/"},
		{"http://[a34:9238::]:8080/", "http://[a34:9238::]:8080/"},
		// IPv6 without scheme.
		{"[12:34::3a53]/", "https://[12:34::3a53]/"},
		// IDN domain without scheme but with port.
		{"ñandú.es:8080/", "https://ñandú.es:8080/"},
		// Scheme-relative.
		{"//example.com", "https://example.com"},
		{"//example.com/path", "https://example.com/path"},
		// Whitespace stripped.
		{"\t\n//example.com  \n\t\n", "https://example.com"},
		{"\t\nhttp://example.com  \n\t\n", "http://example.com"},
	}
	for _, tc := range tests {
		t.Run(tc[0], func(t *testing.T) {
			cleans(t, f, tc[0], tc[1])
		})
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_clean_invalid
func TestUrlfieldCleanInvalid(t *testing.T) {
	f := forms.NewURLField()
	tests := []any{
		"foo",
		"com.",
		".",
		"http://",
		"http://example",
		"http://example.",
		"http://.com",
		"http://invalid-.com",
		"http://-invalid.com",
		"http://inv-.alid-.com",
		"http://inv-.-alid.com",
		"[a",
		"http://[a",
		// Non-string.
		23,
		// Hangs "forever" before fixing a catastrophic backtracking,
		// see #11198.
		"http://" + strings.Repeat("X", 60),
		// A second example, to make sure the problem is really addressed,
		// even on domains that don't fail the domain label length check in
		// the regex.
		"http://" + strings.Repeat("X", 200),
		// Scheme prepend yields a structurally invalid URL.
		"////]@N.AN",
		// Scheme prepend yields an empty hostname.
		"#@A.bO",
		// Known problematic unicode chars.
		"http://" + strings.Repeat("¾", 200),
		// Non-ASCII character before the first colon.
		"¾:example.com",
		// ASCII digit before the first colon.
		"1http://example.com",
		// Empty scheme.
		"://example.com",
		":example.com",
	}
	for _, value := range tests {
		rejects(t, f, value, "Enter a valid URL.")
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_clean_required
func TestUrlfieldCleanRequired(t *testing.T) {
	f := forms.NewURLField()
	rejects(t, f, nil, "This field is required.")
	rejects(t, f, "", "This field is required.")
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_clean_not_required
func TestUrlfieldCleanNotRequired(t *testing.T) {
	f := forms.NewURLField(forms.Required(false))
	cleans(t, f, nil, "")
	cleans(t, f, "", "")
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_strip_on_none_value
func TestUrlfieldStripOnNoneValue(t *testing.T) {
	f := forms.NewURLField(forms.Required(false), forms.EmptyValue(nil))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_assume_scheme
func TestUrlfieldAssumeScheme(t *testing.T) {
	f := forms.NewURLField()
	cleans(t, f, "example.com", "https://example.com")
	f = forms.NewURLField(forms.AssumeScheme("http"))
	cleans(t, f, "example.com", "http://example.com")
	f = forms.NewURLField(forms.AssumeScheme("https"))
	cleans(t, f, "example.com", "https://example.com")
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_assume_scheme_when_colons
func TestUrlfieldAssumeSchemeWhenColons(t *testing.T) {
	f := forms.NewURLField()
	tests := [][2]string{
		// Port number.
		{"http://example.com:8080/", "http://example.com:8080/"},
		{"https://example.com:443/path", "https://example.com:443/path"},
		// Userinfo with password.
		{"http://user:pass@example.com", "http://user:pass@example.com"},
		{"http://user:pass@example.com:8080/", "http://user:pass@example.com:8080/"},
		// Colon in path segment.
		{"http://example.com/path:segment", "http://example.com/path:segment"},
		{"http://example.com/a:b/c:d", "http://example.com/a:b/c:d"},
		// Colon in query string.
		{"http://example.com/?key=val:ue", "http://example.com/?key=val:ue"},
		// Colon in fragment.
		{"http://example.com/#section:1", "http://example.com/#section:1"},
		// IPv6 -- multiple colons in host.
		{"http://[::1]/", "http://[::1]/"},
		{"http://[2001:db8::1]/", "http://[2001:db8::1]/"},
		{"http://[2001:db8::1]:8080/", "http://[2001:db8::1]:8080/"},
		// Colons across multiple components.
		{"http://user:pass@example.com:8080/path:x?q=a:b#id:1", "http://user:pass@example.com:8080/path:x?q=a:b#id:1"},
		// FTP with port and userinfo.
		{"ftp://user:pass@ftp.example.com:21/file", "ftp://user:pass@ftp.example.com:21/file"},
		{"ftps://user:pass@ftp.example.com:990/", "ftps://user:pass@ftp.example.com:990/"},
		// Scheme-relative URLs, starts with "//".
		{"//example.com:8080/path", "https://example.com:8080/path"},
		{"//user:pass@example.com/", "https://user:pass@example.com/"},
	}
	for _, tc := range tests {
		t.Run(tc[0], func(t *testing.T) {
			cleans(t, f, tc[0], tc[1])
		})
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_urlfield.py::URLFieldTest.test_urlfield_non_hierarchical_schemes_unchanged_in_to_python
func TestUrlfieldNonHierarchicalSchemesUnchangedInToPython(t *testing.T) {
	f := forms.NewURLField()
	tests := []string{
		"mailto:test@example.com",
		"mailto:test@example.com?subject=Hello",
		"tel:+1-800-555-0100",
		"tel:555-0100",
		"urn:isbn:0-486-27557-4",
		"urn:ietf:rfc:2648",
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			got, err := f.ToGo(value)
			if err != nil {
				t.Fatalf("ToGo(%q) failed: %v", value, err)
			}
			equal(t, got, value)
		})
	}
}
