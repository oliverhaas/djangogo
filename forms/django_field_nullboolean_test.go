package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// Ported from Django: tests/forms_tests/field_tests/test_nullbooleanfield.py::NullBooleanFieldTest.test_nullbooleanfield_clean
func TestNullbooleanfieldClean(t *testing.T) {
	f := forms.NewNullBooleanField()
	cleans(t, f, "", nil)
	cleans(t, f, true, true)
	cleans(t, f, false, false)
	cleans(t, f, nil, nil)
	cleans(t, f, "0", false)
	cleans(t, f, "1", true)
	cleans(t, f, "2", nil)
	cleans(t, f, "3", nil)
	cleans(t, f, "hello", nil)
	cleans(t, f, "true", true)
	cleans(t, f, "false", false)
}

// hiddenNullBooleanForm is the HiddenNullBooleanForm of the Django tests.
func hiddenNullBooleanForm() *forms.Form {
	return forms.New(
		forms.F("hidden_nullbool1", forms.NewNullBooleanField(forms.UseWidget(forms.HiddenInput{}), forms.Initial(true))),
		forms.F("hidden_nullbool2", forms.NewNullBooleanField(forms.UseWidget(forms.HiddenInput{}), forms.Initial(false))),
	)
}

// Ported from Django: tests/forms_tests/field_tests/test_nullbooleanfield.py::NullBooleanFieldTest.test_nullbooleanfield_2
func TestNullbooleanfield2(t *testing.T) {
	// The internal value is preserved if using HiddenInput (#7753).
	f := hiddenNullBooleanForm()
	assertHTMLEqual(t, f.String(),
		`<input type="hidden" name="hidden_nullbool1" value="True" id="id_hidden_nullbool1">`+
			`<input type="hidden" name="hidden_nullbool2" value="False" id="id_hidden_nullbool2">`)
}

// Ported from Django: tests/forms_tests/field_tests/test_nullbooleanfield.py::NullBooleanFieldTest.test_nullbooleanfield_3
func TestNullbooleanfield3(t *testing.T) {
	f := hiddenNullBooleanForm().Bind(vals("hidden_nullbool1", "True", "hidden_nullbool2", "False"))
	f.Errors()
	cleaned := f.Cleaned()
	equal(t, cleaned["hidden_nullbool1"], true)
	equal(t, cleaned["hidden_nullbool2"], false)
}

// Ported from Django: tests/forms_tests/field_tests/test_nullbooleanfield.py::NullBooleanFieldTest.test_nullbooleanfield_4
func TestNullbooleanfield4(t *testing.T) {
	// Make sure we're compatible with MySQL, which uses 0 and 1 for its
	// boolean values (#9609).
	nullboolChoices := []forms.Choice{{Value: "1", Label: "Yes"}, {Value: "0", Label: "No"}, {Value: "", Label: "Unknown"}}
	radio := func() forms.FieldOption { return forms.UseWidget(forms.RadioSelect{Choices: nullboolChoices}) }
	f := forms.New(
		forms.F("nullbool0", forms.NewNullBooleanField(radio())),
		forms.F("nullbool1", forms.NewNullBooleanField(radio())),
		forms.F("nullbool2", forms.NewNullBooleanField(radio())),
	).Bind(vals("nullbool0", "1", "nullbool1", "0", "nullbool2", ""))
	f.Errors()
	cleaned := f.Cleaned()
	equal(t, cleaned["nullbool0"], true)
	equal(t, cleaned["nullbool1"], false)
	equal(t, cleaned["nullbool2"], nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_nullbooleanfield.py::NullBooleanFieldTest.test_nullbooleanfield_changed
func TestNullbooleanfieldChanged(t *testing.T) {
	f := forms.NewNullBooleanField()
	equal(t, f.HasChanged(false, nil), true)
	equal(t, f.HasChanged(nil, false), true)
	equal(t, f.HasChanged(nil, nil), false)
	equal(t, f.HasChanged(false, false), false)
	equal(t, f.HasChanged(true, false), true)
	equal(t, f.HasChanged(true, nil), true)
	equal(t, f.HasChanged(true, false), true)
	// HiddenInput widget sends string values for boolean but doesn't clean
	// them in value_from_datadict.
	equal(t, f.HasChanged(false, "False"), false)
	equal(t, f.HasChanged(true, "True"), false)
	equal(t, f.HasChanged(nil, ""), false)
	equal(t, f.HasChanged(false, "True"), true)
	equal(t, f.HasChanged(true, "False"), true)
	equal(t, f.HasChanged(nil, "False"), true)
}
