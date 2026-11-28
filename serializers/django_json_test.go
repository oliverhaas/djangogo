package serializers_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/internal/pyjson"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_indentation_whitespace
// Check that indented JSON output has no trailing whitespace after commas.
func TestJSONSerializerIndentationWhitespace(t *testing.T) {
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		jsonData, err := serializers.Serialize(
			ctx, db, "json",
			[]Score{{Score: 5.0}, {Score: 6.0}},
			serializers.Indent(2),
		)
		if err != nil {
			t.Fatalf("Serialize: %v", err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(jsonData, "\n"), "\n") {
			trimmed := strings.TrimRight(line, " \t\r")
			if strings.HasSuffix(trimmed, ",") && len(trimmed) > 1 {
				djangotest.AssertEqual(t, line, trimmed)
			}
		}
	})
}

// No Django test exists: Indent(0) formats each object with newlines and no
// leading spaces while keeping the outer array separator ", " without extra
// newlines between objects, matching Django's falsy indent=0 check.
func TestJSONSerializerIndentZero(t *testing.T) {
	const want = "[{\n\"model\": \"serializers.score\",\n\"pk\": null,\n\"fields\": {\n\"score\": 5.0\n}\n}, {\n\"model\": \"serializers.score\",\n\"pk\": null,\n\"fields\": {\n\"score\": 6.0\n}\n}]\n"
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		got, err := serializers.Serialize(
			ctx, db, "json",
			[]Score{{Score: 5.0}, {Score: 6.0}},
			serializers.Indent(0),
		)
		if err != nil {
			t.Fatalf("Serialize: %v", err)
		}
		djangotest.AssertEqual(t, got, want)
	})
}

// No Go counterpart: JsonSerializerTestCase.test_custom_encoder: passes a custom json.JSONEncoder subclass via **options to Python's json.dump.

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_json_deserializer_exception
// Check that malformed JSON input yields a DeserializationError.
func TestJSONSerializerDeserializerException(t *testing.T) {
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "json", `[{"pk":1}`)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
	})
}

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_helpful_error_message_invalid_pk
// Check that an invalid primary key produces a DeserializationError naming the model and bad pk.
func TestJSONSerializerHelpfulErrorMessageInvalidPK(t *testing.T) {
	const testString = `[{
            "pk": "badpk",
            "model": "serializers.player",
            "fields": {
                "name": "Bob",
                "rank": 1,
                "team": "Team"
            }
        }]`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "json", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.player:pk=badpk)"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_helpful_error_message_invalid_field
// Check that an invalid field value produces a DeserializationError naming the model, pk, and field_value.
func TestJSONSerializerHelpfulErrorMessageInvalidField(t *testing.T) {
	const testString = `[{
            "pk": "1",
            "model": "serializers.player",
            "fields": {
                "name": "Bob",
                "rank": "invalidint",
                "team": "Team"
            }
        }]`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "json", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.player:pk=1) field_value was 'invalidint'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_helpful_error_message_for_foreign_keys
// Check that an unknown natural foreign key produces a DeserializationError showing the failing key tuple.
func TestJSONSerializerHelpfulErrorMessageForForeignKeys(t *testing.T) {
	const testString = `[{
            "pk": 1,
            "model": "serializers.category",
            "fields": {
                "name": "Unknown foreign key",
                "meta_data": [
                    "doesnotexist",
                    "metadata"
                ]
            }
        }]`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "json", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.category:pk=1) field_value was '['doesnotexist', 'metadata']'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_helpful_error_message_for_many2many_non_natural
// Check that an invalid non-natural M2M primary key produces a DeserializationError naming the failing value.
func TestJSONSerializerHelpfulErrorMessageForMany2ManyNonNatural(t *testing.T) {
	const testString = `[{
            "pk": 1,
            "model": "serializers.article",
            "fields": {
                "author": 1,
                "headline": "Unknown many to many",
                "pub_date": "2014-09-15T10:35:00",
                "categories": [1, "doesnotexist"]
            }
        }, {
            "pk": 1,
            "model": "serializers.author",
            "fields": {
                "name": "Agnes"
            }
        }, {
            "pk": 1,
            "model": "serializers.category",
            "fields": {
                "name": "Reference"
            }
        }]`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "json", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.article:pk=1) field_value was 'doesnotexist'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_helpful_error_message_for_many2many_natural1
// Check that an unknown natural M2M key among a list of natural keys reports the failing natural key in DeserializationError.
func TestJSONSerializerHelpfulErrorMessageForMany2ManyNatural1(t *testing.T) {
	const testString = `[{
            "pk": 1,
            "model": "serializers.categorymetadata",
            "fields": {
                "kind": "author",
                "name": "meta1",
                "value": "Agnes"
            }
        }, {
            "pk": 1,
            "model": "serializers.article",
            "fields": {
                "author": 1,
                "headline": "Unknown many to many",
                "pub_date": "2014-09-15T10:35:00",
                "meta_data": [
                    ["author", "meta1"],
                    ["doesnotexist", "meta1"],
                    ["author", "meta1"]
                ]
            }
        }, {
            "pk": 1,
            "model": "serializers.author",
            "fields": {
                "name": "Agnes"
            }
        }]`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		var gotErr error
		for obj, err := range serializers.Deserialize(ctx, db, "json", strings.NewReader(testString)) {
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

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_helpful_error_message_for_many2many_natural2
// Check that a scalar invalid key on a natural-key M2M relation reports that single value in DeserializationError.
func TestJSONSerializerHelpfulErrorMessageForMany2ManyNatural2(t *testing.T) {
	const testString = `[{
            "pk": 1,
            "model": "serializers.article",
            "fields": {
                "author": 1,
                "headline": "Unknown many to many",
                "pub_date": "2014-09-15T10:35:00",
                "meta_data": [1, "doesnotexist"]
            }
        }, {
            "pk": 1,
            "model": "serializers.categorymetadata",
            "fields": {
                "kind": "author",
                "name": "meta1",
                "value": "Agnes"
            }
        }, {
            "pk": 1,
            "model": "serializers.author",
            "fields": {
                "name": "Agnes"
            }
        }]`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		var gotErr error
		for obj, err := range serializers.Deserialize(ctx, db, "json", strings.NewReader(testString)) {
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

// Ported from Django: tests/serializers/test_json.py::JsonSerializerTestCase.test_helpful_error_message_for_many2many_not_iterable
// Check that a non-iterable M2M field value such as null produces a DeserializationError reporting field_value was 'None'.
func TestJSONSerializerHelpfulErrorMessageForMany2ManyNotIterable(t *testing.T) {
	const testString = `[{
            "pk": 1,
            "model": "serializers.m2mdata",
            "fields": {"data": null}
        }]`
	djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		_, err := collectDeserialized(ctx, db, "json", testString)
		djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
		want := "(serializers.m2mdata:pk=1) field_value was 'None'"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want substring %q", err, want)
		}
	})
}

// No Go counterpart: DjangoJSONEncoderTests.test_lazy_string_encoding: tests gettext_lazy Promise objects with locale override.

// Ported from Django: tests/serializers/test_json.py::DjangoJSONEncoderTests.test_timedelta
// Check that DjangoJSONDefault encodes time.Duration values in Django's ISO 8601 duration format.
func TestDjangoJSONEncoderTimedelta(t *testing.T) {
	opts := pyjson.Options{
		Indent:      -1,
		EnsureASCII: true,
		Default:     serializers.DjangoJSONDefault,
	}

	duration := 24*time.Hour + 2*time.Hour + 3*time.Second
	got, err := pyjson.Marshal(map[string]any{"duration": duration}, opts)
	if err != nil {
		t.Fatalf("Marshal(1d2h3s): %v", err)
	}
	djangotest.AssertEqual(t, got, `{"duration": "P1DT02H00M03S"}`)

	gotZero, err := pyjson.Marshal(map[string]any{"duration": time.Duration(0)}, opts)
	if err != nil {
		t.Fatalf("Marshal(0): %v", err)
	}
	djangotest.AssertEqual(t, gotZero, `{"duration": "P0DT00H00M00S"}`)
}
