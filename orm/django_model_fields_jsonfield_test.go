package orm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// jfJSONModel represents Django's model_fields.JSONModel.
type jfJSONModel struct {
	ID    int64
	Value json.RawMessage
}

// jfNullableJSONModel represents Django's model_fields.NullableJSONModel.
type jfNullableJSONModel struct {
	ID    int64
	Value json.RawMessage `orm:"null"`
}

// jfRelatedJSONModel represents Django's model_fields.RelatedJSONModel.
type jfRelatedJSONModel struct {
	ID        int64
	Value     json.RawMessage
	JSONModel orm.FK[jfNullableJSONModel] `orm:"on_delete=cascade"`
}

// jfAnyJSONModel holds an any field tagged type=json so invalid Go values fail
// during JSON marshaling.
type jfAnyJSONModel struct {
	ID    int64
	Value any `orm:"type=json;null"`
}

// jfModels returns the models for the ports of tests/model_fields/test_jsonfield.py.
func jfModels() []any {
	return []any{djangotest.App("model_fields", &jfJSONModel{}, &jfNullableJSONModel{}, &jfRelatedJSONModel{})}
}

// jfID returns the primary key of a jfNullableJSONModel row.
func jfID(m jfNullableJSONModel) int64 { return m.ID }

// jfIDs returns the primary keys of the given jfNullableJSONModel pointers.
func jfIDs(objs ...*jfNullableJSONModel) []int64 {
	ids := make([]int64, len(objs))
	for i, o := range objs {
		ids[i] = o.ID
	}
	return ids
}

// jfPrimitives holds the primitive JSON values of TestQuerying.setUpTestData.
var jfPrimitives = []struct {
	name  string
	raw   json.RawMessage
	value any
}{
	{"true", json.RawMessage(`true`), true},
	{"false", json.RawMessage(`false`), false},
	{"yes", json.RawMessage(`"yes"`), "yes"},
	{"7", json.RawMessage(`7`), 7},
	{"9.6", json.RawMessage(`9.6`), 9.6},
}

// jfQueryingSetUp creates the rows of TestQuerying.setUpTestData in Django's
// order and returns them.
func jfQueryingSetUp(t *testing.T, db *orm.DB) []*jfNullableJSONModel {
	t.Helper()
	values := []json.RawMessage{
		nil,
		json.RawMessage(`[]`),
		json.RawMessage(`{}`),
		json.RawMessage(`{"a": "b", "c": 14}`),
		json.RawMessage(`{"a": "b", "c": 14, "d": ["e", {"f": "g"}], "h": true, "i": false, "j": null, "k": {"l": "m"}, "n": [null, true, false], "o": "\"quoted\"", "p": 4.2, "r": {"s": true, "t": false}}`),
		json.RawMessage(`[1, [2]]`),
		json.RawMessage(`{"k": true, "l": false, "foo": "bax"}`),
		json.RawMessage(`{"foo": "bar", "baz": {"a": "b", "c": "d"}, "bar": ["foo", "bar"], "bax": {"foo": "bar"}}`),
	}
	objs := make([]*jfNullableJSONModel, 0, len(values)+len(jfPrimitives))
	for _, v := range values {
		objs = append(objs, create(t, db, &jfNullableJSONModel{Value: v}))
	}
	for _, p := range jfPrimitives {
		objs = append(objs, create(t, db, &jfNullableJSONModel{Value: p.raw}))
	}
	return objs
}

// jfEachQuerying runs fn on each database with the rows of jfQueryingSetUp.
func jfEachQuerying(t *testing.T, fn func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel)) {
	djangotest.EachDB(t, jfModels(), func(t *testing.T, db *orm.DB) {
		objs := jfQueryingSetUp(t, db)
		fn(t, db, objs)
	})
}

// jfMustExists runs Exists on qs and fails the test if the query errors.
func jfMustExists[T any](t *testing.T, qs *orm.QuerySet[T]) bool {
	t.Helper()
	ok, err := qs.Exists(context.Background())
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	return ok
}

// Ported from Django: tests/model_fields/test_jsonfield.py::JSONFieldTests.test_invalid_value
func TestJSONFieldInvalidValue(t *testing.T) {
	djangotest.EachDB(t, []any{djangotest.App("model_fields", &jfAnyJSONModel{})}, func(t *testing.T, db *orm.DB) {
		err := orm.Query[jfAnyJSONModel](db).Create(context.Background(), &jfAnyJSONModel{
			Value: map[string]any{"fn": func() {}},
		})
		assertErrorContains(t, err, "json marshal")
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::JSONFieldTests.test_custom_encoder_decoder
func TestJSONFieldCustomEncoderDecoder(t *testing.T) {
	t.Skip("FIXME-PORT: custom JSON encoder/decoder on a model field")
}

// No Go counterpart: TestMethods.test_deconstruct tests Python's Field.deconstruct().

// No Go counterpart: TestMethods.test_deconstruct_custom_encoder_decoder tests Python's Field.deconstruct() with encoder and decoder classes.

// No Go counterpart: TestMethods.test_get_transforms tests Python's JSONField.register_lookup and get_transform returning KeyTransformFactory.

// No Go counterpart: TestMethods.test_key_transform_text_lookup_mixin_non_key_transform tests Python's KeyTransformTextLookupMixin constructor type check.

// No Go counterpart: TestMethods.test_get_prep_value mocks Python's connection.ops.adapt_json_value on a JSONField subclass.

// No Go counterpart: TestValidation.test_invalid_encoder tests Python's JSONField(encoder=...) callable check.

// No Go counterpart: TestValidation.test_invalid_decoder tests Python's JSONField(decoder=...) callable check.

// Ported from Django: tests/model_fields/test_jsonfield.py::TestValidation.test_validation_error
func TestJSONFieldValidationValidationError(t *testing.T) {
	reg := djangotest.NewRegistry(t, djangotest.App("model_fields", &jfNullableJSONModel{}))
	m, _ := reg.ModelOf(&jfNullableJSONModel{})
	f, _ := m.FieldByAttr("value")
	_, err := f.Clean(json.RawMessage(`{invalid`), nil)
	assertErrorContains(t, err, "Value must be valid JSON.")
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestValidation.test_custom_encoder
func TestJSONFieldValidationCustomEncoder(t *testing.T) {
	t.Skip("FIXME-PORT: custom JSON encoder/decoder on a model field")
}

// The ports of TestFormField are in package forms and those of
// TestSerialization in package serializers, each in its file
// django_model_fields_jsonfield_test.go.

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSaveLoad.test_primitives
func TestJSONFieldSaveLoadPrimitives(t *testing.T) {
	djangotest.EachDB(t, jfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		values := []string{
			`true`,
			`1`,
			`1.45`,
			`"String"`,
			`""`,
		}
		for _, raw := range values {
			t.Run(raw, func(t *testing.T) {
				obj := create(t, db, &jfJSONModel{Value: json.RawMessage(raw)})
				if err := orm.Query[jfJSONModel](db).RefreshFromDB(ctx, obj); err != nil {
					t.Fatalf("RefreshFromDB: %v", err)
				}
				djangotest.AssertJSONEqual(t, string(obj.Value), raw)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSaveLoad.test_dict
func TestJSONFieldSaveLoadDict(t *testing.T) {
	djangotest.EachDB(t, jfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		values := []string{
			`{}`,
			`{"name": "John", "age": 20, "height": 180.3}`,
			`{"a": true, "b": {"b1": false, "b2": null}}`,
		}
		for _, raw := range values {
			t.Run(raw, func(t *testing.T) {
				obj := create(t, db, &jfJSONModel{Value: json.RawMessage(raw)})
				if err := orm.Query[jfJSONModel](db).RefreshFromDB(ctx, obj); err != nil {
					t.Fatalf("RefreshFromDB: %v", err)
				}
				djangotest.AssertJSONEqual(t, string(obj.Value), raw)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSaveLoad.test_list
func TestJSONFieldSaveLoadList(t *testing.T) {
	djangotest.EachDB(t, jfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		values := []string{
			`[]`,
			`["John", 20, 180.3]`,
			`[true, [false, null]]`,
		}
		for _, raw := range values {
			t.Run(raw, func(t *testing.T) {
				obj := create(t, db, &jfJSONModel{Value: json.RawMessage(raw)})
				if err := orm.Query[jfJSONModel](db).RefreshFromDB(ctx, obj); err != nil {
					t.Fatalf("RefreshFromDB: %v", err)
				}
				djangotest.AssertJSONEqual(t, string(obj.Value), raw)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestSaveLoad.test_realistic_object
func TestJSONFieldSaveLoadRealisticObject(t *testing.T) {
	djangotest.EachDB(t, jfModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		raw := `{"name": "John", "age": 20, "pets": [{"name": "Kit", "type": "cat", "age": 2}, {"name": "Max", "type": "dog", "age": 1}], "courses": [["A1", "A2", "A3"], ["B1", "B2"], ["C1"]]}`
		obj := create(t, db, &jfJSONModel{Value: json.RawMessage(raw)})
		if err := orm.Query[jfJSONModel](db).RefreshFromDB(ctx, obj); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		djangotest.AssertJSONEqual(t, string(obj.Value), raw)
	})
}

// No Go counterpart: TestSaveLoad.test_bulk_update_custom_get_prep_value subclasses Python's JSONField to override get_prep_value and calls QuerySet.bulk_update.

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_exact
func TestJSONFieldQueryingExact(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__exact", json.RawMessage(`{}`)),
			jfIDs(objs[2]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_exact_complex
func TestJSONFieldQueryingExactComplex(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__exact", json.RawMessage(`{"a": "b", "c": 14}`)),
			jfIDs(objs[3]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_icontains
func TestJSONFieldQueryingIcontains(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__icontains", "BaX"),
			jfIDs(objs[6:8]...), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_isnull
func TestJSONFieldQueryingIsnull(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__isnull", true),
			jfIDs(objs[0]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_ordering_by_transform
func TestJSONFieldQueryingOrderingByTransform(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		values := []string{
			`{"ord": 93, "name": "bar"}`,
			`{"ord": 22.1, "name": "foo"}`,
			`{"ord": -1, "name": "baz"}`,
			`{"ord": 21.931902, "name": "spam"}`,
			`{"ord": -100291029, "name": "eggs"}`,
		}
		objs := make([]*jfNullableJSONModel, len(values))
		for i, v := range values {
			objs[i] = create(t, db, &jfNullableJSONModel{Value: json.RawMessage(v)})
		}
		qs := orm.Query[jfNullableJSONModel](db).Filter("value__name__isnull", false).OrderBy("value__ord")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[4], objs[2], objs[3], objs[1], objs[0]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_ordering_grouping_by_key_transform
func TestJSONFieldQueryingOrderingGroupingByKeyTransform(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		baseQS := orm.Query[jfNullableJSONModel](db).Filter("value__d__0__isnull", false)
		for _, qs := range []*orm.QuerySet[jfNullableJSONModel]{
			baseQS.OrderBy("value__d__0"),
			baseQS.Annotate("key", orm.KeyTransform("0", orm.KeyTransform("d", "value"))).OrderBy("key"),
		} {
			djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[4]), jfID)
		}
		qs := orm.Query[jfNullableJSONModel](db).Filter("value__isnull", false)
		vs := qs.Filter("value__isnull", false).
			Annotate("key", orm.KT("value__d__1__f")).
			Values("key").
			Annotate("count", orm.Count("key")).
			OrderBy("count")
		assertValues(t, vs, []map[string]any{
			{"key": nil, "count": int64(0)},
			{"key": "g", "count": int64(1)},
		})
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_ordering_grouping_by_count
func TestJSONFieldQueryingOrderingGroupingByCount(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		vs := orm.Query[jfNullableJSONModel](db).
			Filter("value__isnull", false).
			Values("value__d__0").
			Annotate("count", orm.Count("value__d__0")).
			OrderBy("count")
		rows, err := vs.All(context.Background())
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		counts := make([]int64, len(rows))
		for i, r := range rows {
			counts[i] = r["count"].(int64)
		}
		djangotest.AssertSequenceEqual(t, counts, []int64{0, 1})
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_order_grouping_custom_decoder
func TestJSONFieldQueryingOrderGroupingCustomDecoder(t *testing.T) {
	t.Skip("FIXME-PORT: custom JSON encoder/decoder on a model field")
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_transform_raw_expression
func TestJSONFieldQueryingKeyTransformRawExpression(t *testing.T) {
	t.Skip("FIXME-PORT: RawSQL expression")
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_nested_key_transform_raw_expression
func TestJSONFieldQueryingNestedKeyTransformRawExpression(t *testing.T) {
	t.Skip("FIXME-PORT: RawSQL expression")
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_transform_expression
func TestJSONFieldQueryingKeyTransformExpression(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).
			Filter("value__d__0__isnull", false).
			Annotate(
				"key", orm.KeyTransform("d", "value"),
				"chain", orm.KeyTransform("0", "key"),
				"expr", orm.KeyTransform("0", orm.Cast("key", orm.Field{Kind: orm.KindJSON})),
			).
			Filter("chain", orm.F("expr"))
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_transform_annotation_expression
func TestJSONFieldQueryingKeyTransformAnnotationExpression(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		obj := create(t, db, &jfNullableJSONModel{Value: json.RawMessage(`{"d": ["e", "e"]}`)})
		qs := orm.Query[jfNullableJSONModel](db).
			Filter("value__d__0__isnull", false).
			Annotate(
				"key", orm.F("value__d"),
				"chain", orm.F("key__0"),
				"expr", orm.Cast("key", orm.Field{Kind: orm.KindJSON}),
			).
			Filter("chain", orm.F("expr__1"))
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(obj), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_nested_key_transform_expression
func TestJSONFieldQueryingNestedKeyTransformExpression(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).
			Filter("value__d__0__isnull", false).
			Annotate(
				"key", orm.KeyTransform("d", "value"),
				"chain", orm.KeyTransform("f", orm.KeyTransform("1", "key")),
				"expr", orm.KeyTransform("f", orm.KeyTransform("1", orm.Cast("key", orm.Field{Kind: orm.KindJSON}))),
			).
			Filter("chain", orm.F("expr"))
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_nested_key_transform_annotation_expression
func TestJSONFieldQueryingNestedKeyTransformAnnotationExpression(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		obj := create(t, db, &jfNullableJSONModel{Value: json.RawMessage(`{"d": ["e", {"f": "g"}, {"f": "g"}]}`)})
		qs := orm.Query[jfNullableJSONModel](db).
			Filter("value__d__0__isnull", false).
			Annotate(
				"key", orm.F("value__d"),
				"chain", orm.F("key__1__f"),
				"expr", orm.Cast("key", orm.Field{Kind: orm.KindJSON}),
			).
			Filter("chain", orm.F("expr__2__f"))
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(obj), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_nested_key_transform_on_subquery
func TestJSONFieldQueryingNestedKeyTransformOnSubquery(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).
			Filter("value__d__0__isnull", false).
			Annotate(
				"subquery_value", orm.Subquery(
					orm.Query[jfNullableJSONModel](db).Filter("pk", orm.OuterRef("pk")).Values("value"),
				),
				"key", orm.KeyTransform("d", "subquery_value"),
				"chain", orm.KeyTransform("f", orm.KeyTransform("1", "key")),
			).
			Filter("chain", "g")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_text_transform_char_lookup
func TestJSONFieldQueryingKeyTextTransformCharLookup(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).
			Annotate("char_value", orm.KeyTextTransform("foo", "value")).
			Filter("char_value__startswith", "bar")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[7]), jfID)

		qs = orm.Query[jfNullableJSONModel](db).
			Annotate("char_value", orm.KeyTextTransform(1, orm.KeyTextTransform("bar", "value"))).
			Filter("char_value__startswith", "bar")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[7]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_expression_wrapper_key_transform
func TestJSONFieldQueryingExpressionWrapperKeyTransform(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).
			Annotate("expr", orm.ExpressionWrapper(orm.KeyTransform("c", "value"), orm.Field{Kind: orm.KindInt})).
			Filter("expr__isnull", false)
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[3:5]...), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_key
func TestJSONFieldQueryingHasKey(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__has_key", "a"),
			jfIDs(objs[3], objs[4]), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_key_null_value
func TestJSONFieldQueryingHasKeyNullValue(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__has_key", "j"),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_key_deep
func TestJSONFieldQueryingHasKeyDeep(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		tests := []struct {
			cond orm.Cond
			want *jfNullableJSONModel
		}{
			{orm.Q("value__baz__has_key", "a"), objs[7]},
			{orm.Q("value__has_key", orm.KeyTransform("a", orm.KeyTransform("baz", "value"))), objs[7]},
			{orm.Q("value__has_key", orm.F("value__baz__a")), objs[7]},
			{orm.Q("value__has_key", orm.KeyTransform("c", orm.KeyTransform("baz", "value"))), objs[7]},
			{orm.Q("value__has_key", orm.F("value__baz__c")), objs[7]},
			{orm.Q("value__d__1__has_key", "f"), objs[4]},
			{orm.Q("value__has_key", orm.KeyTransform("f", orm.KeyTransform("1", orm.KeyTransform("d", "value")))), objs[4]},
			{orm.Q("value__has_key", orm.F("value__d__1__f")), objs[4]},
		}
		for _, tc := range tests {
			t.Run(tc.cond.String(), func(t *testing.T) {
				djangotest.AssertQuerySetEqual(t,
					orm.Query[jfNullableJSONModel](db).Filter(tc.cond),
					jfIDs(tc.want), jfID)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_key_literal_lookup
func TestJSONFieldQueryingHasKeyLiteralLookup(t *testing.T) {
	t.Skip("FIXME-PORT: lookup expressions such as HasKey(lhs, rhs) as filter conditions")
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_key_list
func TestJSONFieldQueryingHasKeyList(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		obj := create(t, db, &jfNullableJSONModel{Value: json.RawMessage(`[{"a": 1}, {"b": "x"}]`)})
		tests := []orm.Cond{
			orm.Q("value__1__has_key", "b"),
			orm.Q("value__has_key", orm.KeyTransform("b", orm.KeyTransform(1, "value"))),
			orm.Q("value__has_key", orm.KeyTransform("b", orm.KeyTransform("1", "value"))),
			orm.Q("value__has_key", orm.F("value__1__b")),
		}
		for _, cond := range tests {
			t.Run(cond.String(), func(t *testing.T) {
				djangotest.AssertQuerySetEqual(t,
					orm.Query[jfNullableJSONModel](db).Filter(cond),
					jfIDs(obj), jfID)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_keys
func TestJSONFieldQueryingHasKeys(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__has_keys", []string{"a", "c", "h"}),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_any_keys
func TestJSONFieldQueryingHasAnyKeys(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__has_any_keys", []string{"c", "l"}),
			jfIDs(objs[3], objs[4], objs[6]), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_has_key_number
func TestJSONFieldQueryingHasKeyNumber(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		obj := create(t, db, &jfNullableJSONModel{
			Value: json.RawMessage(`{"123": "value", "nested": {"456": "bar", "lorem": "abc", "999": true}, "array": [{"789": "baz", "777": "def", "ipsum": 200}], "000": "val"}`),
		})
		tests := []orm.Cond{
			orm.Q("value__has_key", "123"),
			orm.Q("value__nested__has_key", "456"),
			orm.Q("value__array__0__has_key", "789"),
			orm.Q("value__has_keys", []string{"nested", "123", "array", "000"}),
			orm.Q("value__nested__has_keys", []string{"lorem", "999", "456"}),
			orm.Q("value__array__0__has_keys", []string{"789", "ipsum", "777"}),
			orm.Q("value__has_any_keys", []string{"000", "nonexistent"}),
			orm.Q("value__nested__has_any_keys", []string{"999", "nonexistent"}),
			orm.Q("value__array__0__has_any_keys", []string{"777", "nonexistent"}),
		}
		for _, cond := range tests {
			t.Run(cond.String(), func(t *testing.T) {
				djangotest.AssertQuerySetEqual(t,
					orm.Query[jfNullableJSONModel](db).Filter(cond),
					jfIDs(obj), jfID)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_contains
func TestJSONFieldQueryingContains(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("supports_json_field_contains")
		}
		tests := []struct {
			raw  string
			want []*jfNullableJSONModel
		}{
			{`{}`, append(append([]*jfNullableJSONModel{}, objs[2:5]...), objs[6:8]...)},
			{`{"baz": {"a": "b", "c": "d"}}`, []*jfNullableJSONModel{objs[7]}},
			{`{"baz": {"a": "b"}}`, []*jfNullableJSONModel{objs[7]}},
			{`{"baz": {"c": "d"}}`, []*jfNullableJSONModel{objs[7]}},
			{`{"k": true, "l": false}`, []*jfNullableJSONModel{objs[6]}},
			{`{"d": ["e", {"f": "g"}]}`, []*jfNullableJSONModel{objs[4]}},
			{`{"d": ["e"]}`, []*jfNullableJSONModel{objs[4]}},
			{`{"d": [{"f": "g"}]}`, []*jfNullableJSONModel{objs[4]}},
			{`[1, [2]]`, []*jfNullableJSONModel{objs[5]}},
			{`[1]`, []*jfNullableJSONModel{objs[5]}},
			{`[[2]]`, []*jfNullableJSONModel{objs[5]}},
			{`{"n": [null, true, false]}`, []*jfNullableJSONModel{objs[4]}},
			{`{"j": null}`, []*jfNullableJSONModel{objs[4]}},
		}
		for _, tc := range tests {
			t.Run(tc.raw, func(t *testing.T) {
				qs := orm.Query[jfNullableJSONModel](db).Filter("value__contains", json.RawMessage(tc.raw))
				djangotest.AssertQuerySetEqual(t, qs, jfIDs(tc.want...), jfID, djangotest.Unordered())
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_contains_unsupported
func TestJSONFieldQueryingContainsUnsupported(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		if db.Dialect().Name() == "postgres" {
			t.Skip("supports_json_field_contains")
		}
		_, err := orm.Query[jfNullableJSONModel](db).
			Filter("value__contains", json.RawMessage(`{"baz": {"a": "b", "c": "d"}}`)).
			Get(context.Background())
		assertErrorContains(t, err, "contains lookup is not supported on this database backend.")
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_contains_primitives
func TestJSONFieldQueryingContainsPrimitives(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("supports_json_field_contains")
		}
		for _, p := range jfPrimitives {
			t.Run(p.name, func(t *testing.T) {
				qs := orm.Query[jfNullableJSONModel](db).Filter("value__contains", p.value)
				djangotest.AssertEqual(t, jfMustExists(t, qs), true)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_contained_by
func TestJSONFieldQueryingContainedBy(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("supports_json_field_contains")
		}
		qs := orm.Query[jfNullableJSONModel](db).
			Filter("value__contained_by", json.RawMessage(`{"a": "b", "c": 14, "h": true}`))
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[2:4]...), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_contained_by_unsupported
func TestJSONFieldQueryingContainedByUnsupported(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		if db.Dialect().Name() == "postgres" {
			t.Skip("supports_json_field_contains")
		}
		_, err := orm.Query[jfNullableJSONModel](db).
			Filter("value__contained_by", json.RawMessage(`{"a": "b"}`)).
			Get(context.Background())
		assertErrorContains(t, err, "contained_by lookup is not supported on this database backend.")
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_values
func TestJSONFieldQueryingDeepValues(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		vl := orm.Query[jfNullableJSONModel](db).ValuesList("value__k__l").OrderBy("pk")
		want := make([][]any, len(objs))
		for i := range want {
			want[i] = []any{nil}
		}
		want[4] = []any{json.RawMessage(`"m"`)}
		assertValuesList(t, vl, want)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_distinct
func TestJSONFieldQueryingDeepDistinct(t *testing.T) {
	t.Skip("FIXME-PORT: DISTINCT ON fields")
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_isnull_key
func TestJSONFieldQueryingIsnullKey(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__a__isnull", true),
			jfIDs(append(append([]*jfNullableJSONModel{}, objs[:3]...), objs[5:]...)...),
			jfID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__j__isnull", true),
			jfIDs(append(append([]*jfNullableJSONModel{}, objs[:4]...), objs[5:]...)...),
			jfID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__a__isnull", false),
			jfIDs(objs[3], objs[4]),
			jfID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__j__isnull", false),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_isnull_key_or_none
func TestJSONFieldQueryingIsnullKeyOrNone(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		obj := create(t, db, &jfNullableJSONModel{Value: json.RawMessage(`{"a": null}`)})
		want := append(append([]*jfNullableJSONModel{}, objs[:3]...), objs[5:]...)
		want = append(want, obj)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter(orm.Q("value__a__isnull", true).Or(orm.Q("value__a", nil))),
			jfIDs(want...), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_none_key
func TestJSONFieldQueryingNoneKey(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__j", nil),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_none_key_exclude
func TestJSONFieldQueryingNoneKeyExclude(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		obj := create(t, db, &jfNullableJSONModel{Value: json.RawMessage(`{"j": 1}`)})
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Exclude("value__j", nil),
			jfIDs(obj), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_shallow_list_lookup
func TestJSONFieldQueryingShallowListLookup(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__0", 1),
			jfIDs(objs[5]), jfID)
	})
}

// No Go counterpart: TestQuerying.test_unsupported_negative_lookup runs on backends without negative JSON array indexing (MySQL and Oracle); SQLite and PostgreSQL both support it.

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_shallow_list_negative_lookup
func TestJSONFieldQueryingShallowListNegativeLookup(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__-2", 1),
			jfIDs(objs[5]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_shallow_obj_lookup
func TestJSONFieldQueryingShallowObjLookup(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__a", "b"),
			jfIDs(objs[3], objs[4]), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_obj_subquery_lookup
func TestJSONFieldQueryingObjSubqueryLookup(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).
			Annotate("field", orm.Subquery(
				orm.Query[jfNullableJSONModel](db).Filter("pk", orm.OuterRef("pk")).Values("value"),
			)).
			Filter("field__a", "b")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[3], objs[4]), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_lookup_objs
func TestJSONFieldQueryingDeepLookupObjs(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__k__l", "m"),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_shallow_lookup_obj_target
func TestJSONFieldQueryingShallowLookupObjTarget(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__k", map[string]any{"l": "m"}),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_lookup_array
func TestJSONFieldQueryingDeepLookupArray(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__1__0", 2),
			jfIDs(objs[5]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_negative_lookup_array
func TestJSONFieldQueryingDeepNegativeLookupArray(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__-1__0", 2),
			jfIDs(objs[5]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_lookup_mixed
func TestJSONFieldQueryingDeepLookupMixed(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__d__1__f", "g"),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_negative_lookup_mixed
func TestJSONFieldQueryingDeepNegativeLookupMixed(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__d__-1__f", "g"),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_deep_lookup_transform
func TestJSONFieldQueryingDeepLookupTransform(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__c__gt", 2),
			jfIDs(objs[3], objs[4]), jfID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__c__gt", 2.33),
			jfIDs(objs[3], objs[4]), jfID, djangotest.Unordered())
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__c__lt", 5)),
			false)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_lookups_special_chars
func TestJSONFieldQueryingLookupsSpecialChars(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		testKeys := []string{
			"CONTROL",
			"single'",
			"dollar$",
			"dot.dot",
			"with space",
			`back\slash`,
			"question?mark",
			"user@name",
			"emo🤡'ji",
			"com,ma",
			"curly{{{brace}}}s",
			"escape\uffff'seq'\uffffue\uffff'nce",
		}
		jsonValue := make(map[string]any, len(testKeys))
		for _, k := range testKeys {
			jsonValue[k] = "some value"
		}
		raw, err := json.Marshal(jsonValue)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		obj := create(t, db, &jfNullableJSONModel{Value: raw})
		if err := orm.Query[jfNullableJSONModel](db).RefreshFromDB(context.Background(), obj); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		djangotest.AssertJSONEqual(t, string(obj.Value), jsonValue)

		for _, key := range testKeys {
			lookups := []struct {
				name string
				cond orm.Cond
			}{
				{"has_key", orm.Q("value__has_key", key)},
				{"has_keys", orm.Q("value__has_keys", []string{key, "CONTROL"})},
				{"has_any_keys", orm.Q("value__has_any_keys", []string{key, "does_not_exist"})},
				{"exact", orm.Q("value__"+key, "some value")},
			}
			for _, lk := range lookups {
				t.Run(key+"/"+lk.name, func(t *testing.T) {
					djangotest.AssertQuerySetEqual(t,
						orm.Query[jfNullableJSONModel](db).Filter(lk.cond),
						jfIDs(obj), jfID)
				})
			}
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_lookups_special_chars_double_quotes
func TestJSONFieldQueryingLookupsSpecialCharsDoubleQuotes(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		testKeys := []string{
			`double"`,
			"m\\i@x. m🤡'a,t{{{ch}}}e?d$\"'es\uffff'ca\uffff'pe",
		}
		jsonValue := make(map[string]any, len(testKeys))
		for _, k := range testKeys {
			jsonValue[k] = "some value"
		}
		raw, err := json.Marshal(jsonValue)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		obj := create(t, db, &jfNullableJSONModel{Value: raw})
		if err := orm.Query[jfNullableJSONModel](db).RefreshFromDB(context.Background(), obj); err != nil {
			t.Fatalf("RefreshFromDB: %v", err)
		}
		djangotest.AssertJSONEqual(t, string(obj.Value), jsonValue)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__has_keys", testKeys),
			jfIDs(obj), jfID)

		for _, key := range testKeys {
			t.Run(key, func(t *testing.T) {
				qs := orm.Query[jfNullableJSONModel](db).Filter(
					orm.Q("value__has_key", key),
					orm.Q("value__has_any_keys", []string{key, "does_not_exist"}),
					orm.Q("value__"+key, "some value"),
				)
				djangotest.AssertQuerySetEqual(t, qs, jfIDs(obj), jfID)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_lookup_exclude
func TestJSONFieldQueryingLookupExclude(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		tests := []struct {
			cond orm.Cond
			want []*jfNullableJSONModel
		}{
			{orm.Q("value__a", "b"), []*jfNullableJSONModel{objs[0]}},
			{orm.Q("value__foo", "bax"), []*jfNullableJSONModel{objs[0], objs[7]}},
		}
		for _, tc := range tests {
			djangotest.AssertQuerySetEqual(t,
				orm.Query[jfNullableJSONModel](db).Exclude(tc.cond),
				jfIDs(tc.want...), jfID, djangotest.Unordered())
			djangotest.AssertQuerySetEqual(t,
				orm.Query[jfNullableJSONModel](db).Filter(tc.cond.Not()),
				jfIDs(tc.want...), jfID, djangotest.Unordered())
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_lookup_exclude_nonexistent_key
func TestJSONFieldQueryingLookupExcludeNonexistentKey(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		condition := orm.Q("value__foo", "bax")
		objsWithValue := []*jfNullableJSONModel{objs[6]}
		objsWithDifferentValue := []*jfNullableJSONModel{objs[0], objs[7]}
		both := append(append([]*jfNullableJSONModel{}, objsWithValue...), objsWithDifferentValue...)

		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Exclude(condition),
			jfIDs(objsWithDifferentValue...), jfID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Exclude(condition.Not()),
			jfIDs(objsWithValue...), jfID)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter(condition.Or(condition.Not())),
			jfIDs(both...), jfID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Exclude(condition.And(condition.Not())),
			jfIDs(both...), jfID, djangotest.Unordered())

		exhaustive := append(append([]*jfNullableJSONModel{}, objs[0:6]...), objs[7:]...)
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Exclude(condition.And(orm.Q("value__foo__isnull", false))),
			jfIDs(exhaustive...), jfID, djangotest.Unordered())
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter(condition.And(orm.Q("value__foo__isnull", false))),
			jfIDs(objsWithValue...), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_usage_in_subquery
func TestJSONFieldQueryingUsageInSubquery(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).Filter(
			"id__in", orm.Query[jfNullableJSONModel](db).Filter("value__c", 14),
		)
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[3:5]...), jfID, djangotest.Unordered())
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_array_key_contains
func TestJSONFieldQueryingArrayKeyContains(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("supports_json_field_contains")
		}
		tests := []struct {
			name  string
			value any
			want  []*jfNullableJSONModel
		}{
			{"empty list", []any{}, []*jfNullableJSONModel{objs[7]}},
			{"bar string", "bar", []*jfNullableJSONModel{objs[7]}},
			{"bar list", []string{"bar"}, []*jfNullableJSONModel{objs[7]}},
			{"ar string", "ar", nil},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				djangotest.AssertQuerySetEqual(t,
					orm.Query[jfNullableJSONModel](db).Filter("value__bar__contains", tc.value),
					jfIDs(tc.want...), jfID)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_iexact
func TestJSONFieldQueryingKeyIexact(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__iexact", "BaR")),
			true)
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__iexact", `"BaR"`)),
			false)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_in
func TestJSONFieldQueryingKeyIn(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		tests := []struct {
			name   string
			lookup string
			value  any
			want   []*jfNullableJSONModel
		}{
			{"value__c__in [14]", "value__c__in", []int{14}, objs[3:5]},
			{"value__c__in [14, 15]", "value__c__in", []int{14, 15}, objs[3:5]},
			{"value__0__in [1]", "value__0__in", []int{1}, []*jfNullableJSONModel{objs[5]}},
			{"value__0__in [1, 3]", "value__0__in", []int{1, 3}, []*jfNullableJSONModel{objs[5]}},
			{"value__foo__in [bar]", "value__foo__in", []string{"bar"}, []*jfNullableJSONModel{objs[7]}},
			{
				"value__foo__in [KeyTransform]",
				"value__foo__in",
				[]any{orm.KeyTransform("foo", orm.KeyTransform("bax", "value"))},
				[]*jfNullableJSONModel{objs[7]},
			},
			{"value__foo__in [F]", "value__foo__in", []any{orm.F("value__bax__foo")}, []*jfNullableJSONModel{objs[7]}},
			{
				"value__foo__in [KeyTransform, baz]",
				"value__foo__in",
				[]any{orm.KeyTransform("foo", orm.KeyTransform("bax", "value")), "baz"},
				[]*jfNullableJSONModel{objs[7]},
			},
			{"value__foo__in [F, baz]", "value__foo__in", []any{orm.F("value__bax__foo"), "baz"}, []*jfNullableJSONModel{objs[7]}},
			{"value__foo__in [bar, baz]", "value__foo__in", []string{"bar", "baz"}, []*jfNullableJSONModel{objs[7]}},
			{"value__bar__in [[foo, bar]]", "value__bar__in", []any{[]string{"foo", "bar"}}, []*jfNullableJSONModel{objs[7]}},
			{"value__bar__in [[foo, bar], [a]]", "value__bar__in", []any{[]string{"foo", "bar"}, []string{"a"}}, []*jfNullableJSONModel{objs[7]}},
			{"value__bax__in [{foo: bar}, {a: b}]", "value__bax__in", []any{map[string]any{"foo": "bar"}, map[string]any{"a": "b"}}, []*jfNullableJSONModel{objs[7]}},
			{"value__h__in [true, foo]", "value__h__in", []any{true, "foo"}, []*jfNullableJSONModel{objs[4]}},
			{"value__i__in [false, foo]", "value__i__in", []any{false, "foo"}, []*jfNullableJSONModel{objs[4]}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				djangotest.AssertQuerySetEqual(t,
					orm.Query[jfNullableJSONModel](db).Filter(tc.lookup, tc.value),
					jfIDs(tc.want...), jfID, djangotest.Unordered())
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_values
func TestJSONFieldQueryingKeyValues(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).Filter("value__h", true)
		tests := []struct {
			lookup string
			want   string
		}{
			{"value__a", `"b"`},
			{"value__c", `14`},
			{"value__d", `["e", {"f": "g"}]`},
			{"value__h", `true`},
			{"value__i", `false`},
			{"value__j", `null`},
			{"value__k", `{"l": "m"}`},
			{"value__n", `[null, true, false]`},
			{"value__p", `4.2`},
			{"value__r", `{"s": true, "t": false}`},
		}
		for _, tc := range tests {
			t.Run(tc.lookup, func(t *testing.T) {
				vals, err := qs.ValuesList(tc.lookup).Flat(context.Background())
				if err != nil {
					t.Fatalf("Flat: %v", err)
				}
				djangotest.AssertEqual(t, len(vals), 1)
				if tc.want == "null" && vals[0] == nil {
					return
				}
				raw, ok := vals[0].(json.RawMessage)
				if !ok {
					t.Fatalf("got %T (%v), want json.RawMessage", vals[0], vals[0])
				}
				djangotest.AssertJSONEqual(t, string(raw), tc.want)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_values_boolean
func TestJSONFieldQueryingKeyValuesBoolean(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).Filter("value__h", true, "value__i", false)
		tests := []struct {
			lookup string
			want   string
		}{
			{"value__h", "true"},
			{"value__i", "false"},
		}
		for _, tc := range tests {
			t.Run(tc.lookup, func(t *testing.T) {
				vals, err := qs.ValuesList(tc.lookup).Flat(context.Background())
				if err != nil {
					t.Fatalf("Flat: %v", err)
				}
				djangotest.AssertEqual(t, len(vals), 1)
				djangotest.AssertEqual(t, string(vals[0].(json.RawMessage)), tc.want)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_contains
func TestJSONFieldQueryingKeyContains(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("supports_json_field_contains")
		}
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__contains", "ar")),
			false)
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__contains", "bar")),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_icontains
func TestJSONFieldQueryingKeyIcontains(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__icontains", "Ar")),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_startswith
func TestJSONFieldQueryingKeyStartswith(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__startswith", "b")),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_istartswith
func TestJSONFieldQueryingKeyIstartswith(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__istartswith", "B")),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_endswith
func TestJSONFieldQueryingKeyEndswith(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__endswith", "r")),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_iendswith
func TestJSONFieldQueryingKeyIendswith(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__iendswith", "R")),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_regex
func TestJSONFieldQueryingKeyRegex(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__regex", `^bar$`)),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_iregex
func TestJSONFieldQueryingKeyIregex(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		djangotest.AssertEqual(t,
			jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter("value__foo__iregex", `^bAr$`)),
			true)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_quoted_string
func TestJSONFieldQueryingKeyQuotedString(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		got, err := orm.Query[jfNullableJSONModel](db).Filter("value__o", `"quoted"`).Get(context.Background())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, objs[4].ID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_sql_injection
func TestJSONFieldQueryingKeySQLInjection(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("has_json_operators")
		}
		qc := djangotest.CaptureQueries(db)
		exists := jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter(
			`value__test' = '"a"') OR 1 = 1 OR ('d`, "x",
		))
		qc.Stop()
		djangotest.AssertEqual(t, exists, false)
		djangotest.AssertEqual(t, len(qc.Queries()), 1)
		sql := qc.Queries()[0].SQL
		want := `."value" -> 'test'' = ''"a"'') OR 1 = 1 OR (''d') = `
		if !strings.Contains(sql, want) {
			t.Errorf("SQL = %s, want it to contain %s", sql, want)
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_sql_injection_escape
func TestJSONFieldQueryingKeySQLInjectionEscape(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		if db.Dialect().Name() == "postgres" {
			t.Skip("has_json_operators")
		}
		query, _, err := orm.CompileSelectForTest(orm.Query[jfJSONModel](db).Filter(
			`value__test") = '"a"' OR 1 = 1 OR ("d`, "x",
		))
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if !strings.Contains(query, `"test\"`) {
			t.Errorf("query = %s, want it to contain %s", query, `"test\"`)
		}
		if !strings.Contains(query, `\"d`) {
			t.Errorf("query = %s, want it to contain %s", query, `\"d`)
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_escape
func TestJSONFieldQueryingKeyEscape(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		obj := create(t, db, &jfNullableJSONModel{Value: json.RawMessage(`{"%total": 10}`)})
		got, err := orm.Query[jfNullableJSONModel](db).Filter("value__%total", 10).Get(context.Background())
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, obj.ID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_none_key_and_exact_lookup
func TestJSONFieldQueryingNoneKeyAndExactLookup(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		djangotest.AssertQuerySetEqual(t,
			orm.Query[jfNullableJSONModel](db).Filter("value__a", "b", "value__j", nil),
			jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_lookups_with_key_transform
func TestJSONFieldQueryingLookupsWithKeyTransform(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		tests := []struct {
			lookup string
			value  any
		}{
			{"value__baz__has_key", "c"},
			{"value__baz__has_keys", []string{"a", "c"}},
			{"value__baz__has_any_keys", []string{"a", "x"}},
			{"value__has_key", orm.KeyTextTransform("foo", "value")},
		}
		for _, tc := range tests {
			t.Run(tc.lookup, func(t *testing.T) {
				djangotest.AssertEqual(t,
					jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter(tc.lookup, tc.value)),
					true)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_cast_with_key_text_transform
func TestJSONFieldQueryingCastWithKeyTextTransform(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		got, err := orm.Query[jfNullableJSONModel](db).
			Annotate("json_data", orm.Cast(orm.Value(json.RawMessage(`{"foo": "bar"}`)), orm.Field{Kind: orm.KindJSON})).
			Get(context.Background(), "pk", objs[0].ID, "json_data__foo__icontains", "bar")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		djangotest.AssertEqual(t, got.ID, objs[0].ID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_contains_contained_by_with_key_transform
func TestJSONFieldQueryingContainsContainedByWithKeyTransform(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("supports_json_field_contains")
		}
		// Django's RawSQL subcase is omitted because the ORM has no RawSQL expression.
		tests := []struct {
			name   string
			lookup string
			value  any
		}{
			{"value__d__contains e", "value__d__contains", "e"},
			{"value__d__contains [{f: g}]", "value__d__contains", json.RawMessage(`[{"f": "g"}]`)},
			{"value__contains KeyTransform(bax)", "value__contains", orm.KeyTransform("bax", "value")},
			{"value__contains F(value__bax)", "value__contains", orm.F("value__bax")},
			{"value__baz__contains {a: b}", "value__baz__contains", json.RawMessage(`{"a": "b"}`)},
			{"value__baz__contained_by {a: b, c: d, e: f}", "value__baz__contained_by", json.RawMessage(`{"a": "b", "c": "d", "e": "f"}`)},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				djangotest.AssertEqual(t,
					jfMustExists(t, orm.Query[jfNullableJSONModel](db).Filter(tc.lookup, tc.value)),
					true)
			})
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_join_key_transform_annotation_expression
func TestJSONFieldQueryingJoinKeyTransformAnnotationExpression(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		relatedObj := create(t, db, &jfRelatedJSONModel{
			Value:     json.RawMessage(`{"d": ["f", "e"]}`),
			JSONModel: fkTo[jfNullableJSONModel](objs[4].ID),
		})
		create(t, db, &jfRelatedJSONModel{
			Value:     json.RawMessage(`{"d": ["e", "f"]}`),
			JSONModel: fkTo[jfNullableJSONModel](objs[4].ID),
		})
		qs := orm.Query[jfRelatedJSONModel](db).
			Annotate(
				"key", orm.F("value__d"),
				"related_key", orm.F("json_model__value__d"),
				"chain", orm.F("key__1"),
				"expr", orm.Cast("key", orm.Field{Kind: orm.KindJSON}),
			).
			Filter("chain", orm.F("related_key__0"))
		djangotest.AssertQuerySetEqual(t, qs, []int64{relatedObj.ID}, func(r jfRelatedJSONModel) int64 { return r.ID })
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_text_transform_from_lookup
func TestJSONFieldQueryingKeyTextTransformFromLookup(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		qs := orm.Query[jfNullableJSONModel](db).
			Annotate("b", orm.KT("value__bax__foo")).
			Filter("b__contains", "ar")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[7]), jfID)

		qs = orm.Query[jfNullableJSONModel](db).
			Annotate("c", orm.KT("value__o")).
			Filter("c__contains", "uot")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs[4]), jfID)
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_key_text_transform_from_lookup_invalid
func TestJSONFieldQueryingKeyTextTransformFromLookupInvalid(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, _ []*jfNullableJSONModel) {
		for _, lookup := range []string{"value", ""} {
			_, err := orm.Query[jfNullableJSONModel](db).Annotate("k", orm.KT(lookup)).All(context.Background())
			assertErrorContains(t, err, "Lookup must contain key or index transforms.")
		}
	})
}

// Ported from Django: tests/model_fields/test_jsonfield.py::TestQuerying.test_literal_annotation_filtering
func TestJSONFieldQueryingLiteralAnnotationFiltering(t *testing.T) {
	jfEachQuerying(t, func(t *testing.T, db *orm.DB, objs []*jfNullableJSONModel) {
		allObjects := orm.Query[jfNullableJSONModel](db).OrderBy("id")
		qs := allObjects.
			Annotate("data", orm.Value(json.RawMessage(`{"foo": "bar"}`))).
			Filter("data__foo", "bar")
		djangotest.AssertQuerySetEqual(t, qs, jfIDs(objs...), jfID)
	})
}
