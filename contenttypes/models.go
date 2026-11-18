// Package contenttypes is the counterpart of django.contrib.contenttypes: it
// records every installed model as a row of the django_content_type table, so
// other apps can refer to "a model" from the database. The auth app builds its
// permissions on it (a permission belongs to a content type).
//
// # Mapping from Django
//
//	ContentType.objects.get_for_model(Model)       contenttypes.GetForModel(ctx, db, model)
//	ContentType.objects.get_for_model(instance)    contenttypes.GetForInstance(ctx, db, instance)
//	ContentType.objects.get_for_models(A, B)       contenttypes.GetForModels(ctx, db, a, b)
//	ContentType.objects.get_for_id(id)             contenttypes.GetForID(ctx, db, id)
//	ContentType.objects.get_by_natural_key(a, m)   contenttypes.GetByNaturalKey(ctx, db, a, m)
//	ContentType.objects.clear_cache()              contenttypes.ClearCache(db)
//	ct.model_class()                               contenttypes.ModelClass(db, ct)
//	ct.natural_key()                               ct.NaturalKey()
//	create_contenttypes (post_migrate)             contenttypes.Sync(ctx, db)
//
// A model is identified by its app label and its lowercased type name, as in
// Django ("auth", "permission"). A model registered without an app has an empty
// app label.
//
// # Differences from Django
//
// The pair (app_label, model) is unique in Django through a database
// constraint. Model-level unique-together is not available yet, so uniqueness
// is enforced in code: a lookup takes the row with the lowest id, and a caller
// that creates a row reads the pair back and removes its own row when another
// one was first. Two transactions that create the same content type at the same
// moment cannot see each other's row and can still leave a duplicate behind;
// run Sync after migrate, so that no row is created on demand.
//
// A ContentType value does not know the model registry, so Name and
// AppLabeledName are built from the stored names and not from verbose names.
package contenttypes

import (
	"github.com/oliverhaas/djangogo/orm"
)

// ContentType identifies one installed model by app label and model name. The
// table and the column names are Django's.
type ContentType struct {
	ID       int64
	AppLabel string `orm:"max_length=100"`
	Model    string `orm:"max_length=100;verbose_name=python model class name"`
}

// Meta names the table as Django does.
func (ContentType) Meta() orm.Meta { return orm.Meta{Table: "django_content_type"} }

// Name returns the model name (Django's ContentType.name for a model that is
// not installed).
func (ct ContentType) Name() string { return ct.Model }

// AppLabeledName returns "<app_label> | <model>", Django's app_labeled_name.
func (ct ContentType) AppLabeledName() string { return ct.AppLabel + " | " + ct.Model }

// String returns AppLabeledName, as Django's __str__ does.
func (ct ContentType) String() string { return ct.AppLabeledName() }

// NaturalKey returns the pair that identifies the content type independently
// of its primary key (Django's natural_key).
func (ct ContentType) NaturalKey() (appLabel, model string) { return ct.AppLabel, ct.Model }

// App is the app config of the contenttypes app. Register it before every app
// whose models refer to ContentType, such as auth.App.
type App struct{}

// Name returns the app label "contenttypes".
func (App) Name() string { return "contenttypes" }

// Models returns the ContentType model.
func (App) Models() []any { return []any{&ContentType{}} }
