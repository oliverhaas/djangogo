package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_emailfield.py::EmailFieldTest.test_emailfield_1
func TestEmailfield1(t *testing.T) {
	f := forms.NewEmailField()
	equal(t, f.MaxLength, 320)
	rendersTo(t, f, `<input type="email" name="f" id="id_f" maxlength="320" required>`)
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, "person@example.com", "person@example.com")
	rejects(t, f, "foo", "Enter a valid email address.")
	cleans(t, f, "local@domain.with.idn.xyzäöüßabc.part.com", "local@domain.with.idn.xyz\xc3\xa4\xc3\xb6\xc3\xbc\xc3\x9fabc.part.com")
}

// Ported from Django: tests/forms_tests/field_tests/test_emailfield.py::EmailFieldTest.test_email_regexp_for_performance
func TestEmailRegexpForPerformance(t *testing.T) {
	f := forms.NewEmailField()
	// Check for runaway regex security problem. This will take a long time
	// if the security fix isn't in place.
	addr := "viewx3dtextx26qx3d@yahoo.comx26latlngx3d15854521645943074058"
	rejects(t, f, addr, "Enter a valid email address.")
}

// Ported from Django: tests/forms_tests/field_tests/test_emailfield.py::EmailFieldTest.test_emailfield_not_required
func TestEmailfieldNotRequired(t *testing.T) {
	f := forms.NewEmailField(forms.Required(false))
	cleans(t, f, "", "")
	cleans(t, f, nil, "")
	cleans(t, f, "person@example.com", "person@example.com")
	cleans(t, f, "      example@example.com  \t   \t ", "example@example.com")
	rejects(t, f, "foo", "Enter a valid email address.")
}

// Ported from Django: tests/forms_tests/field_tests/test_emailfield.py::EmailFieldTest.test_emailfield_min_max_length
func TestEmailfieldMinMaxLength(t *testing.T) {
	f := forms.NewEmailField(forms.MinLength(10), forms.MaxLength(15))
	rendersTo(t, f, `<input id="id_f" type="email" name="f" maxlength="15" minlength="10" required>`)
	rejects(t, f, "a@foo.com", "Ensure this value has at least 10 characters (it has 9).")
	cleans(t, f, "alf@foo.com", "alf@foo.com")
	rejects(t, f, "alf123456788@foo.com", "Ensure this value has at most 15 characters (it has 20).")
}

// Ported from Django: tests/forms_tests/field_tests/test_emailfield.py::EmailFieldTest.test_emailfield_strip_on_none_value
func TestEmailfieldStripOnNoneValue(t *testing.T) {
	f := forms.NewEmailField(forms.Required(false), forms.EmptyValue(nil))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
}
