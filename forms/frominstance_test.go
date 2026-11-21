package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// No Django test exists: FromInstance is the Go form of a ModelForm whose
// foreign key options come from the caller, which the admin uses. The test
// checks that validation fills the instance also when the form is invalid,
// and that the model validation and the unique checks run with their
// overridden messages.
func TestFromInstance(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		model, _ := db.Registry().ModelOf(&Price{})
		existing := &Price{Price: orm.MustDecimal("6.00"), Quantity: 1}
		if err := orm.Query[Price](db).Create(t.Context(), existing); err != nil {
			t.Fatal(err)
		}

		price := &Price{}
		form := forms.FromInstance(db, model, price, forms.WithFields(forms.AllFields),
			forms.WithErrorMessages(forms.NonFieldErrors, map[string]string{
				"unique_together": "%(model_name)s's %(field_labels)s not unique.",
			})).Bind(vals("price", "6.00", "quantity", "1"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, form.Errors().Messages()[forms.NonFieldErrors],
			[]string{"Price's Price and Quantity not unique."})
		djangotest.AssertEqual(t, price.Price.String(), "6.00")
		djangotest.AssertEqual(t, price.Quantity, 1)

		// The unique check leaves out the row of the instance itself.
		stored, err := orm.Query[Price](db).Get(t.Context(), "id", existing.ID)
		if err != nil {
			t.Fatal(err)
		}
		form = forms.FromInstance(db, model, &stored, forms.WithFields(forms.AllFields)).
			Bind(vals("price", "7.00", "quantity", "1"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		djangotest.AssertEqual(t, stored.Price.String(), "7.00")
	})
}
