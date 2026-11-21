package forms_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/forms"
	"github.com/oliverhaas/djangogo/orm"
)

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_valid
func TestJSONFieldValid(t *testing.T) {
	field := forms.NewJSONField()
	cleans(t, field, `{"a": "b"}`, map[string]any{"a": "b"})
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_valid_empty
func TestJSONFieldValidEmpty(t *testing.T) {
	field := forms.NewJSONField(forms.Required(false))
	cleans(t, field, "", nil)
	cleans(t, field, nil, nil)
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_invalid
func TestJSONFieldInvalid(t *testing.T) {
	field := forms.NewJSONField()
	rejects(t, field, "{some badly formed: json}", "Enter a valid JSON.")
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_invalid_deeply_nested
func TestJSONFieldInvalidDeeplyNested(t *testing.T) {
	// Go's encoding/json rejects nesting deeper than 10000 levels directly.
	field := forms.NewJSONField()
	deep := strings.Repeat("[", 10001) + strings.Repeat("]", 10001)
	rejects(t, field, deep, "Enter a valid JSON.")
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_prepare_value
func TestJSONFieldPrepareValue(t *testing.T) {
	field := forms.NewJSONField()
	equal(t, field.PrepareValue(map[string]any{"a": "b"}), `{"a": "b"}`)
	equal(t, field.PrepareValue(nil), "null")
	equal(t, field.PrepareValue("foo"), `"foo"`)
	equal(t, field.PrepareValue("你好，世界"), `"你好，世界"`)
	equal(t, field.PrepareValue(map[string]any{"a": "😀🐱"}), `{"a": "😀🐱"}`)
	equal(t, field.PrepareValue([]string{"你好，世界", "jaźń"}), `["你好，世界", "jaźń"]`)
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_widget
func TestJSONFieldWidget(t *testing.T) {
	field := forms.NewJSONField()
	if _, ok := field.Widget.(forms.Textarea); !ok {
		t.Errorf("widget is %T, want forms.Textarea", field.Widget)
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_custom_widget_kwarg
func TestJSONFieldCustomWidgetKwarg(t *testing.T) {
	field := forms.NewJSONField(forms.UseWidget(forms.TextInput{}))
	if _, ok := field.Widget.(forms.TextInput); !ok {
		t.Errorf("widget is %T, want forms.TextInput", field.Widget)
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_converted_value
func TestJSONFieldConvertedValue(t *testing.T) {
	field := forms.NewJSONField(forms.Required(false))
	tests := []string{
		`["a", "b", "c"]`,
		`{"a": 1, "b": 2}`,
		"1",
		"1.5",
		`"foo"`,
		"true",
		"false",
		"null",
	}
	for _, jsonString := range tests {
		t.Run(jsonString, func(t *testing.T) {
			val, err := field.Clean(jsonString)
			if err != nil {
				t.Fatalf("Clean(%q) failed: %v", jsonString, err)
			}
			cleans(t, field, val, val)
		})
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_has_changed
func TestJSONFieldHasChanged(t *testing.T) {
	field := forms.NewJSONField()
	equal(t, field.HasChanged(map[string]any{"a": true}, `{"a": 1}`), true)
	equal(t, field.HasChanged(map[string]any{"a": 1, "b": 2}, `{"b": 2, "a": 1}`), false)
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_disabled_has_changed
func TestJSONFieldDisabledHasChanged(t *testing.T) {
	field := forms.NewJSONField(forms.Disabled())
	equal(t, field.HasChanged(map[string]any{"a": 1}, `{"a": 2}`), false)
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_custom_encoder_decoder
func TestJSONFieldCustomEncoderDecoder(t *testing.T) {
	decoder := func(data []byte) (any, error) {
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		if s, ok := m["uuid"].(string); ok {
			u, err := orm.ParseUUID(s)
			if err != nil {
				return nil, err
			}
			m["uuid"] = u
		}
		return m, nil
	}
	encoder := func(v any) ([]byte, error) {
		m, ok := v.(map[string]any)
		if !ok {
			return json.Marshal(v)
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			if u, ok := val.(orm.UUID); ok {
				out[k] = u.String()
			}
		}
		return []byte(`{"uuid": "` + out["uuid"] + `"}`), nil
	}
	value := map[string]any{"uuid": orm.MustUUID("{c141e152-6550-4172-a784-05448d98204b}")}
	encodedValue := `{"uuid": "c141e152-6550-4172-a784-05448d98204b"}`
	field := forms.NewJSONField(forms.JSONEncoder(encoder), forms.JSONDecoder(decoder))
	equal(t, field.PrepareValue(value), encodedValue)
	cleans(t, field, encodedValue, value)
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_formfield_disabled
func TestJSONFieldFormfieldDisabled(t *testing.T) {
	form := forms.New(forms.F("json_field", forms.NewJSONField(forms.Disabled())))
	form.Initial = map[string]any{"json_field": []string{"foo"}}
	bound := form.Bind(vals("json_field", `["bar"]`))
	if !strings.Contains(bound.AsP(), "[&quot;foo&quot;]</textarea>") {
		t.Errorf("AsP() = %q, want [&quot;foo&quot;]</textarea>", bound.AsP())
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_redisplay_none_input
func TestJSONFieldRedisplayNoneInput(t *testing.T) {
	form := forms.New(forms.F("json_field", forms.NewJSONField(forms.Required(true)))).Bind(vals())
	equal(t, form.Field("json_field").Value(), "null")
	if !strings.Contains(form.AsP(), "null</textarea>") {
		t.Errorf("AsP() = %q, want null</textarea>", form.AsP())
	}
	equal(t, errorsOf(form, "json_field"), []string{"This field is required."})
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_redisplay_wrong_input
func TestJSONFieldRedisplayWrongInput(t *testing.T) {
	newForm := func() *forms.Form {
		return forms.New(
			forms.F("name", forms.NewCharField(forms.MaxLength(2))),
			forms.F("json_field", forms.NewJSONField()),
		)
	}
	// JSONField input is valid, name is too long.
	form := newForm().Bind(vals("name", "xyz", "json_field", `["foo"]`))
	equal(t, form.HasError("json_field"), false)
	if !strings.Contains(form.AsP(), "[&quot;foo&quot;]</textarea>") {
		t.Errorf("AsP() = %q, want [&quot;foo&quot;]</textarea>", form.AsP())
	}
	// Invalid JSONField.
	form = newForm().Bind(vals("name", "xy", "json_field", `{"foo"}`))
	equal(t, errorsOf(form, "json_field"), []string{"Enter a valid JSON."})
	if !strings.Contains(form.AsP(), "{&quot;foo&quot;}</textarea>") {
		t.Errorf("AsP() = %q, want {&quot;foo&quot;}</textarea>", form.AsP())
	}
}

// Ported from Django: tests/forms_tests/field_tests/test_jsonfield.py::JSONFieldTest.test_redisplay_deeply_nested_input
func TestJSONFieldRedisplayDeeplyNestedInput(t *testing.T) {
	failDecoder := func([]byte) (any, error) {
		return nil, errors.New("maximum recursion depth exceeded")
	}
	form := forms.New(forms.F("json_field", forms.NewJSONField(forms.JSONDecoder(failDecoder)))).
		Bind(vals("json_field", "[[[]]]"))
	equal(t, errorsOf(form, "json_field"), []string{"Enter a valid JSON."})
	if !strings.Contains(form.AsP(), "[[[]]]</textarea>") {
		t.Errorf("AsP() = %q, want [[[]]]</textarea>", form.AsP())
	}
}
