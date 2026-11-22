package contenttypes_test

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Gecko is tests/generic_relations/models.py::Gecko without GeckoManager,
// which only hides geckos without a tail.
type Gecko struct {
	ID      int64
	HasTail bool
}

// BadModel is the model of test_incorrect_content_type, whose content type
// field is an integer.
type BadModel struct {
	ID          int64
	ContentType int64 `orm:"type=positive_int"`
}

// formsModels is the registry of the ported generic inline formset tests.
func formsModels() []any {
	return []any{
		contenttypes.App{},
		djangotest.App("generic_relations",
			&TaggedItem{}, &Comparison{}, &Animal{}, &Mineral{}, &Gecko{}, &BadModel{}),
	}
}

// taggedItemPrefix is the default prefix of a generic inline formset of
// TaggedItem.
const taggedItemPrefix = "generic_relations-taggeditem-content_type-object_id"

// tagFormSet returns generic_inlineformset_factory(TaggedItem, **opts) with
// extra forms.
func tagFormSet(db *orm.DB, extra int, opts ...forms.ModelFormOption) *forms.GenericInlineFormSet[TaggedItem] {
	fs := forms.NewGenericInlineFormSet[TaggedItem](db, "", "", opts...)
	fs.Extra = extra
	return fs
}

// formsHTML joins form.as_p() of the forms of fs.
func formsHTML(fs *forms.FormSet) string {
	var b strings.Builder
	for _, f := range fs.Forms() {
		b.WriteString(f.AsP())
	}
	return b.String()
}

// emptyTagForm is the as_p() of extra form i of a TaggedItem formset with
// prefix.
func emptyTagForm(prefix string, i int) string {
	return fmt.Sprintf(`<p><label for="id_%[1]s-%[2]d-tag">Tag:</label>
		<input id="id_%[1]s-%[2]d-tag" type="text" name="%[1]s-%[2]d-tag" maxlength="50"></p>
		<p><label for="id_%[1]s-%[2]d-DELETE">Delete:</label>
		<input type="checkbox" name="%[1]s-%[2]d-DELETE" id="id_%[1]s-%[2]d-DELETE">
		<input type="hidden" name="%[1]s-%[2]d-id" id="id_%[1]s-%[2]d-id"></p>`, prefix, i)
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_output
func TestGenericInlineFormsetOutput(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		formset := tagFormSet(db, 1)
		djangotest.AssertHTMLEqual(t, formsHTML(formset.FormSet), emptyTagForm(taggedItemPrefix, 0))
		formset = tagFormSet(db, 1).WithInstance(&Animal{})
		djangotest.AssertHTMLEqual(t, formsHTML(formset.FormSet), emptyTagForm(taggedItemPrefix, 0))
		platypus := mustCreate(t, db, &Animal{CommonName: "Platypus", LatinName: "Ornithorhynchus anatinus"})
		shiny := createTag(t, db, platypus, "shiny")
		formset = tagFormSet(db, 1).WithInstance(platypus)
		djangotest.AssertHTMLEqual(t, formsHTML(formset.FormSet), fmt.Sprintf(
			`<p><label for="id_%[1]s-0-tag">Tag:</label>
			<input id="id_%[1]s-0-tag" type="text" name="%[1]s-0-tag" value="shiny" maxlength="50"></p>
			<p><label for="id_%[1]s-0-DELETE">Delete:</label>
			<input type="checkbox" name="%[1]s-0-DELETE" id="id_%[1]s-0-DELETE">
			<input type="hidden" name="%[1]s-0-id" value="%[2]d" id="id_%[1]s-0-id"></p>`,
			taggedItemPrefix, shiny.ID)+emptyTagForm(taggedItemPrefix, 1))
		lion := mustCreate(t, db, &Animal{CommonName: "Lion", LatinName: "Panthera leo"})
		formset = tagFormSet(db, 1).WithInstance(lion)
		formset.Prefix = "x"
		djangotest.AssertHTMLEqual(t, formsHTML(formset.FormSet), emptyTagForm("x", 0))
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_options
func TestGenericInlineFormsetOptions(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		newFormSet := func() *forms.GenericInlineFormSet[TaggedItem] {
			fs := tagFormSet(db, 3, forms.WithExclude("tag"))
			fs.CanDelete = false
			return fs
		}
		platypus := mustCreate(t, db, &Animal{CommonName: "Platypus", LatinName: "Ornithorhynchus anatinus"})
		harmless := createTag(t, db, platypus, "harmless")
		mammal := createTag(t, db, platypus, "mammal")
		// Works without a queryset.
		formset := newFormSet().WithInstance(platypus)
		djangotest.AssertEqual(t, len(formset.Forms()), 5)
		djangotest.AssertHTMLEqual(t, formset.Forms()[0].AsP(), fmt.Sprintf(
			`<input type="hidden" name="%[1]s-0-id" value="%[2]d" id="id_%[1]s-0-id">`, taggedItemPrefix, harmless.ID))
		assertFormRows(t, formset, harmless.ID, mammal.ID, 0)
		// A queryset can be used to alter display ordering.
		formset = newFormSet()
		formset.QuerySet = orm.Query[TaggedItem](db).OrderBy("-tag")
		formset = formset.WithInstance(platypus)
		djangotest.AssertEqual(t, len(formset.Forms()), 5)
		assertFormRows(t, formset, mammal.ID, harmless.ID, 0)
		// A queryset that omits items.
		formset = newFormSet()
		formset.QuerySet = orm.Query[TaggedItem](db).Filter("tag__startswith", "harm")
		formset = formset.WithInstance(platypus)
		djangotest.AssertEqual(t, len(formset.Forms()), 4)
		assertFormRows(t, formset, harmless.ID, 0)
	})
}

// assertFormRows asserts the primary keys of the instances of the first
// forms of fs, 0 for a new row.
func assertFormRows(t *testing.T, fs *forms.GenericInlineFormSet[TaggedItem], want ...int64) {
	t.Helper()
	mfs := fs.ModelForms()
	for i, id := range want {
		djangotest.AssertEqual(t, mfs[i].Instance().ID, id)
	}
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_get_queryset_ordering
func TestGenericInlineFormsetGetQuerySetOrdering(t *testing.T) {
	// GetQuerySet adds default ordering, if needed.
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		gecko := mustCreate(t, db, &Gecko{})
		formset := tagFormSet(db, 3, forms.WithExclude("tag")).WithInstance(gecko)
		djangotest.AssertEqual(t, len(formset.GetQuerySet().Ordering()) > 0, true)
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_initial
func TestGenericInlineFormsetInitial(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		quartz := mustCreate(t, db, &Mineral{Name: "Quartz", Hardness: 7})
		ct, err := contenttypes.GetForInstance(context.Background(), db, quartz)
		if err != nil {
			t.Fatal(err)
		}
		initial := []map[string]any{{"tag": "lizard", "content_type": ct.ID, "object_id": quartz.ID}}
		formset := tagFormSet(db, 1)
		formset.Initial = initial
		djangotest.AssertEqual(t, formset.Forms()[0].Initial, initial[0])
	})
}

// customWidget is the CustomWidget of test_meta_widgets.
type customWidget struct{ forms.TextInput }

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_meta_widgets
func TestGenericInlineFormsetMetaWidgets(t *testing.T) {
	// The options of the form, TaggedItemForm with a widget in Meta, reach
	// the forms.
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		formset := tagFormSet(db, 3, forms.WithFields(forms.AllFields), forms.WithWidget("tag", customWidget{}))
		form := formset.Forms()[0]
		if _, ok := form.Field("tag").Field().Base().Widget.(customWidget); !ok {
			t.Fatalf("widget %T, want customWidget", form.Field("tag").Field().Base().Widget)
		}
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_incorrect_content_type
func TestGenericInlineFormsetIncorrectContentType(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		formset := forms.NewGenericInlineFormSet[BadModel](db, "", "")
		msg := "fk_name 'generic_relations.BadModel.content_type' is not a ForeignKey to ContentType"
		if err := formset.Err(); err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatalf("Err() = %v, want %q", err, msg)
		}
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_save_new_uses_form_save
//
// A ModelForm has no save method to override, so the SaveNew hook of the
// formset takes the place of the save of SaveTestForm. The new row points
// at the object when the hook runs, as in Django's form.save.
func TestGenericInlineFormsetSaveNewUsesFormSave(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		instance := mustCreate(t, db, &Animal{CommonName: "Lion", LatinName: "Panthera leo"})
		formset := tagFormSet(db, 3, forms.WithFields(forms.AllFields))
		formset.Prefix = "form"
		savedBy := ""
		formset.SaveNew = func(ctx context.Context, form *forms.ModelForm[TaggedItem], _ bool) (*TaggedItem, error) {
			savedBy = "custom method"
			djangotest.AssertEqual(t, form.Instance().ObjectID, instance.ID)
			return form.Save(ctx)
		}
		bound := formset.WithInstance(instance).Bind(url.Values{
			"form-TOTAL_FORMS":   {"1"},
			"form-INITIAL_FORMS": {"0"},
			"form-MAX_NUM_FORMS": {""},
			"form-0-tag":         {"foo"},
		})
		djangotest.AssertEqual(t, bound.IsValid(), true)
		saved, err := bound.Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		djangotest.AssertEqual(t, savedBy, "custom method")
		assertSameObject(t, getObject(t, db, saved[0], "content_object"), instance)
	})
}

// No Go counterpart: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_save_new_for_proxy
// and test_save_new_for_concrete test for_concrete_model with a proxy model,
// and the orm has no proxy models.

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_initial_count
func TestGenericInlineFormsetInitialCount(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		data := url.Values{
			"form-TOTAL_FORMS":   {"3"},
			"form-INITIAL_FORMS": {"3"},
			"form-MAX_NUM_FORMS": {""},
		}
		formset := tagFormSet(db, 3)
		formset.Prefix = "form"
		djangotest.AssertEqual(t, formset.Bind(data).InitialFormCount(), 3)
		bound := formset.Bind(data)
		bound.SaveAsNew = true
		djangotest.AssertEqual(t, bound.InitialFormCount(), 0)
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_save_as_new
func TestGenericInlineFormsetSaveAsNew(t *testing.T) {
	// SaveAsNew creates new items that are associated with the object.
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		lion := mustCreate(t, db, &Animal{CommonName: "Lion", LatinName: "Panthera leo"})
		yellow := createTag(t, db, lion, "yellow")
		hairy := createTag(t, db, lion, "hairy")
		formset := tagFormSet(db, 3)
		formset.Prefix = "form"
		bound := formset.WithInstance(lion).Bind(url.Values{
			"form-TOTAL_FORMS":   {"3"},
			"form-INITIAL_FORMS": {"2"},
			"form-MAX_NUM_FORMS": {""},
			"form-0-id":          {strconv.FormatInt(yellow.ID, 10)},
			"form-0-tag":         {"hunts"},
			"form-1-id":          {strconv.FormatInt(hairy.ID, 10)},
			"form-1-tag":         {"roars"},
		})
		bound.SaveAsNew = true
		djangotest.AssertEqual(t, bound.IsValid(), true)
		tags, err := bound.Save(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, len(tags))
		for i, tag := range tags {
			names[i] = tag.Tag
		}
		djangotest.AssertEqual(t, names, []string{"hunts", "roars"})
		hunts, roars := tags[0], tags[1]
		djangotest.AssertQuerySetEqual(t, tagsOf(db, lion).QuerySet().OrderBy("tag"),
			ids(hairy, hunts, roars, yellow), tagID)
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_absolute_max
func TestGenericInlineFormsetAbsoluteMax(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		formset := tagFormSet(db, 3)
		formset.AbsoluteMax = 1500
		formset.Prefix = "form"
		bound := formset.Bind(url.Values{
			"form-TOTAL_FORMS":   {"1501"},
			"form-INITIAL_FORMS": {"0"},
			"form-MAX_NUM_FORMS": {"0"},
		})
		djangotest.AssertEqual(t, bound.IsValid(), false)
		djangotest.AssertEqual(t, len(bound.Forms()), 1500)
		djangotest.AssertEqual(t, bound.NonFormErrors().Messages(), []string{"Please submit at most 1000 forms."})
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_absolute_max_with_max_num
func TestGenericInlineFormsetAbsoluteMaxWithMaxNum(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		formset := tagFormSet(db, 3)
		formset.MaxNum = 20
		formset.AbsoluteMax = 100
		formset.Prefix = "form"
		bound := formset.Bind(url.Values{
			"form-TOTAL_FORMS":   {"101"},
			"form-INITIAL_FORMS": {"0"},
			"form-MAX_NUM_FORMS": {"0"},
		})
		djangotest.AssertEqual(t, bound.IsValid(), false)
		djangotest.AssertEqual(t, len(bound.Forms()), 100)
		djangotest.AssertEqual(t, bound.NonFormErrors().Messages(), []string{"Please submit at most 20 forms."})
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_can_delete_extra
func TestGenericInlineFormsetCanDeleteExtra(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		formset := tagFormSet(db, 2)
		formset.CanDelete, formset.CanDeleteExtra = true, true
		djangotest.AssertEqual(t, len(formset.Forms()), 2)
		for _, form := range formset.Forms() {
			djangotest.AssertEqual(t, form.Field(forms.DeletionFieldName) != nil, true)
		}
	})
}

// Ported from Django: tests/generic_relations/test_forms.py::GenericInlineFormsetTests.test_disable_delete_extra
func TestGenericInlineFormsetDisableDeleteExtra(t *testing.T) {
	djangotest.EachDB(t, formsModels(), func(t *testing.T, db *orm.DB) {
		formset := tagFormSet(db, 2)
		formset.CanDelete, formset.CanDeleteExtra = true, false
		djangotest.AssertEqual(t, len(formset.Forms()), 2)
		for _, form := range formset.Forms() {
			djangotest.AssertEqual(t, form.Field(forms.DeletionFieldName) == nil, true)
		}
	})
}
