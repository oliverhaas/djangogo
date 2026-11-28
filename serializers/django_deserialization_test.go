package serializers_test

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/internal/pyjson"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// authorObjectList returns the two-element Author Object slice used by
// TestDeserializer tests.
func authorObjectList() []*serializers.Object {
	return []*serializers.Object{
		{
			PK:     int64(1),
			HasPK:  true,
			Model:  "serializers.author",
			Fields: []serializers.FieldValue{{Name: "name", Value: "Jane"}},
		},
		{
			PK:     int64(2),
			HasPK:  true,
			Model:  "serializers.author",
			Fields: []serializers.FieldValue{{Name: "name", Value: "Joe"}},
		},
	}
}

// objectSliceSeq wraps objs as an iter.Seq2[*serializers.Object, error] for
// serializers.FromObjects.
func objectSliceSeq(objs []*serializers.Object) iter.Seq2[*serializers.Object, error] {
	return func(yield func(*serializers.Object, error) bool) {
		for _, o := range objs {
			if !yield(o, nil) {
				return
			}
		}
	}
}

// newTestDeserializerDB builds an in-memory *orm.DB whose registry contains the
// serializers test models for non-database deserializer tests.
func newTestDeserializerDB(t *testing.T) *orm.DB {
	t.Helper()
	reg := djangotest.NewRegistry(t, serializerModels()...)
	return orm.NewDB(nil, nil, reg)
}

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_deserialized_object_repr
// Check that DeserializedObject.String formats the model label and primary key like Django's __repr__.
func TestDeserializerDeserializedObjectRepr(t *testing.T) {
	db := newTestDeserializerDB(t)
	jane := &Author{ID: 1, Name: "Jane"}
	authorModel, _ := db.Registry().ModelOf(jane)
	deserialObj := &serializers.DeserializedObject{Object: jane, Model: authorModel}
	djangotest.AssertEqual(t, deserialObj.String(), "<DeserializedObject: serializers.Author(pk=1)>")
}

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_next_functionality
// Check that pulling items sequentially from FromObjects yields Jane, then Joe, and then stops cleanly.
func TestDeserializerNextFunctionality(t *testing.T) {
	db := newTestDeserializerDB(t)
	ctx := context.Background()
	seq := serializers.FromObjects(ctx, db, objectSliceSeq(authorObjectList()))
	next, stop := iter.Pull2(seq)
	defer stop()

	firstItem, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, *firstItem.Object.(*Author), Author{ID: 1, Name: "Jane"})

	secondItem, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, *secondItem.Object.(*Author), Author{ID: 2, Name: "Joe"})

	_, _, ok = next()
	djangotest.AssertEqual(t, ok, false)
}

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_invalid_model_identifier
// Check that an unknown model identifier yields a DeserializationError and an empty list yields no items.
func TestDeserializerInvalidModelIdentifier(t *testing.T) {
	db := newTestDeserializerDB(t)
	ctx := context.Background()
	invalidList := []*serializers.Object{
		{
			PK:     int64(1),
			HasPK:  true,
			Model:  "serializers.author2",
			Fields: []serializers.FieldValue{{Name: "name", Value: "Jane"}},
		},
	}
	next, stop := iter.Pull2(serializers.FromObjects(ctx, db, objectSliceSeq(invalidList)))
	defer stop()

	_, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)

	emptyNext, emptyStop := iter.Pull2(serializers.FromObjects(ctx, db, objectSliceSeq(nil)))
	defer emptyStop()
	_, _, ok = emptyNext()
	djangotest.AssertEqual(t, ok, false)
}

// No Go counterpart: TestDeserializer.test_custom_deserializer: subclasses Python's Deserializer to override the private _get_model_from_node staticmethod.

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_empty_object_list
// Check that FromObjects on an empty object list finishes immediately without yielding any objects.
func TestDeserializerEmptyObjectList(t *testing.T) {
	db := newTestDeserializerDB(t)
	ctx := context.Background()
	next, stop := iter.Pull2(serializers.FromObjects(ctx, db, objectSliceSeq(nil)))
	defer stop()

	_, _, ok := next()
	djangotest.AssertEqual(t, ok, false)
}

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_json_bytes_input
// Check that the json deserializer reads a UTF-8 byte stream and yields the expected Author objects.
func TestDeserializerJSONBytesInput(t *testing.T) {
	db := newTestDeserializerDB(t)
	ctx := context.Background()
	rawList := []any{
		map[string]any{"pk": 1, "model": "serializers.author", "fields": map[string]any{"name": "Jane"}},
		map[string]any{"pk": 2, "model": "serializers.author", "fields": map[string]any{"name": "Joe"}},
	}
	testString, err := pyjson.Marshal(rawList, pyjson.Options{Indent: -1, EnsureASCII: true})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	stream := bytes.NewReader([]byte(testString))

	next, stop := iter.Pull2(serializers.Deserialize(ctx, db, "json", stream))
	defer stop()

	firstItem, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, *firstItem.Object.(*Author), Author{ID: 1, Name: "Jane"})

	secondItem, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, *secondItem.Object.(*Author), Author{ID: 2, Name: "Joe"})
}

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_jsonl_bytes_input
// Check that the jsonl deserializer reads a UTF-8 byte stream with leading blank lines and yields objects lazily.
func TestDeserializerJSONLBytesInput(t *testing.T) {
	db := newTestDeserializerDB(t)
	ctx := context.Background()
	const testString = `
        {"pk": 1, "model": "serializers.author", "fields": {"name": "Jane"}}
        {"pk": 2, "model": "serializers.author", "fields": {"name": "Joe"}}
        {"pk": 3, "model": "serializers.author", "fields": {"name": "John"}}
        {"pk": 4, "model": "serializers.author", "fields": {"name": "Smith"}}`
	stream := bytes.NewReader([]byte(testString))

	next, stop := iter.Pull2(serializers.Deserialize(ctx, db, "jsonl", stream))
	defer stop()

	firstItem, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, *firstItem.Object.(*Author), Author{ID: 1, Name: "Jane"})

	secondItem, err, ok := next()
	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, *secondItem.Object.(*Author), Author{ID: 2, Name: "Joe"})
}

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_yaml_bytes_input
// Check that the yaml deserializer reads a UTF-8 byte stream and yields the expected Author objects.
func TestDeserializerYAMLBytesInput(t *testing.T) {
	t.Skip("FIXME-PORT: Go: yaml serializer format, Django: test_yaml_bytes_input deserializes YAML bytes into Author objects")
}

// Ported from Django: tests/serializers/test_deserialization.py::TestDeserializer.test_crafted_xml_performance
// Check that the xml deserializer processes deeply nested crafted XML in linear time.
// Django times one run of each size. The Go run is short enough that scheduler
// noise under a loaded test run breaks the factor bound, so each size takes the
// fastest of craftedXMLRuns runs.
func TestDeserializerCraftedXMLPerformance(t *testing.T) {
	const craftedXMLRuns = 5
	db := newTestDeserializerDB(t)
	ctx := context.Background()

	buildCraftedXML := func(depth, leafTextLen int) string {
		nestedOpen := strings.Repeat("<nested>", depth)
		nestedClose := strings.Repeat("</nested>", depth)
		leaf := strings.Repeat("x", leafTextLen)
		fieldContent := nestedOpen + leaf + nestedClose
		return fmt.Sprintf(`
                <django-objects version="1.0">
                   <object model="contenttypes.contenttype" pk="1">
                      <field name="app_label">%s</field>
                      <field name="model">m</field>
                   </object>
                </django-objects>
            `, fieldContent)
	}

	deserialize := func(t *testing.T, craftedXML string) time.Duration {
		t.Helper()
		runtime.GC()
		start := time.Now()
		result, err := collectDeserialized(ctx, db, "xml", craftedXML)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("Deserialize crafted XML: %v", err)
		}
		djangotest.AssertEqual(t, len(result), 1)
		if _, ok := result[0].Object.(*contenttypes.ContentType); !ok {
			t.Fatalf("got %T, want *contenttypes.ContentType", result[0].Object)
		}
		return elapsed
	}

	averageFactor := func(t *testing.T, params [][2]int) float64 {
		t.Helper()
		var factors []float64
		var prevTime time.Duration
		for i, p := range params {
			craftedXML := buildCraftedXML(p[0], p[1])
			elapsed := deserialize(t, craftedXML)
			for range craftedXMLRuns - 1 {
				elapsed = min(elapsed, deserialize(t, craftedXML))
			}
			if i > 0 && prevTime > 0 {
				factors = append(factors, float64(elapsed)/float64(prevTime))
			}
			prevTime = elapsed
		}
		var sum float64
		for _, f := range factors {
			sum += f
		}
		return sum / float64(len(factors))
	}

	assertFactor := func(label string, params [][2]int, factor float64) {
		t.Run(label, func(t *testing.T) {
			// Go has less fixed cost per call than Python, so a linear parse
			// gives a factor near 2, Django's limit, and the load of other
			// test packages can push one series over it. Measure up to three
			// series: a quadratic parse, with a factor near 4, fails each.
			var avg float64
			for range 3 {
				if avg = averageFactor(t, params); avg <= factor {
					return
				}
			}
			t.Errorf("average factor = %v, want <= %v", avg, factor)
		})
	}

	assertFactor(
		"varying depth, varying length",
		[][2]int{{50, 2000}, {100, 4000}, {200, 8000}, {400, 16000}, {800, 32000}},
		2,
	)
	assertFactor("constant depth, varying length", [][2]int{{100, 1}, {100, 1000}}, 2)
}
