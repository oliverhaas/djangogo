package orm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// valProduct mirrors Django's validation.Product.
type valProduct struct {
	ID              int64
	Price           *int `orm:"null"`
	DiscountedPrice *int `orm:"null"`
}

// Meta declares the check constraint of Django's Product.
func (valProduct) Meta() orm.Meta {
	return orm.Meta{
		VerboseName: "product",
		Constraints: []orm.Constraint{
			orm.CheckConstraint{
				Condition: orm.Q("price__gt", orm.F("discounted_price")),
				Name:      "price_gt_discounted_price_validation",
			},
		},
	}
}

// valUniqueConstraintProduct mirrors Django's
// validation.UniqueConstraintProduct.
type valUniqueConstraintProduct struct {
	ID    int64
	Name  string `orm:"max_length=255"`
	Color string `orm:"max_length=32"`
	Rank  int
}

// Meta declares the unique constraints of Django's UniqueConstraintProduct.
func (valUniqueConstraintProduct) Meta() orm.Meta {
	return orm.Meta{
		VerboseName: "unique constraint product",
		Constraints: []orm.Constraint{
			orm.UniqueConstraint{Fields: []string{"name", "color"}, Name: "name_color_uniq_validation"},
			orm.UniqueConstraint{Fields: []string{"rank"}, Name: "rank_uniq_validation"},
		},
	}
}

// valConditionProduct mirrors Django's
// validation.UniqueConstraintConditionProduct.
type valConditionProduct struct {
	ID    int64
	Name  string  `orm:"max_length=255"`
	Color *string `orm:"max_length=31;null;blank"`
}

// Meta declares the partial unique constraint of Django's
// UniqueConstraintConditionProduct.
func (valConditionProduct) Meta() orm.Meta {
	return orm.Meta{
		Constraints: []orm.Constraint{
			orm.UniqueConstraint{
				Fields:    []string{"name"},
				Name:      "name_without_color_uniq_validation",
				Condition: orm.Q("color__isnull", true),
			},
		},
	}
}

// valConstraintModels returns the models of the constraint checks of Django's
// validation app.
func valConstraintModels() []any {
	return []any{djangotest.App("validation", &valProduct{}, &valUniqueConstraintProduct{}, &valConditionProduct{})}
}

// assertMessageDict checks that err is a validation error with the messages
// want.
func assertMessageDict(t *testing.T, err error, want map[string][]string) {
	t.Helper()
	var ve *orm.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want a validation error", err)
	}
	djangotest.AssertEqual(t, ve.MessageDict(), want)
}

// No Go counterpart: PerformConstraintChecksTest.test_full_clean_with_check_constraints_on_child_model
// and the child model of test_full_clean_with_unique_constraints need
// multi-table inheritance.

// Ported from Django: tests/validation/test_constraints.py::PerformConstraintChecksTest.test_full_clean_with_check_constraints
func TestPerformConstraintChecksCheckConstraints(t *testing.T) {
	djangotest.EachDB(t, valConstraintModels(), func(t *testing.T, db *orm.DB) {
		err := orm.FullClean(context.Background(), db, &valProduct{Price: consPtr(10), DiscountedPrice: consPtr(15)})
		assertMessageDict(t, err, map[string][]string{
			orm.NonFieldErrors: {"Constraint “price_gt_discounted_price_validation” is violated."},
		})
	})
}

// Ported from Django: tests/validation/test_constraints.py::PerformConstraintChecksTest.test_full_clean_with_check_constraints_disabled
func TestPerformConstraintChecksCheckConstraintsDisabled(t *testing.T) {
	djangotest.EachDB(t, valConstraintModels(), func(t *testing.T, db *orm.DB) {
		err := orm.FullClean(context.Background(), db, &valProduct{Price: consPtr(10), DiscountedPrice: consPtr(15)},
			orm.WithValidateConstraints(false))
		assertValid(t, err)
	})
}

// Ported from Django: tests/validation/test_constraints.py::PerformConstraintChecksTest.test_full_clean_with_unique_constraints
func TestPerformConstraintChecksUniqueConstraints(t *testing.T) {
	djangotest.EachDB(t, valConstraintModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &valUniqueConstraintProduct{Name: "product", Color: "yellow", Rank: 1})
		err := orm.FullClean(context.Background(), db, &valUniqueConstraintProduct{Name: "product", Color: "yellow", Rank: 1})
		assertMessageDict(t, err, map[string][]string{
			orm.NonFieldErrors: {"Unique constraint product with this Name and Color already exists."},
			"rank":             {"Unique constraint product with this Rank already exists."},
		})
	})
}

// Ported from Django: tests/validation/test_constraints.py::PerformConstraintChecksTest.test_full_clean_with_unique_constraints_disabled
func TestPerformConstraintChecksUniqueConstraintsDisabled(t *testing.T) {
	djangotest.EachDB(t, valConstraintModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &valUniqueConstraintProduct{Name: "product", Color: "yellow", Rank: 1})
		err := orm.FullClean(context.Background(), db, &valUniqueConstraintProduct{Name: "product", Color: "yellow", Rank: 1},
			orm.WithValidateConstraints(false))
		assertValid(t, err)
	})
}

// Ported from Django: tests/validation/test_constraints.py::PerformConstraintChecksTest.test_full_clean_with_partial_unique_constraints
func TestPerformConstraintChecksPartialUniqueConstraints(t *testing.T) {
	djangotest.EachDB(t, valConstraintModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &valConditionProduct{Name: "product"})
		err := orm.FullClean(context.Background(), db, &valConditionProduct{Name: "product"})
		assertMessageDict(t, err, map[string][]string{
			orm.NonFieldErrors: {"Constraint “name_without_color_uniq_validation” is violated."},
		})
	})
}

// Ported from Django: tests/validation/test_constraints.py::PerformConstraintChecksTest.test_full_clean_with_partial_unique_constraints_disabled
func TestPerformConstraintChecksPartialUniqueConstraintsDisabled(t *testing.T) {
	djangotest.EachDB(t, valConstraintModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &valConditionProduct{Name: "product"})
		err := orm.FullClean(context.Background(), db, &valConditionProduct{Name: "product"}, orm.WithValidateConstraints(false))
		assertValid(t, err)
	})
}
