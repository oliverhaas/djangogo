package djangogo_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo"
	"github.com/oliverhaas/djangogo/apps"
	"github.com/oliverhaas/djangogo/conf"
	"github.com/oliverhaas/djangogo/contenttypes"
	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
)

// Site is tests/contenttypes_tests/models.py::Site.
type Site struct {
	ID     int64
	Domain string `orm:"max_length=100"`
}

// Post is tests/contenttypes_tests/models.py::Post without
// order_with_respect_to, which the orm does not have.
type Post struct {
	ID          int64
	Title       string                           `orm:"max_length=200"`
	ContentType orm.FK[contenttypes.ContentType] `orm:"null;on_delete=cascade"`
	ObjectID    *int64                           `orm:"type=positive_int"`
	Parent      contenttypes.GenericForeignKey
	Children    contenttypes.GenericRelation[Post]
}

// ModelWithNullFKToSite is
// tests/contenttypes_tests/models.py::ModelWithNullFKToSite.
type ModelWithNullFKToSite struct {
	ID    int64
	Title string       `orm:"max_length=200"`
	Site  orm.FK[Site] `orm:"null;on_delete=cascade"`
	Post  orm.FK[Post] `orm:"null;on_delete=cascade"`
}

// contenttypesTestsApp is the contenttypes_tests app of Django's tests.
type contenttypesTestsApp struct{}

// Name returns "contenttypes_tests".
func (contenttypesTestsApp) Name() string { return "contenttypes_tests" }

// Models returns the models the ported tests use.
func (contenttypesTestsApp) Models() []any {
	return []any{&Site{}, &Post{}, &ModelWithNullFKToSite{}}
}

// emptyModelsApp is the empty_models app of Django's tests, whose models
// module defines no model.
type emptyModelsApp struct{}

// Name returns "empty_models".
func (emptyModelsApp) Name() string { return "empty_models" }

// Models returns no model.
func (emptyModelsApp) Models() []any { return nil }

// noModelsApp is the no_models app of Django's tests, which has no models
// module.
type noModelsApp struct{}

// Name returns "no_models".
func (noModelsApp) Name() string { return "no_models" }

// staleFixture is RemoveStaleContentTypesTests: an application with the
// content types of its models, after a first run of the command, and one
// stale content type.
type staleFixture struct {
	app         *djangogo.Application
	out         *bytes.Buffer
	beforeCount int64
	contentType contenttypes.ContentType
}

// newStaleFixture runs RemoveStaleContentTypesTests.setUpTestData with the
// apps of the test installed. withEmptyModels installs empty_models, which
// two tests remove with modify_settings.
func newStaleFixture(t *testing.T, withEmptyModels bool) *staleFixture {
	t.Helper()
	ctx := context.Background()
	configs := []apps.Config{contenttypes.App{}, contenttypesTestsApp{}}
	if withEmptyModels {
		configs = append(configs, emptyModelsApp{})
	}
	configs = append(configs, noModelsApp{})
	app, err := djangogo.New(conf.Settings{
		SecretKey: "k",
		Database:  conf.Database{Driver: "sqlite", DSN: "file:" + t.Name() + "?mode=memory&cache=shared"},
	}, configs...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if err := app.DB.CreateTables(ctx, app.Models.Models()...); err != nil {
		t.Fatal(err)
	}
	if _, err := contenttypes.Sync(ctx, app.DB); err != nil {
		t.Fatal(err)
	}
	f := &staleFixture{app: app, out: &bytes.Buffer{}}
	app.Out, app.Err = f.out, f.out
	f.run(t, "", "--noinput", "--include-stale-apps", "-v", "2")
	f.out.Reset()
	f.beforeCount = f.countContentTypes(t)
	f.contentType = contenttypes.ContentType{AppLabel: "contenttypes_tests", Model: "Fake"}
	if err := orm.Query[contenttypes.ContentType](app.DB).Create(ctx, &f.contentType); err != nil {
		t.Fatal(err)
	}
	return f
}

// run runs remove_stale_contenttypes with args and gives answer to every
// question, as the mocked input() of Django's tests does.
func (f *staleFixture) run(t *testing.T, answer string, args ...string) {
	t.Helper()
	f.app.In = strings.NewReader(strings.Repeat(answer, 10))
	if err := f.app.Execute(append([]string{"remove_stale_contenttypes"}, args...)); err != nil {
		t.Fatal(err)
	}
}

// createContentType creates a content type row.
func (f *staleFixture) createContentType(t *testing.T, appLabel, model string) {
	t.Helper()
	ct := contenttypes.ContentType{AppLabel: appLabel, Model: model}
	if err := orm.Query[contenttypes.ContentType](f.app.DB).Create(context.Background(), &ct); err != nil {
		t.Fatal(err)
	}
}

// countContentTypes returns ContentType.objects.count().
func (f *staleFixture) countContentTypes(t *testing.T) int64 {
	t.Helper()
	n, err := orm.Query[contenttypes.ContentType](f.app.DB).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// assertOutput checks whether the output of the command holds each of want.
func (f *staleFixture) assertOutput(t *testing.T, contains bool, want ...string) {
	t.Helper()
	for _, w := range want {
		if strings.Contains(f.out.String(), w) != contains {
			t.Errorf("output contains %q: %v, want %v\n%s", w, !contains, contains, f.out.String())
		}
	}
}

// Ported from Django: tests/contenttypes_tests/test_management.py::RemoveStaleContentTypesTests.test_interactive_true_with_dependent_objects
//
// interactive mode (the default) deletes stale content types and warns of
// dependent objects.
func TestRemoveStaleContentTypesInteractiveTrueWithDependentObjects(t *testing.T) {
	f := newStaleFixture(t, true)
	ctx := context.Background()
	post := &Post{Title: "post"}
	post.ContentType.SetPK(f.contentType.ID)
	if err := orm.Query[Post](f.app.DB).Create(ctx, post); err != nil {
		t.Fatal(err)
	}
	// A related object is needed to show that a custom collector with
	// can_fast_delete=False is needed.
	child := &ModelWithNullFKToSite{}
	child.Post.SetPK(post.ID)
	if err := orm.Query[ModelWithNullFKToSite](f.app.DB).Create(ctx, child); err != nil {
		t.Fatal(err)
	}
	f.run(t, "yes\n", "-v", "2")
	n, err := orm.Query[Post](f.app.DB).Count(ctx)
	djangotest.AssertEqual(t, err, nil)
	djangotest.AssertEqual(t, n, int64(0))
	f.assertOutput(t, true,
		"- Content type for contenttypes_tests.Fake",
		"- 1 contenttypes_tests.Post object(s)",
		"- 1 contenttypes_tests.ModelWithNullFKToSite",
		"Deleting stale content type")
	djangotest.AssertEqual(t, f.countContentTypes(t), f.beforeCount)
}

// Ported from Django: tests/contenttypes_tests/test_management.py::RemoveStaleContentTypesTests.test_interactive_true_without_dependent_objects
//
// interactive mode deletes stale content types even if there aren't any
// dependent objects.
func TestRemoveStaleContentTypesInteractiveTrueWithoutDependentObjects(t *testing.T) {
	f := newStaleFixture(t, true)
	f.run(t, "yes\n", "-v", "2")
	f.assertOutput(t, true, "Deleting stale content type")
	djangotest.AssertEqual(t, f.countContentTypes(t), f.beforeCount)
}

// Ported from Django: tests/contenttypes_tests/test_management.py::RemoveStaleContentTypesTests.test_interactive_false
//
// non-interactive mode deletes stale content types.
func TestRemoveStaleContentTypesInteractiveFalse(t *testing.T) {
	f := newStaleFixture(t, true)
	f.run(t, "", "--noinput", "-v", "2")
	f.assertOutput(t, true, "Deleting stale content type")
	djangotest.AssertEqual(t, f.countContentTypes(t), f.beforeCount)
}

// No Go counterpart: tests/contenttypes_tests/test_management.py::RemoveStaleContentTypesTests.test_unavailable_content_type_model
// passes create_contenttypes the registry of a migration state without the
// ContentType model. contenttypes.Sync takes no registry argument: it always
// uses the models of the database.

// Ported from Django: tests/contenttypes_tests/test_management.py::RemoveStaleContentTypesTests.test_contenttypes_removed_in_installed_apps_without_models
func TestRemoveStaleContentTypesRemovedInInstalledAppsWithoutModels(t *testing.T) {
	f := newStaleFixture(t, false)
	f.createContentType(t, "empty_models", "Fake 1")
	f.createContentType(t, "no_models", "Fake 2")
	f.run(t, "yes\n", "-v", "2")
	f.assertOutput(t, false, "Deleting stale content type 'empty_models | Fake 1'")
	f.assertOutput(t, true, "Deleting stale content type 'no_models | Fake 2'")
	djangotest.AssertEqual(t, f.countContentTypes(t), f.beforeCount+1)
}

// Ported from Django: tests/contenttypes_tests/test_management.py::RemoveStaleContentTypesTests.test_contenttypes_removed_for_apps_not_in_installed_apps
func TestRemoveStaleContentTypesRemovedForAppsNotInInstalledApps(t *testing.T) {
	f := newStaleFixture(t, false)
	f.createContentType(t, "empty_models", "Fake 1")
	f.createContentType(t, "no_models", "Fake 2")
	f.run(t, "yes\n", "--include-stale-apps", "-v", "2")
	f.assertOutput(t, true,
		"Deleting stale content type 'empty_models | Fake 1'",
		"Deleting stale content type 'no_models | Fake 2'")
	djangotest.AssertEqual(t, f.countContentTypes(t), f.beforeCount)
}

// No Django test exists: any answer other than "yes" keeps the stale content
// types.
func TestRemoveStaleContentTypesAnswerNo(t *testing.T) {
	f := newStaleFixture(t, true)
	f.run(t, "no\n", "-v", "2")
	f.assertOutput(t, true, "Stale content types remain.")
	f.assertOutput(t, false, "Deleting stale content type")
	djangotest.AssertEqual(t, f.countContentTypes(t), f.beforeCount+1)
}
