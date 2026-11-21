package forms_test

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/forms"
)

// choiceForm is the Choice form of Django's formset tests.
func choiceForm() *forms.Form {
	return forms.New(
		forms.F("choice", forms.NewCharField()),
		forms.F("votes", forms.NewIntegerField()),
	)
}

// favoriteDrinkForm is the FavoriteDrinkForm of Django's formset tests.
func favoriteDrinkForm() *forms.Form {
	return forms.New(forms.F("name", forms.NewCharField()))
}

// articleFormsetForm is the ArticleForm of Django's formset tests.
func articleFormsetForm() *forms.Form {
	return forms.New(forms.F("title", forms.NewCharField()), forms.F("pub_date", forms.NewDateField()))
}

// favoriteDrinksFormSet is the FavoriteDrinksFormSet of Django's formset
// tests: three extra forms, and an error when a drink is named twice.
func favoriteDrinksFormSet() *forms.FormSet {
	fs := forms.NewFormSet(favoriteDrinkForm)
	fs.Extra = 3
	return fs.CleanWith(func(fs *forms.FormSet) error {
		seen := map[any]bool{}
		for _, drink := range fs.CleanedData() {
			name, ok := drink["name"]
			if !ok {
				continue
			}
			if seen[name] {
				return forms.NewValidationError("You may only specify a drink once.", "")
			}
			seen[name] = true
		}
		return nil
	})
}

// choiceFormSet returns a formset of Choice forms with the prefix "choices"
// and no ids, the defaults of Django's make_choiceformset.
func choiceFormSet() *forms.FormSet {
	fs := forms.NewFormSet(choiceForm)
	fs.Prefix, fs.AutoID = "choices", ""
	return fs
}

// choiceData returns the submitted data of a choices formset from (choice,
// votes) pairs, with initialForms initial forms, Django's
// make_choiceformset.
func choiceData(pairs [][2]string, initialForms int) url.Values {
	data := vals(
		"choices-TOTAL_FORMS", strconv.Itoa(len(pairs)),
		"choices-INITIAL_FORMS", strconv.Itoa(initialForms),
		"choices-MAX_NUM_FORMS", "0",
		"choices-MIN_NUM_FORMS", "0",
	)
	for i, p := range pairs {
		data.Set(fmt.Sprintf("choices-%d-choice", i), p[0])
		data.Set(fmt.Sprintf("choices-%d-votes", i), p[1])
	}
	return data
}

// makeChoiceFormSet binds a choices formset to the data of the pairs.
func makeChoiceFormSet(pairs ...[2]string) *forms.FormSet {
	return choiceFormSet().Bind(choiceData(pairs, 0))
}

// cleanedOf returns the cleaned data of each form.
func cleanedOf(list []*forms.Form) []map[string]any {
	out := make([]map[string]any, len(list))
	for i, f := range list {
		out[i] = f.CleanedData()
	}
	return out
}

// formsetErrors returns the error messages of each form of a formset.
func formsetErrors(fs *forms.FormSet) []map[string][]string {
	errs := fs.Errors()
	out := make([]map[string][]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Messages())
	}
	return out
}

// joinForms renders each form with the layout and joins them with newlines.
func joinForms(list []*forms.Form, layout func(*forms.Form) string) string {
	parts := make([]string, len(list))
	for i, f := range list {
		parts[i] = layout(f)
	}
	return strings.Join(parts, "\n")
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_basic_formset
func TestBasicFormset(t *testing.T) {
	assertHTMLEqual(t, choiceFormSet().String(), `<input type="hidden" name="choices-TOTAL_FORMS" value="1">
<input type="hidden" name="choices-INITIAL_FORMS" value="0">
<input type="hidden" name="choices-MIN_NUM_FORMS" value="0">
<input type="hidden" name="choices-MAX_NUM_FORMS" value="1000">
<div>Choice:<input type="text" name="choices-0-choice"></div>
<div>Votes:<input type="number" name="choices-0-votes"></div>`)
	// A formset validates like a form; its cleaned data is a list with one
	// entry per form.
	fs := makeChoiceFormSet([2]string{"Calexico", "100"})
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, cleanedOf(fs.Forms()), []map[string]any{{"votes": int64(100), "choice": "Calexico"}})
	// A formset without data is neither valid nor changed.
	fs = choiceFormSet()
	if fs.IsValid() || fs.HasChanged() {
		t.Error("an unbound formset is valid or changed")
	}
}

// test_formset_name has no Go counterpart: formset_factory names the class
// it creates, and a Go formset is a value, not a class.

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_form_kwargs_formset
//
// A Go form has no custom constructor arguments; ConfigureForm is the hook
// that form_kwargs feeds, and the test records what it passes.
func TestFormKwargsFormset(t *testing.T) {
	custom := map[*forms.Form]any{}
	fs := forms.NewFormSet(choiceForm)
	fs.Extra = 2
	fs.ConfigureForm = func(f *forms.Form, _ int) { custom[f] = 1 }
	if len(fs.Forms()) != 2 {
		t.Fatalf("got %d forms, want 2", len(fs.Forms()))
	}
	for _, f := range fs.Forms() {
		equal(t, custom[f], 1)
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_form_kwargs_formset_dynamic
func TestFormKwargsFormsetDynamic(t *testing.T) {
	custom := map[*forms.Form]any{}
	fs := forms.NewFormSet(choiceForm)
	fs.Extra = 2
	fs.ConfigureForm = func(f *forms.Form, index int) { custom[f] = index }
	for i, f := range fs.Forms() {
		equal(t, custom[f], i)
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_form_kwargs_empty_form
func TestFormKwargsEmptyForm(t *testing.T) {
	custom := map[*forms.Form]any{}
	fs := forms.NewFormSet(choiceForm)
	fs.ConfigureForm = func(f *forms.Form, _ int) { custom[f] = 1 }
	empty := fs.EmptyForm()
	equal(t, custom[empty], 1)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_empty_permitted_ignored_empty_form
func TestEmptyPermittedIgnoredEmptyForm(t *testing.T) {
	fs := forms.NewFormSet(articleFormsetForm)
	fs.ConfigureForm = func(f *forms.Form, _ int) { f.EmptyPermitted = false }
	if !fs.EmptyForm().EmptyPermitted {
		t.Error("the empty form is not empty permitted")
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validation
func TestFormsetValidation(t *testing.T) {
	fs := makeChoiceFormSet([2]string{"Calexico", ""})
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, formsetErrors(fs), []map[string][]string{{"votes": {"This field is required."}}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validation_count
//
// Django counts the calls through mocks of ManagementForm.is_valid and
// BaseForm.full_clean. Go has no such mocks; a clean function on each form
// counts how often the forms are cleaned.
func TestFormsetValidationCount(t *testing.T) {
	cleans := 0
	fs := forms.NewFormSet(func() *forms.Form {
		return choiceForm().CleanWith(func(*forms.Form) error {
			cleans++
			return nil
		})
	})
	fs.Prefix, fs.AutoID = "choices", ""
	fs = fs.Bind(choiceData([][2]string{{"Calexico", "100"}, {"Any1", "42"}, {"Any2", "101"}}, 0))
	// Validating twice cleans each form once.
	fs.IsValid()
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, cleans, 3)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_has_changed
func TestFormsetHasChanged(t *testing.T) {
	if makeChoiceFormSet([2]string{"", ""}).HasChanged() {
		t.Error("a blank formset has changed")
	}
	invalid := makeChoiceFormSet([2]string{"Calexico", ""})
	if invalid.IsValid() || !invalid.HasChanged() {
		t.Error("the invalid formset is valid or unchanged")
	}
	valid := makeChoiceFormSet([2]string{"Calexico", "100"})
	if !valid.IsValid() || !valid.HasChanged() {
		t.Error("the valid formset is invalid or unchanged")
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_initial_data
func TestFormsetInitialData(t *testing.T) {
	fs := choiceFormSet()
	fs.Initial = []map[string]any{{"choice": "Calexico", "votes": 100}}
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL),
		`<li>Choice: <input type="text" name="choices-0-choice" value="Calexico"></li>`+
			`<li>Votes: <input type="number" name="choices-0-votes" value="100"></li>`+
			`<li>Choice: <input type="text" name="choices-1-choice"></li>`+
			`<li>Votes: <input type="number" name="choices-1-votes"></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_blank_form_unfilled
func TestBlankFormUnfilled(t *testing.T) {
	fs := choiceFormSet().Bind(choiceData([][2]string{{"Calexico", "100"}, {"", ""}}, 1))
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, cleanedOf(fs.Forms()), []map[string]any{{"votes": int64(100), "choice": "Calexico"}, {}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_second_form_partially_filled
func TestSecondFormPartiallyFilled(t *testing.T) {
	fs := choiceFormSet().Bind(choiceData([][2]string{{"Calexico", "100"}, {"The Decemberists", ""}}, 1))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, formsetErrors(fs), []map[string][]string{{}, {"votes": {"This field is required."}}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_delete_prefilled_data
func TestDeletePrefilledData(t *testing.T) {
	fs := choiceFormSet().Bind(choiceData([][2]string{{"", ""}, {"", ""}}, 1))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, formsetErrors(fs), []map[string][]string{
		{"votes": {"This field is required."}, "choice": {"This field is required."}},
		{},
	})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_displaying_more_than_one_blank_form
func TestDisplayingMoreThanOneBlankForm(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra = 3
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL), `<li>Choice: <input type="text" name="choices-0-choice"></li>
<li>Votes: <input type="number" name="choices-0-votes"></li>
<li>Choice: <input type="text" name="choices-1-choice"></li>
<li>Votes: <input type="number" name="choices-1-votes"></li>
<li>Choice: <input type="text" name="choices-2-choice"></li>
<li>Votes: <input type="number" name="choices-2-votes"></li>`)
	// Every form was shown blank, so each may be submitted blank; MinNum
	// requires a number of filled-in forms.
	bound := fs.Bind(choiceData([][2]string{{"", ""}, {"", ""}, {"", ""}}, 0))
	if !bound.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(bound))
	}
	equal(t, cleanedOf(bound.Forms()), []map[string]any{{}, {}, {}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_min_num_displaying_more_than_one_blank_form
func TestMinNumDisplayingMoreThanOneBlankForm(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra, fs.MinNum = 1, 1
	list := fs.Forms()
	// The MinNum forms are required; the extra forms may stay empty.
	if list[0].EmptyPermitted || !list[1].EmptyPermitted {
		t.Errorf("EmptyPermitted = %v, %v, want false, true", list[0].EmptyPermitted, list[1].EmptyPermitted)
	}
	assertHTMLEqual(t, joinForms(list, (*forms.Form).AsUL), `<li>Choice: <input type="text" name="choices-0-choice"></li>
<li>Votes: <input type="number" name="choices-0-votes"></li>
<li>Choice: <input type="text" name="choices-1-choice"></li>
<li>Votes: <input type="number" name="choices-1-votes"></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_min_num_displaying_more_than_one_blank_form_with_zero_extra
func TestMinNumDisplayingMoreThanOneBlankFormWithZeroExtra(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra, fs.MinNum = 0, 3
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL), `<li>Choice: <input type="text" name="choices-0-choice"></li>
<li>Votes: <input type="number" name="choices-0-votes"></li>
<li>Choice: <input type="text" name="choices-1-choice"></li>
<li>Votes: <input type="number" name="choices-1-votes"></li>
<li>Choice: <input type="text" name="choices-2-choice"></li>
<li>Votes: <input type="number" name="choices-2-votes"></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_single_form_completed
func TestSingleFormCompleted(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra = 3
	fs = fs.Bind(choiceData([][2]string{{"Calexico", "100"}, {"", ""}, {"", ""}}, 0))
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, cleanedOf(fs.Forms()), []map[string]any{{"votes": int64(100), "choice": "Calexico"}, {}, {}})
}

// twoChoices is the data of two filled-in choices with MAX_NUM_FORMS 2, which
// the server ignores.
func twoChoices() url.Values {
	data := choiceData([][2]string{{"Zero", "0"}, {"One", "1"}}, 0)
	data.Set("choices-MAX_NUM_FORMS", "2")
	return data
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validate_max_flag
func TestFormsetValidateMaxFlag(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra, fs.MaxNum, fs.ValidateMax = 1, 1, true
	fs = fs.Bind(twoChoices())
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"Please submit at most 1 form."})
	equal(t, fs.NonFormErrors().String(), `<ul class="errorlist nonform"><li>Please submit at most 1 form.</li></ul>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validate_max_flag_custom_error
func TestFormsetValidateMaxFlagCustomError(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra, fs.MaxNum, fs.ValidateMax = 1, 1, true
	fs.ErrorMessages = map[string]string{"too_many_forms": "Number of submitted forms should be at most %(num)d."}
	fs = fs.Bind(twoChoices())
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"Number of submitted forms should be at most 1."})
	equal(t, fs.NonFormErrors().String(),
		`<ul class="errorlist nonform"><li>Number of submitted forms should be at most 1.</li></ul>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validate_min_flag
func TestFormsetValidateMinFlag(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra, fs.MinNum, fs.ValidateMin = 1, 3, true
	fs = fs.Bind(choiceData([][2]string{{"Zero", "0"}, {"One", "1"}}, 0))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"Please submit at least 3 forms."})
	equal(t, fs.NonFormErrors().String(), `<ul class="errorlist nonform"><li>Please submit at least 3 forms.</li></ul>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validate_min_flag_custom_formatted_error
func TestFormsetValidateMinFlagCustomFormattedError(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra, fs.MinNum, fs.ValidateMin = 1, 3, true
	fs.ErrorMessages = map[string]string{"too_few_forms": "Number of submitted forms should be at least %(num)d."}
	fs = fs.Bind(choiceData([][2]string{{"Zero", "0"}, {"One", "1"}}, 0))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"Number of submitted forms should be at least 3."})
	equal(t, fs.NonFormErrors().String(),
		`<ul class="errorlist nonform"><li>Number of submitted forms should be at least 3.</li></ul>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validate_min_unchanged_forms
func TestFormsetValidateMinUnchangedForms(t *testing.T) {
	fs := choiceFormSet()
	fs.MinNum, fs.ValidateMin = 2, true
	fs.Initial = []map[string]any{{"choice": "Zero", "votes": 0}, {"choice": "One", "votes": 0}}
	data := choiceData([][2]string{{"Zero", "0"}, {"One", "1"}}, 2) // votes of One changed
	data.Set("choices-MAX_NUM_FORMS", "2")
	fs = fs.Bind(data)
	if fs.Forms()[0].HasChanged() || !fs.Forms()[1].HasChanged() {
		t.Error("HasChanged of the forms is not false, true")
	}
	if !fs.IsValid() {
		t.Errorf("formset not valid: %v", fs.NonFormErrors().Messages())
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_validate_min_excludes_empty_forms
func TestFormsetValidateMinExcludesEmptyForms(t *testing.T) {
	fs := forms.NewFormSet(choiceForm)
	fs.Prefix = "choices"
	fs.Extra, fs.MinNum, fs.ValidateMin, fs.CanDelete = 2, 1, true, true
	fs = fs.Bind(vals("choices-TOTAL_FORMS", "2", "choices-INITIAL_FORMS", "0"))
	if fs.HasChanged() {
		t.Error("formset has changed")
	}
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"Please submit at least 1 form."})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_second_form_partially_filled_2
func TestSecondFormPartiallyFilled2(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra = 3
	fs = fs.Bind(choiceData([][2]string{{"Calexico", "100"}, {"The Decemberists", ""}, {"", ""}}, 0))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, formsetErrors(fs), []map[string][]string{{}, {"votes": {"This field is required."}}, {}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_more_initial_data
func TestMoreInitialData(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra = 3
	fs.Initial = []map[string]any{{"choice": "Calexico", "votes": 100}}
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL),
		`<li>Choice: <input type="text" name="choices-0-choice" value="Calexico"></li>`+
			`<li>Votes: <input type="number" name="choices-0-votes" value="100"></li>`+
			`<li>Choice: <input type="text" name="choices-1-choice"></li>`+
			`<li>Votes: <input type="number" name="choices-1-votes"></li>`+
			`<li>Choice: <input type="text" name="choices-2-choice"></li>`+
			`<li>Votes: <input type="number" name="choices-2-votes"></li>`+
			`<li>Choice: <input type="text" name="choices-3-choice"></li>`+
			`<li>Votes: <input type="number" name="choices-3-votes"></li>`)
	empty := fs.EmptyForm()
	if !empty.EmptyPermitted {
		t.Error("the empty form is not empty permitted")
	}
	assertHTMLEqual(t, empty.AsUL(), `<li>Choice: <input type="text" name="choices-__prefix__-choice"></li>
<li>Votes: <input type="number" name="choices-__prefix__-votes"></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_with_deletion
func TestFormsetWithDeletion(t *testing.T) {
	fs := choiceFormSet()
	fs.CanDelete = true
	fs.Initial = []map[string]any{{"choice": "Calexico", "votes": 100}, {"choice": "Fergie", "votes": 900}}
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL),
		`<li>Choice: <input type="text" name="choices-0-choice" value="Calexico"></li>`+
			`<li>Votes: <input type="number" name="choices-0-votes" value="100"></li>`+
			`<li>Delete: <input type="checkbox" name="choices-0-DELETE"></li>`+
			`<li>Choice: <input type="text" name="choices-1-choice" value="Fergie"></li>`+
			`<li>Votes: <input type="number" name="choices-1-votes" value="900"></li>`+
			`<li>Delete: <input type="checkbox" name="choices-1-DELETE"></li>`+
			`<li>Choice: <input type="text" name="choices-2-choice"></li>`+
			`<li>Votes: <input type="number" name="choices-2-votes"></li>`+
			`<li>Delete: <input type="checkbox" name="choices-2-DELETE"></li>`)
	// To delete a form, its DELETE field is set to "on"; this deletes Fergie.
	data := choiceData([][2]string{{"Calexico", "100"}, {"Fergie", "900"}, {"", ""}}, 2)
	data.Set("choices-0-DELETE", "")
	data.Set("choices-1-DELETE", "on")
	data.Set("choices-2-DELETE", "")
	bound := fs.Bind(data)
	if !bound.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(bound))
	}
	equal(t, cleanedOf(bound.Forms()), []map[string]any{
		{"votes": int64(100), "DELETE": false, "choice": "Calexico"},
		{"votes": int64(900), "DELETE": true, "choice": "Fergie"},
		{},
	})
	equal(t, cleanedOf(bound.DeletedForms()), []map[string]any{
		{"votes": int64(900), "DELETE": true, "choice": "Fergie"},
	})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_with_deletion_remove_deletion_flag
func TestFormsetWithDeletionRemoveDeletionFlag(t *testing.T) {
	checkForm := func() *forms.Form {
		return forms.New(forms.F("field", forms.NewIntegerField(forms.MinValue(100))))
	}
	data := vals(
		"check-TOTAL_FORMS", "3",
		"check-INITIAL_FORMS", "2",
		"choices-MIN_NUM_FORMS", "0",
		"check-MAX_NUM_FORMS", "0",
		"check-0-field", "200",
		"check-0-DELETE", "",
		"check-1-field", "50",
		"check-1-DELETE", "on",
		"check-2-field", "",
		"check-2-DELETE", "",
	)
	fs := forms.NewFormSet(checkForm)
	fs.CanDelete, fs.Prefix = true, "check"
	if bound := fs.Bind(data); !bound.IsValid() {
		t.Errorf("formset not valid: %v", formsetErrors(bound))
	}
	// Without the deletion flag the form is validated.
	data.Set("check-1-DELETE", "")
	if fs.Bind(data).IsValid() {
		t.Error("formset is valid")
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_with_deletion_invalid_deleted_form
func TestFormsetWithDeletionInvalidDeletedForm(t *testing.T) {
	fs := forms.NewFormSet(favoriteDrinkForm)
	fs.CanDelete = true
	fs = fs.Bind(vals(
		"form-0-name", "",
		"form-0-DELETE", "on", // no name
		"form-TOTAL_FORMS", "1",
		"form-INITIAL_FORMS", "1",
		"form-MIN_NUM_FORMS", "0",
		"form-MAX_NUM_FORMS", "1",
	))
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, formsetErrors(fs), []map[string][]string{})
	equal(t, len(fs.DeletedForms()), 1)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_with_deletion_custom_widget
//
// Django sets the widget with a class attribute or a method; a Go formset
// has the field DeletionWidget.
func TestFormsetWithDeletionCustomWidget(t *testing.T) {
	tests := []struct {
		name   string
		widget forms.Widget
		html   string
	}{
		{"attribute", forms.HiddenInput{}, `<input type="hidden" name="form-0-DELETE">`},
		{"method", forms.HiddenInput{Attrs: forms.Attrs{"class": "deletion"}}, `<input class="deletion" type="hidden" name="form-0-DELETE">`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := forms.NewFormSet(articleFormsetForm)
			fs.CanDelete, fs.AutoID, fs.DeletionWidget = true, "", tt.widget
			assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL),
				`<li>Title: <input type="text" name="form-0-title"></li>`+
					`<li>Pub date: <input type="text" name="form-0-pub_date">`+tt.html+`</li>`)
		})
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formsets_with_ordering
func TestFormsetsWithOrdering(t *testing.T) {
	fs := choiceFormSet()
	fs.CanOrder = true
	fs.Initial = []map[string]any{{"choice": "Calexico", "votes": 100}, {"choice": "Fergie", "votes": 900}}
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL),
		`<li>Choice: <input type="text" name="choices-0-choice" value="Calexico"></li>`+
			`<li>Votes: <input type="number" name="choices-0-votes" value="100"></li>`+
			`<li>Order: <input type="number" name="choices-0-ORDER" value="1"></li>`+
			`<li>Choice: <input type="text" name="choices-1-choice" value="Fergie"></li>`+
			`<li>Votes: <input type="number" name="choices-1-votes" value="900"></li>`+
			`<li>Order: <input type="number" name="choices-1-ORDER" value="2"></li>`+
			`<li>Choice: <input type="text" name="choices-2-choice"></li>`+
			`<li>Votes: <input type="number" name="choices-2-votes"></li>`+
			`<li>Order: <input type="number" name="choices-2-ORDER"></li>`)
	data := choiceData([][2]string{{"Calexico", "100"}, {"Fergie", "900"}, {"The Decemberists", "500"}}, 2)
	data.Set("choices-0-ORDER", "1")
	data.Set("choices-1-ORDER", "2")
	data.Set("choices-2-ORDER", "0")
	bound := fs.Bind(data)
	if !bound.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(bound))
	}
	equal(t, cleanedOf(bound.OrderedForms()), []map[string]any{
		{"votes": int64(500), "ORDER": int64(0), "choice": "The Decemberists"},
		{"votes": int64(100), "ORDER": int64(1), "choice": "Calexico"},
		{"votes": int64(900), "ORDER": int64(2), "choice": "Fergie"},
	})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formsets_with_ordering_custom_widget
//
// Django sets the widget with a class attribute or a method; a Go formset
// has the field OrderingWidget.
func TestFormsetsWithOrderingCustomWidget(t *testing.T) {
	tests := []struct {
		name   string
		widget forms.Widget
		html   string
	}{
		{"attribute", forms.HiddenInput{}, `<input type="hidden" name="form-0-ORDER">`},
		{"method", forms.HiddenInput{Attrs: forms.Attrs{"class": "ordering"}}, `<input class="ordering" type="hidden" name="form-0-ORDER">`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := forms.NewFormSet(articleFormsetForm)
			fs.CanOrder, fs.AutoID, fs.OrderingWidget = true, "", tt.widget
			assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL),
				`<li>Title: <input type="text" name="form-0-title"></li>`+
					`<li>Pub date: <input type="text" name="form-0-pub_date">`+tt.html+`</li>`)
		})
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_empty_ordered_fields
func TestEmptyOrderedFields(t *testing.T) {
	data := choiceData([][2]string{
		{"Calexico", "100"}, {"Fergie", "900"}, {"The Decemberists", "500"}, {"Basia Bulat", "50"},
	}, 3)
	data.Set("choices-0-ORDER", "1")
	data.Set("choices-1-ORDER", "2")
	data.Set("choices-2-ORDER", "")
	data.Set("choices-3-ORDER", "")
	fs := choiceFormSet()
	fs.CanOrder = true
	fs = fs.Bind(data)
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, cleanedOf(fs.OrderedForms()), []map[string]any{
		{"votes": int64(100), "ORDER": int64(1), "choice": "Calexico"},
		{"votes": int64(900), "ORDER": int64(2), "choice": "Fergie"},
		{"votes": int64(500), "ORDER": nil, "choice": "The Decemberists"},
		{"votes": int64(50), "ORDER": nil, "choice": "Basia Bulat"},
	})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_ordering_blank_fieldsets
func TestOrderingBlankFieldsets(t *testing.T) {
	fs := choiceFormSet()
	fs.CanOrder = true
	fs = fs.Bind(choiceData(nil, 0))
	fs.Data().Set("choices-TOTAL_FORMS", "3")
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, len(fs.OrderedForms()), 0)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_with_ordering_and_deletion
func TestFormsetWithOrderingAndDeletion(t *testing.T) {
	fs := choiceFormSet()
	fs.CanOrder, fs.CanDelete = true, true
	fs.Initial = []map[string]any{
		{"choice": "Calexico", "votes": 100},
		{"choice": "Fergie", "votes": 900},
		{"choice": "The Decemberists", "votes": 500},
	}
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).AsUL),
		`<li>Choice: <input type="text" name="choices-0-choice" value="Calexico"></li>`+
			`<li>Votes: <input type="number" name="choices-0-votes" value="100"></li>`+
			`<li>Order: <input type="number" name="choices-0-ORDER" value="1"></li>`+
			`<li>Delete: <input type="checkbox" name="choices-0-DELETE"></li>`+
			`<li>Choice: <input type="text" name="choices-1-choice" value="Fergie"></li>`+
			`<li>Votes: <input type="number" name="choices-1-votes" value="900"></li>`+
			`<li>Order: <input type="number" name="choices-1-ORDER" value="2"></li>`+
			`<li>Delete: <input type="checkbox" name="choices-1-DELETE"></li>`+
			`<li>Choice: <input type="text" name="choices-2-choice" value="The Decemberists"></li>`+
			`<li>Votes: <input type="number" name="choices-2-votes" value="500"></li>`+
			`<li>Order: <input type="number" name="choices-2-ORDER" value="3"></li>`+
			`<li>Delete: <input type="checkbox" name="choices-2-DELETE"></li>`+
			`<li>Choice: <input type="text" name="choices-3-choice"></li>`+
			`<li>Votes: <input type="number" name="choices-3-votes"></li>`+
			`<li>Order: <input type="number" name="choices-3-ORDER"></li>`+
			`<li>Delete: <input type="checkbox" name="choices-3-DELETE"></li>`)
	// Delete Fergie and put The Decemberists ahead of Calexico.
	data := choiceData([][2]string{{"Calexico", "100"}, {"Fergie", "900"}, {"The Decemberists", "500"}, {"", ""}}, 3)
	for i, v := range [][2]string{{"1", ""}, {"2", "on"}, {"0", ""}, {"", ""}} {
		data.Set(fmt.Sprintf("choices-%d-ORDER", i), v[0])
		data.Set(fmt.Sprintf("choices-%d-DELETE", i), v[1])
	}
	bound := fs.Bind(data)
	if !bound.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(bound))
	}
	equal(t, cleanedOf(bound.OrderedForms()), []map[string]any{
		{"votes": int64(500), "DELETE": false, "ORDER": int64(0), "choice": "The Decemberists"},
		{"votes": int64(100), "DELETE": false, "ORDER": int64(1), "choice": "Calexico"},
	})
	equal(t, cleanedOf(bound.DeletedForms()), []map[string]any{
		{"votes": int64(900), "DELETE": true, "ORDER": int64(2), "choice": "Fergie"},
	})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_invalid_deleted_form_with_ordering
func TestInvalidDeletedFormWithOrdering(t *testing.T) {
	fs := forms.NewFormSet(favoriteDrinkForm)
	fs.CanDelete, fs.CanOrder = true, true
	fs = fs.Bind(vals(
		"form-0-name", "",
		"form-0-DELETE", "on", // no name
		"form-TOTAL_FORMS", "1",
		"form-INITIAL_FORMS", "1",
		"form-MIN_NUM_FORMS", "0",
		"form-MAX_NUM_FORMS", "1",
	))
	if !fs.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(fs))
	}
	equal(t, len(fs.OrderedForms()), 0)
}

// drinksData is the data of two favorite drinks with the prefix "drinks".
func drinksData(first, second string) url.Values {
	return vals(
		"drinks-TOTAL_FORMS", "2",
		"drinks-INITIAL_FORMS", "0",
		"drinks-MIN_NUM_FORMS", "0",
		"drinks-MAX_NUM_FORMS", "0",
		"drinks-0-name", first,
		"drinks-1-name", second,
	)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_clean_hook
func TestFormsetCleanHook(t *testing.T) {
	fs := favoriteDrinksFormSet()
	fs.Prefix = "drinks"
	bound := fs.Bind(drinksData("Gin and Tonic", "Gin and Tonic"))
	if bound.IsValid() {
		t.Error("formset is valid")
	}
	// The errors of the clean functions are the non-form errors.
	equal(t, bound.NonFormErrors().Messages(), []string{"You may only specify a drink once."})
	bound = fs.Bind(drinksData("Gin and Tonic", "Bloody Mary"))
	if !bound.IsValid() {
		t.Errorf("formset not valid: %v", bound.NonFormErrors().Messages())
	}
	equal(t, bound.NonFormErrors().Messages(), []string{})
}

// drinkForms renders the forms of an unbound favorite drinks formset with
// the given settings.
func drinkForms(extra, maxNum int, initial ...string) *forms.FormSet {
	fs := forms.NewFormSet(favoriteDrinkForm)
	fs.Extra, fs.MaxNum = extra, maxNum
	for _, name := range initial {
		fs.Initial = append(fs.Initial, map[string]any{"name": name})
	}
	return fs
}

// drinkHTML is the HTML of a favorite drink form at index i.
func drinkHTML(i int, value string) string {
	v := ""
	if value != "" {
		v = ` value="` + value + `"`
	}
	return fmt.Sprintf(`<div><label for="id_form-%d-name">Name:</label>`+
		`<input type="text" name="form-%d-name"%s id="id_form-%d-name"></div>`, i, i, v, i)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_limiting_max_forms
func TestLimitingMaxForms(t *testing.T) {
	// Without MaxNum the number of forms follows Extra.
	fs := drinkForms(3, forms.DefaultMaxNum)
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).String), drinkHTML(0, "")+drinkHTML(1, "")+drinkHTML(2, ""))
	// A MaxNum of 0 shows no form at all.
	equal(t, len(drinkForms(3, 0).Forms()), 0)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_limited_max_forms_two
func TestLimitedMaxFormsTwo(t *testing.T) {
	fs := drinkForms(5, 2)
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).String), drinkHTML(0, "")+drinkHTML(1, ""))
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_limiting_extra_lest_than_max_num
func TestLimitingExtraLessThanMaxNum(t *testing.T) {
	fs := drinkForms(1, 2)
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).String), drinkHTML(0, ""))
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_max_num_with_initial_data
func TestMaxNumWithInitialData(t *testing.T) {
	fs := drinkForms(1, forms.DefaultMaxNum, "Fernet and Coke")
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).String), drinkHTML(0, "Fernet and Coke")+drinkHTML(1, ""))
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_max_num_zero
func TestMaxNumZero(t *testing.T) {
	equal(t, len(drinkForms(1, 0).Forms()), 0)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_max_num_zero_with_initial
func TestMaxNumZeroWithInitial(t *testing.T) {
	// The initial forms win over MaxNum.
	fs := drinkForms(1, 0, "Fernet and Coke", "Bloody Mary")
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).String), drinkHTML(0, "Fernet and Coke")+drinkHTML(1, "Bloody Mary"))
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_more_initial_than_max_num
func TestMoreInitialThanMaxNum(t *testing.T) {
	fs := drinkForms(1, 2, "Gin Tonic", "Bloody Mary", "Jack and Coke")
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).String),
		drinkHTML(0, "Gin Tonic")+drinkHTML(1, "Bloody Mary")+drinkHTML(2, "Jack and Coke"))
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_default_absolute_max
func TestDefaultAbsoluteMax(t *testing.T) {
	// AbsoluteMax defaults to twice DefaultMaxNum with the default MaxNum.
	fs := favoriteDrinksFormSet().Bind(vals("form-TOTAL_FORMS", "2001", "form-INITIAL_FORMS", "0", "form-MAX_NUM_FORMS", "0"))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"Please submit at most 1000 forms."})
	equal(t, len(fs.Forms()), 2000)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_absolute_max
func TestAbsoluteMax(t *testing.T) {
	data := vals("form-TOTAL_FORMS", "2001", "form-INITIAL_FORMS", "0", "form-MAX_NUM_FORMS", "0")
	fs := forms.NewFormSet(favoriteDrinkForm)
	fs.AbsoluteMax = 3000
	bound := fs.Bind(data)
	if !bound.IsValid() {
		t.Errorf("formset not valid: %v", bound.NonFormErrors().Messages())
	}
	equal(t, len(bound.Forms()), 2001)
	// AbsoluteMax is a hard limit.
	data.Set("form-TOTAL_FORMS", "3001")
	bound = fs.Bind(data)
	if bound.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, len(bound.Forms()), 3000)
	equal(t, bound.NonFormErrors().Messages(), []string{"Please submit at most 1000 forms."})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_absolute_max_with_max_num
func TestAbsoluteMaxWithMaxNum(t *testing.T) {
	fs := forms.NewFormSet(favoriteDrinkForm)
	fs.MaxNum, fs.AbsoluteMax = 30, 1000
	fs = fs.Bind(vals("form-TOTAL_FORMS", "1001", "form-INITIAL_FORMS", "0", "form-MAX_NUM_FORMS", "0"))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, len(fs.Forms()), 1000)
	equal(t, fs.NonFormErrors().Messages(), []string{"Please submit at most 30 forms."})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_absolute_max_invalid
//
// formset_factory raises a ValueError; a Go formset reports the mistake with
// Err.
func TestAbsoluteMaxInvalid(t *testing.T) {
	for _, maxNum := range []int{forms.DefaultMaxNum, 31} {
		fs := forms.NewFormSet(favoriteDrinkForm)
		fs.MaxNum, fs.AbsoluteMax = maxNum, 30
		err := fs.Err()
		if err == nil || !strings.Contains(err.Error(), "'absolute_max' must be greater or equal to 'max_num'") {
			t.Errorf("MaxNum %d: Err() = %v", maxNum, err)
		}
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_more_initial_form_result_in_one
func TestMoreInitialFormResultInOne(t *testing.T) {
	// One initial form, Extra 3 and MaxNum 2 give the initial form and one
	// extra form.
	fs := drinkForms(3, 2, "Gin Tonic")
	assertHTMLEqual(t, joinForms(fs.Forms(), (*forms.Form).String), drinkHTML(0, "Gin Tonic")+drinkHTML(1, ""))
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_management_form_field_names
func TestManagementFormFieldNames(t *testing.T) {
	equal(t, forms.NewFormSet(choiceForm).ManagementForm().FieldNames(), []string{
		forms.TotalFormCount, forms.InitialFormCount, forms.MinNumFormCount, forms.MaxNumFormCount,
	})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_management_form_prefix
func TestManagementFormPrefix(t *testing.T) {
	equal(t, favoriteDrinksFormSet().ManagementForm().Prefix, "form")
	bound := favoriteDrinksFormSet().Bind(vals(
		"form-TOTAL_FORMS", "2", "form-INITIAL_FORMS", "0", "form-MIN_NUM_FORMS", "0", "form-MAX_NUM_FORMS", "0"))
	equal(t, bound.ManagementForm().Prefix, "form")
	fs := favoriteDrinksFormSet()
	fs.Initial = []map[string]any{}
	equal(t, fs.ManagementForm().Prefix, "form")
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_non_form_errors
func TestNonFormErrors(t *testing.T) {
	fs := favoriteDrinksFormSet()
	fs.Prefix = "drinks"
	fs = fs.Bind(drinksData("Gin and Tonic", "Gin and Tonic"))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"You may only specify a drink once."})
	equal(t, fs.NonFormErrors().String(),
		`<ul class="errorlist nonform"><li>You may only specify a drink once.</li></ul>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_iteration
//
// A Go formset yields its forms with Forms(); overriding the iteration order
// of a formset class has no Go counterpart.
func TestFormsetIteration(t *testing.T) {
	fs := choiceFormSet()
	fs.Extra = 3
	list := fs.Forms()
	equal(t, len(list), 3)
	if list[0] != fs.Forms()[0] {
		t.Error("Forms() builds new forms on each call")
	}
}

// test_formset_nonzero has no Go counterpart: it checks the truth value of
// a Python object.

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_splitdatetimefield
func TestFormsetSplitDateTimeField(t *testing.T) {
	fs := forms.NewFormSet(func() *forms.Form {
		return forms.New(forms.F("when", forms.NewSplitDateTimeField(
			forms.Initial(func() any { return time.Now() }))))
	})
	fs = fs.Bind(vals(
		"form-TOTAL_FORMS", "1",
		"form-INITIAL_FORMS", "0",
		"form-0-when_0", "1904-06-16",
		"form-0-when_1", "15:51:33",
	))
	if !fs.IsValid() {
		t.Errorf("formset not valid: %v", formsetErrors(fs))
	}
}

// test_formset_error_class has no Go counterpart: a Go form has no
// error_class to replace.

// test_formset_calls_forms_is_valid has no Go counterpart: a Go form's
// IsValid cannot be overridden. TestFormsetValidationCount checks that the
// formset cleans each form.

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_hard_limit_on_instantiated_forms
//
// Django lowers DEFAULT_MAX_NUM to 2, which makes absolute_max 3; the Go
// test sets AbsoluteMax to 3.
func TestHardLimitOnInstantiatedForms(t *testing.T) {
	fs := choiceFormSet()
	fs.MaxNum, fs.AbsoluteMax = 1, 3
	// Someone fiddles with the management form data.
	fs = fs.Bind(vals(
		"choices-TOTAL_FORMS", "4",
		"choices-INITIAL_FORMS", "0",
		"choices-MIN_NUM_FORMS", "0",
		"choices-MAX_NUM_FORMS", "4",
	))
	// Only 3 forms are built, and the formset is not valid.
	equal(t, len(fs.Forms()), 3)
	if fs.IsValid() {
		t.Error("formset is valid")
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_increase_hard_limit
//
// Django lowers DEFAULT_MAX_NUM to 3, which makes absolute_max 7 for a
// max_num of 4; the Go test sets AbsoluteMax to 7.
func TestIncreaseHardLimit(t *testing.T) {
	fs := choiceFormSet()
	fs.MaxNum, fs.AbsoluteMax = 4, 7
	fs = fs.Bind(vals(
		"choices-TOTAL_FORMS", "4",
		"choices-INITIAL_FORMS", "0",
		"choices-MIN_NUM_FORMS", "0",
		"choices-MAX_NUM_FORMS", "4",
	))
	equal(t, len(fs.Forms()), 4)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_non_form_errors_run_full_clean
func TestNonFormErrorsRunFullClean(t *testing.T) {
	fs := choiceFormSet().CleanWith(func(*forms.FormSet) error {
		return forms.NewValidationError("This is a non-form error", "")
	})
	fs = fs.Bind(vals("choices-TOTAL_FORMS", "1", "choices-INITIAL_FORMS", "0"))
	equal(t, fs.NonFormErrors().Messages(), []string{"This is a non-form error"})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_validate_max_ignores_forms_marked_for_deletion
func TestValidateMaxIgnoresFormsMarkedForDeletion(t *testing.T) {
	fs := forms.NewFormSet(func() *forms.Form {
		return forms.New(forms.F("field", forms.NewIntegerField()))
	})
	fs.Prefix, fs.MaxNum, fs.ValidateMax, fs.CanDelete = "check", 1, true, true
	fs = fs.Bind(vals(
		"check-TOTAL_FORMS", "2",
		"check-INITIAL_FORMS", "0",
		"check-MAX_NUM_FORMS", "1",
		"check-0-field", "200",
		"check-0-DELETE", "",
		"check-1-field", "50",
		"check-1-DELETE", "on",
	))
	if !fs.IsValid() {
		t.Errorf("formset not valid: %v", fs.NonFormErrors().Messages())
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_total_error_count
func TestFormsetTotalErrorCount(t *testing.T) {
	tests := []struct {
		data [][2]string
		want int
	}{
		{[][2]string{{"Calexico", "100"}}, 0},
		{[][2]string{{"Calexico", ""}}, 1},
		{[][2]string{{"", "invalid"}}, 2},
		{[][2]string{{"Calexico", "100"}, {"Calexico", ""}}, 1},
		{[][2]string{{"Calexico", ""}, {"Calexico", ""}}, 2},
	}
	for _, tt := range tests {
		equal(t, makeChoiceFormSet(tt.data...).TotalErrorCount(), tt.want)
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_formset_total_error_count_with_non_form_errors
func TestFormsetTotalErrorCountWithNonFormErrors(t *testing.T) {
	data := vals(
		"choices-TOTAL_FORMS", "2",
		"choices-INITIAL_FORMS", "0",
		"choices-MAX_NUM_FORMS", "2", // ignored
		"choices-0-choice", "Zero",
		"choices-0-votes", "0",
		"choices-1-choice", "One",
		"choices-1-votes", "1",
	)
	fs := choiceFormSet()
	fs.Extra, fs.MaxNum, fs.ValidateMax = 1, 1, true
	equal(t, fs.Bind(data).TotalErrorCount(), 1)
	data.Set("choices-1-votes", "")
	equal(t, fs.Bind(data).TotalErrorCount(), 2)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_html_safe
func TestFormsetHTMLSafe(t *testing.T) {
	fs := choiceFormSet()
	equal(t, fs.HTML(), fs.String())
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_can_delete_extra_formset_forms
func TestCanDeleteExtraFormsetForms(t *testing.T) {
	fs := forms.NewFormSet(choiceForm)
	fs.CanDelete, fs.Extra = true, 2
	list := fs.Forms()
	equal(t, len(list), 2)
	for i, f := range list {
		if _, ok := f.FieldDef("DELETE"); !ok {
			t.Errorf("form %d has no DELETE field", i)
		}
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_disable_delete_extra_formset_forms
//
// Django asserts _should_delete_form of each form; the Go test checks which
// forms DeletedForms returns.
func TestDisableDeleteExtraFormsetForms(t *testing.T) {
	newSet := func() *forms.FormSet {
		fs := forms.NewFormSet(choiceForm)
		fs.CanDelete, fs.CanDeleteExtra, fs.Extra = true, false, 2
		return fs
	}
	hasDelete := func(f *forms.Form) bool {
		_, ok := f.FieldDef("DELETE")
		return ok
	}
	fs := newSet()
	equal(t, len(fs.Forms()), 2)
	if hasDelete(fs.Forms()[0]) || hasDelete(fs.Forms()[1]) {
		t.Error("an extra form has a DELETE field")
	}

	fs = newSet()
	fs.Initial = []map[string]any{{"choice": "Zero", "votes": "1"}}
	list := fs.Forms()
	equal(t, len(list), 3)
	if !hasDelete(list[0]) || hasDelete(list[1]) || hasDelete(list[2]) || hasDelete(fs.EmptyForm()) {
		t.Error("only the initial form should have a DELETE field")
	}

	bound := fs.Bind(vals(
		"form-0-choice", "Zero",
		"form-0-votes", "0",
		"form-0-DELETE", "on",
		"form-1-choice", "One",
		"form-1-votes", "1",
		"form-2-choice", "",
		"form-2-votes", "",
		"form-TOTAL_FORMS", "3",
		"form-INITIAL_FORMS", "1",
	))
	if !bound.IsValid() {
		t.Fatalf("formset not valid: %v", formsetErrors(bound))
	}
	equal(t, bound.CleanedData(), []map[string]any{
		{"choice": "Zero", "votes": int64(0), "DELETE": true},
		{"choice": "One", "votes": int64(1)},
		{},
	})
	deleted := bound.DeletedForms()
	if len(deleted) != 1 || deleted[0] != bound.Forms()[0] {
		t.Errorf("DeletedForms() = %v, want the first form", cleanedOf(deleted))
	}
}

// test_template_name_uses_renderer_value, test_template_name_can_be_overridden,
// test_custom_renderer, test_form_default_renderer and
// test_form_default_renderer_class have no Go counterpart: forms has no
// renderers, and a formset renders in Go code.

// choiceFormSetWithNonFormError is the ChoiceFormsetWithNonFormError of
// Django's formset tests.
func choiceFormSetWithNonFormError() *forms.FormSet {
	return choiceFormSet().CleanWith(func(*forms.FormSet) error {
		return forms.NewValidationError("non-form error", "")
	})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_repr
//
// Django shows the name of the formset class; a Go formset is a FormSet.
func TestFormsetRepr(t *testing.T) {
	validated := func(fs *forms.FormSet) *forms.FormSet {
		fs.Errors()
		return fs
	}
	extra10 := choiceFormSet()
	extra10.Extra = 10
	tests := []struct {
		fs   *forms.FormSet
		want string
	}{
		{choiceFormSet(), "<FormSet: bound=False valid=Unknown total_forms=1>"},
		{extra10, "<FormSet: bound=False valid=Unknown total_forms=10>"},
		{makeChoiceFormSet(), "<FormSet: bound=True valid=Unknown total_forms=0>"},
		{makeChoiceFormSet([2]string{"test", "1"}), "<FormSet: bound=True valid=Unknown total_forms=1>"},
		{validated(makeChoiceFormSet([2]string{"test", "1"})), "<FormSet: bound=True valid=True total_forms=1>"},
		{validated(makeChoiceFormSet([2]string{"test", ""})), "<FormSet: bound=True valid=False total_forms=1>"},
		{validated(makeChoiceFormSet([2]string{"test", "1"}, [2]string{"test", ""})), "<FormSet: bound=True valid=False total_forms=2>"},
		{validated(choiceFormSetWithNonFormError().Bind(choiceData([][2]string{{"test", ""}}, 0))), "<FormSet: bound=True valid=False total_forms=1>"},
	}
	for _, tt := range tests {
		equal(t, tt.fs.Repr(), tt.want)
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsFormsetTestCase.test_repr_do_not_trigger_validation
//
// Django mocks full_clean; the Go test sees from Repr that the formset is not
// validated until IsValid.
func TestFormsetReprDoNotTriggerValidation(t *testing.T) {
	fs := makeChoiceFormSet([2]string{"test", "1"})
	fs.Repr()
	equal(t, fs.Repr(), "<FormSet: bound=True valid=Unknown total_forms=1>")
	fs.IsValid()
	equal(t, fs.Repr(), "<FormSet: bound=True valid=True total_forms=1>")
}

// The Jinja2 variants of the formset test cases have no Go counterpart: Go
// has one template engine.

// asTagFormSet is the bound formset of Django's FormsetAsTagTests.
func asTagFormSet() *forms.FormSet {
	data := choiceData([][2]string{{"Calexico", "100"}}, 0)
	return choiceFormSet().Bind(data)
}

// asTagManagementHTML is the management form of asTagFormSet.
const asTagManagementHTML = `<input type="hidden" name="choices-TOTAL_FORMS" value="1">` +
	`<input type="hidden" name="choices-INITIAL_FORMS" value="0">` +
	`<input type="hidden" name="choices-MIN_NUM_FORMS" value="0">` +
	`<input type="hidden" name="choices-MAX_NUM_FORMS" value="0">`

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsetAsTagTests.test_as_table
func TestFormsetAsTable(t *testing.T) {
	assertHTMLEqual(t, asTagFormSet().AsTable(), asTagManagementHTML+
		`<tr><th>Choice:</th><td><input type="text" name="choices-0-choice" value="Calexico"></td></tr>`+
		`<tr><th>Votes:</th><td><input type="number" name="choices-0-votes" value="100"></td></tr>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsetAsTagTests.test_as_p
func TestFormsetAsP(t *testing.T) {
	assertHTMLEqual(t, asTagFormSet().AsP(), asTagManagementHTML+
		`<p>Choice: <input type="text" name="choices-0-choice" value="Calexico"></p>`+
		`<p>Votes: <input type="number" name="choices-0-votes" value="100"></p>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsetAsTagTests.test_as_ul
func TestFormsetAsUL(t *testing.T) {
	assertHTMLEqual(t, asTagFormSet().AsUL(), asTagManagementHTML+
		`<li>Choice: <input type="text" name="choices-0-choice" value="Calexico"></li>`+
		`<li>Votes: <input type="number" name="choices-0-votes" value="100"></li>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::FormsetAsTagTests.test_as_div
func TestFormsetAsDiv(t *testing.T) {
	assertHTMLEqual(t, asTagFormSet().AsDiv(), asTagManagementHTML+
		`<div>Choice: <input type="text" name="choices-0-choice" value="Calexico"></div>`+
		`<div>Votes: <input type="number" name="choices-0-votes" value="100"></div>`)
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestIsBoundBehavior.test_no_data_error
func TestFormsetNoDataError(t *testing.T) {
	fs := forms.NewFormSet(articleFormsetForm).Bind(url.Values{})
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{
		"ManagementForm data is missing or has been tampered with. " +
			"Missing fields: form-TOTAL_FORMS, form-INITIAL_FORMS. " +
			"You may need to file a bug report if the issue persists.",
	})
	equal(t, formsetErrors(fs), []map[string][]string{})
	// The formset still renders.
	assertHTMLEqual(t, fs.String(), `<ul class="errorlist nonfield">`+
		`<li>(Hidden field TOTAL_FORMS) This field is required.</li>`+
		`<li>(Hidden field INITIAL_FORMS) This field is required.</li>`+
		`</ul>`+
		`<div>`+
		`<input type="hidden" name="form-TOTAL_FORMS" id="id_form-TOTAL_FORMS">`+
		`<input type="hidden" name="form-INITIAL_FORMS" id="id_form-INITIAL_FORMS">`+
		`<input type="hidden" name="form-MIN_NUM_FORMS" id="id_form-MIN_NUM_FORMS">`+
		`<input type="hidden" name="form-MAX_NUM_FORMS" id="id_form-MAX_NUM_FORMS">`+
		"</div>\n")
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestIsBoundBehavior.test_management_form_invalid_data
func TestManagementFormInvalidData(t *testing.T) {
	fs := forms.NewFormSet(articleFormsetForm).Bind(vals("form-TOTAL_FORMS", "two", "form-INITIAL_FORMS", "one"))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{
		"ManagementForm data is missing or has been tampered with. " +
			"Missing fields: form-TOTAL_FORMS, form-INITIAL_FORMS. " +
			"You may need to file a bug report if the issue persists.",
	})
	equal(t, formsetErrors(fs), []map[string][]string{})
	assertHTMLEqual(t, fs.String(), `<ul class="errorlist nonfield">`+
		`<li>(Hidden field TOTAL_FORMS) Enter a whole number.</li>`+
		`<li>(Hidden field INITIAL_FORMS) Enter a whole number.</li>`+
		`</ul>`+
		`<div>`+
		`<input type="hidden" name="form-TOTAL_FORMS" value="two" id="id_form-TOTAL_FORMS">`+
		`<input type="hidden" name="form-INITIAL_FORMS" value="one" id="id_form-INITIAL_FORMS">`+
		`<input type="hidden" name="form-MIN_NUM_FORMS" id="id_form-MIN_NUM_FORMS">`+
		`<input type="hidden" name="form-MAX_NUM_FORMS" id="id_form-MAX_NUM_FORMS">`+
		"</div>\n")
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestIsBoundBehavior.test_customize_management_form_error
func TestCustomizeManagementFormError(t *testing.T) {
	fs := forms.NewFormSet(articleFormsetForm)
	fs.ErrorMessages = map[string]string{"missing_management_form": "customized"}
	fs = fs.Bind(url.Values{})
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, fs.NonFormErrors().Messages(), []string{"customized"})
	equal(t, formsetErrors(fs), []map[string][]string{})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestIsBoundBehavior.test_with_management_data_attrs_work_fine
func TestWithManagementDataAttrsWorkFine(t *testing.T) {
	fs := forms.NewFormSet(articleFormsetForm).Bind(vals("form-TOTAL_FORMS", "1", "form-INITIAL_FORMS", "0"))
	equal(t, fs.InitialFormCount(), 0)
	equal(t, fs.TotalFormCount(), 1)
	if !fs.IsBound() || !fs.Forms()[0].IsBound() {
		t.Error("the formset or its form is not bound")
	}
	if !fs.IsValid() || !fs.Forms()[0].IsValid() {
		t.Error("the formset or its form is not valid")
	}
	equal(t, fs.CleanedData(), []map[string]any{{}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestIsBoundBehavior.test_form_errors_are_caught_by_formset
func TestFormErrorsAreCaughtByFormset(t *testing.T) {
	fs := forms.NewFormSet(articleFormsetForm).Bind(vals(
		"form-TOTAL_FORMS", "2",
		"form-INITIAL_FORMS", "0",
		"form-0-title", "Test",
		"form-0-pub_date", "1904-06-16",
		"form-1-title", "Test",
		"form-1-pub_date", "", // missing but required
	))
	if fs.IsValid() {
		t.Error("formset is valid")
	}
	equal(t, formsetErrors(fs), []map[string][]string{{}, {"pub_date": {"This field is required."}}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestIsBoundBehavior.test_empty_forms_are_unbound
func TestEmptyFormsAreUnbound(t *testing.T) {
	unbound := forms.NewFormSet(articleFormsetForm)
	bound := unbound.Bind(vals(
		"form-TOTAL_FORMS", "1",
		"form-INITIAL_FORMS", "0",
		"form-0-title", "Test",
		"form-0-pub_date", "1904-06-16",
	))
	a, b := unbound.EmptyForm(), bound.EmptyForm()
	if a.IsBound() || b.IsBound() {
		t.Error("an empty form is bound")
	}
	assertHTMLEqual(t, a.AsP(), b.AsP())
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestEmptyFormSet.test_empty_formset_is_valid
func TestEmptyFormsetIsValid(t *testing.T) {
	newSet := func() *forms.FormSet {
		fs := forms.NewFormSet(favoriteDrinkForm)
		fs.Extra = 0
		return fs.CleanWith(func(*forms.FormSet) error {
			return forms.NewValidationError("Clean method called", "")
		})
	}
	// An empty formset still runs its clean functions.
	empty := newSet().Bind(vals("form-INITIAL_FORMS", "0", "form-TOTAL_FORMS", "0"))
	filled := newSet().Bind(vals("form-INITIAL_FORMS", "0", "form-TOTAL_FORMS", "1", "form-0-name", "bah"))
	if empty.IsValid() || filled.IsValid() {
		t.Error("a formset is valid")
	}
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::TestEmptyFormSet.test_empty_formset_media
func TestEmptyFormsetMedia(t *testing.T) {
	fs := forms.NewFormSet(func() *forms.Form {
		f := forms.New()
		f.ExtraMedia = forms.NewMedia(nil, []string{"some-file.js"})
		return f
	})
	fs.Extra = 0
	if !strings.Contains(fs.Media().String(), "some-file.js") {
		t.Errorf("media = %q", fs.Media().String())
	}
}

// test_empty_formset_is_multipart has no Go counterpart: forms has no file
// fields, so no form is multipart.

// Ported from Django: tests/forms_tests/tests/test_formsets.py::AllValidTests.test_valid
func TestAllValidValid(t *testing.T) {
	data := choiceData([][2]string{{"Zero", "0"}, {"One", "1"}}, 0)
	a, b := choiceFormSet().Bind(data), choiceFormSet().Bind(data)
	if !forms.AllValid(a, b) {
		t.Error("AllValid() = false")
	}
	equal(t, formsetErrors(a), []map[string][]string{{}, {}})
	equal(t, formsetErrors(b), []map[string][]string{{}, {}})
}

// Ported from Django: tests/forms_tests/tests/test_formsets.py::AllValidTests.test_invalid
func TestAllValidInvalid(t *testing.T) {
	// AllValid validates every formset, also after an invalid one.
	data := choiceData([][2]string{{"Zero", ""}, {"One", ""}}, 0)
	a, b := choiceFormSet().Bind(data), choiceFormSet().Bind(data)
	if forms.AllValid(a, b) {
		t.Error("AllValid() = true")
	}
	want := []map[string][]string{{"votes": {"This field is required."}}, {"votes": {"This field is required."}}}
	equal(t, formsetErrors(a), want)
	equal(t, formsetErrors(b), want)
}

// No Django test exists for this: a formset in a template, as Django's
// formset templates and the admin's inline templates use it.
func TestFormsetTemplateValue(t *testing.T) {
	fs := choiceFormSet()
	fs.Initial = []map[string]any{{"choice": "Calexico", "votes": 100}}
	ctx := map[string]any{"formset": fs.TemplateValue()}
	got := renderTemplate(t, `{{ formset.management_form }}{% for form in formset.forms %}{{ form.as_p }}{% endfor %}`+
		`{{ formset.empty_form.as_p }}{{ formset.prefix }} {{ formset.total_error_count }} {{ formset.is_bound }}`, ctx)
	assertHTMLEqual(t, got, `<input type="hidden" name="choices-TOTAL_FORMS" value="2">`+
		`<input type="hidden" name="choices-INITIAL_FORMS" value="1">`+
		`<input type="hidden" name="choices-MIN_NUM_FORMS" value="0">`+
		`<input type="hidden" name="choices-MAX_NUM_FORMS" value="1000">`+
		`<p>Choice: <input type="text" name="choices-0-choice" value="Calexico"></p>`+
		`<p>Votes: <input type="number" name="choices-0-votes" value="100"></p>`+
		`<p>Choice: <input type="text" name="choices-1-choice"></p>`+
		`<p>Votes: <input type="number" name="choices-1-votes"></p>`+
		`<p>Choice: <input type="text" name="choices-__prefix__-choice"></p>`+
		`<p>Votes: <input type="number" name="choices-__prefix__-votes"></p>`+
		`choices 0 False`)
	bound := makeChoiceFormSet([2]string{"Calexico", ""})
	got = renderTemplate(t, `{% if formset.is_valid %}valid{% else %}{{ formset.total_error_count }} {{ formset }}{% endif %}`,
		map[string]any{"formset": bound.TemplateValue()})
	if !strings.HasPrefix(got, "1 ") || !strings.Contains(got, "This field is required.") {
		t.Errorf("bound formset rendered %q", got)
	}
}
