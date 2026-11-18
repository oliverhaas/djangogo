package forms_test

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/forms"
)

// assertHTMLEqual is djangotest.AssertHTMLEqual, short for the many widget
// comparisons.
func assertHTMLEqual(t *testing.T, got, want string) {
	t.Helper()
	djangotest.AssertHTMLEqual(t, got, want)
}

// vals builds submitted data from alternating names and values; a repeated
// name adds a value, as a multi-value form field does.
func vals(pairs ...string) url.Values {
	data := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		data.Add(pairs[i], pairs[i+1])
	}
	return data
}

// date returns midnight UTC of a day, the cleaned value of a DateField.
func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// equal fails the test when got and want differ structurally.
func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// errorsOf returns the error messages of a form field.
func errorsOf(f *forms.Form, field string) []string {
	return f.Errors().Get(field).Messages()
}

// cleanErr cleans value with the field and returns the error messages; a
// value that validates yields nil.
func cleanErr(f forms.Field, value any) []string {
	_, err := f.Clean(value)
	if err == nil {
		return nil
	}
	return forms.ValidationErrors(err).Messages()
}

// cleans asserts that the field cleans value to want.
func cleans(t *testing.T, f forms.Field, value, want any) {
	t.Helper()
	got, err := f.Clean(value)
	if err != nil {
		t.Errorf("Clean(%#v) failed: %v", value, err)
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Clean(%#v) = %#v, want %#v", value, got, want)
	}
}

// rejects asserts that the field rejects value with exactly the messages.
func rejects(t *testing.T, f forms.Field, value any, want ...string) {
	t.Helper()
	got := cleanErr(f, value)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Clean(%#v) errors = %q, want %q", value, got, want)
	}
}

// personForm is the Person form of Django's forms tests.
func personForm() *forms.Form {
	return forms.New(
		forms.F("first_name", forms.NewCharField()),
		forms.F("last_name", forms.NewCharField()),
		forms.F("birthday", forms.NewDateField()),
	)
}

// personNewForm is PersonNew: Person with an explicit id on first_name.
func personNewForm() *forms.Form {
	return forms.New(
		forms.F("first_name", forms.NewCharField(
			forms.UseWidget(forms.TextInput{Attrs: forms.Attrs{"id": "first_name_id"}}))),
		forms.F("last_name", forms.NewCharField()),
		forms.F("birthday", forms.NewDateField()),
	)
}
