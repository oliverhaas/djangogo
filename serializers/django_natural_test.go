package serializers_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/orm"
	"github.com/oliverhaas/djangogo/serializers"
)

// Ported from Django: tests/serializers/test_natural.py::natural_key_serializer_test
// Check that objects with natural foreign keys serialize, deserialize, and save back with identical relation data.
func TestNaturalKeySerializer(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				a1 := mustCreate(t, db, &NaturalKeyAnchor{ID: 1100, Data: "Natural Key Anghor"})
				f1 := mustCreate(t, db, &FKDataNaturalKey{ID: 1101, Data: fkTo[NaturalKeyAnchor](1100)})
				f2 := mustCreate(t, db, &FKDataNaturalKey{ID: 1102})
				objects := []any{a1, f1, f2}

				serializedData, err := serializers.Serialize(
					ctx, db, format, objects,
					serializers.Indent(2),
					serializers.NaturalForeignKeys(),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				for obj, err := range serializers.Deserialize(ctx, db, format, strings.NewReader(serializedData)) {
					if err != nil {
						t.Fatalf("Deserialize: %v", err)
					}
					if err := obj.Save(ctx, db); err != nil {
						t.Fatalf("Save: %v", err)
					}
				}

				gotAnchor, err := orm.Query[NaturalKeyAnchor](db).Get(ctx, "id", 1100)
				if err != nil {
					t.Fatalf("Get(1100): %v", err)
				}
				djangotest.AssertEqual(t, gotAnchor.Data, a1.Data)

				gotF1, err := orm.Query[FKDataNaturalKey](db).Get(ctx, "id", 1101)
				if err != nil {
					t.Fatalf("Get(1101): %v", err)
				}
				djangotest.AssertEqual(t, gotF1.Data.PK(), f1.Data.PK())

				gotF2, err := orm.Query[FKDataNaturalKey](db).Get(ctx, "id", 1102)
				if err != nil {
					t.Fatalf("Get(1102): %v", err)
				}
				djangotest.AssertEqual(t, gotF2.Data.PK(), f2.Data.PK())
			})
		})
	}
}

// Ported from Django: tests/serializers/test_natural.py::natural_key_test
// Check that deserializing objects with NaturalPrimaryKeys restores primary keys for existing rows and leaves deleted rows unset.
func TestNaturalKeySerializerNaturalKeys(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				title1 := "The Definitive Guide to Django: Web Development Done Right"
				title2 := "Practical Django Projects"
				adrian := mustCreate(t, db, &NaturalKeyAnchor{Data: "978-1590597255", Title: &title1})
				james := mustCreate(t, db, &NaturalKeyAnchor{Data: "978-1590599969", Title: &title2})

				allBooks, err := orm.Query[NaturalKeyAnchor](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				stringData, err := serializers.Serialize(
					ctx, db, format, allBooks,
					serializers.Indent(2),
					serializers.NaturalForeignKeys(),
					serializers.NaturalPrimaryKeys(),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}

				if _, err := orm.Query[NaturalKeyAnchor](db).Filter("id", james.ID).Delete(ctx); err != nil {
					t.Fatalf("Delete(james): %v", err)
				}

				books, err := collectDeserialized(ctx, db, format, stringData)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(books), 2)

				type titlePK struct {
					title string
					pk    int64
				}
				got := make([]titlePK, 0, len(books))
				for _, b := range books {
					anchor := b.Object.(*NaturalKeyAnchor)
					var title string
					if anchor.Title != nil {
						title = *anchor.Title
					}
					got = append(got, titlePK{title: title, pk: anchor.ID})
				}
				djangotest.AssertEqual(t, slices.Contains(got, titlePK{title: title1, pk: adrian.ID}), true)
				djangotest.AssertEqual(t, slices.Contains(got, titlePK{title: title2, pk: 0}), true)
			})
		})
	}
}

// Ported from Django: tests/serializers/test_natural.py::natural_pk_mti_test
// Check that multi-table inheritance child models serialize and deserialize their parent link using natural primary keys.
func TestNaturalKeySerializerNaturalPKMTI(t *testing.T) {
	t.Skip("FIXME-PORT: Go: multi-table model inheritance, Django: natural_pk_mti_test serializes and deserializes Child and Parent models with natural primary keys")
}

// Ported from Django: tests/serializers/test_natural.py::forward_ref_fk_test
// Check that HandleForwardReferences defers and resolves forward-referenced natural foreign keys via SaveDeferredFields.
func TestNaturalKeySerializerForwardRefFK(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				t1 := mustCreate(t, db, &NaturalKeyThing{Key: "t1"})
				t2 := mustCreate(t, db, &NaturalKeyThing{Key: "t2", OtherThing: fkTo[NaturalKeyThing](t1.ID)})
				t1.OtherThing = fkTo[NaturalKeyThing](t2.ID)
				if err := orm.Query[NaturalKeyThing](db).Save(ctx, t1); err != nil {
					t.Fatalf("Save(t1): %v", err)
				}

				stringData, err := serializers.Serialize(
					ctx, db, format, []*NaturalKeyThing{t1, t2},
					serializers.NaturalPrimaryKeys(),
					serializers.NaturalForeignKeys(),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				if _, err := orm.Query[NaturalKeyThing](db).Delete(ctx); err != nil {
					t.Fatalf("Delete: %v", err)
				}

				var objsWithDeferredFields []*serializers.DeserializedObject
				for obj, err := range serializers.Deserialize(ctx, db, format, strings.NewReader(stringData), serializers.HandleForwardReferences()) {
					if err != nil {
						t.Fatalf("Deserialize: %v", err)
					}
					if err := obj.Save(ctx, db); err != nil {
						t.Fatalf("Save: %v", err)
					}
					if len(obj.DeferredFields) > 0 {
						objsWithDeferredFields = append(objsWithDeferredFields, obj)
					}
				}
				for _, obj := range objsWithDeferredFields {
					if err := obj.SaveDeferredFields(ctx, db); err != nil {
						t.Fatalf("SaveDeferredFields: %v", err)
					}
				}

				gotT1, err := orm.Query[NaturalKeyThing](db).Get(ctx, "key", "t1")
				if err != nil {
					t.Fatalf("Get(t1): %v", err)
				}
				gotT2, err := orm.Query[NaturalKeyThing](db).Get(ctx, "key", "t2")
				if err != nil {
					t.Fatalf("Get(t2): %v", err)
				}
				djangotest.AssertEqual(t, gotT1.OtherThing.PK(), gotT2.ID)
				djangotest.AssertEqual(t, gotT2.OtherThing.PK(), gotT1.ID)
			})
		})
	}
}

// Ported from Django: tests/serializers/test_natural.py::forward_ref_fk_with_error_test
// Check that SaveDeferredFields returns a DeserializationError when a deferred natural foreign key still does not exist.
func TestNaturalKeySerializerForwardRefFKWithError(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				t1 := mustCreate(t, db, &NaturalKeyThing{Key: "t1"})
				t2 := mustCreate(t, db, &NaturalKeyThing{Key: "t2", OtherThing: fkTo[NaturalKeyThing](t1.ID)})
				t1.OtherThing = fkTo[NaturalKeyThing](t2.ID)
				if err := orm.Query[NaturalKeyThing](db).Save(ctx, t1); err != nil {
					t.Fatalf("Save(t1): %v", err)
				}

				stringData, err := serializers.Serialize(
					ctx, db, format, []*NaturalKeyThing{t1},
					serializers.NaturalPrimaryKeys(),
					serializers.NaturalForeignKeys(),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				if _, err := orm.Query[NaturalKeyThing](db).Delete(ctx); err != nil {
					t.Fatalf("Delete: %v", err)
				}

				var objsWithDeferredFields []*serializers.DeserializedObject
				for obj, err := range serializers.Deserialize(ctx, db, format, strings.NewReader(stringData), serializers.HandleForwardReferences()) {
					if err != nil {
						t.Fatalf("Deserialize: %v", err)
					}
					if err := obj.Save(ctx, db); err != nil {
						t.Fatalf("Save: %v", err)
					}
					if len(obj.DeferredFields) > 0 {
						objsWithDeferredFields = append(objsWithDeferredFields, obj)
					}
				}
				djangotest.AssertEqual(t, len(objsWithDeferredFields), 1)
				err = objsWithDeferredFields[0].SaveDeferredFields(ctx, db)
				djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
				want := "NaturalKeyThing matching query does not exist"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("error = %v, want substring %q", err, want)
				}
			})
		})
	}
}

// Ported from Django: tests/serializers/test_natural.py::forward_ref_m2m_test
// Check that HandleForwardReferences defers and resolves forward-referenced natural M2M keys via SaveDeferredFields.
func TestNaturalKeySerializerForwardRefM2M(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				t1 := mustCreate(t, db, &NaturalKeyThing{Key: "t1"})
				t2 := mustCreate(t, db, &NaturalKeyThing{Key: "t2"})
				t3 := mustCreate(t, db, &NaturalKeyThing{Key: "t3"})
				if err := orm.Many[NaturalKeyThing](db, t1, "other_things").Set(ctx, []*NaturalKeyThing{t2, t3}); err != nil {
					t.Fatalf("Set(other_things): %v", err)
				}

				stringData, err := serializers.Serialize(
					ctx, db, format, []*NaturalKeyThing{t1, t2, t3},
					serializers.NaturalPrimaryKeys(),
					serializers.NaturalForeignKeys(),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				if _, err := orm.Query[NaturalKeyThing](db).Delete(ctx); err != nil {
					t.Fatalf("Delete: %v", err)
				}

				var objsWithDeferredFields []*serializers.DeserializedObject
				for obj, err := range serializers.Deserialize(ctx, db, format, strings.NewReader(stringData), serializers.HandleForwardReferences()) {
					if err != nil {
						t.Fatalf("Deserialize: %v", err)
					}
					if err := obj.Save(ctx, db); err != nil {
						t.Fatalf("Save: %v", err)
					}
					if len(obj.DeferredFields) > 0 {
						objsWithDeferredFields = append(objsWithDeferredFields, obj)
					}
				}
				for _, obj := range objsWithDeferredFields {
					if err := obj.SaveDeferredFields(ctx, db); err != nil {
						t.Fatalf("SaveDeferredFields: %v", err)
					}
				}

				gotT1, err := orm.Query[NaturalKeyThing](db).Get(ctx, "key", "t1")
				if err != nil {
					t.Fatalf("Get(t1): %v", err)
				}
				gotT2, err := orm.Query[NaturalKeyThing](db).Get(ctx, "key", "t2")
				if err != nil {
					t.Fatalf("Get(t2): %v", err)
				}
				gotT3, err := orm.Query[NaturalKeyThing](db).Get(ctx, "key", "t3")
				if err != nil {
					t.Fatalf("Get(t3): %v", err)
				}
				related, err := orm.Many[NaturalKeyThing](db, &gotT1, "other_things").All(ctx)
				if err != nil {
					t.Fatalf("Many(other_things): %v", err)
				}
				djangotest.AssertEqual(t, len(related), 2)
				relatedIDs := []int64{related[0].ID, related[1].ID}
				djangotest.AssertEqual(t, slices.Contains(relatedIDs, gotT2.ID), true)
				djangotest.AssertEqual(t, slices.Contains(relatedIDs, gotT3.ID), true)
			})
		})
	}
}

// Ported from Django: tests/serializers/test_natural.py::forward_ref_m2m_with_error_test
// Check that SaveDeferredFields returns a DeserializationError when a deferred natural M2M target still does not exist.
func TestNaturalKeySerializerForwardRefM2MWithError(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				t1 := mustCreate(t, db, &NaturalKeyThing{Key: "t1"})
				t2 := mustCreate(t, db, &NaturalKeyThing{Key: "t2"})
				t3 := mustCreate(t, db, &NaturalKeyThing{Key: "t3"})
				if err := orm.Many[NaturalKeyThing](db, t1, "other_things").Set(ctx, []*NaturalKeyThing{t2, t3}); err != nil {
					t.Fatalf("Set(other_things): %v", err)
				}

				stringData, err := serializers.Serialize(
					ctx, db, format, []*NaturalKeyThing{t1, t2},
					serializers.NaturalPrimaryKeys(),
					serializers.NaturalForeignKeys(),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				if _, err := orm.Query[NaturalKeyThing](db).Delete(ctx); err != nil {
					t.Fatalf("Delete: %v", err)
				}

				var objsWithDeferredFields []*serializers.DeserializedObject
				for obj, err := range serializers.Deserialize(ctx, db, format, strings.NewReader(stringData), serializers.HandleForwardReferences()) {
					if err != nil {
						t.Fatalf("Deserialize: %v", err)
					}
					if err := obj.Save(ctx, db); err != nil {
						t.Fatalf("Save: %v", err)
					}
					if len(obj.DeferredFields) > 0 {
						objsWithDeferredFields = append(objsWithDeferredFields, obj)
					}
				}
				djangotest.AssertEqual(t, len(objsWithDeferredFields), 1)
				err = objsWithDeferredFields[0].SaveDeferredFields(ctx, db)
				djangotest.AssertErrorIs(t, err, serializers.ErrDeserialization)
				want := "NaturalKeyThing matching query does not exist"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("error = %v, want substring %q", err, want)
				}
			})
		})
	}
}

// Ported from Django: tests/serializers/test_natural.py::pk_with_default
// Check that deserializing with natural primary keys resolves an existing primary key when the pk field has a default.
func TestNaturalKeySerializerPKWithDefault(t *testing.T) {
	for _, format := range testFormats {
		t.Run(format, func(t *testing.T) {
			djangotest.EachDB(t, serializerModels(), func(t *testing.T, db *orm.DB) {
				ctx := context.Background()
				obj := mustCreate(t, db, &NaturalPKWithDefault{Name: "name"})
				allObjs, err := orm.Query[NaturalPKWithDefault](db).All(ctx)
				if err != nil {
					t.Fatalf("All: %v", err)
				}
				stringData, err := serializers.Serialize(
					ctx, db, format, allObjs,
					serializers.NaturalForeignKeys(),
					serializers.NaturalPrimaryKeys(),
				)
				if err != nil {
					t.Fatalf("Serialize: %v", err)
				}
				objs, err := collectDeserialized(ctx, db, format, stringData)
				if err != nil {
					t.Fatalf("Deserialize: %v", err)
				}
				djangotest.AssertEqual(t, len(objs), 1)
				got := objs[0].Object.(*NaturalPKWithDefault)
				djangotest.AssertEqual(t, got.ID, obj.ID)
			})
		})
	}
}

// Ported from Django: tests/serializers/test_natural.py::fk_as_pk_natural_key_not_called
// Check that deserializing a model whose primary key is a ForeignKey does not invoke its natural_key method.
func TestNaturalKeySerializerFKAsPKNaturalKeyNotCalled(t *testing.T) {
	t.Skip("FIXME-PORT: Go: ForeignKey with primary_key=True, Django: fk_as_pk_natural_key_not_called serializes and deserializes FKAsPKNoNaturalKey with a ForeignKey primary key")
}
