package forms_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// personModel returns the model of Person, registered without a database, for
// the tests that only build a form.
func personModel(t *testing.T) *orm.Model {
	t.Helper()
	m, err := orm.NewRegistry().Register(&Person{})
	if err != nil {
		t.Fatalf("register Person: %v", err)
	}
	return m
}

// widgetOf returns the widget of the field name of form.
func widgetOf(t *testing.T, form *forms.Form, name string) forms.Widget {
	t.Helper()
	fd, ok := form.FieldDef(name)
	if !ok {
		t.Fatalf("form has no field %q (fields %v)", name, form.FieldNames())
	}
	return fd.Base().Widget
}

// The Django test passes Meta.widgets of a base form to modelform_factory; Go
// options compose by appending, so both tests build the form from the same
// list of options.
//
// Ported from Django: tests/model_forms/tests.py::FormFieldCallbackTests.test_baseform_with_widgets_in_meta
// Ported from Django: tests/model_forms/tests.py::FormFieldCallbackTests.test_factory_with_widget_argument
func TestFormFieldCallbackWidgetArgument(t *testing.T) {
	m := personModel(t)
	// Without a widget should not set the widget to textarea
	form := forms.FromModel(m, forms.WithFields(forms.AllFields))
	if _, ok := widgetOf(t, form, "name").(forms.Textarea); ok {
		t.Error("name widget is a Textarea without WithWidget")
	}
	// With a widget should set the widget to textarea
	form = forms.FromModel(m, forms.WithFields(forms.AllFields), forms.WithWidget("name", forms.Textarea{}))
	if _, ok := widgetOf(t, form, "name").(forms.Textarea); !ok {
		t.Errorf("name widget = %T, want forms.Textarea", widgetOf(t, form, "name"))
	}
}

// Django passes Meta.widgets to the callback as the widget argument; the Go
// callback receives the default form field, and WithWidget applies to the
// field the callback returns. The Go form leaves out the auto primary key, so
// the callback sees only the name field.
//
// Ported from Django: tests/model_forms/tests.py::FormFieldCallbackTests.test_custom_callback
func TestFormFieldCallbackCustomCallback(t *testing.T) {
	m := personModel(t)
	var callbackArgs []string
	callback := func(mf *orm.Field, f forms.Field) forms.Field {
		callbackArgs = append(callbackArgs, mf.Attr)
		return f
	}
	form := forms.FromModel(m, forms.WithFields(forms.AllFields),
		forms.WithWidget("name", forms.Textarea{}), forms.WithFormFieldCallback(callback))
	if len(callbackArgs) != 1 || callbackArgs[0] != "name" {
		t.Errorf("callback args = %v, want [name]", callbackArgs)
	}
	if _, ok := widgetOf(t, form, "name").(forms.Textarea); !ok {
		t.Errorf("name widget = %T, want forms.Textarea", widgetOf(t, form, "name"))
	}
}

// The Go form leaves out the auto primary key that the Django test names in
// Meta.fields, so only the name field is checked.
//
// FIXME-PORT: test_inherit_after_custom_callback and
// test_custom_callback_from_base_form_meta subclass a form class; Go forms
// have no classes to inherit from. test_bad_callback passes a string as the
// callback, which the Go types reject at compile time.
//
// Ported from Django: tests/model_forms/tests.py::FormFieldCallbackTests.test_custom_callback_in_meta
func TestFormFieldCallbackCustomCallbackInMeta(t *testing.T) {
	m := personModel(t)
	callback := func(*orm.Field, forms.Field) forms.Field {
		return forms.NewCharField(forms.UseWidget(forms.Textarea{}))
	}
	form := forms.FromModel(m, forms.WithFields("id", "name"), forms.WithFormFieldCallback(callback))
	if names := form.FieldNames(); len(names) != 1 {
		t.Fatalf("fields = %v, want [name]", names)
	}
	for _, name := range form.FieldNames() {
		if _, ok := widgetOf(t, form, name).(forms.Textarea); !ok {
			t.Errorf("%s widget = %T, want forms.Textarea", name, widgetOf(t, form, name))
		}
	}
}

// No Django test covers a callback that leaves a field out; Django's
// fields_for_model adds the field to the ignored ones when the callback
// returns None.
func TestFormFieldCallbackNilLeavesFieldOut(t *testing.T) {
	m := personModel(t)
	form := forms.FromModel(m, forms.WithFields(forms.AllFields),
		forms.WithFormFieldCallback(func(*orm.Field, forms.Field) forms.Field { return nil }))
	if names := form.FieldNames(); len(names) != 0 {
		t.Errorf("fields = %v, want none", names)
	}
}
