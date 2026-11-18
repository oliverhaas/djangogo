package checks_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/checks"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file are ports of Django's tests/invalid_models_tests. A
// Django test declares a model class in the test and calls Model.check() or
// field.check(). A port declares a struct in the test, registers it under the
// app label invalid_models_tests in a registry of its own and runs the checks
// tagged models through the Default registry.
//
// Django adds an id primary key to a model without one; a Go model declares
// its ID field. A message about a field carries the text
// "<app>.<Model>.<attr>" as its object where Django carries the field.
//
// The model registry refuses some of the states Django reports with a check
// (a max_length that is not positive, on_delete=set_null without null, two
// fields with one column). A port of such a test registers a valid model and
// then sets the exported metadata of the field, which is what the checks read.

// invalidModelsApp is the app label of the models of this file.
const invalidModelsApp = "invalid_models_tests"

// model returns the registered descriptor of the model value v.
func model(t *testing.T, reg *orm.Registry, v any) *orm.Model {
	t.Helper()
	m, ok := reg.ModelOf(v)
	if !ok {
		t.Fatalf("model %T is not registered", v)
	}
	return m
}

// ValidChoices is the model of CharFieldTests.test_valid_field. It is declared
// here and has another name than Django's Model, because choices are declared
// with a method, which a type declared in a function cannot have.
type ValidChoices struct {
	ID    int64
	Field string `orm:"max_length=255;index"`
}

// Choices declares the choices of Field.
func (*ValidChoices) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{
		"Field": {{Value: "1", Label: "item1"}, {Value: "2", Label: "item2"}},
	}
}

// LongChoices is the model of CharFieldTests.test_choices_in_max_length: the
// value "ABC" does not fit max_length=2.
type LongChoices struct {
	ID    int64
	Field string `orm:"max_length=2"`
}

// Choices declares the choices of Field.
func (*LongChoices) Choices() map[string][]orm.Choice {
	return map[string][]orm.Choice{
		"Field": {{Value: "ABC", Label: "Value Too Long!"}, {Value: "OK", Label: "Good"}},
	}
}

// TestCharFieldValidField accepts a string field with max_length, choices and
// an index.
//
// Ported from Django: tests/invalid_models_tests/test_ordinary_fields.py::CharFieldTests.test_valid_field
func TestCharFieldValidField(t *testing.T) {
	reg := modelRegistry(t, invalidModelsApp, &ValidChoices{})

	assertMessages(t, modelMessages(reg))
}

// TestCharFieldMissingMaxLength reports a string field without max_length. A
// struct tag without max_length gets the default 255 from the registry, so the
// port clears the length in the metadata.
//
// Ported from Django: tests/invalid_models_tests/test_ordinary_fields.py::CharFieldTests.test_missing_max_length
func TestCharFieldMissingMaxLength(t *testing.T) {
	type Model struct {
		ID    int64
		Field string
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})
	field(t, reg, &Model{}, "Field").MaxLength = 0

	assertMessages(t, modelMessages(reg), checks.Error(
		"CharFields must define a 'max_length' attribute.",
		checks.Obj("invalid_models_tests.Model.field"),
		checks.ID("fields.E120"),
	))
}

// TestCharFieldNegativeMaxLength reports a negative max_length. The registry
// refuses the tag max_length=-1, so the port sets the length in the metadata.
//
// Ported from Django: tests/invalid_models_tests/test_ordinary_fields.py::CharFieldTests.test_negative_max_length
func TestCharFieldNegativeMaxLength(t *testing.T) {
	type Model struct {
		ID    int64
		Field string
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})
	field(t, reg, &Model{}, "Field").MaxLength = -1

	assertMessages(t, modelMessages(reg), checks.Error(
		"'max_length' must be a positive integer.",
		checks.Obj("invalid_models_tests.Model.field"),
		checks.ID("fields.E121"),
	))
}

// TestCharFieldChoicesInMaxLength reports a choice value longer than
// max_length, with the length of the longest value. Django's second field has
// grouped choices, which the ORM does not have; the port keeps the first.
//
// Ported from Django: tests/invalid_models_tests/test_ordinary_fields.py::CharFieldTests.test_choices_in_max_length
func TestCharFieldChoicesInMaxLength(t *testing.T) {
	reg := modelRegistry(t, invalidModelsApp, &LongChoices{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"'max_length' is too small to fit the longest value in 'choices' (3 characters).",
		checks.Obj("invalid_models_tests.LongChoices.field"),
		checks.ID("fields.E009"),
	))
}

// TestFieldNamesEndingWithUnderscore reports a field whose name ends with an
// underscore. Django's second field is a many-to-many field, which the ORM
// does not have; the port keeps the first.
//
// Ported from Django: tests/invalid_models_tests/test_models.py::FieldNamesTests.test_ending_with_underscore
func TestFieldNamesEndingWithUnderscore(t *testing.T) {
	type Model struct {
		ID     int64
		Field_ string `orm:"max_length=10"` //nolint:revive // the name is the case under test
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"Field names must not end with an underscore.",
		checks.Obj("invalid_models_tests.Model.field_"),
		checks.ID("fields.E001"),
	))
}

// TestFieldNamesIncludingSeparator reports a field whose name contains the
// lookup separator. The Go field Some_Field has the attribute name
// some__field.
//
// Ported from Django: tests/invalid_models_tests/test_models.py::FieldNamesTests.test_including_separator
func TestFieldNamesIncludingSeparator(t *testing.T) {
	type Model struct {
		ID         int64
		Some_Field int64 //nolint:revive // the name is the case under test
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})

	assertMessages(t, modelMessages(reg), checks.Error(
		`Field names must not contain "__".`,
		checks.Obj("invalid_models_tests.Model.some__field"),
		checks.ID("fields.E002"),
	))
}

// TestFieldNamesPK reports a field named pk.
//
// Ported from Django: tests/invalid_models_tests/test_models.py::FieldNamesTests.test_pk
func TestFieldNamesPK(t *testing.T) {
	type Model struct {
		ID int64
		Pk int64
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"'pk' is a reserved word that cannot be used as a field name.",
		checks.Obj("invalid_models_tests.Model.pk"),
		checks.ID("fields.E003"),
	))
}

// TestFieldNamesDBColumnClash reports a field whose column is the column of
// an earlier field. The registry refuses the tag column=foo on the second
// field, so the port sets the column in the metadata.
//
// Ported from Django: tests/invalid_models_tests/test_models.py::FieldNamesTests.test_db_column_clash
func TestFieldNamesDBColumnClash(t *testing.T) {
	type Model struct {
		ID  int64
		Foo int64
		Bar int64
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})
	field(t, reg, &Model{}, "Bar").Column = "foo"

	assertMessages(t, modelMessages(reg), checks.Error(
		"Field 'bar' has column name 'foo' that is used by another field.",
		checks.Hint("Specify a 'db_column' for the field."),
		checks.Obj(model(t, reg, &Model{})),
		checks.ID("models.E007"),
	))
}

// TestNameBeginningWithUnderscore reports a model whose name begins with an
// underscore.
//
// Ported from Django: tests/invalid_models_tests/test_models.py::OtherModelTests.test_name_beginning_with_underscore
func TestNameBeginningWithUnderscore(t *testing.T) {
	type _Model struct { //nolint:revive // the name is the case under test
		ID int64
	}
	reg := modelRegistry(t, invalidModelsApp, &_Model{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"The model name '_Model' cannot start or end with an underscore as it collides with the query lookup syntax.",
		checks.Obj(model(t, reg, &_Model{})),
		checks.ID("models.E023"),
	))
}

// TestNameEndingWithUnderscore reports a model whose name ends with an
// underscore.
//
// Ported from Django: tests/invalid_models_tests/test_models.py::OtherModelTests.test_name_ending_with_underscore
func TestNameEndingWithUnderscore(t *testing.T) {
	type Model_ struct { //nolint:revive // the name is the case under test
		ID int64
	}
	reg := modelRegistry(t, invalidModelsApp, &Model_{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"The model name 'Model_' cannot start or end with an underscore as it collides with the query lookup syntax.",
		checks.Obj(model(t, reg, &Model_{})),
		checks.ID("models.E023"),
	))
}

// TestNameContainsDoubleUnderscores reports a model whose name contains the
// lookup separator.
//
// Ported from Django: tests/invalid_models_tests/test_models.py::OtherModelTests.test_name_contains_double_underscores
func TestNameContainsDoubleUnderscores(t *testing.T) {
	type Test__Model struct { //nolint:revive // the name is the case under test
		ID int64
	}
	reg := modelRegistry(t, invalidModelsApp, &Test__Model{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"The model name 'Test__Model' cannot contain double underscores as it collides with the query lookup syntax.",
		checks.Obj(model(t, reg, &Test__Model{})),
		checks.ID("models.E024"),
	))
}

// TestValidForeignKeyWithoutAccessor accepts a foreign key to a registered
// model. Django's related_name="+" has no counterpart: a Go model has no
// reverse accessor that could clash with the field named model.
//
// Ported from Django: tests/invalid_models_tests/test_relative_fields.py::RelativeFieldTests.test_valid_foreign_key_without_accessor
func TestValidForeignKeyWithoutAccessor(t *testing.T) {
	type Target struct {
		ID    int64
		Model int64
	}
	type Model struct {
		ID    int64
		Field orm.FK[Target] `orm:"on_delete=cascade"`
	}
	reg := modelRegistry(t, invalidModelsApp, &Target{}, &Model{})

	assertMessages(t, modelMessages(reg))
}

// TestForeignKeyToMissingModel reports a foreign key to a model that is not
// registered. Django names the missing model with a string; the Go field names
// a type, which the port leaves out of the registry.
//
// Ported from Django: tests/invalid_models_tests/test_relative_fields.py::RelativeFieldTests.test_foreign_key_to_missing_model
func TestForeignKeyToMissingModel(t *testing.T) {
	type Rel1 struct {
		ID int64
	}
	type Model struct {
		ID         int64
		ForeignKey orm.FK[Rel1] `orm:"on_delete=cascade"`
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"Field defines a relation with model 'Rel1', which is either not installed, or is abstract.",
		checks.Obj("invalid_models_tests.Model.foreign_key"),
		checks.ID("fields.E300"),
	))
}

// TestOnDeleteSetNullOnNonNullableField reports on_delete=SET_NULL on a field
// that is not nullable. The registry refuses the tag on_delete=set_null
// without null, so the port sets the action in the metadata.
//
// Ported from Django: tests/invalid_models_tests/test_relative_fields.py::RelativeFieldTests.test_on_delete_set_null_on_non_nullable_field
func TestOnDeleteSetNullOnNonNullableField(t *testing.T) {
	type Person struct {
		ID int64
	}
	type Model struct {
		ID         int64
		ForeignKey orm.FK[Person]
	}
	reg := modelRegistry(t, invalidModelsApp, &Person{}, &Model{})
	field(t, reg, &Model{}, "ForeignKey").Rel.OnDelete = orm.OnDeleteSetNull

	assertMessages(t, modelMessages(reg), checks.Error(
		"Field specifies on_delete=SET_NULL, but cannot be null.",
		checks.Hint("Set null=True argument on the field, or change the on_delete rule."),
		checks.Obj("invalid_models_tests.Model.foreign_key"),
		checks.ID("fields.E320"),
	))
}

// TestNullablePrimaryKey reports a primary key that allows NULL.
//
// Ported from Django: tests/invalid_models_tests/test_relative_fields.py::RelativeFieldTests.test_nullable_primary_key
func TestNullablePrimaryKey(t *testing.T) {
	type Model struct {
		Field int64 `orm:"pk;null"`
	}
	reg := modelRegistry(t, invalidModelsApp, &Model{})

	assertMessages(t, modelMessages(reg), checks.Error(
		"Primary keys must not have null=True.",
		checks.Hint("Set null=False on the field, or remove primary_key=True argument."),
		checks.Obj("invalid_models_tests.Model.field"),
		checks.ID("fields.E007"),
	))
}
