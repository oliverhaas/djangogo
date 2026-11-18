package forms_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_base_form
func TestModelFormBaseForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, categoryOptions()...)
		djangotest.AssertEqual(t, f.FieldNames(), []string{"name", "slug", "url"})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_empty_fields_to_fields_for_model
func TestModelFormEmptyFieldsToFieldsForModel(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Person](db, forms.WithFields())
		djangotest.AssertEqual(t, len(f.FieldNames()), 0)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_fields_for_model_form_fields
func TestModelFormFieldsForModelFormFields(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		declared := forms.NewCharField(forms.Required(false))
		f := forms.NewModelForm[Writer](db, forms.WithFields("name"), forms.WithFormField("name", declared))
		got, _ := f.FieldDef("name")
		if got != forms.Field(declared) {
			t.Errorf("field name is %#v, want the declared field", got)
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_empty_fields_on_modelform
func TestModelFormEmptyFieldsOnModelForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := forms.NewModelForm[Person](db, forms.WithFields())
		djangotest.AssertEqual(t, len(form.FieldNames()), 0)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_empty_fields_to_construct_instance
//
// PopulateStruct copies the entries of the cleaned data, so empty cleaned data
// is the counterpart of construct_instance(fields=()).
func TestModelFormEmptyFieldsToConstructInstance(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		form := forms.NewModelForm[Person](db, forms.WithFields(forms.AllFields)).Bind(vals("name", "John Doe"))
		djangotest.AssertEqual(t, form.IsValid(), true)
		model, _ := db.Registry().ModelOf(&Person{})
		instance := &Person{}
		if err := forms.PopulateStruct(model, map[string]any{}, instance); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, instance.Name, "")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_blank_with_null_foreign_key_field
//
// #13776 -- ModelForm's with models having a FK set to null=False and
// required=False should be valid.
func TestModelFormBlankWithNullForeignKeyField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		char := &Character{Username: "user", LastAction: date(2024, 1, 1)}
		if err := orm.Query[Character](db).Create(ctx, char); err != nil {
			t.Fatal(err)
		}
		newForm := func() *forms.ModelForm[Student] {
			return forms.NewModelForm[Student](db, forms.WithFields(forms.AllFields), forms.WithRequired("character", false))
		}

		// form is valid because required=False for field 'character'
		f1 := newForm().Bind(vals("study", "Engineering"))
		djangotest.AssertEqual(t, f1.IsValid(), true)

		f2 := newForm().Bind(vals("study", "Engineering", "character", "1"))
		djangotest.AssertEqual(t, f2.IsValid(), true)
		obj, err := f2.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, obj.Character.PK(), char.ID)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_blank_false_with_null_true_foreign_key_field
//
// A ModelForm with a model having ForeignKey(blank=False, null=True) and the
// form field set to required=False should allow the field to be unset.
func TestModelFormBlankFalseWithNullTrueForeignKeyField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		character := &Character{Username: "user", LastAction: date(2024, 1, 1)}
		if err := orm.Query[Character](db).Create(ctx, character); err != nil {
			t.Fatal(err)
		}
		award := &Award{Name: "Best sprinter"}
		award.Character.SetPK(character.ID)
		if err := orm.Query[Award](db).Create(ctx, award); err != nil {
			t.Fatal(err)
		}
		// remove character
		form := forms.NewModelForm[Award](db, forms.WithFields(forms.AllFields), forms.WithRequired("character", false)).
			WithInstance(award).Bind(vals("name", "Best tester", "character", ""))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saved, err := form.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, saved.Character.PK(), int64(0))
		fresh, err := orm.Query[Award](db).Get(ctx, "id", award.ID)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, fresh.Character.PK(), int64(0))
	})
}

// renderedField returns the HTML of a field of a form whose options are
// loaded from the database.
func renderedField(form *forms.Form, name string) string {
	form.SetContext(context.Background())
	return form.Field(name).String()
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_blank_foreign_key_with_radio
func TestModelFormBlankForeignKeyWithRadio(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		writer := &Writer{Name: "Joe Doe"}
		if err := orm.Query[Writer](db).Create(context.Background(), writer); err != nil {
			t.Fatal(err)
		}
		form := forms.NewModelForm[Book](db, forms.WithFields("author"), forms.WithWidget("author", forms.RadioSelect{}))
		// Django's choices are [("", blank label), (pk, "Joe Doe")]; the radio
		// buttons show them.
		assertHTMLEqual(t, renderedField(form.Form, "author"),
			`<div id="id_author"><div>`+
				`<label for="id_author_0"><input type="radio" name="author" value="" id="id_author_0" checked> ---------</label></div><div>`+
				`<label for="id_author_1"><input type="radio" name="author" value="1" id="id_author_1"> Joe Doe</label></div></div>`)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_non_blank_foreign_key_with_radio
func TestModelFormNonBlankForeignKeyWithRadio(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		character := &Character{Username: "user", LastAction: date(2024, 1, 1)}
		if err := orm.Query[Character](db).Create(context.Background(), character); err != nil {
			t.Fatal(err)
		}
		form := forms.NewModelForm[Award](db, forms.WithFields("character"), forms.WithWidget("character", forms.RadioSelect{}))
		assertHTMLEqual(t, renderedField(form.Form, "character"),
			`<div id="id_character"><div>`+
				`<label for="id_character_0"><input type="radio" name="character" value="1" required id="id_character_0"> user</label></div></div>`)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_save_blank_false_with_required_false
//
// A ModelForm with a model with a field set to blank=False and the form field
// set to required=False should allow the field to be unset.
func TestModelFormSaveBlankFalseWithRequiredFalse(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		obj := &Writer{Name: "test"}
		if err := orm.Query[Writer](db).Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
		form := customWriterForm(db).WithInstance(obj).Bind(vals("name", ""))
		djangotest.AssertEqual(t, form.IsValid(), true)
		saved, err := form.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, saved.Name, "")
	})
}

// customWriterForm is CustomWriterForm of Django's tests/model_forms/tests.py:
// a Writer form that declares name as an optional field.
func customWriterForm(db *orm.DB) *forms.ModelForm[Writer] {
	return forms.NewModelForm[Writer](db, forms.WithFields(forms.AllFields),
		forms.WithFormField("name", forms.NewCharField(forms.Required(false))))
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_save_blank_null_unique_charfield_saves_null
func TestModelFormSaveBlankNullUniqueCharfieldSavesNull(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a string model field cannot be NULL, so a blank unique char field is saved as an empty string, Django: saves NULL so that blank values do not collide")
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_extra_fields
func TestModelFormExtraFields(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, categoryOptions()...)
		f.AddField("some_extra_field", forms.NewBooleanField())
		djangotest.AssertEqual(t, f.FieldNames(), []string{"name", "slug", "url", "some_extra_field"})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_extra_field_model_form
func TestModelFormExtraFieldModelForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Person](db, forms.WithFields("name", "no-field"))
		assertErrContains(t, f.Err(), "no-field")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_extra_declared_field_model_form
func TestModelFormExtraDeclaredFieldModelForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Person](db, forms.WithFields("name", "age"),
			forms.WithFormField("age", forms.NewIntegerField()))
		djangotest.AssertEqual(t, f.Err(), nil)
		djangotest.AssertEqual(t, f.FieldNames(), []string{"name", "age"})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_extra_field_modelform_factory
func TestModelFormExtraFieldModelformFactory(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Person](db, forms.WithFields("no-field", "name"))
		assertErrContains(t, f.Err(), "Unknown field(s) (no-field) specified for Person")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_replace_field
func TestModelFormReplaceField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, categoryOptions(forms.WithFields(forms.AllFields),
			forms.WithFormField("url", forms.NewBooleanField()))...)
		field, _ := f.FieldDef("url")
		if _, ok := field.(*forms.BooleanField); !ok {
			t.Errorf("url is %T, want a BooleanField", field)
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_replace_field_variant_2
//
// Should have the same result as before, but the fields are specified
// differently.
func TestModelFormReplaceFieldVariant2(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithFields("url"),
			forms.WithFormField("url", forms.NewBooleanField()))
		field, _ := f.FieldDef("url")
		if _, ok := field.(*forms.BooleanField); !ok {
			t.Errorf("url is %T, want a BooleanField", field)
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_replace_field_variant_3
//
// Should have the same result as before, but the fields are specified
// differently: url will still appear, since it is explicit.
func TestModelFormReplaceFieldVariant3(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithFields(),
			forms.WithFormField("url", forms.NewBooleanField()))
		field, ok := f.FieldDef("url")
		if !ok {
			t.Fatal("url is not a field of the form")
		}
		if _, ok := field.(*forms.BooleanField); !ok {
			t.Errorf("url is %T, want a BooleanField", field)
		}
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_override_field
func TestModelFormOverrideField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		wf := forms.NewModelForm[Writer](db, forms.WithFields(forms.AllFields),
			forms.WithFormField("book", forms.NewCharField(forms.Required(false)))).
			Bind(vals("name", "Richard Lockridge"))
		djangotest.AssertEqual(t, wf.IsValid(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_limit_nonexistent_field
func TestModelFormLimitNonexistentField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithFields("nonexistent"))
		assertErrContains(t, f.Err(), "Unknown field(s) (nonexistent) specified for Category")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_exclude_fields
func TestModelFormExcludeFields(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithExclude("url"))
		djangotest.AssertEqual(t, f.FieldNames(), []string{"name", "slug"})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_exclude_nonexistent_field
func TestModelFormExcludeNonexistentField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithExclude("nonexistent"))
		djangotest.AssertEqual(t, f.FieldNames(), []string{"name", "slug", "url"})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_exclude_and_validation
//
// The ORM has no commit=False and no model full_clean: the instance of the
// form before Save is the counterpart of save(commit=False), and the full_clean
// part is last. Django's Price.price is a DecimalField, a string here.
func TestModelFormExcludeAndValidation(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		// This Price instance generated by this form is not valid because the
		// quantity field is required, but the form is valid because the field
		// is excluded from the form. This is for backwards compatibility.
		form := forms.NewModelForm[Price](db, forms.WithExclude("quantity")).Bind(vals("price", "6.00"))
		djangotest.AssertEqual(t, form.IsValid(), true)

		// The form should not validate fields that it doesn't contain even if
		// they are specified using 'fields', not 'exclude'.
		form = forms.NewModelForm[Price](db, forms.WithFields("price")).Bind(vals("price", "6.00"))
		djangotest.AssertEqual(t, form.IsValid(), true)

		// The form should still have an instance of a model that is not
		// complete and not saved into a DB yet.
		djangotest.AssertEqual(t, form.Instance().Price, "6.00")
		djangotest.AssertEqual(t, form.Instance().Quantity, 0)
		djangotest.AssertEqual(t, form.Instance().ID, int64(0))

		t.Skip("FIXME-PORT: Go: the ORM has no model full_clean, Django: price.full_clean() raises {'quantity': ['This field cannot be null.']}")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_confused_form
//
// Using 'fields' *and* 'exclude'. Not sure why you'd want to do this, but uh,
// "be liberal in what you accept" and all.
func TestModelFormConfusedForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithFields("name", "url"), forms.WithExclude("url"))
		djangotest.AssertEqual(t, f.FieldNames(), []string{"name"})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_article_form
func TestModelFormArticleForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Article](db, articleOptions()...)
		djangotest.AssertEqual(t, f.FieldNames(),
			[]string{"headline", "slug", "pub_date", "writer", "article", "status"})
		t.Skip("FIXME-PORT: Go: the ORM has no ManyToManyField, Django: the form has a categories field between article and status")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_subclassmeta_form
//
// A form with an extra declared field and the url field excluded.
func TestModelFormSubclassMetaForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, categoryOptions(forms.WithExclude("url"))...)
		f.AddField("checkbox", forms.NewBooleanField(forms.Required(true)))
		assertHTMLEqual(t, f.String(),
			`<div><label for="id_name">Name:</label>`+
				`<input type="text" name="name" maxlength="20" required id="id_name">`+
				`</div><div><label for="id_slug">Slug:</label><input type="text" `+
				`name="slug" maxlength="20" required id="id_slug"></div><div>`+
				`<label for="id_checkbox">Checkbox:</label>`+
				`<input type="checkbox" name="checkbox" required id="id_checkbox"></div>`)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_orderfields_form
func TestModelFormOrderfieldsForm(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithFields("url", "name"))
		djangotest.AssertEqual(t, f.FieldNames(), []string{"url", "name"})
		assertHTMLEqual(t, f.String(),
			`<div><label for="id_url">The URL:</label>`+
				`<input type="text" name="url" maxlength="40" required id="id_url">`+
				`</div><div><label for="id_name">Name:</label><input type="text" `+
				`name="name" maxlength="20" required id="id_name"></div>`)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelFormBaseTest.test_orderfields2_form
func TestModelFormOrderfields2Form(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := forms.NewModelForm[Category](db, forms.WithFields("slug", "url", "name"), forms.WithExclude("url"))
		djangotest.AssertEqual(t, f.FieldNames(), []string{"slug", "name"})
	})
}
