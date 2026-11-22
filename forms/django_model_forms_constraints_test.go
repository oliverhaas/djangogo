package forms_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/model_forms/tests.py::ConstraintValidationTests.test_unique_constraint_refs_excluded_field
func TestConstraintValidationUniqueConstraintRefsExcludedField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		obj := &ConstraintsModel{Name: "product", Category: "uncategorized", Price: orm.MustDecimal("1.00")}
		if err := orm.Query[ConstraintsModel](db).Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
		data := vals("id", "", "name", obj.Name, "price", "1337.00", "category", obj.Category)
		fullForm := forms.NewModelForm[ConstraintsModel](db, forms.WithFields(forms.AllFields)).Bind(data)
		excludeCategoryForm := forms.NewModelForm[ConstraintsModel](db, forms.WithExclude("category")).Bind(data)
		djangotest.AssertEqual(t, excludeCategoryForm.IsValid(), true)
		djangotest.AssertEqual(t, fullForm.IsValid(), false)
		djangotest.AssertEqual(t, fullForm.Errors().Messages(), map[string][]string{
			"__all__": {"This product already exists."},
		})
	})
}

// Ported from Django: tests/model_forms/tests.py::ConstraintValidationTests.test_check_constraint_refs_excluded_field
func TestConstraintValidationCheckConstraintRefsExcludedField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		data := vals("id", "", "name", "priceless", "price", "0.00", "category", "category 1")
		fullForm := forms.NewModelForm[ConstraintsModel](db, forms.WithFields(forms.AllFields)).Bind(data)
		excludePriceForm := forms.NewModelForm[ConstraintsModel](db, forms.WithExclude("price")).Bind(data)
		djangotest.AssertEqual(t, excludePriceForm.IsValid(), true)
		djangotest.AssertEqual(t, fullForm.IsValid(), false)
		djangotest.AssertEqual(t, fullForm.Errors().Messages(), map[string][]string{
			"__all__": {"Price must be greater than zero."},
		})
	})
}

// Ported from Django: tests/model_forms/tests.py::ConstraintValidationTests.test_check_constraint_refs_excluded_field_attname
func TestConstraintValidationCheckConstraintRefsExcludedFieldAttname(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		left := &AttnameConstraintsModel{}
		if err := orm.Query[AttnameConstraintsModel](db).Create(ctx, left); err != nil {
			t.Fatal(err)
		}
		instance := &AttnameConstraintsModel{}
		instance.Left.SetPK(left.ID)
		if err := orm.Query[AttnameConstraintsModel](db).Create(ctx, instance); err != nil {
			t.Fatal(err)
		}
		data := vals("left", strconv.FormatInt(left.ID, 10), "right", "")
		fullForm := forms.NewModelForm[AttnameConstraintsModel](db, forms.WithFields(forms.AllFields)).
			WithInstance(instance).Bind(data)
		djangotest.AssertEqual(t, fullForm.IsValid(), false)
		djangotest.AssertEqual(t, fullForm.Errors().Messages(), map[string][]string{
			"right": {"This field is required."},
		})
	})
}
