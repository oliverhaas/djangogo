package forms_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// pubFormMode is PubForm of Django's ModelFormBaseTest default tests: a
// PublicationDefaults form that declares mode as an optional char field.
func pubFormMode(db *orm.DB, field forms.Field) *forms.ModelForm[PublicationDefaults] {
	return forms.NewModelForm[PublicationDefaults](db, forms.WithFields("mode"), forms.WithFormField("mode", field))
}

// modelDefault returns the default of a model field of the model of obj.
func modelDefault(t *testing.T, db *orm.DB, obj any, attr string) any {
	t.Helper()
	model, ok := db.Registry().ModelOf(obj)
	if !ok {
		t.Fatalf("%T is not registered", obj)
	}
	field, ok := model.FieldByAttr(attr)
	if !ok {
		t.Fatalf("%T has no field %s", obj, attr)
	}
	return field.Default
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_default_populated_on_optional_field
//
// The ORM has no commit=False: the instance of the form after validation is
// the counterpart of save(commit=False).
func TestModelFormDefaultPopulatedOnOptionalField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		newForm := func() *forms.ModelForm[PublicationDefaults] {
			return pubFormMode(db, forms.NewCharField(forms.MaxLength(255), forms.Required(false)))
		}

		// Blank data doesn't use the model field default.
		mf2 := newForm().Bind(vals("mode", ""))
		djangotest.AssertEqual(t, mf2.IsValid(), true)
		djangotest.AssertEqual(t, mf2.Instance().Mode, "")

		// Empty data uses the model field default.
		mf1 := newForm().Bind(vals())
		djangotest.AssertEqual(t, mf1.IsValid(), true)
		djangotest.AssertEqual(t, modelDefault(t, db, &PublicationDefaults{}, "mode"), any("di"))
		djangotest.AssertEqual(t, mf1.Instance().Mode, "di")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_default_not_populated_on_non_empty_value_in_cleaned_data
func TestModelFormDefaultNotPopulatedOnNonEmptyValueInCleanedData(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		newForm := func(mocked any) *forms.ModelForm[PublicationDefaults] {
			f := pubFormMode(db, forms.NewCharField(forms.MaxLength(255), forms.Required(false)))
			f.CleanWith(func(form *forms.Form) error {
				form.CleanedData()["mode"] = mocked
				return nil
			})
			return f.Bind(vals())
		}
		pub := newForm("de")
		pub.IsValid()
		djangotest.AssertEqual(t, pub.Instance().Mode, "de")

		// Default should be populated on an empty value in cleaned_data.
		for _, empty := range []any{nil, "", []any{}} {
			pub = newForm(empty)
			pub.IsValid()
			if got := pub.Instance().Mode; got != "di" {
				t.Errorf("empty value %#v: mode = %q, want the default %q", empty, got, "di")
			}
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_default_not_populated_on_optional_checkbox_input
func TestModelFormDefaultNotPopulatedOnOptionalCheckboxInput(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		// Empty data doesn't use the model default because CheckboxInput
		// doesn't have a value in HTML form submission.
		mf1 := forms.NewModelForm[PublicationDefaults](db, forms.WithFields("active")).Bind(vals())
		djangotest.AssertEqual(t, mf1.Errors().Len(), 0)
		djangotest.AssertEqual(t, mf1.IsValid(), true)
		djangotest.AssertEqual(t, mf1.Instance().Active, false)
		field, _ := mf1.FieldDef("active")
		if _, ok := field.Base().Widget.(forms.CheckboxInput); !ok {
			t.Errorf("widget of active is %T, want a CheckboxInput", field.Base().Widget)
		}
		djangotest.AssertEqual(t, modelDefault(t, db, &PublicationDefaults{}, "active"), any(true))
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_default_not_populated_on_checkboxselectmultiple
func TestModelFormDefaultNotPopulatedOnCheckboxSelectMultiple(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		// Empty data doesn't use the model default because an unchecked
		// CheckboxSelectMultiple doesn't have a value in HTML form submission.
		mf1 := pubFormMode(db, forms.NewCharField(forms.Required(false), forms.UseWidget(forms.CheckboxSelectMultiple{}))).Bind(vals())
		djangotest.AssertEqual(t, mf1.Errors().Len(), 0)
		djangotest.AssertEqual(t, mf1.IsValid(), true)
		djangotest.AssertEqual(t, mf1.Instance().Mode, "")
		djangotest.AssertEqual(t, modelDefault(t, db, &PublicationDefaults{}, "mode"), any("di"))
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_default_not_populated_on_selectmultiple
func TestModelFormDefaultNotPopulatedOnSelectMultiple(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		// Empty data doesn't use the model default because an unselected
		// SelectMultiple doesn't have a value in HTML form submission.
		mf1 := pubFormMode(db, forms.NewCharField(forms.Required(false), forms.UseWidget(forms.SelectMultiple{}))).Bind(vals())
		djangotest.AssertEqual(t, mf1.Errors().Len(), 0)
		djangotest.AssertEqual(t, mf1.IsValid(), true)
		djangotest.AssertEqual(t, mf1.Instance().Mode, "")
		djangotest.AssertEqual(t, modelDefault(t, db, &PublicationDefaults{}, "mode"), any("di"))
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_prefixed_form_with_default_field
func TestModelFormPrefixedFormWithDefaultField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		mode := "de"
		if modelDefault(t, db, &PublicationDefaults{}, "mode") == any(mode) {
			t.Fatal("the default of mode is the tested mode")
		}
		form := forms.NewModelForm[PublicationDefaults](db, forms.WithFields("mode"))
		form.Prefix = "form-prefix"
		mf1 := form.Bind(vals("form-prefix-mode", mode))
		djangotest.AssertEqual(t, mf1.Errors().Len(), 0)
		djangotest.AssertEqual(t, mf1.IsValid(), true)
		djangotest.AssertEqual(t, mf1.Instance().Mode, mode)
	})
}

// fieldOverridesForm is FieldOverridesByFormMetaForm of Django's
// tests/model_forms/tests.py. Go's URLField takes the place of Meta.field_classes,
// so the form field carries the label and length of the model field.
func fieldOverridesForm(db *orm.DB) *forms.ModelForm[Category] {
	return forms.NewModelForm[Category](db,
		forms.WithFields("name", "url", "slug"),
		forms.WithFormField("slug", forms.NewSlugField(forms.MaxLength(20))),
		forms.WithFormField("url", forms.NewURLField(forms.MaxLength(40), forms.Label("The URL"))),
		forms.WithWidget("name", forms.Textarea{}),
		forms.WithWidget("url", forms.TextInput{Attrs: forms.Attrs{"class": "url"}}),
		forms.WithLabel("name", "Title"),
		forms.WithHelpText("slug", "Watch out! Letters, numbers, underscores and hyphens only."),
		forms.WithErrorMessages("slug", map[string]string{
			"invalid": "Didn't you read the help text? " +
				"We said letters, numbers, underscores and hyphens only!",
		}),
	)
}

// Ported from Django: tests/model_forms/tests.py::TestFieldOverridesByFormMeta.test_widget_overrides
func TestFieldOverridesByFormMetaWidgetOverrides(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := fieldOverridesForm(db)
		assertHTMLEqual(t, form.Field("name").String(),
			`<textarea id="id_name" rows="10" cols="40" name="name" maxlength="20" required></textarea>`)
		assertHTMLEqual(t, form.Field("url").String(),
			`<input id="id_url" type="text" class="url" name="url" maxlength="40" required>`)
		assertHTMLEqual(t, form.Field("slug").String(),
			`<input id="id_slug" type="text" name="slug" maxlength="20" aria-describedby="id_slug_helptext" required>`)
	})
}

// Ported from Django: tests/model_forms/tests.py::TestFieldOverridesByFormMeta.test_label_overrides
func TestFieldOverridesByFormMetaLabelOverrides(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := fieldOverridesForm(db)
		assertHTMLEqual(t, form.Field("name").LabelTag(), `<label for="id_name">Title:</label>`)
		assertHTMLEqual(t, form.Field("url").LabelTag(), `<label for="id_url">The URL:</label>`)
		assertHTMLEqual(t, form.Field("slug").LabelTag(), `<label for="id_slug">Slug:</label>`)
		assertHTMLEqual(t, form.Field("name").LegendTag(), "<legend>Title:</legend>")
		assertHTMLEqual(t, form.Field("url").LegendTag(), "<legend>The URL:</legend>")
		assertHTMLEqual(t, form.Field("slug").LegendTag(), "<legend>Slug:</legend>")
	})
}

// Ported from Django: tests/model_forms/tests.py::TestFieldOverridesByFormMeta.test_help_text_overrides
func TestFieldOverridesByFormMetaHelpTextOverrides(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := fieldOverridesForm(db)
		djangotest.AssertEqual(t, form.Field("slug").HelpText(),
			"Watch out! Letters, numbers, underscores and hyphens only.")
	})
}

// Ported from Django: tests/model_forms/tests.py::TestFieldOverridesByFormMeta.test_error_messages_overrides
func TestFieldOverridesByFormMetaErrorMessagesOverrides(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := fieldOverridesForm(db).Bind(vals(
			"name", "Category", "url", "http://www.example.com/category/", "slug", "!%#*@"))
		djangotest.AssertEqual(t, form.Errors().Messages(), map[string][]string{
			"slug": {"Didn't you read the help text? We said letters, numbers, underscores and hyphens only!"},
		})
	})
}

// Ported from Django: tests/model_forms/tests.py::TestFieldOverridesByFormMeta.test_field_type_overrides
//
// Meta.field_classes becomes WithFormField, which takes the field itself.
func TestFieldOverridesByFormMetaFieldTypeOverrides(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := fieldOverridesForm(db)
		model, _ := db.Registry().ModelOf(&Category{})
		field, _ := model.FieldByAttr("url")
		djangotest.AssertEqual(t, field.Kind, orm.KindChar)
		got, _ := form.FieldDef("url")
		if _, ok := got.(*forms.URLField); !ok {
			t.Errorf("url is %T, want a URLField", got)
		}
	})
}

// incompleteCategoryForm is IncompleteCategoryFormWithFields and
// IncompleteCategoryFormWithExclude of Django's tests/model_forms/tests.py: a
// form that replaces the url field with a custom one.
func incompleteCategoryForm(db *orm.DB, opt forms.ModelFormOption) *forms.ModelForm[Category] {
	return forms.NewModelForm[Category](db, opt,
		forms.WithFormField("slug", forms.NewSlugField(forms.MaxLength(20))),
		forms.WithFormField("url", forms.NewCharField(forms.Required(false))))
}

// Ported from Django: tests/model_forms/tests.py::ValidationTest.test_validates_with_replaced_field_not_specified
func TestValidationValidatesWithReplacedFieldNotSpecified(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := incompleteCategoryForm(db, forms.WithFields("name", "slug")).Bind(vals("name", "some name", "slug", "some-slug"))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::ValidationTest.test_validates_with_replaced_field_excluded
func TestValidationValidatesWithReplacedFieldExcluded(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := incompleteCategoryForm(db, forms.WithExclude("url")).Bind(vals("name", "some name", "slug", "some-slug"))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::ValidationTest.test_notrequired_overrides_notblank
func TestValidationNotrequiredOverridesNotblank(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := customWriterForm(db).Bind(vals())
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_simple_unique
func TestUniqueSimpleUnique(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		newForm := func() *forms.ModelForm[Product] {
			return forms.NewModelForm[Product](db, forms.WithFields(forms.AllFields),
				forms.WithFormField("slug", forms.NewSlugField(forms.MaxLength(50))))
		}
		form := newForm().Bind(vals("slug", "teddy-bear-blue"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		obj, err := form.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		form = newForm().Bind(vals("slug", "teddy-bear-blue"))
		djangotest.AssertEqual(t, form.Errors().Len(), 1)
		djangotest.AssertEqual(t, form.Errors().Messages()["slug"], []string{"Product with this Slug already exists."})
		form = newForm().WithInstance(obj).Bind(vals("slug", "teddy-bear-blue"))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_unique_together
func TestUniqueUniqueTogether(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		newForm := func() *forms.ModelForm[Price] {
			return forms.NewModelForm[Price](db, forms.WithFields(forms.AllFields))
		}
		form := newForm().Bind(vals("price", "6.00", "quantity", "1"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		if _, err := form.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		form = newForm().Bind(vals("price", "6.00", "quantity", "1"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, form.Errors().Len(), 1)
		djangotest.AssertEqual(t, form.Errors().Messages()["__all__"],
			[]string{"Price with this Price and Quantity already exists."})
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_unique_null
func TestUniqueUniqueNull(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		writer := &Writer{Name: "Mike Royko"}
		if err := orm.Query[Writer](db).Create(ctx, writer); err != nil {
			t.Fatal(err)
		}
		title := "I May Be Wrong But I Doubt It"
		newForm := func() *forms.ModelForm[Book] {
			return forms.NewModelForm[Book](db, forms.WithExclude("special_id"))
		}
		form := newForm().Bind(vals("title", title, "author", "1"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		if _, err := form.Save(ctx); err != nil {
			t.Fatal(err)
		}
		form = newForm().Bind(vals("title", title, "author", "1"))
		djangotest.AssertEqual(t, form.IsValid(), false)
		djangotest.AssertEqual(t, form.Errors().Len(), 1)
		djangotest.AssertEqual(t, form.Errors().Messages()["__all__"],
			[]string{"Book with this Title and Author already exists."})

		// NULL author does not trigger unique_together.
		form = newForm().Bind(vals("title", title))
		djangotest.AssertEqual(t, form.IsValid(), true)
		if _, err := form.Save(ctx); err != nil {
			t.Fatal(err)
		}
		form = newForm().Bind(vals("title", title))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_explicitpk_unspecified
//
// Test for primary_key being in the form and failing validation.
func TestUniqueExplicitpkUnspecified(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := forms.NewModelForm[ExplicitPK](db, forms.WithFields("key", "desc")).Bind(vals("key", "", "desc", ""))
		djangotest.AssertEqual(t, form.IsValid(), false)
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_explicitpk_unique
//
// Ensure keys and blank character strings are tested for uniqueness.
func TestUniqueExplicitpkUnique(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		newForm := func() *forms.ModelForm[ExplicitPK] {
			return forms.NewModelForm[ExplicitPK](db, forms.WithFields("key", "desc"))
		}
		form := newForm().Bind(vals("key", "key1", "desc", ""))
		djangotest.AssertEqual(t, form.IsValid(), true)
		if _, err := form.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		form = newForm().Bind(vals("key", "key1", "desc", ""))
		djangotest.AssertEqual(t, form.IsValid(), false)
		messages := form.Errors().Messages()
		djangotest.AssertEqual(t, form.Errors().Len(), 3)
		djangotest.AssertEqual(t, messages["desc"], []string{"Explicit pk with this Desc already exists."})
		djangotest.AssertEqual(t, messages["key"], []string{"Explicit pk with this Key already exists."})
		djangotest.AssertEqual(t, messages["__all__"],
			[]string{"Explicit pk with this Key and Desc already exists."})
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_override_unique_message
func TestUniqueOverrideUniqueMessage(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		if err := orm.Query[Product](db).Create(context.Background(), &Product{Slug: "teddy-bear-blue"}); err != nil {
			t.Fatal(err)
		}
		form := forms.NewModelForm[Product](db, forms.WithFields(forms.AllFields),
			forms.WithFormField("slug", forms.NewSlugField(forms.MaxLength(50))),
			forms.WithErrorMessages("slug", map[string]string{
				"unique": "%(model_name)s's %(field_label)s not unique.",
			})).Bind(vals("slug", "teddy-bear-blue"))
		djangotest.AssertEqual(t, form.Errors().Len(), 1)
		djangotest.AssertEqual(t, form.Errors().Messages()["slug"], []string{"Product's Slug not unique."})
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_override_unique_together_message
func TestUniqueOverrideUniqueTogetherMessage(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		if err := orm.Query[Price](db).Create(context.Background(), &Price{Price: orm.MustDecimal("6.00"), Quantity: 1}); err != nil {
			t.Fatal(err)
		}
		form := forms.NewModelForm[Price](db, forms.WithFields(forms.AllFields),
			forms.WithErrorMessages(forms.NonFieldErrors, map[string]string{
				"unique_together": "%(model_name)s's %(field_labels)s not unique.",
			})).Bind(vals("price", "6.00", "quantity", "1"))
		djangotest.AssertEqual(t, form.Errors().Len(), 1)
		djangotest.AssertEqual(t, form.Errors().Messages()[forms.NonFieldErrors],
			[]string{"Price's Price and Quantity not unique."})
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_unique_together_exclusion
func TestUniqueUniqueTogetherExclusion(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		if err := orm.Query[DerivedBook](db).Create(context.Background(), &DerivedBook{ISBN: "12345"}); err != nil {
			t.Fatal(err)
		}
		// Only suffix1 is included in the form, so unique_together(suffix1, suffix2) is skipped.
		form := forms.NewModelForm[DerivedBook](db, forms.WithFields("isbn", "suffix1")).
			Bind(vals("isbn", "56789", "suffix1", "0"))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_multiple_field_unique_together
func TestUniqueMultipleFieldUniqueTogether(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		if err := orm.Query[Triple](db).Create(context.Background(), &Triple{Left: 1, Middle: 2, Right: 3}); err != nil {
			t.Fatal(err)
		}
		form := forms.NewModelForm[Triple](db, forms.WithFields(forms.AllFields)).
			Bind(vals("left", "1", "middle", "2", "right", "3"))
		djangotest.AssertEqual(t, form.IsValid(), false)

		form = forms.NewModelForm[Triple](db, forms.WithFields(forms.AllFields)).
			Bind(vals("left", "1", "middle", "3", "right", "1"))
		djangotest.AssertEqual(t, form.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_unique_for_date
func TestUniqueUniqueForDate(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no unique_for_date, unique_for_month or unique_for_year, Django: Post.title is unique per day of posted, slug per year and subtitle per month")
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_unique_for_date_in_exclude
func TestUniqueUniqueForDateInExclude(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no unique_for_date, Django: a unique_for_date check is skipped when the date field is excluded (DateTimePost)")
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_unique_for_date_with_nullable_date
func TestUniqueUniqueForDateWithNullableDate(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no unique_for_date, Django: a unique_for_date check is skipped for a null date (FlexibleDatePost)")
}

// Ported from Django: tests/model_forms/tests.py::UniqueTest.test_override_unique_for_date_message
func TestUniqueOverrideUniqueForDateMessage(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no unique_for_date, Django: Meta.error_messages 'unique_for_date' gives \"%(model_name)s's %(field_label)s not unique for %(date_field_label)s date.\"")
}
