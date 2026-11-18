package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// plainField is a field with no behavior of its own beyond BaseField, the
// stand-in for Django's bare Field class.
type plainField struct{ forms.BaseField }

// newPlainField returns a plainField configured with opts.
func newPlainField(opts ...forms.FieldOption) *plainField {
	f := &plainField{}
	f.Init(f, forms.TextInput{}, nil, opts...)
	return f
}

// Ported from Django: tests/forms_tests/field_tests/test_base.py::DisabledFieldTests.test_disabled_field_has_changed_always_false
func TestDisabledFieldHasChangedAlwaysFalse(t *testing.T) {
	disabledField := newPlainField(forms.Disabled())
	equal(t, disabledField.HasChanged("x", "y"), false)
}
