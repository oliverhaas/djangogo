package orm_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// consProduct mirrors Django's constraints.Product.
type consProduct struct {
	ID              int64
	Price           *int    `orm:"null"`
	DiscountedPrice *int    `orm:"null"`
	Unit            *string `orm:"max_length=15;null"`
}

// Meta declares the check constraints of Django's Product.
func (consProduct) Meta() orm.Meta {
	return orm.Meta{
		VerboseName: "product",
		Constraints: []orm.Constraint{
			orm.CheckConstraint{
				Condition: orm.Q("price__gt", orm.F("discounted_price")),
				Name:      "price_gt_discounted_price",
			},
			orm.CheckConstraint{
				Condition: orm.Q("price__gt", 0),
				Name:      "%(app_label)s_%(class)s_price_gt_0",
			},
			orm.CheckConstraint{
				Condition: orm.Q("unit__isnull", true).Or(orm.Q("unit__in", []string{"μg/mL", "ng/mL"})),
				Name:      "unicode_unit_list",
			},
		},
	}
}

// consUniqueProduct mirrors Django's constraints.UniqueConstraintProduct.
type consUniqueProduct struct {
	ID      int64
	Name    string     `orm:"max_length=255"`
	Color   *string    `orm:"max_length=32;null"`
	Age     *int       `orm:"null"`
	Updated *time.Time `orm:"null"`
}

// Meta declares the unique constraint of Django's UniqueConstraintProduct.
func (consUniqueProduct) Meta() orm.Meta {
	return orm.Meta{
		VerboseName: "unique constraint product",
		Constraints: []orm.Constraint{
			orm.UniqueConstraint{Fields: []string{"name", "color"}, Name: "name_color_uniq"},
		},
	}
}

// consConditionProduct mirrors Django's
// constraints.UniqueConstraintConditionProduct.
type consConditionProduct struct {
	ID    int64
	Name  string  `orm:"max_length=255"`
	Color *string `orm:"max_length=32;null"`
}

// Meta declares the partial unique constraint of Django's
// UniqueConstraintConditionProduct.
func (consConditionProduct) Meta() orm.Meta {
	return orm.Meta{
		Constraints: []orm.Constraint{
			orm.UniqueConstraint{
				Fields:    []string{"name"},
				Name:      "name_without_color_uniq",
				Condition: orm.Q("color__isnull", true),
			},
		},
	}
}

// consChildModel mirrors Django's constraints.ChildModel, which inherits the
// check constraint of an abstract model.
type consChildModel struct {
	ID  int64
	Age int
}

// Meta declares the check constraint of Django's AbstractModel.
func (consChildModel) Meta() orm.Meta {
	return orm.Meta{
		Constraints: []orm.Constraint{
			orm.CheckConstraint{Condition: orm.Q("age__gte", 18), Name: "%(app_label)s_%(class)s_adult"},
		},
	}
}

// consJSONFieldModel mirrors Django's constraints.JSONFieldModel.
type consJSONFieldModel struct {
	ID   int64
	Data json.RawMessage `orm:"null"`
}

// consParentLink stands in for Django's ChildUniqueConstraintProduct, whose
// parent link uniqueconstraintproduct_ptr is a foreign key.
type consParentLink struct {
	ID     int64
	Parent orm.FK[consUniqueProduct] `orm:"null"`
}

// consModels returns the models of Django's constraints app.
func consModels() []any {
	return []any{djangotest.App("constraints",
		&consProduct{}, &consUniqueProduct{}, &consConditionProduct{},
		&consChildModel{}, &consJSONFieldModel{}, &consParentLink{}, &consSingleUnique{},
	)}
}

// consConstraint returns the constraint of model T named name.
func consConstraint[T any](t *testing.T, db *orm.DB, name string) orm.Constraint {
	t.Helper()
	var zero T
	m, ok := db.Registry().ModelOf(&zero)
	if !ok {
		t.Fatalf("%T is not registered", zero)
	}
	for _, c := range m.Constraints() {
		if c.ConstraintName() == name {
			return c
		}
	}
	t.Fatalf("%s has no constraint %q", m.Name(), name)
	return nil
}

// assertViolation checks that err is a validation error whose message
// contains msg, and returns it.
func assertViolation(t *testing.T, err error, msg string) *orm.ValidationError {
	t.Helper()
	var ve *orm.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want a validation error %q", err, msg)
	}
	if !strings.Contains(ve.Error(), msg) {
		t.Errorf("error = %q, want it to contain %q", ve.Error(), msg)
	}
	return ve
}

// assertValid checks that a constraint check found no violation.
func assertValid(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("error = %v, want none", err)
	}
}

// consPtr returns a pointer to v.
func consPtr[T any](v T) *T { return &v }

// No Go counterpart: BaseConstraintTests.test_constraint_sql, test_create_sql,
// test_remove_sql, test_validate, test_contains_expressions and
// test_deconstruction test the methods of the abstract BaseConstraint, which
// orm.Constraint seals. CheckConstraintTests.test_invalid_check_types and
// UniqueConstraintTests.test_condition_must_be_q are type errors in Go.
// test_eq and test_repr of both classes have no counterpart: constraints are
// plain struct values. The test_deconstruction* tests are ported in
// django_constraints_source_test.go.

// Ported from Django: tests/constraints/tests.py::BaseConstraintTests.test_default_violation_error_message
func TestConstraintsDefaultViolationErrorMessage(t *testing.T) {
	c := orm.CheckConstraint{Name: "name"}
	djangotest.AssertEqual(t, c.ViolationMessage(), "Constraint “name” is violated.")
}

// Ported from Django: tests/constraints/tests.py::BaseConstraintTests.test_custom_violation_error_message
// Ported from Django: tests/constraints/tests.py::BaseConstraintTests.test_custom_violation_error_message_clone
func TestConstraintsCustomViolationErrorMessage(t *testing.T) {
	c := orm.UniqueConstraint{Name: "base_name", ViolationErrorMessage: "custom %(name)s message"}
	djangotest.AssertEqual(t, c.ViolationMessage(), "custom base_name message")
	// A copy of the value is Django's clone().
	clone := c
	djangotest.AssertEqual(t, clone.ViolationMessage(), "custom base_name message")
}

// Ported from Django: tests/constraints/tests.py::BaseConstraintTests.test_custom_violation_code_message
func TestConstraintsCustomViolationCodeMessage(t *testing.T) {
	c := orm.CheckConstraint{Name: "base_name", ViolationErrorCode: "custom_code"}
	djangotest.AssertEqual(t, c.ViolationErrorCode, "custom_code")
}

// Ported from Django: tests/constraints/tests.py::BaseConstraintTests.test_name_required
//
// Go checks the name when the model registers.
func TestConstraintsNameRequired(t *testing.T) {
	_, err := orm.NewRegistry().Register(&consNamelessCheck{})
	assertErrorContains(t, err, "a check constraint must be named")
}

// consNamelessCheck declares a check constraint without a name.
type consNamelessCheck struct {
	ID  int64
	Age int
}

// Meta declares the nameless constraint.
func (consNamelessCheck) Meta() orm.Meta {
	return orm.Meta{Constraints: []orm.Constraint{orm.CheckConstraint{Condition: orm.Q("age__gt", 0)}}}
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_database_constraint
func TestConstraintsCheckDatabaseConstraint(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		products := orm.Query[consProduct](db)
		djangotest.AssertEqual(t, products.Create(ctx, &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(5)}), nil)
		err := products.Create(ctx, &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(20)})
		if !errors.Is(err, orm.ErrIntegrity) || !errors.Is(err, orm.ErrCheckViolation) {
			t.Errorf("error = %v, want a check violation", err)
		}
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_database_constraint_unicode
func TestConstraintsCheckDatabaseConstraintUnicode(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		products := orm.Query[consProduct](db)
		djangotest.AssertEqual(t, products.Create(ctx, &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(5), Unit: consPtr("μg/mL")}), nil)
		err := products.Create(ctx, &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(7), Unit: consPtr("l")})
		if !errors.Is(err, orm.ErrIntegrity) {
			t.Errorf("error = %v, want an integrity error", err)
		}
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_name
// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_abstract_name
//
// The Go models are named consProduct and consChildModel, which "%(class)s"
// lowercases.
func TestConstraintsCheckName(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		for _, name := range []string{"price_gt_discounted_price", "constraints_consproduct_price_gt_0"} {
			djangotest.AssertConstraintExists(t, db, "constraints_consproduct", name)
		}
		djangotest.AssertConstraintExists(t, db, "constraints_conschildmodel", "constraints_conschildmodel_adult")
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate
func TestConstraintsCheckValidate(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		constraint := orm.CheckConstraint{Condition: orm.Q("price__gt", orm.F("discounted_price")), Name: "price"}
		// Invalid product.
		invalid := &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(42)}
		assertViolation(t, constraint.Validate(ctx, db, invalid), "Constraint “price” is violated.")
		assertViolation(t, constraint.Validate(ctx, db, invalid, "unit"), "Constraint “price” is violated.")
		// Fields used by the check constraint are excluded.
		assertValid(t, constraint.Validate(ctx, db, invalid, "price"))
		assertValid(t, constraint.Validate(ctx, db, invalid, "discounted_price"))
		assertValid(t, constraint.Validate(ctx, db, invalid, "discounted_price", "price"))
		// Valid product.
		assertValid(t, constraint.Validate(ctx, db, &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(5)}))
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_custom_error
func TestConstraintsCheckValidateCustomError(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		constraint := orm.CheckConstraint{
			Condition:             orm.Q("price__gt", orm.F("discounted_price")),
			Name:                  "price",
			ViolationErrorMessage: "discount is fake",
			ViolationErrorCode:    "fake_discount",
		}
		invalid := &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(42)}
		ve := assertViolation(t, constraint.Validate(context.Background(), db, invalid), "discount is fake")
		djangotest.AssertEqual(t, ve.Code, "fake_discount")
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_boolean_expressions
//
// Django wraps the condition in an ExpressionWrapper; a Go condition is a
// Cond already.
func TestConstraintsCheckValidateBooleanExpressions(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		constraint := orm.CheckConstraint{
			Condition: orm.Q("price__gt", 500).Or(orm.Q("price__lt", 500)),
			Name:      "price_neq_500_wrap",
		}
		assertViolation(t, constraint.Validate(ctx, db, &consProduct{Price: consPtr(500), DiscountedPrice: consPtr(5)}),
			"Constraint “price_neq_500_wrap” is violated.")
		assertValid(t, constraint.Validate(ctx, db, &consProduct{Price: consPtr(501), DiscountedPrice: consPtr(5)}))
		assertValid(t, constraint.Validate(ctx, db, &consProduct{Price: consPtr(499), DiscountedPrice: consPtr(5)}))
	})
}

// No Go counterpart: CheckConstraintTests.test_validate_rawsql_expressions_noop
// needs RawSQL in a condition, which orm.Cond does not take.

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_nullable_field_with_none
// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_nullable_field_with_isnull
//
// Django runs the second test only on Oracle; both conditions hold here.
func TestConstraintsCheckValidateNullableField(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		// Nullable fields should be considered valid on None values.
		constraint := orm.CheckConstraint{Condition: orm.Q("price__gte", 0), Name: "positive_price"}
		assertValid(t, constraint.Validate(ctx, db, &consProduct{}))
		constraint = orm.CheckConstraint{
			Condition: orm.Q("price__gte", 0).Or(orm.Q("price__isnull", true)),
			Name:      "positive_price",
		}
		assertValid(t, constraint.Validate(ctx, db, &consProduct{}))
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_nullable_jsonfield
func TestConstraintsCheckValidateNullableJSONField(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		isNull := orm.CheckConstraint{Condition: orm.Q("data__isnull", true), Name: "nullable_data"}
		isNotNull := orm.CheckConstraint{Condition: orm.Q("data__isnull", false), Name: "nullable_data"}
		assertValid(t, isNull.Validate(ctx, db, &consJSONFieldModel{Data: nil}))
		assertViolation(t, isNull.Validate(ctx, db, &consJSONFieldModel{Data: json.RawMessage(`{}`)}),
			"Constraint “nullable_data” is violated.")
		assertViolation(t, isNotNull.Validate(ctx, db, &consJSONFieldModel{Data: nil}),
			"Constraint “nullable_data” is violated.")
		assertValid(t, isNotNull.Validate(ctx, db, &consJSONFieldModel{Data: json.RawMessage(`{}`)}))
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_pk_field
func TestConstraintsCheckValidatePKField(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		constraint := orm.CheckConstraint{Condition: orm.Not(orm.Q("pk", orm.F("age"))), Name: "pk_not_age_check"}
		assertValid(t, constraint.Validate(ctx, db, &consChildModel{ID: 1, Age: 2}))
		assertViolation(t, constraint.Validate(ctx, db, &consChildModel{ID: 1, Age: 1}),
			"Constraint “pk_not_age_check” is violated.")
		idConstraint := orm.CheckConstraint{Condition: orm.Not(orm.Q("id", orm.F("age"))), Name: "pk_not_age_check"}
		assertViolation(t, idConstraint.Validate(ctx, db, &consChildModel{ID: 1, Age: 1}),
			"Constraint “pk_not_age_check” is violated.")
		assertValid(t, constraint.Validate(ctx, db, &consChildModel{ID: 1, Age: 1}, "pk"))
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_fk_attname
//
// Django uses the parent link of a multi-table child; a plain foreign key
// stands in for it.
func TestConstraintsCheckValidateFKAttname(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		constraint := orm.CheckConstraint{Condition: orm.Q("parent_id__isnull", false), Name: "parent_ptr_present"}
		assertViolation(t, constraint.Validate(ctx, db, &consParentLink{}), "Constraint “parent_ptr_present” is violated.")
		assertValid(t, constraint.Validate(ctx, db, &consParentLink{Parent: fkTo[consUniqueProduct](1)}))
		// The name of the field excludes its references by column.
		assertValid(t, constraint.Validate(ctx, db, &consParentLink{}, "parent"))
	})
}

// Ported from Django: tests/constraints/tests.py::CheckConstraintTests.test_validate_jsonfield_exact
func TestConstraintsCheckValidateJSONFieldExact(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		constraint := orm.CheckConstraint{Condition: orm.Q("data__version", "stable"), Name: "only_stable_version"}
		assertValid(t, constraint.Validate(ctx, db, &consJSONFieldModel{Data: json.RawMessage(`{"release": "5.0.2", "version": "stable"}`)}))
		assertViolation(t, constraint.Validate(ctx, db, &consJSONFieldModel{Data: json.RawMessage(`{"release": "5.0.2", "version": "not stable"}`)}),
			"Constraint “only_stable_version” is violated.")
	})
}

// No Go counterpart: CheckConstraintTests.test_validate_generated_field_stored,
// test_validate_generated_field_virtual and test_database_default need
// GeneratedField and db_default, which the orm does not have.

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_database_constraint
func TestConstraintsUniqueDatabaseConstraint(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		p1, _ := consUniqueSetup(t, db)
		err := orm.Query[consUniqueProduct](db).Create(context.Background(), &consUniqueProduct{Name: p1.Name, Color: p1.Color})
		if !errors.Is(err, orm.ErrIntegrity) || !errors.Is(err, orm.ErrUniqueViolation) {
			t.Errorf("error = %v, want a unique violation", err)
		}
	})
}

// consUniqueSetup creates the rows of UniqueConstraintTests.setUpTestData.
func consUniqueSetup(t *testing.T, db *orm.DB) (p1, p2 *consUniqueProduct) {
	t.Helper()
	p1 = create(t, db, &consUniqueProduct{Name: "p1", Color: consPtr("red")})
	p2 = create(t, db, &consUniqueProduct{Name: "p2"})
	return p1, p2
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_database_constraint_with_condition
func TestConstraintsUniqueDatabaseConstraintWithCondition(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		products := orm.Query[consConditionProduct](db)
		djangotest.AssertEqual(t, products.Create(ctx, &consConditionProduct{Name: "p1"}), nil)
		djangotest.AssertEqual(t, products.Create(ctx, &consConditionProduct{Name: "p2"}), nil)
		err := products.Create(ctx, &consConditionProduct{Name: "p1"})
		if !errors.Is(err, orm.ErrUniqueViolation) {
			t.Errorf("error = %v, want a unique violation", err)
		}
		// A color takes the row out of the partial index.
		djangotest.AssertEqual(t, products.Create(ctx, &consConditionProduct{Name: "p1", Color: consPtr("red")}), nil)
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_model_validation
func TestConstraintsUniqueModelValidation(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		p1, _ := consUniqueSetup(t, db)
		err := orm.ValidateConstraints(context.Background(), db, &consUniqueProduct{Name: p1.Name, Color: p1.Color})
		assertViolation(t, err, "Unique constraint product with this Name and Color already exists.")
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_model_validation_with_condition
func TestConstraintsUniqueModelValidationWithCondition(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		obj1 := create(t, db, &consConditionProduct{Name: "p1", Color: consPtr("red")})
		obj2 := create(t, db, &consConditionProduct{Name: "p2"})
		assertValid(t, orm.ValidateConstraints(ctx, db, &consConditionProduct{Name: obj1.Name, Color: consPtr("blue")}))
		err := orm.ValidateConstraints(ctx, db, &consConditionProduct{Name: obj2.Name})
		assertViolation(t, err, "Constraint “name_without_color_uniq” is violated.")
	})
}

// No Go counterpart: UniqueConstraintTests.test_model_validation_constraint_no_code_error
// overrides validate() in a subclass; orm.Constraint is sealed.

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate
//
// The last part of Django's test validates a multi-table child, which Go
// models do not have.
func TestConstraintsUniqueValidate(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p1, p2 := consUniqueSetup(t, db)
		constraint := consConstraint[consUniqueProduct](t, db, "name_color_uniq")
		msg := "Unique constraint product with this Name and Color already exists."
		nonUnique := &consUniqueProduct{Name: p1.Name, Color: p1.Color}
		ve := assertViolation(t, constraint.Validate(ctx, db, nonUnique), msg)
		djangotest.AssertEqual(t, ve.Code, "unique_together")
		// Null values are ignored.
		assertValid(t, constraint.Validate(ctx, db, &consUniqueProduct{Name: p2.Name}))
		// Existing instances have their existing row excluded.
		assertValid(t, constraint.Validate(ctx, db, p1))
		// Unique fields are excluded.
		assertValid(t, constraint.Validate(ctx, db, nonUnique, "name"))
		assertValid(t, constraint.Validate(ctx, db, nonUnique, "color"))
		assertValid(t, constraint.Validate(ctx, db, nonUnique, "name", "color"))
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_unique_custom_code_and_message
func TestConstraintsUniqueValidateCustomCodeAndMessage(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		consUniqueSetup(t, db)
		product := create(t, db, &consUniqueProduct{Name: "test", Color: consPtr("red"), Age: consPtr(42)})
		multiple := orm.UniqueConstraint{
			Fields: []string{"color", "age"}, Name: "color_age_uniq",
			ViolationErrorCode: "custom_code", ViolationErrorMessage: "Custom message",
		}
		single := orm.UniqueConstraint{
			Fields: []string{"color"}, Name: "color_uniq",
			ViolationErrorCode: "custom_code", ViolationErrorMessage: "Custom message",
		}
		ve := assertViolation(t, multiple.Validate(ctx, db, &consUniqueProduct{Name: "new-test", Color: product.Color, Age: product.Age}), "Custom message")
		djangotest.AssertEqual(t, ve.Code, "custom_code")
		ve = assertViolation(t, single.Validate(ctx, db, &consUniqueProduct{Name: "new-test", Color: product.Color}), "Custom message")
		djangotest.AssertEqual(t, ve.Code, "custom_code")
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_fields_unattached
func TestConstraintsUniqueValidateFieldsUnattached(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &consProduct{Price: consPtr(42)})
		constraint := orm.UniqueConstraint{Fields: []string{"price"}, Name: "uniq_prices"}
		assertViolation(t, constraint.Validate(context.Background(), db, &consProduct{Price: consPtr(42)}),
			"Product with this Price already exists.")
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_condition
func TestConstraintsUniqueValidateCondition(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p1 := create(t, db, &consConditionProduct{Name: "p1"})
		constraint := consConstraint[consConditionProduct](t, db, "name_without_color_uniq")
		assertViolation(t, constraint.Validate(ctx, db, &consConditionProduct{Name: p1.Name}),
			"Constraint “name_without_color_uniq” is violated.")
		// Values not matching condition are ignored.
		assertValid(t, constraint.Validate(ctx, db, &consConditionProduct{Name: p1.Name, Color: consPtr("anything-but-none")}))
		// Existing instances have their existing row excluded.
		assertValid(t, constraint.Validate(ctx, db, p1))
		// Unique field is excluded.
		assertValid(t, constraint.Validate(ctx, db, &consConditionProduct{Name: p1.Name}, "name"))
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_condition_custom_error
func TestConstraintsUniqueValidateConditionCustomError(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		p1 := create(t, db, &consConditionProduct{Name: "p1"})
		constraint := orm.UniqueConstraint{
			Fields:                []string{"name"},
			Name:                  "name_without_color_uniq",
			Condition:             orm.Q("color__isnull", true),
			ViolationErrorCode:    "custom_code",
			ViolationErrorMessage: "Custom message",
		}
		ve := assertViolation(t, constraint.Validate(context.Background(), db, &consConditionProduct{Name: p1.Name}), "Custom message")
		djangotest.AssertEqual(t, ve.Code, "custom_code")
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_expression
func TestConstraintsUniqueValidateExpression(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p1, _ := consUniqueSetup(t, db)
		constraint := orm.UniqueConstraint{Expressions: []orm.Expr{orm.Lower("name")}, Name: "name_lower_uniq"}
		upper := &consUniqueProduct{Name: strings.ToUpper(p1.Name)}
		assertViolation(t, constraint.Validate(ctx, db, upper), "Constraint “name_lower_uniq” is violated.")
		assertValid(t, constraint.Validate(ctx, db, &consUniqueProduct{Name: "another-name"}))
		// Existing instances have their existing row excluded.
		assertValid(t, constraint.Validate(ctx, db, p1))
		// Unique field is excluded.
		assertValid(t, constraint.Validate(ctx, db, upper, "name"))
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_field_transform
func TestConstraintsUniqueValidateFieldTransform(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		updated := time.Date(2005, 7, 26, 0, 0, 0, 0, time.UTC)
		create(t, db, &consUniqueProduct{Name: "p1", Updated: &updated})
		constraint := orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("updated__date")}, Name: "date_created_unique"}
		assertViolation(t, constraint.Validate(ctx, db, &consUniqueProduct{Updated: &updated}),
			"Constraint “date_created_unique” is violated.")
		nextDay := updated.AddDate(0, 0, 1)
		assertValid(t, constraint.Validate(ctx, db, &consUniqueProduct{Updated: &nextDay}))
	})
}

// No Go counterpart: UniqueConstraintTests.test_validate_case_when needs Case
// and When, test_validate_ordered_expression needs an ordered expression, and
// the generated field tests need GeneratedField, none of which the orm has.

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_expression_condition
func TestConstraintsUniqueValidateExpressionCondition(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p1, p2 := consUniqueSetup(t, db)
		constraint := orm.UniqueConstraint{
			Expressions: []orm.Expr{orm.Lower("name")},
			Name:        "name_lower_without_color_uniq",
			Condition:   orm.Q("color__isnull", true),
		}
		nonUnique := &consUniqueProduct{Name: strings.ToUpper(p2.Name)}
		assertViolation(t, constraint.Validate(ctx, db, nonUnique), "Constraint “name_lower_without_color_uniq” is violated.")
		// Values not matching condition are ignored.
		assertValid(t, constraint.Validate(ctx, db, &consUniqueProduct{Name: p1.Name, Color: p1.Color}))
		// Existing instances have their existing row excluded.
		assertValid(t, constraint.Validate(ctx, db, p2))
		// Unique field is excluded.
		assertValid(t, constraint.Validate(ctx, db, nonUnique, "name"))
		// Field from a condition is excluded.
		assertValid(t, constraint.Validate(ctx, db, nonUnique, "color"))
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_expression_str
//
// Django turns the string "name" into F("name").
func TestConstraintsUniqueValidateExpressionStr(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p1, _ := consUniqueSetup(t, db)
		constraint := orm.UniqueConstraint{Expressions: []orm.Expr{orm.F("name")}, Name: "name_uniq"}
		assertViolation(t, constraint.Validate(ctx, db, &consUniqueProduct{Name: p1.Name}), "Constraint “name_uniq” is violated.")
		assertValid(t, constraint.Validate(ctx, db, &consUniqueProduct{Name: p1.Name}, "name"))
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_nullable_textfield_with_isnull_true
func TestConstraintsUniqueValidateNullableTextFieldWithIsNullTrue(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		prices := []orm.Expr{orm.F("price"), orm.F("discounted_price")}
		isNull := orm.UniqueConstraint{Expressions: prices, Condition: orm.Q("unit__isnull", true), Name: "uniq_prices_no_unit"}
		isNotNull := orm.UniqueConstraint{Expressions: prices, Condition: orm.Q("unit__isnull", false), Name: "uniq_prices_unit"}

		create(t, db, &consProduct{Price: consPtr(2), DiscountedPrice: consPtr(1)})
		create(t, db, &consProduct{Price: consPtr(4), DiscountedPrice: consPtr(3), Unit: consPtr("ng/mL")})

		assertViolation(t, isNull.Validate(ctx, db, &consProduct{Price: consPtr(2), DiscountedPrice: consPtr(1)}),
			"Constraint “uniq_prices_no_unit” is violated.")
		assertValid(t, isNull.Validate(ctx, db, &consProduct{Price: consPtr(2), DiscountedPrice: consPtr(1), Unit: consPtr("ng/mL")}))
		assertValid(t, isNull.Validate(ctx, db, &consProduct{Price: consPtr(4), DiscountedPrice: consPtr(3)}))

		assertViolation(t, isNotNull.Validate(ctx, db, &consProduct{Price: consPtr(4), DiscountedPrice: consPtr(3), Unit: consPtr("μg/mL")}),
			"Constraint “uniq_prices_unit” is violated.")
		assertValid(t, isNotNull.Validate(ctx, db, &consProduct{Price: consPtr(4), DiscountedPrice: consPtr(3)}))
		assertValid(t, isNotNull.Validate(ctx, db, &consProduct{Price: consPtr(2), DiscountedPrice: consPtr(1)}))
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_nulls_distinct_fields
func TestConstraintsUniqueValidateNullsDistinctFields(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &consProduct{Price: consPtr(42)})
		constraint := orm.UniqueConstraint{Fields: []string{"price"}, NullsNotDistinct: true, Name: "uniq_prices_nulls_distinct"}
		assertValid(t, constraint.Validate(ctx, db, &consProduct{}))
		create(t, db, &consProduct{})
		assertViolation(t, constraint.Validate(ctx, db, &consProduct{}), "Product with this Price already exists.")
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_validate_nulls_distinct_expressions
//
// The orm has no Abs; Lower of the nullable unit stands in for it.
func TestConstraintsUniqueValidateNullsDistinctExpressions(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &consProduct{Unit: consPtr("ng/mL")})
		constraint := orm.UniqueConstraint{
			Expressions:      []orm.Expr{orm.Lower("unit")},
			NullsNotDistinct: true,
			Name:             "uniq_units_nulls_distinct",
		}
		assertValid(t, constraint.Validate(ctx, db, &consProduct{}))
		create(t, db, &consProduct{})
		assertViolation(t, constraint.Validate(ctx, db, &consProduct{}), "Constraint “uniq_units_nulls_distinct” is violated.")
	})
}

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_name
func TestConstraintsUniqueName(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		djangotest.AssertConstraintExists(t, db, "constraints_consuniqueproduct", "name_color_uniq")
		djangotest.AssertConstraintExists(t, db, "constraints_consconditionproduct", "name_without_color_uniq")
	})
}

// FIXME-PORT: UniqueConstraintTests.test_initially_deferred_database_constraint,
// test_initially_immediate_database_constraint, test_deferrable_with_*,
// test_invalid_defer_argument, test_include_database_constraint,
// test_invalid_include_argument, test_invalid_opclasses_argument,
// test_opclasses_and_fields_same_length and test_expressions_with_opclasses
// need the deferrable, include and opclasses options, which UniqueConstraint
// does not have. test_invalid_nulls_distinct_argument is a type error in Go,
// and test_database_default needs db_default.

// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_requires_field_or_expression
// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_expressions_and_fields_mutually_exclusive
// Ported from Django: tests/constraints/tests.py::UniqueConstraintTests.test_requires_name
//
// Go checks the options when the model registers.
func TestConstraintsUniqueInvalidOptions(t *testing.T) {
	cases := []struct {
		constraint orm.UniqueConstraint
		want       string
	}{
		{orm.UniqueConstraint{Name: "name"}, "At least one field or expression is required to define a unique constraint."},
		{
			orm.UniqueConstraint{Fields: []string{"age"}, Expressions: []orm.Expr{orm.F("age")}, Name: "name"},
			"UniqueConstraint.fields and expressions are mutually exclusive.",
		},
		{orm.UniqueConstraint{Fields: []string{"age"}}, "A unique constraint must be named."},
		{orm.UniqueConstraint{Fields: []string{"missing"}, Name: "name"}, `unique constraint "name" names unknown field "missing"`},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			_, err := orm.NewRegistry().Register(&consInvalid{constraint: tc.constraint})
			assertErrorContains(t, err, tc.want)
		})
	}
}

// consInvalid declares the constraint that a test sets.
type consInvalid struct {
	ID         int64
	Age        int
	constraint orm.Constraint
}

// Meta declares the constraint of the test.
func (c consInvalid) Meta() orm.Meta {
	return orm.Meta{Constraints: []orm.Constraint{c.constraint}}
}

// consDuplicateNames declares two constraints of the same name.
type consDuplicateNames struct {
	ID int64
}

// Meta declares the two constraints.
func (consDuplicateNames) Meta() orm.Meta {
	return orm.Meta{Constraints: []orm.Constraint{
		orm.CheckConstraint{Condition: orm.Q("id__gt", 0), Name: "foo"},
		orm.CheckConstraint{Condition: orm.Q("id__lt", 100), Name: "foo"},
	}}
}

// Ported from Django: tests/check_framework/test_model_checks.py::ConstraintNameTests.test_collision_in_same_model
//
// Go reports the collision when the model registers.
//
// FIXME-PORT: ConstraintNameTests.test_collision_in_different_models and the
// related tests need a check across the models of a registry.
func TestConstraintsNameCollisionInSameModel(t *testing.T) {
	_, err := orm.NewRegistry().RegisterApp("check_framework", &consDuplicateNames{})
	assertErrorContains(t, err, "constraint name 'foo' is not unique for model check_framework.consDuplicateNames")
}

// Constraints referring to joined fields cannot be validated or created. No
// Django test exists: Django reports them with the system check models.E041.
func TestConstraintsJoinedField(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		constraint := orm.CheckConstraint{Condition: orm.Q("parent__name", "x"), Name: "joined"}
		err := constraint.Validate(context.Background(), db, &consParentLink{})
		assertErrorContains(t, err, `constraint "joined" refers to the joined field "parent__name"`)
	})
}

// FullClean checks Meta.Constraints after the unique checks and skips them
// for fields that failed already, and WithValidateConstraints(false) turns
// them off. No Django test exists for the Go option; the order is Django's
// full_clean.
func TestConstraintsFullClean(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		invalid := &consProduct{Price: consPtr(10), DiscountedPrice: consPtr(42), Unit: consPtr("ng/mL")}
		ve := assertViolation(t, orm.FullClean(ctx, db, invalid), "Constraint “price_gt_discounted_price” is violated.")
		djangotest.AssertEqual(t, len(ve.MessageDict()[orm.NonFieldErrors]), 1)
		assertValid(t, orm.FullClean(ctx, db, invalid, orm.WithValidateConstraints(false)))
		// A unit longer than 15 characters fails its field, which keeps the
		// unit list constraint from running on it.
		err := orm.FullClean(ctx, db, &consProduct{Unit: consPtr(strings.Repeat("x", 16))})
		ve = assertViolation(t, err, "Ensure this value has at most 15 characters")
		djangotest.AssertEqual(t, len(ve.MessageDict()[orm.NonFieldErrors]), 0)
	})
}

// A unique constraint on one field reports its error under that field, as
// Django's validate_constraints does for the code "unique". No Django test
// exists for this placement.
func TestConstraintsValidateConstraintsSingleField(t *testing.T) {
	djangotest.EachDB(t, consModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		create(t, db, &consSingleUnique{Name: "a"})
		err := orm.ValidateConstraints(ctx, db, &consSingleUnique{Name: "a"})
		ve := assertViolation(t, err, "Cons single unique with this Name already exists.")
		djangotest.AssertEqual(t, ve.MessageDict()["name"], []string{"Cons single unique with this Name already exists."})
		assertValid(t, orm.ValidateConstraints(ctx, db, &consSingleUnique{Name: "a"}, orm.WithExclude("Name")))
		// The constraint is a unique check of a model formset too.
		reg := db.Registry()
		m, _ := reg.ModelOf(&consSingleUnique{})
		djangotest.AssertEqual(t, m.UniqueChecksWithConstraints(), [][]string{{"name"}, {"id"}})
		djangotest.AssertEqual(t, m.UniqueChecks(), [][]string{{"id"}})
	})
}

// consSingleUnique declares a unique constraint on one field.
type consSingleUnique struct {
	ID   int64
	Name string `orm:"max_length=20"`
}

// Meta declares the constraint.
func (consSingleUnique) Meta() orm.Meta {
	return orm.Meta{Constraints: []orm.Constraint{orm.UniqueConstraint{Fields: []string{"name"}, Name: "single_name_uniq"}}}
}

// consIndexed declares the unique constraints that CreateTable writes as
// unique indexes.
type consIndexed struct {
	ID     int64
	Name   string `orm:"max_length=20"`
	Height *int   `orm:"null"`
	Weight *int   `orm:"null"`
}

// Meta declares a unique constraint on an expression and one on fields whose
// NULL values are equal.
func (consIndexed) Meta() orm.Meta {
	return orm.Meta{Constraints: []orm.Constraint{
		orm.UniqueConstraint{Expressions: []orm.Expr{orm.Upper("name")}, Name: "func_upper_uq"},
		orm.UniqueConstraint{Fields: []string{"height", "weight"}, Name: "constraint", NullsNotDistinct: true},
	}}
}

// CreateTable writes a unique constraint on an expression, or with
// NullsNotDistinct, as a unique index. SQLite cannot treat NULL values as
// equal, so it gets no index for that constraint, as Django does. No Django
// test exists for the table of a model; tests/schema/tests.py checks the same
// SQL through add_constraint.
func TestConstraintsCreateTableUniqueIndexes(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("constraints", &consIndexed{})}, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		table := "constraints_consindexed"
		authors := orm.Query[consIndexed](db)
		djangotest.AssertConstraintExists(t, db, table, "func_upper_uq")
		djangotest.AssertEqual(t, authors.Create(ctx, &consIndexed{Name: "a"}), nil)
		if err := authors.Create(ctx, &consIndexed{Name: "A"}); !errors.Is(err, orm.ErrUniqueViolation) {
			t.Errorf("error = %v, want a unique violation", err)
		}

		djangotest.AssertEqual(t, authors.Create(ctx, &consIndexed{Name: "b", Height: consPtr(1)}), nil)
		djangotest.AssertEqual(t, authors.Create(ctx, &consIndexed{Name: "c", Weight: consPtr(1)}), nil)
		repeats := []*consIndexed{{Name: "d"}, {Name: "e", Height: consPtr(1)}, {Name: "f", Weight: consPtr(1)}}
		if db.Dialect().Name() == "sqlite" {
			djangotest.AssertConstraintNotExists(t, db, table, "constraint")
			for _, obj := range repeats {
				djangotest.AssertEqual(t, authors.Create(ctx, obj), nil)
			}
			// Model validation still finds the repeated values.
			err := orm.ValidateConstraints(ctx, db, &consIndexed{Name: "g"})
			assertViolation(t, err, "Cons indexed with this Height and Weight already exists.")
			return
		}
		djangotest.AssertConstraintExists(t, db, table, "constraint")
		for _, obj := range repeats {
			err := db.Atomic(ctx, func(ctx context.Context) error { return authors.Create(ctx, obj) })
			if !errors.Is(err, orm.ErrUniqueViolation) {
				t.Errorf("create %s: error = %v, want a unique violation", obj.Name, err)
			}
		}
	})
}
