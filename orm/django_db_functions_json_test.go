package orm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// fnJSONModels returns the models of the ports of tests/db_functions/json.
func fnJSONModels() []any {
	return []any{djangotest.App("db_functions", &fnAuthor{}, &fnArticle{})}
}

// assertFirstJSON fails t unless the annotation name of the first author, by
// primary key, is the JSON document want.
func assertFirstJSON(t *testing.T, qs *orm.QuerySet[fnAuthor], name, want string) {
	t.Helper()
	raw, ok := firstValue(t, qs.OrderBy("pk").ValuesList(name)).(json.RawMessage)
	if !ok {
		t.Fatalf("%s is not a json.RawMessage", name)
	}
	djangotest.AssertJSONEqual(t, string(raw), want)
}

// fnJSONArrayAuthor creates the author of Django's JSONArrayTests.
func fnJSONArrayAuthor(t *testing.T, db *orm.DB) {
	t.Helper()
	create(t, db, &fnAuthor{Name: "Ivan Ivanov", Alias: new("iivanov")})
}

// fnJSONObjectAuthors creates the authors of Django's JSONObjectTests.
func fnJSONObjectAuthors(t *testing.T, db *orm.DB) {
	t.Helper()
	create(t, db, &fnAuthor{Name: "Ivan Ivanov", Alias: new("iivanov")})
	create(t, db, &fnAuthor{Name: "Bertha Berthy", Alias: new("bberthy")})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_empty
func TestJSONArrayEmpty(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		assertFirstJSON(t, orm.Query[fnAuthor](db).Annotate("json_array", orm.JSONArray()), "json_array", `[]`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_basic
func TestJSONArrayBasic(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_array", orm.JSONArray(orm.Value("name"), orm.F("name")))
		assertFirstJSON(t, qs, "json_array", `["name", "Ivan Ivanov"]`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_expressions
func TestJSONArrayExpressions(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_array", orm.JSONArray(
			orm.Lower("name"), orm.F("alias"), orm.F("goes_by"), orm.Value(30000.15), orm.F("age").Mul(2)))
		assertFirstJSON(t, qs, "json_array", `["ivan ivanov", "iivanov", null, 30000.15, 60]`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_nested_json_array
func TestJSONArrayNestedJSONArray(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_array",
			orm.JSONArray(orm.F("name"), orm.JSONArray(orm.F("alias"), orm.F("age"))))
		assertFirstJSON(t, qs, "json_array", `["Ivan Ivanov", ["iivanov", 30]]`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_nested_empty_json_array
func TestJSONArrayNestedEmptyJSONArray(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_array", orm.JSONArray(orm.F("name"), orm.JSONArray()))
		assertFirstJSON(t, qs, "json_array", `["Ivan Ivanov", []]`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_textfield
func TestJSONArrayTextfield(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		text := strings.Repeat("x", 4000)
		create(t, db, &fnArticle{Title: "The Title", Text: text, Written: time.Now()})
		raw := firstValue(t, orm.Query[fnArticle](db).Annotate("json_array", orm.JSONArray(orm.F("text"))).ValuesList("json_array"))
		djangotest.AssertJSONEqual(t, string(raw.(json.RawMessage)), []any{text})
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_explicit_cast
//
// The type is written in upper case.
func TestJSONArrayExplicitCast(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() != "postgres" {
			t.Skip("PostgreSQL specific tests")
		}
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_array", orm.JSONArray(orm.Cast("age", orm.Field{Kind: orm.KindChar, MaxLength: 255})))
		capture := djangotest.CaptureQueries(db)
		rows, err := qs.Values("json_array").All(context.Background())
		capture.Stop()
		if err != nil {
			t.Fatalf("Values: %v", err)
		}
		djangotest.AssertEqual(t, len(rows), 1)
		djangotest.AssertJSONEqual(t, string(rows[0]["json_array"].(json.RawMessage)), `["30"]`)
		djangotest.AssertEqual(t, len(capture.Queries()), 1)
		sql := capture.Queries()[0].SQL
		if !strings.Contains(sql, "::VARCHAR") || strings.Contains(sql, "::VARCHAR(255))::VARCHAR") {
			t.Errorf("SQL = %s, want one cast to VARCHAR", sql)
		}
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_order_by_key
func TestJSONArrayOrderByKey(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("arr", orm.JSONArray(orm.F("alias"))).OrderBy("arr__0")
		rawSameRows(t, mustAll(t, qs), mustAll(t, orm.Query[fnAuthor](db).OrderBy("alias")))
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayTests.test_order_by_nested_key
func TestJSONArrayOrderByNestedKey(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("arr", orm.JSONArray(orm.JSONArray(orm.F("alias")))).OrderBy("-arr__0__0")
		rawSameRows(t, mustAll(t, qs), mustAll(t, orm.Query[fnAuthor](db).OrderBy("-alias")))
	})
}

// No Go counterpart: tests/db_functions/json/test_json_array.py::JSONArrayNotSupportedTests.test_not_supported
// runs on a database without JSON support. SQLite and PostgreSQL have it.

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayObjectTests.test_nested_json_array_object
func TestJSONArrayNestedJSONArrayObject(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_array", orm.JSONArray(
			orm.JSONObject("name1", "name", "nested_json_object1", orm.JSONObject("alias1", "alias", "age1", "age")),
			orm.JSONObject("name2", "name", "nested_json_object2", orm.JSONObject("alias2", "alias", "age2", "age")),
		))
		assertFirstJSON(t, qs, "json_array", `[
			{"name1": "Ivan Ivanov", "nested_json_object1": {"alias1": "iivanov", "age1": 30}},
			{"name2": "Ivan Ivanov", "nested_json_object2": {"alias2": "iivanov", "age2": 30}}
		]`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayObjectTests.test_nested_json_object_array
func TestJSONArrayNestedJSONObjectArray(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_object", orm.JSONObject(
			"name", "name",
			"nested_json_array", orm.JSONArray(
				orm.JSONObject("alias1", "alias", "age1", "age"),
				orm.JSONObject("alias2", "alias", "age2", "age"),
			),
		))
		assertFirstJSON(t, qs, "json_object", `{
			"name": "Ivan Ivanov",
			"nested_json_array": [{"alias1": "iivanov", "age1": 30}, {"alias2": "iivanov", "age2": 30}]
		}`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_array.py::JSONArrayObjectTests.test_order_by_nested_key
func TestJSONArrayObjectOrderByNestedKey(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONArrayAuthor(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("arr", orm.JSONArray(orm.JSONObject("alias", orm.F("alias")))).OrderBy("-arr__0__alias")
		rawSameRows(t, mustAll(t, qs), mustAll(t, orm.Query[fnAuthor](db).OrderBy("-alias")))
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_empty
func TestJSONObjectEmpty(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		assertFirstJSON(t, orm.Query[fnAuthor](db).Annotate("json_object", orm.JSONObject()), "json_object", `{}`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_basic
func TestJSONObjectBasic(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_object", orm.JSONObject("name", "name"))
		assertFirstJSON(t, qs, "json_object", `{"name": "Ivan Ivanov"}`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_expressions
func TestJSONObjectExpressions(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_object", orm.JSONObject(
			"name", orm.Lower("name"),
			"alias", "alias",
			"goes_by", "goes_by",
			"salary", orm.Value(30000.15),
			"age", orm.F("age").Mul(2),
		))
		assertFirstJSON(t, qs, "json_object",
			`{"name": "ivan ivanov", "alias": "iivanov", "goes_by": null, "salary": 30000.15, "age": 60}`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_nested_json_object
func TestJSONObjectNestedJSONObject(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_object",
			orm.JSONObject("name", "name", "nested_json_object", orm.JSONObject("alias", "alias", "age", "age")))
		assertFirstJSON(t, qs, "json_object", `{"name": "Ivan Ivanov", "nested_json_object": {"alias": "iivanov", "age": 30}}`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_nested_empty_json_object
func TestJSONObjectNestedEmptyJSONObject(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("json_object", orm.JSONObject("name", "name", "nested_json_object", orm.JSONObject()))
		assertFirstJSON(t, qs, "json_object", `{"name": "Ivan Ivanov", "nested_json_object": {}}`)
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_textfield
func TestJSONObjectTextfield(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		text := strings.Repeat("x", 4000)
		create(t, db, &fnArticle{Title: "The Title", Text: text, Written: time.Now()})
		raw := firstValue(t, orm.Query[fnArticle](db).Annotate("json_object", orm.JSONObject("text", orm.F("text"))).ValuesList("json_object"))
		djangotest.AssertJSONEqual(t, string(raw.(json.RawMessage)), map[string]any{"text": text})
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_order_by_key
func TestJSONObjectOrderByKey(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("attrs", orm.JSONObject("alias", orm.F("alias"))).OrderBy("attrs__alias")
		rawSameRows(t, mustAll(t, qs), mustAll(t, orm.Query[fnAuthor](db).OrderBy("alias")))
	})
}

// Ported from Django: tests/db_functions/json/test_json_object.py::JSONObjectTests.test_order_by_nested_key
func TestJSONObjectOrderByNestedKey(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnJSONObjectAuthors(t, db)
		qs := orm.Query[fnAuthor](db).Annotate("attrs", orm.JSONObject("nested", orm.JSONObject("alias", orm.F("alias")))).OrderBy("-attrs__nested__alias")
		rawSameRows(t, mustAll(t, qs), mustAll(t, orm.Query[fnAuthor](db).OrderBy("-alias")))
	})
}

// No Go counterpart: tests/db_functions/json/test_json_object.py::JSONObjectNotSupportedTests.test_not_supported
// runs on a database without a JSON object function. SQLite and PostgreSQL
// have one.

// No Django test exists: JSONObject refuses an odd number of arguments and a
// key that is not a string, the errors of Go's form of keyword arguments.
func TestJSONObjectInvalidPairs(t *testing.T) {
	djangotest.EachDB(t, fnJSONModels(), func(t *testing.T, db *orm.DB) {
		fnAssertQueryError(t, orm.Query[fnAuthor](db).Annotate("o", orm.JSONObject("name")).ValuesList("o"),
			"JSONObject takes pairs of a key and a value, not 1 arguments")
		fnAssertQueryError(t, orm.Query[fnAuthor](db).Annotate("o", orm.JSONObject(1, "name")).ValuesList("o"),
			"the JSONObject key 1 is a int, not a string")
	})
}
