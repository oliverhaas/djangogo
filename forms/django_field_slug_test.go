package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_slugfield.py::SlugFieldTest.test_slugfield_normalization
func TestSlugfieldNormalization(t *testing.T) {
	f := forms.NewSlugField()
	cleans(t, f, "    aa-bb-cc    ", "aa-bb-cc")
}

// Ported from Django: tests/forms_tests/field_tests/test_slugfield.py::SlugFieldTest.test_slugfield_unicode_normalization
func TestSlugfieldUnicodeNormalization(t *testing.T) {
	f := forms.NewSlugField(forms.AllowUnicode())
	cleans(t, f, "a", "a")
	cleans(t, f, "1", "1")
	cleans(t, f, "a1", "a1")
	cleans(t, f, "你好", "你好")
	cleans(t, f, "  你-好  ", "你-好")
	cleans(t, f, "ıçğüş", "ıçğüş")
	cleans(t, f, "foo-ıç-bar", "foo-ıç-bar")
}

// Ported from Django: tests/forms_tests/field_tests/test_slugfield.py::SlugFieldTest.test_empty_value
func TestSlugfieldEmptyValue(t *testing.T) {
	f := forms.NewSlugField(forms.Required(false))
	cleans(t, f, "", "")
	cleans(t, f, nil, "")
	f = forms.NewSlugField(forms.Required(false), forms.EmptyValue(nil))
	cleans(t, f, "", nil)
	cleans(t, f, nil, nil)
}
