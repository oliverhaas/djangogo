package serializers_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_no_indentation
// Check that jsonl output ignores Indent and produces one compact JSON line per object.
func TestJsonlSerializerNoIndentation(t *testing.T) {
	const want = "{\"model\": \"serializers.score\",\"pk\": null,\"fields\": {\"score\": 5.0}}\n" +
		"{\"model\": \"serializers.score\",\"pk\": null,\"fields\": {\"score\": 6.0}}\n"
	trailingCommaRE := regexp.MustCompile(`.+,\s*$`)
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		jsonData, err := serializers.Serialize(
			ctx, db, "jsonl",
			[]Score{{Score: 5.0}, {Score: 6.0}},
			serializers.Indent(2),
		)
		if err != nil {
			t.Fatalf("Serialize: %v", err)
		}
		djangotest.AssertEqual(t, jsonData, want)
		for _, line := range strings.Split(strings.TrimSuffix(jsonData, "\n"), "\n") {
			djangotest.AssertEqual(t, trailingCommaRE.MatchString(line), false)
		}
	})
}

// No Go counterpart: JsonlSerializerTestCase.test_custom_encoder: passes a custom json.JSONEncoder subclass via **options to Python's json.dump.

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_json_deserializer_exception
// Check that malformed JSONL input yields a DeserializationError.
func TestJsonlSerializerDeserializerException(t *testing.T) {
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "jsonl", `[{"pk":1}`)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
	})
}

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_helpful_error_message_invalid_pk
// Check that an invalid primary key in JSONL input produces a DeserializationError naming the model and bad pk.
func TestJsonlSerializerHelpfulErrorMessageInvalidPK(t *testing.T) {
	const testString = `{"pk": "badpk","model": "serializers.player","fields": {"name": "Bob","rank": 1,"team": "Team"}}`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "jsonl", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.player:pk=badpk)"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_helpful_error_message_invalid_field
// Check that an invalid field value in JSONL input produces a DeserializationError naming the model, pk, and field_value.
func TestJsonlSerializerHelpfulErrorMessageInvalidField(t *testing.T) {
	const testString = `{"pk": "1","model": "serializers.player","fields": {"name": "Bob","rank": "invalidint","team": "Team"}}`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "jsonl", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.player:pk=1) field_value was 'invalidint'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_helpful_error_message_for_foreign_keys
// Check that an unknown natural foreign key in JSONL input produces a DeserializationError showing the failing key tuple.
func TestJsonlSerializerHelpfulErrorMessageForForeignKeys(t *testing.T) {
	const testString = `{"pk": 1, "model": "serializers.category","fields": {"name": "Unknown foreign key","meta_data": ["doesnotexist","metadata"]}}`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "jsonl", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.category:pk=1) field_value was '['doesnotexist', 'metadata']'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_helpful_error_message_for_many2many_non_natural
// Check that an invalid non-natural M2M primary key in JSONL input produces a DeserializationError naming the failing value.
func TestJsonlSerializerHelpfulErrorMessageForMany2ManyNonNatural(t *testing.T) {
	const testString = `{"pk": 1,"model": "serializers.article","fields": {"author": 1,"headline": "Unknown many to many","pub_date": "2014-09-15T10:35:00","categories": [1, "doesnotexist"]}}` + "\n" +
		`{"pk": 1,"model": "serializers.author","fields": {"name": "Agnes"}}` + "\n" +
		`{"pk": 1,"model": "serializers.category","fields": {"name": "Reference"}}`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "jsonl", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.article:pk=1) field_value was 'doesnotexist'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_helpful_error_message_for_many2many_natural1
// Check that an unknown natural M2M key in JSONL input reports the failing natural key in DeserializationError.
func TestJsonlSerializerHelpfulErrorMessageForMany2ManyNatural1(t *testing.T) {
	const testString = `{"pk": 1,"model": "serializers.categorymetadata","fields": {"kind": "author","name": "meta1","value": "Agnes"}}` + "\n" +
		`{"pk": 1,"model": "serializers.article","fields": {"author": 1,"headline": "Unknown many to many","pub_date": "2014-09-15T10:35:00","meta_data": [["author", "meta1"],["doesnotexist", "meta1"],["author", "meta1"]]}}` + "\n" +
		`{"pk": 1,"model": "serializers.author","fields": {"name": "Agnes"}}`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		var gotErr error
		for obj, err := range serializers.Deserialize(ctx, db, "jsonl", strings.NewReader(testString)) {
			if err != nil {
				gotErr = err
				break
			}
			if err := obj.Save(ctx, db); err != nil {
				gotErr = err
				break
			}
		}
		djangotest.AssertErrorIs(t, gotErr, serializers.ErrDeserialization)
		want := "(serializers.article:pk=1) field_value was '['doesnotexist', 'meta1']'"
		if gotErr == nil || !strings.Contains(gotErr.Error(), want) {
			t.Errorf("error = %v, want substring %q", gotErr, want)
		}
	})
}

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_helpful_error_message_for_many2many_natural2
// Check that a scalar invalid key on a natural-key M2M relation in JSONL input reports that value in DeserializationError.
func TestJsonlSerializerHelpfulErrorMessageForMany2ManyNatural2(t *testing.T) {
	const testString = `{"pk": 1,"model": "serializers.article","fields": {"author": 1,"headline": "Unknown many to many","pub_date": "2014-09-15T10:35:00","meta_data": [1, "doesnotexist"]}}` + "\n" +
		`{"pk": 1,"model": "serializers.categorymetadata","fields": {"kind": "author","name": "meta1","value": "Agnes"}}` + "\n" +
		`{"pk": 1,"model": "serializers.author","fields": {"name": "Agnes"}}`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		var gotErr error
		for obj, err := range serializers.Deserialize(ctx, db, "jsonl", strings.NewReader(testString)) {
			if err != nil {
				gotErr = err
				break
			}
			if err := obj.Save(ctx, db); err != nil {
				gotErr = err
				break
			}
		}
		djangotest.AssertErrorIs(t, gotErr, serializers.ErrDeserialization)
		want := "(serializers.article:pk=1) field_value was 'doesnotexist'"
		if gotErr == nil || !strings.Contains(gotErr.Error(), want) {
			t.Errorf("error = %v, want substring %q", gotErr, want)
		}
	})
}

// Ported from Django: tests/serializers/test_jsonl.py::JsonlSerializerTestCase.test_helpful_error_message_for_many2many_not_iterable
// Check that a non-iterable M2M field value in JSONL input produces a DeserializationError reporting field_value was 'None'.
func TestJsonlSerializerHelpfulErrorMessageForMany2ManyNotIterable(t *testing.T) {
	const testString = `{"pk": 1,"model": "serializers.m2mdata","fields": {"data": null}}`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "jsonl", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.m2mdata:pk=1) field_value was 'None'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}
