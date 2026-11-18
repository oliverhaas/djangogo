package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_booleanfield.py::BooleanFieldTest.test_booleanfield_clean_1
func TestBooleanfieldClean1(t *testing.T) {
	f := forms.NewBooleanField()
	rejects(t, f, "", "This field is required.")
	rejects(t, f, nil, "This field is required.")
	cleans(t, f, true, true)
	rejects(t, f, false, "This field is required.")
	cleans(t, f, 1, true)
	rejects(t, f, 0, "This field is required.")
	cleans(t, f, "Django rocks", true)
	cleans(t, f, "True", true)
	rejects(t, f, "False", "This field is required.")
}

// Ported from Django: tests/forms_tests/field_tests/test_booleanfield.py::BooleanFieldTest.test_booleanfield_clean_2
func TestBooleanfieldClean2(t *testing.T) {
	f := forms.NewBooleanField(forms.Required(false))
	cleans(t, f, "", false)
	cleans(t, f, nil, false)
	cleans(t, f, true, true)
	cleans(t, f, false, false)
	cleans(t, f, 1, true)
	cleans(t, f, 0, false)
	cleans(t, f, "1", true)
	cleans(t, f, "0", false)
	cleans(t, f, "Django rocks", true)
	cleans(t, f, "False", false)
	cleans(t, f, "false", false)
	cleans(t, f, "FaLsE", false)
}

// Ported from Django: tests/forms_tests/field_tests/test_booleanfield.py::BooleanFieldTest.test_booleanfield_changed
func TestBooleanfieldChanged(t *testing.T) {
	f := forms.NewBooleanField()
	equal(t, f.HasChanged(nil, nil), false)
	equal(t, f.HasChanged(nil, ""), false)
	equal(t, f.HasChanged("", nil), false)
	equal(t, f.HasChanged("", ""), false)
	equal(t, f.HasChanged(false, "on"), true)
	equal(t, f.HasChanged(true, "on"), false)
	equal(t, f.HasChanged(true, ""), true)
	// Initial value may have mutated to a string due to show_hidden_initial
	// (#19537)
	equal(t, f.HasChanged("False", "on"), true)
	// HiddenInput widget sends string values for boolean but doesn't clean
	// them in value_from_datadict.
	equal(t, f.HasChanged(false, "False"), false)
	equal(t, f.HasChanged(true, "True"), false)
	equal(t, f.HasChanged(false, "True"), true)
	equal(t, f.HasChanged(true, "False"), true)
}

// Ported from Django: tests/forms_tests/field_tests/test_booleanfield.py::BooleanFieldTest.test_disabled_has_changed
func TestBooleanfieldDisabledHasChanged(t *testing.T) {
	f := forms.NewBooleanField(forms.Disabled())
	equal(t, f.HasChanged("True", "False"), false)
}
