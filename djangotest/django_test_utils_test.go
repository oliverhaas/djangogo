package djangotest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The tests in this file are ports of Django's tests/test_utils/tests.py: the
// tests of the HTML, JSON, URL, query-count, and queryset assertions. The
// tests that look at the parsed HTML tree are in django_html_parser_test.go.

// Car is the port of tests/test_utils/models.py::Car.
type Car struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// Person is the port of tests/test_utils/models.py::Person. The Django model
// also has a many-to-many relation to Car through PossessedCar and a binary
// field, which the tests ported here do not need.
type Person struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// PossessedCar is the port of tests/test_utils/models.py::PossessedCar.
type PossessedCar struct {
	ID        int64
	Car       orm.FK[Car]    `orm:"on_delete=cascade"`
	BelongsTo orm.FK[Person] `orm:"on_delete=cascade"`
}

// utilsModels are the models of Django's test_utils app.
var utilsModels = []any{djangotest.App("test_utils", &Car{}, &Person{}, &PossessedCar{})}

// createPerson is Person.objects.create(name=name).
func createPerson(t *testing.T, db *orm.DB, name string) Person {
	t.Helper()
	p := Person{Name: name}
	if err := orm.Query[Person](db).Create(context.Background(), &p); err != nil {
		t.Fatalf("create person %q: %v", name, err)
	}
	return p
}

// countPersons is Person.objects.count(), one query.
func countPersons(t *testing.T, db *orm.DB) {
	t.Helper()
	if _, err := orm.Query[Person](db).Count(context.Background()); err != nil {
		t.Fatalf("count persons: %v", err)
	}
}

// personHandler is the port of tests/test_utils/urls.py and the get_person view
// of tests/test_utils/views.py.
func personHandler(db *orm.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/test_utils/get_person/{pk}/{$}", func(w http.ResponseWriter, r *http.Request) {
		pk, err := strconv.ParseInt(r.PathValue("pk"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		person, err := orm.Query[Person](db).Get(r.Context(), "id", pk)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, person.Name)
	})
	return mux
}

// personURL is the URL of the get_person view for p.
func personURL(p Person) string {
	return fmt.Sprintf("/test_utils/get_person/%d/", p.ID)
}

// TestAssertNumQueries checks that a failure of the function under count
// reaches the caller: a Python exception there, a panic here.
//
// Ported from Django: tests/test_utils/tests.py::AssertNumQueriesTests.test_assert_num_queries
func TestAssertNumQueries(t *testing.T) {
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		errValue := errors.New("value error")
		defer func() {
			if err, _ := recover().(error); !errors.Is(err, errValue) {
				t.Errorf("recovered %v, want the panic of the function", err)
			}
		}()
		djangotest.AssertNumQueries(t, db, 2, func() { panic(errValue) })
		t.Error("the panic of the function did not reach the test")
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertNumQueriesTests.test_assert_num_queries_with_client
// Ported from Django: tests/test_utils/tests.py::AssertNumQueriesContextManagerTests.test_with_client
func TestAssertNumQueriesWithClient(t *testing.T) {
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		person := createPerson(t, db, "test")
		c := djangotest.NewClient(t, personHandler(db))

		djangotest.AssertNumQueries(t, db, 1, func() { c.Get(personURL(person)) })
		djangotest.AssertNumQueries(t, db, 1, func() { c.Get(personURL(person)) })
		djangotest.AssertNumQueries(t, db, 2, func() {
			c.Get(personURL(person))
			c.Get(personURL(person))
		})
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertNumQueriesContextManagerTests.test_simple
func TestAssertNumQueriesSimple(t *testing.T) {
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		djangotest.AssertNumQueries(t, db, 0, func() {})
		djangotest.AssertNumQueries(t, db, 1, func() { countPersons(t, db) })
		djangotest.AssertNumQueries(t, db, 2, func() {
			countPersons(t, db)
			countPersons(t, db)
		})
	})
}

// TestAssertNumQueriesFailure checks the failure message. The Django message
// starts with "1 != 2 : ", which unittest adds.
//
// Ported from Django: tests/test_utils/tests.py::AssertNumQueriesContextManagerTests.test_failure
func TestAssertNumQueriesFailure(t *testing.T) {
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		msg := "1 queries executed, 2 expected\nCaptured queries were:\n1."
		mustFail(t, msg, func(ft djangotest.TB) {
			djangotest.AssertNumQueries(ft, db, 2, func() { countPersons(t, db) })
		})
	})
}

// TestCaptureQueriesSimple checks that a capture records the queries run while
// it is active. Django looks for the primary key in the SQL text; the Go ORM
// sends it as a query argument.
//
// Ported from Django: tests/test_utils/tests.py::CaptureQueriesContextManagerTests.test_simple
// Ported from Django: tests/test_utils/tests.py::CaptureQueriesContextManagerTests.test_within
func TestCaptureQueriesSimple(t *testing.T) {
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		person := createPerson(t, db, "test")

		captured := djangotest.CaptureQueries(db)
		if _, err := orm.Query[Person](db).Get(context.Background(), "id", person.ID); err != nil {
			t.Fatal(err)
		}
		// The queries are available while the capture is still active.
		djangotest.AssertEqual(t, captured.Len(), 1)
		captured.Stop()
		djangotest.AssertEqual(t, captured.Len(), 1)
		if !slices.Contains(captured.Queries()[0].Args, any(person.ID)) {
			t.Errorf("query arguments %v do not hold the primary key %d", captured.Queries()[0].Args, person.ID)
		}

		captured = djangotest.CaptureQueries(db)
		captured.Stop()
		djangotest.AssertEqual(t, captured.Len(), 0)
	})
}

// Ported from Django: tests/test_utils/tests.py::CaptureQueriesContextManagerTests.test_nested
func TestCaptureQueriesNested(t *testing.T) {
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		captured := djangotest.CaptureQueries(db)
		countPersons(t, db)
		nested := djangotest.CaptureQueries(db)
		countPersons(t, db)
		nested.Stop()
		captured.Stop()

		djangotest.AssertEqual(t, nested.Len(), 1)
		djangotest.AssertEqual(t, captured.Len(), 2)
	})
}

// Ported from Django: tests/test_utils/tests.py::CaptureQueriesContextManagerTests.test_with_client
func TestCaptureQueriesWithClient(t *testing.T) {
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		person := createPerson(t, db, "test")
		c := djangotest.NewClient(t, personHandler(db))

		captured := djangotest.CaptureQueries(db)
		c.Get(personURL(person))
		captured.Stop()

		djangotest.AssertEqual(t, captured.Len(), 1)
		if !slices.Contains(captured.Queries()[0].Args, any(person.ID)) {
			t.Errorf("query arguments %v do not hold the primary key %d", captured.Queries()[0].Args, person.ID)
		}
	})
}

// eachPersonDB runs fn on every database with the two persons the Django test
// class creates in setUpTestData.
func eachPersonDB(t *testing.T, fn func(t *testing.T, db *orm.DB, p1, p2 Person)) {
	t.Helper()
	djangotest.EachDB(t, utilsModels, func(t *testing.T, db *orm.DB) {
		fn(t, db, createPerson(t, db, "p1"), createPerson(t, db, "p2"))
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertQuerySetEqualTests.test_empty
func TestAssertQuerySetEqualEmpty(t *testing.T) {
	eachPersonDB(t, func(t *testing.T, db *orm.DB, _, _ Person) {
		djangotest.AssertQuerySetEqual(t, orm.Query[Person](db).Filter("name", "p3"), []Person{}, nil)
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertQuerySetEqualTests.test_ordered
func TestAssertQuerySetEqualOrdered(t *testing.T) {
	eachPersonDB(t, func(t *testing.T, db *orm.DB, p1, p2 Person) {
		djangotest.AssertQuerySetEqual(t, orm.Query[Person](db).OrderBy("name"), []Person{p1, p2}, nil)
		mustFail(t, "sequences differ", func(ft djangotest.TB) {
			djangotest.AssertQuerySetEqual(ft, orm.Query[Person](db).OrderBy("name"), []Person{p2, p1}, nil)
		})
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertQuerySetEqualTests.test_unordered
func TestAssertQuerySetEqualUnordered(t *testing.T) {
	eachPersonDB(t, func(t *testing.T, db *orm.DB, p1, p2 Person) {
		djangotest.AssertQuerySetEqual(t, orm.Query[Person](db).OrderBy("name"), []Person{p2, p1}, nil,
			djangotest.Unordered())
	})
}

// TestAssertQuerySetEqualFlatValuesList compares the names of the rows. Django
// uses values_list("name", flat=True); the transform does the same job here.
//
// Ported from Django: tests/test_utils/tests.py::AssertQuerySetEqualTests.test_flat_values_list
func TestAssertQuerySetEqualFlatValuesList(t *testing.T) {
	eachPersonDB(t, func(t *testing.T, db *orm.DB, _, _ Person) {
		djangotest.AssertQuerySetEqual(t, orm.Query[Person](db).OrderBy("name"), []string{"p1", "p2"},
			func(p Person) string { return p.Name })
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertQuerySetEqualTests.test_transform
func TestAssertQuerySetEqualTransform(t *testing.T) {
	eachPersonDB(t, func(t *testing.T, db *orm.DB, p1, p2 Person) {
		djangotest.AssertQuerySetEqual(t, orm.Query[Person](db).OrderBy("name"), []int64{p1.ID, p2.ID},
			func(p Person) int64 { return p.ID })
	})
}

// TestAssertQuerySetEqualRepeatedValues checks that the number of appearances
// of each item counts when the order does not. Django reads the cars through
// the many-to-many relation (p1.cars.all()); the port reads the rows of the
// through model and compares the car of each.
//
// Ported from Django: tests/test_utils/tests.py::AssertQuerySetEqualTests.test_repeated_values
func TestAssertQuerySetEqualRepeatedValues(t *testing.T) {
	eachPersonDB(t, func(t *testing.T, db *orm.DB, p1, _ Person) {
		ctx := context.Background()
		batmobile, k2000 := Car{Name: "Batmobile"}, Car{Name: "K 2000"}
		for _, car := range []*Car{&batmobile, &k2000} {
			if err := orm.Query[Car](db).Create(ctx, car); err != nil {
				t.Fatal(err)
			}
		}
		for _, car := range []Car{batmobile, batmobile, k2000, k2000, k2000, k2000} {
			possessed := PossessedCar{}
			possessed.Car.SetPK(car.ID)
			possessed.BelongsTo.SetPK(p1.ID)
			if err := orm.Query[PossessedCar](db).Create(ctx, &possessed); err != nil {
				t.Fatal(err)
			}
		}
		cars := orm.Query[PossessedCar](db).Filter("belongs_to_id", p1.ID)
		carID := func(pc PossessedCar) int64 { return pc.Car.PK() }

		mustFail(t, "sequences differ", func(ft djangotest.TB) {
			djangotest.AssertQuerySetEqual(ft, cars, []int64{batmobile.ID, k2000.ID}, carID, djangotest.Unordered())
		})
		want := []int64{batmobile.ID, batmobile.ID, k2000.ID, k2000.ID, k2000.ID, k2000.ID}
		djangotest.AssertQuerySetEqual(t, cars, want, carID, djangotest.Unordered())
	})
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_simple_equal_html
func TestSimpleEqualHTML(t *testing.T) {
	pairs := [][2]string{
		{"", ""},
		{"<p></p>", "<p></p>"},
		{"<p></p>", " <p> </p> "},
		{"<div><p>Hello</p></div>", "<div><p>Hello</p></div>"},
		{"<div><p>Hello</p></div>", "<div> <p>Hello</p> </div>"},
		{"<div>\n<p>Hello</p></div>", "<div><p>Hello</p></div>\n"},
		{"<div><p>Hello\nWorld !</p></div>", "<div><p>Hello World\n!</p></div>"},
		{"<p>Hello  World   !</p>", "<p>Hello World\n\n!</p>"},
		{"<p> </p>", "<p></p>"},
		{"<p/>", "<p></p>"},
		{"<p />", "<p></p>"},
		{"<input checked>", `<input checked="checked">`},
		{"<p>Hello", "<p> Hello"},
		{"<p>Hello</p>World", "<p>Hello</p> World"},
	}
	for _, pair := range pairs {
		t.Run(pair[0], func(t *testing.T) {
			djangotest.AssertHTMLEqual(t, pair[0], pair[1])
		})
	}
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_ignore_comments
func TestIgnoreComments(t *testing.T) {
	djangotest.AssertHTMLEqual(t,
		"<div>Hello<!-- this is a comment --> World!</div>",
		"<div>Hello World!</div>",
	)
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_unequal_html
func TestUnequalHTML(t *testing.T) {
	pairs := [][2]string{
		{"<p>Hello</p>", "<p>Hello!</p>"},
		{"<p>foo&#20;bar</p>", "<p>foo&nbsp;bar</p>"},
		{"<p>foo bar</p>", "<p>foo &nbsp;bar</p>"},
		{"<p>foo nbsp</p>", "<p>foo &nbsp;</p>"},
		{"<p>foo #20</p>", "<p>foo &#20;</p>"},
		{"<p><span>Hello</span><span>World</span></p>", "<p><span>Hello</span>World</p>"},
		{"<p><span>Hello</span>World</p>", "<p><span>Hello</span><span>World</span></p>"},
	}
	for _, pair := range pairs {
		t.Run(pair[0], func(t *testing.T) {
			djangotest.AssertHTMLNotEqual(t, pair[0], pair[1])
		})
	}
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_attributes
func TestAttributes(t *testing.T) {
	djangotest.AssertHTMLEqual(t,
		`<input type="text" id="id_name" />`, `<input id="id_name" type="text" />`)
	djangotest.AssertHTMLEqual(t,
		`<input type='text' id="id_name" />`, `<input id="id_name" type="text" />`)
	djangotest.AssertHTMLNotEqual(t,
		`<input type="text" id="id_name" />`, `<input type="password" id="id_name" />`)
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_class_attribute
func TestClassAttribute(t *testing.T) {
	pairs := [][2]string{
		{`<p class="foo bar"></p>`, `<p class="bar foo"></p>`},
		{`<p class=" foo bar "></p>`, `<p class="bar foo"></p>`},
		{`<p class="   foo    bar    "></p>`, `<p class="bar foo"></p>`},
		{"<p class=\"foo\tbar\"></p>", `<p class="bar foo"></p>`},
		{"<p class=\"\tfoo\tbar\t\"></p>", `<p class="bar foo"></p>`},
		{"<p class=\"\t\t\tfoo\t\t\tbar\t\t\t\"></p>", `<p class="bar foo"></p>`},
		{"<p class=\"\t \nfoo \t\nbar\n\t \"></p>", `<p class="bar foo"></p>`},
	}
	for _, pair := range pairs {
		t.Run(pair[0], func(t *testing.T) {
			djangotest.AssertHTMLEqual(t, pair[0], pair[1])
		})
	}
}

// TestBooleanAttribute checks the three spellings of a boolean attribute. The
// str(parse_html(...)) checks of the Django test are in
// django_html_parser_test.go.
//
// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_boolean_attribute
func TestBooleanAttribute(t *testing.T) {
	html1 := "<input checked>"
	html2 := `<input checked="">`
	html3 := `<input checked="checked">`
	djangotest.AssertHTMLEqual(t, html1, html2)
	djangotest.AssertHTMLEqual(t, html1, html3)
	djangotest.AssertHTMLEqual(t, html2, html3)
	djangotest.AssertHTMLNotEqual(t, html1, `<input checked="invalid">`)
}

// TestNonBooleanAttributes keeps the meaning of the Django test, whose name has
// a typo (test_non_boolean_attibutes).
//
// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_non_boolean_attibutes
func TestNonBooleanAttributes(t *testing.T) {
	html1 := "<input value>"
	html2 := `<input value="">`
	html3 := `<input value="value">`
	djangotest.AssertHTMLEqual(t, html1, html2)
	djangotest.AssertHTMLNotEqual(t, html1, html3)
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_normalize_refs
func TestNormalizeRefs(t *testing.T) {
	pairs := [][2]string{
		{"&#39;", "&#x27;"},
		{"&#39;", "'"},
		{"&#x27;", "&#39;"},
		{"&#x27;", "'"},
		{"'", "&#39;"},
		{"'", "&#x27;"},
		{"&amp;", "&#38;"},
		{"&amp;", "&#x26;"},
		{"&amp;", "&"},
		{"&#38;", "&amp;"},
		{"&#38;", "&#x26;"},
		{"&#38;", "&"},
		{"&#x26;", "&amp;"},
		{"&#x26;", "&#38;"},
		{"&#x26;", "&"},
		{"&", "&amp;"},
		{"&", "&#38;"},
		{"&", "&#x26;"},
	}
	for _, pair := range pairs {
		t.Run(pair[0]+" "+pair[1], func(t *testing.T) {
			djangotest.AssertHTMLEqual(t, pair[0], pair[1])
		})
	}
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_complex_examples
func TestComplexExamples(t *testing.T) {
	djangotest.AssertHTMLEqual(t,
		`<tr><th><label for="id_first_name">First name:</label></th>
<td><input type="text" name="first_name" value="John" id="id_first_name" /></td></tr>
<tr><th><label for="id_last_name">Last name:</label></th>
<td><input type="text" id="id_last_name" name="last_name" value="Lennon" /></td></tr>
<tr><th><label for="id_birthday">Birthday:</label></th>
<td><input type="text" value="1940-10-9" name="birthday" id="id_birthday" /></td></tr>`,
		`
        <tr><th>
            <label for="id_first_name">First name:</label></th><td>
            <input type="text" name="first_name" value="John" id="id_first_name" />
        </td></tr>
        <tr><th>
            <label for="id_last_name">Last name:</label></th><td>
            <input type="text" name="last_name" value="Lennon" id="id_last_name" />
        </td></tr>
        <tr><th>
            <label for="id_birthday">Birthday:</label></th><td>
            <input type="text" name="birthday" value="1940-10-9" id="id_birthday" />
        </td></tr>
        `,
	)

	djangotest.AssertHTMLEqual(t,
		`<!DOCTYPE html>
        <html>
        <head>
            <link rel="stylesheet">
            <title>Document</title>
            <meta attribute="value">
        </head>
        <body>
            <p>
            This is a valid paragraph
            <div> this is a div AFTER the p</div>
        </body>
        </html>`,
		`
        <html>
        <head>
            <link rel="stylesheet">
            <title>Document</title>
            <meta attribute="value">
        </head>
        <body>
            <p> This is a valid paragraph
            <!-- browsers would close the p tag here -->
            <div> this is a div AFTER the p</div>
            </p> <!-- this is invalid HTML parsing, but it should make no
            difference in most cases -->
        </body>
        </html>`,
	)
}

// TestParsingErrors checks how invalid HTML fails an assertion. Django's
// message quotes a Python exception ("('Unexpected end tag `div` (Line 1,
// Column 6)', (1, 6))"); the Go message carries the same facts as an error
// text. The parse_html("</p>") check of the Django test is in
// django_html_parser_test.go.
//
// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_parsing_errors
func TestParsingErrors(t *testing.T) {
	mustFail(t, "", func(ft djangotest.TB) { djangotest.AssertHTMLEqual(ft, "<p>", "") })
	mustFail(t, "", func(ft djangotest.TB) { djangotest.AssertHTMLEqual(ft, "", "<p>") })

	msg := "First argument is not valid HTML:\nunexpected end tag `div` (line 1, column 6)"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertHTMLEqual(ft, "< div></div>", "<div></div>")
	})
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_escaped_html_errors
func TestEscapedHTMLErrors(t *testing.T) {
	msg := "<p>\n<foo>\n</p> != <p>\n&lt;foo&gt;\n</p>\n"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertHTMLEqual(ft, "<p><foo></p>", "<p>&lt;foo&gt;</p>")
	})
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertHTMLEqual(ft, "<p><foo></p>", "<p>&#60;foo&#62;</p>")
	})
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_contains_html
func TestContainsHTML(t *testing.T) {
	resp := &djangotest.Response{StatusCode: http.StatusOK, Body: []byte(`<body>
        This is a form: <form method="get">
            <input type="text" name="Hello" />
        </form></body>`)}

	djangotest.AssertNotContains(t, resp, "<input name='Hello' type='text'>")
	djangotest.AssertContains(t, resp, `<form method="get">`)

	djangotest.AssertContains(t, resp, "<input name='Hello' type='text'>", djangotest.HTML())
	djangotest.AssertNotContains(t, resp, `<form method="get">`, djangotest.HTML())

	invalid := &djangotest.Response{StatusCode: http.StatusOK, Body: []byte(`<body <bad>>`)}
	mustFail(t, "", func(ft djangotest.TB) {
		djangotest.AssertContains(ft, invalid, "<p></p>")
	})
	mustFail(t, "", func(ft djangotest.TB) {
		djangotest.AssertContains(ft, resp, `<p "whats" that>`)
	})
}

// Ported from Django: tests/test_utils/tests.py::HTMLEqualTests.test_unicode_handling
func TestUnicodeHandling(t *testing.T) {
	content := `<p class="help">Some help text for the title (with Unicode ŠĐĆŽćžšđ)</p>`
	resp := &djangotest.Response{StatusCode: http.StatusOK, Body: []byte(content)}
	djangotest.AssertContains(t, resp, content, djangotest.HTML())
}

// The messages of the InHTMLTests start with "False is not true : " or
// "2 != 1 : " in Django, which unittest adds; the Go messages are the part
// after it.

// Ported from Django: tests/test_utils/tests.py::InHTMLTests.test_needle_msg
func TestNeedleMsg(t *testing.T) {
	msg := "Couldn't find '<b>Hello</b>' in the following response\n'<p>Test</p>'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertInHTML(ft, "<b>Hello</b>", "<p>Test</p>")
	})
}

// Ported from Django: tests/test_utils/tests.py::InHTMLTests.test_msg_prefix
func TestMsgPrefix(t *testing.T) {
	msg := "Prefix: Couldn't find '<b>Hello</b>' in the following " +
		"response\n'<input type=\"text\" name=\"Hello\" />'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertInHTML(ft, "<b>Hello</b>", `<input type="text" name="Hello" />`,
			djangotest.MsgPrefix("Prefix"))
	})
}

// Ported from Django: tests/test_utils/tests.py::InHTMLTests.test_count_msg_prefix
func TestCountMsgPrefix(t *testing.T) {
	msg := "Prefix: Found 2 instances of '<b>Hello</b>' (expected 1) in the " +
		"following response\n'<b>Hello</b><b>Hello</b>'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertInHTML(ft, "<b>Hello</b>", "<b>Hello</b><b>Hello</b>",
			djangotest.Count(1), djangotest.MsgPrefix("Prefix"))
	})
}

// Ported from Django: tests/test_utils/tests.py::InHTMLTests.test_base
func TestBase(t *testing.T) {
	haystack := "<p><b>Hello</b> <span>there</span>! Hi <span>there</span>!</p>"

	djangotest.AssertInHTML(t, "<b>Hello</b>", haystack)
	msg := "Couldn't find '<p>Howdy</p>' in the following response\n'" + haystack + "'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertInHTML(ft, "<p>Howdy</p>", haystack)
	})

	djangotest.AssertInHTML(t, "<span>there</span>", haystack, djangotest.Count(2))
	msg = "Found 1 instances of '<b>Hello</b>' (expected 2) in the following response\n'" + haystack + "'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertInHTML(ft, "<b>Hello</b>", haystack, djangotest.Count(2))
	})
}

// Ported from Django: tests/test_utils/tests.py::InHTMLTests.test_long_haystack
func TestLongHaystack(t *testing.T) {
	haystack := "<p>This is a very very very very very very very very long message which " +
		"exceeds the max limit of truncation.</p>"
	msg := "Couldn't find '<b>Hello</b>' in the following response\n'" + haystack + "'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertInHTML(ft, "<b>Hello</b>", haystack)
	})

	msg = "Found 0 instances of '<b>This</b>' (expected 3) in the following response\n'" + haystack + "'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertInHTML(ft, "<b>This</b>", haystack, djangotest.Count(3))
	})
}

// Ported from Django: tests/test_utils/tests.py::InHTMLTests.test_assert_not_in_html
func TestAssertNotInHTML(t *testing.T) {
	haystack := "<p><b>Hello</b> <span>there</span>! Hi <span>there</span>!</p>"
	djangotest.AssertNotInHTML(t, "<b>Hi</b>", haystack)
	msg := "'<b>Hello</b>' unexpectedly found in the following response\n'" + haystack + "'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertNotInHTML(ft, "<b>Hello</b>", haystack)
	})
}

// Ported from Django: tests/test_utils/tests.py::InHTMLTests.test_assert_not_in_html_msg_prefix
func TestAssertNotInHTMLMsgPrefix(t *testing.T) {
	haystack := "<p>Hello</p>"
	msg := "Prefix: '<p>Hello</p>' unexpectedly found in the following response\n'" + haystack + "'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertNotInHTML(ft, "<p>Hello</p>", haystack, djangotest.MsgPrefix("Prefix"))
	})
}

// Ported from Django: tests/test_utils/tests.py::JSONEqualTests.test_simple_equal
func TestSimpleEqual(t *testing.T) {
	json1 := `{"attr1": "foo", "attr2":"baz"}`
	json2 := `{"attr1": "foo", "attr2":"baz"}`
	djangotest.AssertJSONEqual(t, json1, json2)
}

// Ported from Django: tests/test_utils/tests.py::JSONEqualTests.test_simple_equal_unordered
func TestSimpleEqualUnordered(t *testing.T) {
	json1 := `{"attr1": "foo", "attr2":"baz"}`
	json2 := `{"attr2":"baz", "attr1": "foo"}`
	djangotest.AssertJSONEqual(t, json1, json2)
}

// Ported from Django: tests/test_utils/tests.py::JSONEqualTests.test_simple_equal_raise
func TestSimpleEqualRaise(t *testing.T) {
	json1 := `{"attr1": "foo", "attr2":"baz"}`
	json2 := `{"attr2":"baz"}`
	mustFail(t, "", func(ft djangotest.TB) { djangotest.AssertJSONEqual(ft, json1, json2) })
}

// Ported from Django: tests/test_utils/tests.py::JSONEqualTests.test_equal_parsing_errors
func TestEqualParsingErrors(t *testing.T) {
	invalidJSON := `{"attr1": "foo, "attr2":"baz"}`
	validJSON := `{"attr1": "foo", "attr2":"baz"}`
	mustFail(t, "First argument is not valid JSON", func(ft djangotest.TB) {
		djangotest.AssertJSONEqual(ft, invalidJSON, validJSON)
	})
	mustFail(t, "Second argument is not valid JSON", func(ft djangotest.TB) {
		djangotest.AssertJSONEqual(ft, validJSON, invalidJSON)
	})
}

// Ported from Django: tests/test_utils/tests.py::JSONEqualTests.test_simple_not_equal
func TestSimpleNotEqual(t *testing.T) {
	json1 := `{"attr1": "foo", "attr2":"baz"}`
	json2 := `{"attr2":"baz"}`
	djangotest.AssertJSONNotEqual(t, json1, json2)
}

// Ported from Django: tests/test_utils/tests.py::JSONEqualTests.test_simple_not_equal_raise
func TestSimpleNotEqualRaise(t *testing.T) {
	json1 := `{"attr1": "foo", "attr2":"baz"}`
	json2 := `{"attr1": "foo", "attr2":"baz"}`
	mustFail(t, "", func(ft djangotest.TB) { djangotest.AssertJSONNotEqual(ft, json1, json2) })
}

// Ported from Django: tests/test_utils/tests.py::JSONEqualTests.test_not_equal_parsing_errors
func TestNotEqualParsingErrors(t *testing.T) {
	invalidJSON := `{"attr1": "foo, "attr2":"baz"}`
	validJSON := `{"attr1": "foo", "attr2":"baz"}`
	mustFail(t, "First argument is not valid JSON", func(ft djangotest.TB) {
		djangotest.AssertJSONNotEqual(ft, invalidJSON, validJSON)
	})
	mustFail(t, "Second argument is not valid JSON", func(ft djangotest.TB) {
		djangotest.AssertJSONNotEqual(ft, validJSON, invalidJSON)
	})
}

// TestURLEqual leaves out the reverse_lazy case of the Django test, which
// compares a path with a lazily reversed URL name.
//
// Ported from Django: tests/test_utils/tests.py::AssertURLEqualTests.test_equal
func TestURLEqual(t *testing.T) {
	validTests := [][2]string{
		{"http://example.com/?", "http://example.com/"},
		{"http://example.com/?x=1&", "http://example.com/?x=1"},
		{"http://example.com/?x=1&y=2", "http://example.com/?y=2&x=1"},
		{"http://example.com/?x=1&y=2&a=1&a=2", "http://example.com/?a=1&a=2&y=2&x=1"},
		{"/path/to/?x=1&y=2&z=3", "/path/to/?z=3&y=2&x=1"},
		{"?x=1&y=2&z=3", "?z=3&y=2&x=1"},
	}
	for _, urls := range validTests {
		t.Run(urls[0], func(t *testing.T) {
			djangotest.AssertURLEqual(t, urls[0], urls[1])
		})
	}
}

// Ported from Django: tests/test_utils/tests.py::AssertURLEqualTests.test_not_equal
func TestURLNotEqual(t *testing.T) {
	invalidTests := [][2]string{
		// Protocol must be the same.
		{"http://example.com/", "https://example.com/"},
		{"http://example.com/?x=1&x=2", "https://example.com/?x=2&x=1"},
		{"http://example.com/?x=1&y=bar&x=2", "https://example.com/?y=bar&x=2&x=1"},
		// Parameters of the same name must be in the same order.
		{"/path/to?a=1&a=2", "/path/to/?a=2&a=1"},
	}
	for _, urls := range invalidTests {
		t.Run(urls[0], func(t *testing.T) {
			mustFail(t, "", func(ft djangotest.TB) { djangotest.AssertURLEqual(ft, urls[0], urls[1]) })
		})
	}
}

// TestURLSameNameParameterOrder is not a Django test. The last case of
// test_not_equal is meant to show that parameters of the same name must keep
// their order, but its two URLs also differ in the path, and Django's
// assertURLEqual in fact accepts the two URLs below (it sorts the name/value
// pairs). AssertURLEqual follows the rule Django documents, which this checks
// on its own.
func TestURLSameNameParameterOrder(t *testing.T) {
	mustFail(t, "", func(ft djangotest.TB) {
		djangotest.AssertURLEqual(ft, "/path/to/?a=1&a=2", "/path/to/?a=2&a=1")
	})
	djangotest.AssertURLEqual(t, "/path/to/?a=1&b=3&a=2", "/path/to/?b=3&a=1&a=2")
}

// Ported from Django: tests/test_utils/tests.py::AssertURLEqualTests.test_message
func TestURLMessage(t *testing.T) {
	msg := "Expected 'http://example.com/?x=1&x=2' to equal 'https://example.com/?x=2&x=1'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertURLEqual(ft, "http://example.com/?x=1&x=2", "https://example.com/?x=2&x=1")
	})
}

// Ported from Django: tests/test_utils/tests.py::AssertURLEqualTests.test_msg_prefix
func TestURLMsgPrefix(t *testing.T) {
	msg := "Prefix: Expected 'http://example.com/?x=1&x=2' to equal 'https://example.com/?x=2&x=1'"
	mustFail(t, msg, func(ft djangotest.TB) {
		djangotest.AssertURLEqual(ft, "http://example.com/?x=1&x=2", "https://example.com/?x=2&x=1",
			djangotest.MsgPrefix("Prefix"))
	})
}
