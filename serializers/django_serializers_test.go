package serializers_test

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// Ported from Django: tests/serializers/tests.py::SerializerRegistrationTests.test_register
// Check that registering a new serializer format makes it visible alongside built-in public formats.
func TestSerializerRegistrationRegister(t *testing.T) {
	jsonFmt, err := serializers.Get("json")
	if err != nil {
		t.Fatalf("Get(json): %v", err)
	}
	serializers.Register("json2", jsonFmt)
	t.Cleanup(func() { _ = serializers.Unregister("json2") })

	serializers.Register("json3", jsonFmt)
	t.Cleanup(func() { _ = serializers.Unregister("json3") })

	publicFormats := serializers.PublicFormats()
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "json3"), true)
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "json2"), true)
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "json"), true)
}

// Ported from Django: tests/serializers/tests.py::SerializerRegistrationTests.test_unregister
// Check that unregistering a serializer removes it without repopulating the registry on subsequent registration.
func TestSerializerRegistrationUnregister(t *testing.T) {
	jsonFmt, err := serializers.Get("json")
	if err != nil {
		t.Fatalf("Get(json): %v", err)
	}
	t.Cleanup(func() {
		serializers.Register("json", jsonFmt)
		_ = serializers.Unregister("json3")
	})

	if err := serializers.Unregister("json"); err != nil {
		t.Fatalf("Unregister(json): %v", err)
	}
	serializers.Register("json3", jsonFmt)

	publicFormats := serializers.PublicFormats()
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "json"), false)
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "json3"), true)
}

// Ported from Django: tests/serializers/tests.py::SerializerRegistrationTests.test_unregister_unknown_serializer
// Check that unregistering an unknown format returns ErrSerializerDoesNotExist.
func TestSerializerRegistrationUnregisterUnknownSerializer(t *testing.T) {
	err := serializers.Unregister("nonsense")
	djangotest.AssertErrorIs(t, err, serializers.ErrSerializerDoesNotExist)
}

// Ported from Django: tests/serializers/tests.py::SerializerRegistrationTests.test_builtin_serializers
// Check that Formats includes internal and public serializers whereas PublicFormats excludes internal ones.
func TestSerializerRegistrationBuiltinSerializers(t *testing.T) {
	jsonFmt, err := serializers.Get("json")
	if err != nil {
		t.Fatalf("Get(json): %v", err)
	}
	serializers.Register("json2", jsonFmt)
	t.Cleanup(func() { _ = serializers.Unregister("json2") })

	allFormats := serializers.Formats()
	publicFormats := serializers.PublicFormats()

	djangotest.AssertEqual(t, slices.Contains(allFormats, "json"), true)
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "json"), true)

	djangotest.AssertEqual(t, slices.Contains(allFormats, "json2"), true)
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "json2"), true)

	djangotest.AssertEqual(t, slices.Contains(allFormats, "python"), true)
	djangotest.AssertEqual(t, slices.Contains(publicFormats, "python"), false)
}

// Ported from Django: tests/serializers/tests.py::SerializerRegistrationTests.test_get_unknown_serializer
// Check that Get with an unknown format returns ErrSerializerDoesNotExist with the format name.
func TestSerializerRegistrationGetUnknownSerializer(t *testing.T) {
	_, err := serializers.Get("nonsense")
	djangotest.AssertErrorIs(t, err, serializers.ErrSerializerDoesNotExist)
	if err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("Get(nonsense) error = %v, want message containing %q", err, "nonsense")
	}
}

// Ported from Django: tests/serializers/tests.py::SerializerRegistrationTests.test_get_unknown_deserializer
// Check that Deserialize with an unknown format yields ErrSerializerDoesNotExist.
func TestSerializerRegistrationGetUnknownDeserializer(t *testing.T) {
	_, err := collectDeserialized(context.Background(), nil, "nonsense", "[]")
	djangotest.AssertErrorIs(t, err, serializers.ErrSerializerDoesNotExist)
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize
// Check that basic serialization produces valid output for each format.
func TestSerializersSerialize(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				serialStr, err := serializers.Serialize(ctx, db, format, articles)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				djangotest.AssertEqual(t, validateOutput(format, serialStr), true)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serializer_roundtrip
// Check that serialized content can be deserialized back into the same number of objects.
func TestSerializersSerializerRoundtrip(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				serialStr, err := serializers.Serialize(ctx, db, format, articles)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				models, err := collectDeserialized(ctx, db, format, serialStr)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(models), 2)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_to_stream
// Check that SerializeTo writes the same output to an io.Writer and http.ResponseWriter as Serialize returns.
func TestSerializersSerializeToStream(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				obj := &ComplexModel{Field1: "first", Field2: "second", Field3: "third"}
				if err := orm.SaveRaw(ctx, db, obj); err != nil {
					t.Fatalf("SaveRaw: %v", err)
				}

				stringData, err := serializers.Serialize(ctx, db, format, []*ComplexModel{obj}, serializers.Indent(2))
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}

				var buf bytes.Buffer
				if err := serializers.SerializeTo(ctx, db, &buf, format, []*ComplexModel{obj}, serializers.Indent(2)); err != nil {
					t.Fatalf("SerializeTo(Buffer): %v", err)
				}
				djangotest.AssertEqual(t, stringData, buf.String())

				rec := httptest.NewRecorder()
				if err := serializers.SerializeTo(ctx, db, rec, format, []*ComplexModel{obj}, serializers.Indent(2)); err != nil {
					t.Fatalf("SerializeTo(ResponseRecorder): %v", err)
				}
				djangotest.AssertEqual(t, stringData, rec.Body.String())
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_specific_fields
// Check that serializing with Fields includes only the requested fields and leaves others at their default when deserialized.
func TestSerializersSerializeSpecificFields(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				obj := &ComplexModel{Field1: "first", Field2: "second", Field3: "third"}
				if err := orm.SaveRaw(ctx, db, obj); err != nil {
					t.Fatalf("SaveRaw: %v", err)
				}

				serializedData, err := serializers.Serialize(
					ctx, db, format, []*ComplexModel{obj},
					serializers.Indent(2),
					serializers.Fields("field1", "field3"),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				results, err := collectDeserialized(ctx, db, format, serializedData)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(results), 1)
				got := results[0].Object.(*ComplexModel)
				djangotest.AssertEqual(t, got.Field1, "first")
				djangotest.AssertEqual(t, got.Field2, "")
				djangotest.AssertEqual(t, got.Field3, "third")
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_altering_serialized_output
// Check that modifying serialized text and saving the deserialized objects updates the database rows.
func TestSerializersAlteringSerializedOutput(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				oldHeadline := "Poker has no place on ESPN"
				newHeadline := "Poker has no place on television"

				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				serialStr, err := serializers.Serialize(ctx, db, format, articles)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				serialStr = strings.ReplaceAll(serialStr, oldHeadline, newHeadline)
				models, err := collectDeserialized(ctx, db, format, serialStr)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}

				oldExists, err := orm.Query[Article](db).Filter("headline", oldHeadline).Exists(ctx)
				if err != nil {
					t.Fatalf("Exists(old): %v", err)
				}
				djangotest.AssertEqual(t, oldExists, true)
				newExists, err := orm.Query[Article](db).Filter("headline", newHeadline).Exists(ctx)
				if err != nil {
					t.Fatalf("Exists(new): %v", err)
				}
				djangotest.AssertEqual(t, newExists, false)

				for _, model := range models {
					if err := model.Save(ctx, db); err != nil {
						t.Fatalf("Save: %v", err)
					}
				}

				newExists, err = orm.Query[Article](db).Filter("headline", newHeadline).Exists(ctx)
				if err != nil {
					t.Fatalf("Exists(new after save): %v", err)
				}
				djangotest.AssertEqual(t, newExists, true)
				oldExists, err = orm.Query[Article](db).Filter("headline", oldHeadline).Exists(ctx)
				if err != nil {
					t.Fatalf("Exists(old after save): %v", err)
				}
				djangotest.AssertEqual(t, oldExists, false)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_one_to_one_as_pk
// Check that a OneToOneField serving as primary key is serialized as pk rather than in fields.
func TestSerializersOneToOneAsPK(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				data := seedBaseTestData(t, db)
				mustCreate(t, db, &AuthorProfile{
					Author:      o2oTo[Author](data.joe.ID),
					DateOfBirth: orm.NewDate(1970, time.January, 1),
				})
				profiles, err := orm.Query[AuthorProfile](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				serialStr, err := serializers.Serialize(ctx, db, format, profiles)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				djangotest.AssertEqual(t, len(getFieldValues(t, format, serialStr, "author")), 0)

				objs, err := collectDeserialized(ctx, db, format, serialStr)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(objs), 1)
				for _, obj := range objs {
					prof := obj.Object.(*AuthorProfile)
					djangotest.AssertEqual(t, prof.Author.PK(), data.joe.ID)
				}
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_field_subset
// Check that output can be restricted to a subset of fields on a model with FK and M2M relations.
func TestSerializersSerializeFieldSubset(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				validFields := []string{"headline", "pub_date"}
				invalidFields := []string{"author", "categories"}

				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				serialStr, err := serializers.Serialize(ctx, db, format, articles, serializers.Fields(validFields...))
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				for _, fieldName := range invalidFields {
					djangotest.AssertEqual(t, len(getFieldValues(t, format, serialStr, fieldName)), 0)
				}
				for _, fieldName := range validFields {
					djangotest.AssertEqual(t, len(getFieldValues(t, format, serialStr, fieldName)) > 0, true)
				}
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_unicode_roundtrip
// Check that Unicode primary keys and foreign keys roundtrip intact.
func TestSerializersSerializeUnicodeRoundtrip(t *testing.T) {
	t.Skip("FIXME-PORT: Go: foreign keys to non-integer primary keys, Django: test_serialize_unicode_roundtrip serializes Movie with a foreign key to Actor's CharField primary key")
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_unicode_serialization
// Check that non-ASCII Unicode strings are serialized unescaped and roundtrip through deserialization.
func TestSerializersUnicodeSerialization(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				unicodeName := "יוניקוד"
				data, err := serializers.Serialize(ctx, db, format, []Author{{Name: unicodeName}})
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				djangotest.AssertEqual(t, strings.Contains(data, unicodeName), true)
				objs, err := collectDeserialized(ctx, db, format, data)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(objs), 1)
				djangotest.AssertEqual(t, objs[0].Object.(*Author).Name, unicodeName)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_progressbar
// Check that serializing with Progress writes a 75-character progress bar to the output writer.
func TestSerializersSerializeProgressBar(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				count, err := orm.Query[Article](db).Count(ctx)
				if err != nil {
					t.Fatalf("Count: %v", err)
				}
				var fakeStdout bytes.Buffer
				_, err = serializers.Serialize(
					ctx, db, format, articles,
					serializers.Progress(&fakeStdout, int(count)),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				wantSuffix := "[" + strings.Repeat(".", serializers.ProgressWidth) + "]\n"
				djangotest.AssertEqual(t, strings.HasSuffix(fakeStdout.String(), wantSuffix), true)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_superfluous_queries
// Check that no database queries are executed when serializing a model with a foreign key.
func TestSerializersSerializeSuperfluousQueries(t *testing.T) {
	t.Skip("FIXME-PORT: Go: foreign keys to non-integer primary keys, Django: test_serialize_superfluous_queries serializes Movie with a foreign key to Actor's CharField primary key")
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_prefetch_related_m2m
// Check that serializing uses prefetched M2M caches without issuing extra queries per instance.
func TestSerializersSerializePrefetchRelatedM2M(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)

				djangotest.AssertNumQueries(t, db, 5, func() {
					articles, err := orm.Query[Article](db).PrefetchRelated(
						"meta_data",
						"topics",
						orm.Prefetch{
							Lookup:   "categories",
							QuerySet: orm.Query[Category](db).PrefetchRelated("topic_set"),
						},
					).All(ctx)
					if err != nil {
						t.Fatalf("PrefetchRelated All: %v", err)
					}
					if _, err := serializers.Serialize(ctx, db, format, articles); err != nil {
						t.Fatalf("Serialize prefetched: %v", err)
					}
				})

				djangotest.AssertNumQueries(t, db, 7, func() {
					articles, err := orm.Query[Article](db).All(ctx)
					if err != nil {
						t.Fatalf("All: %v", err)
					}
					if _, err := serializers.Serialize(ctx, db, format, articles); err != nil {
						t.Fatalf("Serialize unprefetched: %v", err)
					}
				})
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_prefetch_related_m2m_with_natural_keys
// Check that serializing with NaturalForeignKeys reads prefetched M2M caches and queries only unprefetched relations.
func TestSerializersSerializePrefetchRelatedM2MWithNaturalKeys(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)

				djangotest.AssertNumQueries(t, db, 5, func() {
					articles, err := orm.Query[Article](db).PrefetchRelated(
						orm.Prefetch{
							Lookup:   "meta_data",
							QuerySet: orm.Query[CategoryMetaData](db).PrefetchRelated("category_set"),
						},
						"topics",
					).All(ctx)
					if err != nil {
						t.Fatalf("PrefetchRelated All: %v", err)
					}
					if _, err := serializers.Serialize(ctx, db, format, articles, serializers.NaturalForeignKeys()); err != nil {
						t.Fatalf("Serialize: %v", err)
					}
				})
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_with_null_pk
// Check that serializing an unsaved instance writes a null pk and deserializes with a zero id.
func TestSerializersSerializeWithNullPK(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				category := Category{Name: "Reference"}
				serialStr, err := serializers.Serialize(ctx, db, format, []Category{category})
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				pkValues := getPKValues(t, format, serialStr)
				djangotest.AssertEqual(t, len(pkValues), 1)
				djangotest.AssertEqual(t, pkValues[0], nil)

				objs, err := collectDeserialized(ctx, db, format, serialStr)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(objs), 1)
				catObj := objs[0].Object.(*Category)
				djangotest.AssertEqual(t, catObj.ID, int64(0))
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_float_serialization
// Check that float values serialize and deserialize intact.
func TestSerializersFloatSerialization(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				sc := mustCreate(t, db, &Score{Score: 3.4})
				serialStr, err := serializers.Serialize(ctx, db, format, []*Score{sc})
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				deserialObjs, err := collectDeserialized(ctx, db, format, serialStr)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(deserialObjs), 1)
				got := deserialObjs[0].Object.(*Score)
				djangotest.AssertEqual(t, math.Abs(got.Score-3.4) < 0.05, true)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_deferred_field_serialization
// Check that an instance loaded with Defer can be serialized and deserialized.
func TestSerializersDeferredFieldSerialization(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				created := mustCreate(t, db, &Author{Name: "Victor Hugo"})
				author, err := orm.Query[Author](db).Defer("name").Get(ctx, "pk", created.ID)
				if err != nil {
					t.Fatalf("Defer Get: %v", err)
				}
				serialStr, err := serializers.Serialize(ctx, db, format, []Author{author})
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				deserialObjs, err := collectDeserialized(ctx, db, format, serialStr)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(deserialObjs), 1)
				_, ok := deserialObjs[0].Object.(*Author)
				djangotest.AssertEqual(t, ok, true)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_custom_field_serialization
// Check that custom model fields serialize and deserialize via value_to_string and to_python.
func TestSerializersCustomFieldSerialization(t *testing.T) {
	t.Skip("FIXME-PORT: Go: custom model fields, Django: test_custom_field_serialization serializes and deserializes a custom TeamField via value_to_string and to_python")
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_pre_1000ad_date
// Check that year values before 1000 AD are formatted with four-digit zero-padded years.
func TestSerializersPre1000ADDate(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				data := seedBaseTestData(t, db)
				a := mustCreate(t, db, &Article{
					Author:   fkTo[Author](data.jane.ID),
					Headline: "Nobody remembers the early years",
					PubDate:  time.Date(1, 2, 3, 4, 5, 6, 0, time.UTC),
				})
				serialStr, err := serializers.Serialize(ctx, db, format, []*Article{a})
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				dateValues := getFieldValues(t, format, serialStr, "pub_date")
				djangotest.AssertEqual(t, len(dateValues), 1)
				// Divergence from Django's USE_TZ=False test setting: djangogo stores
				// datetimes in UTC and writes them with a trailing "Z".
				got := strings.ReplaceAll(dateValues[0].(string), "T", " ")
				djangotest.AssertEqual(t, got, "0001-02-03 04:05:06Z")
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_pkless_serialized_strings
// Check that serialized objects with null or omitted primary keys deserialize with zero id and insert on Save.
func TestSerializersPKLessSerializedStrings(t *testing.T) {
	const pklessStr = `[
    {
        "pk": null,
        "model": "serializers.category",
        "fields": {"name": "Reference"}
    }, {
        "model": "serializers.category",
        "fields": {"name": "Non-fiction"}
    }]`

	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)
				deserialObjs, err := collectDeserialized(ctx, db, format, pklessStr)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				for _, obj := range deserialObjs {
					cat := obj.Object.(*Category)
					djangotest.AssertEqual(t, cat.ID, int64(0))
					if err := obj.Save(ctx, db); err != nil {
						t.Fatalf("Save: %v", err)
					}
				}
				count, err := orm.Query[Category](db).Count(ctx)
				if err != nil {
					t.Fatalf("Count: %v", err)
				}
				djangotest.AssertEqual(t, count, int64(5))
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_deterministic_mapping_ordering
// Check that serialized object keys and field mappings follow deterministic declaration order.
func TestSerializersDeterministicMappingOrdering(t *testing.T) {
	// Divergence from Django's USE_TZ=False test setting: pub_date carries the UTC "Z" suffix.
	const mappingOrderingFmt = `[
{
  "model": "serializers.article",
  "pk": %d,
  "fields": {
    "author": %d,
    "headline": "Poker has no place on ESPN",
    "pub_date": "2006-06-16T11:00:00Z",
    "categories": [
      %d,
      %d
    ],
    "meta_data": [],
    "topics": []
  }
}
]
`
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				data := seedBaseTestData(t, db)
				output, err := serializers.Serialize(ctx, db, format, []*Article{data.a1}, serializers.Indent(2))
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				cats, err := orm.Many[Category](db, data.a1, "categories").All(ctx)
				if err != nil {
					t.Fatalf("Many(categories): %v", err)
				}
				want := fmt.Sprintf(mappingOrderingFmt, data.a1.ID, data.a1.Author.PK(), cats[0].ID, cats[1].ID)
				djangotest.AssertEqual(t, output, want)
			})
		})
	}
}

// No Go counterpart: SerializersTestBase.test_deserialize_force_insert: mocks Python's Model.save_base to check **kwargs forwarding.

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_proxy_model
// Check that proxy models serialize using their concrete parent model.
func TestSerializersSerializeProxyModel(t *testing.T) {
	t.Skip("FIXME-PORT: Go: proxy models, Django: test_serialize_proxy_model serializes proxy and proxy-of-proxy models using their concrete model")
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_inherited_fields
// Check that multi-table inheritance child models serialize only their local fields.
func TestSerializersSerializeInheritedFields(t *testing.T) {
	t.Skip("FIXME-PORT: Go: multi-table model inheritance, Django: test_serialize_inherited_fields omits parent model fields when serializing a child model")
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_only_pk
// Check that M2M queries select only the target primary key when NaturalForeignKeys is not enabled.
func TestSerializersSerializeOnlyPK(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)

				qc := djangotest.CaptureQueries(db)
				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					qc.Stop()
					t.Fatalf("All: %v", err)
				}
				_, err = serializers.Serialize(ctx, db, format, articles)
				qc.Stop()
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}

				queries := qc.Queries()
				djangotest.AssertEqual(t, len(queries), 7)

				q := db.Dialect().Quote
				categoriesSQL := queries[1].SQL
				djangotest.AssertEqual(t, strings.Contains(categoriesSQL, q("meta_data_id")), false)
				metaDataSQL := queries[2].SQL
				djangotest.AssertEqual(t, strings.Contains(metaDataSQL, q("kind")), false)
				topicsDataSQL := queries[3].SQL
				djangotest.AssertEqual(t, strings.Contains(topicsDataSQL, q("category_id")), false)
			})
		})
	}
}

// Ported from Django: tests/serializers/tests.py::SerializersTestBase.test_serialize_no_only_pk_with_natural_keys
// Check that M2M queries load full rows for targets with natural keys and select only pk for other targets.
func TestSerializersSerializeNoOnlyPKWithNaturalKeys(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				seedBaseTestData(t, db)

				qc := djangotest.CaptureQueries(db)
				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					qc.Stop()
					t.Fatalf("All: %v", err)
				}
				_, err = serializers.Serialize(ctx, db, format, articles, serializers.NaturalForeignKeys())
				qc.Stop()
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}

				queries := qc.Queries()
				djangotest.AssertEqual(t, len(queries), 7)

				q := db.Dialect().Quote
				categoriesSQL := queries[1].SQL
				djangotest.AssertEqual(t, strings.Contains(categoriesSQL, q("meta_data_id")), false)
				metaDataSQL := queries[2].SQL
				djangotest.AssertEqual(t, strings.Contains(metaDataSQL, q("kind")), true)
				topicsDataSQL := queries[3].SQL
				djangotest.AssertEqual(t, strings.Contains(topicsDataSQL, q("category_id")), false)
			})
		})
	}
}

// No Go counterpart: SerializerAPITests.test_stream_class: subclasses Python Serializer to override the stream_class attribute; Go uses SerializeTo with an io.Writer.

// Ported from Django: tests/serializers/tests.py::SerializersTransactionTestBase.test_forward_refs
// Check that objects referencing primary keys defined later in the fixture save inside an atomic transaction.
func TestSerializersForwardRefs(t *testing.T) {
	const fwdRefStr = `[
    {
        "pk": 1,
        "model": "serializers.article",
        "fields": {
            "headline": "Forward references pose no problem",
            "pub_date": "2006-06-16T15:00:00",
            "categories": [1],
            "author": 1
        }
    },
    {
        "pk": 1,
        "model": "serializers.category",
        "fields": {
            "name": "Reference"
        }
    },
    {
        "pk": 1,
        "model": "serializers.author",
        "fields": {
            "name": "Agnes"
        }
    }]`

	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				err := db.Atomic(ctx, func(txCtx context.Context) error {
					for obj, err := range serializers.Deserialize(txCtx, db, format, strings.NewReader(fwdRefStr)) {
						if err != nil {
							return err
						}
						if err := obj.Save(txCtx, db); err != nil {
							return err
						}
					}
					return nil
				})
				if err != nil {
					t.Fatalf("Atomic forward refs: %v", err)
				}

				catCount, err := orm.Query[Category](db).Count(ctx)
				if err != nil {
					t.Fatalf("Category Count: %v", err)
				}
				djangotest.AssertEqual(t, catCount, int64(1))

				authorCount, err := orm.Query[Author](db).Count(ctx)
				if err != nil {
					t.Fatalf("Author Count: %v", err)
				}
				djangotest.AssertEqual(t, authorCount, int64(1))

				articles, err := orm.Query[Article](db).All(ctx)
				if err != nil {
					t.Fatalf("Article All: %v", err)
				}
				djangotest.AssertEqual(t, len(articles), 1)

				artObj := articles[0]
				artCatCount, err := orm.Many[Category](db, &artObj, "categories").Count(ctx)
				if err != nil {
					t.Fatalf("Article categories Count: %v", err)
				}
				djangotest.AssertEqual(t, artCatCount, int64(1))

				author, err := artObj.Author.Fetch(ctx, db)
				if err != nil {
					t.Fatalf("Author Fetch: %v", err)
				}
				djangotest.AssertEqual(t, author.Name, "Agnes")
			})
		})
	}
}
