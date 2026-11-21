package forms_test

import (
	"context"
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// ChoiceCategory is the Category model of Django's tests/model_forms/models.py.
type ChoiceCategory struct {
	ID   int64
	Name string `orm:"max_length=20"`
	Slug string `orm:"max_length=20"`
	URL  string `orm:"max_length=40"`
}

// String is the label of a category in a choice, Django's __str__.
func (c ChoiceCategory) String() string { return c.Name }

// ChoiceWriter is the Writer model of Django's tests/model_forms/models.py.
type ChoiceWriter struct {
	ID   int64
	Name string `orm:"max_length=50"`
}

// String is the label of a writer in a choice, Django's __str__.
func (w ChoiceWriter) String() string { return w.Name }

// ChoiceAuthor is the Author model of Django's tests/model_forms/models.py,
// without its one-to-one relation to Publication.
type ChoiceAuthor struct {
	ID       int64
	FullName string `orm:"max_length=255"`
}

// ChoiceBook is the Book model of Django's tests/model_forms/models.py.
type ChoiceBook struct {
	ID     int64
	Title  string               `orm:"max_length=40"`
	Author orm.FK[ChoiceWriter] `orm:"null;on_delete=set_null"`
}

// choiceModels returns the models of the ported tests.
func choiceModels() []any {
	return []any{&ChoiceCategory{}, &ChoiceWriter{}, &ChoiceAuthor{}, &ChoiceBook{}}
}

// choiceCategories creates the three categories of the test case's
// setUpTestData.
func choiceCategories(t *testing.T, db *orm.DB) (c1, c2, c3 *ChoiceCategory) {
	t.Helper()
	ctx := context.Background()
	c1 = &ChoiceCategory{Name: "Entertainment", Slug: "entertainment", URL: "entertainment"}
	c2 = &ChoiceCategory{Name: "A test", Slug: "test", URL: "test"}
	c3 = &ChoiceCategory{Name: "Third", Slug: "third-test", URL: "third"}
	for _, c := range []*ChoiceCategory{c1, c2, c3} {
		if err := orm.Query[ChoiceCategory](db).Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	return c1, c2, c3
}

// choiceLoaded returns the field with its rows read from the database now, as
// Django's field reads its queryset on every use.
func choiceLoaded(t *testing.T, f *forms.ModelChoiceField) forms.Field {
	t.Helper()
	loaded, err := f.WithContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// choiceRender renders the field in a form without ids.
func choiceRender(t *testing.T, f *forms.ModelChoiceField) string {
	t.Helper()
	form := forms.New(forms.F("c", f))
	form.AutoID = ""
	return form.Field("c").String()
}

var (
	choiceOptionRe = regexp.MustCompile(`<option value="([^"]*)"[^>]*>([^<]*)</option>`)
	choiceValueRe  = regexp.MustCompile(`value="([^"]*)"`)
)

// choiceOptions returns the (value, label) pairs of a rendered select.
func choiceOptions(t *testing.T, f *forms.ModelChoiceField) [][2]string {
	t.Helper()
	matches := choiceOptionRe.FindAllStringSubmatch(choiceRender(t, f), -1)
	out := make([][2]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, [2]string{m[1], m[2]})
	}
	return out
}

// choiceValues returns the submitted values of the options of a rendered
// select or radio list.
func choiceValues(t *testing.T, f *forms.ModelChoiceField) []string {
	t.Helper()
	matches := choiceValueRe.FindAllStringSubmatch(choiceRender(t, f), -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// choiceID formats a primary key as a widget submits it.
func choiceID(id int64) string { return strconv.FormatInt(id, 10) }

// choiceBlank is the blank choice, get_blank_choice_label().
var choiceBlank = [2]string{"", "---------"}

// choiceAllCategories is the queryset Category.objects.all().
func choiceAllCategories(db *orm.DB) *orm.QuerySet[ChoiceCategory] {
	return orm.Query[ChoiceCategory](db).OrderBy("id")
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_basics
func TestModelChoiceFieldBasics(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		c1, c2, c3 := choiceCategories(t, db)
		f := forms.NewModelChoiceField(db, choiceAllCategories(db))
		djangotest.AssertEqual(t, choiceOptions(t, f), [][2]string{
			choiceBlank,
			{choiceID(c1.ID), "Entertainment"},
			{choiceID(c2.ID), "A test"},
			{choiceID(c3.ID), "Third"},
		})
		field := func() forms.Field { return choiceLoaded(t, f) }
		rejects(t, field(), "", "This field is required.")
		rejects(t, field(), nil, "This field is required.")
		invalid := "Select a valid choice. That choice is not one of the available choices."
		rejects(t, field(), 0, invalid)

		// Invalid types that require TypeError to be caught.
		rejects(t, field(), [][]string{{"fail"}}, invalid)
		rejects(t, field(), []map[string]string{{"foo": "bar"}}, invalid)

		got, err := field().Clean(c2.ID)
		if err != nil || got.(*ChoiceCategory).Name != "A test" {
			t.Errorf("Clean(c2.ID) = %#v, %v", got, err)
		}
		got, err = field().Clean(c3.ID)
		if err != nil || got.(*ChoiceCategory).Name != "Third" {
			t.Errorf("Clean(c3.ID) = %#v, %v", got, err)
		}

		// A category added after the field exists is found: clean() checks
		// the database and does not cache at instantiation.
		c4 := &ChoiceCategory{Name: "Fourth", URL: "4th"}
		if err := orm.Query[ChoiceCategory](db).Create(ctx, c4); err != nil {
			t.Fatal(err)
		}
		got, err = field().Clean(c4.ID)
		if err != nil || got.(*ChoiceCategory).Name != "Fourth" {
			t.Errorf("Clean(c4.ID) = %#v, %v", got, err)
		}

		// A deleted category is no longer a valid choice.
		if _, err := orm.Query[ChoiceCategory](db).Filter("url", "4th").Delete(ctx); err != nil {
			t.Fatal(err)
		}
		rejects(t, field(), c4.ID, invalid)
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_clean_model_instance
func TestModelChoiceFieldCleanModelInstance(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		c1, _, _ := choiceCategories(t, db)
		f := choiceLoaded(t, forms.NewModelChoiceField(db, choiceAllCategories(db)))
		got, err := f.Clean(c1)
		if err != nil {
			t.Fatal(err)
		}
		equal(t, *got.(*ChoiceCategory), *c1)

		// An instance of an incorrect model.
		book := &ChoiceBook{}
		if err := orm.Query[ChoiceBook](db).Create(context.Background(), book); err != nil {
			t.Fatal(err)
		}
		rejects(t, f, book, "Select a valid choice. That choice is not one of the available choices.")
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_clean_to_field_name
func TestModelChoiceFieldCleanToFieldName(t *testing.T) {
	t.Skip("FIXME-PORT: Go: ModelChoiceField has no to_field_name, a row is selected by primary key only, Django: to_field_name=\"slug\" selects by slug")
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_model_choice_null_characters
func TestModelChoiceFieldNullCharacters(t *testing.T) {
	djangotest.EachDB(t, []any{&ExplicitPK{}}, func(t *testing.T, db *orm.DB) {
		f := choiceLoaded(t, forms.NewModelChoiceField(db, orm.Query[ExplicitPK](db)))
		rejects(t, f, "\x00something", "Null characters are not allowed.")
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_choices
//
// Go cannot change the queryset of a field, so each queryset gets its own
// field.
func TestModelChoiceFieldChoices(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		c1, c2, c3 := choiceCategories(t, db)
		invalid := "Select a valid choice. That choice is not one of the available choices."
		f := forms.NewModelChoiceField(db, orm.Query[ChoiceCategory](db).Filter("id", c1.ID), forms.Required(false))
		cleans(t, choiceLoaded(t, f), "", nil)
		got, err := choiceLoaded(t, f).Clean(choiceID(c1.ID))
		if err != nil || got.(*ChoiceCategory).Name != "Entertainment" {
			t.Errorf("Clean(c1) = %#v, %v", got, err)
		}
		rejects(t, choiceLoaded(t, f), "100", invalid)

		// The choices are the blank choice and one category.
		equal(t, len(choiceOptions(t, f)), 2)

		f = forms.NewModelChoiceField(db,
			orm.Query[ChoiceCategory](db).Exclude("name", "Third").OrderBy("id"), forms.Required(false))
		want := [][2]string{choiceBlank, {choiceID(c1.ID), "Entertainment"}, {choiceID(c2.ID), "A test"}}
		djangotest.AssertEqual(t, choiceOptions(t, f), want)
		got, err = choiceLoaded(t, f).Clean(c2.ID)
		if err != nil || got.(*ChoiceCategory).Name != "A test" {
			t.Errorf("Clean(c2) = %#v, %v", got, err)
		}
		rejects(t, choiceLoaded(t, f), c3.ID, invalid)

		// The choices can be rendered repeatedly.
		djangotest.AssertEqual(t, choiceOptions(t, f), want)
		djangotest.AssertEqual(t, choiceOptions(t, f), want)

		t.Skip("FIXME-PORT: Go: a label is the model's String() and cannot be replaced per field (no label_from_instance), Django: f.label_from_instance = lambda obj: \"category \" + str(obj)")
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_choices_freshness
func TestModelChoiceFieldChoicesFreshness(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		c1, c2, c3 := choiceCategories(t, db)
		f := forms.NewModelChoiceField(db, choiceAllCategories(db))
		want := make([][2]string, 0, 5)
		want = append(want, choiceBlank,
			[2]string{choiceID(c1.ID), "Entertainment"},
			[2]string{choiceID(c2.ID), "A test"},
			[2]string{choiceID(c3.ID), "Third"})
		equal(t, len(choiceOptions(t, f)), 4)
		djangotest.AssertEqual(t, choiceOptions(t, f), want)
		c4 := &ChoiceCategory{Name: "Fourth", Slug: "4th", URL: "4th"}
		if err := orm.Query[ChoiceCategory](db).Create(context.Background(), c4); err != nil {
			t.Fatal(err)
		}
		equal(t, len(choiceOptions(t, f)), 5)
		djangotest.AssertEqual(t, choiceOptions(t, f), append(want, [2]string{choiceID(c4.ID), "Fourth"}))
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_choices_bool
//
// Django tests bool(f.choices); Go tests whether the field renders an option.
func TestModelChoiceFieldChoicesBool(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		choiceCategories(t, db)
		f := forms.NewModelChoiceField(db, choiceAllCategories(db), forms.NoEmptyLabel())
		equal(t, len(choiceOptions(t, f)) > 0, true)
		if _, err := orm.Query[ChoiceCategory](db).Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		equal(t, len(choiceOptions(t, f)) > 0, false)
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_choices_bool_empty_label
func TestModelChoiceFieldChoicesBoolEmptyLabel(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		choiceCategories(t, db)
		f := forms.NewModelChoiceField(db, choiceAllCategories(db), forms.EmptyLabel("--------"))
		if _, err := orm.Query[ChoiceCategory](db).Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		equal(t, len(choiceOptions(t, f)) > 0, true)
	})
}

// choiceRadioRe matches one choice of a rendered RadioSelect.
var choiceRadioRe = regexp.MustCompile(`<input type="radio" name="c" value="([^"]*)"[^>]*> ([^<]*)</label>`)

// choiceRadios returns the (value, label) pairs of a rendered radio list.
func choiceRadios(t *testing.T, f *forms.ModelChoiceField) [][2]string {
	t.Helper()
	matches := choiceRadioRe.FindAllStringSubmatch(choiceRender(t, f), -1)
	out := make([][2]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, [2]string{m[1], m[2]})
	}
	return out
}

// Django passes the widget as a class and as an instance; Go has only the
// instance. The blank argument is the Blank field.
//
// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_choices_radio_blank
func TestModelChoiceFieldChoicesRadioBlank(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		c1, c2, c3 := choiceCategories(t, db)
		choices := [][2]string{
			{choiceID(c1.ID), "Entertainment"},
			{choiceID(c2.ID), "A test"},
			{choiceID(c3.ID), "Third"},
		}
		for _, blank := range []bool{true, false} {
			f := forms.NewModelChoiceField(db, choiceAllCategories(db), forms.UseWidget(forms.RadioSelect{}))
			f.Blank = blank
			want := choices
			if blank {
				want = append([][2]string{choiceBlank}, choices...)
			}
			if got := choiceRadios(t, f); !reflect.DeepEqual(got, want) {
				t.Errorf("blank=%v: choices %v, want %v", blank, got, want)
			}
		}
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_deepcopies_widget
func TestModelChoiceFieldDeepcopiesWidget(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a form holds no deep-copied fields and the widget has no choices.field back reference, Django: field1.widget.choices.field is field1 and not the base field")
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_result_cache_not_shared
func TestModelChoiceFieldResultCacheNotShared(t *testing.T) {
	t.Skip("FIXME-PORT: Go: a queryset has no _result_cache to inspect, Django: the second form's queryset has an empty result cache")
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_queryset_none
func TestModelChoiceFieldQuerysetNone(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		_, c2, c3 := choiceCategories(t, db)
		// The field is created with the queryset set later in Django's
		// form constructor; Go passes the queryset when it builds the field.
		f := forms.NewModelChoiceField(db, orm.Query[ChoiceCategory](db).Filter("slug__contains", "test").OrderBy("id"))
		djangotest.AssertEqual(t, choiceValues(t, f), []string{"", choiceID(c2.ID), choiceID(c3.ID)})
		t.Skip("FIXME-PORT: Go: NewModelChoiceField(db, nil) means every row and the queryset of a field cannot be assigned afterwards, Django: ModelChoiceField(queryset=None) takes its queryset from the form constructor")
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_no_extra_query_when_accessing_attrs
func TestModelChoiceFieldNoExtraQueryWhenAccessingAttrs(t *testing.T) {
	t.Skip("FIXME-PORT: Go: the ORM has no query counter and a rendered field loads its rows once per WithContext, Django: assertNumQueries(1) around a template render")
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_disabled_modelchoicefield
func TestModelChoiceFieldDisabled(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		writer := &ChoiceWriter{Name: "Test writer"}
		if err := orm.Query[ChoiceWriter](db).Create(ctx, writer); err != nil {
			t.Fatal(err)
		}
		book := &ChoiceBook{}
		book.Author.SetPK(writer.ID)
		if err := orm.Query[ChoiceBook](db).Create(ctx, book); err != nil {
			t.Fatal(err)
		}
		form := forms.NewModelForm[ChoiceBook](db,
			forms.WithFields("author"),
			forms.WithFormField("author", forms.NewModelChoiceField(db, orm.Query[ChoiceAuthor](db), forms.Disabled())),
		).WithInstance(book).Bind(vals())
		equal(t, errorsOf(form.Form, "author"),
			[]string{"Select a valid choice. That choice is not one of the available choices."})
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_disabled_modelchoicefield_has_changed
func TestModelChoiceFieldDisabledHasChanged(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		f := forms.NewModelChoiceField(db, orm.Query[ChoiceAuthor](db), forms.Disabled())
		equal(t, f.HasChanged("x", "y"), false)
	})
}

// Ported from Django: tests/model_forms/test_modelchoicefield.py::ModelChoiceFieldTests.test_disabled_modelchoicefield_initial_model_instance
func TestModelChoiceFieldDisabledInitialModelInstance(t *testing.T) {
	djangotest.EachDB(t, choiceModels(), func(t *testing.T, db *orm.DB) {
		c1, _, _ := choiceCategories(t, db)
		form := forms.New(forms.F("categories",
			forms.NewModelChoiceField(db, choiceAllCategories(db), forms.Disabled(), forms.Initial(c1))))
		equal(t, form.Bind(vals("categories", choiceID(c1.ID))).IsValid(), true)
	})
}
