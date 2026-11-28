package serializers_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// JSONModel is Django's model_fields JSONModel: a JSON field that is not
// null.
type JSONModel struct {
	ID    int64
	Value any `orm:"type=json"`
}

// NullableJSONModel is Django's model_fields NullableJSONModel: a JSON
// field that accepts null.
type NullableJSONModel struct {
	ID    int64
	Value any `orm:"type=json;null"`
}

// jsonFieldModels registers the models of the JSON field serialization
// tests under Django's app label.
func jsonFieldModels() []any {
	return []any{djangotest.App("model_fields", &JSONModel{}, &NullableJSONModel{})}
}

// jsonFieldTestData is TestSerialization.test_data: one unsaved JSONModel
// whose value is the serialized JSON text.
const jsonFieldTestData = `[{"fields": {"value": %s}, "model": "model_fields.jsonmodel", "pk": null}]`

// jsonFieldTestValues is TestSerialization.test_values: a Go value and its
// serialized JSON text.
var jsonFieldTestValues = []struct {
	value      any
	serialized string
}{
	{map[string]any{"a": "b", "c": nil}, `{"a": "b", "c": null}`},
	{"abc", `"abc"`},
	{`{"a": "a"}`, `"{\"a\": \"a\"}"`},
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSerialization.test_dumping
func TestJSONFieldSerializationDumping(t *testing.T) {
	djangotest.EachDB(t, jsonFieldModels(), func(t *testing.T, db *orm.DB) {
		for _, tc := range jsonFieldTestValues {
			t.Run(tc.serialized, func(t *testing.T) {
				data, err := serializers.Serialize(context.Background(), db, "json",
					[]JSONModel{{Value: tc.value}})
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				djangotest.AssertJSONEqual(t, data, fmt.Sprintf(jsonFieldTestData, tc.serialized))
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSerialization.test_loading
func TestJSONFieldSerializationLoading(t *testing.T) {
	djangotest.EachDB(t, jsonFieldModels(), func(t *testing.T, db *orm.DB) {
		for _, tc := range jsonFieldTestValues {
			t.Run(tc.serialized, func(t *testing.T) {
				input := fmt.Sprintf(jsonFieldTestData, tc.serialized)
				instance := firstDeserialized[JSONModel](t, db, "json", input)
				djangotest.AssertEqual(t, instance.Value, tc.value)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSerialization.test_xml_serialization
func TestJSONFieldSerializationXMLSerialization(t *testing.T) {
	const testXMLData = `<django-objects version="1.0">` +
		`<object model="model_fields.nullablejsonmodel">` +
		`<field name="value" type="JSONField">%s` +
		`</field></object></django-objects>`
	djangotest.EachDB(t, jsonFieldModels(), func(t *testing.T, db *orm.DB) {
		for _, tc := range jsonFieldTestValues {
			t.Run(tc.serialized, func(t *testing.T) {
				instance := NullableJSONModel{Value: tc.value}
				data, err := serializers.Serialize(context.Background(), db, "xml",
					[]NullableJSONModel{instance}, serializers.Fields("value"))
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				djangotest.AssertXMLEqual(t, data, fmt.Sprintf(testXMLData, tc.serialized))
				newInstance := firstDeserialized[NullableJSONModel](t, db, "xml", data)
				djangotest.AssertEqual(t, newInstance.Value, instance.Value)
			})
		}
	})
}

// firstDeserialized deserializes input in format and returns the first
// object, which must be a *T.
func firstDeserialized[T any](t *testing.T, db *orm.DB, format, input string) *T {
	t.Helper()
	for obj, err := range serializers.Deserialize(context.Background(), db, format, strings.NewReader(input)) {
		if err != nil {
			t.Fatalf("Deserialize: %v", err)
		}
		instance, ok := obj.Object.(*T)
		if !ok {
			t.Fatalf("Deserialize object is %T, want *%T", obj.Object, *new(T))
		}
		return instance
	}
	t.Fatal("Deserialize returned no object")
	return nil
}
