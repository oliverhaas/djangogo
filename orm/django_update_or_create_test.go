package orm_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// The ports of the update_or_create() tests of Django's
// tests/get_or_create/tests.py. The models are in
// django_get_or_create_test.go. UpdateOrCreate has no create_defaults, so the
// tests on it are not ported.

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_update
func TestUpdateOrCreateUpdate(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		gocLennon(t, db)
		p, created, err := orm.Query[gocPerson](db).UpdateOrCreate(context.Background(),
			orm.Defaults{"birthday": day(1940, time.October, 10)}, "first_name", "John", "last_name", "Lennon")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, p.FirstName, "John")
		djangotest.AssertEqual(t, p.LastName, "Lennon")
		gocAssertTime(t, p.Birthday, day(1940, time.October, 10))
	})
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_create
func TestUpdateOrCreateCreate(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		p, created, err := orm.Query[gocPerson](db).UpdateOrCreate(context.Background(),
			orm.Defaults{"birthday": day(1940, time.October, 10)}, "first_name", "John", "last_name", "Lennon")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, p.FirstName, "John")
		djangotest.AssertEqual(t, p.LastName, "Lennon")
		gocAssertTime(t, p.Birthday, day(1940, time.October, 10))
	})
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_integrity
//
// Django raises IntegrityError, because the birthday is missing. A Go struct
// field always holds a value, so the row is created with the zero time.
func TestUpdateOrCreateIntegrity(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		tom, created, err := orm.Query[gocPerson](db).UpdateOrCreate(context.Background(), nil,
			"first_name", "Tom", "last_name", "Smith")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, tom.Birthday.IsZero(), true)
	})
}

// gocAssertDuplicatePrimaryKey checks that UpdateOrCreate with an existing
// primary key and other field values is an error that leaves the row alone.
func gocAssertDuplicatePrimaryKey(t *testing.T, db *orm.DB) {
	t.Helper()
	create(t, db, &gocManualPrimaryKeyTest{ID: 1, Data: "Original"})
	rows := orm.Query[gocManualPrimaryKeyTest](db)
	_, _, err := rows.UpdateOrCreate(context.Background(), nil, "id", 1, "data", "Different")
	if !errors.Is(err, orm.ErrIntegrity) {
		t.Errorf("error = %v, want an integrity error", err)
	}
	row, err := rows.Get(context.Background(), "id", 1)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, row.Data, "Original")
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_manual_primary_key_test
func TestUpdateOrCreateManualPrimaryKey(t *testing.T) {
	djangotest.EachDB(t, gocModels(), gocAssertDuplicatePrimaryKey)
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTestsWithManualPKs.test_create_with_duplicate_primary_key
func TestUpdateOrCreateWithDuplicatePrimaryKey(t *testing.T) {
	djangotest.EachDB(t, gocModels(), gocAssertDuplicatePrimaryKey)
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_with_pk_property
func TestUpdateOrCreateWithPKProperty(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		thing, created, err := orm.Query[gocThing](db).UpdateOrCreate(context.Background(), nil, "pk", 1)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, thing.ID, int64(1))
	})
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_create_with_related_manager
func TestUpdateOrCreateCreateWithRelatedManager(t *testing.T) {
	t.Skip("needs reverse related managers (publisher.books), which another work package adds")
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_update_with_related_manager
func TestUpdateOrCreateUpdateWithRelatedManager(t *testing.T) {
	t.Skip("needs reverse related managers (publisher.books), which another work package adds")
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_create_with_many
func TestUpdateOrCreateCreateWithMany(t *testing.T) {
	t.Skip("needs many-to-many relations (author.books), which another work package adds")
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_update_with_many
func TestUpdateOrCreateUpdateWithMany(t *testing.T) {
	t.Skip("needs many-to-many relations (author.books), which another work package adds")
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_defaults_exact
func TestUpdateOrCreateDefaultsExact(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		harrison := func(defaults string) (gocPerson, bool) {
			t.Helper()
			obj, created, err := orm.Query[gocPerson](db).UpdateOrCreate(context.Background(),
				orm.Defaults{"birthday": day(1943, time.February, 25), "defaults": defaults},
				"first_name", "George", "last_name", "Harrison", "defaults__exact", "testing")
			djangotest.AssertEqual(t, err, nil)
			return obj, created
		}
		obj, created := harrison("testing")
		djangotest.AssertEqual(t, created, true)
		djangotest.AssertEqual(t, obj.Defaults, "testing")
		obj, created = harrison("another testing")
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, obj.Defaults, "another testing")
	})
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_create_callable_default
func TestUpdateOrCreateCreateCallableDefault(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		obj, created, err := orm.Query[gocPerson](db).UpdateOrCreate(context.Background(),
			orm.Defaults{"birthday": func() any { return day(1943, time.February, 25) }},
			"first_name", "George", "last_name", "Harrison")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, true)
		gocAssertTime(t, obj.Birthday, day(1943, time.February, 25))
	})
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_update_callable_default
func TestUpdateOrCreateUpdateCallableDefault(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		people := orm.Query[gocPerson](db)
		_, _, err := people.UpdateOrCreate(context.Background(), nil,
			"first_name", "George", "last_name", "Harrison", "birthday", day(1942, time.February, 25))
		djangotest.AssertEqual(t, err, nil)
		obj, created, err := people.UpdateOrCreate(context.Background(),
			orm.Defaults{"last_name": func() any { return "NotHarrison" }}, "first_name", "George")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, created, false)
		djangotest.AssertEqual(t, obj.LastName, "NotHarrison")
	})
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTests.test_update_only_defaults_and_pre_save_fields_when_local_fields
func TestUpdateOrCreateUpdatesOnlyDefaultsAndPreSaveFields(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		publisher := create(t, db, &gocPublisher{Name: "Acme Publishing"})
		book := create(t, db, &gocBook{Publisher: fkTo[gocPublisher](publisher.ID), Name: "The Book of Ed & Fred"})
		quote := db.Dialect().Quote
		for _, defaults := range []orm.Defaults{{"publisher": publisher}, {"publisher_id": publisher}} {
			capture := djangotest.CaptureQueries(db)
			_, created, err := orm.Query[gocBook](db).UpdateOrCreate(context.Background(), defaults, "pk", book.ID)
			capture.Stop()
			djangotest.AssertEqual(t, err, nil)
			djangotest.AssertEqual(t, created, false)
			var updates []string
			for _, q := range capture.Queries() {
				if strings.HasPrefix(q.SQL, "UPDATE") {
					updates = append(updates, q.SQL)
				}
			}
			if len(updates) != 1 {
				t.Errorf("%d UPDATE statements, want 1: %v", len(updates), updates)
				continue
			}
			// The statement writes the default and the auto_now field, not the name.
			for _, column := range []string{"publisher_id_column", "updated"} {
				if !strings.Contains(updates[0], quote(column)) {
					t.Errorf("the UPDATE does not write %s: %s", column, updates[0])
				}
			}
			if strings.Contains(updates[0], quote("name")) {
				t.Errorf("the UPDATE writes the name: %s", updates[0])
			}
		}
	})
}

// Ported from Django: tests/get_or_create/tests.py::UpdateOrCreateTransactionTests.test_updates_in_transaction
func TestUpdateOrCreateUpdatesInTransaction(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		features, ok := db.Dialect().(orm.FeatureDialect)
		if !ok || !features.Features().SelectForUpdate {
			t.Skip("the database has no SELECT ... FOR UPDATE")
		}
		ctx := context.Background()
		gocLennon(t, db)
		people := orm.Query[gocPerson](db)

		// The default is evaluated while UpdateOrCreate holds the lock on the
		// row, and keeps it for a while.
		const hold = 500 * time.Millisecond
		locked := make(chan struct{})
		birthday := func() any {
			close(locked)
			time.Sleep(hold)
			return day(1940, time.October, 10)
		}
		done := make(chan error, 1)
		start := time.Now()
		go func() {
			_, _, err := people.UpdateOrCreate(ctx, orm.Defaults{"birthday": birthday}, "first_name", "John")
			done <- err
		}()
		select {
		case <-locked:
		case <-time.After(5 * time.Second):
			t.Fatal("the database took too long to lock the row")
		}

		// This update has to wait for the lock.
		_, err := people.Filter("first_name", "John").Update(ctx, "last_name", "NotLennon")
		waited := time.Since(start)
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, <-done, nil)

		// The update blocked, and UpdateOrCreate did not overwrite it: the last
		// name is not among its defaults.
		if waited < hold {
			t.Errorf("the update returned after %v, before the lock was released", waited)
		}
		updated, err := people.Get(ctx, "first_name", "John")
		djangotest.AssertEqual(t, err, nil)
		djangotest.AssertEqual(t, updated.LastName, "NotLennon")
		gocAssertTime(t, updated.Birthday, day(1940, time.October, 10))
	})
}

// Ported from Django: tests/get_or_create/tests.py::InvalidCreateArgumentsTests.test_update_or_create_with_invalid_defaults
func TestUpdateOrCreateWithInvalidDefaults(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		_, _, err := orm.Query[gocThing](db).UpdateOrCreate(context.Background(),
			orm.Defaults{"nonexistent": "b"}, "name", "a")
		assertFieldError(t, err, "nvalid field name(s) for model gocThing: 'nonexistent'.")
	})
}

// Ported from Django: tests/get_or_create/tests.py::InvalidCreateArgumentsTests.test_update_or_create_with_invalid_kwargs
func TestUpdateOrCreateWithInvalidKwargs(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		_, _, err := orm.Query[gocThing](db).UpdateOrCreate(context.Background(), nil, "name", "a", "nonexistent", "b")
		assertFieldError(t, err, "annot resolve keyword 'nonexistent' into field. Choices are: id, name")
	})
}

// Ported from Django: tests/get_or_create/tests.py::InvalidCreateArgumentsTests.test_multiple_invalid_fields
func TestUpdateOrCreateMultipleInvalidFields(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		// The lookup is resolved before the defaults are looked at.
		_, _, err := orm.Query[gocThing](db).UpdateOrCreate(context.Background(),
			orm.Defaults{"invalid": "c"}, "name", "a", "nonexistent", "b")
		assertFieldError(t, err, "annot resolve keyword 'nonexistent' into field. Choices are: id, name")
	})
}

// TestGetOrCreateConcurrentCreate has no Django counterpart: the callers that
// lose the race to create a row get the row of the winner.
func TestGetOrCreateConcurrentCreate(t *testing.T) {
	djangotest.EachDB(t, gocModels(), func(t *testing.T, db *orm.DB) {
		if db.Dialect().Name() == "sqlite" {
			t.Skip("an in-memory SQLite database serializes its writers with lock errors")
		}
		const callers = 8
		type result struct {
			id      int64
			created bool
			err     error
		}
		results := make(chan result, callers)
		start := make(chan struct{})
		for range callers {
			go func() {
				<-start
				tag, created, err := orm.Query[gocTag](db).GetOrCreate(context.Background(), nil, "text", "shared")
				results <- result{tag.ID, created, err}
			}()
		}
		close(start)
		ids := map[int64]bool{}
		creations := 0
		for range callers {
			r := <-results
			djangotest.AssertEqual(t, r.err, nil)
			ids[r.id] = true
			if r.created {
				creations++
			}
		}
		djangotest.AssertEqual(t, creations, 1)
		djangotest.AssertEqual(t, len(ids), 1)
		djangotest.AssertEqual(t, mustCount(t, orm.Query[gocTag](db)), 1)
	})
}
