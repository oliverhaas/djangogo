package inlineformsets_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// poemFormSet is the PoemFormSet of DeletionTests, with can_delete=True and
// every field.
func poemFormSet(db *orm.DB) *forms.InlineFormSet[Poet, Poem] {
	fs := forms.NewInlineFormSet[Poet, Poem](db, "", forms.WithFields(forms.AllFields))
	fs.CanDelete = true
	return fs
}

// Ported from Django: tests/inline_formsets/tests.py::DeletionTests.test_deletion
func TestDeletion(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := create(t, db, &Poet{Name: "test"})
		poem := create(t, db, &Poem{Poet: fk[Poet](poet.ID), Name: "test poem"})
		data := postData(
			"poem_set-TOTAL_FORMS", "1",
			"poem_set-INITIAL_FORMS", "1",
			"poem_set-MAX_NUM_FORMS", "0",
			"poem_set-0-id", strconv.FormatInt(poem.ID, 10),
			"poem_set-0-poet", strconv.FormatInt(poet.ID, 10),
			"poem_set-0-name", "test",
			"poem_set-0-DELETE", "on",
		)
		formset := poemFormSet(db).WithInstance(poet).Bind(data)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, formset.IsValid(), true)
		djangotest.AssertEqual(t, count(t, orm.Query[Poem](db)), int64(0))
	})
}

// Ported from Django: tests/inline_formsets/tests.py::DeletionTests.test_add_form_deletion_when_invalid
func TestAddFormDeletionWhenInvalid(t *testing.T) {
	// Make sure that an add form that is filled out, but marked for deletion
	// doesn't cause validation errors.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := create(t, db, &Poet{Name: "test"})
		data := postData(
			"poem_set-TOTAL_FORMS", "1",
			"poem_set-INITIAL_FORMS", "0",
			"poem_set-MAX_NUM_FORMS", "0",
			"poem_set-0-id", "",
			"poem_set-0-poem", "1",
			"poem_set-0-name", strings.Repeat("x", 1000),
		)
		formset := poemFormSet(db).WithInstance(poet).Bind(data)
		// Make sure this form doesn't pass validation.
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, count(t, orm.Query[Poem](db)), int64(0))

		// Then make sure that it *does* pass validation and delete the
		// object, even though the data isn't actually valid.
		data.Set("poem_set-0-DELETE", "on")
		formset = poemFormSet(db).WithInstance(poet).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count(t, orm.Query[Poem](db)), int64(0))
	})
}

// Ported from Django: tests/inline_formsets/tests.py::DeletionTests.test_change_form_deletion_when_invalid
func TestChangeFormDeletionWhenInvalid(t *testing.T) {
	// Make sure that a change form that is filled out, but marked for
	// deletion doesn't cause validation errors.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := create(t, db, &Poet{Name: "test"})
		poem := create(t, db, &Poem{Poet: fk[Poet](poet.ID), Name: "test poem"})
		data := postData(
			"poem_set-TOTAL_FORMS", "1",
			"poem_set-INITIAL_FORMS", "1",
			"poem_set-MAX_NUM_FORMS", "0",
			"poem_set-0-id", strconv.FormatInt(poem.ID, 10),
			"poem_set-0-poem", strconv.FormatInt(poem.ID, 10),
			"poem_set-0-name", strings.Repeat("x", 1000),
		)
		formset := poemFormSet(db).WithInstance(poet).Bind(data)
		// Make sure this form doesn't pass validation.
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, count(t, orm.Query[Poem](db)), int64(1))

		// Then make sure that it *does* pass validation and delete the
		// object, even though the data isn't actually valid.
		data.Set("poem_set-0-DELETE", "on")
		formset = poemFormSet(db).WithInstance(poet).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count(t, orm.Query[Poem](db)), int64(0))
	})
}

// Ported from Django: tests/inline_formsets/tests.py::DeletionTests.test_save_new
func TestSaveNew(t *testing.T) {
	// Make sure inlineformsets respect commit=False, regression for #10750.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// Exclude some required field from the forms.
		childFormSet := forms.NewInlineFormSet[School, Child](db, "", forms.WithExclude("father", "mother"))
		school := create(t, db, &School{Name: "test"})
		mother := create(t, db, &Parent{Name: "mother"})
		father := create(t, db, &Parent{Name: "father"})
		data := postData(
			"child_set-TOTAL_FORMS", "1",
			"child_set-INITIAL_FORMS", "0",
			"child_set-MAX_NUM_FORMS", "0",
			"child_set-0-name", "child",
		)
		formset := childFormSet.WithInstance(school).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		objects, err := formset.SaveCommit(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		for _, obj := range objects {
			obj.Mother = fk[Parent](mother.ID)
			obj.Father = fk[Parent](father.ID)
			if err := orm.Query[Child](db).Save(context.Background(), obj); err != nil {
				t.Fatal(err)
			}
		}
		djangotest.AssertEqual(t, count(t, orm.Query[Child](db).Filter("school", school.ID)), int64(1))
	})
}

// Ported from Django: tests/inline_formsets/tests.py::DeletionTests.test_add_form_uuid_pk
func TestAddFormUUIDPK(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no db_default; Django: ParentUUIDPk.uuid has db_default=UUID4()")
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_inline_formset_factory
func TestInlineFormsetFactory(t *testing.T) {
	// These should both work without a problem.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		djangotest.AssertEqual(t, forms.NewInlineFormSet[Parent, Child](db, "mother", forms.WithFields(forms.AllFields)).Err(), nil)
		djangotest.AssertEqual(t, forms.NewInlineFormSet[Parent, Child](db, "father", forms.WithFields(forms.AllFields)).Err(), nil)
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_exception_on_unspecified_foreign_key
func TestExceptionOnUnspecifiedForeignKey(t *testing.T) {
	// Child has two ForeignKeys to Parent, so if we don't specify which one
	// to use for the inline formset, we should get an exception.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		msg := "'inline_formsets.Child' has more than one ForeignKey to 'inline_formsets.Parent'."
		assertErrContains(t, forms.NewInlineFormSet[Parent, Child](db, "").Err(), msg)
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_fk_name_not_foreign_key_field_from_child
func TestFKNameNotForeignKeyFieldFromChild(t *testing.T) {
	// If we specify fk_name, but it isn't a ForeignKey from the child model
	// to the parent model, we should get an exception.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		msg := "fk_name 'school' is not a ForeignKey to 'inline_formsets.Parent'."
		assertErrContains(t, forms.NewInlineFormSet[Parent, Child](db, "school").Err(), msg)
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_non_foreign_key_field
func TestNonForeignKeyField(t *testing.T) {
	// If the field specified in fk_name is not a ForeignKey, we should get an
	// exception.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		msg := "'inline_formsets.Child' has no field named 'test'."
		assertErrContains(t, forms.NewInlineFormSet[Parent, Child](db, "test").Err(), msg)
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_any_iterable_allowed_as_argument_to_exclude
func TestAnyIterableAllowedAsArgumentToExclude(t *testing.T) {
	// Regression test for #9171. Go takes the names as one list, which
	// covers Django's list and tuple.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		names := []string{"school"}
		djangotest.AssertEqual(t, forms.NewInlineFormSet[Parent, Child](db, "mother", forms.WithExclude("school")).Err(), nil)
		djangotest.AssertEqual(t, forms.NewInlineFormSet[Parent, Child](db, "mother", forms.WithExclude(names...)).Err(), nil)
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_zero_primary_key
func TestZeroPrimaryKey(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a zero integer primary key means the row is not saved; " +
		"Django: a saved Poet can have id=0 where allows_auto_pk_0")
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_unsaved_fk_validate_unique
func TestUnsavedFKValidateUnique(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := &Poet{Name: "unsaved"}
		poemFormSet := forms.NewInlineFormSet[Poet, Poem](db, "", forms.WithFields("name"))
		data := postData(
			"poem_set-TOTAL_FORMS", "2",
			"poem_set-INITIAL_FORMS", "0",
			"poem_set-MAX_NUM_FORMS", "2",
			"poem_set-0-name", "Poem",
			"poem_set-1-name", "Poem",
		)
		formset := poemFormSet.WithInstance(poet).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(),
			[]string{"Please correct the duplicate data for name."})
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_fk_not_duplicated_in_form_fields
func TestFKNotDuplicatedInFormFields(t *testing.T) {
	// A foreign key name isn't duplicated in form._meta fields (#21332).
	// Go has no form._meta.fields; the field names of the forms show the
	// foreign key once, after the fields that the formset adds.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := create(t, db, &Poet{Name: "test"})
		create(t, db, &Poem{Poet: fk[Poet](poet.ID), Name: "first test poem"})
		create(t, db, &Poem{Poet: fk[Poet](poet.ID), Name: "second test poem"})
		create(t, db, &Poem{Poet: fk[Poet](poet.ID), Name: "third test poem"})
		poemFormSet := forms.NewInlineFormSet[Poet, Poem](db, "", forms.WithFields("name"))
		poemFormSet.Extra = 0
		formset := poemFormSet.WithInstance(poet)
		djangotest.AssertEqual(t, len(formset.Forms()), 3)
		for _, form := range formset.Forms() {
			djangotest.AssertEqual(t, form.FieldNames(), []string{"name", "id", "DELETE", "poet"})
		}
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetFactoryTest.test_fk_in_all_formset_forms
func TestFKInAllFormsetForms(t *testing.T) {
	// A foreign key field is in Meta for all forms in the formset (#26538).
	// Django's PoemModelForm asserts it when the form is built; here every
	// form of the formset has the field.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := create(t, db, &Poet{Name: "test"})
		create(t, db, &Poem{Poet: fk[Poet](poet.ID), Name: "first test poem"})
		create(t, db, &Poem{Poet: fk[Poet](poet.ID), Name: "second test poem"})
		poemFormSet := forms.NewInlineFormSet[Poet, Poem](db, "", forms.WithFields("name"))
		poemFormSet.Extra = 0
		formset := poemFormSet.WithInstance(poet)
		// Trigger form instantiation to run the assert below.
		for _, form := range formset.Forms() {
			if _, ok := form.FieldDef("poet"); !ok {
				t.Errorf("a form has no field poet: %v", form.FieldNames())
			}
		}
	})
}

// Ported from Django: tests/inline_formsets/tests.py::InlineFormsetConstraintsValidationTests.test_constraint_refs_inline_foreignkey_field
func TestConstraintRefsInlineForeignkeyField(t *testing.T) {
	// Constraints that reference an InlineForeignKeyField should not be
	// skipped from validation (#35676).
	eachDB(t, func(t *testing.T, db *orm.DB) {
		childFormSet := forms.NewInlineFormSet[Parent, Child](db, "mother", forms.WithFields(forms.AllFields))
		childFormSet.Extra = 1
		father := create(t, db, &Parent{Name: "James"})
		school := create(t, db, &School{Name: "Hogwarts"})
		mother := create(t, db, &Parent{Name: "Lily"})
		create(t, db, &Child{
			Name: "Harry", Father: fk[Parent](father.ID), Mother: fk[Parent](mother.ID), School: fk[School](school.ID),
		})
		data := postData(
			"mothers_children-TOTAL_FORMS", "1",
			"mothers_children-INITIAL_FORMS", "0",
			"mothers_children-MIN_NUM_FORMS", "0",
			"mothers_children-MAX_NUM_FORMS", "1000",
			"mothers_children-0-id", "",
			"mothers_children-0-father", strconv.FormatInt(father.ID, 10),
			"mothers_children-0-school", strconv.FormatInt(school.ID, 10),
			"mothers_children-0-name", "Mary",
		)
		formset := childFormSet.WithInstance(mother).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		errs := make([]map[string][]string, 0, len(formset.Errors()))
		for _, e := range formset.Errors() {
			errs = append(errs, e.Messages())
		}
		djangotest.AssertEqual(t, errs, []map[string][]string{
			{"__all__": {"Constraint “unique_parents” is violated."}},
		})
	})
}
