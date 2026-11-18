package contenttypes_test

import (
	"context"
	"testing"

	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Site, Author, ConcreteModel and FooWithURL are the models of
// tests/contenttypes_tests/models.py the ported tests use.
type Site struct {
	ID     int64
	Domain string `orm:"max_length=100"`
}

// Author is tests/contenttypes_tests/models.py::Author.
type Author struct {
	ID   int64
	Name string `orm:"max_length=100"`
}

// ConcreteModel is tests/contenttypes_tests/models.py::ConcreteModel.
type ConcreteModel struct {
	ID   int64
	Name string `orm:"max_length=10"`
}

// FooWithURL is tests/contenttypes_tests/models.py::FooWithUrl.
type FooWithURL struct {
	ID   int64
	Name string `orm:"max_length=30;unique"`
}

// testModels is the registry of the ported tests: the contenttypes app and the
// models of the Django test app "contenttypes_tests".
func testModels() []any {
	return []any{
		contenttypes.App{},
		djangotest.App("contenttypes_tests", &Site{}, &Author{}, &ConcreteModel{}, &FooWithURL{}),
	}
}

// model returns the registered descriptor of the given model instance.
func model(t *testing.T, db *orm.DB, instance any) *orm.Model {
	t.Helper()
	m, ok := db.Registry().ModelOf(instance)
	if !ok {
		t.Fatalf("model of %T is not registered", instance)
	}
	return m
}

// mustGet returns the content type of instance's model, failing the test on an
// error.
func mustGet(t *testing.T, db *orm.DB, instance any) contenttypes.ContentType {
	t.Helper()
	ct, err := contenttypes.GetForModel(context.Background(), db, model(t, db, instance))
	if err != nil {
		t.Fatalf("GetForModel(%T): %v", instance, err)
	}
	return ct
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_lookup_cache
//
// Lookups by model, id, or natural key hit the database only on the first
// lookup. The first lookup on an empty table is two statements here (a SELECT
// and the INSERT that creates the row); Django's test database is populated by
// post_migrate, so its first lookup is the SELECT alone. The rows are created
// up front to keep Django's counts.
func TestLookupCache(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		if _, err := contenttypes.Sync(ctx, db); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		contenttypes.ClearCache(db)
		ctModel := model(t, db, &contenttypes.ContentType{})

		// At this point, a lookup for a ContentType should hit the DB.
		djangotest.AssertNumQueries(t, db, 1, func() {
			_, _ = contenttypes.GetForModel(ctx, db, ctModel)
		})

		// A second hit, though, won't hit the DB, nor will a lookup by ID or
		// natural key.
		var ct contenttypes.ContentType
		djangotest.AssertNumQueries(t, db, 0, func() {
			ct, _ = contenttypes.GetForModel(ctx, db, ctModel)
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, _ = contenttypes.GetForID(ctx, db, ct.ID)
		})
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, _ = contenttypes.GetByNaturalKey(ctx, db, "contenttypes", "contenttype")
		})

		// Once the cache is cleared, another lookup will again hit the DB.
		contenttypes.ClearCache(db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			_, _ = contenttypes.GetForModel(ctx, db, ctModel)
		})

		// The same should happen with a lookup by natural key.
		contenttypes.ClearCache(db)
		djangotest.AssertNumQueries(t, db, 1, func() {
			_, _ = contenttypes.GetByNaturalKey(ctx, db, "contenttypes", "contenttype")
		})
		// And a second hit shouldn't hit the DB.
		djangotest.AssertNumQueries(t, db, 0, func() {
			_, _ = contenttypes.GetByNaturalKey(ctx, db, "contenttypes", "contenttype")
		})
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_get_for_models_creation
//
// One SELECT finds nothing, then every model is created. Django needs one
// INSERT per model, four statements in all; here an INSERT is followed by a
// SELECT that stands in for the unique constraint (see the package
// documentation), which makes seven. Django passes a proxy model as a fourth
// argument, which shares the row of its concrete model; there are no proxy
// models here.
func TestGetForModelsCreation(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		models := []*orm.Model{
			model(t, db, &contenttypes.ContentType{}),
			model(t, db, &FooWithURL{}),
			model(t, db, &ConcreteModel{}),
		}
		var cts map[*orm.Model]contenttypes.ContentType
		djangotest.AssertNumQueries(t, db, 7, func() {
			var err error
			cts, err = contenttypes.GetForModels(ctx, db, models...)
			if err != nil {
				t.Fatalf("GetForModels: %v", err)
			}
		})
		assertContentTypes(t, db, cts, &contenttypes.ContentType{}, &FooWithURL{}, &ConcreteModel{})
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_get_for_models_empty_cache
func TestGetForModelsEmptyCache(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		if _, err := contenttypes.Sync(ctx, db); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		contenttypes.ClearCache(db)

		var cts map[*orm.Model]contenttypes.ContentType
		djangotest.AssertNumQueries(t, db, 1, func() {
			cts, _ = contenttypes.GetForModels(ctx, db,
				model(t, db, &contenttypes.ContentType{}),
				model(t, db, &FooWithURL{}),
				model(t, db, &ConcreteModel{}),
			)
		})
		assertContentTypes(t, db, cts, &contenttypes.ContentType{}, &FooWithURL{}, &ConcreteModel{})
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_get_for_models_partial_cache
func TestGetForModelsPartialCache(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		if _, err := contenttypes.Sync(ctx, db); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		contenttypes.ClearCache(db)

		mustGet(t, db, &contenttypes.ContentType{})
		var cts map[*orm.Model]contenttypes.ContentType
		djangotest.AssertNumQueries(t, db, 1, func() {
			cts, _ = contenttypes.GetForModels(ctx, db,
				model(t, db, &contenttypes.ContentType{}),
				model(t, db, &FooWithURL{}),
			)
		})
		assertContentTypes(t, db, cts, &contenttypes.ContentType{}, &FooWithURL{})
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_get_for_models_full_cache
func TestGetForModelsFullCache(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		mustGet(t, db, &contenttypes.ContentType{})
		mustGet(t, db, &FooWithURL{})
		var cts map[*orm.Model]contenttypes.ContentType
		djangotest.AssertNumQueries(t, db, 0, func() {
			cts, _ = contenttypes.GetForModels(ctx, db,
				model(t, db, &contenttypes.ContentType{}),
				model(t, db, &FooWithURL{}),
			)
		})
		assertContentTypes(t, db, cts, &contenttypes.ContentType{}, &FooWithURL{})
	})
}

// assertContentTypes checks that cts maps exactly the models of instances to
// their content types.
func assertContentTypes(t *testing.T, db *orm.DB, cts map[*orm.Model]contenttypes.ContentType, instances ...any) {
	t.Helper()
	djangotest.AssertEqual(t, len(cts), len(instances))
	for _, instance := range instances {
		m := model(t, db, instance)
		got, ok := cts[m]
		if !ok {
			t.Errorf("no content type returned for %s", m.Label())
			continue
		}
		djangotest.AssertEqual(t, got, mustGet(t, db, instance))
	}
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_get_for_model_create_contenttype
//
// GetForModel creates the content type when the database has none. Django
// declares the model inside the test; here it is the registered Author model on
// a database whose content type table is still empty.
func TestGetForModelCreateContentType(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ct := mustGet(t, db, &Author{})
		djangotest.AssertEqual(t, ct.AppLabel, "contenttypes_tests")
		djangotest.AssertEqual(t, ct.Model, "author")
		djangotest.AssertEqual(t, ct.String(), "contenttypes_tests | author")

		n, err := orm.Query[contenttypes.ContentType](db).Count(context.Background())
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(1))
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_cache_not_shared_between_managers
//
// Django keeps one cache per manager. Here the cache belongs to the database
// handle, so two handles do not share one.
func TestCacheNotSharedBetweenDatabases(t *testing.T) {
	ctx := context.Background()
	first := djangotest.NewDB(t, testModels()...)
	second := djangotest.NewDB(t, testModels()...)
	for _, db := range []*orm.DB{first, second} {
		if _, err := contenttypes.Sync(ctx, db); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		contenttypes.ClearCache(db)
	}
	djangotest.AssertNumQueries(t, first, 1, func() {
		_, _ = contenttypes.GetForModel(ctx, first, model(t, first, &contenttypes.ContentType{}))
	})
	djangotest.AssertNumQueries(t, first, 0, func() {
		_, _ = contenttypes.GetForModel(ctx, first, model(t, first, &contenttypes.ContentType{}))
	})
	djangotest.AssertNumQueries(t, second, 1, func() {
		_, _ = contenttypes.GetForModel(ctx, second, model(t, second, &contenttypes.ContentType{}))
	})
	djangotest.AssertNumQueries(t, second, 0, func() {
		_, _ = contenttypes.GetForModel(ctx, second, model(t, second, &contenttypes.ContentType{}))
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_missing_model
//
// A leftover content type whose model is gone does not break anything: it has a
// label, no model, and can be fetched like any other row.
func TestMissingModel(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		ct := contenttypes.ContentType{AppLabel: "contenttypes", Model: "OldModel"}
		if err := orm.Query[contenttypes.ContentType](db).Create(ctx, &ct); err != nil {
			t.Fatalf("Create: %v", err)
		}
		djangotest.AssertEqual(t, ct.String(), "contenttypes | OldModel")
		if _, ok := contenttypes.ModelClass(db, ct); ok {
			t.Error("ModelClass found a model for a stale content type")
		}

		// Stale content types can be fetched like any other object.
		fetched, err := contenttypes.GetForID(ctx, db, ct.ID)
		if err != nil {
			t.Fatalf("GetForID: %v", err)
		}
		if _, ok := contenttypes.ModelClass(db, fetched); ok {
			t.Error("ModelClass found a model for a fetched stale content type")
		}
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_missing_model_with_existing_model_name
//
// A stale content type that carries the name of a model of another app does not
// disturb the lookup of the existing models.
func TestMissingModelWithExistingModelName(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		stale := contenttypes.ContentType{AppLabel: "contenttypes", Model: "author"}
		if err := orm.Query[contenttypes.ContentType](db).Create(ctx, &stale); err != nil {
			t.Fatalf("Create: %v", err)
		}
		contenttypes.ClearCache(db)

		cts, err := contenttypes.GetForModels(ctx, db,
			model(t, db, &contenttypes.ContentType{}),
			model(t, db, &Author{}),
		)
		if err != nil {
			t.Fatalf("GetForModels: %v", err)
		}
		assertContentTypes(t, db, cts, &contenttypes.ContentType{}, &Author{})
		djangotest.AssertEqual(t, cts[model(t, db, &Author{})].AppLabel, "contenttypes_tests")
	})
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_str
// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_app_labeled_name
//
// Django shows the verbose name of the app ("Contenttypes_Tests | site"). An
// app has no verbose name here, so the label is built from the app label.
func TestStr(t *testing.T) {
	db := djangotest.NewDB(t, testModels()...)
	ct := mustGet(t, db, &Site{})
	djangotest.AssertEqual(t, ct.String(), "contenttypes_tests | site")
	djangotest.AssertEqual(t, ct.AppLabeledName(), "contenttypes_tests | site")
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_name
func TestName(t *testing.T) {
	db := djangotest.NewDB(t, testModels()...)
	ct := mustGet(t, db, &Site{})
	djangotest.AssertEqual(t, ct.Name(), "site")
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_name_unknown_model
func TestNameUnknownModel(t *testing.T) {
	ct := contenttypes.ContentType{AppLabel: "contenttypes_tests", Model: "unknown"}
	djangotest.AssertEqual(t, ct.Name(), "unknown")
}

// Ported from Django: tests/contenttypes_tests/test_models.py::ContentTypesTests.test_app_labeled_name_unknown_model
func TestAppLabeledNameUnknownModel(t *testing.T) {
	ct := contenttypes.ContentType{AppLabel: "contenttypes_tests", Model: "unknown"}
	djangotest.AssertEqual(t, ct.AppLabeledName(), "contenttypes_tests | unknown")
}

// TestSyncCreatesMissingContentTypes covers create_contenttypes, the handler
// Django runs after migrate: one row per registered model, none of them twice.
func TestSyncCreatesMissingContentTypes(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		created, err := contenttypes.Sync(ctx, db)
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
		labels := make([]string, 0, len(created))
		for _, ct := range created {
			labels = append(labels, ct.AppLabel+"."+ct.Model)
		}
		djangotest.AssertSequenceEqual(t, labels, []string{
			"contenttypes.contenttype",
			"contenttypes_tests.site",
			"contenttypes_tests.author",
			"contenttypes_tests.concretemodel",
			"contenttypes_tests.foowithurl",
		})

		// A second run finds every row and creates nothing.
		again, err := contenttypes.Sync(ctx, db)
		if err != nil {
			t.Fatalf("second Sync: %v", err)
		}
		djangotest.AssertEqual(t, len(again), 0)
		n, err := orm.Query[contenttypes.ContentType](db).Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		djangotest.AssertEqual(t, n, int64(5))
	})
}

// TestNaturalKey covers ContentType.natural_key and get_by_natural_key, which
// Django's serializers rely on.
func TestNaturalKey(t *testing.T) {
	djangotest.EachDB(t, testModels(), func(t *testing.T, db *orm.DB) {
		ctx := context.Background()
		ct := mustGet(t, db, &Author{})
		appLabel, name := ct.NaturalKey()
		djangotest.AssertEqual(t, appLabel, "contenttypes_tests")
		djangotest.AssertEqual(t, name, "author")

		contenttypes.ClearCache(db)
		got, err := contenttypes.GetByNaturalKey(ctx, db, appLabel, name)
		if err != nil {
			t.Fatalf("GetByNaturalKey: %v", err)
		}
		djangotest.AssertEqual(t, got, ct)

		_, err = contenttypes.GetByNaturalKey(ctx, db, "contenttypes_tests", "missing")
		djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
	})
}

// TestGetForInstance resolves the content type from a model instance, as
// get_for_model does when Django is handed an object.
func TestGetForInstance(t *testing.T) {
	db := djangotest.NewDB(t, testModels()...)
	ctx := context.Background()
	want := mustGet(t, db, &Author{})

	got, err := contenttypes.GetForInstance(ctx, db, &Author{Name: "a"})
	if err != nil {
		t.Fatalf("GetForInstance(pointer): %v", err)
	}
	djangotest.AssertEqual(t, got, want)

	got, err = contenttypes.GetForInstance(ctx, db, Author{Name: "b"})
	if err != nil {
		t.Fatalf("GetForInstance(value): %v", err)
	}
	djangotest.AssertEqual(t, got, want)

	type unregistered struct{ ID int64 }
	if _, err := contenttypes.GetForInstance(ctx, db, &unregistered{}); err == nil {
		t.Error("GetForInstance accepted an unregistered model")
	}
}

// TestModelClass maps a content type back to its registered model.
func TestModelClass(t *testing.T) {
	db := djangotest.NewDB(t, testModels()...)
	ct := mustGet(t, db, &Author{})
	m, ok := contenttypes.ModelClass(db, ct)
	if !ok {
		t.Fatal("ModelClass did not find the Author model")
	}
	djangotest.AssertEqual(t, m.Label(), "contenttypes_tests.Author")
}

// TestGetForIDUnknown reports a missing row with the ORM's sentinel error.
func TestGetForIDUnknown(t *testing.T) {
	db := djangotest.NewDB(t, testModels()...)
	_, err := contenttypes.GetForID(context.Background(), db, 12345)
	djangotest.AssertErrorIs(t, err, orm.ErrDoesNotExist)
}
