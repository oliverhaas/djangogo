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
//	manage.py remove_stale_contenttypes            the remove_stale_contenttypes command of djangogo
//
// A model is identified by its app label and its lowercased type name, as in
// Django ("auth", "permission"). A model registered without an app has an empty
// app label.
//
// # Generic relations
//
//	GenericForeignKey("content_type", "object_id")   contenttypes.GenericForeignKey `orm:"ct_field=content_type;fk_field=object_id"`
//	GenericRelation(TaggedItem)                      contenttypes.GenericRelation[TaggedItem]
//	tag.content_object                               contenttypes.GetObject(ctx, db, &tag, "content_object")
//	tag.content_object = obj                         contenttypes.SetObject(ctx, db, &tag, "content_object", obj)
//	animal.tags.add(tag)                             orm.GenericSet[TaggedItem](db, &animal, "tags").Add(ctx, &tag)
//	TaggedItem.objects.filter(animal__name="Lion")   orm.Query[TaggedItem](db).Filter("animal__name", "Lion")
//	tag.animal.all() (related_query_name)            orm.Related[Animal](db, &tag, "animal")
//	prefetch_related("content_object")               PrefetchRelated("content_object")
//	GenericPrefetch("content_object", [qs1, qs2])    orm.Prefetch{Lookup: "content_object", QuerySets: []any{qs1, qs2}}
//	generic_inlineformset_factory(TaggedItem)        forms.NewGenericInlineFormSet[TaggedItem](db, "", "")
//	GenericTabularInline with model = TaggedItem     admin.GenericTabularInline[TaggedItem](admin.InlineModelAdmin{})
//
// Deleting an instance deletes the rows of its generic relations. GetOrCreate
// and UpdateOrCreate take a generic foreign key in their defaults.
//
// # Differences from Django
//
// A ContentType value does not know the model registry, so Name and
// AppLabeledName are built from the stored names and not from verbose names.
package contenttypes

import (
	"context"

	"github.com/oliverhaas/djangogo/orm"
)

// ContentType identifies one installed model by app label and model name. The
// table and the column names are Django's.
type ContentType struct {
	ID       int64
	AppLabel string `orm:"max_length=100"`
	Model    string `orm:"max_length=100;verbose_name=python model class name"`
}

// Meta names the table and makes the pair (app_label, model) unique, as
// Django does.
func (ContentType) Meta() orm.Meta {
	return orm.Meta{Table: "django_content_type", UniqueTogether: [][]string{{"AppLabel", "Model"}}}
}

// Name returns the model name (Django's ContentType.name for a model that is
// not installed).
func (ct ContentType) Name() string { return ct.Model }

// AppLabeledName returns "<app_label> | <model>", Django's app_labeled_name.
func (ct ContentType) AppLabeledName() string { return ct.AppLabel + " | " + ct.Model }

// String returns AppLabeledName, as Django's __str__ does.
func (ct ContentType) String() string { return ct.AppLabeledName() }

// NaturalKey returns the pair [appLabel, model] that identifies the content
// type independently of its primary key (Django's natural_key).
func (ct ContentType) NaturalKey() []any { return []any{ct.AppLabel, ct.Model} }

// GetByNaturalKey looks up a content type by its (app_label, model) natural
// key (Django's ContentTypeManager.get_by_natural_key).
func (*ContentType) GetByNaturalKey(ctx context.Context, db *orm.DB, key ...any) (any, error) {
	if len(key) != 2 {
		return nil, orm.ErrDoesNotExist
	}
	appLabel, ok1 := key[0].(string)
	model, ok2 := key[1].(string)
	if !ok1 || !ok2 {
		return nil, orm.ErrDoesNotExist
	}
	ct, err := GetByNaturalKey(ctx, db, appLabel, model)
	if err != nil {
		return nil, err
	}
	return &ct, nil
}

// App is the app config of the contenttypes app. Register it before every app
// whose models refer to ContentType, such as auth.App.
type App struct{}

// Name returns the app label "contenttypes".
func (App) Name() string { return "contenttypes" }

// Models returns the ContentType model.
func (App) Models() []any { return []any{&ContentType{}} }
