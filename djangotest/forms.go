package djangotest

import (
	"slices"
	"strings"

	"github.com/oliverhaas/djangogo/forms"
)

// AssertFormError asserts that a field of a form has exactly the given errors
// (Django's assertFormError). form is a *forms.Form, a type that embeds one
// such as a forms.ModelForm, or the form of a rendered template context, as
// resp.Context("form") returns it. An empty field name checks the errors of the
// form as a whole (Django's field=None), and no errors checks that there are
// none. MsgPrefix is the only option that applies.
//
// The form is validated if it has not been. The failure messages are Django's,
// with the form described by forms.Form.Repr.
func AssertFormError(t TB, form any, field string, errors []string, opts ...AssertOption) {
	t.Helper()
	c := newAssertConfig(0, opts)
	f, ok := forms.FormOf(form)
	if !ok {
		t.Errorf("%sAssertFormError needs a form, got %T", c.msgPrefix, form)
		return
	}
	if !f.IsBound() {
		t.Errorf("%sThe form %s is not bound, it will never have any errors.", c.msgPrefix, f.Repr())
		return
	}
	if _, ok := f.FieldDef(field); field != "" && !ok {
		t.Errorf("%sThe form %s does not contain the field %s.", c.msgPrefix, f.Repr(), pyRepr(field))
		return
	}
	var got []string
	if field == "" {
		got = f.NonFieldErrors().Messages()
	} else {
		got = f.Errors().Get(field).Messages()
	}
	if slices.Equal(got, errors) {
		return
	}
	if field == "" {
		t.Errorf("%sThe non-field errors of form %s don't match.\n%s != %s", c.msgPrefix, f.Repr(), pyList(got), pyList(errors))
	} else {
		t.Errorf("%sThe errors of field %s on form %s don't match.\n%s != %s", c.msgPrefix, pyRepr(field), f.Repr(), pyList(got), pyList(errors))
	}
}

// pyList renders a list of strings the way Python's repr() renders it, as
// unittest shows the two sides of a failed assertEqual: ['a', 'b'].
func pyList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = pyRepr(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// pyRepr quotes s the way Python's repr() quotes a string: in single quotes,
// or in double quotes when s contains a single quote and no double quote.
func pyRepr(s string) string {
	quote := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = `"`
	}
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`, quote, `\`+quote)
	return quote + r.Replace(s) + quote
}
