package forms_test

import (
	"context"
	"net/url"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// multiCategories creates the three categories of
// ModelMultipleChoiceFieldTests.setUpTestData.
func multiCategories(t *testing.T, db *orm.DB) (c1, c2, c3 *Category) {
	t.Helper()
	ctx := context.Background()
	c1 = &Category{Name: "Entertainment", Slug: "entertainment", URL: "entertainment"}
	c2 = &Category{Name: "It's a test", Slug: "its-test", URL: "test"}
	c3 = &Category{Name: "Third", Slug: "third-test", URL: "third"}
	for _, c := range []*Category{c1, c2, c3} {
		if err := orm.Query[Category](db).Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	return c1, c2, c3
}

// multiLoaded returns the field with its rows read from the database now, as
// Django's field reads its queryset on every clean.
func multiLoaded(t *testing.T, f *forms.ModelMultipleChoiceField) forms.Field {
	t.Helper()
	loaded, err := f.WithContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// multiOptions returns the (value, label) pairs of the rendered field, Django's
// list(f.choices).
func multiOptions(t *testing.T, f *forms.ModelMultipleChoiceField) [][2]string {
	t.Helper()
	form := forms.New(forms.F("c", f))
	form.AutoID = ""
	matches := choiceOptionRe.FindAllStringSubmatch(form.Field("c").String(), -1)
	out := make([][2]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, [2]string{m[1], m[2]})
	}
	return out
}

// cleanedNames cleans value and returns the names of the selected categories.
func cleanedNames(t *testing.T, f forms.Field, value any) []string {
	t.Helper()
	got, err := f.Clean(value)
	if err != nil {
		t.Errorf("Clean(%#v) failed: %v", value, err)
		return nil
	}
	cats, ok := got.([]*Category)
	if !ok {
		t.Errorf("Clean(%#v) = %#v, want a []*Category", value, got)
		return nil
	}
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
	}
	return names
}

// createWriters creates n writers named "Person 0" and up.
func createWriters(t *testing.T, db *orm.DB, n int) []*Writer {
	t.Helper()
	out := make([]*Writer, n)
	for i := range out {
		out[i] = &Writer{Name: "Person " + strconv.Itoa(i)}
		if err := orm.Query[Writer](db).Create(context.Background(), out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_model_multiple_choice_field
func TestModelMultipleChoiceField(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		c1, c2, c3 := multiCategories(t, db)
		f := forms.NewModelMultipleChoiceField(db, orm.Query[Category](db).OrderBy("id"))
		djangotest.AssertEqual(t, multiOptions(t, f), [][2]string{
			{choiceID(c1.ID), "Entertainment"},
			{choiceID(c2.ID), "It&#x27;s a test"},
			{choiceID(c3.ID), "Third"},
		})
		field := func() forms.Field { return multiLoaded(t, f) }
		rejects(t, field(), nil, "This field is required.")
		rejects(t, field(), []any{}, "This field is required.")
		equal(t, cleanedNames(t, field(), []int64{c1.ID}), []string{"Entertainment"})
		equal(t, cleanedNames(t, field(), []int64{c2.ID}), []string{"It's a test"})
		equal(t, cleanedNames(t, field(), []string{choiceID(c1.ID)}), []string{"Entertainment"})
		equal(t, cleanedNames(t, field(), []string{choiceID(c1.ID), choiceID(c2.ID)}),
			[]string{"Entertainment", "It's a test"})
		equal(t, cleanedNames(t, field(), []any{c1.ID, choiceID(c2.ID)}),
			[]string{"Entertainment", "It's a test"})
		equal(t, cleanedNames(t, field(), [2]any{c1.ID, choiceID(c2.ID)}),
			[]string{"Entertainment", "It's a test"})
		rejects(t, field(), []string{"0"}, "Select a valid choice. 0 is not one of the available choices.")
		rejects(t, field(), "hello", "Enter a list of values.")
		rejects(t, field(), []string{"fail"}, "“fail” is not a valid value.")

		// Invalid types that require TypeError to be caught (#22808).
		rejects(t, field(), []any{[]string{"fail"}}, "Enter a list of values.")
		rejects(t, field(), []any{map[string]string{"foo": "bar"}}, "Enter a list of values.")

		// A category added after the field exists is found: clean() checks
		// the database and does not cache at instantiation.
		c6 := &Category{ID: 1006, Name: "Sixth", URL: "6th"}
		if err := orm.Query[Category](db).Create(ctx, c6); err != nil {
			t.Fatal(err)
		}
		equal(t, cleanedNames(t, field(), []int64{c6.ID}), []string{"Sixth"})

		// A deleted category is no longer a valid choice.
		if _, err := orm.Query[Category](db).Filter("url", "6th").Delete(ctx); err != nil {
			t.Fatal(err)
		}
		rejects(t, field(), []int64{c6.ID}, "Select a valid choice. 1006 is not one of the available choices.")
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_model_multiple_choice_required_false
//
// Go cannot change the queryset or the label function of a field, so each
// queryset gets its own field, and the label function is an option.
func TestModelMultipleChoiceRequiredFalse(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		c1, c2, c3 := multiCategories(t, db)
		f := forms.NewModelMultipleChoiceField(db, orm.Query[Category](db).OrderBy("id"), forms.Required(false))
		cleans(t, multiLoaded(t, f), []any{}, []*Category{})
		cleans(t, multiLoaded(t, f), [0]any{}, []*Category{})
		invalid := "Select a valid choice. 0 is not one of the available choices."
		rejects(t, multiLoaded(t, f), []string{"0"}, invalid)
		rejects(t, multiLoaded(t, f), []string{choiceID(c3.ID), "0"}, invalid)
		rejects(t, multiLoaded(t, f), []string{choiceID(c1.ID), "0"}, invalid)

		// A field over another queryset.
		f = forms.NewModelMultipleChoiceField(db,
			orm.Query[Category](db).Exclude("name", "Third").OrderBy("id"), forms.Required(false))
		djangotest.AssertEqual(t, multiOptions(t, f), [][2]string{
			{choiceID(c1.ID), "Entertainment"},
			{choiceID(c2.ID), "It&#x27;s a test"},
		})
		equal(t, cleanedNames(t, multiLoaded(t, f), []int64{c2.ID}), []string{"It's a test"})
		third := "Select a valid choice. " + choiceID(c3.ID) + " is not one of the available choices."
		rejects(t, multiLoaded(t, f), []int64{c3.ID}, third)
		rejects(t, multiLoaded(t, f), []string{choiceID(c2.ID), choiceID(c3.ID)}, third)

		f = forms.NewModelMultipleChoiceField(db, orm.Query[Category](db), forms.Required(false),
			forms.LabelFromInstance(func(obj *Category) string { return "multicategory " + obj.String() }))
		djangotest.AssertSequenceEqual(t, multiOptions(t, f), [][2]string{
			{choiceID(c1.ID), "multicategory Entertainment"},
			{choiceID(c2.ID), "multicategory It&#x27;s a test"},
			{choiceID(c3.ID), "multicategory Third"},
		}, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_model_multiple_choice_number_of_queries
//
// The Go field reads its rows when it is loaded and cleans without a query,
// so the one query covers loading and cleaning.
func TestModelMultipleChoiceNumberOfQueries(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		persons := createWriters(t, db, 30)
		var pks []int64
		for i := 1; i < 11; i += 2 {
			pks = append(pks, persons[i].ID)
		}
		f := forms.NewModelMultipleChoiceField(db, orm.Query[Writer](db))
		djangotest.AssertNumQueries(t, db, 1, func() {
			if _, err := multiLoaded(t, f).Clean(pks); err != nil {
				t.Error(err)
			}
		})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_model_multiple_choice_null_characters
func TestModelMultipleChoiceNullCharacters(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		f := multiLoaded(t, forms.NewModelMultipleChoiceField(db, orm.Query[ExplicitPK](db)))
		msg := "Null characters are not allowed."
		rejects(t, f, []string{"\x00something"}, msg)
		rejects(t, f, []string{"valid", "\x00something"}, msg)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_model_multiple_choice_run_validators
func TestModelMultipleChoiceRunValidators(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		createWriters(t, db, 30)
		validatorRun := false
		myValidator := func(any) error {
			validatorRun = true
			return nil
		}
		f := forms.NewModelMultipleChoiceField(db, orm.Query[Writer](db), forms.Validators(myValidator))
		writers, err := orm.Query[Writer](db).Offset(8).Limit(1).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := multiLoaded(t, f).Clean([]int64{writers[0].ID}); err != nil {
			t.Fatal(err)
		}
		equal(t, validatorRun, true)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_model_multiple_choice_show_hidden_initial
//
// show_hidden_initial does not exist in the package, so the submitted value
// is compared with the form's initial value instead of the hidden one.
func TestModelMultipleChoiceShowHiddenInitial(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ws := createWriters(t, db, 3)
		person1, person2 := ws[1], ws[2]
		writerForm := func(data url.Values) *forms.Form {
			form := forms.New(forms.F("persons", forms.NewModelMultipleChoiceField(db, orm.Query[Writer](db))))
			form.Initial = map[string]any{"persons": []*Writer{person1, person2}}
			return form.Bind(data)
		}
		form := writerForm(url.Values{"persons": {choiceID(person1.ID), choiceID(person2.ID)}})
		equal(t, form.IsValid(), true)
		equal(t, form.HasChanged(), false)

		form = writerForm(url.Values{"persons": {choiceID(person2.ID)}})
		equal(t, form.IsValid(), true)
		equal(t, form.HasChanged(), true)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_model_multiple_choice_field_22745
//
// #22745 -- Make sure that ModelMultipleChoiceField with
// CheckboxSelectMultiple widget doesn't produce unnecessary db queries when
// accessing its BoundField's attrs.
func TestModelMultipleChoiceField22745(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		multiCategories(t, db)
		form := forms.New(forms.F("categories", forms.NewModelMultipleChoiceField(db, orm.Query[Category](db),
			forms.UseWidget(forms.CheckboxSelectMultiple{}))))
		field := form.Field("categories")
		djangotest.AssertNumQueries(t, db, 1, func() {
			renderTemplate(t, "{{ field.name }}{{ field }}{{ field.help_text }}", map[string]any{"field": field})
		})
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_show_hidden_initial_changed_queries_efficiently
func TestModelMultipleChoiceShowHiddenInitialChangedQueriesEfficiently(t *testing.T) {
	t.Skip("FIXME-PORT: Go: no show_hidden_initial, so a form has no initial-<name> input to compare with, Django: has_changed() reads the hidden initial value with one query")
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_clean_does_deduplicate_values
//
// Django reads the parameters of the returned queryset; the Go field returns
// the rows, so the test reads the parameters of the query that clean runs.
func TestModelMultipleChoiceCleanDoesDeduplicateValues(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		person1 := &Person{Name: "Person 1"}
		if err := orm.Query[Person](db).Create(context.Background(), person1); err != nil {
			t.Fatal(err)
		}
		form := forms.New(forms.F("persons", forms.NewModelMultipleChoiceField(db, orm.Query[Person](db)))).
			Bind(url.Values{})
		field := form.Field("persons").Field()
		keys := make([]string, 50)
		for i := range keys {
			keys[i] = choiceID(person1.ID)
		}
		qc := djangotest.CaptureQueries(db)
		_, err := field.Clean(keys)
		qc.Stop()
		if err != nil {
			t.Fatal(err)
		}
		queries := qc.Queries()
		if len(queries) != 1 {
			t.Fatalf("clean ran %d queries, want 1", len(queries))
		}
		equal(t, len(queries[0].Args), 1)
	})
}

// Ported from Django: tests/model_forms/tests.py::ModelMultipleChoiceFieldTests.test_to_field_name_with_initial_data
func TestModelMultipleChoiceToFieldNameWithInitialData(t *testing.T) {
	eachMF(t, func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, c2, c3 := multiCategories(t, db)
		writer := &Writer{Name: "Test writer"}
		if err := orm.Query[Writer](db).Create(ctx, writer); err != nil {
			t.Fatal(err)
		}
		article := &Article{
			Headline: "Test article", Slug: "test-article", PubDate: date(1988, 1, 4),
			Created: date(2024, 1, 1), Article: "Hello.",
		}
		article.Writer.SetPK(writer.ID)
		if err := orm.Query[Article](db).Create(ctx, article); err != nil {
			t.Fatal(err)
		}
		if err := orm.Many[Category](db, article, "categories").Add(ctx, c2, c3); err != nil {
			t.Fatal(err)
		}
		form := forms.NewModelForm[Article](db,
			forms.WithFields("categories"),
			forms.WithFormField("categories", forms.NewModelMultipleChoiceField(db, orm.Query[Category](db),
				forms.ToFieldName("slug"))),
		).WithInstance(article)
		value, _ := form.Field("categories").Value().([]string)
		djangotest.AssertSequenceEqual(t, value, []string{c2.Slug, c3.Slug}, djangotest.Unordered())
	})
}

// ChoiceModel is the ChoiceModel model of Django's tests/forms_tests/models.py,
// with only the name field the error message tests use.
type ChoiceModel struct {
	ID   int64
	Name string `orm:"max_length=10"`
}

// createChoiceModels creates the rows a, b and c with the primary keys 1 to 3.
func createChoiceModels(t *testing.T, db *orm.DB) {
	t.Helper()
	for i, name := range []string{"a", "b", "c"} {
		if err := orm.Query[ChoiceModel](db).Create(context.Background(), &ChoiceModel{ID: int64(i + 1), Name: name}); err != nil {
			t.Fatal(err)
		}
	}
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::ModelChoiceFieldErrorMessagesTestCase.test_modelchoicefield
func TestErrorMessagesModelchoicefield(t *testing.T) {
	djangotest.EachDB(t, []any{&ChoiceModel{}}, func(t *testing.T, db *orm.DB) {
		createChoiceModels(t, db)

		// ModelChoiceField
		e := map[string]string{
			"required":       "REQUIRED",
			"invalid_choice": "INVALID CHOICE",
		}
		single, err := forms.NewModelChoiceField(db, orm.Query[ChoiceModel](db), forms.ErrorMessages(e)).
			WithContext(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		rejects(t, single, "", "REQUIRED")
		rejects(t, single, "4", "INVALID CHOICE")

		// ModelMultipleChoiceField
		e = map[string]string{
			"required":       "REQUIRED",
			"invalid_choice": "%(value)s IS INVALID CHOICE",
			"invalid_list":   "NOT A LIST OF VALUES",
		}
		multi := multiLoaded(t, forms.NewModelMultipleChoiceField(db, orm.Query[ChoiceModel](db), forms.ErrorMessages(e)))
		rejects(t, multi, "", "REQUIRED")
		rejects(t, multi, "3", "NOT A LIST OF VALUES")
		rejects(t, multi, []string{"4"}, "4 IS INVALID CHOICE")
	})
}

// Ported from Django: tests/forms_tests/tests/test_error_messages.py::ModelChoiceFieldErrorMessagesTestCase.test_modelchoicefield_value_placeholder
func TestErrorMessagesModelchoicefieldValuePlaceholder(t *testing.T) {
	djangotest.EachDB(t, []any{&ChoiceModel{}}, func(t *testing.T, db *orm.DB) {
		f, err := forms.NewModelChoiceField(db, orm.Query[ChoiceModel](db), forms.ErrorMessages(map[string]string{
			"invalid_choice": `"%(value)s" is not one of the available choices.`,
		})).WithContext(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		rejects(t, f, "invalid", `"invalid" is not one of the available choices.`)
	})
}
