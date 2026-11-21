package orm_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// choicefulModel mirrors Django's tests/model_fields/models.py::Choiceful.
type choicefulModel struct {
	ID                  int64
	NoChoices           *int `orm:"null"`
	EmptyChoices        *int `orm:"null"`
	WithChoices         *int `orm:"null"`
	WithChoicesGrouped  *int `orm:"null"`
	EmptyChoicesBool    bool
	EmptyChoicesText    string `orm:"type=text"`
	ChoicesFromCallable int
	ChoicesFromChar     string      `orm:"max_length=10"`
	ChoicesDecimal      orm.Decimal `orm:"max_digits=5;decimal_places=2"`
	ChoicesDate         orm.Date    `orm:"null"`
	ChoicesBool         bool
	ChoicesFloat        float64
	ChoicesUUID         orm.UUID
}

// Meta declares choices of every supported shape: empty, flat, grouped,
// callable, and across integer, boolean, text, char, decimal, date, float, and
// UUID kinds.
func (choicefulModel) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"EmptyChoices":       {},
			"EmptyChoicesBool":   {},
			"EmptyChoicesText":   {},
			"WithChoices":        {orm.ChoiceOf(1, "A")},
			"WithChoicesGrouped": {orm.ChoiceGroup("Thing", orm.ChoiceOf(1, "A"))},
			"ChoicesFromChar":    {orm.ChoiceOf("a", "A"), orm.ChoiceOf("b", "B")},
			"ChoicesDecimal":     {orm.ChoiceOf(orm.MustDecimal("1.50"), "One and a half"), orm.ChoiceOf(orm.MustDecimal("2.00"), "Two")},
			"ChoicesDate":        {orm.ChoiceOf(orm.MustDate("2025-01-01"), "New Year")},
			"ChoicesBool":        {orm.ChoiceOf(true, "Yes"), orm.ChoiceOf(false, "No")},
			"ChoicesFloat":       {orm.ChoiceOf(0.5, "Half"), orm.ChoiceOf(1.0, "Whole")},
			"ChoicesUUID":        {orm.ChoiceOf(orm.MustUUID("550e8400-e29b-41d4-a716-446655440000"), "Primary")},
		},
		ChoicesFunc: map[string]func() []orm.Choice{
			"ChoicesFromCallable": func() []orm.Choice {
				out := make([]orm.Choice, 3)
				for i := range 3 {
					out[i] = orm.ChoiceOf(i, strconv.Itoa(i))
				}
				return out
			},
		},
	}
}

// whizModel mirrors Django's tests/model_fields/models.py::Whiz with grouped
// and top-level integer choices.
type whizModel struct {
	ID int64
	C  *int `orm:"null"`
}

// Meta declares nested choice groups and top-level choices.
func (whizModel) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"C": {
				orm.ChoiceGroup("Group 1",
					orm.ChoiceOf(1, "First"),
					orm.ChoiceOf(2, "Second"),
				),
				orm.ChoiceGroup("Group 2",
					orm.ChoiceOf(3, "Third"),
					orm.ChoiceOf(4, "Fourth"),
				),
				orm.ChoiceOf(0, "Other"),
				orm.ChoiceOf(5, "translated"),
			},
		},
	}
}

// whizCharModel tests empty-string display on a string choices field.
type whizCharModel struct {
	ID int64
	C  string `orm:"max_length=10;blank"`
}

// Meta declares choices on C.
func (whizCharModel) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"C": {orm.ChoiceOf("a", "Alpha")},
		},
	}
}

// fooBarOverride mirrors Django's test_overriding_FIELD_display.
type fooBarOverride struct {
	ID     int64
	FooBar int
}

// Meta declares choices on FooBar.
func (fooBarOverride) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"FooBar": {orm.ChoiceOf(1, "foo"), orm.ChoiceOf(2, "bar")},
		},
	}
}

// GetFooBarDisplay overrides the default choice display label.
func (fooBarOverride) GetFooBarDisplay() string { return "something" }

// blankInChoicesModel mirrors Django's GetChoicesTests.
type blankInChoicesModel struct {
	ID      int64
	Empty   string `orm:"max_length=10"`
	Direct  string `orm:"max_length=10"`
	Grouped string `orm:"max_length=10"`
	Plain   string `orm:"max_length=10"`
}

// Meta declares flat, blank-containing, and grouped-blank choices.
func (blankInChoicesModel) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"Empty":  {},
			"Direct": {orm.ChoiceOf("", "<><>"), orm.ChoiceOf("a", "A")},
			"Grouped": {
				orm.ChoiceOf("f", "Foo"),
				orm.ChoiceOf("b", "Bar"),
				orm.ChoiceGroup("Group",
					orm.ChoiceOf("", "No Preference"),
					orm.ChoiceOf("fg", "Foo"),
					orm.ChoiceOf("bg", "Bar"),
				),
			},
			"Plain": {orm.ChoiceOf("a", "A"), orm.ChoiceOf("b", "B")},
		},
	}
}

// charChoicesModel mirrors Django's tests/model_fields/test_charfield.py::ValidationTests.
type charChoicesModel struct {
	ID           int64
	Required     string `orm:"max_length=10"`
	BlankAllowed string `orm:"max_length=10;blank"`
	WithChoices  string `orm:"max_length=10"`
	Callable     string `orm:"max_length=1"`
}

// Meta declares static and callable string choices.
func (charChoicesModel) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"WithChoices": {orm.ChoiceOf("a", "A"), orm.ChoiceOf("b", "B")},
		},
		ChoicesFunc: map[string]func() []orm.Choice{
			"Callable": func() []orm.Choice {
				return []orm.Choice{
					orm.ChoiceOf("0", "Option 0"),
					orm.ChoiceOf("1", "Option 1"),
					orm.ChoiceOf("2", "Option 2"),
				}
			},
		},
	}
}

// Ported from Django: tests/model_fields/tests.py::ChoicesTests.test_choices
func TestChoicesMetaOptions(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &choicefulModel{}))
	m, _ := reg.ModelOf(&choicefulModel{})

	noChoices, _ := m.FieldByName("NoChoices")
	djangotest.AssertEqual(t, noChoices.HasChoices, false)
	djangotest.AssertEqual(t, len(noChoices.EffectiveChoices()), 0)

	emptyChoices, _ := m.FieldByName("EmptyChoices")
	djangotest.AssertEqual(t, emptyChoices.HasChoices, true)
	djangotest.AssertEqual(t, len(emptyChoices.EffectiveChoices()), 0)

	emptyBool, _ := m.FieldByName("EmptyChoicesBool")
	djangotest.AssertEqual(t, emptyBool.HasChoices, true)
	djangotest.AssertEqual(t, len(emptyBool.EffectiveChoices()), 0)

	emptyText, _ := m.FieldByName("EmptyChoicesText")
	djangotest.AssertEqual(t, emptyText.HasChoices, true)
	djangotest.AssertEqual(t, len(emptyText.EffectiveChoices()), 0)

	withChoices, _ := m.FieldByName("WithChoices")
	djangotest.AssertEqual(t, len(withChoices.EffectiveChoices()), 1)
	djangotest.AssertEqual(t, withChoices.EffectiveChoices()[0].Value, "1")
	djangotest.AssertEqual(t, withChoices.EffectiveChoices()[0].Label, "A")

	callable, _ := m.FieldByName("ChoicesFromCallable")
	djangotest.AssertEqual(t, callable.HasChoices, true)
	djangotest.AssertEqual(t, len(callable.EffectiveChoices()), 3)
	djangotest.AssertEqual(t, callable.EffectiveChoices()[2].Label, "2")
}

// Ported from Django: tests/model_fields/tests.py::ChoicesTests.test_flatchoices
func TestFlatChoices(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &choicefulModel{}))
	m, _ := reg.ModelOf(&choicefulModel{})

	noChoices, _ := m.FieldByName("NoChoices")
	djangotest.AssertEqual(t, len(noChoices.FlatChoices()), 0)

	grouped, _ := m.FieldByName("WithChoicesGrouped")
	flat := grouped.FlatChoices()
	djangotest.AssertEqual(t, len(flat), 1)
	djangotest.AssertEqual(t, flat[0].Value, "1")
	djangotest.AssertEqual(t, flat[0].Label, "A")

	callable, _ := m.FieldByName("ChoicesFromCallable")
	flatCall := callable.FlatChoices()
	djangotest.AssertEqual(t, len(flatCall), 3)
	djangotest.AssertEqual(t, flatCall[0].Value, "0")
	djangotest.AssertEqual(t, flatCall[1].Value, "1")
	djangotest.AssertEqual(t, flatCall[2].Value, "2")
}

// Ported from Django: tests/model_fields/tests.py::ChoicesTests.test_invalid_choice
func TestInvalidChoiceValidation(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &choicefulModel{}))
	m, _ := reg.ModelOf(&choicefulModel{})

	noChoices, _ := m.FieldByName("NoChoices")
	emptyChoices, _ := m.FieldByName("EmptyChoices")
	withChoices, _ := m.FieldByName("WithChoices")
	withGrouped, _ := m.FieldByName("WithChoicesGrouped")

	djangotest.AssertEqual(t, noChoices.Validate(0, nil), nil)

	for _, f := range []*orm.Field{emptyChoices, withChoices, withGrouped} {
		err := f.Validate(99, nil)
		var ve *orm.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("field %s Validate(99) = %v, want *ValidationError", f.Name, err)
		}
		djangotest.AssertEqual(t, ve.Code, "invalid_choice")
		djangotest.AssertEqual(t, ve.Messages(), []string{"Value 99 is not a valid choice."})
	}
	djangotest.AssertEqual(t, withChoices.Validate(1, nil), nil)
	djangotest.AssertEqual(t, withGrouped.Validate(1, nil), nil)
}

// TestChoicesAllKinds verifies choices validation and display across decimal,
// date, bool, float, UUID, and integer kinds, including DB persistence on both
// dialects.
func TestChoicesAllKinds(t *testing.T) {
	djangotest.EachDB(t, []any{
		djangotest.App("model_fields", &choicefulModel{}),
	}, func(t *testing.T, db *orm.DB) {
		m, _ := db.Registry().ModelOf(&choicefulModel{})
		ctx := context.Background()
		one := 1
		obj := &choicefulModel{
			WithChoices:         &one,
			WithChoicesGrouped:  &one,
			ChoicesFromCallable: 2,
			ChoicesFromChar:     "a",
			ChoicesDecimal:      orm.MustDecimal("1.5"), // numerically equal to 1.50
			ChoicesDate:         orm.MustDate("2025-01-01"),
			ChoicesBool:         true,
			ChoicesFloat:        0.5,
			ChoicesUUID:         orm.MustUUID("550e8400-e29b-41d4-a716-446655440000"),
		}
		// Exclude EmptyChoices* fields which have empty choice sets and NoChoices
		// (null=true, blank=false, unset here).
		err := orm.FullClean(ctx, db, obj, orm.WithExclude("NoChoices", "EmptyChoices", "EmptyChoicesBool", "EmptyChoicesText"))
		djangotest.AssertEqual(t, err, nil)

		djangotest.AssertEqual(t, m.FieldDisplay(obj, "ChoicesDecimal"), "One and a half")
		djangotest.AssertEqual(t, m.FieldDisplay(obj, "ChoicesDate"), "New Year")
		djangotest.AssertEqual(t, m.FieldDisplay(obj, "ChoicesBool"), "Yes")
		djangotest.AssertEqual(t, m.FieldDisplay(obj, "ChoicesFloat"), "Half")
		djangotest.AssertEqual(t, m.FieldDisplay(obj, "ChoicesUUID"), "Primary")

		// Invalid decimal choice fails with Django's message.
		bad := *obj
		bad.ChoicesDecimal = orm.MustDecimal("9.99")
		err = orm.FullClean(ctx, db, &bad, orm.WithExclude("NoChoices", "EmptyChoices", "EmptyChoicesBool", "EmptyChoicesText"))
		var ve *orm.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("FullClean(bad decimal choice) = %v, want *ValidationError", err)
		}
		djangotest.AssertEqual(t, ve.MessageDict()["choices_decimal"], []string{"Value 9.99 is not a valid choice."})

		// Round-trip through SQLite and PostgreSQL.
		if err := orm.Query[choicefulModel](db).Create(ctx, obj); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := orm.Query[choicefulModel](db).Get(ctx, "id", obj.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, m.FieldDisplay(got, "WithChoices"), "A")
		djangotest.AssertEqual(t, m.FieldDisplay(got, "ChoicesDecimal"), "One and a half")
	})
}

// Ported from Django: tests/model_fields/tests.py::GetFieldDisplayTests.test_choices_and_field_display
func TestChoicesAndFieldDisplay(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &whizModel{}, &whizCharModel{}))
	wm, _ := reg.ModelOf(&whizModel{})
	cm, _ := reg.ModelOf(&whizCharModel{})

	intPtr := func(n int) *int { return &n }

	djangotest.AssertEqual(t, orm.GetDisplay(wm, &whizModel{C: intPtr(1)}, "C"), "First")
	djangotest.AssertEqual(t, orm.GetDisplay(wm, &whizModel{C: intPtr(0)}, "c"), "Other")
	djangotest.AssertEqual(t, orm.GetDisplay(wm, &whizModel{C: intPtr(9)}, "C"), 9)
	djangotest.AssertEqual(t, orm.GetDisplay(wm, &whizModel{C: nil}, "C"), nil)
	djangotest.AssertEqual(t, orm.GetDisplay(wm, &whizModel{C: intPtr(5)}, "C"), "translated")
	djangotest.AssertEqual(t, orm.GetDisplay(cm, &whizCharModel{C: ""}, "C"), "")
}

// Ported from Django: tests/model_fields/tests.py::GetFieldDisplayTests.test_overriding_FIELD_display
func TestOverridingFieldDisplay(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &fooBarOverride{}))
	m, _ := reg.ModelOf(&fooBarOverride{})
	obj := &fooBarOverride{FooBar: 1}
	djangotest.AssertEqual(t, orm.GetDisplay(m, obj, "FooBar"), "something")
	djangotest.AssertEqual(t, orm.GetDisplay(m, obj, "foo_bar"), "something")
}

// Ported from Django: tests/model_fields/tests.py::GetChoicesTests.test_empty_choices
// Ported from Django: tests/model_fields/tests.py::GetChoicesTests.test_blank_in_choices
// Ported from Django: tests/model_fields/tests.py::GetChoicesTests.test_blank_in_grouped_choices
func TestGetChoicesBlankHandling(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &blankInChoicesModel{}))
	m, _ := reg.ModelOf(&blankInChoicesModel{})

	empty, _ := m.FieldByName("Empty")
	djangotest.AssertEqual(t, len(empty.GetChoices(false)), 0)

	direct, _ := m.FieldByName("Direct")
	gotDirect := direct.GetChoices(true)
	djangotest.AssertEqual(t, len(gotDirect), 2)
	djangotest.AssertEqual(t, gotDirect[0].Value, "")
	djangotest.AssertEqual(t, gotDirect[0].Label, "<><>")

	grouped, _ := m.FieldByName("Grouped")
	gotGrouped := grouped.GetChoices(true)
	// Blank is already inside "Group", so BLANK_CHOICE_DASH is not prepended.
	djangotest.AssertEqual(t, len(gotGrouped), 3)
	djangotest.AssertEqual(t, gotGrouped[0].Value, "f")

	plain, _ := m.FieldByName("Plain")
	gotPlain := plain.GetChoices(true)
	djangotest.AssertEqual(t, len(gotPlain), 3)
	djangotest.AssertEqual(t, gotPlain[0].Value, "")
	djangotest.AssertEqual(t, gotPlain[0].Label, orm.BlankChoiceDash)
}

// Ported from Django: tests/model_fields/test_charfield.py::ValidationTests.test_charfield_raises_error_on_empty_string
// Ported from Django: tests/model_fields/test_charfield.py::ValidationTests.test_charfield_cleans_empty_string_when_blank_true
// Ported from Django: tests/model_fields/test_charfield.py::ValidationTests.test_charfield_with_choices_cleans_valid_choice
// Ported from Django: tests/model_fields/test_charfield.py::ValidationTests.test_charfield_with_choices_raises_error_on_invalid_choice
// Ported from Django: tests/model_fields/test_charfield.py::ValidationTests.test_charfield_raises_error_on_empty_input
// Ported from Django: tests/model_fields/test_charfield.py::ValidationTests.test_callable_choices
func TestCharFieldValidation(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &charChoicesModel{}))
	m, _ := reg.ModelOf(&charChoicesModel{})

	reqField, _ := m.FieldByName("Required")
	blankField, _ := m.FieldByName("BlankAllowed")
	choiceField, _ := m.FieldByName("WithChoices")
	callableField, _ := m.FieldByName("Callable")

	// Empty string on blank=False raises "This field cannot be blank."
	_, err := reqField.Clean("", nil)
	var ve *orm.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Clean('') = %v, want *ValidationError", err)
	}
	djangotest.AssertEqual(t, ve.Code, "blank")
	djangotest.AssertEqual(t, ve.Messages(), []string{"This field cannot be blank."})

	// nil on null=False raises "This field cannot be null."
	_, err = reqField.Clean(nil, nil)
	if !errors.As(err, &ve) {
		t.Fatalf("Clean(nil) = %v, want *ValidationError", err)
	}
	djangotest.AssertEqual(t, ve.Code, "null")
	djangotest.AssertEqual(t, ve.Messages(), []string{"This field cannot be null."})

	// Empty string on blank=True succeeds.
	got, err := blankField.Clean("", nil)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual[any](t, got, "")

	// Valid choice succeeds.
	got, err = choiceField.Clean("a", nil)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual[any](t, got, "a")

	// Invalid choice fails with quoted string repr.
	_, err = choiceField.Clean("not a", nil)
	if !errors.As(err, &ve) {
		t.Fatalf("Clean('not a') = %v, want *ValidationError", err)
	}
	djangotest.AssertEqual(t, ve.Code, "invalid_choice")
	djangotest.AssertEqual(t, ve.Messages(), []string{"Value 'not a' is not a valid choice."})

	// Callable choices clean valid values and reject invalid ones.
	for _, valid := range []string{"0", "1", "2"} {
		v, err := callableField.Clean(valid, nil)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual[any](t, v, valid)
	}
	for _, bad := range []string{"A", "3"} {
		_, err := callableField.Clean(bad, nil)
		if !errors.As(err, &ve) {
			t.Fatalf("Clean(%q) = %v, want *ValidationError", bad, err)
		}
		djangotest.AssertEqual(t, ve.Code, "invalid_choice")
	}
}

// badMetaChoicesModel references an unknown field in Meta.Choices.
type badMetaChoicesModel struct {
	ID   int64
	Name string
}

// Meta declares choices on a nonexistent field.
func (badMetaChoicesModel) Meta() orm.Meta {
	return orm.Meta{
		Choices: map[string][]orm.Choice{
			"Missing": {orm.ChoiceOf("a", "A")},
		},
	}
}

// badMetaValidatorsModel references an unknown field in Meta.Validators.
type badMetaValidatorsModel struct {
	ID   int64
	Name string
}

// Meta declares validators on a nonexistent field.
func (badMetaValidatorsModel) Meta() orm.Meta {
	return orm.Meta{
		Validators: map[string][]orm.Validator{
			"Missing": {func(any) error { return nil }},
		},
	}
}

// badMetaErrorMessagesModel references an unknown field in Meta.ErrorMessages.
type badMetaErrorMessagesModel struct {
	ID   int64
	Name string
}

// Meta declares error messages on a nonexistent field.
func (badMetaErrorMessagesModel) Meta() orm.Meta {
	return orm.Meta{
		ErrorMessages: map[string]map[string]string{
			"Missing": {"null": "custom"},
		},
	}
}

// TestMetaFieldHookErrors verifies registration rejects unknown fields in
// Meta.Choices, Meta.Validators, and Meta.ErrorMessages.
func TestMetaFieldHookErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		model any
	}{
		{"unknown Meta.Choices field", &badMetaChoicesModel{}},
		{"unknown Meta.Validators field", &badMetaValidatorsModel{}},
		{"unknown Meta.ErrorMessages field", &badMetaErrorMessagesModel{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := orm.NewRegistry()
			_, err := reg.Register(tc.model)
			if err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Errorf("Register(%s) = %v, want unknown field error", tc.name, err)
			}
		})
	}
}
