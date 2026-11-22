package orm_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The models of Django's tests/get_or_create/models.py that the ported tests
// use, with these differences:
//
//   - Person.birthday is a DateField in Django; the ORM has datetimes only.
//   - ManualPrimaryKeyTest has an auto primary key, which takes a given value
//     as well.
//   - Profile has its own primary key; Django makes the foreign key the
//     primary key.
//   - Journalist, a child of Author through multi-table inheritance, is left
//     out.

type gocPerson struct {
	ID             int64
	FirstName      string `orm:"max_length=100;unique"`
	LastName       string `orm:"max_length=100"`
	Birthday       time.Time
	Defaults       string `orm:"type=text"`
	CreateDefaults string `orm:"type=text"`
}

type gocDefaultPerson struct {
	ID        int64
	FirstName string `orm:"max_length=100;default=Anonymous"`
}

type gocManualPrimaryKeyTest struct {
	ID   int64
	Data string `orm:"max_length=100"`
}

type gocProfile struct {
	ID     int64
	Person orm.FK[gocPerson]
}

type gocTag struct {
	ID   int64
	Text string `orm:"max_length=255;unique"`
}

type gocThing struct {
	ID   int64
	Name string `orm:"max_length=255"`
	Tags orm.M2M[gocTag]
}

type gocPublisher struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type gocAuthor struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

type gocBook struct {
	ID        int64
	Name      string               `orm:"max_length=100"`
	Authors   orm.M2M[gocAuthor]   `orm:"related_name=books"`
	Publisher orm.FK[gocPublisher] `orm:"column=publisher_id_column;related_name=books"`
	Updated   time.Time            `orm:"auto_now"`
}

// gocModels returns the models of the get_or_create app.
func gocModels() []any {
	return []any{djangotest.App("get_or_create",
		&gocPerson{}, &gocDefaultPerson{}, &gocManualPrimaryKeyTest{}, &gocProfile{},
		&gocTag{}, &gocThing{}, &gocPublisher{}, &gocAuthor{}, &gocBook{})}
}

// gocLennon creates the row of GetOrCreateTests.setUpTestData.
func gocLennon(t *testing.T, db *orm.DB) *gocPerson {
	t.Helper()
	return create(t, db, &gocPerson{FirstName: "John", LastName: "Lennon", Birthday: day(1940, time.October, 9)})
}

// gocAssertTime checks that got is the instant want.
func gocAssertTime(t *testing.T, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("time = %v, want %v", got, want)
	}
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_get_or_create_method_with_get
func TestGetOrCreateMethodWithGet(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		gocLennon(t, db)
		people := orm.Query[gocPerson](db)
		_, created, err := people.GetOrCreate(context.Background(),
			orm.Defaults{"birthday": day(1940, time.October, 9)}, "first_name", "John", "last_name", "Lennon")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, mustCount(t, people), 1)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_get_or_create_method_with_create
func TestGetOrCreateMethodWithCreate(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		gocLennon(t, db)
		people := orm.Query[gocPerson](db)
		_, created, err := people.GetOrCreate(context.Background(),
			orm.Defaults{"birthday": day(1943, time.February, 25)}, "first_name", "George", "last_name", "Harrison")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, mustCount(t, people), 2)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_get_or_create_redundant_instance
func TestGetOrCreateRedundantInstance(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		gocLennon(t, db)
		people := orm.Query[gocPerson](db)
		harrison := func() bool {
			t.Helper()
			_, created, err := people.GetOrCreate(context.Background(),
				orm.Defaults{"birthday": day(1943, time.February, 25)}, "first_name", "George", "last_name", "Harrison")
			djangotest.AssertEqual(t, err, nil)
			return created
		}
		harrison()
		// The same statement a second time creates nothing.
		djangotest.AssertEqual(t, harrison(), false)
		djangotest.AssertEqual(t, mustCount(t, people), 2)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_get_or_create_invalid_params
//
// Django raises IntegrityError, because the birthday is missing. A Go struct
// field always holds a value, so the row is created with the zero time.
func TestGetOrCreateInvalidParams(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		tom, created, err := orm.Query[gocPerson](db).GetOrCreate(context.Background(), nil,
			"first_name", "Tom", "last_name", "Smith")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, tom.Birthday.IsZero(), true)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_get_or_create_with_pk_property
func TestGetOrCreateWithPKProperty(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		thing, created, err := orm.Query[gocThing](db).GetOrCreate(context.Background(), nil, "pk", 1)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, thing.ID, int64(1))
	})
}

// No Go counterpart: tests/get_or_create/tests.py::GetOrCreateTests.test_get_or_create_with_model_property_defaults
// sets a Python property with a setter through defaults. A Go struct has no
// properties.

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_get_or_create_on_related_manager
func TestGetOrCreateOnRelatedManager(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		p := create(t, db, &gocPublisher{Name: "Acme Publishing"})
		books := orm.RelatedSet[gocBook](db, p, "books")

		// Create a book through the publisher.
		book, created, err := books.GetOrCreate(ctx, nil, "name", "The Book of Ed & Fred")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, mustCount(t, books.QuerySet()), 1)

		// The same call again creates nothing.
		_, created, err = books.GetOrCreate(ctx, nil, "name", "The Book of Ed & Fred")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, mustCount(t, books.QuerySet()), 1)

		// Add an author to the book.
		authors := orm.Many[gocAuthor](db, &book, "authors")
		ed, created, err := authors.GetOrCreate(ctx, nil, "name", "Ed")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, mustCount(t, authors.QuerySet()), 1)

		// The same call again creates nothing.
		_, created, err = authors.GetOrCreate(ctx, nil, "name", "Ed")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, mustCount(t, authors.QuerySet()), 1)

		// Add a second author to the book.
		fred, created, err := authors.GetOrCreate(ctx, nil, "name", "Fred")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, mustCount(t, authors.QuerySet()), 2)

		// An author without books counts among all authors, not among those
		// of the book.
		create(t, db, &gocAuthor{Name: "Ted"})
		djangotest.AssertEqual(t, mustCount(t, orm.Query[gocAuthor](db)), 3)
		djangotest.AssertEqual(t, mustCount(t, authors.QuerySet()), 2)

		// Create a book through an author.
		edBooks := orm.Many[gocBook](db, &ed, "books")
		_, created, err = edBooks.GetOrCreate(ctx, nil, "name", "Ed's Recipes", "publisher", p)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, mustCount(t, edBooks.QuerySet()), 2)
		djangotest.AssertEqual(t, mustCount(t, orm.Many[gocBook](db, &fred, "books").QuerySet()), 1)

		// The primary key of the publisher instead of the instance.
		_, created, err = edBooks.GetOrCreate(ctx, nil, "name", "The Great Book of Ed", "publisher_id", p.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		_, created, err = edBooks.GetOrCreate(ctx, nil, "name", "The Great Book of Ed", "publisher_id", p.ID)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, mustCount(t, books.QuerySet()), 3)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_defaults_exact
func TestGetOrCreateDefaultsExact(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		gocLennon(t, db)
		// A field named defaults is looked up as "defaults__exact" in Django,
		// where a keyword argument of that name is taken. Here the defaults are an
		// argument of their own, and the key works either way.
		harrison := func() (gocPerson, bool) {
			t.Helper()
			obj, created, err := orm.Query[gocPerson](db).GetOrCreate(context.Background(),
				orm.Defaults{"birthday": day(1943, time.February, 25), "defaults": "testing"},
				"first_name", "George", "last_name", "Harrison", "defaults__exact", "testing")
			djangotest.AssertEqual(t, err, nil)
			return obj, created
		}
		obj, created := harrison()
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, obj.Defaults, "testing")
		obj2, created := harrison()
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, obj2.ID, obj.ID)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_callable_defaults
func TestGetOrCreateCallableDefaults(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		gocLennon(t, db)
		obj, created, err := orm.Query[gocPerson](db).GetOrCreate(context.Background(),
			orm.Defaults{"last_name": "Harrison", "birthday": func() any { return day(1943, time.February, 25) }},
			"first_name", "George")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		gocAssertTime(t, obj.Birthday, day(1943, time.February, 25))
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTests.test_callable_defaults_not_called
func TestGetOrCreateCallableDefaultsNotCalled(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		gocLennon(t, db)
		_, created, err := orm.Query[gocPerson](db).GetOrCreate(context.Background(),
			orm.Defaults{"birthday": func() any {
				t.Error("the default was evaluated for a row that exists")
				return time.Time{}
			}},
			"first_name", "John", "last_name", "Lennon")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
	})
}

// No Go counterpart: tests/get_or_create/tests.py::GetOrCreateTests.test_defaults_not_evaluated_unless_needed
// passes a lazy object as defaults, which Python evaluates only when it reads
// it. Go builds the Defaults map before the call; its func() any values are
// called only when they are needed, which
// TestGetOrCreateCallableDefaultsNotCalled checks.

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTestsWithManualPKs.test_create_with_duplicate_primary_key
func TestGetOrCreateWithDuplicatePrimaryKey(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &gocManualPrimaryKeyTest{ID: 1, Data: "Original"})
		rows := orm.Query[gocManualPrimaryKeyTest](db)
		// An existing primary key with other field values is an error, and the
		// row keeps its data.
		_, _, err := rows.GetOrCreate(context.Background(), nil, "id", 1, "data", "Different")
		if !errors.Is(err, orm.ErrIntegrity) || !errors.Is(err, orm.ErrUniqueViolation) {
			t.Errorf("error = %v, want a unique violation", err)
		}
		row, err := rows.Get(context.Background(), "id", 1)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, row.Data, "Original")
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTestsWithManualPKs.test_savepoint_rollback
//
// A Django TestCase runs in a transaction; the Atomic block stands in for it.
func TestGetOrCreateSavepointRollback(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		err := db.Atomic(context.Background(), func(ctx context.Context) error {
			tags := orm.Query[gocTag](db)
			if err := tags.Create(ctx, &gocTag{Text: "foo"}); err != nil {
				return err
			}
			// No tag has this primary key, so one is created, which violates the
			// unique constraint on the text.
			_, _, err := tags.GetOrCreate(ctx, orm.Defaults{"text": "foo"}, "pk", 123456789)
			if !errors.Is(err, orm.ErrIntegrity) {
				t.Errorf("error = %v, want an integrity error", err)
			}
			// The transaction is still usable after the error.
			return tags.Create(ctx, &gocTag{Text: "bar"})
		})
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[gocTag](db)), 2)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTestsWithManualPKs.test_get_or_create_empty
func TestGetOrCreateEmpty(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		// Every field of the model has a default, so no argument is needed.
		person, created, err := orm.Query[gocDefaultPerson](db).GetOrCreate(context.Background(), nil)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, person.FirstName, "Anonymous")
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateTransactionTests.test_get_or_create_integrityerror
func TestGetOrCreateIntegrityError(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		// No person has the primary key 1.
		_, _, err := orm.Query[gocProfile](db).GetOrCreate(context.Background(), nil, "person", &gocPerson{ID: 1})
		if err == nil {
			t.Skip("the database does not enforce foreign keys")
		}
		if !errors.Is(err, orm.ErrIntegrity) || !errors.Is(err, orm.ErrForeignKeyViolation) {
			t.Errorf("error = %v, want a foreign key violation", err)
		}
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateThroughManyToMany.test_get_get_or_create
func TestGetOrCreateThroughManyToManyGet(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		tag := create(t, db, &gocTag{Text: "foo"})
		thing := create(t, db, &gocThing{Name: "a"})
		tags := orm.Many[gocTag](db, thing, "tags")
		djangotest.AssertEqual(t, tags.Add(ctx, tag), nil)
		obj, created, err := tags.GetOrCreate(ctx, nil, "text", "foo")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, obj.ID, tag.ID)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateThroughManyToMany.test_create_get_or_create
func TestGetOrCreateThroughManyToManyCreate(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		thing := create(t, db, &gocThing{Name: "a"})
		tags := orm.Many[gocTag](db, thing, "tags")
		obj, created, err := tags.GetOrCreate(context.Background(), nil, "text", "foo")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, obj.Text, "foo")
		in := slices.ContainsFunc(mustAll(t, tags.QuerySet()), func(tag gocTag) bool { return tag.ID == obj.ID })
		djangotest.AssertEqual(t, in, true)
	})
}

// Ported from Django: tests/get_or_create/tests.py::GetOrCreateThroughManyToMany.test_something
func TestGetOrCreateThroughManyToManySomething(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		create(t, db, &gocTag{Text: "foo"})
		thing := create(t, db, &gocThing{Name: "a"})
		_, _, err := orm.Many[gocTag](db, thing, "tags").GetOrCreate(context.Background(), nil, "text", "foo")
		if !errors.Is(err, orm.ErrIntegrity) {
			t.Fatalf("err = %v, want an integrity error", err)
		}
	})
}

// Ported from Django: tests/get_or_create/tests.py::InvalidCreateArgumentsTests.test_get_or_create_with_invalid_defaults
func TestGetOrCreateWithInvalidDefaults(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		_, _, err := orm.Query[gocThing](db).GetOrCreate(context.Background(),
			orm.Defaults{"nonexistent": "b"}, "name", "a")
		assertFieldError(t, err, "nvalid field name(s) for model gocThing: 'nonexistent'.")
	})
}

// Ported from Django: tests/get_or_create/tests.py::InvalidCreateArgumentsTests.test_get_or_create_with_invalid_kwargs
func TestGetOrCreateWithInvalidKwargs(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		_, _, err := orm.Query[gocThing](db).GetOrCreate(context.Background(), nil, "name", "a", "nonexistent", "b")
		assertFieldError(t, err, "annot resolve keyword 'nonexistent' into field. Choices are: id, name, tags")
	})
}
