package modelformsets_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// poetFormSet is Django's modelformset_factory(Poet, fields="__all__",
// can_delete=True).
func poetFormSet(db *orm.DB) *forms.ModelFormSet[Poet] {
	fs := forms.NewModelFormSet[Poet](db)
	fs.CanDelete = true
	return fs
}

// Ported from Django: tests/model_formsets/tests.py::DeletionTests.test_deletion
func TestDeletion(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		poet := create(t, db, &Poet{Name: "test"})
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "0",
			"form-0-id", fmt.Sprint(poet.ID),
			"form-0-name", "test",
			"form-0-DELETE", "on",
		)
		fs := poetFormSet(db)
		fs.QuerySet = orm.Query[Poet](db)
		formset := fs.Bind(data)
		if _, err := formset.SaveCommit(ctx, false); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count[Poet](t, db), int64(1))

		if _, err := formset.Save(ctx); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, formset.IsValid(), true)
		djangotest.AssertEqual(t, count[Poet](t, db), int64(0))
	})
}

// Ported from Django: tests/model_formsets/tests.py::DeletionTests.test_add_form_deletion_when_invalid
func TestAddFormDeletionWhenInvalid(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := create(t, db, &Poet{Name: "test"})
		// One existing untouched and two new unvalid forms
		data := postData(
			"form-TOTAL_FORMS", "3",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "0",
			"form-0-id", fmt.Sprint(poet.ID),
			"form-0-name", "test",
			"form-1-id", "",
			"form-1-name", strings.Repeat("x", 1000), // Too long
			"form-2-id", fmt.Sprint(poet.ID), // Violate unique constraint
			"form-2-name", "test2",
		)
		fs := poetFormSet(db)
		fs.QuerySet = orm.Query[Poet](db)
		formset := fs.Bind(data)
		// Make sure this form doesn't pass validation.
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, count[Poet](t, db), int64(1))

		// Then make sure that it *does* pass validation and delete the object,
		// even though the data in new forms aren't actually valid.
		data.Set("form-0-DELETE", "on")
		data.Set("form-1-DELETE", "on")
		data.Set("form-2-DELETE", "on")
		formset = fs.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count[Poet](t, db), int64(0))
	})
}

// Ported from Django: tests/model_formsets/tests.py::DeletionTests.test_change_form_deletion_when_invalid
func TestChangeFormDeletionWhenInvalid(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poet := create(t, db, &Poet{Name: "test"})
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "0",
			"form-0-id", fmt.Sprint(poet.ID),
			"form-0-name", strings.Repeat("x", 1000),
		)
		fs := poetFormSet(db)
		fs.QuerySet = orm.Query[Poet](db)
		formset := fs.Bind(data)
		// Make sure this form doesn't pass validation.
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, count[Poet](t, db), int64(1))

		// Then make sure that it *does* pass validation and delete the object,
		// even though the data isn't actually valid.
		data.Set("form-0-DELETE", "on")
		formset = fs.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count[Poet](t, db), int64(0))
	})
}

// Ported from Django: tests/model_formsets/tests.py::DeletionTests.test_outdated_deletion
func TestOutdatedDeletion(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		poet := create(t, db, &Poet{Name: "test"})
		poem := create(t, db, &Poem{Name: "Brevity is the soul of wit", Poet: fk[Poet](poet.ID)})

		fs := forms.NewInlineFormSet[Poet, Poem](db, "")
		fs.CanDelete = true

		// Simulate deletion of an object that doesn't exist in the database
		data := postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "2",
			"form-0-id", fmt.Sprint(poem.ID),
			"form-0-name", "foo",
			"form-1-id", fmt.Sprint(poem.ID+1), // doesn't exist
			"form-1-name", "bar",
			"form-1-DELETE", "on",
		)
		inline := fs.WithInstance(poet)
		inline.Prefix = "form"
		formset := inline.Bind(data)

		// The formset is valid even though poem.pk + 1 doesn't exist,
		// because it's marked for deletion anyway
		djangotest.AssertEqual(t, formset.IsValid(), true)

		if _, err := formset.Save(ctx); err != nil {
			t.Fatal(err)
		}

		// Make sure the save went through correctly
		djangotest.AssertEqual(t, get[Poem](t, db, "pk", poem.ID).Name, "foo")
		n, err := orm.Query[Poem](db).Filter("poet", poet.ID).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, n, int64(1))
		exists, err := orm.Query[Poem](db).Filter("pk", poem.ID+1).Exists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, exists, false)
	})
}

// Ported from Django: tests/model_formsets/tests.py::DeletionTests.test_deletion_not_allowed_outside_formset_queryset
func TestDeletionNotAllowedOutsideFormsetQueryset(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fs := forms.NewInlineFormSet[Owner, OwnerProfile](db, "")
		fs.CanDelete = true
		place := create(t, db, &Place{Name: "Sal's Place", City: "Cairo"})
		owner := create(t, db, &Owner{Name: "Sal", Place: fk[Place](place.ID)})
		profile := create(t, db, &OwnerProfile{Owner: o2o[Owner](owner.AutoID), Age: 42})
		limitedQS := orm.Query[OwnerProfile](db).Exclude("pk", profile.Owner.PK())
		data := postData(
			"ownerprofile-TOTAL_FORMS", "1",
			"ownerprofile-INITIAL_FORMS", "1",
			"ownerprofile-MAX_NUM_FORMS", "0",
			"ownerprofile-0-owner", fmt.Sprint(profile.Owner.PK()), // Not in `limited_qs`.
			"ownerprofile-0-age", fmt.Sprint(profile.Age),
			"ownerprofile-0-DELETE", "on",
		)
		inline := fs.WithInstance(owner)
		inline.QuerySet = limitedQS
		formset := inline.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(ctx); err != nil {
			t.Fatal(err)
		}
		// The deletion is not allowed.
		exists, err := orm.Query[OwnerProfile](db).Filter("pk", profile.Owner.PK()).Exists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, exists, true)
	})
}

// Ported from Django: tests/model_formsets/tests.py::DeletionTests.test_deletion_not_allowed_outside_formset_queryset_editable_pk
func TestDeletionNotAllowedOutsideFormsetQuerysetEditablePK(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		fs := forms.NewModelFormSet[CustomPrimaryKey](db)
		fs.CanDelete = true
		obj := create(t, db, &CustomPrimaryKey{MyPK: "pk", SomeField: "data"})
		fs.QuerySet = orm.Query[CustomPrimaryKey](db).Exclude("pk", obj.MyPK)
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "0",
			"form-0-my_pk", obj.MyPK, // Not in `limited_qs`.
			"form-0-some_field", obj.SomeField,
			"form-0-DELETE", "on",
		)
		formset := fs.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(ctx); err != nil {
			t.Fatal(err)
		}
		// The deletion is not allowed.
		exists, err := orm.Query[CustomPrimaryKey](db).Filter("pk", obj.MyPK).Exists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, exists, true)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_modelformset_factory_without_fields
func TestModelFormsetFactoryWithoutFields(t *testing.T) {
	t.Skip("FIXME-PORT: Go: NewModelForm without WithFields takes every field by design, " +
		"Django: modelformset_factory without fields or exclude raises ImproperlyConfigured")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_simple_save
func TestSimpleSave(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		qs := orm.Query[Author](db)
		authorFormSet := forms.NewModelFormSet[Author](db)
		authorFormSet.Extra = 3
		authorFormSet.QuerySet = qs

		formset := authorFormSet
		djangotest.AssertEqual(t, len(formset.Forms()), 3)
		for i := range 3 {
			djangotest.AssertHTMLEqual(t, formset.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_form-%[1]d-name">Name:</label>`+
					`<input id="id_form-%[1]d-name" type="text" name="form-%[1]d-name" maxlength="100">`+
					`<input type="hidden" name="form-%[1]d-id" id="id_form-%[1]d-id"></p>`, i))
		}

		data := postData(
			"form-TOTAL_FORMS", "3", // the number of forms rendered
			"form-INITIAL_FORMS", "0", // the number of forms with initial data
			"form-MAX_NUM_FORMS", "", // the max number of forms
			"form-0-name", "Charles Baudelaire",
			"form-1-name", "Arthur Rimbaud",
			"form-2-name", "",
		)

		formset = authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		saved, err := formset.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 2)
		author1, author2 := *saved[0], *saved[1]
		djangotest.AssertEqual(t, author1, get[Author](t, db, "name", "Charles Baudelaire"))
		djangotest.AssertEqual(t, author2, get[Author](t, db, "name", "Arthur Rimbaud"))

		authors := all(t, orm.Query[Author](db).OrderBy("name"))
		djangotest.AssertEqual(t, authors, []Author{author2, author1})

		// Gah! We forgot Paul Verlaine. Create a formset to edit the
		// existing authors with an extra form to add him. We *could* pass in a
		// queryset to restrict the Author objects we edit, but in this case
		// we'll use it to display them in alphabetical order by name.

		qs = orm.Query[Author](db).OrderBy("name")
		authorFormSet = forms.NewModelFormSet[Author](db)
		authorFormSet.Extra, authorFormSet.CanDelete = 1, false
		authorFormSet.QuerySet = qs

		formset = authorFormSet
		djangotest.AssertEqual(t, len(formset.Forms()), 3)
		djangotest.AssertHTMLEqual(t, formset.Forms()[0].AsP(),
			`<p><label for="id_form-0-name">Name:</label>`+
				`<input id="id_form-0-name" type="text" name="form-0-name" `+
				`value="Arthur Rimbaud" maxlength="100">`+
				fmt.Sprintf(`<input type="hidden" name="form-0-id" value="%d" id="id_form-0-id"></p>`, author2.ID))
		djangotest.AssertHTMLEqual(t, formset.Forms()[1].AsP(),
			`<p><label for="id_form-1-name">Name:</label>`+
				`<input id="id_form-1-name" type="text" name="form-1-name" `+
				`value="Charles Baudelaire" maxlength="100">`+
				fmt.Sprintf(`<input type="hidden" name="form-1-id" value="%d" id="id_form-1-id"></p>`, author1.ID))
		djangotest.AssertHTMLEqual(t, formset.Forms()[2].AsP(),
			`<p><label for="id_form-2-name">Name:</label>`+
				`<input id="id_form-2-name" type="text" name="form-2-name" maxlength="100">`+
				`<input type="hidden" name="form-2-id" id="id_form-2-id"></p>`)

		data = postData(
			"form-TOTAL_FORMS", "3", // the number of forms rendered
			"form-INITIAL_FORMS", "2", // the number of forms with initial data
			"form-MAX_NUM_FORMS", "", // the max number of forms
			"form-0-id", fmt.Sprint(author2.ID),
			"form-0-name", "Arthur Rimbaud",
			"form-1-id", fmt.Sprint(author1.ID),
			"form-1-name", "Charles Baudelaire",
			"form-2-name", "Paul Verlaine",
		)

		formset = authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		// Only changed or new objects are returned from formset.save()
		saved, err = formset.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		author3 := *saved[0]
		djangotest.AssertEqual(t, author3, get[Author](t, db, "name", "Paul Verlaine"))

		authors = all(t, orm.Query[Author](db).OrderBy("name"))
		djangotest.AssertEqual(t, authors, []Author{author2, author1, author3})

		// This probably shouldn't happen, but it will. If an add form was
		// marked for deletion, make sure we don't save that form.

		qs = orm.Query[Author](db).OrderBy("name")
		authorFormSet = forms.NewModelFormSet[Author](db)
		authorFormSet.Extra, authorFormSet.CanDelete = 1, true
		authorFormSet.QuerySet = qs

		formset = authorFormSet
		djangotest.AssertEqual(t, len(formset.Forms()), 4)
		for i, a := range []Author{author2, author1, author3} {
			djangotest.AssertHTMLEqual(t, formset.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_form-%[1]d-name">Name:</label>`+
					`<input id="id_form-%[1]d-name" type="text" name="form-%[1]d-name" `+
					`value="%[2]s" maxlength="100"></p>`+
					`<p><label for="id_form-%[1]d-DELETE">Delete:</label>`+
					`<input type="checkbox" name="form-%[1]d-DELETE" id="id_form-%[1]d-DELETE">`+
					`<input type="hidden" name="form-%[1]d-id" value="%[3]d" id="id_form-%[1]d-id"></p>`,
				i, a.Name, a.ID))
		}
		djangotest.AssertHTMLEqual(t, formset.Forms()[3].AsP(),
			`<p><label for="id_form-3-name">Name:</label>`+
				`<input id="id_form-3-name" type="text" name="form-3-name" maxlength="100">`+
				`</p><p><label for="id_form-3-DELETE">Delete:</label>`+
				`<input type="checkbox" name="form-3-DELETE" id="id_form-3-DELETE">`+
				`<input type="hidden" name="form-3-id" id="id_form-3-id"></p>`)

		data = postData(
			"form-TOTAL_FORMS", "4", // the number of forms rendered
			"form-INITIAL_FORMS", "3", // the number of forms with initial data
			"form-MAX_NUM_FORMS", "", // the max number of forms
			"form-0-id", fmt.Sprint(author2.ID),
			"form-0-name", "Arthur Rimbaud",
			"form-1-id", fmt.Sprint(author1.ID),
			"form-1-name", "Charles Baudelaire",
			"form-2-id", fmt.Sprint(author3.ID),
			"form-2-name", "Paul Verlaine",
			"form-3-name", "Walt Whitman",
			"form-3-DELETE", "on",
		)

		formset = authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		// No objects were changed or saved so nothing will come back.
		saved, err = formset.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, saved, []*Author{})

		authors = all(t, orm.Query[Author](db).OrderBy("name"))
		djangotest.AssertEqual(t, authors, []Author{author2, author1, author3})

		// Edit a record to ensure save only returns that one record.

		data = postData(
			"form-TOTAL_FORMS", "4", // the number of forms rendered
			"form-INITIAL_FORMS", "3", // the number of forms with initial data
			"form-MAX_NUM_FORMS", "", // the max number of forms
			"form-0-id", fmt.Sprint(author2.ID),
			"form-0-name", "Walt Whitman",
			"form-1-id", fmt.Sprint(author1.ID),
			"form-1-name", "Charles Baudelaire",
			"form-2-id", fmt.Sprint(author3.ID),
			"form-2-name", "Paul Verlaine",
			"form-3-name", "",
			"form-3-DELETE", "",
		)

		formset = authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		// One record has changed.
		saved, err = formset.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		djangotest.AssertEqual(t, *saved[0], get[Author](t, db, "name", "Walt Whitman"))
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_commit_false
func TestCommitFalse(t *testing.T) {
	// Test the behavior of commit=False and save_m2m
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		author1 := create(t, db, &Author{Name: "Charles Baudelaire"})
		author2 := create(t, db, &Author{Name: "Paul Verlaine"})
		author3 := create(t, db, &Author{Name: "Walt Whitman"})

		meeting := create(t, db, &AuthorMeeting{Created: orm.DateFromTime(time.Now())})
		authors := orm.Many[Author](db, meeting, "Authors")
		if err := authors.Set(ctx, []*Author{author1, author2, author3}); err != nil {
			t.Fatal(err)
		}

		// create an Author instance to add to the meeting.

		author4 := create(t, db, &Author{Name: "John Steinbeck"})

		fs := forms.NewModelFormSet[AuthorMeeting](db)
		fs.Extra, fs.CanDelete = 1, true
		fs.QuerySet = orm.Query[AuthorMeeting](db)
		data := postData(
			"form-TOTAL_FORMS", "2", // the number of forms rendered
			"form-INITIAL_FORMS", "1", // the number of forms with initial data
			"form-MAX_NUM_FORMS", "", // the max number of forms
			"form-0-id", fmt.Sprint(meeting.ID),
			"form-0-name", "2nd Tuesday of the Week Meeting",
			"form-1-name", "",
			// Django posts "form-1-authors": "" in a plain dict, which its
			// SelectMultiple reads as no selection; url.Values has no such value.
			"form-1-DELETE", "",
		)
		data["form-0-authors"] = []string{
			fmt.Sprint(author2.ID), fmt.Sprint(author1.ID), fmt.Sprint(author3.ID), fmt.Sprint(author4.ID),
		}
		formset := fs.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		instances, err := formset.SaveCommit(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, instance := range instances {
			instance.Created = orm.DateFromTime(time.Now())
			if err := orm.Query[AuthorMeeting](db).Save(ctx, instance); err != nil {
				t.Fatal(err)
			}
		}
		if err := formset.SaveM2M(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := orm.Many[Author](db, instances[0], "Authors").All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, got, []Author{*author1, *author4, *author2, *author3})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_max_num
func TestMaxNum(t *testing.T) {
	// Test the behavior of max_num with model formsets. It should allow
	// all existing related objects/inlines for a given object to be
	// displayed, but not allow the creation of new inlines beyond max_num.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		a1 := create(t, db, &Author{Name: "Charles Baudelaire"})
		a2 := create(t, db, &Author{Name: "Paul Verlaine"})
		a3 := create(t, db, &Author{Name: "Walt Whitman"})

		qs := orm.Query[Author](db).OrderBy("name")
		formSet := func(maxNum, extra int) *forms.ModelFormSet[Author] {
			fs := forms.NewModelFormSet[Author](db)
			fs.MaxNum, fs.Extra, fs.QuerySet = maxNum, extra, qs
			return fs
		}

		formset := formSet(forms.DefaultMaxNum, 3)
		djangotest.AssertEqual(t, len(formset.Forms()), 6)
		djangotest.AssertEqual(t, len(formset.ExtraForms()), 3)

		formset = formSet(4, 3)
		djangotest.AssertEqual(t, len(formset.Forms()), 4)
		djangotest.AssertEqual(t, len(formset.ExtraForms()), 1)

		formset = formSet(0, 3)
		djangotest.AssertEqual(t, len(formset.Forms()), 3)
		djangotest.AssertEqual(t, len(formset.ExtraForms()), 0)

		for _, maxNum := range []int{forms.DefaultMaxNum, 0, 4} {
			formset = formSet(maxNum, 1)
			djangotest.AssertEqual(t, all(t, formset.GetQuerySet()), []Author{*a1, *a2, *a3})
		}
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_min_num
func TestMinNum(t *testing.T) {
	// Test the behavior of min_num with model formsets. It should be
	// added to extra.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := func(minNum, extra int) *forms.ModelFormSet[Author] {
			fs := forms.NewModelFormSet[Author](db)
			fs.MinNum, fs.Extra = minNum, extra
			fs.QuerySet = orm.Query[Author](db).None()
			return fs
		}
		djangotest.AssertEqual(t, len(formSet(0, 0).Forms()), 0)
		djangotest.AssertEqual(t, len(formSet(1, 0).Forms()), 1)
		djangotest.AssertEqual(t, len(formSet(1, 1).Forms()), 2)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_min_num_with_existing
func TestMinNumWithExisting(t *testing.T) {
	// Test the behavior of min_num with existing objects.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		create(t, db, &Author{Name: "Charles Baudelaire"})
		fs := forms.NewModelFormSet[Author](db)
		fs.Extra, fs.MinNum = 0, 1
		fs.QuerySet = orm.Query[Author](db)
		djangotest.AssertEqual(t, len(fs.Forms()), 1)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_custom_save_method
//
// Django overrides the save() of the form; a Go form has no method to
// override, so the formset's SaveNew does the same.
func TestCustomSaveMethod(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		fs := forms.NewModelFormSet[Poet](db)
		fs.SaveNew = func(ctx context.Context, form *forms.ModelForm[Poet], commit bool) (*Poet, error) {
			// change the name to "Vladimir Mayakovsky" just to be a jerk.
			author := form.Instance()
			author.Name = "Vladimir Mayakovsky"
			if commit {
				if err := orm.Query[Poet](db).Save(ctx, author); err != nil {
					return nil, err
				}
			}
			return author, nil
		}

		data := postData(
			"form-TOTAL_FORMS", "3", // the number of forms rendered
			"form-INITIAL_FORMS", "0", // the number of forms with initial data
			"form-MAX_NUM_FORMS", "", // the max number of forms
			"form-0-name", "Walt Whitman",
			"form-1-name", "Charles Baudelaire",
			"form-2-name", "",
		)

		fs.QuerySet = orm.Query[Poet](db)
		formset := fs.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		poets, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(poets), 2)
		djangotest.AssertEqual(t, poets[0].Name, "Vladimir Mayakovsky")
		djangotest.AssertEqual(t, poets[1].Name, "Vladimir Mayakovsky")
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_custom_form
//
// model_formset_factory() respects fields and exclude parameters of a
// custom form.
func TestCustomForm(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		postFormSet := forms.NewModelFormSet[Post](db, forms.WithFields("title", "posted"))
		formset := postFormSet
		if _, ok := formset.Forms()[0].FieldDef("subtitle"); ok {
			t.Error("subtitle is in the form of fields (title, posted)")
		}

		postFormSet = forms.NewModelFormSet[Post](db, forms.WithExclude("subtitle"))
		formset = postFormSet
		if _, ok := formset.Forms()[0].FieldDef("subtitle"); ok {
			t.Error("subtitle is in the form that excludes it")
		}
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_custom_queryset_init
//
// A queryset can be overridden in the formset's __init__() method.
func TestCustomQuerysetInit(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		create(t, db, &Author{Name: "Charles Baudelaire"})
		create(t, db, &Author{Name: "Paul Verlaine"})

		formset := forms.NewModelFormSet[Author](db)
		formset.QuerySet = orm.Query[Author](db).Filter("name__startswith", "Charles")
		djangotest.AssertEqual(t, len(all(t, formset.GetQuerySet())), 1)
	})
}

// No Django test exists: Django passes an evaluated queryset as the queryset
// argument, and Objects is the Go form of that. The formset builds its forms
// from the given rows in their order and runs no query.
func TestObjectsReplaceTheQuerySet(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		a := &Author{Name: "Charles Baudelaire"}
		b := &Author{Name: "Paul Verlaine"}
		create(t, db, a)
		create(t, db, b)

		formset := forms.NewModelFormSet[Author](db, forms.WithFields("name"))
		formset.Extra = 0
		formset.Objects = []*Author{b, a}
		djangotest.AssertNumQueries(t, db, 0, func() {
			mfs := formset.ModelForms()
			djangotest.AssertEqual(t, len(mfs), 2)
			djangotest.AssertEqual(t, mfs[0].Instance() == b, true)
			djangotest.AssertEqual(t, mfs[1].Instance() == a, true)
			if html := mfs[0].Form.Field("id").HTML(); !strings.Contains(html, fmt.Sprintf(`value="%d"`, b.ID)) {
				t.Errorf("the first form edits %q, want the row of %q", html, b.Name)
			}
		})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_model_inheritance
func TestModelInheritance(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance, " +
		"Django: a formset of BetterAuthor(Author) with the hidden author_ptr")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets
func TestInlineFormsets(t *testing.T) {
	// We can also create a formset that is tied to a parent model. This is
	// how the admin system's edit inline functionality works.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		authorBooksFormSet := forms.NewInlineFormSet[Author, Book](db, "")
		authorBooksFormSet.CanDelete, authorBooksFormSet.Extra = false, 3
		author := create(t, db, &Author{Name: "Charles Baudelaire"})

		formset := authorBooksFormSet.WithInstance(author)
		djangotest.AssertEqual(t, len(formset.Forms()), 3)
		for i := range 3 {
			djangotest.AssertHTMLEqual(t, formset.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_book_set-%[1]d-title">Title:</label>`+
					`<input id="id_book_set-%[1]d-title" type="text" name="book_set-%[1]d-title" `+
					`maxlength="100">`+
					`<input type="hidden" name="book_set-%[1]d-author" value="%[2]d" `+
					`id="id_book_set-%[1]d-author">`+
					`<input type="hidden" name="book_set-%[1]d-id" id="id_book_set-%[1]d-id">`+
					`</p>`, i, author.ID))
		}

		data := postData(
			"book_set-TOTAL_FORMS", "3", // the number of forms rendered
			"book_set-INITIAL_FORMS", "0", // the number of forms with initial data
			"book_set-MAX_NUM_FORMS", "", // the max number of forms
			"book_set-0-title", "Les Fleurs du Mal",
			"book_set-1-title", "",
			"book_set-2-title", "",
		)

		formset = authorBooksFormSet.WithInstance(author).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		saved, err := formset.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		book1 := saved[0]
		djangotest.AssertEqual(t, book1.ID, get[Book](t, db, "title", "Les Fleurs du Mal").ID)
		books := all(t, orm.Query[Book](db).Filter("author", author.ID))
		djangotest.AssertEqual(t, len(books), 1)
		djangotest.AssertEqual(t, books[0].ID, book1.ID)

		// Now that we've added a book to Charles Baudelaire, try adding
		// another one. This time though, an edit form will be available for
		// every existing book.

		authorBooksFormSet = forms.NewInlineFormSet[Author, Book](db, "")
		authorBooksFormSet.CanDelete, authorBooksFormSet.Extra = false, 2
		reloaded := get[Author](t, db, "name", "Charles Baudelaire")
		author = &reloaded

		formset = authorBooksFormSet.WithInstance(author)
		djangotest.AssertEqual(t, len(formset.Forms()), 3)
		djangotest.AssertHTMLEqual(t, formset.Forms()[0].AsP(),
			`<p><label for="id_book_set-0-title">Title:</label>`+
				`<input id="id_book_set-0-title" type="text" name="book_set-0-title" `+
				`value="Les Fleurs du Mal" maxlength="100">`+
				fmt.Sprintf(`<input type="hidden" name="book_set-0-author" value="%d" `, author.ID)+
				`id="id_book_set-0-author">`+
				fmt.Sprintf(`<input type="hidden" name="book_set-0-id" value="%d" `, book1.ID)+
				`id="id_book_set-0-id"></p>`)
		for i := 1; i < 3; i++ {
			djangotest.AssertHTMLEqual(t, formset.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_book_set-%[1]d-title">Title:</label>`+
					`<input id="id_book_set-%[1]d-title" type="text" name="book_set-%[1]d-title" `+
					`maxlength="100">`+
					`<input type="hidden" name="book_set-%[1]d-author" value="%[2]d" `+
					`id="id_book_set-%[1]d-author">`+
					`<input type="hidden" name="book_set-%[1]d-id" id="id_book_set-%[1]d-id"></p>`,
				i, author.ID))
		}

		data = postData(
			"book_set-TOTAL_FORMS", "3", // the number of forms rendered
			"book_set-INITIAL_FORMS", "1", // the number of forms with initial data
			"book_set-MAX_NUM_FORMS", "", // the max number of forms
			"book_set-0-id", fmt.Sprint(book1.ID),
			"book_set-0-title", "Les Fleurs du Mal",
			"book_set-1-title", "Les Paradis Artificiels",
			"book_set-2-title", "",
		)

		formset = authorBooksFormSet.WithInstance(author).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		saved, err = formset.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		book2 := saved[0]
		djangotest.AssertEqual(t, book2.ID, get[Book](t, db, "title", "Les Paradis Artificiels").ID)

		// As you can see, 'Les Paradis Artificiels' is now a book belonging to
		// Charles Baudelaire.
		books = all(t, orm.Query[Book](db).Filter("author", author.ID).OrderBy("title"))
		djangotest.AssertEqual(t, len(books), 2)
		djangotest.AssertEqual(t, []int64{books[0].ID, books[1].ID}, []int64{book1.ID, book2.ID})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets_save_as_new
func TestInlineFormsetsSaveAsNew(t *testing.T) {
	// The save_as_new parameter lets you re-associate the data to a new
	// instance. This is used in the admin for save_as functionality.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorBooksFormSet := forms.NewInlineFormSet[Author, Book](db, "")
		authorBooksFormSet.CanDelete, authorBooksFormSet.Extra = false, 2
		create(t, db, &Author{Name: "Charles Baudelaire"})

		data := postData(
			"book_set-TOTAL_FORMS", "3", // the number of forms rendered
			"book_set-INITIAL_FORMS", "2", // the number of forms with initial data
			"book_set-MAX_NUM_FORMS", "", // the max number of forms
			"book_set-0-id", "1",
			"book_set-0-title", "Les Fleurs du Mal",
			"book_set-1-id", "2",
			"book_set-1-title", "Les Paradis Artificiels",
			"book_set-2-title", "",
		)
		// Django passes an immutable QueryDict; here the data must come back
		// unchanged.
		before := fmt.Sprint(data)

		withNew := authorBooksFormSet.WithInstance(&Author{})
		withNew.SaveAsNew = true
		formset := withNew.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		djangotest.AssertEqual(t, fmt.Sprint(data), before)

		newAuthor := create(t, db, &Author{Name: "Charles Baudelaire"})
		withNew = authorBooksFormSet.WithInstance(newAuthor)
		withNew.SaveAsNew = true
		formset = withNew.Bind(data)
		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 2)
		djangotest.AssertEqual(t, saved[0].Title, "Les Fleurs du Mal")
		djangotest.AssertEqual(t, saved[1].Title, "Les Paradis Artificiels")

		// Test using a custom prefix on an inline formset.

		prefixed := forms.NewInlineFormSet[Author, Book](db, "")
		prefixed.CanDelete, prefixed.Extra = false, 2
		prefixed.Prefix = "test"
		djangotest.AssertEqual(t, len(prefixed.Forms()), 2)
		for i := range 2 {
			djangotest.AssertHTMLEqual(t, prefixed.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_test-%[1]d-title">Title:</label>`+
					`<input id="id_test-%[1]d-title" type="text" name="test-%[1]d-title" `+
					`maxlength="100">`+
					`<input type="hidden" name="test-%[1]d-author" id="id_test-%[1]d-author">`+
					`<input type="hidden" name="test-%[1]d-id" id="id_test-%[1]d-id"></p>`, i))
		}
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets_with_custom_pk
func TestInlineFormsetsWithCustomPK(t *testing.T) {
	// Test inline formsets where the inline-edited object has a custom
	// primary key that is not the fk to the parent object.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorBooksFormSet2 := forms.NewInlineFormSet[Author, BookWithCustomPK](db, "")
		authorBooksFormSet2.CanDelete, authorBooksFormSet2.Extra = false, 1
		author := create(t, db, &Author{Name: "Charles Baudelaire"})

		formset := authorBooksFormSet2.WithInstance(author)
		djangotest.AssertEqual(t, len(formset.Forms()), 1)
		djangotest.AssertHTMLEqual(t, formset.Forms()[0].AsP(),
			`<p><label for="id_bookwithcustompk_set-0-my_pk">My pk:</label>`+
				`<input id="id_bookwithcustompk_set-0-my_pk" type="number" `+
				`name="bookwithcustompk_set-0-my_pk" step="1"></p>`+
				`<p><label for="id_bookwithcustompk_set-0-title">Title:</label>`+
				`<input id="id_bookwithcustompk_set-0-title" type="text" `+
				`name="bookwithcustompk_set-0-title" maxlength="100">`+
				`<input type="hidden" name="bookwithcustompk_set-0-author" `+
				fmt.Sprintf(`value="%d" id="id_bookwithcustompk_set-0-author"></p>`, author.ID))

		data := postData(
			// The number of forms rendered.
			"bookwithcustompk_set-TOTAL_FORMS", "1",
			// The number of forms with initial data.
			"bookwithcustompk_set-INITIAL_FORMS", "0",
			// The max number of forms.
			"bookwithcustompk_set-MAX_NUM_FORMS", "",
			"bookwithcustompk_set-0-my_pk", "77777",
			"bookwithcustompk_set-0-title", "Les Fleurs du Mal",
		)

		formset = authorBooksFormSet2.WithInstance(author).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		djangotest.AssertEqual(t, saved[0].MyPK.String(), "77777")

		book1 := get[BookWithCustomPK](t, db, "author", author.ID)
		djangotest.AssertEqual(t, book1.Title, "Les Fleurs du Mal")
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets_with_multi_table_inheritance
func TestInlineFormsetsWithMultiTableInheritance(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance, " +
		"Django: an inline formset of AlternateBook(Book) with the hidden book_ptr")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets_with_nullable_unique_together
func TestInlineFormsetsWithNullableUniqueTogether(t *testing.T) {
	// Test inline formsets where the inline-edited object has a
	// unique_together constraint with a nullable member
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorBooksFormSet4 := forms.NewInlineFormSet[Author, BookWithOptionalAltEditor](db, "")
		authorBooksFormSet4.CanDelete, authorBooksFormSet4.Extra = false, 2
		author := create(t, db, &Author{Name: "Charles Baudelaire"})

		data := postData(
			// The number of forms rendered.
			"bookwithoptionalalteditor_set-TOTAL_FORMS", "2",
			// The number of forms with initial data.
			"bookwithoptionalalteditor_set-INITIAL_FORMS", "0",
			// The max number of forms.
			"bookwithoptionalalteditor_set-MAX_NUM_FORMS", "",
			"bookwithoptionalalteditor_set-0-author", fmt.Sprint(author.ID),
			"bookwithoptionalalteditor_set-0-title", "Les Fleurs du Mal",
			"bookwithoptionalalteditor_set-1-author", fmt.Sprint(author.ID),
			"bookwithoptionalalteditor_set-1-title", "Les Fleurs du Mal",
		)
		formset := authorBooksFormSet4.WithInstance(author).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 2)
		book1, book2 := saved[0], saved[1]
		djangotest.AssertEqual(t, book1.Author.PK(), author.ID)
		djangotest.AssertEqual(t, book1.Title, "Les Fleurs du Mal")
		djangotest.AssertEqual(t, book2.Author.PK(), author.ID)
		djangotest.AssertEqual(t, book2.Title, "Les Fleurs du Mal")
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets_with_custom_save_method
//
// Django overrides the save() of the form; the formset's SaveNew does the
// same in Go.
func TestInlineFormsetsWithCustomSaveMethod(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorBooksFormSet := forms.NewInlineFormSet[Author, Book](db, "")
		authorBooksFormSet.CanDelete, authorBooksFormSet.Extra = false, 2
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		book1 := create(t, db, &Book{Author: fk[Author](author.ID), Title: "Les Paradis Artificiels"})
		book2 := create(t, db, &Book{Author: fk[Author](author.ID), Title: "Les Fleurs du Mal"})
		book3 := create(t, db, &Book{Author: fk[Author](author.ID), Title: "Flowers of Evil"})

		poemFormSet := forms.NewInlineFormSet[Poet, Poem](db, "")
		poemFormSet.SaveNew = func(ctx context.Context, form *forms.ModelForm[Poem], commit bool) (*Poem, error) {
			// change the name to "Brooklyn Bridge" just to be a jerk.
			poem := form.Instance()
			poem.Name = "Brooklyn Bridge"
			if commit {
				if err := orm.Query[Poem](db).Save(ctx, poem); err != nil {
					return nil, err
				}
			}
			return poem, nil
		}

		data := postData(
			"poem_set-TOTAL_FORMS", "3", // the number of forms rendered
			"poem_set-INITIAL_FORMS", "0", // the number of forms with initial data
			"poem_set-MAX_NUM_FORMS", "", // the max number of forms
			"poem_set-0-name", "The Cloud in Trousers",
			"poem_set-1-name", "I",
			"poem_set-2-name", "",
		)

		poet := create(t, db, &Poet{Name: "Vladimir Mayakovsky"})
		formset := poemFormSet.WithInstance(poet).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 2)
		djangotest.AssertEqual(t, saved[0].Name, "Brooklyn Bridge")
		djangotest.AssertEqual(t, saved[1].Name, "Brooklyn Bridge")

		// We can provide a custom queryset to our InlineFormSet:

		customQS := orm.Query[Book](db).OrderBy("-title")
		books := authorBooksFormSet.WithInstance(author)
		books.QuerySet = customQS
		djangotest.AssertEqual(t, len(books.Forms()), 5)
		for i, b := range []*Book{book1, book2, book3} {
			djangotest.AssertHTMLEqual(t, books.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_book_set-%[1]d-title">Title:</label>`+
					`<input id="id_book_set-%[1]d-title" type="text" name="book_set-%[1]d-title" `+
					`value="%[2]s" maxlength="100">`+
					`<input type="hidden" name="book_set-%[1]d-author" `+
					`value="%[3]d" id="id_book_set-%[1]d-author">`+
					`<input type="hidden" name="book_set-%[1]d-id" `+
					`value="%[4]d" id="id_book_set-%[1]d-id">`+
					`</p>`, i, b.Title, author.ID, b.ID))
		}
		for i := 3; i < 5; i++ {
			djangotest.AssertHTMLEqual(t, books.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_book_set-%[1]d-title">Title:</label>`+
					`<input id="id_book_set-%[1]d-title" type="text" name="book_set-%[1]d-title" `+
					`maxlength="100">`+
					`<input type="hidden" name="book_set-%[1]d-author" `+
					`value="%[2]d" id="id_book_set-%[1]d-author">`+
					`<input type="hidden" name="book_set-%[1]d-id" id="id_book_set-%[1]d-id"></p>`,
				i, author.ID))
		}

		data = postData(
			"book_set-TOTAL_FORMS", "5", // the number of forms rendered
			"book_set-INITIAL_FORMS", "3", // the number of forms with initial data
			"book_set-MAX_NUM_FORMS", "", // the max number of forms
			"book_set-0-id", fmt.Sprint(book1.ID),
			"book_set-0-title", "Les Paradis Artificiels",
			"book_set-1-id", fmt.Sprint(book2.ID),
			"book_set-1-title", "Les Fleurs du Mal",
			"book_set-2-id", fmt.Sprint(book3.ID),
			"book_set-2-title", "Flowers of Evil",
			"book_set-3-title", "Revue des deux mondes",
			"book_set-4-title", "",
		)
		djangotest.AssertEqual(t, books.Bind(data).IsValid(), true)

		customQS = orm.Query[Book](db).Filter("title__startswith", "F")
		books = authorBooksFormSet.WithInstance(author)
		books.QuerySet = customQS
		djangotest.AssertHTMLEqual(t, books.Forms()[0].AsP(),
			`<p><label for="id_book_set-0-title">Title:</label>`+
				`<input id="id_book_set-0-title" type="text" name="book_set-0-title" `+
				`value="Flowers of Evil" maxlength="100">`+
				`<input type="hidden" name="book_set-0-author" `+
				fmt.Sprintf(`value="%d" id="id_book_set-0-author">`, author.ID)+
				`<input type="hidden" name="book_set-0-id" `+
				fmt.Sprintf(`value="%d" id="id_book_set-0-id"></p>`, book3.ID))
		for i := 1; i < 3; i++ {
			djangotest.AssertHTMLEqual(t, books.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_book_set-%[1]d-title">Title:</label>`+
					`<input id="id_book_set-%[1]d-title" type="text" name="book_set-%[1]d-title" `+
					`maxlength="100">`+
					`<input type="hidden" name="book_set-%[1]d-author" `+
					`value="%[2]d" id="id_book_set-%[1]d-author">`+
					`<input type="hidden" name="book_set-%[1]d-id" id="id_book_set-%[1]d-id"></p>`,
				i, author.ID))
		}

		data = postData(
			"book_set-TOTAL_FORMS", "3", // the number of forms rendered
			"book_set-INITIAL_FORMS", "1", // the number of forms with initial data
			"book_set-MAX_NUM_FORMS", "", // the max number of forms
			"book_set-0-id", fmt.Sprint(book3.ID),
			"book_set-0-title", "Flowers of Evil",
			"book_set-1-title", "Revue des deux mondes",
			"book_set-2-title", "",
		)
		djangotest.AssertEqual(t, books.Bind(data).IsValid(), true)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets_with_custom_save_method_related_instance
//
// The ModelForm.save() method should be able to access the related object
// if it exists in the database (#24395).
func TestInlineFormsetsWithCustomSaveMethodRelatedInstance(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poemFormSet := forms.NewInlineFormSet[Poet, Poem](db, "")
		poemFormSet.SaveNew = func(ctx context.Context, form *forms.ModelForm[Poem], commit bool) (*Poem, error) {
			poem := form.Instance()
			poet, ok := poem.Poet.Object()
			if !ok {
				t.Fatal("the poem has no poet")
			}
			poem.Name = fmt.Sprintf("%s by %s", poem.Name, poet.Name)
			if commit {
				if err := orm.Query[Poem](db).Save(ctx, poem); err != nil {
					return nil, err
				}
			}
			return poem, nil
		}
		data := postData(
			"poem_set-TOTAL_FORMS", "1",
			"poem_set-INITIAL_FORMS", "0",
			"poem_set-MAX_NUM_FORMS", "",
			"poem_set-0-name", "Le Lac",
		)
		poet := &Poet{}
		formset := poemFormSet.WithInstance(poet).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)

		// The Poet instance is saved after the formset instantiation. This
		// happens in admin's changeform_view() when adding a new object and
		// some inlines in the same request.
		poet.Name = "Lamartine"
		create(t, db, poet)
		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, saved[0].Name, "Le Lac by Lamartine")
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inline_formsets_with_wrong_fk_name
//
// Regression for #23451
func TestInlineFormsetsWithWrongFKName(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		fs := forms.NewInlineFormSet[Author, Book](db, "title")
		assertErrContains(t, fs.Err(), "fk_name 'title' is not a ForeignKey to 'model_formsets.Author'.")
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_custom_pk
func TestCustomPK(t *testing.T) {
	// We need to ensure that it is displayed
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		customPrimaryKeyFormSet := forms.NewModelFormSet[CustomPrimaryKey](db)
		formset := customPrimaryKeyFormSet
		djangotest.AssertEqual(t, len(formset.Forms()), 1)
		djangotest.AssertHTMLEqual(t, formset.Forms()[0].AsP(),
			`<p><label for="id_form-0-my_pk">My pk:</label>`+
				`<input id="id_form-0-my_pk" type="text" name="form-0-my_pk" `+
				`maxlength="10"></p>`+
				`<p><label for="id_form-0-some_field">Some field:</label>`+
				`<input id="id_form-0-some_field" type="text" name="form-0-some_field" `+
				`maxlength="100"></p>`)

		// Custom primary keys with ForeignKey, OneToOneField and AutoField.

		place := create(t, db, &Place{Name: "Giordanos", City: "Chicago"})

		ownerFormSet := forms.NewInlineFormSet[Place, Owner](db, "")
		ownerFormSet.Extra, ownerFormSet.CanDelete = 2, false
		owners := ownerFormSet.WithInstance(place)
		djangotest.AssertEqual(t, len(owners.Forms()), 2)
		for i := range 2 {
			djangotest.AssertHTMLEqual(t, owners.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_owner_set-%[1]d-name">Name:</label>`+
					`<input id="id_owner_set-%[1]d-name" type="text" name="owner_set-%[1]d-name" `+
					`maxlength="100">`+
					`<input type="hidden" name="owner_set-%[1]d-place" value="%[2]d" `+
					`id="id_owner_set-%[1]d-place">`+
					`<input type="hidden" name="owner_set-%[1]d-auto_id" `+
					`id="id_owner_set-%[1]d-auto_id"></p>`, i, place.ID))
		}

		data := postData(
			"owner_set-TOTAL_FORMS", "2",
			"owner_set-INITIAL_FORMS", "0",
			"owner_set-MAX_NUM_FORMS", "",
			"owner_set-0-auto_id", "",
			"owner_set-0-name", "Joe Perry",
			"owner_set-1-auto_id", "",
			"owner_set-1-name", "",
		)
		owners = ownerFormSet.WithInstance(place).Bind(data)
		djangotest.AssertEqual(t, owners.IsValid(), true)
		saved, err := owners.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		owner1 := saved[0]
		djangotest.AssertEqual(t, owner1.Name, "Joe Perry")
		djangotest.AssertEqual(t, get[Place](t, db, "pk", owner1.Place.PK()).Name, "Giordanos")

		owners = ownerFormSet.WithInstance(place)
		djangotest.AssertEqual(t, len(owners.Forms()), 3)
		djangotest.AssertHTMLEqual(t, owners.Forms()[0].AsP(),
			`<p><label for="id_owner_set-0-name">Name:</label>`+
				`<input id="id_owner_set-0-name" type="text" name="owner_set-0-name" `+
				`value="Joe Perry" maxlength="100">`+
				fmt.Sprintf(`<input type="hidden" name="owner_set-0-place" value="%d" `, place.ID)+
				`id="id_owner_set-0-place">`+
				fmt.Sprintf(`<input type="hidden" name="owner_set-0-auto_id" value="%d" `, owner1.AutoID)+
				`id="id_owner_set-0-auto_id"></p>`)
		for i := 1; i < 3; i++ {
			djangotest.AssertHTMLEqual(t, owners.Forms()[i].AsP(), fmt.Sprintf(
				`<p><label for="id_owner_set-%[1]d-name">Name:</label>`+
					`<input id="id_owner_set-%[1]d-name" type="text" name="owner_set-%[1]d-name" `+
					`maxlength="100">`+
					`<input type="hidden" name="owner_set-%[1]d-place" value="%[2]d" `+
					`id="id_owner_set-%[1]d-place">`+
					`<input type="hidden" name="owner_set-%[1]d-auto_id" `+
					`id="id_owner_set-%[1]d-auto_id"></p>`, i, place.ID))
		}

		data = postData(
			"owner_set-TOTAL_FORMS", "3",
			"owner_set-INITIAL_FORMS", "1",
			"owner_set-MAX_NUM_FORMS", "",
			"owner_set-0-auto_id", fmt.Sprint(owner1.AutoID),
			"owner_set-0-name", "Joe Perry",
			"owner_set-1-auto_id", "",
			"owner_set-1-name", "Jack Berry",
			"owner_set-2-auto_id", "",
			"owner_set-2-name", "",
		)
		owners = ownerFormSet.WithInstance(place).Bind(data)
		djangotest.AssertEqual(t, owners.IsValid(), true)
		saved, err = owners.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		owner2 := saved[0]
		djangotest.AssertEqual(t, owner2.Name, "Jack Berry")
		djangotest.AssertEqual(t, get[Place](t, db, "pk", owner2.Place.PK()).Name, "Giordanos")

		// A custom primary key that is a ForeignKey or OneToOneField get
		// rendered for the user to choose. The Go Owner's String has no
		// " at Giordanos", and the empty choice keeps forms.EmptyChoiceLabel.
		profiles := forms.NewModelFormSet[OwnerProfile](db)
		djangotest.AssertHTMLEqual(t, profiles.Forms()[0].AsP(), fmt.Sprintf(
			`<p><label for="id_form-0-owner">Owner:</label>`+
				`<select name="form-0-owner" id="id_form-0-owner">`+
				`<option value="" selected>%s</option>`+
				`<option value="%d">Joe Perry</option>`+
				`<option value="%d">Jack Berry</option>`+
				`</select></p>`+
				`<p><label for="id_form-0-age">Age:</label>`+
				`<input type="number" name="form-0-age" id="id_form-0-age" min="0"></p>`,
			forms.EmptyChoiceLabel, owner1.AutoID, owner2.AutoID))

		reloaded := get[Owner](t, db, "name", "Joe Perry")
		owner1 = &reloaded
		profileFormSet := forms.NewInlineFormSet[Owner, OwnerProfile](db, "")
		profileFormSet.MaxNum, profileFormSet.CanDelete = 1, false
		djangotest.AssertEqual(t, profileFormSet.MaxNum, 1)

		inline := profileFormSet.WithInstance(owner1)
		djangotest.AssertEqual(t, len(inline.Forms()), 1)
		djangotest.AssertHTMLEqual(t, inline.Forms()[0].AsP(),
			`<p><label for="id_ownerprofile-0-age">Age:</label>`+
				`<input type="number" name="ownerprofile-0-age" `+
				`id="id_ownerprofile-0-age" min="0">`+
				fmt.Sprintf(`<input type="hidden" name="ownerprofile-0-owner" value="%d" `, owner1.AutoID)+
				`id="id_ownerprofile-0-owner"></p>`)

		data = postData(
			"ownerprofile-TOTAL_FORMS", "1",
			"ownerprofile-INITIAL_FORMS", "0",
			"ownerprofile-MAX_NUM_FORMS", "1",
			"ownerprofile-0-owner", "",
			"ownerprofile-0-age", "54",
		)
		inline = profileFormSet.WithInstance(owner1).Bind(data)
		djangotest.AssertEqual(t, inline.IsValid(), true)
		profiles1, err := inline.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(profiles1), 1)
		djangotest.AssertEqual(t, profiles1[0].Owner.PK(), owner1.AutoID)
		djangotest.AssertEqual(t, profiles1[0].Age, 54)

		inline = profileFormSet.WithInstance(owner1)
		djangotest.AssertEqual(t, len(inline.Forms()), 1)
		djangotest.AssertHTMLEqual(t, inline.Forms()[0].AsP(),
			`<p><label for="id_ownerprofile-0-age">Age:</label>`+
				`<input type="number" name="ownerprofile-0-age" value="54" `+
				`id="id_ownerprofile-0-age" min="0">`+
				fmt.Sprintf(`<input type="hidden" name="ownerprofile-0-owner" value="%d" `, owner1.AutoID)+
				`id="id_ownerprofile-0-owner"></p>`)

		data = postData(
			"ownerprofile-TOTAL_FORMS", "1",
			"ownerprofile-INITIAL_FORMS", "1",
			"ownerprofile-MAX_NUM_FORMS", "1",
			"ownerprofile-0-owner", fmt.Sprint(owner1.AutoID),
			"ownerprofile-0-age", "55",
		)
		inline = profileFormSet.WithInstance(owner1).Bind(data)
		djangotest.AssertEqual(t, inline.IsValid(), true)
		profiles1, err = inline.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(profiles1), 1)
		djangotest.AssertEqual(t, profiles1[0].Owner.PK(), owner1.AutoID)
		djangotest.AssertEqual(t, profiles1[0].Age, 55)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_unique_true_enforces_max_num_one
func TestUniqueTrueEnforcesMaxNumOne(t *testing.T) {
	// ForeignKey with unique=True should enforce max_num=1
	eachDB(t, func(t *testing.T, db *orm.DB) {
		place := create(t, db, &Place{Name: "Giordanos", City: "Chicago"})

		formSet := forms.NewInlineFormSet[Place, Location](db, "")
		formSet.CanDelete = false
		djangotest.AssertEqual(t, formSet.MaxNum, 1)

		formset := formSet.WithInstance(place)
		djangotest.AssertEqual(t, len(formset.Forms()), 1)
		djangotest.AssertHTMLEqual(t, formset.Forms()[0].AsP(),
			`<p><label for="id_location_set-0-lat">Lat:</label>`+
				`<input id="id_location_set-0-lat" type="text" name="location_set-0-lat" `+
				`maxlength="100"></p>`+
				`<p><label for="id_location_set-0-lon">Lon:</label>`+
				`<input id="id_location_set-0-lon" type="text" name="location_set-0-lon" `+
				`maxlength="100">`+
				fmt.Sprintf(`<input type="hidden" name="location_set-0-place" value="%d" `, place.ID)+
				`id="id_location_set-0-place">`+
				`<input type="hidden" name="location_set-0-id" `+
				`id="id_location_set-0-id"></p>`)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_foreign_keys_in_parents
func TestForeignKeysInParents(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance, " +
		"Django: _get_foreign_key finds the foreign key of Restaurant(Place) to Owner")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_unique_validation
func TestUniqueValidation(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := forms.NewModelFormSet[Product](db)
		formSet.Extra = 1
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-slug", "car-red",
		)
		formset := formSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		djangotest.AssertEqual(t, saved[0].Slug, "car-red")

		formset = formSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, errorMessages(formset.Errors()),
			[]map[string][]string{{"slug": {"Product with this Slug already exists."}}})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_modelformset_validate_max_flag
func TestModelformsetValidateMaxFlag(t *testing.T) {
	// If validate_max is set and max_num is less than TOTAL_FORMS in the
	// data, then throw an exception. MAX_NUM_FORMS in the data is
	// irrelevant here (it's output as a hint for the client but its
	// value in the returned data is not checked)
	eachDB(t, func(t *testing.T, db *orm.DB) {
		data := postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "2", // should be ignored
			"form-0-price", "12.00",
			"form-0-quantity", "1",
			"form-1-price", "24.00",
			"form-1-quantity", "2",
		)

		formSet := forms.NewModelFormSet[Price](db)
		formSet.Extra, formSet.MaxNum, formSet.ValidateMax = 1, 1, true
		formset := formSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(), []string{"Please submit at most 1 form."})

		// Now test the same thing without the validate_max flag to ensure
		// default behavior is unchanged
		formSet = forms.NewModelFormSet[Price](db)
		formSet.Extra, formSet.MaxNum = 1, 1
		djangotest.AssertEqual(t, formSet.Bind(data).IsValid(), true)
	})
}

// productMinMaxFormSet is the Product formset of the min_num and max_num
// tests: extra=1, max_num=2, validate_max, min_num=2, validate_min.
func productMinMaxFormSet(db *orm.DB) *forms.ModelFormSet[Product] {
	fs := forms.NewModelFormSet[Product](db)
	fs.Extra, fs.MaxNum, fs.ValidateMax, fs.MinNum, fs.ValidateMin = 1, 2, true, 2, true
	return fs
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_modelformset_min_num_equals_max_num_less_than
func TestModelformsetMinNumEqualsMaxNumLessThan(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		data := postData(
			"form-TOTAL_FORMS", "3",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "2",
			"form-0-slug", "car-red",
			"form-1-slug", "car-blue",
			"form-2-slug", "car-black",
		)
		formset := productMinMaxFormSet(db).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(), []string{"Please submit at most 2 forms."})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_modelformset_min_num_equals_max_num_more_than
func TestModelformsetMinNumEqualsMaxNumMoreThan(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "2",
			"form-0-slug", "car-red",
		)
		formset := productMinMaxFormSet(db).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(), []string{"Please submit at least 2 forms."})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_unique_together_validation
func TestUniqueTogetherValidation(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := forms.NewModelFormSet[Price](db)
		formSet.Extra = 1
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-price", "12.00",
			"form-0-quantity", "1",
		)
		formset := formSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		djangotest.AssertEqual(t, saved[0].Price.Equal(orm.MustDecimal("12.00")), true)
		djangotest.AssertEqual(t, saved[0].Quantity, 1)

		formset = formSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, errorMessages(formset.Errors()),
			[]map[string][]string{{"__all__": {"Price with this Price and Quantity already exists."}}})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_unique_together_with_inlineformset_factory
func TestUniqueTogetherWithInlineformsetFactory(t *testing.T) {
	// Also see bug #8882.
	eachDB(t, func(t *testing.T, db *orm.DB) {
		repository := create(t, db, &Repository{Name: "Test Repo"})
		formSet := forms.NewInlineFormSet[Repository, Revision](db, "")
		formSet.Extra = 1
		data := postData(
			"revision_set-TOTAL_FORMS", "1",
			"revision_set-INITIAL_FORMS", "0",
			"revision_set-MAX_NUM_FORMS", "",
			"revision_set-0-repository", fmt.Sprint(repository.ID),
			"revision_set-0-revision", "146239817507f148d448db38840db7c3cbf47c76",
			"revision_set-0-DELETE", "",
		)
		formset := formSet.WithInstance(repository).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		saved, err := formset.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, len(saved), 1)
		djangotest.AssertEqual(t, saved[0].Repository.PK(), repository.ID)
		djangotest.AssertEqual(t, saved[0].Revision, "146239817507f148d448db38840db7c3cbf47c76")

		// attempt to save the same revision against the same repo.
		formset = formSet.WithInstance(repository).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, errorMessages(formset.Errors()),
			[]map[string][]string{{"__all__": {"Revision with this Repository and Revision already exists."}}})

		// unique_together with inlineformset_factory with overridden form
		// fields Also see #9494

		formSet = forms.NewInlineFormSet[Repository, Revision](db, "", forms.WithFields("revision"))
		formSet.Extra = 1
		djangotest.AssertEqual(t, formSet.WithInstance(repository).Bind(data).IsValid(), false)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_callable_defaults
func TestCallableDefaults(t *testing.T) {
	t.Skip("FIXME-PORT: Go: no callable datetime defaults and no show_hidden_initial, " +
		"Django: a Membership with date_joined=timezone.now renders a hidden initial field")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inlineformset_factory_with_null_fk
func TestInlineformsetFactoryWithNullFK(t *testing.T) {
	// inlineformset_factory tests with fk having null=True. see #9462.
	// create some data that will exhibit the issue
	eachDB(t, func(t *testing.T, db *orm.DB) {
		team := create(t, db, &Team{Name: "Red Vipers"})
		create(t, db, &Player{Name: "Timmy"})
		create(t, db, &Player{Name: "Bobby", Team: fk[Team](team.ID)})

		playerInlineFormSet := forms.NewInlineFormSet[Team, Player](db, "")
		djangotest.AssertEqual(t, len(all(t, playerInlineFormSet.GetQuerySet())), 0)

		players := all(t, playerInlineFormSet.WithInstance(team).GetQuerySet())
		djangotest.AssertEqual(t, len(players), 1)
		djangotest.AssertEqual(t, players[0].Team.PK(), team.ID)
		djangotest.AssertEqual(t, players[0].Name, "Bobby")
	})
}

// simpleArrayField is the SimpleArrayField of Django's test, a proxy for
// django.contrib.postgres.forms.SimpleArrayField: a CharField that splits
// its value at commas.
type simpleArrayField struct {
	forms.CharField
}

// newSimpleArrayField returns a simpleArrayField with a text widget.
func newSimpleArrayField() *simpleArrayField {
	f := &simpleArrayField{}
	f.Init(f, forms.TextInput{}, nil)
	return f
}

// ToGo splits the value at commas.
func (f *simpleArrayField) ToGo(value any) (any, error) {
	v, err := f.CharField.ToGo(value)
	if err != nil {
		return nil, err
	}
	if s, _ := v.(string); s != "" {
		return strings.Split(s, ","), nil
	}
	return []string{}, nil
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inlineformset_with_arrayfield
//
// Django also asserts that BookForm.Meta.fields stays ["title"]; Go options
// are never changed by a formset, so that has no Go counterpart.
func TestInlineformsetWithArrayfield(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "",
			forms.WithFields("title"), forms.WithFormField("title", newSimpleArrayField()))
		data := postData(
			"book_set-TOTAL_FORMS", "3",
			"book_set-INITIAL_FORMS", "0",
			"book_set-MAX_NUM_FORMS", "",
			"book_set-0-title", "test1,test2",
			"book_set-1-title", "test1,test2",
			"book_set-2-title", "test3,test4",
		)
		author := create(t, db, &Author{Name: "test"})
		formset := bookFormSet.WithInstance(author).Bind(data)
		djangotest.AssertEqual(t, errorMessages(formset.Errors()), []map[string][]string{
			{}, {"__all__": {"Please correct the duplicate values below."}}, {},
		})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_inlineformset_with_jsonfield
//
// Django posts dicts in a plain dict; url.Values carries their JSON.
func TestInlineformsetWithJsonfield(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "",
			forms.WithFields("title"), forms.WithFormField("title", forms.NewJSONField()))
		data := postData(
			"book_set-TOTAL_FORMS", "3",
			"book_set-INITIAL_FORMS", "0",
			"book_set-MAX_NUM_FORMS", "",
			"book_set-0-title", `{"test1": "test2"}`,
			"book_set-1-title", `{"test1": "test2"}`,
			"book_set-2-title", `{"test3": "test4"}`,
		)
		author := create(t, db, &Author{Name: "test"})
		formset := bookFormSet.WithInstance(author).Bind(data)
		djangotest.AssertEqual(t, errorMessages(formset.Errors()), []map[string][]string{
			{}, {"__all__": {"Please correct the duplicate values below."}}, {},
		})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_model_formset_with_custom_pk
func TestModelFormsetWithCustomPK(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance, " +
		"Django: ClassyMexicanRestaurant keeps the hidden the_restaurant parent link")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_model_formset_with_initial_model_instance
func TestModelFormsetWithInitialModelInstance(t *testing.T) {
	// has_changed should compare model instance and primary key
	// see #18898
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := forms.NewModelFormSet[Poem](db)
		johnMilton := create(t, db, &Poet{Name: "John Milton"})
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-name", "",
			"form-0-poet", fmt.Sprint(johnMilton.ID),
		)
		formSet.Initial = []map[string]any{{"poet": johnMilton}}
		formset := formSet.Bind(data)
		djangotest.AssertEqual(t, formset.ExtraForms()[0].HasChanged(), false)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_model_formset_with_initial_queryset
//
// Django's initial is a queryset; a Go slice of the rows stands for it.
func TestModelFormsetWithInitialQueryset(t *testing.T) {
	// has_changed should work with queryset and list of pk's
	// see #18898
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formSet := forms.NewModelFormSet[AuthorMeeting](db)
		create(t, db, &Author{Name: "Charles Baudelaire"})
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-name", "",
			"form-0-created", "",
		)
		ids, err := orm.Query[Author](db).ValuesList("id").All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range ids {
			data.Add("form-0-authors", fmt.Sprint(row[0]))
		}
		formSet.Initial = []map[string]any{{"authors": all(t, orm.Query[Author](db))}}
		formset := formSet.Bind(data)
		djangotest.AssertEqual(t, formset.ExtraForms()[0].HasChanged(), false)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_get_queryset_falls_back_to_pk_when_no_ordering_defined
func TestGetQuerysetFallsBackToPKWhenNoOrderingDefined(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// Product has no Meta.ordering, so the default queryset is not
		// totally ordered.
		djangotest.AssertEqual(t, orm.Query[Product](db).TotallyOrdered(), false)

		formset := forms.NewModelFormSet[Product](db)
		djangotest.AssertEqual(t, formset.GetQuerySet().Ordering(), []any{"pk"})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_get_queryset_appends_pk_to_explicit_queryset_ordering
func TestGetQuerysetAppendsPKToExplicitQuerysetOrdering(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// Author.name is non-unique, so order_by("name") is not totally
		// ordered.
		formset := forms.NewModelFormSet[Author](db)
		formset.QuerySet = orm.Query[Author](db).OrderBy("name")
		djangotest.AssertEqual(t, formset.GetQuerySet().Ordering(), []any{"name", "pk"})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_get_queryset_appends_pk_to_meta_ordering
func TestGetQuerysetAppendsPKToMetaOrdering(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// Author has Meta.ordering = ("name",) which is not deterministic
		// by itself.
		formset := forms.NewModelFormSet[Author](db)
		djangotest.AssertEqual(t, formset.GetQuerySet().Ordering(), []any{"name", "pk"})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_get_queryset_unchanged_when_already_totally_ordered
func TestGetQuerysetUnchangedWhenAlreadyTotallyOrdered(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		// Ordering by pk is already totally ordered; pk must not be
		// appended again.
		formset := forms.NewModelFormSet[Author](db)
		formset.QuerySet = orm.Query[Author](db).OrderBy("pk")
		qs := formset.GetQuerySet()
		// Must be exactly ("pk",), not ("pk", "pk").
		djangotest.AssertEqual(t, qs.Ordering(), []any{"pk"})
		djangotest.AssertEqual(t, qs.TotallyOrdered(), true)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_prevent_duplicates_from_with_the_same_formset
func TestPreventDuplicatesFromWithTheSameFormset(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		products := forms.NewModelFormSet[Product](db)
		products.Extra = 2
		data := postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-slug", "red_car",
			"form-1-slug", "red_car",
		)
		formset := products.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(),
			[]string{"Please correct the duplicate data for slug."})

		prices := forms.NewModelFormSet[Price](db)
		prices.Extra = 2
		data = postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-price", "25",
			"form-0-quantity", "7",
			"form-1-price", "25",
			"form-1-quantity", "7",
		)
		pricesBound := prices.Bind(data)
		djangotest.AssertEqual(t, pricesBound.IsValid(), false)
		djangotest.AssertEqual(t, pricesBound.NonFormErrors().Messages(),
			[]string{"Please correct the duplicate data for price and quantity, which must be unique."})

		// Only the price field is specified, this should skip any unique
		// checks since the unique_together is not fulfilled. This will fail
		// with a KeyError if broken.
		prices = forms.NewModelFormSet[Price](db, forms.WithFields("price"))
		prices.Extra = 2
		data = postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-price", "24",
			"form-1-price", "24",
		)
		djangotest.AssertEqual(t, prices.Bind(data).IsValid(), true)

		books := forms.NewInlineFormSet[Author, Book](db, "")
		books.Extra = 0
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		create(t, db, &Book{Author: fk[Author](author.ID), Title: "Les Paradis Artificiels"})
		create(t, db, &Book{Author: fk[Author](author.ID), Title: "Les Fleurs du Mal"})
		create(t, db, &Book{Author: fk[Author](author.ID), Title: "Flowers of Evil"})

		bookIDs := all(t, orm.Query[Book](db).Filter("author", author.ID).OrderBy("id"))
		data = postData(
			"book_set-TOTAL_FORMS", "2",
			"book_set-INITIAL_FORMS", "2",
			"book_set-MAX_NUM_FORMS", "",
			"book_set-0-title", "The 2008 Election",
			"book_set-0-author", fmt.Sprint(author.ID),
			"book_set-0-id", fmt.Sprint(bookIDs[0].ID),
			"book_set-1-title", "The 2008 Election",
			"book_set-1-author", fmt.Sprint(author.ID),
			"book_set-1-id", fmt.Sprint(bookIDs[1].ID),
		)
		booksBound := books.WithInstance(author).Bind(data)
		djangotest.AssertEqual(t, booksBound.IsValid(), false)
		djangotest.AssertEqual(t, booksBound.NonFormErrors().Messages(),
			[]string{"Please correct the duplicate data for title."})
		djangotest.AssertEqual(t, errorMessages(booksBound.Errors()), []map[string][]string{
			{}, {"__all__": {"Please correct the duplicate values below."}},
		})

		posts := forms.NewModelFormSet[Post](db)
		posts.Extra = 2
		data = postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-title", "blah",
			"form-0-slug", "Morning",
			"form-0-subtitle", "foo",
			"form-0-posted", "2009-01-01",
			"form-1-title", "blah",
			"form-1-slug", "Morning in Prague",
			"form-1-subtitle", "rawr",
			"form-1-posted", "2009-01-01",
		)
		postsBound := posts.Bind(data)
		djangotest.AssertEqual(t, postsBound.IsValid(), false)
		djangotest.AssertEqual(t, postsBound.NonFormErrors().Messages(), []string{
			"Please correct the duplicate data for title which must be unique for the date in posted.",
		})
		djangotest.AssertEqual(t, errorMessages(postsBound.Errors()), []map[string][]string{
			{}, {"__all__": {"Please correct the duplicate values below."}},
		})

		data = postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-title", "foo",
			"form-0-slug", "Morning in Prague",
			"form-0-subtitle", "foo",
			"form-0-posted", "2009-01-01",
			"form-1-title", "blah",
			"form-1-slug", "Morning in Prague",
			"form-1-subtitle", "rawr",
			"form-1-posted", "2009-08-02",
		)
		postsBound = posts.Bind(data)
		djangotest.AssertEqual(t, postsBound.IsValid(), false)
		djangotest.AssertEqual(t, postsBound.NonFormErrors().Messages(), []string{
			"Please correct the duplicate data for slug which must be unique for the year in posted.",
		})

		data = postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-title", "foo",
			"form-0-slug", "Morning in Prague",
			"form-0-subtitle", "rawr",
			"form-0-posted", "2008-08-01",
			"form-1-title", "blah",
			"form-1-slug", "Prague",
			"form-1-subtitle", "rawr",
			"form-1-posted", "2009-08-02",
		)
		postsBound = posts.Bind(data)
		djangotest.AssertEqual(t, postsBound.IsValid(), false)
		djangotest.AssertEqual(t, postsBound.NonFormErrors().Messages(), []string{
			"Please correct the duplicate data for subtitle which must be unique for the month in posted.",
		})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_prevent_change_outer_model_and_create_invalid_data
func TestPreventChangeOuterModelAndCreateInvalidData(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		author := create(t, db, &Author{Name: "Charles"})
		otherAuthor := create(t, db, &Author{Name: "Walt"})
		authorFormSet := forms.NewModelFormSet[Author](db)
		data := postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "2",
			"form-MAX_NUM_FORMS", "",
			"form-0-id", fmt.Sprint(author.ID),
			"form-0-name", "Charles",
			"form-1-id", fmt.Sprint(otherAuthor.ID), // A model not in the formset's queryset.
			"form-1-name", "Changed name",
		)
		// This formset is only for Walt Whitman and shouldn't accept data for
		// other_author.
		authorFormSet.QuerySet = orm.Query[Author](db).Filter("id__in", []any{author.ID})
		formset := authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		// The name of other_author shouldn't be changed and new models aren't
		// created.
		djangotest.AssertEqual(t, all(t, orm.Query[Author](db)), []Author{*author, *otherAuthor})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_overridden_add_prefix
func TestOverriddenAddPrefix(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a form has no add_prefix to override, " +
		"Django: an AuthorForm whose add_prefix joins with a dot finds form-0.id")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_form_initialized_once_with_default_add_prefix
//
// Django counts the calls of the form's __init__; Go counts the calls of
// NewForm.
func TestFormInitializedOnceWithDefaultAddPrefix(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		initialization := 0
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		authorFormSet := forms.NewModelFormSet[Author](db)
		authorFormSet.NewForm = func() *forms.ModelForm[Author] {
			initialization++
			return forms.NewModelForm[Author](db)
		}
		authorFormSet.QuerySet = orm.Query[Author](db)
		formset := authorFormSet.Bind(postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-0-id", fmt.Sprint(author.ID),
		))

		djangotest.AssertEqual(t, *formset.ModelForms()[0].Instance(), *author)
		djangotest.AssertEqual(t, initialization, 1)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_validation_without_id
func TestValidationWithoutID(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorFormSet := forms.NewModelFormSet[Author](db)
		formset := authorFormSet.Bind(postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "",
			"form-0-name", "Charles",
		))
		djangotest.AssertEqual(t, errorMessages(formset.Errors()),
			[]map[string][]string{{"id": {"This field is required."}}})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_validation_with_child_model_without_id
func TestValidationWithChildModelWithoutID(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no multi-table inheritance, " +
		"Django: a BetterAuthor formset requires the author_ptr parent link")
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_validation_with_invalid_id
func TestValidationWithInvalidID(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorFormSet := forms.NewModelFormSet[Author](db)
		formset := authorFormSet.Bind(postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "",
			"form-0-id", "abc",
			"form-0-name", "Charles",
		))
		djangotest.AssertEqual(t, errorMessages(formset.Errors()), []map[string][]string{
			{"id": {"Select a valid choice. That choice is not one of the available choices."}},
		})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_validation_with_nonexistent_id
func TestValidationWithNonexistentID(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorFormSet := forms.NewModelFormSet[Author](db)
		formset := authorFormSet.Bind(postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "",
			"form-0-id", "12345",
			"form-0-name", "Charles",
		))
		djangotest.AssertEqual(t, errorMessages(formset.Errors()), []map[string][]string{
			{"id": {"Select a valid choice. That choice is not one of the available choices."}},
		})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_initial_form_count_empty_data
func TestInitialFormCountEmptyData(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewModelFormSet[Author](db).Bind(url.Values{})
		djangotest.AssertEqual(t, formset.InitialFormCount(), 0)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_edit_only
func TestEditOnly(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		charles := create(t, db, &Author{Name: "Charles Baudelaire"})
		authorFormSet := forms.NewModelFormSet[Author](db)
		authorFormSet.EditOnly = true
		data := postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "0",
			"form-0-name", "Arthur Rimbaud",
			"form-1-name", "Walt Whitman",
		)
		formset := authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(ctx); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, all(t, orm.Query[Author](db)), []Author{*charles})
		data = postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "0",
			"form-0-id", fmt.Sprint(charles.ID),
			"form-0-name", "Arthur Rimbaud",
			"form-1-name", "Walt Whitman",
		)
		formset = authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(ctx); err != nil {
			t.Fatal(err)
		}
		reloaded := get[Author](t, db, "pk", charles.ID)
		djangotest.AssertEqual(t, reloaded.Name, "Arthur Rimbaud")
		djangotest.AssertEqual(t, all(t, orm.Query[Author](db)), []Author{reloaded})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_edit_only_inlineformset_factory
func TestEditOnlyInlineformsetFactory(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		charles := create(t, db, &Author{Name: "Charles Baudelaire"})
		book := create(t, db, &Book{Author: fk[Author](charles.ID), Title: "Les Paradis Artificiels"})
		authorFormSet := forms.NewInlineFormSet[Author, Book](db, "")
		authorFormSet.CanDelete, authorFormSet.EditOnly = false, true
		data := postData(
			"book_set-TOTAL_FORMS", "4",
			"book_set-INITIAL_FORMS", "1",
			"book_set-MAX_NUM_FORMS", "0",
			"book_set-0-id", fmt.Sprint(book.ID),
			"book_set-0-title", "Les Fleurs du Mal",
			"book_set-0-author", fmt.Sprint(charles.ID),
			"book_set-1-title", "Flowers of Evil",
			"book_set-1-author", fmt.Sprint(charles.ID),
		)
		formset := authorFormSet.WithInstance(charles).Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		reloaded := get[Book](t, db, "pk", book.ID)
		djangotest.AssertEqual(t, reloaded.Title, "Les Fleurs du Mal")
		books := all(t, orm.Query[Book](db))
		djangotest.AssertEqual(t, len(books), 1)
		djangotest.AssertEqual(t, books[0].ID, book.ID)
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_edit_only_object_outside_of_queryset
func TestEditOnlyObjectOutsideOfQueryset(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		charles := create(t, db, &Author{Name: "Charles Baudelaire"})
		walt := create(t, db, &Author{Name: "Walt Whitman"})
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-0-id", fmt.Sprint(walt.ID),
			"form-0-name", "Parth Patil",
		)
		authorFormSet := forms.NewModelFormSet[Author](db)
		authorFormSet.EditOnly = true
		authorFormSet.QuerySet = orm.Query[Author](db).Filter("pk", charles.ID)
		formset := authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, all(t, orm.Query[Author](db).OrderBy("pk")), []Author{*charles, *walt})
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_edit_only_editable_pk
func TestEditOnlyEditablePK(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		customPrimaryKeyFormSet := forms.NewModelFormSet[CustomPrimaryKey](db)
		data := postData(
			"form-TOTAL_FORMS", "1",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "0",
			"form-0-my_pk", "forged",
			"form-0-some_field", "value",
		)
		formset := customPrimaryKeyFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		_, err := orm.Query[CustomPrimaryKey](db).Get(context.Background(), "pk", "forged")
		if !errors.Is(err, orm.ErrDoesNotExist) {
			t.Errorf("Get(pk=forged) = %v, want orm.ErrDoesNotExist", err)
		}
	})
}

// Ported from Django: tests/model_formsets/tests.py::ModelFormsetTest.test_edit_only_formset_factory_with_basemodelformset
//
// Django builds the formset with formset_factory and a BaseModelFormSet
// that sets the model; NewModelFormSet is the only way in Go.
func TestEditOnlyFormsetFactoryWithBasemodelformset(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		charles := create(t, db, &Author{Name: "Charles Baudelaire"})
		authorFormSet := forms.NewModelFormSet[Author](db)
		data := postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "1",
			"form-MAX_NUM_FORMS", "0",
			"form-0-id", fmt.Sprint(charles.ID),
			"form-0-name", "Shawn Dong",
			"form-1-name", "Walt Whitman",
		)
		formset := authorFormSet.Bind(data)
		djangotest.AssertEqual(t, formset.IsValid(), true)
		if _, err := formset.Save(context.Background()); err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, count[Author](t, db), int64(2))
		djangotest.AssertEqual(t, get[Author](t, db, "pk", charles.ID).Name, "Shawn Dong")
		djangotest.AssertEqual(t, count[Author](t, db), int64(2))
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_widgets
func TestModelformsetFactoryWidgets(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		poetFormSet := forms.NewModelFormSet[Poet](db,
			forms.WithWidget("name", forms.TextInput{Attrs: forms.Attrs{"class": "poet"}}))
		form := poetFormSet.NewForm()
		djangotest.AssertHTMLEqual(t, form.Field("name").String(),
			`<input id="id_name" maxlength="100" type="text" class="poet" name="name" required>`)
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_widgets
func TestInlineformsetFactoryWidgets(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "",
			forms.WithWidget("title", forms.TextInput{Attrs: forms.Attrs{"class": "book"}}))
		form := bookFormSet.NewForm()
		djangotest.AssertHTMLEqual(t, form.Field("title").String(),
			`<input class="book" id="id_title" maxlength="100" name="title" type="text" required>`)
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_labels_overrides
func TestModelformsetFactoryLabelsOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		bookFormSet := forms.NewModelFormSet[Book](db, forms.WithLabel("title", "Name"))
		form := bookFormSet.NewForm()
		djangotest.AssertHTMLEqual(t, form.Field("title").LabelTag(), `<label for="id_title">Name:</label>`)
		djangotest.AssertHTMLEqual(t, form.Field("title").LegendTag(), `<legend>Name:</legend>`)
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_labels_overrides
func TestInlineformsetFactoryLabelsOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "", forms.WithLabel("title", "Name"))
		form := bookFormSet.NewForm()
		djangotest.AssertHTMLEqual(t, form.Field("title").LabelTag(), `<label for="id_title">Name:</label>`)
		djangotest.AssertHTMLEqual(t, form.Field("title").LegendTag(), `<legend>Name:</legend>`)
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_help_text_overrides
func TestModelformsetFactoryHelpTextOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		bookFormSet := forms.NewModelFormSet[Book](db, forms.WithHelpText("title", "Choose carefully."))
		form := bookFormSet.NewForm()
		djangotest.AssertEqual(t, form.Field("title").HelpText(), "Choose carefully.")
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_help_text_overrides
func TestInlineformsetFactoryHelpTextOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "", forms.WithHelpText("title", "Choose carefully."))
		form := bookFormSet.NewForm()
		djangotest.AssertEqual(t, form.Field("title").HelpText(), "Choose carefully.")
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_error_messages_overrides
func TestModelformsetFactoryErrorMessagesOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		bookFormSet := forms.NewModelFormSet[Book](db,
			forms.WithErrorMessages("title", map[string]string{"max_length": "Title too long!!"}))
		form := bookFormSet.NewForm().Bind(postData("title", strings.Repeat("Foo ", 30), "author", fmt.Sprint(author.ID)))
		djangotest.AssertEqual(t, form.Errors().Messages(), map[string][]string{"title": {"Title too long!!"}})
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_error_messages_overrides
func TestInlineformsetFactoryErrorMessagesOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "",
			forms.WithErrorMessages("title", map[string]string{"max_length": "Title too long!!"}))
		form := bookFormSet.NewForm().Bind(postData("title", strings.Repeat("Foo ", 30), "author", fmt.Sprint(author.ID)))
		djangotest.AssertEqual(t, form.Errors().Messages(), map[string][]string{"title": {"Title too long!!"}})
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_field_class_overrides
func TestModelformsetFactoryFieldClassOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		bookFormSet := forms.NewModelFormSet[Book](db, forms.WithFieldClass("title", forms.NewSlugField))
		form := bookFormSet.NewForm().Bind(postData("title", strings.Repeat("Foo ", 30), "author", fmt.Sprint(author.ID)))
		title, _ := bookFormSet.Model().FieldByAttr("title")
		djangotest.AssertEqual(t, title.Kind, orm.KindChar)
		field, _ := form.FieldDef("title")
		if _, ok := field.(*forms.SlugField); !ok {
			t.Errorf("title is a %T, want *forms.SlugField", field)
		}
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_field_class_overrides
func TestInlineformsetFactoryFieldClassOverrides(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "", forms.WithFieldClass("title", forms.NewSlugField))
		form := bookFormSet.NewForm().Bind(postData("title", strings.Repeat("Foo ", 30), "author", fmt.Sprint(author.ID)))
		title, _ := bookFormSet.Model().FieldByAttr("title")
		djangotest.AssertEqual(t, title.Kind, orm.KindChar)
		field, _ := form.FieldDef("title")
		if _, ok := field.(*forms.SlugField); !ok {
			t.Errorf("title is a %T, want *forms.SlugField", field)
		}
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_absolute_max
func TestModelformsetFactoryAbsoluteMax(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorFormSet := forms.NewModelFormSet[Author](db)
		authorFormSet.AbsoluteMax = 1500
		formset := authorFormSet.Bind(postData(
			"form-TOTAL_FORMS", "1501",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "0",
		))
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, len(formset.Forms()), 1500)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(), []string{"Please submit at most 1000 forms."})
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_absolute_max_with_max_num
func TestModelformsetFactoryAbsoluteMaxWithMaxNum(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		authorFormSet := forms.NewModelFormSet[Author](db)
		authorFormSet.MaxNum, authorFormSet.AbsoluteMax = 20, 100
		formset := authorFormSet.Bind(postData(
			"form-TOTAL_FORMS", "101",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "0",
		))
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, len(formset.Forms()), 100)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(), []string{"Please submit at most 20 forms."})
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_absolute_max
func TestInlineformsetFactoryAbsoluteMax(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "")
		bookFormSet.AbsoluteMax = 1500
		formset := bookFormSet.WithInstance(author).Bind(postData(
			"book_set-TOTAL_FORMS", "1501",
			"book_set-INITIAL_FORMS", "0",
			"book_set-MAX_NUM_FORMS", "0",
		))
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, len(formset.Forms()), 1500)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(), []string{"Please submit at most 1000 forms."})
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_absolute_max_with_max_num
func TestInlineformsetFactoryAbsoluteMaxWithMaxNum(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		author := create(t, db, &Author{Name: "Charles Baudelaire"})
		bookFormSet := forms.NewInlineFormSet[Author, Book](db, "")
		bookFormSet.MaxNum, bookFormSet.AbsoluteMax = 20, 100
		formset := bookFormSet.WithInstance(author).Bind(postData(
			"book_set-TOTAL_FORMS", "101",
			"book_set-INITIAL_FORMS", "0",
			"book_set-MAX_NUM_FORMS", "0",
		))
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, len(formset.Forms()), 100)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(), []string{"Please submit at most 20 forms."})
	})
}

// hasField reports whether the form has a field with the name.
func hasField(f *forms.Form, name string) bool {
	_, ok := f.FieldDef(name)
	return ok
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_can_delete_extra
func TestModelformsetFactoryCanDeleteExtra(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewModelFormSet[Author](db)
		formset.CanDelete, formset.CanDeleteExtra, formset.Extra = true, true, 2
		djangotest.AssertEqual(t, len(formset.Forms()), 2)
		djangotest.AssertEqual(t, hasField(formset.Forms()[0], "DELETE"), true)
		djangotest.AssertEqual(t, hasField(formset.Forms()[1], "DELETE"), true)
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_modelformset_factory_disable_delete_extra
func TestModelformsetFactoryDisableDeleteExtra(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewModelFormSet[Author](db)
		formset.CanDelete, formset.CanDeleteExtra, formset.Extra = true, false, 2
		djangotest.AssertEqual(t, len(formset.Forms()), 2)
		djangotest.AssertEqual(t, hasField(formset.Forms()[0], "DELETE"), false)
		djangotest.AssertEqual(t, hasField(formset.Forms()[1], "DELETE"), false)
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_can_delete_extra
func TestInlineformsetFactoryCanDeleteExtra(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewInlineFormSet[Author, Book](db, "")
		formset.CanDelete, formset.CanDeleteExtra, formset.Extra = true, true, 2
		djangotest.AssertEqual(t, len(formset.Forms()), 2)
		djangotest.AssertEqual(t, hasField(formset.Forms()[0], "DELETE"), true)
		djangotest.AssertEqual(t, hasField(formset.Forms()[1], "DELETE"), true)
	})
}

// Ported from Django: tests/model_formsets/tests.py::TestModelFormsetOverridesTroughFormMeta.test_inlineformset_factory_can_not_delete_extra
func TestInlineformsetFactoryCanNotDeleteExtra(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		formset := forms.NewInlineFormSet[Author, Book](db, "")
		formset.CanDelete, formset.CanDeleteExtra, formset.Extra = true, false, 2
		djangotest.AssertEqual(t, len(formset.Forms()), 2)
		djangotest.AssertEqual(t, hasField(formset.Forms()[0], "DELETE"), false)
		djangotest.AssertEqual(t, hasField(formset.Forms()[1], "DELETE"), false)
	})
}

// test_inlineformset_factory_passes_renderer,
// test_modelformset_factory_passes_renderer,
// test_modelformset_factory_default_renderer and
// test_inlineformset_factory_default_renderer of
// TestModelFormsetOverridesTroughFormMeta have no Go counterpart: forms has
// no renderers, and a formset renders in Go code.

// No Django test exists: Django's BaseModelFormSet.validate_unique checks the
// fields of a unique constraint without a condition like unique_together.
func TestPreventDuplicatesOfUniqueConstraintFields(t *testing.T) {
	eachDB(t, func(t *testing.T, db *orm.DB) {
		prices := forms.NewModelFormSet[ConstraintPrice](db)
		prices.Extra = 2
		formset := prices.Bind(postData(
			"form-TOTAL_FORMS", "2",
			"form-INITIAL_FORMS", "0",
			"form-MAX_NUM_FORMS", "",
			"form-0-price", "25",
			"form-0-quantity", "7",
			"form-1-price", "25",
			"form-1-quantity", "7",
		))
		djangotest.AssertEqual(t, formset.IsValid(), false)
		djangotest.AssertEqual(t, formset.NonFormErrors().Messages(),
			[]string{"Please correct the duplicate data for price and quantity, which must be unique."})
	})
}
