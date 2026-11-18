package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
)

// A UUIDField cleans to the UUID in canonical text form in Go, where Django
// returns a uuid.UUID.

// Ported from Django: tests/forms_tests/field_tests/test_uuidfield.py::UUIDFieldTest.test_uuidfield_1
func TestUuidfield1(t *testing.T) {
	field := forms.NewUUIDField()
	cleans(t, field, "550e8400e29b41d4a716446655440000", "550e8400-e29b-41d4-a716-446655440000")
}

// Ported from Django: tests/forms_tests/field_tests/test_uuidfield.py::UUIDFieldTest.test_clean_value_with_dashes
func TestCleanValueWithDashes(t *testing.T) {
	field := forms.NewUUIDField()
	cleans(t, field, "550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440000")
}

// Ported from Django: tests/forms_tests/field_tests/test_uuidfield.py::UUIDFieldTest.test_uuidfield_2
func TestUuidfield2(t *testing.T) {
	field := forms.NewUUIDField(forms.Required(false))
	cleans(t, field, "", nil)
	cleans(t, field, nil, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_uuidfield.py::UUIDFieldTest.test_uuidfield_3
func TestUuidfield3(t *testing.T) {
	field := forms.NewUUIDField()
	rejects(t, field, "550e8400", "Enter a valid UUID.")
}

// Ported from Django: tests/forms_tests/field_tests/test_uuidfield.py::UUIDFieldTest.test_uuidfield_4
func TestUuidfield4(t *testing.T) {
	field := forms.NewUUIDField()
	value := field.PrepareValue("550e8400-e29b-41d4-a716-446655440000")
	equal(t, value, "550e8400-e29b-41d4-a716-446655440000")
}
